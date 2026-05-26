package interfaces

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// AgentPageShareRepository defines persistence operations for public agent-share records.
type AgentPageShareRepository interface {
	Create(ctx context.Context, share *types.AgentPageShare) error
	GetByAgent(ctx context.Context, agentID string, sourceTenantID uint64) (*types.AgentPageShare, error)
	GetByShareCode(ctx context.Context, shareCode string) (*types.AgentPageShare, error)
	TouchLastAccessedAt(ctx context.Context, shareID string, accessedAt time.Time) error
	Update(ctx context.Context, share *types.AgentPageShare) error
}

// AgentPageShareService defines management and public-read operations for agent page shares.
type AgentPageShareService interface {
	GetByAgent(ctx context.Context, agentID string, sourceTenantID uint64) (*types.AgentPageShare, error)
	CreateOrEnable(ctx context.Context, agentID string, userID string, sourceTenantID uint64) (*types.AgentPageShare, error)
	Disable(ctx context.Context, agentID string, sourceTenantID uint64) error
	GetPublicInfo(ctx context.Context, shareCode string) (*types.AgentPageSharePublicInfo, error)
}
