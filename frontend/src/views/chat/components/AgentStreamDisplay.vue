<template>
  <div ref="rootElement" class="agent-stream-display">
    
    <!-- Collapsed intermediate steps -->
    <div v-if="shouldShowCollapsedSteps" class="intermediate-steps-collapsed">
      <div class="intermediate-steps-header" @click="toggleIntermediateSteps">
        <div class="intermediate-steps-title">
          <img :src="agentIcon" alt="" />
          <span v-html="intermediateStepsSummaryHtml"></span>
        </div>
        <div class="intermediate-steps-show-icon">
          <t-icon :name="showIntermediateSteps ? 'chevron-up' : 'chevron-down'" />
        </div>
      </div>
    </div>
    
    <!-- Event Stream -->
    <template v-for="(event, index) in displayEvents" :key="getEventKey(event, index)">
      <div v-if="event && event.type" class="event-item" :data-event-index="index">
        
        <!-- Plan Task Change Event -->
        <div v-if="event.type === 'plan_task_change'" class="plan-task-change-event">
          <div class="plan-task-change-card">
            <div class="plan-task-change-content">
              <strong>{{ $t('agent.taskLabel') }}</strong> {{ event.task }}
            </div>
          </div>
        </div>
        
        <!-- Thinking Event -->
        <div v-if="event.type === 'thinking'" class="thinking-event">
          <div 
            class="thinking-phase" 
            :class="{ 
              'thinking-active': event.thinking,
              'thinking-last': isLastThinking(event.event_id)
            }"
          >
            <div v-if="event.content" 
                 class="thinking-content markdown-content" 
                 v-html="renderMarkdown(event.content)">
            </div>
          </div>
        </div>
        
        <!-- Answer Event -->
        <div v-else-if="event.type === 'answer' && (event.done || (event.content && event.content.trim()))" class="answer-event">
          <div 
            v-if="event.content && event.content.trim()"
            class="answer-content-wrapper"
            :class="{ 
              'answer-active': !event.done,
              'answer-done': event.done
            }"
          >
            <div class="answer-content markdown-content" 
                 v-html="renderMarkdown(event.content)">
            </div>
          </div>
          <div v-if="event.done" class="answer-toolbar">
            <t-button size="small" variant="outline" shape="round" @click.stop="handleCopyAnswer(event)" :title="$t('agent.copy')">
              <t-icon name="copy" />
            </t-button>
            <t-button size="small" variant="outline" shape="round" @click.stop="handleAddToKnowledge(event)" :title="$t('agent.addToKnowledgeBase')">
              <t-icon name="add" />
            </t-button>
          </div>
        </div>
        
        <!-- Tool Call Event -->
        <div v-else-if="event.type === 'tool_call'" class="tool-event">
        <div 
          class="action-card" 
          :class="{ 
            'action-pending': event.pending,
            'action-error': event.success === false 
          }"
        >
          <div class="action-header" @click="toggleEvent(event.tool_call_id)">
            <div class="action-title">
              <img v-if="event.tool_name && !isBookIcon(event.tool_name)" class="action-title-icon" :src="getToolIcon(event.tool_name)" alt="" />
              <t-icon v-if="event.tool_name && isBookIcon(event.tool_name)" class="action-title-icon" name="book" />
              <!-- Custom header for todo_write tool -->
              <t-tooltip v-if="event.tool_name === 'todo_write' && event.tool_data?.steps" :content="t('agent.updatePlan')" placement="top">
                <span class="action-name">
                  {{ $t('agent.updatePlan') }}
                </span>
              </t-tooltip>
              <!-- Use tool summary as title if available, otherwise use description -->
              <t-tooltip v-else :content="getToolTitle(event)" placement="top">
                <span class="action-name">{{ getToolTitle(event) }}</span>
              </t-tooltip>
            </div>
            <div v-if="!event.pending" class="action-show-icon">
              <t-icon :name="isEventExpanded(event.tool_call_id) ? 'chevron-up' : 'chevron-down'" />
            </div>
          </div>
          
          <!-- Plan Status Summary (Fixed, always visible, outside action-details) -->
          <div v-if="!event.pending && event.tool_name === 'todo_write' && event.tool_data?.steps" class="plan-status-summary-fixed">
            <div class="plan-status-text">
              <template v-for="(part, partIndex) in getPlanStatusItems(event)" :key="partIndex">
                <t-icon :name="part.icon" :class="['status-icon', part.class]" />
                <span>{{ part.label }} {{ part.count }}</span>
                <span v-if="partIndex < getPlanStatusItems(event).length - 1" class="separator">·</span>
              </template>
            </div>
          </div>
          
          <!-- Search Results Summary (Fixed, always visible, outside action-details) -->
          <div v-if="!event.pending && (event.tool_name === 'search_knowledge' || event.tool_name === 'knowledge_search') && event.tool_data" class="search-results-summary-fixed">
            <div class="results-summary-text" v-html="getSearchResultsSummary(event.tool_data)"></div>
          </div>
          
          <!-- Web Search Results Summary (Fixed, always visible, outside action-details) -->
          <div v-if="!event.pending && event.tool_name === 'web_search' && event.tool_data" class="search-results-summary-fixed">
            <div class="results-summary-text" v-html="t('agent.webSearchFound', { count: getResultsCount(event.tool_data) })"></div>
          </div>
          
          <div v-if="isEventExpanded(event.tool_call_id) && !event.pending" class="action-details">
            <!-- Thinking tool: only render markdown thought content -->
            <template v-if="event.tool_name === 'thinking' && event.tool_data?.thought">
              <div class="thinking-thought-content">
                <div class="thinking-thought-markdown markdown-content" v-html="renderMarkdown(event.tool_data.thought)"></div>
              </div>
            </template>
            
            <!-- For other tools: show ToolResultRenderer or output -->
            <template v-else>
              <!-- Use ToolResultRenderer if display_type is available -->
              <div v-if="event.display_type && event.tool_data" class="tool-result-wrapper">
                <ToolResultRenderer 
                  :display-type="event.display_type"
                  :tool-data="event.tool_data"
                  :output="event.output"
                  :arguments="event.arguments"
                />
              </div>
              
              <!-- Fallback to original output display -->
              <div v-else-if="event.output" class="tool-output-wrapper">
                <div class="fallback-header">
                  <span class="fallback-label">{{ $t('chat.rawOutputLabel') }}</span>
                </div>
                <div class="detail-output-wrapper">
                  <div class="detail-output">{{ event.output }}</div>
                </div>
              </div>
              
              <!-- Show Arguments only if no display_type and not for todo_write -->
              <div v-if="event.arguments && event.tool_name !== 'todo_write' && !event.display_type" class="tool-arguments-wrapper">
                <div class="arguments-header">
                  <span class="arguments-label">{{ $t('agent.argumentsLabel') }}</span>
                </div>
                <pre class="detail-code">{{ formatJSON(event.arguments) }}</pre>
              </div>
            </template>
          </div>
        </div>
      </div>
      </div>
    </template>
    
    <!-- Loading Indicator -->
    <div v-if="!isConversationDone && eventStream.length > 0" class="loading-indicator">
      <img class="botanswer_loading_gif" src="@/assets/img/botanswer_loading.gif" alt="Processing...">
    </div>
  </div>
  <!-- 全局浮层：统一承载 Web/KB 的 hover 内容 -->
  <Teleport to="body">
    <div
      v-if="floatPopup.visible"
      class="kb-float-popup"
      :style="{ top: floatPopup.top + 'px', left: floatPopup.left + 'px', width: floatPopup.width + 'px' }"
      @mouseenter="cancelFloatClose()"
      @mouseleave="scheduleFloatClose()"
    >
      <div class="t-popup__content">
        <template v-if="floatPopup.type === 'web'">
          <div class="tip-title">{{ floatPopup.title || '' }}</div>
          <div class="tip-url">{{ floatPopup.url || '' }}</div>
        </template>
        <template v-else>
          <div v-if="floatPopup.knowledgeTitle" class="tip-meta"><strong>{{ floatPopup.knowledgeTitle }}</strong></div>
          <div v-if="floatPopup.loading" class="tip-loading">{{ $t('common.loading') }}</div>
          <div v-else-if="floatPopup.error" class="tip-error">{{ floatPopup.error }}</div>
          <div v-else class="tip-content" v-html="floatPopup.content"></div>
          <div v-if="floatPopup.chunkId" class="tip-meta">{{ $t('chat.chunkIdLabel') }} {{ floatPopup.chunkId }}</div>
        </template>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue';
import { useRouter } from 'vue-router';
import { marked } from 'marked';
import DOMPurify from 'dompurify';
import ToolResultRenderer from './ToolResultRenderer.vue';
import { getChunkByIdOnly } from '@/api/knowledge-base';
import { MessagePlugin } from 'tdesign-vue-next';
import { useUIStore } from '@/stores/ui';
import { useI18n } from 'vue-i18n';

const router = useRouter();
const uiStore = useUIStore();
const { t } = useI18n();

