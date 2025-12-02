<template>
  <div class="tenant-selector">
    <div v-if="canAccessAllTenants" class="tenant-selector-wrapper">
      <div class="tenant-menu-item" @click="toggleDropdown" ref="triggerRef">
        <div class="tenant-item-box">
          <div class="tenant-icon">
            <t-icon name="usergroup" size="16px" />
          </div>
          <span class="tenant-title">{{ currentTenantName }}</span>
        </div>
        <t-icon name="chevron-up" class="tenant-arrow" :class="{ rotated: showDropdown }" />
      </div>
      <div v-if="showDropdown" class="tenant-overlay" @click="close">
        <div class="tenant-dropdown" @click.stop :style="dropdownStyle">
          <div class="tenant-list" ref="tenantList">
            <div
              v-for="tenant in tenants"
              :key="tenant.id"
              :class="['tenant-item', { selected: isSelected(tenant.id) }]"
              @click="selectTenant(tenant.id)"
            >
              <div class="tenant-item-info">
                <span class="tenant-item-name">{{ tenant.name }}</span>
                <span v-if="tenant.description" class="tenant-item-desc">{{ tenant.description }}</span>
                <span class="tenant-item-id">ID: {{ tenant.id }}</span>
              </div>
              <t-icon v-if="isSelected(tenant.id)" name="check" size="16px" class="tenant-check-icon" />
            </div>
            <div v-if="tenants.length === 0 && !loading" class="tenant-empty">
              {{ $t('tenant.noMatch') }}
            </div>
            <div v-if="loading" class="tenant-loading">
              {{ $t('tenant.loading') }}
            </div>
            <div v-if="hasMore && !loading" class="tenant-load-more" @click="loadMore">
              {{ $t('tenant.loadMore') }}
            </div>
          </div>
          <div class="tenant-search">
            <input
              ref="searchInput"
              v-model="searchQuery"
              type="text"
              :placeholder="$t('tenant.searchPlaceholder')"
              class="tenant-search-input"
              @keydown.esc="closeDropdown"
              @input="handleSearchInput"
            />
            <div class="tenant-search-hint">
              {{ $t('tenant.searchHint') }}
            </div>
          </div>
        </div>
      </div>
    </div>
    <div v-else class="tenant-menu-item readonly">
      <div class="tenant-item-box">
        <div class="tenant-icon">
          <t-icon name="usergroup" size="16px" />
        </div>
        <span class="tenant-title">{{ currentTenantName }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch, onUnmounted, nextTick } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { searchTenants, type TenantInfo } from '@/api/tenant'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'

const { t } = useI18n()
const authStore = useAuthStore()

const showDropdown = ref(false)
const searchQuery = ref('')
const tenants = ref<TenantInfo[]>([])
const triggerRef = ref<HTMLElement | null>(null)
const tenantList = ref<HTMLElement | null>(null)
const searchInput = ref<HTMLInputElement | null>(null)
const dropdownStyle = ref<Record<string, string>>({})

// 分页相关
const currentPage = ref(1)
const pageSize = ref(20)
const total = ref(0)
const loading = ref(false)
const searchTimer = ref<number | null>(null)

const canAccessAllTenants = computed(() => authStore.canAccessAllTenants)
const selectedTenantId = computed(() => authStore.selectedTenantId)
const defaultTenantId = computed(() => authStore.tenant?.id ? Number(authStore.tenant.id) : null)

const currentTenantId = computed(() => {
  return selectedTenantId.value || defaultTenantId.value
})

const currentTenantName = computed(() => {
  if (!currentTenantId.value) return t('tenant.unknown')
  const tenant = tenants.value.find(t => t.id === currentTenantId.value)
  if (tenant) return tenant.name
  return authStore.tenant?.name || t('tenant.unknown')
})

const hasMore = computed(() => {
  return tenants.value.length < total.value
})

const isSelected = (tenantId: number) => {
  return currentTenantId.value === tenantId
}

