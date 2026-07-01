package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/im"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
)

// CustomAgentHandler defines the HTTP handler for custom agent operations
type CustomAgentHandler struct {
	service           interfaces.CustomAgentService
	pageShareService  interfaces.AgentPageShareService
	visibilityService interfaces.AgentVisibilityService
	imService         *im.Service
	disabledRepo      interfaces.TenantDisabledSharedAgentRepository
}

// NewCustomAgentHandler creates a new custom agent handler instance
func NewCustomAgentHandler(
	service interfaces.CustomAgentService,
	pageShareService interfaces.AgentPageShareService,
	visibilityService interfaces.AgentVisibilityService,
	imService *im.Service,
	disabledRepo interfaces.TenantDisabledSharedAgentRepository,
) *CustomAgentHandler {
	return &CustomAgentHandler{
		service:           service,
		pageShareService:  pageShareService,
		visibilityService: visibilityService,
		imService:         imService,
		disabledRepo:      disabledRepo,
	}
}

// CreateAgentRequest defines the request body for creating an agent
type CreateAgentRequest struct {
	Name           string                  `json:"name" binding:"required"`
	Description    string                  `json:"description"`
	Avatar         string                  `json:"avatar"`
	Config         types.CustomAgentConfig `json:"config"`
	Visibility     string                  `json:"visibility"`
	OrganizationID string                  `json:"organization_id"`
}

// UpdateAgentRequest defines the request body for updating an agent
type UpdateAgentRequest struct {
	Name           string                  `json:"name"`
	Description    string                  `json:"description"`
	Avatar         string                  `json:"avatar"`
	Visibility     string                  `json:"visibility"`
	OrganizationID string                  `json:"organization_id"`
	Config         types.CustomAgentConfig `json:"config"`
}

// CreateAgent godoc
// @Summary      创建智能体
// @Description  创建新的自定义智能体
// @Tags         智能体
// @Accept       json
// @Produce      json
// @Param        request  body      CreateAgentRequest  true  "智能体信息"
// @Success      201      {object}  map[string]interface{}  "创建的智能体"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /agents [post]
func (h *CustomAgentHandler) CreateAgent(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start creating custom agent")

	// Parse request body
	var req CreateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewBadRequestError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	// Get user from context for created_by
	userID := c.GetString(types.UserIDContextKey.String())

	// Validate and default visibility
	visibility := req.Visibility
	if visibility == "" {
		visibility = types.AgentVisibilityPrivate
	}
	if visibility != types.AgentVisibilityGlobal && visibility != types.AgentVisibilityOrg && visibility != types.AgentVisibilityPrivate {
		c.Error(errors.NewBadRequestError("Invalid visibility value, must be one of: global, org, private"))
		return
	}
	if visibility == types.AgentVisibilityOrg && req.OrganizationID == "" {
		c.Error(errors.NewBadRequestError("organization_id is required when visibility is 'org'"))
		return
	}

	// Build agent object
	agent := &types.CustomAgent{
		Name:           req.Name,
		Description:    req.Description,
		Avatar:         req.Avatar,
		Config:         req.Config,
		CreatedBy:      userID,
		Visibility:     visibility,
		OrganizationID: req.OrganizationID,
	}

	logger.Infof(ctx, "Creating custom agent, name: %s, agent_mode: %s",
		secutils.SanitizeForLog(req.Name), req.Config.AgentMode)

	// Create agent using the service
	createdAgent, err := h.service.CreateAgent(ctx, agent)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		if err == service.ErrAgentNameRequired {
			c.Error(errors.NewBadRequestError(err.Error()))
			return
		}
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Custom agent created successfully, ID: %s, name: %s",
		secutils.SanitizeForLog(createdAgent.ID), secutils.SanitizeForLog(createdAgent.Name))
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    createdAgent,
	})
}

