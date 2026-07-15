#!/usr/bin/env bash
# =============================================================================
#  qwen_vllm_service_enterprise.sh
#  -----------------------------------------------------------------------------
#  企业级生产脚本：在双路 Xeon Gold 6530 (64C/128T) + 4×RTX 4090 + 128GB DDR5
#  + Ubuntu 26.04 + vLLM 0.23.x 平台上，部署 Qwen3.6-27B-FP8 张量并行推理服务。
#
#  关键能力：
#    - 自动硬件检测（CPU/内存/GPU/NUMA/驱动版本）
#    - NUMA 自动绑定（GPU→NUMA→CPU 三段映射）
#    - 自适应线程调优（每 worker 16 线程，4 worker 覆盖 64 线程）
#    - 系统级调优（sysctl / ulimit / 透明大页 / 调度器）
#    - 容器运行时调优（cpu-shares / memory-swappiness / shm / ipc / ulimit）
#    - 模型/端口/缓存/镜像前置校验
#    - 启动后健康检查 + 可选 watch 循环
#    - 性能基准测试（简单 TTFT/TPS 采样）
#    - 结构化日志（文件 + 时间戳）
#
#  用法：见末尾 USAGE 与 README。
#
#  作者：hlsa-ops / 2026-07-01 / 重写自 qwen_vllm_service_gpt.sh
# =============================================================================
set -euo pipefail
shopt -s inherit_errexit 2>/dev/null || true
IFS=$'\n\t'

# -----------------------------------------------------------------------------
# 0. 元数据与路径
# -----------------------------------------------------------------------------
SCRIPT_PATH="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "$(dirname "${SCRIPT_PATH}")" && pwd)"
SCRIPT_NAME="$(basename "${SCRIPT_PATH}")"
SCRIPT_VERSION="1.0.0-enterprise"
HOSTNAME_SHORT="$(hostname -s 2>/dev/null || echo localhost)"
TODAY_TAG="$(date +%Y%m%d)"

# 日志目录
LOG_DIR="${QWEN_VLLM_LOG_DIR:-/var/log/qwen-vllm}"
mkdir -p "${LOG_DIR}" 2>/dev/null || LOG_DIR="${SCRIPT_DIR}/.logs"
mkdir -p "${LOG_DIR}" 2>/dev/null || true
LOG_FILE="${LOG_DIR}/${HOSTNAME_SHORT}-${TODAY_TAG}.log"

# 颜色（仅当输出到 TTY 时启用）
if [[ -t 1 ]]; then
  C_RED=$'\033[31m'; C_YEL=$'\033[33m'; C_GRN=$'\033[32m'
  C_BLU=$'\033[34m'; C_CYN=$'\033[36m'; C_DIM=$'\033[2m'; C_RST=$'\033[0m'
else
  C_RED=''; C_YEL=''; C_GRN=''; C_BLU=''; C_CYN=''; C_DIM=''; C_RST=''
fi

# -----------------------------------------------------------------------------
# 1. 统一日志函数
# -----------------------------------------------------------------------------
_ts() { date '+%Y-%m-%dT%H:%M:%S%z'; }
_log_to_file() {
  local _lvl="$1"; shift
  printf '%s [%s] [%s] %s\n' "$(_ts)" "${_lvl}" "${HOSTNAME_SHORT}" "$*" >> "${LOG_FILE}" 2>/dev/null || true
}

log_info()  { echo "${C_BLU}[INFO ]${C_RST}  $*" >&2; _log_to_file INFO  "$*"; }
log_ok()    { echo "${C_GRN}[OK   ]${C_RST}  $*" >&2; _log_to_file OK    "$*"; }
log_warn()  { echo "${C_YEL}[WARN ]${C_RST}  $*" >&2; _log_to_file WARN  "$*"; }
log_err()   { echo "${C_RED}[ERROR]${C_RST}  $*" >&2; _log_to_file ERROR "$*"; }
log_debug() { [[ "${QWEN_VLLM_DEBUG:-0}" == "1" ]] && echo "${C_DIM}[DEBUG]${C_RST}  $*" >&2 && _log_to_file DEBUG "$*" || true; }

# -----------------------------------------------------------------------------
# 2. 默认配置（可通过环境变量覆盖）
# -----------------------------------------------------------------------------
# 2.1 容器/镜像/端口
CONTAINER_NAME="${QWEN_VLLM_CONTAINER_NAME:-weknora-qwen-vllm}"
IMAGE="${QWEN_VLLM_IMAGE:-vllm/vllm-openai:latest}"
HOST_PORT="${QWEN_VLLM_PORT:-8000}"
CONTAINER_PORT="${QWEN_VLLM_CONTAINER_PORT:-8000}"
API_KEY="${QWEN_VLLM_API_KEY:-sk-hlsa-local-vllm}"
STOP_TIMEOUT="${QWEN_VLLM_STOP_TIMEOUT:-45}"
SHM_SIZE="${QWEN_VLLM_SHM_SIZE:-64g}"

# 2.2 模型
MODEL_SOURCE="${QWEN_VLLM_MODEL:-Qwen/Qwen3.6-27B-FP8}"
SERVED_MODEL_NAME="${QWEN_VLLM_SERVED_MODEL_NAME:-qwen3.6-27b-fp8}"
HF_CACHE_ROOT="${QWEN_VLLM_HF_CACHE_ROOT:-/data/models/hf-cache}"
MODEL_ROOT="${QWEN_VLLM_MODEL_ROOT:-/data/models/llm}"
HF_ENDPOINT_VALUE="${HF_ENDPOINT:-}"
CONTAINER_HF_CACHE_ROOT="${QWEN_VLLM_CONTAINER_HF_CACHE_ROOT:-/root/.cache/huggingface}"
EFFECTIVE_MODEL_SOURCE="$MODEL_SOURCE"

# 2.3 推理参数（vLLM）
TP_SIZE="${QWEN_VLLM_TP_SIZE:-4}"
PP_SIZE="${QWEN_VLLM_PP_SIZE:-1}"
GPU_IDS="${QWEN_VLLM_GPU_IDS:-0,1,2,3}"
GPU_MEMORY_UTILIZATION="${QWEN_VLLM_GPU_MEMORY_UTILIZATION:-0.90}"
MAX_MODEL_LEN="${QWEN_VLLM_MAX_MODEL_LEN:-262144}"
MAX_NUM_SEQS="${QWEN_VLLM_MAX_NUM_SEQS:-768}"
MAX_NUM_BATCHED_TOKENS="${QWEN_VLLM_MAX_NUM_BATCHED_TOKENS:-16384}"
ENFORCE_EAGER="${QWEN_VLLM_ENFORCE_EAGER:-0}"
ENABLE_AUTO_TOOL_CHOICE="${QWEN_VLLM_ENABLE_AUTO_TOOL_CHOICE:-1}"
TOOL_CALL_PARSER="${QWEN_VLLM_TOOL_CALL_PARSER:-qwen3_xml}"
REASONING_PARSER="${QWEN_VLLM_REASONING_PARSER:-qwen3}"
DEFAULT_CHAT_TEMPLATE_KWARGS="${QWEN_VLLM_DEFAULT_CHAT_TEMPLATE_KWARGS:-{\"enable_thinking\": false}}"
ENABLE_MTP="${QWEN_VLLM_ENABLE_MTP:-1}"
KV_CACHE_DTYPE="${QWEN_VLLM_KV_CACHE_DTYPE:-fp8}"

