package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/common"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

// streamLLMResult holds accumulated output from a streaming LLM call.
type streamLLMResult struct {
	Content string
	// ReasoningContent 累积模型返回的 reasoning_content / thinking 片段，
	// 与面向用户展示的答案正文分离保存，供后续轮次按 provider 要求原样回放。
	ReasoningContent      string
	VisibleAnswerContent  string // answer chunks emitted to users, including source-tagged chunks
	AnswerStreamed        bool
	FinalAnswerStreamed   bool
	DuplicateDocumentHead bool
	SawDone               bool
	ToolCalls             []types.LLMToolCall
	Usage                 *types.TokenUsage
	FinishReason          string // actual finish_reason from LLM (captured from last stream chunk)
	StreamError           string // error message from stream (e.g., timeout), kept separate from Content
}

const duplicateDocumentHeadFinishReason = "partial_duplicate_head"

func recentAssistantDocumentSnapshot(messages []chat.Message) (string, int) {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != "assistant" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		if len([]rune(content)) < 600 {
			continue
		}
		if len(streamDocumentHeadingTitles(content, 2)) < 2 {
			continue
		}
		return content, i
	}
	return "", -1
}

func shouldEnableDuplicateDocumentHeadGuard(messages []chat.Message, snapshotIndex int) bool {
	if snapshotIndex < 0 {
		return false
	}
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != "user" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		if content == lengthContinuationPrompt {
			return true
		}
		return snapshotIndex > i
	}
	return true
}

func streamDocumentHeadingTitles(content string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	headings := make([]string, 0, limit)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		title := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		if title == "" {
			continue
		}
		headings = append(headings, title)
		if len(headings) >= limit {
			break
		}
	}
	return headings
}

func normalizeStreamDocumentHeading(title string) string {
	replacer := strings.NewReplacer(
		" ", "",
		"\t", "",
		"\r", "",
		"\n", "",
		"-", "",
		"_", "",
		"|", "",
		"`", "",
		"*", "",
		"~", "",
		">", "",
		"　", "",
	)
	return strings.ToLower(replacer.Replace(strings.TrimSpace(title)))
}

func streamPrefixLooksLikeDocumentHead(prefix string) bool {
	trimmed := strings.TrimSpace(prefix)
	if trimmed == "" {
		return false
	}
	return strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "---")
}

func streamDuplicateHeadBufferReady(prefix string) bool {
	trimmed := strings.TrimSpace(prefix)
	if trimmed == "" {
		return false
	}
	if !streamPrefixLooksLikeDocumentHead(trimmed) {
		return true
	}
	if strings.Contains(trimmed, "\n") {
		return true
	}
	return len([]rune(trimmed)) >= 80
}

func streamPrefixRepeatsDocumentHead(base string, prefix string) bool {
	baseHeadings := streamDocumentHeadingTitles(base, 4)
	if len(baseHeadings) == 0 {
		return false
	}
	prefixHeadings := streamDocumentHeadingTitles(prefix, 2)
	if len(prefixHeadings) == 0 {
		return false
	}
	firstPrefixHeading := normalizeStreamDocumentHeading(prefixHeadings[0])
	if firstPrefixHeading == "" {
		return false
	}
	for _, heading := range baseHeadings {
		if firstPrefixHeading == normalizeStreamDocumentHeading(heading) {
			return true
		}
	}
	return false
}

func streamChunkSource(chunk types.StreamResponse) string {
	if chunk.Data == nil {
		return ""
	}
	source, _ := chunk.Data["source"].(string)
	return source
}

