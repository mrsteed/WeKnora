<template>
  <div class="agent-list-container">
    <!-- 头部 -->
    <div class="header">
      <div class="header-title">
        <h2>{{ $t('agent.title') }}</h2>
        <p class="header-subtitle">{{ $t('agent.subtitle') }}</p>
      </div>
    </div>
    <div class="header-divider"></div>

    <!-- 卡片网格 -->
    <div v-if="agents.length > 0" class="agent-card-wrap">
      <div 
        v-for="agent in agents" 
        :key="agent.id" 
        class="agent-card"
        :class="{ 
          'is-builtin': agent.is_builtin,
          'agent-mode-normal': agent.config?.agent_mode === 'normal',
          'agent-mode-agent': agent.config?.agent_mode === 'agent'
        }"
        @click="handleCardClick(agent)"
      >
        <!-- 装饰星星 -->
        <div class="card-decoration">
          <svg class="star-icon" width="24" height="24" viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M10 3L10.8 6.2C10.9 6.7 11.3 7.1 11.8 7.2L15 8L11.8 8.8C11.3 8.9 10.9 9.3 10.8 9.8L10 13L9.2 9.8C9.1 9.3 8.7 8.9 8.2 8.8L5 8L8.2 7.2C8.7 7.1 9.1 6.7 9.2 6.2L10 3Z" stroke="currentColor" stroke-width="0.8" stroke-linecap="round" stroke-linejoin="round" fill="currentColor" fill-opacity="0.15"/>
          </svg>
          <svg class="star-icon small" width="14" height="14" viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M10 3L10.8 6.2C10.9 6.7 11.3 7.1 11.8 7.2L15 8L11.8 8.8C11.3 8.9 10.9 9.3 10.8 9.8L10 13L9.2 9.8C9.1 9.3 8.7 8.9 8.2 8.8L5 8L8.2 7.2C8.7 7.1 9.1 6.7 9.2 6.2L10 3Z" stroke="currentColor" stroke-width="0.8" stroke-linecap="round" stroke-linejoin="round" fill="currentColor" fill-opacity="0.15"/>
          </svg>
        </div>
        
        <!-- 卡片头部 -->
        <div class="card-header">
          <div class="card-header-left">
            <AgentAvatar :name="agent.name" size="medium" />
            <span class="card-title" :title="agent.name">{{ agent.name }}</span>
          </div>
          <t-popup 
            v-if="!agent.is_builtin"
            v-model="agent.showMore" 
            overlayClassName="card-more-popup"
            :on-visible-change="(visible: boolean) => onVisibleChange(visible, agent)"
            trigger="click" 
            destroy-on-close 
            placement="bottom-right"
          >
            <div 
              class="more-wrap" 
              @click.stop
              :class="{ 'active-more': agent.showMore }"
            >
              <img class="more-icon" src="@/assets/img/more.png" alt="" />
            </div>
            <template #content>
              <div class="popup-menu">
                <div class="popup-menu-item" @click="handleEdit(agent)">
                  <t-icon class="menu-icon" name="edit" />
                  <span>{{ $t('common.edit') }}</span>
                </div>
                <div class="popup-menu-item delete" @click="handleDelete(agent)">
                  <t-icon class="menu-icon" name="delete" />
                  <span>{{ $t('common.delete') }}</span>
                </div>
              </div>
            </template>
          </t-popup>
          <div v-else class="builtin-badge">
            <t-icon name="lock-on" size="12px" />
            <span>{{ $t('agent.builtin') }}</span>
          </div>
        </div>

        <!-- 卡片内容 -->
        <div class="card-content">
          <div class="card-description">
            {{ agent.description || $t('agent.noDescription') }}
          </div>
        </div>

        <!-- 卡片底部 -->
        <div class="card-bottom">
          <div class="bottom-left">
            <div class="type-badge" :class="{ 'normal': agent.config?.agent_mode === 'normal', 'agent': agent.config?.agent_mode === 'agent' }">
              <t-icon :name="agent.config?.agent_mode === 'agent' ? 'control-platform' : 'chat'" size="14px" />
              <span>{{ agent.config?.agent_mode === 'agent' ? $t('agent.mode.agent') : $t('agent.mode.normal') }}</span>
            </div>
            <div class="feature-badges">
              <t-tooltip v-if="agent.config?.web_search_enabled" :content="$t('agent.features.webSearch')" placement="top">
                <div class="feature-badge web-search">
                  <t-icon name="search" size="14px" />
                </div>
              </t-tooltip>
              <t-tooltip v-if="agent.config?.knowledge_bases?.length" :content="$t('agent.features.knowledgeBase')" placement="top">
                <div class="feature-badge knowledge">
                  <t-icon name="folder" size="14px" />
                </div>
              </t-tooltip>
              <t-tooltip v-if="agent.config?.multi_turn_enabled" :content="$t('agent.features.multiTurn')" placement="top">
                <div class="feature-badge multi-turn">
                  <t-icon name="chat-bubble" size="14px" />
                </div>
              </t-tooltip>
            </div>
          </div>
          <span v-if="agent.updated_at" class="card-time">{{ formatDate(agent.updated_at) }}</span>
        </div>
      </div>
    </div>

    <!-- 空状态 -->
    <div v-else-if="!loading" class="empty-state">
      <img class="empty-img" src="@/assets/img/upload.svg" alt="">
      <span class="empty-txt">{{ $t('agent.empty.title') }}</span>
      <span class="empty-desc">{{ $t('agent.empty.description') }}</span>
    </div>

    <!-- 删除确认对话框 -->
    <t-dialog 
      v-model:visible="deleteVisible" 
      dialogClassName="del-agent-dialog" 
      :closeBtn="false" 
      :cancelBtn="null"
      :confirmBtn="null"
    >
      <div class="circle-wrap">
        <div class="dialog-header">
          <img class="circle-img" src="@/assets/img/circle.png" alt="">
          <span class="circle-title">{{ $t('agent.delete.confirmTitle') }}</span>
        </div>
        <span class="del-circle-txt">
          {{ $t('agent.delete.confirmMessage', { name: deletingAgent?.name ?? '' }) }}
        </span>
        <div class="circle-btn">
          <span class="circle-btn-txt" @click="deleteVisible = false">{{ $t('common.cancel') }}</span>
          <span class="circle-btn-txt confirm" @click="confirmDelete">{{ $t('agent.delete.confirmButton') }}</span>
        </div>
      </div>
    </t-dialog>

    <!-- 智能体编辑器弹窗 -->
    <AgentEditorModal 
      :visible="editorVisible"
      :mode="editorMode"
      :agent="editingAgent"
      @update:visible="editorVisible = $event"
      @success="handleEditorSuccess"
    />
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { MessagePlugin, Icon as TIcon } from 'tdesign-vue-next'
import { listAgents, deleteAgent, type CustomAgent } from '@/api/agent'
import { formatStringDate } from '@/utils/index'
import { useI18n } from 'vue-i18n'
import AgentEditorModal from './AgentEditorModal.vue'
import AgentAvatar from '@/components/AgentAvatar.vue'

