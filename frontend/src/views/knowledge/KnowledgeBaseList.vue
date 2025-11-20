<template>
  <div class="kb-list-container">
    <!-- 头部 -->
    <div class="header">
      <div class="header-title">
        <h2>{{ $t('knowledgeBase.title') }}</h2>
        <p class="header-subtitle">{{ $t('knowledgeList.subtitle') }}</p>
      </div>
    </div>
    <div class="header-divider"></div>
    
    <!-- 未初始化知识库提示 -->
    <div v-if="hasUninitializedKbs" class="warning-banner">
      <t-icon name="info-circle" size="16px" />
      <span>{{ $t('knowledgeList.uninitializedBanner') }}</span>
    </div>

    <!-- 卡片网格 -->
    <div v-if="kbs.length > 0" class="kb-card-wrap">
      <div 
        v-for="(kb, index) in kbs" 
        :key="kb.id" 
        class="kb-card"
        :class="{ 
          'uninitialized': !isInitialized(kb),
          'kb-type-document': (kb.type || 'document') === 'document',
          'kb-type-faq': kb.type === 'faq'
        }"
        @click="handleCardClick(kb)"
      >
        <!-- 卡片头部 -->
        <div class="card-header">
          <span class="card-title" :title="kb.name">{{ kb.name }}</span>
          <t-popup 
            v-model="kb.showMore" 
            overlayClassName="card-more-popup"
            :on-visible-change="onVisibleChange"
            trigger="click" 
            destroy-on-close 
            placement="bottom-right"
          >
            <div 
              variant="outline" 
              class="more-wrap" 
              @click.stop="openMore(index)"
              :class="{ 'active-more': currentMoreIndex === index }"
            >
              <img class="more-icon" src="@/assets/img/more.png" alt="" />
            </div>
            <template #content>
              <div class="popup-menu" @click.stop>
                <div class="popup-menu-item" @click.stop="handleSettings(kb)">
                  <t-icon class="menu-icon" name="setting" />
                  <span>{{ $t('knowledgeBase.settings') }}</span>
                </div>
                <div class="popup-menu-item delete" @click.stop="handleDelete(kb)">
                  <t-icon class="menu-icon" name="delete" />
                  <span>{{ $t('common.delete') }}</span>
                </div>
              </div>
            </template>
          </t-popup>
        </div>

        <!-- 卡片内容 -->
        <div class="card-content">
          <div class="card-description">
            {{ kb.description || $t('knowledgeBase.noDescription') }}
          </div>
        </div>

        <!-- 卡片底部 -->
        <div class="card-bottom">
          <div class="bottom-left">
            <div class="type-badge" :class="{ 'document': (kb.type || 'document') === 'document', 'faq': kb.type === 'faq' }">
              <t-icon :name="kb.type === 'faq' ? 'chat-bubble-help' : 'folder'" size="14px" />
              <span>{{ kb.type === 'faq' ? $t('knowledgeEditor.basic.typeFAQ') : $t('knowledgeEditor.basic.typeDocument') }}</span>
            </div>
            <div class="feature-badges">
              <t-tooltip v-if="kb.extract_config?.enabled" :content="$t('knowledgeList.features.knowledgeGraph')" placement="top">
                <div class="feature-badge kg">
                  <t-icon name="relation" size="14px" />
                </div>
              </t-tooltip>
              <t-tooltip v-if="kb.vlm_config?.enabled || (kb.cos_config?.provider && kb.cos_config?.bucket_name)" :content="$t('knowledgeList.features.multimodal')" placement="top">
                <div class="feature-badge multimodal">
                  <t-icon name="image" size="14px" />
                </div>
              </t-tooltip>
            </div>
          </div>
          <span class="card-time">{{ kb.updated_at }}</span>
        </div>
      </div>
    </div>

    <!-- 空状态 -->
    <div v-else-if="!loading" class="empty-state">
      <img class="empty-img" src="@/assets/img/upload.svg" alt="">
      <span class="empty-txt">{{ $t('knowledgeList.empty.title') }}</span>
      <span class="empty-desc">{{ $t('knowledgeList.empty.description') }}</span>
    </div>


    <!-- 删除确认对话框 -->
    <t-dialog 
      v-model:visible="deleteVisible" 
      dialogClassName="del-knowledge-dialog" 
      :closeBtn="false" 
      :cancelBtn="null"
      :confirmBtn="null"
    >
      <div class="circle-wrap">
        <div class="dialog-header">
          <img class="circle-img" src="@/assets/img/circle.png" alt="">
          <span class="circle-title">{{ $t('knowledgeList.delete.confirmTitle') }}</span>
        </div>
        <span class="del-circle-txt">
          {{ $t('knowledgeList.delete.confirmMessage', { name: deletingKb?.name ?? '' }) }}
        </span>
        <div class="circle-btn">
          <span class="circle-btn-txt" @click="deleteVisible = false">{{ $t('common.cancel') }}</span>
          <span class="circle-btn-txt confirm" @click="confirmDelete">{{ $t('knowledgeList.delete.confirmButton') }}</span>
        </div>
      </div>
    </t-dialog>

    <!-- 知识库编辑器（创建/编辑统一组件） -->
    <KnowledgeBaseEditorModal 
      :visible="uiStore.showKBEditorModal"
      :mode="uiStore.kbEditorMode"
      :kb-id="uiStore.currentKBId || undefined"
      :initial-type="uiStore.kbEditorType"
      @update:visible="(val) => val ? null : uiStore.closeKBEditor()"
      @success="handleKBEditorSuccess"
    />
    
    <!-- 全局设置模态框 -->
    <Settings />
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { listKnowledgeBases, deleteKnowledgeBase } from '@/api/knowledge-base'
import { formatStringDate } from '@/utils/index'
import { useUIStore } from '@/stores/ui'
import KnowledgeBaseEditorModal from './KnowledgeBaseEditorModal.vue'
import Settings from '@/views/settings/Settings.vue'
import { useI18n } from 'vue-i18n'

