package types

import "time"

// SQLDialect identifies the SQL dialect used by an external database connector.
// The value is carried through validation and execution layers so later services
// can apply dialect-aware SQL guards and error handling without guessing from
// the datasource type string again.
type SQLDialect string

const (
	SQLDialectMySQL      SQLDialect = "mysql"
	SQLDialectPostgreSQL SQLDialect = "postgresql"
)

// DatabaseSchema represents a discovered snapshot of an external database's
// logical structure. The service layer will later persist this DTO into schema
// snapshot tables, but the connector layer already needs a stable in-memory
// shape to return tables, columns, primary keys and indexes.
type DatabaseSchema struct {
	ID                string        `json:"id,omitempty"`
	TenantID          uint64        `json:"tenant_id,omitempty"`
	KnowledgeBaseID   string        `json:"knowledge_base_id,omitempty"`
	DataSourceID      string        `json:"data_source_id,omitempty"`
	DatabaseType      string        `json:"database_type"`
	DatabaseName      string        `json:"database_name"`
	SchemaName        string        `json:"schema_name,omitempty"`
	Tables            []TableSchema `json:"tables,omitempty"`
	BusinessJoinHints []string      `json:"business_join_hints,omitempty"`
	SchemaHash        string        `json:"schema_hash,omitempty"`
	RefreshedAt       time.Time     `json:"refreshed_at,omitempty"`
}

// TableSchema represents a single table or view discovered from the external
// database. RowEstimate is best-effort metadata and may be zero when the
// underlying database does not expose reliable statistics to the current user.
type TableSchema struct {
	Name        string             `json:"name"`
	Type        string             `json:"type"`
	Comment     string             `json:"comment,omitempty"`
	RowEstimate int64              `json:"row_estimate,omitempty"`
	Columns     []ColumnSchema     `json:"columns,omitempty"`
	PrimaryKeys []string           `json:"primary_keys,omitempty"`
	ForeignKeys []ForeignKeySchema `json:"foreign_keys,omitempty"`
	Indexes     []IndexSchema      `json:"indexes,omitempty"`
}

type ForeignKeySchema struct {
	Name              string   `json:"name,omitempty"`
	Columns           []string `json:"columns,omitempty"`
	ReferencedTable   string   `json:"referenced_table"`
	ReferencedColumns []string `json:"referenced_columns,omitempty"`
}

// ColumnSchema represents one column exposed by a discovered table.
// IsSensitive is not inferred by connectors yet; later policy layers can set it
// after applying deny-lists or masking rules.
type ColumnSchema struct {
	Name         string   `json:"name"`
	DataType     string   `json:"data_type"`
	Nullable     bool     `json:"nullable"`
	Comment      string   `json:"comment,omitempty"`
	IsSensitive  bool     `json:"is_sensitive"`
	SampleValues []string `json:"sample_values,omitempty"`
}

// IndexSchema represents a discovered index definition. Columns preserve index
// order so later SQL guards or prompt builders can reason about likely join and
// filter paths without reparsing raw catalog rows.
type IndexSchema struct {
	Name      string   `json:"name"`
	Unique    bool     `json:"unique"`
	Columns   []string `json:"columns,omitempty"`
	IndexType string   `json:"index_type,omitempty"`
}

type PromptSchemaMode string

const (
	PromptSchemaModeAuto    PromptSchemaMode = "auto"
	PromptSchemaModeCatalog PromptSchemaMode = "catalog"
	PromptSchemaModeDetail  PromptSchemaMode = "detail"
)

type PromptSchemaOptions struct {
	Mode PromptSchemaMode `json:"mode,omitempty"`
}

type PromptSchemaBuildResult struct {
	Prompt                   string           `json:"prompt"`
	Mode                     PromptSchemaMode `json:"mode"`
	DatabaseName             string           `json:"database_name,omitempty"`
	SchemaName               string           `json:"schema_name,omitempty"`
	SchemaHash               string           `json:"schema_hash,omitempty"`
	RefreshedAt              time.Time        `json:"refreshed_at,omitempty"`
	AllTables                []TableSchema    `json:"all_tables,omitempty"`
	DisplayTables            []TableSchema    `json:"display_tables,omitempty"`
	PossibleJoinHints        []string         `json:"possible_join_hints,omitempty"`
	TableCount               int              `json:"table_count"`
	ColumnCount              int              `json:"column_count"`
	AdditionalTablesOmitted  int              `json:"additional_tables_omitted"`
	AdditionalColumnsOmitted int              `json:"additional_columns_omitted"`
}
