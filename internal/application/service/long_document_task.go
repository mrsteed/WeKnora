package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/hibiken/asynq"
)

const (
	longDocumentNextActionContinueAuto   = "continue_auto"
	longDocumentNextActionWaitUserReview = "wait_user_review"
	longDocumentNextActionBlocked        = "blocked"
	longDocumentNextActionDone           = "done"
	longDocumentNextActionManualRetry    = "manual_retry"
	longDocumentAutoContinuePrompt       = "以当前文档为基准，继续剩余内容输出"
)

type longDocumentTaskHandler struct {
	sessionService              interfaces.SessionService
	messageService              interfaces.MessageService
	streamManager               interfaces.StreamManager
	chatDocumentArtifactService interfaces.ChatDocumentArtifactService
}

type longDocumentStreamForwarder struct {
	ctx           context.Context
	sessionID     string
	messageID     string
	streamManager interfaces.StreamManager
}

type longDocumentTaskCollector struct {
	messageService              interfaces.MessageService
	streamManager               interfaces.StreamManager
	chatDocumentArtifactService interfaces.ChatDocumentArtifactService
	sessionService              interfaces.SessionService
	streamedAnswer              strings.Builder
	completeData                *event.AgentCompleteData
	errorData                   *event.ErrorData
}

type longDocumentContinuationDecision struct {
	action          string
	reason          string
	reasonMessage   string
	canAutoContinue bool
}

func NewLongDocumentTaskHandler(
	sessionService interfaces.SessionService,
	messageService interfaces.MessageService,
	streamManager interfaces.StreamManager,
	chatDocumentArtifactService interfaces.ChatDocumentArtifactService,
) interfaces.TaskHandler {
	return &longDocumentTaskHandler{
		sessionService:              sessionService,
		messageService:              messageService,
		streamManager:               streamManager,
		chatDocumentArtifactService: chatDocumentArtifactService,
	}
}

func shouldDispatchLongDocumentTask(mode string, req *types.QARequest) bool {
	if req == nil || req.Session == nil || strings.TrimSpace(req.AssistantMessageID) == "" {
		return false
	}
	if len(req.ImageURLs) > 0 || len(req.Attachments) > 0 {
		return false
	}
	if shouldUseLongDocumentTranslationPath(req) || shouldUseLongDocumentTranslationContinuationPath(req) {
		return true
	}
	if mode != types.LongDocumentExecutionModeAgentQA {
		return false
	}
	if req.AutoContinue && strings.TrimSpace(req.GenerationRunID) != "" {
		return true
	}
	return strings.TrimSpace(req.DocumentOutputMode) == types.ChatDocumentOutputModeFull
}

func (s *sessionService) DispatchLongDocumentTask(ctx context.Context, req *types.QARequest, mode string) (bool, error) {
	if s == nil || s.taskEnqueuer == nil || s.streamManager == nil {
		return false, nil
	}
	if !shouldDispatchLongDocumentTask(mode, req) {
		return false, nil
	}
	payload := types.LongDocumentExecutionPayload{
		Mode:      mode,
		TenantID:  types.MustTenantIDFromContext(ctx),
		RequestID: strings.TrimSpace(req.AssistantMessageID),
		Language:  firstNonEmptyString(languageFromContextOrDefault(ctx), types.DefaultLanguage()),
		Request:   *req,
	}
	if sessionTenantID, ok := types.SessionTenantIDFromContext(ctx); ok {
		payload.SessionTenantID = sessionTenantID
	}
	if requestID, ok := types.RequestIDFromContext(ctx); ok {
		payload.RequestID = requestID
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	task := asynq.NewTask(types.TypeLongDocumentExecution, body)
	if _, err := s.taskEnqueuer.Enqueue(task); err != nil {
		return false, err
	}
	queuedEvent := interfaces.StreamEvent{
		ID:        generateEventID("document-queued"),
		Type:      types.ResponseTypeThinking,
		Content:   "长文档批次任务已入队，正在启动后台执行。",
		Done:      true,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"event_id":       generateEventID("document-queued-meta"),
			"synthetic":      true,
			"stage":          "queued",
			"progress_label": "queued",
		},
	}
	if err := s.streamManager.AppendEvent(ctx, req.Session.ID, req.AssistantMessageID, queuedEvent); err != nil {
		logger.Warnf(ctx, "Failed to append long document queued event, session_id: %s, message_id: %s, error: %v", req.Session.ID, req.AssistantMessageID, err)
	}
	return true, nil
}

func (h *longDocumentTaskHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.sessionService == nil || h.messageService == nil || h.streamManager == nil {
		return fmt.Errorf("long document task handler dependencies are incomplete")
	}
	var payload types.LongDocumentExecutionPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	execCtx := withLongDocumentTaskContext(ctx, &payload)
	return h.process(execCtx, &payload)
}

func (h *longDocumentTaskHandler) process(ctx context.Context, payload *types.LongDocumentExecutionPayload) error {
	if payload == nil || payload.Request.Session == nil {
		return fmt.Errorf("long document task request is incomplete")
	}
	req := &payload.Request
	message, err := h.messageService.GetMessage(ctx, req.Session.ID, req.AssistantMessageID)
	if err != nil {
		return err
	}
	if message == nil {
		return fmt.Errorf("assistant message %s not found", req.AssistantMessageID)
	}

	bus := event.NewEventBus()
	forwarder := &longDocumentStreamForwarder{
		ctx:           ctx,
		sessionID:     req.Session.ID,
		messageID:     req.AssistantMessageID,
		streamManager: h.streamManager,
	}
	forwarder.Subscribe(bus)
	collector := &longDocumentTaskCollector{
		messageService:              h.messageService,
		streamManager:               h.streamManager,
		chatDocumentArtifactService: h.chatDocumentArtifactService,
		sessionService:              h.sessionService,
	}
	collector.Subscribe(bus)

	var runErr error
	switch payload.Mode {
	case types.LongDocumentExecutionModeKnowledgeQA:
		runErr = h.sessionService.KnowledgeQA(ctx, req, bus)
	case types.LongDocumentExecutionModeAgentQA:
		runErr = h.sessionService.AgentQA(ctx, req, bus)
	default:
		runErr = fmt.Errorf("unsupported long document task mode: %s", payload.Mode)
	}

	return collector.Finalize(ctx, req, message, runErr)
}

