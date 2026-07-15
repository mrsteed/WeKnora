# WeKnora 本机部署与本地模型方案

## 1. 目标与本次修正

本文档基于当前主机资源与 WeKnora 代码结构，给出一套更贴近你要求的本机部署方案。相较上一版，核心调整如下：

- 不再使用 TEI 服务。
- 不再部署 Ollama 服务。
- Embedding 与 Rerank 统一改为 Infinity 部署。
- BAAI/bge-m3 与 BAAI/bge-reranker-v2-m3 固定部署在 GPU2，同卡运行。
- 增加 Qwen3-VL 视频/视觉模型方案，参考 docs/WeKnora/docker-compose.yml_qwen3-vl-llamacpp。
- 增加 ASR 模型选型与本地部署方案。
- WeKnora 启动时显式开启 MinIO 与 Neo4j。
- 输出统一的模型编排文件和启动/关闭脚本到当前目录。

## 2. 当前机器资源结论

当前主机资源：

- CPU：128 线程
- 内存：123 GiB
- GPU：4 x RTX 4090 24 GiB
- 数据盘：/data 约 7.2 TiB 可用

这套机器完全适合做本地完整部署。为避免编号歧义，本文统一使用 nvidia-smi 的 0-based 编号，也就是：

- device 0 = 第 1 张卡
- device 1 = 第 2 张卡
- device 2 = 第 3 张卡
- device 3 = 第 4 张卡

本次建议的显卡分配如下：

| NVIDIA device_id | 组件 | 说明 |
| --- | --- | --- |
| 0 + 1 | Qwen 大语言模型 | 主对话模型，vLLM 承载 |
| 2 | Infinity + Qwen3-VL + ASR | bge-m3、bge-reranker-v2-m3、视觉模型、语音模型共用 |
| 3 | 预留空闲 | 当前不部署任何模型 |

说明：

- 你要求的“把原 GPU3 的模型也部署到 GPU2，上面统一改成 device 2 共用”。
- 如果按自然语言计数，这意味着前 3 张卡使用，第 4 张卡空出来。
- 由于 GPU2 同时承载 Infinity、Qwen3-VL、ASR，建议 ASR 先从较轻模型开始。

## 3. WeKnora 代码结构与能力边界

### 3.1 核心启动链路

- WeKnora 标准启动入口在 WeKnora/cmd/server/main.go。
- 启动期自动迁移数据库、初始化存储与模型相关依赖的逻辑在 WeKnora/internal/container/container.go。
- 内置模型配置在启动时通过 WeKnora/config/builtin_models.yaml 注入。

因此，生产上更稳的方式不是手工进页面逐个创建模型，而是直接维护 builtin_models.yaml。

### 3.2 模型接入边界

WeKnora 当前支持的接入模式里，本次方案会用到以下 4 条：

- LLM：OpenAI 兼容接口
- Embedding：OpenAI 兼容 /embeddings
- Rerank：OpenAI 风格 /rerank
- ASR：OpenAI 兼容 /v1/audio/transcriptions

与代码对应关系：

- 对话模型：WeKnora/internal/models/chat/remote_api.go
- Embedding：WeKnora/internal/models/embedding/openai.go
- Rerank：WeKnora/internal/models/rerank/remote_api.go
- ASR：WeKnora/internal/models/asr/openai.go

### 3.3 视频模型的真实边界

这一点必须讲清楚：

- WeKnora 当前原生多模态入口是图像/VLM。
- 前端当前直接暴露的是图片上传与 VLM 模型配置。
- 因此“视频模型”在这套方案里的准确含义，是部署一个 Qwen3-VL 服务，用于视频关键帧、封面图、截图序列的理解。

也就是说：

- 能做：视频抽帧后送入 Qwen3-VL 做理解、摘要、问答。
- 不能直接宣称：WeKnora 当前已经原生支持整段视频文件无预处理地走 VLM 推理。

所以本文档会把“视频模型”写成“Qwen3-VL 视频关键帧/截图理解方案”。

## 4. 推荐部署拓扑

推荐把系统拆成两部分：

### 4.1 模型服务栈

- qwen-vllm：主 LLM
- infinity-api：Embedding + Rerank
- qwen3-vl-llamacpp：视觉/视频关键帧理解
- speaches-asr：语音识别

