# 20261016 WeKnora M5-M7 具体执行清单

## 1. 文档目标

本文档继续承接前四份同步文档，把 M5、M6、M7 拆成可以直接执行的清单，重点覆盖：

1. 阶段主分支与子分支命名。
2. 每批建议参考的本地提交或现成结论。
3. 每批建议命令。
4. 每批验证命令。
5. 最终集成、试运行与切换顺序。

本文档默认前置条件已经满足：

1. `sync_v0.8.3_stage4_kb` 已完成。
2. 你已经可以在本地正常运行 0.8.3 基线和前几阶段代码。

默认工作目录：

```bash
cd "$HOME/workspace/WeKnora"
```

再次同步时，实际代码命令统一在执行 worktree 根目录运行：`$HOME/workspace/WeKnora-sync_v0.8.3_base`。
主仓 `WeKnora` 仅用于同步文档、remote 管理和 worktree 管理。

---

## 2. M5-M7 的执行原则

在进入 M5-M7 前，必须先服从总计划 [my_docs/sync_0.8.3/20261016-06-sync-master-execution-plan.md](my_docs/sync_0.8.3/20261016-06-sync-master-execution-plan.md) 第 `3.1` 节的第二轮同步硬约束。

M5、M6、M7 与前几阶段的最大区别在于：

1. **M5 以 0.8.3 上游骨架为主，本地只补门禁、范围与部署策略。**
2. **M6 不允许把导出链路和解析链路混成一个大包回迁。**
3. **M7 不是继续开发功能，而是做集成、试运行、备份、回滚和彩排。**

因此统一要求：

1. M5 不回退上游 MCP、sandbox、model catalog 主骨架。
2. M6 先验证上游 anydoc/MinerU/docreader，再决定回迁哪些本地补丁。
3. M7 先跑模块化验证，再跑全量回归；只有问题被归类回对应模块后，才允许返工。
4. 不允许为了“快速恢复”而把上游已有装配文件整文件替换成私有分支版本。
5. 若某能力在上游 `v0.8.3` 已存在但当前执行树表现异常，应先恢复上游文件形态，再补最小本地语义差异。

---

## 3. M5 执行清单：Agent、Model Catalog、MCP、Sandbox 的本地策略

## 3.1 M5 主分支

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage5_agent sync_v0.8.3_stage4_kb
git status --short --branch
```

## 3.2 M5 建议拆分

建议拆成四个子分支：

1. `sync_v0.8.3_stage5_agent_a_readiness`
2. `sync_v0.8.3_stage5_agent_b_tool_scope`
3. `sync_v0.8.3_stage5_agent_c_mcp`
4. `sync_v0.8.3_stage5_agent_d_sandbox`

---

## 4. M5-A：Agent 创建门禁与模型入口稳定性

### 分支名

`sync_v0.8.3_stage5_agent_a_readiness`

### 参考来源

1. `da33523e4`：协作者创建知识库/智能体时隐藏全局可见选项。
2. `2026-10-09` readiness 结论：创建校验必须收口到 shared opener，而不是只绑一个按钮。
3. `2026-10-09` contributor 403 结论：`AgentEditorModal` 不能被无关 admin-only 接口拖空模型列表。

### 关键文件

1. `frontend/src/views/agent/AgentList.vue`
2. `frontend/src/views/agent/AgentEditorModal.vue`
3. `frontend/src/stores/editorResources.*`
4. `frontend/src/utils/agent-readiness.ts`
5. `frontend/src/components/modelEditorSourceState.ts`

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage5_agent_a_readiness sync_v0.8.3_stage5_agent
git show --stat da33523e4
rg -n 'openCreateModal|handleOpenAgentEditor|ensureModels|prefetchAgentEditorDeps|storage-engine-status|allowed_tools' frontend/src/views/agent frontend/src/stores frontend/src/utils
```

### 执行重点

1. 创建门禁统一落在 shared opener，而不是单个按钮事件。
2. contributor 视角下，模型列表加载必须先走 viewer-safe 路径，不能被 admin-only 403 拖空。
3. 只保留本地创建门禁与可见性限制，不回退上游 model catalog/agent editor 骨架。

