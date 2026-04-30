<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import DataSourceSettings from '../settings/DataSourceSettings.vue'
import {
  getDatabaseSchema,
  listDataSources,
  listDatabaseQueryAudits,
  type DataSource,
  type DatabaseSchema,
} from '@/api/datasource'

const props = defineProps<{
  kbId: string
  kbInfo?: Record<string, any> | null
}>()

const { t } = useI18n()
const overviewLoading = ref(false)
const dataSources = ref<DataSource[]>([])
const schema = ref<DatabaseSchema | null>(null)
const auditTotal = ref(0)
const latestAuditAt = ref('')

function translateOrFallback(key: string, fallback: string) {
  const translated = t(key)
  return translated === key ? fallback : translated
}

function formatDateTime(value?: string) {
  if (!value) return '--'
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString()
}

function connectorLabel(type?: string) {
  switch (type) {
    case 'mysql':
      return 'MySQL'
    case 'postgresql':
      return 'PostgreSQL'
    default:
      return type || '--'
  }
}

function getStatusTheme(status?: string): 'success' | 'warning' | 'danger' | 'default' {
  if (status === 'active') return 'success'
  if (status === 'paused') return 'warning'
  if (status === 'error') return 'danger'
  return 'default'
}

const primaryDataSource = computed(() => dataSources.value[0] || null)
const primarySettings = computed<Record<string, any>>(() => {
  const cfg = primaryDataSource.value?.config as Record<string, any> | undefined
  return (cfg?.settings as Record<string, any>) || {}
})

const businessDatabaseName = computed(() => schema.value?.database_name || primarySettings.value.database || '--')
const businessSchemaName = computed(() => schema.value?.schema_name || primarySettings.value.schema || '--')
const tableCount = computed(() => schema.value?.tables?.filter(table => table.type !== 'view').length || 0)
const viewCount = computed(() => schema.value?.tables?.filter(table => table.type === 'view').length || 0)
const columnCount = computed(() => schema.value?.tables?.reduce((count, table) => count + (table.columns?.length || 0), 0) || 0)
const schemaRefreshText = computed(() => formatDateTime(schema.value?.refreshed_at))
const latestAuditText = computed(() => formatDateTime(latestAuditAt.value))
const queryLimitText = computed(() => primarySettings.value.max_rows ? String(primarySettings.value.max_rows) : '--')
const timeoutText = computed(() => primarySettings.value.query_timeout_sec ? `${primarySettings.value.query_timeout_sec}s` : '--')
const allowedTableCount = computed(() => Array.isArray(primarySettings.value.table_allowlist) ? primarySettings.value.table_allowlist.length : 0)
const sampleRowsText = computed(() => primarySettings.value.sample_rows ? `${primarySettings.value.sample_rows}` : '--')
const connectionAddress = computed(() => {
  if (!primaryDataSource.value) return '--'
  return `${primarySettings.value.host || '--'}:${primarySettings.value.port || '--'}`
})

