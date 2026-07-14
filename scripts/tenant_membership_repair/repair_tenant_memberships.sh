#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

usage() {
	cat <<'EOF'
Usage: repair_tenant_memberships.sh (--tenant-id ID | --tenant-name NAME) [options]

Rebind tenant_members from the current org-tree personnel source
(organization_members_pre_plan3) plus an explicit owner account.

Role mapping:
1. owner-login / system-admin target -> owner
2. org-tree admin               -> admin
3. org-tree editor/contributor  -> contributor
4. org-tree viewer              -> viewer

The script intentionally does NOT use organization_tenant_members as the
truth source, because the current 组织人员管理 UI still reads
organization_members_pre_plan3. It also does NOT soft-delete users; it only
rebinds tenant_members for the target tenant.

Options:
  --tenant-id ID         Target tenant id
  --tenant-name NAME     Resolve tenant by exact name
  --owner-login LOGIN    Username or email to force as tenant owner (default: hlsa)
  --demote-admin LOGIN   Username or email to force as non-admin tenant member.
                         May be repeated. Defaults to contributor unless
                         overridden by --demote-admin-role.
  --demote-admin-role R  Target tenant role for --demote-admin entries:
                         contributor or viewer (default: contributor)
  --remove-owner LOGIN   Username or email to remove from tenant owner/admin
                         target set after owner reassignment. May be repeated.
  --execute              Apply changes. Without this flag the script runs in dry-run mode.
  --help                 Show this help

The script reads DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME from .env when present.
EOF
}

TENANT_ID=""
TENANT_NAME=""
OWNER_LOGIN="${OWNER_LOGIN:-hlsa}"
EXECUTE=0
DEMOTE_ADMIN_ROLE="contributor"
declare -a DEMOTE_ADMINS=()
declare -a REMOVE_OWNERS=()

while [[ $# -gt 0 ]]; do
	case "$1" in
		--tenant-id)
			TENANT_ID="$2"
			shift 2
			;;
		--tenant-name)
			TENANT_NAME="$2"
			shift 2
			;;
		--owner-login)
			OWNER_LOGIN="$2"
			shift 2
			;;
    --demote-admin)
      DEMOTE_ADMINS+=("$2")
      shift 2
      ;;
    --demote-admin-role)
      DEMOTE_ADMIN_ROLE="$2"
      shift 2
      ;;
    --remove-owner)
      REMOVE_OWNERS+=("$2")
      shift 2
      ;;
		--execute)
			EXECUTE=1
			shift
			;;
		--help|-h)
			usage
			exit 0
			;;
		*)
			echo "Unknown argument: $1" >&2
			usage >&2
			exit 1
			;;
	esac
done

if [[ -z "$TENANT_ID" && -z "$TENANT_NAME" ]]; then
	echo "Either --tenant-id or --tenant-name is required" >&2
	usage >&2
	exit 1
fi

if [[ -n "$TENANT_ID" && -n "$TENANT_NAME" ]]; then
	echo "Use either --tenant-id or --tenant-name, not both" >&2
	exit 1
fi

if [[ "$DEMOTE_ADMIN_ROLE" != "contributor" && "$DEMOTE_ADMIN_ROLE" != "viewer" ]]; then
  echo "--demote-admin-role must be contributor or viewer" >&2
  exit 1
fi

if [[ -f "$PROJECT_ROOT/.env" ]]; then
	set -a
	source "$PROJECT_ROOT/.env"
	set +a
fi

DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-postgres}"
DB_PASSWORD="${DB_PASSWORD:-postgres}"
DB_NAME="${DB_NAME:-WeKnora}"

if ! command -v psql >/dev/null 2>&1; then
	echo "psql is required but not installed" >&2
	exit 1
fi

export PGPASSWORD="$DB_PASSWORD"
export PSQL_PAGER=off
psql_base=(psql -X -P pager=off -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1)

run_scalar() {
	local sql="$1"
	"${psql_base[@]}" -Atqc "$sql"
}

run_query() {
  local sql="$1"
  local tenant_id="$2"
  local owner_id="$3"
  local rendered_sql="$sql"
  rendered_sql="${rendered_sql//__TENANT_ID__/$tenant_id}"
  rendered_sql="${rendered_sql//__OWNER_ID__/$owner_id}"
  local out
  out=$("${psql_base[@]}" -A -F '|' -P footer=off -c "$rendered_sql")
  printf '%s\n' "$out"
}

