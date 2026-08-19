package service

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// AgentKBScopeResolver centralizes how agent runtime KB scope is resolved for
// suggestion, QA, @mention, and IM command paths.
type AgentKBScopeResolver struct {
	kbService        interfaces.KnowledgeBaseService
	kbVisibility     interfaces.KBVisibilityService
	kbShareService   interfaces.KBShareService
	knowledgeService interfaces.KnowledgeService
}

func NewAgentKBScopeResolver(
	kbService interfaces.KnowledgeBaseService,
	kbVisibility interfaces.KBVisibilityService,
	kbShareService interfaces.KBShareService,
	knowledgeService interfaces.KnowledgeService,
) *AgentKBScopeResolver {
	return &AgentKBScopeResolver{
		kbService:        kbService,
		kbVisibility:     kbVisibility,
		kbShareService:   kbShareService,
		knowledgeService: knowledgeService,
	}
}

func (r *AgentKBScopeResolver) ResolveKnowledgeBaseIDs(
	ctx context.Context,
	agent *types.CustomAgent,
	sessionTenantID uint64,
) ([]string, error) {
	kbs, err := r.ResolveKnowledgeBases(ctx, agent, sessionTenantID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(kbs))
	for _, kb := range kbs {
		if kb == nil || strings.TrimSpace(kb.ID) == "" {
			continue
		}
		ids = append(ids, kb.ID)
	}
	return ids, nil
}

func (r *AgentKBScopeResolver) ResolveKnowledgeBases(
	ctx context.Context,
	agent *types.CustomAgent,
	sessionTenantID uint64,
) ([]*types.KnowledgeBase, error) {
	if r == nil || agent == nil {
		return nil, nil
	}

	mode := strings.ToLower(strings.TrimSpace(agent.Config.KBSelectionMode))
	if mode == "" {
		mode = "selected"
	}

	switch mode {
	case "none":
		return nil, nil
	case "all":
		return r.resolveAllKnowledgeBases(ctx, agent, sessionTenantID)
	default:
		return r.resolveConfiguredKnowledgeBases(ctx, agent, sessionTenantID)
	}
}

func (r *AgentKBScopeResolver) RestrictKnowledgeTargets(
	ctx context.Context,
	agent *types.CustomAgent,
	sessionTenantID uint64,
	kbIDs []string,
	knowledgeIDs []string,
	tagScopes []types.TagScope,
) ([]string, []string, []types.TagScope, error) {
	kbIDs = uniqueNonEmptyStrings(kbIDs)
	knowledgeIDs = uniqueNonEmptyStrings(knowledgeIDs)
	if agent == nil {
		return kbIDs, knowledgeIDs, tagScopes, nil
	}

	requestedKBIDs := append([]string(nil), kbIDs...)
	for _, scope := range tagScopes {
		if strings.TrimSpace(scope.KnowledgeBaseID) != "" {
			requestedKBIDs = append(requestedKBIDs, scope.KnowledgeBaseID)
		}
	}

	var knowledgeList []*types.Knowledge
	var err error
	if len(knowledgeIDs) > 0 && r.knowledgeService != nil {
		knowledgeList, err = r.knowledgeService.GetKnowledgeBatchWithSharedAccess(ctx, agent.TenantID, knowledgeIDs)
		if err != nil {
			logger.Warnf(ctx, "Failed to validate knowledge IDs for agent %s scope: %v", agent.ID, err)
			knowledgeList = nil
		}
		for _, knowledge := range knowledgeList {
			if knowledge != nil && strings.TrimSpace(knowledge.KnowledgeBaseID) != "" {
				requestedKBIDs = append(requestedKBIDs, knowledge.KnowledgeBaseID)
			}
		}
	}

	allowedKBs, err := r.filterCandidateKnowledgeBases(ctx, agent, sessionTenantID, requestedKBIDs)
	if err != nil {
		return nil, nil, nil, err
	}
	allowedSet := make(map[string]struct{}, len(allowedKBs))
	for _, kb := range allowedKBs {
		if kb == nil || strings.TrimSpace(kb.ID) == "" {
			continue
		}
		allowedSet[kb.ID] = struct{}{}
	}

	filterKBID := func(kbID string) bool {
		_, ok := allowedSet[kbID]
		return ok
	}

	filteredKBIDs := make([]string, 0, len(kbIDs))
	for _, kbID := range kbIDs {
		if filterKBID(kbID) {
			filteredKBIDs = append(filteredKBIDs, kbID)
			continue
		}
		logger.Warnf(ctx, "Blocking KB %s from agent %s scope", kbID, agent.ID)
	}

	filteredTagScopes := make([]types.TagScope, 0, len(tagScopes))
	for _, scope := range tagScopes {
		if scope.KnowledgeBaseID == "" {
			continue
		}
		if filterKBID(scope.KnowledgeBaseID) {
			filteredTagScopes = append(filteredTagScopes, scope)
			continue
		}
		logger.Warnf(ctx, "Blocking tag scope for KB %s from agent %s scope", scope.KnowledgeBaseID, agent.ID)
	}

	if len(knowledgeIDs) == 0 {
		return filteredKBIDs, nil, filteredTagScopes, nil
	}
	if len(knowledgeList) == 0 {
		return filteredKBIDs, nil, filteredTagScopes, nil
	}

	filteredKnowledgeIDs := make([]string, 0, len(knowledgeList))
	for _, knowledge := range knowledgeList {
		if knowledge == nil {
			continue
		}
		if filterKBID(knowledge.KnowledgeBaseID) {
			filteredKnowledgeIDs = append(filteredKnowledgeIDs, knowledge.ID)
			continue
		}
		logger.Warnf(ctx, "Blocking knowledge %s (KB %s) from agent %s scope", knowledge.ID, knowledge.KnowledgeBaseID, agent.ID)
	}

	return filteredKBIDs, filteredKnowledgeIDs, filteredTagScopes, nil
}

