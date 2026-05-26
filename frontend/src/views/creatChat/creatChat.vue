<template>
    <div class="dialogue-wrap" :class="{ 'is-embedded': embeddedMode }">
        <div class="dialogue-answers" :class="{ 'is-embedded': embeddedMode }">
            <div class="dialogue-title" :class="{ 'is-embedded': embeddedMode }" style="--wails-draggable: drag">
                <span style="--wails-draggable: drag">{{ displayTitle }}</span>
            </div>
            <!-- 推荐问题 -->
            <div ref="sqContainerRef" class="suggested-questions-container">
                <!-- 骨架屏占位 -->
                <div v-if="sqLoading && suggestedQuestions.length === 0" class="suggested-questions-inner">
                    <div class="suggested-questions-title"><t-skeleton animation="gradient" :row-col="[{ width: '120px', height: '18px' }]" /></div>
                    <div class="suggested-questions-grid">
                        <div v-for="n in 6" :key="'sq-skel-'+n" class="suggested-question-card sq-card-skeleton">
                            <t-skeleton animation="gradient" :row-col="[{ width: '90%', height: '14px' }, { width: '60%', height: '14px' }]" />
                        </div>
                    </div>
                </div>
                <transition v-else appear name="sq-slide-fade" mode="out-in"
                    @before-leave="onBeforeLeave"
                    @after-leave="onAfterLeave"
                    @enter="onEnter"
                    @after-enter="onQuestionsEntered"
                >
                    <div v-if="suggestedQuestions.length > 0" :key="sqRenderKey" class="suggested-questions-inner">
                        <div class="suggested-questions-title">{{ $t('chat.suggestedQuestions') }}</div>
                        <div class="suggested-questions-grid">
                            <div
                                v-for="(item, index) in suggestedQuestions"
                                :key="item.question"
                                class="suggested-question-card"
                                :class="{ 'sq-card-visible': sqCardsRevealed }"
                                :style="{ transitionDelay: sqCardsRevealed ? `${index * 50}ms` : '0ms' }"
                                @click="handleSuggestedQuestionClick(item.question)"
                            >
                                <span class="suggested-question-text">{{ item.question }}</span>
                                <span v-if="item.source === 'faq'" class="suggested-question-badge faq">FAQ</span>
                            </div>
                        </div>
                    </div>
                </transition>
            </div>
            <InputField ref="inputFieldRef" :embeddedMode="embeddedMode" :runtimeContext="runtimeContext" @send-msg="sendMsg" @model-change="handleModelChange"></InputField>
        </div>
    </div>
    
    <!-- 知识库编辑器（创建/编辑统一组件） -->
    <KnowledgeBaseEditorModal 
      :visible="uiStore.showKBEditorModal"
      :mode="uiStore.kbEditorMode"
      :kb-id="uiStore.currentKBId || undefined"
      :initial-type="uiStore.kbEditorType"
      @update:visible="(val) => val ? null : uiStore.closeKBEditor()"
      @success="handleKBEditorSuccess"
    />
</template>
<script setup lang="ts">
import { computed, ref, watch, onMounted, nextTick, type PropType } from 'vue';
import InputField from '@/components/Input-field.vue';
import { createSessions } from "@/api/chat/index";
import { getSuggestedQuestions } from "@/api/agent/index";
import type { SuggestedQuestion } from "@/api/agent/index";
import { useMenuStore } from '@/stores/menu';
import { useSettingsStore } from '@/stores/settings';
import { useUIStore } from '@/stores/ui';
import { useRouter } from 'vue-router';
import { MessagePlugin } from 'tdesign-vue-next';
import { useI18n } from 'vue-i18n';
import KnowledgeBaseEditorModal from '@/views/knowledge/KnowledgeBaseEditorModal.vue';
import { useKnowledgeBaseCreationNavigation } from '@/hooks/useKnowledgeBaseCreationNavigation';
import { isAgentSharePageRuntimeContext, type ChatRuntimeContext, type ChatRuntimeSuggestedQuestion } from '@/types/chat-runtime';

const props = defineProps({
    embeddedMode: {
        type: Boolean,
        default: false,
    },
    runtimeContext: {
        type: Object as PropType<ChatRuntimeContext | null>,
        default: null,
    },
    suggestedQuestionsOverride: {
        type: Array as PropType<ChatRuntimeSuggestedQuestion[]>,
        default: () => [],
    },
});

const emit = defineEmits<{
    (e: 'send-msg', query: string, modelId: string, mentionedItems: any[], imageFiles: File[], attachmentFiles: any[]): void;
    (e: 'model-change', modelId: string): void;
}>();

