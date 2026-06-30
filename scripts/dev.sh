#!/bin/bash
# 开发环境启动脚本 - 按 .env 启动基础设施，app 和 frontend 需要手动在本地运行

# 设置颜色
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # 无颜色

# 获取项目根目录
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." && pwd )"
# 默认跟随仓库历史 compose project 命名空间，继续复用既有 weknora_* named volumes。
WEKNORA_DEV_COMPOSE_PROJECT_NAME="${WEKNORA_DEV_COMPOSE_PROJECT_NAME:-weknora}"
WEKNORA_DEV_COMPOSE_FILE="$PROJECT_ROOT/docker-compose.dev.yml"

# 日志函数
log_info() {
    printf "%b\n" "${BLUE}[INFO]${NC} $1"
}

log_success() {
    printf "%b\n" "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    printf "%b\n" "${RED}[ERROR]${NC} $1"
}

log_warning() {
    printf "%b\n" "${YELLOW}[WARNING]${NC} $1"
}

# 选择可用的 Docker Compose 命令
DOCKER_COMPOSE_BIN=""
DOCKER_COMPOSE_SUBCMD=""

detect_compose_cmd() {
    if docker compose version &> /dev/null; then
        DOCKER_COMPOSE_BIN="docker"
        DOCKER_COMPOSE_SUBCMD="compose"
        return 0
    fi
    if command -v docker-compose &> /dev/null; then
        if docker-compose version &> /dev/null; then
            DOCKER_COMPOSE_BIN="docker-compose"
            DOCKER_COMPOSE_SUBCMD=""
            return 0
        fi
    fi
    return 1
}

dev_compose() {
    local compose_cmd=("$DOCKER_COMPOSE_BIN")

    if [ -n "$DOCKER_COMPOSE_SUBCMD" ]; then
        compose_cmd+=("$DOCKER_COMPOSE_SUBCMD")
    fi

    compose_cmd+=(--project-name "$WEKNORA_DEV_COMPOSE_PROJECT_NAME" -f docker-compose.dev.yml)
    compose_cmd+=("$@")
    "${compose_cmd[@]}"
}

# 显示帮助信息
show_help() {
    printf "%b\n" "${GREEN}WeKnora 开发环境脚本${NC}"
    echo "用法: $0 [命令] [选项]"
    echo ""
    echo "命令:"
    echo "  start      按 .env 启动基础设施服务（postgres、redis、docreader、minio）"
    echo "  stop       停止所有服务"
    echo "  restart    重启所有服务"
    echo "  logs       查看服务日志"
    echo "  status     查看服务状态"
    echo "  app        启动后端应用（本地运行）"
    echo "  frontend   启动前端开发服务器（本地运行）"
    echo "  help       显示此帮助信息"
    echo ""
    echo "可选 Profile（用于 start 命令）:"
    echo "  --minio    启动 MinIO 对象存储"
    echo "  --qdrant   启动 Qdrant 向量数据库"
    echo "  --milvus   启动 Milvus 向量数据库"
    echo "  --neo4j    启动 Neo4j 图数据库"
    echo "  --jaeger   启动 Jaeger 链路追踪"
    echo "  --dex      启动 Dex（OIDC 身份认证）"
    echo "  --full     启动所有可选服务"
    echo ""
    echo "环境变量:"
    echo "  WEKNORA_DEV_DATA_ROOT  显式设置后改为绑定宿主机目录；推荐默认值: $WEKNORA_DEV_DATA_ROOT_DEFAULT；普通启动未设置时沿用 Docker named volume，sudo/root 启动未设置时自动使用默认目录"
    echo ""
    echo "示例："
    echo "  $0 start                    # 按 .env 启动基础服务（仓库默认含 MinIO）"
    echo "  $0 start --qdrant           # 启动基础服务 + Qdrant"
    echo "  $0 start --milvus           # 启动基础服务 + Milvus"
    echo "  $0 start --qdrant --jaeger  # 启动基础服务 + Qdrant + Jaeger"
    echo "  $0 start --dex             # 启动基础服务 + Dex"
    echo "  $0 start --full             # 启动所有服务"
    echo "  WEKNORA_DEV_DATA_ROOT=$WEKNORA_DEV_DATA_ROOT_DEFAULT $0 start"
    echo "  source ./scripts/dev-host-data.sh && $0 start"
    echo "  source ./scripts/dev-named-volume.sh && $0 start"
    echo "  $0 app                      # 在另一个终端启动后端"
    echo "  $0 frontend                 # 在另一个终端启动前端"
}

