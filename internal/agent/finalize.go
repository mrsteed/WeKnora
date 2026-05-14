package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/common"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	finalAnswerToolOutputBudget   = 1800
	finalAnswerRowPreviewLimit    = 3
	finalAnswerColumnPreviewLimit = 6
	finalAnswerQueryCharBudget    = 2400
	finalAnswerToolSummaryBudget  = 4000
)

type completionEventState struct {
	completionStatus string
	finishReason     string
	failureReason    string
	allowIndexing    bool
	allowComplete    bool
}

func normalizeCompletionEventState(state *types.AgentState) completionEventState {
	completionStatus := state.CompletionStatus
	finishReason := state.FinishReason
	failureReason := state.FailureReason
	allowIndexing := state.AllowIndexing
	allowComplete := state.AllowComplete

	if state.FinalAnswerSynthesized && strings.TrimSpace(state.FinalAnswer) != "" &&
		completionStatus == types.MessageCompletionStatusFailed &&
		(finishReason == "tool_error" || failureReason == "tool_error") {
		completionStatus = types.MessageCompletionStatusPartial
		finishReason = "fallback_stop"
		failureReason = ""
		allowIndexing = false
		allowComplete = false
	}

	if completionStatus == "" {
		completionStatus = types.MessageCompletionStatusCompleted
	}
	if finishReason == "" {
		finishReason = "stop"
	}
	if completionStatus == types.MessageCompletionStatusCompleted && finishReason == "stop" && !allowIndexing && !allowComplete {
		allowIndexing = true
		allowComplete = true
	}

	return completionEventState{
		completionStatus: completionStatus,
		finishReason:     finishReason,
		failureReason:    failureReason,
		allowIndexing:    allowIndexing,
		allowComplete:    allowComplete,
	}
}

func buildFinalAnswerEventData(state *types.AgentState, content string, done bool) event.AgentFinalAnswerData {
	normalized := normalizeCompletionEventState(state)

	return event.AgentFinalAnswerData{
		Content:          content,
		Done:             done,
		CompletionStatus: normalized.completionStatus,
		FinishReason:     normalized.finishReason,
		IsPartial:        normalized.completionStatus == types.MessageCompletionStatusPartial,
		AllowIndexing:    normalized.allowIndexing,
		AllowComplete:    normalized.allowComplete,
		FailureReason:    normalized.failureReason,
	}
}

func buildFinalSynthesisSystemPrompt(language string, documentContext *types.AgentDocumentContext) string {
	var builder strings.Builder
	builder.WriteString("You are a final answer synthesizer. Use only the provided user goal, document context, and tool summaries. ")
	builder.WriteString("Do not call tools. Do not output hidden reasoning. Keep the answer directly usable. ")
	if strings.TrimSpace(language) != "" {
		builder.WriteString(fmt.Sprintf("Respond in %s. ", language))
	}
	if documentContext != nil && documentContext.Intent == types.ChatDocumentIntentRevise {
		builder.WriteString("For document revision tasks, prefer outputting a structured <document_patch> instead of repeating the full document. ")
	}
	return strings.TrimSpace(builder.String())
}

func isDocumentContinuationFinalSynthesis(documentContext *types.AgentDocumentContext) bool {
	if documentContext == nil {
		return false
	}
	return documentContext.AutoContinue ||
		documentContext.Intent == types.ChatDocumentIntentContinue ||
		documentContext.Operation == types.ChatDocumentOperationContinue
}

func documentCompletionMarker(documentContext *types.AgentDocumentContext) string {
	if documentContext != nil && strings.TrimSpace(documentContext.CompletionMarker) != "" {
		return strings.TrimSpace(documentContext.CompletionMarker)
	}
	return types.ChatDocumentCompletionMarker
}

func buildDocumentContinuationSynthesisProtocol(documentContext *types.AgentDocumentContext) string {
	if !isDocumentContinuationFinalSynthesis(documentContext) {
		return ""
	}
	marker := documentCompletionMarker(documentContext)
	return fmt.Sprintf(`
Document continuation requirements:
1. The current task is continuing the same document, not regenerating a full new document.
2. Output only the missing new body text after the existing base document.
3. Do not repeat the document title, top-level headings, already completed sections, or paragraphs already present near the end of the base document.
4. Preserve the existing heading hierarchy, numbering style, terminology, table style, and Markdown style.
5. If the whole document is already complete, append %s on its own line at the end of the answer.
6. If there is no new content to add, output only %s.
7. Do not explain that you are continuing; output document body content directly.`, marker, marker)
}

