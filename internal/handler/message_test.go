package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type messageHandlerMessageServiceStub struct {
	recentMessages  []*types.Message
	updatedMessages []*types.Message
}

func (s *messageHandlerMessageServiceStub) CreateMessage(context.Context, *types.Message) (*types.Message, error) {
	return nil, nil
}

func (s *messageHandlerMessageServiceStub) GetMessage(context.Context, string, string) (*types.Message, error) {
	return nil, nil
}

func (s *messageHandlerMessageServiceStub) GetMessagesBySession(context.Context, string, int, int) ([]*types.Message, error) {
	return nil, nil
}

func (s *messageHandlerMessageServiceStub) GetRecentMessagesBySession(context.Context, string, int) ([]*types.Message, error) {
	return s.recentMessages, nil
}

func (s *messageHandlerMessageServiceStub) GetMessagesBySessionBeforeTime(context.Context, string, time.Time, int) ([]*types.Message, error) {
	return nil, nil
}

func (s *messageHandlerMessageServiceStub) UpdateMessage(_ context.Context, message *types.Message) error {
	copyMessage := *message
	s.updatedMessages = append(s.updatedMessages, &copyMessage)
	return nil
}

func (s *messageHandlerMessageServiceStub) UpdateMessageImages(context.Context, string, string, types.MessageImages) error {
	return nil
}

func (s *messageHandlerMessageServiceStub) UpdateMessageRenderedContent(context.Context, string, string, string) error {
	return nil
}

func (s *messageHandlerMessageServiceStub) DeleteMessage(context.Context, string, string) error {
	return nil
}

func (s *messageHandlerMessageServiceStub) ClearSessionMessages(context.Context, string) error {
	return nil
}

func (s *messageHandlerMessageServiceStub) SearchMessages(context.Context, *types.MessageSearchParams) (*types.MessageSearchResult, error) {
	return nil, nil
}

func (s *messageHandlerMessageServiceStub) IndexMessageToKB(context.Context, string, string, string, string, interfaces.MessageIndexOptions) {
}

func (s *messageHandlerMessageServiceStub) DeleteMessageKnowledge(context.Context, string) {}

func (s *messageHandlerMessageServiceStub) DeleteSessionKnowledge(context.Context, string) {}

func (s *messageHandlerMessageServiceStub) GetChatHistoryKBStats(context.Context) (*types.ChatHistoryKBStats, error) {
	return nil, nil
}

type messageHandlerSessionServiceStub struct{}

func (s *messageHandlerSessionServiceStub) CreateSession(context.Context, *types.Session) (*types.Session, error) {
	return nil, nil
}

func (s *messageHandlerSessionServiceStub) GetSession(context.Context, string) (*types.Session, error) {
	return &types.Session{ID: "sess-1"}, nil
}

func (s *messageHandlerSessionServiceStub) GetSessionsByTenant(context.Context) ([]*types.Session, error) {
	return nil, nil
}

func (s *messageHandlerSessionServiceStub) GetPagedSessionsByTenant(context.Context, *types.Pagination) (*types.PageResult, error) {
	return nil, nil
}

func (s *messageHandlerSessionServiceStub) UpdateSession(context.Context, *types.Session) error {
	return nil
}

func (s *messageHandlerSessionServiceStub) DeleteSession(context.Context, string) error { return nil }

func (s *messageHandlerSessionServiceStub) BatchDeleteSessions(context.Context, []string) error {
	return nil
}

func (s *messageHandlerSessionServiceStub) DeleteAllSessions(context.Context) error { return nil }

func (s *messageHandlerSessionServiceStub) ListSessions(context.Context, *types.SessionListQuery) (*types.PageResult, error) {
	return nil, nil
}

func (s *messageHandlerSessionServiceStub) SetSessionPinned(context.Context, string, bool) (int64, error) {
	return 0, nil
}

func (s *messageHandlerSessionServiceStub) GenerateTitle(context.Context, *types.Session, []types.Message, string) (string, error) {
	return "", nil
}

func (s *messageHandlerSessionServiceStub) GenerateTitleAsync(context.Context, *types.Session, string, string, *event.EventBus) {
}

func (s *messageHandlerSessionServiceStub) KnowledgeQA(context.Context, *types.QARequest, *event.EventBus) error {
	return nil
}

func (s *messageHandlerSessionServiceStub) KnowledgeQAByEvent(context.Context, *types.ChatManage, []types.EventType) error {
	return nil
}

func (s *messageHandlerSessionServiceStub) SearchKnowledge(context.Context, []string, []string, string) ([]*types.SearchResult, error) {
	return nil, nil
}

func (s *messageHandlerSessionServiceStub) AgentQA(context.Context, *types.QARequest, *event.EventBus) error {
	return nil
}

func (s *messageHandlerSessionServiceStub) ClearContext(context.Context, string) error { return nil }

type messageHandlerStreamManagerStub struct {
	eventsByMessageID map[string][]interfaces.StreamEvent
}

func (s *messageHandlerStreamManagerStub) AppendEvent(context.Context, string, string, interfaces.StreamEvent) error {
	return nil
}

func (s *messageHandlerStreamManagerStub) GetEvents(_ context.Context, _ string, messageID string, fromOffset int) ([]interfaces.StreamEvent, int, error) {
	events := s.eventsByMessageID[messageID]
	if fromOffset >= len(events) {
		return nil, len(events), nil
	}
	return events[fromOffset:], len(events), nil
}

