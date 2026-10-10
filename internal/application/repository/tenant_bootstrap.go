package repository

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type tenantBootstrapRepository struct {
	db *gorm.DB
}

func NewTenantBootstrapRepository(db *gorm.DB) interfaces.TenantBootstrapRepository {
	return &tenantBootstrapRepository{db: db}
}

func (r *tenantBootstrapRepository) Create(ctx context.Context, op *types.TenantBootstrapOperation) error {
	return r.db.WithContext(ctx).Create(op).Error
}

func (r *tenantBootstrapRepository) GetByTenantAndKey(ctx context.Context, tenantID uint64, idempotencyKey string) (*types.TenantBootstrapOperation, error) {
	var op types.TenantBootstrapOperation
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND idempotency_key = ?", tenantID, idempotencyKey).
		First(&op).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &op, nil
}

func (r *tenantBootstrapRepository) Update(ctx context.Context, op *types.TenantBootstrapOperation) error {
	return r.db.WithContext(ctx).
		Model(&types.TenantBootstrapOperation{}).
		Where("id = ?", op.ID).
		Select(
			"status",
			"request_hash",
			"request_payload",
			"result_payload",
			"root_org_id",
			"root_org_name",
			"user_id",
			"invite_url",
			"tenant_membership_done",
			"org_assignment_done",
			"updated_at",
		).
		Updates(op).Error
}
