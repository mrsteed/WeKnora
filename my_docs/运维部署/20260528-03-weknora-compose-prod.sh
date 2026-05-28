#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${WEKNORA_REPO_ROOT:-$(cd "${SCRIPT_DIR}/../.." && pwd)}"
CONFIG_FILE="${WEKNORA_COMPOSE_ENV:-${SCRIPT_DIR}/20260528-03-weknora-compose-prod.env}"

DOCKER_COMPOSE_BIN=""
DOCKER_COMPOSE_SUBCMD=""
DRY_RUN="${DRY_RUN:-false}"

log() { printf '[INFO] %s\n' "$*"; }
warn() { printf '[WARN] %s\n' "$*" >&2; }
die() { printf '[ERROR] %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'USAGE'
WeKnora Docker Compose production deploy script

Usage:
  bash 20260528-03-weknora-compose-prod.sh <command>

Config:
  Default config file:
    my_docs/运维部署/20260528-03-weknora-compose-prod.env
  Override:
    WEKNORA_COMPOSE_ENV=/path/to/env bash 20260528-03-weknora-compose-prod.sh <command>

Commands:
  init-config          Create a local Compose production config template
  check                Check Docker, Compose, repo layout and required config
  prepare-dir          Copy runtime files into COMPOSE_PROJECT_DIR
  render-env           Render COMPOSE_PROJECT_DIR/.env
  render-override      Render COMPOSE_PROJECT_DIR/docker-compose.prod.override.yml
  pull                 Pull images through Compose
  deploy               Render env/override, pull if enabled, then docker compose up -d
  online               check, prepare-dir, deploy, verify
  verify               Check Compose status, logs, app health and frontend entry
  status               Show Compose service status
  logs                 Follow recent app/docreader/frontend logs
  backup-db            Dump PostgreSQL with pg_dump -Fc
  migrate              Run migration helper inside app container
  restart              Restart Compose services
  stop                 Stop Compose services
  rollback             Set ROLLBACK_VERSION, render old images, deploy and verify
USAGE
}

write_default_config() {
  local target="$CONFIG_FILE"
  if [[ -e "$target" && "${FORCE:-false}" != "true" ]]; then
    die "Config already exists: ${target}. Set FORCE=true to overwrite."
  fi
  umask 077
  cat > "$target" <<'EOF'
# WeKnora Docker Compose production config
# Replace every change-me-* value before deploy.

REGISTRY=registry.example.com/weknora
WEKNORA_VERSION=0.6.0-20260528.1
COMPOSE_PROJECT_DIR=/opt/weknora
COMPOSE_PROFILES=
COMPOSE_PULL=true
DRY_RUN=false

DOMAIN=weknora.example.com
APP_EXTERNAL_URL=https://weknora.example.com
TZ=Asia/Shanghai
WEKNORA_LANGUAGE=zh-CN
DISABLE_REGISTRATION=true

DB_DRIVER=postgres
DB_HOST=postgres
DB_PORT=5432
DB_USER=weknora
DB_PASSWORD=change-me-strong-db-password
DB_NAME=weknora
RETRIEVE_DRIVER=postgres
AUTO_RECOVER_DIRTY=true

REDIS_PASSWORD=change-me-strong-redis-password
REDIS_DB=0
REDIS_PREFIX=stream:
STREAM_MANAGER_TYPE=redis

STORAGE_TYPE=minio
# Container-to-container endpoint. If using Compose built-in MinIO, keep minio:9000.
MINIO_ENDPOINT=minio:9000
MINIO_ACCESS_KEY_ID=change-me-minio-access-key
MINIO_SECRET_ACCESS_KEY=change-me-minio-secret-key
MINIO_BUCKET_NAME=weknora-prod
LOCAL_STORAGE_BASE_DIR=/data/files

FRONTEND_PORT=80
APP_PORT=8080
APP_BACKEND_PORT=8080
APP_HOST=app
APP_SCHEME=http
DOCREADER_ADDR=docreader:50051
DOCREADER_TRANSPORT=grpc
MAX_FILE_SIZE_MB=50
CONCURRENCY_POOL_SIZE=5

JWT_SECRET=change-me-openssl-rand-base64-48
TENANT_AES_KEY=change-me-keep-stable-value
SYSTEM_AES_KEY=change-me-exactly-32-byte-value
CRYPTO_MASTER_KEY=change-me-openssl-rand-hex-32
CRYPTO_SALT=change-me-openssl-rand-base64-32

WEKNORA_SANDBOX_MODE=docker
WEKNORA_SANDBOX_TIMEOUT=60

# Optional model / observability settings
OLLAMA_BASE_URL=http://host.docker.internal:11434
LANGFUSE_ENABLED=
LANGFUSE_HOST=
LANGFUSE_PUBLIC_KEY=
LANGFUSE_SECRET_KEY=

# Backup settings
COMPOSE_BACKUP_DIR=/opt/weknora/backups
BACKUP_BEFORE_DEPLOY=false

# Rollback usage:
# ROLLBACK_VERSION=0.6.0-previous bash ... rollback
ROLLBACK_VERSION=
EOF
  chmod 600 "$target"
  log "Created config: ${target}"
}

