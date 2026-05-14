package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
// newFinalAnswerResponse builds a ChatResponse that carries a single
// final_answer tool call with the given raw JSON arguments.
func newFinalAnswerResponse(rawArgs string) *types.ChatResponse {
	return &types.ChatResponse{
		FinishReason: "tool_calls",
		ToolCalls: []types.LLMToolCall{
			{
				ID:   "call-1",
				Type: "function",
				Function: types.FunctionCall{
					Name:      agenttools.ToolFinalAnswer,
					Arguments: rawArgs,
				},
			},
		},
	}
}

// TestAnalyzeResponse_FinalAnswer_ValidArgs guards the happy path: well-formed
// arguments must be extracted into the final answer and terminate the loop.
func TestAnalyzeResponse_FinalAnswer_ValidArgs(t *testing.T) {
	engine := newTestEngine(t, &mockChat{})
	resp := newFinalAnswerResponse(`{"answer": "Here is the answer."}`)

	verdict := engine.analyzeResponse(
		context.Background(), resp, types.AgentStep{}, 0, "sess-1", time.Now(),
	)

	assert.True(t, verdict.isDone, "final_answer must terminate the loop")
	assert.Equal(t, "Here is the answer.", verdict.finalAnswer)
}

func TestAnalyzeResponse_FinalAnswer_ValidArgs_EmitsAuthoritativeAnswer(t *testing.T) {
	engine := newTestEngine(t, &mockChat{})
	resp := newFinalAnswerResponse(`{"answer": "Here is the answer."}`)

	var emitted []event.AgentFinalAnswerData
	engine.eventBus.On(event.EventAgentFinalAnswer, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		require.True(t, ok)
		emitted = append(emitted, data)
		return nil
	})

	verdict := engine.analyzeResponse(
		context.Background(), resp, types.AgentStep{}, 0, "sess-1", time.Now(),
	)

	require.True(t, verdict.isDone)
	require.Len(t, emitted, 2)
	assert.Equal(t, "Here is the answer.", emitted[0].Content)
	assert.False(t, emitted[0].Done)
	assert.Empty(t, emitted[1].Content)
	assert.True(t, emitted[1].Done)
}

// TestAnalyzeResponse_FinalAnswer_MalformedJSON_RecoveredViaRepair covers the
// common case reported in issue #1008: the LLM emits final_answer with a
// trailing comma / missing brace. RepairJSON should recover the answer and
// the loop must still terminate in this single round (not re-invoke
// final_answer in the next round).
func TestAnalyzeResponse_FinalAnswer_MalformedJSON_RecoveredViaRepair(t *testing.T) {
	engine := newTestEngine(t, &mockChat{})
	resp := newFinalAnswerResponse(`{"answer": "repaired"`) // missing closing brace

	verdict := engine.analyzeResponse(
		context.Background(), resp, types.AgentStep{}, 0, "sess-1", time.Now(),
	)

	assert.True(t, verdict.isDone,
		"final_answer must terminate the loop even when JSON repair is needed")
	assert.Equal(t, "repaired", verdict.finalAnswer)
}

// TestAnalyzeResponse_FinalAnswer_UnrecoverableArgs_StillTerminates is the
// direct regression test for issue #1008: when the arguments are so malformed
// that even RepairJSON + regex cannot recover an answer, the loop MUST still
// terminate (with a user-visible fallback message) rather than continuing and
// letting the LLM re-emit final_answer on the next round.
func TestAnalyzeResponse_FinalAnswer_UnrecoverableArgs_StillTerminates(t *testing.T) {
	engine := newTestEngine(t, &mockChat{})
	// No `answer` key at all — strict parse succeeds (returns zero-value
	// answer), RepairJSON is a no-op on already-valid JSON, regex finds
	// nothing. All three tiers fail to recover an answer.
	resp := newFinalAnswerResponse(`{"unexpected": "field"}`)

	verdict := engine.analyzeResponse(
		context.Background(), resp, types.AgentStep{}, 0, "sess-1", time.Now(),
	)

	assert.True(t, verdict.isDone,
		"final_answer must terminate the loop even when args are unrecoverable — "+
			"otherwise the LLM re-emits final_answer and duplicates the answer (issue #1008)")
	assert.Equal(t, finalAnswerParseFallback, verdict.finalAnswer,
		"unrecoverable final_answer should surface the parse-failure fallback message")
}

