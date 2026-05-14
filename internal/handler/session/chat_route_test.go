package session

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type handlerChatRouteServiceStub struct {
	called   bool
	decision *types.ChatRouteDecision
	err      error
}

func (s *handlerChatRouteServiceStub) Decide(_ context.Context, _ types.ChatRouteInput) (*types.ChatRouteDecision, error) {
	s.called = true
	return s.decision, s.err
}

func TestApplyDocumentRouteDecision_ShortDocumentClearsFullDocumentFlags(t *testing.T) {
	handler := &Handler{}
	reqCtx := &qaRequestContext{
		query:                 "请整理平台建设思路",
		documentIntent:        types.ChatDocumentIntentNormal,
		documentOperation:     types.ChatDocumentOperationCreate,
		documentOutputMode:    types.ChatDocumentOutputModeFull,
		documentTargetHeading: "第二章",
		documentMergeMode:     types.ChatDocumentMergeModeAppendToSection,
		documentQuotedContext: "quoted",
	}

	applied := handler.applyDocumentRouteDecision(context.Background(), reqCtx, &CreateKnowledgeQARequest{}, &types.ChatRouteDecision{Kind: types.ChatRouteShortDocument}, false)

	assert.True(t, applied)
	assert.Empty(t, reqCtx.documentIntent)
	assert.Empty(t, reqCtx.documentOperation)
	assert.Empty(t, reqCtx.documentOutputMode)
	assert.Empty(t, reqCtx.documentTargetHeading)
	assert.Empty(t, reqCtx.documentMergeMode)
	assert.Empty(t, reqCtx.documentQuotedContext)
	assert.Empty(t, reqCtx.baseArtifactID)
	assert.Nil(t, reqCtx.baseArtifact)
}

func TestApplyDocumentRouteDecision_DocumentEditHydratesArtifactContext(t *testing.T) {
	artifactService := &handlerChatDocumentArtifactServiceStub{
		artifact: &types.ChatDocumentArtifact{
			ID:              "artifact-1",
			SessionID:       "session-1",
			ArtifactKind:    types.ChatDocumentArtifactKindMarkdown,
			Status:          types.ChatDocumentArtifactStatusAvailable,
			ContentSnapshot: "# 技术方案\n\n## 智慧运行\n\n原始内容",
		},
		quotedContext: "quoted-context",
	}
	handler := &Handler{chatDocumentArtifactService: artifactService}
	reqCtx := &qaRequestContext{
		session: &types.Session{ID: "session-1"},
		query:   "继续补充智慧运行章节",
	}

	applied := handler.applyDocumentRouteDecision(context.Background(), reqCtx, &CreateKnowledgeQARequest{}, &types.ChatRouteDecision{Kind: types.ChatRouteDocumentEdit}, false)

	require.True(t, applied)
	assert.Equal(t, types.ChatDocumentIntentRevise, reqCtx.documentIntent)
	assert.Equal(t, types.ChatDocumentOperationRevise, reqCtx.documentOperation)
	assert.Equal(t, types.ChatDocumentOutputModeDelta, reqCtx.documentOutputMode)
	assert.Equal(t, "artifact-1", reqCtx.baseArtifactID)
	require.NotNil(t, reqCtx.baseArtifact)
	assert.Equal(t, "quoted-context", reqCtx.documentQuotedContext)
	assert.Equal(t, "智慧运行", reqCtx.documentTargetHeading)
	assert.Equal(t, types.ChatDocumentMergeModeAppendToSection, reqCtx.documentMergeMode)
	assert.Equal(t, types.ChatDocumentIntentRevise, artifactService.buildQuotedIntent)
	assert.Equal(t, types.ChatDocumentOutputModeDelta, artifactService.buildQuotedOutput)
	assert.Equal(t, "智慧运行", artifactService.buildQuotedTarget)
	assert.Equal(t, types.ChatDocumentMergeModeAppendToSection, artifactService.buildQuotedMergeMode)
}

