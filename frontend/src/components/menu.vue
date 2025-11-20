<template>
    <div class="aside_box">
        <div class="logo_box" @click="router.push('/platform/knowledge-bases')" style="cursor: pointer;">
            <img class="logo" src="@/assets/img/weknora.png" alt="">
        </div>
        
        <!-- 上半部分：知识库和对话 -->
        <div class="menu_top">
            <div v-if="showKbActions" class="kb-action-wrapper">
                <div class="kb-action-label">{{ t('knowledgeBase.quickActions') }}</div>
                <div class="kb-action-menu">
                    <template v-if="showCreateKbAction">
                        <div class="menu_item kb-action-item" @click.stop="handleCreateKnowledgeBase">
                            <div class="menu_item-box">
                                <div class="menu_icon">
                                    <t-icon name="add" />
                                </div>
                                <span class="menu_title">{{ t('knowledgeList.create') }}</span>
                            </div>
                        </div>
                    </template>
                    <template v-else-if="showDocActions">
                        <div class="menu_item kb-action-item" @click.stop="handleDocUploadClick">
                            <div class="menu_item-box">
                                <div class="menu_icon">
                                    <t-icon name="upload" />
                                </div>
                                <span class="menu_title">{{ t('upload.uploadDocument') }}</span>
                            </div>
                        </div>
                        <div class="menu_item kb-action-item" @click.stop="handleDocManualCreate">
                            <div class="menu_item-box">
                                <div class="menu_icon">
                                    <t-icon name="edit-1" />
                                </div>
                                <span class="menu_title">{{ t('upload.onlineEdit') }}</span>
                            </div>
                        </div>
                    </template>
                    <template v-else-if="showFaqActions">
                        <div class="menu_item kb-action-item" @click.stop="handleFaqCreateFromMenu">
                            <div class="menu_item-box">
                                <div class="menu_icon">
                                    <t-icon name="add" />
                                </div>
                                <span class="menu_title">{{ t('knowledgeEditor.faq.editorCreate') }}</span>
                            </div>
                        </div>
                        <div class="menu_item kb-action-item" @click.stop="handleFaqImportFromMenu">
                            <div class="menu_item-box">
                                <div class="menu_icon">
                                    <t-icon name="import" />
                                </div>
                                <span class="menu_title">{{ t('knowledgeEditor.faqImport.importButton') }}</span>
                            </div>
                        </div>
                    </template>
                </div>
            </div>
            <div class="menu_box" :class="{ 'has-submenu': item.children }" v-for="(item, index) in topMenuItems" :key="index">
                <div @click="handleMenuClick(item.path)"
                    @mouseenter="mouseenteMenu(item.path)" @mouseleave="mouseleaveMenu(item.path)"
                     :class="['menu_item', item.childrenPath && item.childrenPath == currentpath ? 'menu_item_c_active' : isMenuItemActive(item.path) ? 'menu_item_active' : '']">
                    <div class="menu_item-box">
                        <div class="menu_icon">
                            <img class="icon" :src="getImgSrc(item.icon == 'zhishiku' ? knowledgeIcon : item.icon == 'logout' ? logoutIcon : item.icon == 'setting' ? settingIcon : prefixIcon)" alt="">
                        </div>
                        <span class="menu_title" :title="item.title">{{ item.title }}</span>
                        <t-icon v-if="item.path === 'creatChat'" name="add" class="menu-create-hint" />
                    </div>
                </div>
                <div ref="submenuscrollContainer" @scroll="handleScroll" class="submenu" v-if="item.children">
                    <template v-for="(group, groupIndex) in groupedSessions" :key="groupIndex">
                        <div class="timeline_header">{{ group.label }}</div>
                        <div class="submenu_item_p" v-for="(subitem, subindex) in group.items" :key="subitem.id">
                            <div :class="['submenu_item', currentSecondpath == subitem.path ? 'submenu_item_active' : '']"
                                @mouseenter="mouseenteBotDownr(subitem.id)" @mouseleave="mouseleaveBotDown"
                                @click="gotopage(subitem.path)">
                                <span class="submenu_title"
                                    :style="currentSecondpath == subitem.path ? 'margin-left:18px;max-width:160px;' : 'margin-left:18px;max-width:185px;'">
                                    {{ subitem.title }}
                                </span>
                                <t-dropdown 
                                    :options="[{ content: t('upload.deleteRecord'), value: 'delete' }]"
                                    @click="handleSessionMenuClick($event, subitem.originalIndex, subitem)"
                                    placement="bottom-right"
                                    trigger="click">
                                    <div @click.stop class="menu-more-wrap">
                                        <t-icon name="ellipsis" class="menu-more" />
                                    </div>
                                </t-dropdown>
                            </div>
                        </div>
                    </template>
                </div>
            </div>
        </div>
        
        
        <!-- 下半部分：用户菜单 -->
        <div class="menu_bottom">
            <UserMenu />
        </div>
        
        <input
            ref="docUploadInput"
            type="file"
            class="kb-upload-input"
            accept=".pdf,.docx,.doc,.txt,.md,.jpg,.jpeg,.png"
            @change="handleDocFileChange"
        />
    </div>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia';
