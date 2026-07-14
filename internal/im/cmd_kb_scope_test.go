package im

import (
	"context"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type stubIMKBVisibility struct {
	interfaces.KBVisibilityService
	list func(context.Context, string, uint64, bool) ([]*types.KnowledgeBase, error)
}

func (s *stubIMKBVisibility) ListAccessibleKBs(ctx context.Context, userID string, tenantID uint64, isSuperAdmin bool) ([]*types.KnowledgeBase, error) {
	if s.list == nil {
		return nil, nil
	}
	return s.list(ctx, userID, tenantID, isSuperAdmin)
}

type stubIMKBShare struct {
	interfaces.KBShareService
	shared []*types.SharedKnowledgeBaseInfo
}

func (s *stubIMKBShare) ListSharedKnowledgeBases(context.Context, string, uint64) ([]*types.SharedKnowledgeBaseInfo, error) {
	return s.shared, nil
}

type stubIMSessionService struct {
	interfaces.SessionService
	lastKBIDs []string
	results   []*types.SearchResult
}

func (s *stubIMSessionService) SearchKnowledge(_ context.Context, knowledgeBaseIDs []string, _ []string, _ string) ([]*types.SearchResult, error) {
	s.lastKBIDs = append([]string(nil), knowledgeBaseIDs...)
	if len(s.results) == 0 {
		return []*types.SearchResult{{KnowledgeTitle: "Doc A", Content: "match content"}}, nil
	}
	return s.results, nil
}

type stubIMKBService struct {
	interfaces.KnowledgeBaseService
	kbs []*types.KnowledgeBase
}

func (s *stubIMKBService) ListKnowledgeBases(context.Context) ([]*types.KnowledgeBase, error) {
	return s.kbs, nil
}

func (s *stubIMKBService) ListKnowledgeBasesByTenantID(context.Context, uint64) ([]*types.KnowledgeBase, error) {
	return s.kbs, nil
}

func testIMScopeContext() context.Context {
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "system-7")
	ctx = context.WithValue(ctx, types.TenantRoleContextKey, types.TenantRoleViewer)
	ctx = types.WithResourceAuthUserID(ctx, "user-1")
	ctx = types.WithResourceAuthUser(ctx, &types.User{ID: "user-1"})
	return ctx
}

func TestSearchCommandUsesCurrentUserKnowledgeScope(t *testing.T) {
	ctx := testIMScopeContext()
	sessionSvc := &stubIMSessionService{}
	cmd := newSearchCommand(
		sessionSvc,
		&stubIMKBService{kbs: []*types.KnowledgeBase{{ID: "kb-hidden", TenantID: 7}}},
		&stubIMKBVisibility{list: func(context.Context, string, uint64, bool) ([]*types.KnowledgeBase, error) {
			return []*types.KnowledgeBase{{ID: "kb-visible", TenantID: 7}}, nil
		}},
		&stubIMKBShare{shared: []*types.SharedKnowledgeBaseInfo{{KnowledgeBase: &types.KnowledgeBase{ID: "kb-shared", TenantID: 88}}}},
	)

	_, err := cmd.Execute(ctx, &CommandContext{
		TenantID: 7,
		CustomAgent: &types.CustomAgent{
			ID:       "agent-1",
			TenantID: 7,
			Config: types.CustomAgentConfig{
				KBSelectionMode: "all",
			},
		},
	}, []string{"退款政策"})
	require.NoError(t, err)
	require.Equal(t, []string{"kb-visible", "kb-shared"}, sessionSvc.lastKBIDs)
}

func TestInfoCommandShowsCurrentUserAccessibleKnowledgeBases(t *testing.T) {
	ctx := testIMScopeContext()
	cmd := newInfoCommand(
		&stubIMKBService{kbs: []*types.KnowledgeBase{{ID: "kb-hidden", Name: "Hidden", TenantID: 7}}},
		&stubIMKBVisibility{list: func(context.Context, string, uint64, bool) ([]*types.KnowledgeBase, error) {
			return []*types.KnowledgeBase{{ID: "kb-visible", Name: "Visible KB", TenantID: 7}}, nil
		}},
		&stubIMKBShare{shared: []*types.SharedKnowledgeBaseInfo{{KnowledgeBase: &types.KnowledgeBase{ID: "kb-shared", Name: "Shared KB", TenantID: 88}}}},
	)

	result, err := cmd.Execute(ctx, &CommandContext{
		AgentName: "Agent A",
		TenantID:  7,
		CustomAgent: &types.CustomAgent{
			ID:       "agent-1",
			TenantID: 7,
			Config: types.CustomAgentConfig{
				KBSelectionMode: "all",
			},
		},
	}, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, strings.Contains(result.Content, "Visible KB"))
	require.True(t, strings.Contains(result.Content, "Shared KB"))
	require.True(t, strings.Contains(result.Content, "全部可访问"))
	require.False(t, strings.Contains(result.Content, "Hidden"))
}