func withLongDocumentTaskContext(ctx context.Context, payload *types.LongDocumentExecutionPayload) context.Context {
	if payload == nil {
		return ctx
	}
	next := logger.CloneContext(ctx)
	if payload.TenantID != 0 {
		next = context.WithValue(next, types.TenantIDContextKey, payload.TenantID)
	}
	if payload.SessionTenantID != 0 {
		next = context.WithValue(next, types.SessionTenantIDContextKey, payload.SessionTenantID)
	}
	if strings.TrimSpace(payload.Language) != "" {
		next = context.WithValue(next, types.LanguageContextKey, strings.TrimSpace(payload.Language))
	}
	if strings.TrimSpace(payload.RequestID) != "" {
		next = context.WithValue(next, types.RequestIDContextKey, strings.TrimSpace(payload.RequestID))
	}
	if payload.Request.Session != nil && strings.TrimSpace(payload.Request.Session.UserID) != "" {
		next = context.WithValue(next, types.UserIDContextKey, strings.TrimSpace(payload.Request.Session.UserID))
	}
	return next
}

func (f *longDocumentStreamForwarder) Subscribe(bus *event.EventBus) {
	if f == nil || bus == nil {
		return
	}
	bus.On(event.EventAgentThought, f.handleThought)
	bus.On(event.EventAgentFinalAnswer, f.handleFinalAnswer)
	bus.On(event.EventError, f.handleError)
	bus.On(event.EventSessionTitle, f.handleSessionTitle)
}

func (f *longDocumentStreamForwarder) handleThought(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.AgentThoughtData)
	if !ok {
		return nil
	}
	metadata := map[string]interface{}{"event_id": evt.ID}
	if data.Replace {
		metadata["replace"] = true
	}
	if data.Synthetic {
		metadata["synthetic"] = true
	}
	if strings.TrimSpace(data.Stage) != "" {
		metadata["stage"] = strings.TrimSpace(data.Stage)
	}
	if len(data.Outline) > 0 {
		metadata["outline"] = data.Outline
	}
	if data.SectionCurrent > 0 {
		metadata["section_current"] = data.SectionCurrent
	}
	if data.SectionTotal > 0 {
		metadata["section_total"] = data.SectionTotal
	}
	if strings.TrimSpace(data.SectionTitle) != "" {
		metadata["section_title"] = strings.TrimSpace(data.SectionTitle)
	}
	if data.QueryCurrent > 0 {
		metadata["query_current"] = data.QueryCurrent
	}
	if data.QueryTotal > 0 {
		metadata["query_total"] = data.QueryTotal
	}
	if strings.TrimSpace(data.ProgressLabel) != "" {
		metadata["progress_label"] = strings.TrimSpace(data.ProgressLabel)
	}
	return f.streamManager.AppendEvent(f.ctx, f.sessionID, f.messageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeThinking,
		Content:   data.Content,
		Done:      data.Done,
		Timestamp: time.Now(),
		Data:      metadata,
	})
}

func (f *longDocumentStreamForwarder) handleFinalAnswer(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.AgentFinalAnswerData)
	if !ok {
		return nil
	}
	metadata := map[string]interface{}{"event_id": evt.ID}
	if data.IsFallback {
		metadata["is_fallback"] = true
	}
	if strings.TrimSpace(data.CompletionStatus) != "" {
		metadata["completion_status"] = data.CompletionStatus
	}
	if strings.TrimSpace(data.FinishReason) != "" {
		metadata["finish_reason"] = data.FinishReason
	}
	if data.IsPartial {
		metadata["is_partial"] = true
	}
	if data.AllowIndexing {
		metadata["allow_indexing"] = true
	}
	if data.AllowComplete {
		metadata["allow_complete"] = true
	}
	if strings.TrimSpace(data.FailureReason) != "" {
		metadata["failure_reason"] = data.FailureReason
	}
	if strings.TrimSpace(data.DocumentGenerationStatus) != "" {
		metadata["document_generation_status"] = types.NormalizeChatDocumentGenerationStatus(data.DocumentGenerationStatus)
	}
	return f.streamManager.AppendEvent(f.ctx, f.sessionID, f.messageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeAnswer,
		Content:   data.Content,
		Done:      data.Done,
		Timestamp: time.Now(),
		Data:      metadata,
	})
}

func (f *longDocumentStreamForwarder) handleError(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.ErrorData)
	if !ok {
		return nil
	}
	return f.streamManager.AppendEvent(f.ctx, f.sessionID, f.messageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeError,
		Content:   data.Error,
		Done:      true,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"stage": data.Stage,
			"error": data.Error,
		},
	})
}

func (f *longDocumentStreamForwarder) handleSessionTitle(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.SessionTitleData)
	if !ok {
		return nil
	}
	return f.streamManager.AppendEvent(context.Background(), f.sessionID, f.messageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeSessionTitle,
		Content:   data.Title,
		Done:      true,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"session_id": data.SessionID,
			"title":      data.Title,
		},
	})
}

func (c *longDocumentTaskCollector) Subscribe(bus *event.EventBus) {
	if c == nil || bus == nil {
		return
	}
	bus.On(event.EventAgentFinalAnswer, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		if !ok {
			return nil
		}
		if strings.TrimSpace(data.Content) != "" {
			c.streamedAnswer.WriteString(data.Content)
		}
		return nil
	})
	bus.On(event.EventAgentComplete, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		if !ok {
			return nil
		}
		copied := data
		c.completeData = &copied
		return nil
	})
	bus.On(event.EventError, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.ErrorData)
		if !ok {
			return nil
		}
		copied := data
		c.errorData = &copied
		return nil
	})
}

