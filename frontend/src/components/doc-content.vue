<script setup lang="ts">
import { marked } from "marked";
import { onMounted, ref, nextTick, onUnmounted, onUpdated, watch } from "vue";
import { downKnowledgeDetails, deleteGeneratedQuestion } from "@/api/knowledge-base/index";
import { MessagePlugin, DialogPlugin } from "tdesign-vue-next";
import { sanitizeHTML, safeMarkdownToHTML, createSafeImage, isValidImageURL } from '@/utils/security';
import { useI18n } from 'vue-i18n';

const { t } = useI18n();

marked.use({
  mangle: false,
  headerIds: false,
});
const renderer = new marked.Renderer();
let page = 1;
let doc = null;
let down = ref()
let mdContentWrap = ref()
let url = ref('')
onMounted(() => {
  nextTick(() => {
    doc = document.getElementsByClassName('t-drawer__body')[0]
    doc.addEventListener('scroll', handleDetailsScroll);
  })
})
onUpdated(() => {
  page = 1
})
onUnmounted(() => {
  doc.removeEventListener('scroll', handleDetailsScroll);
})
const checkImage = (url) => {
  return new Promise((resolve) => {
    const img = new Image();
    img.onload = () => resolve(true);
    img.onerror = () => resolve(false);
    img.src = url;
  });
};
renderer.image = function (href, title, text) {
  // 安全地处理图片链接
  if (!isValidImageURL(href)) {
    return `<p>${t('error.invalidImageLink')}</p>`;
  }
  
  // 使用安全的图片创建函数
  const safeImage = createSafeImage(href, text || '', title || '');
  return `<figure>
                ${safeImage}
                <figcaption style="text-align: left;">${text || ''}</figcaption>
            </figure>`;
};
const props = defineProps(["visible", "details", "knowledgeType", "sourceInfo"]);
const emit = defineEmits(["closeDoc", "getDoc", "questionDeleted"]);
watch(() => props.details.md, (newVal) => {
  nextTick(async () => {
    const images = mdContentWrap.value.querySelectorAll('img.markdown-image');
    if (images) {
      images.forEach(async item => {
        const isValid = await checkImage(item.src);
        if (!isValid) {
          item.remove();
        }
      })
    }
  })
}, {
  immediate: true,
  deep: true
})

// 安全地处理 Markdown 内容
const processMarkdown = (markdownText) => {
  if (!markdownText || typeof markdownText !== 'string') {
    return '';
  }
  
  // 先将文本中的 <br> 标签（作为纯文本）转换为换行符
  // 这样 marked.parse 会将其正确解析为段落分隔或换行
  let processedText = markdownText.replace(/<br\s*\/?>/gi, '\n');
  
  // 首先对 Markdown 内容进行安全处理
  const safeMarkdown = safeMarkdownToHTML(processedText);
  
  // 使用安全的渲染器
  marked.use({ renderer });
  let html = marked.parse(safeMarkdown);
  
  // 如果 marked.parse 转义了 <br> 标签，将其还原为实际的 <br> 标签
  // 这样可以确保原本的 <br> 标签被正确渲染为换行
  html = html.replace(/&lt;br\s*\/?&gt;/gi, '<br>');
  
  // 使用 DOMPurify 进行最终的安全清理（br 标签在允许列表中）
  const sanitizedHTML = sanitizeHTML(html);
  
  return sanitizedHTML;
};
const handleClose = () => {
  emit("closeDoc", false);
  doc.scrollTop = 0;
};

// 获取显示标题
const getDisplayTitle = () => {
  if (!props.details.title) return '';
  if (props.details.type === 'file') {
    // 文件类型去掉扩展名
    const lastDotIndex = props.details.title.lastIndexOf(".");
    return lastDotIndex > 0 ? props.details.title.substring(0, lastDotIndex) : props.details.title;
  }
  // URL和手动创建直接返回标题
  return props.details.title;
};

