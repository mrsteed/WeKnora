# WeKnora 编译与运行依赖

> 本文聚焦**从源码编译**与**本地直接运行**二进制所需的环境依赖（区别于 Docker Compose 部署形态）。
> 来源：`Makefile`、`go.mod`、`cli/go.mod`、`docker/Dockerfile.app`、`docker/Dockerfile.docreader`、`docreader/pyproject.toml`、`frontend/package.json`、`mcp-server/pyproject.toml`、`scripts/*.sh`、`cmd/desktop/`。

---

## 1. 项目组件与语言

| 组件 | 路径 | 语言/栈 | 产出 |
|------|------|---------|------|
| 主应用（server） | `cmd/server` | Go 1.26（CGO 必需） | `WeKnora` 二进制 |
| Lite 单文件版 | 同上，`-tags sqlite_fts5`，`EDITION=lite` | Go 1.26 + CGO | `WeKnora-lite` |
| CLI | `cli/` | Go（独立 module） | `weknora` |
| docreader 文档解析 | `docreader/` | Python ≥ 3.10.18 | gRPC 服务（:50051） |
| 前端 | `frontend/` | Vue 3 + Vite 7 + TypeScript，Node.js | `frontend/dist` 静态资源 |
| MCP Server | `mcp-server/` | Python ≥ 3.10 | `tencent-weknora-mcp` |
| Desktop 桌面端 | `cmd/desktop/` | Go + Wails v2（仅 macOS 打包脚本 `make package-mac-app`） | `.app` |
| anydoc 解析引擎（可选） | `third_party/anydoc-go` | Rust（cgo 静态库 `libanydoc_go.a`） | 静态归档（~30MB） |

---

## 2. 宿主机工具链（编译必需）

### 2.0 依赖总览（按任务裁剪）

| 我要做 | 必须工具链 | 可选工具链 |
|--------|-----------|-----------|
| 只编译后端二进制 | 2.1 Go + CGO（gcc + libsqlite3-dev）+ git/curl/make | 2.3 Rust（anydoc 引擎） |
| 编译 + 本地运行（标准版） | 上行全部 | 2.2 Docker/Compose（起 postgres/redis/docreader）；Chromium（网页抓取） |
| 构建前端 / Lite 内嵌前端 | 2.2 Node.js | — |
| 本地编译运行 docreader | 2.4 Python + uv + protoc + LibreOffice 栈 | — |
| 构建 CLI（`cli/`） | 2.1 Go | system keyring 库（`zalando/go-keyring`，Linux 需 `libsecret`） |
| macOS 桌面应用 | 2.1 Go + Xcode 命令行工具 | 2.3 Wails CLI |
| 手动 DB 迁移 / 文档生成 / lint | — | golang-migrate / swag / golangci-lint |

### 2.1 Go 工具链（核心，必装）

| 项 | 要求 |
|----|------|
| **Go 版本** | **≥ 1.26**（根 module 与 `cli/go.mod` 均为 `go 1.26.0`；Docker 构建镜像 `golang:1.26-bookworm`） |
| **C 编译器** | gcc 或 clang（CGO 全程启用：Makefile `build-prod`/`build-lite` 显式 `CGO_ENABLED=1`）。依赖 CGO 的库：`mattn/go-sqlite3`、`duckdb-go`、`pganalyze/pg_query_go`、`asg017/sqlite-vec-go-bindings/cgo`、anydoc cgo 绑定 |
| **libsqlite3 开发库** | 必须**含 FTS5**（Debian bookworm+ / Ubuntu 22.04+ 默认满足）。用途：`DB_DRIVER=sqlite`、Lite（`sqlite_fts5` tag + `initFTS5` 内容表）、sqlite-vec 向量扩展 |
| **git / curl / make / bash** | 构建脚本、`go mod download`、`scripts/*.sh`、Makefile |

安装命令（Linux/macOS）：

