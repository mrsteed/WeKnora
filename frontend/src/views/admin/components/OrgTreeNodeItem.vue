<template>
  <div class="org-tree-node-item" :style="{ paddingLeft: level * 24 + 'px' }">
    <div
      class="node-row"
      :class="{ expanded: isExpanded, 'drag-over': isDragOver }"
      draggable="true"
      @dragstart.stop="handleDragStart"
      @dragover.prevent="handleDragOver"
      @dragleave="handleDragLeave"
      @drop.prevent="handleDrop"
      @dragend="handleDragEnd"
    >
      <div class="node-expand" @click="toggleExpand">
        <t-icon
          v-if="node.children && node.children.length > 0"
          :name="isExpanded ? 'chevron-down' : 'chevron-right'"
          class="expand-icon"
        />
        <span v-else class="expand-placeholder"></span>
      </div>
      <div class="node-info">
        <t-icon name="folder" class="node-icon" />
        <span class="node-name">{{ node.name }}</span>
        <span v-if="node.description" class="node-desc">{{ node.description }}</span>
        <span class="node-member-count">
          <t-icon name="user" size="14px" />
          {{ node.member_count || 0 }}
        </span>
      </div>
      <div class="node-actions">
        <t-button size="small" variant="text" theme="primary" @click.stop="$emit('create', node.id)">
          <t-icon name="add" />
        </t-button>
        <t-button size="small" variant="text" theme="default" @click.stop="$emit('edit', node)">
          <t-icon name="edit" />
        </t-button>
        <t-popconfirm
          :content="$t('admin.org.deleteConfirm')"
          @confirm="$emit('delete', node)"
        >
          <t-button size="small" variant="text" theme="danger" @click.stop>
            <t-icon name="delete" />
          </t-button>
        </t-popconfirm>
      </div>
    </div>
    <div v-if="isExpanded && node.children && node.children.length > 0" class="node-children">
      <OrgTreeNodeItem
        v-for="child in node.children"
        :key="child.id"
        :node="child"
        :level="level + 1"
        @create="$emit('create', $event)"
        @edit="$emit('edit', $event)"
        @delete="$emit('delete', $event)"
        @move="$emit('move', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import type { OrgTreeNode } from '@/api/org-tree'

const props = defineProps<{
  node: OrgTreeNode
  level: number
}>()

const emit = defineEmits<{
  (e: 'create', parentId: string): void
  (e: 'edit', node: OrgTreeNode): void
  (e: 'delete', node: OrgTreeNode): void
  (e: 'move', payload: { nodeId: string; newParentId: string | null }): void
}>()

const isExpanded = ref(true)
const isDragOver = ref(false)

const toggleExpand = () => {
  isExpanded.value = !isExpanded.value
}

const handleDragStart = (e: DragEvent) => {
  // Store both id and path for circular reference detection
  e.dataTransfer?.setData('text/plain', props.node.id)
  e.dataTransfer?.setData('application/x-org-path', props.node.path)
  e.dataTransfer!.effectAllowed = 'move'
}

const handleDragOver = (e: DragEvent) => {
  e.dataTransfer!.dropEffect = 'move'
  isDragOver.value = true
}

const handleDragLeave = () => {
  isDragOver.value = false
}

const handleDrop = (e: DragEvent) => {
  isDragOver.value = false
  const draggedId = e.dataTransfer?.getData('text/plain')
  const draggedPath = e.dataTransfer?.getData('application/x-org-path')
  if (!draggedId || draggedId === props.node.id) return
  // Prevent moving a parent node into its own descendant (circular reference)
  if (draggedPath && props.node.path.startsWith(draggedPath + '/')) return
  emit('move', { nodeId: draggedId, newParentId: props.node.id })
}

const handleDragEnd = () => {
  isDragOver.value = false
}
</script>

<style lang="less" scoped>
.org-tree-node-item {
  .node-row {
    display: flex;
    align-items: center;
    padding: 8px 12px;
    border-radius: 6px;
    gap: 4px;
    transition: background 0.2s;

    &:hover {
      background: #f5f7fa;

      .node-actions {
        opacity: 1;
      }
    }

    &.drag-over {
      background: #e6f7ff;
      outline: 2px dashed #0052d9;
      outline-offset: -2px;
    }
  }

  .node-expand {
    width: 20px;
    height: 20px;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    flex-shrink: 0;

    .expand-icon {
      font-size: 14px;
      color: #999;
      transition: transform 0.2s;
    }

    .expand-placeholder {
      width: 14px;
    }
  }

  .node-info {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: 1;
    min-width: 0;

    .node-icon {
      font-size: 16px;
      color: #0052d9;
      flex-shrink: 0;
    }

    .node-name {
      font-size: 14px;
      color: #333;
      font-weight: 500;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }

    .node-desc {
      font-size: 12px;
      color: #999;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 200px;
    }

    .node-member-count {
      display: flex;
      align-items: center;
      gap: 2px;
      font-size: 12px;
      color: #999;
      flex-shrink: 0;
    }
  }

  .node-actions {
    display: flex;
    align-items: center;
    gap: 2px;
    opacity: 0;
    transition: opacity 0.2s;
    flex-shrink: 0;
  }
}
</style>