// 获取类型标签
const getTypeLabel = () => {
  switch (props.details.type) {
    case 'url':
      return t('knowledgeBase.typeURL') || '网页';
    case 'manual':
      return t('knowledgeBase.typeManual') || '手动创建';
    case 'file':
      return props.details.file_type ? props.details.file_type.toUpperCase() : t('knowledgeBase.typeFile') || '文件';
    default:
      return '';
  }
};

// 获取类型主题色
const getTypeTheme = () => {
  switch (props.details.type) {
    case 'url':
      return 'primary';
    case 'manual':
      return 'success';
    case 'file':
      return 'default';
    default:
      return 'default';
  }
};

// 获取内容标签
const getContentLabel = () => {
  switch (props.details.type) {
    case 'url':
      return t('knowledgeBase.webContent') || '网页内容';
    case 'manual':
      return t('knowledgeBase.documentContent') || '文档内容';
    case 'file':
    default:
      return t('knowledgeBase.fileContent') || '文件内容';
  }
};

// 获取时间标签
const getTimeLabel = () => {
  switch (props.details.type) {
    case 'url':
      return t('knowledgeBase.importTime') || '导入时间';
    case 'manual':
      return t('knowledgeBase.createTime') || '创建时间';
    case 'file':
    default:
      return t('knowledgeBase.uploadTime') || '上传时间';
  }
};

// 获取Chunk样式类
const getChunkClass = (index: number) => {
  return index % 2 !== 0 ? 'chunk-odd' : 'chunk-even';
};

// 获取Chunk元数据
const getChunkMeta = (item: any) => {
  if (!item) return '';
  const parts = [];
  if (item.char_count) {
    parts.push(`${item.char_count} ${t('knowledgeBase.characters') || '字符'}`);
  }
  if (item.token_count) {
    parts.push(`${item.token_count} tokens`);
  }
  return parts.join(' · ');
};

// 生成的问题类型
interface GeneratedQuestion {
  id: string;
  question: string;
}

// 解析生成的问题
const getGeneratedQuestions = (item: any): GeneratedQuestion[] => {
  if (!item || !item.metadata) return [];
  try {
    const metadata = typeof item.metadata === 'string' ? JSON.parse(item.metadata) : item.metadata;
    const questions = metadata.generated_questions || [];
    // 兼容旧格式（字符串数组）和新格式（对象数组）
    return questions.map((q: string | GeneratedQuestion, index: number) => {
      if (typeof q === 'string') {
        // 旧格式：字符串，生成临时ID
        return { id: `legacy-${index}`, question: q };
      }
      return q;
    });
  } catch {
    return [];
  }
};

// 展开状态管理
const expandedChunks = ref<Set<number>>(new Set());

const toggleQuestions = (index: number) => {
  if (expandedChunks.value.has(index)) {
    expandedChunks.value.delete(index);
  } else {
    expandedChunks.value.add(index);
  }
  // 触发响应式更新
  expandedChunks.value = new Set(expandedChunks.value);
};

const isExpanded = (index: number) => expandedChunks.value.has(index);

// 删除中的状态
const deletingQuestion = ref<{ chunkIndex: number; questionId: string } | null>(null);

