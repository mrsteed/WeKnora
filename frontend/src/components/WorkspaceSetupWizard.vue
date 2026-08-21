<template>
  <t-dialog
    :visible="visible"
    width="760px"
    :footer="false"
    :close-btn="false"
    :close-on-overlay-click="false"
    :close-on-esc-keydown="false"
  >
    <template #header>
      <div class="workspace-setup-header">
        <span class="workspace-setup-header__avatar">
          <t-icon name="system-sum" size="20px" />
        </span>
        <div class="workspace-setup-header__body">
          <div class="workspace-setup-header__row">
            <span class="workspace-setup-header__title">{{ t('tenant.setupWizard.title') }}</span>
          </div>
          <p class="workspace-setup-header__desc">
            {{ t('tenant.setupWizard.subtitle', { name: pendingTenantName }) }}
          </p>
        </div>
      </div>
    </template>

    <div class="workspace-setup">
      <div class="workspace-setup-steps">
        <div
          v-for="(label, index) in stepTitles"
          :key="label"
          :class="['workspace-setup-step', { 'is-active': step === index, 'is-done': step > index }]"
        >
          <span class="workspace-setup-step__num">
            <t-icon v-if="step > index" name="check" size="12px" />
            <template v-else>{{ index + 1 }}</template>
          </span>
          <span class="workspace-setup-step__label">{{ label }}</span>
        </div>
      </div>

      <div v-if="step === 0" class="workspace-setup-body">
        <section class="workspace-setup-section workspace-setup-section--info">
          <p class="workspace-setup-section__copy">{{ t('tenant.setupWizard.tip') }}</p>
        </section>

        <section class="workspace-setup-section">
          <div class="workspace-setup-section__head">
            <h4>{{ t('tenant.setupWizard.modeLabel') }}</h4>
            <p>{{ t('tenant.setupWizard.introDesc') }}</p>
          </div>

          <div class="workspace-setup-mode-grid" role="radiogroup" :aria-label="t('tenant.setupWizard.modeLabel')">
            <button
              v-for="mode in modeOptions"
              :key="mode.value"
              type="button"
              :class="['workspace-setup-mode', { 'is-active': form.personMode === mode.value }]"
              :aria-checked="form.personMode === mode.value"
              role="radio"
              @click="form.personMode = mode.value"
            >
              <t-icon :name="mode.icon" class="workspace-setup-mode__icon" />
              <span class="workspace-setup-mode__title">{{ mode.title }}</span>
              <span class="workspace-setup-mode__desc">{{ mode.desc }}</span>
            </button>
          </div>
        </section>

        <section class="workspace-setup-section">
          <div class="workspace-setup-section__head">
            <h4>{{ t('tenant.setupWizard.tenantRoleLabel') }}</h4>
            <p>{{ t('tenant.setupWizard.tenantRoleDesc') }}</p>
          </div>

          <t-radio-group v-model="form.tenantRole" class="workspace-setup-radio-group">
            <t-radio v-for="role in tenantRoleOptions" :key="role.value" :value="role.value">
              {{ role.label }}
            </t-radio>
          </t-radio-group>
        </section>

        <section class="workspace-setup-section">
          <div class="workspace-setup-section__head">
            <h4>{{ currentModeTitle }}</h4>
            <p>{{ currentModeDesc }}</p>
          </div>

          <div v-if="isExistingMode" class="workspace-setup-form-grid">
            <div class="workspace-setup-field">
              <label>{{ t('tenant.setupWizard.existingEmailLabel') }}</label>
              <t-input v-model="form.existingEmail" :placeholder="t('tenant.setupWizard.existingEmailPlaceholder')" clearable />
              <p class="workspace-setup-field__hint">{{ t('tenant.setupWizard.existingEmailHint') }}</p>
            </div>
          </div>

          <div v-else-if="isNewMode" class="workspace-setup-form-grid workspace-setup-form-grid--two-col">
            <div class="workspace-setup-field">
              <label>{{ t('admin.member.username') }}</label>
              <t-input v-model="form.newUsername" :placeholder="t('admin.member.usernamePlaceholder')" clearable />
            </div>
            <div class="workspace-setup-field">
              <label>{{ t('auth.password') }}</label>
              <t-input
                v-model="form.newPassword"
                type="password"
                :placeholder="t('admin.member.passwordPlaceholder')"
                clearable
              />
            </div>
            <div class="workspace-setup-field">
              <label>{{ t('admin.member.email') }}</label>
              <t-input v-model="form.newEmail" :placeholder="t('admin.member.emailOptionalPlaceholder')" clearable />
            </div>
            <div class="workspace-setup-field">
              <label>{{ t('admin.member.phone') }}</label>
              <t-input v-model="form.newPhone" :placeholder="t('admin.member.phonePlaceholder')" clearable />
            </div>
          </div>

          <div v-else class="workspace-setup-form-grid">
            <div class="workspace-setup-field">
              <label>{{ t('tenant.setupWizard.inviteMessageLabel') }}</label>
              <t-textarea
                v-model="form.inviteMessage"
                :placeholder="t('tenant.setupWizard.inviteMessagePlaceholder')"
                :autosize="{ minRows: 3, maxRows: 5 }"
                :maxlength="240"
              />
            </div>
          </div>
        </section>
      </div>

      <div v-else-if="step === 1" class="workspace-setup-body">
        <section class="workspace-setup-section">
          <div class="workspace-setup-section__head">
            <h4>{{ t('tenant.setupWizard.rootOrgTitle') }}</h4>
            <p>{{ t('tenant.setupWizard.rootOrgDesc') }}</p>
          </div>

          <div
            v-if="createdRootOrg"
            class="workspace-setup-callout workspace-setup-callout--success"
            role="status"
          >
            {{ t('tenant.setupWizard.retryReuseRootHint', { name: createdRootOrg.name }) }}
          </div>
          <div
            v-else-if="isInviteMode"
            class="workspace-setup-callout workspace-setup-callout--neutral"
            role="status"
          >
            {{ t('tenant.setupWizard.inviteOrgHint') }}
          </div>
          <div
            v-else-if="isNewMode"
            class="workspace-setup-callout workspace-setup-callout--neutral"
            role="status"
          >
            {{ t('tenant.setupWizard.rootOrgRequiredHint') }}
          </div>

          <div class="workspace-setup-checkbox-row">
            <t-checkbox v-model="form.shouldSetupOrg" :disabled="isOrgSetupLocked">
              {{ t('tenant.setupWizard.createRootToggle') }}
            </t-checkbox>
          </div>

          <div v-if="form.shouldSetupOrg" class="workspace-setup-form-grid">
            <div class="workspace-setup-field">
              <label>{{ t('tenant.setupWizard.rootOrgNameLabel') }}</label>
              <t-input v-model="form.rootOrgName" :disabled="!!createdRootOrg" :placeholder="t('tenant.setupWizard.rootOrgNamePlaceholder', { name: pendingTenantName })" clearable />
            </div>
            <div class="workspace-setup-field">
              <label>{{ t('tenant.setupWizard.rootOrgDescLabel') }}</label>
              <t-textarea
                v-model="form.rootOrgDescription"
                :disabled="!!createdRootOrg"
                :placeholder="t('tenant.setupWizard.rootOrgDescPlaceholder')"
                :autosize="{ minRows: 2, maxRows: 4 }"
                :maxlength="240"
              />
            </div>
          </div>
        </section>

        <section v-if="form.shouldSetupOrg && !isInviteMode" class="workspace-setup-section">
          <div class="workspace-setup-section__head">
            <h4>{{ t('tenant.setupWizard.orgRoleLabel') }}</h4>
            <p>{{ t('tenant.setupWizard.orgRoleDesc') }}</p>
          </div>

          <t-radio-group v-model="form.orgRole" class="workspace-setup-radio-group">
            <t-radio v-for="role in orgRoleOptions" :key="role.value" :value="role.value">
              {{ role.label }}
            </t-radio>
          </t-radio-group>
        </section>
      </div>

      <div v-else class="workspace-setup-result">
        <div class="workspace-setup-result__hero">
          <span class="workspace-setup-result__icon">
            <t-icon name="check-circle-filled" size="24px" />
          </span>
          <div>
            <h4>{{ t('tenant.setupWizard.resultTitle') }}</h4>
            <p>{{ t('tenant.setupWizard.resultDesc', { name: pendingTenantName }) }}</p>
          </div>
        </div>

        <div class="workspace-setup-result__card">
          <p class="workspace-setup-result__summary">{{ resultSummary }}</p>
          <p class="workspace-setup-result__summary workspace-setup-result__summary--sub">
            {{ resultOrgSummary }}
          </p>
        </div>

        <div
          v-if="resultWarning"
          class="workspace-setup-callout workspace-setup-callout--neutral"
          role="status"
        >
          {{ resultWarning }}
        </div>

        <div v-if="resultInviteUrl" class="workspace-setup-result__card">
          <label class="workspace-setup-field__label">{{ t('tenant.setupWizard.inviteLinkLabel') }}</label>
          <div class="workspace-setup-result__link-row">
            <t-input :value="resultInviteUrl" readonly />
            <t-button theme="primary" variant="outline" @click="copyInviteLink">
              {{ t('tenant.setupWizard.copyInviteLink') }}
            </t-button>
          </div>
        </div>
      </div>

      <div class="workspace-setup-footer">
        <div class="workspace-setup-footer__left">
          <t-button v-if="step === 1" variant="outline" @click="step = 0">
            {{ t('common.back') }}
          </t-button>
        </div>
        <div class="workspace-setup-footer__right">
          <t-button v-if="step === 0" theme="primary" @click="goNext">
            {{ t('common.next') }}
          </t-button>
          <t-button v-else-if="step === 1" theme="primary" :loading="submitting" @click="runSetup">
            {{ t('tenant.setupWizard.finish') }}
          </t-button>
          <t-button v-else theme="primary" @click="enterWorkspace">
            {{ t('tenant.setupWizard.enter') }}
          </t-button>
        </div>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'

