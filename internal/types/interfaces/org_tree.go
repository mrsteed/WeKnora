package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// OrgTreeService defines the organization tree management service
type OrgTreeService interface {
	// CreateNode creates a new organization tree node under a tenant
	CreateNode(ctx context.Context, tenantID uint64, userID string, req *types.CreateOrgTreeNodeRequest) (*types.Organization, error)
	// UpdateNode updates an org-tree node's metadata
	UpdateNode(ctx context.Context, nodeID string, tenantID uint64, req *types.UpdateOrgTreeNodeRequest) (*types.Organization, error)
	// DeleteNode deletes an org-tree node (must be a leaf or re-parent children)
	DeleteNode(ctx context.Context, nodeID string, tenantID uint64) error
	// MoveNode moves a node to a new parent (updates path for self and all descendants)
	MoveNode(ctx context.Context, nodeID string, tenantID uint64, req *types.MoveOrgNodeRequest) error
	// GetTree returns the full organization tree for a tenant
	GetTree(ctx context.Context, tenantID uint64) ([]*types.OrgTreeNode, error)
	// GetNode returns a single tree node by ID
	GetNode(ctx context.Context, nodeID string, tenantID uint64) (*types.Organization, error)

	// GetOrgAndDescendantIDs returns the given org ID plus all descendant org IDs (for visibility queries)
	GetOrgAndDescendantIDs(ctx context.Context, orgID string, tenantID uint64) ([]string, error)
	// GetOrgAndAncestorIDs returns the given org ID plus all ancestor org IDs
	GetOrgAndAncestorIDs(ctx context.Context, orgID string, tenantID uint64) ([]string, error)
	// GetDescendantIDsByPaths returns all descendant org IDs for multiple path prefixes within a tenant (batch optimization)
	GetDescendantIDsByPaths(ctx context.Context, pathPrefixes []string, tenantID uint64) ([]string, error)

	// AssignUserToOrg assigns a user to an organization with a given role
	AssignUserToOrg(ctx context.Context, orgID string, tenantID uint64, req *types.AssignUserToOrgRequest) error
	// RemoveUserFromOrg removes a user from an organization (with tenant validation)
	RemoveUserFromOrg(ctx context.Context, orgID string, tenantID uint64, req *types.RemoveUserFromOrgRequest) error
	// SetOrgAdmin sets or unsets a user as organization admin
	SetOrgAdmin(ctx context.Context, orgID string, tenantID uint64, req *types.SetOrgAdminRequest) error
	// GetUserOrganizations returns all org-tree organizations a user belongs to (within a tenant)
	GetUserOrganizations(ctx context.Context, userID string, tenantID uint64) ([]*types.Organization, error)
	// ListOrgMembers returns all members of an organization
	ListOrgMembers(ctx context.Context, orgID string, tenantID uint64) ([]*types.OrganizationMember, error)
}

// OrgTreeRepository defines the repository interface for org-tree operations
type OrgTreeRepository interface {
	// GetByIDAndTenant gets an organization by ID within a specific tenant
	GetByIDAndTenant(ctx context.Context, id string, tenantID uint64) (*types.Organization, error)
	// ListByTenantID lists all organizations belonging to a tenant (org-tree nodes)
	ListByTenantID(ctx context.Context, tenantID uint64) ([]*types.Organization, error)
	// GetChildren returns direct children of an organization
	GetChildren(ctx context.Context, parentID string) ([]*types.Organization, error)
	// GetDescendantsByPath returns all descendants by matching path prefix
	GetDescendantsByPath(ctx context.Context, pathPrefix string) ([]*types.Organization, error)
	// GetDescendantsByPathAndTenant returns all descendants by matching path prefix within a tenant
	GetDescendantsByPathAndTenant(ctx context.Context, pathPrefix string, tenantID uint64) ([]*types.Organization, error)
	// GetDescendantsByPathsAndTenant returns all descendants matching any of the path prefixes within a tenant
	GetDescendantsByPathsAndTenant(ctx context.Context, pathPrefixes []string, tenantID uint64) ([]*types.Organization, error)
	// UpdatePath updates the path and level for an organization
	UpdatePath(ctx context.Context, id string, path string, level int) error
	// UpdatePathBatch updates path and level for multiple organizations (for move operations)
	UpdatePathBatch(ctx context.Context, oldPathPrefix string, newPathPrefix string, levelDelta int) error
	// GetByIDs returns organizations by a list of IDs
	GetByIDs(ctx context.Context, ids []string) ([]*types.Organization, error)
	// MoveNodeInTx atomically updates a node's path/level, its descendants' paths/levels, and its parent_id/sort_order in a single transaction
	MoveNodeInTx(ctx context.Context, nodeID string, newPath string, newLevel int, oldPathPrefix string, levelDelta int, parentID *string, sortOrder int) error
}
