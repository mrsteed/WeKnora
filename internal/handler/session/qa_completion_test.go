package session

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type messageServiceStub struct {
	mu               sync.Mutex
	getMessageResult *types.Message
	getMessageErr    error
	updatedMessages  []*types.Message
	updateMessageErr error
	indexedCalls     int
	indexedOptions   []interfaces.MessageIndexOptions
	indexedCallCh    chan struct{}
}

func (s *messageServiceStub) CreateMessage(context.Context, *types.Message) (*types.Message, error) {
	return nil, nil
}

func TestChatDocumentAutoContinueReasonPrefersQualityIssue(t *testing.T) {
	artifact := &types.ChatDocumentArtifact{
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusNeedsReview,
		QualityIssues: []string{
			types.ChatDocumentQualityIssueSectionNumberReset,
		},
	}

	reason := chatDocumentAutoContinueReason(types.ChatDocumentGenerationStatusNeedsReview, "", "", 0, artifact)

	assert.Equal(t, types.ChatDocumentQualityIssueSectionNumberReset, reason)
}

func TestOptionalTranslationFieldsHandleNilOptions(t *testing.T) {
	assert.Equal(t, "", optionalTranslationTargetLanguage(nil))
	assert.Equal(t, "", optionalTranslationOutputFormat(nil))

	options := &types.ChatDocumentTranslationOptions{
		TargetLanguage: " English ",
		OutputFormat:   " markdown ",
	}
	assert.Equal(t, "English", optionalTranslationTargetLanguage(options))
	assert.Equal(t, "markdown", optionalTranslationOutputFormat(options))
}

func TestBuildTranslationSessionTitleSeed_PrefersKnowledgeTitleAndLanguage(t *testing.T) {
	seed := buildTranslationSessionTitleSeed("对全文完成翻译。", "设备巡检手册", "English", "markdown")
	assert.Equal(t, "请将《设备巡检手册》完整翻译为English Markdown", seed)

	fallback := buildTranslationSessionTitleSeed("对全文完成翻译。", "", "English", "markdown")
	assert.Equal(t, "对全文完成翻译。", fallback)
}

func (s *messageServiceStub) GetMessage(context.Context, string, string) (*types.Message, error) {
	return s.getMessageResult, s.getMessageErr
}

func (s *messageServiceStub) GetMessagesBySession(context.Context, string, int, int) ([]*types.Message, error) {
	return nil, nil
}

func (s *messageServiceStub) GetRecentMessagesBySession(context.Context, string, int) ([]*types.Message, error) {
	return nil, nil
}

func (s *messageServiceStub) GetMessagesBySessionBeforeTime(context.Context, string, time.Time, int) ([]*types.Message, error) {
	return nil, nil
}

