package session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type continueStreamSessionServiceStub struct {
	session *types.Session
	err     error
}

func (s *continueStreamSessionServiceStub) CreateSession(context.Context, *types.Session) (*types.Session, error) {
	return nil, nil
}

func (s *continueStreamSessionServiceStub) GetSession(context.Context, string) (*types.Session, error) {
	return s.session, s.err
}

func (s *continueStreamSessionServiceStub) GetSessionsByTenant(context.Context) ([]*types.Session, error) {
	return nil, nil
}

func (s *continueStreamSessionServiceStub) GetPagedSessionsByTenant(context.Context, *types.Pagination) (*types.PageResult, error) {
	return nil, nil
}

func (s *continueStreamSessionServiceStub) UpdateSession(context.Context, *types.Session) error {
	return nil
}

func (s *continueStreamSessionServiceStub) DeleteSession(context.Context, string) error {
	return nil
}

func (s *continueStreamSessionServiceStub) BatchDeleteSessions(context.Context, []string) error {
	return nil
}

func (s *continueStreamSessionServiceStub) DeleteAllSessions(context.Context) error {
	return nil
}

func (s *continueStreamSessionServiceStub) ListSessions(context.Context, *types.SessionListQuery) (*types.PageResult, error) {
	return nil, nil
}

func (s *continueStreamSessionServiceStub) SetSessionPinned(context.Context, string, bool) (int64, error) {
	return 0, nil
}

func (s *continueStreamSessionServiceStub) GenerateTitle(context.Context, *types.Session, []types.Message, string) (string, error) {
	return "", nil
}

func (s *continueStreamSessionServiceStub) GenerateTitleAsync(context.Context, *types.Session, string, string, *event.EventBus) {
}

func (s *continueStreamSessionServiceStub) KnowledgeQA(context.Context, *types.QARequest, *event.EventBus) error {
	return nil
}

func (s *continueStreamSessionServiceStub) KnowledgeQAByEvent(context.Context, *types.ChatManage, []types.EventType) error {
	return nil
}

func (s *continueStreamSessionServiceStub) SearchKnowledge(context.Context, []string, []string, string) ([]*types.SearchResult, error) {
	return nil, nil
}

func (s *continueStreamSessionServiceStub) AgentQA(context.Context, *types.QARequest, *event.EventBus) error {
	return nil
}

func (s *continueStreamSessionServiceStub) ClearContext(context.Context, string) error {
	return nil
}

func newContinueStreamTestContext(t *testing.T, sessionID, messageID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/continue-stream/"+sessionID+"?message_id="+messageID, nil)
	c.Request = req
	c.Params = gin.Params{{Key: "session_id", Value: sessionID}}
	return c, recorder
}

func decodeSSEPayload(t *testing.T, body string) types.StreamResponse {
	t.Helper()
	const prefix = "data:"
	idx := strings.Index(body, prefix)
	require.NotEqual(t, -1, idx, "expected SSE payload in response body")
	dataLine := body[idx+len(prefix):]
	if newline := strings.IndexByte(dataLine, '\n'); newline >= 0 {
		dataLine = dataLine[:newline]
	}
	var response types.StreamResponse
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(dataLine)), &response))
	return response
}

func TestContinueStream_RecoversMissingPendingStreamAsFailed(t *testing.T) {
	messageStub := &messageServiceStub{getMessageResult: &types.Message{
		ID:               "msg-1",
		SessionID:        "sess-1",
		RequestID:        "req-1",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusPending,
	}}
	handler := &Handler{
		sessionService: &continueStreamSessionServiceStub{session: &types.Session{ID: "sess-1"}},
		messageService: messageStub,
		streamManager:  &streamManagerStub{},
	}

	c, recorder := newContinueStreamTestContext(t, "sess-1", "msg-1")
	handler.ContinueStream(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, messageStub.updatedMessages, 1)
	assert.Equal(t, types.MessageCompletionStatusFailed, messageStub.updatedMessages[0].CompletionStatus)
	assert.Equal(t, "stream_unavailable", messageStub.updatedMessages[0].FinishReason)
	assert.Equal(t, "stream_unavailable", messageStub.updatedMessages[0].FailureReason)

	response := decodeSSEPayload(t, recorder.Body.String())
	assert.Equal(t, types.ResponseTypeAnswer, response.ResponseType)
	assert.True(t, response.Done)
	assert.Equal(t, types.MessageCompletionStatusFailed, response.Data["completion_status"])
	assert.Equal(t, "stream_unavailable", response.Data["finish_reason"])
	assert.Equal(t, "stream_unavailable", response.Data["failure_reason"])
	assert.Equal(t, false, response.Data["is_partial"])
}

func TestContinueStream_RecoversMissingPendingStreamWithContentAsPartial(t *testing.T) {
	messageStub := &messageServiceStub{getMessageResult: &types.Message{
		ID:               "msg-2",
		SessionID:        "sess-1",
		RequestID:        "req-2",
		Role:             "assistant",
		Content:          "partial answer",
		CompletionStatus: types.MessageCompletionStatusPending,
	}}
	handler := &Handler{
		sessionService: &continueStreamSessionServiceStub{session: &types.Session{ID: "sess-1"}},
		messageService: messageStub,
		streamManager:  &streamManagerStub{},
	}

	c, recorder := newContinueStreamTestContext(t, "sess-1", "msg-2")
	handler.ContinueStream(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, messageStub.updatedMessages, 1)
	assert.Equal(t, types.MessageCompletionStatusPartial, messageStub.updatedMessages[0].CompletionStatus)

	response := decodeSSEPayload(t, recorder.Body.String())
	assert.Equal(t, types.MessageCompletionStatusPartial, response.Data["completion_status"])
	assert.Equal(t, true, response.Data["is_partial"])
}

var _ interfaces.SessionService = (*continueStreamSessionServiceStub)(nil)
