#!/usr/bin/env bash
set -euo pipefail
#
# WeKnora 生产启动入口（默认带 --build：本地缺镜像才构建，已存在则直接 up）。
# 用法:
#   ./start.sh               # build(需则) + up 核心集 + 健康验收
#   ./start.sh --no-build    # 跳过构建直接 up
#   ./start.sh --help        # 看 compose.sh 全量命令
#
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
exec bash "${SCRIPT_DIR}/compose.sh" up --build "$@"
