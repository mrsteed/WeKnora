package types

import "time"

// OrgTreeNode is the user-scoped org-tree projection used by local same-tenant
// visibility and personnel-management flows.
type OrgTreeNode struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Description       string  `json:"description"`
	ParentID          *string `json:"parent_id,omitempty"`
	Path              string  `json:"path"`
	Level             int     `json:"level"`
	SortOrder         int     `json:"sort_order"`
	DirectMemberCount int64   `json:"direct_member_count"`
	MemberCount       int64   `json:"member_count"`
	TotalMemberCount  int64   `json:"total_member_count"`
	MyIsAdmin         bool    `json:"my_is_admin"`
	// MyRoleInOrg is the current user's member role in this organization
	// (admin/editor/viewer). Empty when the user has no direct membership.
	MyRoleInOrg string         `json:"my_role_in_org,omitempty"`
	Children    []*OrgTreeNode `json:"children,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}
