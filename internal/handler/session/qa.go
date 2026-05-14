package session

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	appservice "github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type assistantCompletionOptions struct {
	CompletionStatus         string
	FinishReason             string
	FailureReason            string
	DocumentGenerationStatus string
	AutoContinueRound        int
	GenerationRunID          string
	KnowledgeRefs            []interface{}
	Extra                    map[string]interface{}
	AllowIndexing            bool
	AllowComplete            bool
	AgentMode                bool
	FinalAnswer              string
	AgentSteps               types.AgentSteps
	AgentDurationMs          int64
	RegisterArtifactOptions  types.RegisterChatDocumentArtifactOptions
	ArtifactObserver         func(*types.ChatDocumentArtifact)
}

func applyChatDocumentCompletionMarker(content string, artifactOptions *types.RegisterChatDocumentArtifactOptions) string {
	cleaned, completed := types.StripChatDocumentCompletionMarker(content)
	if !completed {
		return content
	}
	if artifactOptions != nil {
		artifactOptions.DocumentGenerationStatus = types.ChatDocumentGenerationStatusCompleted
	}
	return cleaned
}

func defaultAssistantCompletionOptions() assistantCompletionOptions {
	return assistantCompletionOptions{
		CompletionStatus: "completed",
		FinishReason:     "stop",
		AllowIndexing:    true,
		AllowComplete:    true,
	}
}

func agentAssistantCompletionOptions() assistantCompletionOptions {
	options := defaultAssistantCompletionOptions()
	options.AgentMode = true
	return options
}

func completionOptionsFromFinalAnswer(data event.AgentFinalAnswerData) assistantCompletionOptions {
	options := defaultAssistantCompletionOptions()
	if data.CompletionStatus != "" {
		options.CompletionStatus = data.CompletionStatus
		options.AllowIndexing = data.AllowIndexing
		options.AllowComplete = data.AllowComplete
	}
	if data.FinishReason != "" {
		options.FinishReason = data.FinishReason
	}
	if data.FailureReason != "" {
		options.FailureReason = data.FailureReason
	}
	if data.DocumentGenerationStatus != "" {
		options.DocumentGenerationStatus = data.DocumentGenerationStatus
	}
	if len(data.Extra) > 0 {
		options.Extra = make(map[string]interface{}, len(data.Extra))
		for key, value := range data.Extra {
			options.Extra[key] = value
		}
		if generationRunID, ok := data.Extra["generation_run_id"].(string); ok {
			options.GenerationRunID = strings.TrimSpace(generationRunID)
		}
	}
	return options
}

func completionOptionsFromComplete(data event.AgentCompleteData) assistantCompletionOptions {
	options := agentAssistantCompletionOptions()
	if data.CompletionStatus != "" {
		options.CompletionStatus = data.CompletionStatus
		options.AllowIndexing = data.AllowIndexing
		options.AllowComplete = data.AllowComplete
	}
	if data.FinishReason != "" {
		options.FinishReason = data.FinishReason
	}
	if data.FailureReason != "" {
		options.FailureReason = data.FailureReason
	}
	if data.DocumentGenerationStatus != "" {
		options.DocumentGenerationStatus = data.DocumentGenerationStatus
	}
	if len(data.KnowledgeRefs) > 0 {
		options.KnowledgeRefs = append([]interface{}(nil), data.KnowledgeRefs...)
	}
	if len(data.Extra) > 0 {
		options.Extra = make(map[string]interface{}, len(data.Extra))
		for key, value := range data.Extra {
			options.Extra[key] = value
		}
		if generationRunID, ok := data.Extra["generation_run_id"].(string); ok {
			options.GenerationRunID = strings.TrimSpace(generationRunID)
		}
	}
	if strings.TrimSpace(data.FinalAnswer) != "" {
		options.FinalAnswer = data.FinalAnswer
	}
	if len(data.AgentSteps) > 0 {
		options.AgentSteps = append(types.AgentSteps(nil), data.AgentSteps...)
	}
	if data.TotalDurationMs > 0 {
		options.AgentDurationMs = data.TotalDurationMs
	}
	return options
}

type generationRunArtifactBinder interface {
	BindKnowledgeGroundedGenerationRunArtifact(ctx context.Context, runID string, artifact *types.ChatDocumentArtifact) error
}

type generationRunStateRecorder interface {
	RecordChatDocumentGenerationRunState(ctx context.Context, runID string, update types.ChatDocumentGenerationRunState) (*types.ChatDocumentGenerationRunState, error)
}

type longDocumentTaskDispatcher interface {
	DispatchLongDocumentTask(ctx context.Context, req *types.QARequest, mode string) (bool, error)
}

func populateRegisterArtifactOptionsFromCompletion(options assistantCompletionOptions, artifactOptions *types.RegisterChatDocumentArtifactOptions) {
	if artifactOptions == nil {
		return
	}
	if strings.TrimSpace(artifactOptions.GenerationRunID) == "" {
		artifactOptions.GenerationRunID = strings.TrimSpace(options.GenerationRunID)
	}
	if len(artifactOptions.EvidenceRefs) == 0 {
		artifactOptions.EvidenceRefs = types.NormalizeChatDocumentEvidenceRefs(options.Extra["evidence_refs"])
	}
	if !artifactOptions.LocalKnowledgeUsed {
		if used, ok := options.Extra["local_knowledge_used"].(bool); ok {
			artifactOptions.LocalKnowledgeUsed = used
		}
	}
	completionIssues := completionQualityIssuesFromExtra(options.Extra)
	if len(completionIssues) > 0 {
		artifactOptions.QualityIssues = mergeCompletionQualityIssues(artifactOptions.QualityIssues, completionIssues)
	}
	if len(artifactOptions.EvidenceRefs) > 0 {
		artifactOptions.LocalKnowledgeUsed = true
	}
	if strings.TrimSpace(artifactOptions.DocumentTaskKind) == "" {
		if taskKind, ok := options.Extra["document_task_kind"].(string); ok {
			artifactOptions.DocumentTaskKind = strings.TrimSpace(taskKind)
		}
	}
	if translationOptions, ok := options.Extra["translation_options"].(map[string]interface{}); ok {
		if strings.TrimSpace(artifactOptions.TargetLanguage) == "" {
			if value, ok := translationOptions["target_language"].(string); ok {
				artifactOptions.TargetLanguage = strings.TrimSpace(value)
			}
		}
		if strings.TrimSpace(artifactOptions.TranslationOutputFormat) == "" {
			if value, ok := translationOptions["output_format"].(string); ok {
				artifactOptions.TranslationOutputFormat = strings.TrimSpace(value)
			}
		}
	}
	if strings.TrimSpace(artifactOptions.SourceTitle) == "" {
		if documentTitle, ok := options.Extra["document_title"].(string); ok {
			artifactOptions.SourceTitle = strings.TrimSpace(documentTitle)
		}
	}
}

func qaModeToLongDocumentExecutionMode(mode qaMode) string {
	if mode == qaModeAgent {
		return types.LongDocumentExecutionModeAgentQA
	}
	return types.LongDocumentExecutionModeKnowledgeQA
}

func mergeCompletionQualityIssues(existing []string, incoming []string) []string {
	if len(existing) == 0 && len(incoming) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	merged := make([]string, 0, len(existing)+len(incoming))
	for _, group := range [][]string{existing, incoming} {
		for _, issue := range group {
			trimmed := strings.TrimSpace(issue)
			if trimmed == "" {
				continue
			}
			if _, exists := seen[trimmed]; exists {
				continue
			}
			seen[trimmed] = struct{}{}
			merged = append(merged, trimmed)
		}
	}
	return merged
}

func completionQualityIssuesFromExtra(extra map[string]interface{}) []string {
	if len(extra) == 0 {
		return nil
	}
	rawIssues, ok := extra["quality_issues"]
	if !ok || rawIssues == nil {
		return nil
	}
	issues := make([]string, 0, 4)
	switch typed := rawIssues.(type) {
	case []string:
		for _, issue := range typed {
			if strings.TrimSpace(issue) != "" {
				issues = append(issues, strings.TrimSpace(issue))
			}
		}
	case []interface{}:
		for _, item := range typed {
			text, ok := item.(string)
			if !ok || strings.TrimSpace(text) == "" {
				continue
			}
			issues = append(issues, strings.TrimSpace(text))
		}
	}
	if len(issues) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(issues))
	result := make([]string, 0, len(issues))
	for _, issue := range issues {
		if _, exists := seen[issue]; exists {
			continue
		}
		seen[issue] = struct{}{}
		result = append(result, issue)
	}
	return result
}

func buildMessageIndexOptions(options assistantCompletionOptions, artifact *types.ChatDocumentArtifact) interfaces.MessageIndexOptions {
	indexOptions := interfaces.MessageIndexOptions{
		CompletionStatus: options.CompletionStatus,
		FinishReason:     options.FinishReason,
		AllowIndexing:    options.AllowIndexing,
	}

	artifactOptions := options.RegisterArtifactOptions
	outline := fullDocumentOutlineMetadataFromExtra(options.Extra)
	documentTitle := strings.TrimSpace(outline.Title)
	if documentTitle == "" && artifact != nil {
		documentTitle = strings.TrimSpace(artifact.Title)
	}
	documentSections := append([]string(nil), outline.Sections...)
	rawDocumentGenerationStatus := firstNonEmptyCompletionString(
		options.DocumentGenerationStatus,
		artifactOptions.DocumentGenerationStatus,
	)
	if rawDocumentGenerationStatus == "" && artifact != nil {
		rawDocumentGenerationStatus = strings.TrimSpace(artifact.DocumentGenerationStatus)
	}
	documentGenerationStatus := ""
	if strings.TrimSpace(rawDocumentGenerationStatus) != "" {
		documentGenerationStatus = types.NormalizeChatDocumentGenerationStatus(rawDocumentGenerationStatus)
	}
	artifactID := ""
	if artifact != nil {
		artifactID = strings.TrimSpace(artifact.ID)
	}

	shouldIndexLongDocument := artifactOptions.UseLongDocument || strings.TrimSpace(artifactOptions.OutputMode) == types.ChatDocumentOutputModeFull
	if !shouldIndexLongDocument && documentGenerationStatus != "" {
		shouldIndexLongDocument = artifactOptions.UseLongDocument || strings.TrimSpace(artifactOptions.GenerationRunID) != ""
	}
	if !shouldIndexLongDocument && len(documentSections) > 0 {
		shouldIndexLongDocument = artifactOptions.UseLongDocument || strings.TrimSpace(artifactOptions.OutputMode) == types.ChatDocumentOutputModeFull
	}
	if shouldIndexLongDocument {
		indexOptions.TaskKind = "long_document"
		indexOptions.DocumentGenerationStatus = documentGenerationStatus
		indexOptions.ArtifactID = artifactID
		indexOptions.DocumentTitle = documentTitle
		indexOptions.DocumentSections = documentSections
	}

	return indexOptions
}

func shouldCreateArtifactForQARequest(reqCtx *qaRequestContext) bool {
	if reqCtx == nil {
		return false
	}
	if reqCtx.routeDecisionApplied && reqCtx.routeDecision != nil {
		return reqCtx.routeDecision.NeedArtifact
	}
	if strings.TrimSpace(reqCtx.documentOutputMode) == types.ChatDocumentOutputModeFull {
		return true
	}
	switch reqCtx.documentIntent {
	case types.ChatDocumentIntentContinue, types.ChatDocumentIntentRevise, types.ChatDocumentIntentRegenerate:
		return true
	}
	switch reqCtx.documentOperation {
	case types.ChatDocumentOperationContinue, types.ChatDocumentOperationRevise, types.ChatDocumentOperationRegenerate:
		return true
	}
	return reqCtx.baseArtifact != nil || strings.TrimSpace(reqCtx.generationRunID) != ""
}

