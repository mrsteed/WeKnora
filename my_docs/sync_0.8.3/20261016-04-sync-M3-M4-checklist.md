# 20261016 WeKnora M3-M4 具体执行清单

## 1. 文档目标

本文档承接前两份执行清单，继续把 M3、M4 拆到可直接执行的粒度，重点覆盖：

1. 阶段主分支与子分支命名。
2. 每批建议参考的本地提交。
3. 每批建议命令。
4. 每批验证命令。
5. 每批回并顺序。

本文档默认前置条件已经满足：

1. `sync_v0.8.3_base` 已可启动。
2. `sync_v0.8.3_stage1_platform` 已完成。
3. `sync_v0.8.3_stage2_auth` 已完成。

默认工作目录：

```bash
cd "$HOME/workspace/WeKnora"
```

再次同步时，实际代码命令统一在执行 worktree 根目录运行：`$HOME/workspace/WeKnora-sync_v0.8.3_base`。
主仓 `WeKnora` 仅用于同步文档、remote 管理和 worktree 管理。

---

## 2. M3、M4 的执行原则

在进入 M3、M4 前，必须先服从总计划 [my_docs/sync_0.8.3/20261016-06-sync-master-execution-plan.md](my_docs/sync_0.8.3/20261016-06-sync-master-execution-plan.md) 第 `3.1` 节的第二轮同步硬约束。

M3 和 M4 与前面不同，不能先改页面再倒推后端。统一执行顺序必须是：

1. 服务层。
2. handler / middleware / router。
3. 前端页面、store、组件。
4. 手工多角色 smoke。

原因：

1. M3 的本质是“谁能看见谁、谁能操作谁”。
2. M4 的本质是“知识条目、目录、移动、下载、来源定位是否服从 M3 的权限语义”。

因此：

1. **M3 不稳定时，不要进入 M4。**
2. **M4 的 move-targets、批量下载、来源定位都必须复用 M3 的授权模型。**
3. **即使为了回迁 org-tree substrate，也不能对 `container.go` / `router.go` 等上游装配文件做整文件覆盖。**
4. **凡是上游 `v0.8.3` 已存在的文件，一律以 `v0.8.3` 为底按 hunk 吸收本地语义。**

---

## 3. M3 执行清单：组织树、RBAC、可见性、IM 归属主线