func (s *messageServiceStub) UpdateMessage(_ context.Context, message *types.Message) error {
	if s.updateMessageErr != nil {
		return s.updateMessageErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	copyMsg := *message
	s.updatedMessages = append(s.updatedMessages, &copyMsg)
	return nil
}

func (s *messageServiceStub) UpdateMessageImages(context.Context, string, string, types.MessageImages) error {
	return nil
}

func (s *messageServiceStub) UpdateMessageRenderedContent(context.Context, string, string, string) error {
	return nil
}

func (s *messageServiceStub) DeleteMessage(context.Context, string, string) error { return nil }

func (s *messageServiceStub) ClearSessionMessages(context.Context, string) error { return nil }

func (s *messageServiceStub) SearchMessages(context.Context, *types.MessageSearchParams) (*types.MessageSearchResult, error) {
	return nil, nil
}

func (s *messageServiceStub) IndexMessageToKB(_ context.Context, _ string, _ string, _ string, _ string, options interfaces.MessageIndexOptions) {
	s.mu.Lock()
	s.indexedCalls++
	s.indexedOptions = append(s.indexedOptions, options)
	s.mu.Unlock()
	if s.indexedCallCh != nil {
		s.indexedCallCh <- struct{}{}
	}
}

func (s *messageServiceStub) DeleteMessageKnowledge(context.Context, string) {}

func (s *messageServiceStub) DeleteSessionKnowledge(context.Context, string) {}

func (s *messageServiceStub) GetChatHistoryKBStats(context.Context) (*types.ChatHistoryKBStats, error) {
	return nil, nil
}

type chatDocumentArtifactServiceStub struct {
	registeredArtifacts []*types.Message
	registerErr         error
	sessionArtifacts    []*types.ChatDocumentArtifact
	lastRegisterOptions types.RegisterChatDocumentArtifactOptions
	registerResult      *types.ChatDocumentArtifact
}

type generationRunBindingSessionServiceStub struct {
	continueStreamSessionServiceStub
	boundRunID    string
	boundArtifact *types.ChatDocumentArtifact
	bindErr       error
	bindCallCount int
	recordedRunID string
	recordedState *types.ChatDocumentGenerationRunState
	returnedState *types.ChatDocumentGenerationRunState
}

func (s *generationRunBindingSessionServiceStub) GetSessionByID(ctx context.Context, tenantID uint64, id string) (*types.Session, error) {
	return s.continueStreamSessionServiceStub.GetSessionByID(ctx, tenantID, id)
}

func (s *generationRunBindingSessionServiceStub) SetSessionOwnerID(ctx context.Context, tenantID uint64, sessionID, ownerID string) error {
	return s.continueStreamSessionServiceStub.SetSessionOwnerID(ctx, tenantID, sessionID, ownerID)
}

func (s *generationRunBindingSessionServiceStub) UpdateSessionLastRequestState(ctx context.Context, sessionID string, state *types.SessionLastRequestState) error {
	return s.continueStreamSessionServiceStub.UpdateSessionLastRequestState(ctx, sessionID, state)
}

func (s *generationRunBindingSessionServiceStub) BindKnowledgeGroundedGenerationRunArtifact(_ context.Context, runID string, artifact *types.ChatDocumentArtifact) error {
	s.boundRunID = runID
	s.bindCallCount++
	if artifact != nil {
		copied := *artifact
		s.boundArtifact = &copied
	}
	return s.bindErr
}

func (s *generationRunBindingSessionServiceStub) RecordChatDocumentGenerationRunState(_ context.Context, runID string, update types.ChatDocumentGenerationRunState) (*types.ChatDocumentGenerationRunState, error) {
	s.recordedRunID = runID
	state := types.NormalizeChatDocumentGenerationRunState(update)
	s.recordedState = &state
	if s.returnedState != nil {
		returned := types.NormalizeChatDocumentGenerationRunState(*s.returnedState)
		return &returned, nil
	}
	return &state, nil
}

func (s *chatDocumentArtifactServiceStub) DetectIntent(context.Context, string, string, string) (*types.DocumentIntentResult, error) {
	return &types.DocumentIntentResult{Intent: types.ChatDocumentIntentNormal, Operation: types.ChatDocumentOperationCreate}, nil
}

func TestBuildChatDocumentContinuationDecision_StopsAtRoundLimit(t *testing.T) {
	decision := buildChatDocumentContinuationDecision(
		types.ChatDocumentGenerationStatusContinuing,
		"stop",
		"",
		2,
		nil,
		map[string]interface{}{
			"generation_run_state": map[string]interface{}{
				"max_auto_continue_rounds": 2,
			},
		},
	)

	assert.Equal(t, chatDocumentNextActionManualRetry, decision.action)
	assert.Equal(t, "auto_continue_round_limit", decision.reason)
	assert.Equal(t, "达到自动续写轮次上限", decision.reasonMessage)
	assert.False(t, decision.canAutoContinue)
}

func TestBuildChatDocumentContinuationDecision_StopsOnLowGrowthState(t *testing.T) {
	decision := buildChatDocumentContinuationDecision(
		types.ChatDocumentGenerationStatusContinuing,
		"stop",
		"",
		1,
		nil,
		map[string]interface{}{
			"generation_run_state": map[string]interface{}{
				"low_growth_rounds":     2,
				"max_low_growth_rounds": 2,
			},
		},
	)

	assert.Equal(t, chatDocumentNextActionManualRetry, decision.action)
	assert.Equal(t, "auto_continue_low_growth", decision.reason)
	assert.Equal(t, "连续多轮新增内容过少，请检查完整文档后继续", decision.reasonMessage)
	assert.False(t, decision.canAutoContinue)
}

func (s *chatDocumentArtifactServiceStub) GetLatestArtifact(context.Context, string) (*types.ChatDocumentArtifact, error) {
	return nil, nil
}

func TestCompleteAssistantMessage_RecordsGenerationRunState(t *testing.T) {
	stub := &messageServiceStub{}
	artifactStub := &chatDocumentArtifactServiceStub{registerResult: &types.ChatDocumentArtifact{
		ID:                       "artifact-1",
		SessionID:                "sess-1",
		SourceMessageID:          "msg-1",
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusContinuing,
		DocumentTaskKind:         types.ChatDocumentTaskKindTranslation,
		SnapshotCharCount:        1024,
	}}
	sessionStub := &generationRunBindingSessionServiceStub{}
	handler := &Handler{messageService: stub, chatDocumentArtifactService: artifactStub, sessionService: sessionStub}
	message := &types.Message{ID: "msg-1", SessionID: "sess-1", RequestID: "req-1", Role: "assistant", Content: "partial content"}
	options := assistantCompletionOptions{
		CompletionStatus:         types.MessageCompletionStatusPartial,
		FinishReason:             "section_batch_limit",
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusContinuing,
		GenerationRunID:          "run-1",
		Extra: map[string]interface{}{
			"document_task_kind": types.ChatDocumentTaskKindTranslation,
		},
		RegisterArtifactOptions: types.RegisterChatDocumentArtifactOptions{GenerationRunID: "run-1"},
	}

	updated := handler.completeAssistantMessage(context.Background(), message, "继续输出", options)
	require.True(t, updated)
	require.NotNil(t, sessionStub.recordedState)
	assert.Equal(t, "run-1", sessionStub.recordedRunID)
	assert.Equal(t, "artifact-1", sessionStub.recordedState.ActiveArtifactID)
	assert.Equal(t, types.ChatDocumentTaskKindTranslation, sessionStub.recordedState.TaskKind)
	assert.Equal(t, 1024, sessionStub.recordedState.LastSnapshotCharCount)
	assert.Equal(t, types.ChatDocumentGenerationStatusContinuing, sessionStub.recordedState.LastDocumentStatus)
}

func (s *chatDocumentArtifactServiceStub) GetArtifact(context.Context, string) (*types.ChatDocumentArtifact, error) {
	return nil, nil
}

func (s *chatDocumentArtifactServiceStub) GetArtifactBySourceMessageID(_ context.Context, sourceMessageID string) (*types.ChatDocumentArtifact, error) {
	for _, artifact := range s.sessionArtifacts {
		if artifact != nil && artifact.SourceMessageID == sourceMessageID {
			return artifact, nil
		}
	}
	return nil, nil
}

func (s *chatDocumentArtifactServiceStub) BuildQuotedContext(context.Context, *types.ChatDocumentArtifact, string, string, string, string, string) (string, error) {
	return "", nil
}

func (s *chatDocumentArtifactServiceStub) RegisterFromAssistantMessage(_ context.Context, message *types.Message, options types.RegisterChatDocumentArtifactOptions) (*types.ChatDocumentArtifact, error) {
	s.lastRegisterOptions = options
	if s.registerErr != nil {
		return nil, s.registerErr
	}
	switch message.CompletionStatusOrLegacy() {
	case types.MessageCompletionStatusCompleted, types.MessageCompletionStatusPartial:
	default:
		return nil, nil
	}
	copyMsg := *message
	s.registeredArtifacts = append(s.registeredArtifacts, &copyMsg)
	if s.registerResult != nil {
		copied := *s.registerResult
		return &copied, nil
	}
	return &types.ChatDocumentArtifact{ID: "artifact-1", SessionID: message.SessionID, RevisionNo: 1, Operation: types.ChatDocumentOperationCreate}, nil
}

func (s *chatDocumentArtifactServiceStub) ListBySession(context.Context, string, int) ([]*types.ChatDocumentArtifact, error) {
	return s.sessionArtifacts, nil
}

func (s *chatDocumentArtifactServiceStub) ListRevisions(context.Context, string) ([]*types.ChatDocumentArtifact, error) {
	return nil, nil
}

func TestCompleteAssistantMessage_PartialDoesNotCompleteOrIndex(t *testing.T) {
	stub := &messageServiceStub{}
	handler := &Handler{messageService: stub}
	message := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant", Content: "partial content"}

	updated := handler.completeAssistantMessage(context.Background(), message, "query", assistantCompletionOptions{
		CompletionStatus: "partial",
		FinishReason:     "length",
		FailureReason:    "length",
		AllowIndexing:    false,
		AllowComplete:    false,
	})

	assert.True(t, updated)
	require.Len(t, stub.updatedMessages, 1)
	assert.False(t, message.IsCompleted)
	assert.Equal(t, types.MessageCompletionStatusPartial, message.CompletionStatus)
	assert.Equal(t, "length", message.FinishReason)
	assert.Equal(t, "length", message.FailureReason)
	assert.Equal(t, 0, stub.indexedCalls)
}

func TestCompleteAssistantMessage_CompletedIndexesWhenAllowed(t *testing.T) {
	stub := &messageServiceStub{indexedCallCh: make(chan struct{}, 1)}
	artifactStub := &chatDocumentArtifactServiceStub{}
	handler := &Handler{messageService: stub, chatDocumentArtifactService: artifactStub}
	message := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant", Content: "final content"}
	var observedArtifact *types.ChatDocumentArtifact

	updated := handler.completeAssistantMessage(context.Background(), message, "query", assistantCompletionOptions{
		CompletionStatus: "completed",
		FinishReason:     "stop",
		AllowIndexing:    true,
		AllowComplete:    true,
		ArtifactObserver: func(artifact *types.ChatDocumentArtifact) {
			observedArtifact = artifact
		},
	})

	assert.True(t, updated)
	require.Len(t, stub.updatedMessages, 1)
	assert.True(t, message.IsCompleted)
	assert.Equal(t, types.MessageCompletionStatusCompleted, message.CompletionStatus)
	assert.Equal(t, "stop", message.FinishReason)

	select {
	case <-stub.indexedCallCh:
	case <-time.After(time.Second):
		t.Fatal("expected asynchronous indexing call")
	}

	assert.Equal(t, 1, stub.indexedCalls)
	require.Len(t, stub.indexedOptions, 1)
	assert.Equal(t, "completed", stub.indexedOptions[0].CompletionStatus)
	assert.Equal(t, "stop", stub.indexedOptions[0].FinishReason)
	assert.True(t, stub.indexedOptions[0].AllowIndexing)
	assert.Empty(t, stub.indexedOptions[0].TaskKind)
	require.Len(t, artifactStub.registeredArtifacts, 1)
	require.NotNil(t, observedArtifact)
	assert.Equal(t, "artifact-1", observedArtifact.ID)
}

func TestCompleteAssistantMessage_FullDocumentIndexPassesStructuredMetadata(t *testing.T) {
	stub := &messageServiceStub{indexedCallCh: make(chan struct{}, 1)}
	artifactStub := &chatDocumentArtifactServiceStub{registerResult: &types.ChatDocumentArtifact{
		ID:                       "artifact-full-1",
		SessionID:                "sess-1",
		RevisionNo:               1,
		Operation:                types.ChatDocumentOperationCreate,
		Title:                    "北海电厂二期智慧电厂项目技术方案",
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusCompleted,
	}}
	handler := &Handler{messageService: stub, chatDocumentArtifactService: artifactStub}
	message := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant", Content: "# 北海电厂二期智慧电厂项目技术方案\n\n## 项目背景与建设目标\n\n正文一。\n\n## 总体技术架构\n\n正文二。\n\n## 实施与运维保障\n\n正文三。"}

	updated := handler.completeAssistantMessage(context.Background(), message, "这是一份提交给甲方的投标技术方案，请输出完整技术方案。", assistantCompletionOptions{
		CompletionStatus:         "completed",
		FinishReason:             "stop",
		AllowIndexing:            true,
		AllowComplete:            true,
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusCompleted,
		Extra: map[string]interface{}{
			"outline": map[string]interface{}{
				"title":    "北海电厂二期智慧电厂项目技术方案",
				"sections": []interface{}{"项目背景与建设目标", "总体技术架构", "实施与运维保障"},
			},
		},
		RegisterArtifactOptions: types.RegisterChatDocumentArtifactOptions{
			OutputMode:               types.ChatDocumentOutputModeFull,
			DocumentGenerationStatus: types.ChatDocumentGenerationStatusCompleted,
		},
	})

	assert.True(t, updated)
	select {
	case <-stub.indexedCallCh:
	case <-time.After(time.Second):
		t.Fatal("expected asynchronous indexing call")
	}
	require.Len(t, stub.indexedOptions, 1)
	assert.Equal(t, "long_document", stub.indexedOptions[0].TaskKind)
	assert.Equal(t, types.ChatDocumentGenerationStatusCompleted, stub.indexedOptions[0].DocumentGenerationStatus)
	assert.Equal(t, "artifact-full-1", stub.indexedOptions[0].ArtifactID)
	assert.Equal(t, "北海电厂二期智慧电厂项目技术方案", stub.indexedOptions[0].DocumentTitle)
	assert.Equal(t, []string{"项目背景与建设目标", "总体技术架构", "实施与运维保障"}, stub.indexedOptions[0].DocumentSections)
}

func TestBuildMessageIndexOptions_FullDocumentMetadataPreservedWhenIndexingDisabled(t *testing.T) {
	artifact := &types.ChatDocumentArtifact{
		ID:                       "artifact-full-2",
		Title:                    "完整技术方案",
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusContinuing,
	}
	options := assistantCompletionOptions{
		CompletionStatus:         types.MessageCompletionStatusPartial,
		FinishReason:             "section_batch_limit",
		AllowIndexing:            false,
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusContinuing,
		Extra: map[string]interface{}{
			"outline": map[string]interface{}{
				"title":    "完整技术方案",
				"sections": []interface{}{"项目背景", "实施路径"},
			},
		},
		RegisterArtifactOptions: types.RegisterChatDocumentArtifactOptions{
			OutputMode:               types.ChatDocumentOutputModeFull,
			DocumentGenerationStatus: types.ChatDocumentGenerationStatusContinuing,
		},
	}

	indexOptions := buildMessageIndexOptions(options, artifact)
	assert.False(t, indexOptions.AllowIndexing)
	assert.Equal(t, "long_document", indexOptions.TaskKind)
	assert.Equal(t, types.ChatDocumentGenerationStatusContinuing, indexOptions.DocumentGenerationStatus)
	assert.Equal(t, "artifact-full-2", indexOptions.ArtifactID)
	assert.Equal(t, "完整技术方案", indexOptions.DocumentTitle)
	assert.Equal(t, []string{"项目背景", "实施路径"}, indexOptions.DocumentSections)
}

func TestBuildMessageIndexOptions_ParsesStructuredOutlineSectionMaps(t *testing.T) {
	options := assistantCompletionOptions{
		CompletionStatus:         types.MessageCompletionStatusCompleted,
		FinishReason:             "stop",
		AllowIndexing:            true,
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusNeedsReview,
		Extra: map[string]interface{}{
			"outline": map[string]interface{}{
				"title": "完整技术方案",
				"sections": []map[string]interface{}{
					{"number": 1, "title": "项目背景", "heading": "第1章 项目背景"},
					{"number": 2, "title": "实施路径", "heading": "第2章 实施路径"},
				},
			},
		},
		RegisterArtifactOptions: types.RegisterChatDocumentArtifactOptions{
			OutputMode:               types.ChatDocumentOutputModeFull,
			DocumentGenerationStatus: types.ChatDocumentGenerationStatusNeedsReview,
		},
	}

	indexOptions := buildMessageIndexOptions(options, nil)
	assert.Equal(t, "long_document", indexOptions.TaskKind)
	assert.Equal(t, types.ChatDocumentGenerationStatusNeedsReview, indexOptions.DocumentGenerationStatus)
	assert.Equal(t, "完整技术方案", indexOptions.DocumentTitle)
	assert.Equal(t, []string{"项目背景", "实施路径"}, indexOptions.DocumentSections)
}

func TestBuildMessageIndexOptions_ShortDocumentArtifactDoesNotIndexAsLongDocument(t *testing.T) {
	artifact := &types.ChatDocumentArtifact{
		ID:    "artifact-short-1",
		Title: "会议纪要",
	}
	options := assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "stop",
		AllowIndexing:    true,
		RegisterArtifactOptions: types.RegisterChatDocumentArtifactOptions{
			NeedArtifact:    true,
			UseLongDocument: false,
		},
	}

	indexOptions := buildMessageIndexOptions(options, artifact)
	assert.Empty(t, indexOptions.TaskKind)
	assert.Empty(t, indexOptions.DocumentGenerationStatus)
	assert.Empty(t, indexOptions.ArtifactID)
}

