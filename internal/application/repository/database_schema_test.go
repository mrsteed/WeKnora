package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const databaseSchemaTestDDL = `
CREATE TABLE IF NOT EXISTS data_sources (
    id VARCHAR(36) NOT NULL PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,
    config TEXT,
    sync_schedule VARCHAR(100),
    sync_mode VARCHAR(20),
    status VARCHAR(32),
    conflict_strategy VARCHAR(32),
    sync_deletions BOOLEAN,
    last_sync_at DATETIME NULL,
    last_sync_cursor TEXT,
    last_sync_result TEXT,
    error_message TEXT,
    sync_log_retention_days INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME NULL
);

CREATE TABLE IF NOT EXISTS database_schema_snapshots (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    data_source_id VARCHAR(36) NOT NULL,
    database_type VARCHAR(32) NOT NULL,
    database_name VARCHAR(255) NOT NULL,
    schema_name VARCHAR(255),
    schema_hash VARCHAR(128) NOT NULL,
    schema_json TEXT NOT NULL,
    refreshed_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_db_schema_snapshots_active_ds
	ON database_schema_snapshots (tenant_id, data_source_id)
	WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS database_table_columns (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    data_source_id VARCHAR(36) NOT NULL,
    table_name VARCHAR(255) NOT NULL,
    column_name VARCHAR(255) NOT NULL,
    data_type VARCHAR(128) NOT NULL,
    nullable BOOLEAN NOT NULL DEFAULT 1,
    comment TEXT,
    is_sensitive BOOLEAN NOT NULL DEFAULT 0,
    ordinal_position INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME NULL
);

CREATE TABLE IF NOT EXISTS database_query_audit_logs (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    user_id VARCHAR(36) NOT NULL,
    session_id VARCHAR(36),
    knowledge_base_id VARCHAR(36) NOT NULL,
    data_source_id VARCHAR(36) NOT NULL,
    original_sql TEXT NOT NULL,
    executed_sql TEXT,
    purpose TEXT,
    status VARCHAR(32) NOT NULL,
    row_count INTEGER NOT NULL DEFAULT 0,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    error_message TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

func setupDatabaseSchemaRepoDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(databaseSchemaTestDDL).Error)
	return db
}

func newSchemaTestDataSource() *types.DataSource {
	return &types.DataSource{
		ID:              uuid.New().String(),
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		Name:            "mysql-prod",
		Type:            types.DatabaseTypeMySQL,
		Status:          types.DataSourceStatusActive,
	}
}

func newSnapshot(t *testing.T, refreshedAt time.Time, hash string, table string) *types.DatabaseSchemaSnapshot {
	t.Helper()
	snapshot := &types.DatabaseSchemaSnapshot{
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		DatabaseType:    types.DatabaseTypeMySQL,
		DatabaseName:    "crm",
		SchemaHash:      hash,
		RefreshedAt:     refreshedAt,
	}
	require.NoError(t, snapshot.SetSchema(&types.DatabaseSchema{
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		DatabaseType:    types.DatabaseTypeMySQL,
		DatabaseName:    "crm",
		SchemaHash:      hash,
		RefreshedAt:     refreshedAt,
		Tables: []types.TableSchema{{
			Name: table,
			Type: "table",
			Columns: []types.ColumnSchema{{
				Name:     "id",
				DataType: "bigint",
			}},
		}},
	}))
	return snapshot
}

func TestDatabaseSchemaRepositoryReplaceSnapshotKeepsLatestEffectiveSnapshot(t *testing.T) {
	ctx := context.Background()
	db := setupDatabaseSchemaRepoDB(t)
	repo := NewDatabaseSchemaRepository(db)
	ds := newSchemaTestDataSource()
	require.NoError(t, db.Create(ds).Error)

	firstRefreshedAt := time.Now().UTC().Add(-time.Hour)
	first := newSnapshot(t, firstRefreshedAt, "hash-v1", "orders")
	first.DataSourceID = ds.ID
	require.NoError(t, first.SetSchema(&types.DatabaseSchema{
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		DataSourceID:    ds.ID,
		DatabaseType:    types.DatabaseTypeMySQL,
		DatabaseName:    "crm",
		SchemaHash:      "hash-v1",
		RefreshedAt:     firstRefreshedAt,
		Tables:          []types.TableSchema{{Name: "orders", Type: "table"}},
	}))
	columnsV1 := []*types.DatabaseTableColumn{{
		Table:           "orders",
		ColumnName:      "id",
		DataType:        "bigint",
		Nullable:        false,
		OrdinalPosition: 1,
	}}
	require.NoError(t, repo.ReplaceSnapshot(ctx, first, columnsV1))

	secondRefreshedAt := time.Now().UTC()
	second := &types.DatabaseSchemaSnapshot{
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		DataSourceID:    ds.ID,
		DatabaseType:    types.DatabaseTypeMySQL,
		DatabaseName:    "crm",
		SchemaHash:      "hash-v2",
		RefreshedAt:     secondRefreshedAt,
	}
	require.NoError(t, second.SetSchema(&types.DatabaseSchema{
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		DataSourceID:    ds.ID,
		DatabaseType:    types.DatabaseTypeMySQL,
		DatabaseName:    "crm",
		SchemaHash:      "hash-v2",
		RefreshedAt:     secondRefreshedAt,
		Tables:          []types.TableSchema{{Name: "customers", Type: "table"}},
	}))
	columnsV2 := []*types.DatabaseTableColumn{{
		Table:           "customers",
		ColumnName:      "customer_id",
		DataType:        "varchar",
		Nullable:        false,
		OrdinalPosition: 1,
	}}
	require.NoError(t, repo.ReplaceSnapshot(ctx, second, columnsV2))

	latest, err := repo.GetLatestSnapshotByKnowledgeBase(ctx, 1, "kb-1")
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, second.ID, latest.ID)
	assert.Equal(t, "hash-v2", latest.SchemaHash)

	columns, err := repo.ListColumnsByKnowledgeBase(ctx, 1, "kb-1")
	require.NoError(t, err)
	require.Len(t, columns, 1)
	assert.Equal(t, "customers", columns[0].Table)
	assert.Equal(t, "customer_id", columns[0].ColumnName)

	var activeSnapshots int64
	require.NoError(t, db.Model(&types.DatabaseSchemaSnapshot{}).
		Where("knowledge_base_id = ? AND data_source_id = ? AND deleted_at IS NULL", "kb-1", ds.ID).
		Count(&activeSnapshots).Error)
	assert.Equal(t, int64(1), activeSnapshots)
}

func TestDatabaseSchemaRepositorySkipsSoftDeletedDataSource(t *testing.T) {
	ctx := context.Background()
	db := setupDatabaseSchemaRepoDB(t)
	repo := NewDatabaseSchemaRepository(db)
	ds := newSchemaTestDataSource()
	require.NoError(t, db.Create(ds).Error)

	snapshot := &types.DatabaseSchemaSnapshot{
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		DataSourceID:    ds.ID,
		DatabaseType:    types.DatabaseTypeMySQL,
		DatabaseName:    "crm",
		SchemaHash:      "hash-v1",
		RefreshedAt:     time.Now().UTC(),
	}
	require.NoError(t, snapshot.SetSchema(&types.DatabaseSchema{DatabaseName: "crm", DatabaseType: types.DatabaseTypeMySQL}))
	require.NoError(t, repo.ReplaceSnapshot(ctx, snapshot, []*types.DatabaseTableColumn{{
		Table:           "orders",
		ColumnName:      "id",
		DataType:        "bigint",
		OrdinalPosition: 1,
	}}))

	now := time.Now().UTC()
	require.NoError(t, db.Model(&types.DataSource{}).Where("id = ?", ds.ID).Update("deleted_at", now).Error)

	latest, err := repo.GetLatestSnapshotByKnowledgeBase(ctx, 1, "kb-1")
	require.NoError(t, err)
	assert.Nil(t, latest)

	columns, err := repo.ListColumnsByKnowledgeBase(ctx, 1, "kb-1")
	require.NoError(t, err)
	assert.Empty(t, columns)
}

func TestDatabaseSchemaRepositoryReplaceSnapshotRollsBackWhenNewColumnsInvalid(t *testing.T) {
	ctx := context.Background()
	db := setupDatabaseSchemaRepoDB(t)
	repo := NewDatabaseSchemaRepository(db)
	ds := newSchemaTestDataSource()
	require.NoError(t, db.Create(ds).Error)

	first := newSnapshot(t, time.Now().UTC().Add(-time.Hour), "hash-v1", "orders")
	first.DataSourceID = ds.ID
	require.NoError(t, first.SetSchema(&types.DatabaseSchema{
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		DataSourceID:    ds.ID,
		DatabaseType:    types.DatabaseTypeMySQL,
		DatabaseName:    "crm",
		SchemaHash:      "hash-v1",
		RefreshedAt:     first.RefreshedAt,
		Tables:          []types.TableSchema{{Name: "orders", Type: "table"}},
	}))
	require.NoError(t, repo.ReplaceSnapshot(ctx, first, []*types.DatabaseTableColumn{{
		Table:           "orders",
		ColumnName:      "id",
		DataType:        "bigint",
		OrdinalPosition: 1,
	}}))

	broken := &types.DatabaseSchemaSnapshot{
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		DataSourceID:    ds.ID,
		DatabaseType:    types.DatabaseTypeMySQL,
		DatabaseName:    "crm",
		SchemaHash:      "hash-v2",
		RefreshedAt:     time.Now().UTC(),
	}
	require.NoError(t, broken.SetSchema(&types.DatabaseSchema{
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		DataSourceID:    ds.ID,
		DatabaseType:    types.DatabaseTypeMySQL,
		DatabaseName:    "crm",
		SchemaHash:      "hash-v2",
		RefreshedAt:     broken.RefreshedAt,
		Tables:          []types.TableSchema{{Name: "customers", Type: "table"}},
	}))

	err := repo.ReplaceSnapshot(ctx, broken, []*types.DatabaseTableColumn{nil})
	require.Error(t, err)

	latest, getErr := repo.GetLatestSnapshotByKnowledgeBase(ctx, 1, "kb-1")
	require.NoError(t, getErr)
	require.NotNil(t, latest)
	assert.Equal(t, first.ID, latest.ID)
	assert.Equal(t, "hash-v1", latest.SchemaHash)

	columns, listErr := repo.ListColumnsByKnowledgeBase(ctx, 1, "kb-1")
	require.NoError(t, listErr)
	require.Len(t, columns, 1)
	assert.Equal(t, "orders", columns[0].Table)
	assert.Equal(t, "id", columns[0].ColumnName)
}

func TestDatabaseSchemaRepositoryRejectsEmptySchemaJSON(t *testing.T) {
	ctx := context.Background()
	db := setupDatabaseSchemaRepoDB(t)
	repo := NewDatabaseSchemaRepository(db)
	ds := newSchemaTestDataSource()
	require.NoError(t, db.Create(ds).Error)

	err := repo.ReplaceSnapshot(ctx, &types.DatabaseSchemaSnapshot{
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		DataSourceID:    ds.ID,
		DatabaseType:    types.DatabaseTypeMySQL,
		DatabaseName:    "crm",
		SchemaHash:      "hash-v1",
		RefreshedAt:     time.Now().UTC(),
	}, nil)
	require.EqualError(t, err, "schema json is required")
}

func TestDatabaseSchemaRepositoryRejectsMismatchedDataSource(t *testing.T) {
	ctx := context.Background()
	db := setupDatabaseSchemaRepoDB(t)
	repo := NewDatabaseSchemaRepository(db)
	ds := newSchemaTestDataSource()
	ds.KnowledgeBaseID = "kb-other"
	require.NoError(t, db.Create(ds).Error)

	snapshot := &types.DatabaseSchemaSnapshot{
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		DataSourceID:    ds.ID,
		DatabaseType:    types.DatabaseTypeMySQL,
		DatabaseName:    "crm",
		SchemaHash:      "hash-v1",
		RefreshedAt:     time.Now().UTC(),
	}
	require.NoError(t, snapshot.SetSchema(&types.DatabaseSchema{
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		DataSourceID:    ds.ID,
		DatabaseType:    types.DatabaseTypeMySQL,
		DatabaseName:    "crm",
		SchemaHash:      "hash-v1",
		RefreshedAt:     snapshot.RefreshedAt,
	}))

	err := repo.ReplaceSnapshot(ctx, snapshot, nil)
	require.EqualError(t, err, "active data source not found for schema snapshot")
}

func TestDatabaseQueryAuditRepositoryCreateAndList(t *testing.T) {
	ctx := context.Background()
	db := setupDatabaseSchemaRepoDB(t)
	repo := NewDatabaseQueryAuditRepository(db)

	require.NoError(t, repo.Create(ctx, &types.DatabaseQueryAuditLog{
		TenantID:        1,
		UserID:          "user-1",
		SessionID:       "session-1",
		KnowledgeBaseID: "kb-1",
		DataSourceID:    "ds-1",
		OriginalSQL:     "SELECT 1",
		ExecutedSQL:     "SELECT 1 LIMIT 1",
		Purpose:         "smoke test",
		Status:          types.DatabaseQueryAuditStatusSuccess,
		RowCount:        1,
		DurationMS:      12,
	}))

	logs, err := repo.ListByTenant(ctx, 1, "kb-1", 10, 0)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, "SELECT 1", logs[0].OriginalSQL)

	count, err := repo.CountByTenant(ctx, 1, "kb-1")
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestDatabaseQueryAuditRepositoryCreateRejectsMissingRequiredFields(t *testing.T) {
	ctx := context.Background()
	db := setupDatabaseSchemaRepoDB(t)
	repo := NewDatabaseQueryAuditRepository(db)

	err := repo.Create(ctx, &types.DatabaseQueryAuditLog{})
	require.EqualError(t, err, "tenant id is required")
}
