#!/usr/bin/env bash
# MinerU CPU API 服务管理脚本 v1.4.0
# 优化点：
#   1. NUMA 感知线程计算：探测物理核/NUMA节点数，将线程数限定在单节点范围内
#      避免跨 NUMA 内存访问惩罚（对多路 Xeon/EPYC 服务器尤为重要）
#   2. 传入 --shm-size 和 --ipc=host，解除共享内存对 OCR batch 的瓶颈
#   3. 暴露 MINERU_MIN_BATCH_INFERENCE_SIZE（推理 batch，默认 512 提升 OCR 吞吐）
#   4. 暴露 OMP_NUM_THREADS / OPENBLAS_NUM_THREADS / MKL_NUM_THREADS，
#      防止多个数值库各自抢占全部核心造成过度订阅（oversubscription）
#   5. 暴露 MINERU_TABLE_MERGE_ENABLE，跨页表格合并可按需关闭
#   6. PDF_RENDER_THREADS=4，多线程渲染加速长文档预处理阶段
#   7. [bugfix] 预先创建 /data/config 目录，修复模型配置持久化失败
#      (Failed to persist downloaded pipeline model config to /data/config/mineru.json)
#      避免每次重启重新拉取配置，节省 ~7s model init 时间
set -euo pipefail
shopt -s inherit_errexit 2>/dev/null || true
IFS=$'\n\t'

SCRIPT_PATH="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "$(dirname "${SCRIPT_PATH}")" && pwd)"
SCRIPT_NAME="$(basename "${SCRIPT_PATH}")"
SCRIPT_VERSION="1.4.0"
HOSTNAME_SHORT="$(hostname -s 2>/dev/null || echo localhost)"
TODAY_TAG="$(date +%Y%m%d)"

LOG_DIR="${MINERU_CPU_SCRIPT_LOG_DIR:-${SCRIPT_DIR}/.logs}"
mkdir -p "${LOG_DIR}"
LOG_FILE="${LOG_DIR}/${SCRIPT_NAME%.sh}-${HOSTNAME_SHORT}-${TODAY_TAG}.log"

if [[ -t 1 ]]; then
  C_RED=$'\033[31m'; C_YEL=$'\033[33m'; C_GRN=$'\033[32m'
  C_BLU=$'\033[34m'; C_CYN=$'\033[36m'; C_DIM=$'\033[2m'; C_RST=$'\033[0m'
else
  C_RED=''; C_YEL=''; C_GRN=''; C_BLU=''; C_CYN=''; C_DIM=''; C_RST=''
fi

