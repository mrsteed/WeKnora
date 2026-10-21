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
  showAll?: boolean
}>()

const modelValue = defineModel<string>()
const orgStore = useOrganizationStore()
const authStore = useAuthStore()

const flattenOrgTree = (nodes: OrgTreeNode[], level = 0): Array<{ label: string; value: string; level: number }> => {
  const result: Array<{ label: string; value: string; level: number }> = []

  for (const node of nodes) {
    const indent = '　'.repeat(level)
    result.push({
      label: indent + node.name,
      value: node.id,
      level
    })

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