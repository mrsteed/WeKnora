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
      :disabled="disabled || !canExport"
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
          :class="{ 'is-disabled': format.disabled }"
          :title="format.title"
          @click="!format.disabled && handleExport(format.value)"
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
  type ExportCapability,
  getExportCapabilities,
  type ExportCapabilities,
  type ExportFormat,
} from '@/utils/exportUtils';

interface Props {
  content?: string;
  contentResolver?: () => Promise<string> | string;
  filenamePrefix?: string;
  disabled?: boolean;
}

const props = withDefaults(defineProps<Props>(), {
  content: '',
  filenamePrefix: '',
  disabled: false,
});

const { t } = useI18n();
const menuVisible = ref(false);
const exporting = ref(false);
const exportCapabilities = ref<ExportCapabilities>({
  markdown: { available: true, engine: 'builtin', maxContentBytes: 2 * 1024 * 1024, timeoutSeconds: 5 },
  pdf: { available: true, engine: 'chromium', maxContentBytes: 1024 * 1024, timeoutSeconds: 45 },
  docx: { available: true, engine: 'pandoc', maxContentBytes: 1024 * 1024, timeoutSeconds: 30 },
  xlsx: { available: true, engine: 'excelize', maxContentBytes: 1024 * 1024, timeoutSeconds: 10 },
});
const canExport = computed(() => Boolean(props.content || props.contentResolver));

const exportFormats = computed(() => [
  buildFormatItem('pdf', 'file-pdf', t('chatExport.pdf'), exportCapabilities.value.pdf),
  buildFormatItem('markdown', 'file', t('chatExport.markdown'), exportCapabilities.value.markdown),
  buildFormatItem('docx', 'file-word', t('chatExport.word'), exportCapabilities.value.docx),
  buildFormatItem('xlsx', 'file-excel', t('chatExport.xlsx'), exportCapabilities.value.xlsx),
]);

const loadCapabilities = async () => {
  exportCapabilities.value = await getExportCapabilities();
};

const toggleMenu = () => {
  menuVisible.value = !menuVisible.value;
};

const resolveExportContent = async (): Promise<string> => {
  if (props.content) {
    return props.content;
  }
  if (props.contentResolver) {
    return await props.contentResolver();
  }
  return '';
};

// 点击页面其他区域时关闭菜单
const onDocumentClick = () => {
  if (menuVisible.value) {
    menuVisible.value = false;
  }
};

onMounted(() => {
  document.addEventListener('click', onDocumentClick);
  void loadCapabilities();
});

onBeforeUnmount(() => {
  document.removeEventListener('click', onDocumentClick);
});

const handleExport = async (format: ExportFormat) => {
  menuVisible.value = false;
  if (!exportCapabilities.value[format].available) {
    MessagePlugin.warning(exportCapabilities.value[format].reason || t('chatExport.failed'));
    return;
  }

  exporting.value = true;
  try {
    const content = await resolveExportContent();
    if (!content || !content.trim()) {
      MessagePlugin.warning(t('chatExport.emptyContent'));
      return;
    }

    const filename = generateFilename(props.filenamePrefix || undefined);
    switch (format) {
      case 'pdf':
        await exportAsPDF(content, filename);
        break;
      case 'markdown':
        await exportAsMarkdown(content, filename);
        break;
      case 'docx':
        await exportAsWord(content, filename);
        break;
      case 'xlsx':
        await exportAsXLSX(content, filename);
        break;
    }
    MessagePlugin.success(t('chatExport.success'));
  } catch (err) {
    console.error('Export failed:', err);
    MessagePlugin.error((err as any)?.message || t('chatExport.failed'));
  } finally {
    exporting.value = false;
  }
};

const buildFormatItem = (
  value: ExportFormat,
  icon: string,
  label: string,
  capability: ExportCapability,
) => ({
  value,
  icon,
  label,
  disabled: !capability.available,
  title: capability.available ? label : `${label}\n${capability.reason || ''}`.trim(),
});
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

    &.is-disabled {
      cursor: not-allowed;
      color: #bbb;

      &:hover {
        background: transparent;
      }
    }

    .export-menu-label {
      white-space: nowrap;
    }
  }
}
</style>
