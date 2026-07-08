# WeKnora 权限角色与 API 边界统一说明

> 状态：当前权威说明，覆盖租户 RBAC、组织树作用域、协作空间共享，以及跨租户超级管理员边界。

本文档用于统一以下四个容易混淆的概念：

- 空间管理员
- 部门管理员
- 协作空间管理员
- 超级管理员

并明确 API 到底是看 `tenant role`、`org-tree role`、`collaboration role`，还是 `system / cross-tenant capability`。

## 1. 术语统一

| 产品术语 | 代码来源 | 作用域 | 关键能力 |
| --- | --- | --- | --- |
| 超级管理员 | `users.is_system_admin` + `users.can_access_all_tenants`（兼容 `is_super_admin`） | 平台级 + 跨空间 | 系统设置、跨空间运营、跨空间切换、跨空间成员/模型/组织树管理 |
| 空间管理员 | `tenant_members.role in (admin, owner)` | 单个租户 / 工作空间 | 管理该空间成员、模型、向量库、MCP、数据源、Agent/KB 顶层资源 |
| 部门管理员 | 组织树成员 `organization_members_pre_plan3.role=admin` 或 `OrgTreeNode.MyIsAdmin=true` | 单个租户内的组织树子树 | 管理本部门及其子树作用域内的 KB / Agent 等同租户资源 |
| 协作空间管理员 | `organization_tenant_members.role=admin` | 跨租户协作空间（Organization） | 管理协作空间成员、邀请、共享关系；不天然拥有目标租户的模型/系统设置权限 |

### 术语边界

- “空间管理员”解决的是**租户内基础设施与成员管理**。
- “部门管理员”解决的是**同租户资源在组织树上的作用域与管理半径**。
- “协作空间管理员”解决的是**跨租户共享关系维护**。
- “超级管理员”解决的是**平台级和跨空间运营能力**。

这四者不是同一维度，不能混用，也不能互相替代。

## 2. 资源判定维度

当前实现把权限拆成三层正交决策：

1. 租户 RBAC：`viewer / contributor / admin / owner`
2. 同租户组织树作用域：读范围、管理范围由组织树决定
3. 跨租户协作空间共享：`organization_tenant_members` + `kb_shares / agent_shares`

### 2.1 同租户资源作用域（当前代码）

当前同租户 KB / Agent 已统一到 `sameTenantResourceAuthorizer`：

- 读范围：本人所在组织 + 祖先组织 + 后代组织
- 管理范围：本人担任 admin 的组织 + 这些 admin 组织的后代组织
- `private`：仅创建者可读 / 可管
- `global`：同租户可读；非创建者不可管（除 tenant admin / super admin）
- `org`：由组织树范围决定

DataSource 当前不单独建 visibility，而是继承其绑定 KB 的访问结论。

> 当前代码库中**不存在 workflow 模块实现**，因此没有可接入的 workflow handler / service / route。后续如新增 workflow 资源，应直接复用 `sameTenantResourceAuthorizer` 与 `CanManage* / CanAccess*` 模式，不再新建第四套权限逻辑。

## 3. API 边界矩阵

### 3.1 租户 / 空间级接口

| 接口族 | 主要路径 | 主要判断维度 | 说明 |
| --- | --- | --- | --- |
| 空间信息 / 成员 | `/tenants/:id/*` | `tenant role` + `PathTenantMatch` | 正常成员按 tenant role；跨空间超管可越过本地 `tenant_members` |
| 系统级租户搜索/切换 | `/tenants/all`, `/tenants/search` | `cross-tenant superuser capability` | 仅超级管理员 / 跨空间运营能力 |
| 系统设置 | `/system/admin/*` | `system admin` | 不看 tenant role |
| 模型 / 向量库 / MCP / 网络搜索 | `/models`, `/vector-stores`, `/mcp-services`, `/web-search-providers` | `tenant role >= admin` | 空间基础设施能力 |

### 3.2 同租户资源接口

| 资源 | 读接口 | 写接口 | 实际判断 |
| --- | --- | --- | --- |
| Knowledge Base | `/knowledge-bases/:id` | `/knowledge-bases/:id` | `KBAccess` + `CanAccessKB / CanManageKB` + creator fallback |
| Knowledge 子资源 | `/knowledge/:id`, `/chunks/*`, `/wiki/*` | 同路径下写接口 | 与所属 KB 完全同构，统一继承 `CanManageKB` |
| FAQ / Tag | `/knowledge-bases/:id/faq/*`, `/knowledge-bases/:id/tags/*` | 同路径下写接口 | 与顶层 KB 同构 |
| Agent | `/agents/:id` | `/agents/:id` | `AgentVisibilityService` + creator fallback |
| DataSource | `/data-sources/*` | `/data-sources/*` | 通过绑定 KB 的 `CanAccessKB / CanManageKB` 继承权限 |

### 3.3 组织树接口

| 接口族 | 主要路径 | 主要判断维度 | 说明 |
| --- | --- | --- | --- |
| 组织树浏览 / 读 | `/org-tree`, `/org-tree/:id`, `/my-organizations` | `org-tree role` 或 bootstrap 特例 | 新空间空树时，tenant admin/owner 可 bootstrap 第一个根组织 |
| 组织树写 | `/org-tree/*` 写接口 | `org-tree admin` / `super admin` | 根组织首次 bootstrap 后，由组织树 admin 链继续管理 |

### 3.4 协作空间（Organization）接口

| 接口族 | 主要路径 | 主要判断维度 | 说明 |
| --- | --- | --- | --- |
| 协作空间成员 / 邀请 / join request | `/organizations/:id/*` | `collaboration role` | 这是协作空间角色，不等于 tenant admin |
| KB / Agent 共享 | `/knowledge-bases/:id/shares`, `/agents/:id/shares` | 源资源管理权限 + `collaboration role` | 共享动作需要先有源资源管理权 |
| Shared list | `/shared-knowledge-bases`, `/shared-agents` | `collaboration role` + tenant role cap | 协作空间只是横向通道，不改写源租户归属 |

## 4. 超级管理员跨空间越权边界

超级管理员的越权边界是：

- 可以切换并管理任意空间
- 不要求目标空间预先存在 `tenant_members` 行
- 在目标空间内等效 `tenant admin`
- 不自动等效目标空间 `owner`

因此：

- 可以管理成员、模型、组织树、Agent、KB
- 不应默认执行“必须 owner”的删除租户等动作，除非接口明确允许系统级越权

## 5. 当前实现与后续约束

### 已实现

- KB / Agent 同租户 visibility 已统一
- KB 子资源 creator lookup 已对齐 `CanManageKB`
- KB route middleware 的 same-tenant 分支已接入 `KBVisibilityService`
- Agent 详情 / 复制 / 推荐问题入口已接入 visibility 判定

### 尚未存在的实现面

- Workflow：当前仓库无模块、无接口、无路由、无文档条目

### 后续新增资源的强约束

后续新增任意“同租户可见性”资源时：

1. 先定义 `visibility + organization_id + created_by`
2. 读权限接 `sameTenantResourceAuthorizer`
3. 写权限复用 `CanManage*` + route-level creator lookup
4. 不再在 handler 内复制一套 `if tenant_id == currentTenant` 的快捷分支
