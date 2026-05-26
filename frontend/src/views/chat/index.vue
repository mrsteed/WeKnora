<template>
    <div class="chat" :class="{ 'is-embedded': effectiveEmbeddedMode, 'is-sidebar-collapsed': uiStore.sidebarCollapsed }">
        <div ref="scrollContainer" class="chat_scroll_box" @scroll="handleScroll">
            <div class="msg_list" :class="{ 'is-embedded': effectiveEmbeddedMode }">
                <!-- 消息列表骨架屏 -->
                <div v-if="historyLoading && messagesList.length === 0" class="msg-skeleton-list">
                    <div class="msg-skeleton msg-skeleton-user">
                        <t-skeleton animation="gradient" :row-col="[{ width: '45%', height: '36px', type: 'rect' }]" />
                    </div>
                    <div class="msg-skeleton msg-skeleton-bot">
                        <t-skeleton animation="gradient" :row-col="[{ width: '80%', height: '16px' }, { width: '100%', height: '16px' }, { width: '60%', height: '16px' }]" />
                    </div>
                    <div class="msg-skeleton msg-skeleton-user">
                        <t-skeleton animation="gradient" :row-col="[{ width: '35%', height: '36px', type: 'rect' }]" />
                    </div>
                    <div class="msg-skeleton msg-skeleton-bot">
                        <t-skeleton animation="gradient" :row-col="[{ width: '70%', height: '16px' }, { width: '90%', height: '16px' }]" />
                    </div>
                </div>
                <!-- 推荐问题卡片 - 仅在新会话（无消息）时展示 -->
                <div v-if="messagesList.length === 0 && !loading" class="suggested-questions-container" :class="{ 'has-questions': suggestedQuestions.length > 0 || suggestedQuestionsLoading }">
                    <!-- 骨架屏占位 -->
                    <div v-if="suggestedQuestionsLoading && suggestedQuestions.length === 0" class="suggested-questions-inner">
                        <div class="suggested-questions-title"><t-skeleton animation="gradient" :row-col="[{ width: '120px', height: '18px' }]" /></div>
                        <div class="suggested-questions-grid">
                            <div v-for="n in 6" :key="'sq-skel-'+n" class="suggested-question-card sq-card-skeleton">
                                <t-skeleton animation="gradient" :row-col="[{ width: '90%', height: '14px' }, { width: '60%', height: '14px' }]" />
                            </div>
                        </div>
                    </div>
                    <transition v-else appear name="sq-fade">
                        <div v-if="suggestedQuestions.length > 0" class="suggested-questions-inner">
                            <div class="suggested-questions-title">{{ t('chat.suggestedQuestions') }}</div>
                            <div class="suggested-questions-grid">
                                <div
                                    v-for="(item, index) in suggestedQuestions"
                                    :key="item.question"
                                    class="suggested-question-card"
                                    @click="handleSuggestedQuestionClick(item.question)"
                                >
                                    <span class="suggested-question-text">{{ item.question }}</span>
                                    <span v-if="item.source === 'faq'" class="suggested-question-badge faq">FAQ</span>
                                </div>
                            </div>
                        </div>
                    </transition>
                </div>
                <div v-for="(session, id) in messagesList" :key='id'>
                    <div v-if="session.role == 'user'">
                        <usermsg :content="session.content" :mentioned_items="session.mentioned_items" :images="session.images" :attachments="session.attachments" :embeddedMode="effectiveEmbeddedMode" :isAutoContinue="Boolean(session.is_auto_continue)"></usermsg>
                    </div>
                    <div v-if="session.role == 'assistant'">
                        <botmsg :content="session.content" :session="session" :user-query="getUserQuery(id)" @scroll-bottom="scrollToBottom" @retry="handleRetry"
                            :isFirstEnter="isFirstEnter" :embeddedMode="effectiveEmbeddedMode" :selectedArtifactId="manuallySelectedBaseArtifactId"
                            @view-artifact-revisions="openArtifactRevisionDrawer" @use-artifact-as-base="useArtifactAsBase" @clear-artifact-base="clearSelectedBaseArtifact"
                            @artifact-display-update="handleArtifactDisplayUpdate"></botmsg>
                    </div>
                </div>
                <div v-if="loading"
                    style="height: 41px;display: flex;align-items: center;padding-left: 4px;">
                    <div class="loading-typing">
                        <span></span>
                        <span></span>
                        <span></span>
                    </div>
                </div>
            </div>
        </div>
        <transition name="scroll-btn-fade">
            <div v-show="userHasScrolledUp" class="scroll-to-bottom-btn" @click="onClickScrollToBottom">
                <t-icon name="chevron-down" size="20px" />
            </div>
        </transition>
        <div class="input-container" :class="{ 'is-embedded': effectiveEmbeddedMode }">
            <div v-if="displayedBaseArtifact" class="document-baseline-banner">
                <div class="document-baseline-text">
                    <span class="document-baseline-label">{{ baselineBannerLabel }}</span>
                    <span class="document-baseline-title">{{ displayedBaseArtifact?.title || '未命名文档' }}</span>
                    <span class="document-baseline-version">V{{ displayedBaseArtifact?.revision_no || 1 }}</span>
                    <span v-if="newerArtifactAvailableForLockedBase" class="document-baseline-latest-hint">
                        已锁定手动基线，最新版本为 V{{ newerArtifactAvailableForLockedBase.revision_no || 1 }}
                    </span>
                </div>
                <t-button v-if="selectedBaseArtifactDisplayLocked" size="small" variant="text" theme="default" @click="clearSelectedBaseArtifact()">取消</t-button>
            </div>
            <div v-if="autoContinueState.enabled || autoContinueState.stoppedReason" class="auto-continue-banner" :class="{ 'is-stopped': !autoContinueState.enabled }">
                <span v-if="autoContinueState.enabled">
                    {{ autoContinueState.round > 0 ? `正在自动续写：第 ${autoContinueState.round} 轮，当前基线 V${displayedBaseArtifact?.revision_no || selectedBaseArtifact?.revision_no || 1}` : '已开启自动续写，等待当前轮完成' }}
                </span>
                <span v-else>自动续写已暂停：{{ autoContinueState.stoppedReason }}</span>
                <t-button v-if="autoContinueState.enabled" size="small" variant="text" theme="default" @click="stopAutoContinue('用户已停止自动续写')">不再续写</t-button>
            </div>
            <InputField
                ref="inputFieldRef"
                @send-msg="(query, modelId, mentionedItems, imageFiles, attachmentFiles) => sendMsg(query, modelId, mentionedItems, imageFiles, attachmentFiles)"
                @stop-generation="handleStopGeneration"
                @model-change="(modelId) => emit('model-change', modelId)"
                :isReplying="isReplying"
                :sessionId="session_id"
                :assistantMessageId="currentAssistantMessageId"
                :embeddedMode="effectiveEmbeddedMode"
                :runtimeContext="runtimeContext"
            ></InputField>
        </div>
    </div>
    <t-drawer v-model:visible="artifactRevisionDrawerVisible" header="文档版本链" size="520px" placement="right">
        <div class="artifact-drawer-body">
            <div v-if="artifactRevisionLoading" class="artifact-drawer-loading">
                <t-loading size="small" />
                <span>正在加载版本链...</span>
            </div>
            <div v-else-if="artifactRevisionList.length === 0" class="artifact-drawer-empty">
                暂无可展示的历史版本
            </div>
            <div v-else class="artifact-drawer-list">
                <div
                    v-for="artifact in artifactRevisionList"
                    :key="artifact.id"
                    class="artifact-drawer-item"
                    :class="{ 'is-selected': manuallySelectedBaseArtifactId === artifact.id, 'is-current': artifactRevisionAnchor?.id === artifact.id }"
                >
                    <div class="artifact-drawer-item-top">
                        <div class="artifact-drawer-item-title">{{ artifact.title || '未命名文档' }}</div>
                        <div class="artifact-drawer-item-tags">
                            <t-tag size="small" theme="primary" variant="light">V{{ artifact.revision_no || 1 }}</t-tag>
                            <t-tag size="small" :theme="getArtifactStatusTheme(artifact)" variant="light">
                                {{ getArtifactStatusText(artifact) }}
                            </t-tag>
                        </div>
                    </div>
                    <div class="artifact-drawer-item-meta">{{ artifact.operation || 'create' }} · {{ artifact.updated_at || artifact.created_at || '-' }}</div>
                    <div v-if="artifact.user_hint" class="artifact-drawer-item-hint">{{ artifact.user_hint }}</div>
                    <div class="artifact-drawer-item-actions">
                        <t-button size="small" variant="text" theme="primary" @click="useArtifactAsBase(artifact)">设为基线</t-button>
                    </div>
                </div>
            </div>
        </div>
    </t-drawer>
    <KnowledgeBaseEditorModal 
        :visible="uiStore.showKBEditorModal"
        :mode="uiStore.kbEditorMode"
        :kb-id="uiStore.currentKBId || undefined"
        :initial-type="uiStore.kbEditorType"
        @update:visible="(val) => val ? null : uiStore.closeKBEditor()"
        @success="handleKBEditorSuccess"
    />
</template>
<script setup>
import { storeToRefs } from 'pinia';
import { ref, computed, onMounted, onUnmounted, nextTick, watch, reactive, onBeforeUnmount, defineProps } from 'vue';
import { useRoute, useRouter, onBeforeRouteLeave, onBeforeRouteUpdate } from 'vue-router';
import InputField from '../../components/Input-field.vue';
import botmsg from './components/botmsg.vue';
import usermsg from './components/usermsg.vue';
import { getChatDocumentArtifact, getChatDocumentArtifactRevisions, getChatDocumentArtifacts, getLatestChatDocumentArtifact, getMessageList, generateSessionsTitle, getSession } from "@/api/chat/index";
import { getPublicAgentShareMessages } from '@/api/agent-share';
import { getSuggestedQuestions } from "@/api/agent/index";
import { useStream } from '../../api/chat/streame'
import { useMenuStore } from '@/stores/menu';
import { useSettingsStore } from '@/stores/settings';
import { MessagePlugin } from 'tdesign-vue-next';
import { useI18n } from 'vue-i18n';
import { useUIStore } from '@/stores/ui';
import KnowledgeBaseEditorModal from '@/views/knowledge/KnowledgeBaseEditorModal.vue';
import { useKnowledgeBaseCreationNavigation } from '@/hooks/useKnowledgeBaseCreationNavigation';
import { upsertThinkingEvent } from './utils/thinkingEvent';
import { extractStructuredPlanningOutlineFromText } from './utils/planningOutline';
import { createPlatformChatRuntimeContext, isAgentSharePageRuntimeContext } from '@/types/chat-runtime';

const props = defineProps({
  session_id: { type: String, default: '' },
  agentId: { type: String, default: '' },
  kbIds: { type: Array, default: () => [] },
  embeddedMode: { type: Boolean, default: false },
  runtimeContext: { type: Object, default: null }
});

const emit = defineEmits(['model-change']);

const defaultRuntimeContext = createPlatformChatRuntimeContext();
const runtimeContext = computed(() => props.runtimeContext || defaultRuntimeContext);
const isSharePageMode = computed(() => isAgentSharePageRuntimeContext(runtimeContext.value));
const effectiveEmbeddedMode = computed(() => props.embeddedMode);
const effectiveAgentId = computed(() => isSharePageMode.value ? (runtimeContext.value.fixedAgentId || '') : props.agentId);
const effectiveKBIds = computed(() => isSharePageMode.value ? (runtimeContext.value.fixedKnowledgeBaseIds || []) : props.kbIds);
const shareCode = computed(() => runtimeContext.value.shareCode || '');
const shareSessionToken = computed(() => runtimeContext.value.shareSessionToken || '');
const publicChatApiBase = computed(() => runtimeContext.value.publicChatApiBase || '');

const getEffectiveAgentEnabled = () => {
    if (isSharePageMode.value) {
        return runtimeContext.value.fixedAgentMode === 'smart-reasoning';
    }
    if (effectiveEmbeddedMode.value) {
        return Boolean(effectiveAgentId.value && effectiveAgentId.value !== 'builtin-quick-answer');
    }
    return useSettingsStoreInstance.isAgentEnabled;
};

const getEffectiveWebSearchEnabled = () => {
    if (isSharePageMode.value) {
        return Boolean(runtimeContext.value.webSearchEnabled);
    }
    if (effectiveEmbeddedMode.value) {
        return false;
    }
    return useSettingsStoreInstance.isWebSearchEnabled;
};

const getEffectiveMemoryEnabled = () => {
    if (isSharePageMode.value || effectiveEmbeddedMode.value) {
        return false;
    }
    return useSettingsStoreInstance.isMemoryEnabled;
};

const getEffectiveKnowledgeBaseIds = () => {
    if (isSharePageMode.value) {
        return [...effectiveKBIds.value];
    }
    if (effectiveEmbeddedMode.value) {
        return [...effectiveKBIds.value];
    }
    return [...(useSettingsStoreInstance.settings.selectedKnowledgeBases || [])];
};

const getEffectiveKnowledgeIds = () => {
    if (isSharePageMode.value || effectiveEmbeddedMode.value) {
        return [];
    }
    return [...(useSettingsStoreInstance.settings.selectedFiles || [])];
};

const getEffectiveMCPServiceIDs = () => {
    if (isSharePageMode.value || effectiveEmbeddedMode.value) {
        return [];
    }
    return [...(useSettingsStoreInstance.settings.selectedMCPServices || [])];
};

const getShareRequestHeaders = () => {
    if (!shareSessionToken.value) {
        return {};
    }
    return {
        'X-Share-Session-Token': shareSessionToken.value,
    };
};

const normalizeRuntimeSuggestedQuestions = () => {
    const source = runtimeContext.value?.suggestedQuestions || [];
    return source.map((item) => typeof item === 'string' ? { question: item } : item);
};

const syncRuntimeSuggestedQuestions = () => {
    suggestedQuestions.value = normalizeRuntimeSuggestedQuestions();
    suggestedQuestionsLoading.value = false;
};

const buildContinueStreamRequest = (targetSessionId, messageId) => {
    if (isSharePageMode.value) {
        return {
            session_id: targetSessionId,
            query: messageId,
            method: 'GET',
            url: `${publicChatApiBase.value}/chat/continue`,
            appendSessionIdToUrl: false,
            requireAuth: false,
            headers: getShareRequestHeaders(),
        };
    }
    return {
        session_id: targetSessionId,
        query: messageId,
        method: 'GET',
        url: '/api/v1/sessions/continue-stream',
    };
};

const applyRuntimeChatRequest = (request) => {
    if (!isSharePageMode.value) {
        return request;
    }
    return {
        ...request,
        url: `${publicChatApiBase.value}/chat`,
        appendSessionIdToUrl: false,
        includeSessionIdInBody: true,
        requireAuth: false,
        headers: getShareRequestHeaders(),
    };
};

const loadMessagesForRuntime = (data) => {
    if (isSharePageMode.value) {
        return getPublicAgentShareMessages(shareCode.value, data.session_id, shareSessionToken.value, {
            beforeTime: data.created_at || undefined,
            limit: data.limit,
        });
    }
    return getMessageList(data);
};

