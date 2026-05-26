<template>
  <div class="section-content">
    <div class="section-header">
      <h3 class="section-title">{{ $t('agent.pageShare.manageTitle') }}</h3>
      <p class="section-desc">{{ $t('agent.pageShare.manageDesc') }}</p>
    </div>

    <div v-if="agent?.config" class="share-scope-block">
      <h4 class="share-scope-title">{{ $t('agent.shareScope.title') }}</h4>
      <p class="share-scope-desc">{{ $t('agent.shareScope.desc') }}</p>
    </div>

    <div class="section-body">
      <div class="subsection-card page-share-section">
        <div class="subsection-header">
          <div>
            <h4 class="subsection-title">{{ $t('agent.pageShare.sectionTitle') }}</h4>
            <p class="subsection-desc">{{ $t('agent.pageShare.sectionDesc') }}</p>
          </div>
          <t-tag v-if="pageShare" :theme="getPageShareStatusTheme(pageShare.status)" variant="light">
            {{ getPageShareStatusText(pageShare.status) }}
          </t-tag>
        </div>

        <div v-if="loadingPageShare" class="shares-loading">
          <t-loading size="small" />
          <span>{{ $t('common.loading') }}</span>
        </div>

        <div v-else-if="!pageShare" class="page-share-empty">
          <t-icon name="link" class="empty-icon" />
          <div class="page-share-empty-body">
            <div class="page-share-empty-title">{{ $t('agent.pageShare.emptyTitle') }}</div>
            <div class="page-share-empty-desc">{{ $t('agent.pageShare.emptyDesc') }}</div>
          </div>
          <t-button theme="primary" :loading="pageShareSubmitting" @click="handleEnablePageShare">
            {{ $t('agent.pageShare.enable') }}
          </t-button>
        </div>

        <div v-else class="page-share-panel">
          <div class="form-item">
            <label class="form-label">{{ $t('agent.pageShare.linkLabel') }}</label>
            <div class="page-share-link-box">{{ pageShareUrl }}</div>
          </div>

          <div class="page-share-actions-row">
            <t-button variant="outline" @click="handleCopyPageShareLink">{{ $t('common.copy') }}</t-button>
            <t-button variant="outline" :disabled="!isPageShareActive" @click="handleOpenPageShareLink">{{ $t('agent.pageShare.openLink') }}</t-button>
            <t-button
              v-if="isPageShareActive"
              theme="danger"
              variant="outline"
              :loading="pageShareSubmitting"
              @click="handleDisablePageShare"
            >
              {{ $t('agent.pageShare.disable') }}
            </t-button>
            <t-button
              v-else
              theme="primary"
              :loading="pageShareSubmitting"
              @click="handleEnablePageShare"
            >
              {{ $t('agent.pageShare.enable') }}
            </t-button>
          </div>

          <div class="page-share-meta-grid">
            <div class="page-share-meta-item">
              <span class="page-share-meta-label">{{ $t('agent.pageShare.shareCode') }}</span>
              <span class="page-share-meta-value">{{ pageShare.share_code }}</span>
            </div>
            <div class="page-share-meta-item">
              <span class="page-share-meta-label">{{ $t('agent.pageShare.createdAt') }}</span>
              <span class="page-share-meta-value">{{ formatDateTime(pageShare.created_at) }}</span>
            </div>
            <div class="page-share-meta-item">
              <span class="page-share-meta-label">{{ $t('agent.pageShare.updatedAt') }}</span>
              <span class="page-share-meta-value">{{ formatDateTime(pageShare.updated_at) }}</span>
            </div>
            <div class="page-share-meta-item">
              <span class="page-share-meta-label">{{ $t('agent.pageShare.lastAccessedAt') }}</span>
              <span class="page-share-meta-value">{{ formatDateTime(pageShare.last_accessed_at) }}</span>
            </div>
          </div>

          <div v-if="!isPageShareActive" class="page-share-hint">
            {{ $t('agent.pageShare.disabledHint') }}
          </div>
        </div>
      </div>

      <div class="subsection-card space-share-section">
        <div class="subsection-header">
          <div>
            <h4 class="subsection-title">{{ $t('organization.share.title') }}</h4>
            <p class="subsection-desc">{{ $t('organization.share.agentShareDesc') }}</p>
          </div>
        </div>

        <div class="share-form">
          <div class="form-item">
            <label class="form-label">{{ $t('organization.share.selectOrg') }}</label>
            <div class="share-input-row">
              <t-select
                v-model="selectedOrgId"
                :placeholder="$t('organization.share.selectOrgPlaceholder')"
                :loading="loadingOrgs"
                class="org-select org-select-dropdown"
                :popup-props="{ overlayClassName: 'org-select-dropdown-popup' }"
              >
                <t-option
                  v-for="org in availableOrganizations"
                  :key="org.id"
                  :value="org.id"
                  :label="org.name"
                >
                  <div class="org-option-content">
                    <div class="org-option-icon-wrap">
                      <SpaceAvatar :name="org.name" :avatar="org.avatar" size="small" />
                    </div>
                    <div class="org-option-body">
                      <div class="org-option-header">
                        <span class="org-option-name">{{ org.name }}</span>
                        <t-tag v-if="org.is_owner" theme="primary" size="small" variant="light">
                          {{ $t('organization.owner') }}
                        </t-tag>
                        <t-tag v-else-if="org.my_role" :theme="org.my_role === 'admin' ? 'warning' : 'default'" size="small" variant="light">
                          {{ $t(`organization.role.${org.my_role}`) }}
                        </t-tag>
                      </div>
                      <div class="org-option-meta">
                        <span class="org-meta-tag">
                          <t-icon name="user" class="org-meta-icon org-meta-icon-user" />
                          {{ org.member_count ?? 0 }}
                        </span>
                        <span class="org-meta-tag">
                          <img src="@/assets/img/zhishiku.svg" class="org-meta-icon org-meta-icon-kb" alt="" aria-hidden="true" />
                          {{ org.share_count ?? 0 }}
                        </span>
                        <span class="org-meta-tag">
                          <img src="@/assets/img/agent.svg" class="org-meta-icon org-meta-icon-agent" alt="" aria-hidden="true" />
                          {{ org.agent_share_count ?? 0 }}
                        </span>
                      </div>
                    </div>
                  </div>
                </t-option>
              </t-select>
              <t-button
                theme="primary"
                :loading="submitting"
                :disabled="!selectedOrgId"
                @click="handleShare"
              >
                {{ $t('knowledgeEditor.share.addShare') }}
              </t-button>
            </div>
          </div>
        </div>

        <div class="shares-section">
          <div class="shares-header">
            <span class="shares-title">{{ $t('organization.share.sharedTo') }}</span>
            <span class="shares-count">{{ shares.length }}</span>
          </div>
          <div v-if="loadingShares" class="shares-loading">
            <t-loading size="small" />
            <span>{{ $t('common.loading') }}</span>
          </div>
          <div v-else-if="shares.length === 0" class="shares-empty">
            <t-icon name="share" class="empty-icon" />
            <span>{{ $t('organization.share.noShares') }}</span>
          </div>
          <div v-else class="shares-list">
            <div v-for="share in shares" :key="share.id" class="share-item">
              <div class="share-info">
                <div class="share-info-top">
                  <div class="share-org">
                    <SpaceAvatar
                      :name="share.organization_name || ''"
                      :avatar="orgStore.organizations.find(o => o.id === share.organization_id)?.avatar"
                      size="small"
                    />
                    <span class="org-name">{{ share.organization_name }}</span>
                  </div>
                </div>
                <div class="share-item-meta">
                  <span class="org-meta-tag">
                    <t-icon name="user" class="org-meta-icon org-meta-icon-user" />
                    {{ getOrgForShare(share.organization_id)?.member_count ?? 0 }}
                  </span>
                  <span class="org-meta-tag">
                    <img src="@/assets/img/zhishiku.svg" class="org-meta-icon org-meta-icon-kb" alt="" aria-hidden="true" />
                    {{ getOrgForShare(share.organization_id)?.share_count ?? 0 }}
                  </span>
                  <t-tooltip :content="$t('organization.share.spaceAgentShareCountTip')" placement="top">
                    <span class="org-meta-tag">
                      <img src="@/assets/img/agent.svg" class="org-meta-icon org-meta-icon-agent" alt="" aria-hidden="true" />
                      {{ getOrgForShare(share.organization_id)?.agent_share_count ?? 0 }}
                    </span>
                  </t-tooltip>
                </div>
              </div>
              <div class="share-actions">
                <t-popconfirm
                  :content="$t('knowledgeEditor.share.unshareConfirm', { name: share.organization_name })"
                  @confirm="handleUnshare(share)"
                >
                  <t-button variant="text" theme="danger" size="small">
                    <t-icon name="delete" />
                  </t-button>
                </t-popconfirm>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { useOrganizationStore } from '@/stores/organization'
