<script setup lang="ts">
import { ref, defineEmits, onMounted, onUnmounted, defineProps, computed, watch, nextTick, h } from "vue";
import { useRoute, useRouter } from 'vue-router';
import { onBeforeRouteUpdate } from 'vue-router';
import { MessagePlugin } from "tdesign-vue-next";
import { useSettingsStore } from '@/stores/settings';
import { useUIStore } from '@/stores/ui';
import { listKnowledgeBases, getKnowledgeBaseById } from '@/api/knowledge-base';
import { stopSession } from '@/api/chat';
import KnowledgeBaseSelector from './KnowledgeBaseSelector.vue';
import { getModel, type ModelConfig } from '@/api/model';
import { getTenantWebSearchConfig } from '@/api/web-search';
import { useI18n } from 'vue-i18n';

const route = useRoute();
const router = useRouter();
const settingsStore = useSettingsStore();
const uiStore = useUIStore();
let query = ref("");
const showKbSelector = ref(false);
const atButtonRef = ref<HTMLElement>();
const showAgentModeSelector = ref(false);
const agentModeButtonRef = ref<HTMLElement>();
const agentModeDropdownStyle = ref<Record<string, string>>({});

const props = defineProps({
  isReplying: {
    type: Boolean,
    required: false
  },
  sessionId: {
    type: String,
    required: false
  },
  assistantMessageId: {
    type: String,
    required: false
  }
});

const isAgentEnabled = computed(() => settingsStore.isAgentEnabled);
const isWebSearchEnabled = computed(() => settingsStore.isWebSearchEnabled);
const selectedKbIds = computed(() => settingsStore.settings.selectedKnowledgeBases || []);
const isWebSearchConfigured = ref(false);

// 获取已选择的知识库信息
const knowledgeBases = ref<Array<{ id: string; name: string }>>([]);
const selectedKbs = computed(() => {
  return knowledgeBases.value.filter(kb => selectedKbIds.value.includes(kb.id));
});

// 模型相关状态
const availableModels = ref<ModelConfig[]>([]);
const selectedModelId = ref<string>('');
const thinkingModelName = ref<string>('');
const showModelSelector = ref(false);
const modelButtonRef = ref<HTMLElement>();
const modelDropdownStyle = ref<Record<string, string>>({});

const { t } = useI18n();

// 显示的知识库标签（最多显示2个）
const displayedKbs = computed(() => selectedKbs.value.slice(0, 2));
const remainingCount = computed(() => Math.max(0, selectedKbs.value.length - 2));

// 加载知识库列表
const loadKnowledgeBases = async () => {
  try {
    const response: any = await listKnowledgeBases();
    if (response.data && Array.isArray(response.data)) {
      const validKbs = response.data.filter((kb: any) => 
        kb.embedding_model_id && kb.embedding_model_id !== '' &&
        kb.summary_model_id && kb.summary_model_id !== ''
      );
      knowledgeBases.value = validKbs;
      
      // 清理无效的知识库ID（已删除或不存在于有效知识库列表中的）
      const validKbIds = new Set(validKbs.map((kb: any) => kb.id));
      const currentSelectedIds = settingsStore.settings.selectedKnowledgeBases || [];
      const validSelectedIds = currentSelectedIds.filter((id: string) => validKbIds.has(id));
      
      // 如果有无效的ID，更新store
      if (validSelectedIds.length !== currentSelectedIds.length) {
        settingsStore.selectKnowledgeBases(validSelectedIds);
      }
    }
  } catch (error) {
    console.error('Failed to load knowledge bases:', error);
  }
};

const loadWebSearchConfig = async () => {
  try {
    const response: any = await getTenantWebSearchConfig();
    const config = response?.data;
    const configured = !!(config && config.provider);
    isWebSearchConfigured.value = configured;

    if (!configured && settingsStore.isWebSearchEnabled) {
      settingsStore.toggleWebSearch(false);
    }
  } catch (error) {
    console.error('Failed to load web search config:', error);
    isWebSearchConfigured.value = false;
    if (settingsStore.isWebSearchEnabled) {
      settingsStore.toggleWebSearch(false);
    }
  }
};

// 解析模型参数大小 (e.g., "7B", "13B", "70B" -> 7, 13, 70)
const parseParameterSize = (paramSize: string | undefined): number => {
  if (!paramSize) return 0;
  const match = paramSize.match(/(\d+\.?\d*)/);
  return match ? parseFloat(match[1]) : 0;
};

// 加载并选择模型
const loadModelsFromKnowledgeBases = async () => {
  if (selectedKbIds.value.length === 0) {
    availableModels.value = [];
    selectedModelId.value = '';
    return;
  }

  try {
    // 获取所有选中知识库的 summary_model_id
    const modelIds = new Set<string>();
    for (const kbId of selectedKbIds.value) {
      try {
        const kb: any = await getKnowledgeBaseById(kbId);
        if (kb.data?.summary_model_id) {
          modelIds.add(kb.data.summary_model_id);
        }
      } catch (error) {
        console.error(`Failed to fetch KB ${kbId}:`, error);
      }
    }

    // 获取所有模型的详细信息
    const models: ModelConfig[] = [];
    for (const modelId of Array.from(modelIds)) {
      try {
        const model = await getModel(modelId);
        models.push(model);
      } catch (error) {
        console.error(`Failed to fetch model ${modelId}:`, error);
      }
    }

    availableModels.value = models;

    // 应用优先级选择算法
    if (models.length > 0) {
      selectModelByPriority(models);
    } else {
      selectedModelId.value = '';
    }
  } catch (error) {
    console.error('Failed to load models from knowledge bases:', error);
  }
};

