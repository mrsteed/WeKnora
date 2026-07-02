package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/infrastructure/chunker"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/models/rerank"
	"github.com/Tencent/WeKnora/internal/types"
)

var documentBudgetNegotiationTimeout = 4 * time.Second

var explicitFullDocumentIntentRE = regexp.MustCompile(`(?i)((输出|生成|撰写|编写|写(?:一份|一篇)?|整理|形成|给我|提供).{0,40}(完整(?:版)?|全文|文档|方案|报告|技术方案|设计方案|标书|投标方案|实施方案|markdown))|((完整(?:版)?|全文).{0,20}(文档|方案|报告|技术方案|设计方案|标书|markdown))`)
var documentEditNumberedHeadingRE = regexp.MustCompile(`^(#{1,6})\s*([0-9]+(?:\.[0-9]+)*)\s+(.+)$`)

const (
	documentRuntimeSlowFirstTokenThresholdMs   = int64(12000)
	documentRuntimeLowEvidenceThreshold        = 1
	documentRuntimeShortSectionThresholdTokens = 384
	documentRuntimeSectionTokenStep            = 512
	documentRuntimeSectionTimeoutStepSeconds   = 30
	documentGenerationDefaultLLMTimeoutSeconds = 210
	fullDocumentRollingSummaryRecentSections   = 2
	fullDocumentRollingSummaryEarlierRunes     = 180
	fullDocumentRollingSummaryRecentRunes      = 320
	fullDocumentRollingSummaryCarryForwardMax  = 4
)

// AgentQA performs agent-based question answering with conversation history and streaming support
// customAgent is optional - if provided, uses custom agent configuration instead of tenant defaults
// summaryModelID is optional - if provided, overrides the model from customAgent config

func (s *sessionService) AgentQA(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
) error {
	sessionID := req.Session.ID
	sessionJSON, err := json.Marshal(req.Session)
	if err != nil {
		logger.Errorf(ctx, "Failed to marshal session, session ID: %s, error: %v", sessionID, err)
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// customAgent is required for AgentQA (handler has already done permission check for shared agent)
	if req.CustomAgent == nil {
		logger.Warnf(ctx, "Custom agent not provided for session: %s", sessionID)
		return errors.New("custom agent configuration is required for agent QA")
	}

	// Resolve retrieval tenant using shared helper
	agentTenantID := s.resolveRetrievalTenantID(ctx, req)
	logger.Infof(ctx, "Start agent-based question answering, session ID: %s, agent tenant ID: %d, query: %s, session: %s",
		sessionID, agentTenantID, req.Query, string(sessionJSON))

	var tenantInfo *types.Tenant
	if v := ctx.Value(types.TenantInfoContextKey); v != nil {
		tenantInfo, _ = v.(*types.Tenant)
	}
	// When agent belongs to another tenant (shared agent), use agent's tenant for KB/model scope; load tenantInfo if needed
	if tenantInfo == nil || tenantInfo.ID != agentTenantID {
		if s.tenantService != nil {
			if agentTenant, err := s.tenantService.GetTenantByID(ctx, agentTenantID); err == nil && agentTenant != nil {
				tenantInfo = agentTenant
				logger.Infof(ctx, "Using agent tenant info for retrieval scope, tenant ID: %d", agentTenantID)
			}
		}
	}
	if tenantInfo == nil {
		logger.Warnf(ctx, "Tenant info not available for agent tenant %d, proceeding with defaults", agentTenantID)
		tenantInfo = &types.Tenant{ID: agentTenantID}
	}
	ctx = ensureRetrievalTenantContext(ctx, agentTenantID, tenantInfo)

	// Ensure defaults are set
	req.CustomAgent.EnsureDefaults()

	// Build AgentConfig from custom agent and tenant info
	agentConfig, err := s.buildAgentConfig(ctx, req, tenantInfo, agentTenantID)
	if err != nil {
		return err
	}

	// Set VLM model ID for tool result image analysis (runtime-only field)
	if req.CustomAgent != nil && req.CustomAgent.Config.VLMModelID != "" {
		agentConfig.VLMModelID = req.CustomAgent.Config.VLMModelID
	}

	// Resolve model ID using shared helper (AgentQA requires a model, so error if not found)
	effectiveModelID, err := s.resolveChatModelID(ctx, req, agentConfig.KnowledgeBases, agentConfig.KnowledgeIDs)
	if err != nil {
		return err
	}
	if effectiveModelID == "" {
		logger.Warnf(ctx, "No summary model configured for custom agent %s", req.CustomAgent.ID)
		return errors.New("summary model (model_id) is not configured in custom agent settings")
	}

	summaryModel, err := s.modelService.GetChatModel(ctx, effectiveModelID)
	if err != nil {
		logger.Warnf(ctx, "Failed to get chat model: %v", err)
		return fmt.Errorf("failed to get chat model: %w", err)
	}
	if shouldUseLongDocumentTranslationContinuationPath(req) {
		logger.Infof(ctx, "[LongDocument][Router] selected=translation_continuation run_id=%s", strings.TrimSpace(req.GenerationRunID))
		return s.runLongDocumentTranslationContinuationPath(ctx, req, eventBus, summaryModel, agentConfig)
	}
	if shouldUseLongDocumentTranslationPath(req) {
		logger.Infof(ctx, "[LongDocument][Router] selected=translation_full_document kb_scope=%d", len(req.KnowledgeIDs))
		return s.runLongDocumentTranslationPath(ctx, req, eventBus, summaryModel, agentConfig)
	}
	if shouldUseKnowledgeGroundedDocumentContinuationPath(req, agentConfig) {
		logger.Infof(ctx, "[FullDocument][Router] selected=knowledge_grounded_document_continuation kb_scope=%d", len(agentConfig.SearchTargets.GetAllKnowledgeBaseIDs()))
		return s.runKnowledgeGroundedDocumentContinuationPath(ctx, req, eventBus, summaryModel, agentConfig)
	}
	if shouldUseDedicatedDocumentEditPath(req) {
		return s.runDedicatedDocumentEditPath(ctx, req, eventBus, summaryModel, agentConfig)
	}
	if shouldUseKnowledgeGroundedFullDocumentGenerationPath(req, agentConfig) {
		logger.Infof(ctx, "[FullDocument][Router] selected=knowledge_grounded_full_document kb_scope=%d", len(agentConfig.SearchTargets.GetAllKnowledgeBaseIDs()))
		return s.runKnowledgeGroundedFullDocumentGenerationPath(ctx, req, eventBus, summaryModel, agentConfig)
	}
	if shouldUseDedicatedFullDocumentGenerationPath(req, agentConfig) {
		logger.Infof(ctx, "[FullDocument][Router] selected=dedicated_full_document has_effective_local_knowledge_scope=false")
		return s.runDedicatedFullDocumentGenerationPath(ctx, req, eventBus, summaryModel, agentConfig)
	}
	if req != nil && req.DocumentOutputMode == types.ChatDocumentOutputModeFull && hasEffectiveLocalKnowledgeScope(req, agentConfig) {
		logger.Infof(ctx, "[FullDocument][Router] selected=generic_agent reason=effective_local_knowledge_scope")
	}

	// Get rerank model from custom agent config only when knowledge_search can
	// actually run. A disabled KB scope makes all KB tools ineffective, so it
	// must not force users to configure an otherwise-unused rerank model.
	var rerankModel rerank.Reranker
	if agentRequiresRerankModel(req.CustomAgent) {
		// Rerank model is resolved purely from the agent config now.
		// We used to fall back to ConversationConfig.RerankModelID at
		// the tenant level, but that path encouraged "leave rerank
		// blank on the agent and inherit silently" which made debugging
		// retrieval quality a guessing game across tenant settings vs
		// agent settings. Forcing the agent to declare its own rerank
		// model puts the configuration where the user actually edits
		// the agent. If a Wiki-only agent doesn't need reranking,
		// agentRequiresRerankModel() below already lets it pass.
		rerankModelID := req.CustomAgent.Config.RerankModelID
		if rerankModelID == "" {
			logger.Warnf(ctx, "No rerank model configured for custom agent %s, but knowledge_search tool is enabled", req.CustomAgent.ID)
			return errors.New("rerank model is not configured: please set rerank_model_id on the agent")
		}

		rerankModel, err = s.modelService.GetRerankModel(ctx, rerankModelID)
		if err != nil {
			logger.Warnf(ctx, "Failed to get rerank model: %v", err)
			return fmt.Errorf("failed to get rerank model: %w", err)
		}
	} else {
		logger.Infof(ctx, "knowledge_search is unavailable for the effective agent scope, skipping rerank model initialization")
	}

	// Load multi-turn history directly from DB (the single source of truth).
	// AgentSteps on each historical assistant message are expanded into proper
	// assistant_with_tool_calls + tool messages so the model can see what was
	// tried last turn — except final_answer, which is replayed as the trailing
	// canonical assistant message.
	var llmContext []chat.Message
	if agentConfig.MultiTurnEnabled {
		historyTurns := agentConfig.HistoryTurns
		if historyTurns <= 0 {
			historyTurns = 5
		}
		llmContext, err = LoadAgentHistory(ctx, s.messageRepo, sessionID, historyTurns)
		if err != nil {
			logger.Warnf(ctx, "Failed to load agent history from DB: %v, continuing without history", err)
			llmContext = []chat.Message{}
		}
		logger.Infof(ctx, "Loaded %d history messages from DB (turns=%d)", len(llmContext), historyTurns)
	} else {
		logger.Infof(ctx, "Multi-turn disabled for this agent, running without history")
		llmContext = []chat.Message{}
	}

	// Create agent engine with EventBus
	logger.Info(ctx, "Creating agent engine")
	engine, err := s.agentService.CreateAgentEngine(
		ctx,
		agentConfig,
		summaryModel,
		rerankModel,
		eventBus,
		sessionID,
		req.AssistantMessageID,
	)
	if err != nil {
		logger.Errorf(ctx, "Failed to create agent engine: %v", err)
		return err
	}
	if documentContext := agentDocumentContextFromQARequest(req); documentContext != nil {
		engine.SetDocumentContext(documentContext)
	}

	// Route image data based on agent model's vision capability
	var agentModelSupportsVision bool
	if effectiveModelID != "" {
		if modelInfo, err := s.modelService.GetModelByID(ctx, effectiveModelID); err == nil && modelInfo != nil {
			agentModelSupportsVision = modelInfo.Parameters.SupportsVision
		}
	}

	agentQuery := req.Query
	var agentImageURLs []string
	if agentModelSupportsVision && len(req.ImageURLs) > 0 {
		agentImageURLs = req.ImageURLs
		logger.Infof(ctx, "Agent model supports vision, passing %d image(s) directly", len(agentImageURLs))
	} else if req.ImageDescription != "" {
		agentQuery = req.Query + "\n\n[用户上传图片内容]\n" + req.ImageDescription
		logger.Infof(ctx, "Agent model does not support vision, appending image description (%d chars)", len(req.ImageDescription))
	}
	if shouldInlineQuotedContext(req) {
		agentQuery += "\n\n" + req.QuotedContext
	}
	// Inject attachment content (documents, audio transcripts, etc.) so the agent
	// can see uploaded files. Mirrors the behavior of the KnowledgeQA pipeline
	// (see chat_pipeline/into_chat_message.go).
	if len(req.Attachments) > 0 {
		agentQuery += req.Attachments.BuildPrompt()
		logger.Infof(ctx, "Appended %d attachment(s) to agent query", len(req.Attachments))
	}

	// Scope envelopes (runtime_context / must_use) are injected per LLM call inside
	// the agent engine only; we intentionally do not persist them on user messages
	// so multi-turn history stays clean and is not skewed by stale @mention scope.

	// Execute agent with streaming (asynchronously)
	// Events will be emitted to EventBus and handled by the Handler layer
	logger.Info(ctx, "Executing agent with streaming")
	if _, err := engine.Execute(ctx, sessionID, req.AssistantMessageID, agentQuery, llmContext, agentImageURLs); err != nil {
		logger.Errorf(ctx, "Agent execution failed: %v", err)
		// Emit error event to the EventBus used by this agent
		eventBus.Emit(ctx, event.Event{
			Type:      event.EventError,
			SessionID: sessionID,
			Data: event.ErrorData{
				Error:     err.Error(),
				Stage:     "agent_execution",
				SessionID: sessionID,
			},
		})
	}
	// Return empty - events will be handled by Handler via EventBus subscription
	return nil
}

func agentDocumentContextFromQARequest(req *types.QARequest) *types.AgentDocumentContext {
	if req == nil {
		return nil
	}
	if req.DocumentIntent == "" && req.DocumentOperation == "" && req.DocumentOutputMode == "" && req.BaseArtifactID == "" && !req.AutoContinue {
		return nil
	}
	return &types.AgentDocumentContext{
		Intent:            req.DocumentIntent,
		Operation:         req.DocumentOperation,
		OutputMode:        req.DocumentOutputMode,
		BaseArtifactID:    req.BaseArtifactID,
		UserGoal:          req.Query,
		TargetHeading:     req.DocumentTargetHeading,
		MergeMode:         req.DocumentMergeMode,
		QuotedContext:     req.QuotedContext,
		AutoContinue:      req.AutoContinue,
		AutoContinueRound: req.AutoContinueRound,
		CompletionMarker:  types.ChatDocumentCompletionMarker,
	}
}

func ensureRetrievalTenantContext(
	ctx context.Context,
	tenantID uint64,
	tenantInfo *types.Tenant,
) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if tenantID != 0 {
		currentTenantID, ok := types.TenantIDFromContext(ctx)
		if !ok || currentTenantID != tenantID {
			ctx = context.WithValue(ctx, types.TenantIDContextKey, tenantID)
		}
	}
	if tenantInfo != nil {
		currentTenantInfo, ok := types.TenantInfoFromContext(ctx)
		if !ok || currentTenantInfo == nil || currentTenantInfo.ID != tenantInfo.ID {
			ctx = context.WithValue(ctx, types.TenantInfoContextKey, tenantInfo)
		}
	}
	return ctx
}

func shouldApplyDocumentStopgap(req *types.QARequest) bool {
	if req == nil || req.BaseArtifactID == "" {
		return false
	}

	switch req.DocumentIntent {
	case types.ChatDocumentIntentContinue, types.ChatDocumentIntentRevise:
		return req.DocumentOutputMode != types.ChatDocumentOutputModeFull
	}

	switch req.DocumentOperation {
	case types.ChatDocumentOperationContinue, types.ChatDocumentOperationRevise:
		return req.DocumentOutputMode != types.ChatDocumentOutputModeFull
	default:
		return false
	}
}

func shouldInlineQuotedContext(req *types.QARequest) bool {
	return req != nil && req.QuotedContext != "" && !shouldApplyDocumentStopgap(req)
}

func shouldUseDedicatedDocumentEditPath(req *types.QARequest) bool {
	if req == nil || strings.TrimSpace(req.BaseArtifactID) == "" {
		return false
	}
	if req.DocumentOutputMode == types.ChatDocumentOutputModeFull {
		return false
	}
	switch req.DocumentIntent {
	case types.ChatDocumentIntentContinue, types.ChatDocumentIntentRevise:
	default:
		switch req.DocumentOperation {
		case types.ChatDocumentOperationContinue, types.ChatDocumentOperationRevise:
		default:
			return false
		}
	}
	if len(req.Attachments) > 0 || len(req.ImageURLs) > 0 || strings.TrimSpace(req.ImageDescription) != "" {
		return false
	}
	return strings.TrimSpace(req.QuotedContext) != ""
}

func shouldUseKnowledgeGroundedDocumentContinuationPath(req *types.QARequest, agentConfig *types.AgentConfig) bool {
	if req == nil || !req.AutoContinue {
		return false
	}
	if strings.TrimSpace(req.BaseArtifactID) == "" || req.BaseArtifact == nil {
		return false
	}
	if req.DocumentOutputMode != types.ChatDocumentOutputModeDelta {
		return false
	}
	if !hasEffectiveLocalKnowledgeScope(req, agentConfig) && strings.TrimSpace(req.GenerationRunID) == "" {
		return false
	}
	switch req.DocumentIntent {
	case types.ChatDocumentIntentContinue, types.ChatDocumentIntentRevise:
	default:
		switch req.DocumentOperation {
		case types.ChatDocumentOperationContinue, types.ChatDocumentOperationRevise:
		default:
			return false
		}
	}
	if len(req.Attachments) > 0 || len(req.ImageURLs) > 0 || strings.TrimSpace(req.ImageDescription) != "" {
		return false
	}
	return true
}

func shouldUseKnowledgeGroundedFullDocumentGenerationPath(req *types.QARequest, agentConfig *types.AgentConfig) bool {
	if req == nil || req.DocumentOutputMode != types.ChatDocumentOutputModeFull {
		return false
	}
	if strings.TrimSpace(req.DocumentTaskKind) == types.ChatDocumentTaskKindTranslation {
		return false
	}
	if strings.TrimSpace(req.Query) == "" || strings.TrimSpace(req.BaseArtifactID) != "" {
		return false
	}
	if req.AutoContinue {
		return false
	}
	switch req.DocumentIntent {
	case "", types.ChatDocumentIntentNormal, types.ChatDocumentIntentRegenerate:
	default:
		return false
	}
	switch req.DocumentOperation {
	case "", types.ChatDocumentOperationCreate, types.ChatDocumentOperationRegenerate:
	default:
		return false
	}
	if req.DocumentIntent != types.ChatDocumentIntentRegenerate && req.DocumentOperation != types.ChatDocumentOperationRegenerate && !hasExplicitFullDocumentIntent(req.Query) && !routeDecisionAllowsFullDocumentKind(req, types.ChatRouteKnowledgeGroundedFullDoc) {
		return false
	}
	if !hasEffectiveLocalKnowledgeScope(req, agentConfig) {
		return false
	}
	if len(req.Attachments) > 0 || len(req.ImageURLs) > 0 || strings.TrimSpace(req.ImageDescription) != "" {
		return false
	}
	return true
}

func hasEffectiveLocalKnowledgeScope(req *types.QARequest, agentConfig *types.AgentConfig) bool {
	if req != nil && (len(req.KnowledgeBaseIDs) > 0 || len(req.KnowledgeIDs) > 0) {
		return true
	}
	if agentConfig == nil {
		return false
	}
	return len(agentConfig.KnowledgeBases) > 0 || len(agentConfig.KnowledgeIDs) > 0 || len(agentConfig.SearchTargets) > 0
}

func hasExplicitFullDocumentIntent(query string) bool {
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return false
	}
	return explicitFullDocumentIntentRE.MatchString(trimmedQuery)
}

func routeDecisionAllowsFullDocumentKind(req *types.QARequest, kind types.ChatRouteKind) bool {
	if req == nil || req.RouteDecision == nil {
		return false
	}
	return req.RouteDecision.Kind == kind
}

func shouldUseDedicatedFullDocumentGenerationPath(req *types.QARequest, agentConfig *types.AgentConfig) bool {
	if req == nil || req.DocumentOutputMode != types.ChatDocumentOutputModeFull {
		return false
	}
	if strings.TrimSpace(req.DocumentTaskKind) == types.ChatDocumentTaskKindTranslation {
		return false
	}
	if strings.TrimSpace(req.Query) == "" || strings.TrimSpace(req.BaseArtifactID) != "" {
		return false
	}
	if req.AutoContinue {
		return false
	}
	switch req.DocumentIntent {
	case "", types.ChatDocumentIntentNormal, types.ChatDocumentIntentRegenerate:
	default:
		return false
	}
	switch req.DocumentOperation {
	case "", types.ChatDocumentOperationCreate, types.ChatDocumentOperationRegenerate:
	default:
		return false
	}
	if req.DocumentIntent != types.ChatDocumentIntentRegenerate && req.DocumentOperation != types.ChatDocumentOperationRegenerate && !hasExplicitFullDocumentIntent(req.Query) && !routeDecisionAllowsFullDocumentKind(req, types.ChatRouteFullDocument) {
		return false
	}
	if hasEffectiveLocalKnowledgeScope(req, agentConfig) || documentEditRequiresExternalRetrieval(req.Query) {
		return false
	}
	if len(req.Attachments) > 0 || len(req.ImageURLs) > 0 || strings.TrimSpace(req.ImageDescription) != "" {
		return false
	}
	return true
}

func documentEditRequiresExternalRetrieval(query string) bool {
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return false
	}
	retrievalKeywords := []string{
		"联网", "网络搜索", "网页搜索", "web search", "网上", "最新资料",
		"检索知识库", "搜索知识库", "查知识库", "参考知识库", "结合知识库",
		"参考资料", "外部资料", "补充资料", "查询数据库", "查数据库",
		"根据原文", "查询原文", "读取原文", "引用来源", "引用依据",
	}
	for _, keyword := range retrievalKeywords {
		if strings.Contains(trimmedQuery, keyword) {
			return true
		}
	}
	return false
}

func buildDedicatedDocumentEditSystemPrompt(language string, req *types.QARequest, useLocalKnowledge bool) string {
	var builder strings.Builder
	builder.WriteString("You are a dedicated document editor. Complete the task in a single pass using only the provided user goal and document editing context. ")
	builder.WriteString("Do not call tools. Do not output hidden reasoning. Do not regenerate the whole document unless the context explicitly asks for a full document. ")
	if useLocalKnowledge {
		builder.WriteString("Use only facts from <local_knowledge_context> and the provided document editing context. If local knowledge is insufficient, say 本地知识不足 or 待确认 instead of inventing content. ")
	}
	if strings.TrimSpace(language) != "" {
		builder.WriteString(fmt.Sprintf("Respond in %s. ", language))
	}
	if req != nil && req.DocumentIntent == types.ChatDocumentIntentRevise {
		builder.WriteString("For revision requests, prefer returning a <document_patch> envelope that can be merged deterministically. ")
	}
	if req != nil && req.DocumentIntent == types.ChatDocumentIntentContinue {
		builder.WriteString("For continuation requests, output only the missing continuation content or the completion marker when the document is complete. ")
	}
	return strings.TrimSpace(builder.String())
}

func buildDedicatedDocumentEditMessages(req *types.QARequest, language string, evidence knowledgeGroundedEvidencePack) []chat.Message {
	userContent := "User goal:\n" + strings.TrimSpace(req.Query)
	if strings.TrimSpace(req.QuotedContext) != "" {
		userContent += "\n\nDocument editing context:\n" + strings.TrimSpace(req.QuotedContext)
	}
	if structurePrompt := buildDocumentEditStructurePrompt(req); structurePrompt != "" {
		userContent += "\n\nDocument structure constraints:\n" + structurePrompt
	}
	if len(evidence.Items) > 0 {
		userContent += "\n\n" + buildKnowledgeGroundedLocalKnowledgeContext(evidence)
	}
	return []chat.Message{
		{Role: "system", Content: buildDedicatedDocumentEditSystemPrompt(language, req, len(evidence.Items) > 0)},
		{Role: "user", Content: userContent},
	}
}

func buildDocumentEditStructurePrompt(req *types.QARequest) string {
	if req == nil {
		return ""
	}
	targetHeading := strings.TrimSpace(extractTaggedDocumentEditValue(req.QuotedContext, "target_section_heading"))
	if targetHeading == "" {
		targetHeading = strings.TrimSpace(resolveDocumentEditPatchHeading(req))
	}
	if targetHeading == "" {
		return ""
	}
	if matches := documentEditNumberedHeadingRE.FindStringSubmatch(targetHeading); len(matches) == 4 {
		headingMarks := matches[1]
		numberPrefix := matches[2]
		childLevel := len(headingMarks) + 1
		if childLevel > 6 {
			childLevel = 6
		}
		childMarks := strings.Repeat("#", childLevel)
		return strings.TrimSpace(fmt.Sprintf("- 目标标题已经存在于基线文档中，不要重复输出该标题：%s\n- 如果需要在该标题下继续展开子标题，子标题必须比目标标题低一级，使用 %s。\n- 目标标题编号分支为 %s，后续子标题必须严格延续该分支，例如：%s %s.1 子标题、%s %s.2 子标题。\n- 禁止在该标题下重新从其他编号开始，例如不要输出 ### 7.1、### 7.2，或与目标标题同级的其他编号标题。", targetHeading, childMarks, numberPrefix, childMarks, numberPrefix, childMarks, numberPrefix))
	}
	if strings.HasPrefix(targetHeading, "#") {
		childLevel := strings.Count(strings.SplitN(targetHeading, " ", 2)[0], "#") + 1
		if childLevel > 6 {
			childLevel = 6
		}
		childMarks := strings.Repeat("#", childLevel)
		return strings.TrimSpace(fmt.Sprintf("- 目标标题已经存在于基线文档中，不要重复输出该标题：%s\n- 如果需要继续展开子标题，子标题必须比目标标题低一级，使用 %s，禁止输出与目标标题同级的兄弟标题。", targetHeading, childMarks))
	}
	return ""
}

func buildDeterministicDocumentEditPatch(req *types.QARequest) (string, bool) {
	if req == nil || req.DocumentIntent != types.ChatDocumentIntentRevise || req.DocumentOutputMode == types.ChatDocumentOutputModeFull {
		return "", false
	}
	quotedContext := strings.TrimSpace(req.QuotedContext)
	if quotedContext == "" ||
		extractDocumentEditMetadataValue(quotedContext, "document_edit_operation") != "move_after_heading_to_section" ||
		extractDocumentEditMetadataValue(quotedContext, "document_merge_mode") != types.ChatDocumentMergeModeAppendToSection {
		return "", false
	}
	sourceSection := extractTaggedDocumentEditValue(quotedContext, "source_section")
	destinationHeading := extractTaggedDocumentEditValue(quotedContext, "destination_section_heading")
	if destinationHeading == "" {
		destinationHeading = extractDocumentEditMetadataValue(quotedContext, "target_heading")
	}
	destinationSection := extractTaggedDocumentEditValue(quotedContext, "destination_section")
	if sourceSection == "" || destinationHeading == "" {
		return "", false
	}
	if documentSectionContains(sourceSection, destinationSection) {
		return "", false
	}
	return buildChatDocumentAppendPatch(destinationHeading, sourceSection), true
}

func documentSectionContains(section string, container string) bool {
	section = strings.TrimSpace(section)
	container = strings.TrimSpace(container)
	if section == "" || container == "" || len([]rune(section)) > len([]rune(container)) {
		return false
	}
	return strings.Contains(container, section)
}

func extractTaggedDocumentEditValue(content string, tag string) string {
	tag = strings.TrimSpace(tag)
	if content == "" || tag == "" {
		return ""
	}
	startMarker := "<" + tag + ">"
	endMarker := "</" + tag + ">"
	start := strings.Index(content, startMarker)
	if start < 0 {
		return ""
	}
	start += len(startMarker)
	end := strings.Index(content[start:], endMarker)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(content[start : start+end])
}

func extractDocumentEditMetadataValue(content string, key string) string {
	key = strings.TrimSpace(key)
	if content == "" || key == "" {
		return ""
	}
	prefix := "- " + key + ":"
	for _, line := range strings.Split(content, "\n") {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmedLine, prefix))
		}
	}
	return ""
}

func emitDedicatedDocumentEditPatch(ctx context.Context, req *types.QARequest, eventBus *event.EventBus, patch string, finishReason string, startTime time.Time) error {
	if req == nil || req.Session == nil || eventBus == nil || strings.TrimSpace(patch) == "" {
		return errors.New("deterministic document edit patch is incomplete")
	}
	if strings.TrimSpace(finishReason) == "" {
		finishReason = "deterministic_patch"
	}
	eventID := generateEventID("document-edit-deterministic")
	if err := eventBus.Emit(ctx, event.Event{
		ID:        eventID,
		Type:      event.EventAgentFinalAnswer,
		SessionID: req.Session.ID,
		Data: dedicatedDocumentEditEventData(
			patch,
			true,
			types.MessageCompletionStatusCompleted,
			finishReason,
			"",
			true,
			true,
		),
	}); err != nil {
		return err
	}
	return eventBus.Emit(ctx, event.Event{
		ID:        generateEventID("complete"),
		Type:      event.EventAgentComplete,
		SessionID: req.Session.ID,
		Data: event.AgentCompleteData{
			SessionID:        req.Session.ID,
			MessageID:        req.AssistantMessageID,
			FinalAnswer:      patch,
			CompletionStatus: types.MessageCompletionStatusCompleted,
			FinishReason:     finishReason,
			AllowIndexing:    true,
			AllowComplete:    true,
			Extra:            buildDocumentEditPatchExtra(req, patch, true),
			TotalDurationMs:  time.Since(startTime).Milliseconds(),
		},
	})
}

func resolveDocumentEditPatchHeading(req *types.QARequest) string {
	if req == nil {
		return ""
	}
	quotedContext := strings.TrimSpace(req.QuotedContext)
	for _, candidate := range []string{
		strings.TrimSpace(req.DocumentTargetHeading),
		extractDocumentEditMetadataValue(quotedContext, "resolved_target_heading"),
		extractDocumentEditMetadataValue(quotedContext, "target_heading"),
		extractTaggedDocumentEditValue(quotedContext, "destination_section_heading"),
	} {
		if strings.TrimSpace(candidate) != "" {
			return strings.TrimSpace(candidate)
		}
	}
	return ""
}

func buildDocumentEditPatchExtra(req *types.QARequest, patch string, deterministic bool) map[string]interface{} {
	trimmedPatch := strings.TrimSpace(patch)
	if trimmedPatch == "" {
		return nil
	}
	operations, detected, valid := parseChatDocumentPatch(trimmedPatch)
	fallbackHeading := resolveDocumentEditPatchHeading(req)
	headings := make([]string, 0, len(operations))
	for _, operation := range operations {
		if strings.TrimSpace(operation.Heading) != "" {
			headings = append(headings, strings.TrimSpace(operation.Heading))
		}
	}
	headings = uniqueNonEmptyStrings(headings)
	resolvedHeading := ""
	if fallbackHeading != "" {
		resolvedHeading = fallbackHeading
	} else if len(headings) == 1 {
		resolvedHeading = headings[0]
	}
	mergeConfidence := "low"
	if deterministic {
		mergeConfidence = "high"
	} else if detected {
		if valid && resolvedHeading != "" {
			mergeConfidence = "high"
		} else {
			mergeConfidence = "medium"
		}
	}
	metadata := map[string]interface{}{
		"structured":            detected,
		"deterministic":         deterministic,
		"merge_confidence":      mergeConfidence,
		"patch_operation_count": len(operations),
	}
	if resolvedHeading != "" {
		metadata["resolved_heading"] = resolvedHeading
	}
	return map[string]interface{}{"document_patch_metadata": metadata}
}

func mergeDocumentEditCompletionExtra(base map[string]interface{}, extra map[string]interface{}) map[string]interface{} {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	merged := make(map[string]interface{}, len(base)+len(extra))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range extra {
		merged[key] = value
	}
	return merged
}

func buildDocumentEditKnowledgeQueries(req *types.QARequest) []string {
	if req == nil {
		return nil
	}
	heading := strings.TrimSpace(resolveDocumentEditPatchHeading(req))
	queries := []string{}
	if query := strings.TrimSpace(req.Query); query != "" {
		queries = append(queries, strings.TrimSpace(strings.Join([]string{query, heading}, " ")))
	}
	if req.BaseArtifact != nil {
		if title := strings.TrimSpace(req.BaseArtifact.Title); title != "" {
			queries = append(queries, strings.TrimSpace(strings.Join([]string{title, heading}, " ")))
		}
	}
	if heading != "" {
		queries = append(queries, heading)
	}
	return uniqueNonEmptyStrings(queries)
}

func emitDedicatedDocumentEditLocalKnowledgeBlocked(ctx context.Context, req *types.QARequest, eventBus *event.EventBus, message string, extra map[string]interface{}, startTime time.Time) error {
	if req == nil || req.Session == nil || eventBus == nil {
		return errors.New("document edit local knowledge blocked event is incomplete")
	}
	if strings.TrimSpace(message) == "" {
		message = "本地知识库未检索到足够证据，当前无法安全扩写目标章节，请补充资料后重试。"
	}
	if err := eventBus.Emit(ctx, event.Event{
		ID:        generateEventID("document-edit-blocked"),
		Type:      event.EventAgentFinalAnswer,
		SessionID: req.Session.ID,
		Data: dedicatedDocumentEditEventData(
			message,
			true,
			types.MessageCompletionStatusPartial,
			"local_knowledge_not_found",
			"local_knowledge_not_found",
			false,
			true,
		),
	}); err != nil {
		return err
	}
	return eventBus.Emit(ctx, event.Event{
		ID:        generateEventID("complete"),
		Type:      event.EventAgentComplete,
		SessionID: req.Session.ID,
		Data: event.AgentCompleteData{
			SessionID:        req.Session.ID,
			MessageID:        req.AssistantMessageID,
			FinalAnswer:      message,
			CompletionStatus: types.MessageCompletionStatusPartial,
			FinishReason:     "local_knowledge_not_found",
			FailureReason:    "local_knowledge_not_found",
			AllowIndexing:    false,
			AllowComplete:    true,
			Extra:            extra,
			TotalDurationMs:  time.Since(startTime).Milliseconds(),
		},
	})
}

var (
	dedicatedDocumentEditProgressHeartbeatInterval = 8 * time.Second
	dedicatedDocumentEditFirstContentTimeout       = 30 * time.Second
	dedicatedFullDocumentSectionLimitPerRun        = 3
	fullDocumentSectionEvidenceLimit               = 8
	fullDocumentSectionMinCompletionTokens         = 4096
	fullDocumentEvidenceChunkRuneLimit             = 1800
)

type DocumentGenerationBudget struct {
	Source                          string
	ModelID                         string
	Provider                        string
	OutlineMaxCompletionTokens      int
	SectionMaxCompletionTokens      int
	ContinuationMaxCompletionTokens int
	OutlineEvidenceTopK             int
	SectionEvidenceTopK             int
	ContinuationEvidenceTopK        int
	SectionCallTimeoutSeconds       int
	ProgressHeartbeatSeconds        int
	ContextWindowTokens             int
	MaxOutputTokens                 int
	NegotiationReason               string
	SupportsStreaming               *bool
	SupportsThinkingControl         *bool
	DefaultThinkingEnabled          *bool
}

type ModelCapability struct {
	ModelID                 string
	Provider                string
	ContextWindowTokens     int
	MaxOutputTokens         int
	SupportsStreaming       *bool
	SupportsThinkingControl *bool
	DefaultThinkingEnabled  *bool
	RecommendedTimeoutSec   int
}

type DocumentProfile struct {
	Goal                 string
	OutputMode           string
	ExpectedSectionCount int
	KnowledgeGrounded    bool
	EvidenceScopeKBCount int
	AutoContinue         bool
}

type documentGenerationRuntimeSectionFeedback struct {
	Section             string   `json:"section,omitempty"`
	EvidenceCount       int      `json:"evidence_count"`
	FirstTokenLatencyMs int64    `json:"first_token_latency_ms"`
	DurationMs          int64    `json:"duration_ms"`
	OutputRuneCount     int      `json:"output_rune_count"`
	OutputTokenEstimate int      `json:"output_token_estimate"`
	CompletionStatus    string   `json:"completion_status,omitempty"`
	FinishReason        string   `json:"finish_reason,omitempty"`
	FailureReason       string   `json:"failure_reason,omitempty"`
	BudgetAdjusted      bool     `json:"budget_adjusted,omitempty"`
	BudgetAdjustReasons []string `json:"budget_adjust_reasons,omitempty"`
}

type documentGenerationRuntimeFeedback struct {
	Sections                      []documentGenerationRuntimeSectionFeedback `json:"sections,omitempty"`
	SectionCount                  int                                        `json:"section_count"`
	LengthStopCount               int                                        `json:"length_stop_count"`
	TimeoutCount                  int                                        `json:"timeout_count"`
	LowEvidenceCount              int                                        `json:"low_evidence_count"`
	ShortSectionCount             int                                        `json:"short_section_count"`
	SlowFirstTokenCount           int                                        `json:"slow_first_token_count"`
	AverageFirstTokenLatencyMs    int64                                      `json:"average_first_token_latency_ms"`
	AverageSectionDurationMs      int64                                      `json:"average_section_duration_ms"`
	AverageOutputTokenEstimate    int                                        `json:"average_output_token_estimate"`
	RecommendedSectionLimitPerRun int                                        `json:"recommended_section_limit_per_run,omitempty"`
	BudgetAdjusted                bool                                       `json:"budget_adjusted"`
	AdjustmentReasons             []string                                   `json:"adjustment_reasons,omitempty"`
	TaskKind                      string                                     `json:"task_kind,omitempty"`
	ActiveArtifactID              string                                     `json:"active_artifact_id,omitempty"`
	LastCompletionStatus          string                                     `json:"last_completion_status,omitempty"`
	LastFinishReason              string                                     `json:"last_finish_reason,omitempty"`
	LastFailureReason             string                                     `json:"last_failure_reason,omitempty"`
	LastDocumentStatus            string                                     `json:"last_document_generation_status,omitempty"`
	LastAutoContinueReason        string                                     `json:"last_auto_continue_reason,omitempty"`
	AutoContinueRound             int                                        `json:"auto_continue_round,omitempty"`
	MaxAutoContinueRounds         int                                        `json:"max_auto_continue_rounds,omitempty"`
	MinGrowthChars                int                                        `json:"min_growth_chars,omitempty"`
	MaxLowGrowthRounds            int                                        `json:"max_low_growth_rounds,omitempty"`
	LastSnapshotCharCount         int                                        `json:"last_snapshot_char_count,omitempty"`
	LowGrowthRounds               int                                        `json:"low_growth_rounds,omitempty"`
	CompletedCount                int                                        `json:"completed_count,omitempty"`
	RemainingCount                int                                        `json:"remaining_count,omitempty"`
	NextSourceChunkStartSeq       int                                        `json:"next_source_chunk_start_seq,omitempty"`
	NextSourceChunkEndSeq         int                                        `json:"next_source_chunk_end_seq,omitempty"`
	NextSection                   string                                     `json:"next_section,omitempty"`
}

type documentBudgetNegotiationResponse struct {
	OutlineMaxCompletionTokens      int    `json:"outline_max_completion_tokens"`
	SectionMaxCompletionTokens      int    `json:"section_max_completion_tokens"`
	ContinuationMaxCompletionTokens int    `json:"continuation_max_completion_tokens"`
	OutlineEvidenceTopK             int    `json:"outline_evidence_top_k"`
	SectionEvidenceTopK             int    `json:"section_evidence_top_k"`
	ContinuationEvidenceTopK        int    `json:"continuation_evidence_top_k"`
	SectionCallTimeoutSeconds       int    `json:"section_call_timeout_seconds"`
	Reason                          string `json:"reason"`
}