import {
  assignUserToOrg,
  createOrgTreeNode,
  createUserInOrg,
} from '@/api/org-tree'
import { createInviteLink } from '@/api/tenant/invitations'
import {
  addMember,
  listMembers,
  updateMemberRole,
  type TenantMember,
  type TenantRole,
} from '@/api/tenant/members'
import { useRoleLabel } from '@/composables/useRoleLabel'
import { useAuthStore } from '@/stores/auth'
import { useWorkspaceSetupStore } from '@/stores/workspaceSetup'
import { navigateAfterTenantSwitch, stashTenantSwitchToast } from '@/utils/tenantSwitch'

type PersonMode = 'existing' | 'new' | 'invite'
type OrgRole = 'admin' | 'editor' | 'viewer'

interface CreatedRootOrg {
  id: string
  name: string
}

interface SetupResult {
  summary: string
  orgSummary: string
  warning?: string
  inviteUrl?: string
}

const { t } = useI18n()
const { formatRole } = useRoleLabel()
const authStore = useAuthStore()
const workspaceSetupStore = useWorkspaceSetupStore()

const step = ref(0)
const submitting = ref(false)
const createdRootOrg = ref<CreatedRootOrg | null>(null)
const resolvedResponsibleUserId = ref<string>('')
const result = ref<SetupResult | null>(null)

