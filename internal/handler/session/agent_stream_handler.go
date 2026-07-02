package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// AgentStreamHandler handles agent events for SSE streaming
// It uses a dedicated EventBus per request to avoid SessionID filtering
// Events are appended to StreamManager without accumulation
type AgentStreamHandler struct {
	ctx                context.Context
	sessionID          string
	assistantMessageID string
	requestID          string
	receivedAt         time.Time // Handler entry timestamp, used for TTFB logging
	ttfbLogged         bool      // Guards one-shot TTFB log on first answer chunk
	assistantMessage   *types.Message
	streamManager      interfaces.StreamManager
	artifactProvider   func() *types.ChatDocumentArtifact

	eventBus *event.EventBus

	// State tracking
	knowledgeRefs   []*types.SearchResult
	finalAnswer     string
	answerSegments  []*answerSegment     // Per-answer-event-ID accumulation, so superseded preambles can be dropped
	eventStartTimes map[string]time.Time // Track start time for duration calculation
	mu              sync.Mutex
}

func shouldUseAuthoritativeCompleteAnswer(data event.AgentCompleteData, streamedAnswer string) bool {
	if data.FinalAnswer == "" {
		return false
	}

	if strings.TrimSpace(data.DocumentGenerationStatus) != "" {
		return streamedAnswer == "" || data.FinalAnswer != streamedAnswer
	}

	if data.CompletionStatus == types.MessageCompletionStatusCompleted {
		return streamedAnswer == "" || (data.FinishReason == "tool_calls" && data.FinalAnswer != streamedAnswer)
	}

	return data.CompletionStatus == types.MessageCompletionStatusPartial && data.FinishReason == "fallback_stop" && data.FinalAnswer != streamedAnswer
}

func shouldFallbackAnswerFromComplete(data event.AgentCompleteData, streamedAnswer, authoritativeAnswer string) bool {
	if streamedAnswer != "" || authoritativeAnswer == "" {
		return false
	}

	if data.CompletionStatus == types.MessageCompletionStatusCompleted {
		return true
	}

	return data.CompletionStatus == types.MessageCompletionStatusPartial && data.FinishReason == "fallback_stop"
}

// answerSegment accumulates the streamed content of a single final-answer event
// ID. A non-terminal round may stream a preamble ("let me search…") under its
// own answer ID and then be marked superseded once the round turns out to call
// tools; tracking segments separately lets us exclude that preamble from the
// persisted assistant message instead of leaking it into the final answer.
type answerSegment struct {
	id         string
	content    string
	superseded bool
}

// findAnswerSegment returns the segment for an answer event ID, or nil.
// Callers must hold h.mu.
func (h *AgentStreamHandler) findAnswerSegment(id string) *answerSegment {
	for _, seg := range h.answerSegments {
		if seg.id == id {
			return seg
		}
	}
	return nil
}

// composeFinalAnswer rebuilds the persisted answer from all non-superseded
// segments in arrival order. Callers must hold h.mu.
func (h *AgentStreamHandler) composeFinalAnswer() string {
	var b strings.Builder
	for _, seg := range h.answerSegments {
		if !seg.superseded {
			b.WriteString(seg.content)
		}
	}
	return b.String()
}

// NewAgentStreamHandler creates a new handler for agent SSE streaming
func NewAgentStreamHandler(
	ctx context.Context,
	sessionID, assistantMessageID, requestID string,
	receivedAt time.Time,
	assistantMessage *types.Message,
	streamManager interfaces.StreamManager,
	eventBus *event.EventBus,
	artifactProvider func() *types.ChatDocumentArtifact,
) *AgentStreamHandler {
	return &AgentStreamHandler{
		ctx:                ctx,
		sessionID:          sessionID,
		assistantMessageID: assistantMessageID,
		requestID:          requestID,
		receivedAt:         receivedAt,
		assistantMessage:   assistantMessage,
		streamManager:      streamManager,
		artifactProvider:   artifactProvider,
		eventBus:           eventBus,
		knowledgeRefs:      make([]*types.SearchResult, 0),
		eventStartTimes:    make(map[string]time.Time),
	}
}

