package agent

import (
	"context"
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
				Result: &types.ToolResult{
					Success: true,
					Output:  "result text",
				},
			}},
			Timestamp: time.Now(),
		}

		out := engine.appendToolResults(nil, step)

		require.Len(t, out, 2, "expect one assistant + one tool message")
		assert.Equal(t, "assistant", out[0].Role)
		assert.Equal(t, "I will call search.", out[0].Content)
		assert.Equal(t, "Detailed chain of thought from MiMo/DeepSeek.", out[0].ReasoningContent,
			"reasoning_content must be propagated to the assistant message so providers like MiMo "+
				"and DeepSeek thinking-mode see it on the next round (issue #1302)")
		require.Len(t, out[0].ToolCalls, 1)
		assert.Equal(t, "call_1", out[0].ToolCalls[0].ID)

		assert.Equal(t, "tool", out[1].Role)
		assert.Equal(t, "result text", out[1].Content)
	})

	t.Run("reasoning_content alone produces an assistant message", func(t *testing.T) {
		// A pure thinking emission with no visible content / tool calls is
		// unusual but legal — preserve it so the next round's request still
		// carries reasoning_content for strict providers.
		step := types.AgentStep{
			Iteration:        0,
			ReasoningContent: "reasoning only",
			Timestamp:        time.Now(),
		}

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
		prior := []chat.Message{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "hi"},
		}
		step := types.AgentStep{
			Iteration:        1,
			Thought:          "answer",
			ReasoningContent: "thinking",
			Timestamp:        time.Now(),
		}
		out := engine.appendToolResults(prior, step)
		require.Len(t, out, 3)
		assert.Equal(t, "system", out[0].Role)
		assert.Equal(t, "user", out[1].Role)
		assert.Equal(t, "assistant", out[2].Role)
		assert.Equal(t, "thinking", out[2].ReasoningContent)
	})
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