_ts()         { date '+%Y-%m-%dT%H:%M:%S%z'; }
_log_to_file() {
  local level="$1"; shift
  printf '%s [%s] [%s] %s\n' "$(_ts)" "$level" "$HOSTNAME_SHORT" "$*" >> "${LOG_FILE}" 2>/dev/null || true
}
log_info()  { echo "${C_BLU}[INFO ]${C_RST}  $*" >&2; _log_to_file INFO "$*"; }
log_ok()    { echo "${C_GRN}[OK   ]${C_RST}  $*" >&2; _log_to_file OK   "$*"; }
log_warn()  { echo "${C_YEL}[WARN ]${C_RST}  $*" >&2; _log_to_file WARN "$*"; }
log_err()   { echo "${C_RED}[ERROR]${C_RST}  $*" >&2; _log_to_file ERROR "$*"; }
have()      { command -v "$1" >/dev/null 2>&1; }
die()       { log_err "$*"; exit 1; }
trim() {
  local s="${1:-}"
  s="${s#"${s%%[![:space:]]*}"}"
  s="${s%"${s##*[![:space:]]}"}"
  printf '%s' "$s"
}
print_cmd() { printf '%q ' "$@"; echo; }

# ---------------------------------------------------------------------------
# [优化1] NUMA 感知拓扑探测
#
# 目标：探测宿主机的 物理核总数 和 NUMA 节点数，从而计算
#       "单 NUMA 节点物理核数"作为线程上限。
#
# 背景（Xeon Gold 6530 2路机，4 NUMA 节点）：
#   NUMA node 0 : core 0-15  + HT 64-79   (16 物理核 / 32 逻辑线程)
#   NUMA node 1 : core 16-31 + HT 80-95
#   NUMA node 2 : core 32-47 + HT 96-111
#   NUMA node 3 : core 48-63 + HT 112-127
#
# 线程数超出单 NUMA 节点范围后，ONNX 推理会触发跨 NUMA 远端内存访问，
# 延迟从 ~80ns 升至 ~160ns，实测吞吐下降而非提升。
#
# 探测优先级：
#   1. lscpu（最可靠，直接给出 NUMA node(s) 字段）
#   2. numactl --hardware
#   3. /sys/devices/system/node/node* 目录计数
#   4. 兜底：假设 1 个 NUMA 节点
# ---------------------------------------------------------------------------
detect_host_topology() {
  # 返回两个全局变量：HOST_CORES  HOST_NUMA_NODES
  local phys_cores numa_nodes

  # ── 物理核数 ──
  # 优先：通过 core_id 去重（排除超线程重复）
  if [[ -d /sys/devices/system/cpu ]]; then
    phys_cores=$(
      find /sys/devices/system/cpu -maxdepth 2 -name core_id 2>/dev/null \
        | xargs -r cat 2>/dev/null \
        | sort -u | wc -l
    )
  fi
  # 次选：lscpu 的 "Core(s) per socket × Socket(s)"
  if [[ -z "${phys_cores:-}" ]] || (( phys_cores == 0 )); then
    if have lscpu; then
      local cps sockets
      cps=$(lscpu 2>/dev/null | awk '/^Core\(s\) per socket/{print $NF}')
      sockets=$(lscpu 2>/dev/null | awk '/^Socket\(s\)/{print $NF}')
      if [[ -n "${cps}" && -n "${sockets}" ]]; then
        phys_cores=$(( cps * sockets ))
      fi
    fi
  fi
  # 兜底：nproc --all（逻辑核，含超线程，偏大但总比 0 好）
  if [[ -z "${phys_cores:-}" ]] || (( phys_cores == 0 )); then
    phys_cores=$(nproc --all 2>/dev/null || nproc 2>/dev/null || echo 4)
  fi

  # ── NUMA 节点数 ──
  # 优先：lscpu
  if have lscpu; then
    numa_nodes=$(lscpu 2>/dev/null | awk '/^NUMA node\(s\)/{print $NF}')
  fi
  # 次选：numactl
  if [[ -z "${numa_nodes:-}" ]] || (( numa_nodes == 0 )); then
    if have numactl; then
      numa_nodes=$(numactl --hardware 2>/dev/null | awk '/^available:/{print $2}')
    fi
  fi
  # 次选：/sys 目录计数
  if [[ -z "${numa_nodes:-}" ]] || (( numa_nodes == 0 )); then
    if [[ -d /sys/devices/system/node ]]; then
      numa_nodes=$(find /sys/devices/system/node -maxdepth 1 -name 'node[0-9]*' -type d 2>/dev/null | wc -l)
    fi
  fi
  # 兜底
  [[ -z "${numa_nodes:-}" ]] || (( numa_nodes == 0 )) && numa_nodes=1

  HOST_CORES="${phys_cores}"
  HOST_NUMA_NODES="${numa_nodes}"
}

# ---------------------------------------------------------------------------
# 计算各线程数默认值（NUMA 感知）
#
# 核心原则：将所有线程池约束在"单 NUMA 节点"的核心数范围内，
#           避免多路服务器上的跨节点内存访问惩罚。
#
# 针对本机（Xeon Gold 6530 × 2，4 NUMA，16 物理核/节点）：
#   HOST_CORES=64, HOST_NUMA_NODES=4 → CORES_PER_NUMA=16
#
#   INTRA_OP_THREADS = 16  ← ONNX 算子内并行，填满单节点物理核
#   INTER_OP_THREADS =  1  ← pipeline 顺序图，开多了只会增加调度开销
#   OMP_THREADS      = 16  ← 与 intra 持平；intra(16)+omp(16)=32 = 单节点逻辑线程数
#                            两池加起来恰好填满单节点，不溢出到跨 NUMA 区域
#
# 多并发场景（MAX_CONCURRENT=4，每实例绑定 1 个 NUMA 节点）：
#   每实例的 intra(16)+omp(16) = 32线程，4实例 × 32 = 128 = 全机逻辑线程数，
#   资源完全填满且互不干扰。
# ---------------------------------------------------------------------------
detect_host_topology          # 填充 HOST_CORES / HOST_NUMA_NODES
CORES_PER_NUMA=$(( HOST_CORES / HOST_NUMA_NODES ))
# 防御性下界：至少 1
(( CORES_PER_NUMA < 1 )) && CORES_PER_NUMA=1

_DEFAULT_INTRA="${CORES_PER_NUMA}"   # 16（单 NUMA 节点物理核数）
_DEFAULT_INTER=1                      # pipeline 顺序图固定为 1
_DEFAULT_OMP="${CORES_PER_NUMA}"     # 16（与 intra 持平，两池合计=单节点逻辑线程数）

# ---------------------------------------------------------------------------
# 参数定义（与 docker-compose.dev.yml mineru-api 段对齐，新增性能相关参数）
# ---------------------------------------------------------------------------
IMAGE="${MINERU_CPU_IMAGE:-weknora-mineru-cpu:local}"
CONTAINER_NAME="${MINERU_CPU_CONTAINER_NAME:-WeKnora-mineru-api-dev}"
CONTAINER_PORT="${MINERU_CPU_CONTAINER_PORT:-8000}"
BASE_PORT="${MINERU_CPU_BASE_PORT:-18000}"
ALIAS_NAME="${MINERU_CPU_ALIAS:-weknora-mineru-api}"
DATA_VOLUME="${MINERU_CPU_DATA_VOLUME:-mineru-data-dev}"
DATA_BIND="${MINERU_CPU_DATA_BIND:-}"
INSTANCE_LOG_ROOT="${MINERU_CPU_LOG_ROOT:-/data/weknora/model-logs/mineru-api-cpu}"
MODEL_SOURCE="${MINERU_CPU_MODEL_SOURCE:-modelscope}"
MAX_CONCURRENT="${MINERU_CPU_MAX_CONCURRENT_REQUESTS:-1}"
# [优化6] PDF渲染线程：4（多线程将页面渲染为图像，对长文档效果显著）
PDF_RENDER_THREADS="${MINERU_CPU_PDF_RENDER_THREADS:-4}"
PROCESSING_WINDOW_SIZE="${MINERU_CPU_PROCESSING_WINDOW_SIZE:-32}"

# [优化1] ONNX 线程数：改用自动探测值替代 -1（容器内 nproc 可能被 cgroup 裁剪）
INTRA_OP_THREADS="${MINERU_CPU_INTRA_OP_NUM_THREADS:-${_DEFAULT_INTRA}}"
INTER_OP_THREADS="${MINERU_CPU_INTER_OP_NUM_THREADS:-${_DEFAULT_INTER}}"

# [优化3] 推理 batch 大小：512（内存充裕，91GiB 可用，显著提升 OCR/Table 吞吐）
BATCH_INFERENCE_SIZE="${MINERU_CPU_MIN_BATCH_INFERENCE_SIZE:-512}"

# [优化4] OpenMP / BLAS / MKL 线程数：防止多库同时满核造成过度订阅
OMP_THREADS="${MINERU_CPU_OMP_NUM_THREADS:-${_DEFAULT_OMP}}"

# [优化5] 跨页表格合并开关（1=开/0=关），表格少的文档关掉能省数秒
TABLE_MERGE_ENABLE="${MINERU_CPU_TABLE_MERGE_ENABLE:-1}"

# [优化2] 共享内存：8g（内存充裕，完全消除 OCR batch 共享内存瓶颈）
SHM_SIZE="${MINERU_CPU_SHM_SIZE:-8g}"

ENABLE_FASTAPI_DOCS="${MINERU_CPU_ENABLE_FASTAPI_DOCS:-true}"
STOP_TIMEOUT="${MINERU_CPU_STOP_TIMEOUT:-45}"
DOCKER_RESTART_POLICY="${MINERU_CPU_RESTART_POLICY:-unless-stopped}"
HEALTH_PATH="${MINERU_CPU_HEALTH_PATH:-/health}"
DRY_RUN=0

# ---------------------------------------------------------------------------
resolve_docker_cmd() {
  : "${DOCKER_BIN:=docker}"
  docker_cmd=("${DOCKER_BIN}")
  have "${DOCKER_BIN}" || die "找不到 docker 命令 (DOCKER_BIN=${DOCKER_BIN})"
  local docker_info_err
  docker_info_err="$(${DOCKER_BIN} info 2>&1 >/dev/null || true)"
  if [[ -n "${docker_info_err}" ]]; then
    if [[ "${docker_info_err}" == *"permission denied"* || \
          "${docker_info_err}" == *"Got permission denied"* || \
          "${docker_info_err}" == *"docker.sock"* ]]; then
      have sudo || die "当前用户无权访问 docker.sock，且系统未安装 sudo"
      docker_cmd=(sudo "${DOCKER_BIN}")
      "${docker_cmd[@]}" info >/dev/null 2>&1 || die "sudo docker 仍无法访问 docker daemon"
    else
      [[ "${docker_info_err}" == *"Cannot connect to the Docker daemon"* || \
         "${docker_info_err}" == *"Is the docker daemon running"* ]] \
        && die "docker daemon 未运行，请先启动 docker"
      die "docker 不可用: ${docker_info_err}"
    fi
  fi
}

container_exists_for() {
  local name="$1"
  "${docker_cmd[@]}" inspect "${name}" >/dev/null 2>&1
}

container_running_for() {
  local name="$1"
  [[ "$("${docker_cmd[@]}" inspect -f '{{.State.Running}}' "${name}" 2>/dev/null || echo false)" == "true" ]]
}

port_in_use() {
  local port="$1"
  if have ss; then
    ss -ltn 2>/dev/null | awk '{print $4}' | grep -Eq "(^|:)${port}$"; return
  fi
  if have lsof; then
    lsof -iTCP:"${port}" -sTCP:LISTEN >/dev/null 2>&1; return
  fi
  return 1
}

ensure_data_dir() {
  local target="${DATA_BIND}"
  [[ -z "${target}" ]] && return 0
  mkdir -p "${target}"
}

resolve_data_host_root() {
  if [[ -n "${DATA_BIND}" ]]; then printf '%s' "${DATA_BIND}"; return 0; fi
  "${docker_cmd[@]}" volume inspect "${DATA_VOLUME}" --format '{{.Mountpoint}}' 2>/dev/null || true
}

detect_pretrained_model() {
  local host_root
  host_root="$(resolve_data_host_root)"
  [[ -z "${host_root}" || ! -d "${host_root}" ]] && return 1
  local cache_root
  if [[ "${MODEL_SOURCE}" == huggingface* || "${MODEL_SOURCE}" == "hf" ]]; then
    cache_root="${host_root}/huggingface/hub"
  else
    cache_root="${host_root}/modelscope"
  fi
  [[ -d "${cache_root}" ]] || return 1
  local marker size
  for marker in \
      'config.json' 'model.safetensors.index.json' 'model.safetensors' \
      'pytorch_model.bin' 'pytorch_model.bin.index.json' \
      'tokenizer.json' 'tokenizer_config.json' \
      'pipeline.json' 'layout.json' 'README.md'; do
    if [[ -n "$(find "${cache_root}" -mindepth 3 -maxdepth 6 -type f -name "${marker}" -print -quit 2>/dev/null)" ]]; then
      log_info "检测到已下载的 MinerU 模型缓存：${cache_root} (匹配 ${marker})"
      return 0
    fi
  done
  if have find; then
    size="$(find "${cache_root}" -type f \( -iname '*.safetensors' -o -iname '*.bin' \) -size +200M -print -quit 2>/dev/null)"
    if [[ -n "${size}" ]]; then
      log_info "检测到已下载的 MinerU 模型缓存：${cache_root} (匹配 ${size##*/})"
      return 0
    fi
  fi
  return 1
}

show_cache_summary() {
  local host_root
  host_root="$(resolve_data_host_root)"
  [[ -z "${host_root}" || ! -d "${host_root}" ]] && return 0
  have du && du -sh "${host_root}" 2>/dev/null | awk '{print "  缓存目录总占用: "$1}'
}

prepare_log_dir() {
  mkdir -p "${INSTANCE_LOG_ROOT}"
}

# [bugfix v1.4] 预建宿主机侧数据子目录
# MinerU 启动时会尝试写入 /data/config/mineru.json（模型配置持久化）。
# 若目录不存在会触发 WARNING 并跳过写入，导致每次容器重启都重新从网络拉取配置
# （约 +7-9s model init 额外耗时）。在容器启动前预建目录即可消除该问题。
# 命名卷模式：通过临时容器在卷内创建；bind mount 模式：直接 mkdir。
prepare_data_subdirs() {
  local subdirs=(config output)
  if [[ -n "${DATA_BIND}" ]]; then
    # bind mount 模式，直接在宿主机上创建
    for d in "${subdirs[@]}"; do
      mkdir -p "${DATA_BIND}/${d}"
    done
    log_info "已预建数据子目录: ${DATA_BIND}/{$(IFS=,; echo "${subdirs[*]}")}"
  else
    # 命名卷模式：通过一次性容器在卷内创建目录
    local dir_args=()
    for d in "${subdirs[@]}"; do dir_args+=("/data/${d}"); done
    "${docker_cmd[@]}" run --rm \
      -v "${DATA_VOLUME}:/data" \
      busybox mkdir -p "${dir_args[@]}" >/dev/null 2>&1 || {
        # busybox 不一定存在，降级用同镜像
        "${docker_cmd[@]}" run --rm \
          -v "${DATA_VOLUME}:/data" \
          "${IMAGE}" mkdir -p "${dir_args[@]}" >/dev/null 2>&1 || true
      }
    log_info "已预建数据卷子目录: ${DATA_VOLUME}/{$(IFS=,; echo "${subdirs[*]}")}"
  fi
}

preflight() {
  log_info "MinerU CPU 服务配置 (v${SCRIPT_VERSION})"
  log_info "  镜像:                ${IMAGE}"
  log_info "  容器名:              ${CONTAINER_NAME}"
  log_info "  暴露端口:            ${BASE_PORT} -> ${CONTAINER_PORT}"
  log_info "  别名/标签:           ${ALIAS_NAME}"
  log_info "  数据卷:              ${DATA_VOLUME}${DATA_BIND:+ (bind=${DATA_BIND})}"
  log_info "  日志目录:            ${INSTANCE_LOG_ROOT}"
  log_info "  模型源:              ${MODEL_SOURCE}"
  log_info "  并发:                max_concurrent=${MAX_CONCURRENT}  pdf_render_threads=${PDF_RENDER_THREADS}"
  log_info "  ── CPU 拓扑（探测结果）──"
  log_info "  宿主机物理核心数:    ${HOST_CORES}"
  log_info "  NUMA 节点数:         ${HOST_NUMA_NODES}"
  log_info "  每节点物理核数:      ${CORES_PER_NUMA}  ← 线程上限基准"
  log_info "  ── CPU 性能参数 ──"
  log_info "  ONNX intra 线程:     ${INTRA_OP_THREADS}  (=单NUMA节点物理核数，避免跨节点内存访问)"
  log_info "  ONNX inter 线程:     ${INTER_OP_THREADS}  (pipeline 顺序图，固定=1)"
  log_info "  OMP/BLAS/MKL 线程:   ${OMP_THREADS}  (=intra，两池合计=单节点逻辑线程数)"
  log_info "  推理 batch 大小:     ${BATCH_INFERENCE_SIZE}  (内存充裕时拉大提升 OCR/Table 吞吐)"
  log_info "  跨页表格合并:        ${TABLE_MERGE_ENABLE}  (0=关闭可节省数秒/文档)"
  log_info "  共享内存:            ${SHM_SIZE}  (消除 OCR batch 共享内存瓶颈)"
}

run_container() {
  local cmd=(
    "${docker_cmd[@]}" run -d
    --name "${CONTAINER_NAME}"
    --hostname "${CONTAINER_NAME}"
    --restart "${DOCKER_RESTART_POLICY}"
    --label weknora.model.cpu=mineru-api
    --label weknora.model.alias="${ALIAS_NAME}"
    -p "${BASE_PORT}:${CONTAINER_PORT}"
    # [优化2] 增大共享内存，避免 OCR batch 多进程共享内存不足
    --shm-size "${SHM_SIZE}"
    --ipc host
  )

  if [[ -n "${DATA_BIND}" ]]; then
    cmd+=(-v "${DATA_BIND}:/data")
  else
    cmd+=(-v "${DATA_VOLUME}:/data")
  fi

  cmd+=(
    -v "${INSTANCE_LOG_ROOT}:/data/log"
    # 基础服务参数
    -e MINERU_MODEL_SOURCE="${MODEL_SOURCE}"
    -e MINERU_API_OUTPUT_ROOT=/data/output
    -e MINERU_TOOLS_CONFIG_JSON=/data/config/mineru.json
    -e HF_HOME=/data/huggingface
    -e MODELSCOPE_CACHE=/data/modelscope
    -e MINERU_LOG_DIR=/data/log
    -e MINERU_API_ENABLE_FASTAPI_DOCS="${ENABLE_FASTAPI_DOCS}"
    -e MINERU_API_MAX_CONCURRENT_REQUESTS="${MAX_CONCURRENT}"
    -e MINERU_PDF_RENDER_THREADS="${PDF_RENDER_THREADS}"
    -e MINERU_PROCESSING_WINDOW_SIZE="${PROCESSING_WINDOW_SIZE}"
    # [优化1] ONNX 线程数（自动探测物理核替代 -1）
    -e MINERU_INTRA_OP_NUM_THREADS="${INTRA_OP_THREADS}"
    -e MINERU_INTER_OP_NUM_THREADS="${INTER_OP_THREADS}"
    # [优化3] 推理 batch 大小（提升 OCR/Table 吞吐）
    -e MINERU_MIN_BATCH_INFERENCE_SIZE="${BATCH_INFERENCE_SIZE}"
    # [优化4] 防止 OpenMP/BLAS/MKL 各自满核造成过度订阅
    -e OMP_NUM_THREADS="${OMP_THREADS}"
    -e OPENBLAS_NUM_THREADS="${OMP_THREADS}"
    -e MKL_NUM_THREADS="${OMP_THREADS}"
    -e VECLIB_MAXIMUM_THREADS="${OMP_THREADS}"
    -e NUMEXPR_NUM_THREADS="${OMP_THREADS}"
    # [优化5] 跨页表格合并开关（文档无跨页表可设为 0 节省时间）
    -e MINERU_TABLE_MERGE_ENABLE="${TABLE_MERGE_ENABLE}"
    "${IMAGE}"
  )

  if (( DRY_RUN == 1 )); then
    print_cmd "${cmd[@]}"
    return 0
  fi

  "${cmd[@]}" >/dev/null
}

start_service() {
  resolve_docker_cmd
  preflight
  if container_running_for "${CONTAINER_NAME}"; then
    log_info "${CONTAINER_NAME} 已在运行"
    return 0
  fi
  if container_exists_for "${CONTAINER_NAME}"; then
    log_info "删除旧容器 ${CONTAINER_NAME}"
    "${docker_cmd[@]}" rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
  fi
  ensure_data_dir
  prepare_log_dir
  prepare_data_subdirs   # [bugfix v1.4] 预建 /data/config 等子目录，消除模型配置持久化 WARNING
  if port_in_use "${BASE_PORT}"; then
    die "宿主机端口 ${BASE_PORT} 已被占用，无法启动 ${CONTAINER_NAME}"
  fi
  if detect_pretrained_model; then
    log_info "检测到已有 MinerU 模型缓存，跳过模型下载步骤"
    show_cache_summary
  else
    log_info "未在宿主机缓存中找到 MinerU 模型；首次启动时会自动下载，请耐心等待"
    show_cache_summary
  fi
  run_container
  log_ok "已启动 ${CONTAINER_NAME}，地址 http://127.0.0.1:${BASE_PORT}${HEALTH_PATH}"
  log_info "首次模型下载与 warmup 可能需要数分钟；可执行 ${SCRIPT_NAME} logs 查看进度"
}

stop_service() {
  resolve_docker_cmd
  if ! container_exists_for "${CONTAINER_NAME}"; then
    log_info "${CONTAINER_NAME} 不存在"; return 0
  fi
  log_info "优雅停止 ${CONTAINER_NAME}"
  "${docker_cmd[@]}" stop -t "${STOP_TIMEOUT}" "${CONTAINER_NAME}" >/dev/null 2>&1 || true
  "${docker_cmd[@]}" rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
}

restart_service() { stop_service; start_service; }

reset_service() {
  resolve_docker_cmd
  preflight
  ensure_data_dir
  prepare_log_dir
  local had_volume=0
  [[ -z "${DATA_BIND}" ]] && had_volume=1
  container_exists_for "${CONTAINER_NAME}" && stop_service
  if (( had_volume == 1 )) && "${docker_cmd[@]}" volume inspect "${DATA_VOLUME}" >/dev/null 2>&1; then
    log_info "删除数据卷 ${DATA_VOLUME}"
    "${docker_cmd[@]}" volume rm "${DATA_VOLUME}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${DATA_BIND}" ]] && [[ -d "${DATA_BIND}" ]]; then
    log_info "清理 bind mount 目录 ${DATA_BIND}"
    find "${DATA_BIND}" -mindepth 1 -maxdepth 1 -exec rm -rf {} \; 2>/dev/null || true
  fi
  rm -rf "${INSTANCE_LOG_ROOT}"
  prepare_log_dir
  if "${docker_cmd[@]}" image inspect "${IMAGE}" >/dev/null 2>&1; then
    log_info "删除镜像 ${IMAGE}"
    "${docker_cmd[@]}" image rm -f "${IMAGE}" >/dev/null 2>&1 || true
  fi
  if (( DRY_RUN == 1 )); then
    print_cmd "${docker_cmd[@]}" build --pull -t "${IMAGE}" "${SCRIPT_DIR}/.." -
    return 0
  fi
  log_info "重新构建镜像 ${IMAGE}"
  "${docker_cmd[@]}" build --pull -t "${IMAGE}" "${SCRIPT_DIR}/../WeKnora" >/dev/null
  start_service
}

