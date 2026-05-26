package session

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	appservice "github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const shareSessionTokenHeader = "X-Share-Session-Token"

var publicShareAudioExtensions = map[string]struct{}{
	"mp3":  {},
	"wav":  {},
	"m4a":  {},
	"aac":  {},
	"ogg":  {},
	"flac": {},
	"webm": {},
}

// CreatePublicAgentPageShareSession creates an anonymous chat session for a public agent share page.
func (h *Handler) CreatePublicAgentPageShareSession(c *gin.Context) {
	ctx := c.Request.Context()
	shareCode := strings.TrimSpace(c.Param("share_code"))
	if shareCode == "" {
		c.Error(errors.NewBadRequestError("share_code cannot be empty"))
		return
	}

	result, err := h.pageShareSessionService.CreateAnonymousSession(ctx, shareCode, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		h.handlePublicShareSessionError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    result,
	})
}

// LoadPublicAgentPageShareMessages loads messages from one anonymous share-page session.
func (h *Handler) LoadPublicAgentPageShareMessages(c *gin.Context) {
	sessionCtx, ok := h.mustLoadPublicShareSession(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	limitInt := 20
	if limit := strings.TrimSpace(c.Query("limit")); limit != "" {
		if parsed, err := strconv.Atoi(limit); err == nil && parsed > 0 {
			limitInt = parsed
		}
	}

	beforeTimeStr := strings.TrimSpace(c.Query("before_time"))
	var (
		messages []*types.Message
		err      error
	)
	if beforeTimeStr == "" {
		messages, err = h.messageService.GetRecentMessagesBySession(ctx, sessionCtx.Session.ID, limitInt)
	} else {
		beforeTime, parseErr := time.Parse(time.RFC3339Nano, beforeTimeStr)
		if parseErr != nil {
			c.Error(errors.NewBadRequestError("Invalid time format, please use RFC3339Nano format"))
			return
		}
		messages, err = h.messageService.GetMessagesBySessionBeforeTime(ctx, sessionCtx.Session.ID, beforeTime, limitInt)
	}
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"session_id": sessionCtx.Session.ID})
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    messages,
	})
}

// PublicAgentPageShareChat runs one anonymous share-page QA request through the existing QA pipeline.
func (h *Handler) PublicAgentPageShareChat(c *gin.Context) {
	reqCtx, request, err := h.buildPublicShareQARequestContext(c)
	if err != nil {
		if isPublicShareSessionServiceError(err) {
			h.handlePublicShareSessionError(c, err)
			return
		}
		c.Error(err)
		return
	}
	reqCtx.titleSeedQuery = request.Query

	if reqCtx.customAgent != nil && reqCtx.customAgent.IsAgentMode() {
		h.executeQA(reqCtx, qaModeAgent, true)
		return
	}
	h.executeQA(reqCtx, qaModeNormal, true)
}

// ContinuePublicAgentPageShareStream continues SSE streaming for one anonymous share-page session.
func (h *Handler) ContinuePublicAgentPageShareStream(c *gin.Context) {
	request, err := parsePublicShareContinueRequest(c)
	if err != nil {
		c.Error(err)
		return
	}
	sessionCtx, ok := h.mustLoadPublicShareSessionByID(c, request.SessionID)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	messageID := request.MessageID
	if messageID == "" {
		c.Error(errors.NewBadRequestError("Missing message ID"))
		return
	}

	message, err := h.messageService.GetMessage(ctx, sessionCtx.Session.ID, messageID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"session_id": sessionCtx.Session.ID, "message_id": messageID})
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	if message == nil {
		c.Error(errors.NewNotFoundError("Incomplete message not found"))
		return
	}

	if message.IsTerminal() {
		var events []interfaces.StreamEvent
		if h.streamManager != nil {
			events, _, err = h.streamManager.GetEvents(ctx, sessionCtx.Session.ID, messageID, 0)
			if err != nil {
				logger.Warnf(ctx, "Failed to load terminal stream events, session ID: %s, message ID: %s, error: %v", sessionCtx.Session.ID, messageID, err)
				events = nil
			}
		}
		h.emitTerminalStreamState(c, message, events)
		return
	}

	events, currentOffset, err := h.streamManager.GetEvents(ctx, sessionCtx.Session.ID, messageID, 0)
	if err != nil {
		c.Error(errors.NewInternalServerError(fmt.Sprintf("Failed to get stream data: %s", err.Error())))
		return
	}
	if len(events) == 0 {
		h.recoverMissingStreamState(ctx, c, message)
		return
	}

	setSSEHeaders(c)
	streamCompleted := false
	for _, evt := range events {
		if evt.Type == types.ResponseTypeComplete || evt.Type == types.ResponseType(event.EventStop) {
			streamCompleted = true
		}
		response := buildStreamResponse(evt, message.RequestID)
		c.SSEvent("message", response)
		c.Writer.Flush()
	}
	if streamCompleted {
		sendCompletionEvent(c, message.RequestID)
		return
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			newEvents, newOffset, err := h.streamManager.GetEvents(ctx, sessionCtx.Session.ID, messageID, currentOffset)
			if err != nil {
				logger.Errorf(ctx, "Failed to get new events: %v", err)
				return
			}
			streamCompletedNow := false
			for _, evt := range newEvents {
				if evt.Type == types.ResponseTypeComplete || evt.Type == types.ResponseType(event.EventStop) {
					streamCompletedNow = true
				}
				response := buildStreamResponse(evt, message.RequestID)
				c.SSEvent("message", response)
				c.Writer.Flush()
			}
			currentOffset = newOffset
			if streamCompletedNow {
				sendCompletionEvent(c, message.RequestID)
				return
			}
		}
	}
}

