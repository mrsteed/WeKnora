<template>
  <div 
    ref="wrapperRef"
    class="faq-tag-wrapper"
    @mouseenter="handleMouseEnter"
    @mouseleave="handleMouseLeave"
  >
    <slot />
    <Teleport to="body">
      <Transition name="fade">
        <div
          v-if="showTooltip && content"
          ref="tooltipRef"
          class="faq-tag-tooltip"
          :class="tooltipClass"
          :style="tooltipStyle"
        >
          <div class="tooltip-content">{{ content }}</div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onUnmounted, watch } from 'vue'

const props = defineProps<{
  content: string
  placement?: 'top' | 'bottom' | 'left' | 'right'
  type?: 'answer' | 'similar' | 'negative'
}>()

const showTooltip = ref(false)
const tooltipRef = ref<HTMLElement | null>(null)
const wrapperRef = ref<HTMLElement | null>(null)
const tooltipStyle = ref<{ top: string; left: string }>({ top: '0px', left: '0px' })

const tooltipClass = computed(() => {
  return {
    [`tooltip-${props.type || 'answer'}`]: true,
    [`placement-${props.placement || 'top'}`]: true,
  }
})

const updatePosition = async () => {
  if (!wrapperRef.value || !tooltipRef.value) return
  
  await nextTick()
  
  // 再次检查，确保DOM已渲染
  if (!tooltipRef.value) return
  
  const rect = wrapperRef.value.getBoundingClientRect()
  const tooltipRect = tooltipRef.value.getBoundingClientRect()
  const placement = props.placement || 'top'
  
  let top = 0
  let left = 0
  
  switch (placement) {
    case 'top':
      top = rect.top - tooltipRect.height - 8
      left = rect.left + (rect.width / 2) - (tooltipRect.width / 2)
      break
    case 'bottom':
      top = rect.bottom + 8
      left = rect.left + (rect.width / 2) - (tooltipRect.width / 2)
      break
    case 'left':
      top = rect.top + (rect.height / 2) - (tooltipRect.height / 2)
      left = rect.left - tooltipRect.width - 8
      break
    case 'right':
      top = rect.top + (rect.height / 2) - (tooltipRect.height / 2)
      left = rect.right + 8
      break
  }
  
  // 边界检测
  const padding = 8
  if (left < padding) left = padding
  if (left + tooltipRect.width > window.innerWidth - padding) {
    left = window.innerWidth - tooltipRect.width - padding
  }
  if (top < padding) {
    // 如果上方空间不足，改为下方显示
    if (placement === 'top') {
      top = rect.bottom + 8
    } else {
      top = padding
    }
  }
  if (top + tooltipRect.height > window.innerHeight - padding) {
    top = window.innerHeight - tooltipRect.height - padding
  }
  
  tooltipStyle.value = {
    top: `${top}px`,
    left: `${left}px`,
  }
}

const handleMouseEnter = () => {
  showTooltip.value = true
  nextTick(() => {
    updatePosition()
  })
}

const handleMouseLeave = () => {
  showTooltip.value = false
}

onMounted(() => {
  window.addEventListener('scroll', updatePosition, true)
  window.addEventListener('resize', updatePosition)
})

onUnmounted(() => {
  window.removeEventListener('scroll', updatePosition, true)
  window.removeEventListener('resize', updatePosition)
})

watch(showTooltip, (newVal) => {
  if (newVal) {
    nextTick(() => {
      updatePosition()
    })
  }
})
</script>

<style scoped lang="less">
.faq-tag-wrapper {
  display: inline-block;
  position: relative;
  max-width: 100%;
  min-width: 0;
  overflow: visible;
  flex-shrink: 1;
  flex: 0 1 auto;
  
  // 确保内部的tag也能正确收缩
  :deep(.t-tag) {
    max-width: 100% !important;
    min-width: 0 !important;
    width: auto !important;
    display: inline-flex !important;
  }
  
  :deep(.t-tag span),
  :deep(.t-tag > span) {
    display: block !important;
    overflow: hidden !important;
    text-overflow: ellipsis !important;
    white-space: nowrap !important;
    max-width: 100% !important;
    min-width: 0 !important;
  }
}