load_config() {
  local env_dry_run="${DRY_RUN:-}"
  local env_rollback_version="${ROLLBACK_VERSION:-}"
  local env_weknora_version="${WEKNORA_VERSION:-}"
  if [[ -f "$CONFIG_FILE" ]]; then
    set -a
    # shellcheck disable=SC1090
    . "$CONFIG_FILE"
    set +a
  else
    warn "Config file not found: ${CONFIG_FILE}"
    warn "Run: bash ${BASH_SOURCE[0]} init-config"
  fi
  [[ -n "$env_dry_run" && "$env_dry_run" != "false" ]] && DRY_RUN="$env_dry_run"
  [[ -n "$env_rollback_version" ]] && ROLLBACK_VERSION="$env_rollback_version"
  [[ -n "$env_weknora_version" ]] && WEKNORA_VERSION="$env_weknora_version"
  DRY_RUN="${DRY_RUN:-false}"
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "Missing required command: $1"
}

is_placeholder() {
  local value="${1:-}"
  [[ -z "$value" || "$value" == change-me* || "$value" == *'<'* || "$value" == *'>'* ]]
}

require_real_env() {
  local name value
  for name in "$@"; do
    value="${!name:-}"
    if is_placeholder "$value"; then
      die "Required config ${name} is empty or still a placeholder"
    fi
  done
}

run() {
  if [[ "${DRY_RUN:-false}" == "true" ]]; then
    printf '[DRY-RUN]'
    printf ' %q' "$@"
    printf '\n'
  else
    "$@"
  fi
}

ensure_repo_root() {
  [[ -f "${REPO_ROOT}/docker-compose.yml" ]] || die "docker-compose.yml not found under REPO_ROOT: ${REPO_ROOT}"
  [[ -d "${REPO_ROOT}/config" ]] || die "config directory not found under REPO_ROOT: ${REPO_ROOT}"
  [[ -d "${REPO_ROOT}/scripts" ]] || die "scripts directory not found under REPO_ROOT: ${REPO_ROOT}"
}

detect_compose() {
  if docker compose version >/dev/null 2>&1; then
    DOCKER_COMPOSE_BIN="docker"
    DOCKER_COMPOSE_SUBCMD="compose"
    return 0
  fi
  if command -v docker-compose >/dev/null 2>&1 && docker-compose version >/dev/null 2>&1; then
    DOCKER_COMPOSE_BIN="docker-compose"
    DOCKER_COMPOSE_SUBCMD=""
    return 0
  fi
  return 1
}

