<template>
  <div v-if="orgStore.myOrgTreeOrgs.length > 0" class="org-switcher">
    <t-select
      v-model="currentOrgId"
      :placeholder="$t('org.switchPlaceholder')"
      size="small"
      :clearable="true"
      :popup-props="{ overlayClassName: 'org-switcher-popup' }"
      @change="handleSwitch"
    >
      <t-option
        v-for="org in orgStore.myOrgTreeOrgs"
        :key="org.id"
        :value="org.id"
        :label="org.name"
      />
    </t-select>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { useOrganizationStore } from '@/stores/organization'

const orgStore = useOrganizationStore()

const currentOrgId = ref<string | undefined>(orgStore.currentOrganizationId || undefined)

onMounted(() => {
  orgStore.fetchMyOrgTreeOrganizations()
})

watch(() => orgStore.currentOrganizationId, (val) => {
  currentOrgId.value = val || undefined
})

const handleSwitch = (val: string | undefined) => {
  orgStore.switchOrganization(val || null)
}
</script>

<style lang="less" scoped>
.org-switcher {
  padding: 4px 8px;
  margin-bottom: 4px;
}
</style>
