<template>
  <div class="organ-manage">
    <div class="page-header">
      <h2 class="page-title">{{ $t('admin.title') }}</h2>
      <t-button
        v-if="canBootstrapRootOrg"
        theme="primary"
        @click="handleCreate(null)"
      >
        <template #icon><t-icon name="add" /></template>
        {{ $t('admin.org.createRoot') }}
      </t-button>
    </div>

    <div class="organ-layout">
      <!-- Left: Org tree (selection + edit actions) -->
      <div class="org-panel">
        <div class="panel-header">
          <span>{{ $t('admin.organ.orgPanel') }}</span>
        </div>
        <div class="org-tree-list">
          <div v-if="orgTreeStore.loading" class="tree-loading">
            <t-loading size="small" />
          </div>
          <div v-else-if="orgTreeStore.tree.length === 0" class="tree-empty">
            <t-icon name="folder-open" class="empty-icon" />
            <p>{{ $t('admin.org.emptyTree') }}</p>
            <t-button
              v-if="canBootstrapRootOrg"
              theme="primary"
              variant="outline"
              size="small"
              @click="handleCreate(null)"
            >
              {{ $t('admin.org.createFirst') }}
            </t-button>
          </div>
          <div v-else>
            <OrgTreeNodeItem
              v-for="node in orgTreeStore.tree"
              :key="node.id"
              :node="node"
              :level="0"
              :selected-id="selectedOrgId"
              @create="handleCreate"
              @edit="handleEdit"
              @delete="handleDelete"
              @move="handleMove"
              @select="handleSelectOrg"
            />
          </div>
        </div>
      </div>

      <!-- Right: Member list -->
      <div class="member-panel">
        <div v-if="!selectedOrgId" class="member-empty">
          <t-icon name="usergroup" class="empty-icon" />
          <p>{{ $t('admin.member.selectOrgHint') }}</p>
        </div>
        <template v-else>
          <div class="member-toolbar">
            <span class="member-org-name">{{ selectedOrgName }}</span>
            <div class="toolbar-buttons">
              <t-button theme="primary" size="small" @click="showCreateUserDialog = true">
                <template #icon><t-icon name="user-add" /></template>
                {{ $t('admin.member.createUser') }}
              </t-button>
              <t-button theme="default" size="small" @click="showAssignDialog = true">
                <template #icon><t-icon name="usergroup-add" /></template>
                {{ $t('admin.member.addMember') }}
              </t-button>
            </div>
          </div>
          <MemberTable
            :org-id="selectedOrgId"
            :refresh-key="memberRefreshKey"
            @remove="handleRemoveMember"
            @set-admin="handleSetAdmin"
            @set-super-admin="handleSetSuperAdmin"
            @edit="handleEditMember"
            @reset-password="handleResetPassword"
          />
        </template>
      </div>
    </div>

    <OrgTreeEditor
      v-model:visible="editorVisible"
      :mode="editorMode"
      :node="editingNode"
      :parent-id="editorParentId"
      @success="handleEditorSuccess"
    />

    <AssignOrgDialog
      v-model:visible="showAssignDialog"
      :org-id="selectedOrgId || ''"
      :org-name="selectedOrgName"
      @success="handleAssignSuccess"
    />

    <CreateUserDialog
      v-model:visible="showCreateUserDialog"
      :org-id="selectedOrgId || ''"
      :org-name="selectedOrgName"
      @success="handleCreateUserSuccess"
    />

    <EditUserDialog
      v-model:visible="showEditUserDialog"
      :org-id="selectedOrgId || ''"
      :org-name="selectedOrgName"
      :user="editingUser"
      @success="handleEditUserSuccess"
    />

    <ResetUserPasswordDialog
      v-model:visible="showResetPasswordDialog"
      :org-id="selectedOrgId || ''"
      :org-name="selectedOrgName"
      :user="editingUser"
      @success="handleEditUserSuccess"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useOrgTreeStore } from '@/stores/orgTree'
