package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type stubSameTenantOrgTreeService struct {
	interfaces.OrgTreeService
	userOrgs                 []*types.OrgTreeNode
	ancestorIDs              []string
	allDescendantIDs         []string
	adminDescendantIDs       []string
	adminPathPrefixesChecked []string
	allPathPrefixesChecked   []string
}

func (s *stubSameTenantOrgTreeService) GetUserOrganizations(context.Context, string, uint64) ([]*types.OrgTreeNode, error) {
	return s.userOrgs, nil
}

func (s *stubSameTenantOrgTreeService) GetAncestorIDsFromPaths(_ []string) []string {
	return append([]string(nil), s.ancestorIDs...)
}

func (s *stubSameTenantOrgTreeService) GetDescendantIDsByPaths(_ context.Context, pathPrefixes []string, _ uint64) ([]string, error) {
	if len(pathPrefixes) == 1 && pathPrefixes[0] == "/root/team-admin" {
		s.adminPathPrefixesChecked = append([]string(nil), pathPrefixes...)
		return append([]string(nil), s.adminDescendantIDs...), nil
	}
	s.allPathPrefixesChecked = append([]string(nil), pathPrefixes...)
	return append([]string(nil), s.allDescendantIDs...), nil
}

func TestBuildSameTenantOrgScope_ReadIncludesOwnOrgsAndDescendants(t *testing.T) {
	scope := buildSameTenantOrgScope(
		[]string{"team-child"},
		[]string{"team-grandchild"},
		[]string{"team-child"},
		[]string{"team-grandchild"},
		[]string{"team-child"},
		[]string{"team-grandchild"},
		false,
	)

	for _, id := range []string{"team-child", "team-grandchild"} {
		if _, ok := scope.readOrgIDs[id]; !ok {
			t.Fatalf("read scope missing %s", id)
		}
	}
	if _, ok := scope.readOrgIDs["team-parent"]; ok {
		t.Fatal("read scope should not include ancestor orgs automatically")
	}
	if _, ok := scope.personnelManageOrgIDs["team-parent"]; ok {
		t.Fatal("personnel manage scope should not include ancestor orgs automatically")
	}
	if _, ok := scope.resourceManageOrgIDs["team-parent"]; ok {
		t.Fatal("resource manage scope should not include ancestor orgs automatically")
	}
}

func TestSameTenantResourceAuthorizer_ManageScopeUsesAdminSubtreeOnly(t *testing.T) {
	authorizer := newSameTenantResourceAuthorizer(&stubSameTenantOrgTreeService{
		userOrgs: []*types.OrgTreeNode{
			{ID: "team-admin", Path: "/root/team-admin", Level: 2, MyIsAdmin: true},
			{ID: "team-member", Path: "/root/team-member", Level: 2, MyIsAdmin: false},
		},
		allDescendantIDs:   []string{"team-admin-child", "team-member-child"},
		adminDescendantIDs: []string{"team-admin-child"},
	})

	scope, err := authorizer.resolveScope(context.Background(), "u1", 1)
	if err != nil {
		t.Fatalf("resolveScope err = %v", err)
	}

	if _, ok := scope.readOrgIDs["root"]; ok {
		t.Fatal("did not expect ancestor org in read scope")
	}
	if _, ok := scope.personnelManageOrgIDs["team-admin-child"]; !ok {
		t.Fatal("expected admin descendant org in personnel-manage scope")
	}
	if _, ok := scope.resourceManageOrgIDs["team-admin-child"]; !ok {
		t.Fatal("expected admin descendant org in resource-manage scope")
	}
	if _, ok := scope.resourceManageOrgIDs["team-member-child"]; ok {
		t.Fatal("non-admin descendant leaked into resource-manage scope")
	}

	canReadParent := authorizer.canReadResource(sameTenantResourceRule{
		Visibility:     types.KBVisibilityOrg,
		OrganizationID: "root",
	}, "u1", false, scope)
	if canReadParent {
		t.Fatal("did not expect child org member to read ancestor-scoped resource")
	}

	canManageAdminChild := authorizer.canManageResource(sameTenantResourceRule{
		Visibility:     types.KBVisibilityOrg,
		OrganizationID: "team-admin-child",
	}, "u1", false, scope)
	if !canManageAdminChild {
		t.Fatal("expected org admin to manage resources in admin subtree")
	}

	canManageParent := authorizer.canManageResource(sameTenantResourceRule{
		Visibility:     types.KBVisibilityOrg,
		OrganizationID: "root",
	}, "u1", false, scope)
	if canManageParent {
		t.Fatal("did not expect child org admin to manage ancestor-scoped resource")
	}
}

func TestSameTenantResourceAuthorizer_TenantOwnerCanManagePrivateResource(t *testing.T) {
	authorizer := newSameTenantResourceAuthorizer(&stubSameTenantOrgTreeService{})
	ctx := context.WithValue(context.Background(), types.TenantRoleContextKey, types.TenantRoleOwner)

	scope, err := authorizer.resolveScope(ctx, "u-admin", 1)
	if err != nil {
		t.Fatalf("resolveScope err = %v", err)
	}
	if !scope.isTenantAdminOrOwner {
		t.Fatal("expected tenant owner flag to be set in scope")
	}

	canManagePrivate := authorizer.canManageResource(sameTenantResourceRule{
		Visibility: types.KBVisibilityPrivate,
		CreatedBy:  "u-other",
	}, "u-admin", false, scope)
	if !canManagePrivate {
		t.Fatal("expected tenant owner to manage same-tenant private resource")
	}

	canReadPrivate := authorizer.canReadResource(sameTenantResourceRule{
		Visibility: types.KBVisibilityPrivate,
		CreatedBy:  "u-other",
	}, "u-admin", false, scope)
	if canReadPrivate {
		t.Fatal("did not expect tenant owner flag to bypass private read visibility")
	}
}

func TestSameTenantResourceAuthorizer_ExplicitRootAdminGetsTenantAdminScope(t *testing.T) {
	authorizer := newSameTenantResourceAuthorizer(&stubSameTenantOrgTreeService{
		userOrgs: []*types.OrgTreeNode{{ID: "root-org", Path: "/root-org", Level: 1, MyIsAdmin: true}},
	})
	ctx := context.WithValue(context.Background(), types.TenantRoleContextKey, types.TenantRoleContributor)

	scope, err := authorizer.resolveScope(ctx, "u-root-admin", 1)
	if err != nil {
		t.Fatalf("resolveScope err = %v", err)
	}
	if !scope.isTenantAdminOrOwner {
		t.Fatal("expected explicit root admin membership to grant tenant-admin scope")
	}
}