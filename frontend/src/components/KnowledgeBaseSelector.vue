<template>
  <div v-if="visible" class="kb-overlay" @click="close">
    <div class="kb-dropdown" @click.stop @wheel.stop :style="dropdownStyle">
      <!-- 搜索 -->
      <div class="kb-search">
        <input
          ref="searchInput"
          v-model="searchQuery"
          type="text"
          :placeholder="$t('knowledgeBase.searchPlaceholder')"
          class="kb-search-input"
          @keydown.down.prevent="moveSelection(1)"
          @keydown.up.prevent="moveSelection(-1)"
          @keydown.enter.prevent="toggleSelection"
          @keydown.esc="close"
        />
      </div>

      <!-- 列表 -->
      <div class="kb-list" ref="kbList" @wheel.stop>
        <div
          v-for="(kb, index) in filteredKnowledgeBases"
          :key="kb.id"
          :class="['kb-item', { selected: isSelected(kb.id), highlighted: highlightedIndex === index }]"
          @click="toggleKb(kb.id)"
          @mouseenter="highlightedIndex = index"
        >
          <div class="kb-item-left">
            <div class="checkbox" :class="{ checked: isSelected(kb.id) }">
              <svg v-if="isSelected(kb.id)" width="12" height="12" viewBox="0 0 12 12" fill="none">
                <path d="M10 3L4.5 8.5L2 6" stroke="#fff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
              </svg>
            </div>
            <div class="kb-name-wrap">
              <span class="kb-name">{{ kb.name }}</span>
              <span class="kb-docs" v-if="kb.docsCount !== undefined">{{ kb.docsCount }} {{ $t('knowledgeBase.documents') }}</span>
            </div>
          </div>
        </div>

        <div v-if="filteredKnowledgeBases.length === 0" class="kb-empty">
          {{ searchQuery ? $t('knowledgeBase.noMatch') : $t('knowledgeBase.noKnowledge') }}
        </div>
      </div>

      <!-- 底部操作 -->
      <div class="kb-actions">
        <button @click="selectAll" class="kb-btn">{{ $t('common.selectAll') }}</button>
        <button @click="clearAll" class="kb-btn">{{ $t('common.clear') }}</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { useSettingsStore } from '@/stores/settings'
import { listKnowledgeBases } from '@/api/knowledge-base'
import { useI18n } from 'vue-i18n'

interface KnowledgeBase {
  id: string
  name: string
  docsCount?: number
  embedding_model_id?: string
  summary_model_id?: string
}

const { t } = useI18n()

const props = defineProps<{
  visible: boolean
  anchorEl?: any | null // 支持 DOM 节点、ref、组件实例
  dropdownWidth?: number
  offsetY?: number
}>()

const emit = defineEmits(['close', 'update:visible'])

const settingsStore = useSettingsStore()

// 本地状态
const searchQuery = ref('')
const highlightedIndex = ref(0)
const knowledgeBases = ref<KnowledgeBase[]>([])
const searchInput = ref<HTMLInputElement | null>(null)
const kbList = ref<HTMLElement | null>(null)
const dropdownStyle = ref<Record<string, string>>({})

// props 默认
const dropdownWidth = props.dropdownWidth ?? 300
const offsetY = props.offsetY ?? 8

// 过滤：只显示已初始化（有 embedding & summary）的
const filteredKnowledgeBases = computed(() => {
  const valid = knowledgeBases.value.filter(
    k => k.embedding_model_id && k.summary_model_id
  )
  if (!searchQuery.value) return valid
  const q = searchQuery.value.toLowerCase()
  return valid.filter(k => k.name.toLowerCase().includes(q))
})

const selectedKbIds = computed(() => settingsStore.settings.selectedKnowledgeBases || [])

// helper: 从 props.anchorEl 获取真实 DOM 元素（支持多种传入形式）
const resolveAnchorEl = () => {
  const a = props.anchorEl
  if (!a) return null
  // 如果是 Vue ref：取 .value
  if (typeof a === 'object' && 'value' in a) {
    return a.value ?? null
  }
  // 如果是组件实例（可能有 $el）
  if (typeof a === 'object' && '$el' in a) {
    // @ts-ignore
    return a.$el ?? null
  }
  // 直接 DOM 节点或 DOMRect
  return a
}

const isSelected = (id: string) => selectedKbIds.value.includes(id)

const toggleKb = (id: string) => {
  isSelected(id) ? settingsStore.removeKnowledgeBase(id) : settingsStore.addKnowledgeBase(id)
}

const toggleSelection = () => {
  const kb = filteredKnowledgeBases.value[highlightedIndex.value]
  if (kb) toggleKb(kb.id)
}

