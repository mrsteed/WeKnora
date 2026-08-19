<template>
  <t-dialog
    :visible="visible"
    :header="$t('admin.member.createUserTo', { org: orgName })"
    :confirm-btn="{ content: $t('admin.member.createUser'), loading: submitting }"
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

      <t-form-item :label="$t('auth.password')" name="password">
        <t-input
          v-model="formData.password"
          type="password"
          :placeholder="$t('admin.member.passwordPlaceholder')"
          clearable
        />
      </t-form-item>

      <t-form-item :label="$t('admin.member.role')" name="role">
        <t-radio-group v-model="formData.role">
          <t-radio v-for="option in orgRoleOptions" :key="option.value" :value="option.value">
            {{ option.label }}
          </t-radio>
        </t-radio-group>
      </t-form-item>

    </t-form>
  </t-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, watch, computed } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { createUserInOrg } from '@/api/org-tree'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps<{
  visible: boolean
  orgId: string
  orgName: string
}>()

const emit = defineEmits<{
  (e: 'update:visible', val: boolean): void
  (e: 'success'): void
}>()

const formRef = ref()
const submitting = ref(false)

type OrgRole = 'admin' | 'editor' | 'viewer'
type TenantRole = 'contributor' | 'viewer'

const formData = reactive({
  username: '',
  email: '',
  phone: '',
  password: '',
  role: 'viewer' as OrgRole,
})

const orgRoleOptions = computed(() => ([
  { value: 'viewer' as const, label: t('admin.member.roleViewer') },
  { value: 'editor' as const, label: t('admin.member.roleEditor') },
  { value: 'admin' as const, label: t('admin.member.roleSubOrgAdmin') },
]))

const formRules = {
  username: [
    { required: true, message: () => t('auth.usernameRequired'), trigger: 'blur' },
    { min: 2, message: () => t('auth.usernameMinLength'), trigger: 'blur' },
  ],
  password: [
    { required: true, message: () => t('auth.passwordRequired'), trigger: 'blur' },
    { min: 8, message: () => t('auth.passwordMinLength'), trigger: 'blur' },
    { max: 32, message: () => t('auth.passwordMaxLength'), trigger: 'blur' },
  ],
  role: [
    { required: true, trigger: 'change' },
  ],
}

const resetForm = () => {
  formData.username = ''
  formData.email = ''
  formData.phone = ''
  formData.password = ''
  formData.role = 'viewer'
}

watch(() => props.visible, (val) => {
  if (val) {
    resetForm()
  }
})

const defaultTenantRoleForOrgRole = (role: OrgRole): TenantRole => {
  return role === 'viewer' ? 'viewer' : 'contributor'
}

const handleClose = () => {
  emit('update:visible', false)
}

const handleSubmit = async () => {
  // Validate form
  const valid = await formRef.value?.validate()
  if (valid !== true) return

  // At least one of email or phone is required
  if (!formData.email && !formData.phone) {
    MessagePlugin.warning(t('admin.member.emailOrPhoneRequired'))
    return
  }

  submitting.value = true
  try {
    const res = await createUserInOrg(props.orgId, {
      username: formData.username,
      email: formData.email || undefined,
      phone: formData.phone || undefined,
      password: formData.password,
      role: formData.role,
      tenant_role: defaultTenantRoleForOrgRole(formData.role),
    })
    if (res.success) {
      MessagePlugin.success(t('admin.member.createUserSuccess'))
      emit('update:visible', false)
      emit('success')
    } else {
      MessagePlugin.error(res.message || t('admin.member.createUserFailed'))
    }
  } catch {
    MessagePlugin.error(t('admin.member.createUserFailed'))
  } finally {
    submitting.value = false
  }
}
</script>