// streamLLMToEventBus streams LLM response through EventBus (generic method)
// emitFunc: callback to emit each chunk event
func (e *AgentEngine) streamLLMToEventBus(
	ctx context.Context,
	messages []chat.Message,
	opts *chat.ChatOptions,
	emitFunc func(chunk *types.StreamResponse, fullContent string),
) (*streamLLMResult, error) {
	logger.Debugf(ctx, "[Agent][Stream] Starting LLM stream with %d messages", len(messages))

	llmCtx, llmCancel := context.WithTimeout(ctx, e.getLLMCallTimeout())
	defer llmCancel()

	stream, err := e.chatModel.ChatStream(llmCtx, messages, opts)
	if err != nil {
		logger.Errorf(ctx, "[Agent][Stream] Failed to start LLM stream: %v", err)
		return nil, err
	}

	result := &streamLLMResult{}
	chunkCount := 0
	responseTypeCounts := make(map[string]int)
	firstChunkTime := time.Time{}

	for chunk := range stream {
		chunkCount++
		if chunkCount == 1 {
			firstChunkTime = time.Now()
		}
		responseTypeCounts[string(chunk.ResponseType)]++
		if chunk.Done {
			result.SawDone = true
		}

		// Capture error messages from the stream (e.g., "context deadline exceeded")
		// but do NOT append them to result.Content — they would leak to the user
		// as if they were part of the LLM answer.
		if chunk.ResponseType == types.ResponseTypeError {
			result.StreamError = chunk.Content
			continue
		}

		if chunk.ResponseType == types.ResponseTypeAnswer && chunk.Content != "" {
			result.AnswerStreamed = true
			result.VisibleAnswerContent += chunk.Content
			if streamChunkSource(chunk) == "final_answer_tool" {
				result.FinalAnswerStreamed = true
			}
		}

		if chunk.Content != "" {
			isExtracted := chunk.Data != nil && chunk.Data["source"] != nil
			if !isExtracted {
				if chunk.ResponseType == types.ResponseTypeThinking {
					result.ReasoningContent += chunk.Content
				} else {
					result.Content += chunk.Content
				}
			}
		}

		if chunk.ResponseType == types.ResponseTypeThinking && chunk.Content != "" {
			source := ""
			if chunk.Data != nil {
				source, _ = chunk.Data["source"].(string)
			}
			if source == "" {
				result.ReasoningContent += chunk.Content
			}
		}

		if len(chunk.ToolCalls) > 0 {
			result.ToolCalls = chunk.ToolCalls
		}

		if chunk.Usage != nil {
			result.Usage = chunk.Usage
		}

		if chunk.FinishReason != "" {
			result.FinishReason = chunk.FinishReason
		}

		if emitFunc != nil {
			emitFunc(&chunk, result.Content)
		}
	}

	// Stream diagnostic summary: helps identify non-streaming patterns
	streamDuration := time.Duration(0)
	if !firstChunkTime.IsZero() {
		streamDuration = time.Since(firstChunkTime)
	}
	logger.Infof(ctx, "[Agent][Stream] Completed: chunks=%d, content_len=%d, tool_calls=%d, "+
		"stream_duration=%dms, type_distribution=%v",
		chunkCount, len(result.Content), len(result.ToolCalls),
		streamDuration.Milliseconds(), responseTypeCounts)

	// If the stream failed after answer chunks were already emitted, keep the
	// partial answer as usable output and let the caller close the stream cleanly.
	// Source-tagged answer chunks are not part of Content, so VisibleAnswerContent
	// is the authoritative signal for "the user has already seen an answer".
	if result.StreamError != "" {
		hasUsableAnswer := strings.TrimSpace(result.Content) != "" || strings.TrimSpace(result.VisibleAnswerContent) != ""
		if hasUsableAnswer || len(result.ToolCalls) > 0 {
			if result.FinishReason == "" && hasUsableAnswer {
				result.FinishReason = "stream_error_after_answer"
			}
			return result, nil
		}
		return result, fmt.Errorf("LLM stream error: %s", result.StreamError)
	}

	return result, nil
}