func TestBuildLongDocumentPersistenceObservability_UsesArtifactIssuesAndBudgetSource(t *testing.T) {
	observability := buildLongDocumentPersistenceObservability(assistantCompletionOptions{
		GenerationRunID: "run-qa-1",
		Extra: map[string]interface{}{
			"quality_issues": []interface{}{types.ChatDocumentQualityIssueMarkdownStructureInvalid},
			"budget": map[string]interface{}{
				"source": "runtime_adjusted",
			},
		},
	}, &types.ChatDocumentArtifact{
		QualityIssues: []string{types.ChatDocumentQualityIssueMarkdownTooShort},
	})

	assert.Equal(t, "run-qa-1", observability.GenerationRunID)
	assert.Equal(t, "runtime_adjusted", observability.BudgetSource)
	assert.Equal(t, []string{types.ChatDocumentQualityIssueMarkdownStructureInvalid, types.ChatDocumentQualityIssueMarkdownTooShort}, observability.QualityIssues)
}

func TestMergeCompletionExtra_PreservesShadowRouteAndCompletionFields(t *testing.T) {
	merged := mergeCompletionExtra(
		buildChatRouteCompletionExtra(&types.ChatRouteDecision{Kind: types.ChatRouteAgentQA, Confidence: 0.42, Reason: "shadow"}, "model-1", false),
		map[string]interface{}{"generation_run_id": "run-1"},
	)

	require.NotNil(t, merged)
	assert.Equal(t, "run-1", merged["generation_run_id"])
	routeRaw, ok := merged["chat_route"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, routeRaw["shadow_mode"])
	assert.Equal(t, false, routeRaw["applied"])
	assert.Equal(t, "model-1", routeRaw["model_id"])
	decision, ok := routeRaw["decision"].(*types.ChatRouteDecision)
	require.True(t, ok)
	assert.Equal(t, types.ChatRouteAgentQA, decision.Kind)
	assert.Equal(t, 0.42, decision.Confidence)
}

func TestCompleteAssistantMessage_AgentModePersistsExistingSteps(t *testing.T) {
	stub := &messageServiceStub{}
	handler := &Handler{messageService: stub}
	message := &types.Message{
		ID:         "msg-1",
		SessionID:  "sess-1",
		RequestID:  "req-1",
		Role:       "assistant",
		Content:    "final content",
		AgentSteps: types.AgentSteps{{Iteration: 0, Thought: "first thought"}},
	}

	updated := handler.completeAssistantMessage(context.Background(), message, "query", assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "stop",
		AllowIndexing:    false,
		AllowComplete:    true,
		AgentMode:        true,
	})

	assert.True(t, updated)
	require.Len(t, stub.updatedMessages, 1)
	assert.True(t, message.IsCompleted)
	assert.Equal(t, types.MessageCompletionStatusCompleted, message.CompletionStatus)
	assert.Equal(t, types.AgentSteps{{Iteration: 0, Thought: "first thought"}}, message.AgentSteps)
	assert.Equal(t, types.AgentSteps{{Iteration: 0, Thought: "first thought"}}, stub.updatedMessages[0].AgentSteps)
	assert.Equal(t, 0, stub.indexedCalls)
}

func TestCompleteAssistantMessage_AgentModeHydratesCompletionPayload(t *testing.T) {
	stub := &messageServiceStub{}
	handler := &Handler{messageService: stub}
	message := &types.Message{
		ID:        "msg-1",
		SessionID: "sess-1",
		RequestID: "req-1",
		Role:      "assistant",
	}

	updated := handler.completeAssistantMessage(context.Background(), message, "query", assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "tool_calls",
		AllowIndexing:    false,
		AllowComplete:    true,
		AgentMode:        true,
		FinalAnswer:      "final content",
		AgentSteps:       types.AgentSteps{{Iteration: 0, Thought: "first thought"}},
		AgentDurationMs:  3210,
	})

	assert.True(t, updated)
	require.Len(t, stub.updatedMessages, 1)
	assert.Equal(t, "final content", message.Content)
	assert.Equal(t, types.AgentSteps{{Iteration: 0, Thought: "first thought"}}, message.AgentSteps)
	assert.Equal(t, int64(3210), message.AgentDurationMs)
	assert.Equal(t, types.AgentSteps{{Iteration: 0, Thought: "first thought"}}, stub.updatedMessages[0].AgentSteps)
	assert.Equal(t, int64(3210), stub.updatedMessages[0].AgentDurationMs)
}

