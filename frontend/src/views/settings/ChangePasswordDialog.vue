<template>
  <t-popup
    v-model="visible"
    trigger="click"
    placement="bottom-end"
    destroy-on-close
    overlay-class-name="user-profile-password-popup-overlay"
  >
    <t-button
      theme="default"
      variant="text"
      shape="square"
      size="small"
      class="change-password-trigger"
      :title="$t('userProfile.changePassword.label')"
      :aria-label="$t('userProfile.changePassword.label')"
    >
      <template #icon>
        <t-icon name="edit" />
      </template>
    </t-button>
    <template #content>
      <div class="password-popup-inner" @click.stop>
        <div class="password-popup-title">{{ $t('userProfile.changePassword.label') }}</div>
        <p class="password-popup-hint">{{ $t('userProfile.changePassword.description') }}</p>
        <t-form
          ref="passwordFormRef"
          :data="passwordForm"
          :rules="passwordRules"
          label-align="top"
          class="password-popup-form"
          @submit.prevent
        >
          <t-form-item :label="$t('userProfile.changePassword.currentLabel')" name="oldPassword">
            <t-input
              v-model="passwordForm.oldPassword"
              type="password"
              autocomplete="current-password"
              :disabled="passwordSubmitting"
              :placeholder="$t('userProfile.changePassword.currentPlaceholder')"
            />
          </t-form-item>
          <t-form-item :label="$t('userProfile.changePassword.newLabel')" name="newPassword">
            <t-input
              v-model="passwordForm.newPassword"
              type="password"
              autocomplete="new-password"
              :disabled="passwordSubmitting"
              :placeholder="$t('userProfile.changePassword.newPlaceholder')"
            />
          </t-form-item>
          <t-form-item :label="$t('userProfile.changePassword.confirmLabel')" name="confirmPassword">
            <t-input
              v-model="passwordForm.confirmPassword"
              type="password"
              autocomplete="new-password"
              :disabled="passwordSubmitting"
              :placeholder="$t('userProfile.changePassword.confirmPlaceholder')"
              @enter="submitPasswordChange"
            />
          </t-form-item>
        </t-form>
        <div class="password-popup-footer">
          <t-button variant="outline" :disabled="passwordSubmitting" @click="closePasswordPopup">
            {{ $t('common.cancel') }}
          </t-button>
          <t-button theme="primary" :loading="passwordSubmitting" @click="submitPasswordChange">
            {{ $t('userProfile.changePassword.submit') }}
          </t-button>
        </div>
      </div>
    </template>
  </t-popup>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import type { FormInstanceFunctions, FormRule } from 'tdesign-vue-next'
import { changePassword, logout as logoutApi } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const router = useRouter()
const authStore = useAuthStore()

const visible = ref(false)
const passwordFormRef = ref<FormInstanceFunctions | null>(null)
const passwordSubmitting = ref(false)
const passwordForm = reactive({
  oldPassword: '',
  newPassword: '',
  confirmPassword: '',
})

watch(visible, (open) => {
  if (open) {
    resetPasswordForm()
  }
})

const passwordRules = computed<Record<string, FormRule[]>>(() => ({
  oldPassword: [
    { required: true, message: t('userProfile.changePassword.currentRequired'), type: 'error' },
  ],
  newPassword: [
    { required: true, message: t('auth.passwordRequired'), type: 'error' },
    { min: 8, message: t('auth.passwordMinLength'), type: 'error' },
    { max: 32, message: t('auth.passwordMaxLength'), type: 'error' },
    { pattern: /[a-zA-Z]/, message: t('auth.passwordMustContainLetter'), type: 'error' },
    { pattern: /\d/, message: t('auth.passwordMustContainNumber'), type: 'error' },
    {
      validator: (val: string) => val !== passwordForm.oldPassword,
      message: t('userProfile.changePassword.sameAsCurrent'),
      type: 'error',
    },
  ],
  confirmPassword: [
    { required: true, message: t('auth.confirmPasswordRequired'), type: 'error' },
    {
      validator: (val: string) => val === passwordForm.newPassword,
      message: t('auth.passwordMismatch'),
      type: 'error',
      trigger: 'blur',
    },
  ],
}))

function resetPasswordForm() {
  passwordForm.oldPassword = ''
  passwordForm.newPassword = ''
  passwordForm.confirmPassword = ''
  passwordFormRef.value?.clearValidate?.()
}

function closePasswordPopup() {
  if (passwordSubmitting.value) return
  visible.value = false
  resetPasswordForm()
}

async function submitPasswordChange() {
  if (passwordSubmitting.value) return
  const result = await passwordFormRef.value?.validate?.()
  if (result !== true) return

  passwordSubmitting.value = true
  try {
    const resp = await changePassword({
      old_password: passwordForm.oldPassword,
      new_password: passwordForm.newPassword,
    })
    if (!resp.success) {
      MessagePlugin.error(resp.message || t('userProfile.changePassword.failed'))
      return
    }

    visible.value = false
    MessagePlugin.success(t('userProfile.changePassword.success'))
    resetPasswordForm()

    try {
      await logoutApi()
    } catch {
      // Ignore; local cleanup still proceeds.
    }
    authStore.logout()
    router.push('/login')
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('userProfile.changePassword.failed'))
  } finally {
    passwordSubmitting.value = false
  }
}
</script>

<style lang="less" scoped>
.change-password-trigger {
  flex-shrink: 0;
}

.password-popup-inner {
  max-width: 100%;
}

.password-popup-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin: 0 0 8px;
  line-height: 1.35;
}

.password-popup-hint {
  margin: 0 0 12px;
  font-size: 13px;
  line-height: 1.55;
  color: var(--td-text-color-secondary);
}

.password-popup-form {
  :deep(.t-form__item) {
    margin-bottom: 14px;

    &:last-child {
      margin-bottom: 4px;
    }
  }
}

.password-popup-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 16px;
}
</style>

<style lang="less">
.user-profile-password-popup-overlay {
  z-index: 3050 !important;

  .t-popup__content {
    padding: 14px 16px !important;
    min-width: 300px;
    max-width: min(392px, calc(100vw - 24px));
    border-radius: 12px !important;
    background: var(--td-bg-color-container) !important;
    border: 0.5px solid var(--td-component-stroke) !important;
    box-shadow:
      0 0 0 0.5px rgba(0, 0, 0, 0.03),
      0 2px 4px rgba(0, 0, 0, 0.04),
      0 8px 24px rgba(0, 0, 0, 0.1) !important;
    backdrop-filter: blur(20px) saturate(180%) !important;
    -webkit-backdrop-filter: blur(20px) saturate(180%) !important;
  }
}

:root[theme-mode='dark'] .user-profile-password-popup-overlay .t-popup__content {
  background: rgba(36, 36, 36, 0.92) !important;
  border-color: rgba(255, 255, 255, 0.08) !important;
  box-shadow:
    0 0 0 0.5px rgba(255, 255, 255, 0.05),
    0 2px 4px rgba(0, 0, 0, 0.12),
    0 8px 32px rgba(0, 0, 0, 0.28) !important;
}
</style>