func buildDocumentRevisionSynthesisProtocol(documentContext *types.AgentDocumentContext) string {
	if documentContext == nil {
		return ""
	}
	if documentContext.Intent != types.ChatDocumentIntentRevise && documentContext.Operation != types.ChatDocumentOperationRevise {
		return ""
	}
	return `
Document revision requirements:
1. Modify the existing document instead of regenerating a full new version.
2. Prefer outputting a <document_patch> envelope with replace/append/insert_after operations.
3. Do not repeat unchanged sections.
4. Keep heading hierarchy, numbering, terminology, and Markdown style consistent.
5. If the available context is insufficient for a safe patch, clearly state the missing section boundary instead of inventing content.`
}

func appendDocumentContinuationContext(messages []chat.Message, query string, documentContext *types.AgentDocumentContext) []chat.Message {
	if !isDocumentContinuationFinalSynthesis(documentContext) {
		return messages
	}
	quotedContext := strings.TrimSpace(documentContext.QuotedContext)
	if quotedContext == "" || strings.Contains(query, quotedContext) {
		return messages
	}
	return append(messages, chat.Message{
		Role:    "user",
		Content: "Document continuation context:\n" + quotedContext,
	})
}

func compactQueryForFinalSynthesis(query string, documentContext *types.AgentDocumentContext) string {
	if documentContext != nil && strings.TrimSpace(documentContext.UserGoal) != "" {
		return compactToolTextForFinalAnswer(strings.TrimSpace(documentContext.UserGoal), finalAnswerQueryCharBudget)
	}
	query = stripTaggedBlock(query, "document_revision_context")
	query = stripTaggedBlock(query, "document_continuation_context")
	query = stripTaggedBlock(query, "runtime_context")
	query = strings.TrimSpace(query)
	if query == "" {
		return "Summarize the collected results into a final answer."
	}
	return compactToolTextForFinalAnswer(query, finalAnswerQueryCharBudget)
}

func stripTaggedBlock(content string, tag string) string {
	if strings.TrimSpace(content) == "" || strings.TrimSpace(tag) == "" {
		return strings.TrimSpace(content)
	}
	openTag := "<" + tag
	closeTag := "</" + tag + ">"
	for {
		start := strings.Index(content, openTag)
		if start < 0 {
			break
		}
		end := strings.Index(content[start:], closeTag)
		if end < 0 {
			content = content[:start]
			break
		}
		end += start + len(closeTag)
		content = content[:start] + content[end:]
	}
	return strings.TrimSpace(content)
}

func buildFinalSynthesisToolMessages(state *types.AgentState) ([]chat.Message, int) {
	if state == nil {
		return nil, 0
	}
	remainingBudget := finalAnswerToolSummaryBudget
	messages := make([]chat.Message, 0)
	toolResultCount := 0
	omittedCount := 0
	for _, step := range state.RoundSteps {
		for _, toolCall := range step.ToolCalls {
			toolResultCount++
			if remainingBudget <= 0 {
				omittedCount++
				continue
			}
			summary := summarizeToolResultForFinalAnswer(toolCall)
			budget := minInt(finalAnswerToolOutputBudget, remainingBudget)
			summary = compactToolTextForFinalAnswer(summary, budget)
			remainingBudget -= len([]rune(summary))
			messages = append(messages, chat.Message{
				Role:    "user",
				Content: fmt.Sprintf("Tool %s summary:\n%s", toolCall.Name, summary),
			})
		}
	}
	if omittedCount > 0 {
		messages = append(messages, chat.Message{
			Role:    "user",
			Content: fmt.Sprintf("Additional tool results omitted to stay within synthesis budget: %d", omittedCount),
		})
	}
	return messages, toolResultCount
}