type dedicatedFullDocumentSubsection struct {
	Number string `json:"number,omitempty"`
	Title  string `json:"title,omitempty"`
}

type dedicatedFullDocumentSection struct {
	Number      int                               `json:"number,omitempty"`
	Title       string                            `json:"title,omitempty"`
	Heading     string                            `json:"heading,omitempty"`
	Subsections []dedicatedFullDocumentSubsection `json:"subsections,omitempty"`
}

type dedicatedFullDocumentOutline struct {
	Title    string                         `json:"title"`
	Sections []dedicatedFullDocumentSection `json:"sections,omitempty"`
}

var (
	dedicatedFullDocumentChapterHeadingRE = regexp.MustCompile(`^第\s*(\d+)\s*章\s*(.+)$`)
	dedicatedFullDocumentNumericHeadingRE = regexp.MustCompile(`^(\d+)[.、]\s*(.+)$`)
	dedicatedFullDocumentSubsectionRE     = regexp.MustCompile(`^(\d+(?:\.\d+)+)\s+(.+)$`)
)

func newDedicatedFullDocumentOutlineFromStrings(title string, sections []string) dedicatedFullDocumentOutline {
	outline := dedicatedFullDocumentOutline{Title: strings.TrimSpace(title), Sections: make([]dedicatedFullDocumentSection, 0, len(sections))}
	for index, section := range sections {
		outline.Sections = append(outline.Sections, dedicatedFullDocumentSection{Number: index + 1, Title: strings.TrimSpace(section)})
	}
	return normalizeDedicatedFullDocumentOutline(outline)
}

func parseDedicatedFullDocumentSectionNumberAndTitle(raw string, fallbackNumber int) (int, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallbackNumber, ""
	}
	if matches := dedicatedFullDocumentChapterHeadingRE.FindStringSubmatch(trimmed); len(matches) == 3 {
		number, err := strconv.Atoi(strings.TrimSpace(matches[1]))
		if err == nil && number > 0 {
			return number, strings.TrimSpace(matches[2])
		}
	}
	if matches := dedicatedFullDocumentNumericHeadingRE.FindStringSubmatch(trimmed); len(matches) == 3 {
		number, err := strconv.Atoi(strings.TrimSpace(matches[1]))
		if err == nil && number > 0 {
			return number, strings.TrimSpace(matches[2])
		}
	}
	return fallbackNumber, trimmed
}

func normalizeDedicatedFullDocumentSubsections(subsections []dedicatedFullDocumentSubsection, chapterNumber int) []dedicatedFullDocumentSubsection {
	if len(subsections) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(subsections))
	normalized := make([]dedicatedFullDocumentSubsection, 0, len(subsections))
	for _, raw := range subsections {
		number := strings.TrimSpace(raw.Number)
		title := strings.TrimSpace(raw.Title)
		if title == "" && number != "" {
			if matches := dedicatedFullDocumentSubsectionRE.FindStringSubmatch(number); len(matches) == 3 {
				number = strings.TrimSpace(matches[1])
				title = strings.TrimSpace(matches[2])
			}
		}
		if title == "" {
			continue
		}
		key := firstNonEmptyString(title, number)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if chapterNumber > 0 {
			number = fmt.Sprintf("%d.%d", chapterNumber, len(normalized)+1)
		} else if number == "" {
			number = strconv.Itoa(len(normalized) + 1)
		}
		normalized = append(normalized, dedicatedFullDocumentSubsection{Number: number, Title: title})
	}
	return normalized
}

func normalizeDedicatedFullDocumentSection(section dedicatedFullDocumentSection, fallbackNumber int) (dedicatedFullDocumentSection, bool) {
	number := section.Number
	if number <= 0 {
		number = fallbackNumber
	}
	title := strings.TrimSpace(section.Title)
	heading := strings.TrimSpace(section.Heading)
	if title == "" && heading != "" {
		parsedNumber, parsedTitle := parseDedicatedFullDocumentSectionNumberAndTitle(heading, number)
		if parsedNumber > 0 {
			number = parsedNumber
		}
		title = parsedTitle
	}
	if title == "" {
		return dedicatedFullDocumentSection{}, false
	}
	if number <= 0 {
		number = fallbackNumber
	}
	if number > 0 {
		heading = fmt.Sprintf("第%d章 %s", number, title)
	} else {
		heading = title
	}
	return dedicatedFullDocumentSection{
		Number:      number,
		Title:       title,
		Heading:     heading,
		Subsections: normalizeDedicatedFullDocumentSubsections(section.Subsections, number),
	}, true
}

func normalizeDedicatedFullDocumentOutline(outline dedicatedFullDocumentOutline) dedicatedFullDocumentOutline {
	outline.Title = strings.TrimSpace(outline.Title)
	if len(outline.Sections) == 0 {
		return outline
	}
	seen := make(map[string]struct{}, len(outline.Sections))
	normalized := make([]dedicatedFullDocumentSection, 0, len(outline.Sections))
	for index, rawSection := range outline.Sections {
		section, ok := normalizeDedicatedFullDocumentSection(rawSection, index+1)
		if !ok {
			continue
		}
		key := firstNonEmptyString(strings.TrimSpace(section.Title), strings.TrimSpace(section.Heading))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		section.Number = len(normalized) + 1
		section.Heading = fmt.Sprintf("第%d章 %s", section.Number, strings.TrimSpace(section.Title))
		section.Subsections = normalizeDedicatedFullDocumentSubsections(section.Subsections, section.Number)
		normalized = append(normalized, section)
	}
	outline.Sections = normalized
	return outline
}

func dedicatedFullDocumentSectionTitles(outline dedicatedFullDocumentOutline) []string {
	if len(outline.Sections) == 0 {
		return nil
	}
	sections := make([]string, 0, len(outline.Sections))
	for _, section := range outline.Sections {
		title := strings.TrimSpace(section.Title)
		if title == "" {
			continue
		}
		sections = append(sections, title)
	}
	return sections
}

func dedicatedFullDocumentSectionHeadings(outline dedicatedFullDocumentOutline) []string {
	if len(outline.Sections) == 0 {
		return nil
	}
	sections := make([]string, 0, len(outline.Sections))
	for _, section := range outline.Sections {
		heading := strings.TrimSpace(section.Heading)
		if heading == "" {
			continue
		}
		sections = append(sections, heading)
	}
	return sections
}

func findDedicatedFullDocumentSection(outline dedicatedFullDocumentOutline, currentSection string) (dedicatedFullDocumentSection, bool) {
	outline = normalizeDedicatedFullDocumentOutline(outline)
	trimmed := strings.TrimSpace(currentSection)
	if trimmed == "" {
		return dedicatedFullDocumentSection{}, false
	}
	_, parsedTitle := parseDedicatedFullDocumentSectionNumberAndTitle(trimmed, 0)
	for _, section := range outline.Sections {
		if strings.TrimSpace(section.Title) == trimmed || strings.TrimSpace(section.Heading) == trimmed {
			return section, true
		}
		if parsedTitle != "" && strings.TrimSpace(section.Title) == parsedTitle {
			return section, true
		}
	}
	fallbackNumber, fallbackTitle := parseDedicatedFullDocumentSectionNumberAndTitle(trimmed, len(outline.Sections)+1)
	fallback := dedicatedFullDocumentSection{Number: fallbackNumber, Title: firstNonEmptyString(strings.TrimSpace(fallbackTitle), trimmed), Heading: trimmed}
	section, ok := normalizeDedicatedFullDocumentSection(fallback, fallbackNumber)
	if !ok {
		return dedicatedFullDocumentSection{}, false
	}
	return section, true
}

func formatDedicatedFullDocumentSectionHeadingMarkdown(section dedicatedFullDocumentSection) string {
	section, ok := normalizeDedicatedFullDocumentSection(section, max(section.Number, 1))
	if !ok {
		return ""
	}
	return "## " + strings.TrimSpace(section.Heading)
}

func buildDedicatedFullDocumentSectionContractPrompt(section dedicatedFullDocumentSection) string {
	section, ok := normalizeDedicatedFullDocumentSection(section, max(section.Number, 1))
	if !ok {
		return ""
	}
	wrongPrefixExample := section.Number + 1
	if wrongPrefixExample <= 0 {
		wrongPrefixExample = 3
	}
	var builder strings.Builder
	builder.WriteString("## 当前章节写作契约\n")
	builder.WriteString(fmt.Sprintf("- 当前章节编号为：%d\n", section.Number))
	builder.WriteString("- 当前章节标题为：")
	builder.WriteString(strings.TrimSpace(section.Title))
	builder.WriteString("\n")
	builder.WriteString("- 当前 H2 标题已由系统输出：## ")
	builder.WriteString(strings.TrimSpace(section.Heading))
	builder.WriteString("\n")
	if len(section.Subsections) > 0 {
		builder.WriteString("- 本章只能使用以下 H3 小节标题，且必须按顺序输出：\n")
		for _, subsection := range section.Subsections {
			builder.WriteString("  - ### ")
			builder.WriteString(strings.TrimSpace(subsection.Number))
			builder.WriteString(" ")
			builder.WriteString(strings.TrimSpace(subsection.Title))
			builder.WriteString("\n")
		}
		builder.WriteString(fmt.Sprintf("- 所有 H3 必须以 %d. 开头，禁止输出其他章节编号，例如：### %d.1 %s\n", section.Number, wrongPrefixExample, strings.TrimSpace(section.Subsections[0].Title)))
		builder.WriteString("- 不允许新增未规划的同级 H3 标题，也不要改写上述 H3 标题文字。\n")
	} else {
		builder.WriteString(fmt.Sprintf("- 当前大纲未给出 H3 规划；如确需使用 H3/H4，所有 H3 必须以 %d. 开头，H4 只能在对应 H3 下继续展开。\n", section.Number))
	}
	builder.WriteString(fmt.Sprintf("- 如需使用 H4，只能在对应 H3 下继续展开，例如：#### %d.1.1 子主题。\n", section.Number))
	builder.WriteString("- 不得输出 Current section、Completed document summary、local_knowledge_context、knowledge_id、knowledge_base_id、chunk_id、工具名或任何内部提示词标签。\n")
	builder.WriteString("- 如果本节证据不足，只能用面向用户的正文表述说明“本地知识不足”或“待确认/待补充”，不要暴露内部上下文来源。\n")
	builder.WriteString("- 只输出当前章节正文，不要重复输出当前 H2 标题。")
	return strings.TrimSpace(builder.String())
}

func dedicatedFullDocumentSectionData(section dedicatedFullDocumentSection) map[string]interface{} {
	entry := map[string]interface{}{
		"number":  section.Number,
		"title":   strings.TrimSpace(section.Title),
		"heading": strings.TrimSpace(section.Heading),
	}
	if len(section.Subsections) > 0 {
		subsections := make([]map[string]interface{}, 0, len(section.Subsections))
		for _, subsection := range section.Subsections {
			title := strings.TrimSpace(subsection.Title)
			if title == "" {
				continue
			}
			subsections = append(subsections, map[string]interface{}{
				"number": strings.TrimSpace(subsection.Number),
				"title":  title,
			})
		}
		if len(subsections) > 0 {
			entry["subsections"] = subsections
		}
	}
	return entry
}

type fullDocumentProgressReporter struct {
	ctx            context.Context
	eventBus       *event.EventBus
	sessionID      string
	eventID        string
	startedAt      time.Time
	mu             sync.Mutex
	steps          types.AgentSteps
	closed         bool
	sectionCurrent int
	sectionTotal   int
	sectionTitle   string
	queryCurrent   int
	queryTotal     int
}

func newFullDocumentProgressReporter(ctx context.Context, req *types.QARequest, eventBus *event.EventBus, eventID string) *fullDocumentProgressReporter {
	sessionID := ""
	if req != nil && req.Session != nil {
		sessionID = req.Session.ID
	}
	return &fullDocumentProgressReporter{ctx: ctx, eventBus: eventBus, sessionID: sessionID, eventID: eventID, startedAt: time.Now(), steps: make(types.AgentSteps, 0, 8)}
}

func (r *fullDocumentProgressReporter) Update(content string) {
	r.UpdateStage("", content)
}

func (r *fullDocumentProgressReporter) SetSectionProgress(current int, total int, title string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if current <= 0 || total <= 0 {
		r.sectionCurrent = 0
		r.sectionTotal = 0
		r.sectionTitle = ""
		return
	}
	r.sectionCurrent = current
	r.sectionTotal = total
	r.sectionTitle = strings.TrimSpace(title)
}

func (r *fullDocumentProgressReporter) ClearSectionProgress() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sectionCurrent = 0
	r.sectionTotal = 0
	r.sectionTitle = ""
	r.queryCurrent = 0
	r.queryTotal = 0
}

func (r *fullDocumentProgressReporter) SetQueryProgress(current int, total int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if current <= 0 || total <= 0 {
		r.queryCurrent = 0
		r.queryTotal = 0
		return
	}
	r.queryCurrent = current
	r.queryTotal = total
}

func (r *fullDocumentProgressReporter) ClearQueryProgress() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queryCurrent = 0
	r.queryTotal = 0
}

func isFullDocumentProgressHeartbeat(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	return strings.Contains(trimmed, "已等待")
}

func buildFullDocumentStructuredProgressLabel(sectionCurrent int, sectionTotal int, sectionTitle string, queryCurrent int, queryTotal int) string {
	if sectionCurrent <= 0 || sectionTotal <= 0 {
		return ""
	}
	label := fmt.Sprintf("第 %d/%d 章", sectionCurrent, sectionTotal)
	if strings.TrimSpace(sectionTitle) != "" {
		label += "：" + strings.TrimSpace(sectionTitle)
	}
	if queryCurrent > 0 && queryTotal > 0 {
		label += fmt.Sprintf(" · 检索 %d/%d", queryCurrent, queryTotal)
	}
	return label
}

func (r *fullDocumentProgressReporter) recordStep(stage string, content string) {
	if r == nil {
		return
	}
	trimmedContent := strings.TrimSpace(content)
	if trimmedContent == "" {
		return
	}
	trimmedStage := strings.TrimSpace(stage)
	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	if len(r.steps) > 0 {
		last := &r.steps[len(r.steps)-1]
		if strings.TrimSpace(last.Thought) == trimmedContent && strings.TrimSpace(last.Stage) == trimmedStage {
			return
		}
		if trimmedStage != "" && strings.TrimSpace(last.Stage) == trimmedStage && isFullDocumentProgressHeartbeat(trimmedContent) {
			last.Thought = trimmedContent
			if !last.Timestamp.IsZero() {
				last.Duration = now.Sub(last.Timestamp).Milliseconds()
			}
			return
		}
	}
	step := types.AgentStep{
		Iteration: len(r.steps),
		Thought:   trimmedContent,
		Stage:     trimmedStage,
		Timestamp: now,
	}
	if len(r.steps) == 0 {
		step.Duration = now.Sub(r.startedAt).Milliseconds()
	} else {
		step.Duration = now.Sub(r.steps[len(r.steps)-1].Timestamp).Milliseconds()
	}
	r.steps = append(r.steps, step)
}

func (r *fullDocumentProgressReporter) AgentSteps() types.AgentSteps {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.steps) == 0 {
		return nil
	}
	return append(types.AgentSteps(nil), r.steps...)
}

func (r *fullDocumentProgressReporter) UpdateStage(stage string, content string) {
	trimmedContent := strings.TrimSpace(content)
	trimmedStage := strings.TrimSpace(stage)
	r.recordStep(trimmedStage, trimmedContent)
	if r == nil || r.eventBus == nil || strings.TrimSpace(r.sessionID) == "" {
		return
	}
	sectionCurrent := 0
	sectionTotal := 0
	sectionTitle := ""
	queryCurrent := 0
	queryTotal := 0
	progressLabel := ""
	r.mu.Lock()
	sectionCurrent = r.sectionCurrent
	sectionTotal = r.sectionTotal
	sectionTitle = strings.TrimSpace(r.sectionTitle)
	queryCurrent = r.queryCurrent
	queryTotal = r.queryTotal
	progressLabel = buildFullDocumentStructuredProgressLabel(sectionCurrent, sectionTotal, sectionTitle, queryCurrent, queryTotal)
	r.mu.Unlock()
	if err := r.eventBus.Emit(r.ctx, event.Event{
		ID:        r.eventID,
		Type:      event.EventAgentThought,
		SessionID: r.sessionID,
		Data: event.AgentThoughtData{
			Content:        trimmedContent,
			Iteration:      0,
			Replace:        true,
			Synthetic:      true,
			Stage:          trimmedStage,
			SectionCurrent: sectionCurrent,
			SectionTotal:   sectionTotal,
			SectionTitle:   sectionTitle,
			QueryCurrent:   queryCurrent,
			QueryTotal:     queryTotal,
			ProgressLabel:  progressLabel,
		},
	}); err != nil {
		logger.Errorf(r.ctx, "Failed to emit full document progress thought: %v", err)
	}
}

func (r *fullDocumentProgressReporter) PublishOutline(outline dedicatedFullDocumentOutline) {
	if r == nil || r.eventBus == nil || strings.TrimSpace(r.sessionID) == "" {
		return
	}
	outlineMarkdown := formatFullDocumentOutlineMarkdown(outline)
	if strings.TrimSpace(outlineMarkdown) == "" {
		return
	}
	if err := r.eventBus.Emit(r.ctx, event.Event{
		ID:        generateEventID("document-outline"),
		Type:      event.EventAgentThought,
		SessionID: r.sessionID,
		Data: event.AgentThoughtData{
			Content:   outlineMarkdown,
			Iteration: 0,
			Done:      true,
			Replace:   false,
			Synthetic: true,
			Stage:     "planning",
			Outline:   dedicatedFullDocumentOutlineData(outline),
		},
	}); err != nil {
		logger.Errorf(r.ctx, "Failed to emit full document outline thought: %v", err)
	}
}

func (r *fullDocumentProgressReporter) Close() {
	if r == nil || r.eventBus == nil || strings.TrimSpace(r.sessionID) == "" {
		return
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	r.mu.Unlock()
	if err := r.eventBus.Emit(r.ctx, event.Event{
		ID:        r.eventID,
		Type:      event.EventAgentThought,
		SessionID: r.sessionID,
		Data:      event.AgentThoughtData{Done: true, Replace: true, Synthetic: true},
	}); err != nil {
		logger.Errorf(r.ctx, "Failed to close full document progress thought: %v", err)
	}
}

type fullDocumentEvidenceProgress func(current int, total int, query string)

type fullDocumentOutlineChatResult struct {
	response *types.ChatResponse
	err      error
}

type fullDocumentThinkingEmitter func(content string, done bool)

type fullDocumentSectionStreamResult struct {
	completionStatus        string
	finishReason            string
	failureReason           string
	documentGenerationState string
	sectionDone             bool
	firstTokenLatencyMs     int64
	durationMs              int64
	outputRuneCount         int
	outputTokenEstimate     int
}

func chatFullDocumentOutlineWithProgress(
	ctx context.Context,
	chatModel chat.Chat,
	messages []chat.Message,
	options *chat.ChatOptions,
	progress *fullDocumentProgressReporter,
	progressText string,
	onThinking fullDocumentThinkingEmitter,
) (*types.ChatResponse, error) {
	stream, err := chatModel.ChatStream(ctx, messages, options)
	if err != nil {
		return nil, err
	}
	heartbeatInterval := dedicatedDocumentEditProgressHeartbeatInterval
	if heartbeatInterval <= 0 {
		heartbeatInterval = 8 * time.Second
	}
	timer := time.NewTimer(heartbeatInterval)
	defer stopAndDrainTimer(timer)
	waited := heartbeatInterval
	var content strings.Builder
	lastTitle := ""
	lastSectionCount := 0
	modelThinkingStarted := false
	modelThinkingClosed := false
	closeModelThinking := func() {
		if modelThinkingClosed || !modelThinkingStarted || onThinking == nil {
			return
		}
		onThinking("", true)
		modelThinkingClosed = true
	}
	defer closeModelThinking()

	resetHeartbeat := func() {
		waited = heartbeatInterval
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(heartbeatInterval)
	}

	emitOutlineProgress := func() {
		outline := parseDedicatedFullDocumentOutline(content.String())
		if progress == nil {
			return
		}
		if strings.TrimSpace(outline.Title) != "" && outline.Title != lastTitle {
			lastTitle = outline.Title
			progress.UpdateStage("planning", fmt.Sprintf("正在生成大纲：已识别标题“%s”。", outline.Title))
		}
		if len(outline.Sections) > lastSectionCount {
			lastSectionCount = len(outline.Sections)
			progress.UpdateStage("planning", fmt.Sprintf("正在生成大纲：已识别 %d 个章节。", lastSectionCount))
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			if progress != nil && strings.TrimSpace(progressText) != "" {
				progress.UpdateStage("planning", fmt.Sprintf("%s，已等待 %d 秒。", strings.TrimRight(strings.TrimSpace(progressText), "。"), int(waited.Seconds())))
			}
			waited += heartbeatInterval
			timer.Reset(heartbeatInterval)
		case response, ok := <-stream:
			if !ok {
				if strings.TrimSpace(content.String()) == "" {
					return nil, errors.New("full document outline stream closed without content")
				}
				return nil, errors.New("full document outline stream closed before completion")
			}
			if response.ResponseType == types.ResponseTypeThinking {
				if strings.TrimSpace(response.Content) != "" {
					modelThinkingStarted = true
					if onThinking != nil {
						onThinking(response.Content, false)
					}
					resetHeartbeat()
				}
				if response.Done {
					closeModelThinking()
				}
				continue
			}
			if response.ResponseType == types.ResponseTypeError {
				if strings.TrimSpace(response.Content) != "" {
					return nil, errors.New(strings.TrimSpace(response.Content))
				}
				return nil, errors.New("full document outline stream failed")
			}
			if response.ResponseType != types.ResponseTypeAnswer {
				continue
			}
			if strings.TrimSpace(response.Content) != "" {
				closeModelThinking()
				content.WriteString(response.Content)
				emitOutlineProgress()
				resetHeartbeat()
			}
			if response.Done {
				return &types.ChatResponse{
					Content:      strings.TrimSpace(content.String()),
					FinishReason: firstNonEmptyString(strings.TrimSpace(response.FinishReason), "stop"),
				}, nil
			}
		}
	}
}

func fullDocumentSectionsForInitialRun(outline dedicatedFullDocumentOutline) []string {
	if len(outline.Sections) == 0 {
		return nil
	}
	return append([]string(nil), dedicatedFullDocumentSectionTitles(outline)...)
}

func formatFullDocumentOutlineMarkdown(outline dedicatedFullDocumentOutline) string {
	outline = normalizeDedicatedFullDocumentOutline(outline)
	var builder strings.Builder
	title := strings.TrimSpace(outline.Title)
	if title != "" {
		builder.WriteString("# ")
		builder.WriteString(title)
	}
	for _, section := range outline.Sections {
		trimmedSection := strings.TrimSpace(section.Heading)
		if trimmedSection == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString("## ")
		builder.WriteString(trimmedSection)
		for _, subsection := range section.Subsections {
			title := strings.TrimSpace(subsection.Title)
			if title == "" {
				continue
			}
			builder.WriteString("\n### ")
			if number := strings.TrimSpace(subsection.Number); number != "" {
				builder.WriteString(number)
				builder.WriteString(" ")
			}
			builder.WriteString(title)
		}
	}
	return strings.TrimSpace(builder.String())
}

func fallbackFullDocumentSectionMaxCompletionTokens(cfg *config.Config) int {
	maxCompletionTokens := 4096
	if cfg != nil && cfg.Conversation != nil && cfg.Conversation.Summary != nil && cfg.Conversation.Summary.MaxCompletionTokens > 0 {
		maxCompletionTokens = cfg.Conversation.Summary.MaxCompletionTokens
	}
	if maxCompletionTokens < fullDocumentSectionMinCompletionTokens {
		return fullDocumentSectionMinCompletionTokens
	}
	return maxCompletionTokens
}

func fallbackDocumentGenerationBudget(cfg *config.Config) DocumentGenerationBudget {
	sectionMaxCompletionTokens := fallbackFullDocumentSectionMaxCompletionTokens(cfg)
	return DocumentGenerationBudget{
		Source:                          "fallback",
		OutlineMaxCompletionTokens:      min(sectionMaxCompletionTokens, 1536),
		SectionMaxCompletionTokens:      sectionMaxCompletionTokens,
		ContinuationMaxCompletionTokens: sectionMaxCompletionTokens,
		OutlineEvidenceTopK:             fullDocumentSectionEvidenceLimit,
		SectionEvidenceTopK:             fullDocumentSectionEvidenceLimit,
		ContinuationEvidenceTopK:        fullDocumentSectionEvidenceLimit,
		SectionCallTimeoutSeconds:       documentGenerationDefaultLLMTimeoutSeconds,
		ProgressHeartbeatSeconds:        int(dedicatedDocumentEditProgressHeartbeatInterval / time.Second),
	}
}

func buildFullDocumentOutlinePlanningRequirements(knowledgeGrounded bool) string {
	var builder strings.Builder
	builder.WriteString("Planning requirements:\n")
	builder.WriteString("1. Return JSON only. Do not output markdown or explanatory text.\n")
	builder.WriteString("2. title must be the document H1 title, written as a clean user-facing deliverable title rather than a raw source filename. Avoid carrying over file extensions, date suffixes, language markers, or ingestion artefact names unless the user explicitly requests them.\n")
	builder.WriteString("3. If the user explicitly provides chapters, headings, numbering, audience, or document structure, preserve that intent and only normalize numbering or duplicates when necessary.\n")
	builder.WriteString("4. If the user does not provide chapters, infer the outline directly from the user goal and available context. Do not inject a fixed chapter template, hidden document classification, preferred chapter set, or assumed lifecycle structure.\n")
	builder.WriteString("5. First extract the concrete deliverable requirements from the user goal, such as target audience, execution depth, required outputs, named topics, constraints, and explicitly requested sections. The outline must visibly cover those requirements instead of only paraphrasing the title.\n")
	builder.WriteString("6. Choose the number of chapters and subsection depth according to the actual task complexity. Do not force a preset chapter count. For a detailed implementation or development guide, the outline should usually be more fine-grained than a high-level summary.\n")
	builder.WriteString("7. Each section must include number, title, and heading.\n")
	builder.WriteString("8. When the user asks for a detailed, executable, or development-guiding document, each section should normally include 2 to 4 planned subsections so later generation remains structurally constrained. Avoid empty high-level chapters unless the user explicitly asked for a compact outline.\n")
	builder.WriteString("9. subsection titles must be specific, actionable, and traceable to the user's requested scope. Do not use generic placeholders when the request already names concrete concerns.\n")
	builder.WriteString("10. number must start at 1 and increase continuously without gaps.\n")
	builder.WriteString("11. heading must equal \"第{number}章 {title}\".\n")
	builder.WriteString("12. Each subsection.number must use \"{chapter}.{index}\" format, such as \"2.3\".\n")
	if knowledgeGrounded {
		builder.WriteString("13. If local knowledge is insufficient, include evidence gaps and待补充项 as planned subsections instead of fabricating facts.\n")
	} else {
		builder.WriteString("13. Later section generation must follow these numbers and titles exactly.\n")
	}
	builder.WriteString("\nJSON schema:\n{\n  \"title\": \"string\",\n  \"sections\": [\n    {\n      \"number\": 1,\n      \"title\": \"string\",\n      \"heading\": \"第1章 string\",\n      \"subsections\": [\n        {\"number\": \"1.1\", \"title\": \"string\"}\n      ]\n    }\n  ]\n}")
	return builder.String()
}

func estimateExpectedSectionsFromArtifact(artifact *types.ChatDocumentArtifact) int {
	if artifact == nil {
		return 0
	}
	outline := extractRenderedFullDocumentOutline(strings.TrimSpace(artifact.ContentSnapshot))
	if len(outline.Sections) > 0 {
		return len(outline.Sections)
	}
	return 0
}

func extractFullDocumentOutlineFromArtifact(artifact *types.ChatDocumentArtifact) dedicatedFullDocumentOutline {
	if artifact == nil {
		return dedicatedFullDocumentOutline{}
	}
	outline := extractRenderedFullDocumentOutline(strings.TrimSpace(artifact.ContentSnapshot))
	if strings.TrimSpace(outline.Title) == "" {
		outline.Title = strings.TrimSpace(artifact.Title)
	}
	return normalizeDedicatedFullDocumentOutline(outline)
}

func buildDocumentProfile(req *types.QARequest, agentConfig *types.AgentConfig, knowledgeGrounded bool, expectedSections int) DocumentProfile {
	goal := ""
	if req != nil {
		goal = strings.TrimSpace(firstNonEmptyString(req.AutoContinueOriginalQuery, req.Query))
	}
	if expectedSections <= 0 && req != nil {
		expectedSections = estimateExpectedSectionsFromArtifact(req.BaseArtifact)
	}
	evidenceScopeKBCount := 0
	if agentConfig != nil {
		evidenceScopeKBCount = len(agentConfig.SearchTargets.GetAllKnowledgeBaseIDs())
	}
	outputMode := ""
	if req != nil {
		outputMode = req.DocumentOutputMode
	}
	return DocumentProfile{
		Goal:                 goal,
		OutputMode:           strings.TrimSpace(outputMode),
		ExpectedSectionCount: expectedSections,
		KnowledgeGrounded:    knowledgeGrounded,
		EvidenceScopeKBCount: evidenceScopeKBCount,
		AutoContinue:         req != nil && req.AutoContinue,
	}
}

func parseModelCapabilityInt(extraConfig map[string]string, keys ...string) int {
	for _, key := range keys {
		value, ok := extraConfig[key]
		if !ok {
			continue
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

func parseModelCapabilityBool(extraConfig map[string]string, keys ...string) *bool {
	for _, key := range keys {
		value, ok := extraConfig[key]
		if !ok {
			continue
		}
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		if err == nil {
			result := parsed
			return &result
		}
	}
	return nil
}

func inferDefaultModelCapability(model *types.Model) *ModelCapability {
	if model == nil {
		return nil
	}
	provider := strings.ToLower(strings.TrimSpace(firstNonEmptyString(model.Parameters.Provider, string(model.Source))))
	modelName := strings.ToLower(strings.TrimSpace(firstNonEmptyString(model.Name, model.ID)))
	switch {
	case provider == "deepseek" || strings.Contains(modelName, "deepseek"):
		streaming := true
		return &ModelCapability{
			ModelID:               strings.TrimSpace(model.ID),
			Provider:              firstNonEmptyString(strings.TrimSpace(model.Parameters.Provider), string(model.Source), "deepseek"),
			ContextWindowTokens:   64000,
			MaxOutputTokens:       8192,
			SupportsStreaming:     &streaming,
			RecommendedTimeoutSec: 180,
		}
	default:
		return nil
	}
}

func buildModelCapability(model *types.Model) *ModelCapability {
	if model == nil {
		return nil
	}
	capability := &ModelCapability{
		ModelID:                 strings.TrimSpace(model.ID),
		Provider:                firstNonEmptyString(strings.TrimSpace(model.Parameters.Provider), string(model.Source)),
		ContextWindowTokens:     model.Parameters.ContextWindowTokens,
		MaxOutputTokens:         model.Parameters.MaxOutputTokens,
		SupportsStreaming:       model.Parameters.SupportsStreaming,
		SupportsThinkingControl: model.Parameters.SupportsThinkingControl,
		DefaultThinkingEnabled:  model.Parameters.DefaultThinkingEnabled,
		RecommendedTimeoutSec:   model.Parameters.RecommendedTimeoutSec,
	}
	if capability.ContextWindowTokens <= 0 {
		capability.ContextWindowTokens = parseModelCapabilityInt(model.Parameters.ExtraConfig, "context_window_tokens", "contextWindowTokens", "max_context_tokens", "maxContextTokens", "context_window", "contextWindow", "context_length", "contextLength")
	}
	if capability.MaxOutputTokens <= 0 {
		capability.MaxOutputTokens = parseModelCapabilityInt(model.Parameters.ExtraConfig, "max_output_tokens", "maxOutputTokens", "max_completion_tokens", "maxCompletionTokens", "max_tokens", "maxTokens")
	}
	if capability.SupportsStreaming == nil {
		capability.SupportsStreaming = parseModelCapabilityBool(model.Parameters.ExtraConfig, "supports_streaming", "supportsStreaming")
	}
	if capability.SupportsThinkingControl == nil {
		capability.SupportsThinkingControl = parseModelCapabilityBool(model.Parameters.ExtraConfig, "supports_thinking_control", "supportsThinkingControl", "supports_thinking", "supportsThinking")
	}
	if capability.DefaultThinkingEnabled == nil {
		capability.DefaultThinkingEnabled = parseModelCapabilityBool(model.Parameters.ExtraConfig, "default_thinking_enabled", "defaultThinkingEnabled")
	}
	if capability.RecommendedTimeoutSec <= 0 {
		capability.RecommendedTimeoutSec = parseModelCapabilityInt(model.Parameters.ExtraConfig, "recommended_timeout_sec", "recommendedTimeoutSec", "recommended_timeout_seconds", "recommendedTimeoutSeconds")
	}
	if capability.ContextWindowTokens <= 0 && capability.MaxOutputTokens <= 0 && capability.SupportsStreaming == nil && capability.SupportsThinkingControl == nil && capability.DefaultThinkingEnabled == nil && capability.RecommendedTimeoutSec <= 0 {
		return inferDefaultModelCapability(model)
	}
	return capability
}

func applyStaticModelCapabilityToBudget(budget DocumentGenerationBudget, capability *ModelCapability) DocumentGenerationBudget {
	if capability == nil {
		return budget
	}
	adjusted := budget
	adjusted.Source = "capability"
	adjusted.ModelID = strings.TrimSpace(capability.ModelID)
	adjusted.Provider = strings.TrimSpace(capability.Provider)
	adjusted.ContextWindowTokens = capability.ContextWindowTokens
	adjusted.MaxOutputTokens = capability.MaxOutputTokens
	adjusted.SupportsStreaming = capability.SupportsStreaming
	adjusted.SupportsThinkingControl = capability.SupportsThinkingControl
	adjusted.DefaultThinkingEnabled = capability.DefaultThinkingEnabled
	if capability.RecommendedTimeoutSec > 0 {
		adjusted.SectionCallTimeoutSeconds = max(adjusted.SectionCallTimeoutSeconds, min(max(capability.RecommendedTimeoutSec, 30), 300))
	}

	if capability.ContextWindowTokens >= 65536 {
		adjusted.OutlineEvidenceTopK = max(adjusted.OutlineEvidenceTopK, 8)
		adjusted.SectionEvidenceTopK = max(adjusted.SectionEvidenceTopK, 8)
		adjusted.ContinuationEvidenceTopK = max(adjusted.ContinuationEvidenceTopK, 8)
	}
	if capability.ContextWindowTokens > 0 && capability.ContextWindowTokens <= 8192 {
		adjusted.OutlineEvidenceTopK = min(adjusted.OutlineEvidenceTopK, 4)
		adjusted.SectionEvidenceTopK = min(adjusted.SectionEvidenceTopK, 4)
		adjusted.ContinuationEvidenceTopK = min(adjusted.ContinuationEvidenceTopK, 4)
	}
	if capability.MaxOutputTokens >= 4096 && capability.ContextWindowTokens >= 16384 {
		adjusted.SectionMaxCompletionTokens = max(adjusted.SectionMaxCompletionTokens, 4096)
		adjusted.ContinuationMaxCompletionTokens = max(adjusted.ContinuationMaxCompletionTokens, 4096)
	}
	if capability.MaxOutputTokens > 0 {
		adjusted.OutlineMaxCompletionTokens = min(adjusted.OutlineMaxCompletionTokens, capability.MaxOutputTokens)
		adjusted.SectionMaxCompletionTokens = min(adjusted.SectionMaxCompletionTokens, capability.MaxOutputTokens)
		adjusted.ContinuationMaxCompletionTokens = min(adjusted.ContinuationMaxCompletionTokens, capability.MaxOutputTokens)
	}
	return adjusted
}

func shouldNegotiateDocumentGenerationBudget(req *types.QARequest, capability *ModelCapability, profile DocumentProfile) bool {
	if req == nil || capability == nil {
		return false
	}
	if capability.ContextWindowTokens <= 0 && capability.MaxOutputTokens <= 0 {
		return false
	}
	if req.DocumentOutputMode == types.ChatDocumentOutputModeFull {
		return true
	}
	if req.AutoContinue || req.DocumentIntent == types.ChatDocumentIntentContinue {
		return true
	}
	if req.DocumentIntent == types.ChatDocumentIntentRevise {
		return true
	}
	return profile.AutoContinue
}

func buildDocumentBudgetNegotiationMessages(req *types.QARequest, profile DocumentProfile, capability ModelCapability, baseline DocumentGenerationBudget) []chat.Message {
	var systemPrompt strings.Builder
	systemPrompt.WriteString("You are estimating safe generation budgets for a long markdown document. ")
	systemPrompt.WriteString("Return JSON only. Do not write the document. Do not use markdown fences. ")
	systemPrompt.WriteString("Do not include commentary or extra keys. Keep the values conservative and safe. ")
	systemPrompt.WriteString("All numeric fields must be integers. ")

	goal := strings.TrimSpace(profile.Goal)
	if goal == "" && req != nil {
		goal = strings.TrimSpace(req.Query)
	}

	userContent := fmt.Sprintf("Known constraints:\n- model_context_window_tokens: %d\n- model_max_output_tokens: %d\n- current_outline_max_completion_tokens: %d\n- current_section_max_completion_tokens: %d\n- current_continuation_max_completion_tokens: %d\n- current_outline_evidence_top_k: %d\n- current_section_evidence_top_k: %d\n- current_continuation_evidence_top_k: %d\n- current_section_call_timeout_seconds: %d\n- user_goal: %s\n- expected_sections: %d\n- local_knowledge_available: %t\n- evidence_scope_kb_count: %d\n- auto_continue: %t\n\nReturn exactly this JSON object:\n{\n  \"outline_max_completion_tokens\": number,\n  \"section_max_completion_tokens\": number,\n  \"continuation_max_completion_tokens\": number,\n  \"outline_evidence_top_k\": number,\n  \"section_evidence_top_k\": number,\n  \"continuation_evidence_top_k\": number,\n  \"section_call_timeout_seconds\": number,\n  \"reason\": string\n}", capability.ContextWindowTokens, capability.MaxOutputTokens, baseline.OutlineMaxCompletionTokens, baseline.SectionMaxCompletionTokens, baseline.ContinuationMaxCompletionTokens, baseline.OutlineEvidenceTopK, baseline.SectionEvidenceTopK, baseline.ContinuationEvidenceTopK, baseline.SectionCallTimeoutSeconds, goal, profile.ExpectedSectionCount, profile.KnowledgeGrounded, profile.EvidenceScopeKBCount, profile.AutoContinue)

	return []chat.Message{
		{Role: "system", Content: strings.TrimSpace(systemPrompt.String())},
		{Role: "user", Content: userContent},
	}
}

func parseDocumentBudgetNegotiationResponse(content string) (documentBudgetNegotiationResponse, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return documentBudgetNegotiationResponse{}, errors.New("empty_negotiation_response")
	}
	var response documentBudgetNegotiationResponse
	if err := json.Unmarshal([]byte(trimmed), &response); err != nil {
		return documentBudgetNegotiationResponse{}, err
	}
	if response.OutlineMaxCompletionTokens <= 0 || response.SectionMaxCompletionTokens <= 0 || response.ContinuationMaxCompletionTokens <= 0 || response.OutlineEvidenceTopK <= 0 || response.SectionEvidenceTopK <= 0 || response.ContinuationEvidenceTopK <= 0 || response.SectionCallTimeoutSeconds <= 0 {
		return documentBudgetNegotiationResponse{}, errors.New("invalid_negotiation_budget_fields")
	}
	return response, nil
}

func clampDocumentBudgetInt(value int, minValue int, maxValue int) int {
	if maxValue > 0 && value > maxValue {
		value = maxValue
	}
	if minValue > 0 && value < minValue {
		value = minValue
	}
	if maxValue > 0 && value > maxValue {
		return maxValue
	}
	return value
}

func maxNegotiatedOutlineTokens(baseBudget DocumentGenerationBudget) int {
	maxOutlineTokens := 2048
	if baseBudget.MaxOutputTokens > 0 {
		maxOutlineTokens = min(maxOutlineTokens, baseBudget.MaxOutputTokens)
	}
	return max(maxOutlineTokens, min(baseBudget.OutlineMaxCompletionTokens, maxOutlineTokens))
}

func maxNegotiatedSectionTokens(baseBudget DocumentGenerationBudget) int {
	maxSectionTokens := 8192
	if baseBudget.MaxOutputTokens > 0 {
		maxSectionTokens = min(maxSectionTokens, baseBudget.MaxOutputTokens)
	}
	if maxSectionTokens <= 0 {
		maxSectionTokens = max(baseBudget.SectionMaxCompletionTokens, 4096)
	}
	return maxSectionTokens
}

func validateAndClampNegotiatedDocumentBudget(baseBudget DocumentGenerationBudget, negotiated documentBudgetNegotiationResponse) DocumentGenerationBudget {
	clamped := baseBudget
	outlineMax := maxNegotiatedOutlineTokens(baseBudget)
	sectionMax := maxNegotiatedSectionTokens(baseBudget)
	continuationMax := maxNegotiatedSectionTokens(baseBudget)
	outlineMin := min(512, outlineMax)
	sectionMin := min(fullDocumentSectionMinCompletionTokens, sectionMax)
	continuationMin := min(fullDocumentSectionMinCompletionTokens, continuationMax)

	clamped.Source = "negotiated"
	clamped.OutlineMaxCompletionTokens = clampDocumentBudgetInt(negotiated.OutlineMaxCompletionTokens, outlineMin, outlineMax)
	clamped.SectionMaxCompletionTokens = clampDocumentBudgetInt(negotiated.SectionMaxCompletionTokens, sectionMin, sectionMax)
	clamped.ContinuationMaxCompletionTokens = clampDocumentBudgetInt(negotiated.ContinuationMaxCompletionTokens, continuationMin, continuationMax)
	clamped.OutlineEvidenceTopK = clampDocumentBudgetInt(negotiated.OutlineEvidenceTopK, 4, 12)
	clamped.SectionEvidenceTopK = clampDocumentBudgetInt(negotiated.SectionEvidenceTopK, 4, 12)
	clamped.ContinuationEvidenceTopK = clampDocumentBudgetInt(negotiated.ContinuationEvidenceTopK, 4, 12)
	clamped.SectionCallTimeoutSeconds = clampDocumentBudgetInt(negotiated.SectionCallTimeoutSeconds, 30, 300)
	clamped.NegotiationReason = strings.TrimSpace(negotiated.Reason)
	return clamped
}

func (s *sessionService) resolveStaticDocumentGenerationBudget(ctx context.Context, chatModel chat.Chat) (DocumentGenerationBudget, *ModelCapability) {
	budget := fallbackDocumentGenerationBudget(s.cfg)
	if s == nil || s.modelService == nil || chatModel == nil {
		return budget, nil
	}
	modelID := strings.TrimSpace(chatModel.GetModelID())
	if modelID == "" {
		return budget, nil
	}
	model, err := s.modelService.GetModelByID(ctx, modelID)
	if err != nil || model == nil {
		if err != nil {
			logger.Warnf(ctx, "[DocumentBudget] capability lookup failed for model %s: %v", modelID, err)
		}
		return budget, nil
	}
	capability := buildModelCapability(model)
	if capability == nil {
		return budget, nil
	}
	resolved := applyStaticModelCapabilityToBudget(budget, capability)
	logger.Infof(ctx, "[DocumentBudget] source=%s model=%s provider=%s outline_tokens=%d section_tokens=%d continuation_tokens=%d outline_top_k=%d section_top_k=%d continuation_top_k=%d context_window=%d max_output=%d", resolved.Source, firstNonEmptyString(resolved.ModelID, modelID), resolved.Provider, resolved.OutlineMaxCompletionTokens, resolved.SectionMaxCompletionTokens, resolved.ContinuationMaxCompletionTokens, resolved.OutlineEvidenceTopK, resolved.SectionEvidenceTopK, resolved.ContinuationEvidenceTopK, resolved.ContextWindowTokens, resolved.MaxOutputTokens)
	return resolved, capability
}

func (s *sessionService) negotiateDocumentGenerationBudget(
	ctx context.Context,
	req *types.QARequest,
	chatModel chat.Chat,
	baseBudget DocumentGenerationBudget,
	capability *ModelCapability,
	profile DocumentProfile,
	progress *fullDocumentProgressReporter,
) DocumentGenerationBudget {
	if !shouldNegotiateDocumentGenerationBudget(req, capability, profile) || chatModel == nil {
		return baseBudget
	}
	if progress != nil {
		progress.UpdateStage("planning", "正在评估模型预算。")
	}
	negotiationTimeout := documentBudgetNegotiationTimeout
	if negotiationTimeout <= 0 {
		negotiationTimeout = 4 * time.Second
	}
	negotiationCtx, cancel := context.WithTimeout(ctx, negotiationTimeout)
	defer cancel()
	thinking := false
	response, err := chatModel.Chat(negotiationCtx, buildDocumentBudgetNegotiationMessages(req, profile, *capability, baseBudget), &chat.ChatOptions{
		Temperature:         0.1,
		MaxCompletionTokens: 256,
		Thinking:            &thinking,
	})
	if err != nil {
		logger.Warnf(ctx, "[DocumentBudget] negotiation_failed model=%s err=%v fallback_section_tokens=%d", firstNonEmptyString(baseBudget.ModelID, chatModel.GetModelID()), err, baseBudget.SectionMaxCompletionTokens)
		return baseBudget
	}
	if response == nil {
		logger.Warnf(ctx, "[DocumentBudget] negotiation_failed model=%s err=nil_response fallback_section_tokens=%d", firstNonEmptyString(baseBudget.ModelID, chatModel.GetModelID()), baseBudget.SectionMaxCompletionTokens)
		return baseBudget
	}
	negotiated, err := parseDocumentBudgetNegotiationResponse(response.Content)
	if err != nil {
		logger.Warnf(ctx, "[DocumentBudget] negotiation_failed model=%s err=%v fallback_section_tokens=%d", firstNonEmptyString(baseBudget.ModelID, chatModel.GetModelID()), err, baseBudget.SectionMaxCompletionTokens)
		return baseBudget
	}
	resolved := validateAndClampNegotiatedDocumentBudget(baseBudget, negotiated)
	logger.Infof(ctx, "[DocumentBudget] source=%s model=%s provider=%s section_tokens=%d section_top_k=%d timeout=%d reason=%q", resolved.Source, firstNonEmptyString(resolved.ModelID, chatModel.GetModelID()), resolved.Provider, resolved.SectionMaxCompletionTokens, resolved.SectionEvidenceTopK, resolved.SectionCallTimeoutSeconds, resolved.NegotiationReason)
	return resolved
}

func (s *sessionService) resolveDocumentGenerationBudget(ctx context.Context, req *types.QARequest, chatModel chat.Chat, profile DocumentProfile, progress *fullDocumentProgressReporter) DocumentGenerationBudget {
	baseBudget, capability := s.resolveStaticDocumentGenerationBudget(ctx, chatModel)
	return s.negotiateDocumentGenerationBudget(ctx, req, chatModel, baseBudget, capability, profile, progress)
}

func effectiveFullDocumentSectionMaxCompletionTokens(cfg *config.Config) int {
	return fallbackDocumentGenerationBudget(cfg).SectionMaxCompletionTokens
}

func documentGenerationBudgetData(budget DocumentGenerationBudget) map[string]interface{} {
	data := map[string]interface{}{
		"source":                             strings.TrimSpace(budget.Source),
		"outline_max_completion_tokens":      budget.OutlineMaxCompletionTokens,
		"section_max_completion_tokens":      budget.SectionMaxCompletionTokens,
		"continuation_max_completion_tokens": budget.ContinuationMaxCompletionTokens,
		"outline_evidence_top_k":             budget.OutlineEvidenceTopK,
		"section_evidence_top_k":             budget.SectionEvidenceTopK,
		"continuation_evidence_top_k":        budget.ContinuationEvidenceTopK,
	}
	if strings.TrimSpace(budget.ModelID) != "" {
		data["model_id"] = strings.TrimSpace(budget.ModelID)
	}
	if strings.TrimSpace(budget.Provider) != "" {
		data["provider"] = strings.TrimSpace(budget.Provider)
	}
	if budget.ContextWindowTokens > 0 {
		data["context_window_tokens"] = budget.ContextWindowTokens
	}
	if budget.MaxOutputTokens > 0 {
		data["max_output_tokens"] = budget.MaxOutputTokens
	}
	if budget.SectionCallTimeoutSeconds > 0 {
		data["section_call_timeout_seconds"] = budget.SectionCallTimeoutSeconds
	}
	if budget.ProgressHeartbeatSeconds > 0 {
		data["progress_heartbeat_seconds"] = budget.ProgressHeartbeatSeconds
	}
	if strings.TrimSpace(budget.NegotiationReason) != "" {
		data["negotiation_reason"] = strings.TrimSpace(budget.NegotiationReason)
	}
	if budget.SupportsStreaming != nil {
		data["supports_streaming"] = *budget.SupportsStreaming
	}
	if budget.SupportsThinkingControl != nil {
		data["supports_thinking_control"] = *budget.SupportsThinkingControl
	}
	if budget.DefaultThinkingEnabled != nil {
		data["default_thinking_enabled"] = *budget.DefaultThinkingEnabled
	}
	return data
}

func withDocumentGenerationBudgetExtra(extra map[string]interface{}, budget DocumentGenerationBudget) map[string]interface{} {
	if extra == nil {
		extra = make(map[string]interface{}, 1)
	}
	extra["budget"] = documentGenerationBudgetData(budget)
	return extra
}

func consumeFullDocumentSectionStream(
	ctx context.Context,
	sectionStream <-chan types.StreamResponse,
	progress *fullDocumentProgressReporter,
	heartbeatLabel string,
	onThinking fullDocumentThinkingEmitter,
	onAnswer func(string),
) fullDocumentSectionStreamResult {
	startedAt := time.Now()
	firstTokenLatencyMs := int64(0)
	firstTokenSeen := false
	outputRuneCount := 0
	modelThinkingStarted := false
	modelThinkingClosed := false
	heartbeatInterval := dedicatedDocumentEditProgressHeartbeatInterval
	if heartbeatInterval <= 0 {
		heartbeatInterval = 8 * time.Second
	}
	timer := time.NewTimer(heartbeatInterval)
	defer stopAndDrainTimer(timer)
	waited := heartbeatInterval
	closeModelThinking := func() {
		if modelThinkingClosed || !modelThinkingStarted || onThinking == nil {
			return
		}
		onThinking("", true)
		modelThinkingClosed = true
	}
	defer closeModelThinking()
	finalize := func(result fullDocumentSectionStreamResult) fullDocumentSectionStreamResult {
		result.durationMs = time.Since(startedAt).Milliseconds()
		if firstTokenSeen {
			result.firstTokenLatencyMs = firstTokenLatencyMs
		} else if result.durationMs > 0 {
			result.firstTokenLatencyMs = result.durationMs
		}
		result.outputRuneCount = outputRuneCount
		result.outputTokenEstimate = chunker.ApproxTokenCountFromRuneLen(outputRuneCount, chunker.LangMixed)
		return result
	}

	resetHeartbeat := func() {
		waited = heartbeatInterval
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(heartbeatInterval)
	}

	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return finalize(fullDocumentSectionStreamResult{
					completionStatus:        types.MessageCompletionStatusPartial,
					finishReason:            "section_generation_timeout",
					failureReason:           classifyDocumentEditError(ctx.Err()),
					documentGenerationState: types.ChatDocumentGenerationStatusContinuing,
				})
			}
			return finalize(fullDocumentSectionStreamResult{
				completionStatus:        types.MessageCompletionStatusCancelled,
				finishReason:            "cancelled",
				failureReason:           "cancelled",
				documentGenerationState: types.ChatDocumentGenerationStatusBlocked,
			})
		case <-timer.C:
			if progress != nil && strings.TrimSpace(heartbeatLabel) != "" {
				progress.UpdateStage("generating", fmt.Sprintf("%s仍在生成中，已等待 %d 秒。", heartbeatLabel, int(waited.Seconds())))
			}
			waited += heartbeatInterval
			timer.Reset(heartbeatInterval)
		case response, ok := <-sectionStream:
			if !ok {
				return finalize(fullDocumentSectionStreamResult{
					completionStatus:        types.MessageCompletionStatusPartial,
					finishReason:            "section_generation_truncated",
					failureReason:           "section_generation_truncated",
					documentGenerationState: types.ChatDocumentGenerationStatusContinuing,
				})
			}
			if response.ResponseType == types.ResponseTypeError {
				failureReason := "section_generation_error"
				if strings.TrimSpace(response.Content) != "" {
					failureReason = classifyDocumentEditError(errors.New(response.Content))
				}
				return finalize(fullDocumentSectionStreamResult{
					completionStatus:        types.MessageCompletionStatusPartial,
					finishReason:            "section_generation_error",
					failureReason:           failureReason,
					documentGenerationState: types.ChatDocumentGenerationStatusContinuing,
				})
			}
			if response.ResponseType == types.ResponseTypeThinking {
				if strings.TrimSpace(response.Content) != "" {
					modelThinkingStarted = true
					if onThinking != nil {
						onThinking(response.Content, false)
					}
					resetHeartbeat()
				}
				if response.Done {
					closeModelThinking()
				}
				continue
			}
			if response.ResponseType != types.ResponseTypeAnswer {
				continue
			}
			if strings.TrimSpace(response.Content) != "" {
				closeModelThinking()
				if !firstTokenSeen {
					firstTokenSeen = true
					firstTokenLatencyMs = time.Since(startedAt).Milliseconds()
				}
				outputRuneCount += utf8.RuneCountInString(response.Content)
				onAnswer(response.Content)
				resetHeartbeat()
			}
			if response.Done {
				finishReason := strings.TrimSpace(response.FinishReason)
				if finishReason == "" {
					finishReason = "stop"
				}
				if finishReason == "length" {
					return finalize(fullDocumentSectionStreamResult{
						completionStatus:        types.MessageCompletionStatusPartial,
						finishReason:            finishReason,
						documentGenerationState: types.ChatDocumentGenerationStatusContinuing,
					})
				}
				return finalize(fullDocumentSectionStreamResult{
					completionStatus:        types.MessageCompletionStatusCompleted,
					finishReason:            finishReason,
					documentGenerationState: types.ChatDocumentGenerationStatusCompleted,
					sectionDone:             true,
				})
			}
		}
	}
}

