#!/usr/bin/env bash
set -euo pipefail

# This helper bootstraps the minimal authentication schema and a reusable admin
# account in an otherwise empty PostgreSQL database. It intentionally avoids the
# repository-wide migrate tool because the current versioned migration directory
# contains duplicate sequence numbers that can block a normal `migrate up` run.
#
# Scope of this script:
# - applies only the minimal migrations needed for tenants/users/auth tables
# - creates or updates a default admin account idempotently
# - mirrors the current code logic for bcrypt password hashing and tenant API key
#   generation by delegating those calculations to weknora_admin_helper.go

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

DEFAULT_ADMIN_EMAIL="admin@hlsa.com"
DEFAULT_ADMIN_USERNAME="admin"
DEFAULT_ADMIN_PASSWORD="a1234567."
DEFAULT_TENANT_NAME="admin's Workspace"
DEFAULT_TENANT_DESCRIPTION="Default workspace"
DEFAULT_TENANT_BUSINESS="Default workspace"

ADMIN_EMAIL="${ADMIN_EMAIL:-$DEFAULT_ADMIN_EMAIL}"
ADMIN_USERNAME="${ADMIN_USERNAME:-$DEFAULT_ADMIN_USERNAME}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-$DEFAULT_ADMIN_PASSWORD}"

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

if ! command -v go >/dev/null 2>&1; then
	echo "go is required but not installed" >&2
	exit 1
fi

tenant_aes_key="${TENANT_AES_KEY:-}"
system_aes_key="${SYSTEM_AES_KEY:-}"

if [[ ${#tenant_aes_key} -ne 32 ]]; then
	echo "TENANT_AES_KEY must be exactly 32 bytes" >&2
	exit 1
fi

if [[ ${#system_aes_key} -ne 32 ]]; then
	echo "SYSTEM_AES_KEY must be exactly 32 bytes" >&2
	exit 1
fi

export PGPASSWORD="$DB_PASSWORD"

psql_base=(psql -X -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1)

run_psql_file() {
	local sql_file="$1"
	"${psql_base[@]}" -f "$sql_file"
}

run_psql_scalar() {
	local sql="$1"
	"${psql_base[@]}" -Atqc "$sql"
}

echo "Applying minimal authentication schema..."
run_psql_file "$PROJECT_ROOT/migrations/versioned/000000_init.up.sql"
run_psql_file "$PROJECT_ROOT/migrations/versioned/000001_agent.up.sql"
"${psql_base[@]}" <<'SQL'
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_super_admin BOOLEAN DEFAULT false;
SQL

existing_user_line="$(run_psql_scalar "SELECT id || '|' || COALESCE(tenant_id::text, '') FROM users WHERE email = '$ADMIN_EMAIL' LIMIT 1;")"

tenant_id=""
user_id=""
user_action=""
tenant_action="unchanged"

if [[ -n "$existing_user_line" ]]; then
	user_id="${existing_user_line%%|*}"
	tenant_id="${existing_user_line#*|}"
	user_action="updated"
else
	tenant_id="$(run_psql_scalar "SELECT id::text FROM tenants WHERE name = '$DEFAULT_TENANT_NAME' ORDER BY id ASC LIMIT 1;")"
	if [[ -z "$tenant_id" ]]; then
		tenant_id="$(run_psql_scalar "INSERT INTO tenants (name, description, api_key, retriever_engines, status, business, created_at, updated_at) VALUES ('$DEFAULT_TENANT_NAME', '$DEFAULT_TENANT_DESCRIPTION', 'sk-bootstrap', '{\"engines\":[]}'::jsonb, 'active', '$DEFAULT_TENANT_BUSINESS', NOW(), NOW()) RETURNING id;")"
		tenant_action="created"
	else
		tenant_action="reused"
	fi
fi

password_hash=""
tenant_api_key=""
encrypted_tenant_api_key=""
while IFS='=' read -r key value; do
	case "$key" in
		PASSWORD_HASH) password_hash="$value" ;;
		TENANT_API_KEY) tenant_api_key="$value" ;;
		ENCRYPTED_TENANT_API_KEY) encrypted_tenant_api_key="$value" ;;
	esac
done < <(go run "$SCRIPT_DIR/weknora_admin_helper.go" generate --tenant-id "$tenant_id" --password "$ADMIN_PASSWORD")

if [[ -z "$password_hash" || -z "$tenant_api_key" || -z "$encrypted_tenant_api_key" ]]; then
	echo "failed to generate helper values" >&2
	exit 1
fi

if [[ "$tenant_action" == "created" ]]; then
	api_key_max_length="$(run_psql_scalar "SELECT COALESCE(character_maximum_length::text, '') FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'tenants' AND column_name = 'api_key';")"
	api_key_to_store="$encrypted_tenant_api_key"
	if [[ -n "$api_key_max_length" && "$api_key_max_length" != "0" ]]; then
		if (( ${#encrypted_tenant_api_key} > api_key_max_length )); then
			# Minimal schema from 000000 keeps tenants.api_key at VARCHAR(64). In that
			# case we store the legacy plaintext sk- key so current code can still read
			# it through AfterFind, while later full migrations can extend the column.
			api_key_to_store="$tenant_api_key"
		fi
	fi
	run_psql_scalar "UPDATE tenants SET api_key = '$api_key_to_store', updated_at = NOW() WHERE id = $tenant_id;" >/dev/null
fi

if [[ -n "$user_id" ]]; then
	run_psql_scalar "UPDATE users SET username = '$ADMIN_USERNAME', password_hash = '$password_hash', tenant_id = ${tenant_id:-NULL}, is_active = true, can_access_all_tenants = true, is_super_admin = true, updated_at = NOW(), deleted_at = NULL WHERE id = '$user_id';" >/dev/null
else
	user_id="$(run_psql_scalar "INSERT INTO users (id, username, email, password_hash, tenant_id, is_active, can_access_all_tenants, is_super_admin, created_at, updated_at) VALUES (uuid_generate_v4(), '$ADMIN_USERNAME', '$ADMIN_EMAIL', '$password_hash', $tenant_id, true, true, true, NOW(), NOW()) RETURNING id;")"
	user_action="created"
fi

echo
echo "Schema applied: migrations/versioned/000000_init.up.sql, migrations/versioned/000001_agent.up.sql, users.is_super_admin"
echo "Tenant action: $tenant_action (tenant_id=${tenant_id:-none})"
echo "User action: $user_action (user_id=${user_id:-none})"
echo
echo "Final admin row:"
run_psql_scalar "SELECT id || '|' || username || '|' || email || '|' || COALESCE(tenant_id::text, '') || '|' || is_active || '|' || can_access_all_tenants || '|' || is_super_admin || '|' || created_at FROM users WHERE email = '$ADMIN_EMAIL' LIMIT 1;"