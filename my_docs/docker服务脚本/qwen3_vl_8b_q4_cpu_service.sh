#!/usr/bin/env bash
set -euo pipefail
shopt -s inherit_errexit 2>/dev/null || true
IFS=$'\n\t'

SCRIPT_PATH="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "$(dirname "${SCRIPT_PATH}")" && pwd)"
SCRIPT_NAME="$(basename "${SCRIPT_PATH}")"
SCRIPT_VERSION="1.0.0"
HOSTNAME_SHORT="$(hostname -s 2>/dev/null || echo localhost)"
TODAY_TAG="$(date +%Y%m%d)"

LOG_DIR="${QWEN3_VL_CPU_SCRIPT_LOG_DIR:-${SCRIPT_DIR}/.logs}"
mkdir -p "${LOG_DIR}"
LOG_FILE="${LOG_DIR}/${SCRIPT_NAME%.sh}-${HOSTNAME_SHORT}-${TODAY_TAG}.log"

if [[ -t 1 ]]; then
  C_RED=$'\033[31m'; C_YEL=$'\033[33m'; C_GRN=$'\033[32m'
  C_BLU=$'\033[34m'; C_CYN=$'\033[36m'; C_DIM=$'\033[2m'; C_RST=$'\033[0m'
else
  C_RED=''; C_YEL=''; C_GRN=''; C_BLU=''; C_CYN=''; C_DIM=''; C_RST=''
fi

_ts() { date '+%Y-%m-%dT%H:%M:%S%z'; }
_log_to_file() {
  local level="$1"; shift
  printf '%s [%s] [%s] %s\n' "$(_ts)" "$level" "$HOSTNAME_SHORT" "$*" >> "${LOG_FILE}" 2>/dev/null || true
}

log_info()  { echo "${C_BLU}[INFO ]${C_RST}  $*" >&2; _log_to_file INFO "$*"; }
log_ok()    { echo "${C_GRN}[OK   ]${C_RST}  $*" >&2; _log_to_file OK "$*"; }
log_warn()  { echo "${C_YEL}[WARN ]${C_RST}  $*" >&2; _log_to_file WARN "$*"; }
log_err()   { echo "${C_RED}[ERROR]${C_RST}  $*" >&2; _log_to_file ERROR "$*"; }
log_inst()  { local idx="$1"; shift; echo "${C_CYN}[INST${idx}]${C_RST} $*" >&2; _log_to_file "INST${idx}" "$*"; }