import { onMounted, watch, computed, ref, reactive } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { getSessionsList, delSession } from "@/api/chat/index";
import { getKnowledgeBaseById, uploadKnowledgeFile } from '@/api/knowledge-base';
import { logout as logoutApi } from '@/api/auth';
import { useMenuStore } from '@/stores/menu';
import { useAuthStore } from '@/stores/auth';
import { useUIStore } from '@/stores/ui';
import { MessagePlugin } from "tdesign-vue-next";
import UserMenu from '@/components/UserMenu.vue';
import { useI18n } from 'vue-i18n';
import { kbFileTypeVerification } from '@/utils';

const { t } = useI18n();
const usemenuStore = useMenuStore();
const authStore = useAuthStore();
const uiStore = useUIStore();
const route = useRoute();
const router = useRouter();
const currentpath = ref('');
const currentPage = ref(1);
const page_size = ref(30);
const total = ref(0);
const currentSecondpath = ref('');
const submenuscrollContainer = ref(null);
// 计算总页数
const totalPages = computed(() => Math.ceil(total.value / page_size.value));
const hasMore = computed(() => currentPage.value < totalPages.value);
type MenuItem = { title: string; icon: string; path: string; childrenPath?: string; children?: any[] };
const { menuArr } = storeToRefs(usemenuStore);
let activeSubmenu = ref<string>('');

// 是否处于知识库详情页（不包括全局聊天）
const isInKnowledgeBase = computed<boolean>(() => {
    return route.name === 'knowledgeBaseDetail' || 
           route.name === 'kbCreatChat' || 
           route.name === 'knowledgeBaseSettings';
});

// 是否在知识库列表页面
const isInKnowledgeBaseList = computed<boolean>(() => {
    return route.name === 'knowledgeBaseList';
});

// 统一的菜单项激活状态判断
const isMenuItemActive = (itemPath: string): boolean => {
    const currentRoute = route.name;
    
    switch (itemPath) {
        case 'knowledge-bases':
            return currentRoute === 'knowledgeBaseList' || 
                   currentRoute === 'knowledgeBaseDetail' || 
                   currentRoute === 'knowledgeBaseSettings';
        case 'creatChat':
            return currentRoute === 'kbCreatChat' || currentRoute === 'globalCreatChat';
        case 'settings':
            return currentRoute === 'settings';
        default:
            return itemPath === currentpath.value;
    }
};

// 统一的图标激活状态判断
const getIconActiveState = (itemPath: string) => {
    const currentRoute = route.name;
    
    return {
        isKbActive: itemPath === 'knowledge-bases' && (
            currentRoute === 'knowledgeBaseList' || 
            currentRoute === 'knowledgeBaseDetail' || 
            currentRoute === 'knowledgeBaseSettings'
        ),
        isCreatChatActive: itemPath === 'creatChat' && (currentRoute === 'kbCreatChat' || currentRoute === 'globalCreatChat'),
        isSettingsActive: itemPath === 'settings' && currentRoute === 'settings',
        isChatActive: itemPath === 'chat' && currentRoute === 'chat'
    };
};

// 分离上下两部分菜单
const topMenuItems = computed<MenuItem[]>(() => {
    return (menuArr.value as unknown as MenuItem[]).filter((item: MenuItem) => 
        item.path === 'creatChat'
    );
});

const bottomMenuItems = computed<MenuItem[]>(() => {
    return (menuArr.value as unknown as MenuItem[]).filter((item: MenuItem) => {
        if (item.path === 'knowledge-bases' || item.path === 'creatChat') {
            return false;
        }
        return true;
    });
});

