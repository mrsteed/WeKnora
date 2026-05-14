package types

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	ChatDocumentGenerationRunStatusPlanning    = "planning"
	ChatDocumentGenerationRunStatusWriting     = "writing"
	ChatDocumentGenerationRunStatusContinuing  = "continuing"
	ChatDocumentGenerationRunStatusCompleted   = "completed"
	ChatDocumentGenerationRunStatusBlocked     = "blocked"
	ChatDocumentGenerationRunStatusNeedsReview = "needs_review"
	ChatDocumentGenerationRunStatusFailed      = "failed"
	ChatDocumentGenerationRunStatusCancelled   = "cancelled"

	ChatDocumentGenerationRunDefaultMaxRounds = 8
)

type ChatDocumentGenerationRunState struct {
	TaskKind                string `json:"task_kind,omitempty"`
	ActiveArtifactID        string `json:"active_artifact_id,omitempty"`
	LastCompletionStatus    string `json:"last_completion_status,omitempty"`
	LastFinishReason        string `json:"last_finish_reason,omitempty"`
	LastFailureReason       string `json:"last_failure_reason,omitempty"`
	LastDocumentStatus      string `json:"last_document_generation_status,omitempty"`
	LastAutoContinueReason  string `json:"last_auto_continue_reason,omitempty"`
	AutoContinueRound       int    `json:"auto_continue_round,omitempty"`
	MaxAutoContinueRounds   int    `json:"max_auto_continue_rounds,omitempty"`
	MinGrowthChars          int    `json:"min_growth_chars,omitempty"`
	MaxLowGrowthRounds      int    `json:"max_low_growth_rounds,omitempty"`
	LastSnapshotCharCount   int    `json:"last_snapshot_char_count,omitempty"`
	LowGrowthRounds         int    `json:"low_growth_rounds,omitempty"`
	CompletedCount          int    `json:"completed_count,omitempty"`
	RemainingCount          int    `json:"remaining_count,omitempty"`
	NextSourceChunkStartSeq int    `json:"next_source_chunk_start_seq,omitempty"`
	NextSourceChunkEndSeq   int    `json:"next_source_chunk_end_seq,omitempty"`
	NextSection             string `json:"next_section,omitempty"`
}

func NormalizeChatDocumentGenerationRunState(state ChatDocumentGenerationRunState) ChatDocumentGenerationRunState {
	state.TaskKind = strings.TrimSpace(state.TaskKind)
	state.ActiveArtifactID = strings.TrimSpace(state.ActiveArtifactID)
	state.LastCompletionStatus = strings.TrimSpace(state.LastCompletionStatus)
	state.LastFinishReason = strings.TrimSpace(state.LastFinishReason)
	state.LastFailureReason = strings.TrimSpace(state.LastFailureReason)
	state.LastDocumentStatus = strings.TrimSpace(state.LastDocumentStatus)
	state.LastAutoContinueReason = strings.TrimSpace(state.LastAutoContinueReason)
	state.NextSection = strings.TrimSpace(state.NextSection)
	if state.AutoContinueRound < 0 {
		state.AutoContinueRound = 0
	}
	if state.MaxAutoContinueRounds < 0 {
		state.MaxAutoContinueRounds = 0
	}
	if state.MinGrowthChars < 0 {
		state.MinGrowthChars = 0
	}
	if state.MaxLowGrowthRounds < 0 {
		state.MaxLowGrowthRounds = 0
	}
	if state.LastSnapshotCharCount < 0 {
		state.LastSnapshotCharCount = 0
	}
	if state.LowGrowthRounds < 0 {
		state.LowGrowthRounds = 0
	}
	if state.CompletedCount < 0 {
		state.CompletedCount = 0
	}
	if state.RemainingCount < 0 {
		state.RemainingCount = 0
	}
	if state.NextSourceChunkStartSeq < 0 {
		state.NextSourceChunkStartSeq = 0
	}
	if state.NextSourceChunkEndSeq < 0 {
		state.NextSourceChunkEndSeq = 0
	}
	return state
}

func (state ChatDocumentGenerationRunState) IsZero() bool {
	normalized := NormalizeChatDocumentGenerationRunState(state)
	return normalized == (ChatDocumentGenerationRunState{})
}

