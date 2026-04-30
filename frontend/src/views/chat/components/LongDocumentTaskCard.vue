<template>
  <div class="long-document-task-card">
    <div class="task-card-header">
      <div>
        <div class="task-card-title">长文档任务</div>
        <div class="task-card-subtitle">{{ taskLabel }}</div>
      </div>
      <t-tag :theme="statusTheme" variant="light">{{ statusText }}</t-tag>
    </div>
    <div class="task-card-progress">
      <div class="task-card-progress-text">
        <span>进度 {{ progressPercent }}%</span>
        <span>{{ currentTask.completed_batches || 0 }}/{{ currentTask.total_batches || 0 }} 批</span>
      </div>
      <t-progress :percentage="progressPercent" :status="progressStatus" />
    </div>
    <div v-if="latestEventText" class="task-card-live" :class="{ 'task-card-live-active': isEventStreaming }">
      <span>{{ latestEventText }}</span>
      <span v-if="isEventStreaming">SSE 实时同步中</span>
      <span v-else-if="isPolling">已切换为轮询同步</span>
    </div>
    <div v-if="currentTask.error_message" class="task-card-error">{{ currentTask.error_message }}</div>
    <div class="task-card-actions">
      <ExportDropdown v-if="canDownload" :content-resolver="resolveArtifactContent" :filename-prefix="downloadFilenamePrefix" />
      <t-button v-if="canRetry" size="small" theme="warning" variant="outline" :loading="isRetrying" @click="handleRetry">重试任务</t-button>
      <t-button v-if="canCancel" size="small" theme="danger" variant="outline" @click="handleCancel">取消任务</t-button>
      <span v-if="isPolling" class="task-card-polling">状态轮询中</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { fetchEventSource } from '@microsoft/fetch-event-source';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { MessagePlugin } from 'tdesign-vue-next';
import i18n from '@/i18n';
import { cancelLongDocumentTask, downloadLongDocumentArtifact, getLongDocumentTask, getLongDocumentTaskArtifact, retryLongDocumentTask } from '@/api/chat/index';
import { getApiBaseUrl } from '@/utils/api-base';
import { generateRandomString } from '@/utils/index';
import ExportDropdown from './ExportDropdown.vue';

const props = defineProps({
  task: {
    type: Object,
    required: true,
  },
});

const currentTask = ref<any>(props.task);
const isPolling = ref(false);
const isEventStreaming = ref(false);
const isRetrying = ref(false);
const latestEventText = ref('');
const artifactMeta = ref<any>(props.task?.artifact || null);
const artifactContent = ref('');
let pollTimer: number | null = null;
let streamController: AbortController | null = null;
let streamClosedByUser = false;
let artifactContentPromise: Promise<string> | null = null;

const terminalStatuses = new Set(['completed', 'partial', 'failed', 'cancelled']);
const isTerminal = computed(() => terminalStatuses.has(currentTask.value?.status));
const canDownload = computed(() => ['completed', 'partial'].includes(currentTask.value?.status) && Boolean(currentTask.value?.artifact_path || artifactMeta.value?.status === 'available' || currentTask.value?.artifact_available));
const canCancel = computed(() => ['pending', 'running', 'assembling'].includes(currentTask.value?.status));
const canRetry = computed(() => ['failed', 'partial', 'cancelled'].includes(currentTask.value?.status));
const progressPercent = computed(() => {
  const total = Number(currentTask.value?.total_batches || 0);
  const completed = Number(currentTask.value?.completed_batches || 0);
  if (!total) return 0;
  return Math.min(100, Math.max(0, Math.round((completed / total) * 100)));
});

const statusText = computed(() => {
  const mapping: Record<string, string> = {
    pending: '等待执行',
    running: '翻译中',
    assembling: '正在拼装文件',
    completed: '已完成',
    partial: '部分完成',
    failed: '执行失败',
    cancelled: '已取消',
  };
  return mapping[currentTask.value?.status] || '处理中';
});

const statusTheme = computed(() => {
  switch (currentTask.value?.status) {
    case 'completed':
      return 'success';
    case 'partial':
      return 'warning';
    case 'failed':
    case 'cancelled':
      return 'danger';
    default:
      return 'primary';
  }
});

