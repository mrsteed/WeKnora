<template>
  <div class="database-query-display">
    <div class="results-summary">
      <div class="summary-grid">
        <div class="summary-item">
          <span class="summary-label">{{ t('chat.rowsLabel') }}</span>
          <strong>{{ data.row_count ?? 0 }}</strong>
        </div>
        <div class="summary-item">
          <span class="summary-label">{{ t('chat.columnsLabel') }}</span>
          <strong>{{ data.columns?.length ?? 0 }}</strong>
        </div>
        <div v-if="data.duration_ms !== undefined" class="summary-item">
          <span class="summary-label">{{ t('chat.queryDurationLabel') }}</span>
          <strong>{{ data.duration_ms }} ms</strong>
        </div>
        <div class="summary-item wide">
          <span class="summary-label">{{ t('chat.queryDisplayedRowsLabel', { displayed: displayedRows, total: data.row_count ?? 0 }) }}</span>
        </div>
      </div>

      <div v-if="statusMessages.length" class="summary-status-list">
        <span v-for="message in statusMessages" :key="message" class="status-pill">{{ message }}</span>
      </div>
    </div>

    <details v-if="data.executed_sql" class="sql-details">
      <summary>{{ t('chat.sqlQueryExecuted') }}</summary>
      <pre class="sql-code">{{ data.executed_sql }}</pre>
    </details>

    <div v-if="data.rows && data.rows.length > 0" class="results-table-container">
      <table class="results-table">
        <thead>
          <tr>
            <th v-for="column in data.columns" :key="column">
              <div>{{ column }}</div>
              <div v-if="getColumnType(column)" class="column-type">{{ getColumnType(column) }}</div>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(row, index) in data.rows" :key="index">
            <td v-for="column in data.columns" :key="column">
              {{ formatValue(row[column]) }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    
    <!-- No Results -->
    <div v-else class="no-results">
      {{ $t('chat.noDatabaseRecords') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { DatabaseQueryData } from '@/types/tool-results';
import { useI18n } from 'vue-i18n';

interface Props {
  data: DatabaseQueryData;
}

const props = defineProps<Props>();
const { t } = useI18n();

const displayedRows = computed(() => props.data.rows?.length ?? 0);

const columnTypeMap = computed<Record<string, string>>(() => {
  const definitions = props.data.column_definitions || [];
  return definitions.reduce<Record<string, string>>((acc, column) => {
    if (column.name && column.data_type) {
      acc[column.name] = column.data_type;
    }
    return acc;
  }, {});
});

const statusMessages = computed(() => {
  const messages: string[] = [];
  if (props.data.truncated) {
    messages.push(t('chat.queryDatabaseTruncatedLabel'));
  }
  if (props.data.output_truncated) {
    messages.push(t('chat.queryOutputTruncatedLabel'));
  }
  if ((props.data.row_count ?? 0) > displayedRows.value) {
    messages.push(t('chat.queryOmittedRowsLabel', { count: (props.data.row_count ?? 0) - displayedRows.value }));
  }
  if ((props.data.cell_truncated_count ?? 0) > 0) {
    messages.push(t('chat.queryCellTruncatedCountLabel', { count: props.data.cell_truncated_count }));
  }
  return messages;
});

const getColumnType = (column: string): string => columnTypeMap.value[column] || '';

const formatValue = (value: any): string => {
  if (value === null || value === undefined) {
    return t('chat.nullValuePlaceholder');
  }
  if (typeof value === 'object') {
    return JSON.stringify(value);
  }
  return String(value);
};
</script>

<style lang="less" scoped>
.database-query-display {
  display: flex;
  flex-direction: column;
  gap: 12px;
  font-size: 13px;
  color: var(--td-text-color-primary);
}

.results-summary {
  padding: 10px 12px;
  background: var(--td-brand-color-light);
  border-left: 3px solid var(--td-brand-color);
  border-radius: 4px;
  margin-bottom: 16px;
  font-size: 13px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  
  strong {
    color: var(--td-brand-color);
    font-weight: 600;
  }
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
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

.summary-label {
  color: var(--td-text-color-secondary);
  font-size: 12px;
}

.summary-status-list {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.status-pill {
  padding: 4px 8px;
  border-radius: 999px;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  font-size: 12px;
}

.sql-details {
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  background: var(--td-bg-color-container);
  padding: 10px 12px;

  summary {
    cursor: pointer;
    color: var(--td-text-color-secondary);
  }
}

.sql-code {
  margin: 10px 0 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: var(--app-font-family-mono);
  font-size: 12px;
}

.results-table-container {
  overflow-x: auto;
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  background: var(--td-bg-color-container);
}

.results-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
  
  thead {
    background: var(--td-bg-color-secondarycontainer);
    border-bottom: 2px solid var(--td-component-stroke);
    
    th {
      padding: 10px 12px;
      text-align: left;
      font-weight: 600;
      color: var(--td-text-color-primary);
      white-space: nowrap;

      .column-type {
        margin-top: 4px;
        color: var(--td-text-color-secondary);
        font-size: 11px;
        font-weight: 400;
      }
    }
  }
  
  tbody {
    tr {
      border-bottom: 1px solid var(--td-component-stroke);
      
      &:hover {
        background: var(--td-bg-color-secondarycontainer);
      }
      
      &:last-child {
        border-bottom: none;
      }
    }
    
    td {
      padding: 10px 12px;
      color: var(--td-text-color-primary);
      vertical-align: top;
      max-width: 400px;
      overflow: hidden;
      text-overflow: ellipsis;
    }
  }
}

.no-results {
  padding: 32px;
  text-align: center;
  color: var(--td-text-color-placeholder);
  font-style: italic;
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 6px;
  border: 1px solid var(--td-component-stroke);
}
</style>

