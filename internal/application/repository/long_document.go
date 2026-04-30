package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type longDocumentTaskRepository struct {
	db *gorm.DB
}

func NewLongDocumentTaskRepository(db *gorm.DB) interfaces.LongDocumentTaskRepository {
	return &longDocumentTaskRepository{db: db}
}

func (r *longDocumentTaskRepository) CreateTask(ctx context.Context, task *types.LongDocumentTask) error {
	task.CreatedAt = time.Now()
	task.UpdatedAt = task.CreatedAt
	return r.db.WithContext(ctx).Create(task).Error
}

func (r *longDocumentTaskRepository) CreateBatches(ctx context.Context, batches []*types.LongDocumentTaskBatch) error {
	if len(batches) == 0 {
		return nil
	}
	now := time.Now()
	for _, batch := range batches {
		batch.CreatedAt = now
		batch.UpdatedAt = now
	}
	return r.db.WithContext(ctx).Create(&batches).Error
}

func (r *longDocumentTaskRepository) CreateArtifact(ctx context.Context, artifact *types.LongDocumentArtifact) error {
	artifact.CreatedAt = time.Now()
	return r.db.WithContext(ctx).Create(artifact).Error
}

func (r *longDocumentTaskRepository) GetTaskByID(ctx context.Context, tenantID uint64, taskID string) (*types.LongDocumentTask, error) {
	var task types.LongDocumentTask
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, taskID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &task, nil
}

func (r *longDocumentTaskRepository) GetTaskByIdempotencyKey(ctx context.Context, tenantID uint64, key string) (*types.LongDocumentTask, error) {
	var task types.LongDocumentTask
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND idempotency_key = ?", tenantID, key).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &task, nil
}

func (r *longDocumentTaskRepository) ListTasksBySession(ctx context.Context, tenantID uint64, sessionID string, page *types.Pagination) ([]*types.LongDocumentTask, int64, error) {
	var (
		tasks []*types.LongDocumentTask
		total int64
	)
	base := r.db.WithContext(ctx).Model(&types.LongDocumentTask{}).Where("tenant_id = ? AND session_id = ?", tenantID, sessionID)
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := base.Order("created_at ASC").Offset(page.Offset()).Limit(page.Limit()).Find(&tasks).Error; err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

func (r *longDocumentTaskRepository) ListBatchesByTaskID(ctx context.Context, tenantID uint64, taskID string) ([]*types.LongDocumentTaskBatch, error) {
	var batches []*types.LongDocumentTaskBatch
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND task_id = ?", tenantID, taskID).Order("batch_no ASC").Find(&batches).Error; err != nil {
		return nil, err
	}
	return batches, nil
}

func (r *longDocumentTaskRepository) GetArtifactByTaskID(ctx context.Context, tenantID uint64, taskID string) (*types.LongDocumentArtifact, error) {
	var artifact types.LongDocumentArtifact
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND task_id = ?", tenantID, taskID).First(&artifact).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &artifact, nil
}

func (r *longDocumentTaskRepository) UpdateTask(ctx context.Context, task *types.LongDocumentTask) error {
	task.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Model(&types.LongDocumentTask{}).
		Where("tenant_id = ? AND id = ?", task.TenantID, task.ID).
		Updates(map[string]interface{}{
			"status":            task.Status,
			"total_batches":     task.TotalBatches,
			"completed_batches": task.CompletedBatches,
			"failed_batches":    task.FailedBatches,
			"artifact_path":     task.ArtifactPath,
			"artifact_id":       task.ArtifactID,
			"error_message":     task.ErrorMessage,
			"quality_status":    task.QualityStatus,
			"completed_at":      task.CompletedAt,
			"cancelled_at":      task.CancelledAt,
			"updated_at":        task.UpdatedAt,
		}).Error
}

func (r *longDocumentTaskRepository) UpdateBatch(ctx context.Context, batch *types.LongDocumentTaskBatch) error {
	batch.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Model(&types.LongDocumentTaskBatch{}).
		Where("tenant_id = ? AND id = ?", batch.TenantID, batch.ID).
		Updates(map[string]interface{}{
			"status":                batch.Status,
			"output_payload":        batch.OutputPayload,
			"retry_count":           batch.RetryCount,
			"error_message":         batch.ErrorMessage,
			"input_token_estimate":  batch.InputTokenEstimate,
			"output_token_estimate": batch.OutputTokenEstimate,
			"model_name":            batch.ModelName,
			"prompt_version":        batch.PromptVersion,
			"quality_status":        batch.QualityStatus,
			"started_at":            batch.StartedAt,
			"completed_at":          batch.CompletedAt,
			"updated_at":            batch.UpdatedAt,
		}).Error
}

func (r *longDocumentTaskRepository) UpdateArtifact(ctx context.Context, artifact *types.LongDocumentArtifact) error {
	return r.db.WithContext(ctx).Model(&types.LongDocumentArtifact{}).
		Where("tenant_id = ? AND id = ?", artifact.TenantID, artifact.ID).
		Updates(map[string]interface{}{
			"file_name":       artifact.FileName,
			"file_path":       artifact.FilePath,
			"file_type":       artifact.FileType,
			"file_size":       artifact.FileSize,
			"checksum":        artifact.Checksum,
			"storage_backend": artifact.StorageBackend,
			"status":          artifact.Status,
			"expires_at":      artifact.ExpiresAt,
		}).Error
}

func (r *longDocumentTaskRepository) DeleteArtifactByTaskID(ctx context.Context, tenantID uint64, taskID string) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND task_id = ?", tenantID, taskID).
		Delete(&types.LongDocumentArtifact{}).Error
}

func (r *longDocumentTaskRepository) ReplaceArtifact(ctx context.Context, task *types.LongDocumentTask, artifact *types.LongDocumentArtifact) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND task_id = ?", task.TenantID, task.ID).Delete(&types.LongDocumentArtifact{}).Error; err != nil {
			return err
		}
		artifact.CreatedAt = time.Now()
		if err := tx.Create(artifact).Error; err != nil {
			return err
		}
		task.UpdatedAt = time.Now()
		return tx.Model(&types.LongDocumentTask{}).Where("tenant_id = ? AND id = ?", task.TenantID, task.ID).Updates(map[string]interface{}{
			"artifact_id":   artifact.ID,
			"artifact_path": artifact.FilePath,
			"updated_at":    task.UpdatedAt,
		}).Error
	})
}

func (r *longDocumentTaskRepository) DeleteTask(ctx context.Context, tenantID uint64, taskID string) error {
	return r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, taskID).Delete(&types.LongDocumentTask{}).Error
}
