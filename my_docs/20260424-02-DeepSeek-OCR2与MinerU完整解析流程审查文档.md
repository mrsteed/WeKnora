# DeepSeek-OCR-2 与 MinerU 并存时的“问文档”完整解析流程审查文档

## 1. 审查范围与前提

本文基于当前仓库代码，审查以下场景下“问文档”知识库导入的真实执行流程：

1. 已部署 DeepSeek-OCR-2。
2. 已在 WeKnora 中完成 `VLLM` 模型配置，并将该模型配置到知识库 `VLMConfig`。
3. 已启用 MinerU 自建服务，且租户侧解析引擎配置中已经填写 `mineru_endpoint` 等参数。
4. 用户从知识库页面导入文档、图片或扫描 PDF。

本文只描述当前代码已经实现的流程，不假设任何未落地的能力。

关键代码入口：

- [internal/application/service/knowledge.go](../internal/application/service/knowledge.go)
- [internal/application/service/image_multimodal.go](../internal/application/service/image_multimodal.go)
- [internal/application/service/knowledge_post_process.go](../internal/application/service/knowledge_post_process.go)
- [internal/infrastructure/docparser/mineru_converter.go](../internal/infrastructure/docparser/mineru_converter.go)
- [internal/types/knowledgebase.go](../internal/types/knowledgebase.go)

## 2. 审查结论摘要

先给结论：

1. 当前系统中，MinerU 和 DeepSeek-OCR-2 不在同一层工作。
2. MinerU 属于“文档解析引擎层”，负责把文件转成 Markdown 和图片引用。
3. DeepSeek-OCR-2 在你当前假设下属于“知识库 VLLM 多模态后处理层”，负责对已经提取出来的图片做 OCR 与 Caption。
4. 即使 MinerU 服务已经启用，也只有当知识库 `ParserEngineRules` 把对应文件类型指向 `mineru` 时，导入流程才会真正走 MinerU。
5. 只配置了 WeKnora 的 VLLM 模型，并不会把 DeepSeek-OCR-2 直接注入到 MinerU 解析引擎内部。
6. 在某些文档类型上，MinerU 已经完成过 OCR，而后续 DeepSeek-OCR-2 又会对抽取出的图片再做一次 OCR，因此当前架构存在“二次 OCR”可能。

## 3. 两套能力的边界

### 3.1 MinerU 负责什么

MinerU 在当前系统里属于 parser engine，接入点位于：

- [internal/application/service/knowledge.go](../internal/application/service/knowledge.go)
- [internal/infrastructure/docparser/mineru_converter.go](../internal/infrastructure/docparser/mineru_converter.go)

它负责：

1. 接收原始文件字节。
2. 调用 MinerU 服务的 `/file_parse`。
3. 返回 Markdown 内容。
4. 返回图片列表并组装成 `ImageRefs`。

它不负责：

1. 知识库级 VLM 模型选择。
2. 通过 `modelService.GetVLMModel()` 调用 WeKnora 模型中心中的 VLLM。
3. 知识库异步多模态子 chunk 的创建。

### 3.2 DeepSeek-OCR-2 在当前假设下负责什么

当你把 DeepSeek-OCR-2 配成知识库的 `VLLM` 模型后，它的主要执行点在：

- [internal/application/service/image_multimodal.go](../internal/application/service/image_multimodal.go)

它负责：

1. 对提取出来的图片执行 OCR Prompt。
2. 对同一张图片再执行 Caption Prompt。
3. 把 OCR 文本写成 `ChunkTypeImageOCR` 子 chunk。
4. 把 Caption 写成 `ChunkTypeImageCaption` 子 chunk。
5. 为这些新 chunk 建索引，供检索召回使用。

这意味着当前 DeepSeek-OCR-2 并不替代 MinerU，而是运行在 MinerU 之后。

## 4. 完整解析流程总览

当前系统的完整流程可以概括为：