const TOOL_NAME_I18N: Record<string, string> = {
  search_knowledge: '知识库检索',
  knowledge_search: '知识库检索',
  web_search: '网络搜索',
  web_fetch: '网页抓取',
  get_document_info: '获取文档信息',
  list_knowledge_chunks: '查看知识分块',
  get_related_documents: '查找相关文档',
  get_document_content: '获取文档内容',
  todo_write: '计划管理',
  knowledge_graph_extract: '知识图谱抽取',
  thinking: '思考',
};

const getLocalizedToolName = (toolName?: string | null): string => {
  if (!toolName) return t('agent.toolFallback');
  return TOOL_NAME_I18N[toolName] || toolName;
};

// 根元素引用
const rootElement = ref<HTMLElement | null>(null);

// 浮层状态（Web/KB 共用）
const KB_SNIPPET_LIMIT = 600;

const floatPopup = ref<{
  visible: boolean;
  top: number;
  left: number;
  width: number;
  type: 'kb' | 'web';
  // web
  url?: string;
  title?: string;
  // kb
  loading: boolean;
  error?: string;
  content?: string;
  chunkId?: string;
  knowledgeTitle?: string;
}>({
  visible: false,
  top: 0,
  left: 0,
  width: 420,
  type: 'kb',
  url: '',
  title: '',
  loading: false,
  error: undefined,
  content: '',
  chunkId: undefined,
});
let floatCloseTimer: number | null = null;

const scheduleFloatClose = () => {
  if (floatCloseTimer) window.clearTimeout(floatCloseTimer);
  floatCloseTimer = window.setTimeout(() => {
    // Double-check mouse is not over citation or popup before closing
    const hoveredCitation = document.querySelector('.citation-kb:hover, .citation-web:hover');
    const hoveredPopup = document.querySelector('.kb-float-popup:hover');
    if (!hoveredCitation && !hoveredPopup) {
      floatPopup.value.visible = false;
    }
  }, 300);
};

const cancelFloatClose = () => {
  if (floatCloseTimer) {
    window.clearTimeout(floatCloseTimer);
    floatCloseTimer = null;
  }
};

const openFloatForEl = (el: HTMLElement, widthAdjust = 120) => {
  const rect = el.getBoundingClientRect();
  const pageTop = window.scrollY || document.documentElement.scrollTop || 0;
  const pageLeft = window.scrollX || document.documentElement.scrollLeft || 0;
  // Reduce gap to minimize mouseout triggers when moving to popup
  floatPopup.value.top = rect.bottom + pageTop + 1;
  floatPopup.value.left = rect.left + pageLeft;
  floatPopup.value.width = Math.min(520, Math.max(380, rect.width + widthAdjust));
  floatPopup.value.visible = true;
  // Cancel any pending close when opening
  cancelFloatClose();
};

// Import icons
import agentIcon from '@/assets/img/agent.svg';
import thinkingIcon from '@/assets/img/Frame3718.svg';
import knowledgeIcon from '@/assets/img/zhishiku-thin.svg';
import documentIcon from '@/assets/img/ziliao.svg';
import fileAddIcon from '@/assets/img/file-add-green.svg';
import webSearchGlobeGreenIcon from '@/assets/img/websearch-globe-green.svg';

interface SessionData {
  isAgentMode?: boolean;
  agentEventStream?: any[];
  knowledge_references?: any[];
}

const props = defineProps<{
  session: SessionData;
  userQuery?: string;
}>();

// Configure marked for security
marked.use({
  mangle: false,
  headerIds: false
});

// Event stream
const eventStream = computed(() => props.session?.agentEventStream || []);

// Expanded events tracking (for tool calls)
// Initialize with thinking tools expanded by default
const expandedEvents = ref<Set<string>>(new Set());

// Watch event stream to auto-expand thinking tools
watch(eventStream, (stream) => {
  if (!stream || !Array.isArray(stream)) return;
  
  stream.forEach((event: any) => {
    if (event?.type === 'tool_call' && event?.tool_name === 'thinking' && event?.tool_call_id) {
      expandedEvents.value.add(event.tool_call_id);
    }
  });
}, { immediate: true, deep: true });

// State for intermediate steps collapse
const showIntermediateSteps = ref(false);

// Find the last thinking event in the current message's event stream
// Only the last thinking should have the green border-left
const lastThinkingEventId = computed(() => {
  const stream = eventStream.value;
  if (!stream || stream.length === 0) return null;
  
  // Find all thinking events
  const thinkingEvents = stream.filter((e: any) => e.type === 'thinking');
  if (thinkingEvents.length === 0) return null;
  
  // Return the event_id of the last thinking event
  const lastThinking = thinkingEvents[thinkingEvents.length - 1];
  return lastThinking.event_id;
});

// Check if a thinking event is the last one (should have green border)
const isLastThinking = (eventId: string): boolean => {
  return eventId === lastThinkingEventId.value;
};

// Check if conversation is done (based on answer event with done=true or stop event)
const isConversationDone = computed(() => {
  const stream = eventStream.value;
  if (!stream || stream.length === 0) {
    console.log('[Collapse] No stream or empty stream');
    return false;
  }
  
  // Check for stop event (user cancelled)
  const stopEvent = stream.find((e: any) => e.type === 'stop');
  if (stopEvent) {
    console.log('[Collapse] Found stop event, conversation done');
    return true;
  }
  
  // Check for answer event with done=true
  const answerEvents = stream.filter((e: any) => e.type === 'answer');
  const doneAnswer = answerEvents.find((e: any) => e.done === true);
  
  console.log('[Collapse] Answer events:', answerEvents.length, 'Done answer:', !!doneAnswer);
  
  return !!doneAnswer;
});

// Find the final content to display (last thinking or answer)
const finalContent = computed(() => {
  const stream = eventStream.value;
  if (!stream || stream.length === 0) {
    console.log('[Collapse] finalContent: no stream');
    return null;
  }
  
  if (!isConversationDone.value) {
    console.log('[Collapse] finalContent: not done yet');
    return null;
  }
  
  // Check if there's an answer event
  const answerEvents = stream.filter((e: any) => e.type === 'answer');
  const doneAnswer = answerEvents.find((e: any) => e.done === true);
  const hasAnswerContent = answerEvents.some((e: any) => e.content && e.content.trim());
  
  console.log('[Collapse] Answer events:', answerEvents.length, 'Done answer:', !!doneAnswer, 'Has content:', hasAnswerContent);
  
  // Priority: answer with content > last thinking (if answer is empty)
  if (hasAnswerContent) {
    // Answer has content, it's the final content
    console.log('[Collapse] finalContent: showing answer with content');
    return { type: 'answer' };
  } else if (doneAnswer) {
    // Answer is done but empty, find last thinking with content to show as final content
    // (answer toolbar will still be shown via displayEvents logic)
    const thinkingEvents = stream.filter((e: any) => e.type === 'thinking' && e.content && e.content.trim());
    console.log('[Collapse] Thinking events with content:', thinkingEvents.length);
    
    if (thinkingEvents.length > 0) {
      const lastThinking = thinkingEvents[thinkingEvents.length - 1];
      console.log('[Collapse] finalContent: showing last thinking (answer empty, toolbar will show)', lastThinking.event_id);
      return { type: 'thinking', event_id: lastThinking.event_id, showAnswerToolbar: true };
    } else {
      // No thinking content, show empty answer
      console.log('[Collapse] finalContent: showing empty answer');
      return { type: 'answer' };
    }
  } else {
    // Answer is empty, find last thinking with content
    const thinkingEvents = stream.filter((e: any) => e.type === 'thinking' && e.content && e.content.trim());
    console.log('[Collapse] Thinking events with content:', thinkingEvents.length);
    
    if (thinkingEvents.length > 0) {
      const lastThinking = thinkingEvents[thinkingEvents.length - 1];
      console.log('[Collapse] finalContent: showing last thinking', lastThinking.event_id);
      return { type: 'thinking', event_id: lastThinking.event_id };
    }
  }
  
  console.log('[Collapse] finalContent: no final content found');
  return null;
});

// Count intermediate steps (tools + thinking that will be collapsed)
const intermediateStepsCount = computed(() => {
  if (!finalContent.value) {
    console.log('[Collapse] intermediateStepsCount: no final content');
    return 0;
  }
  
  const stream = eventStream.value;
  let count = 0;
  
  for (const event of stream) {
    if (event.type === 'tool_call') {
      count++;
    } else if (event.type === 'thinking' && event.content) {
      // Count if it's not the final thinking
      if (finalContent.value.type !== 'thinking' || event.event_id !== finalContent.value.event_id) {
        count++;
      }
    }
  }
  
  console.log('[Collapse] intermediateStepsCount:', count);
  return count;
});

