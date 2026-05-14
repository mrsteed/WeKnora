package service

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type documentGenerationArtifactBinder interface {
	BindKnowledgeGroundedGenerationRunArtifact(ctx context.Context, runID string, artifact *types.ChatDocumentArtifact) error
}

type documentGenerationRunStateRecorder interface {
	RecordChatDocumentGenerationRunState(ctx context.Context, runID string, update types.ChatDocumentGenerationRunState) (*types.ChatDocumentGenerationRunState, error)
}

type DocumentGenerationRunStateBuilder func(artifact *types.ChatDocumentArtifact) types.ChatDocumentGenerationRunState

type DocumentGenerationAggregateInput struct {
	Message         *types.Message
	RegisterOptions types.RegisterChatDocumentArtifactOptions
	StateBuilder    DocumentGenerationRunStateBuilder
}

type DocumentGenerationAggregateResult struct {
	Artifact    *types.ChatDocumentArtifact
	State       *types.ChatDocumentGenerationRunState
	ArtifactErr error
	BindErr     error
	StateErr    error
}

func AggregateDocumentGenerationArtifact(
	ctx context.Context,
	sessionService interfaces.SessionService,
	artifactService interfaces.ChatDocumentArtifactService,
	input DocumentGenerationAggregateInput,
) DocumentGenerationAggregateResult {
	result := DocumentGenerationAggregateResult{}
	if artifactService != nil {
		result.Artifact, result.ArtifactErr = artifactService.RegisterFromAssistantMessage(ctx, input.Message, input.RegisterOptions)
		if result.Artifact != nil {
			if binder, ok := sessionService.(documentGenerationArtifactBinder); ok && strings.TrimSpace(input.RegisterOptions.GenerationRunID) != "" {
				result.BindErr = binder.BindKnowledgeGroundedGenerationRunArtifact(ctx, input.RegisterOptions.GenerationRunID, result.Artifact)
			}
		}
	}
	if recorder, ok := sessionService.(documentGenerationRunStateRecorder); ok && strings.TrimSpace(input.RegisterOptions.GenerationRunID) != "" && input.StateBuilder != nil {
		update := input.StateBuilder(result.Artifact)
		result.State, result.StateErr = recorder.RecordChatDocumentGenerationRunState(ctx, input.RegisterOptions.GenerationRunID, update)
	}
	return result
}
