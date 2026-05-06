<template>
  <div class="external-database-schema-display">
    <div class="schema-summary">
      <div class="summary-grid">
        <div class="summary-item">
          <span class="summary-label">{{ t('chat.schemaOutputModeLabel') }}</span>
          <strong>{{ data.mode || 'detail' }}</strong>
        </div>
        <div class="summary-item">
          <span class="summary-label">{{ t('chat.schemaTableCountLabel') }}</span>
          <strong>{{ data.table_count ?? 0 }}</strong>
        </div>
        <div class="summary-item">
          <span class="summary-label">{{ t('chat.schemaColumnCountLabel') }}</span>
          <strong>{{ data.column_count ?? 0 }}</strong>
        </div>
        <div v-if="data.refreshed_at" class="summary-item wide">
          <span class="summary-label">{{ t('chat.schemaUpdatedAtLabel') }}</span>
          <strong>{{ formatTimestamp(data.refreshed_at) }}</strong>
        </div>
        <div v-if="data.schema_hash" class="summary-item wide">
          <span class="summary-label">{{ t('chat.schemaHashLabel') }}</span>
          <strong class="hash-text">{{ data.schema_hash }}</strong>
        </div>
      </div>

      <div v-if="(data.additional_tables_omitted ?? 0) > 0 || (data.additional_columns_omitted ?? 0) > 0" class="schema-omitted">
        <span v-if="(data.additional_tables_omitted ?? 0) > 0">{{ t('chat.schemaAdditionalTablesOmitted', { count: data.additional_tables_omitted }) }}</span>
        <span v-if="(data.additional_columns_omitted ?? 0) > 0">{{ t('chat.schemaAdditionalColumnsOmitted', { count: data.additional_columns_omitted }) }}</span>
      </div>
    </div>

    <div v-if="data.allowed_tables?.length" class="schema-section">
      <div class="section-title">{{ t('chat.schemaAllowedTablesLabel') }}</div>
      <div class="pill-list">
        <span v-for="tableName in data.allowed_tables" :key="tableName" class="pill">{{ tableName }}</span>
      </div>
    </div>

    <div v-if="data.tables?.length" class="schema-table-list">
      <div v-for="table in data.tables" :key="table.name" class="table-card">
        <div class="table-header">
          <div>
            <div class="table-title">{{ table.name }}</div>
            <div class="table-subtitle">{{ table.type }}</div>
          </div>
          <div class="table-metrics">
            <span>{{ t('chat.schemaColumnCountLabel') }} {{ table.column_count ?? 0 }}</span>
            <span>{{ t('chat.schemaIndexesLabel') }} {{ table.index_count ?? 0 }}</span>
          </div>
        </div>

        <div v-if="table.comment" class="table-comment">{{ table.comment }}</div>

        <div v-if="table.primary_keys?.length" class="table-detail-row">
          <span class="detail-label">{{ t('chat.schemaPrimaryKeysLabel') }}</span>
          <span>{{ table.primary_keys.join(', ') }}</span>
        </div>

        <div v-if="(table.row_estimate ?? 0) > 0" class="table-detail-row">
          <span class="detail-label">{{ t('chat.schemaRowEstimateLabel') }}</span>
          <span>{{ table.row_estimate }}</span>
        </div>

        <div class="table-detail-block">
          <div class="detail-label">{{ t('chat.schemaColumnsLabel') }}</div>
          <ul class="detail-list">
            <li v-for="column in table.columns || []" :key="column.name">
              <span class="detail-main">{{ column.name }} {{ column.data_type }}</span>
              <span class="detail-meta">{{ column.nullable ? 'NULL' : 'NOT NULL' }}</span>
              <span v-if="column.is_sensitive" class="detail-meta sensitive">sensitive</span>
              <span v-if="column.comment" class="detail-comment">{{ column.comment }}</span>
            </li>
          </ul>
          <div v-if="(table.additional_columns_omitted ?? 0) > 0" class="detail-omitted">
            {{ t('chat.schemaAdditionalColumnsOmitted', { count: table.additional_columns_omitted }) }}
          </div>
        </div>

        <div v-if="table.indexes?.length" class="table-detail-block">
          <div class="detail-label">{{ t('chat.schemaIndexesLabel') }}</div>
          <ul class="detail-list compact">
            <li v-for="index in table.indexes" :key="index.name">
              <span class="detail-main">{{ index.name }}</span>
              <span class="detail-meta">{{ (index.columns || []).join(', ') }}</span>
              <span v-if="index.unique" class="detail-meta">unique</span>
            </li>
          </ul>
        </div>

        <div v-if="table.foreign_keys?.length" class="table-detail-block">
          <div class="detail-label">{{ t('chat.schemaForeignKeysLabel') }}</div>
          <ul class="detail-list compact">
            <li v-for="fk in table.foreign_keys" :key="foreignKeyKey(table.name, fk)">
              {{ formatForeignKey(fk) }}
            </li>
          </ul>
        </div>
      </div>
    </div>

    <div v-else class="empty-schema">{{ t('chat.schemaEmptyLabel') }}</div>

    <div v-if="data.foreign_keys?.length" class="schema-section">
      <div class="section-title">{{ t('chat.schemaForeignKeysLabel') }}</div>
      <ul class="detail-list compact">
        <li v-for="foreignKey in data.foreign_keys" :key="foreignKey">{{ foreignKey }}</li>
      </ul>
    </div>

    <div v-if="possibleJoinHints.length" class="schema-section">
      <div class="section-title">{{ t('chat.schemaPossibleJoinHintsLabel') }}</div>
      <ul class="detail-list compact">
        <li v-for="hint in possibleJoinHints" :key="hint">{{ hint }}</li>
      </ul>
    </div>

    <div v-if="data.sample_queries?.length" class="schema-section">
      <div class="section-title">{{ t('chat.schemaSampleQueriesLabel') }}</div>
      <ul class="detail-list compact mono">
        <li v-for="query in data.sample_queries" :key="query">{{ query }}</li>
      </ul>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { ExternalDatabaseSchemaData, ExternalDatabaseSchemaForeignKey } from '@/types/tool-results';
