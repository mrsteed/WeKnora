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
	artifact             *types.ChatDocumentArtifact
	artifacts            []*types.ChatDocumentArtifact
	err                  error
	detectIntentResult   *types.DocumentIntentResult
	quotedContext        string
	buildQuotedQuery     string
	buildQuotedIntent    string
	buildQuotedOutput    string
	buildQuotedTarget    string
	buildQuotedMergeMode string
}

func (s *handlerChatDocumentArtifactServiceStub) DetectIntent(context.Context, string, string, string) (*types.DocumentIntentResult, error) {
	if s.detectIntentResult != nil {
		return s.detectIntentResult, nil
	}
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

func (s *handlerChatDocumentArtifactServiceStub) BuildQuotedContext(_ context.Context, _ *types.ChatDocumentArtifact, query string, intent string, outputMode string, targetHeading string, mergeMode string) (string, error) {
	s.buildQuotedQuery = query
	s.buildQuotedIntent = intent
	s.buildQuotedOutput = outputMode
	s.buildQuotedTarget = targetHeading
	s.buildQuotedMergeMode = mergeMode
	return s.quotedContext, s.err
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

func TestPrepareDocumentRequest_UsesStructuredIntentFieldsFromService(t *testing.T) {
	artifactService := &handlerChatDocumentArtifactServiceStub{
		artifact: &types.ChatDocumentArtifact{
			ID:              "artifact-1",
			SessionID:       "session-1",
			ArtifactKind:    types.ChatDocumentArtifactKindMarkdown,
			Status:          types.ChatDocumentArtifactStatusAvailable,
			ContentSnapshot: "# 技术方案\n\n## 智慧运行\n\n原始内容",
		},
		detectIntentResult: &types.DocumentIntentResult{Intent: types.ChatDocumentIntentRevise, Operation: types.ChatDocumentOperationRevise, TargetHeading: "智慧运行", MergeMode: types.ChatDocumentMergeModeAppendToSection},
		quotedContext:      "quoted-context",
	}
	handler := &Handler{chatDocumentArtifactService: artifactService}
	session := &types.Session{ID: "session-1"}

	result := handler.prepareDocumentRequest(context.Background(), session, "继续补充这一节", "", "", types.ChatDocumentOutputModeDelta, "", "")

	require.NotNil(t, result.baseArtifact)
	assert.Equal(t, types.ChatDocumentIntentRevise, result.intent)
	assert.Equal(t, types.ChatDocumentOperationRevise, result.operation)
	assert.Equal(t, "quoted-context", result.quotedContext)
	assert.Equal(t, "智慧运行", result.targetHeading)
	assert.Equal(t, types.ChatDocumentMergeModeAppendToSection, result.mergeMode)
	assert.Equal(t, "智慧运行", artifactService.buildQuotedTarget)
	assert.Equal(t, types.ChatDocumentMergeModeAppendToSection, artifactService.buildQuotedMergeMode)
}

func TestPrepareDocumentRequest_NormalIntentLeavesDocumentFieldsEmpty(t *testing.T) {
	handler := &Handler{chatDocumentArtifactService: &handlerChatDocumentArtifactServiceStub{}}
	session := &types.Session{ID: "session-1"}

	result := handler.prepareDocumentRequest(context.Background(), session, "火灾警情登记表包含哪些信息？", "", "", "", "", "")

	assert.Empty(t, result.intent)
	assert.Empty(t, result.operation)
	assert.Nil(t, result.baseArtifact)
	assert.Empty(t, result.quotedContext)
	assert.Empty(t, result.targetHeading)
	assert.Empty(t, result.mergeMode)
}

func TestNormalizeDocumentOutputModeForRequest(t *testing.T) {
	assert.Equal(t, types.ChatDocumentOutputModeFull, normalizeDocumentOutputModeForRequest(types.ChatDocumentOutputModeFull, ""))
	assert.Equal(t, types.ChatDocumentOutputModeDelta, normalizeDocumentOutputModeForRequest("", types.ChatDocumentIntentContinue))
	assert.Equal(t, types.ChatDocumentOutputModeFull, normalizeDocumentOutputModeForRequest("", types.ChatDocumentIntentRegenerate))
	assert.Empty(t, normalizeDocumentOutputModeForRequest("", ""))
	assert.Empty(t, normalizeDocumentOutputModeForRequest("", types.ChatDocumentIntentNormal))
}
