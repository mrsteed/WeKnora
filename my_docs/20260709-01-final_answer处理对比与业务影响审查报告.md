# final_answer 处理对比与业务影响审查报告

## 1. 审查范围

本次审查聚焦以下问题：

- 对比 main 分支与当前 dev 分支对 final_answer 的处理差异。
- 评估“去掉 final_answer”对普通 Agent 问答、长文档相关路径、历史消息回放、前端流式展示的业务影响。
- 判断是否需要从 main 分支还原相关代码。

本次审查覆盖的核心代码路径包括：

- `internal/agent/observe.go`
- `internal/agent/think.go`
- `internal/models/chat/openai_stream.go`
- `internal/application/service/agent_service.go`
- `internal/application/service/session_agent_qa.go`
- `internal/application/service/agent_history.go`
- `internal/types/agent.go`
- `internal/handler/session/agent_stream_handler.go`
- `internal/handler/message.go`

同时核对了 main 分支历史提交 `d38a6efd87df3c95290a73c5fe1283ecfbc9b85d`：

- 提交标题：`refactor(agent): remove final_answer tool references and update related logic`
- 该提交明确将普通 Agent 的终止模式从“调用 final_answer 工具”切换为“模型直接输出 plain text 并自然 stop”。

## 2. 结论摘要

结论先行：

1. main 分支并没有保留 final_answer 作为普通 Agent QA 的默认终止工具。
2. main 分支保留的是 final_answer 事件语义、历史兼容语义，以及最终答案落库/流式展示的数据契约，不是普通 Agent 的 final_answer 工具注册。
3. 当前 dev 分支相对 main 的新增点，是为文档止损场景局部恢复了一套“stopgap 专用 final_answer 工具链路”，而不是把普通 Agent 全面改回 final_answer 工具模式。
4. 不建议把普通 Agent QA 全面还原成依赖 final_answer 工具的 old path。
5. 建议保留当前分支里“仅 stopgap 场景可见”的 final_answer 工具链路，以及与之配套的流式解析、终止判定和历史兼容处理。

## 3. main 与当前分支的实际差异

### 3.1 普通 Agent QA 终止方式

main 分支：

- 普通 Agent QA 通过 `finish_reason=stop && tool_calls=0` 走自然结束分支。
- `internal/agent/observe.go` 不再把 final_answer 当作普通主链终止工具。

当前 dev 分支：

- 普通 Agent QA 仍然以自然 stop 为主终止方式。
- 额外增加了一层兼容逻辑：如果响应里真的出现 final_answer 工具调用，仍可在 `internal/agent/observe.go` 中收口并终止，避免重复回答或空白回答。

结论：

- 在“普通 Agent QA 是否默认依赖 final_answer 工具”这件事上，当前分支并没有比 main 更激进地去掉能力；main 本身就已经去掉了这条默认路径。

### 3.2 final_answer 工具注册策略

main 分支：

- 不存在 `AllowFinalAnswerTool` 运行时开关。
- `internal/application/service/agent_service.go` 不向普通 Agent 注册 final_answer 工具。

当前 dev 分支：

- `internal/types/agent.go` 新增 `AllowFinalAnswerTool`。
- `internal/application/service/agent_service.go` 默认仍不允许注册 final_answer。
- 只有当 `AllowFinalAnswerTool=true` 时，才允许把 `ToolFinalAnswer` 暴露给模型。

结论：

- 当前分支不是“继续删除 final_answer”，而是“把 final_answer 缩到一个显式受控的专用场景里”。

### 3.3 文档 stopgap 路径

main 分支：

- 不存在 `applyDocumentStopgapAgentConfig()` 这套 final_answer-only 的运行时缩减逻辑。

当前 dev 分支：

- `internal/application/service/session_agent_qa.go` 中的 `applyDocumentStopgapAgentConfig()` 会把文档止损请求改造成：
  - `MaxIterations=1`
  - 关闭 WebSearch
  - 关闭 MultiTurn
  - 关闭 MCP
  - 只允许 `ToolFinalAnswer`
  - 开启 `AllowFinalAnswerTool`

结论：

- 这是当前分支为长文档/文档修订止损场景补的一条专用链路，不是 main 的遗留代码。

### 3.4 历史消息回放兼容

main 分支与当前分支一致保留：

- `internal/application/service/agent_history.go` 中的 `legacyFinalAnswerToolName = "final_answer"`
- 回放历史时会过滤掉旧会话里落下的 final_answer 工具调用，只保留 trailing assistant canonical answer。

