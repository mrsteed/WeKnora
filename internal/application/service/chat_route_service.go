package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	jsonutil "github.com/Tencent/WeKnora/internal/utils"
)

type chatRouteService struct {
	modelService          interfaces.ModelService
	llmCallTimeoutSeconds int
	llmCallTimeoutSource  string
}

const defaultChatRouteLLMCallTimeout = 120 * time.Second

const (
	chatRouteTimeoutSourceAgentConfig = "agent_llm_timeout_config"
	chatRouteTimeoutSourceDefault     = "chat_route_default"
)

var explicitRouteFullDocumentIntentRE = regexp.MustCompile(`(?i)((输出|生成|撰写|编写|写(?:一份|一篇)?|整理|形成|给我|提供).{0,40}(完整(?:版)?(?:方案|报告|文档)?|全文|文档|报告|技术方案|设计方案|标书|投标方案|实施方案|markdown))|((完整(?:版)?|全文).{0,20}(文档|方案|报告|技术方案|设计方案|标书|markdown))`)
var explicitRouteFullTranslationIntentRE = regexp.MustCompile(`(?i)((全文|整篇|全篇|整个文档|整份文档|完整文档|完整译文|文档全文|full\s*document|whole\s*document|entire\s*document).{0,24}(翻译|译成|translate))|((翻译|译成|translate).{0,24}(全文|整篇|全篇|整个文档|整份文档|完整文档|完整译文|文档全文|full\s*document|whole\s*document|entire\s*document))|((文档|markdown).{0,12}(完整|全文|整篇|全篇).{0,12}(翻译|译成))|((翻译|译成).{0,12}(完整|全文|整篇|全篇).{0,12}(文档|markdown))`)

func NewChatRouteService(modelService interfaces.ModelService, cfg *config.Config) interfaces.ChatRouteService {
	llmCallTimeoutSeconds := 0
	llmCallTimeoutSource := chatRouteTimeoutSourceDefault
	if cfg != nil && cfg.Agent != nil && cfg.Agent.LLMCallTimeout > 0 {
		llmCallTimeoutSeconds = cfg.Agent.LLMCallTimeout
		llmCallTimeoutSource = chatRouteTimeoutSourceAgentConfig
	}
	return &chatRouteService{modelService: modelService, llmCallTimeoutSeconds: llmCallTimeoutSeconds, llmCallTimeoutSource: llmCallTimeoutSource}
}

func (s *chatRouteService) routeDecisionTimeout() time.Duration {
	timeout, _ := s.routeDecisionTimeoutInfo()
	return timeout
}

func (s *chatRouteService) routeDecisionTimeoutInfo() (time.Duration, string) {
	if s != nil && s.llmCallTimeoutSeconds > 0 {
		source := strings.TrimSpace(s.llmCallTimeoutSource)
		if source == "" {
			source = chatRouteTimeoutSourceAgentConfig
		}
		return time.Duration(s.llmCallTimeoutSeconds) * time.Second, source
	}
	return defaultChatRouteLLMCallTimeout, chatRouteTimeoutSourceDefault
}

