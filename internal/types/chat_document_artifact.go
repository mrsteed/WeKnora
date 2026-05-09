package types

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	ChatDocumentArtifactKindMarkdown = "chat_markdown"
	ChatDocumentArtifactKindText     = "chat_text"

	ChatDocumentArtifactStatusAvailable = "available"
	ChatDocumentArtifactStatusPartial   = "partial"
	ChatDocumentArtifactStatusFailed    = "failed"

	ChatDocumentIntentNormal     = "normal"
	ChatDocumentIntentContinue   = "continue_document"
	ChatDocumentIntentRevise     = "revise_document"
	ChatDocumentIntentRegenerate = "regenerate_document"

	ChatDocumentOperationCreate     = "create"
	ChatDocumentOperationContinue   = "continue"
	ChatDocumentOperationRevise     = "revise"
	ChatDocumentOperationRegenerate = "regenerate"

	ChatDocumentOutputModeFull  = "full_document"
	ChatDocumentOutputModeDelta = "delta_only"

	ChatDocumentFinalDocumentModeFetchArtifactSnapshot = "fetch_artifact_snapshot"
	ChatDocumentFinalDocumentModeInlineSnapshot        = "inline_snapshot"

	ChatDocumentArtifactSnapshotMaxChars           = 100000
	ChatDocumentArtifactInlineContinuationMaxChars = 80000

	ChatDocumentQualityIssueInlineContextTooLarge   = "inline_context_too_large"
	ChatDocumentQualityIssueSnapshotTruncated       = "snapshot_truncated"
	ChatDocumentQualityIssueUnclosedCodeFence       = "unclosed_code_fence"
	ChatDocumentQualityIssueDeltaMergeUncertain     = "delta_merge_uncertain"
	ChatDocumentQualityIssueRevisionTooShort        = "revision_too_short"
	ChatDocumentQualityIssueRevisionMissingHeading  = "revision_missing_heading"
	ChatDocumentQualityIssueRevisionPreambleTrimmed = "revision_preamble_trimmed"
)

type ChatDocumentStructureInfo struct {
	HeadingCount         int      `json:"heading_count"`
	HeadingTitles        []string `json:"heading_titles,omitempty"`
	HasList              bool     `json:"has_list"`
	HasTable             bool     `json:"has_table"`
	CodeFenceCount       int      `json:"code_fence_count"`
	HasUnclosedCodeFence bool     `json:"has_unclosed_code_fence"`
}

type ChatDocumentArtifact struct {
	ID                string                     `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID          uint64                     `json:"tenant_id" gorm:"index;not null"`
	SessionID         string                     `json:"session_id" gorm:"type:varchar(36);index;not null"`
	SourceMessageID   string                     `json:"source_message_id" gorm:"type:varchar(36);uniqueIndex;not null"`
	SourceRequestID   string                     `json:"source_request_id" gorm:"type:varchar(64);index"`
	ParentArtifactID  string                     `json:"parent_artifact_id,omitempty" gorm:"type:varchar(36);index"`
	RevisionNo        int                        `json:"revision_no" gorm:"not null;default:1"`
	Title             string                     `json:"title" gorm:"type:varchar(255)"`
	ArtifactKind      string                     `json:"artifact_kind" gorm:"type:varchar(32);index;not null"`
	ContentType       string                     `json:"content_type" gorm:"type:varchar(64);not null"`
	ContentSnapshot   string                     `json:"content_snapshot" gorm:"type:text;not null"`
	ContentChecksum   string                     `json:"content_checksum" gorm:"type:varchar(64);index;not null"`
	Status            string                     `json:"status" gorm:"type:varchar(32);index;not null"`
	CompletionStatus  string                     `json:"completion_status" gorm:"type:varchar(32);index"`
	Operation         string                     `json:"operation" gorm:"type:varchar(32);index"`
	CreatedBy         string                     `json:"created_by" gorm:"type:varchar(36);index"`
	CreatedAt         time.Time                  `json:"created_at"`
	UpdatedAt         time.Time                  `json:"updated_at"`
	DeletedAt         gorm.DeletedAt             `json:"deleted_at" gorm:"index"`
	SnapshotCharCount int                        `json:"snapshot_char_count,omitempty" gorm:"-"`
	CanInlineContinue bool                       `json:"can_inline_continue" gorm:"-"`
	QualityIssues     []string                   `json:"quality_issues,omitempty" gorm:"-"`
	UserHint          string                     `json:"user_hint,omitempty" gorm:"-"`
	StructureInfo     *ChatDocumentStructureInfo `json:"structure_info,omitempty" gorm:"-"`
}

type DocumentIntentResult struct {
	Intent    string `json:"intent"`
	Operation string `json:"operation"`
}

type RegisterChatDocumentArtifactOptions struct {
	UserQuery    string
	Intent       string
	Operation    string
	OutputMode   string
	BaseArtifact *ChatDocumentArtifact
}

func (a *ChatDocumentArtifact) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	if a.RevisionNo <= 0 {
		a.RevisionNo = 1
	}
	if a.Status == "" {
		a.Status = ChatDocumentArtifactStatusAvailable
	}
	if a.Operation == "" {
		a.Operation = ChatDocumentOperationCreate
	}
	return nil
}

func (a *ChatDocumentArtifact) CanContinue() bool {
	if a == nil {
		return false
	}
	if a.ArtifactKind != ChatDocumentArtifactKindMarkdown && a.ArtifactKind != ChatDocumentArtifactKindText {
		return false
	}
	switch a.Status {
	case ChatDocumentArtifactStatusAvailable, ChatDocumentArtifactStatusPartial:
		return len([]rune(strings.TrimSpace(a.ContentSnapshot))) <= ChatDocumentArtifactInlineContinuationMaxChars
	default:
		return false
	}
}
