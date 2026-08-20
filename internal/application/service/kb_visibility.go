package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// kbVisibilityService implements KBVisibilityService.
type kbVisibilityService struct {
	kbRepo         interfaces.KnowledgeBaseRepository
	orgTreeService interfaces.OrgTreeService
	kgRepo         interfaces.KnowledgeRepository
	chunkRepo      interfaces.ChunkRepository
	userRepo       interfaces.UserRepository
	authorizer     *sameTenantResourceAuthorizer
}

func NewKBVisibilityService(
	kbRepo interfaces.KnowledgeBaseRepository,
	orgTreeService interfaces.OrgTreeService,
	kgRepo interfaces.KnowledgeRepository,
	chunkRepo interfaces.ChunkRepository,
	userRepo interfaces.UserRepository,
) interfaces.KBVisibilityService {
	return &kbVisibilityService{
		kbRepo:         kbRepo,
		orgTreeService: orgTreeService,
		kgRepo:         kgRepo,
		chunkRepo:      chunkRepo,
		userRepo:       userRepo,
		authorizer:     newSameTenantResourceAuthorizer(orgTreeService),
	}
}

func (s *kbVisibilityService) ListAccessibleKBs(ctx context.Context, userID string, tenantID uint64, isSuperAdmin bool) ([]*types.KnowledgeBase, error) {
	logger.Infof(ctx, "Listing accessible KBs for user %s in tenant %d (superAdmin=%v)", userID, tenantID, isSuperAdmin)

	if isSuperAdmin {
		kbs, err := s.kbRepo.ListKnowledgeBasesByTenantID(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("failed to list KBs: %w", err)
		}
		s.fillKnowledgeCounts(ctx, kbs)
		s.fillCreatorNames(ctx, kbs)
		s.applyUserKBPins(ctx, tenantID, userID, kbs)
		return kbs, nil
	}

	scope, err := s.authorizer.resolveScope(ctx, userID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve knowledge-base visibility scope: %w", err)
	}

	kbs, err := s.kbRepo.ListAccessibleKBs(ctx, userID, tenantID, scope.readOrgList)
	if err != nil {
		return nil, fmt.Errorf("failed to list accessible KBs: %w", err)
	}

	s.fillKnowledgeCounts(ctx, kbs)
	s.fillCreatorNames(ctx, kbs)
	s.applyUserKBPins(ctx, tenantID, userID, kbs)
	return kbs, nil
}

func (s *kbVisibilityService) fillKnowledgeCounts(ctx context.Context, kbs []*types.KnowledgeBase) {
	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		kb.EnsureDefaults()
		tenantID := kb.TenantID
		switch kb.Type {
		case types.KnowledgeBaseTypeDocument, types.KnowledgeBaseTypeWiki:
			if cnt, err := s.kgRepo.CountKnowledgeByKnowledgeBaseID(ctx, tenantID, kb.ID); err == nil {
				kb.KnowledgeCount = cnt
			}
		case types.KnowledgeBaseTypeFAQ:
			if cnt, err := s.chunkRepo.CountChunksByKnowledgeBaseID(ctx, tenantID, kb.ID); err == nil {
				kb.ChunkCount = cnt
			}
		}

		if processingCount, err := s.kgRepo.CountKnowledgeByStatus(ctx, tenantID, kb.ID, []string{"pending", "processing"}); err == nil {
			kb.IsProcessing = processingCount > 0
			kb.ProcessingCount = processingCount
		} else {
			logger.Warnf(ctx, "Failed to get processing count for KB %s: %v", kb.ID, err)
		}
	}
}

func (s *kbVisibilityService) CanAccessKB(ctx context.Context, userID string, tenantID uint64, kbID string, isSuperAdmin bool) (bool, error) {
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
		CreatedBy:      kb.CreatorID,
	}, userID, isSuperAdmin, scope), nil
}

func (s *kbVisibilityService) CanManageKB(ctx context.Context, userID string, tenantID uint64, kbID string, isSuperAdmin bool) (bool, error) {
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
		CreatedBy:      kb.CreatorID,
	}, userID, isSuperAdmin, scope), nil
}

func (s *kbVisibilityService) fillCreatorNames(ctx context.Context, kbs []*types.KnowledgeBase) {
	fillKnowledgeBaseCreatorNames(ctx, s.userRepo, kbs)
}

func (s *kbVisibilityService) applyUserKBPins(ctx context.Context, tenantID uint64, userID string, kbs []*types.KnowledgeBase) {
	if len(kbs) == 0 || userID == "" {
		return
	}
	pins, err := s.kbRepo.ListUserKBPinIDs(ctx, tenantID, userID)
	if err != nil {
		logger.Warnf(ctx, "applyUserKBPins: failed to load pins for tenant=%d user=%s: %v", tenantID, userID, err)
		return
	}
	if len(pins) == 0 {
		return
	}
	for _, kb := range kbs {
		if ts, ok := pins[kb.ID]; ok {
			kb.IsPinned = true
			t := ts
			kb.PinnedAt = &t
		}
	}
	sort.SliceStable(kbs, func(i, j int) bool {
		a, b := kbs[i], kbs[j]
		if a.IsPinned != b.IsPinned {
			return a.IsPinned
		}
		if a.IsPinned && b.IsPinned {
			at, bt := a.PinnedAt, b.PinnedAt
			if at != nil && bt != nil && !at.Equal(*bt) {
				return at.After(*bt)
			}
		}
		return a.CreatedAt.After(b.CreatedAt)
	})
}