append_profile() {
    local profile="$1"
    if [[ " $ENABLED_SERVICES " != *" $profile "* ]]; then
        PROFILES="$PROFILES --profile $profile"
        ENABLED_SERVICES="$ENABLED_SERVICES $profile"
    fi
}

is_local_minio_endpoint() {
    local endpoint="$1"
    case "$endpoint" in
        ""|minio:9000|localhost:9000|127.0.0.1:9000|http://localhost:9000|http://127.0.0.1:9000)
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

auto_detect_profiles_from_env() {
    local retrieve_driver="${RETRIEVE_DRIVER:-postgres}"
    local storage_type="${STORAGE_TYPE:-local}"
    local minio_endpoint="${MINIO_ENDPOINT:-}"
    local enable_graph_rag="${ENABLE_GRAPH_RAG:-false}"
    local neo4j_enable="${NEO4J_ENABLE:-false}"

    case "$storage_type" in
        minio)
            if is_local_minio_endpoint "$minio_endpoint"; then
                append_profile "minio"
            fi
            ;;
    esac

    case "$retrieve_driver" in
        qdrant)
            append_profile "qdrant"
            ;;
        milvus)
            append_profile "milvus"
            ;;
    esac

    if [[ "$enable_graph_rag" == "true" || "$neo4j_enable" == "true" ]]; then
        append_profile "neo4j"
    fi
}

# 检查 Docker
check_docker() {
    if ! command -v docker &> /dev/null; then
        log_error "未安装Docker，请先安装Docker"
        return 1
    fi
    
    if ! detect_compose_cmd; then
        log_error "未检测到 Docker Compose"
        return 1
    fi
    
    # 尝试连接 Docker 守护进程，捕获错误输出以便诊断
    DOCKER_INFO_OUTPUT=$(docker info 2>&1)
    if [ $? -ne 0 ]; then
        log_error "无法连接到 Docker 守护进程: $DOCKER_INFO_OUTPUT"

        # 在 WSL 环境提供额外提示
        if grep -qi "microsoft" /proc/version 2>/dev/null || [ -n "${WSL_DISTRO_NAME:-}" ]; then
            log_warning "检测到 WSL 环境。请确保 Docker Desktop 已在 Windows 上运行并已启用 WSL 集成，或在 WSL 中运行 Docker 守护进程。"
            log_info "常见修复（任选其一）:"
            echo "  - 在 Windows 的 Docker Desktop 设置中启用 WSL 集成并选择当前发行版。"
            echo "  - 在 WSL 中启用守护进程: sudo service docker start 或 sudo dockerd &"
            echo "  - 检查 DOCKER_HOST 环境变量: echo \$DOCKER_HOST（如指向远程地址可能需要调整）"
        else
            log_info "建议检查 Docker 是否已启动: sudo systemctl start docker; sudo systemctl status docker"
        fi

        # 给出一些快速诊断命令，帮助用户贴出错误信息
        log_info "可运行的诊断命令:" 
        echo "  docker --version"
        echo "  docker info"
        echo "  docker ps"
        return 1
    fi

    return 0
}

load_project_env() {
    local required="${1:-false}"

    if [ -f "$PROJECT_ROOT/.env" ]; then
        set -a
        source "$PROJECT_ROOT/.env"
        set +a
        return 0
    fi

    if [ "$required" = "true" ]; then
        log_error ".env 文件不存在，请先创建配置文件"
        return 1
    fi

    return 0
}

