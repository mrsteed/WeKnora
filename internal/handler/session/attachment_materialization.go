package session

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

func (h *Handler) getAttachmentTempKBState(ctx context.Context, sessionID string) *types.AttachmentTempKBState {
	if h == nil || h.attachmentTempKBStateRepo == nil || strings.TrimSpace(sessionID) == "" {
		state := &types.AttachmentTempKBState{}
		state.EnsureDefaults()
		return state
	}
	state := h.attachmentTempKBStateRepo.GetAttachmentTempKBState(ctx, sessionID)
	if state == nil {
		state = &types.AttachmentTempKBState{}
	}
	state.EnsureDefaults()
	if strings.TrimSpace(state.KBID) != "" && h.knowledgebaseService != nil {
		if _, err := h.knowledgebaseService.GetKnowledgeBaseByID(ctx, state.KBID); err != nil {
			logger.Warnf(ctx, "Attachment temp KB %s missing for session %s, resetting cached state", state.KBID, sessionID)
			state.KBID = ""
			state.KnowledgeIDs = []string{}
			state.AttachmentKnowledge = map[string]string{}
			h.attachmentTempKBStateRepo.SaveAttachmentTempKBState(ctx, sessionID, state)
		}
	}
	return state
}

func (h *Handler) materializeAttachmentsForSession(
	ctx context.Context,
	session *types.Session,
	request *CreateKnowledgeQARequest,
	customAgent *types.CustomAgent,
	kbIDs []string,
	knowledgeIDs []string,
	results []*AttachmentProcessResult,
) *types.AttachmentTempKBState {
	state := h.getAttachmentTempKBState(ctx, session.ID)
	if len(results) == 0 || h == nil || h.attachmentKnowledgeService == nil || h.knowledgebaseService == nil || h.modelService == nil || session == nil {
		return state
	}

	tempKB, createdState := h.ensureAttachmentTempKnowledgeBase(ctx, session, request, customAgent, kbIDs, knowledgeIDs, state)
	if tempKB == nil {
		if createdState {
			h.attachmentTempKBStateRepo.SaveAttachmentTempKBState(ctx, session.ID, state)
		}
		return state
	}

	stateChanged := createdState
	for _, result := range results {
		if result == nil || result.Attachment == nil {
			continue
		}
		attachmentURL := strings.TrimSpace(result.Attachment.URL)
		if attachmentURL == "" {
			continue
		}
		if existing := strings.TrimSpace(state.AttachmentKnowledge[attachmentURL]); existing != "" {
			state.KnowledgeIDs = appendUniqueStrings(state.KnowledgeIDs, existing)
			continue
		}

		var (
			knowledge *types.Knowledge
			err       error
		)
		if strings.TrimSpace(result.MarkdownContent) != "" {
			knowledge, err = h.attachmentKnowledgeService.CreateKnowledgeFromReadResult(
				ctx,
				tempKB.ID,
				attachmentURL,
				result.Attachment.FileName,
				strings.TrimPrefix(result.Attachment.FileType, "."),
				result.Attachment.FileSize,
				request.Channel,
				&types.ReadResult{
					MarkdownContent: result.MarkdownContent,
					Metadata:        result.Metadata,
				},
				result.StoredImages,
				nil,
			)
		}
		if err != nil || knowledge == nil {
			knowledge, err = h.attachmentKnowledgeService.CreateKnowledgeFromStoredFile(
				ctx,
				tempKB.ID,
				attachmentURL,
				result.Attachment.FileName,
				strings.TrimPrefix(result.Attachment.FileType, "."),
				result.Attachment.FileSize,
				request.Channel,
				nil,
			)
		}
		if err != nil {
			logger.Warnf(ctx, "Failed to materialize attachment as temporary knowledge: session=%s file=%s err=%v",
				session.ID, result.Attachment.FileName, err)
			continue
		}
		state.AttachmentKnowledge[attachmentURL] = knowledge.ID
		state.KnowledgeIDs = appendUniqueStrings(state.KnowledgeIDs, knowledge.ID)
		stateChanged = true
	}

	if stateChanged && h.attachmentTempKBStateRepo != nil {
		h.attachmentTempKBStateRepo.SaveAttachmentTempKBState(ctx, session.ID, state)
	}
	return state
}

