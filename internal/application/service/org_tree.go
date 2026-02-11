package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

// orgTreeService implements OrgTreeService interface
type orgTreeService struct {
	orgTreeRepo interfaces.OrgTreeRepository
	orgRepo     interfaces.OrganizationRepository
}

// NewOrgTreeService creates a new organization tree service
func NewOrgTreeService(
	orgTreeRepo interfaces.OrgTreeRepository,
	orgRepo interfaces.OrganizationRepository,
) interfaces.OrgTreeService {
	return &orgTreeService{
		orgTreeRepo: orgTreeRepo,
		orgRepo:     orgRepo,
	}
}

// CreateNode creates a new organization tree node under a tenant
func (s *orgTreeService) CreateNode(ctx context.Context, tenantID uint64, userID string, req *types.CreateOrgTreeNodeRequest) (*types.Organization, error) {
	logger.Infof(ctx, "Creating org-tree node: %s under tenant: %d by user: %s", req.Name, tenantID, userID)

	org := &types.Organization{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		OwnerID:     userID,
		ParentID:    req.ParentID,
		SortOrder:   req.SortOrder,
		OrgTenantID: &tenantID,
	}

	// Calculate path and level
	if req.ParentID != nil && *req.ParentID != "" {
		parent, err := s.orgTreeRepo.GetByIDAndTenant(ctx, *req.ParentID, tenantID)
		if err != nil {
			logger.Errorf(ctx, "Failed to get parent node %s: %v", *req.ParentID, err)
			return nil, fmt.Errorf("parent node not found: %w", err)
		}
		org.Path = parent.Path + "/" + org.ID
		org.Level = parent.Level + 1
	} else {
		// Top-level node
		org.Path = "/" + org.ID
		org.Level = 1
		org.ParentID = nil
	}

	if err := s.orgRepo.Create(ctx, org); err != nil {
		logger.Errorf(ctx, "Failed to create org-tree node: %v", err)
		return nil, fmt.Errorf("failed to create org-tree node: %w", err)
	}

	logger.Infof(ctx, "Org-tree node created: %s (path: %s)", org.ID, org.Path)
	return org, nil
}

// UpdateNode updates an org-tree node's metadata
func (s *orgTreeService) UpdateNode(ctx context.Context, nodeID string, tenantID uint64, req *types.UpdateOrgTreeNodeRequest) (*types.Organization, error) {
	logger.Infof(ctx, "Updating org-tree node: %s", nodeID)

	org, err := s.orgTreeRepo.GetByIDAndTenant(ctx, nodeID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}

	if req.Name != nil {
		org.Name = *req.Name
	}
	if req.Description != nil {
		org.Description = *req.Description
	}
	if req.SortOrder != nil {
		org.SortOrder = *req.SortOrder
	}

	if err := s.orgRepo.Update(ctx, org); err != nil {
		logger.Errorf(ctx, "Failed to update org-tree node: %v", err)
		return nil, fmt.Errorf("failed to update org-tree node: %w", err)
	}

	return org, nil
}

// DeleteNode deletes an org-tree node (must be a leaf node)
func (s *orgTreeService) DeleteNode(ctx context.Context, nodeID string, tenantID uint64) error {
	logger.Infof(ctx, "Deleting org-tree node: %s", nodeID)

	org, err := s.orgTreeRepo.GetByIDAndTenant(ctx, nodeID, tenantID)
	if err != nil {
		return fmt.Errorf("node not found: %w", err)
	}

	// Check if node has children
	children, err := s.orgTreeRepo.GetChildren(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("failed to check children: %w", err)
	}
	if len(children) > 0 {
		return fmt.Errorf("cannot delete node with children; move or delete children first")
	}

	if err := s.orgRepo.Delete(ctx, org.ID); err != nil {
		logger.Errorf(ctx, "Failed to delete org-tree node: %v", err)
		return fmt.Errorf("failed to delete org-tree node: %w", err)
	}

	logger.Infof(ctx, "Org-tree node deleted: %s", nodeID)
	return nil
}