func TestCompleteAssistantMessage_AgentModePrefersFallbackFinalAnswer(t *testing.T) {
	stub := &messageServiceStub{}
	handler := &Handler{messageService: stub}
	message := &types.Message{
		ID:        "msg-1",
		SessionID: "sess-1",
		RequestID: "req-1",
		Role:      "assistant",
		Content:   "partial streamed content",
	}

	updated := handler.completeAssistantMessage(context.Background(), message, "query", assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusPartial,
		FinishReason:     "fallback_stop",
		AllowIndexing:    false,
		AllowComplete:    false,
		AgentMode:        true,
		FinalAnswer:      "partial streamed content with authoritative recovered suffix",
	})

	assert.True(t, updated)
	require.Len(t, stub.updatedMessages, 1)
	assert.Equal(t, "partial streamed content with authoritative recovered suffix", message.Content)
	assert.Equal(t, "partial streamed content with authoritative recovered suffix", stub.updatedMessages[0].Content)
}

func TestCompleteAssistantMessage_AgentModeDoesNotReplaceWithShorterFinalAnswer(t *testing.T) {
	stub := &messageServiceStub{}
	handler := &Handler{messageService: stub}
	message := &types.Message{
		ID:        "msg-1",
		SessionID: "sess-1",
		RequestID: "req-1",
		Role:      "assistant",
		Content:   "streamed content already longer than final",
	}

	updated := handler.completeAssistantMessage(context.Background(), message, "query", assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "stop",
		AllowIndexing:    false,
		AllowComplete:    true,
		AgentMode:        true,
		FinalAnswer:      "short final",
	})

	assert.True(t, updated)
	require.Len(t, stub.updatedMessages, 1)
	assert.Equal(t, "streamed content already longer than final", message.Content)
	assert.Equal(t, "streamed content already longer than final", stub.updatedMessages[0].Content)
}

func TestRecoveredToolErrorCompletePayloadPersistsPartialFinalAnswer(t *testing.T) {
	stub := &messageServiceStub{}
	handler := &Handler{messageService: stub}
	message := &types.Message{
		ID:            "msg-1",
		SessionID:     "sess-1",
		RequestID:     "req-1",
		Role:          "assistant",
		Content:       "partial streamed answer",
		FinishReason:  "tool_error",
		FailureReason: "tool_error",
	}

	options := completionOptionsFromComplete(event.AgentCompleteData{
		CompletionStatus: types.MessageCompletionStatusPartial,
		FinishReason:     "fallback_stop",
		AllowIndexing:    false,
		AllowComplete:    false,
		FinalAnswer:      "partial streamed answer with authoritative recovered suffix",
		AgentSteps:       types.AgentSteps{{Iteration: 0, Thought: "first thought"}},
		TotalDurationMs:  789,
	})

	updated := handler.completeAssistantMessage(context.Background(), message, "query", options)

	assert.True(t, updated)
	require.Len(t, stub.updatedMessages, 1)
	assert.Equal(t, types.MessageCompletionStatusPartial, message.CompletionStatus)
	assert.Equal(t, "fallback_stop", message.FinishReason)
	assert.Equal(t, "", message.FailureReason)
	assert.False(t, message.IsCompleted)
	assert.Equal(t, "partial streamed answer with authoritative recovered suffix", message.Content)
	assert.Equal(t, int64(789), message.AgentDurationMs)
	assert.Equal(t, "partial streamed answer with authoritative recovered suffix", stub.updatedMessages[0].Content)
	assert.Equal(t, "fallback_stop", stub.updatedMessages[0].FinishReason)
	assert.Equal(t, "", stub.updatedMessages[0].FailureReason)
	assert.Equal(t, 0, stub.indexedCalls)
}

func TestCompletionOptionsFromComplete_MarksAgentMode(t *testing.T) {
	options := completionOptionsFromComplete(event.AgentCompleteData{
		CompletionStatus:         types.MessageCompletionStatusCompleted,
		FinishReason:             "stop",
		AllowIndexing:            true,
		AllowComplete:            true,
		FinalAnswer:              "final content",
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusCompleted,
		KnowledgeRefs:            []interface{}{map[string]interface{}{"chunk_id": "chunk-1"}},
		Extra:                    map[string]interface{}{"local_knowledge_used": true},
		AgentSteps:               types.AgentSteps{{Iteration: 0, Thought: "first thought"}},
		TotalDurationMs:          4567,
	})

	assert.True(t, options.AgentMode)
	assert.Equal(t, types.MessageCompletionStatusCompleted, options.CompletionStatus)
	assert.Equal(t, "stop", options.FinishReason)
	assert.Equal(t, "final content", options.FinalAnswer)
	assert.Equal(t, types.ChatDocumentGenerationStatusCompleted, options.DocumentGenerationStatus)
	require.Len(t, options.KnowledgeRefs, 1)
	assert.Equal(t, true, options.Extra["local_knowledge_used"])
	assert.Equal(t, types.AgentSteps{{Iteration: 0, Thought: "first thought"}}, options.AgentSteps)
	assert.Equal(t, int64(4567), options.AgentDurationMs)
}

func TestCompleteAssistantMessage_PropagatesDocumentGenerationStatusToArtifactRegistration(t *testing.T) {
	stub := &messageServiceStub{}
	artifactStub := &chatDocumentArtifactServiceStub{}
	handler := &Handler{messageService: stub, chatDocumentArtifactService: artifactStub}
	message := &types.Message{ID: "msg-1", SessionID: "sess-1", RequestID: "req-1", Role: "assistant", Content: "# 文档\n\n## 第一章\n\n内容"}

	updated := handler.completeAssistantMessage(context.Background(), message, "生成完整文档", assistantCompletionOptions{
		CompletionStatus:         types.MessageCompletionStatusPartial,
		FinishReason:             "section_batch_limit",
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusContinuing,
		AllowIndexing:            false,
		AllowComplete:            false,
		RegisterArtifactOptions: types.RegisterChatDocumentArtifactOptions{
			Intent:     types.ChatDocumentIntentNormal,
			Operation:  types.ChatDocumentOperationCreate,
			OutputMode: types.ChatDocumentOutputModeFull,
		},
	})

	assert.True(t, updated)
	assert.Equal(t, types.ChatDocumentGenerationStatusContinuing, artifactStub.lastRegisterOptions.DocumentGenerationStatus)
}

func TestCompleteAssistantMessage_PropagatesQualityIssuesToArtifactRegistration(t *testing.T) {
	stub := &messageServiceStub{}
	artifactStub := &chatDocumentArtifactServiceStub{}
	handler := &Handler{messageService: stub, chatDocumentArtifactService: artifactStub}
	message := &types.Message{ID: "msg-1", SessionID: "sess-1", RequestID: "req-1", Role: "assistant", Content: "# 文档\n\n## 第一章\n\n内容"}

	updated := handler.completeAssistantMessage(context.Background(), message, "生成完整文档", assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "stop",
		AllowIndexing:    true,
		AllowComplete:    true,
		Extra: map[string]interface{}{
			"quality_issues": []interface{}{types.ChatDocumentQualityIssueMarkdownHeadingNormalized, types.ChatDocumentQualityIssueMarkdownStructureInvalid},
		},
		RegisterArtifactOptions: types.RegisterChatDocumentArtifactOptions{
			Intent:     types.ChatDocumentIntentNormal,
			Operation:  types.ChatDocumentOperationCreate,
			OutputMode: types.ChatDocumentOutputModeFull,
		},
	})

	assert.True(t, updated)
	assert.Equal(t, []string{types.ChatDocumentQualityIssueMarkdownHeadingNormalized, types.ChatDocumentQualityIssueMarkdownStructureInvalid}, artifactStub.lastRegisterOptions.QualityIssues)
}