const router = useRouter()
const uiStore = useUIStore()
const { t } = useI18n()

interface KB { 
  id: string; 
  name: string; 
  description?: string; 
  updated_at?: string;
  embedding_model_id?: string;
  summary_model_id?: string;
  type?: 'document' | 'faq';
  showMore?: boolean;
  vlm_config?: { enabled?: boolean; model_id?: string };
  extract_config?: { enabled?: boolean };
  cos_config?: { provider?: string; bucket_name?: string };
}

const kbs = ref<KB[]>([])
const loading = ref(false)
const deleteVisible = ref(false)
const deletingKb = ref<KB | null>(null)
const currentMoreIndex = ref<number>(-1)

const fetchList = () => {
  loading.value = true
  listKnowledgeBases().then((res: any) => {
    const data = res.data || []
    // 格式化时间，并初始化 showMore 状态
    kbs.value = data.map((kb: KB) => ({
      ...kb,
      updated_at: kb.updated_at ? formatStringDate(new Date(kb.updated_at)) : '',
      showMore: false
    }))
  }).finally(() => loading.value = false)
}

onMounted(() => {
  fetchList()
})

const openMore = (index: number) => {
  // 只记录当前打开的索引，用于显示激活样式
  // 弹窗的开关由 v-model 自动管理
  currentMoreIndex.value = index
}

const onVisibleChange = (visible: boolean) => {
  // 弹窗关闭时重置索引
  if (!visible) {
    currentMoreIndex.value = -1
  }
}

const handleSettings = (kb: KB) => {
  // 手动关闭弹窗
  kb.showMore = false
  goSettings(kb.id)
}

const handleDelete = (kb: KB) => {
  // 手动关闭弹窗
  kb.showMore = false
  deletingKb.value = kb
  deleteVisible.value = true
}

const confirmDelete = () => {
  if (!deletingKb.value) return
  
  deleteKnowledgeBase(deletingKb.value.id).then((res: any) => {
    if (res.success) {
      MessagePlugin.success(t('knowledgeList.messages.deleted'))
      deleteVisible.value = false
      deletingKb.value = null
      fetchList()
    } else {
      MessagePlugin.error(res.message || t('knowledgeList.messages.deleteFailed'))
    }
  }).catch((e: any) => {
    MessagePlugin.error(e?.message || t('knowledgeList.messages.deleteFailed'))
  })
}

const isInitialized = (kb: KB) => {
  return !!(kb.embedding_model_id && kb.embedding_model_id !== '' && 
            kb.summary_model_id && kb.summary_model_id !== '')
}

// 计算是否有未初始化的知识库
const hasUninitializedKbs = computed(() => {
  return kbs.value.some(kb => !isInitialized(kb))
})

