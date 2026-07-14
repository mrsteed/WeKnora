package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type stubAgentScopeKBVisibility struct {
	interfaces.KBVisibilityService
	list func(context.Context, string, uint64, bool) ([]*types.KnowledgeBase, error)
}

func (s *stubAgentScopeKBVisibility) ListAccessibleKBs(ctx context.Context, userID string, tenantID uint64, isSuperAdmin bool) ([]*types.KnowledgeBase, error) {
	if s.list == nil {
		return nil, nil
	}
	return s.list(ctx, userID, tenantID, isSuperAdmin)
}

type stubAgentScopeKBShare struct {
	interfaces.KBShareService
	shared []*types.SharedKnowledgeBaseInfo
}

func (s *stubAgentScopeKBShare) ListSharedKnowledgeBases(context.Context, string, uint64) ([]*types.SharedKnowledgeBaseInfo, error) {
	return s.shared, nil
}

func agentScopeTestContext() context.Context {
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-1")
	ctx = context.WithValue(ctx, types.UserContextKey, &types.User{ID: "user-1"})
	return ctx
}

func agentScopeIMTestContext() context.Context {
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "system-7")
	ctx = context.WithValue(ctx, types.TenantRoleContextKey, types.TenantRoleViewer)
	ctx = types.WithResourceAuthUserID(ctx, "user-1")
	ctx = types.WithResourceAuthUser(ctx, &types.User{ID: "user-1"})
	return ctx
}

func TestResolveAgentKnowledgeBasesForCurrentUser_AllModeUsesAccessibleAndDirectSharedKBs(t *testing.T) {
	ctx := agentScopeTestContext()
	visibility := &stubAgentScopeKBVisibility{list: func(_ context.Context, userID string, tenantID uint64, isSuperAdmin bool) ([]*types.KnowledgeBase, error) {
		require.Equal(t, "user-1", userID)
		require.Equal(t, uint64(7), tenantID)
		require.False(t, isSuperAdmin)
		return []*types.KnowledgeBase{
			{ID: "kb-visible-1", TenantID: 7},
			{ID: "kb-visible-2", TenantID: 7},
		}, nil
	}}
	shareSvc := &stubAgentScopeKBShare{shared: []*types.SharedKnowledgeBaseInfo{
		{KnowledgeBase: &types.KnowledgeBase{ID: "kb-shared", TenantID: 99}},
	}}

	kbs, ok, err := ResolveAgentKnowledgeBasesForCurrentUser(ctx, &types.CustomAgent{
		ID:       "agent-1",
		TenantID: 7,
		Config: types.CustomAgentConfig{
			KBSelectionMode: "all",
		},
	}, 7, visibility, shareSvc)
	require.True(t, ok)
	require.NoError(t, err)
	require.Len(t, kbs, 3)
	require.Equal(t, []string{"kb-visible-1", "kb-visible-2", "kb-shared"}, []string{kbs[0].ID, kbs[1].ID, kbs[2].ID})
}

func TestResolveAgentKnowledgeBasesForCurrentUser_SelectedModeIntersectsAccessibleScope(t *testing.T) {
	ctx := agentScopeTestContext()
	visibility := &stubAgentScopeKBVisibility{list: func(context.Context, string, uint64, bool) ([]*types.KnowledgeBase, error) {
		return []*types.KnowledgeBase{{ID: "kb-visible", TenantID: 7}}, nil
	}}
	shareSvc := &stubAgentScopeKBShare{shared: []*types.SharedKnowledgeBaseInfo{
		{KnowledgeBase: &types.KnowledgeBase{ID: "kb-shared", TenantID: 88}},
	}}

	kbs, ok, err := ResolveAgentKnowledgeBasesForCurrentUser(ctx, &types.CustomAgent{
		ID:       "agent-1",
		TenantID: 7,
		Config: types.CustomAgentConfig{
			KBSelectionMode: "selected",
			KnowledgeBases:  []string{"kb-hidden", "kb-visible", "kb-shared"},
		},
	}, 7, visibility, shareSvc)
	require.True(t, ok)
	require.NoError(t, err)
	require.Len(t, kbs, 2)
	require.Equal(t, []string{"kb-visible", "kb-shared"}, []string{kbs[0].ID, kbs[1].ID})
}

func TestResolveAgentKnowledgeBasesForCurrentUser_SkipsCrossTenantSharedAgent(t *testing.T) {
	ctx := agentScopeTestContext()
	kbs, ok, err := ResolveAgentKnowledgeBasesForCurrentUser(ctx, &types.CustomAgent{
		ID:       "agent-1",
		TenantID: 9,
		Config: types.CustomAgentConfig{
			KBSelectionMode: "all",
		},
	}, 7, &stubAgentScopeKBVisibility{}, &stubAgentScopeKBShare{})
	require.False(t, ok)
	require.NoError(t, err)
	require.Nil(t, kbs)
}

func TestSessionResolveKnowledgeBasesFromAgent_UsesCurrentUserScopeForSameTenantAllMode(t *testing.T) {
	ctx := agentScopeTestContext()
	svc := &sessionService{
		kbVisibility: &stubAgentScopeKBVisibility{list: func(context.Context, string, uint64, bool) ([]*types.KnowledgeBase, error) {
			return []*types.KnowledgeBase{{ID: "kb-visible", TenantID: 7}}, nil
		}},
		kbShareService: &stubAgentScopeKBShare{shared: []*types.SharedKnowledgeBaseInfo{{KnowledgeBase: &types.KnowledgeBase{ID: "kb-shared", TenantID: 42}}}},
	}

	kbIDs := svc.resolveKnowledgeBasesFromAgent(ctx, &types.CustomAgent{
		ID:       "agent-1",
		TenantID: 7,
		Config: types.CustomAgentConfig{
			KBSelectionMode: "all",
		},
	}, 7)
	require.Equal(t, []string{"kb-visible", "kb-shared"}, kbIDs)
}

func TestResolveAgentKnowledgeBasesForCurrentUser_UsesResourceAuthUserWhenSessionOwnerIsSynthetic(t *testing.T) {
	ctx := agentScopeIMTestContext()
	visibility := &stubAgentScopeKBVisibility{list: func(_ context.Context, userID string, tenantID uint64, isSuperAdmin bool) ([]*types.KnowledgeBase, error) {
		require.Equal(t, "user-1", userID)
		require.Equal(t, uint64(7), tenantID)
		require.False(t, isSuperAdmin)
		return []*types.KnowledgeBase{{ID: "kb-visible", TenantID: 7}}, nil
	}}

	kbs, ok, err := ResolveAgentKnowledgeBasesForCurrentUser(ctx, &types.CustomAgent{
		ID:       "agent-1",
		TenantID: 7,
		Config: types.CustomAgentConfig{
			KBSelectionMode: "all",
		},
	}, 7, visibility, &stubAgentScopeKBShare{})
	require.True(t, ok)
	require.NoError(t, err)
	require.Len(t, kbs, 1)
	require.Equal(t, "kb-visible", kbs[0].ID)
}