const updateDropdownPosition = () => {
  if (!triggerRef.value) return
  
  const rect = triggerRef.value.getBoundingClientRect()
  const dropdownMaxHeight = 400 // max-height
  const spaceAbove = rect.top
  const padding = 16
  
  // 计算可用高度，确保有足够空间显示搜索框
  const availableHeight = Math.min(dropdownMaxHeight, spaceAbove - padding)
  
  // 确保最小高度（至少能显示搜索框和一些列表项）
  const minHeight = 200
  const finalHeight = Math.max(minHeight, availableHeight)
  
  // 向上弹出，确保不遮挡用户头像
  dropdownStyle.value = {
    bottom: `${window.innerHeight - rect.top + 8}px`,
    left: `${rect.left}px`,
    width: '280px',
    height: `${finalHeight}px`,
    maxHeight: `${finalHeight}px`
  }
}

const toggleDropdown = () => {
  if (!canAccessAllTenants.value) return
  showDropdown.value = !showDropdown.value
  if (showDropdown.value) {
    if (tenants.value.length === 0) {
      loadTenants()
    }
    nextTick(() => {
      updateDropdownPosition()
      searchInput.value?.focus()
    })
  } else {
    // 关闭时重置搜索
    searchQuery.value = ''
    currentPage.value = 1
    tenants.value = []
    total.value = 0
  }
}

const closeDropdown = () => {
  showDropdown.value = false
  searchQuery.value = ''
  currentPage.value = 1
  tenants.value = []
  total.value = 0
  if (searchTimer.value) {
    clearTimeout(searchTimer.value)
    searchTimer.value = null
  }
}

const close = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (!target.closest('.tenant-dropdown') && !target.closest('.tenant-menu-item')) {
    closeDropdown()
  }
}

const selectTenant = (tenantId: number) => {
  // 如果选择的是默认租户，清除选择
  if (tenantId === defaultTenantId.value) {
    authStore.setSelectedTenant(null)
  } else {
    authStore.setSelectedTenant(tenantId)
  }
  closeDropdown()
  // 触发页面刷新以加载新租户的数据
  MessagePlugin.success(t('tenant.switchSuccess'))
  setTimeout(() => {
    window.location.reload()
  }, 500)
}

const loadTenants = async (append = false) => {
  if (loading.value) return
  
  loading.value = true
  try {
    // 解析搜索关键词，判断是否是租户ID
    let keyword = searchQuery.value.trim()
    let tenantID: number | undefined = undefined
    
    // 如果搜索关键词是纯数字，尝试作为租户ID查询
    if (keyword && /^\d+$/.test(keyword)) {
      tenantID = Number(keyword)
      keyword = '' // 清空关键词，使用租户ID查询
    }
    
    const response = await searchTenants({
      keyword: keyword || undefined,
      tenant_id: tenantID,
      page: currentPage.value,
      page_size: pageSize.value
    })
    
    if (response.success && response.data) {
      if (append) {
        tenants.value = [...tenants.value, ...response.data.items]
      } else {
        tenants.value = response.data.items
      }
      total.value = response.data.total
      authStore.setAllTenants(tenants.value)
    } else {
      MessagePlugin.error(response.message || t('tenant.loadTenantsFailed'))
    }
  } catch (error) {
    console.error('Failed to load tenants:', error)
    MessagePlugin.error(t('tenant.loadTenantsFailed'))
  } finally {
    loading.value = false
  }
}

const handleSearchInput = () => {
  // 防抖处理，延迟500ms后搜索
  if (searchTimer.value) {
    clearTimeout(searchTimer.value)
  }
  
  searchTimer.value = window.setTimeout(() => {
    currentPage.value = 1
    tenants.value = []
    total.value = 0
    loadTenants()
  }, 500)
}

const loadMore = () => {
  if (hasMore.value && !loading.value) {
    currentPage.value++
    loadTenants(true)
  }
}

const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (!target.closest('.tenant-selector-wrapper')) {
    closeDropdown()
  }
}

const handleResize = () => {
  if (showDropdown.value) {
    updateDropdownPosition()
  }
}

