<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch, reactive, computed } from "vue";
import DocContent from "@/components/doc-content.vue";
import InputField from "@/components/Input-field.vue";
import useKnowledgeBase from '@/hooks/useKnowledgeBase';
import { useRoute, useRouter } from 'vue-router';
import EmptyKnowledge from '@/components/empty-knowledge.vue';
import { getSessionsList, createSessions, generateSessionsTitle } from "@/api/chat/index";
import { useMenuStore } from '@/stores/menu';
import { useUIStore } from '@/stores/ui';
import KnowledgeBaseEditorModal from './KnowledgeBaseEditorModal.vue';
const usemenuStore = useMenuStore();
const uiStore = useUIStore();
const router = useRouter();
import {
  batchQueryKnowledge,
  listKnowledgeFiles,
  getKnowledgeBaseById,
} from "@/api/knowledge-base/index";
import FAQEntryManager from './components/FAQEntryManager.vue';
import { formatStringDate } from "@/utils/index";
import { useI18n } from 'vue-i18n';
const route = useRoute();
const { t } = useI18n();
const kbId = computed(() => (route.params as any).kbId as string || '');
const kbInfo = ref<any>(null);
const kbLoading = ref(false);
const isFAQ = computed(() => (kbInfo.value?.type || '') === 'faq');
let { cardList, total, moreIndex, details, getKnowled, delKnowledge, openMore, onVisibleChange, getCardDetails, getfDetails } = useKnowledgeBase(kbId.value)
let isCardDetails = ref(false);
let timeout: ReturnType<typeof setInterval> | null = null;
let delDialog = ref(false)
let knowledge = ref<KnowledgeCard>({ id: '', parse_status: '' })
let knowledgeIndex = ref(-1)
let knowledgeScroll = ref()
let page = 1;
let pageSize = 35;
const getPageSize = () => {
  const viewportHeight = window.innerHeight || document.documentElement.clientHeight;
  const itemHeight = 174;
  let itemsInView = Math.floor(viewportHeight / itemHeight) * 5;
  pageSize = Math.max(35, itemsInView);
}
getPageSize()
// 直接调用 API 获取知识库文件列表
const loadKnowledgeFiles = async (kbIdValue: string) => {
  if (!kbIdValue) return;
  
  try {
    const result = await listKnowledgeFiles(kbIdValue, { page: 1, page_size: pageSize });
    
    // 由于响应拦截器已经返回了 data，所以 result 就是响应的 data 部分
    // 按照 useKnowledgeBase hook 中的方式处理
    const { data, total: totalResult } = result as any;
    
    if (!data || !Array.isArray(data)) {
      console.error('Invalid data format. Expected array, got:', typeof data, data);
      return;
    }
    
    const cardList_ = data.map((item: any) => {
      const rawName = item.file_name || item.title || item.source || t('knowledgeBase.untitledDocument')
      const dotIndex = rawName.lastIndexOf('.')
      const displayName = dotIndex > 0 ? rawName.substring(0, dotIndex) : rawName
      const fileTypeSource = item.file_type || (item.type === 'manual' ? 'MANUAL' : '')
      return {
        ...item,
        original_file_name: item.file_name,
        display_name: displayName,
        file_name: displayName,
        updated_at: formatStringDate(new Date(item.updated_at)),
        isMore: false,
        file_type: fileTypeSource ? String(fileTypeSource).toLocaleUpperCase() : '',
      };
    });
    
    cardList.value = cardList_ as any[];
    total.value = totalResult;
  } catch (err) {
    console.error('Failed to load knowledge files:', err);
  }
};

const loadKnowledgeBaseInfo = async (targetKbId: string) => {
  if (!targetKbId) {
    kbInfo.value = null;
    return;
  }
  kbLoading.value = true;
  try {
    const res: any = await getKnowledgeBaseById(targetKbId);
    kbInfo.value = res?.data || null;
    if (!isFAQ.value) {
      getKnowled({ page: 1, page_size: pageSize }, targetKbId);
      loadKnowledgeFiles(targetKbId);
    } else {
      cardList.value = [];
      total.value = 0;
    }
  } catch (error) {
    console.error('Failed to load knowledge base info:', error);
    kbInfo.value = null;
  } finally {
    kbLoading.value = false;
  }
};

