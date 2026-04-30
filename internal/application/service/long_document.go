package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"time"

	filesvc "github.com/Tencent/WeKnora/internal/application/service/file"
	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/logger"
	modelchat "github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/tracing/langfuse"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/hibiken/asynq"
)

const longDocumentPromptVersion = "p2-translation-markdown-v1"

var markdownFenceRE = regexp.MustCompile("(?s)^```(?:markdown|md)?\\s*(.*?)\\s*```$")

type longDocumentBatchPlan struct {
	start int
	end   int
	input string
}

type longDocumentBatchAccumulator struct {
	parts []string
	chars int
	count int
}

type longDocumentTaskService struct {
	cfg              *config.Config
	repo             interfaces.LongDocumentTaskRepository
	sessionService   interfaces.SessionService
	tenantService    interfaces.TenantService
	knowledgeService interfaces.KnowledgeService
	kbService        interfaces.KnowledgeBaseService
	chunkService     interfaces.ChunkService
	messageService   interfaces.MessageService
	modelService     interfaces.ModelService
	fileService      interfaces.FileService
	taskEnqueuer     interfaces.TaskEnqueuer
}

func NewLongDocumentTaskService(
	cfg *config.Config,
	repo interfaces.LongDocumentTaskRepository,
	sessionService interfaces.SessionService,
	tenantService interfaces.TenantService,
	knowledgeService interfaces.KnowledgeService,
	kbService interfaces.KnowledgeBaseService,
	chunkService interfaces.ChunkService,
	messageService interfaces.MessageService,
	modelService interfaces.ModelService,
	fileService interfaces.FileService,
	taskEnqueuer interfaces.TaskEnqueuer,
) interfaces.LongDocumentTaskService {
	return &longDocumentTaskService{
		cfg:              cfg,
		repo:             repo,
		sessionService:   sessionService,
		tenantService:    tenantService,
		knowledgeService: knowledgeService,
		kbService:        kbService,
		chunkService:     chunkService,
		messageService:   messageService,
		modelService:     modelService,
		fileService:      fileService,
		taskEnqueuer:     taskEnqueuer,
	}
}

func (s *longDocumentTaskService) InferTaskKind(ctx context.Context, query string, knowledgeIDs []string) string {
	if len(knowledgeIDs) != 1 {
		return ""
	}
	normalized := strings.ToLower(strings.TrimSpace(query))
	if normalized == "" {
		return ""
	}
	keywords := []string{
		"全文翻译", "完整翻译", "整篇翻译", "翻译成markdown", "翻译成 markdown", "导出markdown", "markdown文件",
		"translate the full document", "translate full document", "translate to markdown", "export markdown",
	}
	for _, keyword := range keywords {
		if strings.Contains(normalized, strings.ToLower(keyword)) {
			return types.LongDocumentTaskKindTranslation
		}
	}
	return ""
}

