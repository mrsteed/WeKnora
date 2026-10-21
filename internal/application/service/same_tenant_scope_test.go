package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestSameTenantResourceAuthorizer_CreatorCanReadOwnOrgScopedResource(t *testing.T) {
	authorizer := &sameTenantResourceAuthorizer{}
	scope := &sameTenantOrgScope{
		readOrgIDs:            map[string]struct{}{},
		personnelManageOrgIDs: map[string]struct{}{},
		resourceManageOrgIDs:  map[string]struct{}{},
	}

	allowed := authorizer.canReadResource(sameTenantResourceRule{
		Visibility:     types.KBVisibilityOrg,
		OrganizationID: "org-cross-tenant",
		CreatedBy:      "user-1",
	}, "user-1", false, scope)
	if !allowed {
		t.Fatal("expected creator to retain read access to own org-scoped resource")
	}
}
