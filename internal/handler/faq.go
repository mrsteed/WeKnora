package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// FAQHandler handles FAQ knowledge base operations.
type FAQHandler struct {
	knowledgeService interfaces.KnowledgeService
}

// NewFAQHandler creates a new FAQ handler.
func NewFAQHandler(knowledgeService interfaces.KnowledgeService) *FAQHandler {
	return &FAQHandler{knowledgeService: knowledgeService}
}

// ListEntries lists FAQ entries under a knowledge base.
func (h *FAQHandler) ListEntries(c *gin.Context) {
	ctx := c.Request.Context()
	var page types.Pagination
	if err := c.ShouldBindQuery(&page); err != nil {
		logger.Error(ctx, "Failed to bind pagination query", err)
		c.Error(errors.NewBadRequestError("分页参数不合法").WithDetails(err.Error()))
		return
	}

	tagID := c.Query("tag_id")

	result, err := h.knowledgeService.ListFAQEntries(ctx, c.Param("id"), &page, tagID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// UpsertEntries appends or replaces FAQ entries in batch.
func (h *FAQHandler) UpsertEntries(c *gin.Context) {
	ctx := c.Request.Context()
	var req types.FAQBatchUpsertPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to bind FAQ upsert payload", err)
		c.Error(errors.NewBadRequestError("请求参数不合法").WithDetails(err.Error()))
		return
	}

	if err := h.knowledgeService.UpsertFAQEntries(ctx, c.Param("id"), &req); err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

// UpdateEntry updates a single FAQ entry.
func (h *FAQHandler) UpdateEntry(c *gin.Context) {
	ctx := c.Request.Context()
	var req types.FAQEntryPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to bind FAQ entry payload", err)
		c.Error(errors.NewBadRequestError("请求参数不合法").WithDetails(err.Error()))
		return
	}

	if err := h.knowledgeService.UpdateFAQEntry(ctx, c.Param("id"), c.Param("entry_id"), &req); err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

type faqDeleteRequest struct {
	IDs []string `json:"ids" binding:"required,min=1,dive,required"`
}

// DeleteEntries deletes FAQ entries in batch.
func (h *FAQHandler) DeleteEntries(c *gin.Context) {
	ctx := c.Request.Context()
	var req faqDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to bind FAQ delete payload", err)
		c.Error(errors.NewBadRequestError("请求参数不合法").WithDetails(err.Error()))
		return
	}

	if err := h.knowledgeService.DeleteFAQEntries(ctx, c.Param("id"), req.IDs); err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

// SearchFAQ searches FAQ entries using hybrid search.
func (h *FAQHandler) SearchFAQ(c *gin.Context) {
	ctx := c.Request.Context()
	var req types.FAQSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to bind FAQ search payload", err)
		c.Error(errors.NewBadRequestError("请求参数不合法").WithDetails(err.Error()))
		return
	}

	entries, err := h.knowledgeService.SearchFAQEntries(ctx, c.Param("id"), &req)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    entries,
	})
}
