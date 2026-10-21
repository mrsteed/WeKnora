# 20261016 WeKnora Phase 2 / M3 执行报告

## 1. 执行摘要

本轮执行对应总计划中修正后的 `Phase 2 = M3`，目标是在 `v0.8.3` 干净基线上恢复：

1. org-tree substrate
2. same-tenant scope core
3. KB/Agent visibility
4. org-tree handler/router/container/front-end substrate 最小接线

执行位置：

1. worktree：`/home/hlsa/workspace/WeKnora-sync_v0.8.3_base`
2. 当前分支：`sync_v0.8.3_base`

执行原则：

1. 上游已有文件一律在当前 `v0.8.3` 文件上做最小补丁，不做整文件覆盖。
2. 上游不存在的 substrate 文件，按本地私有线业务差异新增。
3. `internal/container/container.go`、`internal/router/router.go` 等装配文件仅做最小 provider / field / route 挂载补丁。

结论：本轮 M3 已达到当前阶段的完成态，可继续按修正后的顺序转入 `M2` 回填。

---

## 2. 本轮落地内容

### 2.1 后端 org-tree substrate

新增：

1. `internal/types/interfaces/org_tree.go`
2. `internal/types/org_tree.go`
3. `internal/application/repository/org_tree.go`
4. `internal/application/service/org_tree.go`
5. `internal/application/service/org_tree_duplicate_test.go`
6. `internal/handler/org_tree.go`
7. `internal/handler/org_tree_provisioning.go`
8. `internal/handler/tenant_role_authz.go`

配套模型增量：

1. `internal/types/organization.go`
   - 增加 `OrgRoleOwner` / `OrgRoleContributor`
   - `Organization` 增加 `parent_id/path/level/sort_order/org_tenant_id`
   - 增加 `OrganizationMember`
   - 增加 org-tree request DTO：`CreateOrgTreeNodeRequest`、`UpdateOrgTreeNodeRequest`、`MoveOrgNodeRequest`、`AssignUserToOrgRequest`、`RemoveUserFromOrgRequest`、`SetOrgAdminRequest`
2. `internal/types/user.go`
   - 增加 `Phone`
   - 增加 `IsSuperAdmin`
   - `UserInfo` / `ToUserInfo()` 同步支持上述字段

### 2.2 same-tenant scope / KB visibility / agent KB scope

新增：

1. `internal/types/interfaces/kb_visibility.go`
2. `internal/application/service/same_tenant_scope.go`
3. `internal/application/service/same_tenant_scope_test.go`
4. `internal/application/service/kb_visibility.go`
5. `internal/application/service/kb_creator_nickname.go`
6. `internal/application/service/agent_kb_scope.go`
7. `internal/application/service/agent_kb_scope_test.go`

现有文件最小补丁：

1. `internal/types/knowledgebase.go`
   - 新增 `KBVisibilityGlobal / Org / Private`
   - `KnowledgeBase` 增加 `visibility` 与 `organization_id`
2. `internal/types/interfaces/knowledgebase.go`
   - `KnowledgeBaseRepository` 新增 `ListAccessibleKBs()`
3. `internal/application/repository/knowledgebase.go`
   - 新增 `ListAccessibleKBs()` 的 same-tenant 可见性过滤实现

### 2.3 user provisioning 支撑

为了支撑 `org_tree_provisioning`，本轮补齐：

1. `internal/types/interfaces/user.go`
   - `GetUserByPhone`
   - `CreateUserByAdmin`
   - `AdminSetPassword`
2. `internal/application/repository/user.go`
   - `GetUserByPhone`
3. `internal/application/service/user.go`
   - admin provisioning 所需错误常量
   - `CreateUserByAdmin`
   - `GetUserByPhone`
   - `AdminSetPassword`
4. `internal/types/user.go`
   - org-tree provisioning 使用的 DTO：`CreateUserInOrgRequest`、`CreateUserInOrgResponse`、`UpdateUserInOrgRequest`、`UpdateUserPasswordInOrgRequest`

