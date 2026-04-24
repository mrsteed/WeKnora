# MinerU 官方 Docker 镜像服务详细部署文档

## 1. 文档目标

本文专门回答一个更落地的问题：

“如果现在要给 WeKnora 部署一个可长期运行的 MinerU Docker 服务，应该怎么部署、暴露什么接口、用哪个服务入口、如何和现有 WeKnora 架构对接？”

这份文档不再讨论 DeepSeek-OCR-2，也不再停留在“MinerU 能不能接”的架构判断层，而是直接给出面向部署的详细说明。

## 2. 先给结论

结合 MinerU 官方文档、官方 `docker/compose.yaml` 结构，以及当前 WeKnora 的 `mineru_converter.go` 实现，结论如下：

1. 对当前 WeKnora，最合适的 MinerU Docker 服务入口是 `mineru-api`，默认端口 `8000`。
2. 如果后续要做多 GPU 或多 worker 调度，才考虑 `mineru-router`，默认端口 `8002`。
3. `mineru-openai-server` 不是给 WeKnora 当前 parser engine 直连用的主入口，它更适合给 MinerU 自己的 `vlm-http-client` / `hybrid-http-client` 后端提供 OpenAI 兼容推理服务。
4. `mineru-gradio` 是 WebUI，不是 WeKnora 对接所需的核心服务。
5. 对当前仓库版本的 WeKnora，最推荐的部署方式是：
   - 单独部署 `mineru-api`
   - 在 WeKnora 租户解析引擎配置中填写 `mineru_endpoint`
   - 让知识库 `parser_engine_rules` 命中 `mineru`

一句话总结：

“如果你要让 WeKnora 稳定接 MinerU，优先部署 `mineru-api`；只有在高并发或多 GPU 统一入口场景下，再上 `mineru-router`。”

## 3. MinerU 官方 Docker 体系里有哪些服务

MinerU 官方文档和官方 `docker/compose.yaml` 已经把 Docker 形态拆成四类服务：

1. `mineru-openai-server`
2. `mineru-api`
3. `mineru-router`
4. `mineru-gradio`

它们的角色分别是：

### 3.1 `mineru-openai-server`

默认端口：`30000`

用途：

1. 给 MinerU 的 `vlm-http-client` 后端提供 OpenAI 兼容模型服务。
2. 本质上是 MinerU 自己的 VLM 推理入口，而不是文档解析 HTTP API。

这类服务更适合下面的场景：

1. MinerU 本身使用 `vlm-http-client`。
2. 或 `hybrid-http-client` 走外部 OpenAI 兼容推理服务。

### 3.2 `mineru-api`

默认端口：`8000`

用途：

1. 提供 `POST /file_parse` 同步解析接口。
2. 提供 `POST /tasks` 异步任务接口。
3. 提供 `GET /health` 健康检查接口。
4. 提供 `/docs` Swagger 文档界面。

这正是当前 WeKnora 最需要的服务形态。

### 3.3 `mineru-router`

默认端口：`8002`

用途：

1. 给多个本地 worker 或多个 `mineru-api` 做统一入口。
2. 提供和 `mineru-api` 兼容的 `/file_parse`、`/tasks`、`/health`。
3. 适合多 GPU、高并发、负载均衡场景。

### 3.4 `mineru-gradio`

默认端口：`7860`

用途：

1. 提供可视化 WebUI。
2. 方便人工测试解析效果。

它对 WeKnora 后端集成不是必需项。

## 4. 为什么 WeKnora 优先接 `mineru-api`

当前 WeKnora 的接入方式在 [internal/infrastructure/docparser/mineru_converter.go](/home/xmkp/workspace/WeKnora/internal/infrastructure/docparser/mineru_converter.go) 里已经很明确。

它会直接请求：

1. `POST <mineru_endpoint>/file_parse`
2. 表单字段包括：
   - `return_md=true`
   - `return_images=true`
   - `table_enable`
   - `formula_enable`
   - `parse_method`
   - `backend`
   - `lang_list`

这意味着：

