package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/event"
	chat "github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type translationChatStub struct {
	responses []types.ChatResponse
	errs      []error
	calls     int
	messages  [][]chat.Message
}

func (s *translationChatStub) Chat(_ context.Context, messages []chat.Message, _ *chat.ChatOptions) (*types.ChatResponse, error) {
	s.calls++
	s.messages = append(s.messages, append([]chat.Message(nil), messages...))
	idx := s.calls - 1
	if idx < len(s.errs) && s.errs[idx] != nil {
		return nil, s.errs[idx]
	}
	if idx < len(s.responses) {
		response := s.responses[idx]
		return &response, nil
	}
	return &types.ChatResponse{}, nil
}

func (s *translationChatStub) ChatStream(context.Context, []chat.Message, *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	return nil, errors.New("not implemented")
}

func (s *translationChatStub) GetModelName() string { return "translation-chat-stub" }

func (s *translationChatStub) GetModelID() string { return "translation-chat-stub" }

type translationKnowledgeServiceStub struct {
	interfaces.KnowledgeService
	knowledge *types.Knowledge
}

func (s *translationKnowledgeServiceStub) GetKnowledgeByID(context.Context, string) (*types.Knowledge, error) {
	return s.knowledge, nil
}

type translationChunkRepositoryStub struct {
	interfaces.ChunkRepository
	chunks []*types.Chunk
}

func (s *translationChunkRepositoryStub) ListChunksByKnowledgeID(context.Context, uint64, string) ([]*types.Chunk, error) {
	return append([]*types.Chunk(nil), s.chunks...), nil
}

type translationChunkServiceStub struct {
	interfaces.ChunkService
	repo interfaces.ChunkRepository
}

func (s *translationChunkServiceStub) GetRepository() interfaces.ChunkRepository {
	return s.repo
}

func TestShouldUseLongDocumentTranslationPath(t *testing.T) {
	req := &types.QARequest{
		DocumentOutputMode: types.ChatDocumentOutputModeFull,
		DocumentTaskKind:   types.ChatDocumentTaskKindTranslation,
		KnowledgeIDs:       []string{"knowledge-1"},
	}
	assert.True(t, shouldUseLongDocumentTranslationPath(req))

	req.GenerationRunID = "run-1"
	assert.False(t, shouldUseLongDocumentTranslationPath(req))

	req.GenerationRunID = ""
	req.Attachments = types.MessageAttachments{{FileName: "appendix.txt"}}
	assert.False(t, shouldUseLongDocumentTranslationPath(req))

	req.Attachments = nil
	req.ImageURLs = []string{"https://example.com/image.png"}
	assert.False(t, shouldUseLongDocumentTranslationPath(req))
}

func TestShouldUseLongDocumentTranslationContinuationPath(t *testing.T) {
	req := &types.QARequest{
		AutoContinue:       true,
		GenerationRunID:    "run-1",
		DocumentOutputMode: types.ChatDocumentOutputModeDelta,
		DocumentTaskKind:   types.ChatDocumentTaskKindTranslation,
	}
	assert.True(t, shouldUseLongDocumentTranslationContinuationPath(req))

	req.Attachments = types.MessageAttachments{{FileName: "appendix.txt"}}
	assert.False(t, shouldUseLongDocumentTranslationContinuationPath(req))

	req.Attachments = nil
	req.DocumentOutputMode = types.ChatDocumentOutputModeFull
	assert.True(t, shouldUseLongDocumentTranslationContinuationPath(req))

	req.GenerationRunID = ""
	assert.False(t, shouldUseLongDocumentTranslationContinuationPath(req))
}

