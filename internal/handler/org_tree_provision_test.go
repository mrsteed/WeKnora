package handler

import (
	"testing"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestSelectProvisionUserCandidate_RejectsConflictingMatches(t *testing.T) {
	candidate := selectProvisionUserCandidate(
		&types.User{ID: "u-1"},
		&types.User{ID: "u-2"},
	)
	if candidate != nil {
		t.Fatal("expected conflicting matches to be rejected")
	}
}

func TestSelectProvisionUserCandidate_AllowsSameUserAcrossIdentifiers(t *testing.T) {
	candidate := selectProvisionUserCandidate(
		&types.User{ID: "u-1"},
		&types.User{ID: "u-1"},
		nil,
	)
	if candidate == nil || candidate.ID != "u-1" {
		t.Fatalf("candidate=%v, want user u-1", candidate)
	}
}

func TestIsReusableProvisionedUser_AcceptsOrphanedExactMatch(t *testing.T) {
	user := &types.User{
		ID:       "u-1",
		TenantID: 42,
		Username: "吴晓栋",
		Email:    "wxd@hlsa.com",
		Phone:    "13663861111",
	}
	matchedCount := countProvisionUserMatches(user, user, user, nil)
	if !isReusableProvisionedUser(user, 42, 0, matchedCount) {
		t.Fatal("expected exact-match orphaned user to be reusable")
	}
}

func TestIsReusableProvisionedUser_RejectsUsersStillInOrg(t *testing.T) {
	user := &types.User{
		ID:       "u-1",
		TenantID: 42,
		Username: "吴晓栋",
		Email:    "wxd@hlsa.com",
		Phone:    "13663861111",
	}
	matchedCount := countProvisionUserMatches(user, user, user, nil)
	if isReusableProvisionedUser(user, 42, 1, matchedCount) {
		t.Fatal("expected users still bound to orgs to be rejected")
	}
}

func TestIsReusableProvisionedUser_AcceptsPhoneChangeAfterDeletion(t *testing.T) {
	user := &types.User{
		ID:       "u-1",
		TenantID: 42,
		Username: "吴晓栋",
		Email:    "wxd@hlsa.com",
		Phone:    "13661111111",
	}
	matchedCount := countProvisionUserMatches(user, user, user, nil)
	if !isReusableProvisionedUser(user, 42, 0, matchedCount) {
		t.Fatal("expected username+email match to allow restoring with a new phone")
	}
}

func TestResolveTenantRoleForProvision_DefaultsViewer(t *testing.T) {
	_, err := resolveTenantRoleForProvision(types.TenantRoleViewer, &types.CreateUserInOrgRequest{})
	if err == nil {
		t.Fatal("expected forbidden error for non-admin tenant role")
	}
	if _, ok := err.(*apperrors.AppError); !ok {
		t.Fatalf("expected AppError, got %T", err)
	}
}

func TestResolveTenantRoleForProvision_DefaultsViewerForTenantAdmin(t *testing.T) {
	role, err := resolveTenantRoleForProvision(types.TenantRoleAdmin, &types.CreateUserInOrgRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if role != types.TenantRoleViewer {
		t.Fatalf("role=%s want viewer", role)
	}
}

func TestResolveTenantRoleForProvision_RequiresTenantAdminForAdminRole(t *testing.T) {
	_, err := resolveTenantRoleForProvision(types.TenantRoleViewer, &types.CreateUserInOrgRequest{TenantRole: "admin"})
	if err == nil {
		t.Fatal("expected forbidden error")
	}
	if _, ok := err.(*apperrors.AppError); !ok {
		t.Fatalf("expected AppError, got %T", err)
	}
}

func TestResolveTenantRoleForProvision_RejectsTenantAdminForExplicitTenantRole(t *testing.T) {
	role, err := resolveTenantRoleForProvision(types.TenantRoleAdmin, &types.CreateUserInOrgRequest{TenantRole: "admin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if role != types.TenantRoleAdmin {
		t.Fatalf("role=%s want admin", role)
	}
}

func TestResolveTenantRoleForProvision_RejectsTenantAdminForOwnerRole(t *testing.T) {
	_, err := resolveTenantRoleForProvision(types.TenantRoleAdmin, &types.CreateUserInOrgRequest{TenantRole: "owner"})
	if err == nil {
		t.Fatal("expected forbidden error for tenant admin assigning owner")
	}
}

func TestResolveTenantRoleForProvision_AllowsTenantOwnerForOwnerRole(t *testing.T) {
	role, err := resolveTenantRoleForProvision(types.TenantRoleOwner, &types.CreateUserInOrgRequest{TenantRole: "owner"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if role != types.TenantRoleOwner {
		t.Fatalf("role=%s want owner", role)
	}
}

func TestResolveTenantRoleForOrgScopedProvision_MapsAdminOrgRoleToTenantAdmin(t *testing.T) {
	role, err := resolveTenantRoleForOrgScopedProvision(types.TenantRoleContributor, true, &types.CreateUserInOrgRequest{Role: "admin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if role != types.TenantRoleContributor {
		t.Fatalf("role=%s want contributor", role)
	}
}

func TestResolveTenantRoleForOrgScopedProvision_MapsEditorOrgRoleToContributor(t *testing.T) {
	role, err := resolveTenantRoleForOrgScopedProvision(types.TenantRoleViewer, true, &types.CreateUserInOrgRequest{Role: "editor"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if role != types.TenantRoleContributor {
		t.Fatalf("role=%s want contributor", role)
	}
}


func TestResolveTenantRoleForOrgScopedProvision_MapsViewerOrgRoleToViewer(t *testing.T) {
	role, err := resolveTenantRoleForOrgScopedProvision(types.TenantRoleContributor, true, &types.CreateUserInOrgRequest{Role: "viewer"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if role != types.TenantRoleViewer {
		t.Fatalf("role=%s want viewer", role)
	}
}

func TestResolveTenantRoleForCreateUser_PrefersExplicitTenantRoleForTenantOwner(t *testing.T) {
	role, err := resolveTenantRoleForCreateUser(types.TenantRoleOwner, false, &types.CreateUserInOrgRequest{
		Role:       "admin",
		TenantRole: "owner",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if role != types.TenantRoleOwner {
		t.Fatalf("role=%s want owner", role)
	}
}

func TestResolveTenantRoleForCreateUser_DefaultsMissingTenantRoleFromOrgRole(t *testing.T) {
	tests := []struct {
		name       string
		callerRole types.TenantRole
		orgRole    string
		want       types.TenantRole
	}{
		{
			name:       "tenant owner suborg admin defaults to contributor",
			callerRole: types.TenantRoleOwner,
			orgRole:    "admin",
			want:       types.TenantRoleContributor,
		},
		{
			name:       "tenant admin editor defaults to contributor",
			callerRole: types.TenantRoleAdmin,
			orgRole:    "editor",
			want:       types.TenantRoleContributor,
		},
		{
			name:       "tenant admin viewer stays viewer",
			callerRole: types.TenantRoleAdmin,
			orgRole:    "viewer",
			want:       types.TenantRoleViewer,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, err := resolveTenantRoleForCreateUser(tt.callerRole, false, &types.CreateUserInOrgRequest{
				Role: tt.orgRole,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if role != tt.want {
				t.Fatalf("role=%s want %s", role, tt.want)
			}
		})
	}
}

func TestResolveTenantRoleForCreateUser_PreservesExplicitTenantRoleForTenantAdmin(t *testing.T) {
	role, err := resolveTenantRoleForCreateUser(types.TenantRoleAdmin, false, &types.CreateUserInOrgRequest{
		Role:       "admin",
		TenantRole: "admin",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if role != types.TenantRoleAdmin {
		t.Fatalf("role=%s want admin", role)
	}
}

func TestResolveTenantRoleForCreateUser_UsesOrgScopedCapForBranchAdmin(t *testing.T) {
	role, err := resolveTenantRoleForCreateUser(types.TenantRoleContributor, false, &types.CreateUserInOrgRequest{
		Role:       "admin",
		TenantRole: "owner",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if role != types.TenantRoleContributor {
		t.Fatalf("role=%s want contributor", role)
	}
}

func TestResolveTenantRoleForCreateUser_AllowsPrivilegedOperatorToAssignOwner(t *testing.T) {
	role, err := resolveTenantRoleForCreateUser(types.TenantRoleAdmin, true, &types.CreateUserInOrgRequest{
		Role:       "admin",
		TenantRole: "owner",
	})
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

func TestCanBootstrapRootOrg_AllowsPlatformSuperAdmin(t *testing.T) {
	if !canBootstrapRootOrg(&types.User{ID: "u1", IsSuperAdmin: true}, types.TenantRoleViewer, 3) {
		t.Fatal("expected platform super admin to bypass bootstrap restriction")
	}
}