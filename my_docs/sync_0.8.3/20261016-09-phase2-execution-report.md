# 20261016 Phase 2 执行报告

> 注意：本文件是第一次尝试的历史执行报告，只用于复盘依赖和踩坑。
> 再次同步时，文件级基线一律以上游 `v0.8.3` 为准，不得依据本报告整文件回迁 `container.go`、`router.go`、`routes_*.go` 等装配文件。

## 1. 执行目标

本次执行对应总计划中的 Phase 2 / M2，目标是：

1. 恢复“用户名或邮箱登录”能力。
2. 恢复空间初始化（bootstrap）向导链路。
3. 恢复创建门禁与会话缓存卫生能力。

## 2. 参考文档

本次执行主要参考以下文档：

1. [my_docs/sync_0.8.3/20261016-06-sync-master-execution-plan.md](my_docs/sync_0.8.3/20261016-06-sync-master-execution-plan.md)
2. [my_docs/sync_0.8.3/20261016-03-sync-M0-M2-checklist.md](my_docs/sync_0.8.3/20261016-03-sync-M0-M2-checklist.md)
3. [my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md](my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md)

## 3. 实际执行位置

本次 Phase 2 在独立基线 worktree 中继续执行：

1. worktree 路径：`/home/hlsa/workspace/WeKnora-sync_v0.8.3_base`
2. 执行分支：`sync_v0.8.3_stage2_auth`

该分支从 Phase 1 的 `sync_v0.8.3_stage1_platform` 切出。

## 4. 已完成内容

### 4.1 M2-A：登录标识由 email 切换为 identifier

已完成以下改动：

1. 后端 `types.LoginRequest` 从 `email` 改为 `identifier`。
2. `userService.Login` 改为先按用户名查找，再按邮箱兜底。
3. 登录失败文案改为 `Invalid username/email or password`。
4. `AuthHandler.Login` 的必填校验和日志记录改为基于 `identifier`。
5. SDK `client/auth.go` 的 `LoginRequest` 同步改为 `identifier`。
6. 前端登录 API `frontend/src/api/auth/index.ts` 同步改为 `identifier`。
7. 前端登录页 `frontend/src/views/auth/Login.vue` 改为“用户名/邮箱”输入框。
8. 注册成功后，登录表单自动回填用户名到 `identifier`。
9. 多语言文案已同步增加：
   - `identifier`
   - `identifierRequired`
   - `identifierPlaceholder`
10. 多语言登录错误文案已同步改为“用户名/邮箱或密码”。

本次涉及文件：

1. `internal/types/user.go`
2. `internal/application/service/user.go`
3. `internal/handler/auth.go`
4. `client/auth.go`
5. `internal/application/service/user_token_persistence_test.go`
6. `frontend/src/api/auth/index.ts`
7. `frontend/src/views/auth/Login.vue`
8. `frontend/src/i18n/locales/en-US.ts`
9. `frontend/src/i18n/locales/ko-KR.ts`
10. `frontend/src/i18n/locales/ru-RU.ts`
11. `frontend/src/i18n/locales/zh-CN.ts`
12. `frontend/src/i18n/locales/zh-TW.ts`
13. `frontend/src/i18n/locales/ja-JP.ts`

### 4.2 M2-C：Agent 创建门禁的可独立部分

在当前 0.8.3 基线上，M2-C 里可以独立落地的部分是“Agent 创建门禁共享入口收口”。

已完成：

1. 在 `frontend/src/views/agent/AgentList.vue` 新增 `ensureAgentCreateReady()`。
2. `openCreateModal()` 现在先经过 `ensureAgentCreateReady()`，不再允许外部入口绕过门禁。
3. `handleCreateAgent()` 改为只有在共享入口成功打开时才标记引导完成。
4. 新增源码级测试：`frontend/src/views/agent/AgentList.test.mjs`，锁定：
   - `openCreateModal()` 必须先走 `ensureAgentCreateReady()`
   - `handleCreateAgent()` 只在门禁通过后才写引导完成状态

## 5. 验证结果

### 5.1 M2-A 验证

已通过：

1. `go test ./internal/application/service -run 'TestLoginFailsWhenTokensCannotBePersisted|TestGenerateTokensFailsWhenEitherTokenCannotBePersisted|TestGenerateTokensPersistsPairBeforeReturning'`
2. `go test ./internal/handler`
3. `frontend: npm run type-check`

结论：