const { t } = useI18n()

interface AgentWithUI extends CustomAgent {
  showMore?: boolean
}

const agents = ref<AgentWithUI[]>([])
const loading = ref(false)
const deleteVisible = ref(false)
const deletingAgent = ref<AgentWithUI | null>(null)
const editorVisible = ref(false)
const editorMode = ref<'create' | 'edit'>('create')
const editingAgent = ref<CustomAgent | null>(null)

const fetchList = () => {
  loading.value = true
  return listAgents().then((res: any) => {
    const data = res.data || []
    // 过滤掉内置智能体，只展示自定义智能体
    agents.value = data
      .filter((agent: CustomAgent) => !agent.is_builtin)
      .map((agent: CustomAgent) => ({
        ...agent,
        showMore: false
      }))
  }).finally(() => loading.value = false)
}

// 监听菜单创建智能体事件
const handleOpenAgentEditor = (event: CustomEvent) => {
  if (event.detail?.mode === 'create') {
    openCreateModal()
  }
}

onMounted(() => {
  fetchList()
  window.addEventListener('openAgentEditor', handleOpenAgentEditor as EventListener)
})

onUnmounted(() => {
  window.removeEventListener('openAgentEditor', handleOpenAgentEditor as EventListener)
})

const onVisibleChange = (visible: boolean, agent: AgentWithUI) => {
  if (!visible) {
    agent.showMore = false
  }
}

