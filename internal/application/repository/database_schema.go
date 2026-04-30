package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type databaseSchemaRepository struct {
	db *gorm.DB
}

func NewDatabaseSchemaRepository(db *gorm.DB) interfaces.DatabaseSchemaRepository {
	return &databaseSchemaRepository{db: db}
}

func (r *databaseSchemaRepository) ReplaceSnapshot(
	ctx context.Context,
	snapshot *types.DatabaseSchemaSnapshot,
	columns []*types.DatabaseTableColumn,
) error {
	if snapshot == nil {
		return errors.New("database schema snapshot is nil")
	}
	if snapshot.TenantID == 0 {
		return errors.New("tenant id is required")
	}
	if snapshot.KnowledgeBaseID == "" {
		return errors.New("knowledge base id is required")
	}
	if snapshot.DataSourceID == "" {
		return errors.New("data source id is required")
	}
	if snapshot.DatabaseType == "" {
		return errors.New("database type is required")
	}
	if snapshot.DatabaseName == "" {
		return errors.New("database name is required")
	}
	if snapshot.SchemaHash == "" {
		return errors.New("schema hash is required")
	}
	if len(snapshot.SchemaJSON) == 0 {
		return errors.New("schema json is required")
	}
	if snapshot.RefreshedAt.IsZero() {
		snapshot.RefreshedAt = time.Now().UTC()
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var dataSource types.DataSource
		if err := tx.Model(&types.DataSource{}).
			Where("id = ?", snapshot.DataSourceID).
			Where("tenant_id = ?", snapshot.TenantID).
			Where("knowledge_base_id = ?", snapshot.KnowledgeBaseID).
			Where("deleted_at IS NULL").
			First(&dataSource).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("active data source not found for schema snapshot")
			}
			return err
		}

		now := time.Now().UTC()
		if err := tx.Model(&types.DatabaseSchemaSnapshot{}).
			Where("tenant_id = ? AND knowledge_base_id = ? AND data_source_id = ? AND id <> ? AND deleted_at IS NULL",
				snapshot.TenantID, snapshot.KnowledgeBaseID, snapshot.DataSourceID, snapshot.ID).
			Update("deleted_at", now).Error; err != nil {
			return err
		}

		if err := tx.Model(&types.DatabaseTableColumn{}).
			Where("tenant_id = ? AND knowledge_base_id = ? AND data_source_id = ? AND deleted_at IS NULL",
				snapshot.TenantID, snapshot.KnowledgeBaseID, snapshot.DataSourceID).
			Update("deleted_at", now).Error; err != nil {
			return err
		}

		if err := tx.Create(snapshot).Error; err != nil {
			return err
		}

		if len(columns) == 0 {
			return nil
		}

		for _, column := range columns {
			if column == nil {
				return errors.New("database table column is nil")
			}
			column.TenantID = snapshot.TenantID
			column.KnowledgeBaseID = snapshot.KnowledgeBaseID
			column.DataSourceID = snapshot.DataSourceID
		}

		return tx.Create(&columns).Error
	})
}

func (r *databaseSchemaRepository) GetLatestSnapshotByKnowledgeBase(
	ctx context.Context,
	tenantID uint64,
	knowledgeBaseID string,
) (*types.DatabaseSchemaSnapshot, error) {
	if knowledgeBaseID == "" {
		return nil, errors.New("knowledge base id is empty")
	}
	var snapshots []*types.DatabaseSchemaSnapshot
	err := r.baseSnapshotQuery(ctx).
		Where("database_schema_snapshots.tenant_id = ?", tenantID).
		Where("database_schema_snapshots.knowledge_base_id = ?", knowledgeBaseID).
		Order("database_schema_snapshots.refreshed_at DESC").
		Order("database_schema_snapshots.created_at DESC").
		Limit(1).
		Find(&snapshots).Error
	if err != nil {
		return nil, err
	}
	if len(snapshots) == 0 {
		return nil, nil
	}
	return snapshots[0], nil
}

func (r *databaseSchemaRepository) GetLatestSnapshotByDataSource(
	ctx context.Context,
	tenantID uint64,
	dataSourceID string,
) (*types.DatabaseSchemaSnapshot, error) {
	if dataSourceID == "" {
		return nil, errors.New("data source id is empty")
	}
	var snapshots []*types.DatabaseSchemaSnapshot
	err := r.baseSnapshotQuery(ctx).
		Where("database_schema_snapshots.tenant_id = ?", tenantID).
		Where("database_schema_snapshots.data_source_id = ?", dataSourceID).
		Order("database_schema_snapshots.refreshed_at DESC").
		Order("database_schema_snapshots.created_at DESC").
		Limit(1).
		Find(&snapshots).Error
	if err != nil {
		return nil, err
	}
	if len(snapshots) == 0 {
		return nil, nil
	}
	return snapshots[0], nil
}

func (r *databaseSchemaRepository) ListColumnsByKnowledgeBase(
	ctx context.Context,
	tenantID uint64,
	knowledgeBaseID string,
) ([]*types.DatabaseTableColumn, error) {
	if knowledgeBaseID == "" {
		return nil, errors.New("knowledge base id is empty")
	}
	var columns []*types.DatabaseTableColumn
	err := r.baseColumnQuery(ctx).
		Where("database_table_columns.tenant_id = ?", tenantID).
		Where("database_table_columns.knowledge_base_id = ?", knowledgeBaseID).
		Order("database_table_columns.table_name ASC").
		Order("database_table_columns.ordinal_position ASC").
		Find(&columns).Error
	return columns, err
}

func (r *databaseSchemaRepository) ListColumnsByTable(
	ctx context.Context,
	tenantID uint64,
	knowledgeBaseID string,
	tableName string,
) ([]*types.DatabaseTableColumn, error) {
	if knowledgeBaseID == "" {
		return nil, errors.New("knowledge base id is empty")
	}
	if tableName == "" {
		return nil, errors.New("table name is empty")
	}
	var columns []*types.DatabaseTableColumn
	err := r.baseColumnQuery(ctx).
		Where("database_table_columns.tenant_id = ?", tenantID).
		Where("database_table_columns.knowledge_base_id = ?", knowledgeBaseID).
		Where("database_table_columns.table_name = ?", tableName).
		Order("database_table_columns.ordinal_position ASC").
		Find(&columns).Error
	return columns, err
}

func (r *databaseSchemaRepository) baseSnapshotQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Model(&types.DatabaseSchemaSnapshot{}).
		Joins("JOIN data_sources ON data_sources.id = database_schema_snapshots.data_source_id").
		Where("database_schema_snapshots.deleted_at IS NULL").
		Where("data_sources.deleted_at IS NULL")
}

func (r *databaseSchemaRepository) baseColumnQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Model(&types.DatabaseTableColumn{}).
		Joins("JOIN data_sources ON data_sources.id = database_table_columns.data_source_id").
		Where("database_table_columns.deleted_at IS NULL").
		Where("data_sources.deleted_at IS NULL")
}
