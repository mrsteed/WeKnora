package repository

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

var (
	ErrOrganizationNotFound   = errors.New("organization not found")
	ErrOrgMemberNotFound      = errors.New("organization member not found")
	ErrOrgMemberAlreadyExists = errors.New("member already exists in organization")
	ErrInviteCodeNotFound     = errors.New("invite code not found")
	ErrInviteCodeExpired      = errors.New("invite code has expired")
)

type organizationMemberRow struct {
	ID             string
	OrganizationID string
	UserID         string
	TenantID       uint64
	Role           types.OrgMemberRole
	JoinedAt       time.Time
	UpdatedAt      time.Time
	Username       string
	Email          string
	Avatar         string
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
	IsOwner        bool
}

func buildOrganizationMember(row organizationMemberRow) *types.OrganizationMember {
	member := &types.OrganizationMember{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		UserID:         row.UserID,
		TenantID:       row.TenantID,
		Role:           row.Role,
		CreatedAt:      row.JoinedAt,
		UpdatedAt:      row.UpdatedAt,
	}
	if row.UserID != "" || row.Username != "" || row.Email != "" || row.Avatar != "" {
		member.User = &types.User{
			ID:       row.UserID,
			Username: row.Username,
			Email:    row.Email,
			Avatar:   row.Avatar,
		}
	}
	return member
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
		ID:           row.UserID,
		Username:     row.Username,
		Email:        row.Email,
		Phone:        row.Phone,
		Avatar:       row.Avatar,
		IsSuperAdmin: row.IsSuperAdmin,
	}
	return member
}

func parseOrganizationMemberIdentifier(identifier string) (uint64, bool) {
	tenantID, err := strconv.ParseUint(identifier, 10, 64)
	if err != nil {
		return 0, false
	}
	return tenantID, true
}

// organizationRepository implements OrganizationRepository interface
type organizationRepository struct {
	db *gorm.DB
}

// NewOrganizationRepository creates a new organization repository
func NewOrganizationRepository(db *gorm.DB) interfaces.OrganizationRepository {
	return &organizationRepository{db: db}
}

// NewOrgTreeRepository creates a new org-tree repository (shares the same implementation)
func NewOrgTreeRepository(db *gorm.DB) interfaces.OrgTreeRepository {
	return &organizationRepository{db: db}
}

// Create creates a new organization
func (r *organizationRepository) Create(ctx context.Context, org *types.Organization) error {
	// If invite_code is empty string, set it to nil BEFORE inserting
	// to avoid unique constraint violations (empty string would trigger the unique index)
	if org.InviteCode == "" {
		// Use sql.NullString or set the field via a map to insert NULL
		return r.db.WithContext(ctx).Model(&types.Organization{}).Create(map[string]interface{}{
			"id":                        org.ID,
			"name":                      org.Name,
			"description":               org.Description,
			"avatar":                    org.Avatar,
			"owner_id":                  org.OwnerID,
			"owner_tenant_id":           org.OwnerTenantID,
			"invite_code":               nil, // Explicitly set to NULL
			"invite_code_expires_at":    org.InviteCodeExpiresAt,
			"invite_code_validity_days": org.InviteCodeValidityDays,
			"require_approval":          org.RequireApproval,
			"searchable":                org.Searchable,
			"member_limit":              org.MemberLimit,
			"parent_id":                 org.ParentID,
			"path":                      org.Path,
			"level":                     org.Level,
			"sort_order":                org.SortOrder,
			"tenant_id":                 org.OrgTenantID,
			"created_at":                org.CreatedAt,
			"updated_at":                org.UpdatedAt,
			"deleted_at":                org.DeletedAt,
		}).Error
	}
	// Normal insert for organizations with invite codes
	return r.db.WithContext(ctx).Create(org).Error
}

// GetByID gets an organization by ID
func (r *organizationRepository) GetByID(ctx context.Context, id string) (*types.Organization, error) {
	var org types.Organization
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&org).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrganizationNotFound
		}
		return nil, err
	}
	return &org, nil
}