// StopPublicAgentPageShareSession stops one in-flight anonymous share-page generation.
func (h *Handler) StopPublicAgentPageShareSession(c *gin.Context) {
	sessionCtx, ok := h.mustLoadPublicShareSession(c)
	if !ok {
		return
	}
	ctx := logger.CloneContext(c.Request.Context())

	var req StopSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("message_id is required"))
		return
	}
	assistantMessageID := strings.TrimSpace(req.MessageID)
	if assistantMessageID == "" {
		c.Error(errors.NewBadRequestError("message_id is required"))
		return
	}

	message, err := h.messageService.GetMessage(ctx, sessionCtx.Session.ID, assistantMessageID)
	if err != nil {
		c.Error(errors.NewNotFoundError("Message not found"))
		return
	}
	if message.SessionID != sessionCtx.Session.ID {
		c.Error(errors.NewForbiddenError("Message does not belong to this session"))
		return
	}
	if message.IsTerminal() {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "Message already completed"})
		return
	}

	stopEvent := interfaces.StreamEvent{
		ID:        fmt.Sprintf("stop-%d", time.Now().UnixNano()),
		Type:      types.ResponseType(event.EventStop),
		Content:   "",
		Done:      true,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"session_id": sessionCtx.Session.ID,
			"message_id": assistantMessageID,
			"reason":     "user_requested",
		},
	}
	if err := h.streamManager.AppendEvent(ctx, sessionCtx.Session.ID, assistantMessageID, stopEvent); err != nil {
		c.Error(errors.NewInternalServerError("Failed to write stop event"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Generation stopped"})
}

func (h *Handler) mustLoadPublicShareSession(c *gin.Context) (*types.AgentPageShareSessionContext, bool) {
	sessionID := strings.TrimSpace(c.Param("session_id"))
	return h.mustLoadPublicShareSessionByID(c, sessionID)
}

func (h *Handler) mustLoadPublicShareSessionByID(c *gin.Context, sessionID string) (*types.AgentPageShareSessionContext, bool) {
	ctx := c.Request.Context()
	shareCode := strings.TrimSpace(c.Param("share_code"))
	sessionID = strings.TrimSpace(sessionID)
	visitorToken := strings.TrimSpace(c.GetHeader(shareSessionTokenHeader))
	if shareCode == "" {
		c.Error(errors.NewBadRequestError("share_code cannot be empty"))
		return nil, false
	}
	if sessionID == "" {
		c.Error(errors.NewBadRequestError("session_id cannot be empty"))
		return nil, false
	}
	if visitorToken == "" {
		c.Error(errors.NewForbiddenError("Missing share session token"))
		return nil, false
	}

	sessionCtx, err := h.pageShareSessionService.ValidateAnonymousSession(ctx, shareCode, sessionID, visitorToken)
	if err != nil {
		h.handlePublicShareSessionError(c, err)
		return nil, false
	}
	h.attachPublicShareSessionContext(c, sessionCtx.Session)
	return sessionCtx, true
}

// attachPublicShareSessionContext binds the validated anonymous session tenant onto the request context.
// This keeps downstream message/session ownership checks working without requiring platform auth middleware.
func (h *Handler) attachPublicShareSessionContext(c *gin.Context, session *types.Session) {
	if c == nil || session == nil || session.TenantID == 0 {
		return
	}

	ctx := context.WithValue(c.Request.Context(), types.SessionTenantIDContextKey, session.TenantID)
	if _, ok := types.TenantIDFromContext(ctx); !ok {
		ctx = context.WithValue(ctx, types.TenantIDContextKey, session.TenantID)
		if _, exists := c.Get(types.TenantIDContextKey.String()); !exists {
			c.Set(types.TenantIDContextKey.String(), session.TenantID)
		}
	}
	c.Set(types.SessionTenantIDContextKey.String(), session.TenantID)
	c.Request = c.Request.WithContext(ctx)
}

func parsePublicShareContinueRequest(c *gin.Context) (*PublicAgentPageShareContinueRequest, error) {
	request := &PublicAgentPageShareContinueRequest{
		SessionID: strings.TrimSpace(c.Param("session_id")),
		MessageID: strings.TrimSpace(c.Query("message_id")),
	}
	if request.SessionID != "" && request.MessageID != "" {
		return request, nil
	}

	if c.Request.Method == http.MethodGet {
		request.SessionID = strings.TrimSpace(c.Query("session_id"))
		request.MessageID = strings.TrimSpace(c.Query("message_id"))
	} else {
		var body PublicAgentPageShareContinueRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			return nil, errors.NewBadRequestError(err.Error())
		}
		request.SessionID = strings.TrimSpace(body.SessionID)
		request.MessageID = strings.TrimSpace(body.MessageID)
	}

	if request.SessionID == "" {
		return nil, errors.NewBadRequestError("session_id cannot be empty")
	}
	if request.MessageID == "" {
		return nil, errors.NewBadRequestError("message_id cannot be empty")
	}
	return request, nil
}

func (h *Handler) buildPublicShareQARequestContext(c *gin.Context) (*qaRequestContext, *PublicAgentPageShareChatRequest, error) {
	baseCtx := logger.CloneContext(c.Request.Context())
	var request PublicAgentPageShareChatRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return nil, nil, errors.NewBadRequestError(err.Error())
	}
	request.Query = strings.TrimSpace(request.Query)
	request.SessionID = strings.TrimSpace(request.SessionID)
	if request.Query == "" {
		return nil, nil, errors.NewBadRequestError("Query content cannot be empty")
	}
	if request.SessionID == "" {
		return nil, nil, errors.NewBadRequestError("session_id cannot be empty")
	}

	shareCode := strings.TrimSpace(c.Param("share_code"))
	visitorToken := strings.TrimSpace(c.GetHeader(shareSessionTokenHeader))
	if shareCode == "" {
		return nil, nil, errors.NewBadRequestError("share_code cannot be empty")
	}
	if visitorToken == "" {
		return nil, nil, errors.NewForbiddenError("Missing share session token")
	}

	sessionCtx, err := h.pageShareSessionService.ValidateAnonymousSession(baseCtx, shareCode, request.SessionID, visitorToken)
	if err != nil {
		return nil, nil, err
	}

	h.attachPublicShareSessionContext(c, sessionCtx.Session)
	ctx := c.Request.Context()

	for i := range request.Images {
		request.Images[i].URL = ""
		request.Images[i].Caption = ""
	}

	customAgent := sessionCtx.Agent
	if customAgent == nil {
		return nil, nil, errors.NewNotFoundError("Shared agent not found")
	}
	customAgent.EnsureDefaults()

	if len(request.Images) > 0 {
		if !customAgent.Config.ImageUploadEnabled {
			return nil, nil, errors.NewBadRequestError("Image upload is not enabled for this agent")
		}
		if err := h.saveImageAttachments(ctx, request.Images, sessionCtx.Session.TenantID, customAgent.Config.ImageStorageProvider); err != nil {
			return nil, nil, errors.NewBadRequestError(fmt.Sprintf("Image save failed: %v", err))
		}
	}

	if err := validatePublicShareAttachmentUploads(customAgent, request.AttachmentUploads); err != nil {
		return nil, nil, errors.NewBadRequestError(err.Error())
	}

	processedAttachments, err := h.processPublicShareAttachments(ctx, sessionCtx.Session.TenantID, customAgent, request.AttachmentUploads)
	if err != nil {
		return nil, nil, errors.NewBadRequestError(fmt.Sprintf("attachment processing failed: %v", err))
	}

	requestID := strings.TrimSpace(c.GetString(types.RequestIDContextKey.String()))
	if requestID == "" {
		requestID = uuid.New().String()
	}

	reqCtx := &qaRequestContext{
		ctx:               ctx,
		c:                 c,
		sessionID:         sessionCtx.Session.ID,
		requestID:         requestID,
		receivedAt:        time.Now(),
		query:             request.Query,
		titleSeedQuery:    request.Query,
		session:           sessionCtx.Session,
		customAgent:       customAgent,
		assistantMessage:  &types.Message{SessionID: sessionCtx.Session.ID, RequestID: requestID, Role: "assistant", Channel: "web"},
		webSearchEnabled:  customAgent.Config.WebSearchEnabled,
		enableMemory:      false,
		effectiveTenantID: sessionCtx.Share.SourceTenantID,
		images:            request.Images,
		channel:           "web",
		attachments:       processedAttachments,
	}
	return reqCtx, &request, nil
}

