package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type knowledgeListCreatorRepoStub struct {
	interfaces.KnowledgeRepository
	items  []*types.Knowledge
	total  int64
	tagMap map[string][]*types.KnowledgeTag
}

func (s *knowledgeListCreatorRepoStub) ListPagedKnowledgeByKnowledgeBaseID(
	_ context.Context,
	_ uint64,
	_ string,
	_ *types.Pagination,
	_ types.KnowledgeListFilter,
) ([]*types.Knowledge, int64, error) {
	return s.items, s.total, nil
}

func (s *knowledgeListCreatorRepoStub) GetKnowledgeTags(
	_ context.Context,
	_ []string,
) (map[string][]*types.KnowledgeTag, error) {
	return s.tagMap, nil
}

type knowledgeListCreatorUserRepoStub struct {
	interfaces.UserRepository
	users map[string]*types.User
}

func (s *knowledgeListCreatorUserRepoStub) GetUsersByIDs(_ context.Context, ids []string) (map[string]*types.User, error) {
	result := make(map[string]*types.User, len(ids))
	for _, id := range ids {
		if user, ok := s.users[id]; ok {
			result[id] = user
		}
	}
	return result, nil
}

func TestListPagedKnowledgeByKnowledgeBaseIDFillsCreatorNickname(t *testing.T) {
	repo := &knowledgeListCreatorRepoStub{
		items: []*types.Knowledge{
			{ID: "k1", CreatedBy: "user-1"},
			{ID: "k2", CreatedBy: "user-2"},
			{ID: "k3"},
		},
		total: 3,
		tagMap: map[string][]*types.KnowledgeTag{
			"k1": {&types.KnowledgeTag{ID: "tag-1", Name: "合同"}},
		},
	}
	userRepo := &knowledgeListCreatorUserRepoStub{users: map[string]*types.User{
		"user-1": {ID: "user-1", Username: "alice"},
	}}
	service := &knowledgeService{repo: repo, userRepo: userRepo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))

	result, err := service.ListPagedKnowledgeByKnowledgeBaseID(ctx, "kb-1", &types.Pagination{Page: 1, PageSize: 20}, types.KnowledgeListFilter{})
	if err != nil {
		t.Fatalf("ListPagedKnowledgeByKnowledgeBaseID() error = %v", err)
	}
	items, ok := result.Data.([]*types.Knowledge)
	if !ok {
		t.Fatalf("page result data type = %T, want []*types.Knowledge", result.Data)
	}
	if got := items[0].CreatedByNickname; got != "alice" {
		t.Fatalf("created_by_nickname for first item = %q, want alice", got)
	}
	if got := items[1].CreatedByNickname; got != "user-2" {
		t.Fatalf("created_by_nickname for second item = %q, want fallback user-2", got)
	}
	if got := items[0].Tags; len(got) != 1 || got[0].Name != "合同" {
		t.Fatalf("tags not attached correctly: %#v", got)
	}
}