// GetAgent godoc
// @Summary      获取智能体详情
// @Description  根据ID获取智能体详情
// @Tags         智能体
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "智能体ID"
// @Success      200  {object}  map[string]interface{}  "智能体详情"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      404  {object}  errors.AppError         "智能体不存在"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /agents/{id} [get]
func (h *CustomAgentHandler) GetAgent(c *gin.Context) {
	ctx := c.Request.Context()

	// Get agent ID from URL parameter
	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Agent ID is empty")
		c.Error(errors.NewBadRequestError("Agent ID cannot be empty"))
		return
	}

	agent, err := h.service.GetAgentByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id": id,
		})
		if err == service.ErrAgentNotFound {
			c.Error(errors.NewNotFoundError("Agent not found"))
			return
		}
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    agent,
	})
}

// ListAgents godoc
// @Summary      获取智能体列表
// @Description  获取当前租户的所有智能体（包括内置智能体）
// @Tags         智能体
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "智能体列表"
// @Failure      500  {object}  errors.AppError         "服务器错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /agents [get]
func (h *CustomAgentHandler) ListAgents(c *gin.Context) {
	ctx := c.Request.Context()

	// Get user info for visibility filtering
	userID := c.GetString(types.UserIDContextKey.String())
	tenantIDVal, exists := c.Get(types.TenantIDContextKey.String())
	if !exists {
		logger.Error(ctx, "Tenant ID not found in context")
		c.Error(errors.NewUnauthorizedError("Missing tenant context"))
		return
	}
	tenantID, ok := tenantIDVal.(uint64)
	if !ok {
		logger.Errorf(ctx, "Tenant ID has unexpected type %T in context", tenantIDVal)
		c.Error(errors.NewInternalServerError("Invalid tenant context type"))
		return
	}
	user, exists := c.Get(types.UserContextKey.String())
	isSuperAdmin := false
	if exists {
		if u, ok := user.(*types.User); ok {
			isSuperAdmin = u.IsSuperAdmin
		}
	}

	// Get agents filtered by visibility
	agents, err := h.visibilityService.ListAccessibleAgents(ctx, userID, tenantID, isSuperAdmin)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	// Per-tenant "disabled by me" for own agents (only affects this tenant's conversation dropdown)
	disabledOwnIDs, err := h.disabledRepo.ListDisabledOwnAgentIDs(ctx, tenantID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tenant_id": tenantID,
		})
		c.Error(errors.NewInternalServerError("Failed to list disabled agent IDs: " + err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":                true,
		"data":                   agents,
		"disabled_own_agent_ids": disabledOwnIDs,
	})
}