func TestCompleteAssistantMessage_MergesQualityIssuesToArtifactRegistration(t *testing.T) {
	stub := &messageServiceStub{}
	artifactStub := &chatDocumentArtifactServiceStub{}
	handler := &Handler{messageService: stub, chatDocumentArtifactService: artifactStub}
	message := &types.Message{ID: "msg-1", SessionID: "sess-1", RequestID: "req-1", Role: "assistant", Content: "# 文档\n\n## 第一章\n\n内容"}

	updated := handler.completeAssistantMessage(context.Background(), message, "生成完整文档", assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "stop",
		AllowIndexing:    true,
		AllowComplete:    true,
		Extra: map[string]interface{}{
			"quality_issues": []interface{}{types.ChatDocumentQualityIssueMarkdownStructureInvalid, types.ChatDocumentQualityIssueMarkdownTooShort},
		},
		RegisterArtifactOptions: types.RegisterChatDocumentArtifactOptions{
			Intent:        types.ChatDocumentIntentNormal,
			Operation:     types.ChatDocumentOperationCreate,
			OutputMode:    types.ChatDocumentOutputModeFull,
			QualityIssues: []string{types.ChatDocumentQualityIssueMarkdownHeadingNormalized, types.ChatDocumentQualityIssueMarkdownStructureInvalid},
		},
	})

	assert.True(t, updated)
	assert.Equal(t, []string{
		types.ChatDocumentQualityIssueMarkdownHeadingNormalized,
		types.ChatDocumentQualityIssueMarkdownStructureInvalid,
		types.ChatDocumentQualityIssueMarkdownTooShort,
	}, artifactStub.lastRegisterOptions.QualityIssues)
}

func TestCompleteAssistantMessage_BindsGenerationRunRootArtifactAfterRegistration(t *testing.T) {
	stub := &messageServiceStub{}
	artifactStub := &chatDocumentArtifactServiceStub{}
	sessionStub := &generationRunBindingSessionServiceStub{}
	handler := &Handler{messageService: stub, chatDocumentArtifactService: artifactStub, sessionService: sessionStub}
	message := &types.Message{ID: "msg-1", SessionID: "sess-1", RequestID: "req-1", Role: "assistant", Content: "# 文档\n\n## 第一章\n\n内容"}

	updated := handler.completeAssistantMessage(context.Background(), message, "生成完整文档", assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusPartial,
		FinishReason:     "section_batch_limit",
		AllowIndexing:    false,
		AllowComplete:    false,
		RegisterArtifactOptions: types.RegisterChatDocumentArtifactOptions{
			Intent:          types.ChatDocumentIntentNormal,
			Operation:       types.ChatDocumentOperationCreate,
			OutputMode:      types.ChatDocumentOutputModeFull,
			GenerationRunID: "run-1",
		},
	})

	assert.True(t, updated)
	assert.Equal(t, 1, sessionStub.bindCallCount)
	assert.Equal(t, "run-1", sessionStub.boundRunID)
	require.NotNil(t, sessionStub.boundArtifact)
	assert.Equal(t, "artifact-1", sessionStub.boundArtifact.ID)
}

func TestCompleteAssistantMessage_PropagatesEvidenceRefsToArtifactRegistration(t *testing.T) {
	stub := &messageServiceStub{}
	artifactStub := &chatDocumentArtifactServiceStub{}
	handler := &Handler{messageService: stub, chatDocumentArtifactService: artifactStub}
	message := &types.Message{ID: "msg-1", SessionID: "sess-1", RequestID: "req-1", Role: "assistant", Content: "# 文档\n\n## 第一章\n\n内容"}

	updated := handler.completeAssistantMessage(context.Background(), message, "生成完整文档", assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusPartial,
		FinishReason:     "section_batch_limit",
		AllowIndexing:    false,
		AllowComplete:    false,
		Extra: map[string]interface{}{
			"local_knowledge_used": true,
			"evidence_refs": []interface{}{
				map[string]interface{}{
					"query":             "智慧运行 平台架构",
					"knowledge_base_id": "kb-1",
					"knowledge_id":      "doc-1",
					"chunk_id":          "chunk-1",
					"source_title":      "智慧运行总体方案",
					"score":             0.91,
					"evidence_type":     types.ChatDocumentEvidenceTypeChunk,
					"content_checksum":  "checksum-1",
				},
			},
		},
		RegisterArtifactOptions: types.RegisterChatDocumentArtifactOptions{
			Intent:          types.ChatDocumentIntentNormal,
			Operation:       types.ChatDocumentOperationCreate,
			OutputMode:      types.ChatDocumentOutputModeFull,
			GenerationRunID: "run-1",
		},
	})

	assert.True(t, updated)
	assert.True(t, artifactStub.lastRegisterOptions.LocalKnowledgeUsed)
	require.Len(t, artifactStub.lastRegisterOptions.EvidenceRefs, 1)
	assert.Equal(t, "chunk-1", artifactStub.lastRegisterOptions.EvidenceRefs[0].ChunkID)
	assert.Equal(t, "智慧运行总体方案", artifactStub.lastRegisterOptions.EvidenceRefs[0].SourceTitle)
}

func TestEmitAssistantCompleteEvent_IncludesArtifactFinalDocumentMetadata(t *testing.T) {
	eventBus := event.NewEventBus()
	message := &types.Message{
		ID:        "msg-1",
		SessionID: "sess-1",
		Role:      "assistant",
		Content:   "delta content",
	}
	artifact := &types.ChatDocumentArtifact{
		ID:                       "artifact-1",
		SessionID:                "sess-1",
		SourceMessageID:          "msg-1",
		RevisionNo:               2,
		Operation:                types.ChatDocumentOperationRevise,
		Status:                   types.ChatDocumentArtifactStatusPartial,
		ArtifactKind:             types.ChatDocumentArtifactKindMarkdown,
		ContentSnapshot:          "# 完整文档",
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusCompleted,
		QualityIssues:            []string{types.ChatDocumentQualityIssueUnclosedCodeFence},
		CanContinueDocument:      true,
		CanInlineContinue:        true,
	}

	var captured event.AgentCompleteData
	eventBus.On(event.EventAgentComplete, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		captured = data
		return nil
	})

	emitAssistantCompleteEvent(eventBus, "sess-1", message, assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusPartial,
		FinishReason:     "fallback_stop",
		AllowIndexing:    false,
		AllowComplete:    false,
		KnowledgeRefs:    []interface{}{map[string]interface{}{"chunk_id": "chunk-1"}},
		Extra:            map[string]interface{}{"local_knowledge_used": true, "evidence_refs": []interface{}{map[string]interface{}{"chunk_id": "chunk-1"}}},
	}, artifact)

	assert.Equal(t, types.ChatDocumentFinalDocumentModeInlineSnapshot, captured.FinalDocumentMode)
	assert.Equal(t, "artifact-1", captured.FinalDocumentArtifactID)
	assert.Equal(t, "# 完整文档", captured.FinalDocument)
	assert.Equal(t, types.ChatDocumentGenerationStatusCompleted, captured.DocumentGenerationStatus)
	require.NotNil(t, captured.AutoContinueNext)
	assert.False(t, *captured.AutoContinueNext)
	assert.Equal(t, "document_complete_marker", captured.AutoContinueReason)
	require.Len(t, captured.KnowledgeRefs, 1)
	require.NotNil(t, captured.Extra)
	assert.Equal(t, true, captured.Extra["local_knowledge_used"])
	_, hasEvidenceRefs := captured.Extra["evidence_refs"]
	assert.True(t, hasEvidenceRefs)
	metadata, ok := captured.Extra["chat_document_artifact"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "artifact-1", metadata["id"])
	assert.Equal(t, "msg-1", metadata["source_message_id"])
	assert.Equal(t, types.ChatDocumentGenerationStatusCompleted, metadata["document_generation_status"])
	assert.Equal(t, true, metadata["can_continue"])
	assert.Equal(t, true, metadata["can_auto_continue"])
	assert.Equal(t, true, metadata["can_manual_continue"])
	assert.Equal(t, true, metadata["can_manual_revise"])
	assert.Equal(t, true, metadata["can_use_as_base"])
	assert.Equal(t, true, metadata["can_view"])
	qualityIssueDetails, ok := metadata["quality_issue_details"].([]types.ChatDocumentQualityIssueDetail)
	require.True(t, ok)
	require.NotEmpty(t, qualityIssueDetails)
	assert.Equal(t, types.ChatDocumentQualityIssueUnclosedCodeFence, qualityIssueDetails[0].Code)
}

