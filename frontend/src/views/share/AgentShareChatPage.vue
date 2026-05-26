<template>
  <div class="agent-share-page">
    <div class="agent-share-shell">
      <AgentShareEmptyState
        v-if="loading"
        icon="..."
        title="正在打开分享页"
        description="正在解析分享信息并恢复匿名会话，请稍候。"
      />

      <AgentShareEmptyState
        v-else-if="errorMessage"
        icon="!"
        title="分享页暂不可用"
        :description="errorMessage"
        action-text="重试"
        @action="initializePage"
      />

      <CreateChatView
        v-else-if="showCreateChatLanding"
        :embeddedMode="true"
        :runtimeContext="runtimeContext"
        :suggestedQuestionsOverride="runtimeSuggestedQuestions"
        @send-msg="handleCreateChatSend"
        @model-change="handleShareModelChange"
      />

      <div v-else-if="activeSessionId && runtimeContext" class="agent-share-chat-container">
        <ChatView
          :key="activeSessionId"
          :session_id="activeSessionId"
          :embeddedMode="true"
          :runtimeContext="runtimeContext"
          @model-change="handleShareModelChange"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRoute } from 'vue-router';

import CreateChatView from '@/views/creatChat/creatChat.vue';
import ChatView from '@/views/chat/index.vue';
import AgentShareEmptyState from '@/views/share/components/AgentShareEmptyState.vue';
import {
  clearStoredAgentShareSession,
  createPublicAgentShareSession,
  getPublicAgentShare,
  getPublicAgentShareMessages,
  getStoredAgentShareSession,
  setStoredAgentShareSession,
  type AgentSharePublicInfo,
  type StoredAgentShareSession,
} from '@/api/agent-share';
import type { ChatRuntimeContext } from '@/types/chat-runtime';
import { useMenuStore } from '@/stores/menu';

const route = useRoute();
const menuStore = useMenuStore();

const loading = ref(true);
const errorMessage = ref('');
const publicInfo = ref<AgentSharePublicInfo | null>(null);
const shareSession = ref<StoredAgentShareSession | null>(null);
const hasMessages = ref(false);
const selectedModelId = ref('');

const shareCode = computed(() => String(route.params.shareCode || '').trim());
const activeSessionId = computed(() => shareSession.value?.sessionId || '');
const runtimeSuggestedQuestions = computed(() => (publicInfo.value?.suggested_questions || []).map((question) => ({
  question,
  source: 'agent_config' as const,
})));
const showCreateChatLanding = computed(() => Boolean(activeSessionId.value && runtimeContext.value) && !hasMessages.value);

const runtimeContext = computed<ChatRuntimeContext | null>(() => {
  if (!publicInfo.value || !shareSession.value) {
    return null;
  }

  const runtime = publicInfo.value.runtime;
  return {
    mode: 'agent-share-page',
    shareCode: shareCode.value,
    shareSessionToken: shareSession.value.visitorToken,
    fixedAgentId: publicInfo.value.agent.id,
    fixedAgentName: publicInfo.value.agent.name,
    fixedAgentMode: runtime.agent_mode,
    fixedKnowledgeBaseIds: [],
    fixedAgent: {
      id: publicInfo.value.agent.id,
      name: publicInfo.value.agent.name,
      description: publicInfo.value.agent.description,
      avatar: publicInfo.value.agent.avatar,
      is_builtin: false,
      config: {
        agent_mode: runtime.agent_mode as 'quick-answer' | 'smart-reasoning',
        image_upload_enabled: runtime.image_upload_enabled,
        audio_upload_enabled: runtime.audio_upload_enabled,
        supported_file_types: runtime.supported_file_types || [],
        web_search_enabled: runtime.web_search_enabled,
        multi_turn_enabled: runtime.multi_turn_enabled,
      },
    },
    publicChatApiBase: `/api/v1/public/agent-page-shares/${encodeURIComponent(shareCode.value)}`,
    allowAgentSwitch: false,
    allowKnowledgeBaseSelect: false,
    allowModelSelect: true,
    allowWebSearchToggle: false,
    allowSettingsNavigation: false,
    allowCommandPalette: false,
    allowSessionListNavigation: false,
    allowConversationHistoryNavigation: false,
    webSearchEnabled: runtime.web_search_enabled,
    multiTurnEnabled: runtime.multi_turn_enabled,
    imageUploadEnabled: runtime.image_upload_enabled,
    audioUploadEnabled: runtime.audio_upload_enabled,
    attachmentUploadEnabled: runtime.attachment_upload_enabled,
    supportedFileTypes: runtime.supported_file_types || [],
    defaultModelId: selectedModelId.value || runtime.default_model_id || '',
    defaultModelName: runtime.default_model_name || '',
    availableModels: (runtime.available_models || []).map((model) => ({
      id: model.id,
      name: model.name,
      type: model.type,
      source: model.source,
      description: model.description,
      parameters: {
        parameter_size: model.parameters?.parameter_size,
      },
      is_default: model.is_default,
      status: model.status,
    })),
    suggestedQuestions: (publicInfo.value.suggested_questions || []).map((question) => ({
      question,
      source: 'agent_config' as const,
    })),
  };
});

