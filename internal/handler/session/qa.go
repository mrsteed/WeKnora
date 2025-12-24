package session

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
)

// qaRequestContext holds all the common data needed for QA requests
type qaRequestContext struct {
	ctx              context.Context
	c                *gin.Context
	sessionID        string
	requestID        string
	query            string
	session          *types.Session
	customAgent      *types.CustomAgent
	assistantMessage *types.Message
	knowledgeBaseIDs []string
	knowledgeIDs     []string
	summaryModelID   string
	webSearchEnabled bool
	mentionedItems   types.MentionedItems
}

// parseQARequest parses and validates a QA request, returns the request context
func (h *Handler) parseQARequest(c *gin.Context, logPrefix string) (*qaRequestContext, *CreateKnowledgeQARequest, error) {
	ctx := logger.CloneContext(c.Request.Context())
	logger.Infof(ctx, "[%s] Start processing request", logPrefix)

	// Get session ID from URL parameter
	sessionID := secutils.SanitizeForLog(c.Param("session_id"))
	if sessionID == "" {
		logger.Error(ctx, "Session ID is empty")
		return nil, nil, errors.NewBadRequestError(errors.ErrInvalidSessionID.Error())
	}

	// Parse request body
	var request CreateKnowledgeQARequest
	if err := c.ShouldBindJSON(&request); err != nil {
		logger.Error(ctx, "Failed to parse request data", err)
		return nil, nil, errors.NewBadRequestError(err.Error())
	}

	// Validate query content
	if request.Query == "" {
		logger.Error(ctx, "Query content is empty")
		return nil, nil, errors.NewBadRequestError("Query content cannot be empty")
	}

	// Log request details
	if requestJSON, err := json.Marshal(request); err == nil {
		logger.Infof(ctx, "[%s] Request: session_id=%s, request=%s",
			logPrefix, sessionID, secutils.SanitizeForLog(string(requestJSON)))
	}

	// Get session
	session, err := h.sessionService.GetSession(ctx, sessionID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get session, session ID: %s, error: %v", sessionID, err)
		return nil, nil, errors.NewNotFoundError("Session not found")
	}

	// Get custom agent if agent_id is provided
	var customAgent *types.CustomAgent
	if request.AgentID != "" {
		logger.Infof(ctx, "Fetching custom agent, agent ID: %s", secutils.SanitizeForLog(request.AgentID))
		agent, err := h.customAgentService.GetAgentByID(ctx, request.AgentID)
		if err != nil {
			logger.Warnf(ctx, "Failed to get custom agent, agent ID: %s, error: %v, using default config",
				secutils.SanitizeForLog(request.AgentID), err)
		} else {
			customAgent = agent
			logger.Infof(ctx, "Using custom agent: ID=%s, Name=%s, Type=%s, AgentMode=%s",
				customAgent.ID, customAgent.Name, customAgent.Type, customAgent.Config.AgentMode)
		}
	}

	// Build request context
	reqCtx := &qaRequestContext{
		ctx:         ctx,
		c:           c,
		sessionID:   sessionID,
		requestID:   secutils.SanitizeForLog(c.GetString(types.RequestIDContextKey.String())),
		query:       secutils.SanitizeForLog(request.Query),
		session:     session,
		customAgent: customAgent,
		assistantMessage: &types.Message{
			SessionID:   sessionID,
			Role:        "assistant",
			RequestID:   c.GetString(types.RequestIDContextKey.String()),
			IsCompleted: false,
		},
		knowledgeBaseIDs: secutils.SanitizeForLogArray(request.KnowledgeBaseIDs),
		knowledgeIDs:     secutils.SanitizeForLogArray(request.KnowledgeIds),
		summaryModelID:   secutils.SanitizeForLog(request.SummaryModelID),
		webSearchEnabled: request.WebSearchEnabled,
		mentionedItems:   convertMentionedItems(request.MentionedItems),
	}

	return reqCtx, &request, nil
}

