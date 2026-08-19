<template>
  <div class="member-table">
    <div v-if="loading" class="table-loading">
      <t-loading />
    </div>
    <div v-else-if="members.length === 0 && inheritedAdmins.length === 0" class="table-empty">
      <p>{{ $t('admin.member.noMembers') }}</p>
    </div>
    <div v-else class="table-content">
      <!-- Inherited admins section (from ancestor orgs, collapsed by default) -->
      <div v-if="inheritedAdmins.length > 0" class="member-section inherited-section">
        <div class="section-header clickable" @click="showInherited = !showInherited">
          <t-icon :name="showInherited ? 'chevron-down' : 'chevron-right'" size="14px" />
          <t-icon name="secured" size="14px" style="margin-left: 4px" />
          <span style="margin-left: 4px">{{ $t('admin.member.inheritedAdmin') }} ({{ inheritedAdmins.length }})</span>
          <span class="section-hint">{{ $t('admin.member.inheritedAdminHint') }}</span>
        </div>
        <template v-if="showInherited">
          <div v-for="admin in inheritedAdmins" :key="admin.user_id" class="table-row inherited-row">
            <span class="col-user">
              <t-icon name="user-circle" class="user-icon" />
              {{ admin.username }}
            </span>
            <span class="col-email">{{ admin.email }}</span>
            <span class="col-phone">-</span>
            <span class="col-role">
              <t-tag :theme="memberRoleTheme(admin)" variant="light" size="small">
                {{ displayRoleLabel(admin) }}
              </t-tag>
              <t-tag theme="default" variant="light" size="small" style="margin-left: 4px">
                {{ $t('admin.member.inheritedFrom') }}: {{ admin.from_org_name }}
              </t-tag>
            </span>
            <span class="col-actions"></span>
          </div>
        </template>
      </div>

      <!-- Direct members section -->
      <div class="table-header">
        <span class="col-user">{{ $t('admin.member.username') }}</span>
        <span class="col-email">{{ $t('admin.member.email') }}</span>
        <span class="col-phone">{{ $t('admin.member.phone') }}</span>
        <span class="col-role">{{ $t('admin.member.role') }}</span>
        <span class="col-actions">{{ $t('admin.member.actions') }}</span>
      </div>
      <div v-for="member in members" :key="member.user_id" class="table-row">
        <span class="col-user">
          <t-icon name="user-circle" class="user-icon" />
          {{ member.username }}
        </span>
        <span class="col-email">{{ member.email }}</span>
        <span class="col-phone">{{ member.phone || '-' }}</span>
        <span class="col-role">
          <t-tag v-if="member.is_super_admin" theme="warning" variant="light" size="small">
            {{ $t('admin.member.superAdmin') }}
          </t-tag>
          <t-tag v-else-if="member.is_system_admin" theme="warning" variant="light" size="small">
          {{ $t('admin.member.systemAdmin') }}
          </t-tag>
          <t-tag v-else :theme="memberRoleTheme(member)" variant="light" size="small">
            {{ displayRoleLabel(member) }}
          </t-tag>
        </span>
        <span class="col-actions">
          <t-button
            size="small"
            variant="text"
            theme="primary"
            :disabled="isProjectedRootAdmin(member)"
            :title="isProjectedRootAdmin(member) ? $t('admin.member.adjustInTenantMembers') : ''"
            @click="$emit('edit', member)"
          >
            {{ $t('common.edit') }}
          </t-button>
          <t-button
            v-if="authStore.isSuperAdmin"
            size="small"
            variant="text"
            theme="primary"
            @click="$emit('resetPassword', member)"
          >
            {{ $t('admin.member.resetPassword') }}
          </t-button>
          <t-button
            size="small"
            variant="text"
            :theme="member.is_admin ? 'default' : 'primary'"
            :disabled="member.is_owner === true || isProjectedRootAdmin(member)"
            :title="isProjectedRootAdmin(member) ? $t('admin.member.adjustInTenantMembers') : ''"
            @click="$emit('setAdmin', member.user_id, !member.is_admin)"
          >
            {{ member.is_admin ? $t('admin.member.revokeAdmin') : $t('admin.member.setAdmin') }}
          </t-button>
          <t-popconfirm
            :content="$t('admin.member.removeConfirm')"
            @confirm="$emit('remove', member.user_id)"
          >
            <t-button
              size="small"
              variant="text"
              theme="danger"
              :disabled="member.user_id === authStore.currentUserId || member.is_owner === true || isProjectedRootAdmin(member)"
              :title="actionBlockedTitle(member)"
            >
              {{ $t('admin.member.remove') }}
            </t-button>
          </t-popconfirm>
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getOrgMembers, type OrgMember, type InheritedAdmin } from '@/api/org-tree'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const authStore = useAuthStore()

