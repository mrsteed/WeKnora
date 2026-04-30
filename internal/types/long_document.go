package types

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	LongDocumentTaskKindTranslation         = "translation"
	LongDocumentOutputFormatMarkdown        = "markdown"
	LongDocumentTaskIdempotencyKeyMaxLength = 128
)

type LongDocumentTaskStatus string

const (
	LongDocumentTaskStatusPending    LongDocumentTaskStatus = "pending"
	LongDocumentTaskStatusRunning    LongDocumentTaskStatus = "running"
	LongDocumentTaskStatusAssembling LongDocumentTaskStatus = "assembling"
	LongDocumentTaskStatusPartial    LongDocumentTaskStatus = "partial"
	LongDocumentTaskStatusCompleted  LongDocumentTaskStatus = "completed"
	LongDocumentTaskStatusFailed     LongDocumentTaskStatus = "failed"
	LongDocumentTaskStatusCancelled  LongDocumentTaskStatus = "cancelled"
)

type LongDocumentBatchStatus string

const (
	LongDocumentBatchStatusPending   LongDocumentBatchStatus = "pending"
	LongDocumentBatchStatusRunning   LongDocumentBatchStatus = "running"
	LongDocumentBatchStatusCompleted LongDocumentBatchStatus = "completed"
	LongDocumentBatchStatusRetrying  LongDocumentBatchStatus = "retrying"
	LongDocumentBatchStatusSplit     LongDocumentBatchStatus = "split"
	LongDocumentBatchStatusFailed    LongDocumentBatchStatus = "failed"
	LongDocumentBatchStatusSkipped   LongDocumentBatchStatus = "skipped"
)

type LongDocumentArtifactStatus string

const (
	LongDocumentArtifactStatusPending   LongDocumentArtifactStatus = "pending"
	LongDocumentArtifactStatusAvailable LongDocumentArtifactStatus = "available"
	LongDocumentArtifactStatusFailed    LongDocumentArtifactStatus = "failed"
	LongDocumentArtifactStatusExpired   LongDocumentArtifactStatus = "expired"
)

type LongDocumentTaskOptions struct {
	SourceLanguage             string `json:"source_language,omitempty"`
	TargetLanguage             string `json:"target_language,omitempty"`
	SummaryModelID             string `json:"summary_model_id,omitempty"`
	TranslateImages            bool   `json:"translate_images,omitempty"`
	TranslateTables            bool   `json:"translate_tables,omitempty"`
	TranslateReferences        bool   `json:"translate_references,omitempty"`
	PreserveStructure          bool   `json:"preserve_structure,omitempty"`
	RequestedOutputDescription string `json:"requested_output_description,omitempty"`
}