func (s *longDocumentTaskService) CreateTask(ctx context.Context, req *types.CreateLongDocumentTaskRequest) (*types.CreateLongDocumentTaskResponse, error) {
	if s.cfg == nil || s.cfg.LongDocument == nil || !s.cfg.LongDocument.EnableTaskRouter {
		return nil, fmt.Errorf("long document task router is disabled")
	}
	tenantID := types.MustTenantIDFromContext(ctx)
	userID, _ := types.UserIDFromContext(ctx)
	if _, err := s.sessionService.GetSession(ctx, req.SessionID); err != nil {
		return nil, err
	}
	knowledge, err := s.knowledgeService.GetKnowledgeByID(ctx, req.KnowledgeID)
	if err != nil {
		return nil, err
	}
	if knowledge == nil {
		return nil, fmt.Errorf("knowledge not found")
	}
	if knowledge.ParseStatus != types.ParseStatusCompleted {
		return nil, fmt.Errorf("knowledge is not ready for long document tasks")
	}
	taskKind := strings.TrimSpace(req.TaskKind)
	if taskKind == "" {
		taskKind = s.InferTaskKind(ctx, req.UserQuery, []string{req.KnowledgeID})
	}
	if taskKind != types.LongDocumentTaskKindTranslation {
		return nil, fmt.Errorf("unsupported long document task kind")
	}
	outputFormat := strings.TrimSpace(req.OutputFormat)
	if outputFormat == "" {
		outputFormat = types.LongDocumentOutputFormatMarkdown
	}
	if outputFormat != types.LongDocumentOutputFormatMarkdown {
		return nil, fmt.Errorf("unsupported output format")
	}
	options := req.Options
	if options.SummaryModelID == "" {
		options.SummaryModelID = strings.TrimSpace(req.SummaryModelID)
	}

	chunks, err := s.chunkService.GetRepository().ListChunksByKnowledgeID(ctx, tenantID, req.KnowledgeID)
	if err != nil {
		return nil, err
	}
	plan := s.planBatches(chunks)
	if len(plan) == 0 {
		return nil, fmt.Errorf("knowledge has no text chunks to process")
	}

	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = types.BuildLongDocumentTaskIdempotencyKey(tenantID, req.SessionID, req.KnowledgeID, taskKind, req.UserQuery, options.SummaryModelID)
	} else {
		idempotencyKey = types.NormalizeLongDocumentTaskIdempotencyKey(idempotencyKey)
	}
	existing, err := s.repo.GetTaskByIdempotencyKey(ctx, tenantID, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return &types.CreateLongDocumentTaskResponse{TaskID: existing.ID, Status: existing.Status, Task: existing}, nil
	}

	snapshotHash := buildLongDocumentSnapshotHash(chunks)
	task := &types.LongDocumentTask{
		TenantID:           tenantID,
		SessionID:          req.SessionID,
		KnowledgeID:        req.KnowledgeID,
		TaskKind:           taskKind,
		SourceRef:          knowledge.FilePath,
		SourceSnapshotHash: snapshotHash,
		OutputFormat:       outputFormat,
		Status:             types.LongDocumentTaskStatusPending,
		TotalBatches:       len(plan),
		IdempotencyKey:     idempotencyKey,
		RetryLimit:         s.cfg.LongDocument.BatchRetryLimit,
		CreatedBy:          userID,
	}
	if err := task.SetOptions(&options); err != nil {
		return nil, err
	}
	batches := make([]*types.LongDocumentTaskBatch, 0, len(plan))
	for idx, item := range plan {
		batches = append(batches, &types.LongDocumentTaskBatch{
			TenantID:           tenantID,
			TaskID:             task.ID,
			BatchNo:            idx + 1,
			ChunkStartSeq:      item.start,
			ChunkEndSeq:        item.end,
			InputSnapshot:      item.input,
			InputTokenEstimate: estimateTokens(item.input),
			PromptVersion:      longDocumentPromptVersion,
			QualityStatus:      "pending",
		})
	}
	if err := s.repo.CreateTask(ctx, task); err != nil {
		return nil, err
	}
	for _, batch := range batches {
		batch.TaskID = task.ID
	}
	if err := s.repo.CreateBatches(ctx, batches); err != nil {
		return nil, err
	}

	if err := s.enqueueTask(ctx, task.ID, req.SessionID, req.KnowledgeID); err != nil {
		task.Status = types.LongDocumentTaskStatusFailed
		task.ErrorMessage = err.Error()
		_ = s.repo.UpdateTask(ctx, task)
		return nil, err
	}
	if s.messageService != nil {
		channel := strings.TrimSpace(req.Channel)
		if channel == "" {
			channel = "web"
		}
		_, _ = s.messageService.CreateMessage(ctx, &types.Message{
			SessionID:        req.SessionID,
			RequestID:        task.ID,
			Role:             "user",
			Content:          req.UserQuery,
			Channel:          channel,
			CompletionStatus: types.MessageCompletionStatusCompleted,
			IsCompleted:      true,
		})
	}
	task.Batches = batches
	return &types.CreateLongDocumentTaskResponse{TaskID: task.ID, Status: task.Status, Task: task}, nil
}

func (s *longDocumentTaskService) GetTask(ctx context.Context, taskID string) (*types.LongDocumentTask, error) {
	task, err := s.getAuthorizedTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	tenantID := types.MustTenantIDFromContext(ctx)
	artifact, err := s.repo.GetArtifactByTaskID(ctx, tenantID, taskID)
	if err != nil {
		return nil, err
	}
	task.Artifact = artifact
	return task, nil
}

func (s *longDocumentTaskService) ListTasksBySession(ctx context.Context, sessionID string, page *types.Pagination) (*types.PageResult, error) {
	tenantID := types.MustTenantIDFromContext(ctx)
	if page == nil {
		page = &types.Pagination{}
	}
	if _, err := s.sessionService.GetSession(ctx, sessionID); err != nil {
		return nil, err
	}
	tasks, total, err := s.repo.ListTasksBySession(ctx, tenantID, sessionID, page)
	if err != nil {
		return nil, err
	}
	for _, task := range tasks {
		artifact, artifactErr := s.repo.GetArtifactByTaskID(ctx, tenantID, task.ID)
		if artifactErr != nil {
			return nil, artifactErr
		}
		task.Artifact = artifact
	}
	return types.NewPageResult(total, page, tasks), nil
}

func (s *longDocumentTaskService) ListBatches(ctx context.Context, taskID string) ([]*types.LongDocumentTaskBatch, error) {
	if _, err := s.getAuthorizedTask(ctx, taskID); err != nil {
		return nil, err
	}
	tenantID := types.MustTenantIDFromContext(ctx)
	return s.repo.ListBatchesByTaskID(ctx, tenantID, taskID)
}

func (s *longDocumentTaskService) GetArtifact(ctx context.Context, taskID string) (*types.LongDocumentArtifact, error) {
	if _, err := s.getAuthorizedTask(ctx, taskID); err != nil {
		return nil, err
	}
	tenantID := types.MustTenantIDFromContext(ctx)
	return s.repo.GetArtifactByTaskID(ctx, tenantID, taskID)
}

