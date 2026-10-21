# 20260820 SystemAdmin 合并方案与开发计划

## 0. 目标与结论

### 0.1 目标

基于当前 `mdev` 分支、`dev` 分支、`main` 分支与本地开发数据库现状，评估是否可以将 `SystemAdmin` 与 `SuperAdmin` 合并到单一 `SystemAdmin` 模型，并给出可执行的迁移方案与详细开发计划。

### 0.2 结论

**可以合并到 `SystemAdmin`，但不能直接一步切换；必须按“兼容过渡 -> 数据迁移 -> 路由/UI 切换 -> 清理字段”四阶段推进。**

原因如下：

1. `main` 分支只有 `SystemAdmin`，没有 `SuperAdmin`，长期收敛方向明确。
2. 当前 `mdev/dev` 的资源权限主线已经大量通过 `IsPlatformPrivilegedUser` 把两者合并使用。
3. 但当前 `mdev/dev` 仍保留真实生效中的 `SystemAdmin-only`、`SuperAdmin-only`、以及“路由要求 SystemAdmin、handler 再要求 SuperAdmin”的双重判定路径，不能直接删字段。
4. 当前本地 `weknora_mdev` 数据库中 **0 个 `SystemAdmin` 账号**、**1 个 `SuperAdmin` 账号**，若直接切到 `SystemAdmin` 单角色模型且不先迁移数据，会导致本地环境没有任何平台管理员。

### 0.3 当前本地数据库事实

查询对象：运行中的 `WeKnora-postgres-dev` 容器，数据库 `weknora_mdev`

#### 当前 `SystemAdmin` 账号

```sql
select id, username, email, is_system_admin, is_super_admin, can_access_all_tenants
from users
where is_system_admin = true
order by created_at desc, id asc;
```

结果：**0 条**

#### 当前 `SuperAdmin` 账号

```sql
select id, username, email, is_system_admin, is_super_admin, can_access_all_tenants
from users
where is_super_admin = true
order by created_at desc, id asc;
```

结果：

| id | username | email | is_system_admin | is_super_admin | can_access_all_tenants |
|---|---|---|---:|---:|---:|
| `63952d33-2c57-49fe-8af6-593674bdbcd7` | `admin` | `admin@hlsa.com` | false | true | true |

**结论**：当前本地环境里“平台管理员能力”实际落在 `SuperAdmin` 账号上，而不是 `SystemAdmin`。因此迁移的第一步必须先解决 bootstrap 问题。

---

## 1. 三条分支的模型对比

### 1.1 `main`

`main` 分支只有 `SystemAdmin`：

1. 存在 `/system/admin/*` 路由组
2. 存在 promote / revoke / list system admins
3. 不存在 `IsSuperAdmin`
4. 不存在 `SetSuperAdmin`
5. 不存在 `RequireSuperAdmin`
6. 不存在 org-tree 的 SuperAdmin 专属业务面

**结论**：若以主干收敛为目标，单一 `SystemAdmin` 模型是正确目标态。

### 1.2 `dev`

`dev` 仍保留 `SuperAdmin` 体系，且与当前 `mdev` 的关键行为一致：

1. 有 `SetSuperAdmin`
2. org-tree 根层移动仅允许 `SuperAdmin`
3. org-tree 用户密码重置路径仍保留 `SuperAdmin` 语义
4. KB/Agent 等资源权限主线已经通过 `IsPlatformPrivilegedUser` 合并使用

**结论**：若以 `dev` 为验收基线，当前不能直接把 `SuperAdmin` 删除；否则不再是“对齐”，而是“权限模型重构”。

### 1.3 当前 `mdev`

当前 `mdev` 与 `dev` 相同，属于“双轨但已部分收敛”的状态：

1. 资源访问主线：大量合并
2. 平台管理入口：仍以 `SystemAdmin` 为准
3. 组织树特化动作：仍有 `SuperAdmin` 专属逻辑
4. 运行时数据：当前本地环境没有任何 `SystemAdmin`

---

## 2. 当前代码中的真实差异面

### 2.1 已经可以视为“平台管理员统一语义”的部分

以下路径已经基本可以合并看待：

1. `internal/types/context_helpers.go`
   - `IsPlatformPrivilegedUser = IsSystemAdmin || IsSuperAdmin`
   - `HasCrossTenantAccessCapability = CanAccessAllTenants || IsPlatformPrivilegedUser`
2. `internal/middleware/access.go`
   - 跨租户访问短路
3. `internal/middleware/kb_access.go`
   - KB 自租户特权访问