### 4.2 WeKnora 业务栈

- frontend
- app
- docreader
- postgres
- redis
- minio
- neo4j

本次不启用：

- ollama
- TEI
- qdrant
- milvus
- weaviate

## 5. 模型选型

### 5.1 主对话模型

你提到的 Qwen3.6-27B 这里统一按“Qwen 27B 级 Instruct 本地模型”处理。

推荐落地方式：

- 推理框架：vLLM
- 模型目录：/data/models/llm/qwen-local-27b
- 端口：8000
- 服务名：qwen-local-27b

建议优先使用：

- 27B/32B 级 Instruct 的量化版
- 2 卡张量并行

### 5.2 Embedding 模型

推荐：BAAI/bge-m3

原因：

- 中英混合检索表现稳定
- 维度 1024，适合知识库场景
- Infinity 已明确验证支持该模型

### 5.3 Rerank 模型

推荐：BAAI/bge-reranker-v2-m3

原因：

- 中文场景更稳
- Infinity 原生支持 rerank
- 可与 bge-m3 同卡部署

### 5.4 视频/视觉模型

推荐：Qwen/Qwen3-VL-8B-Instruct-GGUF

推理框架：llama.cpp server-cuda

原因：

- 你已经给了现成参考样例 docs/WeKnora/docker-compose.yml_qwen3-vl-llamacpp
- 用 GGUF + llama.cpp 对 4090 更友好
- 适合做图像理解、截图理解、视频关键帧理解

### 5.5 ASR 模型选型

ASR 服务推荐用 Speaches，因为它是 OpenAI API 兼容的本地 STT/TTS 服务，和 WeKnora 的 ASR 客户端最匹配。

推荐分三档：

| 档位 | 模型名 | 适合场景 |
| --- | --- | --- |
| 默认均衡 | Systran/faster-whisper-medium | 中文会议转写、共享 GPU 场景 |
| 高精度英语优先 | distil-whisper/distil-large-v3 | 英文长音频、速度和精度平衡 |
| 最高精度多语种 | Systran/faster-whisper-large-v3 | 多语种、资源充足场景 |

默认建议先用：

- Systran/faster-whisper-medium

原因：

- 放在 device 2 与 Infinity、Qwen3-VL 共存时更稳
- 显存占用更保守
- 首次部署更不容易爆显存

## 6. /data 目录规划

建议统一使用如下目录：

```text
/data/
├── docker/
├── models/
│   ├── hf-cache/
│   │   └── qwen3-vl-llamacpp/
│   ├── llm/
│   │   └── qwen-local-27b/
│   ├── embedding/
│   └── rerank/
└── weknora/
  │   ├── app/
  │   │   └── files/
  │   ├── docreader/
  │   │   └── tmp/
  │   ├── postgres/
  │   ├── redis/
  │   ├── minio/
  │   ├── neo4j/
  │   └── langfuse/
    ├── model-logs/
    │   └── qwen3-vl-llamacpp/
```

说明：

- WeKnora 业务栈容器统一挂载到 `/data/weknora`。
- 模型服务不并入这个规则：模型缓存与模型本体继续放在 `/data/models`，仅视觉模型日志保留在 `/data/weknora/model-logs`。
- 三个目录的关系、管理员创建脚本与模型服务地址清单，见 `docs/WeKnora/本地栈目录与账号模型清单.md`。

初始化目录：

```bash
sudo mkdir -p /data/docker
sudo mkdir -p /data/models/hf-cache
sudo mkdir -p /data/models/hf-cache/qwen3-vl-llamacpp
sudo mkdir -p /data/models/llm/qwen-local-27b
sudo mkdir -p /data/models/embedding
sudo mkdir -p /data/models/rerank
sudo mkdir -p /data/weknora/app/files
sudo mkdir -p /data/weknora/docreader/tmp
sudo mkdir -p /data/weknora/postgres
sudo mkdir -p /data/weknora/redis
sudo mkdir -p /data/weknora/minio
sudo mkdir -p /data/weknora/neo4j
sudo mkdir -p /data/weknora/model-logs/qwen3-vl-llamacpp
sudo chown -R $USER:$USER /data/models /data/weknora
```