func (c *longDocumentTaskCollector) Finalize(ctx context.Context, req *types.QARequest, message *types.Message, runErr error) error {
	if c == nil || req == nil || req.Session == nil || message == nil {
		return runErr
	}
	completion := c.completeData
	if completion == nil {
		completion = c.buildFallbackCompletion(req, runErr)
	}
	artifact, data, persistErr := c.persistCompletion(ctx, req, message, completion)
	if persistErr != nil {
		logger.Warnf(ctx, "Failed to persist long document task completion, session_id: %s, message_id: %s, error: %v", req.Session.ID, message.ID, persistErr)
	}
	completePayload := c.buildCompletePayload(req, message, completion, artifact, data)
	if err := c.streamManager.AppendEvent(ctx, req.Session.ID, message.ID, interfaces.StreamEvent{
		ID:        generateEventID("task-complete"),
		Type:      types.ResponseTypeComplete,
		Content:   "",
		Done:      true,
		Timestamp: time.Now(),
		Data:      completePayload,
	}); err != nil {
		logger.Warnf(ctx, "Failed to append long document task complete event, session_id: %s, message_id: %s, error: %v", req.Session.ID, message.ID, err)
	}
	if runErr != nil {
		return runErr
	}
	return persistErr
}

func (c *longDocumentTaskCollector) buildFallbackCompletion(req *types.QARequest, runErr error) *event.AgentCompleteData {
	finalAnswer := strings.TrimSpace(c.streamedAnswer.String())
	completionStatus := types.MessageCompletionStatusFailed
	finishReason := "task_failed"
	failureReason := "task_failed"
	documentStatus := types.ChatDocumentGenerationStatusBlocked
	if finalAnswer != "" {
		completionStatus = types.MessageCompletionStatusPartial
		finishReason = "task_partial"
		failureReason = firstNonEmptyString(errorFailureReason(c.errorData), classifyDocumentEditError(runErr))
		documentStatus = types.ChatDocumentGenerationStatusContinuing
	} else if c.errorData != nil {
		finishReason = firstNonEmptyString(strings.TrimSpace(c.errorData.Stage), finishReason)
		failureReason = firstNonEmptyString(classifyDocumentEditError(runErr), strings.TrimSpace(c.errorData.Stage), failureReason)
	}
	return &event.AgentCompleteData{
		SessionID:                req.Session.ID,
		MessageID:                req.AssistantMessageID,
		FinalAnswer:              finalAnswer,
		CompletionStatus:         completionStatus,
		FinishReason:             finishReason,
		FailureReason:            failureReason,
		AllowIndexing:            false,
		AllowComplete:            false,
		IsPartial:                completionStatus == types.MessageCompletionStatusPartial,
		DocumentGenerationStatus: documentStatus,
		TotalDurationMs:          0,
		Extra:                    map[string]interface{}{},
	}
}

func (c *longDocumentTaskCollector) persistCompletion(
	ctx context.Context,
	req *types.QARequest,
	message *types.Message,
	completion *event.AgentCompleteData,
) (*types.ChatDocumentArtifact, map[string]interface{}, error) {
	if completion == nil {
		return nil, nil, fmt.Errorf("long document completion is empty")
	}
	stored, err := c.messageService.GetMessage(ctx, req.Session.ID, message.ID)
	if err == nil && stored != nil {
		*message = *stored
	}
	finalAnswer := strings.TrimSpace(completion.FinalAnswer)
	if finalAnswer == "" {
		finalAnswer = strings.TrimSpace(c.streamedAnswer.String())
	}
	artifactDocumentStatus := types.NormalizeChatDocumentGenerationStatus(completion.DocumentGenerationStatus)
	if stripped, completed := types.StripChatDocumentCompletionMarker(finalAnswer); completed {
		finalAnswer = stripped
		artifactDocumentStatus = types.ChatDocumentGenerationStatusCompleted
	}
	message.Content = finalAnswer
	message.CompletionStatus = firstNonEmptyString(strings.TrimSpace(completion.CompletionStatus), message.CompletionStatus)
	message.FinishReason = firstNonEmptyString(strings.TrimSpace(completion.FinishReason), message.FinishReason)
	message.FailureReason = firstNonEmptyString(strings.TrimSpace(completion.FailureReason), message.FailureReason)
	message.IsCompleted = completion.AllowComplete && message.CompletionStatus == types.MessageCompletionStatusCompleted
	if len(completion.AgentSteps) > 0 {
		message.AgentSteps = append(types.AgentSteps(nil), completion.AgentSteps...)
	}
	if completion.TotalDurationMs > 0 {
		message.AgentDurationMs = completion.TotalDurationMs
	}
	if err := c.messageService.UpdateMessage(ctx, message); err != nil {
		return nil, nil, err
	}

	extra := cloneLongDocumentExtra(completion.Extra)
	generationRunID := longDocumentGenerationRunIDFromExtra(extra)
	artifactOptions := types.RegisterChatDocumentArtifactOptions{
		UserQuery:                req.Query,
		Intent:                   req.DocumentIntent,
		Operation:                req.DocumentOperation,
		OutputMode:               req.DocumentOutputMode,
		DocumentTaskKind:         strings.TrimSpace(req.DocumentTaskKind),
		TargetLanguage:           longDocumentTargetLanguage(req.TranslationOptions),
		TranslationOutputFormat:  longDocumentOutputFormat(req.TranslationOptions),
		NeedArtifact:             true,
		UseLongDocument:          true,
		TargetHeading:            req.DocumentTargetHeading,
		MergeMode:                req.DocumentMergeMode,
		BaseArtifact:             req.BaseArtifact,
		GenerationRunID:          generationRunID,
		DocumentGenerationStatus: artifactDocumentStatus,
		EvidenceRefs:             types.NormalizeChatDocumentEvidenceRefs(extra["evidence_refs"]),
	}
	aggregated := AggregateDocumentGenerationArtifact(ctx, c.sessionService, c.chatDocumentArtifactService, DocumentGenerationAggregateInput{
		Message:         message,
		RegisterOptions: artifactOptions,
		StateBuilder: func(artifact *types.ChatDocumentArtifact) types.ChatDocumentGenerationRunState {
			return buildLongDocumentGenerationRunStateUpdate(message, artifact, completion, req, extra)
		},
	})
	artifact := aggregated.Artifact
	if aggregated.ArtifactErr != nil {
		logger.Warnf(ctx, "Failed to register long document artifact, session_id: %s, message_id: %s, error: %v", req.Session.ID, message.ID, aggregated.ArtifactErr)
	}
	if aggregated.BindErr != nil {
		logger.Warnf(ctx, "Failed to bind long document root artifact, session_id: %s, message_id: %s, run_id: %s, error: %v", req.Session.ID, message.ID, generationRunID, aggregated.BindErr)
	}
	if aggregated.StateErr != nil {
		logger.Warnf(ctx, "Failed to record long document generation run state, session_id: %s, message_id: %s, run_id: %s, error: %v", req.Session.ID, message.ID, generationRunID, aggregated.StateErr)
	} else if aggregated.State != nil {
		if extra == nil {
			extra = map[string]interface{}{}
		}
		if data := aggregated.State.Data(); len(data) > 0 {
			extra["generation_run_state"] = data
		}
	}
	if completion.AllowIndexing && strings.TrimSpace(message.Content) != "" {
		bgCtx := context.WithoutCancel(ctx)
		go c.messageService.IndexMessageToKB(bgCtx, req.Query, message.Content, message.ID, message.SessionID, interfaces.MessageIndexOptions{
			CompletionStatus:         message.CompletionStatus,
			FinishReason:             message.FinishReason,
			AllowIndexing:            true,
			TaskKind:                 strings.TrimSpace(req.DocumentTaskKind),
			DocumentGenerationStatus: artifactDocumentStatus,
			ArtifactID:               completedArtifactID(artifact),
			DocumentTitle:            completedArtifactTitle(artifact),
		})
	}
	return artifact, extra, nil
}