4. `internal/application/service/kb_visibility.go`
   - super-admin bypass
5. `internal/application/service/agent_visibility.go`
   - super-admin bypass
6. `internal/router/rbac.go`
   - `OwnerOrSystemAdminDelete` 对 `IsSystemAdmin || IsSuperAdmin` 放行

这些路径说明：**从资源权限的角度，两个角色已经高度重叠。**

### 2.2 仍然不能直接合并的部分

#### A. SystemAdmin-only 路径

1. `internal/middleware/rbac.go`
   - `RequireSystemAdmin` 只读 `IsSystemAdminFromContext(ctx)`
2. `internal/router/routes_auth_tenant.go`
   - `/system/admin/promote`
   - `/system/admin/revoke`
   - `/system/admin/list`
3. `internal/router/router.go`
   - `RegisterOrgTreeSuperAdminRoutes` 被挂在 `rbacGuards.SystemAdmin()` 下面

#### B. SuperAdmin-only 路径

1. `internal/handler/org_tree.go`
   - `MoveOrgNode`：移动到 root level 仅允许 `SuperAdmin`
2. `internal/handler/custom_agent.go`
   - fallback 兼容逻辑仍直接认 `user.IsSuperAdmin`
3. `internal/handler/knowledgebase.go`
   - fallback 兼容逻辑仍直接认 `user.IsSuperAdmin`

#### C. SystemAdmin + SuperAdmin 双重要求路径

1. `internal/router/router.go`
   - `/org-tree/:id/users/:user_id/password` 被放在 `SystemAdmin()` 路由组
2. `internal/handler/org_tree.go`
   - `UpdateUserPasswordInOrg` 内部又要求 `currentUser.IsSuperAdmin`

这条路径在当前实现里不是“SystemAdmin 或 SuperAdmin”，而是接近“同时满足平台路由入口 + SuperAdmin 业务语义”的组合条件。

### 2.3 运行时标志不一致问题

`internal/middleware/auth.go` 写入 ctx 的是：

```go
SystemAdmin: user.IsSystemAdmin
```

而前端通过 `ToUserInfo()` 拿到的是：

```go
IsSystemAdmin: u.IsSystemAdmin || u.IsSuperAdmin
```

这意味着：

1. 前端 UI 可能把纯 `SuperAdmin` 视为“平台管理员”
2. 后端 `RequireSystemAdmin` 实际不会放行纯 `SuperAdmin`
3. 若直接删 `SuperAdmin` 而不先调整 ctx 写入、路由守卫与 UI 语义，会出现前后端行为断裂

---

## 3. 目标态设计

## 3.1 角色模型

目标态只保留一个平台管理员角色：`SystemAdmin`

### 角色定义

1. `User.IsSystemAdmin` = 唯一平台管理员标志
2. 删除 `User.IsSuperAdmin`
3. 删除前端 `is_super_admin`
4. 删除 `SetSuperAdmin`、`RequireSuperAdmin`、与其相关的 UI 文案

### 权限边界

平台级动作全部收口到 `SystemAdmin`：

1. `/system/admin/*`
2. 平台设置
3. 系统管理员名单维护
4. 跨租户访问
5. org-tree root-level move（若业务保留）
6. org-tree 用户密码重置（若业务保留）
7. 资源权限中的平台 bypass

### UI 语义

前端不再区分 `is_super_admin` 与 `is_system_admin`，统一使用：

1. `is_system_admin`
2. 或重命名为 `isPlatformAdmin` 的前端派生态

---

## 4. 合并策略

## 4.1 推荐策略：四阶段迁移

### Phase 0：决策冻结

在写代码前先确认两件业务决策：

1. org-tree 根层移动未来是否属于平台管理员权限
2. org-tree 用户密码重置未来是否属于平台管理员权限

建议：**都并入 `SystemAdmin`**，不要再额外保留 `SuperAdmin` 语义。

### Phase 1：兼容过渡层

目标：先让“纯 SuperAdmin 用户”在迁移窗口内能安全过渡到 `SystemAdmin` 语义，避免平台管理员失联。

#### 后端改动

1. `internal/middleware/auth.go`
   - 将 ctx 写入改为：
   ```go
   SystemAdmin: user.IsSystemAdmin || user.IsSuperAdmin
   ```
   目的：让纯 `SuperAdmin` 在 `RequireSystemAdmin` 路由下临时可用

2. `internal/middleware/rbac.go`
   - 保持 `RequireSystemAdmin` 不变，仍然只读 ctx
   - 通过 Phase 1 的 ctx 兼容完成过渡，而不是在守卫里继续引入新的双分支