## 7. 模型服务编排文件

我已经把参考样例合并成一份当前目录下的编排文件：

- docs/WeKnora/docker-compose.local-ai-models.yml

这份文件包含：

- qwen-vllm
- infinity-api
- qwen3-vl-llamacpp
- speaches-asr

### 7.1 服务与端口约定

| 服务 | 端口 | 用途 |
| --- | --- | --- |
| qwen-vllm | 8000 | 主 LLM |
| infinity-api | 7997 | Embedding + Rerank |
| qwen3-vl-llamacpp | 18080 | 视觉/视频关键帧理解 |
| speaches-asr | 18000 | ASR |

当前最终 GPU 落点：

- qwen-vllm -> device 0,1
- infinity-api -> device 2
- qwen3-vl-llamacpp -> device 2
- speaches-asr -> device 2
- device 3 空闲保留

### 7.2 Infinity 的接口约定

这里特别说明一下，因为这是本次修正文档的关键点：

- Infinity 的 Embedding 端点是 /embeddings
- Infinity 的 Rerank 端点是 /rerank
- 因此 WeKnora 里配置 Infinity 时，base_url 应写成根路径 http://host.docker.internal:7997

这样 WeKnora 才会正确拼接出：

- Embedding -> http://host.docker.internal:7997/embeddings
- Rerank -> http://host.docker.internal:7997/rerank

## 8. 启动/关闭脚本

本次已经把脚本输出到当前目录：

- docs/WeKnora/start_model_services.sh
- docs/WeKnora/stop_model_services.sh
- docs/WeKnora/start_weknora_stack.sh
- docs/WeKnora/stop_weknora_stack.sh
- docs/WeKnora/start_all_local_stack.sh
- docs/WeKnora/stop_all_local_stack.sh
- docs/WeKnora/create_admin_hlsa_account.sh
- docs/WeKnora/本地栈目录与账号模型清单.md

### 8.1 模型服务脚本用法

启动全部模型服务：

```bash
cd /home/hlsa/workspace/docs/WeKnora
./start_model_services.sh
```

只启动单个服务：

```bash
cd /home/hlsa/workspace/docs/WeKnora
./start_model_services.sh infinity-api
./start_model_services.sh qwen-vllm
./start_model_services.sh qwen3-vl-llamacpp
./start_model_services.sh speaches-asr
```

停止全部模型服务：

```bash
cd /home/hlsa/workspace/docs/WeKnora
./stop_model_services.sh
```

只停止单个服务：

```bash
cd /home/hlsa/workspace/docs/WeKnora
./stop_model_services.sh infinity-api
```

### 8.2 WeKnora 服务脚本用法

启动 WeKnora 业务栈，并启用 MinIO + Neo4j：

```bash
cd /home/hlsa/workspace/docs/WeKnora
./start_weknora_stack.sh
```

停止 WeKnora 业务栈：

```bash
cd /home/hlsa/workspace/docs/WeKnora
./stop_weknora_stack.sh
```

### 8.3 一键全部启动/停止

