package repository

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// ErrCustomAgentNotFound is returned when a custom agent is not found
var ErrCustomAgentNotFound = errors.New("custom agent not found")

// customAgentRepository implements the CustomAgentRepository interface
type customAgentRepository struct {
	db *gorm.DB
}

// NewCustomAgentRepository creates a new custom agent repository
func NewCustomAgentRepository(db *gorm.DB) interfaces.CustomAgentRepository {
	return &customAgentRepository{db: db}
}

// CreateAgent creates a new custom agent
func (r *customAgentRepository) CreateAgent(ctx context.Context, agent *types.CustomAgent) error {
	return r.db.WithContext(ctx).Create(agent).Error
}

// GetAgentByID gets an agent by id and tenant
func (r *customAgentRepository) GetAgentByID(ctx context.Context, id string, tenantID uint64) (*types.CustomAgent, error) {
	var agent types.CustomAgent
	if err := r.db.WithContext(ctx).
		Table("custom_agents").
		Select("custom_agents.*, users.username as creator_name").
		Joins("LEFT JOIN users ON custom_agents.created_by = users.id").
		Where("custom_agents.id = ? AND custom_agents.tenant_id = ? AND custom_agents.deleted_at IS NULL", id, tenantID).
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCustomAgentNotFound
		}
		return nil, err
	}
	return &agent, nil
}

// ListAgentsByTenantID lists all agents for a specific tenant
func (r *customAgentRepository) ListAgentsByTenantID(ctx context.Context, tenantID uint64) ([]*types.CustomAgent, error) {
	var agents []*types.CustomAgent
	if err := r.db.WithContext(ctx).
		Table("custom_agents").
		Select("custom_agents.*, users.username as creator_name").
		Joins("LEFT JOIN users ON custom_agents.created_by = users.id").
		Where("custom_agents.tenant_id = ? AND custom_agents.deleted_at IS NULL", tenantID).
		Order("custom_agents.created_at DESC").
		Scan(&agents).Error; err != nil {
		return nil, err
	}
	return agents, nil
}

// UpdateAgent updates an agent
func (r *customAgentRepository) UpdateAgent(ctx context.Context, agent *types.CustomAgent) error {
	return r.db.WithContext(ctx).Save(agent).Error
}

// ListAccessibleAgents lists agents accessible to a user based on visibility rules
func (r *customAgentRepository) ListAccessibleAgents(
	ctx context.Context, userID string, tenantID uint64, orgIDs []string,
) ([]*types.CustomAgent, error) {
	var agents []*types.CustomAgent

	query := r.db.WithContext(ctx).
		Table("custom_agents").
		Select("custom_agents.*, users.username as creator_name").
		Joins("LEFT JOIN users ON custom_agents.created_by = users.id").
		Where("custom_agents.tenant_id = ? AND custom_agents.is_builtin = ? AND custom_agents.deleted_at IS NULL", tenantID, false)

	// Build visibility conditions (same pattern as KnowledgeBase):
	// visibility='global' OR (visibility='org' AND organization_id IN orgIDs) OR (visibility='private' AND created_by=userID)
	// Also include legacy agents with empty visibility (treat as global)
	if len(orgIDs) > 0 {
		query = query.Where(
			"(custom_agents.visibility = ? OR custom_agents.visibility = '' OR custom_agents.visibility IS NULL) OR (custom_agents.visibility = ? AND custom_agents.organization_id IN ?) OR (custom_agents.visibility = ? AND custom_agents.created_by = ?)",
			types.AgentVisibilityGlobal,
			types.AgentVisibilityOrg, orgIDs,
			types.AgentVisibilityPrivate, userID,
		)
	} else {
		query = query.Where(
			"(custom_agents.visibility = ? OR custom_agents.visibility = '' OR custom_agents.visibility IS NULL) OR (custom_agents.visibility = ? AND custom_agents.created_by = ?)",
			types.AgentVisibilityGlobal,
			types.AgentVisibilityPrivate, userID,
		)
	}

	if err := query.Order("custom_agents.created_at DESC").Scan(&agents).Error; err != nil {
		return nil, err
	}
	return agents, nil
}

// DeleteAgent deletes an agent (soft delete)
func (r *customAgentRepository) DeleteAgent(ctx context.Context, id string, tenantID uint64) error {
	return r.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&types.CustomAgent{}).Error
}