// Get intermediate steps summary with special info
const intermediateStepsSummary = computed(() => {
  if (!finalContent.value || !eventStream.value) {
    return '';
  }
  
  const stream = eventStream.value;
  const toolCalls: string[] = [];
  let searchCount = 0;
  let thinkingCount = 0;
  
  for (const event of stream) {
    if (event.type === 'tool_call' && event.tool_name) {
      const toolName = event.tool_name;
      if (toolName === 'search_knowledge' || toolName === 'knowledge_search') {
        searchCount++;
      } else if (toolName === 'thinking') {
        // Count if it's not the final thinking
        if (finalContent.value.type !== 'thinking' || event.event_id !== finalContent.value.event_id) {
          thinkingCount++;
        }
      } else if (toolName !== 'todo_write') {
        // Only add unique tool names
        if (!toolCalls.includes(toolName)) {
          toolCalls.push(toolName);
        }
      }
      
    } else if (event.type === 'thinking' && event.content) {
      // Count if it's not the final thinking
      if (finalContent.value.type !== 'thinking' || event.event_id !== finalContent.value.event_id) {
        thinkingCount++;
      }
    } 
  }
  
  const parts: string[] = [];
  if (searchCount > 0) {
    parts.push(`检索知识库 <strong>${searchCount}</strong> 次`);
  }
  if (thinkingCount > 0) {
    parts.push(`思考 <strong>${thinkingCount}</strong> 次`);
  }
  if (toolCalls.length > 0) {
    const toolNames = toolCalls.map(name => {
      if (name === 'get_document_info') return '获取文档';
      if (name === 'list_knowledge_chunks') return '查看知识分块';
      return name;
    });
    if (toolNames.length === 1) {
      parts.push(`调用 ${toolNames[0]}`);
    } else {
      parts.push(`调用工具 ${toolNames.join('、')}`);
    }
  }
  
  if (parts.length === 0) {
    return `<strong>${intermediateStepsCount.value}</strong> 个中间步骤`;
  }
  
  // 优化连接词，使语句更流畅
  if (parts.length === 1) {
    return parts[0];
  } else if (parts.length === 2) {
    return `${parts[0]}，${parts[1]}`;
  } else {
    // 3个或以上：前几个用顿号，最后一个用逗号
    const last = parts.pop();
    return `${parts.join('、')}，${last}`;
  }
});

// HTML version of intermediate steps summary with colored numbers
const intermediateStepsSummaryHtml = computed(() => {
  return intermediateStepsSummary.value;
});

// Should show the collapsed steps indicator
const shouldShowCollapsedSteps = computed(() => {
  const result = isConversationDone.value && intermediateStepsCount.value > 0;
  console.log('[Collapse] shouldShowCollapsedSteps:', result, 'done:', isConversationDone.value, 'count:', intermediateStepsCount.value);
  return result;
});

// Events to display (based on collapse state)
const displayEvents = computed(() => {
  const stream = eventStream.value;
  if (!stream || !Array.isArray(stream)) {
    console.log('[Collapse] displayEvents: no stream or not array');
    return [];
  }
  
  // Filter out invalid events
  const validStream = stream.filter((e: any) => e && typeof e === 'object' && e.type);
  
  console.log('[Collapse] displayEvents: total stream length:', validStream.length);
  
  // Track task changes for todo_write events
  // This works for both real-time streaming and historical messages
  let lastTask: string | null = null;
  const result: any[] = [];
  
  for (let i = 0; i < validStream.length; i++) {
    const event = validStream[i];
    
    // Check if this is a todo_write event with task change
    if (event.type === 'tool_call' && event.tool_name === 'todo_write' && event.tool_data?.task) {
      const currentTask = event.tool_data.task;
      
      // If task changed (or is first task), insert a task change event before the todo_write event
      // For historical messages, we need to show the first task as well
      if (lastTask === null || currentTask !== lastTask) {
        result.push({
          type: 'plan_task_change',
          task: currentTask,
          event_id: `plan-task-change-${event.tool_call_id || i}`,
          timestamp: event.timestamp || Date.now()
        });
      }
      
      lastTask = currentTask;
    }
    
    result.push(event);
  }
  
  // If not done, show everything (with task change events)
  if (!isConversationDone.value) {
    console.log('[Collapse] displayEvents: not done, showing all', result.length);
    return result;
  }
  
  // If done but user wants to see intermediate steps, show all
  if (showIntermediateSteps.value) {
    console.log('[Collapse] displayEvents: user expanded, showing all', result.length);
    return result;
  }
  
  // Otherwise, show only final content
  const final = finalContent.value;
  if (!final) {
    console.log('[Collapse] displayEvents: no final content, showing all', result.length);
    return result;
  }
  
  if (final.type === 'answer') {
    // Filter to show only answer events
    const filtered = result.filter((e: any) => e.type === 'answer');
    console.log('[Collapse] displayEvents: showing answer only', filtered.length);
    return filtered;
  } else if (final.type === 'thinking') {
    // Show the last thinking as final content
    const thinkingFiltered = result.filter((e: any) => 
      e.type === 'thinking' && e.event_id === final.event_id
    );
    
    // If answer is done but empty, also include answer event for toolbar
    if (final.showAnswerToolbar) {
      const answerEvents = result.filter((e: any) => e.type === 'answer' && e.done === true);
      const combined = [...thinkingFiltered, ...answerEvents];
      console.log('[Collapse] displayEvents: showing last thinking + answer toolbar', combined.length);
      return combined;
    }
    
    console.log('[Collapse] displayEvents: showing last thinking only', thinkingFiltered.length);
    return thinkingFiltered;
  }
  
  console.log('[Collapse] displayEvents: fallback, showing all', result.length);
  return result;
});

// Get unique key for event
const getEventKey = (event: any, index: number): string => {
  if (!event) return `event-${index}`;
  if (event.event_id) return `event-${event.event_id}`;
  if (event.tool_call_id) return `tool-${event.tool_call_id}`;
  return `event-${index}-${event.type || 'unknown'}`;
};

const toggleIntermediateSteps = () => {
  showIntermediateSteps.value = !showIntermediateSteps.value;
};

const toggleEvent = (eventId: string) => {
  if (expandedEvents.value.has(eventId)) {
    expandedEvents.value.delete(eventId);
  } else {
    expandedEvents.value.add(eventId);
  }
};

const isEventExpanded = (eventId: string): boolean => {
  return expandedEvents.value.has(eventId);
};

// Delegated handlers for span-based citation clicks/keyboard
const handleCitationActivate = (el: HTMLElement) => {
  const url = el.getAttribute('data-url');
  if (!url) return;
  try {
    const newWindow = window.open(url, '_blank', 'noopener,noreferrer');
    if (!newWindow) {
      window.location.assign(url);
    }
  } catch {
    window.location.assign(url);
  }
};

// KB citations: 悬停用浮层展示摘要；点击跳转 KB 详情
type KbTooltipState = {
  loading: boolean;
  error?: string;
  html?: string;
};

const kbChunkDetails = ref<Record<string, KbTooltipState>>({});

const escapeHtml = (value: string): string =>
  value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');

const buildKbTooltipContent = (content: string): string => {
  const escapedContent = escapeHtml(content).replace(/\n/g, '<br>');
  return `<span class="tip-content">${escapedContent}</span>`;
};

const getKbTooltipInnerHtml = (state: KbTooltipState): string => {
  if (state.error) {
    return `<span class="tip-error">${escapeHtml(state.error)}</span>`;
  }
  if (state.html) {
    return state.html;
  }
  return `<span class="tip-loading">加载中...</span>`;
};

const syncFloatPopupFromCache = (chunkId: string, state: KbTooltipState) => {
  if (floatPopup.value.type !== 'kb' || floatPopup.value.chunkId !== chunkId) {
    return;
  }
  floatPopup.value.loading = state.loading;
  floatPopup.value.error = state.error;
  floatPopup.value.content = state.html || '';
};

const setKbCacheState = (chunkId: string, state: KbTooltipState) => {
  kbChunkDetails.value[chunkId] = state;
  updateKBCitationTooltip(chunkId, state);
  syncFloatPopupFromCache(chunkId, state);
};

const loadChunkDetails = async (chunkId: string) => {
  const cacheEntry = kbChunkDetails.value[chunkId];
  if (cacheEntry) {
    if (cacheEntry.loading) {
      updateKBCitationTooltip(chunkId, cacheEntry);
      syncFloatPopupFromCache(chunkId, cacheEntry);
      return;
    }
    if (cacheEntry.html || cacheEntry.error) {
      updateKBCitationTooltip(chunkId, cacheEntry);
      syncFloatPopupFromCache(chunkId, cacheEntry);
      return;
    }
  }

  setKbCacheState(chunkId, { loading: true });

  try {
    const response = await getChunkByIdOnly(chunkId);
    const content = response.data?.content;
    if (content) {
      const html = buildKbTooltipContent(content);
      setKbCacheState(chunkId, { loading: false, html });
      return;
    }

    setKbCacheState(chunkId, { loading: false, error: '未找到内容' });
  } catch (error: any) {
    console.error('Failed to load chunk details:', error);
    const errorMsg = error?.message || '加载失败';
    setKbCacheState(chunkId, { loading: false, error: errorMsg });
  }
};

const updateKBCitationTooltip = (chunkId: string, state: KbTooltipState) => {
  // Find all KB citation elements with this chunk ID
  const citations = document.querySelectorAll(`.citation-kb[data-chunk-id="${chunkId}"]`);
  citations.forEach((citation) => {
    const tipElement = citation.querySelector('.citation-tip');
    if (tipElement) {
      const shortChunkId = `${chunkId.substring(0, 25)}...`;
      
      const renderContent = (inner: string) => {
        tipElement.innerHTML = `
          <span class="t-popup__content">
            ${inner}
            <span class="tip-meta">片段ID: ${shortChunkId}</span>
          </span>
        `;
      };

      renderContent(getKbTooltipInnerHtml(state));
    }
  });
};