```bash
cd /home/hlsa/workspace/docs/WeKnora
./start_all_local_stack.sh
./stop_all_local_stack.sh

## 9. 当前 .env 评估

结论：当前的 /home/hlsa/workspace/WeKnora/.env 不需要完全重写，但要做一轮定向修正，否则和这次部署方案不完全一致。

### 9.1 当前 .env 中已经对齐的项

以下配置已经基本符合本次方案：

- GIN_MODE=release
- TZ=Asia/Shanghai
- DB_DRIVER=postgres
- RETRIEVE_DRIVER=postgres
- STORAGE_TYPE=minio
- STREAM_MANAGER_TYPE=redis
- MINIO_ENDPOINT=minio:9000
- MINIO_ACCESS_KEY_ID=minioadmin
- MINIO_SECRET_ACCESS_KEY=minioadmin
- MINIO_BUCKET_NAME=weknora
- DOCREADER_ADDR=docreader:50051
- DOCREADER_TRANSPORT=grpc

### 9.2 当前 .env 里建议修改的项

下面这些项建议改：

| 配置项 | 当前值 | 建议值 | 原因 |
| --- | --- | --- | --- |
| SSRF_WHITELIST | localhost,127.0.0.1,infinity-api | host.docker.internal,localhost,127.0.0.1 | WeKnora app 容器通过宿主机端口访问模型服务，更稳的是 host.docker.internal |
| ENABLE_GRAPH_RAG | false | true | 你要求启用 Neo4j 图谱能力 |
| OLLAMA_OPTIONAL | 未设置 | true | 明确告诉系统不依赖 Ollama |
| MINIO_USE_SSL | 未设置 | false | 本地 MinIO 走 HTTP，更清晰 |
| NEO4J_ENABLE | 注释掉 | true | 启用图谱能力 |
| NEO4J_URI | 注释掉 | bolt://neo4j:7687 | 对接 compose 内的 neo4j 服务 |
| NEO4J_USERNAME | 注释掉 | neo4j | 与 compose 默认一致 |
| NEO4J_PASSWORD | 注释掉 | password | 与 compose 默认一致，后续可再换 |
| WEKNORA_LANGUAGE | 注释掉 | zh-CN | 默认语言明确化 |
| CRYPTO_MASTER_KEY | 未设置 | 建议显式设置 | 便于长期稳定恢复 |
| CRYPTO_SALT | 未设置 | 建议显式设置 | 便于长期稳定恢复 |

### 9.3 当前 .env 中可以保留不动的项

- APP_PORT=8081：可以保留，表示宿主机 API 端口是 8081；如果你更想统一成 8080，可以改，但不是必须。
- OLLAMA_BASE_URL=http://host.docker.internal:11434：即使保留也不会影响本次方案，只要补上 OLLAMA_OPTIONAL=true 即可。
- DB_HOST=localhost：对容器内 app 没有决定作用，因为 compose 里已经固定注入 DB_HOST=postgres；保留也无妨。

## 10. WeKnora 环境变量建议
```

## 9. WeKnora 环境变量建议

### 9.1 基础变量

在 WeKnora/.env 中至少建议设置：

```bash
GIN_MODE=release
TZ=Asia/Shanghai

DB_DRIVER=postgres
DB_USER=postgres
DB_PASSWORD=please-change-this-postgres-password
DB_NAME=WeKnora

REDIS_PASSWORD=please-change-this-redis-password
REDIS_DB=0

RETRIEVE_DRIVER=postgres
STREAM_MANAGER_TYPE=redis

APP_PORT=8080
FRONTEND_PORT=80
DOCREADER_PORT=50051

SSRF_WHITELIST=host.docker.internal
OLLAMA_OPTIONAL=true

TENANT_AES_KEY=replace-with-32-byte-secret-123456
SYSTEM_AES_KEY=replace-with-32-byte-secret-123456
JWT_SECRET=replace-with-random-jwt-secret
CRYPTO_MASTER_KEY=replace-with-32-byte-hex-or-string
CRYPTO_SALT=replace-with-random-salt
```

### 9.2 MinIO 变量

既然这次要求启用 MinIO，建议直接把文件存储切到 MinIO：

```bash
STORAGE_TYPE=minio
MINIO_ENDPOINT=minio:9000
MINIO_ACCESS_KEY_ID=minioadmin
MINIO_SECRET_ACCESS_KEY=minioadmin
MINIO_BUCKET_NAME=weknora
MINIO_USE_SSL=false
```

### 9.3 Neo4j / GraphRAG 变量

```bash
ENABLE_GRAPH_RAG=true
NEO4J_ENABLE=true
NEO4J_URI=bolt://neo4j:7687
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=password
```

### 10.1 建议直接补齐到 .env 的配置块

如果你想直接按命令补齐，建议至少把下面这一段合并进 .env：

```bash
WEKNORA_LANGUAGE=zh-CN
SSRF_WHITELIST=host.docker.internal,localhost,127.0.0.1
OLLAMA_OPTIONAL=true
ENABLE_GRAPH_RAG=true
NEO4J_ENABLE=true
NEO4J_URI=bolt://neo4j:7687
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=password
MINIO_USE_SSL=false
CRYPTO_MASTER_KEY=replace-with-random-32-byte-key
CRYPTO_SALT=replace-with-random-salt
```

## 11. 内置模型配置示例

