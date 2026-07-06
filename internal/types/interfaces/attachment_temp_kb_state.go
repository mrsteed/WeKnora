package interfaces

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// AttachmentTempKBStateService manages the session-scoped hidden KB used to
// materialize chat attachments into retrievable knowledge.
type AttachmentTempKBStateService interface {
	GetAttachmentTempKBState(ctx context.Context, sessionID string) *types.AttachmentTempKBState
	SaveAttachmentTempKBState(ctx context.Context, sessionID string, state *types.AttachmentTempKBState)
	DeleteAttachmentTempKBState(ctx context.Context, sessionID string) error
	CleanupExpiredAttachmentTempKBStates(ctx context.Context, maxIdle time.Duration) (int, error)
}
