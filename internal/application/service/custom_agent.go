package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

// Custom agent related errors
var (
	ErrAgentNotFound       = errors.New("agent not found")
	ErrCannotModifyBuiltin = errors.New("cannot modify built-in agent basic info")
	ErrCannotDeleteBuiltin = errors.New("cannot delete built-in agent")
	ErrAgentNameRequired   = errors.New("agent name is required")
)

// customAgentService implements the CustomAgentService interface
type customAgentService struct {
	repo interfaces.CustomAgentRepository
}

// NewCustomAgentService creates a new custom agent service
func NewCustomAgentService(repo interfaces.CustomAgentRepository) interfaces.CustomAgentService {
	return &customAgentService{
		repo: repo,
	}
}

// CreateAgent creates a new custom agent
func (s *customAgentService) CreateAgent(ctx context.Context, agent *types.CustomAgent) (*types.CustomAgent, error) {
	// Validate required fields
	if strings.TrimSpace(agent.Name) == "" {
		return nil, ErrAgentNameRequired
	}

	// Generate UUID and set creation timestamps
	if agent.ID == "" {
		agent.ID = uuid.New().String()
	}

	// Get tenant ID from context
	tenantID, ok := ctx.Value(types.TenantIDContextKey).(uint64)
	if !ok {
		return nil, ErrInvalidTenantID
	}
	agent.TenantID = tenantID

	// Set timestamps
	agent.CreatedAt = time.Now()
	agent.UpdatedAt = time.Now()

	// Ensure custom type for user-created agents
	if agent.Type == "" {
		agent.Type = types.CustomAgentTypeCustom
	}

	// Cannot create built-in agents
	agent.IsBuiltin = false

	// Set defaults
	agent.EnsureDefaults()

	logger.Infof(ctx, "Creating custom agent, ID: %s, tenant ID: %d, name: %s, type: %s",
		agent.ID, agent.TenantID, agent.Name, agent.Type)

	if err := s.repo.CreateAgent(ctx, agent); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id":  agent.ID,
			"tenant_id": agent.TenantID,
		})
		return nil, err
	}

	logger.Infof(ctx, "Custom agent created successfully, ID: %s, name: %s", agent.ID, agent.Name)
	return agent, nil
}

// GetAgentByID retrieves an agent by its ID (including built-in agents)
func (s *customAgentService) GetAgentByID(ctx context.Context, id string) (*types.CustomAgent, error) {
	if id == "" {
		logger.Error(ctx, "Agent ID is empty")
		return nil, errors.New("agent ID cannot be empty")
	}

	// Get tenant ID from context
	tenantID, ok := ctx.Value(types.TenantIDContextKey).(uint64)
	if !ok {
		return nil, ErrInvalidTenantID
	}

	// Check if it's a built-in agent
	if id == types.BuiltinAgentNormalID || id == types.BuiltinAgentAgentID {
		// Try to get from database first (for customized config)
		agent, err := s.repo.GetAgentByID(ctx, id, tenantID)
		if err == nil {
			// Found in database, return with customized config
			return agent, nil
		}
		// Not in database, return default built-in agent
		if id == types.BuiltinAgentNormalID {
			return types.GetBuiltinNormalAgent(tenantID), nil
		}
		return types.GetBuiltinAgentAgent(tenantID), nil
	}

	// Query from database
	agent, err := s.repo.GetAgentByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrCustomAgentNotFound) {
			return nil, ErrAgentNotFound
		}
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id": id,
		})
		return nil, err
	}

	return agent, nil
}

// ListAgents lists all agents for the current tenant (including built-in agents)
func (s *customAgentService) ListAgents(ctx context.Context) ([]*types.CustomAgent, error) {
	tenantID, ok := ctx.Value(types.TenantIDContextKey).(uint64)
	if !ok {
		return nil, ErrInvalidTenantID
	}

	// Get all agents from database (including built-in agents with customized config)
	allAgents, err := s.repo.ListAgentsByTenantID(ctx, tenantID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tenant_id": tenantID,
		})
		return nil, err
	}

	// Check if built-in agents exist in database
	hasNormalBuiltin := false
	hasAgentBuiltin := false
	for _, agent := range allAgents {
		if agent.ID == types.BuiltinAgentNormalID {
			hasNormalBuiltin = true
		}
		if agent.ID == types.BuiltinAgentAgentID {
			hasAgentBuiltin = true
		}
	}

	// Build result: built-in agents first, then custom agents
	result := make([]*types.CustomAgent, 0, len(allAgents)+2)

	// Add built-in normal agent
	if hasNormalBuiltin {
		for _, agent := range allAgents {
			if agent.ID == types.BuiltinAgentNormalID {
				result = append(result, agent)
				break
			}
		}
	} else {
		result = append(result, types.GetBuiltinNormalAgent(tenantID))
	}

	// Add built-in agent mode agent
	if hasAgentBuiltin {
		for _, agent := range allAgents {
			if agent.ID == types.BuiltinAgentAgentID {
				result = append(result, agent)
				break
			}
		}
	} else {
		result = append(result, types.GetBuiltinAgentAgent(tenantID))
	}

	// Add custom agents
	for _, agent := range allAgents {
		if agent.ID != types.BuiltinAgentNormalID && agent.ID != types.BuiltinAgentAgentID {
			result = append(result, agent)
		}
	}

	return result, nil
}