const router = useRouter();
const usemenuStore = useMenuStore();
const settingsStore = useSettingsStore();
const uiStore = useUIStore();
const { t } = useI18n();
const { navigateToKnowledgeBaseList } = useKnowledgeBaseCreationNavigation();
const runtimeContext = computed(() => props.runtimeContext || null);
const isSharePageMode = computed(() => isAgentSharePageRuntimeContext(runtimeContext.value));
const displayTitle = computed(() => {
    const defaultTitle = t('createChat.title');
    if (!isSharePageMode.value) {
        return defaultTitle;
    }

    const agentName = String(runtimeContext.value?.fixedAgentName || '').trim();
    if (!agentName) {
        return defaultTitle;
    }

    const titleParts = defaultTitle.split(' - ');
    if (titleParts.length >= 2) {
        return `${titleParts[0]} - ${agentName}`;
    }

    return agentName;
});

// ===== 推荐问题 =====
const suggestedQuestions = ref<SuggestedQuestion[]>([]);
const sqLoading = ref(true);
const sqCardsRevealed = ref(false);
const sqRenderKey = ref(0);
const sqContainerRef = ref<HTMLElement | null>(null);
let suggestedQuestionsFetchId = 0;
let debounceTimer: ReturnType<typeof setTimeout> | null = null;

// --- 高度平滑过渡钩子 ---
const onBeforeLeave = () => {
    const c = sqContainerRef.value;
    if (!c) return;
    c.style.height = c.offsetHeight + 'px';
    c.style.overflow = 'hidden';
};

const onAfterLeave = () => {
    const c = sqContainerRef.value;
    if (!c) return;
    if (suggestedQuestions.value.length === 0) {
        requestAnimationFrame(() => { c.style.height = '0px'; });
        c.addEventListener('transitionend', () => {
            c.style.height = '';
            c.style.overflow = '';
        }, { once: true });
    }
};

const onEnter = (el: Element) => {
    const c = sqContainerRef.value;
    if (!c) return;
    const startHeight = c.offsetHeight;
    c.style.height = 'auto';
    c.style.overflow = 'hidden';
    const targetHeight = c.offsetHeight;
    c.style.height = startHeight + 'px';
    requestAnimationFrame(() => {
        c.style.height = targetHeight + 'px';
    });
};

const onQuestionsEntered = () => {
    const c = sqContainerRef.value;
    if (c) {
        c.style.height = '';
        c.style.overflow = '';
    }
    nextTick(() => { sqCardsRevealed.value = true; });
};

const syncRuntimeSuggestedQuestions = () => {
    sqCardsRevealed.value = false;
    sqRenderKey.value++;
    suggestedQuestions.value = (props.suggestedQuestionsOverride || []).map((item) => {
        return {
            question: item.question,
            source: item.source === 'document' || item.source === 'faq' || item.source === 'wiki' || item.source === 'agent_config'
                ? item.source
                : 'agent_config',
        };
    });
    sqLoading.value = false;
};

const fetchSuggestedQuestions = async () => {
    if (isSharePageMode.value) {
        syncRuntimeSuggestedQuestions();
        return;
    }
    const fetchId = ++suggestedQuestionsFetchId;
    if (suggestedQuestions.value.length === 0) sqLoading.value = true;
    try {
        const agentId = settingsStore.selectedAgentId;
        if (!agentId) return;
        const selectedKBs = settingsStore.getSelectedKnowledgeBases();
        const selectedFiles = settingsStore.getSelectedFiles();
        const res = await getSuggestedQuestions(agentId, {
            knowledge_base_ids: selectedKBs.length > 0 ? selectedKBs : undefined,
            knowledge_ids: selectedFiles.length > 0 ? selectedFiles : undefined,
            limit: 6,
        });
        if (fetchId === suggestedQuestionsFetchId) {
            sqCardsRevealed.value = false;
            sqRenderKey.value++;
            suggestedQuestions.value = res?.data?.questions || [];
        }
    } catch (err) {
        console.warn('[SuggestedQuestions] Failed to fetch:', err);
        if (fetchId === suggestedQuestionsFetchId) {
            suggestedQuestions.value = [];
        }
    } finally {
        if (fetchId === suggestedQuestionsFetchId) {
            sqLoading.value = false;
        }
    }
};

// 防抖包装，切换知识库/文件时300ms内不重复请求
const debouncedFetch = () => {
    if (isSharePageMode.value) {
        syncRuntimeSuggestedQuestions();
        return;
    }
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => { fetchSuggestedQuestions(); }, 300);
};

