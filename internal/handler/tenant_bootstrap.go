package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
)

func hashWorkspaceBootstrapRequest(req *types.WorkspaceBootstrapRequest) (string, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func marshalWorkspaceBootstrapRequest(req *types.WorkspaceBootstrapRequest) (types.JSON, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	return types.JSON(payload), nil
}

func marshalWorkspaceBootstrapResult(result *types.WorkspaceBootstrapResult) (types.JSON, error) {
	payload, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return types.JSON(payload), nil
}

func loadWorkspaceBootstrapResult(op *types.TenantBootstrapOperation) (*types.WorkspaceBootstrapResult, error) {
	if op == nil || len(op.ResultPayload) == 0 {
		return nil, nil
	}
	var result types.WorkspaceBootstrapResult
	if err := json.Unmarshal(op.ResultPayload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func makeBootstrapResult(req *types.WorkspaceBootstrapRequest) *types.WorkspaceBootstrapResult {
	return &types.WorkspaceBootstrapResult{
		IdempotencyKey: req.IdempotencyKey,
		Status:         types.WorkspaceBootstrapStatusInProgress,
		Mode:           req.Mode,
		Steps: types.WorkspaceBootstrapSteps{
			RootOrg:          types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepPending},
			UserProvision:    types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepSkipped},
			TenantMembership: types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepSkipped},
			OrgAssignment:    types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepSkipped},
			InviteLink:       types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepSkipped},
		},
	}
}

func trimWorkspaceBootstrapRequest(req *types.WorkspaceBootstrapRequest) {
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.TenantRole = strings.TrimSpace(req.TenantRole)
	req.RootOrgID = strings.TrimSpace(req.RootOrgID)
	req.RootOrgName = strings.TrimSpace(req.RootOrgName)
	req.RootOrgDescription = strings.TrimSpace(req.RootOrgDescription)
	req.OrgRole = strings.TrimSpace(req.OrgRole)
	req.ExistingEmail = strings.TrimSpace(req.ExistingEmail)
	req.InviteMessage = strings.TrimSpace(req.InviteMessage)
	req.NewUsername = strings.TrimSpace(req.NewUsername)
	req.NewEmail = strings.TrimSpace(req.NewEmail)
	req.NewPhone = strings.TrimSpace(req.NewPhone)
}

func validateWorkspaceBootstrapRequest(req *types.WorkspaceBootstrapRequest) error {
	if !req.Mode.IsValid() {
		return apperrors.NewValidationError("mode must be one of existing/new/invite")
	}
	role := types.TenantRole(req.TenantRole)
	if !role.IsValid() {
		return apperrors.NewValidationError("tenant_role must be one of owner/admin/contributor/viewer")
	}
	if req.Mode == types.WorkspaceBootstrapModeNew && !req.ShouldSetupOrg {
		return apperrors.NewValidationError("new-user bootstrap requires an organization node")
	}
	if req.ShouldSetupOrg && req.RootOrgID == "" && req.RootOrgName == "" {
		return apperrors.NewValidationError("root_org_name is required when creating a root organization")
	}
	switch req.Mode {
	case types.WorkspaceBootstrapModeExisting:
		if req.ExistingEmail == "" {
			return apperrors.NewValidationError("existing_email is required")
		}
		if req.ShouldSetupOrg && req.OrgRole == "" {
			return apperrors.NewValidationError("org_role is required when assigning an existing user to an organization")
		}
	case types.WorkspaceBootstrapModeNew:
		if req.NewUsername == "" || req.NewPassword == "" {
			return apperrors.NewValidationError("new_username and new_password are required")
		}
		if req.NewEmail == "" && req.NewPhone == "" {
			return apperrors.NewValidationError("at least one of new_email or new_phone is required")
		}
		if req.OrgRole == "" {
			return apperrors.NewValidationError("org_role is required in new-user mode")
		}
	case types.WorkspaceBootstrapModeInvite:
	}
	return nil
}

func (h *TenantHandler) saveBootstrapProgress(ctx context.Context, op *types.TenantBootstrapOperation, result *types.WorkspaceBootstrapResult) error {
	if h.bootstrapService == nil || op == nil {
		return nil
	}
	payload, err := marshalWorkspaceBootstrapResult(result)
	if err != nil {
		return err
	}
	op.ResultPayload = payload
	return h.bootstrapService.Save(ctx, op)
}

