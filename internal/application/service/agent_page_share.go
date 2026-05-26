package service

import (
	"context"
	crand "crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

var (
	ErrAgentPageShareNotFound           = errors.New("agent page share not found")
	ErrAgentPageShareUnavailable        = errors.New("agent page share unavailable")
	ErrSharedAgentNotFound              = errors.New("shared agent not found")
	ErrAgentNotConfiguredForPageSharing = errors.New("agent is not fully configured for page sharing")
)

type agentPageShareService struct {
	shareRepo    interfaces.AgentPageShareRepository
	customAgent  interfaces.CustomAgentService
	modelService interfaces.ModelService
}

// NewAgentPageShareService creates a new service for public agent page sharing.
func NewAgentPageShareService(
	shareRepo interfaces.AgentPageShareRepository,
	customAgent interfaces.CustomAgentService,
	modelService interfaces.ModelService,
) interfaces.AgentPageShareService {
	return &agentPageShareService{
		shareRepo:    shareRepo,
		customAgent:  customAgent,
		modelService: modelService,
	}
}

// GetByAgent returns the current share state for one owned custom agent.
func (s *agentPageShareService) GetByAgent(ctx context.Context, agentID string, sourceTenantID uint64) (*types.AgentPageShare, error) {
	share, err := s.shareRepo.GetByAgent(ctx, agentID, sourceTenantID)
	if err != nil {
		if errors.Is(err, repository.ErrAgentPageShareNotFound) {
			return nil, ErrAgentPageShareNotFound
		}
		return nil, err
	}
	return share, nil
}

// CreateOrEnable creates a new share record or re-enables an existing one.
func (s *agentPageShareService) CreateOrEnable(ctx context.Context, agentID string, userID string, sourceTenantID uint64) (*types.AgentPageShare, error) {
	agent, err := s.customAgent.GetAgentByIDAndTenant(ctx, agentID, sourceTenantID)
	if err != nil {
		if errors.Is(err, ErrAgentNotFound) {
			return nil, ErrSharedAgentNotFound
		}
		return nil, err
	}
	agent.EnsureDefaults()
	if err := validateAgentPageShareReady(agent); err != nil {
		return nil, err
	}

	share, err := s.shareRepo.GetByAgent(ctx, agentID, sourceTenantID)
	if err == nil {
		changed := false
		now := time.Now()
		if share.ShareCode == "" {
			share.ShareCode = generateAgentPageShareCode()
			changed = true
		}
		if share.AccessScope == "" {
			share.AccessScope = types.AgentPageShareAccessScopeAnonymous
			changed = true
		}
		if share.Status != types.AgentPageShareStatusActive {
			share.Status = types.AgentPageShareStatusActive
			changed = true
		}
		if share.CreatedBy == "" {
			share.CreatedBy = userID
			changed = true
		}
		if share.ExpiresAt != nil && now.After(*share.ExpiresAt) {
			share.ExpiresAt = nil
			changed = true
		}
		if changed {
			share.UpdatedAt = now
			if err := s.shareRepo.Update(ctx, share); err != nil {
				return nil, err
			}
		}
		return share, nil
	}
	if !errors.Is(err, repository.ErrAgentPageShareNotFound) {
		return nil, err
	}

	now := time.Now()
	share = &types.AgentPageShare{
		ID:                    uuid.New().String(),
		AgentID:               agentID,
		SourceTenantID:        sourceTenantID,
		ShareCode:             generateAgentPageShareCode(),
		AccessScope:           types.AgentPageShareAccessScopeAnonymous,
		Status:                types.AgentPageShareStatusActive,
		CreatedBy:             userID,
		AnonymousSessionLimit: 0,
		RateLimitPerMinute:    0,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := s.shareRepo.Create(ctx, share); err != nil {
		return nil, err
	}
	return share, nil
}

// Disable closes a share without deleting its history or share code.
func (s *agentPageShareService) Disable(ctx context.Context, agentID string, sourceTenantID uint64) error {
	share, err := s.shareRepo.GetByAgent(ctx, agentID, sourceTenantID)
	if err != nil {
		if errors.Is(err, repository.ErrAgentPageShareNotFound) {
			return ErrAgentPageShareNotFound
		}
		return err
	}
	if share.Status == types.AgentPageShareStatusDisabled {
		return nil
	}
	share.Status = types.AgentPageShareStatusDisabled
	share.UpdatedAt = time.Now()
	return s.shareRepo.Update(ctx, share)
}

// GetPublicInfo resolves a public share code into the limited metadata needed by the share page.
func (s *agentPageShareService) GetPublicInfo(ctx context.Context, shareCode string) (*types.AgentPageSharePublicInfo, error) {
	share, err := s.shareRepo.GetByShareCode(ctx, strings.TrimSpace(shareCode))
	if err != nil {
		if errors.Is(err, repository.ErrAgentPageShareNotFound) {
			return nil, ErrAgentPageShareNotFound
		}
		return nil, err
	}
	if !isAgentPageShareAvailable(share) {
		return nil, ErrAgentPageShareUnavailable
	}

	agent, err := s.customAgent.GetAgentByIDAndTenant(ctx, share.AgentID, share.SourceTenantID)
	if err != nil {
		if errors.Is(err, ErrAgentNotFound) {
			return nil, ErrSharedAgentNotFound
		}
		return nil, err
	}
	agent.EnsureDefaults()

	now := time.Now()
	share.LastAccessedAt = &now
	if err := s.shareRepo.TouchLastAccessedAt(ctx, share.ID, now); err != nil {
		logger.Warnf(ctx, "failed to update agent page share access time, share_code=%s, err=%v", share.ShareCode, err)
	}

	availableModels, defaultModelName := s.buildPublicShareModels(ctx, share.SourceTenantID, agent.Config.ModelID)

	return &types.AgentPageSharePublicInfo{
		Share: types.AgentPageSharePublicSummary{
			ID:          share.ID,
			ShareCode:   share.ShareCode,
			Status:      share.Status,
			AccessScope: share.AccessScope,
		},
		Agent: types.AgentPageShareAgentSummary{
			ID:          agent.ID,
			Name:        agent.Name,
			Description: agent.Description,
			Avatar:      agent.Avatar,
		},
		Runtime: types.AgentPageShareRuntimeSummary{
			AgentMode:               agent.Config.AgentMode,
			KBSelectionMode:         agent.Config.KBSelectionMode,
			MCPSelectionMode:        agent.Config.MCPSelectionMode,
			WebSearchEnabled:        agent.Config.WebSearchEnabled,
			MultiTurnEnabled:        agent.Config.MultiTurnEnabled,
			ImageUploadEnabled:      agent.Config.ImageUploadEnabled,
			AudioUploadEnabled:      agent.Config.AudioUploadEnabled,
			AttachmentUploadEnabled: true,
			SupportedFileTypes:      append([]string(nil), agent.Config.SupportedFileTypes...),
			DefaultModelID:          strings.TrimSpace(agent.Config.ModelID),
			DefaultModelName:        defaultModelName,
			AvailableModels:         availableModels,
			ShowWebSearchToggle:     false,
			ShowModelSelector:       len(availableModels) > 0,
			ShowKBSelector:          false,
			ShowAgentSelector:       false,
		},
		SuggestedQuestions: append([]string(nil), agent.Config.SuggestedPrompts...),
	}, nil
}

func (s *agentPageShareService) buildPublicShareModels(ctx context.Context, tenantID uint64, defaultModelID string) ([]types.AgentPageSharePublicModelSummary, string) {
	if s.modelService == nil || tenantID == 0 {
		return nil, ""
	}

	modelCtx := context.WithValue(ctx, types.TenantIDContextKey, tenantID)
	models, err := s.modelService.ListModels(modelCtx)
	if err != nil {
		logger.Warnf(ctx, "failed to load share page models, tenant_id=%d, err=%v", tenantID, err)
		return nil, ""
	}

	trimmedDefaultModelID := strings.TrimSpace(defaultModelID)
	defaultModelName := ""
	availableModels := make([]types.AgentPageSharePublicModelSummary, 0, len(models))
	defaultFound := false
	for _, model := range models {
		if model == nil || model.Type != types.ModelTypeKnowledgeQA {
			continue
		}
		summary := types.AgentPageSharePublicModelSummary{
			ID:          model.ID,
			Name:        model.Name,
			Type:        model.Type,
			Source:      model.Source,
			Description: model.Description,
			Parameters: types.AgentPageSharePublicModelParameters{
				ParameterSize: model.Parameters.ParameterSize,
			},
			IsDefault: model.IsDefault,
			Status:    model.Status,
		}
		if trimmedDefaultModelID != "" && model.ID == trimmedDefaultModelID {
			defaultFound = true
			defaultModelName = model.Name
		}
		availableModels = append(availableModels, summary)
	}

	if trimmedDefaultModelID != "" && !defaultFound {
		model, err := s.modelService.GetModelByID(modelCtx, trimmedDefaultModelID)
		if err != nil {
			logger.Warnf(ctx, "failed to load default share model, tenant_id=%d, model_id=%s, err=%v", tenantID, trimmedDefaultModelID, err)
			return availableModels, defaultModelName
		}
		if model != nil && model.Type == types.ModelTypeKnowledgeQA {
			defaultModelName = model.Name
			availableModels = append(availableModels, types.AgentPageSharePublicModelSummary{
				ID:          model.ID,
				Name:        model.Name,
				Type:        model.Type,
				Source:      model.Source,
				Description: model.Description,
				Parameters: types.AgentPageSharePublicModelParameters{
					ParameterSize: model.Parameters.ParameterSize,
				},
				IsDefault: model.IsDefault,
				Status:    model.Status,
			})
		}
	}

	return availableModels, defaultModelName
}

func validateAgentPageShareReady(agent *types.CustomAgent) error {
	if agent == nil {
		return ErrSharedAgentNotFound
	}
	if agent.Config.ModelID == "" {
		return ErrAgentNotConfiguredForPageSharing
	}
	if agentRequiresRerankModel(agent) && agent.Config.RerankModelID == "" {
		return ErrAgentNotConfiguredForPageSharing
	}
	return nil
}

func isAgentPageShareAvailable(share *types.AgentPageShare) bool {
	if share == nil {
		return false
	}
	if share.AccessScope != types.AgentPageShareAccessScopeAnonymous {
		return false
	}
	if share.Status == types.AgentPageShareStatusExpired {
		return false
	}
	if share.Status == types.AgentPageShareStatusDisabled {
		return false
	}
	if share.ExpiresAt != nil && time.Now().After(*share.ExpiresAt) {
		return false
	}
	return true
}

func generateAgentPageShareCode() string {
	b := make([]byte, 24)
	if _, err := crand.Read(b); err != nil {
		return uuid.New().String()
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
