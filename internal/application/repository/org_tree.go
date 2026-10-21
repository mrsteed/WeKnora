package repository

import (
	"context"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type orgTreeRepository struct {
	db *gorm.DB
}

type orgTreeMemberRow struct {
	ID             string
	OrganizationID string
	UserID         string
	TenantID       uint64
	Role           types.OrgMemberRole
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Username       string
	Email          string
	Phone          string
	Avatar         string
	IsSuperAdmin   bool
	IsSystemAdmin  bool
	IsOwner        bool
}

func buildOrgTreeMember(row orgTreeMemberRow) *types.OrganizationMember {
	member := &types.OrganizationMember{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		UserID:         row.UserID,
		TenantID:       row.TenantID,
		Role:           row.Role,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
		IsOwner:        row.IsOwner,
	}
	member.User = &types.User{
		ID:            row.UserID,
		Username:      row.Username,
		Email:         row.Email,
		Phone:         row.Phone,
		Avatar:        row.Avatar,
		IsSuperAdmin:  row.IsSuperAdmin,
		IsSystemAdmin: row.IsSystemAdmin,
	}
	return member
}

// NewOrgTreeRepository creates a repository for user-scoped org-tree operations.
func NewOrgTreeRepository(db *gorm.DB) interfaces.OrgTreeRepository {
	return &orgTreeRepository{db: db}
}

func (r *orgTreeRepository) Create(ctx context.Context, org *types.Organization) error {
	return r.db.WithContext(ctx).Create(org).Error
}

func (r *orgTreeRepository) Update(ctx context.Context, org *types.Organization) error {
	return r.db.WithContext(ctx).Model(&types.Organization{}).Where("id = ?", org.ID).
		Select("name", "description", "avatar", "require_approval", "searchable", "invite_code_validity_days", "member_limit", "parent_id", "sort_order", "updated_at").
		Updates(org).Error
}

func (r *orgTreeRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&types.Organization{}).Error
}

func (r *orgTreeRepository) GetByID(ctx context.Context, id string) (*types.Organization, error) {
	var org types.Organization
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&org).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrOrganizationNotFound
		}
		return nil, err
	}
	return &org, nil
}

func (r *orgTreeRepository) GetByIDAndTenant(ctx context.Context, id string, tenantID uint64) (*types.Organization, error) {
	var org types.Organization
	if err := r.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).First(&org).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrOrganizationNotFound
		}
		return nil, err
	}
	return &org, nil
}

func (r *orgTreeRepository) ListByTenantID(ctx context.Context, tenantID uint64) ([]*types.Organization, error) {
	var orgs []*types.Organization
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("level ASC, sort_order ASC, created_at ASC").
		Find(&orgs).Error; err != nil {
		return nil, err
	}
	return orgs, nil
}

func (r *orgTreeRepository) GetChildren(ctx context.Context, parentID string) ([]*types.Organization, error) {
	var orgs []*types.Organization
	if err := r.db.WithContext(ctx).
		Where("parent_id = ?", parentID).
		Order("sort_order ASC, created_at ASC").
		Find(&orgs).Error; err != nil {
		return nil, err
	}
	return orgs, nil
}

func (r *orgTreeRepository) GetDescendantsByPath(ctx context.Context, pathPrefix string) ([]*types.Organization, error) {
	var orgs []*types.Organization
	pattern := strings.TrimRight(pathPrefix, "/") + "/%"
	if err := r.db.WithContext(ctx).
		Where("path = ? OR path LIKE ?", pathPrefix, pattern).
		Order("level ASC, sort_order ASC, created_at ASC").
		Find(&orgs).Error; err != nil {
		return nil, err
	}
	return orgs, nil
}

func (r *orgTreeRepository) GetDescendantsByPathAndTenant(ctx context.Context, pathPrefix string, tenantID uint64) ([]*types.Organization, error) {
	var orgs []*types.Organization
	pattern := strings.TrimRight(pathPrefix, "/") + "/%"
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND (path = ? OR path LIKE ?)", tenantID, pathPrefix, pattern).
		Order("level ASC, sort_order ASC, created_at ASC").
		Find(&orgs).Error; err != nil {
		return nil, err
	}
	return orgs, nil
}

