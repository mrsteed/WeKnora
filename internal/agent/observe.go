package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	agenttoken "github.com/Tencent/WeKnora/internal/agent/token"
	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/common"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

// finalAnswerParseFallback is the user-visible message surfaced when the LLM
// calls final_answer with arguments we cannot recover into an answer string
// (even after RepairJSON + regex fallback). Terminating the loop with this
// message prevents the agent from re-entering and emitting duplicate answers
// on every subsequent round — the behavior reported in issue #1008.
const finalAnswerParseFallback = "Sorry, the model's final answer could not be parsed due to malformed output. Please try again or rephrase your question."

const historyToolSummaryBudget = 900

var finalAnswerFieldPattern = regexp.MustCompile(`(?s)"answer"\s*:\s*"((?:\\.|[^"\\])*)"`)

func parseFinalAnswerArgs(rawArgs string) (string, bool) {
	var payload struct {
		Answer string `json:"answer"`
	}
	if err := json.Unmarshal([]byte(rawArgs), &payload); err == nil {
		if answer := strings.TrimSpace(payload.Answer); answer != "" {
			return answer, true
		}
	}

	repaired := agenttools.RepairJSON(rawArgs)
	if repaired != rawArgs {
		if err := json.Unmarshal([]byte(repaired), &payload); err == nil {
			if answer := strings.TrimSpace(payload.Answer); answer != "" {
				return answer, true
			}
		}
	}

	match := finalAnswerFieldPattern.FindStringSubmatch(rawArgs)
	if len(match) == 2 {
		unquoted, err := strconv.Unquote(`"` + match[1] + `"`)
		if err == nil {
			if answer := strings.TrimSpace(unquoted); answer != "" {
				return answer, true
			}
		}
	}

	return "", false
}

// manageContextWindow consolidates or compresses messages if approaching the token limit.
// currentTokens is the caller's best estimate of the current context size (using
// API-reported Usage when available, falling back to BPE estimation).
func (e *AgentEngine) manageContextWindow(ctx context.Context, messages []chat.Message, round, currentTokens int) []chat.Message {
	if e.config.MaxContextTokens <= 0 {
		return messages
	}

	beforeLen := len(messages)

	if e.memoryConsolidator != nil && e.memoryConsolidator.ShouldConsolidate(currentTokens) {
		logger.Infof(ctx, "[Agent][Round-%d] Token threshold exceeded (est=%d), consolidating memory",
			round, currentTokens)
		consolidated, consolidateErr := e.memoryConsolidator.Consolidate(ctx, messages)
		if consolidateErr != nil {
			logger.Warnf(ctx, "[Agent][Round-%d] Memory consolidation failed: %v, "+
				"falling back to simple compression", round, consolidateErr)
		} else {
			messages = consolidated
			currentTokens = e.tokenEstimator.EstimateMessages(messages)
		}
	}

	messages = agenttoken.CompressContext(messages, e.tokenEstimator, e.config.MaxContextTokens, currentTokens)

	if len(messages) < beforeLen {
		logger.Infof(ctx, "[Agent][Round-%d] Context managed: %d → %d messages (max_tokens=%d)",
			round, beforeLen, len(messages), e.config.MaxContextTokens)
	}

	return messages
}

// responseVerdict captures the result of analyzing an LLM response to determine
// whether the agent loop should stop and what the final answer is (if any).
type responseVerdict struct {
	isDone           bool
	finalAnswer      string
	emptyContent     bool // LLM returned stop with no tool calls and empty content
	completionStatus string
	finishReason     string
	isPartial        bool
	allowIndexing    bool
	allowComplete    bool
	failureReason    string
	step             types.AgentStep
}