func (s *longDocumentTaskService) DownloadArtifact(ctx context.Context, taskID string) (io.ReadCloser, string, error) {
	if s.cfg == nil || s.cfg.LongDocument == nil || !s.cfg.LongDocument.EnableArtifactDownload {
		return nil, "", fmt.Errorf("artifact download is disabled")
	}
	artifact, err := s.GetArtifact(ctx, taskID)
	if err != nil {
		return nil, "", err
	}
	if artifact == nil || artifact.FilePath == "" {
		return nil, "", fmt.Errorf("artifact not found")
	}
	provider := resolveLongDocumentArtifactProvider(nil, nil, artifact.FilePath)
	fileSvc := s.resolveFileServiceForProvider(ctx, provider)
	reader, err := fileSvc.GetFile(ctx, artifact.FilePath)
	if err != nil {
		return nil, "", err
	}
	return reader, artifact.FileName, nil
}

func (s *longDocumentTaskService) CancelTask(ctx context.Context, taskID string) (*types.LongDocumentTask, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.IsTerminal() {
		return task, nil
	}
	now := time.Now()
	task.Status = types.LongDocumentTaskStatusCancelled
	task.CancelledAt = &now
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *longDocumentTaskService) RetryTask(ctx context.Context, taskID string) (*types.LongDocumentTask, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status == types.LongDocumentTaskStatusPending || task.Status == types.LongDocumentTaskStatusRunning || task.Status == types.LongDocumentTaskStatusAssembling {
		return nil, fmt.Errorf("task is already active")
	}
	tenantID := types.MustTenantIDFromContext(ctx)
	batches, err := s.repo.ListBatchesByTaskID(ctx, tenantID, taskID)
	if err != nil {
		return nil, err
	}
	if !prepareTaskForRetry(task, batches) {
		return nil, fmt.Errorf("task has no retryable batches")
	}
	for _, batch := range batches {
		if batch == nil || batch.Status == types.LongDocumentBatchStatusCompleted {
			continue
		}
		if err := s.repo.UpdateBatch(ctx, batch); err != nil {
			return nil, err
		}
	}
	if task.ArtifactID != "" || task.ArtifactPath != "" {
		if err := s.repo.DeleteArtifactByTaskID(ctx, tenantID, taskID); err != nil {
			return nil, err
		}
	}
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	if err := s.enqueueTask(ctx, task.ID, task.SessionID, task.KnowledgeID); err != nil {
		task.Status = types.LongDocumentTaskStatusFailed
		task.ErrorMessage = err.Error()
		_ = s.repo.UpdateTask(ctx, task)
		return nil, err
	}
	artifact, artifactErr := s.repo.GetArtifactByTaskID(ctx, tenantID, taskID)
	if artifactErr != nil {
		return nil, artifactErr
	}
	task.Artifact = artifact
	return task, nil
}

func (s *longDocumentTaskService) BuildTaskEvents(ctx context.Context, taskID string) ([]types.LongDocumentTaskEvent, *types.LongDocumentTask, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, nil, err
	}
	tenantID := types.MustTenantIDFromContext(ctx)
	batches, err := s.repo.ListBatchesByTaskID(ctx, tenantID, taskID)
	if err != nil {
		return nil, nil, err
	}
	events := buildLongDocumentTaskEvents(task, batches, task.Artifact)
	return events, task, nil
}

func (s *longDocumentTaskService) enqueueTask(ctx context.Context, taskID, sessionID, knowledgeID string) error {
	tenantID := types.MustTenantIDFromContext(ctx)
	payload := types.LongDocumentTaskPayload{TaskID: taskID, TenantID: tenantID, SessionID: sessionID, KnowledgeID: knowledgeID}
	langfuse.InjectTracing(ctx, &payload)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	job := asynq.NewTask(types.TypeLongDocumentTask, payloadBytes, asynq.Queue("default"), asynq.MaxRetry(0))
	_, err = s.taskEnqueuer.Enqueue(job)
	return err
}

