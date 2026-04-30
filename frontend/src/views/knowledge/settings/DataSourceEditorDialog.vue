<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import {
  createDataSource,
  updateDataSource,
  triggerSync,
  validateConnection,
  validateCredentials,
  listResources,
  deleteDataSource,
  type DataSource,
  type Resource,
} from '@/api/datasource'
import DataSourceTypeIcon from './DataSourceTypeIcon.vue'

const props = defineProps<{
  kbId: string
  dataSource: DataSource | null
}>()

const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ saved: [] }>()
const { t } = useI18n()

const DEFAULT_SYNC_SCHEDULE = '0 0 */6 * * *'

interface ConnectorField {
  key: string
  labelKey?: string
  label?: string
  placeholder: string
  section: 'credentials' | 'settings'
  secret?: boolean
  optional?: boolean
  hintKey?: string
  hint?: string
  inputType?: 'text' | 'password' | 'number'
}

interface ConnectorDef {
  type: string
  available: boolean
  kind: 'sync' | 'database'
  labelKey?: string
  label?: string
  descriptionKey?: string
  description?: string
  docUrl: string
  permissionDocUrl: string
  permissionPageUrl: string
  requiredPermissions: string[]
  fields: ConnectorField[]
}

const isEdit = computed(() => !!props.dataSource)
const step = ref(0)
const submitting = ref(false)

function splitMultilineList(raw: string): string[] {
  return raw
    .split(/\r?\n|,/)
    .map(item => item.trim())
    .filter(Boolean)
}

const form = ref({
  name: '',
  type: '',
  config: {
    credentials: {} as Record<string, any>,
    resource_ids: [] as string[],
    settings: {} as Record<string, any>,
  },
  sync_schedule: DEFAULT_SYNC_SCHEDULE,
  sync_mode: 'incremental' as 'incremental' | 'full',
  conflict_strategy: 'overwrite' as 'overwrite' | 'skip',
  sync_deletions: true,
})

const resources = ref<Resource[]>([])
const loadingResources = ref(false)
const selectedResourceIds = ref<string[]>([])
const expandedResourceIds = ref(new Set<string>())

const childrenMap = computed(() => {
  const map = new Map<string, Resource[]>()
  for (const resource of resources.value) {
    if (!resource.parent_id) continue
    const siblings = map.get(resource.parent_id)
    if (siblings) siblings.push(resource)
    else map.set(resource.parent_id, [resource])
  }
  return map
})

const parentMap = computed(() => {
  const map = new Map<string, string>()
  for (const resource of resources.value) {
    if (resource.parent_id) map.set(resource.external_id, resource.parent_id)
  }
  return map
})

type CheckState = 'checked' | 'indeterminate' | 'unchecked'

const checkStates = computed(() => {
  const states = new Map<string, CheckState>()
  const cover = new Set(selectedResourceIds.value)

  function walk(node: Resource, ancestorChecked: boolean): boolean {
    const selfChecked = ancestorChecked || cover.has(node.external_id)
    let descendantChecked = false
    for (const child of childrenMap.value.get(node.external_id) || []) {
      if (walk(child, selfChecked)) descendantChecked = true
    }
    if (selfChecked) states.set(node.external_id, 'checked')
    else states.set(node.external_id, descendantChecked ? 'indeterminate' : 'unchecked')
    return selfChecked || descendantChecked
  }

  for (const resource of resources.value) {
    if (!resource.parent_id) walk(resource, false)
  }
  return states
})

