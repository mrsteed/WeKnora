package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type agentScopeKBService struct {
	interfaces.KnowledgeBaseService
	allByTenant map[uint64][]*types.KnowledgeBase
	byID        map[string]*types.KnowledgeBase
}

func (s *agentScopeKBService) ListKnowledgeBasesByTenantID(_ context.Context, tenantID uint64) ([]*types.KnowledgeBase, error) {
	return append([]*types.KnowledgeBase(nil), s.allByTenant[tenantID]...), nil
}

func (s *agentScopeKBService) GetKnowledgeBasesByIDsOnly(_ context.Context, ids []string) ([]*types.KnowledgeBase, error) {
	result := make([]*types.KnowledgeBase, 0, len(ids))
	for _, id := range ids {
		if kb := s.byID[id]; kb != nil {
			result = append(result, kb)
		}
	}
	return result, nil
}

type agentScopeKBVisibility struct {
	interfaces.KBVisibilityService
	accessible []*types.KnowledgeBase
	canAccess  map[string]bool
}

func (s *agentScopeKBVisibility) ListAccessibleKBs(_ context.Context, _ string, _ uint64, _ bool) ([]*types.KnowledgeBase, error) {
	return append([]*types.KnowledgeBase(nil), s.accessible...), nil
}

func (s *agentScopeKBVisibility) CanAccessKB(_ context.Context, _ string, _ uint64, kbID string, _ bool) (bool, error) {
	return s.canAccess[kbID], nil
}

type agentScopeKBShareService struct {
	interfaces.KBShareService
	shared  []*types.SharedKnowledgeBaseInfo
	allowed map[string]bool
}

func (s *agentScopeKBShareService) ListSharedKnowledgeBases(_ context.Context, _ uint64, _ types.TenantRole) ([]*types.SharedKnowledgeBaseInfo, error) {
	return append([]*types.SharedKnowledgeBaseInfo(nil), s.shared...), nil
}

func (s *agentScopeKBShareService) HasTenantKBPermission(
	_ context.Context,
	kbID string,
	_ uint64,
	_ types.TenantRole,
	_ types.OrgMemberRole,
) (bool, error) {
	return s.allowed[kbID], nil
}

type agentScopeKnowledgeService struct {
	interfaces.KnowledgeService
	byID map[string]*types.Knowledge
}

func (s *agentScopeKnowledgeService) GetKnowledgeBatchWithSharedAccess(_ context.Context, _ uint64, ids []string) ([]*types.Knowledge, error) {
	result := make([]*types.Knowledge, 0, len(ids))
	for _, id := range ids {
		if knowledge := s.byID[id]; knowledge != nil {
			result = append(result, knowledge)
		}
	}
	return result, nil
}

func agentScopeTestContext() context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(100))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-1")
	ctx = context.WithValue(ctx, types.TenantRoleContextKey, types.TenantRoleViewer)
	return ctx
}

func kbIDsFromList(kbs []*types.KnowledgeBase) []string {
	ids := make([]string, 0, len(kbs))
	for _, kb := range kbs {
		if kb != nil {
			ids = append(ids, kb.ID)
		}
	}
	return ids
}

func kbIDsFromScopes(scopes []types.TagScope) []string {
	ids := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		ids = append(ids, scope.KnowledgeBaseID)
	}
	return ids
}

func TestAgentKBScopeResolverResolveKnowledgeBasesAllModeUsesAccessibleAndShared(t *testing.T) {
	resolver := NewAgentKBScopeResolver(
		&agentScopeKBService{allByTenant: map[uint64][]*types.KnowledgeBase{
			100: {{ID: "local-hidden", TenantID: 100, Name: "Local Hidden"}},
		}},
		&agentScopeKBVisibility{accessible: []*types.KnowledgeBase{{ID: "local-visible", TenantID: 100, Name: "Local Visible"}}},
		&agentScopeKBShareService{shared: []*types.SharedKnowledgeBaseInfo{{
			KnowledgeBase: &types.KnowledgeBase{ID: "shared-visible", TenantID: 200, Name: "Shared Visible"},
		}}},
		nil,
	)
	agent := &types.CustomAgent{
		ID:       "agent-1",
		TenantID: 100,
		Config: types.CustomAgentConfig{
			KBSelectionMode: "all",
			AgentMode:       types.AgentModeSmartReasoning,
			AllowedTools:    []string{"thinking"},
		},
	}

	kbs, err := resolver.ResolveKnowledgeBases(agentScopeTestContext(), agent, 100)
	require.NoError(t, err)
	assert.Equal(t, []string{"local-visible", "shared-visible"}, kbIDsFromList(kbs))
}

func TestAgentKBScopeResolverRestrictKnowledgeTargetsFiltersConfiguredAndVisible(t *testing.T) {
	resolver := NewAgentKBScopeResolver(
		&agentScopeKBService{byID: map[string]*types.KnowledgeBase{
			"local-allowed":  {ID: "local-allowed", TenantID: 100, Name: "Local Allowed"},
			"local-blocked":  {ID: "local-blocked", TenantID: 100, Name: "Local Blocked"},
			"shared-allowed": {ID: "shared-allowed", TenantID: 200, Name: "Shared Allowed"},
		}},
		&agentScopeKBVisibility{canAccess: map[string]bool{
			"local-allowed": true,
			"local-blocked": false,
		}},
		&agentScopeKBShareService{allowed: map[string]bool{
			"shared-allowed": true,
		}},
		&agentScopeKnowledgeService{byID: map[string]*types.Knowledge{
			"doc-1": {ID: "doc-1", TenantID: 100, KnowledgeBaseID: "local-allowed"},
			"doc-2": {ID: "doc-2", TenantID: 100, KnowledgeBaseID: "local-blocked"},
			"doc-3": {ID: "doc-3", TenantID: 200, KnowledgeBaseID: "shared-allowed"},
		}},
	)
	agent := &types.CustomAgent{
		ID:       "agent-1",
		TenantID: 100,
		Config: types.CustomAgentConfig{
			KBSelectionMode: "selected",
			KnowledgeBases:  []string{"local-allowed", "local-blocked", "shared-allowed"},
		},
	}

	kbIDs, knowledgeIDs, tagScopes, err := resolver.RestrictKnowledgeTargets(
		agentScopeTestContext(),
		agent,
		100,
		[]string{"local-allowed", "local-blocked", "shared-allowed"},
		[]string{"doc-1", "doc-2", "doc-3"},
		[]types.TagScope{
			{KnowledgeBaseID: "local-blocked", TagIDs: []string{"tag-hidden"}},
			{KnowledgeBaseID: "shared-allowed", TagIDs: []string{"tag-shared"}},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"local-allowed", "shared-allowed"}, kbIDs)
	assert.Equal(t, []string{"doc-1", "doc-3"}, knowledgeIDs)
	assert.Equal(t, []string{"shared-allowed"}, kbIDsFromScopes(tagScopes))
}