// 监听路由参数变化，重新获取知识库内容
watch(() => kbId.value, (newKbId, oldKbId) => {
  if (newKbId && newKbId !== oldKbId) {
    loadKnowledgeBaseInfo(newKbId);
  }
}, { immediate: false });

// 监听文件上传事件
const handleFileUploaded = (event: CustomEvent) => {
  const uploadedKbId = event.detail.kbId;
  console.log('接收到文件上传事件，上传的知识库ID:', uploadedKbId, '当前知识库ID:', kbId.value);
  if (uploadedKbId && uploadedKbId === kbId.value && !isFAQ.value) {
    console.log('匹配当前知识库，开始刷新文件列表');
    // 如果上传的文件属于当前知识库，使用 loadKnowledgeFiles 刷新文件列表
    loadKnowledgeFiles(uploadedKbId);
  }
};

onMounted(() => {
  loadKnowledgeBaseInfo(kbId.value);
  
  // 监听文件上传事件
  window.addEventListener('knowledgeFileUploaded', handleFileUploaded as EventListener);
});

onUnmounted(() => {
  window.removeEventListener('knowledgeFileUploaded', handleFileUploaded as EventListener);
});
watch(() => cardList.value, (newValue) => {
  if (isFAQ.value) return;
  let analyzeList = [];
  analyzeList = newValue.filter(item => {
    return item.parse_status == 'pending' || item.parse_status == 'processing';
  })
  if (timeout !== null) {
    clearInterval(timeout);
    timeout = null;
  }
  if (analyzeList.length) {
    updateStatus(analyzeList)
  }
}, { deep: true })
type KnowledgeCard = {
  id: string;
  knowledge_base_id?: string;
  parse_status: string;
  description?: string;
  file_name?: string;
  original_file_name?: string;
  display_name?: string;
  title?: string;
  type?: string;
  updated_at?: string;
  file_type?: string;
  isMore?: boolean;
  metadata?: any;
  error_message?: string;
};
const updateStatus = (analyzeList: KnowledgeCard[]) => {
  let query = ``;
  for (let i = 0; i < analyzeList.length; i++) {
    query += `ids=${analyzeList[i].id}&`;
  }
  timeout = setInterval(() => {
    batchQueryKnowledge(query).then((result: any) => {
      if (result.success && result.data) {
        (result.data as KnowledgeCard[]).forEach((item: KnowledgeCard) => {
          if (item.parse_status == 'failed' || item.parse_status == 'completed') {
            let index = cardList.value.findIndex(card => card.id == item.id);
            if (index != -1) {
              cardList.value[index].parse_status = item.parse_status;
              cardList.value[index].description = item.description;
            }
          }
        });
      }
    }).catch((_err) => {
      // 错误处理
    });
  }, 1500);
};

const closeDoc = () => {
  isCardDetails.value = false;
};
const openCardDetails = (item: KnowledgeCard) => {
  isCardDetails.value = true;
  getCardDetails(item);
};

const delCard = (index: number, item: KnowledgeCard) => {
  knowledgeIndex.value = index;
  knowledge.value = item;
  delDialog.value = true;
};

const manualEditorSuccess = ({ kbId: savedKbId }: { kbId: string; knowledgeId: string; status: 'draft' | 'publish' }) => {
  if (savedKbId === kbId.value && !isFAQ.value) {
    loadKnowledgeFiles(savedKbId);
  }
};

const handleManualEdit = (index: number, item: KnowledgeCard) => {
  if (isFAQ.value) return;
  if (cardList.value[index]) {
    cardList.value[index].isMore = false;
  }
  uiStore.openManualEditor({
    mode: 'edit',
    kbId: item.knowledge_base_id || kbId.value,
    knowledgeId: item.id,
    onSuccess: manualEditorSuccess,
  });
};