import { shareAgent, listAgentShares, removeAgentShare } from '@/api/organization'
import type { AgentShareResponse } from '@/api/organization'
import type { CustomAgent } from '@/api/agent'
import {
  createOrEnableAgentPageShare,
  deleteAgentPageShare,
  getAgentPageShare,
  type AgentPageShareManagementView,
} from '@/api/agent-share'
import { copyTextToClipboard } from '@/utils/chatMessageShared'
import SpaceAvatar from '@/components/SpaceAvatar.vue'

const { t } = useI18n()
const orgStore = useOrganizationStore()

function getOrgForShare(organizationId: string) {
  return orgStore.organizations.find(o => o.id === organizationId)
}

interface Props {
  agentId: string
  agent?: CustomAgent | null
}

const props = defineProps<Props>()

const loadingOrgs = ref(false)
const loadingShares = ref(false)
const loadingPageShare = ref(false)
const submitting = ref(false)
const pageShareSubmitting = ref(false)
const selectedOrgId = ref('')
const shares = ref<(AgentShareResponse & { organization_name?: string })[]>([])
const pageShare = ref<AgentPageShareManagementView | null>(null)

const availableOrganizations = computed(() => {
  const sharedOrgIds = new Set(shares.value.map(s => s.organization_id))
  return orgStore.organizations.filter(
    (org) =>
      !sharedOrgIds.has(org.id) &&
      (org.is_owner === true || org.my_role === 'admin' || org.my_role === 'editor')
  )
})

