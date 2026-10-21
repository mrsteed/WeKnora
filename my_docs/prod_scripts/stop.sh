#!/usr/bin/env bash
set -euo pipefail
# WeKnora 生产停止入口: 数据卷保留。危险选项透传: ./stop.sh --purge-data
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
exec bash "${SCRIPT_DIR}/compose.sh" down "$@"