function toggleExpand(id: string) {
  const next = new Set(expandedResourceIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expandedResourceIds.value = next
}

const visibleTree = computed(() => {
  const roots = resources.value.filter(resource => !resource.parent_id)
  const result: { resource: Resource; depth: number }[] = []
  function walk(items: Resource[], depth: number) {
    for (const resource of items) {
      result.push({ resource, depth })
      if (resource.has_children && expandedResourceIds.value.has(resource.external_id)) {
        walk(childrenMap.value.get(resource.external_id) || [], depth + 1)
      }
    }
  }
  walk(roots, 0)
  return result
})

const testing = ref(false)
const testResult = ref<'success' | 'error' | ''>('')
const testErrorMsg = ref('')
const prereqExpanded = ref(false)
const tempDsId = ref('')
const keepDraftOnClose = ref(false)

const schedulePresets = computed(() => [
  { label: t('datasource.schedule30min'), value: '0 */30 * * * *' },
  { label: t('datasource.schedule1h'), value: '0 0 * * * *' },
  { label: t('datasource.schedule6h'), value: '0 0 */6 * * *' },
  { label: t('datasource.schedule12h'), value: '0 0 */12 * * *' },
  { label: t('datasource.schedule24h'), value: '0 0 2 * * *' },
])

function translateOrFallback(key: string, fallback: string) {
  const translated = t(key)
  return translated === key ? fallback : translated
}

const connectorDefs = computed<ConnectorDef[]>(() => [
  {
    type: 'feishu',
    available: true,
    kind: 'sync',
    labelKey: 'datasource.connector.feishu',
    descriptionKey: 'datasource.connectorDesc.feishu',
    docUrl: 'https://open.feishu.cn/app',
    permissionDocUrl: 'https://open.feishu.cn/document/server-docs/docs/wiki-v2/wiki-overview',
    permissionPageUrl: 'https://open.feishu.cn/app',
    requiredPermissions: [
      'wiki:wiki:readonly',
      'drive:drive:readonly',
      'drive:export:readonly',
      'docx:document:readonly',
    ],
    fields: [
      { key: 'app_id', labelKey: 'datasource.field.appId', placeholder: 'cli_xxxx', section: 'credentials' },
      { key: 'app_secret', labelKey: 'datasource.field.appSecret', placeholder: '', section: 'credentials', secret: true },
    ],
  },
  {
    type: 'notion',
    available: true,
    kind: 'sync',
    labelKey: 'datasource.connector.notion',
    descriptionKey: 'datasource.connectorDesc.notion',
    docUrl: 'https://www.notion.so/my-integrations',
    permissionDocUrl: '',
    permissionPageUrl: '',
    requiredPermissions: [],
    fields: [
      { key: 'api_key', labelKey: 'datasource.field.integrationToken', placeholder: 'ntn_xxxx', section: 'credentials', secret: true },
    ],
  },
  {
    type: 'yuque',
    available: true,
    kind: 'sync',
    labelKey: 'datasource.connector.yuque',
    descriptionKey: 'datasource.connectorDesc.yuque',
    docUrl: 'https://www.yuque.com/yuque/developer/api',
    permissionDocUrl: 'https://www.yuque.com/yuque/developer/api',
    permissionPageUrl: 'https://www.yuque.com/settings/tokens',
    requiredPermissions: [
      'repo:read',
      'doc:read',
    ],
    fields: [
      { key: 'api_token', labelKey: 'datasource.field.apiToken', placeholder: '', section: 'credentials', secret: true },
      { key: 'base_url', labelKey: 'datasource.field.baseUrl', placeholder: 'https://www.yuque.com', section: 'credentials', optional: true, hintKey: 'datasource.field.baseUrlHint' },
    ],
  },
  {
    type: 'mysql',
    available: true,
    kind: 'database',
    label: 'MySQL',
    description: '连接 MySQL 并发现当前库下可查询的表和视图。',
    docUrl: '',
    permissionDocUrl: '',
    permissionPageUrl: '',
    requiredPermissions: [],
    fields: [
      { key: 'host', label: 'Host', placeholder: '127.0.0.1', section: 'settings' },
      { key: 'port', label: 'Port', placeholder: '3306', section: 'settings', inputType: 'number' },
      { key: 'database', label: 'Database', placeholder: 'crm', section: 'settings' },
      { key: 'ssl_mode', label: 'SSL Mode', placeholder: 'false / preferred / required', section: 'settings', optional: true },
      { key: 'username', label: 'Username', placeholder: 'readonly_user', section: 'credentials' },
      { key: 'password', label: 'Password', placeholder: '', section: 'credentials', secret: true, optional: true },
    ],
  },
  {
    type: 'postgresql',
    available: true,
    kind: 'database',
    label: 'PostgreSQL',
    description: '连接 PostgreSQL 并发现当前 Schema 下可查询的表和视图。',
    docUrl: '',
    permissionDocUrl: '',
    permissionPageUrl: '',
    requiredPermissions: [],
    fields: [
      { key: 'host', label: 'Host', placeholder: '127.0.0.1', section: 'settings' },
      { key: 'port', label: 'Port', placeholder: '5432', section: 'settings', inputType: 'number' },
      { key: 'database', label: 'Database', placeholder: 'crm', section: 'settings' },
      { key: 'schema', label: 'Schema', placeholder: 'public', section: 'settings', optional: true },
      { key: 'ssl_mode', label: 'SSL Mode', placeholder: 'disable / require / verify-full', section: 'settings', optional: true },
      { key: 'username', label: 'Username', placeholder: 'readonly_user', section: 'credentials' },
      { key: 'password', label: 'Password', placeholder: '', section: 'credentials', secret: true, optional: true },
    ],
  },
])

const currentDef = computed(() => connectorDefs.value.find(def => def.type === form.value.type))
const isDatabaseConnector = computed(() => currentDef.value?.kind === 'database')

function connectorLabel(type: string) {
  const def = connectorDefs.value.find(item => item.type === type)
  if (!def) return type
  if (def.label) return def.label
  if (def.labelKey) return translateOrFallback(def.labelKey, type)
  return type
}

function connectorDescription(def: ConnectorDef) {
  if (def.description) return def.description
  if (def.descriptionKey) return translateOrFallback(def.descriptionKey, def.type)
  return def.type
}

function fieldLabel(field: ConnectorField) {
  if (field.label) return field.label
  if (field.labelKey) return translateOrFallback(field.labelKey, field.key)
  return field.key
}

function fieldHint(field: ConnectorField) {
  if (field.hint) return field.hint
  if (field.hintKey) return translateOrFallback(field.hintKey, '')
  return ''
}

function isEncryptedPassword(value: unknown) {
  return typeof value === 'string' && value.startsWith('enc:v1:')
}

function defaultConfigForType(type: string) {
  if (type === 'mysql') {
    return {
      credentials: { username: '', password: '' },
      resource_ids: [] as string[],
      settings: {
        host: '',
        port: 3306,
        database: '',
        ssl_mode: 'false',
        table_allowlist: [] as string[],
        column_denylist: [] as string[],
        max_rows: undefined as number | undefined,
        query_timeout_sec: undefined as number | undefined,
      },
    }
  }
  if (type === 'postgresql') {
    return {
      credentials: { username: '', password: '' },
      resource_ids: [] as string[],
      settings: {
        host: '',
        port: 5432,
        database: '',
        schema: 'public',
        ssl_mode: 'disable',
        table_allowlist: [] as string[],
        column_denylist: [] as string[],
        max_rows: undefined as number | undefined,
        query_timeout_sec: undefined as number | undefined,
      },
    }
  }
  return {
    credentials: {} as Record<string, any>,
    resource_ids: [] as string[],
    settings: {} as Record<string, any>,
  }
}

function normalizeConnectorConfig(type: string, rawConfig?: any) {
  const fallback = defaultConfigForType(type)
  const config = JSON.parse(JSON.stringify(rawConfig || fallback))
  config.credentials = config.credentials || {}
  config.settings = config.settings || {}
  config.resource_ids = Array.isArray(config.resource_ids) ? config.resource_ids : []

  if (type === 'mysql' && (config.settings.port == null || config.settings.port === '')) {
    config.settings.port = 3306
  }
  if (type === 'postgresql' && (config.settings.port == null || config.settings.port === '')) {
    config.settings.port = 5432
  }
  config.settings.table_allowlist = Array.isArray(config.settings.table_allowlist) ? config.settings.table_allowlist : []
  config.settings.column_denylist = Array.isArray(config.settings.column_denylist) ? config.settings.column_denylist : []
  if (isEncryptedPassword(config.credentials.password)) {
    config.credentials.password = ''
  }
  return config
}

const tableAllowlistText = computed({
  get: () => (Array.isArray(form.value.config.settings.table_allowlist) ? form.value.config.settings.table_allowlist.join('\n') : ''),
  set: (value: string) => {
    form.value.config.settings.table_allowlist = splitMultilineList(value)
  },
})

const columnDenylistText = computed({
  get: () => (Array.isArray(form.value.config.settings.column_denylist) ? form.value.config.settings.column_denylist.join('\n') : ''),
  set: (value: string) => {
    form.value.config.settings.column_denylist = splitMultilineList(value)
  },
})

function resetForm() {
  form.value = {
    name: '',
    type: '',
    config: defaultConfigForType(''),
    sync_schedule: DEFAULT_SYNC_SCHEDULE,
    sync_mode: 'incremental',
    conflict_strategy: 'overwrite',
    sync_deletions: true,
  }
}

watch(visible, (value) => {
  if (!value) return
  step.value = isEdit.value ? 1 : 0
  testResult.value = ''
  testErrorMsg.value = ''
  tempDsId.value = ''
  keepDraftOnClose.value = false
  prereqExpanded.value = false
  resources.value = []
  selectedResourceIds.value = []
  expandedResourceIds.value = new Set<string>()

  if (isEdit.value && props.dataSource) {
    const config = normalizeConnectorConfig(props.dataSource.type, props.dataSource.config)
    form.value = {
      name: props.dataSource.name,
      type: props.dataSource.type,
      config,
      sync_schedule: props.dataSource.sync_schedule || DEFAULT_SYNC_SCHEDULE,
      sync_mode: props.dataSource.sync_mode,
      conflict_strategy: props.dataSource.conflict_strategy,
      sync_deletions: props.dataSource.sync_deletions,
    }
    selectedResourceIds.value = config.resource_ids || []
    tempDsId.value = props.dataSource.id
    return
  }

  resetForm()
})

function selectType(def: ConnectorDef) {
  if (!def.available) return
  form.value.type = def.type
  form.value.name = connectorLabel(def.type)
  form.value.config = normalizeConnectorConfig(def.type)
  if (def.kind === 'database') {
    form.value.sync_schedule = ''
    form.value.sync_mode = 'incremental'
    form.value.conflict_strategy = 'overwrite'
    form.value.sync_deletions = false
  }
  step.value = 1
}

function getFieldValue(field: ConnectorField) {
  return field.section === 'settings'
    ? form.value.config.settings[field.key]
    : form.value.config.credentials[field.key]
}

function setFieldValue(field: ConnectorField, value: any) {
  const nextValue = field.inputType === 'number'
    ? (value === '' || value == null ? undefined : Number(value))
    : value

  if (field.section === 'settings') form.value.config.settings[field.key] = nextValue
  else form.value.config.credentials[field.key] = nextValue
}

function validateStep1Fields(): boolean {
  const fields = currentDef.value?.fields || []
  for (const field of fields) {
    if (field.optional) continue
    const value = getFieldValue(field)
    if (value == null || (typeof value === 'string' && value.trim() === '')) {
      MessagePlugin.warning(`${fieldLabel(field)} ${t('datasource.isRequired')}`)
      return false
    }
  }
  return true
}

async function upsertDraftDataSource(status: 'paused' | 'active') {
  const payload = {
    ...form.value,
    knowledge_base_id: props.kbId,
    status,
  } as any

  if (!tempDsId.value) {
    const response = await createDataSource(payload)
    const created = response?.data || response
    tempDsId.value = created.id
    return created.id
  }

  await updateDataSource(tempDsId.value, payload)
  return tempDsId.value
}

async function testConnection() {
  if (!validateStep1Fields()) return

  testing.value = true
  testResult.value = ''
  testErrorMsg.value = ''
  try {
    if (isDatabaseConnector.value || (isEdit.value && tempDsId.value)) {
      await upsertDraftDataSource('paused')
      await validateConnection(tempDsId.value)
    } else {
      await validateCredentials(form.value.type, form.value.config.credentials)
    }
    testResult.value = 'success'
    MessagePlugin.success(t('datasource.testSuccess'))
  } catch (error: any) {
    testResult.value = 'error'
    testErrorMsg.value = error?.message || error?.error || ''
    MessagePlugin.error(t('datasource.testFailed'))
  }
  testing.value = false
}

async function loadResources() {
  loadingResources.value = true
  try {
    await upsertDraftDataSource('paused')
    const response = await listResources(tempDsId.value)
    resources.value = response?.data || response || []
    if (isDatabaseConnector.value) {
      expandedResourceIds.value = new Set(
        resources.value
          .filter(resource => resource.type === 'database' || resource.type === 'schema')
          .map(resource => resource.external_id),
      )
    }
  } catch (error: any) {
    MessagePlugin.error(error?.message || error?.error || t('datasource.resourceLoadFailed'))
  }
  loadingResources.value = false
}

function getDescendantIds(id: string): string[] {
  const ids: string[] = []
  const children = childrenMap.value.get(id) || []
  for (const child of children) {
    ids.push(child.external_id)
    ids.push(...getDescendantIds(child.external_id))
  }
  return ids
}

function getAncestorChain(id: string): string[] {
  const chain = [id]
  for (let parent = parentMap.value.get(id); parent; parent = parentMap.value.get(parent)) {
    chain.push(parent)
  }
  return chain
}

function isCovered(id: string, cover: Set<string>): boolean {
  for (let current: string | undefined = id; current; current = parentMap.value.get(current)) {
    if (cover.has(current)) return true
  }
  return false
}

function checkResource(id: string, cover: Set<string>) {
  if (isCovered(id, cover)) return
  const descendants = new Set(getDescendantIds(id))
  for (const covered of [...cover]) {
    if (descendants.has(covered)) cover.delete(covered)
  }
  cover.add(id)
}

function uncheckResource(id: string, cover: Set<string>) {
  const chain = getAncestorChain(id)
  let highestIdx = -1
  for (let index = chain.length - 1; index >= 0; index--) {
    if (cover.has(chain[index])) {
      highestIdx = index
      break
    }
  }
  if (highestIdx > 0) {
    cover.delete(chain[highestIdx])
    for (let index = highestIdx; index > 0; index--) {
      const parent = chain[index]
      const next = chain[index - 1]
      for (const sibling of childrenMap.value.get(parent) || []) {
        if (sibling.external_id !== next) cover.add(sibling.external_id)
      }
    }
  }
  cover.delete(id)
  const descendants = new Set(getDescendantIds(id))
  for (const covered of [...cover]) {
    if (descendants.has(covered)) cover.delete(covered)
  }
}

function toggleResource(id: string) {
  if (isDatabaseConnector.value) return
  const cover = new Set(selectedResourceIds.value)
  if ((checkStates.value.get(id) || 'unchecked') === 'unchecked') checkResource(id, cover)
  else uncheckResource(id, cover)
  selectedResourceIds.value = [...cover]
}

function nextStep() {
  if (step.value === 1) {
    if (!validateStep1Fields()) return
    if (testResult.value !== 'success') {
      MessagePlugin.warning(t('datasource.pleaseTestFirst'))
      return
    }
    step.value = 2
    loadResources()
    return
  }

  if (!isDatabaseConnector.value && step.value === 2) {
    step.value = 3
  }
}

function prevStep() {
  step.value--
}

async function handleSubmit() {
  form.value.config.resource_ids = isDatabaseConnector.value ? [] : selectedResourceIds.value
  submitting.value = true
  try {
    const dataSourceId = await upsertDraftDataSource('active')

    if (isEdit.value) {
      MessagePlugin.success(t('datasource.updateSuccess'))
    } else if (isDatabaseConnector.value) {
      MessagePlugin.success(t('datasource.createSuccess'))
    } else {
      try {
        await triggerSync(dataSourceId)
        MessagePlugin.success(t('datasource.createAndSyncSuccess'))
      } catch (error: any) {
        MessagePlugin.warning(error?.message || error?.error || t('datasource.createButSyncFailed'))
      }
    }

    emit('saved')
    keepDraftOnClose.value = true
    visible.value = false
  } catch (error: any) {
    MessagePlugin.error(error?.message || error?.error || t('datasource.saveFailed'))
  }
  submitting.value = false
}

async function handleClose() {
  if (!isEdit.value && tempDsId.value && !keepDraftOnClose.value) {
    try {
      await deleteDataSource(tempDsId.value)
    } catch {
      // Ignore cleanup errors for unsaved draft data sources.
    }
  }
  keepDraftOnClose.value = false
  tempDsId.value = ''
  visible.value = false
}

const resourceTypeLabelMap: Record<string, string> = {
  database: 'Database',
  schema: 'Schema',
  table: 'Table',
  view: 'View',
  wiki_space: 'datasource.resourceType.wikiSpace',
  doc_category: 'datasource.resourceType.docCategory',
  book: 'datasource.resourceType.book',
}

function resourceTypeLabel(type: string): string {
  const key = resourceTypeLabelMap[type]
  if (!key) return type
  if (key.startsWith('datasource.')) return translateOrFallback(key, type)
  return key
}

const resourceHintText = computed(() => (
  isDatabaseConnector.value
    ? translateOrFallback('datasource.resourceHintDatabase', '查看当前连接下可用的数据库、Schema、表和视图。')
    : t('datasource.resourceHint')
))

const noResourcesTitle = computed(() => (
  isDatabaseConnector.value
    ? translateOrFallback('datasource.noResourcesDatabase', '未发现可用的数据库对象')
    : t('datasource.noResources')
))

const noResourcesDescription = computed(() => {
  if (isDatabaseConnector.value && form.value.type === 'mysql') {
    return translateOrFallback('datasource.noResourcesDesc_mysql', '连接成功，但当前账号下没有可见的表或视图。')
  }
  if (isDatabaseConnector.value && form.value.type === 'postgresql') {
    return translateOrFallback('datasource.noResourcesDesc_postgresql', '连接成功，但当前 Schema 下没有可见的表或视图。')
  }
  return translateOrFallback(`datasource.noResourcesDesc_${form.value.type}`, t('datasource.noResourcesDesc'))
})

const stepTitles = computed(() => {
  if (isDatabaseConnector.value) {
    return [
      t('datasource.step.selectType'),
      t('datasource.step.credentials'),
      translateOrFallback('datasource.step.schema', 'Schema'),
    ]
  }
  return [
    t('datasource.step.selectType'),
    t('datasource.step.credentials'),
    t('datasource.step.resources'),
    t('datasource.step.strategy'),
  ]
})
</script>

<template>
  <t-dialog
    v-model:visible="visible"
    :header="isEdit ? t('datasource.editTitle') : t('datasource.createTitle')"
    :footer="false"
    width="640px"
    destroy-on-close
    :on-close="handleClose"
  >
    <!-- Step indicator -->
    <div class="ds-steps">
      <div
        v-for="(title, i) in stepTitles"
        :key="i"
        :class="['ds-step', { active: step === i, done: step > i }]"
      >
        <span class="ds-step-num">{{ step > i ? '&#10003;' : i + 1 }}</span>
        <span class="ds-step-title">{{ title }}</span>
      </div>
    </div>

    <!-- Step 0: Select connector type -->
    <div v-if="step === 0" class="ds-step-content">
      <div class="ds-type-grid">
        <div
          v-for="def in connectorDefs"
          :key="def.type"
          :class="['ds-type-card', { disabled: !def.available }]"
          @click="selectType(def)"
        >
          <div class="ds-type-header">
            <DataSourceTypeIcon :type="def.type" :size="20" />
            <span class="ds-type-name">{{ connectorLabel(def.type) }}</span>
            <span v-if="!def.available" class="ds-type-soon">{{ t('datasource.comingSoon') }}</span>
          </div>
          <div class="ds-type-desc">{{ connectorDescription(def) }}</div>
        </div>
      </div>
    </div>

    <!-- Step 1: Credentials -->
    <div v-if="step === 1" class="ds-step-content">
      <!-- Compact collapsible prereq hint -->
      <div v-if="currentDef && currentDef.requiredPermissions.length > 0" class="ds-prereq-bar" @click="prereqExpanded = !prereqExpanded">
        <t-icon name="help-circle" size="14px" />
        <span>{{ t(`datasource.prereqBarText_${form.type}`, t('datasource.prereqBarText')) }}</span>
        <t-icon :name="prereqExpanded ? 'chevron-up' : 'chevron-down'" size="14px" class="ds-prereq-arrow" />
      </div>
      <div v-if="prereqExpanded && currentDef" class="ds-prereq-detail">
        <div class="ds-prereq-item">
          <span class="ds-prereq-num">1</span>
          <div>
            <div class="ds-prereq-item-title">{{ t(`datasource.prereqStep1Brief_${form.type}`, t('datasource.prereqBotBrief')) }}</div>
            <div class="ds-prereq-item-desc">{{ t(`datasource.prereqStep1Desc_${form.type}`, t('datasource.prereqBotDesc')) }}</div>
          </div>
        </div>
        <div class="ds-prereq-item">
          <span class="ds-prereq-num">2</span>
          <div>
            <div class="ds-prereq-item-title">{{ t(`datasource.prereqStep2Brief_${form.type}`, t('datasource.prereqPermBrief')) }}</div>
            <div class="ds-prereq-item-desc">
              <template v-if="!t(`datasource.prereqStep2Desc_${form.type}`)">
                <code v-for="perm in currentDef.requiredPermissions" :key="perm" class="ds-perm-tag">{{ perm }}</code>
              </template>
              <template v-else>{{ t(`datasource.prereqStep2Desc_${form.type}`) }}</template>
            </div>
          </div>
        </div>
        <div class="ds-prereq-item">
          <span class="ds-prereq-num">3</span>
          <div>
            <div class="ds-prereq-item-title">{{ t(`datasource.prereqStep3Brief_${form.type}`, t('datasource.prereqMemberBrief')) }}</div>
            <div class="ds-prereq-item-desc">{{ t(`datasource.prereqStep3Desc_${form.type}`, t('datasource.prereqMemberDesc')) }}</div>
          </div>
        </div>
        <a :href="currentDef.permissionPageUrl" target="_blank" rel="noopener" class="ds-prereq-link">
          {{ t(`datasource.prereqOpenConsole_${form.type}`, t('datasource.prereqOpenConsole')) }}
        </a>
      </div>

      <div class="form-item">
        <label class="form-label">{{ t('datasource.nameLabel') }}</label>
        <t-input v-model="form.name" :placeholder="t('datasource.namePlaceholder')" />
      </div>

      <div v-if="currentDef?.docUrl" class="ds-doc-link">
        <t-icon name="info-circle" size="14px" />
        <span>{{ t('datasource.docHint') }}</span>
        <a :href="currentDef.docUrl" target="_blank" rel="noopener">{{ currentDef.docUrl }}</a>
      </div>

      <div v-for="field in currentDef?.fields || []" :key="field.key" class="form-item">
        <label class="form-label">
          {{ fieldLabel(field) }}
          <span v-if="!field.optional" class="required-mark">*</span>
        </label>
        <t-input
          :model-value="getFieldValue(field)"
          :placeholder="field.placeholder"
          :type="field.secret ? 'password' : (field.inputType === 'number' ? 'number' : 'text')"
          @update:model-value="setFieldValue(field, $event)"
        />
        <div v-if="fieldHint(field)" class="form-hint">{{ fieldHint(field) }}</div>
      </div>

      <template v-if="isDatabaseConnector">
        <div class="ds-db-advanced">
          <div class="ds-db-advanced-title">数据库查询约束</div>
          <div class="ds-db-advanced-desc">配置允许查询的表、禁止读取的敏感字段，以及默认的结果行数和超时限制。</div>
        </div>

        <div class="form-item">
          <label class="form-label">表白名单</label>
          <t-textarea
            v-model="tableAllowlistText"
            autosize
            :placeholder="'orders\ncustomers\norder_items'"
          />
          <div class="form-hint">为空表示不额外限制；填写后只允许查询这些表。支持逗号或换行分隔。</div>
        </div>

        <div class="form-item">
          <label class="form-label">字段黑名单</label>
          <t-textarea
            v-model="columnDenylistText"
            autosize
            :placeholder="'password_hash\nid_card\nmobile'"
          />
          <div class="form-hint">这些字段会被视为敏感列，Schema 展示和 SQL 查询都会受限。支持逗号或换行分隔。</div>
        </div>

        <div class="ds-db-limit-grid">
          <div class="form-item">
            <label class="form-label">最大返回行数</label>
            <t-input
              :model-value="form.config.settings.max_rows ?? ''"
              type="number"
              placeholder="200"
              @update:model-value="(value: string) => { form.config.settings.max_rows = value === '' ? undefined : Number(value) }"
            />
            <div class="form-hint">为空时使用后端默认值；超出上限时会被自动裁剪。</div>
          </div>

          <div class="form-item">
            <label class="form-label">查询超时（秒）</label>
            <t-input
              :model-value="form.config.settings.query_timeout_sec ?? ''"
              type="number"
              placeholder="15"
              @update:model-value="(value: string) => { form.config.settings.query_timeout_sec = value === '' ? undefined : Number(value) }"
            />
            <div class="form-hint">为空时使用后端默认值；超时会被中止并记录审计。</div>
          </div>
        </div>
      </template>

      <div class="form-actions">
        <t-button variant="outline" :loading="testing" @click="testConnection">
          {{ t('datasource.testConnection') }}
        </t-button>
        <span v-if="testResult === 'success'" class="test-ok">
          <t-icon name="check-circle-filled" size="14px" />
          {{ t('datasource.connected') }}
        </span>
      </div>
      <div v-if="testResult === 'error'" class="test-error-box">
        <t-icon name="error-circle-filled" size="16px" />
        <div class="test-error-content">
          <span class="test-error-title">{{ t('datasource.connectionFailed') }}</span>
          <span v-if="testErrorMsg" class="test-error-detail">{{ testErrorMsg }}</span>
        </div>
      </div>

      <div class="ds-dialog-footer">
        <t-button variant="outline" @click="step = 0" v-if="!isEdit">{{ t('datasource.back') }}</t-button>
        <t-button theme="primary" @click="nextStep">{{ t('datasource.next') }}</t-button>
      </div>
    </div>

    <!-- Step 2: Select resources -->
    <div v-if="step === 2" class="ds-step-content">
      <p class="form-tip">{{ resourceHintText }}</p>
      <div v-if="loadingResources" style="text-align:center;padding:20px"><t-loading /></div>
      <div v-else-if="resources.length > 0" class="ds-resource-list">
        <div
          v-for="{ resource: r, depth } in visibleTree"
          :key="r.external_id"
          :class="['ds-resource-row', { selected: !isDatabaseConnector && checkStates.get(r.external_id) === 'checked', readonly: isDatabaseConnector }]"
          :style="{ paddingLeft: `${12 + depth * 24}px` }"
          @click="toggleResource(r.external_id)"
        >
          <span
            v-if="r.has_children"
            class="ds-expand-btn"
            @click.stop="toggleExpand(r.external_id)"
          >
            <t-icon :name="expandedResourceIds.has(r.external_id) ? 'chevron-down' : 'chevron-right'" size="16px" />
          </span>
          <span v-else class="ds-expand-placeholder" />
          <t-checkbox
            v-if="!isDatabaseConnector"
            :checked="checkStates.get(r.external_id) === 'checked'"
            :indeterminate="checkStates.get(r.external_id) === 'indeterminate'"
            @click.stop
            @change="toggleResource(r.external_id)"
          />
          <div class="ds-resource-info">
            <div class="ds-resource-name">{{ r.name || t('datasource.untitled') }}</div>
            <div class="ds-resource-meta">
              <span class="ds-resource-type">{{ resourceTypeLabel(r.type) }}</span>
              <span v-if="r.description" class="ds-resource-desc">{{ r.description }}</span>
            </div>
          </div>
        </div>
      </div>
      <!-- Empty state: concise guide -->
      <div v-else class="ds-resource-empty">
        <t-icon name="info-circle" size="32px" style="color: var(--td-warning-color); margin-bottom: 8px;" />
        <p class="ds-empty-title">{{ noResourcesTitle }}</p>
        <p class="ds-empty-desc">{{ noResourcesDescription }}</p>
        <div v-if="!isDatabaseConnector" class="ds-guide-steps">
          <div class="ds-guide-step">
            <span class="ds-guide-num">1</span>
            <span>{{ t(`datasource.guideStep1_${form.type}`, t('datasource.guideStep1')) }}</span>
          </div>
          <div class="ds-guide-step">
            <span class="ds-guide-num">2</span>
            <span>{{ t(`datasource.guideStep2_${form.type}`, t('datasource.guideStep2')) }}</span>
          </div>
          <div class="ds-guide-step">
            <span class="ds-guide-num">3</span>
            <span>{{ t(`datasource.guideStep3_${form.type}`, t('datasource.guideStep3')) }}</span>
          </div>
        </div>
        <div class="ds-empty-actions">
          <t-button variant="outline" size="small" @click="loadResources">
            {{ t('datasource.retryLoadResources') }}
          </t-button>
          <a v-if="currentDef?.permissionDocUrl" :href="currentDef.permissionDocUrl" target="_blank" rel="noopener" class="ds-doc-link-inline">
            {{ t('datasource.permissionDocLink') }}
          </a>
        </div>
      </div>

      <div class="ds-dialog-footer">
        <t-button variant="outline" @click="prevStep">{{ t('datasource.back') }}</t-button>
        <t-button v-if="isDatabaseConnector" theme="primary" :loading="submitting" @click="handleSubmit">{{ t('datasource.save') }}</t-button>
        <t-button v-else theme="primary" @click="nextStep">{{ t('datasource.next') }}</t-button>
      </div>
    </div>

    <!-- Step 3: Sync strategy -->
    <div v-if="!isDatabaseConnector && step === 3" class="ds-step-content">
      <div class="form-item">
        <label class="form-label">{{ t('datasource.syncScheduleLabel') }}</label>
        <t-select v-model="form.sync_schedule">
          <t-option v-for="p in schedulePresets" :key="p.value" :value="p.value" :label="p.label" />
        </t-select>
      </div>

      <div class="form-item">
        <label class="form-label">{{ t('datasource.syncModeLabel') }}</label>
        <t-radio-group v-model="form.sync_mode">
          <t-radio value="incremental">{{ t('datasource.syncMode.incremental') }}</t-radio>
          <t-radio value="full">{{ t('datasource.syncMode.full') }}</t-radio>
        </t-radio-group>
      </div>

      <div class="form-item">
        <label class="form-label">{{ t('datasource.conflictLabel') }}</label>
        <t-radio-group v-model="form.conflict_strategy">
          <t-radio value="overwrite">{{ t('datasource.conflict.overwrite') }}</t-radio>
          <t-radio value="skip">{{ t('datasource.conflict.skip') }}</t-radio>
        </t-radio-group>
      </div>

      <div class="form-item">
        <t-checkbox v-model="form.sync_deletions">{{ t('datasource.syncDeletions') }}</t-checkbox>
      </div>

      <div class="ds-dialog-footer">
        <t-button variant="outline" @click="prevStep">{{ t('datasource.back') }}</t-button>
        <t-button theme="primary" :loading="submitting" @click="handleSubmit">
          {{ isEdit ? t('datasource.save') : t('datasource.createAndSync') }}
        </t-button>
      </div>
    </div>
  </t-dialog>
</template>

<style scoped>
.ds-steps {
  display: flex;
  gap: 4px;
  margin-bottom: 24px;
  border-bottom: 1px solid var(--td-border-level-2-color);
  padding-bottom: 16px;
}

.ds-step {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  font-size: 13px;
  color: var(--td-text-color-placeholder);
}

.ds-step.active { color: var(--td-brand-color); font-weight: 600; }
.ds-step.done { color: var(--td-success-color); }

.ds-step-num {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  border: 1px solid currentColor;
}

.ds-step.active .ds-step-num { background: var(--td-brand-color); color: #fff; border-color: var(--td-brand-color); }
.ds-step.done .ds-step-num { background: var(--td-success-color); color: #fff; border-color: var(--td-success-color); }

.ds-step-content { min-height: 200px; }

/* --- Step 0: type cards --- */
.ds-type-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
}

.ds-type-card {
  border: 1px solid var(--td-border-level-2-color);
  border-radius: 8px;
  padding: 14px;
  cursor: pointer;
  transition: all 0.2s;
}

.ds-type-card:hover:not(.disabled) { border-color: var(--td-brand-color); background: var(--td-brand-color-light); }
.ds-type-card.disabled { opacity: 0.5; cursor: not-allowed; }

.ds-type-header { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.ds-type-name { font-size: 13px; font-weight: 600; }
.ds-type-soon { font-size: 10px; color: var(--td-text-color-placeholder); background: var(--td-bg-color-component); padding: 1px 6px; border-radius: 3px; }
.ds-type-desc { font-size: 11px; color: var(--td-text-color-secondary); line-height: 1.5; }

/* --- Step 1: collapsible prereq --- */
.ds-prereq-bar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  margin-bottom: 16px;
  border-radius: 6px;
  background: var(--td-warning-color-1);
  color: var(--td-warning-color);
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  user-select: none;
  transition: background 0.15s;
}

.ds-prereq-bar:hover {
  background: var(--td-warning-color-2);
}

.ds-prereq-arrow {
  margin-left: auto;
}

.ds-prereq-detail {
  border: 1px solid var(--td-border-level-2-color);
  border-radius: 8px;
  padding: 14px;
  margin-bottom: 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.ds-prereq-item {
  display: flex;
  gap: 10px;
  align-items: flex-start;
}

.ds-prereq-num {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: var(--td-brand-color);
  color: #fff;
  font-size: 11px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  margin-top: 1px;
}

.ds-prereq-item-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--td-text-color-primary);
  line-height: 20px;
}

.ds-prereq-item-desc {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  margin-top: 2px;
  line-height: 1.5;
}

.ds-perm-tag {
  font-size: 11px;
  padding: 1px 5px;
  border-radius: 3px;
  background: var(--td-bg-color-component);
  color: var(--td-text-color-secondary);
  font-family: monospace;
  margin-right: 4px;
}

.ds-prereq-link {
  font-size: 12px;
  color: var(--td-brand-color);
  padding-left: 30px;
}

/* --- Step 1: doc link & form --- */
.ds-doc-link {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-component);
  padding: 8px 12px;
  border-radius: 6px;
  margin-bottom: 16px;
}

