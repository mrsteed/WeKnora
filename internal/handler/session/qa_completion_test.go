package session

import (
	"context"
	"errors"
	"strings"
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

type chatDocumentArtifactServiceStub struct {
	registeredArtifacts []*types.Message
	registerErr         error
	sessionArtifacts    []*types.ChatDocumentArtifact
}

func (s *chatDocumentArtifactServiceStub) DetectIntent(context.Context, string, string, string) (*types.DocumentIntentResult, error) {
	return &types.DocumentIntentResult{Intent: types.ChatDocumentIntentNormal, Operation: types.ChatDocumentOperationCreate}, nil
}

func (s *chatDocumentArtifactServiceStub) GetLatestArtifact(context.Context, string) (*types.ChatDocumentArtifact, error) {
	return nil, nil
}

func (s *chatDocumentArtifactServiceStub) GetArtifact(context.Context, string) (*types.ChatDocumentArtifact, error) {
	return nil, nil
}

func (s *chatDocumentArtifactServiceStub) GetArtifactBySourceMessageID(_ context.Context, sourceMessageID string) (*types.ChatDocumentArtifact, error) {
	for _, artifact := range s.sessionArtifacts {
		if artifact != nil && artifact.SourceMessageID == sourceMessageID {
			return artifact, nil
		}
	}
	return nil, nil
}

func (s *chatDocumentArtifactServiceStub) BuildQuotedContext(context.Context, *types.ChatDocumentArtifact, string, string, string) (string, error) {
	return "", nil
}

func (s *chatDocumentArtifactServiceStub) RegisterFromAssistantMessage(_ context.Context, message *types.Message, options types.RegisterChatDocumentArtifactOptions) (*types.ChatDocumentArtifact, error) {
	_ = options
	if s.registerErr != nil {
		return nil, s.registerErr
	}
	copyMsg := *message
	s.registeredArtifacts = append(s.registeredArtifacts, &copyMsg)
	return &types.ChatDocumentArtifact{ID: "artifact-1", SessionID: message.SessionID, RevisionNo: 1, Operation: types.ChatDocumentOperationCreate}, nil
}

func (s *chatDocumentArtifactServiceStub) ListBySession(context.Context, string, int) ([]*types.ChatDocumentArtifact, error) {
	return s.sessionArtifacts, nil
}

func (s *chatDocumentArtifactServiceStub) ListRevisions(context.Context, string) ([]*types.ChatDocumentArtifact, error) {
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
	artifactStub := &chatDocumentArtifactServiceStub{}
	handler := &Handler{messageService: stub, chatDocumentArtifactService: artifactStub}
	message := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant", Content: "final content"}
	var observedArtifact *types.ChatDocumentArtifact

	updated := handler.completeAssistantMessage(context.Background(), message, "query", assistantCompletionOptions{
		CompletionStatus: "completed",
		FinishReason:     "stop",
		AllowIndexing:    true,
		AllowComplete:    true,
		ArtifactObserver: func(artifact *types.ChatDocumentArtifact) {
			observedArtifact = artifact
		},
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
	require.Len(t, artifactStub.registeredArtifacts, 1)
	require.NotNil(t, observedArtifact)
	assert.Equal(t, "artifact-1", observedArtifact.ID)
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

func TestCompleteAssistantMessage_AgentModePrefersFallbackFinalAnswer(t *testing.T) {
	stub := &messageServiceStub{}
	handler := &Handler{messageService: stub}
	message := &types.Message{
		ID:        "msg-1",
		SessionID: "sess-1",
		RequestID: "req-1",
		Role:      "assistant",
		Content:   "partial streamed content",
	}

	updated := handler.completeAssistantMessage(context.Background(), message, "query", assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusPartial,
		FinishReason:     "fallback_stop",
		AllowIndexing:    false,
		AllowComplete:    false,
		AgentMode:        true,
		FinalAnswer:      "partial streamed content with authoritative recovered suffix",
	})

	assert.True(t, updated)
	require.Len(t, stub.updatedMessages, 1)
	assert.Equal(t, "partial streamed content with authoritative recovered suffix", message.Content)
	assert.Equal(t, "partial streamed content with authoritative recovered suffix", stub.updatedMessages[0].Content)
}

func TestCompleteAssistantMessage_AgentModeDoesNotReplaceWithShorterFinalAnswer(t *testing.T) {
	stub := &messageServiceStub{}
	handler := &Handler{messageService: stub}
	message := &types.Message{
		ID:        "msg-1",
		SessionID: "sess-1",
		RequestID: "req-1",
		Role:      "assistant",
		Content:   "streamed content already longer than final",
	}

	updated := handler.completeAssistantMessage(context.Background(), message, "query", assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "stop",
		AllowIndexing:    false,
		AllowComplete:    true,
		AgentMode:        true,
		FinalAnswer:      "short final",
	})

	assert.True(t, updated)
	require.Len(t, stub.updatedMessages, 1)
	assert.Equal(t, "streamed content already longer than final", message.Content)
	assert.Equal(t, "streamed content already longer than final", stub.updatedMessages[0].Content)
}

func TestRecoveredToolErrorCompletePayloadPersistsPartialFinalAnswer(t *testing.T) {
	stub := &messageServiceStub{}
	handler := &Handler{messageService: stub}
	message := &types.Message{
		ID:            "msg-1",
		SessionID:     "sess-1",
		RequestID:     "req-1",
		Role:          "assistant",
		Content:       "partial streamed answer",
		FinishReason:  "tool_error",
		FailureReason: "tool_error",
	}

	options := completionOptionsFromComplete(event.AgentCompleteData{
		CompletionStatus: types.MessageCompletionStatusPartial,
		FinishReason:     "fallback_stop",
		AllowIndexing:    false,
		AllowComplete:    false,
		FinalAnswer:      "partial streamed answer with authoritative recovered suffix",
		AgentSteps:       []types.AgentStep{{Iteration: 0, Thought: "first thought"}},
		TotalDurationMs:  789,
	})

	updated := handler.completeAssistantMessage(context.Background(), message, "query", options)

	assert.True(t, updated)
	require.Len(t, stub.updatedMessages, 1)
	assert.Equal(t, types.MessageCompletionStatusPartial, message.CompletionStatus)
	assert.Equal(t, "fallback_stop", message.FinishReason)
	assert.Equal(t, "", message.FailureReason)
	assert.False(t, message.IsCompleted)
	assert.Equal(t, "partial streamed answer with authoritative recovered suffix", message.Content)
	assert.Equal(t, int64(789), message.AgentDurationMs)
	assert.Equal(t, "partial streamed answer with authoritative recovered suffix", stub.updatedMessages[0].Content)
	assert.Equal(t, "fallback_stop", stub.updatedMessages[0].FinishReason)
	assert.Equal(t, "", stub.updatedMessages[0].FailureReason)
	assert.Equal(t, 0, stub.indexedCalls)
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

func TestEmitAssistantCompleteEvent_IncludesArtifactFinalDocumentMetadata(t *testing.T) {
	eventBus := event.NewEventBus()
	message := &types.Message{
		ID:        "msg-1",
		SessionID: "sess-1",
		Role:      "assistant",
		Content:   "delta content",
	}
	artifact := &types.ChatDocumentArtifact{
		ID:                "artifact-1",
		SessionID:         "sess-1",
		SourceMessageID:   "msg-1",
		RevisionNo:        2,
		Operation:         types.ChatDocumentOperationRevise,
		Status:            types.ChatDocumentArtifactStatusPartial,
		ArtifactKind:      types.ChatDocumentArtifactKindMarkdown,
		ContentSnapshot:   "# 完整文档",
		CanInlineContinue: true,
	}

	var captured event.AgentCompleteData
	eventBus.On(event.EventAgentComplete, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		captured = data
		return nil
	})

	emitAssistantCompleteEvent(eventBus, "sess-1", message, assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusPartial,
		FinishReason:     "fallback_stop",
		AllowIndexing:    false,
		AllowComplete:    false,
	}, artifact)

	assert.Equal(t, types.ChatDocumentFinalDocumentModeInlineSnapshot, captured.FinalDocumentMode)
	assert.Equal(t, "artifact-1", captured.FinalDocumentArtifactID)
	assert.Equal(t, "# 完整文档", captured.FinalDocument)
	require.NotNil(t, captured.Extra)
	metadata, ok := captured.Extra["chat_document_artifact"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "artifact-1", metadata["id"])
	assert.Equal(t, "msg-1", metadata["source_message_id"])
}

func TestEmitAssistantCompleteEvent_UsesFetchModeForOversizedSnapshot(t *testing.T) {
	eventBus := event.NewEventBus()
	message := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant", Content: "delta"}
	artifact := &types.ChatDocumentArtifact{
		ID:              "artifact-oversized",
		SessionID:       "sess-1",
		SourceMessageID: "msg-1",
		RevisionNo:      3,
		ContentSnapshot: strings.Repeat("超长正文", types.ChatDocumentArtifactInlineContinuationMaxChars/4+10),
	}

	var captured event.AgentCompleteData
	eventBus.On(event.EventAgentComplete, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		captured = data
		return nil
	})

	emitAssistantCompleteEvent(eventBus, "sess-1", message, assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "stop",
		AllowIndexing:    true,
		AllowComplete:    true,
	}, artifact)

	assert.Equal(t, types.ChatDocumentFinalDocumentModeFetchArtifactSnapshot, captured.FinalDocumentMode)
	assert.Equal(t, "artifact-oversized", captured.FinalDocumentArtifactID)
	assert.Empty(t, captured.FinalDocument)
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