func (c *longDocumentTaskCollector) buildCompletePayload(
	req *types.QARequest,
	message *types.Message,
	completion *event.AgentCompleteData,
	artifact *types.ChatDocumentArtifact,
	extra map[string]interface{},
) map[string]interface{} {
	status := strings.TrimSpace(completion.CompletionStatus)
	finishReason := strings.TrimSpace(completion.FinishReason)
	failureReason := strings.TrimSpace(completion.FailureReason)
	payload := map[string]interface{}{
		"final_answer":      message.Content,
		"agent_steps":       message.AgentSteps,
		"agent_duration_ms": completion.TotalDurationMs,
		"total_steps":       len(message.AgentSteps),
		"total_duration_ms": completion.TotalDurationMs,
		"completion_status": status,
		"finish_reason":     finishReason,
		"is_partial":        status == types.MessageCompletionStatusPartial,
		"allow_indexing":    completion.AllowIndexing,
		"allow_complete":    completion.AllowComplete,
		"failure_reason":    failureReason,
	}
	if documentStatus := types.NormalizeChatDocumentGenerationStatus(firstNonEmptyString(completion.DocumentGenerationStatus, completedArtifactStatus(artifact))); documentStatus != "" {
		payload["document_generation_status"] = documentStatus
	}
	if completion.AutoContinueNext != nil {
		payload["auto_continue_next"] = *completion.AutoContinueNext
	}
	if strings.TrimSpace(completion.AutoContinueReason) != "" {
		payload["auto_continue_reason"] = strings.TrimSpace(completion.AutoContinueReason)
	}
	if strings.TrimSpace(completion.AutoContinueReasonMessage) != "" {
		payload["auto_continue_reason_message"] = strings.TrimSpace(completion.AutoContinueReasonMessage)
	}
	if strings.TrimSpace(completion.NextAction) != "" {
		payload["next_action"] = strings.TrimSpace(completion.NextAction)
	}
	if strings.TrimSpace(completion.NextReason) != "" {
		payload["next_reason"] = strings.TrimSpace(completion.NextReason)
	}
	if strings.TrimSpace(completion.NextReasonMessage) != "" {
		payload["next_reason_message"] = strings.TrimSpace(completion.NextReasonMessage)
	}
	if completion.CanAutoContinue != nil {
		payload["can_auto_continue"] = *completion.CanAutoContinue
	}
	if len(completion.RecommendedRequest) > 0 {
		payload["recommended_request"] = completion.RecommendedRequest
	}
	for key, value := range extra {
		if _, exists := payload[key]; !exists {
			payload[key] = value
		}
	}
	populateLongDocumentContinuationPayload(payload, artifact, req.AutoContinueRound)
	if artifact != nil {
		payload["chat_document_artifact"] = longDocumentArtifactMetadata(artifact)
		finalDocumentMode, finalDocument, finalDocumentArtifactID := longDocumentFinalDocumentDelivery(artifact)
		payload["final_document_mode"] = finalDocumentMode
		if finalDocument != "" {
			payload["final_document"] = finalDocument
		}
		payload["final_document_artifact_id"] = finalDocumentArtifactID
	}
	return payload
}

