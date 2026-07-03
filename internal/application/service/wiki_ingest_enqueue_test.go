package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/hibiken/asynq"
)

type wikiIngestEnqueueTaskStub struct {
	err   error
	calls int
}

func (s *wikiIngestEnqueueTaskStub) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return &asynq.TaskInfo{ID: "wiki-task", Queue: "low"}, nil
}

type wikiIngestPendingRepoStub struct {
	enqueueErr   error
	deleteErr    error
	enqueued     []*types.TaskPendingOp
	deleteCalls  []wikiPendingDeleteCall
	pendingCount int64
}

type wikiPendingDeleteCall struct {
	taskType string
	scope    string
	scopeID  string
	dedupKey string
	op       string
}

func (s *wikiIngestPendingRepoStub) Enqueue(ctx context.Context, op *types.TaskPendingOp) error {
	if s.enqueueErr != nil {
		return s.enqueueErr
	}
	clone := *op
	clone.Payload = append([]byte(nil), op.Payload...)
	s.enqueued = append(s.enqueued, &clone)
	return nil
}

func (s *wikiIngestPendingRepoStub) PeekBatch(ctx context.Context, taskType, scope, scopeID string, limit int) ([]*types.TaskPendingOp, error) {
	return nil, nil
}

func (s *wikiIngestPendingRepoStub) DeleteByIDs(ctx context.Context, ids []int64) error {
	return nil
}

func (s *wikiIngestPendingRepoStub) IncrFailCount(ctx context.Context, id int64) (int, error) {
	return 0, nil
}

func (s *wikiIngestPendingRepoStub) PendingCount(ctx context.Context, taskType, scope, scopeID string) (int64, error) {
	return s.pendingCount, nil
}

func (s *wikiIngestPendingRepoStub) DeleteByDedupKey(ctx context.Context, taskType, scope, scopeID, dedupKey, op string) error {
	s.deleteCalls = append(s.deleteCalls, wikiPendingDeleteCall{
		taskType: taskType,
		scope:    scope,
		scopeID:  scopeID,
		dedupKey: dedupKey,
		op:       op,
	})
	return s.deleteErr
}

func TestEnqueueWikiIngestSuccess(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.LanguageContextKey, "zh-CN")
	repo := &wikiIngestPendingRepoStub{}
	task := &wikiIngestEnqueueTaskStub{}

	ok, err := EnqueueWikiIngest(ctx, task, repo, 10000, "kb-1", "knowledge-1", 7)
	if err != nil {
		t.Fatalf("EnqueueWikiIngest returned error: %v", err)
	}
	if !ok {
		t.Fatal("EnqueueWikiIngest should report accepted=true on success")
	}
	if task.calls != 1 {
		t.Fatalf("task enqueue calls = %d, want 1", task.calls)
	}
	if len(repo.enqueued) != 1 {
		t.Fatalf("pending op count = %d, want 1", len(repo.enqueued))
	}
	if len(repo.deleteCalls) != 0 {
		t.Fatalf("unexpected cleanup calls: %d", len(repo.deleteCalls))
	}

	var op WikiPendingOp
	if err := json.Unmarshal(repo.enqueued[0].Payload, &op); err != nil {
		t.Fatalf("unmarshal pending op: %v", err)
	}
	if op.Op != WikiOpIngest {
		t.Fatalf("op type = %q, want %q", op.Op, WikiOpIngest)
	}
	if op.KnowledgeID != "knowledge-1" {
		t.Fatalf("knowledge id = %q, want knowledge-1", op.KnowledgeID)
	}
	if op.Attempt != 7 {
		t.Fatalf("attempt = %d, want 7", op.Attempt)
	}
	if op.Language != "zh-CN" {
		t.Fatalf("language = %q, want zh-CN", op.Language)
	}
}

func TestEnqueueWikiIngestStopsWhenPendingOpPersistFails(t *testing.T) {
	repo := &wikiIngestPendingRepoStub{enqueueErr: errors.New("pg down")}
	task := &wikiIngestEnqueueTaskStub{}

	ok, err := EnqueueWikiIngest(context.Background(), task, repo, 10000, "kb-1", "knowledge-1", 1)
	if err == nil {
		t.Fatal("EnqueueWikiIngest should surface pending-op insert failure")
	}
	if ok {
		t.Fatal("EnqueueWikiIngest should report accepted=false when pending-op insert fails")
	}
	if task.calls != 0 {
		t.Fatalf("task enqueue calls = %d, want 0", task.calls)
	}
	if len(repo.deleteCalls) != 0 {
		t.Fatalf("cleanup should not run when pending op was never inserted, got %d call(s)", len(repo.deleteCalls))
	}
}

func TestEnqueueWikiIngestScrubsPendingOpWhenTriggerEnqueueFails(t *testing.T) {
	repo := &wikiIngestPendingRepoStub{}
	task := &wikiIngestEnqueueTaskStub{err: errors.New("redis down")}

	ok, err := EnqueueWikiIngest(context.Background(), task, repo, 10000, "kb-1", "knowledge-1", 3)
	if err == nil {
		t.Fatal("EnqueueWikiIngest should surface trigger enqueue failure")
	}
	if ok {
		t.Fatal("EnqueueWikiIngest should report accepted=false when trigger enqueue fails")
	}
	if task.calls != 1 {
		t.Fatalf("task enqueue calls = %d, want 1", task.calls)
	}
	if len(repo.enqueued) != 1 {
		t.Fatalf("pending op count = %d, want 1", len(repo.enqueued))
	}
	if len(repo.deleteCalls) != 1 {
		t.Fatalf("cleanup calls = %d, want 1", len(repo.deleteCalls))
	}
	call := repo.deleteCalls[0]
	if call.taskType != wikiTaskType || call.scope != wikiTaskScope || call.scopeID != "kb-1" || call.dedupKey != "knowledge-1" || call.op != WikiOpIngest {
		t.Fatalf("unexpected cleanup call: %+v", call)
	}
}