import { useAuthStore } from '@/stores/auth'
import { removeUserFromOrg, setOrgAdmin, setSuperAdmin, moveOrgTreeNode } from '@/api/org-tree'
import { useI18n } from 'vue-i18n'
import type { OrgMember, OrgTreeNode } from '@/api/org-tree'
import OrgTreeNodeItem from './components/OrgTreeNodeItem.vue'
import OrgTreeEditor from './components/OrgTreeEditor.vue'
import MemberTable from './components/MemberTable.vue'
import AssignOrgDialog from './components/AssignOrgDialog.vue'
import CreateUserDialog from './components/CreateUserDialog.vue'
import EditUserDialog from './components/EditUserDialog.vue'
import ResetUserPasswordDialog from './components/ResetUserPasswordDialog.vue'

const orgTreeStore = useOrgTreeStore()
const authStore = useAuthStore()
const { t } = useI18n()

// ---------- 组织树（选中 + 结构管理） ----------
const selectedOrgId = ref<string | null>(null)
const editorVisible = ref(false)
const editorMode = ref<'create' | 'edit'>('create')
const editingNode = ref<OrgTreeNode | null>(null)
const editorParentId = ref<string | null>(null)

const canBootstrapRootOrg = computed(
  () => authStore.isSuperAdmin || (orgTreeStore.tree.length === 0 && authStore.hasRole('admin')),
)

onMounted(() => {
  // 组织树按登录账号的权限过滤（超管看全量，子组织管理员仅看管辖子树）。
  // 不能沿用"store 有缓存就不请求"：同账号来回切换没问题，
  // 但 SPA 内登出再换账号登录时缓存仍是上一个账号的树，会显示错误的全量结构。
  orgTreeStore.fetchTree()
})

// 兜底：若 store 里残留的是上一个账号的树，用户变化时强制重新拉取并清状态。
watch(
  () => authStore.currentUserId,
  (uid, prev) => {
    if (!uid || uid === prev) return
    selectedOrgId.value = null
    orgTreeStore.clearState()
    orgTreeStore.fetchTree()
  },
)

const handleSelectOrg = (orgId: string) => {
  selectedOrgId.value = orgId
}

const handleCreate = (parentId: string | null) => {
  editorMode.value = 'create'
  editingNode.value = null
  editorParentId.value = parentId
  editorVisible.value = true
}

const handleEdit = (node: OrgTreeNode) => {
  editorMode.value = 'edit'
  editingNode.value = node
  editorParentId.value = null
  editorVisible.value = true
}

const handleDelete = async (node: OrgTreeNode) => {
  try {
    await orgTreeStore.deleteNode(node.id)
    MessagePlugin.success(t('admin.org.deleteSuccess'))
    // 删除的是当前选中组织时清空选中
    if (selectedOrgId.value === node.id) {
      selectedOrgId.value = null
    }
  } catch {
    MessagePlugin.error(t('admin.org.deleteFailed'))
  }
}

const handleEditorSuccess = () => {
  editorVisible.value = false
  // 编辑时可能变更了上级组织（内部走了 move 接口），move 不会改本地 store，
  // 因此成功关闭后统一重新拉树，保证结构与选中状态一致。
  orgTreeStore.fetchTree()
}

const handleMove = async (payload: { nodeId: string; newParentId: string | null }) => {
  try {
    const res = await moveOrgTreeNode(payload.nodeId, { new_parent_id: payload.newParentId })
    if (res.success) {
      MessagePlugin.success(t('admin.org.moveSuccess'))
      orgTreeStore.fetchTree()
    } else {
      MessagePlugin.error(res.message || t('admin.org.moveFailed'))
    }
  } catch {
    MessagePlugin.error(t('admin.org.moveFailed'))
  }
}

// ---------- 成员管理 ----------
const showAssignDialog = ref(false)
const showCreateUserDialog = ref(false)
const showEditUserDialog = ref(false)
const showResetPasswordDialog = ref(false)
const editingUser = ref<any>(null)
const memberRefreshKey = ref(0)