func shouldTreatQARequestAsLongDocument(reqCtx *qaRequestContext) bool {
	if reqCtx == nil {
		return false
	}
	if reqCtx.routeDecisionApplied && reqCtx.routeDecision != nil {
		switch reqCtx.routeDecision.Kind {
		case types.ChatRouteShortDocument:
			return false
		case types.ChatRouteDocumentEdit, types.ChatRouteKnowledgeGroundedContinue, types.ChatRouteFullDocument, types.ChatRouteKnowledgeGroundedFullDoc:
			return true
		}
		if reqCtx.routeDecision.UseLongDocument {
			return true
		}
	}
	if strings.TrimSpace(reqCtx.documentOutputMode) == types.ChatDocumentOutputModeFull {
		return true
	}
	if strings.TrimSpace(reqCtx.generationRunID) != "" || reqCtx.autoContinue {
		return true
	}
	switch reqCtx.documentIntent {
	case types.ChatDocumentIntentContinue, types.ChatDocumentIntentRevise, types.ChatDocumentIntentRegenerate:
		return reqCtx.baseArtifact != nil
	}
	switch reqCtx.documentOperation {
	case types.ChatDocumentOperationContinue, types.ChatDocumentOperationRevise, types.ChatDocumentOperationRegenerate:
		return reqCtx.baseArtifact != nil
	}
	return false
}

func normalizeAssistantCompletionRetryBudgetOutcome(options assistantCompletionOptions) assistantCompletionOptions {
	options.FinishReason, options.FailureReason = normalizeChatDocumentRetryBudgetOutcome(options.FinishReason, options.FailureReason, options.AutoContinueRound)
	return options
}

type fullDocumentOutlineMetadata struct {
	Title    string
	Sections []string
}

func firstNonEmptyCompletionString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func fullDocumentOutlineMetadataFromExtra(extra map[string]interface{}) fullDocumentOutlineMetadata {
	if len(extra) == 0 {
		return fullDocumentOutlineMetadata{}
	}
	rawOutline, ok := extra["outline"].(map[string]interface{})
	if !ok || rawOutline == nil {
		return fullDocumentOutlineMetadata{}
	}
	metadata := fullDocumentOutlineMetadata{}
	if title, ok := rawOutline["title"].(string); ok {
		metadata.Title = strings.TrimSpace(title)
	}
	if rawSections, ok := rawOutline["sections"].([]string); ok {
		metadata.Sections = append([]string(nil), rawSections...)
		return metadata
	}
	if rawSections, ok := rawOutline["sections"].([]map[string]interface{}); ok {
		metadata.Sections = make([]string, 0, len(rawSections))
		for _, rawSection := range rawSections {
			if trimmed := strings.TrimSpace(fullDocumentOutlineSectionTitleFromRaw(rawSection)); trimmed != "" {
				metadata.Sections = append(metadata.Sections, trimmed)
			}
		}
		return metadata
	}
	if rawSections, ok := rawOutline["sections"].([]interface{}); ok {
		metadata.Sections = make([]string, 0, len(rawSections))
		for _, rawSection := range rawSections {
			section := fullDocumentOutlineSectionTitleFromRaw(rawSection)
			if trimmed := strings.TrimSpace(section); trimmed != "" {
				metadata.Sections = append(metadata.Sections, trimmed)
			}
		}
	}
	return metadata
}

func fullDocumentOutlineSectionTitleFromRaw(rawSection interface{}) string {
	switch section := rawSection.(type) {
	case string:
		return normalizeFullDocumentSectionTitle(section)
	case map[string]interface{}:
		if title, ok := section["title"].(string); ok && strings.TrimSpace(title) != "" {
			return normalizeFullDocumentSectionTitle(title)
		}
		if heading, ok := section["heading"].(string); ok && strings.TrimSpace(heading) != "" {
			return normalizeFullDocumentSectionTitle(heading)
		}
	case map[string]string:
		if title := strings.TrimSpace(section["title"]); title != "" {
			return normalizeFullDocumentSectionTitle(title)
		}
		if heading := strings.TrimSpace(section["heading"]); heading != "" {
			return normalizeFullDocumentSectionTitle(heading)
		}
	}
	return ""
}

func normalizeFullDocumentSectionTitle(section string) string {
	trimmed := strings.TrimSpace(section)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "第") {
		remainder := strings.TrimSpace(strings.TrimPrefix(trimmed, "第"))
		if chapterIndex := strings.Index(remainder, "章"); chapterIndex > 0 {
			if _, err := strconv.Atoi(strings.TrimSpace(remainder[:chapterIndex])); err == nil {
				title := strings.TrimSpace(remainder[chapterIndex+len("章"):])
				if title != "" {
					return title
				}
			}
		}
	}
	fields := strings.Fields(trimmed)
	if len(fields) >= 2 {
		candidate := strings.TrimSuffix(fields[0], ".")
		if _, err := strconv.Atoi(candidate); err == nil {
			title := strings.TrimSpace(strings.Join(fields[1:], " "))
			if title != "" {
				return title
			}
		}
	}
	return trimmed
}

func extractCompletedFullDocumentOutline(content string) fullDocumentOutlineMetadata {
	content = strings.ReplaceAll(strings.TrimSpace(content), "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	metadata := fullDocumentOutlineMetadata{Sections: make([]string, 0, 8)}
	seenSections := make(map[string]struct{}, 8)
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "# ") && metadata.Title == "":
			metadata.Title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
		case strings.HasPrefix(line, "## "):
			section := normalizeFullDocumentSectionTitle(strings.TrimSpace(strings.TrimPrefix(line, "## ")))
			if section == "" {
				continue
			}
			if _, exists := seenSections[section]; exists {
				continue
			}
			seenSections[section] = struct{}{}
			metadata.Sections = append(metadata.Sections, section)
		}
	}
	return metadata
}

func missingCompletedFullDocumentSections(expected []string, actual []string) []string {
	if len(expected) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(actual))
	for _, section := range actual {
		if trimmed := normalizeFullDocumentSectionTitle(section); trimmed != "" {
			seen[trimmed] = struct{}{}
		}
	}
	missing := make([]string, 0)
	for _, section := range expected {
		trimmed := normalizeFullDocumentSectionTitle(section)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		missing = append(missing, trimmed)
	}
	return missing
}

func invalidCompletedFullDocumentFailure(content string, options assistantCompletionOptions) (string, string) {
	if options.CompletionStatus != types.MessageCompletionStatusCompleted || !options.AllowComplete {
		return "", ""
	}
	if strings.TrimSpace(options.RegisterArtifactOptions.OutputMode) != types.ChatDocumentOutputModeFull {
		return "", ""
	}
	documentGenerationStatus := firstNonEmptyCompletionString(options.DocumentGenerationStatus, options.RegisterArtifactOptions.DocumentGenerationStatus)
	if normalizedStatus := types.NormalizeChatDocumentGenerationStatus(documentGenerationStatus); normalizedStatus == types.ChatDocumentGenerationStatusNeedsReview {
		return "", ""
	} else if normalizedStatus != types.ChatDocumentGenerationStatusCompleted {
		return firstNonEmptyCompletionString(options.FailureReason, options.FinishReason, "outline_or_section_incomplete"), normalizedStatus
	}
	outline := fullDocumentOutlineMetadataFromExtra(options.Extra)
	if strings.TrimSpace(outline.Title) == "" && len(outline.Sections) == 0 {
		return "", ""
	}
	if strings.TrimSpace(outline.Title) == "" || strings.Contains(outline.Title, "##") || len(outline.Sections) == 0 {
		return "outline_parse_failed", types.ChatDocumentGenerationStatusBlocked
	}
	rendered := extractCompletedFullDocumentOutline(content)
	if strings.TrimSpace(rendered.Title) == "" || strings.Contains(rendered.Title, "##") {
		return "outline_or_section_incomplete", types.ChatDocumentGenerationStatusContinuing
	}
	if len(missingCompletedFullDocumentSections(outline.Sections, rendered.Sections)) > 0 {
		return "outline_or_section_incomplete", types.ChatDocumentGenerationStatusContinuing
	}
	return "", ""
}

func markAgentCompletion(options assistantCompletionOptions) assistantCompletionOptions {
	options.AgentMode = true
	return options
}

func completionOptionsFromError(data event.ErrorData) assistantCompletionOptions {
	failureReason := data.Stage
	if failureReason == "" {
		failureReason = "error"
	}

	return assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusFailed,
		FinishReason:     "error",
		FailureReason:    failureReason,
		AllowIndexing:    false,
		AllowComplete:    false,
	}
}

func shouldUseAgentFinalAnswerContent(currentContent string, options assistantCompletionOptions) bool {
	finalAnswer := strings.TrimSpace(options.FinalAnswer)
	if finalAnswer == "" {
		return false
	}

	trimmedCurrent := strings.TrimSpace(currentContent)
	if trimmedCurrent == "" {
		return true
	}

	if options.FinishReason == "fallback_stop" {
		return true
	}

	if options.CompletionStatus == types.MessageCompletionStatusFailed || options.CompletionStatus == types.MessageCompletionStatusCancelled {
		return false
	}

	if strings.TrimSpace(options.RegisterArtifactOptions.OutputMode) == types.ChatDocumentOutputModeFull {
		return true
	}

	return len([]rune(finalAnswer)) > len([]rune(trimmedCurrent))
}

func messageCompletionStatusPriority(status string) int {
	switch status {
	case types.MessageCompletionStatusCancelled:
		return 4
	case types.MessageCompletionStatusFailed:
		return 3
	case types.MessageCompletionStatusPartial:
		return 2
	case types.MessageCompletionStatusCompleted:
		return 1
	default:
		return 0
	}
}

func shouldPreserveExistingAssistantCompletion(existing *types.Message, nextStatus string) bool {
	if existing == nil {
		return false
	}
	existingStatus := existing.CompletionStatusOrLegacy()
	if messageCompletionStatusPriority(existingStatus) == 0 {
		return false
	}
	return messageCompletionStatusPriority(existingStatus) >= messageCompletionStatusPriority(nextStatus)
}

func hasHigherPriorityAssistantCompletion(existing *types.Message, nextStatus string) bool {
	if existing == nil {
		return false
	}
	existingStatus := existing.CompletionStatusOrLegacy()
	if messageCompletionStatusPriority(existingStatus) == 0 {
		return false
	}
	return messageCompletionStatusPriority(existingStatus) > messageCompletionStatusPriority(nextStatus)
}

func emitAssistantCompleteEvent(eventBus *event.EventBus, sessionID string, message *types.Message, options assistantCompletionOptions, artifact *types.ChatDocumentArtifact) {
	options = normalizeAssistantCompletionRetryBudgetOutcome(options)
	completeData := event.AgentCompleteData{
		FinalAnswer:      message.Content,
		CompletionStatus: options.CompletionStatus,
		FinishReason:     options.FinishReason,
		IsPartial:        options.CompletionStatus == types.MessageCompletionStatusPartial,
		AllowIndexing:    options.AllowIndexing,
		AllowComplete:    options.AllowComplete,
		FailureReason:    options.FailureReason,
		KnowledgeRefs:    append([]interface{}(nil), options.KnowledgeRefs...),
		AgentSteps:       message.AgentSteps,
		TotalDurationMs:  message.AgentDurationMs,
		Extra:            cloneCompleteExtra(options.Extra),
	}
	if options.DocumentGenerationStatus != "" {
		completeData.DocumentGenerationStatus = types.NormalizeChatDocumentGenerationStatus(options.DocumentGenerationStatus)
		completeData.AutoContinueNext = chatDocumentAutoContinueNext(completeData.DocumentGenerationStatus, options.FinishReason, options.FailureReason, options.AutoContinueRound)
		completeData.AutoContinueReason = chatDocumentAutoContinueReason(completeData.DocumentGenerationStatus, options.FinishReason, options.FailureReason, options.AutoContinueRound)
		completeData.AutoContinueReasonMessage = chatDocumentContinuationReasonMessage(completeData.AutoContinueReason, completeData.FailureReason, completeData.FinishReason)
		applyChatDocumentContinuationDecision(&completeData, nil, options.AutoContinueRound)
	}
	if artifact != nil {
		completeData.FinalDocumentMode, completeData.FinalDocument, completeData.FinalDocumentArtifactID = buildFinalDocumentDelivery(artifact)
		completeData.DocumentGenerationStatus = types.NormalizeChatDocumentGenerationStatus(artifact.DocumentGenerationStatus)
		completeData.AutoContinueNext = chatDocumentAutoContinueNext(completeData.DocumentGenerationStatus, options.FinishReason, options.FailureReason, options.AutoContinueRound)
		completeData.AutoContinueReason = chatDocumentAutoContinueReason(completeData.DocumentGenerationStatus, options.FinishReason, options.FailureReason, options.AutoContinueRound, artifact)
		completeData.AutoContinueReasonMessage = chatDocumentContinuationReasonMessage(completeData.AutoContinueReason, completeData.FailureReason, completeData.FinishReason)
		if completeData.Extra == nil {
			completeData.Extra = map[string]interface{}{}
		}
		completeData.Extra["chat_document_artifact"] = chatDocumentArtifactMetadata(artifact)
		applyChatDocumentContinuationDecision(&completeData, artifact, options.AutoContinueRound)
	}

	eventBus.Emit(context.Background(), event.Event{
		Type:      event.EventAgentComplete,
		SessionID: sessionID,
		Data:      completeData,
	})
}

