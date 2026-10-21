package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// KBVisibilityService resolves same-tenant visibility and manageability of
// knowledge bases using local org-tree semantics.
type KBVisibilityService interface {
	ListAccessibleKBs(ctx context.Context, userID string, tenantID uint64, isSuperAdmin bool) ([]*types.KnowledgeBase, error)
	CanAccessKB(ctx context.Context, userID string, tenantID uint64, kbID string, isSuperAdmin bool) (bool, error)
	CanManageKB(ctx context.Context, userID string, tenantID uint64, kbID string, isSuperAdmin bool) (bool, error)
}
