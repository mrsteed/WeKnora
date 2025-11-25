<template>
    <div class="bot_msg">
        <div style="display: flex;flex-direction: column; gap:8px">
            <docInfo :session="session"></docInfo>
            <AgentStreamDisplay :session="session" :user-query="userQuery" v-if="session.isAgentMode"></AgentStreamDisplay>
            <deepThink :deepSession="session" v-if="session.showThink && !session.isAgentMode"></deepThink>
        </div>
        <!-- 非 Agent 模式下才显示传统的 markdown 渲染 -->
        <div ref="parentMd" v-if="!session.hideContent && !session.isAgentMode">
            <!-- 消息正在总结中则渲染加载动画  -->
            <div v-if="session.thinking" class="thinking-loading">
                <div class="loading-typing">
                    <span></span>
                    <span></span>
                    <span></span>
                </div>
            </div>
            <!-- 直接渲染完整内容，避免切分导致的问题，样式与 thinking 一致 -->
            <div class="content-wrapper">
                <div class="ai-markdown-template markdown-content" v-html="processMarkdown(content || session.content)"></div>
            </div>
            <div v-if="isImgLoading" class="img_loading"><t-loading size="small"></t-loading><span>{{ $t('common.loading') }}</span></div>
        </div>
        <picturePreview :reviewImg="reviewImg" :reviewUrl="reviewUrl" @closePreImg="closePreImg"></picturePreview>
    </div>
</template>
<script setup>
import { onMounted, watch, computed, ref, reactive, defineProps, nextTick } from 'vue';
import { marked } from 'marked';
import docInfo from './docInfo.vue';
import deepThink from './deepThink.vue';
import AgentStreamDisplay from './AgentStreamDisplay.vue';
import picturePreview from '@/components/picture-preview.vue';
import { sanitizeHTML, safeMarkdownToHTML, createSafeImage, isValidImageURL } from '@/utils/security';
import { useI18n } from 'vue-i18n';

