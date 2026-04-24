# MinerU 镜像 OCR 能力与 DeepSeek-OCR-2 容器部署方案

## 1. 结论摘要

基于当前仓库代码与部署文件，可以先给出三个直接结论：

1. 当前 WeKnora 仓库里并没有内置或编排一个 MinerU 容器服务，MinerU 在当前架构里是“外部解析服务”，WeKnora 只负责把请求转发给它。
2. 当前仓库内置的 `weknora-docreader` 镜像并不是一个默认启用本地 OCR 的重型镜像，`docker/Dockerfile.docreader` 明确把它定义为“仅文档解析 + 图片提取，无 OCR/VLM”的轻量化镜像。
3. 如果外部 MinerU 服务已经部署完成，那么在 WeKnora 当前实现里，只要解析引擎命中 `mineru` 且配置了 `mineru_endpoint`，OCR 实际上默认就是开启的；只有显式把 `mineru_enable_ocr` 设为 `false` 时，才会降级成非 OCR 解析。

因此，问题不能简单理解成“当前 MinerU 镜像是否开启 OCR”。更准确的说法是：

- WeKnora 自身并不打包 MinerU 镜像。
- WeKnora 当前默认发布物也没有把 MinerU 外部服务完整接好。
- 但 WeKnora 对外部 MinerU 的调用参数里，OCR 缺省值是开启的。

DeepSeek-OCR-2 方面，当前可以确认：

1. 官方仓库提供了源码安装与 `vLLM` / `transformers` 推理脚本。
2. 官方仓库没有看到现成的 Dockerfile 或 docker-compose 方案。
3. 社区存在可运行的 GPU Docker 方案，但它是社区封装，不是官方发布物。

## 2. 当前项目里 MinerU 的真实接入方式

### 2.1 MinerU 不是 DocReader 内部 OCR 插件，而是 Go 侧原生解析引擎

当前主链路里，`mineru` 解析引擎是在 Go 侧注册的原生引擎，不是把请求再转给 Python DocReader 内部 OCR 模块。

关键代码事实：

1. `internal/infrastructure/docparser/engine_registry.go` 中，`mineru` 被定义为 `Go-native, calls self-hosted MinerU API directly`。
2. 引擎可用性检查直接读取 `overrides["mineru_endpoint"]`，说明当前接入依赖的是解析引擎覆盖参数，而不是 DocReader 进程内某个固定环境变量。
3. `internal/application/service/knowledge.go` 中有明确注释：构建文档解析请求时，优先从上下文中的 tenant 配置获取 parser engine overrides，也就是 UI 保存的 MinerU 参数优先于环境变量。

这意味着当前实际生效的 MinerU 配置入口是：

- 租户解析引擎配置 `ParserEngineConfig`
- 知识库 `parser_engine_rules` 决定哪些文件类型命中 `mineru`

而不是简单地“给 docreader 容器塞一个 `MINERU_ENDPOINT` 环境变量”就一定完成接入。

### 2.2 OCR 是否开启，由请求参数决定

`internal/infrastructure/docparser/mineru_converter.go` 的 `NewMinerUReader` 中，默认值如下：

- `backend`: 默认 `pipeline`
- `formula_enable`: 默认 `true`
- `table_enable`: 默认 `true`
- `ocr_enable`: 默认 `true`
- `language`: 默认 `ch`

同文件 `callFileParse()` 会向外部 MinerU `/file_parse` 接口提交如下参数：

```text
return_md=true
return_images=true
table_enable=<true|false>
formula_enable=<true|false>
backend=<pipeline|vlm-*|hybrid-*>
parse_method=ocr
lang_list=<language>
```

只有当 `mineru_enable_ocr=false` 时，代码才会把：

```text
parse_method=txt
```

替代默认的 `parse_method=ocr`。

所以从 WeKnora 这一侧看：

- 命中 `mineru` 解析引擎时，OCR 默认就是开的。
- 关闭 OCR 是显式动作，不是默认动作。

## 3. 当前仓库发布物的默认状态判断

### 3.1 当前仓库没有内置 MinerU 服务镜像

当前仓库中没有找到 `mineru` 服务的 Dockerfile、compose service 或 Helm 模板。也就是说：

