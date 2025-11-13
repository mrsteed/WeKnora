<template>
  <div class="plan-display">
    <div v-if="data.steps && data.steps.length > 0" class="plan-steps">
      <div v-for="(step, index) in data.steps" :key="step.id || index" class="step-item" :class="`status-${step.status}`">
        <div class="step-checkbox" :class="{ 'checked': step.status === 'completed', 'in-progress': step.status === 'in_progress' }">
          <svg v-if="step.status === 'completed'" width="16" height="16" viewBox="0 0 16 16" fill="none">
            <rect x="2" y="2" width="12" height="12" rx="2" fill="#07C05F"/>
            <path d="M5 8L7 10L11 6" stroke="#fff" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
          <svg v-else width="16" height="16" viewBox="0 0 16 16" fill="none">
            <rect x="2" y="2" width="12" height="12" rx="2" stroke="#d1d5db" stroke-width="1.5" fill="none"/>
          </svg>
        </div>
        <span class="step-description" :class="{ 'completed': step.status === 'completed' }">
          {{ step.description }}
          <span v-if="step.status === 'in_progress'" class="sparkle">✨</span>
        </span>
      </div>
    </div>
    
    <div v-else class="no-steps">
      {{ $t('chat.noPlanSteps') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import type { PlanData } from '@/types/tool-results';

interface Props {
  data: PlanData;
}

const props = defineProps<Props>();
</script>

<style lang="less" scoped>
.plan-display {
  font-size: 13px;
  color: #666;
  background: transparent;
  padding: 8px 0 8px 12px;
  margin: 0;
  border: none !important;
  box-shadow: none !important;
  outline: none;
}

.plan-steps {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.step-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 2px 0;
  transition: all 0.2s;
  
  &:last-child {
    margin-bottom: 0;
  }
  
  &.status-in_progress {
    .step-description {
      color: #333;
      font-weight: 500;
    }
  }
}

.step-checkbox {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
  margin-top: 1px;
  display: flex;
  align-items: center;
  justify-content: center;
  
  &.checked {
    svg {
      rect {
        fill: #07C05F;
      }
    }
  }
  
  &.in-progress {
    svg {
      rect {
        stroke: #07C05F;
        stroke-width: 2;
      }
    }
  }
}

.step-description {
  flex: 1;
  color: #666;
  line-height: 1.5;
  font-size: 13px;
  
  &.completed {
    text-decoration: line-through;
    color: #999;
  }
  
  .sparkle {
    margin-left: 4px;
    font-size: 12px;
  }
}

.no-steps {
  padding: 8px 12px;
  text-align: center;
  color: #9ca3af;
  font-style: italic;
  font-size: 13px;
}
</style>

