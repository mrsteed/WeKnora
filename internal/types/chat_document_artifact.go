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

	ChatDocumentMergeModeAppendToSection = "append_to_section"
	ChatDocumentMergeModeReplaceSection  = "replace_section"
	ChatDocumentMergeModeAppendToTail    = "append_to_document_tail"

	ChatDocumentOutputModeFull  = "full_document"
	ChatDocumentOutputModeDelta = "delta_only"

	ChatDocumentFinalDocumentModeFetchArtifactSnapshot = "fetch_artifact_snapshot"
	ChatDocumentFinalDocumentModeInlineSnapshot        = "inline_snapshot"

	ChatDocumentCompletionMarker = "<!-- document_complete -->"

	ChatDocumentGenerationStatusContinuing  = "continuing"
	ChatDocumentGenerationStatusCompleted   = "completed"
	ChatDocumentGenerationStatusBlocked     = "blocked"
	ChatDocumentGenerationStatusNeedsReview = "needs_review"

	ChatDocumentContinuationContextModeInlineFull  = "inline_full"
	ChatDocumentContinuationContextModeOutlineTail = "outline_tail"

	ChatDocumentArtifactSnapshotMaxChars           = 1200000
	ChatDocumentArtifactInlineContinuationMaxChars = 80000
	ChatDocumentArtifactContinuationMaxChars       = 1200000

	ChatDocumentQualityIssueInlineContextTooLarge       = "inline_context_too_large"
	ChatDocumentQualityIssueSnapshotTruncated           = "snapshot_truncated"
	ChatDocumentQualityIssueUnclosedCodeFence           = "unclosed_code_fence"
	ChatDocumentQualityIssueDeltaMergeUncertain         = "delta_merge_uncertain"
	ChatDocumentQualityIssueTargetSectionUncertain      = "target_section_uncertain"
	ChatDocumentQualityIssueRevisionTooShort            = "revision_too_short"
	ChatDocumentQualityIssueRevisionMissingHeading      = "revision_missing_heading"
	ChatDocumentQualityIssueRevisionPreambleTrimmed     = "revision_preamble_trimmed"
	ChatDocumentQualityIssueDuplicateDocumentHead       = "duplicate_document_head"
	ChatDocumentQualityIssueSectionNumberReset          = "section_number_reset"
	ChatDocumentQualityIssueLowNoveltyDelta             = "low_novelty_delta"
	ChatDocumentQualityIssueTerminalSectionTail         = "terminal_section_tail"
	ChatDocumentQualityIssueMarkdownHeadingNormalized   = "markdown_heading_normalized"
	ChatDocumentQualityIssueMarkdownStructureInvalid    = "markdown_structure_invalid"
	ChatDocumentQualityIssueInternalPromptLeakage       = "internal_prompt_leakage"
	ChatDocumentQualityIssueMarkdownUnplannedSubsection = "markdown_unplanned_subsection"
	ChatDocumentQualityIssueMarkdownTooShort            = "markdown_too_short"
)

type ChatDocumentStructureInfo struct {
	HeadingCount         int      `json:"heading_count"`
	HeadingTitles        []string `json:"heading_titles,omitempty"`
	HasList              bool     `json:"has_list"`
	HasTable             bool     `json:"has_table"`
	CodeFenceCount       int      `json:"code_fence_count"`
	HasUnclosedCodeFence bool     `json:"has_unclosed_code_fence"`
}

type ChatDocumentEvidenceSourceSummary struct {
	KnowledgeBaseID string  `json:"knowledge_base_id,omitempty"`
	KnowledgeID     string  `json:"knowledge_id,omitempty"`
	SourceTitle     string  `json:"source_title,omitempty"`
	ChunkCount      int     `json:"chunk_count"`
	MaxScore        float64 `json:"max_score,omitempty"`
}

type ChatDocumentEvidenceSummary struct {
	RefCount           int                                 `json:"ref_count"`
	KnowledgeBaseCount int                                 `json:"knowledge_base_count"`
	KnowledgeCount     int                                 `json:"knowledge_count"`
	ChunkCount         int                                 `json:"chunk_count"`
	Sources            []ChatDocumentEvidenceSourceSummary `json:"sources,omitempty"`
}