func (r *orgTreeRepository) GetDescendantsByPathsAndTenant(ctx context.Context, pathPrefixes []string, tenantID uint64) ([]*types.Organization, error) {
	if len(pathPrefixes) == 0 {
		return []*types.Organization{}, nil
	}
	query := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	for i, prefix := range pathPrefixes {
		pattern := strings.TrimRight(prefix, "/") + "/%"
		if i == 0 {
			query = query.Where("(path = ? OR path LIKE ?)", prefix, pattern)
		} else {
			query = query.Or("tenant_id = ? AND (path = ? OR path LIKE ?)", tenantID, prefix, pattern)
		}
	}
	var orgs []*types.Organization
	if err := query.Order("level ASC, sort_order ASC, created_at ASC").Find(&orgs).Error; err != nil {
		return nil, err
	}
	return orgs, nil
}

func (r *orgTreeRepository) UpdatePath(ctx context.Context, id string, path string, level int) error {
	return r.db.WithContext(ctx).Model(&types.Organization{}).Where("id = ?", id).Updates(map[string]interface{}{
		"path":  path,
		"level": level,
	}).Error
}

func (r *orgTreeRepository) UpdatePathBatch(ctx context.Context, oldPathPrefix string, newPathPrefix string, levelDelta int) error {
	pattern := strings.TrimRight(oldPathPrefix, "/") + "/%"
	var descendants []*types.Organization
	if err := r.db.WithContext(ctx).Where("path LIKE ?", pattern).Find(&descendants).Error; err != nil {
		return err
	}
	for _, org := range descendants {
		newPath := strings.Replace(org.Path, oldPathPrefix, newPathPrefix, 1)
		newLevel := org.Level + levelDelta
		if err := r.db.WithContext(ctx).Model(&types.Organization{}).Where("id = ?", org.ID).Updates(map[string]interface{}{
			"path":  newPath,
			"level": newLevel,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *orgTreeRepository) GetByIDs(ctx context.Context, ids []string) ([]*types.Organization, error) {
	if len(ids) == 0 {
		return []*types.Organization{}, nil
	}
	var orgs []*types.Organization
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&orgs).Error; err != nil {
		return nil, err
	}
	return orgs, nil
}

func (r *orgTreeRepository) MoveNodeInTx(ctx context.Context, nodeID string, newPath string, newLevel int, oldPathPrefix string, levelDelta int, parentID *string, sortOrder int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&types.Organization{}).Where("id = ?", nodeID).Updates(map[string]interface{}{
			"path":       newPath,
			"level":      newLevel,
			"parent_id":  parentID,
			"sort_order": sortOrder,
		}).Error; err != nil {
			return err
		}

		pattern := strings.TrimRight(oldPathPrefix, "/") + "/%"
		var descendants []*types.Organization
		if err := tx.Where("path LIKE ?", pattern).Find(&descendants).Error; err != nil {
			return err
		}
		for _, org := range descendants {
			updatedPath := strings.Replace(org.Path, oldPathPrefix, newPath, 1)
			updatedLevel := org.Level + levelDelta
			if err := tx.Model(&types.Organization{}).Where("id = ?", org.ID).Updates(map[string]interface{}{
				"path":  updatedPath,
				"level": updatedLevel,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *orgTreeRepository) AddOrgTreeMember(ctx context.Context, member *types.OrganizationMember) error {
	var count int64
	r.db.WithContext(ctx).
		Table("organization_members").
		Where("organization_id = ? AND user_id = ?", member.OrganizationID, member.UserID).
		Count(&count)
	if count > 0 {
		return ErrOrgMemberAlreadyExists
	}

	createdAt := member.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	updatedAt := member.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}

	return r.db.WithContext(ctx).Table("organization_members").Create(map[string]interface{}{
		"id":              member.ID,
		"organization_id": member.OrganizationID,
		"user_id":         member.UserID,
		"tenant_id":       member.TenantID,
		"role":            member.Role,
		"created_at":      createdAt,
		"updated_at":      updatedAt,
	}).Error
}

func (r *orgTreeRepository) RemoveOrgTreeMember(ctx context.Context, orgID string, userID string) error {
	result := r.db.WithContext(ctx).Table("organization_members").Where("organization_id = ? AND user_id = ?", orgID, userID).Delete(nil)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrOrgMemberNotFound
	}
	return nil
}

func (r *orgTreeRepository) UpdateOrgTreeMemberRole(ctx context.Context, orgID string, userID string, role types.OrgMemberRole) error {
	result := r.db.WithContext(ctx).Table("organization_members").Where("organization_id = ? AND user_id = ?", orgID, userID).Update("role", role)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrOrgMemberNotFound
	}
	return nil
}

func (r *orgTreeRepository) ListOrgTreeMembers(ctx context.Context, orgID string) ([]*types.OrganizationMember, error) {
	var rows []orgTreeMemberRow
	err := r.db.WithContext(ctx).
		Table("organization_members omp").
		Select("omp.id, omp.organization_id, omp.user_id, omp.tenant_id, omp.role, omp.created_at, omp.updated_at, u.username, u.email, u.phone, u.avatar, u.is_super_admin, u.is_system_admin, (o.owner_id = omp.user_id) AS is_owner").
		Joins("JOIN organizations o ON o.id = omp.organization_id").
		Joins("LEFT JOIN users u ON u.id = omp.user_id").
		Where("omp.organization_id = ?", orgID).
		Where("(u.id IS NULL OR COALESCE(u.is_super_admin, FALSE) = FALSE)").
		Order("omp.created_at ASC, omp.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	members := make([]*types.OrganizationMember, 0, len(rows))
	for _, row := range rows {
		members = append(members, buildOrgTreeMember(row))
	}
	return members, nil
}

func (r *orgTreeRepository) ListOrgTreeOrganizationsByUserID(ctx context.Context, userID string) ([]*types.Organization, error) {
	var orgs []*types.Organization
	err := r.db.WithContext(ctx).
		Table("organizations").
		Distinct("organizations.*").
		Joins("JOIN organization_members omp ON omp.organization_id = organizations.id").
		Where("omp.user_id = ?", userID).
		Order("organizations.created_at DESC").
		Find(&orgs).Error
	if err != nil {
		return nil, err
	}
	return orgs, nil
}

func (r *orgTreeRepository) BatchCountOrgTreeMembers(ctx context.Context, orgIDs []string) (map[string]int, error) {
	if len(orgIDs) == 0 {
		return map[string]int{}, nil
	}
	type countResult struct {
		OrganizationID string `gorm:"column:organization_id"`
		Count          int    `gorm:"column:count"`
	}
	var results []countResult
	err := r.db.WithContext(ctx).
		Table("organization_members").
		Select("organization_id, COUNT(*) as count").
		Where("organization_id IN ?", orgIDs).
		Group("organization_id").
		Find(&results).Error
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(results))
	for _, row := range results {
		counts[row.OrganizationID] = row.Count
	}
	return counts, nil
}

func (r *orgTreeRepository) BatchListOrgTreeMemberUserIDs(ctx context.Context, orgIDs []string) (map[string][]string, error) {
	if len(orgIDs) == 0 {
		return map[string][]string{}, nil
	}
	type memberRow struct {
		OrganizationID string `gorm:"column:organization_id"`
		UserID         string `gorm:"column:user_id"`
	}
	var rows []memberRow
	err := r.db.WithContext(ctx).
		Table("organization_members").
		Select("organization_id, user_id").
		Where("organization_id IN ?", orgIDs).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string][]string, len(orgIDs))
	for _, row := range rows {
		result[row.OrganizationID] = append(result[row.OrganizationID], row.UserID)
	}
	return result, nil
}

func (r *orgTreeRepository) IsAdminOfAnyOrgTree(ctx context.Context, userID string, orgIDs []string, tenantID uint64) bool {
	if len(orgIDs) == 0 {
		return false
	}
	var count int64
	err := r.db.WithContext(ctx).
		Table("organization_members omp").
		Where("omp.user_id = ? AND omp.organization_id IN ? AND omp.tenant_id = ? AND omp.role = ?", userID, orgIDs, tenantID, types.OrgRoleAdmin).
		Limit(1).
		Count(&count).Error
	if err != nil {
		return false
	}
	return count > 0
}
