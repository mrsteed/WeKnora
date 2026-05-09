package session

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

type documentRequestPreparation struct {
	intent        string
	operation     string
	baseArtifact  *types.ChatDocumentArtifact
	quotedContext string
}

func (h *Handler) prepareDocumentRequest(ctx context.Context, session *types.Session, query string, intentHint string, baseArtifactID string, outputMode string) documentRequestPreparation {
	result := documentRequestPreparation{
		intent:    types.ChatDocumentIntentNormal,
		operation: types.ChatDocumentOperationCreate,
	}
	if h.chatDocumentArtifactService == nil || session == nil {
		return result
	}

	intentResult, err := h.chatDocumentArtifactService.DetectIntent(ctx, session.ID, query, intentHint)
	if err != nil || intentResult == nil {
		if err != nil {
			logger.Warnf(ctx, "Failed to detect chat document intent, session_id: %s, error: %v", session.ID, err)
		}
		return result
	}
	result.intent = intentResult.Intent
	result.operation = intentResult.Operation

	if result.intent != types.ChatDocumentIntentContinue && result.intent != types.ChatDocumentIntentRevise {
		return result
	}

	var artifact *types.ChatDocumentArtifact
	if strings.TrimSpace(baseArtifactID) != "" {
		artifact, err = h.chatDocumentArtifactService.GetArtifact(ctx, baseArtifactID)
	} else {
		artifact, err = h.chatDocumentArtifactService.GetLatestArtifact(ctx, session.ID)
	}
	if err != nil {
		logger.Warnf(ctx, "Failed to load chat document artifact, session_id: %s, base_artifact_id: %s, error: %v", session.ID, baseArtifactID, err)
		return documentRequestPreparation{intent: types.ChatDocumentIntentNormal, operation: types.ChatDocumentOperationCreate}
	}
	if artifact == nil || artifact.SessionID != session.ID || !artifact.CanContinue() {
		return documentRequestPreparation{intent: types.ChatDocumentIntentNormal, operation: types.ChatDocumentOperationCreate}
	}

	quotedContext, err := h.chatDocumentArtifactService.BuildQuotedContext(ctx, artifact, query, result.intent, outputMode)
	if err != nil {
		logger.Warnf(ctx, "Failed to build chat document quoted context, session_id: %s, artifact_id: %s, error: %v", session.ID, artifact.ID, err)
		return documentRequestPreparation{intent: types.ChatDocumentIntentNormal, operation: types.ChatDocumentOperationCreate}
	}
	if strings.TrimSpace(quotedContext) == "" {
		return documentRequestPreparation{intent: types.ChatDocumentIntentNormal, operation: types.ChatDocumentOperationCreate}
	}

	result.baseArtifact = artifact
	result.quotedContext = quotedContext
	return result
}

func appendQuotedContext(existing string, extra string) string {
	existing = strings.TrimSpace(existing)
	extra = strings.TrimSpace(extra)
	switch {
	case existing == "":
		return extra
	case extra == "":
		return existing
	default:
		return existing + "\n\n" + extra
	}
}

func normalizeDocumentOutputModeForRequest(outputMode string, intent string) string {
	switch strings.TrimSpace(outputMode) {
	case types.ChatDocumentOutputModeFull:
		return types.ChatDocumentOutputModeFull
	case types.ChatDocumentOutputModeDelta:
		return types.ChatDocumentOutputModeDelta
	}

	switch intent {
	case types.ChatDocumentIntentContinue, types.ChatDocumentIntentRevise:
		return types.ChatDocumentOutputModeDelta
	case types.ChatDocumentIntentRegenerate:
		return types.ChatDocumentOutputModeFull
	default:
		return types.ChatDocumentOutputModeFull
	}
}

func (h *Handler) ListChatDocumentArtifacts(c *gin.Context) {
	ctx := c.Request.Context()
	sessionID := strings.TrimSpace(c.Query("session_id"))
	if sessionID == "" {
		c.Error(apperrors.NewBadRequestError("session_id is required"))
		return
	}
	if _, err := h.sessionService.GetSession(ctx, sessionID); err != nil {
		c.Error(apperrors.NewNotFoundError("Session not found"))
		return
	}
	limit := 20
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			c.Error(apperrors.NewBadRequestError("limit must be a positive integer"))
			return
		}
		limit = parsed
	}
	artifacts, err := h.chatDocumentArtifactService.ListBySession(ctx, sessionID, limit)
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": artifacts})
}

func (h *Handler) GetLatestChatDocumentArtifact(c *gin.Context) {
	ctx := c.Request.Context()
	sessionID := strings.TrimSpace(c.Query("session_id"))
	if sessionID == "" {
		c.Error(apperrors.NewBadRequestError("session_id is required"))
		return
	}
	if _, err := h.sessionService.GetSession(ctx, sessionID); err != nil {
		c.Error(apperrors.NewNotFoundError("Session not found"))
		return
	}
	artifact, err := h.chatDocumentArtifactService.GetLatestArtifact(ctx, sessionID)
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": artifact})
}

func (h *Handler) GetChatDocumentArtifact(c *gin.Context) {
	ctx := c.Request.Context()
	artifact, err := h.chatDocumentArtifactService.GetArtifact(ctx, c.Param("artifact_id"))
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	if artifact == nil {
		c.Error(apperrors.NewNotFoundError("Artifact not found"))
		return
	}
	if _, err := h.sessionService.GetSession(ctx, artifact.SessionID); err != nil {
		c.Error(apperrors.NewNotFoundError("Session not found"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": artifact})
}

func (h *Handler) ListChatDocumentArtifactRevisions(c *gin.Context) {
	ctx := c.Request.Context()
	artifact, err := h.chatDocumentArtifactService.GetArtifact(ctx, c.Param("artifact_id"))
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	if artifact == nil {
		c.Error(apperrors.NewNotFoundError("Artifact not found"))
		return
	}
	if _, err := h.sessionService.GetSession(ctx, artifact.SessionID); err != nil {
		c.Error(apperrors.NewNotFoundError("Session not found"))
		return
	}
	artifacts, err := h.chatDocumentArtifactService.ListRevisions(ctx, c.Param("artifact_id"))
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": artifacts})
}