// 删除生成的问题
const handleDeleteQuestion = async (item: any, chunkIndex: number, question: GeneratedQuestion) => {
  if (!item || !item.id) {
    MessagePlugin.error(t('common.error') || '操作失败');
    return;
  }

  // 检查是否是旧格式数据（无法删除）
  if (question.id.startsWith('legacy-')) {
    MessagePlugin.warning(t('knowledgeBase.legacyQuestionCannotDelete') || '旧格式问题无法删除，请重新生成问题');
    return;
  }

  const confirmDialog = DialogPlugin.confirm({
    header: t('common.confirmDelete') || '确认删除',
    body: t('knowledgeBase.confirmDeleteQuestion') || '确定要删除这个问题吗？删除后将同时移除对应的向量索引。',
    confirmBtn: t('common.confirm') || '确认',
    cancelBtn: t('common.cancel') || '取消',
    onConfirm: async () => {
      confirmDialog.hide();
      deletingQuestion.value = { chunkIndex, questionId: question.id };
      try {
        await deleteGeneratedQuestion(item.id, question.id);
        MessagePlugin.success(t('common.deleteSuccess') || '删除成功');
        
        // 更新本地数据
        const metadata = typeof item.metadata === 'string' ? JSON.parse(item.metadata) : item.metadata;
        if (metadata && metadata.generated_questions) {
          const idx = metadata.generated_questions.findIndex((q: GeneratedQuestion) => q.id === question.id);
          if (idx > -1) {
            metadata.generated_questions.splice(idx, 1);
          }
          item.metadata = typeof item.metadata === 'string' ? JSON.stringify(metadata) : metadata;
        }
        
        // 通知父组件刷新数据
        emit('questionDeleted', { chunkId: item.id, questionId: question.id });
      } catch (error: any) {
        MessagePlugin.error(error?.message || t('common.deleteFailed') || '删除失败');
      } finally {
        deletingQuestion.value = null;
      }
    },
    onClose: () => {
      confirmDialog.hide();
    }
  });
};

// 检查是否正在删除某个问题
const isDeleting = (chunkIndex: number, questionId: string) => {
  return deletingQuestion.value?.chunkIndex === chunkIndex && deletingQuestion.value?.questionId === questionId;
};

