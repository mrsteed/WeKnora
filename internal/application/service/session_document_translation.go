package service

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	chat "github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

type longDocumentTranslationRunOutline struct {
	TaskKind           string                              `json:"task_kind"`
	KnowledgeID        string                              `json:"knowledge_id"`
	KnowledgeTitle     string                              `json:"knowledge_title,omitempty"`
	SourceSnapshotHash string                              `json:"source_snapshot_hash"`
	SourceLanguage     string                              `json:"source_language,omitempty"`
	TargetLanguage     string                              `json:"target_language"`
	OutputFormat       string                              `json:"output_format"`
	PreserveStructure  bool                                `json:"preserve_structure"`
	Segments           []longDocumentTranslationRunSegment `json:"segments,omitempty"`
}

type longDocumentTranslationRunSegment struct {
	ID            string `json:"id"`
	BatchNo       int    `json:"batch_no"`
	ChunkStartSeq int    `json:"chunk_start_seq"`
	ChunkEndSeq   int    `json:"chunk_end_seq"`
}

type longDocumentTranslationContinuationState struct {
	run                *types.ChatDocumentGenerationRun
	outline            longDocumentTranslationRunOutline
	knowledge          *types.Knowledge
	translationOptions types.ChatDocumentTranslationOptions
	completedSegments  []string
	remainingSegments  []longDocumentTranslationRunSegment
}

func shouldUseLongDocumentTranslationPath(req *types.QARequest) bool {
	if req == nil {
		return false
	}
	if req.DocumentOutputMode != types.ChatDocumentOutputModeFull {
		return false
	}
	if strings.TrimSpace(req.DocumentTaskKind) != types.ChatDocumentTaskKindTranslation {
		return false
	}
	if len(req.KnowledgeIDs) != 1 || len(req.KnowledgeBaseIDs) > 0 {
		return false
	}
	if strings.TrimSpace(req.BaseArtifactID) != "" {
		return false
	}
	if strings.TrimSpace(req.GenerationRunID) != "" {
		return false
	}
	if req.AutoContinue {
		return false
	}
	if len(req.ImageURLs) > 0 || len(req.Attachments) > 0 {
		return false
	}
	return true
}

func shouldUseLongDocumentTranslationContinuationPath(req *types.QARequest) bool {
	if req == nil || !req.AutoContinue {
		return false
	}
	if strings.TrimSpace(req.DocumentTaskKind) != types.ChatDocumentTaskKindTranslation {
		return false
	}
	if strings.TrimSpace(req.GenerationRunID) == "" {
		return false
	}
	switch req.DocumentOutputMode {
	case types.ChatDocumentOutputModeDelta, types.ChatDocumentOutputModeFull:
	default:
		return false
	}
	if len(req.ImageURLs) > 0 || len(req.Attachments) > 0 {
		return false
	}
	return true
}

