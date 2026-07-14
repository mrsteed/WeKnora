package service

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// ResolveAgentKnowledgeBasesForCurrentUser resolves the effective knowledge
// bases for the current human caller when the agent belongs to the caller's
// active tenant. It returns ok=false when the optimization should not apply,
// for example cross-tenant shared agents or when no human resource-auth user
// can be identified.
//
// Scope rules for this first-phase fix:
//   - Same-tenant human callers only
//   - Prefer explicit resource-auth user over session owner identity
//   - Base scope = same-tenant accessible KBs + directly shared KBs
//   - Agent mode then narrows that base scope according to KBSelectionMode
func ResolveAgentKnowledgeBasesForCurrentUser(
	ctx context.Context,
	agent *types.CustomAgent,
	currentTenantID uint64,
	kbVisibility interfaces.KBVisibilityService,
	kbShareService interfaces.KBShareService,
) ([]*types.KnowledgeBase, bool, error) {
	if agent == nil || currentTenantID == 0 || agent.TenantID == 0 || agent.TenantID != currentTenantID {
		return nil, false, nil
	}
	if kbVisibility == nil {
		return nil, false, nil
	}
	userID, user, ok := resolveAgentScopeUserFromContext(ctx)
	if !ok {
		return nil, false, nil
	}

	bypassVisibility := false
	if user != nil {
		bypassVisibility = types.IsPlatformPrivilegedUser(user)
	}

	accessible, err := kbVisibility.ListAccessibleKBs(ctx, userID, currentTenantID, bypassVisibility)
	if err != nil {
		return nil, true, err
	}

	baseScope, byID := dedupeKnowledgeBaseScope(accessible)
	if kbShareService != nil {
		shared, err := kbShareService.ListSharedKnowledgeBases(ctx, userID, currentTenantID)
		if err != nil {
			logger.Warnf(ctx, "ResolveAgentKnowledgeBasesForCurrentUser: failed to list shared knowledge bases: %v", err)
		} else {
			for _, item := range shared {
				if item == nil || item.KnowledgeBase == nil {
					continue
				}
				baseScope = appendKnowledgeBaseIfMissing(baseScope, byID, item.KnowledgeBase)
			}
		}
	}

	switch strings.TrimSpace(agent.Config.KBSelectionMode) {
	case "all":
		return baseScope, true, nil
	case "none":
		return []*types.KnowledgeBase{}, true, nil
	case "selected", "":
		return selectKnowledgeBasesFromScope(ctx, agent, byID), true, nil
	default:
		if len(agent.Config.KnowledgeBases) > 0 {
			return selectKnowledgeBasesFromScope(ctx, agent, byID), true, nil
		}
		return []*types.KnowledgeBase{}, true, nil
	}
}

func resolveAgentScopeUserFromContext(ctx context.Context) (string, *types.User, bool) {
	if userID, ok := types.ResourceAuthUserIDFromContext(ctx); ok && !types.IsSyntheticUserID(userID) {
		user, _ := types.ResourceAuthUserFromContext(ctx)
		return userID, user, true
	}
	userID, ok := types.UserIDFromContext(ctx)
	if !ok || strings.TrimSpace(userID) == "" || types.IsSyntheticUserID(userID) {
		return "", nil, false
	}
	user, _ := ctx.Value(types.UserContextKey).(*types.User)
	return userID, user, true
}

func dedupeKnowledgeBaseScope(kbs []*types.KnowledgeBase) ([]*types.KnowledgeBase, map[string]*types.KnowledgeBase) {
	result := make([]*types.KnowledgeBase, 0, len(kbs))
	byID := make(map[string]*types.KnowledgeBase, len(kbs))
	for _, kb := range kbs {
		result = appendKnowledgeBaseIfMissing(result, byID, kb)
	}
	return result, byID
}

func appendKnowledgeBaseIfMissing(
	result []*types.KnowledgeBase,
	byID map[string]*types.KnowledgeBase,
	kb *types.KnowledgeBase,
) []*types.KnowledgeBase {
	if kb == nil || strings.TrimSpace(kb.ID) == "" {
		return result
	}
	if _, exists := byID[kb.ID]; exists {
		return result
	}
	byID[kb.ID] = kb
	return append(result, kb)
}

func selectKnowledgeBasesFromScope(
	ctx context.Context,
	agent *types.CustomAgent,
	byID map[string]*types.KnowledgeBase,
) []*types.KnowledgeBase {
	if agent == nil || len(agent.Config.KnowledgeBases) == 0 {
		return []*types.KnowledgeBase{}
	}
	selected := make([]*types.KnowledgeBase, 0, len(agent.Config.KnowledgeBases))
	seen := make(map[string]struct{}, len(agent.Config.KnowledgeBases))
	dropped := 0
	for _, kbID := range agent.Config.KnowledgeBases {
		trimmed := strings.TrimSpace(kbID)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		kb, ok := byID[trimmed]
		if !ok || kb == nil {
			dropped++
			continue
		}
		selected = append(selected, kb)
	}
	if dropped > 0 {
		logger.Infof(ctx,
			"ResolveAgentKnowledgeBasesForCurrentUser(agent=%s, mode=selected): visibility filter removed %d configured KBs",
			agent.ID, dropped)
	}
	return selected
}