const form = reactive({
  personMode: 'existing' as PersonMode,
  tenantRole: 'admin' as TenantRole,
  existingEmail: '',
  inviteMessage: '',
  newUsername: '',
  newEmail: '',
  newPhone: '',
  newPassword: '',
  shouldSetupOrg: true,
  rootOrgName: '',
  rootOrgDescription: '',
  orgRole: 'admin' as OrgRole,
})

const pending = computed(() => workspaceSetupStore.pending)
const pendingTenantName = computed(() => pending.value?.tenantName || '')
const visible = computed(() => {
  return !!pending.value && authStore.isLoggedIn && authStore.effectiveTenantId === pending.value.tenantId
})

const isExistingMode = computed(() => form.personMode === 'existing')
const isNewMode = computed(() => form.personMode === 'new')
const isInviteMode = computed(() => form.personMode === 'invite')
const isOrgSetupLocked = computed(() => isNewMode.value)

const stepTitles = computed(() => [
  t('tenant.setupWizard.steps.person'),
  t('tenant.setupWizard.steps.organization'),
  t('tenant.setupWizard.steps.done'),
])

const modeOptions = computed(() => [
  {
    value: 'existing' as PersonMode,
    icon: 'user',
    title: t('tenant.setupWizard.modes.existing.title'),
    desc: t('tenant.setupWizard.modes.existing.desc'),
  },
  {
    value: 'new' as PersonMode,
    icon: 'user-add',
    title: t('tenant.setupWizard.modes.new.title'),
    desc: t('tenant.setupWizard.modes.new.desc'),
  },
  {
    value: 'invite' as PersonMode,
    icon: 'link',
    title: t('tenant.setupWizard.modes.invite.title'),
    desc: t('tenant.setupWizard.modes.invite.desc'),
  },
])