将以下内容保存为 WeKnora/config/builtin_models.yaml：

```yaml
builtin_models:
  - id: builtin-llm-local-qwen
    name: qwen-local-27b
    type: KnowledgeQA
    source: remote
    is_default: true
    description: Local Qwen LLM served by vLLM
    parameters:
      base_url: http://host.docker.internal:8000/v1
      api_key: local-vllm
      provider: generic

  - id: builtin-embedding-local-bge
    name: bge-m3
    type: Embedding
    source: remote
    is_default: true
    description: Local embedding model served by Infinity on GPU2
    parameters:
      base_url: http://host.docker.internal:7997
      api_key: local-infinity
      provider: generic
      embedding_parameters:
        dimension: 1024
        truncate_prompt_tokens: 0

  - id: builtin-rerank-local-bge
    name: bge-reranker-v2-m3
    type: Rerank
    source: remote
    is_default: true
    description: Local rerank model served by Infinity on GPU2
    parameters:
      base_url: http://host.docker.internal:7997
      api_key: local-infinity
      provider: generic

  - id: builtin-vlm-qwen3-vl
    name: qwen3-vl-8b-q4
    type: VLLM
    source: remote
    is_default: true
    description: Qwen3-VL via llama.cpp for image and video keyframe understanding
    parameters:
      base_url: http://host.docker.internal:18080/v1
      api_key: ""
      provider: generic
      supports_vision: true

  - id: builtin-asr-local-default
    name: Systran/faster-whisper-medium
    type: ASR
    source: remote
    is_default: true
    description: Default ASR model served by Speaches
    parameters:
      base_url: http://host.docker.internal:18000/v1
      api_key: ""
      provider: generic
```

## 12. WeKnora 启动方式

### 12.1 启动模型服务

```bash
cd /home/hlsa/workspace/docs/WeKnora
./start_model_services.sh
```

### 12.2 启动 WeKnora，并开启 MinIO + Neo4j

```bash
cd /home/hlsa/workspace/docs/WeKnora
./start_weknora_stack.sh
```

等价命令是：

```bash
cd /home/hlsa/workspace/WeKnora
docker compose --profile minio --profile neo4j up -d
```

## 13. 当前 WeKnora volumes 约定

当前 `WeKnora/docker-compose.yml` 已改为 bind mount，业务栈统一落到 `/data/weknora`：

### 12.1 app

```yaml
volumes:
  - /data/weknora/app/files:/data/files
  - /data/weknora/docreader/tmp:/tmp/docreader:ro
  - ./config/config.yaml:/app/config/config.yaml
  - ./skills/preloaded:/app/skills/preloaded
  - ./config/builtin_models.yaml:/app/config/builtin_models.yaml:ro
```

### 12.2 docreader

```yaml
volumes:
  - /data/weknora/docreader/tmp:/tmp/docreader
```

### 12.3 postgres

```yaml
volumes:
  - /data/weknora/postgres:/var/lib/postgresql/data
```

### 12.4 redis

```yaml
volumes:
  - /data/weknora/redis:/data
```

### 12.5 minio

```yaml
volumes:
  - /data/weknora/minio:/data
```

### 12.6 neo4j

```yaml
volumes:
  - /data/weknora/neo4j:/data
```

## 14. 验证清单

### 13.1 模型服务

```bash
curl -s http://127.0.0.1:8000/v1/models \
  -H 'Authorization: Bearer local-vllm'

curl -s http://127.0.0.1:7997/embeddings \
  -H 'Authorization: Bearer local-infinity' \
  -H 'Content-Type: application/json' \
  -d '{"model":"bge-m3","input":["你好，WeKnora"]}'

curl -s http://127.0.0.1:7997/rerank \
  -H 'Authorization: Bearer local-infinity' \
  -H 'Content-Type: application/json' \
  -d '{"model":"bge-reranker-v2-m3","query":"知识库是什么","documents":["这是一个知识库","这是天气预报"]}'

curl -s http://127.0.0.1:18080/health

curl -s http://127.0.0.1:18000/health
```

### 13.2 WeKnora 内置模型

```bash
cd /home/hlsa/workspace/WeKnora
docker compose logs app | grep -E 'Built-in model|Built-in models config'
```

