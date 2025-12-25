package types

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// CustomAgentType represents the type of the custom agent
type CustomAgentType string

const (
	// CustomAgentTypeNormal represents the normal RAG-based chat mode
	CustomAgentTypeNormal CustomAgentType = "normal"
	// CustomAgentTypeAgent represents the ReAct agent mode with tool calling
	CustomAgentTypeAgent CustomAgentType = "agent"
	// CustomAgentTypeCustom represents user-defined custom agents
	CustomAgentTypeCustom CustomAgentType = "custom"
)

// BuiltinAgentID constants for built-in agents
const (
	BuiltinAgentNormalID = "builtin-normal"
	BuiltinAgentAgentID  = "builtin-agent"
)

// CustomAgent represents a configurable AI agent (similar to GPTs)
type CustomAgent struct {
	// Unique identifier of the agent
	ID string `yaml:"id" json:"id" gorm:"type:varchar(36);primaryKey"`
	// Name of the agent
	Name string `yaml:"name" json:"name" gorm:"type:varchar(255);not null"`
	// Description of the agent
	Description string `yaml:"description" json:"description" gorm:"type:text"`
	// Avatar/Icon of the agent (emoji or icon name)
	Avatar string `yaml:"avatar" json:"avatar" gorm:"type:varchar(64)"`
	// Whether this is a built-in agent (normal mode / agent mode)
	IsBuiltin bool `yaml:"is_builtin" json:"is_builtin" gorm:"default:false"`
	// Type of the agent: normal, agent, custom
	Type CustomAgentType `yaml:"type" json:"type" gorm:"type:varchar(32);default:'custom'"`
	// Tenant ID
	TenantID uint64 `yaml:"tenant_id" json:"tenant_id" gorm:"index"`
	// Created by user ID
	CreatedBy string `yaml:"created_by" json:"created_by" gorm:"type:varchar(36)"`

	// Agent configuration
	Config CustomAgentConfig `yaml:"config" json:"config" gorm:"type:json"`

	// Timestamps
	CreatedAt time.Time      `yaml:"created_at" json:"created_at"`
	UpdatedAt time.Time      `yaml:"updated_at" json:"updated_at"`
	DeletedAt gorm.DeletedAt `yaml:"deleted_at" json:"deleted_at" gorm:"index"`
}

