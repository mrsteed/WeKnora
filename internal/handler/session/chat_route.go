package session

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

func (h *Handler) detectChatRouteDecision(ctx context.Context, logPrefix string, reqCtx *qaRequestContext, request *CreateKnowledgeQARequest) {
	if h.chatRouteService == nil || reqCtx == nil || request == nil {
		return
	}
	if strings.TrimSpace(reqCtx.documentTaskKind) == types.ChatDocumentTaskKindTranslation {
		reqCtx.routeDecision = &types.ChatRouteDecision{
			Kind:            types.ChatRouteAgentQA,
			UseAgent:        reqCtx.customAgent != nil && reqCtx.customAgent.IsAgentMode(),
			UseKnowledge:    len(reqCtx.knowledgeBaseIDs) > 0 || len(reqCtx.knowledgeIDs) > 0,
			UseLongDocument: reqCtx.documentOutputMode == types.ChatDocumentOutputModeFull,
			NeedArtifact:    reqCtx.documentOutputMode == types.ChatDocumentOutputModeFull,
			Confidence:      1,
			Reason:          "explicit_translation_task_kind_bypass",
		}
		reqCtx.routeDecisionApplied = false
		reqCtx.routeModelID = ""
		logger.Infof(ctx, "[ChatRouter] skip shadow route decision for explicit document task kind=%s reason=%s", secutils.SanitizeForLog(reqCtx.documentTaskKind), secutils.SanitizeForLog(reqCtx.routeDecision.Reason))
		return
	}
	if shouldBypassChatRouteDecision(reqCtx) {
		reqCtx.routeDecision = buildBypassedChatRouteDecision(reqCtx)
		reqCtx.routeDecisionApplied = false
		reqCtx.routeModelID = ""
		logger.Infof(ctx, "[ChatRouter] skip shadow route decision for database agent, agent_id=%s agent_type=%s reason=%s", secutils.SanitizeForLog(reqCtx.customAgent.ID), secutils.SanitizeForLog(reqCtx.customAgent.Config.AgentType), secutils.SanitizeForLog(reqCtx.routeDecision.Reason))
		return
	}
	routeCtx := h.applyEffectiveTenantContext(ctx, reqCtx.effectiveTenantID)
	routeModelID := h.resolveChatRouteModelID(routeCtx, reqCtx)
	input := h.buildChatRouteInput(logPrefix, reqCtx, request, routeModelID)
	decision, err := h.chatRouteService.Decide(routeCtx, input)
	if err != nil {
		logger.Warnf(routeCtx, "[ChatRouter] shadow decision fallback, request_id=%s err=%v", reqCtx.requestID, err)
	}
	if decision == nil {
		return
	}
	reqCtx.routeDecision = decision
	reqCtx.routeDecisionApplied = false
	reqCtx.routeModelID = routeModelID
	applyBlockedReason := "not_agent_qa_endpoint"
	if input.EndpointMode == "agent_qa" {
		reqCtx.routeDecisionApplied, applyBlockedReason = h.applyDocumentRouteDecisionWithReason(routeCtx, reqCtx, request, decision, input.HasEffectiveAgentKB)
	}
	if !reqCtx.routeDecisionApplied {
		logger.Infof(routeCtx, "[ChatRouter] document route not applied kind=%s reason=%s blocked_by=%s", decision.Kind, secutils.SanitizeForLog(decision.Reason), applyBlockedReason)
	}
	logger.Infof(routeCtx, "[ChatRouter] decision kind=%s confidence=%.2f reason=%q requested_output_mode=%s effective_agent_kb=%t model=%s applied=%t endpoint=%s", decision.Kind, decision.Confidence, secutils.SanitizeForLog(decision.Reason), secutils.SanitizeForLog(input.UserExplicitOutputMode), input.HasEffectiveAgentKB, secutils.SanitizeForLog(routeModelID), reqCtx.routeDecisionApplied, secutils.SanitizeForLog(input.EndpointMode))
}

func shouldBypassChatRouteDecision(reqCtx *qaRequestContext) bool {
	if reqCtx == nil || reqCtx.customAgent == nil || !reqCtx.customAgent.IsAgentMode() {
		return false
	}
	if strings.TrimSpace(reqCtx.customAgent.Config.AgentType) == types.AgentTypeDatabaseAnalysis {
		return true
	}
	return hasDatabaseOnlyAllowedTools(reqCtx.customAgent.Config.AllowedTools)
}