const progressStatus = computed(() => {
  if (currentTask.value?.status === 'failed') return 'error';
  if (currentTask.value?.status === 'partial') return 'warning';
  if (currentTask.value?.status === 'completed') return 'success';
  return 'active';
});

const taskLabel = computed(() => {
  return currentTask.value?.output_format === 'markdown' ? '全文翻译为 Markdown 文件' : '长文档处理任务';
});

const stripFileExtension = (fileName?: string): string => {
  const normalized = (fileName || '').trim();
  if (!normalized) {
    return '';
  }
  return normalized.replace(/\.[^./\\]+$/, '');
};

const downloadFilenamePrefix = computed(() => {
  return stripFileExtension(artifactMeta.value?.file_name || currentTask.value?.artifact?.file_name) || currentTask.value?.id || '长文档导出';
});

const resolveAuthHeaders = () => {
  const token = localStorage.getItem('weknora_token');
  if (!token) {
    throw new Error('登录状态已失效，请重新登录后查看任务状态');
  }
  const headers: Record<string, string> = {
    Authorization: `Bearer ${token}`,
    'Accept-Language': i18n.global.locale?.value || localStorage.getItem('locale') || 'zh-CN',
    'X-Request-ID': generateRandomString(12),
  };

  const selectedTenantId = localStorage.getItem('weknora_selected_tenant_id');
  const defaultTenant = localStorage.getItem('weknora_tenant');
  if (selectedTenantId) {
    try {
      const parsedTenant = defaultTenant ? JSON.parse(defaultTenant) : null;
      const defaultTenantId = parsedTenant?.id ? String(parsedTenant.id) : null;
      if (selectedTenantId !== defaultTenantId) {
        headers['X-Tenant-ID'] = selectedTenantId;
      }
    } catch (error) {
      console.error('[LongDocumentTask] Failed to parse tenant header context:', error);
    }
  }
  return headers;
};

const mergeTaskState = (patch: Record<string, any>) => {
  currentTask.value = {
    ...currentTask.value,
    ...patch,
  };
};

const resolveArtifactContent = async (): Promise<string> => {
  if (!currentTask.value?.id) {
    throw new Error('任务不存在，无法导出');
  }
  if (artifactContent.value) {
    return artifactContent.value;
  }
  if (!artifactContentPromise) {
    artifactContentPromise = (async () => {
      const res: any = await downloadLongDocumentArtifact(currentTask.value.id);
      const blob = res instanceof Blob ? res : res?.data instanceof Blob ? res.data : res;
      if (!(blob instanceof Blob)) {
        throw new Error('下载失败');
      }
      const text = await blob.text();
      artifactContent.value = text;
      return text;
    })();
  }

  try {
    return await artifactContentPromise;
  } finally {
    artifactContentPromise = null;
  }
};

const refreshArtifact = async (silent = true) => {
  if (!currentTask.value?.id) return;
  try {
    const res: any = await getLongDocumentTaskArtifact(currentTask.value.id);
    artifactMeta.value = res?.data || artifactMeta.value;
    if (artifactMeta.value?.status === 'available') {
      mergeTaskState({ artifact_available: true });
    }
  } catch (error: any) {
    if (!silent) {
      MessagePlugin.error(error?.message || '获取产物信息失败');
    }
  }
};

const refreshTask = async (options?: { silent?: boolean; fromPolling?: boolean }) => {
  if (!currentTask.value?.id) return;
  try {
    if (options?.fromPolling) {
      isPolling.value = true;
    }
    const res: any = await getLongDocumentTask(currentTask.value.id);
    currentTask.value = res?.data || currentTask.value;
    artifactMeta.value = currentTask.value?.artifact || artifactMeta.value;
    if ((currentTask.value?.status === 'completed' || currentTask.value?.status === 'partial') && !artifactMeta.value) {
      await refreshArtifact(options?.silent ?? true);
    }
    if (terminalStatuses.has(currentTask.value?.status)) {
      stopSync();
    }
  } catch (error: any) {
    if (options?.fromPolling) {
      stopPolling();
    }
    if (!options?.silent) {
      MessagePlugin.error(error?.message || '获取任务状态失败');
    }
  } finally {
    if (options?.fromPolling) {
      isPolling.value = false;
    }
  }
};