type LongDocumentTask struct {
	ID                 string                   `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID           uint64                   `json:"tenant_id"`
	SessionID          string                   `json:"session_id" gorm:"type:varchar(36);index"`
	KnowledgeID        string                   `json:"knowledge_id" gorm:"type:varchar(36);index"`
	TaskKind           string                   `json:"task_kind" gorm:"type:varchar(32);index"`
	SourceRef          string                   `json:"source_ref" gorm:"type:varchar(255)"`
	SourceSnapshotHash string                   `json:"source_snapshot_hash" gorm:"type:varchar(64)"`
	OutputFormat       string                   `json:"output_format" gorm:"type:varchar(32)"`
	Status             LongDocumentTaskStatus   `json:"status" gorm:"type:varchar(32);index"`
	TotalBatches       int                      `json:"total_batches"`
	CompletedBatches   int                      `json:"completed_batches"`
	FailedBatches      int                      `json:"failed_batches"`
	ArtifactPath       string                   `json:"artifact_path" gorm:"type:text"`
	ArtifactID         string                   `json:"artifact_id" gorm:"type:varchar(36);index"`
	ErrorMessage       string                   `json:"error_message" gorm:"type:text"`
	TaskOptionsJSON    JSON                     `json:"task_options_json,omitempty" gorm:"column:task_options_json;type:json"`
	IdempotencyKey     string                   `json:"idempotency_key" gorm:"type:varchar(128);uniqueIndex"`
	RetryLimit         int                      `json:"retry_limit"`
	QualityStatus      string                   `json:"quality_status" gorm:"type:varchar(32)"`
	CreatedBy          string                   `json:"created_by" gorm:"type:varchar(36)"`
	CompletedAt        *time.Time               `json:"completed_at,omitempty"`
	CancelledAt        *time.Time               `json:"cancelled_at,omitempty"`
	CreatedAt          time.Time                `json:"created_at"`
	UpdatedAt          time.Time                `json:"updated_at"`
	DeletedAt          gorm.DeletedAt           `json:"deleted_at" gorm:"index"`
	Artifact           *LongDocumentArtifact    `json:"artifact,omitempty" gorm:"-"`
	Batches            []*LongDocumentTaskBatch `json:"batches,omitempty" gorm:"-"`
}

type LongDocumentTaskBatch struct {
	ID                  string                  `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID            uint64                  `json:"tenant_id"`
	TaskID              string                  `json:"task_id" gorm:"type:varchar(36);index"`
	BatchNo             int                     `json:"batch_no" gorm:"index"`
	ChunkStartSeq       int                     `json:"chunk_start_seq"`
	ChunkEndSeq         int                     `json:"chunk_end_seq"`
	InputSnapshot       string                  `json:"input_snapshot" gorm:"type:text"`
	OutputPayload       string                  `json:"output_payload" gorm:"type:text"`
	Status              LongDocumentBatchStatus `json:"status" gorm:"type:varchar(32);index"`
	RetryCount          int                     `json:"retry_count"`
	ErrorMessage        string                  `json:"error_message" gorm:"type:text"`
	InputTokenEstimate  int                     `json:"input_token_estimate"`
	OutputTokenEstimate int                     `json:"output_token_estimate"`
	ModelName           string                  `json:"model_name" gorm:"type:varchar(255)"`
	PromptVersion       string                  `json:"prompt_version" gorm:"type:varchar(64)"`
	QualityStatus       string                  `json:"quality_status" gorm:"type:varchar(32)"`
	StartedAt           *time.Time              `json:"started_at,omitempty"`
	CompletedAt         *time.Time              `json:"completed_at,omitempty"`
	CreatedAt           time.Time               `json:"created_at"`
	UpdatedAt           time.Time               `json:"updated_at"`
	DeletedAt           gorm.DeletedAt          `json:"deleted_at" gorm:"index"`
}