func (s *sessionService) runLongDocumentTranslationPath(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
	chatModel chat.Chat,
	agentConfig *types.AgentConfig,
) error {
	if req == nil || req.Session == nil || eventBus == nil || chatModel == nil {
		return fmt.Errorf("long document translation request is incomplete")
	}
	if s == nil || s.knowledgeService == nil || s.chunkService == nil {
		return fmt.Errorf("long document translation dependencies are incomplete")
	}

	knowledgeID := strings.TrimSpace(req.KnowledgeIDs[0])
	knowledge, err := s.knowledgeService.GetKnowledgeByID(ctx, knowledgeID)
	if err != nil {
		return err
	}
	if knowledge == nil {
		return fmt.Errorf("knowledge %s not found", knowledgeID)
	}

	tenantID := types.MustTenantIDFromContext(ctx)
	chunks, err := s.chunkService.GetRepository().ListChunksByKnowledgeID(ctx, tenantID, knowledgeID)
	if err != nil {
		return err
	}
	if len(chunks) == 0 {
		return s.emitLongDocumentTranslationTerminal(
			ctx,
			req,
			eventBus,
			"未找到可用于全文翻译的文档内容，请确认文件已完成解析并生成分块。",
			types.MessageCompletionStatusPartial,
			"local_knowledge_not_found",
			"local_knowledge_not_found",
			types.ChatDocumentGenerationStatusBlocked,
			nil,
			nil,
			time.Now(),
		)
	}

	plans := planLongDocumentTranslationBatches(s.cfg, chunks)
	if len(plans) == 0 {
		return s.emitLongDocumentTranslationTerminal(
			ctx,
			req,
			eventBus,
			"未找到可用于全文翻译的有效文档内容，请检查文档是否仅包含空白或重复解析噪声。",
			types.MessageCompletionStatusPartial,
			"local_knowledge_not_found",
			"local_knowledge_not_found",
			types.ChatDocumentGenerationStatusBlocked,
			nil,
			nil,
			time.Now(),
		)
	}

	startTime := time.Now()
	translationOptions := normalizeLongDocumentTranslationOptions(ctx, req)
	snapshotHash := buildLongDocumentSnapshotHash(chunks)
	runOutline := buildLongDocumentTranslationRunOutline(plans, knowledge, snapshotHash, translationOptions)
	run, err := s.createLongDocumentTranslationGenerationRun(ctx, req, chatModel, runOutline)
	if err != nil {
		return err
	}

	progress := newFullDocumentProgressReporter(ctx, req, eventBus, req.Session.ID)
	defer progress.Close()
	progress.SetSectionProgress(0, len(runOutline.Segments), "全文翻译")
	progress.UpdateStage("planning", fmt.Sprintf("正在规划全文翻译，共 %d 个片段。", len(runOutline.Segments)))

	answerEventID := generateEventID("document-translation")
	translationExtra := buildLongDocumentTranslationExtra(run, runOutline, nil)
	var finalAnswer strings.Builder
	completedSegments := make([]string, 0, len(runOutline.Segments))

	for index, plan := range plans {
		segment := runOutline.Segments[index]
		progress.SetSectionProgress(index+1, len(runOutline.Segments), fmt.Sprintf("片段 %d", index+1))
		progress.UpdateStage(
			"generating",
			fmt.Sprintf("正在翻译第 %d/%d 段（chunk %d-%d）。", index+1, len(runOutline.Segments), plan.start, plan.end),
		)

		prompt, err := buildLongDocumentTranslationBatchPrompt(ctx, translationOptions.OutputFormat, longDocumentTranslationPromptBatch{
			ChunkStartSeq: plan.start,
			ChunkEndSeq:   plan.end,
			InputSnapshot: plan.input,
		}, translationOptions)
		if err != nil {
			return err
		}

		content, callErr := translateLongDocumentBatchWithGuard(ctx, chatModel, agentConfig, prompt, plan.input)
		if callErr != nil {
			return s.finalizeLongDocumentTranslationFailure(ctx, req, eventBus, run, runOutline, completedSegments, finalAnswer.String(), translationExtra, startTime, callErr)
		}

		if finalAnswer.Len() > 0 && !strings.HasSuffix(finalAnswer.String(), "\n\n") {
			finalAnswer.WriteString("\n\n")
			if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, "\n\n", false, types.MessageCompletionStatusCompleted, "streaming"); err != nil {
				logger.Warnf(ctx, "Failed to emit translation separator chunk: %v", err)
			}
		}
		finalAnswer.WriteString(content)
		completedSegments = append(completedSegments, segment.ID)
		translationExtra = buildLongDocumentTranslationExtra(run, runOutline, completedSegments)
		if err := s.updateLongDocumentTranslationGenerationRun(ctx, run, runOutline, completedSegments, types.ChatDocumentGenerationStatusContinuing, types.MessageCompletionStatusPartial); err != nil {
			logger.Warnf(ctx, "Failed to update translation generation run progress: %v", err)
		}
		if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, content, false, types.MessageCompletionStatusCompleted, "streaming"); err != nil {
			logger.Warnf(ctx, "Failed to emit translation body chunk: %v", err)
		}
	}

	translationExtra = buildLongDocumentTranslationExtra(run, runOutline, completedSegments)
	if err := s.updateLongDocumentTranslationGenerationRun(ctx, run, runOutline, completedSegments, types.ChatDocumentGenerationStatusCompleted, types.MessageCompletionStatusCompleted); err != nil {
		logger.Warnf(ctx, "Failed to finalize translation generation run: %v", err)
	}
	return s.emitLongDocumentTranslationTerminal(
		ctx,
		req,
		eventBus,
		strings.TrimSpace(finalAnswer.String()),
		types.MessageCompletionStatusCompleted,
		"stop",
		"",
		types.ChatDocumentGenerationStatusCompleted,
		translationExtra,
		progress.AgentSteps(),
		startTime,
	)
}

