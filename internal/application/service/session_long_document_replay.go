package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

func chatDocumentGenerationStatusFromRunStatus(status string) string {
	switch types.NormalizeChatDocumentGenerationRunStatus(status) {
	case types.ChatDocumentGenerationRunStatusCompleted:
		return types.ChatDocumentGenerationStatusCompleted
	case types.ChatDocumentGenerationRunStatusBlocked:
		return types.ChatDocumentGenerationStatusBlocked
	case types.ChatDocumentGenerationRunStatusNeedsReview:
		return types.ChatDocumentGenerationStatusNeedsReview
	case types.ChatDocumentGenerationRunStatusContinuing,
		types.ChatDocumentGenerationRunStatusPlanning,
		types.ChatDocumentGenerationRunStatusWriting:
		return types.ChatDocumentGenerationStatusContinuing
	default:
		return ""
	}
}

func buildHistoricalPlanningOutlineExtra(run *types.ChatDocumentGenerationRun) map[string]interface{} {
	if run == nil || len(run.OutlineJSON) == 0 {
		return nil
	}
	var outline dedicatedFullDocumentOutline
	if err := json.Unmarshal(run.OutlineJSON, &outline); err != nil {
		return nil
	}
	outline = normalizeDedicatedFullDocumentOutline(outline)
	if strings.TrimSpace(outline.Title) == "" && len(outline.Sections) == 0 {
		return nil
	}
	return withPlanningOutlineExtra(nil, outline, longDocumentOutlineSourceGenerationRun)
}

func mergeReplayExtra(base map[string]interface{}, overlay map[string]interface{}) map[string]interface{} {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	if base == nil {
		base = map[string]interface{}{}
	}
	for key, value := range overlay {
		base[key] = value
	}
	return base
}

func buildHistoricalLongDocumentReplayExtraFromRun(run *types.ChatDocumentGenerationRun, artifact *types.ChatDocumentArtifact) map[string]interface{} {
	if run == nil && artifact == nil {
		return nil
	}
	extra := map[string]interface{}{}
	if run != nil {
		extra["generation_run_id"] = run.ID
		extra["long_document_enabled"] = true
		extra = mergeReplayExtra(extra, buildHistoricalPlanningOutlineExtra(run))
		if status := chatDocumentGenerationStatusFromRunStatus(run.Status); status != "" {
			extra["document_generation_status"] = status
		}
	}
	if artifact != nil {
		extra["long_document_enabled"] = true
		if status := types.NormalizeOptionalChatDocumentGenerationStatus(artifact.DocumentGenerationStatus); status != "" {
			extra["document_generation_status"] = status
		}
		if taskKind := strings.TrimSpace(artifact.DocumentTaskKind); taskKind != "" {
			extra["document_task_kind"] = taskKind
		}
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

func (s *sessionService) BuildHistoricalLongDocumentReplayExtra(
	ctx context.Context,
	message *types.Message,
	artifact *types.ChatDocumentArtifact,
	rootArtifactID string,
) (map[string]interface{}, error) {
	if s == nil || s.generationRunRepo == nil || message == nil {
		return nil, nil
	}
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return nil, nil
	}
	rootArtifactID = strings.TrimSpace(rootArtifactID)
	if rootArtifactID == "" && artifact != nil {
		rootArtifactID = strings.TrimSpace(artifact.ID)
	}
	run, err := s.generationRunRepo.GetLatestRunBySessionAndRoot(
		ctx,
		tenantID,
		strings.TrimSpace(message.SessionID),
		strings.TrimSpace(message.ID),
		rootArtifactID,
	)
	if err != nil {
		return nil, err
	}
	return buildHistoricalLongDocumentReplayExtraFromRun(run, artifact), nil
}
