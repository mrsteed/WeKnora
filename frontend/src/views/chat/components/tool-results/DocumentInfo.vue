<template>
  <div class="document-info">

    <div v-if="documents.length" class="documents-list">
      <div
        v-for="(doc, index) in documents"
        :key="doc.knowledge_id || index"
        class="result-card document-card"
      >
        <div class="result-header document-header">
          <div class="result-title">
            <span class="doc-index">#{{ index + 1 }}</span>
            <span class="doc-title">{{ doc.title || $t('chat.notProvided') }}</span>
          </div>
          <div class="result-meta">
            <span class="meta-chip" v-if="doc.chunk_count">
              {{ $t('chat.chunkCountValue', { count: doc.chunk_count }) }}
            </span>
          </div>
        </div>
        <div class="result-content expanded">
          <div class="info-section">
            <div class="info-field">
              <span class="field-label">{{ $t('chat.documentIdLabel') }}</span>
              <span class="field-value"><code>{{ doc.knowledge_id }}</code></span>
            </div>
            <div class="info-field" v-if="doc.description">
              <span class="field-label">{{ $t('chat.documentDescriptionLabel') }}</span>
              <span class="field-value">{{ doc.description }}</span>
            </div>
            <div class="info-field" v-if="doc.source || doc.type">
              <span class="field-label">{{ $t('chat.documentSourceLabel') }}</span>
              <span class="field-value">{{ formatSource(doc) }}</span>
            </div>
            <div class="info-field" v-if="doc.file_name || doc.file_type || doc.file_size">
              <span class="field-label">{{ $t('chat.documentFileLabel') }}</span>
              <span class="field-value">
                <span v-if="doc.file_name">{{ doc.file_name }}</span>
                <template v-if="doc.file_type">&nbsp;({{ doc.file_type }})</template>
                <template v-if="doc.file_size">&nbsp;· {{ formatFileSize(doc.file_size) }}</template>
              </span>
            </div>
          </div>

          <div
            v-if="doc.metadata && Object.keys(doc.metadata).length"
            class="info-section metadata-section"
          >
            <div class="info-section-title">{{ $t('chat.documentMetadataLabel') }}</div>
            <ul class="metadata-list">
              <li
                v-for="(value, key) in doc.metadata"
                :key="`${doc.knowledge_id}-${key}`"
              >
                <span class="metadata-key">{{ key }}:</span>
                <span class="metadata-value">{{ formatMetadataValue(value) }}</span>
              </li>
            </ul>
          </div>
        </div>
      </div>
    </div>

    <div v-else class="empty-state">
      {{ $t('chat.documentInfoEmpty') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, defineProps } from 'vue';
import { useI18n } from 'vue-i18n';
import type { DocumentInfoData, DocumentInfoDocument } from '@/types/tool-results';

const props = defineProps<{
  data: DocumentInfoData;
}>();

const { t } = useI18n();

const documents = computed(() => props.data?.documents ?? []);
const errors = computed(() => props.data?.errors?.filter(Boolean) ?? []);
const totalChunkCount = computed(() =>
  documents.value.reduce((sum, doc) => sum + (doc.chunk_count || 0), 0),
);

const formatSource = (doc: DocumentInfoDocument) => {
  if (doc.type && doc.source) {
    return `${doc.type} · ${doc.source}`;
  }
  return doc.source || doc.type || t('chat.notProvided');
};

const formatFileSize = (size?: number) => {
  if (!size || size <= 0) {
    return t('chat.notProvided');
  }
  const units = ['B', 'KB', 'MB', 'GB'];
  let value = size;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  const fixed = value >= 10 || unitIndex === 0 ? 0 : 1;
  return `${value.toFixed(fixed)} ${units[unitIndex]}`;
};

const formatMetadataValue = (value: unknown) => {
  if (value === null || value === undefined) {
    return t('chat.notProvided');
  }
  if (typeof value === 'object') {
    try {
      return JSON.stringify(value);
    } catch {
      return String(value);
    }
  }
  return String(value);
};
</script>

<style lang="less" scoped>
@import './tool-results.less';

.document-info {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.document-summary {
  .summary-main {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
  }

  .summary-label {
    font-size: 13px;
    font-weight: 600;
    color: #333;
  }

  .summary-value {
    font-size: 13px;
    color: #555;
  }

  .summary-meta {
    margin-top: 8px;
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }

  .summary-errors {
    margin-top: 10px;
    padding-top: 10px;
    border-top: 1px solid @card-border;
  }
}

.meta-chip {
  font-size: 12px;
  color: #555;
  background: #ffffff;
  border: 1px solid @card-border;
  border-radius: 12px;
  padding: 2px 10px;
  line-height: 1.6;
  white-space: nowrap;
}

.documents-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.document-card {
  margin: 0 10px 10px 10px;
  .document-header {
    align-items: center;
  }

  .doc-index {
    font-weight: 600;
    color: #07c05f;
  }

  .doc-title {
    font-size: 14px;
    font-weight: 500;
    color: #222;
  }

  .status-pill {
    font-size: 12px;
    color: #07c05f;
    border: 1px solid rgba(7, 192, 95, 0.5);
    border-radius: 12px;
    padding: 2px 10px;
    line-height: 1.4;
  }
}

.info-section {
  margin-top: 0;
  padding: 8px 0;

  &:first-of-type {
    padding-top: 4px;
  }
}

.info-field {
  display: flex;
  gap: 12px;
  margin-bottom: 6px;
  font-size: 13px;

  .field-label {
    color: #8b8b8b;
    min-width: 100px;
    font-weight: 600;
  }

  .field-value {
    flex: 1;
    color: #333;
    line-height: 1.5;
  }
}

.metadata-section {
  padding-top: 12px;
  border-top: 1px dashed @card-border;
}

.metadata-list {
  list-style: none;
  margin: 6px 0 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;

  li {
    font-size: 12px;
    color: #333;
  }

  .metadata-key {
    font-weight: 600;
    margin-right: 4px;
  }

  .metadata-value {
    font-family: 'Monaco', 'Courier New', monospace;
  }
}

.empty-state {
  font-size: 13px;
  color: #666;
  text-align: center;
  padding: 16px;
  border: 1px dashed @card-border;
  border-radius: @card-radius;
  background: #fff;
}

code {
  font-family: 'Monaco', 'Courier New', monospace;
  font-size: 11px;
  background: #f0f0f0;
  padding: 2px 4px;
  border-radius: 3px;
}
</style>