1. 当前 WeKnora 并不是对接 OpenAI 兼容接口。
2. 当前 WeKnora 是按 MinerU 的 `file_parse` 语义对接。
3. 所以最自然的入口就是 `mineru-api` 或兼容它的 `mineru-router`。

## 5. 一个重要限制：当前 WeKnora 还不能把 `server_url` 传给 MinerU

这是部署时很容易忽略的点。

MinerU 官方 FastAPI 在 `ParseRequestOptions` 中支持：

1. `backend`
2. `parse_method`
3. `formula_enable`
4. `table_enable`
5. `server_url`

其中 `server_url` 是给：

1. `vlm-http-client`
2. `hybrid-http-client`

这类后端用来连接 OpenAI 兼容服务的。

但当前 WeKnora 的 `mineru_converter.go` 并没有向 `/file_parse` 提交 `server_url` 字段。

这直接带来一个结论：

1. 如果你只是部署一个给 WeKnora 用的 MinerU 服务，当前最稳妥的是 `pipeline`。
2. 如果你想让 MinerU 在 API 内部走 `*-http-client` 后端，当前 WeKnora 侧还缺少 `server_url` 传递链路。

因此，对当前 WeKnora 版本，推荐优先方案仍然是：

1. MinerU 自己在本地容器内完成解析。
2. WeKnora 只把它当成 `file_parse` 服务调用。

## 6. 官方 Docker 部署方式概览

MinerU 官方提供了两种 Docker 思路：

### 6.1 方式一：自己构建 `mineru:latest`

官方中文文档给出的思路是先拉 Dockerfile，再本地构建：

```bash
wget https://gcore.jsdelivr.net/gh/opendatalab/MinerU@master/docker/china/Dockerfile
docker build -t mineru:latest -f Dockerfile .
```

这里有一个关键事实：

官方 Compose 中用到的镜像名是：

```text
mineru:latest
```

也就是说，官方 `compose.yaml` 默认假设你已经提前把 MinerU 镜像构建好了，而不是默认从某个公开 registry 直接拉一个固定 tag。

### 6.2 方式二：下载官方 `compose.yaml`

官方文档给出的 Compose 下载方式是：

```bash
wget https://gcore.jsdelivr.net/gh/opendatalab/MinerU@master/docker/compose.yaml
```

然后按 profile 启动不同服务。

## 7. 面向 WeKnora 的推荐部署方式

如果你的目标是“给 WeKnora 提供一个稳定的 MinerU 文档解析服务”，推荐采用下面的最小部署方案：

1. 构建 `mineru:latest`
2. 使用官方 `compose.yaml`
3. 只启动 `api` profile
4. 对外暴露 `8000`
5. 让 WeKnora 的 `mineru_endpoint` 指向它

这是当前最贴合 WeKnora 的方案。

## 8. 推荐目录结构

建议在宿主机上准备单独部署目录，例如：

```bash
mkdir -p /home/xmkp/workspace/deploy/mineru
cd /home/xmkp/workspace/deploy/mineru
```

建议把结构整理成：

```text
/home/xmkp/workspace/deploy/mineru/
├── compose.yaml
├── Dockerfile
├── .env
├── output/
└── logs/
```

其中：

1. `output/` 用于保存 MinerU API 解析输出。
2. `logs/` 用于挂日志或做排障备份。

## 9. 详细部署步骤

### 9.1 第一步：准备部署目录

```bash
mkdir -p /home/xmkp/workspace/deploy/mineru/output
mkdir -p /home/xmkp/workspace/deploy/mineru/logs
cd /home/xmkp/workspace/deploy/mineru
```

### 9.2 第二步：下载官方 Compose 文件

```bash
wget -O compose.yaml https://gcore.jsdelivr.net/gh/opendatalab/MinerU@master/docker/compose.yaml
```

### 9.3 第三步：下载官方 Dockerfile 并构建镜像

如果你在中国网络环境里，优先用官方文档给出的 `docker/china/Dockerfile`：

```bash
wget -O Dockerfile https://gcore.jsdelivr.net/gh/opendatalab/MinerU@master/docker/china/Dockerfile
docker build -t mineru:latest -f Dockerfile .
```

这一阶段完成后，本机应该存在：

```text
mineru:latest
```