func (h *TenantHandler) persistBootstrapRequest(ctx context.Context, op *types.TenantBootstrapOperation, req *types.WorkspaceBootstrapRequest, result *types.WorkspaceBootstrapResult) error {
	requestPayload, err := marshalWorkspaceBootstrapRequest(req)
	if err != nil {
		return err
	}
	hash, err := hashWorkspaceBootstrapRequest(req)
	if err != nil {
		return err
	}
	op.Mode = req.Mode
	op.Status = types.WorkspaceBootstrapStatusInProgress
	op.RequestHash = hash
	op.RequestPayload = requestPayload
	return h.saveBootstrapProgress(ctx, op, result)
}

func (h *TenantHandler) finalizeBootstrapResult(
	ctx context.Context,
	op *types.TenantBootstrapOperation,
	result *types.WorkspaceBootstrapResult,
	status types.WorkspaceBootstrapStatus,
) error {
	result.Status = status
	if op != nil {
		op.Status = status
	}
	return h.saveBootstrapProgress(ctx, op, result)
}

func (h *TenantHandler) resolveBootstrapRootOrg(
	ctx context.Context,
	tenantID uint64,
	callerUserID string,
	op *types.TenantBootstrapOperation,
	req *types.WorkspaceBootstrapRequest,
	result *types.WorkspaceBootstrapResult,
) (*types.WorkspaceBootstrapRootOrg, error) {
	if !req.ShouldSetupOrg {
		result.Steps.RootOrg = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepSkipped}
		return nil, nil
	}

	tryExistingID := strings.TrimSpace(req.RootOrgID)
	if tryExistingID == "" && op != nil {
		tryExistingID = strings.TrimSpace(op.RootOrgID)
	}
	if tryExistingID != "" {
		org, err := h.orgTreeService.GetNode(ctx, tryExistingID, tenantID)
		if err == nil && org != nil && bootstrapOrgIsRoot(org) {
			root := &types.WorkspaceBootstrapRootOrg{ID: org.ID, Name: org.Name, Reused: true}
			op.RootOrgID = org.ID
			op.RootOrgName = org.Name
			result.RootOrg = root
			result.Steps.RootOrg = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepReused}
			if err := h.saveBootstrapProgress(ctx, op, result); err != nil {
				return nil, err
			}
			return root, nil
		}
	}

	tree, err := h.orgTreeService.GetTree(ctx, tenantID)
	if err != nil {
		return nil, apperrors.NewInternalServerError("Failed to inspect organization tree").WithDetails(err.Error())
	}
	reusable := selectReusableRootOrgCandidate(tree, req.RootOrgName, op.RootOrgName)
	if reusable != nil {
		root := &types.WorkspaceBootstrapRootOrg{ID: reusable.ID, Name: reusable.Name, Reused: true}
		op.RootOrgID = reusable.ID
		op.RootOrgName = reusable.Name
		result.RootOrg = root
		result.Steps.RootOrg = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepReused}
		if err := h.saveBootstrapProgress(ctx, op, result); err != nil {
			return nil, err
		}
		return root, nil
	}

	org, err := h.orgTreeService.CreateNode(ctx, tenantID, callerUserID, &types.CreateOrgTreeNodeRequest{
		Name:        req.RootOrgName,
		Description: req.RootOrgDescription,
	})
	if err != nil {
		return nil, apperrors.NewInternalServerError("Failed to create organization tree node").WithDetails(err.Error())
	}
	root := &types.WorkspaceBootstrapRootOrg{ID: org.ID, Name: org.Name, Reused: false}
	op.RootOrgID = org.ID
	op.RootOrgName = org.Name
	result.RootOrg = root
	result.Steps.RootOrg = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepCompleted}
	if err := h.saveBootstrapProgress(ctx, op, result); err != nil {
		return nil, err
	}
	return root, nil
}

