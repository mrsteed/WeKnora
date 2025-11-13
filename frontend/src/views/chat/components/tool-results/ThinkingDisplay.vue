<template>
  <div class="thinking-display">
    <div class="thinking-content">
      <div class="thinking-icon" aria-hidden="true">💭</div>
      <div class="thinking-text">{{ data.thought }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { defineProps } from 'vue';
import type { ThinkingData } from '@/types/tool-results';
import { useI18n } from 'vue-i18n';

const props = defineProps<{
  data: ThinkingData;
}>();

useI18n(); // ensure component reacts to locale changes if needed
</script>

<style lang="less" scoped>
@import './tool-results.less';

.thinking-display {
  padding: 8px 0;
}

.thinking-content {
  display: flex;
  gap: 8px;
  padding: 10px 12px;
  background: #f3f3f3;
  border-radius: 6px;
  border-left: 3px solid #8b8b8b;
}

.thinking-icon {
  font-size: 16px;
  flex-shrink: 0;
}

.thinking-text {
  font-size: 13px;
  color: #333;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
  flex: 1;
}
</style>

