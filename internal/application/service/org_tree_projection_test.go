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
	userOrgs      []*types.Organization
	membersByOrg  map[string][]*types.OrganizationMember
	memberCounts  map[string]int
	memberUserIDs map[string][]string
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

func (s *stubProjectedOrganizationRepo) BatchCountOrgTreeMembers(_ context.Context, orgIDs []string) (map[string]int, error) {
	out := make(map[string]int, len(orgIDs))
	for _, orgID := range orgIDs {
		if s.memberCounts != nil {
			out[orgID] = s.memberCounts[orgID]
			continue
		}
		out[orgID] = len(s.membersByOrg[orgID])
	}
	return out, nil
}

func (s *stubProjectedOrganizationRepo) BatchListOrgTreeMemberUserIDs(_ context.Context, orgIDs []string) (map[string][]string, error) {
	out := make(map[string][]string, len(orgIDs))
	for _, orgID := range orgIDs {
		if s.memberUserIDs != nil {
			out[orgID] = s.memberUserIDs[orgID]
			continue
		}
		members := s.membersByOrg[orgID]
		ids := make([]string, 0, len(members))
		for _, member := range members {
			if member != nil {
				ids = append(ids, member.UserID)
			}
		}
		out[orgID] = ids
	}
	return out, nil
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

func TestGetTreeForUser_TenantOwnerGetsManageableFullTree(t *testing.T) {
	tenantID := uint64(42)
	ctx := context.WithValue(context.Background(), types.TenantRoleContextKey, types.TenantRoleOwner)
	rootID := "root-org"
	orgTreeRepo := &stubProjectedOrgTreeRepo{
		tenantOrgs: []*types.Organization{
			{
				ID:          rootID,
				Name:        "Root",
				Path:        "/root-org",
				Level:       1,
				OrgTenantID: &tenantID,
			},
			{
				ID:          "child-org",
				Name:        "Child",
				ParentID:    &rootID,
				Path:        "/root-org/child-org",
				Level:       2,
				OrgTenantID: &tenantID,
			},
		},
	}
	orgRepo := &stubProjectedOrganizationRepo{}
	svc := &orgTreeService{orgTreeRepo: orgTreeRepo, orgRepo: orgRepo}

	tree, err := svc.GetTreeForUser(ctx, "u-owner", tenantID, false)
	if err != nil {
		t.Fatalf("GetTreeForUser err = %v", err)
	}
	if len(tree) != 1 {
		t.Fatalf("len(tree) = %d, want 1", len(tree))
	}
	if !tree[0].MyIsAdmin {
		t.Fatal("expected owner root node to be manageable")
	}
	if len(tree[0].Children) != 1 {
		t.Fatalf("len(children) = %d, want 1", len(tree[0].Children))
	}
	if !tree[0].Children[0].MyIsAdmin {
		t.Fatal("expected owner child node to inherit manageable flag")
	}
}