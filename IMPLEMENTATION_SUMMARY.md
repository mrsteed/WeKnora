# 功能实现总结

##  1. 知识库显示创建者昵称

### 后端已完成 ✅
- 在 `internal/types/knowledgebase.go` 添加 `CreatedByNickname` 字段
- 在 `internal/application/service/kb_visibility.go` 添加 `fillCreatorNicknames()` 方法
- 后端 API 返回知识库列表时已包含 `created_by_nickname` 字段
- 全局知识库显示"系统"，其他显示用户名

### 前端需要修改
**文件**: `frontend/src/views/knowledge/KnowledgeBaseList.vue`

在知识库卡片的 `card-bottom` 右下角，将原来的"我的"标签替换为显示创建者昵称：

```vue
<!-- 原代码 (约第 560 行) -->
<div class="personal-source">
  <t-icon name="user" size="14px" />
  <span>{{ $t('knowledgeList.myLabel') }}</span>
</div>

<!-- 修改为 -->
<div class="personal-source">
  <t-icon name="user" size="14px" />
  <span>{{ kb.created_by_nickname || $t('knowledgeList.unknownCreator') }}</span>
</div>
```

需要在所有显示知识库卡片的地方应用此修改（约 5-6 处）。

## 2. 组织管理员显示组织人员管理菜单

### 需要修改的文件

#### 2.1 路由配置
**文件**: `frontend/src/router/index.ts`

```typescript
// 原代码 (约第 93-108 行)
{
  path: "admin",
  name: "admin",
  component: () => import("../views/admin/AdminLayout.vue"),
  meta: { requiresInit: true, requiresAuth: true, requiresSuperAdmin: true },
  redirect: "/platform/admin/org-tree",
  children: [...]
}

// 修改为：允许组织管理员访问
{
  path: "admin",
  name: "admin",
  component: () => import("../views/admin/AdminLayout.vue"),
  meta: { requiresInit: true, requiresAuth: true, requiresOrgAdmin: true }, // 改为 requiresOrgAdmin
  redirect: "/platform/admin/org-tree",
  children: [
    {
      path: "org-tree",
      name: "orgTreeManage",
      component: () => import("../views/admin/OrgTreeManage.vue"),
      meta: { requiresInit: true, requiresAuth: true, requiresOrgAdmin: true }
    },
    {
      path: "members",
      name: "memberManage",
      component: () => import("../views/admin/MemberManage.vue"),
      meta: { requiresInit: true, requiresAuth: true, requiresOrgAdmin: true }
    },
  ],
}

// 在路由守卫中添加组织管理员检查 (约第 120-165 行)
router.beforeEach(async (to, from, next) => {
  const authStore = useAuthStore()
  
  // ... 现有代码 ...

  // 检查超级管理员权限
  if (to.meta.requiresSuperAdmin) {
    if (!authStore.isSuperAdmin) {
      next('/platform/knowledge-bases')
      return
    }
  }

  // 新增：检查组织管理员权限
  if (to.meta.requiresOrgAdmin) {
    // 超级管理员或组织管理员都可以访问
    if (!authStore.isSuperAdmin && !authStore.isOrgAdmin) {
      next('/platform/knowledge-bases')
      return
    }
  }

  next()
})
```

#### 2.2 Auth Store
**文件**: `frontend/src/stores/auth.ts`

添加 `isOrgAdmin` 计算属性用于判断当前用户是否是组织管理员：

```typescript
const isOrgAdmin = computed(() => {
  // 检查用户在当前组织中是否有管理员权限
  const orgStore = useOrganizationStore()
  if (!orgStore.currentOrganization) return false
  
  // 检查当前用户在该组织中的角色
  const membership = orgStore.currentOrganization.members?.find(
    m => m.user_id === user.value?.id
  )
  return membership?.role === 'admin'
})
```

#### 2.3 菜单显示
**文件**: `frontend/src/stores/menu.ts`

修改菜单项的显示逻辑：

```typescript
// 原代码 (约第 23 行)
{ title: '', titleKey: 'menu.admin', icon: 'setting', path: 'admin', superAdminOnly: true },

// 修改为
{ title: '', titleKey: 'menu.admin', icon: 'setting', path: 'admin', orgAdminOnly: true },
```

**文件**: `frontend/src/components/menu.vue`

