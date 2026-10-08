#!/usr/bin/env bash
set -euo pipefail

#
# WeKnora PROD 运维脚本核心（参照 BeadForge/deploy_prod/scripts/compose.sh 风格）
#
# 依据: my_docs/20260929-02-生产模式编译启动与运维命令方案.md
#   - 默认 profile 集: minio + langfuse（对齐 make dev-start），可用 --no-minio/--no-langfuse 关闭
#   - 本地自编译: 仅 app（SELF_BUILT_SERVICES），绝不被 docker pull 覆盖
#   - 其它官方镜像（docreader/minio/langfuse/clickhouse/paradedb/redis/…）在 up 前
#     由 ensure_official_images 按 profile 感知预拉；up 命令绝不用 --build/--pull，
#     避免 "up -d --build" 连带重建 docreader，或 "up -d --pull" 拉官方 weknora-app:latest 覆盖本地
#   - 与 dev 套容器（docker-compose.dev.yml, WeKnora-*-dev）互斥，5432/6379 端口冲突
#   - app 容器内固定监听 8080，宿主机端口 = ${APP_PORT:-8080}
#   - frontend(nginx+dist) 由宿主机自管（不进镜像、不由本脚本启动），代理 /api → app:8080
#
# 仓库根查找: 本目录 my_docs/prod_scripts/ → 上 2 级 = 仓库根（可 WEKNORA_REPO_ROOT 覆盖）

TAG="[weknora-prod]"

log() { printf '%s %s\n' "$TAG" "$*"; }
warn() { printf '%s WARN: %s\n' "$TAG" "$*" >&2; }
die() { printf '%s ERROR: %s\n' "$TAG" "$*" >&2; exit 1; }

