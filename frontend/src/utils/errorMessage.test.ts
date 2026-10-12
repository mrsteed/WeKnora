import assert from 'node:assert/strict'
import test from 'node:test'

(globalThis as any).localStorage = {
  getItem: () => null,
  setItem: () => undefined,
  removeItem: () => undefined,
};

(globalThis as any).window = {
  __RUNTIME_CONFIG__: {},
  location: { pathname: '/' },
};

const modPromise = import('./errorMessage.ts');

const translations: Record<string, string> = {
  'error.generic': '通用兜底',
  'error.messages.user.invalid_credentials': '用户名或密码错误',
  'error.messages.user.account_disabled': '账号已被禁用',
  'error.messages.user.already_exists': '用户已存在',
  'error.messages.user.username_exists': '用户名已存在',
  'error.messages.user.email_exists': '邮箱已存在',
  'error.messages.user.phone_exists': '手机号已存在',
  'error.messages.auth.invalid_or_expired_token': '登录已失效，请重新登录',
  'error.messages.tenant.required': '请先选择或创建空间',
  'error.messages.bootstrap.idempotency_key_reused': '请刷新后重试初始化',
  'error.messages.bootstrap.user_create_failed': '初始化失败，请稍后重试',
}

function fakeT(key: string): string {
  return translations[key] || key
}

test('extracts reason and field from nested app error details', () => {
  return modPromise.then(({ getErrorField, getErrorReason }) => {
  const error = {
    error: {
      details: {
        reason: 'user_conflict',
        field: 'phone',
      },
    },
  }

    assert.equal(getErrorReason(error), 'user_conflict')
    assert.equal(getErrorField(error), 'phone')
  })
})

test('extracts string code and raw message from top-level legacy payload', () => {
  return modPromise.then(({ getErrorStringCode, getRawErrorMessage }) => {
  const error = {
    code: 'TENANT_REQUIRED',
    msg: 'Workspace required',
  }

    assert.equal(getErrorStringCode(error), 'TENANT_REQUIRED')
    assert.equal(getRawErrorMessage(error), 'Workspace required')
  })
})

test('reason mapping wins over raw message', () => {
  return modPromise.then(({ resolveErrorMessageWithT }) => {
  const error = {
    message: 'Invalid email or password',
    details: {
      reason: 'user.invalid_credentials',
    },
  }

    assert.equal(
      resolveErrorMessageWithT(error, fakeT, 'error.generic'),
      '用户名或密码错误',
    )
  })
})

test('direct reason mapping localizes registration conflicts', () => {
  return modPromise.then(({ resolveErrorMessageWithT }) => {
    const error = {
      message: 'user with this email already exists',
      details: {
        reason: 'user.email_exists',
      },
    }

    assert.equal(
      resolveErrorMessageWithT(error, fakeT, 'error.generic'),
      '邮箱已存在',
    )
  })
})

test('field-aware bootstrap mapping wins over generic reason fallback', () => {
  return modPromise.then(({ resolveErrorMessageWithT }) => {
  const error = {
    details: {
      reason: 'user_conflict',
      field: 'phone',
    },
    message: 'Phone already exists',
  }

    assert.equal(
      resolveErrorMessageWithT(error, fakeT, 'error.generic'),
      '手机号已存在',
    )
  })
})

test('string code fallback localizes tenant required errors', () => {
  return modPromise.then(({ resolveErrorMessageWithT }) => {
  const error = {
    code: 'TENANT_REQUIRED',
    message: 'Workspace required',
  }

    assert.equal(
      resolveErrorMessageWithT(error, fakeT, 'error.generic'),
      '请先选择或创建空间',
    )
  })
})

test('returns raw message when no mapping exists', () => {
  return modPromise.then(({ resolveErrorMessageWithT }) => {
  const error = {
    message: 'plain upstream failure',
  }

    assert.equal(
      resolveErrorMessageWithT(error, fakeT, 'error.generic'),
      'plain upstream failure',
    )
  })
})

test('falls back to the provided key when no message or mapping exists', () => {
  return modPromise.then(({ resolveErrorMessageWithT }) => {
    assert.equal(resolveErrorMessageWithT({}, fakeT, 'error.generic'), '通用兜底')
  })
})

test('maps idempotency key reuse to the setup wizard copy', () => {
  return modPromise.then(({ resolveErrorMessageWithT }) => {
  const error = {
    details: {
      reason: 'idempotency_key_reused',
    },
  }

    assert.equal(
      resolveErrorMessageWithT(error, fakeT, 'error.generic'),
      '请刷新后重试初始化',
    )
  })
})