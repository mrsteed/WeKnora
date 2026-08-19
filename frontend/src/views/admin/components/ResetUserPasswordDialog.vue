<template>
  <t-dialog
    :visible="visible"
    :header="$t('admin.member.resetPasswordFor', { org: orgName })"
    :confirm-btn="{ content: $t('admin.member.resetPassword'), loading: submitting }"
    :cancel-btn="$t('common.cancel')"
    @confirm="handleSubmit"
    @close="handleClose"
    width="480px"
  >
    <t-form ref="formRef" :data="formData" :rules="formRules" label-align="top">
      <t-form-item :label="$t('admin.member.username')">
        <t-input :value="user?.username || ''" readonly />
      </t-form-item>

      <t-form-item :label="$t('auth.password')" name="newPassword">
        <t-input
          v-model="formData.newPassword"
          type="password"
          :placeholder="$t('auth.passwordPlaceholder')"
          clearable
        />
      </t-form-item>

      <t-form-item :label="$t('auth.confirmPassword')" name="confirmPassword">
        <t-input
          v-model="formData.confirmPassword"
          type="password"
          :placeholder="$t('auth.confirmPasswordPlaceholder')"
          clearable
        />
      </t-form-item>
    </t-form>
  </t-dialog>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { updateUserPasswordInOrg, type OrgMember } from '@/api/org-tree'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps<{
  visible: boolean
  orgId: string
  orgName: string
  user: OrgMember | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', val: boolean): void
  (e: 'success'): void
}>()

const formRef = ref()
const submitting = ref(false)

const formData = reactive({
  newPassword: '',
  confirmPassword: '',
})

const formRules = {
  newPassword: [
    { required: true, message: () => t('auth.passwordRequired'), trigger: 'blur' },
    { min: 8, message: () => t('auth.passwordMinLength'), trigger: 'blur' },
    { max: 32, message: () => t('auth.passwordMaxLength'), trigger: 'blur' },
    { pattern: /[a-zA-Z]/, message: () => t('auth.passwordMustContainLetter'), trigger: 'blur' },
    { pattern: /\d/, message: () => t('auth.passwordMustContainNumber'), trigger: 'blur' },
  ],
  confirmPassword: [
    { required: true, message: () => t('auth.confirmPasswordRequired'), trigger: 'blur' },
    {
      validator: (val: string) => val === formData.newPassword,
      message: () => t('auth.passwordMismatch'),
      trigger: 'blur',
    },
  ],
}

const resetForm = () => {
  formData.newPassword = ''
  formData.confirmPassword = ''
}

watch(() => props.visible, (val) => {
  if (val) {
    resetForm()
  }
})

const handleClose = () => {
  emit('update:visible', false)
}

const handleSubmit = async () => {
  const valid = await formRef.value?.validate()
  if (valid !== true || !props.user) {
    if (!props.user) {
      MessagePlugin.error(t('admin.member.resetPasswordFailed'))
    }
    return
  }

  submitting.value = true
  try {
    const res = await updateUserPasswordInOrg(props.orgId, props.user.user_id, {
      new_password: formData.newPassword,
      confirm_password: formData.confirmPassword,
    })
    if (res.success) {
      MessagePlugin.success(t('admin.member.resetPasswordSuccess'))
      emit('update:visible', false)
      emit('success')
    } else {
      MessagePlugin.error(res.message || t('admin.member.resetPasswordFailed'))
    }
  } catch {
    MessagePlugin.error(t('admin.member.resetPasswordFailed'))
  } finally {
    submitting.value = false
  }
}
</script>