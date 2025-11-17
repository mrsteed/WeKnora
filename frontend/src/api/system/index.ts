import { get, put } from '@/utils/request'

export interface SystemInfo {
  version: string
  commit_id?: string
  build_time?: string
  go_version?: string
  keyword_index_engine?: string
  vector_store_engine?: string
  graph_database_engine?: string
  minio_enabled?: boolean
}

export interface ToolDefinition {
  name: string
  label: string
  description: string
}

export interface PlaceholderDefinition {
  name: string
  label: string
  description: string
}

export interface AgentConfig {
  enabled: boolean
  max_iterations: number
  reflection_enabled: boolean
  allowed_tools: string[]
  temperature: number
  thinking_model_id: string
  rerank_model_id: string
  knowledge_bases?: string[]
  system_prompt?: string  // System prompt template with placeholders (optional)
  use_custom_system_prompt?: boolean
  available_tools?: ToolDefinition[]  // GET 响应中包含，POST/PUT 不需要
  available_placeholders?: PlaceholderDefinition[]  // GET 响应中包含，POST/PUT 不需要
}

export interface ConversationConfig {
  prompt: string
  context_template: string
  temperature: number
  max_tokens: number
}

export function getSystemInfo(): Promise<{ data: SystemInfo }> {
  return get('/api/v1/system/info')
}

export function getAgentConfig(): Promise<{ data: AgentConfig }> {
  return get('/api/v1/tenants/kv/agent-config')
}

export function updateAgentConfig(config: AgentConfig): Promise<{ data: AgentConfig }> {
  return put('/api/v1/tenants/kv/agent-config', config)
}

export function getConversationConfig(): Promise<{ data: ConversationConfig }> {
  return get('/api/v1/tenants/kv/conversation-config')
}

export function updateConversationConfig(config: ConversationConfig): Promise<{ data: ConversationConfig }> {
  return put('/api/v1/tenants/kv/conversation-config', config)
}
