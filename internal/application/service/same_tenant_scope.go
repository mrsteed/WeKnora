package service

import (
	"context"
	"sort"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type sameTenantResourceRule struct {
	Visibility     string
	OrganizationID string
	CreatedBy      string
}

type sameTenantOrgScope struct {
	readOrgIDs             map[string]struct{}
	personnelManageOrgIDs  map[string]struct{}
	resourceManageOrgIDs   map[string]struct{}
	readOrgList            []string
	isTenantAdminOrOwner   bool
}

type sameTenantResourceAuthorizer struct {
	orgTreeService interfaces.OrgTreeService
}

func userHasExplicitRootAdminMembership(userOrgs []*types.OrgTreeNode) bool {
	for _, org := range userOrgs {
		if org == nil || !org.MyIsAdmin {
			continue
		}
		if org.Level <= 1 {
			return true
		}
	}
	return false
}

func newSameTenantResourceAuthorizer(orgTreeService interfaces.OrgTreeService) *sameTenantResourceAuthorizer {
	return &sameTenantResourceAuthorizer{orgTreeService: orgTreeService}
}

func (a *sameTenantResourceAuthorizer) resolveScope(ctx context.Context, userID string, tenantID uint64) (*sameTenantOrgScope, error) {
	if a == nil || a.orgTreeService == nil || strings.TrimSpace(userID) == "" || tenantID == 0 {
		return &sameTenantOrgScope{
			readOrgIDs:            map[string]struct{}{},
			personnelManageOrgIDs: map[string]struct{}{},
			resourceManageOrgIDs:  map[string]struct{}{},
			readOrgList:           []string{},
		}, nil
	}

	userOrgs, err := a.orgTreeService.GetUserOrganizations(ctx, userID, tenantID)
	if err != nil {
		return nil, err
	}

	allPathPrefixes := make([]string, 0, len(userOrgs))
	adminPathPrefixes := make([]string, 0, len(userOrgs))
	readBaseIDs := make([]string, 0, len(userOrgs))
	manageBaseIDs := make([]string, 0, len(userOrgs))

	for _, org := range userOrgs {
		if org == nil || strings.TrimSpace(org.ID) == "" {
			continue
		}
		readBaseIDs = append(readBaseIDs, org.ID)
		if strings.TrimSpace(org.Path) != "" {
			allPathPrefixes = append(allPathPrefixes, org.Path)
		}
		if org.MyIsAdmin {
			manageBaseIDs = append(manageBaseIDs, org.ID)
			if strings.TrimSpace(org.Path) != "" {
				adminPathPrefixes = append(adminPathPrefixes, org.Path)
			}
		}
	}

	allDescendantIDs := make([]string, 0)
	if len(allPathPrefixes) > 0 {
		ids, err := a.orgTreeService.GetDescendantIDsByPaths(ctx, allPathPrefixes, tenantID)
		if err != nil {
			return nil, err
		}
		allDescendantIDs = ids
	}

	adminDescendantIDs := make([]string, 0)
	if len(adminPathPrefixes) > 0 {
		ids, err := a.orgTreeService.GetDescendantIDsByPaths(ctx, adminPathPrefixes, tenantID)
		if err != nil {
			return nil, err
		}
		adminDescendantIDs = ids
	}

	return buildSameTenantOrgScope(
		readBaseIDs,
		allDescendantIDs,
		manageBaseIDs,
		adminDescendantIDs,
		manageBaseIDs,
		adminDescendantIDs,
		types.TenantRoleFromContext(ctx) == types.TenantRoleOwner || userHasExplicitRootAdminMembership(userOrgs),
	), nil
}

func buildSameTenantOrgScope(
	readBaseIDs []string,
	allDescendantIDs []string,
	personnelManageBaseIDs []string,
	personnelDescendantIDs []string,
	resourceManageBaseIDs []string,
	resourceDescendantIDs []string,
	isTenantAdminOrOwner bool,
) *sameTenantOrgScope {
	readSet := make(map[string]struct{})
	personnelManageSet := make(map[string]struct{})
	resourceManageSet := make(map[string]struct{})

	for _, id := range readBaseIDs {
		if id = strings.TrimSpace(id); id != "" {
			readSet[id] = struct{}{}
		}
	}
	for _, id := range allDescendantIDs {
		if id = strings.TrimSpace(id); id != "" {
			readSet[id] = struct{}{}
		}
	}

	for _, id := range personnelManageBaseIDs {
		if id = strings.TrimSpace(id); id != "" {
			personnelManageSet[id] = struct{}{}
		}
	}
	for _, id := range personnelDescendantIDs {
		if id = strings.TrimSpace(id); id != "" {
			personnelManageSet[id] = struct{}{}
		}
	}

	for _, id := range resourceManageBaseIDs {
		if id = strings.TrimSpace(id); id != "" {
			resourceManageSet[id] = struct{}{}
		}
	}
	for _, id := range resourceDescendantIDs {
		if id = strings.TrimSpace(id); id != "" {
			resourceManageSet[id] = struct{}{}
		}
	}

	readList := make([]string, 0, len(readSet))
	for id := range readSet {
		readList = append(readList, id)
	}
	sort.Strings(readList)

	return &sameTenantOrgScope{
		readOrgIDs:            readSet,
		personnelManageOrgIDs: personnelManageSet,
		resourceManageOrgIDs:  resourceManageSet,
		readOrgList:           readList,
		isTenantAdminOrOwner:  isTenantAdminOrOwner,
	}
}

func (a *sameTenantResourceAuthorizer) canReadResource(
	rule sameTenantResourceRule,
	userID string,
	isPrivileged bool,
	scope *sameTenantOrgScope,
) bool {
	if isPrivileged {
		return true
	}
	if scope == nil {
		scope = &sameTenantOrgScope{
			readOrgIDs:            map[string]struct{}{},
			personnelManageOrgIDs: map[string]struct{}{},
			resourceManageOrgIDs:  map[string]struct{}{},
		}
	}
	visibility := normalizeScopedResourceVisibility(rule.Visibility)
	switch visibility {
	case types.KBVisibilityGlobal:
		return true
	case types.KBVisibilityPrivate:
		return rule.CreatedBy != "" && rule.CreatedBy == userID
	case types.KBVisibilityOrg:
		_, ok := scope.readOrgIDs[strings.TrimSpace(rule.OrganizationID)]
		return ok
	default:
		return false
	}
}

func (a *sameTenantResourceAuthorizer) canManageResource(
	rule sameTenantResourceRule,
	userID string,
	isPrivileged bool,
	scope *sameTenantOrgScope,
) bool {
	if isPrivileged {
		return true
	}
	if rule.CreatedBy != "" && rule.CreatedBy == userID {
		return true
	}
	if scope == nil {
		scope = &sameTenantOrgScope{
			readOrgIDs:            map[string]struct{}{},
			personnelManageOrgIDs: map[string]struct{}{},
			resourceManageOrgIDs:  map[string]struct{}{},
		}
	}
	if scope.isTenantAdminOrOwner {
		return true
	}
	visibility := normalizeScopedResourceVisibility(rule.Visibility)
	if visibility != types.KBVisibilityOrg {
		return false
	}
	_, ok := scope.resourceManageOrgIDs[strings.TrimSpace(rule.OrganizationID)]
	return ok
}

func normalizeScopedResourceVisibility(raw string) string {
	visibility := strings.TrimSpace(raw)
	if visibility == "" {
		return types.KBVisibilityGlobal
	}
	return visibility
}