package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// OrgTreeHandler handles HTTP requests for organization tree management
type OrgTreeHandler struct {
	orgTreeService interfaces.OrgTreeService
	userService    interfaces.UserService
}

// NewOrgTreeHandler creates a new org-tree handler
func NewOrgTreeHandler(
	orgTreeService interfaces.OrgTreeService,
	userService interfaces.UserService,
) *OrgTreeHandler {
	return &OrgTreeHandler{
		orgTreeService: orgTreeService,
		userService:    userService,
	}
}

// GetOrgTree returns the full organization tree for the current tenant
// @Summary      获取组织树
// @Description  返回当前租户的完整组织树
// @Tags         组织树管理
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  errors.AppError
// @Security     Bearer
// @Router       /org-tree [get]
func (h *OrgTreeHandler) GetOrgTree(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())

	tree, err := h.orgTreeService.GetTree(ctx, tenantID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get org tree: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to get organization tree").WithDetails(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tree,
	})
}

// CreateOrgNode creates a new node in the organization tree
// @Summary      创建组织树节点
// @Description  在组织树中创建新节点
// @Tags         组织树管理
// @Accept       json
// @Produce      json
// @Param        request  body      types.CreateOrgTreeNodeRequest  true  "节点信息"
// @Success      201      {object}  map[string]interface{}
// @Failure      400      {object}  errors.AppError
// @Security     Bearer
// @Router       /org-tree [post]
func (h *OrgTreeHandler) CreateOrgNode(c *gin.Context) {
	ctx := c.Request.Context()
	userID := c.GetString(types.UserIDContextKey.String())
	tenantID := c.GetUint64(types.TenantIDContextKey.String())

	var req types.CreateOrgTreeNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Errorf(ctx, "Invalid request parameters: %v", err)
		c.Error(apperrors.NewValidationError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	org, err := h.orgTreeService.CreateNode(ctx, tenantID, userID, &req)
	if err != nil {
		logger.Errorf(ctx, "Failed to create org-tree node: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to create organization tree node").WithDetails(err.Error()))
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    org,
	})
}

