# 20260820 SystemAdmin 与 SuperAdmin 合并性复核结论

## 0. 复核范围

- 复核对象：[my_docs/20260820-06-mdev权限实现复验与平台权限模型审查报告.md](my_docs/20260820-06-mdev权限实现复验与平台权限模型审查报告.md)
- 对照基线：当前工作区 `mdev`、`dev` 分支、`main` 分支
- 复核目标：回答两个问题
  1. `SystemAdmin` 和 `SuperAdmin` 在当前实现里是否重复
  2. 两者是否可以合并，以及应当按什么方向合并

---

## 1. 结论先行

### 1.1 是否重复

**部分重复，但并非完全重复。**

- 在“平台特权主线”上，两者大量复用同一权限语义：跨租户放行、KB/Agent 可见性 bypass、部分 tenant 删除放行，都已通过 `IsPlatformPrivilegedUser` 收敛。
- 但在“平台管理入口”和“组织树业务动作”上，两者当前仍有**真实、在线、生效中的差异**，并且至少存在 1 条需要“同时具备两个标志”才能通过的路径。

### 1.2 是否能够合并

**可以合并为单一平台角色，但不能在当前 `mdev/dev` 代码上直接等价替换；若直接删掉其中一个标志，会改变现有行为。**

更准确地说：

- **若以 `main` 为目标态**：可以，且推荐收敛到单一 `SystemAdmin` 角色。因为 `main` 分支只有 `SystemAdmin`，没有 `SuperAdmin`。
- **若以当前 `mdev/dev` 行为为准**：暂时不能直接合并。因为 `dev` 与当前工作区都还保留着真实的 `SuperAdmin` 专属/混合语义，必须先完成路由、handler、前端和数据迁移，才能删掉或折叠该角色。

---

## 2. 对 06 报告的复核结论

### 2.1 06 报告中成立的部分

06 报告关于以下判断仍然成立：

1. `main` 上游主推的平台权限是 `SystemAdmin`，`SuperAdmin` 不是上游原生模型。
2. 当前 `mdev/dev` 的资源权限主线已经把两者在大量场景下收敛到 `IsPlatformPrivilegedUser`。
3. `ctx` 里的 `SystemAdmin` 标志没有叠加 `IsSuperAdmin`，因此纯 `SuperAdmin` 用户在直接消费 `IsSystemAdminFromContext` 的代码路径上会被降级。

### 2.2 06 报告中需要修正的两处关键判断

#### 修正 1：06 报告称“全 router 中不存在仅当一个身份为 true 才放行的生效路由”是不成立的