func (s *sessionService) runLongDocumentTranslationContinuationPath(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
	chatModel chat.Chat,
	agentConfig *types.AgentConfig,
) error {
	if req == nil || req.Session == nil || eventBus == nil || chatModel == nil {
		return fmt.Errorf("long document translation continuation request is incomplete")
	}
	state, err := s.loadLongDocumentTranslationContinuationState(ctx, req)
	if err != nil {
		return err
	}
	if state == nil {
		return fmt.Errorf("long document translation continuation state is unavailable")
	}

	startTime := time.Now()
	translationExtra := buildLongDocumentTranslationExtra(state.run, state.outline, state.completedSegments)
	if len(state.remainingSegments) == 0 {
		if err := s.updateLongDocumentTranslationGenerationRun(ctx, state.run, state.outline, state.completedSegments, types.ChatDocumentGenerationStatusCompleted, types.MessageCompletionStatusCompleted); err != nil {
			logger.Warnf(ctx, "Failed to persist already-completed translation run: %v", err)
		}
		return s.emitLongDocumentTranslationTerminal(
			ctx,
			req,
			eventBus,
			"",
			types.MessageCompletionStatusCompleted,
			"stop",
			"",
			types.ChatDocumentGenerationStatusCompleted,
			translationExtra,
			nil,
			startTime,
		)
	}

	progress := newFullDocumentProgressReporter(ctx, req, eventBus, req.Session.ID)
	defer progress.Close()
	totalSegments := len(state.outline.Segments)
	progress.SetSectionProgress(len(state.completedSegments), totalSegments, "全文翻译")
	progress.UpdateStage("planning", fmt.Sprintf("正在恢复全文翻译，剩余 %d/%d 个片段。", len(state.remainingSegments), totalSegments))

	answerEventID := generateEventID("document-translation-continuation")
	var deltaAnswer strings.Builder
	completedSegments := append([]string(nil), state.completedSegments...)

	for offset, segment := range state.remainingSegments {
		progress.SetSectionProgress(len(completedSegments)+1, totalSegments, fmt.Sprintf("片段 %d", segment.BatchNo))
		progress.UpdateStage(
			"generating",
			fmt.Sprintf("正在翻译剩余第 %d/%d 段（chunk %d-%d）。", offset+1, len(state.remainingSegments), segment.ChunkStartSeq, segment.ChunkEndSeq),
		)

		inputSnapshot := buildLongDocumentTranslationSegmentInput(ctx, state.knowledge.ID, segment, s.chunkService.GetRepository(), state.knowledge.TenantID)
		if strings.TrimSpace(inputSnapshot) == "" {
			return s.finalizeLongDocumentTranslationFailure(
				ctx,
				req,
				eventBus,
				state.run,
				state.outline,
				completedSegments,
				deltaAnswer.String(),
				translationExtra,
				startTime,
				fmt.Errorf("translation_segment_missing_source"),
			)
		}

		prompt, promptErr := buildLongDocumentTranslationBatchPrompt(ctx, state.translationOptions.OutputFormat, longDocumentTranslationPromptBatch{
			ChunkStartSeq: segment.ChunkStartSeq,
			ChunkEndSeq:   segment.ChunkEndSeq,
			InputSnapshot: inputSnapshot,
		}, state.translationOptions)
		if promptErr != nil {
			return promptErr
		}

		content, callErr := translateLongDocumentBatchWithGuard(ctx, chatModel, agentConfig, prompt, inputSnapshot)
		if callErr != nil {
			return s.finalizeLongDocumentTranslationFailure(ctx, req, eventBus, state.run, state.outline, completedSegments, deltaAnswer.String(), translationExtra, startTime, callErr)
		}

		if deltaAnswer.Len() > 0 && !strings.HasSuffix(deltaAnswer.String(), "\n\n") {
			deltaAnswer.WriteString("\n\n")
			if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, "\n\n", false, types.MessageCompletionStatusCompleted, "streaming"); err != nil {
				logger.Warnf(ctx, "Failed to emit translation continuation separator chunk: %v", err)
			}
		}
		deltaAnswer.WriteString(content)
		completedSegments = append(completedSegments, segment.ID)
		translationExtra = buildLongDocumentTranslationExtra(state.run, state.outline, completedSegments)
		if err := s.updateLongDocumentTranslationGenerationRun(ctx, state.run, state.outline, completedSegments, types.ChatDocumentGenerationStatusContinuing, types.MessageCompletionStatusPartial); err != nil {
			logger.Warnf(ctx, "Failed to update translation continuation progress: %v", err)
		}
		if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, content, false, types.MessageCompletionStatusCompleted, "streaming"); err != nil {
			logger.Warnf(ctx, "Failed to emit translation continuation chunk: %v", err)
		}
	}

	translationExtra = buildLongDocumentTranslationExtra(state.run, state.outline, completedSegments)
	if err := s.updateLongDocumentTranslationGenerationRun(ctx, state.run, state.outline, completedSegments, types.ChatDocumentGenerationStatusCompleted, types.MessageCompletionStatusCompleted); err != nil {
		logger.Warnf(ctx, "Failed to finalize translation continuation run: %v", err)
	}
	return s.emitLongDocumentTranslationTerminal(
		ctx,
		req,
		eventBus,
		strings.TrimSpace(deltaAnswer.String()),
		types.MessageCompletionStatusCompleted,
		"stop",
		"",
		types.ChatDocumentGenerationStatusCompleted,
		translationExtra,
		progress.AgentSteps(),
		startTime,
	)
}

