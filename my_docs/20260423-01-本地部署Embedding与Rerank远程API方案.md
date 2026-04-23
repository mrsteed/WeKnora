# BGE-M3 与 BGE-Reranker-v2-m3 本地 Remote API 启动方案

## 1. 文档目标

本文档只解决一件事：

- 在本地启动一套可供 WeKnora 通过 Remote API 调用的模型服务

本文只围绕这两个模型展开：

- Embedding：BAAI/bge-m3
- Rerank：BAAI/bge-reranker-v2-m3

本文明确不采用 TEI 方案。

原因很简单：当前目标不是做多套协议适配，也不是分别维护 Embedding 服务和 Rerank 服务，而是用一套最短路径把服务启动起来，并同时提供：

- POST /embeddings
- POST /rerank

对这个目标，最合适的方案是使用 Infinity 作为统一推理服务。

## 2. 最终方案

### 2.1 选型结论

本方案使用 Infinity 统一承载两个模型：

- BAAI/bge-m3
- BAAI/bge-reranker-v2-m3

Infinity 适合本场景的原因：

1. 一套服务即可同时加载 Embedding 和 Rerank 模型。
2. 同时提供 /embeddings 和 /rerank 两个 REST 接口。
3. 启动方式简单，直接用 Docker 即可。
4. 不需要额外再写一层 Python 网关。
5. 比 TEI 更适合当前这两个模型一起对外提供统一 Remote API。

### 2.2 服务形态

启动后，统一服务地址示例：

```text
http://127.0.0.1:7997
```

对外提供的关键接口：

- GET /models
- POST /embeddings
- POST /rerank
- GET /docs

## 3. 部署前提

### 3.0 按当前机器的推荐结论

你的机器配置：

- CPU：Intel Core i9-14900HX
- 内存：32GB
- GPU：NVIDIA GeForce RTX 4080 Laptop GPU
- 显存：12GB

基于这套配置，建议直接使用 GPU 方案，不建议走 CPU 方案。

对这台机器，最合适的启动策略是：

1. 使用单张 GPU，也就是 `device_id=0`
2. 使用 `torch` 引擎，不额外切到 `optimum`
3. 显式使用 `float16`
4. Embedding 与 Rerank 分别使用不同 batch size
5. 首次启动关闭 warmup，优先保证稳定拉起服务

推荐参数如下：

| 项目 | 推荐值 |
|------|--------|
| engine | `torch` |
| device | `cuda` |
| device_id | `0` |
| dtype | `float16` |
| bge-m3 batch-size | `8` |
| bge-reranker-v2-m3 batch-size | `4` |
| model_warmup | `false` |

这样配置的原因是：

- 12GB 显存足够把这两个模型一起拉起来
- 但如果直接用较大的默认 batch，首次 warmup 时更容易出现显存峰值偏高
- 对笔记本 GPU 来说，优先把服务稳定启动成功，比一开始就追求高吞吐更合理

### 3.1 推荐环境

最低建议：

- CPU 可启动
- 内存 16GB 以上
- 磁盘预留 20GB 以上模型缓存空间

推荐环境：

- 1 张 NVIDIA GPU
- 显存 12GB 以上
- 内存 32GB 以上

说明：

- CPU 环境可以启动成功，但推理速度会明显慢一些。
- 如果你的目标是正式接入 WeKnora 并承担知识库检索请求，优先使用 GPU。
- 对 12GB 显存的笔记本 GPU，建议通过较小 batch-size 控制显存峰值。

### 3.2 需要的软件

- Docker
- Docker Compose Plugin

如果使用 GPU，还需要：

- NVIDIA Driver
- NVIDIA Container Toolkit

## 4. 推荐目录

建议在任意运维目录下建立如下结构：

```text
remote-models/
├── docker-compose.yml
└── hf-cache/
```

说明：

