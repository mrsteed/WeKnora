package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// MessageHandler handles HTTP requests related to messages within chat sessions
// It provides endpoints for loading and managing message history
type MessageHandler struct {
	MessageService interfaces.MessageService // Service that implements message business logic
	SessionService interfaces.SessionService // Service for verifying session ownership
	StreamManager  interfaces.StreamManager  // Stream manager used to reconcile stale pending assistant messages
}

// NewMessageHandler creates a new message handler instance with the required service
// Parameters:
//   - messageService: Service that implements message business logic
//   - sessionService: Service for verifying session ownership
//
// Returns a pointer to a new MessageHandler
func NewMessageHandler(messageService interfaces.MessageService, sessionService interfaces.SessionService, streamManager interfaces.StreamManager) *MessageHandler {
	return &MessageHandler{
		MessageService: messageService,
		SessionService: sessionService,
		StreamManager:  streamManager,
	}
}

func (h *MessageHandler) reconcilePendingAssistantMessages(ctx context.Context, messages []*types.Message) {
	if h.StreamManager == nil {
		return
	}

	for _, message := range messages {
		if !shouldReconcileAssistantMessage(message) {
			continue
		}

		repaired, err := h.reconcilePendingAssistantMessage(ctx, message)
		if err != nil {
			logger.Warnf(ctx, "Failed to reconcile pending assistant message, session ID: %s, message ID: %s, error: %v",
				message.SessionID, message.ID, err)
			continue
		}
		if repaired {
			logger.Infof(ctx, "Reconciled stale assistant message on load, session ID: %s, message ID: %s, completion_status: %s",
				message.SessionID, message.ID, message.CompletionStatusOrLegacy())
		}
	}
}

