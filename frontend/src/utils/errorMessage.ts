type TranslateFn = (key: string, params?: Record<string, unknown>) => string

const REASON_KEY_MAP: Record<string, string> = {
  'user.invalid_credentials': 'error.messages.user.invalid_credentials',
  'user.account_disabled': 'error.messages.user.account_disabled',
  'user.already_exists': 'error.messages.user.already_exists',
  'user.username_exists': 'error.messages.user.username_exists',
  'user.email_exists': 'error.messages.user.email_exists',
  'user.phone_exists': 'error.messages.user.phone_exists',
  'bootstrap.user_create_failed': 'error.messages.bootstrap.user_create_failed',
  'tenant.required': 'error.messages.tenant.required',
  'auth.login_failed': 'error.auth.loginFailed',
  'auth.register_failed': 'error.auth.registerFailed',
  'auth.api_key_service_unavailable': 'error.messages.auth.api_key_service_unavailable',
  'auth.invalid_or_expired_token': 'error.messages.auth.invalid_or_expired_token',
  'auth.missing_authentication': 'error.messages.auth.missing_authentication',
  'auth.invalid_workspace': 'error.messages.auth.invalid_workspace',
  'auth.not_member_of_target_workspace': 'error.messages.auth.not_member_of_target_workspace',
  'auth.invalid_tenant_header': 'error.messages.auth.invalid_tenant_header',
  'auth.target_workspace_forbidden': 'error.messages.auth.target_workspace_forbidden',
  'auth.invalid_api_key': 'error.messages.auth.invalid_api_key',
  'auth.invalid_api_key_scope': 'error.messages.auth.invalid_api_key_scope',
  'auth.workspace_switch_forbidden': 'error.messages.auth.workspace_switch_forbidden',
  'embed.channel_id_required': 'error.messages.embed.channel_id_required',
  'embed.token_required': 'error.messages.embed.token_required',
  'embed.invalid_channel_or_token': 'error.messages.embed.invalid_channel_or_token',
  'embed.channel_disabled': 'error.messages.embed.channel_disabled',
  'embed.origin_not_allowed': 'error.messages.embed.origin_not_allowed',
  'embed.rate_limited': 'error.messages.embed.rate_limited',
  'embed.workspace_unavailable': 'error.messages.embed.workspace_unavailable',
  'system.docreader.addr_required': 'error.messages.system.docreader.addr_required',
  'system.docreader.addr_empty': 'error.messages.system.docreader.addr_empty',
  'system.docreader.addr_invalid': 'error.messages.system.docreader.addr_invalid',
  'system.docreader.unavailable': 'error.messages.system.docreader.unavailable',
  'system.docreader.reconnect_failed': 'error.messages.system.docreader.reconnect_failed',
  'system.parser.invalid_request_body': 'error.messages.system.parser.invalid_request_body',
  'system.storage.invalid_request_body': 'error.messages.system.storage.invalid_request_body',
  'system.storage.provider_disabled': 'error.messages.system.storage.provider_disabled',
  'invite.invalid_or_revoked': 'inviteRegister.invalidBody',
  'invite.email_already_has_account': 'error.messages.invite.email_already_has_account',
  'invite.link_no_longer_valid_login': 'error.messages.invite.link_no_longer_valid_login',
}

const STRING_CODE_KEY_MAP: Record<string, string> = {
  TENANT_REQUIRED: 'error.messages.tenant.required',
  sandboxes_still_live: 'error.messages.sandbox.sandboxes_still_live',
  sandbox_inventory_unverifiable: 'error.messages.sandbox.sandbox_inventory_unverifiable',
  sandbox_config_cordoned: 'error.messages.sandbox.sandbox_config_cordoned',
}

function isRecord(value: unknown): value is Record<string, any> {
  return typeof value === 'object' && value !== null
}

export function getErrorReason(error: unknown): string | undefined {
  if (!isRecord(error)) return undefined
  const topLevelReason = error.reason
  if (typeof topLevelReason === 'string' && topLevelReason) return topLevelReason

  const details = error.details
  if (isRecord(details) && typeof details.reason === 'string' && details.reason) {
    return details.reason
  }

  const nestedError = error.error
  if (isRecord(nestedError)) {
    if (typeof nestedError.reason === 'string' && nestedError.reason) {
      return nestedError.reason
    }
    const nestedDetails = nestedError.details
    if (isRecord(nestedDetails) && typeof nestedDetails.reason === 'string' && nestedDetails.reason) {
      return nestedDetails.reason
    }
  }

  return undefined
}

export function getErrorField(error: unknown): string | undefined {
  if (!isRecord(error)) return undefined
  const topLevelField = error.field
  if (typeof topLevelField === 'string' && topLevelField) return topLevelField

  const details = error.details
  if (isRecord(details) && typeof details.field === 'string' && details.field) {
    return details.field
  }

  const nestedError = error.error
  if (isRecord(nestedError)) {
    if (typeof nestedError.field === 'string' && nestedError.field) {
      return nestedError.field
    }
    const nestedDetails = nestedError.details
    if (isRecord(nestedDetails) && typeof nestedDetails.field === 'string' && nestedDetails.field) {
      return nestedDetails.field
    }
  }

  return undefined
}

export function getErrorStringCode(error: unknown): string | undefined {
  if (!isRecord(error)) return undefined

  const topLevelCode = error.code
  if (typeof topLevelCode === 'string' && topLevelCode) return topLevelCode

  const nestedError = error.error
  if (isRecord(nestedError) && typeof nestedError.code === 'string' && nestedError.code) {
    return nestedError.code
  }

  return undefined
}

export function getRawErrorMessage(error: unknown): string | undefined {
  if (typeof error === 'string' && error) return error
  if (!isRecord(error)) return undefined

  if (typeof error.message === 'string' && error.message) return error.message
  if (typeof error.msg === 'string' && error.msg) return error.msg

  const nestedError = error.error
  if (typeof nestedError === 'string' && nestedError) return nestedError
  if (isRecord(nestedError) && typeof nestedError.message === 'string' && nestedError.message) {
    return nestedError.message
  }

  return undefined
}

function resolveMappedKey(reason: string | undefined, field: string | undefined): string | undefined {
  if (reason === 'user_conflict') {
    if (field === 'phone') return 'error.messages.user.phone_exists'
    if (field === 'email') return 'error.messages.user.email_exists'
    if (field === 'username') return 'error.messages.user.username_exists'
  }
  if (reason === 'idempotency_key_reused') {
    return 'error.messages.bootstrap.idempotency_key_reused'
  }
  if (reason) {
    return REASON_KEY_MAP[reason]
  }
  return undefined
}

export function resolveErrorMessageWithT(
  error: unknown,
  translate: TranslateFn,
  fallbackKey: string,
  fallbackParams?: Record<string, unknown>,
): string {
  const reason = getErrorReason(error)
  const field = getErrorField(error)
  const mappedReasonKey = resolveMappedKey(reason, field)
  if (mappedReasonKey) {
    return translate(mappedReasonKey)
  }

  const stringCode = getErrorStringCode(error)
  if (stringCode && STRING_CODE_KEY_MAP[stringCode]) {
    return translate(STRING_CODE_KEY_MAP[stringCode])
  }

  const rawMessage = getRawErrorMessage(error)
  if (rawMessage) {
    return rawMessage
  }

  return translate(fallbackKey, fallbackParams)
}

export function resolveErrorMessage(
  error: unknown,
  translate: TranslateFn,
  fallbackKey: string,
  fallbackParams?: Record<string, unknown>,
): string {
  return resolveErrorMessageWithT(error, translate, fallbackKey, fallbackParams)
}