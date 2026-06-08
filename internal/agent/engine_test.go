package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock: chat.Chat
// ---------------------------------------------------------------------------

type mockResponse struct {
	chunks []types.StreamResponse
}

type mockChat struct {
	mu           sync.Mutex
	responses    []mockResponse
	callCount    int
	lastMessages []chat.Message
	lastOptions  *chat.ChatOptions
}

func (m *mockChat) ChatStream(_ context.Context, messages []chat.Message, opts *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastMessages = append([]chat.Message(nil), messages...)
	m.lastOptions = opts
	if m.callCount >= len(m.responses) {
		return nil, fmt.Errorf("unexpected ChatStream call #%d (only %d responses prepared)", m.callCount, len(m.responses))
	}
	resp := m.responses[m.callCount]
	m.callCount++

	ch := make(chan types.StreamResponse, len(resp.chunks))
	for _, chunk := range resp.chunks {
		ch <- chunk
	}
	close(ch)
	return ch, nil
}

func (m *mockChat) Chat(_ context.Context, messages []chat.Message, opts *chat.ChatOptions) (*types.ChatResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastMessages = append([]chat.Message(nil), messages...)
	m.lastOptions = opts
	return nil, fmt.Errorf("not implemented")
}

func (m *mockChat) GetModelName() string { return "mock-model" }
func (m *mockChat) GetModelID() string   { return "mock-id" }

type errorChat struct {
	mu           sync.Mutex
	err          error
	lastMessages []chat.Message
	lastOptions  *chat.ChatOptions
}

func (e *errorChat) ChatStream(_ context.Context, messages []chat.Message, opts *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastMessages = append([]chat.Message(nil), messages...)
	e.lastOptions = opts
	return nil, e.err
}

func (e *errorChat) Chat(_ context.Context, messages []chat.Message, opts *chat.ChatOptions) (*types.ChatResponse, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastMessages = append([]chat.Message(nil), messages...)
	e.lastOptions = opts
	return nil, e.err
}

func (e *errorChat) GetModelName() string { return "error-model" }
func (e *errorChat) GetModelID() string   { return "error-model-id" }

type cancelOnStreamChat struct {
	cancel context.CancelFunc
	chunks []types.StreamResponse
}

func (m *cancelOnStreamChat) ChatStream(context.Context, []chat.Message, *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	ch := make(chan types.StreamResponse, len(m.chunks))
	for _, chunk := range m.chunks {
		ch <- chunk
	}
	if m.cancel != nil {
		m.cancel()
	}
	close(ch)
	return ch, nil
}

func (m *cancelOnStreamChat) Chat(context.Context, []chat.Message, *chat.ChatOptions) (*types.ChatResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *cancelOnStreamChat) GetModelName() string { return "cancel-on-stream-model" }
func (m *cancelOnStreamChat) GetModelID() string   { return "cancel-on-stream-model-id" }

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

type testEngineOption func(*types.AgentConfig)

func withMaxIterations(n int) testEngineOption {
	return func(cfg *types.AgentConfig) {
		cfg.MaxIterations = n
	}
}

func withThinking(enabled bool) testEngineOption {
	return func(cfg *types.AgentConfig) {
		cfg.Thinking = &enabled
	}
}