// 统一 hover 入口（Web/KB）
let kbHoverTimer: number | null = null;
const onHover = (e: Event) => {
  const target = e.target as HTMLElement;
  if (!target) return;
  const kbEl = target.closest?.('.citation-kb') as HTMLElement | null;
  const webEl = target.closest?.('.citation-web') as HTMLElement | null;
  // KB
  if (kbEl) {
    const chunkId = kbEl.getAttribute('data-chunk-id') || '';
    const knowledgeTitle = kbEl.getAttribute('data-doc') || '';
    if (!chunkId) return;
    if (kbHoverTimer) window.clearTimeout(kbHoverTimer);
    kbHoverTimer = window.setTimeout(() => {
      cancelFloatClose();
      floatPopup.value.type = 'kb';
      floatPopup.value.chunkId = chunkId;
      floatPopup.value.knowledgeTitle = knowledgeTitle;
      const cacheEntry = kbChunkDetails.value[chunkId];
      if (cacheEntry) {
        syncFloatPopupFromCache(chunkId, cacheEntry);
        updateKBCitationTooltip(chunkId, cacheEntry);
      } else {
        floatPopup.value.loading = true;
        floatPopup.value.error = undefined;
        floatPopup.value.content = '';
      }
      openFloatForEl(kbEl);

      if (!cacheEntry || (!cacheEntry.loading && !cacheEntry.html && !cacheEntry.error)) {
        loadChunkDetails(chunkId);
      }
    }, 80);
    return;
  }
  // Web
  if (webEl) {
    const url = webEl.getAttribute('data-url') || '';
    const title = webEl.querySelector('.tip-title')?.textContent || webEl.getAttribute('data-title') || '';
    if (kbHoverTimer) window.clearTimeout(kbHoverTimer);
    kbHoverTimer = window.setTimeout(() => {
      cancelFloatClose(); // Cancel any pending close
      floatPopup.value.type = 'web';
      floatPopup.value.url = url;
      floatPopup.value.title = title || '';
      openFloatForEl(webEl, 60);
    }, 40);
    return;
  }
};

const onHoverOut = (e: Event) => {
  const rt = (e as MouseEvent).relatedTarget as HTMLElement | null;
  // If mouse is moving to another citation or the popup, don't close
  if (rt && (rt.closest?.('.citation-kb') || rt.closest?.('.citation-web') || rt.closest?.('.kb-float-popup'))) {
    return;
  }
  // Cancel any pending hover timer
  if (kbHoverTimer) {
    window.clearTimeout(kbHoverTimer);
    kbHoverTimer = null;
  }
  // Use a small delay to allow mouse to move to popup
  // The scheduleFloatClose will double-check before actually closing
  scheduleFloatClose();
};

const onRootClick = (e: Event) => {
  const target = e.target as HTMLElement;
  if (!target) return;
  
  // Handle web citation clicks
  const webEl = target.closest?.('.citation-web') as HTMLElement | null;
  if (webEl && webEl.getAttribute('data-url')) {
    if (!(webEl instanceof HTMLAnchorElement)) {
      e.preventDefault();
      handleCitationActivate(webEl);
    }
    return;
  }
  
  // Handle KB citation clicks -> navigate to KB detail page
  const kbEl = target.closest?.('.citation-kb') as HTMLElement | null;
  if (kbEl && kbEl.getAttribute('data-kb-id')) {
    e.preventDefault();
    e.stopPropagation();
    const kbId = kbEl.getAttribute('data-kb-id');
    if (kbId) {
      try {
        // Navigate to knowledge base detail page
        router.push(`/platform/knowledge-bases/${kbId}`);
      } catch (error) {
        console.error('Failed to navigate to knowledge base:', error);
      }
    }
    return;
  }
};

const onRootKeydown = (e: KeyboardEvent) => {
  const target = e.target as HTMLElement;
  if (!target) return;
  
  // Handle web citation keyboard
  const webEl = target.closest?.('.citation-web') as HTMLElement | null;
  if (webEl) {
    if (e.key === 'Enter' || e.key === ' ') {
      if (webEl instanceof HTMLAnchorElement && e.key === 'Enter') {
        return;
      }
      e.preventDefault();
      if (webEl instanceof HTMLAnchorElement) {
        webEl.click();
      } else {
        handleCitationActivate(webEl);
      }
    }
    return;
  }
  
  // Handle KB citation keyboard -> navigate to KB detail
  const kbEl = target.closest?.('.citation-kb') as HTMLElement | null;
  if (kbEl) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      const kbId = kbEl.getAttribute('data-kb-id');
      if (kbId) {
        try {
          router.push(`/platform/knowledge-bases/${kbId}`);
        } catch (error) {
          console.error('Failed to navigate to knowledge base:', error);
        }
      }
    }
    return;
  }
};

onMounted(() => {
  // 使用 nextTick 确保 DOM 已渲染
  nextTick(() => {
    const root = rootElement.value;
    if (!root) return;
    root.addEventListener('click', onRootClick, true);
    const keydownListener: EventListener = (evt: Event) => onRootKeydown(evt as KeyboardEvent);
    // Store on element for removal
    (root as any).__citationKeydown__ = keydownListener;
    root.addEventListener('keydown', keydownListener, true);
    // 统一 hover 监听
    root.addEventListener('mouseover', onHover, true);
    root.addEventListener('mouseout', onHoverOut, true);
    window.addEventListener('scroll', scheduleFloatClose, true);
    window.addEventListener('resize', scheduleFloatClose, true);
  });
});

onBeforeUnmount(() => {
  const root = rootElement.value;
  if (!root) return;
  root.removeEventListener('click', onRootClick, true);
  root.removeEventListener('mouseover', onHover, true);
  root.removeEventListener('mouseout', onHoverOut, true);
  window.removeEventListener('scroll', scheduleFloatClose, true);
  window.removeEventListener('resize', scheduleFloatClose, true);
  const keydownListener: EventListener | undefined = (root as any).__citationKeydown__;
  if (keydownListener) {
    root.removeEventListener('keydown', keydownListener, true);
    delete (root as any).__citationKeydown__;
  }
});

const ATTRIBUTE_REGEX = /([\w-]+)\s*=\s*"([^"]*)"/g;

const parseTagAttributes = (attrString: string): Record<string, string> => {
  const attributes: Record<string, string> = {};
  if (!attrString) return attributes;

  ATTRIBUTE_REGEX.lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = ATTRIBUTE_REGEX.exec(attrString)) !== null) {
    const key = match[1];
    const value = match[2];
    attributes[key] = value;
  }

  return attributes;
};