const resolveErrorMessage = (error: any) => {
  if (error?.status === 404) {
    return '分享链接不存在、已关闭或已过期。';
  }
  if (error?.status === 410) {
    return '当前匿名会话已过期，请刷新页面重新创建会话。';
  }
  return error?.message || '当前分享页暂时无法打开，请稍后重试。';
};

const ensureAnonymousSession = async () => {
  const stored = getStoredAgentShareSession(shareCode.value);
  if (stored) {
    try {
      const response = await getPublicAgentShareMessages(shareCode.value, stored.sessionId, stored.visitorToken, { limit: 1 });
      hasMessages.value = Array.isArray(response?.data) && response.data.length > 0;
      shareSession.value = stored;
      return;
    } catch (error: any) {
      if ([403, 404, 410].includes(Number(error?.status))) {
        clearStoredAgentShareSession(shareCode.value);
      } else {
        throw error;
      }
    }
  }

  const created = await createPublicAgentShareSession(shareCode.value);
  const nextSession: StoredAgentShareSession = {
    sessionId: created.data.session_id,
    anonymousVisitorId: created.data.anonymous_visitor_id,
    visitorToken: created.data.visitor_token,
    expiresAt: created.data.expires_at,
  };
  setStoredAgentShareSession(shareCode.value, nextSession);
  shareSession.value = nextSession;
  hasMessages.value = false;
};

const handleCreateChatSend = (value: string, modelId: string, mentionedItems: any[], imageFiles: File[] = [], attachmentFiles: any[] = []) => {
  menuStore.changeFirstQuery(value, mentionedItems, modelId, imageFiles, attachmentFiles);
  hasMessages.value = true;
};

const handleShareModelChange = (modelId: string) => {
  selectedModelId.value = String(modelId || '').trim();
};

const initializePage = async () => {
  if (!shareCode.value) {
    publicInfo.value = null;
    shareSession.value = null;
    hasMessages.value = false;
    selectedModelId.value = '';
    errorMessage.value = '分享码不能为空。';
    loading.value = false;
    return;
  }

  loading.value = true;
  errorMessage.value = '';
  publicInfo.value = null;
  shareSession.value = null;
  hasMessages.value = false;
  selectedModelId.value = '';
  menuStore.changeFirstQuery('', [], '', [], []);

  try {
    const info = await getPublicAgentShare(shareCode.value);
    publicInfo.value = info.data;
    await ensureAnonymousSession();
  } catch (error: any) {
    publicInfo.value = null;
    errorMessage.value = resolveErrorMessage(error);
  } finally {
    loading.value = false;
  }
};

watch(shareCode, () => {
  initializePage();
}, { immediate: true });
</script>

<style scoped lang="less">
.agent-share-page {
  min-height: 100vh;
  min-height: 100dvh;
  padding: 24px;
  background:
    radial-gradient(circle at top left, rgba(7, 192, 95, 0.12), transparent 28%),
    radial-gradient(circle at top right, rgba(15, 23, 42, 0.1), transparent 24%),
    linear-gradient(180deg, #f8fafc 0%, #eef2f7 100%);
}

.agent-share-shell {
  min-height: calc(100vh - 48px);
  min-height: calc(100dvh - 48px);
  border-radius: 28px;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.82);
  border: 1px solid rgba(15, 23, 42, 0.08);
  box-shadow: 0 24px 60px rgba(15, 23, 42, 0.12);
  backdrop-filter: blur(18px);
}

.agent-share-chat-container {
  min-height: calc(100vh - 180px);
  min-height: calc(100dvh - 180px);
}

@media (max-width: 900px) {
  .agent-share-page {
    min-height: 100dvh;
    padding:
      max(12px, env(safe-area-inset-top))
      max(12px, env(safe-area-inset-right))
      max(12px, env(safe-area-inset-bottom))
      max(12px, env(safe-area-inset-left));
    background: #f8fafc;
  }

  .agent-share-shell {
    min-height: calc(100dvh - env(safe-area-inset-top) - env(safe-area-inset-bottom));
    border-radius: 24px;
    border: 0;
    box-shadow: 0 12px 36px rgba(15, 23, 42, 0.08);
    backdrop-filter: none;
  }

  .agent-share-chat-container {
    min-height: calc(100dvh - 164px - env(safe-area-inset-top) - env(safe-area-inset-bottom));
  }
}

@media (max-width: 640px) {
  .agent-share-page {
    padding:
      max(8px, env(safe-area-inset-top))
      max(8px, env(safe-area-inset-right))
      max(8px, env(safe-area-inset-bottom))
      max(8px, env(safe-area-inset-left));
  }

  .agent-share-shell {
    border-radius: 18px;
  }

  .agent-share-chat-container {
    min-height: calc(100dvh - 140px - env(safe-area-inset-top) - env(safe-area-inset-bottom));
  }
}
</style>