// GetByInviteCode gets an organization by invite code (returns ErrInviteCodeExpired if code has expired)
func (r *organizationRepository) GetByInviteCode(ctx context.Context, inviteCode string) (*types.Organization, error) {
	var org types.Organization
	if err := r.db.WithContext(ctx).Where("invite_code = ?", inviteCode).First(&org).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInviteCodeNotFound
		}
		return nil, err
	}
	if org.InviteCodeExpiresAt != nil && org.InviteCodeExpiresAt.Before(time.Now()) {
		return nil, ErrInviteCodeExpired
	}
	return &org, nil
}

// ListByUserID lists organizations that a user belongs to within the current tenant
func (r *organizationRepository) ListByUserID(ctx context.Context, userID string, tenantID uint64) ([]*types.Organization, error) {
	var orgs []*types.Organization

	err := r.db.WithContext(ctx).
		Table("organizations").
		Distinct("organizations.*").
		Joins("JOIN organization_tenant_members otm ON otm.organization_id = organizations.id AND otm.tenant_id = ?", tenantID).
		Joins("JOIN tenant_members tm ON tm.tenant_id = otm.tenant_id AND tm.user_id = ? AND tm.deleted_at IS NULL AND tm.status = ?", userID, types.TenantMemberStatusActive).
		Where("organizations.deleted_at IS NULL").
		Order("organizations.created_at DESC").
		Find(&orgs).Error

	if err != nil {
		return nil, err
	}
	return orgs, nil
}

// ListSearchable lists organizations that are searchable (open for discovery), optionally filtered by name/description/ID
func (r *organizationRepository) ListSearchable(ctx context.Context, query string, limit int) ([]*types.Organization, error) {
	if limit <= 0 {
		limit = 20
	}
	var orgs []*types.Organization
	q := r.db.WithContext(ctx).Where("searchable = ?", true)
	if query != "" {
		pattern := "%" + query + "%"
		// 支持按名称、描述或空间 ID 搜索，便于区分同名空间
		q = q.Where("name ILIKE ? OR description ILIKE ? OR id::text ILIKE ?", pattern, pattern, pattern)
	}
	err := q.Order("created_at DESC").Limit(limit).Find(&orgs).Error
	if err != nil {
		return nil, err
	}
	return orgs, nil
}

// Update updates an organization (Select ensures zero values like invite_code_validity_days=0 are persisted)
func (r *organizationRepository) Update(ctx context.Context, org *types.Organization) error {
	return r.db.WithContext(ctx).Model(&types.Organization{}).Where("id = ?", org.ID).
		Select("name", "description", "avatar", "require_approval", "searchable", "invite_code_validity_days", "member_limit", "parent_id", "sort_order", "updated_at").
		Updates(org).Error
}

// Delete soft deletes an organization
func (r *organizationRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&types.Organization{}).Error
}

// AddMember adds a member to an organization
func (r *organizationRepository) AddMember(ctx context.Context, member *types.OrganizationMember) error {
	var count int64
	r.db.WithContext(ctx).
		Table("organization_tenant_members").
		Where("organization_id = ? AND tenant_id = ?", member.OrganizationID, member.TenantID).
		Count(&count)

	if count > 0 {
		return ErrOrgMemberAlreadyExists
	}

	joinedAt := member.CreatedAt
	if joinedAt.IsZero() {
		joinedAt = time.Now()
	}
	updatedAt := member.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = joinedAt
	}

	return r.db.WithContext(ctx).
		Table("organization_tenant_members").
		Create(map[string]interface{}{
			"id":                     member.ID,
			"organization_id":        member.OrganizationID,
			"tenant_id":              member.TenantID,
			"role":                   member.Role,
			"representative_user_id": member.UserID,
			"joined_at":              joinedAt,
			"created_at":             joinedAt,
			"updated_at":             updatedAt,
		}).Error
}

// RemoveMember removes a member from an organization
func (r *organizationRepository) RemoveMember(ctx context.Context, orgID string, userID string) error {
	query := r.db.WithContext(ctx).Table("organization_tenant_members").Where("organization_id = ?", orgID)
	if tenantID, ok := parseOrganizationMemberIdentifier(userID); ok {
		query = query.Where("tenant_id = ?", tenantID)
	} else {
		query = query.Where("representative_user_id = ?", userID)
	}
	result := query.Delete(nil)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrOrgMemberNotFound
	}
	return nil
}