1. 登录链路的结构修改没有破坏后端服务层/handler 编译与关键测试。
2. 前端表单和多语言键改动已通过类型检查。

### 5.2 M2-C 验证

已通过：

1. `frontend: npm test -- src/views/agent/AgentList.test.mjs`
2. `frontend: npm run type-check`
3. `frontend: npm run build`

结论：

1. 共享入口门禁已被源码级测试锁定。
2. 前端类型检查通过。
3. 当前前端构建链在引入登录与门禁改动后仍可成功产出 `dist/`。

## 6. 当前阻塞点

### 6.1 M2-B：空间初始化与 bootstrap 未能在本轮落地

阻塞原因不是单个 bug，而是**当前上游 v0.8.3 基线里缺少整套 org-tree 子系统**。

已确认当前基线中不存在：

1. `internal/handler/org_tree.go`
2. `internal/handler/org_tree_provisioning.go`
3. `internal/application/service/org_tree.go`
4. `frontend/src/api/org-tree/`
5. `frontend/src/stores/orgTree.ts`
6. `frontend/src/components/WorkspaceSetupWizard.vue`
7. `frontend/src/utils/workspaceSetupRootOrg.ts`

而本地参考提交中的 M2-B 实际依赖链包含：

1. `tenant_bootstrap` 服务 / 仓储 / handler / 路由 / 迁移
2. `WorkspaceSetupWizard.vue`
3. `workspaceSetupRootOrg.ts`
4. `org_tree` handler / provisioning
5. 若干 `editorResources` / `organization` / `workspaceSetup` store 接线

也就是说：

1. 当前基线中，M2-B 不是“修几处已有逻辑”，而是要把一整套本地 org-tree / bootstrap 能力带回来。
2. 这部分已经明显越过了原本 M2 的“小入口层修复”边界，实际上开始依赖 M3 的部分基础设施。

### 6.2 M2-C：跨账号 orgTree cache 卫生也被同一问题阻塞

原计划中的“跨账号 orgTree 缓存残留修复”依赖：

1. `stores/orgTree.ts`
2. 组织树管理页
3. `clearSessionResourceCaches()` 对 orgTree store 的清理

当前基线没有这套 orgTree 前端存储和页面，因此这部分也无法按原计划直接执行。

## 7. 综合判断

本次 Phase 2 的状态应判定为：

**部分完成。**

完成部分：

1. M2-A 已落地并通过验证。
2. M2-C 中“Agent 创建门禁共享入口收口”已落地并通过验证。

未完成部分：

1. M2-B 空间初始化与 bootstrap。
2. M2-C 中依赖 orgTree store 的跨账号缓存卫生。

根因：

1. 当前上游 `v0.8.3` 基线不包含 org-tree 子系统。
2. 严格按本地原始计划顺序执行时，M2-B 和部分 M2-C 实际上已经依赖 M3 的底座。

## 8. 建议后续动作

有两个可行方向：

### 方向 A：保持当前总计划顺序，但修订依赖说明

做法：

1. 将 M2-B 标记为“等待 org-tree substrate 先回迁”。
2. 先进入 M3，优先回迁：
   - same-tenant scope core
   - org-tree handler/service/store 基础设施
3. 再回头完成 M2-B 的 WorkspaceSetupWizard 与 bootstrap 接线。

优点：

1. 更符合当前 0.8.3 基线的真实架构。

### 方向 B：把 org-tree substrate 视为 Phase 2.5，提前引入

做法：

1. 在 `sync_v0.8.3_stage2_auth` 上直接提前移植 M3 的 org-tree 底座文件。
2. 然后再完成 M2-B 的 bootstrap 与向导。

优点：

1. 能保持“Phase 2 一次性完成”的表面顺序。

缺点：

1. 实质上已经开始执行 M3，阶段边界会变模糊。

## 9. 结论

本次 Phase 2 已经真实推进到了两个明确结果：

1. 登录入口已完成 `identifier` 双路解析改造。
2. Agent 创建门禁已收口到共享入口并由测试锁定。

同时也得到一个明确的架构结论：

1. 在当前上游 `v0.8.3` 基线上，原计划中的 M2-B 和部分 M2-C 不能孤立执行。
2. 它们依赖一整套并不存在于当前基线的 org-tree / bootstrap substrate。

因此，后续继续执行时，应该先在计划层决定：

1. 是接受“Phase 2 部分完成，先转入 M3 substrate”，还是
2. 直接把 org-tree substrate 视为 Phase 2 的扩展范围提前引入。