version_value() {
  if [[ -n "${1:-}" ]]; then
    printf '%s' "$1"
  elif [[ -n "${WEKNORA_VERSION:-}" ]]; then
    printf '%s' "$WEKNORA_VERSION"
  elif [[ -f "${REPO_ROOT}/VERSION" ]]; then
    tr -d '\n\r' < "${REPO_ROOT}/VERSION"
  else
    printf 'latest'
  fi
}

resolve_images() {
  ensure_repo_root
  require_real_env REGISTRY
  VERSION="$(version_value "${1:-}")"
  APP_IMAGE="${REGISTRY}/weknora-app:${VERSION}"
  DOCREADER_IMAGE="${REGISTRY}/weknora-docreader:${VERSION}"
  FRONTEND_IMAGE="${REGISTRY}/weknora-ui:${VERSION}"
  SANDBOX_IMAGE="${REGISTRY}/weknora-sandbox:${VERSION}"
}

compose_profile_args() {
  COMPOSE_PROFILE_ARGS=()
  local profiles="${COMPOSE_PROFILES:-}"
  local item
  if [[ -n "$profiles" ]]; then
    IFS=',' read -r -a _profiles <<< "$profiles"
    for item in "${_profiles[@]}"; do
      item="${item// /}"
      [[ -n "$item" ]] && COMPOSE_PROFILE_ARGS+=(--profile "$item")
    done
  fi
}

compose_file_args() {
  local project_dir="${COMPOSE_PROJECT_DIR:-$REPO_ROOT}"
  COMPOSE_FILE_ARGS=(-f docker-compose.yml)
  if [[ -f "${project_dir}/docker-compose.prod.override.yml" ]]; then
    COMPOSE_FILE_ARGS+=(-f docker-compose.prod.override.yml)
  fi
}

compose_run() {
  detect_compose || die "Docker Compose is not available"
  compose_profile_args
  compose_file_args
  local project_dir="${COMPOSE_PROJECT_DIR:-$REPO_ROOT}"
  if [[ "${DRY_RUN:-false}" == "true" ]]; then
    printf '[DRY-RUN] (cd %q && %q' "$project_dir" "$DOCKER_COMPOSE_BIN"
    [[ -n "$DOCKER_COMPOSE_SUBCMD" ]] && printf ' %q' "$DOCKER_COMPOSE_SUBCMD"
    printf ' %q' "${COMPOSE_FILE_ARGS[@]}" "${COMPOSE_PROFILE_ARGS[@]}" "$@"
    printf ')\n'
    return 0
  fi
  if [[ -n "$DOCKER_COMPOSE_SUBCMD" ]]; then
    (cd "$project_dir" && "$DOCKER_COMPOSE_BIN" "$DOCKER_COMPOSE_SUBCMD" "${COMPOSE_FILE_ARGS[@]}" "${COMPOSE_PROFILE_ARGS[@]}" "$@")
  else
    (cd "$project_dir" && "$DOCKER_COMPOSE_BIN" "${COMPOSE_FILE_ARGS[@]}" "${COMPOSE_PROFILE_ARGS[@]}" "$@")
  fi
}

cmd_check() {
  ensure_repo_root
  require_cmd docker
  require_cmd curl
  docker info >/dev/null 2>&1 || die "Docker daemon is not running"
  detect_compose || die "Docker Compose is not available"
  resolve_images
  log "Repository        : ${REPO_ROOT}"
  log "Config            : ${CONFIG_FILE}"
  log "Compose directory : ${COMPOSE_PROJECT_DIR:-$REPO_ROOT}"
  log "Compose command   : ${DOCKER_COMPOSE_BIN} ${DOCKER_COMPOSE_SUBCMD}"
  log "Version           : ${VERSION}"
  log "App image         : ${APP_IMAGE}"
  log "Docreader image   : ${DOCREADER_IMAGE}"
  log "Frontend image    : ${FRONTEND_IMAGE}"
  log "Sandbox image     : ${SANDBOX_IMAGE}"
}

