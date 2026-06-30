# WeKnora 项目详细介绍

生成日期：2026-06-09

## 1. 项目概述

WeKnora 是一个面向企业知识管理场景的开源知识框架，核心能力覆盖文档理解、语义检索、RAG 问答、ReAct Agent 推理、自动 Wiki 生成、多租户权限管理和私有化部署。

从产品形态看，它不是单纯的 RAG 示例工程，而是一个完整的知识平台。用户可以通过 Web UI、REST API、CLI、IM 渠道、MCP Server、微信小程序等方式接入系统，完成知识库创建、文档上传、知识检索、智能问答、Agent 配置、模型配置和系统管理。

项目主仓库采用 monorepo 组织方式，包含 Go 后端、Vue 前端、Python 文档解析服务、Go CLI、桌面端、小程序、MCP Server、Helm Chart、Docker Compose、数据库迁移和大量文档。

## 2. 核心定位

WeKnora 的主要目标是帮助团队把分散在文件、网页、外部知识平台和 IM 系统中的内容，沉淀为可查询、可推理、可维护的知识资产。

它围绕三个主要场景展开：

1. 快速问答：基于知识库的 RAG 检索增强问答，适合日常知识查询。
2. 智能推理：通过 ReAct Agent 编排知识检索、MCP 工具和网页搜索，处理多步骤任务。
3. 自动 Wiki：从原始文档中生成结构化、相互链接的 Markdown Wiki，并形成知识图谱。

项目还强调企业级能力，包括多租户、RBAC、审计日志、私有化部署、模型与存储可替换、凭据加密、Langfuse 可观测性和 IM 集成。

## 3. 技术栈概览

### 3.1 后端

后端主体使用 Go 编写，入口位于：

- `cmd/server/main.go`
- `internal/router/router.go`
- `config/config.yaml`
- `go.mod`

主要框架与依赖包括：

- Gin：HTTP API 服务
- GORM / pgx：数据库访问
- PostgreSQL / SQLite / MySQL 相关驱动
- Redis / Asynq：任务队列与异步处理
- dig：依赖注入
- OpenTelemetry / Langfuse：链路追踪与可观测性
- 多种向量库 SDK：pgvector、Elasticsearch、OpenSearch、Milvus、Weaviate、Qdrant、Doris、腾讯云 VectorDB
- 多种对象存储 SDK：本地存储、COS、TOS、MinIO、AWS S3、OSS、KS3、OBS
- 多模型调用适配：OpenAI 兼容接口、Ollama、Gemini、腾讯云、NVIDIA 等

### 3.2 前端

前端使用 Vue 3、Vite、TypeScript 和 TDesign Vue Next。

主要文件包括：

- `frontend/src/main.ts`
- `frontend/src/App.vue`
- `frontend/src/router/index.ts`
- `frontend/package.json`

前端主要负责：

- 登录与初始化
- 知识库列表与详情
- 文档管理
- 智能问答页面
- Agent 列表与配置
- 模型、MCP、网页搜索、向量库、解析引擎、存储配置
- 租户、成员、组织管理
- 分享聊天页面

### 3.3 文档解析服务

文档解析能力位于 `docreader` 目录，主要使用 Python 实现。

该服务独立于 Go 后端，承担文档读取、格式转换、解析、OCR、多模态处理、分块前处理等工作。Go 后端通过 gRPC 或 HTTP 调用文档解析服务。

这种拆分方式的好处是：

- 后端主服务保持稳定；
- 解析链路可以独立演进；
- Python 生态更适合处理文档、OCR、模型推理和数据转换；
- 容器部署时可以独立扩缩容。

### 3.4 CLI

项目包含独立的 Go CLI，目录为 `cli`。

CLI 支持：

- 登录认证和多 profile 管理
- 知识库管理
- 文档上传、等待解析、删除
- chunk 检索与调试
- RAG 问答
- Agent 管理
- MCP Server 模式
- 面向 AI Agent 的 JSON / NDJSON 输出协议

CLI 是 WeKnora 的重要扩展面，适合 CI、自动化脚本和 AI 编程工具集成。

### 3.5 其他组件

仓库还包含：

- `cmd/desktop`：Wails 桌面端
- `miniprogram`：微信小程序
- `mcp-server`：独立 MCP Server
- `helm`：Kubernetes Helm 部署
- `dataset`：评测数据集与样例
- `skills/preloaded`：预置 Agent Skills
- `docs`：功能说明、API 文档、开发文档
- `migrations`：数据库迁移

