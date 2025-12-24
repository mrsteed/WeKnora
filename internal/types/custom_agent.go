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
	// Agent mode: "normal" for RAG mode, "agent" for ReAct agent mode
	AgentMode string `yaml:"agent_mode" json:"agent_mode"`
	// System prompt for the agent
	SystemPrompt string `yaml:"system_prompt" json:"system_prompt"`
	// Model ID to use for conversations
	ModelID string `yaml:"model_id" json:"model_id"`
	// Temperature for LLM (0-1)
	Temperature float64 `yaml:"temperature" json:"temperature"`
	// Maximum iterations for ReAct loop (only for agent type)
	MaxIterations int `yaml:"max_iterations" json:"max_iterations"`
	// Allowed tools (only for agent type)
	AllowedTools []string `yaml:"allowed_tools" json:"allowed_tools"`
	// Associated knowledge base IDs
	KnowledgeBases []string `yaml:"knowledge_bases" json:"knowledge_bases"`
	// Whether to allow user to select knowledge bases when no KnowledgeBases are configured
	// If true (default), user can freely select any knowledge base
	// If false, knowledge base selection is disabled for this agent
	AllowUserKBSelection *bool `yaml:"allow_user_kb_selection" json:"allow_user_kb_selection"`
	// Whether web search is enabled
	WebSearchEnabled bool `yaml:"web_search_enabled" json:"web_search_enabled"`
	// Maximum web search results
	WebSearchMaxResults int `yaml:"web_search_max_results" json:"web_search_max_results"`
	// Whether reflection is enabled (only for agent type)
	ReflectionEnabled bool `yaml:"reflection_enabled" json:"reflection_enabled"`
	// Welcome message displayed when agent is selected
	MultiTurnEnabled bool `yaml:"multi_turn_enabled" json:"multi_turn_enabled"`
	// Number of history turns to keep in context
	HistoryTurns int `yaml:"history_turns" json:"history_turns"`
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
			AgentMode:        "normal",
			SystemPrompt:     "",
			Temperature:      0.7,
			WebSearchEnabled: false,
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
			AllowedTools:        []string{"knowledge_search", "web_search"},
			WebSearchEnabled:    true,
			WebSearchMaxResults: 5,
			ReflectionEnabled:   false,
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