// TestAnalyzeResponse_FinalAnswer_Garbage_StillTerminates exercises the most
// hostile case: completely non-JSON arguments. The loop must still terminate
// — protecting against the duplicate-answer loop reported in issue #1008.
func TestAnalyzeResponse_FinalAnswer_Garbage_StillTerminates(t *testing.T) {
	engine := newTestEngine(t, &mockChat{})
	resp := newFinalAnswerResponse(`not json at all`)

	verdict := engine.analyzeResponse(
		context.Background(), resp, types.AgentStep{}, 0, "sess-1", time.Now(),
	)

	assert.True(t, verdict.isDone)
	assert.Equal(t, finalAnswerParseFallback, verdict.finalAnswer)
}

// TestAnalyzeResponse_NonFinalAnswerTool_DoesNotTerminate is a regression
// guard: only final_answer is terminal. Other tool calls (e.g. thinking,
// knowledge_search) must keep the loop running.
func TestAnalyzeResponse_NonFinalAnswerTool_DoesNotTerminate(t *testing.T) {
	engine := newTestEngine(t, &mockChat{})
	resp := &types.ChatResponse{
		FinishReason: "tool_calls",
		ToolCalls: []types.LLMToolCall{
			{
				ID:   "call-1",
				Type: "function",
				Function: types.FunctionCall{
					Name:      agenttools.ToolKnowledgeSearch,
					Arguments: `{"query": "hi"}`,
				},
			},
		},
	}

	verdict := engine.analyzeResponse(
		context.Background(), resp, types.AgentStep{}, 0, "sess-1", time.Now(),
	)

	assert.False(t, verdict.isDone,
		"non-terminal tool calls must keep the loop running")
}

func TestAnalyzeResponse_LengthWithoutToolCalls_ReturnsPartial(t *testing.T) {
	engine := newTestEngine(t, &mockChat{})
	resp := &types.ChatResponse{
		Content:      "partial answer",
		FinishReason: "length",
	}

	verdict := engine.analyzeResponse(
		context.Background(), resp, types.AgentStep{}, 0, "sess-1", time.Now(),
	)

	assert.True(t, verdict.isDone)
	assert.Equal(t, "partial", verdict.completionStatus)
	assert.Equal(t, "length", verdict.finishReason)
	assert.True(t, verdict.isPartial)
	assert.False(t, verdict.allowIndexing)
	assert.False(t, verdict.allowComplete)
	assert.Equal(t, "partial answer", verdict.finalAnswer)
}

func TestAnalyzeResponse_StopWithStreamedAnswer_OnlyEmitsDoneMarker(t *testing.T) {
	engine := newTestEngine(t, &mockChat{})
	resp := &types.ChatResponse{
		Content:             "streamed answer",
		FinishReason:        "stop",
		AnswerStreamed:      true,
		FinalAnswerStreamed: true,
	}

	var emitted []event.AgentFinalAnswerData
	engine.eventBus.On(event.EventAgentFinalAnswer, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		require.True(t, ok)
		emitted = append(emitted, data)
		return nil
	})

	verdict := engine.analyzeResponse(
		context.Background(), resp, types.AgentStep{}, 0, "sess-1", time.Now(),
	)

	require.True(t, verdict.isDone)
	require.Len(t, emitted, 1)
	assert.True(t, emitted[0].Done)
	assert.Empty(t, emitted[0].Content)
}

func TestAnalyzeResponse_FinalAnswerStreamErrorTerminatesAsPartial(t *testing.T) {
	engine := newTestEngine(t, &mockChat{})
	resp := &types.ChatResponse{
		Content:             "# 北海电厂二期智慧电厂项目\n\n## 一、项目背景与总体思路",
		FinishReason:        "stream_error_after_answer",
		AnswerStreamed:      true,
		FinalAnswerStreamed: true,
	}

	var emitted []event.AgentFinalAnswerData
	engine.eventBus.On(event.EventAgentFinalAnswer, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		require.True(t, ok)
		emitted = append(emitted, data)
		return nil
	})

	verdict := engine.analyzeResponse(
		context.Background(), resp, types.AgentStep{}, 0, "sess-1", time.Now(),
	)

	require.True(t, verdict.isDone)
	assert.Equal(t, resp.Content, verdict.finalAnswer)
	assert.Equal(t, types.MessageCompletionStatusPartial, verdict.completionStatus)
	assert.Equal(t, "stream_error_after_answer", verdict.finishReason)
	assert.Equal(t, "stream_error_after_answer", verdict.failureReason)
	assert.True(t, verdict.isPartial)
	assert.False(t, verdict.allowIndexing)
	assert.False(t, verdict.allowComplete)
	require.Len(t, emitted, 1)
	assert.Empty(t, emitted[0].Content)
	assert.True(t, emitted[0].Done)
	assert.Equal(t, types.MessageCompletionStatusPartial, emitted[0].CompletionStatus)
	assert.Equal(t, "stream_error_after_answer", emitted[0].FinishReason)
	assert.True(t, emitted[0].IsPartial)
	assert.False(t, emitted[0].AllowIndexing)
	assert.False(t, emitted[0].AllowComplete)
}