// UpdateMemberRole updates a member's role in an organization
func (r *organizationRepository) UpdateMemberRole(ctx context.Context, orgID string, userID string, role types.OrgMemberRole) error {
	query := r.db.WithContext(ctx).
		Table("organization_tenant_members").
		Where("organization_id = ?", orgID)
	if tenantID, ok := parseOrganizationMemberIdentifier(userID); ok {
		query = query.Where("tenant_id = ?", tenantID)
	} else {
		query = query.Where("representative_user_id = ?", userID)
	}
	result := query.
		Update("role", role)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrOrgMemberNotFound
	}
	return nil
}

// ListMembers lists all members of an organization
func (r *organizationRepository) ListMembers(ctx context.Context, orgID string) ([]*types.OrganizationMember, error) {
	var rows []organizationMemberRow
	err := r.db.WithContext(ctx).
		Table("organization_tenant_members otm").
		Select("otm.id, otm.organization_id, otm.tenant_id, otm.role, COALESCE(otm.joined_at, otm.created_at) AS joined_at, otm.updated_at, u.id AS user_id, u.username, u.email, u.avatar").
		Joins("LEFT JOIN users u ON u.id = otm.representative_user_id").
		Where("otm.organization_id = ?", orgID).
		Order("COALESCE(otm.joined_at, otm.created_at) ASC, otm.id ASC").
		Scan(&rows).Error

	if err != nil {
		return nil, err
	}
	members := make([]*types.OrganizationMember, 0, len(rows))
	for _, row := range rows {
		members = append(members, buildOrganizationMember(row))
	}
	return members, nil
}

// GetMember gets a specific member of an organization
func (r *organizationRepository) GetMember(ctx context.Context, orgID string, userID string) (*types.OrganizationMember, error) {
	query := r.db.WithContext(ctx).
		Table("organization_tenant_members otm").
		Select("otm.id, otm.organization_id, otm.tenant_id, otm.role, COALESCE(otm.joined_at, otm.created_at) AS joined_at, otm.updated_at, u.id AS user_id, u.username, u.email, u.avatar").
		Joins("LEFT JOIN users u ON u.id = otm.representative_user_id").
		Where("otm.organization_id = ?", orgID)
	if tenantID, ok := parseOrganizationMemberIdentifier(userID); ok {
		query = query.Where("otm.tenant_id = ?", tenantID)
	} else {
		query = query.Joins("JOIN tenant_members tm ON tm.tenant_id = otm.tenant_id AND tm.user_id = ? AND tm.deleted_at IS NULL AND tm.status = ?", userID, types.TenantMemberStatusActive)
	}

	var row organizationMemberRow
	err := query.Order("COALESCE(otm.joined_at, otm.created_at) ASC, otm.id ASC").Take(&row).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrgMemberNotFound
		}
		return nil, err
	}
	return buildOrganizationMember(row), nil
}