## 4. 仓库结构说明

项目关键目录如下：

```text
.
├── cmd
│   ├── server        # Go 后端服务入口
│   ├── desktop       # 桌面端入口
│   └── download      # 下载相关辅助命令
├── internal          # 后端核心代码
│   ├── agent         # ReAct Agent 推理、prompt、观察与执行
│   ├── application   # service 和 repository
│   ├── handler       # HTTP handler
│   ├── infrastructure# 文档解析、分块、模型、存储、向量库等基础设施适配
│   ├── im            # IM 渠道集成
│   ├── mcp           # MCP 管理
│   ├── middleware    # 认证、RBAC、审计、日志等中间件
│   ├── router        # API 路由注册
│   ├── runtime       # 启动与运行时管理
│   └── types         # 领域类型与接口
├── frontend          # Vue 前端
├── docreader         # Python 文档解析服务
├── cli               # WeKnora 命令行工具
├── mcp-server        # 独立 MCP Server
├── migrations        # 数据库迁移
├── docs              # 项目文档
├── helm              # Kubernetes 部署
├── docker            # Docker 镜像构建文件
├── config            # 默认配置与 prompt 模板
├── miniprogram       # 微信小程序
├── dataset           # 样例数据集
└── skills            # 预置 Agent Skills
```

## 5. 后端架构

后端采用较典型的分层结构：

1. Router 层负责注册 HTTP 路由、中间件和公共入口。
2. Handler 层负责接收请求、参数解析和响应格式。
3. Service 层负责业务逻辑。
4. Repository 层负责数据库访问。
5. Infrastructure 层负责外部系统适配。
6. Types / Interfaces 定义领域对象和接口边界。

### 5.1 启动流程

服务入口是 `cmd/server/main.go`。

启动时主要流程为：

1. 根据环境变量设置 Gin 运行模式。
2. 打印启动环境信息。
3. 构建依赖注入容器。
4. 执行启动 bootstrap，例如系统管理员初始化。
5. 注入配置、路由、追踪器、资源清理器等组件。
6. 启动 HTTP Server。
7. 监听系统信号，执行优雅关闭。

### 5.2 路由与中间件

路由集中在 `internal/router/router.go`。

主要中间件包括：

- CORS
- Request ID
- Language
- Logger
- Recovery
- Error Handler
- Auth
- RBAC
- Langfuse tracing

主要 API 分组包括：

- 认证
- 租户与成员
- 知识库
- 知识条目
- FAQ
- Chunk
- 会话与聊天
- 消息
- 模型
- 评估
- MCP 服务
- 网页搜索
- 向量库
- 自定义 Agent
- Skills
- 组织树
- IM 渠道
- 数据源
- Wiki 页面
- 导出
- 系统设置

### 5.3 业务服务

业务逻辑集中在 `internal/application/service`。

重要服务包括：

- `knowledgebase.go`：知识库管理
- `knowledge.go`、`knowledge_create.go`、`knowledge_process.go`：知识创建、处理、入库
- `knowledgebase_search*.go`：知识库检索、混合检索、跨向量库 fanout
- `session_knowledge_qa.go`：基于知识库的问答
- `session_agent_qa.go`：Agent 问答
- `agent_service.go`：Agent 服务
- `wiki_ingest*.go`、`wiki_page.go`：Wiki 入库与页面管理
- `model.go`：模型管理
- `vectorstore.go`：向量库管理
- `mcp_service.go`：MCP 服务管理
- `web_search.go`：网页搜索
- `tenant.go`、`tenant_member.go`、`organization.go`：租户与组织管理
- `audit_log.go`：审计日志

## 6. 核心功能模块

### 6.1 知识库管理

知识库是系统的核心资源。用户可以创建不同类型的知识库，并向其中导入文档、FAQ、URL 或手工 Markdown 内容。

支持的知识来源包括：

- 本地文件上传
- URL 导入
- 手工 Markdown
- FAQ
- 外部数据源同步
- 结构化数据库接入

知识导入后会经历解析、切分、向量化、索引、摘要、问题生成等流程。

### 6.2 文档解析与分块

文档解析由 Go 后端和 Python docreader 协作完成。

处理链路大致为：

1. 用户上传文件或提交 URL。
2. 后端创建知识条目与处理任务。
3. docreader 解析文件内容。
4. 后端根据配置进行文本清洗、分块和索引。
5. 调用 embedding 模型生成向量。
6. 写入向量库、数据库和对象存储。
7. 更新知识处理状态与追踪信息。

