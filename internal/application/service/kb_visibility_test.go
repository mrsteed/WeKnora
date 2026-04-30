package service

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
)

type stubKBVisibilityKnowledgeRepo struct {
	interfaces.KnowledgeRepository
}

func (s *stubKBVisibilityKnowledgeRepo) CountKnowledgeByStatus(context.Context, uint64, string, []string) (int64, error) {
	return 0, nil
}

type stubKBVisibilityChunkRepo struct {
	interfaces.ChunkRepository
}

type stubKBVisibilitySchemaRepo struct {
	interfaces.DatabaseSchemaRepository
	snapshot *types.DatabaseSchemaSnapshot
	called   bool
}

func (s *stubKBVisibilitySchemaRepo) GetLatestSnapshotByKnowledgeBase(context.Context, uint64, string) (*types.DatabaseSchemaSnapshot, error) {
	s.called = true
	return s.snapshot, nil
}

func TestKBVisibilityServiceFillKnowledgeCountsDatabaseKB(t *testing.T) {
	schemaRepo := &stubKBVisibilitySchemaRepo{
		snapshot: &types.DatabaseSchemaSnapshot{
			SchemaJSON:  types.JSON(`{"tables":[{"name":"orders","type":"BASE TABLE"},{"name":"customers","type":"BASE TABLE"},{"name":"report_view","type":"VIEW"}]}`),
			RefreshedAt: time.Now(),
		},
	}
	svc := &kbVisibilityService{
		kgRepo:     &stubKBVisibilityKnowledgeRepo{},
		chunkRepo:  &stubKBVisibilityChunkRepo{},
		schemaRepo: schemaRepo,
	}
	kbs := []*types.KnowledgeBase{{ID: "kb-db", TenantID: 1, Type: types.KnowledgeBaseTypeDatabase}}

	svc.fillKnowledgeCounts(context.Background(), kbs)

	assert.Len(t, kbs, 1)
	assert.EqualValues(t, 0, kbs[0].KnowledgeCount)
	assert.EqualValues(t, 0, kbs[0].ChunkCount)
	assert.EqualValues(t, 2, kbs[0].BusinessTableCount)
	assert.True(t, schemaRepo.called)
}