3. `internal/handler/org_tree.go`
   - 将 `MoveOrgNode` 的 root-level 判断从 `IsSuperAdmin` 改为“平台管理员语义”
   - 将 `UpdateUserPasswordInOrg` 的 `IsSuperAdmin` 校验改为 `IsSystemAdminFromContext` 或平台管理员 helper
   - 暂时保留 `SetSuperAdmin` API，但进入只读兼容期，不再作为长期目标接口

4. `internal/handler/custom_agent.go`
   - fallback 分支中的 `user.IsSuperAdmin` 改成平台管理员 helper

5. `internal/handler/knowledgebase.go`
   - fallback 分支中的 `user.IsSuperAdmin` 改成平台管理员 helper

6. `internal/types/context_helpers.go`
   - 保留 `IsPlatformPrivilegedUser`，但把它明确标注为迁移期 helper

#### 前端改动

1. `frontend/src/stores/auth.ts`
   - 新增或保留统一派生态 `isPlatformAdmin`
   - 所有平台特权 UI 门控统一基于该派生态，而不是分别读两个字段

2. `frontend/src/api/auth/index.ts`
   - 迁移期继续兼容 `is_super_admin`
   - 但所有新页面、新逻辑只消费 `is_system_admin`

3. `frontend/src/views/admin/MemberManage.vue`
   - 标记 `set-super-admin` 操作为“待下线”
   - 增加提示：未来统一通过 SystemAdmin 管理

#### Phase 1 验收条件

1. 纯 `SuperAdmin` 账号在不改 DB 的情况下也能进入 `/system/admin/*`
2. org-tree root-level move 与 org 密码重置在迁移窗口内仍可正常使用
3. KB/Agent 平台 bypass 行为不回退

### Phase 2：数据迁移

目标：把现有 `SuperAdmin` 用户提升为 `SystemAdmin`，确保切换后平台管理账号不丢失。

#### 迁移前检查 SQL

```sql
select id, username, email, is_system_admin, is_super_admin, can_access_all_tenants
from users
where is_system_admin = true or is_super_admin = true
order by created_at desc, id asc;
```

#### 迁移 SQL

```sql
update users
set is_system_admin = true
where is_super_admin = true
  and is_system_admin = false;
```

#### 当前本地环境预期结果

执行后，至少会把：

- `admin / admin@hlsa.com / 63952d33-2c57-49fe-8af6-593674bdbcd7`

提升为 `SystemAdmin`。

#### 迁移后核验 SQL

```sql
select id, username, email, is_system_admin, is_super_admin, can_access_all_tenants
from users
where is_system_admin = true or is_super_admin = true
order by created_at desc, id asc;
```

要求：

1. 所有原 `SuperAdmin` 用户均已具备 `SystemAdmin`
2. 平台管理员总数大于 0
3. `/system/admin/list` 可返回这些用户

### Phase 3：切换读写面

目标：让新增和维护动作全部只操作 `SystemAdmin`。

#### 后端改动

1. 删除 `SetSuperAdmin` 的业务入口
2. 将 org-tree 的平台特权动作全部改为 `SystemAdmin`
3. `ListSystemAdmins` 成为唯一管理员名单接口
4. 所有 `IsSuperAdmin` 的 runtime 读路径改为：
   - `IsSystemAdmin`
   - 或统一 helper（最终 helper 只读 `IsSystemAdmin`）

#### 前端改动

1. 删除 `MemberManage.vue` 里的 super-admin 设定 UI
2. 若保留管理员管理页，则改用 system admin promote/revoke/list 接口
3. 删除 `is_super_admin` 文案与展示
4. 将 KB/Agent/平台入口门控统一基于 `is_system_admin`

#### 文档改动

1. 更新 swagger
2. 更新平台管理员说明文档
3. 更新测试场景文档

### Phase 4：清理模型与 schema

目标：彻底删除 `SuperAdmin`。

#### 代码删除项

1. `internal/middleware/super_admin.go`
2. `SetSuperAdmin`
3. `is_super_admin` DTO 字段
4. 前端 `is_super_admin` 消费路径
5. `IsPlatformPrivilegedUser` 中对 `IsSuperAdmin` 的兼容

#### 数据库迁移

待确认所有环境完成 Phase 2 和 Phase 3 后，再执行：

```sql
alter table users drop column is_super_admin;
```

若当前环境需要更保守，也可先保留列一段时间，仅停止读写。

---

## 5. 详细开发任务清单

## 5.1 后端任务

### 任务 B1：兼容期 ctx 写入改造

文件：