.ds-doc-link a {
  color: var(--td-brand-color);
  word-break: break-all;
}

.form-item { margin-bottom: 16px; }
.form-label { display: block; font-size: 13px; font-weight: 500; margin-bottom: 6px; color: var(--td-text-color-primary); }
.required-mark { color: var(--td-error-color); margin-left: 2px; }
.form-tip { font-size: 12px; color: var(--td-text-color-placeholder); margin: 4px 0 12px; }
.form-hint { font-size: 12px; color: var(--td-text-color-placeholder); margin-top: 6px; line-height: 1.5; }
.ds-db-advanced { margin: 16px 0 8px; }
.ds-db-advanced-title { font-size: 14px; font-weight: 600; color: var(--td-text-color-primary); }
.ds-db-advanced-desc { margin-top: 4px; font-size: 12px; line-height: 18px; color: var(--td-text-color-placeholder); }
.ds-db-limit-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.form-actions { display: flex; align-items: center; gap: 8px; margin-top: 12px; }
.test-ok { color: var(--td-success-color); font-size: 13px; display: flex; align-items: center; gap: 4px; }

.test-error-box {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-top: 10px;
  padding: 10px 14px;
  border-radius: 8px;
  background: var(--td-error-color-1);
  color: var(--td-error-color);
  font-size: 13px;
  line-height: 20px;
}

