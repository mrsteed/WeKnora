package handler

import (
	"io"
	"net/http"
	"net/url"
	"time"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type LongDocumentTaskHandler struct {
	service interfaces.LongDocumentTaskService
}

func NewLongDocumentTaskHandler(service interfaces.LongDocumentTaskService) *LongDocumentTaskHandler {
	return &LongDocumentTaskHandler{service: service}
}

func (h *LongDocumentTaskHandler) CreateTask(c *gin.Context) {
	ctx := c.Request.Context()
	var req types.CreateLongDocumentTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	resp, err := h.service.CreateTask(ctx, &req)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"session_id": req.SessionID, "knowledge_id": req.KnowledgeID})
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

func (h *LongDocumentTaskHandler) GetTask(c *gin.Context) {
	ctx := c.Request.Context()
	task, err := h.service.GetTask(ctx, c.Param("id"))
	if err != nil {
		c.Error(apperrors.NewNotFoundError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": task})
}

func (h *LongDocumentTaskHandler) ListTasksBySession(c *gin.Context) {
	ctx := c.Request.Context()
	sessionID := c.Query("session_id")
	if sessionID == "" {
		c.Error(apperrors.NewBadRequestError("session_id is required"))
		return
	}
	var page types.Pagination
	if err := c.ShouldBindQuery(&page); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid query parameters").WithDetails(err.Error()))
		return
	}
	result, err := h.service.ListTasksBySession(ctx, sessionID, &page)
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *LongDocumentTaskHandler) ListBatches(c *gin.Context) {
	ctx := c.Request.Context()
	batches, err := h.service.ListBatches(ctx, c.Param("id"))
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": batches})
}

func (h *LongDocumentTaskHandler) GetArtifact(c *gin.Context) {
	ctx := c.Request.Context()
	artifact, err := h.service.GetArtifact(ctx, c.Param("id"))
	if err != nil {
		c.Error(apperrors.NewNotFoundError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": artifact})
}

func (h *LongDocumentTaskHandler) CancelTask(c *gin.Context) {
	ctx := c.Request.Context()
	task, err := h.service.CancelTask(ctx, c.Param("id"))
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": task})
}

func (h *LongDocumentTaskHandler) RetryTask(c *gin.Context) {
	ctx := c.Request.Context()
	task, err := h.service.RetryTask(ctx, c.Param("id"))
	if err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": task})
}

func (h *LongDocumentTaskHandler) DownloadArtifact(c *gin.Context) {
	ctx := c.Request.Context()
	reader, fileName, err := h.service.DownloadArtifact(ctx, c.Param("id"))
	if err != nil {
		c.Error(apperrors.NewNotFoundError(err.Error()))
		return
	}
	defer reader.Close()
	c.Header("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(fileName))
	c.Header("Content-Type", "text/markdown; charset=utf-8")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, reader); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"task_id": c.Param("id")})
	}
}

func (h *LongDocumentTaskHandler) StreamEvents(c *gin.Context) {
	ctx := c.Request.Context()
	taskID := c.Param("id")
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.Error(apperrors.NewInternalServerError("streaming is not supported"))
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	emitted := make(map[string]struct{})
	for {
		events, task, err := h.service.BuildTaskEvents(ctx, taskID)
		if err != nil {
			logger.ErrorWithFields(ctx, err, map[string]interface{}{"task_id": taskID})
			return
		}
		for _, event := range events {
			key := longDocumentEventKey(event)
			if _, ok := emitted[key]; ok {
				continue
			}
			emitted[key] = struct{}{}
			c.SSEvent(event.Type, event)
			flusher.Flush()
		}
		if task != nil && task.IsTerminal() {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func longDocumentEventKey(event types.LongDocumentTaskEvent) string {
	return event.Type + ":" + event.Timestamp.UTC().Format(time.RFC3339Nano) + ":" + event.TaskID + ":" + taskEventIdentity(event.Data)
}

func taskEventIdentity(data map[string]interface{}) string {
	if len(data) == 0 {
		return ""
	}
	if batchID, ok := data["batch_id"].(string); ok && batchID != "" {
		return batchID
	}
	if artifactID, ok := data["artifact_id"].(string); ok && artifactID != "" {
		return artifactID
	}
	if status, ok := data["status"].(string); ok && status != "" {
		return status
	}
	return "snapshot"
}