const usemenuStore = useMenuStore();
const useSettingsStoreInstance = useSettingsStore();
const uiStore = useUIStore();
const { navigateToKnowledgeBaseList } = useKnowledgeBaseCreationNavigation();
const { t } = useI18n();
const { menuArr, isFirstSession, firstQuery, firstMentionedItems, firstModelId, firstImageFiles, firstAttachmentFiles } = storeToRefs(usemenuStore);
const { output, onChunk, onClose, isStreaming, isLoading, error, startStream, stopStream } = useStream();
const route = useRoute();
const router = useRouter();
const session_id = ref(props.session_id || route.params.chatid);
const sessionData = ref(null);
const inputFieldRef = ref();
const created_at = ref('');
const limit = ref(20);
const messagesList = reactive([]);
const isReplying = ref(false);
const currentAssistantMessageId = ref(''); // 当前正在生成的 assistant message ID
const scrollLock = ref(false);
const isNeedTitle = ref(false);
const isFirstEnter = ref(true);
const loading = ref(false);
const historyLoading = ref(true);
let fullContent = ref('')
let userquery = ref('')
const lastRequestMeta = ref(null);
const chatDocumentArtifacts = ref([]);
const selectedBaseArtifact = ref(null);
const selectedBaseArtifactDisplayLocked = ref(false);
const artifactDisplayHint = ref(null);
const latestArtifactForDisplay = ref(null);
const artifactRevisionDrawerVisible = ref(false);
const artifactRevisionLoading = ref(false);
const artifactRevisionList = ref([]);
const artifactRevisionAnchor = ref(null);
const scrollContainer = ref(null)
const userHasScrolledUp = ref(false)
const suppressNextStreamCloseFinalize = ref(false)
const SCROLL_BOTTOM_THRESHOLD = 80
const AUTO_CONTINUE_PROMPT = '以当前文档为基准，继续剩余内容输出';
const DOCUMENT_COMPLETE_MARKER = '<!-- document_complete -->';
const autoContinueState = ref({
    enabled: false,
    rootId: '',
    round: 0,
    prompt: AUTO_CONTINUE_PROMPT,
    generationRunId: '',
    effectiveKBIDs: [],
    outline: null,
    originalRequest: null,
    stoppedReason: ''
});

const isNearBottom = () => {
    if (!scrollContainer.value) return true;
    const { scrollTop, scrollHeight, clientHeight } = scrollContainer.value;
    return scrollHeight - scrollTop - clientHeight < SCROLL_BOTTOM_THRESHOLD;
}

const handleKBEditorSuccess = (payload) => {
    navigateToKnowledgeBaseList(typeof payload === 'string' ? payload : payload.id)
}

// ===== 推荐问题 =====
const suggestedQuestions = ref([]);
const suggestedQuestionsLoading = ref(false);
let suggestedQuestionsFetchId = 0; // 用于取消过时的请求
let suggestedDebounceTimer = null;

const fetchSuggestedQuestions = async () => {
    if (isSharePageMode.value) {
        syncRuntimeSuggestedQuestions();
        return;
    }
    const fetchId = ++suggestedQuestionsFetchId;
    suggestedQuestionsLoading.value = true;
    // 加载期间保留旧数据，不清空，避免布局抖动
    try {
        const agentId = effectiveEmbeddedMode.value ? effectiveAgentId.value : useSettingsStoreInstance.selectedAgentId;
        if (!agentId) return;
        const selectedKBs = effectiveEmbeddedMode.value ? effectiveKBIds.value : useSettingsStoreInstance.getSelectedKnowledgeBases();
        const selectedFiles = effectiveEmbeddedMode.value ? [] : useSettingsStoreInstance.getSelectedFiles();
        const res = await getSuggestedQuestions(agentId, {
            knowledge_base_ids: selectedKBs.length > 0 ? selectedKBs : undefined,
            knowledge_ids: selectedFiles.length > 0 ? selectedFiles : undefined,
            limit: 6,
        });
        if (fetchId === suggestedQuestionsFetchId) {
            suggestedQuestions.value = res?.data?.questions || [];
        }
    } catch (err) {
        console.warn('[SuggestedQuestions] Failed to fetch:', err);
        if (fetchId === suggestedQuestionsFetchId) {
            suggestedQuestions.value = [];
        }
    } finally {
        if (fetchId === suggestedQuestionsFetchId) {
            suggestedQuestionsLoading.value = false;
        }
    }
};

const handleSuggestedQuestionClick = (question) => {
    if (inputFieldRef.value?.triggerSend) {
        inputFieldRef.value.triggerSend(question);
    } else {
        sendMsg(question);
    }
};

// 防抖包装，切换知识库/文件时300ms内不重复请求
const debouncedFetchSuggestions = () => {
    if (isSharePageMode.value) {
        syncRuntimeSuggestedQuestions();
        return;
    }
    if (suggestedDebounceTimer) clearTimeout(suggestedDebounceTimer);
    suggestedDebounceTimer = setTimeout(() => { fetchSuggestedQuestions(); }, 300);
};

// 监听 Agent / 知识库 / 文件切换，重新获取推荐问题
watch(
    () => useSettingsStoreInstance.selectedAgentId,
    debouncedFetchSuggestions,
);
watch(
    () => useSettingsStoreInstance.settings.selectedKnowledgeBases,
    debouncedFetchSuggestions,
    { deep: true },
);
watch(
    () => useSettingsStoreInstance.settings.selectedFiles,
    debouncedFetchSuggestions,
    { deep: true },
);
watch(
    () => runtimeContext.value?.suggestedQuestions,
    () => {
        if (isSharePageMode.value) {
            syncRuntimeSuggestedQuestions();
        }
    },
    { deep: true },
);

function fileToBase64(file) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(reader.result);
        reader.onerror = reject;
        reader.readAsDataURL(file);
    });
}

const getUserQuery = (index) => {
    if (index <= 0) {
        return '';
    }
    const previous = messagesList[index - 1];
    if (previous && previous.role === 'user') {
        return previous.content || '';
    }
    return '';
};

const terminalCompletionStatuses = new Set(['completed', 'partial', 'failed', 'cancelled']);
const internalFailureReasonMessages = {
    stream_unavailable: () => t('chat.streamUnavailable'),
};

const normalizeFailureMessage = (reason) => {
    const normalizedReason = typeof reason === 'string' ? reason.trim() : '';
    return internalFailureReasonMessages[normalizedReason]?.() || normalizedReason;
};

const resumableDocumentStopReasons = new Set([
    'llm_timeout',
    'llm_timeout_retry_exhausted',
    'section_generation_timeout',
    'section_generation_error',
    'section_generation_truncated',
]);

const normalizeCompletionStatus = (message = {}) => {
    if (message?.data?.completion_status) {
        return message.data.completion_status;
    }
    if (message?.completion_status) {
        return message.completion_status;
    }
    if (message?.response_type === 'stop') {
        return 'cancelled';
    }
    if (message?.is_failed) {
        return 'failed';
    }
    if (message?.is_completed) {
        return 'completed';
    }
    if (message?.role === 'assistant' || message?.isAgentMode) {
        return 'pending';
    }
    return 'completed';
};

const isTerminalCompletionStatus = (completionStatus) => terminalCompletionStatuses.has(completionStatus || '');

const isRecoveredAgentCompletion = (completionStatus, finishReason) => {
    return finishReason === 'fallback_stop' && completionStatus !== 'failed';
};

const syncMessageCompletionState = (message, payload = {}) => {
    if (!message) {
        return 'pending';
    }

    const completePayload = payload?.data || payload;
    const nextStatus = normalizeCompletionStatus({ ...message, ...payload, ...completePayload });
    message.completion_status = nextStatus;

    const finishReason = completePayload?.finish_reason;
    if (finishReason !== undefined) {
        message.finish_reason = finishReason;
    }

    const failureReason = completePayload?.failure_reason;
    if (failureReason !== undefined) {
        message.failure_reason = failureReason;
    }

    if (completePayload?.document_generation_status !== undefined) {
        message.document_generation_status = completePayload.document_generation_status;
    }
    if (completePayload?.auto_continue_next !== undefined) {
        message.auto_continue_next = completePayload.auto_continue_next;
    }
    if (completePayload?.auto_continue_reason !== undefined) {
        message.auto_continue_reason = completePayload.auto_continue_reason;
    }
    if (completePayload?.generation_run_id !== undefined) {
        message.generation_run_id = completePayload.generation_run_id;
    }
    if (completePayload?.translation_progress !== undefined) {
        message.translation_progress = completePayload.translation_progress;
    }
    if (completePayload?.document_task_kind !== undefined) {
        message.document_task_kind = completePayload.document_task_kind;
    }
    if (Array.isArray(completePayload?.effective_kb_ids)) {
        message.effective_kb_ids = [...completePayload.effective_kb_ids];
    }

    message.is_partial = nextStatus === 'partial' || Boolean(completePayload?.is_partial);
    message.is_completed = nextStatus === 'completed';
    message.is_failed = nextStatus === 'failed';
    message.is_recovered = isRecoveredAgentCompletion(nextStatus, message.finish_reason);

    if (message.is_failed && !message.error_message && message.failure_reason) {
        message.error_message = normalizeFailureMessage(message.failure_reason);
    } else if (!message.is_failed) {
        message.error_message = '';
        if (message.is_recovered && message.failure_reason === 'tool_error') {
            message.failure_reason = '';
        }
    }

    rehydrateHistoricalDocumentContinuationState(message);

    return nextStatus;
};

const rehydrateHistoricalDocumentContinuationState = (message = {}) => {
    if (!message || message.role !== 'assistant') {
        return;
    }

    const artifact = message.chat_document_artifact;
    if (artifact?.document_generation_status && !message.document_generation_status) {
        message.document_generation_status = artifact.document_generation_status;
    }
    if (!canContinueChatDocumentArtifact(artifact)) {
        return;
    }

    const completionStatus = message.completion_status || normalizeCompletionStatus(message);
    const documentGenerationStatus = typeof (message.document_generation_status || artifact?.document_generation_status) === 'string'
        ? (message.document_generation_status || artifact?.document_generation_status).trim()
        : '';
    const finishReason = typeof message.finish_reason === 'string' ? message.finish_reason.trim() : '';
    const failureReason = typeof message.failure_reason === 'string' ? message.failure_reason.trim() : '';
    const stopReason = finishReason === 'llm_timeout_retry_exhausted'
        ? finishReason
        : (failureReason || finishReason);

    const isResumableInterruptedDocument = completionStatus === 'failed'
        || (
            completionStatus === 'partial'
            && documentGenerationStatus === 'continuing'
            && resumableDocumentStopReasons.has(stopReason)
        );

    if (!isResumableInterruptedDocument) {
        return;
    }

    if (message.auto_continue_next === undefined) {
        message.auto_continue_next = false;
    }
    if (!message.auto_continue_reason && stopReason) {
        message.auto_continue_reason = stopReason;
    }
};

const upsertAgentCompleteEvent = (message, payload = {}) => {
    if (!message) {
        return;
    }
    if (!message.agentEventStream) {
        message.agentEventStream = [];
    }

    const completePayload = payload?.data || payload;
    const completeExtra = completePayload?.extra && typeof completePayload.extra === 'object'
        ? completePayload.extra
        : {};
    const completionStatus = normalizeCompletionStatus({ ...message, ...payload, ...completePayload });
    const nextEvent = {
        type: 'agent_complete',
        total_duration_ms: completePayload?.total_duration_ms,
        total_steps: completePayload?.total_steps,
        completion_status: completionStatus,
        finish_reason: completePayload?.finish_reason,
        failure_reason: completePayload?.failure_reason,
        document_generation_status: completePayload?.document_generation_status,
        auto_continue_next: completePayload?.auto_continue_next,
        auto_continue_reason: completePayload?.auto_continue_reason,
        generation_run_id: completePayload?.generation_run_id,
        translation_progress: completePayload?.translation_progress && typeof completePayload.translation_progress === 'object'
            ? completePayload.translation_progress
            : null,
        is_partial: completionStatus === 'partial' || Boolean(completePayload?.is_partial),
        final_answer: completePayload?.final_answer,
        outline: completePayload?.outline,
        outline_role: typeof completePayload?.outline_role === 'string' ? completePayload.outline_role : '',
        outline_source: typeof completePayload?.outline_source === 'string' ? completePayload.outline_source : '',
        base_outline: completePayload?.base_outline && typeof completePayload.base_outline === 'object' ? completePayload.base_outline : null,
        planning_outline: completePayload?.planning_outline && typeof completePayload.planning_outline === 'object' ? completePayload.planning_outline : null,
        quality_issues: Array.isArray(completePayload?.quality_issues)
            ? [...completePayload.quality_issues]
            : (Array.isArray(completeExtra?.quality_issues) ? [...completeExtra.quality_issues] : []),
        quality_issue_details: Array.isArray(completePayload?.quality_issue_details)
            ? [...completePayload.quality_issue_details]
            : (Array.isArray(completeExtra?.quality_issue_details) ? [...completeExtra.quality_issue_details] : []),
        document_patch_metadata: completePayload?.document_patch_metadata && typeof completePayload.document_patch_metadata === 'object'
            ? completePayload.document_patch_metadata
            : (completeExtra?.document_patch_metadata && typeof completeExtra.document_patch_metadata === 'object'
                ? completeExtra.document_patch_metadata
                : null),
        chat_document_artifact: completePayload?.chat_document_artifact && typeof completePayload.chat_document_artifact === 'object'
            ? (normalizeChatDocumentArtifact(completePayload.chat_document_artifact) || completePayload.chat_document_artifact)
            : null,
    };

    const existingCompleteEvent = message.agentEventStream.find((event) => event.type === 'agent_complete');
    if (existingCompleteEvent) {
        Object.assign(existingCompleteEvent, nextEvent);
        return;
    }

    message.agentEventStream.push(nextEvent);
};

const isMessageTerminal = (message) => isTerminalCompletionStatus(normalizeCompletionStatus(message));

const findMessageByStreamId = (streamId) => {
    if (!streamId) {
        return null;
    }
    return messagesList.findLast((item) => item.request_id === streamId || item.id === streamId) || null;
};

const getRecoverableAssistantMessages = (items = []) => {
    return items.filter((message) => message?.role === 'assistant' && !isMessageTerminal(message));
};

const isHistoricalAgentMessage = (message) => {
    if (!message || message.role !== 'assistant') {
        return false;
    }

    if (Array.isArray(message.agent_steps) && message.agent_steps.length > 0) {
        return true;
    }

    if (Number(message.agent_duration_ms || 0) > 0) {
        return true;
    }

    if (typeof message.finish_reason === 'string' && message.finish_reason.trim() === 'tool_calls') {
        return true;
    }

    if (Array.isArray(message.agentEventStream) && message.agentEventStream.length > 0) {
        return true;
    }

    return false;
};

const recoverHistoricalAssistantMessages = async (items = []) => {
    const recoverableMessages = getRecoverableAssistantMessages(items);
    if (!recoverableMessages.length) {
        return;
    }

    const previousAssistantMessageId = currentAssistantMessageId.value;
    const previousReplying = isReplying.value;
    const previousLoading = loading.value;

    try {
        for (const message of recoverableMessages) {
            if (!message?.id || isMessageTerminal(message)) {
                continue;
            }
            await startStream(buildContinueStreamRequest(session_id.value, message.id));
        }
    } finally {
        currentAssistantMessageId.value = previousAssistantMessageId;
        isReplying.value = previousReplying;
        loading.value = previousLoading;
    }
};

const buildLocalAssistantMessage = (isAgentMode = false) => {
    const localId = `local-assistant-${Date.now()}`;
    return {
        id: localId,
        request_id: localId,
        role: 'assistant',
        content: '',
        isAgentMode,
        completion_status: 'pending',
        finish_reason: '',
        failure_reason: '',
        is_completed: false,
        is_failed: false,
        error_message: '',
        agentEventStream: [],
        _eventMap: new Map(),
        _pendingToolCalls: new Map(),
        knowledge_references: []
    };
};

const normalizeChatDocumentArtifact = (artifact) => {
    if (!artifact || typeof artifact !== 'object' || !artifact.id) {
        return null;
    }
    return {
        ...artifact,
        revision_no: Number(artifact.revision_no || 1),
    };
};

