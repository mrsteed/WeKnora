<template>
  <Teleport to="body">
    <Transition name="fade">
      <div v-if="visible" class="dialog-overlay" @click.self="handleClose">
        <div class="dialog-content">
          <div class="dialog-header">
            <h3>{{ $t('admin.member.addMemberTo', { org: orgName }) }}</h3>
            <t-button variant="text" shape="square" @click="handleClose">
              <t-icon name="close" />
            </t-button>
          </div>
          <div class="dialog-body">
            <div class="form-item">
              <label class="form-label required">{{ $t('admin.member.searchUser') }}</label>
              <t-input
                v-model="searchQuery"
                :placeholder="$t('admin.member.searchUserPlaceholder')"
                clearable
                @change="handleSearch"
                @enter="handleSearch"
              >
                <template #suffix-icon>
                  <t-icon name="search" />
                </template>
              </t-input>
            </div>
            <div v-if="searchResults.length > 0" class="search-results">
              <div
                v-for="user in searchResults"
                :key="user.id"
                :class="['search-result-item', { selected: selectedUserId === user.id }]"
                @click="selectedUserId = user.id"
              >
                <t-icon name="user-circle" class="result-icon" />
                <div class="result-info">
                  <span class="result-name">{{ user.username }}</span>
                  <span class="result-email">{{ user.email }}</span>
                </div>
                <t-icon v-if="selectedUserId === user.id" name="check" class="check-icon" />
              </div>
            </div>
            <div v-else-if="searched && !searching" class="no-results">
              {{ $t('admin.member.noUsersFound') }}
            </div>
            <div v-if="searching" class="searching">
              <t-loading size="small" />
            </div>
            <div class="form-item" style="margin-top: 12px;">
              <label class="form-label">{{ $t('admin.member.role') }}</label>
              <t-radio-group v-model="selectedRole">
                <t-radio value="viewer">{{ $t('admin.member.roleViewer') }}</t-radio>
                <t-radio value="editor">{{ $t('admin.member.roleEditor') }}</t-radio>
                <t-radio value="admin">{{ $t('admin.member.roleAdmin') }}</t-radio>
              </t-radio-group>
            </div>
          </div>
          <div class="dialog-footer">
            <t-button variant="outline" @click="handleClose">{{ $t('common.cancel') }}</t-button>
            <t-button theme="primary" :loading="submitting" :disabled="!selectedUserId" @click="handleSubmit">
              {{ $t('common.confirm') }}
            </t-button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { assignUserToOrg, searchUsersForAssign } from '@/api/org-tree'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  visible: boolean
  orgId: string
  orgName: string
}>()

const emit = defineEmits<{
  (e: 'update:visible', val: boolean): void
  (e: 'success'): void
}>()

const { t } = useI18n()

const searchQuery = ref('')
const searchResults = ref<import('@/api/org-tree').SearchUserResult[]>([])
const selectedUserId = ref<string>('')
const selectedRole = ref<'admin' | 'editor' | 'viewer'>('viewer')
const searching = ref(false)
const searched = ref(false)
const submitting = ref(false)

watch(() => props.visible, (val) => {
  if (val) {
    searchQuery.value = ''
    searchResults.value = []
    selectedUserId.value = ''
    selectedRole.value = 'viewer'
    searched.value = false
  }
})

const handleSearch = async () => {
  const q = searchQuery.value.trim()
  if (!q) return
  searching.value = true
  searched.value = false
  try {
    // Search users in tenant via org-tree admin API
    const res = await searchUsersForAssign(q, 20)
    if (res.success && res.data) {
      searchResults.value = res.data
    } else {
      searchResults.value = []
    }
  } catch {
    searchResults.value = []
  } finally {
    searching.value = false
    searched.value = true
  }
}

const handleClose = () => {
  emit('update:visible', false)
}

const handleSubmit = async () => {
  if (!selectedUserId.value || !props.orgId) return
  submitting.value = true
  try {
    const res = await assignUserToOrg(props.orgId, {
      user_id: selectedUserId.value,
      role: selectedRole.value,
    })
    if (res.success) {
      MessagePlugin.success(t('admin.member.assignSuccess'))
      emit('success')
      handleClose()
    } else {
      MessagePlugin.error(res.message || t('common.operationFailed'))
    }
  } catch {
    MessagePlugin.error(t('common.operationFailed'))
  } finally {
    submitting.value = false
  }
}
</script>

<style lang="less" scoped>
.dialog-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.4);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 2000;
}

.dialog-content {
  background: #fff;
  border-radius: 12px;
  width: 500px;
  max-width: 90vw;
  max-height: 80vh;
  display: flex;
  flex-direction: column;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.12);
}

.dialog-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px 16px;
  border-bottom: 1px solid #e7e7e7;

  h3 {
    font-size: 16px;
    font-weight: 600;
    margin: 0;
    color: #1a1a1a;
  }
}

.dialog-body {
  padding: 20px 24px;
  overflow-y: auto;
  flex: 1;

  .form-item {
    margin-bottom: 12px;

    .form-label {
      display: block;
      font-size: 14px;
      font-weight: 500;
      color: #333;
      margin-bottom: 8px;

      &.required::before {
        content: '*';
        color: #e34d59;
        margin-right: 4px;
      }
    }
  }
}

.search-results {
  max-height: 200px;
  overflow-y: auto;
  border: 1px solid #e7e7e7;
  border-radius: 8px;
  margin-top: 8px;
}

.search-result-item {
  display: flex;
  align-items: center;
  padding: 8px 12px;
  gap: 8px;
  cursor: pointer;
  transition: background 0.15s;

  &:hover {
    background: #f2f3f5;
  }

  &.selected {
    background: #e8f3ff;
  }

  .result-icon {
    font-size: 20px;
    color: #999;
    flex-shrink: 0;
  }

  .result-info {
    flex: 1;
    min-width: 0;

    .result-name {
      display: block;
      font-size: 14px;
      color: #333;
    }

    .result-email {
      display: block;
      font-size: 12px;
      color: #999;
    }
  }

  .check-icon {
    color: #0052d9;
    flex-shrink: 0;
  }
}

.no-results,
.searching {
  text-align: center;
  padding: 16px;
  color: #999;
  font-size: 13px;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 16px 24px 20px;
  border-top: 1px solid #e7e7e7;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