func (h *Handler) processPublicShareAttachments(ctx context.Context, tenantID uint64, customAgent *types.CustomAgent, uploads []AttachmentUpload) (types.MessageAttachments, error) {
	if len(uploads) == 0 {
		return nil, nil
	}
	maxSize := secutils.GetMaxFileSize()
	for i, upload := range uploads {
		if upload.FileSize > maxSize {
			return nil, fmt.Errorf("attachment %d exceeds size limit of %dMB", i+1, secutils.GetMaxFileSizeMB())
		}
	}
	asrModelID := ""
	if customAgent != nil && customAgent.Config.AudioUploadEnabled && customAgent.Config.ASRModelID != "" {
		asrModelID = customAgent.Config.ASRModelID
	}
	processedAttachments := make(types.MessageAttachments, len(uploads))
	var wg sync.WaitGroup
	errChan := make(chan error, len(uploads))
	for i, upload := range uploads {
		wg.Add(1)
		go func(idx int, att AttachmentUpload) {
			defer wg.Done()
			data, err := DecodeBase64Attachment(att.Data)
			if err != nil {
				errChan <- fmt.Errorf("attachment %d decode failed: %w", idx+1, err)
				return
			}
			processed, err := h.attachmentProcessor.ProcessAttachment(ctx, data, att.FileName, att.FileSize, tenantID, asrModelID)
			if err != nil {
				errChan <- fmt.Errorf("attachment %d processing failed: %w", idx+1, err)
				return
			}
			processedAttachments[idx] = *processed
		}(i, upload)
	}
	wg.Wait()
	close(errChan)
	if len(errChan) > 0 {
		return nil, <-errChan
	}
	return processedAttachments, nil
}