watch(showDropdown, (newVal) => {
  if (newVal) {
    document.addEventListener('click', handleClickOutside)
    window.addEventListener('resize', handleResize)
    window.addEventListener('scroll', handleResize, true)
    updateDropdownPosition()
  } else {
    document.removeEventListener('click', handleClickOutside)
    window.removeEventListener('resize', handleResize)
    window.removeEventListener('scroll', handleResize, true)
  }
})

onMounted(() => {
  // 不再自动加载，等用户打开下拉框时再加载
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  window.removeEventListener('resize', handleResize)
  window.removeEventListener('scroll', handleResize, true)
  if (searchTimer.value) {
    clearTimeout(searchTimer.value)
  }
})
</script>

<style scoped lang="less">
.tenant-selector {
  width: 100%;
  margin-bottom: 4px;
}

.tenant-selector-wrapper {
  width: 100%;
  position: relative;
}

.tenant-menu-item {
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 48px;
  padding: 13px 8px 13px 16px;
  box-sizing: border-box;
  transition: all 0.2s;

  &:hover {
    border-radius: 4px;
    background: #30323605;
    color: #00000099;

    .tenant-icon,
    .tenant-title {
      color: #00000099;
    }
  }

  &.readonly {
    cursor: default;
    
    &:hover {
      background: transparent;
    }
  }
}

.tenant-item-box {
  display: flex;
  align-items: center;
  flex: 1;
  min-width: 0;
}

.tenant-icon {
  width: 20px;
  height: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-right: 12px;
  color: #00000099;
  flex-shrink: 0;
}

.tenant-title {
  font-size: 14px;
  font-weight: 400;
  color: #000000e6;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1;
  min-width: 0;
}

.tenant-arrow {
  font-size: 16px;
  color: #00000066;
  flex-shrink: 0;
  margin-left: 8px;
  transition: transform 0.2s;

  &.rotated {
    transform: rotate(180deg);
  }
}

.tenant-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 999;
}

.tenant-dropdown {
  position: fixed;
  background: #fff;
  border: 1px solid #e7e9eb;
  border-radius: 8px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  z-index: 1000;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  transform-origin: bottom left;
  box-sizing: border-box;
}

.tenant-search {
  padding: 8px;
  border-top: 1px solid #f0f0f0;
  background: #fafafa;
  flex-shrink: 0;
  box-sizing: border-box;
}

.tenant-search-input {
  width: 100%;
  padding: 6px 8px;
  border: 1px solid #e7e9eb;
  border-radius: 4px;
  font-size: 14px;
  outline: none;
  background: #fff;
  color: #333;
  transition: border-color 0.2s;
  box-sizing: border-box;

  &:focus {
    border-color: #07c05f;
  }

  &::placeholder {
    color: #999;
  }
}

.tenant-list {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 4px 0;
  min-height: 0;
  max-height: 100%;
  box-sizing: border-box;
}

.tenant-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px;
  cursor: pointer;
  transition: background 0.2s;

  &:hover {
    background: #f5f7fa;
  }

  &.selected {
    background: #07c05f1a;

    .tenant-item-name {
      color: #07c05f;
      font-weight: 500;
    }
  }
}

.tenant-item-info {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.tenant-item-name {
  font-size: 14px;
  color: #333;
  font-weight: 400;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.tenant-item-desc {
  font-size: 12px;
  color: #999;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.tenant-item-id {
  font-size: 11px;
  color: #999;
  margin-top: 2px;
}

.tenant-loading {
  padding: 16px;
  text-align: center;
  color: #999;
  font-size: 14px;
}

.tenant-load-more {
  padding: 12px;
  text-align: center;
  color: #07c05f;
  font-size: 14px;
  cursor: pointer;
  border-top: 1px solid #f0f0f0;
  transition: background 0.2s;

  &:hover {
    background: #f5f7fa;
  }
}

.tenant-search-hint {
  font-size: 11px;
  color: #999;
  margin-top: 4px;
  padding: 0 2px;
}

.tenant-check-icon {
  color: #07c05f;
  flex-shrink: 0;
  margin-left: 8px;
}

.tenant-empty {
  padding: 16px;
  text-align: center;
  color: #999;
  font-size: 14px;
}
</style>