const handleScroll = () => {
  if (isFAQ.value) return;
  const element = knowledgeScroll.value;
  if (element) {
    let pageNum = Math.ceil(total.value / pageSize)
    const { scrollTop, scrollHeight, clientHeight } = element;
    if (scrollTop + clientHeight >= scrollHeight) {
      page++;
      if (cardList.value.length < total.value && page <= pageNum) {
        getKnowled({ page, page_size: pageSize });
      }
    }
  }
};
const getDoc = (page: number) => {
  getfDetails(details.id, page)
};

const delCardConfirm = () => {
  delDialog.value = false;
  delKnowledge(knowledgeIndex.value, knowledge.value);
};

const sendMsg = (value: string) => {
  createNewSession(value);
};

// 处理知识库编辑成功后的回调
const handleKBEditorSuccess = (kbIdValue: string) => {
  // 如果编辑的是当前知识库，刷新文件列表
  if (kbIdValue === kbId.value) {
    loadKnowledgeFiles(kbIdValue);
  }
};

const getTitle = (session_id: string, value: string) => {
  const now = new Date().toISOString();
  let obj = { 
    title: t('knowledgeBase.newSession'), 
    path: `chat/${session_id}`, 
    id: session_id, 
    isMore: false, 
    isNoTitle: true,
    created_at: now,
    updated_at: now
  };
  usemenuStore.updataMenuChildren(obj);
  usemenuStore.changeIsFirstSession(true);
  usemenuStore.changeFirstQuery(value);
  router.push(`/platform/chat/${session_id}`);
};

async function createNewSession(value: string): Promise<void> {
  // Session 不再和知识库绑定，直接创建 Session
  createSessions({}).then(res => {
    if (res.data && res.data.id) {
      getTitle(res.data.id, value);
    } else {
      // 错误处理
      console.error(t('knowledgeBase.createSessionFailed'));
    }
  }).catch(error => {
    console.error(t('knowledgeBase.createSessionError'), error);
  });
}
</script>

<template>
  <template v-if="!isFAQ">
    <div v-show="cardList.length" class="knowledge-card-box" style="position: relative">
      <div class="knowledge-card-wrap" ref="knowledgeScroll" @scroll="handleScroll">
        <div class="knowledge-card" v-for="(item, index) in cardList" :key="index" @click="openCardDetails(item)">
          <div class="card-content">
            <div class="card-content-nav">
              <span class="card-content-title">{{ item.file_name }}</span>
              <t-popup v-model="item.isMore" overlayClassName="card-more"
                :on-visible-change="onVisibleChange" trigger="click" destroy-on-close placement="bottom-right">
                <div variant="outline" class="more-wrap" @click.stop="openMore(index)"
                  :class="[moreIndex == index ? 'active-more' : '']">
                  <img class="more" src="@/assets/img/more.png" alt="" />
                </div>
                <template #content>
                  <div class="card-menu">
                    <div
                      v-if="item.type === 'manual'"
                      class="card-menu-item"
                      @click.stop="handleManualEdit(index, item)"
                    >
                      <t-icon class="icon" name="edit" />
                      <span>{{ t('knowledgeBase.editDocument') }}</span>
                    </div>
                    <div
                      class="card-menu-item danger"
                      @click.stop="delCard(index, item)"
                    >
                      <t-icon class="icon" name="delete" />
                      <span>{{ t('knowledgeBase.deleteDocument') }}</span>
                    </div>
                  </div>
                </template>
              </t-popup>
            </div>
            <div
              v-if="item.parse_status === 'processing' || item.parse_status === 'pending'"
              class="card-analyze"
            >
              <t-icon name="loading" class="card-analyze-loading"></t-icon>
              <span class="card-analyze-txt">{{ t('knowledgeBase.parsingInProgress') }}</span>
            </div>
            <div
              v-else-if="item.parse_status === 'failed'"
              class="card-analyze failure"
            >
              <t-icon name="close-circle" class="card-analyze-loading failure"></t-icon>
              <span class="card-analyze-txt failure">{{ t('knowledgeBase.parsingFailed') }}</span>
            </div>
            <div v-else-if="item.parse_status === 'draft'" class="card-draft">
              <t-tag size="small" theme="warning" variant="light-outline">{{ t('knowledgeBase.draft') }}</t-tag>
              <span class="card-draft-tip">{{ t('knowledgeBase.draftTip') }}</span>
            </div>
            <div v-else-if="item.parse_status === 'completed'" class="card-content-txt">
              {{ item.description }}
            </div>
          </div>
          <div class="card-bottom">
            <span class="card-time">{{ item.updated_at }}</span>
            <div class="card-type">
              <span>{{ item.file_type }}</span>
            </div>
          </div>
        </div>
        <t-dialog v-model:visible="delDialog" dialogClassName="del-knowledge" :closeBtn="false" :cancelBtn="null"
          :confirmBtn="null">
          <div class="circle-wrap">
            <div class="header">
              <img class="circle-img" src="@/assets/img/circle.png" alt="">
              <span class="circle-title">{{ t('knowledgeBase.deleteConfirmation') }}</span>
            </div>
            <span class="del-circle-txt">
              {{ t('knowledgeBase.confirmDeleteDocument', { fileName: knowledge.file_name || '' }) }}
            </span>
            <div class="circle-btn">
              <span class="circle-btn-txt" @click="delDialog = false">{{ t('common.cancel') }}</span>
              <span class="circle-btn-txt confirm" @click="delCardConfirm">{{ t('knowledgeBase.confirmDelete') }}</span>
            </div>
          </div>
        </t-dialog>
      </div>
      <InputField @send-msg="sendMsg"></InputField>
      <DocContent :visible="isCardDetails" :details="details" @closeDoc="closeDoc" @getDoc="getDoc"></DocContent>
    </div>
    <EmptyKnowledge v-show="!cardList.length"></EmptyKnowledge>
  </template>
  <template v-else>
    <div class="faq-manager-wrapper">
      <FAQEntryManager v-if="kbId" :kb-id="kbId" />
    </div>
  </template>

  <!-- 知识库编辑器（创建/编辑统一组件） -->
  <KnowledgeBaseEditorModal 
    :visible="uiStore.showKBEditorModal"
    :mode="uiStore.kbEditorMode"
    :kb-id="uiStore.currentKBId || undefined"
    :initial-type="uiStore.kbEditorType"
    @update:visible="(val) => val ? null : uiStore.closeKBEditor()"
    @success="handleKBEditorSuccess"
  />
