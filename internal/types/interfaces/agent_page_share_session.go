package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// AgentPageShareSessionRepository defines share-session specific persistence lookups.
type AgentPageShareSessionRepository interface {
	GetByID(ctx context.Context, sessionID string) (*types.Session, error)
	CountByShareID(ctx context.Context, shareID string) (int64, error)
}

// AgentPageShareSessionService defines anonymous share-session creation and validation operations.
type AgentPageShareSessionService interface {
	CreateAnonymousSession(ctx context.Context, shareCode string, clientIP string, userAgent string) (*types.AgentPageShareSessionCreateResult, error)
	ValidateAnonymousSession(ctx context.Context, shareCode string, sessionID string, visitorToken string) (*types.AgentPageShareSessionContext, error)
}