// 当前知识库信息
const currentKbName = ref<string>('')
const currentKbInfo = ref<any>(null)
const docUploadInput = ref<HTMLInputElement | null>(null)
const pendingUploadKbId = ref<string | null>(null)

const showKbActions = computed(() => (isInKnowledgeBase.value && !!currentKbInfo.value) || isInKnowledgeBaseList.value)
const currentKbType = computed(() => currentKbInfo.value?.type || 'document')
const showDocActions = computed(() => showKbActions.value && isInKnowledgeBase.value && currentKbType.value !== 'faq')
const showFaqActions = computed(() => showKbActions.value && isInKnowledgeBase.value && currentKbType.value === 'faq')
const showCreateKbAction = computed(() => showKbActions.value && isInKnowledgeBaseList.value)

// 时间分组函数
const getTimeCategory = (dateStr: string): string => {
    if (!dateStr) return t('time.earlier');
    
    const date = new Date(dateStr);
    const now = new Date();
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    const yesterday = new Date(today.getTime() - 24 * 60 * 60 * 1000);
    const sevenDaysAgo = new Date(today.getTime() - 7 * 24 * 60 * 60 * 1000);
    const thirtyDaysAgo = new Date(today.getTime() - 30 * 24 * 60 * 60 * 1000);
    const oneYearAgo = new Date(today.getTime() - 365 * 24 * 60 * 60 * 1000);
    
    const sessionDate = new Date(date.getFullYear(), date.getMonth(), date.getDate());
    
    if (sessionDate.getTime() >= today.getTime()) {
        return t('time.today');
    } else if (sessionDate.getTime() >= yesterday.getTime()) {
        return t('time.yesterday');
    } else if (date.getTime() >= sevenDaysAgo.getTime()) {
        return t('time.last7Days');
    } else if (date.getTime() >= thirtyDaysAgo.getTime()) {
        return t('time.last30Days');
    } else if (date.getTime() >= oneYearAgo.getTime()) {
        return t('time.lastYear');
    } else {
        return t('time.earlier');
    }
};

// 按时间分组Session列表
const groupedSessions = computed(() => {
    const chatMenu = (menuArr.value as unknown as MenuItem[]).find((item: MenuItem) => item.path === 'creatChat');
    if (!chatMenu || !chatMenu.children || chatMenu.children.length === 0) {
        return [];
    }
    
    const groups: { [key: string]: any[] } = {
        [t('time.today')]: [],
        [t('time.yesterday')]: [],
        [t('time.last7Days')]: [],
        [t('time.last30Days')]: [],
        [t('time.lastYear')]: [],
        [t('time.earlier')]: []
    };
    
    // 将sessions按时间分组
    (chatMenu.children as any[]).forEach((session: any, index: number) => {
        const category = getTimeCategory(session.updated_at || session.created_at);
        groups[category].push({
            ...session,
            originalIndex: index
        });
    });
    
    // 按顺序返回非空分组
    const orderedLabels = [t('time.today'), t('time.yesterday'), t('time.last7Days'), t('time.last30Days'), t('time.lastYear'), t('time.earlier')];
    return orderedLabels
        .filter(label => groups[label].length > 0)
        .map(label => ({
            label,
            items: groups[label]
        }));
});

const loading = ref(false)
const mouseenteBotDownr = (val: string) => {
    activeSubmenu.value = val;
}
const mouseleaveBotDown = () => {
    activeSubmenu.value = '';
}

const handleSessionMenuClick = (data: { value: string }, index: number, item: any) => {
    if (data?.value === 'delete') {
        delCard(index, item);
    }
};

