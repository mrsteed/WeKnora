<template>
  <Teleport to="body">
    <div v-if="visible" class="agent-selector-overlay" @click="$emit('close')">
      <div 
        class="agent-selector-dropdown"
        :style="dropdownStyle"
        @click.stop
      >
        <!-- 头部 -->
        <div class="agent-selector-header">
          <span>{{ $t('agent.selectAgent') }}</span>
          <router-link to="/platform/agents" class="agent-selector-add" @click="$emit('close')">
            <span class="add-icon">+</span>
            <span class="add-text">{{ $t('agent.manageAgents') }}</span>
          </router-link>
        </div>
        
        <!-- 内容区域 -->
        <div class="agent-selector-content">
          <!-- 内置智能体分组 -->
          <div class="agent-group">
            <div class="agent-group-title">{{ $t('agent.builtinAgents') }}</div>
            <t-tooltip 
              v-for="agent in builtinAgents" 
              :key="agent.id"
              :content="getAgentCapabilities(agent)"
              :disabled="currentAgentId === agent.id"
              placement="right"
            >
              <div 
                class="agent-option"
                :class="{ 'selected': currentAgentId === agent.id }"
                @click="selectAgent(agent)"
              >
                <div class="builtin-icon" :class="agent.type">
                  <TIcon :name="agent.type === 'agent' ? 'control-platform' : 'chat'" size="14px" />
                </div>
                <span class="agent-option-name">{{ agent.name }}</span>
                <svg 
                  v-if="currentAgentId === agent.id"
                  width="14" 
                  height="14" 
                  viewBox="0 0 16 16" 
                  fill="currentColor"
                  class="check-icon"
                >
                  <path d="M13.5 4.5L6 12L2.5 8.5L3.5 7.5L6 10L12.5 3.5L13.5 4.5Z"/>
                </svg>
              </div>
            </t-tooltip>
          </div>

          <!-- 自定义智能体分组 -->
          <div v-if="customAgents.length > 0" class="agent-group">
            <div class="agent-group-title">{{ $t('agent.customAgents') }}</div>
            <t-tooltip 
              v-for="agent in customAgents" 
              :key="agent.id"
              :content="getAgentCapabilities(agent)"
              :disabled="currentAgentId === agent.id"
              placement="right"
            >
              <div 
                class="agent-option"
                :class="{ 'selected': currentAgentId === agent.id }"
                @click="selectAgent(agent)"
              >
                <AgentAvatar :name="agent.name" size="small" />
                <span class="agent-option-name">{{ agent.name }}</span>
                <svg 
                  v-if="currentAgentId === agent.id"
                  width="14" 
                  height="14" 
                  viewBox="0 0 16 16" 
                  fill="currentColor"
                  class="check-icon"
                >
                  <path d="M13.5 4.5L6 12L2.5 8.5L3.5 7.5L6 10L12.5 3.5L13.5 4.5Z"/>
                </svg>
              </div>
            </t-tooltip>
          </div>

          <!-- 空状态 -->
          <div v-if="builtinAgents.length === 0 && customAgents.length === 0" class="agent-option empty">
            {{ $t('agent.noAgents') }}
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, nextTick } from 'vue';
import { useI18n } from 'vue-i18n';
import { Icon as TIcon } from 'tdesign-vue-next';
import { listAgents, type CustomAgent } from '@/api/agent';
import AgentAvatar from '@/components/AgentAvatar.vue';

const { t } = useI18n();

const props = defineProps<{
  visible: boolean;
  anchorEl?: HTMLElement;
  currentAgentId: string;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'select', agent: CustomAgent): void;
}>();

const agents = ref<CustomAgent[]>([]);
const dropdownStyle = ref<Record<string, string>>({});

// 内置智能体
const builtinAgents = computed(() => {
  return [
    {
      id: 'builtin-normal',
      name: t('input.normalMode'),
      description: t('input.normalModeDesc'),
      type: 'normal',
      is_builtin: true
    },
    {
      id: 'builtin-agent',
      name: t('input.agentMode'),
      description: t('input.agentModeDesc'),
      type: 'agent',
      is_builtin: true
    }
  ] as CustomAgent[];
});

// 自定义智能体
const customAgents = computed(() => {
  return agents.value.filter(a => !a.is_builtin);
});

// 获取智能体能力描述（用于 tooltip）
const getAgentCapabilities = (agent: CustomAgent): string => {
  // 内置智能体
  if (agent.is_builtin) {
    if (agent.type === 'normal') {
      return t('agent.capabilities.normal');
    } else if (agent.type === 'agent') {
      return t('agent.capabilities.agent');
    }
    return '';
  }
  
  // 自定义智能体
  const capabilities: string[] = [];
  const config = agent.config || {};
  
  if (config.model_id) {
    capabilities.push(t('agent.capabilities.modelSpecified'));
  }
  if (config.knowledge_bases && config.knowledge_bases.length > 0) {
    capabilities.push(t('agent.capabilities.kbCount', { count: config.knowledge_bases.length }));
  } else if (config.allow_user_kb_selection === false) {
    capabilities.push(t('agent.capabilities.kbDisabled'));
  }
  if (config.rerank_model_id) {
    capabilities.push(t('agent.capabilities.rerankSpecified'));
  }
  if (config.web_search_enabled === true) {
    capabilities.push(t('agent.capabilities.webSearchOn'));
  } else if (config.web_search_enabled === false) {
    capabilities.push(t('agent.capabilities.webSearchOff'));
  }
  if (config.system_prompt) {
    capabilities.push(t('agent.capabilities.hasPrompt'));
  }
  
  return capabilities.length > 0 ? capabilities.join('、') : t('agent.capabilities.default');
};

