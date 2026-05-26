<template>
  <div class="agent-share-header">
    <div class="agent-share-agent">
      <div class="agent-share-avatar">{{ agent.avatar || 'AI' }}</div>
      <div class="agent-share-meta">
        <div class="agent-share-title-row">
          <h1 class="agent-share-title">{{ agent.name || '分享智能体' }}</h1>
          <span class="agent-share-badge">{{ runtime.agent_mode === 'smart-reasoning' ? '智能体模式' : '问答模式' }}</span>
        </div>
        <p v-if="agent.description" class="agent-share-description">{{ agent.description }}</p>
      </div>
    </div>
    <div class="agent-share-capabilities">
      <span class="capability-chip" :class="{ active: runtime.multi_turn_enabled }">多轮{{ runtime.multi_turn_enabled ? '开启' : '关闭' }}</span>
      <span class="capability-chip" :class="{ active: runtime.image_upload_enabled }">图片{{ runtime.image_upload_enabled ? '可用' : '关闭' }}</span>
      <span class="capability-chip" :class="{ active: runtime.attachment_upload_enabled }">附件{{ runtime.attachment_upload_enabled ? '可用' : '关闭' }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { AgentSharePublicAgentSummary, AgentSharePublicRuntimeSummary } from '@/api/agent-share';

defineProps<{
  agent: AgentSharePublicAgentSummary;
  runtime: AgentSharePublicRuntimeSummary;
}>();
</script>

<style scoped lang="less">
.agent-share-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24px;
  padding: 24px 28px 18px;
  border-bottom: 1px solid rgba(15, 23, 42, 0.08);
  background:
    radial-gradient(circle at top left, rgba(7, 192, 95, 0.12), transparent 42%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.96), rgba(248, 250, 252, 0.96));
}

.agent-share-agent {
  display: flex;
  gap: 16px;
  min-width: 0;
}

.agent-share-avatar {
  width: 56px;
  height: 56px;
  border-radius: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
  background: linear-gradient(135deg, #0f172a, #1e293b);
  color: #f8fafc;
  flex-shrink: 0;
}

.agent-share-meta {
  min-width: 0;
}

.agent-share-title-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.agent-share-title {
  margin: 0;
  font-size: 24px;
  line-height: 1.2;
  color: #0f172a;
}

.agent-share-badge {
  display: inline-flex;
  align-items: center;
  padding: 4px 10px;
  border-radius: 999px;
  background: rgba(15, 23, 42, 0.06);
  color: #334155;
  font-size: 12px;
}

.agent-share-description {
  margin: 8px 0 0;
  color: #475569;
  line-height: 1.6;
}

.agent-share-capabilities {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.capability-chip {
  display: inline-flex;
  align-items: center;
  padding: 6px 12px;
  border-radius: 999px;
  background: rgba(148, 163, 184, 0.14);
  color: #475569;
  font-size: 12px;
}

.capability-chip.active {
  background: rgba(7, 192, 95, 0.12);
  color: #047857;
}

@media (max-width: 900px) {
  .agent-share-header {
    flex-direction: column;
    gap: 16px;
    padding: 20px 18px 14px;
  }

  .agent-share-capabilities {
    justify-content: flex-start;
  }

  .agent-share-title {
    font-size: 22px;
  }
}
</style>