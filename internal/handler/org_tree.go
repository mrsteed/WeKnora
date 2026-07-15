package handler

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"

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
	memberService  interfaces.TenantMemberService
}

const (
	orgTreeDisplayRoleOwner        = "owner"
	orgTreeDisplayRoleAdmin        = "admin"
	orgTreeDisplayRoleSubOrgAdmin  = "suborg_admin"
	orgTreeDisplayRoleCollaborator = "collaborator"
	orgTreeDisplayRoleReadonly     = "readonly"

	orgTreeRoleScopeWorkspaceOwner = "workspace_owner"
	orgTreeRoleScopeWorkspaceAdmin = "workspace_admin"
	orgTreeRoleScopeSubOrgAdmin    = "suborg_admin"
	orgTreeRoleScopeOrgEditor      = "org_editor"
	orgTreeRoleScopeOrgViewer      = "org_viewer"
	orgTreeRoleScopeInheritedAdmin = "inherited_admin"

	orgTreeMemberSourceTenantProjection = "tenant_projection"
	orgTreeMemberSourceOrgDirect        = "org_direct"
	orgTreeMemberSourceOrgInherited     = "org_inherited"
)

// NewOrgTreeHandler creates a new org-tree handler
func NewOrgTreeHandler(
	orgTreeService interfaces.OrgTreeService,
	userService interfaces.UserService,
	memberService interfaces.TenantMemberService,
) *OrgTreeHandler {
	return &OrgTreeHandler{
		orgTreeService: orgTreeService,
		userService:    userService,
		memberService:  memberService,
	}
}

func normalizeOrgRole(raw string) (types.OrgMemberRole, error) {
	role := types.OrgMemberRole(strings.TrimSpace(raw))
	if role == "" {
		return "", apperrors.NewValidationError("organization role is required")
	}
	if !role.IsValid() {
		return "", apperrors.NewValidationError("invalid organization role")
	}
	return role, nil
}

func isPrivilegedOrgTreeOperator(user *types.User) bool {
	return types.IsPlatformPrivilegedUser(user)
}

func callerHasTenantAdminOrOwner(ctx context.Context) bool {
	return types.TenantRoleFromContext(ctx).HasPermission(types.TenantRoleAdmin)
}

func isRootOrgNode(org *types.Organization) bool {
	if org == nil {
		return false
	}
	if org.ParentID == nil {
		return true
	}
	return strings.TrimSpace(*org.ParentID) == "" || org.Level <= 1
}

func displayRoleForOrgMembership(role types.OrgMemberRole) (string, string, bool, bool) {
	switch role {
	case types.OrgRoleAdmin:
		return orgTreeDisplayRoleSubOrgAdmin, orgTreeRoleScopeSubOrgAdmin, true, true
	case types.OrgRoleEditor, types.OrgRoleContributor:
		return orgTreeDisplayRoleCollaborator, orgTreeRoleScopeOrgEditor, false, false
	default:
		return orgTreeDisplayRoleReadonly, orgTreeRoleScopeOrgViewer, false, false
	}
}

func buildDirectOrgMemberEntry(member *types.OrganizationMember) gin.H {
	if member == nil || member.User == nil {
		return gin.H{}
	}
	displayRole, roleScope, canManagePersonnel, canManageResources := displayRoleForOrgMembership(member.Role)
	return gin.H{
		"user_id":                member.UserID,
		"username":               member.User.Username,
		"email":                  member.User.Email,
		"phone":                  member.User.Phone,
		"role":                   string(member.Role),
		"is_owner":               false,
		"is_admin":               member.Role == types.OrgRoleAdmin,
		"is_super_admin":         member.User.IsSuperAdmin,
		"is_system_admin":        member.User.IsSystemAdmin,
		"is_direct":              true,
		"joined_at":              member.CreatedAt,
		"display_role":           displayRole,
		"role_scope":             roleScope,
		"tenant_role":            "",
		"org_role":               string(member.Role),
		"source":                 orgTreeMemberSourceOrgDirect,
		"is_projected_root_admin": false,
		"can_manage_personnel":   canManagePersonnel,
		"can_manage_resources":   canManageResources,
	}
}