// streamThinkingToEventBus streams the thinking process through EventBus
func (e *AgentEngine) streamThinkingToEventBus(
	ctx context.Context,
	messages []chat.Message,
	tools []chat.Tool,
	iteration int,
	sessionID string,
) (*types.ChatResponse, error) {
	logger.Debugf(ctx, "[Agent][Thinking] Iteration-%d: temp=%.2f, tools=%d, thinking=%v",
		iteration+1, e.config.Temperature, len(tools), e.config.Thinking)

	parallelToolCalls := true
	opts := &chat.ChatOptions{
		Temperature:       e.config.Temperature,
		Tools:             tools,
		Thinking:          e.config.Thinking,
		ParallelToolCalls: &parallelToolCalls,
	}

	pendingToolCalls := make(map[string]bool)
	thinkingToolIDs := make(map[string]string) // tool_call_id -> event ID for thinking tool streams
	priorDocumentSnapshot, priorDocumentSnapshotIndex := recentAssistantDocumentSnapshot(messages)
	var answerPrefixBuffer strings.Builder
	answerPrefixSource := "answer"
	duplicateDocumentHeadBlocked := false
	duplicateDocumentHeadDecided := !shouldEnableDuplicateDocumentHeadGuard(messages, priorDocumentSnapshotIndex)

	// Track which event types we emitted for diagnostics
	emittedEventTypes := make(map[string]int)
	answerStreamed := false
	finalAnswerStreamed := false

	// Generate IDs for this stream
	thinkingID := generateEventID("thinking")
	answerID := generateEventID("answer")
	emitAnswerChunk := func(content string, source string) {
		if content == "" {
			return
		}
		answerStreamed = true
		if source == "final_answer_tool" {
			finalAnswerStreamed = true
		}
		emittedEventTypes[source+"_chunk"]++
		e.eventBus.Emit(ctx, event.Event{
			ID:        answerID,
			Type:      event.EventAgentFinalAnswer,
			SessionID: sessionID,
			Data: event.AgentFinalAnswerData{
				Content: content,
				Done:    false,
			},
		})
	}
	flushAnswerPrefixBuffer := func(source string) {
		if answerPrefixBuffer.Len() == 0 {
			return
		}
		if source == "" {
			source = answerPrefixSource
		}
		buffered := answerPrefixBuffer.String()
		answerPrefixBuffer.Reset()
		answerPrefixSource = "answer"
		emitAnswerChunk(buffered, source)
	}
	emitDuplicateDocumentHeadDone := func() {
		emittedEventTypes["answer_done_duplicate_document_head"]++
		e.eventBus.Emit(ctx, event.Event{
			ID:        answerID,
			Type:      event.EventAgentFinalAnswer,
			SessionID: sessionID,
			Data: event.AgentFinalAnswerData{
				Content:          "",
				Done:             true,
				CompletionStatus: types.MessageCompletionStatusPartial,
				FinishReason:     duplicateDocumentHeadFinishReason,
				IsPartial:        true,
				AllowIndexing:    false,
				AllowComplete:    false,
				FailureReason:    duplicateDocumentHeadFinishReason,
			},
		})
	}

	// Routing state shared across chunk callbacks:
	//   - splitter separates inline <think>…</think> reasoning from answer text
	//     in the plain `content` channel (models that don't use reasoning_content).
	//   - thinkingOpen tracks whether the thought stream still needs a Done marker.
	//   - answerStreamed records that user-facing answer text was sent live to
	//     the final-answer area, so the natural-stop branch only emits Done.
	splitter := agenttools.NewThinkStreamSplitter()
	thinkingOpen := false

	emitThought := func(content string, done bool) {
		if content == "" && !done {
			return
		}
		emittedEventTypes["thought_chunk"]++
		e.eventBus.Emit(ctx, event.Event{
			ID:        thinkingID,
			Type:      event.EventAgentThought,
			SessionID: sessionID,
			Data: event.AgentThoughtData{
				Content:   content,
				Iteration: iteration,
				Done:      done,
			},
		})
	}
	// closeThinking emits the thought Done marker once, used right before the
	// first answer chunk so the UI flips the thinking card to "completed"
	// instead of leaving it spinning while the answer streams.
	closeThinking := func() {
		if thinkingOpen {
			emitThought("", true)
			thinkingOpen = false
		}
	}
	emitAnswer := func(content string) {
		if content == "" {
			return
		}
		// Suppress whitespace-only content emitted before the real answer has
		// started. OpenAI-compatible models frequently prepend a stray newline
		// (e.g. "\n\n") to the plain content channel in the same chunk where
		// they request tool calls. Routing that to the final-answer area leaks
		// spurious empty "answer" events interleaved with tool_call events.
		// Once genuine answer text has streamed (answerStreamed), preserve all
		// whitespace so the answer's own formatting stays intact.
		if !answerStreamed && strings.TrimSpace(content) == "" {
			return
		}
		closeThinking()
		answerStreamed = true
		emittedEventTypes["final_answer_chunk"]++
		e.eventBus.Emit(ctx, event.Event{
			ID:        answerID,
			Type:      event.EventAgentFinalAnswer,
			SessionID: sessionID,
			Data: event.AgentFinalAnswerData{
				Content: content,
				Done:    false,
			},
		})
	}

	llmResult, err := e.streamLLMToEventBus(
		ctx,
		messages,
		opts,
		func(chunk *types.StreamResponse, fullContent string) {
			if chunk.ResponseType == types.ResponseTypeToolCall && chunk.Data != nil {
				toolCallID, _ := chunk.Data["tool_call_id"].(string)
				toolName, _ := chunk.Data["tool_name"].(string)

				if toolCallID != "" && toolName != "" && !pendingToolCalls[toolCallID] {
					pendingToolCalls[toolCallID] = true
					emittedEventTypes["tool_call_pending"]++
					e.eventBus.Emit(ctx, event.Event{
						ID:        fmt.Sprintf("%s-tool-call-pending", toolCallID),
						Type:      event.EventAgentToolCall,
						SessionID: sessionID,
						Data: event.AgentToolCallData{
							ToolCallID: toolCallID,
							ToolName:   toolName,
							Iteration:  iteration,
						},
					})
				}
			}
			if chunk.ResponseType == types.ResponseTypeAnswer && chunk.Data != nil {
				if chunk.Content != "" {
					source := "answer"
					if chunk.Data != nil {
						if chunkSource, _ := chunk.Data["source"].(string); chunkSource != "" {
							source = chunkSource
						}
					}
					closeThinking()
					if duplicateDocumentHeadBlocked {
						return
					}
					if !duplicateDocumentHeadDecided {
						answerPrefixSource = source
						answerPrefixBuffer.WriteString(chunk.Content)
						buffered := answerPrefixBuffer.String()
						if !streamDuplicateHeadBufferReady(buffered) {
							return
						}
						duplicateDocumentHeadDecided = true
						if streamPrefixLooksLikeDocumentHead(buffered) && streamPrefixRepeatsDocumentHead(priorDocumentSnapshot, buffered) {
							duplicateDocumentHeadBlocked = true
							logger.Warnf(ctx, "[Agent][Thinking] Iteration-%d detected duplicate document head in streamed answer; suppressing restart chunk", iteration+1)
							emitDuplicateDocumentHeadDone()
							return
						}
						flushAnswerPrefixBuffer(source)
						return
					}
					emitAnswerChunk(chunk.Content, source)
				}
				return
			}

			// Handle thinking tool's streaming thought content
			if chunk.ResponseType == types.ResponseTypeThinking && chunk.Data != nil {
				if source, _ := chunk.Data["source"].(string); source == "thinking_tool" {
					toolCallID, _ := chunk.Data["tool_call_id"].(string)
					eventID, exists := thinkingToolIDs[toolCallID]
					if !exists {
						eventID = generateEventID("thinking-tool")
						thinkingToolIDs[toolCallID] = eventID
					}
					emittedEventTypes["thinking_tool_chunk"]++
					e.eventBus.Emit(ctx, event.Event{
						ID:        eventID,
						Type:      event.EventAgentThought,
						SessionID: sessionID,
						Data: event.AgentThoughtData{
							Content:   chunk.Content,
							Iteration: iteration,
							Done:      false,
						},
					})
					return
				}
			}
			// reasoning_content (separate thinking channel, e.g. DeepSeek V4) →
			// thought area. Forward the Done marker the provider sends when it
			// transitions from reasoning to answer.
			if chunk.ResponseType == types.ResponseTypeThinking {
				if chunk.Content != "" {
					thinkingOpen = true
					emitThought(chunk.Content, false)
				} else if chunk.Done && thinkingOpen {
					closeThinking()
				}
				return
			}

			// Plain content channel. Streamed live to the answer area
			// (optimistically rendered as the final answer). If the round turns
			// out to call tools, this was a preamble; the subsequent tool-call
			// events let the UI retract it from the answer area and relocate it
			// into the steps. Split out any inline <think> reasoning so it goes
			// to the thought area instead.
			if chunk.Content != "" {
				thinkPart, answerPart := splitter.Feed(chunk.Content)
				if thinkPart != "" {
					thinkingOpen = true
					emitThought(thinkPart, false)
				}
				emitAnswer(answerPart)
			}
			if chunk.Done {
				thinkPart, answerPart := splitter.Flush()
				if thinkPart != "" {
					thinkingOpen = true
					emitThought(thinkPart, false)
				}
				emitAnswer(answerPart)
				closeThinking()
			}
		},
	)
	if err != nil {
		logger.Errorf(ctx, "[Agent][Thinking] Iteration-%d failed: %v", iteration+1, err)
		return nil, err
	}
	if !duplicateDocumentHeadBlocked && answerPrefixBuffer.Len() > 0 {
		flushAnswerPrefixBuffer("")
	}

	// Emit diagnostics: helps identify when answer content went to "thought" vs "final_answer" events
	logger.Infof(ctx, "[Agent][Thinking] Iteration-%d completed: content=%d chars, tool_calls=%d, emitted_events=%v",
		iteration+1, len(llmResult.Content), len(llmResult.ToolCalls), emittedEventTypes)

	fullContent := llmResult.Content
	if strings.TrimSpace(fullContent) == "" && strings.TrimSpace(llmResult.VisibleAnswerContent) != "" {
		fullContent = llmResult.VisibleAnswerContent
	}
	fullContent = agenttools.StripThinkBlocks(fullContent)

	// Use actual finish_reason from LLM stream instead of hardcoding "stop".
	// Fallback to "stop" when the stream did not report a finish_reason
	// (e.g., certain Ollama models or providers that omit the field).
	finishReason := llmResult.FinishReason
	if finishReason == "" {
		finishReason = "stop"
	}
	if duplicateDocumentHeadBlocked {
		llmResult.DuplicateDocumentHead = true
		llmResult.Content = ""
		llmResult.VisibleAnswerContent = ""
		llmResult.AnswerStreamed = false
		llmResult.FinalAnswerStreamed = false
		llmResult.ToolCalls = nil
		finishReason = duplicateDocumentHeadFinishReason
		fullContent = ""
	}

	if llmResult.StreamError != "" && llmResult.AnswerStreamed && !llmResult.SawDone {
		emittedEventTypes["answer_done_after_stream_error"]++
		e.eventBus.Emit(ctx, event.Event{
			ID:        answerID,
			Type:      event.EventAgentFinalAnswer,
			SessionID: sessionID,
			Data: event.AgentFinalAnswerData{
				Content:          "",
				Done:             true,
				CompletionStatus: types.MessageCompletionStatusPartial,
				FinishReason:     finishReason,
				IsPartial:        true,
				AllowIndexing:    false,
				AllowComplete:    false,
				FailureReason:    finishReason,
			},
		})
	}

	resp := &types.ChatResponse{
		Content:          fullContent,
		ReasoningContent: llmResult.ReasoningContent,
		ToolCalls:        llmResult.ToolCalls,
		FinishReason:     finishReason,
		AnswerStreamed:   answerStreamed || llmResult.AnswerStreamed,
		FinalAnswerStreamed: finalAnswerStreamed || llmResult.FinalAnswerStreamed,
	}
	if answerStreamed || llmResult.AnswerStreamed {
		resp.AnswerEventID = answerID
	}
	if llmResult.Usage != nil {
		resp.Usage = *llmResult.Usage
	}
	return resp, nil
}

