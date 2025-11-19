package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// ChunkRepository defines the interface for chunk repository operations
type ChunkRepository interface {
	// CreateChunks creates chunks
	CreateChunks(ctx context.Context, chunks []*types.Chunk) error
	// GetChunkByID gets a chunk by id
	GetChunkByID(ctx context.Context, tenantID uint, id string) (*types.Chunk, error)
	// ListChunksByID lists chunks by ids
	ListChunksByID(ctx context.Context, tenantID uint, ids []string) ([]*types.Chunk, error)
	// ListChunksByKnowledgeID lists chunks by knowledge id
	ListChunksByKnowledgeID(ctx context.Context, tenantID uint, knowledgeID string) ([]*types.Chunk, error)
	// ListPagedChunksByKnowledgeID lists paged chunks by knowledge id.
	// When tagID is non-empty, results are filtered by tag_id.
	ListPagedChunksByKnowledgeID(
		ctx context.Context,
		tenantID uint,
		knowledgeID string,
		page *types.Pagination,
		chunkType []types.ChunkType,
		tagID string,
	) ([]*types.Chunk, int64, error)
	ListChunkByParentID(ctx context.Context, tenantID uint, parentID string) ([]*types.Chunk, error)
	// UpdateChunk updates a chunk
	UpdateChunk(ctx context.Context, chunk *types.Chunk) error
	// UpdateChunks updates chunks in batch
	UpdateChunks(ctx context.Context, chunks []*types.Chunk) error
	// DeleteChunk deletes a chunk
	DeleteChunk(ctx context.Context, tenantID uint, id string) error
	// DeleteChunksByKnowledgeID deletes chunks by knowledge id
	DeleteChunksByKnowledgeID(ctx context.Context, tenantID uint, knowledgeID string) error
	// DeleteByKnowledgeList deletes all chunks for a knowledge list
	DeleteByKnowledgeList(ctx context.Context, tenantID uint, knowledgeIDs []string) error
}

// ChunkService defines the interface for chunk service operations
type ChunkService interface {
	// CreateChunks creates chunks
	CreateChunks(ctx context.Context, chunks []*types.Chunk) error
	// GetChunkByID gets a chunk by id
	GetChunkByID(ctx context.Context, id string) (*types.Chunk, error)
	// ListChunksByKnowledgeID lists chunks by knowledge id
	ListChunksByKnowledgeID(ctx context.Context, knowledgeID string) ([]*types.Chunk, error)
	// ListPagedChunksByKnowledgeID lists paged chunks by knowledge id
	ListPagedChunksByKnowledgeID(
		ctx context.Context,
		knowledgeID string,
		page *types.Pagination,
		chunkType []types.ChunkType,
	) (*types.PageResult, error)
	// UpdateChunk updates a chunk
	UpdateChunk(ctx context.Context, chunk *types.Chunk) error
	// DeleteChunk deletes a chunk
	DeleteChunk(ctx context.Context, id string) error
	// DeleteChunksByKnowledgeID deletes chunks by knowledge id
	DeleteChunksByKnowledgeID(ctx context.Context, knowledgeID string) error
	// DeleteByKnowledgeList deletes all chunks for a knowledge list
	DeleteByKnowledgeList(ctx context.Context, ids []string) error
	// ListChunkByParentID lists chunks by parent id
	ListChunkByParentID(ctx context.Context, tenantID uint, parentID string) ([]*types.Chunk, error)
	// GetRepository gets the chunk repository
	GetRepository() ChunkRepository
}