func normalizeLongDocumentTranslationOptions(ctx context.Context, req *types.QARequest) types.ChatDocumentTranslationOptions {
	options := types.ChatDocumentTranslationOptions{
		SourceLanguage:    "auto",
		TargetLanguage:    types.LanguageNameFromContext(ctx),
		PreserveStructure: true,
		OutputFormat:      longDocumentTranslationOutputFormatMarkdown,
	}
	if req == nil || req.TranslationOptions == nil {
		return options
	}
	if trimmed := strings.TrimSpace(req.TranslationOptions.SourceLanguage); trimmed != "" {
		options.SourceLanguage = trimmed
	}
	if trimmed := strings.TrimSpace(req.TranslationOptions.TargetLanguage); trimmed != "" {
		options.TargetLanguage = trimmed
	}
	if trimmed := strings.TrimSpace(req.TranslationOptions.OutputFormat); trimmed != "" {
		options.OutputFormat = trimmed
	}
	options.PreserveStructure = req.TranslationOptions.PreserveStructure
	if !options.PreserveStructure {
		options.PreserveStructure = true
	}
	return options
}

func buildLongDocumentTranslationRunOutline(plans []longDocumentBatchPlan, knowledge *types.Knowledge, snapshotHash string, options types.ChatDocumentTranslationOptions) longDocumentTranslationRunOutline {
	segments := make([]longDocumentTranslationRunSegment, 0, len(plans))
	for index, plan := range plans {
		segments = append(segments, longDocumentTranslationRunSegment{
			ID:            fmt.Sprintf("segment-%d", index+1),
			BatchNo:       index + 1,
			ChunkStartSeq: plan.start,
			ChunkEndSeq:   plan.end,
		})
	}
	return longDocumentTranslationRunOutline{
		TaskKind:           types.ChatDocumentTaskKindTranslation,
		KnowledgeID:        strings.TrimSpace(knowledge.ID),
		KnowledgeTitle:     firstNonEmptyString(strings.TrimSpace(knowledge.Title), strings.TrimSpace(knowledge.FileName)),
		SourceSnapshotHash: snapshotHash,
		SourceLanguage:     strings.TrimSpace(options.SourceLanguage),
		TargetLanguage:     strings.TrimSpace(options.TargetLanguage),
		OutputFormat:       firstNonEmptyString(strings.TrimSpace(options.OutputFormat), longDocumentTranslationOutputFormatMarkdown),
		PreserveStructure:  options.PreserveStructure,
		Segments:           segments,
	}
}

func (s *sessionService) createLongDocumentTranslationGenerationRun(
	ctx context.Context,
	req *types.QARequest,
	chatModel chat.Chat,
	outline longDocumentTranslationRunOutline,
) (*types.ChatDocumentGenerationRun, error) {
	if s == nil || s.generationRunRepo == nil {
		return nil, nil
	}
	userID, _ := types.UserIDFromContext(ctx)
	agentID := ""
	if req.CustomAgent != nil {
		agentID = req.CustomAgent.ID
	}
	run := &types.ChatDocumentGenerationRun{
		ID:                    uuid.NewString(),
		TenantID:              types.MustTenantIDFromContext(ctx),
		SessionID:             req.Session.ID,
		RootMessageID:         req.AssistantMessageID,
		AgentID:               firstNonEmptyString(agentID),
		OriginalQuery:         req.Query,
		DocumentTitle:         outline.KnowledgeTitle,
		OutlineJSON:           marshalGenerationRunJSON(outline),
		RuntimeFeedbackJSON:   marshalGenerationRunJSON(newDocumentGenerationRuntimeFeedback(types.ChatDocumentTaskKindTranslation, resolveLongDocumentAutoContinueMaxRounds(s.cfg), resolveLongDocumentAutoContinueMinGrowthChars(s.cfg), resolveLongDocumentAutoContinueMaxLowGrowthRounds(s.cfg))),
		CompletedSectionsJSON: marshalGenerationRunJSON([]string{}),
		Status:                types.ChatDocumentGenerationRunStatusWriting,
		AutoContinueRound:     req.AutoContinueRound,
		MaxRounds:             resolveLongDocumentAutoContinueMaxRounds(s.cfg),
		ModelID:               chatModel.GetModelID(),
		CreatedBy:             userID,
	}
	if err := s.generationRunRepo.CreateRun(ctx, run); err != nil {
		return nil, err
	}
	return run, nil
}