// detectAndApplyConfigChanges detects configuration changes and updates session if needed
// Returns true if any configuration changed
func (h *Handler) detectAndApplyConfigChanges(
	reqCtx *qaRequestContext,
	request *CreateKnowledgeQARequest,
) (bool, error) {
	ctx := reqCtx.ctx
	session := reqCtx.session
	sessionID := reqCtx.sessionID

	// Initialize AgentConfig if it doesn't exist
	if session.AgentConfig == nil {
		session.AgentConfig = &types.SessionAgentConfig{}
	}

	configChanged := false

	// Check knowledge bases change
	if hasArrayChanged(session.AgentConfig.KnowledgeBases, request.KnowledgeBaseIDs) {
		logger.Infof(ctx, "Knowledge bases changed from %v to %v",
			session.AgentConfig.KnowledgeBases, request.KnowledgeBaseIDs)
		configChanged = true
	}

	// Check knowledge IDs change
	if hasArrayChanged(session.AgentConfig.KnowledgeIDs, request.KnowledgeIds) {
		logger.Infof(ctx, "Knowledge IDs changed from %v to %v",
			session.AgentConfig.KnowledgeIDs, request.KnowledgeIds)
		configChanged = true
	}

	// Check agent mode change
	if request.AgentEnabled != session.AgentConfig.AgentModeEnabled {
		logger.Infof(ctx, "Agent mode changed from %v to %v",
			session.AgentConfig.AgentModeEnabled, request.AgentEnabled)
		configChanged = true
	}

	// Check web search change
	if request.WebSearchEnabled != session.AgentConfig.WebSearchEnabled {
		logger.Infof(ctx, "Web search mode changed from %v to %v",
			session.AgentConfig.WebSearchEnabled, request.WebSearchEnabled)
		configChanged = true
	}

	// Resolve summary model ID
	summaryModelID := reqCtx.summaryModelID
	if summaryModelID == "" {
		summaryModelID = session.SummaryModelID
	}
	if summaryModelID == "" {
		if tenantInfo, ok := ctx.Value(types.TenantInfoContextKey).(*types.Tenant); ok && tenantInfo.ConversationConfig != nil {
			summaryModelID = tenantInfo.ConversationConfig.SummaryModelID
		}
	}
	if summaryModelID != session.SummaryModelID {
		configChanged = true
	}

	// Apply changes if any
	if configChanged {
		logger.Warnf(ctx, "Configuration changed, clearing context for session: %s", sessionID)

		// Clear LLM context
		if err := h.sessionService.ClearContext(ctx, sessionID); err != nil {
			logger.Errorf(ctx, "Failed to clear context for session %s: %v", sessionID, err)
		}

		// Delete temp KB state
		if err := h.sessionService.DeleteWebSearchTempKBState(ctx, sessionID); err != nil {
			logger.Errorf(ctx, "Failed to delete temp knowledge base for session %s: %v", sessionID, err)
		}

		// Update session config
		session.AgentConfig.KnowledgeBases = secutils.SanitizeForLogArray(request.KnowledgeBaseIDs)
		session.AgentConfig.KnowledgeIDs = secutils.SanitizeForLogArray(request.KnowledgeIds)
		session.AgentConfig.AgentModeEnabled = request.AgentEnabled
		session.AgentConfig.WebSearchEnabled = request.WebSearchEnabled
		session.SummaryModelID = summaryModelID

		// Persist changes
		if err := h.sessionService.UpdateSession(ctx, session); err != nil {
			logger.Errorf(ctx, "Failed to update session %s: %v", sessionID, err)
			return false, errors.NewInternalServerError("Failed to update session configuration")
		}
		logger.Infof(ctx, "Session configuration updated successfully for session: %s", sessionID)
	}

	return configChanged, nil
}

// hasArrayChanged checks if two string arrays are different
func hasArrayChanged(current, new []string) bool {
	if len(current) != len(new) {
		return true
	}
	if len(current) == 0 && len(new) == 0 {
		return false
	}

	currentMap := make(map[string]bool)
	for _, v := range current {
		currentMap[v] = true
	}
	for _, v := range new {
		if !currentMap[v] {
			return true
		}
	}
	return false
}