copy_path() {
  local src="$1"
  local dst="$2"
  [[ -e "$src" ]] || return 0
  if [[ "${DRY_RUN:-false}" == "true" ]]; then
    log "[DRY-RUN] copy ${src} -> ${dst}"
    return 0
  fi
  rm -rf "$dst"
  mkdir -p "$(dirname "$dst")"
  cp -a "$src" "$dst"
}

cmd_prepare_dir() {
  ensure_repo_root
  local target="${COMPOSE_PROJECT_DIR:-$REPO_ROOT}"
  if [[ "$target" == "$REPO_ROOT" ]]; then
    log "COMPOSE_PROJECT_DIR is repository root; skip copy"
    return 0
  fi
  run mkdir -p "$target"
  copy_path "${REPO_ROOT}/docker-compose.yml" "${target}/docker-compose.yml"
  copy_path "${REPO_ROOT}/.env.example" "${target}/.env.example"
  copy_path "${REPO_ROOT}/config" "${target}/config"
  copy_path "${REPO_ROOT}/scripts" "${target}/scripts"
  copy_path "${REPO_ROOT}/migrations" "${target}/migrations"
  copy_path "${REPO_ROOT}/skills" "${target}/skills"
  copy_path "${REPO_ROOT}/docker/searxng" "${target}/docker/searxng"
  log "Prepared Compose directory: ${target}"
}