run_exec() {
  local sql="$1"
  local tenant_id="$2"
  local owner_id="$3"
  local rendered_sql="$sql"
  rendered_sql="${rendered_sql//__TENANT_ID__/$tenant_id}"
  rendered_sql="${rendered_sql//__OWNER_ID__/$owner_id}"
  local out
  out=$("${psql_base[@]}" -c "$rendered_sql")
  printf '%s\n' "$out"
}

if [[ -z "$TENANT_ID" ]]; then
	TENANT_ID="$(run_scalar "SELECT id::text FROM tenants WHERE name = '$(printf "%s" "$TENANT_NAME" | sed "s/'/''/g")' ORDER BY id ASC LIMIT 1;")"
	if [[ -z "$TENANT_ID" ]]; then
		echo "Tenant not found: $TENANT_NAME" >&2
		exit 1
	fi
fi

OWNER_SQL_LOGIN="$(printf "%s" "$OWNER_LOGIN" | sed "s/'/''/g")"
OWNER_ID="$(run_scalar "SELECT id FROM users WHERE username = '$OWNER_SQL_LOGIN' OR email = '$OWNER_SQL_LOGIN' ORDER BY CASE WHEN username = '$OWNER_SQL_LOGIN' THEN 0 ELSE 1 END, created_at ASC LIMIT 1;")"
if [[ -z "$OWNER_ID" ]]; then
	echo "Owner login not found: $OWNER_LOGIN" >&2
	exit 1
fi

lookup_user_id() {
  local login="$1"
  local login_sql
  login_sql="$(printf "%s" "$login" | sed "s/'/''/g")"
  run_scalar "SELECT id FROM users WHERE username = '$login_sql' OR email = '$login_sql' ORDER BY CASE WHEN username = '$login_sql' THEN 0 ELSE 1 END, created_at ASC LIMIT 1;"
}

DEMOTE_USER_IDS=()
for login in "${DEMOTE_ADMINS[@]}"; do
  uid="$(lookup_user_id "$login")"
  if [[ -z "$uid" ]]; then
    echo "Demote admin login not found: $login" >&2
    exit 1
  fi
  DEMOTE_USER_IDS+=("$uid")
done

REMOVE_OWNER_IDS=()
for login in "${REMOVE_OWNERS[@]}"; do
  uid="$(lookup_user_id "$login")"
  if [[ -z "$uid" ]]; then
    echo "Remove owner login not found: $login" >&2
    exit 1
  fi
  REMOVE_OWNER_IDS+=("$uid")
done

join_sql_ids() {
  local joined=""
  local id
  for id in "$@"; do
    if [[ -n "$joined" ]]; then
      joined+=","
    fi
    joined+="$id"
  done
  printf '%s' "$joined"
}

DEMOTE_IDS_SQL="$(join_sql_ids "${DEMOTE_USER_IDS[@]}")"
REMOVE_OWNER_IDS_SQL="$(join_sql_ids "${REMOVE_OWNER_IDS[@]}")"

echo "Tenant ID      : $TENANT_ID"
echo "Owner login    : $OWNER_LOGIN"
echo "Owner user id  : $OWNER_ID"
echo "Demote admins  : ${DEMOTE_ADMINS[*]:-(none)}"
echo "Demote role    : $DEMOTE_ADMIN_ROLE"
echo "Remove owners  : ${REMOVE_OWNERS[*]:-(none)}"
echo "Mode           : $([[ $EXECUTE -eq 1 ]] && echo EXECUTE || echo DRY-RUN)"
echo

