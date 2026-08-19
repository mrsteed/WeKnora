<template>
  <div class="reset-user-password-dialog">
    <t-button
      theme="danger"
      variant="text"
      class="password-reset-trigger"
      @click="openDialog"
    >
      <template #icon><t-icon name="lock-on" /></template>
      {{ $t('system.globalSettings.passwordReset.action') }}
    </t-button>

    <t-dialog
      v-model:visible="visible"
      :header="$t('system.globalSettings.passwordReset.dialogTitle')"
      width="480px"
      destroy-on-close
      dialog-class-name="password-reset-dialog"
      :confirm-btn="{
        content: t('system.globalSettings.passwordReset.confirmBtn'),
        theme: 'danger',
        loading: submitting,
      }"
      :cancel-btn="{
        content: t('system.globalSettings.confirm.cancelBtn'),
        variant: 'outline',
      }"
      :close-on-overlay-click="!submitting"
      :close-btn="!submitting"
      @confirm="submit"
      @close="resetForm"
    >
      <t-alert
        theme="warning"
        :message="$t('system.globalSettings.passwordReset.warning')"
        class="password-reset-warning"
      />
      <t-form
        ref="formRef"
        :data="form"
        :rules="rules"
        label-align="top"
        class="password-reset-form"
      >
        <t-form-item :label="$t('system.globalSettings.passwordReset.emailLabel')" name="email">
          <t-input
            v-model="form.email"
            type="text"
            clearable
            autocomplete="off"
            :disabled="submitting"
            :placeholder="$t('system.globalSettings.passwordReset.emailPlaceholder')"
          />
        </t-form-item>
        <t-form-item :label="$t('system.globalSettings.passwordReset.newPasswordLabel')" name="newPassword">
          <t-input
            v-model="form.newPassword"
            type="password"
            autocomplete="new-password"
            :disabled="submitting"
            :placeholder="$t('system.globalSettings.passwordReset.newPasswordPlaceholder')"
          >
            <template #prefix-icon><t-icon name="lock-on" /></template>
          </t-input>
        </t-form-item>
        <t-form-item :label="$t('system.globalSettings.passwordReset.confirmPasswordLabel')" name="confirmPassword">
          <t-input
            v-model="form.confirmPassword"
            type="password"
            autocomplete="new-password"
            :disabled="submitting"
            :placeholder="$t('system.globalSettings.passwordReset.confirmPasswordPlaceholder')"
            @enter="submit"
          >
            <template #prefix-icon><t-icon name="lock-on" /></template>
          </t-input>
        </t-form-item>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { nextTick, reactive, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import type { FormInstanceFunctions, FormRule } from 'tdesign-vue-next'
import { resetUserPassword } from '@/api/system'
import { useI18n } from 'vue-i18n'

const emit = defineEmits<{
  (e: 'success', message: string): void
}>()

const { t } = useI18n()

const visible = ref(false)
const submitting = ref(false)
const formRef = ref<FormInstanceFunctions>()
const form = reactive({
  email: '',
  newPassword: '',
  confirmPassword: '',
})

const rules: Record<string, FormRule[]> = {
  email: [
    { required: true, message: t('system.globalSettings.passwordReset.validation.emailRequired'), trigger: 'blur' },
    { email: true, message: t('system.globalSettings.passwordReset.validation.emailInvalid'), trigger: 'blur' },
  ],
  newPassword: [
    { required: true, message: t('system.globalSettings.passwordReset.validation.passwordRequired'), trigger: 'blur' },
    { min: 8, message: t('system.globalSettings.passwordReset.validation.passwordLength'), trigger: 'blur' },
    { max: 32, message: t('system.globalSettings.passwordReset.validation.passwordLength'), trigger: 'blur' },
    { pattern: /[a-zA-Z]/, message: t('system.globalSettings.passwordReset.validation.passwordLetter'), trigger: 'blur' },
    { pattern: /\d/, message: t('system.globalSettings.passwordReset.validation.passwordNumber'), trigger: 'blur' },
  ],
  confirmPassword: [
    { required: true, message: t('system.globalSettings.passwordReset.validation.confirmRequired'), trigger: 'blur' },
    {
      validator: (value: string) => value === form.newPassword,
      message: t('system.globalSettings.passwordReset.validation.passwordMismatch'),
      trigger: 'blur',
    },
  ],
}

function resetForm() {
  form.email = ''
  form.newPassword = ''
  form.confirmPassword = ''
  formRef.value?.clearValidate?.()
}

async function openDialog() {
  resetForm()
  visible.value = true
  await nextTick()
  formRef.value?.clearValidate?.()
}

async function submit() {
  if (submitting.value) return
  const valid = await formRef.value?.validate?.()
  if (valid !== true) return

  submitting.value = true
  try {
    await resetUserPassword({
      email: form.email.trim(),
      new_password: form.newPassword,
    })
    const message = t('system.globalSettings.passwordReset.success')
    MessagePlugin.success(message)
    emit('success', message)
    visible.value = false
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('system.globalSettings.passwordReset.failed'))
  } finally {
    submitting.value = false
  }
}
</script>

<style lang="less" scoped>
.password-reset-trigger {
  min-width: 112px;
  height: 32px;
  padding: 0 12px;
  color: var(--td-error-color);
  background: var(--td-error-color-light);
  border: 1px solid transparent;
  border-radius: 6px;

  &:hover {
    color: var(--td-error-color-hover);
    background: var(--td-error-color-light-hover);
    border-color: var(--td-error-color-focus);
  }

  &:active {
    color: var(--td-error-color-active);
    background: var(--td-error-color-focus);
  }
}

.password-reset-warning {
  margin-bottom: 20px;
}
</style>

<style lang="less">
.password-reset-dialog {
  padding: 0;
  overflow: hidden;
  border-color: var(--td-component-stroke);
  border-radius: 12px;
  box-shadow:
    0 12px 32px rgba(15, 23, 42, 0.12),
    0 2px 8px rgba(15, 23, 42, 0.08);

  .t-dialog__header {
    min-height: 64px;
    padding: 0 24px;
    font-size: 18px;
    line-height: 26px;
    border-bottom: 1px solid var(--td-component-stroke);
  }

  .t-dialog__close {
    width: 28px;
    height: 28px;
    padding: 0;
    justify-content: center;
    border-radius: 6px;
  }

  .t-dialog__body {
    padding: 20px 24px 4px;
  }

  .password-reset-warning {
    padding: 12px 14px;
    border-radius: 8px;

    .t-alert__content {
      font-size: 13px;
      line-height: 20px;
    }
  }

  .password-reset-form {
    .t-form__item {
      margin-bottom: 16px;
    }

    .t-form__label--top {
      min-height: 28px;
      padding: 0;
      font-size: 14px;
      line-height: 22px;
    }

    .t-input {
      border-radius: 6px;
    }
  }

  .t-dialog__footer {
    box-sizing: border-box;
    padding: 16px 24px 20px;
    border-top: 1px solid var(--td-component-stroke);

    .t-button {
      min-width: 88px;
      border-radius: 6px;
    }
  }
}

@media (max-width: 480px) {
  .password-reset-dialog {
    width: calc(100vw - 24px) !important;

    .t-dialog__header {
      min-height: 56px;
      padding: 0 20px;
      font-size: 17px;
    }

    .t-dialog__body {
      padding: 16px 20px 4px;
    }

    .t-dialog__footer {
      padding: 14px 20px 18px;
    }
  }
}
</style>
