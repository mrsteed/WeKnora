package interfaces

import (
	"context"
	"io"
	"mime/multipart"

	"github.com/Tencent/WeKnora/internal/types"
)

// KnowledgeService defines the interface for knowledge services.
type KnowledgeService interface {
	// CreateKnowledgeFromFile creates knowledge from a file.
	CreateKnowledgeFromFile(
		ctx context.Context,
		kbID string,
		file *multipart.FileHeader,
		metadata map[string]string,
		enableMultimodel *bool,
	) (*types.Knowledge, error)
	// CreateKnowledgeFromURL creates knowledge from a URL.
	CreateKnowledgeFromURL(ctx context.Context, kbID string, url string, enableMultimodel *bool) (*types.Knowledge, error)
	// CreateKnowledgeFromPassage creates knowledge from text passages.
	CreateKnowledgeFromPassage(ctx context.Context, kbID string, passage []string) (*types.Knowledge, error)
	// CreateKnowledgeFromPassageSync creates knowledge from text passages and waits until chunks are indexed.
	CreateKnowledgeFromPassageSync(ctx context.Context, kbID string, passage []string) (*types.Knowledge, error)
	// CreateKnowledgeFromManual creates or saves manual Markdown knowledge content.
	CreateKnowledgeFromManual(ctx context.Context, kbID string, payload *types.ManualKnowledgePayload) (*types.Knowledge, error)
	// GetKnowledgeByID retrieves knowledge by ID.
	GetKnowledgeByID(ctx context.Context, id string) (*types.Knowledge, error)
	// GetKnowledgeBatch retrieves a batch of knowledge by IDs.
	GetKnowledgeBatch(ctx context.Context, tenantID uint, ids []string) ([]*types.Knowledge, error)
	// ListKnowledgeByKnowledgeBaseID lists all knowledge under a knowledge base.
	ListKnowledgeByKnowledgeBaseID(ctx context.Context, kbID string) ([]*types.Knowledge, error)
	// ListPagedKnowledgeByKnowledgeBaseID lists all knowledge under a knowledge base with pagination.
	// When tagID is non-empty, results are filtered by tag_id.
	ListPagedKnowledgeByKnowledgeBaseID(
		ctx context.Context,
		kbID string,
		page *types.Pagination,
		tagID string,
	) (*types.PageResult, error)
	// DeleteKnowledge deletes knowledge by ID.
	DeleteKnowledge(ctx context.Context, id string) error
	// GetKnowledgeFile retrieves the file associated with the knowledge.
	GetKnowledgeFile(ctx context.Context, id string) (io.ReadCloser, string, error)
	// UpdateKnowledge updates knowledge information.
	UpdateKnowledge(ctx context.Context, knowledge *types.Knowledge) error
	// UpdateManualKnowledge updates manual Markdown knowledge content.
	UpdateManualKnowledge(ctx context.Context, knowledgeID string, payload *types.ManualKnowledgePayload) (*types.Knowledge, error)
	// CloneKnowledgeBase clones knowledge to another knowledge base.
	CloneKnowledgeBase(ctx context.Context, srcID, dstID string) error
	// UpdateImageInfo updates image information for a knowledge chunk.
	UpdateImageInfo(ctx context.Context, knowledgeID string, chunkID string, imageInfo string) error
	// ListFAQEntries lists FAQ entries under a FAQ knowledge base.
	// When tagID is non-empty, results are filtered by tag_id on FAQ chunks.
	ListFAQEntries(ctx context.Context, kbID string, page *types.Pagination, tagID string) (*types.PageResult, error)
	// UpsertFAQEntries imports or appends FAQ entries.
	UpsertFAQEntries(ctx context.Context, kbID string, payload *types.FAQBatchUpsertPayload) error
	// UpdateFAQEntry updates a single FAQ entry.
	UpdateFAQEntry(ctx context.Context, kbID string, entryID string, payload *types.FAQEntryPayload) error
	// DeleteFAQEntries deletes FAQ entries in batch.
	DeleteFAQEntries(ctx context.Context, kbID string, entryIDs []string) error
	// SearchFAQEntries searches FAQ entries using hybrid search.
	SearchFAQEntries(ctx context.Context, kbID string, req *types.FAQSearchRequest) ([]*types.FAQEntry, error)
}

// KnowledgeRepository defines the interface for knowledge repositories.
type KnowledgeRepository interface {
	CreateKnowledge(ctx context.Context, knowledge *types.Knowledge) error
	GetKnowledgeByID(ctx context.Context, tenantID uint, id string) (*types.Knowledge, error)
	ListKnowledgeByKnowledgeBaseID(ctx context.Context, tenantID uint, kbID string) ([]*types.Knowledge, error)
	// ListPagedKnowledgeByKnowledgeBaseID lists all knowledge in a knowledge base with pagination.
	// When tagID is non-empty, results are filtered by tag_id.
	ListPagedKnowledgeByKnowledgeBaseID(ctx context.Context,
		tenantID uint, kbID string, page *types.Pagination, tagID string,
	) ([]*types.Knowledge, int64, error)
	UpdateKnowledge(ctx context.Context, knowledge *types.Knowledge) error
	DeleteKnowledge(ctx context.Context, tenantID uint, id string) error
	DeleteKnowledgeList(ctx context.Context, tenantID uint, ids []string) error
	GetKnowledgeBatch(ctx context.Context, tenantID uint, ids []string) ([]*types.Knowledge, error)
	// CheckKnowledgeExists checks if knowledge already exists.
	// For file types, check by fileHash or (fileName+fileSize).
	// For URL types, check by URL.
	// Returns whether it exists, the existing knowledge object (if any), and possible error.
	CheckKnowledgeExists(
		ctx context.Context,
		tenantID uint,
		kbID string,
		params *types.KnowledgeCheckParams,
	) (bool, *types.Knowledge, error)
	// AminusB returns the difference set of A and B.
	AminusB(ctx context.Context, Atenant uint, A string, Btenant uint, B string) ([]string, error)
	UpdateKnowledgeColumn(ctx context.Context, id string, column string, value interface{}) error
}