read -r -d '' DRY_RUN_SQL <<'SQL' || true
WITH params AS (
  SELECT CAST(__TENANT_ID__ AS bigint) AS tenant_id,
         CAST('__OWNER_ID__' AS varchar) AS owner_id
),
org_tree_users AS (
  SELECT omp.user_id,
         MAX(CASE
               WHEN omp.role IN ('owner', 'admin') THEN 30
               WHEN omp.role IN ('editor', 'contributor') THEN 20
               ELSE 10
             END) AS role_rank
  FROM organization_members_pre_plan3 omp
  JOIN organizations o ON o.id = omp.organization_id
  JOIN users u ON u.id = omp.user_id
  JOIN params p ON p.tenant_id = o.tenant_id
  WHERE o.deleted_at IS NULL
    AND u.deleted_at IS NULL
  GROUP BY omp.user_id
),
target_members AS (
  SELECT p.owner_id AS user_id, 'owner'::varchar AS target_role
  FROM params p
  UNION ALL
  SELECT otu.user_id,
         CASE
           WHEN otu.role_rank >= 30 THEN 'admin'
           WHEN otu.role_rank >= 20 THEN 'contributor'
           ELSE 'viewer'
         END::varchar AS target_role
  FROM org_tree_users otu
  JOIN params p ON TRUE
  WHERE otu.user_id <> p.owner_id
),
target_overrides AS (
  SELECT user_id, target_role
  FROM target_members
  WHERE user_id NOT IN (SELECT unnest(CASE WHEN '__DEMOTE_IDS__' = '' THEN ARRAY[]::varchar[] ELSE string_to_array('__DEMOTE_IDS__', ',')::varchar[] END))
    AND user_id NOT IN (SELECT unnest(CASE WHEN '__REMOVE_OWNER_IDS__' = '' THEN ARRAY[]::varchar[] ELSE string_to_array('__REMOVE_OWNER_IDS__', ',')::varchar[] END))
  UNION ALL
  SELECT unnest(CASE WHEN '__DEMOTE_IDS__' = '' THEN ARRAY[]::varchar[] ELSE string_to_array('__DEMOTE_IDS__', ',')::varchar[] END) AS user_id,
         '__DEMOTE_ROLE__'::varchar AS target_role
),
current_live AS (
  SELECT tm.user_id, tm.role, tm.status
  FROM tenant_members tm
  JOIN params p ON p.tenant_id = tm.tenant_id
  WHERE tm.deleted_at IS NULL
),
missing_memberships AS (
  SELECT t.user_id, t.target_role
  FROM target_overrides t
  LEFT JOIN current_live c ON c.user_id = t.user_id
  WHERE c.user_id IS NULL
),
role_changes AS (
  SELECT t.user_id, c.role AS current_role, t.target_role
  FROM target_overrides t
  JOIN current_live c ON c.user_id = t.user_id
  WHERE c.role <> t.target_role OR c.status <> 'active'
),
stale_memberships AS (
  SELECT c.user_id, c.role AS current_role
  FROM current_live c
  LEFT JOIN target_overrides t ON t.user_id = c.user_id
  WHERE t.user_id IS NULL
)
SELECT 'owner_target' AS item, owner_id AS value FROM params
UNION ALL
SELECT 'org_tree_user_count', COUNT(*)::text FROM org_tree_users
UNION ALL
SELECT 'target_member_count', COUNT(*)::text FROM target_overrides
UNION ALL
SELECT 'target_owner_count', COUNT(*)::text FROM target_overrides WHERE target_role = 'owner'
UNION ALL
SELECT 'target_admin_count', COUNT(*)::text FROM target_overrides WHERE target_role = 'admin'
UNION ALL
SELECT 'target_contributor_count', COUNT(*)::text FROM target_overrides WHERE target_role = 'contributor'
UNION ALL
SELECT 'target_viewer_count', COUNT(*)::text FROM target_overrides WHERE target_role = 'viewer'
UNION ALL
SELECT 'missing_membership_count', COUNT(*)::text FROM missing_memberships
UNION ALL
SELECT 'role_change_count', COUNT(*)::text FROM role_changes
UNION ALL
SELECT 'stale_membership_count', COUNT(*)::text FROM stale_memberships
UNION ALL
SELECT 'missing_member', u.username || ' -> ' || m.target_role
FROM missing_memberships m
JOIN users u ON u.id = m.user_id
UNION ALL
SELECT 'role_change', u.username || ' ' || rc.current_role || ' -> ' || rc.target_role
FROM role_changes rc
JOIN users u ON u.id = rc.user_id
UNION ALL
SELECT 'stale_membership_user', u.username || ' (' || s.current_role || ')'
FROM stale_memberships s
JOIN users u ON u.id = s.user_id
ORDER BY item, value;
SQL

read -r -d '' EXECUTE_SQL <<'SQL' || true
BEGIN;

