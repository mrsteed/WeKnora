package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// AttachmentKnowledgeService materializes an already-uploaded provider:// file
// into a normal Knowledge record so chat attachments can reuse the standard KB
// parsing, chunking, indexing and post-processing pipeline.
type AttachmentKnowledgeService interface {
	CreateKnowledgeFromStoredFile(
		ctx context.Context,
		kbID string,
		filePath string,
		fileName string,
		fileType string,
		fileSize int64,
		channel string,
		processOverrides *types.KnowledgeProcessOverrides,
	) (*types.Knowledge, error)
	CreateKnowledgeFromReadResult(
		ctx context.Context,
		kbID string,
		filePath string,
		fileName string,
		fileType string,
		fileSize int64,
		channel string,
		readResult *types.ReadResult,
		storedImages []types.StoredImageReference,
		processOverrides *types.KnowledgeProcessOverrides,
	) (*types.Knowledge, error)
}
