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
	ErrCannotModifyBuiltin = errors.New("cannot modify built-in agent")
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
	if id == types.BuiltinAgentNormalID {
		return types.GetBuiltinNormalAgent(tenantID), nil
	}
	if id == types.BuiltinAgentAgentID {
		return types.GetBuiltinAgentAgent(tenantID), nil
	}

	// Query from database
	agent, err := s.repo.GetAgentByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrCustomAgentNotFound) {
			return nil, ErrAgentNotFound
		}
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id": id,
		})
		return nil, err
	}

	// Verify tenant ownership
	if agent.TenantID != tenantID {
		return nil, ErrAgentNotFound
	}

	return agent, nil
}

// ListAgents lists all agents for the current tenant (including built-in agents)
func (s *customAgentService) ListAgents(ctx context.Context) ([]*types.CustomAgent, error) {
	tenantID, ok := ctx.Value(types.TenantIDContextKey).(uint64)
	if !ok {
		return nil, ErrInvalidTenantID
	}

	// Get built-in agents first
	builtinAgents := types.GetBuiltinAgents(tenantID)

	// Get custom agents from database
	customAgents, err := s.repo.ListAgentsByTenantID(ctx, tenantID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tenant_id": tenantID,
		})
		return nil, err
	}

	// Combine: built-in agents first, then custom agents
	result := make([]*types.CustomAgent, 0, len(builtinAgents)+len(customAgents))
	result = append(result, builtinAgents...)
	result = append(result, customAgents...)

	return result, nil
}

// UpdateAgent updates an agent's information
func (s *customAgentService) UpdateAgent(ctx context.Context, agent *types.CustomAgent) (*types.CustomAgent, error) {
	if agent.ID == "" {
		logger.Error(ctx, "Agent ID is empty")
		return nil, errors.New("agent ID cannot be empty")
	}

	// Cannot modify built-in agents
	if agent.ID == types.BuiltinAgentNormalID || agent.ID == types.BuiltinAgentAgentID {
		return nil, ErrCannotModifyBuiltin
	}

	// Get tenant ID from context
	tenantID, ok := ctx.Value(types.TenantIDContextKey).(uint64)
	if !ok {
		return nil, ErrInvalidTenantID
	}

	// Get existing agent
	existingAgent, err := s.repo.GetAgentByID(ctx, agent.ID)
	if err != nil {
		if errors.Is(err, repository.ErrCustomAgentNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}

	// Verify tenant ownership
	if existingAgent.TenantID != tenantID {
		return nil, ErrAgentNotFound
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
	existingAgent, err := s.repo.GetAgentByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrCustomAgentNotFound) {
			return ErrAgentNotFound
		}
		return err
	}

	// Verify tenant ownership
	if existingAgent.TenantID != tenantID {
		return ErrAgentNotFound
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
