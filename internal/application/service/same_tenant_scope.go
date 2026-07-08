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
	readOrgIDs   map[string]struct{}
	manageOrgIDs map[string]struct{}
	readOrgList  []string
}

type sameTenantResourceAuthorizer struct {
	orgTreeService interfaces.OrgTreeService
}

func newSameTenantResourceAuthorizer(orgTreeService interfaces.OrgTreeService) *sameTenantResourceAuthorizer {
	return &sameTenantResourceAuthorizer{orgTreeService: orgTreeService}
}

func (a *sameTenantResourceAuthorizer) resolveScope(ctx context.Context, userID string, tenantID uint64) (*sameTenantOrgScope, error) {
	if a == nil || a.orgTreeService == nil || strings.TrimSpace(userID) == "" || tenantID == 0 {
		return &sameTenantOrgScope{
			readOrgIDs:   map[string]struct{}{},
			manageOrgIDs: map[string]struct{}{},
			readOrgList:  []string{},
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

	ancestorIDs := a.orgTreeService.GetAncestorIDsFromPaths(allPathPrefixes)

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

	return buildSameTenantOrgScope(readBaseIDs, ancestorIDs, allDescendantIDs, manageBaseIDs, adminDescendantIDs), nil
}

func buildSameTenantOrgScope(
	readBaseIDs []string,
	ancestorIDs []string,
	allDescendantIDs []string,
	manageBaseIDs []string,
	adminDescendantIDs []string,
) *sameTenantOrgScope {
	readSet := make(map[string]struct{})
	manageSet := make(map[string]struct{})

	for _, id := range readBaseIDs {
		if id = strings.TrimSpace(id); id != "" {
			readSet[id] = struct{}{}
		}
	}
	for _, id := range ancestorIDs {
		if id = strings.TrimSpace(id); id != "" {
			readSet[id] = struct{}{}
		}
	}
	for _, id := range allDescendantIDs {
		if id = strings.TrimSpace(id); id != "" {
			readSet[id] = struct{}{}
		}
	}

	for _, id := range manageBaseIDs {
		if id = strings.TrimSpace(id); id != "" {
			manageSet[id] = struct{}{}
		}
	}
	for _, id := range adminDescendantIDs {
		if id = strings.TrimSpace(id); id != "" {
			manageSet[id] = struct{}{}
		}
	}

	readList := make([]string, 0, len(readSet))
	for id := range readSet {
		readList = append(readList, id)
	}
	sort.Strings(readList)

	return &sameTenantOrgScope{
		readOrgIDs:   readSet,
		manageOrgIDs: manageSet,
		readOrgList:  readList,
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
			readOrgIDs:   map[string]struct{}{},
			manageOrgIDs: map[string]struct{}{},
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
			readOrgIDs:   map[string]struct{}{},
			manageOrgIDs: map[string]struct{}{},
		}
	}
	visibility := normalizeScopedResourceVisibility(rule.Visibility)
	if visibility != types.KBVisibilityOrg {
		return false
	}
	_, ok := scope.manageOrgIDs[strings.TrimSpace(rule.OrganizationID)]
	return ok
}

func normalizeScopedResourceVisibility(raw string) string {
	visibility := strings.TrimSpace(raw)
	if visibility == "" {
		return types.KBVisibilityGlobal
	}
	return visibility
}