func (h *Handler) ensureAttachmentTempKnowledgeBase(
	ctx context.Context,
	session *types.Session,
	request *CreateKnowledgeQARequest,
	customAgent *types.CustomAgent,
	kbIDs []string,
	knowledgeIDs []string,
	state *types.AttachmentTempKBState,
) (*types.KnowledgeBase, bool) {
	if state == nil {
		state = &types.AttachmentTempKBState{}
	}
	state.EnsureDefaults()
	if strings.TrimSpace(state.KBID) != "" {
		if kb, err := h.knowledgebaseService.GetKnowledgeBaseByID(ctx, state.KBID); err == nil && kb != nil {
			return kb, false
		}
		logger.Warnf(ctx, "Attachment temp KB %s no longer exists for session %s, recreating", state.KBID, session.ID)
		state.KBID = ""
		state.KnowledgeIDs = nil
		state.AttachmentKnowledge = map[string]string{}
	}

	templateKB := h.resolveAttachmentTemplateKnowledgeBase(ctx, kbIDs, knowledgeIDs, customAgent)
	embeddingModelID, err := h.resolveAttachmentEmbeddingModelID(ctx, templateKB)
	if err != nil || strings.TrimSpace(embeddingModelID) == "" {
		logger.Warnf(ctx, "Unable to resolve embedding model for attachment temp KB in session %s: %v", session.ID, err)
		return nil, false
	}

	tempKB := &types.KnowledgeBase{
		Name:             fmt.Sprintf("tmp-attachment-%s-%d", session.ID, time.Now().UnixNano()),
		Description:      "Ephemeral session attachment knowledge base",
		Type:             types.KnowledgeBaseTypeDocument,
		IsTemporary:      true,
		EmbeddingModelID: embeddingModelID,
		SummaryModelID:   h.resolveAttachmentSummaryModelID(request, customAgent, templateKB),
		ChunkingConfig:   h.resolveAttachmentChunkingConfig(templateKB),
		VLMConfig:        h.resolveAttachmentVLMConfig(customAgent, templateKB),
		ASRConfig:        h.resolveAttachmentASRConfig(customAgent, templateKB),
		IndexingStrategy: h.resolveAttachmentIndexingStrategy(templateKB),
	}
	createdKB, err := h.knowledgebaseService.CreateKnowledgeBase(ctx, tempKB)
	if err != nil {
		logger.Warnf(ctx, "Failed to create attachment temp KB for session %s: %v", session.ID, err)
		return nil, false
	}
	state.KBID = createdKB.ID
	state.KnowledgeIDs = []string{}
	state.AttachmentKnowledge = map[string]string{}
	return createdKB, true
}

func (h *Handler) resolveAttachmentTemplateKnowledgeBase(
	ctx context.Context,
	kbIDs []string,
	knowledgeIDs []string,
	customAgent *types.CustomAgent,
) *types.KnowledgeBase {
	if h == nil || h.knowledgebaseService == nil {
		return nil
	}
	for _, kbID := range kbIDs {
		if kb, err := h.knowledgebaseService.GetKnowledgeBaseByIDOnly(ctx, kbID); err == nil && kb != nil {
			return kb
		}
	}
	if h.knowledgeService != nil && len(knowledgeIDs) > 0 {
		tenantID, _ := types.TenantIDFromContext(ctx)
		if knowledgeList, err := h.knowledgeService.GetKnowledgeBatchWithSharedAccess(ctx, tenantID, knowledgeIDs[:1]); err == nil && len(knowledgeList) > 0 && knowledgeList[0] != nil {
			if kb, kbErr := h.knowledgebaseService.GetKnowledgeBaseByIDOnly(ctx, knowledgeList[0].KnowledgeBaseID); kbErr == nil && kb != nil {
				return kb
			}
		}
	}
	if customAgent != nil {
		for _, kbID := range customAgent.Config.KnowledgeBases {
			if kb, err := h.knowledgebaseService.GetKnowledgeBaseByIDOnly(ctx, kbID); err == nil && kb != nil {
				return kb
			}
		}
	}
	return nil
}

