<template>
  <div class="member-table">
    <div v-if="loading" class="table-loading">
      <t-loading />
    </div>
    <div v-else-if="members.length === 0" class="table-empty">
      <p>{{ $t('admin.member.noMembers') }}</p>
    </div>
    <div v-else class="table-content">
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
          <t-tag v-else-if="member.is_admin" theme="primary" variant="light" size="small">
            {{ $t('admin.member.admin') }}
          </t-tag>
          <t-tag v-else theme="default" variant="light" size="small">
            {{ member.role || $t('admin.member.member') }}
          </t-tag>
        </span>
        <span class="col-actions">
          <t-button
            size="small"
            variant="text"
            theme="primary"
            @click="$emit('edit', member)"
          >
            {{ $t('common.edit') }}
          </t-button>
          <t-button
            size="small"
            variant="text"
            :theme="member.is_admin ? 'default' : 'primary'"
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
              :disabled="member.user_id === authStore.currentUserId"
              :title="member.user_id === authStore.currentUserId ? $t('admin.member.cannotModifySelf') : ''"
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
import { ref, watch, onMounted } from 'vue'
import { getOrgMembers, type OrgMember } from '@/api/org-tree'
import { useAuthStore } from '@/stores/auth'

const authStore = useAuthStore()

const props = defineProps<{
  orgId: string
  refreshKey: number
}>()

defineEmits<{
  (e: 'remove', userId: string): void
  (e: 'setAdmin', userId: string, isAdmin: boolean): void
  (e: 'setSuperAdmin', userId: string, isSuperAdmin: boolean): void
  (e: 'edit', member: OrgMember): void
}>()

const members = ref<OrgMember[]>([])
const loading = ref(false)

const fetchMembers = async () => {
  if (!props.orgId) return
  loading.value = true
  try {
    const res = await getOrgMembers(props.orgId)
    if (res.success && res.data) {
      members.value = res.data
    } else {
      members.value = []
    }
  } catch {
    members.value = []
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
