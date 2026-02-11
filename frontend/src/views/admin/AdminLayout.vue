<template>
  <div class="admin-layout">
    <div class="admin-sidebar">
      <div class="admin-sidebar-header">
        <h3 class="admin-title">{{ $t('admin.title') }}</h3>
      </div>
      <nav class="admin-nav" role="navigation" aria-label="Admin navigation">
        <div
          v-for="item in navItems"
          :key="item.key"
          role="menuitem"
          :tabindex="0"
          :aria-current="currentRoute === item.key ? 'page' : undefined"
          :class="['admin-nav-item', { active: currentRoute === item.key }]"
          @click="handleNav(item.key)"
          @keydown.enter="handleNav(item.key)"
          @keydown.space.prevent="handleNav(item.key)"
        >
          <t-icon :name="item.icon" class="admin-nav-icon" />
          <span>{{ item.label }}</span>
        </div>
      </nav>
    </div>
    <div class="admin-content">
      <RouterView />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const navItems = computed(() => [
  { key: 'org-tree', icon: 'tree-round-dot', label: t('admin.orgTreeManage') },
  { key: 'members', icon: 'usergroup', label: t('admin.memberManage') },
])

const currentRoute = computed(() => {
  const name = route.name as string
  if (name === 'orgTreeManage') return 'org-tree'
  if (name === 'memberManage') return 'members'
  return ''
})

const handleNav = (key: string) => {
  router.push(`/platform/admin/${key}`)
}
</script>

<style lang="less" scoped>
.admin-layout {
  display: flex;
  width: 100%;
  height: 100%;
  background: #fafbfc;
}

.admin-sidebar {
  width: 220px;
  min-width: 220px;
  background: #fff;
  border-right: 1px solid #e7e7e7;
  display: flex;
  flex-direction: column;
  padding: 16px 0;
}

.admin-sidebar-header {
  padding: 0 20px 16px;
  border-bottom: 1px solid #e7e7e7;

  .admin-title {
    font-size: 16px;
    font-weight: 600;
    color: #1a1a1a;
    margin: 0;
  }
}

.admin-nav {
  padding: 8px;
  flex: 1;
}

.admin-nav-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 8px;
  cursor: pointer;
  color: #666;
  font-size: 14px;
  transition: all 0.2s;
  margin-bottom: 2px;

  &:hover {
    background: #f2f3f5;
    color: #333;
  }

  &.active {
    background: #e8f3ff;
    color: #0052d9;
    font-weight: 500;
  }

  .admin-nav-icon {
    font-size: 18px;
  }
}

.admin-content {
  flex: 1;
  overflow: auto;
  padding: 24px;
}
</style>
