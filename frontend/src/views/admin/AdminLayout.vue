<!--
  组织树管理 / 组织人员管理 合并为单一组织管理页后，
  admin 区不再有二级导航，侧栏整体移除，仅保留内容容器。
-->
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