WITH params AS (
  SELECT CAST(__TENANT_ID__ AS bigint) AS tenant_id,
         CAST('__OWNER_ID__' AS varchar) AS owner_id
),
org_tree_users AS (
  SELECT omp.user_id,
         MAX(CASE
               WHEN omp.role IN ('owner', 'admin') THEN 30
               WHEN omp.role IN ('editor', 'contributor') THEN 20
               ELSE 10
             END) AS role_rank
  FROM organization_members_pre_plan3 omp
  JOIN organizations o ON o.id = omp.organization_id
  JOIN users u ON u.id = omp.user_id
  JOIN params p ON p.tenant_id = o.tenant_id
  WHERE o.deleted_at IS NULL
    AND u.deleted_at IS NULL
  GROUP BY omp.user_id
),
target_members AS (
  SELECT p.owner_id AS user_id, 'owner'::varchar AS target_role
  FROM params p
  UNION ALL
  SELECT otu.user_id,
         CASE
           WHEN otu.role_rank >= 30 THEN 'admin'
           WHEN otu.role_rank >= 20 THEN 'contributor'
           ELSE 'viewer'
         END::varchar AS target_role
  FROM org_tree_users otu
  JOIN params p ON TRUE
  WHERE otu.user_id <> p.owner_id
),
target_overrides AS (
  SELECT user_id, target_role
  FROM target_members
  WHERE user_id NOT IN (SELECT unnest(CASE WHEN '__DEMOTE_IDS__' = '' THEN ARRAY[]::varchar[] ELSE string_to_array('__DEMOTE_IDS__', ',')::varchar[] END))
    AND user_id NOT IN (SELECT unnest(CASE WHEN '__REMOVE_OWNER_IDS__' = '' THEN ARRAY[]::varchar[] ELSE string_to_array('__REMOVE_OWNER_IDS__', ',')::varchar[] END))
  UNION ALL
  SELECT unnest(CASE WHEN '__DEMOTE_IDS__' = '' THEN ARRAY[]::varchar[] ELSE string_to_array('__DEMOTE_IDS__', ',')::varchar[] END) AS user_id,
         '__DEMOTE_ROLE__'::varchar AS target_role
),
update_live AS (
  UPDATE tenant_members tm
  SET role = t.target_role,
      status = 'active',
      updated_at = NOW()
  FROM target_overrides t, params p
  WHERE tm.tenant_id = p.tenant_id
    AND tm.deleted_at IS NULL
    AND tm.user_id = t.user_id
  RETURNING tm.user_id
),
insert_missing AS (
  INSERT INTO tenant_members (user_id, tenant_id, role, status, joined_at, created_at, updated_at, deleted_at)
  SELECT t.user_id, p.tenant_id, t.target_role, 'active', NOW(), NOW(), NOW(), NULL
  FROM target_overrides t
  CROSS JOIN params p
  WHERE NOT EXISTS (
    SELECT 1
    FROM tenant_members tm
    WHERE tm.tenant_id = p.tenant_id
      AND tm.user_id = t.user_id
      AND tm.deleted_at IS NULL
  )
  RETURNING user_id
),
stale_memberships AS (
  UPDATE tenant_members tm
  SET deleted_at = NOW(), updated_at = NOW()
  FROM params p
  WHERE tm.tenant_id = p.tenant_id
    AND tm.deleted_at IS NULL
    AND tm.user_id NOT IN (SELECT user_id FROM target_overrides)
  RETURNING tm.user_id
)
SELECT (SELECT COUNT(*) FROM update_live) AS updated_count,
       (SELECT COUNT(*) FROM insert_missing) AS inserted_count,
       (SELECT COUNT(*) FROM stale_memberships) AS stale_deleted_count;

COMMIT;
SQL

if [[ $EXECUTE -eq 1 ]]; then
  exec_sql="$EXECUTE_SQL"
  exec_sql="${exec_sql//__DEMOTE_IDS__/$DEMOTE_IDS_SQL}"
  exec_sql="${exec_sql//__REMOVE_OWNER_IDS__/$REMOVE_OWNER_IDS_SQL}"
  exec_sql="${exec_sql//__DEMOTE_ROLE__/$DEMOTE_ADMIN_ROLE}"
  run_exec "$exec_sql" "$TENANT_ID" "$OWNER_ID"
	echo "Repair completed."
else
  dry_sql="$DRY_RUN_SQL"
  dry_sql="${dry_sql//__DEMOTE_IDS__/$DEMOTE_IDS_SQL}"
  dry_sql="${dry_sql//__REMOVE_OWNER_IDS__/$REMOVE_OWNER_IDS_SQL}"
  dry_sql="${dry_sql//__DEMOTE_ROLE__/$DEMOTE_ADMIN_ROLE}"
  run_query "$dry_sql" "$TENANT_ID" "$OWNER_ID"
	echo
	echo "Dry-run only. Re-run with --execute to apply changes."
fi