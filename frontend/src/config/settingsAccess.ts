export type SettingsRoleKey = 'viewer' | 'contributor' | 'admin' | 'owner'

/**
 * Workspace-scoped settings access policy.
 *
 * Keep this as the single frontend source of truth for both the complete
 * Settings navigation and any shortcuts that lead into it. Backend route
 * guards remain authoritative.
 */
export const SETTINGS_SECTION_MIN_ROLE: Record<string, SettingsRoleKey> = {
  general: 'viewer',
  ollama: 'admin',
  weknoracloud: 'admin',
  models: 'viewer',
  websearch: 'admin',
  chathistory: 'admin',
  vectorstore: 'admin',
  parser: 'admin',
  storage: 'admin',
  sandbox: 'admin',
  // Install writes a root shell into the sandbox image every session of
  // that config boots. Same Admin+ bar as the sandbox editor itself.
  skills: 'admin',
  mcp: 'admin',
  system: 'viewer',
  userprofile: 'viewer',
  browserconnection: 'viewer',
  tenant: 'viewer',
  members: 'owner',
  mymemory: 'viewer',
  memory: 'admin',
  // 沙箱密钥入口已收紧为 owner-only（仅空间 owner / 超级管理员可见）；
  // 超级管理员经 canAccessAllTenants bypass，后端 /me/env-vars 仍只读写
  // 调用者自己的值（routes_auth_tenant.go 刻意不加角色门）。
  envvars: 'owner',
}

/**
 * A management-labelled avatar shortcut has a stricter threshold than the
 * corresponding read-only Settings page.
 */
export const SETTINGS_MANAGEMENT_SHORTCUT_MIN_ROLE = {
  members: 'owner',
  models: 'admin',
} as const satisfies Record<string, SettingsRoleKey>

export const SYSTEM_ADMIN_SETTINGS_SECTIONS = new Set([
  'system-global',
  'model-catalog',
  'runtime-queues',
  'platform-api-keys',
  'system-audit-log',
])

/**
 * 仅超级管理员可见的设置入口（isSuperAdmin / superuser bypass）。
 * 「Ollama 配置」「系统信息」已按需求收紧：普通空间 owner/admin
 * 不再看到这两个入口；后端路由鉴权仍为准，这里只控制 UI 是否露出。
 */
export const SUPER_ADMIN_ONLY_SETTINGS_SECTIONS = new Set(['ollama', 'system'])