// ListMembersByUserForOrgs returns one member record per org where the user is a member (batch).
func (r *organizationRepository) ListMembersByUserForOrgs(ctx context.Context, userID string, orgIDs []string) (map[string]*types.OrganizationMember, error) {
	if len(orgIDs) == 0 {
		return make(map[string]*types.OrganizationMember), nil
	}
	var rows []organizationMemberRow
	err := r.db.WithContext(ctx).
		Table("organization_tenant_members otm").
		Select("otm.id, otm.organization_id, otm.tenant_id, otm.role, COALESCE(otm.joined_at, otm.created_at) AS joined_at, otm.updated_at, u.id AS user_id, u.username, u.email, u.avatar").
		Joins("JOIN tenant_members tm ON tm.tenant_id = otm.tenant_id AND tm.user_id = ? AND tm.deleted_at IS NULL AND tm.status = ?", userID, types.TenantMemberStatusActive).
		Joins("LEFT JOIN users u ON u.id = otm.representative_user_id").
		Where("otm.organization_id IN ?", orgIDs).
		Order("COALESCE(otm.joined_at, otm.created_at) ASC, otm.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]*types.OrganizationMember, len(rows))
	for _, row := range rows {
		if _, exists := out[row.OrganizationID]; !exists {
			out[row.OrganizationID] = buildOrganizationMember(row)
		}
	}
	return out, nil
}

// CountMembers counts the number of members in an organization
func (r *organizationRepository) CountMembers(ctx context.Context, orgID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table("organization_tenant_members").
		Where("organization_id = ?", orgID).
		Count(&count).Error
	return count, err
}

// BatchCountMembers counts members for multiple organizations in a single query
func (r *organizationRepository) BatchCountMembers(ctx context.Context, orgIDs []string) (map[string]int, error) {
	if len(orgIDs) == 0 {
		return make(map[string]int), nil
	}
	type countResult struct {
		OrganizationID string `gorm:"column:organization_id"`
		Count          int    `gorm:"column:count"`
	}
	var results []countResult
	err := r.db.WithContext(ctx).
		Table("organization_tenant_members").
		Select("organization_id, COUNT(*) as count").
		Where("organization_id IN ?", orgIDs).
		Group("organization_id").
		Find(&results).Error
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(results))
	for _, r := range results {
		counts[r.OrganizationID] = r.Count
	}
	return counts, nil
}

// BatchListMemberUserIDs returns user IDs grouped by organization for batch processing
func (r *organizationRepository) BatchListMemberUserIDs(ctx context.Context, orgIDs []string) (map[string][]string, error) {
	if len(orgIDs) == 0 {
		return make(map[string][]string), nil
	}
	type memberRow struct {
		OrganizationID string `gorm:"column:organization_id"`
		UserID         string `gorm:"column:user_id"`
	}
	var rows []memberRow
	err := r.db.WithContext(ctx).
		Table("organization_tenant_members").
		Select("organization_id, representative_user_id AS user_id").
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

// IsAdminOfAnyOrg checks if the user is an admin of any org in the given list (single SQL query)
func (r *organizationRepository) IsAdminOfAnyOrg(ctx context.Context, userID string, orgIDs []string, tenantID uint64) bool {
	if len(orgIDs) == 0 {
		return false
	}
	var count int64
	err := r.db.WithContext(ctx).
		Table("organization_tenant_members otm").
		Joins("JOIN tenant_members tm ON tm.tenant_id = otm.tenant_id AND tm.user_id = ? AND tm.deleted_at IS NULL AND tm.status = ?", userID, types.TenantMemberStatusActive).
		Where("otm.organization_id IN ? AND otm.role = ? AND otm.tenant_id = ?",
			orgIDs, types.OrgRoleAdmin, tenantID).
		Limit(1).
		Count(&count).Error
	if err != nil {
		return false
	}
	return count > 0
}

func (r *organizationRepository) AddOrgTreeMember(ctx context.Context, member *types.OrganizationMember) error {
	var count int64
	r.db.WithContext(ctx).
		Table("organization_members_pre_plan3").
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

	return r.db.WithContext(ctx).
		Table("organization_members_pre_plan3").
		Create(map[string]interface{}{
			"id":              member.ID,
			"organization_id": member.OrganizationID,
			"user_id":         member.UserID,
			"tenant_id":       member.TenantID,
			"role":            member.Role,
			"created_at":      createdAt,
			"updated_at":      updatedAt,
		}).Error
}

func (r *organizationRepository) RemoveOrgTreeMember(ctx context.Context, orgID string, userID string) error {
	result := r.db.WithContext(ctx).
		Table("organization_members_pre_plan3").
		Where("organization_id = ? AND user_id = ?", orgID, userID).
		Delete(nil)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrOrgMemberNotFound
	}
	return nil
}

func (r *organizationRepository) UpdateOrgTreeMemberRole(ctx context.Context, orgID string, userID string, role types.OrgMemberRole) error {
	result := r.db.WithContext(ctx).
		Table("organization_members_pre_plan3").
		Where("organization_id = ? AND user_id = ?", orgID, userID).
		Update("role", role)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrOrgMemberNotFound
	}
	return nil
}

