package session

import (
	"context"
	"strings"
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
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", time.Time{}, assistant, streamStub, event.NewEventBus(), nil)

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
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", time.Time{}, assistant, streamStub, event.NewEventBus(), nil)

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
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", time.Time{}, assistant, streamStub, event.NewEventBus(), nil)

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
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", time.Time{}, assistant, streamStub, event.NewEventBus(), nil)

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

func TestHandleComplete_RecoveredPartialFallsBackFromCompletePayload(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", time.Time{}, assistant, streamStub, event.NewEventBus(), nil)

	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "recovered partial final answer",
			CompletionStatus: types.MessageCompletionStatusPartial,
			FinishReason:     "fallback_stop",
			FailureReason:    "",
			TotalDurationMs:  time.Second.Milliseconds(),
		},
	}))

	assert.Equal(t, "recovered partial final answer", assistant.Content)
	assert.Equal(t, types.MessageCompletionStatusPartial, assistant.CompletionStatus)
	assert.Equal(t, "fallback_stop", assistant.FinishReason)
	assert.False(t, assistant.IsCompleted)

	answerEvents := 0
	completeEvents := 0
	for _, evt := range streamStub.events {
		switch evt.Type {
		case types.ResponseTypeAnswer:
			answerEvents++
		case types.ResponseTypeComplete:
			completeEvents++
			assert.Equal(t, "recovered partial final answer", evt.Data["final_answer"])
			assert.Equal(t, types.MessageCompletionStatusPartial, evt.Data["completion_status"])
			assert.Equal(t, "fallback_stop", evt.Data["finish_reason"])
		}
	}
	assert.Equal(t, 2, answerEvents)
	assert.Equal(t, 1, completeEvents)
}

func TestHandleComplete_AppendsAgentStepsToCompletePayload(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", time.Time{}, assistant, streamStub, event.NewEventBus(), nil)

	steps := []types.AgentStep{{
		Iteration: 0,
		Thought:   "first thought",
	}}

	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "final answer",
			CompletionStatus: types.MessageCompletionStatusCompleted,
			FinishReason:     "stop",
			TotalDurationMs:  456,
			AgentSteps:       steps,
			TotalSteps:       len(steps),
		},
	}))

	require.Len(t, streamStub.events, 3)
	completeEvent := streamStub.events[len(streamStub.events)-1]
	require.Equal(t, types.ResponseTypeComplete, completeEvent.Type)
	require.NotNil(t, completeEvent.Data)

	assert.Equal(t, "final answer", completeEvent.Data["final_answer"])
	assert.Equal(t, int64(456), completeEvent.Data["agent_duration_ms"])
	assert.Equal(t, int64(456), completeEvent.Data["total_duration_ms"])

	streamedSteps, ok := completeEvent.Data["agent_steps"].(types.AgentSteps)
	if !ok {
		legacySteps, ok := completeEvent.Data["agent_steps"].([]types.AgentStep)
		require.True(t, ok)
		assert.Equal(t, steps, legacySteps)
		return
	}
	assert.Equal(t, types.AgentSteps(steps), streamedSteps)
}

func TestHandleComplete_IncludesChatDocumentArtifactMetadata(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(
		context.Background(),
		"sess-1",
		"msg-1",
		"req-1",
		time.Time{},
		assistant,
		streamStub,
		event.NewEventBus(),
		func() *types.ChatDocumentArtifact {
			return &types.ChatDocumentArtifact{ID: "artifact-1", SessionID: "sess-1", SourceMessageID: "msg-1", RevisionNo: 2, Title: "技术方案", Status: types.ChatDocumentArtifactStatusAvailable, Operation: types.ChatDocumentOperationContinue, CanInlineContinue: true, QualityIssues: []string{"unclosed_code_fence"}, UserHint: "检测到末尾代码块未闭合，系统已自动补全代码围栏。", ContentSnapshot: "# 完整文档\n\n## 第一章"}
		},
	)

	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "final answer",
			CompletionStatus: types.MessageCompletionStatusCompleted,
			FinishReason:     "stop",
		},
	}))

	completeEvent := streamStub.events[len(streamStub.events)-1]
	require.Equal(t, types.ResponseTypeComplete, completeEvent.Type)
	artifact, ok := completeEvent.Data["chat_document_artifact"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "artifact-1", artifact["id"])
	assert.Equal(t, "msg-1", artifact["source_message_id"])
	assert.Equal(t, types.ChatDocumentOperationContinue, artifact["operation"])
	assert.Equal(t, true, artifact["can_inline_continue"])
	assert.Equal(t, "检测到末尾代码块未闭合，系统已自动补全代码围栏。", artifact["user_hint"])
	assert.Equal(t, types.ChatDocumentFinalDocumentModeInlineSnapshot, completeEvent.Data["final_document_mode"])
	assert.Equal(t, "artifact-1", completeEvent.Data["final_document_artifact_id"])
	assert.Equal(t, "# 完整文档\n\n## 第一章", completeEvent.Data["final_document"])
}

func TestHandleComplete_UsesFetchModeForOversizedArtifactSnapshot(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(
		context.Background(),
		"sess-1",
		"msg-1",
		"req-1",
		time.Time{},
		assistant,
		streamStub,
		event.NewEventBus(),
		func() *types.ChatDocumentArtifact {
			return &types.ChatDocumentArtifact{
				ID:              "artifact-oversized",
				SessionID:       "sess-1",
				SourceMessageID: "msg-1",
				RevisionNo:      4,
				ContentSnapshot: strings.Repeat("超长正文", types.ChatDocumentArtifactInlineContinuationMaxChars/4+10),
			}
		},
	)

	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "final answer",
			CompletionStatus: types.MessageCompletionStatusCompleted,
			FinishReason:     "stop",
		},
	}))

	completeEvent := streamStub.events[len(streamStub.events)-1]
	require.Equal(t, types.ResponseTypeComplete, completeEvent.Type)
	assert.Equal(t, types.ChatDocumentFinalDocumentModeFetchArtifactSnapshot, completeEvent.Data["final_document_mode"])
	assert.Equal(t, "artifact-oversized", completeEvent.Data["final_document_artifact_id"])
	_, hasInlineDocument := completeEvent.Data["final_document"]
	assert.False(t, hasInlineDocument)
}