func newTestEngine(t *testing.T, chatModel chat.Chat, opts ...testEngineOption) *AgentEngine {
	t.Helper()
	cfg := &types.AgentConfig{
		MaxIterations: 10,
		Temperature:   0.7,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	engine := NewAgentEngine(
		cfg,
		chatModel,
		nil,
		event.NewEventBus(),
		nil,
		nil,
		"test-session",
		"",
	)
	require.NotNil(t, engine, "NewAgentEngine returned nil (agenttoken.NewEstimator failed?)")
	return engine
}

func emptyMessages() []chat.Message {
	return []chat.Message{
		{Role: "system", Content: "You are a test agent."},
		{Role: "user", Content: "test query"},
	}
}

func emptyTools() []chat.Tool {
	return nil
}

func TestClassifyAgentExecutionErrorTimeouts(t *testing.T) {
	assert.Equal(t, "llm_timeout", classifyAgentExecutionError(errors.New("LLM call failed: LLM stream error: context deadline exceeded")))
	assert.Equal(t, "synthesis_timeout", classifyAgentExecutionError(errors.New("LLM call failed: foo (synthesis also failed: context deadline exceeded)")))
	assert.Equal(t, "synthesis_timeout", classifyFinalSynthesisError(errors.New("LLM stream error: context deadline exceeded")))
}

// ---------------------------------------------------------------------------
// TC1: Empty content + stop → should NOT complete with empty FinalAnswer
// ---------------------------------------------------------------------------

func TestExecuteLoop_EmptyContentWithStop_ShouldNotCompleteWithEmpty(t *testing.T) {
	// Simulate: LLM returns empty content with no tool calls (natural stop).
	// The stream closes with no content chunks → streamLLMToEventBus returns fullContent="".
	// streamThinkingToEventBus wraps it as ChatResponse{Content:"", FinishReason:"stop"}.
	// analyzeResponse() returns verdict{isDone:true, finalAnswer:""} → BUG: empty answer.
	//
	// Prepare 3 responses for initial attempt + 2 retries (after fix).
	mock := &mockChat{
		responses: []mockResponse{
			{chunks: []types.StreamResponse{{Done: true}}},
			{chunks: []types.StreamResponse{{Done: true}}},
			{chunks: []types.StreamResponse{{Done: true}}},
		},
	}

	engine := newTestEngine(t, mock)
	state := &types.AgentState{}
	ctx := context.Background()

	_, err := engine.executeLoop(ctx, state, "test query", emptyMessages(), emptyTools(), "sess-1", "msg-1")

	assert.NoError(t, err)
	assert.True(t, state.IsComplete)
	assert.NotEmpty(t, state.FinalAnswer,
		"BUG: FinalAnswer is empty when LLM returns empty content with stop. "+
			"analyzeResponse() should not allow empty content to be accepted as final answer.")
}

// ---------------------------------------------------------------------------
// TC2: Non-empty content + stop → normal completion (regression guard)
// ---------------------------------------------------------------------------

func TestExecuteLoop_NonEmptyContentWithStop_ShouldComplete(t *testing.T) {
	mock := &mockChat{
		responses: []mockResponse{
			{chunks: []types.StreamResponse{
				{Content: "Here is my answer", Done: true},
			}},
		},
	}

	engine := newTestEngine(t, mock)
	state := &types.AgentState{}
	ctx := context.Background()

	_, err := engine.executeLoop(ctx, state, "test query", emptyMessages(), emptyTools(), "sess-1", "msg-1")

	assert.NoError(t, err)
	assert.True(t, state.IsComplete)
	assert.Equal(t, "Here is my answer", state.FinalAnswer)
}

// ---------------------------------------------------------------------------
// TC4: Empty → retry with nudge → non-empty → success
// ---------------------------------------------------------------------------

func TestExecuteLoop_EmptyThenNonEmpty_ShouldRetryAndComplete(t *testing.T) {
	mock := &mockChat{
		responses: []mockResponse{
			// Round 1: empty content → triggers retry + nudge
			{chunks: []types.StreamResponse{{Done: true}}},
			// Round 2: after nudge, LLM produces answer
			{chunks: []types.StreamResponse{
				{Content: "Here is the answer.", Done: true},
			}},
		},
	}

	engine := newTestEngine(t, mock)
	state := &types.AgentState{}
	ctx := context.Background()

	_, err := engine.executeLoop(ctx, state, "test query", emptyMessages(), emptyTools(), "sess-1", "msg-1")

	assert.NoError(t, err)
	assert.True(t, state.IsComplete)
	assert.Equal(t, "Here is the answer.", state.FinalAnswer)
}

// ---------------------------------------------------------------------------
// TC5: FinishReason propagation through streamThinkingToEventBus
// ---------------------------------------------------------------------------

func TestStreamThinkingToEventBus_PropagatesFinishReason(t *testing.T) {
	tests := []struct {
		name         string
		finishReason string
		wantReason   string
	}{
		{"stop", "stop", "stop"},
		{"tool_calls", "tool_calls", "tool_calls"},
		{"length", "length", "length"},
		{"empty_fallback", "", "stop"}, // empty FinishReason → fallback to "stop"
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockChat{
				responses: []mockResponse{
					{chunks: []types.StreamResponse{
						{Content: "test content", Done: true, FinishReason: tt.finishReason},
					}},
				},
			}

			engine := newTestEngine(t, mock)
			ctx := context.Background()
			msgs := []chat.Message{{Role: "user", Content: "test"}}
			tools := []chat.Tool{}

			resp, err := engine.streamThinkingToEventBus(ctx, msgs, tools, 0, "sess-1")

			assert.NoError(t, err)
			assert.Equal(t, tt.wantReason, resp.FinishReason)
		})
	}
}