// 根据优先级选择模型
const selectModelByPriority = (models: ModelConfig[]) => {
  // 1. 优先选择远程模型，按创建时间降序
  const remoteModels = models
    .filter(m => m.source === 'remote')
    .sort((a, b) => {
      const timeA = new Date(a.created_at || 0).getTime();
      const timeB = new Date(b.created_at || 0).getTime();
      return timeB - timeA;
    });

  if (remoteModels.length > 0) {
    selectedModelId.value = remoteModels[0].id || '';
    return;
  }

  // 2. 否则选择 Ollama 模型，按参数大小降序
  const ollamaModels = models
    .filter(m => m.source === 'local')
    .sort((a, b) => {
      const sizeA = parseParameterSize(a.parameters?.parameter_size);
      const sizeB = parseParameterSize(b.parameters?.parameter_size);
      return sizeB - sizeA;
    });

  if (ollamaModels.length > 0) {
    selectedModelId.value = ollamaModels[0].id || '';
    return;
  }

  // 3. 如果都没有，选择第一个
  selectedModelId.value = models[0]?.id || '';
};

// 获取 Thinking 模型名称（Agent 模式）
const loadThinkingModelName = async () => {
  const thinkingModelId = settingsStore.agentConfig.thinkingModelId;
  if (thinkingModelId) {
    try {
      const model = await getModel(thinkingModelId);
      thinkingModelName.value = model.name || thinkingModelId;
    } catch (error) {
      console.error('Failed to fetch thinking model:', error);
      thinkingModelName.value = thinkingModelId;
    }
  } else {
    thinkingModelName.value = '';
  }
};

// 计算当前选中的模型
const selectedModel = computed(() => {
  return availableModels.value.find(m => m.id === selectedModelId.value);
});

// 关闭模型选择器（点击外部）
const closeModelSelector = () => {
  showModelSelector.value = false;
};

// 关闭 Agent 模式选择器（点击外部）
const closeAgentModeSelector = () => {
  showAgentModeSelector.value = false;
};

onMounted(() => {
  loadKnowledgeBases();
  loadWebSearchConfig();
  
  // 如果从知识库内部进入，自动选中该知识库
  const kbId = (route.params as any)?.kbId as string;
  if (kbId && !selectedKbIds.value.includes(kbId)) {
    settingsStore.addKnowledgeBase(kbId);
  }

  // 加载模型
  if (isAgentEnabled.value) {
    loadThinkingModelName();
  } else {
    loadModelsFromKnowledgeBases();
  }

  // 监听点击外部关闭下拉菜单
  document.addEventListener('click', closeModelSelector);
  document.addEventListener('click', closeAgentModeSelector);
  // 监听窗口大小变化，重新计算位置
  window.addEventListener('resize', () => {
    if (showModelSelector.value) {
      updateModelDropdownPosition();
    }
    if (showAgentModeSelector.value) {
      updateAgentModeDropdownPosition();
    }
  });
});

onUnmounted(() => {
  document.removeEventListener('click', closeModelSelector);
  document.removeEventListener('click', closeAgentModeSelector);
});

// 监听路由变化
watch(() => route.params.kbId, (newKbId) => {
  if (newKbId && typeof newKbId === 'string' && !selectedKbIds.value.includes(newKbId)) {
    settingsStore.addKnowledgeBase(newKbId);
  }
});

watch(() => uiStore.showSettingsModal, (visible, prevVisible) => {
  if (prevVisible && !visible) {
    loadWebSearchConfig();
  }
});

// 监听知识库选择变化，重新加载模型
watch(selectedKbIds, () => {
  if (!isAgentEnabled.value) {
    loadModelsFromKnowledgeBases();
  }
}, { deep: true });

// 监听 Agent 模式变化
watch(isAgentEnabled, (newVal) => {
  if (newVal) {
    loadThinkingModelName();
  } else {
    loadModelsFromKnowledgeBases();
  }
});

// 监听 Thinking 模型变化
watch(() => settingsStore.agentConfig.thinkingModelId, () => {
  if (isAgentEnabled.value) {
    loadThinkingModelName();
  }
});

const emit = defineEmits(['send-msg', 'stop-generation']);

const createSession = (val: string) => {
  if (!val.trim()) {
    MessagePlugin.info(t('input.messages.enterContent'));
    return;
  }
  if (selectedKbIds.value.length === 0) {
    MessagePlugin.warning(t('input.messages.selectKnowledge'));
    return;
  }
  if (props.isReplying) {
    return MessagePlugin.error(t('input.messages.replying'));
  }
  emit('send-msg', val, selectedModelId.value);
  clearvalue();
}