func hasDatabaseOnlyAllowedTools(allowedTools []string) bool {
	if len(allowedTools) == 0 {
		return false
	}
	databaseToolSeen := false
	allowed := map[string]struct{}{
		"thinking":                        {},
		"todo_write":                      {},
		"final_answer":                    {},
		"external_database_schema":        {},
		"external_database_search_tables": {},
		"external_database_query":         {},
	}
	for _, toolName := range allowedTools {
		trimmed := strings.TrimSpace(toolName)
		if trimmed == "" {
			continue
		}
		if _, ok := allowed[trimmed]; !ok {
			return false
		}
		switch trimmed {
		case "external_database_schema", "external_database_search_tables", "external_database_query":
			databaseToolSeen = true
		}
	}
	return databaseToolSeen
}

func buildBypassedChatRouteDecision(reqCtx *qaRequestContext) *types.ChatRouteDecision {
	if reqCtx == nil {
		return nil
	}
	return &types.ChatRouteDecision{
		Kind:            types.ChatRouteAgentQA,
		UseAgent:        reqCtx.customAgent != nil && reqCtx.customAgent.IsAgentMode(),
		UseKnowledge:    hasEffectiveKnowledgeScopeForBypass(reqCtx),
		UseLongDocument: false,
		NeedArtifact:    false,
		Confidence:      1,
		Reason:          chatRouteBypassReason(reqCtx),
	}
}

func hasEffectiveKnowledgeScopeForBypass(reqCtx *qaRequestContext) bool {
	if reqCtx == nil {
		return false
	}
	if len(reqCtx.knowledgeBaseIDs) > 0 || len(reqCtx.knowledgeIDs) > 0 {
		return true
	}
	if reqCtx.customAgent == nil {
		return false
	}
	switch reqCtx.customAgent.Config.KBSelectionMode {
	case "all":
		return true
	case "selected", "":
		return len(reqCtx.customAgent.Config.KnowledgeBases) > 0
	default:
		return false
	}
}

func chatRouteBypassReason(reqCtx *qaRequestContext) string {
	if reqCtx == nil || reqCtx.customAgent == nil {
		return "database_agent_route_bypass"
	}
	if strings.TrimSpace(reqCtx.customAgent.Config.AgentType) == types.AgentTypeDatabaseAnalysis {
		return "database_agent_type_bypass"
	}
	return "database_tool_only_agent_bypass"
}

func (h *Handler) applyDocumentRouteDecision(ctx context.Context, reqCtx *qaRequestContext, request *CreateKnowledgeQARequest, decision *types.ChatRouteDecision, hasEffectiveAgentKB bool) bool {
	applied, _ := h.applyDocumentRouteDecisionWithReason(ctx, reqCtx, request, decision, hasEffectiveAgentKB)
	return applied
}