func (state ChatDocumentGenerationRunState) Data() map[string]interface{} {
	normalized := NormalizeChatDocumentGenerationRunState(state)
	if normalized.IsZero() {
		return nil
	}
	data := map[string]interface{}{}
	if normalized.TaskKind != "" {
		data["task_kind"] = normalized.TaskKind
	}
	if normalized.ActiveArtifactID != "" {
		data["active_artifact_id"] = normalized.ActiveArtifactID
	}
	if normalized.LastCompletionStatus != "" {
		data["last_completion_status"] = normalized.LastCompletionStatus
	}
	if normalized.LastFinishReason != "" {
		data["last_finish_reason"] = normalized.LastFinishReason
	}
	if normalized.LastFailureReason != "" {
		data["last_failure_reason"] = normalized.LastFailureReason
	}
	if normalized.LastDocumentStatus != "" {
		data["last_document_generation_status"] = normalized.LastDocumentStatus
	}
	if normalized.LastAutoContinueReason != "" {
		data["last_auto_continue_reason"] = normalized.LastAutoContinueReason
	}
	if normalized.AutoContinueRound > 0 {
		data["auto_continue_round"] = normalized.AutoContinueRound
	}
	if normalized.MaxAutoContinueRounds > 0 {
		data["max_auto_continue_rounds"] = normalized.MaxAutoContinueRounds
	}
	if normalized.MinGrowthChars > 0 {
		data["min_growth_chars"] = normalized.MinGrowthChars
	}
	if normalized.MaxLowGrowthRounds > 0 {
		data["max_low_growth_rounds"] = normalized.MaxLowGrowthRounds
	}
	if normalized.LastSnapshotCharCount > 0 {
		data["last_snapshot_char_count"] = normalized.LastSnapshotCharCount
	}
	if normalized.LowGrowthRounds > 0 {
		data["low_growth_rounds"] = normalized.LowGrowthRounds
	}
	if normalized.CompletedCount > 0 {
		data["completed_count"] = normalized.CompletedCount
	}
	if normalized.RemainingCount > 0 {
		data["remaining_count"] = normalized.RemainingCount
	}
	if normalized.NextSourceChunkStartSeq > 0 {
		data["next_source_chunk_start_seq"] = normalized.NextSourceChunkStartSeq
	}
	if normalized.NextSourceChunkEndSeq > 0 {
		data["next_source_chunk_end_seq"] = normalized.NextSourceChunkEndSeq
	}
	if normalized.NextSection != "" {
		data["next_section"] = normalized.NextSection
	}
	return data
}

type ChatDocumentGenerationRun struct {
	ID                    string         `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID              uint64         `json:"tenant_id" gorm:"index;not null"`
	SessionID             string         `json:"session_id" gorm:"type:varchar(36);index;not null"`
	RootMessageID         string         `json:"root_message_id,omitempty" gorm:"type:varchar(36);index"`
	RootArtifactID        string         `json:"root_artifact_id,omitempty" gorm:"type:varchar(36);index"`
	AgentID               string         `json:"agent_id,omitempty" gorm:"type:varchar(36);index"`
	OriginalQuery         string         `json:"original_query" gorm:"type:text;not null"`
	DocumentTitle         string         `json:"document_title,omitempty" gorm:"type:varchar(255)"`
	OutlineJSON           JSON           `json:"outline_json,omitempty" gorm:"column:outline_json;type:jsonb"`
	BudgetJSON            JSON           `json:"budget_json,omitempty" gorm:"column:budget_json;type:jsonb"`
	RuntimeFeedbackJSON   JSON           `json:"runtime_feedback_json,omitempty" gorm:"column:runtime_feedback_json;type:jsonb"`
	EffectiveKBIDsJSON    JSON           `json:"effective_kb_ids_json,omitempty" gorm:"column:effective_kb_ids_json;type:jsonb"`
	CompletedSectionsJSON JSON           `json:"completed_sections_json,omitempty" gorm:"column:completed_sections_json;type:jsonb"`
	Status                string         `json:"status" gorm:"type:varchar(32);index;not null"`
	AutoContinueRound     int            `json:"auto_continue_round" gorm:"not null;default:0"`
	MaxRounds             int            `json:"max_rounds" gorm:"not null;default:8"`
	ModelID               string         `json:"model_id,omitempty" gorm:"type:varchar(128)"`
	CreatedBy             string         `json:"created_by,omitempty" gorm:"type:varchar(36);index"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DeletedAt             gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

func NormalizeChatDocumentGenerationRunStatus(status string) string {
	switch strings.TrimSpace(status) {
	case ChatDocumentGenerationRunStatusPlanning:
		return ChatDocumentGenerationRunStatusPlanning
	case ChatDocumentGenerationRunStatusWriting:
		return ChatDocumentGenerationRunStatusWriting
	case ChatDocumentGenerationRunStatusContinuing:
		return ChatDocumentGenerationRunStatusContinuing
	case ChatDocumentGenerationRunStatusCompleted:
		return ChatDocumentGenerationRunStatusCompleted
	case ChatDocumentGenerationRunStatusBlocked:
		return ChatDocumentGenerationRunStatusBlocked
	case ChatDocumentGenerationRunStatusNeedsReview:
		return ChatDocumentGenerationRunStatusNeedsReview
	case ChatDocumentGenerationRunStatusFailed:
		return ChatDocumentGenerationRunStatusFailed
	case ChatDocumentGenerationRunStatusCancelled:
		return ChatDocumentGenerationRunStatusCancelled
	default:
		return ChatDocumentGenerationRunStatusPlanning
	}
}

func (r *ChatDocumentGenerationRun) BeforeCreate(tx *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	if r.Status == "" {
		r.Status = ChatDocumentGenerationRunStatusPlanning
	}
	if r.MaxRounds <= 0 {
		r.MaxRounds = ChatDocumentGenerationRunDefaultMaxRounds
	}
	return nil
}
