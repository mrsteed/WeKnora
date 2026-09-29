#!/usr/bin/env bash
set -euo pipefail
#
# WeKnora 生产发版入口：重建镜像 + 重建运行中的容器（数据卷不动）。
#   ./rebuild.sh                 # build app docreader frontend，然后全部 --force-recreate
#   ./rebuild.sh frontend        # 只重建前端镜像与容器
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