// CustomAgentConfig represents the configuration of a custom agent
type CustomAgentConfig struct {
	// ===== Basic Settings =====
	// Agent mode: "normal" for RAG mode, "agent" for ReAct agent mode
	AgentMode string `yaml:"agent_mode" json:"agent_mode"`
	// System prompt for the agent (for normal mode, or agent mode when web search is disabled)
	SystemPrompt string `yaml:"system_prompt" json:"system_prompt"`
	// System prompt for agent mode when web search is enabled
	SystemPromptWebEnabled string `yaml:"system_prompt_web_enabled" json:"system_prompt_web_enabled"`
	// Context template for normal mode (how to format retrieved chunks)
	ContextTemplate string `yaml:"context_template" json:"context_template"`

	// ===== Model Settings =====
	// Model ID to use for conversations
	ModelID string `yaml:"model_id" json:"model_id"`
	// ReRank model ID for retrieval
	RerankModelID string `yaml:"rerank_model_id" json:"rerank_model_id"`
	// Temperature for LLM (0-1)
	Temperature float64 `yaml:"temperature" json:"temperature"`
	// Maximum completion tokens (only for normal mode)
	MaxCompletionTokens int `yaml:"max_completion_tokens" json:"max_completion_tokens"`

	// ===== Agent Mode Settings =====
	// Maximum iterations for ReAct loop (only for agent type)
	MaxIterations int `yaml:"max_iterations" json:"max_iterations"`
	// Allowed tools (only for agent type)
	AllowedTools []string `yaml:"allowed_tools" json:"allowed_tools"`
	// Whether reflection is enabled (only for agent type)
	ReflectionEnabled bool `yaml:"reflection_enabled" json:"reflection_enabled"`

	// ===== Knowledge Base Settings =====
	// Knowledge base selection mode: "all" = all KBs, "selected" = specific KBs, "none" = no KB
	KBSelectionMode string `yaml:"kb_selection_mode" json:"kb_selection_mode"`
	// Associated knowledge base IDs (only used when KBSelectionMode is "selected")
	KnowledgeBases []string `yaml:"knowledge_bases" json:"knowledge_bases"`
	// Whether to allow user to select knowledge bases during conversation
	// If true, user can select from available KBs (all or selected based on KBSelectionMode)
	// If false, the configured KBs are used automatically without user selection
	AllowUserKBSelection *bool `yaml:"allow_user_kb_selection" json:"allow_user_kb_selection"`

	// ===== Web Search Settings =====
	// Whether web search is enabled
	WebSearchEnabled bool `yaml:"web_search_enabled" json:"web_search_enabled"`
	// Maximum web search results
	WebSearchMaxResults int `yaml:"web_search_max_results" json:"web_search_max_results"`

	// ===== Multi-turn Conversation Settings =====
	// Whether multi-turn conversation is enabled
	MultiTurnEnabled bool `yaml:"multi_turn_enabled" json:"multi_turn_enabled"`
	// Number of history turns to keep in context
	HistoryTurns int `yaml:"history_turns" json:"history_turns"`

	// ===== Retrieval Strategy Settings (for both modes) =====
	// Embedding/Vector retrieval top K
	EmbeddingTopK int `yaml:"embedding_top_k" json:"embedding_top_k"`
	// Keyword retrieval threshold
	KeywordThreshold float64 `yaml:"keyword_threshold" json:"keyword_threshold"`
	// Vector retrieval threshold
	VectorThreshold float64 `yaml:"vector_threshold" json:"vector_threshold"`
	// Rerank top K
	RerankTopK int `yaml:"rerank_top_k" json:"rerank_top_k"`
	// Rerank threshold
	RerankThreshold float64 `yaml:"rerank_threshold" json:"rerank_threshold"`

	// ===== Advanced Settings (mainly for normal mode) =====
	// Whether to enable query expansion
	EnableQueryExpansion bool `yaml:"enable_query_expansion" json:"enable_query_expansion"`
	// Whether to enable query rewrite for multi-turn conversations
	EnableRewrite bool `yaml:"enable_rewrite" json:"enable_rewrite"`
	// Rewrite prompt system message
	RewritePromptSystem string `yaml:"rewrite_prompt_system" json:"rewrite_prompt_system"`
	// Rewrite prompt user message template
	RewritePromptUser string `yaml:"rewrite_prompt_user" json:"rewrite_prompt_user"`
	// Fallback strategy: "fixed" for fixed response, "model" for model generation
	FallbackStrategy string `yaml:"fallback_strategy" json:"fallback_strategy"`
	// Fixed fallback response (when FallbackStrategy is "fixed")
	FallbackResponse string `yaml:"fallback_response" json:"fallback_response"`
	// Fallback prompt (when FallbackStrategy is "model")
	FallbackPrompt string `yaml:"fallback_prompt" json:"fallback_prompt"`
}

// Value implements driver.Valuer interface for CustomAgentConfig
func (c CustomAgentConfig) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements sql.Scanner interface for CustomAgentConfig
func (c *CustomAgentConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, c)
}

// TableName returns the table name for CustomAgent
func (CustomAgent) TableName() string {
	return "custom_agents"
}

