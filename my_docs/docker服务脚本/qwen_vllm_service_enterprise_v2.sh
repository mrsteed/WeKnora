#!/usr/bin/env bash
# =============================================================================
#  qwen_vllm_service_enterprise_v2.sh
#  -----------------------------------------------------------------------------
#  企业级多实例生产脚本：在双路 Xeon Gold 6530 (64C/128T) + 4×RTX 4090 + 128GB
#  DDR5 + Ubuntu 26.04 + vLLM 0.23.x 平台上，部署两份 Qwen3.6-27B-FP8 推理服务：
#
#    实例 A : GPU0 + GPU1 (TP=2)，端口 8000 → 容器内 8000
#    实例 B : GPU2 + GPU3 (TP=2)，端口 8001 → 容器内 8000
#
#  v2 相对 v1 的关键改造：
#    - 单实例 → 多实例：所有 per-instance 配置改为数组
#    - 加入 `--language-model-only`（省 vision encoder 显存）
#    - 调小 `--max-cudagraph-capture-size=256`（绕开混合架构 CUDA graph 坑）
#    - NUMA 调度按实例分别计算（实例 A 锁 NUMA0，实例 B 锁 NUMA2/3）
#    - 每个实例独立 `--served-model-name`（后缀 -a / -b），方便客户端分流
#
#  关于"内存共享"：
#    - GPU P2P 显存共享：本机拓扑显示所有 GPU 对为 CNS（芯片组不支持），不可用
#    - CPU offload：可通过 --cpu-offload-gb 实现，但会与 PCIe 上的 TP 通信抢带宽
#    - 本脚本默认**不**启用 CPU offload；先跑满 2×4090 + 262K 上下文，按需再加
#
#  用法：见末尾 USAGE 与 README。
#
#  适用硬件：双路 Xeon Gold 6530 / 4×RTX 4090 / 128GB DDR5 ECC / Ubuntu 26.04
#  依赖：vLLM 0.23.x、Docker + nvidia-container-toolkit、numactl、lscpu、nvidia-smi
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
SCRIPT_VERSION="2.0.0-enterprise-multi"
HOSTNAME_SHORT="$(hostname -s 2>/dev/null || echo localhost)"
TODAY_TAG="$(date +%Y%m%d)"

LOG_DIR="${QWEN_VLLM_LOG_DIR:-/var/log/qwen-vllm}"
mkdir -p "${LOG_DIR}" 2>/dev/null || LOG_DIR="${SCRIPT_DIR}/.logs"
mkdir -p "${LOG_DIR}" 2>/dev/null || true
LOG_FILE="${LOG_DIR}/${HOSTNAME_SHORT}-${TODAY_TAG}.log"

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

# 带实例标签的日志（用于多实例场景）
log_inst() {
  local idx="$1"; shift
  local tag
  tag="[INST${idx}]"
  echo "${C_CYN}${tag}${C_RST} $*" >&2
  _log_to_file "${tag}" "$*"
}

# -----------------------------------------------------------------------------
# 2. 默认配置（共享 + 每实例）
# -----------------------------------------------------------------------------

# 2.0 实例数量（可通过 QWEN_VLLM_INSTANCES 覆盖；默认 2）
INSTANCES_DEFAULT=2

# 2.1 共享配置（所有实例共用）
CONTAINER_PORT="${QWEN_VLLM_CONTAINER_PORT:-8000}"   # 容器内 vLLM 服务端口（固定）
IMAGE="${QWEN_VLLM_IMAGE:-vllm/vllm-openai:latest}"
API_KEY="${QWEN_VLLM_API_KEY:-sk-hlsa-local-vllm}"
STOP_TIMEOUT="${QWEN_VLLM_STOP_TIMEOUT:-45}"
SHM_SIZE="${QWEN_VLLM_SHM_SIZE:-32g}"
BASE_PORT="${QWEN_VLLM_BASE_PORT:-8000}"             # 实例 0 监听此端口，后续递增

MODEL_SOURCE="${QWEN_VLLM_MODEL:-Qwen/Qwen3.6-27B-FP8}"
SERVED_MODEL_NAME_BASE="${QWEN_VLLM_SERVED_MODEL_NAME:-qwen3.6-27b-fp8}"
HF_CACHE_ROOT="${QWEN_VLLM_HF_CACHE_ROOT:-/data/models/hf-cache}"
MODEL_ROOT="${QWEN_VLLM_MODEL_ROOT:-/data/models/llm}"
HF_ENDPOINT_VALUE="${HF_ENDPOINT:-}"
CONTAINER_HF_CACHE_ROOT="${QWEN_VLLM_CONTAINER_HF_CACHE_ROOT:-/root/.cache/huggingface}"
EFFECTIVE_MODEL_SOURCE="$MODEL_SOURCE"

# 2.2 推理参数（共享；不同实例可单独覆盖 TP/GPU 等）
GPU_MEMORY_UTILIZATION="${QWEN_VLLM_GPU_MEMORY_UTILIZATION:-0.85}"
#MAX_MODEL_LEN="${QWEN_VLLM_MAX_MODEL_LEN:-262144}"
#MAX_MODEL_LEN="${QWEN_VLLM_MAX_MODEL_LEN:-131072}"
MAX_MODEL_LEN="${QWEN_VLLM_MAX_MODEL_LEN:-102400}"
#MAX_MODEL_LEN="${QWEN_VLLM_MAX_MODEL_LEN:-98304}"
#MAX_MODEL_LEN="${QWEN_VLLM_MAX_MODEL_LEN:-65536}"

MAX_NUM_SEQS="${QWEN_VLLM_MAX_NUM_SEQS:-128}"
MAX_NUM_BATCHED_TOKENS="${QWEN_VLLM_MAX_NUM_BATCHED_TOKENS:-8192}"
ENFORCE_EAGER="${QWEN_VLLM_ENFORCE_EAGER:-0}"
ENABLE_AUTO_TOOL_CHOICE="${QWEN_VLLM_ENABLE_AUTO_TOOL_CHOICE:-1}"
TOOL_CALL_PARSER="${QWEN_VLLM_TOOL_CALL_PARSER:-qwen3_coder}"
REASONING_PARSER="${QWEN_VLLM_REASONING_PARSER:-qwen3}"
DEFAULT_CHAT_TEMPLATE_KWARGS="${QWEN_VLLM_DEFAULT_CHAT_TEMPLATE_KWARGS:-{\"enable_thinking\": false}}"
ENABLE_MTP="${QWEN_VLLM_ENABLE_MTP:-1}"
KV_CACHE_DTYPE="${QWEN_VLLM_KV_CACHE_DTYPE:-fp8}"

# 2.3 v2 新增：长上下文/混合架构友好开关
LANGUAGE_MODEL_ONLY="${QWEN_VLLM_LANGUAGE_MODEL_ONLY:-1}"   # 默认开启，省 vision encoder 显存
MAX_CUDAGRAPH_CAPTURE_SIZE="${QWEN_VLLM_MAX_CUDAGRAPH_CAPTURE_SIZE:-256}"  # 避开 Mamba cache 报错
CHUNKED_PREFILL_ENABLED="${QWEN_VLLM_CHUNKED_PREFILL:-1}"
PREFIX_CACHING_ENABLED="${QWEN_VLLM_PREFIX_CACHING:-1}"

