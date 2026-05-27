package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/asr"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/models/rerank"
	"github.com/Tencent/WeKnora/internal/models/vlm"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type chatRouteModelServiceStub struct {
	chatModel chat.Chat
	err       error
}

func (s *chatRouteModelServiceStub) CreateModel(context.Context, *types.Model) error { return nil }
func (s *chatRouteModelServiceStub) GetModelByID(context.Context, string) (*types.Model, error) {
	return nil, nil
}
func (s *chatRouteModelServiceStub) ListModels(context.Context) ([]*types.Model, error) {
	return nil, nil
}
func (s *chatRouteModelServiceStub) UpdateModel(context.Context, *types.Model) error { return nil }
func (s *chatRouteModelServiceStub) DeleteModel(context.Context, string) error       { return nil }
func (s *chatRouteModelServiceStub) UpdateModelCredentials(context.Context, string, *string, *string) (*types.Model, error) {
	return nil, nil
}
func (s *chatRouteModelServiceStub) ClearModelCredential(context.Context, string, string) error {
	return nil
}
func (s *chatRouteModelServiceStub) GetEmbeddingModel(context.Context, string) (embedding.Embedder, error) {
	return nil, nil
}
func (s *chatRouteModelServiceStub) GetEmbeddingModelForTenant(context.Context, string, uint64) (embedding.Embedder, error) {
	return nil, nil
}
func (s *chatRouteModelServiceStub) GetRerankModel(context.Context, string) (rerank.Reranker, error) {
	return nil, nil
}
func (s *chatRouteModelServiceStub) GetChatModel(context.Context, string) (chat.Chat, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.chatModel, nil
}
func (s *chatRouteModelServiceStub) GetVLMModel(context.Context, string) (vlm.VLM, error) {
	return nil, nil
}
func (s *chatRouteModelServiceStub) GetASRModel(context.Context, string) (asr.ASR, error) {
	return nil, nil
}

type chatRouteChatModelStub struct {
	response string
	err      error
	options  *chat.ChatOptions
	ctx      context.Context
	messages []chat.Message
}

func (s *chatRouteChatModelStub) Chat(ctx context.Context, messages []chat.Message, opts *chat.ChatOptions) (*types.ChatResponse, error) {
	s.ctx = ctx
	s.options = opts
	s.messages = messages
	if s.err != nil {
		return nil, s.err
	}
	return &types.ChatResponse{Content: s.response}, nil
}
func (s *chatRouteChatModelStub) ChatStream(context.Context, []chat.Message, *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	return nil, nil
}
func (s *chatRouteChatModelStub) GetModelName() string { return "route-model" }
func (s *chatRouteChatModelStub) GetModelID() string   { return "route-model-id" }

func TestChatRouteServiceDecide_UsesModelDecision(t *testing.T) {
	chatStub := &chatRouteChatModelStub{response: `{"kind":"knowledge_grounded_full_document","confidence":0.91,"reason":"用户明确要求完整技术方案","use_knowledge":true}`}
	svc := NewChatRouteService(&chatRouteModelServiceStub{chatModel: chatStub}, &config.Config{Agent: &config.AgentConfig{LLMCallTimeout: 120}})

	decision, err := svc.Decide(context.Background(), types.ChatRouteInput{
		Query:                    "请结合知识库输出一份完整技术方案",
		ModelID:                  "model-1",
		EndpointMode:             "agent_qa",
		AgentModeEnabledByConfig: true,
		HasSelectedKnowledge:     true,
	})

	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, types.ChatRouteKnowledgeGroundedFullDoc, decision.Kind)
	assert.Equal(t, types.ChatDocumentOutputModeFull, decision.OutputMode)
	assert.True(t, decision.UseLongDocument)
	assert.True(t, decision.NeedArtifact)
	require.NotNil(t, chatStub.options)
	assert.NotEmpty(t, chatStub.options.Format)
	require.NotNil(t, chatStub.options.Thinking)
	assert.False(t, *chatStub.options.Thinking)
	require.Len(t, chatStub.messages, 2)
	assert.Contains(t, chatStub.messages[0].Content, "完整译文")
	assert.Contains(t, chatStub.messages[0].Content, "全文翻译")
	assert.Contains(t, chatStub.messages[1].Content, "技术方案")
	assert.Contains(t, chatStub.messages[1].Content, "实施方案")
	assert.Contains(t, chatStub.messages[1].Content, "投标方案")
	assert.Contains(t, chatStub.messages[1].Content, "完整翻译成中文 Markdown")
	assert.Contains(t, chatStub.messages[1].Content, "把这段话翻译成英文")
	assert.Contains(t, chatStub.messages[1].Content, "这个方案有哪些风险")
	deadline, ok := chatStub.ctx.Deadline()
	require.True(t, ok)
	remaining := time.Until(deadline)
	assert.LessOrEqual(t, remaining, 120*time.Second)
	assert.Greater(t, remaining, 110*time.Second)
}