func TestLoadMessages_ReconcilesCompletedAssistantFromStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	messageService := &messageHandlerMessageServiceStub{recentMessages: []*types.Message{{
		ID:               "assistant-1",
		SessionID:        "sess-1",
		RequestID:        "req-1",
		Role:             "assistant",
		Content:          "",
		CompletionStatus: types.MessageCompletionStatusPending,
	}}}
	handler := NewMessageHandler(
		messageService,
		&messageHandlerSessionServiceStub{},
		&messageHandlerStreamManagerStub{eventsByMessageID: map[string][]interfaces.StreamEvent{
			"assistant-1": {
				{Type: types.ResponseTypeAnswer, Content: "北下街当前有62名人员。", Done: true},
				{Type: types.ResponseTypeComplete, Done: true, Data: map[string]interface{}{
					"completion_status": types.MessageCompletionStatusCompleted,
					"finish_reason":     "stop",
					"final_answer":      "北下街当前有62名人员。",
				}},
			},
		}},
	)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/messages/sess-1/load?limit=20", nil)
	ctx.Params = gin.Params{{Key: "session_id", Value: "sess-1"}}

	handler.LoadMessages(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, messageService.updatedMessages, 1)
	assert.Equal(t, types.MessageCompletionStatusCompleted, messageService.updatedMessages[0].CompletionStatus)
	assert.True(t, messageService.updatedMessages[0].IsCompleted)
	assert.Equal(t, "北下街当前有62名人员。", messageService.updatedMessages[0].Content)

	var response struct {
		Success bool             `json:"success"`
		Data    []*types.Message `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Len(t, response.Data, 1)
	assert.Equal(t, types.MessageCompletionStatusCompleted, response.Data[0].CompletionStatus)
	assert.True(t, response.Data[0].IsCompleted)
	assert.Equal(t, "北下街当前有62名人员。", response.Data[0].Content)
}

func TestLoadMessages_MarksMissingStreamAsFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	messageService := &messageHandlerMessageServiceStub{recentMessages: []*types.Message{{
		ID:               "assistant-1",
		SessionID:        "sess-1",
		RequestID:        "req-1",
		Role:             "assistant",
		Content:          "",
		CompletionStatus: types.MessageCompletionStatusPending,
	}}}
	handler := NewMessageHandler(
		messageService,
		&messageHandlerSessionServiceStub{},
		&messageHandlerStreamManagerStub{eventsByMessageID: map[string][]interfaces.StreamEvent{}},
	)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/messages/sess-1/load?limit=20", nil)
	ctx.Params = gin.Params{{Key: "session_id", Value: "sess-1"}}

	handler.LoadMessages(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, messageService.updatedMessages, 1)
	assert.Equal(t, types.MessageCompletionStatusFailed, messageService.updatedMessages[0].CompletionStatus)
	assert.Equal(t, "stream_unavailable", messageService.updatedMessages[0].FinishReason)
	assert.Equal(t, "stream_unavailable", messageService.updatedMessages[0].FailureReason)
	assert.False(t, messageService.updatedMessages[0].IsCompleted)

	var response struct {
		Success bool             `json:"success"`
		Data    []*types.Message `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Len(t, response.Data, 1)
	assert.Equal(t, types.MessageCompletionStatusFailed, response.Data[0].CompletionStatus)
	assert.Equal(t, "stream_unavailable", response.Data[0].FailureReason)
	assert.False(t, response.Data[0].IsCompleted)
}

func TestLoadMessages_ReconcilesCompletedAssistantMissingAgentStepsFromStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	timestamp := time.Date(2026, time.May, 6, 12, 0, 0, 0, time.UTC)
	messageService := &messageHandlerMessageServiceStub{recentMessages: []*types.Message{{
		ID:               "assistant-1",
		SessionID:        "sess-1",
		RequestID:        "req-1",
		Role:             "assistant",
		Content:          "北下街当前有62名人员。",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		IsCompleted:      true,
		FinishReason:     "tool_calls",
	}}}
	handler := NewMessageHandler(
		messageService,
		&messageHandlerSessionServiceStub{},
		&messageHandlerStreamManagerStub{eventsByMessageID: map[string][]interfaces.StreamEvent{
			"assistant-1": {
				{Type: types.ResponseTypeComplete, Done: true, Data: map[string]interface{}{
					"completion_status": types.MessageCompletionStatusCompleted,
					"finish_reason":     "tool_calls",
					"final_answer":      "北下街当前有62名人员。",
					"agent_duration_ms": float64(3210),
					"agent_steps": []interface{}{
						map[string]interface{}{
							"iteration":         float64(0),
							"thought":           "先查询北下街人员数量",
							"reasoning_content": "",
							"tool_calls":        []interface{}{},
							"timestamp":         timestamp.Format(time.RFC3339Nano),
						},
					},
				}},
			},
		}},
	)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/messages/sess-1/load?limit=20", nil)
	ctx.Params = gin.Params{{Key: "session_id", Value: "sess-1"}}

	handler.LoadMessages(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, messageService.updatedMessages, 1)
	require.Len(t, messageService.updatedMessages[0].AgentSteps, 1)
	assert.Equal(t, "先查询北下街人员数量", messageService.updatedMessages[0].AgentSteps[0].Thought)
	assert.Equal(t, int64(3210), messageService.updatedMessages[0].AgentDurationMs)

	var response struct {
		Success bool             `json:"success"`
		Data    []*types.Message `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Len(t, response.Data, 1)
	require.Len(t, response.Data[0].AgentSteps, 1)
	assert.Equal(t, "先查询北下街人员数量", response.Data[0].AgentSteps[0].Thought)
	assert.Equal(t, int64(3210), response.Data[0].AgentDurationMs)
}

var _ interfaces.MessageService = (*messageHandlerMessageServiceStub)(nil)
var _ interfaces.SessionService = (*messageHandlerSessionServiceStub)(nil)
var _ interfaces.StreamManager = (*messageHandlerStreamManagerStub)(nil)
