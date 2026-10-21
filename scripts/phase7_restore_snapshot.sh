#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/phase7_restore_snapshot.sh <snapshot-dir> [--apply] [--with-db]

Default mode is a dry-run that prints the actions without mutating the environment.
Use --apply to copy .env back and run docker compose up -d.
Use --with-db together with --apply to restore weknora.sql through psql using DB_URL/DATABASE_URL.
EOF
}

if [[ $# -lt 1 ]]; then
  usage
  exit 1
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SNAPSHOT_DIR="$1"
shift

APPLY=0
WITH_DB=0
for arg in "$@"; do
  case "$arg" in
    --apply) APPLY=1 ;;
    --with-db) WITH_DB=1 ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $arg" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ ! -d "$SNAPSHOT_DIR" ]]; then
  echo "Snapshot directory not found: $SNAPSHOT_DIR" >&2
  exit 1
fi

ENV_FILE="$SNAPSHOT_DIR/.env.pre-upgrade"
DB_DUMP="$SNAPSHOT_DIR/weknora.sql"

echo "Phase 7 restore plan"
echo "  root:      $ROOT"
echo "  snapshot:  $SNAPSHOT_DIR"
echo "  apply:     $APPLY"
echo "  with_db:   $WITH_DB"

if [[ $APPLY -ne 1 ]]; then
  [[ -f "$ENV_FILE" ]] && echo "DRY-RUN: would restore $ENV_FILE -> $ROOT/.env"
  echo "DRY-RUN: would run docker compose up -d in $ROOT"
  if [[ $WITH_DB -eq 1 ]]; then
    echo "DRY-RUN: would restore $DB_DUMP through psql using DB_URL/DATABASE_URL"
  fi
  exit 0
fi

if [[ -f "$ENV_FILE" ]]; then
  cp "$ENV_FILE" "$ROOT/.env"
fi

if [[ $WITH_DB -eq 1 ]]; then
  DB_URL_VALUE="${DB_URL:-${DATABASE_URL:-}}"
  if [[ -z "$DB_URL_VALUE" ]]; then
    echo "DB_URL / DATABASE_URL is required for --with-db" >&2
    exit 1
  fi
  if [[ ! -f "$DB_DUMP" ]]; then
    echo "SQL dump not found: $DB_DUMP" >&2
    exit 1
  fi
  psql "$DB_URL_VALUE" < "$DB_DUMP"
fi

(cd "$ROOT" && docker compose up -d)