func buildLongDocumentGenerationRunStateUpdate(
	message *types.Message,
	artifact *types.ChatDocumentArtifact,
	completion *event.AgentCompleteData,
	req *types.QARequest,
	extra map[string]interface{},
) types.ChatDocumentGenerationRunState {
	state := types.ChatDocumentGenerationRunState{
		TaskKind:             firstNonEmptyString(completedArtifactTaskKind(artifact), strings.TrimSpace(req.DocumentTaskKind)),
		LastCompletionStatus: strings.TrimSpace(completion.CompletionStatus),
		LastFinishReason:     strings.TrimSpace(completion.FinishReason),
		LastFailureReason:    strings.TrimSpace(completion.FailureReason),
		LastDocumentStatus:   types.NormalizeChatDocumentGenerationStatus(completion.DocumentGenerationStatus),
		AutoContinueRound:    req.AutoContinueRound,
	}
	if message != nil {
		state.LastCompletionStatus = firstNonEmptyString(strings.TrimSpace(message.CompletionStatus), state.LastCompletionStatus)
		state.LastFinishReason = firstNonEmptyString(strings.TrimSpace(message.FinishReason), state.LastFinishReason)
		state.LastFailureReason = firstNonEmptyString(strings.TrimSpace(message.FailureReason), state.LastFailureReason)
	}
	if artifact != nil {
		state.ActiveArtifactID = strings.TrimSpace(artifact.ID)
		state.LastDocumentStatus = types.NormalizeChatDocumentGenerationStatus(firstNonEmptyString(artifact.DocumentGenerationStatus, state.LastDocumentStatus))
		state.LastSnapshotCharCount = max(artifact.SnapshotCharCount, 0)
	}
	mergeLongDocumentRunStateProgress(&state, extra)
	return types.NormalizeChatDocumentGenerationRunState(state)
}

func populateLongDocumentContinuationPayload(data map[string]interface{}, artifact *types.ChatDocumentArtifact, autoContinueRound int) {
	if data == nil {
		return
	}
	status, _ := data["document_generation_status"].(string)
	finishReason, _ := data["finish_reason"].(string)
	failureReason, _ := data["failure_reason"].(string)
	decision := buildLongDocumentContinuationDecision(status, finishReason, failureReason, autoContinueRound, artifact, data)
	data["auto_continue_next"] = decision.canAutoContinue
	if decision.reason != "" {
		data["auto_continue_reason"] = decision.reason
	}
	if decision.reasonMessage != "" {
		data["auto_continue_reason_message"] = decision.reasonMessage
	}
	if decision.action != "" {
		data["next_action"] = decision.action
	}
	if decision.reason != "" {
		data["next_reason"] = decision.reason
	}
	if decision.reasonMessage != "" {
		data["next_reason_message"] = decision.reasonMessage
	}
	data["can_auto_continue"] = decision.canAutoContinue
	if request := buildLongDocumentRecommendedRequest(decision, artifact, data, autoContinueRound); len(request) > 0 {
		data["recommended_request"] = request
	}
}

func buildLongDocumentContinuationDecision(status string, finishReason string, failureReason string, autoContinueRound int, artifact *types.ChatDocumentArtifact, extra map[string]interface{}) longDocumentContinuationDecision {
	normalizedStatus := types.NormalizeChatDocumentGenerationStatus(status)
	state := longDocumentGenerationRunStateFromExtra(extra)
	reason := longDocumentAutoContinueReasonWithState(normalizedStatus, finishReason, failureReason, autoContinueRound, state, artifact)
	canContinue := canAutoContinueLongDocumentWithState(normalizedStatus, finishReason, failureReason, autoContinueRound, state)
	reasonMessage := longDocumentContinuationReasonMessage(reason, failureReason, finishReason)
	switch normalizedStatus {
	case types.ChatDocumentGenerationStatusCompleted:
		return longDocumentContinuationDecision{action: longDocumentNextActionDone, reason: firstNonEmptyString(reason, "document_complete_marker"), reasonMessage: reasonMessage}
	case types.ChatDocumentGenerationStatusBlocked:
		return longDocumentContinuationDecision{action: longDocumentNextActionBlocked, reason: firstNonEmptyString(reason, "document_generation_blocked"), reasonMessage: reasonMessage}
	case types.ChatDocumentGenerationStatusNeedsReview:
		return longDocumentContinuationDecision{action: longDocumentNextActionWaitUserReview, reason: firstNonEmptyString(reason, "document_generation_needs_review"), reasonMessage: reasonMessage}
	case types.ChatDocumentGenerationStatusContinuing:
		if canContinue {
			return longDocumentContinuationDecision{action: longDocumentNextActionContinueAuto, reason: reason, reasonMessage: reasonMessage, canAutoContinue: true}
		}
		return longDocumentContinuationDecision{action: longDocumentNextActionManualRetry, reason: firstNonEmptyString(reason, failureReason, finishReason), reasonMessage: reasonMessage}
	default:
		if canContinue {
			return longDocumentContinuationDecision{action: longDocumentNextActionContinueAuto, reason: reason, reasonMessage: reasonMessage, canAutoContinue: true}
		}
		return longDocumentContinuationDecision{action: longDocumentNextActionManualRetry, reason: firstNonEmptyString(reason, failureReason, finishReason), reasonMessage: reasonMessage}
	}
}

func buildLongDocumentRecommendedRequest(decision longDocumentContinuationDecision, artifact *types.ChatDocumentArtifact, extra map[string]interface{}, autoContinueRound int) map[string]interface{} {
	if decision.action != longDocumentNextActionContinueAuto {
		return nil
	}
	request := map[string]interface{}{
		"query":                   longDocumentAutoContinuePrompt,
		"intent_hint":             types.ChatDocumentIntentContinue,
		"auto_continue":           true,
		"auto_continue_prompt":    longDocumentAutoContinuePrompt,
		"auto_continue_round":     autoContinueRound + 1,
		"document_target_heading": nil,
		"document_merge_mode":     nil,
	}
	if artifact != nil && strings.TrimSpace(artifact.ID) != "" {
		request["base_artifact_id"] = strings.TrimSpace(artifact.ID)
		request["document_output_mode"] = types.ChatDocumentOutputModeDelta
	}
	if runID := longDocumentGenerationRunIDFromExtra(extra); runID != "" {
		request["generation_run_id"] = runID
		if artifact == nil || strings.TrimSpace(artifact.ID) == "" {
			request["document_output_mode"] = types.ChatDocumentOutputModeFull
		}
	}
	if _, ok := extra["translation_progress"]; ok {
		request["document_task_kind"] = types.ChatDocumentTaskKindTranslation
	}
	return request
}