# 2.4 调度参数（共享；按实例分别生成 cpuset）
CPU_THREADS_PER_WORKER="${QWEN_VLLM_CPU_THREADS_PER_WORKER:-16}"  # 固定 16
DOCKER_CPU_SHARES="${QWEN_VLLM_CPU_SHARES:-4096}"
DOCKER_MEM_SWAPPINESS="${QWEN_VLLM_MEM_SWAPPINESS:-0}"
# Docker --restart 策略：默认 on-failure:3（最多自愈 3 次），避免崩溃后无限重启循环
#   可选：no / on-failure[:N] / always / unless-stopped
DOCKER_RESTART_POLICY="${QWEN_VLLM_RESTART_POLICY:-on-failure:3}"
# start 命令是否阻塞等 vLLM /v1/models 就绪（默认 0：立即返回）
#   设为 1 时与 v1 行为一致，会阻塞最长 HEALTH_START_PERIOD
WAIT_HEALTHY_ON_START="${QWEN_VLLM_WAIT_HEALTHY:-0}"
MALLOC_ARENA_MAX="${QWEN_VLLM_MALLOC_ARENA_MAX:-2}"
UV_THREADPOOL_SIZE="${QWEN_VLLM_UV_THREADPOOL_SIZE:-32}"
CUDA_DEVICE_MAX_CONNECTIONS="${QWEN_VLLM_CUDA_DEVICE_MAX_CONNECTIONS:-32}"
CUDA_MODULE_LOADING="${QWEN_VLLM_CUDA_MODULE_LOADING:-LAZY}"
NCCL_IGNORE_CPU_AFFINITY="${QWEN_VLLM_NCCL_IGNORE_CPU_AFFINITY:-1}"
NCCL_BUFFSIZE="${QWEN_VLLM_NCCL_BUFFSIZE:-8388608}"     # 8 MiB，推理场景足够
NCCL_P2P_LEVEL="${QWEN_VLLM_NCCL_P2P_LEVEL:-SYS}"
NCCL_IB_DISABLE="${QWEN_VLLM_NCCL_IB_DISABLE:-1}"
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

# 2.8 硬件检测结果（运行时填充）
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
declare -A HW_GPU_NUMA=()
declare -A HW_GPU_CPU_AFFINITY=()
declare -A HW_NUMA_CPUS=()
DOCKER_GPU_BACKEND=""
DOCKER_NVIDIA_CDI_SPEC=""

# 2.9 实例数组（运行时填充；下标 = 实例编号 0..N-1）
INST_COUNT=0
INST_NAME=()
INST_GPUS=()           # CSV，如 "0,1"
INST_GPU_COUNT=()      # 该实例 GPU 数（用于校验 TP）
INST_TP=()             # 张量并行度
INST_PORT=()           # 宿主机端口
INST_MODEL_NAME=()     # --served-model-name
INST_CPUSET_CPUS=()    # 容器 cpuset-cpus
INST_CPUSET_MEMS=()    # 容器 cpuset-mems
INST_WORKER_CPUS=()    # 每 worker 的 CPU 列表（调试用）

