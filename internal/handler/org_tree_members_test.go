package handler

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

func TestBuildDirectOrgMemberEntry_UsesSubOrgAdminDisplayRole(t *testing.T) {
	entry := buildDirectOrgMemberEntry(&types.OrganizationMember{
		UserID:    "u-1",
		Role:      types.OrgRoleAdmin,
		CreatedAt: time.Unix(123, 0).UTC(),
		User: &types.User{
			ID:       "u-1",
			Username: "alice",
			Email:    "alice@example.com",
		},
	})

	if got := entry["display_role"]; got != orgTreeDisplayRoleSubOrgAdmin {
		t.Fatalf("display_role = %v, want %s", got, orgTreeDisplayRoleSubOrgAdmin)
	}
	if got := entry["role_scope"]; got != orgTreeRoleScopeSubOrgAdmin {
		t.Fatalf("role_scope = %v, want %s", got, orgTreeRoleScopeSubOrgAdmin)
	}
	if got := entry["can_manage_personnel"]; got != true {
		t.Fatalf("can_manage_personnel = %v, want true", got)
	}
	if got := entry["can_manage_resources"]; got != true {
		t.Fatalf("can_manage_resources = %v, want true", got)
	}
	if got := entry["is_owner"]; got != false {
		t.Fatalf("is_owner = %v, want false", got)
	}
}

func TestBuildProjectedRootMemberEntry_UsesWorkspaceOwnerProjection(t *testing.T) {
	entry := buildProjectedRootMemberEntry(&types.TenantMember{
		UserID:   "u-owner",
		Role:     types.TenantRoleOwner,
		JoinedAt: time.Unix(456, 0).UTC(),
	}, &types.User{ID: "u-owner", Username: "owner", Email: "owner@example.com"})

	if got := entry["display_role"]; got != orgTreeDisplayRoleOwner {
		t.Fatalf("display_role = %v, want %s", got, orgTreeDisplayRoleOwner)
	}
	if got := entry["role_scope"]; got != orgTreeRoleScopeWorkspaceOwner {
		t.Fatalf("role_scope = %v, want %s", got, orgTreeRoleScopeWorkspaceOwner)
	}
	if got := entry["source"]; got != orgTreeMemberSourceTenantProjection {
		t.Fatalf("source = %v, want %s", got, orgTreeMemberSourceTenantProjection)
	}
	if got := entry["is_projected_root_admin"]; got != true {
		t.Fatalf("is_projected_root_admin = %v, want true", got)
	}
	if got := entry["is_owner"]; got != true {
		t.Fatalf("is_owner = %v, want true", got)
	}
}

func TestDecorateInheritedAdminEntries_AddsDisplayMetadata(t *testing.T) {
	entries := decorateInheritedAdminEntries([]map[string]interface{}{{
		"user_id": "u-inherited",
		"role":    "admin",
	}})

	if got := entries[0]["display_role"]; got != orgTreeDisplayRoleSubOrgAdmin {
		t.Fatalf("display_role = %v, want %s", got, orgTreeDisplayRoleSubOrgAdmin)
	}
	if got := entries[0]["role_scope"]; got != orgTreeRoleScopeInheritedAdmin {
		t.Fatalf("role_scope = %v, want %s", got, orgTreeRoleScopeInheritedAdmin)
	}
	if got := entries[0]["can_manage_resources"]; got != true {
		t.Fatalf("can_manage_resources = %v, want true", got)
	}
}

func TestIsOrgAdminOf_TenantOwnerBypassesMembershipChecks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(resp)
	ctx := context.WithValue(context.Background(), types.TenantRoleContextKey, types.TenantRoleOwner)
	c.Request = httptest.NewRequest("GET", "/", nil).WithContext(ctx)

	h := &OrgTreeHandler{}
	if !h.isOrgAdminOf(c, &types.User{ID: "u-owner"}, "org-1", 1) {
		t.Fatal("expected tenant owner to bypass explicit org-tree membership checks")
	}
}