### 验证命令

```bash
cd "$HOME/workspace/WeKnora/frontend"
npm test -- src/utils/agent-readiness.test.mjs src/views/agent/AgentEditorModal.test.mjs src/stores/editorResources.test.mjs src/components/modelEditorSourceState.test.ts
npm run type-check
```

当前代码树里**没有**现成的 `AgentList.test.mjs` 文件，因此 `openCreateModal` 的 shared opener 收口仍要补手工 smoke。

### 手工 smoke

```bash
cd "$HOME/workspace/WeKnora"
make dev-start
make dev-app
make dev-frontend
```

手工验证：

1. 从所有“创建智能体”入口打开创建弹窗，确认都先经过 readiness 校验。
2. contributor 账号打开 Agent 编辑器时，模型列表不能因为无权读取存储配置而为空。
3. 协作者视角下，不该开放的全局可见选项仍被隐藏或受限。

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage5_agent
git merge --ff-only sync_v0.8.3_stage5_agent_a_readiness
```

---

## 5. M5-B：知识范围与默认工具语义

### 分支名

`sync_v0.8.3_stage5_agent_b_tool_scope`

### 参考来源

1. `8585a8c65`：同租户 KB/知识/智能体可见性与统一 KB 作用域解析。
2. `6116ddcb9`：平台特权统一与组织可见性。
3. 当前上游 0.8.3 tool catalog / search targets / wiki scopes 主骨架。

### 关键文件

1. `internal/application/service/agent_kb_scope.go`
2. `internal/agent/tools/scope_authorization_test.go`
3. `internal/agent/tools/grep_chunks_scope_test.go`
4. `internal/types/search_scope*`
5. `frontend/src/views/agent/AgentEditorModal.vue`

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage5_agent_b_tool_scope sync_v0.8.3_stage5_agent
git show --stat 8585a8c65
git show --stat 6116ddcb9
rg -n 'SearchTargets|WikiScope|allowed_tools|knowledge_search|wiki_search|RestrictKnowledgeTargets|NewWikiScopes' internal/application internal/agent frontend/src/views/agent
```

### 执行重点

1. Agent 的 all KB / selected KB / wiki-only 范围必须复用 M3 的授权结果。
2. 允许的工具列表不能比真实可访问范围更宽。
3. “默认工具为空数组”与“工具未设置”要保持 0.8.3 的语义，不要误改。

### 验证命令

```bash
cd "$HOME/workspace/WeKnora"
go test ./internal/application/service -run 'TestAgentKBScopeResolver'
go test ./internal/agent/tools -run 'TestScopeClause_|TestAuthorizeKnowledgeInSearchTargets|TestNewWikiScopes|TestPagePassesWikiScope|TestValidateKnowledgeBaseIDsInSearchTargets'
```

前端：

```bash
cd "$HOME/workspace/WeKnora/frontend"
npm run type-check
```

### 手工 smoke

1. 智能体分别配置 `all`、`selected`、`none` 三种 KB 模式。
2. 分别尝试 `knowledge_search`、`wiki_search`、`read_document` 等工具组合。
3. 验证被组织可见性、shared KB、private KB 限制的知识不会被工具越权检索。

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage5_agent
git merge --ff-only sync_v0.8.3_stage5_agent_b_tool_scope
```

---

## 6. M5-C：Built-in MCP Server 与 OAuth / 工具目录

### 分支名

`sync_v0.8.3_stage5_agent_c_mcp`

### 参考来源

1. 上游 0.8.3 built-in MCP server 主骨架。
2. 当前私有仓库关于 MCP 的原则：旧 `mcp-server/` Python 目录不再作为主产品面。
3. 本地只补部署与门禁，不重造 MCP 产品面。

### 关键文件

1. `internal/mcp/`
2. `internal/application/service/mcp_service.go`
3. `internal/agent/tools/mcp_tool*.go`
4. `internal/handler/dto/mcp.go`
5. `frontend/src/views/chat/components/McpOAuthCard.vue`
6. `frontend/src/views/settings/` 或 `integrations` 对应设置入口

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage5_agent_c_mcp sync_v0.8.3_stage5_agent
rg -n 'mcp-endpoint|discover_mcp_tools|call_mcp_tool|oauth|MCPOAuth|MCPService' internal frontend/src
```

