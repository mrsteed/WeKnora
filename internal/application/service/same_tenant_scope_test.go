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

func TestBuildSameTenantOrgScope_ReadIncludesAncestorsAndDescendants(t *testing.T) {
	scope := buildSameTenantOrgScope(
		[]string{"team-child"},
		[]string{"team-parent"},
		[]string{"team-grandchild"},
		[]string{"team-child"},
		[]string{"team-grandchild"},
	)

	for _, id := range []string{"team-child", "team-parent", "team-grandchild"} {
		if _, ok := scope.readOrgIDs[id]; !ok {
			t.Fatalf("read scope missing %s", id)
		}
	}
	if _, ok := scope.manageOrgIDs["team-parent"]; ok {
		t.Fatal("manage scope should not include ancestor orgs automatically")
	}
}

func TestSameTenantResourceAuthorizer_ManageScopeUsesAdminSubtreeOnly(t *testing.T) {
	authorizer := newSameTenantResourceAuthorizer(&stubSameTenantOrgTreeService{
		userOrgs: []*types.OrgTreeNode{
			{ID: "team-admin", Path: "/root/team-admin", MyIsAdmin: true},
			{ID: "team-member", Path: "/root/team-member", MyIsAdmin: false},
		},
		ancestorIDs:        []string{"root"},
		allDescendantIDs:   []string{"team-admin-child", "team-member-child"},
		adminDescendantIDs: []string{"team-admin-child"},
	})

	scope, err := authorizer.resolveScope(context.Background(), "u1", 1)
	if err != nil {
		t.Fatalf("resolveScope err = %v", err)
	}

	if _, ok := scope.readOrgIDs["root"]; !ok {
		t.Fatal("expected ancestor org in read scope")
	}
	if _, ok := scope.manageOrgIDs["team-admin-child"]; !ok {
		t.Fatal("expected admin descendant org in manage scope")
	}
	if _, ok := scope.manageOrgIDs["team-member-child"]; ok {
		t.Fatal("non-admin descendant leaked into manage scope")
	}

	canReadParent := authorizer.canReadResource(sameTenantResourceRule{
		Visibility:     types.KBVisibilityOrg,
		OrganizationID: "root",
	}, "u1", false, scope)
	if !canReadParent {
		t.Fatal("expected child org member to read ancestor-scoped resource")
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