</template>
<style>
.card-more {
  z-index: 99 !important;
}

.card-more .t-popup__content {
  width: 180px;
  padding: 6px 0;
  margin-top: 4px !important;
  color: #000000e6;
}
.card-more .t-popup__content:hover {
  color: inherit !important;
}
</style>
<style scoped lang="less">
.knowledge-card-box {
  flex: 1;
}

.faq-manager-wrapper {
  flex: 1;
  padding: 24px 44px;
  overflow-y: auto;
}

@media (max-width: 1250px) and (min-width: 1045px) {
  .answers-input {
    transform: translateX(-329px);
  }

  :deep(.t-textarea__inner) {
    width: 654px !important;
  }
}

@media (max-width: 1045px) {
  .answers-input {
    transform: translateX(-250px);
  }

  :deep(.t-textarea__inner) {
    width: 500px !important;
  }
}

@media (max-width: 750px) {
  .answers-input {
    transform: translateX(-182px);
  }

  :deep(.t-textarea__inner) {
    width: 340px !important;
  }
}

@media (max-width: 600px) {
  .answers-input {
    transform: translateX(-164px);
  }

  :deep(.t-textarea__inner) {
    width: 300px !important;
  }
}

.knowledge-card-wrap {
  // padding: 24px 44px;
  padding: 24px 44px 80px 44px;
  box-sizing: border-box;
  display: grid;
  gap: 20px;
  overflow-y: auto;
  height: 100%;
  align-content: flex-start;
}

:deep(.del-knowledge) {
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
  .header {
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
    justify-content: end;
  }

  .circle-btn-txt {
    color: #000000e6;
    font-family: "PingFang SC";
    font-size: 14px;
    font-weight: 400;
    line-height: 22px;
    cursor: pointer;
  }

  .confirm {
    color: #FA5151;
    margin-left: 40px;
  }
}

.card-menu {
  display: flex;
  flex-direction: column;
  min-width: 140px;
}