const tenantRoleOptions = computed(() => [
  { value: 'viewer' as TenantRole, label: t('tenantMember.role.viewer') },
  { value: 'contributor' as TenantRole, label: t('tenantMember.role.contributor') },
  { value: 'admin' as TenantRole, label: t('tenantMember.role.admin') },
  { value: 'owner' as TenantRole, label: t('tenantMember.role.owner') },
])

const orgRoleOptions = computed(() => [
  { value: 'viewer' as OrgRole, label: t('admin.member.roleViewer') },
  { value: 'editor' as OrgRole, label: t('admin.member.roleEditor') },
  { value: 'admin' as OrgRole, label: t('admin.member.roleAdmin') },
])

const currentModeTitle = computed(() => {
  if (isExistingMode.value) return t('tenant.setupWizard.modes.existing.title')
  if (isNewMode.value) return t('tenant.setupWizard.modes.new.title')
  return t('tenant.setupWizard.modes.invite.title')
})

const currentModeDesc = computed(() => {
  if (isExistingMode.value) return t('tenant.setupWizard.modes.existing.desc')
  if (isNewMode.value) return t('tenant.setupWizard.modes.new.desc')
  return t('tenant.setupWizard.modes.invite.desc')
})

const resultSummary = computed(() => result.value?.summary || '')
const resultOrgSummary = computed(() => result.value?.orgSummary || '')
const resultWarning = computed(() => result.value?.warning || '')
const resultInviteUrl = computed(() => result.value?.inviteUrl || '')

function resetState() {
  step.value = 0
  submitting.value = false
  createdRootOrg.value = null
  resolvedResponsibleUserId.value = ''
  result.value = null
  form.personMode = 'existing'
  form.tenantRole = 'admin'
  form.existingEmail = ''
  form.inviteMessage = ''
  form.newUsername = ''
  form.newEmail = ''
  form.newPhone = ''
  form.newPassword = ''
  form.shouldSetupOrg = true
  form.rootOrgName = pending.value?.tenantName || ''
  form.rootOrgDescription = pending.value?.tenantDescription || ''
  form.orgRole = 'admin'
}

watch(
  () => pending.value?.tenantId,
  () => {
    if (pending.value) resetState()
  },
  { immediate: true },
)

watch(
  () => form.personMode,
  (mode) => {
    resolvedResponsibleUserId.value = ''
    result.value = null
    if (mode === 'new') {
      form.shouldSetupOrg = true
      form.orgRole = 'admin'
    }
  },
)

watch(
  () => form.existingEmail,
  () => {
    resolvedResponsibleUserId.value = ''
  },
)

