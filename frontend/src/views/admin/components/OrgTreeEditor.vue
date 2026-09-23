<template>
  <Teleport to="body">
    <Transition name="fade">
      <!-- 点击遮罩空白处不关闭，避免编辑时被误触（与其他管理弹窗一致，需通过 ✕/取消/确认 关闭） -->
      <div v-if="visible" class="editor-overlay">
        <div class="editor-dialog">
          <div class="editor-header">
            <h3>{{ mode === 'create' ? $t('admin.org.createNode') : $t('admin.org.editNode') }}</h3>
            <t-button variant="text" shape="square" @click="handleClose">
              <t-icon name="close" />
            </t-button>
          </div>
          <div class="editor-body">
            <div class="form-item">
              <label class="form-label required">{{ $t('admin.org.nameLabel') }}</label>
              <t-input
                v-model="form.name"
                :placeholder="$t('admin.org.namePlaceholder')"
                :maxlength="50"
              />
            </div>
            <div class="form-item">
              <label class="form-label">{{ $t('admin.org.descriptionLabel') }}</label>
              <t-textarea
                v-model="form.description"
                :placeholder="$t('admin.org.descriptionPlaceholder')"
                :maxlength="200"
                :autosize="{ minRows: 3, maxRows: 6 }"
              />
            </div>
            <div class="form-item" v-if="mode === 'edit'">
              <label class="form-label">{{ $t('admin.org.parentOrgLabel') }}</label>
              <t-select
                v-model="form.parent_id"
                :options="parentOptions"
                :placeholder="$t('admin.org.parentOrgPlaceholder')"
                filterable
                clearable
              />
            </div>
            <div class="form-item">
              <label class="form-label">{{ $t('admin.org.sortOrder') }}</label>
              <t-input-number
                v-model="form.sort_order"
                :min="0"
                :max="9999"
                theme="normal"
              />
            </div>
          </div>
          <div class="editor-footer">
            <t-button variant="outline" @click="handleClose">{{ $t('common.cancel') }}</t-button>
            <t-button theme="primary" :loading="saving" @click="handleSubmit">{{ $t('common.confirm') }}</t-button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useOrgTreeStore } from '@/stores/orgTree'
import { useI18n } from 'vue-i18n'
import { moveOrgTreeNode } from '@/api/org-tree'
import type { OrgTreeNode } from '@/api/org-tree'

const props = defineProps<{
  visible: boolean
  mode: 'create' | 'edit'
  node: OrgTreeNode | null
  parentId: string | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', val: boolean): void
  (e: 'success'): void
}>()

const orgTreeStore = useOrgTreeStore()
const { t } = useI18n()
const saving = ref(false)

const form = ref({
  name: '',
  description: '',
  parent_id: '' as string,
  sort_order: 0,
})

// 编辑模式下可选的上级组织：排除自身及其子孙，防止循环引用
const parentOptions = computed(() => {
  if (props.mode !== 'edit' || !props.node) return []
  const excludedId = props.node.id
  const descendants = new Set<string>()
  const collectDescendants = (nodes: OrgTreeNode[]) => {
    for (const n of nodes) {
      if (n.id === excludedId) {
        descendants.add(n.id)
        if (n.children) collectDescendants(n.children)
        continue
      }
      if (n.children) collectDescendants(n.children)
    }
  }
  collectDescendants(orgTreeStore.tree)
  const buildIndented = (nodes: OrgTreeNode[], depth: number): { label: string; value: string }[] => {
    const result: { label: string; value: string }[] = []
    for (const n of nodes) {
      if (n.id === excludedId || descendants.has(n.id)) continue
      result.push({ label: '　'.repeat(depth) + n.name, value: n.id })
      if (n.children) result.push(...buildIndented(n.children, depth + 1))
    }
    return result
  }
  return buildIndented(orgTreeStore.tree, 0)
})