func buildFinalSynthesisBudgetFallback(language string, documentContext *types.AgentDocumentContext) string {
	isChinese := strings.Contains(strings.ToLower(language), "chinese") || language == ""
	if documentContext != nil && documentContext.Intent == types.ChatDocumentIntentRevise {
		if isChinese {
			return "已获取中间结果，但当前文档修订上下文仍然过长，无法在安全预算内完成最终合成。请缩小到具体章节、段落或拆成多次修改后重试。"
		}
		return "Intermediate results are available, but the document revision context is still too large to synthesize safely. Please narrow the request to specific sections or split it into smaller edits."
	}
	if isChinese {
		return "已获取中间结果，但当前上下文仍然过长，无法在安全预算内完成最终合成。请缩小问题范围后重试。"
	}
	return "Intermediate results are available, but the context is still too large to synthesize safely. Please narrow the request and try again."
}

func (e *AgentEngine) emitStaticFinalAnswer(ctx context.Context, sessionID string, state *types.AgentState, content string) {
	answerID := generateEventID("answer")
	e.eventBus.Emit(ctx, event.Event{
		ID:        answerID,
		Type:      event.EventAgentFinalAnswer,
		SessionID: sessionID,
		Data:      buildFinalAnswerEventData(state, content, false),
	})
	e.eventBus.Emit(ctx, event.Event{
		ID:        answerID,
		Type:      event.EventAgentFinalAnswer,
		SessionID: sessionID,
		Data:      buildFinalAnswerEventData(state, "", true),
	})
}

func buildFinalAnswerSynthesisPrompt(query string, state *types.AgentState) string {
	documentProtocol := ""
	if state != nil {
		documentProtocol = buildDocumentContinuationSynthesisProtocol(state.DocumentContext) + buildDocumentRevisionSynthesisProtocol(state.DocumentContext)
	}

	return fmt.Sprintf(`Based on the above tool call results, generate a complete answer for the user's question.%s

User question: %s

Requirements:
1. Answer based on the actually retrieved content
2. Clearly cite information sources (chunk_id, document name)
3. Organize the answer in a structured format
4. If information is insufficient, honestly state so
5. IMPORTANT: Respond in the same language as the user's question

Now generate the final answer:`, documentProtocol, query)
}