const handleCardClick = (agent: AgentWithUI) => {
  // 如果弹窗正在显示，不触发编辑
  if (agent.showMore) {
    return
  }
  // 点击卡片查看详情或编辑
  if (agent.is_builtin) {
    // 内置智能体只能查看
    MessagePlugin.info(t('agent.messages.builtinReadonly'))
  } else {
    handleEdit(agent)
  }
}

const handleEdit = (agent: AgentWithUI) => {
  agent.showMore = false
  editingAgent.value = agent
  editorMode.value = 'edit'
  editorVisible.value = true
}

const handleDelete = (agent: AgentWithUI) => {
  agent.showMore = false
  deletingAgent.value = agent
  deleteVisible.value = true
}

const confirmDelete = () => {
  if (!deletingAgent.value) return
  
  deleteAgent(deletingAgent.value.id).then((res: any) => {
    if (res.success) {
      MessagePlugin.success(t('agent.messages.deleted'))
      deleteVisible.value = false
      deletingAgent.value = null
      fetchList()
    } else {
      MessagePlugin.error(res.message || t('agent.messages.deleteFailed'))
    }
  }).catch((e: any) => {
    MessagePlugin.error(e?.message || t('agent.messages.deleteFailed'))
  })
}

const handleEditorSuccess = () => {
  editorVisible.value = false
  editingAgent.value = null
  fetchList()
}

const formatDate = (dateStr: string) => {
  if (!dateStr) return ''
  return formatStringDate(new Date(dateStr))
}

// 暴露创建方法供外部调用
const openCreateModal = () => {
  editingAgent.value = null
  editorMode.value = 'create'
  editorVisible.value = true
}

defineExpose({
  openCreateModal
})
</script>

<style scoped lang="less">
.agent-list-container {
  padding: 24px 44px;
  margin: 0 20px;
  height: calc(100vh);
  overflow-y: auto;
  box-sizing: border-box;
  flex: 1;
}

.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;

  .header-title {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  h2 {
    margin: 0;
    color: #000000e6;
    font-family: "PingFang SC";
    font-size: 24px;
    font-weight: 600;
    line-height: 32px;
  }
}

.header-subtitle {
  margin: 0;
  color: #00000099;
  font-family: "PingFang SC";
  font-size: 14px;
  font-weight: 400;
  line-height: 20px;
}

.header-divider {
  height: 1px;
  background: #e7ebf0;
  margin-bottom: 20px;
}

.agent-card-wrap {
  display: grid;
  gap: 20px;
  grid-template-columns: 1fr;
}