func (r *organizationRepository) ListOrgTreeMembers(ctx context.Context, orgID string) ([]*types.OrganizationMember, error) {
	var rows []orgTreeMemberRow
	err := r.db.WithContext(ctx).
		Table("organization_members_pre_plan3 omp").
		Select("omp.id, omp.organization_id, omp.user_id, omp.tenant_id, omp.role, omp.created_at, omp.updated_at, u.username, u.email, u.phone, u.avatar, u.is_super_admin, (o.owner_id = omp.user_id) AS is_owner").
		Joins("JOIN organizations o ON o.id = omp.organization_id").
		Joins("LEFT JOIN users u ON u.id = omp.user_id").
		Where("omp.organization_id = ?", orgID).
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

func (r *organizationRepository) ListOrgTreeOrganizationsByUserID(ctx context.Context, userID string) ([]*types.Organization, error) {
	var orgs []*types.Organization
	err := r.db.WithContext(ctx).
		Table("organizations").
		Distinct("organizations.*").
		Joins("JOIN organization_members_pre_plan3 omp ON omp.organization_id = organizations.id").
		Where("omp.user_id = ?", userID).
		Order("organizations.created_at DESC").
		Find(&orgs).Error
	if err != nil {
		return nil, err
	}
	return orgs, nil
}

func (r *organizationRepository) BatchCountOrgTreeMembers(ctx context.Context, orgIDs []string) (map[string]int, error) {
	if len(orgIDs) == 0 {
		return make(map[string]int), nil
	}
	type countResult struct {
		OrganizationID string `gorm:"column:organization_id"`
		Count          int    `gorm:"column:count"`
	}
	var results []countResult
	err := r.db.WithContext(ctx).
		Table("organization_members_pre_plan3").
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

func (r *organizationRepository) BatchListOrgTreeMemberUserIDs(ctx context.Context, orgIDs []string) (map[string][]string, error) {
	if len(orgIDs) == 0 {
		return make(map[string][]string), nil
	}
	type memberRow struct {
		OrganizationID string `gorm:"column:organization_id"`
		UserID         string `gorm:"column:user_id"`
	}
	var rows []memberRow
	err := r.db.WithContext(ctx).
		Table("organization_members_pre_plan3").
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

func (r *organizationRepository) IsAdminOfAnyOrgTree(ctx context.Context, userID string, orgIDs []string, tenantID uint64) bool {
	if len(orgIDs) == 0 {
		return false
	}
	var count int64
	err := r.db.WithContext(ctx).
		Table("organization_members_pre_plan3 omp").
		Joins("JOIN organizations o ON o.id = omp.organization_id").
		Where("omp.user_id = ? AND omp.organization_id IN ? AND omp.tenant_id = ? AND (omp.role = ? OR o.owner_id = ?)", userID, orgIDs, tenantID, types.OrgRoleAdmin, userID).
		Limit(1).
		Count(&count).Error
	if err != nil {
		return false
	}
	return count > 0
}

// UpdateInviteCode updates the invite code and optional expiry for an organization (expiresAt nil = never expire)
func (r *organizationRepository) UpdateInviteCode(ctx context.Context, orgID string, inviteCode string, expiresAt *time.Time) error {
	updates := map[string]interface{}{"invite_code": inviteCode, "invite_code_expires_at": expiresAt}
	return r.db.WithContext(ctx).
		Model(&types.Organization{}).
		Where("id = ?", orgID).
		Updates(updates).Error
}

// ----------------
// Join Requests
// ----------------

var ErrJoinRequestNotFound = errors.New("join request not found")

// CreateJoinRequest creates a new join request
func (r *organizationRepository) CreateJoinRequest(ctx context.Context, request *types.OrganizationJoinRequest) error {
	return r.db.WithContext(ctx).Create(request).Error
}

// GetJoinRequestByID gets a join request by ID
func (r *organizationRepository) GetJoinRequestByID(ctx context.Context, id string) (*types.OrganizationJoinRequest, error) {
	var request types.OrganizationJoinRequest
	err := r.db.WithContext(ctx).
		Preload("User").
		Where("id = ?", id).
		First(&request).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrJoinRequestNotFound
		}
		return nil, err
	}
	return &request, nil
}

// GetPendingJoinRequest gets a pending join request for a user in an organization (any type)
func (r *organizationRepository) GetPendingJoinRequest(ctx context.Context, orgID string, userID string) (*types.OrganizationJoinRequest, error) {
	var request types.OrganizationJoinRequest
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND user_id = ? AND status = ?", orgID, userID, types.JoinRequestStatusPending).
		First(&request).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrJoinRequestNotFound
		}
		return nil, err
	}
	return &request, nil
}