func validatePublicShareAttachmentUploads(agent *types.CustomAgent, uploads []AttachmentUpload) error {
	if agent == nil || len(uploads) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(agent.Config.SupportedFileTypes))
	for _, ext := range agent.Config.SupportedFileTypes {
		trimmed := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
		if trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}
	for _, upload := range uploads {
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(upload.FileName)), ".")
		if ext == "" {
			continue
		}
		if _, isAudio := publicShareAudioExtensions[ext]; isAudio && !agent.Config.AudioUploadEnabled {
			return fmt.Errorf("audio upload is not enabled for this agent")
		}
		if len(allowed) > 0 {
			if _, ok := allowed[ext]; !ok {
				return fmt.Errorf("attachment file type .%s is not supported by this agent", ext)
			}
		}
	}
	return nil
}

func (h *Handler) handlePublicShareSessionError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	ctx := c.Request.Context()
	logger.ErrorWithFields(ctx, err, map[string]interface{}{"share_code": secutils.SanitizeForLog(c.Param("share_code")), "session_id": secutils.SanitizeForLog(c.Param("session_id"))})
	switch err {
	case appservice.ErrAgentPageShareNotFound, appservice.ErrAgentPageShareUnavailable, appservice.ErrSharedAgentNotFound, appservice.ErrAgentPageShareSessionNotFound:
		c.Error(errors.NewNotFoundError("Agent page share not found"))
	case appservice.ErrAgentPageShareSessionForbidden:
		c.Error(errors.NewForbiddenError("Invalid share session token"))
	case appservice.ErrAgentPageShareSessionExpired:
		c.JSON(http.StatusGone, gin.H{"success": false, "error": "Share session expired"})
	case appservice.ErrAgentPageShareSessionLimitReached:
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "error": "Share session limit reached"})
	default:
		c.Error(errors.NewInternalServerError(err.Error()))
	}
}

func isPublicShareSessionServiceError(err error) bool {
	switch err {
	case appservice.ErrAgentPageShareNotFound,
		appservice.ErrAgentPageShareUnavailable,
		appservice.ErrSharedAgentNotFound,
		appservice.ErrAgentPageShareSessionNotFound,
		appservice.ErrAgentPageShareSessionForbidden,
		appservice.ErrAgentPageShareSessionExpired,
		appservice.ErrAgentPageShareSessionLimitReached:
		return true
	default:
		return false
	}
}