### 2.4 container / router 最小接线

最小补丁落在：

1. `internal/container/container.go`
   - 新增 `repository.NewOrgTreeRepository`
   - 新增 `service.NewOrgTreeService`
   - 新增 `service.NewKBVisibilityService`
   - 新增 `service.NewAgentKBScopeResolver`
   - 新增 `handler.NewOrgTreeHandler`
2. `internal/router/router.go`
   - `RouterParams` 新增 `KBVisibilityService`、`OrgTreeHandler` 等最小字段
   - 挂载 `RegisterOrgTreeRoutes()`
   - 挂载 `RegisterOrgTreeSuperAdminRoutes()`
   - 挂载 `GET /api/v1/my-organizations`

### 2.5 前端 substrate 与最小接线

新增：

1. `frontend/src/api/org-tree/index.ts`
2. `frontend/src/stores/orgTree.ts`
3. `frontend/src/components/OrgTreeSelector.vue`
4. `frontend/src/components/OrganizationSwitcher.vue`
5. `frontend/src/views/admin/AdminLayout.vue`
6. `frontend/src/views/admin/OrganManage.vue`
7. `frontend/src/views/admin/components/*`
8. `frontend/src/utils/errorMessage.ts`

现有文件最小补丁：

1. `frontend/src/stores/auth.ts`
2. `frontend/src/stores/organization.ts`
3. `frontend/src/router/index.ts`
4. `frontend/src/views/platform/index.vue`
5. `frontend/src/api/auth/index.ts`

其中：

1. `auth` typing 补回 `is_super_admin`
2. `orgTree` store 与 `OrganizationSwitcher` 接入当前认证状态
3. `OrganManage.vue` 及其子组件恢复 org-tree / 成员管理基础页面

---

## 3. 验证结果

### 3.1 后端服务窄测

已通过：

```bash
go test ./internal/application/service -run 'TestHasDuplicateName|TestCreateNode|TestSameTenantResourceAuthorizer|TestAgentKBScopeResolver'
```

### 3.2 handler / router / container 编译验证

已通过：

```bash
go test ./internal/handler -run '^$'
go test ./internal/router -run '^$'
go test ./internal/container -run '^$'
```

说明：

1. 这里使用 `-run '^$'` 只做编译层验证，不跑整包测试。
2. 这样可以验证当前 M3 接线是否完整，同时避免扩大到与本轮无关的慢测。

### 3.3 前端验证

已通过：

```bash
cd frontend
npm run type-check
npm run build
```

本轮前端收尾时实际修过两个局部阻塞：

1. `src/api/org-tree/index.ts` 依赖的 `src/utils/errorMessage.ts` 缺失，已补回。
2. `OrganManage.vue` 初次构建时报相对路径缺少 `./components/*`，后已把 `frontend/src/views/admin/components/` 补齐，最终 build 通过。

### 3.4 diff 检查

本轮文本改动已通过：

```bash
git diff --check
```

---

## 4. 未并入本轮的内容

本轮没有把以下能力混入 M3：

1. `M2-B` 的 `WorkspaceSetupWizard` / `tenant_bootstrap` / root org reuse
2. `M2-C` 依赖 orgTree store 的跨账号缓存卫生回填

原因：

1. 这些属于修正后顺序里的 `M3 完成后 -> 回头补 M2`，不应在本轮混做。

---

## 5. 结论

本轮 M3 已完成当前阶段应承担的工作：

1. org-tree substrate 已恢复。
2. same-tenant KB visibility 已恢复。
3. agent KB scope resolver 已恢复。
4. org-tree handler/router/container 接线已恢复。
5. 前端 org-tree substrate 已恢复，并已通过 `type-check` 与 `build`。

因此后续应按修正后的总计划继续：

1. 回头执行 `M2-B` / 余下 `M2-C` 回填
2. 再继续 M4 及后续阶段