func buildProjectedRootMemberEntry(member *types.TenantMember, user *types.User) gin.H {
	username := ""
	email := ""
	phone := ""
	avatarIsSuperAdmin := false
	if user != nil {
		username = user.Username
		email = user.Email
		phone = user.Phone
		avatarIsSuperAdmin = user.IsSuperAdmin
	}
	displayRole := orgTreeDisplayRoleAdmin
	roleScope := orgTreeRoleScopeWorkspaceAdmin
	isOwner := false
	if member != nil && member.Role == types.TenantRoleOwner {
		displayRole = orgTreeDisplayRoleOwner
		roleScope = orgTreeRoleScopeWorkspaceOwner
		isOwner = true
	}
	role := ""
	joinedAt := member.JoinedAt
	userID := ""
	tenantRole := ""
	if member != nil {
		role = string(member.Role)
		userID = member.UserID
		tenantRole = string(member.Role)
	}
	return gin.H{
		"user_id":                userID,
		"username":               username,
		"email":                  email,
		"phone":                  phone,
		"role":                   role,
		"is_owner":               isOwner,
		"is_admin":               true,
		"is_super_admin":         avatarIsSuperAdmin,
		"is_system_admin":        user != nil && user.IsSystemAdmin,
		"is_direct":              true,
		"joined_at":              joinedAt,
		"display_role":           displayRole,
		"role_scope":             roleScope,
		"tenant_role":            tenantRole,
		"org_role":               "",
		"source":                 orgTreeMemberSourceTenantProjection,
		"is_projected_root_admin": true,
		"can_manage_personnel":   true,
		"can_manage_resources":   true,
	}
}

func buildRootOrgDirectMemberEntry(member *types.OrganizationMember, effectiveRole types.TenantRole) gin.H {
	if member == nil || member.User == nil {
		return gin.H{}
	}
	displayRole := orgTreeDisplayRoleReadonly
	roleScope := orgTreeRoleScopeOrgViewer
	canManagePersonnel := false
	canManageResources := false
	isOwner := false
	isAdmin := false
	switch effectiveRole {
	case types.TenantRoleOwner:
		displayRole = orgTreeDisplayRoleOwner
		roleScope = orgTreeRoleScopeWorkspaceOwner
		canManagePersonnel = true
		canManageResources = true
		isOwner = true
		isAdmin = true
	case types.TenantRoleAdmin:
		displayRole = orgTreeDisplayRoleAdmin
		roleScope = orgTreeRoleScopeWorkspaceAdmin
		canManagePersonnel = true
		canManageResources = true
		isAdmin = true
	case types.TenantRoleContributor:
		displayRole = orgTreeDisplayRoleCollaborator
		roleScope = orgTreeRoleScopeOrgEditor
	case types.TenantRoleViewer:
		displayRole = orgTreeDisplayRoleReadonly
		roleScope = orgTreeRoleScopeOrgViewer
	}
	return gin.H{
		"user_id":                 member.UserID,
		"username":                member.User.Username,
		"email":                   member.User.Email,
		"phone":                   member.User.Phone,
		"role":                    string(member.Role),
		"is_owner":                isOwner,
		"is_admin":                isAdmin,
		"is_super_admin":          member.User.IsSuperAdmin,
		"is_system_admin":         member.User.IsSystemAdmin,
		"is_direct":               true,
		"joined_at":               member.CreatedAt,
		"display_role":            displayRole,
		"role_scope":              roleScope,
		"tenant_role":             string(effectiveRole),
		"org_role":                string(member.Role),
		"source":                  orgTreeMemberSourceOrgDirect,
		"is_projected_root_admin": false,
		"can_manage_personnel":    canManagePersonnel,
		"can_manage_resources":    canManageResources,
	}
}

func decorateInheritedAdminEntries(entries []map[string]interface{}) []map[string]interface{} {
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		entry["display_role"] = orgTreeDisplayRoleSubOrgAdmin
		entry["role_scope"] = orgTreeRoleScopeInheritedAdmin
		entry["tenant_role"] = ""
		entry["org_role"] = string(types.OrgRoleAdmin)
		entry["source"] = orgTreeMemberSourceOrgInherited
		entry["is_projected_root_admin"] = false
		entry["can_manage_personnel"] = true
		entry["can_manage_resources"] = true
		entry["is_owner"] = false
		entry["is_admin"] = true
	}
	return entries
}