// 监听 Agent / 知识库 / 文件切换
watch(() => settingsStore.selectedAgentId, debouncedFetch);
watch(() => settingsStore.settings.selectedKnowledgeBases, debouncedFetch, { deep: true });
watch(() => settingsStore.settings.selectedFiles, debouncedFetch, { deep: true });
watch(() => props.suggestedQuestionsOverride, () => {
    if (isSharePageMode.value) {
        syncRuntimeSuggestedQuestions();
    }
}, { deep: true });
watch(runtimeContext, () => {
    if (isSharePageMode.value) {
        syncRuntimeSuggestedQuestions();
    }
}, { deep: true });

onMounted(() => { fetchSuggestedQuestions(); });

const inputFieldRef = ref();

const handleSuggestedQuestionClick = (question: string) => {
    inputFieldRef.value?.triggerSend(question);
};

const handleModelChange = (modelId: string) => {
    emit('model-change', modelId);
};

const sendMsg = (value: string, modelId: string, mentionedItems: any[], imageFiles: any[] = [], attachmentFiles: any[] = []) => {
    if (isSharePageMode.value) {
        emit('send-msg', value, modelId, mentionedItems, imageFiles, attachmentFiles);
        return;
    }
    createNewSession(value, modelId, mentionedItems, imageFiles, attachmentFiles);
}

async function createNewSession(value: string, modelId: string, mentionedItems: any[] = [], imageFiles: any[] = [], attachmentFiles: any[] = []) {
    const selectedKbs = settingsStore.settings.selectedKnowledgeBases || [];
    const selectedFiles = settingsStore.settings.selectedFiles || [];

    // 构建 session 数据，包含 Agent 配置
    const sessionData: any = {};
    
    // 添加 Agent 配置（知识库信息在 agent_config 中）
    sessionData.agent_config = {
        enabled: true,
        max_iterations: settingsStore.agentConfig.maxIterations,
        temperature: settingsStore.agentConfig.temperature,
        knowledge_bases: selectedKbs,  // 所有选中的知识库
        knowledge_ids: selectedFiles,  // 所有选中的普通知识/文件
        allowed_tools: settingsStore.agentConfig.allowedTools
    };

    try {
        const res = await createSessions(sessionData);
        if (res.data && res.data.id) {
            await navigateToSession(res.data.id, value, modelId, mentionedItems, imageFiles, attachmentFiles);
        } else {
            console.error('[createChat] Failed to create session');
            MessagePlugin.error(t('createChat.messages.createFailed'));
        }
    } catch (error) {
        console.error('[createChat] Create session error:', error);
        MessagePlugin.error(t('createChat.messages.createError'));
    }
}

const navigateToSession = async (sessionId: string, value: string, modelId: string, mentionedItems: any[], imageFiles: any[] = [], attachmentFiles: any[] = []) => {
    const now = new Date().toISOString();
    let obj = { 
        title: t('createChat.newSessionTitle'), 
        path: `chat/${sessionId}`, 
        id: sessionId, 
        isMore: false, 
        isNoTitle: true,
        created_at: now,
        updated_at: now
    };
    usemenuStore.updataMenuChildren(obj);
    usemenuStore.changeIsFirstSession(true);
    usemenuStore.changeFirstQuery(value, mentionedItems, modelId, imageFiles, attachmentFiles);
    router.push(`/platform/chat/${sessionId}`);
}

const handleKBEditorSuccess = (payload: string | { id: string }) => {
    navigateToKnowledgeBaseList(typeof payload === 'string' ? payload : payload.id)
}

</script>
<style lang="less" scoped>
.dialogue-wrap {
    flex: 1;
    display: flex;
    justify-content: center;
    align-items: center;
    width: 100%;

    &.is-embedded {
        align-items: stretch;
        padding: 0;
    }
}

.dialogue-answers {
    display: flex;
    flex-flow: column;
    align-items: center;
    width: 100%;
    max-width: 800px;

    &.is-embedded {
        max-width: 100%;
        min-height: 100%;
        justify-content: center;
        padding: 12px 0 0;
    }

    :deep(.answers-input) {
        position: static;
        transform: translateX(0);
    }
}