// sseStreamContext holds the context for SSE streaming
type sseStreamContext struct {
	eventBus         *event.EventBus
	asyncCtx         context.Context
	cancel           context.CancelFunc
	assistantMessage *types.Message
}

// setupSSEStream sets up the SSE streaming context
func (h *Handler) setupSSEStream(reqCtx *qaRequestContext) *sseStreamContext {
	// Set SSE headers
	setSSEHeaders(reqCtx.c)

	// Write initial agent_query event
	h.writeAgentQueryEvent(reqCtx.ctx, reqCtx.sessionID, reqCtx.assistantMessage.ID)

	// Create EventBus and cancellable context
	eventBus := event.NewEventBus()
	asyncCtx, cancel := context.WithCancel(logger.CloneContext(reqCtx.ctx))

	streamCtx := &sseStreamContext{
		eventBus:         eventBus,
		asyncCtx:         asyncCtx,
		cancel:           cancel,
		assistantMessage: reqCtx.assistantMessage,
	}

	// Setup stop event handler
	h.setupStopEventHandler(eventBus, reqCtx.sessionID, reqCtx.assistantMessage, cancel)

	// Setup stream handler
	h.setupStreamHandler(asyncCtx, reqCtx.sessionID, reqCtx.assistantMessage.ID,
		reqCtx.requestID, reqCtx.assistantMessage, eventBus)

	// Generate title if needed
	if reqCtx.session.Title == "" {
		logger.Infof(reqCtx.ctx, "Session has no title, starting async title generation, session ID: %s", reqCtx.sessionID)
		h.sessionService.GenerateTitleAsync(asyncCtx, reqCtx.session, reqCtx.query, eventBus)
	}

	return streamCtx
}

// SearchKnowledge godoc
// @Summary      知识搜索
// @Description  在知识库中搜索（不使用LLM总结）
// @Tags         问答
// @Accept       json
// @Produce      json
// @Param        request  body      SearchKnowledgeRequest  true  "搜索请求"
// @Success      200      {object}  map[string]interface{}  "搜索结果"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /sessions/search [post]
func (h *Handler) SearchKnowledge(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())
	logger.Info(ctx, "Start processing knowledge search request")

	// Parse request body
	var request SearchKnowledgeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		logger.Error(ctx, "Failed to parse request data", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	// Validate request parameters
	if request.Query == "" {
		logger.Error(ctx, "Query content is empty")
		c.Error(errors.NewBadRequestError("Query content cannot be empty"))
		return
	}

	if request.KnowledgeBaseID == "" {
		logger.Error(ctx, "Knowledge base ID is empty")
		c.Error(errors.NewBadRequestError("Knowledge base ID cannot be empty"))
		return
	}

	logger.Infof(ctx, "Knowledge search request, knowledge base ID: %s, query: %s",
		secutils.SanitizeForLog(request.KnowledgeBaseID),
		secutils.SanitizeForLog(request.Query))

	// Perform search
	searchResults, err := h.sessionService.SearchKnowledge(ctx, request.KnowledgeBaseID, request.Query)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Knowledge search completed, found %d results", len(searchResults))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    searchResults,
	})
}

// KnowledgeQA godoc
// @Summary      知识问答
// @Description  基于知识库的问答（使用LLM总结），支持SSE流式响应
// @Tags         问答
// @Accept       json
// @Produce      text/event-stream
// @Param        session_id  path      string                   true  "会话ID"
// @Param        request     body      CreateKnowledgeQARequest true  "问答请求"
// @Success      200         {object}  map[string]interface{}   "问答结果（SSE流）"
// @Failure      400         {object}  errors.AppError          "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /sessions/{session_id}/knowledge-qa [post]
func (h *Handler) KnowledgeQA(c *gin.Context) {
	// Parse and validate request
	reqCtx, _, err := h.parseQARequest(c, "KnowledgeQA")
	if err != nil {
		c.Error(err)
		return
	}

	// Execute normal mode QA
	h.executeNormalModeQA(reqCtx, true)
}