marked.use({
    mangle: false,
    headerIds: false,
    breaks: true,  // 全局启用单个换行支持
});
const emit = defineEmits(['scroll-bottom'])
const { t } = useI18n()
const renderer = new marked.Renderer();
let parentMd = ref()
let reviewUrl = ref('')
let reviewImg = ref(false)
let isImgLoading = ref(false);
const props = defineProps({
    // 必填项
    content: {
        type: String,
        required: false
    },
    session: {
        type: Object,
        required: false
    },
    userQuery: {
        type: String,
        required: false,
        default: ''
    },
    isFirstEnter: {
        type: Boolean,
        required: false
    }
});
const processedMarkdown = ref([]);
const preview = (url) => {
    nextTick(() => {
        reviewUrl.value = url;
        reviewImg.value = true
    })
}
const removeImg = () => {
    nextTick(() => {
        if (!parentMd.value) return;
        const images = parentMd.value.querySelectorAll('img.ai-markdown-img');
        if (images) {
            images.forEach(async item => {
                const isValid = await checkImage(item.src);
                if (!isValid) {
                    item.remove();
                }
            })
        }
    })
}
const closePreImg = () => {
    reviewImg.value = false
    reviewUrl.value = '';
}
const debounce = (fn, delay) => {
    let timer
    return (...args) => {
        clearTimeout(timer)
        timer = setTimeout(() => fn(...args), delay)
    }
}
const checkImage = (url) => {
    return new Promise((resolve) => {
        const img = new Image();
        img.onload = () => {
            resolve(true);
        }
        img.onerror = () => resolve(false);
        img.src = url;
    });
};
// 安全地处理 Markdown 内容
const processMarkdown = (markdownText) => {
    if (!markdownText || typeof markdownText !== 'string') {
        return '';
    }
    
    // 首先对 Markdown 内容进行安全处理
    const safeMarkdown = safeMarkdownToHTML(markdownText);
    
    // 创建自定义渲染器实例，继承默认渲染器
    const customRenderer = new marked.Renderer();
    // 覆盖图片渲染方法
    customRenderer.image = function(href, title, text) {
        // 验证图片 URL 是否安全
        if (!isValidImageURL(href)) {
            return `<p>${t('error.invalidImageLink')}</p>`;
        }
        // 使用安全的图片创建函数
        return createSafeImage(href, text || '', title || '');
    };

    // 创建临时的 marked 配置，包含 renderer 和 breaks 选项
    // breaks: true 会将单个换行渲染为 <br>，而不是忽略
    const markedOptions = {
        renderer: customRenderer,
        breaks: true  // 启用单个换行支持
    };

    // 安全地渲染 Markdown，直接传递选项
    let html = marked.parse(safeMarkdown, markedOptions);

    // 使用 DOMPurify 进行最终的安全清理
    const sanitizedHTML = sanitizeHTML(html);
    
    return sanitizedHTML;
};
const handleImg = async (newVal) => {
    let index = newVal.lastIndexOf('![');
    if (index != -1) {
        isImgLoading.value = true;
        let str = newVal.slice(index)
        if (str.includes('](') && str.includes(')')) {
            processedMarkdown.value = splitMarkdownByImages(newVal)
            isImgLoading.value = false;
        } else {
            processedMarkdown.value = splitMarkdownByImages(newVal.slice(0, index))
        }
    } else {
        processedMarkdown.value = splitMarkdownByImages(newVal)
    }
    removeImg()
}
function splitMarkdownByImages(markdown) {
    const imageRegex = /!\[.*?\]\(\s*(?:<([^>]*)>|([^)\s]*))\s*(?:["'].*?["'])?\s*\)/g;
    const result = [];
    let lastIndex = 0;
    let match;

    while ((match = imageRegex.exec(markdown)) !== null) {
        const textBefore = markdown.slice(lastIndex, match.index);
        if (textBefore) result.push(textBefore);
        const url = match[1] || match[2];
        result.push(url);
        lastIndex = imageRegex.lastIndex;
    }

    const remainingText = markdown.slice(lastIndex);
    if (remainingText) result.push(remainingText);

    return result;
}
function isLink(str) {
    const trimmedStr = str.trim();
    // 正则表达式匹配常见链接格式
    const urlPattern = /^(https?:\/\/|ftp:\/\/|www\.)(?:(?:[\w-]+(?:\.[\w-]+)*)|(?:\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})|(?:\[[a-fA-F0-9:]+\]))(?::\d{1,5})?(?:[\/\w.,@?^=%&:~+#-]*[\w@?^=%&\/~+#-])?/i;
    return urlPattern.test(trimmedStr);
}

watch(() => props.content, (newVal) => {
    debounce(handleImg(newVal), 800)
}, {
    immediate: true
})

const myMarkdown = (res) => {
    return marked.parse(res, { renderer })
}

onMounted(async () => {
    processedMarkdown.value = splitMarkdownByImages(props.content);
    removeImg()
});
</script>
<style lang="less" scoped>
@import '../../../components/css/markdown.less';

// 内容包装器 - 与 thinking 样式一致
.content-wrapper {
    background: #ffffff;
    border-radius: 8px;
    padding: 1px 14px;
    box-shadow: 0 2px 4px rgba(7, 192, 95, 0.08);
    transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
    animation: fadeInUp 0.3s ease-out;
}

@keyframes fadeInUp {
    from {
        opacity: 0;
        transform: translateY(8px);
    }
    to {
        opacity: 1;
        transform: translateY(0);
    }
}

.ai-markdown-template {
    font-size: 14px;
    color: #333333;
    line-height: 1.6;
    /* 确保换行符正确显示 */
    // white-space: pre-line;  /* 保留换行符，但合并多个空格 */
}

.markdown-content {
    :deep(p) {
        margin: 8px 0;
        line-height: 1.6;
    }

    :deep(code) {
        background: #f0f0f0;
        padding: 2px 6px;
        border-radius: 3px;
        font-family: 'Monaco', 'Courier New', monospace;
        font-size: 12px;
    }

    :deep(pre) {
        background: #f5f5f5;
        padding: 12px;
        border-radius: 4px;
        overflow-x: auto;
        margin: 8px 0;

        code {
            background: none;
            padding: 0;
        }
    }

    :deep(ul), :deep(ol) {
        margin: 8px 0;
        padding-left: 24px;
    }

    :deep(li) {
        margin: 4px 0;
    }

    :deep(blockquote) {
        border-left: 3px solid #07c05f;
        padding-left: 12px;
        margin: 8px 0;
        color: #666;
    }

    :deep(h1), :deep(h2), :deep(h3), :deep(h4), :deep(h5), :deep(h6) {
        margin: 12px 0 8px 0;
        font-weight: 600;
        color: #333;
    }

    :deep(a) {
        color: #07c05f;
        text-decoration: none;

        &:hover {
            text-decoration: underline;
        }
    }

    :deep(table) {
        border-collapse: collapse;
        margin: 8px 0;
        font-size: 12px;
        width: 100%;

        th, td {
            border: 1px solid #e5e7eb;
            padding: 6px 10px;
            text-align: left;
    }

        th {
            background: #f5f5f5;
            font-weight: 600;
        }

        tbody tr:nth-child(even) {
            background: #fafafa;
        }
    }
}

.ai-markdown-img {
    border-radius: 8px;
    display: block;
    cursor: pointer;
    object-fit: scale-down;
    contain: content;
    margin-left: 16px;
    border: 0.5px solid #E7E7E7;
    max-width: 708px;
    height: 230px;
}

.bot_msg {
    // background: #fff;
    border-radius: 4px;
    color: rgba(0, 0, 0, 0.9);
    font-size: 16px;
    // padding: 10px 12px;
    margin-right: auto;
    max-width: 100%;
    box-sizing: border-box;
}

.botanswer_laoding_gif {
    width: 24px;
    height: 18px;
    margin-left: 16px;
}

.thinking-loading {
    margin-left: 16px;
    margin-bottom: 8px;
}

.loading-typing {
    display: flex;
    align-items: center;
    gap: 4px;
    
    span {
        width: 6px;
        height: 6px;
        border-radius: 50%;
        background: #07c05f;
        animation: typingBounce 1.4s ease-in-out infinite;
        
        &:nth-child(1) {
            animation-delay: 0s;
        }
        
        &:nth-child(2) {
            animation-delay: 0.2s;
        }
        
        &:nth-child(3) {
            animation-delay: 0.4s;
        }
    }
}

@keyframes typingBounce {
    0%, 60%, 100% {
        transform: translateY(0);
    }
    30% {
        transform: translateY(-8px);
    }
}

.img_loading {
    background: #3032360f;
    height: 230px;
    width: 230px;
    color: #00000042;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-direction: column;
    font-size: 12px;
    gap: 4px;
    margin-left: 16px;
    border-radius: 8px;
}

:deep(.t-loading__gradient-conic) {
    background: conic-gradient(from 90deg at 50% 50%, #fff 0deg, #676767 360deg) !important;

}
</style>