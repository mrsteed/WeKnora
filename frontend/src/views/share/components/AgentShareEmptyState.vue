<template>
  <div class="agent-share-empty-state">
    <div class="agent-share-empty-card">
      <div class="agent-share-empty-icon">{{ icon }}</div>
      <h2 class="agent-share-empty-title">{{ title }}</h2>
      <p v-if="description" class="agent-share-empty-description">{{ description }}</p>
      <t-button v-if="actionText" theme="primary" @click="$emit('action')">{{ actionText }}</t-button>
    </div>
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  icon?: string;
  title: string;
  description?: string;
  actionText?: string;
}>(), {
  icon: '···',
  description: '',
  actionText: '',
});

defineEmits<{
  (e: 'action'): void;
}>();
</script>

<style scoped lang="less">
.agent-share-empty-state {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 420px;
  padding: 24px;
}

.agent-share-empty-card {
  width: min(520px, 100%);
  padding: 36px 28px;
  border-radius: 24px;
  background: rgba(255, 255, 255, 0.88);
  box-shadow: 0 20px 40px rgba(15, 23, 42, 0.08);
  text-align: center;
}

.agent-share-empty-icon {
  width: 64px;
  height: 64px;
  margin: 0 auto 16px;
  border-radius: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, rgba(7, 192, 95, 0.14), rgba(15, 23, 42, 0.12));
  color: #0f172a;
  font-size: 24px;
}

.agent-share-empty-title {
  margin: 0;
  font-size: 24px;
  color: #0f172a;
}

.agent-share-empty-description {
  margin: 12px 0 24px;
  color: #475569;
  line-height: 1.7;
}
</style>