// UpdateAgent updates an agent's information
func (s *customAgentService) UpdateAgent(ctx context.Context, agent *types.CustomAgent) (*types.CustomAgent, error) {
	if agent.ID == "" {
		logger.Error(ctx, "Agent ID is empty")
		return nil, errors.New("agent ID cannot be empty")
	}

	// Get tenant ID from context
	tenantID, ok := ctx.Value(types.TenantIDContextKey).(uint64)
	if !ok {
		return nil, ErrInvalidTenantID
	}

	// Handle built-in agents specially
	isBuiltinAgent := agent.ID == types.BuiltinAgentNormalID || agent.ID == types.BuiltinAgentAgentID
	if isBuiltinAgent {
		return s.updateBuiltinAgent(ctx, agent, tenantID)
	}

	// Get existing agent
	existingAgent, err := s.repo.GetAgentByID(ctx, agent.ID, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrCustomAgentNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}

	// Cannot modify built-in status
	if existingAgent.IsBuiltin {
		return nil, ErrCannotModifyBuiltin
	}

	// Validate name
	if strings.TrimSpace(agent.Name) == "" {
		return nil, ErrAgentNameRequired
	}

	// Update fields
	existingAgent.Name = agent.Name
	existingAgent.Description = agent.Description
	existingAgent.Avatar = agent.Avatar
	existingAgent.Type = agent.Type
	existingAgent.Config = agent.Config
	existingAgent.UpdatedAt = time.Now()

	// Ensure defaults
	existingAgent.EnsureDefaults()

	logger.Infof(ctx, "Updating custom agent, ID: %s, name: %s", agent.ID, agent.Name)

	if err := s.repo.UpdateAgent(ctx, existingAgent); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id": agent.ID,
		})
		return nil, err
	}

	logger.Infof(ctx, "Custom agent updated successfully, ID: %s", agent.ID)
	return existingAgent, nil
}

// updateBuiltinAgent updates a built-in agent's configuration (but not basic info)
func (s *customAgentService) updateBuiltinAgent(ctx context.Context, agent *types.CustomAgent, tenantID uint64) (*types.CustomAgent, error) {
	// Get the default built-in agent
	var defaultAgent *types.CustomAgent
	if agent.ID == types.BuiltinAgentNormalID {
		defaultAgent = types.GetBuiltinNormalAgent(tenantID)
	} else {
		defaultAgent = types.GetBuiltinAgentAgent(tenantID)
	}

	// Try to get existing customized config from database
	existingAgent, err := s.repo.GetAgentByID(ctx, agent.ID, tenantID)
	if err != nil && !errors.Is(err, repository.ErrCustomAgentNotFound) {
		return nil, err
	}

	if existingAgent != nil {
		// Update existing record - only update config, keep basic info unchanged
		existingAgent.Config = agent.Config
		existingAgent.UpdatedAt = time.Now()
		existingAgent.EnsureDefaults()

		logger.Infof(ctx, "Updating built-in agent config, ID: %s", agent.ID)

		if err := s.repo.UpdateAgent(ctx, existingAgent); err != nil {
			logger.ErrorWithFields(ctx, err, map[string]interface{}{
				"agent_id": agent.ID,
			})
			return nil, err
		}

		logger.Infof(ctx, "Built-in agent config updated successfully, ID: %s", agent.ID)
		return existingAgent, nil
	}

	// Create new record for built-in agent with customized config
	newAgent := &types.CustomAgent{
		ID:          defaultAgent.ID,
		Name:        defaultAgent.Name,
		Description: defaultAgent.Description,
		Avatar:      defaultAgent.Avatar,
		IsBuiltin:   true,
		Type:        defaultAgent.Type,
		TenantID:    tenantID,
		Config:      agent.Config,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	newAgent.EnsureDefaults()

	logger.Infof(ctx, "Creating built-in agent config record, ID: %s, tenant ID: %d", agent.ID, tenantID)

	if err := s.repo.CreateAgent(ctx, newAgent); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id":  agent.ID,
			"tenant_id": tenantID,
		})
		return nil, err
	}

	logger.Infof(ctx, "Built-in agent config record created successfully, ID: %s", agent.ID)
	return newAgent, nil
}

// DeleteAgent deletes an agent
func (s *customAgentService) DeleteAgent(ctx context.Context, id string) error {
	if id == "" {
		logger.Error(ctx, "Agent ID is empty")
		return errors.New("agent ID cannot be empty")
	}

	// Cannot delete built-in agents
	if id == types.BuiltinAgentNormalID || id == types.BuiltinAgentAgentID {
		return ErrCannotDeleteBuiltin
	}

	// Get tenant ID from context
	tenantID, ok := ctx.Value(types.TenantIDContextKey).(uint64)
	if !ok {
		return ErrInvalidTenantID
	}

	// Get existing agent to verify ownership
	existingAgent, err := s.repo.GetAgentByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrCustomAgentNotFound) {
			return ErrAgentNotFound
		}
		return err
	}

	// Cannot delete built-in agents
	if existingAgent.IsBuiltin {
		return ErrCannotDeleteBuiltin
	}

	logger.Infof(ctx, "Deleting custom agent, ID: %s", id)

	if err := s.repo.DeleteAgent(ctx, id); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id": id,
		})
		return err
	}

	logger.Infof(ctx, "Custom agent deleted successfully, ID: %s", id)
	return nil
}