.card-menu-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  cursor: pointer;
  color: #000000e6;

  &:hover {
    background: #f5f5f5;
  }

  .icon {
    font-size: 16px;
  }

  &.danger {
    color: #fa5151;
  }
}

.card-draft {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 0;
}

.card-draft-tip {
  color: #b05b00;
  font-size: 12px;
}

.knowledge-card {
  border: 2px solid #fbfbfb;
  height: 174px;
  border-radius: 6px;
  overflow: hidden;
  box-sizing: border-box;
  box-shadow: 0 0 8px 0 #00000005;
  background: #fff;
  position: relative;
  cursor: pointer;

  .card-content {
    padding: 10px 20px 23px;
  }

  .card-analyze {
    height: 66px;
    display: flex;
  }

  .card-analyze-loading {
    display: block;
    color: #07c05f;
    font-size: 15px;
    margin-top: 2px;
  }

  .card-analyze-txt {
    color: #07c05f;
    font-family: "PingFang SC";
    font-size: 12px;
    margin-left: 9px;
  }

  .failure {
    color: #fa5151;
  }

  .card-content-nav {
    display: flex;
    justify-content: space-between;
    margin-bottom: 10px;
  }

  .card-content-title {
    width: 200px;
    height: 32px;
    line-height: 32px;
    display: inline-block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: #000000e6;
    font-family: "PingFang SC";
    font-size: 14px;
    font-weight: 400;
  }

  .more-wrap {
    display: flex;
    width: 32px;
    height: 32px;
    justify-content: center;
    align-items: center;
    border-radius: 3px;
    cursor: pointer;
  }

  .more-wrap:hover {
    background: #e7e7e7;
  }

  .more {
    width: 16px;
    height: 16px;
  }

  .active-more {
    background: #e7e7e7;
  }

  .card-content-txt {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 3;
    overflow: hidden;
    color: #00000066;
    font-family: "PingFang SC";
    font-size: 12px;
    font-weight: 400;
    line-height: 20px;
  }

  .card-bottom {
    position: absolute;
    bottom: 0;
    padding: 0 20px;
    box-sizing: border-box;
    height: 32px;
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    background: rgba(48, 50, 54, 0.02);
  }

  .card-time {
    color: #00000066;
    font-family: "PingFang SC";
    font-size: 12px;
    font-weight: 400;
  }

  .card-type {
    color: #00000066;
    font-family: "PingFang SC";
    font-size: 12px;
    font-weight: 400;
    padding: 2px 4px;
    background: #3032360f;
    border-radius: 4px;
  }
}

.knowledge-card:hover {
  border: 2px solid #07c05f;
}

.knowledge-card-upload {
  color: #000000e6;
  font-family: "PingFang SC";
  font-size: 14px;
  font-weight: 400;
  cursor: pointer;

  .btn-upload {
    margin: 33px auto 0;
    width: 112px;
    height: 32px;
    border: 1px solid #dcdcdc;
    display: flex;
    justify-content: center;
    align-items: center;
    margin-bottom: 24px;
  }

  .svg-icon-download {
    margin-right: 8px;
  }
}

.upload-described {
  color: #00000066;
  font-family: "PingFang SC";
  font-size: 12px;
  font-weight: 400;
  text-align: center;
  display: block;
  width: 188px;
  margin: 0 auto;
}

.knowledge-card-wrap {
  grid-template-columns: 1fr;
}

.del-card {
  vertical-align: middle;
}

/* 小屏幕平板 - 2列 */
@media (min-width: 900px) {
  .knowledge-card-wrap {
    grid-template-columns: repeat(2, 1fr);
  }
}

/* 中等屏幕 - 3列 */
@media (min-width: 1250px) {
  .knowledge-card-wrap {
    grid-template-columns: repeat(3, 1fr);
  }
}

/* 中等屏幕 - 3列 */
@media (min-width: 1600px) {
  .knowledge-card-wrap {
    grid-template-columns: repeat(4, 1fr);
  }
}

/* 大屏幕 - 4列 */
@media (min-width: 2000px) {
  .knowledge-card-wrap {
    grid-template-columns: repeat(5, 1fr);
  }
}
</style>
