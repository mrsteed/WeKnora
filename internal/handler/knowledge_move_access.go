package handler

import (
	"context"
	"strings"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

func moveCallerBypassesKBVisibility(c *gin.Context) bool {
	if c == nil {
		return false
	}
	userVal, _ := c.Get(types.UserContextKey.String())
	user, _ := userVal.(*types.User)
	return types.IsPlatformPrivilegedUser(user)
}

// requireMoveKBManagePermission enforces the KB permission model used by the
// knowledge-move workflow. Human callers must be able to manage the KB under
// same-tenant visibility rules; tenant API key callers keep their existing
// machine-principal semantics (route capability + KB allow-list) and are
// therefore not subject to human org-scope checks here.
func requireMoveKBManagePermission(
	c *gin.Context,
	kbVisibility interfaces.KBVisibilityService,
	kbID string,
) error {
	if c == nil {
		return apperrors.NewUnauthorizedError("Unauthorized")
	}
	ctx := c.Request.Context()
	if _, ok := types.TenantAPIKeyScopeFromContext(ctx); ok {
		return nil
	}
	if kbVisibility == nil {
		return apperrors.NewInternalServerError("knowledge base visibility service unavailable")
	}
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	userID := strings.TrimSpace(c.GetString(types.UserIDContextKey.String()))
	if tenantID == 0 || userID == "" {
		return apperrors.NewUnauthorizedError("Unauthorized")
	}
	canManage, err := kbVisibility.CanManageKB(ctx, userID, tenantID, kbID, moveCallerBypassesKBVisibility(c))
	if err != nil {
		return apperrors.NewInternalServerError(err.Error())
	}
	if !canManage {
		return apperrors.NewForbiddenError("No permission to operate on this knowledge base")
	}
	return nil
}

// listManageableMoveTargetKBs returns the candidate KB set for the move flow
// before source/target business-compatibility filters are applied.
//
// Human callers: current user's same-tenant visible KBs intersected with the
// KBs they can manage.
// Tenant API key callers: tenant KB list intersected with the key allow-list.
func listManageableMoveTargetKBs(
	c *gin.Context,
	kbService interfaces.KnowledgeBaseService,
	kbVisibility interfaces.KBVisibilityService,
) ([]*types.KnowledgeBase, error) {
	if c == nil {
		return nil, apperrors.NewUnauthorizedError("Unauthorized")
	}
	ctx := c.Request.Context()
	if _, ok := types.TenantAPIKeyScopeFromContext(ctx); ok {
		if kbService == nil {
			return nil, apperrors.NewInternalServerError("knowledge base service unavailable")
		}
		kbs, err := kbService.ListKnowledgeBases(ctx)
		if err != nil {
			return nil, apperrors.NewInternalServerError(err.Error())
		}
		return filterKnowledgeBasesForAPIKeyScope(ctx, kbs), nil
	}
	if kbVisibility == nil {
		return nil, apperrors.NewInternalServerError("knowledge base visibility service unavailable")
	}
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	userID := strings.TrimSpace(c.GetString(types.UserIDContextKey.String()))
	if tenantID == 0 || userID == "" {
		return nil, apperrors.NewUnauthorizedError("Unauthorized")
	}
	bypass := moveCallerBypassesKBVisibility(c)
	kbs, err := kbVisibility.ListAccessibleKBs(ctx, userID, tenantID, bypass)
	if err != nil {
		return nil, apperrors.NewInternalServerError(err.Error())
	}
	filtered := make([]*types.KnowledgeBase, 0, len(kbs))
	for _, kb := range kbs {
		if kb == nil || strings.TrimSpace(kb.ID) == "" {
			continue
		}
		canManage, err := kbVisibility.CanManageKB(ctx, userID, tenantID, kb.ID, bypass)
		if err != nil {
			return nil, apperrors.NewInternalServerError(err.Error())
		}
		if canManage {
			filtered = append(filtered, kb)
		}
	}
	return filtered, nil
}

func moveTargetBusinessCompatible(sourceKB *types.KnowledgeBase, candidate *types.KnowledgeBase) bool {
	if sourceKB == nil || candidate == nil {
		return false
	}
	if candidate.ID == sourceKB.ID {
		return false
	}
	if candidate.IsTemporary {
		return false
	}
	if candidate.Type != sourceKB.Type {
		return false
	}
	if candidate.EmbeddingModelID != sourceKB.EmbeddingModelID {
		return false
	}
	return true
}

func filterMoveTargetsByBusinessRules(sourceKB *types.KnowledgeBase, candidates []*types.KnowledgeBase) []*types.KnowledgeBase {
	filtered := make([]*types.KnowledgeBase, 0, len(candidates))
	for _, kb := range candidates {
		if moveTargetBusinessCompatible(sourceKB, kb) {
			filtered = append(filtered, kb)
		}
	}
	return filtered
}

func contextWithTenantAPIKeyScope(ctx context.Context, scope types.TenantAPIKeyScope) context.Context {
	return types.WithTenantAPIKeyScope(ctx, scope)
}