func (r *AgentKBScopeResolver) filterCandidateKnowledgeBases(
	ctx context.Context,
	agent *types.CustomAgent,
	sessionTenantID uint64,
	candidateKBIDs []string,
) ([]*types.KnowledgeBase, error) {
	candidateKBIDs = uniqueNonEmptyStrings(candidateKBIDs)
	if len(candidateKBIDs) == 0 || r.kbService == nil || agent == nil {
		return nil, nil
	}

	kbs, err := r.kbService.GetKnowledgeBasesByIDsOnly(ctx, candidateKBIDs)
	if err != nil {
		return nil, err
	}
	kbByID := make(map[string]*types.KnowledgeBase, len(kbs))
	for _, kb := range kbs {
		if kb != nil {
			kbByID[kb.ID] = kb
		}
	}

	mode := strings.ToLower(strings.TrimSpace(agent.Config.KBSelectionMode))
	if mode == "" {
		mode = "selected"
	}
	selectedSet := make(map[string]struct{}, len(agent.Config.KnowledgeBases))
	for _, kbID := range agent.Config.KnowledgeBases {
		if strings.TrimSpace(kbID) == "" {
			continue
		}
		selectedSet[kbID] = struct{}{}
	}

	isSharedAgent := sessionTenantID != 0 && sessionTenantID != agent.TenantID
	callerTenantID := agent.TenantID
	userID, hasRealUser, isPrivileged := resolveAgentScopeUser(ctx)
	callerRole := types.TenantRoleFromContext(ctx)

	result := make([]*types.KnowledgeBase, 0, len(candidateKBIDs))
	for _, kbID := range candidateKBIDs {
		kb := kbByID[kbID]
		if kb == nil {
			continue
		}
		if mode != "all" {
			if _, ok := selectedSet[kbID]; !ok {
				logger.Warnf(ctx, "Blocking KB %s: not configured on agent %s", kbID, agent.ID)
				continue
			}
		}

		if isSharedAgent {
			if mode == "all" && kb.TenantID != agent.TenantID {
				logger.Warnf(ctx, "Blocking KB %s from shared agent %s all-scope", kbID, agent.ID)
				continue
			}
			result = append(result, kb)
			continue
		}

		allowed := false
		switch {
		case kb.TenantID == callerTenantID && hasRealUser && r.kbVisibility != nil:
			allowed, err = r.kbVisibility.CanAccessKB(ctx, userID, callerTenantID, kb.ID, isPrivileged)
			if err != nil {
				return nil, err
			}
		case kb.TenantID == callerTenantID:
			allowed = true
		case r.kbShareService != nil:
			allowed, err = r.kbShareService.HasTenantKBPermission(ctx, kb.ID, callerTenantID, callerRole, types.OrgRoleViewer)
			if err != nil {
				return nil, err
			}
		}
		if !allowed {
			logger.Warnf(ctx, "Blocking KB %s from agent %s scope", kb.ID, agent.ID)
			continue
		}
		result = append(result, kb)
	}

	if mode == "all" {
		result = filterKnowledgeBasesByAgentCapabilities(ctx, agent, result, "requested_targets")
	}
	return dedupeKnowledgeBasesByID(result), nil
}

func (r *AgentKBScopeResolver) resolveAllKnowledgeBases(
	ctx context.Context,
	agent *types.CustomAgent,
	sessionTenantID uint64,
) ([]*types.KnowledgeBase, error) {
	isSharedAgent := sessionTenantID != 0 && sessionTenantID != agent.TenantID
	if isSharedAgent {
		if r.kbService == nil {
			return nil, nil
		}
		kbs, err := r.kbService.ListKnowledgeBasesByTenantID(ctx, agent.TenantID)
		if err != nil {
			return nil, err
		}
		return filterKnowledgeBasesByAgentCapabilities(ctx, agent, kbs, "shared_agent_all"), nil
	}

	localKBs, err := r.resolveLocalKnowledgeBases(ctx, agent.TenantID)
	if err != nil {
		return nil, err
	}

	merged := make([]*types.KnowledgeBase, 0, len(localKBs))
	merged = append(merged, localKBs...)
	if r.kbShareService != nil {
		sharedKBs, err := r.kbShareService.ListSharedKnowledgeBases(ctx, agent.TenantID, types.TenantRoleFromContext(ctx))
		if err != nil {
			return nil, err
		}
		for _, info := range sharedKBs {
			if info == nil || info.KnowledgeBase == nil {
				continue
			}
			merged = append(merged, info.KnowledgeBase)
		}
	}

	merged = dedupeKnowledgeBasesByID(merged)
	return filterKnowledgeBasesByAgentCapabilities(ctx, agent, merged, "local_agent_all"), nil
}

