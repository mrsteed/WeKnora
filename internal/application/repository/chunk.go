package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/Tencent/WeKnora/internal/common"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// chunkRepository implements the ChunkRepository interface
type chunkRepository struct {
	db *gorm.DB
}

// NewChunkRepository creates a new chunk repository
func NewChunkRepository(db *gorm.DB) interfaces.ChunkRepository {
	return &chunkRepository{db: db}
}

// CreateChunks creates multiple chunks in batches
func (r *chunkRepository) CreateChunks(ctx context.Context, chunks []*types.Chunk) error {
	for _, chunk := range chunks {
		chunk.Content = common.CleanInvalidUTF8(chunk.Content)
	}
	return r.db.WithContext(ctx).CreateInBatches(chunks, 100).Error
}

// GetChunkByID retrieves a chunk by its ID and tenant ID
func (r *chunkRepository) GetChunkByID(ctx context.Context, tenantID uint64, id string) (*types.Chunk, error) {
	var chunk types.Chunk
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&chunk).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("chunk not found")
		}
		return nil, err
	}
	return &chunk, nil
}

// ListChunksByID retrieves multiple chunks by their IDs
func (r *chunkRepository) ListChunksByID(
	ctx context.Context, tenantID uint64, ids []string,
) ([]*types.Chunk, error) {
	var chunks []*types.Chunk
	if err := r.db.Debug().WithContext(ctx).
		Where("tenant_id = ? AND id IN ?", tenantID, ids).
		Find(&chunks).Error; err != nil {
		return nil, err
	}
	return chunks, nil
}

// ListChunksByKnowledgeID lists all chunks for a knowledge ID
func (r *chunkRepository) ListChunksByKnowledgeID(
	ctx context.Context, tenantID uint64, knowledgeID string,
) ([]*types.Chunk, error) {
	var chunks []*types.Chunk
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND knowledge_id = ? and chunk_type = ?", tenantID, knowledgeID, "text").
		Order("chunk_index ASC").
		Find(&chunks).Error; err != nil {
		return nil, err
	}
	return chunks, nil
}

// ListPagedChunksByKnowledgeID lists chunks for a knowledge ID with pagination
func (r *chunkRepository) ListPagedChunksByKnowledgeID(
	ctx context.Context,
	tenantID uint64,
	knowledgeID string,
	page *types.Pagination,
	chunkType []types.ChunkType,
	tagID string,
	keyword string,
) ([]*types.Chunk, int64, error) {
	var chunks []*types.Chunk
	var total int64
	keyword = strings.TrimSpace(keyword)

	baseFilter := func(db *gorm.DB) *gorm.DB {
		db = db.Where("tenant_id = ? AND knowledge_id = ? AND chunk_type IN (?) AND status in (?)",
			tenantID, knowledgeID, chunkType, []int{int(types.ChunkStatusIndexed), int(types.ChunkStatusDefault)})
		if tagID != "" {
			db = db.Where("tag_id = ?", tagID)
		}
		if keyword != "" {
			like := "%" + keyword + "%"
			db = db.Where("(content LIKE ? OR metadata::text LIKE ?)", like, like)
		}
		return db
	}

	query := baseFilter(r.db.WithContext(ctx).Model(&types.Chunk{}))

	// First query the total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Then query the paginated data
	dataQuery := baseFilter(
		r.db.WithContext(ctx).
			Select("id, content, knowledge_id, knowledge_base_id, start_at, end_at, chunk_index, is_enabled, chunk_type, parent_chunk_id, image_info, metadata, tag_id, status"),
	)

	if err := dataQuery.
		Order("chunk_index ASC").
		Offset(page.Offset()).
		Limit(page.Limit()).
		Find(&chunks).Error; err != nil {
		return nil, 0, err
	}

	return chunks, total, nil
}

func (r *chunkRepository) ListChunkByParentID(
	ctx context.Context,
	tenantID uint64,
	parentID string,
) ([]*types.Chunk, error) {
	var chunks []*types.Chunk
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND parent_chunk_id = ?", tenantID, parentID).
		Find(&chunks).Error; err != nil {
		return nil, err
	}
	return chunks, nil
}

// UpdateChunk updates a chunk
func (r *chunkRepository) UpdateChunk(ctx context.Context, chunk *types.Chunk) error {
	return r.db.WithContext(ctx).Save(chunk).Error
}

// UpdateChunks updates chunks in batch
func (r *chunkRepository) UpdateChunks(ctx context.Context, chunks []*types.Chunk) error {
	if len(chunks) == 0 {
		return nil
	}
	for _, chunk := range chunks {
		chunk.Content = common.CleanInvalidUTF8(chunk.Content)
	}
	return r.db.WithContext(ctx).Save(chunks).Error
}