// callLLMWithRetry logs messages, sanitizes them, calls the LLM with retry on transient errors,
// and handles graceful degradation when prior tool results exist.
// Returns nil response (with state.IsComplete=true) when graceful degradation succeeds.
// Returns a non-nil error only when the call fails irrecoverably.
func (e *AgentEngine) callLLMWithRetry(
	ctx context.Context, messages []chat.Message, tools []chat.Tool,
	state *types.AgentState, query string, iteration int, sessionID string,
) (*types.ChatResponse, error) {
	round := iteration + 1
	originalMessageCount := len(messages)
	originalToolMessageCount := countToolRoleMessages(messages)

	// Sanitize messages before sending to LLM (fix consecutive roles, orphaned tool results)
	messages = agenttools.SanitizeMessages(messages)
	protocolProblems := agenttools.ValidateToolMessageProtocol(messages)
	if len(protocolProblems) > 0 {
		logger.Warnf(ctx, "[Agent][Round-%d] Invalid tool protocol after sanitize; dropping tool protocol messages: %v",
			round, protocolProblems)
		messages = agenttools.DropInvalidToolProtocolMessages(messages)
	}
	finalProtocolProblems := agenttools.ValidateToolMessageProtocol(messages)
	if len(finalProtocolProblems) > 0 {
		logger.Errorf(ctx, "[Agent][Round-%d] Invalid tool protocol after final guard: %v",
			round, finalProtocolProblems)
	} else if originalMessageCount != len(messages) || originalToolMessageCount != countToolRoleMessages(messages) || len(protocolProblems) > 0 {
		logger.Infof(ctx, "[Agent][Round-%d] Message sanitize complete: before=%d after=%d tool_before=%d tool_after=%d protocol_problems=%d",
			round, originalMessageCount, len(messages), originalToolMessageCount, countToolRoleMessages(messages), len(protocolProblems))
	}

	// Log message summary; only detail the tail messages to avoid repeating what prior rounds already logged
	const maxDetailMsgs = 4
	logger.Infof(ctx, "[Agent][Round-%d] Calling LLM: %d messages, %d tools",
		round, len(messages), len(tools))
	startIdx := 0
	if len(messages) > maxDetailMsgs {
		startIdx = len(messages) - maxDetailMsgs
		logger.Debugf(ctx, "[Agent][Round-%d] (skipping msg[0..%d], already logged in prior rounds)",
			round, startIdx-1)
	}
	for i := startIdx; i < len(messages); i++ {
		msg := messages[i]
		if msg.Role == "tool" {
			logger.Debugf(ctx, "[Agent][Round-%d] msg[%d]: role=tool, name=%s, len=%d",
				round, i, msg.Name, len(msg.Content))
		} else if len(msg.ToolCalls) > 0 {
			tcNames := make([]string, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				tcNames[j] = tc.Function.Name
			}
			logger.Debugf(ctx, "[Agent][Round-%d] msg[%d]: role=%s, len=%d, tool_calls=%v",
				round, i, msg.Role, len(msg.Content), tcNames)
		} else {
			preview := msg.Content
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			logger.Debugf(ctx, "[Agent][Round-%d] msg[%d]: role=%s, len=%d, content=%s",
				round, i, msg.Role, len(msg.Content), preview)
		}
	}
	common.PipelineInfo(ctx, "Agent", "think_start", map[string]interface{}{
		"iteration":                iteration,
		"round":                    round,
		"tool_cnt":                 len(tools),
		"messages_before_sanitize": originalMessageCount,
		"messages_after_sanitize":  len(messages),
		"tool_messages_before":     originalToolMessageCount,
		"tool_messages_after":      countToolRoleMessages(messages),
		"protocol_problems":        len(protocolProblems) + len(finalProtocolProblems),
	})

	response, err := e.streamThinkingToEventBus(ctx, messages, tools, iteration, sessionID)
	if err != nil && isTransientError(err) {
		// Retry transient errors (timeout, rate limit, server errors) up to maxLLMRetries times
		for retry := 1; retry <= maxLLMRetries; retry++ {
			retryDelay := time.Duration(retry) * time.Second
			logger.Warnf(ctx, "[Agent][Round-%d] LLM transient error (attempt %d/%d), retrying in %v: %v",
				round, retry, maxLLMRetries, retryDelay, err)
			time.Sleep(retryDelay)

			response, err = e.streamThinkingToEventBus(ctx, messages, tools, iteration, sessionID)
			if err == nil || !isTransientError(err) {
				break
			}
		}
	}
	if err != nil {
		logger.Errorf(ctx, "[Agent][Round-%d] LLM call failed: %v", round, err)
		common.PipelineError(ctx, "Agent", "think_failed", map[string]interface{}{
			"iteration": iteration,
			"error":     err.Error(),
		})

		// Graceful degradation: if we have tool results from previous rounds,
		// try to synthesize a final answer from them instead of losing everything.
		if totalTC := countTotalToolCalls(state.RoundSteps); totalTC > 0 {
			logger.Warnf(ctx, "[Agent] LLM failed but have %d steps with %d tool calls — "+
				"attempting final answer synthesis from existing results",
				len(state.RoundSteps), totalTC)
			common.PipelineWarn(ctx, "Agent", "llm_failed_synthesizing", map[string]interface{}{
				"steps":      len(state.RoundSteps),
				"tool_calls": totalTC,
			})
			state.CompletionStatus = "partial"
			state.FinishReason = "fallback_stop"
			state.FailureReason = ""
			state.AllowIndexing = false
			state.AllowComplete = false
			if synthErr := e.streamFinalAnswerToEventBus(ctx, query, state, sessionID); synthErr != nil {
				reason := classifyFinalSynthesisError(synthErr)
				state.CompletionStatus = "failed"
				state.FinishReason = reason
				state.FailureReason = reason
				logger.Errorf(ctx, "[Agent] Final answer synthesis also failed: %v", synthErr)
				return nil, fmt.Errorf("LLM call failed: %w (synthesis also failed: %v)", err, synthErr)
			}
			state.IsComplete = true
			return nil, nil // graceful degradation succeeded
		}

		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	common.PipelineInfo(ctx, "Agent", "think_result", map[string]interface{}{
		"iteration":     iteration,
		"finish_reason": response.FinishReason,
		"tool_calls":    len(response.ToolCalls),
		"content_len":   len(response.Content),
	})

	// Log LLM response summary
	if len(response.ToolCalls) > 0 {
		tcNames := make([]string, len(response.ToolCalls))
		for i, tc := range response.ToolCalls {
			tcNames[i] = tc.Function.Name
		}
		logger.Infof(ctx, "[Agent][Round-%d] LLM responded: finish=%s, content=%d chars, tools=%v",
			round, response.FinishReason, len(response.Content), tcNames)
	} else {
		logger.Infof(ctx, "[Agent][Round-%d] LLM responded: finish=%s, content=%d chars, tool_calls=0",
			round, response.FinishReason, len(response.Content))
		// Early signal for natural-stop path: this round will be analyzed as a
		// likely final answer (no tool call branch).
		if response.FinishReason == "stop" {
			logger.Infof(ctx, "[Agent][Round-%d] Natural-stop candidate detected (finish=stop, tool_calls=0, content=%d chars)",
				round, len(response.Content))
		}
	}
	if response.Content != "" {
		preview := response.Content
		if len(preview) > 300 {
			preview = preview[:300] + "..."
		}
		logger.Debugf(ctx, "[Agent][Round-%d] LLM content preview:\n%s", round, preview)
	}

	return response, nil
}

func countToolRoleMessages(messages []chat.Message) int {
	count := 0
	for _, msg := range messages {
		if msg.Role == "tool" {
			count++
		}
	}
	return count
}