function displayRoleLabel(member: Pick<OrgMember, 'display_role' | 'is_owner' | 'is_admin' | 'role'> | Pick<InheritedAdmin, 'display_role' | 'role'>) {
  switch (member.display_role) {
    case 'owner':
      return t('tenantMember.role.owner')
    case 'admin':
      return t('tenantMember.role.admin')
    case 'suborg_admin':
      return t('admin.member.roleSubOrgAdmin')
    case 'collaborator':
      return t('tenantMember.role.contributor')
    case 'readonly':
      return t('tenantMember.role.viewer')
    default:
      break
  }

  if ('is_owner' in member && member.is_owner) return t('tenantMember.role.owner')
  if ('is_admin' in member && member.is_admin) return t('admin.member.roleSubOrgAdmin')
  if (!member.role) return t('admin.member.member')

  switch (member.role) {
    case 'admin':
      return t('admin.member.roleSubOrgAdmin')
    case 'editor':
      return t('tenantMember.role.contributor')
    case 'viewer':
      return t('tenantMember.role.viewer')
    default:
      return member.role
  }
}

function isProjectedRootAdmin(member: OrgMember) {
  return member.is_projected_root_admin === true || member.source === 'tenant_projection'
}

function memberRoleTheme(member: Pick<OrgMember, 'display_role' | 'is_owner' | 'is_admin'> | Pick<InheritedAdmin, 'display_role'>) {
  if (member.display_role === 'owner' || ('is_owner' in member && member.is_owner)) return 'primary'
  if (member.display_role === 'admin') return 'primary'
  if (member.display_role === 'suborg_admin' || ('is_admin' in member && member.is_admin)) return 'warning'
  if (member.display_role === 'collaborator') return 'success'
  return 'default'
}

function actionBlockedTitle(member: OrgMember) {
  if (member.user_id === authStore.currentUserId) return t('admin.member.cannotModifySelf')
  if (isProjectedRootAdmin(member)) return t('admin.member.adjustInTenantMembers')
  if (member.is_owner === true) return t('tenantMember.role.owner')
  return ''
}

const props = defineProps<{
  orgId: string
  refreshKey: number
}>()

defineEmits<{
  (e: 'remove', userId: string): void
  (e: 'setAdmin', userId: string, isAdmin: boolean): void
  (e: 'setSuperAdmin', userId: string, isSuperAdmin: boolean): void
  (e: 'edit', member: OrgMember): void
  (e: 'resetPassword', member: OrgMember): void
}>()

const members = ref<OrgMember[]>([])
const inheritedAdmins = ref<InheritedAdmin[]>([])
const loading = ref(false)
const showInherited = ref(false)

const fetchMembers = async () => {
  if (!props.orgId) return
  loading.value = true
  try {
    const res = await getOrgMembers(props.orgId)
    if (res.success && res.data) {
      members.value = res.data
      inheritedAdmins.value = res.inherited_admins || []
    } else {
      members.value = []
      inheritedAdmins.value = []
    }
  } catch {
    members.value = []
    inheritedAdmins.value = []
  } finally {
    loading.value = false
  }
}

watch(() => props.orgId, fetchMembers, { immediate: true })
watch(() => props.refreshKey, fetchMembers)
</script>

<style lang="less" scoped>
.member-table {
  flex: 1;
  overflow-y: auto;
}

.table-loading {
  display: flex;
  justify-content: center;
  padding: 48px;
}

.table-empty {
  display: flex;
  justify-content: center;
  padding: 48px;
  color: #999;
  font-size: 14px;
}

.table-content {
  flex: 1;
}

.member-section.inherited-section {
  border-bottom: 1px solid #e7e7e7;
  margin-bottom: 8px;

  .section-header {
    display: flex;
    align-items: center;
    padding: 8px 16px;
    font-size: 13px;
    color: #666;
    cursor: pointer;
    user-select: none;

    &:hover {
      background: #fafbfc;
    }

    .section-hint {
      margin-left: 8px;
      font-size: 12px;
      color: #bbb;
    }
  }

  .inherited-row {
    background: #fafbfc;
    color: #999;
  }
}

.table-header,
.table-row {
  display: flex;
  align-items: center;
  padding: 10px 16px;
  gap: 12px;
}

.table-header {
  font-size: 12px;
  color: #999;
  font-weight: 500;
  border-bottom: 1px solid #e7e7e7;
  background: #fafbfc;
}

.table-row {
  font-size: 14px;
  color: #333;
  border-bottom: 1px solid #f0f0f0;
  transition: background 0.15s;

  &:hover {
    background: #f9f9fb;
  }

  &:last-child {
    border-bottom: none;
  }
}

.col-user {
  flex: 1.5;
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  .user-icon {
    font-size: 16px;
    color: #999;
    flex-shrink: 0;
  }
}

.col-email {
  flex: 1.5;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #666;
}

.col-phone {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #666;
}

.col-role {
  flex: 1;
  min-width: 0;
}

.col-actions {
  flex: 1.5;
  display: flex;
  align-items: center;
  gap: 4px;
  justify-content: flex-end;
}
</style>