// Subscribe subscribes to all agent streaming events on the dedicated EventBus
// No SessionID filtering needed since we have a dedicated EventBus per request
func (h *AgentStreamHandler) Subscribe() {
	// Subscribe to all agent streaming events on the dedicated EventBus
	h.eventBus.On(event.EventAgentThought, h.handleThought)
	h.eventBus.On(event.EventAgentToolCall, h.handleToolCall)
	h.eventBus.On(event.EventAgentToolResult, h.handleToolResult)
	h.eventBus.On(event.EventAgentReferences, h.handleReferences)
	h.eventBus.On(event.EventAgentFinalAnswer, h.handleFinalAnswer)
	h.eventBus.On(event.EventAgentReflection, h.handleReflection)
	h.eventBus.On(event.EventError, h.handleError)
	h.eventBus.On(event.EventSessionTitle, h.handleSessionTitle)
	h.eventBus.On(event.EventAgentComplete, h.handleComplete)
	h.eventBus.On(event.EventToolApprovalRequired, h.handleToolApprovalRequired)
	h.eventBus.On(event.EventToolApprovalResolved, h.handleToolApprovalResolved)
	h.eventBus.On(event.EventMCPOAuthRequired, h.handleMCPOAuthRequired)
	h.eventBus.On(event.EventMCPOAuthResolved, h.handleMCPOAuthResolved)
}

// handleThought handles agent thought events
func (h *AgentStreamHandler) handleThought(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.AgentThoughtData)
	if !ok {
		return nil
	}

	h.mu.Lock()

	// Track start time on first chunk
	if _, exists := h.eventStartTimes[evt.ID]; !exists {
		h.eventStartTimes[evt.ID] = time.Now()
	}

	// Calculate duration if done
	var metadata map[string]interface{}
	if data.Done {
		startTime := h.eventStartTimes[evt.ID]
		duration := time.Since(startTime)
		metadata = map[string]interface{}{
			"event_id":     evt.ID,
			"duration_ms":  duration.Milliseconds(),
			"completed_at": time.Now().Unix(),
		}
		delete(h.eventStartTimes, evt.ID)
	} else {
		metadata = map[string]interface{}{
			"event_id": evt.ID,
		}
	}
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

	h.mu.Unlock()

	// Append this chunk to stream (no accumulation - frontend will accumulate)
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeThinking,
		Content:   data.Content, // Just this chunk
		Done:      data.Done,
		Timestamp: time.Now(),
		Data:      metadata,
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append thought event to stream failed", "error", err)
	}

	return nil
}

// handleToolCall handles tool call events
func (h *AgentStreamHandler) handleToolCall(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.AgentToolCallData)
	if !ok {
		return nil
	}

	h.mu.Lock()
	// Track start time for this tool call (use tool_call_id as key)
	h.eventStartTimes[data.ToolCallID] = time.Now()
	// Any answer text streamed before this tool call was a non-terminal round's
	// preamble, not the final answer (the agent only ends by stopping naturally
	// with plain text and no tool calls). Drop those segments from the persisted
	// answer so the preamble never leaks into Message.Content.
	supersededAny := false
	for _, seg := range h.answerSegments {
		if !seg.superseded && seg.content != "" {
			seg.superseded = true
			supersededAny = true
		}
	}
	if supersededAny {
		h.finalAnswer = h.composeFinalAnswer()
	}
	h.mu.Unlock()

	metadata := map[string]interface{}{
		"tool_name":    data.ToolName,
		"arguments":    data.Arguments,
		"tool_call_id": data.ToolCallID,
	}

	// Append event to stream
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeToolCall,
		Content:   fmt.Sprintf("Calling tool: %s", data.ToolName),
		Done:      false,
		Timestamp: time.Now(),
		Data:      metadata,
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append tool call event to stream failed", "error", err)
	}

	return nil
}

// handleToolResult handles tool result events
func (h *AgentStreamHandler) handleToolResult(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.AgentToolResultData)
	if !ok {
		return nil
	}

	h.mu.Lock()
	// Calculate duration from start time if available, otherwise use provided duration
	var durationMs int64
	if startTime, exists := h.eventStartTimes[data.ToolCallID]; exists {
		durationMs = time.Since(startTime).Milliseconds()
		delete(h.eventStartTimes, data.ToolCallID)
	} else if data.Duration > 0 {
		// Fallback to provided duration if start time not tracked
		durationMs = data.Duration
	}
	h.mu.Unlock()

	// Send SSE response (both success and failure)
	responseType := types.ResponseTypeToolResult
	content := agenttools.StreamContentForToolResult(data.ToolName, data.Success, data.Error, data.Data)
	if !data.Success {
		responseType = types.ResponseTypeError
		if content == "" && data.Error != "" {
			content = data.Error
		}
	}

	// Build metadata including tool result data for rich frontend rendering
	metadata := map[string]interface{}{
		"tool_name":    data.ToolName,
		"success":      data.Success,
		"error":        data.Error,
		"duration_ms":  durationMs,
		"tool_call_id": data.ToolCallID,
	}

	clientData := agenttools.SanitizeToolResultForClient(data.ToolName, &types.ToolResult{
		Success: data.Success,
		Output:  data.Output,
		Error:   data.Error,
		Data:    data.Data,
	})
	for k, v := range clientData {
		metadata[k] = v
	}

	// Append event to stream
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      responseType,
		Content:   content,
		Done:      false,
		Timestamp: time.Now(),
		Data:      metadata,
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append tool result event to stream failed", "error", err)
	}

	return nil
}

