<template>
  <div v-if="visible" class="long-document-outline-panel">
    <div class="outline-panel-header">
      <div class="outline-panel-title">文档规划</div>
      <t-tag size="small" :theme="stageTheme" variant="light">{{ stageText }}</t-tag>
    </div>
    <div v-if="progressLabel" class="outline-panel-progress">{{ progressLabel }}</div>
    <div v-if="outline?.title" class="outline-panel-document-title">{{ outline.title }}</div>
    <div v-if="outline?.sections?.length" class="outline-panel-sections">
      <div
        v-for="(section, index) in outline.sections"
        :key="`${index}-${section}`"
        class="outline-panel-section"
        :class="{ 'is-current': isCurrentSection(section) }"
      >
        <span class="outline-panel-index">{{ index + 1 }}</span>
        <span class="outline-panel-section-text">{{ section }}</span>
        <t-tag v-if="isCurrentSection(section)" size="small" theme="primary" variant="light">当前</t-tag>
      </div>
    </div>
    <div v-else class="outline-panel-placeholder">正在生成规划大纲...</div>
  </div>
</template>

<script setup>
import { computed } from 'vue';

const props = defineProps({
  visible: {
    type: Boolean,
    default: false,
  },
  outline: {
    type: Object,
    default: null,
  },
  stage: {
    type: String,
    default: '',
  },
  progressLabel: {
    type: String,
    default: '',
  },
  sectionTitle: {
    type: String,
    default: '',
  },
});

const stageText = computed(() => {
  switch (props.stage) {
    case 'queued':
      return '排队中';
    case 'planning':
      return '规划中';
    case 'planned':
      return '已规划';
    case 'retrieving':
      return '检索中';
    case 'generating':
      return '生成中';
    case 'finalizing':
      return '收尾中';
    case 'document_edit':
      return '修订中';
    case 'completed':
      return '已完成';
    case 'needs_review':
      return '待复核';
    case 'blocked':
      return '已阻断';
    default:
      return '已规划';
  }
});

const stageTheme = computed(() => {
  switch (props.stage) {
    case 'queued':
      return 'default';
    case 'planning':
      return 'primary';
    case 'planned':
      return 'success';
    case 'retrieving':
      return 'warning';
    case 'generating':
      return 'success';
    case 'finalizing':
      return 'default';
    case 'document_edit':
      return 'primary';
    case 'completed':
      return 'success';
    case 'needs_review':
      return 'warning';
    case 'blocked':
      return 'danger';
    default:
      return 'default';
  }
});

const normalize = (value) => typeof value === 'string' ? value.trim() : '';

const isCurrentSection = (section) => {
  const current = normalize(props.sectionTitle);
  const target = normalize(section);
  if (!current || !target) {
    return false;
  }
  return current.includes(target) || target.includes(current);
};
</script>

<style lang="less" scoped>
.long-document-outline-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-top: 10px;
  padding: 14px;
  border-radius: 12px;
  border: 1px solid color-mix(in srgb, var(--td-brand-color) 16%, transparent);
  background: linear-gradient(180deg, color-mix(in srgb, var(--td-brand-color) 5%, var(--td-bg-color-container)) 0%, var(--td-bg-color-container) 100%);
}

.outline-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.outline-panel-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.outline-panel-progress {
  font-size: 12px;
  line-height: 1.6;
  color: var(--td-text-color-secondary);
}

.outline-panel-document-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  word-break: break-word;
}

.outline-panel-sections {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.outline-panel-section {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 10px;
  background: color-mix(in srgb, var(--td-bg-color-secondarycontainer) 72%, transparent);
  border: 1px solid transparent;

  &.is-current {
    border-color: color-mix(in srgb, var(--td-brand-color) 28%, transparent);
    background: color-mix(in srgb, var(--td-brand-color) 8%, var(--td-bg-color-secondarycontainer));
  }
}

.outline-panel-index {
  width: 22px;
  height: 22px;
  border-radius: 999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--td-brand-color);
  background: color-mix(in srgb, var(--td-brand-color) 10%, transparent);
}

.outline-panel-section-text {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  line-height: 1.6;
  color: var(--td-text-color-primary);
  word-break: break-word;
}

.outline-panel-placeholder {
  font-size: 13px;
  line-height: 1.6;
  color: var(--td-text-color-secondary);
}
</style>