func (s *sessionService) updateLongDocumentTranslationGenerationRun(
	ctx context.Context,
	run *types.ChatDocumentGenerationRun,
	outline longDocumentTranslationRunOutline,
	completedSegments []string,
	documentGenerationStatus string,
	completionStatus string,
) error {
	if s == nil || s.generationRunRepo == nil || run == nil {
		return nil
	}
	remainingSegments := filterRemainingLongDocumentTranslationSegments(outline.Segments, completedSegments)
	feedback := normalizeDocumentGenerationRuntimeFeedback(applyDocumentGenerationRunState(
		unmarshalGenerationRunRuntimeFeedback(run.RuntimeFeedbackJSON),
		types.ChatDocumentGenerationRunState{
			TaskKind:              types.ChatDocumentTaskKindTranslation,
			AutoContinueRound:     run.AutoContinueRound,
			MaxAutoContinueRounds: firstPositiveInt(run.MaxRounds, resolveLongDocumentAutoContinueMaxRounds(s.cfg)),
			MinGrowthChars:        resolveLongDocumentAutoContinueMinGrowthChars(s.cfg),
			MaxLowGrowthRounds:    resolveLongDocumentAutoContinueMaxLowGrowthRounds(s.cfg),
			CompletedCount:        len(completedSegments),
			RemainingCount:        len(remainingSegments),
			NextSection:           "",
		},
	))
	if len(remainingSegments) > 0 {
		feedback.NextSourceChunkStartSeq = remainingSegments[0].ChunkStartSeq
		feedback.NextSourceChunkEndSeq = remainingSegments[0].ChunkEndSeq
	} else {
		feedback.NextSourceChunkStartSeq = 0
		feedback.NextSourceChunkEndSeq = 0
	}
	feedback.LastCompletionStatus = strings.TrimSpace(completionStatus)
	feedback.LastDocumentStatus = types.NormalizeChatDocumentGenerationStatus(documentGenerationStatus)
	run.DocumentTitle = outline.KnowledgeTitle
	run.OutlineJSON = marshalGenerationRunJSON(outline)
	run.RuntimeFeedbackJSON = marshalGenerationRunJSON(feedback)
	run.CompletedSectionsJSON = marshalGenerationRunJSON(completedSegments)
	run.Status = chatDocumentGenerationRunStatusFromOutcome(documentGenerationStatus, completionStatus)
	return s.generationRunRepo.UpdateRun(ctx, run)
}

func (s *sessionService) loadLongDocumentTranslationContinuationState(ctx context.Context, req *types.QARequest) (*longDocumentTranslationContinuationState, error) {
	if s == nil {
		return nil, fmt.Errorf("translation continuation service is unavailable")
	}
	run, err := s.loadKnowledgeGroundedGenerationRun(ctx, req.GenerationRunID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, fmt.Errorf("translation generation run %s not found", strings.TrimSpace(req.GenerationRunID))
	}
	var outline longDocumentTranslationRunOutline
	if err := json.Unmarshal(run.OutlineJSON, &outline); err != nil {
		return nil, fmt.Errorf("failed to parse translation generation run outline: %w", err)
	}
	if strings.TrimSpace(outline.KnowledgeID) == "" {
		return nil, fmt.Errorf("translation generation run is missing knowledge reference")
	}
	completedSegments, err := decodeLongDocumentTranslationCompletedSegments(run.CompletedSectionsJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to parse translation generation progress: %w", err)
	}
	remainingSegments := filterRemainingLongDocumentTranslationSegments(outline.Segments, completedSegments)
	translationOptions := buildLongDocumentTranslationRunOptions(ctx, req, outline)
	knowledge, err := s.knowledgeService.GetKnowledgeByID(ctx, outline.KnowledgeID)
	if err != nil {
		return nil, err
	}
	if knowledge == nil {
		return nil, fmt.Errorf("knowledge %s not found", outline.KnowledgeID)
	}
	if strings.TrimSpace(outline.SourceSnapshotHash) != "" {
		chunks, chunkErr := s.chunkService.GetRepository().ListChunksByKnowledgeID(ctx, types.MustTenantIDFromContext(ctx), outline.KnowledgeID)
		if chunkErr != nil {
			return nil, chunkErr
		}
		if snapshotHash := buildLongDocumentSnapshotHash(chunks); snapshotHash != strings.TrimSpace(outline.SourceSnapshotHash) {
			return nil, fmt.Errorf("translation source snapshot changed")
		}
	}
	return &longDocumentTranslationContinuationState{
		run:                run,
		outline:            outline,
		knowledge:          knowledge,
		translationOptions: translationOptions,
		completedSegments:  completedSegments,
		remainingSegments:  remainingSegments,
	}, nil
}

