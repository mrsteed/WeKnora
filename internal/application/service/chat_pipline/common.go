package chatpipline

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

const (
	logValueMaxRune     = 300
	defaultStageName    = "PIPELINE"
	defaultActionName   = "info"
	pipelineLogPrefix   = "[PIPELINE]"
	pipelineTruncateEll = "..."
)

func pipelineLog(stage, action string, fields map[string]interface{}) string {
	if stage == "" {
		stage = defaultStageName
	}
	if action == "" {
		action = defaultActionName
	}

	builder := strings.Builder{}
	builder.Grow(128)
	builder.WriteString(pipelineLogPrefix)
	builder.WriteString(" stage=")
	builder.WriteString(stage)
	builder.WriteString(" action=")
	builder.WriteString(action)

	if len(fields) > 0 {
		keys := make([]string, 0, len(fields))
		for k := range fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			builder.WriteString(" ")
			builder.WriteString(key)
			builder.WriteString("=")
			builder.WriteString(formatLogValue(fields[key]))
		}
	}
	return builder.String()
}

func pipelineInfo(ctx context.Context, stage, action string, fields map[string]interface{}) {
	logger.GetLogger(ctx).Info(pipelineLog(stage, action, fields))
}

func pipelineWarn(ctx context.Context, stage, action string, fields map[string]interface{}) {
	logger.GetLogger(ctx).Warn(pipelineLog(stage, action, fields))
}

func pipelineError(ctx context.Context, stage, action string, fields map[string]interface{}) {
	logger.GetLogger(ctx).Error(pipelineLog(stage, action, fields))
}

func formatLogValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strconv.Quote(truncateForLog(v))
	case fmt.Stringer:
		return strconv.Quote(truncateForLog(v.String()))
	default:
		return fmt.Sprintf("%v", v)
	}
}

func truncateForLog(content string) string {
	content = strings.ReplaceAll(content, "\n", "\\n")
	runes := []rune(content)
	if len(runes) <= logValueMaxRune {
		return content
	}
	return string(runes[:logValueMaxRune]) + pipelineTruncateEll
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