1. 用户上传文件。
2. 系统读取知识库配置。
3. 根据文件类型解析 `parserEngine`。
4. 若规则命中 `mineru`，则调用 MinerU。
5. MinerU 返回 Markdown 与图片引用。
6. 系统把图片落存储并改写 Markdown 中的图片链接。
7. 系统按 chunk 配置切块并建立基础索引。
8. 如果知识库启用了多模态，系统为每张图片创建 `image:multimodal` 异步任务。
9. 每个任务使用知识库配置的 VLLM 模型，也就是你配置的 DeepSeek-OCR-2，对图片做 OCR 与 Caption。
10. 所有图片处理完成后，系统再触发 `knowledge:post_process`，把知识状态改成 `completed`，并继续派发摘要、问题生成、图谱抽取等任务。

这是当前仓库中“MinerU + DeepSeek-OCR-2 VLLM”并存时的真实闭环。

## 5. 逐步链路审查

### 5.1 第一步：知识库导入请求进入文档处理任务

入口位于：

- [internal/types/task.go](../internal/types/task.go)
- [internal/application/service/knowledge.go](../internal/application/service/knowledge.go)

文件导入后会进入 `document:process` 任务，载荷中包含：

1. `KnowledgeID`
2. `KnowledgeBaseID`
3. `FilePath`
4. `FileName`
5. `FileType`
6. `EnableMultimodel`

这里的 `EnableMultimodel` 只是“是否对文档图片继续做多模态后处理”的开关，不等同于“当前解析引擎是 VLLM”。

### 5.2 第二步：根据知识库规则选择解析引擎

关键实现：

- [internal/types/knowledgebase.go](../internal/types/knowledgebase.go)

知识库里存在 `ChunkingConfig.ParserEngineRules`。实际解析逻辑是：

1. 读取文件扩展名。
2. 遍历 `ParserEngineRules`。
3. 命中规则就返回该规则对应的 engine。
4. 如果没有命中，返回空字符串。

空字符串并不表示错误，而是表示“走默认分流”：

1. 简单格式走 Go 的 `SimpleFormatReader`。
2. 复杂格式走默认 `docreader`，通常等价于 builtin。

因此，“MinerU 服务启用”本身并不等于“上传文档一定走 MinerU”。

必须满足以下条件之一，MinerU 才会真正参与：

1. 知识库的 `parser_engine_rules` 中把 `pdf/doc/docx/ppt/pptx/jpg/png` 等类型显式映射到 `mineru`。
2. URL 类型规则显式映射到 `mineru`，且你的导入是 URL 场景。

### 5.3 第三步：构造 Reader 并调用 MinerU

关键实现：

- [internal/application/service/knowledge.go](../internal/application/service/knowledge.go)

`resolveDocReader()` 的行为很明确：

1. engine=`mineru` 时，返回 `docparser.NewMinerUReader(overrides)`。
2. engine=`mineru_cloud` 时，返回 `NewMinerUCloudReader(overrides)`。
3. engine=`builtin` 时，返回 Python docreader。
4. engine 为空时，简单格式走 `SimpleFormatReader`，否则走 docreader。

也就是说，如果文件类型命中了 `mineru` 规则，这一步会直接绕过 builtin docreader 的主解析器，进入 Go 侧的 MinerU 适配器。

### 5.4 第四步：传给 MinerU 的参数是什么

关键实现：

- [internal/types/tenant.go](../internal/types/tenant.go)
- [internal/infrastructure/docparser/mineru_converter.go](../internal/infrastructure/docparser/mineru_converter.go)

传入 MinerU 的配置来自租户级 `ParserEngineConfig.ToOverridesMap()`，当前只包含：

1. `mineru_endpoint`
2. `mineru_model`
3. `mineru_enable_formula`
4. `mineru_enable_table`
5. `mineru_enable_ocr`
6. `mineru_language`

这里有一个非常重要的边界：

WeKnora 当前不会把知识库 `VLMConfig` 中配置的 DeepSeek-OCR-2 地址、模型名、API Key 传给 MinerU。