分块能力位于 `internal/infrastructure/chunker`，支持标题层级、启发式切分、token 控制、诊断和调试。

### 6.3 RAG 检索问答

RAG 问答链路通常包括：

1. 用户提问。
2. 根据配置进行 query rewrite 或 query expansion。
3. 执行关键词检索、向量检索、混合检索或图谱增强检索。
4. 对候选 chunk 进行融合、过滤和 rerank。
5. 构建上下文 prompt。
6. 调用大模型生成答案。
7. 返回答案、引用、调试信息和会话记录。

配置项位于 `config/config.yaml` 的 `conversation` 部分，例如：

- `embedding_top_k`
- `vector_threshold`
- `rerank_top_k`
- `rerank_threshold`
- `enable_rewrite`
- `enable_query_expansion`
- `enable_rerank`
- `fallback_strategy`

### 6.4 ReAct Agent

Agent 模块位于 `internal/agent` 和 `internal/application/service/agent*`。

Agent 支持：

- 多轮思考与执行
- 工具调用
- 知识检索
- MCP 工具
- 网页搜索
- final answer 生成
- Langfuse 追踪
- 自定义 Agent 配置
- Agent 可见性与共享

它适合处理比普通 RAG 更复杂的问题，例如需要查找多个知识库、调用外部工具、分步骤分析和综合回答的任务。

### 6.5 Wiki 模式

Wiki 模式是 WeKnora 的重要特色能力。

它可以从原始文档中自动生成结构化 Markdown 页面，并建立页面之间的链接关系。相关代码集中在：

- `internal/application/service/wiki_ingest.go`
- `internal/application/service/wiki_ingest_batch.go`
- `internal/application/service/wiki_linkify.go`
- `internal/application/service/wiki_lint.go`
- `internal/application/service/wiki_page.go`
- `internal/agent/prompts_wiki.go`

Wiki 模式适合将大量散乱文档整理成可浏览、可检索、可持续维护的知识库。

### 6.6 多模型管理

项目支持多家模型厂商与 OpenAI 兼容接口。

常见模型来源包括：

- OpenAI
- Azure OpenAI
- Anthropic
- DeepSeek
- Qwen
- 智谱
- 混元
- 豆包
- Gemini
- MiniMax
- NVIDIA
- Novita AI
- SiliconFlow
- OpenRouter
- Ollama

系统支持在租户、知识库或 Agent 层面配置模型，并支持内置模型声明式配置。

### 6.7 向量库与检索后端

WeKnora 支持多种向量库与检索后端：

- PostgreSQL pgvector
- Elasticsearch
- OpenSearch
- Milvus
- Weaviate
- Qdrant
- Apache Doris
- 腾讯云 VectorDB

从代码结构看，项目对向量库做了抽象，允许用户在不同部署环境下选择不同后端。

### 6.8 存储系统

文件和图片等二进制资源支持多种存储方式：

- 本地文件系统
- 腾讯云 COS
- 火山引擎 TOS
- MinIO
- AWS S3
- 阿里云 OSS
- 金山云 KS3
- 华为云 OBS

对象存储能力对文档预览、图片引用、导入导出和私有化部署都很关键。

### 6.9 多租户、RBAC 与审计

项目具备较完整的企业权限模型。

主要能力包括：

- 多租户工作区
- Owner / Admin / Contributor / Viewer 四级角色
- 知识库资源归属
- 租户成员管理
- 邀请机制
- 跨租户系统管理员
- 租户审计日志
- 组织树与成员管理

相关代码分布在：

- `internal/handler/tenant*.go`
- `internal/handler/organization.go`
- `internal/handler/org_tree.go`
- `internal/middleware/rbac.go`
- `internal/application/service/tenant*.go`
- `internal/application/service/organization.go`
- `internal/application/service/org_tree.go`

### 6.10 IM 与外部集成

WeKnora 支持多个 IM 渠道接入，让用户在聊天工具中直接使用知识问答能力。

支持或规划的渠道包括：

- 企业微信
- 飞书
- Slack
- Telegram
- 钉钉
- Mattermost
- 微信

IM 相关代码位于 `internal/im` 和 `internal/handler/im.go`。

### 6.11 MCP 与 Agent Skills

项目支持 MCP 工具集成和预置 Skills。

MCP 能力包括：

- 内置 MCP 服务
- 外部 MCP 服务管理
- MCP 工具调用
- MCP 工具审批
- CLI 以 MCP Server 方式运行