func dedicatedFullDocumentOutlineData(outline dedicatedFullDocumentOutline) map[string]interface{} {
	outline = normalizeDedicatedFullDocumentOutline(outline)
	sections := make([]map[string]interface{}, 0, len(outline.Sections))
	for _, section := range outline.Sections {
		sections = append(sections, dedicatedFullDocumentSectionData(section))
	}
	return map[string]interface{}{
		"title":    strings.TrimSpace(outline.Title),
		"sections": sections,
	}
}

func fullDocumentCompletionExtra(outline dedicatedFullDocumentOutline, completedSections []string, budget DocumentGenerationBudget) map[string]interface{} {
	return withDocumentGenerationBudgetExtra(map[string]interface{}{
		"outline":            dedicatedFullDocumentOutlineData(outline),
		"completed_sections": uniqueNonEmptyStrings(completedSections),
	}, budget)
}

func fullDocumentOutlineChatOptions(budget DocumentGenerationBudget, temperature float64) *chat.ChatOptions {
	thinking := false
	return &chat.ChatOptions{
		Temperature:         temperature,
		MaxCompletionTokens: budget.OutlineMaxCompletionTokens,
		Thinking:            &thinking,
	}
}

func fullDocumentSectionChatOptions(budget DocumentGenerationBudget, temperature float64) *chat.ChatOptions {
	thinking := false
	return &chat.ChatOptions{
		Temperature:         temperature,
		MaxCompletionTokens: budget.SectionMaxCompletionTokens,
		Thinking:            &thinking,
	}
}

func fullDocumentContinuationChatOptions(budget DocumentGenerationBudget, temperature float64) *chat.ChatOptions {
	thinking := false
	return &chat.ChatOptions{
		Temperature:         temperature,
		MaxCompletionTokens: budget.ContinuationMaxCompletionTokens,
		Thinking:            &thinking,
	}
}

func emitFullDocumentModelThinking(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
	eventID string,
	stage string,
	content string,
	done bool,
	sectionCurrent int,
	sectionTotal int,
	sectionTitle string,
) {
	if req == nil || req.Session == nil || eventBus == nil || strings.TrimSpace(eventID) == "" {
		return
	}
	trimmedTitle := strings.TrimSpace(sectionTitle)
	progressLabel := buildFullDocumentStructuredProgressLabel(sectionCurrent, sectionTotal, trimmedTitle, 0, 0)
	if err := eventBus.Emit(ctx, event.Event{
		ID:        eventID,
		Type:      event.EventAgentThought,
		SessionID: req.Session.ID,
		Data: event.AgentThoughtData{
			Content:        content,
			Iteration:      0,
			Done:           done,
			Replace:        false,
			Synthetic:      false,
			Stage:          strings.TrimSpace(stage),
			SectionCurrent: sectionCurrent,
			SectionTotal:   sectionTotal,
			SectionTitle:   trimmedTitle,
			ProgressLabel:  progressLabel,
		},
	}); err != nil {
		logger.Errorf(ctx, "Failed to emit full document model thinking event: %v", err)
	}
}

type knowledgeGroundedEvidenceItem struct {
	Query  string
	Result *types.SearchResult
}

type knowledgeGroundedEvidencePack struct {
	Queries        []string
	ScopeKBIDs     []string
	SectionHeading string
	Items          []knowledgeGroundedEvidenceItem
	MissingReason  string
}

type fullDocumentKnowledgeSearchService interface {
	GetKnowledgeBasesByIDsOnly(ctx context.Context, ids []string) ([]*types.KnowledgeBase, error)
	HybridSearch(ctx context.Context, knowledgeBaseID string, params types.SearchParams) ([]*types.SearchResult, error)
}

func buildDedicatedFullDocumentOutlineMessages(req *types.QARequest, language string) []chat.Message {
	var systemPrompt strings.Builder
	systemPrompt.WriteString("You are planning a complete long-form markdown document. Return JSON only. ")
	systemPrompt.WriteString("Do not write the document body. Do not call tools. Do not output markdown fences, commentary, or hidden reasoning. ")
	systemPrompt.WriteString("Do not echo internal input labels or metadata such as User goal, Planning requirements, local_knowledge_context, Current section, Completed document summary, knowledge_id, knowledge_base_id, or chunk_id. ")
	systemPrompt.WriteString("The outline is a contract for later generation. Each section must have a stable chapter number, heading, and ordered subsection plan. ")
	systemPrompt.WriteString("Prioritize the user's explicit requirements, audience, chapters, numbering, and detail level over any generic assumptions. ")
	systemPrompt.WriteString("Derive the outline directly from the user goal and available context. Do not inject a default document taxonomy, preferred chapter set, or hidden document classification. ")
	systemPrompt.WriteString(" ")
	if strings.TrimSpace(language) != "" {
		systemPrompt.WriteString(fmt.Sprintf("Respond in %s. ", language))
	}

	userContent := "User goal:\n" + strings.TrimSpace(req.Query) + "\n\n" + buildFullDocumentOutlinePlanningRequirements(false)

	return []chat.Message{
		{Role: "system", Content: strings.TrimSpace(systemPrompt.String())},
		{Role: "user", Content: userContent},
	}
}

func buildKnowledgeGroundedFullDocumentOutlineMessages(req *types.QARequest, language string, evidence knowledgeGroundedEvidencePack) []chat.Message {
	var systemPrompt strings.Builder
	systemPrompt.WriteString("You are planning a long-form markdown document grounded in local knowledge. Return JSON only. ")
	systemPrompt.WriteString("Use only facts from <local_knowledge_context>. Do not call tools. Do not write section body paragraphs. Do not output markdown fences, commentary, or hidden reasoning. ")
	systemPrompt.WriteString("Do not echo internal input labels or metadata such as User goal, Planning requirements, local_knowledge_context, Current section, Completed document summary, knowledge_id, knowledge_base_id, or chunk_id. ")
	systemPrompt.WriteString("Do not invent project background, system modules, implementation scope, product capabilities, or technical indicators that are not present in <local_knowledge_context>. ")
	systemPrompt.WriteString("Prioritize the user's explicit requirements, audience, chapters, numbering, and detail level over any generic assumptions. ")
	systemPrompt.WriteString("If the local knowledge is insufficient, keep a useful structure aligned with the user's requested deliverable and include evidence-gap/open-item chapters or subsections instead of fabricating facts. ")
	systemPrompt.WriteString("Each section must have a stable chapter number, heading, and ordered subsection plan. ")
	systemPrompt.WriteString("Derive the outline directly from the user goal and available evidence. Do not inject a default document taxonomy, preferred chapter set, or hidden document classification. ")
	systemPrompt.WriteString(" ")
	if strings.TrimSpace(language) != "" {
		systemPrompt.WriteString(fmt.Sprintf("Respond in %s. ", language))
	}

	var userContent strings.Builder
	userContent.WriteString("User goal:\n")
	userContent.WriteString(strings.TrimSpace(req.Query))
	userContent.WriteString("\n\n")
	userContent.WriteString(buildFullDocumentOutlinePlanningRequirements(true))
	userContent.WriteString("\n\n")
	userContent.WriteString(buildKnowledgeGroundedLocalKnowledgeContext(evidence))

	return []chat.Message{
		{Role: "system", Content: strings.TrimSpace(systemPrompt.String())},
		{Role: "user", Content: userContent.String()},
	}
}

func stripDedicatedFullDocumentFence(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ""
	}
	if matches := markdownFenceRE.FindStringSubmatch(trimmed); len(matches) == 2 {
		return strings.TrimSpace(matches[1])
	}
	return trimmed
}

func normalizeDedicatedFullDocumentOutlineMarkdown(content string) string {
	trimmed := stripDedicatedFullDocumentFence(content)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.ReplaceAll(trimmed, "\r\n", "\n")
	trimmed = strings.ReplaceAll(trimmed, "\r", "\n")

	normalizedLines := make([]string, 0, 12)
	for _, rawLine := range strings.Split(trimmed, "\n") {
		line := strings.TrimSpace(strings.TrimLeft(rawLine, "\ufeff"))
		if line == "" {
			continue
		}
		line = strings.ReplaceAll(line, "##", "\n##")
		for _, rawSegment := range strings.Split(line, "\n") {
			segment := strings.TrimSpace(rawSegment)
			if segment == "" {
				continue
			}
			switch {
			case strings.HasPrefix(segment, "##") && !strings.HasPrefix(segment, "## "):
				segment = "## " + strings.TrimSpace(strings.TrimPrefix(segment, "##"))
			case strings.HasPrefix(segment, "#") && !strings.HasPrefix(segment, "# ") && !strings.HasPrefix(segment, "##"):
				segment = "# " + strings.TrimSpace(strings.TrimPrefix(segment, "#"))
			}
			normalizedLines = append(normalizedLines, segment)
		}
	}

	return strings.Join(normalizedLines, "\n")
}

func parseDedicatedFullDocumentOutline(content string) dedicatedFullDocumentOutline {
	if outline, ok := parseDedicatedFullDocumentOutlineJSON(content); ok {
		return outline
	}
	content = normalizeDedicatedFullDocumentOutlineMarkdown(content)
	if content == "" {
		return dedicatedFullDocumentOutline{}
	}
	lines := strings.Split(content, "\n")
	outline := dedicatedFullDocumentOutline{Sections: make([]dedicatedFullDocumentSection, 0, 8)}
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "# ") && outline.Title == "":
			outline.Title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
		case strings.HasPrefix(line, "## "):
			sectionHeading := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			section, ok := normalizeDedicatedFullDocumentSection(dedicatedFullDocumentSection{Heading: sectionHeading}, len(outline.Sections)+1)
			if !ok {
				continue
			}
			outline.Sections = append(outline.Sections, section)
		case strings.HasPrefix(line, "### ") && len(outline.Sections) > 0:
			rawSubsection := strings.TrimSpace(strings.TrimPrefix(line, "### "))
			section := &outline.Sections[len(outline.Sections)-1]
			number := fmt.Sprintf("%d.%d", section.Number, len(section.Subsections)+1)
			title := rawSubsection
			if matches := dedicatedFullDocumentSubsectionRE.FindStringSubmatch(rawSubsection); len(matches) == 3 {
				number = strings.TrimSpace(matches[1])
				title = strings.TrimSpace(matches[2])
			}
			section.Subsections = append(section.Subsections, dedicatedFullDocumentSubsection{Number: number, Title: strings.TrimSpace(title)})
		case outline.Title == "":
			outline.Title = strings.TrimLeft(strings.TrimSpace(strings.TrimLeft(line, "-*0123456789. ")), "#")
		case strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* "):
			sectionTitle := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* ")), "0123456789. "))
			section, ok := normalizeDedicatedFullDocumentSection(dedicatedFullDocumentSection{Title: sectionTitle}, len(outline.Sections)+1)
			if !ok {
				continue
			}
			outline.Sections = append(outline.Sections, section)
		}
	}
	return normalizeDedicatedFullDocumentOutline(outline)
}

func validateDedicatedFullDocumentOutline(outline dedicatedFullDocumentOutline) error {
	outline = normalizeDedicatedFullDocumentOutline(outline)
	title := strings.TrimSpace(outline.Title)
	if title == "" || strings.Contains(title, "##") {
		return errors.New("outline_parse_failed")
	}
	if len(outline.Sections) == 0 {
		return errors.New("outline_parse_failed")
	}
	for index, section := range outline.Sections {
		expectedNumber := index + 1
		if section.Number != expectedNumber {
			return errors.New("outline_parse_failed")
		}
		if strings.TrimSpace(section.Title) == "" {
			return errors.New("outline_parse_failed")
		}
		expectedHeading := fmt.Sprintf("第%d章 %s", expectedNumber, strings.TrimSpace(section.Title))
		if strings.TrimSpace(section.Heading) != expectedHeading {
			return errors.New("outline_parse_failed")
		}
		for subsectionIndex, subsection := range section.Subsections {
			if strings.TrimSpace(subsection.Title) == "" {
				return errors.New("outline_parse_failed")
			}
			expectedSubsectionNumber := fmt.Sprintf("%d.%d", expectedNumber, subsectionIndex+1)
			if strings.TrimSpace(subsection.Number) != expectedSubsectionNumber {
				return errors.New("outline_parse_failed")
			}
		}
	}
	return nil
}