const moveSelection = (dir: number) => {
  const max = filteredKnowledgeBases.value.length
  if (max === 0) return
  highlightedIndex.value = Math.max(0, Math.min(max - 1, highlightedIndex.value + dir))
  nextTick(() => {
    const items = kbList.value?.querySelectorAll('.kb-item')
    items?.[highlightedIndex.value]?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  })
}

const selectAll = () => settingsStore.selectKnowledgeBases(filteredKnowledgeBases.value.map(k => k.id))
const clearAll = () => settingsStore.clearKnowledgeBases()

const close = () => {
  emit('update:visible', false)
  emit('close')
}

const loadKnowledgeBases = async () => {
  try {
    const res: any = await listKnowledgeBases()
    if (res?.data && Array.isArray(res.data)) knowledgeBases.value = res.data
  } catch (e) {
    console.error(t('knowledgeBase.loadingFailed'), e)
  }
}

// 计算下拉位置：水平居中对齐到按钮中点，处理视口边界
const updateDropdownPosition = () => {
  const anchor = resolveAnchorEl()
  
  // fallback 函数
  const applyFallback = () => {
    const vw = window.innerWidth
    const topFallback = Math.max(80, window.innerHeight / 2 - 160)
    dropdownStyle.value = {
      position: 'fixed',
      width: `${dropdownWidth}px`,
      left: `${Math.round((vw - dropdownWidth) / 2)}px`,
      top: `${Math.round(topFallback)}px`
    }
  }
  
  if (!anchor) {
    applyFallback()
    return
  }

  // 获取 anchor 的 bounding rect（相对于视口）
  let rect: DOMRect | null = null
  try {
    if (typeof anchor.getBoundingClientRect === 'function') {
      // 强制重新计算布局，确保获取最新位置
      rect = anchor.getBoundingClientRect()
    } else if (anchor.width !== undefined && anchor.left !== undefined) {
      // 已经是 DOMRect
      rect = anchor as DOMRect
    }
  } catch (e) {
    console.error('[KnowledgeBaseSelector] Error getting bounding rect:', e)
  }
  
  if (!rect || rect.width === 0 || rect.height === 0) {
    applyFallback()
    return
  }

  const vw = window.innerWidth
  const vh = window.innerHeight
  
  // 左对齐到触发元素的左边缘
  // 使用 Math.floor 而不是 Math.round，避免像素对齐问题
  let left = Math.floor(rect.left)
  
  // 边界处理：不超出视口左右（留 16px margin）
  const minLeft = 16
  const maxLeft = Math.max(16, vw - dropdownWidth - 16)
  left = Math.max(minLeft, Math.min(maxLeft, left))

  // 垂直定位：智能判断向上还是向下
  const dropdownHeight = 360 // 弹窗高度估算
  const spaceBelow = vh - rect.bottom // 下方剩余空间
  const spaceAbove = rect.top // 上方剩余空间
  
  let top: number
  
  // 判断应该向上还是向下弹出
  if (spaceBelow >= dropdownHeight + offsetY + 16) {
    // 下方空间充足，向下弹出
    top = Math.floor(rect.bottom + offsetY)
  } else if (spaceAbove >= dropdownHeight + offsetY + 16) {
    // 下方不够但上方充足，向上弹出
    top = Math.floor(rect.top - dropdownHeight - offsetY)
  } else {
    // 上下都不够，选择空间较大的一侧
    if (spaceAbove > spaceBelow) {
      // 上方空间更大，贴着顶部
      top = Math.max(16, Math.floor(rect.top - dropdownHeight - offsetY))
    } else {
      // 下方空间更大，贴着底部或按钮下方
      top = Math.floor(rect.bottom + offsetY)
      // 确保不超出视口底部
      if (top + dropdownHeight > vh - 16) {
        top = Math.max(16, Math.floor(vh - dropdownHeight - 16))
      }
    }
  }

  // 使用 fixed 定位，确保没有 transform 或其他样式影响
  dropdownStyle.value = {
    position: 'fixed',
    width: `${dropdownWidth}px`,
    left: `${left}px`,
    top: `${top}px`,
    maxHeight: `${Math.min(dropdownHeight, vh - 32)}px`, // 添加最大高度限制
    transform: 'none', // 确保没有 transform 影响定位
    margin: '0', // 确保没有 margin 影响定位
    padding: '0' // 确保没有 padding 影响定位
  }
}

// 事件监听器引用，用于清理
let resizeHandler: (() => void) | null = null
let scrollHandler: (() => void) | null = null