const isPageShareActive = computed(() => pageShare.value?.status === 'active')
const pageShareUrl = computed(() => {
  const shareUrl = pageShare.value?.share_url?.trim()
  if (!shareUrl) return ''
  if (/^https?:\/\//i.test(shareUrl)) return shareUrl
  if (typeof window === 'undefined') return shareUrl
  return new URL(shareUrl, window.location.origin).toString()
})

async function loadOrganizations() {
  loadingOrgs.value = true
  try {
    await orgStore.fetchOrganizations()
  } finally {
    loadingOrgs.value = false
  }
}

async function loadPageShare() {
  if (!props.agentId) {
    pageShare.value = null
    return
  }
  loadingPageShare.value = true
  try {
    const result = await getAgentPageShare(props.agentId)
    pageShare.value = result.success ? (result.data || null) : null
  } catch (e) {
    console.error('Failed to load agent page share:', e)
    pageShare.value = null
  } finally {
    loadingPageShare.value = false
  }
}

async function loadShares() {
  if (!props.agentId) return
  loadingShares.value = true
  try {
    const result = await listAgentShares(props.agentId)
    if (result.success && result.data) {
      const sharesData = (result.data as any).shares || result.data
      const sharesList = Array.isArray(sharesData) ? sharesData : []
      shares.value = sharesList.map((share: AgentShareResponse) => ({
        ...share,
        organization_name: share.organization_name || orgStore.organizations.find(o => o.id === share.organization_id)?.name || share.organization_id
      }))
    }
  } catch (e) {
    console.error('Failed to load agent shares:', e)
  } finally {
    loadingShares.value = false
  }
}

async function handleShare() {
  if (!selectedOrgId.value) return
  submitting.value = true
  try {
    const result = await shareAgent(props.agentId, {
      organization_id: selectedOrgId.value,
      permission: 'viewer'
    })
    if (result.success) {
      MessagePlugin.success(t('organization.share.shareSuccess'))
      selectedOrgId.value = ''
      await loadShares()
    } else {
      MessagePlugin.error(result.message || t('organization.share.shareFailed'))
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('organization.share.shareFailed'))
  } finally {
    submitting.value = false
  }
}

async function handleUnshare(share: AgentShareResponse) {
  try {
    const result = await removeAgentShare(props.agentId, share.id)
    if (result.success) {
      MessagePlugin.success(t('organization.share.unshareSuccess'))
      await loadShares()
    } else {
      MessagePlugin.error(result.message || t('organization.share.unshareFailed'))
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('organization.share.unshareFailed'))
  }
}

function formatDateTime(value?: string | null) {
  if (!value) return '--'
  const timestamp = new Date(value)
  if (Number.isNaN(timestamp.getTime())) return '--'
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(timestamp)
}

function getPageShareStatusTheme(status?: string) {
  switch (status) {
    case 'active':
      return 'success'
    case 'disabled':
      return 'warning'
    case 'expired':
      return 'danger'
    default:
      return 'default'
  }
}

function getPageShareStatusText(status?: string) {
  switch (status) {
    case 'active':
      return t('agent.pageShare.statusActive')
    case 'disabled':
      return t('agent.pageShare.statusDisabled')
    case 'expired':
      return t('agent.pageShare.statusExpired')
    default:
      return status || '--'
  }
}

function resolvePageShareErrorMessage(error: any) {
  const message = error?.message || ''
  if (typeof message === 'string' && message.includes('not fully configured for page sharing')) {
    return t('agent.pageShare.notReady')
  }
  return message || t('agent.pageShare.operationFailed')
}

async function handleEnablePageShare() {
  if (!props.agentId) return
  pageShareSubmitting.value = true
  try {
    const result = await createOrEnableAgentPageShare(props.agentId)
    pageShare.value = result.data
    MessagePlugin.success(t('agent.pageShare.enableSuccess'))
  } catch (e: any) {
    MessagePlugin.error(resolvePageShareErrorMessage(e))
  } finally {
    pageShareSubmitting.value = false
  }
}

async function handleDisablePageShare() {
  if (!props.agentId) return
  pageShareSubmitting.value = true
  try {
    await deleteAgentPageShare(props.agentId)
    await loadPageShare()
    MessagePlugin.success(t('agent.pageShare.disableSuccess'))
  } catch (e: any) {
    MessagePlugin.error(resolvePageShareErrorMessage(e))
  } finally {
    pageShareSubmitting.value = false
  }
}

async function handleCopyPageShareLink() {
  if (!pageShareUrl.value) return
  try {
    await copyTextToClipboard(pageShareUrl.value)
    MessagePlugin.success(t('common.copied'))
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('common.copyFailed'))
  }
}

