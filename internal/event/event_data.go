package event

import "github.com/Tencent/WeKnora/internal/types"

// EventData contains common event data structures for different stages

// QueryData represents query-related event data
type QueryData struct {
	OriginalQuery  string                 `json:"original_query"`
	RewrittenQuery string                 `json:"rewritten_query,omitempty"`
	SessionID      string                 `json:"session_id"`
	UserID         string                 `json:"user_id,omitempty"`
	Extra          map[string]interface{} `json:"extra,omitempty"`
}

// RetrievalData represents retrieval event data
type RetrievalData struct {
	Query           string                 `json:"query"`
	KnowledgeBaseID string                 `json:"knowledge_base_id"`
	TopK            int                    `json:"top_k"`
	Threshold       float64                `json:"threshold"`
	RetrievalType   string                 `json:"retrieval_type"` // vector, keyword, entity
	ResultCount     int                    `json:"result_count"`
	Results         interface{}            `json:"results,omitempty"`
	Duration        int64                  `json:"duration_ms,omitempty"` // 检索耗时（毫秒）
	Extra           map[string]interface{} `json:"extra,omitempty"`
}

// RerankData represents reranking event data
type RerankData struct {
	Query       string                 `json:"query"`
	InputCount  int                    `json:"input_count"`  // 输入的候选数量
	OutputCount int                    `json:"output_count"` // 输出的结果数量
	ModelID     string                 `json:"model_id"`
	Threshold   float64                `json:"threshold"`
	Results     interface{}            `json:"results,omitempty"`
	Duration    int64                  `json:"duration_ms,omitempty"` // 排序耗时（毫秒）
	Extra       map[string]interface{} `json:"extra,omitempty"`
}

// MergeData represents merge event data
type MergeData struct {
	InputCount  int                    `json:"input_count"`
	OutputCount int                    `json:"output_count"`
	MergeType   string                 `json:"merge_type"` // dedup, fusion, etc.
	Results     interface{}            `json:"results,omitempty"`
	Duration    int64                  `json:"duration_ms,omitempty"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
}

// ChatData represents chat completion event data
type ChatData struct {
	Query       string                 `json:"query"`
	ModelID     string                 `json:"model_id"`
	Response    string                 `json:"response,omitempty"`
	StreamChunk string                 `json:"stream_chunk,omitempty"`
	TokenCount  int                    `json:"token_count,omitempty"`
	Duration    int64                  `json:"duration_ms,omitempty"`
	IsStream    bool                   `json:"is_stream"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
}

// ErrorData represents error event data
type ErrorData struct {
	Error     string                 `json:"error"`
	ErrorCode string                 `json:"error_code,omitempty"`
	Stage     string                 `json:"stage"` // 错误发生的阶段
	SessionID string                 `json:"session_id"`
	Query     string                 `json:"query,omitempty"`
	Extra     map[string]interface{} `json:"extra,omitempty"`
}

// NewEvent creates a new Event with metadata
func NewEvent(eventType EventType, data interface{}) Event {
	return Event{
		Type:     eventType,
		Data:     data,
		Metadata: make(map[string]interface{}),
	}
}

// WithSessionID sets the session ID for the event
func (e Event) WithSessionID(sessionID string) Event {
	e.SessionID = sessionID
	return e
}

// WithRequestID sets the request ID for the event
func (e Event) WithRequestID(requestID string) Event {
	e.RequestID = requestID
	return e
}

// WithMetadata adds metadata to the event
func (e Event) WithMetadata(key string, value interface{}) Event {
	if e.Metadata == nil {
		e.Metadata = make(map[string]interface{})
	}
	e.Metadata[key] = value
	return e
}

// AgentPlanData represents agent planning event data
type AgentPlanData struct {
	Query    string   `json:"query"`
	Plan     []string `json:"plan"` // Step descriptions
	Duration int64    `json:"duration_ms,omitempty"`
}

// AgentStepData represents agent step event data
type AgentStepData struct {
	Iteration int         `json:"iteration"`
	Thought   string      `json:"thought"`
	ToolCalls interface{} `json:"tool_calls"` // []types.ToolCall
	Duration  int64       `json:"duration_ms"`
}

// AgentActionData represents agent tool execution event data
type AgentActionData struct {
	Iteration  int                    `json:"iteration"`
	ToolName   string                 `json:"tool_name"`
	ToolInput  map[string]interface{} `json:"tool_input"`
	ToolOutput string                 `json:"tool_output"`
	Success    bool                   `json:"success"`
	Error      string                 `json:"error,omitempty"`
	Duration   int64                  `json:"duration_ms"`
}

