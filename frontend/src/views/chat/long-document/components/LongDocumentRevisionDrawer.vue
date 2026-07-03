<template>
  <t-drawer :visible="visible" header="文档版本链" size="520px" placement="right" @update:visible="$emit('update:visible', $event)">
    <div class="artifact-drawer-body">
      <div v-if="loading" class="artifact-drawer-loading">
        <t-loading size="small" />
        <span>正在加载版本链...</span>
      </div>
      <div v-else-if="artifacts.length === 0" class="artifact-drawer-empty">
        暂无可展示的历史版本
      </div>
      <div v-else class="artifact-drawer-list">
        <div
          v-for="artifact in artifacts"
          :key="artifact.id"
          class="artifact-drawer-item"
          :class="{ 'is-selected': selectedArtifactId === artifact.id, 'is-current': anchorArtifact?.id === artifact.id }"
        >
          <div class="artifact-drawer-item-top">
            <div class="artifact-drawer-item-title">{{ artifact.title || '未命名文档' }}</div>
            <div class="artifact-drawer-item-tags">
              <t-tag size="small" theme="primary" variant="light">V{{ artifact.revision_no || 1 }}</t-tag>
              <t-tag size="small" :theme="getArtifactStatusTheme(artifact)" variant="light">{{ getArtifactStatusText(artifact) }}</t-tag>
            </div>
          </div>
          <div class="artifact-drawer-item-meta">{{ artifact.operation || 'create' }} · {{ artifact.updated_at || artifact.created_at || '-' }}</div>
          <div v-if="artifact.user_hint" class="artifact-drawer-item-hint">{{ artifact.user_hint }}</div>
          <div class="artifact-drawer-item-actions">
            <t-button size="small" variant="text" theme="primary" @click="$emit('view-artifact', artifact)">查看全文</t-button>
            <t-button size="small" variant="text" theme="primary" @click="$emit('use-as-base', artifact)">设为基线</t-button>
            <ExportDropdown
              v-if="typeof artifact.content_snapshot === 'string' && artifact.content_snapshot.trim().length > 0"
              :content="artifact.content_snapshot"
              :filename-prefix="artifact.title || `文档版本_V${artifact.revision_no || 1}`"
              :export-api-base="exportApiBase"
            />
          </div>
        </div>
      </div>
    </div>
  </t-drawer>
</template>

<script setup>
import ExportDropdown from '../../components/ExportDropdown.vue';

const props = defineProps({
  visible: {
    type: Boolean,
    default: false,
  },
  loading: {
    type: Boolean,
    default: false,
  },
  artifacts: {
    type: Array,
    default: () => [],
  },
  anchorArtifact: {
    type: Object,
    default: null,
  },
  selectedArtifactId: {
    type: String,
    default: '',
  },
  exportApiBase: {
    type: String,
    default: '',
  },
});

defineEmits(['update:visible', 'use-as-base', 'view-artifact']);

const trimText = (value) => typeof value === 'string' ? value.trim() : '';

const getArtifactStatusTheme = (artifact = {}) => {
  const generationStatus = trimText(artifact?.document_generation_status);
  if (generationStatus === 'needs_review') {
    return 'warning';
  }
  if (generationStatus === 'blocked') {
    return 'danger';
  }
  if (artifact?.status === 'available') {
    return 'success';
  }
  if (artifact?.status === 'partial') {
    return 'warning';
  }
  if (artifact?.status === 'failed') {
    return 'danger';
  }
  return 'default';
};

const getArtifactStatusText = (artifact = {}) => {
  const generationStatus = trimText(artifact?.document_generation_status);
  if (generationStatus === 'needs_review') {
    return '待复核';
  }
  if (generationStatus === 'blocked') {
    return '已阻断';
  }
  if (artifact?.can_manual_continue !== undefined && artifact.can_manual_continue !== false) {
    return '可继续';
  }
  if (artifact?.can_manual_revise !== undefined && artifact.can_manual_revise !== false) {
    return '可修订';
  }
  if (artifact?.can_view !== undefined && artifact.can_view !== false) {
    return '可查看';
  }
  if (artifact?.status === 'available') {
    return '已完成';
  }
  if (artifact?.status === 'partial') {
    return '部分完成';
  }
  if (artifact?.status === 'failed') {
    return '失败';
  }
  return '未知';
};
</script>

<style lang="less" scoped>
.artifact-drawer-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.artifact-drawer-loading,
.artifact-drawer-empty {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 160px;
  justify-content: center;
  color: var(--td-text-color-secondary);
}

.artifact-drawer-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.artifact-drawer-item {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px;
  border-radius: 12px;
  border: 1px solid color-mix(in srgb, var(--td-text-color-secondary) 12%, transparent);
  background: var(--td-bg-color-container);

  &.is-selected {
    border-color: color-mix(in srgb, var(--td-brand-color) 38%, transparent);
  }

  &.is-current {
    box-shadow: 0 10px 24px color-mix(in srgb, var(--td-brand-color) 10%, transparent);
  }
}

.artifact-drawer-item-top {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.artifact-drawer-item-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  word-break: break-word;
}

.artifact-drawer-item-tags {
  display: flex;
  gap: 6px;
  flex-shrink: 0;
}

.artifact-drawer-item-meta,
.artifact-drawer-item-hint {
  font-size: 12px;
  line-height: 1.6;
  color: var(--td-text-color-secondary);
}

.artifact-drawer-item-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
</style>