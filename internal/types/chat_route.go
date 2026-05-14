package types

type ChatRouteKind string

const (
	ChatRouteNormalQA                  ChatRouteKind = "normal_qa"
	ChatRouteAgentQA                   ChatRouteKind = "agent_qa"
	ChatRouteShortDocument             ChatRouteKind = "short_document"
	ChatRouteFullDocument              ChatRouteKind = "full_document"
	ChatRouteKnowledgeGroundedFullDoc  ChatRouteKind = "knowledge_grounded_full_document"
	ChatRouteDocumentEdit              ChatRouteKind = "document_edit"
	ChatRouteKnowledgeGroundedContinue ChatRouteKind = "knowledge_grounded_document_continuation"
)

type ChatArtifactRouteSummary struct {
	ID                       string `json:"id,omitempty"`
	Title                    string `json:"title,omitempty"`
	Operation                string `json:"operation,omitempty"`
	DocumentGenerationStatus string `json:"document_generation_status,omitempty"`
}

type ChatRouteInput struct {
	Query                    string                    `json:"query"`
	Channel                  string                    `json:"channel,omitempty"`
	EndpointMode             string                    `json:"endpoint_mode,omitempty"`
	ModelID                  string                    `json:"model_id,omitempty"`
	AgentConfigured          bool                      `json:"agent_configured"`
	AgentModeEnabledByConfig bool                      `json:"agent_mode_enabled_by_config"`
	HasSelectedKnowledge     bool                      `json:"has_selected_knowledge"`
	HasEffectiveAgentKB      bool                      `json:"has_effective_agent_kb"`
	WebSearchEnabled         bool                      `json:"web_search_enabled"`
	HasAttachments           bool                      `json:"has_attachments"`
	HasImages                bool                      `json:"has_images"`
	AutoContinue             bool                      `json:"auto_continue"`
	ExplicitBaseArtifactID   string                    `json:"explicit_base_artifact_id,omitempty"`
	LatestArtifactSummary    *ChatArtifactRouteSummary `json:"latest_artifact_summary,omitempty"`
	UserExplicitOutputMode   string                    `json:"user_explicit_output_mode,omitempty"`
	UserRequestedRoute       string                    `json:"user_requested_route,omitempty"`
}

type ChatRouteDecision struct {
	Kind            ChatRouteKind `json:"kind"`
	Intent          string        `json:"intent,omitempty"`
	Operation       string        `json:"operation,omitempty"`
	OutputMode      string        `json:"output_mode,omitempty"`
	UseAgent        bool          `json:"use_agent"`
	UseKnowledge    bool          `json:"use_knowledge"`
	UseLongDocument bool          `json:"use_long_document"`
	NeedArtifact    bool          `json:"need_artifact"`
	TargetHeading   string        `json:"target_heading,omitempty"`
	MergeMode       string        `json:"merge_mode,omitempty"`
	Confidence      float64       `json:"confidence"`
	Reason          string        `json:"reason,omitempty"`
}