const overviewCards = computed(() => [
  {
    key: 'datasource',
    tagLabel: translateOrFallback('knowledgeBase.databaseDetail.cards.datasource.tag', '连接'),
    eyebrow: translateOrFallback('knowledgeBase.databaseDetail.cards.datasource.eyebrow', '业务库接入'),
    title: primaryDataSource.value?.name || translateOrFallback('knowledgeBase.databaseDetail.cards.datasource.empty', '未配置数据源'),
    value: businessDatabaseName.value,
    suffix: translateOrFallback('knowledgeBase.databaseDetail.cards.datasource.suffix', '业务库'),
    meta: primaryDataSource.value
      ? `${connectorLabel(primaryDataSource.value.type)} · ${primarySettings.value.host || '--'}:${primarySettings.value.port || '--'} · ${translateOrFallback(`datasource.status.${primaryDataSource.value.status}`, primaryDataSource.value.status || '--')}`
      : translateOrFallback('knowledgeBase.databaseDetail.cards.datasource.meta', '配置连接后即可读取业务数据库结构'),
    details: [
      {
        label: translateOrFallback('knowledgeBase.databaseDetail.cards.datasource.connector', '接入方式'),
        value: connectorLabel(primaryDataSource.value?.type),
      },
      {
        label: translateOrFallback('knowledgeBase.databaseDetail.cards.datasource.address', '连接地址'),
        value: connectionAddress.value,
      },
      {
        label: translateOrFallback('knowledgeBase.databaseDetail.cards.datasource.refreshTime', '最近刷新'),
        value: schemaRefreshText.value,
      },
    ],
    theme: getStatusTheme(primaryDataSource.value?.status),
  },
  {
    key: 'schema',
    tagLabel: translateOrFallback('knowledgeBase.databaseDetail.cards.schema.tag', 'Schema'),
    eyebrow: translateOrFallback('knowledgeBase.databaseDetail.cards.schema.eyebrow', '可查询结构'),
    title: businessSchemaName.value !== '--' ? businessSchemaName.value : translateOrFallback('knowledgeBase.databaseDetail.cards.schema.empty', '等待首次刷新'),
    value: String(tableCount.value),
    suffix: translateOrFallback('knowledgeBase.databaseDetail.cards.schema.suffix', '张业务表'),
    meta: `${columnCount.value} ${translateOrFallback('knowledgeBase.databaseDetail.cards.schema.columns', '个字段')} · ${viewCount.value} ${translateOrFallback('knowledgeBase.databaseDetail.cards.schema.views', '个视图')}`,
    details: [
      {
        label: translateOrFallback('knowledgeBase.databaseDetail.cards.schema.database', '业务库'),
        value: businessDatabaseName.value,
      },
      {
        label: translateOrFallback('knowledgeBase.databaseDetail.cards.schema.fields', '字段总量'),
        value: `${columnCount.value}`,
      },
      {
        label: translateOrFallback('knowledgeBase.databaseDetail.cards.schema.viewsLabel', '视图数量'),
        value: `${viewCount.value}`,
      },
    ],
    theme: 'primary' as const,
  },
  {
    key: 'guard',
    tagLabel: translateOrFallback('knowledgeBase.databaseDetail.cards.guard.tag', '约束'),
    eyebrow: translateOrFallback('knowledgeBase.databaseDetail.cards.guard.eyebrow', '查询保护'),
    title: translateOrFallback('knowledgeBase.databaseDetail.cards.guard.title', '只读查询边界'),
    value: queryLimitText.value,
    suffix: translateOrFallback('knowledgeBase.databaseDetail.cards.guard.suffix', '行返回上限'),
    meta: allowedTableCount.value > 0
      ? `${translateOrFallback('knowledgeBase.databaseDetail.cards.guard.scope', '已限定')} ${allowedTableCount.value} ${translateOrFallback('knowledgeBase.databaseDetail.cards.guard.scopeSuffix', '张表')} · ${translateOrFallback('knowledgeBase.databaseDetail.cards.guard.timeout', '超时')} ${timeoutText.value}`
      : `${translateOrFallback('knowledgeBase.databaseDetail.cards.guard.timeout', '超时')} ${timeoutText.value}`,
    details: [
      {
        label: translateOrFallback('knowledgeBase.databaseDetail.cards.guard.timeoutLabel', '执行超时'),
        value: timeoutText.value,
      },
      {
        label: translateOrFallback('knowledgeBase.databaseDetail.cards.guard.scopeLabel', '授权表数'),
        value: allowedTableCount.value > 0 ? `${allowedTableCount.value}` : '--',
      },
      {
        label: translateOrFallback('knowledgeBase.databaseDetail.cards.guard.sampleRows', '示例采样'),
        value: sampleRowsText.value,
      },
    ],
    theme: 'warning' as const,
  },
  {
    key: 'audit',
    tagLabel: translateOrFallback('knowledgeBase.databaseDetail.cards.audit.tag', '审计'),
    eyebrow: translateOrFallback('knowledgeBase.databaseDetail.cards.audit.eyebrow', '访问审计'),
    title: translateOrFallback('knowledgeBase.databaseDetail.cards.audit.title', '查询留痕'),
    value: String(auditTotal.value),
    suffix: translateOrFallback('knowledgeBase.databaseDetail.cards.audit.suffix', '条记录'),
    meta: `${translateOrFallback('knowledgeBase.databaseDetail.cards.audit.latest', '最近一次')} ${latestAuditText.value}`,
    details: [
      {
        label: translateOrFallback('knowledgeBase.databaseDetail.cards.audit.latestLabel', '最新查询'),
        value: latestAuditText.value,
      },
      {
        label: translateOrFallback('knowledgeBase.databaseDetail.cards.audit.statusLabel', '审计状态'),
        value: auditTotal.value > 0
          ? translateOrFallback('knowledgeBase.databaseDetail.cards.audit.statusReady', '已留痕')
          : translateOrFallback('knowledgeBase.databaseDetail.cards.audit.statusEmpty', '暂无记录'),
      },
      {
        label: translateOrFallback('knowledgeBase.databaseDetail.cards.audit.scopeLabel', '关联表范围'),
        value: allowedTableCount.value > 0 ? `${allowedTableCount.value}` : '--',
      },
    ],
    theme: 'default' as const,
  },
])

