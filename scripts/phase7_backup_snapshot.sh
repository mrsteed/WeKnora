#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKUP_ROOT="${PHASE7_BACKUP_ROOT:-$ROOT/backups}"
TIMESTAMP="${1:-$(date +%Y%m%d-%H%M%S)}"
SNAPSHOT_DIR="$BACKUP_ROOT/$TIMESTAMP"

mkdir -p "$SNAPSHOT_DIR"

if [[ -f "$ROOT/.env" ]]; then
  cp "$ROOT/.env" "$SNAPSHOT_DIR/.env.pre-upgrade"
fi

git -C "$ROOT" rev-parse HEAD > "$SNAPSHOT_DIR/git.head.txt"
git -C "$ROOT" status --short --branch > "$SNAPSHOT_DIR/git.status.txt"

if command -v docker >/dev/null 2>&1; then
  (cd "$ROOT" && docker compose ps) > "$SNAPSHOT_DIR/docker-compose.ps.txt" 2>&1 || true
  (cd "$ROOT" && docker compose config) > "$SNAPSHOT_DIR/docker-compose.config.yaml" 2>&1 || true
fi

if [[ -d "$ROOT/config/export" ]]; then
  tar -C "$ROOT" -czf "$SNAPSHOT_DIR/config-export.tgz" config/export
fi

DB_URL_VALUE="${DB_URL:-${DATABASE_URL:-}}"
if [[ -n "$DB_URL_VALUE" && "${PHASE7_SKIP_DB:-0}" != "1" ]]; then
  if command -v pg_dump >/dev/null 2>&1; then
    pg_dump "$DB_URL_VALUE" > "$SNAPSHOT_DIR/weknora.sql"
  else
    echo "pg_dump not found; skipped database backup" > "$SNAPSHOT_DIR/db-backup-skipped.txt"
  fi
else
  echo "DB_URL / DATABASE_URL not set or PHASE7_SKIP_DB=1; skipped database backup" > "$SNAPSHOT_DIR/db-backup-skipped.txt"
fi

echo "$SNAPSHOT_DIR"