// 计算模型下拉菜单位置
const updateModelDropdownPosition = () => {
  const anchor = modelButtonRef.value;
  
  if (!anchor) {
    modelDropdownStyle.value = {
      position: 'fixed',
      top: '50%',
      left: '50%',
      transform: 'translate(-50%, -50%)'
    };
    return;
  }

  const rect = anchor.getBoundingClientRect();
  const dropdownWidth = 250;
  const offsetY = 8;
  
  // 计算位置：默认在按钮下方
  let top = rect.bottom + offsetY;
  let left = rect.left;
  
  // 检查右侧边界
  if (left + dropdownWidth > window.innerWidth) {
    left = window.innerWidth - dropdownWidth - 10;
  }
  
  // 检查下方空间
  const dropdownMaxHeight = 400;
  if (top + dropdownMaxHeight > window.innerHeight) {
    // 如果下方空间不足，显示在按钮上方
    top = rect.top - dropdownMaxHeight - offsetY;
    if (top < 10) {
      // 如果上方也不够，显示在屏幕中间
      top = 10;
    }
  }
  
  modelDropdownStyle.value = {
    position: 'fixed',
    top: `${Math.round(top)}px`,
    left: `${Math.round(left)}px`,
    width: `${dropdownWidth}px`
  };
};

// 计算 Agent 模式下拉菜单位置
const updateAgentModeDropdownPosition = () => {
  const anchor = agentModeButtonRef.value;
  
  if (!anchor) {
    agentModeDropdownStyle.value = {
      position: 'fixed',
      top: '50%',
      left: '50%',
      transform: 'translate(-50%, -50%)'
    };
    return;
  }

  const rect = anchor.getBoundingClientRect();
  const dropdownWidth = 200;
  const offsetY = 8;
  
  // 计算位置：默认在按钮下方
  let top = rect.bottom + offsetY;
  let left = rect.left;
  
  // 检查右侧边界
  if (left + dropdownWidth > window.innerWidth) {
    left = window.innerWidth - dropdownWidth - 10;
  }
  
  // 检查下方空间
  const dropdownMaxHeight = 150;
  if (top + dropdownMaxHeight > window.innerHeight) {
    // 如果下方空间不足，显示在按钮上方
    top = rect.top - dropdownMaxHeight - offsetY;
    if (top < 10) {
      top = 10;
    }
  }
  
  agentModeDropdownStyle.value = {
    position: 'fixed',
    top: `${Math.round(top)}px`,
    left: `${Math.round(left)}px`,
    width: `${dropdownWidth}px`
  };
};

const toggleModelSelector = () => {
  showModelSelector.value = !showModelSelector.value;
  if (showModelSelector.value) {
    nextTick(() => {
      updateModelDropdownPosition();
    });
  }
}

const toggleAgentModeSelector = () => {
  // 如果 Agent 未就绪，显示提示
  if (!settingsStore.isAgentReady && !isAgentEnabled.value) {
    toggleAgentMode();
    return;
  }
  
  showAgentModeSelector.value = !showAgentModeSelector.value;
  if (showAgentModeSelector.value) {
    nextTick(() => {
      updateAgentModeDropdownPosition();
    });
  }
}

const selectAgentMode = (mode: 'normal' | 'agent') => {
  if (mode === 'agent' && !settingsStore.isAgentReady) {
    toggleAgentMode();
    showAgentModeSelector.value = false;
    return;
  }
  
  const shouldEnableAgent = mode === 'agent';
  if (shouldEnableAgent !== isAgentEnabled.value) {
    settingsStore.toggleAgent(shouldEnableAgent);
    MessagePlugin.success(shouldEnableAgent ? t('input.messages.agentSwitchedOn') : t('input.messages.agentSwitchedOff'));
  }
  showAgentModeSelector.value = false;
}

const clearvalue = () => {
  query.value = "";
}

const onKeydown = (val: string, event: { e: { preventDefault(): unknown; keyCode: number; shiftKey: any; ctrlKey: any; }; }) => {
  if ((event.e.keyCode == 13 && event.e.shiftKey) || (event.e.keyCode == 13 && event.e.ctrlKey)) {
    return;
  }
  if (event.e.keyCode == 13) {
    event.e.preventDefault();
    createSession(val)
  }
}

const handleGoToWebSearchSettings = () => {
  uiStore.openSettings('websearch');
  if (route.path !== '/platform/settings') {
    router.push('/platform/settings');
  }
};

const handleGoToAgentSettings = () => {
  // 使用 uiStore 打开设置并跳转到 agent 部分
  uiStore.openSettings('agent');
  // 如果当前不在设置页面，导航到设置页面
  if (route.path !== '/platform/settings') {
    router.push('/platform/settings');
  }
}

const toggleAgentMode = () => {
  // 如果要启用 Agent，先检查是否就绪
  // 注意：isAgentReady 是从 store 中计算的，需要确保 store 中的配置是最新的
  if (!isAgentEnabled.value) {
    // 尝试启用 Agent，先检查是否就绪
    const agentReady = settingsStore.isAgentReady
    if (!agentReady) {
      // 创建带跳转链接的自定义消息
      const messageContent = h('div', { style: 'display: flex; align-items: center; gap: 8px; flex-wrap: wrap;' }, [
        h('span', { style: 'flex: 1; min-width: 0;' }, t('input.messages.agentNotReadyDetail')),
        h('a', {
          href: '#',
          onClick: (e: Event) => {
            e.preventDefault();
            handleGoToAgentSettings();
          },
          style: 'color: #07C05F; text-decoration: none; font-weight: 500; cursor: pointer; white-space: nowrap; flex-shrink: 0;',
          onMouseenter: (e: Event) => {
            (e.target as HTMLElement).style.textDecoration = 'underline';
          },
          onMouseleave: (e: Event) => {
            (e.target as HTMLElement).style.textDecoration = 'none';
          }
        }, t('input.goToSettings'))
      ]);
      
      MessagePlugin.warning({
        content: () => messageContent,
        duration: 5000
      });
      return
    }
  }
  
  // 正常切换 Agent 状态
  settingsStore.toggleAgent(!isAgentEnabled.value);
  const message = isAgentEnabled.value ? t('input.messages.agentEnabled') : t('input.messages.agentDisabled');
  MessagePlugin.success(message);
}

