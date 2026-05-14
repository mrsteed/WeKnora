<template>
  <div v-if="artifact" class="chat-document-artifact-card" :class="{ 'is-selected': isSelected }">
    <div class="artifact-card-header">
      <div class="artifact-card-title-wrap">
        <div class="artifact-card-caption">文档版本</div>
        <div class="artifact-card-title">{{ artifactTitle }}</div>
      </div>
      <div class="artifact-card-tags">
        <t-tag size="small" theme="primary" variant="light">V{{ artifact.revision_no || 1 }}</t-tag>
        <t-tag size="small" :theme="statusTheme" variant="light">{{ statusText }}</t-tag>
      </div>
    </div>
    <div class="artifact-card-meta">
      <span>{{ operationText }}</span>
      <span v-if="translationSummary">{{ translationSummary }}</span>
      <span v-if="artifact.parent_artifact_id">{{ relationText }}</span>
      <span v-if="structureSummary">{{ structureSummary }}</span>
    </div>
    <div v-if="artifact.user_hint" class="artifact-card-hint">{{ artifact.user_hint }}</div>
    <div class="artifact-card-actions">
      <ExportDropdown v-if="canExport" :content="previewContent" :filename-prefix="exportFilenamePrefix" />
      <t-button size="small" variant="text" theme="primary" @click="$emit('view-revisions', artifact)">查看版本链</t-button>
      <t-button
        v-if="canToggleDocumentDisplay"
        size="small"
        variant="text"
        theme="primary"
        @click="$emit('toggle-document-display', artifact)"
      >{{ documentDisplayToggleText }}</t-button>
      <t-button
        v-else-if="hasPreviewContent"
        size="small"
        variant="text"
        theme="primary"
        @click="togglePreview"
      >{{ previewExpanded ? '收起完整文档' : '查看完整文档' }}</t-button>
      <t-button
        v-if="!isSelected"
        size="small"
        variant="text"
        theme="primary"
        @click="$emit('use-as-base', artifact)"
      >设为基线</t-button>
      <t-button
        v-else
        size="small"
        variant="text"
        theme="default"
        @click="$emit('clear-base', artifact)"
      >取消基线</t-button>
    </div>
    <div v-if="previewExpanded && hasPreviewContent" class="artifact-card-preview">
      <div class="artifact-card-preview-title">完整文档预览</div>
      <div
        class="artifact-card-preview-content ai-markdown-template markdown-content"
        v-html="renderedPreviewHTML"
      ></div>
    </div>
  </div>
</template>

<script setup>
import { marked } from 'marked';
import { computed, ref } from 'vue';
import { safeMarkdownToHTML, sanitizeHTML } from '@/utils/security';
import ExportDropdown from './ExportDropdown.vue';

marked.use({
  breaks: true,
});

const props = defineProps({
  artifact: {
    type: Object,
    default: null,
  },
  selectedArtifactId: {
    type: String,
    default: '',
  },
  previewContent: {
    type: String,
    default: '',
  },
  canToggleDocumentDisplay: {
    type: Boolean,
    default: false,
  },
  documentDisplayMode: {
    type: String,
    default: 'delta',
  },
});

defineEmits(['view-revisions', 'use-as-base', 'clear-base', 'toggle-document-display']);

const previewExpanded = ref(false);

const artifactTitle = computed(() => props.artifact?.title || '未命名文档');

const isTranslationArtifact = computed(() => props.artifact?.document_task_kind === 'translation');

const isSelected = computed(() => Boolean(props.artifact?.id) && props.artifact?.id === props.selectedArtifactId);

const hasPreviewContent = computed(() => typeof props.previewContent === 'string' && props.previewContent.trim().length > 0);

const documentDisplayToggleText = computed(() => props.documentDisplayMode === 'full' ? '查看修改内容' : '查看全文');

const renderedPreviewHTML = computed(() => {
  if (!hasPreviewContent.value) {
    return '';
  }

  const safeMarkdown = safeMarkdownToHTML(props.previewContent);
  const html = marked.parse(safeMarkdown, { breaks: true });
  return sanitizeHTML(typeof html === 'string' ? html : '');
});

const canExport = computed(() => ['available', 'partial'].includes(props.artifact?.status) && hasPreviewContent.value);

const exportFilenamePrefix = computed(() => {
  if (isTranslationArtifact.value) {
    const sourceTitle = typeof props.artifact?.source_title === 'string' ? props.artifact.source_title.trim() : '';
    const targetLanguage = typeof props.artifact?.target_language === 'string' ? props.artifact.target_language.trim() : '';
    const revision = props.artifact?.revision_no || 1;
    if (sourceTitle && targetLanguage) {
      return `${sourceTitle}_${targetLanguage}_V${revision}`;
    }
  }
  const title = typeof props.artifact?.title === 'string' ? props.artifact.title.trim() : '';
  if (title) {
    return title;
  }
  const revision = props.artifact?.revision_no || 1;
  return `文档版本_V${revision}`;
});

const togglePreview = () => {
  previewExpanded.value = !previewExpanded.value;
};

const statusTheme = computed(() => {
  switch (props.artifact?.status) {
    case 'available':
      return 'success';
    case 'partial':
      return 'warning';
    case 'failed':
      return 'danger';
    default:
      return 'default';
  }
});

