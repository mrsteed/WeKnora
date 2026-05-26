package repository

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

var (
	ErrAgentPageShareSessionNotFound = errors.New("agent page share session not found")
)

type agentPageShareSessionRepository struct {
	db *gorm.DB
}

// NewAgentPageShareSessionRepository creates a repository for anonymous share sessions.
func NewAgentPageShareSessionRepository(db *gorm.DB) interfaces.AgentPageShareSessionRepository {
	return &agentPageShareSessionRepository{db: db}
}

// GetByID loads one anonymous share session by session ID.
func (r *agentPageShareSessionRepository) GetByID(ctx context.Context, sessionID string) (*types.Session, error) {
	var session types.Session
	err := r.db.WithContext(ctx).
		Where("id = ? AND access_mode = ?", sessionID, types.SessionAccessModeAgentSharePage).
		First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentPageShareSessionNotFound
		}
		return nil, err
	}
	return &session, nil
}

// CountByShareID counts all anonymous share sessions created for one share record.
func (r *agentPageShareSessionRepository) CountByShareID(ctx context.Context, shareID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&types.Session{}).
		Where("access_mode = ? AND agent_page_share_id = ?", types.SessionAccessModeAgentSharePage, shareID).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}
