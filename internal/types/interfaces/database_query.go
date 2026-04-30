package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// SchemaRegistryService manages persisted database schema snapshots and
// produces prompt-friendly schema summaries for agent tools.
type SchemaRegistryService interface {
	RefreshSchema(ctx context.Context, dataSourceID string) error
	GetDatabaseSchema(ctx context.Context, kbID string) (*types.DatabaseSchema, error)
	GetTableSchema(ctx context.Context, kbID string, tableName string) (*types.TableSchema, error)
	BuildPromptSchema(ctx context.Context, kbID string, selectedTables []string) (string, error)
}

// StructuredQueryService validates and executes realtime external database
// queries for database knowledge bases.
type StructuredQueryService interface {
	ValidateSQL(ctx context.Context, req types.ValidateSQLRequest) (*types.ValidatedSQL, error)
	ExecuteQuery(ctx context.Context, req types.ExecuteQueryRequest) (*types.QueryResult, error)
	ExplainQuery(ctx context.Context, req types.ExplainQueryRequest) (*types.QueryPlan, error)
}

// DatabaseSchemaRepository persists and retrieves effective schema snapshots
// for database-backed knowledge bases.
type DatabaseSchemaRepository interface {
	// ReplaceSnapshot stores a freshly refreshed schema snapshot and replaces the
	// currently effective snapshot/column set for the same datasource.
	ReplaceSnapshot(
		ctx context.Context,
		snapshot *types.DatabaseSchemaSnapshot,
		columns []*types.DatabaseTableColumn,
	) error

	// GetLatestSnapshotByKnowledgeBase returns the effective snapshot for the
	// given knowledge base, excluding soft-deleted datasources.
	GetLatestSnapshotByKnowledgeBase(
		ctx context.Context,
		tenantID uint64,
		knowledgeBaseID string,
	) (*types.DatabaseSchemaSnapshot, error)

	// GetLatestSnapshotByDataSource returns the effective snapshot for one
	// datasource, excluding soft-deleted datasources.
	GetLatestSnapshotByDataSource(
		ctx context.Context,
		tenantID uint64,
		dataSourceID string,
	) (*types.DatabaseSchemaSnapshot, error)

	// ListColumnsByKnowledgeBase returns the effective flattened columns for one
	// knowledge base, excluding soft-deleted datasources.
	ListColumnsByKnowledgeBase(
		ctx context.Context,
		tenantID uint64,
		knowledgeBaseID string,
	) ([]*types.DatabaseTableColumn, error)

	// ListColumnsByTable returns the effective flattened columns for one table.
	ListColumnsByTable(
		ctx context.Context,
		tenantID uint64,
		knowledgeBaseID string,
		tableName string,
	) ([]*types.DatabaseTableColumn, error)
}

// DatabaseQueryAuditRepository persists and pages external database query audit
// logs.
type DatabaseQueryAuditRepository interface {
	Create(ctx context.Context, log *types.DatabaseQueryAuditLog) error
	ListByTenant(
		ctx context.Context,
		tenantID uint64,
		knowledgeBaseID string,
		limit int,
		offset int,
	) ([]*types.DatabaseQueryAuditLog, error)
	CountByTenant(ctx context.Context, tenantID uint64, knowledgeBaseID string) (int64, error)
}