### 13.3 业务功能验证顺序

建议按下面顺序验证：

1. 先验证 qwen-vllm、infinity-api、qwen3-vl-llamacpp、speaches-asr 全部可访问。
2. 再启动 WeKnora 业务栈。
3. 登录后确认内置模型列表已经出现 5 类模型：LLM、Embedding、Rerank、VLM、ASR。
4. 新建知识库，确认文件存储使用 MinIO。
5. 上传一批中文文档，验证 Embedding 和 Rerank 都走 Infinity。
6. 在知识库中打开图谱相关配置，验证 Neo4j 可连通。
7. 给智能体开启图片上传，测试 Qwen3-VL。
8. 给智能体开启音频上传，测试 ASR。
9. 如果需要视频理解，先抽关键帧再上传给 Qwen3-VL 测试。

## 15. 推荐与注意事项

### 14.1 推荐

- 先用 PostgreSQL 默认检索，不要一开始叠加额外向量库。
- Infinity 单独占用 GPU2，避免和 LLM 抢资源。
- Qwen3-VL、ASR、Infinity 同放 device 2 时，优先选较轻的 ASR 模型。
- MinIO 与 Neo4j 这次直接纳入标准启动，而不是后补。

### 14.2 注意

- WeKnora 当前多模态主路径是图片，不是整段视频原生推理。
- 如果 device 2 显存压力大，先把 ASR 降到 Systran/faster-whisper-medium，必要时切 CPU 镜像。
- 如果不想启用 ASR，只需要停掉 speaches-asr 服务，并从 builtin_models.yaml 删除对应 ASR 条目。

## 16. 本次输出物

本次已在当前目录输出：

- 更新后的部署文档：docs/WeKnora/WeKnora本机部署与本地模型方案.md
- 组合编排文件：docs/WeKnora/docker-compose.local-ai-models.yml
- 模型服务启动脚本：docs/WeKnora/start_model_services.sh
- 模型服务停止脚本：docs/WeKnora/stop_model_services.sh
- WeKnora 启动脚本：docs/WeKnora/start_weknora_stack.sh
- WeKnora 停止脚本：docs/WeKnora/stop_weknora_stack.sh
- 全栈启动脚本：docs/WeKnora/start_all_local_stack.sh
- 全栈停止脚本：docs/WeKnora/stop_all_local_stack.sh

## 17. 逐步执行命令

下面这组命令按顺序执行，可以在当前机器上完成部署准备与启动。

### 第 1 步：进入目录并备份 .env

```bash
cd /home/hlsa/workspace/WeKnora
cp .env ".env.bak.$(date +%F-%H%M%S)"
```

### 第 2 步：补齐并覆盖本次方案需要的 .env 关键变量

```bash
grep -vE '^(WEKNORA_LANGUAGE|SSRF_WHITELIST|OLLAMA_OPTIONAL|ENABLE_GRAPH_RAG|NEO4J_ENABLE|NEO4J_URI|NEO4J_USERNAME|NEO4J_PASSWORD|MINIO_USE_SSL|CRYPTO_MASTER_KEY|CRYPTO_SALT)=' .env > .env.new
cat >> .env.new <<'EOF'
WEKNORA_LANGUAGE=zh-CN
SSRF_WHITELIST=host.docker.internal,localhost,127.0.0.1
OLLAMA_OPTIONAL=true
ENABLE_GRAPH_RAG=true
NEO4J_ENABLE=true
NEO4J_URI=bolt://neo4j:7687
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=password
MINIO_USE_SSL=false
CRYPTO_MASTER_KEY=replace-with-random-32-byte-key
CRYPTO_SALT=replace-with-random-salt
EOF
mv .env.new .env
```

然后生成两段随机值并手工替换：

```bash
openssl rand -hex 32
openssl rand -base64 32
```

把第一条结果替换 CRYPTO_MASTER_KEY，把第二条结果替换 CRYPTO_SALT。

### 第 3 步：准备 /data 目录