```bash
# Go 1.26
# Linux（tarball 或包管理器）
wget https://go.dev/dl/go1.26.linux-amd64.tar.gz && sudo tar -C /usr/local -xzf go1.26.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
# macOS
brew install go

# CGO 与 FTS5
sudo apt install -y build-essential git curl make libsqlite3-dev     # Debian/Ubuntu
brew install git curl make                                            # macOS（sqlite3 系统自带）

# 验证 FTS5
sqlite3 :memory: "CREATE VIRTUAL TABLE t USING fts5(x);" && echo OK   # 需另装 sqlite3 CLI
```

Go 模块环境变量（国内/离线环境按需；与 `docker/Dockerfile.app` 的构建参数一一对应）：

| 变量 | 说明 |
|------|------|
| `GOPROXY` | 模块代理，如 `https://goproxy.cn,direct`（Docker 构建经 `GOPROXY_ARG` 传入） |
| `GOPRIVATE` | 私有仓库前缀（`GOPRIVATE_ARG`） |
| `GOSUMDB` | 校验库；构建镜像中默认 `off`（`GOSUMDB_ARG=off`） |

### 2.2 前端工具链（构建 Web UI / Lite 内嵌前端时必需）

| 项 | 要求 |
|----|------|
| **Node.js** | 建议 20/22+（devDependencies 按 `@tsconfig/node22`、`@types/node@^22`） |
| **npm** | 随 Node 自带；必须用 `npm ci`（package-lock 锁定，`scripts/build_frontend_dist.sh`） |
| 关键构建依赖 | Vite ^7.3.5、`@vitejs/plugin-vue` 6、Vue ^3.5、TypeScript ~6.0.3、vue-tsc、less、tsx、npm-run-all2 |

```bash
brew install node                       # 或 nvm install 22
cd frontend && npm ci && npm run build  # 产物 frontend/dist
```

注意：`frontend/packages/xlsx-0.20.2.tgz` 为本地 tgz 依赖（`"xlsx": "file:./packages/xlsx-0.20.2.tgz"`），**不可删**，否则 `npm ci` 失败。

### 2.3 Rust 工具链（可选，仅 anydoc 引擎）

| 项 | 要求 |
|----|------|
| rustup + cargo + rustc | stable 即可（`--profile minimal --default-toolchain stable`，与 `Dockerfile.app` 一致） |
| 支持目标三元组 | `x86_64-apple-darwin`、`aarch64-apple-darwin`、`x86_64-pc-windows-msvc`、`{x86_64,aarch64}-unknown-linux-gnu`、`{x86_64,aarch64}-unknown-linux-musl`（见 `scripts/build-anydoc-lib.sh`，其他目标直接报错退出） |

```bash
curl https://sh.rustup.rs -sSf | sh -s -- -y --profile minimal --default-toolchain stable
make anydoc-lib        # 产物 third_party/anydoc-go 下 ~30MB 静态归档（libanydoc_go.a / anydoc_go.lib）
make build-anydoc      # -tags anydoc 链接
# 交叉构建：TARGET=aarch64-unknown-linux-musl make anydoc-lib
```

说明：`build-anydoc-lib.sh` 会从 `rsproxy.cn`（回退 `static.crates.io`）拉取 pinned 的 anydoc crate 并 patch `document_to_markdown`，需**可访问外网**（或提前预置 `~/.cargo/registry`）。跳过本项时 anydoc 引擎在设置页显示不可用，不影响其他解析引擎。

### 2.4 Python 工具链（本地编译/运行 docreader 时必需）