// streamFinalAnswerToEventBus streams the final answer generation through EventBus
func (e *AgentEngine) streamFinalAnswerToEventBus(
	ctx context.Context,
	query string,
	state *types.AgentState,
	sessionID string,
) error {
	totalToolCalls := countTotalToolCalls(state.RoundSteps)
	logger.Infof(ctx, "[Agent][FinalAnswer] Synthesizing from %d steps, %d tool calls",
		len(state.RoundSteps), totalToolCalls)
	common.PipelineInfo(ctx, "Agent", "final_answer_start", map[string]interface{}{
		"session_id":   sessionID,
		"query":        query,
		"steps":        len(state.RoundSteps),
		"tool_results": totalToolCalls,
	})

	// Build a lightweight synthesis context instead of reusing the full agent system prompt.
	language := types.LanguageNameFromContext(ctx)
	var documentContext *types.AgentDocumentContext
	if state != nil {
		documentContext = state.DocumentContext
	}
	compactQuery := compactQueryForFinalSynthesis(query, documentContext)
	systemPrompt := buildFinalSynthesisSystemPrompt(language, documentContext)

	messages := []chat.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: "User goal:\n" + compactQuery},
	}
	messages = appendDocumentContinuationContext(messages, compactQuery, documentContext)

	toolMessages, toolResultCount := buildFinalSynthesisToolMessages(state)
	messages = append(messages, toolMessages...)

	logger.Debugf(ctx, "[Agent][FinalAnswer] Built context: %d messages, %d tool results",
		len(messages), toolResultCount)

	// Add final answer prompt
	finalPrompt := buildFinalAnswerSynthesisPrompt(compactQuery, state)

	messages = append(messages, chat.Message{
		Role:    "user",
		Content: finalPrompt,
	})
	if len([]rune(compactQuery)) >= finalAnswerQueryCharBudget {
		logger.Warnf(ctx, "[Agent][FinalAnswer] Compact query hit char budget: %d", len([]rune(compactQuery)))
	}
	totalChars := 0
	for _, msg := range messages {
		totalChars += len([]rune(msg.Content))
	}
	if totalChars > finalAnswerQueryCharBudget+finalAnswerToolSummaryBudget+2000 {
		fallback := buildFinalSynthesisBudgetFallback(language, documentContext)
		logger.Warnf(ctx, "[Agent][FinalAnswer] Skipping LLM synthesis because compact context is still too large: chars=%d", totalChars)
		e.emitStaticFinalAnswer(ctx, sessionID, state, fallback)
		state.FinalAnswer = fallback
		state.FinalAnswerSynthesized = true
		return nil
	}

	// Generate a single ID for this entire final answer stream
	answerID := generateEventID("answer")
	logger.Debugf(ctx, "[Agent][FinalAnswer] AnswerID: %s", answerID)
	answerDoneEmitted := false

	finalAnswerThinking := false
	llmResult, err := e.streamLLMToEventBus(
		ctx,
		messages,
		&chat.ChatOptions{Temperature: e.config.Temperature, Thinking: &finalAnswerThinking},
		func(chunk *types.StreamResponse, fullContent string) {
			// Defensive filter: only emit answer content, skip thinking chunks
			if chunk.ResponseType == types.ResponseTypeThinking {
				return
			}
			if chunk.Content != "" {
				logger.Debugf(ctx, "[Agent][FinalAnswer] Emitting answer chunk: %d chars", len(chunk.Content))
				e.eventBus.Emit(ctx, event.Event{
					ID:        answerID,
					Type:      event.EventAgentFinalAnswer,
					SessionID: sessionID,
					Data:      buildFinalAnswerEventData(state, chunk.Content, chunk.Done),
				})
				if chunk.Done {
					answerDoneEmitted = true
				}
			}
		},
	)
	if err != nil {
		logger.Errorf(ctx, "[Agent][FinalAnswer] Final answer generation failed: %v", err)
		common.PipelineError(ctx, "Agent", "final_answer_stream_failed", map[string]interface{}{
			"session_id": sessionID,
			"error":      err.Error(),
		})
		return err
	}

	if !answerDoneEmitted {
		e.eventBus.Emit(ctx, event.Event{
			ID:        answerID,
			Type:      event.EventAgentFinalAnswer,
			SessionID: sessionID,
			Data: event.AgentFinalAnswerData{
				Content: "",
				Done:    true,
			},
		})
	}

	// Safety net: strip any residual <think> blocks that may have leaked through
	fullAnswer := agenttools.StripThinkBlocks(llmResult.Content)
	logger.Infof(ctx, "[Agent][FinalAnswer] Final answer generated: %d characters", len(fullAnswer))
	common.PipelineInfo(ctx, "Agent", "final_answer_done", map[string]interface{}{
		"session_id": sessionID,
		"answer_len": len(fullAnswer),
	})
	state.FinalAnswer = fullAnswer
	state.FinalAnswerSynthesized = true
	return nil
}

func summarizeToolResultForFinalAnswer(toolCall types.ToolCall) string {
	if toolCall.Result == nil {
		return "(no tool result)"
	}
	if structured := summarizeStructuredToolResult(toolCall.Name, toolCall.Result); structured != "" {
		return structured
	}
	if !toolCall.Result.Success && toolCall.Result.Error != "" {
		return compactToolTextForFinalAnswer("Error: "+toolCall.Result.Error, finalAnswerToolOutputBudget)
	}
	return compactToolTextForFinalAnswer(toolCall.Result.Output, finalAnswerToolOutputBudget)
}

func summarizeStructuredToolResult(toolName string, result *types.ToolResult) string {
	if result == nil || result.Data == nil {
		return ""
	}
	switch toolName {
	case agenttools.ToolExternalDatabaseSchema:
		return summarizeDatabaseSchemaToolData(result.Data)
	case agenttools.ToolExternalDatabaseSearchTables:
		return summarizeDatabaseTableSearchToolData(result.Data)
	case agenttools.ToolExternalDatabaseQuery:
		return summarizeDatabaseQueryToolData(result.Data)
	default:
		return ""
	}
}