func (h *OrgTreeHandler) listProjectedRootAdminMembers(ctx context.Context, tenantID uint64) ([]gin.H, map[string]struct{}, error) {
	if h.memberService == nil {
		return nil, map[string]struct{}{}, nil
	}
	memberships, err := h.memberService.ListByTenant(ctx, tenantID)
	if err != nil {
		return nil, nil, err
	}
	projectedMemberships := make([]*types.TenantMember, 0)
	for _, membership := range memberships {
		if membership == nil {
			continue
		}
		effectiveRole := effectiveTenantRoleForUser(ctx, membership.Role, membership.UserID, tenantID, h.orgTreeService)
		if effectiveRole == types.TenantRoleOwner || effectiveRole == types.TenantRoleAdmin {
			membership.Role = effectiveRole
			projectedMemberships = append(projectedMemberships, membership)
		}
	}
	sort.SliceStable(projectedMemberships, func(i, j int) bool {
		left := projectedMemberships[i]
		right := projectedMemberships[j]
		leftOwner := left.Role == types.TenantRoleOwner
		rightOwner := right.Role == types.TenantRoleOwner
		if leftOwner != rightOwner {
			return leftOwner
		}
		if !left.JoinedAt.Equal(right.JoinedAt) {
			return left.JoinedAt.Before(right.JoinedAt)
		}
		return left.UserID < right.UserID
	})

	userIDs := make([]string, 0, len(projectedMemberships))
	for _, membership := range projectedMemberships {
		userIDs = append(userIDs, membership.UserID)
	}
	usersByID := map[string]*types.User{}
	if h.userService != nil && len(userIDs) > 0 {
		usersByID, err = h.userService.GetUsersByIDs(ctx, userIDs)
		if err != nil {
			return nil, nil, err
		}
	}

	projectedRows := make([]gin.H, 0, len(projectedMemberships))
	projectedUserIDs := make(map[string]struct{}, len(projectedMemberships))
	for _, membership := range projectedMemberships {
		if membership == nil {
			continue
		}
		if _, exists := projectedUserIDs[membership.UserID]; exists {
			continue
		}
		if user := usersByID[membership.UserID]; user != nil && user.IsSuperAdmin {
			continue
		}
		projectedUserIDs[membership.UserID] = struct{}{}
		projectedRows = append(projectedRows, buildProjectedRootMemberEntry(membership, usersByID[membership.UserID]))
	}
	return projectedRows, projectedUserIDs, nil
}

func canBootstrapRootOrg(user *types.User, currentRole types.TenantRole, orgCount int) bool {
	if isPrivilegedOrgTreeOperator(user) {
		return true
	}
	if orgCount != 0 {
		return false
	}
	return currentRole.HasPermission(types.TenantRoleAdmin)
}

func resolveTenantRoleForProvision(callerTenantRole types.TenantRole, req *types.CreateUserInOrgRequest) (types.TenantRole, error) {
	requested := types.TenantRole(strings.TrimSpace(req.TenantRole))
	if requested == "" {
		requested = types.TenantRoleViewer
	}
	if err := authorizeTenantRoleProvision(callerTenantRole, requested); err != nil {
		return "", err
	}
	return requested, nil
}

func resolveTenantRoleForOrgScopedProvision(
	callerTenantRole types.TenantRole,
	allowBranchScopedProvision bool,
	req *types.CreateUserInOrgRequest,
) (types.TenantRole, error) {
	orgRole, err := normalizeOrgRole(req.Role)
	if err != nil {
		return "", err
	}
	if allowBranchScopedProvision && !callerTenantRole.HasPermission(types.TenantRoleAdmin) {
		switch orgRole {
		case types.OrgRoleViewer:
			return types.TenantRoleViewer, nil
		default:
			return types.TenantRoleContributor, nil
		}
	}
	var mappedRole types.TenantRole
	switch orgRole {
	case types.OrgRoleAdmin:
		mappedRole = types.TenantRoleAdmin
	case types.OrgRoleEditor, types.OrgRoleContributor:
		mappedRole = types.TenantRoleContributor
	default:
		mappedRole = types.TenantRoleViewer
	}
	return mappedRole, nil
}

func resolveTenantRoleForCreateUser(
	callerTenantRole types.TenantRole,
	callerIsPrivileged bool,
	req *types.CreateUserInOrgRequest,
) (types.TenantRole, error) {
	if callerIsPrivileged || callerTenantRole.HasPermission(types.TenantRoleAdmin) {
		authorizingRole := callerTenantRole
		if callerIsPrivileged && !authorizingRole.HasPermission(types.TenantRoleOwner) {
			authorizingRole = types.TenantRoleOwner
		}
		return resolveTenantRoleForProvision(authorizingRole, req)
	}
	return resolveTenantRoleForOrgScopedProvision(callerTenantRole, true, req)
}

func selectProvisionUserCandidate(matches ...*types.User) *types.User {
	var candidate *types.User
	for _, match := range matches {
		if match == nil {
			continue
		}
		if candidate == nil {
			candidate = match
			continue
		}
		if candidate.ID != match.ID {
			return nil
		}
	}
	return candidate
}

func countProvisionUserMatches(candidate *types.User, matches ...*types.User) int {
	if candidate == nil {
		return 0
	}
	count := 0
	for _, match := range matches {
		if match != nil && match.ID == candidate.ID {
			count++
		}
	}
	return count
}