have() { command -v "$1" >/dev/null 2>&1; }
trim() {
  local s="${1:-}"
  s="${s#"${s%%[![:space:]]*}"}"
  s="${s%"${s##*[![:space:]]}"}"
  printf '%s' "$s"
}
die() { log_err "$*"; exit 1; }

resolve_docker_cmd() {
  : "${DOCKER_BIN:=docker}"
  docker_cmd=("${DOCKER_BIN}")

  have "${DOCKER_BIN}" || die "找不到 docker 命令 (DOCKER_BIN=${DOCKER_BIN})"

  local docker_info_err
  docker_info_err="$(${DOCKER_BIN} info 2>&1 >/dev/null || true)"
  if [[ -n "${docker_info_err}" ]]; then
    if [[ "${docker_info_err}" == *"permission denied"* || "${docker_info_err}" == *"Got permission denied"* || "${docker_info_err}" == *"docker.sock"* ]]; then
      have sudo || die "当前用户无权访问 docker.sock，且系统未安装 sudo"
      docker_cmd=(sudo "${DOCKER_BIN}")
      "${docker_cmd[@]}" info >/dev/null 2>&1 || die "sudo docker 仍无法访问 docker daemon"
    else
      [[ "${docker_info_err}" == *"Cannot connect to the Docker daemon"* || "${docker_info_err}" == *"Is the docker daemon running"* ]] && die "docker daemon 未运行，请先启动 docker"
      die "docker 不可用: ${docker_info_err}"
    fi
  fi
}

spaces_to_csv() {
  local text="${1:-}"
  awk '{for (i=1; i<=NF; i++) print $i}' <<< "${text}" | paste -sd, -
}

compress_cpuset() {
  local raw="${1:-}"
  [[ -z "${raw}" ]] && return 0

  local -a nums=()
  local token
  while IFS= read -r token; do
    [[ -n "${token}" ]] && nums+=("${token}")
  done < <(tr ', ' '\n\n' <<< "${raw}" | awk 'NF {print $1}' | sort -nu)

  [[ ${#nums[@]} -eq 0 ]] && return 0

  local -a ranges=()
  local start="${nums[0]}" prev="${nums[0]}" cur
  for cur in "${nums[@]:1}"; do
    if (( cur == prev + 1 )); then
      prev="${cur}"
      continue
    fi
    if (( start == prev )); then
      ranges+=("${start}")
    else
      ranges+=("${start}-${prev}")
    fi
    start="${cur}"
    prev="${cur}"
  done
  if (( start == prev )); then
    ranges+=("${start}")
  else
    ranges+=("${start}-${prev}")
  fi
  local joined=''
  local part
  for part in "${ranges[@]}"; do
    joined+="${joined:+,}${part}"
  done
  printf '%s' "${joined}"
}

merge_cpusets() {
  local merged=''
  local part
  for part in "$@"; do
    [[ -n "${part}" ]] && merged+="${merged:+,}${part}"
  done
  compress_cpuset "${merged}"
}

count_cpuset_items() {
  local spec="${1:-}"
  [[ -z "${spec}" ]] && { echo 0; return; }
  local count=0 part start end
  IFS=',' read -r -a __parts <<< "${spec}"
  for part in "${__parts[@]}"; do
    if [[ "${part}" == *-* ]]; then
      start="${part%-*}"
      end="${part#*-}"
      count=$(( count + end - start + 1 ))
    else
      count=$(( count + 1 ))
    fi
  done
  echo "${count}"
}

split_cpu_list_evenly() {
  local idx="$1"
  local total="${HW_TOTAL_THREADS:-$(nproc)}"
  local half=$(( total / 2 ))
  local start=0 end=$(( half - 1 ))
  if (( idx == 1 )); then
    start="${half}"
    end=$(( total - 1 ))
  fi
  compress_cpuset "${start}-${end}"
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
    ss -ltn 2>/dev/null | awk '{print $4}' | grep -Eq "(^|:)${port}$"
    return
  fi
  if have lsof; then
    lsof -iTCP:"${port}" -sTCP:LISTEN >/dev/null 2>&1
    return
  fi
  return 1
}

suffix_for_instance() {
  local idx="$1"
  printf "\\$(printf '%03o' $((97 + idx)))"
}

IMAGE="${QWEN3_VL_CPU_IMAGE:-ghcr.io/ggml-org/llama.cpp:server}"
INSTANCE_COUNT="${QWEN3_VL_CPU_INSTANCES:-2}"
CONTAINER_PORT="${QWEN3_VL_CPU_CONTAINER_PORT:-8080}"
BASE_PORT="${QWEN3_VL_CPU_BASE_PORT:-18080}"
HF_REPO="${QWEN3_VL_CPU_HF_REPO:-Qwen/Qwen3-VL-8B-Instruct-GGUF}"
HF_FILE="${QWEN3_VL_CPU_HF_FILE:-Qwen3VL-8B-Instruct-Q4_K_M.gguf}"
MMPROJ_URL="${QWEN3_VL_CPU_MMPROJ_URL:-https://huggingface.co/Qwen/Qwen3-VL-8B-Instruct-GGUF/resolve/main/mmproj-Qwen3VL-8B-Instruct-F16.gguf}"
ALIAS_BASE="${QWEN3_VL_CPU_ALIAS_BASE:-qwen3-vl-8b-q4-cpu}"
CACHE_ROOT="${QWEN3_VL_CPU_CACHE_ROOT:-/data/models/hf-cache/qwen3-vl-8b-q4-cpu}"
INSTANCE_LOG_ROOT="${QWEN3_VL_CPU_LOG_ROOT:-/data/weknora/model-logs/qwen3-vl-8b-q4-cpu}"
CTX_SIZE="${QWEN3_VL_CPU_CTX_SIZE:-4096}"
PARALLEL="${QWEN3_VL_CPU_PARALLEL:-1}"
IMAGE_MIN_TOKENS="${QWEN3_VL_CPU_IMAGE_MIN_TOKENS:-256}"
IMAGE_MAX_TOKENS="${QWEN3_VL_CPU_IMAGE_MAX_TOKENS:-4096}"
STOP_TIMEOUT="${QWEN3_VL_CPU_STOP_TIMEOUT:-45}"
DOCKER_RESTART_POLICY="${QWEN3_VL_CPU_RESTART_POLICY:-unless-stopped}"
HEALTH_PATH="${QWEN3_VL_CPU_HEALTH_PATH:-/health}"
dry_run=0

HW_THREADS_PER_CORE=1
HW_TOTAL_THREADS=0
HW_NUMA_NODES=0
HW_MEM_TOTAL_GB=0
HW_MEM_AVAIL_GB=0
HW_NUMA_CPUS=()

INST_NAME=()
INST_ALIAS=()
INST_PORT=()
INST_CPUSET_CPUS=()
INST_CPUSET_MEMS=()
INST_THREADS=()

detect_cpu_topology() {
  log_info "检测 CPU / NUMA 拓扑 ..."
  if have lscpu; then
    HW_THREADS_PER_CORE="$(lscpu | awk -F: '/^Thread\(s\) per core/{gsub(/ /, "", $2); print $2; exit}')"
    HW_NUMA_NODES="$(lscpu | awk -F: '/^NUMA node\(s\)/{gsub(/ /, "", $2); print $2; exit}')"
  fi
  [[ -z "${HW_THREADS_PER_CORE}" ]] && HW_THREADS_PER_CORE=1
  [[ -z "${HW_NUMA_NODES}" ]] && HW_NUMA_NODES=1
  HW_TOTAL_THREADS="$(nproc)"
}

detect_memory() {
  if [[ -r /proc/meminfo ]]; then
    HW_MEM_TOTAL_GB="$(awk '/^MemTotal:/ {printf "%d", $2/1024/1024}' /proc/meminfo)"
    HW_MEM_AVAIL_GB="$(awk '/^MemAvailable:/ {printf "%d", $2/1024/1024}' /proc/meminfo)"
  fi
  log_info "内存: total=${HW_MEM_TOTAL_GB}GiB avail=${HW_MEM_AVAIL_GB}GiB"
}

detect_numa_topology() {
  HW_NUMA_CPUS=()
  local i cpu_list
  for (( i=0; i<HW_NUMA_NODES; i++ )); do
    cpu_list=''
    if have numactl; then
      cpu_list="$(numactl --hardware 2>/dev/null | awk -v n="${i}" '$0 ~ "^node " n " cpus:" {sub(/^node [0-9]+ cpus:[ \t]*/, ""); print; exit}')"
    fi
    if [[ -z "${cpu_list}" ]] && have lscpu; then
      cpu_list="$(lscpu -p=cpu,node 2>/dev/null | awk -F',' -v n="${i}" '!/^#/ && $2 == n {print $1}' | paste -sd, -)"
    fi
    HW_NUMA_CPUS[i]="$(compress_cpuset "$(spaces_to_csv "${cpu_list}")")"
  done
}

default_binding_for_instance() {
  local idx="$1"
  local cpus mems
  if (( HW_NUMA_NODES >= 4 )); then
    if (( idx == 0 )); then
      mems='0,1'
      cpus="$(merge_cpusets "${HW_NUMA_CPUS[0]:-}" "${HW_NUMA_CPUS[1]:-}")"
    else
      mems='2,3'
      cpus="$(merge_cpusets "${HW_NUMA_CPUS[2]:-}" "${HW_NUMA_CPUS[3]:-}")"
    fi
  elif (( HW_NUMA_NODES == 3 )); then
    if (( idx == 0 )); then
      mems='0,1'
      cpus="$(merge_cpusets "${HW_NUMA_CPUS[0]:-}" "${HW_NUMA_CPUS[1]:-}")"
    else
      mems='2'
      cpus="$(merge_cpusets "${HW_NUMA_CPUS[2]:-}")"
    fi
  elif (( HW_NUMA_NODES == 2 )); then
    mems="${idx}"
    cpus="$(merge_cpusets "${HW_NUMA_CPUS[$idx]:-}")"
  else
    mems='0'
    cpus="$(split_cpu_list_evenly "${idx}")"
  fi
  printf '%s|%s\n' "${cpus}" "${mems}"
}

cache_dir_for() { printf '%s/inst%s' "${CACHE_ROOT}" "$1"; }
instance_log_dir_for() { printf '%s/inst%s' "${INSTANCE_LOG_ROOT}" "$1"; }

ensure_instance_dirs() {
  local idx="$1"
  mkdir -p "$(cache_dir_for "${idx}")" "$(instance_log_dir_for "${idx}")"
}

prepare_instance_dirs() {
  local idx
  for (( idx=0; idx<INSTANCE_COUNT; idx++ )); do
    ensure_instance_dirs "${idx}"
  done
}

load_instance_config() {
  INST_NAME=(); INST_ALIAS=(); INST_PORT=(); INST_CPUSET_CPUS=(); INST_CPUSET_MEMS=(); INST_THREADS=()
  local idx suffix default_binding cpus mems cpu_count default_threads
  for (( idx=0; idx<INSTANCE_COUNT; idx++ )); do
    suffix="$(suffix_for_instance "${idx}")"
    default_binding="$(default_binding_for_instance "${idx}")"
    cpus="${default_binding%%|*}"
    mems="${default_binding##*|}"

    local name_var="QWEN3_VL_CPU_INSTANCE_${idx}_NAME"
    local alias_var="QWEN3_VL_CPU_INSTANCE_${idx}_ALIAS"
    local port_var="QWEN3_VL_CPU_INSTANCE_${idx}_PORT"
    local cpus_var="QWEN3_VL_CPU_INSTANCE_${idx}_CPUSET_CPUS"
    local mems_var="QWEN3_VL_CPU_INSTANCE_${idx}_CPUSET_MEMS"
    local threads_var="QWEN3_VL_CPU_INSTANCE_${idx}_THREADS"

    cpus="${!cpus_var:-${cpus}}"
    mems="${!mems_var:-${mems}}"
    cpus="$(compress_cpuset "${cpus}")"

    cpu_count="$(count_cpuset_items "${cpus}")"
    default_threads=$(( cpu_count / HW_THREADS_PER_CORE ))
    (( default_threads < 1 )) && default_threads="${cpu_count}"

    INST_NAME[idx]="${!name_var:-weknora-qwen3-vl-8b-q4-cpu-${suffix}}"
    INST_ALIAS[idx]="${!alias_var:-${ALIAS_BASE}-${suffix}}"
    INST_PORT[idx]="${!port_var:-$(( BASE_PORT + idx ))}"
    INST_CPUSET_CPUS[idx]="${cpus}"
    INST_CPUSET_MEMS[idx]="${mems}"
    INST_THREADS[idx]="${!threads_var:-${default_threads}}"
  done
}

target_matches() {
  local target="$1" idx="$2"
  [[ "${target}" == "all" || "${target}" == "${idx}" || "${target}" == "${INST_NAME[$idx]}" || "${target}" == "${INST_ALIAS[$idx]}" ]]
}

preflight() {
  detect_cpu_topology
  detect_memory
  detect_numa_topology
  load_instance_config
  local idx
  log_info "llama.cpp CPU 镜像: ${IMAGE}"
  log_info "实例数: ${INSTANCE_COUNT}"
  for (( idx=0; idx<INSTANCE_COUNT; idx++ )); do
    log_info "实例 ${idx}: name=${INST_NAME[$idx]} alias=${INST_ALIAS[$idx]} port=${INST_PORT[$idx]} cpus=${INST_CPUSET_CPUS[$idx]} mems=${INST_CPUSET_MEMS[$idx]} threads=${INST_THREADS[$idx]}"
  done
}

check_ports_available() {
  local target="$1"
  local idx port
  for (( idx=0; idx<INSTANCE_COUNT; idx++ )); do
    target_matches "${target}" "${idx}" || continue
    container_running_for "${INST_NAME[$idx]}" && continue
    port="${INST_PORT[$idx]}"
    if port_in_use "${port}"; then
      die "宿主机端口 ${port} 已被占用，无法启动 ${INST_NAME[$idx]}"
    fi
  done
}

print_cmd() {
  printf '%q ' "$@"
  echo
}

run_container_for_instance() {
  local idx="$1"
  local name="${INST_NAME[$idx]}"
  local port="${INST_PORT[$idx]}"
  local cpus="${INST_CPUSET_CPUS[$idx]}"
  local mems="${INST_CPUSET_MEMS[$idx]}"
  local threads="${INST_THREADS[$idx]}"
  local cache_dir
  cache_dir="$(cache_dir_for "${idx}")"

  local -a cmd=(
    "${docker_cmd[@]}" run -d
    --name "${name}"
    --hostname "${name}"
    --restart "${DOCKER_RESTART_POLICY}"
    --label weknora.model.cpu=qwen3-vl-8b-q4
    --label "weknora.instance.index=${idx}"
    --cpuset-cpus "${cpus}"
    --cpuset-mems "${mems}"
    -p "${port}:${CONTAINER_PORT}"
    -v "${cache_dir}:/root/.cache"
    "${IMAGE}"
    --hf-repo "${HF_REPO}"
    --hf-file "${HF_FILE}"
    --mmproj-url "${MMPROJ_URL}"
    --alias "${INST_ALIAS[$idx]}"
    --host 0.0.0.0
    --port "${CONTAINER_PORT}"
    --ctx-size "${CTX_SIZE}"
    --threads "${threads}"
    --parallel "${PARALLEL}"
    --cont-batching
    --image-min-tokens "${IMAGE_MIN_TOKENS}"
    --image-max-tokens "${IMAGE_MAX_TOKENS}"
  )

  if (( dry_run == 1 )); then
    print_cmd "${cmd[@]}"
    return 0
  fi

  "${cmd[@]}" >/dev/null
}

start_instance() {
  local idx="$1"
  local name="${INST_NAME[$idx]}"
  if container_running_for "${name}"; then
    log_inst "${idx}" "${name} 已在运行"
    return 0
  fi
  if container_exists_for "${name}"; then
    log_inst "${idx}" "删除旧容器 ${name}"
    "${docker_cmd[@]}" rm -f "${name}" >/dev/null 2>&1 || true
  fi
  run_container_for_instance "${idx}"
  log_inst "${idx}" "已启动 ${name}，地址 http://127.0.0.1:${INST_PORT[$idx]}${HEALTH_PATH}"
  log_inst "${idx}" "首次模型下载与 warmup 可能需要数分钟；可执行 ${SCRIPT_NAME} logs ${idx} 查看进度"
}

start_specific() {
  local target="$1"
  resolve_docker_cmd
  preflight
  prepare_instance_dirs
  check_ports_available "${target}"
  local idx found=0
  for (( idx=0; idx<INSTANCE_COUNT; idx++ )); do
    target_matches "${target}" "${idx}" || continue
    found=1
    start_instance "${idx}"
  done
  (( found == 1 )) || die "未找到实例: ${target}"
}

stop_instance() {
  local idx="$1"
  local name="${INST_NAME[$idx]}"
  if ! container_exists_for "${name}"; then
    log_inst "${idx}" "${name} 不存在"
    return 0
  fi
  log_inst "${idx}" "优雅停止 ${name}"
  "${docker_cmd[@]}" stop -t "${STOP_TIMEOUT}" "${name}" >/dev/null 2>&1 || true
  "${docker_cmd[@]}" rm -f "${name}" >/dev/null 2>&1 || true
}

stop_specific() {
  local target="$1"
  resolve_docker_cmd
  detect_cpu_topology
  detect_numa_topology
  load_instance_config
  local idx found=0
  for (( idx=0; idx<INSTANCE_COUNT; idx++ )); do
    target_matches "${target}" "${idx}" || continue
    found=1
    stop_instance "${idx}"
  done
  (( found == 1 )) || die "未找到实例: ${target}"
}

restart_specific() {
  local target="$1"
  stop_specific "${target}"
  start_specific "${target}"
}

reset_specific() {
  local target="$1"
  resolve_docker_cmd
  preflight
  prepare_instance_dirs

  local -a running_before=()
  local idx
  for (( idx=0; idx<INSTANCE_COUNT; idx++ )); do
    if container_running_for "${INST_NAME[$idx]}"; then
      running_before[idx]=1
    else
      running_before[idx]=0
    fi
  done

  if [[ "${target}" != "all" ]]; then
    log_warn "两个实例共享镜像 ${IMAGE}；reset 单实例时会短暂停止本服务全部实例以删除旧镜像"
  fi

  for (( idx=0; idx<INSTANCE_COUNT; idx++ )); do
    container_exists_for "${INST_NAME[$idx]}" && stop_instance "${idx}"
  done

  for (( idx=0; idx<INSTANCE_COUNT; idx++ )); do
    target_matches "${target}" "${idx}" || continue
    rm -rf "$(cache_dir_for "${idx}")"
    ensure_instance_dirs "${idx}"
  done

  if "${docker_cmd[@]}" image inspect "${IMAGE}" >/dev/null 2>&1; then
    log_info "删除镜像 ${IMAGE}"
    "${docker_cmd[@]}" image rm -f "${IMAGE}" >/dev/null 2>&1 || true
  fi

  if (( dry_run == 1 )); then
    print_cmd "${docker_cmd[@]}" pull "${IMAGE}"
    return 0
  fi

  log_info "重新拉取镜像 ${IMAGE}"
  "${docker_cmd[@]}" pull "${IMAGE}" >/dev/null

  for (( idx=0; idx<INSTANCE_COUNT; idx++ )); do
    if target_matches "${target}" "${idx}" || [[ "${running_before[$idx]}" == "1" ]]; then
      start_instance "${idx}"
    fi
  done
}

instance_status() {
  local idx="$1"
  local name="${INST_NAME[$idx]}"
  if ! container_exists_for "${name}"; then
    printf 'missing\t-\n'
    return 0
  fi
  local status health
  status="$("${docker_cmd[@]}" inspect -f '{{.State.Status}}' "${name}" 2>/dev/null || echo unknown)"
  health="$("${docker_cmd[@]}" inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}n/a{{end}}' "${name}" 2>/dev/null || echo n/a)"
  printf '%s\t%s\n' "${status}" "${health}"
}

status_all() {
  resolve_docker_cmd
  preflight
  echo "===== 实例状态 ====="
  printf '%-3s %-36s %-8s %-8s %-12s %-8s %-18s\n' 'IDX' 'NAME' 'PORT' 'THREADS' 'STATUS' 'HEALTH' 'CPUS/MEMS'
  echo "-----------------------------------------------------------------------------------------------------------"
  local idx status health
  for (( idx=0; idx<INSTANCE_COUNT; idx++ )); do
    IFS=$'\t' read -r status health < <(instance_status "${idx}")
    printf '%-3s %-36s %-8s %-8s %-12s %-8s %-18s\n' \
      "${idx}" "${INST_NAME[$idx]}" "${INST_PORT[$idx]}" "${INST_THREADS[$idx]}" "${status}" "${health}" "${INST_CPUSET_CPUS[$idx]}/${INST_CPUSET_MEMS[$idx]}"
  done
  echo
  echo "===== NUMA 拓扑 ====="
  for (( idx=0; idx<HW_NUMA_NODES; idx++ )); do
    printf 'NUMA%-2s cpus=%s\n' "${idx}" "${HW_NUMA_CPUS[$idx]:-(empty)}"
  done
}

logs_for() {
  local target="$1"
  resolve_docker_cmd
  detect_cpu_topology
  detect_numa_topology
  load_instance_config

  trap 'jobs -pr | xargs -r kill 2>/dev/null || true' INT TERM EXIT
  local idx found=0
  for (( idx=0; idx<INSTANCE_COUNT; idx++ )); do
    target_matches "${target}" "${idx}" || continue
    found=1
    if container_exists_for "${INST_NAME[$idx]}"; then
      "${docker_cmd[@]}" logs --tail "${QWEN3_VL_CPU_LOG_TAIL:-200}" -f "${INST_NAME[$idx]}" 2>&1 | sed -u "s/^/[${INST_NAME[$idx]}] /" &
    else
      log_inst "${idx}" "${INST_NAME[$idx]} 不存在"
    fi
  done
  (( found == 1 )) || die "未找到实例: ${target}"
  wait
}

health_for() {
  local idx="$1"
  local name="${INST_NAME[$idx]}"
  local url="http://127.0.0.1:${INST_PORT[$idx]}${HEALTH_PATH}"
  if ! container_running_for "${name}"; then
    log_inst "${idx}" "容器未运行"
    return 1
  fi
  if curl -fsS "${url}" >/dev/null 2>&1; then
    log_inst "${idx}" "健康检查通过: ${url}"
    return 0
  fi
  log_inst "${idx}" "健康检查失败: ${url}"
  return 1
}

health_specific() {
  local target="$1"
  resolve_docker_cmd
  detect_cpu_topology
  detect_numa_topology
  load_instance_config
  local idx found=0 rc=0
  for (( idx=0; idx<INSTANCE_COUNT; idx++ )); do
    target_matches "${target}" "${idx}" || continue
    found=1
    health_for "${idx}" || rc=1
  done
  (( found == 1 )) || die "未找到实例: ${target}"
  return "${rc}"
}

usage() {
  cat <<USAGE_EOF
用法: ./${SCRIPT_NAME} <command> [target]

命令:
  start [target]       预检 + 启动实例（target 可省，默认 all；可填 0/1 或实例名）
  stop [target]        优雅停止实例
  restart [target]     stop + start
  reset [target]       删除容器/镜像 + 清理缓存 + 重拉 + 重启
  status               所有实例状态 + NUMA 拓扑
  logs [target]        跟踪日志（多实例时并行输出，Ctrl+C 全部退出）
  health [target]      单次健康检查
  preflight            仅运行预检
  version | -V         打印脚本版本
  help | -h | --help   打印帮助

默认实例:
  0 -> weknora-qwen3-vl-8b-q4-cpu-a  port=18080
  1 -> weknora-qwen3-vl-8b-q4-cpu-b  port=18081

关键环境变量:
  QWEN3_VL_CPU_IMAGE                  默认 ghcr.io/ggml-org/llama.cpp:server
  QWEN3_VL_CPU_BASE_PORT              默认 18080
  QWEN3_VL_CPU_CTX_SIZE               默认 4096
  QWEN3_VL_CPU_PARALLEL               默认 1
  QWEN3_VL_CPU_CACHE_ROOT             默认 /data/models/hf-cache/qwen3-vl-8b-q4-cpu
  QWEN3_VL_CPU_INSTANCE_<idx>_PORT    单实例宿主机端口覆盖
  QWEN3_VL_CPU_INSTANCE_<idx>_CPUSET_CPUS / _CPUSET_MEMS / _THREADS

示例:
  ./${SCRIPT_NAME} preflight
  ./${SCRIPT_NAME} start
  ./${SCRIPT_NAME} start 0
  ./${SCRIPT_NAME} logs 1
  ./${SCRIPT_NAME} health all
  ./${SCRIPT_NAME} reset weknora-qwen3-vl-8b-q4-cpu-a
USAGE_EOF
}

version() {
  echo "${SCRIPT_NAME} ${SCRIPT_VERSION} (hostname=${HOSTNAME_SHORT}, date=${TODAY_TAG})"
}

cmd="${1:-status}"
shift || true

case "${cmd}" in
  start)
    target="all"
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --dry-run) dry_run=1; shift ;;
        all|0|1) target="$1"; shift ;;
        *) target="$1"; shift ;;
      esac
    done
    start_specific "${target}"
    ;;
  stop)
    stop_specific "${1:-all}"
    ;;
  restart)
    restart_specific "${1:-all}"
    ;;
  reset)
    reset_specific "${1:-all}"
    ;;
  status)
    status_all
    ;;
  logs)
    logs_for "${1:-all}"
    ;;
  health)
    health_specific "${1:-all}"
    ;;
  preflight)
    resolve_docker_cmd
    preflight
    ;;
  version|-V|--version)
    version
    ;;
  help|-h|--help)
    usage
    ;;
  *)
    die "未知命令: ${cmd}"
    ;;
esac