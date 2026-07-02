<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import {
  listDataSources,
  deleteDataSource,
  triggerSync,
  pauseDataSource,
  resumeDataSource,
  getDatabaseSchema,
  refreshDataSourceSchema,
  type DataSource,
  type DatabaseSchema,
} from '@/api/datasource'
import { humanizeCron, relativeTime } from '@/utils/cronHumanize'
import DataSourceEditorDialog from './DataSourceEditorDialog.vue'
import DataSourceSyncLogs from './DataSourceSyncLogs.vue'
import DataSourceTypeIcon from './DataSourceTypeIcon.vue'
import DatabaseSchemaDialog from './DatabaseSchemaDialog.vue'
import DatabaseQueryAuditDialog from './DatabaseQueryAuditDialog.vue'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{ kbId: string }>()
const emit = defineEmits<{
  (e: 'count', value: number): void
  (e: 'database-change'): void
}>()
const { t, te } = useI18n()
const authStore = useAuthStore()

// 后端 /datasource 的 list/logs 是 Viewer+，但所有写操作（POST/PUT/DELETE
// 以及 sync/pause/resume/validate）都是 Admin+。低权限用户保留只读视图，
// 增删改和触发同步全部隐藏，而不是按下去再撞 403。
const canManageDataSource = computed(() => authStore.hasRole('admin'))

const dataSources = ref<DataSource[]>([])
const loading = ref(false)
const editorVisible = ref(false)
const editingDs = ref<DataSource | null>(null)
const logsVisible = ref(false)
const logsDsId = ref('')
const logsDsName = ref('')
const pollTimer = ref<number | null>(null)
const schemaSnapshot = ref<DatabaseSchema | null>(null)
const schemaDialogVisible = ref(false)
const auditDialogVisible = ref(false)
const activeDatabaseDataSource = ref<DataSource | null>(null)

function translateOrFallback(key: string, fallback: string) {
  return te(key) ? t(key) : fallback
}

function emitDatabaseChange() {
  emit('database-change')
}

function isDatabaseDataSource(ds: DataSource) {
  return ds.type === 'mysql' || ds.type === 'postgresql'
}

async function loadDatabaseSchemaSnapshot() {
  if (!props.kbId) {
    schemaSnapshot.value = null
    return
  }
  try {
    const res: any = await getDatabaseSchema(props.kbId)
    schemaSnapshot.value = (res?.data || res || null) as DatabaseSchema | null
  } catch {
    schemaSnapshot.value = null
  }
}

function databaseSettings(ds: DataSource): Record<string, any> {
  const cfg = ds.config as Record<string, any> | undefined
  return (cfg?.settings as Record<string, any>) || {}
}

function matchesSchemaSnapshot(ds: DataSource) {
  if (!schemaSnapshot.value) return false
  if (schemaSnapshot.value.data_source_id) {
    return schemaSnapshot.value.data_source_id === ds.id
  }
  return true
}

function schemaDatabaseName(ds: DataSource) {
  if (matchesSchemaSnapshot(ds) && schemaSnapshot.value?.database_name) {
    return schemaSnapshot.value.database_name
  }
  return databaseSettings(ds).database || '--'
}

function schemaScopeName(ds: DataSource) {
  if (matchesSchemaSnapshot(ds) && schemaSnapshot.value?.schema_name) {
    return schemaSnapshot.value.schema_name
  }
  return databaseSettings(ds).schema || databaseSettings(ds).database || '--'
}

function schemaRefreshedAt(ds: DataSource) {
  if (matchesSchemaSnapshot(ds) && schemaSnapshot.value?.refreshed_at) {
    return relativeTime(schemaSnapshot.value.refreshed_at, t)
  }
  return '--'
}

function schemaRefreshedAtFull(ds: DataSource) {
  if (matchesSchemaSnapshot(ds) && schemaSnapshot.value?.refreshed_at) {
    return new Date(schemaSnapshot.value.refreshed_at).toLocaleString()
  }
  return ''
}

function schemaStatusLabel(ds: DataSource) {
  if (matchesSchemaSnapshot(ds) && (schemaSnapshot.value?.tables?.length || 0) > 0) {
    return translateOrFallback('datasource.schemaReady', '结构已更新')
  }
  return translateOrFallback('datasource.schemaPending', '等待刷新结构')
}