const delCard = (index: number, item: any) => {
    delSession(item.id).then((res: any) => {
        if (res && (res as any).success) {
            // 使用 originalIndex 找到正确的位置进行删除
            const actualIndex = index !== undefined ? index : 
                (menuArr.value as any[])[1]?.children?.findIndex((s: any) => s.id === item.id);
            
            if (actualIndex !== -1) {
                (menuArr.value as any[])[1]?.children?.splice(actualIndex, 1);
            }
            
            if (item.id == route.params.chatid) {
                // 删除当前会话后，跳转到全局创建聊天页面
                router.push('/platform/creatChat');
            }
        } else {
            MessagePlugin.error("删除失败，请稍后再试!");
        }
    })
}
const debounce = (fn: (...args: any[]) => void, delay: number) => {
    let timer: ReturnType<typeof setTimeout>
    return (...args: any[]) => {
        clearTimeout(timer)
        timer = setTimeout(() => fn(...args), delay)
    }
}
// 滚动处理
const checkScrollBottom = () => {
    const container = submenuscrollContainer.value
    if (!container || !container[0]) return

    const { scrollTop, scrollHeight, clientHeight } = container[0]
    const isBottom = scrollHeight - (scrollTop + clientHeight) < 100 // 触底阈值
    
    if (isBottom && hasMore.value && !loading.value) {
        currentPage.value++;
        getMessageList(true);
    }
}
const handleScroll = debounce(checkScrollBottom, 200)
const getMessageList = async (isLoadMore = false) => {
    if (loading.value) return Promise.resolve();
    loading.value = true;
    
    // 只有在首次加载或路由变化时才清空数组，滚动加载时不清空
    if (!isLoadMore) {
        currentPage.value = 1; // 重置页码
        usemenuStore.clearMenuArr();
    }
    
    return getSessionsList(currentPage.value, page_size.value).then((res: any) => {
        if (res.data && res.data.length) {
            // Display all sessions globally without filtering
            res.data.forEach((item: any) => {
                let obj = { 
                    title: item.title ? item.title : "新会话", 
                    path: `chat/${item.id}`, 
                    id: item.id, 
                    isMore: false, 
                    isNoTitle: item.title ? false : true,
                    created_at: item.created_at,
                    updated_at: item.updated_at
                }
                usemenuStore.updatemenuArr(obj)
            });
        }
        if ((res as any).total) {
            total.value = (res as any).total;
        }
        loading.value = false;
    }).catch(() => {
        loading.value = false;
    })
}

onMounted(async () => {
    const routeName = typeof route.name === 'string' ? route.name : (route.name ? String(route.name) : '')
    currentpath.value = routeName;
    if (route.params.chatid) {
        currentSecondpath.value = `chat/${route.params.chatid}`;
    }
    
    // 初始化知识库信息
    const kbId = (route.params as any)?.kbId as string
    if (kbId && isInKnowledgeBase.value) {
        try {
            const kbRes: any = await getKnowledgeBaseById(kbId)
            if (kbRes?.data) {
                currentKbName.value = kbRes.data.name || ''
                currentKbInfo.value = kbRes.data
            }
        } catch {}
    } else {
        currentKbName.value = ''
        currentKbInfo.value = null
    }
    
    // 加载对话列表
    getMessageList();
});