// AgentQA godoc
// @Summary      Agent问答
// @Description  基于Agent的智能问答，支持多轮对话和SSE流式响应
// @Tags         问答
// @Accept       json
// @Produce      text/event-stream
// @Param        session_id  path      string                   true  "会话ID"
// @Param        request     body      CreateKnowledgeQARequest true  "问答请求"
// @Success      200         {object}  map[string]interface{}   "问答结果（SSE流）"
// @Failure      400         {object}  errors.AppError          "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /sessions/{session_id}/agent-qa [post]
func (h *Handler) AgentQA(c *gin.Context) {
	// Parse and validate request
	reqCtx, request, err := h.parseQARequest(c, "AgentQA")
	if err != nil {
		c.Error(err)
		return
	}

	// Detect and apply configuration changes
	if _, err := h.detectAndApplyConfigChanges(reqCtx, request); err != nil {
		c.Error(err)
		return
	}

	// Determine if agent mode should be enabled
	// Priority: customAgent.IsAgentMode() > request.AgentEnabled
	agentModeEnabled := request.AgentEnabled
	if reqCtx.customAgent != nil {
		agentModeEnabled = reqCtx.customAgent.IsAgentMode()
		logger.Infof(reqCtx.ctx, "Agent mode determined by custom agent: %v (config.agent_mode=%s)",
			agentModeEnabled, reqCtx.customAgent.Config.AgentMode)
	}

	// Route to appropriate handler based on agent mode
	if agentModeEnabled {
		h.executeAgentModeQA(reqCtx)
	} else {
		logger.Infof(reqCtx.ctx, "Agent mode disabled, delegating to normal mode for session: %s", reqCtx.sessionID)
		// Fallback to session's knowledge bases if not specified in request
		if len(reqCtx.knowledgeBaseIDs) == 0 {
			reqCtx.knowledgeBaseIDs = reqCtx.session.AgentConfig.KnowledgeBases
		}
		if len(reqCtx.knowledgeBaseIDs) == 0 && reqCtx.session.KnowledgeBaseID != "" {
			reqCtx.knowledgeBaseIDs = []string{reqCtx.session.KnowledgeBaseID}
		}
		h.executeNormalModeQA(reqCtx, false)
	}
}

// executeNormalModeQA executes the normal (KnowledgeQA) mode
func (h *Handler) executeNormalModeQA(reqCtx *qaRequestContext, generateTitle bool) {
	ctx := reqCtx.ctx
	sessionID := reqCtx.sessionID

	// Create user message
	if err := h.createUserMessage(ctx, sessionID, reqCtx.query, reqCtx.requestID, reqCtx.mentionedItems); err != nil {
		reqCtx.c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	// Create assistant message
	if _, err := h.createAssistantMessage(ctx, reqCtx.assistantMessage); err != nil {
		reqCtx.c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Using knowledge bases: %v", reqCtx.knowledgeBaseIDs)

	// Setup SSE stream
	streamCtx := h.setupSSEStream(reqCtx)

	// Setup completion handler for normal mode
	streamCtx.eventBus.On(event.EventAgentFinalAnswer, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		if !ok {
			return nil
		}
		streamCtx.assistantMessage.Content += data.Content
		if data.Done {
			logger.Infof(streamCtx.asyncCtx, "Knowledge QA service completed for session: %s", sessionID)
			h.completeAssistantMessage(streamCtx.asyncCtx, streamCtx.assistantMessage)
			streamCtx.eventBus.Emit(streamCtx.asyncCtx, event.Event{
				Type:      event.EventAgentComplete,
				SessionID: sessionID,
				Data:      event.AgentCompleteData{FinalAnswer: streamCtx.assistantMessage.Content},
			})
			streamCtx.cancel()
		}
		return nil
	})

	// Execute KnowledgeQA asynchronously
	go func() {
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 10240)
				runtime.Stack(buf, true)
				logger.ErrorWithFields(streamCtx.asyncCtx,
					errors.NewInternalServerError(fmt.Sprintf("Knowledge QA service panicked: %v\n%s", r, string(buf))), nil)
			}
		}()

		err := h.sessionService.KnowledgeQA(
			streamCtx.asyncCtx,
			reqCtx.session,
			reqCtx.query,
			reqCtx.knowledgeBaseIDs,
			reqCtx.knowledgeIDs,
			reqCtx.assistantMessage.ID,
			reqCtx.summaryModelID,
			reqCtx.webSearchEnabled,
			streamCtx.eventBus,
			reqCtx.customAgent,
		)
		if err != nil {
			logger.ErrorWithFields(streamCtx.asyncCtx, err, nil)
			streamCtx.eventBus.Emit(streamCtx.asyncCtx, event.Event{
				Type:      event.EventError,
				SessionID: sessionID,
				Data: event.ErrorData{
					Error:     err.Error(),
					Stage:     "knowledge_qa_execution",
					SessionID: sessionID,
				},
			})
		}
	}()

	// Handle SSE events (blocking)
	shouldWaitForTitle := generateTitle && reqCtx.session.Title == ""
	h.handleAgentEventsForSSE(ctx, reqCtx.c, sessionID, reqCtx.assistantMessage.ID,
		reqCtx.requestID, streamCtx.eventBus, shouldWaitForTitle)
}

