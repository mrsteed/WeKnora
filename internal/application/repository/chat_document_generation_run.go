package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type chatDocumentGenerationRunRepository struct {
	db *gorm.DB
}

func NewChatDocumentGenerationRunRepository(db *gorm.DB) interfaces.ChatDocumentGenerationRunRepository {
	return &chatDocumentGenerationRunRepository{db: db}
}

func (r *chatDocumentGenerationRunRepository) CreateRun(ctx context.Context, run *types.ChatDocumentGenerationRun) error {
	now := time.Now()
	run.CreatedAt = now
	run.UpdatedAt = now
	return r.db.WithContext(ctx).Create(run).Error
}

func (r *chatDocumentGenerationRunRepository) GetRunByID(ctx context.Context, tenantID uint64, runID string) (*types.ChatDocumentGenerationRun, error) {
	var run types.ChatDocumentGenerationRun
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, runID).First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &run, nil
}

func (r *chatDocumentGenerationRunRepository) GetLatestRunBySessionAndRoot(
	ctx context.Context,
	tenantID uint64,
	sessionID string,
	rootMessageID string,
	rootArtifactID string,
) (*types.ChatDocumentGenerationRun, error) {
	query := r.db.WithContext(ctx).Where("tenant_id = ? AND session_id = ?", tenantID, sessionID)
	switch {
	case rootArtifactID != "" && rootMessageID != "":
		query = query.Where("(root_artifact_id = ? OR root_message_id = ?)", rootArtifactID, rootMessageID)
	case rootArtifactID != "":
		query = query.Where("root_artifact_id = ?", rootArtifactID)
	case rootMessageID != "":
		query = query.Where("root_message_id = ?", rootMessageID)
	default:
		return nil, nil
	}

	var run types.ChatDocumentGenerationRun
	if err := query.Order("updated_at DESC").First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &run, nil
}

func (r *chatDocumentGenerationRunRepository) UpdateRun(ctx context.Context, run *types.ChatDocumentGenerationRun) error {
	run.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Model(&types.ChatDocumentGenerationRun{}).
		Where("tenant_id = ? AND id = ?", run.TenantID, run.ID).
		Updates(map[string]interface{}{
			"root_message_id":         run.RootMessageID,
			"root_artifact_id":        run.RootArtifactID,
			"agent_id":                run.AgentID,
			"original_query":          run.OriginalQuery,
			"document_title":          run.DocumentTitle,
			"outline_json":            run.OutlineJSON,
			"budget_json":             run.BudgetJSON,
			"runtime_feedback_json":   run.RuntimeFeedbackJSON,
			"effective_kb_ids_json":   run.EffectiveKBIDsJSON,
			"completed_sections_json": run.CompletedSectionsJSON,
			"status":                  run.Status,
			"auto_continue_round":     run.AutoContinueRound,
			"max_rounds":              run.MaxRounds,
			"model_id":                run.ModelID,
			"updated_at":              run.UpdatedAt,
		}).Error
}
