package chatpipline

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// PluginPreprocess Query preprocessing plugin
type PluginPreprocess struct {
	config       *config.Config
	modelService interfaces.ModelService
}

// Regular expressions for text cleaning
var (
	multiSpaceRegex = regexp.MustCompile(`\s+`) // Multiple spaces
)

// NewPluginPreprocess Creates a new query preprocessing plugin
func NewPluginPreprocess(
	eventManager *EventManager,
	config *config.Config,
	cleaner interfaces.ResourceCleaner,
	modelService interfaces.ModelService,
) *PluginPreprocess {
	res := &PluginPreprocess{
		config:       config,
		modelService: modelService,
	}

	eventManager.Register(res)
	return res
}

// ActivationEvents Register activation events
func (p *PluginPreprocess) ActivationEvents() []types.EventType {
	return []types.EventType{types.PREPROCESS_QUERY}
}

// OnEvent Process events
func (p *PluginPreprocess) OnEvent(
	ctx context.Context,
	eventType types.EventType,
	chatManage *types.ChatManage,
	next func() *PluginError,
) *PluginError {
	rawQuery := strings.TrimSpace(chatManage.RewriteQuery)
	if rawQuery == "" {
		return next()
	}

	pipelineInfo(ctx, "Preprocess", "input", map[string]interface{}{
		"session_id":    chatManage.SessionID,
		"rewrite_query": rawQuery,
	})

	// Lightweight normalization: just collapse multiple spaces
	processed := multiSpaceRegex.ReplaceAllString(rawQuery, " ")
	processed = strings.TrimSpace(processed)

	chatManage.ProcessedQuery = processed
	chatManage.QueryIntent = p.detectIntentLLM(ctx, chatManage, processed)

	pipelineInfo(ctx, "Preprocess", "output", map[string]interface{}{
		"session_id":      chatManage.SessionID,
		"processed_query": processed,
		"query_intent":    chatManage.QueryIntent,
	})

	return next()
}

// intentResp is a response for intent detection
type intentResp struct {
	Intent     string  `json:"intent"`
	Confidence float64 `json:"confidence"`
}

// detectIntentLLM detects the intent of a query using an LLM
func (p *PluginPreprocess) detectIntentLLM(ctx context.Context, chatManage *types.ChatManage, text string) string {
	if p.modelService == nil || chatManage.ChatModelID == "" {
		pipelineWarn(
			ctx,
			"IntentDetect",
			"skip",
			map[string]interface{}{"reason": "no_model", "session_id": chatManage.SessionID},
		)
		return "general"
	}
	chatModel, err := p.modelService.GetChatModel(ctx, chatManage.ChatModelID)
	if err != nil {
		pipelineWarn(
			ctx,
			"IntentDetect",
			"get_model_failed",
			map[string]interface{}{"error": err.Error(), "model_id": chatManage.ChatModelID},
		)
		return "general"
	}
	pipelineInfo(
		ctx,
		"IntentDetect",
		"start",
		map[string]interface{}{"session_id": chatManage.SessionID, "model_id": chatManage.ChatModelID},
	)
	sys := "You are a query intent classifier. Classify the user's query into one of: definition, howto, compare, qa, general. Respond ONLY with a JSON object {\"intent\": \"...\", \"confidence\": 0.0 } inside a markdown fenced block."
	usr := text
	think := false
	resp, err := chatModel.Chat(ctx, []chat.Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: usr},
	}, &chat.ChatOptions{Temperature: 0.0, MaxCompletionTokens: 64, Thinking: &think})
	if err != nil || resp.Content == "" {
		pipelineWarn(ctx, "IntentDetect", "model_call_failed", map[string]interface{}{"error": err})
		return "general"
	}
	body := extractJSONBody(resp.Content)
	var ir intentResp
	if err := json.Unmarshal([]byte(body), &ir); err != nil {
		pipelineWarn(ctx, "IntentDetect", "parse_failed", map[string]interface{}{"body": body, "error": err.Error()})
		return "general"
	}
	pipelineInfo(
		ctx,
		"IntentDetect",
		"result",
		map[string]interface{}{"intent": ir.Intent, "confidence": ir.Confidence},
	)
	switch strings.ToLower(strings.TrimSpace(ir.Intent)) {
	case "definition", "howto", "compare", "qa", "general":
		return strings.ToLower(ir.Intent)
	default:
		return "general"
	}
}

// extractJSONBody extracts a JSON body from a string
func extractJSONBody(text string) string {
	t := strings.TrimSpace(text)
	// Try fenced block first
	if i := strings.Index(t, "{"); i >= 0 {
		j := strings.LastIndex(t, "}")
		if j > i {
			return t[i : j+1]
		}
	}
	return "{}"
}

// Close Releases resources
func (p *PluginPreprocess) Close() {
}

// ShutdownHandler Returns shutdown function
func (p *PluginPreprocess) ShutdownHandler() func() {
	return func() {
		p.Close()
	}
}