// 新建时的默认排序号：取"上级组织的现有子节点"中的最大排序号 +1；
// 新建根组织时即为全体根节点的最大号 +1。无同级时从 1 开始。
const defaultSortOrder = (): number => {
  let siblings: OrgTreeNode[] = orgTreeStore.tree
  if (props.parentId) {
    const find = (nodes: OrgTreeNode[]): OrgTreeNode | null => {
      for (const n of nodes) {
        if (n.id === props.parentId) return n
        if (n.children) {
          const f = find(n.children)
          if (f) return f
        }
      }
      return null
    }
    const parent = find(orgTreeStore.tree)
    siblings = parent?.children || []
  }
  const max = siblings.reduce((m, n) => Math.max(m, n.sort_order || 0), 0)
  return max + 1
}

watch(() => props.visible, (val) => {
  if (val) {
    if (props.mode === 'edit' && props.node) {
      form.value = {
        name: props.node.name,
        description: props.node.description || '',
        // 空串 = 成为根组织，与 t-select 的 clearable 语义一致
        parent_id: props.node.parent_id || '',
        sort_order: props.node.sort_order || 0,
      }
    } else {
      form.value = { name: '', description: '', parent_id: '', sort_order: defaultSortOrder() }
    }
  }
})

const handleClose = () => {
  emit('update:visible', false)
}

const handleSubmit = async () => {
  if (!form.value.name.trim()) {
    MessagePlugin.warning(t('admin.org.nameRequired'))
    return
  }
  saving.value = true
  try {
    if (props.mode === 'create') {
      await orgTreeStore.createNode({
        name: form.value.name.trim(),
        description: form.value.description.trim() || undefined,
        parent_id: props.parentId,
        sort_order: form.value.sort_order,
      })
      MessagePlugin.success(t('admin.org.createSuccess'))
    } else if (props.node) {
      // 循环引用双保险（选项理论上已排除自身子孙）：新上级路径若在本节点子树下，拒绝
      const findNodePath = (nodes: OrgTreeNode[]): string | null => {
        for (const n of nodes) {
          if (n.id === form.value.parent_id) return n.path
          if (n.children) {
            const found = findNodePath(n.children)
            if (found) return found
          }
        }
        return null
      }
      const newParentId = form.value.parent_id || null
      const newParentPath = newParentId ? findNodePath(orgTreeStore.tree) : null
      if (newParentId && newParentPath && (newParentPath === props.node.path || newParentPath.startsWith(props.node.path + '/'))) {
        MessagePlugin.error(t('admin.org.parentInvalid'))
        saving.value = false
        return
      }
      await orgTreeStore.updateNode(props.node.id, {
        name: form.value.name.trim(),
        description: form.value.description.trim() || undefined,
        sort_order: form.value.sort_order,
      })
      // 上级组织变化时调用 move 接口调整从属关系（与拖拽移动同一后端能力）
      if (newParentId !== props.node.parent_id) {
        const res = await moveOrgTreeNode(props.node.id, { new_parent_id: newParentId })
        if (!res.success) {
          MessagePlugin.error(res.message || t('admin.org.moveFailed'))
          throw new Error(res.message || 'move failed')
        }
        MessagePlugin.success(t('admin.org.moveSuccess'))
      }
      MessagePlugin.success(t('admin.org.updateSuccess'))
    }
    emit('success')
  } catch {
    MessagePlugin.error(t('common.operationFailed'))
  } finally {
    saving.value = false
  }
}
</script>

<style lang="less" scoped>
.editor-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.4);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 2000;
}

.editor-dialog {
  background: #fff;
  border-radius: 12px;
  width: 480px;
  max-width: 90vw;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.12);
}

.editor-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px 16px;
  border-bottom: 1px solid #e7e7e7;

  h3 {
    font-size: 16px;
    font-weight: 600;
    margin: 0;
    color: #1a1a1a;
  }
}

.editor-body {
  padding: 20px 24px;

  .form-item {
    margin-bottom: 16px;

    &:last-child {
      margin-bottom: 0;
    }

    .form-label {
      display: block;
      font-size: 14px;
      font-weight: 500;
      color: #333;
      margin-bottom: 8px;

      &.required::before {
        content: '*';
        color: #e34d59;
        margin-right: 4px;
      }
    }
  }
}

.editor-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 16px 24px 20px;
  border-top: 1px solid #e7e7e7;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