func (h *TenantHandler) ensureExistingBootstrapMember(
	ctx context.Context,
	tenantID uint64,
	role types.TenantRole,
	op *types.TenantBootstrapOperation,
	req *types.WorkspaceBootstrapRequest,
	result *types.WorkspaceBootstrapResult,
) (*types.User, error) {
	user, err := h.userService.GetUserByEmail(ctx, req.ExistingEmail)
	if err != nil {
		return nil, apperrors.NewInternalServerError("Failed to look up existing user").WithDetails(err.Error())
	}
	if user == nil {
		return nil, apperrors.NewNotFoundError("Existing user not found").WithDetails("existing_email")
	}
	op.UserID = user.ID
	result.User = user.ToUserInfo()
	result.Steps.UserProvision = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepReused}

	currentMembership, err := h.memberService.GetMembership(ctx, user.ID, tenantID)
	if err != nil {
		return nil, apperrors.NewInternalServerError("Failed to get tenant membership").WithDetails(err.Error())
	}
	switch {
	case currentMembership == nil:
		if _, err := h.memberService.AddMember(ctx, user.ID, tenantID, role, nil); err != nil {
			if errors.Is(err, service.ErrMembershipAlreadyExists) {
				result.Steps.TenantMembership = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepReused}
				break
			}
			return nil, apperrors.NewInternalServerError("Failed to create tenant membership").WithDetails(err.Error())
		}
		result.Steps.TenantMembership = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepCompleted}
	case currentMembership.Role == role:
		result.Steps.TenantMembership = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepReused}
	default:
		if err := h.memberService.UpdateRole(ctx, user.ID, tenantID, role); err != nil {
			return nil, apperrors.NewInternalServerError("Failed to sync workspace role").WithDetails(err.Error())
		}
		result.Steps.TenantMembership = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepCompleted}
	}
	op.TenantMembershipDone = true
	if err := h.saveBootstrapProgress(ctx, op, result); err != nil {
		return nil, err
	}
	return user, nil
}

func (h *TenantHandler) ensureBootstrapNewUser(
	ctx context.Context,
	tenantID uint64,
	role types.TenantRole,
	op *types.TenantBootstrapOperation,
	req *types.WorkspaceBootstrapRequest,
	result *types.WorkspaceBootstrapResult,
) (*types.User, error) {
	createReq := &types.CreateUserInOrgRequest{
		Username:   req.NewUsername,
		Email:      req.NewEmail,
		Phone:      req.NewPhone,
		Password:   req.NewPassword,
		Role:       req.OrgRole,
		TenantRole: string(role),
	}

	if op.UserID != "" {
		existingUser, err := h.userService.GetUserByID(ctx, op.UserID)
		if err == nil && existingUser != nil {
			if err := validateUserIdentityUniqueness(ctx, h.userService, existingUser.ID, createReq.Username, createReq.Email, createReq.Phone); err != nil {
				return nil, translateUserProvisionError(err)
			}
			existingUser.Username = createReq.Username
			existingUser.Email = createReq.Email
			existingUser.Phone = createReq.Phone
			existingUser.IsActive = true
			if err := h.userService.UpdateUser(ctx, existingUser); err != nil {
				return nil, apperrors.NewInternalServerError("Failed to update user").WithDetails(err.Error())
			}
			if err := h.userService.AdminSetPassword(ctx, existingUser.ID, createReq.Password); err != nil {
				return nil, apperrors.NewInternalServerError("Failed to update user password").WithDetails(err.Error())
			}
			result.User = existingUser.ToUserInfo()
			result.Steps.UserProvision = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepReused}
			changed, err := ensureProvisionedMembership(ctx, h.memberService, existingUser.ID, tenantID, role)
			if err != nil {
				return nil, apperrors.NewInternalServerError("Failed to create tenant membership").WithDetails(err.Error())
			}
			op.TenantMembershipDone = true
			result.Steps.TenantMembership = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepReused}
			if changed {
				result.Steps.TenantMembership.Status = types.WorkspaceBootstrapStepCompleted
			}
			if err := h.saveBootstrapProgress(ctx, op, result); err != nil {
				return nil, err
			}
			return existingUser, nil
		}
		op.UserID = ""
	}

	user, createdNewUser, err := provisionUserForOrg(ctx, h.orgTreeService, h.userService, tenantID, createReq)
	if err != nil {
		return nil, translateUserProvisionError(err)
	}
	op.UserID = user.ID
	result.User = user.ToUserInfo()
	result.Steps.UserProvision = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepReused}
	if createdNewUser {
		result.Steps.UserProvision.Status = types.WorkspaceBootstrapStepCompleted
	}
	if err := h.saveBootstrapProgress(ctx, op, result); err != nil {
		return nil, err
	}

	changed, err := ensureProvisionedMembership(ctx, h.memberService, user.ID, tenantID, role)
	if err != nil {
		if createdNewUser {
			_ = h.userService.DeleteUser(ctx, user.ID)
		}
		return nil, apperrors.NewInternalServerError("Failed to create tenant membership").WithDetails(err.Error())
	}
	op.TenantMembershipDone = true
	result.Steps.TenantMembership = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepReused}
	if changed {
		result.Steps.TenantMembership.Status = types.WorkspaceBootstrapStepCompleted
	}
	if err := h.saveBootstrapProgress(ctx, op, result); err != nil {
		return nil, err
	}
	return user, nil
}

