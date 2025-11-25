package repository

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type knowledgeTagRepository struct {
	db *gorm.DB
}

// NewKnowledgeTagRepository creates a new tag repository.
func NewKnowledgeTagRepository(db *gorm.DB) interfaces.KnowledgeTagRepository {
	return &knowledgeTagRepository{db: db}
}

func (r *knowledgeTagRepository) Create(ctx context.Context, tag *types.KnowledgeTag) error {
	return r.db.WithContext(ctx).Create(tag).Error
}

func (r *knowledgeTagRepository) Update(ctx context.Context, tag *types.KnowledgeTag) error {
	return r.db.WithContext(ctx).Save(tag).Error
}

func (r *knowledgeTagRepository) GetByID(ctx context.Context, tenantID uint64, id string) (*types.KnowledgeTag, error) {
	var tag types.KnowledgeTag
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		First(&tag).Error; err != nil {
		return nil, err
	}
	return &tag, nil
}

func (r *knowledgeTagRepository) ListByKB(ctx context.Context, tenantID uint64, kbID string) ([]*types.KnowledgeTag, error) {
	var tags []*types.KnowledgeTag
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, kbID).
		Order("sort_order ASC, created_at ASC").
		Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

func (r *knowledgeTagRepository) Delete(ctx context.Context, tenantID uint64, id string) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Delete(&types.KnowledgeTag{}).Error
}

// CountReferences returns how many knowledges and chunks reference this tag.
func (r *knowledgeTagRepository) CountReferences(
	ctx context.Context,
	tenantID uint64,
	kbID string,
	tagID string,
) (knowledgeCount int64, chunkCount int64, err error) {
	if err = r.db.WithContext(ctx).
		Model(&types.Knowledge{}).
		Where("tenant_id = ? AND knowledge_base_id = ? AND tag_id = ?", tenantID, kbID, tagID).
		Count(&knowledgeCount).Error; err != nil {
		return
	}
	if err = r.db.WithContext(ctx).
		Model(&types.Chunk{}).
		Where("tenant_id = ? AND knowledge_base_id = ? AND tag_id = ?", tenantID, kbID, tagID).
		Count(&chunkCount).Error; err != nil {
		return
	}
	return
}
