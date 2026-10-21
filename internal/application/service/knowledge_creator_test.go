package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type creatorUserRepoStub struct {
	interfaces.UserRepository
	users map[string]*types.User
	ids   []string
	err   error
}

func (s *creatorUserRepoStub) GetUsersByIDs(_ context.Context, ids []string) (map[string]*types.User, error) {
	s.ids = append([]string(nil), ids...)
	if s.err != nil {
		return nil, s.err
	}
	return s.users, nil
}

func TestResolveKnowledgeCreatorIDFromContext_PrefersResourceAuthUserID(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.UserIDContextKey, "system-7")
	ctx = types.WithResourceAuthUserID(ctx, "owner-1")
	if got := resolveKnowledgeCreatorIDFromContext(ctx); got != "owner-1" {
		t.Fatalf("resolveKnowledgeCreatorIDFromContext() = %q, want owner-1", got)
	}
}

func TestResolveKnowledgeCreatorIDFromContext_FallsBackToRealUserAndSkipsSynthetic(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.UserIDContextKey, "user-1")
	if got := resolveKnowledgeCreatorIDFromContext(ctx); got != "user-1" {
		t.Fatalf("resolveKnowledgeCreatorIDFromContext() = %q, want user-1", got)
	}

	synthetic := context.WithValue(context.Background(), types.UserIDContextKey, "system-9")
	if got := resolveKnowledgeCreatorIDFromContext(synthetic); got != "" {
		t.Fatalf("resolveKnowledgeCreatorIDFromContext() = %q, want empty for synthetic user", got)
	}
}

func TestFillKnowledgeCreatorNicknames_ResolvesAndFallsBack(t *testing.T) {
	repo := &creatorUserRepoStub{users: map[string]*types.User{
		"user-1": {ID: "user-1", Username: "Alice"},
	}}
	knowledges := []*types.Knowledge{
		{CreatedBy: "user-1"},
		{CreatedBy: "builtin-agent-1"},
		{CreatedBy: "missing-user"},
		{CreatedBy: "system"},
		{CreatedBy: ""},
	}
	orig, existed := types.BuiltinAgentRegistry["builtin-agent-1"]
	types.BuiltinAgentRegistry["builtin-agent-1"] = func(uint64) *types.CustomAgent {
		return &types.CustomAgent{ID: "builtin-agent-1"}
	}
	t.Cleanup(func() {
		if existed {
			types.BuiltinAgentRegistry["builtin-agent-1"] = orig
			return
		}
		delete(types.BuiltinAgentRegistry, "builtin-agent-1")
	})

	fillKnowledgeCreatorNicknames(context.Background(), repo, knowledges)

	if knowledges[0].CreatedByNickname != "Alice" {
		t.Fatalf("nickname = %q, want Alice", knowledges[0].CreatedByNickname)
	}
	if knowledges[1].CreatedByNickname != "系统" {
		t.Fatalf("builtin agent nickname = %q, want 系统", knowledges[1].CreatedByNickname)
	}
	if knowledges[2].CreatedByNickname != "missing-user" {
		t.Fatalf("fallback nickname = %q, want missing-user", knowledges[2].CreatedByNickname)
	}
	if knowledges[3].CreatedByNickname != "系统" {
		t.Fatalf("system nickname = %q, want 系统", knowledges[3].CreatedByNickname)
	}
	if knowledges[4].CreatedByNickname != "" {
		t.Fatalf("empty creator should stay empty, got %q", knowledges[4].CreatedByNickname)
	}
	if len(repo.ids) != 2 {
		t.Fatalf("GetUsersByIDs ids = %v, want 2 non-system creators", repo.ids)
	}
}
