package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type messageServiceStub struct {
	mu               sync.Mutex
	getMessageResult *types.Message
	getMessageErr    error
	updatedMessages  []*types.Message
	updateMessageErr error
	indexedCalls     int
	indexedOptions   []interfaces.MessageIndexOptions
	indexedCallCh    chan struct{}
}

func (s *messageServiceStub) CreateMessage(context.Context, *types.Message) (*types.Message, error) {
	return nil, nil
}

func (s *messageServiceStub) GetMessage(context.Context, string, string) (*types.Message, error) {
	return s.getMessageResult, s.getMessageErr
}

func (s *messageServiceStub) GetMessagesBySession(context.Context, string, int, int) ([]*types.Message, error) {
	return nil, nil
}

func (s *messageServiceStub) GetRecentMessagesBySession(context.Context, string, int) ([]*types.Message, error) {
	return nil, nil
}

func (s *messageServiceStub) GetMessagesBySessionBeforeTime(context.Context, string, time.Time, int) ([]*types.Message, error) {
	return nil, nil
}

func (s *messageServiceStub) UpdateMessage(_ context.Context, message *types.Message) error {
	if s.updateMessageErr != nil {
		return s.updateMessageErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	copyMsg := *message
	s.updatedMessages = append(s.updatedMessages, &copyMsg)
	return nil
}

func (s *messageServiceStub) UpdateMessageImages(context.Context, string, string, types.MessageImages) error {
	return nil
}

func (s *messageServiceStub) UpdateMessageRenderedContent(context.Context, string, string, string) error {
	return nil
}

func (s *messageServiceStub) DeleteMessage(context.Context, string, string) error { return nil }

func (s *messageServiceStub) ClearSessionMessages(context.Context, string) error { return nil }

func (s *messageServiceStub) SearchMessages(context.Context, *types.MessageSearchParams) (*types.MessageSearchResult, error) {
	return nil, nil
}

func (s *messageServiceStub) IndexMessageToKB(_ context.Context, _ string, _ string, _ string, _ string, options interfaces.MessageIndexOptions) {
	s.mu.Lock()
	s.indexedCalls++
	s.indexedOptions = append(s.indexedOptions, options)
	s.mu.Unlock()
	if s.indexedCallCh != nil {
		s.indexedCallCh <- struct{}{}
	}
}

func (s *messageServiceStub) DeleteMessageKnowledge(context.Context, string) {}

func (s *messageServiceStub) DeleteSessionKnowledge(context.Context, string) {}

func (s *messageServiceStub) GetChatHistoryKBStats(context.Context) (*types.ChatHistoryKBStats, error) {
	return nil, nil
}

func TestCompleteAssistantMessage_PartialDoesNotCompleteOrIndex(t *testing.T) {
	stub := &messageServiceStub{}
	handler := &Handler{messageService: stub}
	message := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant", Content: "partial content"}

	updated := handler.completeAssistantMessage(context.Background(), message, "query", assistantCompletionOptions{
		CompletionStatus: "partial",
		FinishReason:     "length",
		FailureReason:    "length",
		AllowIndexing:    false,
		AllowComplete:    false,
	})

	assert.True(t, updated)
	require.Len(t, stub.updatedMessages, 1)
	assert.False(t, message.IsCompleted)
	assert.Equal(t, types.MessageCompletionStatusPartial, message.CompletionStatus)
	assert.Equal(t, "length", message.FinishReason)
	assert.Equal(t, "length", message.FailureReason)
	assert.Equal(t, 0, stub.indexedCalls)
}

func TestCompleteAssistantMessage_CompletedIndexesWhenAllowed(t *testing.T) {
	stub := &messageServiceStub{indexedCallCh: make(chan struct{}, 1)}
	handler := &Handler{messageService: stub}
	message := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant", Content: "final content"}

	updated := handler.completeAssistantMessage(context.Background(), message, "query", assistantCompletionOptions{
		CompletionStatus: "completed",
		FinishReason:     "stop",
		AllowIndexing:    true,
		AllowComplete:    true,
	})

	assert.True(t, updated)
	require.Len(t, stub.updatedMessages, 1)
	assert.True(t, message.IsCompleted)
	assert.Equal(t, types.MessageCompletionStatusCompleted, message.CompletionStatus)
	assert.Equal(t, "stop", message.FinishReason)

	select {
	case <-stub.indexedCallCh:
	case <-time.After(time.Second):
		t.Fatal("expected asynchronous indexing call")
	}

	assert.Equal(t, 1, stub.indexedCalls)
	require.Len(t, stub.indexedOptions, 1)
	assert.Equal(t, "completed", stub.indexedOptions[0].CompletionStatus)
	assert.Equal(t, "stop", stub.indexedOptions[0].FinishReason)
	assert.True(t, stub.indexedOptions[0].AllowIndexing)
}

func TestCompleteAssistantMessage_AgentModePersistsExistingSteps(t *testing.T) {
	stub := &messageServiceStub{}
	handler := &Handler{messageService: stub}
	message := &types.Message{
		ID:         "msg-1",
		SessionID:  "sess-1",
		RequestID:  "req-1",
		Role:       "assistant",
		Content:    "final content",
		AgentSteps: types.AgentSteps{{Iteration: 0, Thought: "first thought"}},
	}

	updated := handler.completeAssistantMessage(context.Background(), message, "query", assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "stop",
		AllowIndexing:    false,
		AllowComplete:    true,
		AgentMode:        true,
	})

	assert.True(t, updated)
	require.Len(t, stub.updatedMessages, 1)
	assert.True(t, message.IsCompleted)
	assert.Equal(t, types.MessageCompletionStatusCompleted, message.CompletionStatus)
	assert.Equal(t, types.AgentSteps{{Iteration: 0, Thought: "first thought"}}, message.AgentSteps)
	assert.Equal(t, types.AgentSteps{{Iteration: 0, Thought: "first thought"}}, stub.updatedMessages[0].AgentSteps)
	assert.Equal(t, 0, stub.indexedCalls)
}