func summarizeDatabaseSchemaToolData(data map[string]interface{}) string {
	var builder strings.Builder
	builder.WriteString("Database schema summary\n")
	if databaseName, _ := data["database_name"].(string); strings.TrimSpace(databaseName) != "" {
		builder.WriteString(fmt.Sprintf("Database: %s\n", databaseName))
	}
	if schemaName, _ := data["schema_name"].(string); strings.TrimSpace(schemaName) != "" {
		builder.WriteString(fmt.Sprintf("Schema: %s\n", schemaName))
	}
	if schemaHash, _ := data["schema_hash"].(string); strings.TrimSpace(schemaHash) != "" {
		builder.WriteString(fmt.Sprintf("Schema hash: %s\n", schemaHash))
	}
	if refreshedAt, _ := data["refreshed_at"].(string); strings.TrimSpace(refreshedAt) != "" {
		builder.WriteString(fmt.Sprintf("Refreshed at: %s\n", refreshedAt))
	}
	if mode, _ := data["mode"].(string); strings.TrimSpace(mode) != "" {
		builder.WriteString(fmt.Sprintf("Mode: %s\n", mode))
	}
	if tableCount, ok := toInt(data["table_count"]); ok {
		builder.WriteString(fmt.Sprintf("Table count: %d\n", tableCount))
	}
	matchedTables := toStringSlice(data["matched_tables"])
	allowedTables := toStringSlice(data["allowed_tables"])
	if len(matchedTables) == 0 {
		matchedTables = append([]string(nil), allowedTables...)
	}
	scopeTableCount, hasScopeTableCount := toInt(data["scope_table_count"])
	matchedTableCount, hasMatchedTableCount := toInt(data["matched_table_count"])
	if !hasMatchedTableCount {
		matchedTableCount = len(matchedTables)
	}
	if !hasScopeTableCount {
		scopeTableCount = len(allowedTables)
	}
	if hasScopeTableCount && scopeTableCount > 0 {
		builder.WriteString(fmt.Sprintf("Scope table count: %d\n", scopeTableCount))
	}
	if hasMatchedTableCount || len(matchedTables) > 0 {
		builder.WriteString(fmt.Sprintf("Matched table count: %d\n", matchedTableCount))
	}
	if displayTableCount, ok := toInt(data["display_table_count"]); ok {
		builder.WriteString(fmt.Sprintf("Current view table count: %d\n", displayTableCount))
	}
	if keyword, _ := data["keyword"].(string); strings.TrimSpace(keyword) != "" {
		builder.WriteString(fmt.Sprintf("Keyword filter: %s\n", keyword))
	}
	if tableNameLike, _ := data["table_name_like"].(string); strings.TrimSpace(tableNameLike) != "" {
		builder.WriteString(fmt.Sprintf("Table name filter: %s\n", tableNameLike))
	}
	if commentLike, _ := data["comment_like"].(string); strings.TrimSpace(commentLike) != "" {
		builder.WriteString(fmt.Sprintf("Comment filter: %s\n", commentLike))
	}
	if len(matchedTables) > 0 {
		sort.Strings(matchedTables)
		previewTables := limitStringSlice(matchedTables, 8)
		builder.WriteString(fmt.Sprintf("Table preview: %s\n", strings.Join(previewTables, ", ")))
		if additionalTables, ok := toInt(data["additional_tables_omitted"]); ok && additionalTables > 0 {
			builder.WriteString(fmt.Sprintf("Additional tables omitted from current view: %d\n", additionalTables))
		} else if len(matchedTables) > len(previewTables) {
			builder.WriteString(fmt.Sprintf("Additional tables omitted from summary preview: %d\n", len(matchedTables)-len(previewTables)))
		}
	}
	if listOnly, _ := data["list_only"].(bool); listOnly {
		builder.WriteString("Retrieval hint: full matched table list was returned in list_only mode; choose target tables and rerun external_database_schema with tables=[...] and mode=detail for full columns.\n")
	} else if scopeTableCount > matchedTableCount && matchedTableCount > 0 {
		builder.WriteString("Retrieval hint: current summary is narrowed relative to the full scope; call external_database_search_tables first to narrow candidate tables, or use keyword/table_name_like/comment_like or list_only=true, then rerun with tables=[...] and mode=detail for full columns.\n")
	} else if matchedTableCount > 8 {
		builder.WriteString("Retrieval hint: summary preview is truncated; use external_database_search_tables or list_only=true for the full candidate list, then rerun external_database_schema with tables=[...] and mode=detail for the tables you need.\n")
	}
	if foreignKeys := toStringSlice(data["foreign_keys"]); len(foreignKeys) > 0 {
		builder.WriteString("Foreign keys:\n")
		for _, hint := range limitStringSlice(foreignKeys, 6) {
			builder.WriteString("- ")
			builder.WriteString(hint)
			builder.WriteString("\n")
		}
	}
	possibleJoinHints := toStringSlice(data["possible_join_hints"])
	if len(possibleJoinHints) == 0 {
		possibleJoinHints = toStringSlice(data["join_hints"])
	}
	if len(possibleJoinHints) > 0 {
		builder.WriteString("Possible join hints:\n")
		for _, hint := range limitStringSlice(possibleJoinHints, 6) {
			builder.WriteString("- ")
			builder.WriteString(hint)
			builder.WriteString("\n")
		}
	}
	if sampleQueries := toStringSlice(data["sample_queries"]); len(sampleQueries) > 0 {
		builder.WriteString("Sample queries:\n")
		for _, query := range limitStringSlice(sampleQueries, 3) {
			builder.WriteString("- ")
			builder.WriteString(compactToolTextForFinalAnswer(query, 220))
			builder.WriteString("\n")
		}
	}
	return strings.TrimSpace(builder.String())
}

