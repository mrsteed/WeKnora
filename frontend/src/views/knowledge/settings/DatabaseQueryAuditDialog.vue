<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { listDatabaseQueryAudits, type DatabaseQueryAuditLog } from '@/api/datasource'

const props = defineProps<{
  kbId: string
  dataSourceName?: string
}>()

const visible = defineModel<boolean>('visible', { default: false })
const { t } = useI18n()

const loading = ref(false)
const errorMessage = ref('')
const items = ref<DatabaseQueryAuditLog[]>([])
const total = ref(0)
const limit = ref(10)
const offset = ref(0)

function translateOrFallback(key: string, fallback: string) {
  const translated = t(key)
  return translated === key ? fallback : translated
}

function formatDateTime(value?: string) {
  if (!value) return '--'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function statusTheme(status: string): 'success' | 'danger' | 'warning' | 'default' {
  switch (status) {
    case 'success': return 'success'
    case 'failed': return 'danger'
    case 'rejected': return 'warning'
    default: return 'default'
  }
}

async function loadAudits() {
  if (!props.kbId) return
  loading.value = true
  errorMessage.value = ''
  try {
    const res: any = await listDatabaseQueryAudits(props.kbId, limit.value, offset.value)
    const data = res?.data || res || { items: [], total: 0, limit: limit.value, offset: offset.value }
    items.value = Array.isArray(data.items) ? data.items : []
    total.value = Number(data.total || 0)
    limit.value = Number(data.limit || limit.value)
    offset.value = Number(data.offset || offset.value)
  } catch (error: any) {
    items.value = []
    total.value = 0
    errorMessage.value = error?.message || error?.error || translateOrFallback('datasource.auditLoadFailed', '加载查询审计失败')
  } finally {
    loading.value = false
  }
}

function prevPage() {
  if (offset.value === 0) return
  offset.value = Math.max(0, offset.value - limit.value)
  loadAudits()
}

function nextPage() {
  if (offset.value + limit.value >= total.value) return
  offset.value += limit.value
  loadAudits()
}

const pageSummary = computed(() => {
  if (total.value === 0) return '0 / 0'
  const start = offset.value + 1
  const end = Math.min(offset.value + items.value.length, total.value)
  return `${start}-${end} / ${total.value}`
})

watch(visible, (value) => {
  if (value) {
    offset.value = 0
    loadAudits()
  }
})

watch(() => props.kbId, () => {
  if (visible.value) {
    offset.value = 0
    loadAudits()
  }
})
</script>

<template>
  <t-dialog
    v-model:visible="visible"
    :header="translateOrFallback('datasource.auditDialogTitle', '数据库查询审计')"
    :footer="false"
    width="920px"
    destroy-on-close
  >
    <div class="db-audit-toolbar">
      <div class="db-audit-subtitle">{{ props.dataSourceName || translateOrFallback('datasource.auditDialogSubtitle', '查看当前知识库下的外部数据库查询审计记录') }}</div>
      <t-button size="small" variant="outline" @click="loadAudits">
        <template #icon><t-icon name="refresh" /></template>
        {{ translateOrFallback('common.refresh', '刷新') }}
      </t-button>
    </div>

    <div v-if="loading" class="db-audit-state">
      <t-loading size="small" />
    </div>

    <div v-else-if="errorMessage" class="db-audit-error">
      <t-icon name="error-circle-filled" size="16px" />
      <span>{{ errorMessage }}</span>
    </div>

    <div v-else-if="items.length === 0" class="db-audit-state">
      <t-empty :description="translateOrFallback('datasource.auditEmpty', '当前没有数据库查询审计记录')" />
    </div>

    <div v-else class="db-audit-list">
      <div v-for="item in items" :key="item.id" class="db-audit-card">
        <div class="db-audit-card-header">
          <div class="db-audit-card-meta">
            <strong>{{ formatDateTime(item.created_at) }}</strong>
            <span>{{ translateOrFallback('datasource.audit.user', '用户') }}: {{ item.user_id || '--' }}</span>
            <span>{{ translateOrFallback('datasource.audit.kb', '知识库') }}: {{ item.knowledge_base_id || '--' }}</span>
            <span>{{ translateOrFallback('datasource.audit.dataSource', '数据源') }}: {{ item.data_source_id || '--' }}</span>
            <span>{{ translateOrFallback('datasource.audit.rows', '行数') }}: {{ item.row_count }}</span>
            <span>{{ translateOrFallback('datasource.audit.duration', '耗时') }}: {{ item.duration_ms }} ms</span>
          </div>
          <t-tag size="small" :theme="statusTheme(item.status)" variant="light-outline">
            {{ item.status }}
          </t-tag>
        </div>

        <div v-if="item.purpose" class="db-audit-purpose">
          <span class="db-audit-label">{{ translateOrFallback('datasource.audit.purpose', '目的') }}</span>
          <span>{{ item.purpose }}</span>
        </div>

        <div class="db-audit-sql-block">
          <div class="db-audit-label">{{ translateOrFallback('datasource.audit.executedSql', '执行 SQL') }}</div>
          <pre>{{ item.executed_sql || item.original_sql }}</pre>
        </div>

        <div v-if="item.executed_sql && item.executed_sql !== item.original_sql" class="db-audit-sql-block">
          <div class="db-audit-label">{{ translateOrFallback('datasource.audit.originalSql', '原始 SQL') }}</div>
          <pre>{{ item.original_sql }}</pre>
        </div>

        <div v-if="item.error_message" class="db-audit-error-inline">
          <span class="db-audit-label">{{ translateOrFallback('datasource.audit.error', '错误') }}</span>
          <span>{{ item.error_message }}</span>
        </div>
      </div>

      <div class="db-audit-footer">
        <span>{{ pageSummary }}</span>
        <div class="db-audit-pager">
          <t-button size="small" variant="outline" :disabled="offset === 0" @click="prevPage">
            {{ translateOrFallback('common.previous', '上一页') }}
          </t-button>
          <t-button size="small" variant="outline" :disabled="offset + limit >= total" @click="nextPage">
            {{ translateOrFallback('common.next', '下一页') }}
          </t-button>
        </div>
      </div>
    </div>
  </t-dialog>
</template>

<style scoped>
.db-audit-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
}

.db-audit-subtitle {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.db-audit-state {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px 0;
}

.db-audit-error,
.db-audit-error-inline {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 14px;
  border-radius: 10px;
  background: var(--td-error-color-1);
  color: var(--td-error-color-7);
}

.db-audit-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.db-audit-card {
  padding: 14px 16px;
  border: 1px solid var(--td-border-level-1-color);
  border-radius: 12px;
  background: var(--td-bg-color-container);
}

.db-audit-card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.db-audit-card-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 14px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.db-audit-purpose,
.db-audit-sql-block,
.db-audit-error-inline {
  margin-top: 12px;
}

.db-audit-label {
  display: block;
  margin-bottom: 6px;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.db-audit-sql-block pre {
  margin: 0;
  padding: 12px;
  border-radius: 10px;
  background: var(--td-bg-color-page);
  color: var(--td-text-color-primary);
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 12px;
  line-height: 18px;
}

.db-audit-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding-top: 4px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.db-audit-pager {
  display: flex;
  gap: 8px;
}

@media (max-width: 640px) {
  .db-audit-toolbar,
  .db-audit-card-header,
  .db-audit-footer {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>