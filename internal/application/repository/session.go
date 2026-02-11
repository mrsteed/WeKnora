package repository

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// sessionRepository implements the SessionRepository interface
type sessionRepository struct {
	db *gorm.DB
}

// NewSessionRepository creates a new session repository instance
func NewSessionRepository(db *gorm.DB) interfaces.SessionRepository {
	return &sessionRepository{db: db}
}

// Create creates a new session
func (r *sessionRepository) Create(ctx context.Context, session *types.Session) (*types.Session, error) {
	session.CreatedAt = time.Now()
	session.UpdatedAt = time.Now()
	if err := r.db.WithContext(ctx).Create(session).Error; err != nil {
		return nil, err
	}
	// Return the session with generated ID
	return session, nil
}

// Get retrieves a session by ID (filtered by tenantID and userID for ownership check)
func (r *sessionRepository) Get(ctx context.Context, tenantID uint64, userID string, id string) (*types.Session, error) {
	var session types.Session
	query := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	err := query.First(&session, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// GetByTenantAndUser retrieves all sessions for a specific user within a tenant
func (r *sessionRepository) GetByTenantAndUser(ctx context.Context, tenantID uint64, userID string) ([]*types.Session, error) {
	var sessions []*types.Session
	query := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	err := query.Order("created_at DESC").Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

// GetPagedByTenantAndUser retrieves sessions for a specific user within a tenant with pagination
func (r *sessionRepository) GetPagedByTenantAndUser(
	ctx context.Context, tenantID uint64, userID string, page *types.Pagination,
) ([]*types.Session, int64, error) {
	var sessions []*types.Session
	var total int64

	baseQuery := r.db.WithContext(ctx).Model(&types.Session{}).Where("tenant_id = ?", tenantID)
	if userID != "" {
		baseQuery = baseQuery.Where("user_id = ?", userID)
	}

	// First query the total count
	err := baseQuery.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// Then query the paginated data
	dataQuery := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if userID != "" {
		dataQuery = dataQuery.Where("user_id = ?", userID)
	}
	err = dataQuery.
		Order("created_at DESC").
		Offset(page.Offset()).
		Limit(page.Limit()).
		Find(&sessions).Error
	if err != nil {
		return nil, 0, err
	}

	return sessions, total, nil
}

// Update updates a session (filtered by tenantID and userID for ownership check)
func (r *sessionRepository) Update(ctx context.Context, session *types.Session) error {
	session.UpdatedAt = time.Now()
	query := r.db.WithContext(ctx).Where("tenant_id = ?", session.TenantID)
	if session.UserID != "" {
		query = query.Where("user_id = ?", session.UserID)
	}
	return query.Save(session).Error
}

// Delete deletes a session (filtered by tenantID and userID for ownership check)
func (r *sessionRepository) Delete(ctx context.Context, tenantID uint64, userID string, id string) error {
	query := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	return query.Delete(&types.Session{}, "id = ?", id).Error
}
