# 20261016 M3 substrate 先行执行报告

> 注意：本文件是第一次尝试的历史执行报告，只用于复盘依赖和踩坑。
> 再次同步时，文件级基线一律以上游 `v0.8.3` 为准，不得依据本报告整文件回迁 `container.go`、`router.go`、`routes_*.go` 等装配文件。

## 1. 执行背景

本报告记录 2026-10-16 对执行计划做出的实际依赖修正，以及随后启动的 **M3 substrate 先行批次**。

触发原因：

1. 在 Phase 2 执行过程中，已确认 `M2-B`（bootstrap / WorkspaceSetupWizard / 根组织复用）并不是一个独立的小入口补丁。
2. 它依赖一整套当前上游 `v0.8.3` 基线中不存在的 org-tree substrate。
3. 因此后续执行顺序被修正为：
   - 先完成 `M2-A` 与可独立完成的 `M2-C`
   - **转入 M3，先带回 org-tree substrate**
   - 待 substrate 稳定后，再回头补完 `M2-B`

## 2. 参考文档

本次执行主要参考：

1. [my_docs/sync_0.8.3/20261016-06-sync-master-execution-plan.md](my_docs/sync_0.8.3/20261016-06-sync-master-execution-plan.md)
2. [my_docs/sync_0.8.3/20261016-04-sync-M3-M4-checklist.md](my_docs/sync_0.8.3/20261016-04-sync-M3-M4-checklist.md)
3. [my_docs/sync_0.8.3/20261016-09-phase2-execution-report.md](my_docs/sync_0.8.3/20261016-09-phase2-execution-report.md)

## 3. 分支与执行位置

当前执行位置：

1. worktree：`/home/hlsa/workspace/WeKnora-sync_v0.8.3_base`
2. 当前分支：`sync_v0.8.3_stage3_scope`

该分支从 `sync_v0.8.3_stage2_auth` 切出，用于承接 M3 substrate 的先行恢复。

## 4. 已实际恢复的 substrate 文件

### 4.1 后端 org-tree 基础文件

已带回：

1. `internal/types/interfaces/org_tree.go`
2. `internal/types/org_tree.go`
3. `internal/application/repository/org_tree.go`
4. `internal/application/service/org_tree.go`
5. `internal/handler/org_tree.go`
6. `internal/handler/org_tree_provisioning.go`

### 4.2 前端 org-tree 基础文件

已带回：

1. `frontend/src/api/org-tree/index.ts`
2. `frontend/src/stores/orgTree.ts`
3. `frontend/src/components/OrgTreeSelector.vue`
4. `frontend/src/components/OrganizationSwitcher.vue`
5. `frontend/src/views/admin/AdminLayout.vue`
6. `frontend/src/views/admin/OrganManage.vue`
7. `frontend/src/views/admin/components/AssignOrgDialog.vue`
8. `frontend/src/views/admin/components/CreateUserDialog.vue`
9. `frontend/src/views/admin/components/EditUserDialog.vue`
10. `frontend/src/views/admin/components/MemberTable.vue`
11. `frontend/src/views/admin/components/OrgTreeEditor.vue`
12. `frontend/src/views/admin/components/OrgTreeNodeItem.vue`
13. `frontend/src/views/admin/components/ResetUserPasswordDialog.vue`

### 4.3 必要接线文件

已同步带回或对齐：

1. `internal/container/container.go`
2. `internal/router/router.go`
3. `internal/handler/knowledgebase.go`
4. `internal/handler/knowledgebase_pr3_response_test.go`
5. `frontend/src/stores/organization.ts`
6. `frontend/src/stores/auth.ts`
7. `frontend/src/router/index.ts`
8. `frontend/src/views/platform/index.vue`
9. `frontend/src/views/agent/AgentEditorModal.vue`
10. `frontend/src/views/knowledge/KnowledgeBaseEditorModal.vue`

### 4.4 附带修补的兼容文件

为了让 substrate 能进入编译阶段，已额外补齐：

1. `frontend/src/utils/errorMessage.ts`
2. `frontend/src/utils/errorMessage.test.ts`
3. `frontend/src/stores/editorResources.ts`
4. `frontend/src/stores/editorResources.test.mjs`
5. `internal/application/service/agent_kb_scope.go`
6. `internal/application/service/agent_kb_scope_test.go`
7. `internal/application/service/kb_visibility.go`
8. `internal/application/service/kb_creator_nickname.go`
9. `internal/application/service/same_tenant_scope.go`
10. `internal/application/service/same_tenant_scope_test.go`
11. `internal/types/interfaces/kb_visibility.go`
12. `internal/types/interfaces/knowledgebase.go`
13. `internal/types/organization.go`
14. `internal/types/knowledgebase.go`
15. `internal/types/user.go`
16. `frontend/src/stores/ui.ts`