换句话说：

1. WeKnora 的知识库 VLLM 模型配置是 KB 级。
2. MinerU 的解析引擎参数是租户级。
3. 这两套配置当前没有打通。

因此你如果“只是在 WeKnora 模型管理里把 DeepSeek-OCR-2 配成了 VLLM 模型”，MinerU 本身并不会自动改用它。

### 5.5 第五步：MinerU 在解析阶段做了什么

关键实现：

- [internal/infrastructure/docparser/mineru_converter.go](../internal/infrastructure/docparser/mineru_converter.go)

`MinerUReader.Read()` 的流程是：

1. 把文件作为 multipart 上传到 MinerU `/file_parse`。
2. 根据 `mineru_enable_ocr` 决定 `parse_method`：
   - `true` 时为 `ocr`
   - `false` 时为 `txt`
3. 根据 `mineru_model` 指定 `backend`，例如 `pipeline`、`vlm-*`、`hybrid-*`。
4. 接收 MinerU 返回的 Markdown 与图片 base64。
5. 转成 WeKnora 的 `ReadResult`。

因此，在“MinerU 已启用”的前提下，第一轮 OCR 是可能已经在 MinerU 内部完成的。

如果你的 MinerU 服务本身又配置成调用某个外部 VLM backend，那么那是 MinerU 内部自己的能力链路，不是 WeKnora 知识库 VLLM 配置链路。

### 5.6 第六步：图片落存储并改写 Markdown

关键实现：

- [internal/application/service/knowledge.go](../internal/application/service/knowledge.go)

当 `convert()` 返回 `ReadResult` 后，系统会做：

1. `imageResolver.ResolveAndStore()`：把 inline image / image refs 存入对象存储或本地存储。
2. `imageResolver.ResolveRemoteImages()`：解析 Markdown 中的外链图片并下载入库。
3. 把 Markdown 中原始图片引用替换成实际存储地址。

产出结果：

1. `convertResult.MarkdownContent` 被更新为可持久访问的图片 URL 版本。
2. `storedImages` 列表被保留下来，供后续多模态任务使用。

### 5.7 第七步：切块与首轮索引

关键实现：

- [internal/application/service/knowledge.go](../internal/application/service/knowledge.go)

系统会先用 Markdown 内容做首轮 chunk：

1. 普通切块，或
2. 父子切块

随后进入 `processChunks()`，执行：

1. 创建文本 chunk。
2. 用 embedding 模型做索引。
3. 更新知识记录状态。

这里有个关键状态判断：

如果 `EnableMultimodel=true` 且 `storedImages` 不为空，知识的 `ParseStatus` 会继续保持为 `processing`，不会在这一步直接完成。

这表示系统知道“还有图片多模态任务没做完”。

### 5.8 第八步：为每张图片创建多模态任务

关键实现：

- [internal/application/service/knowledge.go](../internal/application/service/knowledge.go)
- [internal/types/task.go](../internal/types/task.go)

`processChunks()` 完成后，如果知识库启用了多模态且存在图片，系统会为每张图片入队一个 `image:multimodal` 任务。

任务内容包括：

1. `ImageURL`
2. `ChunkID`
3. `EnableOCR=true`
4. `EnableCaption=true`
5. `ImageSourceType`

因此，在当前代码里，DeepSeek-OCR-2 不会只做 OCR；它会同时被用于 OCR 和 Caption。

### 5.9 第九步：DeepSeek-OCR-2 真正开始工作

关键实现：

- [internal/application/service/image_multimodal.go](../internal/application/service/image_multimodal.go)

这是你配置的 DeepSeek-OCR-2 真正被调用的地方。

流程如下：

1. `resolveVLM()` 读取知识库 `VLMConfig`。
2. 如果配置的是新式 `ModelID`，通过 `modelService.GetVLMModel()` 获取模型实例。
3. 如果该 `ModelID` 对应的就是 DeepSeek-OCR-2，那么这里得到的就是该模型。
4. 对同一张图片执行两次推理：
   - 用 OCR Prompt 提取正文
   - 用 Caption Prompt 生成中文描述