// analyzeResponse inspects the LLM response for stop conditions:
//   - finish_reason == "stop" with no tool calls → agent is done (natural stop)
//   - finish_reason == "content_filter" with no tool calls → agent is done (content filtered)
//
// The agent ends a turn by stopping naturally with its answer as plain
// assistant text (there is no dedicated final_answer tool). Any round that
// still requests tool calls is non-terminal and the caller continues the loop.
// It returns a responseVerdict. If isDone is true the caller should break out of the loop.
func (e *AgentEngine) analyzeResponse(
	ctx context.Context, response *types.ChatResponse,
	step types.AgentStep, iteration int, sessionID string, roundStart time.Time,
) responseVerdict {
	// Case 0: Content was blocked by the model's content filter.
	// Treat this as a terminal condition to avoid an infinite loop where
	// the same filtered response accumulates in the context.
	if response.FinishReason == "content_filter" && len(response.ToolCalls) == 0 {
		logger.Warnf(ctx, "[Agent][Round-%d] Content filter triggered, stopping agent loop (content=%d chars)",
			iteration+1, len(response.Content))
		common.PipelineWarn(ctx, "Agent", "content_filter_stop", map[string]interface{}{
			"iteration":   iteration,
			"round":       iteration + 1,
			"content_len": len(response.Content),
		})

		answer := response.Content
		if answer == "" {
			answer = "Sorry, this request was blocked by the content safety policy. Please try rephrasing your question."
		}

		answerID := generateEventID("answer")
		if !response.FinalAnswerStreamed && answer != "" {
			e.eventBus.Emit(ctx, event.Event{
				ID:        answerID,
				Type:      event.EventAgentFinalAnswer,
				SessionID: sessionID,
				Data: event.AgentFinalAnswerData{
					Content:          answer,
					Done:             false,
					CompletionStatus: "failed",
					FinishReason:     "content_filter",
					AllowIndexing:    false,
					AllowComplete:    false,
					FailureReason:    "content_filter",
				},
			})
		}
		e.eventBus.Emit(ctx, event.Event{
			ID:        answerID,
			Type:      event.EventAgentFinalAnswer,
			SessionID: sessionID,
			Data: event.AgentFinalAnswerData{
				Content:          "",
				Done:             true,
				CompletionStatus: "failed",
				FinishReason:     "content_filter",
				AllowIndexing:    false,
				AllowComplete:    false,
				FailureReason:    "content_filter",
			},
		})

		return responseVerdict{
			isDone:           true,
			finalAnswer:      answer,
			completionStatus: "failed",
			finishReason:     "content_filter",
			allowIndexing:    false,
			allowComplete:    false,
			failureReason:    "content_filter",
			step:             step,
		}
	}

	if response.FinishReason == "length" && len(response.ToolCalls) == 0 {
		response.Content = agenttools.StripThinkBlocks(response.Content)
		logger.Warnf(ctx, "[Agent][Round-%d] Agent response truncated by length: answer=%d chars, duration=%dms",
			iteration+1, len(response.Content), time.Since(roundStart).Milliseconds())
		common.PipelineWarn(ctx, "Agent", "round_partial_length", map[string]interface{}{
			"iteration":     iteration,
			"round":         iteration + 1,
			"answer_len":    len(response.Content),
			"finish_reason": response.FinishReason,
		})

		answerID := generateEventID("answer")
		if !response.AnswerStreamed && response.Content != "" {
			e.eventBus.Emit(ctx, event.Event{
				ID:        answerID,
				Type:      event.EventAgentFinalAnswer,
				SessionID: sessionID,
				Data: event.AgentFinalAnswerData{
					Content:          response.Content,
					Done:             false,
					CompletionStatus: "partial",
					FinishReason:     "length",
					IsPartial:        true,
					AllowIndexing:    false,
					AllowComplete:    false,
					FailureReason:    "length",
				},
			})
		}
		e.eventBus.Emit(ctx, event.Event{
			ID:        answerID,
			Type:      event.EventAgentFinalAnswer,
			SessionID: sessionID,
			Data: event.AgentFinalAnswerData{
				Content:          "",
				Done:             true,
				CompletionStatus: "partial",
				FinishReason:     "length",
				IsPartial:        true,
				AllowIndexing:    false,
				AllowComplete:    false,
				FailureReason:    "length",
			},
		})

		return responseVerdict{
			isDone:           true,
			finalAnswer:      response.Content,
			completionStatus: "partial",
			finishReason:     "length",
			isPartial:        true,
			allowIndexing:    false,
			allowComplete:    false,
			failureReason:    "length",
			step:             step,
		}
	}

	if response.FinishReason == "stream_error_after_answer" && response.FinalAnswerStreamed && response.AnswerStreamed && len(response.ToolCalls) == 0 {
		response.Content = agenttools.StripThinkBlocks(response.Content)
		logger.Warnf(ctx, "[Agent][Round-%d] final_answer stream ended with tail error after %d chars; stopping loop as partial",
			iteration+1, len(response.Content))
		common.PipelineWarn(ctx, "Agent", "final_answer_stream_error_stop", map[string]interface{}{
			"iteration":     iteration,
			"round":         iteration + 1,
			"answer_len":    len(response.Content),
			"finish_reason": response.FinishReason,
		})

		answerID := generateEventID("answer")
		e.eventBus.Emit(ctx, event.Event{
			ID:        answerID,
			Type:      event.EventAgentFinalAnswer,
			SessionID: sessionID,
			Data: event.AgentFinalAnswerData{
				Content:          "",
				Done:             true,
				CompletionStatus: "partial",
				FinishReason:     "stream_error_after_answer",
				IsPartial:        true,
				AllowIndexing:    false,
				AllowComplete:    false,
				FailureReason:    "stream_error_after_answer",
			},
		})

		return responseVerdict{
			isDone:           true,
			finalAnswer:      response.Content,
			completionStatus: "partial",
			finishReason:     "stream_error_after_answer",
			isPartial:        true,
			allowIndexing:    false,
			allowComplete:    false,
			failureReason:    "stream_error_after_answer",
			step:             step,
		}
	}

	if response.FinishReason == duplicateDocumentHeadFinishReason && len(response.ToolCalls) == 0 {
		logger.Warnf(ctx, "[Agent][Round-%d] duplicate document head detected in streamed answer; stopping loop as partial",
			iteration+1)
		common.PipelineWarn(ctx, "Agent", "duplicate_document_head_stop", map[string]interface{}{
			"iteration":     iteration,
			"round":         iteration + 1,
			"finish_reason": response.FinishReason,
		})

		return responseVerdict{
			isDone:           true,
			finalAnswer:      response.Content,
			completionStatus: types.MessageCompletionStatusPartial,
			finishReason:     duplicateDocumentHeadFinishReason,
			isPartial:        true,
			allowIndexing:    false,
			allowComplete:    false,
			failureReason:    duplicateDocumentHeadFinishReason,
			step:             step,
		}
	}

	// Case 1: LLM stopped naturally without requesting any tool calls
	if response.FinishReason == "stop" && len(response.ToolCalls) == 0 {
		// Strip <think>…</think> blocks that some models embed in content
		// (DeepSeek, Qwen, etc.) before processing or displaying.
		response.Content = agenttools.StripThinkBlocks(response.Content)
		logger.Infof(ctx, "[Agent][Round-%d] Agent finished naturally: answer=%d chars, duration=%dms",
			iteration+1, len(response.Content), time.Since(roundStart).Milliseconds())
		common.PipelineInfo(ctx, "Agent", "round_final_answer", map[string]interface{}{
			"iteration":  iteration,
			"round":      iteration + 1,
			"answer_len": len(response.Content),
		})

		// Emit the final answer. Reuse the live stream's event ID when the
		// answer was already streamed during the think phase; otherwise emit the
		// accumulated content once before closing the stream.
		// Emit the final answer. The answer text reaches the UI by one of two
		// paths:
		//   (a) Already streamed live during the think phase — the common case
		//       now that plain assistant content is routed straight to
		//       EventAgentFinalAnswer (response.AnswerStreamed). Re-emitting the
		//       full content here would render it twice and produce the
		//       end-of-stream "jump from Thinking to Answer" the user reported,
		//       so we only close the existing stream with a Done marker on the
		//       same event ID.
		//   (b) Not streamed live (e.g. the content only surfaced in the
		//       accumulated result) — emit the full content, then Done.
		var answerID string
		if response.AnswerStreamed {
			answerID = response.AnswerEventID
			if answerID == "" {
				answerID = generateEventID("answer")
			}
		} else {
			answerID = generateEventID("answer")
			if response.Content != "" {
				e.eventBus.Emit(ctx, event.Event{
					ID:        answerID,
					Type:      event.EventAgentFinalAnswer,
					SessionID: sessionID,
					Data: event.AgentFinalAnswerData{
						Content:          response.Content,
						Done:             false,
						CompletionStatus: "completed",
						FinishReason:     "stop",
						AllowIndexing:    true,
						AllowComplete:    true,
					},
				})
			}
		}
		e.eventBus.Emit(ctx, event.Event{
			ID:        answerID,
			Type:      event.EventAgentFinalAnswer,
			SessionID: sessionID,
			Data: event.AgentFinalAnswerData{
				Content:          "",
				Done:             true,
				CompletionStatus: "completed",
				FinishReason:     "stop",
				AllowIndexing:    true,
				AllowComplete:    true,
			},
		})

		return responseVerdict{
			isDone:           true,
			finalAnswer:      response.Content,
			emptyContent:     response.Content == "",
			completionStatus: "completed",
			finishReason:     "stop",
			allowIndexing:    true,
			allowComplete:    true,
			step:             step,
		}
	}

	// Case 2: final_answer tool call present.
	//
	// final_answer is always a terminal signal: regardless of whether we can
	// parse its arguments, we must end the ReAct loop here. Otherwise the LLM
	// will see the tool result in the next round, re-invoke final_answer with
	// near-identical content, and surface duplicate answers to the user (see
	// issue #1008). Parse with three levels of tolerance:
	//
	//   1. strict json.Unmarshal
	//   2. RepairJSON + Unmarshal
	//   3. regex best-effort extraction of the "answer" field
	//
	// If all three fail, terminate with a user-visible fallback message.
	if len(response.ToolCalls) > 0 {
		for _, tc := range response.ToolCalls {
			if tc.Function.Name != agenttools.ToolFinalAnswer {
				continue
			}

			rawArgs := tc.Function.Arguments
			answer, ok := parseFinalAnswerArgs(rawArgs)
			recovered := false
			if !ok {
				// Could not recover any answer text — fall back to a generic
				// message so the user doesn't see a blank response.
				logger.Warnf(ctx, "[Agent][Round-%d] Failed to parse final_answer args (args=%q) — "+
					"terminating loop with fallback message",
					iteration+1, rawArgs)
				answer = finalAnswerParseFallback
			} else {
				recovered = true
				logger.Infof(ctx, "[Agent][Round-%d] final_answer tool: answer=%d chars, duration=%dms",
					iteration+1, len(answer), time.Since(roundStart).Milliseconds())
			}

			answerID := generateEventID("answer-done")
			if response.FinalAnswerStreamed {
				answerID = response.AnswerEventID
				if answerID == "" {
					answerID = generateEventID("answer-done")
				}
			}
			if !response.FinalAnswerStreamed && answer != "" {
				e.eventBus.Emit(ctx, event.Event{
					ID:        answerID,
					Type:      event.EventAgentFinalAnswer,
					SessionID: sessionID,
					Data: event.AgentFinalAnswerData{
						Content:          answer,
						Done:             false,
						CompletionStatus: "completed",
						FinishReason:     response.FinishReason,
						AllowIndexing:    true,
						AllowComplete:    true,
					},
				})
			}
			e.eventBus.Emit(ctx, event.Event{
				ID:        answerID,
				Type:      event.EventAgentFinalAnswer,
				SessionID: sessionID,
				Data: event.AgentFinalAnswerData{
					Content:          "",
					Done:             true,
					CompletionStatus: "completed",
					FinishReason:     response.FinishReason,
					AllowIndexing:    true,
					AllowComplete:    true,
				},
			})

			pipelineFields := map[string]interface{}{
				"iteration":  iteration,
				"round":      iteration + 1,
				"answer_len": len(answer),
				"recovered":  recovered,
			}
			if recovered {
				common.PipelineInfo(ctx, "Agent", "final_answer_tool", pipelineFields)
			} else {
				pipelineFields["raw_args"] = rawArgs
				common.PipelineWarn(ctx, "Agent", "final_answer_tool_parse_failed", pipelineFields)
			}

			return responseVerdict{
				isDone:           true,
				finalAnswer:      answer,
				completionStatus: "completed",
				finishReason:     response.FinishReason,
				allowIndexing:    true,
				allowComplete:    true,
				step:             step,
			}
		}
	}

	// Any round that still requests tool calls is non-terminal: the caller
	// executes the tools and loops again. The agent only ends by stopping
	// naturally (Case 1) with its answer as plain assistant text.
	return responseVerdict{isDone: false, step: step}
}