func cloneCompleteExtra(extra map[string]interface{}) map[string]interface{} {
	if len(extra) == 0 {
		return nil
	}
	cloned := make(map[string]interface{}, len(extra))
	for key, value := range extra {
		cloned[key] = value
	}
	return cloned
}

func mergeCompletionExtra(base map[string]interface{}, overlay map[string]interface{}) map[string]interface{} {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	merged := cloneCompleteExtra(base)
	if merged == nil {
		merged = make(map[string]interface{}, len(overlay))
	}
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
}

func chatDocumentAutoContinueNext(status string, finishReason string, failureReason string, autoContinueRound int) *bool {
	next := shouldAutoContinueChatDocument(status, finishReason, failureReason, autoContinueRound)
	return &next
}

const (
	chatDocumentNextActionContinueAuto   = "continue_auto"
	chatDocumentNextActionWaitUserReview = "wait_user_review"
	chatDocumentNextActionBlocked        = "blocked"
	chatDocumentNextActionDone           = "done"
	chatDocumentNextActionManualRetry    = "manual_retry"
	chatDocumentAutoContinuePrompt       = "以当前文档为基准，继续剩余内容输出"
)

func applyChatDocumentContinuationDecision(data *event.AgentCompleteData, artifact *types.ChatDocumentArtifact, autoContinueRound int) {
	if data == nil || strings.TrimSpace(data.DocumentGenerationStatus) == "" {
		return
	}
	decision := buildChatDocumentContinuationDecision(data.DocumentGenerationStatus, data.FinishReason, data.FailureReason, autoContinueRound, artifact, data.Extra)
	data.AutoContinueNext = boolPointer(decision.canAutoContinue)
	data.AutoContinueReason = decision.reason
	data.AutoContinueReasonMessage = decision.reasonMessage
	data.NextAction = decision.action
	data.NextReason = decision.reason
	data.NextReasonMessage = decision.reasonMessage
	data.CanAutoContinue = &decision.canAutoContinue
	data.RecommendedRequest = buildChatDocumentRecommendedRequest(decision, artifact, data.Extra, autoContinueRound)
}

func boolPointer(value bool) *bool {
	result := value
	return &result
}

type chatDocumentContinuationDecision struct {
	action          string
	reason          string
	reasonMessage   string
	canAutoContinue bool
}

func buildChatDocumentContinuationDecision(status string, finishReason string, failureReason string, autoContinueRound int, artifact *types.ChatDocumentArtifact, extra map[string]interface{}) chatDocumentContinuationDecision {
	normalizedStatus := types.NormalizeChatDocumentGenerationStatus(status)
	state := chatDocumentGenerationRunStateFromExtra(extra)
	reason := chatDocumentAutoContinueReasonWithState(normalizedStatus, finishReason, failureReason, autoContinueRound, state, artifact)
	canContinue := canAutoContinueChatDocumentWithState(normalizedStatus, finishReason, failureReason, autoContinueRound, state)
	reasonMessage := chatDocumentContinuationReasonMessage(reason, failureReason, finishReason)

	switch normalizedStatus {
	case types.ChatDocumentGenerationStatusCompleted:
		return chatDocumentContinuationDecision{action: chatDocumentNextActionDone, reason: firstNonEmptyString(reason, "document_complete_marker"), reasonMessage: reasonMessage}
	case types.ChatDocumentGenerationStatusBlocked:
		return chatDocumentContinuationDecision{action: chatDocumentNextActionBlocked, reason: firstNonEmptyString(reason, "document_generation_blocked"), reasonMessage: reasonMessage}
	case types.ChatDocumentGenerationStatusNeedsReview:
		return chatDocumentContinuationDecision{action: chatDocumentNextActionWaitUserReview, reason: firstNonEmptyString(reason, "document_generation_needs_review"), reasonMessage: reasonMessage}
	case types.ChatDocumentGenerationStatusContinuing:
		if canContinue {
			return chatDocumentContinuationDecision{action: chatDocumentNextActionContinueAuto, reason: reason, reasonMessage: reasonMessage, canAutoContinue: true}
		}
		return chatDocumentContinuationDecision{action: chatDocumentNextActionManualRetry, reason: firstNonEmptyString(reason, failureReason, finishReason), reasonMessage: reasonMessage}
	default:
		if canContinue {
			return chatDocumentContinuationDecision{action: chatDocumentNextActionContinueAuto, reason: reason, reasonMessage: reasonMessage, canAutoContinue: true}
		}
		return chatDocumentContinuationDecision{action: chatDocumentNextActionManualRetry, reason: firstNonEmptyString(reason, failureReason, finishReason), reasonMessage: reasonMessage}
	}
}