Skills 位于：

- `skills/preloaded`
- `examples/skills`

它们可以扩展 Agent 的工具能力，例如数据处理、文档协作、引用生成等。

### 6.12 可观测性

项目集成 Langfuse 和 OpenTelemetry。

可观测内容包括：

- Agent 运行过程
- Token 使用
- 工具调用
- 文档解析任务
- 处理阶段 span
- 系统日志与错误追踪

文档解析追踪时间线是项目近期重要能力之一，前端组件包括：

- `frontend/src/components/knowledge-processing-timeline.vue`

## 7. 前端结构与页面

前端以 `frontend/src/router/index.ts` 为主要路由入口。

主要页面包括：

- `/login`：登录
- `/platform/knowledge-bases`：知识库列表
- `/platform/knowledge-bases/:kbId`：知识库详情
- `/platform/chat/:chatid`：聊天会话
- `/platform/agents`：Agent 列表
- `/platform/settings`：设置中心
- `/platform/organizations`：组织列表
- `/platform/admin`：管理后台
- `/share/agents/:shareCode`：Agent 分享聊天

设置中心包含：

- 通用设置
- 用户资料
- 租户信息
- 租户成员
- 模型设置
- Ollama 设置
- MCP 设置
- 网页搜索设置
- 向量库设置
- 存储引擎设置
- 解析引擎设置
- WeKnora Cloud 设置
- 聊天历史设置
- 系统信息

前端状态管理使用 Pinia，主要 store 位于 `frontend/src/stores`。

## 8. 数据库与迁移

数据库迁移文件位于 `migrations`。

当前仓库中 SQL 迁移文件数量较多，说明系统的数据模型持续演进，覆盖了：

- 初始化表结构
- Agent
- 知识库
- Chunk
- 消息与会话
- MCP
- 数据源
- 向量库
- Wiki
- 租户与组织
- 审计日志
- 模型配置
- 长文档任务
- Agent 分享
- 系统设置

迁移命令由 Makefile 和 `scripts/migrate.sh` 提供：

```bash
make migrate-up
make migrate-down
make migrate-version
make migrate-create name=your_migration_name
```

## 9. 部署方式

### 9.1 Docker Compose

默认推荐使用 Docker Compose。

核心文件：

- `docker-compose.yml`
- `.env.example`
- `docker/Dockerfile.app`
- `frontend/Dockerfile`
- `docker/Dockerfile.docreader`

默认核心服务包括：

- `frontend`：Web UI
- `app`：Go 后端
- `docreader`：文档解析服务
- `postgres`：数据库
- `redis`：缓存与任务队列

可选 profile 包括：

- `full`：完整功能
- `neo4j`：知识图谱
- `minio`：对象存储
- `langfuse`：链路追踪

典型启动方式：

```bash
cp .env.example .env
docker compose up -d
```

### 9.2 开发模式

Makefile 提供了快速开发命令：

```bash
make dev-start
make dev-app
make dev-frontend
```

这种模式下，基础设施由 Docker 启动，后端和前端在本地运行，适合频繁开发调试。

### 9.3 Lite 模式

项目支持 Lite 模式，用于更轻量的本地运行或桌面端打包。

相关命令包括：

```bash
make build-lite
make run-lite
make package-lite
```

### 9.4 Kubernetes

`helm` 目录提供 Kubernetes 部署能力，适合生产环境或私有云部署。

## 10. 开发与测试

### 10.1 后端测试

Go 测试覆盖较广，仓库中存在大量 `*_test.go` 文件，覆盖 handler、service、repository、middleware、agent、chunker、wiki、rbac 等模块。

运行方式：

```bash
make test
```

或：

```bash
go test ./...
```

### 10.2 前端测试与构建

前端 package scripts 包括：

```bash
pnpm dev
pnpm build
pnpm type-check
pnpm test
```

项目使用 Vue 3、TypeScript、Vite 和 vue-tsc。

### 10.3 CLI 测试

CLI 有独立的单元测试、契约测试和 e2e 测试，目录包括：

- `cli/cmd`
- `cli/acceptance/contract`
- `cli/acceptance/e2e`
- `cli/acceptance/testdata`

CLI 设计了面向 AI Agent 的稳定 wire contract，这是项目工程化程度较高的体现。

## 11. 项目规模观察

粗略统计显示，项目规模较大：