## 3.1 M3 主分支

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage3_scope sync_v0.8.3_stage2_auth
git status --short --branch
```

## 3.2 M3 建议拆分

建议拆成四个子分支：

1. `sync_v0.8.3_stage3_scope_a_scope_core`
2. `sync_v0.8.3_stage3_scope_b_visibility`
3. `sync_v0.8.3_stage3_scope_c_orgtree`
4. `sync_v0.8.3_stage3_scope_d_im`

### 3.2.1 实际依赖修正（2026-10-16）

执行 Phase 2 后已确认：

1. `M2-B`（bootstrap / WorkspaceSetupWizard / 根组织复用）依赖 org-tree substrate。
2. 当前上游 `v0.8.3` 基线不包含 org-tree 子系统。
3. 因此进入 M3 后，**不能直接从 M3-A 开始**。

实际执行顺序应改为：

1. **M3-S0：先带回 org-tree substrate**。
2. 再做 `M3-A same-tenant scope core`。
3. 再做 `M3-B KB/Agent visibility`。
4. 再做 `M3-C orgtree 页面与成员管理`。
5. 再做 `M3-D IM owner scope`。

然后回头补完：

1. `M2-B` 的 `WorkspaceSetupWizard` / `tenant_bootstrap` 接线。
2. `M2-C` 中依赖 `stores/orgTree.ts` 的跨账号缓存卫生。

## 3.3 M3-S0：org-tree substrate 先行批次

### 目标

把当前上游 `v0.8.3` 基线里缺失的一整套 org-tree substrate 先带回来，至少包含：

1. 后端 org-tree 类型 / 仓储 / service / handler。
2. 前端 `api/org-tree`、`stores/orgTree.ts`、`views/admin/OrganManage.vue` 及其组件。
3. 让 Agent / KB 编辑器里依赖 org-tree 的组织可见性 UI 不再缺底座。

### 关键文件

后端：

1. `internal/types/interfaces/org_tree.go`
2. `internal/types/org_tree.go`
3. `internal/application/repository/org_tree.go`
4. `internal/application/service/org_tree.go`
5. `internal/handler/org_tree.go`
6. `internal/handler/org_tree_provisioning.go`

前端：

1. `frontend/src/api/org-tree/index.ts`
2. `frontend/src/stores/orgTree.ts`
3. `frontend/src/views/admin/OrganManage.vue`
4. `frontend/src/views/admin/components/*`
5. `frontend/src/components/OrgTreeSelector.vue`
6. `frontend/src/components/OrganizationSwitcher.vue`

接线文件：

1. `internal/container/container.go`
2. `internal/router/router.go`
3. `internal/handler/knowledgebase.go`
4. `frontend/src/stores/organization.ts`
5. `frontend/src/stores/auth.ts`
6. `frontend/src/router/index.ts`
7. `frontend/src/views/platform/index.vue`
8. `frontend/src/views/agent/AgentEditorModal.vue`
9. `frontend/src/views/knowledge/KnowledgeBaseEditorModal.vue`

### 参考提交

1. `eef494451`
2. `442958421`
3. `ee6cbeade`
4. `fb629a50e`（仅取其中的 org-tree / tenant_bootstrap substrate 相关文件，不在本批次完成 WorkspaceSetupWizard）

### 完成标准

1. org-tree substrate 文件已带回当前执行分支。
2. 前端 `npm run type-check` 可通过。
3. 后端至少能通过 org-tree / visibility 相关窄编译或窄测试。
4. 到这一步才允许继续做 `M3-A`，也才允许回头继续 `M2-B`。

---

## 4. M3-A：同租户作用域解析器

### 分支名

`sync_v0.8.3_stage3_scope_a_scope_core`

### 参考提交

1. `8585a8c65`
2. `6116ddcb9`
3. `442958421`

### 关键文件

1. `internal/application/service/same_tenant_scope.go`
2. `internal/application/service/kb_visibility.go`
3. `internal/application/service/agent_visibility.go`
4. `internal/router/rbac.go`
5. `internal/middleware/access.go`
6. `internal/middleware/kb_access.go`

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage3_scope_a_scope_core sync_v0.8.3_stage3_scope
git show --stat 8585a8c65
git show --stat 6116ddcb9
git show --stat 442958421
rg -n 'sameTenantResourceAuthorizer|ListAccessibleKBs|CanManageKB|CanAccessKB' internal/application/service internal/handler internal/middleware internal/router
```

### 执行重点

1. 先恢复统一的 same-tenant scope 解析层。
2. 明确 `global/org/private` 在 0.8.3 类型和 handler 里的表达方式。
3. 把“用户组织 + 后代组织 + 创建者 + tenant role”解析收口在一个地方。
4. 让 API key allow-list 和用户视角共用一套收口规则。

### 验证命令

```bash
cd "$HOME/workspace/WeKnora"
go test ./internal/application/service -run 'TestSameTenantResourceAuthorizer'
go test ./internal/application/service -run 'TestAgentKBScopeResolver'
```

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage3_scope
git merge --ff-only sync_v0.8.3_stage3_scope_a_scope_core
```

---

## 5. M3-B：KB/智能体可见性

### 分支名

`sync_v0.8.3_stage3_scope_b_visibility`

### 参考提交

1. `8585a8c65`
2. `6116ddcb9`
3. `2462d4885`

### 关键文件

1. `internal/application/service/kb_visibility.go`
2. `internal/application/service/agent_kb_scope.go`
3. `internal/application/repository/knowledgebase.go`
4. `internal/application/repository/custom_agent.go`
5. `internal/handler/knowledgebase.go`
6. `internal/handler/custom_agent.go`
7. `frontend/src/views/agent/AgentEditorModal.vue`
8. `frontend/src/stores/editorResources.*`

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage3_scope_b_visibility sync_v0.8.3_stage3_scope
git show --stat 8585a8c65
git show --stat 6116ddcb9
git show --stat 2462d4885
rg -n 'ListAccessibleKBs|RestrictKnowledgeTargets|visibility|organization_id|created_by' internal/application internal/handler frontend/src/views/agent frontend/src/stores
```

### 执行重点

1. KB 可见性服务先落地，再把 Agent 的知识范围接上去。
2. “智能体看到哪些 KB”与“用户在页面里能选哪些 KB”必须一致。
3. 不要让前端自己拼可见范围，前端只消费后端收口后的结果。
4. contributor 视角下 AgentEditorModal 不能因为 admin-only 接口失败而把模型下拉拖空。

### 验证命令

后端：

```bash
cd "$HOME/workspace/WeKnora"
go test ./internal/application/service -run 'TestAgentKBScopeResolver'
```

前端：

```bash
cd "$HOME/workspace/WeKnora/frontend"
npm test -- src/views/agent/AgentEditorModal.test.mjs src/stores/editorResources.test.mjs
npm run type-check
```

### 手工 smoke

启动开发环境：

```bash
cd "$HOME/workspace/WeKnora"
make dev-start
make dev-app
make dev-frontend
```

手工验证：

1. Owner/Admin/Contributor/Viewer 四种空间角色进入 Agent 编辑器，查看可选 KB 是否符合预期。
2. contributor 账号打开 Agent 创建/编辑弹窗时，模型列表不应被无关 403 拖空。
3. shared KB、org visible KB、private KB 在 Agent 里的可选范围与页面展示一致。

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage3_scope
git merge --ff-only sync_v0.8.3_stage3_scope_b_visibility
```

---

## 6. M3-C：组织树与成员管理

### 分支名

`sync_v0.8.3_stage3_scope_c_orgtree`

### 参考提交

1. `eef494451`
2. `442958421`
3. `ee6cbeade`

### 关键文件

1. `internal/application/service/org_tree.go`
2. `internal/handler/org_tree.go`
3. `frontend/src/stores/orgTree.ts`
4. `frontend/src/stores/auth.ts`
5. `frontend/src/views/admin/OrganManage.vue`
6. `frontend/src/components/OrgTreeSelector.vue`

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage3_scope_c_orgtree sync_v0.8.3_stage3_scope
git show --stat eef494451
git show --stat 442958421
git show --stat ee6cbeade
rg -n 'fetchTree\(|clearSessionResourceCaches|useOrgTreeStore\(\)\.clearState\(|CreateNode|ErrOrgTreeNodeNameDuplicate' internal/application/service internal/handler frontend/src/stores frontend/src/views/admin
```

### 执行重点

1. 组织树后端过滤语义要先稳定，再接前端树和成员页。
2. 全局重名校验作为 M3-C 尾部补丁并入，不要在前面 scope 未稳时提前落地。
3. 前端必须清理跨账号缓存残留，不能依赖刷新页面自愈。

### 验证命令

后端：

```bash
cd "$HOME/workspace/WeKnora"
go test ./internal/application/service -run 'TestHasDuplicateName|TestCreateNode'
```

前端：

```bash
cd "$HOME/workspace/WeKnora/frontend"
npm run type-check
```

### 手工 smoke

建议至少准备两类账号：

1. 超管/owner。
2. 子组织管理员或 contributor。

手工验证：

1. 先用高权限账号进入组织管理页，确认能看到全树。
2. 在不刷新浏览器的情况下登出，再用子组织管理员登录，确认只看到有权限的子树。
3. 在组织树中创建节点，确认全局重名会被拒绝，不重名会通过。

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage3_scope
git merge --ff-only sync_v0.8.3_stage3_scope_c_orgtree
```

---

## 7. M3-D：IM 渠道归属与隔离

### 分支名

`sync_v0.8.3_stage3_scope_d_im`

### 参考提交

1. `3d6cca084`
2. `6116ddcb9`
3. `a1c7efd86`

### 关键文件

1. `internal/im/service.go`
2. `internal/im/types.go`
3. `internal/handler/im.go`
4. `frontend/src/components/IMChannelPanel.vue`

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage3_scope_d_im sync_v0.8.3_stage3_scope
git show --stat 3d6cca084
git show --stat 6116ddcb9
git show --stat a1c7efd86
rg -n 'created_by = \?|CreatedBy|ListChannelsByAgent|ListChannelsByTenant|GetChannelByIDAndTenant|ToggleChannel|DeleteChannel' internal/im internal/handler
```

### 执行重点

1. `created_by` 归属必须先在后端查询条件和模型层稳定下来。
2. 然后再接列表、详情、切换启停、删除等 UI 行为。
3. 文件上传归属若依赖渠道创建者，也要在本批一起核对。

### 验证命令

后端：

```bash
cd "$HOME/workspace/WeKnora"
go test ./internal/im -run 'TestListChannelsByAgentFiltersByCreator|TestListChannelsByTenantFiltersByCreator|TestChannelCRUDRespectsCreatorScope'
```

前端：

```bash
cd "$HOME/workspace/WeKnora/frontend"
npm run type-check
```

### 手工 smoke

当前仓库没有现成的 IMChannelPanel 前端单测文件，建议用手工验证补齐：

1. 同租户两个不同创建者分别创建 IM 渠道。
2. 登录 A 账号时只能看到自己的渠道。
3. 登录 B 账号时不能操作 A 的渠道。
4. 内置 agent 与自定义 agent 绑定渠道都要各测一次。

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage3_scope
git merge --ff-only sync_v0.8.3_stage3_scope_d_im
```

---

## 8. M3 阶段验证命令

四个子分支都回并后，统一跑：

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage3_scope
go test ./internal/application/service -run 'TestSameTenantResourceAuthorizer|TestAgentKBScopeResolver|TestHasDuplicateName|TestCreateNode'
go test ./internal/im -run 'TestListChannelsByAgentFiltersByCreator|TestListChannelsByTenantFiltersByCreator|TestChannelCRUDRespectsCreatorScope'
cd "$HOME/workspace/WeKnora/frontend" && npm test -- src/views/agent/AgentEditorModal.test.mjs src/stores/editorResources.test.mjs
cd "$HOME/workspace/WeKnora/frontend" && npm run type-check
```

然后补一轮开发环境 smoke：

```bash
cd "$HOME/workspace/WeKnora"
make dev-start
make dev-app
make dev-frontend
```

M3 必查手工场景：

1. 同租户不同空间角色查看 KB/Agent 范围。
2. 组织树跨账号切换。
3. Agent 编辑器 contributor 视角。
4. IM 渠道创建者隔离。

## 8.1 M3 完成标准

1. same-tenant scope 收口完成。
2. KB/Agent 可见性和知识范围一致。
3. 组织树按角色展示正确且无跨账号残留。
4. IM 渠道按创建者隔离。

---

## 9. M4 执行清单：知识库目录、文档权限、移动目标、来源定位、批量下载

## 9.1 M4 主分支

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage4_kb sync_v0.8.3_stage3_scope
git status --short --branch
```

## 9.2 M4 建议拆分

建议拆成五个子分支：

1. `sync_v0.8.3_stage4_kb_a_batch_download`
2. `sync_v0.8.3_stage4_kb_b_source_locator`
3. `sync_v0.8.3_stage4_kb_c_folder_tree`
4. `sync_v0.8.3_stage4_kb_d_move_scope`
5. `sync_v0.8.3_stage4_kb_e_owner_uploader`

---

## 10. M4-A：批量下载

### 分支名

`sync_v0.8.3_stage4_kb_a_batch_download`

### 参考提交

1. `440fc66fa`
2. `eee72b9db`

### 关键文件

1. `internal/handler/knowledge.go`
2. `internal/handler/knowledge_batch_download_test.go`
3. `frontend/src/views/knowledge/KnowledgeBase.vue`
4. 知识列表相关批量操作组件

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage4_kb_a_batch_download sync_v0.8.3_stage4_kb
git show --stat 440fc66fa
git show --stat eee72b9db
rg -n 'BatchDownloadKnowledge|batch-download|downloadKnowledge' internal/handler frontend/src/views/knowledge frontend/src/views/knowledge/components
```

### 执行重点

1. 先复用上游 0.8.2 的批量 ZIP 能力。
2. 本地只补进度、按钮位置、文件名细节，避免保留两套后端实现。
3. 权限必须继续服从 0.8.3 现有下载语义。

### 验证命令

```bash
cd "$HOME/workspace/WeKnora"
go test ./internal/handler -run 'TestBatchDownloadKnowledge'
cd "$HOME/workspace/WeKnora/frontend" && npm run type-check
```

### 手工 smoke

1. 在 document KB 中多选多个文档执行批量下载。
2. 验证重复文档 ID 会被去重。
3. 验证无权限或跨 KB 文档不会静默混入 zip。

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage4_kb
git merge --ff-only sync_v0.8.3_stage4_kb_a_batch_download
```

---

## 11. M4-B：引用来源定位

### 分支名

`sync_v0.8.3_stage4_kb_b_source_locator`

### 参考提交

1. `5a881eebb`
2. 参考分析文档：`my_docs/20260924-01-聊天参考来源文档定位失效审查与修正方案.md`

### 关键文件

1. `frontend/src/components/ChatReferencesDrawer.vue`
2. `frontend/src/views/chat/components/docInfo.vue`
3. `frontend/src/views/knowledge/KnowledgeBase.vue`
4. 0.8.3 上游 source locator 对应 handler/service

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage4_kb_b_source_locator sync_v0.8.3_stage4_kb
git show --stat 5a881eebb
rg -n 'knowledge_id|folder_path|ChatReferencesDrawer|docInfo|tryAutoOpenDocument|source locator|citation' frontend/src internal
```

### 执行重点

1. 先验证上游 0.8.3 source locator 是否已经覆盖本地需求。
2. 如需补丁，只补 UI 流程和兼容兜底，不重造后端定位链路。
3. 旧的“根目录列表里找卡片”逻辑不能再作为主路径。

### 验证命令

当前私有线没有找到一条与 source locator 完全同构的现成本地自动化测试，因此这一批以：

1. 前端静态校验。
2. 开发环境手工 smoke。

为主：

```bash
cd "$HOME/workspace/WeKnora/frontend"
npm run type-check
```

开发环境：

```bash
cd "$HOME/workspace/WeKnora"
make dev-start
make dev-app
make dev-frontend
```

### 手工 smoke

建议使用一个“文档在子目录里”的 KB 做验证：

1. 上传或选一个位于子目录的 Office/PDF/Markdown 文档。
2. 在聊天中触发引用。
3. 点击“查看文档详情/来源”时，确认不会落到空白根目录视图。
4. 验证子目录文档、分页较深文档、大库文档都不会静默失败。

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage4_kb
git merge --ff-only sync_v0.8.3_stage4_kb_b_source_locator
```

---

## 12. M4-C：目录树与空目录

### 分支名

`sync_v0.8.3_stage4_kb_c_folder_tree`

### 参考提交

1. `52f3b7b82`
2. `e7d63659b`
3. `9045017e6`

### 关键文件

1. `internal/router/routes_knowledge.go`
2. `internal/application/service/knowledge.go`
3. `internal/application/repository/knowledge.go`
4. `internal/types/knowledge_folder.go`
5. `frontend/src/views/knowledge/KnowledgeBase.vue`
6. `frontend/src/views/knowledge/folderTree.ts`
7. `frontend/src/views/knowledge/components/KbFolderTree.vue`
8. `frontend/src/views/knowledge/components/FolderPickerMenu.vue`

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage4_kb_c_folder_tree sync_v0.8.3_stage4_kb
git show --stat 52f3b7b82
git show --stat e7d63659b
git show --stat 9045017e6
rg -n 'folder_path|ListKnowledgeFolders|MoveKnowledgeToFolder|RenameKnowledgeFolder|FolderPickerMenu|KbFolderTree' internal frontend/src/views/knowledge
```

### 执行重点

1. 明确目录树仍然是 `folder_path` 派生模型，不要误做独立目录表。
2. 空目录如果要保留，必须设计占位语义和过滤规则。
3. `FolderPickerMenu` 的 create/delete 入口必须接真实后端，不能保留纯前端假行为。
4. 中文目录/多字节目录名要一并验证路径截断安全。

### 验证命令

后端：

```bash
cd "$HOME/workspace/WeKnora"
go test ./internal/handler -run 'TestListKnowledge_FolderScopeFromQuery|TestListKnowledgeFolders_ReturnsTree'
go test ./internal/application/service -run 'TestMoveKnowledgeToFolder|TestRenameKnowledgeFolder'
go test ./internal/application/repository -run 'TestRenameKnowledgeFolderPath'
```

前端：

```bash
cd "$HOME/workspace/WeKnora/frontend"
npm test -- src/views/knowledge/folderTree.test.ts src/views/knowledge/components/KbFolderTree.test.ts
npm run type-check
```

### 手工 smoke

1. 目录重命名。
2. 文档移动到子目录。
3. 文档批量移回根目录。
4. 空目录创建/删除。
5. 中文目录名、深层目录名。

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage4_kb
git merge --ff-only sync_v0.8.3_stage4_kb_c_folder_tree
```

---

## 13. M4-D：移动目标范围与执行权限统一

### 分支名

`sync_v0.8.3_stage4_kb_d_move_scope`

### 参考提交

1. `14d918936`
2. `eb1002d46`

### 关键文件

1. `internal/handler/knowledgebase.go`
2. `internal/handler/knowledge_move_access.go`
3. `internal/handler/knowledge_move_gate_test.go`
4. `internal/handler/knowledgebase_move_targets_test.go`
5. `internal/application/service/kb_visibility.go`

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage4_kb_d_move_scope sync_v0.8.3_stage4_kb
git show --stat 14d918936
git show --stat eb1002d46
rg -n 'ListMoveTargets|move-targets|CanManageKB|MoveKnowledge|reuse_vectors|No permission to operate on this knowledge base' internal/handler internal/application/service
```

### 执行重点

1. `move-targets` 列表必须走同一套可见性服务。
2. 执行端必须与列表端使用同一 manage 语义。
3. API key allow-list 要被纳入目标列表与执行权限。
4. “看得见但不能移”和“能移但列表不显示”两类不一致都要消掉。

### 验证命令

```bash
cd "$HOME/workspace/WeKnora"
go test ./internal/handler -run 'TestListMoveTargets_|TestMoveKnowledge_CrossStoreGate|TestMoveKnowledge_RejectsTargetWithoutManagePermission'
```

### 手工 smoke

1. global/org/private 三类 KB 各准备一套。
2. Owner/Admin/Contributor 各自测试 move-targets 列表。
3. 再用受限 API key 测一次目标列表与执行是否一致。
4. 测试批量回根目录时目标选择和对号标记是否正确。

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage4_kb
git merge --ff-only sync_v0.8.3_stage4_kb_d_move_scope
```

---

## 14. M4-E：文档创建者归属与上传人展示

### 分支名

`sync_v0.8.3_stage4_kb_e_owner_uploader`

### 参考提交

1. `75ecb604b`
2. `905b451da`

### 关键文件

1. `internal/application/service/knowledge_creator.go`
2. `internal/application/service/knowledge_owner_chain_test.go`
3. `internal/types/knowledge.go`
4. `frontend/src/views/knowledge/components/DocumentListView.vue`
5. 相关 API/DTO 文件

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage4_kb_e_owner_uploader sync_v0.8.3_stage4_kb
git show --stat 75ecb604b
git show --stat 905b451da
rg -n 'CreatedBy|CreatedByNickname|columnUploader|GetOwningKBCreatorID|resource-auth' internal frontend/src/views/knowledge
```

### 执行重点

1. 文档创建者归属要走 `resource-auth` 身份链，不要继续依赖模糊上下文。
2. 上传人列显示要和后端归属链保持一致。
3. legacy KB/legacy knowledge 的空 creator 情况要兼容。

### 验证命令

后端：

```bash
cd "$HOME/workspace/WeKnora"
go test ./internal/application/service -run 'TestGetOwningKBCreatorID'
```

前端：

```bash
cd "$HOME/workspace/WeKnora/frontend"
npm run type-check
```

### 手工 smoke

1. 分别用 Web 上传、IM 上传、不同用户上传文档。
2. 查看知识库列表中的上传人是否与真实归属一致。
3. legacy 文档和新文档各抽一条核对显示。

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage4_kb
git merge --ff-only sync_v0.8.3_stage4_kb_e_owner_uploader
```

---

## 15. M4 阶段验证命令

五个子分支都回并后，统一跑：

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage4_kb
go test ./internal/handler -run 'TestBatchDownloadKnowledge|TestListKnowledge_FolderScopeFromQuery|TestListKnowledgeFolders_ReturnsTree|TestListMoveTargets_|TestMoveKnowledge_CrossStoreGate|TestMoveKnowledge_RejectsTargetWithoutManagePermission'
go test ./internal/application/service -run 'TestMoveKnowledgeToFolder|TestRenameKnowledgeFolder|TestGetOwningKBCreatorID'
go test ./internal/application/repository -run 'TestRenameKnowledgeFolderPath'
cd "$HOME/workspace/WeKnora/frontend" && npm test -- src/views/knowledge/folderTree.test.ts src/views/knowledge/components/KbFolderTree.test.ts
cd "$HOME/workspace/WeKnora/frontend" && npm run type-check
```

然后补一轮开发环境 smoke：

```bash
cd "$HOME/workspace/WeKnora"
make dev-start
make dev-app
make dev-frontend
```

M4 必查手工场景：

1. 批量下载。
2. 子目录文档来源定位。
3. 空目录增删。
4. 移动目标列表与执行权限一致。
5. 上传人列与归属链一致。

## 15.1 M4 完成标准

1. 批量下载复用上游主路径且可用。
2. 来源定位不再依赖旧根目录卡片查找逻辑。
3. 目录树、空目录、回根目录语义稳定。
4. move-targets 与执行权限统一。
5. 文档创建者归属和上传人展示恢复。

---

## 16. M3-M4 串行回并顺序

建议按下面顺序推进：

```text
sync_v0.8.3_stage3_scope
  -> sync_v0.8.3_stage3_scope_a_scope_core
  -> sync_v0.8.3_stage3_scope_b_visibility
  -> sync_v0.8.3_stage3_scope_c_orgtree
  -> sync_v0.8.3_stage3_scope_d_im

sync_v0.8.3_stage4_kb
  -> sync_v0.8.3_stage4_kb_a_batch_download
  -> sync_v0.8.3_stage4_kb_b_source_locator
  -> sync_v0.8.3_stage4_kb_c_folder_tree
  -> sync_v0.8.3_stage4_kb_d_move_scope
  -> sync_v0.8.3_stage4_kb_e_owner_uploader
```

强依赖约束：

1. M3-A 完成前不要开始 M3-B。
2. M3-B 完成前不要开始 M4-D。
3. M4-B 最好在 M3 完成后再做，否则来源定位与可见性联调会来回返工。

---

## 17. 一页式命令索引

如果只看命令，建议最短路径如下：

```bash
cd "$HOME/workspace/WeKnora"

git switch -c sync_v0.8.3_stage3_scope sync_v0.8.3_stage2_auth
git switch -c sync_v0.8.3_stage3_scope_a_scope_core
git switch sync_v0.8.3_stage3_scope && git merge --ff-only sync_v0.8.3_stage3_scope_a_scope_core
git switch -c sync_v0.8.3_stage3_scope_b_visibility
git switch sync_v0.8.3_stage3_scope && git merge --ff-only sync_v0.8.3_stage3_scope_b_visibility
git switch -c sync_v0.8.3_stage3_scope_c_orgtree
git switch sync_v0.8.3_stage3_scope && git merge --ff-only sync_v0.8.3_stage3_scope_c_orgtree
git switch -c sync_v0.8.3_stage3_scope_d_im
git switch sync_v0.8.3_stage3_scope && git merge --ff-only sync_v0.8.3_stage3_scope_d_im

git switch -c sync_v0.8.3_stage4_kb sync_v0.8.3_stage3_scope
git switch -c sync_v0.8.3_stage4_kb_a_batch_download
git switch sync_v0.8.3_stage4_kb && git merge --ff-only sync_v0.8.3_stage4_kb_a_batch_download
git switch -c sync_v0.8.3_stage4_kb_b_source_locator
git switch sync_v0.8.3_stage4_kb && git merge --ff-only sync_v0.8.3_stage4_kb_b_source_locator
git switch -c sync_v0.8.3_stage4_kb_c_folder_tree
git switch sync_v0.8.3_stage4_kb && git merge --ff-only sync_v0.8.3_stage4_kb_c_folder_tree
git switch -c sync_v0.8.3_stage4_kb_d_move_scope
git switch sync_v0.8.3_stage4_kb && git merge --ff-only sync_v0.8.3_stage4_kb_d_move_scope
git switch -c sync_v0.8.3_stage4_kb_e_owner_uploader
git switch sync_v0.8.3_stage4_kb && git merge --ff-only sync_v0.8.3_stage4_kb_e_owner_uploader
```

---

## 18. 结论

M3-M4 真正难的地方不在代码量，而在依赖顺序：

1. M3 要先把“可见性与归属规则”做稳。
2. M4 才能把目录、下载、移动、来源定位和上传人展示建立在稳定授权模型之上。

这份清单的核心价值，就是把 M3、M4 从“大块高风险改造”拆成可逐批验证的 9 个小执行单元。只要严格按分支与验证节奏推进，后续进入 M5、M6 时，知识与权限的底座就不会再反复重构。