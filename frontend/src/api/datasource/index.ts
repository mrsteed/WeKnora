import { get, post, put, del } from '../../utils/request'

// --- Types ---

export interface DataSource {
  id: string
  tenant_id: number
  knowledge_base_id: string
  name: string
  type: string
  config: DataSourceConfig
  sync_schedule: string
  sync_mode: 'incremental' | 'full'
  status: 'active' | 'paused' | 'error'
  conflict_strategy: 'overwrite' | 'skip'
  sync_deletions: boolean
  last_sync_at: string | null
  last_sync_result: any
  error_message: string
  created_at: string
  updated_at: string
  latest_sync_log?: SyncLog
}

export interface SyncLog {
  id: string
  data_source_id: string
  status: 'running' | 'success' | 'partial' | 'failed' | 'canceled'
  started_at: string
  finished_at: string | null
  items_total: number
  items_created: number
  items_updated: number
  items_deleted: number
  items_skipped: number
  items_failed: number
  error_message: string
}

export interface ConnectorMeta {
  type: string
  name: string
  description: string
  icon: string
  priority: number
  auth_type: string
  capabilities: string[]
}

export interface Resource {
  external_id: string
  name: string
  type: string
  description: string
  url: string
  parent_id?: string
  has_children?: boolean
}

export interface DatabaseCredentials {
  username: string
  password?: string
}

export interface DatabaseSourceSettings {
  host: string
  port: number
  database: string
  schema?: string
  ssl_mode?: string
  table_allowlist?: string[]
  column_denylist?: string[]
  max_rows?: number
  query_timeout_sec?: number
  sample_rows?: number
  schema_refresh_cron?: string
}

export interface DatabaseConnectionConfig {
  type?: string
  credentials: DatabaseCredentials
  settings: DatabaseSourceSettings
  resource_ids?: string[]
}

export interface GenericDataSourceConfig {
  credentials?: Record<string, any>
  settings?: Record<string, any>
  resource_ids?: string[]
}

export type DataSourceConfig = DatabaseConnectionConfig | GenericDataSourceConfig | Record<string, any>

export interface DatabaseSchemaColumn {
  name: string
  data_type: string
  nullable: boolean
  comment?: string
  is_sensitive: boolean
  sample_values?: string[]
}

export interface DatabaseSchemaIndex {
  name: string
  unique: boolean
  columns?: string[]
  index_type?: string
}

export interface DatabaseSchemaTable {
  name: string
  type: string
  comment?: string
  row_estimate?: number
  columns?: DatabaseSchemaColumn[]
  primary_keys?: string[]
  indexes?: DatabaseSchemaIndex[]
}

export interface DatabaseSchema {
  id?: string
  tenant_id?: number
  knowledge_base_id?: string
  data_source_id?: string
  database_type: string
  database_name: string
  schema_name?: string
  schema_hash?: string
  refreshed_at?: string
  tables?: DatabaseSchemaTable[]
}

export interface DatabaseQueryAuditLog {
  id: string
  tenant_id: number
  user_id: string
  session_id?: string
  knowledge_base_id: string
  data_source_id: string
  original_sql: string
  executed_sql?: string
  purpose?: string
  status: 'success' | 'failed' | 'rejected'
  row_count: number
  duration_ms: number
  error_message?: string
  created_at: string
}

export interface DatabaseQueryAuditListResponse {
  items: DatabaseQueryAuditLog[]
  total: number
  limit: number
  offset: number
}

// --- API calls ---

export function getConnectorTypes() {
  return get('/api/v1/datasource/types')
}

export function listDataSources(kbId: string) {
  return get(`/api/v1/datasource?kb_id=${encodeURIComponent(kbId)}`)
}

export function getDataSource(id: string) {
  return get(`/api/v1/datasource/${id}`)
}

export function createDataSource(data: Partial<DataSource>) {
  return post('/api/v1/datasource', data)
}

export function updateDataSource(id: string, data: Partial<DataSource>) {
  return put(`/api/v1/datasource/${id}`, data)
}

export function deleteDataSource(id: string) {
  return del(`/api/v1/datasource/${id}`)
}

export function validateConnection(id: string) {
  return post(`/api/v1/datasource/${id}/validate`, {})
}

// Validate credentials without persisting (for "Test Connection" during creation)
export function validateCredentials(type: string, credentials: Record<string, any>, settings?: Record<string, any>) {
  return post('/api/v1/datasource/validate-credentials', { type, credentials, settings })
}

export function listResources(id: string) {
  return get(`/api/v1/datasource/${id}/resources`)
}

export function triggerSync(id: string) {
  return post(`/api/v1/datasource/${id}/sync`, {})
}

export function pauseDataSource(id: string) {
  return post(`/api/v1/datasource/${id}/pause`, {})
}

export function resumeDataSource(id: string) {
  return post(`/api/v1/datasource/${id}/resume`, {})
}

export function getSyncLogs(id: string, limit = 20, offset = 0) {
  return get(`/api/v1/datasource/${id}/logs?limit=${limit}&offset=${offset}`)
}

export function refreshDataSourceSchema(id: string) {
  return post<DatabaseSchema>(`/api/v1/datasource/${id}/refresh-schema`, {})
}

export function getDatabaseSchema(kbId: string) {
  return get<DatabaseSchema>(`/api/v1/knowledge-bases/${kbId}/database-schema`)
}

export function listDatabaseQueryAudits(kbId?: string, limit = 20, offset = 0) {
  const params = new URLSearchParams()
  if (kbId) params.set('knowledge_base_id', kbId)
  params.set('limit', String(limit))
  params.set('offset', String(offset))
  return get<DatabaseQueryAuditListResponse>(`/api/v1/database-query-audits?${params.toString()}`)
}
