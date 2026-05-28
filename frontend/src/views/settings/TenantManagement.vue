<template>
  <div class="tenant-management">
    <div class="section-header">
      <div class="section-header-copy">
        <h2>{{ t('tenant.management.title') }}</h2>
        <p class="section-description">{{ t('tenant.management.description') }}</p>
      </div>
      <t-button theme="primary" @click="createTenantVisible = true">
        <template #icon>
          <t-icon name="add" />
        </template>
        {{ t('tenant.create.button') }}
      </t-button>
    </div>

    <div class="summary-grid">
      <div class="summary-card">
        <span class="summary-label">{{ t('tenant.management.currentTenantLabel') }}</span>
        <div class="summary-value-row">
          <span class="summary-value">{{ authStore.currentTenantName || t('tenant.unknown') }}</span>
          <t-tag v-if="authStore.currentTenantRole" :theme="roleTheme(authStore.currentTenantRole)" variant="light" size="small">
            {{ roleLabel(authStore.currentTenantRole) }}
          </t-tag>
        </div>
      </div>
      <div class="summary-card">
        <span class="summary-label">{{ t('tenant.management.totalTenantsLabel') }}</span>
        <span class="summary-value">{{ tenantOptions.length }}</span>
      </div>
    </div>

    <div class="tenant-list-panel">
      <div class="tenant-list-header">
        <div>
          <h3>{{ t('tenant.management.listTitle') }}</h3>
          <p>{{ t('tenant.management.listDescription') }}</p>
        </div>
        <t-button variant="outline" size="small" :loading="loading" @click="loadMemberships">
          {{ t('tenant.management.refresh') }}
        </t-button>
      </div>

      <div v-if="loading" class="loading-inline">
        <t-loading size="small" />
        <span>{{ t('tenant.loadingInfo') }}</span>
      </div>

      <div v-else-if="error" class="error-inline">
        <t-alert theme="error" :message="error">
          <template #operation>
            <t-button size="small" @click="loadMemberships">{{ t('tenant.retry') }}</t-button>
          </template>
        </t-alert>
      </div>

      <div v-else-if="tenantOptions.length === 0" class="empty-state">
        <t-icon name="usergroup" size="32px" />
        <p class="empty-title">{{ t('tenant.management.emptyTitle') }}</p>
        <p class="empty-description">{{ t('tenant.management.emptyDescription') }}</p>
      </div>

      <div v-else class="tenant-grid">
        <article
          v-for="tenant in tenantOptions"
          :key="tenant.tenant_id"
          :class="['tenant-card', { 'tenant-card--active': isCurrentTenant(tenant.tenant_id) }]"
        >
          <div class="tenant-card-header">
            <div class="tenant-card-title-wrap">
              <div class="tenant-card-name-row">
                <h4>{{ tenant.tenant_name || formatTenantFallbackName(tenant.tenant_id) }}</h4>
                <t-tag v-if="isCurrentTenant(tenant.tenant_id)" theme="success" variant="light" size="small">
                  {{ t('tenant.management.currentBadge') }}
                </t-tag>
                <t-tag v-if="isHomeTenant(tenant.tenant_id)" theme="primary" variant="light" size="small">
                  {{ t('tenant.management.homeBadge') }}
                </t-tag>
              </div>
              <p>{{ t('tenant.management.tenantIdLabel', { id: tenant.tenant_id }) }}</p>
            </div>
            <t-tag :theme="roleTheme(tenant.role)" variant="light">{{ roleLabel(tenant.role) }}</t-tag>
          </div>

          <div class="tenant-card-footer">
            <span class="tenant-card-hint">
              {{ isCurrentTenant(tenant.tenant_id) ? t('tenant.management.currentHint') : t('tenant.management.switchHint') }}
            </span>
            <t-button
              theme="primary"
              variant="outline"
              size="small"
              :disabled="isCurrentTenant(tenant.tenant_id)"
              :loading="switchingTenantId === tenant.tenant_id"
              @click="switchTenant(tenant.tenant_id)"
            >
              {{ isCurrentTenant(tenant.tenant_id) ? t('tenant.management.currentAction') : t('tenant.management.switchAction') }}
            </t-button>
          </div>
        </article>
      </div>
    </div>

    <CreateTenantDialog v-model:visible="createTenantVisible" @created="handleTenantCreated" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import CreateTenantDialog from '@/components/CreateTenantDialog.vue'
import type { TenantInfo } from '@/api/tenant'
import { useAuthStore } from '@/stores/auth'

type TenantMembership = {
  tenant_id: number
  tenant_name?: string
  role: string
}

const { t } = useI18n()
const authStore = useAuthStore()

const loading = ref(false)
const error = ref('')
const createTenantVisible = ref(false)
const switchingTenantId = ref<number | null>(null)

const homeTenantId = computed(() => {
  const raw = authStore.tenant?.id
  return raw != null && raw !== '' ? Number(raw) : null
})

const activeTenantId = computed(() => {
  return authStore.selectedTenantId ?? homeTenantId.value
})

const tenantOptions = computed<TenantMembership[]>(() => {
  const merged = new Map<number, TenantMembership>()

  for (const member of authStore.memberships) {
    merged.set(Number(member.tenant_id), {
      tenant_id: Number(member.tenant_id),
      tenant_name: member.tenant_name,
      role: member.role,
    })
  }

  if (homeTenantId.value !== null && !merged.has(homeTenantId.value)) {
    merged.set(homeTenantId.value, {
      tenant_id: homeTenantId.value,
      tenant_name: authStore.tenant?.name,
      role: authStore.currentTenantRole || 'owner',
    })
  }

  return Array.from(merged.values()).sort((left, right) => {
    const leftCurrent = Number(left.tenant_id) === Number(activeTenantId.value)
    const rightCurrent = Number(right.tenant_id) === Number(activeTenantId.value)
    if (leftCurrent !== rightCurrent) return leftCurrent ? -1 : 1

    const leftHome = Number(left.tenant_id) === Number(homeTenantId.value)
    const rightHome = Number(right.tenant_id) === Number(homeTenantId.value)
    if (leftHome !== rightHome) return leftHome ? -1 : 1

    return (left.tenant_name || '').localeCompare(right.tenant_name || '', 'zh-CN')
  })
})