func toolApprovalDataToMap(v interface{}) map[string]interface{} {
	b, err := json.Marshal(v)
	if err != nil {
		return map[string]interface{}{}
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]interface{}{}
	}
	return m
}

// handleToolApprovalRequired persists MCP tool human-approval prompts for SSE / replay (issue #1173).
func (h *AgentStreamHandler) handleToolApprovalRequired(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.ToolApprovalRequiredData)
	if !ok {
		return nil
	}
	meta := toolApprovalDataToMap(data)
	meta["pending_id"] = data.PendingID
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeToolApprovalRequired,
		Content:   "MCP tool requires human approval",
		Done:      true,
		Timestamp: time.Now(),
		Data:      meta,
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append tool approval required event failed", "error", err)
	}
	return nil
}

// handleToolApprovalResolved persists the outcome of a tool approval (issue #1173).
func (h *AgentStreamHandler) handleToolApprovalResolved(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.ToolApprovalResolvedData)
	if !ok {
		return nil
	}
	meta := toolApprovalDataToMap(data)
	meta["pending_id"] = data.PendingID
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeToolApprovalResolved,
		Content:   "MCP tool approval resolved",
		Done:      true,
		Timestamp: time.Now(),
		Data:      meta,
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append tool approval resolved event failed", "error", err)
	}
	return nil
}

// handleMCPOAuthRequired forwards an in-conversation "authorize this MCP
// service" prompt to the SSE stream so the UI can render an Authorize card.
func (h *AgentStreamHandler) handleMCPOAuthRequired(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.MCPOAuthRequiredData)
	if !ok {
		return nil
	}
	meta := toolApprovalDataToMap(data)
	meta["pending_id"] = data.PendingID
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeMCPOAuthRequired,
		Content:   "MCP service requires OAuth authorization",
		Done:      true,
		Timestamp: time.Now(),
		Data:      meta,
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append mcp oauth required event failed", "error", err)
	}
	return nil
}

// handleMCPOAuthResolved forwards the outcome of an in-conversation OAuth prompt.
func (h *AgentStreamHandler) handleMCPOAuthResolved(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.MCPOAuthResolvedData)
	if !ok {
		return nil
	}
	meta := toolApprovalDataToMap(data)
	meta["pending_id"] = data.PendingID
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeMCPOAuthResolved,
		Content:   "MCP OAuth authorization resolved",
		Done:      true,
		Timestamp: time.Now(),
		Data:      meta,
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append mcp oauth resolved event failed", "error", err)
	}
	return nil
}

// handleReferences handles knowledge references events
func (h *AgentStreamHandler) handleReferences(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.AgentReferencesData)
	if !ok {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Extract knowledge references
	// Try to cast directly to []*types.SearchResult first
	if searchResults, ok := data.References.([]*types.SearchResult); ok {
		h.knowledgeRefs = append(h.knowledgeRefs, searchResults...)
	} else if refs, ok := data.References.([]interface{}); ok {
		// Fallback: convert from []interface{}
		for _, ref := range refs {
			if sr, ok := ref.(*types.SearchResult); ok {
				h.knowledgeRefs = append(h.knowledgeRefs, sr)
			} else if refMap, ok := ref.(map[string]interface{}); ok {
				// Parse from map if needed
				searchResult := &types.SearchResult{
					ID:                   getString(refMap, "id"),
					Content:              getString(refMap, "content"),
					Score:                getFloat64(refMap, "score"),
					KnowledgeID:          getString(refMap, "knowledge_id"),
					KnowledgeTitle:       getString(refMap, "knowledge_title"),
					ChunkIndex:           int(getFloat64(refMap, "chunk_index")),
					KnowledgeDescription: getString(refMap, "knowledge_description"),
					KnowledgeBaseID:      getString(refMap, "knowledge_base_id"),
				}

				if meta, ok := refMap["metadata"].(map[string]interface{}); ok {
					metadata := make(map[string]string)
					for k, v := range meta {
						if strVal, ok := v.(string); ok {
							metadata[k] = strVal
						}
					}
					searchResult.Metadata = metadata
				}

				h.knowledgeRefs = append(h.knowledgeRefs, searchResult)
			}
		}
	}

	// Update assistant message references
	h.assistantMessage.KnowledgeReferences = h.knowledgeRefs

	// Append references event to stream
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeReferences,
		Content:   "",
		Done:      false,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"references": types.References(h.knowledgeRefs),
		},
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append references event to stream failed", "error", err)
	}

	return nil
}

