package handler

import (
	"testing"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestResolveTenantRoleForProvision_DefaultsViewer(t *testing.T) {
	role, err := resolveTenantRoleForProvision(&types.User{ID: "u1"}, types.TenantRoleViewer, &types.CreateUserInOrgRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if role != types.TenantRoleViewer {
		t.Fatalf("role=%s want viewer", role)
	}
}

func TestResolveTenantRoleForProvision_RequiresTenantAdminForAdminRole(t *testing.T) {
	_, err := resolveTenantRoleForProvision(&types.User{ID: "u1"}, types.TenantRoleViewer, &types.CreateUserInOrgRequest{TenantRole: "admin"})
	if err == nil {
		t.Fatal("expected forbidden error")
	}
	if _, ok := err.(*apperrors.AppError); !ok {
		t.Fatalf("expected AppError, got %T", err)
	}
}

func TestResolveTenantRoleForProvision_RejectsTenantAdminForExplicitTenantRole(t *testing.T) {
	_, err := resolveTenantRoleForProvision(&types.User{ID: "u1"}, types.TenantRoleAdmin, &types.CreateUserInOrgRequest{TenantRole: "admin"})
	if err == nil {
		t.Fatal("expected forbidden error for tenant admin assigning explicit tenant role")
	}
}

func TestResolveTenantRoleForProvision_AllowsSuperAdminForOwnerRole(t *testing.T) {
	role, err := resolveTenantRoleForProvision(&types.User{ID: "u1", IsSystemAdmin: true}, types.TenantRoleAdmin, &types.CreateUserInOrgRequest{TenantRole: "owner"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if role != types.TenantRoleOwner {
		t.Fatalf("role=%s want owner", role)
	}
}

func TestCanBootstrapRootOrg_AllowsTenantOwnerOnEmptyTree(t *testing.T) {
	if !canBootstrapRootOrg(&types.User{ID: "u1"}, types.TenantRoleOwner, 0) {
		t.Fatal("expected owner to bootstrap first root org")
	}
}

func TestCanBootstrapRootOrg_DeniesViewerOnEmptyTree(t *testing.T) {
	if canBootstrapRootOrg(&types.User{ID: "u1"}, types.TenantRoleViewer, 0) {
		t.Fatal("expected viewer to be denied bootstrap")
	}
}

func TestCanBootstrapRootOrg_DeniesTenantAdminWhenTreeExists(t *testing.T) {
	if canBootstrapRootOrg(&types.User{ID: "u1"}, types.TenantRoleAdmin, 1) {
		t.Fatal("expected tenant admin to be denied once org tree already exists")
	}
}

func TestCanBootstrapRootOrg_AllowsCrossTenantSuperuser(t *testing.T) {
	if !canBootstrapRootOrg(&types.User{ID: "u1", CanAccessAllTenants: true}, types.TenantRoleViewer, 3) {
		t.Fatal("expected cross-tenant superuser to bypass bootstrap restriction")
	}
}