func (h *MessageHandler) reconcilePendingAssistantMessage(ctx context.Context, message *types.Message) (bool, error) {
	events, _, err := h.StreamManager.GetEvents(ctx, message.SessionID, message.ID, 0)
	if err != nil {
		return false, err
	}

	updatedMessage := *message
	answerContent := ""
	terminalDetected := false
	hasAnswerDone := false
	hasAgentStream := len(updatedMessage.AgentSteps) > 0 || updatedMessage.AgentDurationMs > 0

	for _, evt := range events {
		if isAgentMessageStreamEvent(evt.Type) {
			hasAgentStream = true
		}
		if evt.Type == types.ResponseTypeAnswer {
			answerContent += evt.Content
			if evt.Done {
				hasAnswerDone = true
			}
		}

		if evt.Type == types.ResponseType(event.EventStop) {
			terminalDetected = true
			updatedMessage.CompletionStatus = types.MessageCompletionStatusCancelled
			updatedMessage.IsCompleted = false
			cancelReason := strings.TrimSpace(streamEventString(evt.Data, "reason"))
			if cancelReason == "" {
				cancelReason = types.MessageCompletionStatusCancelled
			}
			updatedMessage.FinishReason = cancelReason
			updatedMessage.FailureReason = cancelReason
			continue
		}

		if evt.Type != types.ResponseTypeComplete {
			continue
		}

		terminalDetected = true
		completionStatus := strings.TrimSpace(streamEventString(evt.Data, "completion_status"))
		if completionStatus == "" {
			completionStatus = types.MessageCompletionStatusCompleted
		}
		updatedMessage.CompletionStatus = completionStatus
		updatedMessage.IsCompleted = completionStatus == types.MessageCompletionStatusCompleted
		updatedMessage.FinishReason = strings.TrimSpace(streamEventString(evt.Data, "finish_reason"))
		updatedMessage.FailureReason = strings.TrimSpace(streamEventString(evt.Data, "failure_reason"))
		if len(updatedMessage.AgentSteps) == 0 {
			updatedMessage.AgentSteps = streamEventAgentSteps(evt.Data, "agent_steps")
		}
		if updatedMessage.AgentDurationMs == 0 {
			updatedMessage.AgentDurationMs = streamEventInt64(evt.Data, "agent_duration_ms")
			if updatedMessage.AgentDurationMs == 0 {
				updatedMessage.AgentDurationMs = streamEventInt64(evt.Data, "total_duration_ms")
			}
		}

		finalAnswer := strings.TrimSpace(streamEventString(evt.Data, "final_answer"))
		if updatedMessage.Content == "" {
			switch {
			case finalAnswer != "":
				updatedMessage.Content = finalAnswer
			case strings.TrimSpace(answerContent) != "":
				updatedMessage.Content = answerContent
			}
		}
	}

	if !terminalDetected && hasAnswerDone {
		terminalDetected = true
		if hasAgentStream {
			updatedMessage.IsCompleted = false
			if strings.TrimSpace(answerContent) != "" {
				updatedMessage.CompletionStatus = types.MessageCompletionStatusPartial
			} else {
				updatedMessage.CompletionStatus = types.MessageCompletionStatusFailed
			}
			if strings.TrimSpace(updatedMessage.FinishReason) == "" {
				updatedMessage.FinishReason = "stream_closed"
			}
			if strings.TrimSpace(updatedMessage.FailureReason) == "" {
				updatedMessage.FailureReason = "stream_closed"
			}
		} else {
			updatedMessage.CompletionStatus = types.MessageCompletionStatusCompleted
			updatedMessage.IsCompleted = true
			if strings.TrimSpace(updatedMessage.FinishReason) == "" {
				updatedMessage.FinishReason = "stop"
			}
		}
		if updatedMessage.Content == "" && strings.TrimSpace(answerContent) != "" {
			updatedMessage.Content = answerContent
		}
	}

	if !terminalDetected && len(events) == 0 {
		terminalDetected = true
		updatedMessage.IsCompleted = false
		updatedMessage.FinishReason = "stream_unavailable"
		updatedMessage.FailureReason = "stream_unavailable"
		if strings.TrimSpace(updatedMessage.Content) != "" {
			updatedMessage.CompletionStatus = types.MessageCompletionStatusPartial
		} else {
			updatedMessage.CompletionStatus = types.MessageCompletionStatusFailed
		}
	}

	if !terminalDetected {
		return false, nil
	}

	if updatedMessage.Content == message.Content &&
		updatedMessage.IsCompleted == message.IsCompleted &&
		updatedMessage.CompletionStatus == message.CompletionStatus &&
		updatedMessage.FinishReason == message.FinishReason &&
		updatedMessage.FailureReason == message.FailureReason &&
		updatedMessage.AgentDurationMs == message.AgentDurationMs &&
		reflect.DeepEqual(updatedMessage.AgentSteps, message.AgentSteps) {
		return false, nil
	}

	updatedMessage.UpdatedAt = time.Now()
	if err := h.MessageService.UpdateMessage(context.WithoutCancel(ctx), &updatedMessage); err != nil {
		return false, err
	}
	*message = updatedMessage
	return true, nil
}

func shouldReconcileAssistantMessage(message *types.Message) bool {
	if message == nil || message.Role != "assistant" {
		return false
	}
	if !message.IsTerminal() {
		return true
	}
	if strings.TrimSpace(message.Content) == "" {
		return true
	}
	return len(message.AgentSteps) == 0 && strings.TrimSpace(message.FinishReason) == "tool_calls"
}

func isAgentMessageStreamEvent(evtType types.ResponseType) bool {
	switch evtType {
	case types.ResponseTypeAgentQuery,
		types.ResponseTypeThinking,
		types.ResponseTypeToolCall,
		types.ResponseTypeToolResult,
		types.ResponseTypeReflection,
		types.ResponseTypeComplete,
		types.ResponseType(event.EventStop):
		return true
	default:
		return false
	}
}