func (s *longDocumentTaskService) HandleTask(ctx context.Context, taskJob *asynq.Task) error {
	if s.cfg == nil || s.cfg.LongDocument == nil || !s.cfg.LongDocument.EnableTaskWorker {
		return fmt.Errorf("long document task worker is disabled")
	}
	var payload types.LongDocumentTaskPayload
	if err := json.Unmarshal(taskJob.Payload(), &payload); err != nil {
		return err
	}
	ctx = context.WithValue(ctx, types.TenantIDContextKey, payload.TenantID)
	ctx = context.WithValue(ctx, types.SessionTenantIDContextKey, payload.TenantID)
	task, err := s.repo.GetTaskByID(ctx, payload.TenantID, payload.TaskID)
	if err != nil {
		return err
	}
	if task == nil || task.IsTerminal() || task.Status == types.LongDocumentTaskStatusCancelled {
		return nil
	}
	batches, err := s.repo.ListBatchesByTaskID(ctx, payload.TenantID, payload.TaskID)
	if err != nil {
		return err
	}
	if len(batches) == 0 {
		return fmt.Errorf("task has no batches")
	}
	knowledge, err := s.knowledgeService.GetKnowledgeByID(ctx, payload.KnowledgeID)
	if err != nil {
		return err
	}
	kb, err := s.kbService.GetKnowledgeBaseByID(ctx, knowledge.KnowledgeBaseID)
	if err != nil {
		return err
	}
	options, err := task.Options()
	if err != nil {
		return err
	}
	if strings.TrimSpace(kb.SummaryModelID) == "" && strings.TrimSpace(options.SummaryModelID) == "" {
		return fmt.Errorf("knowledge base summary model is not configured")
	}
	chatModel, err := s.resolveTaskChatModel(ctx, kb, options)
	if err != nil {
		return err
	}
	task.Status = types.LongDocumentTaskStatusRunning
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return err
	}

	completed := 0
	failed := 0
	for _, batch := range batches {
		freshTask, taskErr := s.repo.GetTaskByID(ctx, payload.TenantID, payload.TaskID)
		if taskErr != nil {
			return taskErr
		}
		if freshTask == nil || freshTask.Status == types.LongDocumentTaskStatusCancelled {
			logger.Infof(ctx, "[LongDocument] task cancelled before batch execution task_id=%s batch_no=%d", payload.TaskID, batch.BatchNo)
			return nil
		}
		if batch.Status == types.LongDocumentBatchStatusCompleted {
			completed++
			continue
		}
		if err := s.executeBatch(ctx, freshTask, batch, chatModel, options); err != nil {
			failed++
			logger.Warnf(ctx, "[LongDocument] batch failed task_id=%s batch_no=%d err=%v", payload.TaskID, batch.BatchNo, err)
			continue
		}
		completed++
		freshTask.CompletedBatches = completed
		freshTask.FailedBatches = failed
		_ = s.repo.UpdateTask(ctx, freshTask)
	}

	task, err = s.repo.GetTaskByID(ctx, payload.TenantID, payload.TaskID)
	if err != nil {
		return err
	}
	batches, err = s.repo.ListBatchesByTaskID(ctx, payload.TenantID, payload.TaskID)
	if err != nil {
		return err
	}
	task.Status = types.LongDocumentTaskStatusAssembling
	task.CompletedBatches = completed
	task.FailedBatches = failed
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return err
	}
	artifact, assembleErr := s.assembleArtifact(ctx, task, knowledge, kb, batches)
	if assembleErr != nil {
		task.Status = types.LongDocumentTaskStatusFailed
		task.ErrorMessage = assembleErr.Error()
		now := time.Now()
		task.CompletedAt = &now
		_ = s.repo.UpdateTask(ctx, task)
		return assembleErr
	}
	task.Artifact = artifact
	task.ArtifactID = artifact.ID
	task.ArtifactPath = artifact.FilePath
	now := time.Now()
	task.CompletedAt = &now
	if failed > 0 {
		task.Status = types.LongDocumentTaskStatusPartial
		task.QualityStatus = "partial"
	} else {
		task.Status = types.LongDocumentTaskStatusCompleted
		task.QualityStatus = "passed"
	}
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return err
	}
	logger.Infof(ctx, "[LongDocument] task completed task_id=%s status=%s completed_batches=%d failed_batches=%d", task.ID, task.Status, completed, failed)
	return nil
}

func (s *longDocumentTaskService) getAuthorizedTask(ctx context.Context, taskID string) (*types.LongDocumentTask, error) {
	tenantID := types.MustTenantIDFromContext(ctx)
	task, err := s.repo.GetTaskByID(ctx, tenantID, taskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("task not found")
	}
	if _, err := s.sessionService.GetSession(ctx, task.SessionID); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *longDocumentTaskService) executeBatch(
	ctx context.Context,
	task *types.LongDocumentTask,
	batch *types.LongDocumentTaskBatch,
	chatModel modelchat.Chat,
	options *types.LongDocumentTaskOptions,
) error {
	var lastErr error
	for attempt := 1; attempt <= max(1, task.RetryLimit); attempt++ {
		now := time.Now()
		batch.Status = types.LongDocumentBatchStatusRunning
		batch.StartedAt = &now
		batch.RetryCount = attempt - 1
		batch.ErrorMessage = ""
		if err := s.repo.UpdateBatch(ctx, batch); err != nil {
			return err
		}

		prompt, err := s.buildBatchPrompt(ctx, task, batch, options)
		if err != nil {
			return err
		}
		thinking := false
		resp, err := chatModel.Chat(ctx, []modelchat.Message{{Role: "user", Content: prompt}}, &modelchat.ChatOptions{
			Temperature: 0.2,
			Thinking:    &thinking,
		})
		if err == nil && resp != nil && strings.TrimSpace(resp.Content) != "" {
			completedAt := time.Now()
			batch.Status = types.LongDocumentBatchStatusCompleted
			batch.OutputPayload = sanitizeGeneratedMarkdown(resp.Content)
			batch.ErrorMessage = ""
			batch.OutputTokenEstimate = resp.Usage.CompletionTokens
			batch.ModelName = chatModel.GetModelName()
			batch.QualityStatus = "passed"
			batch.CompletedAt = &completedAt
			return s.repo.UpdateBatch(ctx, batch)
		}
		if err == nil {
			err = fmt.Errorf("empty response from chat model")
		}
		lastErr = err
		batch.ErrorMessage = err.Error()
		batch.ModelName = chatModel.GetModelName()
		if attempt < max(1, task.RetryLimit) {
			batch.Status = types.LongDocumentBatchStatusRetrying
		} else {
			batch.Status = types.LongDocumentBatchStatusFailed
			batch.QualityStatus = "failed"
		}
		if updateErr := s.repo.UpdateBatch(ctx, batch); updateErr != nil {
			return updateErr
		}
	}
	return lastErr
}