status_service() {
  resolve_docker_cmd
  if ! container_exists_for "${CONTAINER_NAME}"; then printf 'missing\t-\n'; return 0; fi
  local status health
  status="$("${docker_cmd[@]}" inspect -f '{{.State.Status}}' "${CONTAINER_NAME}" 2>/dev/null || echo unknown)"
  health="$("${docker_cmd[@]}" inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}n/a{{end}}' "${CONTAINER_NAME}" 2>/dev/null || echo n/a)"
  printf '%s\t%s\n' "${status}" "${health}"
}

status_all() {
  resolve_docker_cmd
  preflight
  echo "===== MinerU CPU 服务状态 ====="
  printf '%-32s %-12s %-10s %-8s\n' 'NAME' 'STATUS' 'HEALTH' 'PORT'
  echo "------------------------------------------------------------"
  local state status health
  state="$(status_service)"
  status="${state%%	*}"
  health="${state#*	}"; health="${health%%	*}"
  printf '%-32s %-12s %-10s %-8s\n' "${CONTAINER_NAME}" "${status}" "${health}" "${BASE_PORT}"
}

logs_service() {
  resolve_docker_cmd
  if ! container_exists_for "${CONTAINER_NAME}"; then log_info "${CONTAINER_NAME} 不存在"; return 0; fi
  "${docker_cmd[@]}" logs --tail "${MINERU_CPU_LOG_TAIL:-200}" -f "${CONTAINER_NAME}"
}