# ---------- 路径 ----------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
REPO_ROOT="${WEKNORA_REPO_ROOT:-$(cd "${SCRIPT_DIR}/../.." && pwd)}"

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
detect_compose() {
  DC=()
  if docker compose version >/dev/null 2>&1; then
    DC=(docker compose)
  elif command -v docker-compose >/dev/null 2>&1; then
    DC=(docker-compose)
  fi
  [[ ${#DC[@]} -gt 0 ]]
}

# ---------- 通用服务集 ----------
# 默认集合（docker compose config --services 实测 = redis docreader postgres app frontend）
# 注意: frontend(nginx+dist) 由宿主机自管，不纳入本栈启动 → 核心集仅起 app 后端。
CORE_SERVICES=(postgres redis docreader app)
# 全部可选 profile（与 docker-compose.yml 一致；opensearch 仅存在于 dev 编排，不在生产列表内）
PROFILES="minio qdrant milvus weaviate neo4j doris dex searxng langfuse odl-hybrid full"

# 默认必启 profile（对齐 make dev-start 的硬编码默认）：
#   - minio   : STORAGE_TYPE=minio 时后端 initFileService 启动查 bucket，缺它会 panic → 强依赖
#   - langfuse: dev-start 默认开的可观测栈（.env 配了 LANGFUSE_*KEY 后 app 会自动上报）
# 可用 up --no-minio / --no-langfuse 关闭；用 profile-up 追加其余（qdrant/neo4j/dex...）。
DEFAULT_PROFILES="minio langfuse"

# 本地自编译服务（绝不从 registry 拉取，避免官方 latest 覆盖本地构建）
#
# 注意：生产 compose 里 docreader/sandbox 也带 build: 段，但官方 wechatopenai/
#   weknora-docreader:latest 镜像已发布且与仓库构建等价（dev-start 亦直接拉取），
#   故归入官方镜像走 pull；真正必须本地构建的只有 app。
#   frontend（nginx + dist）由宿主机自管，前端包与 nginx 配置都不进镜像。
SELF_BUILT_SERVICES=("app")

# minio 官方仓库 minio/minio 已从 Docker Hub 下架（2025-09 起匿名拉取 401，
# 各国内镜像 1panel/daocloud 透传上游也是 403）。兜底转推源见下：
#   实际镜像: gowah/minio:${MINIO_MIRROR_TAG}  （MinIO 版本 RELEASE.2025-04-22T22-12-26Z，
#   与 compose 中 minio/minio:RELEASE.2025-09-07T16-13-09Z 的 S3 语义完全兼容）
# pull 失败时自动改用此源并 retag 为 compose 所需的原始名，见 ensure_official_images()
MINIO_MIRROR_TAG="RELEASE.2025-04-22T22-12-26Z"
MINIO_MIRROR_IMAGE="gowah/minio:${MINIO_MIRROR_TAG}"

# $1=服务名，$@（其余，可选）= 已构造好的 --profile 前缀（--profile a --profile b ...）。
# 关键：profile 服务（minio/langfuse/...）不带 --profile 时不出现在 config 输出，
# 必须把 profile 透传，否则查不到该镜像。
service_image() {
  local svc="$1"; shift || true
  "${DC[@]}" "$@" config --format json 2>/dev/null |
    python3 -c "import json,sys;print((json.load(sys.stdin).get('services') or {}).get('${svc}',{}).get('image',''))" 2>/dev/null || true
}

# $@ = --profile 前缀；输出将被启动的服务名（逐行）
active_services() {
  "${DC[@]}" "$@" config --services 2>/dev/null || true
}

# 预拉取官方镜像。参数分两段，按顺序：
#   第一段：profile 名（可选，如 minio langfuse）
#   第二段：服务名（可选）
# 区分规则：出现"服务名"后，后续全部视为服务名。profile 名与 PROFILES 白名单比对。
# 语义：
#   无参              → 用 CORE_SERVICES 预拉
#   仅 profile        → 按 profile 列出 active 服务，逐一预拉
#   仅服务名          → 只处理这些服务（自编译自动跳过）
#   profile + 服务名  → 按 profile 提供 config 解析上下文，只处理列出的服务名
ensure_official_images() {
  local -a profiles=() svcs=() a
  # 先按 PROFILES 白名单判断：命中=profile，否则=服务名
  local seen_svc=false
  for a in "$@"; do
    if [[ "$seen_svc" == true ]]; then svcs+=("$a"); continue; fi
    case " ${PROFILES} " in
      *" ${a} "*) profiles+=("$a") ;;
      *) seen_svc=true; svcs+=("$a") ;;
    esac
  done
  local -a prof_args=()
  for a in "${profiles[@]}"; do prof_args+=(--profile "$a"); done

  if [[ ${#svcs[@]} -eq 0 ]]; then
    if [[ ${#profiles[@]} -gt 0 ]]; then
      # profile 模式：列出该 profile 生效下的全部服务
      local -a active=() s
      while IFS= read -r s; do [[ -n "$s" ]] && active+=("$s"); done < <(active_services "${prof_args[@]}")
      svcs=("${active[@]}")
    else
      svcs=("${CORE_SERVICES[@]}")
    fi
  fi

  local s img seen_imgs=" "
  for s in "${svcs[@]}"; do
    case " ${SELF_BUILT_SERVICES[*]} " in *" ${s} "*) continue ;; esac
    img="$(service_image "$s" "${prof_args[@]}")"
    [[ -z "$img" ]] && continue
    # 去重：compose 里多个服务可共用同一镜像（minio 主服务与 langfuse 各起一个
    # minio 都用 minio/minio:RELEASE.2025-09-07...），同一次调用只处理一次
    if [[ "${seen_imgs}" == *" ${img} "* ]]; then continue; fi
    seen_imgs+="${img} "
    # 本地已有该 tag → 跳过（这些官方镜像都是固定版本 tag，本地备好即视为满足）
    if docker image inspect "$img" >/dev/null 2>&1; then
      log "官方镜像 ${img} 本地已存在，跳过预拉取"
      continue
    fi
    # 本地没有 → 拉取；minio 官方仓库已从 Docker Hub 下架，直接走转推源兜底
    # 注意：pull 的完整 stderr（401/403/manifest unknown/超时等真实原因）必须展示给
    # 调用者，不允许再 2>/dev/null 吞掉 —— 只压掉 "Pulling..." 进度条噪声行。
    if [[ "$img" == minio/minio:* ]]; then
      log "官方源 ${img} 已下架，改用转推源 ${MINIO_MIRROR_IMAGE} 并 retag"
      if docker pull "${MINIO_MIRROR_IMAGE}" 2>&1 | grep -v 'Pulling\|Pull complete\|^Downloading\|^Waiting\|^Done\|^Status:'; then
        docker tag "${MINIO_MIRROR_IMAGE}" "$img"
      else
        warn "pull ${img} 与兜底源 ${MINIO_MIRROR_IMAGE} 均失败（真实错误见上方输出）；up 时该服务将因缺镜像启动失败"
      fi
    else
      log "预拉取官方镜像 ${img}"
      docker pull "$img" 2>&1 | grep -v 'Pulling\|Pull complete\|^Downloading\|^Waiting\|^Done\|^Status:' \
        || warn "pull ${img} 失败（真实错误见上方输出）；up 时该服务可能因缺镜像启动失败"
    fi
  done
}

# 把 profile 名列表转成 `--profile a --profile b ...` 形式的参数数组，结果写入全局 PROF_FLAGS
_profile_flags() {
  PROF_FLAGS=()
  local p
  for p in "$@"; do PROF_FLAGS+=(--profile "$p"); done
}

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
  if ! docker info >/dev/null 2>&1; then
    if ! id -nG 2>/dev/null | tr ' ' '\n' | grep -qx docker; then
      die "docker daemon 访问失败：当前用户不在 docker 组。处理: sudo usermod -aG docker \$USER（执行后重新登录）"
    fi
    die "docker daemon 未运行（docker info 失败）"
  fi
  detect_compose 2>/dev/null || true
}

port_in_use() { # $1=port
  ss -ltn 2>/dev/null | awk '{print $4}' | grep -Eq "[:.]$1\$"
}

check_dev_conflict() {
  # dev 套容器（docker-compose.dev.yml）在跑会抢 5432/6379/宿主映射端口等
  local dev_up
  dev_up="$(docker ps --format '{{.Names}}' 2>/dev/null | grep -cE 'WeKnora-(postgres|redis|app|frontend|docreader)-dev' || true)"
  if [[ "${dev_up}" -gt 0 ]]; then
    warn "检测到 dev 基础设施容器在运行（WeKnora-postgres-dev/redis-dev），生产容器会因 5432/6379 端口冲突起不来；请先: cd <repo> && make dev-stop"
    return 1
  fi
  return 0
}

check_host_ports() {
  # 端口以 .env 为准（compose 未起时读不到 env，直接 grep .env）
  # frontend(nginx) 已改宿主机自管、不在本栈启动 → 只校验 app 端口，不再探测 FRONTEND_PORT。
  local ap
  ap="$(grep -E '^APP_PORT=' .env 2>/dev/null | cut -d= -f2 || true)"
  ap="${ap:-8080}"
  local fail=0
  if port_in_use "${ap}"; then
    warn "宿主机端口 ${ap}（app API 对外映射）已被占用，up 后 WeKnora-app 将起不来。请在 .env 改 APP_PORT 或释放端口"
    fail=1
  fi
  return "${fail}"
}

# ---------- 健康等待 ----------
wait_healthy() { # $1=服务名 $2=超时秒
  local svc="$1" timeout="${2:-180}" waited=0 empty_streak=0
  log "等待 ${svc} 变为 healthy (最长 ${timeout}s)..."
  while (( waited < timeout )); do
    local status
    status="$("${DC[@]}" ps --format '{{.State}} {{.Health}}' "${svc}" 2>/dev/null | head -1 || true)"
    if [[ "$status" == *healthy* ]]; then log "${svc} healthy"; return 0; fi
    if [[ "$status" == *"exited"* || "$status" == *"dead"* ]]; then
      # 异常退出只告警不 die：已拉起的基础设施（postgres/redis/...）不应因
      # 单个服务失败而让整个 up 流程"失败"（set -e 会中断后续步骤）
      warn "${svc} 容器异常退出: ${status}。日志: ${0} logs -n ${svc}"
      return 1
    fi
    if [[ -z "${status//[[:space:]]/}" ]]; then
      # 验收发生在 up 之后，容器长期不可见说明 up 阶段就失败了（如端口冲突/镜像构建失败）
      empty_streak=$((empty_streak+1))
      (( empty_streak >= 6 )) && die "${svc} 容器持续不可见（约 30s）；up 可能失败，查看: ${0} ps / ${0} logs ${svc}"
    else
      empty_streak=0
    fi
    sleep 5; waited=$((waited+5))
 

# 非致命健康等待：超时/异常只 warn + 打印排障指引，永不返回非零，
# 用于"可独立失败"的服务（如 app）—— 失败不得影响已拉起的其他容器。
try_healthy() { # $1=服务名 $2=超时秒
  local svc="$1"
  if ! wait_healthy "$svc" "${2:-180}"; then
    warn "${svc} 未就绪且无法自动恢复。不影响其他已启动服务；排查: ${0} logs -n ${svc} ; ${0} status"
    return 0
  fi
  return 0
} done
  warn "${svc} ${timeout}s 内未 healthy（可能 start_period 较长），继续执行；用 ${0} status 复查"
  return 0
}

# 生产 app 的 AUTO_MIGRATE 默认 true，迁移在容器启动时自动完成，宿主无需单独跑 migrate
#（如需手动: cd <repo> && make migrate-up / migrate-version）

# ---------- 命令实现 ----------
cmd_up() {
  build=false
  explicit_svcs=()
  # 默认 profile 集（对齐 make dev-start）：minio + langfuse，可逐项 --no-* 关闭
  local -a up_profiles
  read -r -a up_profiles <<< "$DEFAULT_PROFILES"
  for a in "$@"; do
    case "$a" in
      --build) build=true ;;
      --no-build) build=false ;;
      --no-minio) up_profiles=("${up_profiles[@]/minio/}") ;;
      --no-langfuse) up_profiles=("${up_profiles[@]/langfuse/}") ;;
      --profile) die "up 不支持 --profile；默认已含 minio langfuse，追加其它请用: ${0} profile-up <profile>" ;;
      -h|--help) usage; exit 1 ;;
      -*) die "up: 未知参数 $a（显式服务名请放参数后）" ;;
      *) explicit_svcs+=("$a") ;;
    esac
  done
  # 去掉空串（--no-* 删除后可能留空元素）
  local -a prof_clean=() p
  for p in "${up_profiles[@]}"; do [[ -n "$p" ]] && prof_clean+=("$p"); done
  _profile_flags "${prof_clean[@]}"
  local prof_disp="${prof_clean[*]:-（无）}"
  log "up 启用 profile: ${prof_disp}"

  check_docker; detect_compose
  check_env_file
  check_dev_conflict || true
  check_host_ports || true

  if [[ ${#explicit_svcs[@]} -gt 0 ]]; then
    # 显式服务模式：profile 名提供 config 解析上下文，确保 profile 服务的 image 能被查到；
    # 自编译服务名跳过；跳过健康等待
    ensure_official_images "${prof_clean[@]}" "${explicit_svcs[@]}"
    log "up 显式服务: ${explicit_svcs[*]}（显式模式不做健康等待）"
    "${DC[@]}" "${PROF_FLAGS[@]}" up -d "${explicit_svcs[@]}"
    "${DC[@]}" "${PROF_FLAGS[@]}" ps
    return 0
  fi

  # 核心集 + 默认 profile（minio langfuse）：
  #  1) 预拉所有将被启动的官方镜像（自编译 app/frontend 自动跳过）
  #     —— 避免 up 阶段 compose 内部逐层拉取失败难定位；
  # 绝不用 --build / --pull：自编译 app 用本地镜像，其他用 ensure_official_images
  # 已 pull 的官方镜像；compose 发现本地已有镜像即直接创建容器。
  #
  # 分两段 up，实现"失败隔离"：
  #   第一段 = 基础设施（postgres/redis/docreader + minio/langfuse 外部组件），
  #            它们之间不依赖 app 健康 → 即使 app 镜像/运行有问题也不受影响；
  #   第二段 = app 后端本体，单独失败只告警、不回滚第一段。
  #   frontend(nginx) 已改宿主机自管，故 app/frontend 均从 compose 启动集合里剔除。
  # 当前 profile 生效的全部服务里去掉 app/frontend，即基础设施集合
  INFRA="$(active_services "${PROF_FLAGS[@]}" | grep -vE '^(app|frontend)$' || true)"
  INFRA="${INFRA//$'\n'/ }"

  log "up 基础设施层 (${INFRA:-无}): 先拉起，任何单点失败不拖垮整段"
  "${DC[@]}" "${PROF_FLAGS[@]}" up -d ${INFRA:+${INFRA}} \
    || warn "基础设施 up 存在失败容器（上方 compose 原始报错）；请 ${0} ps 查看哪个未起"

  log "up 应用层 (app) —— 失败只告警，不影响基础设施"
  "${DC[@]}" "${PROF_FLAGS[@]}" up -d app \
    || warn "app 启动失败（上方为其真实报错）；基础设施仍在运行。排查: ${0} logs -n app"

  log "状态:"; "${DC[@]}" "${PROF_FLAGS[@]}" ps
  # 显式服务名列表跳过验收
  [[ ${#explicit_svcs[@]} -gt 0 ]] && return 0
  wait_healthy postgres 120
  wait_healthy docreader 180
  # app 首启含 AUTO_MIGRATE 迁移 + healthcheck start_period 60s，放宽到 300s 防冷启误判；
  # try_healthy：app 变 unhealthy 只告警，postgres/redis 等已成功拉起的容器不受影响
  try_healthy app 300
  log "DONE. 日志: ${0} logs   状态: ${0} status   停止: ${0} down"
  warn "frontend(nginx) 不在本栈内：请确认宿主机 nginx 已自行启动并代理 API 到 app:8080"
}

cmd_up_services() {
  [[ $# -ge 1 ]] || die "up-services 需要至少一个服务名（app docreader frontend postgres redis ...）"
  check_docker; detect_compose; check_env_file
  # 若指定了官方镜像服务（docreader 等），先补拉（本地缺才下，已有秒过）
  ensure_official_images "$@"
  log "重建容器（复用现镜像）: $*"
  "${DC[@]}" up -d --force-recreate "$@"
  "${DC[@]}" ps
}

cmd_down() {
  check_docker; detect_compose; check_env_file
  local purge=false all=false
  for a in "$@"; do
    case "$a" in
      --purge-data) purge=true ;;
      --all) all=true ;;
      -h|--help) usage; exit 1 ;;
      *) warn "down: 忽略未知参数 $a" ;;
    esac
  done
  # down 必须带上 up 所用 profile，否则 minio/langfuse 容器删不掉、残留占网络
  # （与 dev.sh stop 的"全量 profile 超集"同理）。--all 时把 PROFILES 全带上以清干净。
  local dn_profiles
  if [[ "$all" == true ]]; then dn_profiles="$PROFILES"; else dn_profiles="$DEFAULT_PROFILES"; fi
  local -a dpl
  read -r -a dpl <<< "$dn_profiles"
  _profile_flags "${dpl[@]}"
  log "stop + remove 生产容器（profile: ${dpl[*]}；named volume 数据保留）"
  if [[ "$purge" == true ]]; then
    warn "--purge-data: 同时删除 weknora-prod 项目数据卷（不可恢复！）"
    "${DC[@]}" "${PROF_FLAGS[@]}" down -v
  else
    "${DC[@]}" "${PROF_FLAGS[@]}" down
  fi
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
  # 默认跟踪集；出现首个裸服务名即改为显式集合（勿依赖默认集长度判断）
  local svcs=(app docreader postgres) explicit=false
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
        [[ "$explicit" == false ]] && { svcs=(); explicit=true; }   # 首个裸服务名 = 覆盖默认集
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

# 只构建本地自编译服务（app）—— 版本注入；frontend 已改宿主机自管不进镜像
# 被 cmd_build 与 cmd_up --build 复用。
_build_self_built() { # $@ = 目标服务（缺省 app）
  local targets=() t
  if [[ $# -eq 0 ]]; then targets=(app); else targets=("$@"); fi
  local app_args=()
  if [[ -f ./scripts/get_version.sh ]]; then
    eval "$(./scripts/get_version.sh env)" 2>/dev/null || true
    app_args=(--build-arg VERSION_ARG="${VERSION:-unknown}"
              --build-arg COMMIT_ID_ARG="${COMMIT_ID:-unknown}"
              --build-arg BUILD_TIME_ARG="${BUILD_TIME:-unknown}"
              --build-arg GO_VERSION_ARG="${GO_VERSION:-unknown}")
  fi
  for t in "${targets[@]}"; do
    case "$t" in
      # app_args 元素含空格（BUILD_TIME/GO_VERSION），必须整体引用展开，
      # 否则单词分割后 compose 会把 "10:11:05" 之类当成服务名 → no such service
      app) "${DC[@]}" build --pull=false ${app_args[@]+"${app_args[@]}"} app ;;
      *) die "_build_self_built: 仅支持 app（frontend 已改宿主机自管不进镜像；收到: $t）" ;;
    esac
  done
}

cmd_build() {
  check_docker; detect_compose
  check_env_file
  local targets=() t
  # 默认 app；显式给 docreader 单独走 build（本仓库 docreader 构建）
  if [[ $# -eq 0 ]]; then targets=(app); else targets=("$@"); fi
  log "build 服务: ${targets[*]}（版本/git 信息注入，对齐 make docker-build-*）"
  local self=() extra=()
  for t in "${targets[@]}"; do
    case "$t" in
      app) self+=("$t") ;;
      docreader) extra+=("docreader") ;;
      *) die "build: 未知服务 $t（可选: app docreader；frontend 已改宿主机自管不进镜像）" ;;
    esac
  done
  if [[ ${#self[@]} -gt 0 ]]; then
    _build_self_built "${self[@]}"
  fi
  for t in "${extra[@]}"; do
    log "build 官方服务（本地构建版本，覆盖官方 pull 镜像）: $t"
    "${DC[@]}" build --pull=false "$t"
  done
  log "镜像列表:"
  docker images --format '{{.Repository}}:{{.Tag}}' | grep -i weknora || true
}

cmd_rebuild_all() {
  # rebuild-all [svc...]: build 指定自编译服务（默认 app）后
  # 对"正在运行的容器"做 --force-recreate；容器未跑则 up -d。
  check_docker; detect_compose; check_env_file
  local targets=() t
  if [[ $# -eq 0 ]]; then targets=(app); else targets=("$@"); fi
  for t in "${targets[@]}"; do
    case "$t" in
      # docreader 属官方镜像（pull 更新），不走 rebuild；显式拒绝防误编译
      app) ;;
      *) die "rebuild-all: 未知服务 $t（可选: app；frontend 已改宿主机自管不进镜像）" ;;
    esac
  done
  _build_self_built "${targets[@]}"
  local up_svcs=()
  for t in "${targets[@]}"; do
    if "${DC[@]}" ps --format '{{.Names}}' 2>/dev/null | grep -qx "WeKnora-${t}"; then
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

# profile → 具体服务名映射（docker-compose.yml 中 profile 名与服务名并非总是一一对应）
# 返回空字符串 = 仅启用 --profile，不指定单一服务（如 full）
profile_services() {
  case "$1" in
    minio)        echo "minio" ;;
    qdrant)       echo "qdrant" ;;
    milvus)       echo "milvus" ;;
    weaviate)     echo "weaviate" ;;
    neo4j)        echo "neo4j" ;;
    searxng)      echo "searxng searxng-init" ;;
    odl-hybrid)   echo "odl-hybrid" ;;
    dex)          echo "dex" ;;
    doris)        echo "doris-fe doris-be" ;;
    langfuse)     echo "langfuse-db-init langfuse-clickhouse langfuse-minio langfuse-worker langfuse-web" ;;
    full)         echo "" ;;
    *)            echo "__unknown__" ;; 
  esac
}