// UpdateAgent godoc
// @Summary      更新智能体
// @Description  更新智能体的名称、描述和配置
// @Tags         智能体
// @Accept       json
// @Produce      json
// @Param        id       path      string              true  "智能体ID"
// @Param        request  body      UpdateAgentRequest  true  "更新请求"
// @Success      200      {object}  map[string]interface{}  "更新后的智能体"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Failure      403      {object}  errors.AppError         "无法修改内置智能体"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /agents/{id} [put]
func (h *CustomAgentHandler) UpdateAgent(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start updating custom agent")

	// Get agent ID from URL parameter
	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Agent ID is empty")
		c.Error(errors.NewBadRequestError("Agent ID cannot be empty"))
		return
	}

	// Get current user
	userVal, ok := c.Get(types.UserContextKey.String())
	if !ok {
		c.Error(errors.NewUnauthorizedError("User context not found"))
		return
	}
	user, ok := userVal.(*types.User)
	if !ok || user == nil {
		c.Error(errors.NewUnauthorizedError("Invalid user context"))
		return
	}

	// Get existing agent to check permissions
	existingAgent, err := h.service.GetAgentByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"agent_id": id})
		if err == service.ErrAgentNotFound {
			c.Error(errors.NewNotFoundError("Agent not found"))
		} else {
			c.Error(errors.NewInternalServerError(err.Error()))
		}
		return
	}

	// Debug log for permission check
	logger.Infof(ctx, "Permission check - Agent ID: %s, CreatedBy: %s, User ID: %s, IsSuperAdmin: %v",
		existingAgent.ID, existingAgent.CreatedBy, user.ID, user.IsSuperAdmin)

	// Check if this is a built-in agent - only super admin can update
	if types.IsBuiltinAgentID(id) {
		if !user.IsSuperAdmin {
			c.Error(errors.NewForbiddenError("Only super admins can update built-in agents"))
			return
		}
	} else {
		// For custom agents - only creator or super admin can update
		// If CreatedBy is empty, allow update (for backward compatibility with old data)
		if existingAgent.CreatedBy != "" && existingAgent.CreatedBy != user.ID && !user.IsSuperAdmin {
			c.Error(errors.NewForbiddenError("Only the creator or super admin can update this agent"))
			return
		}
	}

	// Parse request body
	var req UpdateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewBadRequestError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	// Build agent object
	agent := &types.CustomAgent{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Avatar:      req.Avatar,
		Config:      req.Config,
	}

	// Handle visibility update
	visibility := req.Visibility
	if visibility != "" {
		if visibility != types.AgentVisibilityGlobal && visibility != types.AgentVisibilityOrg && visibility != types.AgentVisibilityPrivate {
			c.Error(errors.NewBadRequestError("Invalid visibility value, must be one of: global, org, private"))
			return
		}
		if visibility == types.AgentVisibilityOrg && req.OrganizationID == "" {
			c.Error(errors.NewBadRequestError("organization_id is required when visibility is org"))
			return
		}
		agent.Visibility = visibility
		agent.OrganizationID = req.OrganizationID
	}

	logger.Infof(ctx, "Updating custom agent, ID: %s, name: %s",
		secutils.SanitizeForLog(id), secutils.SanitizeForLog(req.Name))

	// Update the agent
	updatedAgent, err := h.service.UpdateAgent(ctx, agent)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id": id,
		})
		switch err {
		case service.ErrAgentNotFound:
			c.Error(errors.NewNotFoundError("Agent not found"))
		case service.ErrCannotModifyBuiltin:
			c.Error(errors.NewForbiddenError("Cannot modify built-in agent"))
		case service.ErrAgentNameRequired:
			c.Error(errors.NewBadRequestError(err.Error()))
		default:
			c.Error(errors.NewInternalServerError(err.Error()))
		}
		return
	}

	logger.Infof(ctx, "Custom agent updated successfully, ID: %s", secutils.SanitizeForLog(id))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    updatedAgent,
	})
}

// DeleteAgent godoc
// @Summary      删除智能体
// @Description  删除指定的智能体
// @Tags         智能体
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "智能体ID"
// @Success      200  {object}  map[string]interface{}  "删除成功"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      403  {object}  errors.AppError         "无法删除内置智能体"
// @Failure      404  {object}  errors.AppError         "智能体不存在"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /agents/{id} [delete]
func (h *CustomAgentHandler) DeleteAgent(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start deleting custom agent")

	// Get agent ID from URL parameter
	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Agent ID is empty")
		c.Error(errors.NewBadRequestError("Agent ID cannot be empty"))
		return
	}

	// Get current user
	userVal, ok := c.Get(types.UserContextKey.String())
	if !ok {
		c.Error(errors.NewUnauthorizedError("User context not found"))
		return
	}
	user, ok := userVal.(*types.User)
	if !ok || user == nil {
		c.Error(errors.NewUnauthorizedError("Invalid user context"))
		return
	}

	// Get existing agent to check permissions
	existingAgent, err := h.service.GetAgentByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"agent_id": id})
		if err == service.ErrAgentNotFound {
			c.Error(errors.NewNotFoundError("Agent not found"))
		} else {
			c.Error(errors.NewInternalServerError(err.Error()))
		}
		return
	}

	// Debug log for permission check
	logger.Infof(ctx, "Permission check - Agent ID: %s, CreatedBy: %s, User ID: %s, IsSuperAdmin: %v",
		existingAgent.ID, existingAgent.CreatedBy, user.ID, user.IsSuperAdmin)

	// Check permissions - only creator or super admin can delete
	// If CreatedBy is empty, allow delete (for backward compatibility with old data)
	if existingAgent.CreatedBy != "" && existingAgent.CreatedBy != user.ID && !user.IsSuperAdmin {
		c.Error(errors.NewForbiddenError("Only the creator or super admin can delete this agent"))
		return
	}

	logger.Infof(ctx, "Deleting custom agent, ID: %s", secutils.SanitizeForLog(id))

	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}

	if err := h.imService.DeleteChannelsByAgent(id, tenantID); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id": id,
		})
		c.Error(errors.NewInternalServerError("Failed to delete agent IM channels"))
		return
	}

	// Delete the agent
	err = h.service.DeleteAgent(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id": id,
		})
		switch err {
		case service.ErrAgentNotFound:
			c.Error(errors.NewNotFoundError("Agent not found"))
		case service.ErrCannotDeleteBuiltin:
			c.Error(errors.NewForbiddenError("Cannot delete built-in agent"))
		default:
			c.Error(errors.NewInternalServerError(err.Error()))
		}
		return
	}

	logger.Infof(ctx, "Custom agent deleted successfully, ID: %s", secutils.SanitizeForLog(id))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Agent deleted successfully",
	})
}