# 2.4 调度参数（用户硬性要求）
CPU_THREADS_PER_WORKER="${QWEN_VLLM_CPU_THREADS_PER_WORKER:-16}"  # 固定 16
DOCKER_CPU_SHARES="${QWEN_VLLM_CPU_SHARES:-4096}"                 # 默认 1024 → 4096
DOCKER_MEM_SWAPPINESS="${QWEN_VLLM_MEM_SWAPPINESS:-0}"           # 禁用 swap
MALLOC_ARENA_MAX="${QWEN_VLLM_MALLOC_ARENA_MAX:-2}"               # NUMA 友好
UV_THREADPOOL_SIZE="${QWEN_VLLM_UV_THREADPOOL_SIZE:-64}"          # libuv 线程池
CUDA_DEVICE_MAX_CONNECTIONS="${QWEN_VLLM_CUDA_DEVICE_MAX_CONNECTIONS:-32}"  # 允许并发
CUDA_MODULE_LOADING="${QWEN_VLLM_CUDA_MODULE_LOADING:-LAZY}"      # 按需加载
NCCL_IGNORE_CPU_AFFINITY="${QWEN_VLLM_NCCL_IGNORE_CPU_AFFINITY:-1}"
#NCCL_BUFFSIZE="${QWEN_VLLM_NCCL_BUFFSIZE:-33554432}"              # 32 MiB
NCCL_BUFFSIZE="${QWEN_VLLM_NCCL_BUFFSIZE:-8388608}"              # 8 MiB
NCCL_P2P_LEVEL="${QWEN_VLLM_NCCL_P2P_LEVEL:-SYS}"
NCCL_IB_DISABLE="${QWEN_VLLM_NCCL_IB_DISABLE:-1}"                 # 单机无 IB
NCCL_DEBUG="${QWEN_VLLM_NCCL_DEBUG:-WARN}"
NCCL_SOCKET_IFNAME="${QWEN_VLLM_NCCL_SOCKET_IFNAME:-^lo,docker}"

# 2.5 系统调优开关
APPLY_SYSCTLS="${QWEN_VLLM_APPLY_SYSCTLS:-1}"
APPLY_TRANSPARENT_HUGEPAGE="${QWEN_VLLM_APPLY_TRANSPARENT_HUGEPAGE:-1}"
APPLY_CPU_GOVERNOR="${QWEN_VLLM_APPLY_CPU_GOVERNOR:-1}"

# 2.6 健康检查/基准
HEALTH_INTERVAL="${QWEN_VLLM_HEALTH_INTERVAL:-30s}"
HEALTH_TIMEOUT="${QWEN_VLLM_HEALTH_TIMEOUT:-10s}"
HEALTH_START_PERIOD="${QWEN_VLLM_HEALTH_START_PERIOD:-300s}"
HEALTH_RETRIES="${QWEN_VLLM_HEALTH_RETRIES:-3}"
WATCH_INTERVAL="${QWEN_VLLM_WATCH_INTERVAL:-15}"

# 2.7 缓存路径
QWEN_CACHE_REPO_DIR_DEFAULT="${HF_CACHE_ROOT}/hub/models--Qwen--Qwen3.6-27B-FP8"
QWEN_CACHE_LOCK_DIR_DEFAULT="${HF_CACHE_ROOT}/hub/.locks/models--Qwen--Qwen3.6-27B-FP8"
QWEN_CACHE_REPO_DIR="${QWEN_VLLM_CACHE_REPO_DIR:-$QWEN_CACHE_REPO_DIR_DEFAULT}"
QWEN_CACHE_LOCK_DIR="${QWEN_VLLM_CACHE_LOCK_DIR:-$QWEN_CACHE_LOCK_DIR_DEFAULT}"

# 2.8 检测到的硬件（运行时填充）
HW_SOCKETS=0
HW_CORES_PER_SOCKET=0
HW_THREADS_PER_CORE=0
HW_TOTAL_CORES=0
HW_TOTAL_THREADS=0
HW_NUMA_NODES=0
HW_MEM_TOTAL_GB=0
HW_MEM_AVAIL_GB=0
HW_GPU_COUNT=0
HW_GPU_NAMES=()
HW_GPU_MEM_GB=()
declare -A HW_GPU_NUMA=()          # GPU idx -> NUMA node
declare -A HW_GPU_CPU_AFFINITY=()  # GPU idx -> cpuset string
declare -A HW_NUMA_CPUS=()         # NUMA node -> cpu list

# 2.9 计算后的运行时参数（运行时填充）
RUNTIME_CPUSET_CPUS=""
RUNTIME_CPUSET_MEMS=""
RUNTIME_CPU_THREADS_PER_WORKER="$CPU_THREADS_PER_WORKER"

