# TEI Reranker (tei_reranker) 集成技术文档

> 版本：v1.0  
> 日期：2025-07  
> 范围：新增 HuggingFace Text Embeddings Inference (TEI) 作为 Rerank 模型提供商，支持在前端添加和管理 TEI Reranker 模型。

---

## 目录

1. [需求概述](#1-需求概述)
2. [TEI 服务简介](#2-tei-服务简介)
3. [现有架构分析](#3-现有架构分析)
4. [问题诊断与修复清单](#4-问题诊断与修复清单)
5. [后端实现详解](#5-后端实现详解)
6. [前端实现详解](#6-前端实现详解)
7. [端到端数据流](#7-端到端数据流)
8. [API 规范](#8-api-规范)
9. [部署与配置](#9-部署与配置)
10. [测试方案](#10-测试方案)
11. [文件变更清单](#11-文件变更清单)

---

## 1. 需求概述

### 1.1 目标

在 WeKnora 的模型管理系统中，新增 `huggingface`（HuggingFace / TEI）作为一级模型提供商（Provider），使用户可以通过前端界面：

- 在 **Rerank 模型**管理页面中选择 "HuggingFace / TEI" 提供商
- 配置 TEI 服务的 Base URL（默认 `http://localhost:8082`）
- 测试 TEI Rerank 连接是否正常
- 将 TEI Reranker 用于知识库检索的重排序

### 1.2 TEI Rerank 请求格式

TEI 的 Rerank API 使用非标准的请求/响应格式（与 OpenAI 兼容接口不同），因此需要专用的适配器。

**请求示例：**

```bash
curl 192.168.1.212:8082/rerank \
    -X POST \
    -d '{"query": "What is Deep Learning?", "texts": ["Deep Learning is not...", "Deep learning is..."]}' \
    -H 'Content-Type: application/json'
```

**响应示例：**

```json
[
  { "index": 1, "score": 0.9987 },
  { "index": 0, "score": 0.0023 }
]
```

### 1.3 TEI 与 OpenAI Rerank API 的关键差异

| 特性 | TEI Rerank | OpenAI Rerank |
|------|-----------|--------------|
| 端点路径 | `/rerank` | `/v1/rerank` |
| 文档字段名 | `texts` | `documents` |
| 评分字段名 | `score` | `relevance_score` |
| 响应格式 | 裸 JSON 数组 `[{...}]` | 包装对象 `{"results":[{...}]}` |
| 可选参数 | `return_text`, `truncate` | `top_n`, `return_documents` |
| 认证方式 | 通常无需认证（本地部署） | Bearer Token |

---

## 2. TEI 服务简介

[Text Embeddings Inference (TEI)](https://github.com/huggingface/text-embeddings-inference) 是 HuggingFace 开发的高性能文本嵌入和重排序推理服务，专为生产环境设计。

### 2.1 典型部署

```bash
# 启动 TEI Reranker 服务（以 bge-reranker-v2-m3 为例）
docker run --gpus all -p 8082:80 \
  ghcr.io/huggingface/text-embeddings-inference:latest \
  --model-id BAAI/bge-reranker-v2-m3
```

### 2.2 支持的模型

| 模型 | 用途 | 说明 |
|------|------|------|
| `BAAI/bge-reranker-v2-m3` | Rerank | 多语言重排序模型 |
| `BAAI/bge-reranker-large` | Rerank | 大规模重排序模型 |
| `BAAI/bge-m3` | Embedding | 多语言嵌入模型 |
| `BAAI/bge-large-zh-v1.5` | Embedding | 中文嵌入模型 |

### 2.3 API 端点

| 端点 | 方法 | 用途 |
|------|------|------|
| `/rerank` | POST | 文档重排序 |
| `/embed` | POST | 文本嵌入 |
| `/health` | GET | 健康检查 |

---

## 3. 现有架构分析

### 3.1 后端模型提供商注册机制

```
internal/models/provider/
├── provider.go          ← Provider 接口定义、注册表、常量
├── huggingface.go       ← HuggingFace Provider 实现（已存在）
├── openai.go
├── aliyun.go
├── zhipu.go
├── jina.go
├── generic.go
└── ...
```

每个 Provider 通过 `init()` 函数自动注册到全局注册表：

```go
func init() {
    Register(&HuggingFaceProvider{})
}
```

`provider.go` 中的 `AllProviders()` 函数控制 Provider 在 API 返回中的顺序。

### 3.2 后端 Reranker 工厂

```
internal/models/rerank/
├── reranker.go          ← Reranker 接口 + NewReranker 工厂
├── tei_reranker.go      ← TEI 专用实现（已存在）
├── remote_api.go        ← OpenAI 兼容实现
├── aliyun_reranker.go
├── zhipu_reranker.go
└── jina_reranker.go
```

`NewReranker()` 工厂函数通过 `switch providerName` 来决定创建哪种 Reranker 实例。

### 3.3 前端模型编辑器

`frontend/src/components/ModelEditorDialog.vue` 包含：
- 硬编码的 `fallbackProviderOptions` 列表（当 API 不可用时使用）
- 从后端 API `GET /api/v1/models/providers?model_type=rerank` 动态获取 Provider 列表
- 根据 Provider 自动填充默认 Base URL
- 模型测试按钮调用 `POST /api/v1/initialization/rerank/check`

### 3.4 数据流概览

```
前端 ModelEditorDialog → POST /api/v1/models (创建模型)
                          └─ model.parameters.provider = "huggingface"
                          └─ model.parameters.base_url = "http://localhost:8082"

知识库检索 → modelService.GetRerankModel(modelId)
              └─ NewReranker(config) with Provider="huggingface"
                  └─ switch → NewTEIReranker(config)
                      └─ POST {baseURL}/rerank 调用 TEI API
```

---

## 4. 问题诊断与修复清单

在实现前发现以下问题，均已在本次修改中解决：

| # | 问题 | 影响 | 修复文件 |
|---|------|------|---------|
| 1 | `ProviderHuggingFace` 常量未在 `provider.go` 中定义 | `huggingface.go` 引用了不存在的常量，编译异常 | `provider.go` |
| 2 | `AllProviders()` 列表中缺少 HuggingFace | 后端 `/models/providers` API 不会返回 HuggingFace 选项 | `provider.go` |
| 3 | `NewReranker()` 工厂 switch 中缺少 `ProviderHuggingFace` case | 即使配置了 provider=huggingface，仍会降级到 OpenAI 兼容适配器 | `reranker.go` |
| 4 | `GetRerankModel()` 服务未传递 `Provider` 字段到 `RerankerConfig` | 工厂函数无法识别 HuggingFace，会通过 URL 检测降级 | `model.go` |
| 5 | `checkRerankModelConnection()` 未接收和传递 `Provider` 字段 | 模型测试按钮无法正确路由到 TEI 适配器 | `initialization.go` |
| 6 | 前端 `checkRerankModel` API 未传递 `provider` 参数 | 后端无法获取 provider 信息用于路由 | `initialization/index.ts` |
| 7 | 前端 `fallbackProviderOptions` 缺少 HuggingFace 条目 | 当 API 不可用时，前端看不到 HuggingFace 选项 | `ModelEditorDialog.vue` |
| 8 | 四种语言文件缺少 HuggingFace Provider 翻译 | 前端显示空白标签 | `zh-CN.ts` 等 |

---

## 5. 后端实现详解

### 5.1 Provider 常量注册

**文件**：`internal/models/provider/provider.go`

新增常量和列表条目：

```go
const (
    // ... 已有常量 ...
    // HuggingFace TEI (Text Embeddings Inference)
    ProviderHuggingFace ProviderName = "huggingface"
)

func AllProviders() []ProviderName {
    return []ProviderName{
        // ... 已有条目 ...
        ProviderHuggingFace,  // ← 新增
    }
}
```

### 5.2 HuggingFace Provider 实现

**文件**：`internal/models/provider/huggingface.go`（已存在，无需修改）

```go
type HuggingFaceProvider struct{}

func (p *HuggingFaceProvider) Info() ProviderInfo {
    return ProviderInfo{
        Name:        ProviderHuggingFace,
        DisplayName: "HuggingFace / TEI",
        Description: "Text Embeddings Inference (bge-m3, etc.)",
        DefaultURLs: map[types.ModelType]string{
            types.ModelTypeEmbedding: "http://localhost:8080",
            types.ModelTypeRerank:    "http://localhost:8080",
        },
        ModelTypes: []types.ModelType{
            types.ModelTypeEmbedding,
            types.ModelTypeRerank,
        },
        RequiresAuth: false,
    }
}
```

### 5.3 TEI Reranker 适配器

**文件**：`internal/models/rerank/tei_reranker.go`（已存在，无需修改）

实现了 `Reranker` 接口，核心逻辑：

```go
func (r *TEIReranker) Rerank(ctx context.Context, query string, documents []string) ([]RankResult, error) {
    // 构建 TEI 格式请求：使用 "texts" 字段（非 "documents"）
    reqBody := TEIRerankRequest{
        Query:      query,
        Texts:      documents,       // TEI 使用 texts
        ReturnText: false,
        Truncate:   true,
    }
    // POST {baseURL}/rerank
    url := fmt.Sprintf("%s/rerank", r.baseURL)
    // ...
    // TEI 返回裸 JSON 数组，score 字段映射为 RelevanceScore
    var teiResults []TEIRankResult
    json.Unmarshal(bodyBytes, &teiResults)
    // 转换到标准 RankResult
}
```

### 5.4 Reranker 工厂路由

**文件**：`internal/models/rerank/reranker.go`

在 `NewReranker()` 的 switch 中新增 case：

```go
func NewReranker(config *RerankerConfig) (Reranker, error) {
    providerName := provider.ProviderName(config.Provider)
    if providerName == "" {
        providerName = provider.DetectProvider(config.BaseURL)
    }

    switch providerName {
    case provider.ProviderAliyun:
        return NewAliyunReranker(config)
    case provider.ProviderZhipu:
        return NewZhipuReranker(config)
    case provider.ProviderJina:
        return NewJinaReranker(config)
    case provider.ProviderHuggingFace:     // ← 新增
        return NewTEIReranker(config)       // ← 新增
    default:
        return NewOpenAIReranker(config)
    }
}
```

### 5.5 服务层 Provider 传递

**文件**：`internal/application/service/model.go`

`GetRerankModel()` 现在传递 `Provider` 字段：

```go
reranker, err := rerank.NewReranker(&rerank.RerankerConfig{
    ModelID:   model.ID,
    APIKey:    model.Parameters.APIKey,
    BaseURL:   model.Parameters.BaseURL,
    ModelName: model.Name,
    Source:    model.Source,
    Provider:  model.Parameters.Provider,  // ← 新增
})
```

### 5.6 模型测试接口

**文件**：`internal/handler/initialization.go`

`CheckRerankModel` handler 和 `checkRerankModelConnection` 方法现在支持 `provider` 参数：

```go
var req struct {
    ModelName string `json:"modelName" binding:"required"`
    BaseURL   string `json:"baseUrl" binding:"required"`
    APIKey    string `json:"apiKey"`
    Provider  string `json:"provider"`           // ← 新增
}

// ...

func (h *InitializationHandler) checkRerankModelConnection(ctx context.Context,
    modelName, baseURL, apiKey, providerName string,  // ← 新增 providerName
) (bool, string) {
    config := &rerank.RerankerConfig{
        // ...
        Provider:  providerName,                      // ← 新增
    }
}
```

---

## 6. 前端实现详解

### 6.1 API 类型更新

**文件**：`frontend/src/api/initialization/index.ts`

`checkRerankModel` 函数新增 `provider` 可选参数：

```typescript
export function checkRerankModel(modelConfig: {
    modelName: string;
    baseUrl: string;
    apiKey?: string;
    provider?: string;   // ← 新增
}): Promise<{ available: boolean; message?: string }>
```

### 6.2 ModelEditorDialog 后备配置

**文件**：`frontend/src/components/ModelEditorDialog.vue`

在 `fallbackProviderOptions` 中新增 HuggingFace/TEI 条目：

```typescript
{
  value: 'huggingface',
  label: t('model.editor.providers.huggingface.label'),
  defaultUrls: {
    embedding: 'http://localhost:8080',
    rerank: 'http://localhost:8082'
  },
  description: t('model.editor.providers.huggingface.description'),
  modelTypes: ['embedding', 'rerank']
}
```

Rerank 模型测试时传递 provider：

```typescript
case 'rerank':
  result = await checkRerankModel({
    modelName: formData.value.modelName,
    baseUrl: formData.value.baseUrl,
    apiKey: formData.value.apiKey || '',
    provider: formData.value.provider    // ← 新增
  })
```

### 6.3 国际化翻译

四个语言文件均新增：

```typescript
huggingface: {
  label: "HuggingFace / TEI",
  description: "Text Embeddings Inference (bge-reranker-v2-m3, bge-m3, etc.)",
},
```

---

## 7. 端到端数据流

### 7.1 添加 TEI Reranker 模型

```
┌─────────────────────────────────────────────────────────────┐
│ 用户在前端"模型设置 → Rerank"页面点击"添加模型"                    │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ ModelEditorDialog 弹出                                        │
│ ① 选择服务商: "HuggingFace / TEI"                              │
│ ② Base URL 自动填充: http://localhost:8082                     │
│ ③ 填写模型名称: bge-reranker-v2-m3                              │
│ ④ API Key: (可选，本地部署通常不需要)                              │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 点击"测试连接"按钮                                              │
│ POST /api/v1/initialization/rerank/check                      │
│ Body: { modelName, baseUrl, apiKey, provider: "huggingface" } │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 后端 CheckRerankModel Handler                                  │
│ → checkRerankModelConnection(ctx, name, url, key, provider)   │
│ → NewReranker(config{Provider: "huggingface"})                │
│ → switch → NewTEIReranker(config)                             │
│ → TEIReranker.Rerank("ping", ["pong"])                        │
│ → POST http://localhost:8082/rerank                           │
│   Body: {"query":"ping","texts":["pong"],"truncate":true}     │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ TEI 服务返回: [{"index":0,"score":0.85}]                       │
│ → 返回 {available: true, message: "重排功能正常，返回1个结果"}     │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 前端显示 ✅ 连接正常                                            │
│ 用户点击"保存"                                                  │
│ POST /api/v1/models                                           │
│ Body: {                                                       │
│   name: "bge-reranker-v2-m3",                                 │
│   type: "Rerank",                                             │
│   source: "remote",                                           │
│   parameters: {                                               │
│     base_url: "http://localhost:8082",                         │
│     provider: "huggingface"                                   │
│   }                                                           │
│ }                                                             │
└─────────────────────────────────────────────────────────────┘
```

### 7.2 使用 TEI Reranker 进行知识库检索

```
用户提问 → Chat Pipeline → RetrieveEngine
    → 向量/全文检索返回候选文档
    → modelService.GetRerankModel(modelId)
        → model.Parameters.Provider = "huggingface"
        → NewReranker({Provider: "huggingface", BaseURL: "http://localhost:8082"})
        → NewTEIReranker(config)
    → TEIReranker.Rerank(query, documents)
        → POST http://localhost:8082/rerank
          Body: {"query": "...", "texts": ["doc1", "doc2", ...], "truncate": true}
        → 解析 TEI 响应 → 按 score 排序 → 返回 top-N 结果
    → 将重排序结果作为上下文注入 LLM 提示词
```

---

## 8. API 规范

### 8.1 TEI Rerank API

**端点**：`POST {base_url}/rerank`

**请求体**：

```json
{
  "query": "What is Deep Learning?",
  "texts": [
    "Deep Learning is not...",
    "Deep learning is..."
  ],
  "return_text": false,
  "truncate": true
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `query` | string | 是 | 查询文本 |
| `texts` | string[] | 是 | 待排序文档列表 |
| `return_text` | bool | 否 | 是否在响应中返回原文（默认 false） |
| `truncate` | bool | 否 | 超出最大长度时自动截断（默认 false） |

**响应体**：

```json
[
  { "index": 1, "score": 0.9987 },
  { "index": 0, "score": 0.0023 }
]
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `index` | int | 对应 `texts` 数组中的原始索引 |
| `score` | float | 相关性评分（0-1） |
| `text` | string | 原文（仅当 `return_text=true` 时返回） |

### 8.2 WeKnora 模型测试接口

**端点**：`POST /api/v1/initialization/rerank/check`

**请求体**：

```json
{
  "modelName": "bge-reranker-v2-m3",
  "baseUrl": "http://192.168.1.212:8082",
  "apiKey": "",
  "provider": "huggingface"
}
```

**响应体**：

```json
{
  "success": true,
  "data": {
    "available": true,
    "message": "重排功能正常，返回1个结果"
  }
}
```

### 8.3 Provider 列表接口

**端点**：`GET /api/v1/models/providers?model_type=rerank`

HuggingFace 条目在响应中的格式：

```json
{
  "value": "huggingface",
  "label": "HuggingFace / TEI",
  "description": "Text Embeddings Inference (bge-m3, etc.)",
  "defaultUrls": {
    "embedding": "http://localhost:8080",
    "rerank": "http://localhost:8080"
  },
  "modelTypes": ["embedding", "rerank"]
}
```

---

## 9. 部署与配置

### 9.1 TEI 服务部署

**Docker 方式（推荐）**：

```bash
# GPU 加速部署 Reranker
docker run -d --name tei-reranker \
  --gpus all \
  -p 8082:80 \
  ghcr.io/huggingface/text-embeddings-inference:latest \
  --model-id BAAI/bge-reranker-v2-m3

# CPU 部署（性能较低，适合测试）
docker run -d --name tei-reranker \
  -p 8082:80 \
  ghcr.io/huggingface/text-embeddings-inference:cpu-latest \
  --model-id BAAI/bge-reranker-v2-m3
```

**同时部署 Embedding + Reranker**：

```bash
# Embedding 服务 (端口 8080)
docker run -d --name tei-embedding \
  --gpus '"device=0"' \
  -p 8080:80 \
  ghcr.io/huggingface/text-embeddings-inference:latest \
  --model-id BAAI/bge-m3

# Reranker 服务 (端口 8082)
docker run -d --name tei-reranker \
  --gpus '"device=1"' \
  -p 8082:80 \
  ghcr.io/huggingface/text-embeddings-inference:latest \
  --model-id BAAI/bge-reranker-v2-m3
```

### 9.2 验证部署

```bash
# 健康检查
curl http://localhost:8082/health

# Rerank 测试
curl http://localhost:8082/rerank \
  -X POST \
  -H 'Content-Type: application/json' \
  -d '{"query": "What is Deep Learning?", "texts": ["Deep Learning is not...", "Deep learning is..."]}'
```

### 9.3 在 WeKnora 中配置

1. 进入 **设置 → 模型管理 → Rerank 模型**
2. 点击 **添加模型**
3. 选择服务商：**HuggingFace / TEI**
4. Base URL 自动填充 `http://localhost:8082`（根据实际部署修改）
5. 模型名称填写实际部署的模型名（如 `bge-reranker-v2-m3`）
6. API Key 留空（本地部署无需认证）
7. 点击 **测试连接** 验证
8. 保存

---

## 10. 测试方案

### 10.1 后端单元测试

| 测试场景 | 验证点 |
|---------|--------|
| `NewReranker` with `Provider="huggingface"` | 返回 `*TEIReranker` 实例 |
| `TEIReranker.Rerank()` 正常响应 | 正确解析 `[{index, score}]` 数组 |
| `TEIReranker.Rerank()` 异常响应 | HTTP 错误码、JSON 解析失败等情况 |
| `ProviderHuggingFace` 在 `AllProviders()` 中 | 列表包含 `"huggingface"` |
| `ListByModelType(Rerank)` | 结果包含 HuggingFace |

### 10.2 前端集成测试

| 测试场景 | 操作 | 预期结果 |
|---------|------|---------|
| Provider 下拉列表 | 打开添加 Rerank 模型弹窗 | 列表中包含 "HuggingFace / TEI" |
| 默认 URL 填充 | 选择 HuggingFace | Base URL 自动填充 `http://localhost:8082` |
| 连接测试（正常） | 配置正确的 TEI 服务地址并测试 | 显示 ✅ 连接正常 |
| 连接测试（失败） | 配置错误地址并测试 | 显示 ❌ 连接失败 |
| 保存模型 | 填写完整信息并保存 | 模型出现在 Rerank 列表中 |
| 知识库检索 | 使用 TEI Reranker 的知识库进行问答 | 正常返回重排序后的结果 |

### 10.3 端到端测试

```bash
# 1. 验证 Provider 列表接口
curl -s http://localhost:8080/api/v1/models/providers?model_type=rerank | \
  python3 -m json.tool | grep -A5 huggingface

# 2. 验证 Rerank 测试接口
curl -X POST http://localhost:8080/api/v1/initialization/rerank/check \
  -H 'Content-Type: application/json' \
  -d '{
    "modelName": "bge-reranker-v2-m3",
    "baseUrl": "http://192.168.1.212:8082",
    "provider": "huggingface"
  }'

# 3. 创建 Rerank 模型
curl -X POST http://localhost:8080/api/v1/models \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "bge-reranker-v2-m3",
    "type": "Rerank",
    "source": "remote",
    "parameters": {
      "base_url": "http://192.168.1.212:8082",
      "provider": "huggingface"
    }
  }'
```

---

## 11. 文件变更清单

### 后端文件

| 文件 | 操作 | 变更说明 |
|------|------|---------|
| `internal/models/provider/provider.go` | **修改** | 新增 `ProviderHuggingFace` 常量；加入 `AllProviders()` 列表 |
| `internal/models/provider/huggingface.go` | 无修改 | 已实现，通过 `init()` 自动注册 |
| `internal/models/rerank/reranker.go` | **修改** | `NewReranker()` switch 新增 `case ProviderHuggingFace → NewTEIReranker` |
| `internal/models/rerank/tei_reranker.go` | 无修改 | TEI 适配器已完整实现 |
| `internal/application/service/model.go` | **修改** | `GetRerankModel()` 传递 `Provider` 字段到 `RerankerConfig` |
| `internal/handler/initialization.go` | **修改** | `CheckRerankModel` 请求体新增 `provider` 字段；`checkRerankModelConnection` 接收并传递 `provider` |

### 前端文件

| 文件 | 操作 | 变更说明 |
|------|------|---------|
| `frontend/src/api/initialization/index.ts` | **修改** | `checkRerankModel` 函数新增 `provider` 可选参数 |
| `frontend/src/components/ModelEditorDialog.vue` | **修改** | `fallbackProviderOptions` 新增 HuggingFace 条目；Rerank 测试传递 `provider` |
| `frontend/src/i18n/locales/zh-CN.ts` | **修改** | 新增 `providers.huggingface` 翻译 |
| `frontend/src/i18n/locales/en-US.ts` | **修改** | 新增 `providers.huggingface` 翻译 |
| `frontend/src/i18n/locales/ru-RU.ts` | **修改** | 新增 `providers.huggingface` 翻译 |
| `frontend/src/i18n/locales/ko-KR.ts` | **修改** | 新增 `providers.huggingface` 翻译 |

---

## 附录：TEI Reranker 与其他 Reranker 对比

| 维度 | TEI Reranker | OpenAI Rerank | Jina Rerank | 阿里云 Rerank |
|------|-------------|--------------|-------------|-------------|
| 部署方式 | 自托管 (Docker/GPU) | 云 API | 云 API | 云 API |
| API 格式 | 自定义 (`texts`, `score`) | OpenAI 标准 | Jina 标准 | 阿里云标准 |
| 认证 | 可选 | Bearer Token | Bearer Token | Bearer Token |
| 成本 | GPU 硬件成本 | 按调用计费 | 按调用计费 | 按调用计费 |
| 延迟 | 低（局域网） | 中（公网） | 中（公网） | 中（公网） |
| 隐私 | 数据不出内网 | 数据上云 | 数据上云 | 数据上云 |
| 适用场景 | 对数据隐私要求高的企业 | 快速接入 | Embedding+Rerank 一体 | 阿里云生态 |