func (s *chatRouteService) Decide(ctx context.Context, input types.ChatRouteInput) (*types.ChatRouteDecision, error) {
	startedAt := time.Now()
	timeout, timeoutSource := s.routeDecisionTimeoutInfo()
	trimmedQuery := strings.TrimSpace(input.Query)
	if trimmedQuery == "" {
		decision := fallbackChatRouteDecision(input, "empty_query")
		logChatRouteFallback(ctx, input, decision, "empty_query", nil, timeout, timeoutSource, time.Since(startedAt))
		return decision, nil
	}

	routeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	modelID := strings.TrimSpace(input.ModelID)
	if modelID == "" {
		decision := fallbackChatRouteDecision(input, "no_route_model")
		logChatRouteFallback(routeCtx, input, decision, "no_route_model", nil, timeout, timeoutSource, time.Since(startedAt))
		return decision, nil
	}
	logger.Infof(ctx, "[ChatRouter][Model] request model=%s timeout=%s timeout_source=%s endpoint=%s", modelID, timeout, timeoutSource, strings.TrimSpace(input.EndpointMode))

	chatModel, err := s.modelService.GetChatModel(routeCtx, modelID)
	if err != nil {
		decision := fallbackChatRouteDecision(input, "route_model_load_failed")
		logChatRouteFallback(routeCtx, input, decision, "route_model_load_failed", err, timeout, timeoutSource, time.Since(startedAt))
		return decision, fmt.Errorf("load route model %s: %w", modelID, err)
	}
	thinkingDisabled := false

	response, err := chatModel.Chat(routeCtx, []chat.Message{
		{Role: "system", Content: chatRouteSystemPrompt},
		{Role: "user", Content: buildChatRoutePrompt(input)},
	}, &chat.ChatOptions{
		Temperature:         0,
		MaxCompletionTokens: 400,
		Thinking:            &thinkingDisabled,
		Format:              jsonutil.GenerateSchema[types.ChatRouteDecision](),
	})
	if err != nil {
		decision := fallbackChatRouteDecision(input, "route_model_request_failed")
		logChatRouteFallback(routeCtx, input, decision, "route_model_request_failed", err, timeout, timeoutSource, time.Since(startedAt))
		return decision, fmt.Errorf("route model chat failed: %w", err)
	}

	var decision types.ChatRouteDecision
	if err := json.Unmarshal([]byte(response.Content), &decision); err != nil {
		fallback := fallbackChatRouteDecision(input, "route_model_invalid_json")
		logChatRouteFallback(routeCtx, input, fallback, "route_model_invalid_json", err, timeout, timeoutSource, time.Since(startedAt))
		return fallback, fmt.Errorf("parse route model response: %w", err)
	}

	normalized := normalizeChatRouteDecision(&decision, input)
	if isChatRouteFallbackDecision(normalized) {
		logChatRouteFallback(routeCtx, input, normalized, "model_route_normalization_fallback", nil, timeout, timeoutSource, time.Since(startedAt))
	}
	logger.Infof(ctx, "[ChatRouter][Model] model=%s raw_kind=%s normalized_kind=%s confidence=%.2f elapsed_ms=%d timeout=%s timeout_source=%s", modelID, strings.TrimSpace(string(decision.Kind)), normalized.Kind, normalized.Confidence, time.Since(startedAt).Milliseconds(), timeout, timeoutSource)
	return normalized, nil
}

const chatRouteSystemPrompt = "你是对话任务路由器，只能输出 JSON。不要回答用户问题，不要生成正文。请根据用户问题、知识范围、是否启用 Agent、是否存在可续写文档、是否有附件/图片等上下文，判断最合适的执行路线。只有用户明确要求撰写完整方案、完整报告、完整标书、完整长篇 Markdown，或明确要求输出完整译文、全文翻译、整篇翻译、完整文档翻译时，才能选择 full_document 或 knowledge_grounded_full_document。普通事实问答、字段问答、解释型问答默认选择 normal_qa 或 agent_qa。若不确定，请优先保持当前智能体/接口模式，不要升级成长文档。"

func buildChatRoutePrompt(input types.ChatRouteInput) string {
	payload, _ := json.MarshalIndent(input, "", "  ")
	return fmt.Sprintf(`请根据下面的上下文进行路由分类，只返回符合 schema 的 JSON。

分类规则：
1. 用户明确要求“输出/生成/撰写/编写/整理/形成”正式完整文档时，优先识别为长文档创建请求。
2. “技术方案”“实施方案”“投标方案”“设计方案”“完整报告”“完整标书”“长篇 Markdown”属于正式长文档类型；“完整译文”“全文翻译”“整篇翻译”“完整文档翻译”也属于长文档创建请求。
3. 若存在有效知识库范围，应选择 knowledge_grounded_full_document；若没有知识库范围，选择 full_document。
4. 若只是询问已有方案的风险、章节、字段、含义、摘要或对比，或者只是要求翻译一句话、一小段内容、某个字段，不要选择长文档，保持 normal_qa 或 agent_qa。
5. 有附件/图片、自动续写、已有 base artifact 时，不要将新建长文档作为首选路线。

正例：
- “请输出北海电厂的技术方案” => knowledge_grounded_full_document 或 full_document
- “帮我生成一份项目实施方案” => knowledge_grounded_full_document 或 full_document
- “请撰写完整投标方案” => knowledge_grounded_full_document 或 full_document
- “请把这篇文档完整翻译成中文 Markdown” => knowledge_grounded_full_document 或 full_document
- “对全文完成翻译” => knowledge_grounded_full_document 或 full_document

反例：
- “这个方案有哪些风险？” => normal_qa 或 agent_qa
- “北海电厂技术方案包含哪些章节？” => normal_qa 或 agent_qa
- “解释一下实施方案中的验收指标” => normal_qa 或 agent_qa
- “把这段话翻译成英文” => normal_qa 或 agent_qa

当前上下文：
%s`, string(payload))
}

