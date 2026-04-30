package interfaces

import (
	"context"
	"io"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/hibiken/asynq"
)

type LongDocumentTaskRepository interface {
	CreateTask(ctx context.Context, task *types.LongDocumentTask) error
	CreateBatches(ctx context.Context, batches []*types.LongDocumentTaskBatch) error
	CreateArtifact(ctx context.Context, artifact *types.LongDocumentArtifact) error
	GetTaskByID(ctx context.Context, tenantID uint64, taskID string) (*types.LongDocumentTask, error)
	GetTaskByIdempotencyKey(ctx context.Context, tenantID uint64, key string) (*types.LongDocumentTask, error)
	ListTasksBySession(ctx context.Context, tenantID uint64, sessionID string, page *types.Pagination) ([]*types.LongDocumentTask, int64, error)
	ListBatchesByTaskID(ctx context.Context, tenantID uint64, taskID string) ([]*types.LongDocumentTaskBatch, error)
	GetArtifactByTaskID(ctx context.Context, tenantID uint64, taskID string) (*types.LongDocumentArtifact, error)
	DeleteArtifactByTaskID(ctx context.Context, tenantID uint64, taskID string) error
	UpdateTask(ctx context.Context, task *types.LongDocumentTask) error
	UpdateBatch(ctx context.Context, batch *types.LongDocumentTaskBatch) error
	UpdateArtifact(ctx context.Context, artifact *types.LongDocumentArtifact) error
	ReplaceArtifact(ctx context.Context, task *types.LongDocumentTask, artifact *types.LongDocumentArtifact) error
	DeleteTask(ctx context.Context, tenantID uint64, taskID string) error
}

type LongDocumentTaskService interface {
	CreateTask(ctx context.Context, req *types.CreateLongDocumentTaskRequest) (*types.CreateLongDocumentTaskResponse, error)
	GetTask(ctx context.Context, taskID string) (*types.LongDocumentTask, error)
	ListTasksBySession(ctx context.Context, sessionID string, page *types.Pagination) (*types.PageResult, error)
	ListBatches(ctx context.Context, taskID string) ([]*types.LongDocumentTaskBatch, error)
	GetArtifact(ctx context.Context, taskID string) (*types.LongDocumentArtifact, error)
	DownloadArtifact(ctx context.Context, taskID string) (io.ReadCloser, string, error)
	CancelTask(ctx context.Context, taskID string) (*types.LongDocumentTask, error)
	RetryTask(ctx context.Context, taskID string) (*types.LongDocumentTask, error)
	BuildTaskEvents(ctx context.Context, taskID string) ([]types.LongDocumentTaskEvent, *types.LongDocumentTask, error)
	HandleTask(ctx context.Context, task *asynq.Task) error
	InferTaskKind(ctx context.Context, query string, knowledgeIDs []string) string
}