func longDocumentGenerationRunIDFromExtra(extra map[string]interface{}) string {
	if len(extra) == 0 {
		return ""
	}
	if runID, ok := extra["generation_run_id"].(string); ok {
		return strings.TrimSpace(runID)
	}
	return ""
}

func longDocumentGenerationRunStateFromExtra(extra map[string]interface{}) types.ChatDocumentGenerationRunState {
	if len(extra) == 0 {
		return types.ChatDocumentGenerationRunState{}
	}
	raw, exists := extra["generation_run_state"]
	if !exists || raw == nil {
		return types.ChatDocumentGenerationRunState{}
	}
	stateMap, ok := raw.(map[string]interface{})
	if !ok {
		return types.ChatDocumentGenerationRunState{}
	}
	return types.NormalizeChatDocumentGenerationRunState(types.ChatDocumentGenerationRunState{
		TaskKind:                stringValueFromMap(stateMap, "task_kind"),
		ActiveArtifactID:        stringValueFromMap(stateMap, "active_artifact_id"),
		LastCompletionStatus:    stringValueFromMap(stateMap, "last_completion_status"),
		LastFinishReason:        stringValueFromMap(stateMap, "last_finish_reason"),
		LastFailureReason:       stringValueFromMap(stateMap, "last_failure_reason"),
		LastDocumentStatus:      stringValueFromMap(stateMap, "last_document_generation_status"),
		LastAutoContinueReason:  stringValueFromMap(stateMap, "last_auto_continue_reason"),
		AutoContinueRound:       intValueFromMap(stateMap, "auto_continue_round"),
		MaxAutoContinueRounds:   intValueFromMap(stateMap, "max_auto_continue_rounds"),
		MinGrowthChars:          intValueFromMap(stateMap, "min_growth_chars"),
		MaxLowGrowthRounds:      intValueFromMap(stateMap, "max_low_growth_rounds"),
		LastSnapshotCharCount:   intValueFromMap(stateMap, "last_snapshot_char_count"),
		LowGrowthRounds:         intValueFromMap(stateMap, "low_growth_rounds"),
		CompletedCount:          intValueFromMap(stateMap, "completed_count"),
		RemainingCount:          intValueFromMap(stateMap, "remaining_count"),
		NextSourceChunkStartSeq: intValueFromMap(stateMap, "next_source_chunk_start_seq"),
		NextSourceChunkEndSeq:   intValueFromMap(stateMap, "next_source_chunk_end_seq"),
		NextSection:             stringValueFromMap(stateMap, "next_section"),
	})
}

func canAutoContinueLongDocumentWithState(status string, finishReason string, failureReason string, autoContinueRound int, state types.ChatDocumentGenerationRunState) bool {
	if !shouldAutoContinueChatDocument(status, finishReason, failureReason, autoContinueRound) {
		return false
	}
	if state.MaxAutoContinueRounds > 0 && autoContinueRound >= state.MaxAutoContinueRounds {
		return false
	}
	if state.MaxLowGrowthRounds > 0 && state.LowGrowthRounds >= state.MaxLowGrowthRounds {
		return false
	}
	return true
}

func longDocumentAutoContinueReasonWithState(status string, finishReason string, failureReason string, autoContinueRound int, state types.ChatDocumentGenerationRunState, artifact *types.ChatDocumentArtifact) string {
	if artifact != nil {
		issues := artifact.QualityIssues
		switch {
		case containsChatDocumentQualityIssue(issues, types.ChatDocumentQualityIssueDuplicateDocumentHead):
			return types.ChatDocumentQualityIssueDuplicateDocumentHead
		case containsChatDocumentQualityIssue(issues, types.ChatDocumentQualityIssueSectionNumberReset):
			return types.ChatDocumentQualityIssueSectionNumberReset
		case containsChatDocumentQualityIssue(issues, types.ChatDocumentQualityIssueLowNoveltyDelta):
			return types.ChatDocumentQualityIssueLowNoveltyDelta
		case containsChatDocumentQualityIssue(issues, types.ChatDocumentQualityIssueTerminalSectionTail):
			return types.ChatDocumentQualityIssueTerminalSectionTail
		}
	}
	switch types.NormalizeChatDocumentGenerationStatus(status) {
	case types.ChatDocumentGenerationStatusCompleted:
		return "document_complete_marker"
	case types.ChatDocumentGenerationStatusBlocked:
		return "document_generation_blocked"
	case types.ChatDocumentGenerationStatusNeedsReview:
		return "document_generation_needs_review"
	default:
		if state.MaxLowGrowthRounds > 0 && state.LowGrowthRounds >= state.MaxLowGrowthRounds {
			return "auto_continue_low_growth"
		}
		if state.MaxAutoContinueRounds > 0 && autoContinueRound >= state.MaxAutoContinueRounds {
			return "auto_continue_round_limit"
		}
		if !shouldAutoContinueChatDocument(status, finishReason, failureReason, autoContinueRound) {
			if strings.TrimSpace(finishReason) == "llm_timeout_retry_exhausted" {
				return finishReason
			}
			return firstNonEmptyString(failureReason, finishReason)
		}
		return ""
	}
}

