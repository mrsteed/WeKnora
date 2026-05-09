package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type handlerChatDocumentArtifactServiceStub struct {
	artifact  *types.ChatDocumentArtifact
	artifacts []*types.ChatDocumentArtifact
	err       error
}

func (s *handlerChatDocumentArtifactServiceStub) DetectIntent(context.Context, string, string, string) (*types.DocumentIntentResult, error) {
	return &types.DocumentIntentResult{Intent: types.ChatDocumentIntentNormal, Operation: types.ChatDocumentOperationCreate}, nil
}

func (s *handlerChatDocumentArtifactServiceStub) GetLatestArtifact(context.Context, string) (*types.ChatDocumentArtifact, error) {
	return s.artifact, s.err
}

func (s *handlerChatDocumentArtifactServiceStub) GetArtifact(context.Context, string) (*types.ChatDocumentArtifact, error) {
	return s.artifact, s.err
}

func (s *handlerChatDocumentArtifactServiceStub) GetArtifactBySourceMessageID(context.Context, string) (*types.ChatDocumentArtifact, error) {
	return s.artifact, s.err
}

func (s *handlerChatDocumentArtifactServiceStub) BuildQuotedContext(context.Context, *types.ChatDocumentArtifact, string, string, string) (string, error) {
	return "", nil
}

func (s *handlerChatDocumentArtifactServiceStub) RegisterFromAssistantMessage(context.Context, *types.Message, types.RegisterChatDocumentArtifactOptions) (*types.ChatDocumentArtifact, error) {
	return nil, nil
}

func (s *handlerChatDocumentArtifactServiceStub) ListBySession(context.Context, string, int) ([]*types.ChatDocumentArtifact, error) {
	return s.artifacts, s.err
}

func (s *handlerChatDocumentArtifactServiceStub) ListRevisions(context.Context, string) ([]*types.ChatDocumentArtifact, error) {
	return s.artifacts, s.err
}

func TestGetChatDocumentArtifact_RequiresSessionAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/chat-document-artifacts/artifact-1", nil)
	c.Params = gin.Params{{Key: "artifact_id", Value: "artifact-1"}}

	handler := &Handler{
		sessionService: &continueStreamSessionServiceStub{session: nil, err: assert.AnError},
		chatDocumentArtifactService: &handlerChatDocumentArtifactServiceStub{
			artifact: &types.ChatDocumentArtifact{ID: "artifact-1", SessionID: "session-1"},
		},
	}

	handler.GetChatDocumentArtifact(c)

	require.Len(t, c.Errors, 1)
	assert.Contains(t, c.Errors[0].Error(), "Session not found")
	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestListChatDocumentArtifactRevisions_RequiresSessionAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/chat-document-artifacts/artifact-1/revisions", nil)
	c.Params = gin.Params{{Key: "artifact_id", Value: "artifact-1"}}

	handler := &Handler{
		sessionService: &continueStreamSessionServiceStub{session: nil, err: assert.AnError},
		chatDocumentArtifactService: &handlerChatDocumentArtifactServiceStub{
			artifact:  &types.ChatDocumentArtifact{ID: "artifact-1", SessionID: "session-1"},
			artifacts: []*types.ChatDocumentArtifact{{ID: "artifact-1", SessionID: "session-1"}},
		},
	}

	handler.ListChatDocumentArtifactRevisions(c)

	require.Len(t, c.Errors, 1)
	assert.Contains(t, c.Errors[0].Error(), "Session not found")
	assert.Equal(t, http.StatusOK, recorder.Code)
}