// 当 visible 变化时处理
watch(() => props.visible, async (v) => {
  if (v) {
    await loadKnowledgeBases()
    // 等 DOM 渲染完再计算位置，防止 rect 读取到不准值
    await nextTick()
    // 延迟一帧再计算位置，确保所有样式都已应用
    requestAnimationFrame(() => {
      updateDropdownPosition()
      // 再次确保位置正确（应对动画或其他延迟渲染）
      requestAnimationFrame(() => {
        updateDropdownPosition()
        // 最后一次确保位置正确
        setTimeout(() => {
          updateDropdownPosition()
        }, 100)
      })
    })
    // 确保 focus
    nextTick(() => searchInput.value?.focus())
    // 监听 resize/scroll 做微调（使用 passive 提高性能）
    resizeHandler = () => updateDropdownPosition()
    scrollHandler = () => updateDropdownPosition()
    window.addEventListener('resize', resizeHandler, { passive: true })
    window.addEventListener('scroll', scrollHandler, { passive: true, capture: true })
  } else {
    searchQuery.value = ''
    highlightedIndex.value = 0
    // 清理事件监听器
    if (resizeHandler) {
      window.removeEventListener('resize', resizeHandler)
      resizeHandler = null
    }
    if (scrollHandler) {
      window.removeEventListener('scroll', scrollHandler, { capture: true })
      scrollHandler = null
    }
  }
})
</script>

<style scoped lang="less">
// 确保所有元素使用 border-box 盒模型
.kb-overlay,
.kb-overlay *,
.kb-overlay *::before,
.kb-overlay *::after {
  box-sizing: border-box;
}

.kb-overlay {
  position: fixed;
  inset: 0;
  z-index: 9999;
  background: transparent;
  /* 不阻止点击穿透，但防止触摸滚动 */
  touch-action: none;
}

/* 下拉面板使用 fixed 定位，相对于视口 */
.kb-dropdown {
  position: fixed !important;
  background: #fff;
  border: 1px solid #e7e9eb;
  border-radius: 10px;
  box-shadow: 0 6px 28px rgba(15, 23, 42, 0.08);
  overflow: hidden;
  animation: fadeIn 0.15s ease-out;
  z-index: 10000;
  margin: 0;
  /* 确保定位准确，动画使用 scale 而不是 translate */
  transform-origin: top left;
  display: flex;
  flex-direction: column;
}

/* 宽度由 JS 控制（dropdownWidth），这里只做内部样式 */
.kb-search {
  padding: 10px 12px;
  border-bottom: 1px solid #f1f3f4;
}
.kb-search-input {
  width: 100%;
  padding: 8px 12px;
  font-size: 13px;
  border: 1px solid #eef1f2;
  border-radius: 8px;
  background: #fbfdfc;
  outline: none;
  transition: border 0.12s;
}
.kb-search-input:focus {
  border-color: #10b981;
  background: #fff;
}

.kb-list {
  flex: 1;
  min-height: 0; /* 允许 flex 子元素缩小 */
  max-height: 260px;
  overflow-y: auto;
  padding: 8px;
  /* 确保滚动限制在此容器内 */
  overscroll-behavior: contain;
  -webkit-overflow-scrolling: touch;
}

.kb-item {
  display: flex;
  align-items: center;
  padding: 8px;
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.12s;
  margin-bottom: 6px;
}
.kb-item:last-child { margin-bottom: 0; }

.kb-item:hover,
.kb-item.highlighted { background: #f6f8f7; }

.kb-item.selected { background: #eefdf5; }

.kb-item-left {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
}

.checkbox {
  width: 18px; height: 18px;
  border-radius: 4px;
  border: 1.5px solid #d7dadd;
  display: flex;
  align-items: center;
  justify-content: center;
}
.checkbox.checked {
  background: #10b981;
  border-color: #10b981;
}
.kb-name-wrap { display:flex; flex-direction: column; min-width: 0; }
.kb-name { font-size: 13px; color: #222; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.kb-docs { font-size: 11px; color: #8b9196; margin-top: 2px; }

.kb-empty { padding: 28px 8px; text-align: center; color: #9aa0a6; font-size: 13px; }

.kb-actions {
  display: flex;
  gap: 10px;
  padding: 10px;
  border-top: 1px solid #f2f4f5;
  background: #fafcfc;
}
.kb-btn {
  flex: 1;
  padding: 8px 10px;
  border-radius: 8px;
  border: 1px solid #e1e5e6;
  background: #fff;
  font-size: 13px;
  color: #52575a;
  cursor: pointer;
  transition: all 0.12s;
}
.kb-btn:hover {
  border-color: #10b981;
  color: #10b981;
  background: #f0fdf6;
}

@keyframes fadeIn {
  from { opacity: 0; transform: scale(0.98); }
  to { opacity: 1; transform: scale(1); }
}
</style>