func (r *AgentKBScopeResolver) resolveConfiguredKnowledgeBases(
	ctx context.Context,
	agent *types.CustomAgent,
	sessionTenantID uint64,
) ([]*types.KnowledgeBase, error) {
	configuredIDs := uniqueNonEmptyStrings(agent.Config.KnowledgeBases)
	if len(configuredIDs) == 0 || r.kbService == nil {
		return nil, nil
	}

	kbs, err := r.kbService.GetKnowledgeBasesByIDsOnly(ctx, configuredIDs)
	if err != nil {
		return nil, err
	}
	kbByID := make(map[string]*types.KnowledgeBase, len(kbs))
	for _, kb := range kbs {
		if kb != nil {
			kbByID[kb.ID] = kb
		}
	}

	isSharedAgent := sessionTenantID != 0 && sessionTenantID != agent.TenantID
	callerTenantID := agent.TenantID
	userID, hasRealUser, isPrivileged := resolveAgentScopeUser(ctx)
	callerRole := types.TenantRoleFromContext(ctx)

	result := make([]*types.KnowledgeBase, 0, len(configuredIDs))
	for _, kbID := range configuredIDs {
		kb := kbByID[kbID]
		if kb == nil {
			continue
		}
		if isSharedAgent {
			result = append(result, kb)
			continue
		}

		allowed := false
		switch {
		case kb.TenantID == callerTenantID && hasRealUser && r.kbVisibility != nil:
			allowed, err = r.kbVisibility.CanAccessKB(ctx, userID, callerTenantID, kb.ID, isPrivileged)
			if err != nil {
				return nil, err
			}
		case kb.TenantID == callerTenantID:
			allowed = true
		case r.kbShareService != nil:
			allowed, err = r.kbShareService.HasTenantKBPermission(ctx, kb.ID, callerTenantID, callerRole, types.OrgRoleViewer)
			if err != nil {
				return nil, err
			}
		}

		if !allowed {
			logger.Warnf(ctx, "Blocking configured KB %s from agent %s scope", kb.ID, agent.ID)
			continue
		}
		result = append(result, kb)
	}

	return result, nil
}

func (r *AgentKBScopeResolver) resolveLocalKnowledgeBases(
	ctx context.Context,
	tenantID uint64,
) ([]*types.KnowledgeBase, error) {
	if tenantID == 0 {
		return nil, nil
	}
	userID, hasRealUser, isPrivileged := resolveAgentScopeUser(ctx)
	if hasRealUser && r.kbVisibility != nil {
		return r.kbVisibility.ListAccessibleKBs(ctx, userID, tenantID, isPrivileged)
	}
	if r.kbService == nil {
		return nil, nil
	}
	return r.kbService.ListKnowledgeBasesByTenantID(ctx, tenantID)
}

func resolveAgentScopeUser(ctx context.Context) (string, bool, bool) {
	userID, ok := types.UserIDFromContext(ctx)
	if !ok || strings.TrimSpace(userID) == "" || types.IsSyntheticUserID(userID) {
		return "", false, types.IsSystemAdminFromContext(ctx)
	}
	return userID, true, types.IsSystemAdminFromContext(ctx)
}

func filterKnowledgeBasesByAgentCapabilities(
	ctx context.Context,
	agent *types.CustomAgent,
	kbs []*types.KnowledgeBase,
	source string,
) []*types.KnowledgeBase {
	if agent == nil || len(kbs) == 0 {
		return kbs
	}
	filter := tools.DeriveKBFilterForAgent(agent.Config.AgentMode, agent.Config.AllowedTools)
	if filter.IsEmpty() {
		return kbs
	}
	kept := make([]*types.KnowledgeBase, 0, len(kbs))
	skipped := 0
	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		if !tools.KBSatisfiesAgentRequirements(kb.Capabilities(), agent.Config.AgentMode, agent.Config.AllowedTools) {
			skipped++
			continue
		}
		kept = append(kept, kb)
	}
	if skipped > 0 {
		logger.Infof(ctx,
			"Agent KB scope (%s) removed %d incompatible KBs for agent %s",
			source, skipped, agent.ID,
		)
	}
	return kept
}

func dedupeKnowledgeBasesByID(kbs []*types.KnowledgeBase) []*types.KnowledgeBase {
	seen := make(map[string]struct{}, len(kbs))
	out := make([]*types.KnowledgeBase, 0, len(kbs))
	for _, kb := range kbs {
		if kb == nil || strings.TrimSpace(kb.ID) == "" {
			continue
		}
		if _, ok := seen[kb.ID]; ok {
			continue
		}
		seen[kb.ID] = struct{}{}
		out = append(out, kb)
	}
	return out
}
