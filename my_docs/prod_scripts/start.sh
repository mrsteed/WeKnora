#!/usr/bin/env bash
set -euo pipefail
#
# WeKnora 生产启动入口
#   - 默认 profile: minio + searxng（可用 --no-minio / --no-searxng 关闭）
#   - langfuse 不再默认启动（2026-10-22 起，与 0.8.3 官方 dev.sh 一致）；
#     需要时: compose.sh profile-up langfuse
#   - sandbox 镜像（无常驻容器）默认就位：pull 官方镜像优先，失败回退本地构建
#   - 核心集: postgres / redis / docreader / app（frontend(nginx) 已改宿主机自管，不入栈）
#   - 官方镜像（docreader/minio/langfuse/clickhouse/paradedb/redis...）自动预拉
#   - minio 现为 pgsty/minio:RELEASE.2026-08-04...（主 minio 与 langfuse minio 同源），
#     属可正常 pull 的公开源，无需转推兜底（0.7.2 下架的 minio/minio 已随版本移除）
#   - 幂等: 已起容器不重建，仅缺镜像时按 build: 定义自构（frontend 不再构建，
#           前端包与 nginx 配置都不进镜像，由宿主机自行编译部署）
# 用法:
#   ./start.sh                       # 启动核心集 + minio + searxng，自动预拉官方镜像
#                                    #   （sandbox 镜像缺失即预拉，失败回退构建）
#   ./start.sh --build               # 先本地重建 app 镜像并 force-recreate app 容器（发版；
#                                    #   frontend 由宿主机自管；等价于 start + rebuild 合一）
#   ./start.sh --no-searxng          # 关 searxng
#   ./start.sh --no-minio --no-searxng  # 只起 CORE_SERVICES（最小化）
#   ./start.sh --help                # compose.sh 全量命令
#   （langfuse 观测栈: ./compose.sh profile-up langfuse，默认不再随 start 启动）
#
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
exec bash "${SCRIPT_DIR}/compose.sh" up "$@"