func TestStreamThinkingToEventBus_OrdinaryAnswerChunksStayOnAnswerChannel(t *testing.T) {
	mock := &mockChat{responses: []mockResponse{{chunks: []types.StreamResponse{
		{ResponseType: types.ResponseTypeAnswer, Content: "hello "},
		{ResponseType: types.ResponseTypeAnswer, Content: "world", Done: true, FinishReason: "stop"},
	}}}}
	engine := newTestEngine(t, mock)

	var answers []event.AgentFinalAnswerData
	var thoughts []event.AgentThoughtData
	engine.eventBus.On(event.EventAgentFinalAnswer, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		require.True(t, ok)
		answers = append(answers, data)
		return nil
	})
	engine.eventBus.On(event.EventAgentThought, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentThoughtData)
		require.True(t, ok)
		thoughts = append(thoughts, data)
		return nil
	})

	response, err := engine.streamThinkingToEventBus(context.Background(), emptyMessages(), emptyTools(), 0, "sess-1")
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.True(t, response.AnswerStreamed)
	assert.False(t, response.FinalAnswerStreamed)
	assert.Len(t, answers, 2)
	assert.Empty(t, thoughts)
	assert.Equal(t, "hello ", answers[0].Content)
	assert.Equal(t, "world", answers[1].Content)
	assert.False(t, answers[0].Done)
	assert.False(t, answers[1].Done)
}

func TestStreamThinkingToEventBus_SourceAnswerTailTimeoutClosesAsPartial(t *testing.T) {
	mock := &mockChat{responses: []mockResponse{{chunks: []types.StreamResponse{
		{
			ResponseType: types.ResponseTypeAnswer,
			Content:      "visible ",
			Data: map[string]interface{}{
				"source": "final_answer_tool",
			},
		},
		{
			ResponseType: types.ResponseTypeAnswer,
			Content:      "answer",
			Data: map[string]interface{}{
				"source": "final_answer_tool",
			},
		},
		{ResponseType: types.ResponseTypeError, Content: "context deadline exceeded"},
	}}}}
	engine := newTestEngine(t, mock)

	var answers []event.AgentFinalAnswerData
	engine.eventBus.On(event.EventAgentFinalAnswer, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		require.True(t, ok)
		answers = append(answers, data)
		return nil
	})

	response, err := engine.streamThinkingToEventBus(context.Background(), emptyMessages(), emptyTools(), 0, "sess-1")
	require.NoError(t, err)
	require.NotNil(t, response)

	assert.Equal(t, "visible answer", response.Content)
	assert.Equal(t, "stream_error_after_answer", response.FinishReason)
	assert.True(t, response.AnswerStreamed)
	assert.True(t, response.FinalAnswerStreamed)
	require.Len(t, answers, 3)
	assert.Equal(t, "visible ", answers[0].Content)
	assert.False(t, answers[0].Done)
	assert.Equal(t, "answer", answers[1].Content)
	assert.False(t, answers[1].Done)
	assert.Empty(t, answers[2].Content)
	assert.True(t, answers[2].Done)
	assert.Equal(t, types.MessageCompletionStatusPartial, answers[2].CompletionStatus)
	assert.Equal(t, "stream_error_after_answer", answers[2].FinishReason)
	assert.True(t, answers[2].IsPartial)
	assert.False(t, answers[2].AllowIndexing)
	assert.False(t, answers[2].AllowComplete)
}

