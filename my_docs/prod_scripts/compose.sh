#!/usr/bin/env bash
set -euo pipefail

#
# WeKnora PROD 运维脚本核心（参照 BeadForge/deploy_prod/scripts/compose.sh 风格）
#
# 依据: my_docs/20260929-02-生产模式编译启动与运维命令方案.md
#   - 生产模式 = docker-compose.yml（app/frontend/postgres/redis/docreader 默认五服务）
#   - 与 dev 套容器（docker-compose.dev.yml, WeKnora-*-dev）互斥，5432/6379 端口冲突
#   - 自建镜像必须不 pull（start_all.sh 默认 --pull always 会拉官方镜像覆盖本地 latest）
#   - app 容器内固定监听 8080，宿主机端口 = ${APP_PORT:-8080}；frontend 对外 ${FRONTEND_PORT:-80}
#
# 仓库根查找: 本目录 my_docs/prod_scripts/ → 上 3 级 = 仓库根（可 WEKNORA_REPO_ROOT 覆盖）

TAG="[weknora-prod]"

log() { printf '%s %s\n' "$TAG" "$*"; }
warn() { printf '%s WARN: %s\n' "$TAG" "$*" >&2; }
die() { printf '%s ERROR: %s\n' "$TAG" "$*" >&2; exit 1; }

# ---------- 路径 ----------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
REPO_ROOT="${WEKNORA_REPO_ROOT:-$(cd "${SCRIPT_DIR}/../../.." && pwd)}"

if [[ -n "${WEKNORA_REPO_ROOT:-}" ]]; then
  log "repo root (WEKNORA_REPO_ROOT): ${REPO_ROOT}"
elif [[ -f "${REPO_ROOT}/docker-compose.yml" ]]; then
  :
else
  die "repo root 不对（${REPO_ROOT} 下无 docker-compose.yml），请 export WEKNORA_REPO_ROOT=<WeKnora 仓库根>"
fi
cd "${REPO_ROOT}"
# 固定生产编排文件，避免同目录下 docker-compose.override.yml / .dev.yml 干扰
export COMPOSE_FILE="docker-compose.yml"
# 独立 project 名：与同目录 dev 环境（project=目录名 weknora）隔离，使 ps/logs/down 只命中
# 生产容器，且 down -v 绝不会误删 dev 的卷；容器名仍为 WeKnora-*（container_name 固定，与 project 无关）。
export COMPOSE_PROJECT_NAME="weknora-prod"

# ---------- compose 命令检测（同 BeadForge 口径） ----------
DC=()
if docker compose version >/dev/null 2>&1; then
  DC=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  DC=(docker-compose)
else
  die "未检测到 docker compose / docker-compose"
fi

# ---------- 通用服务集 ----------
# 默认集合（docker compose config --services 实测 = redis docreader postgres app frontend）
CORE_SERVICES=(postgres redis docreader app frontend)
# 全部可选 profile（与 docker-compose.yml 一致；odl-hybrid/full/sandbox 按需）
PROFILES="minio qdrant milvus weaviate neo4j doris dex searxng opensearch langfuse odl-hybrid sandbox"

# ---------- 前置检查 ----------
check_env_file() {
  if [[ ! -f .env ]]; then
    if [[ -f .env.example ]]; then
      cp .env.example .env
      warn ".env 缺失，已从 .env.example 复制生成 —— 上线前请逐项核对 DB_*/密钥/LLM/STORAGE 配置！"
    else
      die ".env 不存在且无 .env.example；compose env_file 强依赖 .env"
    fi
  fi
}

check_docker() {
  command -v docker >/dev/null 2>&1 || die "未安装 docker"
  docker info >/dev/null 2>&1 || die "docker daemon 未运行"
  detect_compose 2>/dev/null || true
}