func TestRunLongDocumentTranslationPath_EmitsCompletedRunAndMetadata(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(1))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-1")
	ctx = context.WithValue(ctx, types.LanguageContextKey, "en-US")

	runRepo := &fullDocumentGenerationRunRepoStub{}
	svc := &sessionService{
		cfg:               &config.Config{LongDocument: &config.LongDocumentConfig{BatchChunkSize: 1, BatchMaxChars: 1024}},
		generationRunRepo: runRepo,
		knowledgeService: &translationKnowledgeServiceStub{knowledge: &types.Knowledge{
			ID:       "knowledge-1",
			TenantID: 1,
			Title:    "原始文档",
			FileName: "source.md",
		}},
		chunkService: &translationChunkServiceStub{repo: &translationChunkRepositoryStub{chunks: []*types.Chunk{
			{KnowledgeID: "knowledge-1", ChunkIndex: 1, Content: "# 原始文档\n\n第一段", StartAt: 0, EndAt: 8},
			{KnowledgeID: "knowledge-1", ChunkIndex: 2, Content: "## 第二节\n\n第二段", StartAt: 8, EndAt: 18},
		}}},
	}
	chatStub := &translationChatStub{responses: []types.ChatResponse{
		{Content: "# Translated Document\n\nFirst paragraph"},
		{Content: "## Second Section\n\nSecond paragraph"},
	}}
	req := &types.QARequest{
		Session:            &types.Session{ID: "session-1"},
		AssistantMessageID: "assistant-1",
		DocumentOutputMode: types.ChatDocumentOutputModeFull,
		DocumentTaskKind:   types.ChatDocumentTaskKindTranslation,
		KnowledgeIDs:       []string{"knowledge-1"},
		Query:              "请把这份文档翻译成英文",
		TranslationOptions: &types.ChatDocumentTranslationOptions{SourceLanguage: "auto", TargetLanguage: "English", PreserveStructure: true, OutputFormat: "markdown"},
	}

	eventBus := event.NewEventBus()
	var finalAnswer event.AgentFinalAnswerData
	var complete event.AgentCompleteData
	eventBus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		if ok && data.Done {
			finalAnswer = data
		}
		return nil
	})
	eventBus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		if ok {
			complete = data
		}
		return nil
	})

	err := svc.runLongDocumentTranslationPath(ctx, req, eventBus, chatStub, nil)
	require.NoError(t, err)
	require.NotNil(t, runRepo.created)
	require.NotNil(t, runRepo.updated)

	assert.Equal(t, types.MessageCompletionStatusCompleted, complete.CompletionStatus)
	assert.Equal(t, types.ChatDocumentGenerationStatusCompleted, complete.DocumentGenerationStatus)
	assert.NotEmpty(t, complete.Extra["generation_run_id"])
	assert.Equal(t, complete.Extra["generation_run_id"], finalAnswer.Extra["generation_run_id"])
	assert.Equal(t, types.ChatDocumentGenerationRunStatusCompleted, runRepo.updated.Status)
	assert.Contains(t, complete.FinalAnswer, "Translated Document")
	assert.NotContains(t, complete.FinalAnswer, "# 原始文档")
	assert.Len(t, chatStub.messages, 2)

	var outline longDocumentTranslationRunOutline
	require.NoError(t, json.Unmarshal(runRepo.created.OutlineJSON, &outline))
	assert.Equal(t, "knowledge-1", outline.KnowledgeID)
	assert.Len(t, outline.Segments, 2)

	var completedSegments []string
	require.NoError(t, json.Unmarshal(runRepo.updated.CompletedSectionsJSON, &completedSegments))
	assert.Equal(t, []string{"segment-1", "segment-2"}, completedSegments)

	progress, ok := complete.Extra["translation_progress"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 2, int(progress["total_segments"].(int)))
	state, ok := complete.Extra["generation_run_state"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, types.ChatDocumentTaskKindTranslation, state["task_kind"])
	assert.Equal(t, 8, state["max_auto_continue_rounds"])
	assert.Equal(t, 200, state["min_growth_chars"])
	feedback := unmarshalGenerationRunRuntimeFeedback(runRepo.updated.RuntimeFeedbackJSON)
	assert.Equal(t, types.ChatDocumentTaskKindTranslation, feedback.TaskKind)
	assert.Equal(t, 2, feedback.CompletedCount)
	assert.Zero(t, feedback.RemainingCount)
}