// DeleteChunk deletes a chunk by its ID
func (r *chunkRepository) DeleteChunk(ctx context.Context, tenantID uint64, id string) error {
	return r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&types.Chunk{}).Error
}

// DeleteChunks deletes chunks by IDs in batch
func (r *chunkRepository) DeleteChunks(ctx context.Context, tenantID uint64, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("tenant_id = ? AND id IN ?", tenantID, ids).Delete(&types.Chunk{}).Error
}

// DeleteChunksByKnowledgeID deletes all chunks for a knowledge ID
func (r *chunkRepository) DeleteChunksByKnowledgeID(ctx context.Context, tenantID uint64, knowledgeID string) error {
	return r.db.WithContext(ctx).Where(
		"tenant_id = ? AND knowledge_id = ?", tenantID, knowledgeID,
	).Delete(&types.Chunk{}).Error
}

// DeleteByKnowledgeList deletes all chunks for a knowledge list
func (r *chunkRepository) DeleteByKnowledgeList(ctx context.Context, tenantID uint64, knowledgeIDs []string) error {
	return r.db.WithContext(ctx).Where(
		"tenant_id = ? AND knowledge_id in ?", tenantID, knowledgeIDs,
	).Delete(&types.Chunk{}).Error
}

// CountChunksByKnowledgeBaseID counts the number of chunks in a knowledge base
func (r *chunkRepository) CountChunksByKnowledgeBaseID(
	ctx context.Context,
	tenantID uint64,
	kbID string,
) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&types.Chunk{}).
		Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, kbID).
		Count(&count).Error
	return count, err
}

// DeleteUnindexedChunks by knowledge id and chunk index range
func (r *chunkRepository) DeleteUnindexedChunks(
	ctx context.Context,
	tenantID uint64,
	knowledgeID string,
) ([]*types.Chunk, error) {
	var chunks []*types.Chunk
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND knowledge_id = ? AND status = ?", tenantID, knowledgeID, types.ChunkStatusStored).
		Find(&chunks).Error; err != nil {
		return nil, err
	}
	if len(chunks) > 0 {
		if err := r.db.WithContext(ctx).
			Where("tenant_id = ? AND knowledge_id = ? AND status = ?", tenantID, knowledgeID, types.ChunkStatusStored).
			Delete(&types.Chunk{}).Error; err != nil {
			return nil, err
		}
	}
	return chunks, nil
}

// ListAllFAQChunksByKnowledgeID lists all FAQ chunks for a knowledge ID (only essential fields for efficiency)
// Uses batch query to handle large datasets
func (r *chunkRepository) ListAllFAQChunksByKnowledgeID(
	ctx context.Context,
	tenantID uint64,
	knowledgeID string,
) ([]*types.Chunk, error) {
	const batchSize = 1000 // 每批查询1000条
	var allChunks []*types.Chunk
	offset := 0

	for {
		var batchChunks []*types.Chunk
		if err := r.db.WithContext(ctx).
			Select("id, content_hash").
			Where("tenant_id = ? AND knowledge_id = ? AND chunk_type = ?", tenantID, knowledgeID, types.ChunkTypeFAQ).
			Offset(offset).
			Limit(batchSize).
			Find(&batchChunks).Error; err != nil {
			return nil, err
		}

		// 如果没有查询到数据，说明已经查询完毕
		if len(batchChunks) == 0 {
			break
		}

		allChunks = append(allChunks, batchChunks...)

		// 如果返回的数据少于批次大小，说明已经是最后一批
		if len(batchChunks) < batchSize {
			break
		}

		offset += batchSize
	}

	return allChunks, nil
}

// ListAllFAQChunksWithMetadataByKnowledgeBaseID lists all FAQ chunks for a knowledge base ID
// Returns ID and Metadata fields for duplicate question checking
// Uses batch query to handle large datasets
func (r *chunkRepository) ListAllFAQChunksWithMetadataByKnowledgeBaseID(
	ctx context.Context,
	tenantID uint64,
	kbID string,
) ([]*types.Chunk, error) {
	const batchSize = 1000 // 每批查询1000条
	var allChunks []*types.Chunk
	offset := 0

	for {
		var batchChunks []*types.Chunk
		if err := r.db.WithContext(ctx).
			Select("id, metadata").
			Where("tenant_id = ? AND knowledge_base_id = ? AND chunk_type = ? AND status = ?",
				tenantID, kbID, types.ChunkTypeFAQ, types.ChunkStatusIndexed).
			Offset(offset).
			Limit(batchSize).
			Find(&batchChunks).Error; err != nil {
			return nil, err
		}

		// 如果没有查询到数据，说明已经查询完毕
		if len(batchChunks) == 0 {
			break
		}

		allChunks = append(allChunks, batchChunks...)

		// 如果返回的数据少于批次大小，说明已经是最后一批
		if len(batchChunks) < batchSize {
			break
		}

		offset += batchSize
	}

	return allChunks, nil
}