- WeKnora 不负责构建 MinerU 镜像。
- 你需要单独部署 MinerU 官方或社区镜像/服务。
- WeKnora 只通过 `mineru_endpoint` 调它的 HTTP API。

### 3.2 当前 `weknora-docreader` 镜像默认不是本地 OCR 镜像

`docker/Dockerfile.docreader` 里有两个非常明确的注释：

1. 构建阶段：`轻量化：仅文档解析 + 图片提取，无 OCR/VLM`
2. 运行阶段：`安装运行时依赖（已移除 OCR/PaddleOCR 相关依赖）`

这说明当前官方 DocReader 镜像的设计目标不是“容器内直接跑 PaddleOCR 或 VLM OCR”，而是：

- 做文档格式转换
- 提取图片引用
- 把后续 OCR/VLM 处理交回 Go 侧或外部服务

因此，如果用户问“当前镜像有没有启用 OCR 解析能力”，需要区分两层：

1. `weknora-docreader` 镜像：默认不是重型本地 OCR 镜像。
2. 外部 MinerU 服务：只要你自己部署的 MinerU 服务本身可用，WeKnora 调它时默认会请求 OCR 模式。

### 3.3 默认部署文件并没有把 MinerU 完整接通

当前部署文件状态如下：

1. `docker-compose.dev.yml` 的 `docreader` 环境变量里包含 `MINERU_ENDPOINT=${MINERU_ENDPOINT:-}`。
2. `docker-compose.yml` 的 `docreader` 环境变量片段里没有默认注入 `MINERU_ENDPOINT`。
3. `helm/values.yaml` 的 `docreader.env` 当前只有 `STORAGE_TYPE: local`，没有 MinerU 相关字段。

这表示仓库当前发布形态更接近：

- 开发环境给了一个“可选接入 MinerU”的入口。
- 主 compose 与 Helm chart 还没有把 MinerU 做成开箱即用配置。

再结合当前代码实现，实际上更推荐的做法也不是依赖 docreader 环境变量，而是直接在租户解析引擎配置中填写：

- `mineru_endpoint`
- `mineru_model`
- `mineru_enable_ocr`
- `mineru_enable_table`
- `mineru_enable_formula`
- `mineru_language`

## 4. 当前系统中如何正确配置 MinerU OCR

### 4.1 配置生效的前提条件

要让 MinerU OCR 在当前系统真正跑起来，至少需要同时满足下面几个条件：

1. 外部 MinerU 服务已经独立部署，且提供可访问的 `/file_parse` 接口。
2. 租户解析引擎配置里填写了可达的 `mineru_endpoint`。
3. 知识库的 `parser_engine_rules` 让目标文件类型命中 `mineru` 解析引擎。
4. `mineru_enable_ocr` 没被显式关闭。

少任一项，都会出现“系统里有 MinerU 选项，但上传文档并没有走 MinerU OCR”的现象。

### 4.2 前端与接口层的配置入口

前端设置页已经暴露了完整的 MinerU 自建参数：

- `mineru_endpoint`
- `mineru_model`
- `mineru_enable_formula`
- `mineru_enable_table`
- `mineru_enable_ocr`
- `mineru_language`

前端默认值也是：

```json
{
  "mineru_model": "pipeline",
  "mineru_enable_formula": true,
  "mineru_enable_table": true,
  "mineru_enable_ocr": true,
  "mineru_language": "ch"
}
```

后端接口侧，租户解析引擎配置会保存在 `Tenant.ParserEngineConfig` 中，并通过 tenant handler 的获取/更新接口保存与读取。

### 4.3 推荐配置示例

建议把租户侧解析引擎配置保存成类似下面这样：

```json
{
  "mineru_endpoint": "http://mineru:8000",
  "mineru_model": "pipeline",
  "mineru_enable_formula": true,
  "mineru_enable_table": true,
  "mineru_enable_ocr": true,
  "mineru_language": "ch"
}
```

如果你希望 PDF、图片、Office 文档统一优先走 MinerU，可以给知识库配置类似的规则：

```json
[
  {
    "file_types": ["pdf", "jpg", "jpeg", "png", "bmp", "tiff", "doc", "docx", "ppt", "pptx"],
    "engine": "mineru"
  }
]
```