### 执行重点

1. built-in MCP server 保持 0.8.3 主路径，不把 Python `mcp-server/` 拉回主入口。
2. OAuth 流程、principal 隔离、tool catalog 和 viewer/detail 脱敏语义保持上游结构。
3. 本地只补配置、权限和集成入口细节。

### 验证命令

后端：

```bash
cd "$HOME/workspace/WeKnora"
go test ./internal/mcp -run 'TestAsOAuthRequired|TestOAuthRuntime|TestOAuthCall|TestDBTokenStoreUsesPrincipal|TestOAuthAttemptCompletesOnlyAfterCallbackExchange|TestValidateServiceOutboundURLsRejectsOAuthMetadataSSRF'
go test ./internal/application/service -run 'TestUpdateMCPService|TestUpdateMCPCredentials|TestClearMCPCredential|TestGetMCPServiceByID|TestListMCPServices'
go test ./internal/agent/tools -run 'TestMCPToolName_|TestMCPToolDescription|TestMCPToolParameters|TestToolRegistryMCPToolLookup|TestExtractContentAndImages'
go test ./internal/handler/dto -run 'TestMCPServiceResponse_ViewerStripsIntegrationDetail'
```

前端：

```bash
cd "$HOME/workspace/WeKnora/frontend"
npm test -- src/views/chat/components/McpOAuthCard.oauth.test.mjs
npm run type-check
```

### 手工 smoke

1. 在集成设置中创建或编辑一个 MCP 服务。
2. 验证 OAuth 授权卡片在聊天中可正确轮询与绑定授权 attempt。
3. 验证工具名稳定，不因重连变成 UUID 风格名称。
4. viewer 视角下检查集成详情是否正确脱敏。

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage5_agent
git merge --ff-only sync_v0.8.3_stage5_agent_c_mcp
```

---

## 7. M5-D：Sandbox 配置、绑定与运行时策略

### 分支名

`sync_v0.8.3_stage5_agent_d_sandbox`

### 参考来源

1. 上游 0.8.3 sandbox 主骨架。
2. 当前私有仓库结论：不回迁任何 0.7.2 时代 host sandbox 逻辑。
3. anydoc 静态库构建结论仅用于 M6，不要在 M5-D 中混入解析补丁。

### 关键文件

1. `internal/sandbox/`
2. `internal/handler/sandbox_check_test.go`
3. `internal/handler/sandbox_config_test.go`
4. `internal/handler/custom_agent_sandbox_test.go`
5. `internal/application/service/session_sandbox_*`
6. `internal/application/service/tenant_sandbox_config_test.go`

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage5_agent_d_sandbox sync_v0.8.3_stage5_agent
rg -n 'SandboxConfig|SessionSandbox|sandbox_config_id|ResolveEffectiveConfig|RemoteSessionLifecycle|E2BRemoteClient|CubeRemoteClient' internal/sandbox internal/handler internal/application/service
```

### 执行重点

1. sandbox config、agent sandbox config id、session binding 三层语义保持 0.8.3 上游骨架。
2. 本地只补配置策略、部署说明和受限入口，不做回退式简化。
3. 若需要实机验证 Docker/E2B/Cube，优先先跑无外部依赖的单元测试，再跑带 build tag 的集成测试。

### 验证命令

后端：