// CopyAgent godoc
// @Summary      复制智能体
// @Description  复制指定的智能体
// @Tags         智能体
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "智能体ID"
// @Success      201  {object}  map[string]interface{}  "复制成功"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      404  {object}  errors.AppError         "智能体不存在"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /agents/{id}/copy [post]
func (h *CustomAgentHandler) CopyAgent(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start copying custom agent")

	// Get agent ID from URL parameter
	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Agent ID is empty")
		c.Error(errors.NewBadRequestError("Agent ID cannot be empty"))
		return
	}

	logger.Infof(ctx, "Copying custom agent, ID: %s", secutils.SanitizeForLog(id))

	// Copy the agent
	copiedAgent, err := h.service.CopyAgent(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id": id,
		})
		switch err {
		case service.ErrAgentNotFound:
			c.Error(errors.NewNotFoundError("Agent not found"))
		default:
			c.Error(errors.NewInternalServerError(err.Error()))
		}
		return
	}

	logger.Infof(ctx, "Custom agent copied successfully, source ID: %s, new ID: %s",
		secutils.SanitizeForLog(id), secutils.SanitizeForLog(copiedAgent.ID))
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    copiedAgent,
	})
}

// GetPlaceholders godoc
// @Summary      获取占位符定义
// @Description  获取所有可用的提示词占位符定义，按字段类型分组
// @Tags         智能体
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "占位符定义"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /agents/placeholders [get]
func (h *CustomAgentHandler) GetPlaceholders(c *gin.Context) {
	// Return all placeholder definitions grouped by field type
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"all":                   types.AllPlaceholders(),
			"system_prompt":         types.PlaceholdersByField(types.PromptFieldSystemPrompt),
			"agent_system_prompt":   types.PlaceholdersByField(types.PromptFieldAgentSystemPrompt),
			"context_template":      types.PlaceholdersByField(types.PromptFieldContextTemplate),
			"rewrite_system_prompt": types.PlaceholdersByField(types.PromptFieldRewriteSystemPrompt),
			"rewrite_prompt":        types.PlaceholdersByField(types.PromptFieldRewritePrompt),
			"fallback_prompt":       types.PlaceholdersByField(types.PromptFieldFallbackPrompt),
		},
	})
}

// GetAgentTypePresets godoc
// @Summary      获取智能体类型预设列表
// @Description  返回所有 smart-reasoning 下可用的智能体类型预设（RAG/Wiki/Hybrid/Data/Database/Custom），用于编辑器自动填充系统提示词、工具和 KB 兼容性
// @Tags         智能体
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "预设列表"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /agents/type-presets [get]
func (h *CustomAgentHandler) GetAgentTypePresets(c *gin.Context) {
	ctx := c.Request.Context()
	presets := types.ListAgentTypePresetsWithContext(ctx)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    presets,
	})
}

