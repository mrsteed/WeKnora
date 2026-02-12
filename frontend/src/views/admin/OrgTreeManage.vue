<template>
  <div class="org-tree-manage">
    <div class="page-header">
      <h2 class="page-title">{{ $t('admin.orgTreeManage') }}</h2>
      <t-button v-if="authStore.isSuperAdmin" theme="primary" @click="handleCreate(null)">
        <template #icon><t-icon name="add" /></template>
        {{ $t('admin.org.createRoot') }}
      </t-button>
    </div>

    <div class="tree-container">
      <div v-if="orgTreeStore.loading" class="tree-loading">
        <t-loading />
      </div>
      <div v-else-if="orgTreeStore.tree.length === 0" class="tree-empty">
        <t-icon name="folder-open" class="empty-icon" />
        <p>{{ $t('admin.org.emptyTree') }}</p>
        <t-button v-if="authStore.isSuperAdmin" theme="primary" variant="outline" @click="handleCreate(null)">
          {{ $t('admin.org.createFirst') }}
        </t-button>
      </div>
      <div v-else class="tree-content">
        <div v-for="node in orgTreeStore.tree" :key="node.id">
          <OrgTreeNodeItem
            :node="node"
            :level="0"
            @create="handleCreate"
            @edit="handleEdit"
            @delete="handleDelete"
            @move="handleMove"
          />
        </div>
      </div>
    </div>

    <OrgTreeEditor
      v-model:visible="editorVisible"
      :mode="editorMode"
      :node="editingNode"
      :parent-id="editorParentId"
      @success="handleEditorSuccess"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useOrgTreeStore } from '@/stores/orgTree'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from 'vue-i18n'
import type { OrgTreeNode } from '@/api/org-tree'
import { moveOrgTreeNode } from '@/api/org-tree'
import OrgTreeEditor from './components/OrgTreeEditor.vue'
import OrgTreeNodeItem from './components/OrgTreeNodeItem.vue'

const orgTreeStore = useOrgTreeStore()
const authStore = useAuthStore()
const { t } = useI18n()

const editorVisible = ref(false)
const editorMode = ref<'create' | 'edit'>('create')
const editingNode = ref<OrgTreeNode | null>(null)
const editorParentId = ref<string | null>(null)

onMounted(() => {
  orgTreeStore.fetchTree()
})

const handleCreate = (parentId: string | null) => {
  editorMode.value = 'create'
  editingNode.value = null
  editorParentId.value = parentId
  editorVisible.value = true
}

const handleEdit = (node: OrgTreeNode) => {
  editorMode.value = 'edit'
  editingNode.value = node
  editorParentId.value = null
  editorVisible.value = true
}

const handleDelete = async (node: OrgTreeNode) => {
  try {
    await orgTreeStore.deleteNode(node.id)
    MessagePlugin.success(t('admin.org.deleteSuccess'))
  } catch {
    MessagePlugin.error(t('admin.org.deleteFailed'))
  }
}

const handleEditorSuccess = () => {
  editorVisible.value = false
}

const handleMove = async (payload: { nodeId: string; newParentId: string | null }) => {
  try {
    const res = await moveOrgTreeNode(payload.nodeId, { new_parent_id: payload.newParentId })
    if (res.success) {
      MessagePlugin.success(t('admin.org.moveSuccess'))
      orgTreeStore.fetchTree()
    } else {
      MessagePlugin.error(res.message || t('admin.org.moveFailed'))
    }
  } catch {
    MessagePlugin.error(t('admin.org.moveFailed'))
  }
}
</script>

<style lang="less" scoped>
.org-tree-manage {
  max-width: 900px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;

  .page-title {
    font-size: 20px;
    font-weight: 600;
    color: #1a1a1a;
    margin: 0;
  }
}

.tree-container {
  background: #fff;
  border-radius: 12px;
  border: 1px solid #e7e7e7;
  min-height: 400px;
}

.tree-loading {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 300px;
}

.tree-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 300px;
  color: #999;

  .empty-icon {
    font-size: 48px;
    margin-bottom: 16px;
    color: #ddd;
  }

  p {
    margin-bottom: 16px;
    font-size: 14px;
  }
}

.tree-content {
  padding: 16px;
}
</style>