// TestStreamThinkingToEventBus_RoutesReasoningAndAnswerSeparately is the
// regression guard for the "answer first shows under Thinking, then jumps to
// the answer area" UX bug. A natural-stop response that carries reasoning in
// the dedicated reasoning channel (ResponseTypeThinking) plus plain answer
// content (ResponseTypeAnswer) must route the reasoning to thought events and
// the answer live to final-answer events — never the reverse.
func TestStreamThinkingToEventBus_RoutesReasoningAndAnswerSeparately(t *testing.T) {
	mock := &mockChat{
		responses: []mockResponse{
			{chunks: []types.StreamResponse{
				{ResponseType: types.ResponseTypeThinking, Content: "let me reason"},
				{ResponseType: types.ResponseTypeThinking, Content: "", Done: true},
				{ResponseType: types.ResponseTypeAnswer, Content: "The answer "},
				{ResponseType: types.ResponseTypeAnswer, Content: "is 42.", Done: true, FinishReason: "stop"},
			}},
		},
	}

	engine := newTestEngine(t, mock)
	var thoughts, answers string
	engine.eventBus.On(event.EventAgentThought, func(_ context.Context, evt event.Event) error {
		if d, ok := evt.Data.(event.AgentThoughtData); ok {
			thoughts += d.Content
		}
		return nil
	})
	engine.eventBus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		if d, ok := evt.Data.(event.AgentFinalAnswerData); ok {
			answers += d.Content
		}
		return nil
	})

	resp, err := engine.streamThinkingToEventBus(context.Background(),
		emptyMessages(), emptyTools(), 0, "sess-1")
	require.NoError(t, err)

	assert.Equal(t, "let me reason", thoughts, "reasoning_content must stream to thought events")
	assert.Equal(t, "The answer is 42.", answers, "plain answer content must stream live to final-answer events")
	assert.True(t, resp.AnswerStreamed, "AnswerStreamed must be set when answer text was streamed live")
	assert.NotEmpty(t, resp.AnswerEventID, "AnswerEventID must identify the live answer stream")
}

// TestStreamThinkingToEventBus_SplitsInlineThinkBlock verifies that models which
// embed reasoning inline as <think>…</think> in the content channel still have
// their reasoning routed to thought events and only the real answer streamed to
// the final-answer area.
func TestStreamThinkingToEventBus_SplitsInlineThinkBlock(t *testing.T) {
	mock := &mockChat{
		responses: []mockResponse{
			{chunks: []types.StreamResponse{
				{ResponseType: types.ResponseTypeAnswer, Content: "<think>hidden reasoning</think>Visible answer.",
					Done: true, FinishReason: "stop"},
			}},
		},
	}

	engine := newTestEngine(t, mock)
	var thoughts, answers string
	engine.eventBus.On(event.EventAgentThought, func(_ context.Context, evt event.Event) error {
		if d, ok := evt.Data.(event.AgentThoughtData); ok {
			thoughts += d.Content
		}
		return nil
	})
	engine.eventBus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		if d, ok := evt.Data.(event.AgentFinalAnswerData); ok {
			answers += d.Content
		}
		return nil
	})

	_, err := engine.streamThinkingToEventBus(context.Background(),
		emptyMessages(), emptyTools(), 0, "sess-1")
	require.NoError(t, err)

	assert.Equal(t, "hidden reasoning", thoughts, "inline <think> content must route to thought events")
	assert.Equal(t, "Visible answer.", answers, "answer outside <think> must stream to final-answer events")
}