func (h *TenantHandler) assignBootstrapUserToOrg(
	ctx context.Context,
	tenantID uint64,
	userID string,
	orgID string,
	orgRole types.OrgMemberRole,
	op *types.TenantBootstrapOperation,
	result *types.WorkspaceBootstrapResult,
) error {
	if op.OrgAssignmentDone {
		result.Steps.OrgAssignment = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepReused}
		return nil
	}
	if err := h.orgTreeService.AssignUserToOrg(ctx, orgID, tenantID, &types.AssignUserToOrgRequest{
		UserID: userID,
		Role:   orgRole,
	}); err != nil {
		result.Steps.OrgAssignment = types.WorkspaceBootstrapStepResult{
			Status:  types.WorkspaceBootstrapStepFailed,
			Message: err.Error(),
		}
		if saveErr := h.finalizeBootstrapResult(ctx, op, result, types.WorkspaceBootstrapStatusPartialSuccess); saveErr != nil {
			return saveErr
		}
		return nil
	}
	op.OrgAssignmentDone = true
	result.Steps.OrgAssignment = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepCompleted}
	return h.saveBootstrapProgress(ctx, op, result)
}

func (h *TenantHandler) ensureBootstrapInviteLink(
	ctx context.Context,
	tenantID uint64,
	role types.TenantRole,
	op *types.TenantBootstrapOperation,
	req *types.WorkspaceBootstrapRequest,
	result *types.WorkspaceBootstrapResult,
) error {
	if op.InviteURL != "" {
		result.InviteURL = op.InviteURL
		result.Steps.InviteLink = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepReused}
		return nil
	}
	caller, _ := types.UserIDFromContext(ctx)
	var invitedBy *string
	if caller != "" && !types.IsSyntheticUserID(caller) {
		invitedBy = &caller
	}
	_, plainToken, err := h.invitationService.CreateShareLink(ctx, tenantID, role, invitedBy, req.InviteMessage)
	if err != nil {
		if errors.Is(err, service.ErrAPIKeyCannotAssignOwner) {
			return apperrors.NewForbiddenError(err.Error())
		}
		return apperrors.NewInternalServerError("Failed to create share link").WithDetails(err.Error())
	}
	op.InviteURL = buildInviteRegisterURL(h.config, plainToken)
	result.InviteURL = op.InviteURL
	result.Steps.InviteLink = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepCompleted}
	return h.saveBootstrapProgress(ctx, op, result)
}