// MoveNode moves a node to a new parent (updates path for self and all descendants)
func (s *orgTreeService) MoveNode(ctx context.Context, nodeID string, tenantID uint64, req *types.MoveOrgNodeRequest) error {
	logger.Infof(ctx, "Moving org-tree node: %s", nodeID)

	org, err := s.orgTreeRepo.GetByIDAndTenant(ctx, nodeID, tenantID)
	if err != nil {
		return fmt.Errorf("node not found: %w", err)
	}

	oldPath := org.Path
	var newPath string
	var newLevel int

	if req.NewParentID != nil && *req.NewParentID != "" {
		// Prevent moving to own descendant
		newParent, err := s.orgTreeRepo.GetByIDAndTenant(ctx, *req.NewParentID, tenantID)
		if err != nil {
			return fmt.Errorf("new parent not found: %w", err)
		}
		if strings.HasPrefix(newParent.Path, oldPath+"/") || newParent.ID == nodeID {
			return fmt.Errorf("cannot move node to its own descendant")
		}
		newPath = newParent.Path + "/" + org.ID
		newLevel = newParent.Level + 1
	} else {
		// Move to top-level
		newPath = "/" + org.ID
		newLevel = 1
	}

	levelDelta := newLevel - org.Level

	// Atomically update self path/level, descendants, and parent_id/sort_order in a single transaction
	if err := s.orgTreeRepo.MoveNodeInTx(ctx, nodeID, newPath, newLevel, oldPath, levelDelta, req.NewParentID, req.SortOrder); err != nil {
		logger.Errorf(ctx, "Failed to move node %s in transaction: %v", nodeID, err)
		return fmt.Errorf("failed to move node: %w", err)
	}

	logger.Infof(ctx, "Org-tree node moved: %s from %s to %s", nodeID, oldPath, newPath)
	return nil
}

// GetTree returns the full organization tree for a tenant
func (s *orgTreeService) GetTree(ctx context.Context, tenantID uint64) ([]*types.OrgTreeNode, error) {
	logger.Infof(ctx, "Getting org tree for tenant: %d", tenantID)

	orgs, err := s.orgTreeRepo.ListByTenantID(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list org nodes: %w", err)
	}

	// Build member count map using batch query to avoid N+1
	orgIDs := make([]string, len(orgs))
	for i, org := range orgs {
		orgIDs[i] = org.ID
	}
	memberCounts, err := s.orgRepo.BatchCountMembers(ctx, orgIDs)
	if err != nil {
		logger.Errorf(ctx, "Failed to batch count members, falling back to per-org counting: %v", err)
		// Fallback: count individually
		memberCounts = make(map[string]int)
		for _, org := range orgs {
			count, err := s.orgRepo.CountMembers(ctx, org.ID)
			if err != nil {
				logger.Errorf(ctx, "Failed to count members for org %s: %v", org.ID, err)
				memberCounts[org.ID] = 0
			} else {
				memberCounts[org.ID] = int(count)
			}
		}
	}

	// Convert to tree nodes
	nodeMap := make(map[string]*types.OrgTreeNode)
	var roots []*types.OrgTreeNode

	for _, org := range orgs {
		node := &types.OrgTreeNode{
			ID:          org.ID,
			Name:        org.Name,
			Description: org.Description,
			ParentID:    org.ParentID,
			Path:        org.Path,
			Level:       org.Level,
			SortOrder:   org.SortOrder,
			MemberCount: memberCounts[org.ID],
			CreatedAt:   org.CreatedAt,
			UpdatedAt:   org.UpdatedAt,
		}
		nodeMap[org.ID] = node
	}

	// Build tree structure
	for _, node := range nodeMap {
		if node.ParentID == nil || *node.ParentID == "" {
			roots = append(roots, node)
		} else {
			if parent, ok := nodeMap[*node.ParentID]; ok {
				parent.Children = append(parent.Children, node)
			} else {
				// Orphaned node, add to roots
				roots = append(roots, node)
			}
		}
	}

	return roots, nil
}

// GetNode returns a single tree node by ID
func (s *orgTreeService) GetNode(ctx context.Context, nodeID string, tenantID uint64) (*types.Organization, error) {
	return s.orgTreeRepo.GetByIDAndTenant(ctx, nodeID, tenantID)
}

// GetOrgAndDescendantIDs returns the given org ID plus all descendant org IDs
func (s *orgTreeService) GetOrgAndDescendantIDs(ctx context.Context, orgID string, tenantID uint64) ([]string, error) {
	org, err := s.orgTreeRepo.GetByIDAndTenant(ctx, orgID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("org not found: %w", err)
	}

	ids := []string{orgID}
	descendants, err := s.orgTreeRepo.GetDescendantsByPathAndTenant(ctx, org.Path, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get descendants: %w", err)
	}

	for _, d := range descendants {
		ids = append(ids, d.ID)
	}
	return ids, nil
}

// GetOrgAndAncestorIDs returns the given org ID plus all ancestor org IDs
func (s *orgTreeService) GetOrgAndAncestorIDs(ctx context.Context, orgID string, tenantID uint64) ([]string, error) {
	org, err := s.orgTreeRepo.GetByIDAndTenant(ctx, orgID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("org not found: %w", err)
	}

	// Parse ancestor IDs from path (e.g., /root_id/parent_id/self_id)
	parts := strings.Split(strings.TrimPrefix(org.Path, "/"), "/")
	return parts, nil
}

