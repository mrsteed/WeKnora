package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type stubProjectedOrgTreeRepo struct {
	interfaces.OrgTreeRepository
	tenantOrgs []*types.Organization
}

func (s *stubProjectedOrgTreeRepo) ListByTenantID(context.Context, uint64) ([]*types.Organization, error) {
	return s.tenantOrgs, nil
}

type stubProjectedOrganizationRepo struct {
	interfaces.OrganizationRepository
	userOrgs     []*types.Organization
	membersByOrg map[string][]*types.OrganizationMember
}

func (s *stubProjectedOrganizationRepo) ListOrgTreeOrganizationsByUserID(context.Context, string) ([]*types.Organization, error) {
	return s.userOrgs, nil
}

func (s *stubProjectedOrganizationRepo) ListOrgTreeMembers(_ context.Context, orgID string) ([]*types.OrganizationMember, error) {
	if s.membersByOrg == nil {
		return nil, nil
	}
	return s.membersByOrg[orgID], nil
}

func TestGetUserOrganizations_ProjectsRootForTenantOwner(t *testing.T) {
	tenantID := uint64(42)
	ctx := context.WithValue(context.Background(), types.TenantRoleContextKey, types.TenantRoleOwner)
	orgTreeRepo := &stubProjectedOrgTreeRepo{
		tenantOrgs: []*types.Organization{{
			ID:          "root-org",
			Name:        "Root",
			Path:        "/root-org",
			Level:       1,
			OrgTenantID: &tenantID,
		}},
	}
	orgRepo := &stubProjectedOrganizationRepo{}
	svc := &orgTreeService{orgTreeRepo: orgTreeRepo, orgRepo: orgRepo}

	orgs, err := svc.GetUserOrganizations(ctx, "u-admin", tenantID)
	if err != nil {
		t.Fatalf("GetUserOrganizations err = %v", err)
	}
	if len(orgs) != 1 {
		t.Fatalf("len(orgs) = %d, want 1", len(orgs))
	}
	if orgs[0].ID != "root-org" {
		t.Fatalf("org id = %q, want root-org", orgs[0].ID)
	}
	if !orgs[0].MyIsAdmin {
		t.Fatal("expected projected root org to be marked manageable for tenant owner")
	}
}

func TestGetUserOrganizations_ProjectsRootForTenantAdminWithoutRootMembership(t *testing.T) {
	tenantID := uint64(42)
	ctx := context.WithValue(context.Background(), types.TenantRoleContextKey, types.TenantRoleAdmin)
	orgTreeRepo := &stubProjectedOrgTreeRepo{
		tenantOrgs: []*types.Organization{{
			ID:          "root-org",
			Name:        "Root",
			Path:        "/root-org",
			Level:       1,
			OrgTenantID: &tenantID,
		}},
	}
	orgRepo := &stubProjectedOrganizationRepo{}
	svc := &orgTreeService{orgTreeRepo: orgTreeRepo, orgRepo: orgRepo}

	orgs, err := svc.GetUserOrganizations(ctx, "u-admin", tenantID)
	if err != nil {
		t.Fatalf("GetUserOrganizations err = %v", err)
	}
	if len(orgs) != 1 {
		t.Fatalf("len(orgs) = %d, want 1", len(orgs))
	}
	if orgs[0].ID != "root-org" {
		t.Fatalf("org id = %q, want root-org", orgs[0].ID)
	}
	if !orgs[0].MyIsAdmin {
		t.Fatal("expected projected root org to be marked manageable for tenant admin")
	}
}

func TestGetUserOrganizations_PreservesExplicitRootAdminMembership(t *testing.T) {
	tenantID := uint64(42)
	ctx := context.WithValue(context.Background(), types.TenantRoleContextKey, types.TenantRoleContributor)
	rootOrg := &types.Organization{
		ID:          "root-org",
		Name:        "Root",
		Path:        "/root-org",
		Level:       1,
		OrgTenantID: &tenantID,
	}
	orgTreeRepo := &stubProjectedOrgTreeRepo{tenantOrgs: []*types.Organization{rootOrg}}
	orgRepo := &stubProjectedOrganizationRepo{
		userOrgs: []*types.Organization{rootOrg},
		membersByOrg: map[string][]*types.OrganizationMember{
			"root-org": {{UserID: "u-root-admin", Role: types.OrgRoleAdmin, User: &types.User{ID: "u-root-admin"}}},
		},
	}
	svc := &orgTreeService{orgTreeRepo: orgTreeRepo, orgRepo: orgRepo}

	orgs, err := svc.GetUserOrganizations(ctx, "u-root-admin", tenantID)
	if err != nil {
		t.Fatalf("GetUserOrganizations err = %v", err)
	}
	if len(orgs) != 1 {
		t.Fatalf("len(orgs) = %d, want 1", len(orgs))
	}
	if !orgs[0].MyIsAdmin {
		t.Fatal("expected explicit root org admin membership to be preserved")
	}
}

func TestGetUserOrganizations_DoesNotProjectRootForViewer(t *testing.T) {
	tenantID := uint64(42)
	ctx := context.WithValue(context.Background(), types.TenantRoleContextKey, types.TenantRoleViewer)
	orgTreeRepo := &stubProjectedOrgTreeRepo{
		tenantOrgs: []*types.Organization{{
			ID:          "root-org",
			Name:        "Root",
			Path:        "/root-org",
			Level:       1,
			OrgTenantID: &tenantID,
		}},
	}
	orgRepo := &stubProjectedOrganizationRepo{}
	svc := &orgTreeService{orgTreeRepo: orgTreeRepo, orgRepo: orgRepo}

	orgs, err := svc.GetUserOrganizations(ctx, "u-viewer", tenantID)
	if err != nil {
		t.Fatalf("GetUserOrganizations err = %v", err)
	}
	if len(orgs) != 0 {
		t.Fatalf("len(orgs) = %d, want 0", len(orgs))
	}
}