cmd_render_env() {
  resolve_images
  require_real_env DB_USER DB_PASSWORD DB_NAME REDIS_PASSWORD JWT_SECRET TENANT_AES_KEY SYSTEM_AES_KEY CRYPTO_MASTER_KEY CRYPTO_SALT
  require_real_env MINIO_ACCESS_KEY_ID MINIO_SECRET_ACCESS_KEY MINIO_BUCKET_NAME
  local target="${COMPOSE_PROJECT_DIR:-$REPO_ROOT}"
  run mkdir -p "$target"
  if [[ "${DRY_RUN:-false}" == "true" ]]; then
    log "[DRY-RUN] render ${target}/.env"
    return 0
  fi
  umask 077
  cat > "${target}/.env" <<EOF
WEKNORA_VERSION=${VERSION}
GIN_MODE=release
DISABLE_REGISTRATION=${DISABLE_REGISTRATION:-true}
TZ=${TZ:-Asia/Shanghai}
WEKNORA_LANGUAGE=${WEKNORA_LANGUAGE:-zh-CN}

DB_DRIVER=${DB_DRIVER:-postgres}
DB_HOST=${DB_HOST:-postgres}
DB_PORT=${DB_PORT:-5432}
DB_USER=${DB_USER}
DB_PASSWORD=${DB_PASSWORD}
DB_NAME=${DB_NAME}
RETRIEVE_DRIVER=${RETRIEVE_DRIVER:-postgres}
AUTO_RECOVER_DIRTY=${AUTO_RECOVER_DIRTY:-true}

REDIS_PASSWORD=${REDIS_PASSWORD}
REDIS_DB=${REDIS_DB:-0}
REDIS_PREFIX=${REDIS_PREFIX:-stream:}
STREAM_MANAGER_TYPE=${STREAM_MANAGER_TYPE:-redis}

STORAGE_TYPE=${STORAGE_TYPE:-minio}
MINIO_ENDPOINT=${MINIO_ENDPOINT:-minio:9000}
MINIO_ACCESS_KEY_ID=${MINIO_ACCESS_KEY_ID}
MINIO_SECRET_ACCESS_KEY=${MINIO_SECRET_ACCESS_KEY}
MINIO_BUCKET_NAME=${MINIO_BUCKET_NAME}
LOCAL_STORAGE_BASE_DIR=${LOCAL_STORAGE_BASE_DIR:-/data/files}

FRONTEND_PORT=${FRONTEND_PORT:-80}
APP_PORT=${APP_PORT:-8080}
APP_BACKEND_PORT=${APP_BACKEND_PORT:-8080}
APP_HOST=${APP_HOST:-app}
APP_SCHEME=${APP_SCHEME:-http}
APP_EXTERNAL_URL=${APP_EXTERNAL_URL:-https://${DOMAIN:-weknora.example.com}}

DOCREADER_ADDR=${DOCREADER_ADDR:-docreader:50051}
DOCREADER_TRANSPORT=${DOCREADER_TRANSPORT:-grpc}
MAX_FILE_SIZE_MB=${MAX_FILE_SIZE_MB:-50}
CONCURRENCY_POOL_SIZE=${CONCURRENCY_POOL_SIZE:-5}

JWT_SECRET=${JWT_SECRET}
TENANT_AES_KEY=${TENANT_AES_KEY}
SYSTEM_AES_KEY=${SYSTEM_AES_KEY}
CRYPTO_MASTER_KEY=${CRYPTO_MASTER_KEY}
CRYPTO_SALT=${CRYPTO_SALT}

WEKNORA_SANDBOX_MODE=${WEKNORA_SANDBOX_MODE:-docker}
WEKNORA_SANDBOX_TIMEOUT=${WEKNORA_SANDBOX_TIMEOUT:-60}
WEKNORA_SANDBOX_DOCKER_IMAGE=${SANDBOX_IMAGE}

OLLAMA_BASE_URL=${OLLAMA_BASE_URL:-http://host.docker.internal:11434}
LANGFUSE_ENABLED=${LANGFUSE_ENABLED:-}
LANGFUSE_HOST=${LANGFUSE_HOST:-}
LANGFUSE_PUBLIC_KEY=${LANGFUSE_PUBLIC_KEY:-}
LANGFUSE_SECRET_KEY=${LANGFUSE_SECRET_KEY:-}
EOF
  chmod 600 "${target}/.env"
  log "Rendered ${target}/.env"
}

cmd_render_override() {
  resolve_images "${1:-}"
  local target="${COMPOSE_PROJECT_DIR:-$REPO_ROOT}"
  run mkdir -p "$target"
  if [[ "${DRY_RUN:-false}" == "true" ]]; then
    log "[DRY-RUN] render ${target}/docker-compose.prod.override.yml"
    return 0
  fi
  cat > "${target}/docker-compose.prod.override.yml" <<EOF
services:
  frontend:
    image: ${FRONTEND_IMAGE}
  app:
    image: ${APP_IMAGE}
    environment:
      - WEKNORA_SANDBOX_DOCKER_IMAGE=${SANDBOX_IMAGE}
  docreader:
    image: ${DOCREADER_IMAGE}
  sandbox:
    image: ${SANDBOX_IMAGE}
EOF
  log "Rendered ${target}/docker-compose.prod.override.yml"
}

cmd_pull() {
  compose_run pull
}

cmd_backup_db() {
  local target="${COMPOSE_PROJECT_DIR:-$REPO_ROOT}"
  if [[ -f "${target}/.env" ]]; then
    set -a
    # shellcheck disable=SC1090
    . "${target}/.env"
    set +a
  fi
  require_real_env DB_USER DB_NAME
  detect_compose || die "Docker Compose is not available"
  local backup_dir="${COMPOSE_BACKUP_DIR:-${target}/backups}"
  local backup_file="${backup_dir}/weknora_${DB_NAME}_$(date +%Y%m%d%H%M%S).dump"
  if [[ "${DRY_RUN:-false}" == "true" ]]; then
    log "[DRY-RUN] pg_dump to ${backup_file}"
    return 0
  fi
  mkdir -p "$backup_dir"
  compose_file_args
  compose_profile_args
  if [[ -n "$DOCKER_COMPOSE_SUBCMD" ]]; then
    (cd "$target" && "$DOCKER_COMPOSE_BIN" "$DOCKER_COMPOSE_SUBCMD" "${COMPOSE_FILE_ARGS[@]}" "${COMPOSE_PROFILE_ARGS[@]}" exec -T postgres pg_dump -U "$DB_USER" -d "$DB_NAME" -Fc) > "$backup_file"
  else
    (cd "$target" && "$DOCKER_COMPOSE_BIN" "${COMPOSE_FILE_ARGS[@]}" "${COMPOSE_PROFILE_ARGS[@]}" exec -T postgres pg_dump -U "$DB_USER" -d "$DB_NAME" -Fc) > "$backup_file"
  fi
  log "Wrote backup: ${backup_file}"
}

cmd_deploy() {
  cmd_render_env
  cmd_render_override
  if [[ "${BACKUP_BEFORE_DEPLOY:-false}" == "true" ]]; then
    cmd_backup_db || warn "Database backup failed or database is not running; continuing because this may be first deploy"
  fi
  if [[ "${COMPOSE_PULL:-true}" == "true" ]]; then
    cmd_pull
  fi
  compose_run up -d --remove-orphans
}

cmd_verify() {
  local target="${COMPOSE_PROJECT_DIR:-$REPO_ROOT}"
  if [[ -f "${target}/.env" ]]; then
    set -a
    # shellcheck disable=SC1090
    . "${target}/.env"
    set +a
  fi
  compose_run ps
  compose_run logs app --tail=120
  compose_run logs docreader --tail=80 || true
  run curl -fsS "http://127.0.0.1:${APP_PORT:-8080}/health"
  run curl -fsSI "http://127.0.0.1:${FRONTEND_PORT:-80}/"
}

cmd_status() {
  compose_run ps
}

cmd_logs() {
  compose_run logs --tail=200 -f app docreader frontend
}

cmd_migrate() {
  compose_run exec app /app/scripts/migrate.sh version || true
  compose_run exec app /app/scripts/migrate.sh up
}

cmd_restart() {
  compose_run restart
  cmd_verify
}

cmd_stop() {
  compose_run down --remove-orphans
}

cmd_rollback() {
  require_real_env ROLLBACK_VERSION
  local target="${COMPOSE_PROJECT_DIR:-$REPO_ROOT}"
  [[ -f "${target}/.env" ]] || die "${target}/.env does not exist"
  if [[ "${DRY_RUN:-false}" == "true" ]]; then
    log "[DRY-RUN] set WEKNORA_VERSION=${ROLLBACK_VERSION} in ${target}/.env"
  else
    if grep -q '^WEKNORA_VERSION=' "${target}/.env"; then
      sed -i.bak "s/^WEKNORA_VERSION=.*/WEKNORA_VERSION=${ROLLBACK_VERSION}/" "${target}/.env"
    else
      printf '\nWEKNORA_VERSION=%s\n' "$ROLLBACK_VERSION" >> "${target}/.env"
    fi
  fi
  cmd_render_override "$ROLLBACK_VERSION"
  if [[ "${COMPOSE_PULL:-true}" == "true" ]]; then
    cmd_pull
  fi
  compose_run up -d --remove-orphans
  cmd_verify
}

cmd_online() {
  cmd_check
  cmd_prepare_dir
  cmd_deploy
  cmd_verify
}

main() {
  local cmd="${1:-help}"
  if [[ "$cmd" == "init-config" ]]; then
    write_default_config
    return 0
  fi
  if [[ "$cmd" == "help" || "$cmd" == "-h" || "$cmd" == "--help" ]]; then
    usage
    return 0
  fi
  load_config
  case "$cmd" in
    check) cmd_check ;;
    prepare-dir) cmd_prepare_dir ;;
    render-env) cmd_render_env ;;
    render-override) cmd_render_override ;;
    pull) cmd_pull ;;
    deploy) cmd_deploy ;;
    online) cmd_online ;;
    verify) cmd_verify ;;
    status) cmd_status ;;
    logs) cmd_logs ;;
    backup-db) cmd_backup_db ;;
    migrate) cmd_migrate ;;
    restart) cmd_restart ;;
    stop) cmd_stop ;;
    rollback) cmd_rollback ;;
    *) usage; die "Unknown command: ${cmd}" ;;
  esac
}

main "$@"