- docker-compose.yml：启动 Infinity 服务
- hf-cache：缓存 HuggingFace 模型，避免每次重启重复下载

## 5. 启动方式

### 5.1 当前机器推荐版本

在 remote-models 目录下创建 docker-compose.yml：

```yaml
version: "3.9"

services:
  infinity-api:
    image: michaelf34/infinity:latest
    container_name: infinity-api
    gpus: all
    ports:
      - "7997:7997"
    volumes:
      - ./hf-cache:/app/.cache
    command:
      - v2
      - --engine
      - torch
      - --device
      - cuda
      - --device-id
      - "0"
      - --dtype
      - float16
      - --batch-size
      - "8"
      - --model-id
      - BAAI/bge-m3
      - --served-model-name
      - bge-m3
      - --batch-size
      - "4"
      - --model-id
      - BAAI/bge-reranker-v2-m3
      - --served-model-name
      - bge-reranker-v2-m3
      - --no-model-warmup
      - --port
      - "7997"
    restart: unless-stopped
```

启动命令：

```bash
docker compose up -d
```

这个版本就是最贴合你当前电脑配置的推荐版本。

### 5.2 如果显存仍然不足的保守版本

如果启动时出现显存不足，优先把 batch-size 再降一档，不要先改模型。

可替换为：

```yaml
version: "3.9"

services:
  infinity-api:
    image: michaelf34/infinity:latest
    container_name: infinity-api
    gpus: all
    ports:
      - "7997:7997"
    volumes:
      - ./hf-cache:/app/.cache
    command:
      - v2
      - --engine
      - torch
      - --device
      - cuda
      - --device-id
      - "0"
      - --dtype
      - float16
      - --batch-size
      - "4"
      - --model-id
      - BAAI/bge-m3
      - --served-model-name
      - bge-m3
      - --batch-size
      - "2"
      - --model-id
      - BAAI/bge-reranker-v2-m3
      - --served-model-name
      - bge-reranker-v2-m3
      - --no-model-warmup
      - --port
      - "7997"
    restart: unless-stopped
```

### 5.3 CPU 备用版本

如果没有 GPU，把上面的 compose 改成下面这样：

```yaml
version: "3.9"

services:
  infinity-api:
    image: michaelf34/infinity:latest-cpu
    container_name: infinity-api
    ports:
      - "7997:7997"
    volumes:
      - ./hf-cache:/app/.cache
    command:
      - v2
      - --engine
      - optimum
      - --model-id
      - BAAI/bge-m3
      - --model-id
      - BAAI/bge-reranker-v2-m3
      - --port
      - "7997"
    restart: unless-stopped
```

启动命令同样是：

```bash
docker compose up -d
```

### 5.4 单命令启动方式

如果你不想写 compose，也可以直接用 docker run。

GPU 版本：

```bash
docker run -d \
  --name infinity-api \
  --gpus all \
  -p 7997:7997 \
  -v $PWD/hf-cache:/app/.cache \
  michaelf34/infinity:latest \
  v2 \
  --engine torch \
  --device cuda \
  --device-id 0 \
  --dtype float16 \
  --batch-size 8 \
  --model-id BAAI/bge-m3 \
  --served-model-name bge-m3 \
  --batch-size 4 \
  --model-id BAAI/bge-reranker-v2-m3 \
  --served-model-name bge-reranker-v2-m3 \
  --no-model-warmup \
  --port 7997
```

CPU 版本：

```bash
docker run -d \
  --name infinity-api \
  -p 7997:7997 \
  -v $PWD/hf-cache:/app/.cache \
  michaelf34/infinity:latest-cpu \
  v2 \
  --engine optimum \
  --model-id BAAI/bge-m3 \
  --model-id BAAI/bge-reranker-v2-m3 \
  --port 7997
```

## 6. 启动验证

### 6.1 查看容器状态

```bash
docker ps | grep infinity-api
```

### 6.2 查看模型是否加载完成