configure_dev_data_mounts() {
    local requested_path="${WEKNORA_DEV_DATA_ROOT:-}"
    local data_base=""
    local postgres_path=""

    if [ -z "${WEKNORA_DEV_DATA_ROOT:-}" ]; then
        unset WEKNORA_DEV_POSTGRES_VOLUME \
            WEKNORA_DEV_REDIS_VOLUME \
            WEKNORA_DEV_MINIO_VOLUME \
            WEKNORA_DEV_QDRANT_VOLUME \
            WEKNORA_DEV_OPENSEARCH_VOLUME \
            WEKNORA_DEV_MILVUS_VOLUME \
            WEKNORA_DEV_NEO4J_VOLUME \
            WEKNORA_DEV_DOCREADER_TMP_VOLUME \
            WEKNORA_DEV_JAEGER_VOLUME \
            WEKNORA_DEV_SEARXNG_CONFIG_VOLUME \
            WEKNORA_DEV_LANGFUSE_CLICKHOUSE_DATA_VOLUME \
            WEKNORA_DEV_LANGFUSE_CLICKHOUSE_LOGS_VOLUME \
            WEKNORA_DEV_LANGFUSE_MINIO_VOLUME \
            WEKNORA_DEV_DATA_BASE
        return 0
    fi

    case "$requested_path" in
        /*)
            ;;
        *)
            requested_path="$PROJECT_ROOT/$requested_path"
            ;;
    esac
    requested_path="${requested_path%/}"

    if [ "${requested_path##*/}" = "postgres" ]; then
        postgres_path="$requested_path"
        data_base="$(dirname "$requested_path")"
    else
        data_base="$requested_path"
        postgres_path="$data_base/postgres"
    fi

    WEKNORA_DEV_DATA_ROOT="$requested_path"
    WEKNORA_DEV_DATA_BASE="$data_base"
    export WEKNORA_DEV_DATA_ROOT
    export WEKNORA_DEV_DATA_BASE

    export WEKNORA_DEV_POSTGRES_VOLUME="$postgres_path"
    export WEKNORA_DEV_REDIS_VOLUME="$WEKNORA_DEV_DATA_BASE/redis"
    export WEKNORA_DEV_MINIO_VOLUME="$WEKNORA_DEV_DATA_BASE/minio"
    export WEKNORA_DEV_QDRANT_VOLUME="$WEKNORA_DEV_DATA_BASE/qdrant"
    export WEKNORA_DEV_OPENSEARCH_VOLUME="$WEKNORA_DEV_DATA_BASE/opensearch"
    export WEKNORA_DEV_MILVUS_VOLUME="$WEKNORA_DEV_DATA_BASE/milvus"
    export WEKNORA_DEV_NEO4J_VOLUME="$WEKNORA_DEV_DATA_BASE/neo4j"
    export WEKNORA_DEV_DOCREADER_TMP_VOLUME="$WEKNORA_DEV_DATA_BASE/docreader/tmp"
    export WEKNORA_DEV_JAEGER_VOLUME="$WEKNORA_DEV_DATA_BASE/jaeger"
    export WEKNORA_DEV_SEARXNG_CONFIG_VOLUME="$WEKNORA_DEV_DATA_BASE/searxng/config"
    export WEKNORA_DEV_LANGFUSE_CLICKHOUSE_DATA_VOLUME="$WEKNORA_DEV_DATA_BASE/langfuse/clickhouse/data"
    export WEKNORA_DEV_LANGFUSE_CLICKHOUSE_LOGS_VOLUME="$WEKNORA_DEV_DATA_BASE/langfuse/clickhouse/logs"
    export WEKNORA_DEV_LANGFUSE_MINIO_VOLUME="$WEKNORA_DEV_DATA_BASE/langfuse/minio"

    return 0
}

prepare_dev_data_dirs() {
    configure_dev_data_mounts
    if [ $? -ne 0 ]; then
        return 1
    fi

    if [ -z "${WEKNORA_DEV_DATA_ROOT:-}" ]; then
        log_info "开发环境数据存储: Docker named volumes（兼容历史脚本行为）"
        return 0
    fi

    local data_dirs=(
        "$WEKNORA_DEV_POSTGRES_VOLUME"
        "$WEKNORA_DEV_REDIS_VOLUME"
        "$WEKNORA_DEV_MINIO_VOLUME"
        "$WEKNORA_DEV_QDRANT_VOLUME"
        "$WEKNORA_DEV_OPENSEARCH_VOLUME"
        "$WEKNORA_DEV_MILVUS_VOLUME"
        "$WEKNORA_DEV_NEO4J_VOLUME"
        "$WEKNORA_DEV_DOCREADER_TMP_VOLUME"
        "$WEKNORA_DEV_JAEGER_VOLUME"
        "$WEKNORA_DEV_SEARXNG_CONFIG_VOLUME"
        "$WEKNORA_DEV_LANGFUSE_CLICKHOUSE_DATA_VOLUME"
        "$WEKNORA_DEV_LANGFUSE_CLICKHOUSE_LOGS_VOLUME"
        "$WEKNORA_DEV_LANGFUSE_MINIO_VOLUME"
    )
    local dir=""

    for dir in "${data_dirs[@]}"; do
        if ! mkdir -p "$dir"; then
            log_error "无法创建开发环境数据目录: $dir"
            log_info "请确保当前用户对 $WEKNORA_DEV_DATA_BASE 有写权限，或设置 WEKNORA_DEV_DATA_ROOT 指向可写目录。"
            return 1
        fi
    done

    for dir in "${data_dirs[@]}"; do
        if ! chmod 0777 "$dir"; then
            log_warning "无法调整目录权限: $dir"
        fi
    done

    log_info "PostgreSQL 数据目录: $WEKNORA_DEV_POSTGRES_VOLUME"
    log_info "开发环境数据根目录: $WEKNORA_DEV_DATA_BASE"
    return 0
}