detect_compose() {
  DC=()
  if docker compose version >/dev/null 2>&1; then
    DC=(docker compose)
  elif command -v docker-compose >/dev/null 2>&1; then
    DC=(docker-compose)
  fi
  [[ ${#DC[@]} -gt 0 ]]
}

port_in_use() { # $1=port
  ss -ltn 2>/dev/null | awk '{print $4}' | grep -Eq "[:.]$1\$"
}

check_dev_conflict() {
  # dev 套容器（docker-compose.dev.yml）在跑会抢 5432/6379 等端口
  local dev_up
  dev_up="$(docker ps --format '{{.Names}}' 2>/dev/null | grep -c 'WeKnora-postgres-dev\|WeKnora-redis-dev' || true)"
  if [[ "${dev_up}" -gt 0 ]]; then
    warn "检测到 dev 基础设施容器在运行（WeKnora-postgres-dev/redis-dev），生产容器会因 5432/6379 端口冲突起不来；请先: cd <repo> && make dev-stop"
    return 1
  fi
  return 0
}

check_host_ports() {
  local fp
  # 端口以 .env 为准（compose 未起时读不到 env，直接 grep .env）
  fp="$(grep -E '^FRONTEND_PORT=' .env 2>/dev/null | cut -d= -f2 || true)"
  fp="${fp:-80}"
  local ap
  ap="$(grep -E '^APP_PORT=' .env 2>/dev/null | cut -d= -f2 || true)"
  ap="${ap:-8080}"
  local fail=0
  if port_in_use "${fp}"; then
    warn "宿主机端口 ${fp}（frontend 对外）已被占用，up 后 WeKnora-frontend 将起不来。请在 .env 改 FRONTEND_PORT 或释放端口"
    fail=1
  fi
  if port_in_use "${ap}"; then
    warn "宿主机端口 ${ap}（app API 对外映射）已被占用，up 后 WeKnora-app 将起不来。请在 .env 改 APP_PORT 或释放端口"
    fail=1
  fi
  return "${fail}"
}

# ---------- 健康等待 ----------
wait_healthy() { # $1=服务名 $2=超时秒
  local svc="$1" timeout="${2:-180}" waited=0
  log "等待 ${svc} 变为 healthy (最长 ${timeout}s)..."
  while (( waited < timeout )); do
    local status
    status="$("${DC[@]}" ps --format '{{.State}} {{.Health}}' "${svc}" 2>/dev/null | head -1 || true)"
    if [[ "$status" == *healthy* ]]; then log "${svc} healthy"; return 0; fi
    if [[ "$status" == *"exited"* || "$status" == *"dead"* ]]; then
      die "${svc} 容器异常退出: ${status}; 看日志: ${0} logs ${svc}"
    fi
    sleep 5; waited=$((waited+5))
  done
  warn "${svc} ${timeout}s 内未 healthy（可能 start_period 较长），继续执行；用 ${0} status 复查"
  return 0
}

# 生产 app 的 AUTO_MIGRATE 默认 true，迁移在容器启动时自动完成，宿主无需单独跑 migrate
#（如需手动: cd <repo> && make migrate-up / migrate-version）

# ---------- 命令实现 ----------
cmd_up() {
  build=false
  no_migrate=false
  explicit_svcs=()
  for a in "$@"; do
    case "$a" in
      --build) build=true ;;
      --no-build) build=false ;;
      --no-migrate) no_migrate=true ;;
      -h|--help) usage; exit 1 ;;
      -*) die "up: 未知参数 $a（显式服务名请放参数后）" ;;
      *) explicit_svcs+=("$a") ;;
    esac
  done

  check_docker; detect_compose
  check_env_file
  check_dev_conflict || true
  check_host_ports || true

  local extra=()
  if [[ "$no_migrate" == true ]]; then extra+=(--no-migrate); fi

  if [[ ${#explicit_svcs[@]} -gt 0 ]]; then
    log "up 显式服务: ${explicit_svcs[*]}（显式模式不做健康等待）"
    "${DC[@]}" up -d "${explicit_svcs[@]}"
  else
    log "up 核心集: ${CORE_SERVICES[*]}${build:+（--build）}${extra[*]:+${extra[*]}}"
    # 自建镜像场景绝不加 --pull；首次本地无镜像时 compose 会按 build: 定义自建
    "${DC[@]}" up -d "${build:---build}" "${extra[@]:+${extra[@]}}"
  fi

  log "状态:"; "${DC[@]}" ps
  # 显式服务名列表跳过验收
  [[ ${#explicit_svcs[@]} -gt 0 ]] && return 0
  wait_healthy postgres 120
  wait_healthy docreader 180
  wait_healthy app 180
  # frontend 无 healthcheck，用 HTTP 探一次（非致命）
  local fp
  fp="$(grep -E '^FRONTEND_PORT=' .env 2>/dev/null | cut -d= -f2 || true)"
  fp="${fp:-80}"
  if command -v curl >/dev/null 2>&1; then
    local code
    code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "http://127.0.0.1:${fp}/" || true)"
    if [[ "$code" == "200" || "$code" == "301" || "$code" == "302" ]]; then
      log "frontend ok: http://<host>:${fp}/ (HTTP ${code})"
    else
      warn "frontend HTTP 探测非 200 (code=${code:-000})，稍等 nginx 就绪后手工复核: curl -I http://127.0.0.1:${fp}/"
    fi
  fi
  log "DONE. 日志: ${0} logs   状态: ${0} status   停止: ${0} down"
}

cmd_up_services() {
  [[ $# -ge 1 ]] || die "up-services 需要至少一个服务名（app docreader frontend postgres redis ...）"
  check_docker; detect_compose; check_env_file
  log "重建容器（复用现镜像）: $*"
  "${DC[@]}" up -d --force-recreate "$@"
  "${DC[@]}" ps
}

cmd_down() {
  check_docker; detect_compose; check_env_file
  local purge=false
  for a in "$@"; do
    case "$a" in
      --purge-data) purge=true ;;
      -h|--help) usage; exit 1 ;;
      *) warn "down: 忽略未知参数 $a" ;;
    esac
  done
  log "stop + remove 生产容器（named volume 数据保留）"
  "${DC[@]}" down
  [[ "$purge" == true ]] && { warn "--purge-data: 同时删除数据卷（不可恢复！）"; "${DC[@]}" down -v; }
  log "已停止。复查: docker ps -a | grep WeKnora"
}

cmd_status() {
  check_docker; detect_compose; check_env_file
  "${DC[@]}" ps
  local ap fp
  ap="$(grep -E '^APP_PORT=' .env 2>/dev/null | cut -d= -f2 || true)"; ap="${ap:-8080}"
  fp="$(grep -E '^FRONTEND_PORT=' .env 2>/dev/null | cut -d= -f2 || true)"; fp="${fp:-80}"
  if command -v curl >/dev/null 2>&1; then
    local hcode fcode
    hcode="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "http://127.0.0.1:${ap}/health" || true)"
    fcode="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "http://127.0.0.1:${fp}/" || true)"
    log "探测: app /health=${hcode:-000}  frontend=${fcode:-000}"
  fi
}

cmd_logs() {
  check_docker; detect_compose; check_env_file
  local tail_n=200 follow=true args=()
  local svcs=(app docreader postgres)
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -n|--no-follow) follow=false ;;
      -f|--follow) follow=true ;;
      --tail=*) args+=(--tail "${1#--tail=}") ;;
      --tail) shift; args+=(--tail "$1") ;;
      --since=*) args+=(--since "${1#--since=}") ;;
      --since) shift; args+=(--since "$1") ;;
      -h|--help) usage; exit 1 ;;
      -*) die "logs: 未知参数 $1" ;;
      *)
        if [[ ${#svcs[@]} -eq 3 ]]; then svcs=(); fi   # 首个裸服务名 = 覆盖默认集
        svcs+=("$1")
        ;;
    esac
    shift || true
  done
  local flag_args=()
  if [[ "$follow" == true ]]; then flag_args+=(-f)
  else flag_args+=(--tail "${tail_n}")
  fi
  log "logs: ${svcs[*]} (follow=${follow})  Ctrl+C 退出日志，容器不停"
  exec "${DC[@]}" logs "${flag_args[@]}" "${args[@]:+${args[@]}}" "${svcs[@]}"
}

cmd_build() {
  check_docker; detect_compose
  check_env_file
  local targets=()
  if [[ $# -eq 0 ]]; then targets=(app docreader frontend); else targets=("$@"); fi
  log "build 服务: ${targets[*]}（等价 make docker-build-*；版本信息自动从 git 注入）"
  for t in "${targets[@]}"; do
    case "$t" in
      app) "${DC[@]}" build --pull=false app ;;
      docreader) "${DC[@]}" build --pull=false docreader ;;
      frontend) "${DC[@]}" build --pull=false frontend ;;
      *) die "build: 未知服务 $t（可选: app docreader frontend 或 compose profile 服务名需配合 --profile）" ;;
    esac
  done
  log "镜像列表:"; docker images | grep -E 'REPOSITORY|weknora' || true
}