const toggleWebSearch = () => {
  if (!isWebSearchConfigured.value) {
    const messageContent = h('div', { style: 'display: flex; flex-direction: column; gap: 6px; max-width: 280px;' }, [
      h('span', { style: 'color: #333; line-height: 1.5;' }, t('input.messages.webSearchNotConfigured')),
      h('a', {
        href: '#',
        onClick: (e: Event) => {
          e.preventDefault();
          handleGoToWebSearchSettings();
        },
        style: 'color: #07C05F; text-decoration: none; font-weight: 500; cursor: pointer; align-self: flex-start;',
        onMouseenter: (e: Event) => {
          (e.target as HTMLElement).style.textDecoration = 'underline';
        },
        onMouseleave: (e: Event) => {
          (e.target as HTMLElement).style.textDecoration = 'none';
        }
      }, t('input.goToSettings'))
    ]);
    MessagePlugin.warning({
      content: () => messageContent,
      duration: 5000
    });
    return;
  }

  const currentValue = settingsStore.isWebSearchEnabled;
  const newValue = !currentValue;
  settingsStore.toggleWebSearch(newValue);
  MessagePlugin.success(newValue ? t('input.messages.webSearchEnabled') : t('input.messages.webSearchDisabled'));
};

const toggleKbSelector = () => {
  showKbSelector.value = !showKbSelector.value;
}

const removeKb = (kbId: string) => {
  settingsStore.removeKnowledgeBase(kbId);
}

const handleStop = async () => {
  if (!props.sessionId) {
    MessagePlugin.warning(t('input.messages.sessionMissing'));
    return;
  }
  
  if (!props.assistantMessageId) {
    console.error('[Stop] Assistant message ID is empty');
    MessagePlugin.warning(t('input.messages.messageMissing'));
    return;
  }
  
  console.log('[Stop] Stopping generation for message:', props.assistantMessageId);
  
  // 发送 stop 事件，通知父组件立即清除 loading 状态
  emit('stop-generation');
  
  try {
    await stopSession(props.sessionId, props.assistantMessageId);
    MessagePlugin.success(t('input.messages.stopSuccess'));
  } catch (error) {
    console.error('Failed to stop session:', error);
    MessagePlugin.error(t('input.messages.stopFailed'));
  }
}

onBeforeRouteUpdate((to, from, next) => {
  clearvalue()
  next()
})

