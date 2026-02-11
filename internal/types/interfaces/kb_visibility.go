package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// KBVisibilityService defines the knowledge base visibility service
type KBVisibilityService interface {
	// ListAccessibleKBs returns all knowledge bases accessible to a user within a tenant,
	// considering visibility rules: global KBs + org KBs (user's orgs) + private KBs (own)
	// Super admins can see all KBs
	ListAccessibleKBs(ctx context.Context, userID string, tenantID uint64, isSuperAdmin bool) ([]*types.KnowledgeBase, error)
	// CanAccessKB checks whether a user can access (read) a specific knowledge base
	CanAccessKB(ctx context.Context, userID string, tenantID uint64, kbID string, isSuperAdmin bool) (bool, error)
	// CanManageKB checks whether a user can manage (edit/delete) a specific knowledge base
	CanManageKB(ctx context.Context, userID string, tenantID uint64, kbID string, isSuperAdmin bool) (bool, error)
}
