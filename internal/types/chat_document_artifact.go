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

type ChatDocumentQualityIssueDetail struct {
	Code     string `json:"code"`
	Category string `json:"category,omitempty"`
	Severity string `json:"severity,omitempty"`
	Message  string `json:"message,omitempty"`
}

type ChatDocumentArtifact struct {
	ID                        string                           `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID                  uint64                           `json:"tenant_id" gorm:"index;not null"`
	SessionID                 string                           `json:"session_id" gorm:"type:varchar(36);index;not null"`
	SourceMessageID           string                           `json:"source_message_id" gorm:"type:varchar(36);uniqueIndex;not null"`
	SourceRequestID           string                           `json:"source_request_id" gorm:"type:varchar(64);index"`
	ParentArtifactID          string                           `json:"parent_artifact_id,omitempty" gorm:"type:varchar(36);index"`
	RevisionNo                int                              `json:"revision_no" gorm:"not null;default:1"`
	Title                     string                           `json:"title" gorm:"type:varchar(255)"`
	ArtifactKind              string                           `json:"artifact_kind" gorm:"type:varchar(32);index;not null"`
	ContentType               string                           `json:"content_type" gorm:"type:varchar(64);not null"`
	ContentSnapshot           string                           `json:"content_snapshot" gorm:"type:text;not null"`
	ContentChecksum           string                           `json:"content_checksum" gorm:"type:varchar(64);index;not null"`
	Status                    string                           `json:"status" gorm:"type:varchar(32);index;not null"`
	CompletionStatus          string                           `json:"completion_status" gorm:"type:varchar(32);index"`
	DocumentGenerationStatus  string                           `json:"document_generation_status,omitempty" gorm:"type:varchar(32);index"`
	DocumentTaskKind          string                           `json:"document_task_kind,omitempty" gorm:"type:varchar(32);index"`
	SourceTitle               string                           `json:"source_title,omitempty" gorm:"type:varchar(255)"`
	TargetLanguage            string                           `json:"target_language,omitempty" gorm:"type:varchar(128)"`
	OutputFormat              string                           `json:"output_format,omitempty" gorm:"type:varchar(32)"`
	Operation                 string                           `json:"operation" gorm:"type:varchar(32);index"`
	CreatedBy                 string                           `json:"created_by" gorm:"type:varchar(36);index"`
	CreatedAt                 time.Time                        `json:"created_at"`
	UpdatedAt                 time.Time                        `json:"updated_at"`
	DeletedAt                 gorm.DeletedAt                   `json:"deleted_at" gorm:"index"`
	EvidenceRefs              []ChatDocumentEvidenceRef        `json:"evidence_refs,omitempty" gorm:"-"`
	EvidenceSummary           *ChatDocumentEvidenceSummary     `json:"evidence_summary,omitempty" gorm:"-"`
	SnapshotCharCount         int                              `json:"snapshot_char_count,omitempty" gorm:"-"`
	CanContinueDocument       bool                             `json:"can_continue" gorm:"-"`
	CanInlineContinue         bool                             `json:"can_inline_continue" gorm:"-"`
	CanAutoContinueDocument   bool                             `json:"can_auto_continue,omitempty" gorm:"-"`
	CanManualContinueDocument bool                             `json:"can_manual_continue,omitempty" gorm:"-"`
	CanManualReviseDocument   bool                             `json:"can_manual_revise,omitempty" gorm:"-"`
	CanUseAsBaseDocument      bool                             `json:"can_use_as_base,omitempty" gorm:"-"`
	CanViewDocument           bool                             `json:"can_view,omitempty" gorm:"-"`
	CanIndexDocument          bool                             `json:"can_index,omitempty" gorm:"-"`
	ContinuationContextMode   string                           `json:"continuation_context_mode,omitempty" gorm:"-"`
	QualityIssues             []string                         `json:"quality_issues,omitempty" gorm:"-"`
	QualityIssueDetails       []ChatDocumentQualityIssueDetail `json:"quality_issue_details,omitempty" gorm:"-"`
	UserHint                  string                           `json:"user_hint,omitempty" gorm:"-"`
	StructureInfo             *ChatDocumentStructureInfo       `json:"structure_info,omitempty" gorm:"-"`
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
	return a.CanManualContinue()
}

func (a *ChatDocumentArtifact) CanView() bool {
	if a == nil {
		return false
	}
	if a.ArtifactKind != ChatDocumentArtifactKindMarkdown && a.ArtifactKind != ChatDocumentArtifactKindText {
		return false
	}
	if strings.TrimSpace(a.ContentSnapshot) == "" {
		return false
	}
	switch a.Status {
	case ChatDocumentArtifactStatusAvailable, ChatDocumentArtifactStatusPartial:
		return true
	default:
		return false
	}
}

func (a *ChatDocumentArtifact) CanAutoContinue() bool {
	if a == nil || !a.CanView() {
		return false
	}
	status := NormalizeOptionalChatDocumentGenerationStatus(a.DocumentGenerationStatus)
	if status == ChatDocumentGenerationStatusBlocked || status == ChatDocumentGenerationStatusNeedsReview {
		return false
	}
	return len([]rune(strings.TrimSpace(a.ContentSnapshot))) < ChatDocumentArtifactContinuationMaxChars
}

func (a *ChatDocumentArtifact) CanManualContinue() bool {
	return a.CanAutoContinue()
}

func (a *ChatDocumentArtifact) CanManualRevise() bool {
	if a == nil || !a.CanView() {
		return false
	}
	status := NormalizeOptionalChatDocumentGenerationStatus(a.DocumentGenerationStatus)
	return status != ChatDocumentGenerationStatusBlocked
}

func (a *ChatDocumentArtifact) CanUseAsBase() bool {
	return a.CanManualContinue() || a.CanManualRevise()
}

func (a *ChatDocumentArtifact) CanIndex() bool {
	if a == nil || !a.CanView() || a.Status != ChatDocumentArtifactStatusAvailable {
		return false
	}
	if NormalizeOptionalChatDocumentGenerationStatus(a.DocumentGenerationStatus) != ChatDocumentGenerationStatusCompleted {
		return false
	}
	for _, issue := range a.QualityIssues {
		if ChatDocumentQualityIssueSeverity(issue) == "error" {
			return false
		}
	}
	return true
}

func (a *ChatDocumentArtifact) CanUseAsBaseForIntent(intent string) bool {
	switch strings.TrimSpace(intent) {
	case ChatDocumentIntentContinue:
		return a.CanManualContinue()
	case ChatDocumentIntentRevise:
		return a.CanManualRevise()
	default:
		return a.CanUseAsBase()
	}
}

func (a *ChatDocumentArtifact) ContinuationMode() string {
	if a == nil || !a.CanManualContinue() {
		return ""
	}
	if len([]rune(strings.TrimSpace(a.ContentSnapshot))) > ChatDocumentArtifactInlineContinuationMaxChars {
		return ChatDocumentContinuationContextModeOutlineTail
	}
	return ChatDocumentContinuationContextModeInlineFull
}

func (a *ChatDocumentArtifact) CanInlineContinueWithFullSnapshot() bool {
	if a == nil || !a.CanManualContinue() {
		return false
	}
	return len([]rune(strings.TrimSpace(a.ContentSnapshot))) <= ChatDocumentArtifactInlineContinuationMaxChars
}

func ChatDocumentQualityIssueCategory(issue string) string {
	switch strings.TrimSpace(issue) {
	case ChatDocumentQualityIssueInlineContextTooLarge, ChatDocumentQualityIssueSnapshotTruncated:
		return "context"
	case ChatDocumentQualityIssueDeltaMergeUncertain, ChatDocumentQualityIssueTargetSectionUncertain, ChatDocumentQualityIssueRevisionTooShort, ChatDocumentQualityIssueRevisionMissingHeading, ChatDocumentQualityIssueRevisionPreambleTrimmed, ChatDocumentQualityIssueLowNoveltyDelta, ChatDocumentQualityIssueTerminalSectionTail:
		return "revision"
	case ChatDocumentQualityIssueDuplicateDocumentHead, ChatDocumentQualityIssueSectionNumberReset, ChatDocumentQualityIssueMarkdownHeadingNormalized, ChatDocumentQualityIssueMarkdownStructureInvalid, ChatDocumentQualityIssueMarkdownUnplannedSubsection, ChatDocumentQualityIssueMarkdownTooShort, ChatDocumentQualityIssueUnclosedCodeFence:
		return "markdown"
	case ChatDocumentQualityIssueInternalPromptLeakage:
		return "safety"
	default:
		return "document"
	}
}

func ChatDocumentQualityIssueSeverity(issue string) string {
	switch strings.TrimSpace(issue) {
	case ChatDocumentQualityIssueMarkdownHeadingNormalized, ChatDocumentQualityIssueRevisionPreambleTrimmed, ChatDocumentQualityIssueInlineContextTooLarge, ChatDocumentQualityIssueSnapshotTruncated:
		return "info"
	case ChatDocumentQualityIssueInternalPromptLeakage, ChatDocumentQualityIssueMarkdownStructureInvalid:
		return "error"
	case ChatDocumentQualityIssueUnclosedCodeFence, ChatDocumentQualityIssueDeltaMergeUncertain, ChatDocumentQualityIssueTargetSectionUncertain, ChatDocumentQualityIssueRevisionTooShort, ChatDocumentQualityIssueRevisionMissingHeading, ChatDocumentQualityIssueDuplicateDocumentHead, ChatDocumentQualityIssueSectionNumberReset, ChatDocumentQualityIssueLowNoveltyDelta, ChatDocumentQualityIssueTerminalSectionTail, ChatDocumentQualityIssueMarkdownUnplannedSubsection, ChatDocumentQualityIssueMarkdownTooShort:
		return "warning"
	default:
		return "warning"
	}
}

func ChatDocumentQualityIssueMessage(issue string) string {
	switch strings.TrimSpace(issue) {
	case ChatDocumentQualityIssueInlineContextTooLarge:
		return "文档过长，当前版本采用了截断上下文策略。"
	case ChatDocumentQualityIssueSnapshotTruncated:
		return "文档快照已被截断，后续操作将基于缩略上下文。"
	case ChatDocumentQualityIssueUnclosedCodeFence:
		return "检测到代码块未闭合，系统已尝试自动修复。"
	case ChatDocumentQualityIssueDeltaMergeUncertain:
		return "修订增量未能稳定定位到目标位置，系统采用了保守合并。"
	case ChatDocumentQualityIssueTargetSectionUncertain:
		return "目标章节定位存在歧义，请人工确认修订位置。"
	case ChatDocumentQualityIssueRevisionTooShort:
		return "修订结果明显短于基线文档，请检查是否丢失正文。"
	case ChatDocumentQualityIssueRevisionMissingHeading:
		return "修订结果缺少必要标题结构。"
	case ChatDocumentQualityIssueRevisionPreambleTrimmed:
		return "系统已移除 patch 前导说明，保留结构化修订内容。"
	case ChatDocumentQualityIssueDuplicateDocumentHead:
		return "文档标题出现重复。"
	case ChatDocumentQualityIssueSectionNumberReset:
		return "章节编号存在回退或跳号。"
	case ChatDocumentQualityIssueLowNoveltyDelta:
		return "本轮修订新增内容过少，请确认是否满足修改目标。"
	case ChatDocumentQualityIssueTerminalSectionTail:
		return "章节尾部存在异常残留，请人工复核。"
	case ChatDocumentQualityIssueMarkdownHeadingNormalized:
		return "部分标题格式已自动规范化。"
	case ChatDocumentQualityIssueMarkdownStructureInvalid:
		return "Markdown 结构未通过校验。"
	case ChatDocumentQualityIssueInternalPromptLeakage:
		return "正文中混入了内部提示或上下文标签。"
	case ChatDocumentQualityIssueMarkdownUnplannedSubsection:
		return "出现了未规划的小节标题。"
	case ChatDocumentQualityIssueMarkdownTooShort:
		return "部分章节内容明显偏短。"
	default:
		return "文档存在待处理的质量告警。"
	}
}

func ChatDocumentQualityIssueDetails(issues []string) []ChatDocumentQualityIssueDetail {
	if len(issues) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(issues))
	details := make([]ChatDocumentQualityIssueDetail, 0, len(issues))
	for _, issue := range issues {
		code := strings.TrimSpace(issue)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		details = append(details, ChatDocumentQualityIssueDetail{
			Code:     code,
			Category: ChatDocumentQualityIssueCategory(code),
			Severity: ChatDocumentQualityIssueSeverity(code),
			Message:  ChatDocumentQualityIssueMessage(code),
		})
	}
	if len(details) == 0 {
		return nil
	}
	return details
}