func (h *Handler) resolveAttachmentEmbeddingModelID(ctx context.Context, templateKB *types.KnowledgeBase) (string, error) {
	if templateKB != nil && strings.TrimSpace(templateKB.EmbeddingModelID) != "" {
		return strings.TrimSpace(templateKB.EmbeddingModelID), nil
	}
	models, err := h.modelService.ListModels(ctx)
	if err != nil {
		return "", err
	}
	available := make([]*types.Model, 0)
	for _, model := range models {
		if model == nil || model.Type != types.ModelTypeEmbedding || model.Status != types.ModelStatusActive {
			continue
		}
		available = append(available, model)
	}
	if len(available) == 0 {
		return "", fmt.Errorf("no active embedding model available")
	}
	sort.SliceStable(available, func(i, j int) bool {
		if available[i].IsDefault != available[j].IsDefault {
			return available[i].IsDefault
		}
		if available[i].IsBuiltin != available[j].IsBuiltin {
			return available[i].IsBuiltin
		}
		return available[i].CreatedAt.Before(available[j].CreatedAt)
	})
	return strings.TrimSpace(available[0].ID), nil
}

func (h *Handler) resolveAttachmentSummaryModelID(
	request *CreateKnowledgeQARequest,
	customAgent *types.CustomAgent,
	templateKB *types.KnowledgeBase,
) string {
	if templateKB != nil && strings.TrimSpace(templateKB.SummaryModelID) != "" {
		return strings.TrimSpace(templateKB.SummaryModelID)
	}
	if strings.TrimSpace(request.SummaryModelID) != "" {
		return strings.TrimSpace(request.SummaryModelID)
	}
	if customAgent != nil && strings.TrimSpace(customAgent.Config.ModelID) != "" {
		return strings.TrimSpace(customAgent.Config.ModelID)
	}
	return ""
}

func (h *Handler) resolveAttachmentChunkingConfig(templateKB *types.KnowledgeBase) types.ChunkingConfig {
	if templateKB != nil {
		return templateKB.ChunkingConfig
	}
	return types.ChunkingConfig{}
}

func (h *Handler) resolveAttachmentVLMConfig(customAgent *types.CustomAgent, templateKB *types.KnowledgeBase) types.VLMConfig {
	if templateKB != nil && templateKB.VLMConfig.IsEnabled() {
		return templateKB.VLMConfig
	}
	if customAgent != nil && strings.TrimSpace(customAgent.Config.VLMModelID) != "" {
		return types.VLMConfig{Enabled: true, ModelID: strings.TrimSpace(customAgent.Config.VLMModelID)}
	}
	return types.VLMConfig{}
}

func (h *Handler) resolveAttachmentASRConfig(customAgent *types.CustomAgent, templateKB *types.KnowledgeBase) types.ASRConfig {
	if templateKB != nil && templateKB.ASRConfig.IsASREnabled() {
		return templateKB.ASRConfig
	}
	if customAgent != nil && customAgent.Config.AudioUploadEnabled && strings.TrimSpace(customAgent.Config.ASRModelID) != "" {
		return types.ASRConfig{Enabled: true, ModelID: strings.TrimSpace(customAgent.Config.ASRModelID)}
	}
	return types.ASRConfig{}
}

func (h *Handler) resolveAttachmentIndexingStrategy(templateKB *types.KnowledgeBase) types.IndexingStrategy {
	strategy := types.DefaultIndexingStrategy()
	if templateKB != nil && !templateKB.IndexingStrategy.IsZero() {
		strategy = templateKB.IndexingStrategy
	}
	strategy.GraphEnabled = false
	strategy.WikiEnabled = false
	if !strategy.VectorEnabled && !strategy.KeywordEnabled {
		strategy = types.DefaultIndexingStrategy()
	}
	return strategy
}

func appendUniqueStrings(values []string, next string) []string {
	next = strings.TrimSpace(next)
	if next == "" {
		return values
	}
	for _, value := range values {
		if value == next {
			return values
		}
	}
	return append(values, next)
}

func annotateAttachmentKnowledgeization(attachments types.MessageAttachments, state *types.AttachmentTempKBState) {
	if len(attachments) == 0 {
		return
	}
	tempKBID := ""
	if state != nil {
		tempKBID = strings.TrimSpace(state.KBID)
	}
	for index := range attachments {
		attachments[index].KnowledgeizationStatus = "failed"
		attachments[index].TempKnowledgeID = ""
		attachments[index].TempKnowledgeBaseID = tempKBID
		if state == nil || len(state.AttachmentKnowledge) == 0 {
			continue
		}
		if knowledgeID := strings.TrimSpace(state.AttachmentKnowledge[attachments[index].URL]); knowledgeID != "" {
			attachments[index].KnowledgeizationStatus = "ready"
			attachments[index].TempKnowledgeID = knowledgeID
			attachments[index].TempKnowledgeBaseID = tempKBID
		}
	}
}