// indentLines prefixes every line of s with indent. Used to nest pre-rendered
// XML blocks inside the `<runtime_context>` envelope without losing readability.
func indentLines(s, indent string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

// escapeXMLAttr escapes a string for safe inclusion in an XML attribute value.
// Titles and names may contain user-supplied characters like <, >, &, ".
func escapeXMLAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// buildRuntimeContextBlock builds a metadata block with current time, session
// info, and the *active retrieval scope for this turn*. The scope snapshot is
// critical for multi-turn correctness: when the user switches their @mention
// to a different KB or document between turns, earlier turns still carry
// their own scope snapshot in history, so the model can see the scope change
// and avoid reusing last turn's answer against the new scope.
//
// The detailed bound-KB metadata (capabilities, recent documents, summaries)
// also lives here — it is turn state, not instructions, so it belongs next
// to the user query rather than baked into the system prompt. Keeping it in
// the user message keeps the system prompt stable/cacheable and lets the
// model see exactly which KBs were in scope at the time of each historical
// turn.
//
// Per-turn communication_instruction and answer_instruction remind the model
// not to leak internal tool names or IDs in user-visible text, and to end the
// turn by writing its complete answer as plain assistant text.
//
// Emitted as an XML-ish block (not free prose) so it is a visually distinct,
// non-instruction envelope that is hard to conflate with user text and
// prompt-injection-safe.
func buildRuntimeContextBlock(
	sessionID string,
	kbs []*KnowledgeBaseInfo,
	docs []*SelectedDocumentInfo,
) string {
	var sb strings.Builder
	sb.WriteString("<runtime_context note=\"turn metadata; follow communication_instruction and answer_instruction\">\n")
	fmt.Fprintf(&sb, "  <current_time>%s</current_time>\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&sb, "  <session>%s</session>\n", escapeXMLAttr(sessionID))

	if len(kbs) > 0 {
		// Render the full bound-KB detail (capabilities + recent docs) so the
		// model has everything it needs to route its retrieval in one place.
		// `formatKnowledgeBaseList` already emits a `<knowledge_bases>…</knowledge_bases>`
		// envelope; we wrap it in `<bound_knowledge_bases>` to make the scope
		// semantics explicit and to match the naming the prompt templates use
		// when referring back to this block.
		sb.WriteString("  <bound_knowledge_bases>\n")
		sb.WriteString(indentLines(formatKnowledgeBaseList(kbs), "    "))
		sb.WriteString("\n  </bound_knowledge_bases>\n")
	}

	if len(docs) > 0 {
		sb.WriteString("  <pinned_documents scope=\"authoritative_for_this_turn\">\n")
		for _, d := range docs {
			if d == nil {
				continue
			}
			title := d.Title
			if title == "" {
				title = d.FileName
			}
			if title == "" {
				title = d.KnowledgeID
			}
			fmt.Fprintf(&sb, "    <document knowledge_id=\"%s\" title=\"%s\" />\n",
				escapeXMLAttr(d.KnowledgeID), escapeXMLAttr(title))
		}
		sb.WriteString("  </pinned_documents>\n")
		sb.WriteString("  <note>The pinned-document set above is authoritative for THIS turn. If an earlier turn in this conversation analysed a different document, do NOT reuse that analysis — re-query against the current scope.</note>\n")
	}

	sb.WriteString("  <communication_instruction>Do not use internal tool names or identifiers in your answers or in Thought. Say \"keyword retrieval\" instead of grep_chunks, \"semantic retrieval\" instead of knowledge_search, \"browse full document\" instead of list_knowledge_chunks; likewise never expose chunk_id, knowledge_id, or other internal IDs—refer to documents by title or name.</communication_instruction>\n")
	sb.WriteString("  <answer_instruction>When you have gathered enough information, write your complete user-facing answer as your reply and stop—do not request any more tools in that final message. Until then, keep using tools; do not give a partial answer mid-investigation.</answer_instruction>\n")

	sb.WriteString("</runtime_context>")
	return sb.String()
}

// listToolNames returns tool.function names for logging
func listToolNames(ts []chat.Tool) []string {
	names := make([]string, 0, len(ts))
	for _, t := range ts {
		names = append(names, t.Function.Name)
	}
	return names
}

// buildToolsForLLM builds the tools list for LLM function calling
func (e *AgentEngine) buildToolsForLLM() []chat.Tool {
	functionDefs := e.toolRegistry.GetFunctionDefinitions()
	tools := make([]chat.Tool, 0, len(functionDefs))
	for _, def := range functionDefs {
		tools = append(tools, chat.Tool{
			Type: "function",
			Function: chat.FunctionDef{
				Name:        def.Name,
				Description: def.Description,
				Parameters:  def.Parameters,
			},
		})
	}

	return tools
}

// appendToolResults adds tool results to the in-turn message history following
// OpenAI's tool-calling format. Cross-turn persistence is handled separately:
// the final AgentSteps are written to the assistant message by the SSE handler,
// and rebuilt from DB on the next turn by service.LoadAgentHistory.
func (e *AgentEngine) appendToolResults(
	messages []chat.Message,
	step types.AgentStep,
) []chat.Message {
	contextMessages := make([]chat.Message, 0, len(step.ToolCalls)+1)

	// Add assistant message with tool calls (if any)
	if step.Thought != "" || len(step.ToolCalls) > 0 || step.ReasoningContent != "" {
		assistantMsg := chat.Message{
			Role:             "assistant",
			Content:          step.Thought,
			ReasoningContent: step.ReasoningContent,
		}

		// Add tool calls to assistant message (following OpenAI format)
		if len(step.ToolCalls) > 0 {
			assistantMsg.ToolCalls = make([]chat.ToolCall, 0, len(step.ToolCalls))
			for _, tc := range step.ToolCalls {
				// Convert arguments back to JSON string
				argsJSON, _ := json.Marshal(tc.Args)

				assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, chat.ToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: chat.FunctionCall{
						Name:      tc.Name,
						Arguments: string(argsJSON),
					},
				})
			}
		}

		messages = append(messages, assistantMsg)
		contextMessages = append(contextMessages, assistantMsg)
	}

	// Add tool result messages (role: "tool", following OpenAI format)
	for _, toolCall := range step.ToolCalls {
		resultContent := toolResultMessageContent(toolCall.Result)

		toolMsg := chat.Message{
			Role:       "tool",
			Content:    resultContent,
			ToolCallID: toolCall.ID,
			Name:       toolCall.Name,
		}

		messages = append(messages, toolMsg)

		contextToolMsg := toolMsg
		if e.config == nil || !e.config.RetainRetrievalHistory {
			contextToolMsg.Content = summarizeToolResultForHistory(toolCall)
		}
		contextMessages = append(contextMessages, contextToolMsg)
	}

	if len(contextMessages) > 0 {
		e.pendingContextGroups = append(e.pendingContextGroups, contextMessages...)
		logger.Debugf(context.Background(), "[Agent] Buffered %d tool-group message(s) for deferred context persistence (session: %s)", len(contextMessages), e.sessionID)
	}

	return messages
}

