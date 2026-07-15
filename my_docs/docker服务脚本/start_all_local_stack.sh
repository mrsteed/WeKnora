#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

: "${DOCKER_BIN:=docker}"

if ! command -v "$DOCKER_BIN" >/dev/null 2>&1; then
  echo "[缺] 找不到 docker 命令 (DOCKER_BIN=$DOCKER_BIN)" >&2
  echo "      请先执行 ./check_prereqs.sh 补齐前置环境。" >&2
  exit 1
fi

"$SCRIPT_DIR/start_model_services.sh"
"$SCRIPT_DIR/start_weknora_stack.sh"
