<template>
  <div class="admin-layout">
    <div class="admin-content">
      <RouterView />
    </div>
  </div>
</template>

<script setup lang="ts">
</script>

<style lang="less" scoped>
.admin-layout {
  display: flex;
  width: 100%;
  height: 100%;
  background: #fafbfc;
}

.admin-content {
  flex: 1;
  overflow: auto;
  padding: 24px;
}
</style>