async function handleRefreshSchema(ds: DataSource) {
  try {
    await refreshDataSourceSchema(ds.id)
    MessagePlugin.success(translateOrFallback('datasource.refreshSchemaSuccess', 'Schema 已刷新'))
    await loadList(true)
    emitDatabaseChange()
  } catch (e: any) {
    MessagePlugin.error(e?.message || e?.error || translateOrFallback('datasource.refreshSchemaFailed', '刷新 Schema 失败'))
  }
}

function openSchema(ds: DataSource) {
  activeDatabaseDataSource.value = ds
  schemaDialogVisible.value = true
}

function openAudit(ds: DataSource) {
  activeDatabaseDataSource.value = ds
  auditDialogVisible.value = true
}

function stopPolling() {
  if (pollTimer.value !== null) {
    window.clearTimeout(pollTimer.value)
    pollTimer.value = null
  }
}

function schedulePolling() {
  stopPolling()
  pollTimer.value = window.setTimeout(() => {
    loadList(true)
  }, 3000)
}

async function loadList(silent = false) {
  if (!silent) loading.value = true
  try {
    const res = await listDataSources(props.kbId)
    dataSources.value = res?.data || res || []
    emit('count', dataSources.value.length)

    if (dataSources.value.some(isDatabaseDataSource)) {
      await loadDatabaseSchemaSnapshot()
    } else {
      schemaSnapshot.value = null
    }

    const hasRunningSync = dataSources.value.some(ds => ds.latest_sync_log?.status === 'running')
    if (hasRunningSync) {
      schedulePolling()
    } else {
      stopPolling()
    }
  } catch (e: any) {
    console.error(e)
  } finally {
    if (!silent) loading.value = false
  }
}

function openCreate() {
  editingDs.value = null
  editorVisible.value = true
}

function openEdit(ds: DataSource) {
  editingDs.value = ds
  editorVisible.value = true
}

function openLogs(ds: DataSource) {
  logsDsId.value = ds.id
  logsDsName.value = ds.name
  logsVisible.value = true
}

async function removeDataSource(ds: DataSource) {
  try {
    await deleteDataSource(ds.id)
    MessagePlugin.success(t('datasource.deleteSuccess'))
    await loadList()
    emitDatabaseChange()
  } catch (e: any) {
    MessagePlugin.error(e?.message || e?.error || t('datasource.deleteFailed'))
  }
}

async function handleSync(ds: DataSource) {
  if (isDatabaseDataSource(ds)) {
    await handleRefreshSchema(ds)
    return
  }
  try {
    await triggerSync(ds.id)
    MessagePlugin.success(t('datasource.syncTriggered'))
    await loadList(true)
  } catch (e: any) {
    MessagePlugin.error(e?.message || e?.error || t('datasource.syncFailed'))
  }
}

async function handlePause(ds: DataSource) {
  try {
    await pauseDataSource(ds.id)
    MessagePlugin.success(t('datasource.paused'))
    await loadList()
    if (isDatabaseDataSource(ds)) {
      emitDatabaseChange()
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || e?.error || t('datasource.pauseFailed'))
  }
}

async function handleResume(ds: DataSource) {
  try {
    await resumeDataSource(ds.id)
    MessagePlugin.success(t('datasource.resumed'))
    await loadList()
    if (isDatabaseDataSource(ds)) {
      emitDatabaseChange()
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || e?.error || t('datasource.resumeFailed'))
  }
}

function statusLabel(status: string) {
  return t(`datasource.status.${status}`)
}

function syncModeLabel(mode: string) {
  return t(`datasource.syncMode.${mode}`)
}

function connectorLabel(type: string) {
  return t(`datasource.connector.${type}`) || type
}

function scheduleLabel(cron: string) {
  return humanizeCron(cron, t)
}

function lastSyncTime(ds: DataSource) {
  return relativeTime(ds.last_sync_at, t)
}

function lastSyncFullTime(ds: DataSource) {
  if (!ds.last_sync_at) return ''
  return new Date(ds.last_sync_at).toLocaleString()
}

function syncResultPills(ds: DataSource) {
  const log = ds.latest_sync_log
  if (!log) return []
  const pills: { text: string; cls: string }[] = []
  if (log.items_created > 0) pills.push({ text: `+${log.items_created}`, cls: 'created' })
  if (log.items_updated > 0) pills.push({ text: `~${log.items_updated}`, cls: 'updated' })
  if (log.items_deleted > 0) pills.push({ text: `-${log.items_deleted}`, cls: 'deleted' })
  if (log.items_failed > 0) pills.push({ text: `${log.items_failed} ${t('datasource.logMetric.failed')}`, cls: 'failed' })
  if (log.items_skipped > 0) pills.push({ text: `${log.items_skipped} ${t('datasource.logMetric.skipped')}`, cls: 'skipped' })
  return pills
}

