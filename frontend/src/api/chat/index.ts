import { get, post, put, del, postChat, getDown } from "../../utils/request";



export async function createSessions(data = {}) {
  return post("/api/v1/sessions", data);
}

export async function getSessionsList(page: number, page_size: number) {
  return get(`/api/v1/sessions?page=${page}&page_size=${page_size}`);
}

export async function generateSessionsTitle(session_id: string, data: any) {
  return post(`/api/v1/sessions/${session_id}/generate_title`, data);
}

export async function knowledgeChat(data: { session_id: string; query: string; }) {
  return postChat(`/api/v1/knowledge-chat/${data.session_id}`, { query: data.query, channel: "web" });
}

// Agent chat with streaming support
export async function agentChat(data: { 
  session_id: string; 
  query: string;
  knowledge_base_ids?: string[];
  agent_enabled: boolean;
}) {
  return postChat(`/api/v1/agent-chat/${data.session_id}`, { 
    query: data.query,
    knowledge_base_ids: data.knowledge_base_ids,
    agent_enabled: data.agent_enabled,
    channel: "web"
  });
}

export async function getMessageList(data: { session_id: string; limit: number, created_at: string }) {
  if (data.created_at) {
    return get(`/api/v1/messages/${data.session_id}/load?before_time=${encodeURIComponent(data.created_at)}&limit=${data.limit}`);
  } else {
    return get(`/api/v1/messages/${data.session_id}/load?limit=${data.limit}`);
  }
}

export async function delSession(session_id: string) {
  return del(`/api/v1/sessions/${session_id}`);
}

export async function batchDelSessions(ids: string[]) {
  return del(`/api/v1/sessions/batch`, { ids });
}

export async function deleteAllSessions() {
  return del(`/api/v1/sessions/batch`, { delete_all: true });
}

export async function getSession(session_id: string) {
  return get(`/api/v1/sessions/${session_id}`);
}

export async function stopSession(session_id: string, message_id: string) {
  return post(`/api/v1/sessions/${session_id}/stop`, { message_id });
}

export async function clearSessionMessages(session_id: string) {
  return del(`/api/v1/sessions/${session_id}/messages`);
}

export async function createLongDocumentTask(data: {
  session_id: string;
  knowledge_id: string;
  user_query: string;
  summary_model_id?: string;
  output_format?: string;
  task_kind?: string;
  idempotency_key?: string;
  options?: Record<string, any>;
}) {
  return post('/api/v1/long-document-tasks', data);
}

export async function getLongDocumentTasksBySession(session_id: string, page = 1, page_size = 100) {
  return get(`/api/v1/long-document-tasks?session_id=${encodeURIComponent(session_id)}&page=${page}&page_size=${page_size}`);
}

export async function getLongDocumentTask(task_id: string) {
  return get(`/api/v1/long-document-tasks/${task_id}`);
}

export async function getLongDocumentTaskArtifact(task_id: string) {
  return get(`/api/v1/long-document-tasks/${task_id}/artifact`);
}

export async function getLongDocumentTaskBatches(task_id: string) {
  return get(`/api/v1/long-document-tasks/${task_id}/batches`);
}

export async function retryLongDocumentTask(task_id: string) {
  return post(`/api/v1/long-document-tasks/${task_id}/retry`, {});
}

export async function cancelLongDocumentTask(task_id: string) {
  return post(`/api/v1/long-document-tasks/${task_id}/cancel`, {});
}

export async function downloadLongDocumentArtifact(task_id: string) {
  return getDown(`/api/v1/long-document-tasks/${task_id}/download`);
}