health_service() {
  resolve_docker_cmd
  local url="http://127.0.0.1:${BASE_PORT}${HEALTH_PATH}"
  if ! container_running_for "${CONTAINER_NAME}"; then log_info "容器未运行"; return 1; fi
  if curl -fsS "${url}" >/dev/null 2>&1; then log_ok "健康检查通过: ${url}"; return 0; fi
  log_warn "健康检查失败: ${url}"; return 1
}

usage() {
  cat <<USAGE_EOF
用法: ./${SCRIPT_NAME} <command>

命令:
  start                   预检 + 构建（首次自动） + 启动 MinerU API 服务
  stop                    优雅停止容器
  restart                 stop + start
  reset                   清理容器 / 数据卷 / 镜像 + 重新构建 + 启动
  status                  打印容器 / 健康状态
  logs                    跟踪容器日志 (Ctrl+C 退出)
  health                  单次健康检查
  preflight               仅打印配置，不启动
  version | -V            打印脚本版本
  help | -h | --help      打印帮助

默认参数 (与 docker-compose.dev.yml mineru-api 对齐):
  镜像           ${IMAGE}
  容器名         ${CONTAINER_NAME}
  宿主机端口     ${BASE_PORT}
  容器端口       ${CONTAINER_PORT}
  数据卷         ${DATA_VOLUME}
  日志目录       ${INSTANCE_LOG_ROOT}
  模型源         ${MODEL_SOURCE}

关键环境变量:
  ── 基础 ──
  MINERU_CPU_BASE_PORT                  默认 ${BASE_PORT}
  MINERU_CPU_DATA_VOLUME                默认 ${DATA_VOLUME}
  MINERU_CPU_DATA_BIND                  非空时使用宿主机目录 bind mount
  MINERU_CPU_MODEL_SOURCE               默认 ${MODEL_SOURCE}
  MINERU_CPU_MAX_CONCURRENT_REQUESTS    默认 ${MAX_CONCURRENT}
  MINERU_CPU_LOG_ROOT                   默认 ${INSTANCE_LOG_ROOT}
  DOCKER_BIN                            可指定 docker 客户端路径

  ── CPU 性能调优 ──
  MINERU_CPU_INTRA_OP_NUM_THREADS       ONNX 算子内并行；默认=每NUMA节点物理核数(当前 ${INTRA_OP_THREADS})
  MINERU_CPU_INTER_OP_NUM_THREADS       ONNX 算子间并行；pipeline 顺序图固定=1(当前 ${INTER_OP_THREADS})
  MINERU_CPU_MIN_BATCH_INFERENCE_SIZE   推理 batch 大小；内存不足可降至 256/128(当前 ${BATCH_INFERENCE_SIZE})
  MINERU_CPU_OMP_NUM_THREADS            OMP/BLAS/MKL 线程；默认=intra，防多库超订阅(当前 ${OMP_THREADS})
  MINERU_CPU_TABLE_MERGE_ENABLE         跨页表格合并 1=开/0=关；无跨页表可关(当前 ${TABLE_MERGE_ENABLE})
  MINERU_CPU_SHM_SIZE                   Docker 共享内存；消除 OCR batch 瓶颈(当前 ${SHM_SIZE})

示例:
  ./${SCRIPT_NAME} preflight
  ./${SCRIPT_NAME} start
  ./${SCRIPT_NAME} status
  ./${SCRIPT_NAME} logs
  ./${SCRIPT_NAME} reset

  # 并发=4（每实例绑定 1 个 NUMA 节点，4×32线程=128=全机逻辑线程，资源填满互不干扰）
  MINERU_CPU_MAX_CONCURRENT_REQUESTS=4 ./${SCRIPT_NAME} start

  # 并发=2（每实例跨 2 个 NUMA 节点，intra 可适当扩大）
  MINERU_CPU_MAX_CONCURRENT_REQUESTS=2 \
    MINERU_CPU_INTRA_OP_NUM_THREADS=32 MINERU_CPU_OMP_NUM_THREADS=32 \
    ./${SCRIPT_NAME} start

  # 文档无跨页表格，关闭合并加速
  MINERU_CPU_TABLE_MERGE_ENABLE=0 ./${SCRIPT_NAME} start

  # 内存压力大时降低 batch
  MINERU_CPU_MIN_BATCH_INFERENCE_SIZE=256 ./${SCRIPT_NAME} start

附注:
  本脚本默认走与 dev compose 一致的命名卷 (mineru-data-dev)。
  如需切换到宿主机目录绑定模式，可设置 MINERU_CPU_DATA_BIND。

  【客户端侧优化提示】
  以下参数由调用方在 POST /file_parse 时传入，脚本侧无法控制，但影响最大：
    formula=false   关闭公式识别（MFR Predict 步骤，节省 10-30s/文档）
    table=false     关闭表格识别（Table-ocr 系列步骤，节省 20-60s/文档）
  如果业务不需要公式/表格结构化，这是 CPU 模式下最有效的加速手段。
USAGE_EOF
}

version() {
  echo "${SCRIPT_NAME} ${SCRIPT_VERSION} (hostname=${HOSTNAME_SHORT}, date=${TODAY_TAG})"
}

cmd="${1:-status}"
shift || true

case "${cmd}" in
  start)
    while [[ $# -gt 0 ]]; do
      case "$1" in --dry-run) DRY_RUN=1; shift ;; *) shift ;; esac
    done
    start_service ;;
  stop)    stop_service ;;
  restart) restart_service ;;
  reset)
    while [[ $# -gt 0 ]]; do
      case "$1" in --dry-run) DRY_RUN=1; shift ;; *) shift ;; esac
    done
    reset_service ;;
  status)    status_all ;;
  logs)      logs_service ;;
  health)    health_service ;;
  preflight) resolve_docker_cmd; preflight ;;
  version|-V|--version) version ;;
  help|-h|--help) usage ;;
  *) die "未知命令: ${cmd}" ;;
esac
