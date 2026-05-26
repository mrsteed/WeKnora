package types

import (
	"time"

	"gorm.io/gorm"
)

const (
	// AgentPageShareAccessScopeAnonymous means the share can be opened without login.
	AgentPageShareAccessScopeAnonymous = "anonymous"
)

const (
	// AgentPageShareStatusActive means the share is publicly accessible.
	AgentPageShareStatusActive = "active"
	// AgentPageShareStatusDisabled means the share was manually closed by the owner.
	AgentPageShareStatusDisabled = "disabled"
	// AgentPageShareStatusExpired means the share is no longer accessible because it has expired.
	AgentPageShareStatusExpired = "expired"
)

// AgentPageShare stores one public chat-share entry for a custom agent.
type AgentPageShare struct {
	ID                    string         `json:"id" gorm:"type:varchar(36);primaryKey"`
	AgentID               string         `json:"agent_id" gorm:"type:varchar(36);not null;index"`
	SourceTenantID        uint64         `json:"source_tenant_id" gorm:"not null;index"`
	ShareCode             string         `json:"share_code" gorm:"type:varchar(64);not null;uniqueIndex"`
	AccessScope           string         `json:"access_scope" gorm:"type:varchar(16);not null;default:'anonymous'"`
	Status                string         `json:"status" gorm:"type:varchar(16);not null;default:'active'"`
	CreatedBy             string         `json:"created_by" gorm:"type:varchar(36);not null"`
	AnonymousSessionLimit int            `json:"anonymous_session_limit" gorm:"not null;default:0"`
	RateLimitPerMinute    int            `json:"rate_limit_per_minute" gorm:"not null;default:0"`
	LastAccessedAt        *time.Time     `json:"last_accessed_at,omitempty"`
	ExpiresAt             *time.Time     `json:"expires_at,omitempty"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DeletedAt             gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	Agent *CustomAgent `json:"agent,omitempty" gorm:"foreignKey:AgentID,SourceTenantID;references:ID,TenantID"`
}

// TableName returns the table name for GORM.
func (AgentPageShare) TableName() string {
	return "agent_page_shares"
}

// AgentPageShareRuntimeSummary is the minimal readonly runtime info exposed to a shared chat page.
type AgentPageShareRuntimeSummary struct {
	AgentMode               string                             `json:"agent_mode"`
	KBSelectionMode         string                             `json:"kb_selection_mode,omitempty"`
	MCPSelectionMode        string                             `json:"mcp_selection_mode,omitempty"`
	WebSearchEnabled        bool                               `json:"web_search_enabled"`
	MultiTurnEnabled        bool                               `json:"multi_turn_enabled"`
	ImageUploadEnabled      bool                               `json:"image_upload_enabled"`
	AudioUploadEnabled      bool                               `json:"audio_upload_enabled"`
	AttachmentUploadEnabled bool                               `json:"attachment_upload_enabled"`
	SupportedFileTypes      []string                           `json:"supported_file_types,omitempty"`
	DefaultModelID          string                             `json:"default_model_id,omitempty"`
	DefaultModelName        string                             `json:"default_model_name,omitempty"`
	AvailableModels         []AgentPageSharePublicModelSummary `json:"available_models,omitempty"`
	ShowWebSearchToggle     bool                               `json:"show_web_search_toggle"`
	ShowModelSelector       bool                               `json:"show_model_selector"`
	ShowKBSelector          bool                               `json:"show_kb_selector"`
	ShowAgentSelector       bool                               `json:"show_agent_selector"`
}

// AgentPageSharePublicModelParameters is the minimal readonly model parameter payload exposed to a share page.
type AgentPageSharePublicModelParameters struct {
	ParameterSize string `json:"parameter_size,omitempty"`
}

// AgentPageSharePublicModelSummary is the minimal readonly model metadata exposed to anonymous visitors.
type AgentPageSharePublicModelSummary struct {
	ID          string                              `json:"id"`
	Name        string                              `json:"name"`
	Type        ModelType                           `json:"type"`
	Source      ModelSource                         `json:"source"`
	Description string                              `json:"description,omitempty"`
	Parameters  AgentPageSharePublicModelParameters `json:"parameters,omitempty"`
	IsDefault   bool                                `json:"is_default"`
	Status      ModelStatus                         `json:"status,omitempty"`
}

// AgentPageShareAgentSummary is the minimal public profile of the shared agent.
type AgentPageShareAgentSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Avatar      string `json:"avatar"`
}

// AgentPageSharePublicSummary is the minimal public share metadata exposed to anonymous visitors.
type AgentPageSharePublicSummary struct {
	ID          string `json:"id"`
	ShareCode   string `json:"share_code"`
	Status      string `json:"status"`
	AccessScope string `json:"access_scope"`
}

// AgentPageShareManagementView is the management-side share payload with the computed share URL.
type AgentPageShareManagementView struct {
	ID                    string     `json:"id"`
	AgentID               string     `json:"agent_id"`
	SourceTenantID        uint64     `json:"source_tenant_id"`
	ShareCode             string     `json:"share_code"`
	Status                string     `json:"status"`
	AccessScope           string     `json:"access_scope"`
	ShareURL              string     `json:"share_url"`
	AnonymousSessionLimit int        `json:"anonymous_session_limit"`
	RateLimitPerMinute    int        `json:"rate_limit_per_minute"`
	LastAccessedAt        *time.Time `json:"last_accessed_at,omitempty"`
	ExpiresAt             *time.Time `json:"expires_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// AgentPageSharePublicInfo is the public readonly payload returned by share_code lookup.
type AgentPageSharePublicInfo struct {
	Share              AgentPageSharePublicSummary  `json:"share"`
	Agent              AgentPageShareAgentSummary   `json:"agent"`
	Runtime            AgentPageShareRuntimeSummary `json:"runtime"`
	SuggestedQuestions []string                     `json:"suggested_questions,omitempty"`
}

// AgentPageShareSessionCreateResult is the anonymous session creation payload returned once to the browser.
type AgentPageShareSessionCreateResult struct {
	SessionID          string    `json:"session_id"`
	AnonymousVisitorID string    `json:"anonymous_visitor_id"`
	VisitorToken       string    `json:"visitor_token"`
	ExpiresAt          time.Time `json:"expires_at"`
}

// AgentPageShareSessionContext bundles the validated share, agent, and session runtime state.
type AgentPageShareSessionContext struct {
	Share   *AgentPageShare `json:"-"`
	Agent   *CustomAgent    `json:"-"`
	Session *Session        `json:"-"`
}