func TestRunLongDocumentTranslationPath_RetriesEmptyBatchOutput(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(1))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-1")

	runRepo := &fullDocumentGenerationRunRepoStub{}
	svc := &sessionService{
		cfg:               &config.Config{LongDocument: &config.LongDocumentConfig{BatchChunkSize: 1, BatchMaxChars: 1024}},
		generationRunRepo: runRepo,
		knowledgeService: &translationKnowledgeServiceStub{knowledge: &types.Knowledge{
			ID:       "knowledge-1",
			TenantID: 1,
			Title:    "原始文档",
		}},
		chunkService: &translationChunkServiceStub{repo: &translationChunkRepositoryStub{chunks: []*types.Chunk{
			{KnowledgeID: "knowledge-1", ChunkIndex: 1, Content: "第一段", StartAt: 0, EndAt: 3},
		}}},
	}
	chatStub := &translationChatStub{responses: []types.ChatResponse{
		{Content: "   "},
		{Content: "First paragraph"},
	}}
	req := &types.QARequest{
		Session:            &types.Session{ID: "session-1"},
		AssistantMessageID: "assistant-1",
		DocumentOutputMode: types.ChatDocumentOutputModeFull,
		DocumentTaskKind:   types.ChatDocumentTaskKindTranslation,
		KnowledgeIDs:       []string{"knowledge-1"},
		Query:              "请把这份文档翻译成英文",
	}

	eventBus := event.NewEventBus()
	var complete event.AgentCompleteData
	eventBus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		if ok {
			complete = data
		}
		return nil
	})

	err := svc.runLongDocumentTranslationPath(ctx, req, eventBus, chatStub, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, chatStub.calls)
	assert.Equal(t, types.MessageCompletionStatusCompleted, complete.CompletionStatus)
	assert.Equal(t, types.ChatDocumentGenerationStatusCompleted, complete.DocumentGenerationStatus)
	assert.Contains(t, complete.FinalAnswer, "First paragraph")
}

func TestRunLongDocumentTranslationPath_EmitsPartialOnBatchFailure(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(1))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-1")

	runRepo := &fullDocumentGenerationRunRepoStub{}
	svc := &sessionService{
		cfg:               &config.Config{LongDocument: &config.LongDocumentConfig{BatchChunkSize: 1, BatchMaxChars: 1024}},
		generationRunRepo: runRepo,
		knowledgeService: &translationKnowledgeServiceStub{knowledge: &types.Knowledge{
			ID:       "knowledge-1",
			TenantID: 1,
			Title:    "原始文档",
		}},
		chunkService: &translationChunkServiceStub{repo: &translationChunkRepositoryStub{chunks: []*types.Chunk{
			{KnowledgeID: "knowledge-1", ChunkIndex: 1, Content: "第一段", StartAt: 0, EndAt: 3},
			{KnowledgeID: "knowledge-1", ChunkIndex: 2, Content: "第二段", StartAt: 3, EndAt: 6},
		}}},
	}
	chatStub := &translationChatStub{
		responses: []types.ChatResponse{{Content: "First paragraph"}},
		errs:      []error{nil, errors.New("llm timeout")},
	}
	req := &types.QARequest{
		Session:            &types.Session{ID: "session-1"},
		AssistantMessageID: "assistant-1",
		DocumentOutputMode: types.ChatDocumentOutputModeFull,
		DocumentTaskKind:   types.ChatDocumentTaskKindTranslation,
		KnowledgeIDs:       []string{"knowledge-1"},
		Query:              "请把这份文档翻译成英文",
	}

	eventBus := event.NewEventBus()
	var complete event.AgentCompleteData
	eventBus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		if ok {
			complete = data
		}
		return nil
	})

	err := svc.runLongDocumentTranslationPath(ctx, req, eventBus, chatStub, nil)
	require.NoError(t, err)
	assert.Equal(t, types.MessageCompletionStatusPartial, complete.CompletionStatus)
	assert.Equal(t, types.ChatDocumentGenerationStatusContinuing, complete.DocumentGenerationStatus)
	assert.Contains(t, complete.FinalAnswer, "First paragraph")
	assert.Equal(t, types.ChatDocumentGenerationRunStatusContinuing, runRepo.updated.Status)

	var completedSegments []string
	require.NoError(t, json.Unmarshal(runRepo.updated.CompletedSectionsJSON, &completedSegments))
	assert.Equal(t, []string{"segment-1"}, completedSegments)
}