</script>
<template>
  <div class="answers-input">
    <t-textarea v-model="query" :placeholder="$t('input.placeholder')" name="description" :autosize="true" @keydown="onKeydown" />
    
    <!-- 控制栏 -->
    <div class="control-bar">
      <!-- 左侧控制按钮 -->
      <div class="control-left">
        <!-- Agent 模式切换按钮 -->
        <div 
          ref="agentModeButtonRef"
          class="control-btn agent-mode-btn"
          :class="{ 
            'active': isAgentEnabled,
            'agent-active': isAgentEnabled
          }"
          @click.stop="toggleAgentModeSelector"
        >
          <img 
            v-if="isAgentEnabled"
            :src="getImgSrc('agent-active.svg')" 
            :alt="$t('input.agentMode')" 
            class="control-icon agent-icon"
          />
          <svg 
            v-else
            width="18" 
            height="18" 
            viewBox="0 0 16 16" 
            fill="currentColor"
            class="control-icon normal-mode-icon"
          >
            <path d="M8 0a8 8 0 1 0 0 16A8 8 0 0 0 8 0zM1.5 8a6.5 6.5 0 1 1 13 0 6.5 6.5 0 0 1-13 0z"/>
            <path d="M8 4.5a3.5 3.5 0 1 0 0 7 3.5 3.5 0 0 0 0-7zM6 8a2 2 0 1 1 4 0 2 2 0 0 1-4 0z"/>
          </svg>
          <span class="agent-mode-text">
            {{ isAgentEnabled ? $t('input.agentMode') : $t('input.normalMode') }}
          </span>
          <svg 
            width="12" 
            height="12" 
            viewBox="0 0 12 12" 
            fill="currentColor"
            class="dropdown-arrow"
            :class="{ 'rotate': showAgentModeSelector }"
          >
            <path d="M2.5 4.5L6 8L9.5 4.5H2.5Z"/>
          </svg>
        </div>

        <!-- Agent 模式选择下拉菜单 -->
        <Teleport to="body">
          <div v-if="showAgentModeSelector" class="agent-mode-selector-overlay" @click="closeAgentModeSelector">
            <div 
              class="agent-mode-selector-dropdown"
              :style="agentModeDropdownStyle"
              @click.stop
            >
              <div 
                class="agent-mode-option"
                :class="{ 'selected': !isAgentEnabled }"
                @click="selectAgentMode('normal')"
              >
                <div class="agent-mode-option-main">
                  <span class="agent-mode-option-name">{{ $t('input.normalMode') }}</span>
                  <span class="agent-mode-option-desc">{{ $t('input.normalModeDesc') }}</span>
                </div>
                <svg 
                  v-if="!isAgentEnabled"
                  width="16" 
                  height="16" 
                  viewBox="0 0 16 16" 
                  fill="currentColor"
                  class="check-icon"
                >
                  <path d="M13.5 4.5L6 12L2.5 8.5L3.5 7.5L6 10L12.5 3.5L13.5 4.5Z"/>
                </svg>
              </div>
              <div 
                class="agent-mode-option"
                :class="{ 
                  'selected': isAgentEnabled,
                  'disabled': !settingsStore.isAgentReady && !isAgentEnabled 
                }"
                @click="selectAgentMode('agent')"
              >
                <div class="agent-mode-option-main">
                  <span class="agent-mode-option-name">{{ $t('input.agentMode') }}</span>
                  <span class="agent-mode-option-desc">{{ $t('input.agentModeDesc') }}</span>
                </div>
                <svg 
                  v-if="isAgentEnabled"
                  width="16" 
                  height="16" 
                  viewBox="0 0 16 16" 
                  fill="currentColor"
                  class="check-icon"
                >
                  <path d="M13.5 4.5L6 12L2.5 8.5L3.5 7.5L6 10L12.5 3.5L13.5 4.5Z"/>
                </svg>
                <div v-if="!settingsStore.isAgentReady && !isAgentEnabled" class="agent-mode-warning">
                  <t-tooltip :content="$t('input.agentNotReadyTooltip')" placement="left">
                    <t-icon name="error-circle" class="warning-icon" />
                  </t-tooltip>
                </div>
              </div>
              <div v-if="!settingsStore.isAgentReady && !isAgentEnabled" class="agent-mode-footer">
                <a 
                  href="#"
                  @click.prevent="handleGoToAgentSettings"
                  class="agent-mode-link"
                >
                  {{ $t('input.goToSettings') }}
                </a>
              </div>
            </div>
          </div>
        </Teleport>

        <!-- WebSearch 开关按钮 -->
        <t-tooltip placement="top">
          <template #content>
            <span v-if="isWebSearchConfigured">{{ isWebSearchEnabled ? $t('input.webSearch.toggleOff') : $t('input.webSearch.toggleOn') }}</span>
            <div v-else class="websearch-tooltip-disabled">
              <span>{{ $t('input.webSearch.notConfigured') }}</span>
              <a href="#" @click.prevent="handleGoToWebSearchSettings">{{ $t('input.goToSettings') }}</a>
            </div>
          </template>
          <div 
            class="control-btn websearch-btn"
            :class="{ 'active': isWebSearchEnabled && isWebSearchConfigured, 'disabled': !isWebSearchConfigured }"
            @click.stop="toggleWebSearch"
          >
            <svg 
              width="18" 
              height="18" 
              viewBox="0 0 18 18" 
              fill="none"
              xmlns="http://www.w3.org/2000/svg"
              class="control-icon websearch-icon"
              :class="{ 'active': isWebSearchEnabled && isWebSearchConfigured }"
            >
              <circle cx="9" cy="9" r="7" stroke="currentColor" stroke-width="1.2" fill="none"/>
              <path d="M 9 2 A 3.5 7 0 0 0 9 16" stroke="currentColor" stroke-width="1.2" fill="none"/>
              <path d="M 9 2 A 3.5 7 0 0 1 9 16" stroke="currentColor" stroke-width="1.2" fill="none"/>
              <line x1="2.94" y1="5.5" x2="15.06" y2="5.5" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>
              <line x1="2.94" y1="12.5" x2="15.06" y2="12.5" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>
            </svg>
          </div>
        </t-tooltip>

        <!-- @ 知识库选择按钮 -->
        <div 
          ref="atButtonRef"
          class="control-btn kb-btn"
          :class="{ 'active': selectedKbIds.length > 0 }"
          @click="toggleKbSelector"
        >
          <img :src="getImgSrc('at-icon.svg')" alt="@" class="control-icon" />
          <span class="kb-btn-text">
            {{ selectedKbIds.length > 0 ? $t('input.knowledgeBaseWithCount', { count: selectedKbIds.length }) : $t('input.knowledgeBase') }}
          </span>
          <svg 
            width="12" 
            height="12" 
            viewBox="0 0 12 12" 
            fill="currentColor"
            class="dropdown-arrow"
            :class="{ 'rotate': showKbSelector }"
          >
            <path d="M2.5 4.5L6 8L9.5 4.5H2.5Z"/>
          </svg>
        </div>

        <!-- 已选择的知识库标签 -->
        <div v-if="displayedKbs.length > 0" class="kb-tags">
          <div 
            v-for="kb in displayedKbs" 
            :key="kb.id" 
            class="kb-tag"
          >
            <span class="kb-tag-text">{{ kb.name }}</span>
            <span class="kb-tag-remove" @click.stop="removeKb(kb.id)">×</span>
          </div>
          <div v-if="remainingCount > 0" class="kb-tag more-tag">
            +{{ remainingCount }}
          </div>
        </div>

        <!-- 模型显示 -->
        <div v-if="selectedKbIds.length > 0" class="model-display">
          <!-- Agent 模式：只读显示 Thinking 模型 -->
          <div v-if="isAgentEnabled" class="model-badge">
            <span class="model-label">{{ $t('input.thinkingLabel') }}</span>
            <span class="model-name">{{ thinkingModelName || $t('input.notConfigured') }}</span>
          </div>

          <!-- 非 Agent 模式：可选择的模型下拉 -->
          <div 
            v-else
            ref="modelButtonRef" 
            class="control-btn model-btn"
            :class="{ 'active': selectedModel }"
            @click.stop="toggleModelSelector"
          >
            <span class="model-btn-text">
              {{ selectedModel ? selectedModel.name : $t('input.model') }}
            </span>
            <svg 
              width="12" 
              height="12" 
              viewBox="0 0 12 12" 
              fill="currentColor"
              class="dropdown-arrow"
              :class="{ 'rotate': showModelSelector }"
            >
              <path d="M2.5 4.5L6 8L9.5 4.5H2.5Z"/>
            </svg>
          </div>

          <!-- 模型选择下拉菜单 -->
          <Teleport to="body">
            <div v-if="showModelSelector && !isAgentEnabled" class="model-selector-overlay">
              <div 
                class="model-selector-dropdown"
                :style="modelDropdownStyle"
                @click.stop
              >
                <div class="model-selector-content">
                  <div 
                    v-for="model in availableModels" 
                    :key="model.id"
                    class="model-option"
                    :class="{ 'selected': model.id === selectedModelId }"
                    @click="selectedModelId = model.id || ''; showModelSelector = false"
                  >
                    <div class="model-option-main">
                      <span class="model-option-name">{{ model.name }}</span>
                      <span v-if="model.source === 'remote'" class="model-badge-remote">{{ $t('input.remote') }}</span>
                      <span v-else-if="model.parameters?.parameter_size" class="model-badge-local">
                        {{ model.parameters.parameter_size }}
                      </span>
                    </div>
                    <div v-if="model.description" class="model-option-desc">
                      {{ model.description }}
                    </div>
                  </div>
                  <div v-if="availableModels.length === 0" class="model-option empty">
                    {{ $t('input.noModel') }}
                  </div>
                </div>
              </div>
            </div>
          </Teleport>
        </div>
      </div>

      <!-- 右侧控制按钮组 -->
      <div class="control-right">
        <!-- 停止按钮（仅在回复中时显示） -->
        <t-tooltip 
          v-if="isReplying"
          :content="$t('input.stopGeneration')"
          placement="top"
        >
          <div 
            @click="handleStop" 
            class="control-btn stop-btn"
          >
            <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
              <rect x="5" y="5" width="6" height="6" rx="1" />
            </svg>
          </div>
        </t-tooltip>

        <!-- 发送按钮 -->
      <div 
          v-if="!isReplying"
        @click="createSession(query)" 
        class="control-btn send-btn"
        :class="{ 'disabled': !query.length || selectedKbIds.length === 0 }"
      >
        <img src="../assets/img/sending-aircraft.svg" :alt="$t('input.send')" />
        </div>
      </div>
    </div>

    <!-- 知识库选择下拉（使用 Teleport 传送到 body，避免父容器定位影响） -->
    <Teleport to="body">
    <KnowledgeBaseSelector
      v-model:visible="showKbSelector"
        :anchorEl="atButtonRef"
      @close="showKbSelector = false"
    />
    </Teleport>
  </div>
