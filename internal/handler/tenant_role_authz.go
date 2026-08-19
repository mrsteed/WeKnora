package handler

import (
	"context"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
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

func effectiveTenantRoleForUser(
	ctx context.Context,
	rawRole types.TenantRole,
	userID string,
	tenantID uint64,
	orgTreeService interfaces.OrgTreeService,
) types.TenantRole {
	_ = ctx
	_ = userID
	_ = tenantID
	_ = orgTreeService
	return rawRole
}

func displayTenantRoleForUser(
	ctx context.Context,
	rawRole types.TenantRole,
	userID string,
	tenantID uint64,
	orgTreeService interfaces.OrgTreeService,
) types.TenantRole {
	_ = ctx
	_ = userID
	_ = tenantID
	_ = orgTreeService
	return rawRole
}