watch([() => route.name, () => route.params], (newvalue, oldvalue) => {
    const nameStr = typeof newvalue[0] === 'string' ? (newvalue[0] as string) : (newvalue[0] ? String(newvalue[0]) : '')
    currentpath.value = nameStr;
    if (newvalue[1].chatid) {
        currentSecondpath.value = `chat/${newvalue[1].chatid}`;
    } else {
        currentSecondpath.value = "";
    }
    
    // 只在必要时刷新对话列表，避免不必要的重新加载导致列表抖动
    // 需要刷新的情况：
    // 1. 创建新会话后（从 creatChat/kbCreatChat 跳转到 chat/:id）
    // 2. 删除会话后已在 delCard 中处理，不需要在这里刷新
    const oldRouteNameStr = typeof oldvalue?.[0] === 'string' ? (oldvalue[0] as string) : (oldvalue?.[0] ? String(oldvalue[0]) : '')
    const isCreatingNewSession = (oldRouteNameStr === 'globalCreatChat' || oldRouteNameStr === 'kbCreatChat') && 
                                 nameStr !== 'globalCreatChat' && nameStr !== 'kbCreatChat';
    
    // 只在创建新会话时才刷新列表
    if (isCreatingNewSession) {
        getMessageList();
    }
    
    // 路由变化时更新图标状态和知识库信息（不涉及对话列表）
    getIcon(nameStr);
    
    // 如果切换了知识库，更新知识库名称但不重新加载对话列表
    if (newvalue[1].kbId !== oldvalue?.[1]?.kbId) {
        const kbId = (newvalue[1] as any)?.kbId as string;
        if (kbId && isInKnowledgeBase.value) {
            getKnowledgeBaseById(kbId).then((kbRes: any) => {
                if (kbRes?.data) {
                    currentKbName.value = kbRes.data.name || '';
                    currentKbInfo.value = kbRes.data;
                }
            }).catch(() => {
                currentKbInfo.value = null;
            });
        } else {
            currentKbName.value = '';
            currentKbInfo.value = null;
        }
    }
});
let knowledgeIcon = ref('zhishiku-green.svg');
let prefixIcon = ref('prefixIcon.svg');
let logoutIcon = ref('logout.svg');
let settingIcon = ref('setting.svg'); // 设置图标
let pathPrefix = ref(route.name)
  const getIcon = (path: string) => {
      // 根据当前路由状态更新所有图标
      const kbActiveState = getIconActiveState('knowledge-bases');
      const creatChatActiveState = getIconActiveState('creatChat');
      const settingsActiveState = getIconActiveState('settings');
      
      // 知识库图标：只在知识库页面显示绿色
      knowledgeIcon.value = kbActiveState.isKbActive ? 'zhishiku-green.svg' : 'zhishiku.svg';
      
      // 对话图标：只在对话创建页面显示绿色，在知识库页面显示灰色，其他情况显示默认
      prefixIcon.value = creatChatActiveState.isCreatChatActive ? 'prefixIcon-green.svg' : 
                        kbActiveState.isKbActive ? 'prefixIcon-grey.svg' : 
                        'prefixIcon.svg';
      
      // 设置图标：只在设置页面显示绿色
      settingIcon.value = settingsActiveState.isSettingsActive ? 'setting-green.svg' : 'setting.svg';
      
      // 退出图标：始终显示默认
      logoutIcon.value = 'logout.svg';
}
getIcon(typeof route.name === 'string' ? route.name as string : (route.name ? String(route.name) : ''))
const handleMenuClick = async (path: string) => {
    if (path === 'knowledge-bases') {
        // 知识库菜单项：如果在知识库内部，跳转到当前知识库文件页；否则跳转到知识库列表
        const kbId = await getCurrentKbId()
        if (kbId) {
            router.push(`/platform/knowledge-bases/${kbId}`)
        } else {
            router.push('/platform/knowledge-bases')
        }
    } else if (path === 'settings') {
        // 设置菜单项：打开设置弹窗并跳转路由
        uiStore.openSettings()
        router.push('/platform/settings')
    } else {
        gotopage(path)
    }
}

// 处理退出登录确认
const handleLogout = () => {
    gotopage('logout')
}

const getCurrentKbId = async (): Promise<string | null> => {
    const kbId = (route.params as any)?.kbId as string
    if (isInKnowledgeBase.value && kbId) {
        return kbId
    }
    return null
}

const gotopage = async (path: string) => {
    pathPrefix.value = path;
    // 处理退出登录
    if (path === 'logout') {
        try {
            // 调用后端API注销
            await logoutApi();
        } catch (error) {
            // 即使API调用失败，也继续执行本地清理
            console.error('注销API调用失败:', error);
        }
        // 清理所有状态和本地存储
        authStore.logout();
        MessagePlugin.success('已退出登录');
        router.push('/login');
        return;
    } else {
        if (path === 'creatChat') {
            // 尝试获取当前知识库ID
            const kbId = await getCurrentKbId()
            if (kbId) {
                // 如果在知识库内部，进入该知识库的对话页
                router.push(`/platform/knowledge-bases/${kbId}/creatChat`)
            } else {
                // 如果不在知识库内，也进入对话创建页，让用户通过 @ 按钮选择知识库
                router.push(`/platform/creatChat`)
            }
        } else {
            router.push(`/platform/${path}`);
        }
    }
    getIcon(path)
}

const getImgSrc = (url: string) => {
    return new URL(`/src/assets/img/${url}`, import.meta.url).href;
}

const mouseenteMenu = (path: string) => {
    if (pathPrefix.value != 'knowledge-bases' && pathPrefix.value != 'creatChat' && path != 'knowledge-bases') {
        prefixIcon.value = 'prefixIcon-grey.svg';
    }
}
const mouseleaveMenu = (path: string) => {
    if (pathPrefix.value != 'knowledge-bases' && pathPrefix.value != 'creatChat' && path != 'knowledge-bases') {
        const nameStr = typeof route.name === 'string' ? route.name as string : (route.name ? String(route.name) : '')
        getIcon(nameStr)
    }
}

