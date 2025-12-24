import { get, post, put, del } from "../../utils/request";

// 智能体类型
export type CustomAgentType = 'normal' | 'agent' | 'custom';

// 智能体配置
export interface CustomAgentConfig {
  agent_mode?: 'normal' | 'agent';  // 运行模式：normal=RAG模式, agent=ReAct Agent模式
  system_prompt?: string;
  model_id?: string;
  rerank_model_id?: string;  // ReRank 模型 ID（当使用知识库时需要）
  temperature?: number;
  max_iterations?: number;
  allowed_tools?: string[];
  knowledge_bases?: string[];
  // 当没有配置知识库时，是否允许用户自由选择知识库
  // true (默认): 用户可以自由选择任意知识库
  // false: 禁用知识库选择功能
  allow_user_kb_selection?: boolean;
  web_search_enabled?: boolean;
  web_search_max_results?: number;
  reflection_enabled?: boolean;
  welcome_message?: string;
  suggested_prompts?: string[];
  multi_turn_enabled?: boolean;  // 是否启用多轮对话
  history_turns?: number;        // 保留历史轮数
}

// 智能体
export interface CustomAgent {
  id: string;
  name: string;
  description?: string;
  avatar?: string;
  is_builtin: boolean;
  type: CustomAgentType;
  tenant_id?: number;
  created_by?: string;
  config: CustomAgentConfig;
  created_at?: string;
  updated_at?: string;
}

// 创建智能体请求
export interface CreateAgentRequest {
  name: string;
  description?: string;
  avatar?: string;
  type?: CustomAgentType;
  config?: CustomAgentConfig;
}

// 更新智能体请求
export interface UpdateAgentRequest {
  name: string;
  description?: string;
  avatar?: string;
  type?: CustomAgentType;
  config?: CustomAgentConfig;
}

// 内置智能体 ID
export const BUILTIN_AGENT_NORMAL_ID = 'builtin-normal';
export const BUILTIN_AGENT_AGENT_ID = 'builtin-agent';

// 获取智能体列表（包括内置智能体）
export function listAgents() {
  return get<{ data: CustomAgent[] }>('/api/v1/agents');
}

// 获取智能体详情
export function getAgentById(id: string) {
  return get<{ data: CustomAgent }>(`/api/v1/agents/${id}`);
}

// 创建智能体
export function createAgent(data: CreateAgentRequest) {
  return post<{ data: CustomAgent }>('/api/v1/agents', data);
}

// 更新智能体
export function updateAgent(id: string, data: UpdateAgentRequest) {
  return put<{ data: CustomAgent }>(`/api/v1/agents/${id}`, data);
}

// 删除智能体
export function deleteAgent(id: string) {
  return del<{ success: boolean }>(`/api/v1/agents/${id}`);
}

// 判断是否为内置智能体
export function isBuiltinAgent(agentId: string): boolean {
  return agentId === BUILTIN_AGENT_NORMAL_ID || agentId === BUILTIN_AGENT_AGENT_ID;
}