// GetDescendantIDsByPaths returns all descendant org IDs for multiple path prefixes within a tenant (batch optimization)
func (s *orgTreeService) GetDescendantIDsByPaths(ctx context.Context, pathPrefixes []string, tenantID uint64) ([]string, error) {
	if len(pathPrefixes) == 0 {
		return nil, nil
	}

	descendants, err := s.orgTreeRepo.GetDescendantsByPathsAndTenant(ctx, pathPrefixes, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to batch get descendants: %w", err)
	}

	ids := make([]string, 0, len(descendants))
	for _, d := range descendants {
		ids = append(ids, d.ID)
	}
	return ids, nil
}

// AssignUserToOrg assigns a user to an organization with a given role
func (s *orgTreeService) AssignUserToOrg(ctx context.Context, orgID string, tenantID uint64, req *types.AssignUserToOrgRequest) error {
	logger.Infof(ctx, "Assigning user %s to org %s with role %s", req.UserID, orgID, req.Role)

	// Verify org exists and belongs to tenant
	_, err := s.orgTreeRepo.GetByIDAndTenant(ctx, orgID, tenantID)
	if err != nil {
		return fmt.Errorf("organization not found: %w", err)
	}

	if !req.Role.IsValid() {
		return fmt.Errorf("invalid role: %s", req.Role)
	}

	member := &types.OrganizationMember{
		ID:             uuid.New().String(),
		OrganizationID: orgID,
		UserID:         req.UserID,
		Role:           req.Role,
		TenantID:       tenantID,
	}

	if err := s.orgRepo.AddMember(ctx, member); err != nil {
		if err == repository.ErrOrgMemberAlreadyExists {
			// Update existing member's role
			if err := s.orgRepo.UpdateMemberRole(ctx, orgID, req.UserID, req.Role); err != nil {
				return fmt.Errorf("failed to update member role: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to add member: %w", err)
	}

	return nil
}

// RemoveUserFromOrg removes a user from an organization
func (s *orgTreeService) RemoveUserFromOrg(ctx context.Context, orgID string, tenantID uint64, req *types.RemoveUserFromOrgRequest) error {
	logger.Infof(ctx, "Removing user %s from org %s", req.UserID, orgID)

	// Verify the org belongs to the tenant
	_, err := s.orgTreeRepo.GetByIDAndTenant(ctx, orgID, tenantID)
	if err != nil {
		return fmt.Errorf("org not found or not in tenant: %w", err)
	}

	if err := s.orgRepo.RemoveMember(ctx, orgID, req.UserID); err != nil {
		return fmt.Errorf("failed to remove member: %w", err)
	}
	return nil
}

// SetOrgAdmin sets or unsets a user as organization admin
func (s *orgTreeService) SetOrgAdmin(ctx context.Context, orgID string, tenantID uint64, req *types.SetOrgAdminRequest) error {
	logger.Infof(ctx, "Setting org admin: user %s, org %s, isAdmin %v", req.UserID, orgID, req.IsAdmin)

	// Verify the org belongs to this tenant (prevents cross-tenant privilege escalation)
	_, err := s.orgTreeRepo.GetByIDAndTenant(ctx, orgID, tenantID)
	if err != nil {
		return fmt.Errorf("organization not found in this tenant: %w", err)
	}

	var role types.OrgMemberRole
	if req.IsAdmin {
		role = types.OrgRoleAdmin
	} else {
		role = types.OrgRoleViewer
	}

	if err := s.orgRepo.UpdateMemberRole(ctx, orgID, req.UserID, role); err != nil {
		return fmt.Errorf("failed to update member role: %w", err)
	}
	return nil
}

// GetUserOrganizations returns all org-tree organizations a user belongs to (within a tenant)
func (s *orgTreeService) GetUserOrganizations(ctx context.Context, userID string, tenantID uint64) ([]*types.Organization, error) {
	logger.Infof(ctx, "Getting organizations for user %s in tenant %d", userID, tenantID)

	// Get all orgs user is a member of
	allOrgs, err := s.orgRepo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list user organizations: %w", err)
	}

	// Filter to only those belonging to the tenant (org-tree nodes have OrgTenantID set)
	var result []*types.Organization
	for _, org := range allOrgs {
		if org.OrgTenantID != nil && *org.OrgTenantID == tenantID {
			result = append(result, org)
		}
	}

	return result, nil
}

// ListOrgMembers returns all members of an organization (after verifying it belongs to the tenant)
func (s *orgTreeService) ListOrgMembers(ctx context.Context, orgID string, tenantID uint64) ([]*types.OrganizationMember, error) {
	logger.Infof(ctx, "Listing members for org %s in tenant %d", orgID, tenantID)

	// Verify the org belongs to the tenant
	_, err := s.orgTreeRepo.GetByIDAndTenant(ctx, orgID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("org not found or not in tenant: %w", err)
	}

	members, err := s.orgRepo.ListMembers(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list org members: %w", err)
	}
	return members, nil
}