// AgentQueryData represents agent query event data
type AgentQueryData struct {
	SessionID string                 `json:"session_id"`
	Query     string                 `json:"query"`
	RequestID string                 `json:"request_id,omitempty"`
	Extra     map[string]interface{} `json:"extra,omitempty"`
}

// AgentCompleteData represents agent completion event data
type AgentCompleteData struct {
	SessionID                 string                 `json:"session_id"`
	TotalSteps                int                    `json:"total_steps"`
	FinalAnswer               string                 `json:"final_answer"`
	CompletionStatus          string                 `json:"completion_status,omitempty"`
	FinishReason              string                 `json:"finish_reason,omitempty"`
	IsPartial                 bool                   `json:"is_partial,omitempty"`
	AllowIndexing             bool                   `json:"allow_indexing,omitempty"`
	AllowComplete             bool                   `json:"allow_complete,omitempty"`
	FailureReason             string                 `json:"failure_reason,omitempty"`
	KnowledgeRefs             []interface{}          `json:"knowledge_refs,omitempty"` // []*types.SearchResult
	AgentSteps                types.AgentSteps       `json:"agent_steps,omitempty"`    // Detailed agent execution steps for persistence/replay
	TotalDurationMs           int64                  `json:"total_duration_ms"`
	MessageID                 string                 `json:"message_id,omitempty"` // Assistant message ID
	FinalDocumentMode         string                 `json:"final_document_mode,omitempty"`
	FinalDocument             string                 `json:"final_document,omitempty"`
	FinalDocumentArtifactID   string                 `json:"final_document_artifact_id,omitempty"`
	DocumentGenerationStatus  string                 `json:"document_generation_status,omitempty"`
	AutoContinueNext          *bool                  `json:"auto_continue_next,omitempty"`
	AutoContinueReason        string                 `json:"auto_continue_reason,omitempty"`
	AutoContinueReasonMessage string                 `json:"auto_continue_reason_message,omitempty"`
	NextAction                string                 `json:"next_action,omitempty"`
	NextReason                string                 `json:"next_reason,omitempty"`
	NextReasonMessage         string                 `json:"next_reason_message,omitempty"`
	CanAutoContinue           *bool                  `json:"can_auto_continue,omitempty"`
	RecommendedRequest        map[string]interface{} `json:"recommended_request,omitempty"`
	RequestID                 string                 `json:"request_id,omitempty"`
	Extra                     map[string]interface{} `json:"extra,omitempty"`
}

// === Streaming Event Data Structures ===
// These are used for real-time streaming feedback to clients

// AgentThoughtData represents agent thought streaming data
type AgentThoughtData struct {
	Content        string                 `json:"content"`
	Iteration      int                    `json:"iteration"`
	Done           bool                   `json:"done"`
	Replace        bool                   `json:"replace,omitempty"`
	Synthetic      bool                   `json:"synthetic,omitempty"`
	Stage          string                 `json:"stage,omitempty"`
	Outline        map[string]interface{} `json:"outline,omitempty"`
	SectionCurrent int                    `json:"section_current,omitempty"`
	SectionTotal   int                    `json:"section_total,omitempty"`
	SectionTitle   string                 `json:"section_title,omitempty"`
	QueryCurrent   int                    `json:"query_current,omitempty"`
	QueryTotal     int                    `json:"query_total,omitempty"`
	ProgressLabel  string                 `json:"progress_label,omitempty"`
}

// AgentToolCallData represents agent tool call notification data
type AgentToolCallData struct {
	ToolCallID string         `json:"tool_call_id"` // Tool call ID for tracking
	ToolName   string         `json:"tool_name"`
	Arguments  map[string]any `json:"arguments,omitempty"`
	Iteration  int            `json:"iteration"`
	Hint       string         `json:"hint,omitempty"` // Human-readable tool hint, e.g. `web_search("query")`
}

// AgentToolResultData represents agent tool execution result data
type AgentToolResultData struct {
	ToolCallID string                 `json:"tool_call_id"` // Tool call ID for tracking
	ToolName   string                 `json:"tool_name"`
	Output     string                 `json:"output"`
	Error      string                 `json:"error,omitempty"`
	Success    bool                   `json:"success"`
	Duration   int64                  `json:"duration_ms,omitempty"`
	Iteration  int                    `json:"iteration"`
	Data       map[string]interface{} `json:"data,omitempty"` // Structured data from tool result (e.g., display_type, formatted results)
}