import { useI18n } from 'vue-i18n';

interface Props {
  data: ExternalDatabaseSchemaData;
}

const props = defineProps<Props>();
const { t } = useI18n();

const possibleJoinHints = computed(() => props.data.possible_join_hints || props.data.join_hints || []);

const formatForeignKey = (fk: ExternalDatabaseSchemaForeignKey): string => {
  const columns = (fk.columns || []).join(', ');
  const referencedColumns = (fk.referenced_columns || []).join(', ');
  if ((fk.columns || []).length === 1 && (fk.referenced_columns || []).length === 1) {
    return `${columns} -> ${fk.referenced_table}.${referencedColumns}`;
  }
  return `(${columns}) -> ${fk.referenced_table}(${referencedColumns})`;
};

const foreignKeyKey = (tableName: string, fk: ExternalDatabaseSchemaForeignKey): string => {
  return `${tableName}:${fk.name || formatForeignKey(fk)}`;
};

const formatTimestamp = (value?: string): string => {
  if (!value) {
    return '';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
};
</script>

<style lang="less" scoped>
.external-database-schema-display {
  display: flex;
  flex-direction: column;
  gap: 16px;
  font-size: 13px;
  color: var(--td-text-color-primary);
}

.schema-summary,
.schema-section,
.table-card,
.empty-schema {
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  background: var(--td-bg-color-container);
}

.schema-summary,
.schema-section,
.empty-schema {
  padding: 14px 16px;
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 10px;
}

.summary-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.summary-item.wide {
  grid-column: 1 / -1;
}

.summary-label,
.detail-label,
.section-title,
.table-subtitle {
  color: var(--td-text-color-secondary);
}

.hash-text {
  word-break: break-all;
}

.schema-omitted {
  margin-top: 12px;
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  color: var(--td-warning-color);
}

.pill-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.pill {
  padding: 4px 10px;
  border-radius: 999px;
  background: var(--td-bg-color-secondarycontainer);
  border: 1px solid var(--td-component-stroke);
  font-size: 12px;
}

.schema-table-list {
  display: grid;
  gap: 12px;
}

.table-card {
  padding: 14px 16px;
}

.table-header {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
}

.table-title {
  font-size: 15px;
  font-weight: 600;
}

.table-metrics {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.table-comment {
  margin-top: 10px;
  color: var(--td-text-color-secondary);
}

.table-detail-row {
  margin-top: 10px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.table-detail-block {
  margin-top: 12px;
}

.detail-list {
  margin: 8px 0 0;
  padding-left: 18px;
  display: grid;
  gap: 6px;
}

.detail-list.compact {
  gap: 4px;
}

.detail-list.mono {
  font-family: var(--app-font-family-mono);
}

.detail-main {
  font-weight: 500;
}

.detail-meta {
  margin-left: 8px;
  color: var(--td-text-color-secondary);
  font-size: 12px;
}

.detail-meta.sensitive {
  color: var(--td-error-color);
}

.detail-comment {
  display: block;
  color: var(--td-text-color-secondary);
}

.detail-omitted {
  margin-top: 8px;
  color: var(--td-warning-color);
}

.empty-schema {
  text-align: center;
  color: var(--td-text-color-placeholder);
}
</style>