const ensureDocKnowledgeBaseReady = async (): Promise<string | null> => {
    const kbId = await getCurrentKbId()
    if (!kbId) {
        MessagePlugin.warning(t('knowledgeEditor.messages.missingId'))
        return null
    }
    if (currentKbType.value === 'faq') {
        MessagePlugin.warning(t('knowledgeBase.docActionUnsupported'))
        return null
    }
    if (!currentKbInfo.value || !currentKbInfo.value.embedding_model_id || !currentKbInfo.value.summary_model_id) {
        MessagePlugin.warning(t('knowledgeBase.notInitialized'))
        return null
    }
    return kbId
}

const handleDocUploadClick = async () => {
    const kbId = await ensureDocKnowledgeBaseReady()
    if (!kbId) return
    pendingUploadKbId.value = kbId
    docUploadInput.value?.click()
}

const handleDocFileChange = async (event: Event) => {
    const input = event.target as HTMLInputElement
    const file = input?.files?.[0]
    if (!file) {
        pendingUploadKbId.value = null
        return
    }
    if (kbFileTypeVerification(file)) {
        input.value = ''
        pendingUploadKbId.value = null
        return
    }
    const kbId = pendingUploadKbId.value || (await ensureDocKnowledgeBaseReady())
    pendingUploadKbId.value = null
    if (!kbId) {
        input.value = ''
        return
    }
    try {
        await uploadKnowledgeFile(kbId, { file })
        window.dispatchEvent(new CustomEvent('knowledgeFileUploaded', {
            detail: { kbId }
        }))
        MessagePlugin.success(t('knowledgeBase.uploadSuccess'))
    } catch (error: any) {
        let errorMessage = error?.error?.message || error?.message || t('knowledgeBase.uploadFailed')
        if (error?.code === 'duplicate_file' || error?.error?.code === 'duplicate_file') {
            errorMessage = t('knowledgeBase.fileExists')
        }
        MessagePlugin.error(errorMessage)
    } finally {
        if (input) {
            input.value = ''
        }
    }
}

const handleDocManualCreate = async () => {
    const kbId = await ensureDocKnowledgeBaseReady()
    if (!kbId) return
    uiStore.openManualEditor({
        mode: 'create',
        kbId,
        status: 'draft',
        onSuccess: ({ kbId: savedKbId }) => {
            if (savedKbId) {
                window.dispatchEvent(new CustomEvent('knowledgeFileUploaded', { detail: { kbId: savedKbId } }))
            }
        },
    })
}

const dispatchFaqMenuAction = (action: 'create' | 'import', kbId: string) => {
    window.dispatchEvent(new CustomEvent('faqMenuAction', {
        detail: { action, kbId }
    }))
}

const handleFaqCreateFromMenu = async () => {
    const kbId = await getCurrentKbId()
    if (!kbId) {
        MessagePlugin.warning(t('knowledgeEditor.messages.missingId'))
        return
    }
    dispatchFaqMenuAction('create', kbId)
}

const handleFaqImportFromMenu = async () => {
    const kbId = await getCurrentKbId()
    if (!kbId) {
        MessagePlugin.warning(t('knowledgeEditor.messages.missingId'))
        return
    }
    dispatchFaqMenuAction('import', kbId)
}

const handleCreateKnowledgeBase = () => {
    uiStore.openCreateKB()
}