```bash
cd "$HOME/workspace/WeKnora"
go test ./internal/handler -run 'TestSandboxConnectionCheckConfigAllowsTemplateDiscoveryAfterConnection|TestRunStatelessSandboxCheckLocal|TestSandboxConfig|TestAgentSandboxConfig'
go test ./internal/application/service -run 'TestSandboxConfigForExistingSandboxReturnsEmptyWhenUnpinned|TestSessionSandboxInputStore|TestResolveSandboxForExecution|TestSandboxIdentityChanged|TestUpdateRefusesIdentityChangeWhileSandboxesLive|TestDeleteRefusesWhileSandboxesLive|TestSanitizeSandboxConfig'
go test ./internal/sandbox -run 'TestResolveEffectiveConfig|TestRemoteSessionLifecycle|TestE2BRemoteClient|TestCubeRemoteClient|TestPolicy|TestNewRemoteClientForCheckRefusesLinkLocalAtDial'
```

可选集成测试，仅在相应控制面已准备好时执行：

```bash
cd "$HOME/workspace/WeKnora"
go test -tags=integration -run Integration ./internal/sandbox/...
go test -tags=e2b_integration -run E2BIntegration ./internal/sandbox/...
```

### 手工 smoke

1. 新建一个 sandbox config 并执行 connection check。
2. 绑定到一个 smart-reasoning agent，确认 `sandbox_config_id` 能随 Agent config round-trip。
3. 若部署允许，实际跑一次会话级 sandbox exec/terminal/文件操作。

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage5_agent
git merge --ff-only sync_v0.8.3_stage5_agent_d_sandbox
```

---

## 8. M5 阶段验证命令

四个子分支都回并后，统一跑：

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage5_agent
go test ./internal/application/service -run 'TestAgentKBScopeResolver|TestUpdateMCPService|TestUpdateMCPCredentials|TestClearMCPCredential|TestSandboxConfigForExistingSandboxReturnsEmptyWhenUnpinned|TestSessionSandboxInputStore|TestResolveSandboxForExecution|TestSandboxIdentityChanged'
go test ./internal/agent/tools -run 'TestScopeClause_|TestAuthorizeKnowledgeInSearchTargets|TestNewWikiScopes|TestPagePassesWikiScope|TestValidateKnowledgeBaseIDsInSearchTargets|TestMCPToolName_|TestMCPToolDescription|TestMCPToolParameters|TestToolRegistryMCPToolLookup|TestExtractContentAndImages'
go test ./internal/mcp -run 'TestAsOAuthRequired|TestOAuthRuntime|TestOAuthCall|TestDBTokenStoreUsesPrincipal|TestOAuthAttemptCompletesOnlyAfterCallbackExchange|TestValidateServiceOutboundURLsRejectsOAuthMetadataSSRF'
go test ./internal/sandbox -run 'TestResolveEffectiveConfig|TestRemoteSessionLifecycle|TestE2BRemoteClient|TestCubeRemoteClient|TestPolicy|TestNewRemoteClientForCheckRefusesLinkLocalAtDial'
cd "$HOME/workspace/WeKnora/frontend" && npm test -- src/utils/agent-readiness.test.mjs src/views/agent/AgentEditorModal.test.mjs src/stores/editorResources.test.mjs src/components/modelEditorSourceState.test.ts src/views/chat/components/McpOAuthCard.oauth.test.mjs
cd "$HOME/workspace/WeKnora/frontend" && npm run type-check
```

## 8.1 M5 完成标准

1. Agent 创建门禁稳定。
2. 工具范围与知识范围不越权。
3. built-in MCP server 以 0.8.3 主骨架运行。
4. sandbox 配置、绑定和运行时策略已稳定。

---

## 9. M6 执行清单：导出链路、解析链路、docreader/anydoc/MinerU 取舍

## 9.1 M6 主分支

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage6_export_parse sync_v0.8.3_stage5_agent
git status --short --branch
```

## 9.2 M6 建议拆分

建议拆成三个子分支：

1. `sync_v0.8.3_stage6_export_parse_a_export`
2. `sync_v0.8.3_stage6_export_parse_b_anydoc`
3. `sync_v0.8.3_stage6_export_parse_c_mineru_docreader`

---

## 10. M6-A：导出链路

### 分支名

`sync_v0.8.3_stage6_export_parse_a_export`

### 参考提交

1. `f3f628b10`
2. `38f9ab559`
3. `49b61b9e5`
4. `cfd6a5b87`

### 关键文件

1. `internal/utils/export/`
2. `config/export/`
3. 聊天或知识导出相关前端入口

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage6_export_parse_a_export sync_v0.8.3_stage6_export_parse
git show --stat f3f628b10
git show --stat 38f9ab559
git show --stat 49b61b9e5
git show --stat cfd6a5b87
rg -n 'export|MarkdownToPDF|MarkdownToDocx|xlsx|pdf.css|reference.docx' internal/utils/export config/export frontend/src
```