// handleFinalAnswer handles final answer events
func (h *AgentStreamHandler) handleFinalAnswer(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.AgentFinalAnswerData)
	if !ok {
		return nil
	}

	h.mu.Lock()

	// Track start time on first chunk
	if _, exists := h.eventStartTimes[evt.ID]; !exists {
		h.eventStartTimes[evt.ID] = time.Now()
	}

	// Emit a one-shot TTFB log the first time *any* answer chunk reaches
	// the stream handler. This lets us compare the backend's "request in →
	// first token out" timing against the frontend-observed TTFB and pin
	// down where latency lives (network vs server vs LLM).
	if !h.ttfbLogged && !h.receivedAt.IsZero() {
		h.ttfbLogged = true
		ttfb := time.Since(h.receivedAt)
		logger.GetLogger(h.ctx).Infof("TTFB:first_answer_chunk request_id=%s, session_id=%s, ttfb_ms=%d",
			h.requestID, h.sessionID, ttfb.Milliseconds())
	}

	// Accumulate final answer locally for assistant message (database). Track
	// per event ID so a later supersede can subtract this segment's content.
	if data.Content != "" {
		seg := h.findAnswerSegment(evt.ID)
		if seg == nil {
			seg = &answerSegment{id: evt.ID}
			h.answerSegments = append(h.answerSegments, seg)
		}
		seg.content += data.Content
		h.finalAnswer = h.composeFinalAnswer()
	}
	if data.IsFallback {
		h.assistantMessage.IsFallback = true
	}

	// Calculate duration if done
	var metadata map[string]interface{}
	if data.Done {
		startTime := h.eventStartTimes[evt.ID]
		duration := time.Since(startTime)
		metadata = map[string]interface{}{
			"event_id":     evt.ID,
			"duration_ms":  duration.Milliseconds(),
			"completed_at": time.Now().Unix(),
		}
		delete(h.eventStartTimes, evt.ID)
	} else {
		metadata = map[string]interface{}{
			"event_id": evt.ID,
		}
	}
	if data.IsFallback {
		metadata["is_fallback"] = true
	}
	if data.CompletionStatus != "" {
		metadata["completion_status"] = data.CompletionStatus
	}
	if data.FinishReason != "" {
		metadata["finish_reason"] = data.FinishReason
	}
	if data.IsPartial {
		metadata["is_partial"] = data.IsPartial
	}
	if data.AllowIndexing {
		metadata["allow_indexing"] = data.AllowIndexing
	}
	if data.AllowComplete {
		metadata["allow_complete"] = data.AllowComplete
	}
	if data.FailureReason != "" {
		metadata["failure_reason"] = data.FailureReason
	}
	h.mu.Unlock()

	// Append this chunk to stream (frontend will accumulate by event ID)
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeAnswer,
		Content:   data.Content, // Just this chunk
		Done:      data.Done,
		Timestamp: time.Now(),
		Data:      metadata,
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append answer event to stream failed", "error", err)
	}

	return nil
}

// handleReflection handles agent reflection events
func (h *AgentStreamHandler) handleReflection(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.AgentReflectionData)
	if !ok {
		return nil
	}

	// Append this chunk to stream (frontend will accumulate by event ID)
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeReflection,
		Content:   data.Content, // Just this chunk
		Done:      data.Done,
		Timestamp: time.Now(),
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append reflection event to stream failed", "error", err)
	}

	return nil
}