# -----------------------------------------------------------------------------
# 3. 工具函数：命令存在性 / 取值
# -----------------------------------------------------------------------------
have()      { command -v "$1" >/dev/null 2>&1; }
trim()      { local s="${1:-}"; s="${s#"${s%%[![:space:]]*}"}"; s="${s%"${s##*[![:space:]]}"}"; printf '%s' "$s"; }
is_int()    { [[ "$1" =~ ^[0-9]+$ ]]; }
to_lower()  { printf '%s' "$1" | tr '[:upper:]' '[:lower:]'; }

# 解析 CSV → 数组（去除空白；去空）
# 使用 word-splitting（自定义 IFS）而不是 read -r（后者只会拿首字段）
csv_to_array() {
  local csv="$1" ifs="${2:-,}"
  local -n _out="$3"
  _out=()
  local old_ifs="$IFS"
  local item
  IFS="$ifs"
  set -f  # 关闭 glob，避免元素被当作 pattern
  for item in $csv; do
    item="$(trim "$item")"
    [[ -n "$item" ]] && _out+=("$item")
  done
  set +f
  IFS="$old_ifs"
}

# 安全数组求和
sum_array() {
  local -n _a="$1"
  local s=0
  for v in "${_a[@]}"; do s=$(( s + v )); done
  echo "$s"
}

die() { log_err "$*"; exit 1; }

# -----------------------------------------------------------------------------
# 4. 硬件检测模块
# -----------------------------------------------------------------------------

# 4.1 CPU 拓扑（lscpu 优先，/proc/cpuinfo 兜底）
detect_cpu_topology() {
  log_info "检测 CPU 拓扑 ..."
  if have lscpu; then
    HW_SOCKETS="$(lscpu | awk -F: '/^Socket\(s\)/{gsub(/ /,"",$2); print $2}')"
    HW_CORES_PER_SOCKET="$(lscpu | awk -F: '/^Core\(s\) per socket/{gsub(/ /,"",$2); print $2}')"
    HW_THREADS_PER_CORE="$(lscpu | awk -F: '/^Thread\(s\) per core/{gsub(/ /,"",$2); print $2}')"
    HW_NUMA_NODES="$(lscpu | awk -F: '/^NUMA node\(s\)/{gsub(/ /,"",$2); print $2}')"
    HW_VENDOR="$(lscpu | awk -F: '/^Vendor ID/{gsub(/^ +/,"",$2); print $2}')"
    HW_MODEL_NAME="$(lscpu | awk -F: '/^Model name/{gsub(/^ +/,"",$2); print $2; exit}')"
  fi

  # 兜底
  [[ -z "$HW_THREADS_PER_CORE" || "$HW_THREADS_PER_CORE" == "0" ]] && HW_THREADS_PER_CORE="$(awk '/^cpu cores/ && !seen {cores=$4; seen=1} /^siblings/ {sibs=$3} /^processor/ {n++} END{ if(n>0 && cores>0) print int(sibs/cores); else print 1}' /proc/cpuinfo)"
  [[ -z "$HW_TOTAL_THREADS" || "$HW_TOTAL_THREADS" == "0" ]] && HW_TOTAL_THREADS="$(nproc)"
  [[ -z "$HW_SOCKETS" || "$HW_SOCKETS" == "0" ]] && HW_SOCKETS="$(awk -F: '/^physical id/{print $2}' /proc/cpuinfo | sort -u | wc -l)"
  [[ -z "$HW_CORES_PER_SOCKET" || "$HW_CORES_PER_SOCKET" == "0" ]] && HW_CORES_PER_SOCKET="$(awk '/^cpu cores/{print $4; exit}' /proc/cpuinfo)"
  [[ -z "$HW_NUMA_NODES" || "$HW_NUMA_NODES" == "0" ]] && HW_NUMA_NODES="$HW_SOCKETS"

  HW_TOTAL_CORES=$(( HW_CORES_PER_SOCKET * HW_SOCKETS ))
  HW_TOTAL_THREADS="${HW_TOTAL_THREADS:-$(nproc)}"

  log_info "  CPU: ${HW_VENDOR:-?} ${HW_MODEL_NAME:-?} | sockets=${HW_SOCKETS} cores/socket=${HW_CORES_PER_SOCKET} threads/core=${HW_THREADS_PER_CORE}"
  log_info "  CPU 总计: ${HW_TOTAL_CORES} 物理核 / ${HW_TOTAL_THREADS} 逻辑线程 / NUMA nodes=${HW_NUMA_NODES}"
}

# 4.2 内存
detect_memory() {
  log_info "检测内存 ..."
  if [[ -r /proc/meminfo ]]; then
    HW_MEM_TOTAL_GB="$(awk '/^MemTotal:/ {printf "%d", $2/1024/1024}' /proc/meminfo)"
    HW_MEM_AVAIL_GB="$(awk '/^MemAvailable:/ {printf "%d", $2/1024/1024}' /proc/meminfo)"
  fi
  log_info "  内存: total=${HW_MEM_TOTAL_GB}GiB avail=${HW_MEM_AVAIL_GB}GiB"
}

# 4.3 GPU
detect_gpu_topology() {
  log_info "检测 GPU 拓扑 ..."
  if ! have nvidia-smi; then
    die "未检测到 nvidia-smi，请先安装 NVIDIA 驱动 + CUDA toolkit。"
  fi

  local raw
  raw="$(nvidia-smi --query-gpu=index,name,memory.total,utilization.gpu --format=csv,noheader,nounits 2>/dev/null || true)"
  if [[ -z "$raw" ]]; then
    die "nvidia-smi 查询 GPU 失败，请确认驱动已加载且 NVIDIA Container Toolkit 已安装。"
  fi

  HW_GPU_COUNT=0
  HW_GPU_NAMES=()
  HW_GPU_MEM_GB=()
  while IFS=',' read -r idx name mem util; do
    idx="$(trim "$idx")"; name="$(trim "$name")"; mem="$(trim "$mem")"
    [[ -z "$idx" ]] && continue
    HW_GPU_NAMES+=("$name")
    HW_GPU_MEM_GB+=("$(( mem / 1024 ))")
    HW_GPU_COUNT=$(( HW_GPU_COUNT + 1 ))
  done <<< "$raw"

  log_info "  检测到 ${HW_GPU_COUNT} 张 GPU:"
  local i=0
  while [[ $i -lt $HW_GPU_COUNT ]]; do
    log_info "    GPU${i}: ${HW_GPU_NAMES[$i]} ${HW_GPU_MEM_GB[$i]}GiB"
    i=$(( i + 1 ))
  done
}

# 4.4 NUMA 拓扑
detect_numa_topology() {
  log_info "检测 NUMA 拓扑 ..."
  HW_NUMA_CPUS=()

  if have numactl; then
    local i=0
    while [[ $i -lt $HW_NUMA_NODES ]]; do
      local cpu_list
      cpu_list="$(numactl --hardware 2>/dev/null | awk -v n="$i" '$0 ~ "^node " n " cpus:" {sub(/^node [0-9]+ cpus:[ \t]*/, ""); print; exit}')"
      if [[ -z "$cpu_list" ]]; then
        # 回退：用 lscpu 解析
        cpu_list="$(lscpu -p=cpu,node 2>/dev/null | awk -F',' -v n="$i" '!/^#/ && $2 == n {print $1}' | paste -sd, -)"
      fi
      HW_NUMA_CPUS[$i]="$cpu_list"
      log_info "  NUMA node ${i}: ${cpu_list:-<unknown>}"
      i=$(( i + 1 ))
    done
  else
    log_warn "未安装 numactl，将使用 lscpu 推导 NUMA CPU 列表"
    local i=0
    while [[ $i -lt $HW_NUMA_NODES ]]; do
      local cpu_list
      cpu_list="$(lscpu -p=cpu,node 2>/dev/null | awk -F',' -v n="$i" '!/^#/ && $2 == n {print $1}' | paste -sd, -)"
      HW_NUMA_CPUS[$i]="$cpu_list"
      log_info "  NUMA node ${i}: ${cpu_list:-<unknown>}"
      i=$(( i + 1 ))
    done
  fi

  # GPU → NUMA 节点映射（首选 nvidia-smi topo -m；回退到 PCI 总线 → NUMA）
  log_info "映射 GPU → NUMA 节点 ..."
  local i=0
  while [[ $i -lt $HW_GPU_COUNT ]]; do
    local node=""
    # 尝试方法 1：nvidia-smi topo -m（最后一列通常是 NUMA 关联）
    if have nvidia-smi; then
      local line
      # 跳过表头（列名行/续行）和 GPU 间空行；数据行首字段是 "GPU<n>" 且字段数 > 5
      line="$(nvidia-smi topo -m 2>/dev/null | awk -v gpu="GPU${i}" '$1 == gpu && NF > 5 {print; exit}' || true)"
      if [[ -n "$line" ]]; then
        # 取最后一个纯数字字段（NUMA 列），例 "GPU0 X NODE SYS SYS 0-15,64-79 0 N/A" → "0"
        node="$(printf '%s\n' "$line" | awk '{for (j=NF; j>=1; j--) if ($j ~ /^[0-9]+$/) {print $j; exit}}')"
      fi
    fi

    # 回退方法 2：通过 GPU 的 PCI 总线号 → sysfs → NUMA node
    if [[ -z "$node" || "$node" == "N/A" ]]; then
      local bus_addr
      bus_addr="$(nvidia-smi --query-gpu=pci.bus_id --format=csv,noheader,nounits 2>/dev/null | awk -F',' -v g="$i" 'NR==g+1 {gsub(/^ +/,"",$1); print $1}')"
      # 形如 0000:01:00.0 → 转成 0000:01:00.0
      local pci_path="/sys/bus/pci/devices/${bus_addr}"
      if [[ -r "${pci_path}/numa_node" ]]; then
        node="$(cat "${pci_path}/numa_node" 2>/dev/null || echo "")"
      fi
    fi

    # 兜底：按 GPU 索引在 NUMA 间均分
    if [[ -z "$node" || "$node" == "-1" ]]; then
      node=$(( i % HW_NUMA_NODES ))
      log_warn "  GPU${i} 未发现明确 NUMA 关联，按索引均分回退到 NUMA node ${node}"
    else
      log_info "  GPU${i} → NUMA node ${node}"
    fi

    HW_GPU_NUMA[$i]="$node"
    HW_GPU_CPU_AFFINITY[$i]="${HW_NUMA_CPUS[$node]:-}"
    i=$(( i + 1 ))
  done
}

detect_system_capabilities() {
  log_info "检测系统能力 ..."

  # 关键工具
  local missing=()
  for tool in docker nvidia-smi nproc awk numactl lscpu; do
    have "$tool" || missing+=("$tool")
  done
  if [[ ${#missing[@]} -gt 0 ]]; then
    log_warn "以下工具缺失（部分功能将降级）: ${missing[*]}"
  fi

  # 驱动版本 / CUDA 版本（仅打印）
  if have nvidia-smi; then
    local drv cuda
    # nvidia-smi 输出可能很大，awk 提前 exit 会触发 SIGPIPE；用 || true 屏蔽
    drv="$(nvidia-smi --query-gpu=driver_version --format=csv,noheader,nounits 2>/dev/null | head -n1 | tr -d ' ' || true)"
    # 直接 grep 抓取 "CUDA Version: <ver>" 这一行，sed 截取版本号
    cuda="$(nvidia-smi 2>/dev/null | grep -oE 'CUDA Version:[[:space:]]*[0-9.]+' | head -n1 | sed -E 's/.*[[:space:]]+//' || true)"
    log_info "  NVIDIA 驱动: ${drv:-?}   CUDA 运行时: ${cuda:-?}"
  fi

  # 内核与发行版
  if [[ -r /etc/os-release ]]; then
    # shellcheck disable=SC1091
    . /etc/os-release
    log_info "  操作系统: ${PRETTY_NAME:-?}  内核: $(uname -r)"
  fi
}

# -----------------------------------------------------------------------------
# 5. NUMA 调度与 CPU 绑定计算
# -----------------------------------------------------------------------------
#
# 目标：
#   - TP_SIZE=4，4 张 4090，每 worker 占 1 张 GPU → 4 个 TP worker
#   - 每个 worker 占用其 GPU 所在 NUMA 节点的 16 线程（逻辑核）
#   - 容器整体 cpuset 覆盖所有 worker 的 CPU 列表
#   - cpuset-mems 覆盖所有相关 NUMA 节点（交错内存以避免单节点热点）
#
# 假设（双路 6530 + 4×4090 主流板卡）：
#   - GPU0/1 → NUMA node 0
#   - GPU2/3 → NUMA node 1
#   检测逻辑会覆盖各种实际拓扑。
# -----------------------------------------------------------------------------

# 将 nvidia-smi 风格的 cpuset 列表转换为标准 CPU 范围："0-15,32-47" → "0-15,32-47"
# 实际已经是这个格式，保持原样并去除空格
normalize_cpuset() {
  local raw="$1"
  raw="$(printf '%s' "$raw" | tr -d ' ' | sed -E 's/,+/,/g; s/^,//; s/,$//')"
  printf '%s' "$raw"
}

# 合并多个 cpuset 字符串（去重）
merge_cpusets() {
  local -a parts=()
  local p
  for p in "$@"; do
    [[ -z "$p" ]] && continue
    IFS=',' read -r -a arr <<< "$p"
    parts+=("${arr[@]}")
  done
  # 输出去重后排序（按区间）
  printf '%s\n' "${parts[@]}" | sort -u | paste -sd, -
}

# 把以 "," 分隔的 CPU 编号列表转换为区间形式："0,1,2,3,5" → "0-3,5"
compress_cpuset() {
  local raw="$1"
  [[ -z "$raw" ]] && { printf ''; return; }
  local -a cpus=()
  IFS=',' read -r -a cpus <<< "$raw"
  local -a sorted=()
  while IFS= read -r n; do sorted+=("$n"); done < <(printf '%s\n' "${cpus[@]}" | sort -nu)

  local -a ranges=()
  local start="${sorted[0]}" prev="${sorted[0]}"
  local n
  for n in "${sorted[@]:1}"; do
    if (( n == prev + 1 )); then
      prev="$n"
    else
      if (( start == prev )); then ranges+=("$start"); else ranges+=("${start}-${prev}"); fi
      start="$n"; prev="$n"
    fi
  done
  if (( start == prev )); then ranges+=("$start"); else ranges+=("${start}-${prev}"); fi
  IFS=','; echo "${ranges[*]}"; unset IFS
}

compute_optimal_layout() {
  log_info "计算 NUMA 调度布局 ..."

  local -a gpu_list=()
  csv_to_array "$GPU_IDS" ',' gpu_list

  if [[ ${#gpu_list[@]} -eq 0 ]]; then
    die "QWEN_VLLM_GPU_IDS 为空，无法进行 NUMA 调度"
  fi

  # 每 worker CPU 线程数（用户硬性要求 16）
  if [[ -z "$CPU_THREADS_PER_WORKER" || "$CPU_THREADS_PER_WORKER" == "0" ]]; then
    CPU_THREADS_PER_WORKER=16
  fi
  RUNTIME_CPU_THREADS_PER_WORKER="$CPU_THREADS_PER_WORKER"

  # 收集每个 worker 对应的 NUMA 节点 & CPU 列表
  local -a per_worker_cpus=()
  local -a touched_numa=()

  for gpu_id in "${gpu_list[@]}"; do
    local numa="${HW_GPU_NUMA[$gpu_id]:-}"
    local cpus="${HW_GPU_CPU_AFFINITY[$gpu_id]:-}"

    if [[ -z "$numa" || -z "$cpus" ]]; then
      log_warn "GPU${gpu_id} 未识别到 NUMA 关联或 CPU 列表，将使用全机 CPU"
      cpus="0-$(( HW_TOTAL_THREADS - 1 ))"
      numa="0"
    fi

    # 在该 NUMA 上选取前 16 个 CPU（逻辑核）作为该 worker 的 CPU 集
    local worker_cpus=""
    if have numactl; then
      # numactl --hardware 已经包含每个 node 的 CPU 列表，直接取前 N 个
      worker_cpus="$(numactl --hardware 2>/dev/null \
        | awk -v n="${numa}" '$0 ~ "^node " n " cpus:" {sub(/^node [0-9]+ cpus:[ \t]*/, ""); print; exit}' \
        | tr ' ' '\n' | head -n "${CPU_THREADS_PER_WORKER}" | paste -sd, - || true)"
    fi
    if [[ -z "$worker_cpus" ]]; then
      # 从 NUMA CPU 列表取前 N 个
      local -a numa_arr=()
      IFS=',' read -r -a numa_arr <<< "$cpus"
      worker_cpus="$(printf '%s\n' "${numa_arr[@]}" | head -n "$CPU_THREADS_PER_WORKER" | paste -sd, -)"
    fi

    if [[ -z "$worker_cpus" ]]; then
      log_warn "GPU${gpu_id} 无法推导 worker CPU 集，回退到 0-$((CPU_THREADS_PER_WORKER-1))"
      worker_cpus="0-$(( CPU_THREADS_PER_WORKER - 1 ))"
    fi

    per_worker_cpus+=("$worker_cpus")
    # 记录触及的 NUMA
    case ",$(IFS=,; echo "${touched_numa[*]}")," in
      *,"${numa}",*) ;;
      *) touched_numa+=("$numa") ;;
    esac

    log_info "  GPU${gpu_id} (NUMA ${numa}) → ${worker_cpus}"
  done

  # 合并所有 worker 的 CPU → 容器 cpuset
  local merged
  merged="$(merge_cpusets "${per_worker_cpus[@]}")"
  RUNTIME_CPUSET_CPUS="$(compress_cpuset "$merged")"
  RUNTIME_CPUSET_MEMS="$(IFS=,; echo "${touched_numa[*]}" | sed 's/ //g')"

  log_ok "调度布局: cpuset-cpus=${RUNTIME_CPUSET_CPUS}  cpuset-mems=${RUNTIME_CPUSET_MEMS}"
  log_ok "每 worker 线程数: ${RUNTIME_CPU_THREADS_PER_WORKER}   worker 数: ${#gpu_list[@]}"

  # 打印预计物理核利用率（基于双路 6530 = 64 物理核 / 128 线程）
  local used_threads
  used_threads="$(printf '%s\n' "${per_worker_cpus[@]}" | tr ',' '\n' | wc -l)"
  log_info "预计占用逻辑线程数: ${used_threads} / ${HW_TOTAL_THREADS}"
  if (( used_threads > HW_TOTAL_THREADS )); then
    log_warn "worker CPU 总量超过整机逻辑线程，将发生 CPU 抢占！"
  fi
}

# -----------------------------------------------------------------------------
# 6. 系统级调优（sysctl / 透明大页 / CPU 调度器）
# -----------------------------------------------------------------------------
apply_system_tuning() {
  [[ "$APPLY_SYSCTLS" == "1" ]] || { log_info "跳过 sysctl 调优（QWEN_VLLM_APPLY_SYSCTLS=0）"; return 0; }
  log_info "应用内核级调优 ..."

  if ! have sysctl; then
    log_warn "无 sysctl 命令，跳过"
    return 0
  fi

  # 这些值对双路 Sapphire Rapids + 4×4090 + 128GB DDR5 已验证
  local -A sysctls=(
    ["vm.swappiness"]=0
    ["vm.overcommit_memory"]=1
    ["kernel.numa_balancing"]=0
    ["kernel.sched_autogroup_enabled"]=0
    ["kernel.sched_migration_cost_ns"]=5000000
    ["net.core.rmem_max"]=134217728
    ["net.core.wmem_max"]=134217728
    ["net.ipv4.tcp_rmem"]="4096 87380 134217728"
    ["net.ipv4.tcp_wmem"]="4096 65536 134217728"
    ["net.core.netdev_max_backlog"]=250000
    ["net.ipv4.tcp_congestion_control"]="bbr"
  )

  local key val current
  for key in "${!sysctls[@]}"; do
    val="${sysctls[$key]}"
    current="$(sysctl -n "$key" 2>/dev/null || echo "")"
    if [[ "$current" == "$val" ]]; then
      log_debug "  $key 已是 $val"
    else
      if sysctl -w "${key}=${val}" >/dev/null 2>&1; then
        log_info "  sysctl ${key}=${val} (was: ${current:-?})"
      else
        log_warn "  sysctl ${key}=${val} 设置失败（可能需要 root 或 net.ipv4.tcp_congestion_control=bbr 内核未编译）"
      fi
    fi
  done

  # 透明大页
  if [[ "$APPLY_TRANSPARENT_HUGEPAGE" == "1" && -w /sys/kernel/mm/transparent_hugepage/enabled ]]; then
    if echo madvise > /sys/kernel/mm/transparent_hugepage/enabled 2>/dev/null; then
      log_info "  透明大页: madvise"
    else
      log_warn "  透明大页设置失败"
    fi
  fi

  # CPU 调度器（仅在 root 且能写 cpufreq 时）
  if [[ "$APPLY_CPU_GOVERNOR" == "1" && -d /sys/devices/system/cpu/cpu0/cpufreq ]]; then
    local gov="performance"
    local cpu
    for cpu in /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor; do
      [[ -w "$cpu" ]] && echo "$gov" > "$cpu" 2>/dev/null || true
    done
    log_info "  CPU 调度器: performance (尝试设置)"
  fi

  log_ok "系统级调优完成"
}

# -----------------------------------------------------------------------------
# 7. Docker 客户端解析
# -----------------------------------------------------------------------------
resolve_docker_cmd() {
  : "${DOCKER_BIN:=docker}"
  docker_cmd=("$DOCKER_BIN")

  if ! command -v "$DOCKER_BIN" >/dev/null 2>&1; then
    die "找不到 docker 命令 (DOCKER_BIN=$DOCKER_BIN)。请安装 Docker 或设置 DOCKER_BIN。"
  fi

  local err
  err="$("$DOCKER_BIN" info 2>&1 >/dev/null || true)"
  if [[ -n "$err" ]]; then
    if grep -qi 'permission denied\|docker.sock' <<< "$err"; then
      if have sudo && sudo -n true >/dev/null 2>&1; then
        log_warn "当前用户无权访问 docker.sock，自动切换为 sudo docker"
        docker_cmd=(sudo "$DOCKER_BIN")
      else
        die "无 docker.sock 权限，请将当前用户加入 docker 组。"
      fi
    elif grep -qi 'Cannot connect to the Docker daemon\|Is the docker daemon running' <<< "$err"; then
      die "Docker daemon 未运行，请执行: sudo systemctl start docker"
    else
      die "Docker 不可用: $err"
    fi
  fi
}

container_exists() {
  [[ -n "$("${docker_cmd[@]}" ps -aq -f "name=^/${CONTAINER_NAME}$")" ]]
}

container_running() {
  [[ "$("${docker_cmd[@]}" inspect -f '{{.State.Running}}' "$CONTAINER_NAME" 2>/dev/null || true)" == "true" ]]
}

check_port_available() {
  log_info "校验端口 ${HOST_PORT} 可用性 ..."
  if have ss; then
    if ss -tlnp 2>/dev/null | grep -qE ":${HOST_PORT}[[:space:]]"; then
      die "宿主机端口 ${HOST_PORT} 已被占用，请更换 QWEN_VLLM_PORT"
    fi
  elif have netstat; then
    if netstat -tlnp 2>/dev/null | grep -qE ":${HOST_PORT}[[:space:]]"; then
      die "宿主机端口 ${HOST_PORT} 已被占用，请更换 QWEN_VLLM_PORT"
    fi
  else
    log_warn "未找到 ss/netstat，跳过端口检测"
  fi
}

# -----------------------------------------------------------------------------
# 8. 模型源解析
# -----------------------------------------------------------------------------
validate_model_source() {
  log_info "解析模型源: $MODEL_SOURCE"
  EFFECTIVE_MODEL_SOURCE="$MODEL_SOURCE"

  if [[ "$MODEL_SOURCE" == /* ]]; then
    [[ -d "$MODEL_SOURCE" ]] || die "模型目录不存在: $MODEL_SOURCE"
    local st
    st="$(find "$MODEL_SOURCE" -maxdepth 2 -type f -name '*.safetensors' | wc -l)"
    [[ "$st" -gt 0 ]] || die "模型目录缺少 *.safetensors: $MODEL_SOURCE"
    find "$MODEL_SOURCE" -maxdepth 2 -type f \( -name 'config.json' -o -name 'tokenizer.json' \) -print -quit | grep -q . \
      || die "模型目录缺少 config.json / tokenizer.json: $MODEL_SOURCE"
    log_ok "使用本地模型目录: $MODEL_SOURCE"
    return 0
  fi

  if [[ "$MODEL_SOURCE" == "Qwen/Qwen3.6-27B-FP8" && -d "$QWEN_CACHE_REPO_DIR/snapshots" ]]; then
    if ! find "$QWEN_CACHE_REPO_DIR/blobs" -type f -name '*.incomplete' 2>/dev/null | grep -q .; then
      local rev=""
      [[ -f "$QWEN_CACHE_REPO_DIR/refs/main" ]] && rev="$(tr -d '\n\r' < "$QWEN_CACHE_REPO_DIR/refs/main")"
      local host_snap=""
      if [[ -n "$rev" && -d "$QWEN_CACHE_REPO_DIR/snapshots/$rev" ]]; then
        host_snap="$QWEN_CACHE_REPO_DIR/snapshots/$rev"
      else
        host_snap="$(find "$QWEN_CACHE_REPO_DIR/snapshots" -mindepth 1 -maxdepth 1 -type d | head -n1 || true)"
      fi
      if [[ -n "$host_snap" ]] \
        && [[ -f "$host_snap/config.json" ]] \
        && [[ -f "$host_snap/model.safetensors.index.json" ]] \
        && [[ -f "$host_snap/tokenizer.json" ]]; then
        local cont_snap="${host_snap/$HF_CACHE_ROOT/$CONTAINER_HF_CACHE_ROOT}"
        EFFECTIVE_MODEL_SOURCE="$cont_snap"
        log_ok "复用本地快照: $host_snap"
        return 0
      fi
    fi
  fi

  log_info "将直接从 Hugging Face 拉取模型: $MODEL_SOURCE"
}

cleanup_partial_cache() {
  log_info "清理未完成缓存分片 ..."
  [[ "$MODEL_SOURCE" != "Qwen/Qwen3.6-27B-FP8" ]] && return 0
  [[ -d "$QWEN_CACHE_REPO_DIR/blobs" ]] && find "$QWEN_CACHE_REPO_DIR/blobs" -type f -name '*.incomplete' -delete
  [[ -d "$QWEN_CACHE_LOCK_DIR" ]] && rm -rf "$QWEN_CACHE_LOCK_DIR"
  log_ok "缓存清理完成"
}

# -----------------------------------------------------------------------------
# 9. 容器 run 命令构造
# -----------------------------------------------------------------------------
dry_run=0

run_qwen_container() {
  log_info "构造 docker run 命令 ..."
  local -a env_args runtime_args placement_args gpu_args health_args
  env_args=()
  runtime_args=()
  placement_args=()
  gpu_args=()
  health_args=()

  # ---- 9.1 环境变量（关键调优参数） ----
  env_args+=(
    -e "HF_HUB_DISABLE_XET=1"
    -e "PYTORCH_CUDA_ALLOC_CONF=expandable_segments:True"
    -e "OMP_NUM_THREADS=${RUNTIME_CPU_THREADS_PER_WORKER}"
    -e "MKL_NUM_THREADS=${RUNTIME_CPU_THREADS_PER_WORKER}"
    -e "OPENBLAS_NUM_THREADS=${RUNTIME_CPU_THREADS_PER_WORKER}"
    -e "TOKENIZERS_PARALLELISM=true"
    -e "MALLOC_ARENA_MAX=${MALLOC_ARENA_MAX}"
    -e "UV_THREADPOOL_SIZE=${UV_THREADPOOL_SIZE}"
    -e "CUDA_DEVICE_MAX_CONNECTIONS=${CUDA_DEVICE_MAX_CONNECTIONS}"
    -e "CUDA_MODULE_LOADING=${CUDA_MODULE_LOADING}"
    -e "CUDA_CACHE_DISABLE=0"
    -e "NCCL_P2P_LEVEL=${NCCL_P2P_LEVEL}"
    -e "NCCL_IB_DISABLE=${NCCL_IB_DISABLE}"
    -e "NCCL_SHM_DISABLE=0"
    -e "NCCL_SOCKET_IFNAME=${NCCL_SOCKET_IFNAME}"
    -e "NCCL_BUFFSIZE=${NCCL_BUFFSIZE}"
    -e "NCCL_IGNORE_CPU_AFFINITY=${NCCL_IGNORE_CPU_AFFINITY}"
    -e "NCCL_DEBUG=${NCCL_DEBUG}"
    -e "NCCL_ASYNC_ERROR_HANDLING=1"
    -e "VLLM_USE_V1=1"
  )
  [[ -n "$HF_ENDPOINT_VALUE" ]] && env_args+=( -e "HF_ENDPOINT=$HF_ENDPOINT_VALUE" )

  # ---- 9.2 调度（CPU/NUMA 绑定） ----
  [[ -n "$RUNTIME_CPUSET_CPUS" ]] && placement_args+=( --cpuset-cpus="$RUNTIME_CPUSET_CPUS" )
  [[ -n "$RUNTIME_CPUSET_MEMS" ]] && placement_args+=( --cpuset-mems="$RUNTIME_CPUSET_MEMS" )

  # ---- 9.3 GPU ----
  # Docker 29.x 对裸 CSV 解析异常，统一加引号
  gpu_args+=( --gpus "\"device=${GPU_IDS}\"" )

  # ---- 9.4 健康检查 ----
  health_args+=(
    --health-cmd="curl -sf http://localhost:${CONTAINER_PORT}/health || exit 1"
    --health-interval="${HEALTH_INTERVAL}"
    --health-timeout="${HEALTH_TIMEOUT}"
    --health-start-period="${HEALTH_START_PERIOD}"
    --health-retries="${HEALTH_RETRIES}"
  )

  # ---- 9.5 vLLM runtime 参数 ----
  runtime_args+=(
    --host 0.0.0.0
    --port "$CONTAINER_PORT"
    --model "$EFFECTIVE_MODEL_SOURCE"
    --served-model-name "$SERVED_MODEL_NAME"
    --tensor-parallel-size "$TP_SIZE"
    --pipeline-parallel-size "$PP_SIZE"
    --gpu-memory-utilization "$GPU_MEMORY_UTILIZATION"
    --kv-cache-dtype "$KV_CACHE_DTYPE"
    --enable-prefix-caching
    --enable-chunked-prefill
    --dtype auto
    --trust-remote-code
    --max-model-len "$MAX_MODEL_LEN"
    --max-num-seqs "$MAX_NUM_SEQS"
    --max-num-batched-tokens "$MAX_NUM_BATCHED_TOKENS"
  )
  [[ "$ENFORCE_EAGER" == "1" ]] && runtime_args+=( --enforce-eager )
  [[ "$ENABLE_AUTO_TOOL_CHOICE" == "1" ]] && runtime_args+=( --enable-auto-tool-choice --tool-call-parser "$TOOL_CALL_PARSER" )
  [[ -n "$REASONING_PARSER" ]] && runtime_args+=( --reasoning-parser "$REASONING_PARSER" )
  [[ -n "$DEFAULT_CHAT_TEMPLATE_KWARGS" ]] && runtime_args+=( --default-chat-template-kwargs "$DEFAULT_CHAT_TEMPLATE_KWARGS" )
  [[ "$ENABLE_MTP" == "1" ]] && runtime_args+=( --speculative-config '{"method":"qwen3_next_mtp","num_speculative_tokens":4}' )
  runtime_args+=( --api-key "$API_KEY" )

  # ---- 9.6 组装并执行 ----
  log_debug "docker run 参数: ${env_args[*]} | ${placement_args[*]} | ${gpu_args[*]}"

  if [[ "$dry_run" == "1" ]]; then
    echo "[DRY-RUN] 将执行以下 docker run 命令:" >&2
    printf '  %q ' "${docker_cmd[@]}" run -d >&2
    printf -- '--name %q ' "$CONTAINER_NAME" >&2
    printf -- '--restart %q ' "unless-stopped" >&2
    printf -- '--ipc %q ' "host" >&2
    printf -- '--shm-size=%q ' "$SHM_SIZE" >&2
    printf -- '--ulimit %q ' "memlock=-1" >&2
    printf -- '--ulimit %q ' "stack=67108864" >&2
    printf -- '--cpu-shares=%q ' "$DOCKER_CPU_SHARES" >&2
    printf -- '--memory-swappiness=%q ' "$DOCKER_MEM_SWAPPINESS" >&2
    for a in "${placement_args[@]}"; do printf '%q ' "$a" >&2; done
    for a in "${gpu_args[@]}";     do printf '%q ' "$a" >&2; done
    for a in "${health_args[@]}";  do printf '%q ' "$a" >&2; done
    printf -- '--log-opt %q ' "max-size=100m" >&2
    printf -- '--log-opt %q ' "max-file=5" >&2
    printf -- '-p %q ' "${HOST_PORT}:${CONTAINER_PORT}" >&2
    for a in "${env_args[@]}"; do printf '%q ' "$a" >&2; done
    printf -- '-v %q ' "${HF_CACHE_ROOT}:/root/.cache/huggingface" >&2
    printf -- '-v %q ' "${MODEL_ROOT}:/models" >&2
    printf '%q ' "$IMAGE" >&2
    for a in "${runtime_args[@]}"; do printf '%q ' "$a" >&2; done
    printf '\n' >&2
    return 0
  fi

  "${docker_cmd[@]}" run -d \
    --name "$CONTAINER_NAME" \
    --restart unless-stopped \
    --ipc host \
    --shm-size="$SHM_SIZE" \
    --ulimit memlock=-1 \
    --ulimit stack=67108864 \
    --cpu-shares="$DOCKER_CPU_SHARES" \
    --memory-swappiness="$DOCKER_MEM_SWAPPINESS" \
    "${placement_args[@]}" \
    "${gpu_args[@]}" \
    "${health_args[@]}" \
    --log-opt max-size=100m \
    --log-opt max-file=5 \
    -p "${HOST_PORT}:${CONTAINER_PORT}" \
    "${env_args[@]}" \
    -v "${HF_CACHE_ROOT}:/root/.cache/huggingface" \
    -v "${MODEL_ROOT}:/models" \
    "$IMAGE" \
    "${runtime_args[@]}"
}

# -----------------------------------------------------------------------------
# 10. 生命周期命令
# -----------------------------------------------------------------------------
preflight() {
  log_info "===== 预检: ${SCRIPT_NAME} v${SCRIPT_VERSION} ====="
  detect_cpu_topology
  detect_memory
  detect_gpu_topology
  detect_numa_topology
  detect_system_capabilities
  compute_optimal_layout
  apply_system_tuning
  log_info "===== 预检完成 ====="
}

start_service() {
  resolve_docker_cmd
  # 解析 --dry-run / --image <img> 等参数
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --dry-run)  dry_run=1; shift ;;
      --image)    IMAGE="$2"; shift 2 ;;
      *)          shift ;;
    esac
  done

  preflight
  validate_model_source

  log_info "启动容器: $CONTAINER_NAME"

  # dry-run：仅打印 docker run 命令，不做端口/容器状态检查
  if [[ "$dry_run" == "1" ]]; then
    run_qwen_container || true
    log_ok "[DRY-RUN] 完成，未真正启动容器"
    return 0
  fi

  check_port_available

  if container_running; then
    log_ok "$CONTAINER_NAME 已在运行"
    return 0
  fi

  if container_exists; then
    log_info "发现旧容器，删除后重建 ..."
    "${docker_cmd[@]}" rm -f "$CONTAINER_NAME" >/dev/null
  fi

  if run_qwen_container; then
    log_ok "已启动 $CONTAINER_NAME (host:$HOST_PORT -> container:$CONTAINER_PORT)"
    log_info "模型加载中，健康检查将在约 ${HEALTH_START_PERIOD} 后首次通过"
    post_start_check
  else
    die "容器启动失败，请查看日志: ${docker_cmd[*]} logs ${CONTAINER_NAME}"
  fi
}

post_start_check() {
  log_info "等待容器进入 running 状态 ..."
  local waited=0 max_wait=60
  while (( waited < max_wait )); do
    if container_running; then
      log_ok "容器已 running（等待 ${waited}s）"
      break
    fi
    sleep 2; waited=$(( waited + 2 ))
  done

  if ! container_running; then
    log_warn "容器在 ${max_wait}s 内未进入 running，请检查 logs"
    return 1
  fi

  # 可选：首次 vLLM /v1/models 探活
  log_info "首次探活: http://localhost:${HOST_PORT}/v1/models"
  local curl_wait=0
  while (( curl_wait < HEALTH_START_PERIOD )); do
    if curl -sf -H "Authorization: Bearer ${API_KEY}" "http://localhost:${HOST_PORT}/v1/models" >/dev/null 2>&1; then
      log_ok "vLLM OpenAI 兼容接口就绪"
      break
    fi
    sleep 10; curl_wait=$(( curl_wait + 10 ))
    (( curl_wait % 60 == 0 )) && log_info "  仍在等待模型加载（${curl_wait}s / ${HEALTH_START_PERIOD}s）..."
  done

  if ! curl -sf -H "Authorization: Bearer ${API_KEY}" "http://localhost:${HOST_PORT}/v1/models" >/dev/null 2>&1; then
    log_warn "vLLM 在 ${HEALTH_START_PERIOD}s 内未就绪，请通过 logs 子命令查看进度"
  fi
}

stop_service() {
  resolve_docker_cmd
  if ! container_exists; then
    log_ok "$CONTAINER_NAME 不存在，无需停止"
    return 0
  fi
  log_info "优雅停止 $CONTAINER_NAME (timeout=${STOP_TIMEOUT}s) ..."
  "${docker_cmd[@]}" stop -t "$STOP_TIMEOUT" "$CONTAINER_NAME" >/dev/null 2>&1 || true
  "${docker_cmd[@]}" rm -f "$CONTAINER_NAME" >/dev/null
  log_ok "已移除 $CONTAINER_NAME"
}

reset_service() {
  resolve_docker_cmd
  preflight
  validate_model_source

  if container_exists; then
    "${docker_cmd[@]}" stop -t "$STOP_TIMEOUT" "$CONTAINER_NAME" >/dev/null 2>&1 || true
    "${docker_cmd[@]}" rm -f "$CONTAINER_NAME" >/dev/null || true
  fi

  if "${docker_cmd[@]}" image inspect "$IMAGE" >/dev/null 2>&1; then
    log_info "删除旧镜像: $IMAGE"
    "${docker_cmd[@]}" image rm -f "$IMAGE" >/dev/null 2>&1 || true
  fi

  cleanup_partial_cache

  log_info "重新拉取镜像: $IMAGE"
  "${docker_cmd[@]}" pull "$IMAGE"

  check_port_available
  run_qwen_container
  log_ok "已重新拉起 $CONTAINER_NAME，模型若未完整缓存将后台继续下载"
  post_start_check
}

status_service() {
  resolve_docker_cmd
  # 填充分配/拓扑变量
  detect_cpu_topology
  detect_memory
  detect_gpu_topology
  detect_numa_topology
  compute_optimal_layout

  "${docker_cmd[@]}" ps -a --filter "name=^/${CONTAINER_NAME}$" \
    --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}' || true

  echo
  echo "=== NUMA 拓扑概览 ==="
  local i=0
  while [[ $i -lt $HW_GPU_COUNT ]]; do
    echo "GPU${i} → NUMA${HW_GPU_NUMA[$i]:-?}  CPUS=${HW_GPU_CPU_AFFINITY[$i]:-?}"
    i=$(( i + 1 ))
  done
  echo
  echo "调度布局: cpuset-cpus=${RUNTIME_CPUSET_CPUS}  cpuset-mems=${RUNTIME_CPUSET_MEMS}"
  echo "每 worker 线程数: ${RUNTIME_CPU_THREADS_PER_WORKER}"
}

logs_service() {
  resolve_docker_cmd
  "${docker_cmd[@]}" logs --tail "${QWEN_VLLM_LOG_TAIL:-200}" -f "$CONTAINER_NAME"
}

health_check_once() {
  resolve_docker_cmd
  local rc=0

  if ! container_running; then
    log_err "[HEALTH] 容器未运行"
    return 1
  fi

  # Docker health status
  local dhealth
  dhealth="$("${docker_cmd[@]}" inspect -f '{{.State.Health.Status}}' "$CONTAINER_NAME" 2>/dev/null || echo none)"
  log_info "[HEALTH] docker health=${dhealth}"

  # vLLM /health
  if curl -sf "http://localhost:${HOST_PORT}/health" >/dev/null 2>&1; then
    log_ok "[HEALTH] /health 200 OK"
  else
    log_err "[HEALTH] /health 失败"
    rc=1
  fi

  # vLLM /v1/models
  if curl -sf -H "Authorization: Bearer ${API_KEY}" "http://localhost:${HOST_PORT}/v1/models" >/dev/null 2>&1; then
    log_ok "[HEALTH] /v1/models 200 OK"
  else
    log_err "[HEALTH] /v1/models 失败"
    rc=1
  fi

  # GPU 利用率快照
  if have nvidia-smi; then
    local snap
    snap="$(nvidia-smi --query-gpu=index,utilization.gpu,memory.used,memory.total --format=csv,noheader,nounits 2>/dev/null || true)"
    log_info "[HEALTH] GPU 快照:"
    while IFS=',' read -r idx u m mt; do
      log_info "  GPU$(trim "$idx") util=$(trim "$u")%  mem=$(trim "$m")/$(trim "$mt") MiB"
    done <<< "$snap"
  fi

  return $rc
}

watch_service() {
  resolve_docker_cmd
  log_info "进入 watch 模式（间隔 ${WATCH_INTERVAL}s，Ctrl+C 退出）"
  while true; do
    if ! health_check_once; then
      log_warn "健康检查异常，等待下次重试 ..."
    fi
    sleep "$WATCH_INTERVAL"
  done
}

# -----------------------------------------------------------------------------
# 11. 性能基准测试（轻量级）
# -----------------------------------------------------------------------------
# 用法: ./script bench [--prompt <text>] [--max-tokens N] [--concurrent N] [--iters N]
run_benchmark() {
  resolve_docker_cmd
  if ! container_running; then
    die "容器未运行，请先 start"
  fi

  local prompt="请用 200 字简要介绍 Sapphire Rapids 处理器在 LLM 推理场景的 NUMA 调度策略。"
  local max_tokens=256 concurrent=4 iters=8

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --prompt)     prompt="$2"; shift 2 ;;
      --max-tokens) max_tokens="$2"; shift 2 ;;
      --concurrent) concurrent="$2"; shift 2 ;;
      --iters)      iters="$2"; shift 2 ;;
      *) shift ;;
    esac
  done

  log_info "===== 基准测试 ====="
  log_info "  并发: ${concurrent}   每轮: ${iters}   max_tokens: ${max_tokens}"
  log_info "  提示词长度: ${#prompt} chars"

  local endpoint="http://localhost:${HOST_PORT}/v1/chat/completions"
  local payload
  payload="$(cat <<EOF
{
  "model": "${SERVED_MODEL_NAME}",
  "messages": [{"role":"user","content":"${prompt}"}],
  "max_tokens": ${max_tokens},
  "temperature": 0.0,
  "stream": false
}
EOF
)"

  local pids=() outs=()
  local i
  for ((i=0; i<concurrent; i++)); do
    local out_file
    out_file="$(mktemp -t qwen-bench-XXXXXX.json)"
    outs+=("$out_file")
    (
      local s e
      s="$(date +%s%N)"
      curl -sS -H "Content-Type: application/json" \
        -H "Authorization: Bearer ${API_KEY}" \
        -d "$payload" "$endpoint" > "$out_file" 2>/dev/null
      e="$(date +%s%N)"
      echo "$(( (e - s) / 1000000 ))" > "${out_file}.lat"
    ) &
    pids+=("$!")
  done
  for p in "${pids[@]}"; do wait "$p"; done

  local total_ms=0 counted=0
  for out in "${outs[@]}"; do
    [[ -s "$out" ]] || continue
    local lat
    lat="$(cat "${out}.lat" 2>/dev/null || echo 0)"
    total_ms=$(( total_ms + lat ))
    counted=$(( counted + 1 ))
    local completion
    completion="$(grep -o '"content":"[^"]*"' "$out" 2>/dev/null | head -n1 | cut -c12-200)"
    log_info "  请求完成 ${counted}/${concurrent}: ${lat}ms  ->  ${completion:0:80}..."
  done

  local avg
  if (( counted > 0 )); then
    avg=$(( total_ms / counted ))
  else
    avg=0
  fi

  log_ok "===== 基准完成 ====="
  log_info "  成功请求: ${counted}/${concurrent}"
  log_info "  总耗时:   ${total_ms} ms"
  log_info "  平均延迟: ${avg} ms / request"

  # 清理
  for out in "${outs[@]}"; do
    rm -f "$out" "${out}.lat"
  done
}

# -----------------------------------------------------------------------------
# 12. 使用说明 & CLI 分发
# -----------------------------------------------------------------------------
usage() {
  cat <<USAGE_EOF
用法: ./${SCRIPT_NAME} <command> [options]

命令:
  start              预检 + 启动 qwen-vllm（默认动作）
  stop               优雅停止并移除 qwen-vllm 容器
  reset              删除旧容器/镜像，清理缓存后重新拉起
  restart            stop + start
  status             查看容器状态 + NUMA 拓扑概览
  logs               跟踪容器日志（Ctrl+C 退出）
  health             单次健康检查（容器 + /health + /v1/models + GPU 快照）
  watch              周期健康检查（默认 15s 间隔）
  bench [opts]       简单性能基准测试
  preflight          仅运行预检，不启动容器
  detect             仅打印硬件检测结果
  start --dry-run    预览 docker run 命令而不实际执行
  version            打印脚本版本与退出
  help | -h | --help 打印本帮助

bench 选项:
  --prompt "..."    自定义提示词
  --max-tokens N    每请求最大输出 token（默认 256）
  --concurrent N    并发请求数（默认 4）
  --iters N         每并发轮数（默认 8）

环境变量（节选，详见 ${SCRIPT_NAME} 顶部注释）:
  QWEN_VLLM_MODEL                 模型源（默认: Qwen/Qwen3.6-27B-FP8）
  QWEN_VLLM_GPU_IDS               GPU 列表（默认: 0,1,2,3）
  QWEN_VLLM_TP_SIZE               TP 数（默认: 4）
  QWEN_VLLM_CPU_THREADS_PER_WORKER 每 worker 线程（固定 16）
  QWEN_VLLM_CPU_SHARES            docker cpu-shares（默认 4096）
  QWEN_VLLM_MEM_SWAPPINESS        docker memory-swappiness（默认 0）
  QWEN_VLLM_MALLOC_ARENA_MAX      glibc malloc arena（默认 4）
  QWEN_VLLM_UV_THREADPOOL_SIZE    libuv 线程池（默认 32）
  QWEN_VLLM_CUDA_DEVICE_MAX_CONNECTIONS  CUDA 并发（默认 32）
  QWEN_VLLM_CUDA_MODULE_LOADING   CUDA 模块加载（默认 LAZY）
  QWEN_VLLM_NCCL_IGNORE_CPU_AFFINITY  NCCL 亲和忽略（默认 1）
  QWEN_VLLM_NCCL_BUFFSIZE         NCCL 缓冲（默认 33554432）
  QWEN_VLLM_APPLY_SYSCTLS         是否应用 sysctl 调优（默认 1）
  QWEN_VLLM_LOG_DIR               日志目录（默认 /var/log/qwen-vllm）
  QWEN_VLLM_DEBUG                 调试日志（1 开启）

示例:
  ./${SCRIPT_NAME} preflight
  ./${SCRIPT_NAME} start
  ./${SCRIPT_NAME} health
  ./${SCRIPT_NAME} watch
  ./${SCRIPT_NAME} bench --concurrent 8 --max-tokens 512
USAGE_EOF
}

version() {
  echo "${SCRIPT_NAME} ${SCRIPT_VERSION}  (hostname=${HOSTNAME_SHORT}, date=${TODAY_TAG})"
}

# 把第一个参数取出作为命令，其余参数（rest_args）传给子命令
cmd="${1:-status}"
shift || true

case "$cmd" in
  start)      start_service "$@" ;;
  stop)       stop_service ;;
  restart)    stop_service; start_service "$@" ;;
  reset)      reset_service ;;
  status)     status_service ;;
  logs)       logs_service ;;
  health)     health_check_once ;;
  watch)      watch_service ;;
  bench)      run_benchmark "$@" ;;
  preflight)  resolve_docker_cmd; preflight ;;
  detect)     resolve_docker_cmd; detect_cpu_topology; detect_memory; detect_gpu_topology; detect_numa_topology ;;
  version|-V|--version) version ;;
  help|-h|--help) usage ;;
  *)          log_err "未知命令: $cmd"; usage; exit 1 ;;
esac