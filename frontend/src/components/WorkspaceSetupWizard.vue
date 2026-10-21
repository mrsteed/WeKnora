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
        <section class="workspace-setup-section">
          <div class="workspace-setup-section__head">
            <div class="workspace-setup-section__title-row">
              <span class="workspace-setup-section__title-icon workspace-setup-section__title-icon--person">
                <t-icon name="user" size="15px" />
              </span>
              <h4>{{ currentModeTitle }}</h4>
            </div>
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
              <label>{{ t('auth.username') }}</label>
              <t-input v-model="form.newUsername" :placeholder="t('tenant.setupWizard.newUserUsernamePlaceholder')" clearable />
            </div>
            <div class="workspace-setup-field">
              <label>{{ t('auth.password') }}</label>
              <t-input v-model="form.newPassword" type="password" :placeholder="t('tenant.setupWizard.newUserPasswordPlaceholder')" clearable />
            </div>
            <div class="workspace-setup-field">
              <label>{{ t('auth.email') }}</label>
              <t-input v-model="form.newEmail" :placeholder="t('tenant.setupWizard.newUserEmailPlaceholder')" clearable />
            </div>
            <div class="workspace-setup-field">
              <label>{{ t('tenant.setupWizard.newUserPhoneLabel') }}</label>
              <t-input v-model="form.newPhone" :placeholder="t('tenant.setupWizard.newUserPhonePlaceholder')" clearable />
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
            <div class="workspace-setup-section__title-row">
              <span class="workspace-setup-section__title-icon">
                <t-icon name="view-module" size="15px" />
              </span>
              <h4>{{ t('tenant.setupWizard.rootOrgTitle') }}</h4>
            </div>
          </div>

          <div v-if="createdRootOrg" class="workspace-setup-inline-note workspace-setup-inline-note--success">
            <t-icon name="check-circle" size="14px" />
            <span>{{ t('tenant.setupWizard.retryReuseRootHint', { name: createdRootOrg.name }) }}</span>
          </div>

          <div class="workspace-setup-checkbox-row">
            <t-checkbox v-model="form.shouldSetupOrg" :disabled="isOrgSetupLocked">
              {{ t('tenant.setupWizard.createRootToggle') }}
            </t-checkbox>
          </div>

          <div v-if="form.shouldSetupOrg" class="workspace-setup-form-grid">
            <div class="workspace-setup-field">
              <label>{{ t('tenant.setupWizard.rootOrgNameLabel') }}</label>
              <t-input
                v-model="form.rootOrgName"
                :disabled="!!createdRootOrg"
                :placeholder="t('tenant.setupWizard.rootOrgNamePlaceholder', { name: pendingTenantName })"
                clearable
              />
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
          <p class="workspace-setup-result__summary workspace-setup-result__summary--sub">{{ resultOrgSummary }}</p>
        </div>

        <div v-if="resultWarning" class="workspace-setup-callout workspace-setup-callout--neutral" role="status">
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
          <template v-else>
            <t-button v-if="result?.retryable" variant="outline" @click="enterWorkspace">
              {{ t('tenant.setupWizard.enter') }}
            </t-button>
            <t-button v-if="result?.retryable" theme="primary" :loading="submitting" @click="retrySetupFromResult">
              {{ t('tenant.setupWizard.retrySetup') }}
            </t-button>
            <t-button v-else theme="primary" @click="enterWorkspace">
              {{ t('tenant.setupWizard.enter') }}
            </t-button>
          </template>
        </div>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'

import { getOrgTree } from '@/api/org-tree'
import { bootstrapWorkspace, type WorkspaceBootstrapMode, type WorkspaceBootstrapResult } from '@/api/tenant'
import type { TenantRole } from '@/api/tenant/members'
import { useRoleLabel } from '@/composables/useRoleLabel'
import { useAuthStore } from '@/stores/auth'
import { useWorkspaceSetupStore } from '@/stores/workspaceSetup'
import { navigateAfterTenantSwitch, stashTenantSwitchToast } from '@/utils/tenantSwitch'
import { getErrorReason, resolveErrorMessage } from '@/utils/errorMessage'
import { selectReusableRootOrg } from '@/utils/workspaceSetupRootOrg'

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
  retryable?: boolean
}

const { t } = useI18n()
const { formatRole } = useRoleLabel()
const authStore = useAuthStore()
const workspaceSetupStore = useWorkspaceSetupStore()