func logChatRouteFallback(ctx context.Context, input types.ChatRouteInput, decision *types.ChatRouteDecision, reason string, err error, timeout time.Duration, timeoutSource string, elapsed time.Duration) {
	decisionKind := ""
	decisionReason := ""
	regexFallbackHit := false
	if decision != nil {
		decisionKind = string(decision.Kind)
		decisionReason = decision.Reason
		regexFallbackHit = strings.Contains(decision.Reason, "regex_full_document_fallback")
	}
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	logger.Warnf(ctx, "[ChatRouter][Fallback] reason=%s decision_kind=%s decision_reason=%q regex_full_document_fallback=%t elapsed_ms=%d timeout=%s timeout_source=%s endpoint=%s err=%s", reason, decisionKind, decisionReason, regexFallbackHit, elapsed.Milliseconds(), timeout, timeoutSource, strings.TrimSpace(input.EndpointMode), errText)
}

func isChatRouteFallbackDecision(decision *types.ChatRouteDecision) bool {
	if decision == nil {
		return false
	}
	reason := strings.TrimSpace(decision.Reason)
	return strings.Contains(reason, "unknown_route_kind") ||
		strings.Contains(reason, "empty_route_decision") ||
		strings.Contains(reason, "regex_full_document_fallback")
}

func fallbackChatRouteDecision(input types.ChatRouteInput, reason string) *types.ChatRouteDecision {
	if decision := regexFallbackChatRouteDecision(input, reason); decision != nil {
		return decision
	}
	return conservativeChatRouteDecision(input, reason)
}

func regexFallbackChatRouteDecision(input types.ChatRouteInput, reason string) *types.ChatRouteDecision {
	if !canRegexFallbackToFullDocument(input) {
		return nil
	}

	kind := types.ChatRouteFullDocument
	useKnowledge := false
	if input.HasSelectedKnowledge || input.HasEffectiveAgentKB {
		kind = types.ChatRouteKnowledgeGroundedFullDoc
		useKnowledge = true
	}

	useAgent := strings.TrimSpace(input.EndpointMode) == "agent_qa" || input.AgentModeEnabledByConfig || input.AgentConfigured
	return &types.ChatRouteDecision{
		Kind:            kind,
		Intent:          types.ChatDocumentIntentNormal,
		Operation:       types.ChatDocumentOperationCreate,
		OutputMode:      types.ChatDocumentOutputModeFull,
		UseAgent:        useAgent,
		UseKnowledge:    useKnowledge,
		UseLongDocument: true,
		NeedArtifact:    true,
		Confidence:      0.65,
		Reason:          appendFallbackReason(reason, "regex_full_document_fallback"),
	}
}

func canRegexFallbackToFullDocument(input types.ChatRouteInput) bool {
	if strings.TrimSpace(input.Query) == "" {
		return false
	}
	if input.AutoContinue || strings.TrimSpace(input.ExplicitBaseArtifactID) != "" {
		return false
	}
	if input.HasAttachments || input.HasImages {
		return false
	}
	return explicitRouteFullDocumentIntentRE.MatchString(input.Query) || explicitRouteFullTranslationIntentRE.MatchString(input.Query)
}

func conservativeChatRouteDecision(input types.ChatRouteInput, reason string) *types.ChatRouteDecision {
	kind := types.ChatRouteNormalQA
	useAgent := false
	if input.AgentModeEnabledByConfig && strings.TrimSpace(input.EndpointMode) == "agent_qa" {
		kind = types.ChatRouteAgentQA
		useAgent = true
	}
	useKnowledge := input.HasSelectedKnowledge || input.HasEffectiveAgentKB || input.WebSearchEnabled
	return &types.ChatRouteDecision{
		Kind:            kind,
		UseAgent:        useAgent,
		UseKnowledge:    useKnowledge,
		UseLongDocument: false,
		NeedArtifact:    false,
		Confidence:      0,
		Reason:          reason,
	}
}