### 执行重点

1. 先确认 M1 已恢复导出资源与挂载。
2. 只把导出逻辑映射到 0.8.3 当前消息/附件/引用结构，不要回退旧前端数据结构。
3. 文件名按主题命名可以保留，但必须与 0.8.3 当前消息模型解耦。

### 验证命令

```bash
cd "$HOME/workspace/WeKnora"
go test ./internal/utils/export -run 'TestMarkdownToHTML|TestBuildPDFHTMLDocument|TestResolveDocxAssets|TestMarkdownToDocx|TestMarkdownToPDF'
cd "$HOME/workspace/WeKnora/frontend" && npm run type-check
```

### 手工 smoke

1. 从聊天记录导出 Markdown。
2. 导出 PDF，确认 KaTeX、图片、表格可见。
3. 导出 Word，确认表格和图片保留。
4. 导出 XLSX，确认文件可打开。
5. 验证导出文件名按内容主题命名。

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage6_export_parse
git merge --ff-only sync_v0.8.3_stage6_export_parse_a_export
```

---

## 11. M6-B：anydoc 解析链路

### 分支名

`sync_v0.8.3_stage6_export_parse_b_anydoc`

### 参考来源

1. 0.8.3 上游 anydoc 主骨架。
2. `2026-09-22` anydoc 静态库构建结论：`make anydoc-lib` 需要 Rust，`make dev-app` 会自动探测 build tag。

### 关键文件

1. `third_party/anydoc-go/`
2. `internal/infrastructure/docparser/anydoc_reader.go`
3. `internal/infrastructure/docparser/anydoc/`
4. `scripts/build-anydoc-lib.sh`
5. `scripts/dev.sh`

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage6_export_parse_b_anydoc sync_v0.8.3_stage6_export_parse
make anydoc-lib
make build-anydoc
rg -n 'anydoc|GO_BUILD_TAGS|enable_anydoc_build_tag' internal/infrastructure/docparser scripts/dev.sh third_party/anydoc-go
```

### 执行重点

1. 先验证 anydoc 链路是否在当前环境可构建、可链接。
2. 不要为了保留本地 patch 去破坏“无静态库时自动回退”的上游行为。
3. 构建好静态库后，重启 `make dev-app` 让 build tag 生效。

### 验证命令

```bash
cd "$HOME/workspace/WeKnora"
go test ./internal/infrastructure/docparser -run 'TestAnydocReader|TestNewAnydocReader|TestImageRefsFromAssets'
go test ./internal/infrastructure/docparser/anydoc
```

如需验证 anydoc build tag 真正生效：

```bash
cd "$HOME/workspace/WeKnora"
make dev-app
```

观察 `scripts/dev.sh` 输出是否出现“已启用 -tags anydoc”。

### 手工 smoke

1. 用 doc/docx/ppt/pptx/xls/xlsx 各选至少一类样本。
2. 上传后核对解析内容、图片抽取和附件预览。
3. 对扫描 PDF 验证 anydoc 是否按上游逻辑回退或报出正确错误。

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage6_export_parse
git merge --ff-only sync_v0.8.3_stage6_export_parse_b_anydoc
```

---

## 12. M6-C：MinerU、HTML 表格与 DocReader 解析链路

### 分支名

`sync_v0.8.3_stage6_export_parse_c_mineru_docreader`

### 参考提交

1. `2b91b999a`
2. `83949fc91`

### 关键文件

1. `internal/infrastructure/docparser/mineru_converter.go`
2. `internal/infrastructure/docparser/resolve_remote_html_images_test.go`
3. `internal/infrastructure/docparser/resolve_remote_images_test.go`
4. `docreader/parser/`
5. `docreader/tests/`

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage6_export_parse_c_mineru_docreader sync_v0.8.3_stage6_export_parse
git show --stat 2b91b999a
git show --stat 83949fc91
rg -n 'MinerU|html_parser|rowspan|colspan|ResolveRemoteImages|docx_merge|excel_parser' internal/infrastructure/docparser docreader/parser docreader/tests
```