function absoluteInviteURL(raw: string): string {
  if (!raw) return ''
  if (/^https?:\/\//i.test(raw)) return raw
  const origin = (typeof window !== 'undefined' && window.location && window.location.origin) || ''
  return raw.startsWith('/') ? origin + raw : origin + '/' + raw
}

async function copyInviteLink() {
  if (!resultInviteUrl.value) return
  try {
    await navigator.clipboard.writeText(resultInviteUrl.value)
    MessagePlugin.success(t('tenantInvitation.copied'))
  } catch {
    MessagePlugin.error(t('tenantInvitation.copyFailed'))
  }
}

function validateStepOne(): boolean {
  if (isExistingMode.value) {
    if (!form.existingEmail.trim()) {
      MessagePlugin.warning(t('tenant.setupWizard.errors.existingEmailRequired'))
      return false
    }
    return true
  }

  if (isNewMode.value) {
    if (form.newUsername.trim().length === 0) {
      MessagePlugin.warning(t('auth.usernameRequired'))
      return false
    }
    if (form.newUsername.trim().length < 2) {
      MessagePlugin.warning(t('auth.usernameMinLength'))
      return false
    }
    if (form.newPassword.length === 0) {
      MessagePlugin.warning(t('auth.passwordRequired'))
      return false
    }
    if (form.newPassword.length < 8) {
      MessagePlugin.warning(t('auth.passwordMinLength'))
      return false
    }
    if (form.newPassword.length > 32) {
      MessagePlugin.warning(t('auth.passwordMaxLength'))
      return false
    }
    if (!form.newEmail.trim() && !form.newPhone.trim()) {
      MessagePlugin.warning(t('admin.member.emailOrPhoneRequired'))
      return false
    }
  }

  return true
}

function validateStepTwo(): boolean {
  if (isNewMode.value && !form.shouldSetupOrg) {
    MessagePlugin.warning(t('tenant.setupWizard.rootOrgRequiredHint'))
    return false
  }
  if (form.shouldSetupOrg && !createdRootOrg.value && !form.rootOrgName.trim()) {
    MessagePlugin.warning(t('tenant.setupWizard.errors.rootOrgNameRequired'))
    return false
  }
  return true
}

function goNext() {
  if (!validateStepOne()) return
  step.value = 1
}

async function ensureRootOrg(tenantId: number): Promise<CreatedRootOrg | null> {
  if (!form.shouldSetupOrg) return null
  if (createdRootOrg.value) return createdRootOrg.value

  const resp = await createOrgTreeNode({
    name: form.rootOrgName.trim(),
    description: form.rootOrgDescription.trim() || undefined,
  })
  if (!resp.success || !resp.data) {
    throw new Error(resp.message || t('tenant.setupWizard.errors.generic'))
  }
  createdRootOrg.value = {
    id: resp.data.id,
    name: resp.data.name,
  }
  return createdRootOrg.value
}

async function findTenantMemberByEmail(tenantId: number, email: string): Promise<TenantMember | null> {
  const resp = await listMembers(tenantId, {
    q: email,
    page: 1,
    page_size: 20,
  })
  if (!resp.success || !resp.data) return null
  const target = email.trim().toLowerCase()
  return resp.data.members.find((member) => member.email.trim().toLowerCase() === target) || null
}

async function ensureExistingMember(tenantId: number): Promise<string> {
  if (resolvedResponsibleUserId.value) return resolvedResponsibleUserId.value

  const email = form.existingEmail.trim()
  try {
    const resp = await addMember(tenantId, { email, role: form.tenantRole })
    if (!resp.success || !resp.data) {
      throw new Error(resp.message || t('tenant.setupWizard.errors.generic'))
    }
    resolvedResponsibleUserId.value = resp.data.user_id
    return resolvedResponsibleUserId.value
  } catch (err: any) {
    if (err?.status === 404) {
      throw new Error(t('tenant.setupWizard.errors.existingUserNotFound'))
    }
    if (err?.status === 409) {
      const member = await findTenantMemberByEmail(tenantId, email)
      if (!member) {
        throw new Error(err?.message || t('tenant.setupWizard.errors.generic'))
      }
      if (member.role !== form.tenantRole) {
        const updateResp = await updateMemberRole(tenantId, member.user_id, form.tenantRole)
        if (!updateResp.success) {
          throw new Error(updateResp.message || t('tenant.setupWizard.errors.generic'))
        }
      }
      resolvedResponsibleUserId.value = member.user_id
      return resolvedResponsibleUserId.value
    }
    throw new Error(err?.message || t('tenant.setupWizard.errors.generic'))
  }
}

async function runSetup() {
  if (!pending.value) return
  if (!validateStepTwo()) return

  submitting.value = true
  try {
    const tenantId = pending.value.tenantId
    const rootOrg = await ensureRootOrg(tenantId)

    if (isExistingMode.value) {
      const userId = await ensureExistingMember(tenantId)
      if (rootOrg) {
        const assignResp = await assignUserToOrg(rootOrg.id, {
          user_id: userId,
          role: form.orgRole,
        })
        if (!assignResp.success) {
          throw new Error(assignResp.message || t('tenant.setupWizard.errors.generic'))
        }
      }
      result.value = {
        summary: rootOrg
          ? t('tenant.setupWizard.resultExistingWithOrg', {
              email: form.existingEmail.trim(),
              org: rootOrg.name,
            })
          : t('tenant.setupWizard.resultExisting', {
              email: form.existingEmail.trim(),
            }),
        orgSummary: rootOrg
          ? t('tenant.setupWizard.resultOrgCreated', { name: rootOrg.name })
          : t('tenant.setupWizard.resultOrgSkipped'),
      }
    } else if (isNewMode.value) {
      if (!rootOrg) {
        throw new Error(t('tenant.setupWizard.rootOrgRequiredHint'))
      }
      const resp = await createUserInOrg(rootOrg.id, {
        username: form.newUsername.trim(),
        email: form.newEmail.trim() || undefined,
        phone: form.newPhone.trim() || undefined,
        password: form.newPassword,
        role: form.orgRole,
        tenant_role: form.tenantRole,
      })
      if (!resp.success) {
        throw new Error(resp.message || t('tenant.setupWizard.errors.generic'))
      }
      result.value = {
        summary: t('tenant.setupWizard.resultNewUserWithOrg', {
          username: form.newUsername.trim(),
          org: rootOrg.name,
        }),
        orgSummary: t('tenant.setupWizard.resultOrgCreated', { name: rootOrg.name }),
        warning: resp.message?.includes('failed to assign to organization')
          ? t('tenant.setupWizard.warnings.newUserPartial', { message: resp.message })
          : undefined,
      }
    } else {
      const resp = await createInviteLink(tenantId, {
        role: form.tenantRole,
        message: form.inviteMessage.trim() || undefined,
      })
      const inviteUrl = absoluteInviteURL(resp.data?.invite_url || '')
      if (!resp.success || !resp.data || !inviteUrl) {
        throw new Error(resp.message || t('tenant.setupWizard.errors.missingInviteLink'))
      }
      result.value = {
        summary: t('tenant.setupWizard.resultInvite'),
        orgSummary: rootOrg
          ? t('tenant.setupWizard.resultOrgCreated', { name: rootOrg.name })
          : t('tenant.setupWizard.resultOrgSkipped'),
        inviteUrl,
      }
      try {
        await navigator.clipboard.writeText(inviteUrl)
        MessagePlugin.success(t('tenantInvitation.copied'))
      } catch {
        // ignore clipboard failures here; the result step still exposes a copy button.
      }
    }

    step.value = 2
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('tenant.setupWizard.errors.generic'))
  } finally {
    submitting.value = false
  }
}