async function loadOverview() {
  if (!props.kbId) return
  overviewLoading.value = true
  try {
    const [dataSourceRes, schemaRes, auditRes] = await Promise.allSettled([
      listDataSources(props.kbId),
      getDatabaseSchema(props.kbId),
      listDatabaseQueryAudits(props.kbId, 1, 0),
    ])

    if (dataSourceRes.status === 'fulfilled') {
      dataSources.value = (dataSourceRes.value as any)?.data || dataSourceRes.value || []
    } else {
      dataSources.value = []
    }

    if (schemaRes.status === 'fulfilled') {
      schema.value = ((schemaRes.value as any)?.data || schemaRes.value || null) as DatabaseSchema | null
    } else {
      schema.value = null
    }

    if (auditRes.status === 'fulfilled') {
      const payload = (auditRes.value as any)?.data || auditRes.value || {}
      auditTotal.value = Number(payload.total || 0)
      latestAuditAt.value = payload.items?.[0]?.created_at || ''
    } else {
      auditTotal.value = 0
      latestAuditAt.value = ''
    }
  } finally {
    overviewLoading.value = false
  }
}

watch(() => props.kbId, () => {
  loadOverview()
}, { immediate: true })
</script>

<template>
  <div class="database-overview">
    <section class="database-card-grid">
      <article v-for="card in overviewCards" :key="card.key" :class="['database-summary-card', `theme-${card.theme}`]">
        <div class="card-eyebrow-row">
          <span class="card-eyebrow">{{ card.eyebrow }}</span>
          <t-tag size="small" variant="light-outline" :theme="card.theme">{{ card.tagLabel }}</t-tag>
        </div>
        <div class="card-title">{{ card.title }}</div>
        <div class="card-metric-row">
          <strong class="card-metric">{{ card.value }}</strong>
          <span class="card-metric-suffix">{{ card.suffix }}</span>
        </div>
        <div class="card-meta">{{ card.meta }}</div>
        <div class="card-detail-grid">
          <div v-for="detail in card.details" :key="detail.label" class="card-detail-item">
            <span class="card-detail-label">{{ detail.label }}</span>
            <strong class="card-detail-value">{{ detail.value }}</strong>
          </div>
        </div>
      </article>
    </section>

    <DataSourceSettings :kb-id="kbId" @database-change="loadOverview" />
  </div>
</template>

<style scoped>
.database-overview {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.database-summary-card {
  border: 1px solid rgba(0, 0, 0, 0.06);
  border-radius: 16px;
}

.card-eyebrow {
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--td-brand-color);
}
.card-meta {
  margin: 0;
  font-size: 13px;
  line-height: 22px;
  color: var(--td-text-color-secondary);
}
.card-detail-label,
.card-detail-label {
  font-size: 13px;
  color: var(--td-text-color-placeholder);
}

.database-card-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.database-summary-card {
  position: relative;
  overflow: hidden;
  padding: 20px 20px 18px;
  background: linear-gradient(180deg, #ffffff 0%, #fcfcfc 100%);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  transition: border-color 0.16s ease, box-shadow 0.16s ease, transform 0.16s ease;
}

.database-summary-card::before {
  content: '';
  position: absolute;
  inset: 0 auto auto 0;
  width: 100%;
  height: 4px;
  background: linear-gradient(90deg, rgba(7, 192, 95, 0.75) 0%, rgba(7, 192, 95, 0.1) 100%);
}

.database-summary-card:hover {
  border-color: rgba(7, 192, 95, 0.24);
  box-shadow: 0 8px 24px rgba(7, 192, 95, 0.12);
  transform: translateY(-1px);
}

.database-summary-card.theme-primary::before {
  background: linear-gradient(90deg, rgba(0, 82, 217, 0.82) 0%, rgba(0, 82, 217, 0.12) 100%);
}

.database-summary-card.theme-warning::before {
  background: linear-gradient(90deg, rgba(237, 108, 2, 0.82) 0%, rgba(237, 108, 2, 0.12) 100%);
}

.database-summary-card.theme-default::before {
  background: linear-gradient(90deg, rgba(96, 125, 139, 0.72) 0%, rgba(96, 125, 139, 0.12) 100%);
}

.card-eyebrow-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.card-title {
  margin-top: 14px;
  font-size: 17px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.card-metric-row {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-top: 18px;
}

.card-metric {
  font-size: 30px;
  line-height: 1;
  color: var(--td-text-color-primary);
}

.card-metric-suffix {
  font-size: 13px;
  color: var(--td-text-color-secondary);
}

.card-detail-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  margin-top: 18px;
  padding-top: 16px;
  border-top: 1px solid var(--td-border-level-1-color);
}

.card-detail-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.card-detail-value {
  font-size: 13px;
  line-height: 20px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  word-break: break-all;
}

@media (max-width: 1200px) {
  .card-detail-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 900px) {
  .database-card-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 640px) {
  .database-summary-card {
    padding-left: 16px;
    padding-right: 16px;
  }

  .card-detail-grid {
    grid-template-columns: 1fr;
  }
}
</style>