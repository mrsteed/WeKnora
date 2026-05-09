package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

type ChatDocumentArtifactRepository interface {
	CreateArtifact(ctx context.Context, artifact *types.ChatDocumentArtifact) error
	GetArtifactByID(ctx context.Context, tenantID uint64, artifactID string) (*types.ChatDocumentArtifact, error)
	GetArtifactBySourceMessageID(ctx context.Context, tenantID uint64, sourceMessageID string) (*types.ChatDocumentArtifact, error)
	GetLatestArtifactBySession(ctx context.Context, tenantID uint64, sessionID string) (*types.ChatDocumentArtifact, error)
	ListArtifactsBySession(ctx context.Context, tenantID uint64, sessionID string, limit int) ([]*types.ChatDocumentArtifact, error)
}

type ChatDocumentArtifactService interface {
	DetectIntent(ctx context.Context, sessionID string, query string, hint string) (*types.DocumentIntentResult, error)
	GetLatestArtifact(ctx context.Context, sessionID string) (*types.ChatDocumentArtifact, error)
	GetArtifact(ctx context.Context, artifactID string) (*types.ChatDocumentArtifact, error)
	GetArtifactBySourceMessageID(ctx context.Context, sourceMessageID string) (*types.ChatDocumentArtifact, error)
	BuildQuotedContext(ctx context.Context, artifact *types.ChatDocumentArtifact, query string, intent string, outputMode string) (string, error)
	RegisterFromAssistantMessage(ctx context.Context, message *types.Message, options types.RegisterChatDocumentArtifactOptions) (*types.ChatDocumentArtifact, error)
	ListBySession(ctx context.Context, sessionID string, limit int) ([]*types.ChatDocumentArtifact, error)
	ListRevisions(ctx context.Context, artifactID string) ([]*types.ChatDocumentArtifact, error)
}