const statusText = computed(() => {
  switch (props.artifact?.status) {
    case 'available':
      return '可继续';
    case 'partial':
      return '部分完成';
    case 'failed':
      return '失败';
    default:
      return '未知';
  }
});

const operationText = computed(() => {
  if (isTranslationArtifact.value) {
    return '全文翻译版本';
  }
  switch (props.artifact?.operation) {
    case 'continue':
      return '基于上一版继续生成';
    case 'revise':
      return '基于上一版修改';
    case 'regenerate':
      return '重新生成的新版本';
    default:
      return '初始版本';
  }
});

const relationText = computed(() => {
  if (!props.artifact?.parent_artifact_id) {
    return '';
  }
  return '已挂入当前会话版本链';
});

const structureSummary = computed(() => {
  const info = props.artifact?.structure_info;
  if (!info) {
    return '';
  }
  const parts = [];
  if (info.heading_count) {
    parts.push(`${info.heading_count} 个标题`);
  }
  if (info.has_list) {
    parts.push('含列表');
  }
  if (info.has_table) {
    parts.push('含表格');
  }
  return parts.join(' · ');
});

const translationSummary = computed(() => {
  if (!isTranslationArtifact.value) {
    return '';
  }
  const parts = [];
  const sourceTitle = typeof props.artifact?.source_title === 'string' ? props.artifact.source_title.trim() : '';
  const targetLanguage = typeof props.artifact?.target_language === 'string' ? props.artifact.target_language.trim() : '';
  const outputFormat = typeof props.artifact?.output_format === 'string' ? props.artifact.output_format.trim() : '';
  if (sourceTitle) {
    parts.push(`源文件：${sourceTitle}`);
  }
  if (targetLanguage) {
    parts.push(`目标语言：${targetLanguage}`);
  }
  if (outputFormat) {
    parts.push(`格式：${outputFormat}`);
  }
  return parts.join(' · ');
});
</script>

<style lang="less" scoped>
.chat-document-artifact-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px 14px;
  border-radius: 10px;
  border: 1px solid color-mix(in srgb, var(--td-brand-color) 18%, transparent);
  background: linear-gradient(180deg, color-mix(in srgb, var(--td-brand-color) 6%, var(--td-bg-color-container)) 0%, var(--td-bg-color-container) 100%);
  margin-top: 10px;

  &.is-selected {
    border-color: color-mix(in srgb, var(--td-brand-color) 48%, transparent);
    box-shadow: 0 8px 24px color-mix(in srgb, var(--td-brand-color) 10%, transparent);
  }
}

.artifact-card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.artifact-card-title-wrap {
  min-width: 0;
}

.artifact-card-caption {
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.artifact-card-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  word-break: break-word;
}

.artifact-card-tags {
  display: flex;
  gap: 6px;
  flex-shrink: 0;
}

.artifact-card-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.artifact-card-actions {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

.artifact-card-hint {
  font-size: 12px;
  line-height: 1.5;
  color: var(--td-warning-color-7);
  background: color-mix(in srgb, var(--td-warning-color) 8%, transparent);
  border-radius: 8px;
  padding: 8px 10px;
}

.artifact-card-preview {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 8px;
  background: color-mix(in srgb, var(--td-bg-color-secondarycontainer) 72%, transparent);
  border: 1px solid color-mix(in srgb, var(--td-text-color-secondary) 14%, transparent);
}

.artifact-card-preview-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--td-text-color-secondary);
}

.ai-markdown-template {
  font-size: 12px;
  color: var(--td-text-color-primary);
  line-height: 1.6;
}

.artifact-card-preview-content {
  margin: 0;
  word-break: break-word;
  font-size: 12px;
  line-height: 1.6;
  color: var(--td-text-color-primary);
  max-height: 360px;
  overflow: auto;
}

.markdown-content {
  :deep(p) {
    margin: 6px 0;
    line-height: 1.6;
  }

  :deep(code) {
    background: var(--td-bg-color-secondarycontainer);
    padding: 2px 5px;
    border-radius: 3px;
    font-family: var(--app-font-family-mono);
    font-size: 11px;
  }

  :deep(pre) {
    background: var(--td-bg-color-secondarycontainer);
    padding: 10px;
    border-radius: 4px;
    overflow-x: auto;
    margin: 6px 0;

    code {
      background: none;
      padding: 0;
    }
  }

  :deep(ul), :deep(ol) {
    margin: 6px 0;
    padding-left: 20px;
  }

  :deep(li) {
    margin: 3px 0;
  }

  :deep(blockquote) {
    border-left: 2px solid var(--td-brand-color);
    padding-left: 10px;
    margin: 6px 0;
    color: var(--td-text-color-secondary);
  }

  :deep(h1), :deep(h2), :deep(h3), :deep(h4), :deep(h5), :deep(h6) {
    margin: 10px 0 6px 0;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  :deep(a) {
    color: var(--td-brand-color);
    text-decoration: none;

    &:hover {
      text-decoration: underline;
    }
  }

  :deep(table) {
    border-collapse: collapse;
    margin: 6px 0;
    font-size: 11px;
    width: 100%;

    th, td {
      border: 1px solid var(--td-component-stroke);
      padding: 5px 8px;
      text-align: left;
    }

    th {
      background: var(--td-bg-color-secondarycontainer);
      font-weight: 600;
    }

    tbody tr:nth-child(even) {
      background: var(--td-bg-color-secondarycontainer);
    }
  }
}
</style>