06 报告在 [my_docs/20260820-06-mdev权限实现复验与平台权限模型审查报告.md](my_docs/20260820-06-mdev权限实现复验与平台权限模型审查报告.md#L173) 写道：

> 全 router 中不存在“仅当一个身份为 true 才放行”的生效路由。

这个结论与当前代码矛盾，至少有 3 类反例：

1. **SystemAdmin-only 路由组是真实生效的**
   - [internal/middleware/rbac.go](internal/middleware/rbac.go#L147-L170) 的 `RequireSystemAdmin` 只读取 `IsSystemAdminFromContext(ctx)`
   - [internal/router/routes_auth_tenant.go](internal/router/routes_auth_tenant.go#L251-L265) 的 `/system/admin/*` 全组挂在这个守卫下
   - [internal/router/router.go](internal/router/router.go#L282-L290) 还把 `RegisterOrgTreeSuperAdminRoutes` 放进了同一个 `SystemAdmin()` 守卫组

2. **纯 SuperAdmin 无法通过上述守卫**
   - [internal/middleware/auth.go](internal/middleware/auth.go#L104-L111) 与 [internal/middleware/auth.go](internal/middleware/auth.go#L257-L264) 写入 ctx 的是 `SystemAdmin: user.IsSystemAdmin`
   - 也就是说，纯 `SuperAdmin` 用户不会因为前端 `ToUserInfo` 的合并下发而自动拥有 `RequireSystemAdmin` 所需的 ctx 标志

3. **至少有一条 SuperAdmin-only 路径和一条“双标志交集”路径**
   - [internal/handler/org_tree.go](internal/handler/org_tree.go#L938-L949)：把组织节点移动到根层仅允许 `SuperAdmin`
   - [internal/router/router.go](internal/router/router.go#L361-L366) + [internal/handler/org_tree.go](internal/handler/org_tree.go#L1300-L1307)：组织内重置用户密码同时要求
     - 路由先通过 `SystemAdmin` 守卫
     - handler 内再要求 `currentUser.IsSuperAdmin`
   - 这条路径不是“二选一”，而是**需要同时满足两个角色语义**

因此，06 报告把两者描述成“运行时只剩一个角色、路由层无差异”是**过度收敛**。

#### 修正 2：06 报告称“SystemAdmin 无 API 降级，撤销必须改 DB”不成立

06 报告在 [my_docs/20260820-06-mdev权限实现复验与平台权限模型审查报告.md](my_docs/20260820-06-mdev权限实现复验与平台权限模型审查报告.md#L177) 把 `SystemAdmin` 描述成“只能 promote、无 API 降级（撤销必须改 DB）”。

这与当前代码、`dev`、`main` 三条线都不一致：

- 当前工作区已有 [internal/handler/system.go](internal/handler/system.go#L1376-L1426) `RevokeSystemAdmin`
- 当前路由已挂载 [internal/router/routes_auth_tenant.go](internal/router/routes_auth_tenant.go#L257-L265) 的 `/system/admin/revoke`
- `dev` 分支与 `main` 分支都存在对应的 revoke API 和前端调用

所以，`SystemAdmin` 与 `SuperAdmin` 的差异**不是“一个可撤销，一个不可撤销”**；真正差异在于：

- 哪些入口要求 `SystemAdmin`
- 哪些业务动作要求 `SuperAdmin`
- 某些动作当前甚至要求两者叠加

---

## 3. 当前代码里的真实角色关系

### 3.1 当前代码中“重叠”的部分

以下能力已经基本收敛，二者在这些场景里近似重复：

1. [internal/types/context_helpers.go](internal/types/context_helpers.go#L143-L156) 的 `IsPlatformPrivilegedUser` / `HasCrossTenantAccessCapability`
2. [internal/middleware/access.go](internal/middleware/access.go#L51-L67) 的跨租户超管放行
3. [internal/middleware/kb_access.go](internal/middleware/kb_access.go#L350-L380) 的 KB 自租户特权放行
4. Agent/KB 可见性 service 中的 super-admin bypass 参数链路
5. [internal/router/rbac.go](internal/router/rbac.go#L225-L237) 的 `OwnerOrSystemAdminDelete`

在这些地方，如果只看“资源访问”而不看“平台管理入口”，两者确实高度重复。

### 3.2 当前代码中“未重叠”的部分

以下行为仍然显式区分两者：

1. **SystemAdmin-only**
   - `/system/admin/*` 平台管理路由组
   - `/org-tree/super-admin`
   - `/org-tree/:id/users/:user_id/password` 的第一层路由守卫

2. **SuperAdmin-only**
   - 组织节点移动到根层：见 [internal/handler/org_tree.go](internal/handler/org_tree.go#L938-L949)

3. **SystemAdmin + SuperAdmin 交集**
   - 组织内重置成员密码：见 [internal/router/router.go](internal/router/router.go#L361-L366) 与 [internal/handler/org_tree.go](internal/handler/org_tree.go#L1300-L1307)

4. **遗留 fallback 代码仍直接读 `IsSuperAdmin`**
   - [internal/handler/custom_agent.go](internal/handler/custom_agent.go#L60-L70)
   - [internal/handler/knowledgebase.go](internal/handler/knowledgebase.go#L936-L944)

上面第 4 类大多是 fallback 或兼容分支，不是最强证据；真正决定“现在不能直接合并”的是前 3 类真实生效路径。

---

## 4. `dev` 与 `main` 对合并决策的影响

### 4.1 `main`：明确支持“最终合并”方向

本轮核查结果：`main` 分支只有 `SystemAdmin`，没有 `IsSuperAdmin`、没有 `RequireSuperAdmin`、没有 `SetSuperAdmin`、也没有 org-tree 超管业务面。

这说明：

- **从上游收敛方向看，单一平台管理员模型就是正确方向**
- 若团队目标是长期贴近 `main`，则 `SuperAdmin` 应被视作分叉期引入的过渡角色，而不应继续成为稳定双轨模型

### 4.2 `dev`：明确反对“立即直接合并”

本轮核查结果：`dev` 与当前工作区一样，仍然保留以下 `SuperAdmin` 业务面：

1. `SetSuperAdmin`
2. `MoveOrgNode` 的 root-level SuperAdmin 校验
3. `UpdateUserPasswordInOrg` 的 SuperAdmin 校验

因此：

- **如果以 `dev` 为验收基线，今天不能直接把 `SuperAdmin` 删掉**
- 否则会改变 `dev` 已有行为，不再是“权限对齐”，而是“权限模型重构”

---

## 5. 最终判断：能否合并

### 5.1 短答案

**能，但不是现在直接删字段；必须作为一轮显式迁移来做。**

### 5.2 具体判断

#### 结论 A：不能把 06 报告里的“几乎完全等价”直接翻译成“现在就能删掉一个角色”

原因很直接：当前代码里至少还存在

1. `SystemAdmin` only 路由
2. `SuperAdmin` only 业务动作
3. `SystemAdmin ∩ SuperAdmin` 的交集动作

这三类都在运行时生效。

#### 结论 B：如果决定合并，唯一合理的收敛方向是“并到 SystemAdmin”

原因：

1. `main` 只有 `SystemAdmin`
2. `SystemAdmin` 已经承载平台管理路由 `/system/admin/*`
3. 现有 `SuperAdmin` 业务面本质上是分叉期新增的 org-tree 管理扩展，不是上游共识模型

因此，不建议“把 SystemAdmin 并到 SuperAdmin”；若要合并，应当是：

- 保留 `SystemAdmin` 作为唯一平台管理员标志
- 将现有 `SuperAdmin` 业务语义迁移到 `SystemAdmin` 或迁移到更明确的组织树角色/能力位

#### 结论 C：若团队短期不做迁移，建议把两者重新命名为“平台管理员”与“组织树超管扩展”，避免继续把它们描述成等价角色

当前最容易造成误解的地方不是代码，而是语义命名：

- `SystemAdmin` 实际是平台管理入口的真实守卫
- `SuperAdmin` 在当前 `mdev/dev` 里既参与平台特权收敛，又保留组织树特化业务动作

如果暂不迁移，文档与评审里应明确写成“部分重叠，但未完成收敛”，而不是“已经等价”。

---

## 6. 建议的合并路线

### 6.1 若目标是跟齐 `main`

建议按以下顺序推进：

1. 先决策 root-level org move 和 org 用户密码重置，未来到底归 `SystemAdmin` 还是归 tenant/org 级能力
2. 将 [internal/middleware/auth.go](internal/middleware/auth.go#L104-L111) 与 [internal/middleware/auth.go](internal/middleware/auth.go#L257-L264) 的 ctx 写入语义与迁移策略统一
3. 把 `SetSuperAdmin` 改造成
   - `SetSystemAdmin`
   - 或一个更通用的平台角色授予接口
4. 前端去掉 `is_super_admin` 专属判断，统一收口到单一平台管理员字段
5. 完成数据回填后，再删除 `IsSuperAdmin` 字段及其死/兼容分支

### 6.2 若目标只是先维持 `dev` 兼容

则不应声称两者已经可合并；更准确的策略是：

1. 保留两个字段
2. 明确文档中哪些路径只认 `SystemAdmin`
3. 明确哪些路径只认 `SuperAdmin`
4. 明确哪些路径要求两者叠加

这能避免后续再出现“看上去合并了，实际运行时没合并”的误判。

---

## 7. 结论摘要

1. `SystemAdmin` 和 `SuperAdmin` **在资源权限主线里高度重复**。
2. 但它们**在当前 `mdev/dev` 里仍不是可直接互换的同义词**，因为仍有 SystemAdmin-only、SuperAdmin-only 和双标志交集三类生效路径。
3. **06 报告需要修正两处关键表述**：
   - 不是“全 router 无差异”
   - 也不是“SystemAdmin 无 API 撤销”
4. **若看长期方向，可以合并；若看当前代码，不能直接合并。**
5. **若决定合并，目标应当是向 `main` 收敛，保留 `SystemAdmin`，逐步清退 `SuperAdmin`。**

---

*复核时间：2026-08-20*
*复核基线：当前工作区 `mdev` + `dev` + `main`*
*前序文档：[my_docs/20260820-06-mdev权限实现复验与平台权限模型审查报告.md](my_docs/20260820-06-mdev权限实现复验与平台权限模型审查报告.md)*