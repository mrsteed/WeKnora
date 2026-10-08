#!/usr/bin/env bash
# Build frontend static assets for Docker / release packaging.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

if [ -z "${VITE_FRONTEND_COMMIT:-}" ]; then
	# shellcheck source=/dev/null
	eval "$("$PROJECT_ROOT/scripts/get_version.sh" env)"
	export VITE_FRONTEND_COMMIT="${COMMIT_ID:-unknown}"
fi

export VITE_IS_DOCKER="${VITE_IS_DOCKER:-true}"

# 依赖安装用 pnpm(项目带 pnpm-lock.yaml, lockfileVersion 9)。
# npm 侧的 package-lock.json 与 pnpm-lock.yaml 并存时, 本机 npm 9 解析
# package-lock 会报 "Invalid comparator: none" / EUSAGE, 故不用 npm ci。
command -v pnpm >/dev/null 2>&1 || {
  echo "!! 需要 pnpm 但未安装。安装: npm i -g pnpm  (或 corepack enable pnpm)" >&2
  exit 1
}

cd "$PROJECT_ROOT/frontend"
pnpm install --frozen-lockfile
pnpm run build