1. `internal/middleware/auth.go`
2. `internal/middleware/auth_context_test.go`

工作内容：

1. 将 `SystemAdmin` ctx 写入改成 `user.IsSystemAdmin || user.IsSuperAdmin`
2. 增加单测覆盖：
   - 纯 SystemAdmin
   - 纯 SuperAdmin
   - 两者都 false

完成标准：`RequireSystemAdmin` 能识别纯 SuperAdmin 账号。

### 任务 B2：组织树平台动作统一到 SystemAdmin

文件：

1. `internal/handler/org_tree.go`
2. `internal/router/router.go`
3. `internal/types/interfaces/org_tree.go`（若 helper 签名变化）

工作内容：

1. root-level move 从 `IsSuperAdmin` 改为平台管理员语义
2. `UpdateUserPasswordInOrg` 删除重复的 `IsSuperAdmin` 条件，改为和路由守卫一致
3. 评估 `SetSuperAdmin`：
   - 迁移期保留但标记 deprecated
   - 或直接改造为 `SetSystemAdmin`

完成标准：组织树平台动作不再要求 `SuperAdmin` 专属语义。

### 任务 B3：资源 handler fallback 清理

文件：

1. `internal/handler/custom_agent.go`
2. `internal/handler/knowledgebase.go`

工作内容：

1. 将所有 `user.IsSuperAdmin` fallback 改为平台管理员 helper
2. 确认 nil-service 回退不再依赖 `SuperAdmin` 字段

完成标准：资源权限 fallback 不再直接读 `IsSuperAdmin`。

### 任务 B4：系统管理员接口与文档统一

文件：

1. `internal/handler/system.go`
2. `internal/application/repository/user.go`
3. `internal/application/service/user.go`
4. `docs/swagger.yaml`
5. `docs/swagger.json`

工作内容：

1. 保持 promote / revoke / list 为唯一平台管理员维护入口
2. 更新文案，去掉和 `SuperAdmin` 并存的描述

完成标准：后端对外文档只暴露单一平台管理员概念。

### 任务 B5：最终模型清理

文件：

1. `internal/types/user.go`
2. `internal/types/context_helpers.go`
3. `internal/application/repository/org_tree.go`
4. `internal/middleware/super_admin.go`
5. 其他 grep `IsSuperAdmin` 命中的文件

工作内容：

1. 删除 `IsSuperAdmin` 字段
2. 删除 `RequireSuperAdmin`
3. 删除相关日志字段、DTO 和 repo 映射

完成标准：全仓 `git grep IsSuperAdmin` 只剩迁移脚本历史或文档说明。

## 5.2 前端任务

### 任务 F1：统一平台管理员状态

文件：

1. `frontend/src/api/auth/index.ts`
2. `frontend/src/stores/auth.ts`
3. `frontend/src/components/UserMenu.vue`

工作内容：

1. 迁移期保留兼容解析
2. UI 门控统一收口到 `is_system_admin` / `isPlatformAdmin`
3. 去掉 `is_super_admin` 专属判断

### 任务 F2：移除 SuperAdmin 管理 UI

文件：

1. `frontend/src/views/admin/MemberManage.vue`
2. `frontend/src/api/org-tree/index.ts`
3. `frontend/src/i18n/locales/*.ts`

工作内容：

1. 删除 `set-super-admin` 交互
2. 删除 `SetSuperAdmin` 相关 API
3. 删除文案

### 任务 F3：系统管理员管理入口成为唯一入口

文件：

1. `frontend/src/api/system/index.ts`
2. `frontend/src/views/system/*`
3. `frontend/src/views/settings/Settings.vue`

工作内容：

1. 用 promote / revoke / list system admins 页面承接管理员维护
2. 若需要展示 org-tree 人员信息，展示时只标识 `system_admin`

## 5.3 数据库任务

### 任务 D1：数据回填

1. 回填所有 `is_super_admin=true` 到 `is_system_admin=true`
2. 迁移前备份名单
3. 迁移后核验数量

### 任务 D2：schema 收敛

在代码确认不再读取 `is_super_admin` 后：

1. 新增 migration drop column
2. 验证回滚脚本

---

## 6. 详细发布计划

## 6.1 发布前检查

1. 统计所有环境的 `SystemAdmin` / `SuperAdmin` 数量
2. 列出所有仅 `is_super_admin=true` 的账号
3. 确认至少存在一个可执行 SQL 的运维入口
4. 备份 users 表相关字段

SQL：

```sql
select id, username, email, is_system_admin, is_super_admin, can_access_all_tenants
from users
where is_system_admin = true or is_super_admin = true
order by created_at desc, id asc;
```

