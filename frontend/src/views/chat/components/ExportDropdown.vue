<template>
  <t-popup
    placement="bottom-left"
    :visible="menuVisible"
    :overlay-style="{ padding: '4px 0', borderRadius: '8px' }"
    :destroy-on-close="true"
  >
    <t-button
      size="small"
      variant="outline"
      shape="round"
      :loading="exporting"
      :disabled="disabled || !content"
      :title="$t('chatExport.title')"
      @click.stop="toggleMenu"
    >
      <t-icon name="download" />
    </t-button>
    <template #content>
      <div class="export-menu" @click.stop>
        <div
          v-for="format in exportFormats"
          :key="format.value"
          class="export-menu-item"
          @click="handleExport(format.value)"
        >
          <t-icon :name="format.icon" size="16px" />
          <span class="export-menu-label">{{ format.label }}</span>
        </div>
      </div>
    </template>
  </t-popup>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue';
import { useI18n } from 'vue-i18n';
import { MessagePlugin } from 'tdesign-vue-next';
import {
  exportAsMarkdown,
  exportAsPDF,
  exportAsWord,
  exportAsXLSX,
  generateFilename,
} from '@/utils/exportUtils';

interface Props {
  content: string;
  filenamePrefix?: string;
  disabled?: boolean;
}

const props = withDefaults(defineProps<Props>(), {
  filenamePrefix: '',
  disabled: false,
});

const { t } = useI18n();
const menuVisible = ref(false);
const exporting = ref(false);

const exportFormats = computed(() => [
  { value: 'pdf',      icon: 'file-pdf',   label: t('chatExport.pdf') },
  { value: 'markdown', icon: 'file',        label: t('chatExport.markdown') },
  { value: 'word',     icon: 'file-word',   label: t('chatExport.word') },
  { value: 'xlsx',     icon: 'file-excel',  label: t('chatExport.xlsx') },
]);

const toggleMenu = () => {
  menuVisible.value = !menuVisible.value;
};

// 点击页面其他区域时关闭菜单
const onDocumentClick = () => {
  if (menuVisible.value) {
    menuVisible.value = false;
  }
};

onMounted(() => {
  document.addEventListener('click', onDocumentClick);
});

onBeforeUnmount(() => {
  document.removeEventListener('click', onDocumentClick);
});

const handleExport = async (format: string) => {
  menuVisible.value = false;
  if (!props.content) {
    MessagePlugin.warning(t('chatExport.emptyContent'));
    return;
  }

  exporting.value = true;
  try {
    const filename = generateFilename(props.filenamePrefix || undefined);
    switch (format) {
      case 'pdf':
        await exportAsPDF(props.content, filename);
        break;
      case 'markdown':
        exportAsMarkdown(props.content, filename);
        break;
      case 'word':
        await exportAsWord(props.content, filename);
        break;
      case 'xlsx':
        exportAsXLSX(props.content, filename);
        break;
    }
    MessagePlugin.success(t('chatExport.success'));
  } catch (err) {
    console.error('Export failed:', err);
    MessagePlugin.error(t('chatExport.failed'));
  } finally {
    exporting.value = false;
  }
};
</script>

<style lang="less" scoped>
.export-menu {
  padding: 4px 0;
  min-width: 160px;

  .export-menu-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
    color: #333;
    transition: background 0.2s;

    &:hover {
      background: #f2f3f5;
    }

    .export-menu-label {
      white-space: nowrap;
    }
  }
}
</style>
