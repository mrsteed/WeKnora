package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/tracing/langfuse"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/hibiken/asynq"
)

const (
	defaultKnowledgeFileDispatchInterval = 3 * time.Second
	knowledgeFileDispatchLockTTL         = 15 * time.Second
	knowledgeFileDispatchLockPrefix      = "knowledge:file_dispatch:"
)

// EnqueueKnowledgeFileDispatchTask schedules the lightweight dispatcher that
// admits pending file uploads into the normal document:process queue.
func EnqueueKnowledgeFileDispatchTask(
	ctx context.Context,
	task interfaces.TaskEnqueuer,
	tenantID uint64,
	kbID string,
	delay time.Duration,
) error {
	if task == nil || tenantID == 0 || kbID == "" {
		return nil
	}
	payload := types.KnowledgeFileDispatchPayload{
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
	}
	langfuse.InjectTracing(ctx, &payload)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal knowledge file dispatch payload: %w", err)
	}
	opts := []asynq.Option{asynq.Queue(types.QueueDefault), asynq.MaxRetry(3)}
	if delay > 0 {
		opts = append(opts, asynq.ProcessIn(delay))
	}
	_, err = task.Enqueue(asynq.NewTask(types.TypeKnowledgeFileDispatch, payloadBytes, opts...))
	if err != nil {
		return fmt.Errorf("enqueue knowledge file dispatch task: %w", err)
	}
	return nil
}

func (s *knowledgeService) ProcessKnowledgeFileDispatch(ctx context.Context, t *asynq.Task) error {
	var payload types.KnowledgeFileDispatchPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		logger.Warnf(ctx, "knowledge file dispatch: invalid payload: %v", err)
		return nil
	}
	if payload.TenantID == 0 || payload.KnowledgeBaseID == "" {
		return nil
	}
	ctx = context.WithValue(ctx, types.TenantIDContextKey, payload.TenantID)

	release, ok := s.acquireKnowledgeFileDispatchLock(ctx, payload.KnowledgeBaseID)
	if !ok {
		return nil
	}
	defer release()

	kb, err := s.kbService.GetKnowledgeBaseByID(ctx, payload.KnowledgeBaseID)
	if err != nil || kb == nil {
		logger.Warnf(ctx, "knowledge file dispatch: kb %s not found: %v", payload.KnowledgeBaseID, err)
		return nil
	}
	kb.EnsureDefaults()

	pendingCount, err := s.repo.CountPendingFileKnowledgeForDispatch(ctx, payload.TenantID, payload.KnowledgeBaseID)
	if err != nil {
		return err
	}
	if pendingCount == 0 {
		return nil
	}

	inFlight, err := s.repo.CountKnowledgeByStatus(ctx, payload.TenantID, payload.KnowledgeBaseID,
		[]string{types.ParseStatusProcessing})
	if err != nil {
		return err
	}
	// Parse admission is keyed to the primary parse stage only. Once a document
	// moves into finalizing, its parse slot is considered released so wiki /
	// summary / graph work can overlap with the next parsing document.
	limit := kb.MaxConcurrentParseTasks
	if limit <= 0 {
		limit = 5
	}
	available := limit - int(inFlight)
	if available <= 0 {
		return EnqueueKnowledgeFileDispatchTask(ctx, s.task, payload.TenantID, payload.KnowledgeBaseID, defaultKnowledgeFileDispatchInterval)
	}

	candidates, err := s.repo.ListPendingFileKnowledgeForDispatch(ctx, payload.TenantID, payload.KnowledgeBaseID, available)
	if err != nil {
		return err
	}
	for _, knowledge := range candidates {
		claimed, err := s.repo.PromoteKnowledgePendingForDispatch(ctx, knowledge.ID)
		if err != nil {
			return err
		}
		if !claimed {
			continue
		}
		if err := s.enqueueFileKnowledgeProcessTask(ctx, kb, knowledge); err != nil {
			logger.Warnf(ctx, "knowledge file dispatch: enqueue process task failed for %s: %v", knowledge.ID, err)
			_ = s.repo.UpdateKnowledgeColumns(ctx, knowledge.ID, map[string]interface{}{
				"parse_status":  types.ParseStatusPending,
				"error_message": "",
				"updated_at":    time.Now(),
			})
		}
	}

	remaining, err := s.repo.CountPendingFileKnowledgeForDispatch(ctx, payload.TenantID, payload.KnowledgeBaseID)
	if err != nil {
		return err
	}
	if remaining > 0 {
		return EnqueueKnowledgeFileDispatchTask(ctx, s.task, payload.TenantID, payload.KnowledgeBaseID, defaultKnowledgeFileDispatchInterval)
	}
	return nil
}

