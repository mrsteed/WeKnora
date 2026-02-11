<template>
  <div class="member-manage">
    <div class="page-header">
      <h2 class="page-title">{{ $t('admin.memberManage') }}</h2>
    </div>

    <div class="member-layout">
      <!-- Left: Org tree selector -->
      <div class="org-selector-panel">
        <div class="panel-header">
          <span>{{ $t('admin.member.selectOrg') }}</span>
        </div>
        <div class="org-tree-list">
          <div v-if="orgTreeStore.loading" class="tree-loading">
            <t-loading size="small" />
          </div>
          <template v-else>
            <OrgTreeSelectItem
              v-for="node in orgTreeStore.tree"
              :key="node.id"
              :node="node"
              :level="0"
              :selected-id="selectedOrgId"
              @select="handleSelectOrg"
            />
          </template>
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
          />
        </template>
      </div>
    </div>

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
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useOrgTreeStore } from '@/stores/orgTree'
import { removeUserFromOrg, setOrgAdmin, setSuperAdmin } from '@/api/org-tree'
import { useI18n } from 'vue-i18n'
import type { OrgTreeNode } from '@/api/org-tree'
import OrgTreeSelectItem from './components/OrgTreeSelectItem.vue'
import MemberTable from './components/MemberTable.vue'
import AssignOrgDialog from './components/AssignOrgDialog.vue'
import CreateUserDialog from './components/CreateUserDialog.vue'
import EditUserDialog from './components/EditUserDialog.vue'

const orgTreeStore = useOrgTreeStore()
const { t } = useI18n()

const selectedOrgId = ref<string | null>(null)
const showAssignDialog = ref(false)
const showCreateUserDialog = ref(false)
const showEditUserDialog = ref(false)
const editingUser = ref<any>(null)
const memberRefreshKey = ref(0)

onMounted(() => {
  if (orgTreeStore.tree.length === 0) {
    orgTreeStore.fetchTree()
  }
})

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

const handleSelectOrg = (orgId: string) => {
  selectedOrgId.value = orgId
}

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

const handleEditMember = (member: any) => {
  editingUser.value = member
  showEditUserDialog.value = true
}

const handleEditUserSuccess = () => {
  memberRefreshKey.value++
  orgTreeStore.fetchTree()
}
</script>

<style lang="less" scoped>
.member-manage {
  max-width: 1100px;
}

.page-header {
  margin-bottom: 24px;

  .page-title {
    font-size: 20px;
    font-weight: 600;
    color: #1a1a1a;
    margin: 0;
  }
}

.member-layout {
  display: flex;
  gap: 16px;
  height: calc(100vh - 160px);
}

.org-selector-panel {
  width: 260px;
  min-width: 260px;
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
