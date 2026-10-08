# WeKnora 生产运维脚本（prod_scripts）

参照 `BeadForge/deploy_prod` 的「顶层薄封装 + 核心脚本」结构，落地
《20260929-02-生产模式编译启动与运维命令方案.md》中的全部命令。

## 文件清单

| 文件 | 作用 | 等价 make / docker 命令 |
|------|------|------------------------|
| `start.sh`   | 启动（对齐 make dev-start：核心集 + minio + langfuse 默认 profile；自动预拉官方镜像；本地只 build app；`--build` 强制自编译；`--no-minio`/`--no-langfuse` 关闭） | `make dev-start`（但生产版固定 `COMPOSE_PROJECT_NAME=weknora-prod`） |
| `stop.sh`    | 停止删除容器（含 minio+langfuse 两个默认 profile；`--all` 含全部 profile；数据卷保留） | `make dev-stop` |
| `status.sh`  | 容器状态 + app /health + frontend HTTP 探测 | `docker compose ps` + curl |
| `logs.sh`    | 日志（默认跟 app/docreader/postgres） | `docker compose logs -f` |
| `rebuild.sh` | 发版：本地 build app + 重建运行中容器（数据不动；frontend 已改宿主机自管） | `make docker-build-app && compose up -d --force-recreate app` |
| `compose.sh` | 核心：up/up-services/down/status/logs/build/rebuild-all/profile-up/config | — |
| `README.md`  | 本文档 | — |

顶层脚本只透传参数给 `compose.sh`（`exec bash compose.sh <cmd> "$@"`），逻辑全在核心里。

## 快速上手

```bash
cd <WeKnora 仓库根>                       # 脚本自动上溯 3 级定位，或用 WEKNORA_REPO_ROOT 指定
S=my_docs/prod_scripts

$S/start.sh                # 首次: 自动 docker pull 官方镜像 + 起核心集 + minio + langfuse + 验收
$S/start.sh --build        # 发版: 先本地 build app（frontend 由宿主机自管，不进镜像），再启动
$S/status.sh               # 看状态 / 健康
$S/logs.sh -n              # 日志快照一次（不跟随）
$S/logs.sh app             # 只看 app（跟踪）
$S/stop.sh                 # 停机（含 minio langfuse，数据卷保留）

# ---- 日常发版 ----
git pull                   # 或切目标 commit
$S/rebuild.sh              # 本地 build app + 重建运行中容器（frontend 由宿主机自管）
$S/status.sh
$S/logs.sh app -n

# ---- 关闭默认 profile（按需）----
$S/start.sh --no-langfuse                 # 只用 minio（例如 langfuse 未部署时）

# ---- 追加其它可选 profile（.env 有对应字段时才建议启用）----
$S/compose.sh profile-up qdrant           # RETRIEVE_DRIVER=qdrant 时
$S/compose.sh profile-up neo4j            # ENABLE_GRAPH_RAG=true 时
$S/compose.sh profile-up milvus weaviate  # 对应向量库
$S/compose.sh profile-up dex              # OIDC_AUTH_ENABLE=true 时
$S/compose.sh profile-up searxng          # 开启 Web 搜索
$S/compose.sh config                        # 看合并后的编排
```

## 设计要点（与方案文档对应）

1. **与 dev 环境互斥**：`start` 前自动探测 `WeKnora-postgres-dev/redis-dev` 是否在跑，
   在则告警（5432/6379 冲突会让生产容器起不来），提示先 `make dev-stop`。
2. **本地构建 vs 官方镜像分工（2026-09-29 对齐 make dev-start；2026-09-30 frontend 改宿主机自管）**：
   - **只有 `app` 本地构建**（`SELF_BUILT_SERVICES`），走 `_build_self_built()`，
     版号注入 `VERSION/COMMIT_ID/BUILD_TIME/GO_VERSION`（对齐 `make docker-build-app`）。
   - **frontend(nginx + dist) 由宿主机自管**：前端包与 nginx 配置都不进镜像，本脚本
     不 build、不启动 `WeKnora-frontend`。需自行编译前端（`pnpm run build` 出 dist）、
     配好 nginx，并把 `/api` 反代到 `app:8080`。
   - **其它一切镜像走官方 pull**（`wechatopenai/weknora-docreader:latest`、
     `paradedb/paradedb`、`redis:7.0-alpine`、`minio/minio`、`clickhouse/clickhouse-server`、
     `langfuse/langfuse*`、`qdrant/qdrant`、`neo4j/neo4j` 等）。`up` 前自动执行
     `ensure_official_images`（依 profile 感知），`docker pull` 失败会明确告警而不是让
     `docker compose up` 在中段崩掉。
   - **up 命令绝不用 `--build` / `--pull`**：`up -d --build` 会把所有带 `build:` 的服务
     （含 docreader）都重建，违背 docreader 走官方的策略；`--pull` 会把官方 `wechatopenai/
     weknora-app:latest` 拉到本地覆盖自编译镜像。均改为「先预拉官方 + 按需显式 build
     app」两步组合。