func toolResultMessageContent(result *types.ToolResult) string {
	if result == nil {
		return "Error: tool returned no result"
	}
	if !result.Success {
		if result.Error != "" {
			return fmt.Sprintf("Error: %s", result.Error)
		}
		return "Error: tool execution failed"
	}
	return result.Output
}

// countTotalToolCalls counts total tool calls across all steps
func countTotalToolCalls(steps []types.AgentStep) int {
	total := 0
	for _, step := range steps {
		total += len(step.ToolCalls)
	}
	return total
}

// kbToolNames lists tools whose results contain knowledge base content that
// may become stale across turns (KB can be switched, updated, or deleted).
// Historical results from these tools are redacted to force fresh retrieval.
var kbToolNames = map[string]bool{
	agenttools.ToolKnowledgeSearch:        true,
	agenttools.ToolGrepChunks:             true,
	agenttools.ToolListKnowledgeChunks:    true,
	agenttools.ToolQueryKnowledgeGraph:    true,
	agenttools.ToolGetDocumentInfo:        true,
	agenttools.ToolExternalDatabaseSchema: true,
	agenttools.ToolExternalDatabaseQuery:  true,
	agenttools.ToolWikiSearch:             true,
	agenttools.ToolWikiReadPage:           true,
	agenttools.ToolWikiReadSourceDoc:      true,
}

