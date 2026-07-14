package handler

import (
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	assignTenantRoleDeniedMessage = "You do not have permission to assign the requested tenant role"
	manageTenantRoleDeniedMessage = "You do not have permission to manage the requested tenant role"
)

func authorizeTenantRoleProvision(callerTenantRole, requestedRole types.TenantRole) error {
	if !requestedRole.IsValid() {
		return apperrors.NewValidationError("tenant_role must be one of owner/admin/contributor/viewer")
	}
	if !callerTenantRole.HasPermission(types.TenantRoleAdmin) {
		return apperrors.NewForbiddenError(assignTenantRoleDeniedMessage)
	}
	if requestedRole == types.TenantRoleOwner && !callerTenantRole.HasPermission(types.TenantRoleOwner) {
		return apperrors.NewForbiddenError(assignTenantRoleDeniedMessage)
	}
	return nil
}

func authorizeTenantMemberRoleChange(callerTenantRole, currentRole, requestedRole types.TenantRole) error {
	if !requestedRole.IsValid() {
		return apperrors.NewValidationError("role must be one of owner/admin/contributor/viewer")
	}
	return authorizeTenantRoleMutation(callerTenantRole, currentRole, requestedRole, true)
}

func authorizeTenantMemberRemoval(callerTenantRole, currentRole types.TenantRole) error {
	return authorizeTenantRoleMutation(callerTenantRole, currentRole, "", false)
}

func authorizeTenantRoleMutation(
	callerTenantRole types.TenantRole,
	currentRole types.TenantRole,
	requestedRole types.TenantRole,
	checkRequestedRole bool,
) error {
	if !callerTenantRole.HasPermission(types.TenantRoleAdmin) {
		return apperrors.NewForbiddenError(manageTenantRoleDeniedMessage)
	}
	if currentRole == types.TenantRoleOwner && !callerTenantRole.HasPermission(types.TenantRoleOwner) {
		return apperrors.NewForbiddenError(manageTenantRoleDeniedMessage)
	}
	if checkRequestedRole && requestedRole == types.TenantRoleOwner && !callerTenantRole.HasPermission(types.TenantRoleOwner) {
		return apperrors.NewForbiddenError(manageTenantRoleDeniedMessage)
	}
	return nil
}