type LongDocumentArtifact struct {
	ID             string                     `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID       uint64                     `json:"tenant_id"`
	TaskID         string                     `json:"task_id" gorm:"type:varchar(36);index"`
	FileName       string                     `json:"file_name" gorm:"type:varchar(255)"`
	FilePath       string                     `json:"file_path" gorm:"type:text"`
	FileType       string                     `json:"file_type" gorm:"type:varchar(64)"`
	FileSize       int64                      `json:"file_size"`
	Checksum       string                     `json:"checksum" gorm:"type:varchar(64)"`
	StorageBackend string                     `json:"storage_backend" gorm:"type:varchar(64)"`
	Status         LongDocumentArtifactStatus `json:"status" gorm:"type:varchar(32);index"`
	CreatedAt      time.Time                  `json:"created_at"`
	ExpiresAt      *time.Time                 `json:"expires_at,omitempty"`
	DeletedAt      gorm.DeletedAt             `json:"deleted_at" gorm:"index"`
}

type LongDocumentTaskEvent struct {
	Type      string                 `json:"type"`
	TaskID    string                 `json:"task_id"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

type LongDocumentTaskPayload struct {
	TracingContext `json:",inline"`
	TaskID         string `json:"task_id"`
	TenantID       uint64 `json:"tenant_id"`
	SessionID      string `json:"session_id"`
	KnowledgeID    string `json:"knowledge_id"`
}

type CreateLongDocumentTaskRequest struct {
	SessionID      string                  `json:"session_id" binding:"required"`
	KnowledgeID    string                  `json:"knowledge_id" binding:"required"`
	TaskKind       string                  `json:"task_kind,omitempty"`
	OutputFormat   string                  `json:"output_format,omitempty"`
	UserQuery      string                  `json:"user_query" binding:"required"`
	SummaryModelID string                  `json:"summary_model_id,omitempty"`
	IdempotencyKey string                  `json:"idempotency_key,omitempty"`
	Channel        string                  `json:"channel,omitempty"`
	Options        LongDocumentTaskOptions `json:"options,omitempty"`
}

type CreateLongDocumentTaskResponse struct {
	TaskID string                 `json:"task_id"`
	Status LongDocumentTaskStatus `json:"status"`
	Task   *LongDocumentTask      `json:"task,omitempty"`
}

func (t *LongDocumentTask) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	if t.Status == "" {
		t.Status = LongDocumentTaskStatusPending
	}
	if t.OutputFormat == "" {
		t.OutputFormat = LongDocumentOutputFormatMarkdown
	}
	if t.RetryLimit <= 0 {
		t.RetryLimit = 3
	}
	if t.TaskKind == "" {
		t.TaskKind = LongDocumentTaskKindTranslation
	}
	return nil
}

func (b *LongDocumentTaskBatch) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	if b.Status == "" {
		b.Status = LongDocumentBatchStatusPending
	}
	return nil
}

func (a *LongDocumentArtifact) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	if a.Status == "" {
		a.Status = LongDocumentArtifactStatusPending
	}
	return nil
}

func (t *LongDocumentTask) Options() (*LongDocumentTaskOptions, error) {
	if len(t.TaskOptionsJSON) == 0 {
		return &LongDocumentTaskOptions{}, nil
	}
	var options LongDocumentTaskOptions
	if err := json.Unmarshal(t.TaskOptionsJSON, &options); err != nil {
		return nil, err
	}
	return &options, nil
}

func (t *LongDocumentTask) SetOptions(options *LongDocumentTaskOptions) error {
	if options == nil {
		t.TaskOptionsJSON = nil
		return nil
	}
	encoded, err := json.Marshal(options)
	if err != nil {
		return err
	}
	t.TaskOptionsJSON = JSON(encoded)
	return nil
}

func (t *LongDocumentTask) ProgressPercent() int {
	if t.TotalBatches <= 0 {
		return 0
	}
	progress := (t.CompletedBatches * 100) / t.TotalBatches
	if progress < 0 {
		return 0
	}
	if progress > 100 {
		return 100
	}
	return progress
}

func (t *LongDocumentTask) IsTerminal() bool {
	switch t.Status {
	case LongDocumentTaskStatusCompleted,
		LongDocumentTaskStatusPartial,
		LongDocumentTaskStatusFailed,
		LongDocumentTaskStatusCancelled:
		return true
	default:
		return false
	}
}

func BuildLongDocumentTaskIdempotencyKey(tenantID uint64, sessionID, knowledgeID, taskKind, userQuery, summaryModelID string) string {
	raw := fmt.Sprintf("%d:%s:%s:%s:%s:%s", tenantID, sessionID, knowledgeID, taskKind, strings.TrimSpace(userQuery), strings.TrimSpace(summaryModelID))
	return NormalizeLongDocumentTaskIdempotencyKey(raw)
}

func NormalizeLongDocumentTaskIdempotencyKey(key string) string {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= LongDocumentTaskIdempotencyKeyMaxLength {
		return trimmed
	}
	sum := sha256.Sum256([]byte(trimmed))
	return fmt.Sprintf("ldt:%x", sum)
}