## 5. 已完成的兼容修复

为让 substrate 适配当前 0.8.3 主骨架，已经做了以下最小兼容修复：

1. `frontend/src/stores/auth.ts`
   - 增加 `ensureAuthMe()`，复用 `refreshFromAuthMe()`
   - 将 `isSuperAdmin` 收口到当前可用的 `is_system_admin`

2. `frontend/src/stores/ui.ts`
   - 补回 `kbEditorInitialVisibility`
   - 补回 `kbEditorInitialOrganizationId`
   - 补回 `sidebarWidth / sidebarResizing / sidebarBrowserStatus / sidebarDisplayWidth`
   - 补回 sidebar 相关 setter / resize 行为

3. `frontend/src/stores/editorResources.ts`
   - 补齐 `AgentScope` 导入
   - 让 `ensureSkills()` 接受当前页面已经在传的参数形态

4. `frontend/src/api/knowledge-base/index.ts`
   - `updateKnowledgeBase()` 请求类型补充 `visibility` / `organization_id`

5. `internal/types/user.go`
   - 补回 `Phone`
   - 补回 `IsSuperAdmin`
   - 补回 `CreateUserInOrgRequest` / `UpdateUserInOrgRequest` 等 DTO

6. `internal/types/interfaces/knowledgebase.go`
   - 补回 `ReadChunkImage` 到 `KnowledgeBaseService`

7. `internal/types/knowledgebase.go`
   - 开始合并 `visibility / organization_id` 与 `ProfileConfig / DefaultParserEngine` 相关字段和 helper

## 6. 当前验证结果

### 6.1 已通过

已确认通过：

1. 前端 `npm run type-check` 已恢复通过。

这说明：

1. org-tree substrate 的前端底座与当前 0.8.3 页面/组件骨架已基本接住。
2. 后续若继续推进，前端不再是当前最先阻塞点。

### 6.2 尚未通过

当前仍未通过的验证是：

1. `go test ./internal/application/service -run 'TestHasDuplicateName|TestCreateNode|TestSameTenantResourceAuthorizer|TestAgentKBScopeResolver'`
2. `go test ./internal/handler -run 'TestCreateKnowledgeBaseResponseOrgVisibility|TestListMoveTargets_'`

最新阻塞点已收敛到：

1. `internal/types/knowledgebase.go` 与当前 0.8.3 模型仍存在漂移
2. 主要是同一个文件里需要继续把“org-tree 可见性语义”和“0.8.3 原生 KnowledgeBase 字段/方法”完全合并

换句话说：

1. 前端 substrate 已基本站住
2. 后端 substrate 已从“缺文件”推进到“类型合并收尾”阶段

## 7. 当前判断

这次转入 M3 substrate 的动作已经证明方向是对的：

1. `M2-B` 的确依赖一整套 org-tree substrate，之前的依赖判断是正确的。
2. 直接进入 M3 先恢复 substrate，是当前基线下正确的执行路径。
3. substrate 恢复已经完成了“文件带回”和“前端站稳”两步。
4. 剩余工作集中在后端类型/接口与当前 0.8.3 模型的合并，而不是方向错误。

## 8. 建议的下一步

建议继续执行 M3 substrate 收尾，而不是回头碰 M2-B：

1. 继续收敛 `internal/types/knowledgebase.go` 与相关仓储/接口的模型漂移。
2. 让 `M3-S0 org-tree substrate` 先达到“后端窄测试可编译/可运行”的完成标准。
3. 然后再正式进入 `M3-A same-tenant scope core`。
4. 待 `M3-A/M3-B/M3-C` 稳定后，再回头补完 `M2-B` 的 `WorkspaceSetupWizard` / `tenant_bootstrap` 接线。

## 9. 结论

本次“按实际依赖修计划，先转入 M3”已经完成了两件关键事：

1. 计划层已经被正式修正为：`M2-A / 局部 M2-C -> M3 substrate -> M2-B 回填 -> M3 主线继续`。
2. 代码层已经实际切入 `sync_v0.8.3_stage3_scope`，并把 org-tree substrate 带回到了“前端已过 type-check、后端只剩模型合并收尾”的状态。

这意味着后续继续推进时，不需要再争论顺序，直接沿着这条修正后的路线走即可。