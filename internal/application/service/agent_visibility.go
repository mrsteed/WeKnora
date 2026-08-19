package service

import (
	"context"
	"fmt"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// agentVisibilityService implements AgentVisibilityService.
type agentVisibilityService struct {
	agentRepo      interfaces.CustomAgentRepository
	agentService   interfaces.CustomAgentService
	orgTreeService interfaces.OrgTreeService
	authorizer     *sameTenantResourceAuthorizer
}

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

func (s *agentVisibilityService) ListAccessibleAgents(ctx context.Context, userID string, tenantID uint64, isSuperAdmin bool) ([]*types.CustomAgent, error) {
	logger.Infof(ctx, "Listing accessible agents for user %s in tenant %d (superAdmin=%v)", userID, tenantID, isSuperAdmin)

	if isSuperAdmin {
		return s.agentService.ListAgents(ctx)
	}

	scope, err := s.authorizer.resolveScope(ctx, userID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve agent visibility scope: %w", err)
	}

	customAgents, err := s.agentRepo.ListAccessibleAgents(ctx, userID, tenantID, scope.readOrgList)
	if err != nil {
		return nil, fmt.Errorf("failed to list accessible agents: %w", err)
	}

	dbAgents, _ := s.agentRepo.ListAgentsByTenantID(ctx, tenantID)
	builtinInDB := make(map[string]*types.CustomAgent)
	for _, agent := range dbAgents {
		if types.IsBuiltinAgentID(agent.ID) {
			builtinInDB[agent.ID] = agent
		}
	}

	builtinIDs := types.GetBuiltinAgentIDs()
	result := make([]*types.CustomAgent, 0, len(builtinIDs)+len(customAgents))
	for _, builtinID := range builtinIDs {
		if override, ok := builtinInDB[builtinID]; ok {
			override.EnsureDefaults()
			result = append(result, override)
			continue
		}
		if agent := types.GetBuiltinAgentWithContext(ctx, builtinID, tenantID); agent != nil {
			result = append(result, agent)
		}
	}

	result = append(result, customAgents...)
	return result, nil
}

func (s *agentVisibilityService) CanAccessAgent(ctx context.Context, userID string, tenantID uint64, agentID string, isSuperAdmin bool) (bool, error) {
	if isSuperAdmin {
		return true, nil
	}
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