### 执行重点

1. 先验证上游 MinerU 4.0 / HTML / Office / image resolver 行为。
2. 只有“上游仍未覆盖”的缺陷才回迁本地补丁。
3. HTML 表格、远程图片、扫描 PDF、Office merge、引用定位要串起来回归。

### 验证命令

Go 侧：

```bash
cd "$HOME/workspace/WeKnora"
go test ./internal/infrastructure/docparser -run 'TestNewMinerUReaderResolvesParseMethod|TestMinerUReaderV4FlowEndToEnd|TestResolveRemoteImages_|TestResolveAndStoreThenResolveRemoteImages|TestResolveAndStoreRelativeHTMLImages'
```

DocReader Python 侧：

```bash
cd "$HOME/workspace/WeKnora/docreader"
python -m unittest tests.test_html_parser tests.test_pdf_router tests.test_docx_merge tests.test_excel_parser tests.test_mhtml_parser tests.test_opendataloader_parser
```

如果要跑更全量的 docreader 回归：

```bash
cd "$HOME/workspace/WeKnora/docreader"
python -m unittest discover -s tests
```

### 手工 smoke

1. HTML 文档含复杂表格、rowspan/colspan。
2. 扫描 PDF。
3. 含合并单元格的 docx/xlsx。
4. 含远程图片和内嵌图片的文档。
5. 导入后检查聊天引用、知识库预览、原文下载是否一致。

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage6_export_parse
git merge --ff-only sync_v0.8.3_stage6_export_parse_c_mineru_docreader
```

---

## 13. M6 阶段验证命令

三个子分支都回并后，统一跑：

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage6_export_parse
go test ./internal/utils/export -run 'TestMarkdownToHTML|TestBuildPDFHTMLDocument|TestResolveDocxAssets|TestMarkdownToDocx|TestMarkdownToPDF'
go test ./internal/infrastructure/docparser -run 'TestAnydocReader|TestNewMinerUReaderResolvesParseMethod|TestMinerUReaderV4FlowEndToEnd|TestResolveRemoteImages_|TestResolveAndStoreThenResolveRemoteImages|TestResolveAndStoreRelativeHTMLImages'
go test ./internal/infrastructure/docparser/anydoc
cd "$HOME/workspace/WeKnora/docreader" && python -m unittest discover -s tests
cd "$HOME/workspace/WeKnora/frontend" && npm run type-check
```

## 13.1 M6 完成标准

1. 四种导出格式可用。
2. anydoc 构建、链接、运行路径已验证。
3. MinerU / HTML / Office / image resolver 解析结果不退化。
4. 导出、预览、下载、聊天引用与解析结果保持一致。

---

## 14. M7 执行清单：全量回归、试运行与切换

## 14.1 M7 主分支

建议把当前 `sync_v0.8.3` 作为最终集成主分支使用。若你想保留它作为纯规划分支，也可以另起：

1. `sync_v0.8.3_rc1`
2. `sync_v0.8.3_trial`

