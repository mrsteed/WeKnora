<template>
  <div class="org-tree-select-item" :style="{ paddingLeft: level * 20 + 'px' }">
    <div
      :class="['select-row', { active: selectedId === node.id }]"
      @click="$emit('select', node.id)"
    >
      <t-icon
        v-if="node.children && node.children.length > 0"
        :name="expanded ? 'chevron-down' : 'chevron-right'"
        class="expand-icon"
        @click.stop="expanded = !expanded"
      />
      <span v-else class="expand-placeholder"></span>
      <t-icon name="folder" class="folder-icon" />
      <span class="node-name" :title="node.name">{{ node.name }}</span>
      <span class="member-count">{{ node.member_count || 0 }}</span>
    </div>
    <div v-if="expanded && node.children" class="children">
      <OrgTreeSelectItem
        v-for="child in node.children"
        :key="child.id"
        :node="child"
        :level="level + 1"
        :selected-id="selectedId"
        @select="$emit('select', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import type { OrgTreeNode } from '@/api/org-tree'

defineProps<{
  node: OrgTreeNode
  level: number
  selectedId: string | null
}>()

defineEmits<{
  (e: 'select', orgId: string): void
}>()

const expanded = ref(true)
</script>

<style lang="less" scoped>
.org-tree-select-item {
  .select-row {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 6px 8px;
    border-radius: 6px;
    cursor: pointer;
    font-size: 13px;
    color: #333;
    transition: all 0.15s;

    &:hover {
      background: #f2f3f5;
    }

    &.active {
      background: #e8f3ff;
      color: #0052d9;
    }

    .expand-icon {
      font-size: 12px;
      color: #999;
      flex-shrink: 0;
      cursor: pointer;
    }

    .expand-placeholder {
      width: 12px;
      flex-shrink: 0;
    }

    .folder-icon {
      font-size: 14px;
      flex-shrink: 0;
    }

    .node-name {
      flex: 1;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .member-count {
      font-size: 11px;
      color: #999;
      flex-shrink: 0;
    }
  }
}
</style>