func parseDedicatedFullDocumentOutlineJSON(content string) (dedicatedFullDocumentOutline, bool) {
	trimmed := strings.TrimSpace(stripDedicatedFullDocumentFence(content))
	if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
		return dedicatedFullDocumentOutline{}, false
	}
	var payload struct {
		Title    string            `json:"title"`
		Sections []json.RawMessage `json:"sections"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return dedicatedFullDocumentOutline{}, false
	}
	outline := dedicatedFullDocumentOutline{Title: strings.TrimSpace(payload.Title), Sections: make([]dedicatedFullDocumentSection, 0, len(payload.Sections))}
	for _, rawSection := range payload.Sections {
		if len(rawSection) == 0 {
			continue
		}
		switch rawSection[0] {
		case '"':
			var title string
			if err := json.Unmarshal(rawSection, &title); err == nil {
				outline.Sections = append(outline.Sections, dedicatedFullDocumentSection{Title: strings.TrimSpace(title)})
			}
		case '{':
			var section dedicatedFullDocumentSection
			if err := json.Unmarshal(rawSection, &section); err == nil {
				outline.Sections = append(outline.Sections, section)
			}
		}
	}
	outline = normalizeDedicatedFullDocumentOutline(outline)
	if strings.TrimSpace(outline.Title) == "" && len(outline.Sections) == 0 {
		return dedicatedFullDocumentOutline{}, false
	}
	return outline, true
}

func generateValidatedFullDocumentOutline(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
	chatModel chat.Chat,
	messages []chat.Message,
	options *chat.ChatOptions,
	progress *fullDocumentProgressReporter,
	progressText string,
	query string,
) (dedicatedFullDocumentOutline, error) {
	var lastErr error
	modelThinkingEventID := generateEventID("document-outline-model-thinking")
	for attempt := 1; attempt <= 2; attempt++ {
		if attempt > 1 && progress != nil {
			progress.UpdateStage("planning", "检测到大纲结构异常，正在重试大纲规划。")
		}
		response, err := chatFullDocumentOutlineWithProgress(ctx, chatModel, messages, options, progress, progressText, func(content string, done bool) {
			emitFullDocumentModelThinking(ctx, req, eventBus, modelThinkingEventID, "planning", content, done, 0, 0, "")
		})
		if err != nil {
			return dedicatedFullDocumentOutline{}, err
		}
		outline := parseDedicatedFullDocumentOutline(response.Content)
		if strings.TrimSpace(outline.Title) == "" {
			outline.Title = fallbackDedicatedFullDocumentTitle(query)
		}
		if err := validateDedicatedFullDocumentOutline(outline); err == nil {
			return outline, nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("outline_parse_failed")
	}
	return dedicatedFullDocumentOutline{}, lastErr
}

func extractRenderedFullDocumentOutline(content string) dedicatedFullDocumentOutline {
	trimmed := stripDedicatedFullDocumentFence(content)
	if trimmed == "" {
		return dedicatedFullDocumentOutline{}
	}
	trimmed = strings.ReplaceAll(trimmed, "\r\n", "\n")
	trimmed = strings.ReplaceAll(trimmed, "\r", "\n")
	outline := dedicatedFullDocumentOutline{Sections: make([]dedicatedFullDocumentSection, 0, 8)}
	for _, rawLine := range strings.Split(trimmed, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "# ") && outline.Title == "":
			outline.Title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
		case strings.HasPrefix(line, "## "):
			sectionHeading := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			section, ok := normalizeDedicatedFullDocumentSection(dedicatedFullDocumentSection{Heading: sectionHeading}, len(outline.Sections)+1)
			if !ok {
				continue
			}
			outline.Sections = append(outline.Sections, section)
		}
	}
	return normalizeDedicatedFullDocumentOutline(outline)
}

func extractRenderedFullDocumentSubsections(content string) map[string][]dedicatedFullDocumentSubsection {
	trimmed := stripDedicatedFullDocumentFence(content)
	if trimmed == "" {
		return nil
	}
	trimmed = strings.ReplaceAll(trimmed, "\r\n", "\n")
	trimmed = strings.ReplaceAll(trimmed, "\r", "\n")
	sections := make(map[string][]dedicatedFullDocumentSubsection)
	currentSectionTitle := ""
	for _, rawLine := range strings.Split(trimmed, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "## "):
			_, title := parseDedicatedFullDocumentSectionNumberAndTitle(strings.TrimSpace(strings.TrimPrefix(line, "## ")), 0)
			currentSectionTitle = strings.TrimSpace(firstNonEmptyString(title, strings.TrimSpace(strings.TrimPrefix(line, "## "))))
			if currentSectionTitle != "" {
				if _, exists := sections[currentSectionTitle]; !exists {
					sections[currentSectionTitle] = nil
				}
			}
		case strings.HasPrefix(line, "### ") && currentSectionTitle != "":
			rawSubsection := strings.TrimSpace(strings.TrimPrefix(line, "### "))
			number := ""
			title := rawSubsection
			if matches := dedicatedFullDocumentSubsectionRE.FindStringSubmatch(rawSubsection); len(matches) == 3 {
				number = strings.TrimSpace(matches[1])
				title = strings.TrimSpace(matches[2])
			}
			title = strings.TrimSpace(title)
			if title == "" {
				continue
			}
			sections[currentSectionTitle] = append(sections[currentSectionTitle], dedicatedFullDocumentSubsection{Number: number, Title: title})
		}
	}
	return sections
}

func renderedFullDocumentSubsectionsMatchOutline(outline dedicatedFullDocumentOutline, finalAnswer string) bool {
	outline = normalizeDedicatedFullDocumentOutline(outline)
	actual := extractRenderedFullDocumentSubsections(finalAnswer)
	for _, section := range outline.Sections {
		if len(section.Subsections) == 0 {
			continue
		}
		actualSubsections, ok := actual[strings.TrimSpace(section.Title)]
		if !ok || len(actualSubsections) != len(section.Subsections) {
			return false
		}
		for index, expected := range section.Subsections {
			if strings.TrimSpace(actualSubsections[index].Number) != strings.TrimSpace(expected.Number) {
				return false
			}
			if strings.TrimSpace(actualSubsections[index].Title) != strings.TrimSpace(expected.Title) {
				return false
			}
		}
	}
	return true
}

func missingFullDocumentSections(expected []string, actual []string) []string {
	if len(expected) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(actual))
	for _, section := range actual {
		trimmed := strings.TrimSpace(section)
		if trimmed == "" {
			continue
		}
		seen[trimmed] = struct{}{}
	}
	missing := make([]string, 0)
	for _, section := range expected {
		trimmed := strings.TrimSpace(section)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		missing = append(missing, trimmed)
	}
	return missing
}

func completedFullDocumentIntegrityFailureReason(outline dedicatedFullDocumentOutline, completedSections []string, finalAnswer string) string {
	if err := validateDedicatedFullDocumentOutline(outline); err != nil {
		return "outline_parse_failed"
	}
	if len(filterOutlineSections(outline, completedSections)) != len(outline.Sections) {
		return "outline_or_section_incomplete"
	}
	rendered := extractRenderedFullDocumentOutline(finalAnswer)
	if strings.TrimSpace(rendered.Title) == "" || strings.Contains(rendered.Title, "##") {
		return "outline_or_section_incomplete"
	}
	if len(missingFullDocumentSections(dedicatedFullDocumentSectionTitles(outline), dedicatedFullDocumentSectionTitles(rendered))) > 0 {
		return "outline_or_section_incomplete"
	}
	if !renderedFullDocumentSubsectionsMatchOutline(outline, finalAnswer) {
		return "outline_or_section_incomplete"
	}
	return ""
}

type fullDocumentIntegrityAssessment struct {
	FailureReason            string
	DocumentGenerationStatus string
	QualityIssues            []string
}

func assessCompletedFullDocumentIntegrityForArtifact(outline dedicatedFullDocumentOutline, completedSections []string, finalAnswer string) fullDocumentIntegrityAssessment {
	if err := validateDedicatedFullDocumentOutline(outline); err != nil {
		return fullDocumentIntegrityAssessment{
			FailureReason:            "outline_parse_failed",
			DocumentGenerationStatus: types.ChatDocumentGenerationStatusBlocked,
			QualityIssues:            []string{types.ChatDocumentQualityIssueMarkdownStructureInvalid},
		}
	}
	if len(filterOutlineSections(outline, completedSections)) != len(outline.Sections) {
		return fullDocumentIntegrityAssessment{
			FailureReason:            "outline_or_section_incomplete",
			DocumentGenerationStatus: types.ChatDocumentGenerationStatusContinuing,
			QualityIssues:            []string{types.ChatDocumentQualityIssueMarkdownStructureInvalid},
		}
	}
	rendered := extractRenderedFullDocumentOutline(finalAnswer)
	if strings.TrimSpace(rendered.Title) == "" || strings.Contains(rendered.Title, "##") {
		return fullDocumentIntegrityAssessment{
			FailureReason:            "outline_or_section_incomplete",
			DocumentGenerationStatus: types.ChatDocumentGenerationStatusNeedsReview,
			QualityIssues:            []string{types.ChatDocumentQualityIssueMarkdownStructureInvalid},
		}
	}
	if len(missingFullDocumentSections(dedicatedFullDocumentSectionTitles(outline), dedicatedFullDocumentSectionTitles(rendered))) > 0 {
		return fullDocumentIntegrityAssessment{
			FailureReason:            "outline_or_section_incomplete",
			DocumentGenerationStatus: types.ChatDocumentGenerationStatusNeedsReview,
			QualityIssues:            []string{types.ChatDocumentQualityIssueMarkdownStructureInvalid},
		}
	}
	if !renderedFullDocumentSubsectionsMatchOutline(outline, finalAnswer) {
		return fullDocumentIntegrityAssessment{
			FailureReason:            "outline_or_section_incomplete",
			DocumentGenerationStatus: types.ChatDocumentGenerationStatusNeedsReview,
			QualityIssues:            []string{types.ChatDocumentQualityIssueMarkdownUnplannedSubsection},
		}
	}
	return fullDocumentIntegrityAssessment{}
}

func normalizeFullDocumentFinalAnswer(content string) (string, []string) {
	normalized, signals := normalizeGeneratedMarkdown(content)
	if strings.TrimSpace(normalized) == "" {
		return strings.TrimSpace(content), signals
	}
	return strings.TrimSpace(normalized), signals
}

func applyFullDocumentArtifactQualityGate(
	ctx context.Context,
	chatModel chat.Chat,
	agentConfig *types.AgentConfig,
	budget DocumentGenerationBudget,
	content string,
	completionStatus string,
	finishReason string,
	failureReason string,
	documentGenerationStatus string,
	qualityIssues []string,
) (string, string, string, string, string, []string) {
	finalContent, finalSignals := normalizeFullDocumentFinalAnswer(content)
	qualityIssues = append(qualityIssues, finalSignals...)
	if strings.TrimSpace(finalContent) == "" {
		return finalContent, completionStatus, finishReason, failureReason, documentGenerationStatus, uniqueNonEmptyStrings(qualityIssues)
	}

	repairedContent, documentSignals, qualityOK := applyGeneratedDocumentMarkdownQualityGate(ctx, chatModel, agentConfig, budget, finalContent)
	qualityIssues = append(qualityIssues, documentSignals...)
	if strings.TrimSpace(repairedContent) != "" {
		finalContent = strings.TrimSpace(repairedContent)
	}
	if !qualityOK {
		if completionStatus == types.MessageCompletionStatusCompleted {
			finishReason = "stop"
			failureReason = ""
			documentGenerationStatus = types.ChatDocumentGenerationStatusNeedsReview
		} else if strings.TrimSpace(documentGenerationStatus) == "" {
			documentGenerationStatus = types.ChatDocumentGenerationStatusContinuing
		}
	}

	return finalContent, completionStatus, finishReason, failureReason, documentGenerationStatus, uniqueNonEmptyStrings(qualityIssues)
}

func applyArtifactFirstFullDocumentIntegrityOutcome(outline dedicatedFullDocumentOutline, completedSections []string, finalAnswer string, completionStatus string, finishReason string, failureReason string, documentGenerationStatus string, qualityIssues []string) (string, string, string, string, []string) {
	if completionStatus != types.MessageCompletionStatusCompleted {
		return completionStatus, finishReason, failureReason, documentGenerationStatus, qualityIssues
	}
	assessment := assessCompletedFullDocumentIntegrityForArtifact(outline, completedSections, finalAnswer)
	if strings.TrimSpace(assessment.FailureReason) == "" {
		return completionStatus, finishReason, failureReason, documentGenerationStatus, qualityIssues
	}
	qualityIssues = append(qualityIssues, assessment.QualityIssues...)
	switch types.NormalizeChatDocumentGenerationStatus(assessment.DocumentGenerationStatus) {
	case types.ChatDocumentGenerationStatusNeedsReview:
		return types.MessageCompletionStatusCompleted, "stop", "", types.ChatDocumentGenerationStatusNeedsReview, qualityIssues
	case types.ChatDocumentGenerationStatusContinuing:
		return types.MessageCompletionStatusPartial, assessment.FailureReason, assessment.FailureReason, types.ChatDocumentGenerationStatusContinuing, qualityIssues
	default:
		return types.MessageCompletionStatusPartial, assessment.FailureReason, assessment.FailureReason, types.ChatDocumentGenerationStatusBlocked, qualityIssues
	}
}

func documentGenerationStatusForCompletedFullDocumentIntegrityFailure(failure string) string {
	switch strings.TrimSpace(failure) {
	case "outline_parse_failed":
		return types.ChatDocumentGenerationStatusBlocked
	case "outline_or_section_incomplete":
		return types.ChatDocumentGenerationStatusContinuing
	default:
		return types.ChatDocumentGenerationStatusBlocked
	}
}

func emitFullDocumentOutlineParseFailure(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
	message string,
	extra map[string]interface{},
	startTime time.Time,
) error {
	notice := strings.TrimSpace(message)
	if notice == "" {
		notice = "生成的大纲结构异常，无法继续生成完整文档，请稍后重试。"
	}
	answerEventID := generateEventID("document-full")
	if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, notice, true, types.MessageCompletionStatusPartial, "outline_parse_failed"); err != nil {
		logger.Errorf(ctx, "Failed to emit full document outline parse failure chunk: %v", err)
	}
	return emitFullDocumentCompletion(ctx, req, eventBus, notice, types.MessageCompletionStatusPartial, "outline_parse_failed", "outline_parse_failed", types.ChatDocumentGenerationStatusBlocked, nil, nil, extra, startTime)
}

func fallbackDedicatedFullDocumentTitle(query string) string {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return "完整文档"
	}
	runes := []rune(trimmed)
	if len(runes) > 32 {
		return string(runes[:32])
	}
	return trimmed
}

func buildKnowledgeGroundedSectionQueriesForGoal(goal string, documentTitle string, section string) []string {
	queries := make([]string, 0, 3)
	if trimmedGoal := strings.TrimSpace(goal); trimmedGoal != "" {
		queries = append(queries, trimmedGoal+" "+strings.TrimSpace(section))
	}
	if strings.TrimSpace(documentTitle) != "" && strings.TrimSpace(section) != "" {
		queries = append(queries, strings.TrimSpace(documentTitle)+" "+strings.TrimSpace(section))
	}
	if strings.TrimSpace(section) != "" {
		queries = append(queries, "请检索与当前章节“"+strings.TrimSpace(section)+"”直接相关的本地事实、关键主题和待确认事项。")
	}
	return uniqueNonEmptyStrings(queries)
}

func marshalGenerationRunJSON(value interface{}) types.JSON {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil || len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return types.JSON(raw)
}

func unmarshalGenerationRunOutline(raw types.JSON) dedicatedFullDocumentOutline {
	if len(raw) == 0 {
		return dedicatedFullDocumentOutline{}
	}
	if outline, ok := parseDedicatedFullDocumentOutlineJSON(string(raw)); ok {
		return outline
	}
	return dedicatedFullDocumentOutline{}
}

func unmarshalGenerationRunStringSlice(raw types.JSON) []string {
	if len(raw) == 0 {
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	return uniqueNonEmptyStrings(values)
}

func unmarshalGenerationRunBudget(raw types.JSON) DocumentGenerationBudget {
	if len(raw) == 0 {
		return DocumentGenerationBudget{}
	}
	var budget DocumentGenerationBudget
	if err := json.Unmarshal(raw, &budget); err != nil {
		return DocumentGenerationBudget{}
	}
	return budget
}

func normalizeDocumentGenerationRuntimeFeedback(feedback documentGenerationRuntimeFeedback) documentGenerationRuntimeFeedback {
	feedback.AdjustmentReasons = uniqueNonEmptyStrings(feedback.AdjustmentReasons)
	feedback.TaskKind = strings.TrimSpace(feedback.TaskKind)
	feedback.ActiveArtifactID = strings.TrimSpace(feedback.ActiveArtifactID)
	feedback.LastCompletionStatus = strings.TrimSpace(feedback.LastCompletionStatus)
	feedback.LastFinishReason = strings.TrimSpace(feedback.LastFinishReason)
	feedback.LastFailureReason = strings.TrimSpace(feedback.LastFailureReason)
	feedback.LastDocumentStatus = strings.TrimSpace(feedback.LastDocumentStatus)
	feedback.LastAutoContinueReason = strings.TrimSpace(feedback.LastAutoContinueReason)
	feedback.NextSection = strings.TrimSpace(feedback.NextSection)
	for index := range feedback.Sections {
		feedback.Sections[index].Section = strings.TrimSpace(feedback.Sections[index].Section)
		feedback.Sections[index].CompletionStatus = strings.TrimSpace(feedback.Sections[index].CompletionStatus)
		feedback.Sections[index].FinishReason = strings.TrimSpace(feedback.Sections[index].FinishReason)
		feedback.Sections[index].FailureReason = strings.TrimSpace(feedback.Sections[index].FailureReason)
		feedback.Sections[index].BudgetAdjustReasons = uniqueNonEmptyStrings(feedback.Sections[index].BudgetAdjustReasons)
	}

	feedback.SectionCount = len(feedback.Sections)
	feedback.LengthStopCount = 0
	feedback.TimeoutCount = 0
	feedback.LowEvidenceCount = 0
	feedback.ShortSectionCount = 0
	feedback.SlowFirstTokenCount = 0
	feedback.AverageFirstTokenLatencyMs = 0
	feedback.AverageSectionDurationMs = 0
	feedback.AverageOutputTokenEstimate = 0
	feedback.AutoContinueRound = max(feedback.AutoContinueRound, 0)
	feedback.MaxAutoContinueRounds = max(feedback.MaxAutoContinueRounds, 0)
	feedback.MinGrowthChars = max(feedback.MinGrowthChars, 0)
	feedback.MaxLowGrowthRounds = max(feedback.MaxLowGrowthRounds, 0)
	feedback.LastSnapshotCharCount = max(feedback.LastSnapshotCharCount, 0)
	feedback.LowGrowthRounds = max(feedback.LowGrowthRounds, 0)
	feedback.CompletedCount = max(feedback.CompletedCount, 0)
	feedback.RemainingCount = max(feedback.RemainingCount, 0)
	feedback.NextSourceChunkStartSeq = max(feedback.NextSourceChunkStartSeq, 0)
	feedback.NextSourceChunkEndSeq = max(feedback.NextSourceChunkEndSeq, 0)

	if len(feedback.Sections) == 0 {
		return feedback
	}

	var totalFirstTokenLatencyMs int64
	var totalDurationMs int64
	var totalOutputTokenEstimate int
	for _, section := range feedback.Sections {
		totalFirstTokenLatencyMs += nonNegativeInt64(section.FirstTokenLatencyMs)
		totalDurationMs += nonNegativeInt64(section.DurationMs)
		totalOutputTokenEstimate += max(section.OutputTokenEstimate, 0)
		if strings.TrimSpace(section.FinishReason) == "length" {
			feedback.LengthStopCount++
		}
		if strings.TrimSpace(section.FinishReason) == "section_generation_timeout" || strings.TrimSpace(section.FailureReason) == "llm_timeout" {
			feedback.TimeoutCount++
		}
		if section.EvidenceCount >= 0 && section.EvidenceCount <= documentRuntimeLowEvidenceThreshold {
			feedback.LowEvidenceCount++
		}
		if section.FirstTokenLatencyMs >= documentRuntimeSlowFirstTokenThresholdMs {
			feedback.SlowFirstTokenCount++
		}
		if strings.TrimSpace(section.FinishReason) == "stop" && section.OutputTokenEstimate > 0 && section.OutputTokenEstimate <= documentRuntimeShortSectionThresholdTokens {
			feedback.ShortSectionCount++
		}
		if section.BudgetAdjusted {
			feedback.BudgetAdjusted = true
			feedback.AdjustmentReasons = uniqueNonEmptyStrings(append(feedback.AdjustmentReasons, section.BudgetAdjustReasons...))
		}
	}
	feedback.AverageFirstTokenLatencyMs = totalFirstTokenLatencyMs / int64(len(feedback.Sections))
	feedback.AverageSectionDurationMs = totalDurationMs / int64(len(feedback.Sections))
	feedback.AverageOutputTokenEstimate = totalOutputTokenEstimate / len(feedback.Sections)
	return feedback
}

func unmarshalGenerationRunRuntimeFeedback(raw types.JSON) documentGenerationRuntimeFeedback {
	if len(raw) == 0 {
		return documentGenerationRuntimeFeedback{}
	}
	var feedback documentGenerationRuntimeFeedback
	if err := json.Unmarshal(raw, &feedback); err != nil {
		return documentGenerationRuntimeFeedback{}
	}
	return normalizeDocumentGenerationRuntimeFeedback(feedback)
}

func documentGenerationRuntimeFeedbackData(feedback documentGenerationRuntimeFeedback) map[string]interface{} {
	normalized := normalizeDocumentGenerationRuntimeFeedback(feedback)
	if normalized.SectionCount == 0 && !normalized.BudgetAdjusted && normalized.RecommendedSectionLimitPerRun <= 0 {
		return nil
	}
	sections := make([]map[string]interface{}, 0, len(normalized.Sections))
	for _, section := range normalized.Sections {
		item := map[string]interface{}{
			"section":                strings.TrimSpace(section.Section),
			"first_token_latency_ms": section.FirstTokenLatencyMs,
			"duration_ms":            section.DurationMs,
			"output_rune_count":      section.OutputRuneCount,
			"output_token_estimate":  section.OutputTokenEstimate,
			"completion_status":      strings.TrimSpace(section.CompletionStatus),
			"finish_reason":          strings.TrimSpace(section.FinishReason),
		}
		if section.EvidenceCount >= 0 {
			item["evidence_count"] = section.EvidenceCount
		}
		if strings.TrimSpace(section.FailureReason) != "" {
			item["failure_reason"] = strings.TrimSpace(section.FailureReason)
		}
		if section.BudgetAdjusted {
			item["budget_adjusted"] = true
			item["budget_adjust_reasons"] = uniqueNonEmptyStrings(section.BudgetAdjustReasons)
		}
		sections = append(sections, item)
	}
	data := map[string]interface{}{
		"section_count":                  normalized.SectionCount,
		"length_stop_count":              normalized.LengthStopCount,
		"timeout_count":                  normalized.TimeoutCount,
		"low_evidence_count":             normalized.LowEvidenceCount,
		"short_section_count":            normalized.ShortSectionCount,
		"slow_first_token_count":         normalized.SlowFirstTokenCount,
		"average_first_token_latency_ms": normalized.AverageFirstTokenLatencyMs,
		"average_section_duration_ms":    normalized.AverageSectionDurationMs,
		"average_output_token_estimate":  normalized.AverageOutputTokenEstimate,
		"budget_adjusted":                normalized.BudgetAdjusted,
		"sections":                       sections,
	}
	if normalized.RecommendedSectionLimitPerRun > 0 {
		data["recommended_section_limit_per_run"] = normalized.RecommendedSectionLimitPerRun
	}
	if len(normalized.AdjustmentReasons) > 0 {
		data["adjustment_reasons"] = normalized.AdjustmentReasons
	}
	return data
}

func withDocumentGenerationRuntimeFeedbackExtra(extra map[string]interface{}, feedback documentGenerationRuntimeFeedback) map[string]interface{} {
	data := documentGenerationRuntimeFeedbackData(feedback)
	if data == nil {
		return withDocumentGenerationRunStateExtra(extra, nil, feedback)
	}
	if extra == nil {
		extra = map[string]interface{}{}
	}
	extra["runtime_feedback"] = data
	return withDocumentGenerationRunStateExtra(extra, nil, feedback)
}

func effectiveDocumentGenerationSectionLimit(feedback documentGenerationRuntimeFeedback) int {
	sectionLimit := dedicatedFullDocumentSectionLimitPerRun
	if sectionLimit <= 0 {
		sectionLimit = 1
	}
	if feedback.RecommendedSectionLimitPerRun > 0 && feedback.RecommendedSectionLimitPerRun < sectionLimit {
		sectionLimit = feedback.RecommendedSectionLimitPerRun
	}
	if sectionLimit <= 0 {
		return 1
	}
	return sectionLimit
}

func mergePersistedDocumentGenerationBudget(baseBudget DocumentGenerationBudget, persistedBudget DocumentGenerationBudget) DocumentGenerationBudget {
	if persistedBudget.SectionMaxCompletionTokens <= 0 && persistedBudget.ContinuationMaxCompletionTokens <= 0 && persistedBudget.SectionEvidenceTopK <= 0 && persistedBudget.ContinuationEvidenceTopK <= 0 && persistedBudget.SectionCallTimeoutSeconds <= 0 {
		return baseBudget
	}
	merged := baseBudget
	outlineMax := maxNegotiatedOutlineTokens(baseBudget)
	sectionMax := maxNegotiatedSectionTokens(baseBudget)
	continuationMax := maxNegotiatedSectionTokens(baseBudget)
	if persistedBudget.OutlineMaxCompletionTokens > 0 {
		merged.OutlineMaxCompletionTokens = clampDocumentBudgetInt(persistedBudget.OutlineMaxCompletionTokens, min(512, outlineMax), outlineMax)
	}
	if persistedBudget.SectionMaxCompletionTokens > 0 {
		merged.SectionMaxCompletionTokens = clampDocumentBudgetInt(persistedBudget.SectionMaxCompletionTokens, min(fullDocumentSectionMinCompletionTokens, sectionMax), sectionMax)
	}
	if persistedBudget.ContinuationMaxCompletionTokens > 0 {
		merged.ContinuationMaxCompletionTokens = clampDocumentBudgetInt(persistedBudget.ContinuationMaxCompletionTokens, min(fullDocumentSectionMinCompletionTokens, continuationMax), continuationMax)
	}
	if persistedBudget.OutlineEvidenceTopK > 0 {
		merged.OutlineEvidenceTopK = clampDocumentBudgetInt(persistedBudget.OutlineEvidenceTopK, 4, 12)
	}
	if persistedBudget.SectionEvidenceTopK > 0 {
		merged.SectionEvidenceTopK = clampDocumentBudgetInt(persistedBudget.SectionEvidenceTopK, 4, 12)
	}
	if persistedBudget.ContinuationEvidenceTopK > 0 {
		merged.ContinuationEvidenceTopK = clampDocumentBudgetInt(persistedBudget.ContinuationEvidenceTopK, 4, 12)
	}
	if persistedBudget.SectionCallTimeoutSeconds > 0 {
		merged.SectionCallTimeoutSeconds = clampDocumentBudgetInt(persistedBudget.SectionCallTimeoutSeconds, 30, 300)
	}
	if strings.TrimSpace(persistedBudget.NegotiationReason) != "" {
		merged.NegotiationReason = strings.TrimSpace(persistedBudget.NegotiationReason)
	}
	merged.Source = "runtime_feedback"
	return merged
}

func nonNegativeInt64(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func buildDocumentRuntimeSectionFeedback(section string, evidenceCount int, streamResult fullDocumentSectionStreamResult) documentGenerationRuntimeSectionFeedback {
	normalizedEvidenceCount := evidenceCount
	if normalizedEvidenceCount < 0 {
		normalizedEvidenceCount = -1
	}
	return documentGenerationRuntimeSectionFeedback{
		Section:             strings.TrimSpace(section),
		EvidenceCount:       normalizedEvidenceCount,
		FirstTokenLatencyMs: nonNegativeInt64(streamResult.firstTokenLatencyMs),
		DurationMs:          nonNegativeInt64(streamResult.durationMs),
		OutputRuneCount:     max(streamResult.outputRuneCount, 0),
		OutputTokenEstimate: max(streamResult.outputTokenEstimate, 0),
		CompletionStatus:    strings.TrimSpace(streamResult.completionStatus),
		FinishReason:        strings.TrimSpace(streamResult.finishReason),
		FailureReason:       strings.TrimSpace(streamResult.failureReason),
	}
}

func adjustDocumentGenerationBudgetWithRuntimeFeedback(budget DocumentGenerationBudget, sectionFeedback documentGenerationRuntimeSectionFeedback) (DocumentGenerationBudget, []string, int) {
	adjusted := budget
	reasons := make([]string, 0, 4)
	recommendedSectionLimit := 0
	canDecreaseEvidence := sectionFeedback.EvidenceCount > documentRuntimeLowEvidenceThreshold+1

	if sectionFeedback.FinishReason == "length" {
		nextSectionTokens := clampDocumentBudgetInt(adjusted.SectionMaxCompletionTokens+documentRuntimeSectionTokenStep, min(fullDocumentSectionMinCompletionTokens, maxNegotiatedSectionTokens(adjusted)), maxNegotiatedSectionTokens(adjusted))
		if nextSectionTokens > adjusted.SectionMaxCompletionTokens {
			adjusted.SectionMaxCompletionTokens = nextSectionTokens
			adjusted.ContinuationMaxCompletionTokens = max(adjusted.ContinuationMaxCompletionTokens, nextSectionTokens)
			reasons = append(reasons, "section_tokens_up_length")
		}
		defaultLimit := effectiveDocumentGenerationSectionLimit(documentGenerationRuntimeFeedback{})
		if defaultLimit > 1 {
			recommendedSectionLimit = defaultLimit - 1
			reasons = append(reasons, "section_batch_limit_down_length")
		}
	}

	if (sectionFeedback.FinishReason == "section_generation_timeout" || sectionFeedback.FailureReason == "llm_timeout") && canDecreaseEvidence {
		nextTopK := clampDocumentBudgetInt(adjusted.SectionEvidenceTopK-1, 4, 12)
		if nextTopK < adjusted.SectionEvidenceTopK {
			adjusted.SectionEvidenceTopK = nextTopK
			adjusted.ContinuationEvidenceTopK = clampDocumentBudgetInt(adjusted.ContinuationEvidenceTopK-1, 4, 12)
			reasons = append(reasons, "section_top_k_down_timeout")
		}
		nextTimeout := clampDocumentBudgetInt(adjusted.SectionCallTimeoutSeconds+documentRuntimeSectionTimeoutStepSeconds, 30, 300)
		if nextTimeout > adjusted.SectionCallTimeoutSeconds {
			adjusted.SectionCallTimeoutSeconds = nextTimeout
			reasons = append(reasons, "section_timeout_up_timeout")
		}
		defaultLimit := effectiveDocumentGenerationSectionLimit(documentGenerationRuntimeFeedback{})
		if defaultLimit > 1 {
			recommendedSectionLimit = defaultLimit - 1
			reasons = append(reasons, "section_batch_limit_down_timeout")
		}
	}

	if sectionFeedback.EvidenceCount >= 0 && sectionFeedback.EvidenceCount <= documentRuntimeLowEvidenceThreshold {
		nextTopK := clampDocumentBudgetInt(adjusted.SectionEvidenceTopK+1, 4, 12)
		if nextTopK > adjusted.SectionEvidenceTopK {
			adjusted.SectionEvidenceTopK = nextTopK
			adjusted.ContinuationEvidenceTopK = clampDocumentBudgetInt(adjusted.ContinuationEvidenceTopK+1, 4, 12)
			reasons = append(reasons, "section_top_k_up_low_evidence")
		}
	}

	if len(reasons) > 0 {
		adjusted.Source = "runtime_feedback"
	}
	return adjusted, uniqueNonEmptyStrings(reasons), recommendedSectionLimit
}

func appendDocumentGenerationRuntimeFeedback(
	existing documentGenerationRuntimeFeedback,
	sectionFeedback documentGenerationRuntimeSectionFeedback,
	adjustmentReasons []string,
	recommendedSectionLimit int,
) documentGenerationRuntimeFeedback {
	sectionFeedback.BudgetAdjusted = len(adjustmentReasons) > 0
	sectionFeedback.BudgetAdjustReasons = uniqueNonEmptyStrings(adjustmentReasons)
	existing.Sections = append(existing.Sections, sectionFeedback)
	if recommendedSectionLimit > 0 && (existing.RecommendedSectionLimitPerRun <= 0 || recommendedSectionLimit < existing.RecommendedSectionLimitPerRun) {
		existing.RecommendedSectionLimitPerRun = recommendedSectionLimit
	}
	if len(adjustmentReasons) > 0 {
		existing.BudgetAdjusted = true
		existing.AdjustmentReasons = uniqueNonEmptyStrings(append(existing.AdjustmentReasons, adjustmentReasons...))
	}
	return normalizeDocumentGenerationRuntimeFeedback(existing)
}

func filterOutlineSections(outline dedicatedFullDocumentOutline, sections []string) []string {
	if len(outline.Sections) == 0 || len(sections) == 0 {
		return nil
	}
	outlineSections := dedicatedFullDocumentSectionTitles(outline)
	seen := make(map[string]struct{}, len(sections))
	for _, section := range sections {
		trimmed := strings.TrimSpace(section)
		if trimmed == "" {
			continue
		}
		seen[trimmed] = struct{}{}
	}
	result := make([]string, 0, len(sections))
	for _, section := range outlineSections {
		trimmed := strings.TrimSpace(section)
		if _, ok := seen[trimmed]; !ok {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func remainingOutlineSections(outline dedicatedFullDocumentOutline, completedSections []string) []string {
	if len(outline.Sections) == 0 {
		return nil
	}
	outlineSections := dedicatedFullDocumentSectionTitles(outline)
	seen := make(map[string]struct{}, len(completedSections))
	for _, section := range completedSections {
		trimmed := strings.TrimSpace(section)
		if trimmed == "" {
			continue
		}
		seen[trimmed] = struct{}{}
	}
	result := make([]string, 0, len(outlineSections))
	for _, section := range outlineSections {
		trimmed := strings.TrimSpace(section)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func searchTargetsFromKnowledgeBaseIDs(kbIDs []string, tenantID uint64) types.SearchTargets {
	if len(kbIDs) == 0 {
		return nil
	}
	result := make(types.SearchTargets, 0, len(kbIDs))
	for _, kbID := range kbIDs {
		trimmed := strings.TrimSpace(kbID)
		if trimmed == "" {
			continue
		}
		result = append(result, &types.SearchTarget{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: trimmed, TenantID: tenantID})
	}
	return result
}

func resolveGenerationRunSearchTargets(agentConfig *types.AgentConfig, run *types.ChatDocumentGenerationRun) types.SearchTargets {
	if run == nil {
		if agentConfig == nil {
			return nil
		}
		return agentConfig.SearchTargets
	}
	kbIDs := unmarshalGenerationRunStringSlice(run.EffectiveKBIDsJSON)
	if len(kbIDs) == 0 {
		if agentConfig == nil {
			return nil
		}
		return agentConfig.SearchTargets
	}
	if agentConfig != nil && len(agentConfig.SearchTargets) > 0 {
		seen := make(map[string]struct{}, len(kbIDs))
		for _, kbID := range kbIDs {
			seen[strings.TrimSpace(kbID)] = struct{}{}
		}
		filtered := make(types.SearchTargets, 0, len(kbIDs))
		for _, target := range agentConfig.SearchTargets {
			if target == nil {
				continue
			}
			if _, ok := seen[strings.TrimSpace(target.KnowledgeBaseID)]; !ok {
				continue
			}
			filtered = append(filtered, target)
		}
		if len(filtered) > 0 {
			return filtered
		}
	}
	return searchTargetsFromKnowledgeBaseIDs(kbIDs, run.TenantID)
}

func chatDocumentGenerationRunStatusFromOutcome(documentGenerationStatus string, completionStatus string) string {
	switch completionStatus {
	case types.MessageCompletionStatusCancelled:
		return types.ChatDocumentGenerationRunStatusCancelled
	case types.MessageCompletionStatusFailed:
		return types.ChatDocumentGenerationRunStatusFailed
	}
	switch types.NormalizeChatDocumentGenerationStatus(documentGenerationStatus) {
	case types.ChatDocumentGenerationStatusCompleted:
		return types.ChatDocumentGenerationRunStatusCompleted
	case types.ChatDocumentGenerationStatusBlocked:
		return types.ChatDocumentGenerationRunStatusBlocked
	case types.ChatDocumentGenerationStatusNeedsReview:
		return types.ChatDocumentGenerationRunStatusNeedsReview
	case types.ChatDocumentGenerationStatusContinuing:
		return types.ChatDocumentGenerationRunStatusContinuing
	default:
		return types.ChatDocumentGenerationRunStatusWriting
	}
}

func buildKnowledgeGroundedGenerationRunExtra(run *types.ChatDocumentGenerationRun, outline dedicatedFullDocumentOutline, effectiveKBIDs []string, budget DocumentGenerationBudget, feedback documentGenerationRuntimeFeedback) map[string]interface{} {
	extra := withDocumentGenerationBudgetExtra(map[string]interface{}{
		"effective_kb_ids": uniqueNonEmptyStrings(effectiveKBIDs),
		"outline":          dedicatedFullDocumentOutlineData(outline),
	}, budget)
	extra = withDocumentGenerationRuntimeFeedbackExtra(extra, feedback)
	extra = withDocumentGenerationRunStateExtra(extra, run, feedback)
	if run != nil {
		extra["generation_run_id"] = run.ID
	}
	return extra
}

func flattenKnowledgeGroundedEvidenceQueries(packs ...knowledgeGroundedEvidencePack) []string {
	queries := make([]string, 0)
	for _, pack := range packs {
		queries = append(queries, pack.Queries...)
	}
	return uniqueNonEmptyStrings(queries)
}

func (s *sessionService) createKnowledgeGroundedGenerationRun(
	ctx context.Context,
	req *types.QARequest,
	chatModel chat.Chat,
	outline dedicatedFullDocumentOutline,
	budget DocumentGenerationBudget,
	effectiveKBIDs []string,
) (*types.ChatDocumentGenerationRun, error) {
	if s.generationRunRepo == nil || req == nil || req.Session == nil {
		return nil, nil
	}
	tenantID, _ := types.TenantIDFromContext(ctx)
	createdBy, _ := types.UserIDFromContext(ctx)
	agentID := ""
	if req.CustomAgent != nil {
		agentID = req.CustomAgent.ID
	}
	run := &types.ChatDocumentGenerationRun{
		TenantID:              tenantID,
		SessionID:             req.Session.ID,
		RootMessageID:         req.AssistantMessageID,
		AgentID:               agentID,
		OriginalQuery:         strings.TrimSpace(req.Query),
		DocumentTitle:         strings.TrimSpace(outline.Title),
		OutlineJSON:           marshalGenerationRunJSON(outline),
		BudgetJSON:            marshalGenerationRunJSON(budget),
		RuntimeFeedbackJSON:   marshalGenerationRunJSON(newDocumentGenerationRuntimeFeedback("", resolveLongDocumentAutoContinueMaxRounds(s.cfg), resolveLongDocumentAutoContinueMinGrowthChars(s.cfg), resolveLongDocumentAutoContinueMaxLowGrowthRounds(s.cfg))),
		EffectiveKBIDsJSON:    marshalGenerationRunJSON(uniqueNonEmptyStrings(effectiveKBIDs)),
		CompletedSectionsJSON: marshalGenerationRunJSON([]string{}),
		Status:                types.ChatDocumentGenerationRunStatusWriting,
		MaxRounds:             resolveLongDocumentAutoContinueMaxRounds(s.cfg),
		ModelID:               strings.TrimSpace(chatModel.GetModelID()),
		CreatedBy:             createdBy,
	}
	if err := s.generationRunRepo.CreateRun(ctx, run); err != nil {
		return nil, err
	}
	return run, nil
}

func (s *sessionService) loadKnowledgeGroundedGenerationRun(ctx context.Context, runID string) (*types.ChatDocumentGenerationRun, error) {
	if s.generationRunRepo == nil || strings.TrimSpace(runID) == "" {
		return nil, nil
	}
	tenantID, _ := types.TenantIDFromContext(ctx)
	return s.generationRunRepo.GetRunByID(ctx, tenantID, strings.TrimSpace(runID))
}

func (s *sessionService) updateKnowledgeGroundedGenerationRun(
	ctx context.Context,
	run *types.ChatDocumentGenerationRun,
	outline dedicatedFullDocumentOutline,
	completedSections []string,
	budget DocumentGenerationBudget,
	runtimeFeedback documentGenerationRuntimeFeedback,
	documentGenerationStatus string,
	completionStatus string,
	executedRound int,
) error {
	if s.generationRunRepo == nil || run == nil {
		return nil
	}
	runtimeFeedback = normalizeDocumentGenerationRuntimeFeedback(applyDocumentGenerationRunState(runtimeFeedback, types.ChatDocumentGenerationRunState{
		AutoContinueRound:     max(run.AutoContinueRound, executedRound),
		MaxAutoContinueRounds: firstPositiveInt(run.MaxRounds, resolveLongDocumentAutoContinueMaxRounds(s.cfg)),
		MinGrowthChars:        resolveLongDocumentAutoContinueMinGrowthChars(s.cfg),
		MaxLowGrowthRounds:    resolveLongDocumentAutoContinueMaxLowGrowthRounds(s.cfg),
		CompletedCount:        len(filterOutlineSections(outline, completedSections)),
		RemainingCount:        max(len(outline.Sections)-len(filterOutlineSections(outline, completedSections)), 0),
		NextSection:           firstRemainingOutlineSection(outline, completedSections),
	}))
	run.DocumentTitle = strings.TrimSpace(outline.Title)
	run.OutlineJSON = marshalGenerationRunJSON(outline)
	run.BudgetJSON = marshalGenerationRunJSON(budget)
	completed := filterOutlineSections(outline, completedSections)
	run.RuntimeFeedbackJSON = marshalGenerationRunJSON(runtimeFeedback)
	run.CompletedSectionsJSON = marshalGenerationRunJSON(completed)
	run.Status = chatDocumentGenerationRunStatusFromOutcome(documentGenerationStatus, completionStatus)
	if executedRound > run.AutoContinueRound {
		run.AutoContinueRound = executedRound
	}
	return s.generationRunRepo.UpdateRun(ctx, run)
}

func firstRemainingOutlineSection(outline dedicatedFullDocumentOutline, completedSections []string) string {
	completedSet := make(map[string]struct{}, len(completedSections))
	for _, section := range filterOutlineSections(outline, completedSections) {
		completedSet[strings.TrimSpace(section)] = struct{}{}
	}
	for _, section := range outline.Sections {
		title := strings.TrimSpace(section.Title)
		if title == "" {
			continue
		}
		if _, exists := completedSet[title]; exists {
			continue
		}
		return title
	}
	return ""
}

func buildFullDocumentRollingSummary(
	outline dedicatedFullDocumentOutline,
	completedSections []string,
	completedContent string,
) string {
	completedTitles := filterOutlineSections(outline, completedSections)
	if len(completedTitles) == 0 || strings.TrimSpace(completedContent) == "" {
		return ""
	}

	completedSet := make(map[string]struct{}, len(completedTitles))
	for _, title := range completedTitles {
		trimmedTitle := strings.TrimSpace(title)
		if trimmedTitle == "" {
			continue
		}
		completedSet[trimmedTitle] = struct{}{}
	}

	orderedCompleted := make([]dedicatedFullDocumentSection, 0, len(completedTitles))
	for _, section := range outline.Sections {
		if _, ok := completedSet[strings.TrimSpace(section.Title)]; !ok {
			continue
		}
		orderedCompleted = append(orderedCompleted, section)
	}
	if len(orderedCompleted) == 0 {
		return ""
	}

	sectionBodies := extractFullDocumentSectionBodiesForSummary(outline, completedContent)
	recentStart := len(orderedCompleted) - fullDocumentRollingSummaryRecentSections
	if recentStart < 0 {
		recentStart = 0
	}
	unfinishedSection, hasUnfinishedSection := findFullDocumentUnfinishedSectionForSummary(outline, completedSet, completedContent)

	var builder strings.Builder
	builder.WriteString("## Completed document summary\n")
	builder.WriteString("- Use this rolling summary to stay consistent with completed sections. Do not restate the completed sections verbatim.\n")
	builder.WriteString(fmt.Sprintf("- Completed sections: %d/%d\n", len(orderedCompleted), len(outline.Sections)))
	builder.WriteString("- Completed chapter headings and planned subsections:\n")
	for _, section := range orderedCompleted {
		builder.WriteString("  - ")
		builder.WriteString(formatDedicatedFullDocumentSectionHeadingMarkdown(section))
		if subsectionSummary := formatFullDocumentSummarySubsections(section.Subsections); subsectionSummary != "" {
			builder.WriteString("；已规划小节：")
			builder.WriteString(subsectionSummary)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("### Structured continuity notes\n")
	if recentStart > 0 {
		builder.WriteString("- Earlier section anchors:\n")
		for _, section := range orderedCompleted[:recentStart] {
			builder.WriteString("  - ")
			builder.WriteString(formatDedicatedFullDocumentSectionHeadingMarkdown(section))
			if summary := summarizeFullDocumentSectionBody(sectionBodies[strings.TrimSpace(section.Title)], fullDocumentRollingSummaryEarlierRunes); summary != "" {
				builder.WriteString("：")
				builder.WriteString(summary)
			}
			builder.WriteString("\n")
		}
	}
	builder.WriteString("- Recent section anchors:\n")
	for _, section := range orderedCompleted[recentStart:] {
		builder.WriteString("  - ")
		builder.WriteString(formatDedicatedFullDocumentSectionHeadingMarkdown(section))
		if summary := summarizeFullDocumentSectionBody(sectionBodies[strings.TrimSpace(section.Title)], fullDocumentRollingSummaryRecentRunes); summary != "" {
			builder.WriteString("：")
			builder.WriteString(summary)
		}
		builder.WriteString("\n")
	}
	carryForwardItems := extractFullDocumentCarryForwardItems(orderedCompleted[recentStart:], sectionBodies)
	if len(carryForwardItems) > 0 {
		builder.WriteString("- Carry-forward constraints and open items:\n")
		for _, item := range carryForwardItems {
			builder.WriteString("  - ")
			builder.WriteString(item)
			builder.WriteString("\n")
		}
	}
	if hasUnfinishedSection {
		builder.WriteString("- Current unfinished section snapshot:\n")
		builder.WriteString("  - ")
		builder.WriteString(formatDedicatedFullDocumentSectionHeadingMarkdown(unfinishedSection))
		if subsectionSummary := formatFullDocumentSummarySubsections(unfinishedSection.Subsections); subsectionSummary != "" {
			builder.WriteString("；已规划小节：")
			builder.WriteString(subsectionSummary)
		}
		if summary := summarizeFullDocumentSectionBody(sectionBodies[strings.TrimSpace(unfinishedSection.Title)], fullDocumentRollingSummaryRecentRunes); summary != "" {
			builder.WriteString("；已生成片段：")
			builder.WriteString(summary)
		}
		builder.WriteString("\n")
	}

	return strings.TrimSpace(builder.String())
}

func findFullDocumentUnfinishedSectionForSummary(
	outline dedicatedFullDocumentOutline,
	completedSet map[string]struct{},
	completedContent string,
) (dedicatedFullDocumentSection, bool) {
	content := strings.TrimSpace(completedContent)
	if content == "" {
		return dedicatedFullDocumentSection{}, false
	}
	var unresolved dedicatedFullDocumentSection
	found := false
	for _, section := range outline.Sections {
		if _, ok := completedSet[strings.TrimSpace(section.Title)]; ok {
			continue
		}
		heading := formatDedicatedFullDocumentSectionHeadingMarkdown(section)
		if !strings.Contains(content, heading) {
			continue
		}
		unresolved = section
		found = true
	}
	return unresolved, found
}

func extractFullDocumentSectionBodiesForSummary(outline dedicatedFullDocumentOutline, completedContent string) map[string]string {
	bodies := make(map[string]string, len(outline.Sections))
	content := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(completedContent, "\r\n", "\n"), "\r", "\n"))
	if content == "" {
		return bodies
	}

	type sectionPosition struct {
		section dedicatedFullDocumentSection
		start   int
		end     int
	}
	positions := make([]sectionPosition, 0, len(outline.Sections))
	searchStart := 0
	for _, section := range outline.Sections {
		heading := formatDedicatedFullDocumentSectionHeadingMarkdown(section)
		idx := strings.Index(content[searchStart:], heading)
		if idx < 0 {
			continue
		}
		absoluteStart := searchStart + idx
		positions = append(positions, sectionPosition{
			section: section,
			start:   absoluteStart,
			end:     absoluteStart + len(heading),
		})
		searchStart = absoluteStart + len(heading)
	}

	for index, position := range positions {
		bodyEnd := len(content)
		if index+1 < len(positions) {
			bodyEnd = positions[index+1].start
		}
		bodies[strings.TrimSpace(position.section.Title)] = strings.TrimSpace(content[position.end:bodyEnd])
	}
	return bodies
}

func formatFullDocumentSummarySubsections(subsections []dedicatedFullDocumentSubsection) string {
	if len(subsections) == 0 {
		return ""
	}
	items := make([]string, 0, len(subsections))
	for _, subsection := range subsections {
		label := strings.TrimSpace(strings.TrimSpace(subsection.Number) + " " + strings.TrimSpace(subsection.Title))
		if label == "" {
			continue
		}
		items = append(items, label)
	}
	if len(items) == 0 {
		return ""
	}
	return trimRunes(strings.Join(items, "；"), fullDocumentRollingSummaryEarlierRunes)
}

func summarizeFullDocumentSectionBody(content string, limit int) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ""
	}
	keywords := []string{"待确认", "待补充", "待明确", "风险", "约束", "前置", "依赖"}
	parts := make([]string, 0, 4)
	for _, rawLine := range strings.Split(strings.ReplaceAll(strings.ReplaceAll(trimmed, "\r\n", "\n"), "\r", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimLeft(line, "-*"))
		line = strings.TrimSpace(strings.TrimPrefix(line, ">"))
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		if len(parts) == 0 {
			parts = append(parts, line)
			continue
		}
		if containsAnySummaryKeyword(line, keywords) {
			parts = append(parts, line)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return trimRunes(strings.Join(parts, " "), limit)
}

func extractFullDocumentCarryForwardItems(recentSections []dedicatedFullDocumentSection, sectionBodies map[string]string) []string {
	if len(recentSections) == 0 {
		return nil
	}
	keywords := []string{"待确认", "待补充", "待明确", "风险", "约束", "前置", "依赖"}
	seen := make(map[string]struct{}, fullDocumentRollingSummaryCarryForwardMax)
	items := make([]string, 0, fullDocumentRollingSummaryCarryForwardMax)
	for _, section := range recentSections {
		body := strings.TrimSpace(sectionBodies[strings.TrimSpace(section.Title)])
		if body == "" {
			continue
		}
		for _, rawLine := range strings.Split(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n"), "\n") {
			line := strings.TrimSpace(rawLine)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			line = strings.TrimSpace(strings.TrimLeft(line, "-*"))
			line = strings.TrimSpace(strings.TrimPrefix(line, ">"))
			line = strings.Join(strings.Fields(line), " ")
			if line == "" || !containsAnySummaryKeyword(line, keywords) {
				continue
			}
			item := fmt.Sprintf("%s：%s", strings.TrimSpace(section.Title), trimRunes(line, fullDocumentRollingSummaryEarlierRunes))
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			items = append(items, item)
			if len(items) >= fullDocumentRollingSummaryCarryForwardMax {
				return items
			}
		}
	}
	return items
}

func containsAnySummaryKeyword(content string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(content, keyword) {
			return true
		}
	}
	return false
}

func buildDedicatedFullDocumentSectionMessages(
	req *types.QARequest,
	language string,
	documentTitle string,
	outline dedicatedFullDocumentOutline,
	currentSection dedicatedFullDocumentSection,
	completedSummary string,
) []chat.Message {
	var systemPrompt strings.Builder
	systemPrompt.WriteString("You are writing one section of a long-form markdown document. Output only the section body for the requested section. ")
	systemPrompt.WriteString("Do not repeat the document title or prior sections. Do not restart the document from the beginning. Do not call tools. Do not output hidden reasoning. ")
	systemPrompt.WriteString("Use the completed document summary to keep terminology, module boundaries, constraints, and open items consistent with earlier sections. ")
	systemPrompt.WriteString(fullDocumentSectionMarkdownStyleInstructions())
	systemPrompt.WriteString(" ")
	if strings.TrimSpace(language) != "" {
		systemPrompt.WriteString(fmt.Sprintf("Respond in %s. ", language))
	}

	outlineMarkdown := formatFullDocumentOutlineMarkdown(outline)

	var userContent strings.Builder
	userContent.WriteString("User goal:\n")
	userContent.WriteString(strings.TrimSpace(req.Query))
	userContent.WriteString("\n\nDocument title:\n# ")
	userContent.WriteString(strings.TrimSpace(documentTitle))
	userContent.WriteString("\n\nFull outline:\n")
	userContent.WriteString(outlineMarkdown)
	if strings.TrimSpace(completedSummary) != "" {
		userContent.WriteString("\n\nCompleted document summary:\n")
		userContent.WriteString(strings.TrimSpace(completedSummary))
	}
	userContent.WriteString("\n\n")
	userContent.WriteString(buildDedicatedFullDocumentSectionContractPrompt(currentSection))
	userContent.WriteString("\n\nCurrent section heading (already emitted to the user, do not repeat it):\n## ")
	userContent.WriteString(strings.TrimSpace(currentSection.Heading))
	userContent.WriteString("\n\n")
	userContent.WriteString(fullDocumentDesignDepthPrompt())
	userContent.WriteString("\n\n")
	userContent.WriteString(fullDocumentSectionMarkdownStylePrompt())
	userContent.WriteString("\n\nWrite the body for this section only. Do not repeat the current H2 heading. If you add lower-level headings, keep them strictly valid markdown headings and follow the planned chapter numbering exactly.")

	return []chat.Message{
		{Role: "system", Content: strings.TrimSpace(systemPrompt.String())},
		{Role: "user", Content: userContent.String()},
	}
}

func buildKnowledgeGroundedFullDocumentSectionMessages(
	req *types.QARequest,
	language string,
	documentTitle string,
	outline dedicatedFullDocumentOutline,
	currentSection dedicatedFullDocumentSection,
	completedSummary string,
	evidence knowledgeGroundedEvidencePack,
) []chat.Message {
	var systemPrompt strings.Builder
	systemPrompt.WriteString("You are writing one section of a long-form markdown document grounded in local knowledge. Output only the section body for the requested section. ")
	systemPrompt.WriteString("Do not repeat the document title or prior sections. Do not restart the document from the beginning. Do not call tools. Do not output hidden reasoning. ")
	systemPrompt.WriteString("Use only facts from <local_knowledge_context> and the completed document summary. Do not invent project facts, product capabilities, numbers, organization names, or implementation scope. ")
	systemPrompt.WriteString("If the evidence is insufficient for this section, explicitly say the local knowledge is insufficient instead of fabricating content. ")
	systemPrompt.WriteString(fullDocumentSectionMarkdownStyleInstructions())
	systemPrompt.WriteString(" ")
	if strings.TrimSpace(language) != "" {
		systemPrompt.WriteString(fmt.Sprintf("Respond in %s. ", language))
	}

	outlineMarkdown := formatFullDocumentOutlineMarkdown(outline)

	var userContent strings.Builder
	userContent.WriteString("User goal:\n")
	userContent.WriteString(strings.TrimSpace(req.Query))
	userContent.WriteString("\n\nDocument title:\n# ")
	userContent.WriteString(strings.TrimSpace(documentTitle))
	userContent.WriteString("\n\nFull outline:\n")
	userContent.WriteString(outlineMarkdown)
	if strings.TrimSpace(completedSummary) != "" {
		userContent.WriteString("\n\nCompleted document summary:\n")
		userContent.WriteString(strings.TrimSpace(completedSummary))
	}
	userContent.WriteString("\n\n")
	userContent.WriteString(buildDedicatedFullDocumentSectionContractPrompt(currentSection))
	userContent.WriteString("\n\nCurrent section heading (already emitted to the user, do not repeat it):\n## ")
	userContent.WriteString(strings.TrimSpace(currentSection.Heading))
	userContent.WriteString("\n\n")
	userContent.WriteString(buildKnowledgeGroundedLocalKnowledgeContext(evidence))
	userContent.WriteString("\n\n")
	userContent.WriteString(fullDocumentDesignDepthPrompt())
	userContent.WriteString("\n\n")
	userContent.WriteString(fullDocumentSectionMarkdownStylePrompt())
	userContent.WriteString("\n\nWrite the body for this section only. Do not repeat the current H2 heading. If you add lower-level headings, keep them strictly valid markdown headings and follow the planned chapter numbering exactly.")

	return []chat.Message{
		{Role: "system", Content: strings.TrimSpace(systemPrompt.String())},
		{Role: "user", Content: userContent.String()},
	}
}

func buildKnowledgeGroundedLocalKnowledgeContext(evidence knowledgeGroundedEvidencePack) string {
	if len(evidence.Items) == 0 {
		return "<local_knowledge_context></local_knowledge_context>"
	}

	var builder strings.Builder
	builder.WriteString("<local_knowledge_context>\n")
	for _, query := range evidence.Queries {
		trimmedQuery := strings.TrimSpace(query)
		if trimmedQuery == "" {
			continue
		}
		builder.WriteString("<query>")
		builder.WriteString(xmlEscape(trimmedQuery))
		builder.WriteString("</query>\n")
	}
	for index, item := range evidence.Items {
		if item.Result == nil {
			continue
		}
		builder.WriteString(fmt.Sprintf("<chunk rank=\"%d\" knowledge_base_id=\"%s\" knowledge_id=\"%s\" chunk_id=\"%s\" score=\"%.4f\">\n", index+1, item.Result.KnowledgeBaseID, item.Result.KnowledgeID, item.Result.ID, item.Result.Score))
		if strings.TrimSpace(item.Query) != "" {
			builder.WriteString("<source_query>")
			builder.WriteString(xmlEscape(strings.TrimSpace(item.Query)))
			builder.WriteString("</source_query>\n")
		}
		if strings.TrimSpace(item.Result.KnowledgeTitle) != "" {
			builder.WriteString("<knowledge_title>")
			builder.WriteString(xmlEscape(strings.TrimSpace(item.Result.KnowledgeTitle)))
			builder.WriteString("</knowledge_title>\n")
		}
		builder.WriteString("<content>")
		builder.WriteString(xmlEscape(trimRunes(strings.TrimSpace(item.Result.Content), fullDocumentEvidenceChunkRuneLimit)))
		builder.WriteString("</content>\n</chunk>\n")
	}
	builder.WriteString("</local_knowledge_context>")
	return builder.String()
}

func fullDocumentDesignDepthPrompt() string {
	return strings.TrimSpace(`## 文档内容深度要求
- 本节需要写成可直接交付或继续修订的正式正文，不要停留在概述层面。
- 优先覆盖与当前章节直接相关的已确认事实、用户要求、功能或内容范围、流程或规则、依赖约束、验收口径、风险和待补充项；具体取舍以当前章节标题和大纲为准。
- 按“已确认事实、合理推导、待补充项”分层表达：已确认事实必须来自用户提示词、已生成大纲、上下文或本地知识；推导内容要使用保守表述；缺失信息要明确标注为待确认或建议补充。
- 对模块、流程、规则、依赖、交付物和验收点，尽量使用条目化清单展开，避免只写抽象口号。
- 如果证据不足，不要把未知内容写成确定事实；可以输出待确认事项、补充资料建议和继续修订所需信息。`)
}

func fullDocumentSectionMarkdownStyleInstructions() string {
	return strings.Join([]string{
		"Use polished, professional writing that matches the user's requested deliverable and intended readers.",
		"Do not repeat the document title or the current H2 heading because the system has already emitted them.",
		"Write detailed section content rather than a high-level summary, while separating confirmed facts, cautious inference, and open items.",
		"Use H3 or H4 headings only when they improve readability inside the current section.",
		"Every markdown heading must be on its own line and must contain exactly one space after heading markers, for example: ### 3.1 全域数据湖建设.",
		"Never write malformed headings such as ###3.1全域数据湖建设, ### 3.1 全域数据湖建设正文内容粘连, or two headings on the same line.",
		"Leave one blank line after each heading before the next paragraph or list.",
		"Keep paragraphs concise and scannable. Prefer bullet lists for requirements, modules, rules, implementation notes, risks, and open items.",
		"You may use bold lead-in labels such as **要点：**, **说明：**, **约束：**, and **验收口径：** when it improves readability.",
		"Do not switch to an unrequested document template or writing genre on your own.",
		"Do not output process text or internal labels such as Current section, Completed document summary, local_knowledge_context, knowledge_id, knowledge_base_id, chunk_id, tool names, or prompt instructions.",
		"If evidence is insufficient, say 本地知识不足 or 待确认/待补充 in user-facing prose instead of exposing internal context fields.",
		"Do not output HTML, code fences, JSON, prompt text, or hidden reasoning.",
	}, " ")
}

func fullDocumentSectionMarkdownStylePrompt() string {
	return strings.TrimSpace(`## 排版与行文要求
- 本节正文用于长文档正式交付，请根据用户要求和当前章节目标，使用正式、清晰、专业的文风。
- 不要额外套用未被用户要求的文档模板、章节范式或写作体裁。
- 当前 H2 章节标题已经由系统输出，请不要重复输出 H1/H2 标题。
- 如需拆分小节，只能使用 H3/H4 标题；标题必须独占一行，且 ### 或 #### 后必须有一个空格。
- 正确示例：### 3.1 全域数据湖建设
- 错误示例：###3.1全域数据湖建设
- 错误示例：### 3.1 全域数据湖建设正文内容粘连
- 每个标题后必须空一行再写正文或列表。
- 每个自然段控制在 2-4 句，避免大段堆叠。
- 对需求、内容模块、流程规则、实现要点、风险与保障等内容，优先使用项目符号拆分。
- 可使用加粗标签提升可读性，例如：**要点：**、**说明：**、**约束：**、**验收口径：**。
- 不得输出 Current section、Completed document summary、local_knowledge_context、knowledge_id、knowledge_base_id、chunk_id、工具名或任何过程说明。
- 如果证据不足，请在用户可见正文中明确写出“本地知识不足”“待确认”或“建议补充资料”，不要泄漏内部上下文标签。
- 不要输出 HTML、代码块、JSON、提示词说明或隐藏推理。`)
}

func trimRunes(content string, limit int) string {
	trimmed := strings.TrimSpace(content)
	if limit <= 0 || len([]rune(trimmed)) <= limit {
		return trimmed
	}
	runes := []rune(trimmed)
	return strings.TrimSpace(string(runes[:limit]))
}

func buildKnowledgeGroundedOutlineQueries(req *types.QARequest) []string {
	if req == nil {
		return nil
	}
	return buildKnowledgeGroundedOutlineQueriesForGoal(req.Query)
}

func buildKnowledgeGroundedOutlineQueriesForGoal(goal string) []string {
	trimmedQuery := strings.TrimSpace(goal)
	if trimmedQuery == "" {
		return nil
	}
	return uniqueNonEmptyStrings([]string{
		trimmedQuery,
		"请检索与“" + trimmedQuery + "”直接相关的本地事实、关键主题和待确认事项。",
	})
}

func buildKnowledgeGroundedSectionQueries(req *types.QARequest, documentTitle string, section string) []string {
	return buildKnowledgeGroundedSectionQueriesForGoal(req.Query, documentTitle, section)
}

func buildKnowledgeGroundedContinuationQueries(req *types.QARequest) []string {
	queries := make([]string, 0, 4)
	baseGoal := strings.TrimSpace(req.AutoContinueOriginalQuery)
	if baseGoal == "" {
		baseGoal = strings.TrimSpace(req.Query)
	}
	if baseGoal != "" {
		queries = append(queries, baseGoal)
		queries = append(queries, baseGoal+" 继续剩余内容")
	}
	if req.BaseArtifact != nil {
		title, heading := inferContinuationAnchors(req.BaseArtifact.ContentSnapshot)
		if title != "" && heading != "" {
			queries = append(queries, title+" "+heading)
		} else if heading != "" {
			queries = append(queries, heading)
		}
	}
	return uniqueNonEmptyStrings(queries)
}

func inferContinuationAnchors(snapshot string) (string, string) {
	trimmed := strings.TrimSpace(snapshot)
	if trimmed == "" {
		return "", ""
	}
	lines := strings.Split(trimmed, "\n")
	title := ""
	lastHeading := ""
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		switch {
		case strings.HasPrefix(line, "# ") && title == "":
			title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
		case strings.HasPrefix(line, "## "):
			lastHeading = strings.TrimSpace(strings.TrimPrefix(line, "## "))
		case strings.HasPrefix(line, "### "):
			lastHeading = strings.TrimSpace(strings.TrimPrefix(line, "### "))
		}
	}
	return title, lastHeading
}

func buildKnowledgeGroundedDocumentContinuationMessages(
	req *types.QARequest,
	language string,
	baseArtifact *types.ChatDocumentArtifact,
	evidence knowledgeGroundedEvidencePack,
) []chat.Message {
	var systemPrompt strings.Builder
	systemPrompt.WriteString("You are continuing a long-form markdown document grounded in local knowledge. Output only the next incremental continuation, not the full document. ")
	systemPrompt.WriteString("Use only facts from <local_knowledge_context> and the current document snapshot. Do not restart the document from the beginning. Do not repeat completed sections unless you are continuing the unfinished current section. Do not call tools. Do not output hidden reasoning. ")
	systemPrompt.WriteString("If the local knowledge is insufficient to continue, clearly state that the local knowledge is insufficient instead of inventing content. ")
	if strings.TrimSpace(language) != "" {
		systemPrompt.WriteString(fmt.Sprintf("Respond in %s. ", language))
	}

	baseGoal := strings.TrimSpace(req.AutoContinueOriginalQuery)
	if baseGoal == "" {
		baseGoal = strings.TrimSpace(req.Query)
	}
	var userContent strings.Builder
	userContent.WriteString("Original user goal:\n")
	userContent.WriteString(baseGoal)
	userContent.WriteString("\n\nCurrent continuation request:\n")
	userContent.WriteString(strings.TrimSpace(req.AutoContinuePrompt))
	if strings.TrimSpace(req.AutoContinuePrompt) == "" {
		userContent.WriteString(strings.TrimSpace(req.Query))
	}
	userContent.WriteString("\n\nCurrent document snapshot:\n")
	userContent.WriteString(strings.TrimSpace(baseArtifact.ContentSnapshot))
	userContent.WriteString("\n\n")
	userContent.WriteString(buildKnowledgeGroundedLocalKnowledgeContext(evidence))
	userContent.WriteString("\n\nContinue the document from the current snapshot. Output only the incremental delta that should be appended to the base document.")

	return []chat.Message{
		{Role: "system", Content: strings.TrimSpace(systemPrompt.String())},
		{Role: "user", Content: userContent.String()},
	}
}

func uniqueNonEmptyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func retrieveKnowledgeGroundedFullDocumentEvidence(
	ctx context.Context,
	searchService fullDocumentKnowledgeSearchService,
	cfg *config.Config,
	searchTargets types.SearchTargets,
	queries []string,
	limit int,
	progress ...fullDocumentEvidenceProgress,
) (knowledgeGroundedEvidencePack, error) {
	pack := knowledgeGroundedEvidencePack{
		Queries:    uniqueNonEmptyStrings(queries),
		ScopeKBIDs: searchTargets.GetAllKnowledgeBaseIDs(),
	}
	if len(pack.Queries) == 0 || len(searchTargets) == 0 || searchService == nil {
		pack.MissingReason = "local_knowledge_not_found"
		return pack, nil
	}

	topK := 5
	vectorThreshold := 0.6
	keywordThreshold := 0.5
	if cfg != nil && cfg.Conversation != nil {
		if cfg.Conversation.EmbeddingTopK > 0 {
			topK = cfg.Conversation.EmbeddingTopK
		}
		if cfg.Conversation.VectorThreshold > 0 {
			vectorThreshold = cfg.Conversation.VectorThreshold
		}
		if cfg.Conversation.KeywordThreshold > 0 {
			keywordThreshold = cfg.Conversation.KeywordThreshold
		}
	}
	if limit > 0 {
		topK = limit
	}

	kbByID := make(map[string]*types.KnowledgeBase)
	if len(pack.ScopeKBIDs) > 0 {
		kbs, err := searchService.GetKnowledgeBasesByIDsOnly(ctx, pack.ScopeKBIDs)
		if err != nil {
			return pack, err
		}
		for _, kb := range kbs {
			if kb != nil {
				kbByID[kb.ID] = kb
			}
		}
	}

	itemsByChunkID := make(map[string]knowledgeGroundedEvidenceItem)
	searchableScope := false
	progressCurrent := 0
	progressTotal := len(searchTargets) * len(pack.Queries)
	for _, target := range searchTargets {
		if target == nil || strings.TrimSpace(target.KnowledgeBaseID) == "" {
			continue
		}
		if kb := kbByID[target.KnowledgeBaseID]; kb != nil {
			switch kb.Type {
			case types.KnowledgeBaseTypeWiki, types.KnowledgeBaseTypeDatabase:
				continue
			}
		}
		searchableScope = true
		for _, query := range pack.Queries {
			progressCurrent++
			if len(progress) > 0 && progress[0] != nil {
				progress[0](progressCurrent, progressTotal, query)
			}
			results, err := searchService.HybridSearch(ctx, target.KnowledgeBaseID, types.SearchParams{
				QueryText:        query,
				VectorThreshold:  vectorThreshold,
				KeywordThreshold: keywordThreshold,
				MatchCount:       topK,
				KnowledgeIDs:     append([]string(nil), target.KnowledgeIDs...),
			})
			if err != nil {
				logger.Warnf(ctx, "knowledge_grounded_full_document_search_failed: kb_id=%s query=%q err=%v", target.KnowledgeBaseID, query, err)
				continue
			}
			for _, result := range results {
				if result == nil || strings.TrimSpace(result.Content) == "" {
					continue
				}
				if strings.TrimSpace(result.KnowledgeBaseID) == "" {
					result.KnowledgeBaseID = target.KnowledgeBaseID
				}
				key := strings.TrimSpace(result.ID)
				if key == "" {
					key = fmt.Sprintf("%s:%s:%d", result.KnowledgeBaseID, result.KnowledgeID, result.ChunkIndex)
				}
				current, exists := itemsByChunkID[key]
				if !exists || current.Result == nil || result.Score > current.Result.Score {
					itemsByChunkID[key] = knowledgeGroundedEvidenceItem{Query: query, Result: result}
				}
			}
		}
	}
	if !searchableScope {
		pack.MissingReason = "local_knowledge_not_found"
		return pack, nil
	}

	pack.Items = make([]knowledgeGroundedEvidenceItem, 0, len(itemsByChunkID))
	for _, item := range itemsByChunkID {
		pack.Items = append(pack.Items, item)
	}
	sort.Slice(pack.Items, func(i, j int) bool {
		left := 0.0
		right := 0.0
		if pack.Items[i].Result != nil {
			left = pack.Items[i].Result.Score
		}
		if pack.Items[j].Result != nil {
			right = pack.Items[j].Result.Score
		}
		return left > right
	})
	if limit > 0 && len(pack.Items) > limit {
		pack.Items = pack.Items[:limit]
	}
	if len(pack.Items) == 0 {
		pack.MissingReason = "local_knowledge_not_found"
	}
	return pack, nil
}

func buildKnowledgeGroundedEvidenceRefs(evidencePacks ...knowledgeGroundedEvidencePack) []interface{} {
	refs := make([]interface{}, 0)
	seen := make(map[string]struct{})
	for _, pack := range evidencePacks {
		for _, item := range pack.Items {
			if item.Result == nil {
				continue
			}
			key := strings.TrimSpace(item.Result.ID)
			if key == "" {
				key = fmt.Sprintf("%s:%s:%d", item.Result.KnowledgeBaseID, item.Result.KnowledgeID, item.Result.ChunkIndex)
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			ref := map[string]interface{}{
				"query":             item.Query,
				"knowledge_base_id": item.Result.KnowledgeBaseID,
				"knowledge_id":      item.Result.KnowledgeID,
				"chunk_id":          item.Result.ID,
				"source_title":      firstNonEmptyString(strings.TrimSpace(item.Result.KnowledgeTitle), strings.TrimSpace(item.Result.KnowledgeFilename)),
				"excerpt":           freezeChatDocumentEvidenceExcerpt(item.Result.Content),
				"source_start_at":   max(item.Result.StartAt, 0),
				"source_end_at":     max(item.Result.EndAt, 0),
				"knowledge_title":   item.Result.KnowledgeTitle,
				"score":             item.Result.Score,
				"evidence_type":     types.ChatDocumentEvidenceTypeChunk,
				"content_checksum":  checksumText(strings.TrimSpace(item.Result.Content)),
			}
			if strings.TrimSpace(pack.SectionHeading) != "" {
				ref["section_heading"] = strings.TrimSpace(pack.SectionHeading)
			}
			refs = append(refs, ref)
		}
	}
	return refs
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func freezeChatDocumentEvidenceExcerpt(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.ReplaceAll(trimmed, "\r\n", "\n")
	trimmed = strings.ReplaceAll(trimmed, "\r", "\n")
	trimmed = strings.Join(strings.Fields(trimmed), " ")
	const maxExcerptRunes = 1200
	runes := []rune(trimmed)
	if len(runes) <= maxExcerptRunes {
		return trimmed
	}
	return strings.TrimSpace(string(runes[:maxExcerptRunes]))
}

func emitDedicatedFullDocumentAnswerChunk(ctx context.Context, req *types.QARequest, eventBus *event.EventBus, eventID string, content string, done bool, completionStatus string, finishReason string) error {
	return emitDedicatedFullDocumentAnswerChunkWithExtra(ctx, req, eventBus, eventID, content, done, completionStatus, finishReason, "", nil)
}

func emitDedicatedFullDocumentAnswerChunkWithExtra(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
	eventID string,
	content string,
	done bool,
	completionStatus string,
	finishReason string,
	documentGenerationStatus string,
	extra map[string]interface{},
) error {
	if req == nil || req.Session == nil || eventBus == nil || strings.TrimSpace(eventID) == "" {
		return errors.New("full document generation event is incomplete")
	}
	data := dedicatedDocumentEditEventData(
		content,
		done,
		completionStatus,
		finishReason,
		"",
		completionStatus == types.MessageCompletionStatusCompleted,
		completionStatus == types.MessageCompletionStatusCompleted,
	)
	data.DocumentGenerationStatus = documentGenerationStatus
	if len(extra) > 0 {
		data.Extra = extra
	}
	return eventBus.Emit(ctx, event.Event{
		ID:        eventID,
		Type:      event.EventAgentFinalAnswer,
		SessionID: req.Session.ID,
		Data:      data,
	})
}

func emitDedicatedFullDocumentCompletion(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
	finalAnswer string,
	completionStatus string,
	finishReason string,
	failureReason string,
	documentGenerationStatus string,
	agentSteps types.AgentSteps,
	extra map[string]interface{},
	startTime time.Time,
) error {
	return emitFullDocumentCompletion(ctx, req, eventBus, finalAnswer, completionStatus, finishReason, failureReason, documentGenerationStatus, agentSteps, nil, extra, startTime)
}

func emitFullDocumentCompletion(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
	finalAnswer string,
	completionStatus string,
	finishReason string,
	failureReason string,
	documentGenerationStatus string,
	agentSteps types.AgentSteps,
	knowledgeRefs []interface{},
	extra map[string]interface{},
	startTime time.Time,
) error {
	if req == nil || req.Session == nil || eventBus == nil {
		return errors.New("full document generation completion is incomplete")
	}
	allowComplete := completionStatus == types.MessageCompletionStatusCompleted
	allowIndexing := allowComplete
	finishReason, failureReason = normalizeFullDocumentRetryBudgetOutcome(finishReason, failureReason, req.AutoContinueRound)
	autoContinueNext := fullDocumentAutoContinueNext(documentGenerationStatus, finishReason, failureReason, req.AutoContinueRound)
	logFullDocumentCompletionSummary(ctx, req, completionStatus, finishReason, failureReason, documentGenerationStatus, agentSteps, extra, startTime, autoContinueNext)
	return eventBus.Emit(ctx, event.Event{
		ID:        generateEventID("complete"),
		Type:      event.EventAgentComplete,
		SessionID: req.Session.ID,
		Data: event.AgentCompleteData{
			SessionID:                req.Session.ID,
			TotalSteps:               len(agentSteps),
			MessageID:                req.AssistantMessageID,
			FinalAnswer:              finalAnswer,
			CompletionStatus:         completionStatus,
			FinishReason:             finishReason,
			FailureReason:            failureReason,
			AllowIndexing:            allowIndexing,
			AllowComplete:            allowComplete,
			IsPartial:                completionStatus == types.MessageCompletionStatusPartial,
			KnowledgeRefs:            knowledgeRefs,
			AgentSteps:               agentSteps,
			DocumentGenerationStatus: documentGenerationStatus,
			AutoContinueNext:         autoContinueNext,
			AutoContinueReason:       fullDocumentAutoContinueReason(documentGenerationStatus, finishReason, failureReason, req.AutoContinueRound),
			TotalDurationMs:          time.Since(startTime).Milliseconds(),
			Extra:                    extra,
		},
	})
}

type fullDocumentObservabilitySummary struct {
	GenerationRunID          string
	ArtifactID               string
	BudgetSource             string
	OutlineMaxTokens         int
	SectionMaxTokens         int
	ContinuationMaxTokens    int
	OutlineEvidenceTopK      int
	SectionEvidenceTopK      int
	ContinuationEvidenceTopK int
	QualityIssues            []string
}

func buildFullDocumentObservabilitySummary(extra map[string]interface{}) fullDocumentObservabilitySummary {
	summary := fullDocumentObservabilitySummary{}
	if len(extra) == 0 {
		return summary
	}
	if runID, ok := extra["generation_run_id"].(string); ok {
		summary.GenerationRunID = strings.TrimSpace(runID)
	}
	if artifactID, ok := extra["artifact_id"].(string); ok {
		summary.ArtifactID = strings.TrimSpace(artifactID)
	}
	if artifactID, ok := extra["final_document_artifact_id"].(string); ok && strings.TrimSpace(summary.ArtifactID) == "" {
		summary.ArtifactID = strings.TrimSpace(artifactID)
	}
	if budget, ok := extra["budget"].(map[string]interface{}); ok && budget != nil {
		summary.BudgetSource = strings.TrimSpace(interfaceStringValue(budget["source"]))
		summary.OutlineMaxTokens = interfaceIntValue(budget["outline_max_completion_tokens"])
		summary.SectionMaxTokens = interfaceIntValue(budget["section_max_completion_tokens"])
		summary.ContinuationMaxTokens = interfaceIntValue(budget["continuation_max_completion_tokens"])
		summary.OutlineEvidenceTopK = interfaceIntValue(budget["outline_evidence_top_k"])
		summary.SectionEvidenceTopK = interfaceIntValue(budget["section_evidence_top_k"])
		summary.ContinuationEvidenceTopK = interfaceIntValue(budget["continuation_evidence_top_k"])
	}
	summary.QualityIssues = uniqueNonEmptyStrings(interfaceStringSlice(extra["quality_issues"]))
	return summary
}

func interfaceStringSlice(raw interface{}) []string {
	if raw == nil {
		return nil
	}
	items := make([]string, 0, 4)
	switch typed := raw.(type) {
	case []string:
		for _, item := range typed {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				items = append(items, trimmed)
			}
		}
	case []interface{}:
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				items = append(items, strings.TrimSpace(text))
			}
		}
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

func interfaceStringValue(raw interface{}) string {
	text, _ := raw.(string)
	return strings.TrimSpace(text)
}

func interfaceIntValue(raw interface{}) int {
	switch value := raw.(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func logFullDocumentSectionConfig(
	ctx context.Context,
	req *types.QARequest,
	agentConfig *types.AgentConfig,
	globalAgentLLMCallTimeoutSeconds int,
	generationRunID string,
	sectionOrdinal int,
	totalSections int,
	sectionTitle string,
	budget DocumentGenerationBudget,
	evidenceTopK int,
	requestMaxCompletionTokens int,
	localKnowledgeUsed bool,
) {
	if req == nil || req.Session == nil {
		return
	}
	timeoutResolution := resolveDocumentGenerationCallTimeoutObservability(req, agentConfig, globalAgentLLMCallTimeoutSeconds, budget)
	logger.Infof(ctx,
		"[LongDocument] section config, session_id: %s, message_id: %s, run_id: %s, section_current: %d, section_total: %d, section_title: %s, budget_source: %s, budget_section_tokens: %d, budget_continuation_tokens: %d, request_max_completion_tokens: %d, evidence_top_k: %d, section_timeout_seconds: %d, budget_section_timeout_seconds: %d, agent_llm_call_timeout_seconds: %d, global_agent_llm_call_timeout_seconds: %d, effective_section_timeout_seconds: %d, effective_timeout_source: %s, local_knowledge_used: %t",
		req.Session.ID,
		strings.TrimSpace(req.AssistantMessageID),
		strings.TrimSpace(generationRunID),
		sectionOrdinal,
		totalSections,
		strings.TrimSpace(sectionTitle),
		strings.TrimSpace(budget.Source),
		budget.SectionMaxCompletionTokens,
		budget.ContinuationMaxCompletionTokens,
		requestMaxCompletionTokens,
		evidenceTopK,
		budget.SectionCallTimeoutSeconds,
		timeoutResolution.BudgetSectionTimeoutSeconds,
		timeoutResolution.AgentLLMCallTimeoutSeconds,
		timeoutResolution.GlobalAgentLLMCallTimeoutSeconds,
		timeoutResolution.EffectiveSectionTimeoutSeconds,
		timeoutResolution.EffectiveTimeoutSource,
		localKnowledgeUsed,
	)
}

type documentGenerationCallTimeoutObservability struct {
	BudgetSectionTimeoutSeconds      int
	AgentLLMCallTimeoutSeconds       int
	GlobalAgentLLMCallTimeoutSeconds int
	EffectiveSectionTimeoutSeconds   int
	EffectiveTimeoutSource           string
}

func globalDocumentGenerationLLMCallTimeoutSeconds(cfg *config.Config) int {
	if cfg == nil || cfg.Agent == nil || cfg.Agent.LLMCallTimeout <= 0 {
		return 0
	}
	return cfg.Agent.LLMCallTimeout
}

func resolveDocumentGenerationCallTimeoutSeconds(budget DocumentGenerationBudget, configuredAgentLLMCallTimeoutSeconds int) int {
	timeoutSeconds := budget.SectionCallTimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = documentGenerationDefaultLLMTimeoutSeconds
	}
	if configuredAgentLLMCallTimeoutSeconds > 0 && configuredAgentLLMCallTimeoutSeconds > timeoutSeconds {
		timeoutSeconds = configuredAgentLLMCallTimeoutSeconds
	}
	return timeoutSeconds
}

func resolveDocumentGenerationCallTimeoutObservability(
	req *types.QARequest,
	agentConfig *types.AgentConfig,
	globalAgentLLMCallTimeoutSeconds int,
	budget DocumentGenerationBudget,
) documentGenerationCallTimeoutObservability {
	resolution := documentGenerationCallTimeoutObservability{
		BudgetSectionTimeoutSeconds:      budget.SectionCallTimeoutSeconds,
		GlobalAgentLLMCallTimeoutSeconds: max(globalAgentLLMCallTimeoutSeconds, 0),
		EffectiveTimeoutSource:           "budget",
	}
	if req != nil && req.CustomAgent != nil && req.CustomAgent.Config.LLMCallTimeout > 0 {
		resolution.AgentLLMCallTimeoutSeconds = req.CustomAgent.Config.LLMCallTimeout
	}
	configuredAgentLLMCallTimeoutSeconds := 0
	if agentConfig != nil && agentConfig.LLMCallTimeout > 0 {
		configuredAgentLLMCallTimeoutSeconds = agentConfig.LLMCallTimeout
	}
	resolution.EffectiveSectionTimeoutSeconds = resolveDocumentGenerationCallTimeoutSeconds(budget, configuredAgentLLMCallTimeoutSeconds)
	if budget.SectionCallTimeoutSeconds <= 0 {
		resolution.EffectiveTimeoutSource = "default"
	}
	if configuredAgentLLMCallTimeoutSeconds > 0 && resolution.EffectiveSectionTimeoutSeconds == configuredAgentLLMCallTimeoutSeconds && configuredAgentLLMCallTimeoutSeconds > max(budget.SectionCallTimeoutSeconds, 0) {
		switch {
		case resolution.AgentLLMCallTimeoutSeconds > 0 && configuredAgentLLMCallTimeoutSeconds == resolution.AgentLLMCallTimeoutSeconds:
			resolution.EffectiveTimeoutSource = "agent_config"
		case resolution.GlobalAgentLLMCallTimeoutSeconds > 0 && configuredAgentLLMCallTimeoutSeconds == resolution.GlobalAgentLLMCallTimeoutSeconds:
			resolution.EffectiveTimeoutSource = "global_agent"
		default:
			resolution.EffectiveTimeoutSource = "agent_config"
		}
	}
	return resolution
}

func logFullDocumentCompletionSummary(
	ctx context.Context,
	req *types.QARequest,
	completionStatus string,
	finishReason string,
	failureReason string,
	documentGenerationStatus string,
	agentSteps types.AgentSteps,
	extra map[string]interface{},
	startTime time.Time,
	autoContinueNext *bool,
) {
	if req == nil || req.Session == nil {
		return
	}
	outlineTitle, outlineSectionCount := extractFullDocumentCompletionOutlineSummary(extra)
	runtimeSectionCount, runtimeBudgetAdjusted := extractFullDocumentRuntimeFeedbackSummary(extra)
	observability := buildFullDocumentObservabilitySummary(extra)
	localKnowledgeUsed := false
	if extra != nil {
		if value, ok := extra["local_knowledge_used"].(bool); ok {
			localKnowledgeUsed = value
		}
	}
	autoContinue := false
	if autoContinueNext != nil {
		autoContinue = *autoContinueNext
	}
	logger.Infof(ctx,
		"[LongDocument] completion summary, session_id: %s, message_id: %s, completion_status: %s, finish_reason: %s, failure_reason: %s, document_generation_status: %s, total_duration_ms: %d, total_steps: %d, outline_title: %s, outline_sections: %d, runtime_sections: %d, runtime_budget_adjusted: %t, local_knowledge_used: %t, auto_continue_next: %t, artifact_id: %s, generation_run_id: %s, quality_issues: %v, budget_source: %s, outline_tokens: %d, section_tokens: %d, continuation_tokens: %d, outline_top_k: %d, section_top_k: %d, continuation_top_k: %d",
		req.Session.ID,
		strings.TrimSpace(req.AssistantMessageID),
		strings.TrimSpace(completionStatus),
		strings.TrimSpace(finishReason),
		strings.TrimSpace(failureReason),
		strings.TrimSpace(documentGenerationStatus),
		time.Since(startTime).Milliseconds(),
		len(agentSteps),
		outlineTitle,
		outlineSectionCount,
		runtimeSectionCount,
		runtimeBudgetAdjusted,
		localKnowledgeUsed,
		autoContinue,
		observability.ArtifactID,
		observability.GenerationRunID,
		observability.QualityIssues,
		observability.BudgetSource,
		observability.OutlineMaxTokens,
		observability.SectionMaxTokens,
		observability.ContinuationMaxTokens,
		observability.OutlineEvidenceTopK,
		observability.SectionEvidenceTopK,
		observability.ContinuationEvidenceTopK,
	)
}

func extractFullDocumentCompletionOutlineSummary(extra map[string]interface{}) (string, int) {
	if len(extra) == 0 {
		return "", 0
	}
	rawOutline, ok := extra["outline"]
	if !ok || rawOutline == nil {
		return "", 0
	}
	switch outline := rawOutline.(type) {
	case dedicatedFullDocumentOutline:
		return strings.TrimSpace(outline.Title), len(outline.Sections)
	case map[string]interface{}:
		title, _ := outline["title"].(string)
		return strings.TrimSpace(title), len(extractOutlineSectionsFromInterfaces(outline["sections"]))
	default:
		return "", 0
	}
}

func extractFullDocumentRuntimeFeedbackSummary(extra map[string]interface{}) (int, bool) {
	if len(extra) == 0 {
		return 0, false
	}
	rawFeedback, ok := extra["runtime_feedback"]
	if !ok || rawFeedback == nil {
		return 0, false
	}
	feedback, ok := rawFeedback.(map[string]interface{})
	if !ok {
		return 0, false
	}
	sectionCount := 0
	if rawCount, ok := feedback["section_count"]; ok {
		switch value := rawCount.(type) {
		case int:
			sectionCount = value
		case int32:
			sectionCount = int(value)
		case int64:
			sectionCount = int(value)
		case float64:
			sectionCount = int(value)
		}
	}
	budgetAdjusted, _ := feedback["budget_adjusted"].(bool)
	return sectionCount, budgetAdjusted
}

func extractOutlineSectionsFromInterfaces(raw interface{}) []string {
	appendSection := func(sections []string, value interface{}) []string {
		switch section := value.(type) {
		case string:
			section = strings.TrimSpace(section)
			if section == "" {
				return sections
			}
			return append(sections, section)
		case map[string]interface{}:
			heading, _ := section["heading"].(string)
			title, _ := section["title"].(string)
			resolved := strings.TrimSpace(firstNonEmptyString(heading, title))
			if resolved == "" {
				return sections
			}
			return append(sections, resolved)
		default:
			return sections
		}
	}

	switch values := raw.(type) {
	case []interface{}:
		sections := make([]string, 0, len(values))
		for _, value := range values {
			sections = appendSection(sections, value)
		}
		return sections
	case []map[string]interface{}:
		sections := make([]string, 0, len(values))
		for _, value := range values {
			sections = appendSection(sections, value)
		}
		return sections
	default:
		return nil
	}
}

func filterEmptyOutlineSections(sections []string) []string {
	filtered := make([]string, 0, len(sections))
	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}
		filtered = append(filtered, section)
	}
	return filtered
}

func fullDocumentAutoContinueNext(status string, finishReason string, failureReason string, autoContinueRound int) *bool {
	next := shouldAutoContinueFullDocument(status, finishReason, failureReason, autoContinueRound)
	return &next
}

func normalizeFullDocumentRetryBudgetOutcome(finishReason string, failureReason string, autoContinueRound int) (string, string) {
	if autoContinueRound < 1 {
		return finishReason, failureReason
	}
	if !isRecoverableFullDocumentContinuationFailure(finishReason, failureReason) {
		return finishReason, failureReason
	}
	return "llm_timeout_retry_exhausted", failureReason
}

func isRecoverableFullDocumentContinuationFailure(finishReason string, failureReason string) bool {
	trimmedFinishReason := strings.TrimSpace(finishReason)
	trimmedFailureReason := strings.TrimSpace(failureReason)
	if trimmedFailureReason != "llm_timeout" {
		return false
	}
	switch trimmedFinishReason {
	case "section_generation_timeout", "section_generation_error", "llm_timeout":
		return true
	default:
		return false
	}
}

func shouldAutoContinueFullDocument(status string, finishReason string, failureReason string, autoContinueRound int) bool {
	if types.NormalizeChatDocumentGenerationStatus(status) != types.ChatDocumentGenerationStatusContinuing {
		return false
	}
	if autoContinueRound >= 1 && isRecoverableFullDocumentContinuationFailure(finishReason, failureReason) {
		return false
	}
	if strings.TrimSpace(failureReason) != "" && !isRecoverableFullDocumentContinuationFailure(finishReason, failureReason) {
		return false
	}
	switch strings.TrimSpace(finishReason) {
	case "", "stop", "length", "section_batch_limit", "continuation_pending", "section_generation_timeout", "section_generation_error":
		return true
	default:
		return false
	}
}

func fullDocumentAutoContinueReason(status string, finishReason string, failureReason string, autoContinueRound int) string {
	switch types.NormalizeChatDocumentGenerationStatus(status) {
	case types.ChatDocumentGenerationStatusCompleted:
		return "document_complete_marker"
	case types.ChatDocumentGenerationStatusBlocked:
		return "document_generation_blocked"
	case types.ChatDocumentGenerationStatusNeedsReview:
		return "document_generation_needs_review"
	default:
		if !shouldAutoContinueFullDocument(status, finishReason, failureReason, autoContinueRound) {
			if strings.TrimSpace(finishReason) == "llm_timeout_retry_exhausted" {
				return finishReason
			}
			return firstNonEmptyString(failureReason, finishReason)
		}
		return ""
	}
}

func (s *sessionService) runKnowledgeGroundedDocumentContinuationPath(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
	chatModel chat.Chat,
	agentConfig *types.AgentConfig,
) error {
	if req == nil || req.Session == nil || eventBus == nil || req.BaseArtifact == nil {
		return errors.New("knowledge grounded document continuation request is incomplete")
	}
	if generationRun, err := s.loadKnowledgeGroundedGenerationRun(ctx, req.GenerationRunID); err != nil {
		logger.Errorf(ctx, "Failed to load knowledge grounded generation run: %v", err)
		return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, classifyDocumentEditError(err), err)
	} else if generationRun != nil {
		outline := unmarshalGenerationRunOutline(generationRun.OutlineJSON)
		searchTargets := resolveGenerationRunSearchTargets(agentConfig, generationRun)
		if len(outline.Sections) > 0 && len(searchTargets) == 0 {
			return s.runDedicatedDocumentContinuationRunPath(ctx, req, eventBus, chatModel, agentConfig, generationRun, outline)
		}
		if len(outline.Sections) > 0 && len(searchTargets) > 0 {
			return s.runKnowledgeGroundedDocumentContinuationRunPath(ctx, req, eventBus, chatModel, agentConfig, generationRun, outline, searchTargets)
		}
	}
	language := types.LanguageNameFromContext(ctx)
	startTime := time.Now()
	progressEventID := generateEventID("document-continuation-progress")
	progress := newFullDocumentProgressReporter(ctx, req, eventBus, progressEventID)
	defer progress.Close()
	budget := s.resolveDocumentGenerationBudget(ctx, req, chatModel, buildDocumentProfile(req, agentConfig, true, 0), progress)
	currentBudget := budget
	runtimeFeedback := documentGenerationRuntimeFeedback{}
	baseOutline := extractFullDocumentOutlineFromArtifact(req.BaseArtifact)
	progress.UpdateStage("planning", "正在解析续写上下文与知识库范围。")

	evidence, err := retrieveKnowledgeGroundedFullDocumentEvidence(ctx, s.knowledgeBaseService, s.cfg, agentConfig.SearchTargets, buildKnowledgeGroundedContinuationQueries(req), currentBudget.ContinuationEvidenceTopK, func(current int, total int, query string) {
		progress.SetQueryProgress(current, total)
		progress.UpdateStage("retrieving", fmt.Sprintf("正在检索本地知识库（%d/%d）：%s", current, total, query))
	})
	progress.ClearQueryProgress()
	if err != nil {
		logger.Errorf(ctx, "Knowledge grounded continuation evidence retrieval failed: %v", err)
		return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, classifyDocumentEditError(err), err)
	}
	if len(evidence.Items) == 0 {
		message := "本地知识库未检索到足够证据，无法继续生成剩余文档内容。"
		if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, generateEventID("document-continuation"), message, true, types.MessageCompletionStatusPartial, "local_knowledge_not_found"); err != nil {
			logger.Errorf(ctx, "Failed to emit knowledge grounded continuation blocked answer chunk: %v", err)
		}
		extra := withDocumentGenerationBudgetExtra(map[string]interface{}{
			"local_knowledge_used": true,
			"evidence_refs":        []interface{}{},
			"effective_kb_ids":     evidence.ScopeKBIDs,
			"evidence_queries":     evidence.Queries,
		}, budget)
		if strings.TrimSpace(baseOutline.Title) != "" || len(baseOutline.Sections) > 0 {
			extra["outline"] = dedicatedFullDocumentOutlineData(baseOutline)
			extra["outline_role"] = "base_document"
		}
		return emitFullDocumentCompletion(ctx, req, eventBus, message, types.MessageCompletionStatusPartial, "local_knowledge_not_found", "local_knowledge_not_found", types.ChatDocumentGenerationStatusBlocked, progress.AgentSteps(), nil, extra, startTime)
	}
	progress.UpdateStage("generating", fmt.Sprintf("已命中 %d 条本地知识证据，正在继续生成剩余文档内容。", len(evidence.Items)))

	messages := buildKnowledgeGroundedDocumentContinuationMessages(req, language, req.BaseArtifact, evidence)
	temperature := 0.2
	if agentConfig != nil {
		temperature = agentConfig.Temperature
	}
	streamCtx, cancelStream := withDocumentGenerationCallTimeout(ctx, agentConfig, currentBudget)
	stream, err := chatModel.ChatStream(streamCtx, messages, fullDocumentContinuationChatOptions(currentBudget, temperature))
	if err != nil {
		cancelStream()
		logger.Errorf(ctx, "Knowledge grounded continuation stream start failed: %v", err)
		return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, classifyDocumentEditError(err), err)
	}

	answerEventID := generateEventID("document-continuation")
	var finalContent strings.Builder
	completionStatus := types.MessageCompletionStatusPartial
	finishReason := "continuation_pending"
	failureReason := ""
	documentGenerationStatus := types.ChatDocumentGenerationStatusContinuing
	modelThinkingEventID := generateEventID("document-continuation-model-thinking")
	streamResult := consumeFullDocumentSectionStream(streamCtx, stream, progress, "剩余文档内容", func(content string, done bool) {
		emitFullDocumentModelThinking(ctx, req, eventBus, modelThinkingEventID, "generating", content, done, 0, 0, "")
	}, func(content string) {
		finalContent.WriteString(content)
		if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, content, false, types.MessageCompletionStatusCompleted, "streaming"); err != nil {
			logger.Errorf(ctx, "Failed to emit knowledge grounded continuation chunk: %v", err)
		}
	})
	cancelStream()
	sectionFeedback := buildDocumentRuntimeSectionFeedback("remaining_document", len(evidence.Items), streamResult)
	adjustedBudget, adjustmentReasons, recommendedSectionLimit := adjustDocumentGenerationBudgetWithRuntimeFeedback(currentBudget, sectionFeedback)
	runtimeFeedback = appendDocumentGenerationRuntimeFeedback(runtimeFeedback, sectionFeedback, adjustmentReasons, recommendedSectionLimit)
	if len(adjustmentReasons) > 0 {
		logger.Infof(ctx, "[DocumentBudget][RuntimeFeedback] section=%s reasons=%v next_section_tokens=%d next_section_top_k=%d next_timeout=%d recommended_section_limit=%d", strings.TrimSpace(sectionFeedback.Section), adjustmentReasons, adjustedBudget.SectionMaxCompletionTokens, adjustedBudget.SectionEvidenceTopK, adjustedBudget.SectionCallTimeoutSeconds, runtimeFeedback.RecommendedSectionLimitPerRun)
	}
	currentBudget = adjustedBudget
	if streamResult.completionStatus != types.MessageCompletionStatusCompleted {
		completionStatus = streamResult.completionStatus
		finishReason = streamResult.finishReason
		failureReason = streamResult.failureReason
		documentGenerationStatus = streamResult.documentGenerationState
	} else {
		finishReason = streamResult.finishReason
		progress.UpdateStage("finalizing", "当前轮剩余内容已生成，正在判断是否完成全文。")
	}

	finalAnswer, qualityIssues, qualityOK := applyGeneratedDocumentMarkdownQualityGate(ctx, chatModel, agentConfig, currentBudget, finalContent.String())
	finalAnswer = strings.TrimSpace(finalAnswer)
	if finalAnswer == "" {
		return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, "empty_document_edit_completion", errors.New("knowledge grounded continuation completed without visible content"))
	}
	if strings.Contains(finalAnswer, types.ChatDocumentCompletionMarker) {
		completionStatus = types.MessageCompletionStatusCompleted
		if finishReason == "continuation_pending" {
			finishReason = "stop"
		}
		documentGenerationStatus = types.ChatDocumentGenerationStatusCompleted
	}
	if !qualityOK {
		if completionStatus == types.MessageCompletionStatusCompleted {
			finishReason = "stop"
			failureReason = ""
			documentGenerationStatus = types.ChatDocumentGenerationStatusNeedsReview
		} else if strings.TrimSpace(documentGenerationStatus) == "" {
			documentGenerationStatus = types.ChatDocumentGenerationStatusContinuing
		}
	}
	if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, "", true, completionStatus, finishReason); err != nil {
		logger.Errorf(ctx, "Failed to emit knowledge grounded continuation done chunk: %v", err)
	}
	refs := buildKnowledgeGroundedEvidenceRefs(evidence)
	extra := map[string]interface{}{
		"local_knowledge_used": true,
		"evidence_refs":        refs,
		"effective_kb_ids":     evidence.ScopeKBIDs,
		"evidence_queries":     evidence.Queries,
	}
	if strings.TrimSpace(baseOutline.Title) != "" || len(baseOutline.Sections) > 0 {
		extra["outline"] = dedicatedFullDocumentOutlineData(baseOutline)
		extra["outline_role"] = "base_document"
	}
	extra = withDocumentGenerationBudgetExtra(extra, currentBudget)
	extra = withDocumentGenerationRuntimeFeedbackExtra(extra, runtimeFeedback)
	if normalizedQualityIssues := uniqueNonEmptyStrings(qualityIssues); len(normalizedQualityIssues) > 0 {
		extra["quality_issues"] = normalizedQualityIssues
	}
	if err := emitFullDocumentCompletion(ctx, req, eventBus, finalAnswer, completionStatus, finishReason, failureReason, documentGenerationStatus, progress.AgentSteps(), refs, extra, startTime); err != nil {
		logger.Errorf(ctx, "Failed to emit knowledge grounded continuation completion event: %v", err)
		return err
	}
	return nil
}

func (s *sessionService) runDedicatedDocumentContinuationRunPath(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
	chatModel chat.Chat,
	agentConfig *types.AgentConfig,
	generationRun *types.ChatDocumentGenerationRun,
	outline dedicatedFullDocumentOutline,
) error {
	language := types.LanguageNameFromContext(ctx)
	startTime := time.Now()
	progressEventID := generateEventID("document-continuation-progress")
	progress := newFullDocumentProgressReporter(ctx, req, eventBus, progressEventID)
	defer progress.Close()
	budget := s.resolveDocumentGenerationBudget(ctx, req, chatModel, buildDocumentProfile(req, agentConfig, false, len(outline.Sections)), progress)
	budget = mergePersistedDocumentGenerationBudget(budget, unmarshalGenerationRunBudget(generationRun.BudgetJSON))
	runtimeFeedback := unmarshalGenerationRunRuntimeFeedback(generationRun.RuntimeFeedbackJSON)

	completedSections := filterOutlineSections(outline, unmarshalGenerationRunStringSlice(generationRun.CompletedSectionsJSON))
	remainingSections := remainingOutlineSections(outline, completedSections)
	if len(remainingSections) == 0 {
		if err := s.updateKnowledgeGroundedGenerationRun(ctx, generationRun, outline, completedSections, budget, runtimeFeedback, types.ChatDocumentGenerationStatusCompleted, types.MessageCompletionStatusCompleted, generationRun.AutoContinueRound); err != nil {
			logger.Errorf(ctx, "Failed to finalize dedicated generation run without remaining sections: %v", err)
		}
		persistedRuntimeFeedback := unmarshalGenerationRunRuntimeFeedback(generationRun.RuntimeFeedbackJSON)
		extra := buildKnowledgeGroundedGenerationRunExtra(generationRun, outline, nil, budget, persistedRuntimeFeedback)
		extra["local_knowledge_used"] = false
		return emitFullDocumentCompletion(ctx, req, eventBus, types.ChatDocumentCompletionMarker, types.MessageCompletionStatusCompleted, "stop", "", types.ChatDocumentGenerationStatusCompleted, progress.AgentSteps(), nil, extra, startTime)
	}
	if generationRun.MaxRounds > 0 && generationRun.AutoContinueRound >= generationRun.MaxRounds {
		notice := "已达到自动续写轮次上限，剩余章节请人工继续生成或重新发起任务。"
		if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, generateEventID("document-continuation"), notice, true, types.MessageCompletionStatusPartial, "auto_continue_round_limit"); err != nil {
			logger.Errorf(ctx, "Failed to emit dedicated run continuation round limit notice: %v", err)
		}
		if err := s.updateKnowledgeGroundedGenerationRun(ctx, generationRun, outline, completedSections, budget, runtimeFeedback, types.ChatDocumentGenerationStatusBlocked, types.MessageCompletionStatusPartial, generationRun.AutoContinueRound); err != nil {
			logger.Errorf(ctx, "Failed to update dedicated generation run after round limit: %v", err)
		}
		persistedRuntimeFeedback := unmarshalGenerationRunRuntimeFeedback(generationRun.RuntimeFeedbackJSON)
		extra := buildKnowledgeGroundedGenerationRunExtra(generationRun, outline, nil, budget, persistedRuntimeFeedback)
		extra["local_knowledge_used"] = false
		return emitFullDocumentCompletion(ctx, req, eventBus, notice, types.MessageCompletionStatusPartial, "auto_continue_round_limit", "auto_continue_round_limit", types.ChatDocumentGenerationStatusBlocked, progress.AgentSteps(), nil, extra, startTime)
	}

	currentBudget := budget
	sectionLimit := effectiveDocumentGenerationSectionLimit(runtimeFeedback)
	if sectionLimit <= 0 || sectionLimit > len(remainingSections) {
		sectionLimit = len(remainingSections)
	}
	nextSections := remainingSections[:sectionLimit]
	progress.UpdateStage("generating", fmt.Sprintf("正在基于已保存的大纲继续生成后续章节，当前续写 %d 个章节。", len(nextSections)))

	answerEventID := generateEventID("document-continuation")
	var deltaContent strings.Builder
	var completedSnapshot strings.Builder
	baseSnapshot := strings.TrimSpace(req.BaseArtifact.ContentSnapshot)
	if baseSnapshot != "" {
		completedSnapshot.WriteString(baseSnapshot)
		if !strings.HasSuffix(baseSnapshot, "\n\n") {
			completedSnapshot.WriteString("\n\n")
		}
	}

	temperature := 0.2
	if agentConfig != nil {
		temperature = agentConfig.Temperature
	}
	completionStatus := types.MessageCompletionStatusCompleted
	finishReason := "stop"
	failureReason := ""
	documentGenerationStatus := types.ChatDocumentGenerationStatusCompleted
	goalQuery := strings.TrimSpace(generationRun.OriginalQuery)
	if goalQuery == "" {
		goalQuery = strings.TrimSpace(req.AutoContinueOriginalQuery)
	}
	if goalQuery == "" {
		goalQuery = strings.TrimSpace(req.Query)
	}
	sectionReq := *req
	sectionReq.Query = goalQuery
	newlyCompletedSections := append([]string(nil), completedSections...)
	qualityIssues := make([]string, 0, len(nextSections))
	executedRound := req.AutoContinueRound + 1
	if executedRound <= 0 {
		executedRound = generationRun.AutoContinueRound + 1
	}
	generationRunID := ""
	if generationRun != nil {
		generationRunID = generationRun.ID
	}

	for index, section := range nextSections {
		select {
		case <-ctx.Done():
			completionStatus = types.MessageCompletionStatusCancelled
			finishReason = "cancelled"
			failureReason = "cancelled"
			documentGenerationStatus = types.ChatDocumentGenerationStatusBlocked
			goto finalize
		default:
		}

		sectionOrdinal := len(newlyCompletedSections) + 1
		totalSections := len(outline.Sections)
		currentSection, found := findDedicatedFullDocumentSection(outline, section)
		if !found {
			currentSection, _ = normalizeDedicatedFullDocumentSection(dedicatedFullDocumentSection{Number: sectionOrdinal, Title: strings.TrimSpace(section)}, sectionOrdinal)
		}
		progress.SetSectionProgress(sectionOrdinal, totalSections, strings.TrimSpace(currentSection.Title))
		progress.UpdateStage("generating", fmt.Sprintf("正在生成第 %d/%d 章“%s”。", sectionOrdinal, totalSections, strings.TrimSpace(currentSection.Title)))
		logFullDocumentSectionConfig(ctx, req, agentConfig, globalDocumentGenerationLLMCallTimeoutSeconds(s.cfg), generationRunID, sectionOrdinal, totalSections, currentSection.Title, currentBudget, 0, currentBudget.SectionMaxCompletionTokens, false)

		completedSummary := buildFullDocumentRollingSummary(outline, newlyCompletedSections, completedSnapshot.String())
		headingChunk := formatDedicatedFullDocumentSectionHeadingMarkdown(currentSection) + "\n\n"
		deltaContent.WriteString(headingChunk)
		completedSnapshot.WriteString(headingChunk)
		if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, headingChunk, false, types.MessageCompletionStatusCompleted, "streaming"); err != nil {
			logger.Errorf(ctx, "Failed to emit dedicated run continuation heading chunk: %v", err)
		}

		sectionMessages := buildDedicatedFullDocumentSectionMessages(&sectionReq, language, outline.Title, outline, currentSection, completedSummary)
		sectionCtx, cancelSection := withDocumentGenerationCallTimeout(ctx, agentConfig, currentBudget)
		sectionStream, streamErr := chatModel.ChatStream(sectionCtx, sectionMessages, fullDocumentSectionChatOptions(currentBudget, temperature))
		if streamErr != nil {
			cancelSection()
			if deltaContent.Len() > 0 {
				completionStatus = types.MessageCompletionStatusPartial
				finishReason = "section_generation_error"
				failureReason = classifyDocumentEditError(streamErr)
				documentGenerationStatus = types.ChatDocumentGenerationStatusContinuing
				break
			}
			return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, classifyDocumentEditError(streamErr), streamErr)
		}

		var rawSectionContent strings.Builder
		modelThinkingEventID := generateEventID("document-section-model-thinking")
		streamResult := consumeFullDocumentSectionStream(sectionCtx, sectionStream, progress, fmt.Sprintf("第 %d/%d 章“%s”", sectionOrdinal, totalSections, strings.TrimSpace(currentSection.Title)), func(content string, done bool) {
			emitFullDocumentModelThinking(ctx, req, eventBus, modelThinkingEventID, "generating", content, done, sectionOrdinal, totalSections, strings.TrimSpace(currentSection.Title))
		}, func(content string) {
			rawSectionContent.WriteString(content)
			if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, content, false, types.MessageCompletionStatusCompleted, "streaming"); err != nil {
				logger.Errorf(ctx, "Failed to emit dedicated run continuation section chunk: %v", err)
			}
		})
		cancelSection()
		sectionFeedback := buildDocumentRuntimeSectionFeedback(currentSection.Title, -1, streamResult)
		adjustedBudget, adjustmentReasons, recommendedSectionLimit := adjustDocumentGenerationBudgetWithRuntimeFeedback(currentBudget, sectionFeedback)
		runtimeFeedback = appendDocumentGenerationRuntimeFeedback(runtimeFeedback, sectionFeedback, adjustmentReasons, recommendedSectionLimit)
		if len(adjustmentReasons) > 0 {
			logger.Infof(ctx, "[DocumentBudget][RuntimeFeedback] section=%s reasons=%v next_section_tokens=%d next_section_top_k=%d next_timeout=%d recommended_section_limit=%d", strings.TrimSpace(currentSection.Title), adjustmentReasons, adjustedBudget.SectionMaxCompletionTokens, adjustedBudget.SectionEvidenceTopK, adjustedBudget.SectionCallTimeoutSeconds, runtimeFeedback.RecommendedSectionLimitPerRun)
		}
		currentBudget = adjustedBudget
		if streamResult.completionStatus != types.MessageCompletionStatusCompleted {
			partialSectionContent, sectionSignals := normalizeGeneratedMarkdown(rawSectionContent.String())
			if strings.TrimSpace(partialSectionContent) != "" {
				deltaContent.WriteString(partialSectionContent)
				completedSnapshot.WriteString(partialSectionContent)
			}
			qualityIssues = append(qualityIssues, sectionSignals...)
			completionStatus = streamResult.completionStatus
			finishReason = streamResult.finishReason
			failureReason = streamResult.failureReason
			documentGenerationStatus = streamResult.documentGenerationState
		} else if streamResult.sectionDone {
			normalizedSectionContent, sectionSignals, qualityOK := applyGeneratedSectionMarkdownQualityGate(ctx, chatModel, agentConfig, currentBudget, currentSection, rawSectionContent.String())
			qualityIssues = append(qualityIssues, sectionSignals...)
			if strings.TrimSpace(normalizedSectionContent) != "" {
				deltaContent.WriteString(normalizedSectionContent)
				completedSnapshot.WriteString(normalizedSectionContent)
			}
			if !qualityOK {
				logger.Warnf(ctx, "dedicated_run_continuation_section_markdown_quality_warning: session_id=%s, message_id=%s, section=%s, issues=%v", req.Session.ID, req.AssistantMessageID, strings.TrimSpace(currentSection.Title), uniqueNonEmptyStrings(sectionSignals))
				if completionStatus == types.MessageCompletionStatusCompleted {
					documentGenerationStatus = types.ChatDocumentGenerationStatusNeedsReview
				}
			}
			if !strings.HasSuffix(completedSnapshot.String(), "\n\n") {
				completedSnapshot.WriteString("\n\n")
				if !strings.HasSuffix(deltaContent.String(), "\n\n") {
					deltaContent.WriteString("\n\n")
					if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, "\n\n", false, types.MessageCompletionStatusCompleted, "streaming"); err != nil {
						logger.Errorf(ctx, "Failed to emit dedicated run continuation spacing chunk: %v", err)
					}
				}
			}
		}

		if completionStatus != types.MessageCompletionStatusCompleted {
			break
		}
		newlyCompletedSections = append(newlyCompletedSections, strings.TrimSpace(currentSection.Title))
		if sectionOrdinal < totalSections {
			progress.UpdateStage("generating", fmt.Sprintf("第 %d/%d 章“%s”已完成，继续生成下一章。", sectionOrdinal, totalSections, strings.TrimSpace(currentSection.Title)))
		} else {
			progress.UpdateStage("finalizing", fmt.Sprintf("第 %d/%d 章“%s”已完成，正在收尾完整文档。", sectionOrdinal, totalSections, strings.TrimSpace(currentSection.Title)))
		}
		if index == sectionLimit-1 && len(remainingSections) > sectionLimit {
			completionStatus = types.MessageCompletionStatusPartial
			finishReason = "section_batch_limit"
			documentGenerationStatus = types.ChatDocumentGenerationStatusContinuing
		}
	}

finalize:
	if completionStatus == types.MessageCompletionStatusCompleted && len(remainingSections) > sectionLimit {
		completionStatus = types.MessageCompletionStatusPartial
		finishReason = "section_batch_limit"
		documentGenerationStatus = types.ChatDocumentGenerationStatusContinuing
	}
	integrityContent := ""
	integrityContent, completionStatus, finishReason, failureReason, documentGenerationStatus, qualityIssues = applyFullDocumentArtifactQualityGate(
		ctx,
		chatModel,
		agentConfig,
		currentBudget,
		completedSnapshot.String(),
		completionStatus,
		finishReason,
		failureReason,
		documentGenerationStatus,
		qualityIssues,
	)
	completionStatus, finishReason, failureReason, documentGenerationStatus, qualityIssues = applyArtifactFirstFullDocumentIntegrityOutcome(outline, newlyCompletedSections, integrityContent, completionStatus, finishReason, failureReason, documentGenerationStatus, qualityIssues)
	if completionStatus == types.MessageCompletionStatusCompleted && strings.TrimSpace(documentGenerationStatus) == "" {
		documentGenerationStatus = types.ChatDocumentGenerationStatusCompleted
	}
	if completionStatus == types.MessageCompletionStatusPartial && strings.TrimSpace(documentGenerationStatus) == "" {
		documentGenerationStatus = types.ChatDocumentGenerationStatusContinuing
	}
	finalAnswer := strings.TrimSpace(deltaContent.String())
	if finalAnswer == "" && completionStatus != types.MessageCompletionStatusCompleted {
		return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, "empty_document_edit_completion", errors.New("dedicated continuation run completed without visible content"))
	}
	if err := s.updateKnowledgeGroundedGenerationRun(ctx, generationRun, outline, newlyCompletedSections, currentBudget, runtimeFeedback, documentGenerationStatus, completionStatus, executedRound); err != nil {
		logger.Errorf(ctx, "Failed to update dedicated generation run: %v", err)
	}
	persistedRuntimeFeedback := unmarshalGenerationRunRuntimeFeedback(generationRun.RuntimeFeedbackJSON)
	extra := buildKnowledgeGroundedGenerationRunExtra(generationRun, outline, nil, currentBudget, persistedRuntimeFeedback)
	extra["local_knowledge_used"] = false
	extra["completed_sections"] = uniqueNonEmptyStrings(newlyCompletedSections)
	if normalizedQualityIssues := uniqueNonEmptyStrings(qualityIssues); len(normalizedQualityIssues) > 0 {
		extra["quality_issues"] = normalizedQualityIssues
	}
	if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, "", true, completionStatus, finishReason); err != nil {
		logger.Errorf(ctx, "Failed to emit dedicated run continuation done chunk: %v", err)
	}
	if completionStatus == types.MessageCompletionStatusCompleted && finalAnswer == "" {
		finalAnswer = types.ChatDocumentCompletionMarker
	}
	return emitFullDocumentCompletion(ctx, req, eventBus, finalAnswer, completionStatus, finishReason, failureReason, documentGenerationStatus, progress.AgentSteps(), nil, extra, startTime)
}

func (s *sessionService) runKnowledgeGroundedDocumentContinuationRunPath(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
	chatModel chat.Chat,
	agentConfig *types.AgentConfig,
	generationRun *types.ChatDocumentGenerationRun,
	outline dedicatedFullDocumentOutline,
	searchTargets types.SearchTargets,
) error {
	language := types.LanguageNameFromContext(ctx)
	startTime := time.Now()
	progressEventID := generateEventID("document-continuation-progress")
	progress := newFullDocumentProgressReporter(ctx, req, eventBus, progressEventID)
	defer progress.Close()
	budget := s.resolveDocumentGenerationBudget(ctx, req, chatModel, buildDocumentProfile(req, agentConfig, true, len(outline.Sections)), progress)
	budget = mergePersistedDocumentGenerationBudget(budget, unmarshalGenerationRunBudget(generationRun.BudgetJSON))
	runtimeFeedback := unmarshalGenerationRunRuntimeFeedback(generationRun.RuntimeFeedbackJSON)

	completedSections := filterOutlineSections(outline, unmarshalGenerationRunStringSlice(generationRun.CompletedSectionsJSON))
	remainingSections := remainingOutlineSections(outline, completedSections)
	if len(remainingSections) == 0 {
		if err := s.updateKnowledgeGroundedGenerationRun(ctx, generationRun, outline, completedSections, budget, runtimeFeedback, types.ChatDocumentGenerationStatusCompleted, types.MessageCompletionStatusCompleted, generationRun.AutoContinueRound); err != nil {
			logger.Errorf(ctx, "Failed to finalize knowledge grounded generation run without remaining sections: %v", err)
		}
		persistedRuntimeFeedback := unmarshalGenerationRunRuntimeFeedback(generationRun.RuntimeFeedbackJSON)
		extra := buildKnowledgeGroundedGenerationRunExtra(generationRun, outline, unmarshalGenerationRunStringSlice(generationRun.EffectiveKBIDsJSON), budget, persistedRuntimeFeedback)
		extra["local_knowledge_used"] = true
		return emitFullDocumentCompletion(ctx, req, eventBus, types.ChatDocumentCompletionMarker, types.MessageCompletionStatusCompleted, "stop", "", types.ChatDocumentGenerationStatusCompleted, progress.AgentSteps(), nil, extra, startTime)
	}
	if generationRun.MaxRounds > 0 && generationRun.AutoContinueRound >= generationRun.MaxRounds {
		notice := "已达到自动续写轮次上限，剩余章节请人工继续生成或重新发起任务。"
		if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, generateEventID("document-continuation"), notice, true, types.MessageCompletionStatusPartial, "auto_continue_round_limit"); err != nil {
			logger.Errorf(ctx, "Failed to emit knowledge grounded run continuation round limit notice: %v", err)
		}
		if err := s.updateKnowledgeGroundedGenerationRun(ctx, generationRun, outline, completedSections, budget, runtimeFeedback, types.ChatDocumentGenerationStatusBlocked, types.MessageCompletionStatusPartial, generationRun.AutoContinueRound); err != nil {
			logger.Errorf(ctx, "Failed to update knowledge grounded generation run after round limit: %v", err)
		}
		persistedRuntimeFeedback := unmarshalGenerationRunRuntimeFeedback(generationRun.RuntimeFeedbackJSON)
		extra := buildKnowledgeGroundedGenerationRunExtra(generationRun, outline, unmarshalGenerationRunStringSlice(generationRun.EffectiveKBIDsJSON), budget, persistedRuntimeFeedback)
		extra["local_knowledge_used"] = true
		return emitFullDocumentCompletion(ctx, req, eventBus, notice, types.MessageCompletionStatusPartial, "auto_continue_round_limit", "auto_continue_round_limit", types.ChatDocumentGenerationStatusBlocked, progress.AgentSteps(), nil, extra, startTime)
	}

	currentBudget := budget
	sectionLimit := effectiveDocumentGenerationSectionLimit(runtimeFeedback)
	if sectionLimit <= 0 || sectionLimit > len(remainingSections) {
		sectionLimit = len(remainingSections)
	}
	nextSections := remainingSections[:sectionLimit]
	progress.UpdateStage("generating", fmt.Sprintf("正在基于已保存的大纲和本地知识继续生成后续章节，当前续写 %d 个章节。", len(nextSections)))

	answerEventID := generateEventID("document-continuation")
	var deltaContent strings.Builder
	var completedSnapshot strings.Builder
	baseSnapshot := strings.TrimSpace(req.BaseArtifact.ContentSnapshot)
	if baseSnapshot != "" {
		completedSnapshot.WriteString(baseSnapshot)
		if !strings.HasSuffix(baseSnapshot, "\n\n") {
			completedSnapshot.WriteString("\n\n")
		}
	}

	temperature := 0.2
	if agentConfig != nil {
		temperature = agentConfig.Temperature
	}
	completionStatus := types.MessageCompletionStatusCompleted
	finishReason := "stop"
	failureReason := ""
	documentGenerationStatus := types.ChatDocumentGenerationStatusCompleted
	goalQuery := strings.TrimSpace(generationRun.OriginalQuery)
	if goalQuery == "" {
		goalQuery = strings.TrimSpace(req.AutoContinueOriginalQuery)
	}
	if goalQuery == "" {
		goalQuery = strings.TrimSpace(req.Query)
	}
	sectionReq := *req
	sectionReq.Query = goalQuery
	evidencePacks := make([]knowledgeGroundedEvidencePack, 0, len(nextSections))
	newlyCompletedSections := append([]string(nil), completedSections...)
	qualityIssues := make([]string, 0, len(nextSections))
	executedRound := req.AutoContinueRound + 1
	if executedRound <= 0 {
		executedRound = generationRun.AutoContinueRound + 1
	}
	generationRunID := ""
	if generationRun != nil {
		generationRunID = generationRun.ID
	}

	for index, section := range nextSections {
		select {
		case <-ctx.Done():
			completionStatus = types.MessageCompletionStatusCancelled
			finishReason = "cancelled"
			failureReason = "cancelled"
			documentGenerationStatus = types.ChatDocumentGenerationStatusBlocked
			goto finalize
		default:
		}

		sectionOrdinal := len(newlyCompletedSections) + 1
		totalSections := len(outline.Sections)
		currentSection, found := findDedicatedFullDocumentSection(outline, section)
		if !found {
			currentSection, _ = normalizeDedicatedFullDocumentSection(dedicatedFullDocumentSection{Number: sectionOrdinal, Title: strings.TrimSpace(section)}, sectionOrdinal)
		}
		progress.SetSectionProgress(sectionOrdinal, totalSections, strings.TrimSpace(currentSection.Title))
		progress.ClearQueryProgress()
		progress.UpdateStage("retrieving", fmt.Sprintf("正在检索第 %d/%d 章“%s”的本地证据。", sectionOrdinal, totalSections, strings.TrimSpace(currentSection.Title)))
		logFullDocumentSectionConfig(ctx, req, agentConfig, globalDocumentGenerationLLMCallTimeoutSeconds(s.cfg), generationRunID, sectionOrdinal, totalSections, currentSection.Title, currentBudget, currentBudget.SectionEvidenceTopK, currentBudget.SectionMaxCompletionTokens, true)
		sectionEvidence, evidenceErr := retrieveKnowledgeGroundedFullDocumentEvidence(ctx, s.knowledgeBaseService, s.cfg, searchTargets, buildKnowledgeGroundedSectionQueriesForGoal(goalQuery, outline.Title, currentSection.Title), currentBudget.SectionEvidenceTopK, func(current int, total int, query string) {
			progress.SetQueryProgress(current, total)
			progress.UpdateStage("retrieving", fmt.Sprintf("正在检索第 %d/%d 章“%s”的本地证据（%d/%d）：%s", sectionOrdinal, totalSections, strings.TrimSpace(currentSection.Title), current, total, query))
		})
		progress.ClearQueryProgress()
		if evidenceErr != nil {
			logger.Errorf(ctx, "Knowledge grounded run continuation evidence retrieval failed: %v", evidenceErr)
			notice := fmt.Sprintf("> 本地知识库未检索到足够证据，无法继续生成章节“%s”。\n\n", strings.TrimSpace(currentSection.Title))
			deltaContent.WriteString(notice)
			if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, notice, false, types.MessageCompletionStatusPartial, "local_knowledge_not_found"); err != nil {
				logger.Errorf(ctx, "Failed to emit knowledge grounded run continuation evidence error notice: %v", err)
			}
			completionStatus = types.MessageCompletionStatusPartial
			finishReason = "local_knowledge_not_found"
			failureReason = classifyDocumentEditError(evidenceErr)
			documentGenerationStatus = types.ChatDocumentGenerationStatusBlocked
			break
		}
		if len(sectionEvidence.Items) == 0 {
			notice := fmt.Sprintf("> 本地知识库未检索到足够证据，无法继续生成章节“%s”。\n\n", strings.TrimSpace(currentSection.Title))
			deltaContent.WriteString(notice)
			if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, notice, false, types.MessageCompletionStatusPartial, "local_knowledge_not_found"); err != nil {
				logger.Errorf(ctx, "Failed to emit knowledge grounded run continuation missing evidence notice: %v", err)
			}
			completionStatus = types.MessageCompletionStatusPartial
			finishReason = "local_knowledge_not_found"
			failureReason = "local_knowledge_not_found"
			documentGenerationStatus = types.ChatDocumentGenerationStatusBlocked
			break
		}
		sectionEvidence.SectionHeading = strings.TrimSpace(currentSection.Heading)
		evidencePacks = append(evidencePacks, sectionEvidence)
		progress.SetSectionProgress(sectionOrdinal, totalSections, strings.TrimSpace(currentSection.Title))
		progress.UpdateStage("generating", fmt.Sprintf("已检索到 %d 条证据，正在生成第 %d/%d 章“%s”。", len(sectionEvidence.Items), sectionOrdinal, totalSections, strings.TrimSpace(currentSection.Title)))

		completedSummary := buildFullDocumentRollingSummary(outline, newlyCompletedSections, completedSnapshot.String())
		headingChunk := formatDedicatedFullDocumentSectionHeadingMarkdown(currentSection) + "\n\n"
		deltaContent.WriteString(headingChunk)
		completedSnapshot.WriteString(headingChunk)
		if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, headingChunk, false, types.MessageCompletionStatusCompleted, "streaming"); err != nil {
			logger.Errorf(ctx, "Failed to emit knowledge grounded run continuation heading chunk: %v", err)
		}

		sectionMessages := buildKnowledgeGroundedFullDocumentSectionMessages(&sectionReq, language, outline.Title, outline, currentSection, completedSummary, sectionEvidence)
		sectionCtx, cancelSection := withDocumentGenerationCallTimeout(ctx, agentConfig, currentBudget)
		sectionStream, streamErr := chatModel.ChatStream(sectionCtx, sectionMessages, fullDocumentSectionChatOptions(currentBudget, temperature))
		if streamErr != nil {
			cancelSection()
			if deltaContent.Len() > 0 {
				completionStatus = types.MessageCompletionStatusPartial
				finishReason = "section_generation_error"
				failureReason = classifyDocumentEditError(streamErr)
				documentGenerationStatus = types.ChatDocumentGenerationStatusContinuing
				break
			}
			return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, classifyDocumentEditError(streamErr), streamErr)
		}

		var rawSectionContent strings.Builder
		modelThinkingEventID := generateEventID("document-section-model-thinking")
		streamResult := consumeFullDocumentSectionStream(sectionCtx, sectionStream, progress, fmt.Sprintf("第 %d/%d 章“%s”", sectionOrdinal, totalSections, strings.TrimSpace(currentSection.Title)), func(content string, done bool) {
			emitFullDocumentModelThinking(ctx, req, eventBus, modelThinkingEventID, "generating", content, done, sectionOrdinal, totalSections, strings.TrimSpace(currentSection.Title))
		}, func(content string) {
			rawSectionContent.WriteString(content)
			if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, content, false, types.MessageCompletionStatusCompleted, "streaming"); err != nil {
				logger.Errorf(ctx, "Failed to emit knowledge grounded run continuation section chunk: %v", err)
			}
		})
		cancelSection()
		sectionFeedback := buildDocumentRuntimeSectionFeedback(currentSection.Title, len(sectionEvidence.Items), streamResult)
		adjustedBudget, adjustmentReasons, recommendedSectionLimit := adjustDocumentGenerationBudgetWithRuntimeFeedback(currentBudget, sectionFeedback)
		runtimeFeedback = appendDocumentGenerationRuntimeFeedback(runtimeFeedback, sectionFeedback, adjustmentReasons, recommendedSectionLimit)
		if len(adjustmentReasons) > 0 {
			logger.Infof(ctx, "[DocumentBudget][RuntimeFeedback] section=%s reasons=%v next_section_tokens=%d next_section_top_k=%d next_timeout=%d recommended_section_limit=%d", strings.TrimSpace(currentSection.Title), adjustmentReasons, adjustedBudget.SectionMaxCompletionTokens, adjustedBudget.SectionEvidenceTopK, adjustedBudget.SectionCallTimeoutSeconds, runtimeFeedback.RecommendedSectionLimitPerRun)
		}
		currentBudget = adjustedBudget
		if streamResult.completionStatus != types.MessageCompletionStatusCompleted {
			partialSectionContent, sectionSignals := normalizeGeneratedMarkdown(rawSectionContent.String())
			if strings.TrimSpace(partialSectionContent) != "" {
				deltaContent.WriteString(partialSectionContent)
				completedSnapshot.WriteString(partialSectionContent)
			}
			qualityIssues = append(qualityIssues, sectionSignals...)
			completionStatus = streamResult.completionStatus
			finishReason = streamResult.finishReason
			failureReason = streamResult.failureReason
			documentGenerationStatus = streamResult.documentGenerationState
		} else if streamResult.sectionDone {
			normalizedSectionContent, sectionSignals, qualityOK := applyGeneratedSectionMarkdownQualityGate(ctx, chatModel, agentConfig, currentBudget, currentSection, rawSectionContent.String())
			qualityIssues = append(qualityIssues, sectionSignals...)
			if strings.TrimSpace(normalizedSectionContent) != "" {
				deltaContent.WriteString(normalizedSectionContent)
				completedSnapshot.WriteString(normalizedSectionContent)
			}
			if !qualityOK {
				logger.Warnf(ctx, "knowledge_grounded_run_continuation_section_markdown_quality_warning: session_id=%s, message_id=%s, section=%s, issues=%v", req.Session.ID, req.AssistantMessageID, strings.TrimSpace(currentSection.Title), uniqueNonEmptyStrings(sectionSignals))
				if completionStatus == types.MessageCompletionStatusCompleted {
					documentGenerationStatus = types.ChatDocumentGenerationStatusNeedsReview
				}
			}
			if !strings.HasSuffix(completedSnapshot.String(), "\n\n") {
				completedSnapshot.WriteString("\n\n")
				if !strings.HasSuffix(deltaContent.String(), "\n\n") {
					deltaContent.WriteString("\n\n")
					if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, "\n\n", false, types.MessageCompletionStatusCompleted, "streaming"); err != nil {
						logger.Errorf(ctx, "Failed to emit knowledge grounded run continuation spacing chunk: %v", err)
					}
				}
			}
		}

		if completionStatus != types.MessageCompletionStatusCompleted {
			break
		}
		newlyCompletedSections = append(newlyCompletedSections, strings.TrimSpace(currentSection.Title))
		if sectionOrdinal < totalSections {
			progress.UpdateStage("generating", fmt.Sprintf("第 %d/%d 章“%s”已完成，继续生成下一章。", sectionOrdinal, totalSections, strings.TrimSpace(currentSection.Title)))
		} else {
			progress.UpdateStage("finalizing", fmt.Sprintf("第 %d/%d 章“%s”已完成，正在收尾完整文档。", sectionOrdinal, totalSections, strings.TrimSpace(currentSection.Title)))
		}
		if index == sectionLimit-1 && len(remainingSections) > sectionLimit {
			completionStatus = types.MessageCompletionStatusPartial
			finishReason = "section_batch_limit"
			documentGenerationStatus = types.ChatDocumentGenerationStatusContinuing
		}
	}

finalize:
	if completionStatus == types.MessageCompletionStatusCompleted && len(remainingSections) > sectionLimit {
		completionStatus = types.MessageCompletionStatusPartial
		finishReason = "section_batch_limit"
		documentGenerationStatus = types.ChatDocumentGenerationStatusContinuing
	}
	integrityContent := ""
	integrityContent, completionStatus, finishReason, failureReason, documentGenerationStatus, qualityIssues = applyFullDocumentArtifactQualityGate(
		ctx,
		chatModel,
		agentConfig,
		currentBudget,
		completedSnapshot.String(),
		completionStatus,
		finishReason,
		failureReason,
		documentGenerationStatus,
		qualityIssues,
	)
	completionStatus, finishReason, failureReason, documentGenerationStatus, qualityIssues = applyArtifactFirstFullDocumentIntegrityOutcome(outline, newlyCompletedSections, integrityContent, completionStatus, finishReason, failureReason, documentGenerationStatus, qualityIssues)
	if completionStatus == types.MessageCompletionStatusCompleted && strings.TrimSpace(documentGenerationStatus) == "" {
		documentGenerationStatus = types.ChatDocumentGenerationStatusCompleted
	}
	if completionStatus == types.MessageCompletionStatusPartial && strings.TrimSpace(documentGenerationStatus) == "" {
		documentGenerationStatus = types.ChatDocumentGenerationStatusContinuing
	}
	finalAnswer := strings.TrimSpace(deltaContent.String())
	if finalAnswer == "" && completionStatus != types.MessageCompletionStatusCompleted {
		return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, "empty_document_edit_completion", errors.New("knowledge grounded continuation run completed without visible content"))
	}
	if err := s.updateKnowledgeGroundedGenerationRun(ctx, generationRun, outline, newlyCompletedSections, currentBudget, runtimeFeedback, documentGenerationStatus, completionStatus, executedRound); err != nil {
		logger.Errorf(ctx, "Failed to update knowledge grounded generation run: %v", err)
	}
	refs := buildKnowledgeGroundedEvidenceRefs(evidencePacks...)
	persistedRuntimeFeedback := unmarshalGenerationRunRuntimeFeedback(generationRun.RuntimeFeedbackJSON)
	extra := buildKnowledgeGroundedGenerationRunExtra(generationRun, outline, unmarshalGenerationRunStringSlice(generationRun.EffectiveKBIDsJSON), currentBudget, persistedRuntimeFeedback)
	extra["local_knowledge_used"] = true
	extra["evidence_refs"] = refs
	extra["evidence_queries"] = flattenKnowledgeGroundedEvidenceQueries(evidencePacks...)
	if normalizedQualityIssues := uniqueNonEmptyStrings(qualityIssues); len(normalizedQualityIssues) > 0 {
		extra["quality_issues"] = normalizedQualityIssues
	}
	if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, "", true, completionStatus, finishReason); err != nil {
		logger.Errorf(ctx, "Failed to emit knowledge grounded run continuation done chunk: %v", err)
	}
	if completionStatus == types.MessageCompletionStatusCompleted && finalAnswer == "" {
		finalAnswer = types.ChatDocumentCompletionMarker
	}
	return emitFullDocumentCompletion(ctx, req, eventBus, finalAnswer, completionStatus, finishReason, failureReason, documentGenerationStatus, progress.AgentSteps(), refs, extra, startTime)
}

func (s *sessionService) runKnowledgeGroundedFullDocumentGenerationPath(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
	chatModel chat.Chat,
	agentConfig *types.AgentConfig,
) error {
	if req == nil || req.Session == nil || eventBus == nil {
		return errors.New("knowledge grounded full document generation request is incomplete")
	}
	language := types.LanguageNameFromContext(ctx)
	startTime := time.Now()
	progressEventID := generateEventID("document-outline-progress")
	progress := newFullDocumentProgressReporter(ctx, req, eventBus, progressEventID)
	defer progress.Close()
	budget := s.resolveDocumentGenerationBudget(ctx, req, chatModel, buildDocumentProfile(req, agentConfig, true, 0), progress)
	currentBudget := budget
	runtimeFeedback := documentGenerationRuntimeFeedback{}
	progress.UpdateStage("planning", "正在解析文档目标与知识库范围。")

	outlineEvidence, err := retrieveKnowledgeGroundedFullDocumentEvidence(ctx, s.knowledgeBaseService, s.cfg, agentConfig.SearchTargets, buildKnowledgeGroundedOutlineQueries(req), currentBudget.OutlineEvidenceTopK, func(current int, total int, query string) {
		progress.SetQueryProgress(current, total)
		progress.UpdateStage("retrieving", fmt.Sprintf("正在检索本地知识库（%d/%d）：%s", current, total, query))
	})
	progress.ClearQueryProgress()
	if err != nil {
		logger.Errorf(ctx, "Knowledge grounded outline evidence retrieval failed: %v", err)
		return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, classifyDocumentEditError(err), err)
	}
	if len(outlineEvidence.Items) == 0 {
		message := "本地知识库未检索到足够内容，无法生成完整技术方案。请先补充相关知识库资料，或缩小文档范围后重试。"
		if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, generateEventID("document-full"), message, true, types.MessageCompletionStatusPartial, "local_knowledge_not_found"); err != nil {
			logger.Errorf(ctx, "Failed to emit knowledge grounded blocked answer chunk: %v", err)
		}
		return emitFullDocumentCompletion(ctx, req, eventBus, message, types.MessageCompletionStatusPartial, "local_knowledge_not_found", "local_knowledge_not_found", types.ChatDocumentGenerationStatusBlocked, progress.AgentSteps(), nil, withDocumentGenerationBudgetExtra(map[string]interface{}{
			"local_knowledge_used": true,
			"evidence_refs":        []interface{}{},
			"effective_kb_ids":     outlineEvidence.ScopeKBIDs,
			"evidence_queries":     outlineEvidence.Queries,
		}, budget), startTime)
	}
	progress.UpdateStage("planning", fmt.Sprintf("已命中 %d 条本地知识证据，正在规划完整大纲。", len(outlineEvidence.Items)))

	outlineMessages := buildKnowledgeGroundedFullDocumentOutlineMessages(req, language, outlineEvidence)
	outline, err := generateValidatedFullDocumentOutline(ctx, req, eventBus, chatModel, outlineMessages, fullDocumentOutlineChatOptions(currentBudget, 0.2), progress, "正在规划完整文档大纲", req.Query)
	if err != nil {
		if strings.TrimSpace(err.Error()) == "outline_parse_failed" {
			return emitFullDocumentOutlineParseFailure(ctx, req, eventBus, "生成的大纲结构异常，无法继续生成完整文档，请稍后重试。", withDocumentGenerationBudgetExtra(map[string]interface{}{
				"local_knowledge_used": true,
				"evidence_refs":        []interface{}{},
				"effective_kb_ids":     outlineEvidence.ScopeKBIDs,
				"evidence_queries":     outlineEvidence.Queries,
			}, budget), startTime)
		}
		logger.Errorf(ctx, "Knowledge grounded full document outline generation failed: %v", err)
		return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, classifyDocumentEditError(err), err)
	}
	progress.PublishOutline(outline)
	generationRun, err := s.createKnowledgeGroundedGenerationRun(ctx, req, chatModel, outline, currentBudget, outlineEvidence.ScopeKBIDs)
	if err != nil {
		logger.Errorf(ctx, "Failed to create knowledge grounded generation run: %v", err)
		return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, classifyDocumentEditError(err), err)
	}

	sectionsToGenerate := fullDocumentSectionsForInitialRun(outline)
	temperature := 0.2
	if agentConfig != nil {
		temperature = agentConfig.Temperature
	}
	progress.UpdateStage("generating", fmt.Sprintf("已检索 %d 条本地知识证据，并规划 %d 个章节，将按大纲连续生成全部章节。", len(outlineEvidence.Items), len(outline.Sections)))
	generationRunID := ""
	if generationRun != nil {
		generationRunID = generationRun.ID
	}

	answerEventID := generateEventID("document-full")
	var finalContent strings.Builder
	titleChunk := "# " + strings.TrimSpace(outline.Title) + "\n\n"
	finalContent.WriteString(titleChunk)
	if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, titleChunk, false, types.MessageCompletionStatusCompleted, "streaming"); err != nil {
		logger.Errorf(ctx, "Failed to emit knowledge grounded title chunk: %v", err)
	}

	completionStatus := types.MessageCompletionStatusCompleted
	finishReason := "stop"
	failureReason := ""
	documentGenerationStatus := types.ChatDocumentGenerationStatusCompleted
	evidencePacks := []knowledgeGroundedEvidencePack{outlineEvidence}
	completedSections := make([]string, 0, len(sectionsToGenerate))
	qualityIssues := make([]string, 0, len(sectionsToGenerate))

	for index, section := range sectionsToGenerate {
		select {
		case <-ctx.Done():
			completionStatus = types.MessageCompletionStatusCancelled
			finishReason = "cancelled"
			failureReason = "cancelled"
			documentGenerationStatus = types.ChatDocumentGenerationStatusBlocked
			goto finalize
		default:
		}

		sectionNumber := index + 1
		currentSection, found := findDedicatedFullDocumentSection(outline, section)
		if !found {
			currentSection, _ = normalizeDedicatedFullDocumentSection(dedicatedFullDocumentSection{Number: sectionNumber, Title: strings.TrimSpace(section)}, sectionNumber)
		}
		progress.SetSectionProgress(sectionNumber, len(sectionsToGenerate), strings.TrimSpace(currentSection.Title))
		progress.ClearQueryProgress()
		progress.UpdateStage("retrieving", fmt.Sprintf("正在检索第 %d/%d 章“%s”的本地证据。", sectionNumber, len(sectionsToGenerate), strings.TrimSpace(currentSection.Title)))
		sectionEvidence, evidenceErr := retrieveKnowledgeGroundedFullDocumentEvidence(ctx, s.knowledgeBaseService, s.cfg, agentConfig.SearchTargets, buildKnowledgeGroundedSectionQueries(req, outline.Title, currentSection.Title), currentBudget.SectionEvidenceTopK, func(current int, total int, query string) {
			progress.SetQueryProgress(current, total)
			progress.UpdateStage("retrieving", fmt.Sprintf("正在检索第 %d/%d 章“%s”的本地证据（%d/%d）：%s", sectionNumber, len(sectionsToGenerate), strings.TrimSpace(currentSection.Title), current, total, query))
		})
		progress.ClearQueryProgress()
		if evidenceErr != nil {
			logger.Errorf(ctx, "Knowledge grounded section evidence retrieval failed: %v", evidenceErr)
			notice := fmt.Sprintf("> 本地知识库未检索到足够证据，无法继续生成章节“%s”。\n\n", strings.TrimSpace(currentSection.Title))
			finalContent.WriteString(notice)
			if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, notice, false, types.MessageCompletionStatusPartial, "local_knowledge_not_found"); err != nil {
				logger.Errorf(ctx, "Failed to emit knowledge grounded evidence error notice: %v", err)
			}
			completionStatus = types.MessageCompletionStatusPartial
			finishReason = "local_knowledge_not_found"
			failureReason = classifyDocumentEditError(evidenceErr)
			documentGenerationStatus = types.ChatDocumentGenerationStatusBlocked
			break
		}
		if len(sectionEvidence.Items) == 0 {
			notice := fmt.Sprintf("> 本地知识库未检索到足够证据，无法继续生成章节“%s”。\n\n", strings.TrimSpace(currentSection.Title))
			finalContent.WriteString(notice)
			if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, notice, false, types.MessageCompletionStatusPartial, "local_knowledge_not_found"); err != nil {
				logger.Errorf(ctx, "Failed to emit knowledge grounded missing evidence notice: %v", err)
			}
			completionStatus = types.MessageCompletionStatusPartial
			finishReason = "local_knowledge_not_found"
			failureReason = "local_knowledge_not_found"
			documentGenerationStatus = types.ChatDocumentGenerationStatusBlocked
			break
		}
		sectionEvidence.SectionHeading = strings.TrimSpace(currentSection.Heading)
		evidencePacks = append(evidencePacks, sectionEvidence)
		progress.SetSectionProgress(sectionNumber, len(sectionsToGenerate), strings.TrimSpace(currentSection.Title))
		progress.UpdateStage("generating", fmt.Sprintf("已检索到 %d 条证据，正在生成第 %d/%d 章“%s”。", len(sectionEvidence.Items), sectionNumber, len(sectionsToGenerate), strings.TrimSpace(currentSection.Title)))
		logFullDocumentSectionConfig(ctx, req, agentConfig, globalDocumentGenerationLLMCallTimeoutSeconds(s.cfg), generationRunID, sectionNumber, len(sectionsToGenerate), currentSection.Title, currentBudget, currentBudget.SectionEvidenceTopK, currentBudget.SectionMaxCompletionTokens, true)

		completedSummary := buildFullDocumentRollingSummary(outline, completedSections, finalContent.String())
		headingChunk := formatDedicatedFullDocumentSectionHeadingMarkdown(currentSection) + "\n\n"
		finalContent.WriteString(headingChunk)
		if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, headingChunk, false, types.MessageCompletionStatusCompleted, "streaming"); err != nil {
			logger.Errorf(ctx, "Failed to emit knowledge grounded heading chunk: %v", err)
		}

		sectionMessages := buildKnowledgeGroundedFullDocumentSectionMessages(req, language, outline.Title, outline, currentSection, completedSummary, sectionEvidence)
		sectionCtx, cancelSection := withDocumentGenerationCallTimeout(ctx, agentConfig, currentBudget)
		sectionStream, streamErr := chatModel.ChatStream(sectionCtx, sectionMessages, fullDocumentSectionChatOptions(currentBudget, temperature))
		if streamErr != nil {
			cancelSection()
			if finalContent.Len() > len([]rune(titleChunk)) {
				completionStatus = types.MessageCompletionStatusPartial
				finishReason = "section_generation_error"
				failureReason = classifyDocumentEditError(streamErr)
				documentGenerationStatus = types.ChatDocumentGenerationStatusContinuing
				break
			}
			return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, classifyDocumentEditError(streamErr), streamErr)
		}

		var rawSectionContent strings.Builder
		modelThinkingEventID := generateEventID("document-section-model-thinking")
		streamResult := consumeFullDocumentSectionStream(sectionCtx, sectionStream, progress, fmt.Sprintf("第 %d/%d 章“%s”", sectionNumber, len(sectionsToGenerate), strings.TrimSpace(currentSection.Title)), func(content string, done bool) {
			emitFullDocumentModelThinking(ctx, req, eventBus, modelThinkingEventID, "generating", content, done, sectionNumber, len(sectionsToGenerate), strings.TrimSpace(currentSection.Title))
		}, func(content string) {
			rawSectionContent.WriteString(content)
			if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, content, false, types.MessageCompletionStatusCompleted, "streaming"); err != nil {
				logger.Errorf(ctx, "Failed to emit knowledge grounded section chunk: %v", err)
			}
		})
		cancelSection()
		sectionFeedback := buildDocumentRuntimeSectionFeedback(currentSection.Title, len(sectionEvidence.Items), streamResult)
		adjustedBudget, adjustmentReasons, recommendedSectionLimit := adjustDocumentGenerationBudgetWithRuntimeFeedback(currentBudget, sectionFeedback)
		runtimeFeedback = appendDocumentGenerationRuntimeFeedback(runtimeFeedback, sectionFeedback, adjustmentReasons, recommendedSectionLimit)
		if len(adjustmentReasons) > 0 {
			logger.Infof(ctx, "[DocumentBudget][RuntimeFeedback] section=%s reasons=%v next_section_tokens=%d next_section_top_k=%d next_timeout=%d recommended_section_limit=%d", strings.TrimSpace(currentSection.Title), adjustmentReasons, adjustedBudget.SectionMaxCompletionTokens, adjustedBudget.SectionEvidenceTopK, adjustedBudget.SectionCallTimeoutSeconds, runtimeFeedback.RecommendedSectionLimitPerRun)
		}
		currentBudget = adjustedBudget
		if streamResult.completionStatus != types.MessageCompletionStatusCompleted {
			partialSectionContent, sectionSignals := normalizeGeneratedMarkdown(rawSectionContent.String())
			if strings.TrimSpace(partialSectionContent) != "" {
				finalContent.WriteString(partialSectionContent)
			}
			qualityIssues = append(qualityIssues, sectionSignals...)
			completionStatus = streamResult.completionStatus
			finishReason = streamResult.finishReason
			failureReason = streamResult.failureReason
			documentGenerationStatus = streamResult.documentGenerationState
		} else if streamResult.sectionDone {
			normalizedSectionContent, sectionSignals, qualityOK := applyGeneratedSectionMarkdownQualityGate(ctx, chatModel, agentConfig, currentBudget, currentSection, rawSectionContent.String())
			qualityIssues = append(qualityIssues, sectionSignals...)
			if strings.TrimSpace(normalizedSectionContent) != "" {
				finalContent.WriteString(normalizedSectionContent)
			}
			if !qualityOK {
				logger.Warnf(ctx, "knowledge_grounded_full_document_section_markdown_quality_warning: session_id=%s, message_id=%s, section=%s, issues=%v", req.Session.ID, req.AssistantMessageID, strings.TrimSpace(currentSection.Title), uniqueNonEmptyStrings(sectionSignals))
				if completionStatus == types.MessageCompletionStatusCompleted {
					documentGenerationStatus = types.ChatDocumentGenerationStatusNeedsReview
				}
			}
			if !strings.HasSuffix(finalContent.String(), "\n\n") {
				finalContent.WriteString("\n\n")
				if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, "\n\n", false, types.MessageCompletionStatusCompleted, "streaming"); err != nil {
					logger.Errorf(ctx, "Failed to emit knowledge grounded spacing chunk: %v", err)
				}
			}
		}

		if completionStatus != types.MessageCompletionStatusCompleted {
			break
		}
		completedSections = append(completedSections, strings.TrimSpace(currentSection.Title))
		if sectionNumber < len(sectionsToGenerate) {
			progress.UpdateStage("generating", fmt.Sprintf("第 %d/%d 章“%s”已完成，继续生成下一章。", sectionNumber, len(sectionsToGenerate), strings.TrimSpace(currentSection.Title)))
		} else {
			progress.UpdateStage("finalizing", fmt.Sprintf("第 %d/%d 章“%s”已完成，正在收尾完整文档。", sectionNumber, len(sectionsToGenerate), strings.TrimSpace(currentSection.Title)))
		}
	}

finalize:
	finalAnswer := ""
	finalAnswer, completionStatus, finishReason, failureReason, documentGenerationStatus, qualityIssues = applyFullDocumentArtifactQualityGate(
		ctx,
		chatModel,
		agentConfig,
		currentBudget,
		finalContent.String(),
		completionStatus,
		finishReason,
		failureReason,
		documentGenerationStatus,
		qualityIssues,
	)
	completionStatus, finishReason, failureReason, documentGenerationStatus, qualityIssues = applyArtifactFirstFullDocumentIntegrityOutcome(outline, completedSections, finalAnswer, completionStatus, finishReason, failureReason, documentGenerationStatus, qualityIssues)
	if completionStatus == types.MessageCompletionStatusCompleted && strings.TrimSpace(documentGenerationStatus) == "" {
		documentGenerationStatus = types.ChatDocumentGenerationStatusCompleted
	}
	if completionStatus == types.MessageCompletionStatusPartial && strings.TrimSpace(documentGenerationStatus) == "" {
		documentGenerationStatus = types.ChatDocumentGenerationStatusContinuing
	}
	if finalAnswer == "" {
		return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, "empty_document_edit_completion", errors.New("knowledge grounded full document generation completed without visible content"))
	}
	if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, "", true, completionStatus, finishReason); err != nil {
		logger.Errorf(ctx, "Failed to emit knowledge grounded full document done chunk: %v", err)
	}
	refs := buildKnowledgeGroundedEvidenceRefs(evidencePacks...)
	if err := s.updateKnowledgeGroundedGenerationRun(ctx, generationRun, outline, completedSections, currentBudget, runtimeFeedback, documentGenerationStatus, completionStatus, 1); err != nil {
		logger.Errorf(ctx, "Failed to update knowledge grounded generation run: %v", err)
	}
	persistedRuntimeFeedback := runtimeFeedback
	if generationRun != nil {
		persistedRuntimeFeedback = unmarshalGenerationRunRuntimeFeedback(generationRun.RuntimeFeedbackJSON)
	}
	extra := buildKnowledgeGroundedGenerationRunExtra(generationRun, outline, outlineEvidence.ScopeKBIDs, currentBudget, persistedRuntimeFeedback)
	extra["local_knowledge_used"] = true
	extra["evidence_refs"] = refs
	extra["evidence_queries"] = outlineEvidence.Queries
	extra["completed_sections"] = uniqueNonEmptyStrings(completedSections)
	if normalizedQualityIssues := uniqueNonEmptyStrings(qualityIssues); len(normalizedQualityIssues) > 0 {
		extra["quality_issues"] = normalizedQualityIssues
	}
	if err := emitFullDocumentCompletion(ctx, req, eventBus, finalAnswer, completionStatus, finishReason, failureReason, documentGenerationStatus, progress.AgentSteps(), refs, extra, startTime); err != nil {
		logger.Errorf(ctx, "Failed to emit knowledge grounded full document completion event: %v", err)
		return err
	}
	return nil
}

func (s *sessionService) runDedicatedFullDocumentGenerationPath(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
	chatModel chat.Chat,
	agentConfig *types.AgentConfig,
) error {
	if req == nil || req.Session == nil || eventBus == nil {
		return errors.New("full document generation request is incomplete")
	}
	language := types.LanguageNameFromContext(ctx)
	startTime := time.Now()
	progressEventID := generateEventID("document-outline-progress")
	progress := newFullDocumentProgressReporter(ctx, req, eventBus, progressEventID)
	defer progress.Close()
	budget := s.resolveDocumentGenerationBudget(ctx, req, chatModel, buildDocumentProfile(req, agentConfig, false, 0), progress)
	currentBudget := budget
	runtimeFeedback := documentGenerationRuntimeFeedback{}
	outlineMessages := buildDedicatedFullDocumentOutlineMessages(req, language)
	progress.UpdateStage("planning", "正在规划完整文档大纲。")

	outline, err := generateValidatedFullDocumentOutline(ctx, req, eventBus, chatModel, outlineMessages, fullDocumentOutlineChatOptions(currentBudget, 0.2), progress, "正在规划完整文档大纲", req.Query)
	if err != nil {
		if strings.TrimSpace(err.Error()) == "outline_parse_failed" {
			return emitFullDocumentOutlineParseFailure(ctx, req, eventBus, "生成的大纲结构异常，无法继续生成完整文档，请稍后重试。", withDocumentGenerationBudgetExtra(nil, budget), startTime)
		}
		logger.Errorf(ctx, "Dedicated full document outline generation failed: %v", err)
		return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, classifyDocumentEditError(err), err)
	}
	progress.PublishOutline(outline)
	generationRun, err := s.createKnowledgeGroundedGenerationRun(ctx, req, chatModel, outline, currentBudget, nil)
	if err != nil {
		logger.Errorf(ctx, "Failed to create dedicated generation run: %v", err)
		return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, classifyDocumentEditError(err), err)
	}

	sectionsToGenerate := fullDocumentSectionsForInitialRun(outline)
	temperature := 0.2
	if agentConfig != nil {
		temperature = agentConfig.Temperature
	}
	progress.UpdateStage("generating", fmt.Sprintf("已规划 %d 个章节，将连续生成全部章节。", len(outline.Sections)))

	answerEventID := generateEventID("document-full")
	var finalContent strings.Builder
	titleChunk := "# " + strings.TrimSpace(outline.Title) + "\n\n"
	finalContent.WriteString(titleChunk)
	if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, titleChunk, false, types.MessageCompletionStatusCompleted, "streaming"); err != nil {
		logger.Errorf(ctx, "Failed to emit full document title chunk: %v", err)
	}

	completionStatus := types.MessageCompletionStatusCompleted
	finishReason := "stop"
	failureReason := ""
	documentGenerationStatus := types.ChatDocumentGenerationStatusCompleted
	completedSections := make([]string, 0, len(sectionsToGenerate))
	qualityIssues := make([]string, 0, len(sectionsToGenerate))

	for index, section := range sectionsToGenerate {
		select {
		case <-ctx.Done():
			completionStatus = types.MessageCompletionStatusCancelled
			finishReason = "cancelled"
			failureReason = "cancelled"
			documentGenerationStatus = types.ChatDocumentGenerationStatusBlocked
			goto finalize
		default:
		}

		sectionNumber := index + 1
		currentSection, found := findDedicatedFullDocumentSection(outline, section)
		if !found {
			currentSection, _ = normalizeDedicatedFullDocumentSection(dedicatedFullDocumentSection{Number: sectionNumber, Title: strings.TrimSpace(section)}, sectionNumber)
		}
		progress.SetSectionProgress(sectionNumber, len(sectionsToGenerate), strings.TrimSpace(currentSection.Title))
		progress.ClearQueryProgress()
		progress.UpdateStage("generating", fmt.Sprintf("正在生成第 %d/%d 章：%s", sectionNumber, len(sectionsToGenerate), strings.TrimSpace(currentSection.Title)))
		generationRunID := ""
		if generationRun != nil {
			generationRunID = generationRun.ID
		}
		logFullDocumentSectionConfig(ctx, req, agentConfig, globalDocumentGenerationLLMCallTimeoutSeconds(s.cfg), generationRunID, sectionNumber, len(sectionsToGenerate), currentSection.Title, currentBudget, 0, currentBudget.SectionMaxCompletionTokens, false)
		completedSummary := buildFullDocumentRollingSummary(outline, completedSections, finalContent.String())
		headingChunk := formatDedicatedFullDocumentSectionHeadingMarkdown(currentSection) + "\n\n"
		finalContent.WriteString(headingChunk)
		if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, headingChunk, false, types.MessageCompletionStatusCompleted, "streaming"); err != nil {
			logger.Errorf(ctx, "Failed to emit full document heading chunk: %v", err)
		}

		sectionMessages := buildDedicatedFullDocumentSectionMessages(req, language, outline.Title, outline, currentSection, completedSummary)
		sectionCtx, cancelSection := withDocumentGenerationCallTimeout(ctx, agentConfig, currentBudget)
		sectionStream, streamErr := chatModel.ChatStream(sectionCtx, sectionMessages, fullDocumentSectionChatOptions(currentBudget, temperature))
		if streamErr != nil {
			cancelSection()
			if finalContent.Len() > len([]rune(titleChunk)) {
				completionStatus = types.MessageCompletionStatusPartial
				finishReason = "section_generation_error"
				failureReason = classifyDocumentEditError(streamErr)
				documentGenerationStatus = types.ChatDocumentGenerationStatusContinuing
				break
			}
			return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, classifyDocumentEditError(streamErr), streamErr)
		}

		var rawSectionContent strings.Builder
		modelThinkingEventID := generateEventID("document-section-model-thinking")
		streamResult := consumeFullDocumentSectionStream(sectionCtx, sectionStream, progress, fmt.Sprintf("第 %d/%d 章“%s”", sectionNumber, len(sectionsToGenerate), strings.TrimSpace(currentSection.Title)), func(content string, done bool) {
			emitFullDocumentModelThinking(ctx, req, eventBus, modelThinkingEventID, "generating", content, done, sectionNumber, len(sectionsToGenerate), strings.TrimSpace(currentSection.Title))
		}, func(content string) {
			rawSectionContent.WriteString(content)
			if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, content, false, types.MessageCompletionStatusCompleted, "streaming"); err != nil {
				logger.Errorf(ctx, "Failed to emit full document section chunk: %v", err)
			}
		})
		cancelSection()
		sectionFeedback := buildDocumentRuntimeSectionFeedback(currentSection.Title, -1, streamResult)
		adjustedBudget, adjustmentReasons, recommendedSectionLimit := adjustDocumentGenerationBudgetWithRuntimeFeedback(currentBudget, sectionFeedback)
		runtimeFeedback = appendDocumentGenerationRuntimeFeedback(runtimeFeedback, sectionFeedback, adjustmentReasons, recommendedSectionLimit)
		if len(adjustmentReasons) > 0 {
			logger.Infof(ctx, "[DocumentBudget][RuntimeFeedback] section=%s reasons=%v next_section_tokens=%d next_section_top_k=%d next_timeout=%d recommended_section_limit=%d", strings.TrimSpace(currentSection.Title), adjustmentReasons, adjustedBudget.SectionMaxCompletionTokens, adjustedBudget.SectionEvidenceTopK, adjustedBudget.SectionCallTimeoutSeconds, runtimeFeedback.RecommendedSectionLimitPerRun)
		}
		currentBudget = adjustedBudget
		if streamResult.completionStatus != types.MessageCompletionStatusCompleted {
			partialSectionContent, sectionSignals := normalizeGeneratedMarkdown(rawSectionContent.String())
			if strings.TrimSpace(partialSectionContent) != "" {
				finalContent.WriteString(partialSectionContent)
			}
			qualityIssues = append(qualityIssues, sectionSignals...)
			completionStatus = streamResult.completionStatus
			finishReason = streamResult.finishReason
			failureReason = streamResult.failureReason
			documentGenerationStatus = streamResult.documentGenerationState
		} else if streamResult.sectionDone {
			normalizedSectionContent, sectionSignals, qualityOK := applyGeneratedSectionMarkdownQualityGate(ctx, chatModel, agentConfig, currentBudget, currentSection, rawSectionContent.String())
			qualityIssues = append(qualityIssues, sectionSignals...)
			if strings.TrimSpace(normalizedSectionContent) != "" {
				finalContent.WriteString(normalizedSectionContent)
			}
			if !qualityOK {
				logger.Warnf(ctx, "dedicated_full_document_section_markdown_quality_warning: session_id=%s, message_id=%s, section=%s, issues=%v", req.Session.ID, req.AssistantMessageID, strings.TrimSpace(currentSection.Title), uniqueNonEmptyStrings(sectionSignals))
				if completionStatus == types.MessageCompletionStatusCompleted {
					documentGenerationStatus = types.ChatDocumentGenerationStatusNeedsReview
				}
			}
			if !strings.HasSuffix(finalContent.String(), "\n\n") {
				finalContent.WriteString("\n\n")
				if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, "\n\n", false, types.MessageCompletionStatusCompleted, "streaming"); err != nil {
					logger.Errorf(ctx, "Failed to emit full document spacing chunk: %v", err)
				}
			}
		}

		if completionStatus != types.MessageCompletionStatusCompleted {
			break
		}
		completedSections = append(completedSections, strings.TrimSpace(currentSection.Title))
		if sectionNumber < len(sectionsToGenerate) {
			progress.UpdateStage("generating", fmt.Sprintf("第 %d/%d 章“%s”已完成，继续生成下一章。", sectionNumber, len(sectionsToGenerate), strings.TrimSpace(currentSection.Title)))
		} else {
			progress.UpdateStage("finalizing", fmt.Sprintf("第 %d/%d 章“%s”已完成，正在收尾完整文档。", sectionNumber, len(sectionsToGenerate), strings.TrimSpace(currentSection.Title)))
		}
	}

finalize:
	finalAnswer := ""
	finalAnswer, completionStatus, finishReason, failureReason, documentGenerationStatus, qualityIssues = applyFullDocumentArtifactQualityGate(
		ctx,
		chatModel,
		agentConfig,
		currentBudget,
		finalContent.String(),
		completionStatus,
		finishReason,
		failureReason,
		documentGenerationStatus,
		qualityIssues,
	)
	completionStatus, finishReason, failureReason, documentGenerationStatus, qualityIssues = applyArtifactFirstFullDocumentIntegrityOutcome(outline, completedSections, finalAnswer, completionStatus, finishReason, failureReason, documentGenerationStatus, qualityIssues)
	if completionStatus == types.MessageCompletionStatusCompleted && strings.TrimSpace(documentGenerationStatus) == "" {
		documentGenerationStatus = types.ChatDocumentGenerationStatusCompleted
	}
	if completionStatus == types.MessageCompletionStatusPartial && strings.TrimSpace(documentGenerationStatus) == "" {
		documentGenerationStatus = types.ChatDocumentGenerationStatusContinuing
	}
	if finalAnswer == "" {
		return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, "empty_document_edit_completion", errors.New("full document generation completed without visible content"))
	}
	if err := emitDedicatedFullDocumentAnswerChunk(ctx, req, eventBus, answerEventID, "", true, completionStatus, finishReason); err != nil {
		logger.Errorf(ctx, "Failed to emit full document done chunk: %v", err)
	}
	if err := s.updateKnowledgeGroundedGenerationRun(ctx, generationRun, outline, completedSections, currentBudget, runtimeFeedback, documentGenerationStatus, completionStatus, 1); err != nil {
		logger.Errorf(ctx, "Failed to update dedicated generation run: %v", err)
	}
	persistedRuntimeFeedback := runtimeFeedback
	if generationRun != nil {
		persistedRuntimeFeedback = unmarshalGenerationRunRuntimeFeedback(generationRun.RuntimeFeedbackJSON)
	}
	extra := buildKnowledgeGroundedGenerationRunExtra(generationRun, outline, nil, currentBudget, persistedRuntimeFeedback)
	extra["local_knowledge_used"] = false
	extra["completed_sections"] = uniqueNonEmptyStrings(completedSections)
	extra = withDocumentGenerationRuntimeFeedbackExtra(extra, persistedRuntimeFeedback)
	if normalizedQualityIssues := uniqueNonEmptyStrings(qualityIssues); len(normalizedQualityIssues) > 0 {
		extra["quality_issues"] = normalizedQualityIssues
	}
	if err := emitDedicatedFullDocumentCompletion(ctx, req, eventBus, finalAnswer, completionStatus, finishReason, failureReason, documentGenerationStatus, progress.AgentSteps(), extra, startTime); err != nil {
		logger.Errorf(ctx, "Failed to emit full document completion event: %v", err)
		return err
	}
	return nil
}

func effectiveDedicatedDocumentEditFirstContentTimeout(agentConfig *types.AgentConfig) time.Duration {
	timeout := dedicatedDocumentEditFirstContentTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if agentConfig != nil && agentConfig.LLMCallTimeout > 0 {
		llmCallTimeout := time.Duration(agentConfig.LLMCallTimeout) * time.Second
		if llmCallTimeout > 0 && llmCallTimeout < timeout {
			timeout = llmCallTimeout
		}
	}
	return timeout
}

func effectiveDocumentGenerationCallTimeout(agentConfig *types.AgentConfig, budget DocumentGenerationBudget) time.Duration {
	configuredAgentLLMCallTimeoutSeconds := 0
	if agentConfig != nil && agentConfig.LLMCallTimeout > 0 {
		configuredAgentLLMCallTimeoutSeconds = agentConfig.LLMCallTimeout
	}
	timeoutSeconds := resolveDocumentGenerationCallTimeoutSeconds(budget, configuredAgentLLMCallTimeoutSeconds)
	return time.Duration(timeoutSeconds) * time.Second
}

func withDocumentGenerationCallTimeout(ctx context.Context, agentConfig *types.AgentConfig, budget DocumentGenerationBudget) (context.Context, context.CancelFunc) {
	timeout := effectiveDocumentGenerationCallTimeout(agentConfig, budget)
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func effectiveDedicatedDocumentEditHeartbeatInterval(firstContentTimeout time.Duration) time.Duration {
	interval := dedicatedDocumentEditProgressHeartbeatInterval
	if interval <= 0 {
		interval = 8 * time.Second
	}
	if firstContentTimeout > 0 && interval >= firstContentTimeout {
		interval = firstContentTimeout / 3
		if interval <= 0 {
			interval = time.Second
		}
	}
	return interval
}

func stopAndDrainTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func localizedDocumentEditProgressMessage(ctx context.Context, phase string) string {
	language := strings.ToLower(strings.TrimSpace(types.LanguageNameFromContext(ctx)))
	useChinese := language == "" || strings.Contains(language, "chinese") || strings.Contains(language, "中文")

	switch phase {
	case "started":
		if useChinese {
			return "正在分析基线文档并生成修订补丁，请稍候。"
		}
		return "Analyzing the baseline document and preparing a revision patch. Please wait."
	case "waiting":
		if useChinese {
			return "\n仍在等待模型输出首个修订片段，将在超时后自动结束本次生成。"
		}
		return "\nStill waiting for the model to emit the first revision chunk. The run will stop automatically if it times out."
	default:
		return ""
	}
}

func emitDedicatedDocumentEditThought(ctx context.Context, req *types.QARequest, eventBus *event.EventBus, eventID string, content string, done bool, replace bool, synthetic bool) {
	if req == nil || req.Session == nil || eventBus == nil || strings.TrimSpace(eventID) == "" {
		return
	}
	if err := eventBus.Emit(ctx, event.Event{
		ID:        eventID,
		Type:      event.EventAgentThought,
		SessionID: req.Session.ID,
		Data: event.AgentThoughtData{
			Content:   content,
			Iteration: 0,
			Done:      done,
			Replace:   replace,
			Synthetic: synthetic,
		},
	}); err != nil {
		logger.Errorf(ctx, "Failed to emit dedicated document edit progress event: %v", err)
	}
}

func dedicatedDocumentEditEventData(content string, done bool, completionStatus string, finishReason string, failureReason string, allowIndexing bool, allowComplete bool) event.AgentFinalAnswerData {
	return event.AgentFinalAnswerData{
		Content:          content,
		Done:             done,
		CompletionStatus: completionStatus,
		FinishReason:     finishReason,
		IsPartial:        completionStatus == types.MessageCompletionStatusPartial,
		AllowIndexing:    allowIndexing,
		AllowComplete:    allowComplete,
		FailureReason:    failureReason,
	}
}

func (s *sessionService) emitDedicatedDocumentEditFailure(ctx context.Context, req *types.QARequest, eventBus *event.EventBus, reason string, err error) error {
	if eventBus != nil {
		completionStatus := types.MessageCompletionStatusFailed
		emitErrorEvent := true
		if reason == types.MessageCompletionStatusCancelled {
			completionStatus = types.MessageCompletionStatusCancelled
			emitErrorEvent = false
		}
		if emitErrorEvent {
			_ = eventBus.Emit(ctx, event.Event{
				ID:        generateEventID("error"),
				Type:      event.EventError,
				SessionID: req.Session.ID,
				Data: event.ErrorData{
					Error:     err.Error(),
					Stage:     "document_edit",
					SessionID: req.Session.ID,
				},
			})
		}
		_ = eventBus.Emit(ctx, event.Event{
			ID:        generateEventID("complete"),
			Type:      event.EventAgentComplete,
			SessionID: req.Session.ID,
			Data: event.AgentCompleteData{
				SessionID:        req.Session.ID,
				MessageID:        req.AssistantMessageID,
				FinalAnswer:      "",
				CompletionStatus: completionStatus,
				FinishReason:     reason,
				FailureReason:    dedicatedDocumentEditFailureReasonForStatus(completionStatus, reason),
				AllowIndexing:    false,
				AllowComplete:    false,
			},
		})
	}
	return err
}

func (s *sessionService) runDedicatedDocumentEditPath(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
	chatModel chat.Chat,
	agentConfig *types.AgentConfig,
) error {
	if req == nil || req.Session == nil || eventBus == nil {
		return errors.New("document edit request is incomplete")
	}
	language := types.LanguageNameFromContext(ctx)
	useLocalKnowledge := hasEffectiveLocalKnowledgeScope(req, agentConfig)
	logger.Infof(ctx, "document_edit_path_selected: session_id=%s, message_id=%s, base_artifact_id=%s, output_mode=%s, quoted_context_len=%d",
		req.Session.ID, req.AssistantMessageID, req.BaseArtifactID, req.DocumentOutputMode, len([]rune(strings.TrimSpace(req.QuotedContext))))
	if patch, ok := buildDeterministicDocumentEditPatch(req); ok {
		logger.Infof(ctx, "document_edit_deterministic_patch_selected: session_id=%s, message_id=%s, patch_len=%d", req.Session.ID, req.AssistantMessageID, len([]rune(patch)))
		if err := emitDedicatedDocumentEditPatch(context.WithoutCancel(ctx), req, eventBus, patch, "deterministic_patch", time.Now()); err != nil {
			logger.Errorf(ctx, "Failed to emit deterministic document edit patch: %v", err)
			return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, "document_edit_error", err)
		}
		return nil
	}
	thinking := false
	editBudget := fallbackDocumentGenerationBudget(s.cfg)
	if s != nil {
		editBudget = s.resolveDocumentGenerationBudget(ctx, req, chatModel, buildDocumentProfile(req, agentConfig, useLocalKnowledge, 1), nil)
	}
	evidence := knowledgeGroundedEvidencePack{}
	completionExtra := map[string]interface{}{}
	if useLocalKnowledge {
		var err error
		evidence, err = retrieveKnowledgeGroundedFullDocumentEvidence(ctx, s.knowledgeBaseService, s.cfg, agentConfig.SearchTargets, buildDocumentEditKnowledgeQueries(req), editBudget.SectionEvidenceTopK)
		if err != nil {
			logger.Errorf(ctx, "Dedicated document edit evidence retrieval failed: %v", err)
			return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, classifyDocumentEditError(err), err)
		}
		if len(evidence.Items) == 0 {
			completionExtra = map[string]interface{}{
				"local_knowledge_used": true,
				"evidence_refs":        []interface{}{},
				"effective_kb_ids":     evidence.ScopeKBIDs,
				"evidence_queries":     evidence.Queries,
			}
			return emitDedicatedDocumentEditLocalKnowledgeBlocked(ctx, req, eventBus, "本地知识库未检索到足够证据，当前无法安全扩写目标章节，请补充资料后重试。", completionExtra, time.Now())
		}
		completionExtra = map[string]interface{}{
			"local_knowledge_used": true,
			"evidence_refs":        buildKnowledgeGroundedEvidenceRefs(evidence),
			"effective_kb_ids":     evidence.ScopeKBIDs,
			"evidence_queries":     evidence.Queries,
		}
	}
	messages := buildDedicatedDocumentEditMessages(req, language, evidence)
	maxCompletionTokens := editBudget.ContinuationMaxCompletionTokens
	if maxCompletionTokens <= 0 {
		maxCompletionTokens = 4096
	}
	firstContentTimeout := effectiveDedicatedDocumentEditFirstContentTimeout(agentConfig)
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	opt := &chat.ChatOptions{
		Temperature:         agentConfig.Temperature,
		MaxCompletionTokens: maxCompletionTokens,
		Thinking:            &thinking,
	}
	responseChan, err := chatModel.ChatStream(streamCtx, messages, opt)
	if err != nil {
		logger.Errorf(ctx, "Dedicated document edit stream start failed: %v", err)
		return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, classifyDocumentEditError(err), err)
	}
	if responseChan == nil {
		err = errors.New("document edit stream returned nil channel")
		return s.emitDedicatedDocumentEditFailure(ctx, req, eventBus, "document_edit_error", err)
	}
	logger.Infof(ctx, "document_edit_stream_started: session_id=%s, message_id=%s, model=%s, budget_source=%s, max_completion_tokens=%d, first_visible_stream_timeout_ms=%d",
		req.Session.ID, req.AssistantMessageID, chatModel.GetModelID(), editBudget.Source, maxCompletionTokens, firstContentTimeout.Milliseconds())
	s.consumeDedicatedDocumentEditStream(context.WithoutCancel(streamCtx), req, eventBus, responseChan, firstContentTimeout, completionExtra)
	return nil
}

func (s *sessionService) consumeDedicatedDocumentEditStream(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
	responseChan <-chan types.StreamResponse,
	firstContentTimeout time.Duration,
	completionExtra map[string]interface{},
) {
	eventID := generateEventID("document-edit")
	progressEventID := generateEventID("document-edit-progress")
	modelThinkingEventID := generateEventID("document-edit-model-thinking")
	startTime := time.Now()
	var finalContent strings.Builder
	completed := false
	firstVisibleStreamSeen := false
	firstContentSeen := false
	progressClosed := false
	modelThinkingStarted := false
	modelThinkingClosed := false
	failureReason := "document_edit_error"
	failureErr := errors.New("document edit stream closed before completion")
	if firstContentTimeout <= 0 {
		firstContentTimeout = 30 * time.Second
	}
	heartbeatInterval := effectiveDedicatedDocumentEditHeartbeatInterval(firstContentTimeout)
	firstContentTimer := time.NewTimer(firstContentTimeout)
	heartbeatTimer := time.NewTimer(heartbeatInterval)
	defer stopAndDrainTimer(firstContentTimer)
	defer stopAndDrainTimer(heartbeatTimer)
	closeProgress := func() {
		if progressClosed {
			return
		}
		emitDedicatedDocumentEditThought(ctx, req, eventBus, progressEventID, "", true, true, true)
		progressClosed = true
	}
	closeModelThinking := func() {
		if !modelThinkingStarted || modelThinkingClosed {
			return
		}
		emitDedicatedDocumentEditThought(ctx, req, eventBus, modelThinkingEventID, "", true, false, false)
		modelThinkingClosed = true
	}
	markFirstVisibleStream := func(source string) {
		if firstVisibleStreamSeen {
			return
		}
		firstVisibleStreamSeen = true
		stopAndDrainTimer(firstContentTimer)
		stopAndDrainTimer(heartbeatTimer)
		closeProgress()
		logger.Infof(ctx, "document_edit_first_visible_stream: session_id=%s, message_id=%s, source=%s, elapsed_ms=%d", req.Session.ID, req.AssistantMessageID, source, time.Since(startTime).Milliseconds())
	}

	emitDedicatedDocumentEditThought(ctx, req, eventBus, progressEventID, localizedDocumentEditProgressMessage(ctx, "started"), false, true, true)

	for !completed {
		select {
		case <-ctx.Done():
			failureErr = ctx.Err()
			failureReason = classifyDocumentEditError(failureErr)
			logger.Warnf(ctx, "document_edit_stream_cancelled: session_id=%s, message_id=%s, reason=%s", req.Session.ID, req.AssistantMessageID, failureReason)
			closeProgress()
			goto finalize
		case <-heartbeatTimer.C:
			if firstVisibleStreamSeen {
				continue
			}
			logger.Infof(ctx, "document_edit_waiting_for_model: session_id=%s, message_id=%s, elapsed_ms=%d", req.Session.ID, req.AssistantMessageID, time.Since(startTime).Milliseconds())
			emitDedicatedDocumentEditThought(ctx, req, eventBus, progressEventID, localizedDocumentEditProgressMessage(ctx, "waiting"), false, true, true)
			heartbeatTimer.Reset(heartbeatInterval)
		case <-firstContentTimer.C:
			if firstVisibleStreamSeen {
				continue
			}
			failureReason = "first_visible_stream_timeout"
			failureErr = fmt.Errorf("timed out waiting for first visible document edit stream after %s", firstContentTimeout)
			logger.Warnf(ctx, "document_edit_first_visible_stream_timeout: session_id=%s, message_id=%s, timeout_ms=%d", req.Session.ID, req.AssistantMessageID, firstContentTimeout.Milliseconds())
			closeProgress()
			goto finalize
		case response, ok := <-responseChan:
			if !ok {
				closeProgress()
				closeModelThinking()
				goto finalize
			}
			if response.ResponseType == types.ResponseTypeError {
				if strings.TrimSpace(response.Content) != "" {
					failureErr = errors.New(response.Content)
					failureReason = classifyDocumentEditError(failureErr)
				}
				closeProgress()
				closeModelThinking()
				goto finalize
			}
			if response.ResponseType == types.ResponseTypeThinking {
				if response.Content != "" {
					markFirstVisibleStream(string(types.ResponseTypeThinking))
					modelThinkingStarted = true
					emitDedicatedDocumentEditThought(ctx, req, eventBus, modelThinkingEventID, response.Content, false, false, false)
				}
				if response.Done {
					closeModelThinking()
				}
				continue
			}
			if response.ResponseType != types.ResponseTypeAnswer {
				continue
			}
			if response.Content == "" {
				if response.Done {
					if finalContent.Len() > 0 {
						completed = true
						finishReason := response.FinishReason
						if strings.TrimSpace(finishReason) == "" {
							finishReason = "stop"
						}
						logger.Infof(ctx, "document_edit_complete: session_id=%s, message_id=%s, finish_reason=%s, final_answer_len=%d, duration_ms=%d", req.Session.ID, req.AssistantMessageID, finishReason, len([]rune(finalContent.String())), time.Since(startTime).Milliseconds())
						if err := eventBus.Emit(ctx, event.Event{
							ID:        generateEventID("complete"),
							Type:      event.EventAgentComplete,
							SessionID: req.Session.ID,
							Data: event.AgentCompleteData{
								SessionID:        req.Session.ID,
								MessageID:        req.AssistantMessageID,
								FinalAnswer:      finalContent.String(),
								CompletionStatus: types.MessageCompletionStatusCompleted,
								FinishReason:     finishReason,
								AllowIndexing:    true,
								AllowComplete:    true,
								Extra:            mergeDocumentEditCompletionExtra(buildDocumentEditPatchExtra(req, finalContent.String(), false), completionExtra),
								TotalDurationMs:  time.Since(startTime).Milliseconds(),
							},
						}); err != nil {
							logger.Errorf(ctx, "Failed to emit dedicated document edit completion event: %v", err)
						}
						break
					}
					failureReason = "empty_document_edit_completion"
					failureErr = errors.New("document edit stream completed without visible content")
					closeProgress()
					goto finalize
				}
				continue
			}
			if !firstContentSeen {
				markFirstVisibleStream(string(types.ResponseTypeAnswer))
				firstContentSeen = true
				logger.Infof(ctx, "document_edit_first_content: session_id=%s, message_id=%s, elapsed_ms=%d", req.Session.ID, req.AssistantMessageID, time.Since(startTime).Milliseconds())
				closeModelThinking()
			}
			finalContent.WriteString(response.Content)
			if err := eventBus.Emit(ctx, event.Event{
				ID:        eventID,
				Type:      event.EventAgentFinalAnswer,
				SessionID: req.Session.ID,
				Data: dedicatedDocumentEditEventData(
					response.Content,
					response.Done,
					types.MessageCompletionStatusCompleted,
					response.FinishReason,
					"",
					true,
					true,
				),
			}); err != nil {
				logger.Errorf(ctx, "Failed to emit dedicated document edit answer chunk: %v", err)
			}
			if response.Done {
				completed = true
				finishReason := response.FinishReason
				if strings.TrimSpace(finishReason) == "" {
					finishReason = "stop"
				}
				logger.Infof(ctx, "document_edit_complete: session_id=%s, message_id=%s, finish_reason=%s, final_answer_len=%d, duration_ms=%d", req.Session.ID, req.AssistantMessageID, finishReason, len([]rune(finalContent.String())), time.Since(startTime).Milliseconds())
				if err := eventBus.Emit(ctx, event.Event{
					ID:        generateEventID("complete"),
					Type:      event.EventAgentComplete,
					SessionID: req.Session.ID,
					Data: event.AgentCompleteData{
						SessionID:        req.Session.ID,
						MessageID:        req.AssistantMessageID,
						FinalAnswer:      finalContent.String(),
						CompletionStatus: types.MessageCompletionStatusCompleted,
						FinishReason:     finishReason,
						AllowIndexing:    true,
						AllowComplete:    true,
						Extra:            mergeDocumentEditCompletionExtra(buildDocumentEditPatchExtra(req, finalContent.String(), false), completionExtra),
						TotalDurationMs:  time.Since(startTime).Milliseconds(),
					},
				}); err != nil {
					logger.Errorf(ctx, "Failed to emit dedicated document edit completion event: %v", err)
				}
			}
		}
	}

	if completed {
		return
	}

finalize:
	closeProgress()
	terminalStatus, emitErrorEvent := classifyDedicatedDocumentEditTerminalState(failureReason, finalContent.String())
	logger.Warnf(ctx, "document_edit_complete: session_id=%s, message_id=%s, completion_status=%s, finish_reason=%s, final_answer_len=%d, duration_ms=%d", req.Session.ID, req.AssistantMessageID, terminalStatus, failureReason, len([]rune(finalContent.String())), time.Since(startTime).Milliseconds())
	if emitErr := eventBus.Emit(ctx, event.Event{
		ID:        generateEventID("complete"),
		Type:      event.EventAgentComplete,
		SessionID: req.Session.ID,
		Data: event.AgentCompleteData{
			SessionID:        req.Session.ID,
			MessageID:        req.AssistantMessageID,
			FinalAnswer:      finalContent.String(),
			CompletionStatus: terminalStatus,
			FinishReason:     failureReason,
			FailureReason:    dedicatedDocumentEditFailureReasonForStatus(terminalStatus, failureReason),
			AllowIndexing:    false,
			AllowComplete:    false,
			Extra:            mergeDocumentEditCompletionExtra(buildDocumentEditPatchExtra(req, finalContent.String(), false), completionExtra),
			TotalDurationMs:  time.Since(startTime).Milliseconds(),
		},
	}); emitErr != nil {
		logger.Errorf(ctx, "Failed to emit dedicated document edit failure completion event: %v", emitErr)
	}
	if emitErrorEvent {
		_ = eventBus.Emit(ctx, event.Event{
			ID:        generateEventID("error"),
			Type:      event.EventError,
			SessionID: req.Session.ID,
			Data: event.ErrorData{
				Error:     failureErr.Error(),
				Stage:     "document_edit",
				SessionID: req.Session.ID,
			},
		})
	}
}

func classifyDedicatedDocumentEditTerminalState(failureReason string, finalAnswer string) (string, bool) {
	trimmedAnswer := strings.TrimSpace(finalAnswer)
	if failureReason == types.MessageCompletionStatusCancelled {
		return types.MessageCompletionStatusCancelled, false
	}
	if trimmedAnswer != "" {
		return types.MessageCompletionStatusPartial, false
	}
	return types.MessageCompletionStatusFailed, true
}

func dedicatedDocumentEditFailureReasonForStatus(status string, failureReason string) string {
	if status == types.MessageCompletionStatusCompleted {
		return ""
	}
	if status == types.MessageCompletionStatusCancelled && failureReason == "" {
		return types.MessageCompletionStatusCancelled
	}
	return failureReason
}

func classifyDocumentEditError(err error) string {
	if err == nil {
		return "document_edit_error"
	}
	if errors.Is(err, context.Canceled) {
		return types.MessageCompletionStatusCancelled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "llm_timeout"
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "context deadline exceeded") || strings.Contains(lower, "deadline exceeded") || strings.Contains(lower, "timeout") || strings.Contains(lower, "timed out") {
		return "llm_timeout"
	}
	for _, reason := range []string{
		"translation_batch_empty_output",
		"translation_batch_repeated_output",
		"translation_batch_source_leak",
		"translation_batch_markdown_fence_unbalanced",
		"translation_batch_header_footer_noise",
		"translation_batch_table_structure_invalid",
		"translation_batch_prompt_leak",
		"translation_chat_model_unavailable",
	} {
		if strings.Contains(lower, reason) {
			return reason
		}
	}
	return "document_edit_error"
}

func applyDocumentStopgapAgentConfig(agentConfig *types.AgentConfig, req *types.QARequest) {
	if agentConfig == nil || !shouldApplyDocumentStopgap(req) {
		return
	}

	thinking := false
	agentConfig.MaxIterations = 1
	agentConfig.WebSearchEnabled = false
	agentConfig.MultiTurnEnabled = false
	agentConfig.Thinking = &thinking
	agentConfig.RetainRetrievalHistory = false
	agentConfig.MCPSelectionMode = "none"
	agentConfig.MCPServices = nil
	agentConfig.AllowFinalAnswerTool = true
	agentConfig.AllowedTools = []string{tools.ToolFinalAnswer}
}

// buildAgentConfig creates a runtime AgentConfig from the QARequest's custom agent configuration,
// tenant info, and resolved knowledge bases / search targets.
func (s *sessionService) buildAgentConfig(
	ctx context.Context,
	req *types.QARequest,
	tenantInfo *types.Tenant,
	agentTenantID uint64,
) (*types.AgentConfig, error) {
	customAgent := req.CustomAgent
	agentConfig := &types.AgentConfig{
		MaxIterations:               customAgent.Config.MaxIterations,
		Temperature:                 customAgent.Config.Temperature,
		WebSearchEnabled:            customAgent.Config.WebSearchEnabled && req.WebSearchEnabled,
		WebSearchMaxResults:         customAgent.Config.WebSearchMaxResults,
		WebSearchProviderID:         customAgent.Config.WebSearchProviderID,
		MultiTurnEnabled:            customAgent.Config.MultiTurnEnabled,
		HistoryTurns:                customAgent.Config.HistoryTurns,
		MCPSelectionMode:            customAgent.Config.MCPSelectionMode,
		MCPServices:                 customAgent.Config.MCPServices,
		Thinking:                    customAgent.Config.Thinking,
		RetrieveKBOnlyWhenMentioned: customAgent.Config.RetrieveKBOnlyWhenMentioned,
		LLMCallTimeout:              customAgent.Config.LLMCallTimeout,
		RetainRetrievalHistory:      customAgent.Config.RetainRetrievalHistory,
	}

	// Falls back to global configuration if no specific timeout is set for the agent.
	if agentConfig.LLMCallTimeout == 0 && s.cfg.Agent != nil && s.cfg.Agent.LLMCallTimeout > 0 {
		agentConfig.LLMCallTimeout = s.cfg.Agent.LLMCallTimeout
	}

	// Configure skills based on CustomAgentConfig
	s.configureSkillsFromAgent(ctx, agentConfig, customAgent)

	// Resolve knowledge bases using shared helper
	agentConfig.KnowledgeBases, agentConfig.KnowledgeIDs = s.resolveKnowledgeBases(ctx, req)

	// Use custom agent's allowed tools if specified, otherwise use defaults
	if len(customAgent.Config.AllowedTools) > 0 {
		agentConfig.AllowedTools = customAgent.Config.AllowedTools
	} else {
		agentConfig.AllowedTools = tools.DefaultAllowedTools()
	}
	// Apply per-turn @Skill / @MCP scope. Each helper narrows the agent's
	// whitelist to the mentioned items and records the pinned set used for the
	// <must_use> hint, keeping all scope logic in one place per resource type.
	isSharedAgent := req.Session != nil && req.Session.TenantID != customAgent.TenantID
	applyPerRequestSkillScope(ctx, agentConfig, customAgent.Config.SkillsSelectionMode, req.SkillNames)
	applyPerRequestMCPScope(ctx, agentConfig, customAgent.Config.MCPServices, isSharedAgent, req.MCPServiceIDs)

	// Use custom agent's system prompt if specified
	if customAgent.Config.SystemPrompt != "" {
		agentConfig.UseCustomSystemPrompt = true
		agentConfig.SystemPrompt = customAgent.Config.SystemPrompt
	}

	applyDocumentStopgapAgentConfig(agentConfig, req)

	logger.Infof(ctx, "Custom agent config applied: MaxIterations=%d, Temperature=%.2f, AllowedTools=%v, WebSearchEnabled=%v",
		agentConfig.MaxIterations, agentConfig.Temperature, agentConfig.AllowedTools, agentConfig.WebSearchEnabled)

	// Set web search max results from tenant config if not set (default: 5)
	if agentConfig.WebSearchMaxResults == 0 {
		agentConfig.WebSearchMaxResults = 5
		if tenantInfo.WebSearchConfig != nil && tenantInfo.WebSearchConfig.MaxResults > 0 {
			agentConfig.WebSearchMaxResults = tenantInfo.WebSearchConfig.MaxResults
		}
	}

	// Resolve web search provider ID: agent-level > tenant default (is_default=true)
	if agentConfig.WebSearchProviderID == "" {
		if defaultProvider, err := s.webSearchProviderRepo.GetDefault(ctx, tenantInfo.ID); err == nil && defaultProvider != nil {
			agentConfig.WebSearchProviderID = defaultProvider.ID
		}
	}

	logger.Infof(ctx, "Merged agent config from tenant %d and session %s", tenantInfo.ID, req.Session.ID)

	// Log knowledge bases if present
	if len(agentConfig.KnowledgeBases) > 0 || len(req.TagScopes) > 0 {
		if len(agentConfig.KnowledgeBases) > 0 {
			logger.Infof(ctx, "Agent configured with %d knowledge base(s): %v",
				len(agentConfig.KnowledgeBases), agentConfig.KnowledgeBases)
		} else {
			logger.Infof(ctx, "Agent configured with %d tag-scoped search target(s)", len(req.TagScopes))
		}
	} else {
		logger.Infof(ctx, "No knowledge bases specified for agent, running in pure agent mode")
	}

	// Build search targets using agent's tenant (handler has validated access for shared agent)
	searchTargets, err := s.buildSearchTargets(ctx, agentTenantID, agentConfig.KnowledgeBases, agentConfig.KnowledgeIDs, req.TagScopes)
	if err != nil {
		logger.Warnf(ctx, "Failed to build search targets for agent: %v", err)
	}
	agentConfig.SearchTargets = searchTargets
	logger.Infof(ctx, "Agent search targets built: %d targets", len(searchTargets))

	if agentConfig.MaxContextTokens <= 0 {
		agentConfig.MaxContextTokens = types.DefaultMaxContextTokens
	}

	return agentConfig, nil
}

// applyPerRequestSkillScope narrows the agent's skill whitelist to the @Skill
// mentions for this turn and records the pinned set for the <must_use> hint.
// It is a no-op when no skills were mentioned or skills are disabled.
func applyPerRequestSkillScope(
	ctx context.Context,
	agentConfig *types.AgentConfig,
	skillsMode string,
	requested []string,
) {
	if len(requested) == 0 {
		return
	}
	if skillsMode == "none" || skillsMode == "" {
		logger.Warnf(ctx, "Ignoring @skill mention: agent skills selection is disabled (mode=%s)", skillsMode)
		return
	}
	if !agentConfig.SkillsEnabled {
		return
	}
	switch skillsMode {
	case "selected":
		agentConfig.AllowedSkills = intersectPreservingRequestOrder(requested, agentConfig.AllowedSkills)
		if len(agentConfig.AllowedSkills) == 0 {
			agentConfig.SkillsEnabled = false
		}
	case "all":
		agentConfig.AllowedSkills = dedupPreservingOrder(requested)
	}
	if agentConfig.SkillsEnabled && len(agentConfig.AllowedSkills) > 0 {
		agentConfig.PinnedSkillNames = intersectPreservingRequestOrder(requested, agentConfig.AllowedSkills)
	}
	logger.Infof(ctx, "Applied per-request @skill scope: requested=%v effective=%v pinned=%v",
		requested, agentConfig.AllowedSkills, agentConfig.PinnedSkillNames)
}

// applyPerRequestMCPScope narrows the agent's MCP services to the @MCP mentions
// for this turn and records the pinned set for the <must_use> hint. It is a
// no-op when no services were mentioned or MCP selection is disabled.
func applyPerRequestMCPScope(
	ctx context.Context,
	agentConfig *types.AgentConfig,
	agentPresetMCPs []string,
	isSharedAgent bool,
	requested []string,
) {
	if len(requested) == 0 {
		return
	}
	if agentConfig.MCPSelectionMode == "none" {
		logger.Warnf(ctx, "Ignoring @MCP mention: agent MCP selection is disabled (mode=none)")
		return
	}
	mentioned := dedupPreservingOrder(requested)
	effective, mode := resolvePerRequestMCPScope(mentioned, agentPresetMCPs, agentConfig.MCPSelectionMode, isSharedAgent)
	if len(effective) == 0 {
		logger.Warnf(ctx, "Ignoring @MCP scope outside agent preset: requested=%v agent=%v shared=%v",
			requested, agentPresetMCPs, isSharedAgent)
		return
	}
	agentConfig.MCPSelectionMode = mode
	agentConfig.MCPServices = effective
	agentConfig.PinnedMCPServiceIDs = intersectPreservingRequestOrder(requested, agentConfig.MCPServices)
	logger.Infof(ctx, "Applied per-request @MCP scope: requested=%v mode=%s effective=%v",
		requested, agentConfig.MCPSelectionMode, agentConfig.MCPServices)
}

// resolvePerRequestMCPScope narrows MCP registration for a per-turn @mention.
// selectionMode "none" rejects all mentions. Shared agents never register MCP
// services outside the agent preset.
func resolvePerRequestMCPScope(
	mentioned, agentMCPs []string,
	selectionMode string,
	isSharedAgent bool,
) (effective []string, mode string) {
	if len(mentioned) == 0 {
		return nil, selectionMode
	}
	if isSharedAgent {
		mentioned = intersectPreservingRequestOrder(mentioned, agentMCPs)
		if len(mentioned) == 0 {
			return nil, selectionMode
		}
	}
	switch selectionMode {
	case "none":
		return nil, selectionMode
	case "selected":
		effective = intersectPreservingRequestOrder(mentioned, agentMCPs)
	case "all", "":
		effective = mentioned
	default:
		effective = mentioned
	}
	if len(effective) == 0 {
		return nil, selectionMode
	}
	return effective, "selected"
}

func intersectPreservingRequestOrder(requested []string, allowed []string) []string {
	allowedSet := make(map[string]bool, len(allowed))
	for _, value := range allowed {
		if value != "" {
			allowedSet[value] = true
		}
	}
	result := make([]string, 0, len(requested))
	seen := make(map[string]bool, len(requested))
	for _, value := range requested {
		if value == "" || seen[value] || !allowedSet[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func dedupPreservingOrder(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

// configureSkillsFromAgent configures skills settings in AgentConfig based on CustomAgentConfig
// Returns the skill directories and allowed skills based on the selection mode:
//   - "all": uses all preloaded skills
//   - "selected": uses the explicitly selected skills
//   - "none" or "": skills are disabled
func (s *sessionService) configureSkillsFromAgent(
	ctx context.Context,
	agentConfig *types.AgentConfig,
	customAgent *types.CustomAgent,
) {
	if customAgent == nil {
		return
	}
	// When sandbox is disabled, skills cannot be enabled (no script execution environment)
	sandboxMode := os.Getenv("WEKNORA_SANDBOX_MODE")
	if sandboxMode == "" || sandboxMode == "disabled" {
		agentConfig.SkillsEnabled = false
		agentConfig.SkillDirs = nil
		agentConfig.AllowedSkills = nil
		logger.Infof(ctx, "Sandbox is disabled: skills are not available")
		return
	}
	dir := getPreloadedSkillsDir()
	switch customAgent.Config.SkillsSelectionMode {
	case "all":
		// Enable all preloaded skills
		agentConfig.SkillsEnabled = true
		agentConfig.SkillDirs = []string{dir}
		agentConfig.AllowedSkills = nil // Empty means all skills allowed
		logger.Infof(ctx, "SkillsSelectionMode=all: enabled all preloaded skills")
	case "selected":
		// Enable only selected skills
		if len(customAgent.Config.SelectedSkills) > 0 {
			agentConfig.SkillsEnabled = true
			agentConfig.SkillDirs = []string{dir}
			agentConfig.AllowedSkills = customAgent.Config.SelectedSkills
			logger.Infof(ctx, "SkillsSelectionMode=selected: enabled %d selected skills: %v",
				len(customAgent.Config.SelectedSkills), customAgent.Config.SelectedSkills)
		} else {
			agentConfig.SkillsEnabled = false
			logger.Infof(ctx, "SkillsSelectionMode=selected but no skills selected: skills disabled")
		}
	case "none", "":
		// Skills disabled
		agentConfig.SkillsEnabled = false
		logger.Infof(ctx, "SkillsSelectionMode=%s: skills disabled", customAgent.Config.SkillsSelectionMode)
	default:
		// Unknown mode, disable skills
		agentConfig.SkillsEnabled = false
		logger.Warnf(ctx, "Unknown SkillsSelectionMode=%s: skills disabled", customAgent.Config.SkillsSelectionMode)
	}

}