const handleCardClick = (kb: KB) => {
  if (isInitialized(kb)) {
    goDetail(kb.id)
  } else {
    goSettings(kb.id)
  }
}

const goDetail = (id: string) => {
  router.push(`/platform/knowledge-bases/${id}`)
}

const goSettings = (id: string) => {
  // 使用模态框打开设置
  uiStore.openKBSettings(id)
}

// 知识库编辑器成功回调（创建或编辑成功）
const handleKBEditorSuccess = (kbId: string) => {
  console.log('[KnowledgeBaseList] knowledge operation success:', kbId)
  fetchList()
}
</script>

<style scoped lang="less">
.kb-list-container {
  padding: 24px 44px;
  // background: #fff;
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

.warning-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  margin-bottom: 20px;
  background: #fff7e6;
  border: 1px solid #ffd591;
  border-radius: 6px;
  color: #d46b08;
  font-family: "PingFang SC";
  font-size: 14px;
  
  .t-icon {
    color: #d46b08;
    flex-shrink: 0;
  }
}

.kb-card-wrap {
  display: grid;
  gap: 20px;
  grid-template-columns: 1fr;
}

.kb-card {
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

  &.uninitialized {
    opacity: 0.9;
  }

  // 文档类型样式
  &.kb-type-document {
    background: linear-gradient(135deg, #ffffff 0%, #f8fcfa 100%);
    border-color: #e8f5ed;
    
    &:hover {
      border-color: #07c05f;
      background: linear-gradient(135deg, #ffffff 0%, #f0fdf4 100%);
    }
    
    // 右上角装饰
    &::after {
      content: '';
      position: absolute;
      top: 0;
      right: 0;
      width: 60px;
      height: 60px;
      background: linear-gradient(135deg, rgba(7, 192, 95, 0.08) 0%, transparent 100%);
      border-radius: 0 6px 0 100%;
      pointer-events: none;
      z-index: 0;
    }
  }

  // 问答类型样式
  &.kb-type-faq {
    background: linear-gradient(135deg, #ffffff 0%, #f8fbff 100%);
    border-color: #e6f0ff;
    
    &:hover {
      border-color: #0052d9;
      background: linear-gradient(135deg, #ffffff 0%, #eff6ff 100%);
    }
    
    // 右上角装饰
    &::after {
      content: '';
      position: absolute;
      top: 0;
      right: 0;
      width: 60px;
      height: 60px;
      background: linear-gradient(135deg, rgba(0, 82, 217, 0.08) 0%, transparent 100%);
      border-radius: 0 6px 0 100%;
      pointer-events: none;
      z-index: 0;
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

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
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
  margin-right: 8px;
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

  &.document {
    background: rgba(7, 192, 95, 0.1);
    color: #059669;
    border: 1px solid rgba(7, 192, 95, 0.2);
  }

  &.faq {
    background: rgba(0, 82, 217, 0.1);
    color: #0052d9;
    border: 1px solid rgba(0, 82, 217, 0.2);
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

  &.kg {
    background: rgba(124, 77, 255, 0.1);
    color: #7c4dff;
    border: 1px solid rgba(124, 77, 255, 0.2);

    &:hover {
      background: rgba(124, 77, 255, 0.15);
    }
  }

  &.multimodal {
    background: rgba(255, 152, 0, 0.1);
    color: #ff9800;
    border: 1px solid rgba(255, 152, 0, 0.2);

    &:hover {
      background: rgba(255, 152, 0, 0.15);
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
  .kb-card-wrap {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (min-width: 1250px) {
  .kb-card-wrap {
    grid-template-columns: repeat(3, 1fr);
  }
}

@media (min-width: 1600px) {
  .kb-card-wrap {
    grid-template-columns: repeat(4, 1fr);
  }
}

// 删除确认对话框样式
:deep(.del-knowledge-dialog) {
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

// 创建对话框样式优化
.create-kb-dialog {
  .t-form-item__label {
    font-family: "PingFang SC";
    font-size: 14px;
    font-weight: 500;
    color: #000000e6;
  }

  .t-input,
  .t-textarea {
    font-family: "PingFang SC";
  }

  .t-button--theme-primary {
    background-color: #07c05f;
    border-color: #07c05f;

    &:hover {
      background-color: #05a04f;
      border-color: #05a04f;
    }
  }
}
</style>