| 项 | 要求 |
|----|------|
| **Python** | **≥ 3.10**（Docker 按 3.10.18-bookworm 锁定） |
| **uv** | 推荐；Docker 构建流程：`pip install uv` → `python -m uv sync --locked --no-dev`（`docreader/uv.lock` 锁定） |
| **protoc** | **3.19.4**（gRPC 代码生成，`docreader/scripts/generate_proto.sh`）。离线方式：`packages/protoc-3.19.4-linux-{x86_64,aarch_64}.zip`；也支持 venv 内 `grpcio-tools` 替代 |
| 系统库（Debian bookworm 参考，docreader Dockerfile 实际安装集） | `libjpeg62-turbo libgl1 libglib2.0-0 antiword libfontconfig1 libcairo2 libdbus-glib-1-2 libcups2 libglu1-mesa libsm6 libxinerama1 tar dpkg gnupg` + **`libreoffice`（office 转 docx/pdf 必需）** + **`openjdk-17-jre-headless`（LibreOffice 运行）** + `curl wget` |
| playwright 浏览器 | `python -m playwright install webkit && python -m playwright install-deps webkit`（网页解析） |
| gRPC 健康检查 | `grpc_health_probe` v0.4.24（amd64/arm64/arm 三架构二进制，compose 健康检查用；本地直跑可省略） |

```bash
# Linux 系统依赖
sudo apt install -y libjpeg62-turbo libgl1 libglib2.0-0 antiword libfontconfig1 \
    libcairo2 libdbus-glib-1-2 libcups2 libglu1-mesa libsm6 libxinerama1 \
    libreoffice openjdk-17-jre-headless

cd docreader
pip install uv && python -m uv sync --locked --no-dev
bash scripts/generate_proto.sh        # 或等价的 grpc_tools 生成
python -m playwright install-deps webkit && python -m playwright install webkit
python -m uv run -m docreader.main    # gRPC :50051
```

### 2.5 容器运行时（本地运行标准版时必需）

| 项 | 要求 |
|----|------|
| **Docker Engine** | 现代版本（支持 Compose v2、buildx 多平台 `linux/amd64 | linux/arm64`） |
| **Docker Compose v2** | `docker compose` 插件优先；`scripts/dev.sh` 自动探测 `docker compose` / `docker-compose` 两种形态 |
| 用途 | `make dev-start` 拉起 postgres（paradedb/pg17）、redis 7、docreader、（默认）minio/langfuse 等基础设施；`make docker-build-*` 打镜像（`--platform` 按 `uname -m` 自动选 amd64/arm64） |

### 2.6 Go 辅助 CLI（按需）

