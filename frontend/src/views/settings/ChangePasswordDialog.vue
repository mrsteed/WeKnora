<template>
  <t-dialog
    :visible="visible"
    :header="$t('userProfile.changePasswordTitle')"
    :confirm-btn="{
      content: $t('userProfile.changePasswordConfirm'),
      loading: submitting,
    }"
    :cancel-btn="$t('common.cancel')"
    :close-btn="!submitting"
    :close-on-overlay-click="!submitting"
    :on-confirm="handleSubmit"
    :on-close="handleClose"
    :on-cancel="handleClose"
    width="480px"
    @update:visible="handleVisibleUpdate"
  >
    <div class="change-password-dialog">
      <p class="change-password-dialog__desc">
        {{ $t('userProfile.changePasswordDesc') }}
      </p>

      <t-form-item :label="$t('userProfile.fields.oldPassword')">
        <t-input
          v-model="oldPassword"
          type="password"
          :placeholder="$t('userProfile.placeholders.oldPassword')"
          :status="oldPasswordError ? 'error' : undefined"
          :tips="oldPasswordError || undefined"
          autocomplete="current-password"
          @blur="touchOldPassword = true"
        />
      </t-form-item>

      <t-form-item :label="$t('userProfile.fields.newPassword')">
        <t-input
          v-model="newPassword"
          type="password"
          :placeholder="$t('userProfile.placeholders.newPassword')"
          :status="newPasswordError ? 'error' : undefined"
          :tips="newPasswordError || undefined"
          autocomplete="new-password"
          @blur="touchNewPassword = true"
        />
      </t-form-item>

      <t-form-item :label="$t('userProfile.fields.confirmPassword')">
        <t-input
          v-model="confirmPassword"
          type="password"
          :placeholder="$t('userProfile.placeholders.confirmPassword')"
          :status="confirmPasswordError ? 'error' : undefined"
          :tips="confirmPasswordError || undefined"
          autocomplete="new-password"
          @blur="touchConfirmPassword = true"
        />
      </t-form-item>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { changePassword } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'

interface Props {
  visible: boolean
}
const props = defineProps<Props>()
const emit = defineEmits<{
  'update:visible': [value: boolean]
}>()

const { t } = useI18n()
const authStore = useAuthStore()

const oldPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')

const touchOldPassword = ref(false)
const touchNewPassword = ref(false)
const touchConfirmPassword = ref(false)

const submitting = ref(false)

// 服务端使用 validateLoginPassword：长度 8-32，同时包含字母与数字，
// 且不能与 username 相同（[internal/application/service/user.go:646]）。
// 前端正则与按钮处的 disabled 判断都按 8 起，handler binding 也是 8，
// 这里与后端规则对齐，先在客户端挡一道以减少不必要的往返。
const strongPasswordRegex = /^(?=.*[A-Za-z])(?=.*\d).{8,32}$/
const validateNewPasswordShape = (value: string): boolean => strongPasswordRegex.test(value)

const oldPasswordError = computed(() => {
  if (!touchOldPassword.value) return ''
  if (!oldPassword.value.trim()) {
    return t('userProfile.errors.oldRequired')
  }
  return ''
})

const newPasswordError = computed(() => {
  if (!touchNewPassword.value) return ''
  if (!newPassword.value) return t('userProfile.errors.newRequired')
  // 服务端 service.validateLoginPassword 校验：8-32 位 + 含字母与数字 +
  // 不能与用户名相同。前端按相同顺序拆开报错，保证错误信息更具体，
  // 而不是把所有弱密码口径都打到 generic 的 weak 文案上。
  if (newPassword.value.length > 0 && newPassword.value.length < 8) {
    return t('userProfile.errors.tooShort')
  }
  if (newPassword.value.length > 32) {
    return t('userProfile.errors.tooLong')
  }
  if (
    newPassword.value === authStore.user?.username ||
    !/^(?=.*[A-Za-z])(?=.*\d)/.test(newPassword.value)
  ) {
    return t('userProfile.errors.weak')
  }
  if (confirmPassword.value && newPassword.value !== confirmPassword.value) {
    return t('userProfile.errors.mismatch')
  }
  return ''
})

const confirmPasswordError = computed(() => {
  if (!touchConfirmPassword.value) return ''
  if (!confirmPassword.value) return t('userProfile.errors.confirmRequired')
  if (newPassword.value && newPassword.value !== confirmPassword.value) {
    return t('userProfile.errors.mismatch')
  }
  return ''
})

const isFormValid = computed(() => {
  return Boolean(
    oldPassword.value.trim() &&
      newPassword.value &&
      confirmPassword.value &&
      validateNewPasswordShape(newPassword.value) &&
      newPassword.value !== authStore.user?.username &&
      newPassword.value === confirmPassword.value,
  )
})

const resetState = () => {
  oldPassword.value = ''
  newPassword.value = ''
  confirmPassword.value = ''
  touchOldPassword.value = false
  touchNewPassword.value = false
  touchConfirmPassword.value = false
}

const handleClose = () => {
  if (submitting.value) return
  resetState()
  emit('update:visible', false)
}

const handleVisibleUpdate = (value: boolean) => {
  if (value) return
  handleClose()
}

watch(
  () => props.visible,
  (next) => {
    if (next) {
      // 重新打开时清空遗留字段，但保留已输入的旧 / 新密码，以支持
      // 用户先去校对再点重试；这里仅在「原本就是关闭状态」转打开时清空。
      resetState()
    }
  },
)

const handleSubmit = async () => {
  touchOldPassword.value = true
  touchNewPassword.value = true
  touchConfirmPassword.value = true
  if (!isFormValid.value) {
    return
  }
  submitting.value = true
  try {
    const res = await changePassword({
      old_password: oldPassword.value,
      new_password: newPassword.value,
    })
    if (res?.success) {
      MessagePlugin.success(t('userProfile.changePasswordSuccess'))
      resetState()
      emit('update:visible', false)
      return
    }
    // 服务端旧密码错误的文案固定以小写 "old password" 命中；
    // 弱密码/通用错误分桶显示，避免给前端用户看到不友好的英文报错。
    const message = (res?.message || '').toLowerCase()
    if (message.includes('old password')) {
      MessagePlugin.error(t('userProfile.errors.oldWrong'))
    } else if (
      message.includes('password') ||
      message.includes('校验') ||
      message.includes('invalid')
    ) {
      MessagePlugin.error(t('userProfile.errors.weak'))
    } else {
      MessagePlugin.error(res?.message || t('userProfile.errors.changePasswordUnknown'))
    }
  } catch (error: any) {
    MessagePlugin.error(
      error?.message || t('userProfile.errors.changePasswordUnknown'),
    )
  } finally {
    submitting.value = false
  }
}
</script>

<style lang="less" scoped>
.change-password-dialog {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 8px 0 4px;

  &__desc {
    margin: 0 0 4px;
    font-size: 13px;
    color: var(--td-text-color-secondary);
    line-height: 1.6;
  }
}
</style>