function handleOpenPageShareLink() {
  if (!pageShareUrl.value) return
  window.open(pageShareUrl.value, '_blank', 'noopener,noreferrer')
}

watch(() => props.agentId, async (newId) => {
  if (!newId) {
    shares.value = []
    pageShare.value = null
    return
  }
  await Promise.all([loadOrganizations(), loadShares(), loadPageShare()])
}, { immediate: true })

defineExpose({ loadShares, loadPageShare })
</script>

<style scoped lang="less">
.section-content { .section-header { margin-bottom: 20px; } .section-title { margin: 0 0 8px 0; font-size: 16px; font-weight: 600; } .section-desc { margin: 0; font-size: 14px; color: var(--td-text-color-disabled); } }
.section-body {
  display: flex;
  flex-direction: column;
  gap: 24px;
}
.subsection-card {
  padding: 20px;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: 12px;
}
.subsection-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
}
.subsection-title {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}
.subsection-desc {
  margin: 6px 0 0 0;
  font-size: 13px;
  line-height: 1.5;
  color: var(--td-text-color-secondary);
}
.share-form { margin-bottom: 24px; padding-bottom: 24px; border-bottom: 1px solid var(--td-component-stroke); }
.form-item {
  .form-label {
    display: block;
    margin-bottom: 12px;
    font-size: 14px;
    font-weight: 500;
  }
}
.share-input-row {
  display: flex;
  gap: 12px;
  align-items: center;
  flex-wrap: wrap;
  .org-select { flex: 1; min-width: 240px; }
}
.shares-section { margin-bottom: 0; }
.shares-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
  .shares-title {
    font-family: var(--app-font-family);
    font-size: 15px;
    font-weight: 500;
    color: var(--td-text-color-primary);
  }
  .shares-count {
    padding: 2px 8px;
    background: var(--td-bg-color-secondarycontainer);
    border-radius: 10px;
    font-size: 12px;
    color: var(--td-text-color-disabled);
  }
}
.shares-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 32px;
  color: var(--td-text-color-disabled);
  font-size: 14px;
}
.shares-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 40px 20px;
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 8px;
  color: var(--td-text-color-disabled);
  .empty-icon { font-size: 32px; opacity: 0.5; }
}
.page-share-empty {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 24px 20px;
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 10px;
}
.page-share-empty-body {
  flex: 1;
  min-width: 0;
}
.page-share-empty-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}
.page-share-empty-desc {
  margin-top: 4px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
  line-height: 1.5;
}
.page-share-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.page-share-link-box {
  padding: 12px 14px;
  border-radius: 8px;
  border: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-primary);
  font-size: 13px;
  line-height: 1.5;
  word-break: break-all;
}
.page-share-actions-row {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
.page-share-meta-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
}
.page-share-meta-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px 14px;
  border-radius: 8px;
  background: var(--td-bg-color-secondarycontainer);
}
.page-share-meta-label {
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
.page-share-meta-value {
  font-size: 13px;
  color: var(--td-text-color-primary);
  word-break: break-all;
}
.page-share-hint {
  padding: 12px 14px;
  border-radius: 8px;
  background: rgba(245, 158, 11, 0.08);
  color: var(--td-warning-color);
  font-size: 13px;
  line-height: 1.5;
}
.shares-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-height: 320px;
  overflow-y: auto;
}
.share-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  background: var(--td-bg-color-secondarycontainer);
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  transition: background 0.2s ease, border-color 0.2s ease;
  &:hover {
    background: var(--td-bg-color-secondarycontainer);
    border-color: var(--td-component-stroke);
  }
}
.share-info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.share-info-top {
  display: flex;
  align-items: center;
  gap: 12px;
}
.share-org {
  display: flex;
  align-items: center;
  gap: 8px;
  .org-name {
    font-family: var(--app-font-family);
    font-size: 14px;
    font-weight: 500;
    color: var(--td-text-color-primary);
  }
}
.share-item-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
  .org-meta-tag {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding: 2px 6px;
    background: var(--td-bg-color-secondarycontainer);
    border-radius: 4px;
  }
  .org-meta-icon {
    flex-shrink: 0;
    vertical-align: middle;
    color: var(--td-text-color-placeholder);
  }
  .org-meta-icon-user {
    font-size: 12px;
  }
  .org-meta-icon-kb {
    width: 12px;
    height: 12px;
    opacity: 0.75;
  }
  .org-meta-icon-agent {
    width: 12px;
    height: 12px;
    opacity: 0.75;
  }
}
.share-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  .permission-change-select { width: 100px; }
}