## 6.2 发布步骤

### Release A：兼容版本

1. 上线 Phase 1 代码
2. 验证纯 SuperAdmin 账号可进入 `/system/admin/*`
3. 验证资源权限行为不变

### Release B：数据迁移

1. 执行 SQL 回填 `is_system_admin=true where is_super_admin=true`
2. 验证 `/system/admin/list` 可见迁移后的账号

### Release C：切换版本

1. 上线 Phase 3 代码
2. 关闭 SuperAdmin UI 入口
3. 组织树平台动作只认 SystemAdmin

### Release D：清理版本

1. 执行 drop column migration
2. 删除全部兼容代码

## 6.3 回滚方案

### 代码回滚

若 Release C 发现问题：

1. 回滚到 Release A/B 兼容版本
2. 保留数据层 `is_system_admin=true` 不回退

### 数据回滚

原则上**不要自动回退**把已提升的 `SystemAdmin` 再降回去，因为这可能导致平台管理员丢失。

若确需回退，只能基于迁移前备份名单人工恢复。

---

## 7. 验证计划

## 7.1 单元测试

至少覆盖：

1. 纯 `SystemAdmin`
2. 纯 `SuperAdmin`
3. 同时具备两个标志
4. 两个标志都没有

重点包：

1. `internal/middleware/...`
2. `internal/router/...`
3. `internal/handler/...`
4. `internal/application/service/...`

## 7.2 运行时场景

账号矩阵：

1. 纯 SystemAdmin
2. 纯 SuperAdmin
3. 迁移后的“双标志管理员”
4. space_admin
5. org_member

场景：

1. `/system/admin/list`
2. `/system/admin/promote`
3. `/system/admin/revoke`
4. org-tree root-level move
5. org-tree 用户密码重置
6. KB 创建/读/改
7. Agent 创建/读/改
8. 跨租户平台访问

## 7.3 本地环境必测点

由于本地 `weknora_mdev` 当前 **0 个 SystemAdmin、1 个 SuperAdmin**，本地环境必须额外验证：

1. Phase 1 前：`admin@hlsa.com` 无法通过纯 `SystemAdmin` 路由
2. Phase 1 后：`admin@hlsa.com` 可通过平台管理路由
3. Phase 2 后：`admin@hlsa.com` 变为真正的 `SystemAdmin`
4. Phase 3 后：删除 `SuperAdmin` UI 后，该账号仍具备全部平台管理能力

---

## 8. 实施建议

### 8.1 推荐决策

建议执行合并，并按以下原则推进：

1. **目标态明确收敛到 `SystemAdmin`**
2. **先解决本地和测试环境 0 SystemAdmin 的 bootstrap 问题**
3. **兼容期必须存在，不能一步删字段**
4. **先迁移行为，再迁移数据，最后迁移 schema**

### 8.2 当前最重要的三项动作

1. 先做 Phase 1：ctx 写入兼容 + org-tree 两条 `SuperAdmin` 专属路径改平台管理员语义
2. 立刻在各环境盘点 `is_super_admin=true` 的账号名单
3. 将这些账号批量补成 `is_system_admin=true`

---

## 9. 最终判断

### 当前 mdev 分支是否能够合并到 SystemAdmin

**能，但必须按迁移工程处理，不能作为一次普通权限对齐或简单字段替换。**

### 当前本地环境里的 SystemAdmin 账号是哪些

**当前本地 `weknora_mdev` 数据库中没有任何 `SystemAdmin` 账号。**

### 当前本地环境里的 SuperAdmin 账号是哪些

1. `admin` / `admin@hlsa.com`
2. `id = 63952d33-2c57-49fe-8af6-593674bdbcd7`

因此，若要启动合并，第一步不是删 `SuperAdmin`，而是：

1. 让该账号在兼容期内具备 `SystemAdmin` 路由能力
2. 将其数据回填为真正的 `SystemAdmin`
3. 再逐步移除 `SuperAdmin`

---

*文档生成时间：2026-08-20*
*基线：当前 `mdev` 工作区 + `dev` + `main` + 本地运行数据库 `weknora_mdev`*
*相关文档：*
- [my_docs/20260820-06-mdev权限实现复验与平台权限模型审查报告.md](my_docs/20260820-06-mdev权限实现复验与平台权限模型审查报告.md)
- [my_docs/20260820-07-SystemAdmin与SuperAdmin合并性复核结论.md](my_docs/20260820-07-SystemAdmin与SuperAdmin合并性复核结论.md)