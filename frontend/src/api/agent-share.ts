import { del, get, post } from '@/utils/request';

export interface AgentPageShareManagementView {
  id: string;
  agent_id: string;
  source_tenant_id: number;
  share_code: string;
  status: string;
  access_scope: string;
  share_url: string;
  anonymous_session_limit: number;
  rate_limit_per_minute: number;
  last_accessed_at?: string;
  expires_at?: string;
  created_at: string;
  updated_at: string;
}

export interface AgentSharePublicSummary {
  id: string;
  share_code: string;
  status: string;
  access_scope: string;
}

export interface AgentSharePublicAgentSummary {
  id: string;
  name: string;
  description?: string;
  avatar?: string;
}

export interface AgentSharePublicModelSummary {
  id: string;
  name: string;
  type: 'KnowledgeQA' | 'Embedding' | 'Rerank' | 'VLLM' | 'ASR';
  source: 'local' | 'remote';
  description?: string;
  parameters?: {
    parameter_size?: string;
  };
  is_default?: boolean;
  status?: string;
}

export interface AgentSharePublicRuntimeSummary {
  agent_mode: string;
  kb_selection_mode?: string;
  mcp_selection_mode?: string;
  web_search_enabled: boolean;
  multi_turn_enabled: boolean;
  image_upload_enabled: boolean;
  audio_upload_enabled: boolean;
  attachment_upload_enabled: boolean;
  supported_file_types?: string[];
  default_model_id?: string;
  default_model_name?: string;
  available_models?: AgentSharePublicModelSummary[];
  show_web_search_toggle: boolean;
  show_model_selector: boolean;
  show_kb_selector: boolean;
  show_agent_selector: boolean;
}

export interface AgentSharePublicInfo {
  share: AgentSharePublicSummary;
  agent: AgentSharePublicAgentSummary;
  runtime: AgentSharePublicRuntimeSummary;
  suggested_questions?: string[];
}

export interface AgentShareSessionCreateResult {
  session_id: string;
  anonymous_visitor_id: string;
  visitor_token: string;
  expires_at: string;
}

export interface StoredAgentShareSession {
  sessionId: string;
  anonymousVisitorId: string;
  visitorToken: string;
  expiresAt?: string;
}

const AGENT_SHARE_SESSION_STORAGE_PREFIX = 'weknora_agent_share_session:';

function buildShareSessionHeaders(visitorToken?: string) {
  return visitorToken
    ? {
        headers: {
          'X-Share-Session-Token': visitorToken,
        },
      }
    : undefined;
}

export function getPublicAgentShare(shareCode: string) {
  return get<{ success: boolean; data: AgentSharePublicInfo }>(`/api/v1/public/agent-page-shares/${encodeURIComponent(shareCode)}`);
}

export function getAgentPageShare(agentId: string) {
  return get<{ success: boolean; data: AgentPageShareManagementView | null }>(`/api/v1/agents/${encodeURIComponent(agentId)}/page-share`);
}

export function createOrEnableAgentPageShare(agentId: string) {
  return post<{ success: boolean; data: AgentPageShareManagementView }>(`/api/v1/agents/${encodeURIComponent(agentId)}/page-share`);
}

export function deleteAgentPageShare(agentId: string) {
  return del<{ success: boolean; message?: string }>(`/api/v1/agents/${encodeURIComponent(agentId)}/page-share`);
}

export function createPublicAgentShareSession(shareCode: string) {
  return post<{ success: boolean; data: AgentShareSessionCreateResult }>(`/api/v1/public/agent-page-shares/${encodeURIComponent(shareCode)}/sessions`);
}

export function getPublicAgentShareMessages(shareCode: string, sessionId: string, visitorToken: string, params?: { beforeTime?: string; limit?: number }) {
  const query = new URLSearchParams();
  if (params?.beforeTime) {
    query.set('before_time', params.beforeTime);
  }
  if (params?.limit) {
    query.set('limit', String(params.limit));
  }
  const suffix = query.toString() ? `?${query.toString()}` : '';
  return get<{ success: boolean; data: any[] }>(
    `/api/v1/public/agent-page-shares/${encodeURIComponent(shareCode)}/sessions/${encodeURIComponent(sessionId)}/messages${suffix}`,
    buildShareSessionHeaders(visitorToken),
  );
}

export function stopPublicAgentShareSession(shareCode: string, sessionId: string, visitorToken: string, messageId: string) {
  return post<{ success: boolean }>(
    `/api/v1/public/agent-page-shares/${encodeURIComponent(shareCode)}/sessions/${encodeURIComponent(sessionId)}/stop`,
    { message_id: messageId },
    buildShareSessionHeaders(visitorToken),
  );
}

export function getStoredAgentShareSession(shareCode: string): StoredAgentShareSession | null {
  const raw = localStorage.getItem(`${AGENT_SHARE_SESSION_STORAGE_PREFIX}${shareCode}`);
  if (!raw) {
    return null;
  }
  try {
    const parsed = JSON.parse(raw) as StoredAgentShareSession;
    if (!parsed?.sessionId || !parsed?.visitorToken) {
      localStorage.removeItem(`${AGENT_SHARE_SESSION_STORAGE_PREFIX}${shareCode}`);
      return null;
    }
    if (parsed.expiresAt) {
      const expiresAt = new Date(parsed.expiresAt).getTime();
      if (!Number.isNaN(expiresAt) && expiresAt <= Date.now()) {
        localStorage.removeItem(`${AGENT_SHARE_SESSION_STORAGE_PREFIX}${shareCode}`);
        return null;
      }
    }
    return parsed;
  } catch {
    localStorage.removeItem(`${AGENT_SHARE_SESSION_STORAGE_PREFIX}${shareCode}`);
    return null;
  }
}

export function setStoredAgentShareSession(shareCode: string, session: StoredAgentShareSession) {
  localStorage.setItem(`${AGENT_SHARE_SESSION_STORAGE_PREFIX}${shareCode}`, JSON.stringify(session));
}

export function clearStoredAgentShareSession(shareCode: string) {
  localStorage.removeItem(`${AGENT_SHARE_SESSION_STORAGE_PREFIX}${shareCode}`);
}