.share-scope-block {
  margin-bottom: 24px;
  padding: 16px;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-success-color-focus);
  border-radius: 8px;
}
.share-scope-title {
  margin: 0 0 6px 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}
.share-scope-desc {
  margin: 0 0 12px 0;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  line-height: 1.4;
}

@media (max-width: 768px) {
  .page-share-empty {
    flex-direction: column;
    align-items: flex-start;
  }
}

:deep(.t-select-option) {
  height: auto;
  align-items: center;
  padding: 6px 12px;
  border-radius: 4px;
  margin: 1px 6px;
  transition: background 0.15s ease;
}
:deep(.t-select-option:hover),
:deep(.t-select-option.t-is-selected) {
  background: var(--td-brand-color-light);
}
:deep(.t-select-option__content) {
  width: 100%;
}
.org-option-content {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0;
  min-width: 260px;
  width: 100%;
}
.org-option-icon-wrap {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}
.org-option-body {
  flex: 1;
  min-width: 0;
}
.org-option-header {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 2px;
}
.org-option-name {
  font-family: var(--app-font-family);
  font-size: 13px;
  font-weight: 500;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.org-option-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  font-family: var(--app-font-family);
  font-size: 12px;
  color: var(--td-text-color-placeholder);

  .org-meta-tag {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding: 0px 4px;
    background: var(--td-bg-color-secondarycontainer);
    border-radius: 4px;
  }

  .org-meta-icon {
    flex-shrink: 0;
    vertical-align: middle;
    color: var(--td-text-color-placeholder);
  }

  .org-meta-icon-user {
    font-size: 12px;
  }

  .org-meta-icon-kb {
    width: 12px;
    height: 12px;
    opacity: 0.75;
  }
  .org-meta-icon-agent {
    width: 12px;
    height: 12px;
    opacity: 0.75;
  }
}
</style>

<style lang="less">
.org-select-dropdown-popup.t-select__dropdown {
  padding: 4px 0;
  max-height: 320px;
  overflow-y: auto;
  border-radius: 6px;
  box-shadow: var(--td-shadow-2);
}
.org-select-dropdown-popup .t-select-option {
  height: auto;
  align-items: center;
  padding: 6px 12px;
  border-radius: 4px;
  margin: 1px 6px;
}
.org-select-dropdown-popup .t-select-option__content {
  width: 100%;
}
html[theme-mode="dark"] .org-meta-icon-kb,
html[theme-mode="dark"] .org-meta-icon-agent {
  filter: invert(1);
  opacity: 0.55;
}
</style>