- Go 文件约 1200+ 个
- Go 测试文件约 370+ 个
- SQL 迁移文件约 180+ 个
- Vue 文件约 140+ 个
- TypeScript 文件约 90+ 个
- Python 文件约 60+ 个

从规模和模块覆盖看，WeKnora 已经超过普通开源示例项目，更接近可私有化部署的企业应用平台。

## 12. 主要优势

### 12.1 功能完整

项目同时覆盖知识导入、文档解析、RAG、Agent、Wiki、权限、审计、部署、CLI 和 IM 集成，产品闭环较完整。

### 12.2 架构可扩展

模型、向量库、对象存储、文档解析、网页搜索、MCP 工具等都存在抽象层，方便按部署环境替换组件。

### 12.3 私有化部署友好

Docker Compose、Helm、本地 Lite、桌面端等形态说明项目重视不同环境下的落地。

### 12.4 企业能力较强

多租户、RBAC、审计、凭据加密、SSRF 防护、API Key、OIDC、系统管理员等能力适合企业内部使用。

### 12.5 AI Agent 集成意识强

CLI 的 wire contract、MCP Server、Skills、工具审批等设计都说明项目不仅服务人类用户，也服务 AI Agent 自动化场景。

## 13. 潜在复杂点

### 13.1 部署依赖多

完整功能依赖数据库、Redis、文档解析服务、向量库、对象存储、模型服务、可选 Neo4j、Langfuse 等组件。生产部署前需要明确最小功能集。

### 13.2 配置面较大

模型、向量库、存储、租户、MCP、网页搜索、解析引擎、Langfuse 等配置项较多，新团队接手时需要建立配置基线。

### 13.3 业务链路长

文档入库链路涉及上传、解析、分块、embedding、索引、状态更新、追踪、错误恢复。排障时需要同时看数据库、任务队列、docreader、对象存储和向量库。

### 13.4 权限模型需要重点理解

多租户、组织树、知识库可见性、资源归属、系统管理员和普通租户角色之间的关系较复杂，修改相关逻辑时需要充分测试。

### 13.5 多后端适配带来测试压力

支持多模型、多向量库、多存储会带来组合复杂度。新增功能时要确认默认后端和非默认后端行为是否一致。

## 14. 适合的使用场景

WeKnora 适合：

- 企业内部知识库问答
- 客服知识库
- 产品文档智能助手
- 研发文档检索与总结
- 运维知识库
- 规章制度问答
- 多部门知识共享平台
- 私有化 RAG 平台
- 需要 Agent 工具调用的知识工作流
- 将大量文档整理为 Wiki 的知识工程场景

不太适合：

- 只需要极简单文件问答的小脚本场景
- 不希望维护任何后端基础设施的轻量个人使用场景
- 完全不需要权限、租户、审计、模型管理的单用户 demo 场景

## 15. 快速上手路径

建议新开发者按以下顺序理解项目：

1. 阅读 `README_CN.md`，理解产品能力。
2. 阅读 `docker-compose.yml`，理解运行依赖。
3. 阅读 `cmd/server/main.go`，理解服务启动流程。
4. 阅读 `internal/router/router.go`，理解 API 分组。
5. 阅读 `internal/application/service/knowledge_create.go` 和 `knowledge_process.go`，理解知识入库。
6. 阅读 `internal/application/service/session_knowledge_qa.go`，理解 RAG 问答。
7. 阅读 `internal/agent`，理解 Agent 推理。
8. 阅读 `frontend/src/router/index.ts`，理解前端页面结构。
9. 根据具体任务深入 handler、service、repository 或 infrastructure。

## 16. 总结

WeKnora 是一个工程化程度较高的企业知识平台。它以 Go 后端为主干，Vue 前端提供管理与交互界面，Python docreader 承担复杂文档解析，CLI 和 MCP 扩展自动化与 AI Agent 使用场景。

项目的核心价值在于把 RAG、Agent、Wiki、多租户权限和私有化部署整合到同一个平台中。它适合需要长期维护知识资产、支持多团队协作、并希望将大模型能力接入内部文档体系的组织。

对于开发者来说，理解该项目的关键不是单点技术，而是掌握几条主链路：

- 文档如何进入系统；
- 内容如何被解析、切分、向量化和索引；
- 用户提问如何经过检索、重排和模型生成；
- Agent 如何调用工具并形成最终答案；
- 租户、角色和资源权限如何约束整个系统；
- 部署环境如何把后端、前端、docreader、数据库、Redis 和可选组件连接起来。

掌握这些链路后，再进入具体模块开发会顺畅很多。
