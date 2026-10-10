package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// TenantBootstrapRepository persists resumable tenant bootstrap operations.
type TenantBootstrapRepository interface {
	Create(ctx context.Context, op *types.TenantBootstrapOperation) error
	GetByTenantAndKey(ctx context.Context, tenantID uint64, idempotencyKey string) (*types.TenantBootstrapOperation, error)
	Update(ctx context.Context, op *types.TenantBootstrapOperation) error
}

// TenantBootstrapService provides the minimal persistence API the HTTP handler
// needs to resume or finalise a workspace bootstrap workflow.
type TenantBootstrapService interface {
	GetOrCreate(ctx context.Context, tenantID uint64, idempotencyKey string) (*types.TenantBootstrapOperation, bool, error)
	Save(ctx context.Context, op *types.TenantBootstrapOperation) error
}