.faq-tag-tooltip {
  position: fixed;
  z-index: 9999;
  max-width: 360px;
  min-width: 120px;
  padding: 12px 16px;
  background: #FFFFFF;
  color: #111827;
  border: 1px solid #E7E7E7;
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
  font-family: "PingFang SC";
  font-size: 13px;
  line-height: 1.6;
  word-break: break-word;
  pointer-events: none;

  &::before {
    content: '';
    position: absolute;
    width: 0;
    height: 0;
    border: 6px solid transparent;
  }

  &.placement-top::before {
    bottom: -12px;
    left: 50%;
    transform: translateX(-50%);
    border-top-color: #E7E7E7;
    filter: drop-shadow(0 2px 4px rgba(0, 0, 0, 0.08));
  }

  &.placement-top::after {
    content: '';
    position: absolute;
    bottom: -11px;
    left: 50%;
    transform: translateX(-50%);
    width: 0;
    height: 0;
    border: 6px solid transparent;
    border-top-color: #FFFFFF;
  }

  &.placement-bottom::before {
    top: -12px;
    left: 50%;
    transform: translateX(-50%);
    border-bottom-color: #E7E7E7;
    filter: drop-shadow(0 -2px 4px rgba(0, 0, 0, 0.08));
  }

  &.placement-bottom::after {
    content: '';
    position: absolute;
    top: -11px;
    left: 50%;
    transform: translateX(-50%);
    width: 0;
    height: 0;
    border: 6px solid transparent;
    border-bottom-color: #FFFFFF;
  }

  &.placement-left::before {
    right: -12px;
    top: 50%;
    transform: translateY(-50%);
    border-left-color: #E7E7E7;
    filter: drop-shadow(2px 0 4px rgba(0, 0, 0, 0.08));
  }

  &.placement-left::after {
    content: '';
    position: absolute;
    right: -11px;
    top: 50%;
    transform: translateY(-50%);
    width: 0;
    height: 0;
    border: 6px solid transparent;
    border-left-color: #FFFFFF;
  }

  &.placement-right::before {
    left: -12px;
    top: 50%;
    transform: translateY(-50%);
    border-right-color: #E7E7E7;
    filter: drop-shadow(-2px 0 4px rgba(0, 0, 0, 0.08));
  }

  &.placement-right::after {
    content: '';
    position: absolute;
    left: -11px;
    top: 50%;
    transform: translateY(-50%);
    width: 0;
    height: 0;
    border: 6px solid transparent;
    border-right-color: #FFFFFF;
  }

  &.tooltip-answer {
    border-color: #07C05F;
    background: #FFFFFF;
    box-shadow: 0 4px 16px rgba(7, 192, 95, 0.15);

    &::before {
      border-top-color: #07C05F;
    }

    &.placement-bottom::before {
      border-bottom-color: #07C05F;
    }

    &.placement-left::before {
      border-left-color: #07C05F;
    }

    &.placement-right::before {
      border-right-color: #07C05F;
    }

    &::after {
      border-top-color: #FFFFFF;
    }

    &.placement-bottom::after {
      border-bottom-color: #FFFFFF;
    }

    &.placement-left::after {
      border-left-color: #FFFFFF;
    }

    &.placement-right::after {
      border-right-color: #FFFFFF;
    }
  }

  &.tooltip-similar {
    border-color: #07C05F;
    background: #FFFFFF;
    box-shadow: 0 4px 16px rgba(7, 192, 95, 0.15);

    &::before {
      border-top-color: #07C05F;
    }

    &.placement-bottom::before {
      border-bottom-color: #07C05F;
    }

    &.placement-left::before {
      border-left-color: #07C05F;
    }

    &.placement-right::before {
      border-right-color: #07C05F;
    }

    &::after {
      border-top-color: #FFFFFF;
    }

    &.placement-bottom::after {
      border-bottom-color: #FFFFFF;
    }

    &.placement-left::after {
      border-left-color: #FFFFFF;
    }

    &.placement-right::after {
      border-right-color: #FFFFFF;
    }
  }

  &.tooltip-negative {
    border-color: #07C05F;
    background: #FFFFFF;
    box-shadow: 0 4px 16px rgba(7, 192, 95, 0.15);

    &::before {
      border-top-color: #07C05F;
    }

    &.placement-bottom::before {
      border-bottom-color: #07C05F;
    }

    &.placement-left::before {
      border-left-color: #07C05F;
    }

    &.placement-right::before {
      border-right-color: #07C05F;
    }

    &::after {
      border-top-color: #FFFFFF;
    }

    &.placement-bottom::after {
      border-bottom-color: #FFFFFF;
    }

    &.placement-left::after {
      border-left-color: #FFFFFF;
    }

    &.placement-right::after {
      border-right-color: #FFFFFF;
    }
  }
}

.tooltip-content {
  color: #111827;
  white-space: pre-wrap;
  word-break: break-word;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}

.fade-enter-from {
  opacity: 0;
}

.fade-leave-to {
  opacity: 0;
}
</style>