const selectedOrgName = computed(() => {
  if (!selectedOrgId.value) return ''
  const findNode = (nodes: OrgTreeNode[]): string => {
    for (const node of nodes) {
      if (node.id === selectedOrgId.value) return node.name
      if (node.children) {
        const found = findNode(node.children)
        if (found) return found
      }
    }
    return ''
  }
  return findNode(orgTreeStore.tree)
})

const handleRemoveMember = async (userId: string) => {
  if (!selectedOrgId.value) return
  try {
    await removeUserFromOrg(selectedOrgId.value, userId)
    MessagePlugin.success(t('admin.member.removeSuccess'))
    memberRefreshKey.value++
    orgTreeStore.fetchTree() // refresh member count
  } catch {
    MessagePlugin.error(t('admin.member.removeFailed'))
  }
}

const handleSetAdmin = async (userId: string, isAdmin: boolean) => {
  if (!selectedOrgId.value) return
  try {
    await setOrgAdmin(selectedOrgId.value, { user_id: userId, is_admin: isAdmin })
    MessagePlugin.success(t('admin.member.updateSuccess'))
    memberRefreshKey.value++
  } catch {
    MessagePlugin.error(t('common.operationFailed'))
  }
}

const handleSetSuperAdmin = async (userId: string, isSuperAdmin: boolean) => {
  try {
    const res = await setSuperAdmin(userId, isSuperAdmin)
    if (res.success) {
      MessagePlugin.success(t('admin.member.updateSuccess'))
      memberRefreshKey.value++
    } else {
      MessagePlugin.error(res.message || t('common.operationFailed'))
    }
  } catch {
    MessagePlugin.error(t('common.operationFailed'))
  }
}

const handleAssignSuccess = () => {
  memberRefreshKey.value++
  orgTreeStore.fetchTree()
}

const handleCreateUserSuccess = () => {
  memberRefreshKey.value++
  orgTreeStore.fetchTree()
}

const handleEditMember = (member: OrgMember) => {
  editingUser.value = member
  showEditUserDialog.value = true
}

const handleResetPassword = (member: OrgMember) => {
  editingUser.value = member
  showResetPasswordDialog.value = true
}

const handleEditUserSuccess = () => {
  memberRefreshKey.value++
  orgTreeStore.fetchTree()
}
</script>

<style lang="less" scoped>
.organ-manage {
  width: 100%;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;

  .page-title {
    font-size: 20px;
    font-weight: 600;
    color: #1a1a1a;
    margin: 0;
  }
}

.organ-layout {
  display: flex;
  gap: 16px;
  height: calc(100vh - 160px);
}

.org-panel {
  width: 420px;
  min-width: 420px;
  background: #fff;
  border-radius: 12px;
  border: 1px solid #e7e7e7;
  display: flex;
  flex-direction: column;
  overflow: hidden;

  .panel-header {
    padding: 14px 16px;
    border-bottom: 1px solid #e7e7e7;
    font-size: 14px;
    font-weight: 500;
    color: #333;
  }

  .org-tree-list {
    flex: 1;
    overflow-y: auto;
    padding: 8px;
  }

  .tree-loading {
    display: flex;
    justify-content: center;
    padding: 24px;
  }

  .tree-empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 40px 16px;
    color: #999;

    .empty-icon {
      font-size: 40px;
      color: #ddd;
      margin-bottom: 12px;
    }

    p {
      font-size: 14px;
      margin-bottom: 12px;
    }
  }
}

.member-panel {
  flex: 1;
  background: #fff;
  border-radius: 12px;
  border: 1px solid #e7e7e7;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.member-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: #999;

  .empty-icon {
    font-size: 48px;
    color: #ddd;
    margin-bottom: 16px;
  }

  p {
    font-size: 14px;
  }
}

.member-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 16px;
  border-bottom: 1px solid #e7e7e7;

  .member-org-name {
    font-size: 14px;
    font-weight: 600;
    color: #333;
  }

  .toolbar-buttons {
    display: flex;
    gap: 8px;
  }
}
</style>