func summarizeDatabaseTableSearchToolData(data map[string]interface{}) string {
	var builder strings.Builder
	builder.WriteString("Database table search summary\n")
	if databaseName, _ := data["database_name"].(string); strings.TrimSpace(databaseName) != "" {
		builder.WriteString(fmt.Sprintf("Database: %s\n", databaseName))
	}
	if schemaName, _ := data["schema_name"].(string); strings.TrimSpace(schemaName) != "" {
		builder.WriteString(fmt.Sprintf("Schema: %s\n", schemaName))
	}
	if scopeTableCount, ok := toInt(data["scope_table_count"]); ok {
		builder.WriteString(fmt.Sprintf("Scope table count: %d\n", scopeTableCount))
	}
	if matchedTableCount, ok := toInt(data["matched_table_count"]); ok {
		builder.WriteString(fmt.Sprintf("Matched table count: %d\n", matchedTableCount))
	}
	if returnedHitCount, ok := toInt(data["returned_hit_count"]); ok {
		builder.WriteString(fmt.Sprintf("Returned hit count: %d\n", returnedHitCount))
	}
	if keyword, _ := data["keyword"].(string); strings.TrimSpace(keyword) != "" {
		builder.WriteString(fmt.Sprintf("Keyword filter: %s\n", keyword))
	}
	if tableNameLike, _ := data["table_name_like"].(string); strings.TrimSpace(tableNameLike) != "" {
		builder.WriteString(fmt.Sprintf("Table name filter: %s\n", tableNameLike))
	}
	if commentLike, _ := data["comment_like"].(string); strings.TrimSpace(commentLike) != "" {
		builder.WriteString(fmt.Sprintf("Comment filter: %s\n", commentLike))
	}
	if columnNameLike, _ := data["column_name_like"].(string); strings.TrimSpace(columnNameLike) != "" {
		builder.WriteString(fmt.Sprintf("Column name filter: %s\n", columnNameLike))
	}
	if columnCommentLike, _ := data["column_comment_like"].(string); strings.TrimSpace(columnCommentLike) != "" {
		builder.WriteString(fmt.Sprintf("Column comment filter: %s\n", columnCommentLike))
	}
	if matchedTables := toStringSlice(data["matched_tables"]); len(matchedTables) > 0 {
		sort.Strings(matchedTables)
		builder.WriteString(fmt.Sprintf("Candidate tables: %s\n", strings.Join(limitStringSlice(matchedTables, 8), ", ")))
		if additionalMatches, ok := toInt(data["additional_matches_omitted"]); ok && additionalMatches > 0 {
			builder.WriteString(fmt.Sprintf("Additional candidate tables omitted: %d\n", additionalMatches))
		} else if len(matchedTables) > 8 {
			builder.WriteString(fmt.Sprintf("Additional candidate tables omitted: %d\n", len(matchedTables)-8))
		}
	}
	if results, ok := data["results"].([]map[string]interface{}); ok && len(results) > 0 {
		builder.WriteString("Top matches:\n")
		for _, result := range results[:minInt(len(results), 3)] {
			tableName, _ := result["table_name"].(string)
			likelyRole, _ := result["likely_role"].(string)
			matchedColumns := toStringSlice(result["matched_columns"])
			builder.WriteString("- ")
			builder.WriteString(tableName)
			if strings.TrimSpace(likelyRole) != "" {
				builder.WriteString(fmt.Sprintf(" [%s]", likelyRole))
			}
			if len(matchedColumns) > 0 {
				builder.WriteString(fmt.Sprintf(" matched columns: %s", strings.Join(limitStringSlice(matchedColumns, 4), ", ")))
			}
			builder.WriteString("\n")
		}
	}
	builder.WriteString("Retrieval hint: inspect the top candidate tables, then rerun external_database_schema with tables=[...] and mode=detail before writing SQL.\n")
	return strings.TrimSpace(builder.String())
}