// BootstrapWorkspace completes the tenant setup wizard in a single backend
// workflow and persists per-tenant idempotent progress for refresh/retry.
func (h *TenantHandler) BootstrapWorkspace(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, ok := parseTenantIDFromPath(c)
	if !ok {
		return
	}
	if h.bootstrapService == nil || h.orgTreeService == nil || h.invitationService == nil || h.memberService == nil {
		c.Error(apperrors.NewInternalServerError("workspace bootstrap service unavailable"))
		return
	}

	var req types.WorkspaceBootstrapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewValidationError("Invalid request parameters").WithDetails(err.Error()))
		return
	}
	trimWorkspaceBootstrapRequest(&req)
	if err := validateWorkspaceBootstrapRequest(&req); err != nil {
		c.Error(err)
		return
	}

	caller, err := h.userService.GetCurrentUser(ctx)
	if err != nil || caller == nil {
		c.Error(apperrors.NewUnauthorizedError("authentication required"))
		return
	}
	op, _, err := h.bootstrapService.GetOrCreate(ctx, tenantID, req.IdempotencyKey)
	if err != nil {
		c.Error(apperrors.NewInternalServerError("Failed to load workspace bootstrap").WithDetails(err.Error()))
		return
	}

	reqHash, err := hashWorkspaceBootstrapRequest(&req)
	if err != nil {
		c.Error(apperrors.NewInternalServerError("Failed to hash workspace bootstrap request").WithDetails(err.Error()))
		return
	}
	if op.Status == types.WorkspaceBootstrapStatusCompleted &&
		op.RequestHash != "" && op.RequestHash != reqHash {
		c.Error(newBootstrapIdempotencyConflictError())
		return
	}
	if op.Status == types.WorkspaceBootstrapStatusCompleted {
		storedResult, err := loadWorkspaceBootstrapResult(op)
		if err != nil {
			c.Error(apperrors.NewInternalServerError("Failed to load workspace bootstrap result").WithDetails(err.Error()))
			return
		}
		if storedResult != nil {
			c.JSON(http.StatusOK, types.WorkspaceBootstrapResponse{
				Success: true,
				Data:    storedResult,
			})
			return
		}
	}

	result := makeBootstrapResult(&req)
	if err := h.persistBootstrapRequest(ctx, op, &req, result); err != nil {
		c.Error(apperrors.NewInternalServerError("Failed to persist workspace bootstrap state").WithDetails(err.Error()))
		return
	}

	callerTenantRole := types.TenantRoleFromContext(ctx)
	callerIsPrivileged := isPrivilegedOrgTreeOperator(caller)
	tenantRoleReq := &types.CreateUserInOrgRequest{
		Role:       req.OrgRole,
		TenantRole: req.TenantRole,
	}
	tenantRole, err := resolveTenantRoleForCreateUser(callerTenantRole, callerIsPrivileged, tenantRoleReq)
	if err != nil {
		c.Error(err)
		return
	}

	var orgRole types.OrgMemberRole
	if req.ShouldSetupOrg {
		orgRole, err = normalizeOrgRole(req.OrgRole)
		if err != nil {
			c.Error(err)
			return
		}
	}

	rootOrg, err := h.resolveBootstrapRootOrg(ctx, tenantID, caller.ID, op, &req, result)
	if err != nil {
		c.Error(err)
		return
	}

	switch req.Mode {
	case types.WorkspaceBootstrapModeExisting:
		user, err := h.ensureExistingBootstrapMember(ctx, tenantID, tenantRole, op, &req, result)
		if err != nil {
			c.Error(err)
			return
		}
		if rootOrg != nil {
			if err := h.assignBootstrapUserToOrg(ctx, tenantID, user.ID, rootOrg.ID, orgRole, op, result); err != nil {
				c.Error(err)
				return
			}
		} else {
			result.Steps.OrgAssignment = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepSkipped}
		}
	case types.WorkspaceBootstrapModeNew:
		user, err := h.ensureBootstrapNewUser(ctx, tenantID, tenantRole, op, &req, result)
		if err != nil {
			c.Error(err)
			return
		}
		if rootOrg == nil {
			c.Error(apperrors.NewValidationError("new-user bootstrap requires an organization node"))
			return
		}
		if err := h.assignBootstrapUserToOrg(ctx, tenantID, user.ID, rootOrg.ID, orgRole, op, result); err != nil {
			c.Error(err)
			return
		}
	case types.WorkspaceBootstrapModeInvite:
		result.Steps.UserProvision = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepSkipped}
		result.Steps.TenantMembership = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepSkipped}
		result.Steps.OrgAssignment = types.WorkspaceBootstrapStepResult{Status: types.WorkspaceBootstrapStepSkipped}
		if err := h.ensureBootstrapInviteLink(ctx, tenantID, tenantRole, op, &req, result); err != nil {
			c.Error(err)
			return
		}
	}

	finalStatus := types.WorkspaceBootstrapStatusCompleted
	if result.Steps.OrgAssignment.Status == types.WorkspaceBootstrapStepFailed {
		finalStatus = types.WorkspaceBootstrapStatusPartialSuccess
	}
	if err := h.finalizeBootstrapResult(ctx, op, result, finalStatus); err != nil {
		c.Error(apperrors.NewInternalServerError("Failed to save workspace bootstrap result").WithDetails(err.Error()))
		return
	}

	message := "Workspace bootstrap completed"
	if finalStatus == types.WorkspaceBootstrapStatusPartialSuccess {
		message = "Workspace bootstrap completed with warnings"
	}
	c.JSON(http.StatusOK, types.WorkspaceBootstrapResponse{
		Success: true,
		Data:    result,
		Message: message,
	})
}
