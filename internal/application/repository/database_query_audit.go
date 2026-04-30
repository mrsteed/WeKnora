package repository

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type databaseQueryAuditRepository struct {
	db *gorm.DB
}

func NewDatabaseQueryAuditRepository(db *gorm.DB) interfaces.DatabaseQueryAuditRepository {
	return &databaseQueryAuditRepository{db: db}
}

func (r *databaseQueryAuditRepository) Create(ctx context.Context, log *types.DatabaseQueryAuditLog) error {
	if log == nil {
		return errors.New("database query audit log is nil")
	}
	if log.TenantID == 0 {
		return errors.New("tenant id is required")
	}
	if log.UserID == "" {
		return errors.New("user id is required")
	}
	if log.KnowledgeBaseID == "" {
		return errors.New("knowledge base id is required")
	}
	if log.DataSourceID == "" {
		return errors.New("data source id is required")
	}
	if log.OriginalSQL == "" {
		return errors.New("original sql is required")
	}
	if log.Status == "" {
		return errors.New("status is required")
	}
	return r.db.WithContext(ctx).Create(log).Error
}

func (r *databaseQueryAuditRepository) ListByTenant(
	ctx context.Context,
	tenantID uint64,
	knowledgeBaseID string,
	limit int,
	offset int,
) ([]*types.DatabaseQueryAuditLog, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	query := r.db.WithContext(ctx).
		Model(&types.DatabaseQueryAuditLog{}).
		Where("tenant_id = ?", tenantID)
	if knowledgeBaseID != "" {
		query = query.Where("knowledge_base_id = ?", knowledgeBaseID)
	}
	var logs []*types.DatabaseQueryAuditLog
	err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&logs).Error
	return logs, err
}

func (r *databaseQueryAuditRepository) CountByTenant(
	ctx context.Context,
	tenantID uint64,
	knowledgeBaseID string,
) (int64, error) {
	query := r.db.WithContext(ctx).
		Model(&types.DatabaseQueryAuditLog{}).
		Where("tenant_id = ?", tenantID)
	if knowledgeBaseID != "" {
		query = query.Where("knowledge_base_id = ?", knowledgeBaseID)
	}
	var count int64
	err := query.Count(&count).Error
	return count, err
}
