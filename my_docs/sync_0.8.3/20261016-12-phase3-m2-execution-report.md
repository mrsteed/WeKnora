# 20261016 WeKnora Phase 3 / M2 执行报告

## 1. 执行摘要

本轮执行对应修正后总计划中的 `Phase 3 / M2`，执行前置条件为：

1. M0 已完成
2. M1 已完成
3. M3 substrate 已完成并通过后端 service/handler/router/container 窄验证与前端 `type-check/build`

本轮目标：

1. 回填 `M2-B`：workspace bootstrap / root org reuse / `WorkspaceSetupWizard`
2. 回填 `M2-C` 余下部分：Agent 创建共享入口门禁收口、跨账号 session cache 卫生

结论：当前 worktree 上的 M2 已完成，并已通过本轮计划内的非破坏性验证。

---

## 2. 本轮落地内容

### 2.1 后端 bootstrap substrate

新增文件：

1. `internal/types/interfaces/tenant_bootstrap.go`
2. `internal/types/tenant_bootstrap.go`
3. `internal/application/repository/tenant_bootstrap.go`
4. `internal/application/service/tenant_bootstrap.go`
5. `internal/application/service/tenant_bootstrap_test.go`
6. `internal/handler/tenant_bootstrap.go`

新增迁移：

1. `migrations/versioned/000092_tenant_bootstrap_operations.up.sql`
2. `migrations/versioned/000092_tenant_bootstrap_operations.down.sql`
3. `migrations/sqlite/000005_tenant_bootstrap_operations.up.sql`
4. `migrations/sqlite/000005_tenant_bootstrap_operations.down.sql`

接线补丁：

1. `internal/handler/tenant.go`
   - `TenantHandler` 增加 `invitationService` / `orgTreeService` / `bootstrapService`
   - `NewTenantHandler` 构造器同步补参
2. `internal/router/routes_auth_tenant.go`
   - 增加 `POST /api/v1/tenants/:id/bootstrap`
3. `internal/container/container.go`
   - 增加 `repository.NewTenantBootstrapRepository`
   - 增加 `service.NewTenantBootstrapService`

### 2.2 前端 workspace setup 链路

新增文件：

1. `frontend/src/stores/workspaceSetup.ts`
2. `frontend/src/components/WorkspaceSetupWizard.vue`
3. `frontend/src/utils/workspaceSetupRootOrg.ts`
4. `frontend/src/utils/workspaceSetupRootOrg.test.ts`

现有文件最小补丁：

1. `frontend/src/api/tenant/index.ts`
   - 增加 `WorkspaceBootstrap*` 类型定义
   - 增加 `bootstrapWorkspace()`
2. `frontend/src/components/UserMenu.vue`
   - 创建空间成功后调用 `workspaceSetupStore.start(newTenant)`
3. `frontend/src/App.vue`
   - 全局挂载 `WorkspaceSetupWizard`
   - `onMounted()` 时执行 `workspaceSetupStore.hydratePending()`

功能行为：

1. 创建空间后，pending bootstrap 状态写入 `sessionStorage`
2. 页面刷新后可恢复向导状态
3. 根组织支持复用，不再盲目重复创建

### 2.3 M2-C：共享入口门禁与缓存卫生

最小补丁：

1. `frontend/src/views/agent/AgentList.vue`
   - 新增 `ensureAgentCreateReady()`
   - `openCreateModal()` 变为统一门禁入口，先过 `ensureAgentCreateReady()`
   - `handleCreateAgent()` 只有在 `openCreateModal()` 成功后才 `markContextualGuideDone('agentList')`
   - `handleOpenAgentEditor()` 通过 shared opener 进入创建，不再绕过门禁
2. `frontend/src/views/agent/AgentList.test.mjs`
   - 源码级锁定 shared opener 必须先过门禁
   - 锁定 guide done 标记只在门禁通过后写入
3. `frontend/src/stores/auth.ts`
   - `clearSessionResourceCaches()` 现在同时清理 `orgTree` 与 `workspaceSetup`，避免跨账号 SPA 登录遗留组织树或 pending wizard 状态

### 2.4 运行时兼容补丁

新增：

1. `frontend/src/utils/errorMessage.ts`

现有文件补丁：

1. `frontend/src/api/auth/index.ts`
   - `LoginResponse.user` / `UserInfo` / `userInfoFromApi()` 补上 `is_super_admin`
2. `frontend/src/i18n/locales/zh-CN.ts`
3. `frontend/src/i18n/locales/en-US.ts`
   - 增加 `tenant.setupWizard.*` 文案块，避免 Wizard 运行时直接显示 i18n key

---

## 3. 验证结果

### 3.1 后端

已通过：

```bash
go test ./internal/application/service -run 'TestTenantBootstrapServiceGetOrCreateIsIdempotent'
go test ./internal/application/service -run 'TestHasDuplicateName|TestCreateNode|TestSameTenantResourceAuthorizer|TestAgentKBScopeResolver'
go test ./internal/handler -run '^$'
go test ./internal/router -run '^$'
go test ./internal/container -run '^$'
```

说明：

1. `tenant_bootstrap` 幂等持久化已通过其自身窄测。
2. M2 回填后，M3 的后端 service 窄测没有回归。
3. `handler/router/container` 的编译验证通过，说明本轮新增的 bootstrap 接线没有打坏现有装配。

### 3.2 前端

已通过：

```bash
cd frontend
npm test -- src/views/agent/AgentList.test.mjs
npm test -- src/utils/workspaceSetupRootOrg.test.ts
npm run type-check
npm run build
```

说明：

1. `WorkspaceSetupWizard` 与 `workspaceSetupRootOrg` 已通过 focused test + `type-check`。
2. `AgentList` shared opener 门禁已通过源码级测试锁定。
3. 前端生产构建通过，说明 M2 回填未破坏当前 M0/M1/M3 的前端基线。

### 3.3 diff 检查

已通过：

```bash
git diff --check
```

---

## 4. 结论

M2 的完成标准在当前执行线已满足：

1. `identifier` 登录已在此前 M2-A 完成。
2. 初始化向导已恢复，支持：
   - 已有账号 / 新建用户 / 邀请链接 三种模式
   - root org 复用
   - bootstrap 幂等恢复
3. Agent 创建共享入口门禁已统一收口。
4. 跨账号 session cache 卫生已补到 `orgTree + workspaceSetup`。

因此，修正后顺序中的 `M2` 已在当前 worktree 完成，后续可进入 `M4`。