func isHistoricalDatabaseTool(toolName string) bool {
	switch toolName {
	case agenttools.ToolExternalDatabaseSchema, agenttools.ToolExternalDatabaseQuery:
		return true
	default:
		return false
	}
}

func summarizeToolResultForHistory(toolCall types.ToolCall) string {
	if toolCall.Result == nil {
		return "(no tool result)"
	}
	if !isHistoricalDatabaseTool(toolCall.Name) {
		if !toolCall.Result.Success && toolCall.Result.Error != "" {
			return fmt.Sprintf("Error: %s", toolCall.Result.Error)
		}
		return toolCall.Result.Output
	}

	body := ""
	if structured := summarizeStructuredToolResult(toolCall.Name, toolCall.Result); structured != "" {
		body = structured
	} else if !toolCall.Result.Success && toolCall.Result.Error != "" {
		body = fmt.Sprintf("Error: %s", toolCall.Result.Error)
	} else {
		body = toolCall.Result.Output
	}
	body = compactToolTextForFinalAnswer(body, historyToolSummaryBudget)

	header := "Historical database tool summary"
	switch toolCall.Name {
	case agenttools.ToolExternalDatabaseSchema:
		header = "Historical database schema summary"
	case agenttools.ToolExternalDatabaseQuery:
		header = "Historical database query summary"
	}

	return strings.TrimSpace(header + "\n" + body + "\nDatabase state may have changed. Re-run this tool before relying on the result.")
}