func TestEmitAssistantCompleteEvent_AutoContinueNextOnlyForStableContinuingState(t *testing.T) {
	tests := []struct {
		name                   string
		finishReason           string
		failureReason          string
		autoContinueRound      int
		expectedFinishReason   string
		expectedNext           bool
		expectedContinueReason string
		expectedNextAction     string
	}{
		{
			name:                 "section batch limit continues",
			finishReason:         "section_batch_limit",
			autoContinueRound:    0,
			expectedFinishReason: "section_batch_limit",
			expectedNext:         true,
			expectedNextAction:   "continue_auto",
		},
		{
			name:                   "llm timeout continues",
			finishReason:           "section_generation_timeout",
			failureReason:          "llm_timeout",
			autoContinueRound:      0,
			expectedFinishReason:   "section_generation_timeout",
			expectedNext:           true,
			expectedContinueReason: "",
			expectedNextAction:     "continue_auto",
		},
		{
			name:                   "second llm timeout stops",
			finishReason:           "section_generation_timeout",
			failureReason:          "llm_timeout",
			autoContinueRound:      1,
			expectedFinishReason:   "llm_timeout_retry_exhausted",
			expectedNext:           false,
			expectedContinueReason: "llm_timeout_retry_exhausted",
			expectedNextAction:     "manual_retry",
		},
		{
			name:                   "truncated section pauses",
			finishReason:           "section_generation_truncated",
			failureReason:          "section_generation_truncated",
			autoContinueRound:      0,
			expectedFinishReason:   "section_generation_truncated",
			expectedNext:           false,
			expectedContinueReason: "section_generation_truncated",
			expectedNextAction:     "manual_retry",
		},
		{
			name:                   "state low growth pauses",
			finishReason:           "section_batch_limit",
			autoContinueRound:      1,
			expectedFinishReason:   "section_batch_limit",
			expectedNext:           false,
			expectedContinueReason: "auto_continue_low_growth",
			expectedNextAction:     "manual_retry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventBus := event.NewEventBus()
			message := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant", Content: "# 文档"}
			var captured event.AgentCompleteData
			eventBus.On(event.EventAgentComplete, func(ctx context.Context, evt event.Event) error {
				data, ok := evt.Data.(event.AgentCompleteData)
				require.True(t, ok)
				captured = data
				return nil
			})

			options := assistantCompletionOptions{
				CompletionStatus:         types.MessageCompletionStatusPartial,
				FinishReason:             tt.finishReason,
				FailureReason:            tt.failureReason,
				DocumentGenerationStatus: types.ChatDocumentGenerationStatusContinuing,
				AutoContinueRound:        tt.autoContinueRound,
				AllowIndexing:            false,
				AllowComplete:            false,
			}
			if tt.expectedContinueReason == "auto_continue_low_growth" {
				options.Extra = map[string]interface{}{
					"generation_run_state": map[string]interface{}{
						"low_growth_rounds":     2,
						"max_low_growth_rounds": 2,
					},
				}
			}

			emitAssistantCompleteEvent(eventBus, "sess-1", message, options, nil)

			require.NotNil(t, captured.AutoContinueNext)
			assert.Equal(t, tt.expectedFinishReason, captured.FinishReason)
			assert.Equal(t, tt.expectedNext, *captured.AutoContinueNext)
			assert.Equal(t, tt.expectedContinueReason, captured.AutoContinueReason)
			assert.Equal(t, tt.expectedNextAction, captured.NextAction)
			require.NotNil(t, captured.CanAutoContinue)
			assert.Equal(t, tt.expectedNext, *captured.CanAutoContinue)
			if tt.expectedNext {
				require.NotNil(t, captured.RecommendedRequest)
				assert.Equal(t, types.ChatDocumentIntentContinue, captured.RecommendedRequest["intent_hint"])
				assert.Equal(t, true, captured.RecommendedRequest["auto_continue"])
			} else {
				assert.Nil(t, captured.RecommendedRequest)
			}
		})
	}
}

func TestCompleteAssistantMessageInPlace_PropagatesRecordedGenerationRunStateToCaller(t *testing.T) {
	stub := &messageServiceStub{}
	artifactStub := &chatDocumentArtifactServiceStub{registerResult: &types.ChatDocumentArtifact{
		ID:                       "artifact-1",
		SessionID:                "sess-1",
		SourceMessageID:          "msg-1",
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusContinuing,
		SnapshotCharCount:        180,
	}}
	sessionStub := &generationRunBindingSessionServiceStub{returnedState: &types.ChatDocumentGenerationRunState{
		ActiveArtifactID:      "artifact-1",
		LowGrowthRounds:       2,
		MaxLowGrowthRounds:    2,
		LastSnapshotCharCount: 180,
	}}
	handler := &Handler{messageService: stub, chatDocumentArtifactService: artifactStub, sessionService: sessionStub}
	message := &types.Message{ID: "msg-1", SessionID: "sess-1", RequestID: "req-1", Role: "assistant", Content: "partial content"}
	options := assistantCompletionOptions{
		CompletionStatus:         types.MessageCompletionStatusPartial,
		FinishReason:             "section_batch_limit",
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusContinuing,
		GenerationRunID:          "run-1",
		AllowIndexing:            false,
		AllowComplete:            false,
		RegisterArtifactOptions:  types.RegisterChatDocumentArtifactOptions{GenerationRunID: "run-1"},
	}

	ok := handler.completeAssistantMessageInPlace(context.Background(), message, "继续输出", &options)
	require.True(t, ok)
	state, ok := options.Extra["generation_run_state"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 2, state["low_growth_rounds"])
	assert.Equal(t, 2, state["max_low_growth_rounds"])

	eventBus := event.NewEventBus()
	var captured event.AgentCompleteData
	eventBus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		captured = data
		return nil
	})
	emitAssistantCompleteEvent(eventBus, "sess-1", message, options, artifactStub.registerResult)
	require.NotNil(t, captured.AutoContinueNext)
	assert.False(t, *captured.AutoContinueNext)
	assert.Equal(t, "auto_continue_low_growth", captured.AutoContinueReason)
	assert.Equal(t, chatDocumentNextActionManualRetry, captured.NextAction)
}

func TestCompleteAssistantMessage_StripsDocumentCompletionMarkerBeforePersist(t *testing.T) {
	stub := &messageServiceStub{}
	handler := &Handler{messageService: stub}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	message := &types.Message{
		ID:               "msg-complete-marker",
		SessionID:        "sess-1",
		Role:             "assistant",
		Content:          "## 总结\n\n正文\n\n" + types.ChatDocumentCompletionMarker,
		CompletionStatus: types.MessageCompletionStatusPending,
	}

	ok := handler.completeAssistantMessage(ctx, message, "继续输出", assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "stop",
		AllowIndexing:    false,
		AllowComplete:    true,
		RegisterArtifactOptions: types.RegisterChatDocumentArtifactOptions{
			Intent:    types.ChatDocumentIntentContinue,
			Operation: types.ChatDocumentOperationContinue,
		},
	})

	require.True(t, ok)
	assert.NotContains(t, message.Content, types.ChatDocumentCompletionMarker)
	require.Len(t, stub.updatedMessages, 1)
	assert.NotContains(t, stub.updatedMessages[0].Content, types.ChatDocumentCompletionMarker)
}

func TestEmitAssistantCompleteEvent_UsesFetchModeForOversizedSnapshot(t *testing.T) {
	eventBus := event.NewEventBus()
	message := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant", Content: "delta"}
	artifact := &types.ChatDocumentArtifact{
		ID:              "artifact-oversized",
		SessionID:       "sess-1",
		SourceMessageID: "msg-1",
		RevisionNo:      3,
		ContentSnapshot: strings.Repeat("超长正文", types.ChatDocumentArtifactInlineContinuationMaxChars/4+10),
	}

	var captured event.AgentCompleteData
	eventBus.On(event.EventAgentComplete, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		captured = data
		return nil
	})

	emitAssistantCompleteEvent(eventBus, "sess-1", message, assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "stop",
		AllowIndexing:    true,
		AllowComplete:    true,
	}, artifact)

	assert.Equal(t, types.ChatDocumentFinalDocumentModeFetchArtifactSnapshot, captured.FinalDocumentMode)
	assert.Equal(t, "artifact-oversized", captured.FinalDocumentArtifactID)
	assert.Empty(t, captured.FinalDocument)
}