| 工具 | 安装 | 用途 |
|------|------|------|
| **golang-migrate** | `go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest` | `make migrate-up/down/version/create/force/goto`（`scripts/migrate.sh`，读 `.env` 构造 `postgres://...?sslmode=disable`）。常规运行 `AUTO_MIGRATE=true` 进程内自动迁移，**无需**此工具 |
| **swag** | `go install github.com/swaggo/swag/cmd/swag@latest` | `make docs` 重新生成 `docs/swagger.json/yaml`（`swag init -g cmd/server/main.go --parseDependency --parseInternal`） |
| **golangci-lint** | `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest` 或包管理器 | `make lint` |
| **wails v2 CLI** | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` | 仅 `make package-mac-app`（macOS 桌面应用；还需 Xcode 命令行工具） |

### 2.7 其他可选工具

| 工具 | 说明 |
|------|------|
| `nc`（netcat） | `scripts/dev.sh` 端口探活；缺失时静默跳过探测，不影响流程 |
| `sqlite3` CLI | 验证 FTS5、排查 `DB_DRIVER=sqlite` 数据 |
| `protobuf` 官方 zip | `packages/` 离线 protoc（见 2.4） |
| 系统 keyring（Linux） | `cli/` 使用 `zalando/go-keyring` 存取凭证：Debian/Ubuntu 需 `libsecret-1-0`（`libsecret-dev`）；无 keyring 会话时 CLI 凭证功能降级 |

### 2.8 平台 / 架构支持

| 平台 | 后端编译 | Rust anydoc | 备注 |
|------|----------|-------------|------|
| Linux amd64 / arm64 | ✅（gnu 或 musl 均可） | ✅ | Makefile `uname -m` 自动映射 Docker `--platform`：`x86_64→linux/amd64`、`aarch64/arm64→linux/arm64`，其他默认 amd64 |
| macOS Intel / Apple Silicon | ✅ | ✅ | `CGO_LDFLAGS=-Wl,-no_warn_duplicate_libraries`（Dockerfile/Makefile 已按 `uname=Darwin` 条件注入） |
| Windows amd64 (MSVC) | 构建支持 | ✅（`x86_64-pc-windows-msvc`，产物 `anydoc_go.lib`） | 官方发行形态为 Linux/macOS；Windows 下 CGO 需 MSVC 工具链 |
| 交叉编译 | CGO 限制：Go 交叉编译需目标平台 C 工具链；anydoc 静态库按目标三元组**单独**构建（`TARGET=<triple> make anydoc-lib`） | | 多平台发行推荐走 Docker 多阶段构建（`Dockerfile.app`） |

### 2.9 CGO 编译参数（Makefile 已内置，手动构建需复刻）

```
CGO_ENABLED=1
CGO_CFLAGS="-Wno-deprecated-declarations"
CGO_LDFLAGS="-Wl,-no_warn_duplicate_libraries"   # 仅 macOS (Darwin)
LDFLAGS="-X '.../handler.Version=$VERSION'        # scripts/get_version.sh 注入
         -X '.../handler.Edition=standard|lite'
         -X '.../handler.CommitID=$COMMIT_ID'
         -X '.../handler.BuildTime=$BUILD_TIME'
         -X '.../handler.GoVersion=$GO_VERSION'
         -X 'google.golang.org/protobuf/reflect/protoregistry.conflictPolicy=warn'"
go build -tags "<anydoc|sqlite_fts5>" -ldflags="-w -s $LDFLAGS" -o WeKnora ./cmd/server
```

- 构建 tag：默认无 tag；`anydoc`（需先 `make anydoc-lib`）；`sqlite_fts5`（Lite 固定携带）。
- `conflictPolicy=warn` 不可省略：qdrant/milvus protoreflect 注册冲突会导致启动 panic。
- 版本信息：`scripts/get_version.sh` 取 `git describe --tags --abbrev=0`、commit、时间、`go version`。

---

## 3. 后端（Go 主应用）

### 3.1 构建

```bash
make build          # 默认构建（无 tag）
make build-anydoc   # 先构建 anydoc Rust 静态库再带 -tags anydoc 构建
make build-lite     # 前端 → web/，再 CGO + -tags sqlite_fts5 构建 WeKnora-lite
make build-prod     # 生产构建（含版本号注入、-w -s 裁剪）
go run cmd/download/duckdb/duckdb.go   # 构建期下载 DuckDB 扩展（make download_spatial）
```

### 3.2 第三方 Go 依赖中影响运行环境的项

| 依赖 | 运行环境含义 |
|------|--------------|
| `gorm.io/driver/postgres` / `mysql` / `sqlite` | **主数据库**三选一：`DB_DRIVER=postgres`（默认，paradedb/pg17 镜像或自建 PG ≥ 15）、`mysql`、`sqlite`（Lite/单机） |
| `milvus-io/milvus/client`、`qdrant/go-client`、`weaviate-go-client`、`elastic/go-elasticsearch v7/v8`、`opensearch-go`、`tencent/vectordatabase-sdk`、`volcengine/vikingdb` | **向量库**（`RETRIEVE_DRIVER`，默认 postgres；其他需自备实例） |
| `redis/go-redis` + `hibiken/asynq` | **Redis**（`STREAM_MANAGER_TYPE=redis` 时必需；Lite 用 `memory` 免 Redis）；Asynq 任务队列复用同一 Redis |
| `neo4j-go-driver` | Neo4j（仅 `NEO4J_ENABLE=true` 时必需） |
| `duckdb-go`、`parquet-go` | 进程内分析，需 `make download_spatial` 下载的 duckdb 文件（`~/.duckdb`，镜像中已复制） |
| `chromedp/chromedp` | 网页抓取需要本机 **Chromium/Chrome**（镜像设 `WEKNORA_EXPORT_CHROME_BIN=/usr/bin/chromium`；Linux 本地运行需自装 chromium） |
| `sqlite-vec-go-bindings`、`pgvector-go` | 向量检索（CGO，sqlite 走 cgo 扩展） |
| `yanyiwu/gojieba` | 中文分词词典随二进制复制（镜像拷贝 `/go/pkg/mod/github.com/yanyiwu/`；本地运行注意同目录数据） |
| 各对象存储 SDK（COS/TOS/S3/OBS/OSS/MinIO/KS3） | 按 `STORAGE_TYPE` 选用的云/自托管对象存储 |
| `ollama/ollama` | Ollama SDK（连 `OLLAMA_BASE_URL`，`OLLAMA_OPTIONAL=true` 默认缺失仅告警） |
| 飞书/Lark、钉钉、Slack、企微 SDK | IM 渠道功能，按需 |

### 3.3 直接运行（本地，非 Docker）

前置：`make dev-start`（起 postgres/redis/docreader 等容器）或自备等效实例。

```bash
# 1. 配置
cp .env.example .env   # 编辑必填项：DB_*、REDIS_ADDR/PASSWORD、SYSTEM_AES_KEY(32字节)

