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
	userRepo       interfaces.UserRepository
}

// NewKBVisibilityService creates a new knowledge base visibility service
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
		return kbs, nil
	}

	// Get user's organizations within this tenant
	userOrgs, err := s.orgTreeService.GetUserOrganizations(ctx, userID, tenantID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get user organizations: %v", err)
		return nil, fmt.Errorf("failed to get user organizations: %w", err)
	}

	// Collect all related org IDs:
	// 1. User's orgs themselves
	// 2. Ancestors of user's orgs (kb in parent org should be visible to child org members)
	// 3. Descendants of user's orgs (for future sub-org scenarios)
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

	// Single batch call to get all descendants for all user orgs
	if len(pathPrefixes) > 0 {
		allDescendants, err := s.orgTreeService.GetDescendantIDsByPaths(ctx, pathPrefixes, tenantID)
		if err != nil {
			logger.Errorf(ctx, "Failed to batch get descendant org IDs: %v", err)
			// Fallback silently — orgIDSet already has user's direct orgs + ancestors
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

	// Query KBs with visibility rules
	kbs, err := s.kbRepo.ListAccessibleKBs(ctx, userID, tenantID, orgIDs)
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

		switch kb.Type {
		case types.KnowledgeBaseTypeDocument:
			if cnt, err := s.kgRepo.CountKnowledgeByKnowledgeBaseID(ctx, tenantID, kb.ID); err == nil {
				kb.KnowledgeCount = cnt
			} else {
				logger.Warnf(ctx, "Failed to get knowledge count for KB %s: %v", kb.ID, err)
			}
		case types.KnowledgeBaseTypeFAQ:
			if cnt, err := s.chunkRepo.CountChunksByKnowledgeBaseID(ctx, tenantID, kb.ID); err == nil {
				kb.ChunkCount = cnt
			} else {
				logger.Warnf(ctx, "Failed to get chunk count for KB %s: %v", kb.ID, err)
			}
		}

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

	switch kb.Visibility {
	case types.KBVisibilityGlobal, "":
		// Global or legacy KBs are accessible to everyone in the tenant
		return true, nil
	case types.KBVisibilityPrivate:
		// Private KBs are only accessible to the creator
		return kb.CreatedBy == userID, nil
	case types.KBVisibilityOrg:
		// Check if user is in the KB's organization or any of its ancestors
		if kb.OrganizationID == "" {
			return false, nil
		}
		userOrgs, err := s.orgTreeService.GetUserOrganizations(ctx, userID, tenantID)
		if err != nil {
			return false, fmt.Errorf("failed to check user organizations: %w", err)
		}
		// Build set of all org IDs user can see through
		for _, org := range userOrgs {
			descendantIDs, err := s.orgTreeService.GetOrgAndDescendantIDs(ctx, org.ID, tenantID)
			if err != nil {
				continue
			}
			for _, id := range descendantIDs {
				if id == kb.OrganizationID {
					return true, nil
				}
			}
		}
		return false, nil
	default:
		return false, nil
	}
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

	// Creator can always manage
	if kb.CreatedBy == userID {
		return true, nil
	}

	// For global KBs, only the creator can manage (non-super-admin users cannot manage global KBs they didn't create)
	if kb.Visibility == types.KBVisibilityGlobal || kb.Visibility == "" {
		return kb.CreatedBy == userID, nil
	}

	// For org KBs, check if user is admin of the organization
	if kb.Visibility == types.KBVisibilityOrg && kb.OrganizationID != "" {
		userOrgs, err := s.orgTreeService.GetUserOrganizations(ctx, userID, tenantID)
		if err != nil {
			return false, fmt.Errorf("failed to check user organizations: %w", err)
		}
		for _, org := range userOrgs {
			if org.ID == kb.OrganizationID {
				// User is a member of the KB's organization — check if admin/editor
				// For now, any member of the org can manage org KBs
				return true, nil
			}
		}
	}

	return false, nil
}

// fillCreatorNicknames fills the CreatedByNickname field for each knowledge base
func (s *kbVisibilityService) fillCreatorNicknames(ctx context.Context, kbs []*types.KnowledgeBase) {
	// Batch query all creator IDs
	creatorIDs := make(map[string]bool)
	for _, kb := range kbs {
		if kb != nil && kb.CreatedBy != "" {
			creatorIDs[kb.CreatedBy] = true
		}
	}

	// Query users by IDs
	userMap := make(map[string]string)
	for creatorID := range creatorIDs {
		// Check for built-in system knowledge bases
		if types.IsBuiltinAgentID(creatorID) || creatorID == "system" {
			userMap[creatorID] = "系统"
			continue
		}

		user, err := s.userRepo.GetUserByID(ctx, creatorID)
		if err != nil {
			logger.Warnf(ctx, "Failed to get user %s: %v", creatorID, err)
			continue
		}
		if user != nil {
			// Use username as nickname
			userMap[creatorID] = user.Username
		}
	}

	// Fill nicknames
	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		if kb.Visibility == types.KBVisibilityGlobal {
			// Global knowledge bases show "系统"
			kb.CreatedByNickname = "系统"
		} else if nickname, ok := userMap[kb.CreatedBy]; ok {
			kb.CreatedByNickname = nickname
		} else if kb.CreatedBy != "" {
			// Fallback: show user ID if username not found
			kb.CreatedByNickname = kb.CreatedBy
		}
	}
}