func normalizeChatRouteDecision(decision *types.ChatRouteDecision, input types.ChatRouteInput) *types.ChatRouteDecision {
	if decision == nil {
		return fallbackChatRouteDecision(input, "empty_route_decision")
	}

	normalized := *decision
	switch strings.TrimSpace(string(decision.Kind)) {
	case string(types.ChatRouteNormalQA):
		normalized.Kind = types.ChatRouteNormalQA
	case string(types.ChatRouteAgentQA):
		normalized.Kind = types.ChatRouteAgentQA
	case string(types.ChatRouteShortDocument):
		normalized.Kind = types.ChatRouteShortDocument
	case string(types.ChatRouteFullDocument):
		normalized.Kind = types.ChatRouteFullDocument
	case string(types.ChatRouteKnowledgeGroundedFullDoc):
		normalized.Kind = types.ChatRouteKnowledgeGroundedFullDoc
	case "doc_generation", "document_generation", "long_document", "long_doc":
		if input.HasSelectedKnowledge || input.HasEffectiveAgentKB {
			normalized.Kind = types.ChatRouteKnowledgeGroundedFullDoc
		} else {
			normalized.Kind = types.ChatRouteFullDocument
		}
	case string(types.ChatRouteDocumentEdit):
		normalized.Kind = types.ChatRouteDocumentEdit
	case string(types.ChatRouteKnowledgeGroundedContinue):
		normalized.Kind = types.ChatRouteKnowledgeGroundedContinue
	default:
		fallback := fallbackChatRouteDecision(input, "unknown_route_kind")
		fallback.Reason = appendFallbackReason(strings.TrimSpace(normalized.Reason), fallback.Reason)
		return fallback
	}

	normalized.Intent = strings.TrimSpace(normalized.Intent)
	normalized.Operation = strings.TrimSpace(normalized.Operation)
	normalized.OutputMode = normalizeRouteOutputMode(normalized.OutputMode, normalized.Kind)
	normalized.TargetHeading = strings.TrimSpace(normalized.TargetHeading)
	normalized.MergeMode = strings.TrimSpace(normalized.MergeMode)
	normalized.Reason = strings.TrimSpace(normalized.Reason)
	if normalized.Confidence < 0 {
		normalized.Confidence = 0
	} else if normalized.Confidence > 1 {
		normalized.Confidence = 1
	}

	// Route models sometimes emit the generic `full_document` kind even when the
	// current turn already has an effective KB scope. In WeKnora that scope means
	// the correct execution path is knowledge-grounded full document generation,
	// otherwise the handler later rejects the route with `has_knowledge_scope` and
	// falls back to ordinary Agent QA.
	if normalized.Kind == types.ChatRouteFullDocument && (input.HasSelectedKnowledge || input.HasEffectiveAgentKB) {
		normalized.Kind = types.ChatRouteKnowledgeGroundedFullDoc
	}

	if !normalized.UseAgent {
		normalized.UseAgent = normalized.Kind == types.ChatRouteAgentQA || normalized.Kind == types.ChatRouteDocumentEdit || normalized.Kind == types.ChatRouteFullDocument || normalized.Kind == types.ChatRouteKnowledgeGroundedFullDoc || normalized.Kind == types.ChatRouteKnowledgeGroundedContinue
	}
	if !normalized.UseKnowledge {
		normalized.UseKnowledge = normalized.Kind == types.ChatRouteKnowledgeGroundedFullDoc || normalized.Kind == types.ChatRouteKnowledgeGroundedContinue || input.HasSelectedKnowledge || input.HasEffectiveAgentKB || input.WebSearchEnabled
	}
	normalized.UseLongDocument = routeKindUsesLongDocument(normalized.Kind)
	if !normalized.NeedArtifact {
		normalized.NeedArtifact = normalized.Kind == types.ChatRouteShortDocument || normalized.Kind == types.ChatRouteFullDocument || normalized.Kind == types.ChatRouteKnowledgeGroundedFullDoc || normalized.Kind == types.ChatRouteDocumentEdit || normalized.Kind == types.ChatRouteKnowledgeGroundedContinue
	}
	if normalized.Reason == "" {
		normalized.Reason = "model_route_decision"
	}
	return &normalized
}

func normalizeRouteOutputMode(outputMode string, kind types.ChatRouteKind) string {
	switch strings.TrimSpace(outputMode) {
	case types.ChatDocumentOutputModeFull:
		return types.ChatDocumentOutputModeFull
	case types.ChatDocumentOutputModeDelta:
		return types.ChatDocumentOutputModeDelta
	}
	switch kind {
	case types.ChatRouteFullDocument, types.ChatRouteKnowledgeGroundedFullDoc:
		return types.ChatDocumentOutputModeFull
	case types.ChatRouteDocumentEdit, types.ChatRouteKnowledgeGroundedContinue:
		return types.ChatDocumentOutputModeDelta
	default:
		return ""
	}
}

func routeKindUsesLongDocument(kind types.ChatRouteKind) bool {
	switch kind {
	case types.ChatRouteFullDocument, types.ChatRouteKnowledgeGroundedFullDoc:
		return true
	default:
		return false
	}
}

func appendFallbackReason(existing string, fallback string) string {
	if existing == "" {
		return fallback
	}
	return existing + "; " + fallback
}