type ChatDocumentArtifact struct {
	ID                       string                       `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID                 uint64                       `json:"tenant_id" gorm:"index;not null"`
	SessionID                string                       `json:"session_id" gorm:"type:varchar(36);index;not null"`
	SourceMessageID          string                       `json:"source_message_id" gorm:"type:varchar(36);uniqueIndex;not null"`
	SourceRequestID          string                       `json:"source_request_id" gorm:"type:varchar(64);index"`
	ParentArtifactID         string                       `json:"parent_artifact_id,omitempty" gorm:"type:varchar(36);index"`
	RevisionNo               int                          `json:"revision_no" gorm:"not null;default:1"`
	Title                    string                       `json:"title" gorm:"type:varchar(255)"`
	ArtifactKind             string                       `json:"artifact_kind" gorm:"type:varchar(32);index;not null"`
	ContentType              string                       `json:"content_type" gorm:"type:varchar(64);not null"`
	ContentSnapshot          string                       `json:"content_snapshot" gorm:"type:text;not null"`
	ContentChecksum          string                       `json:"content_checksum" gorm:"type:varchar(64);index;not null"`
	Status                   string                       `json:"status" gorm:"type:varchar(32);index;not null"`
	CompletionStatus         string                       `json:"completion_status" gorm:"type:varchar(32);index"`
	DocumentGenerationStatus string                       `json:"document_generation_status,omitempty" gorm:"type:varchar(32);index"`
	DocumentTaskKind         string                       `json:"document_task_kind,omitempty" gorm:"type:varchar(32);index"`
	SourceTitle              string                       `json:"source_title,omitempty" gorm:"type:varchar(255)"`
	TargetLanguage           string                       `json:"target_language,omitempty" gorm:"type:varchar(128)"`
	OutputFormat             string                       `json:"output_format,omitempty" gorm:"type:varchar(32)"`
	Operation                string                       `json:"operation" gorm:"type:varchar(32);index"`
	CreatedBy                string                       `json:"created_by" gorm:"type:varchar(36);index"`
	CreatedAt                time.Time                    `json:"created_at"`
	UpdatedAt                time.Time                    `json:"updated_at"`
	DeletedAt                gorm.DeletedAt               `json:"deleted_at" gorm:"index"`
	EvidenceRefs             []ChatDocumentEvidenceRef    `json:"evidence_refs,omitempty" gorm:"-"`
	EvidenceSummary          *ChatDocumentEvidenceSummary `json:"evidence_summary,omitempty" gorm:"-"`
	SnapshotCharCount        int                          `json:"snapshot_char_count,omitempty" gorm:"-"`
	CanContinueDocument      bool                         `json:"can_continue" gorm:"-"`
	CanInlineContinue        bool                         `json:"can_inline_continue" gorm:"-"`
	ContinuationContextMode  string                       `json:"continuation_context_mode,omitempty" gorm:"-"`
	QualityIssues            []string                     `json:"quality_issues,omitempty" gorm:"-"`
	UserHint                 string                       `json:"user_hint,omitempty" gorm:"-"`
	StructureInfo            *ChatDocumentStructureInfo   `json:"structure_info,omitempty" gorm:"-"`
}

type DocumentIntentResult struct {
	Intent        string `json:"intent"`
	Operation     string `json:"operation"`
	TargetHeading string `json:"target_heading,omitempty"`
	MergeMode     string `json:"merge_mode,omitempty"`
}

type RegisterChatDocumentArtifactOptions struct {
	UserQuery                string
	Intent                   string
	Operation                string
	OutputMode               string
	DocumentTaskKind         string
	SourceTitle              string
	TargetLanguage           string
	TranslationOutputFormat  string
	NeedArtifact             bool
	UseLongDocument          bool
	TargetHeading            string
	MergeMode                string
	DocumentGenerationStatus string
	QualityIssues            []string
	LocalKnowledgeUsed       bool
	EvidenceRefs             []ChatDocumentEvidenceRef
	GenerationRunID          string
	BaseArtifact             *ChatDocumentArtifact
}

func StripChatDocumentCompletionMarker(content string) (string, bool) {
	if !strings.Contains(content, ChatDocumentCompletionMarker) {
		return content, false
	}
	cleaned := strings.ReplaceAll(content, ChatDocumentCompletionMarker, "")
	return strings.TrimSpace(cleaned), true
}

func NormalizeChatDocumentGenerationStatus(status string) string {
	switch strings.TrimSpace(status) {
	case ChatDocumentGenerationStatusCompleted:
		return ChatDocumentGenerationStatusCompleted
	case ChatDocumentGenerationStatusBlocked:
		return ChatDocumentGenerationStatusBlocked
	case ChatDocumentGenerationStatusNeedsReview:
		return ChatDocumentGenerationStatusNeedsReview
	case ChatDocumentGenerationStatusContinuing:
		return ChatDocumentGenerationStatusContinuing
	default:
		return ChatDocumentGenerationStatusContinuing
	}
}

func NormalizeOptionalChatDocumentGenerationStatus(status string) string {
	if strings.TrimSpace(status) == "" {
		return ""
	}
	return NormalizeChatDocumentGenerationStatus(status)
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
	status := NormalizeOptionalChatDocumentGenerationStatus(a.DocumentGenerationStatus)
	if status == ChatDocumentGenerationStatusBlocked || status == ChatDocumentGenerationStatusNeedsReview {
		return false
	}
	switch a.Status {
	case ChatDocumentArtifactStatusAvailable, ChatDocumentArtifactStatusPartial:
		return len([]rune(strings.TrimSpace(a.ContentSnapshot))) < ChatDocumentArtifactContinuationMaxChars
	default:
		return false
	}
}

func (a *ChatDocumentArtifact) ContinuationMode() string {
	if a == nil || !a.CanContinue() {
		return ""
	}
	if len([]rune(strings.TrimSpace(a.ContentSnapshot))) > ChatDocumentArtifactInlineContinuationMaxChars {
		return ChatDocumentContinuationContextModeOutlineTail
	}
	return ChatDocumentContinuationContextModeInlineFull
}

func (a *ChatDocumentArtifact) CanInlineContinueWithFullSnapshot() bool {
	if a == nil || !a.CanContinue() {
		return false
	}
	return len([]rune(strings.TrimSpace(a.ContentSnapshot))) <= ChatDocumentArtifactInlineContinuationMaxChars
}