func TestAgentComplete_HandlerRegistersArtifactBeforeStreamCompleteEvent(t *testing.T) {
	messageStub := &messageServiceStub{}
	artifactStub := &chatDocumentArtifactServiceStub{registerResult: &types.ChatDocumentArtifact{
		ID:                       "artifact-edit-1",
		SessionID:                "sess-1",
		SourceMessageID:          "msg-1",
		RevisionNo:               2,
		Operation:                types.ChatDocumentOperationRevise,
		Status:                   types.ChatDocumentArtifactStatusAvailable,
		ArtifactKind:             types.ChatDocumentArtifactKindMarkdown,
		ContentSnapshot:          "# 完整文档\n\n## 第4章 核心功能设计\n\n补充后的完整内容",
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusCompleted,
	}}
	streamStub := &streamManagerStub{}
	handler := &Handler{
		messageService:              messageStub,
		chatDocumentArtifactService: artifactStub,
		streamManager:               streamStub,
	}
	assistantMessage := &types.Message{ID: "msg-1", SessionID: "sess-1", RequestID: "req-1", Role: "assistant"}
	eventBus := event.NewEventBus()

	var artifactMu sync.RWMutex
	var completedArtifact *types.ChatDocumentArtifact
	setCompletedArtifact := func(artifact *types.ChatDocumentArtifact) {
		artifactMu.Lock()
		completedArtifact = artifact
		artifactMu.Unlock()
	}
	getCompletedArtifact := func() *types.ChatDocumentArtifact {
		artifactMu.RLock()
		defer artifactMu.RUnlock()
		return completedArtifact
	}

	agentCompletionOptions := agentAssistantCompletionOptions()
	agentCompletionOptions.RegisterArtifactOptions = types.RegisterChatDocumentArtifactOptions{
		Intent:       types.ChatDocumentIntentRevise,
		Operation:    types.ChatDocumentOperationRevise,
		OutputMode:   types.ChatDocumentOutputModeDelta,
		NeedArtifact: true,
		BaseArtifact: &types.ChatDocumentArtifact{ID: "artifact-base-1", ContentSnapshot: "# 原始完整文档"},
	}
	agentCompletionOptions.ArtifactObserver = setCompletedArtifact

	eventBus.On(event.EventAgentComplete, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		if !ok {
			return nil
		}
		nextOptions := completionOptionsFromComplete(data)
		nextOptions.RegisterArtifactOptions = agentCompletionOptions.RegisterArtifactOptions
		nextOptions.ArtifactObserver = agentCompletionOptions.ArtifactObserver
		agentCompletionOptions = nextOptions
		handler.completeAssistantMessageInPlace(context.Background(), assistantMessage, "补充第4章 核心功能设计章节的功能清单和功能描述。", &agentCompletionOptions)
		return nil
	})
	handler.setupStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", time.Time{}, assistantMessage, eventBus, getCompletedArtifact)

	require.NoError(t, eventBus.Emit(context.Background(), event.Event{
		ID:        "complete-1",
		Type:      event.EventAgentComplete,
		SessionID: "sess-1",
		Data: event.AgentCompleteData{
			SessionID:        "sess-1",
			MessageID:        "msg-1",
			FinalAnswer:      "<document_patch><append heading=\"## 第4章 核心功能设计\">补充内容</append></document_patch>",
			CompletionStatus: types.MessageCompletionStatusCompleted,
			FinishReason:     "stop",
			AllowIndexing:    true,
			AllowComplete:    true,
		},
	}))

	require.NotEmpty(t, streamStub.events)
	lastEvent := streamStub.events[len(streamStub.events)-1]
	assert.Equal(t, types.ResponseTypeComplete, lastEvent.Type)
	assert.Equal(t, "artifact-edit-1", lastEvent.Data["final_document_artifact_id"])
	artifactMetadata, ok := lastEvent.Data["chat_document_artifact"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "artifact-edit-1", artifactMetadata["id"])
	assert.Equal(t, types.ChatDocumentGenerationStatusCompleted, lastEvent.Data["document_generation_status"])
	assert.Equal(t, types.ChatDocumentFinalDocumentModeInlineSnapshot, lastEvent.Data["final_document_mode"])
	assert.Equal(t, "# 完整文档\n\n## 第4章 核心功能设计\n\n补充后的完整内容", lastEvent.Data["final_document"])
	assert.Equal(t, "artifact-edit-1", getCompletedArtifact().ID)
}

func TestCompleteAssistantMessage_DoesNotOverrideTerminalState(t *testing.T) {
	stub := &messageServiceStub{}
	handler := &Handler{messageService: stub}
	message := &types.Message{
		ID:               "msg-1",
		SessionID:        "sess-1",
		Role:             "assistant",
		Content:          "partial content",
		CompletionStatus: types.MessageCompletionStatusCancelled,
		FinishReason:     "cancelled",
		FailureReason:    "cancelled",
	}

	updated := handler.completeAssistantMessage(context.Background(), message, "query", assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "stop",
		AllowIndexing:    true,
		AllowComplete:    true,
	})

	assert.False(t, updated)
	assert.Empty(t, stub.updatedMessages)
	assert.Equal(t, types.MessageCompletionStatusCancelled, message.CompletionStatus)
	assert.Equal(t, 0, stub.indexedCalls)
}

func TestCompleteAssistantMessage_DoesNotOverrideStoredCancelledState(t *testing.T) {
	stub := &messageServiceStub{getMessageResult: &types.Message{
		ID:               "msg-1",
		SessionID:        "sess-1",
		Role:             "assistant",
		Content:          "cancelled content",
		CompletionStatus: types.MessageCompletionStatusCancelled,
		FinishReason:     "cancelled",
		FailureReason:    "cancelled",
	}}
	handler := &Handler{messageService: stub}
	message := &types.Message{
		ID:               "msg-1",
		SessionID:        "sess-1",
		Role:             "assistant",
		Content:          "stale in-memory content",
		CompletionStatus: types.MessageCompletionStatusPending,
	}

	updated := handler.completeAssistantMessage(context.Background(), message, "query", assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "stop",
		AllowIndexing:    true,
		AllowComplete:    true,
	})

	assert.False(t, updated)
	assert.Empty(t, stub.updatedMessages)
	assert.Equal(t, types.MessageCompletionStatusCancelled, message.CompletionStatus)
	assert.Equal(t, "cancelled", message.FinishReason)
	assert.Equal(t, "cancelled content", message.Content)
	assert.Equal(t, 0, stub.indexedCalls)
}

func TestCompleteAssistantMessage_ReturnsFalseWhenUpdateFails(t *testing.T) {
	stub := &messageServiceStub{updateMessageErr: errors.New("update failed")}
	handler := &Handler{messageService: stub}
	message := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant", Content: "final content"}

	updated := handler.completeAssistantMessage(context.Background(), message, "query", assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "stop",
		AllowIndexing:    true,
		AllowComplete:    true,
	})

	assert.False(t, updated)
	assert.False(t, message.IsCompleted)
	assert.Empty(t, message.CompletionStatus)
	assert.Empty(t, stub.updatedMessages)
	assert.Equal(t, 0, stub.indexedCalls)
}

func TestMessageUpdateContext_PreservesTenantAfterParentCancellation(t *testing.T) {
	baseCtx, cancel := context.WithCancel(context.Background())
	cancel()

	updateCtx := messageUpdateContext(baseCtx, 42)

	assert.NoError(t, updateCtx.Err())
	tenantID, ok := updateCtx.Value(types.TenantIDContextKey).(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(42), tenantID)
}

func TestCompletionOptionsFromError_MarksFailedAndDisablesIndexing(t *testing.T) {
	options := completionOptionsFromError(event.ErrorData{Stage: "agent_execution", Error: "boom"})

	assert.Equal(t, types.MessageCompletionStatusFailed, options.CompletionStatus)
	assert.Equal(t, "error", options.FinishReason)
	assert.Equal(t, "agent_execution", options.FailureReason)
	assert.False(t, options.AllowIndexing)
	assert.False(t, options.AllowComplete)
}

func TestCompleteAssistantMessage_FailedAgentCompletionDoesNotIndexOrMarkCompleted(t *testing.T) {
	stub := &messageServiceStub{}
	handler := &Handler{messageService: stub}
	message := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant", Content: ""}
	options := completionOptionsFromComplete(event.AgentCompleteData{
		CompletionStatus: types.MessageCompletionStatusFailed,
		FinishReason:     "llm_error",
		FailureReason:    "llm_error",
		AllowIndexing:    false,
		AllowComplete:    false,
	})

	updated := handler.completeAssistantMessage(context.Background(), message, "query", options)

	assert.True(t, updated)
	require.Len(t, stub.updatedMessages, 1)
	assert.False(t, message.IsCompleted)
	assert.Equal(t, types.MessageCompletionStatusFailed, message.CompletionStatus)
	assert.Equal(t, "llm_error", message.FinishReason)
	assert.Equal(t, "llm_error", message.FailureReason)
	assert.Equal(t, 0, stub.indexedCalls)
}