# -----------------------------------------------------------------------------
# 3. 工具函数
# -----------------------------------------------------------------------------
have()      { command -v "$1" >/dev/null 2>&1; }
trim()      { local s="${1:-}"; s="${s#"${s%%[![:space:]]*}"}"; s="${s%"${s##*[![:space:]]}"}"; printf '%s' "$s"; }
is_int()    { [[ "$1" =~ ^[0-9]+$ ]]; }
die()       { log_err "$*"; exit 1; }

csv_to_array() {
  local csv="$1" ifs="${2:-,}"
  local -n _out="$3"
  _out=()
  local old_ifs="$IFS" item
  IFS="$ifs"
  set -f
  for item in $csv; do
    item="$(trim "$item")"
    [[ -n "$item" ]] && _out+=("$item")
  done
  set +f
  IFS="$old_ifs"
}

sum_array() {
  local -n _a="$1" s=0 v
  for v in "${_a[@]}"; do s=$(( s + v )); done
  echo "$s"
}

# 生成区间形式的 cpuset
compress_cpuset() {
  local raw="$1"
  [[ -z "$raw" ]] && { printf ''; return; }
  local -a cpus=()
  IFS=',' read -r -a cpus <<< "$raw"
  local -a sorted=()
  while IFS= read -r n; do sorted+=("$n"); done < <(printf '%s\n' "${cpus[@]}" | sort -nu)
  local -a ranges=()
  local start="${sorted[0]}" prev="${sorted[0]}" n
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

merge_cpusets() {
  local -a parts=() p arr
  for p in "$@"; do
    [[ -z "$p" ]] && continue
    IFS=',' read -r -a arr <<< "$p"
    parts+=("${arr[@]}")
  done
  printf '%s\n' "${parts[@]}" | sort -u | paste -sd, -
}

# -----------------------------------------------------------------------------
# 4. 硬件检测（同 v1）
# -----------------------------------------------------------------------------
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

detect_memory() {
  log_info "检测内存 ..."
  if [[ -r /proc/meminfo ]]; then
    HW_MEM_TOTAL_GB="$(awk '/^MemTotal:/ {printf "%d", $2/1024/1024}' /proc/meminfo)"
    HW_MEM_AVAIL_GB="$(awk '/^MemAvailable:/ {printf "%d", $2/1024/1024}' /proc/meminfo)"
  fi
  log_info "  内存: total=${HW_MEM_TOTAL_GB}GiB avail=${HW_MEM_AVAIL_GB}GiB"
}

detect_gpu_topology() {
  log_info "检测 GPU 拓扑 ..."
  if ! have nvidia-smi; then
    die "未检测到 nvidia-smi，请先安装 NVIDIA 驱动 + CUDA toolkit。"
  fi
  local raw
  raw="$(nvidia-smi --query-gpu=index,name,memory.total,utilization.gpu --format=csv,noheader,nounits 2>/dev/null || true)"
  [[ -z "$raw" ]] && die "nvidia-smi 查询 GPU 失败。"
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

detect_numa_topology() {
  log_info "检测 NUMA 拓扑 ..."
  HW_NUMA_CPUS=()
  if have numactl; then
    local i=0 cpu_list
    while [[ $i -lt $HW_NUMA_NODES ]]; do
      cpu_list="$(numactl --hardware 2>/dev/null | awk -v n="$i" '$0 ~ "^node " n " cpus:" {sub(/^node [0-9]+ cpus:[ \t]*/, ""); print; exit}')"
      if [[ -z "$cpu_list" ]]; then
        cpu_list="$(lscpu -p=cpu,node 2>/dev/null | awk -F',' -v n="$i" '!/^#/ && $2 == n {print $1}' | paste -sd, -)"
      fi
      HW_NUMA_CPUS[$i]="$cpu_list"
      log_info "  NUMA node ${i}: ${cpu_list:-<unknown>}"
      i=$(( i + 1 ))
    done
  else
    local i=0 cpu_list
    while [[ $i -lt $HW_NUMA_NODES ]]; do
      cpu_list="$(lscpu -p=cpu,node 2>/dev/null | awk -F',' -v n="$i" '!/^#/ && $2 == n {print $1}' | paste -sd, -)"
      HW_NUMA_CPUS[$i]="$cpu_list"
      log_info "  NUMA node ${i}: ${cpu_list:-<unknown>}"
      i=$(( i + 1 ))
    done
  fi
  log_info "映射 GPU → NUMA 节点 ..."
  local i=0 line node bus_addr
  while [[ $i -lt $HW_GPU_COUNT ]]; do
    node=""
    if have nvidia-smi; then
      line="$(nvidia-smi topo -m 2>/dev/null | awk -v gpu="GPU${i}" '$1 == gpu && NF > 5 {print; exit}' || true)"
      if [[ -n "$line" ]]; then
        node="$(printf '%s\n' "$line" | awk '{for (j=NF; j>=1; j--) if ($j ~ /^[0-9]+$/) {print $j; exit}}')"
      fi
    fi
    if [[ -z "$node" || "$node" == "N/A" ]]; then
      bus_addr="$(nvidia-smi --query-gpu=pci.bus_id --format=csv,noheader,nounits 2>/dev/null | awk -F',' -v g="$i" 'NR==g+1 {gsub(/^ +/,"",$1); print $1}')"
      if [[ -r "/sys/bus/pci/devices/${bus_addr}/numa_node" ]]; then
        node="$(cat "/sys/bus/pci/devices/${bus_addr}/numa_node" 2>/dev/null || echo "")"
      fi
    fi
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
  local missing=()
  for tool in docker nvidia-smi nproc awk numactl lscpu; do
    have "$tool" || missing+=("$tool")
  done
  [[ ${#missing[@]} -gt 0 ]] && log_warn "以下工具缺失（部分功能将降级）: ${missing[*]}"
  if have nvidia-smi; then
    local drv cuda
    drv="$(nvidia-smi --query-gpu=driver_version --format=csv,noheader,nounits 2>/dev/null | head -n1 | tr -d ' ' || true)"
    cuda="$(nvidia-smi 2>/dev/null | grep -oE 'CUDA Version:[[:space:]]*[0-9.]+' | head -n1 | sed -E 's/.*[[:space:]]+//' || true)"
    log_info "  NVIDIA 驱动: ${drv:-?}   CUDA 运行时: ${cuda:-?}"
  fi
  if [[ -r /etc/os-release ]]; then
    # shellcheck disable=SC1091
    . /etc/os-release
    log_info "  操作系统: ${PRETTY_NAME:-?}  内核: $(uname -r)"
  fi
  if (( _cgroup_v2 == 1 )); then
    log_info "  cgroup: v2（已自动跳过 --memory-swappiness 以消除 docker 警告）"
  else
    log_info "  cgroup: v1（--memory-swappiness 生效）"
  fi
  detect_docker_gpu_backend
}

# -----------------------------------------------------------------------------
# 5. 实例配置加载（核心：把环境变量转为实例数组）
# -----------------------------------------------------------------------------
#
# 设计：
#   - QWEN_VLLM_INSTANCES = 实例数量（默认 2）
#   - 每个实例可独立设置以下变量：
#       QWEN_VLLM_INSTANCE_<idx>_NAME        容器名（默认：${CONTAINER_NAME_PREFIX}-${idx}）
#       QWEN_VLLM_INSTANCE_<idx>_GPUS        GPU 列表 CSV（默认：自动均分）
#       QWEN_VLLM_INSTANCE_<idx>_TP          张量并行度（默认：实例 GPU 数）
#       QWEN_VLLM_INSTANCE_<idx>_PORT         宿主机端口（默认：BASE_PORT + idx）
#       QWEN_VLLM_INSTANCE_<idx>_MODEL_NAME   --served-model-name（默认：${BASE}-${idx}）
#
#   - 未显式设置的实例使用默认值
#   - 默认 2 实例分配：
#       实例 0 → GPU0+1 → TP=2 → 端口 8000 → 模型名 qwen3.6-27b-fp8（共享）
#       实例 1 → GPU2+3 → TP=2 → 端口 8001 → 模型名 qwen3.6-27b-fp8（共享）
#   - 如需独立 model name，设 QWEN_VLLM_INSTANCE_<idx>_MODEL_NAME 覆盖
# -----------------------------------------------------------------------------
load_instance_config() {
  # 自检：未执行过硬件探测时自动补齐（让 stop/list 等独立命令也能直接用）
  if (( HW_GPU_COUNT == 0 )); then
    detect_cpu_topology >/dev/null
    detect_memory >/dev/null
    detect_gpu_topology >/dev/null
    detect_numa_topology >/dev/null
  fi

  INST_COUNT="${QWEN_VLLM_INSTANCES:-$INSTANCES_DEFAULT}"
  is_int "$INST_COUNT" || die "QWEN_VLLM_INSTANCES 必须是整数: $INST_COUNT"
  (( INST_COUNT >= 1 )) || die "实例数量必须 ≥ 1"

  # GPU 数必须 ≥ 实例数（每个实例至少 1 张 GPU）
  if (( HW_GPU_COUNT > 0 && HW_GPU_COUNT < INST_COUNT )); then
    die "GPU 数量 ($HW_GPU_COUNT) 不足以分给 $INST_COUNT 个实例"
  fi

  # 默认容器名前缀
  local prefix="${QWEN_VLLM_CONTAINER_NAME_PREFIX:-weknora-qwen-vllm}"
  local suffix_chars=(a b c d e f g h i j k l m n o p)

  local i
  for (( i=0; i<INST_COUNT; i++ )); do
    local name_v="QWEN_VLLM_INSTANCE_${i}_NAME"
    local gpus_v="QWEN_VLLM_INSTANCE_${i}_GPUS"
    local tp_v="QWEN_VLLM_INSTANCE_${i}_TP"
    local port_v="QWEN_VLLM_INSTANCE_${i}_PORT"
    local mn_v="QWEN_VLLM_INSTANCE_${i}_MODEL_NAME"

    # 后缀字符（默认 a/b/c/...）
    local suf="${suffix_chars[$i]:-$i}"

    # NAME
    local nm="${!name_v:-${prefix}-${suf}}"
    INST_NAME+=("$nm")

    # GPUS（自动均分）
    local gpus="${!gpus_v:-}"
    if [[ -z "$gpus" && $HW_GPU_COUNT -gt 0 ]]; then
      local per=$(( HW_GPU_COUNT / INST_COUNT ))
      local rem=$(( HW_GPU_COUNT % INST_COUNT ))
      # 余数分配给前 rem 个实例，每个多分 1 张
      local sz=$per
      (( i < rem )) && sz=$(( per + 1 ))
      local start=$(( i * per + (i < rem ? i : rem) ))
      local end=$(( start + sz - 1 ))
      gpus="$(seq -s, "$start" "$end")"
    fi
    [[ -z "$gpus" ]] && die "实例 $i 未指定 GPU 列表 (QWEN_VLLM_INSTANCE_${i}_GPUS) 且无法自动分配"
    INST_GPUS+=("$gpus")

    # 校验该实例 GPU 数量
    local -a gpu_arr=()
    csv_to_array "$gpus" ',' gpu_arr
    INST_GPU_COUNT+=("${#gpu_arr[@]}")

    # TP（默认 = 实例 GPU 数）
    local tp="${!tp_v:-${#gpu_arr[@]}}"
    is_int "$tp" || die "实例 $i 的 TP 非整数: $tp"
    (( tp >= 1 && tp <= ${#gpu_arr[@]} )) || die "实例 $i 的 TP=$tp 超出 [1, ${#gpu_arr[@]}]"
    INST_TP+=("$tp")

    # PORT
    INST_PORT+=("${!port_v:-$(( BASE_PORT + i ))}")

    # MODEL_NAME
    # 默认所有实例同名；用户可单独覆盖（QWEN_VLLM_INSTANCE_<idx>_MODEL_NAME）
    INST_MODEL_NAME+=("${!mn_v:-$SERVED_MODEL_NAME_BASE}")
  done

  log_ok "已加载 ${INST_COUNT} 个实例配置:"
  for (( i=0; i<INST_COUNT; i++ )); do
    log_ok "  实例 ${i}: name=${INST_NAME[$i]}  gpus=${INST_GPUS[$i]} (${INST_GPU_COUNT[$i]} 张)  tp=${INST_TP[$i]}  port=${INST_PORT[$i]}  model=${INST_MODEL_NAME[$i]}"
  done
}

# -----------------------------------------------------------------------------
# 6. NUMA 调度布局（按实例）
# -----------------------------------------------------------------------------
#
# 策略：
#   - 每个实例的 NUMA 节点 = 该实例所有 GPU 的 NUMA 节点并集
#   - 该实例 cpuset = 对应 NUMA 节点的全部 CPU 中取 TP_SIZE * CPU_THREADS_PER_WORKER 个
#   - 该实例 cpuset-mems = 对应 NUMA 节点列表
#
# 本机拓扑（实测）：
#   GPU0/1 → NUMA0 (cpus 0-15,64-79)
#   GPU2   → NUMA2 (cpus 32-47,96-111)
#   GPU3   → NUMA3 (cpus 48-63,112-127)
# -----------------------------------------------------------------------------
compute_layout_for_instance() {
  local idx="$1"
  local gpus="${INST_GPUS[$idx]}"
  local tp="${INST_TP[$idx]}"
  local threads_needed=$(( tp * CPU_THREADS_PER_WORKER ))

  local -a gpu_arr=()
  csv_to_array "$gpus" ',' gpu_arr

  # 收集该实例触及的 NUMA 节点
  local -a numa_set=()
  local g
  for g in "${gpu_arr[@]}"; do
    local n="${HW_GPU_NUMA[$g]:-}"
    [[ -z "$n" ]] && continue
    case ",$(IFS=,; echo "${numa_set[*]}")," in
      *,"$n",*) ;;
      *) numa_set+=("$n") ;;
    esac
  done

  if [[ ${#numa_set[@]} -eq 0 ]]; then
    log_warn "实例 ${idx} 无法推导 NUMA，回退到全机 CPU"
    INST_CPUSET_CPUS[$idx]="0-$(( HW_TOTAL_THREADS - 1 ))"
    INST_CPUSET_MEMS[$idx]="0"
    INST_WORKER_CPUS[$idx]="$INST_CPUSET_CPUS[$idx]"
    return 0
  fi

  # 收集这些 NUMA 的全部 CPU
  # 注意：HW_NUMA_CPUS 是空格分隔（numactl --hardware 原始输出），需先转 CSV
  local -a all_cpus=()
  local n c cpus_csv
  for n in "${numa_set[@]}"; do
    local -a cpus_n=()
    cpus_csv="$(printf '%s' "${HW_NUMA_CPUS[$n]:-}" | tr ' \t' ',' | sed -E 's/,+/,/g; s/^,//; s/,$//')"
    IFS=',' read -r -a cpus_n <<< "$cpus_csv"
    all_cpus+=("${cpus_n[@]}")
  done

  # 取前 threads_needed 个作为 worker CPU 集
  local -a picked=()
  for (( c=0; c<${#all_cpus[@]} && c<threads_needed; c++ )); do
    picked+=("${all_cpus[$c]}")
  done

  local joined
  joined="$(IFS=,; echo "${picked[*]}")"
  INST_CPUSET_CPUS[$idx]="$(compress_cpuset "$joined")"
  INST_CPUSET_MEMS[$idx]="$(IFS=,; echo "${numa_set[*]}")"
  INST_WORKER_CPUS[$idx]="$joined"

  log_inst "$idx" "NUMA 调度: GPUs=${gpus} NUMA=${numa_set[*]} cpuset=${INST_CPUSET_CPUS[$idx]} mems=${INST_CPUSET_MEMS[$idx]} 占用线程=${#picked[@]}/${threads_needed}"
}

# 精细版：按 worker (rank) 从各自 GPU 的本地 NUMA 拉线程
# 适用于 TP=2/4 等每个 worker 绑一张 GPU 的场景
compute_layout_for_instance_per_worker() {
  local idx="$1"
  local gpus="${INST_GPUS[$idx]}"
  local tp="${INST_TP[$idx]}"

  local -a gpu_arr=()
  csv_to_array "$gpus" ',' gpu_arr

  # 每个 worker 取 CPU_THREADS_PER_WORKER 个本地 NUMA 线程
  local -a picked=()
  local -a numa_set=()
  local g n cpus_csv cpus_n
  for (( g=0; g<tp && g<${#gpu_arr[@]}; g++ )); do
    local gpu_id="${gpu_arr[$g]}"
    local numa="${HW_GPU_NUMA[$gpu_id]:-}"

    if [[ -z "$numa" ]]; then
      log_warn "  GPU${gpu_id} 无 NUMA 关联，回退 NUMA0"
      numa=0
    fi

    # 记录 NUMA
    case ",$(IFS=,; echo "${numa_set[*]}")," in
      *,"$numa",*) ;;
      *) numa_set+=("$numa") ;;
    esac

    # 取该 NUMA 的 CPU，跳过前 (g * threads_needed) 个，避免 worker 间重叠
    local -a numa_cpus=()
    cpus_csv="$(printf '%s' "${HW_NUMA_CPUS[$numa]:-}" | tr ' \t' ',' | sed -E 's/,+/,/g; s/^,//; s/,$//')"
    IFS=',' read -r -a cpus_n <<< "$cpus_csv"
    numa_cpus=("${cpus_n[@]}")

    local start=$(( g * CPU_THREADS_PER_WORKER ))
    local end=$(( start + CPU_THREADS_PER_WORKER - 1 ))
    (( end >= ${#numa_cpus[@]} )) && end=$(( ${#numa_cpus[@]} - 1 ))

    log_inst "$idx" "  worker ${g} (GPU${gpu_id} @ NUMA${numa}) → CPUs ${numa_cpus[$start]}-${numa_cpus[$end]}"

    for (( c=start; c<=end; c++ )); do
      picked+=("${numa_cpus[$c]}")
    done
  done

  local joined
  joined="$(IFS=,; echo "${picked[*]}")"
  INST_CPUSET_CPUS[$idx]="$(compress_cpuset "$joined")"
  INST_CPUSET_MEMS[$idx]="$(IFS=,; echo "${numa_set[*]}")"
  INST_WORKER_CPUS[$idx]="$joined"

  log_inst "$idx" "NUMA 调度(per-worker): cpuset=${INST_CPUSET_CPUS[$idx]} mems=${INST_CPUSET_MEMS[$idx]} 占用线程=${#picked[@]}"
}

compute_all_layouts() {
  log_info "计算 ${INST_COUNT} 个实例的 NUMA 调度布局 (per-worker) ..."
  local i
  for (( i=0; i<INST_COUNT; i++ )); do
    # 每个 worker 绑本地 NUMA：worker 0 → GPU0 的 NUMA，worker 1 → GPU1 的 NUMA...
    compute_layout_for_instance_per_worker "$i"
  done
}

# -----------------------------------------------------------------------------
# 7. 系统级调优
# -----------------------------------------------------------------------------
# 检测 cgroup 版本（v2 不支持 --memory-swappiness）
_cgroup_v2=0
if [[ -f /sys/fs/cgroup/cgroup.controllers ]]; then
  _cgroup_v2=1
fi

apply_system_tuning() {
  [[ "$APPLY_SYSCTLS" == "1" ]] || { log_info "跳过 sysctl 调优"; return 0; }
  log_info "应用内核级调优 ..."
  have sysctl || { log_warn "无 sysctl，跳过"; return 0; }
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
        log_warn "  sysctl ${key}=${val} 设置失败（可能需要 root）"
      fi
    fi
  done
  if [[ "$APPLY_TRANSPARENT_HUGEPAGE" == "1" && -w /sys/kernel/mm/transparent_hugepage/enabled ]]; then
    if echo madvise > /sys/kernel/mm/transparent_hugepage/enabled 2>/dev/null; then
      log_info "  透明大页: madvise"
    fi
  fi
  if [[ "$APPLY_CPU_GOVERNOR" == "1" && -d /sys/devices/system/cpu/cpu0/cpufreq ]]; then
    local cpu
    for cpu in /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor; do
      [[ -w "$cpu" ]] && echo performance > "$cpu" 2>/dev/null || true
    done
    log_info "  CPU 调度器: performance (尝试设置)"
  fi
  log_ok "系统级调优完成"
}

# -----------------------------------------------------------------------------
# 8. Docker 客户端解析
# -----------------------------------------------------------------------------
resolve_docker_cmd() {
  : "${DOCKER_BIN:=docker}"
  docker_cmd=("$DOCKER_BIN")
  if ! command -v "$DOCKER_BIN" >/dev/null 2>&1; then
    die "找不到 docker 命令 (DOCKER_BIN=$DOCKER_BIN)"
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

detect_docker_gpu_backend() {
  local runtimes_json="" cdi_spec="" cdi_hook="" runtime_ready=0
  local -a missing_bins=()

  runtimes_json="$("${docker_cmd[@]}" info --format '{{json .Runtimes}}' 2>/dev/null || true)"
  if grep -q '"nvidia"' <<< "$runtimes_json"; then
    if have nvidia-container-runtime; then
      runtime_ready=1
    else
      missing_bins+=( nvidia-container-runtime )
    fi
  fi

  for cdi_spec in /etc/cdi/nvidia.yaml /var/run/cdi/nvidia.yaml; do
    [[ -f "$cdi_spec" ]] || continue
    cdi_hook="$(awk '/hookName: createContainer/{seen=1} seen && $1 == "path:" {print $2; exit}' "$cdi_spec" 2>/dev/null || true)"
    [[ -z "$cdi_hook" ]] && cdi_hook="/usr/bin/nvidia-cdi-hook"
    if [[ -x "$cdi_hook" ]]; then
      DOCKER_GPU_BACKEND="cdi"
      DOCKER_NVIDIA_CDI_SPEC="$cdi_spec"
      log_info "  Docker GPU 后端: NVIDIA CDI (${cdi_spec})"
      return 0
    fi
    missing_bins+=( "$cdi_hook" )
  done

  if (( runtime_ready == 1 )); then
    DOCKER_GPU_BACKEND="runtime"
    log_info "  Docker GPU 后端: nvidia runtime ($(command -v nvidia-container-runtime))"
    return 0
  fi

  if [[ ${#missing_bins[@]} -gt 0 ]]; then
    local missing_joined
    missing_joined="$(printf '%s\n' "${missing_bins[@]}" | awk '!seen[$0]++' | paste -sd, -)"
    die "Docker GPU 后端不可用：缺少 ${missing_joined}。请先执行 ${SCRIPT_DIR}/check_prereqs.sh，或安装 nvidia-container-toolkit 后重试。"
  fi

  die "Docker GPU 后端不可用：未检测到可用的 nvidia runtime 或 NVIDIA CDI 规范。请先安装并配置 nvidia-container-toolkit。"
}

build_gpu_args_for_instance() {
  local gpus="$1"
  local -n _env_args="$2"
  local -n _gpu_args="$3"
  local -a gpu_arr=()
  local gpu_id

  case "$DOCKER_GPU_BACKEND" in
    cdi)
      csv_to_array "$gpus" ',' gpu_arr
      for gpu_id in "${gpu_arr[@]}"; do
        _gpu_args+=( --device "nvidia.com/gpu=${gpu_id}" )
      done
      ;;
    runtime)
      _gpu_args+=( --runtime=nvidia )
      _env_args+=(
        -e "NVIDIA_VISIBLE_DEVICES=${gpus}"
        -e "NVIDIA_DRIVER_CAPABILITIES=compute,utility"
      )
      ;;
    *)
      die "Docker GPU 后端未初始化，请先执行 preflight。"
      ;;
  esac
}

container_exists_for() {
  local name="$1"
  [[ -n "$("${docker_cmd[@]}" ps -aq -f "name=^/${name}$")" ]]
}

container_running_for() {
  local name="$1"
  [[ "$("${docker_cmd[@]}" inspect -f '{{.State.Running}}' "$name" 2>/dev/null || true)" == "true" ]]
}

check_ports_available() {
  log_info "校验实例端口可用性 ..."
  local i port
  for (( i=0; i<INST_COUNT; i++ )); do
    port="${INST_PORT[$i]}"
    if have ss; then
      if ss -tlnp 2>/dev/null | grep -qE ":${port}[[:space:]]"; then
        die "实例 ${i} 端口 ${port} 已被占用，请更换 QWEN_VLLM_BASE_PORT 或 QWEN_VLLM_INSTANCE_${i}_PORT"
      fi
    elif have netstat; then
      if netstat -tlnp 2>/dev/null | grep -qE ":${port}[[:space:]]"; then
        die "实例 ${i} 端口 ${port} 已被占用"
      fi
    fi
  done
  log_ok "端口校验通过"
}

# -----------------------------------------------------------------------------
# 9. 模型源解析（同 v1）
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
      local rev="" host_snap=""
      [[ -f "$QWEN_CACHE_REPO_DIR/refs/main" ]] && rev="$(tr -d '\n\r' < "$QWEN_CACHE_REPO_DIR/refs/main")"
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
# 10. 容器 run 命令构造（每实例）
# -----------------------------------------------------------------------------
dry_run=0

run_container_for_instance() {
  local idx="$1"
  local name="${INST_NAME[$idx]}"
  local gpus="${INST_GPUS[$idx]}"
  local tp="${INST_TP[$idx]}"
  local port="${INST_PORT[$idx]}"
  local model_name="${INST_MODEL_NAME[$idx]}"
  local cpuset_cpus="${INST_CPUSET_CPUS[$idx]}"
  local cpuset_mems="${INST_CPUSET_MEMS[$idx]}"

  log_inst "$idx" "构造 docker run 命令 ..."

  local -a env_args=() runtime_args=() placement_args=() gpu_args=() health_args=()

  # 共享环境变量
  env_args+=(
    -e "HF_HUB_DISABLE_XET=1"
    -e "PYTORCH_CUDA_ALLOC_CONF=expandable_segments:True"
    -e "OMP_NUM_THREADS=${CPU_THREADS_PER_WORKER}"
    -e "MKL_NUM_THREADS=${CPU_THREADS_PER_WORKER}"
    -e "OPENBLAS_NUM_THREADS=${CPU_THREADS_PER_WORKER}"
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

  # 调度
  [[ -n "$cpuset_cpus" ]] && placement_args+=( --cpuset-cpus="$cpuset_cpus" )
  [[ -n "$cpuset_mems" ]] && placement_args+=( --cpuset-mems="$cpuset_mems" )

  # GPU
  build_gpu_args_for_instance "$gpus" env_args gpu_args

  # 健康检查
  health_args+=(
    --health-cmd="curl -sf http://localhost:${CONTAINER_PORT}/health || exit 1"
    --health-interval="${HEALTH_INTERVAL}"
    --health-timeout="${HEALTH_TIMEOUT}"
    --health-start-period="${HEALTH_START_PERIOD}"
    --health-retries="${HEALTH_RETRIES}"
  )

  # vLLM runtime
  runtime_args+=(
    --host 0.0.0.0
    --port "$CONTAINER_PORT"
    --model "$EFFECTIVE_MODEL_SOURCE"
    --served-model-name "$model_name"
    --tensor-parallel-size "$tp"
    --pipeline-parallel-size 1
    --gpu-memory-utilization "$GPU_MEMORY_UTILIZATION"
    --kv-cache-dtype "$KV_CACHE_DTYPE"
    --dtype auto
    --trust-remote-code
    --max-model-len "$MAX_MODEL_LEN"
    --max-num-seqs "$MAX_NUM_SEQS"
    --max-num-batched-tokens "$MAX_NUM_BATCHED_TOKENS"
    --max-cudagraph-capture-size "$MAX_CUDAGRAPH_CAPTURE_SIZE"
  )
  [[ "$PREFIX_CACHING_ENABLED" == "1" ]] && runtime_args+=( --enable-prefix-caching )
  [[ "$CHUNKED_PREFILL_ENABLED" == "1" ]] && runtime_args+=( --enable-chunked-prefill )
  [[ "$ENFORCE_EAGER" == "1" ]] && runtime_args+=( --enforce-eager )
  [[ "$ENABLE_AUTO_TOOL_CHOICE" == "1" ]] && runtime_args+=( --enable-auto-tool-choice --tool-call-parser "$TOOL_CALL_PARSER" )
  [[ -n "$REASONING_PARSER" ]] && runtime_args+=( --reasoning-parser "$REASONING_PARSER" )
  [[ -n "$DEFAULT_CHAT_TEMPLATE_KWARGS" ]] && runtime_args+=( --default-chat-template-kwargs "$DEFAULT_CHAT_TEMPLATE_KWARGS" )
  [[ "$ENABLE_MTP" == "1" ]] && runtime_args+=( --speculative-config '{"method":"mtp","num_speculative_tokens":2}' )
  [[ "$LANGUAGE_MODEL_ONLY" == "1" ]] && runtime_args+=( --language-model-only )
  runtime_args+=( --api-key "$API_KEY" )

  # dry-run 打印
  if [[ "$dry_run" == "1" ]]; then
    echo "[DRY-RUN ${name}] 将执行 docker run:" >&2
    printf '  %q ' "${docker_cmd[@]}" run -d >&2
    printf -- '--name %q ' "$name" >&2
    printf -- '--restart %q ' "$DOCKER_RESTART_POLICY" >&2
    printf -- '--ipc %q ' "host" >&2
    printf -- '--shm-size=%q ' "$SHM_SIZE" >&2
    printf -- '--ulimit %q ' "memlock=-1" >&2
    printf -- '--ulimit %q ' "stack=67108864" >&2
    printf -- '--cpu-shares=%q ' "$DOCKER_CPU_SHARES" >&2
    # cgroup v2 不支持 --memory-swappiness
    (( _cgroup_v2 == 0 )) && printf -- '--memory-swappiness=%q ' "$DOCKER_MEM_SWAPPINESS" >&2
    for a in "${placement_args[@]}"; do printf '%q ' "$a" >&2; done
    for a in "${gpu_args[@]}";     do printf '%q ' "$a" >&2; done
    for a in "${health_args[@]}";  do printf '%q ' "$a" >&2; done
    printf -- '--log-opt %q ' "max-size=100m" >&2
    printf -- '--log-opt %q ' "max-file=5" >&2
    printf -- '-p %q ' "${port}:${CONTAINER_PORT}" >&2
    for a in "${env_args[@]}"; do printf '%q ' "$a" >&2; done
    printf -- '-v %q ' "${HF_CACHE_ROOT}:/root/.cache/huggingface" >&2
    printf -- '-v %q ' "${MODEL_ROOT}:/models" >&2
    printf '%q ' "$IMAGE" >&2
    for a in "${runtime_args[@]}"; do printf '%q ' "$a" >&2; done
    printf '\n' >&2
    return 0
  fi

  # cgroup v2 不支持 --memory-swappiness（docker 会输出 WARNING 然后丢弃该 flag）
  local -a mem_args=()
  if (( _cgroup_v2 == 0 )); then
    mem_args+=( --memory-swappiness="$DOCKER_MEM_SWAPPINESS" )
  fi

  "${docker_cmd[@]}" run -d \
    --name "$name" \
    --restart "$DOCKER_RESTART_POLICY" \
    --ipc host \
    --shm-size="$SHM_SIZE" \
    --ulimit memlock=-1 \
    --ulimit stack=67108864 \
    --cpu-shares="$DOCKER_CPU_SHARES" \
    "${mem_args[@]}" \
    "${placement_args[@]}" \
    "${gpu_args[@]}" \
    "${health_args[@]}" \
    --log-opt max-size=100m \
    --log-opt max-file=5 \
    -p "${port}:${CONTAINER_PORT}" \
    "${env_args[@]}" \
    -v "${HF_CACHE_ROOT}:/root/.cache/huggingface" \
    -v "${MODEL_ROOT}:/models" \
    "$IMAGE" \
    "${runtime_args[@]}"
}

# -----------------------------------------------------------------------------
# 11. 生命周期命令（按实例）
# -----------------------------------------------------------------------------
preflight() {
  log_info "===== 预检: ${SCRIPT_NAME} v${SCRIPT_VERSION} ====="
  detect_cpu_topology
  detect_memory
  detect_gpu_topology
  detect_numa_topology
  detect_system_capabilities
  load_instance_config
  compute_all_layouts
  apply_system_tuning
  log_info "===== 预检完成 ====="
}

start_instance() {
  local idx="$1"
  local name="${INST_NAME[$idx]}"

  log_inst "$idx" "启动容器: $name"

  if [[ "$dry_run" != "1" ]]; then
    if container_running_for "$name"; then
      log_inst "$idx" "$name 已在运行"
      return 0
    fi
    if container_exists_for "$name"; then
      log_inst "$idx" "发现旧容器，删除后重建"
      "${docker_cmd[@]}" rm -f "$name" >/dev/null
    fi
  fi

  if run_container_for_instance "$idx"; then
    log_inst "$idx" "已启动 $name (host:${INST_PORT[$idx]} -> container:${CONTAINER_PORT}, gpus=${INST_GPUS[$idx]})"
    [[ "$dry_run" == "1" ]] && return 0

    # 默认不阻塞等 vLLM 就绪（避免长时间挂起）；可通过 QWEN_VLLM_WAIT_HEALTHY=1 恢复旧行为
    if [[ "$WAIT_HEALTHY_ON_START" == "1" ]]; then
      post_start_check_for "$idx"
    else
      log_inst "$idx" "容器已创建，vLLM 模型仍在加载中（约需 1–5 分钟）"
      log_inst "$idx" "后续操作："
      log_inst "$idx" "  查看实时日志: ${SCRIPT_NAME} logs ${idx}"
      log_inst "$idx" "  阻塞等就绪:   ${SCRIPT_NAME} wait-healthy ${idx}"
      log_inst "$idx" "  快速健康检查: ${SCRIPT_NAME} health ${idx}"
    fi
  else
    log_inst "$idx" "容器启动失败，请查看 logs（不会自动重试）"
    return 1
  fi
}

start_all() {
  resolve_docker_cmd
  preflight
  validate_model_source
  [[ "$dry_run" == "1" ]] || check_ports_available
  local i rc=0
  for (( i=0; i<INST_COUNT; i++ )); do
    if ! start_instance "$i"; then rc=1; fi
  done
  [[ $rc -eq 0 ]] && log_ok "所有实例已启动（或 dry-run 完成）"
}

start_specific() {
  local target="$1"
  resolve_docker_cmd
  preflight
  validate_model_source
  [[ "$dry_run" == "1" ]] || check_ports_available
  local i rc=0
  for (( i=0; i<INST_COUNT; i++ )); do
    if [[ "$target" == "all" || "$target" == "$i" || "$target" == "${INST_NAME[$i]}" ]]; then
      if ! start_instance "$i"; then rc=1; fi
    fi
  done
  return $rc
}

post_start_check_for() {
  local idx="$1"
  local name="${INST_NAME[$idx]}"
  local port="${INST_PORT[$idx]}"
  log_inst "$idx" "等待容器进入 running 状态 ..."
  local waited=0 max_wait=60
  while (( waited < max_wait )); do
    if container_running_for "$name"; then
      log_inst "$idx" "容器已 running（等待 ${waited}s）"
      break
    fi
    sleep 2; waited=$(( waited + 2 ))
  done
  if ! container_running_for "$name"; then
    log_inst "$idx" "容器在 ${max_wait}s 内未进入 running，请检查 logs"
    return 1
  fi

  # 把 "300s" / "5m" 转成秒；默认按 300s 处理
  local period_s
  case "${HEALTH_START_PERIOD}" in
    *s) period_s="${HEALTH_START_PERIOD%s}" ;;
    *m) period_s=$(( ${HEALTH_START_PERIOD%m} * 60 )) ;;
    *)  period_s="${HEALTH_START_PERIOD}" ;;
  esac
  is_int "$period_s" || period_s=300

  log_inst "$idx" "首次探活: http://localhost:${port}/v1/models (timeout=${period_s}s)"
  local curl_wait=0
  while (( curl_wait < period_s )); do
    if curl -sf -H "Authorization: Bearer ${API_KEY}" "http://localhost:${port}/v1/models" >/dev/null 2>&1; then
      log_inst "$idx" "vLLM OpenAI 兼容接口就绪（${curl_wait}s）"
      return 0
    fi
    sleep 10; curl_wait=$(( curl_wait + 10 ))
    (( curl_wait % 60 == 0 && curl_wait > 0 )) && log_inst "$idx" "仍在等待模型加载（${curl_wait}s / ${period_s}s）..."
  done
  log_inst "$idx" "vLLM 在 ${period_s}s 内未就绪，请通过 logs 查看"
  return 1
}

stop_instance() {
  local idx="$1"
  local name="${INST_NAME[$idx]}"
  if ! container_exists_for "$name"; then
    log_inst "$idx" "$name 不存在"
    return 0
  fi
  log_inst "$idx" "优雅停止 $name ..."
  "${docker_cmd[@]}" stop -t "$STOP_TIMEOUT" "$name" >/dev/null 2>&1 || true
  "${docker_cmd[@]}" rm -f "$name" >/dev/null
  log_inst "$idx" "已移除 $name"
}

stop_all() {
  resolve_docker_cmd
  load_instance_config
  local i
  for (( i=0; i<INST_COUNT; i++ )); do
    stop_instance "$i"
  done
  log_ok "所有实例已停止"
}

stop_specific() {
  local target="$1"
  resolve_docker_cmd
  load_instance_config
  local i
  for (( i=0; i<INST_COUNT; i++ )); do
    if [[ "$target" == "all" || "$target" == "$i" || "$target" == "${INST_NAME[$i]}" ]]; then
      stop_instance "$i"
    fi
  done
}

restart_specific() {
  local target="$1"
  stop_specific "$target"
  start_specific "$target"
}

reset_specific() {
  local target="$1"
  resolve_docker_cmd
  preflight
  validate_model_source

  # 仅停止目标实例
  local i
  for (( i=0; i<INST_COUNT; i++ )); do
    if [[ "$target" == "all" || "$target" == "$i" || "$target" == "${INST_NAME[$i]}" ]]; then
      local name="${INST_NAME[$i]}"
      container_exists_for "$name" && "${docker_cmd[@]}" rm -f "$name" >/dev/null 2>&1 || true
    fi
  done

  if "${docker_cmd[@]}" image inspect "$IMAGE" >/dev/null 2>&1; then
    log_info "删除旧镜像: $IMAGE"
    "${docker_cmd[@]}" image rm -f "$IMAGE" >/dev/null 2>&1 || true
  fi
  cleanup_partial_cache
  log_info "重新拉取镜像: $IMAGE"
  "${docker_cmd[@]}" pull "$IMAGE"
  [[ "$dry_run" == "1" ]] || check_ports_available

  for (( i=0; i<INST_COUNT; i++ )); do
    if [[ "$target" == "all" || "$target" == "$i" || "$target" == "${INST_NAME[$i]}" ]]; then
      start_instance "$i"
    fi
  done
}

status_all() {
  resolve_docker_cmd
  detect_cpu_topology
  detect_memory
  detect_gpu_topology
  detect_numa_topology
  load_instance_config
  compute_all_layouts

  log_info "===== 实例状态 ====="
  "${docker_cmd[@]}" ps -a --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}' \
    | (head -n1; "${docker_cmd[@]}" ps -a --filter "name=^/weknora-qwen-vllm" --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}' | tail -n +2 | sort) || true

  echo
  echo "===== 实例配置 ====="
  printf '%-4s %-30s %-8s %-4s %-6s %-25s\n' 'IDX' 'NAME' 'GPUS' 'TP' 'PORT' 'MODEL'
  echo "------------------------------------------------------------------------------------"
  local i
  for (( i=0; i<INST_COUNT; i++ )); do
    printf '%-4s %-30s %-8s %-4s %-6s %-25s\n' \
      "$i" "${INST_NAME[$i]}" "${INST_GPUS[$i]}" "${INST_TP[$i]}" "${INST_PORT[$i]}" "${INST_MODEL_NAME[$i]}"
  done

  echo
  echo "===== NUMA 调度 ====="
  for (( i=0; i<INST_COUNT; i++ )); do
    printf '实例 %d: cpuset-cpus=%-25s cpuset-mems=%s\n' \
      "$i" "${INST_CPUSET_CPUS[$i]}" "${INST_CPUSET_MEMS[$i]}"
  done
}

logs_for() {
  local target="${1:-all}"
  resolve_docker_cmd
  detect_gpu_topology
  load_instance_config
  local i found=0
  for (( i=0; i<INST_COUNT; i++ )); do
    if [[ "$target" == "all" || "$target" == "$i" || "$target" == "${INST_NAME[$i]}" ]]; then
      local name="${INST_NAME[$i]}"
      if container_exists_for "$name"; then
        log_inst "$i" "跟踪日志: $name (Ctrl+C 退出)"
        # 多实例日志加前缀便于区分
        "${docker_cmd[@]}" logs --tail "${QWEN_VLLM_LOG_TAIL:-200}" -f "$name" 2>&1 \
          | sed -u "s/^/[${name}] /" &
        found=1
      else
        log_inst "$i" "$name 不存在（容器未创建）"
      fi
    fi
  done
  if (( found == 0 )); then
    log_warn "未找到匹配 'target=$target' 的实例，或所有实例都未运行"
    return 1
  fi
  wait
}

health_for() {
  local idx="$1"
  local name="${INST_NAME[$idx]}"
  local port="${INST_PORT[$idx]}"
  local rc=0

  if ! container_running_for "$name"; then
    log_inst "$idx" "[HEALTH] 容器未运行"
    return 1
  fi

  local dhealth
  dhealth="$("${docker_cmd[@]}" inspect -f '{{.State.Health.Status}}' "$name" 2>/dev/null || echo none)"
  log_inst "$idx" "[HEALTH] docker health=${dhealth}"

  if curl -sf "http://localhost:${port}/health" >/dev/null 2>&1; then
    log_inst "$idx" "[HEALTH] /health 200 OK"
  else
    log_inst "$idx" "[HEALTH] /health 失败"
    rc=1
  fi

  if curl -sf -H "Authorization: Bearer ${API_KEY}" "http://localhost:${port}/v1/models" >/dev/null 2>&1; then
    log_inst "$idx" "[HEALTH] /v1/models 200 OK"
  else
    log_inst "$idx" "[HEALTH] /v1/models 失败"
    rc=1
  fi

  # GPU 快照（仅打印该实例的 GPU）
  if have nvidia-smi; then
    local -a gpu_arr=()
    csv_to_array "${INST_GPUS[$idx]}" ',' gpu_arr
    local g
    for g in "${gpu_arr[@]}"; do
      local snap
      snap="$(nvidia-smi --query-gpu=index,utilization.gpu,memory.used,memory.total --format=csv,noheader,nounits 2>/dev/null | awk -F',' -v gg="$g" '$1+0 == gg+0 {print}')"
      [[ -n "$snap" ]] && log_inst "$idx" "[HEALTH] GPU${g}: $snap"
    done
  fi

  return $rc
}

health_all() {
  resolve_docker_cmd
  load_instance_config
  local i rc=0
  for (( i=0; i<INST_COUNT; i++ )); do
    health_for "$i" || rc=1
  done
  return $rc
}

watch_for() {
  local target="${1:-all}"
  resolve_docker_cmd
  load_instance_config
  log_info "进入 watch 模式（间隔 ${WATCH_INTERVAL}s，Ctrl+C 退出）"
  while true; do
    local i
    for (( i=0; i<INST_COUNT; i++ )); do
      if [[ "$target" == "all" || "$target" == "$i" || "$target" == "${INST_NAME[$i]}" ]]; then
        health_for "$i" >/dev/null 2>&1 || log_inst "$i" "健康检查异常"
      fi
    done
    sleep "$WATCH_INTERVAL"
  done
}

# 阻塞等待指定实例 vLLM /v1/models 就绪（替代 start 内置的自动等待）
wait_healthy_for() {
  local target="${1:-all}"
  resolve_docker_cmd
  load_instance_config
  local i rc=0 found=0
  for (( i=0; i<INST_COUNT; i++ )); do
    if [[ "$target" == "all" || "$target" == "$i" || "$target" == "${INST_NAME[$i]}" ]]; then
      found=1
      post_start_check_for "$i" || rc=1
    fi
  done
  (( found == 0 )) && { log_err "未找到实例: $target"; return 1; }
  return $rc
}

# -----------------------------------------------------------------------------
# 12. 性能基准（每实例）
# -----------------------------------------------------------------------------
run_benchmark_for() {
  local idx="$1"; shift
  local port="${INST_PORT[$idx]}"
  local model_name="${INST_MODEL_NAME[$idx]}"

  if ! container_running_for "${INST_NAME[$idx]}"; then
    log_inst "$idx" "实例未运行，跳过基准"
    return 1
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

  log_inst "$idx" "===== 基准测试 (port=${port}, model=${model_name}) ====="
  log_inst "$idx" "并发=${concurrent}  max_tokens=${max_tokens}"

  local endpoint="http://localhost:${port}/v1/chat/completions"
  local payload
  payload="$(cat <<EOF
{
  "model": "${model_name}",
  "messages": [{"role":"user","content":"${prompt}"}],
  "max_tokens": ${max_tokens},
  "temperature": 0.0,
  "stream": false
}
EOF
)"

  local pids=() outs=()
  local i
  for (( i=0; i<concurrent; i++ )); do
    local out_file
    out_file="$(mktemp -t qwen-bench-inst${idx}-XXXXXX.json)"
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

  local total_ms=0 counted=0 lat out
  for out in "${outs[@]}"; do
    [[ -s "$out" ]] || continue
    lat="$(cat "${out}.lat" 2>/dev/null || echo 0)"
    total_ms=$(( total_ms + lat ))
    counted=$(( counted + 1 ))
    local completion
    completion="$(grep -o '"content":"[^"]*"' "$out" 2>/dev/null | head -n1 | cut -c12-200)"
    log_inst "$idx" "请求 ${counted}/${concurrent}: ${lat}ms  -> ${completion:0:80}..."
  done

  local avg=0
  (( counted > 0 )) && avg=$(( total_ms / counted ))

  log_inst "$idx" "===== 完成 =====  成功=${counted}/${concurrent}  总耗时=${total_ms}ms  平均=${avg}ms/req"

  for out in "${outs[@]}"; do
    rm -f "$out" "${out}.lat"
  done
}

bench_all() {
  resolve_docker_cmd
  load_instance_config
  local i
  for (( i=0; i<INST_COUNT; i++ )); do
    run_benchmark_for "$i" "$@"
  done
}

bench_specific() {
  local target="$1"; shift
  resolve_docker_cmd
  load_instance_config
  local i
  for (( i=0; i<INST_COUNT; i++ )); do
    if [[ "$target" == "all" || "$target" == "$i" || "$target" == "${INST_MODEL_NAME[$i]}" ]]; then
      run_benchmark_for "$i" "$@"
    fi
  done
}

# -----------------------------------------------------------------------------
# 13. CLI 分发
# -----------------------------------------------------------------------------
usage() {
  cat <<USAGE_EOF
用法: ./${SCRIPT_NAME} <command> [target] [options]

命令:
  start [target]       预检 + 启动实例（target 可省，默认 all；可填 0/1 或实例名）
  stop [target]        优雅停止实例
  restart [target]     stop + start
  reset [target]       删除容器/镜像 + 清理缓存 + 重拉 + 重启
  status               所有实例状态 + NUMA 拓扑
  logs [target]        跟踪日志（多实例时并行输出，Ctrl+C 全部退出）
  health [target]      单次健康检查
  watch [target]       周期健康检查（默认 15s）
  wait-healthy [target] 阻塞等 vLLM /v1/models 就绪（start 不再自动等待）
  bench [target] [opts] 简单性能基准（target 默认 all）
  list                 列出所有已配置实例
  preflight            仅运行预检
  detect               仅打印硬件检测
  version | -V         脚本版本
  help | -h | --help   打印帮助

target 取值:
  all                  所有实例（默认）
  <数字>               实例下标（0、1、...）
  <实例名>             weknora-qwen-vllm-a / -b
  <served-model-name>  qwen3.6-27b-fp8-a / -b

bench 选项:
  --prompt "..."       自定义提示词
  --max-tokens N       每请求最大输出 token（默认 256）
  --concurrent N       并发请求数（默认 4）

默认实例配置（无需环境变量即可用）:
  实例 0: GPU0+1, TP=2, port=8000, model=qwen3.6-27b-fp8-a
  实例 1: GPU2+3, TP=2, port=8001, model=qwen3.6-27b-fp8-b

环境变量:
  QWEN_VLLM_INSTANCES                       实例数（默认 2）
  QWEN_VLLM_INSTANCE_<idx>_NAME             容器名
  QWEN_VLLM_INSTANCE_<idx>_GPUS             GPU 列表 CSV
  QWEN_VLLM_INSTANCE_<idx>_TP               张量并行度
  QWEN_VLLM_INSTANCE_<idx>_PORT             宿主机端口
  QWEN_VLLM_INSTANCE_<idx>_MODEL_NAME       --served-model-name
  QWEN_VLLM_LANGUAGE_MODEL_ONLY             启用 --language-model-only（默认 1）
  QWEN_VLLM_MAX_CUDAGRAPH_CAPTURE_SIZE      CUDA graph 捕获大小（默认 256）
  QWEN_VLLM_MAX_MODEL_LEN                   上下文长度（默认 262144）
  QWEN_VLLM_CPU_THREADS_PER_WORKER          每 worker 线程（固定 16）
  QWEN_VLLM_NCCL_BUFFSIZE                   NCCL 缓冲（默认 8388608）
  QWEN_VLLM_APPLY_SYSCTLS                   是否应用 sysctl 调优（默认 1）
  QWEN_VLLM_RESTART_POLICY                  docker --restart 策略（默认 on-failure:3）
  QWEN_VLLM_WAIT_HEALTHY                    start 是否阻塞等就绪（默认 0=否）
  QWEN_VLLM_DEBUG                           调试日志（1 开启）

示例:
  ./${SCRIPT_NAME} preflight
  ./${SCRIPT_NAME} start                      # 启动全部 2 个实例（不再阻塞等就绪）
  ./${SCRIPT_NAME} start --dry-run            # 仅打印 docker 命令
  ./${SCRIPT_NAME} start 0                    # 仅启动实例 A
  ./${SCRIPT_NAME} wait-healthy               # 阻塞等所有实例 vLLM 就绪
  ./${SCRIPT_NAME} stop 1                     # 仅停止实例 B
  ./${SCRIPT_NAME} health
  ./${SCRIPT_NAME} watch
  ./${SCRIPT_NAME} bench --concurrent 8 --max-tokens 512
USAGE_EOF
}

version() {
  echo "${SCRIPT_NAME} ${SCRIPT_VERSION}  (hostname=${HOSTNAME_SHORT}, date=${TODAY_TAG})"
}

list_instances() {
  resolve_docker_cmd
  detect_cpu_topology
  detect_memory
  detect_gpu_topology
  detect_numa_topology
  load_instance_config
  compute_all_layouts
  local i
  for (( i=0; i<INST_COUNT; i++ )); do
    printf '实例 %d: %-30s gpus=%-6s tp=%d port=%d model=%s  cpuset=%s/%s\n' \
      "$i" "${INST_NAME[$i]}" "${INST_GPUS[$i]}" "${INST_TP[$i]}" \
      "${INST_PORT[$i]}" "${INST_MODEL_NAME[$i]}" \
      "${INST_CPUSET_CPUS[$i]}" "${INST_CPUSET_MEMS[$i]}"
  done
}

# 主入口
cmd="${1:-status}"
shift || true

case "$cmd" in
  start)
    target="all"
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --dry-run) dry_run=1; shift ;;
        --image)   IMAGE="$2"; shift 2 ;;
        all|[0-9]*) target="$1"; shift ;;
        -*)        shift ;;        # 忽略未知 flag
        *)         shift ;;        # 忽略位置参数后的其他参数
      esac
    done
    start_specific "$target"
    ;;
  stop)
    target="${1:-all}"
    stop_specific "$target"
    ;;
  restart)
    target="${1:-all}"
    restart_specific "$target"
    ;;
  reset)
    target="${1:-all}"
    reset_specific "$target"
    ;;
  status)    status_all ;;
  logs)
    target="${1:-all}"
    logs_for "$target"
    ;;
  health)
    target="${1:-all}"
    if [[ "$target" == "all" ]]; then
      health_all
    else
      resolve_docker_cmd
      load_instance_config
      local i found=0
      for (( i=0; i<INST_COUNT; i++ )); do
        if [[ "$target" == "$i" || "$target" == "${INST_NAME[$i]}" ]]; then
          health_for "$i"; found=1
        fi
      done
      (( found == 1 )) || { log_err "未找到实例: $target"; exit 1; }
    fi
    ;;
  watch)
    target="${1:-all}"
    watch_for "$target"
    ;;
  wait-healthy|wait)
    target="${1:-all}"
    wait_healthy_for "$target"
    ;;
  bench)
    target="${1:-all}"
    shift || true
    if [[ "$target" == "all" ]]; then
      bench_all "$@"
    else
      bench_specific "$target" "$@"
    fi
    ;;
  list)       list_instances ;;
  preflight)  resolve_docker_cmd; preflight ;;
  detect)     resolve_docker_cmd; detect_cpu_topology; detect_memory; detect_gpu_topology; detect_numa_topology ;;
  version|-V|--version) version ;;
  help|-h|--help) usage ;;
  *) log_err "未知命令: $cmd"; usage; exit 1 ;;
esac