推荐流程：

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3
git merge --ff-only sync_v0.8.3_stage6_export_parse
git switch -c sync_v0.8.3_rc1
git status --short --branch
```

## 14.2 M7 建议拆分

建议拆成两个执行批次：

1. `sync_v0.8.3_rc1`：全量自动化回归与缺陷归类。
2. `sync_v0.8.3_trial`：试运行、备份、切换与回滚演练。

---

## 15. M7-A：全量自动化回归与综合 smoke

### 分支名

`sync_v0.8.3_rc1`

### 建议命令

后端全量：

```bash
cd "$HOME/workspace/WeKnora"
go test ./...
go vet ./...
```

前端全量：

```bash
cd "$HOME/workspace/WeKnora"
./scripts/verify_frontend_pr.sh
```

DocReader 全量：

```bash
cd "$HOME/workspace/WeKnora/docreader"
python -m unittest discover -s tests
```

CLI 合同测试：

```bash
cd "$HOME/workspace/WeKnora/cli"
go test ./acceptance/contract/...
```

CLI 实服 e2e，仅在测试环境变量已准备好时执行：

```bash
cd "$HOME/workspace/WeKnora/cli"
WEKNORA_E2E_HOST=https://your-host \
WEKNORA_E2E_TOKEN=your-token \
go test -tags acceptance_e2e ./acceptance/e2e/...
```

可选 CLI 实服 smoke：

```bash
cd "$HOME/workspace/WeKnora/cli"
go run . kb check <kb-id>
go run . agent check <agent-id>
go run . doc wait <doc-id> --timeout 10m
```

### 说明

1. `go test ./...` 很可能会暴露与本次同步无关的存量红测；遇到这种情况不要在 M7 临时乱修，应回分类到对应模块或明确列为历史问题。
2. `verify_frontend_pr.sh` 会顺序跑 `npm test`、`npm run type-check`、`npm run build`。
3. CLI 实服 e2e 需要真实 host 与 token，不具备条件时不要伪造。

### 手工 smoke

M7-A 必做的综合 smoke：

1. 用户名/邮箱登录。
2. 初始化向导。
3. 组织树多角色切换。
4. Agent 创建、编辑、运行。
5. MCP OAuth。
6. sandbox 检查与一次真实会话执行。
7. 批量下载、来源定位、目录树。
8. 四种导出格式。
9. 至少一组 Office/PDF/HTML 文档解析。

### 输出物

建议输出一份 `rc1-回归报告`，至少包含：

1. 自动化通过项。
2. 自动化失败项。
3. 手工 smoke 结果。
4. 待回模块修复项。

---

## 16. M7-B：试运行、备份、切换与回滚演练

### 分支名

`sync_v0.8.3_trial`

### 建议命令

#### 1. 试运行前备份

```bash
cd "$HOME/workspace/WeKnora"
ts="$(date +%Y%m%d-%H%M%S)"
mkdir -p "backups/$ts"
cp .env "backups/$ts/.env.pre-upgrade"
docker compose ps > "backups/$ts/docker-compose.ps.txt"
[ -n "$DB_URL" ] && pg_dump "$DB_URL" > "backups/$ts/weknora.sql"
```

如果对象存储、导出资源或挂载目录使用本地卷，也应同时做卷级备份；这部分因部署形态不同，需要按你的实际环境补充。

#### 2. 部署试运行分支

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_trial sync_v0.8.3_rc1
docker compose pull
docker compose up -d
make migrate-version
curl -sf http://localhost:8080/health
curl -sf http://localhost:8080/api/v1/system/capabilities >/dev/null
```

#### 3. 观察日志与运行态

```bash
cd "$HOME/workspace/WeKnora"
docker compose logs --tail=120 app
docker compose logs --tail=120 frontend
docker compose logs --tail=120 docreader
```

#### 4. 回滚演练

代码/镜像回滚路径：

```bash
cd "$HOME/workspace/WeKnora"
cp "backups/$ts/.env.pre-upgrade" .env
docker compose pull
docker compose up -d
```

数据库回滚只在必须时执行，且优先走整库恢复，而不是盲目 `migrate-down`：

```bash
psql "$DB_URL" < "backups/$ts/weknora.sql"
```

### 说明

1. 升级回滚优先走“旧镜像 + 旧配置 + 备份库恢复”，不要把 `migrate-down` 当默认生产回滚方案。
2. 试运行期间要重点观察：附件保留、引用定位、MCP OAuth、sandbox 绑定、ParadeDB/索引构建、对象存储 URL 重写。

