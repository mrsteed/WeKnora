package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubSuggestedQuestionsAgentRepo struct {
	interfaces.CustomAgentRepository
	agent *types.CustomAgent
}

func (s *stubSuggestedQuestionsAgentRepo) GetAgentByID(context.Context, string, uint64) (*types.CustomAgent, error) {
	return s.agent, nil
}

type stubSuggestedQuestionsKBService struct {
	interfaces.KnowledgeBaseService
	kbs []*types.KnowledgeBase
}

func (s *stubSuggestedQuestionsKBService) ListKnowledgeBases(context.Context) ([]*types.KnowledgeBase, error) {
	return s.kbs, nil
}

type stubSuggestedQuestionsChunkRepo struct {
	interfaces.ChunkRepository
	faqKBIDs []string
	docKBIDs []string
}

func (s *stubSuggestedQuestionsChunkRepo) ListRecommendedFAQChunks(_ context.Context, _ uint64, kbIDs []string, _ []string, _ int) ([]*types.Chunk, error) {
	s.faqKBIDs = append([]string(nil), kbIDs...)
	return nil, nil
}

func (s *stubSuggestedQuestionsChunkRepo) ListRecentDocumentChunksWithQuestions(_ context.Context, _ uint64, kbIDs []string, _ []string, _ int) ([]*types.Chunk, error) {
	s.docKBIDs = append([]string(nil), kbIDs...)
	return nil, nil
}

type stubSuggestedQuestionsWikiRepo struct {
	interfaces.WikiPageRepository
}

type stubSuggestedQuestionsKBShareService struct {
	interfaces.KBShareService
	shared []*types.SharedKnowledgeBaseInfo
}

func (s *stubSuggestedQuestionsKBShareService) ListSharedKnowledgeBases(context.Context, string, uint64) ([]*types.SharedKnowledgeBaseInfo, error) {
	return s.shared, nil
}

func (s *stubSuggestedQuestionsWikiRepo) ListRecentForSuggestions(context.Context, uint64, []string, int) ([]*types.WikiPage, error) {
	return nil, nil
}

func TestGetSuggestedQuestionsFiltersKnowledgeBasesByAllowedToolsInAllMode(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(9))

	repo := &stubSuggestedQuestionsAgentRepo{agent: &types.CustomAgent{
		ID:       "agent-db",
		TenantID: 9,
		Config: types.CustomAgentConfig{
			KBSelectionMode: "all",
			AllowedTools:    []string{"external_database_schema", "external_database_query"},
		},
	}}
	kbService := &stubSuggestedQuestionsKBService{kbs: []*types.KnowledgeBase{
		{ID: "kb-document", Type: types.KnowledgeBaseTypeDocument, IndexingStrategy: types.DefaultIndexingStrategy()},
		{ID: "kb-database", Type: types.KnowledgeBaseTypeDatabase},
	}}
	chunkRepo := &stubSuggestedQuestionsChunkRepo{}
	svc := &customAgentService{
		repo:         repo,
		chunkRepo:    chunkRepo,
		kbService:    kbService,
		wikiPageRepo: &stubSuggestedQuestionsWikiRepo{},
	}

	questions, err := svc.GetSuggestedQuestions(ctx, "agent-db", nil, nil, 6)
	require.NoError(t, err)
	assert.Empty(t, questions)
	assert.Equal(t, []string{"kb-database"}, chunkRepo.faqKBIDs)
	assert.Equal(t, []string{"kb-database"}, chunkRepo.docKBIDs)
}

func TestGetSuggestedQuestionsIncludesCompatibleSharedKnowledgeBasesInAllMode(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(9))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-1")

	repo := &stubSuggestedQuestionsAgentRepo{agent: &types.CustomAgent{
		ID:       "agent-db",
		TenantID: 9,
		Config: types.CustomAgentConfig{
			KBSelectionMode: "all",
			AllowedTools:    []string{"external_database_schema", "external_database_query"},
		},
	}}
	kbService := &stubSuggestedQuestionsKBService{kbs: []*types.KnowledgeBase{{
		ID: "kb-own-document", Type: types.KnowledgeBaseTypeDocument, IndexingStrategy: types.DefaultIndexingStrategy(),
	}}}
	chunkRepo := &stubSuggestedQuestionsChunkRepo{}
	kbShareService := &stubSuggestedQuestionsKBShareService{shared: []*types.SharedKnowledgeBaseInfo{
		{KnowledgeBase: &types.KnowledgeBase{ID: "kb-shared-database", Type: types.KnowledgeBaseTypeDatabase}},
		{KnowledgeBase: &types.KnowledgeBase{ID: "kb-shared-document", Type: types.KnowledgeBaseTypeDocument, IndexingStrategy: types.DefaultIndexingStrategy()}},
	}}
	svc := &customAgentService{
		repo:           repo,
		chunkRepo:      chunkRepo,
		kbService:      kbService,
		kbShareService: kbShareService,
		wikiPageRepo:   &stubSuggestedQuestionsWikiRepo{},
	}

	questions, err := svc.GetSuggestedQuestions(ctx, "agent-db", nil, nil, 6)
	require.NoError(t, err)
	assert.Empty(t, questions)
	assert.Equal(t, []string{"kb-shared-database"}, chunkRepo.faqKBIDs)
	assert.Equal(t, []string{"kb-shared-database"}, chunkRepo.docKBIDs)
}
