package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

type ChatDocumentGenerationRunRepository interface {
	CreateRun(ctx context.Context, run *types.ChatDocumentGenerationRun) error
	GetRunByID(ctx context.Context, tenantID uint64, runID string) (*types.ChatDocumentGenerationRun, error)
	GetLatestRunBySessionAndRoot(ctx context.Context, tenantID uint64, sessionID string, rootMessageID string, rootArtifactID string) (*types.ChatDocumentGenerationRun, error)
	UpdateRun(ctx context.Context, run *types.ChatDocumentGenerationRun) error
}