</script>
<style lang="less" scoped>
.aside_box {
    min-width: 260px;
    padding: 8px;
    background: #fff;
    box-sizing: border-box;
    height: 100vh;
    overflow: hidden;
    display: flex;
    flex-direction: column;

    .logo_box {
        height: 80px;
        display: flex;
        align-items: center;
        .logo{
            width: 134px;
            height: auto;
            margin-left: 24px;
        }
    }

    .logo_img {
        margin-left: 24px;
        width: 30px;
        height: 30px;
        margin-right: 7.25px;
    }

    .logo_txt {
        transform: rotate(0.049deg);
        color: #000000;
        font-family: "TencentSans";
        font-size: 24.12px;
        font-style: normal;
        font-weight: W7;
        line-height: 21.7px;
    }

    .menu_top {
        flex: 1;
        display: flex;
        flex-direction: column;
        overflow: hidden;
        min-height: 0;
    }

    .menu_bottom {
        flex-shrink: 0;
        display: flex;
        flex-direction: column;
    }

    .kb-action-wrapper {
        border: 1px dashed #e5e7eb;
        border-radius: 8px;
        padding: 12px 8px;
        margin-bottom: 12px;
        background: #fcfefe;
    }

    .kb-action-label {
        font-size: 12px;
        color: #98a2b3;
        margin-bottom: 6px;
        padding: 0 8px;
    }

    .kb-action-menu {
        display: flex;
        flex-direction: column;
        gap: 6px;
    }

    .kb-action-item {
        background: #f7f8fa;
        border-radius: 6px;
        border: 1px solid transparent;

        .menu_icon {
            color: #07c05f;

            :deep(.t-icon) {
                font-size: 16px;
            }
        }

        .menu_title {
            font-size: 13px;
            color: #1d2129;
        }

        &:hover {
            background: #ecfdf5;
            border-color: #07c05f;
        }
    }

    .menu_box {
        display: flex;
        flex-direction: column;
        
        &.has-submenu {
            flex: 1;
            min-height: 0;
        }
    }


    .upload-file-wrap {
        padding: 6px;
        border-radius: 3px;
        height: 32px;
        width: 32px;
        box-sizing: border-box;
    }

    .upload-file-wrap:hover {
        background-color: #dbede4;
        color: #07C05F;

    }

    .upload-file-icon {
        width: 20px;
        height: 20px;
        color: rgba(0, 0, 0, 0.6);
    }

    .active-upload {
        color: #07C05F;
    }

    .menu_item_active {
        border-radius: 4px;
        background: #07c05f1a !important;

        .menu_icon,
        .menu_title {
            color: #07c05f !important;
        }

        .menu-create-hint {
            color: #07c05f !important;
            opacity: 1;
        }
    }

    .menu_item_c_active {

        .menu_icon,
        .menu_title {
            color: #000000e6;
        }
    }

    .menu_p {
        height: 56px;
        padding: 6px 0;
        box-sizing: border-box;
    }

    .menu_item {
        cursor: pointer;
        display: flex;
        align-items: center;
        justify-content: space-between;
        height: 48px;
        padding: 13px 8px 13px 16px;
        box-sizing: border-box;
        margin-bottom: 4px;

        .menu_item-box {
            display: flex;
            align-items: center;
        }

        &:hover {
            border-radius: 4px;
            background: #30323605;
            color: #00000099;

            .menu_icon,
            .menu_title {
                color: #00000099;
            }
        }
    }

    .menu_icon {
        display: flex;
        margin-right: 10px;
        color: #00000099;

        .icon {
            width: 20px;
            height: 20px;
            fill: currentColor;
            overflow: hidden;
        }
    }

    .menu_title {
        color: #00000099;
        text-overflow: ellipsis;
        font-family: "PingFang SC";
        font-size: 14px;
        font-style: normal;
        font-weight: 600;
        line-height: 22px;
        overflow: hidden;
        white-space: nowrap;
        max-width: 120px;
        flex: 1;
    }

    .submenu {
        font-family: "PingFang SC";
        font-size: 14px;
        font-style: normal;
        overflow-y: auto;
        scrollbar-width: none;
        flex: 1;
        min-height: 0;
        margin-left: 4px;
    }
    
    .timeline_header {
        font-family: "PingFang SC";
        font-size: 12px;
        font-weight: 600;
        color: #00000066;
        padding: 12px 18px 6px 18px;
        margin-top: 8px;
        line-height: 20px;
        user-select: none;
        
        &:first-child {
            margin-top: 4px;
        }
    }

    .submenu_item_p {
        height: 44px;
        padding: 4px 0px 4px 0px;
        box-sizing: border-box;
    }


    .submenu_item {
        cursor: pointer;
        display: flex;
        align-items: center;
        color: #00000099;
        font-weight: 400;
        line-height: 22px;
        height: 36px;
        padding-left: 0px;
        padding-right: 14px;
        position: relative;

        .submenu_title {
            overflow: hidden;
            white-space: nowrap;
            text-overflow: ellipsis;
        }

        .menu-more-wrap {
            margin-left: auto;
            opacity: 0;
            transition: opacity 0.2s ease;
        }

        .menu-more {
            display: inline-block;
            font-weight: bold;
            color: #07C05F;
        }

        .sub_title {
            margin-left: 14px;
        }

        &:hover {
            background: #30323605;
            color: #00000099;
            border-radius: 3px;

            .menu-more {
                color: #00000099;
            }

            .menu-more-wrap {
                opacity: 1;
            }

            .submenu_title {
                max-width: 160px !important;

            }
        }
    }

    .submenu_item_active {
        background: #07c05f1a !important;
        color: #07c05f !important;
        border-radius: 3px;

        .menu-more {
            color: #07c05f !important;
        }

        .menu-more-wrap {
            opacity: 1;
        }

        .submenu_title {
            max-width: 160px !important;
        }
    }
}

