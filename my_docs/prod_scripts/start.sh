#!/usr/bin/env bash
set -euo pipefail
#
# WeKnora 生产启动入口（对齐 make dev-start 逻辑）
#   - 默认 profile: minio + langfuse（可用 --no-minio / --no-langfuse 关闭）
#   - 核心集: postgres / redis / docreader / app（frontend(nginx) 已改宿主机自管，不入栈）
#   - 官方镜像（docreader/minio/langfuse/clickhouse/paradedb/redis...）自动预拉
#   - minio 官方仓库 minio/minio 已从 Docker Hub 下架：官方 pull 失败时自动改用
#     gowah/minio:RELEASE.2025-04-22T22-12-26Z 转推镜像并 retag（见 compose.sh MINIO_MIRROR_*）
#   - 幂等: 已起容器不重建，仅缺镜像时按 build: 定义自构（frontend 不再构建，
#           前端包与 nginx 配置都不进镜像，由宿主机自行编译部署）
# 用法:
#   ./start.sh                       # 启动核心集 + minio + langfuse，自动预拉官方镜像
#   ./start.sh --build               # 先本地重建 app 镜像并 force-recreate app 容器（发版；
#                                    #   frontend 由宿主机自管；等价于 start + rebuild 合一）
#   ./start.sh --no-langfuse         # 关 langfuse
#   ./start.sh --no-minio --no-langfuse  # 只起 CORE_SERVICES（最小化）
#   ./start.sh --help                # compose.sh 全量命令
#
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
exec bash "${SCRIPT_DIR}/compose.sh" up "$@"
