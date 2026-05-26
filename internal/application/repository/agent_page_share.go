package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

var (
	ErrAgentPageShareNotFound = errors.New("agent page share not found")
)

type agentPageShareRepository struct {
	db *gorm.DB
}

// NewAgentPageShareRepository creates a new agent page share repository.
func NewAgentPageShareRepository(db *gorm.DB) interfaces.AgentPageShareRepository {
	return &agentPageShareRepository{db: db}
}

// Create inserts a new agent page share record.
func (r *agentPageShareRepository) Create(ctx context.Context, share *types.AgentPageShare) error {
	return r.db.WithContext(ctx).Create(share).Error
}

// GetByAgent loads the share record for one agent inside its owner tenant.
func (r *agentPageShareRepository) GetByAgent(ctx context.Context, agentID string, sourceTenantID uint64) (*types.AgentPageShare, error) {
	var share types.AgentPageShare
	err := r.db.WithContext(ctx).
		Where("agent_id = ? AND source_tenant_id = ?", agentID, sourceTenantID).
		First(&share).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentPageShareNotFound
		}
		return nil, err
	}
	return &share, nil
}

// GetByShareCode resolves a public share by its opaque share code.
func (r *agentPageShareRepository) GetByShareCode(ctx context.Context, shareCode string) (*types.AgentPageShare, error) {
	var share types.AgentPageShare
	err := r.db.WithContext(ctx).
		Where("share_code = ?", shareCode).
		First(&share).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentPageShareNotFound
		}
		return nil, err
	}
	return &share, nil
}

// TouchLastAccessedAt updates only the public last-access timestamp.
func (r *agentPageShareRepository) TouchLastAccessedAt(ctx context.Context, shareID string, accessedAt time.Time) error {
	return r.db.WithContext(ctx).
		Model(&types.AgentPageShare{}).
		Where("id = ?", shareID).
		UpdateColumn("last_accessed_at", accessedAt).Error
}

// Update persists changes to an existing share record.
func (r *agentPageShareRepository) Update(ctx context.Context, share *types.AgentPageShare) error {
	return r.db.WithContext(ctx).Save(share).Error
}
