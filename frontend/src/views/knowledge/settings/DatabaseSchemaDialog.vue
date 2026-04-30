<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { getDatabaseSchema, refreshDataSourceSchema, type DatabaseSchema, type DatabaseSchemaTable } from '@/api/datasource'

const props = defineProps<{
  kbId: string
  dataSourceId?: string
  dataSourceName?: string
}>()

const visible = defineModel<boolean>('visible', { default: false })
const { t } = useI18n()

const loading = ref(false)
const refreshing = ref(false)
const errorMessage = ref('')
const schema = ref<DatabaseSchema | null>(null)
const expandedTables = ref<string[]>([])

function translateOrFallback(key: string, fallback: string) {
  const translated = t(key)
  return translated === key ? fallback : translated
}

function formatDateTime(value?: string) {
  if (!value) return '--'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function tableSummary(table: DatabaseSchemaTable) {
  const type = table.type || 'table'
  const count = Array.isArray(table.columns) ? table.columns.length : 0
  return `${table.name} · ${type} · ${count} 列`
}

async function loadSchema() {
  if (!props.kbId) return
  loading.value = true
  errorMessage.value = ''
  try {
    const res: any = await getDatabaseSchema(props.kbId)
    const next = (res?.data || res || null) as DatabaseSchema | null
    schema.value = next
    expandedTables.value = (next?.tables || []).slice(0, 8).map(table => table.name)
  } catch (error: any) {
    schema.value = null
    errorMessage.value = error?.message || error?.error || translateOrFallback('datasource.schemaLoadFailed', '加载 Schema 失败')
  } finally {
    loading.value = false
  }
}

async function handleRefreshSchema() {
  if (!props.dataSourceId) return
  refreshing.value = true
  try {
    await refreshDataSourceSchema(props.dataSourceId)
    MessagePlugin.success(translateOrFallback('datasource.refreshSchemaSuccess', 'Schema 已刷新'))
    await loadSchema()
  } catch (error: any) {
    MessagePlugin.error(error?.message || error?.error || translateOrFallback('datasource.refreshSchemaFailed', '刷新 Schema 失败'))
  } finally {
    refreshing.value = false
  }
}

watch(visible, (value) => {
  if (value) {
    loadSchema()
  }
})

watch(() => props.kbId, () => {
  if (visible.value) {
    loadSchema()
  }
})

const schemaMeta = computed(() => {
  if (!schema.value) return []
  return [
    { label: translateOrFallback('datasource.schema.databaseType', '数据库类型'), value: schema.value.database_type || '--' },
    { label: translateOrFallback('datasource.schema.databaseName', '数据库名'), value: schema.value.database_name || '--' },
    { label: translateOrFallback('datasource.schema.schemaName', 'Schema'), value: schema.value.schema_name || '--' },
    { label: translateOrFallback('datasource.schema.refreshedAt', '刷新时间'), value: formatDateTime(schema.value.refreshed_at) },
  ]
})
</script>

<template>
  <t-dialog
    v-model:visible="visible"
    :header="translateOrFallback('datasource.schemaDialogTitle', '数据库结构')"
    :footer="false"
    width="960px"
    destroy-on-close
  >
    <div class="db-schema-toolbar">
      <div class="db-schema-subtitle">{{ props.dataSourceName || translateOrFallback('datasource.schemaDialogSubtitle', '查看当前知识库缓存的数据库结构快照') }}</div>
      <t-button v-if="props.dataSourceId" size="small" theme="primary" :loading="refreshing" @click="handleRefreshSchema">
        <template #icon><t-icon name="refresh" /></template>
        {{ translateOrFallback('datasource.refreshSchema', '刷新 Schema') }}
      </t-button>
    </div>

    <div v-if="loading" class="db-schema-state">
      <t-loading size="small" />
    </div>

    <div v-else-if="errorMessage" class="db-schema-error">
      <t-icon name="error-circle-filled" size="16px" />
      <span>{{ errorMessage }}</span>
    </div>

    <div v-else-if="!schema || !schema.tables || schema.tables.length === 0" class="db-schema-state">
      <t-empty :description="translateOrFallback('datasource.schemaEmpty', '当前还没有可展示的数据库结构，请先刷新 Schema。')" />
    </div>

    <div v-else class="db-schema-content">
      <div class="db-schema-meta-grid">
        <div v-for="item in schemaMeta" :key="item.label" class="db-schema-meta-card">
          <span class="db-schema-meta-label">{{ item.label }}</span>
          <strong class="db-schema-meta-value">{{ item.value }}</strong>
        </div>
      </div>

      <t-collapse v-model="expandedTables">
        <t-collapse-panel v-for="table in schema.tables" :key="table.name" :value="table.name">
          <template #header>
            <div class="db-schema-panel-header">
              <span class="db-schema-panel-title">{{ tableSummary(table) }}</span>
              <span v-if="table.comment" class="db-schema-panel-comment">{{ table.comment }}</span>
            </div>
          </template>

          <div class="db-schema-table-wrap">
            <div class="db-schema-table-meta">
              <span>{{ translateOrFallback('datasource.schema.primaryKeys', '主键') }}: {{ table.primary_keys?.length ? table.primary_keys.join(', ') : '--' }}</span>
              <span>{{ translateOrFallback('datasource.schema.rowEstimate', '估算行数') }}: {{ table.row_estimate ?? '--' }}</span>
            </div>

            <table class="db-schema-table">
              <thead>
                <tr>
                  <th>{{ translateOrFallback('datasource.schema.columnName', '字段') }}</th>
                  <th>{{ translateOrFallback('datasource.schema.columnType', '类型') }}</th>
                  <th>{{ translateOrFallback('datasource.schema.nullable', '可空') }}</th>
                  <th>{{ translateOrFallback('datasource.schema.sensitive', '敏感') }}</th>
                  <th>{{ translateOrFallback('datasource.schema.comment', '注释') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="column in table.columns || []" :key="`${table.name}-${column.name}`">
                  <td>{{ column.name }}</td>
                  <td>{{ column.data_type }}</td>
                  <td>{{ column.nullable ? 'YES' : 'NO' }}</td>
                  <td>
                    <t-tag size="small" :theme="column.is_sensitive ? 'danger' : 'default'" variant="light-outline">
                      {{ column.is_sensitive ? translateOrFallback('datasource.schema.sensitiveYes', '是') : translateOrFallback('datasource.schema.sensitiveNo', '否') }}
                    </t-tag>
                  </td>
                  <td>{{ column.comment || '--' }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </t-collapse-panel>
      </t-collapse>
    </div>
  </t-dialog>
</template>

<style scoped>
.db-schema-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
}

.db-schema-subtitle {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.db-schema-state {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px 0;
}

.db-schema-error {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 14px;
  border-radius: 10px;
  background: var(--td-error-color-1);
  color: var(--td-error-color-7);
}

.db-schema-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.db-schema-meta-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.db-schema-meta-card {
  padding: 12px 14px;
  border: 1px solid var(--td-border-level-1-color);
  border-radius: 12px;
  background: var(--td-bg-color-container);
}

.db-schema-meta-label {
  display: block;
  margin-bottom: 6px;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.db-schema-meta-value {
  font-size: 14px;
  color: var(--td-text-color-primary);
}

.db-schema-panel-header {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.db-schema-panel-title {
  font-weight: 600;
}

.db-schema-panel-comment {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.db-schema-table-wrap {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.db-schema-table-meta {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.db-schema-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

.db-schema-table th,
.db-schema-table td {
  padding: 10px 12px;
  border-bottom: 1px solid var(--td-border-level-1-color);
  text-align: left;
  vertical-align: top;
}

.db-schema-table th {
  color: var(--td-text-color-secondary);
  font-weight: 600;
}

@media (max-width: 900px) {
  .db-schema-meta-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .db-schema-toolbar {
    flex-direction: column;
    align-items: stretch;
  }

  .db-schema-meta-grid {
    grid-template-columns: 1fr;
  }

  .db-schema-table {
    display: block;
    overflow-x: auto;
    white-space: nowrap;
  }
}
</style>