cmd_rebuild_all() {
  # rebuild-all [svc...]: build 指定服务（默认 app docreader frontend）后
  # 对"正在运行的容器"做 --force-recreate；容器未跑则 up -d。
  check_docker; detect_compose; check_env_file
  local targets=()
  if [[ $# -eq 0 ]]; then targets=(app docreader frontend); else targets=("$@"); fi
  for t in "${targets[@]}"; do
    case "$t" in
      app|docreader|frontend) ;;
      *) die "rebuild-all: 未知服务 $t（可选: app docreader frontend）" ;;
    esac
  done
  log "build: ${targets[*]}"
  "${DC[@]}" build --pull=false "${targets[@]}"
  local up_svcs=() t
  for t in "${targets[@]}"; do
    if "${DC[@]}" ps --format '{{.Names}}' 2>/dev/null | grep -q "WeKnora-${t}"; then
      up_svcs+=("$t")
    fi
  done
  if [[ ${#up_svcs[@]} -gt 0 ]]; then
    log "recreate 运行中容器: ${up_svcs[*]}"
    "${DC[@]}" up -d --force-recreate "${up_svcs[@]}"
  else
    log "目标服务容器未在运行，执行 up -d"
    "${DC[@]}" up -d "${targets[@]}"
  fi
  "${DC[@]}" ps
  wait_healthy app 180 || true
  log "重建完成。验收: ${0} status"
}

cmd_profile_up() {
  # 启用可选 profile 服务，如: compose.sh profile-up minio / langfuse / qdrant
  check_docker; detect_compose; check_env_file
  [[ $# -ge 1 ]] || { usage; exit 1; }
  local pargs=()
  for p in "$@"; do
    case " ${PROFILES} " in *" ${p} "*) ;; *) die "profile-up: 未知 profile '${p}'；可选: ${PROFILES}" ;; esac
    pargs+=(--profile "$p")
  done
  log "up profiles: $*"
  "${DC[@]}" "${pargs[@]}" up -d "$@"
  "${DC[@]}" "${pargs[@]}" ps
}

usage() {
  cat <<USAGE
Usage: ${0##*/} <command> [args...]    （在 \$REPO_ROOT=\${REPO_ROOT} 下执行）

Commands:
  up [--build] [--no-build] [--no-migrate] [svc...]
      启动生产核心集 (postgres redis docreader app frontend)。
      默认不 pull、不重建已有容器；--build 时本地无镜像才构建。
      传 svc... 则只 up 这些服务并跳过健康等待。
  up-services <svc...>
      --force-recreate 重建指定服务容器（复用现镜像；换镜像后最常用）。
  down [--purge-data]
      停止并删除生产容器（数据卷保留；--purge-data 才会删卷，不可恢复）。
  status
      compose ps + app /health + frontend HTTP 探测（端口读 .env）。
  logs [--tail=N] [--since=N] [-f|-n] [svc...]
      默认跟踪 app docreader postgres（--tail=200）。裸服务名 = 只跟该服务。
      -n 一次性快照不跟随。
  build [svc...]
      构建镜像，默认 app docreader frontend（--pull=false，绝不拉官方覆盖本地）。
  rebuild-all [svc...]
      build + 对运行中容器 --force-recreate（日常发版核心；svc 默认 app docreader
      frontend）。rebuild.sh 封装此命令。
  profile-up <profile...>
      启用可选 profile 并 up：minio qdrant milvus weaviate neo4j doris dex
      searxng opensearch langfuse odl-hybrid sandbox。

Examples:
  ${0##*/} build                  # 首次: 构建三个镜像
  ${0##*/} up                      # 启动
  ${0##*/} rebuild-all             # 发版: 重建镜像+容器
  ${0##*/} status
  ${0##*/} logs -n                 # 快照一次
  ${0##*/} logs app
  ${0##*/} build app && ${0##*/} up-services app   # 日常发版
  ${0##*/} profile-up langfuse minio
  ${0##*/} down

Notes:
  * 与 dev 环境互斥: 起生产前先 make dev-stop（脚本会探测并告警）。
  * 宿主机 80/8080 被占时脚本告警，请改 .env 的 FRONTEND_PORT/APP_PORT。
  * 迁移: app 容器启动时 AUTO_MIGRATE=true 自动执行；手动 make migrate-up。
  * Ollama 为可选宿主进程（LLM），compose 不负责；如需自管: 生产 .env OLLAMA_BASE_URL
    指向对应地址即可（host.docker.internal 在容器内可用）。
USAGE
}

# ---------- dispatch ----------
CMD="${1:-help}"
shift || true
case "$CMD" in
  up)               cmd_up "$@" ;;
  up-services)      cmd_up_services "$@" ;;
  down|stop)        cmd_down "$@" ;;
  status|ps)        cmd_status "$@" ;;
  logs)             cmd_logs "$@" ;;
  build)            cmd_build "$@" ;;
  rebuild-all)      cmd_rebuild_all "$@" ;;
  profile-up)       cmd_profile_up "$@" ;;
  config)           check_env_file; "${DC[@]}" config "$@" ;;
  -h|--help|help)   usage ;;
  *) die "未知命令: $CMD（help 看用法）" ;;
esac