.dialogue-title {
    display: flex;
    color: var(--td-text-color-primary);
    font-family: var(--app-font-family);
    font-size: 28px;
    font-weight: 600;
    align-items: center;
    margin-bottom: 30px;

    &.is-embedded {
        width: 100%;
        justify-content: center;
        margin-bottom: 20px;
        text-align: center;
    }

    .icon {
        display: flex;
        width: 32px;
        height: 32px;
        justify-content: center;
        align-items: center;
        border-radius: 6px;
        background: var(--td-bg-color-container);
        box-shadow: var(--td-shadow-1);
        margin-right: 12px;

        .logo_img {
            height: 24px;
            width: 24px;
        }
    }
}

.suggested-questions-container {
    display: flex;
    flex-direction: column;
    align-items: center;
    margin-bottom: 24px;
    width: 100%;
    max-width: 800px;
    transition: height 0.35s cubic-bezier(0.4, 0, 0.2, 1);
}

@keyframes skeletonFadeIn {
    from { opacity: 0; }
    to { opacity: 1; }
}

.suggested-questions-inner {
    display: flex;
    flex-direction: column;
    align-items: center;
    width: 100%;
    animation: skeletonFadeIn 0.3s ease-out;
}

// 容器整体过渡：淡入 + 轻微上滑
.sq-slide-fade-enter-active {
    transition: opacity 0.35s cubic-bezier(0.4, 0, 0.2, 1),
                transform 0.35s cubic-bezier(0.4, 0, 0.2, 1);
}
.sq-slide-fade-leave-active {
    transition: opacity 0.15s cubic-bezier(0.4, 0, 1, 1),
                transform 0.15s cubic-bezier(0.4, 0, 1, 1);
}
.sq-slide-fade-enter-from {
    opacity: 0;
    transform: translateY(10px);
}
.sq-slide-fade-leave-to {
    opacity: 0;
    transform: translateY(-4px);
}

.suggested-questions-title {
    font-size: 14px;
    color: var(--td-text-color-secondary);
    margin-bottom: 12px;
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
    max-width: 100%;
    opacity: 0;
    transform: translateY(8px) scale(0.97);
    transition: opacity 0.35s cubic-bezier(0.4, 0, 0.2, 1),
                transform 0.35s cubic-bezier(0.4, 0, 0.2, 1),
                border-color 0.2s ease,
                background 0.2s ease,
                box-shadow 0.2s ease;

    &.sq-card-skeleton {
        opacity: 1;
        transform: none;
        cursor: default;
    }

    &.sq-card-visible {
        opacity: 1;
        transform: translateY(0) scale(1);
    }

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

@media (max-width: 1250px) and (min-width: 1045px) {
    .answers-input {
        transform: translateX(-329px);
    }

    :deep(.t-textarea__inner) {
        width: 654px !important;
    }
}

@media (max-width: 1045px) {
    .answers-input {
        transform: translateX(-250px);
    }

    :deep(.t-textarea__inner) {
        width: 500px !important;
    }
}
@media (max-width: 750px) {
    .dialogue-wrap {
        align-items: stretch;
    }

    .dialogue-answers {
        max-width: 100%;
    }

    .dialogue-title {
        font-size: 24px;
        margin-bottom: 22px;
        text-align: center;
    }

    .suggested-questions-container {
        margin-bottom: 16px;
    }

    .answers-input {
        transform: translateX(-250px);
    }

    :deep(.t-textarea__inner) {
        width: 340px !important;
    }
}
@media (max-width: 600px) {
    .dialogue-wrap {
        padding: 0 4px;
    }

    .dialogue-answers {
        padding-top: 8px;
    }

    .dialogue-title {
        font-size: 20px;
        line-height: 1.3;
        margin-bottom: 16px;
    }

    .suggested-questions-grid {
        gap: 8px;
        justify-content: flex-start;
    }

    .suggested-question-card {
        width: 100%;
        justify-content: space-between;
        padding: 10px 14px;
        border-radius: 16px;
    }

    .suggested-question-text {
        white-space: normal;
        overflow: visible;
        text-overflow: unset;
    }

    :deep(.answers-input) {
        width: 100%;
    }

    .answers-input {
        transform: none;
    }

    :deep(.t-textarea__inner) {
        width: 100% !important;
    }
}

@media (max-width: 600px) {
    .dialogue-wrap.is-embedded {
        padding: 0;
    }

    .dialogue-title.is-embedded {
        font-size: 18px;
        margin-bottom: 12px;
    }
}

</style>
<style lang="less">
.del-menu-popup {
    z-index: 99 !important;

    .t-popup__content {
        width: 100px;
        height: 40px;
        line-height: 30px;
        padding-left: 14px;
        cursor: pointer;
        margin-top: 4px !important;

    }
}
</style>