</template>
<script lang="ts">
const getImgSrc = (url: string) => {
  return new URL(`/src/assets/img/${url}`, import.meta.url).href;
}
</script>
<style scoped lang="less">
.answers-input {
  position: absolute;
  z-index: 99;
  bottom: 60px;
  left: 50%;
  transform: translateX(-400px);
}

:deep(.t-textarea__inner) {
  width: 100%;
  width: 800px;
  max-height: 250px !important;
  min-height: 112px !important;
  resize: none;
  color: #000000e6;
  font-size: 16px;
  font-weight: 400;
  line-height: 24px;
  font-family: "PingFang SC";
  padding: 16px 12px 52px 16px;  /* 增加底部padding为控制栏腾出空间 */
  border-radius: 12px;
  border: 1px solid #E7E7E7;
  box-sizing: border-box;
  background: #FFF;
  box-shadow: 0 6px 6px 0 #0000000a, 0 12px 12px -1px #00000014;

  &:focus {
    border: 1px solid #07C05F;
  }

  &::placeholder {
    color: #00000066;
    font-family: "PingFang SC";
    font-size: 16px;
    font-weight: 400;
    line-height: 24px;
  }
}

/* 控制栏 */
.control-bar {
  position: absolute;
  bottom: 12px;
  left: 16px;
  right: 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.control-left {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  overflow: hidden;
}

.control-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 6px 10px;
  border-radius: 6px;
  background: #f5f5f5;
  cursor: pointer;
  transition: all 0.2s ease;
  user-select: none;
  flex-shrink: 0;

  &:hover {
    background: #e6e6e6;
    transform: scale(1.02);
  }

  &:active {
    transform: scale(0.98);
  }

  &.disabled {
    opacity: 0.5;
    cursor: not-allowed;
    
    &:hover {
      background: #f5f5f5;
      transform: none;
    }
  }
}