// TestExecuteLoop_NaturalStop_DoesNotDuplicateAnswer ensures the natural-stop
// branch does not re-emit the full answer (it was already streamed live), so
// the final-answer content appears exactly once instead of streaming under
// Thinking and then "jumping" to a duplicate answer block.
func TestExecuteLoop_NaturalStop_DoesNotDuplicateAnswer(t *testing.T) {
	mock := &mockChat{
		responses: []mockResponse{
			{chunks: []types.StreamResponse{
				{ResponseType: types.ResponseTypeAnswer, Content: "Hello "},
				{ResponseType: types.ResponseTypeAnswer, Content: "world", Done: true, FinishReason: "stop"},
			}},
		},
	}

	engine := newTestEngine(t, mock)
	var answerContent string
	var doneCount int
	engine.eventBus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		if d, ok := evt.Data.(event.AgentFinalAnswerData); ok {
			answerContent += d.Content
			if d.Done {
				doneCount++
			}
		}
		return nil
	})

	state := &types.AgentState{}
	_, err := engine.executeLoop(context.Background(), state, "test query",
		emptyMessages(), emptyTools(), "sess-1", "msg-1")
	require.NoError(t, err)

	assert.True(t, state.IsComplete)
	assert.Equal(t, "Hello world", state.FinalAnswer)
	assert.Equal(t, "Hello world", answerContent,
		"answer content must be emitted exactly once (streamed live, not re-emitted by the natural-stop branch)")
	assert.GreaterOrEqual(t, doneCount, 1, "a Done marker must close the answer stream")
}

func TestStreamFinalAnswerToEventBus_EmitsDoneWhenProviderEndsWithEmptyChunk(t *testing.T) {
	mock := &mockChat{
		responses: []mockResponse{
			{chunks: []types.StreamResponse{
				{ResponseType: types.ResponseTypeAnswer, Content: "final answer", Done: false},
				{ResponseType: types.ResponseTypeAnswer, Done: true, FinishReason: "stop"},
			}},
		},
	}

	engine := newTestEngine(t, mock)
	var finalAnswerEvents []event.AgentFinalAnswerData
	engine.eventBus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		require.True(t, ok)
		finalAnswerEvents = append(finalAnswerEvents, data)
		return nil
	})

	state := &types.AgentState{}
	err := engine.streamFinalAnswerToEventBus(context.Background(), "test query", state, "sess-1")

	require.NoError(t, err)
	require.Len(t, finalAnswerEvents, 2)
	assert.Equal(t, "final answer", finalAnswerEvents[0].Content)
	assert.False(t, finalAnswerEvents[0].Done)
	assert.Empty(t, finalAnswerEvents[1].Content)
	assert.True(t, finalAnswerEvents[1].Done)
	assert.Equal(t, "final answer", state.FinalAnswer)
}

func TestCallLLMWithRetry_GracefulDegradationRewritesRecoveredState(t *testing.T) {
	mock := &mockChat{
		responses: []mockResponse{
			{chunks: []types.StreamResponse{{ResponseType: types.ResponseTypeError, Content: "tool backend failed"}}},
			{chunks: []types.StreamResponse{{ResponseType: types.ResponseTypeAnswer, Content: "recovered answer", Done: true}}},
		},
	}

	engine := newTestEngine(t, mock)
	state := &types.AgentState{
		RoundSteps: []types.AgentStep{{
			Iteration: 0,
			ToolCalls: []types.ToolCall{{
				Name:   "knowledge_retrieval",
				Result: &types.ToolResult{Success: true, Output: "retrieved context"},
			}},
		}},
	}

	var finalAnswerEvents []event.AgentFinalAnswerData
	engine.eventBus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		require.True(t, ok)
		finalAnswerEvents = append(finalAnswerEvents, data)
		return nil
	})

	resp, err := engine.callLLMWithRetry(context.Background(), emptyMessages(), emptyTools(), state, "test query", 0, "sess-1")
	require.NoError(t, err)
	assert.Nil(t, resp)
	assert.True(t, state.IsComplete)
	assert.Equal(t, "partial", state.CompletionStatus)
	assert.Equal(t, "fallback_stop", state.FinishReason)
	assert.Empty(t, state.FailureReason)
	assert.False(t, state.AllowIndexing)
	assert.False(t, state.AllowComplete)
	assert.Equal(t, "recovered answer", state.FinalAnswer)
	require.NotEmpty(t, finalAnswerEvents)
	assert.Equal(t, "partial", finalAnswerEvents[0].CompletionStatus)
	assert.Equal(t, "fallback_stop", finalAnswerEvents[0].FinishReason)
	assert.Empty(t, finalAnswerEvents[0].FailureReason)
	assert.False(t, finalAnswerEvents[0].AllowIndexing)
	assert.False(t, finalAnswerEvents[0].AllowComplete)
}