func (h *Handler) applyDocumentRouteDecisionWithReason(ctx context.Context, reqCtx *qaRequestContext, request *CreateKnowledgeQARequest, decision *types.ChatRouteDecision, hasEffectiveAgentKB bool) (bool, string) {
	if reqCtx == nil || request == nil || decision == nil {
		return false, "missing_request_context"
	}

	if decision.Kind == types.ChatRouteFullDocument {
		hasKnowledgeScope := hasEffectiveAgentKB || len(reqCtx.knowledgeBaseIDs) > 0 || len(reqCtx.knowledgeIDs) > 0
		if hasKnowledgeScope {
			promoted := *decision
			promoted.Kind = types.ChatRouteKnowledgeGroundedFullDoc
			promoted.UseKnowledge = true
			decision = &promoted
		}
	}

	switch decision.Kind {
	case types.ChatRouteShortDocument:
		if reqCtx.autoContinue || strings.TrimSpace(reqCtx.baseArtifactID) != "" {
			return false, documentRouteBlockedReason(reqCtx.autoContinue, strings.TrimSpace(reqCtx.baseArtifactID) != "", false, false, false)
		}
		reqCtx.documentIntent = ""
		reqCtx.documentOperation = ""
		reqCtx.documentOutputMode = ""
		reqCtx.documentTargetHeading = ""
		reqCtx.documentMergeMode = ""
		reqCtx.documentQuotedContext = ""
		reqCtx.baseArtifact = nil
		reqCtx.baseArtifactID = ""
		return true, ""
	case types.ChatRouteFullDocument:
		if ok, reason := canApplyFullDocumentRouteDecisionWithReason(reqCtx, hasEffectiveAgentKB, false); !ok {
			return false, reason
		}
		reqCtx.documentIntent = routeDocumentIntent(decision, "", types.ChatDocumentIntentNormal)
		reqCtx.documentOperation = routeDocumentOperation(decision, "", types.ChatDocumentOperationCreate)
		reqCtx.documentOutputMode = types.ChatDocumentOutputModeFull
		reqCtx.documentTargetHeading = ""
		reqCtx.documentMergeMode = ""
		reqCtx.documentQuotedContext = ""
		reqCtx.baseArtifact = nil
		reqCtx.baseArtifactID = ""
		return true, ""
	case types.ChatRouteKnowledgeGroundedFullDoc:
		if ok, reason := canApplyFullDocumentRouteDecisionWithReason(reqCtx, hasEffectiveAgentKB, true); !ok {
			return false, reason
		}
		reqCtx.documentIntent = routeDocumentIntent(decision, "", types.ChatDocumentIntentNormal)
		reqCtx.documentOperation = routeDocumentOperation(decision, "", types.ChatDocumentOperationCreate)
		reqCtx.documentOutputMode = types.ChatDocumentOutputModeFull
		reqCtx.documentTargetHeading = ""
		reqCtx.documentMergeMode = ""
		reqCtx.documentQuotedContext = ""
		reqCtx.baseArtifact = nil
		reqCtx.baseArtifactID = ""
		return true, ""
	case types.ChatRouteDocumentEdit:
		if ok, reason := canApplyDocumentEditRouteDecisionWithReason(reqCtx, false); !ok {
			return false, reason
		}
		prep := h.prepareDocumentRequestFromRouteDecision(ctx, reqCtx.session, reqCtx.query, reqCtx.baseArtifactID, decision, false)
		if prep.baseArtifact == nil || strings.TrimSpace(prep.quotedContext) == "" {
			return false, "missing_document_artifact_context"
		}
		reqCtx.documentIntent = prep.intent
		reqCtx.documentOperation = prep.operation
		reqCtx.baseArtifact = prep.baseArtifact
		reqCtx.baseArtifactID = prep.baseArtifact.ID
		reqCtx.documentQuotedContext = prep.quotedContext
		reqCtx.documentOutputMode = types.ChatDocumentOutputModeDelta
		reqCtx.documentTargetHeading = prep.targetHeading
		reqCtx.documentMergeMode = prep.mergeMode
		return true, ""
	case types.ChatRouteKnowledgeGroundedContinue:
		if ok, reason := canApplyDocumentEditRouteDecisionWithReason(reqCtx, true); !ok {
			return false, reason
		}
		if !hasEffectiveAgentKB && len(reqCtx.knowledgeBaseIDs) == 0 && len(reqCtx.knowledgeIDs) == 0 {
			return false, "missing_knowledge_scope"
		}
		prep := h.prepareDocumentRequestFromRouteDecision(ctx, reqCtx.session, reqCtx.query, reqCtx.baseArtifactID, decision, true)
		if prep.baseArtifact == nil || strings.TrimSpace(prep.quotedContext) == "" {
			return false, "missing_document_artifact_context"
		}
		reqCtx.documentIntent = prep.intent
		reqCtx.documentOperation = prep.operation
		reqCtx.baseArtifact = prep.baseArtifact
		reqCtx.baseArtifactID = prep.baseArtifact.ID
		reqCtx.documentQuotedContext = prep.quotedContext
		reqCtx.documentOutputMode = types.ChatDocumentOutputModeDelta
		reqCtx.documentTargetHeading = prep.targetHeading
		reqCtx.documentMergeMode = prep.mergeMode
		return true, ""
	default:
		return false, "not_document_route"
	}
}

func canApplyFullDocumentRouteDecision(reqCtx *qaRequestContext, hasEffectiveAgentKB bool, requireKnowledge bool) bool {
	ok, _ := canApplyFullDocumentRouteDecisionWithReason(reqCtx, hasEffectiveAgentKB, requireKnowledge)
	return ok
}