// GetPendingRequestByType gets a pending request for a user filtered by request type
func (r *organizationRepository) GetPendingRequestByType(ctx context.Context, orgID string, userID string, requestType types.JoinRequestType) (*types.OrganizationJoinRequest, error) {
	var requests []types.OrganizationJoinRequest
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND user_id = ? AND status = ? AND request_type = ?", orgID, userID, types.JoinRequestStatusPending, requestType).
		Limit(1).
		Find(&requests).Error
	if err != nil {
		return nil, err
	}
	if len(requests) == 0 {
		return nil, ErrJoinRequestNotFound
	}
	return &requests[0], nil
}

// ListJoinRequests lists join requests for an organization
func (r *organizationRepository) ListJoinRequests(ctx context.Context, orgID string, status types.JoinRequestStatus) ([]*types.OrganizationJoinRequest, error) {
	var requests []*types.OrganizationJoinRequest
	query := r.db.WithContext(ctx).
		Preload("User").
		Where("organization_id = ?", orgID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	err := query.Order("created_at DESC").Find(&requests).Error
	if err != nil {
		return nil, err
	}
	return requests, nil
}

// CountJoinRequests counts join requests for an organization by status
func (r *organizationRepository) CountJoinRequests(ctx context.Context, orgID string, status types.JoinRequestStatus) (int64, error) {
	var count int64
	query := r.db.WithContext(ctx).Model(&types.OrganizationJoinRequest{}).Where("organization_id = ?", orgID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	err := query.Count(&count).Error
	return count, err
}

// UpdateJoinRequestStatus updates the status of a join request
func (r *organizationRepository) UpdateJoinRequestStatus(ctx context.Context, id string, status types.JoinRequestStatus, reviewedBy string, reviewMessage string) error {
	return r.db.WithContext(ctx).
		Model(&types.OrganizationJoinRequest{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":         status,
			"reviewed_by":    reviewedBy,
			"reviewed_at":    gorm.Expr("NOW()"),
			"review_message": reviewMessage,
		}).Error
}

// --------------------------------
// Org-tree operations
// --------------------------------

// GetByIDAndTenant gets an organization by ID within a specific tenant
func (r *organizationRepository) GetByIDAndTenant(ctx context.Context, id string, tenantID uint64) (*types.Organization, error) {
	var org types.Organization
	if err := r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		First(&org).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrganizationNotFound
		}
		return nil, err
	}
	return &org, nil
}

// ListByTenantID lists all organizations belonging to a tenant (org-tree nodes)
func (r *organizationRepository) ListByTenantID(ctx context.Context, tenantID uint64) ([]*types.Organization, error) {
	var orgs []*types.Organization
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("level ASC, sort_order ASC, created_at ASC").
		Find(&orgs).Error; err != nil {
		return nil, err
	}
	return orgs, nil
}

// GetChildren returns direct children of an organization
func (r *organizationRepository) GetChildren(ctx context.Context, parentID string) ([]*types.Organization, error) {
	var orgs []*types.Organization
	if err := r.db.WithContext(ctx).
		Where("parent_id = ?", parentID).
		Order("sort_order ASC, created_at ASC").
		Find(&orgs).Error; err != nil {
		return nil, err
	}
	return orgs, nil
}

// GetDescendantsByPath returns all descendants by matching path prefix
func (r *organizationRepository) GetDescendantsByPath(ctx context.Context, pathPrefix string) ([]*types.Organization, error) {
	var orgs []*types.Organization
	if err := r.db.WithContext(ctx).
		Where("path LIKE ?", pathPrefix+"/%").
		Order("level ASC, sort_order ASC").
		Find(&orgs).Error; err != nil {
		return nil, err
	}
	return orgs, nil
}

