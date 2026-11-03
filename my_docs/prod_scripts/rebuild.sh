#!/usr/bin/env bash
set -euo pipefail
#
# WeKnora 生产发版入口：本地重建 app + 重建运行中的容器（数据卷不动）。
#   docreader / minio / langfuse / clickhouse / paradedb / redis 等官方镜像
#   由 compose.sh up 阶段的 ensure_official_images 自动 docker pull 到最新，
#   不属于本地 build 目标。
#   重建前会自动预热 app 的硬依赖 profile 服务（minio/langfuse）：minio 启动即查
#   bucket，缺失会导致 app panic 重启死循环；已在运行者幂等秒过（见 compose.sh rebuild-all）。
#   frontend(nginx+dist) 已改宿主机自管（不进镜像、不由本栈重建），请勿传 frontend。
#   ./rebuild.sh                 # build app，然后 --force-recreate 运行中容器
#   ./rebuild.sh app             # 只重建 app（默认即 app）
#   ./rebuild.sh --fast          # 只改 Go: 增量构建 app + 重建容器(跳过依赖预热/官方镜像预拉)
#   ./rebuild.sh --profile minio # 顺带 up 可选 profile 服务
# 回滚: git 切回旧 commit 后重跑本脚本。
#
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
die() { printf '[weknora-prod] ERROR: %s\n' "$*" >&2; exit 1; }

profile=""
fast=0
targets=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --profile) [[ $# -ge 2 ]] || die "--profile 需要参数"; profile="$2"; shift ;;
    --profile=*) profile="${1#--profile=}" ;;
    --fast) fast=1 ;;
    -h|--help)
      cat <<'EOF'
WeKnora 生产发版入口：本地重建 app + 重建运行中的容器（数据卷不动）。

用法:
  rebuild.sh [选项] [target...]

选项:
  -h, --help             显示本帮助并退出
  --fast                 只改 Go: 增量构建 app + --force-recreate 容器，
                         跳过依赖预热/官方镜像预拉/sandbox（假设 minio、searxng 已健康）
  --profile <name>       顺带 up 可选 profile 服务（如 minio / searxng），等价 --profile=<name>

示例:
  rebuild.sh                   # build app，然后 --force-recreate 运行中容器
  rebuild.sh app               # 只重建 app（默认即 app）
  rebuild.sh --fast            # 只改了 Go 代码时的快速发版路径（最快）
  rebuild.sh --profile minio   # 顺带起 minio profile

说明:
  - docreader/minio/langfuse/clickhouse/paradedb/redis 等官方镜像由
    compose.sh up 阶段自动 docker pull 到最新，不属于本地 build 目标。
  - 重建前会自动预热 app 硬依赖(minio/langfuse)，已运行者幂等秒过。
  - frontend(nginx+dist) 已改宿主机自管，请勿传 frontend。
  - 回滚：git 切回旧 commit 后重跑本脚本。
EOF
      exit 0
      ;;
    -*) die "未知参数: $1" ;;
    *) targets+=("$1") ;;
  esac
  shift
done

if [[ -n "$profile" ]]; then
  bash "${SCRIPT_DIR}/compose.sh" profile-up "${profile}"
fi
fast_flag=""
[[ "$fast" == 1 ]] && fast_flag="--fast"
if [[ ${#targets[@]} -eq 0 ]]; then
  exec bash "${SCRIPT_DIR}/compose.sh" rebuild-all $fast_flag
else
  exec bash "${SCRIPT_DIR}/compose.sh" rebuild-all $fast_flag "${targets[@]}"
fi
