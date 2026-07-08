package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

type knowledgeFileDispatchRepoStub struct {
	interfaces.KnowledgeRepository
	pending []*types.Knowledge
}

func (r *knowledgeFileDispatchRepoStub) CountPendingFileKnowledgeForDispatch(
	ctx context.Context, tenantID uint64, kbID string,
) (int64, error) {
	var count int64
	for _, knowledge := range r.pending {
		if knowledge.TenantID == tenantID && knowledge.KnowledgeBaseID == kbID && knowledge.ParseStatus == types.ParseStatusPending {
			count++
		}
	}
	return count, nil
}

func (r *knowledgeFileDispatchRepoStub) CountKnowledgeByStatus(
	ctx context.Context, tenantID uint64, kbID string, parseStatuses []string,
) (int64, error) {
	allowed := make(map[string]struct{}, len(parseStatuses))
	for _, status := range parseStatuses {
		allowed[status] = struct{}{}
	}
	var count int64
	for _, knowledge := range r.pending {
		if knowledge.TenantID != tenantID || knowledge.KnowledgeBaseID != kbID {
			continue
		}
		if _, ok := allowed[knowledge.ParseStatus]; ok {
			count++
		}
	}
	return count, nil
}

func (r *knowledgeFileDispatchRepoStub) ListPendingFileKnowledgeForDispatch(
	ctx context.Context, tenantID uint64, kbID string, limit int,
) ([]*types.Knowledge, error) {
	rows := make([]*types.Knowledge, 0, limit)
	for _, knowledge := range r.pending {
		if knowledge.TenantID == tenantID && knowledge.KnowledgeBaseID == kbID && knowledge.ParseStatus == types.ParseStatusPending {
			rows = append(rows, knowledge)
			if len(rows) == limit {
				break
			}
		}
	}
	return rows, nil
}

func (r *knowledgeFileDispatchRepoStub) PromoteKnowledgePendingForDispatch(
	ctx context.Context, id string,
) (bool, error) {
	for _, knowledge := range r.pending {
		if knowledge.ID == id && knowledge.ParseStatus == types.ParseStatusPending {
			knowledge.ParseStatus = types.ParseStatusProcessing
			return true, nil
		}
	}
	return false, nil
}

func (r *knowledgeFileDispatchRepoStub) UpdateKnowledgeColumns(
	ctx context.Context, id string, values map[string]interface{},
) error {
	for _, knowledge := range r.pending {
		if knowledge.ID != id {
			continue
		}
		if status, ok := values["parse_status"].(string); ok {
			knowledge.ParseStatus = status
		}
		return nil
	}
	return nil
}

type knowledgeFileDispatchKBServiceStub struct {
	interfaces.KnowledgeBaseService
	kb *types.KnowledgeBase
}

func (s *knowledgeFileDispatchKBServiceStub) GetKnowledgeBaseByID(
	ctx context.Context, id string,
) (*types.KnowledgeBase, error) {
	return s.kb, nil
}

type knowledgeFileDispatchTaskStub struct {
	tasks []*asynq.Task
}

func (s *knowledgeFileDispatchTaskStub) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	s.tasks = append(s.tasks, task)
	return &asynq.TaskInfo{ID: "task-1", Queue: "default"}, nil
}

func TestProcessKnowledgeFileDispatchEnqueuesUpToKnowledgeBaseLimit(t *testing.T) {
	t.Parallel()

	repo := &knowledgeFileDispatchRepoStub{pending: []*types.Knowledge{
		{ID: "k1", TenantID: 1, KnowledgeBaseID: "kb-1", Type: "file", FileName: "a.txt", FilePath: "p1", ParseStatus: types.ParseStatusPending},
		{ID: "k2", TenantID: 1, KnowledgeBaseID: "kb-1", Type: "file", FileName: "b.txt", FilePath: "p2", ParseStatus: types.ParseStatusPending},
		{ID: "k3", TenantID: 1, KnowledgeBaseID: "kb-1", Type: "file", FileName: "c.txt", FilePath: "p3", ParseStatus: types.ParseStatusPending},
	}}
	task := &knowledgeFileDispatchTaskStub{}
	svc := &knowledgeService{
		repo:      repo,
		kbService: &knowledgeFileDispatchKBServiceStub{kb: &types.KnowledgeBase{ID: "kb-1", MaxConcurrentParseTasks: 2, SummaryModelID: "sum", EmbeddingModelID: "emb"}},
		task:      task,
	}

	payloadBytes, err := json.Marshal(types.KnowledgeFileDispatchPayload{TenantID: 1, KnowledgeBaseID: "kb-1"})
	require.NoError(t, err)
	require.NoError(t, svc.ProcessKnowledgeFileDispatch(newCreateKnowledgeFileContext(), asynq.NewTask(types.TypeKnowledgeFileDispatch, payloadBytes)))

	require.Len(t, task.tasks, 3)
	require.Equal(t, types.TypeDocumentProcess, task.tasks[0].Type())
	require.Equal(t, types.TypeDocumentProcess, task.tasks[1].Type())
	require.Equal(t, types.TypeKnowledgeFileDispatch, task.tasks[2].Type())
	require.Equal(t, types.ParseStatusProcessing, repo.pending[0].ParseStatus)
	require.Equal(t, types.ParseStatusProcessing, repo.pending[1].ParseStatus)
	require.Equal(t, types.ParseStatusPending, repo.pending[2].ParseStatus)
}

func TestProcessKnowledgeFileDispatchFinalizingDoesNotConsumeParseSlot(t *testing.T) {
	t.Parallel()

	repo := &knowledgeFileDispatchRepoStub{pending: []*types.Knowledge{
		{ID: "k-final", TenantID: 1, KnowledgeBaseID: "kb-1", Type: "file", FileName: "done.txt", FilePath: "pf", ParseStatus: types.ParseStatusFinalizing},
		{ID: "k1", TenantID: 1, KnowledgeBaseID: "kb-1", Type: "file", FileName: "a.txt", FilePath: "p1", ParseStatus: types.ParseStatusPending},
		{ID: "k2", TenantID: 1, KnowledgeBaseID: "kb-1", Type: "file", FileName: "b.txt", FilePath: "p2", ParseStatus: types.ParseStatusPending},
	}}
	task := &knowledgeFileDispatchTaskStub{}
	svc := &knowledgeService{
		repo:      repo,
		kbService: &knowledgeFileDispatchKBServiceStub{kb: &types.KnowledgeBase{ID: "kb-1", MaxConcurrentParseTasks: 2, SummaryModelID: "sum", EmbeddingModelID: "emb"}},
		task:      task,
	}

	payloadBytes, err := json.Marshal(types.KnowledgeFileDispatchPayload{TenantID: 1, KnowledgeBaseID: "kb-1"})
	require.NoError(t, err)
	require.NoError(t, svc.ProcessKnowledgeFileDispatch(newCreateKnowledgeFileContext(), asynq.NewTask(types.TypeKnowledgeFileDispatch, payloadBytes)))

	require.Len(t, task.tasks, 2)
	require.Equal(t, types.TypeDocumentProcess, task.tasks[0].Type())
	require.Equal(t, types.TypeDocumentProcess, task.tasks[1].Type())
	require.Equal(t, types.ParseStatusFinalizing, repo.pending[0].ParseStatus)
	require.Equal(t, types.ParseStatusProcessing, repo.pending[1].ParseStatus)
	require.Equal(t, types.ParseStatusProcessing, repo.pending[2].ParseStatus)
}