```bash
sudo mkdir -p /data/docker
sudo mkdir -p /data/models/hf-cache
sudo mkdir -p /data/models/hf-cache/qwen3-vl-llamacpp
sudo mkdir -p /data/models/llm/qwen-local-27b
sudo mkdir -p /data/models/embedding
sudo mkdir -p /data/models/rerank
sudo mkdir -p /data/weknora/app/files
sudo mkdir -p /data/weknora/docreader/tmp
sudo mkdir -p /data/weknora/postgres
sudo mkdir -p /data/weknora/redis
sudo mkdir -p /data/weknora/minio
sudo mkdir -p /data/weknora/neo4j
sudo mkdir -p /data/weknora/model-logs/qwen3-vl-llamacpp
sudo chown -R $USER:$USER /data/models /data/weknora
```

### 第 4 步：写入 builtin_models.yaml

```bash
cat > /home/hlsa/workspace/WeKnora/config/builtin_models.yaml <<'EOF'
builtin_models:
  - id: builtin-llm-local-qwen
    name: qwen-local-27b
    type: KnowledgeQA
    source: remote
    is_default: true
    description: Local Qwen LLM served by vLLM
    parameters:
      base_url: http://host.docker.internal:8000/v1
      api_key: local-vllm
      provider: generic

  - id: builtin-embedding-local-bge
    name: bge-m3
    type: Embedding
    source: remote
    is_default: true
    description: Local embedding model served by Infinity on device 2
    parameters:
      base_url: http://host.docker.internal:7997
      api_key: local-infinity
      provider: generic
      embedding_parameters:
        dimension: 1024
        truncate_prompt_tokens: 0

  - id: builtin-rerank-local-bge
    name: bge-reranker-v2-m3
    type: Rerank
    source: remote
    is_default: true
    description: Local rerank model served by Infinity on device 2
    parameters:
      base_url: http://host.docker.internal:7997
      api_key: local-infinity
      provider: generic

  - id: builtin-vlm-qwen3-vl
    name: qwen3-vl-8b-q4
    type: VLLM
    source: remote
    is_default: true
    description: Qwen3-VL via llama.cpp
    parameters:
      base_url: http://host.docker.internal:18080/v1
      api_key: ""
      provider: generic
      supports_vision: true

  - id: builtin-asr-local-default
    name: Systran/faster-whisper-medium
    type: ASR
    source: remote
    is_default: true
    description: Default ASR model served by Speaches
    parameters:
      base_url: http://host.docker.internal:18000/v1
      api_key: ""
      provider: generic
EOF
```

### 第 5 步：启动模型服务

```bash
cd /home/hlsa/workspace/docs/WeKnora
chmod +x start_model_services.sh stop_model_services.sh start_weknora_stack.sh stop_weknora_stack.sh start_all_local_stack.sh stop_all_local_stack.sh
./start_model_services.sh
```

### 第 6 步：验证模型服务

```bash
curl -s http://127.0.0.1:8000/v1/models -H 'Authorization: Bearer local-vllm'

curl -s http://127.0.0.1:7997/embeddings \
  -H 'Authorization: Bearer local-infinity' \
  -H 'Content-Type: application/json' \
  -d '{"model":"bge-m3","input":["你好，WeKnora"]}'

curl -s http://127.0.0.1:7997/rerank \
  -H 'Authorization: Bearer local-infinity' \
  -H 'Content-Type: application/json' \
  -d '{"model":"bge-reranker-v2-m3","query":"知识库是什么","documents":["这是一个知识库","这是天气预报"]}'

curl -s http://127.0.0.1:18080/health
curl -s http://127.0.0.1:18000/health
```

### 第 7 步：启动 WeKnora 业务栈

```bash
cd /home/hlsa/workspace/docs/WeKnora
./start_weknora_stack.sh
```

### 第 8 步：检查 WeKnora 启动结果

```bash
cd /home/hlsa/workspace/WeKnora
docker compose ps
docker compose logs app | grep -E 'Built-in model|Built-in models config'
```

### 第 9 步：检查访问地址

```bash
echo 'UI:  http://localhost'
echo 'API: http://localhost:8081'
echo 'VLM: http://localhost:18080'
echo 'ASR: http://localhost:18000'
echo 'LLM: http://localhost:8000/v1'
echo 'INF: http://localhost:7997'
```

### 第 10 步：如需停止

```bash
cd /home/hlsa/workspace/docs/WeKnora
./stop_all_local_stack.sh
```