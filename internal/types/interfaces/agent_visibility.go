package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// AgentVisibilityService resolves same-tenant visibility and manageability of
// custom agents using local org-tree semantics.
type AgentVisibilityService interface {
	ListAccessibleAgents(ctx context.Context, userID string, tenantID uint64, isSuperAdmin bool) ([]*types.CustomAgent, error)
	CanAccessAgent(ctx context.Context, userID string, tenantID uint64, agentID string, isSuperAdmin bool) (bool, error)
	CanManageAgent(ctx context.Context, userID string, tenantID uint64, agentID string, isSuperAdmin bool) (bool, error)
}