func redactHistoricalDatabaseToolContent(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return "[Historical database result omitted — database state may have changed. Please perform a fresh schema/query call.]"
	}
	content = compactToolTextForFinalAnswer(content, historyToolSummaryBudget)
	if strings.Contains(content, "Database state may have changed.") {
		return content
	}
	return strings.TrimSpace(content + "\nDatabase state may have changed. Perform a fresh schema/query call before relying on this historical result.")
}

// redactHistoryKBResults replaces full KB tool results in historical context
// with brief markers. This prevents the LLM from reusing stale retrieval data
// when the knowledge base has been modified or switched between turns.
func redactHistoryKBResults(llmContext []chat.Message) []chat.Message {
	redacted := make([]chat.Message, 0, len(llmContext))
	for _, msg := range llmContext {
		if msg.Role == "tool" && kbToolNames[msg.Name] {
			if isHistoricalDatabaseTool(msg.Name) {
				redacted = append(redacted, chat.Message{
					Role:       msg.Role,
					Content:    redactHistoricalDatabaseToolContent(msg.Content),
					ToolCallID: msg.ToolCallID,
					Name:       msg.Name,
				})
				continue
			}
			redacted = append(redacted, chat.Message{
				Role:       msg.Role,
				Content:    "[Previous retrieval result omitted — knowledge base may have changed. Please perform a fresh search.]",
				ToolCallID: msg.ToolCallID,
				Name:       msg.Name,
			})
		} else {
			redacted = append(redacted, msg)
		}
	}
	return redacted
}