function lastSyncStatusLabel(ds: DataSource) {
  const log = ds.latest_sync_log
  if (!log) return '--'
  return t(`datasource.logStatus.${log.status}`)
}

function isSyncRunning(ds: DataSource) {
  return ds.latest_sync_log?.status === 'running'
}

async function onEditorSaved() {
  editorVisible.value = false
  await loadList()
  emitDatabaseChange()
}

onMounted(loadList)
onBeforeUnmount(stopPolling)
</script>

<template>
  <div class="ds-settings">
    <div class="section-header">
      <h2>{{ t('datasource.title') }}</h2>
      <p class="section-description">{{ t('datasource.description') }}</p>
    </div>

    <t-loading :loading="loading" size="small" class="ds-list-loading">
      <div
        v-if="!loading && dataSources.length === 0 && !canManageDataSource"
        class="empty-state"
      >
        <t-empty :description="t('datasource.empty')" />
      </div>

      <div v-else-if="!loading" class="ds-grid">
        <template v-for="ds in dataSources" :key="ds.id">
        <div
          v-if="isDatabaseDataSource(ds)"
          :class="['ds-card', 'ds-card--database', { 'ds-card--clickable': canManageDataSource }]"
          @click="canManageDataSource ? openEdit(ds) : undefined"
        >
          <div class="ds-card__badge">
            <DataSourceTypeIcon :type="ds.type" variant="badge" />
          </div>
          <div class="ds-card__body">
            <div class="ds-card__header">
              <div>
                <h3 class="ds-card__title" :title="ds.name">{{ ds.name }}</h3>
                <p class="ds-card__subtitle">
                  {{ connectorLabel(ds.type) }}
                  <span class="ds-card__sep">·</span>
                  <span class="ds-card__status ds-card__status--active">
                    <span class="ds-status-dot" aria-hidden="true" />
                    {{ translateOrFallback('datasource.connected', '已连接') }}
                  </span>
                </p>
              </div>
              <div class="ds-db-actions" @click.stop>
                <t-tooltip :content="translateOrFallback('datasource.refreshSchema', '刷新 Schema')" placement="top">
                  <t-button size="small" variant="text" class="ds-card__action-btn" @click.stop="handleRefreshSchema(ds)">
                    <template #icon><t-icon name="refresh" /></template>
                  </t-button>
                </t-tooltip>
                <t-tooltip :content="translateOrFallback('datasource.schemaDialogTitle', '数据库结构')" placement="top">
                  <t-button size="small" variant="text" class="ds-card__action-btn" @click.stop="openSchema(ds)">
                    <template #icon><t-icon name="table" /></template>
                  </t-button>
                </t-tooltip>
                <t-tooltip :content="translateOrFallback('datasource.auditDialogTitle', '数据库查询审计')" placement="top">
                  <t-button size="small" variant="text" class="ds-card__action-btn" @click.stop="openAudit(ds)">
                    <template #icon><t-icon name="list" /></template>
                  </t-button>
                </t-tooltip>
                <t-dropdown v-if="canManageDataSource" trigger="click" :min-column-width="140" attach="body">
                  <t-button variant="text" shape="square" size="small" class="ds-card__action-btn" @click.stop>
                    <template #icon><t-icon name="ellipsis" /></template>
                  </t-button>
                  <template #dropdown>
                    <t-dropdown-menu>
                      <t-dropdown-item @click="openEdit(ds)">
                        <t-icon name="edit" /> {{ t('datasource.edit') }}
                      </t-dropdown-item>
                      <t-dropdown-item theme="error" class="ds-dropdown-delete-item">
                        <t-popconfirm
                          :content="t('datasource.deleteConfirm')"
                          :confirm-btn="{ content: t('datasource.delete'), theme: 'danger' }"
                          :cancel-btn="{ content: t('common.cancel') }"
                          placement="left"
                          attach="body"
                          @confirm="removeDataSource(ds)"
                        >
                          <span class="ds-dropdown-delete-trigger" @click.stop>
                            <t-icon name="delete" />
                            <span>{{ t('datasource.delete') }}</span>
                          </span>
                        </t-popconfirm>
                      </t-dropdown-item>
                    </t-dropdown-menu>
                  </template>
                </t-dropdown>
              </div>
            </div>
            <div class="ds-db-summary">
              <div class="ds-db-summary__item">
                <span class="ds-db-summary__label">{{ translateOrFallback('knowledgeBase.databaseDetail.cards.datasource.suffix', '业务库') }}</span>
                <strong class="ds-db-summary__value">{{ schemaDatabaseName(ds) }}</strong>
              </div>
              <div class="ds-db-summary__item">
                <span class="ds-db-summary__label">{{ translateOrFallback('knowledgeBase.databaseDetail.cards.schema.tag', '结构范围') }}</span>
                <strong class="ds-db-summary__value">{{ schemaScopeName(ds) }}</strong>
              </div>
              <div class="ds-db-summary__item">
                <span class="ds-db-summary__label">{{ translateOrFallback('datasource.schema.refreshedAt', '上次刷新结构') }}</span>
                <t-tooltip :content="schemaRefreshedAtFull(ds)" :disabled="!schemaRefreshedAtFull(ds)">
                  <strong class="ds-db-summary__value">{{ schemaRefreshedAt(ds) }}</strong>
                </t-tooltip>
              </div>
              <div class="ds-db-summary__item">
                <span class="ds-db-summary__label">{{ translateOrFallback('datasource.schema.statusLabel', '结构状态') }}</span>
                <strong class="ds-db-summary__value ds-db-summary__value--success">{{ schemaStatusLabel(ds) }}</strong>
              </div>
            </div>
          </div>
        </div>
        <component
          v-else
          :is="canManageDataSource ? 'button' : 'div'"
          :type="canManageDataSource ? 'button' : undefined"
          :class="['ds-card', `ds-card--${ds.type}`, { 'ds-card--clickable': canManageDataSource }]"
          @click="canManageDataSource ? openEdit(ds) : undefined"
        >
          <div class="ds-card__badge">
            <DataSourceTypeIcon :type="ds.type" variant="badge" />
          </div>
          <div class="ds-card__body">
            <div class="ds-card__header">
              <h3 class="ds-card__title" :title="ds.name">{{ ds.name }}</h3>
              <div class="ds-card__actions" @click.stop>
                <t-dropdown trigger="click" :min-column-width="140" attach="body">
                  <t-button
                    variant="text"
                    shape="square"
                    size="small"
                    class="ds-card__action-btn"
                    @click.stop
                  >
                    <template #icon><t-icon name="ellipsis" /></template>
                  </t-button>
                  <template #dropdown>
                    <t-dropdown-menu>
                      <t-dropdown-item v-if="canManageDataSource" @click="openEdit(ds)">
                        <t-icon name="edit" /> {{ t('datasource.edit') }}
                      </t-dropdown-item>
                      <t-dropdown-item
                        v-if="canManageDataSource"
                        :disabled="isSyncRunning(ds)"
                        @click="handleSync(ds)"
                      >
                        <t-icon name="refresh" :class="{ 'ds-icon-spin': isSyncRunning(ds) }" />
                        {{ isSyncRunning(ds) ? t('datasource.logStatus.running') : t('datasource.syncNow') }}
                      </t-dropdown-item>
                      <t-dropdown-item @click="openLogs(ds)">
                        <t-icon name="root-list" /> {{ t('datasource.logs') }}
                      </t-dropdown-item>
                      <t-dropdown-item
                        v-if="canManageDataSource && ds.status === 'active'"
                        @click="handlePause(ds)"
                      >
                        <t-icon name="pause-circle" /> {{ t('datasource.pause') }}
                      </t-dropdown-item>
                      <t-dropdown-item
                        v-else-if="canManageDataSource && ds.status === 'paused'"
                        @click="handleResume(ds)"
                      >
                        <t-icon name="play-circle" /> {{ t('datasource.resume') }}
                      </t-dropdown-item>
                      <t-dropdown-item
                        v-if="canManageDataSource"
                        theme="error"
                        class="ds-dropdown-delete-item"
                      >
                        <t-popconfirm
                          :content="t('datasource.deleteConfirm')"
                          :confirm-btn="{ content: t('datasource.delete'), theme: 'danger' }"
                          :cancel-btn="{ content: t('common.cancel') }"
                          placement="left"
                          attach="body"
                          @confirm="removeDataSource(ds)"
                        >
                          <span class="ds-dropdown-delete-trigger" @click.stop>
                            <t-icon name="delete" />
                            <span>{{ t('datasource.delete') }}</span>
                          </span>
                        </t-popconfirm>
                      </t-dropdown-item>
                    </t-dropdown-menu>
                  </template>
                </t-dropdown>
              </div>
            </div>
            <p class="ds-card__subtitle">
              {{ connectorLabel(ds.type) }} · {{ syncModeLabel(ds.sync_mode) }}
              <span class="ds-card__sep">·</span>
              <span class="ds-card__status" :class="`ds-card__status--${ds.status}`">
                <span class="ds-status-dot" aria-hidden="true" />
                {{ statusLabel(ds.status) }}
              </span>
            </p>
            <p class="ds-card__detail">
              {{ scheduleLabel(ds.sync_schedule) }}
              <span class="ds-card__sep">·</span>
              <t-tooltip :content="lastSyncFullTime(ds)" :disabled="!lastSyncFullTime(ds)">
                <span>{{ lastSyncTime(ds) || '--' }}</span>
              </t-tooltip>
              <template v-if="ds.latest_sync_log">
                <span class="ds-card__sep">·</span>
                <span
                  class="ds-card__sync-result"
                  :class="`ds-card__sync-result--${ds.latest_sync_log.status}`"
                >
                  {{ lastSyncStatusLabel(ds) }}
                </span>
                <span
                  v-for="pill in syncResultPills(ds)"
                  :key="pill.cls"
                  class="ds-card__metric"
                >{{ pill.text }}</span>
              </template>
            </p>
            <div v-if="ds.error_message" class="ds-card__error">
              <t-icon name="error-circle-filled" size="14px" />
              <span>{{ ds.error_message }}</span>
            </div>
          </div>
        </component>
        </template>

        <button
          v-if="canManageDataSource"
          type="button"
          class="ds-card ds-card--add"
          @click="openCreate"
        >
          <span class="ds-card--add__icon" aria-hidden="true">
            <t-icon name="add" />
          </span>
          <span class="ds-card--add__label">{{ t('datasource.add') }}</span>
        </button>
      </div>
    </t-loading>

    <DataSourceEditorDialog
      v-model:visible="editorVisible"
      :kb-id="kbId"
      :data-source="editingDs"
      @saved="onEditorSaved"
    />

    <DataSourceSyncLogs
      v-model:visible="logsVisible"
      :data-source-id="logsDsId"
      :data-source-name="logsDsName"
    />

    <DatabaseSchemaDialog
      v-model:visible="schemaDialogVisible"
      :kb-id="kbId"
      :data-source-id="activeDatabaseDataSource?.id"
      :data-source-name="activeDatabaseDataSource?.name"
    />

    <DatabaseQueryAuditDialog
      v-model:visible="auditDialogVisible"
      :kb-id="kbId"
      :data-source-name="activeDatabaseDataSource?.name"
    />
  </div>