const compareArtifactRevision = (left, right) => {
    const revisionDiff = Number(right?.revision_no || 0) - Number(left?.revision_no || 0);
    if (revisionDiff !== 0) {
        return revisionDiff;
    }
    const rightTime = new Date(right?.updated_at || right?.created_at || 0).getTime();
    const leftTime = new Date(left?.updated_at || left?.created_at || 0).getTime();
    return rightTime - leftTime;
};

const findLatestSessionArtifact = (artifacts = chatDocumentArtifacts.value, targetSessionId = session_id.value) => {
    return (artifacts || [])
        .filter((item) => item?.session_id === targetSessionId)
        .sort(compareArtifactRevision)[0] || null;
};

const refreshLatestArtifactForDisplay = (artifacts = chatDocumentArtifacts.value, targetSessionId = session_id.value) => {
    latestArtifactForDisplay.value = findLatestSessionArtifact(artifacts, targetSessionId);
    return latestArtifactForDisplay.value;
};

const displayedBaseArtifact = computed(() => {
    return selectedBaseArtifact.value || latestArtifactForDisplay.value;
});

const manuallySelectedBaseArtifactId = computed(() => {
    if (!selectedBaseArtifactDisplayLocked.value) {
        return '';
    }
    return selectedBaseArtifact.value?.id || '';
});

const baselineBannerLabel = computed(() => {
    return selectedBaseArtifactDisplayLocked.value ? '当前基线' : '自动基线';
});

const newerArtifactAvailableForLockedBase = computed(() => {
    if (!selectedBaseArtifactDisplayLocked.value || !selectedBaseArtifact.value?.id) {
        return null;
    }
    const latestArtifact = latestArtifactForDisplay.value;
    if (!latestArtifact?.id || latestArtifact.id === selectedBaseArtifact.value.id) {
        return null;
    }
    if (Number(latestArtifact.revision_no || 0) <= Number(selectedBaseArtifact.value.revision_no || 0)) {
        return null;
    }
    return latestArtifact;
});

const applyChatDocumentArtifactsToMessages = (artifacts = chatDocumentArtifacts.value) => {
    const artifactByMessageId = new Map();
    for (const artifact of artifacts || []) {
        if (artifact?.source_message_id) {
            artifactByMessageId.set(artifact.source_message_id, artifact);
        }
    }
    messagesList.forEach((message) => {
        if (message?.role !== 'assistant') {
            return;
        }
        const artifact = artifactByMessageId.get(message.id);
        if (artifact) {
            message.chat_document_artifact = artifact;
            rehydrateHistoricalDocumentContinuationState(message);
        }
    });
};

const upsertChatDocumentArtifact = (artifact) => {
    const normalized = normalizeChatDocumentArtifact(artifact);
    if (!normalized) {
        return null;
    }
    const existingIndex = chatDocumentArtifacts.value.findIndex((item) => item.id === normalized.id);
    if (existingIndex >= 0) {
        chatDocumentArtifacts.value[existingIndex] = {
            ...chatDocumentArtifacts.value[existingIndex],
            ...normalized,
        };
    } else {
        chatDocumentArtifacts.value = [normalized, ...chatDocumentArtifacts.value];
    }
    const merged = chatDocumentArtifacts.value.find((item) => item.id === normalized.id) || normalized;
    if (selectedBaseArtifact.value?.id === merged.id) {
        selectedBaseArtifact.value = merged;
    }
    refreshLatestArtifactForDisplay();
    applyChatDocumentArtifactsToMessages([merged]);
    return merged;
};

const assignChatDocumentArtifactToMessage = (message, artifact) => {
    const normalized = upsertChatDocumentArtifact(artifact);
    if (message && normalized) {
        message.chat_document_artifact = normalized;
        rehydrateHistoricalDocumentContinuationState(message);
    }
    return normalized;
};

const isContinuableArtifactStatus = (artifact = {}) => {
    return artifact?.status === 'available' || artifact?.status === 'partial';
};

const canContinueChatDocumentArtifact = (artifact = {}) => {
    if (!isContinuableArtifactStatus(artifact)) {
        return false;
    }
    const generationStatus = typeof artifact?.document_generation_status === 'string'
        ? artifact.document_generation_status.trim()
        : '';
    if (generationStatus === 'needs_review' || generationStatus === 'blocked') {
        return false;
    }
    if (artifact?.can_continue !== undefined) {
        return artifact.can_continue !== false;
    }
    return artifact?.can_inline_continue !== false;
};

const canManualContinueChatDocumentArtifact = (artifact = {}) => {
    if (!isContinuableArtifactStatus(artifact)) {
        return false;
    }
    if (artifact?.can_manual_continue !== undefined) {
        return artifact.can_manual_continue !== false;
    }
    return canContinueChatDocumentArtifact(artifact);
};

const canManualReviseChatDocumentArtifact = (artifact = {}) => {
    if (!artifact || typeof artifact !== 'object') {
        return false;
    }
    if (artifact?.can_manual_revise !== undefined) {
        return artifact.can_manual_revise !== false;
    }
    return canUseChatDocumentArtifactAsBase(artifact);
};

const canUseChatDocumentArtifactAsBase = (artifact = {}) => {
    if (!artifact || typeof artifact !== 'object') {
        return false;
    }
    if (artifact?.can_use_as_base !== undefined) {
        return artifact.can_use_as_base !== false;
    }
    return canContinueChatDocumentArtifact(artifact);
};

const canUseChatDocumentArtifactForIntent = (artifact = {}, intentHint = 'normal') => {
    if (intentHint === 'continue_document') {
        if (artifact?.can_manual_continue !== undefined) {
            return artifact.can_manual_continue !== false;
        }
        return canContinueChatDocumentArtifact(artifact);
    }
    if (intentHint === 'revise_document') {
        if (artifact?.can_manual_revise !== undefined) {
            return artifact.can_manual_revise !== false;
        }
        return canUseChatDocumentArtifactAsBase(artifact);
    }
    return canUseChatDocumentArtifactAsBase(artifact);
};

const getArtifactStatusTheme = (artifact = {}) => {
    const generationStatus = typeof artifact?.document_generation_status === 'string'
        ? artifact.document_generation_status.trim()
        : '';
    if (generationStatus === 'needs_review') {
        return 'warning';
    }
    if (generationStatus === 'blocked') {
        return 'danger';
    }
    if (artifact?.status === 'available') {
        return 'success';
    }
    if (artifact?.status === 'partial') {
        return 'warning';
    }
    if (artifact?.status === 'failed') {
        return 'danger';
    }
    return 'default';
};

const getArtifactStatusText = (artifact = {}) => {
    const generationStatus = typeof artifact?.document_generation_status === 'string'
        ? artifact.document_generation_status.trim()
        : '';
    if (generationStatus === 'needs_review') {
        return '待复核';
    }
    if (generationStatus === 'blocked') {
        return '已阻断';
    }
    if (canManualContinueChatDocumentArtifact(artifact)) {
        return '可继续';
    }
    if (canManualReviseChatDocumentArtifact(artifact)) {
        return '可修订';
    }
    if (artifact?.can_view !== undefined && artifact.can_view !== false) {
        return '可查看';
    }
    if (artifact?.status === 'available') {
        return '已完成';
    }
    if (artifact?.status === 'partial') {
        return '部分完成';
    }
    if (artifact?.status === 'failed') {
        return '失败';
    }
    return '未知';
};

const findLatestContinuableArtifact = (artifacts = chatDocumentArtifacts.value, targetSessionId = session_id.value) => {
    return (artifacts || [])
        .filter((item) => item?.session_id === targetSessionId && canContinueChatDocumentArtifact(item))
        .sort(compareArtifactRevision)[0] || null;
};

const findLatestBaseUsableArtifact = (artifacts = chatDocumentArtifacts.value, targetSessionId = session_id.value) => {
    return (artifacts || [])
        .filter((item) => item?.session_id === targetSessionId && canUseChatDocumentArtifactAsBase(item))
        .sort(compareArtifactRevision)[0] || null;
};

const shouldAutoSelectCompletedArtifact = (message, artifact) => {
    if (!message || !artifact?.id || !canUseChatDocumentArtifactAsBase(artifact)) {
        return false;
    }
    if (artifact.source_message_id && message.id) {
        return artifact.source_message_id === message.id;
    }
    return true;
};

const promoteCompletedArtifactAsBase = (message, artifact) => {
    const normalized = assignChatDocumentArtifactToMessage(message, artifact);
    if (shouldAutoSelectCompletedArtifact(message, normalized)) {
        if (selectedBaseArtifactDisplayLocked.value && selectedBaseArtifact.value?.id && selectedBaseArtifact.value.id !== normalized?.id) {
            artifactDisplayHint.value = normalized;
            return normalized;
        }
        selectedBaseArtifact.value = normalized;
        selectedBaseArtifactDisplayLocked.value = false;
    }
    return normalized;
};

const resolveMessageForArtifactDisplay = (payload = {}) => {
    const messageId = typeof payload?.messageId === 'string' ? payload.messageId : '';
    const requestId = typeof payload?.requestId === 'string' ? payload.requestId : '';
    if (!messageId && !requestId) {
        return null;
    }
    return messagesList.findLast((item) => {
        if (!item || item.role !== 'assistant') {
            return false;
        }
        if (messageId && item.id === messageId) {
            return true;
        }
        return Boolean(requestId) && item.request_id === requestId;
    }) || null;
};

const ensureMessageFinalDocumentContent = async (message, artifactHint = null) => {
    if (!message) {
        return null;
    }
    const cachedContent = typeof message.final_document_content === 'string' ? message.final_document_content.trim() : '';
    if (cachedContent) {
        return message.chat_document_artifact || normalizeChatDocumentArtifact(artifactHint);
    }

    const artifactId = artifactHint?.id || message.chat_document_artifact?.id;
    if (!artifactId) {
        return null;
    }

    const res = await getChatDocumentArtifact(artifactId);
    const artifact = normalizeChatDocumentArtifact(res?.data);
    if (!artifact?.id) {
        return null;
    }

    const normalized = assignChatDocumentArtifactToMessage(message, artifact);
    const snapshot = typeof normalized?.content_snapshot === 'string' ? normalized.content_snapshot.trim() : '';
    if (snapshot) {
        message.final_document_content = snapshot;
    }
    return normalized;
};

const hydrateFinalDocumentFromComplete = async (message, payload = {}) => {
    if (!message) {
        return null;
    }

    if (payload.final_document_mode === 'inline_snapshot' && payload.final_document) {
        message.final_document_content = payload.final_document;
        if (payload.chat_document_artifact?.id) {
            return promoteCompletedArtifactAsBase(message, {
                ...payload.chat_document_artifact,
                content_snapshot: payload.final_document,
            });
        }
        return message.chat_document_artifact || null;
    }

    if (payload.final_document_mode !== 'fetch_artifact_snapshot') {
        return message.chat_document_artifact || null;
    }

    const artifactId = payload.final_document_artifact_id || payload.chat_document_artifact?.id;
    if (!artifactId) {
        return message.chat_document_artifact || null;
    }

    try {
        const res = await getChatDocumentArtifact(artifactId);
        const artifact = res?.data;
        if (!artifact?.id) {
            return message.chat_document_artifact || null;
        }
        const normalized = promoteCompletedArtifactAsBase(message, artifact);
        if (normalized?.content_snapshot) {
            message.final_document_content = normalized.content_snapshot;
        }
        return normalized || null;
    } catch (error) {
        console.warn('[ChatDocumentArtifact] Failed to hydrate final document snapshot:', error);
        return message.chat_document_artifact || null;
    }
};

