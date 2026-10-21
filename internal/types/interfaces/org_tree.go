package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// OrgTreeService defines organization tree management operations.
type OrgTreeService interface {
	CreateNode(ctx context.Context, tenantID uint64, userID string, req *types.CreateOrgTreeNodeRequest) (*types.Organization, error)
	UpdateNode(ctx context.Context, nodeID string, tenantID uint64, req *types.UpdateOrgTreeNodeRequest) (*types.Organization, error)
	DeleteNode(ctx context.Context, nodeID string, tenantID uint64) error
	MoveNode(ctx context.Context, nodeID string, tenantID uint64, req *types.MoveOrgNodeRequest) error
	GetTree(ctx context.Context, tenantID uint64) ([]*types.OrgTreeNode, error)
	GetTreeForUser(ctx context.Context, userID string, tenantID uint64, isSuperAdmin bool) ([]*types.OrgTreeNode, error)
	GetNode(ctx context.Context, nodeID string, tenantID uint64) (*types.Organization, error)
	GetOrgAndDescendantIDs(ctx context.Context, orgID string, tenantID uint64) ([]string, error)
	GetOrgAndAncestorIDs(ctx context.Context, orgID string, tenantID uint64) ([]string, error)
	GetUserOrganizations(ctx context.Context, userID string, tenantID uint64) ([]*types.OrgTreeNode, error)
	GetDescendantIDsByPaths(ctx context.Context, pathPrefixes []string, tenantID uint64) ([]string, error)
	GetAncestorIDsFromPaths(paths []string) []string
	AssignUserToOrg(ctx context.Context, orgID string, tenantID uint64, req *types.AssignUserToOrgRequest) error
	RemoveUserFromOrg(ctx context.Context, orgID string, tenantID uint64, req *types.RemoveUserFromOrgRequest) error
	SetOrgAdmin(ctx context.Context, orgID string, tenantID uint64, req *types.SetOrgAdminRequest) error
	ListOrgMembers(ctx context.Context, orgID string, tenantID uint64) ([]*types.OrganizationMember, error)
	IsAdminOfAnyOrg(ctx context.Context, userID string, orgIDs []string, tenantID uint64) bool
	ListInheritedAdmins(ctx context.Context, orgID string, tenantID uint64) ([]map[string]interface{}, error)
}

// OrgTreeRepository defines persistence operations used by the local user-scoped org tree.
type OrgTreeRepository interface {
	Create(ctx context.Context, org *types.Organization) error
	Update(ctx context.Context, org *types.Organization) error
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*types.Organization, error)
	GetByIDAndTenant(ctx context.Context, id string, tenantID uint64) (*types.Organization, error)
	ListByTenantID(ctx context.Context, tenantID uint64) ([]*types.Organization, error)
	GetChildren(ctx context.Context, parentID string) ([]*types.Organization, error)
	GetDescendantsByPath(ctx context.Context, pathPrefix string) ([]*types.Organization, error)
	GetDescendantsByPathAndTenant(ctx context.Context, pathPrefix string, tenantID uint64) ([]*types.Organization, error)
	GetDescendantsByPathsAndTenant(ctx context.Context, pathPrefixes []string, tenantID uint64) ([]*types.Organization, error)
	UpdatePath(ctx context.Context, id string, path string, level int) error
	UpdatePathBatch(ctx context.Context, oldPathPrefix string, newPathPrefix string, levelDelta int) error
	GetByIDs(ctx context.Context, ids []string) ([]*types.Organization, error)
	MoveNodeInTx(ctx context.Context, nodeID string, newPath string, newLevel int, oldPathPrefix string, levelDelta int, parentID *string, sortOrder int) error
	AddOrgTreeMember(ctx context.Context, member *types.OrganizationMember) error
	RemoveOrgTreeMember(ctx context.Context, orgID string, userID string) error
	UpdateOrgTreeMemberRole(ctx context.Context, orgID string, userID string, role types.OrgMemberRole) error
	ListOrgTreeMembers(ctx context.Context, orgID string) ([]*types.OrganizationMember, error)
	ListOrgTreeOrganizationsByUserID(ctx context.Context, userID string) ([]*types.Organization, error)
	BatchCountOrgTreeMembers(ctx context.Context, orgIDs []string) (map[string]int, error)
	BatchListOrgTreeMemberUserIDs(ctx context.Context, orgIDs []string) (map[string][]string, error)
	IsAdminOfAnyOrgTree(ctx context.Context, userID string, orgIDs []string, tenantID uint64) bool
}
