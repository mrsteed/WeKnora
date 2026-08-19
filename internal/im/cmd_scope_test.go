package im

import (
	"context"
	"testing"

	appservice "github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type commandKBService struct {
	interfaces.KnowledgeBaseService
	all      []*types.KnowledgeBase
	byTenant map[uint64][]*types.KnowledgeBase
}

func (s *commandKBService) ListKnowledgeBases(_ context.Context) ([]*types.KnowledgeBase, error) {
	return append([]*types.KnowledgeBase(nil), s.all...), nil
}

func (s *commandKBService) ListKnowledgeBasesByTenantID(_ context.Context, tenantID uint64) ([]*types.KnowledgeBase, error) {
	return append([]*types.KnowledgeBase(nil), s.byTenant[tenantID]...), nil
}

type commandKBVisibility struct {
	interfaces.KBVisibilityService
	accessible []*types.KnowledgeBase
}

func (s *commandKBVisibility) ListAccessibleKBs(_ context.Context, _ string, _ uint64, _ bool) ([]*types.KnowledgeBase, error) {
	return append([]*types.KnowledgeBase(nil), s.accessible...), nil
}

type commandKBShareService struct {
	interfaces.KBShareService
	shared []*types.SharedKnowledgeBaseInfo
}

func (s *commandKBShareService) ListSharedKnowledgeBases(_ context.Context, _ uint64, _ types.TenantRole) ([]*types.SharedKnowledgeBaseInfo, error) {
	return append([]*types.SharedKnowledgeBaseInfo(nil), s.shared...), nil
}

type commandSessionService struct {
	interfaces.SessionService
	seenKBIDs []string
}

func (s *commandSessionService) SearchKnowledge(_ context.Context, kbIDs []string, _ []string, _ []types.TagScope, _ string) ([]*types.SearchResult, error) {
	s.seenKBIDs = append([]string(nil), kbIDs...)
	return nil, nil
}

func commandScopeContext() context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(100))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-1")
	ctx = context.WithValue(ctx, types.TenantRoleContextKey, types.TenantRoleViewer)
	return ctx
}

func commandScopeAgent() *types.CustomAgent {
	return &types.CustomAgent{
		ID:       "agent-1",
		TenantID: 100,
		Name:     "Scoped Agent",
		Config: types.CustomAgentConfig{
			KBSelectionMode: "all",
			AgentMode:       types.AgentModeSmartReasoning,
			AllowedTools:    []string{"thinking"},
		},
	}
}

func TestSearchCommandUsesAgentKBScope(t *testing.T) {
	kbService := &commandKBService{all: []*types.KnowledgeBase{{ID: "local-hidden", TenantID: 100, Name: "Local Hidden"}}}
	resolver := appservice.NewAgentKBScopeResolver(
		kbService,
		&commandKBVisibility{accessible: []*types.KnowledgeBase{{ID: "local-visible", TenantID: 100, Name: "Local Visible"}}},
		&commandKBShareService{shared: []*types.SharedKnowledgeBaseInfo{{
			KnowledgeBase: &types.KnowledgeBase{ID: "shared-visible", TenantID: 200, Name: "Shared Visible"},
		}}},
		nil,
	)
	sessionService := &commandSessionService{}
	cmd := newSearchCommand(sessionService, kbService, resolver)

	result, err := cmd.Execute(commandScopeContext(), &CommandContext{TenantID: 100, CustomAgent: commandScopeAgent()}, []string{"退款政策"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{"local-visible", "shared-visible"}, sessionService.seenKBIDs)
}

func TestInfoCommandUsesAgentKBScope(t *testing.T) {
	kbService := &commandKBService{byTenant: map[uint64][]*types.KnowledgeBase{
		100: {{ID: "local-hidden", TenantID: 100, Name: "Local Hidden"}},
	}}
	resolver := appservice.NewAgentKBScopeResolver(
		kbService,
		&commandKBVisibility{accessible: []*types.KnowledgeBase{{ID: "local-visible", TenantID: 100, Name: "Local Visible"}}},
		&commandKBShareService{shared: []*types.SharedKnowledgeBaseInfo{{
			KnowledgeBase: &types.KnowledgeBase{ID: "shared-visible", TenantID: 200, Name: "Shared Visible"},
		}}},
		nil,
	)
	cmd := newInfoCommand(kbService, resolver)

	result, err := cmd.Execute(commandScopeContext(), &CommandContext{
		TenantID:    100,
		AgentName:   "Scoped Agent",
		CustomAgent: commandScopeAgent(),
	}, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Contains(t, result.Content, "Local Visible")
	assert.Contains(t, result.Content, "Shared Visible")
	assert.NotContains(t, result.Content, "Local Hidden")
}
