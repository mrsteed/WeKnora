package service

import (
	"context"
	"fmt"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// kbVisibilityService implements KBVisibilityService interface
type kbVisibilityService struct {
	kbRepo         interfaces.KnowledgeBaseRepository
	orgTreeService interfaces.OrgTreeService
	kgRepo         interfaces.KnowledgeRepository
	chunkRepo      interfaces.ChunkRepository
	schemaRepo     interfaces.DatabaseSchemaRepository
	userRepo       interfaces.UserRepository
	authorizer     *sameTenantResourceAuthorizer
}

// NewKBVisibilityService creates a new knowledge base visibility service
func NewKBVisibilityService(
	kbRepo interfaces.KnowledgeBaseRepository,
	orgTreeService interfaces.OrgTreeService,
	kgRepo interfaces.KnowledgeRepository,
	chunkRepo interfaces.ChunkRepository,
	schemaRepo interfaces.DatabaseSchemaRepository,
	userRepo interfaces.UserRepository,
) interfaces.KBVisibilityService {
	return &kbVisibilityService{
		kbRepo:         kbRepo,
		orgTreeService: orgTreeService,
		kgRepo:         kgRepo,
		chunkRepo:      chunkRepo,
		schemaRepo:     schemaRepo,
		userRepo:       userRepo,
		authorizer:     newSameTenantResourceAuthorizer(orgTreeService),
	}
}

// ListAccessibleKBs returns all knowledge bases accessible to a user within a tenant,
// considering visibility rules: global KBs + org KBs (user's orgs and their descendants) + private KBs (own)
// Super admins bypass visibility rules and see all KBs
func (s *kbVisibilityService) ListAccessibleKBs(ctx context.Context, userID string, tenantID uint64, isSuperAdmin bool) ([]*types.KnowledgeBase, error) {
	logger.Infof(ctx, "Listing accessible KBs for user %s in tenant %d (superAdmin=%v)", userID, tenantID, isSuperAdmin)

	// Super admin bypass: return all non-temporary KBs in the tenant
	if isSuperAdmin {
		kbs, err := s.kbRepo.ListKnowledgeBasesByTenantID(ctx, tenantID)
		if err != nil {
			logger.Errorf(ctx, "Failed to list all KBs for super admin: %v", err)
			return nil, fmt.Errorf("failed to list KBs: %w", err)
		}
		s.fillKnowledgeCounts(ctx, kbs)
		s.fillCreatorNicknames(ctx, kbs)
		return kbs, nil
	}

	scope, err := s.authorizer.resolveScope(ctx, userID, tenantID)
	if err != nil {
		logger.Errorf(ctx, "Failed to resolve KB visibility scope: %v", err)
		return nil, fmt.Errorf("failed to resolve knowledge-base visibility scope: %w", err)
	}

	// Query KBs with visibility rules
	kbs, err := s.kbRepo.ListAccessibleKBs(ctx, userID, tenantID, scope.readOrgList)
	if err != nil {
		logger.Errorf(ctx, "Failed to list accessible KBs: %v", err)
		return nil, fmt.Errorf("failed to list accessible KBs: %w", err)
	}

	// Fill knowledge counts for each knowledge base
	s.fillKnowledgeCounts(ctx, kbs)

	// Fill creator nicknames for each knowledge base
	s.fillCreatorNicknames(ctx, kbs)

	return kbs, nil
}

// fillKnowledgeCounts fills KnowledgeCount, ChunkCount, IsProcessing, ProcessingCount for all KBs
func (s *kbVisibilityService) fillKnowledgeCounts(ctx context.Context, kbs []*types.KnowledgeBase) {
	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		kb.EnsureDefaults()
		tenantID := kb.TenantID

		fillKnowledgeBaseUsageCounts(ctx, kb, tenantID, s.kgRepo, s.chunkRepo, s.schemaRepo)

		// Check processing status
		if processingCount, err := s.kgRepo.CountKnowledgeByStatus(ctx, tenantID, kb.ID, []string{"pending", "processing"}); err == nil {
			kb.IsProcessing = processingCount > 0
			kb.ProcessingCount = processingCount
		} else {
			logger.Warnf(ctx, "Failed to get processing count for KB %s: %v", kb.ID, err)
		}
	}
}

// CanAccessKB checks whether a user can access (read) a specific knowledge base
func (s *kbVisibilityService) CanAccessKB(ctx context.Context, userID string, tenantID uint64, kbID string, isSuperAdmin bool) (bool, error) {
	// Super admin can access all knowledge bases
	if isSuperAdmin {
		return true, nil
	}

	kb, err := s.kbRepo.GetKnowledgeBaseByIDAndTenant(ctx, kbID, tenantID)
	if err != nil {
		return false, fmt.Errorf("failed to get knowledge base: %w", err)
	}
	scope, err := s.authorizer.resolveScope(ctx, userID, tenantID)
	if err != nil {
		return false, fmt.Errorf("failed to resolve knowledge-base visibility scope: %w", err)
	}
	return s.authorizer.canReadResource(sameTenantResourceRule{
		Visibility:     kb.Visibility,
		OrganizationID: kb.OrganizationID,
		CreatedBy:      kb.CreatedBy,
	}, userID, isSuperAdmin, scope), nil
}

// CanManageKB checks whether a user can manage (edit/delete) a specific knowledge base
func (s *kbVisibilityService) CanManageKB(ctx context.Context, userID string, tenantID uint64, kbID string, isSuperAdmin bool) (bool, error) {
	// Super admin can manage all knowledge bases
	if isSuperAdmin {
		return true, nil
	}

	kb, err := s.kbRepo.GetKnowledgeBaseByIDAndTenant(ctx, kbID, tenantID)
	if err != nil {
		return false, fmt.Errorf("failed to get knowledge base: %w", err)
	}
	scope, err := s.authorizer.resolveScope(ctx, userID, tenantID)
	if err != nil {
		return false, fmt.Errorf("failed to resolve knowledge-base visibility scope: %w", err)
	}
	return s.authorizer.canManageResource(sameTenantResourceRule{
		Visibility:     kb.Visibility,
		OrganizationID: kb.OrganizationID,
		CreatedBy:      kb.CreatedBy,
	}, userID, isSuperAdmin, scope), nil
}

// fillCreatorNicknames fills the CreatedByNickname field for each knowledge base.
func (s *kbVisibilityService) fillCreatorNicknames(ctx context.Context, kbs []*types.KnowledgeBase) {
	fillKnowledgeBaseCreatorNicknames(ctx, s.userRepo, kbs)
}