const step = ref(0)
const submitting = ref(false)
const createdRootOrg = ref<CreatedRootOrg | null>(null)
const resolvedResponsibleUserId = ref('')
const result = ref<SetupResult | null>(null)

const form = reactive({
  personMode: 'new' as PersonMode,
  tenantRole: 'owner' as TenantRole,
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
  return (
    !!pending.value
    && authStore.isLoggedIn
    && authStore.effectiveTenantId === pending.value.tenantId
  )
})

const tenantRoleOptions = computed(() => [
  { value: 'owner', label: formatRole('owner') },
  { value: 'admin', label: formatRole('admin') },
  { value: 'contributor', label: formatRole('contributor') },
  { value: 'viewer', label: formatRole('viewer') },
])
const modeOptions = computed(() => [
  {
    value: 'existing' as const,
    icon: 'user-visible',
    title: t('tenant.setupWizard.modes.existing.title'),
    desc: t('tenant.setupWizard.modes.existing.desc'),
  },
  {
    value: 'new' as const,
    icon: 'user-add',
    title: t('tenant.setupWizard.modes.new.title'),
    desc: t('tenant.setupWizard.modes.new.desc'),
  },
  {
    value: 'invite' as const,
    icon: 'send',
    title: t('tenant.setupWizard.modes.invite.title'),
    desc: t('tenant.setupWizard.modes.invite.desc'),
  },
])

const stepTitles = computed(() => [
  t('tenant.setupWizard.steps.person'),
  t('tenant.setupWizard.steps.organization'),
  t('tenant.setupWizard.steps.done'),
])

const isExistingMode = computed(() => form.personMode === 'existing')
const isNewMode = computed(() => form.personMode === 'new')
const isInviteMode = computed(() => form.personMode === 'invite')

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

const isOrgSetupLocked = computed(() => !!createdRootOrg.value || isInviteMode.value)
const resultSummary = computed(() => result.value?.summary || '')
const resultOrgSummary = computed(() => result.value?.orgSummary || '')
const resultWarning = computed(() => result.value?.warning || '')
const resultInviteUrl = computed(() => result.value?.inviteUrl || '')

const resetWizard = () => {
  step.value = 0
  submitting.value = false
  createdRootOrg.value = pending.value?.createdRootOrg || null
  resolvedResponsibleUserId.value = ''
  result.value = null
  form.personMode = 'new'
  form.tenantRole = 'owner'
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

watch(visible, (open) => {
  if (open) {
    resetWizard()
  }
})

watch(
  [() => pending.value?.tenantId, visible, () => form.shouldSetupOrg],
  ([tenantId, isVisible, shouldSetupOrg]) => {
    if (!tenantId || !isVisible || !shouldSetupOrg || createdRootOrg.value || isInviteMode.value) return
    void probeExistingRootOrg()
  },
  { immediate: true },
)

watch(
  () => form.personMode,
  (mode) => {
    resolvedResponsibleUserId.value = ''
    result.value = null
    workspaceSetupStore.rotateIdempotencyKey()
    if (mode === 'new') {
      form.shouldSetupOrg = true
      form.orgRole = 'admin'
    }
  },
)

const validateStepOne = (): boolean => {
  if (isExistingMode.value && !form.existingEmail.trim()) {
    MessagePlugin.warning(t('tenant.setupWizard.errors.existingEmailRequired'))
    return false
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
      MessagePlugin.warning(t('tenant.setupWizard.errors.emailOrPhoneRequired'))
      return false
    }
  }

  return true
}

const validateStepTwo = (): boolean => {
  if (form.shouldSetupOrg && !createdRootOrg.value && !form.rootOrgName.trim()) {
    MessagePlugin.warning(t('tenant.setupWizard.errors.rootOrgNameRequired'))
    return false
  }
  return true
}

const probeExistingRootOrg = async (): Promise<CreatedRootOrg | null> => {
  if (!form.shouldSetupOrg || createdRootOrg.value || isInviteMode.value) return createdRootOrg.value
  try {
    const treeResp = await getOrgTree()
    const tree = Array.isArray(treeResp?.data) ? treeResp.data : []
    const reusable = selectReusableRootOrg(tree, [form.rootOrgName.trim(), pendingTenantName.value])
    if (reusable) {
      createdRootOrg.value = reusable
      form.rootOrgName = reusable.name
      workspaceSetupStore.setCreatedRootOrg(reusable)
      return reusable
    }
  } catch {
    // ignore prefetch failures; backend remains source of truth on submit
  }
  return null
}

