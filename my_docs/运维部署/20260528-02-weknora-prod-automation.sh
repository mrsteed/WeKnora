#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${WEKNORA_REPO_ROOT:-$(cd "${SCRIPT_DIR}/../.." && pwd)}"
CONFIG_FILE="${WEKNORA_AUTOMATION_ENV:-${SCRIPT_DIR}/20260528-02-weknora-prod-automation.env}"

DRY_RUN="${DRY_RUN:-false}"
REGISTRY="${REGISTRY:-}"
RELEASE_VERSION="${RELEASE_VERSION:-}"
PLATFORM="${PLATFORM:-}"
TARGETARCH="${TARGETARCH:-}"

DOCKER_COMPOSE_BIN=""
DOCKER_COMPOSE_SUBCMD=""

log() { printf '[INFO] %s\n' "$*"; }
warn() { printf '[WARN] %s\n' "$*" >&2; }
die() { printf '[ERROR] %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'USAGE'
WeKnora production automation script

Usage:
  bash 20260528-02-weknora-prod-automation.sh <command>

Config:
  Default config file:
    my_docs/运维部署/20260528-02-weknora-prod-automation.env
  Override:
    WEKNORA_AUTOMATION_ENV=/path/to/env bash 20260528-02-weknora-prod-automation.sh <command>

Build commands:
  check                 Check local tools and repository layout
  version               Print resolved release metadata
  test-source           Run source-level checks configured for release
  build-images          Build app/docreader/frontend/sandbox images with version tags
  push-images           Push versioned images to REGISTRY
  package-artifacts     Build binary/package artifacts and SHA256SUMS
  build-release         Run check, optional tests, build-images, package-artifacts
  publish-release       Run build-release then push-images

Docker Compose commands:
  prepare-compose-dir   Copy runtime files into COMPOSE_PROJECT_DIR
  render-compose-env    Render COMPOSE_PROJECT_DIR/.env from automation config
  deploy-compose        Start or upgrade Docker Compose deployment
  verify-compose        Check Compose containers and HTTP health endpoints
  backup-compose        Dump PostgreSQL from Compose deployment
  rollback-compose      Set WEKNORA_VERSION to ROLLBACK_VERSION and restart Compose

Kubernetes / Helm commands:
  generate-helm-values  Render Helm production values file
  apply-helm-secrets    Apply namespace, imagePullSecret, app Secret and TLS Secret
  deploy-helm           helm upgrade --install using generated values
  verify-helm           Check rollout status and Kubernetes resources
  backup-helm           Dump PostgreSQL from Helm deployment
  rollback-helm         helm rollback to HELM_ROLLBACK_REVISION

Composite commands:
  compose-online        prepare-compose-dir, render-compose-env, deploy-compose, verify-compose
  helm-online           generate-helm-values, apply-helm-secrets, deploy-helm, verify-helm
USAGE
}

load_config() {
  if [[ -f "$CONFIG_FILE" ]]; then
    set -a
    # shellcheck disable=SC1090
    . "$CONFIG_FILE"
    set +a
  else
    warn "Config file not found: $CONFIG_FILE"
    warn "Copy the .env.example next to this script and edit placeholders before deploy commands."
  fi
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "Missing required command: $1"
}

is_placeholder() {
  local value="${1:-}"
  [[ -z "$value" || "$value" == *'<'* || "$value" == *'>'* || "$value" == change-me* ]]
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
  [[ -f "${REPO_ROOT}/Makefile" ]] || die "REPO_ROOT is not a WeKnora repository: ${REPO_ROOT}"
  [[ -f "${REPO_ROOT}/docker-compose.yml" ]] || die "docker-compose.yml not found under REPO_ROOT"
  [[ -d "${REPO_ROOT}/helm" ]] || die "helm chart not found under REPO_ROOT"
}

