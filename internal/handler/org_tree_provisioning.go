package handler

import (
	"context"
	"errors"
	"strings"

	"github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

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

func findReusableProvisionedUser(
	ctx context.Context,
	orgTreeService interfaces.OrgTreeService,
	userService interfaces.UserService,
	tenantID uint64,
	req *types.CreateUserInOrgRequest,
) (*types.User, error) {
	usernameMatch, _ := userService.GetUserByUsername(ctx, req.Username)
	var emailMatch *types.User
	if req.Email != "" {
		emailMatch, _ = userService.GetUserByEmail(ctx, req.Email)
	}
	var phoneMatch *types.User
	if req.Phone != "" {
		phoneMatch, _ = userService.GetUserByPhone(ctx, req.Phone)
	}

	candidate := selectProvisionUserCandidate(usernameMatch, emailMatch, phoneMatch)
	if candidate == nil {
		return nil, nil
	}

	orgs, err := orgTreeService.GetUserOrganizations(ctx, candidate.ID, tenantID)
	if err != nil {
		return nil, err
	}
	matchedCount := countProvisionUserMatches(candidate, usernameMatch, emailMatch, phoneMatch)
	if !isReusableProvisionedUser(candidate, tenantID, len(orgs), matchedCount) {
		return nil, nil
	}
	return candidate, nil
}

func provisionUserForOrg(
	ctx context.Context,
	orgTreeService interfaces.OrgTreeService,
	userService interfaces.UserService,
	tenantID uint64,
	req *types.CreateUserInOrgRequest,
) (*types.User, bool, error) {
	reusableUser, err := findReusableProvisionedUser(ctx, orgTreeService, userService, tenantID, req)
	if err != nil {
		return nil, false, err
	}
	if reusableUser == nil {
		user, err := userService.CreateUserByAdmin(ctx, req, tenantID)
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
	if err := userService.UpdateUser(ctx, reusableUser); err != nil {
		return nil, false, err
	}
	if err := userService.AdminSetPassword(ctx, reusableUser.ID, req.Password); err != nil {
		return nil, false, err
	}
	return reusableUser, false, nil
}

func ensureProvisionedMembership(
	ctx context.Context,
	memberService interfaces.TenantMemberService,
	userID string,
	tenantID uint64,
	role types.TenantRole,
) (bool, error) {
	currentMember, err := memberService.GetMembership(ctx, userID, tenantID)
	if err != nil {
		return false, err
	}
	if currentMember == nil {
		_, err := memberService.AddMember(ctx, userID, tenantID, role, nil)
		return true, err
	}
	if currentMember.Role == role {
		return false, nil
	}
	return true, memberService.UpdateRole(ctx, userID, tenantID, role)
}

func newUserConflictAppError(field string, message string) *apperrors.AppError {
	return apperrors.NewConflictError(message).WithDetails(map[string]string{
		"reason": "user_conflict",
		"field":  field,
	})
}

func newBootstrapIdempotencyConflictError() *apperrors.AppError {
	return apperrors.NewConflictError("Bootstrap idempotency key is already bound to a completed request").WithDetails(map[string]string{
		"reason": "idempotency_key_reused",
		"field":  "idempotency_key",
	})
}

func translateUserProvisionError(err error) *apperrors.AppError {
	switch {
	case errors.Is(err, service.ErrUserContactRequired),
		errors.Is(err, service.ErrUserCredentialsRequired),
		errors.Is(err, service.ErrPasswordPolicy):
		return apperrors.NewValidationError(err.Error())
	case errors.Is(err, service.ErrUsernameAlreadyExists):
		return newUserConflictAppError("username", "Username already exists")
	case errors.Is(err, service.ErrEmailAlreadyExists):
		return newUserConflictAppError("email", "Email already exists")
	case errors.Is(err, service.ErrPhoneAlreadyExists):
		return newUserConflictAppError("phone", "Phone already exists")
	case errors.Is(err, service.ErrUserAlreadyExists):
		return apperrors.NewConflictError("User already exists")
	default:
		return apperrors.NewInternalServerError("Failed to create user").WithDetails(err.Error())
	}
}

func validateUserIdentityUniqueness(
	ctx context.Context,
	userService interfaces.UserService,
	currentUserID string,
	username string,
	email string,
	phone string,
) error {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	phone = strings.TrimSpace(phone)
	if username != "" {
		existingUser, _ := userService.GetUserByUsername(ctx, username)
		if existingUser != nil && existingUser.ID != currentUserID {
			return service.ErrUsernameAlreadyExists
		}
	}
	if email != "" {
		existingUser, _ := userService.GetUserByEmail(ctx, email)
		if existingUser != nil && existingUser.ID != currentUserID {
			return service.ErrEmailAlreadyExists
		}
	}
	if phone != "" {
		existingUser, _ := userService.GetUserByPhone(ctx, phone)
		if existingUser != nil && existingUser.ID != currentUserID {
			return service.ErrPhoneAlreadyExists
		}
	}
	return nil
}

type bootstrapRootOrgCandidate struct {
	ID   string
	Name string
}

func bootstrapOrgIsRoot(org *types.Organization) bool {
	if org == nil {
		return false
	}
	if org.ParentID == nil {
		return true
	}
	return strings.TrimSpace(*org.ParentID) == "" || org.Level <= 1
}

func selectReusableRootOrgCandidate(tree []*types.OrgTreeNode, preferredNames ...string) *bootstrapRootOrgCandidate {
	var roots []*types.OrgTreeNode
	var walk func(nodes []*types.OrgTreeNode)
	walk = func(nodes []*types.OrgTreeNode) {
		for _, node := range nodes {
			if node == nil {
				continue
			}
			if node.ParentID == nil || strings.TrimSpace(*node.ParentID) == "" || node.Level <= 1 {
				roots = append(roots, node)
			}
			walk(node.Children)
		}
	}
	walk(tree)
	if len(roots) == 0 {
		return nil
	}

	for _, preferredName := range preferredNames {
		preferredName = strings.TrimSpace(preferredName)
		if preferredName == "" {
			continue
		}
		for _, root := range roots {
			if strings.TrimSpace(root.Name) == preferredName {
				return &bootstrapRootOrgCandidate{ID: root.ID, Name: root.Name}
			}
		}
	}

	if len(roots) == 1 {
		return &bootstrapRootOrgCandidate{ID: roots[0].ID, Name: roots[0].Name}
	}

	return nil
}