// handleError handles error events
func (h *AgentStreamHandler) handleError(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.ErrorData)
	if !ok {
		return nil
	}

	h.mu.Lock()
	if messageCompletionStatusPriority(h.assistantMessage.CompletionStatusOrLegacy()) > 0 {
		logger.GetLogger(h.ctx).Infof(
			"Ignored late stream error after terminal completion, session_id=%s, message_id=%s, stage=%s, completion_status=%s",
			h.sessionID,
			h.assistantMessageID,
			data.Stage,
			h.assistantMessage.CompletionStatusOrLegacy(),
		)
		h.mu.Unlock()
		return nil
	}
	h.mu.Unlock()

	// Build error metadata
	metadata := map[string]interface{}{
		"stage": data.Stage,
		"error": data.Error,
	}

	// Append error event to stream
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeError,
		Content:   data.Error,
		Done:      true,
		Timestamp: time.Now(),
		Data:      metadata,
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append error event to stream failed", "error", err)
	}

	return nil
}

// handleSessionTitle handles session title update events
func (h *AgentStreamHandler) handleSessionTitle(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.SessionTitleData)
	if !ok {
		return nil
	}

	// Use background context for title event since it may arrive after stream completion
	bgCtx := context.Background()

	// Append title event to stream
	if err := h.streamManager.AppendEvent(bgCtx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeSessionTitle,
		Content:   data.Title,
		Done:      true,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"session_id": data.SessionID,
			"title":      data.Title,
		},
	}); err != nil {
		logger.GetLogger(h.ctx).Warn("Append session title event to stream failed (stream may have ended)", "error", err)
	}

	return nil
}