// GetOrgNode gets a single organization tree node
// @Summary      获取组织树节点
// @Description  根据ID获取单个组织树节点
// @Tags         组织树管理
// @Produce      json
// @Param        id  path  string  true  "节点ID"
// @Success      200  {object}  map[string]interface{}
// @Failure      404  {object}  errors.AppError
// @Security     Bearer
// @Router       /org-tree/{id} [get]
func (h *OrgTreeHandler) GetOrgNode(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	nodeID := c.Param("id")

	org, err := h.orgTreeService.GetNode(ctx, nodeID, tenantID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get org-tree node %s: %v", nodeID, err)
		c.Error(apperrors.NewNotFoundError("Organization tree node not found").WithDetails(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    org,
	})
}

// UpdateOrgNode updates an organization tree node
// @Summary      更新组织树节点
// @Description  更新组织树节点的名称、描述等
// @Tags         组织树管理
// @Accept       json
// @Produce      json
// @Param        id       path      string                         true  "节点ID"
// @Param        request  body      types.UpdateOrgTreeNodeRequest  true  "更新信息"
// @Success      200      {object}  map[string]interface{}
// @Failure      400      {object}  errors.AppError
// @Security     Bearer
// @Router       /org-tree/{id} [put]
func (h *OrgTreeHandler) UpdateOrgNode(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	nodeID := c.Param("id")

	var req types.UpdateOrgTreeNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Errorf(ctx, "Invalid request parameters: %v", err)
		c.Error(apperrors.NewValidationError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	org, err := h.orgTreeService.UpdateNode(ctx, nodeID, tenantID, &req)
	if err != nil {
		logger.Errorf(ctx, "Failed to update org-tree node: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to update organization tree node").WithDetails(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    org,
	})
}

// DeleteOrgNode deletes an organization tree node
// @Summary      删除组织树节点
// @Description  删除组织树中的叶子节点
// @Tags         组织树管理
// @Produce      json
// @Param        id  path  string  true  "节点ID"
// @Success      200  {object}  map[string]interface{}
// @Failure      400  {object}  errors.AppError
// @Security     Bearer
// @Router       /org-tree/{id} [delete]
func (h *OrgTreeHandler) DeleteOrgNode(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	nodeID := c.Param("id")

	if err := h.orgTreeService.DeleteNode(ctx, nodeID, tenantID); err != nil {
		logger.Errorf(ctx, "Failed to delete org-tree node: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to delete organization tree node").WithDetails(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Organization tree node deleted",
	})
}

// MoveOrgNode moves a node in the organization tree
// @Summary      移动组织树节点
// @Description  将组织树节点移动到新的父节点下
// @Tags         组织树管理
// @Accept       json
// @Produce      json
// @Param        id       path      string               true  "节点ID"
// @Param        request  body      types.MoveOrgNodeRequest  true  "移动信息"
// @Success      200      {object}  map[string]interface{}
// @Failure      400      {object}  errors.AppError
// @Security     Bearer
// @Router       /org-tree/{id}/move [post]
func (h *OrgTreeHandler) MoveOrgNode(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	nodeID := c.Param("id")

	var req types.MoveOrgNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Errorf(ctx, "Invalid request parameters: %v", err)
		c.Error(apperrors.NewValidationError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	if err := h.orgTreeService.MoveNode(ctx, nodeID, tenantID, &req); err != nil {
		logger.Errorf(ctx, "Failed to move org-tree node: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to move organization tree node").WithDetails(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Organization tree node moved",
	})
}

// AssignUser assigns a user to an organization in the tree
// @Summary      分配用户到组织
// @Description  将用户分配到组织树中的某个节点
// @Tags         组织树管理
// @Accept       json
// @Produce      json
// @Param        id       path      string                       true  "组织ID"
// @Param        request  body      types.AssignUserToOrgRequest  true  "分配信息"
// @Success      200      {object}  map[string]interface{}
// @Failure      400      {object}  errors.AppError
// @Security     Bearer
// @Router       /org-tree/{id}/members [post]
func (h *OrgTreeHandler) AssignUser(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	orgID := c.Param("id")

	var req types.AssignUserToOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Errorf(ctx, "Invalid request parameters: %v", err)
		c.Error(apperrors.NewValidationError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	if err := h.orgTreeService.AssignUserToOrg(ctx, orgID, tenantID, &req); err != nil {
		logger.Errorf(ctx, "Failed to assign user to org: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to assign user to organization").WithDetails(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User assigned to organization",
	})
}

// CreateUserInOrg creates a new user and assigns them to an organization
// @Summary      在组织中创建用户
// @Description  管理员创建新用户并将其分配到指定组织节点
// @Tags         OrgTree
// @Accept       json
// @Produce      json
// @Param        id       path      string                         true   "组织节点ID"
// @Param        request  body      types.CreateUserInOrgRequest   true   "创建用户信息"
// @Success      200      {object}  types.CreateUserInOrgResponse
// @Failure      400      {object}  errors.AppError
// @Security     Bearer
// @Router       /org-tree/{id}/create-user [post]
func (h *OrgTreeHandler) CreateUserInOrg(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	orgID := c.Param("id")

	var req types.CreateUserInOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Errorf(ctx, "Invalid request parameters: %v", err)
		c.Error(apperrors.NewValidationError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	// Step 1: Create user via user service
	user, err := h.userService.CreateUserByAdmin(ctx, &req, tenantID)
	if err != nil {
		logger.Errorf(ctx, "Failed to create user: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to create user").WithDetails(err.Error()))
		return
	}

	// Step 2: Assign user to org
	assignReq := &types.AssignUserToOrgRequest{
		UserID: user.ID,
		Role:   types.OrgMemberRole(req.Role),
	}
	if err := h.orgTreeService.AssignUserToOrg(ctx, orgID, tenantID, assignReq); err != nil {
		logger.Errorf(ctx, "User created but failed to assign to org: %v", err)
		// User is created but assignment failed — return partial success
		c.JSON(http.StatusOK, types.CreateUserInOrgResponse{
			Success: true,
			Message: "User created but failed to assign to organization: " + err.Error(),
			User:    user.ToUserInfo(),
		})
		return
	}

	c.JSON(http.StatusOK, types.CreateUserInOrgResponse{
		Success: true,
		Message: "User created and assigned to organization successfully",
		User:    user.ToUserInfo(),
	})
}

// UpdateUserInOrg updates user information
// @Summary      更新用户信息
// @Description  管理员更新用户的基本信息和组织角色
// @Tags         OrgTree
// @Accept       json
// @Produce      json
// @Param        id       path      string                         true   "组织节点ID"
// @Param        user_id  path      string                         true   "用户ID"
// @Param        request  body      types.UpdateUserInOrgRequest   true   "更新用户信息"
// @Success      200      {object}  types.CreateUserInOrgResponse
// @Failure      400      {object}  errors.AppError
// @Security     Bearer
// @Router       /org-tree/{id}/users/{user_id} [put]
func (h *OrgTreeHandler) UpdateUserInOrg(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	orgID := c.Param("id")
	userID := c.Param("user_id")

	var req types.UpdateUserInOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Errorf(ctx, "Invalid request parameters: %v", err)
		c.Error(apperrors.NewValidationError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	// Get user
	user, err := h.userService.GetUserByID(ctx, userID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get user: %v", err)
		c.Error(apperrors.NewNotFoundError("User not found").WithDetails(err.Error()))
		return
	}

	// Check if user belongs to tenant
	if user.TenantID != tenantID {
		logger.Errorf(ctx, "User does not belong to tenant")
		c.Error(apperrors.NewForbiddenError("User does not belong to your tenant"))
		return
	}

	// At least one of email or phone is required
	if req.Email == "" && req.Phone == "" {
		c.Error(apperrors.NewValidationError("At least one of email or phone is required"))
		return
	}

	// Check username uniqueness (if changed)
	if req.Username != user.Username {
		existingUser, _ := h.userService.GetUserByUsername(ctx, req.Username)
		if existingUser != nil && existingUser.ID != userID {
			c.Error(apperrors.NewBadRequestError("Username already exists"))
			return
		}
	}

	// Check email uniqueness (if provided and changed)
	if req.Email != "" && req.Email != user.Email {
		existingUser, _ := h.userService.GetUserByEmail(ctx, req.Email)
		if existingUser != nil && existingUser.ID != userID {
			c.Error(apperrors.NewBadRequestError("Email already exists"))
			return
		}
	}

	// Check phone uniqueness (if provided and changed)
	if req.Phone != "" && req.Phone != user.Phone {
		existingUser, _ := h.userService.GetUserByPhone(ctx, req.Phone)
		if existingUser != nil && existingUser.ID != userID {
			c.Error(apperrors.NewBadRequestError("Phone already exists"))
			return
		}
	}

	// Update user info
	user.Username = req.Username
	user.Email = req.Email
	user.Phone = req.Phone

	if err := h.userService.UpdateUser(ctx, user); err != nil {
		logger.Errorf(ctx, "Failed to update user: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to update user").WithDetails(err.Error()))
		return
	}

	// Update role in organization if provided
	if req.Role != "" {
		assignReq := &types.AssignUserToOrgRequest{
			UserID: userID,
			Role:   types.OrgMemberRole(req.Role),
		}
		if err := h.orgTreeService.AssignUserToOrg(ctx, orgID, tenantID, assignReq); err != nil {
			logger.Warnf(ctx, "User updated but failed to update role in org: %v", err)
		}
	}

	c.JSON(http.StatusOK, types.CreateUserInOrgResponse{
		Success: true,
		Message: "User updated successfully",
		User:    user.ToUserInfo(),
	})
}

// RemoveUser removes a user from an organization in the tree
// @Summary      从组织移除用户
// @Description  将用户从组织树中的某个节点移除
// @Tags         组织树管理
// @Produce      json
// @Param        id       path  string  true  "组织ID"
// @Param        user_id  path  string  true  "用户ID"
// @Success      200  {object}  map[string]interface{}
// @Failure      400  {object}  errors.AppError
// @Security     Bearer
// @Router       /org-tree/{id}/members/{user_id} [delete]
func (h *OrgTreeHandler) RemoveUser(c *gin.Context) {
	ctx := c.Request.Context()
	orgID := c.Param("id")
	userID := c.Param("user_id")
	tenantID := c.GetUint64(types.TenantIDContextKey.String())

	req := &types.RemoveUserFromOrgRequest{
		UserID: userID,
	}

	if err := h.orgTreeService.RemoveUserFromOrg(ctx, orgID, tenantID, req); err != nil {
		logger.Errorf(ctx, "Failed to remove user from org: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to remove user from organization").WithDetails(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User removed from organization",
	})
}

// SetOrgAdmin sets or unsets a user as organization admin
// @Summary      设置组织管理员
// @Description  设置或取消用户的组织管理员身份
// @Tags         组织树管理
// @Accept       json
// @Produce      json
// @Param        id       path      string                  true  "组织ID"
// @Param        request  body      types.SetOrgAdminRequest  true  "管理员设置"
// @Success      200  {object}  map[string]interface{}
// @Failure      400  {object}  errors.AppError
// @Security     Bearer
// @Router       /org-tree/{id}/admin [put]
func (h *OrgTreeHandler) SetOrgAdmin(c *gin.Context) {
	ctx := c.Request.Context()
	orgID := c.Param("id")
	tenantID := c.GetUint64(types.TenantIDContextKey.String())

	var req types.SetOrgAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Errorf(ctx, "Invalid request parameters: %v", err)
		c.Error(apperrors.NewValidationError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	if err := h.orgTreeService.SetOrgAdmin(ctx, orgID, tenantID, &req); err != nil {
		logger.Errorf(ctx, "Failed to set org admin: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to set organization admin").WithDetails(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Organization admin updated",
	})
}

// GetMyOrganizations returns the current user's org-tree organizations within the tenant
// @Summary      获取我的组织
// @Description  返回当前用户在租户组织树中所属的组织列表
// @Tags         组织树管理
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  errors.AppError
// @Security     Bearer
// @Router       /my-organizations [get]
func (h *OrgTreeHandler) GetMyOrganizations(c *gin.Context) {
	ctx := c.Request.Context()
	userID := c.GetString(types.UserIDContextKey.String())
	tenantID := c.GetUint64(types.TenantIDContextKey.String())

	orgs, err := h.orgTreeService.GetUserOrganizations(ctx, userID, tenantID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get user organizations: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to get user organizations").WithDetails(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    orgs,
	})
}

// ListOrgMembers returns the members of a specific org-tree node
// @Summary      获取组织成员列表
// @Description  返回组织树中某个节点的成员列表
// @Tags         组织树管理
// @Produce      json
// @Param        id  path  string  true  "组织ID"
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  errors.AppError
// @Security     Bearer
// @Router       /org-tree/{id}/members [get]
func (h *OrgTreeHandler) ListOrgMembers(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	orgID := c.Param("id")

	members, err := h.orgTreeService.ListOrgMembers(ctx, orgID, tenantID)
	if err != nil {
		logger.Errorf(ctx, "Failed to list org members: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to list organization members").WithDetails(err.Error()))
		return
	}

	// Transform to flat structure with user info
	result := make([]gin.H, 0, len(members))
	for _, m := range members {
		if m.User == nil {
			continue
		}
		result = append(result, gin.H{
			"user_id":        m.UserID,
			"username":       m.User.Username,
			"email":          m.User.Email,
			"phone":          m.User.Phone,
			"role":           string(m.Role),
			"is_admin":       m.Role == types.OrgRoleAdmin,
			"is_super_admin": m.User.IsSuperAdmin,
			"joined_at":      m.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// SearchUsersForAssign searches users that can be assigned to organizations
// @Summary      搜索可分配用户
// @Description  搜索可分配到组织中的用户
// @Tags         组织树管理
// @Produce      json
// @Param        q      query  string  false  "搜索关键词"
// @Param        limit  query  int     false  "最大返回数量"
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  errors.AppError
// @Security     Bearer
// @Router       /org-tree/search-users [get]
func (h *OrgTreeHandler) SearchUsersForAssign(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	query := c.Query("q")
	limitStr := c.DefaultQuery("limit", "20")
	limit := 20
	if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
		limit = v
	}

	users, err := h.userService.SearchUsers(ctx, query, limit)
	if err != nil {
		logger.Errorf(ctx, "Failed to search users: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to search users").WithDetails(err.Error()))
		return
	}

	// Return safe user info (no passwords), filter by tenant
	result := make([]gin.H, 0, len(users))
	for _, u := range users {
		// Filter: only users belonging to the same tenant
		if u.TenantID != tenantID {
			continue
		}
		result = append(result, gin.H{
			"id":             u.ID,
			"username":       u.Username,
			"email":          u.Email,
			"is_super_admin": u.IsSuperAdmin,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// SetSuperAdmin sets or unsets a user as super admin
// @Summary      设为/取消超级管理员
// @Description  设置或取消某用户的超级管理员权限
// @Tags         组织树管理
// @Accept       json
// @Produce      json
// @Param        request  body      object  true  "用户ID和超管状态"
// @Success      200      {object}  map[string]interface{}
// @Failure      400      {object}  errors.AppError
// @Security     Bearer
// @Router       /org-tree/super-admin [put]
func (h *OrgTreeHandler) SetSuperAdmin(c *gin.Context) {
	ctx := c.Request.Context()
	currentUserID := c.GetString(types.UserIDContextKey.String())

	var req struct {
		UserID       string `json:"user_id" binding:"required"`
		IsSuperAdmin bool   `json:"is_super_admin"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewValidationError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	// Self-protection: prevent super admin from revoking their own super admin status
	if req.UserID == currentUserID && !req.IsSuperAdmin {
		c.Error(apperrors.NewBadRequestError("Cannot revoke your own super admin privileges"))
		return
	}

	user, err := h.userService.GetUserByID(ctx, req.UserID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get user %s: %v", req.UserID, err)
		c.Error(apperrors.NewNotFoundError("User not found").WithDetails(err.Error()))
		return
	}

	user.IsSuperAdmin = req.IsSuperAdmin
	if err := h.userService.UpdateUser(ctx, user); err != nil {
		logger.Errorf(ctx, "Failed to update user super admin status: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to update super admin status").WithDetails(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Super admin status updated",
	})
}
