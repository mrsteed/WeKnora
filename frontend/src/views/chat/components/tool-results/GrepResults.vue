<template>
  <div class="grep-results">
    <!-- Results List -->
    <div v-if="results && results.length > 0" class="results-list">
      <div 
        v-for="(result, index) in results" 
        :key="result.id"
        class="result-item"
      >
        <t-popup 
          :overlayClassName="`grep-popup-${result.id}`"
          placement="bottom-left"
          width="400"
          :showArrow="false"
          trigger="click"
          destroy-on-close
        >
          <template #content>
            <ContentPopup 
              :content="result.content"
              :chunk-id="result.id"
              :knowledge-id="result.knowledge_id"
            />
          </template>
          <div class="result-header">
            <div class="result-title">
              <span class="result-index">#{{ index + 1 }}</span>
              <span class="knowledge-title">{{ result.knowledge_title || 'Untitled' }}</span>
            </div>
          </div>
        </t-popup>
      </div>
    </div>

    <!-- Empty State -->
    <div v-else class="empty-state">
      未找到匹配的内容
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { GrepResultsData } from '@/types/tool-results';
import ContentPopup from './ContentPopup.vue';

const props = defineProps<{
  data: GrepResultsData;
}>();

const maxDisplayPatterns = 2;

const displayPatterns = computed(() => {
  if (!props.data.patterns || props.data.patterns.length === 0) {
    return [];
  }
  return props.data.patterns.slice(0, maxDisplayPatterns);
});

const results = computed(() => props.data.results || []);
</script>

<style lang="less" scoped>
@import './tool-results.less';

.grep-results {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 0 0 0 12px;
}

.results-list {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.result-item {
  background: transparent;
  border: none;
  border-radius: 0;
  overflow: visible;
}

.result-header {
  padding: 2px 0;
  cursor: pointer;
  user-select: none;
  display: flex;
  align-items: center;
  gap: 6px;
  transition: color 0.15s ease;
  
  &:hover {
    color: #07c05f;
  }
}

.result-title {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  min-width: 0;
  font-size: 12px;
  line-height: 1.4;
}

.result-index {
  font-size: 11px;
  color: #9ca3af;
  font-weight: 600;
  flex-shrink: 0;
}

.pattern-badge {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  flex-shrink: 0;
  
  .pattern-text {
    font-family: 'Monaco', 'Menlo', 'Courier New', monospace;
    font-size: 10px;
    background: #f3f4f6;
    color: #6b7280;
    padding: 2px 5px;
    border-radius: 3px;
    white-space: nowrap;
    font-weight: 500;
  }
  
  .more-patterns {
    font-size: 10px;
    color: #9ca3af;
    padding: 2px 4px;
    background: #f3f4f6;
    border-radius: 3px;
  }
}

.knowledge-title {
  font-size: 12px;
  color: #374151;
  flex: 1;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

.chunk-info {
  font-size: 10px;
  color: #9ca3af;
  background: #f3f4f6;
  padding: 2px 5px;
  border-radius: 3px;
  flex-shrink: 0;
}

.empty-state {
  padding: 20px;
  text-align: center;
  color: #9ca3af;
  font-size: 12px;
  font-style: italic;
}

// Popup overlay styles
:deep([class*="grep-popup-"]) {
  .t-popup__content {
    max-height: 400px;
    max-width: 500px;
    overflow-y: auto;
    overflow-x: hidden;
    padding: 0;
    border-radius: 6px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.12);
    word-wrap: break-word;
    word-break: break-word;
  }
}
</style>
