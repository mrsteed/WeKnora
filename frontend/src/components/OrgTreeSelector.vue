<!-- 组织选择器：创建知识库时选择归属组织（使用用户自己的组织列表，非超管也可使用） -->
<template>
  <t-select
    v-model="modelValue"
    :options="orgOptions"
    :placeholder="$t('org.selectTargetOrg')"
    clearable
    filterable
  />
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useOrganizationStore } from '@/stores/organization'
import { useAuthStore } from '@/stores/auth'
import type { OrgTreeNode } from '@/api/org-tree'

const props = defineProps<{
  showAll?: boolean  // For super admin: show all org-tree organizations instead of just user's orgs
}>()

const modelValue = defineModel<string>()
const orgStore = useOrganizationStore()
const authStore = useAuthStore()

// Flatten tree structure to list with indentation for display
const flattenOrgTree = (nodes: OrgTreeNode[], level = 0): Array<{ label: string; value: string; level: number }> => {
  const result: Array<{ label: string; value: string; level: number }> = []
  
  for (const node of nodes) {
    // Add indentation based on level
    const indent = '　'.repeat(level) // Use full-width space for better alignment
    result.push({
      label: indent + node.name,
      value: node.id,
      level
    })
    
    // Recursively add children
    if (node.children && node.children.length > 0) {
      result.push(...flattenOrgTree(node.children, level + 1))
    }
  }
  
  return result
}

const orgOptions = computed(() => {
  const orgs = props.showAll && authStore.isSuperAdmin 
    ? orgStore.allOrgTreeOrgs 
    : orgStore.myOrgTreeOrgs
  
  // Flatten the tree structure if it has children
  return flattenOrgTree(orgs)
})

onMounted(() => {
  if (props.showAll && authStore.isSuperAdmin) {
    if (orgStore.allOrgTreeOrgs.length === 0) {
      orgStore.fetchAllOrgTreeOrgs()
    }
  } else {
    if (orgStore.myOrgTreeOrgs.length === 0) {
      orgStore.fetchMyOrgTreeOrganizations()
    }
  }
})
</script>
