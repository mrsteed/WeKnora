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
	authorizer     *sameTenantResourceAuthorizer
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
		authorizer:     newSameTenantResourceAuthorizer(orgTreeService),
	}
}

// ListAccessibleAgents returns all agents accessible to a user within a tenant,
// considering visibility rules: global agents + org agents (user's orgs and descendants) + private agents (own)
// Super admins bypass visibility rules and see all agents.
// Built-in agents are always included.
func (s *agentVisibilityService) ListAccessibleAgents(ctx context.Context, userID string, tenantID uint64, isSuperAdmin bool) ([]*types.CustomAgent, error) {
	logger.Infof(ctx, "Listing accessible agents for user %s in tenant %d (superAdmin=%v)", userID, tenantID, isSuperAdmin)

	// Super admin bypass: use existing ListAgents which returns all (including built-in)
	if isSuperAdmin {
		return s.agentService.ListAgents(ctx)
	}

	scope, err := s.authorizer.resolveScope(ctx, userID, tenantID)
	if err != nil {
		logger.Errorf(ctx, "Failed to resolve agent visibility scope: %v", err)
		return nil, fmt.Errorf("failed to resolve agent visibility scope: %w", err)
	}

	// Query custom agents with visibility rules
	customAgents, err := s.agentRepo.ListAccessibleAgents(ctx, userID, tenantID, scope.readOrgList)
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
			result = append(result, types.ApplyBuiltinAgentLocalizedMetadata(ctx, override))
		} else {
			if agent := types.GetBuiltinAgentWithContext(ctx, builtinID, tenantID); agent != nil {
				result = append(result, agent)
			}
		}
	}

	// Append custom agents filtered by visibility
	result = append(result, customAgents...)

	return result, nil
}

func (s *agentVisibilityService) CanAccessAgent(ctx context.Context, userID string, tenantID uint64, agentID string, isSuperAdmin bool) (bool, error) {
	if isSuperAdmin {
		return true, nil
	}
	// Built-in agents are platform resources: they are always readable by
	// anyone in the tenant (tenant ownership is already guaranteed by
	// GetAgentByID's tenant-scoped lookup). Their persisted config override
	// rows historically carry visibility=''/'private' with an empty
	// created_by (no owner), which under the private rule would deny every
	// non-privileged user — so the private-visibility rule must NOT apply.
	if types.IsBuiltinAgentID(agentID) {
		agent, err := s.agentService.GetAgentByID(ctx, agentID)
		if err != nil {
			return false, err
		}
		return agent != nil && agent.TenantID == tenantID, nil
	}
	agent, err := s.agentService.GetAgentByID(ctx, agentID)
	if err != nil {
		return false, err
	}
	scope, err := s.authorizer.resolveScope(ctx, userID, tenantID)
	if err != nil {
		return false, fmt.Errorf("failed to resolve agent visibility scope: %w", err)
	}
	return s.authorizer.canReadResource(sameTenantResourceRule{
		Visibility:     agent.Visibility,
		OrganizationID: agent.OrganizationID,
		CreatedBy:      agent.CreatedBy,
	}, userID, isSuperAdmin, scope), nil
}

func (s *agentVisibilityService) CanManageAgent(ctx context.Context, userID string, tenantID uint64, agentID string, isSuperAdmin bool) (bool, error) {
	if isSuperAdmin {
		return true, nil
	}
	agent, err := s.agentService.GetAgentByID(ctx, agentID)
	if err != nil {
		return false, err
	}
	scope, err := s.authorizer.resolveScope(ctx, userID, tenantID)
	if err != nil {
		return false, fmt.Errorf("failed to resolve agent visibility scope: %w", err)
	}
	return s.authorizer.canManageResource(sameTenantResourceRule{
		Visibility:     agent.Visibility,
		OrganizationID: agent.OrganizationID,
		CreatedBy:      agent.CreatedBy,
	}, userID, isSuperAdmin, scope), nil
}