// Markdown rendering function
const renderMarkdown = (content: any): string => {
  if (!content) return '';
  
  // Ensure content is a string
  const contentStr = typeof content === 'string' ? content : String(content || '');
  if (!contentStr.trim()) return '';
  
  try {
    // Preprocess custom citation tags into safe HTML the sanitizer will allow
    // Supported formats:
    //   <kb kb_id="..." doc="..." chunk_id="..." />
    //   <web url="https://..." title="Example" />
    const processed = contentStr
      // Web citations -> compact clickable badges with domain text; hover shows title via native tooltip
      .replace(
        /<web\b([^>]*)\/>/g,
        (_m: string, attrString: string) => {
          const attrs = parseTagAttributes(attrString);
          const url = attrs.url || '';
          const title = attrs.title || '';

          if (!url) {
            return '';
          }

          // Extract domain for compact display
          let domain = url;
          try {
            const u = new URL(url);
            const host = u.hostname || '';
            const parts = host.split('.');
            if (parts.length >= 2) {
              // prefer second-level domain: last two labels
              domain = parts.slice(-2).join('.');
            } else {
              domain = host || url;
            }
          } catch {
            // keep original url text if parsing fails
          }
          // Escape double quotes in title for attribute safety
          const safeTitle = String(title || '').replace(/"/g, '&quot;');
          const safeUrl = String(url || '').replace(/"/g, '&quot;');
          // Render with embedded tooltip blocks to allow styled parts (bold title)
          const tipTitle = safeTitle || '';
          const tipUrl = safeUrl || '';
          // Keep lightweight tooltip (no t-popup container) to avoid global popup styling
          return `<a class="citation citation-web" data-url="${safeUrl}" href="${safeUrl}" target="_blank" rel="noopener noreferrer"><span class="citation-icon web"></span><span class="citation-domain">${domain}</span><span class="citation-tip"><span class="tip-title">${tipTitle}</span><span class="tip-url">${tipUrl}</span></span></a>`;
        }
      )
      // KB citations -> inline badges with simplified display
      .replace(
        /<kb\b([^>]*)\/>/g,
        (_m, attrString: string) => {
          const attrs = parseTagAttributes(attrString);
          const doc = attrs.doc || '';
          const chunkId = attrs.chunk_id || attrs.chunkId || '';
          const kbId = attrs.kb_id || attrs.kbId || '';

          if (!doc || !chunkId) {
            return '';
          }

          // Escape attributes for safety
          const safeDoc = escapeHtml(doc);
          const safeKbId = escapeHtml(kbId);
          const safeChunkId = escapeHtml(chunkId);

          const truncateMiddle = (text: string, maxLength = 13): string => {
            if (!text) return '';
            if (text.length <= maxLength) return text;
            const half = Math.floor((maxLength - 3) / 2);
            const start = text.slice(0, half + ((maxLength - 3) % 2));
            const end = text.slice(-half);
            return `${start}...${end}`;
          };

          const displayDoc = escapeHtml(truncateMiddle(doc));
          console.log('displayDoc', displayDoc);

          // Initial tooltip content (single t-popup container; will be updated on hover)
          return `<span class="citation citation-kb" data-kb-id="${safeKbId}" data-chunk-id="${safeChunkId}" data-doc="${safeDoc}" role="button" tabindex="0"><span class="citation-icon kb"></span><span class="citation-text">${displayDoc}</span><span class="citation-tip"><span class="t-popup__content"><span class="tip-loading">加载中...</span></span></span></span>`;
        }
      );

    const html = marked.parse(processed) as string;
    if (!html) return '';
    
    return DOMPurify.sanitize(html, {
      ALLOWED_TAGS: ['p', 'br', 'strong', 'em', 'u', 'code', 'pre', 'ul', 'ol', 'li', 'blockquote', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'a', 'span', 'table', 'thead', 'tbody', 'tr', 'th', 'td'],
      ALLOWED_ATTR: ['href', 'title', 'target', 'rel', 'data-tooltip', 'data-url', 'data-kb-id', 'data-chunk-id', 'data-doc', 'class', 'role', 'tabindex']
    });
  } catch (e) {
    console.error('Markdown rendering error:', e, 'Content:', contentStr.substring(0, 100));
    // Return escaped HTML instead of raw content for safety
    return contentStr.replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }
};

// Tool summary - extract key info to display externally
const getToolSummary = (event: any): string => {
  if (!event || event.pending || !event.success) return '';
  
  const toolName = event.tool_name;
  const toolData = event.tool_data;
  
  // For search tools, don't return summary here - it will be displayed in SearchResults component
  if (toolName === 'search_knowledge' || toolName === 'knowledge_search') {
    return '';
  } else if (toolName === 'get_document_info') {
    if (toolData?.title) {
      return `获取文档：${toolData.title}`;
    }
  } else if (toolName === 'list_knowledge_chunks') {
    if (toolData?.fetched_chunks !== undefined) {
      const title = toolData?.knowledge_title || toolData?.knowledge_id || '文档';
      return `查看 ${title} 的 ${toolData.fetched_chunks}/${toolData.total_chunks ?? '?'} 个分块`;
    }
  } else if (toolName === 'todo_write') {
    // Extract steps from tool data
    const steps = toolData?.steps;
    if (Array.isArray(steps)) {
      const inProgress = steps.filter((s: any) => s.status === 'in_progress').length;
      const pending = steps.filter((s: any) => s.status === 'pending').length;
      const completed = steps.filter((s: any) => s.status === 'completed').length;
      
      const parts = [];
      if (inProgress > 0) parts.push(`🚀 进行中 ${inProgress}`);
      if (pending > 0) parts.push(`📋 待处理 ${pending}`);
      if (completed > 0) parts.push(`✅ 已完成 ${completed}`);
      
      return parts.join(' · ');
    }
  } else if (toolName === 'thinking') {
    // Return truthy value to trigger rendering, actual content rendered in template
    return toolData?.thought ? '深度思考' : '';
  }
  
  return '';
};

// Get plan status parts for todo_write tool header
const getPlanStatusParts = (event: any) => {
  if (!event || !event.tool_data?.steps) {
    return { inProgress: 0, pending: 0, completed: 0 };
  }
  
  const steps = event.tool_data.steps;
  if (!Array.isArray(steps)) {
    return { inProgress: 0, pending: 0, completed: 0 };
  }
  
  return {
    inProgress: steps.filter((s: any) => s.status === 'in_progress').length,
    pending: steps.filter((s: any) => s.status === 'pending').length,
    completed: steps.filter((s: any) => s.status === 'completed').length
  };
};

// Get plan status items for display with icons
const getPlanStatusItems = (event: any) => {
  const parts = getPlanStatusParts(event);
  const items: Array<{ icon: string; class: string; label: string; count: number }> = [];
  
  if (parts.inProgress > 0) {
    items.push({
      icon: 'play-circle-filled',
      class: 'in-progress',
      label: '进行中',
      count: parts.inProgress
    });
  }
  
  if (parts.pending > 0) {
    items.push({
      icon: 'time',
      class: 'pending',
      label: '待处理',
      count: parts.pending
    });
  }
  
  if (parts.completed > 0) {
    items.push({
      icon: 'check-circle-filled',
      class: 'completed',
      label: '已完成',
      count: parts.completed
    });
  }
  
  return items;
};

// Get plan status summary for todo_write tool header (deprecated, use getPlanStatusParts instead)
const getPlanStatusSummary = (event: any): string => {
  const parts = getPlanStatusParts(event);
  const textParts = [];
  if (parts.inProgress > 0) textParts.push(`🚀 进行中 ${parts.inProgress}`);
  if (parts.pending > 0) textParts.push(`📋 待处理 ${parts.pending}`);
  if (parts.completed > 0) textParts.push(`✅ 已完成 ${parts.completed}`);
  return textParts.length > 0 ? textParts.join(' · ') : '';
};

// Check if tool should use book icon
const isBookIcon = (toolName: string): boolean => {
  return false; // 不再使用 t-icon 的 book，改用 SVG 图标
};

// Get icon for tool type
const getToolIcon = (toolName: string): string => {
  if (toolName === 'thinking') {
    return thinkingIcon;
  } else if (toolName === 'search_knowledge' || toolName === 'knowledge_search') {
    return knowledgeIcon;
  } else if (toolName === 'web_search') {
    return webSearchGlobeGreenIcon;
  } else if (toolName === 'get_document_info' || toolName === 'list_knowledge_chunks') {
    return documentIcon;
  } else if (toolName === 'todo_write') {
    return fileAddIcon;
  } else {
    return documentIcon; // default icon
  }
};

// Get search results summary text (returns HTML with colored numbers)
const getSearchResultsSummary = (toolData: any): string => {
  if (!toolData) return '';
  
  const count = toolData.results?.length || toolData.count || 0;
  if (count === 0) return '';
  
  const kbCount = toolData.kb_counts ? Object.keys(toolData.kb_counts).length : 0;
  if (kbCount > 0) {
    return `找到 <strong>${count}</strong> 个结果，来自 <strong>${kbCount}</strong> 个知识库`;
  }
  return `找到 <strong>${count}</strong> 个结果`;
};

// Get web search results summary text
const getWebSearchResultsSummary = (toolData: any): string => {
  if (!toolData) return '';
  
  const count = toolData.results?.length || toolData.count || 0;
  if (count === 0) return '';
  
  return `找到 ${count} 个网络搜索结果`;
};

// Get results count (number only) for web search summary
const getResultsCount = (toolData: any): number => {
  if (!toolData) return 0;
  return toolData.results?.length || toolData.count || 0;
};

// Extract and format query parameters from args
const getQueryText = (args: any): string => {
  if (!args) return '';
  
  // Parse if it's a string
  let parsedArgs = args;
  if (typeof parsedArgs === 'string') {
    try {
      parsedArgs = JSON.parse(parsedArgs);
    } catch (e) {
      return '';
    }
  }
  
  if (!parsedArgs || typeof parsedArgs !== 'object') return '';
  
  const queries: string[] = [];
  
  // Add query if exists
  if (parsedArgs.query && typeof parsedArgs.query === 'string') {
    queries.push(parsedArgs.query);
  }
  
  // Add vector_queries if exists
  if (Array.isArray(parsedArgs.vector_queries) && parsedArgs.vector_queries.length > 0) {
    const vectorQueries = parsedArgs.vector_queries
      .filter((q: any) => q && typeof q === 'string')
      .join(' ');
    if (vectorQueries) {
      queries.push(vectorQueries);
    }
  }
  
  // Add keyword_queries if exists
  if (Array.isArray(parsedArgs.keyword_queries) && parsedArgs.keyword_queries.length > 0) {
    const keywordQueries = parsedArgs.keyword_queries
      .filter((q: any) => q && typeof q === 'string')
      .join(' ');
    if (keywordQueries) {
      queries.push(keywordQueries);
    }
  }
  
  // Join all queries with space and remove duplicates
  const uniqueQueries = Array.from(new Set(queries));
  return uniqueQueries.join(' ');
};

// Get tool title - prefer summary over description, add query for search tools
const getToolTitle = (event: any): string => {
  if (event.pending) {
    const localizedName = getLocalizedToolName(event.tool_name);
    return `正在调用 ${localizedName}...`;
  }
  
  const toolName = event.tool_name;
  const isSearchTool = toolName === 'search_knowledge' || toolName === 'knowledge_search';
  const isWebSearchTool = toolName === 'web_search';
  
  // For search tools, use description with query text
  if (isSearchTool) {
    const baseTitle = getToolDescription(event);
    if (event.arguments) {
      const queryText = getQueryText(event.arguments);
      if (queryText) {
        return `${baseTitle}：「${queryText}」`;
      }
    }
    return baseTitle;
  }
  
  // For web search tools, use description with query text
  if (isWebSearchTool) {
    const baseTitle = getToolDescription(event);
    // Try to get query from arguments or tool_data
    let queryText = '';
    if (event.arguments && typeof event.arguments === 'object' && event.arguments.query) {
      queryText = event.arguments.query;
    } else if (event.tool_data && event.tool_data.query) {
      queryText = event.tool_data.query;
    }
    if (queryText) {
      return `${baseTitle}：「${queryText}」`;
    }
    return baseTitle;
  }
  
  // Use tool summary if available
  const summary = getToolSummary(event);
  return summary || getToolDescription(event);
};

// Tool description
const getToolDescription = (event: any): string => {
  if (event.pending) {
    const localizedName = getLocalizedToolName(event.tool_name);
    return `正在调用 ${localizedName}...`;
  }
  
  const success = event.success === true;
  const toolName = event.tool_name;
  
  if (toolName === 'search_knowledge' || toolName === 'knowledge_search') {
    return success ? '检索知识库' : '检索知识库失败';
  } else if (toolName === 'web_search') {
    return success ? '网络搜索' : '网络搜索失败';
  } else if (toolName === 'get_document_info') {
    return success ? '获取文档信息' : '获取文档信息失败';
  } else if (toolName === 'thinking') {
    return success ? '完成思考' : '思考失败';
  } else if (toolName === 'todo_write') {
    return success ? '更新任务列表' : '更新任务列表失败';
  } else {
    const localizedName = getLocalizedToolName(toolName);
    return success ? `调用 ${localizedName}` : `调用 ${localizedName} 失败`;
  }
};

// Helper functions
const formatDuration = (ms?: number): string => {
  if (!ms) return '0s';
  if (ms < 1000) return `${ms}ms`;
  const seconds = Math.floor(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  return `${minutes}m ${remainingSeconds}s`;
};

const formatJSON = (obj: any): string => {
  try {
    if (typeof obj === 'string') {
      // Try to parse if it's a JSON string
      try {
        const parsed = JSON.parse(obj);
        return JSON.stringify(parsed, null, 2);
      } catch {
        return obj;
      }
    }
    return JSON.stringify(obj, null, 2);
  } catch {
    return String(obj);
  }
};

const buildManualMarkdown = (question: string, answer: string): string => {
  const safeQuestion = question?.trim() || '（无提问内容）';
  const safeAnswer = answer?.trim() || '（无回答内容）';
  return `${safeAnswer}`;
};

const formatManualTitle = (question: string): string => {
  if (!question) {
    return '会话摘录';
  }
  const condensed = question.replace(/\s+/g, ' ').trim();
  if (!condensed) {
    return '会话摘录';
  }
  return condensed.length > 40 ? `${condensed.slice(0, 40)}...` : condensed;
};

// Helper function to get actual content (from answer or last thinking)
const getActualContent = (answerEvent: any): string => {
  // First try to get content from answer event
  const answerContent = (answerEvent?.content || '').trim();
  if (answerContent) {
    return answerContent;
  }
  
  // If answer is empty, try to get from last thinking
  const stream = eventStream.value;
  if (stream && Array.isArray(stream)) {
    const thinkingEvents = stream.filter((e: any) => e.type === 'thinking' && e.content && e.content.trim());
    if (thinkingEvents.length > 0) {
      const lastThinking = thinkingEvents[thinkingEvents.length - 1];
      return (lastThinking.content || '').trim();
    }
  }
  
  return '';
};

const handleCopyAnswer = async (answerEvent: any) => {
  const content = getActualContent(answerEvent);
  if (!content) {
    MessagePlugin.warning('当前回答为空，无法复制');
    return;
  }

  try {
    // 尝试使用现代 Clipboard API
    if (navigator.clipboard && navigator.clipboard.writeText) {
      await navigator.clipboard.writeText(content);
      MessagePlugin.success('已复制到剪贴板');
    } else {
      // 降级到传统方式
      const textArea = document.createElement('textarea');
      textArea.value = content;
      textArea.style.position = 'fixed';
      textArea.style.opacity = '0';
      document.body.appendChild(textArea);
      textArea.select();
      document.execCommand('copy');
      document.body.removeChild(textArea);
      MessagePlugin.success('已复制到剪贴板');
    }
  } catch (err) {
    console.error('复制失败:', err);
    MessagePlugin.error('复制失败，请手动复制');
  }
};

const handleAddToKnowledge = (answerEvent: any) => {
  const content = getActualContent(answerEvent);
  if (!content) {
    MessagePlugin.warning('当前回答为空，无法保存到知识库');
    return;
  }

  const question = (props.userQuery || '').trim();
  const manualContent = buildManualMarkdown(question, content);
  const manualTitle = formatManualTitle(question);

  uiStore.openManualEditor({
    mode: 'create',
    title: manualTitle,
    content: manualContent,
    status: 'draft',
  });

  MessagePlugin.info('已打开编辑器，请选择知识库后保存');
};
</script>

<style lang="less" scoped>
@import '../../../components/css/markdown.less';

.agent-stream-display {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 12px;
}

.intermediate-steps-collapsed {
  display: flex;
  flex-direction: column;
  font-size: 14px;
  width: 100%;
  border-radius: 8px;
  background-color: #ffffff;
  border-left: 3px solid #07c05f;
  box-shadow: 0 2px 4px rgba(7, 192, 95, 0.08);
  overflow: hidden;
  box-sizing: border-box;
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  margin-bottom: 8px;
  
  .intermediate-steps-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 10px 14px;
    color: #333333;
    font-weight: 500;
    cursor: pointer;
  }
  
  .intermediate-steps-title {
    display: flex;
    align-items: center;
    
    img {
      width: 16px;
      height: 16px;
      margin-right: 8px;
    }
    
    span {
      white-space: nowrap;
      font-size: 14px;
      
      :deep(strong) {
        color: #07c05f;
        font-weight: 600;
      }
    }
  }
  
  .intermediate-steps-show-icon {
    font-size: 14px;
    padding: 0 2px 1px 2px;
    color: #07c05f;
  }
  
  .intermediate-steps-header:hover {
    background-color: rgba(7, 192, 95, 0.04);
  }
}

.event-item {
  margin-bottom: 0;
}

// Thinking Event
.thinking-event {
  animation: fadeInUp 0.3s ease-out;
  
  .thinking-phase {
    background: #ffffff;
    border-radius: 8px;
    padding: 1px 14px;
    // 默认不显示 border-left
    border-left: 3px solid transparent;
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
    transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
    
    // 只有最后一个 thinking 显示绿色 border-left
    &.thinking-last {
      border-left-color: #07c05f;
      box-shadow: 0 2px 4px rgba(7, 192, 95, 0.08);
    }
    
    // 进行中的 thinking（实时对话）显示绿色 border-left 和动画
    &.thinking-active {
      border-left-color: #07c05f;
      box-shadow: 0 2px 4px rgba(7, 192, 95, 0.08);
      animation: pulseBorder 2s ease-in-out infinite;
    }
  }
  
  .thinking-content {
    font-size: 14px;
    color: #333333;
    line-height: 1.6;
    
    &.markdown-content {
      :deep(p) {
        margin: 8px 0;
        line-height: 1.6;
      }
      
      :deep(code) {
        background: #f0f0f0;
        padding: 2px 6px;
        border-radius: 3px;
        font-family: 'Monaco', 'Courier New', monospace;
        font-size: 12px;
      }
      
      :deep(pre) {
        background: #f5f5f5;
        padding: 12px;
        border-radius: 4px;
        overflow-x: auto;
        margin: 8px 0;
        
        code {
          background: none;
          padding: 0;
        }
      }
      
      :deep(ul), :deep(ol) {
        margin: 8px 0;
        padding-left: 24px;
      }
      
      :deep(li) {
        margin: 4px 0;
      }
      
      :deep(blockquote) {
        border-left: 3px solid #07c05f;
        padding-left: 12px;
        margin: 8px 0;
        color: #666;
      }
      
      :deep(h1), :deep(h2), :deep(h3), :deep(h4), :deep(h5), :deep(h6) {
        margin: 12px 0 8px 0;
        font-weight: 600;
        color: #333;
      }
      
      :deep(a) {
        color: #07c05f;
        text-decoration: none;
        
        &:hover {
          text-decoration: underline;
        }
      }
      
      :deep(table) {
        border-collapse: collapse;
        margin: 8px 0;
        font-size: 12px;
        
        th, td {
          border: 1px solid #e5e7eb;
          padding: 6px 10px;
        }
        
        th {
          background: #f5f5f5;
          font-weight: 600;
        }
      }
    }
  }
}

// Answer Event - 类似 thinking 但有独特样式
.answer-event {
  animation: fadeInUp 0.3s ease-out;
  
  .answer-content-wrapper {
    background: #ffffff;
    border-radius: 8px;
    padding: 1px 14px;
    border-left: 3px solid #07c05f;
    box-shadow: 0 2px 4px rgba(7, 192, 95, 0.08);
    transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
    
    // 进行中的 answer 显示动画
    &.answer-active {
      animation: pulseBorder 2s ease-in-out infinite;
    }
    
    // 完成的 answer 保持绿色边框
    &.answer-done {
      border-left-color: #07c05f;
    }
  }
  
  .answer-content {
    font-size: 14px;
    color: #333333;
    line-height: 1.6;
    
    &.markdown-content {
      /* citation-web styles moved to global fallback below to avoid duplication */
      
      /* keyboard focus */
      :deep(.citation-web:focus-visible) {
        outline: 2px solid #34d399; /* green-400 */
        outline-offset: 2px;
      }
      
      /* KB citation styles are defined globally, no need to override here */
      
      :deep(p) {
        margin: 8px 0;
        line-height: 1.6;
      }
      
      :deep(code) {
        background: #f0f0f0;
        padding: 2px 6px;
        border-radius: 3px;
        font-family: 'Monaco', 'Courier New', monospace;
        font-size: 12px;
      }
      
      :deep(pre) {
        background: #f5f5f5;
        padding: 12px;
        border-radius: 4px;
        overflow-x: auto;
        margin: 8px 0;
        
        code {
          background: none;
          padding: 0;
        }
      }
      
      :deep(ul), :deep(ol) {
        margin: 8px 0;
        padding-left: 24px;
      }
      
      :deep(li) {
        margin: 4px 0;
      }
      
      :deep(blockquote) {
        border-left: 3px solid #07c05f;
        padding-left: 12px;
        margin: 8px 0;
        color: #666;
      }
      
      :deep(h1), :deep(h2), :deep(h3), :deep(h4), :deep(h5), :deep(h6) {
        margin: 12px 0 8px 0;
        font-weight: 600;
        color: #333;
      }
      
      :deep(a) {
        color: #07c05f;
        text-decoration: none;
        
        &:hover {
          text-decoration: underline;
        }
      }
      
      :deep(table) {
        border-collapse: collapse;
        margin: 8px 0;
        font-size: 12px;
        
        th, td {
          border: 1px solid #e5e7eb;
          padding: 6px 10px;
        }
        
        th {
          background: #f5f5f5;
          font-weight: 600;
        }
      }
    }
  }

  .answer-toolbar {
    display: flex;
    justify-content: flex-start;
    gap: 8px;
    margin-top: 5px;

    :deep(.t-button) {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      min-width: auto;
      width: auto;
      border: 1px solid #e0e0e0;
      border-radius: 6px;
      background: #ffffff;
      color: #666;
      transition: all 0.2s ease;
      
      // 确保按钮内容区域正确显示
      .t-button__content {
        display: inline-flex !important;
        align-items: center;
        justify-content: center;
        gap: 0;
      }
      
      // t-button__text 包含图标，需要显示但只显示图标
      .t-button__text {
        display: inline-flex !important;
        align-items: center;
        justify-content: center;
        gap: 0;
      }
      
      // 确保图标显示
      .t-icon {
        display: inline-flex !important;
        visibility: visible !important;
        opacity: 1 !important;
        align-items: center;
        justify-content: center;
        font-size: 16px;
        width: 16px;
        height: 16px;
        flex-shrink: 0;
        color: #666;
      }
      
      // 确保 SVG 图标也显示
      .t-icon svg {
        display: block !important;
        width: 16px;
        height: 16px;
      }
      
      // 隐藏文字节点（但不是图标）
      .t-button__text > :not(.t-icon) {
        display: none;
      }
      
      // Hover 效果
      &:hover:not(:disabled) {
        background: rgba(7, 192, 95, 0.08);
        border-color: rgba(7, 192, 95, 0.3);
        color: #07c05f;
        
        .t-icon {
          color: #07c05f;
        }
      }
      
      // Active 效果
      &:active:not(:disabled) {
        background: rgba(7, 192, 95, 0.12);
        border-color: rgba(7, 192, 95, 0.4);
        transform: translateY(0.5px);
      }
    }
  }
}

// Tool Event
.tool-event {
  animation: fadeInUp 0.3s ease-out;
  
  .action-card {
    background: #ffffff;
    border-radius: 8px;
    border: 1px solid #e5e7eb;
    overflow: hidden;
    position: relative;
    transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);

    > * {
      position: relative;
      z-index: 1;
    }

    &:hover {
      border-color: #07c05f;
      box-shadow: 0 2px 8px rgba(7, 192, 95, 0.12);
      // transform: translateY(-1px);
    }

    &.action-error {
      border-left: 3px solid #e34d59;
      animation: shakeError 0.4s ease-out;
    }
    
    &.action-pending {
      opacity: 1;
      box-shadow: none;
      border-color: rgba(7, 192, 95, 0.18);
      background: linear-gradient(120deg, rgba(7, 192, 95, 0.01), rgba(255, 255, 255, 0.98));

      &::after {
        content: '';
        position: absolute;
        inset: 0;
        background: linear-gradient(
          120deg,
          rgba(255, 255, 255, 0) 0%,
          rgba(255, 255, 255, 0.3) 40%,
          rgba(7, 192, 95, 0.06) 55%,
          rgba(255, 255, 255, 0) 85%
        );
        transform: translateX(-100%);
        animation: actionPendingShimmer 2.8s ease-in-out infinite;
        pointer-events: none;
        z-index: 0;
      }
    }
  }
  
  .tool-summary {
    padding: 8px 14px;
    font-size: 13px;
    color: #333333;
    background: #ffffff;
    border-top: 1px solid #f0f0f0;
    line-height: 1.6;
    font-weight: 500;
    animation: slideIn 0.25s ease-out;
    
    .tool-summary-markdown {
      font-weight: 400;
      line-height: 1.6;
      color: #333333;
      
      :deep(p) {
        margin: 4px 0;
        color: #333333;
      }
      
      :deep(ul), :deep(ol) {
        margin: 4px 0;
        padding-left: 20px;
      }
      
      :deep(code) {
        background: #f5f5f5;
        padding: 2px 6px;
        border-radius: 4px;
        font-size: 12px;
        color: #07c05f;
        font-weight: 500;
      }
      
      :deep(strong) {
        font-weight: 600;
        color: #333333;
      }
    }
  }
}

.action-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 14px;
  color: #333333;
  font-weight: 500;
  cursor: pointer;
  user-select: none;
  transition: background-color 0.25s cubic-bezier(0.4, 0, 0.2, 1);

  &:hover {
    background-color: rgba(7, 192, 95, 0.04);
  }
}

