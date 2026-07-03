<template>
  <div v-if="visible" class="long-document-planning-timeline">
    <div
      v-for="item in items"
      :key="item.id"
      class="timeline-card"
      :class="{ 'is-done': item.done }"
    >
      <div class="timeline-card-header">
        <div class="timeline-card-title">{{ item.label }}</div>
        <t-tag size="small" :theme="item.done ? 'success' : 'primary'" variant="light">{{ item.done ? '已记录' : '进行中' }}</t-tag>
      </div>
      <div v-if="item.progressLabel" class="timeline-card-content">{{ item.progressLabel }}</div>
      <div v-else-if="item.content" class="timeline-card-content">{{ item.content }}</div>
      <div v-if="item.sectionTitle" class="timeline-card-meta">
        当前章节：{{ item.sectionTitle }}
        <span v-if="item.sectionTotal > 0">（{{ item.sectionCurrent }}/{{ item.sectionTotal }}）</span>
      </div>
    </div>
  </div>
</template>

<script setup>
defineProps({
  visible: {
    type: Boolean,
    default: false,
  },
  items: {
    type: Array,
    default: () => [],
  },
});
</script>

<style lang="less" scoped>
.long-document-planning-timeline {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-top: 10px;
}

.timeline-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px 14px;
  border-radius: 12px;
  border: 1px solid color-mix(in srgb, var(--td-brand-color) 12%, transparent);
  background: color-mix(in srgb, var(--td-brand-color) 3%, var(--td-bg-color-container));

  &.is-done {
    border-color: color-mix(in srgb, var(--td-success-color) 20%, transparent);
  }
}

.timeline-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.timeline-card-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.timeline-card-content,
.timeline-card-meta {
  font-size: 12px;
  line-height: 1.6;
  color: var(--td-text-color-secondary);
}
</style>