func canApplyFullDocumentRouteDecisionWithReason(reqCtx *qaRequestContext, hasEffectiveAgentKB bool, requireKnowledge bool) (bool, string) {
	if reqCtx == nil || strings.TrimSpace(reqCtx.query) == "" {
		return false, "empty_query"
	}
	if reqCtx.autoContinue || strings.TrimSpace(reqCtx.baseArtifactID) != "" {
		return false, documentRouteBlockedReason(reqCtx.autoContinue, strings.TrimSpace(reqCtx.baseArtifactID) != "", false, false, false)
	}
	if len(reqCtx.attachments) > 0 || len(reqCtx.images) > 0 {
		return false, documentRouteBlockedReason(false, false, len(reqCtx.attachments) > 0, len(reqCtx.images) > 0, false)
	}
	hasKnowledgeScope := hasEffectiveAgentKB || len(reqCtx.knowledgeBaseIDs) > 0 || len(reqCtx.knowledgeIDs) > 0
	if requireKnowledge {
		if !hasKnowledgeScope {
			return false, "missing_knowledge_scope"
		}
		return true, ""
	}
	if hasKnowledgeScope {
		return false, "has_knowledge_scope"
	}
	return true, ""
}

func canApplyDocumentEditRouteDecision(reqCtx *qaRequestContext, requireAutoContinue bool) bool {
	ok, _ := canApplyDocumentEditRouteDecisionWithReason(reqCtx, requireAutoContinue)
	return ok
}

func canApplyDocumentEditRouteDecisionWithReason(reqCtx *qaRequestContext, requireAutoContinue bool) (bool, string) {
	if reqCtx == nil || strings.TrimSpace(reqCtx.query) == "" {
		return false, "empty_query"
	}
	if len(reqCtx.attachments) > 0 || len(reqCtx.images) > 0 {
		return false, documentRouteBlockedReason(false, false, len(reqCtx.attachments) > 0, len(reqCtx.images) > 0, false)
	}
	if requireAutoContinue {
		if !reqCtx.autoContinue {
			return false, "missing_auto_continue"
		}
		return true, ""
	}
	if reqCtx.autoContinue {
		return false, "auto_continue"
	}
	return true, ""
}

func documentRouteBlockedReason(autoContinue bool, hasBaseArtifact bool, hasAttachments bool, hasImages bool, hasKnowledgeScope bool) string {
	switch {
	case autoContinue:
		return "auto_continue"
	case hasBaseArtifact:
		return "existing_artifact"
	case hasAttachments:
		return "has_attachments"
	case hasImages:
		return "has_images"
	case hasKnowledgeScope:
		return "has_knowledge_scope"
	default:
		return "blocked_by_guard"
	}
}

func routeDocumentIntent(decision *types.ChatRouteDecision, current string, fallback string) string {
	if decision != nil {
		switch strings.TrimSpace(decision.Intent) {
		case types.ChatDocumentIntentContinue, types.ChatDocumentIntentRevise, types.ChatDocumentIntentRegenerate, types.ChatDocumentIntentNormal:
			return strings.TrimSpace(decision.Intent)
		}
	}
	switch strings.TrimSpace(current) {
	case types.ChatDocumentIntentContinue, types.ChatDocumentIntentRevise, types.ChatDocumentIntentRegenerate, types.ChatDocumentIntentNormal:
		return strings.TrimSpace(current)
	default:
		return fallback
	}
}

func routeDocumentOperation(decision *types.ChatRouteDecision, current string, fallback string) string {
	if decision != nil {
		switch strings.TrimSpace(decision.Operation) {
		case types.ChatDocumentOperationContinue, types.ChatDocumentOperationRevise, types.ChatDocumentOperationRegenerate, types.ChatDocumentOperationCreate:
			return strings.TrimSpace(decision.Operation)
		}
	}
	switch strings.TrimSpace(current) {
	case types.ChatDocumentOperationContinue, types.ChatDocumentOperationRevise, types.ChatDocumentOperationRegenerate, types.ChatDocumentOperationCreate:
		return strings.TrimSpace(current)
	default:
		return fallback
	}
}