function navigateIntoWorkspace() {
  if (!pending.value) return
  const currentRole = authStore.currentTenantRole || 'owner'
  stashTenantSwitchToast({
    name: pending.value.tenantName,
    role: formatRole(currentRole),
    roleEnum: currentRole,
  })
  workspaceSetupStore.clear()
  navigateAfterTenantSwitch()
}

function enterWorkspace() {
  navigateIntoWorkspace()
}
</script>

<style scoped lang="less">
.workspace-setup {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* —— Header：与 logo 一起保持左对齐，贴近站点主面板/设置面板的标题样式 —— */
.workspace-setup-header {
  display: flex;
  flex-direction: row;
  align-items: center;
  gap: 12px;
  padding: 4px 4px 16px;
  border-bottom: 1px solid var(--td-component-stroke);
}

.workspace-setup-header__avatar {
  flex-shrink: 0;
  width: 36px;
  height: 36px;
  border-radius: 10px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: color-mix(in srgb, var(--td-brand-color) 12%, transparent);
  color: var(--td-brand-color);
}

.workspace-setup-header__body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.workspace-setup-header__row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.workspace-setup-header__title {
  font-size: 16px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  letter-spacing: -0.01em;
}

.workspace-setup-header__desc {
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
  color: var(--td-text-color-secondary);
  text-align: left;
}

/* —— 步骤条：用胶囊连接线代替纯色指示，柔和而非刺眼的纯绿 —— */
.workspace-setup-steps {
  display: flex;
  align-items: center;
  gap: 0;
  padding: 4px 4px 14px;
}

.workspace-setup-step {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--td-text-color-placeholder);
  font-size: 13px;
}