func TestApplyDocumentRouteDecision_FullDocumentOverridesLegacyIntentFallback(t *testing.T) {
	handler := &Handler{}
	reqCtx := &qaRequestContext{
		query:              "请整理平台建设思路并形成完整输出",
		documentIntent:     types.ChatDocumentIntentRevise,
		documentOperation:  types.ChatDocumentOperationRevise,
		documentOutputMode: types.ChatDocumentOutputModeDelta,
	}

	applied := handler.applyDocumentRouteDecision(context.Background(), reqCtx, &CreateKnowledgeQARequest{}, &types.ChatRouteDecision{Kind: types.ChatRouteFullDocument}, false)

	require.True(t, applied)
	assert.Equal(t, types.ChatDocumentIntentNormal, reqCtx.documentIntent)
	assert.Equal(t, types.ChatDocumentOperationCreate, reqCtx.documentOperation)
	assert.Equal(t, types.ChatDocumentOutputModeFull, reqCtx.documentOutputMode)
	assert.Empty(t, reqCtx.baseArtifactID)
	assert.Nil(t, reqCtx.baseArtifact)
	assert.Empty(t, reqCtx.documentQuotedContext)
}

func TestApplyDocumentRouteDecisionWithReason_ReportsFullDocumentBlocker(t *testing.T) {
	handler := &Handler{}
	reqCtx := &qaRequestContext{
		query:            "请输出北海电厂的技术方案",
		autoContinue:     true,
		knowledgeBaseIDs: []string{"kb-1"},
	}

	applied, reason := handler.applyDocumentRouteDecisionWithReason(context.Background(), reqCtx, &CreateKnowledgeQARequest{}, &types.ChatRouteDecision{Kind: types.ChatRouteKnowledgeGroundedFullDoc}, true)

	assert.False(t, applied)
	assert.Equal(t, "auto_continue", reason)
}

func TestDetectChatRouteDecision_SkipsDatabaseAnalysisAgent(t *testing.T) {
	routeService := &handlerChatRouteServiceStub{}
	handler := &Handler{chatRouteService: routeService}
	reqCtx := &qaRequestContext{
		query: "请输出北下街组织结构",
		customAgent: &types.CustomAgent{
			ID: "agent-db",
			Config: types.CustomAgentConfig{
				AgentMode:       types.AgentModeSmartReasoning,
				AgentType:       types.AgentTypeDatabaseAnalysis,
				KBSelectionMode: "all",
			},
		},
	}

	handler.detectChatRouteDecision(context.Background(), "AgentQA", reqCtx, &CreateKnowledgeQARequest{})

	assert.False(t, routeService.called)
	require.NotNil(t, reqCtx.routeDecision)
	assert.Equal(t, types.ChatRouteAgentQA, reqCtx.routeDecision.Kind)
	assert.Equal(t, "database_agent_type_bypass", reqCtx.routeDecision.Reason)
	assert.True(t, reqCtx.routeDecision.UseKnowledge)
	assert.False(t, reqCtx.routeDecision.UseLongDocument)
	assert.False(t, reqCtx.routeDecisionApplied)
	assert.Empty(t, reqCtx.routeModelID)
}

func TestDetectChatRouteDecision_SkipsLegacyDatabaseToolOnlyAgent(t *testing.T) {
	routeService := &handlerChatRouteServiceStub{}
	handler := &Handler{chatRouteService: routeService}
	reqCtx := &qaRequestContext{
		query: "查询本月订单总金额",
		customAgent: &types.CustomAgent{
			ID: "agent-db-legacy",
			Config: types.CustomAgentConfig{
				AgentMode: types.AgentModeSmartReasoning,
				AllowedTools: []string{
					"thinking",
					"todo_write",
					"external_database_schema",
					"external_database_query",
					"final_answer",
				},
				KBSelectionMode: "selected",
				KnowledgeBases:  []string{"kb-db"},
			},
		},
	}

	handler.detectChatRouteDecision(context.Background(), "AgentQA", reqCtx, &CreateKnowledgeQARequest{})

	assert.False(t, routeService.called)
	require.NotNil(t, reqCtx.routeDecision)
	assert.Equal(t, types.ChatRouteAgentQA, reqCtx.routeDecision.Kind)
	assert.Equal(t, "database_tool_only_agent_bypass", reqCtx.routeDecision.Reason)
	assert.True(t, reqCtx.routeDecision.UseKnowledge)
	assert.False(t, reqCtx.routeDecision.UseLongDocument)
	assert.False(t, reqCtx.routeDecisionApplied)
	assert.Empty(t, reqCtx.routeModelID)
}

