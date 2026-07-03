package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type chatDocumentArtifactRepository struct {
	db *gorm.DB
}

func NewChatDocumentArtifactRepository(db *gorm.DB) interfaces.ChatDocumentArtifactRepository {
	return &chatDocumentArtifactRepository{db: db}
}

func (r *chatDocumentArtifactRepository) CreateArtifact(ctx context.Context, artifact *types.ChatDocumentArtifact) error {
	now := time.Now()
	artifact.CreatedAt = now
	artifact.UpdatedAt = now
	return r.db.WithContext(ctx).Create(artifact).Error
}

func (r *chatDocumentArtifactRepository) GetArtifactByID(ctx context.Context, tenantID uint64, artifactID string) (*types.ChatDocumentArtifact, error) {
	var artifact types.ChatDocumentArtifact
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, artifactID).First(&artifact).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &artifact, nil
}

func (r *chatDocumentArtifactRepository) GetArtifactBySourceMessageID(ctx context.Context, tenantID uint64, sourceMessageID string) (*types.ChatDocumentArtifact, error) {
	var artifact types.ChatDocumentArtifact
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND source_message_id = ?", tenantID, sourceMessageID).First(&artifact).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &artifact, nil
}

func (r *chatDocumentArtifactRepository) GetLatestArtifactBySession(ctx context.Context, tenantID uint64, sessionID string) (*types.ChatDocumentArtifact, error) {
	var artifact types.ChatDocumentArtifact
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND session_id = ?", tenantID, sessionID).
		Order("created_at DESC").
		First(&artifact).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &artifact, nil
}

func (r *chatDocumentArtifactRepository) ListArtifactsBySession(ctx context.Context, tenantID uint64, sessionID string, limit int) ([]*types.ChatDocumentArtifact, error) {
	if limit <= 0 {
		limit = 20
	}
	var artifacts []*types.ChatDocumentArtifact
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND session_id = ?", tenantID, sessionID).
		Order("created_at DESC").
		Limit(limit).
		Find(&artifacts).Error; err != nil {
		return nil, err
	}
	return artifacts, nil
}

func (r *chatDocumentArtifactRepository) ListArtifactsByRootArtifact(ctx context.Context, tenantID uint64, rootArtifactID string) ([]*types.ChatDocumentArtifact, error) {
	if rootArtifactID == "" {
		return nil, nil
	}
	var artifacts []*types.ChatDocumentArtifact
	query := `
WITH RECURSIVE artifact_chain AS (
	SELECT *
	FROM chat_document_artifacts
	WHERE tenant_id = ? AND id = ? AND deleted_at IS NULL
	UNION ALL
	SELECT child.*
	FROM chat_document_artifacts child
	JOIN artifact_chain parent ON child.parent_artifact_id = parent.id
	WHERE child.tenant_id = ? AND child.deleted_at IS NULL
)
SELECT *
FROM artifact_chain
ORDER BY revision_no ASC, created_at ASC
`
	if err := r.db.WithContext(ctx).Raw(query, tenantID, rootArtifactID, tenantID).Scan(&artifacts).Error; err != nil {
		return nil, err
	}
	return artifacts, nil
}