.workspace-setup-step__num {
  flex-shrink: 0;
  width: 26px;
  height: 26px;
  border-radius: 999px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: color-mix(in srgb, var(--td-text-color-primary) 6%, transparent);
  color: var(--td-text-color-placeholder);
  font-size: 13px;
  font-weight: 500;
  border: 1px solid transparent;
  transition: background 0.18s ease, color 0.18s ease, border-color 0.18s ease;
}

.workspace-setup-step.is-active {
  color: var(--td-text-color-primary);
  font-weight: 500;
}

.workspace-setup-step.is-active .workspace-setup-step__num {
  background: color-mix(in srgb, var(--td-brand-color) 14%, transparent);
  border-color: color-mix(in srgb, var(--td-brand-color) 36%, transparent);
  color: var(--td-brand-color);
}

.workspace-setup-step.is-done {
  color: var(--td-text-color-secondary);
}

.workspace-setup-step.is-done .workspace-setup-step__num {
  background: color-mix(in srgb, var(--td-brand-color) 18%, transparent);
  border-color: color-mix(in srgb, var(--td-brand-color) 40%, transparent);
  color: var(--td-brand-color);
}

/* 步骤之间的连接线 */
.workspace-setup-step::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--td-component-stroke);
  margin: 0 10px;
  transition: background 0.18s ease;
}

.workspace-setup-step:last-child::after {
  display: none;
}

.workspace-setup-step.is-done::after {
  background: color-mix(in srgb, var(--td-brand-color) 40%, var(--td-component-stroke));
}

.workspace-setup-step__label {
  white-space: nowrap;
}

/* —— 正文分区 —— */
.workspace-setup-body,
.workspace-setup-result {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.workspace-setup-section {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 16px 18px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 12px;
  background: var(--td-bg-color-container);
  transition: border-color 0.18s ease, box-shadow 0.18s ease;
}

/* info 段用作软提示条；保持与品牌色系的统一，避免引入主题绿/黄 */
.workspace-setup-section--info {
  background: color-mix(in srgb, var(--td-brand-color) 6%, transparent);
  border-color: color-mix(in srgb, var(--td-brand-color) 24%, var(--td-component-stroke));
  padding: 10px 14px;
}

.workspace-setup-section__copy {
  margin: 0;
  font-size: 13px;
  line-height: 1.6;
  color: color-mix(in srgb, var(--td-text-color-primary) 86%, transparent);
  display: flex;
  align-items: flex-start;
  gap: 8px;
}

.workspace-setup-section__copy::before {
  content: '';
  display: inline-block;
  flex-shrink: 0;
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background: var(--td-brand-color);
  margin-top: 8px;
}

.workspace-setup-section__head h4 {
  margin: 0 0 4px;
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.workspace-setup-section__head p {
  margin: 0;
  font-size: 12.5px;
  line-height: 1.55;
  color: var(--td-text-color-secondary);
}

/* —— Callout：替换 TDesign t-alert；只用品牌色与中性色 —— */
.workspace-setup-callout {
  margin: 0;
  padding: 10px 14px;
  border-radius: 10px;
  font-size: 13px;
  line-height: 1.55;
  border: 1px solid transparent;
}

.workspace-setup-callout--success {
  background: color-mix(in srgb, var(--td-brand-color) 8%, transparent);
  border-color: color-mix(in srgb, var(--td-brand-color) 22%, var(--td-component-stroke));
  color: color-mix(in srgb, var(--td-text-color-primary) 88%, transparent);
}

.workspace-setup-callout--neutral {
  background: color-mix(in srgb, var(--td-text-color-primary) 4%, transparent);
  border-color: var(--td-component-stroke);
  color: var(--td-text-color-secondary);
}

/* —— 模式选择：三张卡片样式更接近设置区 rows —— */
.workspace-setup-mode-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.workspace-setup-mode {
  border: 1px solid var(--td-component-stroke);
  border-radius: 10px;
  background: var(--td-bg-color-container);
  padding: 14px;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
  text-align: left;
  color: var(--td-text-color-primary);
  cursor: pointer;
  transition: border-color 0.18s ease, background 0.18s ease, transform 0.18s ease, box-shadow 0.18s ease;
}

.workspace-setup-mode:hover {
  border-color: color-mix(in srgb, var(--td-brand-color) 32%, var(--td-component-stroke));
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.04);
}

.workspace-setup-mode.is-active {
  border-color: var(--td-brand-color);
  background: color-mix(in srgb, var(--td-brand-color) 6%, var(--td-bg-color-container));
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--td-brand-color) 14%, transparent);
}