func buildChatDocumentRecommendedRequest(decision chatDocumentContinuationDecision, artifact *types.ChatDocumentArtifact, extra map[string]interface{}, autoContinueRound int) map[string]interface{} {
	if decision.action != chatDocumentNextActionContinueAuto {
		return nil
	}
	request := map[string]interface{}{
		"query":                   chatDocumentAutoContinuePrompt,
		"intent_hint":             types.ChatDocumentIntentContinue,
		"auto_continue":           true,
		"auto_continue_prompt":    chatDocumentAutoContinuePrompt,
		"auto_continue_round":     autoContinueRound + 1,
		"document_target_heading": nil,
		"document_merge_mode":     nil,
	}
	if artifact != nil && strings.TrimSpace(artifact.ID) != "" {
		request["base_artifact_id"] = strings.TrimSpace(artifact.ID)
		request["document_output_mode"] = types.ChatDocumentOutputModeDelta
	}
	if runID := chatDocumentGenerationRunIDFromExtra(extra); runID != "" {
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

func chatDocumentGenerationRunIDFromExtra(extra map[string]interface{}) string {
	if len(extra) == 0 {
		return ""
	}
	if runID, ok := extra["generation_run_id"].(string); ok {
		return strings.TrimSpace(runID)
	}
	return ""
}

func chatDocumentGenerationRunStateFromExtra(extra map[string]interface{}) types.ChatDocumentGenerationRunState {
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

func normalizeChatDocumentRetryBudgetOutcome(finishReason string, failureReason string, autoContinueRound int) (string, string) {
	if autoContinueRound < 1 {
		return finishReason, failureReason
	}
	if !isRecoverableChatDocumentContinuationFailure(finishReason, failureReason) {
		return finishReason, failureReason
	}
	return "llm_timeout_retry_exhausted", failureReason
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

func canAutoContinueChatDocumentWithState(status string, finishReason string, failureReason string, autoContinueRound int, state types.ChatDocumentGenerationRunState) bool {
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

func chatDocumentAutoContinueReason(status string, finishReason string, failureReason string, autoContinueRound int, artifact ...*types.ChatDocumentArtifact) string {
	return chatDocumentAutoContinueReasonWithState(status, finishReason, failureReason, autoContinueRound, types.ChatDocumentGenerationRunState{}, artifact...)
}

func chatDocumentAutoContinueReasonWithState(status string, finishReason string, failureReason string, autoContinueRound int, state types.ChatDocumentGenerationRunState, artifact ...*types.ChatDocumentArtifact) string {
	if len(artifact) > 0 && artifact[0] != nil {
		issues := artifact[0].QualityIssues
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

func chatDocumentContinuationReasonMessage(reason string, failureReason string, finishReason string) string {
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

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func containsChatDocumentQualityIssue(issues []string, issue string) bool {
	for _, current := range issues {
		if current == issue {
			return true
		}
	}
	return false
}

func addChatDocumentGenerationPayload(data map[string]interface{}, artifact *types.ChatDocumentArtifact) {
	if data == nil || artifact == nil {
		return
	}
	status := types.NormalizeChatDocumentGenerationStatus(artifact.DocumentGenerationStatus)
	finishReason, _ := data["finish_reason"].(string)
	failureReason, _ := data["failure_reason"].(string)
	data["document_generation_status"] = status
	if _, exists := data["auto_continue_next"]; !exists {
		data["auto_continue_next"] = shouldAutoContinueChatDocument(status, finishReason, failureReason, 0)
	}
	if _, exists := data["auto_continue_reason"]; !exists {
		if reason := chatDocumentAutoContinueReason(status, finishReason, failureReason, 0, artifact); reason != "" {
			data["auto_continue_reason"] = reason
		}
	} else if reason := chatDocumentAutoContinueReason(status, finishReason, failureReason, 0, artifact); reason != "" {
		data["auto_continue_reason"] = reason
	}
	if reason, _ := data["auto_continue_reason"].(string); reason != "" {
		if _, exists := data["auto_continue_reason_message"]; !exists {
			data["auto_continue_reason_message"] = chatDocumentContinuationReasonMessage(reason, failureReason, finishReason)
		}
	}
	decision := buildChatDocumentContinuationDecision(status, finishReason, failureReason, 0, artifact, data)
	data["auto_continue_next"] = decision.canAutoContinue
	if decision.reason != "" {
		data["auto_continue_reason"] = decision.reason
	}
	if decision.reasonMessage != "" {
		data["auto_continue_reason_message"] = decision.reasonMessage
	}
	if _, exists := data["next_action"]; !exists && decision.action != "" {
		data["next_action"] = decision.action
	}
	if _, exists := data["next_reason"]; !exists && decision.reason != "" {
		data["next_reason"] = decision.reason
	}
	if _, exists := data["next_reason_message"]; !exists && decision.reasonMessage != "" {
		data["next_reason_message"] = decision.reasonMessage
	}
	if _, exists := data["can_auto_continue"]; !exists {
		data["can_auto_continue"] = decision.canAutoContinue
	}
	if _, exists := data["recommended_request"]; !exists {
		if request := buildChatDocumentRecommendedRequest(decision, artifact, data, 0); len(request) > 0 {
			data["recommended_request"] = request
		}
	}
}

func buildFinalDocumentDelivery(artifact *types.ChatDocumentArtifact) (string, string, string) {
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

func chatDocumentArtifactMetadata(artifact *types.ChatDocumentArtifact) map[string]interface{} {
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
		"continuation_context_mode":  continuationContextMode,
		"quality_issues":             artifact.QualityIssues,
		"user_hint":                  artifact.UserHint,
		"structure_info":             artifact.StructureInfo,
		"created_by":                 artifact.CreatedBy,
		"created_at":                 artifact.CreatedAt,
		"updated_at":                 artifact.UpdatedAt,
	}
}

// qaRequestContext holds all the common data needed for QA requests
type qaRequestContext struct {
	ctx                   context.Context
	c                     *gin.Context
	sessionID             string
	requestID             string
	receivedAt            time.Time // Wall-clock time the handler started processing the request
	query                 string
	titleSeedQuery            string
	intentHint                string
	baseArtifactID            string
	documentIntent            string
	documentOperation         string
	documentOutputMode        string
	documentTaskKind          string
	translationOptions        *types.ChatDocumentTranslationOptions
	documentTargetHeading     string
	documentMergeMode         string
	autoContinue              bool
	generationRunID           string
	autoContinueRootID        string
	autoContinueRound         int
	autoContinuePrompt        string
	autoContinueOriginalQuery string
	baseArtifact              *types.ChatDocumentArtifact
	documentQuotedContext     string
	routeDecision             *types.ChatRouteDecision
	routeDecisionApplied      bool
	routeModelID              string
	session                   *types.Session
	customAgent               *types.CustomAgent
	assistantMessage          *types.Message
	knowledgeBaseIDs          []string
	knowledgeIDs              []string
	summaryModelID            string
	webSearchEnabled          bool
	enableMemory              bool // Whether memory feature is enabled
	mentionedItems            types.MentionedItems
	effectiveTenantID         uint64                   // when using shared agent, tenant ID for model/KB/MCP resolution; 0 = use context tenant
	images                    []ImageAttachment        // Uploaded images with analysis text
	userMessageID             string                   // Created user message ID (populated after createUserMessage)
	channel                   string                   // Source channel: "web", "api", "im", etc.
	attachments               types.MessageAttachments // Processed file attachments
}

// buildQARequest converts the qaRequestContext into a types.QARequest for service invocation.
func (rc *qaRequestContext) buildQARequest() *types.QARequest {
	imageURLs, imageDescription := extractImageURLsAndOCRText(rc.images)
	quotedContext := appendQuotedContext("", rc.documentQuotedContext)
	return &types.QARequest{
		Session:                   rc.session,
		Query:                     rc.query,
		AssistantMessageID:        rc.assistantMessage.ID,
		SummaryModelID:            rc.summaryModelID,
		CustomAgent:               rc.customAgent,
		KnowledgeBaseIDs:          rc.knowledgeBaseIDs,
		KnowledgeIDs:              rc.knowledgeIDs,
		ImageURLs:                 imageURLs,
		ImageDescription:          imageDescription,
		UserMessageID:             rc.userMessageID,
		WebSearchEnabled:          rc.webSearchEnabled,
		EnableMemory:              rc.enableMemory,
		QuotedContext:             quotedContext,
		DocumentIntent:            rc.documentIntent,
		BaseArtifactID:            rc.baseArtifactID,
		DocumentOperation:         rc.documentOperation,
		DocumentOutputMode:        rc.documentOutputMode,
		DocumentTaskKind:          rc.documentTaskKind,
		TranslationOptions:        cloneTranslationOptions(rc.translationOptions),
		DocumentTargetHeading:     rc.documentTargetHeading,
		DocumentMergeMode:         rc.documentMergeMode,
		RouteDecision:             rc.routeDecision,
		AutoContinue:              rc.autoContinue,
		GenerationRunID:           rc.generationRunID,
		AutoContinueRound:         rc.autoContinueRound,
		AutoContinuePrompt:        rc.autoContinuePrompt,
		AutoContinueOriginalQuery: rc.autoContinueOriginalQuery,
		BaseArtifact:              rc.baseArtifact,
		Attachments:               rc.attachments,
	}
}

func cloneTranslationOptions(options *types.ChatDocumentTranslationOptions) *types.ChatDocumentTranslationOptions {
	if options == nil {
		return nil
	}
	cloned := *options
	return &cloned
}

// parseQARequest parses and validates a QA request, returns the request context
func (h *Handler) parseQARequest(c *gin.Context, logPrefix string) (*qaRequestContext, *CreateKnowledgeQARequest, error) {
	receivedAt := time.Now()
	ctx := logger.CloneContext(c.Request.Context())
	requestID := secutils.SanitizeForLog(c.GetString(types.RequestIDContextKey.String()))
	logger.Infof(ctx, "[%s] TTFB:start request_id=%s received_at=%d",
		logPrefix, requestID, receivedAt.UnixMilli())

	// Get session ID from URL parameter
	sessionID := secutils.SanitizeForLog(c.Param("session_id"))
	if sessionID == "" {
		logger.Error(ctx, "Session ID is empty")
		return nil, nil, errors.NewBadRequestError(errors.ErrInvalidSessionID.Error())
	}

	// Parse request body
	var request CreateKnowledgeQARequest
	if err := c.ShouldBindJSON(&request); err != nil {
		logger.Error(ctx, "Failed to parse request data", err)
		return nil, nil, errors.NewBadRequestError(err.Error())
	}

	// Validate query content
	if request.Query == "" {
		logger.Error(ctx, "Query content is empty")
		return nil, nil, errors.NewBadRequestError("Query content cannot be empty")
	}

	// SSRF protection: strip client-supplied URL/Caption fields from image attachments.
	// The URL field must only be populated server-side by saveImageAttachments; an
	// attacker could inject internal network URLs to trigger SSRF via the LLM provider.
	for i := range request.Images {
		request.Images[i].URL = ""
		request.Images[i].Caption = ""
	}

	// Log request details
	if requestJSON, err := json.Marshal(request); err == nil {
		logger.Infof(ctx, "[%s] Request: session_id=%s, request=%s",
			logPrefix, sessionID, secutils.SanitizeForLog(secutils.CompactImageDataURLForLog(string(requestJSON))))
	}

	// Get session
	session, err := h.sessionService.GetSession(ctx, sessionID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get session, session ID: %s, error: %v", sessionID, err)
		return nil, nil, errors.NewNotFoundError("Session not found")
	}

	// Get custom agent if agent_id is provided. Backend resolves shared agent from share relation (no client-provided tenant).
	customAgent, effectiveTenantID := h.resolveAgent(ctx, c, request.AgentID)

	// Merge @mentioned items into knowledge_base_ids and knowledge_ids
	kbIDs, knowledgeIDs := mergeKnowledgeTargets(request.KnowledgeBaseIDs, request.KnowledgeIds, request.MentionedItems)

	// Log merge results for debugging
	logger.Infof(ctx, "[%s] @mention merge: request.KnowledgeBaseIDs=%v, request.MentionedItems=%d, merged kbIDs=%v, merged knowledgeIDs=%v",
		logPrefix, request.KnowledgeBaseIDs, len(request.MentionedItems), kbIDs, knowledgeIDs)

	// Process inline base64 images: decode and save to storage.
	// VLM analysis for RAG paths is deferred to the pipeline rewrite step.
	// For pure chat paths with non-vision models, VLM analysis runs here as fallback.
	if len(request.Images) > 0 {
		if customAgent == nil || !customAgent.Config.ImageUploadEnabled {
			logger.Warnf(ctx, "[%s] Image upload is not enabled for this agent, rejecting %d images", logPrefix, len(request.Images))
			return nil, nil, errors.NewBadRequestError("Image upload is not enabled for this agent")
		}
		tenantID := c.GetUint64(types.TenantIDContextKey.String())
		agentStorageProvider := customAgent.Config.ImageStorageProvider
		if err := h.saveImageAttachments(ctx, request.Images, tenantID, agentStorageProvider); err != nil {
			logger.Errorf(ctx, "[%s] Failed to save images: %v", logPrefix, err)
			return nil, nil, errors.NewBadRequestError(fmt.Sprintf("Image save failed: %v", err))
		}

		// VLM analysis is always deferred to after SSE stream is up:
		// - Agent mode: runs in async execution flow with tool_call/tool_result events
		// - Normal RAG mode: runs in the pipeline rewrite step with progress events
		// - Normal pure-chat mode: runs in the async goroutine with progress events
	}

	// Process file attachments: decode and save to storage, extract content
	var processedAttachments types.MessageAttachments
	if len(request.AttachmentUploads) > 0 {
		logger.Infof(ctx, "[%s] processing %d attachment(s)", logPrefix, len(request.AttachmentUploads))

		maxSize := secutils.GetMaxFileSize()
		for i, upload := range request.AttachmentUploads {
			if upload.FileSize > maxSize {
				return nil, nil, errors.NewBadRequestError(
					fmt.Sprintf("attachment %d exceeds size limit of %dMB", i+1, secutils.GetMaxFileSizeMB()))
			}
		}

		tenantID := c.GetUint64(types.TenantIDContextKey.String())

		// Use ASR only when the agent has audio upload enabled.
		asrModelID := ""
		if customAgent != nil && customAgent.Config.AudioUploadEnabled && customAgent.Config.ASRModelID != "" {
			asrModelID = customAgent.Config.ASRModelID
		}

		// Process all attachments concurrently.
		processedAttachments = make(types.MessageAttachments, len(request.AttachmentUploads))
		var wg sync.WaitGroup
		errChan := make(chan error, len(request.AttachmentUploads))

		for i, upload := range request.AttachmentUploads {
			wg.Add(1)
			go func(idx int, att AttachmentUpload) {
				defer wg.Done()

				data, err := DecodeBase64Attachment(att.Data)
				if err != nil {
					errChan <- fmt.Errorf("attachment %d decode failed: %w", idx+1, err)
					return
				}

				processed, err := h.attachmentProcessor.ProcessAttachment(
					ctx, data, att.FileName, att.FileSize, tenantID, asrModelID,
				)
				if err != nil {
					errChan <- fmt.Errorf("attachment %d processing failed: %w", idx+1, err)
					return
				}

				processedAttachments[idx] = *processed
			}(i, upload)
		}

		wg.Wait()
		close(errChan)

		if len(errChan) > 0 {
			err := <-errChan
			logger.Errorf(ctx, "[%s] attachment processing failed: %v", logPrefix, err)
			return nil, nil, errors.NewBadRequestError(fmt.Sprintf("attachment processing failed: %v", err))
		}

		logger.Infof(ctx, "[%s] all attachments processed", logPrefix)
	}

	// Build request context
	reqCtx := &qaRequestContext{
		ctx:                ctx,
		c:                  c,
		sessionID:          sessionID,
		requestID:          requestID,
		receivedAt:         receivedAt,
		query:              secutils.SanitizeForLog(request.Query),
		intentHint:         secutils.SanitizeForLog(request.IntentHint),
		baseArtifactID:     secutils.SanitizeForLog(request.BaseArtifactID),
		documentOutputMode: secutils.SanitizeForLog(request.DocumentOutputMode),
		documentTaskKind:          secutils.SanitizeForLog(request.DocumentTaskKind),
		translationOptions:        sanitizeTranslationOptions(request.TranslationOptions),
		documentTargetHeading:     secutils.SanitizeForLog(request.DocumentTargetHeading),
		documentMergeMode:         secutils.SanitizeForLog(request.DocumentMergeMode),
		autoContinue:              request.AutoContinue,
		generationRunID:           secutils.SanitizeForLog(request.GenerationRunID),
		autoContinueRootID:        secutils.SanitizeForLog(request.AutoContinueRootID),
		autoContinueRound:         request.AutoContinueRound,
		autoContinuePrompt:        secutils.SanitizeForLog(request.AutoContinuePrompt),
		autoContinueOriginalQuery: secutils.SanitizeForLog(request.AutoContinueOriginalQuery),
		session:                   session,
		customAgent:               customAgent,
		assistantMessage: &types.Message{
			SessionID:        sessionID,
			Role:             "assistant",
			RequestID:        c.GetString(types.RequestIDContextKey.String()),
			IsCompleted:      false,
			CompletionStatus: types.MessageCompletionStatusPending,
			Channel:          request.Channel,
		},
		knowledgeBaseIDs:  secutils.SanitizeForLogArray(kbIDs),
		knowledgeIDs:      secutils.SanitizeForLogArray(knowledgeIDs),
		summaryModelID:    secutils.SanitizeForLog(request.SummaryModelID),
		webSearchEnabled:  request.WebSearchEnabled,
		enableMemory:      request.EnableMemory,
		mentionedItems:    convertMentionedItems(request.MentionedItems),
		effectiveTenantID: effectiveTenantID,
		images:            request.Images,
		channel:           request.Channel,
		attachments:       processedAttachments,
	}

	documentContextQuery := reqCtx.query
	if reqCtx.autoContinue && strings.TrimSpace(reqCtx.autoContinueOriginalQuery) != "" {
		documentContextQuery = reqCtx.autoContinueOriginalQuery
	}
	documentPrep := h.prepareDocumentRequest(ctx, session, documentContextQuery, request.IntentHint, request.BaseArtifactID, request.DocumentOutputMode, request.DocumentTargetHeading, request.DocumentMergeMode)
	reqCtx.documentIntent = documentPrep.intent
	reqCtx.documentOperation = documentPrep.operation
	reqCtx.baseArtifact = documentPrep.baseArtifact
	reqCtx.documentQuotedContext = documentPrep.quotedContext
	reqCtx.documentOutputMode = normalizeDocumentOutputModeForRequest(request.DocumentOutputMode, reqCtx.documentIntent)
	reqCtx.documentTargetHeading = documentPrep.targetHeading
	reqCtx.documentMergeMode = documentPrep.mergeMode
	if documentPrep.baseArtifact != nil {
		reqCtx.baseArtifactID = documentPrep.baseArtifact.ID
	}
	inferNaturalLanguageFullTranslationRequest(reqCtx, &request)
	reqCtx.titleSeedQuery = h.buildSessionTitleSeed(ctx, reqCtx)
	h.detectChatRouteDecision(ctx, logPrefix, reqCtx, &request)

	return reqCtx, &request, nil
}

func sanitizeTranslationOptions(options *types.ChatDocumentTranslationOptions) *types.ChatDocumentTranslationOptions {
	if options == nil {
		return nil
	}
	return &types.ChatDocumentTranslationOptions{
		SourceLanguage:    secutils.SanitizeForLog(options.SourceLanguage),
		TargetLanguage:    secutils.SanitizeForLog(options.TargetLanguage),
		PreserveStructure: options.PreserveStructure,
		OutputFormat:      secutils.SanitizeForLog(options.OutputFormat),
	}
}

var naturalLanguageFullTranslationQueryRE = regexp.MustCompile(`(?i)((全文|整篇|全篇|整个文档|整份文档|完整文档|完整译文|文档全文|full\s*document|whole\s*document|entire\s*document).{0,24}(翻译|译成|translate))|((翻译|译成|translate).{0,24}(全文|整篇|全篇|整个文档|整份文档|完整文档|完整译文|文档全文|full\s*document|whole\s*document|entire\s*document))|((文档|markdown).{0,12}(完整|全文|整篇|全篇).{0,12}(翻译|译成))|((翻译|译成).{0,12}(完整|全文|整篇|全篇).{0,12}(文档|markdown))`)

func inferNaturalLanguageFullTranslationRequest(reqCtx *qaRequestContext, request *CreateKnowledgeQARequest) {
	if reqCtx == nil || request == nil {
		return
	}
	if strings.TrimSpace(reqCtx.documentTaskKind) != "" || strings.TrimSpace(reqCtx.documentOutputMode) != "" {
		return
	}
	if strings.TrimSpace(reqCtx.baseArtifactID) != "" || reqCtx.autoContinue {
		return
	}
	if len(reqCtx.knowledgeIDs) != 1 || len(reqCtx.knowledgeBaseIDs) > 0 {
		return
	}
	if len(reqCtx.images) > 0 || len(reqCtx.attachments) > 0 {
		return
	}
	if !naturalLanguageFullTranslationQueryRE.MatchString(strings.TrimSpace(reqCtx.query)) {
		return
	}
	reqCtx.documentTaskKind = types.ChatDocumentTaskKindTranslation
	reqCtx.documentOutputMode = types.ChatDocumentOutputModeFull
	if strings.TrimSpace(reqCtx.documentIntent) == "" {
		reqCtx.documentIntent = types.ChatDocumentIntentNormal
	}
	if strings.TrimSpace(reqCtx.documentOperation) == "" {
		reqCtx.documentOperation = types.ChatDocumentOperationCreate
	}
	request.DocumentTaskKind = types.ChatDocumentTaskKindTranslation
	request.DocumentOutputMode = types.ChatDocumentOutputModeFull
}

func buildTranslationSessionTitleSeed(query, knowledgeTitle, targetLanguage, outputFormat string) string {
	trimmedKnowledgeTitle := strings.TrimSpace(knowledgeTitle)
	if trimmedKnowledgeTitle == "" {
		return strings.TrimSpace(query)
	}
	trimmedTargetLanguage := strings.TrimSpace(targetLanguage)
	if trimmedTargetLanguage == "" {
		trimmedTargetLanguage = "目标语言"
	}
	seed := fmt.Sprintf("请将《%s》完整翻译为%s", trimmedKnowledgeTitle, trimmedTargetLanguage)
	if strings.EqualFold(strings.TrimSpace(outputFormat), "markdown") {
		seed += " Markdown"
	}
	return seed
}

func (h *Handler) buildSessionTitleSeed(ctx context.Context, reqCtx *qaRequestContext) string {
	if reqCtx == nil {
		return ""
	}
	if strings.TrimSpace(reqCtx.documentTaskKind) != types.ChatDocumentTaskKindTranslation {
		return strings.TrimSpace(reqCtx.query)
	}
	if h == nil || h.knowledgeService == nil || len(reqCtx.knowledgeIDs) != 1 {
		return strings.TrimSpace(reqCtx.query)
	}
	knowledge, err := h.knowledgeService.GetKnowledgeByID(ctx, reqCtx.knowledgeIDs[0])
	if err != nil || knowledge == nil {
		return strings.TrimSpace(reqCtx.query)
	}
	return buildTranslationSessionTitleSeed(
		reqCtx.query,
		firstNonEmptyString(strings.TrimSpace(knowledge.Title), strings.TrimSpace(knowledge.FileName)),
		optionalTranslationTargetLanguage(reqCtx.translationOptions),
		optionalTranslationOutputFormat(reqCtx.translationOptions),
	)
}

func optionalTranslationTargetLanguage(options *types.ChatDocumentTranslationOptions) string {
	if options == nil {
		return ""
	}
	return strings.TrimSpace(firstNonEmptyString(options.TargetLanguage))
}

func optionalTranslationOutputFormat(options *types.ChatDocumentTranslationOptions) string {
	if options == nil {
		return ""
	}
	return strings.TrimSpace(firstNonEmptyString(options.OutputFormat))
}

// resolveAgent resolves the custom agent by ID, trying shared agent first, then own agent.
// Returns (nil, 0) if agentID is empty or not found.
func (h *Handler) resolveAgent(ctx context.Context, c *gin.Context, agentID string) (*types.CustomAgent, uint64) {
	if agentID == "" {
		return nil, 0
	}

	logger.Infof(ctx, "Resolving agent, agent ID: %s", secutils.SanitizeForLog(agentID))

	// Try shared agent first
	var customAgent *types.CustomAgent
	var effectiveTenantID uint64
	userIDVal, _ := c.Get(types.UserIDContextKey.String())
	currentTenantID := c.GetUint64(types.TenantIDContextKey.String())
	if h.agentShareService != nil && userIDVal != nil && currentTenantID != 0 {
		userID, _ := userIDVal.(string)
		agent, err := h.agentShareService.GetSharedAgentForUser(ctx, userID, currentTenantID, agentID)
		if err == nil && agent != nil {
			effectiveTenantID = agent.TenantID
			customAgent = agent
			logger.Infof(ctx, "Using shared agent: ID=%s, Name=%s, effectiveTenantID=%d (retrieval scope)",
				customAgent.ID, customAgent.Name, effectiveTenantID)
		}
	}

	// Fall back to own agent
	if customAgent == nil {
		agent, err := h.customAgentService.GetAgentByID(ctx, agentID)
		if err == nil {
			customAgent = agent
			logger.Infof(ctx, "Using own agent: ID=%s, Name=%s, AgentMode=%s",
				customAgent.ID, customAgent.Name, customAgent.Config.AgentMode)
		} else {
			logger.Warnf(ctx, "Failed to get custom agent, agent ID: %s, error: %v, using default config",
				secutils.SanitizeForLog(agentID), err)
		}
	} else {
		logger.Infof(ctx, "Using custom agent: ID=%s, Name=%s, IsBuiltin=%v, AgentMode=%s, effectiveTenantID=%d",
			customAgent.ID, customAgent.Name, customAgent.IsBuiltin, customAgent.Config.AgentMode, effectiveTenantID)
	}

	return customAgent, effectiveTenantID
}

// mergeKnowledgeTargets merges request KB/knowledge IDs with @mentioned items into deduplicated slices.
func mergeKnowledgeTargets(requestKBIDs []string, requestKnowledgeIDs []string, mentionedItems []MentionedItemRequest) (kbIDs []string, knowledgeIDs []string) {
	kbIDSet := make(map[string]bool)
	kbIDs = make([]string, 0, len(requestKBIDs)+len(mentionedItems))
	for _, id := range requestKBIDs {
		if id != "" && !kbIDSet[id] {
			kbIDs = append(kbIDs, id)
			kbIDSet[id] = true
		}
	}

	knowledgeIDSet := make(map[string]bool)
	knowledgeIDs = make([]string, 0, len(requestKnowledgeIDs)+len(mentionedItems))
	for _, id := range requestKnowledgeIDs {
		if id != "" && !knowledgeIDSet[id] {
			knowledgeIDs = append(knowledgeIDs, id)
			knowledgeIDSet[id] = true
		}
	}

	for _, item := range mentionedItems {
		if item.ID == "" {
			continue
		}
		switch item.Type {
		case "kb":
			if !kbIDSet[item.ID] {
				kbIDs = append(kbIDs, item.ID)
				kbIDSet[item.ID] = true
			}
		case "file":
			if !knowledgeIDSet[item.ID] {
				knowledgeIDs = append(knowledgeIDs, item.ID)
				knowledgeIDSet[item.ID] = true
			}
		}
	}
	return kbIDs, knowledgeIDs
}

// sseStreamContext holds the context for SSE streaming
type sseStreamContext struct {
	eventBus         *event.EventBus
	asyncCtx         context.Context
	cancel           context.CancelFunc
	assistantMessage *types.Message
}

func messageUpdateContext(ctx context.Context, tenantID uint64) context.Context {
	return context.WithValue(context.WithoutCancel(ctx), types.TenantIDContextKey, tenantID)
}

// setupSSEStream sets up the SSE streaming context
func (h *Handler) setupSSEStream(reqCtx *qaRequestContext, generateTitle bool) *sseStreamContext {
	// Set SSE headers
	setSSEHeaders(reqCtx.c)

	// Write initial agent_query event
	h.writeAgentQueryEvent(reqCtx.ctx, reqCtx.sessionID, reqCtx.assistantMessage.ID)

	// Base context for async work: when using shared agent, use source tenant for model/KB/MCP resolution
	baseCtx := reqCtx.ctx
	if reqCtx.effectiveTenantID != 0 && h.tenantService != nil {
		if tenant, err := h.tenantService.GetTenantByID(reqCtx.ctx, reqCtx.effectiveTenantID); err == nil && tenant != nil {
			baseCtx = context.WithValue(context.WithValue(reqCtx.ctx, types.TenantIDContextKey, reqCtx.effectiveTenantID), types.TenantInfoContextKey, tenant)
			logger.Infof(reqCtx.ctx, "Using effective tenant %d for shared agent (model/KB/MCP)", reqCtx.effectiveTenantID)
		}
	}

	// Create EventBus and cancellable context
	eventBus := event.NewEventBus()
	asyncCtx, cancel := context.WithCancel(logger.CloneContext(baseCtx))

	streamCtx := &sseStreamContext{
		eventBus:         eventBus,
		asyncCtx:         asyncCtx,
		cancel:           cancel,
		assistantMessage: reqCtx.assistantMessage,
	}

	// Setup stop event handler
	h.setupStopEventHandler(eventBus, reqCtx.sessionID, reqCtx.session.TenantID, reqCtx.assistantMessage, cancel)
	// Generate title if needed
	if generateTitle && reqCtx.session.Title == "" {
		// Use the same model as the conversation for title generation
		modelID := ""
		if reqCtx.customAgent != nil && reqCtx.customAgent.Config.ModelID != "" {
			modelID = reqCtx.customAgent.Config.ModelID
		}
		titleSeedQuery := strings.TrimSpace(reqCtx.titleSeedQuery)
		if titleSeedQuery == "" {
			titleSeedQuery = reqCtx.query
		}
		logger.Infof(reqCtx.ctx, "Session has no title, starting async title generation, session ID: %s, model: %s", reqCtx.sessionID, modelID)
		h.sessionService.GenerateTitleAsync(asyncCtx, reqCtx.session, titleSeedQuery, modelID, eventBus)
	}

	return streamCtx
}

// SearchKnowledge godoc
// @Summary      知识搜索
// @Description  在知识库中搜索（不使用LLM总结）
// @Tags         问答
// @Accept       json
// @Produce      json
// @Param        request  body      SearchKnowledgeRequest  true  "搜索请求"
// @Success      200      {object}  map[string]interface{}  "搜索结果"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /sessions/search [post]
func (h *Handler) SearchKnowledge(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())
	logger.Info(ctx, "Start processing knowledge search request")

	// Parse request body
	var request SearchKnowledgeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		logger.Error(ctx, "Failed to parse request data", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	// Validate request parameters
	if request.Query == "" {
		logger.Error(ctx, "Query content is empty")
		c.Error(errors.NewBadRequestError("Query content cannot be empty"))
		return
	}

	// Merge single knowledge_base_id into knowledge_base_ids for backward compatibility
	knowledgeBaseIDs := request.KnowledgeBaseIDs
	if request.KnowledgeBaseID != "" {
		// Check if it's already in the list to avoid duplicates
		found := false
		for _, id := range knowledgeBaseIDs {
			if id == request.KnowledgeBaseID {
				found = true
				break
			}
		}
		if !found {
			knowledgeBaseIDs = append(knowledgeBaseIDs, request.KnowledgeBaseID)
		}
	}

	if len(knowledgeBaseIDs) == 0 && len(request.KnowledgeIDs) == 0 {
		logger.Error(ctx, "No knowledge base IDs or knowledge IDs provided")
		c.Error(errors.NewBadRequestError("At least one knowledge_base_id, knowledge_base_ids or knowledge_ids must be provided"))
		return
	}

	logger.Infof(
		ctx,
		"Knowledge search request, knowledge base IDs: %v, knowledge IDs: %v, query: %s",
		secutils.SanitizeForLogArray(knowledgeBaseIDs),
		secutils.SanitizeForLogArray(request.KnowledgeIDs),
		secutils.SanitizeForLog(request.Query),
	)

	// Directly call knowledge retrieval service without LLM summarization
	searchResults, err := h.sessionService.SearchKnowledge(ctx, knowledgeBaseIDs, request.KnowledgeIDs, request.Query)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Knowledge search completed, found %d results", len(searchResults))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    searchResults,
	})
}

// KnowledgeQA godoc
// @Summary      知识问答
// @Description  基于知识库的问答（使用LLM总结），支持SSE流式响应
// @Tags         问答
// @Accept       json
// @Produce      text/event-stream
// @Param        session_id  path      string                   true  "会话ID"
// @Param        request     body      CreateKnowledgeQARequest true  "问答请求"
// @Success      200         {object}  map[string]interface{}   "问答结果（SSE流）"
// @Failure      400         {object}  errors.AppError          "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /sessions/{session_id}/knowledge-qa [post]
func (h *Handler) KnowledgeQA(c *gin.Context) {
	// Parse and validate request
	reqCtx, request, err := h.parseQARequest(c, "KnowledgeQA")
	if err != nil {
		c.Error(err)
		return
	}

	// Execute normal mode QA, generate title unless disabled
	h.executeQA(reqCtx, qaModeNormal, !request.DisableTitle)
}

// AgentQA godoc
// @Summary      Agent问答
// @Description  基于Agent的智能问答，支持多轮对话和SSE流式响应
// @Tags         问答
// @Accept       json
// @Produce      text/event-stream
// @Param        session_id  path      string                   true  "会话ID"
// @Param        request     body      CreateKnowledgeQARequest true  "问答请求"
// @Success      200         {object}  map[string]interface{}   "问答结果（SSE流）"
// @Failure      400         {object}  errors.AppError          "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /sessions/{session_id}/agent-qa [post]
func (h *Handler) AgentQA(c *gin.Context) {
	// Parse and validate request
	reqCtx, request, err := h.parseQARequest(c, "AgentQA")
	if err != nil {
		c.Error(err)
		return
	}

	// Determine if agent mode should be enabled
	// Priority: customAgent.IsAgentMode() > request.AgentEnabled
	agentModeEnabled := request.AgentEnabled
	if reqCtx.customAgent != nil {
		agentModeEnabled = reqCtx.customAgent.IsAgentMode()
		logger.Infof(reqCtx.ctx, "Agent mode determined by custom agent: %v (config.agent_mode=%s)",
			agentModeEnabled, reqCtx.customAgent.Config.AgentMode)
	}

	// Route to appropriate handler based on agent mode
	if agentModeEnabled {
		h.executeQA(reqCtx, qaModeAgent, true)
	} else {
		logger.Infof(reqCtx.ctx, "Agent mode disabled, delegating to normal mode for session: %s", reqCtx.sessionID)
		h.executeQA(reqCtx, qaModeNormal, false)
	}
}

// qaMode determines which QA execution path to use.
type qaMode int

const (
	qaModeNormal qaMode = iota // KnowledgeQA pipeline (RAG / pure chat)
	qaModeAgent                // Agent engine with tool calling
)

// executeQA is the unified execution flow for both KnowledgeQA and AgentQA modes.
// It handles message creation, SSE setup, VLM analysis, service invocation, and error handling.
func (h *Handler) executeQA(reqCtx *qaRequestContext, mode qaMode, generateTitle bool) {
	ctx := reqCtx.ctx
	sessionID := reqCtx.sessionID

	// Agent mode: emit agent query event before message creation
	if mode == qaModeAgent {
		if err := event.Emit(ctx, event.Event{
			Type:      event.EventAgentQuery,
			SessionID: sessionID,
			RequestID: reqCtx.requestID,
			Data: event.AgentQueryData{
				SessionID: sessionID,
				Query:     reqCtx.query,
				RequestID: reqCtx.requestID,
			},
		}); err != nil {
			logger.Errorf(ctx, "Failed to emit agent query event: %v", err)
			return
		}
	}

	// Create user message
	userMsg, err := h.createUserMessage(ctx, sessionID, reqCtx.query, reqCtx.requestID, reqCtx.mentionedItems, convertImageAttachments(reqCtx.images), reqCtx.attachments, reqCtx.channel)
	if err != nil {
		reqCtx.c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	reqCtx.userMessageID = userMsg.ID

	// Create assistant message
	assistantMessagePtr, err := h.createAssistantMessage(ctx, reqCtx.assistantMessage)
	if err != nil {
		reqCtx.c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	reqCtx.assistantMessage = assistantMessagePtr

	if mode == qaModeNormal {
		logger.Infof(ctx, "Using knowledge bases: %v", reqCtx.knowledgeBaseIDs)
	} else {
		logger.Infof(ctx, "Calling agent QA service, session ID: %s", sessionID)
	}

	// Setup SSE stream
	streamCtx := h.setupSSEStream(reqCtx, generateTitle)
	var artifactMu sync.RWMutex
	var completedArtifact *types.ChatDocumentArtifact
	setCompletedArtifact := func(artifact *types.ChatDocumentArtifact) {
		if artifact == nil {
			return
		}
		artifactMu.Lock()
		completedArtifact = artifact
		artifactMu.Unlock()
	}
	getCompletedArtifact := func() *types.ChatDocumentArtifact {
		artifactMu.RLock()
		defer artifactMu.RUnlock()
		return completedArtifact
	}
	qaReq := reqCtx.buildQARequest()
	if dispatcher, ok := h.sessionService.(longDocumentTaskDispatcher); ok {
		dispatched, dispatchErr := dispatcher.DispatchLongDocumentTask(streamCtx.asyncCtx, qaReq, qaModeToLongDocumentExecutionMode(mode))
		if dispatchErr != nil {
			streamCtx.cancel()
			reqCtx.c.Error(errors.NewInternalServerError(dispatchErr.Error()))
			return
		}
		if dispatched {
			defer streamCtx.cancel()
			h.streamMessageFromManager(reqCtx.c, reqCtx.sessionID, reqCtx.assistantMessage.ID, reqCtx.requestID)
			return
		}
	}
	artifactOptions := types.RegisterChatDocumentArtifactOptions{
		UserQuery:               reqCtx.query,
		Intent:                  reqCtx.documentIntent,
		Operation:               reqCtx.documentOperation,
		OutputMode:              reqCtx.documentOutputMode,
		DocumentTaskKind:        strings.TrimSpace(reqCtx.documentTaskKind),
		TargetLanguage:          optionalTranslationTargetLanguage(reqCtx.translationOptions),
		TranslationOutputFormat: optionalTranslationOutputFormat(reqCtx.translationOptions),
		NeedArtifact:            shouldCreateArtifactForQARequest(reqCtx),
		UseLongDocument:         shouldTreatQARequestAsLongDocument(reqCtx),
		TargetHeading:           reqCtx.documentTargetHeading,
		MergeMode:               reqCtx.documentMergeMode,
		BaseArtifact:            reqCtx.baseArtifact,
	}

	// Normal mode: register completion handler on EventAgentFinalAnswer
	// (Agent mode handles completion in the defer block instead)
	if mode == qaModeNormal {
		var completionHandled bool
		completionOptions := defaultAssistantCompletionOptions()
		completionOptions.AutoContinueRound = reqCtx.autoContinueRound
		completionOptions.RegisterArtifactOptions = artifactOptions
		completionOptions.ArtifactObserver = setCompletedArtifact
		completionOptions.Extra = mergeCompletionExtra(completionOptions.Extra, buildChatRouteCompletionExtra(reqCtx.routeDecision, reqCtx.routeModelID, reqCtx.routeDecisionApplied))
		streamCtx.eventBus.On(event.EventAgentFinalAnswer, func(ctx context.Context, evt event.Event) error {
			data, ok := evt.Data.(event.AgentFinalAnswerData)
			if !ok {
				return nil
			}
			streamCtx.assistantMessage.Content += data.Content
			if data.IsFallback {
				streamCtx.assistantMessage.IsFallback = true
			}
			nextOptions := completionOptionsFromFinalAnswer(data)
			nextOptions.AutoContinueRound = reqCtx.autoContinueRound
			nextOptions.FinishReason, nextOptions.FailureReason = normalizeChatDocumentRetryBudgetOutcome(nextOptions.FinishReason, nextOptions.FailureReason, reqCtx.autoContinueRound)
			nextOptions.Extra = mergeCompletionExtra(completionOptions.Extra, nextOptions.Extra)
			nextOptions.RegisterArtifactOptions = completionOptions.RegisterArtifactOptions
			completionOptions = nextOptions
			if data.Done {
				if completionHandled {
					return nil
				}
				completionHandled = true

				logger.Infof(streamCtx.asyncCtx, "Knowledge QA service completed for session: %s", sessionID)
				updateCtx := messageUpdateContext(streamCtx.asyncCtx, reqCtx.session.TenantID)
				if h.completeAssistantMessageInPlace(updateCtx, streamCtx.assistantMessage, reqCtx.query, &completionOptions) {
					emitAssistantCompleteEvent(streamCtx.eventBus, sessionID, streamCtx.assistantMessage, completionOptions, getCompletedArtifact())
				}
			}
			return nil
		})
		streamCtx.eventBus.On(event.EventError, func(ctx context.Context, evt event.Event) error {
			data, ok := evt.Data.(event.ErrorData)
			if !ok || completionHandled {
				return nil
			}
			completionHandled = true
			options := completionOptionsFromError(data)
			options.AutoContinueRound = reqCtx.autoContinueRound
			options.Extra = mergeCompletionExtra(completionOptions.Extra, options.Extra)
			options.ArtifactObserver = completionOptions.ArtifactObserver
			updateCtx := messageUpdateContext(streamCtx.asyncCtx, reqCtx.session.TenantID)
			if h.completeAssistantMessageInPlace(updateCtx, streamCtx.assistantMessage, reqCtx.query, &options) {
				emitAssistantCompleteEvent(streamCtx.eventBus, sessionID, streamCtx.assistantMessage, options, getCompletedArtifact())
			}
			return nil
		})
	}

	agentCompletionOptions := agentAssistantCompletionOptions()
	agentCompletionOptions.AutoContinueRound = reqCtx.autoContinueRound
	agentCompletionOptions.RegisterArtifactOptions = artifactOptions
	agentCompletionOptions.ArtifactObserver = setCompletedArtifact
	agentCompletionOptions.Extra = mergeCompletionExtra(agentCompletionOptions.Extra, buildChatRouteCompletionExtra(reqCtx.routeDecision, reqCtx.routeModelID, reqCtx.routeDecisionApplied))
	if mode == qaModeAgent {
		agentCompletionObserved := false
		streamCtx.eventBus.On(event.EventAgentComplete, func(ctx context.Context, evt event.Event) error {
			data, ok := evt.Data.(event.AgentCompleteData)
			if !ok {
				return nil
			}
			nextOptions := completionOptionsFromComplete(data)
			nextOptions.AutoContinueRound = reqCtx.autoContinueRound
			nextOptions.FinishReason, nextOptions.FailureReason = normalizeChatDocumentRetryBudgetOutcome(nextOptions.FinishReason, nextOptions.FailureReason, reqCtx.autoContinueRound)
			nextOptions.Extra = mergeCompletionExtra(agentCompletionOptions.Extra, nextOptions.Extra)
			nextOptions.RegisterArtifactOptions = agentCompletionOptions.RegisterArtifactOptions
			if strings.TrimSpace(nextOptions.GenerationRunID) != "" {
				nextOptions.RegisterArtifactOptions.GenerationRunID = nextOptions.GenerationRunID
			}
			nextOptions.ArtifactObserver = agentCompletionOptions.ArtifactObserver
			agentCompletionOptions = nextOptions
			agentCompletionObserved = true
			updateCtx := messageUpdateContext(streamCtx.asyncCtx, reqCtx.session.TenantID)
			h.completeAssistantMessageInPlace(updateCtx, streamCtx.assistantMessage, reqCtx.query, &agentCompletionOptions)
			return nil
		})
		streamCtx.eventBus.On(event.EventError, func(ctx context.Context, evt event.Event) error {
			data, ok := evt.Data.(event.ErrorData)
			if !ok {
				return nil
			}
			if agentCompletionObserved {
				logger.Infof(streamCtx.asyncCtx, "Ignored agent error after terminal complete, session_id: %s, message_id: %s, stage: %s", reqCtx.sessionID, reqCtx.assistantMessage.ID, data.Stage)
				return nil
			}
			nextOptions := markAgentCompletion(completionOptionsFromError(data))
			nextOptions.AutoContinueRound = reqCtx.autoContinueRound
			nextOptions.Extra = mergeCompletionExtra(agentCompletionOptions.Extra, nextOptions.Extra)
			nextOptions.RegisterArtifactOptions = agentCompletionOptions.RegisterArtifactOptions
			nextOptions.ArtifactObserver = agentCompletionOptions.ArtifactObserver
			agentCompletionOptions = nextOptions
			return nil
		})
	}

	h.setupStreamHandler(streamCtx.asyncCtx, reqCtx.sessionID, reqCtx.assistantMessage.ID,
		reqCtx.requestID, reqCtx.receivedAt, reqCtx.assistantMessage, streamCtx.eventBus, getCompletedArtifact)

	// Execute QA asynchronously
	go func() {
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 10240)
				runtime.Stack(buf, true)
				stageName := "Knowledge QA"
				if mode == qaModeAgent {
					stageName = "Agent QA"
				}
				logger.ErrorWithFields(streamCtx.asyncCtx,
					errors.NewInternalServerError(fmt.Sprintf("%s service panicked: %v\n%s", stageName, r, string(buf))),
					map[string]interface{}{"session_id": sessionID})
			}
			// Agent mode: complete the assistant message in defer (normal mode does it via event handler)
			if mode == qaModeAgent {
				// Use WithoutCancel so a user-triggered stop (which cancels
				// asyncCtx) doesn't also cancel the GORM UPDATE that persists
				// AgentSteps/Content. Without this, cancelled-ctx makes
				// GORM skip the write and the agent's intermediate steps
				// (thinking / tool_call history) are lost on page refresh.
				updateCtx := messageUpdateContext(streamCtx.asyncCtx, reqCtx.session.TenantID)
				h.completeAssistantMessageInPlace(updateCtx, streamCtx.assistantMessage, reqCtx.query, &agentCompletionOptions)
				logger.Infof(streamCtx.asyncCtx, "Agent QA service completed for session: %s", sessionID)
			}
		}()

		// Run VLM image analysis if applicable
		h.runVLMAnalysisIfNeeded(streamCtx, reqCtx, mode)

		// Build QA request and invoke the appropriate service
		var serviceErr error
		var stageName string
		if mode == qaModeNormal {
			stageName = "knowledge_qa_execution"
			serviceErr = h.sessionService.KnowledgeQA(streamCtx.asyncCtx, qaReq, streamCtx.eventBus)
		} else {
			stageName = "agent_execution"
			serviceErr = h.sessionService.AgentQA(streamCtx.asyncCtx, qaReq, streamCtx.eventBus)
		}

		if serviceErr != nil {
			logger.ErrorWithFields(streamCtx.asyncCtx, serviceErr, nil)
			if mode == qaModeAgent {
				agentCompletionOptions = markAgentCompletion(completionOptionsFromError(event.ErrorData{Stage: stageName, Error: serviceErr.Error()}))
			}
			streamCtx.eventBus.Emit(streamCtx.asyncCtx, event.Event{
				Type:      event.EventError,
				SessionID: sessionID,
				Data: event.ErrorData{
					Error:     serviceErr.Error(),
					Stage:     stageName,
					SessionID: sessionID,
				},
			})
		}
	}()

	// Handle SSE events (blocking)
	shouldWaitForTitle := generateTitle && reqCtx.session.Title == ""
	h.handleAgentEventsForSSE(ctx, reqCtx.c, sessionID, reqCtx.assistantMessage.ID,
		reqCtx.requestID, streamCtx.eventBus, shouldWaitForTitle)
}

// runVLMAnalysisIfNeeded runs VLM image analysis within the async goroutine,
// emitting tool_call/tool_result events so the user can see progress.
// For normal mode, VLM only runs on the pure-chat path (no KB, no web search);
// RAG paths defer VLM to the pipeline rewrite step.
// For agent mode, VLM always runs when images and a VLM model are present.
func (h *Handler) runVLMAnalysisIfNeeded(streamCtx *sseStreamContext, reqCtx *qaRequestContext, mode qaMode) {
	if len(reqCtx.images) == 0 || reqCtx.customAgent == nil || reqCtx.customAgent.Config.VLMModelID == "" {
		return
	}

	sessionID := reqCtx.sessionID

	// In normal mode, only run VLM for pure-chat path
	if mode == qaModeNormal {
		hasRequestKBs := len(reqCtx.knowledgeBaseIDs) > 0 || len(reqCtx.knowledgeIDs) > 0
		agentWillResolveKBs := false
		if !hasRequestKBs && reqCtx.customAgent != nil && !reqCtx.customAgent.Config.RetrieveKBOnlyWhenMentioned {
			switch reqCtx.customAgent.Config.KBSelectionMode {
			case "all":
				agentWillResolveKBs = true
			case "selected", "":
				agentWillResolveKBs = len(reqCtx.customAgent.Config.KnowledgeBases) > 0
			case "none":
				agentWillResolveKBs = false
			default:
				agentWillResolveKBs = len(reqCtx.customAgent.Config.KnowledgeBases) > 0
			}
		}
		if hasRequestKBs || agentWillResolveKBs || reqCtx.webSearchEnabled {
			return // VLM will be handled by the pipeline rewrite step
		}
	}

	// Emit VLM tool call/result events
	toolCallID := uuid.New().String()
	iteration := 0 // agent mode uses iteration field

	streamCtx.eventBus.Emit(streamCtx.asyncCtx, event.Event{
		Type:      event.EventAgentToolCall,
		SessionID: sessionID,
		Data: event.AgentToolCallData{
			ToolCallID: toolCallID,
			ToolName:   "image_analysis",
			Iteration:  iteration,
		},
	})

	vlmStart := time.Now()
	h.analyzeImageAttachments(streamCtx.asyncCtx, reqCtx.images,
		reqCtx.customAgent.Config.VLMModelID, reqCtx.query)

	outputMsg := "已分析图片内容"
	if mode == qaModeAgent {
		outputMsg = "已查看图片内容"
	}
	streamCtx.eventBus.Emit(streamCtx.asyncCtx, event.Event{
		Type:      event.EventAgentToolResult,
		SessionID: sessionID,
		Data: event.AgentToolResultData{
			ToolCallID: toolCallID,
			ToolName:   "image_analysis",
			Output:     outputMsg,
			Success:    true,
			Duration:   time.Since(vlmStart).Milliseconds(),
			Iteration:  iteration,
		},
	})
}

// completeAssistantMessage marks an assistant message as complete, updates it,
// and asynchronously indexes the Q&A pair into the chat history knowledge base.
func (h *Handler) completeAssistantMessage(
	ctx context.Context,
	assistantMessage *types.Message,
	userQuery string,
	options assistantCompletionOptions,
) bool {
	return h.completeAssistantMessageInPlace(ctx, assistantMessage, userQuery, &options)
}

// completeAssistantMessageInPlace allows the caller to observe normalized or
// enriched completion options produced during persistence, such as recorded
// generation_run_state needed by the immediate complete event.
func (h *Handler) completeAssistantMessageInPlace(
	ctx context.Context,
	assistantMessage *types.Message,
	userQuery string,
	options *assistantCompletionOptions,
) bool {
	if options == nil {
		defaultOptions := defaultAssistantCompletionOptions()
		options = &defaultOptions
	}
	*options = normalizeAssistantCompletionRetryBudgetOutcome(*options)
	if shouldPreserveExistingAssistantCompletion(assistantMessage, options.CompletionStatus) {
		return false
	}
	if strings.TrimSpace(options.DocumentGenerationStatus) != "" {
		options.RegisterArtifactOptions.DocumentGenerationStatus = options.DocumentGenerationStatus
	}
	if h.messageService != nil {
		latestMessage, err := h.messageService.GetMessage(ctx, assistantMessage.SessionID, assistantMessage.ID)
		if err != nil {
			logger.Warnf(ctx,
				"Failed to reload assistant message before completion persistence, session_id: %s, message_id: %s, request_id: %s, next_status: %s, error: %v",
				assistantMessage.SessionID, assistantMessage.ID, assistantMessage.RequestID, options.CompletionStatus, err,
			)
		} else if shouldPreserveExistingAssistantCompletion(latestMessage, options.CompletionStatus) {
			*assistantMessage = *latestMessage
			logger.Infof(ctx,
				"Skipped assistant completion persistence because stored terminal state has higher priority, session_id: %s, message_id: %s, request_id: %s, stored_status: %s, next_status: %s",
				assistantMessage.SessionID, assistantMessage.ID, assistantMessage.RequestID,
				assistantMessage.CompletionStatusOrLegacy(), options.CompletionStatus,
			)
			return false
		}
	}
	updatedMessage := *assistantMessage
	updatedMessage.UpdatedAt = time.Now()
	updatedMessage.CompletionStatus = options.CompletionStatus
	updatedMessage.FinishReason = options.FinishReason
	updatedMessage.FailureReason = options.FailureReason
	updatedMessage.IsCompleted = options.AllowComplete && options.CompletionStatus == types.MessageCompletionStatusCompleted
	if options.AgentMode {
		if shouldUseAgentFinalAnswerContent(updatedMessage.Content, *options) {
			updatedMessage.Content = options.FinalAnswer
		}
		if len(updatedMessage.AgentSteps) == 0 && len(options.AgentSteps) > 0 {
			updatedMessage.AgentSteps = append(types.AgentSteps(nil), options.AgentSteps...)
		}
		if updatedMessage.AgentDurationMs == 0 && options.AgentDurationMs > 0 {
			updatedMessage.AgentDurationMs = options.AgentDurationMs
		}
	}
	updatedMessage.Content = applyChatDocumentCompletionMarker(updatedMessage.Content, &options.RegisterArtifactOptions)
	if options.FinalAnswer != "" {
		options.FinalAnswer = applyChatDocumentCompletionMarker(options.FinalAnswer, &options.RegisterArtifactOptions)
	}
	if shouldRejectEmptyCompletedDocumentEdit(updatedMessage.Content, *options) {
		options.CompletionStatus = types.MessageCompletionStatusFailed
		options.FinishReason = "empty_document_edit_completion"
		options.FailureReason = "empty_document_edit_completion"
		options.AllowIndexing = false
		options.AllowComplete = false
		updatedMessage.CompletionStatus = options.CompletionStatus
		updatedMessage.FinishReason = options.FinishReason
		updatedMessage.FailureReason = options.FailureReason
		updatedMessage.IsCompleted = false
		logger.Warnf(ctx,
			"Blocked empty completed document edit persistence, session_id: %s, message_id: %s, request_id: %s, intent: %s, operation: %s, output_mode: %s",
			updatedMessage.SessionID, updatedMessage.ID, updatedMessage.RequestID,
			options.RegisterArtifactOptions.Intent, options.RegisterArtifactOptions.Operation, options.RegisterArtifactOptions.OutputMode,
		)
	}
	if failureReason, documentGenerationStatus := invalidCompletedFullDocumentFailure(updatedMessage.Content, *options); failureReason != "" {
		options.CompletionStatus = types.MessageCompletionStatusPartial
		options.FinishReason = failureReason
		options.FailureReason = failureReason
		options.DocumentGenerationStatus = documentGenerationStatus
		options.RegisterArtifactOptions.DocumentGenerationStatus = documentGenerationStatus
		options.AllowIndexing = false
		options.AllowComplete = false
		updatedMessage.CompletionStatus = options.CompletionStatus
		updatedMessage.FinishReason = options.FinishReason
		updatedMessage.FailureReason = options.FailureReason
		updatedMessage.IsCompleted = false
		logger.Warnf(ctx,
			"Blocked invalid completed full document persistence, session_id: %s, message_id: %s, request_id: %s, finish_reason: %s, document_generation_status: %s, output_mode: %s",
			updatedMessage.SessionID, updatedMessage.ID, updatedMessage.RequestID,
			failureReason, documentGenerationStatus, options.RegisterArtifactOptions.OutputMode,
		)
	}
	agentStepsCount := len(updatedMessage.AgentSteps)
	logger.Infof(ctx,
		"Preparing assistant completion persistence, session_id: %s, message_id: %s, request_id: %s, completion_status: %s, finish_reason: %s, failure_reason: %s, allow_complete: %t, allow_indexing: %t, agent_mode: %t, content_len: %d, agent_steps_count: %d",
		updatedMessage.SessionID, updatedMessage.ID, updatedMessage.RequestID,
		options.CompletionStatus, options.FinishReason, options.FailureReason,
		options.AllowComplete, options.AllowIndexing, options.AgentMode, len([]rune(updatedMessage.Content)), agentStepsCount,
	)
	if options.AgentMode {
		logger.Infof(ctx,
			"Preparing agent assistant completion persistence, session_id: %s, message_id: %s, request_id: %s, agent_steps_count: %d, completion_status: %s, finish_reason: %s",
			updatedMessage.SessionID, updatedMessage.ID, updatedMessage.RequestID, agentStepsCount,
			options.CompletionStatus, options.FinishReason,
		)
		if options.AllowComplete && options.CompletionStatus == types.MessageCompletionStatusCompleted && agentStepsCount == 0 {
			logger.Warnf(ctx,
				"Agent completion is being persisted without agent_steps, session_id: %s, message_id: %s, request_id: %s, completion_status: %s, finish_reason: %s",
				updatedMessage.SessionID, updatedMessage.ID, updatedMessage.RequestID,
				options.CompletionStatus, options.FinishReason,
			)
		}
	}
	if err := h.messageService.UpdateMessage(ctx, &updatedMessage); err != nil {
		logger.Errorf(ctx,
			"Failed to persist assistant message completion state, session_id: %s, message_id: %s, request_id: %s, status: %s, finish_reason: %s, agent_mode: %t, agent_steps_count: %d, error: %v",
			assistantMessage.SessionID, assistantMessage.ID, assistantMessage.RequestID,
			options.CompletionStatus, options.FinishReason, options.AgentMode, agentStepsCount, err,
		)
		return false
	}
	*assistantMessage = updatedMessage
	logger.Infof(ctx,
		"Persisted assistant completion state, session_id: %s, message_id: %s, request_id: %s, completion_status: %s, finish_reason: %s, failure_reason: %s, is_completed: %t, allow_indexing: %t, agent_mode: %t, content_len: %d",
		assistantMessage.SessionID, assistantMessage.ID, assistantMessage.RequestID,
		assistantMessage.CompletionStatus, assistantMessage.FinishReason, assistantMessage.FailureReason,
		assistantMessage.IsCompleted, options.AllowIndexing, options.AgentMode, len([]rune(assistantMessage.Content)),
	)
	if options.AgentMode {
		logger.Infof(ctx,
			"Persisted agent assistant completion state, session_id: %s, message_id: %s, request_id: %s, agent_steps_count: %d, completion_status: %s, finish_reason: %s, update_success: true",
			assistantMessage.SessionID, assistantMessage.ID, assistantMessage.RequestID, len(assistantMessage.AgentSteps),
			assistantMessage.CompletionStatus, assistantMessage.FinishReason,
		)
	}
	var completedArtifact *types.ChatDocumentArtifact
	if h.chatDocumentArtifactService != nil {
		artifactOptions := options.RegisterArtifactOptions
		if artifactOptions.UserQuery == "" {
			artifactOptions.UserQuery = userQuery
		}
		populateRegisterArtifactOptionsFromCompletion(*options, &artifactOptions)
		aggregated := appservice.AggregateDocumentGenerationArtifact(ctx, h.sessionService, h.chatDocumentArtifactService, appservice.DocumentGenerationAggregateInput{
			Message:         assistantMessage,
			RegisterOptions: artifactOptions,
			StateBuilder: func(artifact *types.ChatDocumentArtifact) types.ChatDocumentGenerationRunState {
				return buildChatDocumentGenerationRunStateUpdate(assistantMessage, artifact, *options)
			},
		})
		if aggregated.ArtifactErr != nil {
			logger.Warnf(ctx,
				"Failed to register chat document artifact, session_id: %s, message_id: %s, request_id: %s, error: %v",
				assistantMessage.SessionID, assistantMessage.ID, assistantMessage.RequestID, aggregated.ArtifactErr,
			)
		} else if aggregated.Artifact != nil {
			completedArtifact = aggregated.Artifact
			if options.ArtifactObserver != nil {
				options.ArtifactObserver(aggregated.Artifact)
			}
			logger.Infof(ctx,
				"Registered chat document artifact, session_id: %s, message_id: %s, artifact_id: %s, revision_no: %d, operation: %s",
				assistantMessage.SessionID, assistantMessage.ID, aggregated.Artifact.ID, aggregated.Artifact.RevisionNo, aggregated.Artifact.Operation,
			)
		}
		if aggregated.BindErr != nil {
			artifactID := ""
			if completedArtifact != nil {
				artifactID = completedArtifact.ID
			}
			logger.Warnf(ctx,
				"Failed to bind generation run root artifact, session_id: %s, message_id: %s, run_id: %s, artifact_id: %s, error: %v",
				assistantMessage.SessionID, assistantMessage.ID, artifactOptions.GenerationRunID, artifactID, aggregated.BindErr,
			)
		}
		if aggregated.StateErr != nil {
			logger.Warnf(ctx,
				"Failed to record chat document generation run state, session_id: %s, message_id: %s, run_id: %s, error: %v",
				assistantMessage.SessionID, assistantMessage.ID, options.GenerationRunID, aggregated.StateErr,
			)
		} else if aggregated.State != nil {
			if options.Extra == nil {
				options.Extra = map[string]interface{}{}
			}
			if data := aggregated.State.Data(); len(data) > 0 {
				options.Extra["generation_run_state"] = data
			}
			if taskKind := firstNonEmptyString(strings.TrimSpace(completedArtifactTaskKind(completedArtifact)), chatDocumentTaskKindFromExtra(options.Extra)); taskKind != "" {
				options.Extra["document_task_kind"] = taskKind
			}
		}
	}

	if !options.AllowIndexing {
		logLongDocumentPersistenceSummary(ctx, assistantMessage, *options, completedArtifact, buildMessageIndexOptions(*options, completedArtifact), false)
		return true
	}

	indexOptions := buildMessageIndexOptions(*options, completedArtifact)
	logLongDocumentPersistenceSummary(ctx, assistantMessage, *options, completedArtifact, indexOptions, true)

	// Asynchronously index the Q&A pair into the chat history knowledge base for vector search.
	// Use WithoutCancel so the goroutine survives after the HTTP request context is done.
	bgCtx := context.WithoutCancel(ctx)
	go h.messageService.IndexMessageToKB(bgCtx, userQuery, assistantMessage.Content, assistantMessage.ID, assistantMessage.SessionID, indexOptions)

	return true
}

func buildChatDocumentGenerationRunStateUpdate(message *types.Message, artifact *types.ChatDocumentArtifact, options assistantCompletionOptions) types.ChatDocumentGenerationRunState {
	state := types.ChatDocumentGenerationRunState{
		TaskKind:             firstNonEmptyString(completedArtifactTaskKind(artifact), chatDocumentTaskKindFromExtra(options.Extra)),
		LastCompletionStatus: strings.TrimSpace(options.CompletionStatus),
		LastFinishReason:     strings.TrimSpace(options.FinishReason),
		LastFailureReason:    strings.TrimSpace(options.FailureReason),
		LastDocumentStatus:   types.NormalizeChatDocumentGenerationStatus(options.DocumentGenerationStatus),
		AutoContinueRound:    options.AutoContinueRound,
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
	return types.NormalizeChatDocumentGenerationRunState(state)
}

func completedArtifactTaskKind(artifact *types.ChatDocumentArtifact) string {
	if artifact == nil {
		return ""
	}
	return strings.TrimSpace(artifact.DocumentTaskKind)
}

func chatDocumentTaskKindFromExtra(extra map[string]interface{}) string {
	if len(extra) == 0 {
		return ""
	}
	if taskKind, ok := extra["document_task_kind"].(string); ok {
		return strings.TrimSpace(taskKind)
	}
	if _, ok := extra["translation_progress"]; ok {
		return types.ChatDocumentTaskKindTranslation
	}
	state := chatDocumentGenerationRunStateFromExtra(extra)
	return strings.TrimSpace(state.TaskKind)
}

func logLongDocumentPersistenceSummary(
	ctx context.Context,
	assistantMessage *types.Message,
	options assistantCompletionOptions,
	artifact *types.ChatDocumentArtifact,
	indexOptions interfaces.MessageIndexOptions,
	indexScheduled bool,
) {
	if assistantMessage == nil {
		return
	}
	if strings.TrimSpace(indexOptions.TaskKind) != "long_document" && artifact == nil && strings.TrimSpace(options.RegisterArtifactOptions.OutputMode) != types.ChatDocumentOutputModeFull {
		return
	}
	artifactID := ""
	artifactRevision := 0
	artifactRegistered := false
	if artifact != nil {
		artifactID = strings.TrimSpace(artifact.ID)
		artifactRevision = artifact.RevisionNo
		artifactRegistered = artifactID != ""
	}
	observability := buildLongDocumentPersistenceObservability(options, artifact)
	logger.Infof(ctx,
		"[LongDocument] persistence summary, session_id: %s, message_id: %s, completion_status: %s, finish_reason: %s, failure_reason: %s, document_generation_status: %s, artifact_registered: %t, artifact_id: %s, artifact_revision_no: %d, allow_indexing: %t, index_scheduled: %t, index_task_kind: %s, index_artifact_id: %s, outline_sections: %d, generation_run_id: %s, quality_issues: %v, budget_source: %s",
		assistantMessage.SessionID,
		assistantMessage.ID,
		strings.TrimSpace(options.CompletionStatus),
		strings.TrimSpace(options.FinishReason),
		strings.TrimSpace(options.FailureReason),
		strings.TrimSpace(indexOptions.DocumentGenerationStatus),
		artifactRegistered,
		artifactID,
		artifactRevision,
		options.AllowIndexing,
		indexScheduled,
		strings.TrimSpace(indexOptions.TaskKind),
		strings.TrimSpace(indexOptions.ArtifactID),
		len(indexOptions.DocumentSections),
		observability.GenerationRunID,
		observability.QualityIssues,
		observability.BudgetSource,
	)
}

type longDocumentPersistenceObservability struct {
	GenerationRunID string
	BudgetSource    string
	QualityIssues   []string
}

func buildLongDocumentPersistenceObservability(options assistantCompletionOptions, artifact *types.ChatDocumentArtifact) longDocumentPersistenceObservability {
	observability := longDocumentPersistenceObservability{
		GenerationRunID: firstNonEmptyCompletionString(options.GenerationRunID, options.RegisterArtifactOptions.GenerationRunID),
		QualityIssues:   completionQualityIssuesFromExtra(options.Extra),
	}
	if artifact != nil && len(artifact.QualityIssues) > 0 {
		observability.QualityIssues = mergeCompletionQualityIssues(observability.QualityIssues, artifact.QualityIssues)
	}
	if budgetSource, ok := options.Extra["budget"].(map[string]interface{}); ok {
		if source, ok := budgetSource["source"].(string); ok {
			observability.BudgetSource = strings.TrimSpace(source)
		}
	}
	return observability
}

func shouldRejectEmptyCompletedDocumentEdit(content string, options assistantCompletionOptions) bool {
	if strings.TrimSpace(content) != "" || options.CompletionStatus != types.MessageCompletionStatusCompleted || !options.AllowComplete {
		return false
	}
	artifactOptions := options.RegisterArtifactOptions
	if types.NormalizeChatDocumentGenerationStatus(artifactOptions.DocumentGenerationStatus) == types.ChatDocumentGenerationStatusCompleted {
		return false
	}
	switch artifactOptions.Intent {
	case types.ChatDocumentIntentContinue, types.ChatDocumentIntentRevise:
		return true
	}
	switch artifactOptions.Operation {
	case types.ChatDocumentOperationContinue, types.ChatDocumentOperationRevise:
		return true
	}
	return false
}
