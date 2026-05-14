import { fetchEventSource } from '@microsoft/fetch-event-source';
import { ref, onUnmounted } from 'vue';
import { generateRandomString } from '@/utils/index';
import i18n from '@/i18n';
import { getApiBaseUrl } from '@/utils/api-base';



interface StreamOptions {
  // 请求方法 (默认POST)
  method?: 'GET' | 'POST'
  // 请求头
  headers?: Record<string, string>
  // 请求体自动序列化
  body?: Record<string, any>
  // 流式渲染间隔 (ms)
  chunkInterval?: number
}

export function useStream() {
  // 响应式状态
  const output = ref('')              // 显示内容
  const isStreaming = ref(false)      // 流状态
  const isLoading = ref(false)        // 初始加载
  const error = ref<string | null>(null)// 错误信息
  let controller = new AbortController()

  // 流式渲染缓冲
  let buffer: string[] = []
  let renderTimer: number | null = null

  // 启动流式请求
  const startStream = async (params: { session_id: any; query: any; knowledge_base_ids?: string[]; knowledge_ids?: string[]; intent_hint?: string; base_artifact_id?: string; document_output_mode?: string; document_task_kind?: string; translation_options?: { source_language?: string; target_language?: string; preserve_structure?: boolean; output_format?: string }; document_target_heading?: string; document_merge_mode?: string; auto_continue?: boolean; generation_run_id?: string; auto_continue_root_id?: string; auto_continue_round?: number; auto_continue_prompt?: string; auto_continue_original_query?: string; agent_enabled?: boolean; agent_id?: string; web_search_enabled?: boolean; enable_memory?: boolean; summary_model_id?: string; mcp_service_ids?: string[]; mentioned_items?: Array<{id: string; name: string; type: string; kb_type?: string}>; images?: Array<{data: string}>; attachment_uploads?: Array<{data: string; file_name: string; file_size: number}>; method: string; url: string }) => {
    // 重置状态
    output.value = '';
    error.value = null;
    isStreaming.value = true;
    isLoading.value = true;

    // 获取API配置
    const apiUrl = getApiBaseUrl();
    
    // 获取JWT Token
    const token = localStorage.getItem('weknora_token');
    if (!token) {
      error.value = i18n.global.t('error.tokenNotFound');
      stopStream();
      return;
    }

    // 获取跨租户访问请求头
    const selectedTenantId = localStorage.getItem('weknora_selected_tenant_id');
    const defaultTenantId = localStorage.getItem('weknora_tenant');
    let tenantIdHeader: string | null = null;
    if (selectedTenantId) {
      try {
        const defaultTenant = defaultTenantId ? JSON.parse(defaultTenantId) : null;
        const defaultId = defaultTenant?.id ? String(defaultTenant.id) : null;
        if (selectedTenantId !== defaultId) {
          tenantIdHeader = selectedTenantId;
        }
      } catch (e) {
        console.error('Failed to parse tenant info', e);
      }
    }

    // Validate knowledge_base_ids for agent-chat requests
    // Note: knowledge_base_ids can be empty if user hasn't selected any, but we allow it
    // The backend will handle the case when no knowledge bases are selected
    const isAgentChat = params.url === '/api/v1/agent-chat';
    // Removed validation - allow empty knowledge_base_ids array
    // The backend should handle this case appropriately

    // TTFB instrumentation: record the moment we kick off the request so
    // we can compare it with the first answer chunk we receive from the
    // server. This makes it possible to correlate the frontend-observed
    // latency with the backend "TTFB:first_answer_chunk" log line by
    // matching on X-Request-ID.
    const sentAt = performance.now();
    const requestID = generateRandomString(12);
    let firstAnswerLogged = false;

    try {
      let url =
        params.method == "POST"
          ? `${apiUrl}${params.url}/${params.session_id}`
          : `${apiUrl}${params.url}/${params.session_id}?message_id=${params.query}`;
      console.log(`[TTFB] request:start request_id=${requestID} url=${url} sent_at=${Date.now()}`);
      
      // Prepare POST body with required fields for agent-chat
      // knowledge_base_ids array and agent_enabled can update Session's SessionAgentConfig
      const postBody: any = { 
        query: params.query,
        agent_enabled: params.agent_enabled !== undefined ? params.agent_enabled : true
      };
      if (params.intent_hint) {
        postBody.intent_hint = params.intent_hint;
      }
      if (params.base_artifact_id) {
        postBody.base_artifact_id = params.base_artifact_id;
      }
      if (params.document_output_mode) {
        postBody.document_output_mode = params.document_output_mode;
      }
      if (params.document_task_kind) {
        postBody.document_task_kind = params.document_task_kind;
      }
      if (params.translation_options) {
        postBody.translation_options = params.translation_options;
      }
      if (params.document_target_heading) {
        postBody.document_target_heading = params.document_target_heading;
      }
      if (params.document_merge_mode) {
        postBody.document_merge_mode = params.document_merge_mode;
      }
      if (params.auto_continue !== undefined) {
        postBody.auto_continue = params.auto_continue;
      }
      if (params.generation_run_id) {
        postBody.generation_run_id = params.generation_run_id;
      }
      if (params.auto_continue_root_id) {
        postBody.auto_continue_root_id = params.auto_continue_root_id;
      }
      if (params.auto_continue_round !== undefined) {
        postBody.auto_continue_round = params.auto_continue_round;
      }
      if (params.auto_continue_prompt) {
        postBody.auto_continue_prompt = params.auto_continue_prompt;
      }
      if (params.auto_continue_original_query) {
        postBody.auto_continue_original_query = params.auto_continue_original_query;
      }
      // Always include knowledge_base_ids for agent-chat (already validated above)
      if (params.knowledge_base_ids !== undefined && params.knowledge_base_ids.length > 0) {
        postBody.knowledge_base_ids = params.knowledge_base_ids;
      }
      // Include knowledge_ids if provided
      if (params.knowledge_ids !== undefined && params.knowledge_ids.length > 0) {
        postBody.knowledge_ids = params.knowledge_ids;
      }
      // Include agent_id if provided (backend resolves shared agent and tenant from share relation)
      if (params.agent_id) {
        postBody.agent_id = params.agent_id;
      }
      // Include web_search_enabled if provided
      if (params.web_search_enabled !== undefined) {
        postBody.web_search_enabled = params.web_search_enabled;
      }
      // Include enable_memory if provided
      if (params.enable_memory !== undefined) {
        postBody.enable_memory = params.enable_memory;
      }
      // Include summary_model_id if provided (for non-Agent mode)
      if (params.summary_model_id) {
        postBody.summary_model_id = params.summary_model_id;
      }
      // Include mcp_service_ids if provided (for Agent mode)
      if (params.mcp_service_ids !== undefined && params.mcp_service_ids.length > 0) {
        postBody.mcp_service_ids = params.mcp_service_ids;
      }
      // Include mentioned_items if provided (for displaying @mentions in chat)
      if (params.mentioned_items !== undefined && params.mentioned_items.length > 0) {
        postBody.mentioned_items = params.mentioned_items;
      }
      // Include images if provided (base64 data URIs for multimodal chat)
      if (params.images !== undefined && params.images.length > 0) {
        postBody.images = params.images;
      }
      // Include attachment_uploads if provided (documents, audio, etc.)
      if (params.attachment_uploads !== undefined && params.attachment_uploads.length > 0) {
        postBody.attachment_uploads = params.attachment_uploads;
      }
      postBody.channel = "web";
      
      await fetchEventSource(url, {
        method: params.method,
        headers: {
          "Content-Type": "application/json",
          "Authorization": `Bearer ${token}`,
          "Accept-Language": i18n.global.locale?.value || localStorage.getItem('locale') || 'zh-CN',
          "X-Request-ID": requestID,
          ...(tenantIdHeader ? { "X-Tenant-ID": tenantIdHeader } : {}),
        },
        body:
          params.method == "POST"
            ? JSON.stringify(postBody)
            : null,
        signal: controller.signal,
        openWhenHidden: true,

        onopen: async (res) => {
          if (!res.ok) throw new Error(`HTTP ${res.status}`);
          console.log(`[TTFB] response:headers request_id=${requestID} elapsed_ms=${(performance.now() - sentAt).toFixed(1)}`);
          isLoading.value = false;
        },

        onmessage: (ev) => {
          const parsed = JSON.parse(ev.data);
          // Log first answer chunk for end-to-end TTFB measurement.
          // Filter by event type so non-answer events (references, tool
          // calls, etc.) don't count as the "first token" arrival.
          if (!firstAnswerLogged && (parsed?.response_type === 'answer' || parsed?.type === 'answer')) {
            firstAnswerLogged = true;
            console.log(`[TTFB] response:first_answer request_id=${requestID} elapsed_ms=${(performance.now() - sentAt).toFixed(1)}`);
          }
          buffer.push(parsed); // 数据存入缓冲
          // 执行自定义处理
          if (chunkHandler) {
            chunkHandler(parsed);
          }
        },

        onerror: (err) => {
          throw new Error(`${i18n.global.t('error.streamFailed')}: ${err}`);
        },

        onclose: () => {
          stopStream();
          if (closeHandler) {
            closeHandler();
          }
        },
      });
    } catch (err) {
      error.value = err instanceof Error ? err.message : String(err)
      stopStream()
    }
  }

  let chunkHandler: ((data: any) => void) | null = null
  let closeHandler: (() => void) | null = null
  // 注册块处理器
  const onChunk = (handler: () => void) => {
    chunkHandler = handler
  }

  // 注册流关闭处理器
  const onClose = (handler: () => void) => {
    closeHandler = handler
  }


  // 停止流
  const stopStream = () => {
    controller.abort();
    controller = new AbortController(); // 重置控制器（如需重新发起）
    isStreaming.value = false;
    isLoading.value = false;
  }

  // 组件卸载时自动清理
  onUnmounted(stopStream)

  return {
    output,          // 显示内容
    isStreaming,     // 是否在流式传输中
    isLoading,       // 初始连接状态
    error,
    onChunk,
    onClose,
    startStream,     // 启动流
    stopStream       // 手动停止
  }
}