3. **默认 profile 集对齐 make dev-start**：`DEFAULT_PROFILES="minio langfuse"`。minio
   因 `STORAGE_TYPE=minio` 时后端 `initFileService` 启动查 bucket 强依赖，缺它 panic；
   langfuse 是 dev-start 默认开的可观测栈（.env 配 `LANGFUSE_*KEY` 后 app 自动上报）。
   可 `--no-minio` / `--no-langfuse` 关闭。`down` 对称带默认 profile，避免 minio/langfuse
   容器残留、占住网络；`down --all` 会把 `PROFILES` 全带上彻底清干净。
4. **锁定编排文件**：`export COMPOSE_FILE=docker-compose.yml`，避免同目录
   `docker-compose.dev.yml`/`*.override.yml` 干扰。
5. **端口体检**：`up` 读 `.env` 的 `APP_PORT`(默认8080) 被宿主占用时告警；`status` 另 curl
   `FRONTEND_PORT`(默认80) 探测宿主机自管的 nginx（只读探测，不影响本栈）。本机 8080 常态被占，
   正式部署建议改 `.env` 的 `APP_PORT`。
6. **健康验收**：`up` 分两段拉起（先基础设施层、再 app 层）实现失败隔离；按次等
   `postgres`→`docreader`→`app` healthy，app 未就绪只告警不回滚已起容器。
7. **迁移**：依赖 app 容器启动时 `AUTO_MIGRATE=true` 自跑；不额外加宿主 migrate 步骤，
   保持与 `make docker-run` 行为一致。手动迁移 `cd <repo> && make migrate-up`。
8. **profile 白名单**：`profile-up` 仅限真实存在的 profile（minio qdrant milvus weaviate
   neo4j doris dex searxng langfuse odl-hybrid full），拼错即报错。profile 名与服务名不一一对应
   （doris→doris-fe/doris-be，langfuse→langfuse-* 五个，full 会一并启用 sandbox/mcp 等
   常驻/按需 build 服务），由脚本内部映射处理。
9. ** frontend dist 前置（已移除）**：frontend 已改宿主机自管，本栈不再 build 前端容器；
   `compose.sh build/rebuild-all` 传 frontend 会明确报错。前端编译请自行在宿主执行
   （`cd frontend && pnpm install --frozen-lockfile && pnpm run build`）并配 nginx 反代。
10. **镜像源**：`/etc/docker/daemon.json` 建议配 `registry-mirrors`（本机
   `https://docker.1panel.live`），配合 `data-root=/data/docker` 把大镜像（paradedb
   ~2.4GB / docreader ~1GB）落到 `/data` 分区，避免挤占系统盘。

## 注意事项

- **前置环境**：运行用户必须在 `docker` 组（`sudo usermod -aG docker $USER`，
  重新登录生效）。frontend 由宿主机自管：前端编译需本机 `node`/`pnpm`，nginx 需自行部署并
  反代 `/api` 到 `http://<app 容器>:8080`。
- **DB 配置已按「.env=生产 / .env.local=dev 覆盖」分工（2026-09-29 切换）**：
  两个 compose 的 `POSTGRES_DB/POSTGRES_USER` 都取自 `.env` 的 `DB_NAME/DB_USER`。
  现 `.env` 为生产值：`DB_HOST=postgres`（生产 postgres 容器不发布宿主端口，
  app 容器内用容器名）、`DB_NAME=weknora_prod`；`scripts/dev.sh` 的
  `load_env_files` 会在 `.env` 之后 source `.env.local`，其中覆盖回
  `DB_HOST=localhost` / `DB_NAME=weknora_mdev`，供本地 `make dev-app` 使用
  （dev 容器卷里旧库已初始化，`POSTGRES_DB` 只对空卷生效，切换不影响 dev 数据）。
  生产与 dev 不能同时运行（见下条）；改 `.env` 的 `DB_*` 后若要 dev app 照常连
  dev 库，请同步维护 `.env.local`。
- **与 dev 环境精确隔离**：脚本固定 `COMPOSE_PROJECT_NAME=weknora-prod`（dev 用目录名
  `weknora`），故 `ps/logs/down` 只命中生产容器，`down -v` 也绝不会误删 dev 的卷。
  容器名仍为 `WeKnora-app/...`（由 `container_name` 固定，与 project 无关；
  `WeKnora-frontend` 已不在本栈启动），因此**生产与 dev 不能同时运行**（同名容器 + 5432/6379 端口冲突），起生产前先 `make dev-stop`。
  数据卷名随 project，生产在 `weknora-prod_postgres-data` 等，与 dev 的 `weknora_postgres-data` 天然分离。
- **仓库根定位**：脚本默认 `脚本目录/../..`（本目录 `my_docs/prod_scripts/` 上 2 级即仓库根）；若把本目录拷走，`export
  WEKNORA_REPO_ROOT=<WeKnora 根>` 显式指定，避免路径猜错。
- **`.env` 前置**：`up/config` 前若 `.env` 缺失会从 `.env.example` 复制并告警——上线前
  务必逐项核对 `DB_*`、密钥（`JWT_SECRET`/`TENANT_AES_KEY`/`SYSTEM_AES_KEY`）、LLM、`STORAGE_TYPE`。
- **危险操作**：`stop.sh --purge-data` = `docker compose down -v`，删数据卷不可恢复。
- **Ollama/LLM 不在 compose 内**：按需宿主自起，`.env` 的 `OLLAMA_BASE_URL` 等指向对应地址
  （容器内可用 `host.docker.internal` 连宿主 Ollama）。