func TestStreamThinkingToEventBus_DuplicateDocumentHeadStopsBeforeEmittingRestart(t *testing.T) {
	baseDocument := strings.Join([]string{
		"# 北海电厂二期智慧电厂项目",
		"",
		"## 一、项目背景与总体思路",
		strings.Repeat("已有建设背景与总体思路内容。\n", 80),
		"## 三、核心价值",
		strings.Repeat("已有核心价值分析内容。\n", 40),
	}, "\n")
	mock := &mockChat{responses: []mockResponse{{chunks: []types.StreamResponse{
		{
			ResponseType: types.ResponseTypeAnswer,
			Content:      "# 北海电厂二期智慧电厂项目\n\n",
			Data: map[string]interface{}{
				"source": "final_answer_tool",
			},
		},
		{
			ResponseType: types.ResponseTypeAnswer,
			Content:      "## 一、项目背景与总体思路\n\n",
			Data: map[string]interface{}{
				"source": "final_answer_tool",
			},
		},
		{
			ResponseType: types.ResponseTypeAnswer,
			Content:      "重复开头不应继续污染当前消息。",
			Data: map[string]interface{}{
				"source": "final_answer_tool",
			},
		},
		{ResponseType: types.ResponseTypeAnswer, Done: true, FinishReason: "stop"},
	}}}}
	engine := newTestEngine(t, mock)

	var answers []event.AgentFinalAnswerData
	engine.eventBus.On(event.EventAgentFinalAnswer, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		require.True(t, ok)
		answers = append(answers, data)
		return nil
	})

	response, err := engine.streamThinkingToEventBus(context.Background(), []chat.Message{{Role: "assistant", Content: baseDocument}}, emptyTools(), 1, "sess-1")
	require.NoError(t, err)
	require.NotNil(t, response)

	assert.Equal(t, duplicateDocumentHeadFinishReason, response.FinishReason)
	assert.Empty(t, response.Content)
	assert.False(t, response.AnswerStreamed)
	assert.False(t, response.FinalAnswerStreamed)
	require.Len(t, answers, 1)
	assert.Empty(t, answers[0].Content)
	assert.True(t, answers[0].Done)
	assert.Equal(t, types.MessageCompletionStatusPartial, answers[0].CompletionStatus)
	assert.Equal(t, duplicateDocumentHeadFinishReason, answers[0].FinishReason)
	assert.True(t, answers[0].IsPartial)
	assert.False(t, answers[0].AllowIndexing)
	assert.False(t, answers[0].AllowComplete)
}

func TestStreamThinkingToEventBus_DuplicateDocumentHeadDoesNotBlockNewUserTurn(t *testing.T) {
	baseDocument := strings.Join([]string{
		"# 北海电厂二期智慧电厂项目",
		"",
		"## 一、项目背景与总体思路",
		strings.Repeat("已有建设背景与总体思路内容。\n", 80),
		"## 三、核心价值",
		strings.Repeat("已有核心价值分析内容。\n", 40),
	}, "\n")
	mock := &mockChat{responses: []mockResponse{{chunks: []types.StreamResponse{
		{
			ResponseType: types.ResponseTypeAnswer,
			Content:      "# 北海电厂二期智慧电厂项目\n\n",
			Data: map[string]interface{}{
				"source": "final_answer_tool",
			},
		},
		{
			ResponseType: types.ResponseTypeAnswer,
			Content:      "## 一、项目背景与总体思路\n\n新的独立回合允许重新生成同名文档。",
			Data: map[string]interface{}{
				"source": "final_answer_tool",
			},
		},
		{ResponseType: types.ResponseTypeAnswer, Done: true, FinishReason: "stop"},
	}}}}
	engine := newTestEngine(t, mock)

	var answers []event.AgentFinalAnswerData
	engine.eventBus.On(event.EventAgentFinalAnswer, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		require.True(t, ok)
		answers = append(answers, data)
		return nil
	})

	response, err := engine.streamThinkingToEventBus(context.Background(), []chat.Message{
		{Role: "assistant", Content: baseDocument},
		{Role: "user", Content: "请重新生成一份同标题的新技术方案"},
	}, emptyTools(), 1, "sess-1")
	require.NoError(t, err)
	require.NotNil(t, response)

	assert.Equal(t, "stop", response.FinishReason)
	assert.True(t, response.AnswerStreamed)
	assert.True(t, response.FinalAnswerStreamed)
	assert.Equal(t, "# 北海电厂二期智慧电厂项目\n\n## 一、项目背景与总体思路\n\n新的独立回合允许重新生成同名文档。", response.Content)
	require.Len(t, answers, 2)
	assert.Equal(t, "# 北海电厂二期智慧电厂项目\n\n", answers[0].Content)
	assert.False(t, answers[0].Done)
	assert.Equal(t, "## 一、项目背景与总体思路\n\n新的独立回合允许重新生成同名文档。", answers[1].Content)
	assert.False(t, answers[1].Done)
}