.workspace-setup-mode__icon {
  color: var(--td-brand-color);
  width: 28px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: color-mix(in srgb, var(--td-brand-color) 10%, transparent);
}

.workspace-setup-mode__title {
  font-size: 14px;
  font-weight: 600;
}

.workspace-setup-mode__desc {
  font-size: 12px;
  line-height: 1.5;
  color: var(--td-text-color-secondary);
}

.workspace-setup-radio-group {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
}

/* TDesign radio 在品牌色调上保持一致 */
.workspace-setup-radio-group :deep(.t-radio) {
  font-size: 13px;
}

.workspace-setup-form-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
}

.workspace-setup-form-grid--two-col {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.workspace-setup-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.workspace-setup-field label,
.workspace-setup-field__label {
  font-size: 13px;
  font-weight: 500;
  color: var(--td-text-color-primary);
}

.workspace-setup-field__hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--td-text-color-secondary);
}

.workspace-setup-checkbox-row {
  display: flex;
  align-items: center;
}

/* —— 结果视图 —— */
.workspace-setup-result__hero {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 18px 20px;
  border-radius: 12px;
  background: color-mix(in srgb, var(--td-brand-color) 7%, var(--td-bg-color-container));
  border: 1px solid color-mix(in srgb, var(--td-brand-color) 22%, var(--td-component-stroke));
}

.workspace-setup-result__hero-content {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.workspace-setup-result__hero h4 {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.workspace-setup-result__hero p {
  margin: 0;
  font-size: 13px;
  line-height: 1.55;
  color: var(--td-text-color-secondary);
}

.workspace-setup-result__icon {
  flex-shrink: 0;
  width: 40px;
  height: 40px;
  border-radius: 12px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: color-mix(in srgb, var(--td-brand-color) 18%, transparent);
  color: var(--td-brand-color);
}

.workspace-setup-result__card {
  padding: 14px 16px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 12px;
  background: var(--td-bg-color-container);
}

.workspace-setup-result__summary {
  margin: 0;
  font-size: 14px;
  line-height: 1.6;
  color: var(--td-text-color-primary);
}

.workspace-setup-result__summary--sub {
  margin-top: 8px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
}

.workspace-setup-result__link-row {
  display: flex;
  gap: 10px;
  align-items: center;
}

/* —— 底部按钮 —— */
.workspace-setup-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding-top: 14px;
  border-top: 1px solid var(--td-component-stroke);
}

.workspace-setup-footer__left,
.workspace-setup-footer__right {
  display: flex;
  align-items: center;
  gap: 8px;
}

@media (max-width: 900px) {
  .workspace-setup-mode-grid,
  .workspace-setup-form-grid--two-col {
    grid-template-columns: minmax(0, 1fr);
  }

  .workspace-setup-result__link-row,
  .workspace-setup-footer {
    flex-direction: column;
    align-items: stretch;
  }

  .workspace-setup-footer__left,
  .workspace-setup-footer__right {
    width: 100%;
    justify-content: flex-end;
  }
}
</style>