func isReusableProvisionedUser(candidate *types.User, tenantID uint64, orgCount int, matchedCount int) bool {
	if candidate == nil {
		return false
	}
	if candidate.TenantID != tenantID || isPrivilegedOrgTreeOperator(candidate) || orgCount != 0 {
		return false
	}
	return matchedCount >= 2
}

func (h *OrgTreeHandler) findReusableProvisionedUser(
	ctx context.Context,
	tenantID uint64,
	req *types.CreateUserInOrgRequest,
) (*types.User, error) {
	usernameMatch, _ := h.userService.GetUserByUsername(ctx, req.Username)
	var emailMatch *types.User
	if req.Email != "" {
		emailMatch, _ = h.userService.GetUserByEmail(ctx, req.Email)
	}
	var phoneMatch *types.User
	if req.Phone != "" {
		phoneMatch, _ = h.userService.GetUserByPhone(ctx, req.Phone)
	}

	candidate := selectProvisionUserCandidate(usernameMatch, emailMatch, phoneMatch)
	if candidate == nil {
		return nil, nil
	}

	orgs, err := h.orgTreeService.GetUserOrganizations(ctx, candidate.ID, tenantID)
	if err != nil {
		return nil, err
	}
	matchedCount := countProvisionUserMatches(candidate, usernameMatch, emailMatch, phoneMatch)
	if !isReusableProvisionedUser(candidate, tenantID, len(orgs), matchedCount) {
		return nil, nil
	}
	return candidate, nil
}

func (h *OrgTreeHandler) provisionUserForOrg(
	ctx context.Context,
	tenantID uint64,
	req *types.CreateUserInOrgRequest,
) (*types.User, bool, error) {
	reusableUser, err := h.findReusableProvisionedUser(ctx, tenantID, req)
	if err != nil {
		return nil, false, err
	}
	if reusableUser == nil {
		user, err := h.userService.CreateUserByAdmin(ctx, req, tenantID)
		if err != nil {
			return nil, false, err
		}
		return user, true, nil
	}

	logger.Infof(ctx, "Reusing orphaned org-tree user %s during provisioning", reusableUser.ID)
	reusableUser.Username = req.Username
	reusableUser.Email = req.Email
	reusableUser.Phone = req.Phone
	reusableUser.IsActive = true
	if err := h.userService.UpdateUser(ctx, reusableUser); err != nil {
		return nil, false, err
	}
	if err := h.userService.AdminSetPassword(ctx, reusableUser.ID, req.Password); err != nil {
		return nil, false, err
	}
	return reusableUser, false, nil
}

func (h *OrgTreeHandler) ensureProvisionedMembership(
	ctx context.Context,
	userID string,
	tenantID uint64,
	role types.TenantRole,
) error {
	currentMember, err := h.memberService.GetMembership(ctx, userID, tenantID)
	if err != nil {
		return err
	}
	if currentMember == nil {
		_, err := h.memberService.AddMember(ctx, userID, tenantID, role, nil)
		return err
	}
	if currentMember.Role == role {
		return nil
	}
	return h.memberService.UpdateRole(ctx, userID, tenantID, role)
}

// getUserFromContext extracts the current user from the Gin context.
// Returns the user and true on success, or sends an error response and returns nil/false.
func (h *OrgTreeHandler) getUserFromContext(c *gin.Context) (*types.User, bool) {
	userVal, exists := c.Get(types.UserContextKey.String())
	if !exists {
		c.Error(apperrors.NewUnauthorizedError("User not found in context"))
		return nil, false
	}
	user, ok := userVal.(*types.User)
	if !ok {
		c.Error(apperrors.NewInternalServerError("Invalid user type in context"))
		return nil, false
	}
	return user, true
}

// isOrgAdminOf checks if the given user is an admin of the specified organization
// or any of its ancestor organizations (permission inheritance via materialized path).
// Super admins are always considered admin of any org.
func (h *OrgTreeHandler) isOrgAdminOf(c *gin.Context, user *types.User, orgID string, tenantID uint64) bool {
	if isPrivilegedOrgTreeOperator(user) || callerHasTenantAdminOrOwner(c.Request.Context()) {
		return true
	}
	ctx := c.Request.Context()

	// Step 1: Check if user is a direct admin of this org (fast path)
	orgMembers, err := h.orgTreeService.ListOrgMembers(ctx, orgID, tenantID)
	if err == nil {
		for _, member := range orgMembers {
			if member.UserID == user.ID && member.Role == types.OrgRoleAdmin {
				return true
			}
		}
	}

	// Step 2: Get the org node to read its path, then check ancestor admin
	org, err := h.orgTreeService.GetNode(ctx, orgID, tenantID)
	if err != nil {
		return false
	}

	ancestorIDs := parseAncestorIDs(org.Path, orgID)
	if len(ancestorIDs) == 0 {
		return false // root node, no ancestors to check
	}

	// Step 3: Batch check if user is admin of any ancestor org (single SQL)
	return h.orgTreeService.IsAdminOfAnyOrg(ctx, user.ID, ancestorIDs, tenantID)
}