### 4.4 什么时候不应该开 MinerU OCR

如果你的目标是：

- 第一阶段只让 MinerU 做结构化抽取或版面切分
- 后面再统一交给 DeepSeek-OCR-2 做图片 OCR

那么建议显式把：

```json
"mineru_enable_ocr": false
```

否则会出现“MinerU 先做一轮 OCR，后续 KB 多模态任务再做一轮 OCR”的重复识别问题。

## 5. DeepSeek-OCR-2 是否有 Docker 部署方案

### 5.1 官方仓库现状

官方仓库 `deepseek-ai/DeepSeek-OCR-2` 当前提供的是：

1. Conda / pip 安装说明
2. `vLLM-Inference` 脚本
3. `Transformers-Inference` 脚本

官方 README 明确给出的环境基线是：

- CUDA 11.8
- PyTorch 2.6.0
- vLLM 0.8.5

官方仓库当前没有看到：

- 官方 Dockerfile
- 官方 docker-compose.yml
- 官方 HTTP 服务封装

因此，如果你要把 DeepSeek-OCR-2 以容器方式部署到 WeKnora 周边，当前结论是：

- 官方有源码级运行方案
- 官方没有现成容器化交付方案

### 5.2 社区现成方案

社区仓库 `groxaxo/deepseek-ocr2-lazy` 已经提供了可运行的 GPU Docker 方案，特点如下：

1. 基于 `nvidia/cuda:12.1.1-devel-ubuntu22.04`
2. 暴露一个 FastAPI 服务
3. 提供 `/health` 和 `/v1/ocr` 接口
4. 支持懒加载与空闲自动卸载模型
5. README 中直接给出了 `docker build` 与 `docker run --gpus all` 用法

它更适合做：

- 单机 PoC
- 快速验证 DeepSeek-OCR-2 的 OCR 输出质量
- 作为独立 OCR 微服务接入其他系统

但需要注意两点：

1. 这是社区方案，不是官方发布。
2. 它的实现路线偏向 Unsloth/FastAPI 服务化，不等于官方 README 里的标准 `vLLM` 示例。

## 6. 推荐的 DeepSeek-OCR-2 容器化落地方式

### 6.1 方案 A：直接采用社区 Lazy Server，最快验证

如果目标是最短时间验证 OCR 能力，可以直接参考社区方案，典型启动方式如下：

```bash
docker build -t deepseek-ocr2-lazy .

docker run --gpus all -d \
  -p 8012:8012 \
  -v $(pwd)/models:/data/models \
  -v $(pwd)/output:/data/output \
  --name deepseek-ocr \
  deepseek-ocr2-lazy
```

优点：

- 上手最快
- 已经封装成 HTTP 服务
- 便于用 curl 或网关直接调用

缺点：

- 不是官方镜像
- 运行栈与官方 `vLLM` 示例不完全一致
- 生产稳定性、升级节奏、兼容性需要自行评估

### 6.2 方案 B：基于官方仓库自建镜像，适合生产化

如果你要做可控的企业内部部署，更推荐基于官方仓库自己封装一个 HTTP 服务镜像。推荐思路是：

1. 以 CUDA 11.8 或与你 GPU 驱动匹配的 NVIDIA 基础镜像为底座。
2. 安装 Python、Torch 2.6.0、vLLM 0.8.5、flash-attn 与官方 requirements。
3. 拉取 `deepseek-ai/DeepSeek-OCR-2` 仓库代码。
4. 自己加一个很薄的 FastAPI 包装层，把官方脚本封成：
   - `/health`
   - `/v1/ocr/image`
   - `/v1/ocr/pdf`
5. 把 Hugging Face 模型缓存目录与输出目录映射成卷。

可参考的 Dockerfile 骨架如下：

