#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

if [[ -f "$PROJECT_ROOT/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$PROJECT_ROOT/.env"
  set +a
fi

DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-postgres}"
DB_PASSWORD="${DB_PASSWORD:-postgres}"
DB_NAME="${DB_NAME:-WeKnora}"

export PGPASSWORD="$DB_PASSWORD"
psql_base=(psql -X -P pager=off -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1)

run_query() {
  local title=$1
  local sql=$2
  printf '\n[%s]\n' "$title"
  "${psql_base[@]}" -A -F $'\t' -P footer=off -c "$sql"
}

run_query \
  "platform_flag_users" \
  "SELECT id, username, is_super_admin, is_system_admin, can_access_all_tenants, tenant_id FROM users WHERE deleted_at IS NULL AND (is_super_admin = TRUE OR is_system_admin = TRUE OR can_access_all_tenants = TRUE) ORDER BY username;"

run_query \
  "tenant_owner_admin_summary" \
  "SELECT tenant_id, COUNT(*) FILTER (WHERE role = 'owner') AS owner_count, string_agg(CASE WHEN role = 'owner' THEN username END, ', ' ORDER BY username) FILTER (WHERE role = 'owner') AS owners, COUNT(*) FILTER (WHERE role = 'admin') AS admin_count, string_agg(CASE WHEN role = 'admin' THEN username END, ', ' ORDER BY username) FILTER (WHERE role = 'admin') AS admins FROM ( SELECT tm.tenant_id, tm.role, u.username FROM tenant_members tm JOIN users u ON u.id = tm.user_id WHERE tm.deleted_at IS NULL AND u.deleted_at IS NULL ) s GROUP BY tenant_id ORDER BY tenant_id;"

run_query \
  "org_tree_admin_vs_tenant_admin" \
  "WITH org_admin_scope AS ( SELECT omp.user_id, o.tenant_id, BOOL_OR(omp.role = 'admin') AS has_any_org_admin, BOOL_OR(omp.role = 'admin' AND (o.level <= 1 OR o.parent_id IS NULL OR btrim(COALESCE(o.parent_id::text, '')) = '')) AS has_root_org_admin FROM organization_members_pre_plan3 omp JOIN organizations o ON o.id = omp.organization_id WHERE o.deleted_at IS NULL GROUP BY omp.user_id, o.tenant_id ) SELECT tm.tenant_id, t.name AS tenant_name, u.username, tm.role, COALESCE(scope.has_any_org_admin, FALSE) AS has_any_org_admin, COALESCE(scope.has_root_org_admin, FALSE) AS has_root_org_admin, u.is_super_admin, u.is_system_admin, u.can_access_all_tenants FROM tenant_members tm JOIN users u ON u.id = tm.user_id LEFT JOIN tenants t ON t.id = tm.tenant_id LEFT JOIN org_admin_scope scope ON scope.user_id = tm.user_id AND scope.tenant_id = tm.tenant_id WHERE tm.deleted_at IS NULL AND u.deleted_at IS NULL AND tm.role IN ('owner','admin') ORDER BY tm.tenant_id, CASE tm.role WHEN 'owner' THEN 0 ELSE 1 END, u.username;"