func TestStreamThinkingToEventBus_ToolCallingPrefaceStaysNonAuthoritative(t *testing.T) {
	mock := &mockChat{responses: []mockResponse{{chunks: []types.StreamResponse{
		{ResponseType: types.ResponseTypeAnswer, Content: "Let me summarize the result first. "},
		{
			ResponseType: types.ResponseTypeToolCall,
			Done:         true,
			FinishReason: "tool_calls",
			ToolCalls: []types.LLMToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: types.FunctionCall{
					Name:      agenttools.ToolFinalAnswer,
					Arguments: `{"answer":"full answer"}`,
				},
			}},
			Data: map[string]interface{}{
				"tool_call_id": "call-1",
				"tool_name":    agenttools.ToolFinalAnswer,
			},
		},
	}}}}
	engine := newTestEngine(t, mock)

	var answers []event.AgentFinalAnswerData
	engine.eventBus.On(event.EventAgentFinalAnswer, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		require.True(t, ok)
		answers = append(answers, data)
		return nil
	})

	response, err := engine.streamThinkingToEventBus(context.Background(), emptyMessages(), emptyTools(), 0, "sess-1")
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.True(t, response.AnswerStreamed)
	assert.False(t, response.FinalAnswerStreamed)
	assert.Len(t, response.ToolCalls, 1)
	require.Len(t, answers, 1)
	assert.Equal(t, "Let me summarize the result first. ", answers[0].Content)
}

func TestAnalyzeResponse_FinalAnswerToolStillEmitsAuthoritativeAnswerAfterPrefaceStream(t *testing.T) {
	engine := newTestEngine(t, &mockChat{})
	resp := &types.ChatResponse{
		Content:             "Let me summarize the result first. ",
		FinishReason:        "tool_calls",
		AnswerStreamed:      true,
		FinalAnswerStreamed: false,
		ToolCalls: []types.LLMToolCall{{
			ID:   "call-1",
			Type: "function",
			Function: types.FunctionCall{
				Name:      agenttools.ToolFinalAnswer,
				Arguments: `{"answer":"full answer"}`,
			},
		}},
	}

	var emitted []event.AgentFinalAnswerData
	engine.eventBus.On(event.EventAgentFinalAnswer, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		require.True(t, ok)
		emitted = append(emitted, data)
		return nil
	})

	verdict := engine.analyzeResponse(context.Background(), resp, types.AgentStep{}, 0, "sess-1", time.Now())
	require.True(t, verdict.isDone)
	require.Len(t, emitted, 2)
	assert.Equal(t, "full answer", emitted[0].Content)
	assert.False(t, emitted[0].Done)
	assert.True(t, emitted[1].Done)
}

func TestRunReActIteration_LengthResponseSchedulesContinuation(t *testing.T) {
	mock := &mockChat{responses: []mockResponse{{chunks: []types.StreamResponse{
		{ResponseType: types.ResponseTypeAnswer, Content: "part-1", FinishReason: "length", Done: true},
	}}}}
	engine := newTestEngine(t, mock)
	state := &types.AgentState{}
	messages := emptyMessages()
	emptyRetries := 0
	consecutiveSameContent := 0
	lastResponseContent := ""

	outcome, err := engine.runReActIteration(
		context.Background(),
		state,
		&messages,
		emptyTools(),
		"sess-1",
		"",
		"test query",
		&emptyRetries,
		&consecutiveSameContent,
		&lastResponseContent,
	)

	require.NoError(t, err)
	assert.Equal(t, iterOutcomeNext, outcome)
	assert.Equal(t, 1, state.ContinuationRounds)
	assert.Equal(t, "part-1", state.PartialAnswer)
	require.Len(t, state.RoundSteps, 1)
	require.Len(t, messages, 4)
	assert.Equal(t, "assistant", messages[2].Role)
	assert.Equal(t, "part-1", messages[2].Content)
	assert.Equal(t, "user", messages[3].Role)
	assert.Equal(t, lengthContinuationPrompt, messages[3].Content)
}

func TestRunReActIteration_ContinuationAnswerIsMergedOnFinalCompletion(t *testing.T) {
	mock := &mockChat{responses: []mockResponse{
		{chunks: []types.StreamResponse{{ResponseType: types.ResponseTypeAnswer, Content: "part-1", FinishReason: "length", Done: true}}},
		{chunks: []types.StreamResponse{{ResponseType: types.ResponseTypeAnswer, Content: "part-2", FinishReason: "stop", Done: true}}},
	}}
	engine := newTestEngine(t, mock)
	state := &types.AgentState{}
	messages := emptyMessages()
	emptyRetries := 0
	consecutiveSameContent := 0
	lastResponseContent := ""

	outcome, err := engine.runReActIteration(
		context.Background(),
		state,
		&messages,
		emptyTools(),
		"sess-1",
		"",
		"test query",
		&emptyRetries,
		&consecutiveSameContent,
		&lastResponseContent,
	)
	require.NoError(t, err)
	assert.Equal(t, iterOutcomeNext, outcome)
	state.CurrentRound++

	outcome, err = engine.runReActIteration(
		context.Background(),
		state,
		&messages,
		emptyTools(),
		"sess-1",
		"",
		"test query",
		&emptyRetries,
		&consecutiveSameContent,
		&lastResponseContent,
	)
	require.NoError(t, err)
	assert.Equal(t, iterOutcomeBreak, outcome)
	assert.Equal(t, "part-1part-2", state.FinalAnswer)
	assert.Equal(t, types.MessageCompletionStatusCompleted, state.CompletionStatus)
}