func longDocumentContinuationReasonMessage(reason string, failureReason string, finishReason string) string {
	switch strings.TrimSpace(reason) {
	case "":
		return firstNonEmptyString(strings.TrimSpace(failureReason), strings.TrimSpace(finishReason))
	case "document_complete_marker":
		return "文档已完成"
	case types.ChatDocumentQualityIssueTerminalSectionTail:
		return "检测到文档已到收尾章节，自动续写已停止"
	case types.ChatDocumentQualityIssueDuplicateDocumentHead:
		return "检测到本轮续写重新输出了文档开头，自动续写已暂停"
	case types.ChatDocumentQualityIssueSectionNumberReset:
		return "检测到本轮续写出现章节编号回退，自动续写已暂停"
	case types.ChatDocumentQualityIssueLowNoveltyDelta:
		return "检测到本轮续写与已有内容高度重复，自动续写已暂停"
	case "document_generation_blocked":
		return "当前文档生成被阻断，自动续写已停止"
	case "document_generation_needs_review":
		return "当前文档需要人工检查，自动续写已暂停"
	case "llm_timeout_retry_exhausted":
		return "模型响应连续两轮超时，自动续写已停止"
	case "section_generation_truncated":
		return "当前章节输出被截断，自动续写已暂停"
	case "section_generation_error":
		return "当前章节生成失败，自动续写已暂停"
	case "auto_continue_round_limit":
		return "达到自动续写轮次上限"
	case "auto_continue_low_growth":
		return "连续多轮新增内容过少，请检查完整文档后继续"
	default:
		return firstNonEmptyString(strings.TrimSpace(reason), strings.TrimSpace(failureReason), strings.TrimSpace(finishReason))
	}
}

func longDocumentArtifactMetadata(artifact *types.ChatDocumentArtifact) map[string]interface{} {
	if artifact == nil {
		return nil
	}
	snapshotCharCount := artifact.SnapshotCharCount
	if snapshotCharCount == 0 && strings.TrimSpace(artifact.ContentSnapshot) != "" {
		snapshotCharCount = len([]rune(strings.TrimSpace(artifact.ContentSnapshot)))
	}
	continuationContextMode := artifact.ContinuationContextMode
	if continuationContextMode == "" {
		continuationContextMode = artifact.ContinuationMode()
	}
	qualityIssueDetails := artifact.QualityIssueDetails
	if len(qualityIssueDetails) == 0 {
		qualityIssueDetails = types.ChatDocumentQualityIssueDetails(artifact.QualityIssues)
	}
	return map[string]interface{}{
		"id":                         artifact.ID,
		"tenant_id":                  artifact.TenantID,
		"session_id":                 artifact.SessionID,
		"source_message_id":          artifact.SourceMessageID,
		"source_request_id":          artifact.SourceRequestID,
		"parent_artifact_id":         artifact.ParentArtifactID,
		"revision_no":                artifact.RevisionNo,
		"title":                      artifact.Title,
		"artifact_kind":              artifact.ArtifactKind,
		"content_type":               artifact.ContentType,
		"status":                     artifact.Status,
		"completion_status":          artifact.CompletionStatus,
		"document_generation_status": types.NormalizeChatDocumentGenerationStatus(artifact.DocumentGenerationStatus),
		"document_task_kind":         artifact.DocumentTaskKind,
		"source_title":               artifact.SourceTitle,
		"target_language":            artifact.TargetLanguage,
		"output_format":              artifact.OutputFormat,
		"operation":                  artifact.Operation,
		"snapshot_char_count":        snapshotCharCount,
		"can_continue":               artifact.CanContinue(),
		"can_inline_continue":        artifact.CanInlineContinueWithFullSnapshot(),
		"can_auto_continue":          artifact.CanAutoContinue(),
		"can_manual_continue":        artifact.CanManualContinue(),
		"can_manual_revise":          artifact.CanManualRevise(),
		"can_use_as_base":            artifact.CanUseAsBase(),
		"can_view":                   artifact.CanView(),
		"can_index":                  artifact.CanIndex(),
		"continuation_context_mode":  continuationContextMode,
		"quality_issues":             artifact.QualityIssues,
		"quality_issue_details":      qualityIssueDetails,
		"user_hint":                  artifact.UserHint,
		"structure_info":             artifact.StructureInfo,
		"created_by":                 artifact.CreatedBy,
		"created_at":                 artifact.CreatedAt,
		"updated_at":                 artifact.UpdatedAt,
	}
}

func longDocumentFinalDocumentDelivery(artifact *types.ChatDocumentArtifact) (string, string, string) {
	if artifact == nil {
		return "", "", ""
	}
	snapshot := strings.TrimSpace(artifact.ContentSnapshot)
	if snapshot == "" {
		return types.ChatDocumentFinalDocumentModeFetchArtifactSnapshot, "", artifact.ID
	}
	if len([]rune(snapshot)) <= types.ChatDocumentArtifactInlineContinuationMaxChars {
		return types.ChatDocumentFinalDocumentModeInlineSnapshot, snapshot, artifact.ID
	}
	return types.ChatDocumentFinalDocumentModeFetchArtifactSnapshot, "", artifact.ID
}

func cloneLongDocumentExtra(extra map[string]interface{}) map[string]interface{} {
	if len(extra) == 0 {
		return map[string]interface{}{}
	}
	cloned := make(map[string]interface{}, len(extra))
	for key, value := range extra {
		cloned[key] = value
	}
	return cloned
}

func languageFromContextOrDefault(ctx context.Context) string {
	if lang, ok := types.LanguageFromContext(ctx); ok {
		return lang
	}
	return types.DefaultLanguage()
}

func errorFailureReason(data *event.ErrorData) string {
	if data == nil {
		return ""
	}
	return strings.TrimSpace(data.Stage)
}

func longDocumentTargetLanguage(options *types.ChatDocumentTranslationOptions) string {
	if options == nil {
		return ""
	}
	return strings.TrimSpace(options.TargetLanguage)
}

func longDocumentOutputFormat(options *types.ChatDocumentTranslationOptions) string {
	if options == nil {
		return ""
	}
	return strings.TrimSpace(options.OutputFormat)
}

func completedArtifactID(artifact *types.ChatDocumentArtifact) string {
	if artifact == nil {
		return ""
	}
	return strings.TrimSpace(artifact.ID)
}

func completedArtifactTitle(artifact *types.ChatDocumentArtifact) string {
	if artifact == nil {
		return ""
	}
	return strings.TrimSpace(artifact.Title)
}