```typescript
//原代码 (约第 190 行)
const topMenuItems = computed<MenuItem[]>(() => {
  return (menuArr.value as unknown as MenuItem[]).filter((item: MenuItem) => {
    if (item.superAdminOnly && !authStore.isSuperAdmin) return false
    // ...
  });
});

// 修改为
const topMenuItems = computed<MenuItem[]>(() => {
  return (menuArr.value as unknown as MenuItem[]).filter((item: MenuItem) => {
    if (item.superAdminOnly && !authStore.isSuperAdmin) return false
    if (item.orgAdminOnly && !authStore.isSuperAdmin && !authStore.isOrgAdmin) return false
    // ...
  });
});
```

## 3. 限制组织管理员权限范围

### 后端需要修改

#### 3.1 组织创建权限
**文件**: `internal/handler/org_tree.go`

在 `CreateOrganization` 函数中添加权限检查：

```go
func (h *OrgTreeHandler) CreateOrganization(c *gin.Context) {
    ctx := c.Request.Context()
    
    // 获取当前用户
    userVal, _ := c.Get(types.UserContextKey.String())
    user, _ := userVal.(*types.User)
    
    var req CreateOrganizationRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.Error(apperrors.NewBadRequestError("Invalid request"))
        return
    }
    
    // 如果不是超级管理员
    if !user.IsSuperAdmin {
        // 如果 parent_id 为空，拒绝创建（不能创建根组织）
        if req.ParentID == "" {
            c.Error(apperrors.NewForbiddenError("Only super admins can create root organizations"))
            return
        }
        
        // 检查父组织是否存在，以及当前用户是否是该组织的管理员
        parentOrg, err := h.service.GetOrganization(ctx, req.ParentID)
        if err != nil {
            c.Error(apperrors.NewNotFoundError("Parent organization not found"))
            return
        }
        
        // 检查当前用户在父组织中的角色
        member, err := h.service.GetOrganizationMember(ctx, req.ParentID, user.ID)
        if err != nil || member.Role != types.OrgRoleAdmin {
            c.Error(apperrors.NewForbiddenError("You must be an admin of the parent organization"))
            return
        }
    }
    
    // 继续原有的创建逻辑...
}
```

#### 3.2 组织成员添加权限
**文件**: `internal/handler/org_tree.go`

在 `AddOrganizationMember` 函数中添加相似的权限检查：

```go
func (h *OrgTreeHandler) AddOrganizationMember(c *gin.Context) {
    ctx := c.Request.Context()
    
    orgID := c.Param("id")
    
    // 获取当前用户
    userVal, _ := c.Get(types.UserContextKey.String())
    user, _ := userVal.(*types.User)
    
    // 如果不是超级管理员，检查是否是该组织的管理员
    if !user.IsSuperAdmin {
        member, err := h.service.GetOrganizationMember(ctx, orgID, user.ID)
        if err != nil || member.Role != types.OrgRoleAdmin {
            c.Error(apperrors.NewForbiddenError("Only organization admins can add members"))
            return
        }
    }
    
    // 继续原有的添加成员逻辑...
}
```

### 前端需要修改

#### 3.3 组织树管理界面
**文件**: `frontend/src/views/admin/OrgTreeManage.vue`

- 如果用户不是超级管理员，隐藏"创建根组织"按钮
- 只显示用户有管理权限的组织及其子树
- 在组织节点上添加权限检查，只允许在有管理权限的组织下创建子组织

```vue
<!-- 创建根组织按钮 -->
<t-button 
  v-if="authStore.isSuperAdmin" 
  @click="createRootOrg"
>
  创建根组织
</t-button>

<!-- 创建子组织按钮 -->
<t-button 
  v-if="canManageOrg(org.id)" 
  @click="createChildOrg(org.id)"
>
  创建子组织
</t-button>
```

## 实施步骤

1. ✅ 后端 - 知识库创建者昵称字段和填充逻辑
2. ⏳ 前端 - 知识库卡片显示创建者昵称
3. ⏳ 前端 - 路由和菜单权限调整
4. ⏳ 后端 - 组织创建和成员管理权限限制
5. ⏳ 前端 - 组织管理界面权限适配

## 测试要点

1. 知识库创建者昵称显示
   - 全局知识库显示"系统"
   - 自己创建的知识库显示自己的用户名
   - 他人创建的知识库显示对方用户名

2. 组织管理员菜单权限
   - 超级管理员能看到并访问组织人员管理菜单
   - 组织管理员能看到并访问组织人员管理菜单
   - 普通用户看不到组织人员管理菜单

3. 组织管理员操作权限
   - 超级管理员可以创建根组织
   - 组织管理员不能创建根组织
   - 组织管理员只能在自己管理的组织下创建子组织
   - 组织管理员只能在自己管理的组织中添加成员