// EnsureDefaults sets default values for the agent
func (a *CustomAgent) EnsureDefaults() {
	if a == nil {
		return
	}
	if a.Type == "" {
		a.Type = CustomAgentTypeCustom
	}
	if a.Config.Temperature == 0 {
		a.Config.Temperature = 0.7
	}
	if a.Config.MaxIterations == 0 {
		a.Config.MaxIterations = 10
	}
	if a.Config.WebSearchMaxResults == 0 {
		a.Config.WebSearchMaxResults = 5
	}
	if a.Config.HistoryTurns == 0 {
		a.Config.HistoryTurns = 5
	}
	// Retrieval strategy defaults
	if a.Config.EmbeddingTopK == 0 {
		a.Config.EmbeddingTopK = 10
	}
	if a.Config.KeywordThreshold == 0 {
		a.Config.KeywordThreshold = 0.3
	}
	if a.Config.VectorThreshold == 0 {
		a.Config.VectorThreshold = 0.5
	}
	if a.Config.RerankTopK == 0 {
		a.Config.RerankTopK = 5
	}
	if a.Config.RerankThreshold == 0 {
		a.Config.RerankThreshold = 0.5
	}
	// Advanced settings defaults
	if a.Config.FallbackStrategy == "" {
		a.Config.FallbackStrategy = "model"
	}
	if a.Config.MaxCompletionTokens == 0 {
		a.Config.MaxCompletionTokens = 2048
	}
}

// IsAgentMode returns true if this agent uses ReAct agent mode
func (a *CustomAgent) IsAgentMode() bool {
	return a.Config.AgentMode == "agent"
}

// GetBuiltinNormalAgent returns the built-in normal mode agent
func GetBuiltinNormalAgent(tenantID uint64) *CustomAgent {
	return &CustomAgent{
		ID:          BuiltinAgentNormalID,
		Name:        "普通模式",
		Description: "基于知识库的 RAG 问答，快速准确地回答问题",
		Avatar:      "💬",
		IsBuiltin:   true,
		Type:        CustomAgentTypeNormal,
		TenantID:    tenantID,
		Config: CustomAgentConfig{
			AgentMode:            "normal",
			SystemPrompt:         "",
			Temperature:          0.7,
			MaxCompletionTokens:  2048,
			WebSearchEnabled:     false,
			MultiTurnEnabled:     true,
			HistoryTurns:         5,
			// Retrieval strategy
			EmbeddingTopK:        10,
			KeywordThreshold:     0.3,
			VectorThreshold:      0.5,
			RerankTopK:           5,
			RerankThreshold:      0.5,
			// Advanced settings
			EnableQueryExpansion: true,
			EnableRewrite:        true,
			FallbackStrategy:     "model",
		},
	}
}

// GetBuiltinAgentAgent returns the built-in agent mode agent
func GetBuiltinAgentAgent(tenantID uint64) *CustomAgent {
	return &CustomAgent{
		ID:          BuiltinAgentAgentID,
		Name:        "Agent 模式",
		Description: "ReAct 推理框架，支持多步思考和工具调用",
		Avatar:      "🤖",
		IsBuiltin:   true,
		Type:        CustomAgentTypeAgent,
		TenantID:    tenantID,
		Config: CustomAgentConfig{
			AgentMode:           "agent",
			SystemPrompt:        "",
			Temperature:         0.7,
			MaxIterations:       10,
			AllowedTools:        []string{"thinking", "todo_write", "knowledge_search", "grep_chunks", "list_knowledge_chunks", "query_knowledge_graph", "get_document_info"},
			WebSearchEnabled:    true,
			WebSearchMaxResults: 5,
			ReflectionEnabled:   false,
			MultiTurnEnabled:    true,
			HistoryTurns:        5,
			// Retrieval strategy
			EmbeddingTopK:       10,
			KeywordThreshold:    0.3,
			VectorThreshold:     0.5,
			RerankTopK:          5,
			RerankThreshold:     0.5,
		},
	}
}

// GetBuiltinAgents returns all built-in agents for a tenant
func GetBuiltinAgents(tenantID uint64) []*CustomAgent {
	return []*CustomAgent{
		GetBuiltinNormalAgent(tenantID),
		GetBuiltinAgentAgent(tenantID),
	}
}