// GetDescendantsByPathAndTenant returns all descendants by matching path prefix within a specific tenant
func (r *organizationRepository) GetDescendantsByPathAndTenant(ctx context.Context, pathPrefix string, tenantID uint64) ([]*types.Organization, error) {
	var orgs []*types.Organization
	if err := r.db.WithContext(ctx).
		Where("path LIKE ? AND tenant_id = ?", pathPrefix+"/%", tenantID).
		Order("level ASC, sort_order ASC").
		Find(&orgs).Error; err != nil {
		return nil, err
	}
	return orgs, nil
}

// GetDescendantsByPathsAndTenant returns all descendants matching any of the path prefixes within a tenant (batch optimization)
func (r *organizationRepository) GetDescendantsByPathsAndTenant(ctx context.Context, pathPrefixes []string, tenantID uint64) ([]*types.Organization, error) {
	if len(pathPrefixes) == 0 {
		return nil, nil
	}
	var orgs []*types.Organization
	query := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	// Build OR conditions for each path prefix
	pathConditions := r.db.WithContext(ctx)
	for i, prefix := range pathPrefixes {
		if i == 0 {
			pathConditions = pathConditions.Where("path LIKE ?", prefix+"/%")
		} else {
			pathConditions = pathConditions.Or("path LIKE ?", prefix+"/%")
		}
	}
	query = query.Where(pathConditions)
	if err := query.Order("level ASC, sort_order ASC").Find(&orgs).Error; err != nil {
		return nil, err
	}
	return orgs, nil
}

// UpdatePath updates the path and level for an organization
func (r *organizationRepository) UpdatePath(ctx context.Context, id string, path string, level int) error {
	return r.db.WithContext(ctx).
		Model(&types.Organization{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"path":  path,
			"level": level,
		}).Error
}

// UpdatePathBatch updates path and level for all organizations matching old path prefix
func (r *organizationRepository) UpdatePathBatch(ctx context.Context, oldPathPrefix string, newPathPrefix string, levelDelta int) error {
	// Use SQL REPLACE for path and arithmetic for level
	return r.db.WithContext(ctx).
		Model(&types.Organization{}).
		Where("path LIKE ?", oldPathPrefix+"/%").
		Updates(map[string]interface{}{
			"path":  gorm.Expr("REPLACE(path, ?, ?)", oldPathPrefix, newPathPrefix),
			"level": gorm.Expr("level + ?", levelDelta),
		}).Error
}

// MoveNodeInTx atomically updates a node's path/level, its descendants' paths/levels, and its parent_id/sort_order
func (r *organizationRepository) MoveNodeInTx(ctx context.Context, nodeID string, newPath string, newLevel int, oldPathPrefix string, levelDelta int, parentID *string, sortOrder int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Step 1: Update self path and level
		if err := tx.Model(&types.Organization{}).Where("id = ?", nodeID).Updates(map[string]interface{}{
			"path":  newPath,
			"level": newLevel,
		}).Error; err != nil {
			return err
		}
		// Step 2: Batch update descendants' paths and levels
		if err := tx.Model(&types.Organization{}).Where("path LIKE ?", oldPathPrefix+"/%").Updates(map[string]interface{}{
			"path":  gorm.Expr("REPLACE(path, ?, ?)", oldPathPrefix, newPath),
			"level": gorm.Expr("level + ?", levelDelta),
		}).Error; err != nil {
			return err
		}
		// Step 3: Update parent_id and sort_order
		if err := tx.Model(&types.Organization{}).Where("id = ?", nodeID).Updates(map[string]interface{}{
			"parent_id":  parentID,
			"sort_order": sortOrder,
		}).Error; err != nil {
			return err
		}
		return nil
	})
}

// GetByIDs returns organizations by a list of IDs
func (r *organizationRepository) GetByIDs(ctx context.Context, ids []string) ([]*types.Organization, error) {
	if len(ids) == 0 {
		return []*types.Organization{}, nil
	}
	var orgs []*types.Organization
	if err := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Find(&orgs).Error; err != nil {
		return nil, err
	}
	return orgs, nil
}
