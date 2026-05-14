package repository

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type chatDocumentEvidenceRefRepository struct {
	db *gorm.DB
}

func NewChatDocumentEvidenceRefRepository(db *gorm.DB) interfaces.ChatDocumentEvidenceRefRepository {
	return &chatDocumentEvidenceRefRepository{db: db}
}

func (r *chatDocumentEvidenceRefRepository) CreateEvidenceRefs(ctx context.Context, refs []*types.ChatDocumentEvidenceRef) error {
	if len(refs) == 0 {
		return nil
	}
	now := time.Now()
	for _, ref := range refs {
		if ref == nil {
			continue
		}
		if ref.CreatedAt.IsZero() {
			ref.CreatedAt = now
		}
	}
	return r.db.WithContext(ctx).CreateInBatches(refs, 100).Error
}

func (r *chatDocumentEvidenceRefRepository) ListEvidenceRefsByArtifactIDs(ctx context.Context, tenantID uint64, artifactIDs []string) ([]*types.ChatDocumentEvidenceRef, error) {
	if len(artifactIDs) == 0 {
		return nil, nil
	}
	var refs []*types.ChatDocumentEvidenceRef
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND artifact_id IN ?", tenantID, artifactIDs).
		Order("created_at ASC").
		Order("score DESC").
		Find(&refs).Error; err != nil {
		return nil, err
	}
	return refs, nil
}
