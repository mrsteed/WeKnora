package chatpipeline

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/models/asr"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/models/rerank"
	"github.com/Tencent/WeKnora/internal/models/vlm"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type closingStreamChatModel struct {
	responses []types.StreamResponse
	lastOpts  *chat.ChatOptions
}

func (m *closingStreamChatModel) Chat(context.Context, []chat.Message, *chat.ChatOptions) (*types.ChatResponse, error) {
	return nil, nil
}

func (m *closingStreamChatModel) ChatStream(_ context.Context, _ []chat.Message, opts *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	m.lastOpts = opts
	ch := make(chan types.StreamResponse, len(m.responses))
	for _, response := range m.responses {
		ch <- response
	}
	close(ch)
	return ch, nil
}

func (m *closingStreamChatModel) GetModelName() string { return "closing-stream" }
func (m *closingStreamChatModel) GetModelID() string   { return "closing-stream" }

type streamTestModelService struct {
	chatModel chat.Chat
}

func (s *streamTestModelService) CreateModel(context.Context, *types.Model) error { return nil }
func (s *streamTestModelService) GetModelByID(context.Context, string) (*types.Model, error) {
	return nil, nil
}
func (s *streamTestModelService) ListModels(context.Context) ([]*types.Model, error) { return nil, nil }
func (s *streamTestModelService) UpdateModel(context.Context, *types.Model) error    { return nil }
func (s *streamTestModelService) DeleteModel(context.Context, string) error          { return nil }
func (s *streamTestModelService) GetEmbeddingModel(context.Context, string) (embedding.Embedder, error) {
	return nil, nil
}
func (s *streamTestModelService) GetEmbeddingModelForTenant(context.Context, string, uint64) (embedding.Embedder, error) {
	return nil, nil
}
func (s *streamTestModelService) GetRerankModel(context.Context, string) (rerank.Reranker, error) {
	return nil, nil
}
func (s *streamTestModelService) GetChatModel(context.Context, string) (chat.Chat, error) {
	return s.chatModel, nil
}
func (s *streamTestModelService) GetVLMModel(context.Context, string) (vlm.VLM, error) {
	return nil, nil
}
func (s *streamTestModelService) GetASRModel(context.Context, string) (asr.ASR, error) {
	return nil, nil
}

func TestPluginChatCompletionStream_EmitsTerminalEventWhenChannelClosesWithoutDone(t *testing.T) {
	bus := event.NewEventBus()
	eventsCh := make(chan event.AgentFinalAnswerData, 4)
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		if ok {
			eventsCh <- data
		}
		return nil
	})

	plugin := &PluginChatCompletionStream{
		modelService: &streamTestModelService{chatModel: &closingStreamChatModel{responses: []types.StreamResponse{{
			ResponseType: types.ResponseTypeAnswer,
			Content:      "正常结束的回答",
			Done:         false,
		}}}},
	}

	chatManage := &types.ChatManage{
		PipelineRequest: types.PipelineRequest{
			SessionID:   "session-1",
			ChatModelID: "model-1",
			SummaryConfig: types.SummaryConfig{
				Prompt: "You are helpful.",
			},
		},
		PipelineState: types.PipelineState{
			UserContent: "请回答",
		},
		PipelineContext: types.PipelineContext{
			EventBus: bus.AsEventBusInterface(),
		},
	}

	nextCalled := false
	err := plugin.OnEvent(context.Background(), types.CHAT_COMPLETION_STREAM, chatManage, func() *PluginError {
		nextCalled = true
		return nil
	})
	require.Nil(t, err)
	assert.True(t, nextCalled)

	var first event.AgentFinalAnswerData
	var terminal event.AgentFinalAnswerData

	select {
	case first = <-eventsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("expected first answer event")
	}

	select {
	case terminal = <-eventsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("expected terminal answer event")
	}

	assert.Equal(t, "正常结束的回答", first.Content)
	assert.False(t, first.Done)
	assert.True(t, terminal.Done)
	assert.Equal(t, "completed", terminal.CompletionStatus)
	assert.Equal(t, "stop", terminal.FinishReason)
	assert.True(t, terminal.AllowComplete)
	assert.Equal(t, "正常结束的回答", chatManage.ChatResponse.Content)
}

func TestPluginChatCompletionStream_DisablesThinkingForQuickAnswer(t *testing.T) {
	bus := event.NewEventBus()
	model := &closingStreamChatModel{responses: []types.StreamResponse{{
		ResponseType: types.ResponseTypeAnswer,
		Content:      "最终答案",
		Done:         true,
	}}}
	plugin := &PluginChatCompletionStream{
		modelService: &streamTestModelService{chatModel: model},
	}
	thinking := true
	chatManage := &types.ChatManage{
		PipelineRequest: types.PipelineRequest{
			SessionID:   "session-quick-answer",
			ChatModelID: "model-thinking",
			SummaryConfig: types.SummaryConfig{
				Prompt:   "You are helpful.",
				Thinking: &thinking,
			},
		},
		PipelineState: types.PipelineState{
			UserContent: "请直接回答",
		},
		PipelineContext: types.PipelineContext{
			EventBus: bus.AsEventBusInterface(),
		},
	}

	err := plugin.OnEvent(context.Background(), types.CHAT_COMPLETION_STREAM, chatManage, func() *PluginError {
		return nil
	})
	require.Nil(t, err)

	require.Eventually(t, func() bool {
		return chatManage.ChatResponse != nil
	}, 2*time.Second, 10*time.Millisecond)
	require.NotNil(t, model.lastOpts)
	require.NotNil(t, model.lastOpts.Thinking)
	assert.False(t, *model.lastOpts.Thinking)
	assert.Equal(t, "最终答案", chatManage.ChatResponse.Content)
}