.action-title {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 0; // Allow flex item to shrink below content size
  
  .action-title-icon {
    width: 16px;
    height: 16px;
    color: #07c05f;
    fill: currentColor;
    flex-shrink: 0;
    
    // For t-icon component
    :deep(svg) {
      width: 16px;
      height: 16px;
      color: #07c05f;
      fill: currentColor;
    }
  }
  
  // Tooltip wrapper should also allow shrinking
  :deep(.t-tooltip) {
    flex: 1;
    min-width: 0;
  }
  
  .action-name {
    white-space: nowrap;
    font-size: 14px;
  }
}


@keyframes fadeInUp {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes slideInDown {
  from {
    opacity: 0;
    transform: translateY(-10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes slideIn {
  from {
    opacity: 0;
    transform: translateX(-8px);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}

@keyframes pulseBorder {
  0%, 100% {
    border-left-color: #07c05f;
    box-shadow: 0 2px 4px rgba(7, 192, 95, 0.08);
  }
  50% {
    border-left-color: #0ae06f;
    box-shadow: 0 2px 6px rgba(7, 192, 95, 0.15);
  }
}

@keyframes shakeError {
  0%, 100% {
    transform: translateX(0);
  }
  10%, 30%, 50%, 70%, 90% {
    transform: translateX(-3px);
  }
  20%, 40%, 60%, 80% {
    transform: translateX(3px);
  }
}

@keyframes actionPendingShimmer {
  0% {
    transform: translateX(-90%);
  }
  50% {
    transform: translateX(-5%);
  }
  100% {
    transform: translateX(90%);
  }
}

.action-name {
  font-size: 14px;
  font-weight: 500;
  color: #333;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: inline-block;
  max-width: 100%;
  vertical-align: middle;
}

.action-show-icon {
  font-size: 14px;
  padding: 0 2px 1px 2px;
  color: #07c05f;
}

.action-details {
  padding: 0;
  border-top: 1px solid #f0f0f0;
  background: #ffffff;
  display: flex;
  flex-direction: column;
}

.tool-result-wrapper {
  margin: 0;
}

.search-results-summary-fixed {
  padding: 8px 12px;
  background: #f8f9fa;
  border-top: 1px solid #e7e7e7;
  
  .results-summary-text {
    font-size: 13px;
    font-weight: 500;
    color: #333;
    line-height: 1.5;
    
    // Use :deep() to apply styles to v-html content
    :deep(strong) {
      color: #07c05f;
      font-weight: 600;
    }
  }
}

.plan-status-summary-fixed {
  padding: 8px 12px;
  background: #f8f9fa;
  border-top: 1px solid #e7e7e7;
  
  .plan-status-text {
    font-size: 13px;
    font-weight: 500;
    color: #333;
    line-height: 1.5;
    display: flex;
    align-items: center;
    gap: 4px;
    flex-wrap: wrap;
    
    .status-icon {
      font-size: 14px;
      flex-shrink: 0;
      
      &.in-progress {
        color: #07C05F;
      }
      
      &.pending {
        color: #fa8c16;
      }
      
      &.completed {
        color: #07C05F;
      }
    }
    
    .separator {
      color: #999;
      margin: 0 4px;
    }
    
    span:not(.separator) {
      display: inline-flex;
      align-items: center;
      gap: 4px;
    }
  }
}

@keyframes rotate {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

.plan-task-change-event {
  margin-bottom: 8px;
  
  .plan-task-change-card {
    padding: 8px 12px;
    background: #f8f9fa;
    border-radius: 4px;
    border-left: 3px solid #07C05F;
    font-size: 13px;
    color: #333;
    
    .plan-task-change-content {
      strong {
        color: #333;
        font-weight: 600;
        margin-right: 4px;
      }
    }
  }
}

.tool-output-wrapper {
  margin: 12px 0;
  padding: 0 10px;
  
  .fallback-header {
    display: flex;
    align-items: center;
    margin-bottom: 10px;
    padding: 0 4px;
    
    .fallback-label {
      font-size: 12px;
      color: #666;
      font-weight: 500;
      line-height: 1.5;
    }
  }
  
  .detail-output-wrapper {
    position: relative;
    background: #fafafa;
    border: 1px solid #e5e7eb;
    border-radius: 6px;
    overflow: hidden;
    margin: 0;
    padding: 0;
    
    .detail-output {
      font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', 'Courier New', monospace;
      font-size: 12px;
      color: #333;
      padding: 16px;
      margin: 0;
      white-space: pre-wrap;
      word-break: break-word;
      line-height: 1.6;
      max-height: 400px;
      overflow-y: auto;
      overflow-x: auto;
      background: #ffffff;
      display: block;
      
      // 滚动条样式
      &::-webkit-scrollbar {
        width: 8px;
        height: 8px;
      }
      
      &::-webkit-scrollbar-track {
        background: #f5f5f5;
        border-radius: 4px;
      }
      
      &::-webkit-scrollbar-thumb {
        background: #d1d5db;
        border-radius: 4px;
        
        &:hover {
          background: #9ca3af;
        }
      }
    }
  }
}

.thinking-thought-content {
  padding: 8px 12px;
  
  .thinking-thought-markdown {
    font-size: 14px;
    color: #333333;
    line-height: 1.6;
    
    :deep(p) {
      margin: 6px 0;
      line-height: 1.6;
      font-size: 14px;
      color: #333333;
      
      &:first-child {
        margin-top: 0;
      }
      
      &:last-child {
        margin-bottom: 0;
      }
    }
    
    :deep(code) {
      background: #f0f0f0;
      padding: 2px 6px;
      border-radius: 3px;
      font-family: 'Monaco', 'Courier New', monospace;
      font-size: 12px;
      color: #333;
    }
    
    :deep(pre) {
      background: #f5f5f5;
      padding: 8px;
      border-radius: 4px;
      overflow-x: auto;
      margin: 6px 0;
      
      code {
        background: none;
        padding: 0;
      }
    }
    
    :deep(ul), :deep(ol) {
      margin: 6px 0;
      padding-left: 24px;
    }
    
    :deep(li) {
      margin: 2px 0;
      line-height: 1.6;
    }
    
    :deep(blockquote) {
      border-left: 3px solid #07c05f;
      margin: 6px 0;
      color: #666;
      background: rgba(7, 192, 95, 0.05);
      padding: 6px 12px;
      border-radius: 4px;
    }
    
    :deep(h1), :deep(h2), :deep(h3), :deep(h4), :deep(h5), :deep(h6) {
      margin: 8px 0 4px 0;
      font-weight: 600;
      color: #333;
      
      &:first-child {
        margin-top: 0;
      }
    }
    
    :deep(a) {
      color: #07c05f;
      text-decoration: none;
      
      &:hover {
        text-decoration: underline;
      }
    }
    
    :deep(table) {
      border-collapse: collapse;
      margin: 6px 0;
      font-size: 12px;
      
      th, td {
        border: 1px solid #e5e7eb;
        padding: 6px 10px;
      }
      
      th {
        background: #f5f5f5;
        font-weight: 600;
      }
    }
  }
}

/* Global citation styles fallback to ensure rendering in any container */
:deep(.citation) {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border-radius: 10px;
  padding: 2px 4px;
  font-size: 11px;
  line-height: 1.4;
  background-clip: padding-box;
  margin: 0 4px;
}

:deep(.citation .citation-tip) {
  display: none;
}

:deep(.citation-web) {
  /* Align with app primary green scheme */
  background: #f0fdf4;           /* green-50 */
  color: #065f46;                /* green-800 */
  border: 1px solid #bbf7d0;     /* green-200 */
  cursor: pointer;
  white-space: nowrap;
  position: relative;
}

:deep(.citation-web:hover) {
  /* Subtle hover in green tone */
  background: #d1fae5;           /* green-100 */
  border-color: #86efac;         /* green-300 */
  color: #065f46;                /* keep readable on light bg */
}

/* Embedded tooltip bubble - hidden, use global floatPopup instead */
:deep(.citation-web .citation-tip) {
  display: none !important;
  pointer-events: none;
}


/* Citation icons */
:deep(.citation .citation-icon) {
  display: inline-block;
  width: 14px;
  height: 14px;
  margin-right: 0px;
  background-repeat: no-repeat;
  background-size: contain;
  background-position: center;
  flex-shrink: 0;
}

/* Web icon (globe) */
:deep(.citation .citation-icon.web) {
  background-image: url("../../../assets/img/websearch-globe-green.svg");
}

/* Knowledge base icon */
:deep(.citation .citation-icon.kb) {
  background-image: url("../../../assets/img/zhishiku-thin.svg");
}

.kb-float-popup {
  position: absolute;
  z-index: 10000;
  pointer-events: auto;
  background: #f9fafb;
  border-radius: 6px;
  border: none !important;
  box-shadow: 0 6px 18px rgba(0,0,0,0.2);
  padding: 12px 14px;
  color: #111827;
  line-height: 1.5;
  font-size: 12px;
  box-sizing: border-box;
  max-width: 520px;
}

.kb-float-popup .t-popup__content {
  display: flex;
  flex-direction: column;
  gap: 4px;
  border: none !important;
  padding: 0 !important;
  margin: 0 !important;
  background: transparent !important;
  box-shadow: none !important;
}

.kb-float-popup .tip-title {
  font-weight: 600;
  color: #07C05F;
}

.kb-float-popup .tip-url {
  word-break: break-word;
}

.kb-float-popup .tip-meta {
  margin-top: 1px;
  font-size: 11px;
  color: #6b7280;
}

.kb-float-popup .tip-loading {
  color: #6b7280;
  font-style: italic;
}

.kb-float-popup .tip-error {
  color: #dc2626;
  font-weight: 500;
}

.kb-float-popup .tip-content {
  border: none !important;
  padding: 0 !important;
  margin: 0 !important;
  background: transparent !important;
  box-shadow: none !important;
  max-height: 250px;
  overflow-y: auto;
  overflow-x: hidden;
}

/* KB citation styles - same green theme as web citations */
:deep(.citation.citation-kb) {
  /* Green theme - same as web citations */
  background: #f0fdf4;           /* green-50 */
  color: #065f46;                /* green-800 */
  border: 1px solid #bbf7d0;     /* green-200 */
  cursor: pointer;
  white-space: nowrap;
  position: relative;
  transition: all 0.2s ease;
}

:deep(.citation.citation-kb:hover) {
  /* Subtle hover in green tone */
  background: #d1fae5;           /* green-100 */
  border-color: #86efac;         /* green-300 */
  color: #065f46;                /* keep readable on light bg */
}

:deep(.citation.citation-kb:focus-visible) {
  outline: 2px solid #34d399;    /* green-400 */
  outline-offset: 2px;
}

/* KB citation tooltip styles (same as web citation) */
:deep(.citation.citation-kb .citation-tip) {
  display: none !important;
  pointer-events: none;
}

.tool-arguments-wrapper {
  margin-top: 8px;
  padding: 0 10px;
  margin-bottom: 8px;
  
  .arguments-header {
    margin-bottom: 6px;
    
    .arguments-label {
      font-size: 12px;
      font-weight: 600;
      color: #666;
      text-transform: uppercase;
      letter-spacing: 0.5px;
    }
  }
  
  .detail-code {
    font-size: 12px;
    background: #ffffff;
    padding: 10px;
    border-radius: 6px;
    font-family: 'Monaco', 'Courier New', monospace;
    color: #333;
    margin: 0;
    overflow-x: auto;
    border: 1px solid #e5e7eb;
    line-height: 1.5;
  }
}

.loading-indicator {
  display: flex;
  align-items: center;
  padding: 8px 0;
  margin-top: 8px;
  animation: fadeInUp 0.3s ease-out;
  
  .botanswer_loading_gif {
    width: 24px;
    height: 18px;
    margin-left: 0;
  }
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

</style>