func TestRunReActIteration_FinalAnswerStreamErrorBreaksWithoutNextRound(t *testing.T) {
	mock := &mockChat{responses: []mockResponse{{chunks: []types.StreamResponse{
		{
			ResponseType: types.ResponseTypeAnswer,
			Content:      "# 北海电厂二期智慧电厂项目\n\n",
			Data: map[string]interface{}{"source": "final_answer_tool"},
		},
		{
			ResponseType: types.ResponseTypeAnswer,
			Content:      "## 一、项目背景与总体思路",
			Data: map[string]interface{}{"source": "final_answer_tool"},
		},
		{ResponseType: types.ResponseTypeError, Content: "context deadline exceeded"},
	}}}}
	engine := newTestEngine(t, mock)
	state := &types.AgentState{}
	messages := emptyMessages()
	emptyRetries := 0
	consecutiveSameContent := 0
	lastResponseContent := ""

	outcome, err := engine.runReActIteration(
		context.Background(), state, &messages, emptyTools(), "sess-1", "", "test query",
		&emptyRetries, &consecutiveSameContent, &lastResponseContent,
	)

	require.NoError(t, err)
	assert.Equal(t, iterOutcomeBreak, outcome)
	assert.Equal(t, 1, mock.callCount)
	assert.True(t, state.IsComplete)
	assert.Equal(t, types.MessageCompletionStatusPartial, state.CompletionStatus)
	assert.Equal(t, "stream_error_after_answer", state.FinishReason)
	assert.Equal(t, "stream_error_after_answer", state.FailureReason)
	assert.False(t, state.AllowIndexing)
	assert.False(t, state.AllowComplete)
	assert.Equal(t, "# 北海电厂二期智慧电厂项目\n\n## 一、项目背景与总体思路", state.FinalAnswer)
	require.Len(t, state.RoundSteps, 1)
	assert.Equal(t, state.FinalAnswer, state.RoundSteps[0].Thought)
	assert.Len(t, messages, 2)
}

func TestRunReActIteration_DuplicateDocumentHeadBreaksWithoutNextRound(t *testing.T) {
	baseDocument := strings.Join([]string{
		"# 北海电厂二期智慧电厂项目",
		"",
		"## 一、项目背景与总体思路",
		strings.Repeat("已有建设背景与总体思路内容。\n", 80),
		"## 三、核心价值",
		strings.Repeat("已有核心价值分析内容。\n", 40),
	}, "\n")
	mock := &mockChat{responses: []mockResponse{{chunks: []types.StreamResponse{
		{
			ResponseType: types.ResponseTypeAnswer,
			Content:      "# 北海电厂二期智慧电厂项目\n\n",
			Data: map[string]interface{}{"source": "final_answer_tool"},
		},
		{
			ResponseType: types.ResponseTypeAnswer,
			Content:      "## 一、项目背景与总体思路\n\n",
			Data: map[string]interface{}{"source": "final_answer_tool"},
		},
		{ResponseType: types.ResponseTypeAnswer, Done: true, FinishReason: "stop"},
	}}}}
	engine := newTestEngine(t, mock)
	state := &types.AgentState{}
	messages := []chat.Message{{Role: "assistant", Content: baseDocument}}
	emptyRetries := 0
	consecutiveSameContent := 0
	lastResponseContent := ""

	outcome, err := engine.runReActIteration(
		context.Background(), state, &messages, emptyTools(), "sess-1", "", "请继续输出完整技术方案",
		&emptyRetries, &consecutiveSameContent, &lastResponseContent,
	)

	require.NoError(t, err)
	assert.Equal(t, iterOutcomeBreak, outcome)
	assert.Equal(t, 1, mock.callCount)
	assert.True(t, state.IsComplete)
	assert.Equal(t, types.MessageCompletionStatusPartial, state.CompletionStatus)
	assert.Equal(t, duplicateDocumentHeadFinishReason, state.FinishReason)
	assert.Equal(t, duplicateDocumentHeadFinishReason, state.FailureReason)
	assert.False(t, state.AllowIndexing)
	assert.False(t, state.AllowComplete)
	assert.Empty(t, state.FinalAnswer)
	require.Len(t, state.RoundSteps, 1)
	assert.Empty(t, state.RoundSteps[0].Thought)
}

