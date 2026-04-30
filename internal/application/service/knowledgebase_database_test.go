package service

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubKnowledgeBaseListRepo struct {
	interfaces.KnowledgeBaseRepository
	kbs []*types.KnowledgeBase
}

func (s *stubKnowledgeBaseListRepo) ListKnowledgeBasesByTenantID(context.Context, uint64) ([]*types.KnowledgeBase, error) {
	return s.kbs, nil
}

type stubKnowledgeBaseCountRepo struct {
	interfaces.KnowledgeRepository
	called bool
}

func (s *stubKnowledgeBaseCountRepo) CountKnowledgeByKnowledgeBaseID(context.Context, uint64, string) (int64, error) {
	s.called = true
	return 99, nil
}

func (s *stubKnowledgeBaseCountRepo) CountKnowledgeByStatus(context.Context, uint64, string, []string) (int64, error) {
	return 0, nil
}

type stubChunkCountRepo struct {
	interfaces.ChunkRepository
	called bool
}

func (s *stubChunkCountRepo) CountChunksByKnowledgeBaseID(context.Context, uint64, string) (int64, error) {
	s.called = true
	return 88, nil
}

type stubKnowledgeBaseSchemaRepo struct {
	interfaces.DatabaseSchemaRepository
	snapshot *types.DatabaseSchemaSnapshot
	called   bool
}

func (s *stubKnowledgeBaseSchemaRepo) GetLatestSnapshotByKnowledgeBase(context.Context, uint64, string) (*types.DatabaseSchemaSnapshot, error) {
	s.called = true
	return s.snapshot, nil
}

func TestKnowledgeBaseServiceFillKnowledgeBaseCountsDatabaseKB(t *testing.T) {
	kgRepo := &stubKnowledgeBaseCountRepo{}
	chunkRepo := &stubChunkCountRepo{}
	schemaRepo := &stubKnowledgeBaseSchemaRepo{snapshot: &types.DatabaseSchemaSnapshot{SchemaJSON: types.JSON(`{"tables":[{"name":"orders","type":"BASE TABLE"},{"name":"customers","type":"BASE TABLE"},{"name":"report_view","type":"VIEW"}]}`), RefreshedAt: time.Now()}}
	svc := &knowledgeBaseService{kgRepo: kgRepo, chunkRepo: chunkRepo, schemaRepo: schemaRepo}
	kb := &types.KnowledgeBase{ID: "kb-db", TenantID: 1, Type: types.KnowledgeBaseTypeDatabase}

	require.NoError(t, svc.FillKnowledgeBaseCounts(context.Background(), kb))
	assert.EqualValues(t, 0, kb.KnowledgeCount)
	assert.EqualValues(t, 0, kb.ChunkCount)
	assert.EqualValues(t, 2, kb.BusinessTableCount)
	assert.False(t, kgRepo.called)
	assert.False(t, chunkRepo.called)
	assert.True(t, schemaRepo.called)
}

func TestKnowledgeBaseServiceListKnowledgeBasesByTenantIDDatabaseKB(t *testing.T) {
	kgRepo := &stubKnowledgeBaseCountRepo{}
	chunkRepo := &stubChunkCountRepo{}
	schemaRepo := &stubKnowledgeBaseSchemaRepo{snapshot: &types.DatabaseSchemaSnapshot{SchemaJSON: types.JSON(`{"tables":[{"name":"orders","type":"BASE TABLE"}]}`), RefreshedAt: time.Now()}}
	svc := &knowledgeBaseService{
		repo:       &stubKnowledgeBaseListRepo{kbs: []*types.KnowledgeBase{{ID: "kb-db", TenantID: 1, Type: types.KnowledgeBaseTypeDatabase}}},
		kgRepo:     kgRepo,
		chunkRepo:  chunkRepo,
		schemaRepo: schemaRepo,
	}

	kbs, err := svc.ListKnowledgeBasesByTenantID(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, kbs, 1)
	assert.EqualValues(t, 0, kbs[0].KnowledgeCount)
	assert.EqualValues(t, 0, kbs[0].ChunkCount)
	assert.EqualValues(t, 1, kbs[0].BusinessTableCount)
	assert.False(t, kgRepo.called)
	assert.False(t, chunkRepo.called)
	assert.True(t, schemaRepo.called)
}