const buildAutoContinueRootId = () => {
    return `auto-doc-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
};

const cloneAutoContinueOriginalRequest = (requestParams) => ({
    ...requestParams,
    images: undefined,
    attachment_uploads: undefined,
    document_target_heading: undefined,
    document_merge_mode: undefined,
    auto_continue: undefined,
    generation_run_id: undefined,
    auto_continue_root_id: undefined,
    auto_continue_round: undefined,
    auto_continue_prompt: undefined,
    auto_continue_original_query: undefined,
});

const getMessageTranslationProgress = (message = {}) => {
    if (message?.translation_progress && typeof message.translation_progress === 'object') {
        return message.translation_progress;
    }
    if (!Array.isArray(message?.agentEventStream)) {
        return null;
    }
    const completeEvent = message.agentEventStream.find((event) => event?.type === 'agent_complete' && event?.translation_progress && typeof event.translation_progress === 'object');
    return completeEvent?.translation_progress || null;
};

const getMessageGenerationRunId = (message = {}) => {
    if (typeof message?.generation_run_id === 'string' && message.generation_run_id) {
        return message.generation_run_id;
    }
    if (!Array.isArray(message?.agentEventStream)) {
        return '';
    }
    const completeEvent = message.agentEventStream.find((event) => event?.type === 'agent_complete' && typeof event?.generation_run_id === 'string' && event.generation_run_id);
    return completeEvent?.generation_run_id || '';
};

const getMessageAutoContinueReason = (message = {}) => {
    if (typeof message?.auto_continue_reason === 'string' && message.auto_continue_reason) {
        return message.auto_continue_reason;
    }
    if (!Array.isArray(message?.agentEventStream)) {
        return '';
    }
    const completeEvent = message.agentEventStream.find((event) => event?.type === 'agent_complete' && typeof event?.auto_continue_reason === 'string' && event.auto_continue_reason);
    return completeEvent?.auto_continue_reason || '';
};

const isTranslationContinuationRequest = (requestParams = {}) => {
    return requestParams?.document_task_kind === 'translation';
};

const startAutoContinueFlow = (requestParams) => {
    autoContinueState.value = {
        enabled: true,
        rootId: buildAutoContinueRootId(),
        round: 0,
        prompt: AUTO_CONTINUE_PROMPT,
        generationRunId: '',
        effectiveKBIDs: [],
        outline: null,
        originalRequest: cloneAutoContinueOriginalRequest(requestParams),
        stoppedReason: ''
    };
};

const stopAutoContinue = (reason = '自动续写已停止') => {
    autoContinueState.value = {
        ...autoContinueState.value,
        enabled: false,
        stoppedReason: reason
    };
};

const resetAutoContinue = () => {
    autoContinueState.value = {
        enabled: false,
        rootId: '',
        round: 0,
        prompt: AUTO_CONTINUE_PROMPT,
        generationRunId: '',
        effectiveKBIDs: [],
        outline: null,
        originalRequest: null,
        stoppedReason: ''
    };
};

const stripDocumentCompleteMarker = (content = '') => {
    return typeof content === 'string'
        ? content.split(DOCUMENT_COMPLETE_MARKER).join('').trim()
        : content;
};

const isContinuingDocumentGenerationPayload = (payload = {}) => {
    if (typeof payload?.next_action === 'string' && payload.next_action) {
        return payload.next_action === 'continue_auto';
    }
    const generationStatus = payload?.document_generation_status;
    return payload?.auto_continue_next === true ||
        (generationStatus === 'continuing' && payload?.finish_reason === 'section_batch_limit');
};

const getDocumentNextReason = (payload = {}) => payload?.next_reason || payload?.auto_continue_reason || '';

const getDocumentNextReasonMessage = (payload = {}, fallback = '文档已完成') => {
    const backendMessage = typeof payload?.next_reason_message === 'string' && payload.next_reason_message.trim()
        ? payload.next_reason_message.trim()
        : typeof payload?.auto_continue_reason_message === 'string' && payload.auto_continue_reason_message.trim()
            ? payload.auto_continue_reason_message.trim()
            : '';
    return backendMessage || getDocumentNextReason(payload) || fallback;
};

const normalizeRecommendedAutoContinueRequest = (request = {}) => {
    if (!request || typeof request !== 'object') {
        return {};
    }
    const normalized = {};
    for (const key of [
        'query',
        'intent_hint',
        'base_artifact_id',
        'document_output_mode',
        'document_task_kind',
        'generation_run_id',
        'auto_continue_prompt',
    ]) {
        if (typeof request[key] === 'string' && request[key].trim()) {
            normalized[key] = request[key].trim();
        }
    }
    if (typeof request.auto_continue === 'boolean') {
        normalized.auto_continue = request.auto_continue;
    }
    if (Number.isFinite(Number(request.auto_continue_round))) {
        normalized.auto_continue_round = Number(request.auto_continue_round);
    }
    return normalized;
};

const shouldImplicitlyStartFullDocumentAutoContinue = (payload = {}) => {
    if (autoContinueState.value.enabled || autoContinueState.value.stoppedReason) {
        return false;
    }
    if (!isContinuingDocumentGenerationPayload(payload)) {
        return false;
    }
    const baseRequest = lastRequestMeta.value;
    return Boolean(baseRequest && baseRequest.document_output_mode === 'full_document' && !baseRequest.auto_continue);
};

const sendAutoContinueMessage = async (artifact, recommendedRequest = {}) => {
    const state = autoContinueState.value;
    const baseRequest = state.originalRequest || lastRequestMeta.value;
    const isTranslationTask = isTranslationContinuationRequest(baseRequest) && Boolean(state.generationRunId);
    if (!baseRequest || (!artifact?.id && !isTranslationTask)) {
        stopAutoContinue('缺少可续写的请求上下文');
        return;
    }

    const nextRequest = {
        ...baseRequest,
        query: state.prompt,
        intent_hint: 'continue_document',
        document_target_heading: undefined,
        document_merge_mode: undefined,
        images: undefined,
        attachment_uploads: undefined,
        auto_continue: true,
        generation_run_id: state.generationRunId || undefined,
        auto_continue_root_id: state.rootId,
        auto_continue_round: state.round,
        auto_continue_prompt: state.prompt,
        auto_continue_original_query: baseRequest.query
    };

    if (isTranslationTask) {
        nextRequest.base_artifact_id = artifact?.id || undefined;
        nextRequest.document_output_mode = artifact?.id ? 'delta_only' : 'full_document';
        nextRequest.document_task_kind = 'translation';
    } else {
        nextRequest.base_artifact_id = artifact.id;
        nextRequest.document_output_mode = 'delta_only';
    }

    Object.assign(nextRequest, normalizeRecommendedAutoContinueRequest(recommendedRequest));

    messagesList.push({
        content: state.prompt,
        role: 'user',
        mentioned_items: [],
        images: [],
        attachments: [],
        channel: 'web',
        is_auto_continue: true,
        created_at: new Date().toISOString()
    });
    userHasScrolledUp.value = false;
    scrollToBottom(true);

    lastRequestMeta.value = nextRequest;
    suppressNextStreamCloseFinalize.value = true;
    stopStream();
    window.setTimeout(() => {
        suppressNextStreamCloseFinalize.value = false;
    }, 300);
    loading.value = true;
    isReplying.value = true;
    userquery.value = state.prompt;

    await startStream({ ...nextRequest });
};

const maybeContinueDocumentAutomatically = async (message, payload = {}, hydratedArtifact = null) => {
    const nextAction = typeof payload?.next_action === 'string' ? payload.next_action : '';
    if (!autoContinueState.value.enabled) {
        if (!shouldImplicitlyStartFullDocumentAutoContinue(payload)) {
            return;
        }
        startAutoContinueFlow(lastRequestMeta.value);
    }

    if (payload.generation_run_id) {
        autoContinueState.value.generationRunId = payload.generation_run_id;
    }
    if (Array.isArray(payload.effective_kb_ids)) {
        autoContinueState.value.effectiveKBIDs = [...payload.effective_kb_ids];
    }
    if (payload.outline && typeof payload.outline === 'object') {
        autoContinueState.value.outline = payload.outline;
    }

    const generationStatus = payload.document_generation_status;
    if (nextAction && nextAction !== 'continue_auto') {
        const fallbackReason = nextAction === 'wait_user_review'
            ? '当前文档需要人工检查，自动续写已暂停'
            : nextAction === 'blocked'
                ? '当前文档生成被阻断，自动续写已停止'
                : nextAction === 'manual_retry'
                    ? '当前轮生成未能稳定完成，自动续写已暂停'
                    : '文档已完成';
        stopAutoContinue(getDocumentNextReasonMessage(payload, fallbackReason));
        return;
    }
    if (!nextAction && (payload.auto_continue_next === false || (generationStatus && generationStatus !== 'continuing'))) {
        const fallbackReason = generationStatus === 'needs_review'
            ? '当前文档需要人工检查，自动续写已暂停'
            : generationStatus === 'blocked'
                ? '当前文档生成被阻断，自动续写已停止'
                : generationStatus === 'continuing'
                    ? '当前轮生成未能稳定完成，自动续写已暂停'
                : '文档已完成';
                stopAutoContinue(getDocumentNextReasonMessage(payload, fallbackReason));
        return;
    }

    const completionStatus = payload.completion_status || message?.completion_status;
    if (completionStatus === 'failed' || completionStatus === 'cancelled') {
        stopAutoContinue('当前轮生成失败或已取消');
        return;
    }

    const artifact = hydratedArtifact || selectedBaseArtifact.value || message?.chat_document_artifact;
    const originalRequest = autoContinueState.value.originalRequest || lastRequestMeta.value || {};
    const isTranslationTask = isTranslationContinuationRequest(originalRequest) && Boolean(autoContinueState.value.generationRunId || payload.generation_run_id);
    if (!artifact?.id && !isTranslationTask) {
        stopAutoContinue('当前轮未生成可续写文档版本');
        return;
    }
    if (artifact?.id && !canContinueChatDocumentArtifact(artifact)) {
        stopAutoContinue(artifact.user_hint || '当前文档无法继续自动续写');
        return;
    }
    autoContinueState.value.round += 1;

    window.setTimeout(() => {
        sendAutoContinueMessage(artifact || null, payload.recommended_request).catch((error) => {
            console.warn('[AutoContinueDocument] Failed to continue automatically:', error);
            stopAutoContinue(error?.message || '自动续写请求失败');
            resetReplyState();
        });
    }, 200);
};

const loadSessionArtifacts = async (targetSessionId = session_id.value) => {
    if (!targetSessionId) {
        return;
    }
    if (isSharePageMode.value) {
        return;
    }
    try {
        const res = await getChatDocumentArtifacts(targetSessionId, 100);
        chatDocumentArtifacts.value = Array.isArray(res?.data)
            ? res.data.map((item) => normalizeChatDocumentArtifact(item)).filter(Boolean)
            : [];
        refreshLatestArtifactForDisplay(chatDocumentArtifacts.value, targetSessionId);
        applyChatDocumentArtifactsToMessages();
        const latestContinuableArtifact = findLatestContinuableArtifact(chatDocumentArtifacts.value, targetSessionId);
        const latestBaseUsableArtifact = findLatestBaseUsableArtifact(chatDocumentArtifacts.value, targetSessionId);
        if (selectedBaseArtifact.value) {
            const nextSelected = chatDocumentArtifacts.value.find((item) => item.id === selectedBaseArtifact.value.id) || null;
            if (selectedBaseArtifactDisplayLocked.value) {
                if (nextSelected?.id) {
                    selectedBaseArtifact.value = nextSelected;
                } else {
                    selectedBaseArtifact.value = latestBaseUsableArtifact;
                    selectedBaseArtifactDisplayLocked.value = false;
                }
            } else {
                selectedBaseArtifact.value = latestBaseUsableArtifact;
                selectedBaseArtifactDisplayLocked.value = false;
            }
        } else {
            selectedBaseArtifact.value = latestBaseUsableArtifact || latestContinuableArtifact;
            selectedBaseArtifactDisplayLocked.value = false;
        }
    } catch (error) {
        console.warn('[ChatDocumentArtifact] Failed to load session artifacts:', error);
    }
};

const inferDocumentContinuationIntent = (query = '') => {
    const text = query.trim().toLowerCase();
    if (!text) return 'normal';
    if (/(修改上一版|基于上一个文档修改|把上一份改成|调整上一版|完善上一版)/.test(text)) return 'revise_document';
    if (hasSectionContinuationTarget(query)) return 'revise_document';
    if (/(继续生成|接着写|续写|从上次中断处继续|补全剩余|继续输出|请继续补齐|继续补齐|继续补充|继续完善|接着补齐|补齐剩余|补充剩余|继续扩写)/.test(text)) return 'continue_document';
    return 'normal';
};

const inferDocumentOutputMode = (intentHint = 'normal') => {
    if (intentHint === 'continue_document' || intentHint === 'revise_document') {
        return 'delta_only';
    }
    return '';
};

const hasScopedRevisionTarget = (query = '') => {
    const text = query.trim();
    if (!text) {
        return false;
    }
    return /(第[0-9一二三四五六七八九十百零]+章|第[0-9一二三四五六七八九十百零]+节|第[0-9一二三四五六七八九十百零]+部分|章节|小节|段落|开头|结尾|引言|前言|背景|目标|方案|设计|实施|风险|附录|总结|标题|表格|代码块)/.test(text);
};

const hasSectionContinuationTarget = (query = '') => {
    const text = query.trim();
    if (!text) {
        return false;
    }
    const hasContinueVerb = /(继续|接着|续写|补充|扩写|完善|细化|补齐)/.test(text);
    if (!hasContinueVerb) {
        return false;
    }
    const hasScopedTarget = /(章节|小节|段落|标题|模块|部分|第[0-9一二三四五六七八九十百零]+(?:章|节|部分)|[0-9]+(?:\.[0-9]+)+|智慧运行|智慧安防|数据湖|算力平台|应急中心|AR眼镜)/i.test(text);
    if (!hasScopedTarget) {
        return false;
    }
    return !/(剩余内容|剩余章节|后续章节|余下章节|从上次中断|文档末尾|继续剩余|当前文档为基准|自动续写)/.test(text);
};

const inferDocumentTargetHeading = (query = '') => {
    const text = query.trim();
    if (!text) {
        return '';
    }

    const quotedTarget = text.match(/["“'‘]([^"”'’\n]{1,40})["”'’](?:章节|小节|模块|部分)?/);
    if (quotedTarget?.[1]) {
        return quotedTarget[1].trim();
    }

    const scopedTarget = text.match(/(?:在|对|把|将|就)?\s*(第[0-9一二三四五六七八九十百零]+(?:章|节|部分)|[0-9]+(?:\.[0-9]+)+|[\u4e00-\u9fa5A-Za-z0-9_-]{2,40})(章节|小节|模块|部分)/);
    if (scopedTarget?.[1]) {
        return scopedTarget[1].trim();
    }

    for (const keyword of ['智慧运行', '智慧安防', '数据湖', '算力平台', '智能安全监控应急中心', '应急中心', 'AR眼镜']) {
        if (text.includes(keyword)) {
            return keyword;
        }
    }
    return '';
};

const inferDocumentMergeMode = (intentHint = 'normal', targetHeading = '') => {
    if (!targetHeading) {
        return undefined;
    }
    if (intentHint === 'revise_document' || intentHint === 'continue_document') {
        return 'append_to_section';
    }
    return undefined;
};

const resolveLatestArtifactIfNeeded = async (intentHint) => {
    if (intentHint !== 'continue_document' && intentHint !== 'revise_document') {
        return null;
    }
    if (selectedBaseArtifactDisplayLocked.value && selectedBaseArtifact.value?.id && selectedBaseArtifact.value?.session_id === session_id.value && canUseChatDocumentArtifactForIntent(selectedBaseArtifact.value, intentHint)) {
        return selectedBaseArtifact.value;
    }
    const latestArtifact = intentHint === 'revise_document'
        ? findLatestBaseUsableArtifact()
        : findLatestContinuableArtifact();
    if (latestArtifact) {
        return latestArtifact;
    }
    if (selectedBaseArtifact.value?.id && selectedBaseArtifact.value?.session_id === session_id.value && canUseChatDocumentArtifactForIntent(selectedBaseArtifact.value, intentHint)) {
        return selectedBaseArtifact.value;
    }
    if (!session_id.value) {
        return null;
    }
    try {
        const res = await getLatestChatDocumentArtifact(session_id.value);
        const latestArtifact = normalizeChatDocumentArtifact(res?.data || null);
        return canUseChatDocumentArtifactForIntent(latestArtifact, intentHint) ? latestArtifact : null;
    } catch (error) {
        console.warn('[ChatDocumentArtifact] Failed to resolve latest artifact:', error);
        return null;
    }
};

const findActiveAssistantMessage = () => {
    if (currentAssistantMessageId.value) {
        const current = messagesList.findLast((item) => item.id === currentAssistantMessageId.value || item.request_id === currentAssistantMessageId.value);
        if (current) {
            return current;
        }
    }

    return messagesList.findLast((item) => item.role === 'assistant' && !isMessageTerminal(item));
};

const ensureAssistantMessage = () => {
    const existing = findActiveAssistantMessage();
    if (existing) {
        return existing;
    }

    const message = buildLocalAssistantMessage(Boolean(lastRequestMeta.value?.agent_enabled));
    messagesList.push(message);
    return message;
};

const resetReplyState = () => {
    loading.value = false;
    isReplying.value = false;
    fullContent.value = '';
    currentAssistantMessageId.value = '';
};

const useArtifactAsBase = (artifact) => {
    const normalized = upsertChatDocumentArtifact(artifact);
    if (!normalized) {
        return;
    }
    if (!canUseChatDocumentArtifactAsBase(normalized)) {
        MessagePlugin.warning(normalized.user_hint || '当前版本无法作为修订或续写基线。');
        return;
    }
    artifactDisplayHint.value = null;
    selectedBaseArtifact.value = normalized;
    selectedBaseArtifactDisplayLocked.value = true;
    if ((normalized.snapshot_char_count || 0) > 30000) {
        MessagePlugin.info('当前文档较长；续写会使用目录和末尾窗口作为上下文，如果要修改请尽量指定章节或段落范围。');
    }
    MessagePlugin.info(`已选择 V${normalized.revision_no || 1} 作为后续基线`);
};

const clearSelectedBaseArtifact = () => {
    selectedBaseArtifact.value = findLatestBaseUsableArtifact() || findLatestContinuableArtifact();
    selectedBaseArtifactDisplayLocked.value = false;
    artifactDisplayHint.value = null;
};

const handleArtifactDisplayUpdate = async (payload) => {
    const normalized = normalizeChatDocumentArtifact(payload?.artifact || payload);
    if (!normalized?.id) {
        return;
    }
    artifactDisplayHint.value = normalized;

    const requestedMode = typeof payload?.mode === 'string' ? payload.mode.trim() : '';
    if (requestedMode !== 'full' && requestedMode !== 'delta') {
        return;
    }

    const message = resolveMessageForArtifactDisplay(payload);
    if (!message) {
        return;
    }

    if (requestedMode === 'full') {
        try {
            const hydratedArtifact = await ensureMessageFinalDocumentContent(message, normalized);
            const finalDocument = typeof message.final_document_content === 'string' ? message.final_document_content.trim() : '';
            if (!hydratedArtifact?.id || !finalDocument) {
                MessagePlugin.warning('当前完整文档暂不可用');
                return;
            }
            artifactDisplayHint.value = hydratedArtifact;
            message.document_display_mode = 'full';
        } catch (error) {
            console.warn('[ChatDocumentArtifact] Failed to hydrate artifact for display toggle:', error);
            MessagePlugin.error('加载完整文档失败');
        }
        return;
    }

    message.document_display_mode = 'delta';
};

const openArtifactRevisionDrawer = async (artifact) => {
    const normalized = upsertChatDocumentArtifact(artifact);
    if (!normalized?.id) {
        return;
    }
    artifactRevisionAnchor.value = normalized;
    artifactRevisionDrawerVisible.value = true;
    artifactRevisionLoading.value = true;
    artifactRevisionList.value = [];
    try {
        const res = await getChatDocumentArtifactRevisions(normalized.id);
        artifactRevisionList.value = Array.isArray(res?.data)
            ? res.data.map((item) => upsertChatDocumentArtifact(item)).filter(Boolean)
            : [];
    } catch (error) {
        console.warn('[ChatDocumentArtifact] Failed to load revisions:', error);
        MessagePlugin.error('加载版本链失败');
    } finally {
        artifactRevisionLoading.value = false;
    }
};

const getAgentStreamSignals = (message) => {
    const stream = Array.isArray(message?.agentEventStream) ? message.agentEventStream : [];
    const completeEvent = stream.find((event) => event.type === 'agent_complete') || null;
    const stopEvent = stream.find((event) => event.type === 'stop') || null;
    const hasAnswerDone = stream.some((event) => event.type === 'answer' && event.done === true);
    const hasAnswerContent = Boolean(
        (message?.content && String(message.content).trim()) ||
        stream.some((event) => event.type === 'answer' && event.content && String(event.content).trim())
    );
    const hasAgentProgress = stream.some((event) =>
        event.type === 'thinking' ||
        event.type === 'tool_call' ||
        event.type === 'tool_result' ||
        event.type === 'reflection'
    );

    return {
        completeEvent,
        stopEvent,
        hasAnswerDone,
        hasAnswerContent,
        hasAgentProgress,
    };
};

const finalizeActiveAssistantOnStreamClose = () => {
    if (!isReplying.value && !loading.value) {
        return;
    }

    const message = findActiveAssistantMessage();
    if (!message) {
        resetReplyState();
        return;
    }

    if (!isMessageTerminal(message)) {
        if (message.isAgentMode) {
            const { completeEvent, stopEvent, hasAnswerDone, hasAnswerContent, hasAgentProgress } = getAgentStreamSignals(message);

            if (completeEvent) {
                syncMessageCompletionState(message, completeEvent);
            } else if (stopEvent) {
                const stopReason = stopEvent.reason || message.finish_reason || message.failure_reason || 'cancelled';
                syncMessageCompletionState(message, {
                    completion_status: 'cancelled',
                    finish_reason: stopReason,
                    failure_reason: stopReason,
                });
            } else if (hasAnswerDone || hasAnswerContent || hasAgentProgress) {
                syncMessageCompletionState(message, {
                    completion_status: 'partial',
                    finish_reason: message.finish_reason || 'stream_closed',
                    failure_reason: message.failure_reason || 'stream_closed'
                });
            }
        } else {
            const hasAnswerContent = Boolean(message.content && String(message.content).trim());
            if (hasAnswerContent) {
                syncMessageCompletionState(message, {
                    completion_status: 'partial',
                    finish_reason: message.finish_reason || 'stream_closed',
                    failure_reason: message.failure_reason || 'stream_closed'
                });
            }
        }
    }

    resetReplyState();
};

const markAssistantFailed = (errorMessage) => {
    const normalizedError = errorMessage || t('chat.processError');
    const message = ensureAssistantMessage();

    message.error_message = normalizedError;
    syncMessageCompletionState(message, {
        completion_status: 'failed',
        finish_reason: message.finish_reason || 'error',
        failure_reason: normalizedError,
        is_failed: true
    });
    if (message.isAgentMode) {
        if (!message.agentEventStream) {
            message.agentEventStream = [];
        }
        const hasTerminalError = message.agentEventStream.some((event) => event.type === 'error' && event.terminal);
        if (!hasTerminalError) {
            message.agentEventStream.push({
                type: 'error',
                content: normalizedError,
                done: true,
                terminal: true,
                timestamp: Date.now()
            });
        }
    } else if (!message.content?.trim()) {
        message.content = normalizedError;
    }

    resetReplyState();
    scrollToBottom(true);
};

const buildRetryPayloadFromUserMessage = (userMessage) => {
    if (userMessage?.retry_payload) {
        return userMessage.retry_payload;
    }

    const agentEnabled = getEffectiveAgentEnabled();
    const selectedAgentId = (isSharePageMode.value || effectiveEmbeddedMode.value)
        ? effectiveAgentId.value
        : (useSettingsStoreInstance.selectedAgentId || '');
    const webSearchEnabled = getEffectiveWebSearchEnabled();
    const enableMemory = getEffectiveMemoryEnabled();
    const kbIds = getEffectiveKnowledgeBaseIds();
    const knowledgeIds = getEffectiveKnowledgeIds();
    const mcpServiceIds = getEffectiveMCPServiceIDs();

    return {
        request: applyRuntimeChatRequest({
            session_id: session_id.value,
            knowledge_base_ids: kbIds,
            knowledge_ids: knowledgeIds,
            agent_enabled: agentEnabled,
            agent_id: selectedAgentId,
            web_search_enabled: webSearchEnabled,
            enable_memory: enableMemory,
            summary_model_id: '',
            mcp_service_ids: mcpServiceIds,
            mentioned_items: userMessage?.mentioned_items || [],
            images: undefined,
            attachment_uploads: undefined,
            query: userMessage?.content || '',
            method: 'POST',
            url: agentEnabled ? '/api/v1/agent-chat' : '/api/v1/knowledge-chat'
        }),
        display: {
            mentioned_items: userMessage?.mentioned_items || [],
            user_images: userMessage?.images || [],
            attachments: userMessage?.attachments || []
        }
    };
};

const findRetrySourceUserMessage = (assistantIndex) => {
    const previousMessages = messagesList.slice(0, assistantIndex).reverse();
    return previousMessages.find((item) => item.role === 'user' && item.retry_payload)
        || previousMessages.find((item) => item.role === 'user' && !item.is_auto_continue)
        || previousMessages.find((item) => item.role === 'user')
        || null;
};

const isRetryableInterruptedDocumentMessage = (assistantSession = {}) => {
    const translationProgress = getMessageTranslationProgress(assistantSession);
    const translationGenerationRunId = getMessageGenerationRunId(assistantSession);

    if (translationGenerationRunId && translationProgress) {
        const completionStatus = assistantSession?.completion_status || '';
        const finishReason = assistantSession?.finish_reason || '';
        const failureReason = assistantSession?.failure_reason || '';
        const generationStatus = assistantSession?.document_generation_status || '';

        if (completionStatus === 'failed') {
            return true;
        }
        if (generationStatus === 'continuing' && (finishReason || failureReason)) {
            return true;
        }
    }

    const artifact = assistantSession?.chat_document_artifact;
    if (!canContinueChatDocumentArtifact(artifact)) {
        return false;
    }

    const completionStatus = assistantSession?.completion_status || '';
    const finishReason = assistantSession?.finish_reason || '';
    const failureReason = assistantSession?.failure_reason || '';
    const generationStatus = assistantSession?.document_generation_status || artifact?.document_generation_status || '';

    if (completionStatus === 'failed') {
        return true;
    }
    if (generationStatus === 'continuing' && (finishReason || failureReason)) {
        return true;
    }
    return false;
};

const shouldReuseGenerationRunOnRetry = (assistantSession = {}) => {
    const stopReason = getMessageAutoContinueReason(assistantSession)
        || assistantSession?.finish_reason
        || assistantSession?.failure_reason
        || '';
    if (stopReason === 'auto_continue_round_limit') {
        return false;
    }
    return Boolean(getMessageGenerationRunId(assistantSession));
};

const buildRetryPayloadFromAssistantMessage = (assistantSession, userMessage) => {
    const baseRetryPayload = buildRetryPayloadFromUserMessage(userMessage);
    if (!isRetryableInterruptedDocumentMessage(assistantSession)) {
        return baseRetryPayload;
    }

    const baseArtifact = normalizeChatDocumentArtifact(assistantSession?.chat_document_artifact);
    const originalRequest = cloneAutoContinueOriginalRequest({
        ...baseRetryPayload.request,
        query: userMessage?.content || baseRetryPayload.request?.query || '',
    });
    const generationRunId = shouldReuseGenerationRunOnRetry(assistantSession)
        ? getMessageGenerationRunId(assistantSession)
        : '';
    const translationProgress = getMessageTranslationProgress(assistantSession);
    const isTranslationRetry = Boolean(generationRunId && translationProgress);

    if (isTranslationRetry) {
        return {
            request: {
                ...baseRetryPayload.request,
                query: AUTO_CONTINUE_PROMPT,
                intent_hint: 'continue_document',
                base_artifact_id: baseArtifact?.id || undefined,
                document_output_mode: baseArtifact?.id ? 'delta_only' : 'full_document',
                document_task_kind: 'translation',
                document_target_heading: undefined,
                document_merge_mode: undefined,
                images: undefined,
                attachment_uploads: undefined,
                auto_continue: true,
                generation_run_id: generationRunId || undefined,
                auto_continue_root_id: undefined,
                auto_continue_round: 0,
                auto_continue_prompt: AUTO_CONTINUE_PROMPT,
                auto_continue_original_query: originalRequest.query,
            },
            display: baseRetryPayload.display || {},
            document_resume_context: {
                baseArtifact: baseArtifact?.id ? baseArtifact : null,
                originalRequest,
                generationRunId,
                effectiveKBIDs: Array.isArray(assistantSession?.effective_kb_ids) ? [...assistantSession.effective_kb_ids] : [],
            }
        };
    }

    if (!baseArtifact?.id) {
        return baseRetryPayload;
    }

    return {
        request: {
            ...baseRetryPayload.request,
            query: AUTO_CONTINUE_PROMPT,
            intent_hint: 'continue_document',
            base_artifact_id: baseArtifact.id,
            document_output_mode: 'delta_only',
            document_target_heading: undefined,
            document_merge_mode: undefined,
            images: undefined,
            attachment_uploads: undefined,
            auto_continue: true,
            generation_run_id: generationRunId || undefined,
            auto_continue_root_id: undefined,
            auto_continue_round: 0,
            auto_continue_prompt: AUTO_CONTINUE_PROMPT,
            auto_continue_original_query: originalRequest.query,
        },
        display: baseRetryPayload.display || {},
        document_resume_context: {
            baseArtifact,
            originalRequest,
            generationRunId,
            effectiveKBIDs: Array.isArray(assistantSession?.effective_kb_ids) ? [...assistantSession.effective_kb_ids] : [],
        }
    };
};

const primeDocumentRetryContext = (retryPayload = {}) => {
    const resumeContext = retryPayload?.document_resume_context;
    if (!resumeContext?.originalRequest || (!resumeContext?.baseArtifact?.id && !resumeContext?.generationRunId)) {
        return;
    }

    const artifact = resumeContext.baseArtifact?.id ? upsertChatDocumentArtifact(resumeContext.baseArtifact) : null;
    if (artifact?.id) {
        selectedBaseArtifact.value = artifact;
        selectedBaseArtifactDisplayLocked.value = false;
    }

    startAutoContinueFlow(resumeContext.originalRequest);
    autoContinueState.value = {
        ...autoContinueState.value,
        generationRunId: resumeContext.generationRunId || '',
        effectiveKBIDs: Array.isArray(resumeContext.effectiveKBIDs) ? [...resumeContext.effectiveKBIDs] : [],
        stoppedReason: ''
    };
};

const resendFromRetryPayload = async (retryPayload) => {
    if (!retryPayload?.request?.query) {
        MessagePlugin.error(t('chat.processError'));
        return;
    }

    const request = {
        ...retryPayload.request,
        session_id: session_id.value
    };
    primeDocumentRetryContext(retryPayload);
    lastRequestMeta.value = request;
    userquery.value = request.query;
    isReplying.value = true;
    loading.value = true;

    messagesList.push({
        content: request.query,
        role: 'user',
        mentioned_items: retryPayload.display?.mentioned_items || [],
        images: retryPayload.display?.user_images || [],
        attachments: retryPayload.display?.attachments || [],
        channel: 'web',
        is_auto_continue: Boolean(retryPayload.document_resume_context) || Boolean(request.auto_continue),
        retry_payload: {
            request,
            display: retryPayload.display || {},
            document_resume_context: retryPayload.document_resume_context || undefined,
        }
    });
    userHasScrolledUp.value = false;
    scrollToBottom(true);

    await startStream(request);
};

const handleRetry = async (assistantSession) => {
    if (isReplying.value) {
        MessagePlugin.warning(t('chat.replyingPleaseWait'));
        return;
    }

    const assistantIndex = messagesList.findIndex((item) => item === assistantSession || item.id === assistantSession?.id || item.request_id === assistantSession?.request_id);
    if (assistantIndex < 0) {
        MessagePlugin.error(t('chat.processError'));
        return;
    }

    try {
        const userMessage = findRetrySourceUserMessage(assistantIndex) || {
            content: getUserQuery(assistantIndex) || '',
            mentioned_items: [],
            images: [],
            attachments: []
        };
        const retryPayload = buildRetryPayloadFromAssistantMessage(assistantSession, userMessage);
        await resendFromRetryPayload(retryPayload);
    } catch (error) {
        console.error('[Retry] Failed to resend interrupted document request:', {
            error,
            assistantSessionId: assistantSession?.id,
            assistantRequestId: assistantSession?.request_id,
            assistantIndex,
        });
        MessagePlugin.error(error?.message || t('chat.processError'));
        resetReplyState();
    }
};
watch([() => route.params], (newvalue) => {
    isFirstEnter.value = true;
    if (newvalue[0].chatid) {
        if (!firstQuery.value) {
            scrollLock.value = false;
        }
        messagesList.splice(0);
        chatDocumentArtifacts.value = [];
        selectedBaseArtifact.value = null;
        selectedBaseArtifactDisplayLocked.value = false;
        artifactDisplayHint.value = null;
        resetAutoContinue();
        artifactRevisionDrawerVisible.value = false;
        artifactRevisionList.value = [];
        artifactRevisionAnchor.value = null;
        session_id.value = newvalue[0].chatid;
        
        // 切换会话时，重置状态
        historyLoading.value = true;
        loading.value = false;
        isReplying.value = false;
        currentAssistantMessageId.value = '';
        userHasScrolledUp.value = false;
        
        checkmenuTitle(session_id.value)
        let data = {
            session_id: session_id.value,
            created_at: '',
            limit: limit.value
        }
        getmsgList(data);
    }
});
const scrollToBottom = (force = false) => {
    if (!force && userHasScrolledUp.value) return;
    nextTick(() => {
        if (scrollContainer.value) {
            scrollContainer.value.scrollTop = scrollContainer.value.scrollHeight;
        }
    })
}
const onClickScrollToBottom = () => {
    userHasScrolledUp.value = false;
    scrollToBottom(true);
}
const debounce = (fn, delay) => {
    let timer
    return (...args) => {
        clearTimeout(timer)
        timer = setTimeout(() => fn(...args), delay)
    }
}
const onChatScrollTop = () => {
    if (scrollLock.value) return;
    const { scrollTop, scrollHeight } = scrollContainer.value;
    isFirstEnter.value = false
    if (scrollTop == 0) {
        let data = {
            session_id: session_id.value,
            created_at: created_at.value,
            limit: limit.value
        }
        getmsgList(data, true, scrollHeight);
    }
}
const debouncedScrollTop = debounce(onChatScrollTop, 500);
const handleScroll = () => {
    userHasScrolledUp.value = !isNearBottom();
    debouncedScrollTop();
};

const getmsgList = (data, isScrollType = false, scrollHeight) => {
    loadMessagesForRuntime(data).then(res => {
        if (res && res.data?.length) {
            created_at.value = res.data[0].created_at;
            handleMsgList(res.data, isScrollType, scrollHeight);
        }
    }).finally(() => {
        historyLoading.value = false;
        if (!isScrollType && !data.created_at) {
            loadSessionArtifacts(data.session_id);
        }
    })
}

// Reconstruct agentEventStream from agent_steps stored in database
// This allows the frontend to restore the exact conversation state including all agent reasoning steps
const reconstructEventStreamFromSteps = (
    agentSteps,
    messageContent,
    isCompleted = false,
    isFallback = false,
    agentDurationMs = 0,
    completionStatus = '',
    finishReason = '',
    failureReason = '',
    outline = null
) => {
    const events = [];
    const normalizedCompletionStatus = normalizeCompletionStatus({
        role: 'assistant',
        completion_status: completionStatus,
        is_completed: isCompleted
    });

    // Process agent steps if they exist
    if (agentSteps && Array.isArray(agentSteps) && agentSteps.length > 0) {
    agentSteps.forEach((step) => {
        // Compute step timestamp (milliseconds) from step.timestamp if available
        const stepTimestamp = step.timestamp ? new Date(step.timestamp).getTime() : 0;

        // Add thinking event if thought content exists.
        // For tool-calling rounds, providers like MiMo / DeepSeek thinking-mode
        // emit reasoning into the OpenAI-protocol `reasoning_content` field
        // rather than visible `content`, so step.thought is often empty even
        // though the model did reason. Fall back to step.reasoning_content so
        // the historical step card mirrors what the user saw live.
        const thoughtText = (step.thought && step.thought.trim())
            ? step.thought
            : (step.reasoning_content && step.reasoning_content.trim())
                ? step.reasoning_content
                : '';
        if (thoughtText) {
            events.push({
                type: 'thinking',
                event_id: `step-${step.iteration}-thought`,
                content: thoughtText,
                done: true,
                thinking: false,
                synthetic: typeof step.stage === 'string' && step.stage.trim().length > 0,
                stage: typeof step.stage === 'string' ? step.stage : '',
                timestamp: stepTimestamp || undefined,
                // Extract duration from step if available
                duration_ms: step.duration || undefined,
            });
        }

        // Add tool call and result events (skip final_answer as its content is in the answer event)
        if (step.tool_calls && Array.isArray(step.tool_calls)) {
            step.tool_calls.forEach((toolCall) => {
                if (toolCall.name === 'final_answer') return; // Skip - shown as answer event
                events.push({
                    type: 'tool_call',
                    tool_call_id: toolCall.id,
                    tool_name: toolCall.name,
                    arguments: toolCall.args,
                    pending: false,
                    success: toolCall.result?.success !== false,
                    output: toolCall.result?.output || '',
                    error: toolCall.result?.error || undefined,
                    timestamp: stepTimestamp || undefined,
                    // Use both duration and duration_ms for compatibility
                    duration: toolCall.duration,
                    duration_ms: toolCall.duration,
                    display_type: toolCall.result?.data?.display_type,
                    tool_data: toolCall.result?.data,
                });
            });
        }
    });
    }
    
    // Add agent_complete event with duration info (before answer event).
    // Cancelled conversations are represented by stop, not agent_complete,
    // so recovery paths must not synthesize an extra agent_complete event.
    if (normalizedCompletionStatus !== 'cancelled' && (agentDurationMs > 0 || isTerminalCompletionStatus(normalizedCompletionStatus))) {
        events.push({
            type: 'agent_complete',
            total_duration_ms: agentDurationMs,
            completion_status: normalizedCompletionStatus,
            finish_reason: finishReason,
            failure_reason: failureReason,
            is_partial: normalizedCompletionStatus === 'partial',
            outline,
        });
    }

    // 总是添加 answer 事件如果有内容（无论是否有 agent_steps）
    // 这样可以确保最终答案始终被渲染
    if (messageContent && messageContent.trim()) {
        const answerEvent = {
            type: 'answer',
            content: messageContent,
            done: true,
            completion_status: normalizedCompletionStatus,
            finish_reason: finishReason,
            failure_reason: failureReason,
        };
        if (isFallback) answerEvent.is_fallback = true;
        events.push(answerEvent);
    }

    if (normalizedCompletionStatus === 'cancelled') {
        const cancelReason = finishReason || failureReason || 'cancelled';
        // 取消态恢复路径始终要保留一个 stop 事件。
        // 即使已有 answer 内容，也仍应和实时流保持一致：答案正文 + stop 终态。
        events.push({
            type: 'stop',
            timestamp: Date.now(),
            reason: cancelReason
        });
    }
    
    return events;
};
const handleMsgList = async (data, isScrollType = false, newScrollHeight) => {
    let chatlist = data.reverse()
    const loadedMessages = [];
    for (let i = 0, len = chatlist.length; i < len; i++) {
        let item = chatlist[i];
        item.isAgentMode = false; // Agent 模式标记
        item.agentEventStream = item.agentEventStream || [];
        item._eventMap = new Map();
        item._pendingToolCalls = new Map();
        syncMessageCompletionState(item);
        
        // Reconstruct historical agent conversations from persisted execution metadata.
        // Older records may be missing agent_steps but still preserve agent_duration_ms
        // or finish_reason=tool_calls, which is enough to rebuild a protocol-compatible
        // agent event stream during refresh/reload.
        if (isHistoricalAgentMessage(item)) {
            item.isAgentMode = true;
            const reconstructedOutline = extractStructuredPlanningOutlineFromText(
                item.final_document_content || item.chat_document_artifact?.content_snapshot || item.content || ''
            );
            item.agentEventStream = reconstructEventStreamFromSteps(
                item.agent_steps,
                item.content,
                item.is_completed,
                item.is_fallback,
                item.agent_duration_ms || 0,
                item.completion_status,
                item.finish_reason,
                item.failure_reason,
                reconstructedOutline
            );
            // 隐藏最终答案内容，因为它已经包含在 agentEventStream 的 answer 事件中
            item.hideContent = true;
        }
        
        if (item.content) {
            if (!item.content.includes('<think>') && !item.content.includes('<\/think>')) {
                item.thinkContent = "";
                item.content = item.content;
                item.showThink = false;
                item.thinking = false;
            } else if (item.content.includes('<\/think>')) {
                // 历史消息中包含完整的 <think>...</think> 标签，说明 thinking 已完成
                item.showThink = true;
                item.thinking = false;  // 关键：标记 thinking 已完成，使 deepThink 默认折叠
                const index = item.content.trim().lastIndexOf('<\/think>');
                item.thinkContent = item.content.trim().substring(0, index).replace('<think>', '').trim();
                item.content = item.content.trim().substring(index + 8);
            } else if (item.content.includes('<think>')) {
                // 内容包含 <think> 但没有 </think>，说明 thinking 还在进行中（不太可能出现在历史消息中）
                item.showThink = true;
                item.thinking = true;
                item.thinkContent = item.content.replace('<think>', '').trim();
                item.content = '';
            }
        }
        
        // 非 Agent 模式下若 content 为空（例如用户停止时尚未产出任何文字），
        // 保持为空；botmsg.vue 会因 hasActualContent=false 不渲染内容区和 toolbar。
        // 此前这里会兜底为 "chat.cannotAnswer"，会让停止场景显示误导性文案并出现复制按钮。
        messagesList.unshift(item);
        loadedMessages.push(item);
        if (isFirstEnter.value) {
            scrollToBottom(true);
        } else if (isScrollType) {
            nextTick(() => {
                const { scrollHeight } = scrollContainer.value;
                scrollContainer.value.scrollTop = scrollHeight - newScrollHeight
            })
        }
    }

    await recoverHistoricalAssistantMessages(loadedMessages);

    applyChatDocumentArtifactsToMessages();

}
const checkmenuTitle = (session_id) => {
    menuArr.value[1].children?.forEach(item => {
        if (item.id == session_id) {
            isNeedTitle.value = item.isNoTitle;
        }
    });
}
// 发送消息
// 处理停止生成事件 - 立即清除 loading 状态
const handleStopGeneration = () => {
    stopAutoContinue('用户已停止自动续写');
    loading.value = false;
    isReplying.value = false;
    // 注意：不在这里清空 currentAssistantMessageId，因为需要它来调用 API
    // API 调用成功后，后端的 stop 事件会清空它
};

const sendMsg = async (value, modelId = '', mentionedItems = [], imageFiles = [], attachmentFiles = []) => {
    userquery.value = value;
    isReplying.value = true;
    loading.value = true;
    resetAutoContinue();

    // Convert images to base64 data URIs for backend processing and local display
    let imageAttachments = [];
    let userImages = [];
    if (imageFiles && imageFiles.length > 0) {
        try {
            for (const file of imageFiles) {
                const dataURI = await fileToBase64(file);
                imageAttachments.push({ data: dataURI });
                userImages.push({ url: dataURI });
            }
        } catch (e) {
            console.error('[Image] Failed to read images:', e);
            loading.value = false;
            isReplying.value = false;
            return;
        }
    }

    // Convert attachment files to base64 for backend processing
    let attachmentUploads = [];
    if (attachmentFiles && attachmentFiles.length > 0) {
        try {
            for (const attachment of attachmentFiles) {
                const reader = new FileReader();
                const base64Promise = new Promise((resolve, reject) => {
                    reader.onload = () => {
                        const result = reader.result;
                        // Extract base64 content (remove data:...;base64, prefix)
                        const base64 = result.split(',')[1];
                        resolve(base64);
                    };
                    reader.onerror = reject;
                    reader.readAsDataURL(attachment.file);
                });
                const base64Data = await base64Promise;
                attachmentUploads.push({
                    data: base64Data,
                    file_name: attachment.name,
                    file_size: attachment.size
                });
            }
        } catch (e) {
            console.error('[Attachment] Failed to read attachments:', e);
            loading.value = false;
            isReplying.value = false;
            return;
        }
    }

    const attachmentDisplay = attachmentFiles.map(a => ({ file_name: a.name, file_size: a.size, file_type: '.' + a.name.split('.').pop()?.toLowerCase() }));
    
    // Get agent mode status from settings store
    const agentEnabled = getEffectiveAgentEnabled();
    
    // Get web search status from settings store
    const webSearchEnabled = getEffectiveWebSearchEnabled();
    
    // Get memory status from settings store
    const enableMemory = getEffectiveMemoryEnabled();
    
    // Get knowledge_base_ids from settings store (selected by user via KnowledgeBaseSelector)
    // Merge @mentioned KB/file IDs so retrieval uses the same targets user @mentioned (including shared KBs)
    const sidebarKbIds = getEffectiveKnowledgeBaseIds();
    const sidebarFileIds = getEffectiveKnowledgeIds();
    const kbIdSet = new Set(sidebarKbIds);
    const fileIdSet = new Set(sidebarFileIds);
    for (const item of mentionedItems || []) {
      if (!item?.id) continue;
      if (item.type === 'kb' && !kbIdSet.has(item.id)) {
        kbIdSet.add(item.id);
      } else if (item.type === 'file' && !fileIdSet.has(item.id)) {
        fileIdSet.add(item.id);
      }
    }
    const kbIds = [...kbIdSet];
    const knowledgeIds = [...fileIdSet];

    // Get selected agent ID (backend resolves shared agent and its tenant from share relation)
    const selectedAgentId = (isSharePageMode.value || effectiveEmbeddedMode.value)
        ? effectiveAgentId.value
        : (useSettingsStoreInstance.selectedAgentId || '');

    // Use agent-chat endpoint when agent is enabled, otherwise use knowledge-chat
    const endpoint = agentEnabled ? '/api/v1/agent-chat' : '/api/v1/knowledge-chat';

    const explicitBaseArtifact = selectedBaseArtifact.value?.id && selectedBaseArtifact.value?.session_id === session_id.value
        ? selectedBaseArtifact.value
        : null;
    let intentHint = inferDocumentContinuationIntent(value);
    if (explicitBaseArtifact && intentHint === 'normal') {
        intentHint = 'revise_document';
    }
    const documentOutputMode = inferDocumentOutputMode(intentHint);
    const documentTargetHeading = inferDocumentTargetHeading(value);
    const documentMergeMode = inferDocumentMergeMode(intentHint, documentTargetHeading);
    const isDocumentEditIntent = intentHint === 'continue_document' || intentHint === 'revise_document';
    const requestIntentHint = intentHint === 'normal' ? undefined : intentHint;
    const latestArtifact = await resolveLatestArtifactIfNeeded(intentHint);
    if ((intentHint === 'continue_document' || intentHint === 'revise_document') && latestArtifact && !canUseChatDocumentArtifactForIntent(latestArtifact, intentHint)) {
        resetReplyState();
        MessagePlugin.warning(latestArtifact.user_hint || '当前文档无法作为本轮修订或续写基线，请检查完整文档后再尝试。');
        return;
    }
    if (intentHint === 'revise_document' && (latestArtifact?.snapshot_char_count || 0) > 30000 && !hasScopedRevisionTarget(value)) {
        resetReplyState();
        MessagePlugin.warning('当前文档较长。修改上一版时请尽量指定章节或段落范围，或先让模型生成精简版后再修改。');
        return;
    }
    const baseArtifactId = (intentHint === 'continue_document' || intentHint === 'revise_document') && latestArtifact?.id
        ? latestArtifact.id
        : undefined;
    
    // Get selected MCP services from settings store (if available)
    const mcpServiceIds = getEffectiveMCPServiceIDs();

    const requestParams = applyRuntimeChatRequest({
        session_id: session_id.value,
        knowledge_base_ids: kbIds,
        knowledge_ids: knowledgeIds,
        intent_hint: requestIntentHint,
        base_artifact_id: baseArtifactId,
        document_output_mode: documentOutputMode,
        document_target_heading: isDocumentEditIntent ? documentTargetHeading || undefined : undefined,
        document_merge_mode: isDocumentEditIntent ? documentMergeMode : undefined,
        agent_enabled: agentEnabled,
        agent_id: selectedAgentId,
        web_search_enabled: webSearchEnabled,
        enable_memory: enableMemory,
        summary_model_id: modelId,
        mcp_service_ids: mcpServiceIds,
        mentioned_items: mentionedItems,
        images: imageAttachments.length > 0 ? imageAttachments : undefined,
        attachment_uploads: attachmentUploads.length > 0 ? attachmentUploads : undefined,
        query: value,
        method: 'POST',
        url: endpoint
    });

    // 将@提及的知识库和文件信息存入用户消息，并保留本次请求参数以便失败后重试
    messagesList.push({
        content: value,
        role: 'user',
        mentioned_items: mentionedItems,
        images: userImages,
        attachments: attachmentDisplay,
        channel: 'web',
        created_at: new Date().toISOString()
    });
    userHasScrolledUp.value = false;
    scrollToBottom(true);

    const retryPayload = {
        request: requestParams,
        display: {
            mentioned_items: mentionedItems,
            user_images: userImages,
            attachments: attachmentDisplay
        }
    };

    messagesList[messagesList.length - 1].retry_payload = retryPayload;
    lastRequestMeta.value = requestParams;
    
    await startStream({ 
        ...requestParams
    });
}

// Watch for stream errors and show message
watch(error, (newError) => {
    if (newError) {
        MessagePlugin.error(newError);
        markAssistantFailed(newError);
    }
});

onClose(() => {
    if (suppressNextStreamCloseFinalize.value) {
        suppressNextStreamCloseFinalize.value = false;
        return;
    }
    finalizeActiveAssistantOnStreamClose();
});

// 处理流式数据
onChunk((data) => {
    // 处理 agent query 事件 - 保存 assistant message ID 并保持 loading 状态
    if (data.response_type === 'agent_query') {
        if (data.assistant_message_id) {
            currentAssistantMessageId.value = data.assistant_message_id;
        }

        // 检查是否是继续流式传输（消息已存在）
        const existingMessage = messagesList.findLast((item) => item.id === data.id || item.request_id === data.id);
        if (!existingMessage) {
            // 新消息，设置 loading 状态
        loading.value = true;
        } else {
            existingMessage.isAgentMode = true;
            existingMessage.agentEventStream = existingMessage.agentEventStream || [];
            existingMessage._eventMap = existingMessage._eventMap || new Map();
            existingMessage._pendingToolCalls = existingMessage._pendingToolCalls || new Map();
            // 继续流式传输（刷新页面场景），不设置 loading，因为消息已经在列表中
        }
        return;
    }
    
    // 处理会话标题更新事件 - 不关闭 loading
    if (data.response_type === 'session_title') {
        const title = data.content || data.data?.title;
        if (title && data.data?.session_id) {
            usemenuStore.updatasessionTitle(data.data.session_id, title);
            usemenuStore.changeIsFirstSession(false);
            isNeedTitle.value = false;
        }
        // 不关闭 loading，等待实际内容
        return;
    }

    // 判断是否是 Agent 模式的响应
    // 注意：'answer', 'complete', 'references' 类型可能在两种模式下都存在
    // 其中 'complete' 目前也是 Agent 专有终态事件，用于恢复路径与实时流收口。
    const isAgentOnlyResponse = data.response_type === 'thinking' || 
                               data.response_type === 'tool_call' || 
                               data.response_type === 'tool_result' ||
                               data.response_type === 'reflection';
    
    // 检查当前消息是否已经是 Agent 模式
    const targetMessage = findMessageByStreamId(data.id);
    const isCurrentlyAgentMode = targetMessage?.isAgentMode === true;
    
    // 如果是 Agent 专有的响应类型，或者当前消息已经是 Agent 模式，则走 Agent 处理
    const shouldHandleAsAgent = isAgentOnlyResponse || isCurrentlyAgentMode;
    
    // 处理 references 事件 - 在两种模式下都需要处理，但不改变模式
    if (data.response_type === 'references') {
        // 如果当前是 Agent 模式，走 Agent 处理
        if (isCurrentlyAgentMode) {
            handleAgentChunk(data);
            return;
        }
        // 非 Agent 模式：将 references 保存到消息中供 botmsg 使用
        let existingMessage = messagesList.findLast((item) => item.request_id === data.id || item.id === data.id);
        
        // 如果消息还不存在，先创建一个空的 assistant 消息
        if (!existingMessage) {
            existingMessage = {
                id: data.id,
                request_id: data.id,
                role: 'assistant',
                content: '',
                showThink: false,
                thinkContent: '',
                thinking: false,
                is_completed: false,
                knowledge_references: []
            };
            messagesList.push(existingMessage);
            loading.value = false; // 消息已创建，关闭 loading
            scrollToBottom(true);
        }
        
        existingMessage.knowledge_references = data.knowledge_references || data.data?.references || [];
        return;
    }
    
    // Agent 模式处理（包括 stop 事件）
    if (shouldHandleAsAgent) {
        // 在 handleAgentChunk 中处理 loading 状态
        handleAgentChunk(data);
        
        // 对于 stop 事件，额外处理全局状态
        if (data.response_type === 'stop') {
            loading.value = false;
            isReplying.value = false;
            // 清空当前 assistant message ID
            currentAssistantMessageId.value = '';
        }
        return;
    }
    
    // 原有的知识库 QA 处理逻辑（非 Agent 模式）
    // answer 内容中可能包含 <think>...</think> 标签

    // 非 Agent 模式下的 stop 事件：只更新状态，不把后端附带的 "Generation stopped by user"
    // 文案拼进 content，保留用户点停止时已经流式输出的内容不变。
    if (data.response_type === 'stop') {
        const stoppedMessage = messagesList.findLast((item) => {
            if (item.request_id === data.id) return true;
            return item.id === data.id;
        });
        if (stoppedMessage) {
            syncMessageCompletionState(stoppedMessage, {
                completion_status: 'cancelled',
                finish_reason: data.data?.reason || 'cancelled',
                failure_reason: data.data?.reason || 'cancelled'
            });
        }
        loading.value = false;
        isReplying.value = false;
        fullContent.value = '';
        currentAssistantMessageId.value = '';
        return;
    }

    if (data.response_type === 'complete') {
        const completedMessage = messagesList.findLast((item) => {
            if (item.request_id === data.id) {
                return true;
            }
            return item.id === data.id;
        });
        if (completedMessage) {
            syncMessageCompletionState(completedMessage, data.data || {});
            if (data.data?.document_generation_status === 'completed') {
                completedMessage.content = stripDocumentCompleteMarker(data.data?.final_answer || completedMessage.content || '');
            } else if (data.data?.final_answer && !completedMessage.content) {
                completedMessage.content = data.data.final_answer;
            }
            if (data.data?.chat_document_artifact) {
                promoteCompletedArtifactAsBase(completedMessage, data.data.chat_document_artifact);
            }
            hydrateFinalDocumentFromComplete(completedMessage, data.data || {})
                .then((artifact) => maybeContinueDocumentAutomatically(completedMessage, data.data || {}, artifact))
                .catch((error) => {
                    console.warn('[AutoContinueDocument] Failed to schedule next continuation:', error);
                    stopAutoContinue(error?.message || '自动续写调度失败');
                });
        }
        resetReplyState();
        return;
    }

    // 检查消息是否已经完成，如果已完成则忽略后续的完成事件（防止空内容覆盖）
    const existingMessage = messagesList.findLast((item) => {
        if (item.request_id === data.id) {
            return true
        }
        return item.id === data.id;
    });
    
    // 如果消息已完成且当前事件是完成事件（done=true 且无内容），直接忽略
    if (isMessageTerminal(existingMessage) && data.done && !data.content) {
        return;
    }
    
    fullContent.value += data.content;
    let obj = {
        ...data,
        content: '',
        role: 'assistant',
        showThink: false,
        completion_status: normalizeCompletionStatus({ ...data, role: 'assistant' }),
        is_completed: false
    };

    // 检查是否为 fallback 回答（未从知识库检索到内容）
    if (data.data?.is_fallback) {
        obj.is_fallback = true;
    }

    if (fullContent.value.includes('<think>') && !fullContent.value.includes('<\/think>')) {
        obj.thinking = true;
        obj.showThink = true;
        obj.content = '';
        obj.thinkContent = fullContent.value.replace('<think>', '').trim();
    } else if (fullContent.value.includes('<think>') && fullContent.value.includes('<\/think>')) {
        obj.thinking = false;
        obj.showThink = true;
        // Use lastIndexOf to handle edge cases with multiple </think> occurrences,
        // consistent with history loading logic (line 280)
        const index = fullContent.value.lastIndexOf('<\/think>');
        obj.thinkContent = fullContent.value.substring(0, index).replace('<think>', '').trim();
        obj.content = fullContent.value.substring(index + 8).trim();
    } else {
        obj.content = fullContent.value;
    }
    
    if (!existingMessage) {
        loading.value = false; // 消息即将创建，关闭 loading
    }
    
    if (data.done) {
        syncMessageCompletionState(obj, data.data?.completion_status ? data : { ...data, completion_status: 'completed' });
        // 标题生成已改为异步事件推送，不再需要在这里手动调用
        // 如果标题还未生成，前端会通过 SSE 事件接收
        isReplying.value = false;
        fullContent.value = "";
        // 清空当前 assistant message ID
        currentAssistantMessageId.value = '';
    }
    updateAssistantSession(obj);
})
// 处理 Agent 流式数据 (Cursor-style UI)
const handleAgentChunk = (data) => {
    let message = messagesList.findLast((item) => item.request_id === data.id || item.id === data.id);
    
    if (!message) {
        // 创建新的 Assistant 消息 - 此时开始显示内容，关闭 loading
        const newMsg = {
            id: data.id,
            request_id: data.id,
            role: 'assistant',
            content: '',
            isAgentMode: true,
            completion_status: 'pending',
            finish_reason: '',
            failure_reason: '',
            is_completed: false,
            is_failed: false,
            // Event stream: ordered list of all agent events (thinking, tool calls, etc)
            agentEventStream: [],
            // Map to track event by event_id for quick lookup
            _eventMap: new Map(),
            knowledge_references: []
        };
        messagesList.push(newMsg);
        loading.value = false; // 消息已创建，关闭 loading
        scrollToBottom(true);
        // Don't return - continue to process the current event data
        message = newMsg;
    }
    
    message.isAgentMode = true;
    
    // 确保在继续流式传输时（刷新页面场景），一旦接收到实际内容就关闭 loading
    // 这是一个保护措施，防止任何边缘情况导致 loading 残留
    if (loading.value && (data.response_type === 'thinking' || data.response_type === 'answer' || data.response_type === 'tool_call' || data.response_type === 'tool_approval_required')) {
        loading.value = false;
    }
    
    switch(data.response_type) {
        case 'thinking':
            {
                const eventId = data.data?.event_id;

                // Initialize structures
                if (!message.agentEventStream) message.agentEventStream = [];
                if (!message._eventMap) message._eventMap = new Map();
                const thinkingEvent = upsertThinkingEvent(message, data);
                if (!thinkingEvent && data.done) {
                    console.warn('[Thinking] Received done for unknown event_id:', eventId);
                }
            }
            break;
            
        case 'tool_approval_required': {
            if (!message.agentEventStream) message.agentEventStream = [];
            const d = data.data || {};
            message.agentEventStream.push({
                type: 'tool_approval_required',
                pending_id: d.pending_id,
                service_name: d.service_name,
                mcp_tool_name: d.mcp_tool_name,
                description: d.description,
                args_json: d.args_json,
                timeout_seconds: d.timeout_seconds,
                requested_at: d.requested_at,
                tool_call_id: d.tool_call_id,
                resolved: false,
            });
            break;
        }
        case 'tool_approval_resolved': {
            const d = data.data || {};
            const pid = d.pending_id;
            const ev = message.agentEventStream?.find(
                (e) => e.type === 'tool_approval_required' && e.pending_id === pid
            );
            if (ev) {
                ev.resolved = true;
                ev.approved = d.approved;
                ev.resolve_reason = d.reason;
                ev.timed_out = d.timed_out;
                ev.canceled = d.canceled;
            }
            break;
        }
        case 'tool_call':
            // Skip final_answer tool call from event stream - its content appears as answer events
            if (data.data && data.data.tool_name === 'final_answer') {
                break;
            }
            // Store or update pending tool call to pair with result later
            if (data.data && (data.data.tool_name || data.data.tool_call_id)) {
                const incomingToolName = data.data.tool_name;
                const incomingArguments = data.data.arguments;
                
                if (!message.agentEventStream) message.agentEventStream = [];
                if (!message._pendingToolCalls) message._pendingToolCalls = new Map();
                
                const toolCallId = data.data.tool_call_id || (incomingToolName ? (incomingToolName + '_' + Date.now()) : null);
                if (!toolCallId) {
                    console.warn('[Tool Call] Received event without identifiable tool_call_id:', data.data);
                    break;
                }
                
                let toolCallEvent = message._pendingToolCalls.get(toolCallId);
                if (!toolCallEvent) {
                    toolCallEvent = message.agentEventStream.find(
                        (event) => event.type === 'tool_call' && event.tool_call_id === toolCallId
                    );
                }
                
                if (toolCallEvent) {
                    if (incomingToolName) toolCallEvent.tool_name = incomingToolName;
                    if (incomingArguments) toolCallEvent.arguments = incomingArguments;
                    toolCallEvent.pending = true;
                    if (!toolCallEvent.timestamp) {
                        toolCallEvent.timestamp = Date.now();
                    }
                    message._pendingToolCalls.set(toolCallId, toolCallEvent);
                } else {
                    const newToolCallEvent = {
                        type: 'tool_call',
                        tool_call_id: toolCallId,
                        tool_name: incomingToolName,
                        arguments: incomingArguments,
                        timestamp: Date.now(),
                        pending: true
                    };
                    message.agentEventStream.push(newToolCallEvent);
                    message._pendingToolCalls.set(toolCallId, newToolCallEvent);
                }
            }
            break;
            
        case 'tool_result':
        case 'error':
            // Tool result - update the corresponding tool call event
            if (data.data) {
                const toolCallId = data.data.tool_call_id;
                const toolName = data.data.tool_name;
                const success = data.response_type !== 'error' && data.data.success !== false;

                // Find and update the pending tool call event
                let toolCallEvent = null;
                if (message._pendingToolCalls) {
                    if (toolCallId && message._pendingToolCalls.has(toolCallId)) {
                        toolCallEvent = message._pendingToolCalls.get(toolCallId);
                        message._pendingToolCalls.delete(toolCallId);
                    } else {
                        // Try to find by tool_name if no tool_call_id match
                        for (const [key, value] of message._pendingToolCalls.entries()) {
                            if (value.tool_name === toolName) {
                                toolCallEvent = value;
                                message._pendingToolCalls.delete(key);
                                break;
                            }
                        }
                    }
                }
                
                if (toolCallEvent) {
                    // Update the existing event with result
                    toolCallEvent.pending = false;
                    toolCallEvent.success = success;
                    toolCallEvent.output = success ? (data.data.output || data.content) : (data.data.error || data.content);
                    toolCallEvent.error = !success ? (data.data.error || data.content) : undefined;
                    // Set both duration and duration_ms for compatibility
                    const duration = data.data.duration_ms !== undefined ? data.data.duration_ms : data.data.duration;
                    toolCallEvent.duration = duration;
                    toolCallEvent.duration_ms = duration;
                    toolCallEvent.display_type = data.data.display_type;
                    toolCallEvent.tool_data = data.data;
                } else {
                    console.warn('[Tool Result] No pending tool call found for', toolCallId || toolName);
                }
                
                // If this is an error response without tool data, handle it
                if (data.response_type === 'error' && !toolName) {
                    const errorMsg = data.content || t('chat.processError');
                    message.content = message.content || errorMsg;
                    markAssistantFailed(errorMsg);
                    MessagePlugin.error(errorMsg);
                    console.error('[Chat Error]', errorMsg);
                }
            } else if (data.response_type === 'error') {
                // Generic error without tool context
                const errorMsg = data.content || t('chat.processError');
                message.content = message.content || errorMsg;
                markAssistantFailed(errorMsg);
                MessagePlugin.error(errorMsg);
                console.error('[Chat Error]', errorMsg);
            }
            break;
            

        case 'references':
            // 知识引用
            if (data.data?.references) {
                message.knowledge_references = data.data.references;
            } else if (data.knowledge_references) {
                // 兼容旧格式
                message.knowledge_references = data.knowledge_references;
            }
            break;
            
        case 'answer':
            // 最终答案
            message.thinking = false;

            // 只有当有实际内容时才追加，避免空内容覆盖
            if (data.content) {
                message.content = (message.content || '') + data.content;
                fullContent.value += data.content;
            }
            
            // Add or update answer event in agentEventStream
            if (!message.agentEventStream) message.agentEventStream = [];
            
            let answerEvent = message.agentEventStream.find((e) => e.type === 'answer');
            if (!answerEvent) {
                answerEvent = {
                    type: 'answer',
                    content: '',
                    done: false,
                    completion_status: 'pending'
                };
                message.agentEventStream.push(answerEvent);
            }
            
            // 只有当有实际内容时才更新 answerEvent.content
            if (data.content) {
                answerEvent.content = message.content;
            }

            // 检查是否为 fallback 回答
            if (data.data?.is_fallback) {
                answerEvent.is_fallback = true;
                message.is_fallback = true;
            }

            if (data.data?.completion_status) {
                answerEvent.completion_status = data.data.completion_status;
                answerEvent.finish_reason = data.data.finish_reason;
                answerEvent.failure_reason = data.data.failure_reason;
            }

            // 只在第一次收到 done:true 时标记 answer 流结束，真正完成态由 complete 事件决定。
            if (data.done && !answerEvent.done) {
                answerEvent.done = true;
            }
            break;
            
        case 'complete':
            // 整个流式响应完成事件 - 确保状态正确关闭
            {
                const completePayload = { ...(data.data || {}) };
                if (completePayload.completion_status && completePayload.completion_status !== 'failed' && completePayload.failure_reason === undefined) {
                    completePayload.failure_reason = '';
                }
                syncMessageCompletionState(message, completePayload);
                if (completePayload.chat_document_artifact) {
                    promoteCompletedArtifactAsBase(message, completePayload.chat_document_artifact);
                }
                if (completePayload.final_answer && completePayload.final_answer !== message.content) {
                    message.content = completePayload.final_answer;
                    if (!message.agentEventStream) message.agentEventStream = [];
                    let answerEvent = message.agentEventStream.find((event) => event.type === 'answer');
                    if (!answerEvent) {
                        answerEvent = {
                            type: 'answer',
                            content: '',
                            done: false,
                            completion_status: completePayload.completion_status || 'completed'
                        };
                        message.agentEventStream.push(answerEvent);
                    }
                    answerEvent.content = completePayload.final_answer;
                    answerEvent.done = true;
                    answerEvent.completion_status = completePayload.completion_status || answerEvent.completion_status;
                    answerEvent.finish_reason = completePayload.finish_reason;
                    answerEvent.failure_reason = completePayload.failure_reason;
                }
                if (completePayload.document_generation_status === 'completed') {
                    message.content = stripDocumentCompleteMarker(message.content || completePayload.final_answer || '');
                    if (message.agentEventStream) {
                        const answerEvent = message.agentEventStream.find((event) => event.type === 'answer');
                        if (answerEvent?.content) {
                            answerEvent.content = stripDocumentCompleteMarker(answerEvent.content);
                        }
                    }
                }
                resetReplyState();
                // 将 total_duration_ms 存入事件流供 AgentStreamDisplay 使用
                if (completePayload) {
                    upsertAgentCompleteEvent(message, completePayload);
                }
                hydrateFinalDocumentFromComplete(message, completePayload)
                    .then((artifact) => maybeContinueDocumentAutomatically(message, completePayload, artifact))
                    .catch((error) => {
                        console.warn('[AutoContinueDocument] Failed to schedule next continuation:', error);
                        stopAutoContinue(error?.message || '自动续写调度失败');
                    });
            }
            break;
            
        case 'stop':
            // 停止事件 - 添加到事件流并标记对话完成
            if (!message.agentEventStream) message.agentEventStream = [];
            syncMessageCompletionState(message, {
                completion_status: 'cancelled',
                finish_reason: data.data?.reason || 'cancelled',
                failure_reason: data.data?.reason || 'cancelled'
            });
            
            // Add stop event to stream
            message.agentEventStream.push({
                type: 'stop',
                timestamp: Date.now(),
                reason: data.data?.reason || 'user_requested'
            });
            
            // Mark conversation as stopped
            resetReplyState();
            break;
    }
    
    scrollToBottom();
};

const updateAssistantSession = (payload) => {
    const message = messagesList.findLast((item) => {
        if (item.request_id === payload.id) {
            return true
        }
        return item.id === payload.id;
    });
    if (message) {
        const shouldPreserveContent =
            payload.content === '' &&
            typeof message.content === 'string' &&
            message.content.length > 0;
        if (!shouldPreserveContent && payload.content !== undefined) {
            message.content = payload.content;
        }
        message.thinking = payload.thinking;
        message.thinkContent = payload.thinkContent;
        message.showThink = payload.showThink;
        message.knowledge_references = message.knowledge_references ? message.knowledge_references : payload.knowledge_references;
        // 更新 fallback 状态
        if (payload.is_fallback) {
            message.is_fallback = true;
        }
        syncMessageCompletionState(message, payload);
        if (payload.error_message) {
            message.error_message = payload.error_message;
        }
        if (payload.chat_document_artifact) {
            assignChatDocumentArtifactToMessage(message, payload.chat_document_artifact);
        }
    } else {
        if (payload.chat_document_artifact) {
            payload.chat_document_artifact = upsertChatDocumentArtifact(payload.chat_document_artifact);
        }
        messagesList.push(payload);
    }
    scrollToBottom();
}
const handleSessionCleared = (e) => {
    if (e.detail?.sessionId === session_id.value) {
        messagesList.splice(0);
        created_at.value = '';
    }
};

onMounted(async () => {
    window.addEventListener('session-messages-cleared', handleSessionCleared);
    messagesList.splice(0);
    
    // 若从智能体列表点击共享智能体进入，URL 带 agent_id 与 source_tenant_id，同步到 store
    const agentIdFromQuery = effectiveAgentId.value || (route.query.agent_id && String(route.query.agent_id));
    const sourceTenantIdFromQuery = route.query.source_tenant_id && String(route.query.source_tenant_id);
    if (!isSharePageMode.value && agentIdFromQuery && sourceTenantIdFromQuery) {
        useSettingsStoreInstance.selectAgent(agentIdFromQuery, sourceTenantIdFromQuery);
    } else if (!isSharePageMode.value && agentIdFromQuery) {
        useSettingsStoreInstance.selectAgent(agentIdFromQuery, null);
    }
    
    if (!isSharePageMode.value && effectiveKBIds.value.length > 0) {
        useSettingsStoreInstance.selectKnowledgeBases(effectiveKBIds.value);
    }
    
    // 初始化状态：加载历史消息时不应显示loading
    loading.value = false;
    isReplying.value = false;
    
    // Load session data to get agent_config
    if (!isSharePageMode.value) {
        try {
            const sessionRes = await getSession(session_id.value);
            if (sessionRes?.data) {
                sessionData.value = sessionRes.data;
            }
        } catch (error) {
            console.error('Failed to load session data:', error);
        }
    }
    
    if (!isSharePageMode.value) {
        checkmenuTitle(session_id.value)
    }
    if (firstQuery.value) {
        scrollLock.value = true;
        historyLoading.value = false;
         sendMsg(firstQuery.value, firstModelId.value || '', firstMentionedItems.value || [], firstImageFiles.value || [], firstAttachmentFiles.value || []);
        usemenuStore.changeFirstQuery('', [], '', [], []);
    } else {
        scrollLock.value = false;
        let data = {
            session_id: session_id.value,
            created_at: '',
            limit: limit.value
        }
        getmsgList(data)
    }

    // 初始加载推荐问题
    fetchSuggestedQuestions();
})
const clearData = () => {
    stopStream();
    resetAutoContinue();
    isReplying.value = false;
    fullContent.value = '';
    userquery.value = '';

}
onUnmounted(() => {
    window.removeEventListener('session-messages-cleared', handleSessionCleared);
});
onBeforeRouteLeave((to, from, next) => {
    clearData()
    next()
})
onBeforeRouteUpdate((to, from, next) => {
    clearData()
    next()
})
</script>
<style lang="less" scoped>
.chat {
    font-size: 20px;
    padding: 20px;
    box-sizing: border-box;
    flex: 1;
    // The parent .platform-route-outlet is a flex column with min-height:0
    // and overflow:hidden — we also need min-height:0 here so that our
    // own flex:1 child (.chat_scroll_box) can shrink below its content
    // height and scroll instead of pushing .input-container out of view.
    min-height: 0;
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: center;
    max-width: calc(100vw - 260px);
    min-width: 400px;

    &.is-sidebar-collapsed {
        max-width: calc(100vw - 60px);
    }

    &.is-embedded {
        max-width: 100%;
        min-width: 100%;
        padding: 0;
        overflow-x: hidden;
    }

    &.is-embedded :deep(.answers-input) {
        transform: translateX(0);
        width: 100%;
        left: 0;
        display: flex;
        justify-content: center;
    }

    :deep(.answers-input) {
        position: static;
        transform: translateX(0);

        .t-textarea__inner {
            width: 100% !important;
        }
    }
}

.chat_scroll_box {
    flex: 1;
    // Without min-height: 0, a flex-column child defaults to min-height: auto
    // and expands to fit all inner content. When there are many messages,
    // that pushes .input-container out of the viewport. Clamping min-height
    // to 0 lets overflow-y: auto take effect so the messages scroll inside
    // this box instead of stretching it.
    min-height: 0;
    width: 100%;
    overflow-y: auto;

    &::-webkit-scrollbar {
        width: 0;
        height: 0;
        color: transparent;
    }
}

.scroll-to-bottom-btn {
    position: absolute;
    left: 50%;
    transform: translateX(-50%);
    bottom: 140px;
    z-index: 10;
    width: 36px;
    height: 36px;
    border-radius: 50%;
    background: var(--td-bg-color-container);
    border: 1px solid var(--td-component-stroke);
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    color: var(--td-text-color-secondary);
    transition: all 0.2s ease;

    &:hover {
        background: var(--td-bg-color-container-hover);
        color: var(--td-text-color-primary);
        box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
    }

    &:active {
        transform: translateX(-50%) scale(0.92);
    }
}

.scroll-btn-fade-enter-active,
.scroll-btn-fade-leave-active {
    transition: opacity 0.2s ease, transform 0.2s ease;
}
.scroll-btn-fade-enter-from,
.scroll-btn-fade-leave-to {
    opacity: 0;
    transform: translateX(-50%) translateY(8px);
}

.agent-mode-indicator {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 16px;
    background: var(--td-brand-color-light);
    border: 1px solid var(--td-brand-color-focus);
    border-radius: 6px;
    margin-bottom: 12px;
    max-width: 800px;
    width: 100%;

    .agent-icon {
        font-size: 20px;
    }

    .agent-text {
        font-size: 14px;
        font-weight: 500;
        color: var(--td-brand-color);
        flex: 1;
    }
}

@keyframes contentFadeIn {
    from { opacity: 0; transform: translateY(6px); }
    to { opacity: 1; transform: translateY(0); }
}

.msg-skeleton-list {
    display: flex;
    flex-direction: column;
    gap: 20px;
    max-width: 800px;
    padding: 16px 0;
    animation: contentFadeIn 0.3s ease-out;
}
.msg-skeleton-user {
    display: flex;
    justify-content: flex-end;
}
.msg-skeleton-bot {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding-left: 4px;
}

.input-container {
    min-height: 115px;
    // Keep the input visible when messages overflow: without flex-shrink: 0
    // a tall .chat_scroll_box can squeeze this container down to 0 height.
    flex-shrink: 0;
    margin: 16px auto 4px;
    width: 100%;
    max-width: 800px;
    box-sizing: border-box;

    &.is-embedded {
        max-width: 100%;
        width: 100%;
        margin: 0;
        overflow-x: hidden;
    }
}

.document-baseline-banner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 10px 14px;
    margin-bottom: 10px;
    border-radius: 10px;
    border: 1px solid color-mix(in srgb, var(--td-brand-color) 18%, transparent);
    background: linear-gradient(180deg, color-mix(in srgb, var(--td-brand-color) 7%, var(--td-bg-color-container)) 0%, var(--td-bg-color-container) 100%);
}

.document-baseline-text {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    min-width: 0;
    font-size: 13px;
    color: var(--td-text-color-primary);
}

.document-baseline-label {
    color: var(--td-text-color-secondary);
}

.document-baseline-title {
    font-weight: 600;
    word-break: break-word;
}

.document-baseline-version {
    color: var(--td-brand-color);
}

.auto-continue-banner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 8px 12px;
    margin-bottom: 10px;
    border-radius: 8px;
    border: 1px solid color-mix(in srgb, var(--td-success-color) 20%, transparent);
    background: color-mix(in srgb, var(--td-success-color) 8%, var(--td-bg-color-container));
    color: var(--td-text-color-primary);
    font-size: 13px;
}

.auto-continue-banner.is-stopped {
    border-color: var(--td-component-border);
    background: var(--td-bg-color-secondarycontainer);
    color: var(--td-text-color-secondary);
}

.artifact-drawer-body {
    display: flex;
    flex-direction: column;
    gap: 12px;
    min-height: 180px;
}

.artifact-drawer-loading,
.artifact-drawer-empty {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    min-height: 160px;
    color: var(--td-text-color-secondary);
}

.artifact-drawer-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
}

.artifact-drawer-item {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 14px;
    border-radius: 12px;
    border: 1px solid var(--td-component-stroke);
    background: var(--td-bg-color-container);

    &.is-selected {
        border-color: color-mix(in srgb, var(--td-brand-color) 40%, transparent);
        box-shadow: 0 8px 24px color-mix(in srgb, var(--td-brand-color) 10%, transparent);
    }

    &.is-current {
        background: color-mix(in srgb, var(--td-brand-color) 5%, var(--td-bg-color-container));
    }
}

.artifact-drawer-item-top {
    display: flex;
    justify-content: space-between;
    gap: 12px;
}

.artifact-drawer-item-title {
    font-size: 14px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    word-break: break-word;
}

.artifact-drawer-item-tags {
    display: flex;
    gap: 6px;
    flex-shrink: 0;
}

.artifact-drawer-item-meta {
    font-size: 12px;
    color: var(--td-text-color-secondary);
}

.artifact-drawer-item-hint {
    font-size: 12px;
    line-height: 1.5;
    color: var(--td-warning-color-7);
}

.artifact-drawer-item-actions {
    display: flex;
    justify-content: flex-end;
}

.msg_list {
    display: flex;
    flex-direction: column;
    gap: 16px;
    max-width: 800px;
    flex: 1;
    margin: 0 auto;
    width: 100%;

    .botanswer_laoding_gif {
        width: 24px;
        height: 18px;
        margin-left: 16px;
    }
    
    .loading-typing {
        display: flex;
        align-items: center;
        gap: 4px;
        
        span {
            width: 6px;
            height: 6px;
            border-radius: 50%;
            background: var(--td-brand-color);
            animation: typingBounce 1.4s ease-in-out infinite;
            
            &:nth-child(1) {
                animation-delay: 0s;
            }
            
            &:nth-child(2) {
                animation-delay: 0.2s;
            }
            
            &:nth-child(3) {
                animation-delay: 0.4s;
            }
        }
    }
}

@keyframes typingBounce {
    0%, 60%, 100% {
        transform: translateY(0);
    }
    30% {
        transform: translateY(-8px);
    }
}

.suggested-questions-container {
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 32px 16px 16px;
    max-width: 800px;
    margin: 0 auto;
    width: 100%;
    min-height: 0;
    transition: min-height 0.3s ease;

    &.has-questions {
        min-height: 80px;
    }
}

.suggested-questions-inner {
    display: flex;
    flex-direction: column;
    align-items: center;
    width: 100%;
    animation: contentFadeIn 0.3s ease-out;
}

.sq-fade-enter-active,
.sq-fade-leave-active {
    transition: opacity 0.25s ease;
}
.sq-fade-enter-from,
.sq-fade-leave-to {
    opacity: 0;
}

.suggested-questions-title {
    font-size: 14px;
    color: var(--td-text-color-secondary);
    margin-bottom: 16px;
    font-weight: 500;
}

.suggested-questions-grid {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    justify-content: center;
    width: 100%;
}

.suggested-question-card {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 10px 16px;
    border-radius: 20px;
    border: 1px solid var(--td-component-stroke);
    background: var(--td-bg-color-container);
    cursor: pointer;
    transition: all 0.2s ease;
    max-width: 100%;

    &:hover {
        border-color: var(--td-brand-color);
        background: var(--td-brand-color-light);
        box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
    }
}

.suggested-question-text {
    font-size: 13px;
    color: var(--td-text-color-primary);
    line-height: 1.4;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.suggested-question-badge {
    font-size: 10px;
    padding: 1px 5px;
    border-radius: 4px;
    flex-shrink: 0;
    font-weight: 500;

    &.faq {
        background: var(--td-success-color-1);
        color: var(--td-success-color);
    }
}
</style>