const downloadFile = () => {
  downKnowledgeDetails(props.details.id)
    .then((result) => {
      if (result) {
        if (url.value) {
          URL.revokeObjectURL(url.value);
        }
        url.value = URL.createObjectURL(result);
        const link = document.createElement("a");
        link.style.display = "none";
        link.setAttribute("href", url.value);
        link.setAttribute("download", props.details.title);
        link.click();
        nextTick(() => {
          document.body.removeChild(link);
          URL.revokeObjectURL(url.value);
        })
      }
    })
    .catch((err) => {
      MessagePlugin.error(t('file.downloadFailed'));
    });
};
const handleDetailsScroll = () => {
  if (doc) {
    let pageNum = Math.ceil(props.details.total / 20);
    const { scrollTop, scrollHeight, clientHeight } = doc;
    if (scrollTop + clientHeight >= scrollHeight) {
      page++;
      if (props.details.md.length < props.details.total && page <= pageNum) {
        emit("getDoc", page);
      }
    }
  }
};
</script>
<template>
  <div class="doc_content" ref="mdContentWrap">
    <t-drawer :visible="visible" :zIndex="2000" :closeBtn="true" @close="handleClose">
      <template #header>
        <div class="drawer-header">
          <span class="header-title">{{ getDisplayTitle() }}</span>
          <t-tag v-if="details.type" size="small" :theme="getTypeTheme()" variant="light">
            {{ getTypeLabel() }}
          </t-tag>
        </div>
      </template>
      
      <!-- 文件类型专属区域 -->
      <div v-if="details.type === 'file'" class="doc_box">
        <a :href="url" style="display: none" ref="down" :download="details.title"></a>
        <span class="label">{{ $t('knowledgeBase.fileName') }}</span>
        <div class="download_box">
          <span class="doc_t">{{ details.title }}</span>
          <div class="icon_box" @click="downloadFile()">
            <img class="download_box" src="@/assets/img/download.svg" alt="">
          </div>
        </div>
      </div>
      
      <!-- URL类型专属区域 -->
      <div v-else-if="details.type === 'url'" class="url_box">
        <span class="label">{{ $t('knowledgeBase.urlSource') || '来源网址' }}</span>
        <div class="url_link_box">
          <a :href="details.source" target="_blank" class="url_link">
            <t-icon name="link" size="14px" />
            <span class="url_text">{{ details.source }}</span>
            <t-icon name="jump" size="14px" class="jump-icon" />
          </a>
        </div>
      </div>
      
      <!-- 手动创建类型专属区域 -->
      <div v-else-if="details.type === 'manual'" class="manual_box">
        <span class="label">{{ $t('knowledgeBase.documentTitle') || '文档标题' }}</span>
        <div class="manual_title_box">
          <span class="manual_title">{{ details.title }}</span>
        </div>
      </div>
      
      <div class="content_header">
        <span class="label">{{ getContentLabel() }}</span>
        <span class="time"> {{ getTimeLabel() }}：{{ details.time }} </span>
      </div>
      
      <div v-if="details.md.length == 0" class="no_content">{{ $t('common.noData') }}</div>
      <div v-else class="chunk-list">
        <div class="chunk-item" 
          v-for="(item, index) in details.md" 
          :key="index"
          :class="getChunkClass(index)"
        >
          <div class="chunk-header">
            <span class="chunk-index">{{ $t('knowledgeBase.segment') || '片段' }} {{ index + 1 }}</span>
            <div class="chunk-header-right">
              <t-tag 
                v-if="getGeneratedQuestions(item).length > 0" 
                size="small" 
                theme="success" 
                variant="light"
              >
                {{ $t('knowledgeBase.questions') || '问题' }} {{ getGeneratedQuestions(item).length }}
              </t-tag>
              <span class="chunk-meta">{{ getChunkMeta(item) }}</span>
            </div>
          </div>
          <div class="md-content" v-html="processMarkdown(item.content)"></div>
          
          <!-- 生成的问题展示 -->
          <div v-if="getGeneratedQuestions(item).length > 0" class="questions-section">
            <div class="questions-toggle" @click="toggleQuestions(index)">
              <t-icon :name="isExpanded(index) ? 'chevron-down' : 'chevron-right'" size="14px" />
              <span>{{ $t('knowledgeBase.generatedQuestions') || '生成的问题' }} ({{ getGeneratedQuestions(item).length }})</span>
            </div>
            <div v-show="isExpanded(index)" class="questions-list">
              <div 
                v-for="question in getGeneratedQuestions(item)" 
                :key="question.id" 
                class="question-item"
              >
                <t-icon name="help-circle" size="14px" class="question-icon" />
                <span class="question-text">{{ question.question }}</span>
                <t-button 
                  theme="default" 
                  variant="text" 
                  size="small"
                  class="delete-question-btn"
                  :loading="isDeleting(index, question.id)"
                  @click.stop="handleDeleteQuestion(item, index, question)"
                >
                  <template #icon>
                    <t-icon name="delete" size="14px" />
                  </template>
                </t-button>
              </div>
            </div>
          </div>
        </div>
      </div>
      
      <template #footer>
        <t-button @click="handleClose">{{ $t('common.confirm') }}</t-button>
        <t-button theme="default" @click="handleClose">{{ $t('common.cancel') }}</t-button>
      </template>
    </t-drawer>
  </div>
</template>
<style scoped lang="less">
@import "./css/markdown.less";

:deep(.t-drawer .t-drawer__content-wrapper) {
  width: 654px !important;
}

:deep(.t-drawer__header) {
  font-weight: 800;
}

:deep(.t-drawer__body.narrow-scrollbar) {
  padding: 16px 24px;
}

