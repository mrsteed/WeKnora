package session

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type streamManagerStub struct {
	events []interfaces.StreamEvent
}

func (s *streamManagerStub) AppendEvent(ctx context.Context, sessionID, messageID string, evt interfaces.StreamEvent) error {
	s.events = append(s.events, evt)
	return nil
}

func (s *streamManagerStub) GetEvents(ctx context.Context, sessionID, messageID string, fromOffset int) ([]interfaces.StreamEvent, int, error) {
	if fromOffset >= len(s.events) {
		return nil, len(s.events), nil
	}
	return s.events[fromOffset:], len(s.events), nil
}

func TestHandleComplete_UsesStreamedAnswerWithoutAppendingCompletePayload(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", assistant, streamStub, event.NewEventBus())

	require.NoError(t, handler.handleFinalAnswer(context.Background(), event.Event{
		ID:   "answer-1",
		Data: event.AgentFinalAnswerData{Content: "streamed answer", Done: false},
	}))
	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "should-not-be-appended",
			CompletionStatus: types.MessageCompletionStatusCompleted,
			FinishReason:     "stop",
			TotalDurationMs:  321,
		},
	}))

	assert.Equal(t, "streamed answer", assistant.Content)
	assert.Equal(t, types.MessageCompletionStatusCompleted, assistant.CompletionStatus)
	assert.Equal(t, "stop", assistant.FinishReason)
	assert.True(t, assistant.IsCompleted)

	answerEvents := 0
	completeEvents := 0
	for _, evt := range streamStub.events {
		if evt.Type == types.ResponseTypeAnswer {
			answerEvents++
			assert.NotEqual(t, "should-not-be-appended", evt.Content)
		}
		if evt.Type == types.ResponseTypeComplete {
			completeEvents++
			assert.Equal(t, "streamed answer", evt.Data["final_answer"])
		}
	}
	assert.Equal(t, 1, answerEvents)
	assert.Equal(t, 1, completeEvents)
}

func TestHandleComplete_FallbackAnswerOnlyForCompletedWithoutStreamedAnswer(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", assistant, streamStub, event.NewEventBus())

	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "fallback answer",
			CompletionStatus: types.MessageCompletionStatusCompleted,
			FinishReason:     "stop",
			TotalDurationMs:  time.Second.Milliseconds(),
		},
	}))

	assert.Equal(t, "fallback answer", assistant.Content)

	answerEvents := 0
	completeEvents := 0
	for _, evt := range streamStub.events {
		switch evt.Type {
		case types.ResponseTypeAnswer:
			answerEvents++
		case types.ResponseTypeComplete:
			completeEvents++
			assert.Equal(t, "fallback answer", evt.Data["final_answer"])
		}
	}
	assert.Equal(t, 2, answerEvents)
	assert.Equal(t, 1, completeEvents)
}

func TestHandleComplete_PrefersAuthoritativeCompleteAnswerOverPartialStreamedContent(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", assistant, streamStub, event.NewEventBus())

	require.NoError(t, handler.handleFinalAnswer(context.Background(), event.Event{
		ID:   "answer-1",
		Data: event.AgentFinalAnswerData{Content: "partial preface", Done: false},
	}))
	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "authoritative full answer",
			CompletionStatus: types.MessageCompletionStatusCompleted,
			FinishReason:     "tool_calls",
			TotalDurationMs:  321,
		},
	}))

	assert.Equal(t, "authoritative full answer", assistant.Content)
	assert.Equal(t, types.MessageCompletionStatusCompleted, assistant.CompletionStatus)
	assert.Equal(t, "tool_calls", assistant.FinishReason)
	assert.True(t, assistant.IsCompleted)

	answerEvents := 0
	completeEvents := 0
	for _, evt := range streamStub.events {
		if evt.Type == types.ResponseTypeAnswer {
			answerEvents++
		}
		if evt.Type == types.ResponseTypeComplete {
			completeEvents++
			assert.Equal(t, "authoritative full answer", evt.Data["final_answer"])
		}
	}
	assert.Equal(t, 1, answerEvents)
	assert.Equal(t, 1, completeEvents)
}

func TestHandleComplete_PartialDoesNotFallbackFromCompletePayload(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", assistant, streamStub, event.NewEventBus())

	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "partial final answer",
			CompletionStatus: types.MessageCompletionStatusPartial,
			FinishReason:     "length",
			FailureReason:    "length",
		},
	}))

	assert.Empty(t, assistant.Content)
	assert.Equal(t, types.MessageCompletionStatusPartial, assistant.CompletionStatus)
	assert.False(t, assistant.IsCompleted)

	answerEvents := 0
	for _, evt := range streamStub.events {
		if evt.Type == types.ResponseTypeAnswer {
			answerEvents++
		}
	}
	assert.Equal(t, 0, answerEvents)
}