func buildLongDocumentTranslationRunOptions(ctx context.Context, req *types.QARequest, outline longDocumentTranslationRunOutline) types.ChatDocumentTranslationOptions {
	options := normalizeLongDocumentTranslationOptions(ctx, req)
	if strings.TrimSpace(options.SourceLanguage) == "" {
		options.SourceLanguage = strings.TrimSpace(outline.SourceLanguage)
	}
	if strings.TrimSpace(options.TargetLanguage) == "" {
		options.TargetLanguage = strings.TrimSpace(outline.TargetLanguage)
	}
	if strings.TrimSpace(options.OutputFormat) == "" {
		options.OutputFormat = strings.TrimSpace(outline.OutputFormat)
	}
	if !options.PreserveStructure {
		options.PreserveStructure = outline.PreserveStructure
	}
	if strings.TrimSpace(options.OutputFormat) == "" {
		options.OutputFormat = longDocumentTranslationOutputFormatMarkdown
	}
	return options
}

func translateLongDocumentBatchWithGuard(ctx context.Context, chatModel chat.Chat, agentConfig *types.AgentConfig, prompt string, inputSnapshot string) (string, error) {
	if chatModel == nil {
		return "", fmt.Errorf("translation_chat_model_unavailable")
	}
	const maxAttempts = 2
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		batchCtx, cancel := withDocumentGenerationCallTimeout(ctx, agentConfig, DocumentGenerationBudget{})
		thinking := false
		response, callErr := chatModel.Chat(batchCtx, []chat.Message{{Role: "user", Content: prompt}}, &chat.ChatOptions{
			Temperature: 0.2,
			Thinking:    &thinking,
		})
		cancel()
		if callErr != nil {
			lastErr = callErr
			if attempt < maxAttempts && shouldRetryLongDocumentTranslationBatch(callErr) {
				continue
			}
			return "", callErr
		}

		content := ""
		if response != nil {
			content = sanitizeGeneratedMarkdown(response.Content)
		}
		content = strings.TrimSpace(content)
		normalizedContent, validationErr := applyLongDocumentTranslationBatchQualityGate(inputSnapshot, content)
		if validationErr != nil {
			lastErr = validationErr
			if attempt < maxAttempts && shouldRetryLongDocumentTranslationBatch(validationErr) {
				continue
			}
			return "", validationErr
		}
		return normalizedContent, nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("translation_batch_empty_output")
}

func shouldRetryLongDocumentTranslationBatch(err error) bool {
	if err == nil {
		return false
	}
	if stderrors.Is(err, context.DeadlineExceeded) || stderrors.Is(err, context.Canceled) {
		return true
	}
	switch classifyDocumentEditError(err) {
	case "llm_timeout", "translation_batch_empty_output", "translation_batch_repeated_output", "translation_batch_markdown_fence_unbalanced", "translation_batch_header_footer_noise", "translation_batch_table_structure_invalid", "translation_batch_prompt_leak":
		return true
	default:
		return false
	}
}

func decodeLongDocumentTranslationCompletedSegments(raw types.JSON) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var completed []string
	if err := json.Unmarshal(raw, &completed); err != nil {
		return nil, err
	}
	return uniqueNonEmptyStrings(completed), nil
}

func filterRemainingLongDocumentTranslationSegments(segments []longDocumentTranslationRunSegment, completedSegments []string) []longDocumentTranslationRunSegment {
	if len(segments) == 0 {
		return nil
	}
	completedSet := make(map[string]struct{}, len(completedSegments))
	for _, id := range completedSegments {
		completedSet[strings.TrimSpace(id)] = struct{}{}
	}
	remaining := make([]longDocumentTranslationRunSegment, 0, len(segments))
	for _, segment := range segments {
		if _, done := completedSet[strings.TrimSpace(segment.ID)]; done {
			continue
		}
		remaining = append(remaining, segment)
	}
	return remaining
}

