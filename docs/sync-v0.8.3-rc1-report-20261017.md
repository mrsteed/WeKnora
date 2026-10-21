# sync v0.8.3 rc1 回归报告（2026-10-17）

## 1. 结论

当前 `WeKnora-sync_v0.8.3_base` 已完成 Phase 0-Phase 6 的实现与回归修补，并进入 Phase 7 `rc1` 回归阶段。

本轮 `rc1` 自动化回归结果为：

1. 核心同步目标链路已通过：M5、M6 checklist 目标命令全部通过。
2. `docreader` 全量 `unittest discover` 已通过。
3. CLI contract 已通过。
4. `go vet ./...` 已通过。
5. 仍存在两类未闭合项：
   - Go 全量测试中的 4 个失败包。
   - 前端全量测试中的 3 个 style guard 红测。
6. 因此当前状态是：
   - `rc1` 已形成。
   - 但尚不建议直接判定“可上线”，除非这些例外被接受或继续清理。

## 2. 本轮已修复的回归

本次 Phase 7 过程中，额外修复了两类由前序同步带入的真实回归：

1. Phase 6 导出/解析回归：
   - 恢复 `internal/utils/export` 整包与导出路由接线。
   - 修正 `docreader` 与导出链路的 HTML table 行为：
     - 全局 `NormalizeHTMLTables` 现在对“轻度 span 表”转 Markdown、对“复杂 span 表”保留可切分 HTML。
     - DOCX 导出路径保留独立的 merged-cell 表格展开逻辑。
   - 修正迁移编号冲突：
     - versioned `000092_tenant_bootstrap_operations` → `000121_*`
     - versioned `000087/000088 created_by` → `000119/000120_*`
     - sqlite `000005_tenant_bootstrap_operations` → `000037_*`
     - 新增 sqlite `000038_im_channels_created_by` / `000039_knowledges_created_by`

2. 前端 locale 回归：
   - 使用项目自带脚本重建 pruned locale 文件。
   - 修正 `en-US` 中 workspace setup wizard 残留的 user-facing `tenant` 术语。

## 3. 自动化回归矩阵

### 3.1 已通过

1. `go test ./internal/utils/export -run 'TestMarkdownToHTML|TestBuildPDFHTMLDocument|TestResolveDocxAssets|TestMarkdownToDocx|TestMarkdownToPDF'`
2. `go test ./internal/infrastructure/docparser -run 'TestAnydocReader|TestNewMinerUReaderResolvesParseMethod|TestMinerUReaderV4FlowEndToEnd|TestResolveRemoteImages_|TestResolveAndStoreThenResolveRemoteImages|TestResolveAndStoreRelativeHTMLImages'`
3. `go test ./internal/infrastructure/docparser/anydoc`
4. `cd docreader && python -m unittest discover -s tests`
5. `cd frontend && pnpm type-check`
6. `go test ./internal/container -run '^$'`
7. `go test ./internal/router -run '^$'`
8. `go vet ./...`
9. `cd cli && go test ./acceptance/contract/...`
10. `git diff --check`

### 3.2 已明确例外

#### Go 全量 `go test ./...`

剩余失败包：

1. `github.com/Tencent/WeKnora/internal/application/repository`
   - 首个失败测试：`TestUpdateKnowledge_DoesNotClobberPendingCounter`
   - 首个失败信号：`attachment_descriptions.go:37 record not found`
   - 当前归类：历史红测

2. `github.com/Tencent/WeKnora/internal/application/service`
   - 首个失败测试：`TestSkillPythonVerifier`
   - 首个失败信号：`temporary_document.go:25 record not found`
   - 当前归类：历史红测

3. `github.com/Tencent/WeKnora/internal/handler`
   - 首个失败测试：`TestMoveAdmissionDeduplicatesIDsAndRequiresBothKBs`
   - 首个失败信号：`audit_log.go:242 db: connection refused`
   - 当前归类：环境缺口

4. `github.com/Tencent/WeKnora/internal/im`
   - 首个失败测试：`TestNewLifecycleTestDBIsolatesConcurrentTests`
   - 首个失败信号：`storageurl.go:148` 相关失败
   - 当前归类：历史红测

#### 前端全量 `npm test`

当前只剩 3 个 style guard 红测：

1. `style guard: radius-literal does not exceed baseline (160)`
   - 实际：`172`
   - 主要超标文件：
     - `views/agent/AgentEditorModal.vue`
     - `views/knowledge/components/FAQEntryManager.vue`
     - `components/ModelEditorDialog.vue`

2. `style guard: font-size-literal does not exceed baseline (33)`
   - 实际：`65`
   - 主要超标文件：
     - `views/admin/OrganManage.vue`
     - `views/admin/components/AssignOrgDialog.vue`
     - `views/admin/components/MemberTable.vue`

3. `style guard: motion-literal does not exceed baseline (64)`
   - 实际：`71`
   - 主要超标文件：
     - `views/knowledge/wiki/WikiBrowser.vue`
     - `views/creatChat/creatChat.vue`
     - `components/SpotlightGuide.vue`

当前归类：历史前端样式债务，不是本轮同步新增的功能性回归。

#### CLI 实服 e2e

1. `go test -tags acceptance_e2e ./acceptance/e2e/...`
2. 未执行原因：缺少 `WEKNORA_E2E_HOST` / `WEKNORA_E2E_TOKEN`
3. 当前归类：环境缺口

## 4. 手工 smoke 状态

本次会话未执行 Phase 7 计划中的完整 UI 手工 smoke，原因如下：

1. 需要可登录的测试账号与角色矩阵。
2. 需要在浏览器中完成 MCP OAuth / sandbox / 导出下载等交互。
3. 当前会话以代码与自动化回归收口为主，未引入新的浏览器端真实登录场景。

仍待执行的 smoke 项：

1. 用户名/邮箱登录
2. 初始化向导
3. 组织树多角色切换
4. Agent 创建/编辑/运行
5. MCP OAuth
6. sandbox 检查与真实会话执行
7. 目录树 / 批量下载 / 来源定位
8. 四种导出格式
9. 至少一组 Office/PDF/HTML 文档解析

## 5. rc1 判定

当前 `rc1` 判定为：

1. 自动化主链路基本成型。
2. Phase 6 相关回归已清干净。
3. 剩余问题已被压缩为：
   - 4 个 Go 全量失败包（历史/环境）
   - 3 个前端 style guard 历史债务
   - 1 个 CLI e2e 环境缺口
4. 若项目接受这些例外，且后续手工 smoke 通过，可进入 trial。
5. 若项目要求 `go test ./...` 与 `verify_frontend_pr.sh` 全绿，则需继续单独清理上述例外后再进入 trial。