const goNext = async () => {
  if (!validateStepOne()) return

  if (form.shouldSetupOrg && !createdRootOrg.value && !isInviteMode.value) {
    await probeExistingRootOrg()
  }

  step.value = 1
}

const resolveBootstrapErrorMessage = (err: any) => {
  const reason = getErrorReason(err)
  if (reason === 'idempotency_key_reused') {
    workspaceSetupStore.rotateIdempotencyKey()
  }
  if (reason === 'existing_email') {
    return t('tenant.setupWizard.errors.existingUserNotFound')
  }
  return resolveErrorMessage(err, t, 'tenant.setupWizard.errors.generic')
}

const absoluteInviteURL = (raw: string): string => {
  if (!raw) return ''
  if (/^https?:\/\//i.test(raw)) return raw
  const origin =
    (typeof window !== 'undefined' && window.location && window.location.origin) ||
    ''
  return raw.startsWith('/') ? origin + raw : origin + '/' + raw
}

const runSetup = async () => {
  if (submitting.value || !pending.value) return
  if (!validateStepTwo()) return

  const tenantId = pending.value.tenantId
  const mode = form.personMode as WorkspaceBootstrapMode
  submitting.value = true
  try {
    const resp = await bootstrapWorkspace(tenantId, {
      idempotency_key: pending.value.idempotencyKey,
      mode,
      tenant_role: form.tenantRole,
      should_setup_org: form.shouldSetupOrg,
      root_org_id: createdRootOrg.value?.id || pending.value.createdRootOrg?.id,
      root_org_name: form.rootOrgName.trim() || pending.value.createdRootOrg?.name,
      root_org_description: form.rootOrgDescription.trim() || undefined,
      org_role: form.shouldSetupOrg ? form.orgRole : undefined,
      existing_email: isExistingMode.value ? form.existingEmail.trim() : undefined,
      invite_message: isInviteMode.value ? form.inviteMessage.trim() : undefined,
      new_username: isNewMode.value ? form.newUsername.trim() : undefined,
      new_email: isNewMode.value ? form.newEmail.trim() || undefined : undefined,
      new_phone: isNewMode.value ? form.newPhone.trim() || undefined : undefined,
      new_password: isNewMode.value ? form.newPassword : undefined,
    })

    if (!resp.success || !resp.data) {
      throw resp
    }

    const data = resp.data
    const resultRootOrg = data.root_org
      ? { id: data.root_org.id, name: data.root_org.name }
      : (createdRootOrg.value || pending.value.createdRootOrg || null)
    createdRootOrg.value = resultRootOrg
    workspaceSetupStore.setCreatedRootOrg(resultRootOrg)
    if (resultRootOrg) {
      form.rootOrgName = resultRootOrg.name
    }
    resolvedResponsibleUserId.value = data.user?.id || resolvedResponsibleUserId.value
    result.value = buildResultSummary({
      ...data,
      invite_url: data.invite_url ? absoluteInviteURL(data.invite_url) : data.invite_url,
    })
    if (data.mode === 'invite' && result.value?.inviteUrl) {
      try {
        await navigator.clipboard.writeText(result.value.inviteUrl)
        MessagePlugin.success(t('tenantInvitation.copied'))
      } catch {
        // ignore clipboard failures here; the result step still exposes a copy button.
      }
    }
    step.value = 2
  } catch (err: any) {
    MessagePlugin.error(resolveBootstrapErrorMessage(err))
  } finally {
    submitting.value = false
  }
}

const buildResultSummary = (data: WorkspaceBootstrapResult): SetupResult => {
  if (data.mode === 'existing') {
    const existingName = data.user?.email || data.user?.username || form.existingEmail.trim() || pendingTenantName.value
    return {
      summary: data.root_org
        ? t('tenant.setupWizard.resultExistingWithOrg', { email: existingName, org: data.root_org.name })
        : t('tenant.setupWizard.resultExisting', { email: existingName }),
      orgSummary: data.root_org
        ? data.root_org.reused
          ? t('tenant.setupWizard.resultOrgReused', { name: data.root_org.name })
          : t('tenant.setupWizard.resultOrgCreated', { name: data.root_org.name })
        : t('tenant.setupWizard.resultOrgSkipped'),
      warning: data.status === 'partial_success' && data.user
        ? t('tenant.setupWizard.warnings.existingUserPartial', { name: data.user.username })
        : undefined,
      retryable: data.status === 'partial_success',
    }
  }
  if (data.mode === 'new') {
    if (!data.root_org) {
      throw new Error(t('tenant.setupWizard.rootOrgRequiredHint'))
    }
    return {
      summary: t('tenant.setupWizard.resultNewUserWithOrg', {
        username: data.user?.username || form.newUsername.trim() || pendingTenantName.value,
        org: data.root_org.name,
      }),
      orgSummary: data.root_org.reused
        ? t('tenant.setupWizard.resultOrgReused', { name: data.root_org.name })
        : t('tenant.setupWizard.resultOrgCreated', { name: data.root_org.name }),
      warning: data.status === 'partial_success' && data.user
        ? t('tenant.setupWizard.warnings.newUserPartial', { name: data.user.username })
        : undefined,
      retryable: data.status === 'partial_success',
    }
  }
  if (!data.invite_url) {
    throw new Error(t('tenant.setupWizard.errors.missingInviteLink'))
  }
  return {
    summary: t('tenant.setupWizard.resultInvite'),
    orgSummary: data.root_org
      ? data.root_org.reused
        ? t('tenant.setupWizard.resultOrgReused', { name: data.root_org.name })
        : t('tenant.setupWizard.resultOrgCreated', { name: data.root_org.name })
      : t('tenant.setupWizard.resultOrgSkipped'),
    inviteUrl: data.invite_url,
    retryable: false,
  }
}

const retrySetupFromResult = async () => {
  step.value = 1
  await runSetup()
}

const enterWorkspace = () => {
  workspaceSetupStore.clear()
  stashTenantSwitchToast({
    name: pendingTenantName.value,
    role: formatRole(authStore.currentTenantRole || form.tenantRole) || undefined,
    roleEnum: authStore.currentTenantRole || form.tenantRole,
  })
  navigateAfterTenantSwitch()
}

const copyInviteLink = async () => {
  if (!resultInviteUrl.value) return
  try {
    await navigator.clipboard.writeText(resultInviteUrl.value)
    MessagePlugin.success(t('tenant.setupWizard.copyInviteLinkSuccess'))
  } catch {
    MessagePlugin.error(t('tenant.setupWizard.copyInviteLinkFailed'))
  }
}
</script>

<style scoped lang="less">
.workspace-setup-header {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}

.workspace-setup-header__avatar {
  flex-shrink: 0;
  width: 36px;
  height: 36px;
  border-radius: 12px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: color-mix(in srgb, var(--td-brand-color) 15%, transparent);
  color: var(--td-brand-color);
}

.workspace-setup-header__title {
  font-size: 18px;
  font-weight: 600;
}

.workspace-setup-header__desc {
  margin: 4px 0 0;
  color: var(--td-text-color-secondary);
}

.workspace-setup { display: flex; flex-direction: column; gap: 20px; }
.workspace-setup-steps { display: flex; gap: 16px; flex-wrap: wrap; align-items: center; }
.workspace-setup-step {
  display: inline-flex; align-items: center; gap: 8px;
  color: var(--td-text-color-placeholder);
  font-size: 13px; font-weight: 500;
}
.workspace-setup-step.is-active, .workspace-setup-step.is-done {
  color: var(--td-text-color-primary);
  font-weight: 600;
}
.workspace-setup-step__num {
  width: 26px; height: 26px;
  border-radius: 999px;
  display: inline-flex; align-items: center; justify-content: center;
  background: var(--td-bg-color-secondarycontainer);
  border: 1.5px solid var(--td-component-border);
  font-size: 12px; font-weight: 600;
  transition: background 0.2s, border-color 0.2s;
}
.workspace-setup-step.is-done .workspace-setup-step__num {
  background: color-mix(in srgb, var(--td-brand-color) 12%, transparent);
  border-color: color-mix(in srgb, var(--td-brand-color) 35%, transparent);
  color: var(--td-brand-color);
}
.workspace-setup-step.is-active .workspace-setup-step__num {
  background: var(--td-brand-color);
  border-color: var(--td-brand-color);
  color: #fff;
  box-shadow: 0 2px 8px 0 color-mix(in srgb, var(--td-brand-color) 35%, transparent);
}
.workspace-setup-body, .workspace-setup-result { display: flex; flex-direction: column; gap: 18px; }
.workspace-setup-section {
  display: flex; flex-direction: column; gap: 16px;
  padding: 20px 24px;
  border-radius: 16px;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  box-shadow: 0 1px 3px 0 color-mix(in srgb, var(--td-gray-color-3) 20%, transparent);
}
.workspace-setup-section__title-row { display: flex; align-items: center; gap: 10px; }
.workspace-setup-section__head h4 { margin: 0; font-size: 16px; font-weight: 600; letter-spacing: 0.02em; }
.workspace-setup-section__title-icon {
  display: inline-flex; align-items: center; justify-content: center;
  width: 30px; height: 30px; flex-shrink: 0;
  border-radius: 9px;
  background: color-mix(in srgb, var(--td-brand-color) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--td-brand-color) 22%, transparent);
  color: var(--td-brand-color);
}
.workspace-setup-section__head p, .workspace-setup-section__copy, .workspace-setup-field__hint {
  margin: 4px 0 0; color: var(--td-text-color-secondary); font-size: 13px;
}
.workspace-setup-form-grid { display: grid; gap: 16px; }
.workspace-setup-form-grid--two-col { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px 20px; }
.workspace-setup-field { display: flex; flex-direction: column; gap: 6px; }
.workspace-setup-field > label, .workspace-setup-field__label {
  font-size: 12px; font-weight: 500;
  color: var(--td-text-color-secondary);
  text-transform: none;
  letter-spacing: 0.01em;
}
.workspace-setup-checkbox-row {
  padding: 10px 14px;
  border-radius: 10px;
  background: var(--td-bg-color-page);
  border: 1px solid color-mix(in srgb, var(--td-component-stroke) 70%, transparent);
}
/* Inline info note (step 1, retry-only) */
.workspace-setup-inline-note {
  display: flex; align-items: flex-start; gap: 8px;
  padding: 8px 12px;
  border-radius: 8px;
  background: var(--td-bg-color-page);
  border: 1px solid color-mix(in srgb, var(--td-component-stroke) 70%, transparent);
  font-size: 12px; line-height: 1.55;
  color: var(--td-text-color-secondary);
}
.workspace-setup-inline-note > t-icon:first-child,
.workspace-setup-inline-note > t-icon:first-child svg {
  flex-shrink: 0; margin-top: 1px;
  color: var(--td-text-color-placeholder);
}
.workspace-setup-inline-note--success {
  background: color-mix(in srgb, var(--td-success-color) 7%, transparent);
  border-color: color-mix(in srgb, var(--td-success-color) 25%, transparent);
  color: var(--td-success-color);
}
.workspace-setup-inline-note--success > t-icon:first-child,
.workspace-setup-inline-note--success > t-icon:first-child svg {
  color: var(--td-success-color);
}
/* Legacy callout (still used in result step) */
.workspace-setup-callout { border-radius: 10px; padding: 10px 14px; font-size: 12.5px; line-height: 1.6; }
.workspace-setup-callout--success { background: color-mix(in srgb, var(--td-success-color) 10%, transparent); color: var(--td-success-color); }
.workspace-setup-callout--neutral { background: var(--td-bg-color-page); color: var(--td-text-color-secondary); }
.workspace-setup-result__hero { display: flex; gap: 12px; align-items: flex-start; }
.workspace-setup-result__icon { color: var(--td-success-color); }
.workspace-setup-result__hero h4 { margin: 0; }
.workspace-setup-result__hero p { margin: 4px 0 0; color: var(--td-text-color-secondary); }
.workspace-setup-result__card { padding: 16px; border-radius: 14px; background: var(--td-bg-color-container); border: 1px solid var(--td-component-stroke); }
.workspace-setup-result__summary { margin: 0; line-height: 1.7; }
.workspace-setup-result__summary--sub { margin-top: 8px; color: var(--td-text-color-secondary); }
.workspace-setup-result__link-row { display: flex; gap: 10px; align-items: center; }
.workspace-setup-footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
@media (max-width: 720px) {
  .workspace-setup-form-grid--two-col { grid-template-columns: 1fr; }
  .workspace-setup-result__link-row { flex-direction: column; align-items: stretch; }
}
</style>