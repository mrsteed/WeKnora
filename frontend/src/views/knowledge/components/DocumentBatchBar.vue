<script setup lang="ts">
import { ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import FolderPickerMenu, { type FolderOption } from './FolderPickerMenu.vue';

const props = defineProps<{
  count: number;
  deleteLoading?: boolean;
  reparseLoading?: boolean;
  tagLoading?: boolean;
  // 批量下载进行中：显示 loading 并禁用按钮。
  downloadLoading?: boolean;
  // 是否渲染“批量下载”按钮；受 KB 级下载权控制（与单文件下载入口一致）。
  canDownload?: boolean;
  // When true the bar stays visible even with 0 selections, so users can exit
  // batch mode from here without selecting anything first.
  visible?: boolean;
  /** Hidden when the knowledge base has no folder structure to file into. */
  showMoveToFolder?: boolean;
  folderOptions?: FolderOption[];
}>();

const emit = defineEmits<{
  (e: 'cancel'): void;
  (e: 'delete'): void;
  (e: 'reparse'): void;
  (e: 'batchTag'): void;
  (e: 'moveToFolder', folderPath: string): void;
  (e: 'batchDownload'): void;
}>();

const { t } = useI18n();

const folderPickerVisible = ref(false);

// 下载是不可逆的长任务（打包 + 传输），期间禁用自身避免重复触发；
// 其他批量按钮不受影响（下载不修改文件集合）。
const anyBatchLoading = computed(() =>
  props.deleteLoading || props.reparseLoading || props.tagLoading || props.downloadLoading
);
</script>

<template>
  <transition name="batch-bar-fade">
    <div v-if="visible || count > 0" class="doc-batch-bar" role="region"
      :aria-label="t('knowledgeBase.selectedCount', { count })">
      <div class="batch-bar-inner">
        <div class="batch-bar-left">
          <span class="batch-bar-count">{{ t('knowledgeBase.selectedCount', { count }) }}</span>
          <t-button variant="text" theme="default" size="small" class="batch-bar-clear" @click="emit('cancel')">
            {{ t('knowledgeBase.clearSelection') }}
          </t-button>
        </div>
        <div class="batch-bar-actions">
          <t-button v-if="canDownload" theme="default" variant="outline" size="small"
            :disabled="count === 0 || anyBatchLoading" :loading="downloadLoading"
            @click="emit('batchDownload')">
            <template #icon><t-icon name="download" size="14px" /></template>
            {{ t('knowledgeBase.batchDownload') }}
          </t-button>

          <t-popconfirm theme="warning" :content="t('knowledgeBase.confirmBatchReparseDocument', { count })"
            :confirm-btn="{ content: t('knowledgeBase.confirmBatchReparse'), theme: 'warning' }"
            :cancel-btn="{ content: t('common.cancel') }" placement="top" @confirm="emit('reparse')">
            <t-button theme="default" variant="outline" size="small"
              :disabled="count === 0 || deleteLoading || reparseLoading || tagLoading" :loading="reparseLoading" @click.stop>
              <template #icon><t-icon name="refresh" size="14px" /></template>
              {{ t('knowledgeBase.rebuildDocument') }}
            </t-button>
          </t-popconfirm>

          <t-button theme="default" variant="outline" size="small"
            :disabled="count === 0 || deleteLoading || reparseLoading || tagLoading" :loading="tagLoading"
            @click="emit('batchTag')">
            <template #icon><t-icon name="discount" size="14px" /></template>
            {{ t('knowledgeBase.batchTag') }}
          </t-button>

          <t-popup v-if="showMoveToFolder" v-model:visible="folderPickerVisible" trigger="click"
            placement="top" overlay-class-name="card-more" destroy-on-close>
            <t-button theme="default" variant="outline" size="small"
              :disabled="count === 0 || deleteLoading || reparseLoading || tagLoading">
              <template #icon><t-icon name="folder" size="14px" /></template>
              {{ t('knowledgeBase.moveToFolder.action') }}
            </t-button>
            <template #content>
              <div class="card-menu">
                <FolderPickerMenu :options="folderOptions || []"
                  @confirm="(path: string) => { folderPickerVisible = false; emit('moveToFolder', path) }" />
              </div>
            </template>
          </t-popup>

          <t-popconfirm theme="warning" :content="t('knowledgeBase.confirmBatchDeleteDocument', { count })"
            :confirm-btn="{ content: t('knowledgeBase.confirmDelete'), theme: 'danger' }"
            :cancel-btn="{ content: t('common.cancel') }" placement="top" @confirm="emit('delete')">
            <t-button theme="danger" variant="outline" size="small"
              :disabled="count === 0 || deleteLoading || reparseLoading || tagLoading" :loading="deleteLoading" @click.stop>
              <template #icon><t-icon name="delete" size="14px" /></template>
              {{ t('knowledgeBase.batchDelete') }}
            </t-button>
          </t-popconfirm>
        </div>
      </div>
    </div>
  </transition>
</template>

<style scoped lang="less">
.doc-batch-bar {
  position: relative;
  z-index: 5;
  /* 宽度随按钮数量自适应（按钮增减时不再留大片空白），窄屏时收缩到父容器 */
  width: max-content;
  max-width: 100%;
  margin: 0 auto;
  padding: 0 4px;
  box-sizing: border-box;
}

.batch-bar-inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 8px 12px;
  padding: 8px 12px;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  box-shadow: 0 6px 16px rgba(0, 0, 0, 0.08);
}

/* 左侧（已选 N + 取消选择）不参与压缩，否则新增按钮后会被
   flex-shrink:0 的右侧操作挤成 0 宽，导致"取消选择"被遮挡。 */
.batch-bar-left {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: 0 0 auto;
  min-width: 0;
}

.batch-bar-count {
  font-size: 13px;
  font-weight: 500;
  color: var(--td-text-color-secondary);
  white-space: nowrap;
}

.batch-bar-clear {
  flex-shrink: 0;
  padding: 0 6px !important;
  height: 28px !important;
  font-size: 12px;
  color: var(--td-text-color-secondary) !important;

  &:hover {
    color: var(--td-brand-color) !important;
  }
}

.batch-bar-actions {
  flex-shrink: 0;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

.batch-bar-fade-enter-active,
.batch-bar-fade-leave-active {
  transition: transform 0.2s ease, opacity 0.2s ease;
}

.batch-bar-fade-enter-from,
.batch-bar-fade-leave-to {
  opacity: 0;
  transform: translateY(6px);
}
</style>