// TestAppendToolResults_PreservesReasoningContent verifies that the assistant
// message produced by appendToolResults carries the reasoning_content emitted
// by the model in the same round. Without this, MiMo and DeepSeek V3.2+
// thinking-mode reject the next ReAct round with HTTP 400
// "The reasoning_content in the thinking mode must be passed back to the API."
// (issue #1302).
func TestAppendToolResults_PreservesReasoningContent(t *testing.T) {
	engine := &AgentEngine{}
	t.Run("assistant message carries reasoning_content alongside thought and tool_calls", func(t *testing.T) {
		step := types.AgentStep{
			Iteration:        0,
			Thought:          "I will call search.",
			ReasoningContent: "Detailed chain of thought from MiMo/DeepSeek.",
			ToolCalls: []types.ToolCall{{
				ID:   "call_1",
				Name: "knowledge_search",
				Args: map[string]interface{}{"query": "hi"},
				Result: &types.ToolResult{Success: true, Output: "result text"},
			}},
			Timestamp: time.Now(),
		}

		out := engine.appendToolResults(nil, step)

		require.Len(t, out, 2, "expect one assistant + one tool message")
		assert.Equal(t, "assistant", out[0].Role)
		assert.Equal(t, "I will call search.", out[0].Content)
		assert.Equal(t, "Detailed chain of thought from MiMo/DeepSeek.", out[0].ReasoningContent,
			"reasoning_content must be propagated to the assistant message so providers like MiMo and DeepSeek thinking-mode see it on the next round (issue #1302)")
		require.Len(t, out[0].ToolCalls, 1)
		assert.Equal(t, "call_1", out[0].ToolCalls[0].ID)
		assert.Equal(t, "tool", out[1].Role)
		assert.Equal(t, "result text", out[1].Content)
		require.Len(t, engine.pendingContextGroups, 2)
		assert.Equal(t, "Detailed chain of thought from MiMo/DeepSeek.", engine.pendingContextGroups[0].ReasoningContent)
	})

	t.Run("reasoning_content alone produces an assistant message", func(t *testing.T) {
		step := types.AgentStep{Iteration: 0, ReasoningContent: "reasoning only", Timestamp: time.Now()}
		out := engine.appendToolResults(nil, step)
		require.Len(t, out, 1)
		assert.Equal(t, "assistant", out[0].Role)
		assert.Equal(t, "reasoning only", out[0].ReasoningContent)
		assert.Empty(t, out[0].Content)
		assert.Empty(t, out[0].ToolCalls)
	})

	t.Run("step without thought/tool_calls/reasoning produces no assistant message", func(t *testing.T) {
		step := types.AgentStep{Iteration: 0, Timestamp: time.Now()}
		out := engine.appendToolResults(nil, step)
		assert.Empty(t, out, "empty steps must not inject empty assistant messages")
	})

	t.Run("appends to existing message slice", func(t *testing.T) {
		prior := []chat.Message{{Role: "system", Content: "sys"}, {Role: "user", Content: "hi"}}
		step := types.AgentStep{Iteration: 1, Thought: "answer", ReasoningContent: "thinking", Timestamp: time.Now()}
		out := engine.appendToolResults(prior, step)
		require.Len(t, out, 3)
		assert.Equal(t, "system", out[0].Role)
		assert.Equal(t, "user", out[1].Role)
		assert.Equal(t, "assistant", out[2].Role)
		assert.Equal(t, "thinking", out[2].ReasoningContent)
	})
}

func TestAppendToolResults_SummarizesExternalDatabaseResultsInContext(t *testing.T) {
	engine := newTestEngine(t, &mockChat{})
	fullOutput := strings.Repeat("full query output ", 80)
	longCell := strings.Repeat("detail-", 40)
	step := types.AgentStep{
		ToolCalls: []types.ToolCall{{
			ID:   "tool-1",
			Name: agenttools.ToolExternalDatabaseQuery,
			Args: map[string]interface{}{"sql": "SELECT id, note FROM orders LIMIT 30"},
			Result: &types.ToolResult{
				Success: true,
				Output:  fullOutput,
				Data: map[string]interface{}{
					"columns":              []string{"id", "note"},
					"rows":                 []map[string]interface{}{{"id": 1, "note": longCell}},
					"row_count":            5,
					"duration_ms":          int64(18),
					"executed_sql":         "SELECT id, note FROM orders LIMIT 30",
					"output_truncated":     true,
					"cell_truncated_count": 1,
				},
			},
		}},
		Timestamp: time.Now(),
	}

	messages := engine.appendToolResults(nil, step)
	require.Len(t, messages, 2)
	require.Len(t, engine.pendingContextGroups, 2)
	assert.Equal(t, fullOutput, messages[1].Content)
	assert.Contains(t, engine.pendingContextGroups[1].Content, "Historical database query summary")
	assert.Contains(t, engine.pendingContextGroups[1].Content, "Database query summary")
	assert.Contains(t, engine.pendingContextGroups[1].Content, "Executed SQL: SELECT id, note FROM orders LIMIT 30")
	assert.Contains(t, engine.pendingContextGroups[1].Content, "Database state may have changed")
	assert.NotContains(t, engine.pendingContextGroups[1].Content, fullOutput)
	assert.NotContains(t, engine.pendingContextGroups[1].Content, longCell)
}