cmd_profile_up() {
  # 启用可选 profile 服务，如: compose.sh profile-up minio / langfuse / doris / full
  check_docker; detect_compose; check_env_file
  [[ $# -ge 1 ]] || { usage; exit 1; }
  local pargs=() svcs=() p slist s
  for p in "$@"; do
    case " ${PROFILES} " in *" ${p} "*) ;; *) die "profile-up: 未知 profile '${p}'；可选: ${PROFILES}" ;; esac
    pargs+=(--profile "$p")
    slist="$(profile_services "$p")"
    [[ "$slist" == "__unknown__" ]] && die "profile '${p}' 缺少服务映射，请在 profile_services() 补全"
    for s in $slist; do svcs+=("$s"); done
  done
  log "up profiles: $*${svcs[*]:+  （服务: ${svcs[*]}）}"
  # full 等“仅启用 profile”场景 svcs 为空 → up -d 不带服务名（拉起全部已启用服务）
  "${DC[@]}" "${pargs[@]}" up -d ${svcs[@]:+${svcs[@]}}
  "${DC[@]}" "${pargs[@]}" ps
}

usage() {
  cat <<USAGE
Usage: ${0##*/} <command> [args...]    （在 \$REPO_ROOT=\${REPO_ROOT} 下执行）

Commands:
  up [--build] [--no-minio] [--no-langfuse] [svc...]
      启动生产核心集 (postgres redis docreader app)
      + 默认 profile (minio langfuse, 对齐 make dev-start)。
      自动: 1) 预拉所有官方镜像（app 跳过）2) 启动
      --build           先本地 build app，再启动（其余服务仍走官方 pull）
      --no-minio        关闭默认 minio profile（当 STORAGE_TYPE 非 minio 时可用）
      --no-langfuse     关闭默认 langfuse profile
      传 svc...         只 up 列出服务（跳过健康等待，显式服务模式）
  up-services <svc...>
      --force-recreate 重建指定服务容器（复用现镜像；换镜像后最常用）。
      可选服务名: app redis postgres docreader（不含 frontend）。
  down [--purge-data] [--all]
      停止并删除生产容器（默认含 minio langfuse profile；--all 含全部 profile）。
      --purge-data 同时删除数据卷（不可恢复！）。
  status
      compose ps + app /health + frontend HTTP 探测（端口读 .env）。
  logs [--tail=N] [--since=N] [-f|-n] [svc...]
      默认跟踪 app docreader postgres（--tail=200）。裸服务名 = 只跟该服务。
      -n 一次性快照不跟随。
  build [svc...]
      构建本地自编译镜像，默认 app（--pull=false，绝不拉官方覆盖本地）。
      docreader 属官方镜像正常走 pull；特殊改动才 build docreader。
      frontend 已改宿主机自管，不在此构建。
  rebuild-all [svc...]
      本地 build（默认 app）+ 对运行中容器 --force-recreate（发版核心）。
      rebuild.sh 封装此命令。
  profile-up <profile...>
      启用其它可选 profile 并 up：qdrant milvus weaviate neo4j doris dex
      searxng odl-hybrid full（minio/langfuse 默认已含，可用此命令再叠加；
      full 会一并启用 sandbox/mcp 等 build 服务，谨慎使用）。

