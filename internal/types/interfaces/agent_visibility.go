package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// AgentVisibilityService defines the agent visibility service
// Mirrors KBVisibilityService but for custom agents
type AgentVisibilityService interface {
	// ListAccessibleAgents returns all agents accessible to a user within a tenant,
	// considering visibility rules: global agents + org agents (user's orgs) + private agents (own)
	// Super admins can see all agents
	ListAccessibleAgents(ctx context.Context, userID string, tenantID uint64, isSuperAdmin bool) ([]*types.CustomAgent, error)
	// CanAccessAgent checks whether a user can read an agent in the current tenant
	CanAccessAgent(ctx context.Context, userID string, tenantID uint64, agentID string, isSuperAdmin bool) (bool, error)
	// CanManageAgent checks whether a user can manage an agent in the current tenant
	CanManageAgent(ctx context.Context, userID string, tenantID uint64, agentID string, isSuperAdmin bool) (bool, error)
}