.drawer-header {
  display: flex;
  align-items: center;
  gap: 12px;
  
  .header-title {
    flex: 1;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.doc_box, .url_box, .manual_box {
  display: flex;
  flex-direction: column;
  margin-bottom: 16px;
}

.label {
  color: #000000e6;
  font-size: 14px;
  font-style: normal;
  font-weight: 500;
  line-height: 22px;
  margin-bottom: 8px;
}

// 文件下载区域
.download_box {
  display: flex;
  align-items: center;
}

.doc_t {
  box-sizing: border-box;
  display: flex;
  padding: 5px 8px;
  align-items: center;
  border-radius: 3px;
  border: 1px solid #dcdcdc;
  background: #30323605;
  word-break: break-all;
  text-align: justify;
}

.icon_box {
  margin-left: 18px;
  display: flex;
  overflow: hidden;
  color: #07c05f;

  .download_box {
    width: 16px;
    height: 16px;
    fill: currentColor;
    overflow: hidden;
    cursor: pointer;
  }
}

// URL链接区域
.url_link_box {
  border-radius: 4px;
  border: 1px solid #d0e8dc;
  background: #f0fdf4;
  padding: 8px 12px;
  
  .url_link {
    display: flex;
    align-items: center;
    gap: 8px;
    color: #059669;
    text-decoration: none;
    transition: all 0.2s ease;
    
    &:hover {
      color: #07c05f;
      background: #e6f7ed;
      border-radius: 3px;
      padding: 4px 6px;
      margin: -4px -6px;
      
      .jump-icon {
        transform: translateX(2px);
      }
    }
    
    .url_text {
      flex: 1;
      font-size: 13px;
      word-break: break-all;
    }
    
    .jump-icon {
      transition: transform 0.2s ease;
      flex-shrink: 0;
      color: #059669;
    }
  }
}

// 手动创建标题区域
.manual_title_box {
  border-radius: 4px;
  border: 1px solid #dcdcdc;
  background: #f0fdf4;
  padding: 8px 12px;
  
  .manual_title {
    color: #1d2129;
    font-size: 14px;
    font-weight: 500;
    word-break: break-word;
  }
}

.content_header {
  margin-top: 22px;
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.time {
  color: #00000066;
  font-size: 12px;
  font-style: normal;
  font-weight: 400;
  line-height: 20px;
}

.no_content {
  margin-top: 12px;
  color: #00000066;
  font-size: 12px;
  padding: 16px;
  background: #fbfbfb;
  text-align: center;
}

// Chunk列表样式
.chunk-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.chunk-item {
  border-radius: 6px;
  padding: 12px;
  transition: all 0.2s ease;
  border: 1px solid transparent;
  
  &.chunk-even {
    background: #3032360f;
  }
  
  &.chunk-odd {
    background: #07c05f0d;
  }
  
  &:hover {
    border-color: #07c05f;
    box-shadow: 0 2px 8px rgba(7, 192, 95, 0.1);
  }
}

.chunk-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
  padding-bottom: 6px;
  border-bottom: 1px solid #e7e7e7;
  
  .chunk-index {
    color: #00000099;
    font-size: 12px;
    font-weight: 600;
    letter-spacing: 0.5px;
  }
  
  .chunk-header-right {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  
  .chunk-meta {
    color: #00000066;
    font-size: 11px;
  }
}

// 生成的问题样式
.questions-section {
  margin-top: 12px;
  padding-top: 10px;
  border-top: 1px dashed #e0e0e0;
}

.questions-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  color: #059669;
  font-size: 12px;
  font-weight: 500;
  padding: 4px 0;
  transition: color 0.2s ease;
  
  &:hover {
    color: #07c05f;
  }
}

.questions-list {
  margin-top: 8px;
  padding-left: 4px;
}

.question-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 6px 8px;
  margin-bottom: 4px;
  background: #f0fdf4;
  border-radius: 4px;
  font-size: 13px;
  color: #1d2129;
  line-height: 1.5;
  transition: background-color 0.2s ease;
  
  &:hover {
    background: #e6f7ed;
    
    .delete-question-btn {
      opacity: 1;
    }
  }
  
  .question-icon {
    color: #059669;
    flex-shrink: 0;
    margin-top: 2px;
  }
  
  .question-text {
    flex: 1;
    word-break: break-word;
  }
  
  .delete-question-btn {
    opacity: 0;
    flex-shrink: 0;
    color: #999;
    transition: opacity 0.2s ease, color 0.2s ease;
    
    &:hover {
      color: #e34d59;
    }
  }
}

.md-content {
  word-break: break-word;
  line-height: 1.6;
  color: #1d2129;
}

// 保留旧样式作为兼容（已被chunk-item替代）
.content {
  word-break: break-word;
  padding: 4px;
  gap: 4px;
  margin-top: 12px;
}
</style>
