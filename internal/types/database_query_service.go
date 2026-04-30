package types

import "time"

// ValidateSQLRequest carries the minimum inputs required to validate one
// candidate SQL statement for a database knowledge base before execution.
// MaxRows and TimeoutSeconds are optional per-call overrides that are clamped
// against datasource policy defaults inside the query service.
type ValidateSQLRequest struct {
	TenantID        uint64 `json:"tenant_id,omitempty"`
	KnowledgeBaseID string `json:"knowledge_base_id"`
	SQL             string `json:"sql"`
	MaxRows         int    `json:"max_rows,omitempty"`
	TimeoutSeconds  int    `json:"timeout_seconds,omitempty"`
}

// ExecuteQueryRequest extends validation input with caller identity and purpose
// so successful, failed and rejected queries can be audited consistently.
type ExecuteQueryRequest struct {
	TenantID        uint64 `json:"tenant_id,omitempty"`
	UserID          string `json:"user_id,omitempty"`
	SessionID       string `json:"session_id,omitempty"`
	KnowledgeBaseID string `json:"knowledge_base_id"`
	SQL             string `json:"sql"`
	Purpose         string `json:"purpose,omitempty"`
	MaxRows         int    `json:"max_rows,omitempty"`
	TimeoutSeconds  int    `json:"timeout_seconds,omitempty"`
}

// ExplainQueryRequest reuses the same query constraints as validation, but is
// intended for future query-plan inspection without executing the statement.
type ExplainQueryRequest struct {
	TenantID        uint64 `json:"tenant_id,omitempty"`
	KnowledgeBaseID string `json:"knowledge_base_id"`
	SQL             string `json:"sql"`
	MaxRows         int    `json:"max_rows,omitempty"`
	TimeoutSeconds  int    `json:"timeout_seconds,omitempty"`
}

// ValidatedSQL describes the effective execution envelope after SQLGuard has
// enforced statement safety, table/column permissions and runtime limits.
type ValidatedSQL struct {
	OriginalSQL   string        `json:"original_sql"`
	ExecutedSQL   string        `json:"executed_sql"`
	NormalizedSQL string        `json:"normalized_sql"`
	Dialect       SQLDialect    `json:"dialect"`
	Tables        []string      `json:"tables,omitempty"`
	SelectFields  []string      `json:"select_fields,omitempty"`
	MaxRows       int           `json:"max_rows"`
	Timeout       time.Duration `json:"timeout"`
}

// QueryColumn describes one returned column from an external database query.
// DataType uses the driver-reported database type name when available.
type QueryColumn struct {
	Name     string `json:"name"`
	DataType string `json:"data_type,omitempty"`
}

// QueryResult is the structured payload returned to later tool and handler
// layers. DisplayType is aligned with the external database tool contract.
type QueryResult struct {
	Columns     []QueryColumn    `json:"columns,omitempty"`
	Rows        []map[string]any `json:"rows,omitempty"`
	RowCount    int              `json:"row_count"`
	Truncated   bool             `json:"truncated"`
	ExecutedSQL string           `json:"executed_sql"`
	DurationMS  int64            `json:"duration_ms"`
	DisplayType string           `json:"display_type"`
}

// QueryPlan is a lightweight MVP explain response. The current implementation
// reports the validated execution envelope without issuing EXPLAIN remotely.
type QueryPlan struct {
	SQL            string     `json:"sql"`
	Dialect        SQLDialect `json:"dialect"`
	Tables         []string   `json:"tables,omitempty"`
	MaxRows        int        `json:"max_rows"`
	TimeoutSeconds int        `json:"timeout_seconds"`
	Notes          []string   `json:"notes,omitempty"`
}
