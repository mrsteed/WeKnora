package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleMaxIterations_MarksStateAsPartial(t *testing.T) {
	mock := &mockChat{
		responses: []mockResponse{
			{chunks: []types.StreamResponse{{Content: "assembled answer", Done: true}}},
		},
	}
	engine := newTestEngine(t, mock)
	state := &types.AgentState{
		RoundSteps: []types.AgentStep{{Iteration: 0}},
	}

	engine.handleMaxIterations(context.Background(), "test query", state, "sess-1")

	assert.True(t, state.IsComplete)
	assert.Equal(t, "partial", state.CompletionStatus)
	assert.Equal(t, "max_iterations", state.FinishReason)
	assert.Equal(t, "max_iterations", state.FailureReason)
	assert.False(t, state.AllowIndexing)
	assert.False(t, state.AllowComplete)
	assert.Equal(t, "assembled answer", state.FinalAnswer)
}

func TestStreamFinalAnswerToEventBus_UsesStateCompletionMetadata(t *testing.T) {
	mock := &mockChat{
		responses: []mockResponse{{chunks: []types.StreamResponse{{Content: "partial answer", Done: true}}}},
	}
	engine := newTestEngine(t, mock)
	state := &types.AgentState{
		CompletionStatus: "partial",
		FinishReason:     "max_iterations",
		FailureReason:    "max_iterations",
		AllowIndexing:    false,
		AllowComplete:    false,
		RoundSteps:       []types.AgentStep{{Iteration: 0}},
	}

	var emitted []event.AgentFinalAnswerData
	engine.eventBus.On(event.EventAgentFinalAnswer, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		require.True(t, ok)
		emitted = append(emitted, data)
		return nil
	})

	err := engine.streamFinalAnswerToEventBus(context.Background(), "test query", state, "sess-1")
	require.NoError(t, err)
	require.NotEmpty(t, emitted)
	assert.Equal(t, "partial", emitted[0].CompletionStatus)
	assert.Equal(t, "max_iterations", emitted[0].FinishReason)
	assert.False(t, emitted[0].AllowIndexing)
	assert.False(t, emitted[0].AllowComplete)
	assert.Equal(t, "max_iterations", emitted[0].FailureReason)
	assert.True(t, emitted[0].Done)
}

func TestStreamFinalAnswerToEventBus_CompressesLargeToolOutputsInSynthesisContext(t *testing.T) {
	mock := &mockChat{
		responses: []mockResponse{{chunks: []types.StreamResponse{{Content: "assembled answer", Done: true}}}},
	}
	engine := newTestEngine(t, mock)
	state := &types.AgentState{
		RoundSteps: []types.AgentStep{{
			Iteration: 0,
			ToolCalls: []types.ToolCall{{
				Name:   "external_database_query",
				Result: &types.ToolResult{Output: strings.Repeat("very long tool output ", 300)},
			}},
		}},
	}

	err := engine.streamFinalAnswerToEventBus(context.Background(), "test query", state, "sess-1")
	require.NoError(t, err)
	require.NotEmpty(t, mock.lastMessages)

	var toolSummary string
	for _, msg := range mock.lastMessages {
		if msg.Role == "user" && strings.Contains(msg.Content, "Tool external_database_query summary:") {
			toolSummary = msg.Content
			break
		}
	}
	require.NotEmpty(t, toolSummary)
	assert.Contains(t, toolSummary, "truncated for synthesis")
	assert.Less(t, len(toolSummary), len(strings.Repeat("very long tool output ", 300)))
}

func TestStreamFinalAnswerToEventBus_DisablesThinkingForFinalSynthesis(t *testing.T) {
	mock := &mockChat{
		responses: []mockResponse{{chunks: []types.StreamResponse{
			{Content: "part-1 ", Done: false},
			{Content: "part-2", Done: true},
		}}},
	}
	engine := newTestEngine(t, mock, withThinking(true))
	state := &types.AgentState{
		RoundSteps: []types.AgentStep{{Iteration: 0}},
	}

	err := engine.streamFinalAnswerToEventBus(context.Background(), "test query", state, "sess-1")
	require.NoError(t, err)
	require.NotNil(t, mock.lastOptions)
	require.NotNil(t, mock.lastOptions.Thinking)
	assert.False(t, *mock.lastOptions.Thinking)
	assert.Equal(t, "part-1 part-2", state.FinalAnswer)
}
