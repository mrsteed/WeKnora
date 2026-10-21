#!/usr/bin/env bash
# Build frontend static assets for Lite / desktop packaging (host-side).
# Docker UI images use the multi-stage frontend/Dockerfile instead — no host npm.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

if [ -z "${VITE_FRONTEND_COMMIT:-}" ]; then
	# shellcheck source=/dev/null
	eval "$("$PROJECT_ROOT/scripts/get_version.sh" env)"
	export VITE_FRONTEND_COMMIT="${COMMIT_ID:-unknown}"
fi

export VITE_IS_DOCKER="${VITE_IS_DOCKER:-true}"

# The project uses pnpm as the authoritative lockfile source.
command -v pnpm >/dev/null 2>&1 || {
	echo "!! pnpm is required but not installed. Install it with: npm i -g pnpm (or corepack enable pnpm)" >&2
	exit 1
}

cd "$PROJECT_ROOT/frontend"
pnpm install --frozen-lockfile
pnpm run build