/* 知识库下拉菜单样式 */
.kb-dropdown-icon {
    margin-left: auto;
    color: #666;
    transition: transform 0.3s ease, color 0.2s ease;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 16px;
    height: 16px;
    
    &.rotate-180 {
        transform: rotate(180deg);
    }
    
    &:hover {
        color: #07c05f;
    }
    
    &.active {
        color: #07c05f;
    }
    
    &.active:hover {
        color: #05a04f;
    }
    
    svg {
        width: 12px;
        height: 12px;
        transition: inherit;
    }
}

.kb-dropdown-menu {
    position: absolute;
    top: 100%;
    left: 0;
    right: 0;
    background: #fff;
    border: 1px solid #e5e7eb;
    border-radius: 6px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
    z-index: 1000;
    max-height: 200px;
    overflow-y: auto;
}

.kb-dropdown-item {
    padding: 8px 16px;
    cursor: pointer;
    transition: background-color 0.2s ease;
    font-size: 14px;
    color: #333;
    
    &:hover {
        background-color: #f5f5f5;
    }
    
    &.active {
        background-color: #07c05f1a;
        color: #07c05f;
        font-weight: 500;
    }
    
    &:first-child {
        border-radius: 6px 6px 0 0;
    }
    
    &:last-child {
        border-radius: 0 0 6px 6px;
    }
}

.menu_item-box {
    display: flex;
    align-items: center;
    width: 100%;
    position: relative;
}

.menu-create-hint {
    margin-left: auto;
    margin-right: 8px;
    font-size: 16px;
    color: #07c05f;
    opacity: 0.7;
    transition: opacity 0.2s ease;
    flex-shrink: 0;
}

.menu_item:hover .menu-create-hint {
    opacity: 1;
}

.menu_box {
    position: relative;
}

.kb-upload-input {
    display: none;
}
</style>
<style lang="less">
// 上传操作下拉菜单样式 - 全局样式（因为 TDesign 的下拉菜单挂载到 body 上）
// 使用更具体的选择器来匹配上传操作下拉菜单
.t-popup[data-popper-placement^="right"] {
    .t-popup__content {
        .t-dropdown__menu {
            background: #ffffff !important;
            border: 1px solid #e5e7eb !important;
            border-radius: 6px !important;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1) !important;
            padding: 4px !important;
            min-width: 100px !important;
        }

        .t-dropdown__item {
            padding: 8px 12px !important;
            border-radius: 4px !important;
            margin: 2px 0 !important;
            transition: all 0.2s ease !important;
            font-size: 14px !important;
            color: #333333 !important;
            min-width: auto !important;
            max-width: none !important;
            width: auto !important;
            cursor: pointer !important;

            &:hover {
                background: #f5f7fa !important;
                color: #07c05f !important;
            }

            .t-dropdown__item-text {
                color: inherit !important;
                font-size: 14px !important;
                line-height: 20px !important;
                white-space: nowrap !important;
            }
        }
    }
}

// 退出登录确认框样式
:deep(.t-popconfirm) {
    .t-popconfirm__content {
        background: #fff;
        border: 1px solid #e7e7e7;
        border-radius: 6px;
        box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
        padding: 12px 16px;
        font-size: 14px;
        color: #333;
        max-width: 200px;
    }
    
    .t-popconfirm__arrow {
        border-bottom-color: #e7e7e7;
    }
    
    .t-popconfirm__arrow::after {
        border-bottom-color: #fff;
    }
    
    .t-popconfirm__buttons {
        margin-top: 8px;
        display: flex;
        justify-content: flex-end;
        gap: 8px;
    }
    
    .t-button--variant-outline {
        border-color: #d9d9d9;
        color: #666;
    }
    
    .t-button--theme-danger {
        background-color: #ff4d4f;
        border-color: #ff4d4f;
    }
    
    .t-button--theme-danger:hover {
        background-color: #ff7875;
        border-color: #ff7875;
    }
}
</style>