func completedArtifactStatus(artifact *types.ChatDocumentArtifact) string {
	if artifact == nil {
		return ""
	}
	return strings.TrimSpace(artifact.DocumentGenerationStatus)
}

func completedArtifactTaskKind(artifact *types.ChatDocumentArtifact) string {
	if artifact == nil {
		return ""
	}
	return strings.TrimSpace(artifact.DocumentTaskKind)
}

func mergeLongDocumentRunStateProgress(state *types.ChatDocumentGenerationRunState, extra map[string]interface{}) {
	if state == nil || len(extra) == 0 {
		return
	}
	persisted := longDocumentGenerationRunStateFromExtra(extra)
	if persisted.CompletedCount > 0 {
		state.CompletedCount = persisted.CompletedCount
	}
	if persisted.RemainingCount > 0 || (persisted.CompletedCount > 0 && persisted.RemainingCount == 0) {
		state.RemainingCount = persisted.RemainingCount
	}
	if persisted.NextSourceChunkStartSeq > 0 || persisted.NextSourceChunkEndSeq > 0 {
		state.NextSourceChunkStartSeq = persisted.NextSourceChunkStartSeq
		state.NextSourceChunkEndSeq = persisted.NextSourceChunkEndSeq
	}
	if persisted.NextSection != "" || (persisted.CompletedCount > 0 && persisted.RemainingCount == 0) {
		state.NextSection = persisted.NextSection
	}
	mergeTranslationProgressIntoRunState(state, extra)
	mergeOutlineProgressIntoRunState(state, extra)
}

func mergeTranslationProgressIntoRunState(state *types.ChatDocumentGenerationRunState, extra map[string]interface{}) {
	raw, ok := extra["translation_progress"].(map[string]interface{})
	if !ok || raw == nil {
		return
	}
	if completedSegments := intValueFromMap(raw, "completed_segments"); completedSegments > 0 {
		state.CompletedCount = completedSegments
	}
	if remainingSegments, exists := raw["remaining_segments"]; exists {
		state.RemainingCount = intValueInterface(remainingSegments)
	}
	nextRange, ok := raw["next_source_chunk_range"].(map[string]interface{})
	if !ok || nextRange == nil {
		if state.RemainingCount == 0 {
			state.NextSourceChunkStartSeq = 0
			state.NextSourceChunkEndSeq = 0
		}
		return
	}
	state.NextSourceChunkStartSeq = intValueFromMap(nextRange, "chunk_start_seq")
	state.NextSourceChunkEndSeq = intValueFromMap(nextRange, "chunk_end_seq")
}

func mergeOutlineProgressIntoRunState(state *types.ChatDocumentGenerationRunState, extra map[string]interface{}) {
	completedSections := stringSliceInterface(extra["completed_sections"])
	if len(completedSections) == 0 {
		return
	}
	outline, ok := extra["outline"].(map[string]interface{})
	if !ok || outline == nil {
		state.CompletedCount = max(state.CompletedCount, len(completedSections))
		return
	}
	sections, _ := outline["sections"].([]interface{})
	totalSections := len(sections)
	state.CompletedCount = max(state.CompletedCount, len(completedSections))
	if totalSections > 0 {
		state.RemainingCount = max(totalSections-len(completedSections), 0)
	}
	completedSet := make(map[string]struct{}, len(completedSections))
	for _, title := range completedSections {
		if trimmed := strings.TrimSpace(title); trimmed != "" {
			completedSet[trimmed] = struct{}{}
		}
	}
	for _, section := range sections {
		sectionMap, ok := section.(map[string]interface{})
		if !ok {
			continue
		}
		title, _ := sectionMap["title"].(string)
		title = strings.TrimSpace(title)
		if title == "" {
			continue
		}
		if _, exists := completedSet[title]; exists {
			continue
		}
		state.NextSection = title
		return
	}
	if totalSections > 0 && state.RemainingCount == 0 {
		state.NextSection = ""
	}
}

func intValueInterface(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func stringSliceInterface(value interface{}) []string {
	items, ok := value.([]interface{})
	if !ok || len(items) == 0 {
		if typed, ok := value.([]string); ok {
			return uniqueNonEmptyStrings(typed)
		}
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			continue
		}
		if trimmed := strings.TrimSpace(text); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return uniqueNonEmptyStrings(result)
}

func stringValueFromMap(values map[string]interface{}, key string) string {
	if len(values) == 0 {
		return ""
	}
	value, ok := values[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func intValueFromMap(values map[string]interface{}, key string) int {
	if len(values) == 0 {
		return 0
	}
	value, exists := values[key]
	if !exists || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	default:
		return 0
	}
}

func shouldAutoContinueChatDocument(status string, finishReason string, failureReason string, autoContinueRound int) bool {
	if types.NormalizeChatDocumentGenerationStatus(status) != types.ChatDocumentGenerationStatusContinuing {
		return false
	}
	if autoContinueRound >= 1 && isRecoverableChatDocumentContinuationFailure(finishReason, failureReason) {
		return false
	}
	if strings.TrimSpace(failureReason) != "" && !isRecoverableChatDocumentContinuationFailure(finishReason, failureReason) {
		return false
	}
	switch strings.TrimSpace(finishReason) {
	case "", "stop", "section_batch_limit", "continuation_pending", "section_generation_timeout", "section_generation_error":
		return true
	default:
		return false
	}
}

func isRecoverableChatDocumentContinuationFailure(finishReason string, failureReason string) bool {
	trimmedFinishReason := strings.TrimSpace(finishReason)
	trimmedFailureReason := strings.TrimSpace(failureReason)
	if trimmedFailureReason != "llm_timeout" {
		return false
	}
	switch trimmedFinishReason {
	case "section_generation_timeout", "section_generation_error", "llm_timeout":
		return true
	default:
		return false
	}
}

func containsChatDocumentQualityIssue(issues []string, issue string) bool {
	for _, current := range issues {
		if current == issue {
			return true
		}
	}
	return false
}
