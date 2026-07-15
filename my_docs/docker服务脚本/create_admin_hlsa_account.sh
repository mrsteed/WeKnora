#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
WEKNORA_DIR="$WORKSPACE_DIR/WeKnora"
HELPER_SCRIPT="$WEKNORA_DIR/scripts/admin_helper/weknora_admin_setup.sh"

ADMIN_EMAIL="${ADMIN_EMAIL:-admin@hlsa.com}"
ADMIN_USERNAME="${ADMIN_USERNAME:-admin}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-hlsa2026.}"

if [[ ! -f "$HELPER_SCRIPT" ]]; then
  echo "[缺] 找不到管理员辅助脚本: $HELPER_SCRIPT" >&2
  exit 1
fi

if [[ -f "$WEKNORA_DIR/.env" ]]; then
  set -a
  source "$WEKNORA_DIR/.env"
  set +a
fi

sql_escape() {
  printf '%s' "$1" | sed "s/'/''/g"
}

DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-postgres}"
DB_PASSWORD="${DB_PASSWORD:-postgres}"
DB_NAME="${DB_NAME:-WeKnora}"

if ! command -v psql >/dev/null 2>&1; then
  echo "[缺] 需要 psql 命令，请先安装 postgresql-client。" >&2
  exit 1
fi

export ADMIN_EMAIL ADMIN_USERNAME ADMIN_PASSWORD
bash "$HELPER_SCRIPT"

export PGPASSWORD="$DB_PASSWORD"
psql_base=(psql -X -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1)

admin_email_escaped="$(sql_escape "$ADMIN_EMAIL")"
admin_username_escaped="$(sql_escape "$ADMIN_USERNAME")"

"${psql_base[@]}" <<SQL
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_system_admin BOOLEAN NOT NULL DEFAULT FALSE;
UPDATE users
SET username = '$admin_username_escaped',
    is_active = TRUE,
    can_access_all_tenants = TRUE,
    is_super_admin = TRUE,
    is_system_admin = TRUE,
    deleted_at = NULL,
    updated_at = NOW()
WHERE email = '$admin_email_escaped';
SQL

echo
echo "[OK] 已确保管理员账号存在并具备 System Admin 权限"
"${psql_base[@]}" -Atqc "SELECT id || '|' || username || '|' || email || '|' || is_active || '|' || can_access_all_tenants || '|' || is_super_admin || '|' || is_system_admin FROM users WHERE email = '$admin_email_escaped' LIMIT 1;"