func (s *longDocumentTaskService) assembleArtifact(
	ctx context.Context,
	task *types.LongDocumentTask,
	knowledge *types.Knowledge,
	kb *types.KnowledgeBase,
	batches []*types.LongDocumentTaskBatch,
) (*types.LongDocumentArtifact, error) {
	if len(batches) == 0 {
		return nil, fmt.Errorf("no batch output to assemble")
	}
	var content strings.Builder
	content.WriteString("# ")
	content.WriteString(strings.TrimSpace(firstNonEmpty(knowledge.Title, knowledge.FileName, "翻译结果")))
	content.WriteString("\n\n")
	for _, batch := range batches {
		switch batch.Status {
		case types.LongDocumentBatchStatusCompleted:
			content.WriteString(strings.TrimSpace(batch.OutputPayload))
			content.WriteString("\n\n")
		case types.LongDocumentBatchStatusFailed:
			content.WriteString(fmt.Sprintf("> 批次 %d 翻译失败，原始分段范围 %d-%d，失败原因：%s\n\n", batch.BatchNo, batch.ChunkStartSeq, batch.ChunkEndSeq, firstNonEmpty(batch.ErrorMessage, "unknown error")))
		}
	}
	assembled := strings.TrimSpace(content.String())
	if assembled == "" {
		return nil, fmt.Errorf("assembled artifact is empty")
	}
	baseName := sanitizeArtifactBaseName(firstNonEmpty(knowledge.Title, knowledge.FileName, knowledge.ID))
	fileName := fmt.Sprintf("%s-translated.md", baseName)
	provider := resolveLongDocumentArtifactProvider(knowledge, kb, "")
	fileSvc := s.resolveFileServiceForProvider(ctx, provider)
	filePath, err := fileSvc.SaveBytes(ctx, []byte(assembled), task.TenantID, fileName, false)
	if err != nil {
		return nil, err
	}
	checksum := sha256.Sum256([]byte(assembled))
	artifact := &types.LongDocumentArtifact{
		TenantID:       task.TenantID,
		TaskID:         task.ID,
		FileName:       fileName,
		FilePath:       filePath,
		FileType:       "text/markdown",
		FileSize:       int64(len([]byte(assembled))),
		Checksum:       hex.EncodeToString(checksum[:]),
		StorageBackend: storageBackendFromPath(filePath),
		Status:         types.LongDocumentArtifactStatusAvailable,
	}
	if err := s.repo.ReplaceArtifact(ctx, task, artifact); err != nil {
		return nil, err
	}
	return artifact, nil
}

