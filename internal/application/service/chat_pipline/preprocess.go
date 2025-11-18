package chatpipline

import (
    "context"
    "encoding/json"
    "regexp"
    "strings"
    "unicode"
    "unicode/utf8"

    "github.com/Tencent/WeKnora/internal/config"
    "github.com/Tencent/WeKnora/internal/models/chat"
    "github.com/Tencent/WeKnora/internal/types"
    "github.com/Tencent/WeKnora/internal/types/interfaces"
    "github.com/yanyiwu/gojieba"
)

// PluginPreprocess Query preprocessing plugin
type PluginPreprocess struct {
    config    *config.Config
    jieba     *gojieba.Jieba
    stopwords map[string]struct{}
    modelService interfaces.ModelService
}

// Regular expressions for text cleaning
var (
	multiSpaceRegex = regexp.MustCompile(`\s+`)                                 // Multiple spaces
	urlRegex        = regexp.MustCompile(`https?://\S+`)                        // URLs
	emailRegex      = regexp.MustCompile(`\b[\w.%+-]+@[\w.-]+\.[a-zA-Z]{2,}\b`) // Email addresses
	punctRegex      = regexp.MustCompile(`[^\p{L}\p{N}\s]`)                     // Punctuation marks
)

const maxProcessedTokens = 12

// NewPluginPreprocess Creates a new query preprocessing plugin
func NewPluginPreprocess(
    eventManager *EventManager,
    config *config.Config,
    cleaner interfaces.ResourceCleaner,
    modelService interfaces.ModelService,
) *PluginPreprocess {
	// Use default dictionary for Jieba tokenizer
	jieba := gojieba.NewJieba()

	// Load stopwords from built-in stopword library
	stopwords := loadStopwords()

    res := &PluginPreprocess{
        config:    config,
        jieba:     jieba,
        stopwords: stopwords,
        modelService: modelService,
    }

	// Register resource cleanup function
	if cleaner != nil {
		cleaner.RegisterWithName("JiebaPreprocessor", func() error {
			res.Close()
			return nil
		})
	}

	eventManager.Register(res)
	return res
}

// Load stopwords
func loadStopwords() map[string]struct{} {
	// Directly use some common stopwords built into Jieba
	commonStopwords := []string{
		"的", "了", "和", "是", "在", "我", "你", "他", "她", "它",
		"这", "那", "什么", "怎么", "如何", "为什么", "哪里", "什么时候",
		"the", "is", "are", "am", "I", "you", "he", "she", "it", "this",
		"that", "what", "how", "a", "an", "and", "or", "but", "if", "of",
		"to", "in", "on", "at", "by", "for", "with", "about", "from",
		"有", "无", "好", "来", "去", "说", "看", "想", "会", "可以",
		"吗", "呢", "啊", "吧", "的话", "就是", "只是", "因为", "所以",
	}

	result := make(map[string]struct{}, len(commonStopwords))
	for _, word := range commonStopwords {
		result[word] = struct{}{}
	}
	return result
}

// ActivationEvents Register activation events
func (p *PluginPreprocess) ActivationEvents() []types.EventType {
	return []types.EventType{types.PREPROCESS_QUERY}
}

// OnEvent Process events
func (p *PluginPreprocess) OnEvent(ctx context.Context, eventType types.EventType, chatManage *types.ChatManage, next func() *PluginError) *PluginError {
    rawQuery := strings.TrimSpace(chatManage.RewriteQuery)
    if rawQuery == "" {
        return next()
    }

	pipelineInfo(ctx, "Preprocess", "input", map[string]interface{}{
		"session_id":    chatManage.SessionID,
		"rewrite_query": rawQuery,
	})

	normalized := normalizeWhitespace(rawQuery)
	sanitized := strings.TrimSpace(p.cleanText(normalized))
	if sanitized == "" {
		sanitized = normalized
	}

    var (
        processed    = sanitized
        strategy     = "original"
        tokenPreview string
        tokenCount   int
    )

	switch {
	case containsChineseCharacters(sanitized):
		segments := p.segmentText(sanitized)
		tokens := p.selectMeaningfulTokens(segments)
		tokenCount = len(tokens)
		if len(tokens) >= 2 {
			processed = strings.Join(tokens, " ")
			strategy = "zh_tokens"
			tokenPreview = strings.Join(tokens, ",")
		} else {
			strategy = "fallback_original"
		}
	case containsLatinLetters(sanitized):
		processed = normalizeLatinQuery(sanitized)
		if processed != sanitized {
			strategy = "latin_normalize"
		}
	default:
		strategy = "original"
	}

	if strings.TrimSpace(processed) == "" {
		processed = rawQuery
		strategy = "fallback_original"
	}

    chatManage.ProcessedQuery = processed
    chatManage.QueryIntent = p.detectIntentLLM(ctx, chatManage, sanitized)

    pipelineInfo(ctx, "Preprocess", "output", map[string]interface{}{
        "session_id":      chatManage.SessionID,
        "processed_query": processed,
        "strategy":        strategy,
        "token_count":     tokenCount,
        "token_preview":   truncateForLog(tokenPreview),
        "query_intent":    chatManage.QueryIntent,
    })

	return next()
}

