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

DB_HOST=${DB_HOST:-localhost}
DB_PORT=${DB_PORT:-5432}
DB_USER=${DB_USER:-postgres}
DB_PASSWORD=${DB_PASSWORD:-postgres}
DB_NAME=${DB_NAME:-WeKnora}

TENANT_ID=""
TENANT_NAME=""
CHANNEL_ID=""
AGENT_ID=""
USER_LOGIN=""
EXECUTE=false

usage() {
  cat <<'EOF'
用法:
  assign_im_channel_creator.sh [--tenant-id ID | --tenant-name NAME] [--channel-id ID] [--agent-id ID] [--user-login LOGIN] [--execute]

默认行为:
  - 不带 --channel-id / --agent-id 时：列出未归属 (created_by='') 的 IM 渠道
  - 带 --channel-id 和 --user-login：把指定渠道归属到该用户
  - 带 --agent-id 和 --user-login：把指定租户下该 agent 的所有未归属渠道批量归属到该用户

选项:
  --tenant-id ID        指定租户 ID
  --tenant-name NAME    按租户名匹配租户
  --channel-id ID       指定单个渠道 ID
  --agent-id ID         指定 agent ID（批量归属未归属渠道）
  --user-login LOGIN    用户名或邮箱
  --execute             真正执行 UPDATE；默认只预览 SQL 和目标记录
EOF
}

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
    --channel-id)
      CHANNEL_ID="$2"
      shift 2
      ;;
    --agent-id)
      AGENT_ID="$2"
      shift 2
      ;;
    --user-login)
      USER_LOGIN="$2"
      shift 2
      ;;
    --execute)
      EXECUTE=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "未知参数: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ -z "$TENANT_ID" && -n "$TENANT_NAME" ]]; then
  TENANT_SQL_NAME="${TENANT_NAME//\'/\'\'}"
  TENANT_ID=$(psql "postgres://${DB_USER}:$(python3 -c 'import os, urllib.parse; print(urllib.parse.quote(os.environ.get("DB_PASSWORD", "postgres"), safe=""))')@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable" -Atqc "SELECT id FROM tenants WHERE name = '${TENANT_SQL_NAME}' ORDER BY id ASC LIMIT 1;")
fi

if [[ -z "$TENANT_ID" ]]; then
  echo "必须提供 --tenant-id 或可解析到租户的 --tenant-name" >&2
  exit 1
fi

ENCODED_PASSWORD=$(python3 -c 'import os, urllib.parse; print(urllib.parse.quote(os.environ.get("DB_PASSWORD", "postgres"), safe=""))')
DB_URL=${DB_URL:-postgres://${DB_USER}:${ENCODED_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable}

run_sql() {
  psql "$DB_URL" -v ON_ERROR_STOP=1 -P pager=off -c "$1"
}

run_scalar() {
  psql "$DB_URL" -v ON_ERROR_STOP=1 -Atqc "$1"
}

if [[ -z "$CHANNEL_ID" && -z "$AGENT_ID" ]]; then
  run_sql "SELECT id, tenant_id, agent_id, platform, name, created_by, created_at FROM im_channels WHERE tenant_id = ${TENANT_ID} AND deleted_at IS NULL AND COALESCE(created_by, '') = '' ORDER BY created_at ASC;"
  exit 0
fi

if [[ -z "$USER_LOGIN" ]]; then
  echo "执行归属操作时必须提供 --user-login" >&2
  exit 1
fi

USER_SQL_LOGIN="${USER_LOGIN//\'/\'\'}"
TARGET_USER_ID=$(run_scalar "SELECT id FROM users WHERE username = '${USER_SQL_LOGIN}' OR email = '${USER_SQL_LOGIN}' ORDER BY CASE WHEN username = '${USER_SQL_LOGIN}' THEN 0 ELSE 1 END, created_at ASC LIMIT 1;")
if [[ -z "$TARGET_USER_ID" ]]; then
  echo "未找到用户: $USER_LOGIN" >&2
  exit 1
fi

if [[ -n "$CHANNEL_ID" ]]; then
  CHANNEL_SQL_ID="${CHANNEL_ID//\'/\'\'}"
  PREVIEW_SQL="SELECT id, tenant_id, agent_id, platform, name, created_by, created_at FROM im_channels WHERE tenant_id = ${TENANT_ID} AND id = '${CHANNEL_SQL_ID}' AND deleted_at IS NULL;"
  run_sql "$PREVIEW_SQL"
  if [[ "$EXECUTE" == true ]]; then
    run_sql "UPDATE im_channels SET created_by = '${TARGET_USER_ID}' WHERE tenant_id = ${TENANT_ID} AND id = '${CHANNEL_SQL_ID}' AND deleted_at IS NULL;"
  else
    echo "[preview] UPDATE im_channels SET created_by = '${TARGET_USER_ID}' WHERE tenant_id = ${TENANT_ID} AND id = '${CHANNEL_SQL_ID}' AND deleted_at IS NULL;"
  fi
  exit 0
fi

AGENT_SQL_ID="${AGENT_ID//\'/\'\'}"
PREVIEW_SQL="SELECT id, tenant_id, agent_id, platform, name, created_by, created_at FROM im_channels WHERE tenant_id = ${TENANT_ID} AND agent_id = '${AGENT_SQL_ID}' AND deleted_at IS NULL AND COALESCE(created_by, '') = '' ORDER BY created_at ASC;"
run_sql "$PREVIEW_SQL"
if [[ "$EXECUTE" == true ]]; then
  run_sql "UPDATE im_channels SET created_by = '${TARGET_USER_ID}' WHERE tenant_id = ${TENANT_ID} AND agent_id = '${AGENT_SQL_ID}' AND deleted_at IS NULL AND COALESCE(created_by, '') = '';"
else
  echo "[preview] UPDATE im_channels SET created_by = '${TARGET_USER_ID}' WHERE tenant_id = ${TENANT_ID} AND agent_id = '${AGENT_SQL_ID}' AND deleted_at IS NULL AND COALESCE(created_by, '') = '';"
fi