detect_platform() {
  if [[ -z "${PLATFORM:-}" || -z "${TARGETARCH:-}" ]]; then
    case "$(uname -m)" in
      x86_64)
        PLATFORM="${PLATFORM:-linux/amd64}"
        TARGETARCH="${TARGETARCH:-amd64}"
        ;;
      aarch64|arm64)
        PLATFORM="${PLATFORM:-linux/arm64}"
        TARGETARCH="${TARGETARCH:-arm64}"
        ;;
      *)
        PLATFORM="${PLATFORM:-linux/amd64}"
        TARGETARCH="${TARGETARCH:-amd64}"
        warn "Unknown architecture $(uname -m), defaulting to ${PLATFORM}"
        ;;
    esac
  fi
}

resolve_release() {
  ensure_repo_root
  detect_platform
  local script_version version edition commit_id build_time go_version
  script_version="$(${REPO_ROOT}/scripts/get_version.sh env)"
  eval "$script_version"
  version="${RELEASE_VERSION:-${VERSION:-unknown}}"
  edition="${EDITION:-standard}"
  commit_id="$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)"
  build_time="$(date -u '+%Y-%m-%d %H:%M:%S UTC')"
  go_version="$(go version 2>/dev/null || echo unknown)"

  VERSION="$version"
  EDITION="$edition"
  COMMIT_ID="${COMMIT_ID:-$commit_id}"
  BUILD_TIME="${BUILD_TIME:-$build_time}"
  GO_VERSION="${GO_VERSION:-$go_version}"

  if is_placeholder "${REGISTRY:-}"; then
    die "REGISTRY must be set for production images"
  fi

  APP_IMAGE="${REGISTRY}/weknora-app:${VERSION}"
  DOCREADER_IMAGE="${REGISTRY}/weknora-docreader:${VERSION}"
  FRONTEND_IMAGE="${REGISTRY}/weknora-ui:${VERSION}"
  SANDBOX_IMAGE="${REGISTRY}/weknora-sandbox:${VERSION}"
  RELEASE_DIR="${REPO_ROOT}/dist/release-${VERSION}"
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

compose_args_with_profiles() {
  local profiles="${COMPOSE_PROFILES:-}"
  local item
  COMPOSE_PROFILE_ARGS=()
  if [[ -n "$profiles" ]]; then
    IFS=',' read -r -a _profiles <<< "$profiles"
    for item in "${_profiles[@]}"; do
      item="${item// /}"
      [[ -n "$item" ]] && COMPOSE_PROFILE_ARGS+=(--profile "$item")
    done
  fi
}

compose_run() {
  detect_compose || die "Docker Compose is not available"
  local project_dir="${COMPOSE_PROJECT_DIR:-$REPO_ROOT}"
  if [[ "${DRY_RUN:-false}" == "true" ]]; then
    printf '[DRY-RUN] (cd %q && %q' "$project_dir" "$DOCKER_COMPOSE_BIN"
    [[ -n "$DOCKER_COMPOSE_SUBCMD" ]] && printf ' %q' "$DOCKER_COMPOSE_SUBCMD"
    printf ' %q' "$@"
    printf ')\n'
    return 0
  fi
  if [[ -n "$DOCKER_COMPOSE_SUBCMD" ]]; then
    (cd "$project_dir" && "$DOCKER_COMPOSE_BIN" "$DOCKER_COMPOSE_SUBCMD" "$@")
  else
    (cd "$project_dir" && "$DOCKER_COMPOSE_BIN" "$@")
  fi
}

helm_fullname() {
  if [[ -n "${HELM_FULLNAME:-}" ]]; then
    printf '%s' "$HELM_FULLNAME"
  elif [[ "${HELM_RELEASE:-weknora}" == *weknora* ]]; then
    printf '%s' "${HELM_RELEASE:-weknora}"
  else
    printf '%s-weknora' "${HELM_RELEASE:-weknora}"
  fi
}

cmd_check() {
  ensure_repo_root
  require_cmd git
  require_cmd go
  require_cmd docker
  require_cmd openssl
  require_cmd curl
  docker info >/dev/null 2>&1 || die "Docker daemon is not running"
  detect_compose || warn "Docker Compose is not available; Compose deploy commands will fail"
  command -v node >/dev/null 2>&1 || warn "node not found; frontend local checks may fail"
  command -v npm >/dev/null 2>&1 || warn "npm not found; frontend local checks may fail"
  command -v python3 >/dev/null 2>&1 || warn "python3 not found; MCP packaging may fail"
  command -v helm >/dev/null 2>&1 || warn "helm not found; Helm deploy commands will fail"
  command -v kubectl >/dev/null 2>&1 || warn "kubectl not found; Kubernetes deploy commands will fail"
  resolve_release
  log "Repository : ${REPO_ROOT}"
  log "Config     : ${CONFIG_FILE}"
  log "Version    : ${VERSION}"
  log "Platform   : ${PLATFORM} (${TARGETARCH})"
  log "Registry   : ${REGISTRY}"
}

cmd_version() {
  resolve_release
  cat <<EOF
VERSION=${VERSION}
EDITION=${EDITION}
COMMIT_ID=${COMMIT_ID}
BUILD_TIME=${BUILD_TIME}
GO_VERSION=${GO_VERSION}
PLATFORM=${PLATFORM}
TARGETARCH=${TARGETARCH}
APP_IMAGE=${APP_IMAGE}
DOCREADER_IMAGE=${DOCREADER_IMAGE}
FRONTEND_IMAGE=${FRONTEND_IMAGE}
SANDBOX_IMAGE=${SANDBOX_IMAGE}
EOF
}

cmd_test_source() {
  ensure_repo_root
  if [[ "${RUN_SOURCE_TESTS:-false}" != "true" ]]; then
    log "RUN_SOURCE_TESTS is not true; skipping source checks"
    return 0
  fi
  run bash -lc "cd '${REPO_ROOT}' && go test ./..."
  run bash -lc "cd '${REPO_ROOT}/cli' && go test ./..."
  run bash -lc "cd '${REPO_ROOT}/client' && go test ./..."
  run bash -lc "cd '${REPO_ROOT}/frontend' && npm ci && npm run build"
  run bash -lc "cd '${REPO_ROOT}/mcp-server' && python3 test_module.py"
}

cmd_build_images() {
  require_cmd docker
  resolve_release
  docker info >/dev/null 2>&1 || die "Docker daemon is not running"

  log "Building ${APP_IMAGE}"
  run docker build \
    --platform "$PLATFORM" \
    --build-arg "GOPRIVATE_ARG=${GOPRIVATE:-}" \
    --build-arg "GOPROXY_ARG=${GOPROXY:-https://goproxy.cn,direct}" \
    --build-arg "GOSUMDB_ARG=${GOSUMDB:-off}" \
    --build-arg "VERSION_ARG=${VERSION}" \
    --build-arg "COMMIT_ID_ARG=${COMMIT_ID}" \
    --build-arg "BUILD_TIME_ARG=${BUILD_TIME}" \
    --build-arg "GO_VERSION_ARG=${GO_VERSION}" \
    --build-arg "APK_MIRROR_ARG=${APK_MIRROR_ARG:-}" \
    -f "${REPO_ROOT}/docker/Dockerfile.app" \
    -t "$APP_IMAGE" \
    "$REPO_ROOT"

  log "Building ${DOCREADER_IMAGE}"
  run docker build \
    --platform "$PLATFORM" \
    --build-arg "TARGETARCH=${TARGETARCH}" \
    --build-arg "APT_MIRROR=${APT_MIRROR:-}" \
    -f "${REPO_ROOT}/docker/Dockerfile.docreader" \
    -t "$DOCREADER_IMAGE" \
    "$REPO_ROOT"

  log "Building ${FRONTEND_IMAGE}"
  run docker build \
    --platform "$PLATFORM" \
    --build-arg "MAX_FILE_SIZE_MB=${MAX_FILE_SIZE_MB:-50}" \
    -f "${REPO_ROOT}/frontend/Dockerfile" \
    -t "$FRONTEND_IMAGE" \
    "${REPO_ROOT}/frontend"

  log "Building ${SANDBOX_IMAGE}"
  run docker build \
    --platform "$PLATFORM" \
    -f "${REPO_ROOT}/docker/Dockerfile.sandbox" \
    -t "$SANDBOX_IMAGE" \
    "$REPO_ROOT"
}

cmd_push_images() {
  resolve_release
  run docker push "$APP_IMAGE"
  run docker push "$DOCREADER_IMAGE"
  run docker push "$FRONTEND_IMAGE"
  run docker push "$SANDBOX_IMAGE"
}

cmd_package_artifacts() {
  resolve_release
  mkdir -p "${RELEASE_DIR}/bin" "${RELEASE_DIR}/cli" "${RELEASE_DIR}/mcp" "${RELEASE_DIR}/sdk" "${RELEASE_DIR}/checksums"

  log "Building App binary"
  run bash -lc "cd '${REPO_ROOT}' && make build-prod"
  run cp "${REPO_ROOT}/WeKnora" "${RELEASE_DIR}/bin/WeKnora_$(go env GOOS)_$(go env GOARCH)"

  if [[ "${PACKAGE_LITE:-true}" == "true" ]]; then
    log "Packaging Lite distribution"
    run bash -lc "cd '${REPO_ROOT}' && ./scripts/package-lite.sh '${VERSION}'"
    run bash -lc "cp '${REPO_ROOT}'/dist/WeKnora-lite_${VERSION}_*.tar.gz* '${RELEASE_DIR}/bin/' 2>/dev/null || true"
  fi

  log "Building CLI"
  run bash -lc "cd '${REPO_ROOT}/cli' && make build"
  run cp "${REPO_ROOT}/cli/bin/weknora" "${RELEASE_DIR}/cli/weknora_$(go env GOOS)_$(go env GOARCH)"

  if [[ "${PACKAGE_MCP:-true}" == "true" ]]; then
    log "Packaging MCP server"
    run bash -lc "cd '${REPO_ROOT}/mcp-server' && python3 -m pip install --upgrade build >/dev/null && python3 -m build"
    run bash -lc "cp '${REPO_ROOT}/mcp-server/dist/'* '${RELEASE_DIR}/mcp/'"
  fi

  if [[ "${PACKAGE_CLIENT_SDK:-true}" == "true" ]]; then
    log "Archiving Go client SDK"
    run bash -lc "cd '${REPO_ROOT}' && tar --exclude='.git' -czf '${RELEASE_DIR}/sdk/weknora-client-go_${VERSION}.tar.gz' client"
  fi

  if [[ "${DRY_RUN:-false}" == "true" ]]; then
    log "[DRY-RUN] skip writing release metadata and checksums"
    return 0
  fi

  log "Writing release metadata and checksums"
  cat > "${RELEASE_DIR}/release.env" <<EOF
VERSION=${VERSION}
EDITION=${EDITION}
COMMIT_ID=${COMMIT_ID}
BUILD_TIME=${BUILD_TIME}
GO_VERSION=${GO_VERSION}
APP_IMAGE=${APP_IMAGE}
DOCREADER_IMAGE=${DOCREADER_IMAGE}
FRONTEND_IMAGE=${FRONTEND_IMAGE}
SANDBOX_IMAGE=${SANDBOX_IMAGE}
EOF
  run bash -lc "cd '${RELEASE_DIR}' && find . -type f ! -path './checksums/*' -print0 | xargs -0 sha256sum > checksums/SHA256SUMS"
  log "Artifacts are under ${RELEASE_DIR}"
}

cmd_build_release() {
  cmd_check
  cmd_test_source
  cmd_build_images
  cmd_package_artifacts
}

cmd_publish_release() {
  cmd_build_release
  cmd_push_images
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

cmd_prepare_compose_dir() {
  ensure_repo_root
  local target="${COMPOSE_PROJECT_DIR:-$REPO_ROOT}"
  if [[ "$target" == "$REPO_ROOT" ]]; then
    log "COMPOSE_PROJECT_DIR is REPO_ROOT; no copy is needed"
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

cmd_render_compose_env() {
  resolve_release
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
MINIO_ENDPOINT=${MINIO_INTERNAL_ENDPOINT:-${MINIO_ENDPOINT:-minio:9000}}
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

cmd_deploy_compose() {
  resolve_release
  compose_args_with_profiles
  local target="${COMPOSE_PROJECT_DIR:-$REPO_ROOT}"
  [[ -f "${target}/.env" ]] || die "${target}/.env does not exist. Run render-compose-env first."
  if [[ "${COMPOSE_PULL:-false}" == "true" ]]; then
    compose_run "${COMPOSE_PROFILE_ARGS[@]}" pull
  fi
  compose_run "${COMPOSE_PROFILE_ARGS[@]}" up -d --remove-orphans
}

cmd_verify_compose() {
  local target="${COMPOSE_PROJECT_DIR:-$REPO_ROOT}"
  if [[ -f "${target}/.env" ]]; then
    set -a
    # shellcheck disable=SC1090
    . "${target}/.env"
    set +a
  fi
  compose_run ps
  compose_run logs app --tail=120
  run curl -fsS "http://127.0.0.1:${APP_PORT:-8080}/health"
  run curl -fsSI "http://127.0.0.1:${FRONTEND_PORT:-80}/"
}

cmd_backup_compose() {
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
  if [[ -n "$DOCKER_COMPOSE_SUBCMD" ]]; then
    (cd "$target" && "$DOCKER_COMPOSE_BIN" "$DOCKER_COMPOSE_SUBCMD" exec -T postgres pg_dump -U "$DB_USER" -d "$DB_NAME" -Fc) > "$backup_file"
  else
    (cd "$target" && "$DOCKER_COMPOSE_BIN" exec -T postgres pg_dump -U "$DB_USER" -d "$DB_NAME" -Fc) > "$backup_file"
  fi
  log "Wrote backup: ${backup_file}"
}

cmd_rollback_compose() {
  local target="${COMPOSE_PROJECT_DIR:-$REPO_ROOT}"
  require_real_env ROLLBACK_VERSION
  [[ -f "${target}/.env" ]] || die "${target}/.env does not exist"
  if [[ "${DRY_RUN:-false}" == "true" ]]; then
    log "[DRY-RUN] set WEKNORA_VERSION=${ROLLBACK_VERSION} in ${target}/.env and restart Compose"
  else
    if grep -q '^WEKNORA_VERSION=' "${target}/.env"; then
      sed -i.bak "s/^WEKNORA_VERSION=.*/WEKNORA_VERSION=${ROLLBACK_VERSION}/" "${target}/.env"
    else
      printf '\nWEKNORA_VERSION=%s\n' "$ROLLBACK_VERSION" >> "${target}/.env"
    fi
  fi
  cmd_deploy_compose
  cmd_verify_compose
}

cmd_generate_helm_values() {
  resolve_release
  require_real_env DOMAIN MINIO_ENDPOINT MINIO_ACCESS_KEY_ID MINIO_SECRET_ACCESS_KEY MINIO_BUCKET_NAME
  local file="${HELM_VALUES_FILE:-${SCRIPT_DIR}/values-production.yaml}"
  run mkdir -p "$(dirname "$file")"
  if [[ "${DRY_RUN:-false}" == "true" ]]; then
    log "[DRY-RUN] render Helm values to ${file}"
    return 0
  fi
  cat > "$file" <<EOF
global:
  storageClass: ${HELM_STORAGE_CLASS:-}
  imagePullSecrets:
    - name: ${HELM_IMAGE_PULL_SECRET:-regcred}

app:
  replicaCount: ${HELM_APP_REPLICAS:-2}
  image:
    repository: ${REGISTRY}/weknora-app
    tag: ${VERSION}
    pullPolicy: IfNotPresent
  env:
    GIN_MODE: release
    RETRIEVE_DRIVER: ${RETRIEVE_DRIVER:-postgres}
    STORAGE_TYPE: ${STORAGE_TYPE:-minio}
    LOCAL_STORAGE_BASE_DIR: ${LOCAL_STORAGE_BASE_DIR:-/data/files}
    STREAM_MANAGER_TYPE: ${STREAM_MANAGER_TYPE:-redis}
    AUTO_RECOVER_DIRTY: "${AUTO_RECOVER_DIRTY:-true}"
    CONCURRENCY_POOL_SIZE: "${CONCURRENCY_POOL_SIZE:-5}"
    ENABLE_GRAPH_RAG: "${ENABLE_GRAPH_RAG:-false}"
    TZ: ${TZ:-Asia/Shanghai}
  extraEnv:
    - name: DISABLE_REGISTRATION
      value: "${DISABLE_REGISTRATION:-true}"
    - name: WEKNORA_LANGUAGE
      value: ${WEKNORA_LANGUAGE:-zh-CN}
    - name: DOCREADER_TRANSPORT
      value: ${DOCREADER_TRANSPORT:-grpc}
    - name: APP_EXTERNAL_URL
      value: ${APP_EXTERNAL_URL:-https://${DOMAIN}}
    - name: WEKNORA_SANDBOX_MODE
      value: ${WEKNORA_SANDBOX_MODE:-docker}
    - name: WEKNORA_SANDBOX_TIMEOUT
      value: "${WEKNORA_SANDBOX_TIMEOUT:-60}"
    - name: WEKNORA_SANDBOX_DOCKER_IMAGE
      value: ${SANDBOX_IMAGE}
    - name: CRYPTO_MASTER_KEY
      valueFrom:
        secretKeyRef:
          name: weknora-secrets
          key: CRYPTO_MASTER_KEY
    - name: CRYPTO_SALT
      valueFrom:
        secretKeyRef:
          name: weknora-secrets
          key: CRYPTO_SALT
    - name: OLLAMA_BASE_URL
      value: ${OLLAMA_BASE_URL:-http://ollama.ollama.svc.cluster.local:11434}
  resources:
    requests:
      cpu: 500m
      memory: 1Gi
    limits:
      cpu: "2"
      memory: 4Gi

frontend:
  replicaCount: ${HELM_FRONTEND_REPLICAS:-2}
  image:
    repository: ${REGISTRY}/weknora-ui
    tag: ${VERSION}
    pullPolicy: IfNotPresent
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 512Mi

docreader:
  replicaCount: ${HELM_DOCREADER_REPLICAS:-1}
  image:
    repository: ${REGISTRY}/weknora-docreader
    tag: ${VERSION}
    pullPolicy: IfNotPresent
  env:
    STORAGE_TYPE: ${STORAGE_TYPE:-minio}
  resources:
    requests:
      cpu: 500m
      memory: 1Gi
    limits:
      cpu: "2"
      memory: 4Gi

postgresql:
  enabled: true
  persistence:
    enabled: true
    size: ${HELM_POSTGRES_SIZE:-100Gi}

redis:
  enabled: true
  persistence:
    enabled: true
    size: ${HELM_REDIS_SIZE:-10Gi}

minio:
  enabled: true
  accessKeyId: ${MINIO_ACCESS_KEY_ID}
  accessKeySecret: ${MINIO_SECRET_ACCESS_KEY}
  endpoint: ${MINIO_ENDPOINT}
  publicEndpoint: ${MINIO_ENDPOINT}
  bucketName: ${MINIO_BUCKET_NAME}
  pathPrefix: ${MINIO_PATH_PREFIX:-prod}
  useSSL: ${MINIO_USE_SSL:-true}

ingress:
  enabled: true
  className: ${HELM_INGRESS_CLASS:-nginx}
  host: ${DOMAIN}
  tls:
    enabled: true
    secretName: ${HELM_TLS_SECRET:-weknora-tls}

secrets:
  existingSecret: weknora-secrets
EOF
  chmod 600 "$file"
  log "Rendered Helm values: ${file}"
}

cmd_apply_helm_secrets() {
  require_cmd kubectl
  require_real_env HELM_NAMESPACE DB_USER DB_PASSWORD DB_NAME REDIS_PASSWORD JWT_SECRET TENANT_AES_KEY SYSTEM_AES_KEY CRYPTO_MASTER_KEY CRYPTO_SALT
  if [[ "${DRY_RUN:-false}" == "true" ]]; then
    log "[DRY-RUN] apply namespace ${HELM_NAMESPACE}"
  else
    kubectl create namespace "$HELM_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
  fi

  local tmp_dir
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "$tmp_dir"' RETURN
  printf '%s' "$DB_USER" > "${tmp_dir}/DB_USER"
  printf '%s' "$DB_PASSWORD" > "${tmp_dir}/DB_PASSWORD"
  printf '%s' "$DB_NAME" > "${tmp_dir}/DB_NAME"
  printf '%s' "${REDIS_USERNAME:-}" > "${tmp_dir}/REDIS_USERNAME"
  printf '%s' "$REDIS_PASSWORD" > "${tmp_dir}/REDIS_PASSWORD"
  printf '%s' "$JWT_SECRET" > "${tmp_dir}/JWT_SECRET"
  printf '%s' "$TENANT_AES_KEY" > "${tmp_dir}/TENANT_AES_KEY"
  printf '%s' "$SYSTEM_AES_KEY" > "${tmp_dir}/SYSTEM_AES_KEY"
  printf '%s' "$CRYPTO_MASTER_KEY" > "${tmp_dir}/CRYPTO_MASTER_KEY"
  printf '%s' "$CRYPTO_SALT" > "${tmp_dir}/CRYPTO_SALT"

  if [[ "${DRY_RUN:-false}" == "true" ]]; then
    log "[DRY-RUN] apply generic secret weknora-secrets in namespace ${HELM_NAMESPACE}"
  else
    kubectl -n "$HELM_NAMESPACE" create secret generic weknora-secrets \
      --from-file="DB_USER=${tmp_dir}/DB_USER" \
      --from-file="DB_PASSWORD=${tmp_dir}/DB_PASSWORD" \
      --from-file="DB_NAME=${tmp_dir}/DB_NAME" \
      --from-file="REDIS_USERNAME=${tmp_dir}/REDIS_USERNAME" \
      --from-file="REDIS_PASSWORD=${tmp_dir}/REDIS_PASSWORD" \
      --from-file="JWT_SECRET=${tmp_dir}/JWT_SECRET" \
      --from-file="TENANT_AES_KEY=${tmp_dir}/TENANT_AES_KEY" \
      --from-file="SYSTEM_AES_KEY=${tmp_dir}/SYSTEM_AES_KEY" \
      --from-file="CRYPTO_MASTER_KEY=${tmp_dir}/CRYPTO_MASTER_KEY" \
      --from-file="CRYPTO_SALT=${tmp_dir}/CRYPTO_SALT" \
      --dry-run=client -o yaml | kubectl apply -f -
  fi

  if [[ -n "${REGISTRY_USERNAME:-}" && -n "${REGISTRY_PASSWORD:-}" ]]; then
    if [[ "${DRY_RUN:-false}" == "true" ]]; then
      log "[DRY-RUN] apply imagePullSecret ${HELM_IMAGE_PULL_SECRET:-regcred} in namespace ${HELM_NAMESPACE}"
    else
      kubectl -n "$HELM_NAMESPACE" create secret docker-registry "${HELM_IMAGE_PULL_SECRET:-regcred}" \
        --docker-server="${REGISTRY%%/*}" \
        --docker-username="$REGISTRY_USERNAME" \
        --docker-password="$REGISTRY_PASSWORD" \
        --dry-run=client -o yaml | kubectl apply -f -
    fi
  else
    warn "REGISTRY_USERNAME/REGISTRY_PASSWORD not set; skipping imagePullSecret automation"
  fi

  if [[ -n "${TLS_CERT_FILE:-}" && -n "${TLS_KEY_FILE:-}" ]]; then
    if [[ "${DRY_RUN:-false}" == "true" ]]; then
      log "[DRY-RUN] apply TLS secret ${HELM_TLS_SECRET:-weknora-tls} in namespace ${HELM_NAMESPACE}"
    else
      kubectl -n "$HELM_NAMESPACE" create secret tls "${HELM_TLS_SECRET:-weknora-tls}" \
        --cert="$TLS_CERT_FILE" \
        --key="$TLS_KEY_FILE" \
        --dry-run=client -o yaml | kubectl apply -f -
    fi
  else
    warn "TLS_CERT_FILE/TLS_KEY_FILE not set; skipping TLS secret automation"
  fi
}

cmd_deploy_helm() {
  require_cmd helm
  require_cmd kubectl
  local file="${HELM_VALUES_FILE:-${SCRIPT_DIR}/values-production.yaml}"
  [[ -f "$file" ]] || die "Helm values file not found: ${file}. Run generate-helm-values first."
  run helm lint "${REPO_ROOT}/helm" -f "$file"
  run helm upgrade --install "${HELM_RELEASE:-weknora}" "${REPO_ROOT}/helm" \
    -n "${HELM_NAMESPACE:-weknora}" \
    -f "$file"
}

cmd_verify_helm() {
  require_cmd kubectl
  local ns="${HELM_NAMESPACE:-weknora}"
  local full
  full="$(helm_fullname)"
  run kubectl -n "$ns" rollout status "deploy/${full}-app" --timeout=300s
  run kubectl -n "$ns" rollout status "deploy/${full}-frontend" --timeout=300s
  run kubectl -n "$ns" rollout status "deploy/${full}-docreader" --timeout=300s
  run kubectl -n "$ns" get pods,svc,ingress,pvc
  run kubectl -n "$ns" logs "deploy/${full}-app" --tail=120
  if [[ -n "${APP_EXTERNAL_URL:-}" ]]; then
    run curl -fsSI "${APP_EXTERNAL_URL}/"
  fi
}

cmd_backup_helm() {
  require_cmd kubectl
  require_real_env DB_USER DB_NAME
  local ns="${HELM_NAMESPACE:-weknora}"
  local full backup_dir backup_file
  full="$(helm_fullname)"
  backup_dir="${K8S_BACKUP_DIR:-./backups}"
  backup_file="${backup_dir}/weknora_${DB_NAME}_$(date +%Y%m%d%H%M%S).dump"
  if [[ "${DRY_RUN:-false}" == "true" ]]; then
    log "[DRY-RUN] kubectl exec deploy/${full}-postgres pg_dump to ${backup_file}"
    return 0
  fi
  mkdir -p "$backup_dir"
  kubectl -n "$ns" exec "deploy/${full}-postgres" -- pg_dump -U "$DB_USER" -d "$DB_NAME" -Fc > "$backup_file"
  log "Wrote backup: ${backup_file}"
}

cmd_rollback_helm() {
  require_cmd helm
  require_real_env HELM_ROLLBACK_REVISION
  run helm -n "${HELM_NAMESPACE:-weknora}" rollback "${HELM_RELEASE:-weknora}" "$HELM_ROLLBACK_REVISION"
  cmd_verify_helm
}

cmd_compose_online() {
  cmd_prepare_compose_dir
  cmd_render_compose_env
  cmd_deploy_compose
  cmd_verify_compose
}

cmd_helm_online() {
  cmd_generate_helm_values
  cmd_apply_helm_secrets
  cmd_deploy_helm
  cmd_verify_helm
}

main() {
  load_config
  local cmd="${1:-help}"
  case "$cmd" in
    help|-h|--help) usage ;;
    check) cmd_check ;;
    version) cmd_version ;;
    test-source) cmd_test_source ;;
    build-images) cmd_build_images ;;
    push-images) cmd_push_images ;;
    package-artifacts) cmd_package_artifacts ;;
    build-release) cmd_build_release ;;
    publish-release) cmd_publish_release ;;
    prepare-compose-dir) cmd_prepare_compose_dir ;;
    render-compose-env) cmd_render_compose_env ;;
    deploy-compose) cmd_deploy_compose ;;
    verify-compose) cmd_verify_compose ;;
    backup-compose) cmd_backup_compose ;;
    rollback-compose) cmd_rollback_compose ;;
    generate-helm-values) cmd_generate_helm_values ;;
    apply-helm-secrets) cmd_apply_helm_secrets ;;
    deploy-helm) cmd_deploy_helm ;;
    verify-helm) cmd_verify_helm ;;
    backup-helm) cmd_backup_helm ;;
    rollback-helm) cmd_rollback_helm ;;
    compose-online) cmd_compose_online ;;
    helm-online) cmd_helm_online ;;
    *) usage; die "Unknown command: ${cmd}" ;;
  esac
}

main "$@"