Examples:
  ${0##*/} up                      # 启动核心 + minio + langfuse，自动预拉官方镜像
  ${0##*/} up --build              # 先构建 app，再启动（frontend 由宿主机自管）
  ${0##*/} up --no-langfuse        # 关 langfuse，只开 minio
  ${0##*/} rebuild-all             # 发版: 重建 app + 容器（frontend 由宿主机自管）
  ${0##*/} status
  ${0##*/} logs -n                 # 快照一次
  ${0##*/} profile-up qdrant       # 追加 qdrant（原默认 profile 仍生效）
  ${0##*/} down                    # 停生产（含 minio langfuse）
  ${0##*/} down --all              # 停生产 + 所有 profile 容器

Notes:
  * 与 dev 环境互斥: 起生产前先 make dev-stop（脚本会探测并告警）。
  * app(API) 端口被占时脚本告警，请改 .env 的 APP_PORT。
  * frontend(nginx) 由宿主机自管：本脚本不构建、不启动 WeKnora-frontend；前端包与
    nginx 配置都不进镜像，需自行把 nginx 配好并代理 /api 到 app:8080。
  * 迁移: app 容器启动时 AUTO_MIGRATE=true 自动执行；手动 make migrate-up。
  * Ollama 为可选宿主进程（LLM），compose 不负责；如需自管: 生产 .env OLLAMA_BASE_URL
    指向对应地址即可（host.docker.internal 在容器内可用）。
  * app 是本地构建镜像，绝不被 docker pull 覆盖；所有其它官方镜像
    （minio/clickhouse/langfuse/docreader/paradedb/redis/qdrant/neo4j/dex...）
    up 前会先预拉，失败会明确告警而非让 compose 中途崩。
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
