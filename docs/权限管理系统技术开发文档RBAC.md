# WeKnora 权限管理系统技术开发文档

> **文档版本**: v2.0  
> **创建日期**: 2026-02-09  
> **更新日期**: 2026-02-09  
> **适用项目**: WeKnora 知识库管理平台  
> **文档性质**: 技术开发设计文档

> **v2.0 更新说明**：
> - 新增知识库可见性模型（public/private）和部门归属
> - 普通用户可创建本部门公开/私有知识库
> - 普通用户可删除自己创建的知识库
> - 普通用户可创建属于自己的 Agent 并进行 CRUD
> - 扩展 knowledge_bases 表（created_by、visibility、department_id）
> - 更新权限矩阵、路由设计、前端实现方案

---

## 目录

- [1. 项目概述与现状分析](#1-项目概述与现状分析)
  - [1.1 技术架构概览](#11-技术架构概览)
  - [1.2 现有权限模型分析](#12-现有权限模型分析)
  - [1.3 改造目标](#13-改造目标)
- [2. 权限模型设计](#2-权限模型设计)
  - [2.1 整体权限架构](#21-整体权限架构)
  - [2.2 角色定义](#22-角色定义)
  - [2.3 权限资源与操作定义](#23-权限资源与操作定义)
  - [2.4 知识库-部门权限关联模型](#24-知识库-部门权限关联模型)
- [3. 数据库设计](#3-数据库设计)
  - [3.1 新增表结构](#31-新增表结构)
  - [3.2 现有表修改](#32-现有表修改)
  - [3.3 数据库迁移脚本](#33-数据库迁移脚本)
  - [3.4 ER 关系图](#34-er-关系图)
- [4. 后端 API 设计](#4-后端-api-设计)
  - [4.1 部门管理 API](#41-部门管理-api)
  - [4.2 人员管理 API](#42-人员管理-api)
  - [4.3 角色与权限 API](#43-角色与权限-api)
  - [4.4 知识库权限 API](#44-知识库权限-api)
- [5. 后端实现方案](#5-后端实现方案)
  - [5.1 Go 类型定义](#51-go-类型定义)
  - [5.2 Repository 层](#52-repository-层)
  - [5.3 Service 层](#53-service-层)
  - [5.4 Handler 层](#54-handler-层)
  - [5.5 权限中间件](#55-权限中间件)
  - [5.6 DI 容器注册](#56-di-容器注册)
- [6. 前端实现方案](#6-前端实现方案)
  - [6.1 权限状态管理](#61-权限状态管理)
  - [6.2 路由权限守卫](#62-路由权限守卫)
  - [6.3 管理员与普通用户页面差异](#63-管理员与普通用户页面差异)
  - [6.4 权限指令与组件](#64-权限指令与组件)
  - [6.5 新增页面与组件](#65-新增页面与组件)
  - [6.6 API 服务层](#66-api-服务层)
- [7. 实施计划与里程碑](#7-实施计划与里程碑)
- [8. 附录](#8-附录)
  - [8.1 权限码清单](#81-权限码清单)
  - [8.2 数据字典](#82-数据字典)

---

## 1. 项目概述与现状分析

### 1.1 技术架构概览

WeKnora 是一个基于知识库的智能问答平台，采用前后端分离架构：

| 层级 | 技术选型 |
|------|----------|
| **前端** | Vue 3.5 + TypeScript 5.8 + Vite 7 + TDesign + Pinia + Vue Router 4 |
| **后端** | Go 1.24 + Gin + GORM + PostgreSQL |
| **DI 框架** | go.uber.org/dig |
| **认证** | JWT (HS256) + API Key 双模式 |
| **异步任务** | Asynq (Redis-backed) |
| **向量检索** | PostgreSQL pgvector / Elasticsearch / Qdrant |
| **文件存储** | MinIO / COS / 本地文件系统 |

**后端分层架构**:

```
cmd/server/          → 应用入口
internal/container/  → DI 容器配置
internal/router/     → 路由定义
internal/middleware/  → 中间件（CORS、Auth、Logger、Recovery、Error、Tracing）
internal/handler/    → HTTP Handler 层
internal/application/service/    → 业务逻辑层（Service）
internal/application/repository/ → 数据访问层（Repository）
internal/types/      → 数据模型 + DTO
internal/types/interfaces/ → Service/Repository 接口定义
```

**前端目录结构**:

```
frontend/src/
├── api/          → API 服务层（Axios 封装）
├── components/   → 全局组件（Menu、UserMenu、TenantSelector 等）
├── router/       → 路由配置 + 守卫
├── stores/       → Pinia 状态管理（auth、menu、settings、knowledge、ui）
├── utils/        → 工具函数（request.ts 统一请求封装）
├── views/        → 页面视图
│   ├── auth/     → 登录页
│   ├── knowledge/→ 知识库管理
│   ├── agent/    → 智能体管理
│   ├── chat/     → 聊天对话
│   ├── settings/ → 系统设置
│   └── platform/ → 主布局
└── locales/      → 国际化（zh-CN, en-US, ru-RU, ko-KR）
```

### 1.2 现有权限模型分析

当前系统的权限模型**极其简单**，存在以下问题：

#### 1.2.1 现有机制

| 维度 | 现状 | 问题 |
|------|------|------|
| **角色系统** | 无。仅有 `users.can_access_all_tenants` 一个布尔字段 | 无法区分管理员和普通用户 |
| **部门组织** | 无。用户直接关联租户 | 无组织架构管理能力 |
| **权限控制** | 同一租户下所有用户权限完全相同 | 无法做细粒度权限管控 |
| **知识库权限** | 仅通过 `tenant_id` 做租户级隔离 | 无法按部门/用户授权知识库 |
| **前端权限** | 所有已登录用户看到完全相同的界面 | 管理员和普通用户无差异 |
| **路由守卫** | 仅检查 `isLoggedIn` | 无角色/权限级别的路由控制 |

#### 1.2.2 现有认证流程

```
请求 → CORS → RequestID → Logger → Recovery → ErrorHandler → Auth中间件 → Tracing → Handler
```

Auth 中间件执行逻辑：
1. 免认证路径（`/health`、`/auth/register`、`/auth/login`、`/auth/refresh`）直接放行
2. 尝试 JWT Bearer Token → 解析 user → 注入 `TenantID`/`User` 到 context
3. 尝试 API Key → 解密获取 `tenant_id` → 注入 `TenantID` 到 context
4. 均无 → 返回 401

#### 1.2.3 现有数据模型

```sql
-- 当前 users 表
CREATE TABLE users (
    id              VARCHAR(36) PRIMARY KEY,
    username        VARCHAR(100) UNIQUE,
    email           VARCHAR(255) UNIQUE,
    password_hash   VARCHAR(255),
    avatar          VARCHAR(500),
    tenant_id       INTEGER REFERENCES tenants(id) ON DELETE SET NULL,
    is_active       BOOLEAN DEFAULT true,
    can_access_all_tenants BOOLEAN DEFAULT false,  -- 唯一的"权限"字段
    created_at      TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ,
    deleted_at      TIMESTAMPTZ
);
```

### 1.3 改造目标

| 序号 | 目标 | 描述 |
|------|------|------|
| 1 | **管理员/普通用户角色区分** | 系统支持管理员和普通用户两种角色，前端页面根据角色展示不同内容 |
| 2 | **部门管理** | 实现完整的树形部门结构管理（创建、编辑、删除、层级调整） |
| 3 | **人员管理** | 管理员可管理租户下所有用户，分配角色和部门 |
| 4 | **知识库可见性与部门归属** | 知识库具有可见性（公开/私有）和部门归属；普通用户可查看本部门所有公开知识库及自己的私有知识库 |
| 5 | **用户创建知识库** | 普通用户可创建属于本部门的公开知识库或仅自己可见的私有知识库 |
| 6 | **用户删除自建知识库** | 普通用户可删除自己创建的知识库（无论公开/私有），管理员可删除所有知识库 |
| 7 | **用户 Agent 自主管理** | 普通用户可创建属于自己的智能体（Agent），并对其进行完整的 CRUD 操作 |
| 8 | **知识库精细授权** | 知识库可通过 `kb_permissions` 额外授权给指定部门/人员，实现跨部门共享 |
| 9 | **前端差异化展示** | 管理员可见系统管理菜单（人员、部门、权限），普通用户仅见授权的业务功能 |

---

## 2. 权限模型设计

### 2.1 整体权限架构

采用 **RBAC（基于角色的访问控制）+ 资源授权** 混合模型：

```
┌─────────────────────────────────────────────────────────────────┐
│                        租户（Tenant）                            │
│                                                                  │
│  ┌─────────────┐    ┌─────────────┐    ┌──────────────────┐     │
│  │  部门管理    │    │  角色管理    │    │   知识库管理      │     │
│  │  Department  │    │    Role     │    │  KnowledgeBase   │     │
│  └──────┬──────┘    └──────┬──────┘    └────────┬─────────┘     │
│         │                  │                     │               │
│         ▼                  ▼                     ▼               │
│  ┌─────────────┐    ┌─────────────┐    ┌──────────────────┐     │
│  │  用户-部门   │    │  用户-角色   │    │  知识库权限授权   │     │
│  │  关联表      │    │  关联表      │    │  (部门/用户)     │     │
│  │ user_depts  │    │ user_roles  │    │ kb_permissions    │     │
│  └──────┬──────┘    └──────┬──────┘    └────────┬─────────┘     │
│         │                  │                     │               │
│         └──────────────────┼─────────────────────┘               │
│                            ▼                                     │
│                    ┌──────────────┐                              │
│                    │    用户      │                              │
│                    │    User     │                              │
│                    └──────────────┘                              │
└─────────────────────────────────────────────────────────────────┘
```

**核心设计原则**：
1. **租户隔离**：所有权限数据均在租户维度内，跨租户完全隔离
2. **角色简洁**：系统预置 `admin`（管理员）和 `user`（普通用户）两种角色
3. **知识库授权双通道**：知识库可授权给「部门」或「个人」，取并集
4. **管理员全权限**：管理员角色自动拥有租户下所有知识库的完全访问权

### 2.2 角色定义

| 角色标识 | 角色名称 | 数据范围 | 功能权限 |
|----------|----------|----------|----------|
| `admin` | 管理员 | 租户内所有数据 | 全部功能 + 系统管理（部门/人员/权限） |
| `user` | 普通用户 | 仅授权的知识库 | 基础业务功能（查看知识库、对话、FAQ 等） |

**详细权限矩阵**：

| 功能模块 | 操作 | admin | user |
|----------|------|-------|------|
| **系统管理** | 部门管理（CRUD） | ✅ | ❌ |
| **系统管理** | 人员管理（CRUD + 角色分配） | ✅ | ❌ |
| **系统管理** | 知识库权限分配 | ✅ | ❌ |
| **知识库** | 创建知识库 | ✅ 全部 | ✅ 仅本部门（公开/私有） |
| **知识库** | 删除知识库 | ✅ 全部 | ✅ 仅自己创建的 |
| **知识库** | 编辑知识库配置 | ✅ 全部 | ✅ 仅自己创建的 |
| **知识库** | 查看知识库列表 | ✅ 全部 | ✅ 本部门公开 + 自己的私有 + 被授权的 |
| **知识库** | 查看知识库详情 | ✅ 全部 | ✅ 本部门公开 + 自己的私有 + 被授权的 |
| **知识库** | 上传/删除文档 | ✅ | ✅ 自己创建的 + write权限的 |
| **知识库** | 搜索知识 | ✅ 全部 | ✅ 可访问的知识库 |
| **智能体** | 创建 Agent | ✅ | ✅ 仅属于自己的 |
| **智能体** | 查看 Agent 列表 | ✅ 全部 | ✅ 全部（可使用对话） |
| **智能体** | 编辑 Agent | ✅ 全部 | ✅ 仅自己创建的 |
| **智能体** | 删除 Agent | ✅ 全部 | ✅ 仅自己创建的 |
| **智能体** | 使用 Agent 对话 | ✅ | ✅ |
| **对话** | 创建/查看/删除会话 | ✅ | ✅ |
| **模型** | 模型管理（CRUD） | ✅ | ❌ |
| **MCP** | MCP 服务管理 | ✅ | ❌ |
| **设置** | 系统全局设置 | ✅ | ❌ |
| **设置** | 个人设置（密码、头像） | ✅ | ✅ |

### 2.3 权限资源与操作定义

采用 `resource:action` 格式定义权限码：

```
department:create    department:read    department:update    department:delete
user:create          user:read          user:update          user:delete
knowledgebase:create knowledgebase:read knowledgebase:update knowledgebase:delete
knowledge:create     knowledge:read     knowledge:update     knowledge:delete
agent:create         agent:read         agent:update         agent:delete
model:create         model:read         model:update         model:delete
session:create       session:read       session:update       session:delete
mcp:create           mcp:read           mcp:update           mcp:delete
settings:read        settings:update
kb_permission:read   kb_permission:update
```

### 2.4 知识库可见性与权限模型

知识库采用 **可见性 + 部门归属 + 精细授权** 三层权限模型：

#### 2.4.1 知识库属性扩展

每个知识库新增以下属性：

| 属性 | 说明 |
|------|------|
| `created_by` | 创建者用户 ID，用于所有权判定 |
| `visibility` | 可见性：`public`（部门公开）/ `private`（仅创建者可见） |
| `department_id` | 所属部门 ID，公开知识库归属的部门 |

#### 2.4.2 可见性规则

```
公开知识库（visibility='public'）：
  → 归属部门下所有用户可查看
  → 其他部门用户需通过 kb_permissions 授权才可访问

私有知识库（visibility='private'）：
  → 仅创建者本人可见
  → 管理员可见
```

#### 2.4.3 知识库访问权限判定逻辑

```
用户可查看知识库 = 
    用户是管理员（admin 角色）
    OR 知识库是用户自己创建的（created_by = user.id）
    OR 知识库为公开且归属用户所在部门（visibility='public' AND department_id IN user_departments）
    OR 知识库通过 kb_permissions 直接授权给该用户
    OR 知识库通过 kb_permissions 授权给用户所在的任一部门
```

```
用户可编辑/删除知识库 = 
    用户是管理员
    OR 知识库是用户自己创建的（created_by = user.id）
```

#### 2.4.4 Agent 所有权模型

```
用户可查看 Agent = 
    租户内所有用户均可查看所有 Agent（用于对话选择）

用户可编辑/删除 Agent = 
    用户是管理员
    OR Agent 是用户自己创建的（created_by = user.id）

用户可创建 Agent = 
    所有登录用户均可创建属于自己的 Agent
```

#### 2.4.5 知识库精细授权（kb_permissions）

除了基于可见性的基本访问控制，管理员还可通过 `kb_permissions` 表实现跨部门精细授权：

```
知识库 ──授权──→ 部门（部门下所有用户可访问）
知识库 ──授权──→ 用户（指定用户可访问）
```

**权限级别**：

| 级别 | 标识 | 说明 |
|------|------|------|
| 只读 | `read` | 查看知识库内容、搜索知识 |
| 读写 | `write` | 只读 + 上传文档、编辑 FAQ、删除文档 |
| 管理 | `manage` | 读写 + 编辑知识库配置、管理标签 |

---

## 3. 数据库设计

### 3.1 新增表结构

#### 3.1.1 `departments` —— 部门表

```sql
CREATE TABLE departments (
    id              VARCHAR(36) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    tenant_id       INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    parent_id       VARCHAR(36) REFERENCES departments(id) ON DELETE SET NULL,
    name            VARCHAR(128) NOT NULL,
    code            VARCHAR(64),              -- 部门编码（租户内唯一）
    description     TEXT DEFAULT '',
    sort_order      INTEGER DEFAULT 0,        -- 同级排序
    leader_user_id  VARCHAR(36),              -- 部门负责人
    status          VARCHAR(20) DEFAULT 'active',  -- active / disabled
    path            TEXT DEFAULT '',           -- 物化路径，如 '/root_id/parent_id/self_id'
    level           INTEGER DEFAULT 1,        -- 层级深度（1=顶级部门）
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE(tenant_id, code)
);

CREATE INDEX idx_departments_tenant_id ON departments(tenant_id);
CREATE INDEX idx_departments_parent_id ON departments(parent_id);
CREATE INDEX idx_departments_path ON departments(path);
CREATE INDEX idx_departments_deleted_at ON departments(deleted_at);
```

#### 3.1.2 `roles` —— 角色表

```sql
CREATE TABLE roles (
    id              VARCHAR(36) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    tenant_id       INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code            VARCHAR(32) NOT NULL,      -- 'admin' / 'user'
    name            VARCHAR(64) NOT NULL,      -- 显示名称
    description     TEXT DEFAULT '',
    is_system       BOOLEAN DEFAULT false,     -- 系统预置角色不可删除
    permissions     JSONB DEFAULT '[]',        -- 权限码列表
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE(tenant_id, code)
);

CREATE INDEX idx_roles_tenant_id ON roles(tenant_id);
CREATE INDEX idx_roles_deleted_at ON roles(deleted_at);
```

#### 3.1.3 `user_roles` —— 用户-角色关联表

```sql
CREATE TABLE user_roles (
    id              VARCHAR(36) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id         VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id         VARCHAR(36) NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    tenant_id       INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, role_id, tenant_id)
);

CREATE INDEX idx_user_roles_user_id ON user_roles(user_id);
CREATE INDEX idx_user_roles_role_id ON user_roles(role_id);
CREATE INDEX idx_user_roles_tenant_id ON user_roles(tenant_id);
```

#### 3.1.4 `user_departments` —— 用户-部门关联表

```sql
CREATE TABLE user_departments (
    id              VARCHAR(36) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id         VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    department_id   VARCHAR(36) NOT NULL REFERENCES departments(id) ON DELETE CASCADE,
    tenant_id       INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    is_primary      BOOLEAN DEFAULT false,     -- 主部门标记
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, department_id, tenant_id)
);

CREATE INDEX idx_user_departments_user_id ON user_departments(user_id);
CREATE INDEX idx_user_departments_department_id ON user_departments(department_id);
CREATE INDEX idx_user_departments_tenant_id ON user_departments(tenant_id);
```

#### 3.1.5 `kb_permissions` —— 知识库权限授权表

```sql
CREATE TABLE kb_permissions (
    id                  VARCHAR(36) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    knowledge_base_id   VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    tenant_id           INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    grantee_type        VARCHAR(20) NOT NULL,  -- 'department' / 'user'
    grantee_id          VARCHAR(36) NOT NULL,  -- departments.id 或 users.id
    permission_level    VARCHAR(20) NOT NULL DEFAULT 'read',  -- 'read' / 'write' / 'manage'
    granted_by          VARCHAR(36),           -- 授权操作人 user_id
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(knowledge_base_id, grantee_type, grantee_id)
);

CREATE INDEX idx_kb_permissions_kb_id ON kb_permissions(knowledge_base_id);
CREATE INDEX idx_kb_permissions_grantee ON kb_permissions(grantee_type, grantee_id);
CREATE INDEX idx_kb_permissions_tenant_id ON kb_permissions(tenant_id);
```

### 3.2 现有表修改

#### 3.2.1 `users` 表增加字段

```sql
ALTER TABLE users ADD COLUMN phone VARCHAR(20) DEFAULT '';
ALTER TABLE users ADD COLUMN position VARCHAR(100) DEFAULT '';     -- 职位
ALTER TABLE users ADD COLUMN employee_no VARCHAR(64) DEFAULT '';   -- 工号

-- can_access_all_tenants 字段保留，用于跨租户超级管理员
```

#### 3.2.2 `knowledge_bases` 表增加字段

```sql
-- 创建者字段，记录知识库是谁创建的，用于所有权判定
ALTER TABLE knowledge_bases ADD COLUMN created_by VARCHAR(36) DEFAULT '';

-- 可见性字段：public=部门公开，private=仅创建者可见
ALTER TABLE knowledge_bases ADD COLUMN visibility VARCHAR(20) DEFAULT 'public';

-- 所属部门，公开知识库归属的部门
ALTER TABLE knowledge_bases ADD COLUMN department_id VARCHAR(36) DEFAULT '';

CREATE INDEX idx_knowledge_bases_created_by ON knowledge_bases(created_by);
CREATE INDEX idx_knowledge_bases_visibility ON knowledge_bases(visibility);
CREATE INDEX idx_knowledge_bases_department_id ON knowledge_bases(department_id);
```

#### 3.2.3 `custom_agents` 表补充说明

`custom_agents` 表已有 `created_by` 字段，但当前未被使用。本次改造需在创建 Agent 时正确赋值 `created_by = user.id`，并在编辑/删除时基于该字段进行所有权校验。

### 3.3 数据库迁移脚本

**新建迁移文件**：`migrations/versioned/000012_rbac_departments.up.sql`

```sql
-- =============================================
-- WeKnora RBAC & Department Management Migration
-- Version: 000012
-- Description: 添加角色权限、部门管理、知识库权限授权
-- =============================================

BEGIN;

-- 1. 创建部门表
CREATE TABLE IF NOT EXISTS departments (
    id              VARCHAR(36) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    tenant_id       INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    parent_id       VARCHAR(36) REFERENCES departments(id) ON DELETE SET NULL,
    name            VARCHAR(128) NOT NULL,
    code            VARCHAR(64),
    description     TEXT DEFAULT '',
    sort_order      INTEGER DEFAULT 0,
    leader_user_id  VARCHAR(36),
    status          VARCHAR(20) DEFAULT 'active',
    path            TEXT DEFAULT '',
    level           INTEGER DEFAULT 1,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE(tenant_id, code)
);

CREATE INDEX IF NOT EXISTS idx_departments_tenant_id ON departments(tenant_id);
CREATE INDEX IF NOT EXISTS idx_departments_parent_id ON departments(parent_id);
CREATE INDEX IF NOT EXISTS idx_departments_path ON departments(path);
CREATE INDEX IF NOT EXISTS idx_departments_deleted_at ON departments(deleted_at);

-- 2. 创建角色表
CREATE TABLE IF NOT EXISTS roles (
    id              VARCHAR(36) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    tenant_id       INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code            VARCHAR(32) NOT NULL,
    name            VARCHAR(64) NOT NULL,
    description     TEXT DEFAULT '',
    is_system       BOOLEAN DEFAULT false,
    permissions     JSONB DEFAULT '[]',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE(tenant_id, code)
);

CREATE INDEX IF NOT EXISTS idx_roles_tenant_id ON roles(tenant_id);
CREATE INDEX IF NOT EXISTS idx_roles_deleted_at ON roles(deleted_at);

-- 3. 创建用户-角色关联表
CREATE TABLE IF NOT EXISTS user_roles (
    id              VARCHAR(36) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id         VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id         VARCHAR(36) NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    tenant_id       INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, role_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_user_roles_user_id ON user_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_role_id ON user_roles(role_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_tenant_id ON user_roles(tenant_id);

-- 4. 创建用户-部门关联表
CREATE TABLE IF NOT EXISTS user_departments (
    id              VARCHAR(36) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id         VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    department_id   VARCHAR(36) NOT NULL REFERENCES departments(id) ON DELETE CASCADE,
    tenant_id       INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    is_primary      BOOLEAN DEFAULT false,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, department_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_user_departments_user_id ON user_departments(user_id);
CREATE INDEX IF NOT EXISTS idx_user_departments_department_id ON user_departments(department_id);
CREATE INDEX IF NOT EXISTS idx_user_departments_tenant_id ON user_departments(tenant_id);

-- 5. 创建知识库权限授权表
CREATE TABLE IF NOT EXISTS kb_permissions (
    id                  VARCHAR(36) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    knowledge_base_id   VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    tenant_id           INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    grantee_type        VARCHAR(20) NOT NULL,
    grantee_id          VARCHAR(36) NOT NULL,
    permission_level    VARCHAR(20) NOT NULL DEFAULT 'read',
    granted_by          VARCHAR(36),
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(knowledge_base_id, grantee_type, grantee_id)
);

CREATE INDEX IF NOT EXISTS idx_kb_permissions_kb_id ON kb_permissions(knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_kb_permissions_grantee ON kb_permissions(grantee_type, grantee_id);
CREATE INDEX IF NOT EXISTS idx_kb_permissions_tenant_id ON kb_permissions(tenant_id);

-- 6. 扩展 users 表
ALTER TABLE users ADD COLUMN IF NOT EXISTS phone VARCHAR(20) DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS position VARCHAR(100) DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS employee_no VARCHAR(64) DEFAULT '';

-- 7. 扩展 knowledge_bases 表（新增创建者、可见性、部门归属字段）
ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS created_by VARCHAR(36) DEFAULT '';
ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS visibility VARCHAR(20) DEFAULT 'public';
ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS department_id VARCHAR(36) DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_knowledge_bases_created_by ON knowledge_bases(created_by);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_visibility ON knowledge_bases(visibility);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_department_id ON knowledge_bases(department_id);

-- 8. 为现有租户初始化系统角色
-- admin 角色
INSERT INTO roles (id, tenant_id, code, name, description, is_system, permissions)
SELECT 
    gen_random_uuid()::text, 
    t.id, 
    'admin', 
    '管理员', 
    '系统管理员，拥有全部权限',
    true,
    '["department:create","department:read","department:update","department:delete","user:create","user:read","user:update","user:delete","knowledgebase:create","knowledgebase:read","knowledgebase:update","knowledgebase:delete","knowledge:create","knowledge:read","knowledge:update","knowledge:delete","agent:create","agent:read","agent:update","agent:delete","model:create","model:read","model:update","model:delete","session:create","session:read","session:update","session:delete","mcp:create","mcp:read","mcp:update","mcp:delete","settings:read","settings:update","kb_permission:read","kb_permission:update"]'::jsonb
FROM tenants t
WHERE t.deleted_at IS NULL
ON CONFLICT (tenant_id, code) DO NOTHING;

-- user 角色
INSERT INTO roles (id, tenant_id, code, name, description, is_system, permissions)
SELECT 
    gen_random_uuid()::text, 
    t.id, 
    'user', 
    '普通用户', 
    '普通用户，可创建知识库和智能体，管理自己创建的资源',
    true,
    '["knowledgebase:create","knowledgebase:read","knowledgebase:update","knowledgebase:delete","knowledge:read","knowledge:create","knowledge:update","agent:create","agent:read","agent:update","agent:delete","session:create","session:read","session:update","session:delete","model:read"]'::jsonb
FROM tenants t
WHERE t.deleted_at IS NULL
ON CONFLICT (tenant_id, code) DO NOTHING;

-- 8. 将现有用户分配为所在租户的 admin 角色
INSERT INTO user_roles (id, user_id, role_id, tenant_id)
SELECT 
    gen_random_uuid()::text,
    u.id,
    r.id,
    u.tenant_id
FROM users u
JOIN roles r ON r.tenant_id = u.tenant_id AND r.code = 'admin'
WHERE u.deleted_at IS NULL AND u.tenant_id IS NOT NULL
ON CONFLICT (user_id, role_id, tenant_id) DO NOTHING;

COMMIT;
```

**回滚迁移文件**：`migrations/versioned/000012_rbac_departments.down.sql`

```sql
BEGIN;

DROP TABLE IF EXISTS kb_permissions;
DROP TABLE IF EXISTS user_departments;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS departments;

ALTER TABLE users DROP COLUMN IF EXISTS phone;
ALTER TABLE users DROP COLUMN IF EXISTS position;
ALTER TABLE users DROP COLUMN IF EXISTS employee_no;

ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS created_by;
ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS visibility;
ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS department_id;

COMMIT;
```

### 3.4 ER 关系图

```
┌─────────────┐     1:N     ┌─────────────┐     N:M     ┌─────────────┐
│   tenants   │────────────→│    users     │←───────────→│    roles    │
│             │             │             │  user_roles  │             │
│  id (PK)    │             │  id (PK)    │              │  id (PK)    │
│  name       │             │  username   │              │  code       │
│  api_key    │             │  email      │              │  name       │
│  ...        │             │  tenant_id  │              │  permissions│
└──────┬──────┘             │  phone      │              │  is_system  │
       │                    │  position   │              │  tenant_id  │
       │                    │  employee_no│              └─────────────┘
       │                    └──────┬──────┘
       │                           │ N:M
       │                    ┌──────┴──────┐
       │     1:N            │user_departments│
       ├───────────→┌───────┴─────────────┴──────┐
       │            │       departments          │
       │            │  id (PK)                   │
       │            │  tenant_id                 │
       │            │  parent_id (自引用)        │
       │            │  name                      │
       │            │  path (物化路径)           │
       │            │  level                     │
       │            └────────────────────────────┘
       │
       │     1:N     ┌─────────────────┐
       ├────────────→│ knowledge_bases  │
       │             │  id (PK)        │
       │             │  tenant_id      │
       │             │  name           │
       │             └────────┬────────┘
       │                      │ 1:N
       │              ┌───────┴────────┐
       │              │ kb_permissions │
       │              │  id (PK)       │
       │              │  kb_id         │
       │              │  grantee_type  │──→ 'department' / 'user'
       │              │  grantee_id    │──→ departments.id / users.id
       │              │  permission_level│──→ 'read' / 'write' / 'manage'
       │              └────────────────┘
```

---

## 4. 后端 API 设计

### 4.1 部门管理 API

> 前缀：`/api/v1/departments`  
> 权限要求：`admin` 角色

| 方法 | 路径 | 描述 | 请求体/参数 | 响应 |
|------|------|------|-------------|------|
| `POST` | `/departments` | 创建部门 | `CreateDepartmentRequest` | `Department` |
| `GET` | `/departments/tree` | 获取部门树 | `?status=active` | `[]DepartmentTreeNode` |
| `GET` | `/departments` | 部门列表（扁平） | `?parent_id=&keyword=` | `[]Department` |
| `GET` | `/departments/:id` | 部门详情 | - | `Department` |
| `PUT` | `/departments/:id` | 更新部门 | `UpdateDepartmentRequest` | `Department` |
| `DELETE` | `/departments/:id` | 删除部门 | `?force=false` | - |
| `GET` | `/departments/:id/users` | 部门下用户列表 | `?page=1&page_size=20` | `PaginatedUsers` |
| `POST` | `/departments/:id/users` | 批量添加用户到部门 | `{user_ids: []}` | - |
| `DELETE` | `/departments/:id/users` | 批量移除部门用户 | `{user_ids: []}` | - |

**请求/响应结构**：

```json
// CreateDepartmentRequest
{
    "name": "技术部",
    "code": "TECH",
    "parent_id": "",          // 空字符串表示顶级部门
    "description": "技术研发部门",
    "sort_order": 1,
    "leader_user_id": "uuid-xxx"
}

// DepartmentTreeNode（递归结构）
{
    "id": "uuid-xxx",
    "name": "技术部",
    "code": "TECH",
    "description": "技术研发部门",
    "sort_order": 1,
    "leader_user_id": "uuid-xxx",
    "leader_name": "张三",
    "user_count": 15,
    "status": "active",
    "level": 1,
    "children": [
        {
            "id": "uuid-yyy",
            "name": "前端组",
            "parent_id": "uuid-xxx",
            "level": 2,
            "children": []
        }
    ]
}
```

### 4.2 人员管理 API

> 前缀：`/api/v1/members`  
> 权限要求：`admin` 角色（查看列表除外）

| 方法 | 路径 | 描述 | 权限 |
|------|------|------|------|
| `GET` | `/members` | 人员列表（分页+搜索+筛选） | admin |
| `POST` | `/members` | 创建用户（管理员邀请） | admin |
| `GET` | `/members/:id` | 用户详情 | admin |
| `PUT` | `/members/:id` | 更新用户信息 | admin |
| `DELETE` | `/members/:id` | 删除/禁用用户 | admin |
| `PUT` | `/members/:id/roles` | 分配角色 | admin |
| `PUT` | `/members/:id/departments` | 分配部门 | admin |
| `POST` | `/members/:id/reset-password` | 重置密码 | admin |
| `GET` | `/members/export` | 导出用户列表 | admin |

**请求/响应结构**:

```json
// GET /members?page=1&page_size=20&keyword=张&department_id=xxx&role_code=admin&status=active
// Response:
{
    "total": 100,
    "page": 1,
    "page_size": 20,
    "items": [
        {
            "id": "uuid-xxx",
            "username": "zhangsan",
            "email": "zhang@example.com",
            "phone": "13800138000",
            "position": "高级工程师",
            "employee_no": "EMP001",
            "avatar": "https://...",
            "is_active": true,
            "roles": [
                {"id": "uuid-r1", "code": "admin", "name": "管理员"}
            ],
            "departments": [
                {"id": "uuid-d1", "name": "技术部", "is_primary": true}
            ],
            "created_at": "2026-01-01T00:00:00Z"
        }
    ]
}

// POST /members（管理员创建用户）
{
    "username": "lisi",
    "email": "lisi@example.com",
    "password": "initial_password",
    "phone": "13900139000",
    "position": "产品经理",
    "employee_no": "EMP002",
    "role_codes": ["user"],
    "department_ids": ["uuid-d1"]
}

// PUT /members/:id/roles
{
    "role_codes": ["admin"]
}

// PUT /members/:id/departments
{
    "department_ids": ["uuid-d1", "uuid-d2"],
    "primary_department_id": "uuid-d1"
}
```

### 4.3 角色与权限 API

> 前缀：`/api/v1/roles`  
> 权限要求：`admin` 角色

| 方法 | 路径 | 描述 |
|------|------|------|
| `GET` | `/roles` | 角色列表 |
| `GET` | `/roles/:id` | 角色详情（含权限列表） |
| `POST` | `/roles` | 创建自定义角色（预留扩展） |
| `PUT` | `/roles/:id` | 更新角色权限 |
| `DELETE` | `/roles/:id` | 删除角色（系统角色不可删） |
| `GET` | `/permissions` | 获取所有可用权限码列表 |

```json
// Role Response
{
    "id": "uuid-xxx",
    "code": "admin",
    "name": "管理员",
    "description": "系统管理员，拥有全部权限",
    "is_system": true,
    "permissions": [
        "department:create",
        "department:read",
        "department:update",
        "department:delete",
        "user:create",
        "..."
    ],
    "user_count": 3
}
```

### 4.4 知识库权限 API

> 前缀：`/api/v1/knowledge-bases/:id/permissions`  
> 权限要求：`admin` 角色

| 方法 | 路径 | 描述 |
|------|------|------|
| `GET` | `/knowledge-bases/:id/permissions` | 获取知识库的权限列表 |
| `POST` | `/knowledge-bases/:id/permissions` | 批量设置知识库权限 |
| `DELETE` | `/knowledge-bases/:id/permissions` | 批量移除权限 |
| `GET` | `/knowledge-bases/:id/permissions/check` | 检查当前用户对该知识库的权限级别 |

```json
// GET /knowledge-bases/:id/permissions
{
    "knowledge_base_id": "uuid-kb1",
    "knowledge_base_name": "产品知识库",
    "permissions": [
        {
            "id": "uuid-p1",
            "grantee_type": "department",
            "grantee_id": "uuid-d1",
            "grantee_name": "技术部",
            "permission_level": "write",
            "granted_by": "uuid-u1",
            "granted_by_name": "管理员",
            "created_at": "2026-01-01T00:00:00Z"
        },
        {
            "id": "uuid-p2",
            "grantee_type": "user",
            "grantee_id": "uuid-u2",
            "grantee_name": "李四",
            "permission_level": "read",
            "granted_by": "uuid-u1",
            "granted_by_name": "管理员",
            "created_at": "2026-01-01T00:00:00Z"
        }
    ]
}

// POST /knowledge-bases/:id/permissions（批量设置）
{
    "permissions": [
        {
            "grantee_type": "department",
            "grantee_id": "uuid-d1",
            "permission_level": "write"
        },
        {
            "grantee_type": "user",
            "grantee_id": "uuid-u2",
            "permission_level": "read"
        }
    ]
}

// DELETE /knowledge-bases/:id/permissions
{
    "permission_ids": ["uuid-p1", "uuid-p2"]
}

// GET /knowledge-bases/:id/permissions/check
// Response
{
    "has_access": true,
    "permission_level": "write",
    "source": "department",      // "department" / "user" / "admin"
    "source_name": "技术部"
}
```

---

## 5. 后端实现方案

### 5.1 Go 类型定义

#### 5.1.1 新增文件 `internal/types/department.go`

```go
package types

import "time"

// Department 部门模型
type Department struct {
    ID            string     `json:"id" gorm:"primaryKey;type:varchar(36)"`
    TenantID      uint64     `json:"tenant_id" gorm:"not null;index"`
    ParentID      string     `json:"parent_id" gorm:"type:varchar(36);index"`
    Name          string     `json:"name" gorm:"type:varchar(128);not null"`
    Code          string     `json:"code" gorm:"type:varchar(64)"`
    Description   string     `json:"description" gorm:"type:text"`
    SortOrder     int        `json:"sort_order" gorm:"default:0"`
    LeaderUserID  string     `json:"leader_user_id" gorm:"type:varchar(36)"`
    Status        string     `json:"status" gorm:"type:varchar(20);default:'active'"`
    Path          string     `json:"path" gorm:"type:text"`
    Level         int        `json:"level" gorm:"default:1"`
    CreatedAt     time.Time  `json:"created_at"`
    UpdatedAt     time.Time  `json:"updated_at"`
    DeletedAt     *time.Time `json:"deleted_at,omitempty" gorm:"index"`

    // 关联（非数据库字段）
    Children      []Department `json:"children,omitempty" gorm:"-"`
    LeaderName    string       `json:"leader_name,omitempty" gorm:"-"`
    UserCount     int64        `json:"user_count,omitempty" gorm:"-"`
}

func (Department) TableName() string { return "departments" }

// CreateDepartmentRequest 创建部门请求
type CreateDepartmentRequest struct {
    Name         string `json:"name" binding:"required,max=128"`
    Code         string `json:"code" binding:"omitempty,max=64"`
    ParentID     string `json:"parent_id"`
    Description  string `json:"description"`
    SortOrder    int    `json:"sort_order"`
    LeaderUserID string `json:"leader_user_id"`
}

// UpdateDepartmentRequest 更新部门请求
type UpdateDepartmentRequest struct {
    Name         *string `json:"name" binding:"omitempty,max=128"`
    Code         *string `json:"code" binding:"omitempty,max=64"`
    ParentID     *string `json:"parent_id"`
    Description  *string `json:"description"`
    SortOrder    *int    `json:"sort_order"`
    LeaderUserID *string `json:"leader_user_id"`
    Status       *string `json:"status"`
}

// DepartmentTreeNode 部门树节点
type DepartmentTreeNode struct {
    Department
    Children []*DepartmentTreeNode `json:"children"`
}
```

#### 5.1.2 新增文件 `internal/types/role.go`

```go
package types

import "time"

// Role 角色模型
type Role struct {
    ID          string     `json:"id" gorm:"primaryKey;type:varchar(36)"`
    TenantID    uint64     `json:"tenant_id" gorm:"not null;index"`
    Code        string     `json:"code" gorm:"type:varchar(32);not null"`
    Name        string     `json:"name" gorm:"type:varchar(64);not null"`
    Description string     `json:"description" gorm:"type:text"`
    IsSystem    bool       `json:"is_system" gorm:"default:false"`
    Permissions JSONArray  `json:"permissions" gorm:"type:jsonb;default:'[]'"`
    CreatedAt   time.Time  `json:"created_at"`
    UpdatedAt   time.Time  `json:"updated_at"`
    DeletedAt   *time.Time `json:"deleted_at,omitempty" gorm:"index"`

    // 计算字段
    UserCount   int64      `json:"user_count,omitempty" gorm:"-"`
}

func (Role) TableName() string { return "roles" }

// JSONArray 用于GORM JSON数组序列化
type JSONArray []string

// UserRole 用户-角色关联
type UserRole struct {
    ID        string    `json:"id" gorm:"primaryKey;type:varchar(36)"`
    UserID    string    `json:"user_id" gorm:"type:varchar(36);not null;index"`
    RoleID    string    `json:"role_id" gorm:"type:varchar(36);not null;index"`
    TenantID  uint64    `json:"tenant_id" gorm:"not null;index"`
    CreatedAt time.Time `json:"created_at"`
}

func (UserRole) TableName() string { return "user_roles" }

// UserDepartment 用户-部门关联
type UserDepartment struct {
    ID           string    `json:"id" gorm:"primaryKey;type:varchar(36)"`
    UserID       string    `json:"user_id" gorm:"type:varchar(36);not null;index"`
    DepartmentID string    `json:"department_id" gorm:"type:varchar(36);not null;index"`
    TenantID     uint64    `json:"tenant_id" gorm:"not null;index"`
    IsPrimary    bool      `json:"is_primary" gorm:"default:false"`
    CreatedAt    time.Time `json:"created_at"`
}

func (UserDepartment) TableName() string { return "user_departments" }

// 角色常量
const (
    RoleCodeAdmin = "admin"
    RoleCodeUser  = "user"
)
```

#### 5.1.3 新增文件 `internal/types/kb_permission.go`

```go
package types

import "time"

// KBPermission 知识库权限授权
type KBPermission struct {
    ID              string    `json:"id" gorm:"primaryKey;type:varchar(36)"`
    KnowledgeBaseID string    `json:"knowledge_base_id" gorm:"type:varchar(36);not null;index"`
    TenantID        uint64    `json:"tenant_id" gorm:"not null;index"`
    GranteeType     string    `json:"grantee_type" gorm:"type:varchar(20);not null"`   // "department" / "user"
    GranteeID       string    `json:"grantee_id" gorm:"type:varchar(36);not null"`
    PermissionLevel string    `json:"permission_level" gorm:"type:varchar(20);not null;default:'read'"`
    GrantedBy       string    `json:"granted_by" gorm:"type:varchar(36)"`
    CreatedAt       time.Time `json:"created_at"`
    UpdatedAt       time.Time `json:"updated_at"`

    // 非数据库字段
    GranteeName   string `json:"grantee_name,omitempty" gorm:"-"`
    GrantedByName string `json:"granted_by_name,omitempty" gorm:"-"`
}

func (KBPermission) TableName() string { return "kb_permissions" }

// 权限级别常量
const (
    PermissionLevelRead   = "read"
    PermissionLevelWrite  = "write"
    PermissionLevelManage = "manage"
)

// 授权对象类型常量
const (
    GranteeTypeDepartment = "department"
    GranteeTypeUser       = "user"
)

// SetKBPermissionsRequest 批量设置知识库权限请求
type SetKBPermissionsRequest struct {
    Permissions []KBPermissionItem `json:"permissions" binding:"required"`
}

type KBPermissionItem struct {
    GranteeType     string `json:"grantee_type" binding:"required,oneof=department user"`
    GranteeID       string `json:"grantee_id" binding:"required"`
    PermissionLevel string `json:"permission_level" binding:"required,oneof=read write manage"`
}

// RemoveKBPermissionsRequest 批量移除权限请求
type RemoveKBPermissionsRequest struct {
    PermissionIDs []string `json:"permission_ids" binding:"required"`
}

// KBPermissionCheckResult 权限检查结果
type KBPermissionCheckResult struct {
    HasAccess       bool   `json:"has_access"`
    PermissionLevel string `json:"permission_level"`
    Source          string `json:"source"`       // "admin" / "department" / "user"
    SourceName      string `json:"source_name"`
}
```

#### 5.1.4 修改 `internal/types/knowledgebase.go` —— KnowledgeBase 结构体增加字段

在现有 `KnowledgeBase` 结构体中新增：

```go
// 新增字段：创建者、可见性、所属部门
CreatedBy    string `json:"created_by" gorm:"type:varchar(36)"`       // 创建者用户ID
Visibility   string `json:"visibility" gorm:"type:varchar(20);default:'public'"`  // 'public' / 'private'
DepartmentID string `json:"department_id" gorm:"type:varchar(36)"`   // 所属部门ID
```

新增请求结构体字段：

```go
// CreateKnowledgeBaseRequest 创建知识库请求增加可见性和部门字段
type CreateKnowledgeBaseRequest struct {
    // ... 现有字段 ...
    Visibility   string `json:"visibility" binding:"omitempty,oneof=public private"`  // 可见性，默认 public
    DepartmentID string `json:"department_id"`                                         // 所属部门（user 角色必填）
}
```

新增可见性常量：

```go
// 知识库可见性常量
const (
    KBVisibilityPublic  = "public"   // 部门公开
    KBVisibilityPrivate = "private"  // 仅创建者可见
)
```

#### 5.1.5 修改 `internal/types/user.go` —— User 结构体增加字段

在现有 `User` 结构体中新增：

```go
// 新增字段
Phone      string `json:"phone" gorm:"type:varchar(20)"`
Position   string `json:"position" gorm:"type:varchar(100)"`
EmployeeNo string `json:"employee_no" gorm:"type:varchar(64)"`

// 新增关联字段（非 DB 列）
Roles       []Role       `json:"roles,omitempty" gorm:"-"`
Departments []Department `json:"departments,omitempty" gorm:"-"`
```

在 `UserInfo` / `LoginResponse` 中新增角色信息：

```go
type UserInfo struct {
    ID                  string   `json:"id"`
    Username            string   `json:"username"`
    Email               string   `json:"email"`
    Avatar              string   `json:"avatar"`
    Phone               string   `json:"phone"`
    Position            string   `json:"position"`
    EmployeeNo          string   `json:"employee_no"`
    IsActive            bool     `json:"is_active"`
    CanAccessAllTenants bool     `json:"can_access_all_tenants"`
    RoleCodes           []string `json:"role_codes"`     // 新增：角色代码列表
    IsAdmin             bool     `json:"is_admin"`       // 新增：是否管理员（便捷字段）
}
```

### 5.2 Repository 层

#### 5.2.1 接口定义 `internal/types/interfaces/department.go`

```go
package interfaces

import "weknora/internal/types"

type DepartmentRepository interface {
    Create(dept *types.Department) error
    GetByID(tenantID uint64, id string) (*types.Department, error)
    Update(dept *types.Department) error
    Delete(tenantID uint64, id string) error
    List(tenantID uint64, parentID string, keyword string) ([]types.Department, error)
    GetTree(tenantID uint64) ([]types.Department, error)
    GetByCode(tenantID uint64, code string) (*types.Department, error)
    GetChildren(tenantID uint64, parentID string) ([]types.Department, error)
    GetByPath(tenantID uint64, pathPrefix string) ([]types.Department, error)
    CountUsers(tenantID uint64, departmentID string) (int64, error)
}

type DepartmentService interface {
    CreateDepartment(tenantID uint64, req *types.CreateDepartmentRequest) (*types.Department, error)
    GetDepartment(tenantID uint64, id string) (*types.Department, error)
    UpdateDepartment(tenantID uint64, id string, req *types.UpdateDepartmentRequest) (*types.Department, error)
    DeleteDepartment(tenantID uint64, id string, force bool) error
    ListDepartments(tenantID uint64, parentID string, keyword string) ([]types.Department, error)
    GetDepartmentTree(tenantID uint64) ([]*types.DepartmentTreeNode, error)
    GetDepartmentUsers(tenantID uint64, deptID string, page, pageSize int) ([]types.User, int64, error)
    AddUsersToDepartment(tenantID uint64, deptID string, userIDs []string) error
    RemoveUsersFromDepartment(tenantID uint64, deptID string, userIDs []string) error
}
```

#### 5.2.2 接口定义 `internal/types/interfaces/role.go`

```go
package interfaces

import "weknora/internal/types"

type RoleRepository interface {
    Create(role *types.Role) error
    GetByID(tenantID uint64, id string) (*types.Role, error)
    GetByCode(tenantID uint64, code string) (*types.Role, error)
    Update(role *types.Role) error
    Delete(tenantID uint64, id string) error
    List(tenantID uint64) ([]types.Role, error)
    GetUserRoles(tenantID uint64, userID string) ([]types.Role, error)
    SetUserRoles(tenantID uint64, userID string, roleIDs []string) error
    ClearUserRoles(tenantID uint64, userID string) error
    CountUsers(tenantID uint64, roleID string) (int64, error)
    InitSystemRoles(tenantID uint64) error
}

type RoleService interface {
    ListRoles(tenantID uint64) ([]types.Role, error)
    GetRole(tenantID uint64, id string) (*types.Role, error)
    CreateRole(tenantID uint64, role *types.Role) (*types.Role, error)
    UpdateRole(tenantID uint64, id string, role *types.Role) (*types.Role, error)
    DeleteRole(tenantID uint64, id string) error
    GetUserRoles(tenantID uint64, userID string) ([]types.Role, error)
    SetUserRoles(tenantID uint64, userID string, roleCodes []string) error
    IsAdmin(tenantID uint64, userID string) (bool, error)
    GetAllPermissions() []string
    InitSystemRoles(tenantID uint64) error
}
```

#### 5.2.3 接口定义 `internal/types/interfaces/kb_permission.go`

```go
package interfaces

import "weknora/internal/types"

type KBPermissionRepository interface {
    Create(perm *types.KBPermission) error
    BatchCreate(perms []types.KBPermission) error
    Delete(tenantID uint64, id string) error
    BatchDelete(tenantID uint64, ids []string) error
    ListByKnowledgeBase(tenantID uint64, kbID string) ([]types.KBPermission, error)
    GetByGrantee(tenantID uint64, kbID string, granteeType string, granteeID string) (*types.KBPermission, error)
    DeleteByKnowledgeBase(tenantID uint64, kbID string) error

    // 权限查询
    GetUserAccessibleKBIDs(tenantID uint64, userID string, departmentIDs []string) ([]string, error)
    CheckUserKBPermission(tenantID uint64, userID string, kbID string, departmentIDs []string) (*types.KBPermissionCheckResult, error)
}

type KBPermissionService interface {
    SetKBPermissions(tenantID uint64, kbID string, grantedBy string, req *types.SetKBPermissionsRequest) error
    RemoveKBPermissions(tenantID uint64, kbID string, permissionIDs []string) error
    GetKBPermissions(tenantID uint64, kbID string) ([]types.KBPermission, error)
    CheckUserKBAccess(tenantID uint64, userID string, kbID string) (*types.KBPermissionCheckResult, error)
    GetUserAccessibleKBIDs(tenantID uint64, userID string) ([]string, error)
    FilterAccessibleKBs(tenantID uint64, userID string, kbs []types.KnowledgeBase) ([]types.KnowledgeBase, error)
}
```

#### 5.2.4 新增 Repository 实现文件

| 文件 | 功能 |
|------|------|
| `internal/application/repository/department.go` | 部门 CRUD、树查询、物化路径管理 |
| `internal/application/repository/role.go` | 角色 CRUD、用户角色关联管理 |
| `internal/application/repository/user_department.go` | 用户-部门关联 |
| `internal/application/repository/kb_permission.go` | 知识库权限授权、权限查询 |

**关键实现 —— `GetUserAccessibleKBIDs`**:

```go
func (r *kbPermissionRepository) GetUserAccessibleKBIDs(
    tenantID uint64, userID string, departmentIDs []string,
) ([]string, error) {
    var kbIDs []string

    query := r.db.Model(&types.KBPermission{}).
        Select("DISTINCT knowledge_base_id").
        Where("tenant_id = ?", tenantID)

    // 用户直接授权 OR 所在部门授权
    if len(departmentIDs) > 0 {
        query = query.Where(
            "(grantee_type = ? AND grantee_id = ?) OR (grantee_type = ? AND grantee_id IN ?)",
            types.GranteeTypeUser, userID,
            types.GranteeTypeDepartment, departmentIDs,
        )
    } else {
        query = query.Where("grantee_type = ? AND grantee_id = ?",
            types.GranteeTypeUser, userID)
    }

    err := query.Pluck("knowledge_base_id", &kbIDs).Error
    return kbIDs, err
}
```

### 5.3 Service 层

#### 5.3.1 新增 Service 实现文件

| 文件 | 功能 |
|------|------|
| `internal/application/service/department.go` | 部门业务逻辑（树构建、路径计算、级联操作） |
| `internal/application/service/role.go` | 角色管理、权限码验证 |
| `internal/application/service/member.go` | 人员管理（创建、角色分配、部门分配） |
| `internal/application/service/kb_permission.go` | 知识库权限业务逻辑 |

**关键业务逻辑示例 —— 部门树构建**:

```go
func (s *departmentService) GetDepartmentTree(tenantID uint64) ([]*types.DepartmentTreeNode, error) {
    departments, err := s.deptRepo.GetTree(tenantID)
    if err != nil {
        return nil, err
    }

    // 构建 map 和树结构
    nodeMap := make(map[string]*types.DepartmentTreeNode)
    var roots []*types.DepartmentTreeNode

    for i := range departments {
        node := &types.DepartmentTreeNode{
            Department: departments[i],
            Children:   make([]*types.DepartmentTreeNode, 0),
        }
        nodeMap[node.ID] = node
    }

    for _, node := range nodeMap {
        if node.ParentID == "" {
            roots = append(roots, node)
        } else if parent, ok := nodeMap[node.ParentID]; ok {
            parent.Children = append(parent.Children, node)
        } else {
            roots = append(roots, node) // 孤儿节点提升为顶级
        }
    }

    // 按 sort_order 排序
    sortTreeNodes(roots)
    return roots, nil
}
```

**关键业务逻辑 —— 知识库权限过滤（基于可见性 + 精细授权）**:

```go
func (s *kbPermissionService) FilterAccessibleKBs(
    tenantID uint64, userID string, kbs []types.KnowledgeBase,
) ([]types.KnowledgeBase, error) {
    // 检查是否管理员
    isAdmin, err := s.roleService.IsAdmin(tenantID, userID)
    if err != nil {
        return nil, err
    }
    if isAdmin {
        return kbs, nil // 管理员可访问所有知识库
    }

    // 获取用户所有部门ID（含上级部门）
    deptIDs, err := s.getUserAllDepartmentIDs(tenantID, userID)
    if err != nil {
        return nil, err
    }

    // 获取通过 kb_permissions 授权的知识库ID列表
    grantedKBIDs, err := s.kbPermRepo.GetUserAccessibleKBIDs(tenantID, userID, deptIDs)
    if err != nil {
        return nil, err
    }
    grantedSet := make(map[string]bool)
    for _, id := range grantedKBIDs {
        grantedSet[id] = true
    }

    // 构建部门ID set
    deptSet := make(map[string]bool)
    for _, id := range deptIDs {
        deptSet[id] = true
    }

    var result []types.KnowledgeBase
    for _, kb := range kbs {
        accessible := false
        switch {
        case kb.CreatedBy == userID:
            // 用户自己创建的知识库（无论公开/私有）始终可见
            accessible = true
        case kb.Visibility == types.KBVisibilityPublic && deptSet[kb.DepartmentID]:
            // 本部门的公开知识库
            accessible = true
        case grantedSet[kb.ID]:
            // 通过 kb_permissions 授权的知识库
            accessible = true
        }
        if accessible {
            result = append(result, kb)
        }
    }
    return result, nil
}
```

#### 5.3.2 修改现有 Service

**`internal/application/service/user.go`** —— 用户注册后初始化角色：

```go
func (s *userService) Register(req *types.RegisterRequest) (*types.User, error) {
    // ... 现有逻辑（创建用户、创建租户）...

    // 新增：为新租户初始化系统角色
    if err := s.roleService.InitSystemRoles(tenant.ID); err != nil {
        return nil, fmt.Errorf("failed to init system roles: %w", err)
    }

    // 新增：将注册用户分配为 admin 角色
    if err := s.roleService.SetUserRoles(tenant.ID, user.ID, []string{types.RoleCodeAdmin}); err != nil {
        return nil, fmt.Errorf("failed to set admin role: %w", err)
    }

    return user, nil
}
```

**`internal/application/service/knowledgebase.go`** —— 创建知识库时设置创建者、可见性和部门：

```go
func (s *knowledgeBaseService) CreateKnowledgeBase(ctx context.Context, kb *types.KnowledgeBase) (*types.KnowledgeBase, error) {
    tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
    kb.TenantID = tenantID

    // 新增：设置创建者
    if user, ok := ctx.Value(types.UserContextKey).(*types.User); ok {
        kb.CreatedBy = user.ID
    }

    // 新增：验证可见性和部门
    if kb.Visibility == "" {
        kb.Visibility = types.KBVisibilityPublic
    }
    // 普通用户创建时，必须指定部门且仅能选择自己所在的部门
    // 管理员可为任意部门创建知识库

    // ... 现有军建逻辑（创建向量表/初始化配置等）...

    return kb, nil
}
```

**`internal/application/service/custom_agent.go`** —— 创建 Agent 时设置创建者，编辑/删除时校验所有权：

```go
func (s *customAgentService) CreateAgent(ctx context.Context, agent *types.CustomAgent) (*types.CustomAgent, error) {
    tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
    agent.TenantID = tenantID

    // 新增：设置创建者
    if user, ok := ctx.Value(types.UserContextKey).(*types.User); ok {
        agent.CreatedBy = user.ID
    }

    // ... 现有创建逻辑 ...
    return agent, nil
}

func (s *customAgentService) UpdateAgent(ctx context.Context, agent *types.CustomAgent) (*types.CustomAgent, error) {
    tenantID := ctx.Value(types.TenantIDContextKey).(uint64)

    existing, err := s.repo.GetAgentByID(ctx, agent.ID, tenantID)
    if err != nil {
        return nil, err
    }

    // 新增：非管理员只能编辑自己创建的 Agent
    if user, ok := ctx.Value(types.UserContextKey).(*types.User); ok {
        isAdmin, _ := s.roleService.IsAdmin(tenantID, user.ID)
        if !isAdmin && existing.CreatedBy != user.ID {
            return nil, errors.New("no permission to update this agent")
        }
    }

    // ... 现有更新逻辑 ...
    return agent, nil
}

func (s *customAgentService) DeleteAgent(ctx context.Context, id string) error {
    tenantID := ctx.Value(types.TenantIDContextKey).(uint64)

    existing, err := s.repo.GetAgentByID(ctx, id, tenantID)
    if err != nil {
        return err
    }

    // 内置 Agent 不可删除
    if existing.IsBuiltin {
        return types.ErrCannotDeleteBuiltin
    }

    // 新增：非管理员只能删除自己创建的 Agent
    if user, ok := ctx.Value(types.UserContextKey).(*types.User); ok {
        isAdmin, _ := s.roleService.IsAdmin(tenantID, user.ID)
        if !isAdmin && existing.CreatedBy != user.ID {
            return errors.New("no permission to delete this agent")
        }
    }

    return s.repo.DeleteAgent(ctx, id, tenantID)
}
```

**`internal/handler/knowledgebase.go`** —— 列表查询增加权限过滤，删除增加所有权校验：

```go
func (h *KnowledgeBaseHandler) ListKnowledgeBases(c *gin.Context) {
    tenantID := c.Value(middleware.TenantIDContextKey).(uint64)
    
    kbs, err := h.kbService.ListKnowledgeBases(tenantID)
    if err != nil {
        // ... error handling
    }

    // 新增：权限过滤（基于可见性 + 部门归属 + 精细授权）
    if user, exists := c.Get(middleware.UserInfoContextKey); exists {
        userInfo := user.(*types.User)
        kbs, err = h.kbPermService.FilterAccessibleKBs(tenantID, userInfo.ID, kbs)
        if err != nil {
            // ... error handling
        }
    }
    // API Key 模式不过滤（兼容旧逻辑）

    c.JSON(200, kbs)
}

func (h *KnowledgeBaseHandler) DeleteKnowledgeBase(c *gin.Context) {
    tenantID := c.Value(middleware.TenantIDContextKey).(uint64)
    kbID := c.Param("id")

    kb, err := h.kbService.GetKnowledgeBaseByID(c, kbID)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "knowledge base not found"})
        return
    }

    // 新增：所有权校验——非管理员只能删除自己创建的知识库
    if user, exists := c.Get(middleware.UserInfoContextKey); exists {
        userInfo := user.(*types.User)
        isAdmin, _ := h.roleService.IsAdmin(tenantID, userInfo.ID)
        if !isAdmin && kb.CreatedBy != userInfo.ID {
            c.JSON(http.StatusForbidden, gin.H{"error": "no permission to delete this knowledge base"})
            return
        }
    }

    err = h.kbService.DeleteKnowledgeBase(c, kbID)
    // ... 后续处理
}
```

### 5.4 Handler 层

#### 5.4.1 新增 Handler 文件

| 文件 | 功能 |
|------|------|
| `internal/handler/department.go` | 部门管理全部端点 |
| `internal/handler/member.go` | 人员管理全部端点 |
| `internal/handler/role.go` | 角色管理全部端点 |
| `internal/handler/kb_permission.go` | 知识库权限管理端点 |

**Handler 实现模式参考**（与现有 Handler 保持一致风格）：

```go
// internal/handler/department.go

package handler

import (
    "net/http"
    "github.com/gin-gonic/gin"
    "weknora/internal/middleware"
    "weknora/internal/types"
    "weknora/internal/types/interfaces"
)

type DepartmentHandler struct {
    deptService interfaces.DepartmentService
}

func NewDepartmentHandler(deptService interfaces.DepartmentService) *DepartmentHandler {
    return &DepartmentHandler{deptService: deptService}
}

func (h *DepartmentHandler) CreateDepartment(c *gin.Context) {
    tenantID := c.Value(middleware.TenantIDContextKey).(uint64)

    var req types.CreateDepartmentRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    dept, err := h.deptService.CreateDepartment(tenantID, &req)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusCreated, dept)
}

func (h *DepartmentHandler) GetDepartmentTree(c *gin.Context) {
    tenantID := c.Value(middleware.TenantIDContextKey).(uint64)

    tree, err := h.deptService.GetDepartmentTree(tenantID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, tree)
}

// ... 其他方法
```

### 5.5 权限中间件

#### 5.5.1 新增文件 `internal/middleware/rbac.go`

```go
package middleware

import (
    "net/http"
    "github.com/gin-gonic/gin"
    "weknora/internal/types"
    "weknora/internal/types/interfaces"
)

// RequireRole 角色校验中间件
func RequireRole(roleService interfaces.RoleService, requiredRoles ...string) gin.HandlerFunc {
    return func(c *gin.Context) {
        tenantID, exists := c.Get(TenantIDContextKey)
        if !exists {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "error": "Tenant context not found",
            })
            return
        }

        userInfo, exists := c.Get(UserInfoContextKey)
        if !exists {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "error": "User context not found",
            })
            return
        }

        user := userInfo.(*types.User)
        roles, err := roleService.GetUserRoles(tenantID.(uint64), user.ID)
        if err != nil {
            c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
                "error": "Failed to get user roles",
            })
            return
        }

        // 检查用户是否拥有任一所需角色
        roleSet := make(map[string]bool)
        for _, role := range roles {
            roleSet[role.Code] = true
        }

        hasRole := false
        for _, required := range requiredRoles {
            if roleSet[required] {
                hasRole = true
                break
            }
        }

        if !hasRole {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                "error": "Insufficient permissions",
            })
            return
        }

        // 缓存角色信息到 context 供后续使用
        c.Set("user_roles", roles)
        c.Set("is_admin", roleSet[types.RoleCodeAdmin])

        c.Next()
    }
}

// RequirePermission 精细权限校验中间件
func RequirePermission(roleService interfaces.RoleService, requiredPermission string) gin.HandlerFunc {
    return func(c *gin.Context) {
        tenantID, _ := c.Get(TenantIDContextKey)
        userInfo, _ := c.Get(UserInfoContextKey)

        if tenantID == nil || userInfo == nil {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "error": "Authentication required",
            })
            return
        }

        user := userInfo.(*types.User)
        roles, err := roleService.GetUserRoles(tenantID.(uint64), user.ID)
        if err != nil {
            c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
                "error": "Failed to verify permissions",
            })
            return
        }

        // 汇总所有权限
        permSet := make(map[string]bool)
        for _, role := range roles {
            for _, perm := range role.Permissions {
                permSet[perm] = true
            }
        }

        if !permSet[requiredPermission] {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                "error": "No permission: " + requiredPermission,
            })
            return
        }

        c.Next()
    }
}

// RequireKBAccess 知识库访问权限中间件
func RequireKBAccess(
    kbPermService interfaces.KBPermissionService,
    minLevel string, // "read" / "write" / "manage"
) gin.HandlerFunc {
    return func(c *gin.Context) {
        // 管理员通过 RequireRole 中间件已设置 is_admin
        if isAdmin, _ := c.Get("is_admin"); isAdmin == true {
            c.Next()
            return
        }

        tenantID := c.Value(TenantIDContextKey).(uint64)
        userInfo := c.Value(UserInfoContextKey).(*types.User)
        kbID := c.Param("id")
        if kbID == "" {
            kbID = c.Param("kbId")
        }

        if kbID == "" {
            c.Next() // 非知识库相关路由，跳过
            return
        }

        result, err := kbPermService.CheckUserKBAccess(tenantID, userInfo.ID, kbID)
        if err != nil {
            c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
                "error": "Failed to check KB permission",
            })
            return
        }

        if !result.HasAccess || !isPermissionSufficient(result.PermissionLevel, minLevel) {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                "error": "No access to this knowledge base",
            })
            return
        }

        c.Set("kb_permission", result)
        c.Next()
    }
}

// isPermissionSufficient 判断权限级别是否满足
func isPermissionSufficient(actual, required string) bool {
    levelMap := map[string]int{
        "read": 1, "write": 2, "manage": 3,
    }
    return levelMap[actual] >= levelMap[required]
}
```

### 5.6 路由注册

修改 `internal/router/router.go`，新增路由组：

```go
func RegisterRoutes(
    r *gin.Engine,
    // ... 现有参数 ...
    deptHandler    *handler.DepartmentHandler,
    memberHandler  *handler.MemberHandler,
    roleHandler    *handler.RoleHandler,
    kbPermHandler  *handler.KBPermissionHandler,
    roleService    interfaces.RoleService,
    kbPermService  interfaces.KBPermissionService,
) {
    v1 := r.Group("/api/v1")
    
    // ... 现有路由 ...

    // ===== 新增：系统管理路由（需要 admin 角色）=====
    adminGroup := v1.Group("", RequireRole(roleService, types.RoleCodeAdmin))
    {
        // 部门管理
        depts := adminGroup.Group("/departments")
        {
            depts.POST("", deptHandler.CreateDepartment)
            depts.GET("/tree", deptHandler.GetDepartmentTree)
            depts.GET("", deptHandler.ListDepartments)
            depts.GET("/:id", deptHandler.GetDepartment)
            depts.PUT("/:id", deptHandler.UpdateDepartment)
            depts.DELETE("/:id", deptHandler.DeleteDepartment)
            depts.GET("/:id/users", deptHandler.GetDepartmentUsers)
            depts.POST("/:id/users", deptHandler.AddUsers)
            depts.DELETE("/:id/users", deptHandler.RemoveUsers)
        }

        // 人员管理
        members := adminGroup.Group("/members")
        {
            members.GET("", memberHandler.ListMembers)
            members.POST("", memberHandler.CreateMember)
            members.GET("/:id", memberHandler.GetMember)
            members.PUT("/:id", memberHandler.UpdateMember)
            members.DELETE("/:id", memberHandler.DeleteMember)
            members.PUT("/:id/roles", memberHandler.SetRoles)
            members.PUT("/:id/departments", memberHandler.SetDepartments)
            members.POST("/:id/reset-password", memberHandler.ResetPassword)
        }

        // 角色管理
        roles := adminGroup.Group("/roles")
        {
            roles.GET("", roleHandler.ListRoles)
            roles.GET("/:id", roleHandler.GetRole)
            roles.POST("", roleHandler.CreateRole)
            roles.PUT("/:id", roleHandler.UpdateRole)
            roles.DELETE("/:id", roleHandler.DeleteRole)
        }
        adminGroup.GET("/permissions", roleHandler.ListPermissions)

        // 知识库权限管理
        kbPerms := adminGroup.Group("/knowledge-bases/:id/permissions")
        {
            kbPerms.GET("", kbPermHandler.GetKBPermissions)
            kbPerms.POST("", kbPermHandler.SetKBPermissions)
            kbPerms.DELETE("", kbPermHandler.RemoveKBPermissions)
        }
    }

    // ===== 所有登录用户可用的路由 =====
    // 知识库 CRUD（权限在 Handler/Service 层校验所有权）
    kbs := v1.Group("/knowledge-bases")
    {
        kbs.POST("", kbHandler.CreateKnowledgeBase)         // 所有用户可创建（user 需指定部门+可见性）
        kbs.GET("", kbHandler.ListKnowledgeBases)           // 列表自动按权限过滤
        kbs.GET("/:id", kbHandler.GetKnowledgeBase)         // 详情按权限校验
        kbs.PUT("/:id", kbHandler.UpdateKnowledgeBase)      // admin 全部, user 仅自己创建的
        kbs.DELETE("/:id", kbHandler.DeleteKnowledgeBase)   // admin 全部, user 仅自己创建的
    }

    // Agent CRUD（权限在 Handler/Service 层校验所有权）
    agents := v1.Group("/agents")
    {
        agents.POST("", agentHandler.CreateAgent)           // 所有用户可创建
        agents.GET("", agentHandler.ListAgents)             // 所有用户可查看列表
        agents.GET("/:id", agentHandler.GetAgent)           // 所有用户可查看详情
        agents.PUT("/:id", agentHandler.UpdateAgent)        // admin 全部, user 仅自己创建的
        agents.DELETE("/:id", agentHandler.DeleteAgent)     // admin 全部, user 仅自己创建的
    }

    // 用户角色信息接口（所有登录用户可用）
    v1.GET("/auth/me", authHandler.GetCurrentUser)  // 增强返回角色信息
    v1.GET("/knowledge-bases/:id/permissions/check", kbPermHandler.CheckPermission)
}
```

### 5.6 DI 容器注册

修改 `internal/container/container.go`，新增依赖注册：

```go
// Repository 层
container.Provide(repository.NewDepartmentRepository)
container.Provide(repository.NewRoleRepository)
container.Provide(repository.NewKBPermissionRepository)

// Service 层
container.Provide(service.NewDepartmentService)
container.Provide(service.NewRoleService)
container.Provide(service.NewMemberService)
container.Provide(service.NewKBPermissionService)

// Handler 层
container.Provide(handler.NewDepartmentHandler)
container.Provide(handler.NewMemberHandler)
container.Provide(handler.NewRoleHandler)
container.Provide(handler.NewKBPermissionHandler)
```

---

## 6. 前端实现方案

### 6.1 权限状态管理

#### 6.1.1 新增 `frontend/src/stores/permission.ts`

```typescript
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { useAuthStore } from './auth'

export interface RoleInfo {
  id: string
  code: string
  name: string
  permissions: string[]
}

export const usePermissionStore = defineStore('permission', () => {
  const roles = ref<RoleInfo[]>([])
  const permissions = ref<string[]>([])

  const authStore = useAuthStore()

  // 是否管理员
  const isAdmin = computed(() => {
    return roles.value.some(r => r.code === 'admin')
  })

  // 是否普通用户
  const isUser = computed(() => {
    return !isAdmin.value
  })

  // 权限检查
  function hasPermission(permission: string): boolean {
    if (isAdmin.value) return true
    return permissions.value.includes(permission)
  }

  // 角色检查
  function hasRole(roleCode: string): boolean {
    return roles.value.some(r => r.code === roleCode)
  }

  // 批量权限检查（任一满足）
  function hasAnyPermission(perms: string[]): boolean {
    if (isAdmin.value) return true
    return perms.some(p => permissions.value.includes(p))
  }

  // 初始化角色权限
  function setRoles(newRoles: RoleInfo[]) {
    roles.value = newRoles
    // 汇总所有权限码
    const permSet = new Set<string>()
    newRoles.forEach(role => {
      role.permissions?.forEach(p => permSet.add(p))
    })
    permissions.value = Array.from(permSet)
  }

  // 清除
  function clear() {
    roles.value = []
    permissions.value = []
  }

  return {
    roles,
    permissions,
    isAdmin,
    isUser,
    hasPermission,
    hasRole,
    hasAnyPermission,
    setRoles,
    clear,
  }
})
```

#### 6.1.2 修改 `frontend/src/stores/auth.ts`

登录成功后初始化权限信息：

```typescript
// 在 login 成功后增加：
import { usePermissionStore } from './permission'

// login flow
async function afterLogin(response: LoginResponse) {
  setUser(response.user)
  setToken(response.token)
  
  // 新增：初始化权限
  const permStore = usePermissionStore()
  permStore.setRoles(response.user.roles || [])
}

// logout 增加清理
function logout() {
  // ... 现有清理 ...
  const permStore = usePermissionStore()
  permStore.clear()
}
```

### 6.2 路由权限守卫

#### 6.2.1 路由 meta 扩展

```typescript
// frontend/src/router/index.ts

// 管理员页面路由
{
  path: '/platform/admin',
  name: 'admin',
  component: () => import('@/views/admin/AdminLayout.vue'),
  meta: { requiresAuth: true, requiresInit: true, requiresAdmin: true },
  children: [
    {
      path: 'departments',
      name: 'departmentManage',
      component: () => import('@/views/admin/DepartmentManage.vue'),
      meta: { requiresAuth: true, requiresAdmin: true },
    },
    {
      path: 'members',
      name: 'memberManage',
      component: () => import('@/views/admin/MemberManage.vue'),
      meta: { requiresAuth: true, requiresAdmin: true },
    },
    {
      path: 'roles',
      name: 'roleManage',
      component: () => import('@/views/admin/RoleManage.vue'),
      meta: { requiresAuth: true, requiresAdmin: true },
    },
    {
      path: 'kb-permissions',
      name: 'kbPermissionManage',
      component: () => import('@/views/admin/KBPermissionManage.vue'),
      meta: { requiresAuth: true, requiresAdmin: true },
    },
  ]
}
```

#### 6.2.2 路由守卫增强

```typescript
router.beforeEach(async (to, from, next) => {
  const authStore = useAuthStore()
  const permStore = usePermissionStore()

  // 免登录页面
  if (to.meta.requiresAuth === false) {
    next()
    return
  }

  // 未登录跳转
  if (!authStore.isLoggedIn) {
    next({ name: 'login', query: { redirect: to.fullPath } })
    return
  }

  // 管理员页面权限检查
  if (to.meta.requiresAdmin && !permStore.isAdmin) {
    next({ name: 'knowledgeBaseList' }) // 重定向到知识库列表
    return
  }

  next()
})
```

### 6.3 管理员与普通用户页面差异

#### 6.3.1 侧边栏菜单差异

修改 `frontend/src/components/menu.vue`，根据角色动态渲染菜单：

```html
<template>
  <div class="sidebar">
    <!-- Logo -->
    <div class="logo" @click="goHome">...</div>

    <!-- 租户选择器（跨租户管理员） -->
    <TenantSelector v-if="canAccessAllTenants" />

    <!-- 快捷操作区 -->
    <div class="quick-actions">
      <!-- 所有用户：创建知识库按钮（user 创建本部门公开/私有，admin 创建任意） -->
      <button @click="createKB">+ 创建知识库</button>
      <!-- 所有用户：创建 Agent -->
      <button @click="createAgent">+ 创建智能体</button>
    </div>

    <!-- 菜单项 -->
    <nav class="menu-items">
      <MenuItem icon="knowledge" label="知识库" to="/platform/knowledge-bases" />
      <MenuItem icon="agent" label="智能体" to="/platform/agents" />
      <MenuItem icon="chat" label="对话" to="/platform/creatChat" />
      
      <!-- 管理员专属菜单组 -->
      <template v-if="permStore.isAdmin">
        <div class="menu-group-title">系统管理</div>
        <MenuItem icon="department" label="部门管理" to="/platform/admin/departments" />
        <MenuItem icon="member" label="人员管理" to="/platform/admin/members" />
        <MenuItem icon="permission" label="权限管理" to="/platform/admin/kb-permissions" />
        <MenuItem icon="settings" label="系统设置" to="/platform/settings" />
      </template>
    </nav>

    <!-- 用户菜单（底部） -->
    <UserMenu />
  </div>
</template>

<script setup lang="ts">
import { usePermissionStore } from '@/stores/permission'
const permStore = usePermissionStore()
</script>
```

#### 6.3.2 页面元素显隐差异总览

| 页面/组件 | 元素 | admin | user |
|-----------|------|-------|------|
| **侧边栏** | 创建知识库按钮 | ✅ | ✅（创建时需选择部门+可见性） |
| **侧边栏** | 创建智能体按钮 | ✅ | ✅（创建属于自己的） |
| **侧边栏** | 智能体菜单 | ✅ | ✅（可查看所有、编辑仅自己的） |
| **侧边栏** | 系统管理菜单组 | ✅ | ❌ |
| **侧边栏** | 系统设置 | ✅ | ❌ |
| **知识库列表** | 创建知识库卡片 | ✅ | ✅ |
| **知识库列表** | 知识库删除按钮 | ✅ 全部 | ✅ 仅自己创建的显示 |
| **知识库列表** | 知识库设置按钮 | ✅ 全部 | ✅ 仅自己创建的显示 |
| **知识库列表** | 可见性标签（公开/私有） | ✅ | ✅ |
| **知识库详情** | 上传文档按钮 | ✅ | ✅（自己创建的 + write 权限） |
| **知识库详情** | 删除文档按钮 | ✅ | ✅（自己创建的 + write 权限） |
| **知识库详情** | 知识库配置 | ✅ | ✅ 仅自己创建的显示 |
| **智能体列表** | 编辑/删除按钮 | ✅ 全部 | ✅ 仅自己创建的显示 |
| **UserMenu** | 模型管理 | ✅ | ❌ |
| **UserMenu** | MCP 设置 | ✅ | ❌ |
| **对话页** | 完全可用 | ✅ | ✅ |

### 6.4 权限指令与组件

#### 6.4.1 自定义指令 `v-permission`

新增 `frontend/src/directives/permission.ts`：

```typescript
import type { Directive, DirectiveBinding } from 'vue'
import { usePermissionStore } from '@/stores/permission'

export const vPermission: Directive = {
  mounted(el: HTMLElement, binding: DirectiveBinding) {
    const permStore = usePermissionStore()
    const { value } = binding

    if (typeof value === 'string') {
      // 单个权限检查
      if (!permStore.hasPermission(value)) {
        el.parentNode?.removeChild(el)
      }
    } else if (Array.isArray(value)) {
      // 多个权限（任一满足）
      if (!permStore.hasAnyPermission(value)) {
        el.parentNode?.removeChild(el)
      }
    }
  },
}

export const vRole: Directive = {
  mounted(el: HTMLElement, binding: DirectiveBinding) {
    const permStore = usePermissionStore()
    const { value } = binding

    if (typeof value === 'string') {
      if (!permStore.hasRole(value)) {
        el.parentNode?.removeChild(el)
      }
    } else if (Array.isArray(value)) {
      if (!value.some(r => permStore.hasRole(r))) {
        el.parentNode?.removeChild(el)
      }
    }
  },
}
```

**注册指令**（`frontend/src/main.ts`）：

```typescript
import { vPermission, vRole } from '@/directives/permission'

app.directive('permission', vPermission)
app.directive('role', vRole)
```

**使用示例**：

```html
<!-- 仅管理员可见 -->
<t-button v-role="'admin'" @click="openSystemSettings">系统设置</t-button>

<!-- 需要特定权限 -->
<t-button v-permission="'knowledgebase:delete'" @click="deleteKB">删除</t-button>

<!-- 所有权校验：所有用户可见，但 Handler 层校验是否为自己创建的 -->
<!-- 建议使用计算属性结合 created_by 判断显隐 -->
<t-button v-if="isAdmin || kb.created_by === currentUser.id" @click="deleteKB(kb)">删除</t-button>
<t-button v-if="isAdmin || agent.created_by === currentUser.id" @click="editAgent(agent)">编辑</t-button>

<!-- 知识库创建弹窗中按角色显示不同选项 -->
<t-select v-model="form.visibility" v-if="!isAdmin">
  <t-option value="public">部门公开</t-option>
  <t-option value="private">仅自己可见</t-option>
</t-select>
```

#### 6.4.2 权限包装组件 `<PermissionGuard>`

新增 `frontend/src/components/PermissionGuard.vue`：

```vue
<template>
  <slot v-if="hasAccess" />
  <slot v-else name="fallback" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { usePermissionStore } from '@/stores/permission'

const props = defineProps<{
  permission?: string | string[]
  role?: string | string[]
}>()

const permStore = usePermissionStore()

const hasAccess = computed(() => {
  if (props.role) {
    const roles = Array.isArray(props.role) ? props.role : [props.role]
    return roles.some(r => permStore.hasRole(r))
  }
  if (props.permission) {
    const perms = Array.isArray(props.permission) ? props.permission : [props.permission]
    return permStore.hasAnyPermission(perms)
  }
  return true
})
</script>
```

### 6.5 新增页面与组件

#### 6.5.1 文件清单

```
frontend/src/
├── api/
│   ├── department/
│   │   └── index.ts          # 部门管理 API
│   ├── member/
│   │   └── index.ts          # 人员管理 API
│   ├── role/
│   │   └── index.ts          # 角色管理 API
│   └── kb-permission/
│       └── index.ts          # 知识库权限 API
├── components/
│   └── PermissionGuard.vue   # 权限包装组件
├── directives/
│   └── permission.ts         # v-permission / v-role 指令
├── stores/
│   └── permission.ts         # 权限状态管理 Store
└── views/
    └── admin/
        ├── AdminLayout.vue       # 管理页面布局
        ├── DepartmentManage.vue  # 部门管理页
        ├── MemberManage.vue      # 人员管理页
        ├── RoleManage.vue        # 角色管理页
        ├── KBPermissionManage.vue# 知识库权限页
        └── components/
            ├── DepartmentTree.vue       # 部门树组件
            ├── DepartmentEditor.vue     # 部门编辑弹窗
            ├── MemberEditor.vue         # 人员编辑弹窗
            ├── MemberRoleDialog.vue     # 角色分配弹窗
            ├── MemberDeptDialog.vue     # 部门分配弹窗
            ├── KBPermEditor.vue         # 知识库权限编辑器
            └── GranteeSelector.vue      # 授权对象选择器（部门/人员）
```

#### 6.5.2 页面设计说明

##### 部门管理页（DepartmentManage.vue）

```
┌──────────────────────────────────────────────────────────┐
│  部门管理                                    [+ 创建部门] │
├──────────────┬───────────────────────────────────────────┤
│              │                                           │
│  部门树      │  部门详情 / 人员列表                       │
│  (左侧)     │  (右侧)                                   │
│              │                                           │
│  ├ 技术部    │  部门名称: 前端组                          │
│  │ ├ 前端组  │  部门编码: FE                              │
│  │ ├ 后端组  │  负责人: 张三                              │
│  │ └ AI组    │  人数: 8                                   │
│  ├ 产品部    │                                           │
│  └ 运营部    │  ┌──────────────────────────────────────┐ │
│              │  │  部门成员列表                   [+添加]│ │
│              │  │  姓名 | 邮箱 | 职位 | 操作          │ │
│              │  │  张三  zhang@ 前端   [移除]         │ │
│              │  │  李四  lisi@  前端   [移除]         │ │
│              │  └──────────────────────────────────────┘ │
└──────────────┴───────────────────────────────────────────┘
```

##### 人员管理页（MemberManage.vue）

```
┌────────────────────────────────────────────────────────────┐
│  人员管理                                [+ 添加人员] [导出]│
├────────────────────────────────────────────────────────────┤
│  搜索: [________] 部门: [全部 ▾] 角色: [全部 ▾] 状态: [全部]│
├────────────────────────────────────────────────────────────┤
│  □ | 姓名     | 邮箱           | 部门   | 角色   | 状态 | 操│
│  ─────────────────────────────────────────────────────────│
│  □ | 张三     | zhang@xx.com   | 技术部 | 管理员 | 正常 |编删│
│  □ | 李四     | lisi@xx.com    | 产品部 | 普通   | 正常 |编删│
│  □ | 王五     | wang@xx.com    | 运营部 | 普通   | 禁用 |编删│
├────────────────────────────────────────────────────────────┤
│  共 50 条  < 1 2 3 ... 5 >       每页 [20 ▾]               │
└────────────────────────────────────────────────────────────┘
```

##### 知识库权限管理页（KBPermissionManage.vue）

```
┌──────────────────────────────────────────────────────────────┐
│  知识库权限管理                                               │
├────────────────┬─────────────────────────────────────────────┤
│                │                                             │
│  知识库列表    │  权限配置                                    │
│  (左侧)      │  (右侧)                                     │
│                │                                             │
│  ○ 产品知识库  │  产品知识库 - 权限设置            [+ 添加授权]│
│  ○ 技术文档库  │                                             │
│  ● FAQ知识库   │  ┌───────────────────────────────────────┐ │
│                │  │ 类型 | 授权对象 | 权限级别 | 操作     │ │
│                │  │ 部门 | 技术部   | 读写     | [编辑][删]│ │
│                │  │ 部门 | 产品部   | 只读     | [编辑][删]│ │
│                │  │ 用户 | 李四     | 读写     | [编辑][删]│ │
│                │  └───────────────────────────────────────┘ │
└────────────────┴─────────────────────────────────────────────┘
```

### 6.6 API 服务层

#### 6.6.1 `frontend/src/api/department/index.ts`

```typescript
import { get, post, put, del } from '@/utils/request'

export interface Department {
  id: string
  tenant_id: number
  parent_id: string
  name: string
  code: string
  description: string
  sort_order: number
  leader_user_id: string
  leader_name?: string
  user_count?: number
  status: string
  level: number
  children?: Department[]
  created_at: string
  updated_at: string
}

export interface CreateDepartmentReq {
  name: string
  code?: string
  parent_id?: string
  description?: string
  sort_order?: number
  leader_user_id?: string
}

export interface UpdateDepartmentReq {
  name?: string
  code?: string
  parent_id?: string
  description?: string
  sort_order?: number
  leader_user_id?: string
  status?: string
}

// 部门管理 API
export const getDepartmentTree = () => get('/api/v1/departments/tree')
export const getDepartments = (params?: any) => get('/api/v1/departments', params)
export const getDepartment = (id: string) => get(`/api/v1/departments/${id}`)
export const createDepartment = (data: CreateDepartmentReq) => post('/api/v1/departments', data)
export const updateDepartment = (id: string, data: UpdateDepartmentReq) => put(`/api/v1/departments/${id}`, data)
export const deleteDepartment = (id: string, force = false) => del(`/api/v1/departments/${id}?force=${force}`)
export const getDepartmentUsers = (id: string, params?: any) => get(`/api/v1/departments/${id}/users`, params)
export const addDepartmentUsers = (id: string, userIds: string[]) => post(`/api/v1/departments/${id}/users`, { user_ids: userIds })
export const removeDepartmentUsers = (id: string, userIds: string[]) => del(`/api/v1/departments/${id}/users`, { user_ids: userIds })
```

#### 6.6.2 `frontend/src/api/member/index.ts`

```typescript
import { get, post, put, del } from '@/utils/request'

export interface MemberInfo {
  id: string
  username: string
  email: string
  phone: string
  position: string
  employee_no: string
  avatar: string
  is_active: boolean
  roles: { id: string; code: string; name: string }[]
  departments: { id: string; name: string; is_primary: boolean }[]
  created_at: string
}

export interface CreateMemberReq {
  username: string
  email: string
  password: string
  phone?: string
  position?: string
  employee_no?: string
  role_codes: string[]
  department_ids?: string[]
}

export interface MemberListParams {
  page?: number
  page_size?: number
  keyword?: string
  department_id?: string
  role_code?: string
  status?: string
}

// 人员管理 API
export const getMembers = (params: MemberListParams) => get('/api/v1/members', params)
export const getMember = (id: string) => get(`/api/v1/members/${id}`)
export const createMember = (data: CreateMemberReq) => post('/api/v1/members', data)
export const updateMember = (id: string, data: any) => put(`/api/v1/members/${id}`, data)
export const deleteMember = (id: string) => del(`/api/v1/members/${id}`)
export const setMemberRoles = (id: string, roleCodes: string[]) => put(`/api/v1/members/${id}/roles`, { role_codes: roleCodes })
export const setMemberDepts = (id: string, deptIds: string[], primaryDeptId?: string) =>
  put(`/api/v1/members/${id}/departments`, { department_ids: deptIds, primary_department_id: primaryDeptId })
export const resetMemberPassword = (id: string) => post(`/api/v1/members/${id}/reset-password`)
```

#### 6.6.3 `frontend/src/api/kb-permission/index.ts`

```typescript
import { get, post, del } from '@/utils/request'

export interface KBPermission {
  id: string
  grantee_type: 'department' | 'user'
  grantee_id: string
  grantee_name: string
  permission_level: 'read' | 'write' | 'manage'
  granted_by: string
  granted_by_name: string
  created_at: string
}

export interface SetKBPermReq {
  permissions: {
    grantee_type: 'department' | 'user'
    grantee_id: string
    permission_level: 'read' | 'write' | 'manage'
  }[]
}

// 知识库权限 API
export const getKBPermissions = (kbId: string) => get(`/api/v1/knowledge-bases/${kbId}/permissions`)
export const setKBPermissions = (kbId: string, data: SetKBPermReq) => post(`/api/v1/knowledge-bases/${kbId}/permissions`, data)
export const removeKBPermissions = (kbId: string, permIds: string[]) => del(`/api/v1/knowledge-bases/${kbId}/permissions`, { permission_ids: permIds })
export const checkKBPermission = (kbId: string) => get(`/api/v1/knowledge-bases/${kbId}/permissions/check`)
```

---

## 7. 实施计划与里程碑

### 阶段一：基础设施（预计 3-4 天）

| 任务 | 说明 | 产出 |
|------|------|------|
| 1.1 数据库迁移 | 编写并执行 `000012_rbac_departments` 迁移脚本（含 knowledge_bases 新字段） | SQL 迁移文件 |
| 1.2 Go 类型定义 | 新增 Department、Role、KBPermission；扩展 KnowledgeBase（visibility/created_by/department_id） | `internal/types/*.go` |
| 1.3 接口定义 | 新增 Repository/Service 接口 | `internal/types/interfaces/*.go` |
| 1.4 Repository 实现 | 实现 Department、Role、KBPermission 的数据访问层 | `internal/application/repository/*.go` |
| 1.5 DI 注册 | 在 container 注册新的依赖 | `internal/container/container.go` |

### 阶段二：后端核心逻辑（预计 5-6 天）

| 任务 | 说明 | 产出 |
|------|------|------|
| 2.1 RBAC 中间件 | 实现 `RequireRole`、`RequirePermission`、`RequireKBAccess` | `internal/middleware/rbac.go` |
| 2.2 部门 Service | 部门树构建、路径计算、级联删除 | `internal/application/service/department.go` |
| 2.3 角色 Service | 角色管理、系统角色初始化 | `internal/application/service/role.go` |
| 2.4 人员 Service | 用户管理增强、角色分配、部门分配 | `internal/application/service/member.go` |
| 2.5 知识库权限 Service | 基于可见性+部门归属+精细授权的过滤逻辑 | `internal/application/service/kb_permission.go` |
| 2.6 修改用户注册流程 | 注册时初始化系统角色 + 分配 admin | `internal/application/service/user.go` |
| 2.7 修改知识库 Service | 创建时设置 created_by/visibility/department_id | `internal/application/service/knowledgebase.go` |
| 2.8 修改 Agent Service | 创建时设置 created_by，编辑/删除时校验所有权 | `internal/application/service/custom_agent.go` |
| 2.9 修改知识库 Handler | 列表权限过滤、删除所有权校验、创建字段设置 | `internal/handler/knowledgebase.go` |
| 2.10 修改 Auth Handler | `/auth/me` 接口返回角色信息 | `internal/handler/auth.go` |

### 阶段三：后端 API（预计 3-4 天）

| 任务 | 说明 | 产出 |
|------|------|------|
| 3.1 部门管理 Handler | 全部端点实现 + 路由注册 | `internal/handler/department.go` |
| 3.2 人员管理 Handler | 全部端点实现 + 路由注册 | `internal/handler/member.go` |
| 3.3 角色管理 Handler | 全部端点实现 + 路由注册 | `internal/handler/role.go` |
| 3.4 知识库权限 Handler | 全部端点实现 + 路由注册 | `internal/handler/kb_permission.go` |
| 3.5 路由注册 | 管理员路由组 + 权限中间件绑定 | `internal/router/router.go` |
| 3.6 Swagger 文档更新 | 新增 API 的 Swagger 注解 | `docs/swagger.*` |

### 阶段四：前端实现（预计 5-7 天）

| 任务 | 说明 | 产出 |
|------|------|------|
| 4.1 权限 Store | Pinia 状态管理 | `stores/permission.ts` |
| 4.2 权限指令 | `v-permission`、`v-role` | `directives/permission.ts` |
| 4.3 权限组件 | `PermissionGuard.vue` | `components/PermissionGuard.vue` |
| 4.4 API 服务层 | department、member、role、kb-permission | `api/**/*.ts` |
| 4.5 路由改造 | 新增管理页路由 + 守卫增强 | `router/index.ts` |
| 4.6 侧边栏改造 | 知识库/智能体创建对所有用户开放；系统管理仅 admin | `components/menu.vue` |
| 4.7 部门管理页 | 部门树 + CRUD | `views/admin/DepartmentManage.vue` |
| 4.8 人员管理页 | 列表+搜索+CRUD+分配角色/部门 | `views/admin/MemberManage.vue` |
| 4.9 知识库权限页 | 权限配置界面 | `views/admin/KBPermissionManage.vue` |
| 4.10 知识库创建弹窗改造 | 新增可见性选择器、部门选择器 | 现有 knowledge 相关组件 |
| 4.11 知识库列表/详情改造 | 基于所有权显示编辑/删除按钮；显示可见性标签 | 现有 view |
| 4.12 智能体列表改造 | 基于所有权显示编辑/删除按钮 | 现有 agent view |
| 4.13 国际化 | 新增 i18n key | `locales/*.ts` |

### 阶段五：测试与优化（预计 2-3 天）

| 任务 | 说明 |
|------|------|
| 5.1 单元测试 | Repository、Service 层关键逻辑测试 |
| 5.2 集成测试 | API 端对端测试 |
| 5.3 权限穿透测试 | 验证普通用户无法越权访问，仅能操作自己创建的资源 |
| 5.4 知识库可见性测试 | 验证公开/私有知识库对不同部门用户的可见性 |
| 5.5 Agent 所有权测试 | 验证用户只能 CRUD 自己创建的 Agent |
| 5.6 数据迁移验证 | 验证存量数据迁移正确性（现有 KB 默认 visibility=public） |
| 5.7 性能优化 | 权限查询缓存、SQL 优化 |

**总预计工时：19-26 天**

---

## 8. 附录

### 8.1 权限码清单

| 资源 | 操作 | 权限码 | admin | user |
|------|------|--------|-------|------|
| 部门 | 创建 | `department:create` | ✅ | ❌ |
| 部门 | 查看 | `department:read` | ✅ | ❌ |
| 部门 | 更新 | `department:update` | ✅ | ❌ |
| 部门 | 删除 | `department:delete` | ✅ | ❌ |
| 用户 | 创建 | `user:create` | ✅ | ❌ |
| 用户 | 查看 | `user:read` | ✅ | ❌ |
| 用户 | 更新 | `user:update` | ✅ | ❌ |
| 用户 | 删除 | `user:delete` | ✅ | ❌ |
| 知识库 | 创建 | `knowledgebase:create` | ✅ | ✅ 本部门（公开/私有） |
| 知识库 | 查看 | `knowledgebase:read` | ✅ | ✅ 本部门公开+自己私有+被授权 |
| 知识库 | 更新 | `knowledgebase:update` | ✅ | ✅ 仅自己创建的 |
| 知识库 | 删除 | `knowledgebase:delete` | ✅ | ✅ 仅自己创建的 |
| 知识 | 创建 | `knowledge:create` | ✅ | ✅ |
| 知识 | 查看 | `knowledge:read` | ✅ | ✅ |
| 知识 | 更新 | `knowledge:update` | ✅ | ✅ |
| 知识 | 删除 | `knowledge:delete` | ✅ | ❌ |
| 智能体 | 创建 | `agent:create` | ✅ | ✅ 属于自己的 |
| 智能体 | 查看 | `agent:read` | ✅ | ✅ 所有（用于对话选择） |
| 智能体 | 更新 | `agent:update` | ✅ | ✅ 仅自己创建的 |
| 智能体 | 删除 | `agent:delete` | ✅ | ✅ 仅自己创建的 |
| 模型 | 创建 | `model:create` | ✅ | ❌ |
| 模型 | 查看 | `model:read` | ✅ | ✅ |
| 模型 | 更新 | `model:update` | ✅ | ❌ |
| 模型 | 删除 | `model:delete` | ✅ | ❌ |
| 会话 | 创建 | `session:create` | ✅ | ✅ |
| 会话 | 查看 | `session:read` | ✅ | ✅ |
| 会话 | 更新 | `session:update` | ✅ | ✅ |
| 会话 | 删除 | `session:delete` | ✅ | ✅ |
| MCP | 创建 | `mcp:create` | ✅ | ❌ |
| MCP | 查看 | `mcp:read` | ✅ | ❌ |
| MCP | 更新 | `mcp:update` | ✅ | ❌ |
| MCP | 删除 | `mcp:delete` | ✅ | ❌ |
| 设置 | 查看 | `settings:read` | ✅ | ❌ |
| 设置 | 更新 | `settings:update` | ✅ | ❌ |
| KB权限 | 查看 | `kb_permission:read` | ✅ | ❌ |
| KB权限 | 更新 | `kb_permission:update` | ✅ | ❌ |

> **注意**：普通用户的 `knowledgebase:create/update/delete` 和 `agent:create/update/delete` 权限均受所有权约束：
> - 知识库：仅能对自己创建的知识库执行更新/删除，创建时需指定本部门和可见性
> - 智能体：仅能对自己创建的 Agent 执行更新/删除

### 8.2 数据字典

#### departments（部门表）

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | VARCHAR(36) | 是 | UUID 主键 |
| tenant_id | INTEGER | 是 | 租户ID，外键 |
| parent_id | VARCHAR(36) | 否 | 父部门ID，NULL 为顶级部门 |
| name | VARCHAR(128) | 是 | 部门名称 |
| code | VARCHAR(64) | 否 | 部门编码（租户内唯一） |
| description | TEXT | 否 | 描述 |
| sort_order | INTEGER | 否 | 排序号（默认 0） |
| leader_user_id | VARCHAR(36) | 否 | 部门负责人用户ID |
| status | VARCHAR(20) | 否 | 状态：active/disabled |
| path | TEXT | 否 | 物化路径（`/parent_id/.../self_id`） |
| level | INTEGER | 否 | 层级深度（1=顶级） |
| created_at | TIMESTAMPTZ | 是 | 创建时间 |
| updated_at | TIMESTAMPTZ | 是 | 更新时间 |
| deleted_at | TIMESTAMPTZ | 否 | 软删除时间 |

#### roles（角色表）

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | VARCHAR(36) | 是 | UUID 主键 |
| tenant_id | INTEGER | 是 | 租户ID，外键 |
| code | VARCHAR(32) | 是 | 角色编码（admin/user，租户内唯一） |
| name | VARCHAR(64) | 是 | 角色显示名称 |
| description | TEXT | 否 | 描述 |
| is_system | BOOLEAN | 否 | 系统预置角色（不可删除） |
| permissions | JSONB | 否 | 权限码列表 JSON 数组 |
| created_at | TIMESTAMPTZ | 是 | 创建时间 |
| updated_at | TIMESTAMPTZ | 是 | 更新时间 |
| deleted_at | TIMESTAMPTZ | 否 | 软删除时间 |

#### user_roles（用户-角色关联表）

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | VARCHAR(36) | 是 | UUID 主键 |
| user_id | VARCHAR(36) | 是 | 用户ID，外键 |
| role_id | VARCHAR(36) | 是 | 角色ID，外键 |
| tenant_id | INTEGER | 是 | 租户ID，外键 |
| created_at | TIMESTAMPTZ | 是 | 创建时间 |

> 唯一约束：(user_id, role_id, tenant_id)

#### user_departments（用户-部门关联表）

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | VARCHAR(36) | 是 | UUID 主键 |
| user_id | VARCHAR(36) | 是 | 用户ID，外键 |
| department_id | VARCHAR(36) | 是 | 部门ID，外键 |
| tenant_id | INTEGER | 是 | 租户ID，外键 |
| is_primary | BOOLEAN | 否 | 是否主部门（默认 false） |
| created_at | TIMESTAMPTZ | 是 | 创建时间 |

> 唯一约束：(user_id, department_id, tenant_id)

#### kb_permissions（知识库权限授权表）

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | VARCHAR(36) | 是 | UUID 主键 |
| knowledge_base_id | VARCHAR(36) | 是 | 知识库ID，外键 |
| tenant_id | INTEGER | 是 | 租户ID，外键 |
| grantee_type | VARCHAR(20) | 是 | 授权对象类型：department/user |
| grantee_id | VARCHAR(36) | 是 | 授权对象ID |
| permission_level | VARCHAR(20) | 是 | 权限级别：read/write/manage |
| granted_by | VARCHAR(36) | 否 | 授权操作人 |
| created_at | TIMESTAMPTZ | 是 | 创建时间 |
| updated_at | TIMESTAMPTZ | 是 | 更新时间 |

> 唯一约束：(knowledge_base_id, grantee_type, grantee_id)

#### knowledge_bases 新增字段

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| created_by | VARCHAR(36) | 否 | 创建者用户ID，用于所有权判定 |
| visibility | VARCHAR(20) | 否 | 可见性：public（部门公开，默认）/ private（仅创建者可见） |
| department_id | VARCHAR(36) | 否 | 所属部门ID，公开知识库归属的部门 |

#### custom_agents 已有字段补充说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| created_by | VARCHAR(36) | 否 | 创建者用户ID（已存在未使用，本次改造后启用） |

> **重要**：存量数据迁移时，现有知识库的 `visibility` 默认设为 `public`，`created_by` 和 `department_id` 为空字符串，管理员可在迁移后手动补充。

---

> **文档结束**  
> 本文档为 WeKnora 权限管理系统改造的完整技术设计，包含数据库设计、后端架构、前端架构及实施计划。具体实现中可根据实际情况进行适当调整。