// 加载智能体列表
const loadAgents = async () => {
  try {
    const response = await listAgents();
    agents.value = response.data || [];
  } catch (error) {
    console.error('Failed to load agents:', error);
  }
};

// 选择智能体
const selectAgent = (agent: CustomAgent) => {
  emit('select', agent);
};

// 更新下拉框位置（与模型选择器一致）
const updateDropdownPosition = () => {
  if (!props.anchorEl) return;
  
  const rect = props.anchorEl.getBoundingClientRect();
  const dropdownWidth = 200;
  const offsetY = 8;
  const vh = window.innerHeight;
  const vw = window.innerWidth;
  
  // 水平位置：左对齐
  let left = Math.floor(rect.left);
  const minLeft = 16;
  const maxLeft = Math.max(16, vw - dropdownWidth - 16);
  left = Math.max(minLeft, Math.min(maxLeft, left));
  
  // 垂直位置
  const preferredDropdownHeight = 320;
  const minDropdownHeight = 100;
  const topMargin = 20;
  const spaceBelow = vh - rect.bottom;
  const spaceAbove = rect.top;
  
  let actualHeight: number;
  
  if (spaceBelow >= minDropdownHeight + offsetY) {
    // 向下弹出
    actualHeight = Math.min(preferredDropdownHeight, spaceBelow - offsetY - 16);
    const top = Math.floor(rect.bottom + offsetY);
    
    dropdownStyle.value = {
      position: 'fixed',
      width: `${dropdownWidth}px`,
      left: `${left}px`,
      top: `${top}px`,
      maxHeight: `${actualHeight}px`,
      zIndex: '9999'
    };
  } else {
    // 向上弹出
    const availableHeight = spaceAbove - offsetY - topMargin;
    actualHeight = availableHeight >= preferredDropdownHeight 
      ? preferredDropdownHeight 
      : Math.max(minDropdownHeight, availableHeight);
    
    const bottom = vh - rect.top + offsetY;
    
    dropdownStyle.value = {
      position: 'fixed',
      width: `${dropdownWidth}px`,
      left: `${left}px`,
      bottom: `${bottom}px`,
      maxHeight: `${actualHeight}px`,
      zIndex: '9999'
    };
  }
};

// 监听显示状态
watch(() => props.visible, (newVal) => {
  if (newVal) {
    loadAgents();
    nextTick(() => {
      updateDropdownPosition();
    });
  }
});

onMounted(() => {
  if (props.visible) {
    loadAgents();
  }
});
</script>

<style scoped lang="less">
.agent-selector-overlay {
  position: fixed;
  inset: 0;
  z-index: 9998;
  background: transparent;
  touch-action: none;
}

.agent-selector-dropdown {
  position: fixed;
  background: var(--td-bg-color-container, #fff);
  border-radius: 10px;
  box-shadow: var(--td-shadow-2, 0 6px 28px rgba(15, 23, 42, 0.08));
  border: 1px solid var(--td-component-border, #e7e9eb);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.agent-selector-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-bottom: 1px solid var(--td-component-stroke, #f0f0f0);
  background: var(--td-bg-color-container, #fff);
  font-size: 12px;
  font-weight: 500;
  color: var(--td-text-color-secondary, #666);
}

.agent-selector-add {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border-radius: 4px;
  border: 1px solid transparent;
  background: transparent;
  color: var(--td-brand-color, #07c05f);
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
  text-decoration: none;
  
  .add-icon {
    font-size: 14px;
    line-height: 1;
    font-weight: 400;
  }
  
  &:hover {
    color: var(--td-brand-color-hover, #05a04f);
    background: var(--td-bg-color-secondarycontainer, #f3f3f3);
  }
}

.agent-selector-content {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  -webkit-overflow-scrolling: touch;
  padding: 6px 8px;
}

.agent-group {
  &:not(:last-child) {
    margin-bottom: 8px;
    padding-bottom: 8px;
    border-bottom: 1px solid var(--td-component-stroke, #f0f0f0);
  }
}

.agent-group-title {
  font-size: 11px;
  color: var(--td-text-color-placeholder, #999);
  padding: 4px 8px 6px;
  font-weight: 500;
}

.agent-option {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  cursor: pointer;
  transition: background 0.12s;
  border-radius: 6px;
  margin-bottom: 4px;
  
  &:last-child {
    margin-bottom: 0;
  }
  
  &:hover {
    background: var(--td-bg-color-container-hover, #f6f8f7);
  }
  
  &.selected {
    background: var(--td-brand-color-light, #eefdf5);
    
    .agent-option-name {
      color: #10b981;
      font-weight: 600;
    }
  }
  
  &.empty {
    color: var(--td-text-color-disabled, #9aa0a6);
    cursor: default;
    text-align: center;
    padding: 20px 8px;
    
    &:hover {
      background: transparent;
    }
  }
}

.agent-option-name {
  font-size: 12px;
  color: var(--td-text-color-primary, #222);
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 1.4;
}

.builtin-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 6px;
  flex-shrink: 0;
  
  &.normal {
    background: rgba(7, 192, 95, 0.1);
    color: #059669;
  }
  
  &.agent {
    background: rgba(124, 77, 255, 0.1);
    color: #7c4dff;
  }
}

.check-icon {
  width: 14px;
  height: 14px;
  color: #10b981;
  flex-shrink: 0;
  margin-left: 6px;
}
</style>