### 9.4 第四步：确认 GPU 运行条件

MinerU 官方 Docker 文档明确提示：

1. Docker 部署主要面向 Linux 和支持 WSL2 的 Windows。
2. 如果要使用 `vllm` 加速 VLM 推理，需要满足 GPU 与驱动前提。
3. Docker 容器必须能访问宿主机 GPU。

推荐先确认：

```bash
docker --version
docker compose version
nvidia-smi
docker info | grep -i nvidia
```

### 9.5 第五步：先启动 `mineru-api`

这是 WeKnora 当前最需要的入口。

```bash
docker compose -f compose.yaml --profile api up -d
```

官方 `compose.yaml` 中，`mineru-api` 的关键结构是：

1. `image: mineru:latest`
2. `container_name: mineru-api`
3. `profiles: ["api"]`
4. `ports: 8000:8000`
5. `entrypoint: mineru-api`
6. `command: --host 0.0.0.0 --port 8000`

这说明官方期望你直接把 `mineru-api` 容器作为 Web API 服务拉起来。

## 10. 推荐的 Compose 定制方式

虽然官方 Compose 能直接用，但对生产环境更建议做两处轻量定制：

1. 把输出目录显式挂出来。
2. 把需要的环境变量显式写出来。

下面是一个更适合 WeKnora 场景的精简版示例：

```yaml
services:
  mineru-api:
    image: mineru:latest
    container_name: mineru-api
    restart: unless-stopped
    ports:
      - "8000:8000"
    environment:
      MINERU_MODEL_SOURCE: local
      MINERU_API_OUTPUT_ROOT: /data/output
      MINERU_API_ENABLE_FASTAPI_DOCS: "true"
    entrypoint: mineru-api
    command:
      - --host
      - 0.0.0.0
      - --port
      - "8000"
    volumes:
      - /home/xmkp/workspace/deploy/mineru/output:/data/output
    ipc: host
    ulimits:
      memlock: -1
      stack: 67108864
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["0"]
              capabilities: [gpu]
    healthcheck:
      test: ["CMD-SHELL", "curl -f http://localhost:8000/health || exit 1"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 120s
```

## 11. 如何验证 MinerU API 是否部署成功

### 11.1 看容器状态

```bash
docker compose -f compose.yaml --profile api ps
```

### 11.2 看日志

```bash
docker compose -f compose.yaml --profile api logs -f mineru-api
```

### 11.3 看健康检查

```bash
curl http://127.0.0.1:8000/health
```

官方 FastAPI 的 `/health` 会返回：

1. `protocol_version`
2. `processing_window_size`
3. `max_concurrent_requests`
4. 任务统计状态

### 11.4 看 Swagger 文档

浏览器打开：

```text
http://<server_ip>:8000/docs
```

## 12. 直接用 curl 验证 `file_parse`

当 API 服务正常后，可以直接做一次同步解析测试：

```bash
curl -X POST http://127.0.0.1:8000/file_parse \
  -F "files=@/path/to/demo.pdf" \
  -F "return_md=true" \
  -F "return_images=true" \
  -F "response_format_zip=false" \
  -F "backend=pipeline" \
  -F "parse_method=ocr" \
  -F "formula_enable=true" \
  -F "table_enable=true" \
  -F "lang_list=ch"
```

如果要看异步任务链路，也可以调用：

```bash
curl -X POST http://127.0.0.1:8000/tasks \
  -F "files=@/path/to/demo.pdf" \
  -F "return_md=true" \
  -F "return_images=true"
```

然后轮询：

```bash
curl http://127.0.0.1:8000/tasks/<task_id>
curl http://127.0.0.1:8000/tasks/<task_id>/result
```

## 13. 和 WeKnora 的对接方式

当前 WeKnora 的接法非常直接。

### 13.1 租户解析引擎配置

在租户解析引擎设置里填写：

```json
{
  "mineru_endpoint": "http://<server_ip>:8000",
  "mineru_model": "pipeline",
  "mineru_enable_formula": true,
  "mineru_enable_table": true,
  "mineru_enable_ocr": true,
  "mineru_language": "ch"
}
```

### 13.2 知识库规则配置

例如：