const loadMemberships = async () => {
  loading.value = true
  error.value = ''
  try {
    const ok = await authStore.refreshFromAuthMe()
    if (!ok) {
      error.value = t('tenant.management.loadFailed')
    }
  } catch (err: any) {
    error.value = err?.message || t('tenant.management.loadFailed')
  } finally {
    loading.value = false
  }
}

const isCurrentTenant = (tenantId: number) => Number(activeTenantId.value) === Number(tenantId)

const isHomeTenant = (tenantId: number) => Number(homeTenantId.value) === Number(tenantId)

const roleTheme = (role: string) => {
  switch (role) {
    case 'owner':
      return 'danger'
    case 'admin':
      return 'primary'
    case 'contributor':
      return 'warning'
    default:
      return 'default'
  }
}

const roleLabel = (role: string) => {
  switch (role) {
    case 'owner':
      return t('tenant.management.roles.owner')
    case 'admin':
      return t('tenant.management.roles.admin')
    case 'contributor':
      return t('tenant.management.roles.contributor')
    default:
      return t('tenant.management.roles.viewer')
  }
}

const formatTenantFallbackName = (tenantId: number) => {
  return t('tenant.management.unnamedTenant', { id: tenantId })
}

const reloadAfterSwitch = () => {
  window.setTimeout(() => {
    window.location.reload()
  }, 300)
}

const switchTenant = (tenantId: number) => {
  if (isCurrentTenant(tenantId)) return

  switchingTenantId.value = tenantId
  const target = tenantOptions.value.find((item) => Number(item.tenant_id) === Number(tenantId))
  if (isHomeTenant(tenantId)) {
    authStore.setSelectedTenant(null, null)
  } else {
    authStore.setSelectedTenant(tenantId, target?.tenant_name || null)
  }
  MessagePlugin.success(t('tenant.switchSuccess'))
  reloadAfterSwitch()
}

const handleTenantCreated = (tenant: TenantInfo) => {
  const nextMemberships = [...authStore.memberships]
  if (!nextMemberships.some((item) => Number(item.tenant_id) === Number(tenant.id))) {
    nextMemberships.push({
      tenant_id: tenant.id,
      tenant_name: tenant.name,
      role: 'owner',
    })
    authStore.setMemberships(nextMemberships)
  }

  if (!authStore.allTenants.some((item) => Number(item.id) === Number(tenant.id))) {
    authStore.setAllTenants([...authStore.allTenants, tenant])
  }

  switchingTenantId.value = tenant.id
  authStore.setSelectedTenant(tenant.id, tenant.name)
  reloadAfterSwitch()
}

onMounted(() => {
  void loadMemberships()
})
</script>

<style lang="less" scoped>
.tenant-management {
  width: 100%;
}

.section-header {
  margin-bottom: 24px;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;

  h2 {
    margin: 0 0 8px;
    font-size: 20px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  .section-description {
    margin: 0;
    font-size: 14px;
    line-height: 1.6;
    color: var(--td-text-color-secondary);
  }
}

.section-header-copy {
  min-width: 0;
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
  margin-bottom: 20px;
}

.summary-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 16px 18px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 12px;
  background: var(--td-bg-color-container);
}

.summary-label {
  font-size: 13px;
  color: var(--td-text-color-secondary);
}

.summary-value-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.summary-value {
  font-size: 18px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.tenant-list-panel {
  border: 1px solid var(--td-component-stroke);
  border-radius: 12px;
  background: var(--td-bg-color-container);
  padding: 18px;
}

.tenant-list-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 18px;

  h3 {
    margin: 0 0 6px;
    font-size: 16px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  p {
    margin: 0;
    font-size: 13px;
    line-height: 1.55;
    color: var(--td-text-color-secondary);
  }
}

.loading-inline {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  min-height: 140px;
  color: var(--td-text-color-secondary);
}

.error-inline {
  padding: 8px 0;
}

.empty-state {
  min-height: 160px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: var(--td-text-color-secondary);
  text-align: center;
}

.empty-title {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.empty-description {
  margin: 0;
  font-size: 13px;
  line-height: 1.55;
}

.tenant-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.tenant-card {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 16px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 12px;
  background: var(--td-bg-color-secondarycontainer);
}

.tenant-card--active {
  border-color: rgba(7, 192, 95, 0.35);
  background: rgba(7, 192, 95, 0.06);
}

.tenant-card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.tenant-card-title-wrap {
  min-width: 0;

  h4 {
    margin: 0;
    font-size: 16px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    overflow-wrap: anywhere;
  }

  p {
    margin: 6px 0 0;
    font-size: 13px;
    color: var(--td-text-color-secondary);
  }
}

.tenant-card-name-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.tenant-card-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.tenant-card-hint {
  min-width: 0;
  font-size: 13px;
  line-height: 1.55;
  color: var(--td-text-color-secondary);
}

@media (max-width: 768px) {
  .section-header,
  .tenant-list-header,
  .tenant-card-footer {
    flex-direction: column;
    align-items: stretch;
  }

  .summary-grid,
  .tenant-grid {
    grid-template-columns: 1fr;
  }

  .tenant-card-header {
    flex-direction: column;
  }
}
</style>