func TestChatRouteServiceDecide_UsesDefaultLLMTimeoutWhenConfigMissing(t *testing.T) {
	chatStub := &chatRouteChatModelStub{response: `{"kind":"agent_qa","confidence":0.88,"reason":"普通问答","use_agent":true}`}
	svc := NewChatRouteService(&chatRouteModelServiceStub{chatModel: chatStub}, nil)

	decision, err := svc.Decide(context.Background(), types.ChatRouteInput{
		Query:                    "北海电厂在哪里？",
		ModelID:                  "model-1",
		EndpointMode:             "agent_qa",
		AgentModeEnabledByConfig: true,
	})

	require.NoError(t, err)
	require.NotNil(t, decision)
	deadline, ok := chatStub.ctx.Deadline()
	require.True(t, ok)
	remaining := time.Until(deadline)
	assert.LessOrEqual(t, remaining, defaultChatRouteLLMCallTimeout)
	assert.Greater(t, remaining, defaultChatRouteLLMCallTimeout-10*time.Second)
}

func TestChatRouteServiceDecide_FallsBackToCurrentModeOnModelFailure(t *testing.T) {
	svc := NewChatRouteService(&chatRouteModelServiceStub{err: errors.New("boom")}, &config.Config{Agent: &config.AgentConfig{LLMCallTimeout: 120}})

	decision, err := svc.Decide(context.Background(), types.ChatRouteInput{
		Query:                    "河南省郑州市中原区桐柏路206号5楼32号的火灾警情登记表包含哪些信息？",
		ModelID:                  "model-1",
		EndpointMode:             "agent_qa",
		AgentModeEnabledByConfig: true,
		HasSelectedKnowledge:     true,
	})

	require.Error(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, types.ChatRouteAgentQA, decision.Kind)
	assert.True(t, decision.UseAgent)
	assert.True(t, decision.UseKnowledge)
	assert.Equal(t, 0.0, decision.Confidence)
	assert.Equal(t, "route_model_load_failed", decision.Reason)
}

func TestChatRouteServiceDecide_RegexFallbackSelectsKnowledgeGroundedFullDocument(t *testing.T) {
	svc := NewChatRouteService(&chatRouteModelServiceStub{err: errors.New("boom")}, &config.Config{Agent: &config.AgentConfig{LLMCallTimeout: 120}})

	decision, err := svc.Decide(context.Background(), types.ChatRouteInput{
		Query:                    "请输出北海电厂的技术方案",
		ModelID:                  "model-1",
		EndpointMode:             "agent_qa",
		AgentConfigured:          true,
		AgentModeEnabledByConfig: true,
		HasSelectedKnowledge:     true,
		HasEffectiveAgentKB:      true,
	})

	require.Error(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, types.ChatRouteKnowledgeGroundedFullDoc, decision.Kind)
	assert.Equal(t, types.ChatDocumentIntentNormal, decision.Intent)
	assert.Equal(t, types.ChatDocumentOperationCreate, decision.Operation)
	assert.Equal(t, types.ChatDocumentOutputModeFull, decision.OutputMode)
	assert.True(t, decision.UseAgent)
	assert.True(t, decision.UseKnowledge)
	assert.True(t, decision.UseLongDocument)
	assert.True(t, decision.NeedArtifact)
	assert.Equal(t, 0.65, decision.Confidence)
	assert.Equal(t, "route_model_load_failed; regex_full_document_fallback", decision.Reason)
}

func TestChatRouteServiceDecide_RegexFallbackSelectsFullDocumentWithoutKnowledge(t *testing.T) {
	svc := NewChatRouteService(&chatRouteModelServiceStub{err: errors.New("boom")}, &config.Config{Agent: &config.AgentConfig{LLMCallTimeout: 120}})

	decision, err := svc.Decide(context.Background(), types.ChatRouteInput{
		Query:                    "请生成一份企业数据治理技术方案",
		ModelID:                  "model-1",
		EndpointMode:             "agent_qa",
		AgentModeEnabledByConfig: true,
	})

	require.Error(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, types.ChatRouteFullDocument, decision.Kind)
	assert.Equal(t, types.ChatDocumentOutputModeFull, decision.OutputMode)
	assert.True(t, decision.UseLongDocument)
	assert.False(t, decision.UseKnowledge)
}