// cleanText Basic text cleaning
func (p *PluginPreprocess) cleanText(text string) string {
	// Remove URLs
	text = urlRegex.ReplaceAllString(text, " ")

	// Remove email addresses
	text = emailRegex.ReplaceAllString(text, " ")

	// Remove excessive spaces
	text = multiSpaceRegex.ReplaceAllString(text, " ")

	// Remove punctuation marks
	text = punctRegex.ReplaceAllString(text, " ")

	// Trim leading and trailing spaces
	text = strings.TrimSpace(text)

	return text
}

// segmentText Text tokenization
func (p *PluginPreprocess) segmentText(text string) []string {
	// Use Jieba tokenizer for tokenization, using search engine mode
	segments := p.jieba.CutForSearch(text, true)
	return segments
}

// filterStopwords Filter stopwords
func (p *PluginPreprocess) selectMeaningfulTokens(segments []string) []string {
	var tokens []string
	seen := make(map[string]struct{})

	for _, word := range segments {
		word = strings.TrimSpace(word)
		if word == "" {
			continue
		}
		if _, stop := p.stopwords[word]; stop {
			continue
		}
		if _, exists := seen[word]; exists {
			continue
		}
		if !isInformativeToken(word) {
			continue
		}

		seen[word] = struct{}{}
		tokens = append(tokens, word)
		if len(tokens) >= maxProcessedTokens {
			break
		}
	}

	return tokens
}

// isBlank Check if a string is blank
func isInformativeToken(token string) bool {
	if token == "" {
		return false
	}

	runeCount := utf8.RuneCountInString(token)
	if runeCount == 1 {
		r, _ := utf8.DecodeRuneInString(token)
		if unicode.IsDigit(r) {
			return true
		}
		if r <= unicode.MaxASCII && unicode.IsLetter(r) {
			return true
		}
		return false
	}

	return true
}

func containsChineseCharacters(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func containsLatinLetters(text string) bool {
	for _, r := range text {
		if r <= unicode.MaxASCII && unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func normalizeWhitespace(text string) string {
	text = strings.TrimSpace(text)
	return multiSpaceRegex.ReplaceAllString(text, " ")
}

func normalizeLatinQuery(text string) string {
    text = strings.ToLower(text)
    text = multiSpaceRegex.ReplaceAllString(text, " ")
    return strings.TrimSpace(text)
}

type intentResp struct {
    Intent     string  `json:"intent"`
    Confidence float64 `json:"confidence"`
}

func (p *PluginPreprocess) detectIntentLLM(ctx context.Context, chatManage *types.ChatManage, text string) string {
    if p.modelService == nil || chatManage.ChatModelID == "" {
        pipelineWarn(ctx, "IntentDetect", "skip", map[string]interface{}{ "reason": "no_model", "session_id": chatManage.SessionID })
        return "general"
    }
    chatModel, err := p.modelService.GetChatModel(ctx, chatManage.ChatModelID)
    if err != nil {
        pipelineWarn(ctx, "IntentDetect", "get_model_failed", map[string]interface{}{ "error": err.Error(), "model_id": chatManage.ChatModelID })
        return "general"
    }
    pipelineInfo(ctx, "IntentDetect", "start", map[string]interface{}{ "session_id": chatManage.SessionID, "model_id": chatManage.ChatModelID })
    sys := "You are a query intent classifier. Classify the user's query into one of: definition, howto, compare, qa, general. Respond ONLY with a JSON object {\"intent\": \"...\", \"confidence\": 0.0 } inside a markdown fenced block."
    usr := text
    think := false
    resp, err := chatModel.Chat(ctx, []chat.Message{
        {Role: "system", Content: sys},
        {Role: "user", Content: usr},
    }, &chat.ChatOptions{Temperature: 0.0, MaxCompletionTokens: 64, Thinking: &think})
    if err != nil || resp.Content == "" {
        pipelineWarn(ctx, "IntentDetect", "model_call_failed", map[string]interface{}{ "error": err })
        return "general"
    }
    body := extractJSONBody(resp.Content)
    var ir intentResp
    if err := json.Unmarshal([]byte(body), &ir); err != nil {
        pipelineWarn(ctx, "IntentDetect", "parse_failed", map[string]interface{}{ "body": truncateForLog(body), "error": err.Error() })
        return "general"
    }
    pipelineInfo(ctx, "IntentDetect", "result", map[string]interface{}{ "intent": ir.Intent, "confidence": ir.Confidence })
    switch strings.ToLower(strings.TrimSpace(ir.Intent)) {
    case "definition", "howto", "compare", "qa", "general":
        return strings.ToLower(ir.Intent)
    default:
        return "general"
    }
}

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

// Ensure resources are properly released
func (p *PluginPreprocess) Close() {
	if p.jieba != nil {
		p.jieba.Free()
		p.jieba = nil
	}
}

// ShutdownHandler Returns shutdown function
func (p *PluginPreprocess) ShutdownHandler() func() {
	return func() {
		p.Close()
	}
}
