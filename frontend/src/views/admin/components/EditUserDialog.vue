<template>
  <t-dialog
    :visible="visible"
    :header="$t('admin.member.editUserIn', { org: orgName })"
    :confirm-btn="{ content: $t('common.confirm'), loading: submitting }"
    :cancel-btn="$t('common.cancel')"
    :close-on-overlay-click="false"
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
          <t-radio value="admin">{{ $t('admin.member.roleSubOrgAdmin') }}</t-radio>
        </t-radio-group>
      </t-form-item>

      <t-form-item :label="$t('admin.member.department')" name="orgId">
        <t-select
          v-model="formData.orgId"
          :options="orgOptions"
          :placeholder="$t('admin.member.departmentPlaceholder')"
          filterable
          clearable
        />
      </t-form-item>
    </t-form>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, reactive, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { updateUserInOrg, assignUserToOrg, removeUserFromOrg, type OrgMember, type OrgTreeNode } from '@/api/org-tree'
import { useOrgTreeStore } from '@/stores/orgTree'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const orgTreeStore = useOrgTreeStore()

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

type OrgRole = 'admin' | 'editor' | 'viewer'
type TenantRole = 'contributor' | 'viewer'

const formData = reactive({
  username: '',
  email: '',
  phone: '',
  role: 'viewer' as OrgRole,
  orgId: '' as string,
})

// 可选部门列表：扁平化整棵组织树，按层级缩进展示；包含当前组织
const orgOptions = computed(() => {
  const build = (nodes: OrgTreeNode[], depth: number): { label: string; value: string }[] => {
    const result: { label: string; value: string }[] = []
    for (const n of nodes) {
      result.push({ label: '　'.repeat(depth) + n.name, value: n.id })
      if (n.children) result.push(...build(n.children, depth + 1))
    }
    return result
  }
  return build(orgTreeStore.tree, 0)
})

const formRules = {
  username: [
    { required: true, message: t('auth.usernameRequired'), trigger: 'blur' },
    { min: 2, message: t('auth.usernameMinLength'), trigger: 'blur' },
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
    formData.orgId = props.orgId
  } else {
    formData.orgId = props.orgId
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
    const oldOrgId = props.orgId
    const newOrgId = formData.orgId
    const userId = props.user.user_id

    // 1) 先更新资料/角色（作用在原所属组织上）
    const res = await updateUserInOrg(oldOrgId, userId, {
      username: formData.username,
      email: formData.email || undefined,
      phone: formData.phone || undefined,
      role: formData.role,
      tenant_role: defaultTenantRoleForOrgRole(formData.role),
    })
    if (!res.success) {
      MessagePlugin.error(res.message || t('admin.member.updateUserFailed'))
      return
    }

    // 2) 部门变更则移动成员：先加入新组织（沿用刚选的角色），再移出旧组织。
    //    顺序保证任意时刻成员都至少属于一个组织，避免中间态丢失。
    if (newOrgId && newOrgId !== oldOrgId) {
      const assignRes = await assignUserToOrg(newOrgId, { user_id: userId, role: formData.role })
      if (!assignRes.success) {
        MessagePlugin.error(assignRes.message || t('admin.member.updateUserFailed'))
        return
      }
      const removeRes = await removeUserFromOrg(oldOrgId, userId)
      if (!removeRes.success) {
        MessagePlugin.error(removeRes.message || t('admin.member.updateUserFailed'))
        return
      }
      MessagePlugin.success(t('admin.member.departmentMoved'))
    }

    MessagePlugin.success(t('admin.member.updateUserSuccess'))
    emit('update:visible', false)
    emit('success')
  } catch (err) {
    MessagePlugin.error(t('admin.member.updateUserFailed'))
  } finally {
    submitting.value = false
  }
}
</script>