// parseAncestorIDs extracts ancestor org IDs from a materialized path, excluding the node itself.
// Path format: /root_id/parent_id/self_id
func parseAncestorIDs(path string, selfID string) []string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	ancestors := make([]string, 0, len(parts)-1)
	for _, part := range parts {
		if part != "" && part != selfID {
			ancestors = append(ancestors, part)
		}
	}
	return ancestors
}

// isOrgAdminOfAny checks if the user is a super admin or admin of any org-tree organization.
func (h *OrgTreeHandler) isOrgAdminOfAny(c *gin.Context, user *types.User, tenantID uint64) bool {
	if isPrivilegedOrgTreeOperator(user) || callerHasTenantAdminOrOwner(c.Request.Context()) {
		return true
	}
	ctx := c.Request.Context()
	userOrgs, err := h.orgTreeService.GetUserOrganizations(ctx, user.ID, tenantID)
	if err != nil {
		return false
	}
	for _, org := range userOrgs {
		if org.MyIsAdmin {
			return true
		}
	}
	return false
}

// GetOrgTree returns the organization tree for the current user
// Super admins see the full tree; org admins see only their managed subtrees
// @Summary      获取组织树
// @Description  返回当前用户可见的组织树（超管看全部，组织管理员看自己管理的子树）
// @Tags         组织树管理
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  errors.AppError
// @Security     Bearer
// @Router       /org-tree [get]
func (h *OrgTreeHandler) GetOrgTree(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())

	user, ok := h.getUserFromContext(c)
	if !ok {
		return
	}
	callerTenantRole := types.TenantRoleFromContext(ctx)

	// Permission gate: privileged operators see the full tree; org admins see
	// their managed subtrees. Tenant admins/owners bootstrapping a brand-new
	// workspace may read an empty tree so they can create the first root org.
	if !h.isOrgAdminOfAny(c, user, tenantID) {
		tree, err := h.orgTreeService.GetTree(ctx, tenantID)
		if err != nil {
			logger.Errorf(ctx, "Failed to get org tree during bootstrap check: %v", err)
			c.Error(apperrors.NewInternalServerError("Failed to get organization tree").WithDetails(err.Error()))
			return
		}
		if !canBootstrapRootOrg(user, callerTenantRole, len(tree)) {
			c.Error(apperrors.NewForbiddenError("You do not have permission to access the organization tree"))
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    tree,
		})
		return
	}

	tree, err := h.orgTreeService.GetTreeForUser(ctx, user.ID, tenantID, isPrivilegedOrgTreeOperator(user))
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
	callerTenantRole := types.TenantRoleFromContext(ctx)

	var req types.CreateOrgTreeNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Errorf(ctx, "Invalid request parameters: %v", err)
		c.Error(apperrors.NewValidationError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	user, ok := h.getUserFromContext(c)
	if !ok {
		return
	}

	// Privileged operators can create any root or child org. Tenant admins and
	// owners may bootstrap exactly the first root org in an empty workspace.
	if !isPrivilegedOrgTreeOperator(user) {
		// 不允许创建根组织（parent_id为空或空字符串）
		if req.ParentID == nil || *req.ParentID == "" {
			tree, err := h.orgTreeService.GetTree(ctx, tenantID)
			if err != nil {
				logger.Errorf(ctx, "Failed to get org tree during bootstrap check: %v", err)
				c.Error(apperrors.NewInternalServerError("Failed to create organization tree node").WithDetails(err.Error()))
				return
			}
			if !canBootstrapRootOrg(user, callerTenantRole, len(tree)) {
				logger.Warnf(ctx, "User %s attempted to create root organization without bootstrap permission", userID)
				c.Error(apperrors.NewForbiddenError("Only super administrators or tenant admins bootstrapping an empty organization tree can create root organizations"))
				return
			}
		}

		// 检查用户是否是父组织的管理员
		if !h.isOrgAdminOf(c, user, *req.ParentID, tenantID) {
			logger.Warnf(ctx, "User %s is not an admin of parent organization %s", userID, *req.ParentID)
			c.Error(apperrors.NewForbiddenError("You must be an administrator of the parent organization to create sub-organizations"))
			return
		}
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

	user, ok := h.getUserFromContext(c)
	if !ok {
		return
	}

	// Permission: super admin or admin of this org (or an ancestor)
	if !h.isOrgAdminOf(c, user, nodeID, tenantID) {
		c.Error(apperrors.NewForbiddenError("You do not have permission to view this organization node"))
		return
	}

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

	user, ok := h.getUserFromContext(c)
	if !ok {
		return
	}

	// Permission: super admin or admin of this org
	if !h.isOrgAdminOf(c, user, nodeID, tenantID) {
		c.Error(apperrors.NewForbiddenError("You do not have permission to update this organization node"))
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

	user, ok := h.getUserFromContext(c)
	if !ok {
		return
	}

	// Permission: super admin or admin of this org
	if !h.isOrgAdminOf(c, user, nodeID, tenantID) {
		c.Error(apperrors.NewForbiddenError("You do not have permission to delete this organization node"))
		return
	}

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

	user, ok := h.getUserFromContext(c)
	if !ok {
		return
	}

	// Permission: must be admin of the node being moved
	if !h.isOrgAdminOf(c, user, nodeID, tenantID) {
		c.Error(apperrors.NewForbiddenError("You do not have permission to move this organization node"))
		return
	}

	// If moving to a new parent, must also be admin of the target parent
	if req.NewParentID != nil && *req.NewParentID != "" {
		if !h.isOrgAdminOf(c, user, *req.NewParentID, tenantID) {
			c.Error(apperrors.NewForbiddenError("You do not have permission to move nodes into the target organization"))
			return
		}
	} else if !user.IsSuperAdmin {
		// Moving to root level requires super admin
		c.Error(apperrors.NewForbiddenError("Only super administrators can move nodes to root level"))
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

	user, ok := h.getUserFromContext(c)
	if !ok {
		return
	}

	// Permission: must be admin of this org
	if !h.isOrgAdminOf(c, user, orgID, tenantID) {
		logger.Warnf(ctx, "User %s is not an admin of organization %s", user.ID, orgID)
		c.Error(apperrors.NewForbiddenError("Only organization administrators can add members to this organization"))
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

	user, ok := h.getUserFromContext(c)
	if !ok {
		return
	}
	callerTenantRole := types.TenantRoleFromContext(ctx)

	// Permission: must be admin of this org
	if !h.isOrgAdminOf(c, user, orgID, tenantID) {
		logger.Warnf(ctx, "User %s is not an admin of organization %s", user.ID, orgID)
		c.Error(apperrors.NewForbiddenError("Only organization administrators can create users in this organization"))
		return
	}

	orgRole, err := normalizeOrgRole(req.Role)
	if err != nil {
		c.Error(err)
		return
	}
	tenantRole, err := resolveTenantRoleForCreateUser(callerTenantRole, isPrivilegedOrgTreeOperator(user), &req)
	if err != nil {
		c.Error(err)
		return
	}
	if h.memberService == nil {
		c.Error(apperrors.NewInternalServerError("tenant member service unavailable"))
		return
	}

	// Step 1: Create user via user service
	newUser, createdNewUser, err := h.provisionUserForOrg(ctx, tenantID, &req)
	if err != nil {
		logger.Errorf(ctx, "Failed to create user: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to create user").WithDetails(err.Error()))
		return
	}

	// Step 2: Bootstrap tenant membership so the new user can actually log into
	// this workspace. Org-tree membership alone is not enough for RBAC.
	if err := h.ensureProvisionedMembership(ctx, newUser.ID, tenantID, tenantRole); err != nil {
		logger.Errorf(ctx, "User created but failed to create tenant membership: %v", err)
		if createdNewUser {
			_ = h.userService.DeleteUser(ctx, newUser.ID)
		}
		c.Error(apperrors.NewInternalServerError("Failed to create tenant membership").WithDetails(err.Error()))
		return
	}

	// Step 3: Assign the newly created user to org
	assignReq := &types.AssignUserToOrgRequest{
		UserID: newUser.ID,
		Role:   orgRole,
	}
	if err := h.orgTreeService.AssignUserToOrg(ctx, orgID, tenantID, assignReq); err != nil {
		logger.Errorf(ctx, "User created but failed to assign to org: %v", err)
		message := "User created and added to workspace, but failed to assign to organization: " + err.Error()
		if !createdNewUser {
			message = "User restored and added to workspace, but failed to assign to organization: " + err.Error()
		}
		// User is created but assignment failed — return partial success
		c.JSON(http.StatusOK, types.CreateUserInOrgResponse{
			Success: true,
			Message: message,
			User:    newUser.ToUserInfo(),
		})
		return
	}

	message := "User created, added to workspace, and assigned to organization successfully"
	if !createdNewUser {
		message = "User restored, added to workspace, and assigned to organization successfully"
	}

	c.JSON(http.StatusOK, types.CreateUserInOrgResponse{
		Success: true,
		Message: message,
		User:    newUser.ToUserInfo(),
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

	currentUser, ok := h.getUserFromContext(c)
	if !ok {
		return
	}

	// Permission: must be admin of this org
	if !h.isOrgAdminOf(c, currentUser, orgID, tenantID) {
		c.Error(apperrors.NewForbiddenError("You do not have permission to update users in this organization"))
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
			logger.Errorf(ctx, "User updated but failed to update role in org: %v", err)
			c.Error(apperrors.NewInternalServerError("Failed to update user role in organization").WithDetails(err.Error()))
			return
		}
	}

	c.JSON(http.StatusOK, types.CreateUserInOrgResponse{
		Success: true,
		Message: "User updated successfully",
		User:    user.ToUserInfo(),
	})
}

// UpdateUserPasswordInOrg resets a direct member's login password in the specified organization.
// @Summary      重置组织内用户登录密码
// @Description  超级管理员为当前组织内直属成员直接重置登录密码，无需校验自己的当前密码
// @Tags         OrgTree
// @Accept       json
// @Produce      json
// @Param        id       path      string                                   true   "组织节点ID"
// @Param        user_id  path      string                                   true   "用户ID"
// @Param        request  body      types.UpdateUserPasswordInOrgRequest     true   "重置登录密码请求"
// @Success      200      {object}  map[string]interface{}
// @Failure      400      {object}  errors.AppError
// @Failure      403      {object}  errors.AppError
// @Failure      404      {object}  errors.AppError
// @Security     Bearer
// @Router       /org-tree/{id}/users/{user_id}/password [put]
func (h *OrgTreeHandler) UpdateUserPasswordInOrg(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	orgID := c.Param("id")
	userID := c.Param("user_id")

	var req types.UpdateUserPasswordInOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Errorf(ctx, "Invalid request parameters: %v", err)
		c.Error(apperrors.NewValidationError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	currentUser, ok := h.getUserFromContext(c)
	if !ok {
		return
	}

	if req.NewPassword != req.ConfirmPassword {
		c.Error(apperrors.NewValidationError("New password and confirm password do not match"))
		return
	}

	if !currentUser.IsSuperAdmin {
		c.Error(apperrors.NewForbiddenError("Only super admins can reset organization user passwords"))
		return
	}

	if !h.isOrgAdminOf(c, currentUser, orgID, tenantID) {
		c.Error(apperrors.NewForbiddenError("You do not have permission to update passwords in this organization"))
		return
	}

	user, err := h.userService.GetUserByID(ctx, userID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get user: %v", err)
		c.Error(apperrors.NewNotFoundError("User not found").WithDetails(err.Error()))
		return
	}

	if user.TenantID != tenantID {
		c.Error(apperrors.NewForbiddenError("User does not belong to your tenant"))
		return
	}

	members, err := h.orgTreeService.ListOrgMembers(ctx, orgID, tenantID)
	if err != nil {
		logger.Errorf(ctx, "Failed to list org members: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to verify organization members").WithDetails(err.Error()))
		return
	}

	isDirectMember := false
	for _, member := range members {
		if member.UserID == userID {
			isDirectMember = true
			break
		}
	}
	if !isDirectMember {
		c.Error(apperrors.NewForbiddenError("Target user is not a direct member of this organization"))
		return
	}

	if err := h.userService.AdminSetPassword(ctx, userID, req.NewPassword); err != nil {
		logger.Errorf(ctx, "Failed to update user password: %v", err)
		c.Error(apperrors.NewBadRequestError("Failed to update user password").WithDetails(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User password updated successfully",
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

	currentUser, ok := h.getUserFromContext(c)
	if !ok {
		return
	}

	// Permission: must be admin of this org
	if !h.isOrgAdminOf(c, currentUser, orgID, tenantID) {
		logger.Warnf(ctx, "User %s is not an admin of organization %s", currentUser.ID, orgID)
		c.Error(apperrors.NewForbiddenError("Only organization administrators can remove members from this organization"))
		return
	}

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

	currentUser, ok := h.getUserFromContext(c)
	if !ok {
		return
	}

	// Permission: must be admin of this org
	if !h.isOrgAdminOf(c, currentUser, orgID, tenantID) {
		logger.Warnf(ctx, "User %s is not an admin of organization %s", currentUser.ID, orgID)
		c.Error(apperrors.NewForbiddenError("Only organization administrators can manage admin roles in this organization"))
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

	currentUser, ok := h.getUserFromContext(c)
	if !ok {
		return
	}

	// Permission: super admin or admin of this org (including ancestor inheritance)
	if !h.isOrgAdminOf(c, currentUser, orgID, tenantID) {
		c.Error(apperrors.NewForbiddenError("You do not have permission to view members of this organization"))
		return
	}

	members, err := h.orgTreeService.ListOrgMembers(ctx, orgID, tenantID)
	if err != nil {
		logger.Errorf(ctx, "Failed to list org members: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to list organization members").WithDetails(err.Error()))
		return
	}
	orgNode, err := h.orgTreeService.GetNode(ctx, orgID, tenantID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get org node for member list: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to get organization node").WithDetails(err.Error()))
		return
	}

	projectedRootAdmins := make([]gin.H, 0)
	projectedRootAdminUserIDs := make(map[string]struct{})
	effectiveTenantRoles := make(map[string]types.TenantRole)
	if h.memberService != nil {
		memberships, listErr := h.memberService.ListByTenant(ctx, tenantID)
		if listErr == nil {
			for _, membership := range memberships {
				if membership == nil {
					continue
				}
				effectiveTenantRoles[membership.UserID] = effectiveTenantRoleForUser(ctx, membership.Role, membership.UserID, tenantID, h.orgTreeService)
			}
		} else {
			logger.Warnf(ctx, "Failed to precompute effective tenant roles for org members: %v", listErr)
		}
	}
	if isRootOrgNode(orgNode) {
		projectedRootAdmins, projectedRootAdminUserIDs, err = h.listProjectedRootAdminMembers(ctx, tenantID)
		if err != nil {
			logger.Errorf(ctx, "Failed to project root admins for org %s: %v", orgID, err)
			c.Error(apperrors.NewInternalServerError("Failed to build projected root admins").WithDetails(err.Error()))
			return
		}
	}

	// Classify direct members into admins and regular members
	directAdmins := make([]gin.H, 0, len(projectedRootAdmins))
	directAdmins = append(directAdmins, projectedRootAdmins...)
	directMembers := make([]gin.H, 0)
	for _, m := range members {
		if m.User == nil {
			continue
		}
		if _, projected := projectedRootAdminUserIDs[m.UserID]; projected {
			continue
		}
		entry := buildDirectOrgMemberEntry(m)
		if isRootOrgNode(orgNode) {
			effectiveRole, ok := effectiveTenantRoles[m.UserID]
			if !ok || effectiveRole == "" {
				effectiveRole = types.TenantRoleViewer
			}
			entry = buildRootOrgDirectMemberEntry(m, effectiveRole)
		}
		if m.Role == types.OrgRoleAdmin {
			directAdmins = append(directAdmins, entry)
		} else {
			directMembers = append(directMembers, entry)
		}
	}

	// Get inherited admins from ancestor orgs
	inheritedAdmins, err := h.orgTreeService.ListInheritedAdmins(ctx, orgID, tenantID)
	if err != nil {
		logger.Warnf(ctx, "Failed to list inherited admins for org %s: %v", orgID, err)
		inheritedAdmins = nil
	}
	inheritedAdmins = decorateInheritedAdminEntries(inheritedAdmins)

	// Build backward-compatible flat list (direct members only, as before)
	flatResult := make([]gin.H, 0, len(members)+len(projectedRootAdmins))
	flatResult = append(flatResult, projectedRootAdmins...)
	for _, m := range members {
		if m.User == nil {
			continue
		}
		if _, projected := projectedRootAdminUserIDs[m.UserID]; projected {
			continue
		}
		entry := buildDirectOrgMemberEntry(m)
		if isRootOrgNode(orgNode) {
			effectiveRole, ok := effectiveTenantRoles[m.UserID]
			if !ok || effectiveRole == "" {
				effectiveRole = types.TenantRoleViewer
			}
			entry = buildRootOrgDirectMemberEntry(m, effectiveRole)
		}
		flatResult = append(flatResult, entry)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"data":             flatResult, // backward compatible
		"direct_admins":    directAdmins,
		"direct_members":   directMembers,
		"inherited_admins": inheritedAdmins,
		"total_direct":     len(flatResult),
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

	currentUser, ok := h.getUserFromContext(c)
	if !ok {
		return
	}

	// Permission: must be super admin or org admin of at least one org
	if !h.isOrgAdminOfAny(c, currentUser, tenantID) {
		c.Error(apperrors.NewForbiddenError("You do not have permission to search users"))
		return
	}

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
