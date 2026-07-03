<template>
  <div v-if="artifact" class="document-baseline-banner">
    <div class="document-baseline-text">
      <span class="document-baseline-label">{{ locked ? '当前基线' : '自动基线' }}</span>
      <span class="document-baseline-title">{{ artifact.title || '未命名文档' }}</span>
      <span class="document-baseline-version">V{{ artifact.revision_no || 1 }}</span>
      <span v-if="newerArtifact" class="document-baseline-latest-hint">
        已锁定手动基线，最新版本为 V{{ newerArtifact.revision_no || 1 }}
      </span>
    </div>
    <t-button v-if="locked" size="small" variant="text" theme="default" @click="$emit('clear')">取消</t-button>
  </div>
</template>

<script setup>
defineProps({
  artifact: {
    type: Object,
    default: null,
  },
  locked: {
    type: Boolean,
    default: false,
  },
  newerArtifact: {
    type: Object,
    default: null,
  },
});

defineEmits(['clear']);
</script>

<style lang="less" scoped>
.document-baseline-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
  padding: 10px 12px;
  border-radius: 10px;
  border: 1px solid color-mix(in srgb, var(--td-brand-color) 20%, transparent);
  background: color-mix(in srgb, var(--td-brand-color) 4%, var(--td-bg-color-container));
}

.document-baseline-text {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  min-width: 0;
}

.document-baseline-label,
.document-baseline-version,
.document-baseline-latest-hint {
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.document-baseline-title {
  min-width: 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  word-break: break-word;
}
</style>