// executeAgentModeQA executes the agent mode
func (h *Handler) executeAgentModeQA(reqCtx *qaRequestContext) {
	ctx := reqCtx.ctx
	sessionID := reqCtx.sessionID

	// Emit agent query event
	if err := event.Emit(ctx, event.Event{
		Type:      event.EventAgentQuery,
		SessionID: sessionID,
		RequestID: reqCtx.requestID,
		Data: event.AgentQueryData{
			SessionID: sessionID,
			Query:     reqCtx.query,
			RequestID: reqCtx.requestID,
		},
	}); err != nil {
		logger.Errorf(ctx, "Failed to emit agent query event: %v", err)
		return
	}

	// Create user message
	if err := h.createUserMessage(ctx, sessionID, reqCtx.query, reqCtx.requestID, reqCtx.mentionedItems); err != nil {
		reqCtx.c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	// Create assistant message
	assistantMessagePtr, err := h.createAssistantMessage(ctx, reqCtx.assistantMessage)
	if err != nil {
		reqCtx.c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	reqCtx.assistantMessage = assistantMessagePtr

	logger.Infof(ctx, "Calling agent QA service, session ID: %s", sessionID)

	// Setup SSE stream
	streamCtx := h.setupSSEStream(reqCtx)

	// Execute AgentQA asynchronously
	go func() {
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 1024)
				runtime.Stack(buf, true)
				logger.ErrorWithFields(streamCtx.asyncCtx,
					errors.NewInternalServerError(fmt.Sprintf("Agent QA service panicked: %v\n%s", r, string(buf))),
					map[string]interface{}{"session_id": sessionID})
			}
			h.completeAssistantMessage(streamCtx.asyncCtx, streamCtx.assistantMessage)
			logger.Infof(streamCtx.asyncCtx, "Agent QA service completed for session: %s", sessionID)
		}()

		err := h.sessionService.AgentQA(
			streamCtx.asyncCtx,
			reqCtx.session,
			reqCtx.query,
			reqCtx.assistantMessage.ID,
			streamCtx.eventBus,
			reqCtx.customAgent,
		)
		if err != nil {
			logger.ErrorWithFields(streamCtx.asyncCtx, err, nil)
			streamCtx.eventBus.Emit(streamCtx.asyncCtx, event.Event{
				Type:      event.EventError,
				SessionID: sessionID,
				Data: event.ErrorData{
					Error:     err.Error(),
					Stage:     "agent_execution",
					SessionID: sessionID,
				},
			})
		}
	}()

	// Handle SSE events (blocking)
	h.handleAgentEventsForSSE(ctx, reqCtx.c, sessionID, reqCtx.assistantMessage.ID,
		reqCtx.requestID, streamCtx.eventBus, reqCtx.session.Title == "")
}

// completeAssistantMessage marks an assistant message as complete and updates it
func (h *Handler) completeAssistantMessage(ctx context.Context, assistantMessage *types.Message) {
	assistantMessage.UpdatedAt = time.Now()
	assistantMessage.IsCompleted = true
	_ = h.messageService.UpdateMessage(ctx, assistantMessage)
}