func TestRunLongDocumentTranslationPath_EmitsFailedCompletionWhenFirstBatchFails(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(1))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-1")

	runRepo := &fullDocumentGenerationRunRepoStub{}
	svc := &sessionService{
		cfg:               &config.Config{LongDocument: &config.LongDocumentConfig{BatchChunkSize: 1, BatchMaxChars: 1024}},
		generationRunRepo: runRepo,
		knowledgeService: &translationKnowledgeServiceStub{knowledge: &types.Knowledge{
			ID:       "knowledge-1",
			TenantID: 1,
			Title:    "原始文档",
		}},
		chunkService: &translationChunkServiceStub{repo: &translationChunkRepositoryStub{chunks: []*types.Chunk{
			{KnowledgeID: "knowledge-1", ChunkIndex: 1, Content: "第一段", StartAt: 0, EndAt: 3},
		}}},
	}
	chatStub := &translationChatStub{
		errs: []error{errors.New("llm timeout"), errors.New("llm timeout")},
	}
	req := &types.QARequest{
		Session:            &types.Session{ID: "session-1"},
		AssistantMessageID: "assistant-1",
		DocumentOutputMode: types.ChatDocumentOutputModeFull,
		DocumentTaskKind:   types.ChatDocumentTaskKindTranslation,
		KnowledgeIDs:       []string{"knowledge-1"},
		Query:              "请把这份文档翻译成英文",
	}

	eventBus := event.NewEventBus()
	var finalAnswer event.AgentFinalAnswerData
	var complete event.AgentCompleteData
	eventBus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		if ok && data.Done {
			finalAnswer = data
		}
		return nil
	})
	eventBus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		if ok {
			complete = data
		}
		return nil
	})

	err := svc.runLongDocumentTranslationPath(ctx, req, eventBus, chatStub, nil)
	require.NoError(t, err)
	assert.Equal(t, types.MessageCompletionStatusFailed, complete.CompletionStatus)
	assert.Equal(t, "llm_timeout", complete.FinishReason)
	assert.Equal(t, types.ChatDocumentGenerationStatusBlocked, complete.DocumentGenerationStatus)
	assert.NotEmpty(t, complete.Extra["generation_run_id"])
	assert.Equal(t, complete.Extra["generation_run_id"], finalAnswer.Extra["generation_run_id"])
	assert.Equal(t, types.MessageCompletionStatusFailed, finalAnswer.CompletionStatus)
	assert.Equal(t, types.ChatDocumentGenerationRunStatusFailed, runRepo.updated.Status)

	var completedSegments []string
	require.NoError(t, json.Unmarshal(runRepo.updated.CompletedSectionsJSON, &completedSegments))
	assert.Empty(t, completedSegments)
}