.agent-mode-btn {
  height: 28px;
  padding: 0 10px;
  min-width: auto;
  font-weight: 500;
  border: 1.5px solid transparent;
  transition: all 0.2s ease;
  position: relative;
  
  &.active,
  &.agent-active {
    background: linear-gradient(135deg, rgba(7, 192, 95, 0.15) 0%, rgba(7, 192, 95, 0.1) 100%);
    border-color: rgba(7, 192, 95, 0.4);
    box-shadow: 0 2px 6px rgba(7, 192, 95, 0.12);
    
    .agent-mode-text {
      color: #07C05F;
      font-weight: 600;
    }
    
    .agent-icon {
      filter: brightness(0) saturate(100%) invert(58%) sepia(87%) saturate(1234%) hue-rotate(95deg) brightness(98%) contrast(89%);
    }
    
    .dropdown-arrow {
      color: #07C05F;
    }
    
    &:hover {
      background: linear-gradient(135deg, rgba(7, 192, 95, 0.2) 0%, rgba(7, 192, 95, 0.15) 100%);
      border-color: rgba(7, 192, 95, 0.6);
      box-shadow: 0 3px 10px rgba(7, 192, 95, 0.2);
      transform: translateY(-1px);
    }
  }
  
  &:not(.agent-active) {
    background: rgba(255, 255, 255, 0.8);
    border-color: #e0e0e0;
    
    .agent-mode-text {
      color: #666;
    }
    
    .normal-mode-icon {
      color: #666;
    }
    
    &:hover {
      background: rgba(255, 255, 255, 1);
      border-color: #b0b0b0;
      box-shadow: 0 2px 4px rgba(0, 0, 0, 0.08);
    }
  }
}

.agent-icon {
  width: 18px;
  height: 18px;
  flex-shrink: 0;
}

.agent-mode-text {
  font-size: 13px;
  color: #666;
  font-weight: 500;
  white-space: nowrap;
  margin: 0 4px;
}

.control-icon {
  width: 18px;
  height: 18px;
  transition: all 0.2s ease;
}

.kb-btn {
  height: 28px;
  padding: 0 10px;
  min-width: auto;
  
  &.active {
    background: rgba(7, 192, 95, 0.1);
    color: #07C05F;
    
    &:hover {
      background: rgba(7, 192, 95, 0.15);
    }
  }
}

.kb-btn-text {
  font-size: 13px;
  color: #666;
  font-weight: 500;
  white-space: nowrap;
}

.kb-btn.active .kb-btn-text {
  color: #07C05F;
}

.websearch-btn {
  width: 28px;
  height: 28px;
  padding: 0;
  min-width: auto;
  display: flex;
  align-items: center;
  justify-content: center;
  
  &.active {
    background: rgba(7, 192, 95, 0.1);
    
    .websearch-icon {
      color: #07C05F;
    }
    
    &:hover {
      background: rgba(7, 192, 95, 0.15);
    }
  }
  
  &:not(.active) {
    .websearch-icon {
      color: #666;
    }
    
    &:hover {
      background: #f0f0f0;
      
      .websearch-icon {
        color: #333;
      }
    }
  }
}

:global(.websearch-tooltip-disabled) {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-width: 220px;
  font-size: 12px;
  color: #666;
}

:global(.websearch-tooltip-disabled a) {
  color: #07C05F;
  font-weight: 500;
  text-decoration: none;
}

:global(.websearch-tooltip-disabled a:hover) {
  text-decoration: underline;
}

.websearch-icon {
  width: 18px;
  height: 18px;
  transition: all 0.2s ease;
}

.dropdown-arrow {
  transition: transform 0.2s ease;
  width: 10px;
  height: 10px;
  margin-left: 2px;
  
  &.rotate {
    transform: rotate(180deg);
  }
}

.kb-tags {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  overflow-x: auto;
  scrollbar-width: none;

  &::-webkit-scrollbar {
    display: none;
  }
}

.kb-tag {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 8px;
  background: rgba(7, 192, 95, 0.1);
  border: 1px solid rgba(7, 192, 95, 0.3);
  border-radius: 4px;
  font-size: 12px;
  color: #07C05F;
  white-space: nowrap;
  transition: all 0.15s ease;
  
  &:hover {
    background: rgba(7, 192, 95, 0.15);
  }
}