func (s *longDocumentTaskService) buildBatchPrompt(ctx context.Context, task *types.LongDocumentTask, batch *types.LongDocumentTaskBatch, options *types.LongDocumentTaskOptions) (string, error) {
	tpl := `你是一个专业文档翻译助手。请将给定文档片段翻译为 {{.TargetLanguage}}，并严格输出 Markdown 正文。

要求：
1. 保留原始标题层级、列表、表格和引用结构。
2. 不要补充与原文无关的解释，不要添加前言或结语。
3. 如果原文已经是 Markdown，请继续输出合法 Markdown，不要人为增加多余空行或重排段落层级。
4. 如果原文来自 PDF/OCR，遇到仅因视觉换行产生的断行时，请合并为自然段；只有在标题、列表项、表格行、引用块等结构边界处才保留换行。
5. 明显属于页眉、页脚、页码、分页符、重复刊名、孤立控制符或重复段落的解析噪声，不要带入结果。
6. 无法确定的专有名词保留原文，并在必要时直接音译。
7. 只输出翻译后的正文，不要包裹代码块围栏。

任务类型：{{.TaskKind}}
目标格式：{{.OutputFormat}}
片段范围：{{.ChunkStart}}-{{.ChunkEnd}}

原文片段：
{{.Input}}
`
	data := map[string]string{
		"TargetLanguage": firstNonEmpty(options.TargetLanguage, types.LanguageNameFromContext(ctx), "Chinese (Simplified)"),
		"TaskKind":       task.TaskKind,
		"OutputFormat":   task.OutputFormat,
		"ChunkStart":     fmt.Sprintf("%d", batch.ChunkStartSeq),
		"ChunkEnd":       fmt.Sprintf("%d", batch.ChunkEndSeq),
		"Input":          batch.InputSnapshot,
	}
	tmpl, err := template.New("long_document_translation").Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (s *longDocumentTaskService) resolveTaskChatModel(ctx context.Context, kb *types.KnowledgeBase, options *types.LongDocumentTaskOptions) (modelchat.Chat, error) {
	candidates := make([]string, 0, 2)
	if options != nil {
		if override := strings.TrimSpace(options.SummaryModelID); override != "" {
			candidates = append(candidates, override)
		}
	}
	if kb != nil {
		if fallback := strings.TrimSpace(kb.SummaryModelID); fallback != "" {
			alreadyAdded := false
			for _, candidate := range candidates {
				if candidate == fallback {
					alreadyAdded = true
					break
				}
			}
			if !alreadyAdded {
				candidates = append(candidates, fallback)
			}
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("knowledge base summary model is not configured")
	}
	var lastErr error
	for idx, candidate := range candidates {
		chatModel, err := s.modelService.GetChatModel(ctx, candidate)
		if err == nil {
			if idx == 0 && options != nil && strings.TrimSpace(options.SummaryModelID) != "" {
				logger.Infof(ctx, "[LongDocument] Using task-level summary model override: %s", candidate)
			}
			return chatModel, nil
		}
		lastErr = err
		if idx == 0 && len(candidates) > 1 {
			logger.Warnf(ctx, "[LongDocument] Failed to load task-level summary model override %s, falling back to knowledge base model: %v", candidate, err)
		}
	}
	return nil, lastErr
}

func (s *longDocumentTaskService) planBatches(chunks []*types.Chunk) []longDocumentBatchPlan {
	if len(chunks) == 0 {
		return nil
	}
	batchChunkSize := max(1, s.cfg.LongDocument.BatchChunkSize)
	batchMaxChars := max(1, s.cfg.LongDocument.BatchMaxChars)
	plans := make([]longDocumentBatchPlan, 0)
	var (
		current longDocumentBatchAccumulator
		start   int
		end     int
		prevEnd int
	)
	prevEnd = -1
	flush := func() {
		if current.count == 0 {
			return
		}
		plans = append(plans, longDocumentBatchPlan{start: start, end: end, input: strings.Join(current.parts, "")})
		current = longDocumentBatchAccumulator{}
		start = 0
		end = 0
	}
	for _, chunk := range chunks {
		content := uniqueChunkContent(prevEnd, chunk)
		if content == "" {
			if chunk != nil && chunk.EndAt > prevEnd {
				prevEnd = chunk.EndAt
			}
			continue
		}
		if current.count == 0 {
			start = chunk.ChunkIndex
		}
		contentLen := len([]rune(content))
		if current.count > 0 && (current.count+1 > batchChunkSize || current.chars+contentLen > batchMaxChars) {
			flush()
			start = chunk.ChunkIndex
		}
		current.parts = append(current.parts, content)
		current.chars += contentLen
		current.count++
		end = chunk.ChunkIndex
		if chunk.EndAt > prevEnd {
			prevEnd = chunk.EndAt
		}
		if current.chars >= batchMaxChars {
			flush()
		}
	}
	flush()
	return plans
}

func buildLongDocumentSnapshotHash(chunks []*types.Chunk) string {
	hash := sha256.New()
	for _, chunk := range chunks {
		_, _ = hash.Write([]byte(fmt.Sprintf("%s:%d:%s\n", chunk.ID, chunk.ChunkIndex, chunk.Content)))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func estimateTokens(content string) int {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return 0
	}
	return len([]rune(trimmed)) / 4
}

func sanitizeGeneratedMarkdown(content string) string {
	trimmed := strings.TrimSpace(content)
	if matches := markdownFenceRE.FindStringSubmatch(trimmed); len(matches) == 2 {
		trimmed = strings.TrimSpace(matches[1])
	}
	trimmed = strings.ReplaceAll(trimmed, "\r\n", "\n")
	trimmed = strings.ReplaceAll(trimmed, "\f", "\n\n")
	return strings.TrimSpace(trimmed)
}

func uniqueChunkContent(previousEnd int, chunk *types.Chunk) string {
	if chunk == nil || strings.TrimSpace(chunk.Content) == "" {
		return ""
	}
	contentRunes := []rune(chunk.Content)
	if previousEnd < 0 || chunk.StartAt >= previousEnd {
		return string(contentRunes)
	}
	if chunk.EndAt <= previousEnd {
		return ""
	}
	suffixLen := chunk.EndAt - previousEnd
	offset := len(contentRunes) - suffixLen
	if offset < 0 {
		offset = 0
	}
	if offset > len(contentRunes) {
		offset = len(contentRunes)
	}
	return string(contentRunes[offset:])
}

func sanitizeArtifactBaseName(name string) string {
	base := strings.TrimSpace(name)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.ReplaceAll(base, "/", "-")
	base = strings.ReplaceAll(base, "\\", "-")
	base = strings.ReplaceAll(base, " ", "-")
	if base == "" {
		return "document"
	}
	return base
}

func storageBackendFromPath(filePath string) string {
	trimmed := strings.TrimSpace(filePath)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err == nil && parsed.Scheme != "" {
		return parsed.Scheme
	}
	if idx := strings.Index(trimmed, "://"); idx > 0 {
		return trimmed[:idx]
	}
	return ""
}

func resolveLongDocumentArtifactProvider(knowledge *types.Knowledge, kb *types.KnowledgeBase, artifactPath string) string {
	if provider := types.ParseProviderScheme(strings.TrimSpace(artifactPath)); provider != "" {
		return provider
	}
	if kb != nil {
		if provider := strings.ToLower(strings.TrimSpace(kb.GetStorageProvider())); provider != "" {
			return provider
		}
	}
	if knowledge != nil {
		if provider := types.InferStorageFromFilePath(strings.TrimSpace(knowledge.FilePath)); provider != "" {
			return provider
		}
	}
	return ""
}

func (s *longDocumentTaskService) resolveFileServiceForProvider(ctx context.Context, provider string) interfaces.FileService {
	resolvedProvider := strings.ToLower(strings.TrimSpace(provider))
	if resolvedProvider == "" {
		return s.fileService
	}
	if s.tenantService == nil {
		return s.fileService
	}
	tenantID := types.MustTenantIDFromContext(ctx)
	tenant, err := s.tenantService.GetTenantByID(ctx, tenantID)
	if err != nil || tenant == nil || tenant.StorageEngineConfig == nil {
		logger.Warnf(ctx, "[LongDocument] Failed to resolve tenant storage config for provider=%s tenant_id=%d err=%v", resolvedProvider, tenantID, err)
		return s.fileService
	}
	baseDir := strings.TrimSpace(os.Getenv("LOCAL_STORAGE_BASE_DIR"))
	fileSvc, actualProvider, err := filesvc.NewFileServiceFromStorageConfig(resolvedProvider, tenant.StorageEngineConfig, baseDir)
	if err != nil {
		logger.Warnf(ctx, "[LongDocument] Failed to build file service for provider=%s tenant_id=%d err=%v", resolvedProvider, tenantID, err)
		return s.fileService
	}
	logger.Infof(ctx, "[LongDocument] Resolved file service for provider=%s tenant_id=%d", actualProvider, tenantID)
	return fileSvc
}

func buildLongDocumentTaskEvents(task *types.LongDocumentTask, batches []*types.LongDocumentTaskBatch, artifact *types.LongDocumentArtifact) []types.LongDocumentTaskEvent {
	if task == nil {
		return nil
	}
	events := make([]types.LongDocumentTaskEvent, 0, len(batches)*2+5)
	if startedAt, ok := taskStartedAt(task, batches); ok {
		events = append(events, types.LongDocumentTaskEvent{
			Type:      "task_started",
			TaskID:    task.ID,
			Timestamp: startedAt,
			Data: map[string]interface{}{
				"status":            task.Status,
				"total_batches":     task.TotalBatches,
				"progress_percent":  task.ProgressPercent(),
				"completed_batches": task.CompletedBatches,
				"failed_batches":    task.FailedBatches,
			},
		})
	}
	for _, batch := range batches {
		if batch == nil {
			continue
		}
		if batch.StartedAt != nil {
			events = append(events, types.LongDocumentTaskEvent{
				Type:      "batch_started",
				TaskID:    task.ID,
				Timestamp: *batch.StartedAt,
				Data: map[string]interface{}{
					"batch_id":             batch.ID,
					"batch_no":             batch.BatchNo,
					"chunk_start_seq":      batch.ChunkStartSeq,
					"chunk_end_seq":        batch.ChunkEndSeq,
					"retry_count":          batch.RetryCount,
					"status":               batch.Status,
					"quality_status":       batch.QualityStatus,
					"input_token_estimate": batch.InputTokenEstimate,
				},
			})
		}
		switch batch.Status {
		case types.LongDocumentBatchStatusCompleted:
			timestamp := batch.UpdatedAt
			if batch.CompletedAt != nil {
				timestamp = *batch.CompletedAt
			}
			events = append(events, types.LongDocumentTaskEvent{
				Type:      "batch_completed",
				TaskID:    task.ID,
				Timestamp: timestamp,
				Data: map[string]interface{}{
					"batch_id":              batch.ID,
					"batch_no":              batch.BatchNo,
					"retry_count":           batch.RetryCount,
					"status":                batch.Status,
					"quality_status":        batch.QualityStatus,
					"output_token_estimate": batch.OutputTokenEstimate,
					"model_name":            batch.ModelName,
				},
			})
		case types.LongDocumentBatchStatusFailed:
			events = append(events, types.LongDocumentTaskEvent{
				Type:      "batch_failed",
				TaskID:    task.ID,
				Timestamp: batch.UpdatedAt,
				Data: map[string]interface{}{
					"batch_id":       batch.ID,
					"batch_no":       batch.BatchNo,
					"retry_count":    batch.RetryCount,
					"status":         batch.Status,
					"quality_status": batch.QualityStatus,
					"error_message":  batch.ErrorMessage,
				},
			})
		}
	}
	if assemblingAt, ok := taskAssemblingAt(task, batches); ok {
		events = append(events, types.LongDocumentTaskEvent{
			Type:      "task_assembling",
			TaskID:    task.ID,
			Timestamp: assemblingAt,
			Data: map[string]interface{}{
				"status":            task.Status,
				"completed_batches": task.CompletedBatches,
				"failed_batches":    task.FailedBatches,
				"total_batches":     task.TotalBatches,
			},
		})
	}
	if artifact != nil && artifact.Status == types.LongDocumentArtifactStatusAvailable {
		events = append(events, types.LongDocumentTaskEvent{
			Type:      "artifact_available",
			TaskID:    task.ID,
			Timestamp: artifact.CreatedAt,
			Data: map[string]interface{}{
				"artifact_id":     artifact.ID,
				"file_name":       artifact.FileName,
				"file_type":       artifact.FileType,
				"file_size":       artifact.FileSize,
				"checksum":        artifact.Checksum,
				"storage_backend": artifact.StorageBackend,
			},
		})
	}
	if terminalType, terminalAt, ok := taskTerminalEvent(task); ok {
		events = append(events, types.LongDocumentTaskEvent{
			Type:      terminalType,
			TaskID:    task.ID,
			Timestamp: terminalAt,
			Data: map[string]interface{}{
				"status":             task.Status,
				"completed_batches":  task.CompletedBatches,
				"failed_batches":     task.FailedBatches,
				"total_batches":      task.TotalBatches,
				"progress_percent":   task.ProgressPercent(),
				"artifact_available": task.ArtifactPath != "",
				"quality_status":     task.QualityStatus,
				"error_message":      task.ErrorMessage,
			},
		})
	}
	events = append(events, types.LongDocumentTaskEvent{
		Type:      "task.snapshot",
		TaskID:    task.ID,
		Timestamp: task.UpdatedAt,
		Data: map[string]interface{}{
			"status":             task.Status,
			"completed_batches":  task.CompletedBatches,
			"failed_batches":     task.FailedBatches,
			"total_batches":      task.TotalBatches,
			"progress_percent":   task.ProgressPercent(),
			"artifact_available": task.ArtifactPath != "",
		},
	})
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Timestamp.Equal(events[j].Timestamp) {
			return events[i].Type < events[j].Type
		}
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	return events
}

func prepareTaskForRetry(task *types.LongDocumentTask, batches []*types.LongDocumentTaskBatch) bool {
	if task == nil {
		return false
	}
	hasRetryable := task.Status == types.LongDocumentTaskStatusFailed
	completed := 0
	for _, batch := range batches {
		if batch == nil {
			continue
		}
		if batch.Status == types.LongDocumentBatchStatusCompleted {
			completed++
			continue
		}
		hasRetryable = true
		batch.Status = types.LongDocumentBatchStatusPending
		batch.ErrorMessage = ""
		batch.OutputPayload = ""
		batch.OutputTokenEstimate = 0
		batch.ModelName = ""
		batch.QualityStatus = "pending"
		batch.StartedAt = nil
		batch.CompletedAt = nil
	}
	if !hasRetryable {
		return false
	}
	task.Status = types.LongDocumentTaskStatusPending
	task.ErrorMessage = ""
	task.CompletedAt = nil
	task.CancelledAt = nil
	task.ArtifactID = ""
	task.ArtifactPath = ""
	task.Artifact = nil
	task.CompletedBatches = completed
	task.FailedBatches = 0
	task.QualityStatus = "pending"
	return true
}

func taskStartedAt(task *types.LongDocumentTask, batches []*types.LongDocumentTaskBatch) (time.Time, bool) {
	for _, batch := range batches {
		if batch != nil && batch.StartedAt != nil {
			return *batch.StartedAt, true
		}
	}
	if task.Status != types.LongDocumentTaskStatusPending {
		return task.UpdatedAt, true
	}
	return time.Time{}, false
}

func taskAssemblingAt(task *types.LongDocumentTask, batches []*types.LongDocumentTaskBatch) (time.Time, bool) {
	switch task.Status {
	case types.LongDocumentTaskStatusAssembling, types.LongDocumentTaskStatusPartial, types.LongDocumentTaskStatusCompleted, types.LongDocumentTaskStatusFailed:
		latest := time.Time{}
		for _, batch := range batches {
			if batch == nil {
				continue
			}
			candidate := batch.UpdatedAt
			if batch.CompletedAt != nil {
				candidate = *batch.CompletedAt
			}
			if candidate.After(latest) {
				latest = candidate
			}
		}
		if latest.IsZero() {
			latest = task.UpdatedAt
		}
		return latest, true
	default:
		return time.Time{}, false
	}
}

func taskTerminalEvent(task *types.LongDocumentTask) (string, time.Time, bool) {
	switch task.Status {
	case types.LongDocumentTaskStatusCompleted:
		return "task_completed", firstNonZeroTime(task.CompletedAt, task.UpdatedAt), true
	case types.LongDocumentTaskStatusPartial:
		return "task_partial", firstNonZeroTime(task.CompletedAt, task.UpdatedAt), true
	case types.LongDocumentTaskStatusFailed:
		return "task_failed", firstNonZeroTime(task.CompletedAt, task.UpdatedAt), true
	case types.LongDocumentTaskStatusCancelled:
		return "task_cancelled", firstNonZeroTime(task.CancelledAt, task.UpdatedAt), true
	default:
		return "", time.Time{}, false
	}
}

func firstNonZeroTime(pointer *time.Time, fallback time.Time) time.Time {
	if pointer != nil && !pointer.IsZero() {
		return *pointer
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