func buildLongDocumentTranslationSegmentInput(
	ctx context.Context,
	knowledgeID string,
	segment longDocumentTranslationRunSegment,
	repo interfaces.ChunkRepository,
	tenantID uint64,
) string {
	if repo == nil || strings.TrimSpace(knowledgeID) == "" {
		return ""
	}
	chunks, err := repo.ListChunksByKnowledgeID(ctx, tenantID, strings.TrimSpace(knowledgeID))
	if err != nil || len(chunks) == 0 {
		return ""
	}
	var builder strings.Builder
	prevEnd := -1
	for _, chunk := range chunks {
		if chunk == nil || chunk.ChunkIndex < segment.ChunkStartSeq || chunk.ChunkIndex > segment.ChunkEndSeq {
			continue
		}
		content := uniqueLongDocumentTranslationChunkContent(prevEnd, chunk)
		if strings.TrimSpace(content) == "" {
			if chunk.EndAt > prevEnd {
				prevEnd = chunk.EndAt
			}
			continue
		}
		builder.WriteString(content)
		if chunk.EndAt > prevEnd {
			prevEnd = chunk.EndAt
		}
	}
	return strings.TrimSpace(builder.String())
}

func buildLongDocumentTranslationChunkRanges(segments []longDocumentTranslationRunSegment, selected map[string]struct{}) []map[string]interface{} {
	ranges := make([]map[string]interface{}, 0, len(segments))
	for _, segment := range segments {
		_, completed := selected[strings.TrimSpace(segment.ID)]
		ranges = append(ranges, map[string]interface{}{
			"segment_id":      segment.ID,
			"chunk_start_seq": segment.ChunkStartSeq,
			"chunk_end_seq":   segment.ChunkEndSeq,
			"completed":       completed,
		})
	}
	return ranges
}

func buildLongDocumentTranslationExtra(
	run *types.ChatDocumentGenerationRun,
	outline longDocumentTranslationRunOutline,
	completedSegments []string,
) map[string]interface{} {
	feedback := documentGenerationRuntimeFeedback{}
	if run != nil {
		feedback = unmarshalGenerationRunRuntimeFeedback(run.RuntimeFeedbackJSON)
	}
	completedSet := make(map[string]struct{}, len(completedSegments))
	for _, segmentID := range completedSegments {
		if trimmed := strings.TrimSpace(segmentID); trimmed != "" {
			completedSet[trimmed] = struct{}{}
		}
	}
	remainingSegments := filterRemainingLongDocumentTranslationSegments(outline.Segments, completedSegments)
	var nextSourceRange map[string]interface{}
	if len(remainingSegments) > 0 {
		nextSourceRange = map[string]interface{}{
			"segment_id":      remainingSegments[0].ID,
			"chunk_start_seq": remainingSegments[0].ChunkStartSeq,
			"chunk_end_seq":   remainingSegments[0].ChunkEndSeq,
		}
	}
	extra := map[string]interface{}{
		"document_task_kind": types.ChatDocumentTaskKindTranslation,
		"translation_options": map[string]interface{}{
			"source_language":    outline.SourceLanguage,
			"target_language":    outline.TargetLanguage,
			"preserve_structure": outline.PreserveStructure,
			"output_format":      outline.OutputFormat,
		},
		"translation_progress": map[string]interface{}{
			"knowledge_id":            outline.KnowledgeID,
			"source_snapshot_hash":    outline.SourceSnapshotHash,
			"total_segments":          len(outline.Segments),
			"completed_segments":      len(completedSegments),
			"remaining_segments":      len(remainingSegments),
			"completed_segment_ids":   append([]string(nil), completedSegments...),
			"next_source_chunk_range": nextSourceRange,
			"source_chunk_ranges":     buildLongDocumentTranslationChunkRanges(outline.Segments, completedSet),
		},
	}
	extra = withDocumentGenerationRunStateExtra(extra, run, feedback)
	if run != nil {
		extra["generation_run_id"] = run.ID
	}
	if outline.KnowledgeTitle != "" {
		extra["document_title"] = outline.KnowledgeTitle
	}
	return extra
}