5. 结果分别写入新的 chunk 并再次建索引。

因此，DeepSeek-OCR-2 在当前系统中的真实位置是：

“文档已经被解析并提取出图片之后的知识增强阶段”。

### 5.10 第十步：所有图片完成后，统一收尾

关键实现：

- [internal/application/service/image_multimodal.go](../internal/application/service/image_multimodal.go)
- [internal/application/service/knowledge_post_process.go](../internal/application/service/knowledge_post_process.go)

系统会用 Redis 统计还有多少图片没处理完：

1. 每张图片开始时占一个 pending count。
2. 每完成一张图片就 `DECR`。
3. 当计数归零时，入队 `knowledge:post_process`。

`knowledge:post_process` 会做：

1. 拉取当前知识的所有 chunk。
2. 把 `text + image OCR + image caption` 一起视为 text-like chunks。
3. 若知识状态仍为 `processing`，则改成 `completed`。
4. 继续派发摘要任务。
5. 若启用了问题生成，则派发问题生成任务。
6. 若启用了图谱或 wiki 索引，则继续派发对应任务。

到这里，整条链路才真正闭环。

## 6. 在当前假设下的真实“完整解析流程图”

你当前假设下，一次 PDF / DOCX / 图片导入的真实顺序可以表达为：

1. 上传文件。
2. 读取知识库配置：
   - `ParserEngineRules`
   - `VLMConfig`
   - `StorageProvider`
   - `EmbeddingModel`
3. 判断该文件类型是否命中 `mineru`。
4. 若命中：走 MinerU 第一阶段解析。
5. MinerU 返回 Markdown 与图片。
6. 系统存储图片并替换 Markdown 图片链接。
7. 系统切块并建立首轮文本索引。
8. 因知识库启用了多模态，系统继续为图片派发异步任务。
9. 异步任务使用知识库的 DeepSeek-OCR-2 做第二阶段 OCR 与 Caption。
10. 新生成的 OCR / Caption chunk 再次建立索引。
11. 所有图片完成后触发统一后处理。
12. 知识状态改为 `completed`。
13. 后续再触发摘要、问题生成、图谱抽取等任务。

## 7. 按文件类型拆解的实际分支

### 7.1 PDF / 扫描 PDF

当 `ParserEngineRules` 把 `pdf` 指向 `mineru` 时：

1. 第一阶段由 MinerU 负责 PDF 解析。
2. 如果 MinerU 已经执行 OCR，那么文档正文可能已经在 Markdown 中出现。
3. 如果返回了图片页或图内图片，后续多模态任务仍会再对这些图片跑 DeepSeek-OCR-2。

这类文件最容易出现“MinerU OCR + DeepSeek-OCR-2 OCR 双阶段并存”。

### 7.2 DOC / DOCX / PPT / PPTX

当规则命中 `mineru` 时，流程与 PDF 类似：

1. MinerU 先完成文档结构化。
2. 文档内图片被抽取并落存储。
3. DeepSeek-OCR-2 再对这些图片做 OCR 与 Caption。

### 7.3 图片文件

当图片类型命中 `mineru` 时：

1. MinerU 先尝试把图片内容转 Markdown。
2. 系统仍会保留原图引用。
3. 后续多模态任务再用 DeepSeek-OCR-2 做 OCR 与 Caption。

如果图片类型没有命中 `mineru`，则可能走 `simple`，此时第一阶段只保留图片引用，真正的 OCR 基本全部落到 DeepSeek-OCR-2 异步任务里。

### 7.4 简单文本格式

对于 `md/txt/csv/json` 等简单格式：

1. 一般不会走 MinerU。
2. 直接走 `SimpleFormatReader` 或 builtin。
3. 若 Markdown 内含图片链接并被成功解析入库，仍可触发 DeepSeek-OCR-2 多模态任务。

