package service

import (
	"context"
	"fmt"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// agentVisibilityService implements AgentVisibilityService interface
type agentVisibilityService struct {
	agentRepo      interfaces.CustomAgentRepository
	agentService   interfaces.CustomAgentService
	orgTreeService interfaces.OrgTreeService
}

// NewAgentVisibilityService creates a new agent visibility service
func NewAgentVisibilityService(
	agentRepo interfaces.CustomAgentRepository,
	agentService interfaces.CustomAgentService,
	orgTreeService interfaces.OrgTreeService,
) interfaces.AgentVisibilityService {
	return &agentVisibilityService{
		agentRepo:      agentRepo,
		agentService:   agentService,
		orgTreeService: orgTreeService,
	}
}

// ListAccessibleAgents returns all agents accessible to a user within a tenant,
// considering visibility rules: global agents + org agents (user's orgs) + private agents (own)
// Super admins bypass visibility rules and see all agents.
// Built-in agents are always included.
func (s *agentVisibilityService) ListAccessibleAgents(ctx context.Context, userID string, tenantID uint64, isSuperAdmin bool) ([]*types.CustomAgent, error) {
	logger.Infof(ctx, "Listing accessible agents for user %s in tenant %d (superAdmin=%v)", userID, tenantID, isSuperAdmin)

	// Super admin bypass: use existing ListAgents which returns all (including built-in)
	if isSuperAdmin {
		return s.agentService.ListAgents(ctx)
	}

	// Get user's organizations within this tenant
	userOrgs, err := s.orgTreeService.GetUserOrganizations(ctx, userID, tenantID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get user organizations: %v", err)
		return nil, fmt.Errorf("failed to get user organizations: %w", err)
	}

	// Collect all related org IDs (same logic as kb_visibility.go):
	// 1. User's orgs themselves
	// 2. Ancestors of user's orgs
	// 3. Descendants of user's orgs
	orgIDSet := make(map[string]bool)
	pathPrefixes := make([]string, 0, len(userOrgs))
	for _, org := range userOrgs {
		orgIDSet[org.ID] = true
		pathPrefixes = append(pathPrefixes, org.Path)
	}

	// Extract ancestor org IDs from paths (no DB query needed)
	ancestorIDs := s.orgTreeService.GetAncestorIDsFromPaths(pathPrefixes)
	for _, id := range ancestorIDs {
		orgIDSet[id] = true
	}

	// Batch get all descendants
	if len(pathPrefixes) > 0 {
		allDescendants, err := s.orgTreeService.GetDescendantIDsByPaths(ctx, pathPrefixes, tenantID)
		if err != nil {
			logger.Errorf(ctx, "Failed to batch get descendant org IDs: %v", err)
		} else {
			for _, id := range allDescendants {
				orgIDSet[id] = true
			}
		}
	}

	orgIDs := make([]string, 0, len(orgIDSet))
	for id := range orgIDSet {
		orgIDs = append(orgIDs, id)
	}

	// Query custom agents with visibility rules
	customAgents, err := s.agentRepo.ListAccessibleAgents(ctx, userID, tenantID, orgIDs)
	if err != nil {
		logger.Errorf(ctx, "Failed to list accessible agents: %v", err)
		return nil, fmt.Errorf("failed to list accessible agents: %w", err)
	}

	// Prepend built-in agents (always visible to everyone)
	// Use the same pattern as customAgentService.ListAgents
	dbAgents, _ := s.agentRepo.ListAgentsByTenantID(ctx, tenantID)
	builtinInDB := make(map[string]*types.CustomAgent)
	for _, a := range dbAgents {
		if types.IsBuiltinAgentID(a.ID) {
			builtinInDB[a.ID] = a
		}
	}

	builtinIDs := types.GetBuiltinAgentIDs()
	result := make([]*types.CustomAgent, 0, len(builtinIDs)+len(customAgents))

	// Add built-in agents in order
	for _, builtinID := range builtinIDs {
		if override, ok := builtinInDB[builtinID]; ok {
			result = append(result, override)
		} else {
			if agent := types.GetBuiltinAgent(builtinID, tenantID); agent != nil {
				result = append(result, agent)
			}
		}
	}

	// Append custom agents filtered by visibility
	result = append(result, customAgents...)

	return result, nil
}