func TestDetectChatRouteDecision_DoesNotSkipMixedToolAgent(t *testing.T) {
	routeService := &handlerChatRouteServiceStub{decision: &types.ChatRouteDecision{Kind: types.ChatRouteAgentQA, Reason: "shadow"}}
	handler := &Handler{chatRouteService: routeService}
	reqCtx := &qaRequestContext{
		query: "查询并整理数据库设计方案",
		customAgent: &types.CustomAgent{
			ID: "agent-mixed",
			Config: types.CustomAgentConfig{
				AgentMode: types.AgentModeSmartReasoning,
				AllowedTools: []string{
					"thinking",
					"external_database_query",
					"knowledge_search",
					"final_answer",
				},
			},
		},
	}

	handler.detectChatRouteDecision(context.Background(), "AgentQA", reqCtx, &CreateKnowledgeQARequest{})

	assert.True(t, routeService.called)
	require.NotNil(t, reqCtx.routeDecision)
	assert.Equal(t, "shadow", reqCtx.routeDecision.Reason)
}

func TestDetectChatRouteDecision_SkipsExplicitTranslationTaskKind(t *testing.T) {
	routeService := &handlerChatRouteServiceStub{}
	handler := &Handler{chatRouteService: routeService}
	reqCtx := &qaRequestContext{
		query:              "请把这篇文档完整翻译成中文 Markdown",
		documentOutputMode: types.ChatDocumentOutputModeFull,
		documentTaskKind:   types.ChatDocumentTaskKindTranslation,
		knowledgeIDs:       []string{"knowledge-1"},
	}

	handler.detectChatRouteDecision(context.Background(), "AgentQA", reqCtx, &CreateKnowledgeQARequest{DocumentTaskKind: types.ChatDocumentTaskKindTranslation})

	assert.False(t, routeService.called)
	require.NotNil(t, reqCtx.routeDecision)
	assert.Equal(t, types.ChatRouteAgentQA, reqCtx.routeDecision.Kind)
	assert.Equal(t, "explicit_translation_task_kind_bypass", reqCtx.routeDecision.Reason)
	assert.True(t, reqCtx.routeDecision.UseLongDocument)
	assert.True(t, reqCtx.routeDecision.NeedArtifact)
	assert.False(t, reqCtx.routeDecisionApplied)
	assert.Empty(t, reqCtx.routeModelID)
}

func TestInferNaturalLanguageFullTranslationRequest_PromotesSingleKnowledgeQuery(t *testing.T) {
	reqCtx := &qaRequestContext{
		query:             "请把这篇文档完整翻译成中文 Markdown",
		knowledgeIDs:      []string{"knowledge-1"},
		documentIntent:    types.ChatDocumentIntentNormal,
		documentOperation: types.ChatDocumentOperationCreate,
	}
	request := &CreateKnowledgeQARequest{}

	inferNaturalLanguageFullTranslationRequest(reqCtx, request)

	assert.Equal(t, types.ChatDocumentTaskKindTranslation, reqCtx.documentTaskKind)
	assert.Equal(t, types.ChatDocumentOutputModeFull, reqCtx.documentOutputMode)
	assert.Equal(t, types.ChatDocumentTaskKindTranslation, request.DocumentTaskKind)
	assert.Equal(t, types.ChatDocumentOutputModeFull, request.DocumentOutputMode)
}

func TestInferNaturalLanguageFullTranslationRequest_DoesNotPromoteRegularQuery(t *testing.T) {
	reqCtx := &qaRequestContext{
		query:        "总结一下这篇文档讲了什么",
		knowledgeIDs: []string{"knowledge-1"},
	}
	request := &CreateKnowledgeQARequest{}

	inferNaturalLanguageFullTranslationRequest(reqCtx, request)

	assert.Empty(t, reqCtx.documentTaskKind)
	assert.Empty(t, reqCtx.documentOutputMode)
	assert.Empty(t, request.DocumentTaskKind)
	assert.Empty(t, request.DocumentOutputMode)
}

func TestInferNaturalLanguageFullTranslationRequest_DoesNotPromoteSnippetTranslation(t *testing.T) {
	reqCtx := &qaRequestContext{
		query:        "把这段话翻译成英文",
		knowledgeIDs: []string{"knowledge-1"},
	}
	request := &CreateKnowledgeQARequest{}

	inferNaturalLanguageFullTranslationRequest(reqCtx, request)

	assert.Empty(t, reqCtx.documentTaskKind)
	assert.Empty(t, reqCtx.documentOutputMode)
	assert.Empty(t, request.DocumentTaskKind)
	assert.Empty(t, request.DocumentOutputMode)
}
