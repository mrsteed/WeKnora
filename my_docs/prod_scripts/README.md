# WeKnora 生产运维脚本（prod_scripts）

参照 `BeadForge/deploy_prod` 的「顶层薄封装 + 核心脚本」结构，落地
《20260929-02-生产模式编译启动与运维命令方案.md》中的全部命令。

## 文件清单

| 文件 | 作用 | 等价 make / docker 命令 |
|------|------|------------------------|
| `start.sh`   | 启动（默认先补 build 再 up 核心集 + 健康验收） | `make docker-build-all && make docker-run` |
| `stop.sh`    | 停止删除容器（数据卷保留） | `make docker-stop` |
| `status.sh`  | 容器状态 + app /health + frontend HTTP 探测 | `docker compose ps` + curl |
| `logs.sh`    | 日志（默认跟 app/docreader/postgres） | `docker compose logs -f` |
| `rebuild.sh` | 发版：重建镜像 + 重建运行中容器（数据不动） | `make docker-build-all && compose up -d --force-recreate` |
| `compose.sh` | 核心：up/up-services/down/status/logs/build/rebuild-all/profile-up/config | — |
| `README.md`  | 本文档 | — |

顶层脚本只透传参数给 `compose.sh`（`exec bash compose.sh <cmd> "$@"`），逻辑全在核心里。

## 快速上手

```bash
cd <WeKnora 仓库根>                       # 脚本自动上溯 3 级定位，或用 WEKNORA_REPO_ROOT 指定
S=my_docs/prod_scripts

$S/start.sh            # 首次: 构建 app/docreader/frontend + 起核心五服务 + 验收
$S/status.sh           # 看状态 / 健康
$S/logs.sh -n          # 日志快照一次（不跟随）
$S/logs.sh app         # 只看 app（跟踪）
$S/stop.sh             # 停机（数据卷保留）

# ---- 日常发版 ----
git pull               # 或切目标 commit
$S/rebuild.sh          # 重建三镜像 + 重建运行中容器
$S/status.sh
$S/logs.sh app -n

# ---- 只改前端 ----
$S/rebuild.sh frontend

# ---- 可选能力（profile）----
$S/compose.sh profile-up langfuse minio     # 起观测/对象存储
$S/compose.sh build app                     # 只重建某镜像
$S/compose.sh config                         # 看合并后的编排
```

## 设计要点（与方案文档对应）

1. **与 dev 环境互斥**：`start` 前自动探测 `WeKnora-postgres-dev/redis-dev` 是否在跑，
   在则告警（5432/6379 冲突会让生产容器起不来），提示先 `make dev-stop`。
2. **绝不 `--pull`**：`build`/`rebuild` 一律 `--pull=false`、`up` 不带 pull，
   避免拉官方 `wechatopenai/weknora-app:latest` 覆盖本地自建镜像。
3. **锁定编排文件**：`export COMPOSE_FILE=docker-compose.yml`，避免同目录
   `docker-compose.dev.yml`/`*.override.yml` 干扰。
4. **端口体检**：`up`/`status` 读 `.env` 的 `FRONTEND_PORT`(默认80)/`APP_PORT`(默认8080)，
   被宿主占用时告警（本机 80/8080 常态被占，正式部署建议改 `.env`）。
5. **健康验收**：`up` 后依次等 `postgres`→`docreader`→`app` healthy，再 curl frontend；
   任一容器 `exited/dead` 立即 die 并指向 `logs`。
6. **迁移**：依赖 app 容器启动时 `AUTO_MIGRATE=true` 自跑；不额外加宿主 migrate 步骤，
   保持与 `make docker-run` 行为一致。手动迁移 `cd <repo> && make migrate-up`。
7. **profile 白名单**：`profile-up` 仅限真实存在的 profile（minio qdrant milvus weaviate
   neo4j doris dex searxng opensearch langfuse odl-hybrid sandbox），拼错即报错。

## 注意事项

- **与 dev 环境精确隔离**：脚本固定 `COMPOSE_PROJECT_NAME=weknora-prod`（dev 用目录名
  `weknora`），故 `ps/logs/down` 只命中生产容器，`down -v` 也绝不会误删 dev 的卷。
  容器名仍为 `WeKnora-app/frontend/...`（由 `container_name` 固定，与 project 无关），
  因此**生产与 dev 不能同时运行**（同名容器 + 5432/6379 端口冲突），起生产前先 `make dev-stop`。
  数据卷名随 project，生产在 `weknora-prod_postgres-data` 等，与 dev 的 `weknora_postgres-data` 天然分离。
- **仓库根定位**：脚本默认 `脚本目录/../../..`；若把本目录拷走，`export
  WEKNORA_REPO_ROOT=<WeKnora 根>` 显式指定，避免路径猜错。
- **`.env` 前置**：`up/config` 前若 `.env` 缺失会从 `.env.example` 复制并告警——上线前
  务必逐项核对 `DB_*`、密钥（`JWT_SECRET`/`TENANT_AES_KEY`/`SYSTEM_AES_KEY`）、LLM、`STORAGE_TYPE`。
- **危险操作**：`stop.sh --purge-data` = `docker compose down -v`，删数据卷不可恢复。
- **Ollama/LLM 不在 compose 内**：按需宿主自起，`.env` 的 `OLLAMA_BASE_URL` 等指向对应地址
  （容器内可用 `host.docker.internal` 连宿主 Ollama）。
