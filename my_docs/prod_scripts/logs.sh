#!/usr/bin/env bash
set -euo pipefail
# WeKnora 生产日志入口: 默认跟踪 app docreader postgres
#   ./logs.sh                 # 跟踪三核心服务
#   ./logs.sh app -n          # 快照 app（不跟随）
#   ./logs.sh --tail=500 app
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
exec bash "${SCRIPT_DIR}/compose.sh" logs "$@"
