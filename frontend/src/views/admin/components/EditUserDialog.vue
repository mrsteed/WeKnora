<template>
  <t-dialog
    :visible="visible"
    :header="$t('admin.member.editUserIn', { org: orgName })"
    :confirm-btn="{ content: $t('common.confirm'), loading: submitting }"
    :cancel-btn="$t('common.cancel')"
    @confirm="handleSubmit"
    @close="handleClose"
    width="480px"
  >
    <t-form ref="formRef" :data="formData" :rules="formRules" label-align="top">
      <t-form-item :label="$t('admin.member.username')" name="username">
        <t-input
          v-model="formData.username"
          :placeholder="$t('admin.member.usernamePlaceholder')"
          clearable
        />
      </t-form-item>

      <t-form-item :label="$t('admin.member.email')" name="email">
        <t-input
          v-model="formData.email"
          :placeholder="$t('admin.member.emailOptionalPlaceholder')"
          clearable
        />
      </t-form-item>

      <t-form-item :label="$t('admin.member.phone')" name="phone">
        <t-input
          v-model="formData.phone"
          :placeholder="$t('admin.member.phonePlaceholder')"
          clearable
        />
      </t-form-item>

      <t-form-item :label="$t('admin.member.role')" name="role">
        <t-radio-group v-model="formData.role">
          <t-radio value="viewer">{{ $t('admin.member.roleViewer') }}</t-radio>
          <t-radio value="editor">{{ $t('admin.member.roleEditor') }}</t-radio>
          <t-radio value="admin">{{ $t('admin.member.roleAdmin') }}</t-radio>
        </t-radio-group>
      </t-form-item>
    </t-form>
  </t-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { updateUserInOrg, type OrgMember } from '@/api/org-tree'
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
  username: '',
  email: '',
  phone: '',
  role: 'viewer' as 'admin' | 'editor' | 'viewer',
})

const formRules = {
  username: [
    { required: true, message: () => t('auth.usernameRequired'), trigger: 'blur' },
    { min: 2, message: () => t('auth.usernameMinLength'), trigger: 'blur' },
  ],
  role: [
    { required: true, trigger: 'change' },
  ],
}

const loadUserData = () => {
  if (props.user) {
    formData.username = props.user.username || ''
    formData.email = props.user.email || ''
    formData.phone = props.user.phone || ''
    formData.role = (props.user.role as 'admin' | 'editor' | 'viewer') || 'viewer'
  }
}

watch(() => props.visible, (val) => {
  if (val) {
    loadUserData()
  }
})

watch(() => props.user, () => {
  if (props.visible) {
    loadUserData()
  }
})

const handleClose = () => {
  emit('update:visible', false)
}

const handleSubmit = async () => {
  // Validate form
  const valid = await formRef.value?.validate()
  if (valid !== true) return

  if (!props.user) {
    MessagePlugin.error(t('admin.member.updateUserFailed'))
    return
  }

  // At least one of email or phone is required
  if (!formData.email && !formData.phone) {
    MessagePlugin.warning(t('admin.member.emailOrPhoneRequired'))
    return
  }

  submitting.value = true
  try {
    const res = await updateUserInOrg(props.orgId, props.user.user_id, {
      username: formData.username,
      email: formData.email || undefined,
      phone: formData.phone || undefined,
      role: formData.role,
    })
    if (res.success) {
      MessagePlugin.success(t('admin.member.updateUserSuccess'))
      emit('update:visible', false)
      emit('success')
    } else {
      MessagePlugin.error(res.message || t('admin.member.updateUserFailed'))
    }
  } catch (err) {
    MessagePlugin.error(t('admin.member.updateUserFailed'))
  } finally {
    submitting.value = false
  }
}
</script>