</template>

<style scoped lang="less">
@import './datasource-surface.less';
.ds-settings {
  width: 100%;
}

.section-header {
  margin-bottom: 20px;

  h2 {
    font-size: 16px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin: 0 0 6px 0;
  }

  .section-description {
    font-size: 13px;
    color: var(--td-text-color-secondary);
    margin: 0;
    line-height: 1.6;
  }
}

.ds-list-loading {
  min-height: 120px;
}

.empty-state {
  padding: 32px 0;
}

.ds-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 12px;

  // 数据库型数据源信息量大，独占整行以避免与添加按钮并排导致子表格列宽压缩
  .ds-card--database {
    grid-column: 1 / -1;
  }

  .ds-card--add {
    width: 100%;
    min-height: 80px;
  }
}

.ds-db-actions {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.ds-db-summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px solid var(--td-border-level-1-color);
}

.ds-db-summary__item {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.ds-db-summary__label {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.ds-db-summary__value {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ds-db-summary__value--success {
  color: var(--td-success-color-5);
}

.ds-card {
  position: relative;
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 14px 16px;
  .ds-surface-card();
  text-align: left;
  font: inherit;
  color: inherit;
  min-width: 0;

  &--clickable {
    cursor: pointer;
    width: 100%;
    .ds-surface-card--interactive();
  }

  &--add {
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    min-height: 68px;
    border-style: dashed;
    background: transparent;
    color: var(--td-text-color-placeholder);
    cursor: pointer;
    width: 100%;

    &:hover,
    &:focus-visible {
      color: var(--td-brand-color);
      border-color: var(--td-brand-color);
      background: color-mix(in srgb, var(--td-brand-color) 6%, transparent);
      box-shadow: none;
    }

    &:focus-visible {
      outline: 2px solid var(--td-brand-color);
      outline-offset: 2px;
    }

    &__icon {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 32px;
      height: 32px;
      border-radius: 8px;
      background: color-mix(in srgb, var(--td-brand-color) 10%, transparent);
      color: var(--td-brand-color);
      font-size: 18px;
    }

    &__label {
      font-size: 13px;
      font-weight: 500;
      line-height: 1.4;
    }
  }

  &__badge {
    flex-shrink: 0;
    width: 36px;
    height: 36px;
    border-radius: 9px;
    display: flex;
    align-items: center;
    justify-content: center;
    margin-top: 1px;
    font-size: 15px;
    font-weight: 600;
    letter-spacing: 0.02em;
    background: rgba(7, 192, 95, 0.12);
    color: #07c05f;
    overflow: hidden;
  }

  &--feishu .ds-card__badge,
  &--notion .ds-card__badge,
  &--yuque .ds-card__badge,
  &--rss .ds-card__badge {
    background: var(--td-bg-color-container, #fff);
    box-shadow: inset 0 0 0 1px var(--td-component-stroke);
  }

  &__body {
    flex: 1;
    min-width: 0;
  }

  &__header {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }

  &__title {
    flex: 1;
    min-width: 0;
    margin: 0;
    font-size: 14px;
    font-weight: 600;
    line-height: 1.4;
    color: var(--td-text-color-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__subtitle {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 4px;
    margin: 2px 0 0;
    font-size: 12px;
    line-height: 1.5;
    color: var(--td-text-color-secondary);
    min-width: 0;
  }

  &__status {
    display: inline-flex;
    align-items: center;
    gap: 4px;

    &--active {
      color: var(--td-success-color);
    }

    &--paused {
      color: var(--td-warning-color);
    }

    &--error {
      color: var(--td-error-color);
    }
  }

  &__detail {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 4px;
    margin: 4px 0 0;
    font-size: 12px;
    line-height: 1.45;
    color: var(--td-text-color-placeholder);
    min-width: 0;
  }

  &__sync-result {
    font-weight: 500;
    color: var(--td-text-color-secondary);

    &--success {
      color: var(--td-success-color);
    }

    &--failed {
      color: var(--td-error-color);
    }

    &--running {
      color: var(--td-brand-color);
    }

    &--partial {
      color: var(--td-warning-color);
    }
  }

  &__metric {
    font-size: 11px;
    font-variant-numeric: tabular-nums;
    color: var(--td-text-color-disabled);
  }

  &__sep {
    color: var(--td-text-color-disabled);
    user-select: none;
  }

  &__error {
    display: flex;
    align-items: flex-start;
    gap: 6px;
    margin-top: 8px;
    padding: 8px 10px;
    border-radius: 6px;
    background: var(--td-error-color-1);
    color: var(--td-error-color);
    font-size: 12px;
    line-height: 1.45;
    text-align: left;
  }

  &__actions {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 2px;
    margin-left: auto;
  }

  &__action-btn {
    flex-shrink: 0;
    padding: 2px;
    opacity: 0;
    color: var(--td-text-color-placeholder);
    transition: opacity 0.15s ease, color 0.15s ease;

    &:hover,
    &:focus-visible {
      background: var(--td-bg-color-secondarycontainer);
      color: var(--td-text-color-primary);
    }
  }

  &:hover .ds-card__action-btn,
  &:focus-within .ds-card__action-btn,
  &__actions:focus-within .ds-card__action-btn {
    opacity: 1;
  }
}

.ds-status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
  flex-shrink: 0;
}

.ds-icon-spin {
  animation: ds-spin 1s linear infinite;
}

@keyframes ds-spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

:deep(.t-dropdown__item.ds-dropdown-delete-item) {
  border-top: 1px solid var(--td-component-stroke);
  margin-top: 4px;
  padding-top: 4px;

  .t-popup__reference {
    display: block;
    width: 100%;
  }
}

.ds-dropdown-delete-trigger {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  cursor: pointer;
  line-height: 22px;
}
</style>