func TestAppendToolResults_RetainsFullExternalDatabaseHistoryWhenConfigured(t *testing.T) {
	engine := newTestEngine(t, &mockChat{})
	engine.config.RetainRetrievalHistory = true
	fullOutput := strings.Repeat("full query output ", 40)
	step := types.AgentStep{
		ToolCalls: []types.ToolCall{{
			ID:   "tool-1",
			Name: agenttools.ToolExternalDatabaseQuery,
			Args: map[string]interface{}{"sql": "SELECT id FROM orders LIMIT 10"},
			Result: &types.ToolResult{
				Success: true,
				Output:  fullOutput,
				Data: map[string]interface{}{
					"columns":   []string{"id"},
					"rows":      []map[string]interface{}{{"id": 1}},
					"row_count": 1,
				},
			},
		}},
	}

	messages := engine.appendToolResults(nil, step)
	require.Len(t, messages, 2)
	require.Len(t, engine.pendingContextGroups, 2)
	assert.Equal(t, fullOutput, messages[1].Content)
	assert.Equal(t, fullOutput, engine.pendingContextGroups[1].Content)
}

func TestAppendToolResults_BatchesAssistantAndAllToolResults(t *testing.T) {
	engine := newTestEngine(t, &mockChat{})
	step := types.AgentStep{
		Thought: "需要并行查看多个结果",
		ToolCalls: []types.ToolCall{
			{ID: "tool-1", Name: agenttools.ToolKnowledgeSearch, Args: map[string]interface{}{"query": "foo"}, Result: &types.ToolResult{Success: true, Output: "result-1"}},
			{ID: "tool-2", Name: agenttools.ToolWikiReadPage, Args: map[string]interface{}{"slug": "bar"}, Result: &types.ToolResult{Success: true, Output: "result-2"}},
		},
	}

	messages := engine.appendToolResults(nil, step)
	require.Len(t, messages, 3)
	require.Len(t, engine.pendingContextGroups, 3)
	assert.Equal(t, "assistant", engine.pendingContextGroups[0].Role)
	assert.Equal(t, "tool", engine.pendingContextGroups[1].Role)
	assert.Equal(t, "tool", engine.pendingContextGroups[2].Role)
}

func TestAppendToolResults_NilToolResultStillWritesProtocolResult(t *testing.T) {
	engine := newTestEngine(t, &mockChat{})
	step := types.AgentStep{
		Thought: "需要调用工具",
		ToolCalls: []types.ToolCall{{
			ID:     "tool-1",
			Name:   agenttools.ToolKnowledgeSearch,
			Args:   map[string]interface{}{"query": "foo"},
			Result: nil,
		}},
	}

	messages := engine.appendToolResults(nil, step)
	require.Len(t, messages, 2)
	require.Len(t, engine.pendingContextGroups, 2)
	assert.Equal(t, "tool", messages[1].Role)
	assert.Equal(t, "Error: tool returned no result", messages[1].Content)
	assert.Contains(t, engine.pendingContextGroups[1].Content, "no tool result")
}

func TestRedactHistoryKBResultsRedactsExternalDatabaseTools(t *testing.T) {
	messages := []chat.Message{
		{Role: "tool", Name: agenttools.ToolExternalDatabaseSchema, Content: "Database schema summary\nDatabase: crm", ToolCallID: "call-schema"},
		{Role: "tool", Name: agenttools.ToolExternalDatabaseQuery, Content: "Database query summary\nRow count: 5", ToolCallID: "call-query"},
	}

	redacted := redactHistoryKBResults(messages)
	require.Len(t, redacted, 2)
	assert.Contains(t, redacted[0].Content, "Database schema summary")
	assert.Contains(t, redacted[0].Content, "Database state may have changed")
	assert.Contains(t, redacted[1].Content, "Database query summary")
	assert.Contains(t, redacted[1].Content, "Database state may have changed")
}