const describeEvent = (type: string, data: Record<string, any> = {}) => {
  switch (type) {
    case 'task_started':
      return '任务已开始，正在准备分批处理';
    case 'batch_started':
      return `批次 ${data.batch_no || '-'} 开始执行`;
    case 'batch_completed':
      return `批次 ${data.batch_no || '-'} 已完成`;
    case 'batch_failed':
      return `批次 ${data.batch_no || '-'} 执行失败`;
    case 'task_assembling':
      return '批次处理结束，正在拼装 Markdown 文件';
    case 'artifact_available':
      return 'Markdown 产物已生成，可下载';
    case 'task_completed':
      return '任务已完成';
    case 'task_partial':
      return '任务部分完成，可下载当前产物或重试失败批次';
    case 'task_failed':
      return '任务执行失败，可重试';
    case 'task_cancelled':
      return '任务已取消，可重新发起重试';
    default:
      return '';
  }
};

const applyTaskEvent = async (payload: any) => {
  if (!payload?.type) {
    return;
  }
  const data = payload.data || {};
  const nextEventText = describeEvent(payload.type, data);
  if (nextEventText) {
    latestEventText.value = nextEventText;
  }
  switch (payload.type) {
    case 'task_started':
      mergeTaskState({
        status: 'running',
        total_batches: data.total_batches ?? currentTask.value?.total_batches,
      });
      break;
    case 'batch_started':
      mergeTaskState({ status: 'running' });
      break;
    case 'batch_completed':
      mergeTaskState({ status: 'running' });
      break;
    case 'batch_failed':
      mergeTaskState({
        status: currentTask.value?.status === 'pending' ? 'running' : currentTask.value?.status,
        error_message: data.error_message || currentTask.value?.error_message || '',
      });
      break;
    case 'task_assembling':
      mergeTaskState({ status: 'assembling' });
      break;
    case 'artifact_available':
      mergeTaskState({ artifact_available: true });
      await refreshArtifact(true);
      break;
    case 'task_completed':
      mergeTaskState({
        status: 'completed',
        error_message: '',
        completed_batches: data.completed_batches ?? currentTask.value?.completed_batches,
        failed_batches: data.failed_batches ?? currentTask.value?.failed_batches,
      });
      await refreshArtifact(true);
      stopSync();
      break;
    case 'task_partial':
      mergeTaskState({
        status: 'partial',
        error_message: data.error_message || currentTask.value?.error_message || '',
        completed_batches: data.completed_batches ?? currentTask.value?.completed_batches,
        failed_batches: data.failed_batches ?? currentTask.value?.failed_batches,
      });
      await refreshArtifact(true);
      stopSync();
      break;
    case 'task_failed':
      mergeTaskState({
        status: 'failed',
        error_message: data.error_message || currentTask.value?.error_message || '任务执行失败',
        failed_batches: data.failed_batches ?? currentTask.value?.failed_batches,
      });
      stopSync();
      break;
    case 'task_cancelled':
      mergeTaskState({ status: 'cancelled' });
      stopSync();
      break;
    case 'task.snapshot':
      mergeTaskState({
        status: data.status ?? currentTask.value?.status,
        completed_batches: data.completed_batches ?? currentTask.value?.completed_batches,
        failed_batches: data.failed_batches ?? currentTask.value?.failed_batches,
        total_batches: data.total_batches ?? currentTask.value?.total_batches,
        artifact_available: data.artifact_available ?? currentTask.value?.artifact_available,
      });
      if (data.artifact_available) {
        await refreshArtifact(true);
      }
      break;
    default:
      break;
  }
};

const startEventStream = () => {
  if (!currentTask.value?.id || isTerminal.value) {
    return;
  }
  stopEventStream(true);
  streamClosedByUser = false;
  const controller = new AbortController();
  streamController = controller;
  const url = `${getApiBaseUrl()}/api/v1/long-document-tasks/${currentTask.value.id}/events`;

  void fetchEventSource(url, {
    method: 'GET',
    headers: resolveAuthHeaders(),
    signal: controller.signal,
    openWhenHidden: true,
    async onopen(response) {
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      isEventStreaming.value = true;
      stopPolling();
      await refreshTask({ silent: true });
    },
    async onmessage(event) {
      if (!event?.data) {
        return;
      }
      try {
        const payload = JSON.parse(event.data);
        await applyTaskEvent(payload);
      } catch (error) {
        console.error('[LongDocumentTask] Failed to parse event payload:', error, event.data);
      }
    },
    onclose() {
      isEventStreaming.value = false;
      streamController = null;
      if (!streamClosedByUser && !isTerminal.value) {
        startPolling();
      }
    },
    onerror(error) {
      isEventStreaming.value = false;
      if (!streamClosedByUser && !isTerminal.value) {
        latestEventText.value = latestEventText.value || '实时订阅中断，已切换为轮询同步';
        startPolling();
      }
      throw error;
    },
  }).catch((error) => {
    if (!streamClosedByUser && !isTerminal.value) {
      console.warn('[LongDocumentTask] Event stream fallback to polling:', error);
      startPolling();
    }
  });
};