func (s *sessionService) BuildChatDocumentTerminalReplayExtra(
	ctx context.Context,
	message *types.Message,
	artifact *types.ChatDocumentArtifact,
) (map[string]interface{}, error) {
	if s == nil || s.generationRunRepo == nil || message == nil {
		return nil, nil
	}
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return nil, nil
	}
	rootArtifactID := ""
	if artifact != nil {
		rootArtifactID = strings.TrimSpace(artifact.ID)
	}
	run, err := s.generationRunRepo.GetLatestRunBySessionAndRoot(
		ctx,
		tenantID,
		strings.TrimSpace(message.SessionID),
		strings.TrimSpace(message.ID),
		rootArtifactID,
	)
	if err != nil || run == nil {
		return nil, err
	}

	var outline longDocumentTranslationRunOutline
	if err := json.Unmarshal(run.OutlineJSON, &outline); err != nil {
		return nil, nil
	}
	if strings.TrimSpace(outline.TaskKind) != types.ChatDocumentTaskKindTranslation {
		return nil, nil
	}
	completedSegments, err := decodeLongDocumentTranslationCompletedSegments(run.CompletedSectionsJSON)
	if err != nil {
		return nil, err
	}
	return buildLongDocumentTranslationExtra(run, outline, completedSegments), nil
}

func (s *sessionService) finalizeLongDocumentTranslationFailure(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
	run *types.ChatDocumentGenerationRun,
	outline longDocumentTranslationRunOutline,
	completedSegments []string,
	partialAnswer string,
	extra map[string]interface{},
	startTime time.Time,
	err error,
) error {
	hasVisibleContent := strings.TrimSpace(partialAnswer) != ""
	failureReason := classifyDocumentEditError(err)
	completionStatus := types.MessageCompletionStatusFailed
	documentGenerationStatus := types.ChatDocumentGenerationStatusBlocked
	if failureReason == types.MessageCompletionStatusCancelled {
		completionStatus = types.MessageCompletionStatusCancelled
		documentGenerationStatus = types.ChatDocumentGenerationStatusContinuing
	} else if hasVisibleContent {
		completionStatus = types.MessageCompletionStatusPartial
		documentGenerationStatus = types.ChatDocumentGenerationStatusContinuing
	}
	if hasVisibleContent && completionStatus == types.MessageCompletionStatusFailed {
		completionStatus = types.MessageCompletionStatusPartial
		documentGenerationStatus = types.ChatDocumentGenerationStatusContinuing
	}
	if updateErr := s.updateLongDocumentTranslationGenerationRun(ctx, run, outline, completedSegments, documentGenerationStatus, completionStatus); updateErr != nil {
		logger.Warnf(ctx, "Failed to persist translation failure progress: %v", updateErr)
	}
	if hasVisibleContent {
		return s.emitLongDocumentTranslationTerminal(
			ctx,
			req,
			eventBus,
			strings.TrimSpace(partialAnswer),
			completionStatus,
			failureReason,
			failureReason,
			documentGenerationStatus,
			extra,
			nil,
			startTime,
		)
	}
	if stderrors.Is(err, context.Canceled) {
		return s.emitLongDocumentTranslationTerminal(
			ctx,
			req,
			eventBus,
			"",
			types.MessageCompletionStatusCancelled,
			types.MessageCompletionStatusCancelled,
			types.MessageCompletionStatusCancelled,
			types.ChatDocumentGenerationStatusContinuing,
			extra,
			nil,
			startTime,
		)
	}
	return s.emitLongDocumentTranslationTerminal(
		ctx,
		req,
		eventBus,
		"",
		completionStatus,
		failureReason,
		failureReason,
		documentGenerationStatus,
		extra,
		nil,
		startTime,
	)
}

func (s *sessionService) emitLongDocumentTranslationTerminal(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
	finalAnswer string,
	completionStatus string,
	finishReason string,
	failureReason string,
	documentGenerationStatus string,
	extra map[string]interface{},
	agentSteps types.AgentSteps,
	startTime time.Time,
) error {
	answerEventID := generateEventID("document-translation")
	if err := emitDedicatedFullDocumentAnswerChunkWithExtra(
		ctx,
		req,
		eventBus,
		answerEventID,
		"",
		true,
		completionStatus,
		finishReason,
		documentGenerationStatus,
		extra,
	); err != nil {
		logger.Warnf(ctx, "Failed to emit translation terminal answer chunk: %v", err)
	}
	return emitDedicatedFullDocumentCompletion(
		ctx,
		req,
		eventBus,
		finalAnswer,
		completionStatus,
		finishReason,
		failureReason,
		documentGenerationStatus,
		agentSteps,
		extra,
		startTime,
	)
}