```json
[
  {
    "file_types": ["pdf", "jpg", "jpeg", "png", "bmp", "tiff", "doc", "docx", "ppt", "pptx"],
    "engine": "mineru"
  }
]
```

### 13.3 当前最推荐的 `mineru_model`

对当前 WeKnora，最稳妥的还是：

```text
pipeline
```

原因不是 MinerU 其他后端不能跑，而是当前 WeKnora 并没有把 `server_url` 传给 MinerU。

这意味着：

1. `vlm-http-client`
2. `hybrid-http-client`

虽然是 MinerU 官方支持的后端，但当前 WeKnora 不能完整驱动这条链路。

## 14. 什么时候考虑 `mineru-router`

如果你后续场景变成下面这样，就不应该只停留在 `mineru-api`：

1. 一台机器多 GPU。
2. 多个 MinerU worker 并发解析。
3. 想给外部系统暴露统一入口。
4. 想用负载均衡方式统一接入多个 `mineru-api`。

此时可以改用：

```bash
docker compose -f compose.yaml --profile router up -d
```

官方文档说明：

1. 默认会以 `--local-gpus auto` 模式在容器内自动拉起本地 worker。
2. 默认统一入口是：

```text
http://<server_ip>:8002/docs
```

3. 如果你不想让 router 自己拉本地 worker，而是想聚合已有 `mineru-api`，可以在 `compose.yaml` 里改成：

```text
--local-gpus none
--upstream-url http://mineru-api:8000
```

### 14.1 WeKnora 能否直接接 router

可以。

因为官方 `mineru-router` 同样暴露兼容的：

1. `/file_parse`
2. `/tasks`
3. `/health`

这意味着你完全可以把 WeKnora 的：

```json
"mineru_endpoint": "http://<server_ip>:8002"
```

指到 router。

## 15. `mineru-openai-server` 在当前架构里的正确定位

这一点很容易误会，所以单独说明。

官方 Docker 文档中：

```bash
docker compose -f compose.yaml --profile openai-server up -d
```

暴露的是：

```text
http://<server_ip>:30000
```

它的定位不是“文档解析 API”，而是“给 MinerU 的 http-client 后端提供 OpenAI 兼容模型服务”。

所以：

1. 它不是当前 WeKnora `mineru_endpoint` 应该优先填写的地址。
2. 它更适合作为 MinerU 内部推理服务或其他客户端的 OpenAI 兼容模型入口。

## 16. 推荐的部署顺序

如果你现在要真正给 WeKnora 上线 MinerU，我建议这样推进：

### 第一阶段：单机 API 先跑通

1. 构建 `mineru:latest`
2. 启动 `mineru-api`
3. 验证 `/health`
4. 验证 `/file_parse`
5. 再把 WeKnora 的 `mineru_endpoint` 指向 `8000`

### 第二阶段：再做知识库级联调

1. 配租户 `mineru_endpoint`
2. 配知识库 `parser_engine_rules`
3. 上传 PDF / 图片 / DOCX 样本
4. 验证 Markdown、图片、公式、表格输出

### 第三阶段：高并发时再升级到 router

1. 需要多 GPU / 多 worker 时，再上 `mineru-router`
2. 让 WeKnora 指向 `8002`
3. 把 `8000` 留给内部 worker 或调试用途

## 17. 最终建议

面向当前 WeKnora 项目，最终建议非常明确：

1. 不要把 MinerU Docker 部署理解成“只要把镜像跑起来就行”。
2. 真正要给 WeKnora 用，首选服务应该是 `mineru-api`。
3. 当前 WeKnora 最稳妥的对接后端仍然是 `pipeline`。
4. 当并发、GPU、统一入口需求上来后，再把 `mineru-router` 引入进来。
5. `mineru-openai-server` 和 `mineru-gradio` 都不是当前 WeKnora parser engine 接入的主入口。

所以，如果你现在就要落地一套能给 WeKnora 用的 MinerU Docker 服务，推荐的最小可行方案就是：

```text
构建 mineru:latest -> 启动 mineru-api -> 验证 /health 与 /file_parse -> 在 WeKnora 中填写 mineru_endpoint
```