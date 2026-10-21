#!/usr/bin/env bash
set -euo pipefail
# WeKnora 生产状态入口: compose ps + app /health + frontend HTTP 探测
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
exec bash "${SCRIPT_DIR}/compose.sh" status "$@"
