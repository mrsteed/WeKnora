#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.local-ai-models.yml"
ALL_SERVICES=(qwen-vllm infinity-api qwen3-vl-llamacpp speaches-asr)

usage() {
  cat <<'EOF'
用法:
  ./start_model_services.sh                启动全部模型服务
  ./start_model_services.sh <service...>   只启动指定服务

可用服务:
  qwen-vllm
  infinity-api
  infinity-api-cpu
  qwen3-vl-llamacpp
  speaches-asr
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

targets=("${ALL_SERVICES[@]}")
if [[ $# -gt 0 ]]; then
  targets=("$@")
fi

for target in "${targets[@]}"; do
  case "$target" in
    qwen-vllm|infinity-api|infinity-api-cpu|qwen3-vl-llamacpp|speaches-asr)
      ;;
    *)
      echo "未知服务: $target" >&2
      usage >&2
      exit 1
      ;;
  esac
done

: "${DOCKER_BIN:=docker}"
docker_cmd=("$DOCKER_BIN")

if ! command -v "$DOCKER_BIN" >/dev/null 2>&1; then
  echo "[缺] 找不到 docker 命令 (DOCKER_BIN=$DOCKER_BIN)" >&2
  echo "      请先执行 ./check_prereqs.sh 补齐前置环境，或设置 DOCKER_BIN 指向 docker 可执行文件。" >&2
  exit 1
fi

docker_info_err="$("$DOCKER_BIN" info 2>&1 >/dev/null || true)"
if [[ -n "$docker_info_err" ]]; then
  if [[ "$docker_info_err" == *"permission denied"* || "$docker_info_err" == *"Got permission denied"* || "$docker_info_err" == *"docker.sock"* ]]; then
    if ! command -v sudo >/dev/null 2>&1; then
      echo "[缺] 当前用户无权访问 docker.sock，且系统未安装 sudo。" >&2
      echo "      请将当前用户加入 docker 组，或以 root 身份执行。" >&2
      exit 1
    fi

    if ! sudo -n true >/dev/null 2>&1; then
      echo "[提示] 当前用户无权访问 docker.sock，将切换为 sudo docker，可能会提示输入 sudo 密码。" >&2
    fi

    docker_cmd=(sudo "$DOCKER_BIN")
    if ! "${docker_cmd[@]}" info >/dev/null 2>&1; then
      echo "[缺] sudo docker 仍无法访问 docker daemon，请确认 docker 服务已启动且 sudo 可用。" >&2
      exit 1
    fi
  else
    echo "$docker_info_err" | grep -qi 'Cannot connect to the Docker daemon\|Is the docker daemon running' && {
      echo "[缺] docker daemon 未运行，请先执行 sudo systemctl start docker" >&2
      exit 1
    }

    echo "[缺] docker 不可用: $docker_info_err" >&2
    exit 1
  fi
fi

needs_qwen=0
needs_infinity_gpu=0
needs_infinity_cpu=0
non_qwen_targets=()
for target in "${targets[@]}"; do
  case "$target" in
    qwen-vllm)
      needs_qwen=1
      ;;
    infinity-api)
      needs_infinity_gpu=1
      ;;
    infinity-api-cpu)
      needs_infinity_cpu=1
      ;;
    *)
      non_qwen_targets+=("$target")
      ;;
  esac
done

if [[ $needs_infinity_gpu -eq 1 && $needs_infinity_cpu -eq 1 ]]; then
  echo "不能同时启动 infinity-api 和 infinity-api-cpu，它们会冲突占用 7997 端口。" >&2
  exit 1
fi

if [[ $needs_qwen -eq 1 ]]; then
  "$SCRIPT_DIR/qwen_vllm_service.sh" start
fi

if [[ $needs_infinity_gpu -eq 1 ]]; then
  "${docker_cmd[@]}" compose -f "$COMPOSE_FILE" stop infinity-api-cpu >/dev/null 2>&1 || true
  "${docker_cmd[@]}" compose -f "$COMPOSE_FILE" rm -f infinity-api-cpu >/dev/null 2>&1 || true
  non_qwen_targets=("infinity-api" "${non_qwen_targets[@]}")
fi

if [[ $needs_infinity_cpu -eq 1 ]]; then
  "${docker_cmd[@]}" compose -f "$COMPOSE_FILE" stop infinity-api >/dev/null 2>&1 || true
  "${docker_cmd[@]}" compose -f "$COMPOSE_FILE" rm -f infinity-api >/dev/null 2>&1 || true
  non_qwen_targets=("infinity-api-cpu" "${non_qwen_targets[@]}")
fi

if [[ ${#non_qwen_targets[@]} -gt 0 ]]; then
  "${docker_cmd[@]}" compose -f "$COMPOSE_FILE" up -d "${non_qwen_targets[@]}"
fi