.kb-tag-text {
  max-width: 100px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.kb-tag-remove {
  cursor: pointer;
  font-weight: bold;
  font-size: 16px;
  line-height: 1;
  opacity: 0.7;
  
  &:hover {
    opacity: 1;
  }
}

.more-tag {
  background: rgba(0, 0, 0, 0.05);
  border-color: rgba(0, 0, 0, 0.1);
  color: #666;
  
  &:hover {
    background: rgba(0, 0, 0, 0.08);
  }
}

.control-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.stop-btn {
  width: 28px;
  height: 28px;
  padding: 0;
  background: rgba(7, 192, 95, 0.08);
  color: #07C05F;
  border: 1.5px solid rgba(7, 192, 95, 0.2);
  position: relative;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
  display: flex;
  align-items: center;
  justify-content: center;
  animation: stopBtnPulse 2s ease-in-out infinite;
  
  &:hover {
    background: rgba(7, 192, 95, 0.12);
    border-color: #07C05F;
    transform: scale(1.05);
    box-shadow: 0 2px 4px rgba(7, 192, 95, 0.2);
    animation: none;
  }
  
  &:active {
    transform: scale(0.95);
    background: rgba(7, 192, 95, 0.15);
  }
  
  svg {
    display: none;
  }
  
  &::before {
    content: '';
    width: 12px;
    height: 12px;
    background: #07C05F;
    border-radius: 50%;
    display: block;
    animation: stopDotPulse 2s ease-in-out infinite;
  }
  
  &:hover::before {
    animation: none;
  }
  
  &::after {
    content: '';
    position: absolute;
    width: 12px;
    height: 12px;
    border-radius: 50%;
    border: 2px solid #07C05F;
    animation: stopRipple 2s ease-out infinite;
  }
  
  &:hover::after {
    animation: none;
    opacity: 0;
  }
}

@keyframes stopBtnPulse {
  0%, 100% {
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
  }
  50% {
    box-shadow: 0 2px 4px rgba(7, 192, 95, 0.1);
  }
}

@keyframes stopDotPulse {
  0%, 100% {
    transform: scale(1);
    opacity: 1;
  }
  50% {
    transform: scale(0.92);
    opacity: 0.9;
  }
}

@keyframes stopRipple {
  0% {
    transform: scale(0.9);
    opacity: 0.4;
  }
  100% {
    transform: scale(2);
    opacity: 0;
  }
}

.send-btn {
  width: 28px;
  height: 28px;
  padding: 0;
  background-color: #07C05F;
  
  &:hover:not(.disabled) {
    background-color: #00a651;
    transform: scale(1.05);
  }
  
  &.disabled {
    background-color: #b5eccf;
  }
  
  img {
    width: 16px;
    height: 16px;
  }
}

/* 模型显示样式 */
.model-display {
  display: flex;
  align-items: center;
  margin-left: 8px;
  flex-shrink: 0;
}

.model-badge {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  background: rgba(7, 192, 95, 0.08);
  border-radius: 6px;
  font-size: 12px;
  white-space: nowrap;
}

.model-label {
  color: #666;
  font-weight: 500;
}

.model-name {
  color: #07C05F;
  font-weight: 600;
}

.model-btn {
  height: 28px;
  padding: 0 10px;
  min-width: auto;
  
  &.active {
    background: rgba(7, 192, 95, 0.1);
    color: #07C05F;
    
    &:hover {
      background: rgba(7, 192, 95, 0.15);
    }
  }
}

.model-btn-text {
  font-size: 13px;
  color: #666;
  font-weight: 500;
  white-space: nowrap;
  max-width: 150px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.model-btn.active .model-btn-text {
  color: #07C05F;
}

/* 模型选择下拉菜单 */
.model-selector-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 9998;
}

.model-selector-dropdown {
  z-index: 9999;
  background: white;
  border-radius: 8px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  max-width: 350px;
  max-height: 400px;
  overflow: hidden;
}

.model-selector-content {
  max-height: 400px;
  overflow-y: auto;
}

.model-option {
  padding: 12px 16px;
  cursor: pointer;
  transition: background 0.2s;
  border-bottom: 1px solid #f0f0f0;
  
  &:last-child {
    border-bottom: none;
  }
  
  &:hover {
    background: #f5f5f5;
  }
  
  &.selected {
    background: rgba(7, 192, 95, 0.08);
    
    .model-option-name {
      color: #07C05F;
      font-weight: 600;
    }
  }
  
  &.empty {
    color: #999;
    cursor: default;
    text-align: center;
    
    &:hover {
      background: white;
    }
  }
}

.model-option-main {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}

.model-option-name {
  font-size: 14px;
  color: #333;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.model-option-desc {
  font-size: 12px;
  color: #999;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.model-badge-remote,
.model-badge-local {
  display: inline-block;
  padding: 2px 6px;
  font-size: 11px;
  border-radius: 3px;
  font-weight: 500;
  flex-shrink: 0;
}

.model-badge-remote {
  background: rgba(7, 192, 95, 0.1);
  color: #07C05F;
}

.model-badge-local {
  background: rgba(82, 196, 26, 0.1);
  color: #52c41a;
}

/* Agent 模式选择下拉菜单 */
.agent-mode-selector-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 9998;
}

.agent-mode-selector-dropdown {
  z-index: 9999;
  background: white;
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12), 0 2px 8px rgba(7, 192, 95, 0.08);
  border: 1px solid #e5e7eb;
  overflow: hidden;
  padding: 4px;
  min-width: 200px;
}

.agent-mode-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px;
  cursor: pointer;
  transition: all 0.2s ease;
  border-radius: 6px;
  position: relative;
  margin: 2px 0;
  
  &:hover:not(.disabled) {
    background: rgba(7, 192, 95, 0.1);
    transform: translateX(2px);
  }
  
  &:active:not(.disabled) {
    transform: translateX(1px);
  }
  
  &.disabled {
    opacity: 0.6;
    cursor: not-allowed;
    
    &:hover {
      background: transparent;
      transform: none;
    }
  }
  
  // 选中状态的选项有更明显的背景
  &.selected {
    background: rgba(7, 192, 95, 0.06);
    
    .agent-mode-option-name {
      color: #07C05F;
      font-weight: 700;
    }
  }
}

.agent-mode-option-main {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
  min-width: 0;
}

.agent-mode-option-name {
  font-size: 14px;
  font-weight: 600;
  color: #333;
  line-height: 1.4;
  transition: color 0.2s ease;
}

.agent-mode-option-desc {
  font-size: 12px;
  color: #999;
  line-height: 1.3;
}

.check-icon {
  width: 16px;
  height: 16px;
  color: #07C05F;
  flex-shrink: 0;
  margin-left: 8px;
}

.agent-mode-warning {
  display: flex;
  align-items: center;
  margin-left: 8px;
  
  .warning-icon {
    color: #ff9800;
    font-size: 16px;
  }
}

.agent-mode-footer {
  padding: 8px 12px;
  border-top: 1px solid #f0f0f0;
  margin-top: 4px;
}

.agent-mode-link {
  color: #07C05F;
  text-decoration: none;
  font-size: 12px;
  font-weight: 500;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  transition: all 0.2s ease;
  
  &:hover {
    color: #00a651;
    text-decoration: underline;
  }
}
</style>