```bash
docker logs -f infinity-api
```

第一次启动会下载模型，耗时取决于网络和磁盘速度。

在你这台机器上，首次启动时间通常主要花在：

- 模型下载
- 模型权重加载到 GPU

如果日志里没有 OOM，后续重启一般会明显更快。

### 6.3 查看 Swagger 文档

浏览器打开：

```text
http://127.0.0.1:7997/docs
```

### 6.4 查看模型列表

```bash
curl http://127.0.0.1:7997/models
```

正常情况下应能看到：

- BAAI/bge-m3
- BAAI/bge-reranker-v2-m3
- 或者你显式设置的别名：`bge-m3`、`bge-reranker-v2-m3`

## 7. 接口自检

### 7.1 Embedding 自检

```bash
curl http://127.0.0.1:7997/embeddings \
  -X POST \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "bge-m3",
    "input": ["你好，WeKnora", "向量检索测试"]
  }'
```

预期结果：

1. 返回 HTTP 200
2. 返回 JSON 中包含 data 数组
3. data[0].embedding 为浮点数组

### 7.2 Rerank 自检

```bash
curl http://127.0.0.1:7997/rerank \
  -X POST \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "bge-reranker-v2-m3",
    "query": "知识库如何检索",
    "documents": [
      "系统先做 embedding 召回，再做 rerank 重排。",
      "今天是个好天气。"
    ]
  }'
```

预期结果：

1. 返回 HTTP 200
2. 返回 JSON 中包含 results 数组
3. 与 query 更相关的文档分数更高

## 8. 给 WeKnora 使用时的地址

本文档不展开 WeKnora 页面配置，只给出最终服务地址规则。

如果 WeKnora 和 Infinity 在同一台宿主机：

- Base URL：`http://127.0.0.1:7997`

如果 WeKnora 也跑在 Docker Compose 网络里：

- Base URL：`http://infinity-api:7997`

后续在 WeKnora 中配置模型时：

- Embedding 模型名填写：`bge-m3`
- Rerank 模型名填写：`bge-reranker-v2-m3`

说明：

- Infinity 的 Embedding 接口是 /embeddings
- Infinity 的 Rerank 接口是 /rerank
- 两个模型都可以共用同一个服务地址
- 使用 `served-model-name` 后，WeKnora 侧配置会更短，也更不容易填错模型名

## 9. 常见问题

### 9.1 第一次启动很慢

这是正常现象。首次启动需要从 HuggingFace 下载两个模型。

### 9.2 为什么本文不用 TEI

因为本次目标是：

- 只围绕 BAAI/bge-m3 和 BAAI/bge-reranker-v2-m3
- 直接启动可用的 Remote API 服务
- 尽量减少协议分裂和额外适配工作

Infinity 用一套服务就能同时满足 Embedding 和 Rerank，因此更适合本文目标。

### 9.3 CPU 能不能跑

能跑，但响应速度会慢很多。只适合功能验证，不适合正式承载检索流量。

### 9.4 这台 12GB 显存的 4080 Laptop 能不能同时跑两个模型

可以，本文的推荐配置就是按这类显存规模写的。

如果出现显存不足，按下面顺序处理：

1. 把 batch-size 从 `8/4` 降到 `4/2`
2. 保持 `--no-model-warmup`
3. 确认没有其他程序占用 GPU 显存

### 9.5 容器删了之后模型要重新下载吗

如果你保留了 hf-cache 挂载目录，就不需要重新下载。

## 10. 最短执行步骤

如果你只想最快把服务拉起来，按下面做即可：

1. 创建 remote-models 目录
2. 新建 docker-compose.yml，内容使用本文 5.1
3. 执行 `docker compose up -d`
4. 用 `curl http://127.0.0.1:7997/models` 验证服务
5. 用本文的 /embeddings 和 /rerank 示例请求做联调

做到这里，这套 Remote API 服务就已经可用了。