func (h *Handler) prepareDocumentRequestFromRouteDecision(ctx context.Context, session *types.Session, query string, baseArtifactID string, decision *types.ChatRouteDecision, preferContinue bool) documentRequestPreparation {
	result := documentRequestPreparation{}
	if h.chatDocumentArtifactService == nil || session == nil || decision == nil {
		return result
	}

	intentFallback := types.ChatDocumentIntentRevise
	operationFallback := types.ChatDocumentOperationRevise
	if preferContinue {
		intentFallback = types.ChatDocumentIntentContinue
		operationFallback = types.ChatDocumentOperationContinue
	}
	result.intent = routeDocumentIntent(decision, "", intentFallback)
	result.operation = routeDocumentOperation(decision, "", operationFallback)

	var artifact *types.ChatDocumentArtifact
	var err error
	if strings.TrimSpace(baseArtifactID) != "" {
		artifact, err = h.chatDocumentArtifactService.GetArtifact(ctx, baseArtifactID)
	} else {
		artifact, err = h.chatDocumentArtifactService.GetLatestArtifact(ctx, session.ID)
	}
	if err != nil {
		logger.Warnf(ctx, "Failed to load chat document artifact for route decision, session_id: %s, base_artifact_id: %s, error: %v", session.ID, baseArtifactID, err)
		return documentRequestPreparation{}
	}
	if artifact == nil || artifact.SessionID != session.ID || !artifact.CanUseAsBaseForIntent(result.intent) {
		return documentRequestPreparation{}
	}

	detectedIntent, detectErr := h.chatDocumentArtifactService.DetectIntent(ctx, session.ID, query, result.intent)
	if detectErr != nil {
		logger.Warnf(ctx, "Failed to detect route-decision document target, session_id: %s, error: %v", session.ID, detectErr)
	}
	effectiveTargetHeading, normalizedMergeMode := resolvePreparedDocumentTargetAndMerge(result.intent, decision.TargetHeading, decision.MergeMode, detectedIntent)
	quotedContext, err := h.chatDocumentArtifactService.BuildQuotedContext(ctx, artifact, query, result.intent, types.ChatDocumentOutputModeDelta, effectiveTargetHeading, normalizedMergeMode)
	if err != nil {
		logger.Warnf(ctx, "Failed to build route-decision quoted context, session_id: %s, artifact_id: %s, error: %v", session.ID, artifact.ID, err)
		return documentRequestPreparation{}
	}
	if strings.TrimSpace(quotedContext) == "" {
		return documentRequestPreparation{}
	}

	result.baseArtifact = artifact
	result.quotedContext = quotedContext
	result.targetHeading = effectiveTargetHeading
	result.mergeMode = normalizedMergeMode
	return result
}

func (h *Handler) applyEffectiveTenantContext(ctx context.Context, effectiveTenantID uint64) context.Context {
	if effectiveTenantID == 0 || h.tenantService == nil {
		return ctx
	}
	tenant, err := h.tenantService.GetTenantByID(ctx, effectiveTenantID)
	if err != nil || tenant == nil {
		return context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)
	}
	return context.WithValue(context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID), types.TenantInfoContextKey, tenant)
}

func (h *Handler) buildChatRouteInput(logPrefix string, reqCtx *qaRequestContext, request *CreateKnowledgeQARequest, routeModelID string) types.ChatRouteInput {
	endpointMode := "knowledge_qa"
	if strings.EqualFold(strings.TrimSpace(logPrefix), "AgentQA") {
		endpointMode = "agent_qa"
	}
	agentModeEnabled := request.AgentEnabled
	if reqCtx.customAgent != nil {
		agentModeEnabled = reqCtx.customAgent.IsAgentMode()
	}
	hasEffectiveAgentKB := false
	if reqCtx.customAgent != nil {
		switch reqCtx.customAgent.Config.KBSelectionMode {
		case "all":
			hasEffectiveAgentKB = true
		case "selected", "":
			hasEffectiveAgentKB = len(reqCtx.customAgent.Config.KnowledgeBases) > 0
		}
	}
	requestedRoute := ""
	if request.AutoContinue {
		requestedRoute = string(types.ChatRouteKnowledgeGroundedContinue)
	} else if strings.TrimSpace(request.BaseArtifactID) != "" {
		requestedRoute = string(types.ChatRouteDocumentEdit)
	}
	var artifactSummary *types.ChatArtifactRouteSummary
	if reqCtx.baseArtifact != nil {
		artifactSummary = &types.ChatArtifactRouteSummary{
			ID:                       reqCtx.baseArtifact.ID,
			Title:                    reqCtx.baseArtifact.Title,
			Operation:                reqCtx.baseArtifact.Operation,
			DocumentGenerationStatus: reqCtx.baseArtifact.DocumentGenerationStatus,
		}
	}
	return types.ChatRouteInput{
		Query:                    reqCtx.query,
		Channel:                  reqCtx.channel,
		EndpointMode:             endpointMode,
		ModelID:                  routeModelID,
		AgentConfigured:          reqCtx.customAgent != nil,
		AgentModeEnabledByConfig: agentModeEnabled,
		HasSelectedKnowledge:     len(reqCtx.knowledgeBaseIDs) > 0 || len(reqCtx.knowledgeIDs) > 0,
		HasEffectiveAgentKB:      hasEffectiveAgentKB,
		WebSearchEnabled:         reqCtx.webSearchEnabled,
		HasAttachments:           len(reqCtx.attachments) > 0,
		HasImages:                len(reqCtx.images) > 0,
		AutoContinue:             reqCtx.autoContinue,
		ExplicitBaseArtifactID:   reqCtx.baseArtifactID,
		LatestArtifactSummary:    artifactSummary,
		UserExplicitOutputMode:   request.DocumentOutputMode,
		UserRequestedRoute:       requestedRoute,
	}
}

