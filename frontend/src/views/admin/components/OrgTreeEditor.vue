<template>
  <Teleport to="body">
    <Transition name="fade">
      <div v-if="visible" class="editor-overlay" @click.self="handleClose">
        <div class="editor-dialog">
          <div class="editor-header">
            <h3>{{ mode === 'create' ? $t('admin.org.createNode') : $t('admin.org.editNode') }}</h3>
            <t-button variant="text" shape="square" @click="handleClose">
              <t-icon name="close" />
            </t-button>
          </div>
          <div class="editor-body">
            <div class="form-item">
              <label class="form-label required">{{ $t('admin.org.nameLabel') }}</label>
              <t-input
                v-model="form.name"
                :placeholder="$t('admin.org.namePlaceholder')"
                :maxlength="50"
              />
            </div>
            <div class="form-item">
              <label class="form-label">{{ $t('admin.org.descriptionLabel') }}</label>
              <t-textarea
                v-model="form.description"
                :placeholder="$t('admin.org.descriptionPlaceholder')"
                :maxlength="200"
                :autosize="{ minRows: 3, maxRows: 6 }"
              />
            </div>
            <div class="form-item">
              <label class="form-label">{{ $t('admin.org.sortOrder') }}</label>
              <t-input-number
                v-model="form.sort_order"
                :min="0"
                :max="9999"
                theme="normal"
              />
            </div>
          </div>
          <div class="editor-footer">
            <t-button variant="outline" @click="handleClose">{{ $t('common.cancel') }}</t-button>
            <t-button theme="primary" :loading="saving" @click="handleSubmit">{{ $t('common.confirm') }}</t-button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useOrgTreeStore } from '@/stores/orgTree'
import { useI18n } from 'vue-i18n'
import type { OrgTreeNode } from '@/api/org-tree'

const props = defineProps<{
  visible: boolean
  mode: 'create' | 'edit'
  node: OrgTreeNode | null
  parentId: string | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', val: boolean): void
  (e: 'success'): void
}>()

const orgTreeStore = useOrgTreeStore()
const { t } = useI18n()
const saving = ref(false)

const form = ref({
  name: '',
  description: '',
  sort_order: 0,
})

watch(() => props.visible, (val) => {
  if (val) {
    if (props.mode === 'edit' && props.node) {
      form.value = {
        name: props.node.name,
        description: props.node.description || '',
        sort_order: props.node.sort_order || 0,
      }
    } else {
      form.value = { name: '', description: '', sort_order: 0 }
    }
  }
})

const handleClose = () => {
  emit('update:visible', false)
}

const handleSubmit = async () => {
  if (!form.value.name.trim()) {
    MessagePlugin.warning(t('admin.org.nameRequired'))
    return
  }
  saving.value = true
  try {
    if (props.mode === 'create') {
      await orgTreeStore.createNode({
        name: form.value.name.trim(),
        description: form.value.description.trim() || undefined,
        parent_id: props.parentId,
        sort_order: form.value.sort_order,
      })
      MessagePlugin.success(t('admin.org.createSuccess'))
    } else if (props.node) {
      await orgTreeStore.updateNode(props.node.id, {
        name: form.value.name.trim(),
        description: form.value.description.trim() || undefined,
        sort_order: form.value.sort_order,
      })
      MessagePlugin.success(t('admin.org.updateSuccess'))
    }
    emit('success')
  } catch {
    MessagePlugin.error(t('common.operationFailed'))
  } finally {
    saving.value = false
  }
}
</script>

<style lang="less" scoped>
.editor-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.4);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 2000;
}

.editor-dialog {
  background: #fff;
  border-radius: 12px;
  width: 480px;
  max-width: 90vw;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.12);
}

.editor-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px 16px;
  border-bottom: 1px solid #e7e7e7;

  h3 {
    font-size: 16px;
    font-weight: 600;
    margin: 0;
    color: #1a1a1a;
  }
}

.editor-body {
  padding: 20px 24px;

  .form-item {
    margin-bottom: 16px;

    &:last-child {
      margin-bottom: 0;
    }

    .form-label {
      display: block;
      font-size: 14px;
      font-weight: 500;
      color: #333;
      margin-bottom: 8px;

      &.required::before {
        content: '*';
        color: #e34d59;
        margin-right: 4px;
      }
    }
  }
}

.editor-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 16px 24px 20px;
  border-top: 1px solid #e7e7e7;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