func TestRunLongDocumentTranslationContinuationPath_UsesRemainingSegments(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(1))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-1")

	runOutline := longDocumentTranslationRunOutline{
		TaskKind:       types.ChatDocumentTaskKindTranslation,
		KnowledgeID:    "knowledge-1",
		KnowledgeTitle: "原始文档",
		SourceSnapshotHash: buildLongDocumentSnapshotHash([]*types.Chunk{
			{KnowledgeID: "knowledge-1", ChunkIndex: 1, Content: "第一段", StartAt: 0, EndAt: 3},
			{KnowledgeID: "knowledge-1", ChunkIndex: 2, Content: "第二段", StartAt: 3, EndAt: 6},
			{KnowledgeID: "knowledge-1", ChunkIndex: 3, Content: "第三段", StartAt: 6, EndAt: 9},
		}),
		TargetLanguage:    "English",
		OutputFormat:      "markdown",
		PreserveStructure: true,
		Segments: []longDocumentTranslationRunSegment{
			{ID: "segment-1", BatchNo: 1, ChunkStartSeq: 1, ChunkEndSeq: 1},
			{ID: "segment-2", BatchNo: 2, ChunkStartSeq: 2, ChunkEndSeq: 2},
			{ID: "segment-3", BatchNo: 3, ChunkStartSeq: 3, ChunkEndSeq: 3},
		},
	}
	runRepo := &fullDocumentGenerationRunRepoStub{runs: map[string]*types.ChatDocumentGenerationRun{
		"run-1": {
			ID:                    "run-1",
			TenantID:              1,
			SessionID:             "session-1",
			OutlineJSON:           marshalGenerationRunJSON(runOutline),
			CompletedSectionsJSON: marshalGenerationRunJSON([]string{"segment-1"}),
			Status:                types.ChatDocumentGenerationRunStatusContinuing,
		},
	}}
	svc := &sessionService{
		cfg:               &config.Config{LongDocument: &config.LongDocumentConfig{BatchChunkSize: 1, BatchMaxChars: 1024}},
		generationRunRepo: runRepo,
		knowledgeService: &translationKnowledgeServiceStub{knowledge: &types.Knowledge{
			ID:       "knowledge-1",
			TenantID: 1,
			Title:    "原始文档",
		}},
		chunkService: &translationChunkServiceStub{repo: &translationChunkRepositoryStub{chunks: []*types.Chunk{
			{KnowledgeID: "knowledge-1", ChunkIndex: 1, Content: "第一段", StartAt: 0, EndAt: 3},
			{KnowledgeID: "knowledge-1", ChunkIndex: 2, Content: "第二段", StartAt: 3, EndAt: 6},
			{KnowledgeID: "knowledge-1", ChunkIndex: 3, Content: "第三段", StartAt: 6, EndAt: 9},
		}}},
	}
	chatStub := &translationChatStub{responses: []types.ChatResponse{
		{Content: "Second paragraph"},
		{Content: "Third paragraph"},
	}}
	req := &types.QARequest{
		Session:            &types.Session{ID: "session-1"},
		AssistantMessageID: "assistant-1",
		DocumentOutputMode: types.ChatDocumentOutputModeDelta,
		DocumentTaskKind:   types.ChatDocumentTaskKindTranslation,
		AutoContinue:       true,
		GenerationRunID:    "run-1",
		BaseArtifactID:     "artifact-1",
		Query:              "以当前文档为基准，继续剩余内容输出",
	}

	eventBus := event.NewEventBus()
	var complete event.AgentCompleteData
	eventBus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		if ok {
			complete = data
		}
		return nil
	})

	err := svc.runLongDocumentTranslationContinuationPath(ctx, req, eventBus, chatStub, nil)
	require.NoError(t, err)
	assert.Equal(t, types.MessageCompletionStatusCompleted, complete.CompletionStatus)
	assert.Equal(t, types.ChatDocumentGenerationStatusCompleted, complete.DocumentGenerationStatus)
	assert.Equal(t, "Second paragraph\n\nThird paragraph", complete.FinalAnswer)
	assert.Len(t, chatStub.messages, 2)
	assert.Equal(t, types.ChatDocumentGenerationRunStatusCompleted, runRepo.updated.Status)

	var completedSegments []string
	require.NoError(t, json.Unmarshal(runRepo.updated.CompletedSectionsJSON, &completedSegments))
	assert.Equal(t, []string{"segment-1", "segment-2", "segment-3"}, completedSegments)

	progress, ok := complete.Extra["translation_progress"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 3, int(progress["total_segments"].(int)))
	assert.Equal(t, 3, int(progress["completed_segments"].(int)))
	assert.Equal(t, 0, int(progress["remaining_segments"].(int)))
}
