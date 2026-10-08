#!/usr/bin/env bash
set -euo pipefail
#
# WeKnora 生产发版入口：本地重建 app + 重建运行中的容器（数据卷不动）。
#   docreader / minio / langfuse / clickhouse / paradedb / redis 等官方镜像
#   由 compose.sh up 阶段的 ensure_official_images 自动 docker pull 到最新，
#   不属于本地 build 目标。
#   frontend(nginx+dist) 已改宿主机自管（不进镜像、不由本栈重建），请勿传 frontend。
#   ./rebuild.sh                 # build app，然后 --force-recreate 运行中容器
#   ./rebuild.sh app             # 只重建 app（默认即 app）
#   ./rebuild.sh --profile minio # 顺带 up 可选 profile 服务
# 回滚: git 切回旧 commit 后重跑本脚本。
#
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
die() { printf '[weknora-prod] ERROR: %s\n' "$*" >&2; exit 1; }

profile=""
targets=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --profile) [[ $# -ge 2 ]] || die "--profile 需要参数"; profile="$2"; shift ;;
    --profile=*) profile="${1#--profile=}" ;;
    -h|--help)
      sed -n '3,9p' "$0" | sed 's/^# \{0,1\}//'
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
if [[ ${#targets[@]} -eq 0 ]]; then
  exec bash "${SCRIPT_DIR}/compose.sh" rebuild-all
else
  exec bash "${SCRIPT_DIR}/compose.sh" rebuild-all "${targets[@]}"
fi