func TestChatRouteServiceDecide_RegexFallbackSelectsKnowledgeGroundedFullDocumentForFullTranslation(t *testing.T) {
	svc := NewChatRouteService(&chatRouteModelServiceStub{err: errors.New("boom")}, &config.Config{Agent: &config.AgentConfig{LLMCallTimeout: 120}})

	decision, err := svc.Decide(context.Background(), types.ChatRouteInput{
		Query:                    "请把这篇文档完整翻译成中文 Markdown",
		ModelID:                  "model-1",
		EndpointMode:             "agent_qa",
		AgentConfigured:          true,
		AgentModeEnabledByConfig: true,
		HasSelectedKnowledge:     true,
		HasEffectiveAgentKB:      true,
	})

	require.Error(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, types.ChatRouteKnowledgeGroundedFullDoc, decision.Kind)
	assert.Equal(t, types.ChatDocumentOutputModeFull, decision.OutputMode)
	assert.True(t, decision.UseLongDocument)
	assert.True(t, decision.NeedArtifact)
	assert.Equal(t, "route_model_load_failed; regex_full_document_fallback", decision.Reason)
}

func TestChatRouteServiceDecide_RegexFallbackRespectsAttachmentGuard(t *testing.T) {
	svc := NewChatRouteService(&chatRouteModelServiceStub{err: errors.New("boom")}, &config.Config{Agent: &config.AgentConfig{LLMCallTimeout: 120}})

	decision, err := svc.Decide(context.Background(), types.ChatRouteInput{
		Query:                    "请输出北海电厂的技术方案",
		ModelID:                  "model-1",
		EndpointMode:             "agent_qa",
		AgentModeEnabledByConfig: true,
		HasSelectedKnowledge:     true,
		HasAttachments:           true,
	})

	require.Error(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, types.ChatRouteAgentQA, decision.Kind)
	assert.False(t, decision.UseLongDocument)
	assert.False(t, decision.NeedArtifact)
}

func TestChatRouteServiceDecide_RegexFallbackRespectsAutoContinueGuard(t *testing.T) {
	svc := NewChatRouteService(&chatRouteModelServiceStub{err: errors.New("boom")}, &config.Config{Agent: &config.AgentConfig{LLMCallTimeout: 120}})

	decision, err := svc.Decide(context.Background(), types.ChatRouteInput{
		Query:                    "请输出北海电厂的技术方案",
		ModelID:                  "model-1",
		EndpointMode:             "agent_qa",
		AgentModeEnabledByConfig: true,
		HasSelectedKnowledge:     true,
		AutoContinue:             true,
	})

	require.Error(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, types.ChatRouteAgentQA, decision.Kind)
	assert.False(t, decision.UseLongDocument)
	assert.False(t, decision.NeedArtifact)
}

func TestChatRouteServiceDecide_RegexFallbackDoesNotPromoteSchemeQuestion(t *testing.T) {
	svc := NewChatRouteService(&chatRouteModelServiceStub{err: errors.New("boom")}, &config.Config{Agent: &config.AgentConfig{LLMCallTimeout: 120}})

	queries := []string{
		"请输出这个方案有哪些风险？",
		"北海电厂技术方案包含哪些章节？",
		"把这段话翻译成英文",
	}
	for _, query := range queries {
		decision, err := svc.Decide(context.Background(), types.ChatRouteInput{
			Query:                    query,
			ModelID:                  "model-1",
			EndpointMode:             "agent_qa",
			AgentModeEnabledByConfig: true,
			HasSelectedKnowledge:     true,
		})

		require.Error(t, err)
		require.NotNil(t, decision)
		assert.Equal(t, types.ChatRouteAgentQA, decision.Kind)
		assert.False(t, decision.UseLongDocument)
		assert.False(t, decision.NeedArtifact)
	}
}

func TestChatRouteServiceDecide_LogsNormalizationFallback(t *testing.T) {
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	defer logger.SetOutput(os.Stdout)

	chatStub := &chatRouteChatModelStub{response: `{"kind":"unexpected_route","confidence":0.44,"reason":"bad kind"}`}
	svc := NewChatRouteService(&chatRouteModelServiceStub{chatModel: chatStub}, &config.Config{Agent: &config.AgentConfig{LLMCallTimeout: 120}})

	decision, err := svc.Decide(context.Background(), types.ChatRouteInput{
		Query:                    "请输出北海电厂的技术方案",
		ModelID:                  "model-1",
		EndpointMode:             "agent_qa",
		AgentModeEnabledByConfig: true,
		HasSelectedKnowledge:     true,
	})

	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, types.ChatRouteKnowledgeGroundedFullDoc, decision.Kind)
	out := buf.String()
	require.NotEmpty(t, out)
	assert.True(t, strings.Contains(out, "[ChatRouter][Fallback]") && strings.Contains(out, "model_route_normalization_fallback"), out)
	assert.Contains(t, out, "regex_full_document_fallback=true")
	assert.Contains(t, out, "timeout_source=agent_llm_timeout_config")
	assert.Contains(t, out, "elapsed_ms=")
}