// GetSuggestedQuestions godoc
// @Summary      获取推荐问题
// @Description  基于智能体关联的知识库，返回推荐问题供用户快捷提问
// @Tags         智能体
// @Accept       json
// @Produce      json
// @Param        id                  path      string  true   "智能体ID"
// @Param        knowledge_base_ids  query     string  false  "知识库ID列表（逗号分隔），覆盖智能体默认配置"
// @Param        knowledge_ids       query     string  false  "知识ID列表（逗号分隔），限定到具体文档"
// @Param        limit               query     int     false  "返回数量上限（默认6）"
// @Success      200                 {object}  map[string]interface{}  "推荐问题列表"
// @Failure      400                 {object}  errors.AppError         "请求参数错误"
// @Failure      404                 {object}  errors.AppError         "智能体不存在"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /agents/{id}/suggested-questions [get]
func (h *CustomAgentHandler) GetSuggestedQuestions(c *gin.Context) {
	ctx := c.Request.Context()

	// Get agent ID from URL parameter
	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Agent ID is empty")
		c.Error(errors.NewBadRequestError("Agent ID cannot be empty"))
		return
	}

	// Parse optional query parameters
	var kbIDs []string
	if kbIDsStr := strings.TrimSpace(c.Query("knowledge_base_ids")); kbIDsStr != "" {
		for _, id := range strings.Split(kbIDsStr, ",") {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				kbIDs = append(kbIDs, trimmed)
			}
		}
	}

	var knowledgeIDs []string
	if kIDsStr := strings.TrimSpace(c.Query("knowledge_ids")); kIDsStr != "" {
		for _, id := range strings.Split(kIDsStr, ",") {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				knowledgeIDs = append(knowledgeIDs, trimmed)
			}
		}
	}

	var tagIDs []string
	if tagIDsStr := strings.TrimSpace(c.Query("tag_ids")); tagIDsStr != "" {
		for _, id := range strings.Split(tagIDsStr, ",") {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				tagIDs = append(tagIDs, trimmed)
			}
		}
	}

	limit := 6
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	logger.Infof(ctx, "Getting suggested questions for agent %s, kbIDs: %v, tagIDs: %v, limit: %d",
		secutils.SanitizeForLog(id), kbIDs, tagIDs, limit)

	questions, err := h.service.GetSuggestedQuestions(ctx, id, kbIDs, knowledgeIDs, tagIDs, limit)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id": id,
		})
		if err == service.ErrAgentNotFound {
			c.Error(errors.NewNotFoundError("Agent not found"))
			return
		}
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"questions": questions,
		},
	})
}

// GetAgentPageShare returns the current page-share state of an owned custom agent.
func (h *CustomAgentHandler) GetAgentPageShare(c *gin.Context) {
	ctx, agent, _, tenantID, ok := h.getManageableCustomAgent(c)
	if !ok {
		return
	}

	share, err := h.pageShareService.GetByAgent(ctx, agent.ID, tenantID)
	if err != nil {
		if err == service.ErrAgentPageShareNotFound {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data":    nil,
			})
			return
		}
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"agent_id": agent.ID, "tenant_id": tenantID})
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    buildAgentPageShareManagementView(share),
	})
}

// CreateOrEnableAgentPageShare opens or re-enables the public share link of an owned custom agent.
func (h *CustomAgentHandler) CreateOrEnableAgentPageShare(c *gin.Context) {
	ctx, agent, user, tenantID, ok := h.getManageableCustomAgent(c)
	if !ok {
		return
	}

	share, err := h.pageShareService.CreateOrEnable(ctx, agent.ID, user.ID, tenantID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"agent_id": agent.ID, "tenant_id": tenantID})
		switch err {
		case service.ErrSharedAgentNotFound:
			c.Error(errors.NewNotFoundError("Agent not found"))
		case service.ErrAgentNotConfiguredForPageSharing:
			c.Error(errors.NewBadRequestError(err.Error()))
		default:
			c.Error(errors.NewInternalServerError(err.Error()))
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    buildAgentPageShareManagementView(share),
	})
}

// DeleteAgentPageShare disables the public share link of an owned custom agent.
func (h *CustomAgentHandler) DeleteAgentPageShare(c *gin.Context) {
	ctx, agent, _, tenantID, ok := h.getManageableCustomAgent(c)
	if !ok {
		return
	}

	err := h.pageShareService.Disable(ctx, agent.ID, tenantID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"agent_id": agent.ID, "tenant_id": tenantID})
		if err == service.ErrAgentPageShareNotFound {
			c.Error(errors.NewNotFoundError("Agent page share not found"))
			return
		}
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Agent page share disabled successfully",
	})
}