const startPolling = () => {
  stopPolling();
  if (isTerminal.value || !currentTask.value?.id) return;
  pollTimer = window.setInterval(() => {
    refreshTask({ silent: true, fromPolling: true });
  }, 3000);
};

const stopPolling = () => {
  if (pollTimer) {
    window.clearInterval(pollTimer);
    pollTimer = null;
  }
};


const stopEventStream = (silent = false) => {
  streamClosedByUser = true;
  if (streamController) {
    streamController.abort();
    streamController = null;
  }
  isEventStreaming.value = false;
  if (!silent) {
    latestEventText.value = latestEventText.value || '实时同步已结束';
  }
};

const stopSync = () => {
  stopEventStream(true);
  stopPolling();
};
const handleCancel = async () => {
  try {
    const res: any = await cancelLongDocumentTask(currentTask.value.id);
    currentTask.value = res?.data || currentTask.value;
    latestEventText.value = '任务已取消';
    stopSync();
  } catch (error: any) {
    MessagePlugin.error(error?.message || '取消任务失败');
  }
};

const handleRetry = async () => {
  if (!currentTask.value?.id) {
    return;
  }
  try {
    isRetrying.value = true;
    stopSync();
    artifactContent.value = '';
    artifactContentPromise = null;
    const res: any = await retryLongDocumentTask(currentTask.value.id);
    currentTask.value = res?.data || currentTask.value;
    artifactMeta.value = null;
    latestEventText.value = '重试已提交，正在重新建立任务订阅';
    await refreshTask({ silent: true });
    startEventStream();
    MessagePlugin.success('任务已重新入队');
  } catch (error: any) {
    MessagePlugin.error(error?.message || '重试任务失败');
    if (!isTerminal.value) {
      startEventStream();
    }
  } finally {
    isRetrying.value = false;
  }
};

const startTaskSync = async () => {
  stopSync();
  artifactMeta.value = currentTask.value?.artifact || artifactMeta.value;
  await refreshTask({ silent: true });
  if (isTerminal.value) {
    if (['completed', 'partial'].includes(currentTask.value?.status)) {
      await refreshArtifact(true);
    }
    return;
  }
  startEventStream();
};

watch(() => props.task, (value) => {
  currentTask.value = value;
  artifactMeta.value = value?.artifact || artifactMeta.value;
  artifactContent.value = '';
  artifactContentPromise = null;
  if (terminalStatuses.has(value?.status)) {
    stopSync();
    return;
  }
  void startTaskSync();
}, { deep: true });

onMounted(() => {
  void startTaskSync();
});

onBeforeUnmount(() => {
  stopSync();
});
</script>

<style scoped lang="less">
.long-document-task-card {
  border: 1px solid var(--td-component-border);
  border-radius: 12px;
  background: linear-gradient(180deg, rgba(27, 87, 165, 0.05) 0%, rgba(27, 87, 165, 0.02) 100%);
  padding: 14px;
}

.task-card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.task-card-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.task-card-subtitle {
  margin-top: 4px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.task-card-progress {
  margin-top: 14px;
}

.task-card-live {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-top: 12px;
  padding: 8px 10px;
  border-radius: 8px;
  background: rgba(27, 87, 165, 0.06);
  color: var(--td-text-color-secondary);
  font-size: 12px;
}

.task-card-live-active {
  color: var(--td-brand-color);
}

.task-card-progress-text {
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.task-card-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
}

.task-card-error {
  margin-top: 10px;
  padding: 8px 10px;
  border-radius: 8px;
  background: rgba(214, 48, 49, 0.08);
  color: #b42318;
  font-size: 12px;
}

.task-card-polling {
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
</style>