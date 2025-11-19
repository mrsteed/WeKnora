package chatpipline

import (
	"context"

	"github.com/Tencent/WeKnora/internal/common"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

func pipelineInfo(ctx context.Context, stage, action string, fields map[string]interface{}) {
	common.PipelineInfo(ctx, stage, action, fields)
}

func pipelineWarn(ctx context.Context, stage, action string, fields map[string]interface{}) {
	common.PipelineWarn(ctx, stage, action, fields)
}

func pipelineError(ctx context.Context, stage, action string, fields map[string]interface{}) {
	common.PipelineError(ctx, stage, action, fields)
}

// prepareChatModel shared logic to prepare chat model and options
func prepareChatModel(ctx context.Context, modelService interfaces.ModelService,
	chatManage *types.ChatManage,
) (chat.Chat, *chat.ChatOptions, error) {
	logger.Infof(ctx, "Getting chat model, model ID: %s", chatManage.ChatModelID)

	chatModel, err := modelService.GetChatModel(ctx, chatManage.ChatModelID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get chat model: %v", err)
		return nil, nil, err
	}

	logger.Info(ctx, "Setting up chat options")
	opt := &chat.ChatOptions{
		Temperature:         chatManage.SummaryConfig.Temperature,
		TopP:                chatManage.SummaryConfig.TopP,
		Seed:                chatManage.SummaryConfig.Seed,
		MaxTokens:           chatManage.SummaryConfig.MaxTokens,
		MaxCompletionTokens: chatManage.SummaryConfig.MaxCompletionTokens,
		FrequencyPenalty:    chatManage.SummaryConfig.FrequencyPenalty,
		PresencePenalty:     chatManage.SummaryConfig.PresencePenalty,
	}

	return chatModel, opt, nil
}

// prepareMessagesWithHistory prepare complete messages including history
func prepareMessagesWithHistory(chatManage *types.ChatManage) []chat.Message {
	chatMessages := []chat.Message{
		{Role: "system", Content: chatManage.SummaryConfig.Prompt},
	}

	chatHistory := chatManage.History
	if len(chatHistory) > 2 {
		chatHistory = chatHistory[len(chatHistory)-2:]
	}

	// Add conversation history
	for _, history := range chatHistory {
		chatMessages = append(chatMessages, chat.Message{Role: "user", Content: history.Query})
		chatMessages = append(chatMessages, chat.Message{Role: "assistant", Content: history.Answer})
	}

	// Add current user message
	chatMessages = append(chatMessages, chat.Message{Role: "user", Content: chatManage.UserContent})

	return chatMessages
}