func TestCompleteAssistantMessage_AgentModeHydratesCompletionPayload(t *testing.T) {
	stub := &messageServiceStub{}
	handler := &Handler{messageService: stub}
	message := &types.Message{
		ID:        "msg-1",
		SessionID: "sess-1",
		RequestID: "req-1",
		Role:      "assistant",
	}

	updated := handler.completeAssistantMessage(context.Background(), message, "query", assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "tool_calls",
		AllowIndexing:    false,
		AllowComplete:    true,
		AgentMode:        true,
		FinalAnswer:      "final content",
		AgentSteps:       types.AgentSteps{{Iteration: 0, Thought: "first thought"}},
		AgentDurationMs:  3210,
	})

	assert.True(t, updated)
	require.Len(t, stub.updatedMessages, 1)
	assert.Equal(t, "final content", message.Content)
	assert.Equal(t, types.AgentSteps{{Iteration: 0, Thought: "first thought"}}, message.AgentSteps)
	assert.Equal(t, int64(3210), message.AgentDurationMs)
	assert.Equal(t, types.AgentSteps{{Iteration: 0, Thought: "first thought"}}, stub.updatedMessages[0].AgentSteps)
	assert.Equal(t, int64(3210), stub.updatedMessages[0].AgentDurationMs)
}

func TestCompletionOptionsFromComplete_MarksAgentMode(t *testing.T) {
	options := completionOptionsFromComplete(event.AgentCompleteData{
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "stop",
		AllowIndexing:    true,
		AllowComplete:    true,
		FinalAnswer:      "final content",
		AgentSteps:       []types.AgentStep{{Iteration: 0, Thought: "first thought"}},
		TotalDurationMs:  4567,
	})

	assert.True(t, options.AgentMode)
	assert.Equal(t, types.MessageCompletionStatusCompleted, options.CompletionStatus)
	assert.Equal(t, "stop", options.FinishReason)
	assert.Equal(t, "final content", options.FinalAnswer)
	assert.Equal(t, types.AgentSteps{{Iteration: 0, Thought: "first thought"}}, options.AgentSteps)
	assert.Equal(t, int64(4567), options.AgentDurationMs)
}

func TestCompleteAssistantMessage_DoesNotOverrideTerminalState(t *testing.T) {
	stub := &messageServiceStub{}
	handler := &Handler{messageService: stub}
	message := &types.Message{
		ID:               "msg-1",
		SessionID:        "sess-1",
		Role:             "assistant",
		Content:          "partial content",
		CompletionStatus: types.MessageCompletionStatusCancelled,
		FinishReason:     "cancelled",
		FailureReason:    "cancelled",
	}

	updated := handler.completeAssistantMessage(context.Background(), message, "query", assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "stop",
		AllowIndexing:    true,
		AllowComplete:    true,
	})

	assert.False(t, updated)
	assert.Empty(t, stub.updatedMessages)
	assert.Equal(t, types.MessageCompletionStatusCancelled, message.CompletionStatus)
	assert.Equal(t, 0, stub.indexedCalls)
}

func TestCompleteAssistantMessage_ReturnsFalseWhenUpdateFails(t *testing.T) {
	stub := &messageServiceStub{updateMessageErr: errors.New("update failed")}
	handler := &Handler{messageService: stub}
	message := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant", Content: "final content"}

	updated := handler.completeAssistantMessage(context.Background(), message, "query", assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "stop",
		AllowIndexing:    true,
		AllowComplete:    true,
	})

	assert.False(t, updated)
	assert.False(t, message.IsCompleted)
	assert.Empty(t, message.CompletionStatus)
	assert.Empty(t, stub.updatedMessages)
	assert.Equal(t, 0, stub.indexedCalls)
}

func TestMessageUpdateContext_PreservesTenantAfterParentCancellation(t *testing.T) {
	baseCtx, cancel := context.WithCancel(context.Background())
	cancel()

	updateCtx := messageUpdateContext(baseCtx, 42)

	assert.NoError(t, updateCtx.Err())
	tenantID, ok := updateCtx.Value(types.TenantIDContextKey).(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(42), tenantID)
}

func TestCompletionOptionsFromError_MarksFailedAndDisablesIndexing(t *testing.T) {
	options := completionOptionsFromError(event.ErrorData{Stage: "agent_execution", Error: "boom"})

	assert.Equal(t, types.MessageCompletionStatusFailed, options.CompletionStatus)
	assert.Equal(t, "error", options.FinishReason)
	assert.Equal(t, "agent_execution", options.FailureReason)
	assert.False(t, options.AllowIndexing)
	assert.False(t, options.AllowComplete)
}