.test-error-content {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.test-error-title {
  font-weight: 500;
}

.test-error-detail {
  font-size: 12px;
  color: var(--td-error-color);
  opacity: 0.8;
  word-break: break-word;
}

.ds-dialog-footer { display: flex; justify-content: flex-end; gap: 8px; margin-top: 24px; padding-top: 16px; border-top: 1px solid var(--td-border-level-2-color); }

/* --- Step 2: resource list --- */
.ds-resource-list { max-height: 400px; overflow-y: auto; display: flex; flex-direction: column; gap: 2px; }

.ds-expand-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 4px;
  cursor: pointer;
  color: var(--td-text-color-secondary);
  flex-shrink: 0;
  transition: background 0.15s;
}
.ds-expand-btn:hover { background: var(--td-bg-color-component-hover); }
.ds-expand-placeholder { width: 20px; flex-shrink: 0; }

.ds-resource-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid transparent;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s;
}

.ds-resource-row:hover {
  background: var(--td-bg-color-container-hover);
}

.ds-resource-row.selected {
  border-color: var(--td-brand-color);
  background: none;
}

.ds-resource-info {
  flex: 1;
  min-width: 0;
}

.ds-resource-name {
  font-size: 14px;
  font-weight: 500;
  color: var(--td-text-color-primary);
  line-height: 1.4;
}

.ds-resource-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 2px;
}

.ds-resource-type {
  font-size: 11px;
  padding: 0 5px;
  border-radius: 3px;
  background: var(--td-bg-color-component);
  color: var(--td-text-color-placeholder);
  line-height: 18px;
}

.ds-resource-desc {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* --- Step 2: empty state --- */
.ds-resource-empty {
  text-align: center;
  padding: 24px 0;
}

.ds-empty-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin: 0 0 4px;
}

.ds-empty-desc {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  margin: 0 0 16px;
}

.ds-guide-steps {
  display: flex;
  flex-direction: column;
  gap: 8px;
  text-align: left;
  max-width: 440px;
  margin: 0 auto 16px;
}

.ds-guide-step {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 13px;
  color: var(--td-text-color-primary);
  line-height: 1.5;
}

.ds-guide-num {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: var(--td-brand-color-light);
  color: var(--td-brand-color);
  font-size: 11px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  margin-top: 1px;
}

.ds-empty-actions {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
}

.ds-doc-link-inline {
  color: var(--td-brand-color);
  font-size: 12px;
}

@media (max-width: 680px) {
  .ds-db-limit-grid {
    grid-template-columns: 1fr;
  }
}
</style>