func summarizeDatabaseQueryToolData(data map[string]interface{}) string {
	var builder strings.Builder
	builder.WriteString("Database query summary\n")
	columns := toStringSlice(data["columns"])
	if len(columns) > 0 {
		builder.WriteString(fmt.Sprintf("Columns: %s", strings.Join(limitStringSlice(columns, finalAnswerColumnPreviewLimit), ", ")))
		if len(columns) > finalAnswerColumnPreviewLimit {
			builder.WriteString(fmt.Sprintf(" (+%d more)", len(columns)-finalAnswerColumnPreviewLimit))
		}
		builder.WriteString("\n")
	}
	if rowCount, ok := toInt(data["row_count"]); ok {
		builder.WriteString(fmt.Sprintf("Row count: %d\n", rowCount))
	}
	if truncated, ok := data["truncated"].(bool); ok && truncated {
		builder.WriteString("Result truncated: true\n")
	}
	if outputTruncated, ok := data["output_truncated"].(bool); ok && outputTruncated {
		builder.WriteString("Output truncated: true\n")
	}
	if cellTruncatedCount, ok := toInt(data["cell_truncated_count"]); ok && cellTruncatedCount > 0 {
		builder.WriteString(fmt.Sprintf("Cells truncated: %d\n", cellTruncatedCount))
	}
	if durationMS, ok := toInt64(data["duration_ms"]); ok {
		builder.WriteString(fmt.Sprintf("Duration: %d ms\n", durationMS))
	}
	if executedSQL, _ := data["executed_sql"].(string); strings.TrimSpace(executedSQL) != "" {
		builder.WriteString(fmt.Sprintf("Executed SQL: %s\n", compactToolTextForFinalAnswer(executedSQL, 240)))
	}
	rows, _ := data["rows"].([]map[string]interface{})
	if len(rows) == 0 {
		rows = toRowMaps(data["rows"])
	}
	if len(rows) > 0 {
		builder.WriteString("Sample rows:\n")
		for index, row := range rows {
			if index >= finalAnswerRowPreviewLimit {
				builder.WriteString(fmt.Sprintf("- ... %d more row(s) omitted\n", len(rows)-finalAnswerRowPreviewLimit))
				break
			}
			builder.WriteString(fmt.Sprintf("- %s\n", summarizeGenericRow(columns, row, finalAnswerColumnPreviewLimit)))
		}
	}
	return strings.TrimSpace(builder.String())
}

func summarizeGenericRow(columns []string, row map[string]interface{}, columnLimit int) string {
	keys := columns
	if len(keys) == 0 {
		keys = make([]string, 0, len(row))
		for key := range row {
			keys = append(keys, key)
		}
		sort.Strings(keys)
	}
	items := make([]string, 0, minInt(len(keys), columnLimit)+1)
	for index, key := range keys {
		if index >= columnLimit {
			items = append(items, fmt.Sprintf("... +%d more column(s)", len(keys)-columnLimit))
			break
		}
		raw, err := json.Marshal(row[key])
		if err != nil {
			items = append(items, fmt.Sprintf("%s=%v", key, row[key]))
			continue
		}
		items = append(items, fmt.Sprintf("%s=%s", key, compactToolTextForFinalAnswer(string(raw), 120)))
	}
	return strings.Join(items, "; ")
}

func compactToolTextForFinalAnswer(text string, maxChars int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "(empty output)"
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	runes := []rune(text)
	if maxChars <= 0 || len(runes) <= maxChars {
		return text
	}
	head := maxChars * 2 / 3
	tail := maxChars - head - len([]rune("\n... [truncated for synthesis] ...\n"))
	if tail < 80 {
		tail = 80
		head = maxChars - tail - len([]rune("\n... [truncated for synthesis] ...\n"))
	}
	return strings.TrimSpace(string(runes[:head])) + "\n... [truncated for synthesis] ...\n" + strings.TrimSpace(string(runes[len(runes)-tail:]))
}

