<script setup lang="ts">
import { marked } from "marked";
import { onMounted, ref, nextTick, onUnmounted, onUpdated, watch } from "vue";
import { downKnowledgeDetails } from "@/api/knowledge-base/index";
import { MessagePlugin } from "tdesign-vue-next";
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
const props = defineProps(["visible", "details"]);
const emit = defineEmits(["closeDoc", "getDoc"]);
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
      <template #header>{{
        details.title.substring(0, details.title.lastIndexOf("."))
      }}</template>
      <div class="doc_box">
        <a :href="url" style="display: none" ref="down" :download="details.title"></a>
        <span class="label">{{ $t('knowledgeBase.fileName') }}</span>
        <div class="download_box">
          <span class="doc_t">{{ details.title }}</span>
          <div class="icon_box" @click="downloadFile()">
            <img class="download_box" src="@/assets/img/download.svg" alt="">
          </div>
        </div>
      </div>
      <div class="content_header">
        <span class="label">{{ $t('knowledgeBase.fileContent') }}</span>
        <span class="time"> {{ $t('knowledgeBase.uploadTime') }}：{{ details.time }} </span>
      </div>
      <div v-if="details.md.length == 0" class="no_content">{{ $t('common.noData') }}</div>
      <div v-else class="content" v-for="(item, index) in details.md" :key="index" :style="index % 2 !== 0
        ? 'background: #07c05f26;'
        : 'background: #3032360f;'
        ">
        <div class="md-content" v-html="processMarkdown(item.content)"></div>
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

.content {
  word-break: break-word;
  padding: 4px;
  gap: 4px;
  margin-top: 12px;
}

.doc_box {
  display: flex;
  flex-direction: column;
}

.label {
  color: #000000e6;
  font-size: 14px;
  font-style: normal;
  font-weight: 500;
  line-height: 22px;
  margin-bottom: 8px;
}

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

.content_header {
  margin-top: 22px;
  margin-bottom: 24px;
}

.time {
  margin-left: 12px;
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
}
</style>