is_sudo_or_root() {
    if [ -n "${SUDO_USER:-}" ]; then
        return 0
    fi

    if [ "$(id -u)" -eq 0 ] 2>/dev/null; then
        return 0
    fi

    return 1
}

apply_default_dev_data_root_for_sudo() {
    if [ -n "${WEKNORA_DEV_DATA_ROOT:-}" ]; then
        return 0
    fi

    if [ -z "${WEKNORA_DEV_DATA_ROOT_DEFAULT:-}" ]; then
        return 0
    fi

    if ! is_sudo_or_root; then
        return 0
    fi

    WEKNORA_DEV_DATA_ROOT="$WEKNORA_DEV_DATA_ROOT_DEFAULT"
    export WEKNORA_DEV_DATA_ROOT
    log_info "检测到 sudo/root 启动且未显式设置 WEKNORA_DEV_DATA_ROOT，改用默认宿主机目录: $WEKNORA_DEV_DATA_ROOT"
    return 0
}

mount_source_missing() {
    local source_path="$1"

    case "$source_path" in
        /data/docker/volumes/*)
            [ ! -e "$source_path" ]
            return
            ;;
    esac

    return 1
}

container_project_mismatch() {
    local container_id="$1"
    local project_label=""

    project_label="$(docker inspect "$container_id" --format '{{ index .Config.Labels "com.docker.compose.project" }}' 2>/dev/null || true)"
    [[ -n "$project_label" && "$project_label" != "$WEKNORA_DEV_COMPOSE_PROJECT_NAME" ]]
}

cleanup_stale_dev_containers() {
    local container_id=""
    local container_name=""
    local mount_source=""
    local has_missing_mount="false"
    local has_project_mismatch="false"
    local removed_any="false"

    while IFS= read -r container_id; do
        [ -n "$container_id" ] || continue
        has_missing_mount="false"
        has_project_mismatch="false"

        if container_project_mismatch "$container_id"; then
            has_project_mismatch="true"
        fi

        while IFS= read -r mount_source; do
            [ -n "$mount_source" ] || continue
            if mount_source_missing "$mount_source"; then
                has_missing_mount="true"
                break
            fi
        done < <(docker inspect "$container_id" --format '{{range .Mounts}}{{.Source}}{{"\n"}}{{end}}' 2>/dev/null)

        if [ "$has_missing_mount" != "true" ] && [ "$has_project_mismatch" != "true" ]; then
            continue
        fi

        container_name="$(docker inspect "$container_id" --format '{{.Name}}' 2>/dev/null | sed 's#^/##')"
        if [ "$has_project_mismatch" = "true" ]; then
            log_warning "检测到旧的开发容器 project 名与当前配置不一致，移除后重建: ${container_name:-$container_id}"
        else
            log_warning "检测到失效的开发容器挂载，移除容器后重建: ${container_name:-$container_id}"
        fi
        if docker rm -fv "$container_id" >/dev/null 2>&1; then
            removed_any="true"
        fi
    done < <(docker ps -aq --filter "label=com.docker.compose.project.config_files=$WEKNORA_DEV_COMPOSE_FILE" 2>/dev/null)

    [ "$removed_any" = "true" ]
}

# 启动基础设施服务
start_services() {
    log_info "启动开发环境基础设施服务..."
    
    check_docker
    if [ $? -ne 0 ]; then
        return 1
    fi
    
    cd "$PROJECT_ROOT"

    load_project_env true
    if [ $? -ne 0 ]; then
        return 1
    fi

    apply_default_dev_data_root_for_sudo
    if [ $? -ne 0 ]; then
        return 1
    fi

    prepare_dev_data_dirs
    if [ $? -ne 0 ]; then
        return 1
    fi
    
    # 解析 profile 参数
    shift  # 移除 "start" 命令本身
    PROFILES=""
    ENABLED_SERVICES=""

    auto_detect_profiles_from_env
    
    while [ $# -gt 0 ]; do
        case "$1" in
            --minio)
                append_profile "minio"
                ;;
            --qdrant)
                append_profile "qdrant"
                ;;
            --milvus)
                append_profile "milvus"
                ;;
            --neo4j)
                append_profile "neo4j"
                ;;
            --jaeger)
                append_profile "jaeger"
                ;;
            --dex)
                append_profile "dex"
                ;;
            --full)
                PROFILES=""
                ENABLED_SERVICES=""
                append_profile "full"
                break
                ;;
            *)
                log_warning "未知参数: $1"
                ;;
        esac
        shift
    done

    if [ -n "$ENABLED_SERVICES" ]; then
        log_info "按 .env/命令参数启用的可选服务:${ENABLED_SERVICES}"
    else
        log_info "按 .env 配置仅启动基础服务: postgres, redis, docreader"
    fi

    if cleanup_stale_dev_containers; then
        log_info "已清理失效的开发容器挂载，继续启动服务"
    fi
    
    # 启动服务
    dev_compose $PROFILES up -d
    
    if [ $? -eq 0 ]; then
        log_success "基础设施服务已启动"
        echo ""
        log_info "服务访问地址:"
        echo "  - PostgreSQL:    localhost:5432"
        echo "  - Redis:         localhost:6379"
        echo "  - DocReader:     localhost:50051"
        
        # 根据启用的 profile 显示额外服务
        if [[ "$ENABLED_SERVICES" == *"minio"* ]]; then
            echo "  - MinIO:         localhost:9000 (Console: localhost:9001)"
        fi
        if [[ "$ENABLED_SERVICES" == *"qdrant"* ]]; then
            echo "  - Qdrant:        localhost:6333 (gRPC: localhost:6334)"
        fi
        if [[ "$ENABLED_SERVICES" == *"milvus"* ]]; then
            echo "  - Milvus:        localhost:19530 (Health: localhost:9091)"
        fi
        if [[ "$ENABLED_SERVICES" == *"neo4j"* ]]; then
            echo "  - Neo4j:         localhost:7474 (Bolt: localhost:7687)"
        fi
        if [[ "$ENABLED_SERVICES" == *"jaeger"* ]]; then
            echo "  - Jaeger:        localhost:16686"
        fi
        if [[ "$ENABLED_SERVICES" == *"dex"* ]]; then
            echo "  - Dex:           localhost:5556"
        fi
        
        echo ""
        log_info "接下来的步骤:"
        printf "%b\n" "${YELLOW}1. 在新终端运行后端:${NC} make dev-app"
        printf "%b\n" "${YELLOW}2. 在新终端运行前端:${NC} make dev-frontend"
        return 0
    else
        log_error "服务启动失败"
        return 1
    fi
}

# 停止服务
stop_services() {
    log_info "停止开发环境服务..."
    local services=()
    local service=""
    
    check_docker
    if [ $? -ne 0 ]; then
        return 1
    fi
    
    cd "$PROJECT_ROOT"
    load_project_env
    if [ $? -ne 0 ]; then
        return 1
    fi
    configure_dev_data_mounts
    if [ $? -ne 0 ]; then
        return 1
    fi

    while IFS= read -r service; do
        if [ -n "$service" ]; then
            services+=("$service")
        fi
    done < <(dev_compose --profile full config --services)

    if [ ${#services[@]} -eq 0 ]; then
        log_error "未能解析开发环境服务列表"
        return 1
    fi

    # Avoid 'down --remove-orphans': sibling compose stacks under the same
    # default project name would be treated as orphans and stopped/removed.
    dev_compose --profile full stop "${services[@]}"
    
    if [ $? -eq 0 ]; then
        log_success "所有服务已停止"
        return 0
    else
        log_error "服务停止失败"
        return 1
    fi
}

# 重启服务
restart_services() {
    stop_services
    sleep 2
    start_services
}

# 查看日志
show_logs() {
    check_docker
    if [ $? -ne 0 ]; then
        return 1
    fi
    cd "$PROJECT_ROOT"
    load_project_env
    if [ $? -ne 0 ]; then
        return 1
    fi
    configure_dev_data_mounts
    if [ $? -ne 0 ]; then
        return 1
    fi
    dev_compose logs -f
}

# 查看状态
show_status() {
    check_docker
    if [ $? -ne 0 ]; then
        return 1
    fi
    cd "$PROJECT_ROOT"
    load_project_env
    if [ $? -ne 0 ]; then
        return 1
    fi
    configure_dev_data_mounts
    if [ $? -ne 0 ]; then
        return 1
    fi
    dev_compose ps
}

# 启动后端应用（本地）
start_app() {
    log_info "启动后端应用（本地开发模式）..."
    
    cd "$PROJECT_ROOT"
    
    # 检查 Go 是否安装
    if ! command -v go &> /dev/null; then
        log_error "Go 未安装"
        return 1
    fi
    
    log_info "加载 .env 文件..."
    load_project_env true
    if [ $? -ne 0 ]; then
        return 1
    fi
    
    # 设置本地开发环境变量（覆盖 Docker 容器地址）
    export DB_HOST=localhost
    export DOCREADER_ADDR=localhost:50051
    export DOCREADER_TRANSPORT=grpc
    if [ -z "$MINIO_ENDPOINT" ] || [ "$MINIO_ENDPOINT" = "minio:9000" ] || [ "$MINIO_ENDPOINT" = "localhost:9000" ]; then
        export MINIO_ENDPOINT=localhost:9000
    fi
    export REDIS_ADDR=localhost:6379
    export MILVUS_ADDRESS=localhost:19530
    export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
    export NEO4J_URI=bolt://localhost:7687
    export QDRANT_HOST=localhost
    export DB_PORT=${DB_PORT:-5432}
    
    # 确保必要的环境变量已设置
    if [ -z "$DB_DRIVER" ]; then
        log_error "DB_DRIVER 环境变量未设置，请检查 .env 文件"
        return 1
    fi
    
    log_info "环境变量已设置，启动应用..."
    log_info "数据库地址: $DB_HOST:${DB_PORT:-5432}"
    
    export CGO_CFLAGS="-Wno-deprecated-declarations -Wno-gnu-folding-constant"
    if [[ "$(uname)" == "Darwin" ]]; then
      export CGO_LDFLAGS="-Wl,-no_warn_duplicate_libraries"
    fi

    # 检查是否安装了 Air（热重载工具）
    if command -v air &> /dev/null; then
        log_success "检测到 Air，使用热重载模式启动..."
        log_info "修改 Go 代码后将自动重新编译和重启"
        air
    else
        log_info "未检测到 Air，使用普通模式启动"
        log_warning "提示: 安装 Air 可以实现代码修改后自动重启"
        log_info "安装命令: go install github.com/air-verse/air@latest"
        LDFLAGS="$(./scripts/get_version.sh ldflags) -X 'google.golang.org/protobuf/reflect/protoregistry.conflictPolicy=warn'"
        go run -ldflags="$LDFLAGS" ./cmd/server
    fi
}

# 启动前端（本地）
start_frontend() {
    log_info "启动前端开发服务器..."
    
    cd "$PROJECT_ROOT/frontend"
    
    # 检查 npm 是否安装
    if ! command -v npm &> /dev/null; then
        log_error "npm 未安装"
        return 1
    fi
    
    # 检查依赖是否已安装
    if [ ! -d "node_modules" ]; then
        log_warning "node_modules 不存在，正在安装依赖..."
        npm install
    fi
    
    log_info "启动 Vite 开发服务器..."
    log_info "前端将运行在 http://localhost:5173"
    log_info "前端 API 代理目标: ${VITE_DEV_PROXY_TARGET:-${FRONTEND_BACKEND_URL:-http://localhost:8080}}"
    
    # 运行开发服务器
    npm run dev
}

# 解析命令
CMD="${1:-help}"
case "$CMD" in
    start)
        start_services "$@"
        ;;
    stop)
        stop_services
        ;;
    restart)
        restart_services
        ;;
    logs)
        show_logs
        ;;
    status)
        show_status
        ;;
    app)
        start_app
        ;;
    frontend)
        start_frontend
        ;;
    help|--help|-h)
        show_help
        ;;
    *)
        log_error "未知命令: $CMD"
        show_help
        exit 1
        ;;
esac

exit 0