func TestCompleteAssistantMessage_RejectsEmptyCompletedDocumentEdit(t *testing.T) {
	stub := &messageServiceStub{}
	artifactStub := &chatDocumentArtifactServiceStub{}
	handler := &Handler{messageService: stub, chatDocumentArtifactService: artifactStub}
	message := &types.Message{ID: "msg-1", SessionID: "sess-1", RequestID: "req-1", Role: "assistant", Content: ""}

	updated := handler.completeAssistantMessage(context.Background(), message, "revise", assistantCompletionOptions{
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "stop",
		AllowIndexing:    true,
		AllowComplete:    true,
		AgentMode:        true,
		RegisterArtifactOptions: types.RegisterChatDocumentArtifactOptions{
			Intent:       types.ChatDocumentIntentRevise,
			Operation:    types.ChatDocumentOperationRevise,
			OutputMode:   types.ChatDocumentOutputModeDelta,
			BaseArtifact: &types.ChatDocumentArtifact{ID: "base-1", ContentSnapshot: "# 基线"},
		},
	})

	assert.True(t, updated)
	require.Len(t, stub.updatedMessages, 1)
	assert.False(t, message.IsCompleted)
	assert.Equal(t, types.MessageCompletionStatusFailed, message.CompletionStatus)
	assert.Equal(t, "empty_document_edit_completion", message.FinishReason)
	assert.Equal(t, "empty_document_edit_completion", message.FailureReason)
	assert.Equal(t, 0, stub.indexedCalls)
	assert.Empty(t, artifactStub.registeredArtifacts)
}

func TestCompleteAssistantMessage_DowngradesInvalidCompletedFullDocument(t *testing.T) {
	stub := &messageServiceStub{}
	artifactStub := &chatDocumentArtifactServiceStub{}
	handler := &Handler{messageService: stub, chatDocumentArtifactService: artifactStub}
	message := &types.Message{
		ID:        "msg-1",
		SessionID: "sess-1",
		RequestID: "req-1",
		Role:      "assistant",
		Content:   "# 北海电厂二期智慧电厂项目投标技术方案\n\n## 项目背景与建设目标\n\n仅输出了第一章。",
	}

	updated := handler.completeAssistantMessage(context.Background(), message, "生成完整文档", assistantCompletionOptions{
		CompletionStatus:         types.MessageCompletionStatusCompleted,
		FinishReason:             "stop",
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusCompleted,
		AllowIndexing:            true,
		AllowComplete:            true,
		AgentMode:                true,
		Extra: map[string]interface{}{
			"outline": map[string]interface{}{
				"title":    "北海电厂二期智慧电厂项目投标技术方案",
				"sections": []interface{}{"项目背景与建设目标", "总体技术架构与模块划分", "智慧运行系统方案"},
			},
		},
		RegisterArtifactOptions: types.RegisterChatDocumentArtifactOptions{
			OutputMode:               types.ChatDocumentOutputModeFull,
			DocumentGenerationStatus: types.ChatDocumentGenerationStatusCompleted,
		},
	})

	assert.True(t, updated)
	require.Len(t, stub.updatedMessages, 1)
	assert.False(t, message.IsCompleted)
	assert.Equal(t, types.MessageCompletionStatusPartial, message.CompletionStatus)
	assert.Equal(t, "outline_or_section_incomplete", message.FinishReason)
	assert.Equal(t, "outline_or_section_incomplete", message.FailureReason)
	assert.Equal(t, 0, stub.indexedCalls)
	require.Len(t, artifactStub.registeredArtifacts, 1)
	assert.Equal(t, types.ChatDocumentGenerationStatusContinuing, artifactStub.lastRegisterOptions.DocumentGenerationStatus)
}

func TestCompleteAssistantMessage_PrefersAuthoritativeFinalAnswerForFullDocument(t *testing.T) {
	stub := &messageServiceStub{}
	artifactStub := &chatDocumentArtifactServiceStub{}
	handler := &Handler{messageService: stub, chatDocumentArtifactService: artifactStub}
	message := &types.Message{
		ID:        "msg-1",
		SessionID: "sess-1",
		RequestID: "req-1",
		Role:      "assistant",
		Content:   "以下为流式累积正文，请以最终整理稿为准。\n#北海电厂二期智慧电厂项目投标技术方案\n\n##第1章 项目背景与建设目标\n\n正文覆盖项目背景。\n\n##第2章 总体技术架构与模块划分\n\n正文覆盖总体架构。",
	}
	finalAnswer := "# 北海电厂二期智慧电厂项目投标技术方案\n\n## 第1章 项目背景与建设目标\n\n正文覆盖项目背景。\n\n## 第2章 总体技术架构与模块划分\n\n正文覆盖总体架构。"

	updated := handler.completeAssistantMessage(context.Background(), message, "生成完整文档", assistantCompletionOptions{
		CompletionStatus:         types.MessageCompletionStatusCompleted,
		FinishReason:             "stop",
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusCompleted,
		AllowIndexing:            false,
		AllowComplete:            true,
		AgentMode:                true,
		FinalAnswer:              finalAnswer,
		Extra: map[string]interface{}{
			"outline": map[string]interface{}{
				"title":    "北海电厂二期智慧电厂项目投标技术方案",
				"sections": []interface{}{"项目背景与建设目标", "总体技术架构与模块划分"},
			},
		},
		RegisterArtifactOptions: types.RegisterChatDocumentArtifactOptions{
			OutputMode:               types.ChatDocumentOutputModeFull,
			DocumentGenerationStatus: types.ChatDocumentGenerationStatusCompleted,
		},
	})

	assert.True(t, updated)
	require.Len(t, stub.updatedMessages, 1)
	assert.True(t, message.IsCompleted)
	assert.Equal(t, types.MessageCompletionStatusCompleted, message.CompletionStatus)
	assert.Equal(t, "stop", message.FinishReason)
	assert.Equal(t, "", message.FailureReason)
	assert.Equal(t, finalAnswer, message.Content)
	require.Len(t, artifactStub.registeredArtifacts, 1)
	assert.Equal(t, finalAnswer, artifactStub.registeredArtifacts[0].Content)
	assert.Equal(t, types.ChatDocumentGenerationStatusCompleted, artifactStub.lastRegisterOptions.DocumentGenerationStatus)
	assert.Equal(t, 0, stub.indexedCalls)
}

func TestCompleteAssistantMessage_AllowsCompletedNeedsReviewFullDocument(t *testing.T) {
	stub := &messageServiceStub{indexedCallCh: make(chan struct{}, 1)}
	artifactStub := &chatDocumentArtifactServiceStub{}
	handler := &Handler{messageService: stub, chatDocumentArtifactService: artifactStub}
	message := &types.Message{
		ID:        "msg-1",
		SessionID: "sess-1",
		RequestID: "req-1",
		Role:      "assistant",
		Content:   "# 北海电厂二期智慧电厂项目投标技术方案\n\n## 第1章 项目背景与建设目标\n\n正文覆盖项目背景。",
	}

	updated := handler.completeAssistantMessage(context.Background(), message, "生成完整文档", assistantCompletionOptions{
		CompletionStatus:         types.MessageCompletionStatusCompleted,
		FinishReason:             "stop",
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusNeedsReview,
		AllowIndexing:            true,
		AllowComplete:            true,
		AgentMode:                true,
		Extra: map[string]interface{}{
			"quality_issues": []string{types.ChatDocumentQualityIssueMarkdownUnplannedSubsection},
			"outline": map[string]interface{}{
				"title": "北海电厂二期智慧电厂项目投标技术方案",
				"sections": []map[string]interface{}{
					{"number": 1, "title": "项目背景与建设目标", "heading": "第1章 项目背景与建设目标"},
					{"number": 2, "title": "总体技术架构与模块划分", "heading": "第2章 总体技术架构与模块划分"},
				},
			},
		},
		RegisterArtifactOptions: types.RegisterChatDocumentArtifactOptions{
			OutputMode:               types.ChatDocumentOutputModeFull,
			DocumentGenerationStatus: types.ChatDocumentGenerationStatusNeedsReview,
		},
	})

	assert.True(t, updated)
	require.Len(t, stub.updatedMessages, 1)
	assert.True(t, message.IsCompleted)
	assert.Equal(t, types.MessageCompletionStatusCompleted, message.CompletionStatus)
	assert.Equal(t, "stop", message.FinishReason)
	assert.Empty(t, message.FailureReason)
	require.Len(t, artifactStub.registeredArtifacts, 1)
	assert.Equal(t, types.ChatDocumentGenerationStatusNeedsReview, artifactStub.lastRegisterOptions.DocumentGenerationStatus)
	assert.Equal(t, []string{types.ChatDocumentQualityIssueMarkdownUnplannedSubsection}, artifactStub.lastRegisterOptions.QualityIssues)
}