### 输出物

建议输出一份 `trial-试运行报告`，至少包含：

1. 备份文件位置。
2. 切换开始与结束时间。
3. 健康检查结果。
4. 是否触发回滚、若触发则回滚耗时。

---

## 17. M5-M7 串行回并顺序

建议按下面顺序推进：

```text
sync_v0.8.3_stage5_agent
  -> sync_v0.8.3_stage5_agent_a_readiness
  -> sync_v0.8.3_stage5_agent_b_tool_scope
  -> sync_v0.8.3_stage5_agent_c_mcp
  -> sync_v0.8.3_stage5_agent_d_sandbox

sync_v0.8.3_stage6_export_parse
  -> sync_v0.8.3_stage6_export_parse_a_export
  -> sync_v0.8.3_stage6_export_parse_b_anydoc
  -> sync_v0.8.3_stage6_export_parse_c_mineru_docreader

sync_v0.8.3
  -> sync_v0.8.3_rc1
  -> sync_v0.8.3_trial
```

强依赖约束：

1. M5-B 必须建立在 M3 已稳定的前提上。
2. M5-C/M5-D 都不要先于 M5-A 完成，否则入口门禁与配置路径会重复返工。
3. M6-B/M6-C 最好在 M4 的来源定位已稳定后再做联调。
4. M7 只有在 M5、M6 阶段验证全部过一遍后才允许开始。

---

## 18. 一页式命令索引

如果只看命令，建议最短路径如下：

```bash
cd "$HOME/workspace/WeKnora"

git switch -c sync_v0.8.3_stage5_agent sync_v0.8.3_stage4_kb
git switch -c sync_v0.8.3_stage5_agent_a_readiness
git switch sync_v0.8.3_stage5_agent && git merge --ff-only sync_v0.8.3_stage5_agent_a_readiness
git switch -c sync_v0.8.3_stage5_agent_b_tool_scope
git switch sync_v0.8.3_stage5_agent && git merge --ff-only sync_v0.8.3_stage5_agent_b_tool_scope
git switch -c sync_v0.8.3_stage5_agent_c_mcp
git switch sync_v0.8.3_stage5_agent && git merge --ff-only sync_v0.8.3_stage5_agent_c_mcp
git switch -c sync_v0.8.3_stage5_agent_d_sandbox
git switch sync_v0.8.3_stage5_agent && git merge --ff-only sync_v0.8.3_stage5_agent_d_sandbox

git switch -c sync_v0.8.3_stage6_export_parse sync_v0.8.3_stage5_agent
git switch -c sync_v0.8.3_stage6_export_parse_a_export
git switch sync_v0.8.3_stage6_export_parse && git merge --ff-only sync_v0.8.3_stage6_export_parse_a_export
git switch -c sync_v0.8.3_stage6_export_parse_b_anydoc
git switch sync_v0.8.3_stage6_export_parse && git merge --ff-only sync_v0.8.3_stage6_export_parse_b_anydoc
git switch -c sync_v0.8.3_stage6_export_parse_c_mineru_docreader
git switch sync_v0.8.3_stage6_export_parse && git merge --ff-only sync_v0.8.3_stage6_export_parse_c_mineru_docreader

git switch sync_v0.8.3
git merge --ff-only sync_v0.8.3_stage6_export_parse
git switch -c sync_v0.8.3_rc1
git switch -c sync_v0.8.3_trial
```

---

## 19. 结论

M5-M7 的关键不是再“多加功能”，而是：

1. M5 把 0.8.3 的 Agent / MCP / sandbox 主骨架稳定接住。
2. M6 把导出链路和解析链路分开验证，避免一个问题拖垮整个知识加工面。
3. M7 用自动化回归、试运行和回滚演练把升级从“能改代码”推进到“能上线”。

到这一步，整套 0.7.2 -> 0.8.3 的同步文档已经从总纲、模块化策略、M0-M2、M3-M4 一直细化到了 M5-M7，可以直接作为后续分阶段执行的操作蓝图。