// GetPublicAgentPageShare returns the anonymous readonly metadata used by the shared chat shell.
func (h *CustomAgentHandler) GetPublicAgentPageShare(c *gin.Context) {
	ctx := c.Request.Context()
	shareCode := strings.TrimSpace(c.Param("share_code"))
	if shareCode == "" {
		c.Error(errors.NewBadRequestError("share_code cannot be empty"))
		return
	}

	info, err := h.pageShareService.GetPublicInfo(ctx, shareCode)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"share_code": secutils.SanitizeForLog(shareCode)})
		switch err {
		case service.ErrAgentPageShareNotFound, service.ErrAgentPageShareUnavailable, service.ErrSharedAgentNotFound:
			c.Error(errors.NewNotFoundError("Agent page share not found"))
		default:
			c.Error(errors.NewInternalServerError(err.Error()))
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    info,
	})
}

func (h *CustomAgentHandler) getManageableCustomAgent(c *gin.Context) (context.Context, *types.CustomAgent, *types.User, uint64, bool) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		c.Error(errors.NewBadRequestError("Agent ID cannot be empty"))
		return nil, nil, nil, 0, false
	}
	if types.IsBuiltinAgentID(id) {
		c.Error(errors.NewBadRequestError("Built-in agents do not support page sharing"))
		return nil, nil, nil, 0, false
	}

	tenantID, ok := getTenantIDFromGin(c)
	if !ok {
		c.Error(errors.NewUnauthorizedError("Missing tenant context"))
		return nil, nil, nil, 0, false
	}

	userVal, ok := c.Get(types.UserContextKey.String())
	if !ok {
		c.Error(errors.NewUnauthorizedError("User context not found"))
		return nil, nil, nil, 0, false
	}
	user, ok := userVal.(*types.User)
	if !ok || user == nil {
		c.Error(errors.NewUnauthorizedError("Invalid user context"))
		return nil, nil, nil, 0, false
	}

	ctx = context.WithValue(ctx, types.TenantIDContextKey, tenantID)
	agent, err := h.service.GetAgentByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"agent_id": id})
		if err == service.ErrAgentNotFound {
			c.Error(errors.NewNotFoundError("Agent not found"))
		} else {
			c.Error(errors.NewInternalServerError(err.Error()))
		}
		return nil, nil, nil, 0, false
	}
	if agent.CreatedBy != "" && agent.CreatedBy != user.ID && !user.IsSuperAdmin {
		c.Error(errors.NewForbiddenError("Only the creator or super admin can manage this agent page share"))
		return nil, nil, nil, 0, false
	}
	return ctx, agent, user, tenantID, true
}

func getTenantIDFromGin(c *gin.Context) (uint64, bool) {
	tenantIDVal, exists := c.Get(types.TenantIDContextKey.String())
	if !exists {
		return 0, false
	}
	tenantID, ok := tenantIDVal.(uint64)
	if !ok {
		return 0, false
	}
	return tenantID, true
}

func buildAgentPageShareManagementView(share *types.AgentPageShare) *types.AgentPageShareManagementView {
	if share == nil {
		return nil
	}
	return &types.AgentPageShareManagementView{
		ID:                    share.ID,
		AgentID:               share.AgentID,
		SourceTenantID:        share.SourceTenantID,
		ShareCode:             share.ShareCode,
		Status:                share.Status,
		AccessScope:           share.AccessScope,
		ShareURL:              "/share/agents/" + share.ShareCode,
		AnonymousSessionLimit: share.AnonymousSessionLimit,
		RateLimitPerMinute:    share.RateLimitPerMinute,
		LastAccessedAt:        share.LastAccessedAt,
		ExpiresAt:             share.ExpiresAt,
		CreatedAt:             share.CreatedAt,
		UpdatedAt:             share.UpdatedAt,
	}
}