# 2. 运行
make run               # go build 后 ./WeKnora
# 或 dev 模式：make dev-app（已 anydoc-lib 时自动链接 anydoc）

# 3. 数据库迁移（可选；AUTO_MIGRATE=true 时进程内自动完成）
make migrate-up        # 需要 migrate 工具 + 可达的 Postgres
```

运行期关键环境变量（必填）：`DB_DRIVER/DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME`、`REDIS_ADDR`（redis 模式）、`SYSTEM_AES_KEY`（32 字节）；LLM/Embedding 模型可在 UI 配置或通过 `config/builtin_models.yaml`（`${ENV}` 占位符从环境读取）声明。

Lite 模式：`make run-lite`（需 `.env.lite`），零外部依赖：SQLite + 内存队列。

---

## 4. docreader（Python 文档解析服务）

| 项 | 要求 |
|----|------|
| Python | **≥ 3.10.18**（按 3.10.18 构建） |
| 依赖管理 | `uv`（`uv sync --locked`，`docreader/uv.lock`） |
| 系统库（Debian bookworm 镜像已装） | `libjpeg62-turbo`、`libgl1`、`libglib2.0-0`、**LibreOffice**（office 转换）、**antiword**（.doc）、**OpenJDK 17 headless**（LibreOffice 运行）、`libfontconfig1`、`libcairo2`/`libdbus-glib-1-2`/`libcups2`/`libglu1-mesa`/`libsm6`/`libxinerama1`（playwright/图形栈） |
| Playwright | 需下载浏览器：`python -m playwright install webkit` + `playwright install-deps webkit`（网页解析） |
| gRPC 健康检查 | `grpc_health_probe` v0.4.4/v0.4.24（按 amd64/arm64/arm 下载二进制） |
| protoc | 3.19.4（构建期生成 gRPC 代码；可离线 `packages/`） |

Python 依赖（`docreader/pyproject.toml` 关键项）：`grpcio(-tools/-health-checking)`、`protobuf`、`markitdown[docx,pdf,xls,xlsx,pptx]`、`opendataloader-pdf`、`pypdfium2`/`pypdf`、`python-docx`/`openpyxl`/`xlrd`/`pandas`、`trafilatura`、`beautifulsoup4`/`lxml`/`markdownify`、`pillow`、`textract`、`playwright`、`ebooklib`、`requests`、`pydantic`。

运行：`uv run -m docreader.main`（gRPC :50051；app 侧 `DOCREADER_ADDR=docreader:50051`）。

---

## 5. 前端（frontend/）

| 项 | 要求 |
|----|------|
| Node.js | LTS，建议 20/22+（dev deps 按 `@types/node@22`） |
| 包管理 | npm（`npm ci`，package-lock 锁定） |
| 关键 dev 依赖 | Vite ^7.3、`@vitejs/plugin-vue` 6、Vue ^3.5、TypeScript ~6.0、vue-tsc、less、tsx、npm-run-all2 |
| 构建产物 | `vite build` → `frontend/dist`（nginx 镜像 `COPY dist`；Lite 复制到 `web/`） |
| 运行环境变量 | `VITE_IS_DOCKER`（默认 true，Docker 内配置走运行时 config）、`VITE_FRONTEND_COMMIT`、`MAX_FILE_SIZE_MB`（nginx 请求体上限）、`DEFAULT_LOCALE` |

命令：开发 `npm run dev`（`make dev-frontend`）；生产 `npm run build`（`scripts/build_frontend_dist.sh`）。

---

## 6. MCP Server（可选组件）

- Python **≥ 3.10**（3.10–3.12），依赖：`mcp>=2,<3`、`requests`、`starlette`、`uvicorn`（`mcp-server/pyproject.toml`）
- 运行需 WeKnora 可达：`WEKNORA_BASE_URL`、`WEKNORA_API_KEY`；HTTP/SSE 传输必须 `MCP_SERVER_AUTH_TOKEN`（缺失拒绝启动）

---

## 7. Desktop 桌面端（可选，macOS）

- 需要 **Wails v2 CLI**（`go install github.com/wailsapp/wails/v2/cmd/wails@latest`）+ Go + CGO
- `make package-mac-app`：`wails build -clean -tags sqlite_fts5`（macOS 系统环境）
- 依赖 wails.json / 前端资源，产出 `.app`

---

## 8. CI/构建镜像内的工具（无需宿主机安装，仅自建镜像时参考）

`docker/Dockerfile.app` 构建阶段额外引入：`build-essential`、`git`、`libsqlite3-dev`、`migrate`（go install，postgres tag）、Rust 工具链（`WITH_ANYDOC=1` 时）、`scripts/build-anydoc-lib.sh` anydoc 静态库、duckdb 下载脚本。最终运行阶段为 `debian:12.12-slim` + Chromium/pandoc/texlive/ffmpeg/python3+uv/nodejs/libsqlite3-0 等（详见第 2 节与 `docs/环境依赖.md`）。

---

## 9. 最小编译运行清单

**编译主应用（Linux）：**
- [ ] Go 1.26+
- [ ] gcc + `libsqlite3-dev`（含 FTS5）
- [ ] git / make / curl
- [ ] （可选）Rust 工具链（如需 anydoc 引擎）
- [ ] （可选）Node.js 20/22+（如需内置前端资源/Lite）
- [ ] `CGO_ENABLED=1 go build ...`（见 `make build-prod` 参数）

**本地运行（标准版）：**
- [ ] Docker + Compose v2（`make dev-start` 起 postgres/redis/docreader）
- [ ] `.env` 已配置必填项（`DB_*`、`REDIS_ADDR`/`REDIS_PASSWORD`、`SYSTEM_AES_KEY`=32 字节）
- [ ] 至少配置 LLM + Embedding 模型（UI 或 `builtin_models.yaml`）
- [ ] 网页抓取功能需本机 Chromium（`WEKNORA_EXPORT_CHROME_BIN`）

**Lite：**
- [ ] 只需 Go 1.26+、gcc、libsqlite3；`make run-lite` + `.env.lite`，无其他外部依赖

**docreader（若不用镜像）：**
- [ ] Python 3.10+、`uv`、protoc 3.19.4、LibreOffice + antiword + JRE17（office 解析）、playwright webkit