func (h *Handler) resolveChatRouteModelID(ctx context.Context, reqCtx *qaRequestContext) string {
	if h.modelService == nil || reqCtx == nil {
		return ""
	}
	if modelID := strings.TrimSpace(reqCtx.summaryModelID); modelID != "" {
		if model, err := h.modelService.GetModelByID(ctx, modelID); err == nil && model != nil {
			return modelID
		}
	}
	if reqCtx.customAgent != nil {
		if modelID := strings.TrimSpace(reqCtx.customAgent.Config.ModelID); modelID != "" {
			return modelID
		}
	}
	for _, kbID := range h.chatRouteCandidateKnowledgeBaseIDs(ctx, reqCtx) {
		if h.knowledgebaseService == nil {
			break
		}
		kb, err := h.knowledgebaseService.GetKnowledgeBaseByID(ctx, kbID)
		if err != nil || kb == nil || strings.TrimSpace(kb.SummaryModelID) == "" {
			continue
		}
		return strings.TrimSpace(kb.SummaryModelID)
	}
	models, err := h.modelService.ListModels(ctx)
	if err != nil {
		return ""
	}
	for _, model := range models {
		if model != nil && model.Type == types.ModelTypeKnowledgeQA {
			return model.ID
		}
	}
	return ""
}

func (h *Handler) chatRouteCandidateKnowledgeBaseIDs(ctx context.Context, reqCtx *qaRequestContext) []string {
	if reqCtx == nil {
		return nil
	}
	seen := make(map[string]struct{})
	appendUnique := func(target []string, values ...string) []string {
		for _, value := range values {
			trimmed := strings.TrimSpace(value)
			if trimmed == "" {
				continue
			}
			if _, exists := seen[trimmed]; exists {
				continue
			}
			seen[trimmed] = struct{}{}
			target = append(target, trimmed)
		}
		return target
	}
	candidateIDs := make([]string, 0, len(reqCtx.knowledgeBaseIDs)+len(reqCtx.knowledgeIDs))
	if len(reqCtx.knowledgeBaseIDs) > 0 {
		candidateIDs = appendUnique(candidateIDs, reqCtx.knowledgeBaseIDs...)
	}
	if len(candidateIDs) == 0 && len(reqCtx.knowledgeIDs) > 0 && h.knowledgeService != nil {
		tenantID, ok := types.TenantIDFromContext(ctx)
		if ok {
			if knowledgeList, err := h.knowledgeService.GetKnowledgeBatchWithSharedAccess(ctx, tenantID, reqCtx.knowledgeIDs); err == nil {
				for _, knowledge := range knowledgeList {
					if knowledge == nil {
						continue
					}
					candidateIDs = appendUnique(candidateIDs, knowledge.KnowledgeBaseID)
				}
			}
		}
	}
	if reqCtx.customAgent == nil || reqCtx.customAgent.Config.RetrieveKBOnlyWhenMentioned {
		return candidateIDs
	}
	switch reqCtx.customAgent.Config.KBSelectionMode {
	case "selected", "":
		candidateIDs = appendUnique(candidateIDs, reqCtx.customAgent.Config.KnowledgeBases...)
	default:
		return candidateIDs
	}
	return candidateIDs
}

func buildChatRouteCompletionExtra(decision *types.ChatRouteDecision, modelID string, applied bool) map[string]interface{} {
	if decision == nil {
		return nil
	}
	return map[string]interface{}{
		"chat_route": map[string]interface{}{
			"shadow_mode": !applied,
			"applied":     applied,
			"model_id":    strings.TrimSpace(modelID),
			"decision":    decision,
		},
	}
}
