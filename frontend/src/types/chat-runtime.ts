import type { CustomAgent } from '@/api/agent';
import type { ModelConfig } from '@/api/model';

export interface ChatRuntimeSuggestedQuestion {
  question: string;
  source?: string;
}

export interface ChatRuntimeContext {
  mode: 'platform' | 'embedded' | 'agent-share-page';
  shareCode?: string;
  shareSessionToken?: string;
  fixedAgentId?: string;
  fixedAgentName?: string;
  fixedSourceTenantId?: string;
  fixedKnowledgeBaseIds?: string[];
  fixedAgentMode?: 'quick-answer' | 'smart-reasoning' | string;
  fixedAgent?: Partial<CustomAgent>;
  publicChatApiBase?: string;
  allowAgentSwitch: boolean;
  allowKnowledgeBaseSelect: boolean;
  allowModelSelect: boolean;
  allowWebSearchToggle: boolean;
  allowSettingsNavigation: boolean;
  allowCommandPalette: boolean;
  allowSessionListNavigation: boolean;
  allowConversationHistoryNavigation: boolean;
  webSearchEnabled?: boolean;
  multiTurnEnabled?: boolean;
  imageUploadEnabled?: boolean;
  audioUploadEnabled?: boolean;
  attachmentUploadEnabled?: boolean;
  supportedFileTypes?: string[];
  defaultModelId?: string;
  defaultModelName?: string;
  availableModels?: ModelConfig[];
  suggestedQuestions?: ChatRuntimeSuggestedQuestion[];
}

export function createPlatformChatRuntimeContext(): ChatRuntimeContext {
  return {
    mode: 'platform',
    allowAgentSwitch: true,
    allowKnowledgeBaseSelect: true,
    allowModelSelect: true,
    allowWebSearchToggle: true,
    allowSettingsNavigation: true,
    allowCommandPalette: true,
    allowSessionListNavigation: true,
    allowConversationHistoryNavigation: true,
  };
}

export function isAgentSharePageRuntimeContext(runtimeContext?: ChatRuntimeContext | null): boolean {
  return runtimeContext?.mode === 'agent-share-page';
}