// AgentReferencesData represents knowledge references data
type AgentReferencesData struct {
	References interface{} `json:"references"` // []*types.SearchResult
	Iteration  int         `json:"iteration"`
}

// AgentFinalAnswerData represents final answer streaming data
type AgentFinalAnswerData struct {
	Content                  string                 `json:"content"`
	Done                     bool                   `json:"done"`
	IsFallback               bool                   `json:"is_fallback,omitempty"` // True when response is a fallback (no knowledge base match)
	CompletionStatus         string                 `json:"completion_status,omitempty"`
	FinishReason             string                 `json:"finish_reason,omitempty"`
	IsPartial                bool                   `json:"is_partial,omitempty"`
	AllowIndexing            bool                   `json:"allow_indexing,omitempty"`
	AllowComplete            bool                   `json:"allow_complete,omitempty"`
	FailureReason            string                 `json:"failure_reason,omitempty"`
	DocumentGenerationStatus string                 `json:"document_generation_status,omitempty"`
	Extra                    map[string]interface{} `json:"extra,omitempty"`
}

// AgentReflectionData represents agent reflection data
type AgentReflectionData struct {
	ToolCallID string `json:"tool_call_id"` // Tool call ID for tracking
	Content    string `json:"content"`
	Iteration  int    `json:"iteration"`
	Done       bool   `json:"done"` // Whether streaming is complete
}

// SessionTitleData represents session title update data
type SessionTitleData struct {
	SessionID string `json:"session_id"`
	Title     string `json:"title"`
}

// StopData represents stop generation request data
type StopData struct {
	SessionID string `json:"session_id"`
	MessageID string `json:"message_id"`
	Reason    string `json:"reason,omitempty"` // Optional reason for stopping
}

// ToolApprovalRequiredData is emitted when an MCP tool marked dangerous is about to run.
type ToolApprovalRequiredData struct {
	PendingID          string      `json:"pending_id"`
	TenantID           uint64      `json:"tenant_id"`
	SessionID          string      `json:"session_id"`
	AssistantMessageID string      `json:"assistant_message_id"`
	ServiceID          string      `json:"service_id"`
	ServiceName        string      `json:"service_name"`
	MCPToolName        string      `json:"mcp_tool_name"`
	RegisteredToolName string      `json:"registered_tool_name"`
	Description        string      `json:"description"`
	Args               interface{} `json:"args,omitempty"`
	ArgsJSON           string      `json:"args_json,omitempty"`
	TimeoutSeconds     int         `json:"timeout_seconds"`
	RequestedAtUnix    int64       `json:"requested_at"`
	ToolCallID         string      `json:"tool_call_id"`
	RequestID          string      `json:"request_id,omitempty"`
}

// ToolApprovalResolvedData confirms the user decision (or timeout/cancel).
type ToolApprovalResolvedData struct {
	PendingID string `json:"pending_id"`
	Approved  bool   `json:"approved"`
	Reason    string `json:"reason,omitempty"`
	TimedOut  bool   `json:"timed_out,omitempty"`
	Canceled  bool   `json:"canceled,omitempty"`
}

// MCPOAuthRequiredData is emitted when an OAuth-enabled MCP service is invoked
// during a conversation but the current user has not authorized it yet. The
// UI surfaces an "Authorize" card; the agent pauses until the user authorizes.
type MCPOAuthRequiredData struct {
	PendingID          string `json:"pending_id"`
	TenantID           uint64 `json:"tenant_id"`
	SessionID          string `json:"session_id"`
	AssistantMessageID string `json:"assistant_message_id"`
	ServiceID          string `json:"service_id"`
	ServiceName        string `json:"service_name"`
	MCPToolName        string `json:"mcp_tool_name"`
	TimeoutSeconds     int    `json:"timeout_seconds"`
	RequestedAtUnix    int64  `json:"requested_at"`
	ToolCallID         string `json:"tool_call_id"`
	RequestID          string `json:"request_id,omitempty"`
}

// MCPOAuthResolvedData confirms the outcome of an in-conversation OAuth prompt
// (authorized / timeout / cancel).
type MCPOAuthResolvedData struct {
	PendingID  string `json:"pending_id"`
	ServiceID  string `json:"service_id"`
	Authorized bool   `json:"authorized"`
	Reason     string `json:"reason,omitempty"`
	TimedOut   bool   `json:"timed_out,omitempty"`
	Canceled   bool   `json:"canceled,omitempty"`
}