// handleComplete handles agent complete events
func (h *AgentStreamHandler) handleComplete(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.AgentCompleteData)
	if !ok {
		return nil
	}

	h.mu.Lock()
	fallbackAnswer := ""
	completionStatus := data.CompletionStatus
	finishReason := data.FinishReason
	failureReason := data.FailureReason
	allowIndexing := data.AllowIndexing
	allowComplete := data.AllowComplete
	isPartial := data.IsPartial

	// Update assistant message with final data
	if data.MessageID == h.assistantMessageID {
		preserveExistingStatus := hasHigherPriorityAssistantCompletion(h.assistantMessage, data.CompletionStatus)
		if preserveExistingStatus {
			completionStatus = h.assistantMessage.CompletionStatusOrLegacy()
			finishReason = h.assistantMessage.FinishReason
			failureReason = h.assistantMessage.FailureReason
			allowIndexing = false
			allowComplete = false
			isPartial = completionStatus == types.MessageCompletionStatusPartial
		} else {
			streamedAnswer := h.finalAnswer
			authoritativeAnswer := streamedAnswer
			if shouldUseAuthoritativeCompleteAnswer(data, streamedAnswer) {
				authoritativeAnswer = data.FinalAnswer
			}
			authoritativeAnswer, _ = types.StripChatDocumentCompletionMarker(authoritativeAnswer)

			h.assistantMessage.CompletionStatus = completionStatus
			h.assistantMessage.FinishReason = finishReason
			h.assistantMessage.FailureReason = failureReason
			h.assistantMessage.IsCompleted = completionStatus == types.MessageCompletionStatusCompleted
			h.finalAnswer = authoritativeAnswer
			h.assistantMessage.Content = authoritativeAnswer
			if shouldFallbackAnswerFromComplete(data, streamedAnswer, authoritativeAnswer) {
				fallbackAnswer = authoritativeAnswer
			}
		}
		h.assistantMessage.AgentDurationMs = data.TotalDurationMs

		// Update knowledge references if provided
		if len(data.KnowledgeRefs) > 0 {
			knowledgeRefs := make([]*types.SearchResult, 0, len(data.KnowledgeRefs))
			for _, ref := range data.KnowledgeRefs {
				if sr, ok := ref.(*types.SearchResult); ok {
					knowledgeRefs = append(knowledgeRefs, sr)
				}
			}
			h.assistantMessage.KnowledgeReferences = knowledgeRefs
		}

		// Update agent steps if provided
		if len(data.AgentSteps) > 0 {
			h.assistantMessage.AgentSteps = agenttools.SanitizeAgentStepsForStorage(data.AgentSteps)
			logger.Infof(h.ctx,
				"Agent completion steps captured, session_id: %s, message_id: %s, request_id: %s, agent_steps_count: %d, completion_status: %s, finish_reason: %s",
				h.sessionID, h.assistantMessageID, h.requestID, len(data.AgentSteps), data.CompletionStatus, data.FinishReason,
			)
		}
	}
	h.mu.Unlock()

	// Fallback: if no answer events were streamed but we have a final answer,
	// emit it as answer events so the frontend can render it properly.
	// This is allowed for completed answers and recovered partial answers where
	// fallback_stop indicates the final answer synthesis succeeded but no answer
	// chunks reached the frontend.
	if fallbackAnswer != "" {
		logger.GetLogger(h.ctx).Warnf(
			"No answer events were streamed, emitting fallback answer (len=%d). "+
				"This typically happens when: (1) model stopped naturally and content was sent as thought events, "+
				"or (2) Ollama model returned tool calls non-incrementally. "+
				"total_steps=%d, total_duration_ms=%d",
			len(fallbackAnswer), data.TotalSteps, data.TotalDurationMs,
		)
		fallbackID := fmt.Sprintf("answer-fallback-%d", time.Now().UnixMilli())
		if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
			ID:        fallbackID,
			Type:      types.ResponseTypeAnswer,
			Content:   fallbackAnswer,
			Done:      false,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"event_id":    fallbackID,
				"is_fallback": true,
			},
		}); err != nil {
			logger.GetLogger(h.ctx).Errorf("Append fallback answer event failed: %v", err)
		}
		if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
			ID:        fallbackID,
			Type:      types.ResponseTypeAnswer,
			Content:   "",
			Done:      true,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"event_id":    fallbackID,
				"is_fallback": true,
			},
		}); err != nil {
			logger.GetLogger(h.ctx).Errorf("Append fallback answer done event failed: %v", err)
		}
	}

	// Send completion event to stream manager so SSE can detect completion
	completeData := map[string]interface{}{
		"final_answer":      h.assistantMessage.Content,
		"agent_steps":       h.assistantMessage.AgentSteps,
		"agent_duration_ms": data.TotalDurationMs,
		"total_steps":       data.TotalSteps,
		"total_duration_ms": data.TotalDurationMs,
		"completion_status": completionStatus,
		"finish_reason":     finishReason,
		"is_partial":        isPartial,
		"allow_indexing":    allowIndexing,
		"allow_complete":    allowComplete,
		"failure_reason":    failureReason,
	}
	if len(h.assistantMessage.KnowledgeReferences) > 0 {
		completeData["knowledge_references"] = types.References(h.assistantMessage.KnowledgeReferences)
		completeData["knowledge_refs"] = types.References(h.assistantMessage.KnowledgeReferences)
	}
	if data.DocumentGenerationStatus != "" {
		status := types.NormalizeChatDocumentGenerationStatus(data.DocumentGenerationStatus)
		completeData["document_generation_status"] = status
		if data.AutoContinueNext != nil {
			completeData["auto_continue_next"] = *data.AutoContinueNext
		} else {
			completeData["auto_continue_next"] = shouldAutoContinueChatDocument(status, finishReason, failureReason, 0)
		}
		if data.AutoContinueReason != "" {
			completeData["auto_continue_reason"] = data.AutoContinueReason
		}
		if data.AutoContinueReasonMessage != "" {
			completeData["auto_continue_reason_message"] = data.AutoContinueReasonMessage
		}
		if data.NextAction != "" {
			completeData["next_action"] = data.NextAction
		}
		if data.NextReason != "" {
			completeData["next_reason"] = data.NextReason
		}
		if data.NextReasonMessage != "" {
			completeData["next_reason_message"] = data.NextReasonMessage
		}
		if data.CanAutoContinue != nil {
			completeData["can_auto_continue"] = *data.CanAutoContinue
		}
		if len(data.RecommendedRequest) > 0 {
			completeData["recommended_request"] = data.RecommendedRequest
		}
	}
	for key, value := range data.Extra {
		if _, exists := completeData[key]; exists {
			continue
		}
		completeData[key] = value
	}
	if h.artifactProvider != nil {
		if artifact := h.artifactProvider(); artifact != nil {
			completeData["chat_document_artifact"] = chatDocumentArtifactMetadata(artifact)
			finalDocumentMode, finalDocument, finalDocumentArtifactID := buildFinalDocumentDelivery(artifact)
			completeData["final_document_mode"] = finalDocumentMode
			if finalDocument != "" {
				completeData["final_document"] = finalDocument
			}
			completeData["final_document_artifact_id"] = finalDocumentArtifactID
			addChatDocumentGenerationPayload(completeData, artifact)
		}
	}
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeComplete,
		Content:   "",
		Done:      true,
		Timestamp: time.Now(),
		Data:      completeData,
	}); err != nil {
		logger.GetLogger(h.ctx).Errorf("Append complete event to stream failed: %v", err)
	}

	return nil
}