// buildMessagesWithLLMContext builds the message array with LLM context
func (e *AgentEngine) buildMessagesWithLLMContext(
	systemPrompt, currentQuery, sessionID string,
	llmContext []chat.Message,
	imageURLs []string,
) []chat.Message {
	messages := []chat.Message{
		{Role: "system", Content: systemPrompt},
	}

	if len(llmContext) > 0 {
		var sanitized []chat.Message
		if e.config != nil && e.config.RetainRetrievalHistory {
			sanitized = llmContext
			logger.Infof(context.Background(), "Retaining full retrieval history in context (RetainRetrievalHistory=true)")
		} else {
			// Redact KB tool results from previous turns to prevent the LLM
			// from reusing stale retrieval data when the KB has been modified.
			sanitized = redactHistoryKBResults(llmContext)
			logger.Infof(context.Background(), "Added %d history messages to context (KB tool history sanitized)", len(llmContext))
		}

		for _, msg := range sanitized {
			if msg.Role == "system" {
				continue
			}
			if msg.Role == "assistant" && len(msg.ToolCalls) > 0 && msg.ReasoningContent == "" && msg.Content != "" {
				// Backfill reasoning_content for older cached history that was
				// persisted before the DeepSeek thinking replay fix.
				msg.ReasoningContent = msg.Content
			}
			if msg.Role == "user" || msg.Role == "assistant" || msg.Role == "tool" {
				messages = append(messages, msg)
			}
		}
	}

	// Build user message with runtime context safety tag.
	// The runtime context carries a per-turn scope snapshot so that multi-turn
	// history preserves the (kb, pinned docs) that each earlier turn ran under;
	// this is what lets the model detect a scope switch instead of silently
	// answering the new question against last turn's retrieval.
	runtimeCtx := buildRuntimeContextBlock(sessionID, e.knowledgeBasesInfo, e.selectedDocs)
	userMsg := chat.Message{
		Role:    "user",
		Content: runtimeCtx + "\n\n" + currentQuery,
		Images:  imageURLs,
	}
	messages = append(messages, userMsg)

	return messages
}