结论：

- 这部分不是是否恢复工具的争议点，而是历史兼容必需项。

### 3.5 流式答案与前端契约

main 分支与当前分支都保留：

- `EventAgentFinalAnswer` 事件类型
- `final_answer` 字段作为 SSE/消息完成态中的最终答案载体
- `internal/handler/session/agent_stream_handler.go` 与 `internal/handler/message.go` 对最终答案内容的读取与持久化逻辑

当前分支额外新增：

- `internal/models/chat/openai_stream.go` 会把 final_answer 工具参数中的 `answer` 字段增量抽取成 answer-type chunk，来源标记为 `final_answer_tool`。
- `internal/agent/observe.go` 能在 final_answer 工具出现时做终态收口。

结论：

- “final_answer 工具”与“final_answer 事件/字段”不是一回事。
- 可以不恢复普通 Agent 的 final_answer 工具，但绝不能把 final_answer 事件/字段这个业务契约一起删掉。

## 4. 业务影响评估

### P0：如果删掉 stopgap 专用 final_answer 链路，文档修订止损路径会退化

影响范围：

- 文档修订、增量输出、base artifact 存在时触发的 stopgap Agent 路径。

原因：

- 当前 stopgap 逻辑明确把工具列表收敛到 `ToolFinalAnswer` 一项。
- 如果再把这条工具链删掉，但仍保留该 stopgap 配置，运行时会出现“策略要求单轮收口，但没有稳定终止工具”的矛盾。
- 结果不是编译错误，而是业务语义退化：可能转成 plain text 自然结束、也可能输出计划话术、也可能在某些 provider 下拿不到可控收口结果。

证据：

- `internal/application/service/session_agent_qa.go` 的 `applyDocumentStopgapAgentConfig()`
- `internal/application/service/agent_service_external_tools_test.go` 的 `TestAgentServiceRegisterToolsAllowsFinalAnswerForStopgapOnly`
- `internal/application/service/session_agent_qa_test.go` 的 `TestApplyDocumentStopgapAgentConfig`

结论：

- 如果当前产品仍保留这条 stopgap 路径，就不能把这套 scoped final_answer 相关代码删掉。

### P1：如果删掉 final_answer 工具兼容解析，但保留 stopgap 工具注册，会出现空白答案或重复答案风险

影响范围：

- 任何仍可能返回 final_answer 工具调用的模型/路径。

原因：

- 当前分支的 `internal/models/chat/openai_stream.go` 会从 final_answer 工具参数里抽取 `answer` 字段流给前端。
- `internal/agent/observe.go` 会在 final_answer 工具出现时直接终止 loop，并容错解析 malformed JSON。
- 如果只保留工具注册，不保留这两段配套逻辑，前端可能只看到半成品，或者 loop 进入下一轮后重复收尾。

证据：

- `internal/agent/observe_test.go` 中的：
  - `TestAnalyzeResponse_FinalAnswer_ValidArgs`
  - `TestAnalyzeResponse_FinalAnswer_MalformedJSON_RecoveredViaRepair`
  - `TestAnalyzeResponse_FinalAnswer_UnrecoverableArgs_StillTerminates`
- `internal/agent/observe_test.go` 中的 `TestAnalyzeResponse_FinalAnswerToolStillEmitsAuthoritativeAnswerAfterPrefaceStream`

结论：

- scoped final_answer 如果保留，必须成套保留：工具定义、注册开关、流式抽取、终态收口、测试。

### P1：如果删掉 legacy final_answer 历史过滤，会污染旧会话回放

影响范围：

- 旧版本已落库会话。

原因：

- main 与当前分支都承认：历史会话里曾记录过 final_answer 工具调用。
- 如果回放这些历史消息时把它重新注入模型上下文，会造成重复答案，或者让模型误以为上一轮还在 mid-flight 工具阶段。

证据：

- `internal/application/service/agent_history.go` 中 `legacyFinalAnswerToolName` 的注释与过滤逻辑。

结论：

- 不应删除这部分兼容代码。

### P2：普通 Agent QA 不建议恢复成默认依赖 final_answer 工具

影响范围：

- 普通知识库问答、Web 搜索问答、一般 Agent ReAct 路径。

原因：