```dockerfile
FROM nvidia/cuda:11.8.0-devel-ubuntu22.04

ENV DEBIAN_FRONTEND=noninteractive \
    PYTHONUNBUFFERED=1 \
    HF_HOME=/data/hf \
    VLLM_USE_V1=0

RUN apt-get update && apt-get install -y \
    python3.12 python3-pip git curl build-essential \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

RUN ln -s /usr/bin/python3.12 /usr/bin/python
RUN python -m pip install --upgrade pip

RUN pip install torch==2.6.0 torchvision==0.21.0 torchaudio==2.6.0 --index-url https://download.pytorch.org/whl/cu118

# 这里需要按官方文档安装匹配的 vllm-0.8.5 wheel
# 例如：pip install vllm-0.8.5+cu118-....whl

RUN git clone https://github.com/deepseek-ai/DeepSeek-OCR-2.git /app/DeepSeek-OCR-2
WORKDIR /app/DeepSeek-OCR-2

RUN pip install -r requirements.txt
RUN pip install flash-attn==2.7.3 --no-build-isolation
RUN pip install fastapi uvicorn python-multipart

COPY server.py /app/server.py

EXPOSE 8012
CMD ["python", "/app/server.py"]
```

对应的 docker-compose 示例：

```yaml
services:
  deepseek-ocr2:
    build: ./deepseek-ocr2
    container_name: deepseek-ocr2
    ports:
      - "8012:8012"
    environment:
      HF_HOME: /data/hf
      CUDA_VISIBLE_DEVICES: "0"
      VLLM_USE_V1: "0"
    volumes:
      - ./hf-cache:/data/hf
      - ./ocr-output:/data/output
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]
```

这个方案的关键点不是 Dockerfile 本身，而是你需要额外补一个服务包装层；因为官方仓库目前给的是脚本，不是 HTTP 服务。

## 7. DeepSeek-OCR-2 部署后如何接到当前 WeKnora

### 7.1 直接部署不等于直接可用

即使你把 DeepSeek-OCR-2 容器跑起来了，在当前 WeKnora 主线代码中，它也不会自动成为“知识库解析引擎”下拉框里的一个新引擎。

原因是当前架构里：

- `parser engine` 是一套系统
- `KB 多模态 VLM` 是另一套系统

DeepSeek-OCR-2 当前更接近下面两种接法之一。

### 7.2 更符合现状的接法

#### 接法 1：继续走 KB 多模态模型路线

这是与当前代码最一致的路线：

1. 文档第一阶段仍由 builtin / docreader / mineru 负责转 markdown 与抽图。
2. 图片 OCR 与图像描述仍由知识库多模态任务负责。
3. 只需要把 DeepSeek-OCR-2 作为视觉模型服务接入现有 VLM 能力层。

这条路线的优点是：

- 与当前主线架构一致
- 不需要新增 parser engine
- 对现有知识入库流程侵入最小

#### 接法 2：把 DeepSeek-OCR-2 包成新的 OCR 微服务

如果你坚持把它当“独立 OCR 服务”来用，那么还需要额外做适配开发，例如：

1. 恢复或重做 DocReader 侧外部 OCR 调用路径。
2. 或新增一个 `deepseek_ocr2` parser engine。
3. 或在图片后处理任务中单独调用你的 `/v1/ocr` 服务。

这条路线可行，但当前仓库并没有现成适配器，需要补代码。

## 8. 最终建议

结合当前仓库现状，建议分两步推进：

### 第一步：把 MinerU 接入方式标准化

建议先完成这些动作：

1. 独立部署一个可访问的 MinerU 服务。
2. 在租户解析引擎配置里填写 `mineru_endpoint`。
3. 在知识库 `parser_engine_rules` 中显式指定哪些文件类型走 `mineru`。
4. 根据是否要避免双重 OCR，决定 `mineru_enable_ocr` 设为 `true` 还是 `false`。

### 第二步：把 DeepSeek-OCR-2 先作为独立容器服务验证

建议优先顺序是：

1. 先用社区 lazy server 快速验证 OCR 质量与显存占用。
2. 验证通过后，再基于官方仓库自建生产镜像。
3. 真正接入 WeKnora 时，优先考虑走现有 KB 多模态模型路线，而不是强行塞进 parser engine 层。

如果目标是最小改造成本，那么当前最稳妥的组合其实是：

- MinerU 负责第一阶段文档结构化解析
- DeepSeek-OCR-2 作为后续图像 OCR / 多模态识别模型

但此时应避免两边同时做 OCR，否则会出现重复计算与语义冲突。