func (s *knowledgeService) enqueueFileKnowledgeProcessTask(
	ctx context.Context,
	kb *types.KnowledgeBase,
	knowledge *types.Knowledge,
) error {
	if kb == nil || knowledge == nil {
		return nil
	}
	processOverrides, _ := knowledge.ProcessOverrides()
	eff := ResolveProcessConfig(kb, processOverrides)
	questionCount := eff.QuestionGenerationConfig.QuestionCount
	if questionCount <= 0 {
		questionCount = 3
	}
	lang, _ := types.LanguageFromContext(ctx)
	taskPayload := types.DocumentProcessPayload{
		TenantID:                 knowledge.TenantID,
		KnowledgeID:              knowledge.ID,
		KnowledgeBaseID:          knowledge.KnowledgeBaseID,
		FilePath:                 knowledge.FilePath,
		FileName:                 knowledge.FileName,
		FileType:                 getFileType(knowledge.FileName),
		EnableMultimodel:         eff.EnableMultimodel,
		EnableQuestionGeneration: eff.QuestionGenerationConfig.Enabled,
		QuestionCount:            questionCount,
		Language:                 lang,
	}
	langfuse.InjectTracing(ctx, &taskPayload)
	taskPayload.Attempt = s.reserveInitialParseAttempt(ctx, knowledge.ID, taskPayload.LangfuseTraceID)
	payloadBytes, err := json.Marshal(taskPayload)
	if err != nil {
		s.failReservedParseAttempt(ctx, knowledge.ID, taskPayload.Attempt,
			fmt.Sprintf("failed to marshal document process task payload: %v", err))
		return err
	}
	task := asynq.NewTask(
		types.TypeDocumentProcess,
		payloadBytes,
		documentProcessTaskOptions(s.config, asynq.MaxRetry(3))...,
	)
	info, err := s.task.Enqueue(task)
	if err != nil {
		s.failReservedParseAttempt(ctx, knowledge.ID, taskPayload.Attempt,
			fmt.Sprintf("failed to enqueue document process task: %v", err))
		return err
	}
	logger.Infof(ctx, "knowledge file dispatch: enqueued document process task id=%s queue=%s knowledge=%s", info.ID, info.Queue, knowledge.ID)
	if slices.Contains([]string{"csv", "xlsx", "xls"}, getFileType(knowledge.FileName)) {
		NewDataTableSummaryTask(ctx, s.task, knowledge.TenantID, knowledge.ID, kb.SummaryModelID, kb.EmbeddingModelID)
	}
	return nil
}

func (s *knowledgeService) acquireKnowledgeFileDispatchLock(
	ctx context.Context,
	kbID string,
) (func(), bool) {
	if kbID == "" {
		return func() {}, false
	}
	if s.redisClient != nil {
		key := knowledgeFileDispatchLockPrefix + kbID
		acquired, err := s.redisClient.SetNX(ctx, key, "1", knowledgeFileDispatchLockTTL).Result()
		if err != nil || !acquired {
			return func() {}, false
		}
		return func() {
			s.redisClient.Del(context.Background(), key)
		}, true
	}
	if _, loaded := s.fileDispatchLocks.LoadOrStore(kbID, struct{}{}); loaded {
		return func() {}, false
	}
	return func() { s.fileDispatchLocks.Delete(kbID) }, true
}