- main 的基线提交 `d38a6efd` 已明确把普通 Agent 改为 plain assistant text + natural stop。
- 该变更的目标是简化终止语义、减少工具协议噪音、消除 preamble 与 answer 区域跳动问题。
- 当前你遇到的 qwen3_coder 首轮“我先搜索知识库”后直接结束，根因是模型没有真正发出 tool_calls，不是因为少了 final_answer 工具。
- 把普通 Agent 全面恢复成 final_answer 工具模式，不能解决首轮不调工具的问题，反而会重新引入工具终态依赖、历史兼容、UI 收口复杂度。

结论：

- 普通 Agent QA 不需要回滚到“默认依赖 final_answer 工具”的旧设计。

## 5. 是否需要还原代码

### 5.1 不建议还原的部分

不建议从历史版本还原以下能力到“普通 Agent 默认路径”：

- 把 final_answer 恢复为所有 Agent 默认可见工具。
- 修改 prompt 让普通 Agent 强依赖 final_answer 才能结束。
- 为了迁就个别模型，把普通 Agent 的终止语义重新整体回退到 old path。

原因：

- 这与 main 分支当前架构方向相反。
- 不能解决当前 qwen3_coder 首轮不发 tool_calls 的根因。
- 会扩大工具协议与前端收口复杂度。

### 5.2 建议保留或恢复的部分

如果当前分支后续有人把 final_answer 相关代码继续删减，以下几组代码建议保留，若已删则建议恢复：

1. 文档 stopgap 专用 final_answer 工具链路
2. final_answer 工具参数的流式抽取与终态解析
3. legacy final_answer 历史过滤逻辑
4. final_answer 事件/字段的前后端契约

对应代码面：

- `internal/types/agent.go` 中的 `AllowFinalAnswerTool`
- `internal/application/service/agent_service.go` 中的 stopgap-only 注册控制
- `internal/application/service/session_agent_qa.go` 中的 `applyDocumentStopgapAgentConfig()`
- `internal/models/chat/openai_stream.go` 中的 `final_answer_tool` 增量抽取
- `internal/agent/observe.go` 中的 final_answer 兼容终止分支
- `internal/application/service/agent_history.go` 中的 legacy 过滤
- `internal/handler/session/agent_stream_handler.go` 与 `internal/handler/message.go` 中的 final_answer 事件/字段消费逻辑

## 6. 与当前 qwen3_coder 问题的关系

本次用户问题“首轮只说我要去搜索知识库，但没有真正完成检索流程”，与 final_answer 是否全局恢复不是同一个问题。

当前问题的根因是：

- 首轮 think 请求拿到了 tools，但模型没有发出任何 tool_calls。
- 服务端随后按 natural stop 收尾。

更合理的修复方向是：

- 对这类模型在首轮且存在工具时，显式发送 `tool_choice=required`。

这项修复和是否恢复普通 Agent 的 final_answer 工具是两条不同的控制轴，不应混用。

## 7. 审查建议

建议按以下原则继续演进：

1. 普通 Agent QA 继续保持 main 的 natural stop 主路径。
2. 当前分支 stopgap 专用 final_answer 工具链路保留，不要误删。
3. 如果未来确认文档 stopgap 路径已经完全退场，再统一删除 scoped final_answer 代码，但必须连同 stopgap 配置、流式抽取、终态解析、测试一起删，不要半删。
4. 对 qwen3_coder 这类“首轮不调工具”的模型，优先用请求约束修复，而不是回滚普通 Agent 的终止协议。

## 8. 本次核验结果

本次审查额外执行并通过了以下测试：

- `go test ./internal/application/service -run 'TestApplyDocumentStopgapAgentConfig$'`
- `go test ./internal/agent -run 'TestAnalyzeResponse_.*FinalAnswer|TestAnalyzeResponse_.*final_answer|TestStreamThinkingToEventBus_SourceAnswerTailTimeoutClosesAsPartial'`
- `go test ./internal/application/service -run 'TestAgentServiceRegisterToolsAllowsFinalAnswerForStopgapOnly$|TestRunDedicatedFullDocumentGenerationPath_NormalizesFinalAnswerAndEmitsQualityIssues$'`

这些测试支持以下结论：

- stopgap 专用 final_answer 配置在当前分支仍是有效设计。
- final_answer 兼容解析与收口逻辑在当前分支仍有测试覆盖。
- 长文档专用生成路径依赖 final answer 结果语义，但不依赖普通 Agent 默认暴露 final_answer 工具。