func streamEventString(data map[string]interface{}, key string) string {
	if data == nil {
		return ""
	}
	value, ok := data[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}

func streamEventInt64(data map[string]interface{}, key string) int64 {
	if data == nil {
		return 0
	}
	value, ok := data[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case float32:
		return int64(typed)
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

func streamEventAgentSteps(data map[string]interface{}, key string) types.AgentSteps {
	if data == nil {
		return nil
	}
	value, ok := data[key]
	if !ok {
		return nil
	}
	switch steps := value.(type) {
	case nil:
		return nil
	case types.AgentSteps:
		return append(types.AgentSteps(nil), steps...)
	case []types.AgentStep:
		return append(types.AgentSteps(nil), steps...)
	default:
		raw, err := json.Marshal(value)
		if err != nil || len(raw) == 0 || string(raw) == "null" {
			return nil
		}

		var decoded types.AgentSteps
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil
		}
		if len(decoded) == 0 {
			return nil
		}
		return decoded
	}
}

// LoadMessages godoc
// @Summary      加载消息历史
// @Description  加载会话的消息历史，支持分页和时间筛选
// @Tags         消息
// @Accept       json
// @Produce      json
// @Param        session_id   path      string  true   "会话ID"
// @Param        limit        query     int     false  "返回数量"  default(20)
// @Param        before_time  query     string  false  "在此时间之前的消息（RFC3339Nano格式）"
// @Success      200          {object}  map[string]interface{}  "消息列表"
// @Failure      400          {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /messages/{session_id}/load [get]
func (h *MessageHandler) LoadMessages(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start loading messages")

	// Get path parameters and query parameters
	sessionID := secutils.SanitizeForLog(c.Param("session_id"))
	limit := secutils.SanitizeForLog(c.DefaultQuery("limit", "20"))
	beforeTimeStr := secutils.SanitizeForLog(c.DefaultQuery("before_time", ""))

	logger.Infof(ctx, "Loading messages params, session ID: %s, limit: %s, before time: %s",
		sessionID, limit, beforeTimeStr)

	// Verify session ownership: GetSession will filter by tenantID + userID
	if _, err := h.SessionService.GetSession(ctx, sessionID); err != nil {
		logger.Warnf(ctx, "Session ownership check failed for session %s: %v", sessionID, err)
		c.Error(errors.NewNotFoundError("Session not found or access denied"))
		return
	}

	logger.Infof(ctx, "Loading messages params, session ID: %s, limit: %s, before time: %s",
		sessionID, limit, beforeTimeStr)

	// Parse limit parameter with fallback to default
	limitInt, err := strconv.Atoi(limit)
	if err != nil {
		logger.Warnf(ctx, "Invalid limit value, using default value 20, input: %s", limit)
		limitInt = 20
	}

	// If no beforeTime is provided, retrieve the most recent messages
	if beforeTimeStr == "" {
		logger.Infof(ctx, "Getting recent messages for session, session ID: %s, limit: %d", sessionID, limitInt)
		messages, err := h.MessageService.GetRecentMessagesBySession(ctx, sessionID, limitInt)
		if err != nil {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError(err.Error()))
			return
		}
		h.reconcilePendingAssistantMessages(ctx, messages)

		logger.Infof(
			ctx,
			"Successfully retrieved recent messages, session ID: %s, message count: %d",
			sessionID, len(messages),
		)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    messages,
		})
		return
	}

	// If beforeTime is provided, parse the timestamp
	beforeTime, err := time.Parse(time.RFC3339Nano, beforeTimeStr)
	if err != nil {
		logger.Errorf(
			ctx,
			"Invalid time format, please use RFC3339Nano format, err: %v, beforeTimeStr: %s",
			err, beforeTimeStr,
		)
		c.Error(errors.NewBadRequestError("Invalid time format, please use RFC3339Nano format"))
		return
	}

	// Retrieve messages before the specified timestamp
	logger.Infof(ctx, "Getting messages before specific time, session ID: %s, before time: %s, limit: %d",
		sessionID, beforeTime.Format(time.RFC3339Nano), limitInt)
	messages, err := h.MessageService.GetMessagesBySessionBeforeTime(ctx, sessionID, beforeTime, limitInt)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	h.reconcilePendingAssistantMessages(ctx, messages)

	logger.Infof(
		ctx,
		"Successfully retrieved messages before time, session ID: %s, message count: %d",
		sessionID, len(messages),
	)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    messages,
	})
}

