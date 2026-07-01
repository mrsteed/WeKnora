package types

const (
	ChatDocumentTaskKindWriting     = "writing"
	ChatDocumentTaskKindTranslation = "translation"
)

type ChatDocumentTranslationOptions struct {
	SourceLanguage    string // Optional source language hint; empty means auto detect
	TargetLanguage    string // Target language for translation output
	PreserveStructure bool   // Whether the translated output should preserve the original structure
	OutputFormat      string // Expected output format, e.g. markdown
}

// QARequest consolidates all parameters for KnowledgeQA and AgentQA service calls,
// replacing the previous 14-parameter method signatures.
// EventBus is passed separately to avoid circular dependency with the event package.
type QARequest struct {
	Session                   *Session     // The conversation session
	Query                     string       // User query text
	AssistantMessageID        string       // Pre-created assistant message ID
	SummaryModelID            string       // Optional model override; empty = use agent/KB default
	CustomAgent               *CustomAgent // Optional custom agent for config override
	KnowledgeBaseIDs          []string     // Knowledge base IDs to search (from request + @mentions)
	KnowledgeIDs              []string     // Specific knowledge (file) IDs to search
	TagScopes                 []TagScope   // Tag-constrained KB scopes from @mentions
	MCPServiceIDs             []string     // Per-request MCP service IDs from @mentions
	SkillNames                []string     // Per-request preloaded skill names from @mentions
	ImageURLs                 []string     // Image URLs for multimodal input
	ImageDescription          string       // VLM-generated image description (fallback for non-vision models)
	UserMessageID             string       // Created user message ID
	WebSearchEnabled          bool         // Whether web search is enabled for this request
	EnableMemory              bool         // Whether memory feature is enabled
	QuotedContext             string       // Quoted message content from IM quote-reply (appended at LLM prompt stage, not used for retrieval)
	DocumentIntent            string       // Normal / continue / revise / regenerate document intent for the current round
	BaseArtifactID            string       // Resolved base artifact ID for document continuation or revision
	DocumentOperation         string       // Artifact operation associated with the current round
	DocumentOutputMode        string       // Document output mode for the current round: full_document or delta_only
	DocumentTaskKind          string       // Structured task kind for long document requests, e.g. writing or translation
	TranslationOptions        *ChatDocumentTranslationOptions
	DocumentTargetHeading     string                // Structured target heading for section-scoped revision/continuation
	DocumentMergeMode         string                // Structured merge mode for section-scoped revision/continuation
	RouteDecision             *ChatRouteDecision    // Shadow route decision from ChatRouteService; non-authoritative before routing rollout
	AutoContinue              bool                  // Whether this round was triggered by automatic document continuation
	GenerationRunID           string                // Persistent generation run ID for long document continuation
	AutoContinueRound         int                   // Current automatic continuation round, when applicable
	AutoContinuePrompt        string                // Prompt used by automatic continuation
	AutoContinueOriginalQuery string                // Original user goal for automatic continuation
	BaseArtifact              *ChatDocumentArtifact // Loaded base artifact for continuation/revision runtime decisions
	Attachments               MessageAttachments    // File attachments (processed and ready for prompt injection)
}