func toStringSlice(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []interface{}:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
				items = append(items, str)
			}
		}
		return items
	default:
		return nil
	}
}

func toInt(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}

func toInt64(value interface{}) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	default:
		return 0, false
	}
}

func toRowMaps(value interface{}) []map[string]interface{} {
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}
	rows := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if row, ok := item.(map[string]interface{}); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func limitStringSlice(items []string, limit int) []string {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// handleMaxIterations generates a final answer when the agent loop exhausted all iterations
// without the LLM producing a natural stop. It marks state.IsComplete = true.
func (e *AgentEngine) handleMaxIterations(
	ctx context.Context, query string, state *types.AgentState, sessionID string,
) {
	logger.Info(ctx, "Reached max iterations, generating final answer")
	common.PipelineWarn(ctx, "Agent", "max_iterations_reached", map[string]interface{}{
		"iterations": state.CurrentRound,
		"max":        e.config.MaxIterations,
	})
	state.CompletionStatus = "partial"
	state.FinishReason = "max_iterations"
	state.FailureReason = "max_iterations"
	state.AllowIndexing = false
	state.AllowComplete = false

	// Stream final answer generation through EventBus
	if err := e.streamFinalAnswerToEventBus(ctx, query, state, sessionID); err != nil {
		logger.Errorf(ctx, "Failed to synthesize final answer: %v", err)
		common.PipelineError(ctx, "Agent", "final_answer_failed", map[string]interface{}{
			"error": err.Error(),
		})
		state.FinalAnswer = "Sorry, I was unable to generate a complete answer."
	}
	if state.PartialAnswer != "" {
		state.FinalAnswer = mergeContinuationAnswer(state.PartialAnswer, state.FinalAnswer)
	}
	state.IsComplete = true
}

// emitCompletionEvent emits the EventAgentComplete event with execution summary.
func (e *AgentEngine) emitCompletionEvent(
	ctx context.Context, state *types.AgentState, sessionID, messageID string, startTime time.Time,
) {
	// Convert knowledge refs to interface{} slice for event data
	knowledgeRefsInterface := make([]interface{}, 0, len(state.KnowledgeRefs))
	for _, ref := range state.KnowledgeRefs {
		knowledgeRefsInterface = append(knowledgeRefsInterface, ref)
	}
	normalized := normalizeCompletionEventState(state)
	e.persistPendingContextGroups(ctx, normalized.completionStatus)

	e.eventBus.Emit(ctx, event.Event{
		ID:        generateEventID("complete"),
		Type:      event.EventAgentComplete,
		SessionID: sessionID,
		Data: event.AgentCompleteData{
			FinalAnswer:      state.FinalAnswer,
			CompletionStatus: normalized.completionStatus,
			FinishReason:     normalized.finishReason,
			IsPartial:        normalized.completionStatus == types.MessageCompletionStatusPartial,
			AllowIndexing:    normalized.allowIndexing,
			AllowComplete:    normalized.allowComplete,
			FailureReason:    normalized.failureReason,
			KnowledgeRefs:    knowledgeRefsInterface,
			AgentSteps:       append(types.AgentSteps(nil), state.RoundSteps...), // Include detailed execution steps for message storage
			TotalSteps:       len(state.RoundSteps),
			TotalDurationMs:  time.Since(startTime).Milliseconds(),
			MessageID:        messageID, // Include message ID for proper message update
		},
	})

	logger.Infof(ctx, "Agent execution completed in %d rounds", state.CurrentRound)
}

func (e *AgentEngine) persistPendingContextGroups(ctx context.Context, completionStatus string) {
	defer func() {
		e.pendingContextGroups = nil
	}()
	if e == nil || len(e.pendingContextGroups) == 0 {
		return
	}
	if completionStatus == types.MessageCompletionStatusFailed {
		logger.Infof(ctx, "[Agent] Skipping deferred context persistence for failed execution (session: %s, pending_messages=%d)", e.sessionID, len(e.pendingContextGroups))
		return
	}
	logger.Debugf(ctx, "[Agent] Clearing %d deferred tool-group message(s) after completion (session: %s)", len(e.pendingContextGroups), e.sessionID)
}