.agent-card {
  border: 2px solid #fbfbfb;
  border-radius: 6px;
  overflow: hidden;
  box-sizing: border-box;
  box-shadow: 0 0 8px 0 #00000005;
  background: #fff;
  position: relative;
  cursor: pointer;
  transition: all 0.2s ease;
  padding: 12px 16px 14px;
  display: flex;
  flex-direction: column;
  min-height: 150px;

  &:hover {
    border-color: #07c05f;
  }

  // 普通模式样式
  &.agent-mode-normal {
    background: linear-gradient(135deg, #ffffff 0%, #f8fcfa 100%);
    border-color: #e8f5ed;
    
    &:hover {
      border-color: #07c05f;
      background: linear-gradient(135deg, #ffffff 0%, #f0fdf4 100%);
    }
    
    .card-decoration {
      color: rgba(7, 192, 95, 0.35);
    }
    
    &:hover .card-decoration {
      color: rgba(7, 192, 95, 0.5);
    }
  }

  // Agent 模式样式
  &.agent-mode-agent {
    background: linear-gradient(135deg, #ffffff 0%, #f8f5ff 100%);
    border-color: #ede8ff;
    
    &:hover {
      border-color: #7c4dff;
      background: linear-gradient(135deg, #ffffff 0%, #f3efff 100%);
    }
    
    .card-decoration {
      color: rgba(124, 77, 255, 0.35);
    }
    
    &:hover .card-decoration {
      color: rgba(124, 77, 255, 0.5);
    }
  }

  // 确保内容在装饰之上
  .card-header,
  .card-content,
  .card-bottom {
    position: relative;
    z-index: 1;
  }
}

.card-decoration {
  position: absolute;
  top: 8px;
  right: 8px;
  display: flex;
  align-items: flex-start;
  gap: 2px;
  pointer-events: none;
  z-index: 0;
  transition: color 0.2s ease;
  
  .star-icon {
    opacity: 0.8;
    
    &.small {
      margin-top: 12px;
      opacity: 0.6;
    }
  }
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.card-header-left {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1;
  min-width: 0;
}

.card-title {
  color: #000000e6;
  font-family: "PingFang SC";
  font-size: 14px;
  font-weight: 600;
  line-height: 22px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
  min-width: 0;
}

.builtin-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border-radius: 4px;
  background: rgba(0, 0, 0, 0.05);
  color: #00000066;
  font-family: "PingFang SC";
  font-size: 12px;
  font-weight: 400;
}

.more-wrap {
  display: flex;
  width: 32px;
  height: 32px;
  justify-content: center;
  align-items: center;
  border-radius: 6px;
  cursor: pointer;
  flex-shrink: 0;
  transition: all 0.2s ease;
  opacity: 0.7;

  &:hover {
    background: rgba(0, 0, 0, 0.06);
    opacity: 1;
  }

  &.active-more {
    background: rgba(0, 0, 0, 0.08);
    opacity: 1;
  }

  .more-icon {
    width: 16px;
    height: 16px;
  }
}

.card-content {
  flex: 1;
  margin-bottom: 10px;
}

.card-description {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  overflow: hidden;
  color: #00000066;
  font-family: "PingFang SC";
  font-size: 12px;
  font-weight: 400;
  line-height: 20px;
  min-height: 40px;
}

.card-bottom {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 0 0;
  border-top: 1px solid #f0f0f0;
  margin-top: auto;
}

.bottom-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.type-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 10px;
  border-radius: 4px;
  font-family: "PingFang SC";
  font-size: 12px;
  font-weight: 500;
  line-height: 18px;

  &.normal {
    background: rgba(7, 192, 95, 0.1);
    color: #059669;
    border: 1px solid rgba(7, 192, 95, 0.2);
  }

  &.agent {
    background: rgba(124, 77, 255, 0.1);
    color: #7c4dff;
    border: 1px solid rgba(124, 77, 255, 0.2);
  }
}

.feature-badges {
  display: flex;
  align-items: center;
  gap: 6px;
}

.feature-badge {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 4px;
  cursor: pointer;

  &.web-search {
    background: rgba(255, 152, 0, 0.1);
    color: #ff9800;
    border: 1px solid rgba(255, 152, 0, 0.2);

    &:hover {
      background: rgba(255, 152, 0, 0.15);
    }
  }

  &.knowledge {
    background: rgba(7, 192, 95, 0.1);
    color: #059669;
    border: 1px solid rgba(7, 192, 95, 0.2);

    &:hover {
      background: rgba(7, 192, 95, 0.15);
    }
  }

  &.multi-turn {
    background: rgba(0, 82, 217, 0.1);
    color: #0052d9;
    border: 1px solid rgba(0, 82, 217, 0.2);

    &:hover {
      background: rgba(0, 82, 217, 0.15);
    }
  }
}

.card-time {
  color: #00000066;
  font-family: "PingFang SC";
  font-size: 12px;
  font-weight: 400;
}

.empty-state {
  flex: 1;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  padding: 60px 20px;

  .empty-img {
    width: 162px;
    height: 162px;
    margin-bottom: 20px;
  }

  .empty-txt {
    color: #00000099;
    font-family: "PingFang SC";
    font-size: 16px;
    font-weight: 600;
    line-height: 26px;
    margin-bottom: 8px;
  }

  .empty-desc {
    color: #00000066;
    font-family: "PingFang SC";
    font-size: 14px;
    font-weight: 400;
    line-height: 22px;
  }
}

// 响应式布局
@media (min-width: 900px) {
  .agent-card-wrap {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (min-width: 1250px) {
  .agent-card-wrap {
    grid-template-columns: repeat(3, 1fr);
  }
}

@media (min-width: 1600px) {
  .agent-card-wrap {
    grid-template-columns: repeat(4, 1fr);
  }
}

// 删除确认对话框样式
:deep(.del-agent-dialog) {
  padding: 0px !important;
  border-radius: 6px !important;

  .t-dialog__header {
    display: none;
  }

  .t-dialog__body {
    padding: 16px;
  }

  .t-dialog__footer {
    padding: 0;
  }
}

:deep(.t-dialog__position.t-dialog--top) {
  padding-top: 40vh !important;
}

.circle-wrap {
  .dialog-header {
    display: flex;
    align-items: center;
    margin-bottom: 8px;
  }

  .circle-img {
    width: 20px;
    height: 20px;
    margin-right: 8px;
  }

  .circle-title {
    color: #000000e6;
    font-family: "PingFang SC";
    font-size: 16px;
    font-weight: 600;
    line-height: 24px;
  }

  .del-circle-txt {
    color: #00000099;
    font-family: "PingFang SC";
    font-size: 14px;
    font-weight: 400;
    line-height: 22px;
    display: inline-block;
    margin-left: 29px;
    margin-bottom: 21px;
  }

  .circle-btn {
    height: 22px;
    width: 100%;
    display: flex;
    justify-content: flex-end;
  }

  .circle-btn-txt {
    color: #000000e6;
    font-family: "PingFang SC";
    font-size: 14px;
    font-weight: 400;
    line-height: 22px;
    cursor: pointer;

    &:hover {
      opacity: 0.8;
    }
  }

  .confirm {
    color: #FA5151;
    margin-left: 40px;

    &:hover {
      opacity: 0.8;
    }
  }
}
</style>

<style lang="less">
// 更多操作弹窗样式
.card-more-popup {
  z-index: 99 !important;

  .t-popup__content {
    padding: 6px 0 !important;
    margin-top: 6px !important;
    min-width: 140px;
    border-radius: 6px !important;
    box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.1) !important;
    border: 1px solid #e7ebf0 !important;
  }
}

.popup-menu {
  display: flex;
  flex-direction: column;
}

.popup-menu-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 16px;
  cursor: pointer;
  transition: all 0.2s ease;
  color: #000000e6;
  font-family: "PingFang SC";
  font-size: 14px;
  font-weight: 400;
  line-height: 20px;

  .menu-icon {
    font-size: 16px;
    flex-shrink: 0;
    color: #00000099;
    transition: color 0.2s ease;
  }

  &:hover {
    background: #f7f9fc;
    
    .menu-icon {
      color: #000000e6;
    }
  }

  &.delete {
    color: #000000e6;
    
    &:hover {
      background: #fff1f0;
      color: #fa5151;

      .menu-icon {
        color: #fa5151;
      }
    }
  }
}
</style>