func TestCallLLMWithRetry_SanitizesMessagesBeforeProviderRequest(t *testing.T) {
	errModel := &errorChat{err: errors.New("provider rejected request")}
	engine := newTestEngine(t, errModel)
	state := &types.AgentState{}
	messages := []chat.Message{
		{Role: "system", Content: "sys"},
		{Role: "assistant", Content: "I searched", ToolCalls: []chat.ToolCall{{ID: "call-1", Function: chat.FunctionCall{Name: "search"}}}},
		{Role: "tool", Content: "stale result", Name: "search"},
		{Role: "user", Content: "next question"},
	}

	resp, err := engine.callLLMWithRetry(context.Background(), messages, emptyTools(), state, "test query", 0, "sess-1")
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.NotEmpty(t, errModel.lastMessages)
	for _, msg := range errModel.lastMessages {
		if msg.Role == "tool" {
			t.Fatalf("unexpected tool message sent to provider: %+v", msg)
		}
	}
	assert.Empty(t, agenttools.ValidateToolMessageProtocol(errModel.lastMessages))
}

func TestExecuteLoop_EmitsFailedCompletionOnLLMError(t *testing.T) {
	errModel := &errorChat{err: errors.New("provider timeout")}
	engine := newTestEngine(t, errModel)
	state := &types.AgentState{}

	var completes []event.AgentCompleteData
	engine.eventBus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	_, err := engine.executeLoop(context.Background(), state, "test query", emptyMessages(), emptyTools(), "sess-1", "msg-1")
	require.Error(t, err)
	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusFailed, state.CompletionStatus)
	assert.Equal(t, "llm_timeout", state.FinishReason)
	assert.Equal(t, "llm_timeout", state.FailureReason)
	assert.False(t, state.AllowIndexing)
	assert.False(t, state.AllowComplete)
	assert.Equal(t, types.MessageCompletionStatusFailed, completes[0].CompletionStatus)
	assert.Equal(t, "llm_timeout", completes[0].FinishReason)
	assert.Equal(t, "llm_timeout", completes[0].FailureReason)
	assert.False(t, completes[0].AllowIndexing)
	assert.False(t, completes[0].AllowComplete)
}

func TestExecuteLoop_EmitsCancelledCompletionWhenContextCancelledBeforeTools(t *testing.T) {
	engine := newTestEngine(t, &mockChat{})
	state := &types.AgentState{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var completes []event.AgentCompleteData
	engine.eventBus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	_, err := engine.executeLoop(ctx, state, "test query", emptyMessages(), emptyTools(), "sess-1", "msg-1")
	require.Error(t, err)
	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusCancelled, state.CompletionStatus)
	assert.Equal(t, "cancelled", state.FinishReason)
	assert.Equal(t, "cancelled", state.FailureReason)
	assert.False(t, state.AllowIndexing)
	assert.False(t, state.AllowComplete)
	assert.Equal(t, types.MessageCompletionStatusCancelled, completes[0].CompletionStatus)
	assert.Equal(t, "cancelled", completes[0].FinishReason)
	assert.Equal(t, "cancelled", completes[0].FailureReason)
	assert.False(t, completes[0].AllowIndexing)
	assert.False(t, completes[0].AllowComplete)
}

func TestExecuteLoop_EmitsCancelledCompletionWhenContextCancelledDuringLLMStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	chatModel := &cancelOnStreamChat{
		cancel: cancel,
		chunks: []types.StreamResponse{
			{ResponseType: types.ResponseTypeThinking, Content: "正在生成"},
			{ResponseType: types.ResponseTypeAnswer, Content: "partial answer", FinishReason: "stop", Done: true},
		},
	}
	engine := newTestEngine(t, chatModel)
	state := &types.AgentState{}

	var completes []event.AgentCompleteData
	engine.eventBus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	_, err := engine.executeLoop(ctx, state, "test query", emptyMessages(), emptyTools(), "sess-1", "msg-1")
	require.NoError(t, err)
	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusCancelled, state.CompletionStatus)
	assert.Equal(t, "cancelled", state.FinishReason)
	assert.Equal(t, "cancelled", state.FailureReason)
	assert.True(t, state.IsComplete)
	assert.False(t, state.AllowIndexing)
	assert.False(t, state.AllowComplete)
	assert.Equal(t, types.MessageCompletionStatusCancelled, completes[0].CompletionStatus)
	assert.Equal(t, "cancelled", completes[0].FinishReason)
	assert.Equal(t, "cancelled", completes[0].FailureReason)
	assert.False(t, completes[0].AllowIndexing)
	assert.False(t, completes[0].AllowComplete)
	require.Len(t, completes[0].AgentSteps, 1)
}