## 8. 当前架构下最重要的三个审查发现

### 8.1 发现一：MinerU 是否生效，取决于知识库规则，不取决于服务是否在线

这是当前配置最容易误判的地方。

MinerU 服务在线，只说明“可被调用”；不说明“本次导入一定会调用”。

真正决定是否走 MinerU 的是：

1. 当前知识库的 `parser_engine_rules`
2. 当前文件类型

### 8.2 发现二：DeepSeek-OCR-2 当前不会直接驱动 MinerU

你在 WeKnora 模型中心里配置的 DeepSeek-OCR-2，只会进入知识库 `VLMConfig` 链路。

当前代码没有把这份 VLM 配置传给 MinerU 解析器。

所以当前系统更准确的描述不是：

“MinerU 使用了 DeepSeek-OCR-2 解析文档”

而是：

“MinerU 负责首轮解析，DeepSeek-OCR-2 负责解析后的图片增强”。

### 8.3 发现三：当前链路可能出现重复 OCR

这是最值得关注的架构副作用。

在以下条件同时成立时：

1. 文档类型命中 `mineru`
2. `mineru_enable_ocr=true`
3. MinerU 返回图片
4. 知识库 `EnableMultimodel=true`
5. DeepSeek-OCR-2 已配置为知识库 VLM 模型

系统就可能出现：

1. MinerU 在文档解析阶段先做了一轮 OCR。
2. DeepSeek-OCR-2 在图片多模态阶段又做一轮 OCR。

这并不一定错误，但会带来三个后果：

1. 时延增加。
2. Token / 推理成本增加。
3. 结果来源变成“双通道”，需要评估是否会引入冗余内容。

## 9. 对当前部署方式的判断

基于当前代码，如果你的部署方式是：

1. WeKnora 中配置了 DeepSeek-OCR-2 为 VLLM 模型。
2. 知识库启用了多模态。
3. 租户中配置了 MinerU 解析引擎。
4. 知识库解析规则把目标文件类型指向了 `mineru`。

那么当前系统的真实语义应理解为：

“MinerU 负责文档第一阶段结构化解析，DeepSeek-OCR-2 负责文档图片第二阶段语义 OCR 与说明增强。”

这是一种串联架构，而不是单层替代架构。

## 10. 建议

### 10.1 如果你当前目标是尽快上线可用链路

建议保持现状，但明确运营认知：

1. MinerU 是首轮解析器。
2. DeepSeek-OCR-2 是后处理增强器。
3. 对扫描 PDF 和图文文档重点做效果验证。

### 10.2 如果你担心重复 OCR 成本

建议优先评估以下两个优化方向：

1. 对走 MinerU 的文件类型关闭后续图片 OCR，只保留 Caption。
2. 或者对 MinerU 已明确完成 OCR 的结果，跳过 DeepSeek-OCR-2 OCR 子任务。

### 10.3 如果你希望 DeepSeek-OCR-2 成为真正的“主 OCR 引擎”

则需要进一步改造，把当前 KB 级 VLM 配置真正下沉到 parser engine 层，或者把 MinerU 和 VLLM 之间的责任边界重新拆分。

在当前代码里，这件事还没有完成。

## 11. 最终结论

在“已部署 DeepSeek-OCR-2 + 已配置 VLLM 模型 + 已启用 MinerU 服务”的前提下，当前系统的完整解析流程是：

1. 先根据知识库规则决定是否使用 MinerU。
2. 若命中 MinerU，则由 MinerU 执行第一阶段文档解析。
3. 解析结果中的图片被保存并进入异步多模态任务。
4. 异步任务再调用知识库配置的 DeepSeek-OCR-2 做 OCR 与 Caption。
5. 最终由统一后处理任务将知识状态改为完成，并继续摘要、问题生成、图谱等后续任务。

因此，当前系统中 DeepSeek-OCR-2 与 MinerU 的关系不是二选一，而是“前后串联、分层协作”。
