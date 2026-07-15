#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
WEKNORA_DIR="$WORKSPACE_DIR/WeKnora"
WEKNORA_STACK_DATA_ROOT="${WEKNORA_STACK_DATA_ROOT:-/data/weknora}"

: "${DOCKER_BIN:=docker}"
docker_cmd=("$DOCKER_BIN")

if ! command -v "$DOCKER_BIN" >/dev/null 2>&1; then
  echo "[缺] 找不到 docker 命令 (DOCKER_BIN=$DOCKER_BIN)" >&2
  echo "      请先执行 ./check_prereqs.sh 补齐前置环境。" >&2
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

export WEKNORA_STACK_DATA_ROOT

"${docker_cmd[@]}" compose -f "$WEKNORA_DIR/docker-compose.yml" --profile minio --profile neo4j down