// DeleteMessage godoc
// @Summary      删除消息
// @Description  从会话中删除指定消息
// @Tags         消息
// @Accept       json
// @Produce      json
// @Param        session_id  path      string  true  "会话ID"
// @Param        id          path      string  true  "消息ID"
// @Success      200         {object}  map[string]interface{}  "删除成功"
// @Failure      500         {object}  errors.AppError         "服务器错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /messages/{session_id}/{id} [delete]
func (h *MessageHandler) DeleteMessage(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start deleting message")

	// Get path parameters for session and message identification
	sessionID := secutils.SanitizeForLog(c.Param("session_id"))
	messageID := secutils.SanitizeForLog(c.Param("id"))

	// Verify session ownership: GetSession will filter by tenantID + userID
	if _, err := h.SessionService.GetSession(ctx, sessionID); err != nil {
		logger.Warnf(ctx, "Session ownership check failed for session %s: %v", sessionID, err)
		c.Error(errors.NewNotFoundError("Session not found or access denied"))
		return
	}

	logger.Infof(ctx, "Deleting message, session ID: %s, message ID: %s", sessionID, messageID)

	// Delete the message using the message service
	if err := h.MessageService.DeleteMessage(ctx, sessionID, messageID); err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Message deleted successfully, session ID: %s, message ID: %s", sessionID, messageID)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Message deleted successfully",
	})
}

// SearchMessages godoc
// @Summary      搜索历史对话
// @Description  通过关键词和/或向量相似度搜索历史对话记录，支持关键词、向量、混合三种模式
// @Tags         消息
// @Accept       json
// @Produce      json
// @Param        request  body      SearchMessagesRequest  true  "搜索请求"
// @Success      200      {object}  map[string]interface{}  "搜索结果"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /messages/search [post]
func (h *MessageHandler) SearchMessages(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start searching messages")

	var request SearchMessagesRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		logger.Error(ctx, "Failed to parse search request", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	if request.Query == "" {
		logger.Error(ctx, "Query content is empty")
		c.Error(errors.NewBadRequestError("Query content cannot be empty"))
		return
	}

	params := &types.MessageSearchParams{
		Query:      secutils.SanitizeForLog(request.Query),
		Mode:       types.MessageSearchMode(request.Mode),
		Limit:      request.Limit,
		SessionIDs: request.SessionIDs,
	}

	logger.Infof(ctx, "Searching messages with params: query=%s, mode=%s, limit=%d, session_ids=%v",
		params.Query, params.Mode, params.Limit, params.SessionIDs)

	result, err := h.MessageService.SearchMessages(ctx, params)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Message search completed, found %d results", result.Total)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// SearchMessagesRequest defines the request structure for searching messages
type SearchMessagesRequest struct {
	// Query text for search
	Query string `json:"query" binding:"required"`
	// Search mode: "keyword", "vector", "hybrid" (default: "hybrid")
	Mode string `json:"mode"`
	// Maximum number of results to return (default: 20)
	Limit int `json:"limit"`
	// Filter by specific session IDs (optional)
	SessionIDs []string `json:"session_ids"`
}

// GetChatHistoryKBStats godoc
// @Summary      获取聊天历史知识库统计
// @Description  获取聊天历史知识库的统计信息（已索引消息数、知识库大小等）
// @Tags         消息
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "统计信息"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /messages/chat-history-stats [get]
func (h *MessageHandler) GetChatHistoryKBStats(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Getting chat history KB stats")

	stats, err := h.MessageService.GetChatHistoryKBStats(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}
