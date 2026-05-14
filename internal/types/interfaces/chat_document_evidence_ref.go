package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

type ChatDocumentEvidenceRefRepository interface {
	CreateEvidenceRefs(ctx context.Context, refs []*types.ChatDocumentEvidenceRef) error
	ListEvidenceRefsByArtifactIDs(ctx context.Context, tenantID uint64, artifactIDs []string) ([]*types.ChatDocumentEvidenceRef, error)
}
