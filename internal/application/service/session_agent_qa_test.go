package service

import (
	"context"
	"strings"
	"testing"
	"time"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/models/asr"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/models/rerank"
	"github.com/Tencent/WeKnora/internal/models/vlm"
	"github.com/Tencent/WeKnora/internal/types"
	interfaces "github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type blockingDocumentEditChat struct {
	responses chan types.StreamResponse
	started   chan struct{}
}

func (m *blockingDocumentEditChat) Chat(context.Context, []chat.Message, *chat.ChatOptions) (*types.ChatResponse, error) {
	return nil, nil
}

func (m *blockingDocumentEditChat) ChatStream(context.Context, []chat.Message, *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	close(m.started)
	return m.responses, nil
}

func (m *blockingDocumentEditChat) GetModelName() string { return "blocking-document-edit-chat" }

func (m *blockingDocumentEditChat) GetModelID() string { return "blocking-document-edit-chat" }

type failIfCalledDocumentEditChat struct {
	called bool
}

func (m *failIfCalledDocumentEditChat) Chat(context.Context, []chat.Message, *chat.ChatOptions) (*types.ChatResponse, error) {
	m.called = true
	return nil, assert.AnError
}

func (m *failIfCalledDocumentEditChat) ChatStream(context.Context, []chat.Message, *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	m.called = true
	return nil, assert.AnError
}

func (m *failIfCalledDocumentEditChat) GetModelName() string {
	return "fail-if-called-document-edit-chat"
}

func (m *failIfCalledDocumentEditChat) GetModelID() string {
	return "fail-if-called-document-edit-chat"
}

func TestFullDocumentProgressReporter_PublishOutlineEmitsStablePlanningThought(t *testing.T) {
	bus := event.NewEventBus()
	req := &types.QARequest{Session: &types.Session{ID: "sess-outline-progress"}}
	reporter := newFullDocumentProgressReporter(context.Background(), req, bus, generateEventID("document-outline-progress"))

	var thoughts []event.AgentThoughtData
	bus.On(event.EventAgentThought, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentThoughtData)
		require.True(t, ok)
		thoughts = append(thoughts, data)
		return nil
	})

	reporter.PublishOutline(dedicatedFullDocumentOutline{
		Title: "北海电厂二期智慧电厂项目技术方案",
		Sections: []dedicatedFullDocumentSection{
			{Title: "项目背景与建设目标", Subsections: []dedicatedFullDocumentSubsection{{Title: "项目背景与行业机遇"}, {Title: "总体建设目标"}}},
			{Title: "总体技术架构"},
			{Title: "实施与运维保障"},
			{Title: "总体技术架构"},
		},
	})

	require.Len(t, thoughts, 1)
	assert.Equal(t, "# 北海电厂二期智慧电厂项目技术方案\n## 第1章 项目背景与建设目标\n### 1.1 项目背景与行业机遇\n### 1.2 总体建设目标\n## 第2章 总体技术架构\n## 第3章 实施与运维保障", thoughts[0].Content)
	assert.Equal(t, "planning", thoughts[0].Stage)
	assert.True(t, thoughts[0].Synthetic)
	assert.True(t, thoughts[0].Done)
	assert.False(t, thoughts[0].Replace)
	assert.Equal(t, "北海电厂二期智慧电厂项目技术方案", thoughts[0].Outline["title"])
	outlineSections, ok := thoughts[0].Outline["sections"].([]map[string]interface{})
	if !ok {
		genericSections, genericOK := thoughts[0].Outline["sections"].([]interface{})
		require.True(t, genericOK)
		outlineSections = make([]map[string]interface{}, 0, len(genericSections))
		for _, item := range genericSections {
			section, sectionOK := item.(map[string]interface{})
			require.True(t, sectionOK)
			outlineSections = append(outlineSections, section)
		}
	}
	require.Len(t, outlineSections, 3)
	assert.Equal(t, 1, outlineSections[0]["number"])
	assert.Equal(t, "项目背景与建设目标", outlineSections[0]["title"])
	assert.Equal(t, "第1章 项目背景与建设目标", outlineSections[0]["heading"])
}

func TestFullDocumentProgressReporter_UpdateStageEmitsStructuredSectionProgress(t *testing.T) {
	bus := event.NewEventBus()
	req := &types.QARequest{Session: &types.Session{ID: "sess-outline-progress-structured"}}
	reporter := newFullDocumentProgressReporter(context.Background(), req, bus, generateEventID("document-outline-progress"))

	var thoughts []event.AgentThoughtData
	bus.On(event.EventAgentThought, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentThoughtData)
		require.True(t, ok)
		thoughts = append(thoughts, data)
		return nil
	})

	reporter.SetSectionProgress(7, 8, "AR眼镜智能作业系统")
	reporter.SetQueryProgress(2, 3)
	reporter.UpdateStage("retrieving", "正在检索第 7/8 章“AR眼镜智能作业系统”的本地证据（2/3）：AR 智能作业")

	require.Len(t, thoughts, 1)
	assert.Equal(t, "retrieving", thoughts[0].Stage)
	assert.Equal(t, 7, thoughts[0].SectionCurrent)
	assert.Equal(t, 8, thoughts[0].SectionTotal)
	assert.Equal(t, "AR眼镜智能作业系统", thoughts[0].SectionTitle)
	assert.Equal(t, 2, thoughts[0].QueryCurrent)
	assert.Equal(t, 3, thoughts[0].QueryTotal)
	assert.Equal(t, "第 7/8 章：AR眼镜智能作业系统 · 检索 2/3", thoughts[0].ProgressLabel)
	assert.Equal(t, "正在检索第 7/8 章“AR眼镜智能作业系统”的本地证据（2/3）：AR 智能作业", thoughts[0].Content)
}

type stagedFullDocumentChat struct {
	negotiationResponse *types.ChatResponse
	outlineResponse     types.ChatResponse
	repairResponse      *types.ChatResponse
	repairErr           error
	outlineStream       []types.StreamResponse
	sectionStreams      [][]types.StreamResponse
	chatCalls           int
	streamCalls         int
	chatMessages        [][]chat.Message
	streamMessages      [][]chat.Message
	chatOptions         []chat.ChatOptions
	streamOptions       []chat.ChatOptions
	streamHasDeadline   []bool
	streamTimeouts      []time.Duration
}

func (m *stagedFullDocumentChat) Chat(_ context.Context, messages []chat.Message, options *chat.ChatOptions) (*types.ChatResponse, error) {
	m.chatCalls++
	m.chatMessages = append(m.chatMessages, append([]chat.Message(nil), messages...))
	if options != nil {
		m.chatOptions = append(m.chatOptions, *options)
	} else {
		m.chatOptions = append(m.chatOptions, chat.ChatOptions{})
	}
	response := m.outlineResponse
	if m.chatCalls == 1 && m.negotiationResponse != nil {
		response = *m.negotiationResponse
	} else if m.repairErr != nil {
		return nil, m.repairErr
	} else if m.repairResponse != nil {
		response = *m.repairResponse
	}
	return &response, nil
}

func (m *stagedFullDocumentChat) ChatStream(ctx context.Context, messages []chat.Message, options *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	m.streamCalls++
	m.streamMessages = append(m.streamMessages, append([]chat.Message(nil), messages...))
	if options != nil {
		m.streamOptions = append(m.streamOptions, *options)
	} else {
		m.streamOptions = append(m.streamOptions, chat.ChatOptions{})
	}
	if deadline, ok := ctx.Deadline(); ok {
		m.streamHasDeadline = append(m.streamHasDeadline, true)
		m.streamTimeouts = append(m.streamTimeouts, time.Until(deadline))
	} else {
		m.streamHasDeadline = append(m.streamHasDeadline, false)
		m.streamTimeouts = append(m.streamTimeouts, 0)
	}
	hasOutlineStream := len(m.outlineStream) > 0 || strings.TrimSpace(m.outlineResponse.Content) != ""
	responses := []types.StreamResponse{}
	if hasOutlineStream && m.streamCalls == 1 {
		responses = m.outlineStream
		if len(responses) == 0 {
			responses = []types.StreamResponse{{
				ResponseType: types.ResponseTypeAnswer,
				Content:      m.outlineResponse.Content,
				Done:         true,
				FinishReason: firstNonEmptyString(strings.TrimSpace(m.outlineResponse.FinishReason), "stop"),
			}}
		}
	} else {
		idx := m.streamCalls - 1
		if hasOutlineStream {
			idx = m.streamCalls - 2
		}
		if idx >= 0 && idx < len(m.sectionStreams) {
			responses = m.sectionStreams[idx]
		}
	}
	ch := make(chan types.StreamResponse, len(responses))
	for _, response := range responses {
		ch <- response
	}
	close(ch)
	return ch, nil
}

func (m *stagedFullDocumentChat) GetModelName() string { return "staged-full-document-chat" }

func (m *stagedFullDocumentChat) GetModelID() string { return "staged-full-document-chat" }

type fullDocumentModelServiceStub struct {
	model *types.Model
}

func (s *fullDocumentModelServiceStub) CreateModel(context.Context, *types.Model) error { return nil }

func (s *fullDocumentModelServiceStub) GetModelByID(context.Context, string) (*types.Model, error) {
	return s.model, nil
}

func (s *fullDocumentModelServiceStub) ListModels(context.Context) ([]*types.Model, error) {
	if s.model == nil {
		return nil, nil
	}
	return []*types.Model{s.model}, nil
}

func (s *fullDocumentModelServiceStub) UpdateModel(context.Context, *types.Model) error { return nil }

func (s *fullDocumentModelServiceStub) DeleteModel(context.Context, string) error { return nil }

func (s *fullDocumentModelServiceStub) GetEmbeddingModel(context.Context, string) (embedding.Embedder, error) {
	return nil, nil
}

func (s *fullDocumentModelServiceStub) GetEmbeddingModelForTenant(context.Context, string, uint64) (embedding.Embedder, error) {
	return nil, nil
}

func (s *fullDocumentModelServiceStub) GetRerankModel(context.Context, string) (rerank.Reranker, error) {
	return nil, nil
}

func (s *fullDocumentModelServiceStub) GetChatModel(context.Context, string) (chat.Chat, error) {
	return nil, nil
}

func (s *fullDocumentModelServiceStub) GetVLMModel(context.Context, string) (vlm.VLM, error) {
	return nil, nil
}

func (s *fullDocumentModelServiceStub) GetASRModel(context.Context, string) (asr.ASR, error) {
	return nil, nil
}

type fullDocumentKnowledgeSearchStub struct {
	kbs     map[string]*types.KnowledgeBase
	results map[string][]*types.SearchResult
	params  []types.SearchParams
}

type fullDocumentGenerationRunRepoStub struct {
	runs    map[string]*types.ChatDocumentGenerationRun
	created *types.ChatDocumentGenerationRun
	updated *types.ChatDocumentGenerationRun
}

func cloneChatDocumentGenerationRun(run *types.ChatDocumentGenerationRun) *types.ChatDocumentGenerationRun {
	if run == nil {
		return nil
	}
	cloned := *run
	if run.OutlineJSON != nil {
		cloned.OutlineJSON = append(types.JSON(nil), run.OutlineJSON...)
	}
	if run.BudgetJSON != nil {
		cloned.BudgetJSON = append(types.JSON(nil), run.BudgetJSON...)
	}
	if run.RuntimeFeedbackJSON != nil {
		cloned.RuntimeFeedbackJSON = append(types.JSON(nil), run.RuntimeFeedbackJSON...)
	}
	if run.EffectiveKBIDsJSON != nil {
		cloned.EffectiveKBIDsJSON = append(types.JSON(nil), run.EffectiveKBIDsJSON...)
	}
	if run.CompletedSectionsJSON != nil {
		cloned.CompletedSectionsJSON = append(types.JSON(nil), run.CompletedSectionsJSON...)
	}
	return &cloned
}

func (s *fullDocumentGenerationRunRepoStub) CreateRun(_ context.Context, run *types.ChatDocumentGenerationRun) error {
	if s.runs == nil {
		s.runs = map[string]*types.ChatDocumentGenerationRun{}
	}
	s.created = cloneChatDocumentGenerationRun(run)
	s.runs[run.ID] = cloneChatDocumentGenerationRun(run)
	return nil
}

func (s *fullDocumentGenerationRunRepoStub) GetRunByID(_ context.Context, _ uint64, runID string) (*types.ChatDocumentGenerationRun, error) {
	if s.runs == nil {
		return nil, nil
	}
	return cloneChatDocumentGenerationRun(s.runs[runID]), nil
}

func (s *fullDocumentGenerationRunRepoStub) GetLatestRunBySessionAndRoot(
	_ context.Context,
	_ uint64,
	sessionID string,
	rootMessageID string,
	rootArtifactID string,
) (*types.ChatDocumentGenerationRun, error) {
	for _, run := range s.runs {
		if run == nil || run.SessionID != sessionID {
			continue
		}
		if rootArtifactID != "" && run.RootArtifactID == rootArtifactID {
			return cloneChatDocumentGenerationRun(run), nil
		}
		if rootMessageID != "" && run.RootMessageID == rootMessageID {
			return cloneChatDocumentGenerationRun(run), nil
		}
	}
	return nil, nil
}

func (s *fullDocumentGenerationRunRepoStub) UpdateRun(_ context.Context, run *types.ChatDocumentGenerationRun) error {
	if s.runs == nil {
		s.runs = map[string]*types.ChatDocumentGenerationRun{}
	}
	s.updated = cloneChatDocumentGenerationRun(run)
	s.runs[run.ID] = cloneChatDocumentGenerationRun(run)
	return nil
}

func (s *fullDocumentKnowledgeSearchStub) CreateKnowledgeBase(_ context.Context, kb *types.KnowledgeBase) (*types.KnowledgeBase, error) {
	return kb, nil
}

func (s *fullDocumentKnowledgeSearchStub) GetKnowledgeBaseByID(_ context.Context, id string) (*types.KnowledgeBase, error) {
	if kb, ok := s.kbs[id]; ok {
		return kb, nil
	}
	return nil, nil
}

func (s *fullDocumentKnowledgeSearchStub) GetKnowledgeBaseByIDOnly(_ context.Context, id string) (*types.KnowledgeBase, error) {
	return s.GetKnowledgeBaseByID(context.Background(), id)
}

func (s *fullDocumentKnowledgeSearchStub) GetKnowledgeBasesByIDsOnly(_ context.Context, ids []string) ([]*types.KnowledgeBase, error) {
	result := make([]*types.KnowledgeBase, 0, len(ids))
	for _, id := range ids {
		if kb, ok := s.kbs[id]; ok {
			result = append(result, kb)
		}
	}
	return result, nil
}

func (s *fullDocumentKnowledgeSearchStub) FillKnowledgeBaseCounts(context.Context, *types.KnowledgeBase) error {
	return nil
}

func (s *fullDocumentKnowledgeSearchStub) ListKnowledgeBases(_ context.Context) ([]*types.KnowledgeBase, error) {
	result := make([]*types.KnowledgeBase, 0, len(s.kbs))
	for _, kb := range s.kbs {
		result = append(result, kb)
	}
	return result, nil
}

func (s *fullDocumentKnowledgeSearchStub) ListKnowledgeBasesByTenantID(_ context.Context, _ uint64) ([]*types.KnowledgeBase, error) {
	return s.ListKnowledgeBases(context.Background())
}

func TestRunKnowledgeGroundedFullDocumentGenerationPath_PersistsGenerationRun(t *testing.T) {
	oldLimit := dedicatedFullDocumentSectionLimitPerRun
	dedicatedFullDocumentSectionLimitPerRun = 2
	defer func() {
		dedicatedFullDocumentSectionLimitPerRun = oldLimit
	}()

	runRepo := &fullDocumentGenerationRunRepoStub{}
	searchStub := &fullDocumentKnowledgeSearchStub{
		kbs: map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", Type: types.KnowledgeBaseTypeDocument}},
		results: map[string][]*types.SearchResult{
			"kb-1|请输出完整的智慧运行建设方案": {
				{ID: "outline-1", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-outline", KnowledgeTitle: "智慧运行总体方案", Content: "包含建设目标、平台架构与实施保障。", Score: 0.92},
			},
			"kb-1|请输出完整的智慧运行建设方案 建设目标": {
				{ID: "sec-1", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "建设目标", Content: "建设目标章节依据。", Score: 0.91},
			},
			"kb-1|智慧运行建设方案 建设目标": {
				{ID: "sec-1b", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "建设目标", Content: "建设目标补充依据。", Score: 0.88},
			},
			"kb-1|请检索与当前章节直接相关的本地事实和能力说明：建设目标": {
				{ID: "sec-1c", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "建设目标", Content: "建设目标事实。", Score: 0.87},
			},
			"kb-1|请输出完整的智慧运行建设方案 平台架构": {
				{ID: "sec-2", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-2", KnowledgeTitle: "平台架构", Content: "平台架构章节依据。", Score: 0.9},
			},
			"kb-1|智慧运行建设方案 平台架构": {
				{ID: "sec-2b", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-2", KnowledgeTitle: "平台架构", Content: "平台架构补充依据。", Score: 0.86},
			},
			"kb-1|请检索与当前章节直接相关的本地事实和能力说明：平台架构": {
				{ID: "sec-2c", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-2", KnowledgeTitle: "平台架构", Content: "平台架构事实。", Score: 0.85},
			},
			"kb-1|请输出完整的智慧运行建设方案 实施保障": {
				{ID: "sec-3", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-3", KnowledgeTitle: "实施保障", Content: "实施保障章节依据。", Score: 0.84},
			},
			"kb-1|智慧运行建设方案 实施保障": {
				{ID: "sec-3b", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-3", KnowledgeTitle: "实施保障", Content: "实施保障补充依据。", Score: 0.83},
			},
			"kb-1|请检索与当前章节直接相关的本地事实和能力说明：实施保障": {
				{ID: "sec-3c", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-3", KnowledgeTitle: "实施保障", Content: "实施保障事实。", Score: 0.82},
			},
		},
	}
	svc := &sessionService{
		cfg:                  &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}},
		knowledgeBaseService: searchStub,
		generationRunRepo:    runRepo,
	}
	chatModel := &stagedFullDocumentChat{
		outlineResponse: types.ChatResponse{Content: "# 智慧运行建设方案\n\n## 建设目标\n## 平台架构\n## 实施保障\n"},
		sectionStreams: [][]types.StreamResponse{
			{{ResponseType: types.ResponseTypeAnswer, Content: strings.Repeat("说明建设目标与业务价值、接口边界和前置依赖。", 40) + "历史正文尾部标记-不应完整透传", Done: true, FinishReason: "stop"}},
			{{ResponseType: types.ResponseTypeAnswer, Content: "说明平台架构与能力分层。", Done: true, FinishReason: "stop"}},
			{{ResponseType: types.ResponseTypeAnswer, Content: "说明实施保障与交付闭环。", Done: true, FinishReason: "stop"}},
		},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:              "请输出完整的智慧运行建设方案",
		Session:            &types.Session{ID: "sess-grounded-run"},
		AssistantMessageID: "msg-grounded-run",
		DocumentOutputMode: types.ChatDocumentOutputModeFull,
	}
	agentConfig := &types.AgentConfig{
		Temperature:    0.3,
		KnowledgeBases: []string{"kb-1"},
		SearchTargets:  types.SearchTargets{{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase}},
	}

	var completes []event.AgentCompleteData
	var thoughts []event.AgentThoughtData
	bus.On(event.EventAgentThought, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentThoughtData)
		require.True(t, ok)
		thoughts = append(thoughts, data)
		return nil
	})
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runKnowledgeGroundedFullDocumentGenerationPath(context.Background(), req, bus, chatModel, agentConfig)
	require.NoError(t, err)
	require.Len(t, completes, 1)
	require.NotNil(t, runRepo.created)
	require.NotNil(t, runRepo.updated)
	assert.Equal(t, types.ChatDocumentGenerationRunStatusCompleted, runRepo.updated.Status)
	assert.Equal(t, 1, runRepo.updated.AutoContinueRound)
	assert.Equal(t, []string{"建设目标", "平台架构", "实施保障"}, unmarshalGenerationRunStringSlice(runRepo.updated.CompletedSectionsJSON))
	assert.Equal(t, types.MessageCompletionStatusCompleted, completes[0].CompletionStatus)
	require.NotNil(t, completes[0].AutoContinueNext)
	assert.False(t, *completes[0].AutoContinueNext)
	agentSteps := completes[0].AgentSteps
	require.NotEmpty(t, agentSteps)
	assert.Equal(t, len(agentSteps), completes[0].TotalSteps)
	stages := make(map[string]bool)
	stageFirstIndex := make(map[string]int)
	for _, step := range agentSteps {
		if strings.TrimSpace(step.Stage) != "" {
			stage := strings.TrimSpace(step.Stage)
			stages[stage] = true
			if _, exists := stageFirstIndex[stage]; !exists {
				stageFirstIndex[stage] = step.Iteration
			}
		}
	}
	assert.True(t, stages["planning"])
	assert.True(t, stages["retrieving"])
	assert.True(t, stages["generating"])
	assert.True(t, stages["finalizing"])
	assert.Less(t, stageFirstIndex["planning"], stageFirstIndex["retrieving"])
	assert.Less(t, stageFirstIndex["retrieving"], stageFirstIndex["generating"])
	assert.Less(t, stageFirstIndex["generating"], stageFirstIndex["finalizing"])
	require.NotNil(t, completes[0].Extra)
	assert.Equal(t, runRepo.created.ID, completes[0].Extra["generation_run_id"])
	outlinePayload, ok := completes[0].Extra["outline"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "智慧运行建设方案", outlinePayload["title"])
	outlineSections := extractOutlineSectionsFromInterfaces(outlinePayload["sections"])
	assert.Equal(t, []string{"第1章 建设目标", "第2章 平台架构", "第3章 实施保障"}, outlineSections)
	budgetPayload, ok := completes[0].Extra["budget"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "fallback", budgetPayload["source"])
	assert.Equal(t, 1536, budgetPayload["outline_max_completion_tokens"])
	assert.Equal(t, 4096, budgetPayload["section_max_completion_tokens"])
	assert.Equal(t, 8, budgetPayload["section_evidence_top_k"])
	var sawOutlineProgress bool
	var sawOutlineStreamingProgress bool
	var sawSectionProgress bool
	var sawPublishedOutline bool
	for _, thought := range thoughts {
		if strings.Contains(thought.Content, "已检索 1 条本地知识证据，并规划 3 个章节，将按大纲连续生成全部章节") {
			sawOutlineProgress = true
		}
		if strings.Contains(thought.Content, "正在生成大纲：已识别 3 个章节") {
			sawOutlineStreamingProgress = true
		}
		if strings.Contains(thought.Content, "正在检索第 3/3 章“实施保障”的本地证据") {
			sawSectionProgress = true
		}
		if thought.Stage == "planning" && thought.Done && !thought.Replace && thought.Synthetic && strings.Contains(thought.Content, "# 智慧运行建设方案") && strings.Contains(thought.Content, "## 第1章 建设目标") && strings.Contains(thought.Content, "## 第2章 平台架构") && strings.Contains(thought.Content, "## 第3章 实施保障") {
			sawPublishedOutline = true
		}
	}
	assert.True(t, sawOutlineProgress)
	assert.True(t, sawOutlineStreamingProgress)
	assert.True(t, sawSectionProgress)
	assert.True(t, sawPublishedOutline)
	require.Len(t, chatModel.streamOptions, 4)
	for index, options := range chatModel.streamOptions {
		require.NotNil(t, options.Thinking)
		assert.False(t, *options.Thinking)
		if index == 0 {
			assert.Equal(t, 1536, options.MaxCompletionTokens)
			continue
		}
		assert.Zero(t, options.MaxCompletionTokens)
	}
	require.Len(t, chatModel.streamMessages, 4)
	assert.Contains(t, chatModel.streamMessages[0][1].Content, "Planning requirements")
	assert.Contains(t, chatModel.streamMessages[0][1].Content, "项目背景与建设目标")
	assert.Contains(t, chatModel.streamMessages[1][1].Content, "设计文档深度要求")
	assert.Contains(t, chatModel.streamMessages[1][1].Content, "已确认事实、设计推导、待补充项")
	assert.Contains(t, chatModel.streamMessages[2][1].Content, "Completed document summary")
	assert.NotContains(t, chatModel.streamMessages[2][1].Content, "Completed content so far")
	assert.NotContains(t, chatModel.streamMessages[2][1].Content, "历史正文尾部标记-不应完整透传")
	assert.Contains(t, chatModel.streamMessages[2][1].Content, "## 第1章 建设目标")
	require.NotEmpty(t, searchStub.params)
	for _, params := range searchStub.params {
		assert.Equal(t, 8, params.MatchCount)
	}
}

func TestRunKnowledgeGroundedFullDocumentGenerationPath_RuntimeFeedbackAdjustsBudgetAndPersistsRun(t *testing.T) {
	oldLimit := dedicatedFullDocumentSectionLimitPerRun
	dedicatedFullDocumentSectionLimitPerRun = 3
	defer func() {
		dedicatedFullDocumentSectionLimitPerRun = oldLimit
	}()

	runRepo := &fullDocumentGenerationRunRepoStub{}
	searchStub := &fullDocumentKnowledgeSearchStub{
		kbs: map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", Type: types.KnowledgeBaseTypeDocument}},
		results: map[string][]*types.SearchResult{
			"kb-1|请输出完整的智慧运行建设方案": {
				{ID: "outline-1", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-outline", KnowledgeTitle: "智慧运行总体方案", Content: "包含建设目标和平台架构。", Score: 0.92},
			},
			"kb-1|请输出完整的智慧运行建设方案 建设目标": {
				{ID: "sec-1", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "建设目标", Content: "建设目标章节依据。", Score: 0.91},
			},
			"kb-1|智慧运行建设方案 建设目标": {
				{ID: "sec-1b", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "建设目标", Content: "建设目标补充依据。", Score: 0.88},
			},
			"kb-1|请检索与当前章节直接相关的本地事实和能力说明：建设目标": {
				{ID: "sec-1c", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "建设目标", Content: "建设目标事实。", Score: 0.87},
			},
			"kb-1|请输出完整的智慧运行建设方案 平台架构": {
				{ID: "sec-2", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-2", KnowledgeTitle: "平台架构", Content: "平台架构章节依据。", Score: 0.9},
			},
			"kb-1|智慧运行建设方案 平台架构": {
				{ID: "sec-2b", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-2", KnowledgeTitle: "平台架构", Content: "平台架构补充依据。", Score: 0.86},
			},
			"kb-1|请检索与当前章节直接相关的本地事实和能力说明：平台架构": {
				{ID: "sec-2c", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-2", KnowledgeTitle: "平台架构", Content: "平台架构事实。", Score: 0.85},
			},
		},
	}
	svc := &sessionService{
		cfg:                  &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}},
		knowledgeBaseService: searchStub,
		generationRunRepo:    runRepo,
	}
	chatModel := &stagedFullDocumentChat{
		outlineResponse: types.ChatResponse{Content: "# 智慧运行建设方案\n\n## 建设目标\n## 平台架构\n"},
		sectionStreams: [][]types.StreamResponse{
			{{ResponseType: types.ResponseTypeAnswer, Content: "说明建设目标与业务价值，当前输出被长度限制截断。", Done: true, FinishReason: "length"}},
			{{ResponseType: types.ResponseTypeAnswer, Content: "说明平台架构与能力分层。", Done: true, FinishReason: "stop"}},
		},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:              "请输出完整的智慧运行建设方案",
		Session:            &types.Session{ID: "sess-grounded-runtime-feedback"},
		AssistantMessageID: "msg-grounded-runtime-feedback",
		DocumentOutputMode: types.ChatDocumentOutputModeFull,
	}
	agentConfig := &types.AgentConfig{
		Temperature:    0.3,
		KnowledgeBases: []string{"kb-1"},
		SearchTargets:  types.SearchTargets{{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase}},
	}

	var completes []event.AgentCompleteData
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runKnowledgeGroundedFullDocumentGenerationPath(context.Background(), req, bus, chatModel, agentConfig)
	require.NoError(t, err)
	require.Len(t, completes, 1)
	require.NotNil(t, runRepo.updated)
	assert.Equal(t, types.ChatDocumentGenerationRunStatusContinuing, runRepo.updated.Status)
	assert.Equal(t, types.MessageCompletionStatusPartial, completes[0].CompletionStatus)
	assert.Equal(t, "length", completes[0].FinishReason)
	assert.Equal(t, types.ChatDocumentGenerationStatusContinuing, completes[0].DocumentGenerationStatus)
	require.NotNil(t, completes[0].AutoContinueNext)
	assert.True(t, *completes[0].AutoContinueNext)
	resolvedBudget := unmarshalGenerationRunBudget(runRepo.updated.BudgetJSON)
	assert.Equal(t, "runtime_feedback", resolvedBudget.Source)
	assert.Equal(t, 4608, resolvedBudget.SectionMaxCompletionTokens)
	assert.Equal(t, 4608, resolvedBudget.ContinuationMaxCompletionTokens)
	feedback := unmarshalGenerationRunRuntimeFeedback(runRepo.updated.RuntimeFeedbackJSON)
	assert.Equal(t, 1, feedback.SectionCount)
	assert.Equal(t, 1, feedback.LengthStopCount)
	assert.True(t, feedback.BudgetAdjusted)
	assert.Equal(t, 2, feedback.RecommendedSectionLimitPerRun)
	assert.Contains(t, feedback.AdjustmentReasons, "section_tokens_up_length")
	require.Len(t, chatModel.streamOptions, 2)
	assert.Equal(t, 1536, chatModel.streamOptions[0].MaxCompletionTokens)
	assert.Zero(t, chatModel.streamOptions[1].MaxCompletionTokens)
	require.NotNil(t, completes[0].Extra)
	budgetPayload, ok := completes[0].Extra["budget"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "runtime_feedback", budgetPayload["source"])
	assert.Equal(t, 4608, budgetPayload["section_max_completion_tokens"])
	feedbackPayload, ok := completes[0].Extra["runtime_feedback"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 1, feedbackPayload["section_count"])
	assert.Equal(t, 1, feedbackPayload["length_stop_count"])
	assert.Equal(t, 2, feedbackPayload["recommended_section_limit_per_run"])
}

func TestRunKnowledgeGroundedDocumentContinuationPath_ValidatesPlannedSubsectionsOnCompletion(t *testing.T) {
	oldLimit := dedicatedFullDocumentSectionLimitPerRun
	dedicatedFullDocumentSectionLimitPerRun = 2
	defer func() {
		dedicatedFullDocumentSectionLimitPerRun = oldLimit
	}()

	outline := dedicatedFullDocumentOutline{
		Title: "智慧运行建设方案",
		Sections: []dedicatedFullDocumentSection{
			{Number: 1, Title: "建设目标", Heading: "第1章 建设目标"},
			{Number: 2, Title: "平台架构", Heading: "第2章 平台架构", Subsections: []dedicatedFullDocumentSubsection{{Number: "2.1", Title: "总体架构"}, {Number: "2.2", Title: "能力分层"}}},
		},
	}
	run := &types.ChatDocumentGenerationRun{
		ID:                    "run-validate-subsections-1",
		TenantID:              0,
		SessionID:             "sess-grounded-validate-subsections",
		OriginalQuery:         "请输出完整的智慧运行建设方案",
		DocumentTitle:         "智慧运行建设方案",
		OutlineJSON:           marshalGenerationRunJSON(outline),
		EffectiveKBIDsJSON:    marshalGenerationRunJSON([]string{"kb-1"}),
		CompletedSectionsJSON: marshalGenerationRunJSON([]string{"建设目标"}),
		Status:                types.ChatDocumentGenerationRunStatusContinuing,
		AutoContinueRound:     1,
	}
	runRepo := &fullDocumentGenerationRunRepoStub{runs: map[string]*types.ChatDocumentGenerationRun{"run-validate-subsections-1": cloneChatDocumentGenerationRun(run)}}
	svc := &sessionService{
		cfg: &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}},
		knowledgeBaseService: &fullDocumentKnowledgeSearchStub{
			kbs: map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", Type: types.KnowledgeBaseTypeDocument}},
			results: map[string][]*types.SearchResult{
				"kb-1|请输出完整的智慧运行建设方案 平台架构": {
					{ID: "sec-2", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-2", KnowledgeTitle: "平台架构", Content: "平台架构章节依据。", Score: 0.9},
				},
				"kb-1|智慧运行建设方案 平台架构": {
					{ID: "sec-2b", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-2", KnowledgeTitle: "平台架构", Content: "平台架构补充依据。", Score: 0.88},
				},
				"kb-1|请检索与当前章节直接相关的本地事实和能力说明：平台架构": {
					{ID: "sec-2c", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-2", KnowledgeTitle: "平台架构", Content: "平台架构事实。", Score: 0.86},
				},
			},
		},
		generationRunRepo: runRepo,
	}
	chatModel := &stagedFullDocumentChat{
		sectionStreams: [][]types.StreamResponse{{
			{ResponseType: types.ResponseTypeAnswer, Content: "说明平台架构与能力分层。", Done: true, FinishReason: "stop"},
		}},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:                     "以当前文档为基准，继续剩余内容输出",
		Session:                   &types.Session{ID: "sess-grounded-validate-subsections"},
		AssistantMessageID:        "msg-grounded-validate-subsections",
		DocumentIntent:            types.ChatDocumentIntentContinue,
		DocumentOperation:         types.ChatDocumentOperationContinue,
		DocumentOutputMode:        types.ChatDocumentOutputModeDelta,
		BaseArtifactID:            "artifact-validate-subsections-1",
		BaseArtifact:              &types.ChatDocumentArtifact{ID: "artifact-validate-subsections-1", ContentSnapshot: "# 智慧运行建设方案\n\n## 建设目标\n\n说明建设目标与业务价值。"},
		AutoContinue:              true,
		GenerationRunID:           "run-validate-subsections-1",
		AutoContinueRound:         1,
		AutoContinuePrompt:        "以当前文档为基准，继续剩余内容输出",
		AutoContinueOriginalQuery: "请输出完整的智慧运行建设方案",
	}
	agentConfig := &types.AgentConfig{
		Temperature:    0.3,
		KnowledgeBases: []string{"kb-1"},
		SearchTargets:  types.SearchTargets{{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase}},
	}

	var completes []event.AgentCompleteData
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runKnowledgeGroundedDocumentContinuationPath(context.Background(), req, bus, chatModel, agentConfig)
	require.NoError(t, err)
	require.Len(t, completes, 1)
	require.NotNil(t, runRepo.updated)
	assert.Equal(t, types.ChatDocumentGenerationRunStatusNeedsReview, runRepo.updated.Status)
	assert.Equal(t, types.MessageCompletionStatusCompleted, completes[0].CompletionStatus)
	assert.Equal(t, "stop", completes[0].FinishReason)
	assert.Empty(t, completes[0].FailureReason)
	assert.Equal(t, types.ChatDocumentGenerationStatusNeedsReview, completes[0].DocumentGenerationStatus)
	assert.Contains(t, completes[0].FinalAnswer, "## 第2章 平台架构")
}

func (s *fullDocumentKnowledgeSearchStub) UpdateKnowledgeBase(_ context.Context, _ string, name string, description string, _ *types.KnowledgeBaseConfig, visibility string, organizationID string) (*types.KnowledgeBase, error) {
	return &types.KnowledgeBase{Name: name, Description: description, Visibility: visibility, OrganizationID: organizationID}, nil
}

func (s *fullDocumentKnowledgeSearchStub) DeleteKnowledgeBase(_ context.Context, _ string) error {
	return nil
}

func (s *fullDocumentKnowledgeSearchStub) TogglePinKnowledgeBase(_ context.Context, id string) (*types.KnowledgeBase, error) {
	return s.GetKnowledgeBaseByID(context.Background(), id)
}

func (s *fullDocumentKnowledgeSearchStub) HybridSearch(_ context.Context, knowledgeBaseID string, params types.SearchParams) ([]*types.SearchResult, error) {
	s.params = append(s.params, params)
	if s.results == nil {
		return nil, nil
	}
	return s.results[knowledgeBaseID+"|"+params.QueryText], nil
}

func (s *fullDocumentKnowledgeSearchStub) GetQueryEmbedding(_ context.Context, _ string, _ string) ([]float32, error) {
	return nil, nil
}

func (s *fullDocumentKnowledgeSearchStub) ResolveEmbeddingModelKeys(_ context.Context, kbs []*types.KnowledgeBase) map[string]string {
	result := make(map[string]string, len(kbs))
	for _, kb := range kbs {
		if kb != nil {
			result[kb.ID] = kb.ID
		}
	}
	return result
}

func (s *fullDocumentKnowledgeSearchStub) CopyKnowledgeBase(_ context.Context, src string, dst string) (*types.KnowledgeBase, *types.KnowledgeBase, error) {
	return s.kbs[src], s.kbs[dst], nil
}

func (s *fullDocumentKnowledgeSearchStub) GetRepository() interfaces.KnowledgeBaseRepository {
	return nil
}

func (s *fullDocumentKnowledgeSearchStub) ProcessKBDelete(_ context.Context, _ *asynq.Task) error {
	return nil
}

func TestShouldApplyDocumentStopgap(t *testing.T) {
	t.Run("revise delta with base artifact enables stopgap", func(t *testing.T) {
		req := &types.QARequest{
			DocumentIntent:     types.ChatDocumentIntentRevise,
			DocumentOutputMode: types.ChatDocumentOutputModeDelta,
			BaseArtifactID:     "artifact-1",
		}

		assert.True(t, shouldApplyDocumentStopgap(req))
	})

	t.Run("full document regeneration style request does not enable stopgap", func(t *testing.T) {
		req := &types.QARequest{
			DocumentIntent:     types.ChatDocumentIntentRevise,
			DocumentOutputMode: types.ChatDocumentOutputModeFull,
			BaseArtifactID:     "artifact-1",
		}

		assert.False(t, shouldApplyDocumentStopgap(req))
	})

	t.Run("request without base artifact does not enable stopgap", func(t *testing.T) {
		req := &types.QARequest{
			DocumentIntent:     types.ChatDocumentIntentContinue,
			DocumentOutputMode: types.ChatDocumentOutputModeDelta,
		}

		assert.False(t, shouldApplyDocumentStopgap(req))
	})
}

func TestApplyDocumentStopgapAgentConfig(t *testing.T) {
	req := &types.QARequest{
		DocumentIntent:     types.ChatDocumentIntentRevise,
		DocumentOutputMode: types.ChatDocumentOutputModeDelta,
		BaseArtifactID:     "artifact-1",
	}
	config := &types.AgentConfig{
		MaxIterations:          8,
		WebSearchEnabled:       true,
		MultiTurnEnabled:       true,
		RetainRetrievalHistory: true,
		AllowedTools:           []string{agenttools.ToolKnowledgeSearch},
	}

	applyDocumentStopgapAgentConfig(config, req)

	require.NotNil(t, config.Thinking)
	assert.Equal(t, 1, config.MaxIterations)
	assert.False(t, config.WebSearchEnabled)
	assert.False(t, config.MultiTurnEnabled)
	assert.False(t, *config.Thinking)
	assert.False(t, config.RetainRetrievalHistory)
	assert.Equal(t, "none", config.MCPSelectionMode)
	assert.Nil(t, config.MCPServices)
	assert.Equal(t, []string{agenttools.ToolFinalAnswer}, config.AllowedTools)
}

func TestShouldInlineQuotedContext(t *testing.T) {
	t.Run("document stopgap request skips quoted context inline", func(t *testing.T) {
		req := &types.QARequest{
			QuotedContext:      "<document_revision_context>...</document_revision_context>",
			DocumentIntent:     types.ChatDocumentIntentRevise,
			DocumentOutputMode: types.ChatDocumentOutputModeDelta,
			BaseArtifactID:     "artifact-1",
		}

		assert.False(t, shouldInlineQuotedContext(req))
	})

	t.Run("normal request keeps quoted context inline", func(t *testing.T) {
		req := &types.QARequest{QuotedContext: "quoted"}

		assert.True(t, shouldInlineQuotedContext(req))
	})
}

func TestAgentDocumentContextFromQARequestCarriesUserGoal(t *testing.T) {
	req := &types.QARequest{
		Query:              "把 2.5.5 后续内容合并到第二章",
		DocumentIntent:     types.ChatDocumentIntentRevise,
		DocumentOperation:  types.ChatDocumentOperationRevise,
		DocumentOutputMode: types.ChatDocumentOutputModeDelta,
		BaseArtifactID:     "artifact-1",
		QuotedContext:      "<document_revision_context>...</document_revision_context>",
	}

	documentContext := agentDocumentContextFromQARequest(req)
	require.NotNil(t, documentContext)
	assert.Equal(t, req.Query, documentContext.UserGoal)
	assert.Equal(t, req.QuotedContext, documentContext.QuotedContext)
}

func TestShouldUseDedicatedDocumentEditPath(t *testing.T) {
	t.Run("pure document revise request uses dedicated path", func(t *testing.T) {
		req := &types.QARequest{
			DocumentIntent:     types.ChatDocumentIntentRevise,
			DocumentOutputMode: types.ChatDocumentOutputModeDelta,
			BaseArtifactID:     "artifact-1",
			QuotedContext:      "<document_revision_context>...</document_revision_context>",
		}

		assert.True(t, shouldUseDedicatedDocumentEditPath(req))
	})

	t.Run("request with retrieval dependencies stays on generic agent path", func(t *testing.T) {
		req := &types.QARequest{
			DocumentIntent:     types.ChatDocumentIntentRevise,
			DocumentOutputMode: types.ChatDocumentOutputModeDelta,
			BaseArtifactID:     "artifact-1",
			QuotedContext:      "<document_revision_context>...</document_revision_context>",
			KnowledgeBaseIDs:   []string{"kb-1"},
		}

		assert.False(t, shouldUseDedicatedDocumentEditPath(req))
	})

	t.Run("web search toggle alone does not block pure document edit path", func(t *testing.T) {
		req := &types.QARequest{
			Query:              "把 2.5.5 后续内容合并到第二章",
			DocumentIntent:     types.ChatDocumentIntentRevise,
			DocumentOutputMode: types.ChatDocumentOutputModeDelta,
			BaseArtifactID:     "artifact-1",
			QuotedContext:      "<document_revision_context>...</document_revision_context>",
			WebSearchEnabled:   true,
		}

		assert.True(t, shouldUseDedicatedDocumentEditPath(req))
	})

	t.Run("explicit external retrieval intent stays on generic agent path", func(t *testing.T) {
		req := &types.QARequest{
			Query:              "结合知识库资料，把 2.5.5 后续内容合并到第二章",
			DocumentIntent:     types.ChatDocumentIntentRevise,
			DocumentOutputMode: types.ChatDocumentOutputModeDelta,
			BaseArtifactID:     "artifact-1",
			QuotedContext:      "<document_revision_context>...</document_revision_context>",
			WebSearchEnabled:   true,
		}

		assert.False(t, shouldUseDedicatedDocumentEditPath(req))
	})
}

func TestShouldUseDedicatedFullDocumentGenerationPath(t *testing.T) {
	t.Run("pure full document request uses dedicated path", func(t *testing.T) {
		req := &types.QARequest{
			Query:              "请输出完整的投标技术方案",
			DocumentOutputMode: types.ChatDocumentOutputModeFull,
			WebSearchEnabled:   true,
		}

		assert.True(t, shouldUseDedicatedFullDocumentGenerationPath(req, nil))
	})

	t.Run("request with retrieval dependencies stays on generic path", func(t *testing.T) {
		req := &types.QARequest{
			Query:              "结合知识库输出完整的投标技术方案",
			DocumentOutputMode: types.ChatDocumentOutputModeFull,
			KnowledgeBaseIDs:   []string{"kb-1"},
		}

		assert.False(t, shouldUseDedicatedFullDocumentGenerationPath(req, nil))
	})

	t.Run("agent effective knowledge scope stays on generic path", func(t *testing.T) {
		req := &types.QARequest{
			Query:              "请输出完整的投标技术方案",
			DocumentOutputMode: types.ChatDocumentOutputModeFull,
		}
		agentConfig := &types.AgentConfig{
			KnowledgeBases: []string{"kb-1"},
		}

		assert.False(t, shouldUseDedicatedFullDocumentGenerationPath(req, agentConfig))
	})

	t.Run("factual query stays off dedicated full document path", func(t *testing.T) {
		req := &types.QARequest{
			Query:              "河南省郑州市中原区桐柏路206号5楼32号的火灾警情登记表包含哪些信息？",
			DocumentOutputMode: types.ChatDocumentOutputModeFull,
		}

		assert.False(t, shouldUseDedicatedFullDocumentGenerationPath(req, nil))
	})

	t.Run("regenerate intent keeps dedicated full document path enabled", func(t *testing.T) {
		req := &types.QARequest{
			Query:              "重新生成上一版完整技术方案",
			DocumentIntent:     types.ChatDocumentIntentRegenerate,
			DocumentOperation:  types.ChatDocumentOperationRegenerate,
			DocumentOutputMode: types.ChatDocumentOutputModeFull,
		}

		assert.True(t, shouldUseDedicatedFullDocumentGenerationPath(req, nil))
	})

	t.Run("route decision can authorize dedicated full document path", func(t *testing.T) {
		req := &types.QARequest{
			Query:              "请整理平台建设思路并形成完整输出",
			DocumentOutputMode: types.ChatDocumentOutputModeFull,
			RouteDecision:      &types.ChatRouteDecision{Kind: types.ChatRouteFullDocument},
		}

		assert.True(t, shouldUseDedicatedFullDocumentGenerationPath(req, nil))
	})

	t.Run("translation task stays off dedicated full document path", func(t *testing.T) {
		req := &types.QARequest{
			Query:              "请把这篇文档完整翻译成中文 Markdown",
			DocumentOutputMode: types.ChatDocumentOutputModeFull,
			DocumentTaskKind:   types.ChatDocumentTaskKindTranslation,
		}

		assert.False(t, shouldUseDedicatedFullDocumentGenerationPath(req, nil))
	})
}

func TestShouldUseKnowledgeGroundedFullDocumentGenerationPath(t *testing.T) {
	t.Run("effective local knowledge scope selects grounded path", func(t *testing.T) {
		req := &types.QARequest{
			Query:              "请输出完整的投标技术方案",
			DocumentOutputMode: types.ChatDocumentOutputModeFull,
		}
		agentConfig := &types.AgentConfig{
			KnowledgeBases: []string{"kb-1"},
			SearchTargets: types.SearchTargets{
				{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase},
			},
		}

		assert.True(t, shouldUseKnowledgeGroundedFullDocumentGenerationPath(req, agentConfig))
	})

	t.Run("request with attachments stays off grounded path", func(t *testing.T) {
		req := &types.QARequest{
			Query:              "请输出完整的投标技术方案",
			DocumentOutputMode: types.ChatDocumentOutputModeFull,
			Attachments:        types.MessageAttachments{{FileName: "资料.pdf", FileType: ".pdf"}},
		}
		agentConfig := &types.AgentConfig{KnowledgeBases: []string{"kb-1"}}

		assert.False(t, shouldUseKnowledgeGroundedFullDocumentGenerationPath(req, agentConfig))
	})

	t.Run("translation task stays off grounded full document path", func(t *testing.T) {
		req := &types.QARequest{
			Query:              "请把这篇文档完整翻译成中文 Markdown",
			DocumentOutputMode: types.ChatDocumentOutputModeFull,
			DocumentTaskKind:   types.ChatDocumentTaskKindTranslation,
			KnowledgeIDs:       []string{"knowledge-1"},
		}
		agentConfig := &types.AgentConfig{KnowledgeIDs: []string{"knowledge-1"}}

		assert.False(t, shouldUseKnowledgeGroundedFullDocumentGenerationPath(req, agentConfig))
	})

	t.Run("factual query stays off grounded full document path", func(t *testing.T) {
		req := &types.QARequest{
			Query:              "河南省郑州市中原区桐柏路206号5楼32号的火灾警情登记表包含哪些信息？",
			DocumentOutputMode: types.ChatDocumentOutputModeFull,
		}
		agentConfig := &types.AgentConfig{
			KnowledgeBases: []string{"kb-1"},
			SearchTargets: types.SearchTargets{
				{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase},
			},
		}

		assert.False(t, shouldUseKnowledgeGroundedFullDocumentGenerationPath(req, agentConfig))
	})

	t.Run("regenerate intent keeps grounded full document path enabled", func(t *testing.T) {
		req := &types.QARequest{
			Query:              "重新生成上一版完整技术方案",
			DocumentIntent:     types.ChatDocumentIntentRegenerate,
			DocumentOperation:  types.ChatDocumentOperationRegenerate,
			DocumentOutputMode: types.ChatDocumentOutputModeFull,
		}
		agentConfig := &types.AgentConfig{
			KnowledgeBases: []string{"kb-1"},
			SearchTargets: types.SearchTargets{
				{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase},
			},
		}

		assert.True(t, shouldUseKnowledgeGroundedFullDocumentGenerationPath(req, agentConfig))
	})

	t.Run("route decision can authorize grounded full document path", func(t *testing.T) {
		req := &types.QARequest{
			Query:              "请结合知识库展开说明平台建设路径",
			DocumentOutputMode: types.ChatDocumentOutputModeFull,
			RouteDecision:      &types.ChatRouteDecision{Kind: types.ChatRouteKnowledgeGroundedFullDoc},
		}
		agentConfig := &types.AgentConfig{
			KnowledgeBases: []string{"kb-1"},
			SearchTargets: types.SearchTargets{
				{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase},
			},
		}

		assert.True(t, shouldUseKnowledgeGroundedFullDocumentGenerationPath(req, agentConfig))
	})
}

func TestShouldUseKnowledgeGroundedDocumentContinuationPath(t *testing.T) {
	req := &types.QARequest{
		Query:                     "以当前文档为基准，继续剩余内容输出",
		DocumentIntent:            types.ChatDocumentIntentContinue,
		DocumentOperation:         types.ChatDocumentOperationContinue,
		DocumentOutputMode:        types.ChatDocumentOutputModeDelta,
		BaseArtifactID:            "artifact-1",
		BaseArtifact:              &types.ChatDocumentArtifact{ID: "artifact-1", ContentSnapshot: "# 文档\n\n## 第一章\n\n已有内容"},
		AutoContinue:              true,
		AutoContinuePrompt:        "以当前文档为基准，继续剩余内容输出",
		AutoContinueOriginalQuery: "请输出完整的投标技术方案",
	}
	agentConfig := &types.AgentConfig{
		KnowledgeBases: []string{"kb-1"},
		SearchTargets:  types.SearchTargets{{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase}},
	}

	assert.True(t, shouldUseKnowledgeGroundedDocumentContinuationPath(req, agentConfig))
}

func TestRetrieveKnowledgeGroundedFullDocumentEvidence_FiltersAndDeduplicates(t *testing.T) {
	stub := &fullDocumentKnowledgeSearchStub{
		kbs: map[string]*types.KnowledgeBase{
			"kb-1": {ID: "kb-1", Type: types.KnowledgeBaseTypeDocument},
			"kb-2": {ID: "kb-2", Type: types.KnowledgeBaseTypeWiki},
		},
		results: map[string][]*types.SearchResult{
			"kb-1|请输出完整的投标技术方案": {
				{ID: "chunk-1", KnowledgeID: "doc-1", KnowledgeTitle: "智慧运行总体方案", Content: "覆盖业务架构与实施方案。", Score: 0.82},
				{ID: "chunk-1", KnowledgeID: "doc-1", KnowledgeTitle: "智慧运行总体方案", Content: "重复但得分更低。", Score: 0.61},
				{ID: "chunk-2", KnowledgeID: "doc-2", KnowledgeTitle: "实施保障", Content: "覆盖实施组织与保障机制。", Score: 0.77},
			},
		},
	}

	pack, err := retrieveKnowledgeGroundedFullDocumentEvidence(context.Background(), stub, &config.Config{Conversation: &config.ConversationConfig{EmbeddingTopK: 5}}, types.SearchTargets{
		{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase},
		{KnowledgeBaseID: "kb-2", Type: types.SearchTargetTypeKnowledgeBase},
	}, []string{"请输出完整的投标技术方案"}, 4)
	require.NoError(t, err)
	assert.Empty(t, pack.MissingReason)
	require.Len(t, pack.Items, 2)
	assert.Equal(t, "chunk-1", pack.Items[0].Result.ID)
	assert.Equal(t, "kb-1", pack.Items[0].Result.KnowledgeBaseID)
	assert.Equal(t, "chunk-2", pack.Items[1].Result.ID)
	assert.ElementsMatch(t, []string{"kb-1", "kb-2"}, pack.ScopeKBIDs)
}

func TestBuildKnowledgeGroundedFullDocumentOutlineMessages_IncludesLocalKnowledgeContext(t *testing.T) {
	req := &types.QARequest{Query: "请输出完整的智慧运行技术方案"}
	messages := buildKnowledgeGroundedFullDocumentOutlineMessages(req, "Chinese (Simplified)", knowledgeGroundedEvidencePack{
		Queries: []string{"请输出完整的智慧运行技术方案"},
		Items: []knowledgeGroundedEvidenceItem{{
			Query: "请输出完整的智慧运行技术方案",
			Result: &types.SearchResult{
				ID:              "chunk-1",
				KnowledgeBaseID: "kb-1",
				KnowledgeID:     "doc-1",
				KnowledgeTitle:  "智慧运行总体方案",
				Content:         "智慧运行平台覆盖数据湖、算力平台与实施保障。",
				Score:           0.91,
			},
		}},
	})
	require.Len(t, messages, 2)
	assert.Contains(t, messages[0].Content, "grounded in local knowledge")
	assert.Contains(t, messages[0].Content, "Return JSON only")
	assert.Contains(t, messages[0].Content, "Do not invent")
	assert.Contains(t, messages[1].Content, "<local_knowledge_context>")
	assert.Contains(t, messages[1].Content, "智慧运行总体方案")
	assert.Contains(t, messages[1].Content, "数据湖、算力平台与实施保障")
	assert.Contains(t, messages[1].Content, "If the user explicitly provides chapters")
	assert.Contains(t, messages[1].Content, "项目背景与建设目标")
	assert.Contains(t, messages[1].Content, "sections should usually contain 6 to 10 chapter objects")
	assert.Contains(t, messages[1].Content, "heading must equal \"第{number}章 {title}\"")
	assert.Contains(t, messages[1].Content, "JSON schema")
}

func TestBuildDedicatedFullDocumentOutlineMessages_DefinesStructuredJSONContract(t *testing.T) {
	req := &types.QARequest{Query: "请输出完整的智慧运行技术方案"}
	messages := buildDedicatedFullDocumentOutlineMessages(req, "Chinese (Simplified)")
	require.Len(t, messages, 2)
	assert.Contains(t, messages[0].Content, "Return JSON only")
	assert.Contains(t, messages[0].Content, "stable chapter number")
	assert.Contains(t, messages[1].Content, "If the user explicitly provides chapters")
	assert.Contains(t, messages[1].Content, "项目背景与建设目标")
	assert.Contains(t, messages[1].Content, "sections should usually contain 6 to 10 chapter objects")
	assert.Contains(t, messages[1].Content, "heading must equal \"第{number}章 {title}\"")
	assert.Contains(t, messages[1].Content, "subsections")
	assert.Contains(t, messages[1].Content, "JSON schema")
}

func TestBuildDedicatedFullDocumentSectionMessages_StrengthensMarkdownLayoutConstraints(t *testing.T) {
	req := &types.QARequest{Query: "请输出完整的智慧电厂投标技术方案"}
	messages := buildDedicatedFullDocumentSectionMessages(req, "Chinese (Simplified)", "北海电厂二期智慧电厂项目技术方案", dedicatedFullDocumentOutline{
		Title: "北海电厂二期智慧电厂项目技术方案",
		Sections: []dedicatedFullDocumentSection{
			{Number: 1, Title: "项目背景与建设目标", Heading: "第1章 项目背景与建设目标"},
			{Number: 2, Title: "数据湖与基础算力平台", Heading: "第2章 数据湖与基础算力平台", Subsections: []dedicatedFullDocumentSubsection{{Number: "2.1", Title: "全域数据湖建设"}, {Number: "2.2", Title: "标准化数据治理"}}},
		},
	}, dedicatedFullDocumentSection{Number: 2, Title: "数据湖与基础算力平台", Heading: "第2章 数据湖与基础算力平台", Subsections: []dedicatedFullDocumentSubsection{{Number: "2.1", Title: "全域数据湖建设"}, {Number: "2.2", Title: "标准化数据治理"}}}, "## 第1章 项目背景与建设目标\n\n已有内容")
	require.Len(t, messages, 2)
	assert.Contains(t, messages[0].Content, "customer-facing technical bid")
	assert.Contains(t, messages[0].Content, "completed document summary")
	assert.Contains(t, messages[0].Content, "### 3.1 全域数据湖建设")
	assert.Contains(t, messages[0].Content, "###3.1全域数据湖建设")
	assert.Contains(t, messages[1].Content, "Completed document summary")
	assert.NotContains(t, messages[1].Content, "Completed content so far")
	assert.Contains(t, messages[1].Content, "当前章节编号为：2")
	assert.Contains(t, messages[1].Content, "## 第2章 数据湖与基础算力平台")
	assert.Contains(t, messages[1].Content, "### 2.1 全域数据湖建设")
	assert.Contains(t, messages[1].Content, "### 2.2 标准化数据治理")
	assert.Contains(t, messages[1].Content, "禁止输出其他章节编号，例如：### 3.1 全域数据湖建设")
	assert.Contains(t, messages[1].Content, "不允许新增未规划的同级 H3 标题")
	assert.Contains(t, messages[1].Content, "## 排版与行文要求")
	assert.Contains(t, messages[1].Content, "错误示例：###3.1全域数据湖建设")
	assert.Contains(t, messages[1].Content, "不得输出 Current section、Completed document summary、local_knowledge_context")
	assert.Contains(t, messages[1].Content, "如果证据不足，请在用户可见正文中明确写出“本地知识不足”")
	assert.Contains(t, messages[1].Content, "不要重复输出 H1/H2 标题")
}

func TestNormalizeGeneratedMarkdown_NormalizesHeadingSpacingAndLists(t *testing.T) {
	normalized, qualityIssues := normalizeGeneratedMarkdown("```markdown\n###3.1总体目标\n -事项一\n**结论：**需要尽快推进\n```")

	assert.Equal(t, "### 3.1 总体目标\n\n  - 事项一\n**结论：** 需要尽快推进", normalized)
	assert.Contains(t, qualityIssues, types.ChatDocumentQualityIssueMarkdownHeadingNormalized)
}

func TestValidateGeneratedSectionMarkdownRejectsWrongChapterPrefix(t *testing.T) {
	issues := validateGeneratedSectionMarkdown("### 2.1 错误章节\n\n说明内容。", dedicatedFullDocumentSection{
		Number:  1,
		Title:   "建设目标",
		Heading: "第1章 建设目标",
		Subsections: []dedicatedFullDocumentSubsection{
			{Number: "1.1", Title: "目标分解"},
		},
	})

	require.NotEmpty(t, issues)
	assert.Contains(t, markdownQualityIssueCodes(issues), types.ChatDocumentQualityIssueMarkdownStructureInvalid)
}

func TestValidateGeneratedSectionMarkdownRejectsTooShortContent(t *testing.T) {
	issues := validateGeneratedSectionMarkdown("### 1.1 总体目标\n\n短。", dedicatedFullDocumentSection{
		Number:  1,
		Title:   "建设目标",
		Heading: "第1章 建设目标",
		Subsections: []dedicatedFullDocumentSubsection{
			{Number: "1.1", Title: "总体目标"},
		},
	})

	require.NotEmpty(t, issues)
	assert.Contains(t, markdownQualityIssueCodes(issues), types.ChatDocumentQualityIssueMarkdownTooShort)
}

func TestValidateGeneratedSectionMarkdownAllowsH4Details(t *testing.T) {
	issues := validateGeneratedSectionMarkdown("### 1.1 目标分解\n\n#### 1.1.1 建设原则\n\n围绕现场业务闭环、数据治理和智能应用持续推进。", dedicatedFullDocumentSection{
		Number:  1,
		Title:   "建设目标",
		Heading: "第1章 建设目标",
		Subsections: []dedicatedFullDocumentSubsection{
			{Number: "1.1", Title: "目标分解"},
		},
	})

	assert.Empty(t, issues)
}

func TestApplyGeneratedSectionMarkdownQualityGate_DoesNotRepairValidNormalizedContent(t *testing.T) {
	chatModel := &failIfCalledDocumentEditChat{}
	section := dedicatedFullDocumentSection{
		Number:  1,
		Title:   "建设目标",
		Heading: "第1章 建设目标",
		Subsections: []dedicatedFullDocumentSubsection{
			{Number: "1.1", Title: "总体目标"},
		},
	}
	normalized, qualityIssues, ok := applyGeneratedSectionMarkdownQualityGate(context.Background(), chatModel, &types.AgentConfig{}, fallbackDocumentGenerationBudget(&config.Config{}), section, "###1.1总体目标\n -事项一\n**结论：**需要尽快推进")

	assert.True(t, ok)
	assert.False(t, chatModel.called)
	assert.Contains(t, normalized, "### 1.1 总体目标")
	assert.Contains(t, normalized, "  - 事项一")
	assert.Contains(t, normalized, "**结论：** 需要尽快推进")
	assert.Contains(t, qualityIssues, types.ChatDocumentQualityIssueMarkdownHeadingNormalized)
}

func TestApplyFullDocumentArtifactQualityGate_DowngradesCompletedToNeedsReviewOnDocumentIssues(t *testing.T) {
	content, completionStatus, finishReason, failureReason, documentGenerationStatus, qualityIssues := applyFullDocumentArtifactQualityGate(
		context.Background(),
		nil,
		&types.AgentConfig{},
		fallbackDocumentGenerationBudget(&config.Config{}),
		"# 智慧运行建设方案\n\n##### 过深标题\n\n正文内容。",
		types.MessageCompletionStatusCompleted,
		"stop",
		"",
		types.ChatDocumentGenerationStatusCompleted,
		nil,
	)

	assert.Equal(t, "# 智慧运行建设方案\n\n##### 过深标题\n\n正文内容。", content)
	assert.Equal(t, types.MessageCompletionStatusCompleted, completionStatus)
	assert.Equal(t, "stop", finishReason)
	assert.Equal(t, "", failureReason)
	assert.Equal(t, types.ChatDocumentGenerationStatusNeedsReview, documentGenerationStatus)
	assert.Contains(t, qualityIssues, types.ChatDocumentQualityIssueMarkdownStructureInvalid)
}

func TestBuildKnowledgeGroundedFullDocumentSectionMessages_IncludeLayoutConstraintsAndEvidence(t *testing.T) {
	req := &types.QARequest{Query: "请输出完整的智慧电厂投标技术方案"}
	messages := buildKnowledgeGroundedFullDocumentSectionMessages(req, "Chinese (Simplified)", "北海电厂二期智慧电厂项目技术方案", dedicatedFullDocumentOutline{
		Title: "北海电厂二期智慧电厂项目技术方案",
		Sections: []dedicatedFullDocumentSection{
			{Number: 1, Title: "项目背景与建设目标", Heading: "第1章 项目背景与建设目标"},
			{Number: 2, Title: "数据湖与基础算力平台", Heading: "第2章 数据湖与基础算力平台", Subsections: []dedicatedFullDocumentSubsection{{Number: "2.1", Title: "全域数据湖建设"}, {Number: "2.2", Title: "标准化数据治理"}}},
		},
	}, dedicatedFullDocumentSection{Number: 2, Title: "数据湖与基础算力平台", Heading: "第2章 数据湖与基础算力平台", Subsections: []dedicatedFullDocumentSubsection{{Number: "2.1", Title: "全域数据湖建设"}, {Number: "2.2", Title: "标准化数据治理"}}}, "## 第1章 项目背景与建设目标\n\n已有内容", knowledgeGroundedEvidencePack{
		Queries: []string{"请检索数据湖与算力平台建设内容"},
		Items: []knowledgeGroundedEvidenceItem{{
			Query:  "请检索数据湖与算力平台建设内容",
			Result: &types.SearchResult{KnowledgeTitle: "数据湖方案", Content: "数据湖平台包含统一汇聚、治理和算力调度能力。"},
		}},
	})
	require.Len(t, messages, 2)
	assert.Contains(t, messages[0].Content, "grounded in local knowledge")
	assert.Contains(t, messages[1].Content, "Completed document summary")
	assert.NotContains(t, messages[1].Content, "Completed content so far")
	assert.Contains(t, messages[0].Content, "### 3.1 全域数据湖建设")
	assert.Contains(t, messages[1].Content, "当前章节编号为：2")
	assert.Contains(t, messages[1].Content, "## 第2章 数据湖与基础算力平台")
	assert.Contains(t, messages[1].Content, "### 2.1 全域数据湖建设")
	assert.Contains(t, messages[1].Content, "### 2.2 标准化数据治理")
	assert.Contains(t, messages[1].Content, "<local_knowledge_context>")
	assert.Contains(t, messages[1].Content, "错误示例：###3.1全域数据湖建设")
	assert.Contains(t, messages[1].Content, "数据湖平台包含统一汇聚、治理和算力调度能力")
}

func TestBuildFullDocumentRollingSummary_UsesBoundedStructuredSummary(t *testing.T) {
	outline := dedicatedFullDocumentOutline{
		Title: "智慧运行建设方案",
		Sections: []dedicatedFullDocumentSection{
			{Number: 1, Title: "建设目标", Heading: "第1章 建设目标", Subsections: []dedicatedFullDocumentSubsection{{Number: "1.1", Title: "目标分解"}}},
			{Number: 2, Title: "平台架构", Heading: "第2章 平台架构", Subsections: []dedicatedFullDocumentSubsection{{Number: "2.1", Title: "全域数据湖建设"}, {Number: "2.2", Title: "标准化数据治理"}}},
			{Number: 3, Title: "实施保障", Heading: "第3章 实施保障"},
		},
	}
	completedContent := strings.Join([]string{
		"# 智慧运行建设方案",
		"",
		"## 第1章 建设目标",
		"",
		strings.Repeat("建设目标约束与边界说明。", 40),
		"",
		"## 第2章 平台架构",
		"",
		"### 2.1 全域数据湖建设",
		"",
		"平台架构围绕统一汇聚、治理、服务目录与算力调度展开。",
		"- 待确认：甲方侧现网接口接入边界。",
		strings.Repeat("平台架构细节说明。", 50),
		"不应整段透传到下一章prompt的尾部标记",
	}, "\n")

	summary := buildFullDocumentRollingSummary(outline, []string{"建设目标", "平台架构"}, completedContent)

	assert.Contains(t, summary, "## Completed document summary")
	assert.Contains(t, summary, "Completed sections: 2/3")
	assert.Contains(t, summary, "## 第1章 建设目标")
	assert.Contains(t, summary, "## 第2章 平台架构")
	assert.Contains(t, summary, "2.1 全域数据湖建设；2.2 标准化数据治理")
	assert.Contains(t, summary, "待确认")
	assert.NotContains(t, summary, "不应整段透传到下一章prompt的尾部标记")
}

func TestBuildFullDocumentRollingSummary_IncludesUnfinishedSectionSnapshot(t *testing.T) {
	outline := dedicatedFullDocumentOutline{
		Title: "智慧运行建设方案",
		Sections: []dedicatedFullDocumentSection{
			{Number: 1, Title: "建设目标", Heading: "第1章 建设目标"},
			{Number: 2, Title: "平台架构", Heading: "第2章 平台架构", Subsections: []dedicatedFullDocumentSubsection{{Number: "2.1", Title: "全域数据湖建设"}}},
			{Number: 3, Title: "实施保障", Heading: "第3章 实施保障"},
		},
	}
	completedContent := strings.Join([]string{
		"# 智慧运行建设方案",
		"",
		"## 第1章 建设目标",
		"",
		"说明建设目标与业务价值。",
		"",
		"## 第2章 平台架构",
		"",
		"### 2.1 全域数据湖建设",
		"",
		"当前已生成平台架构开篇，并明确接口依赖和模块边界。",
	}, "\n")

	summary := buildFullDocumentRollingSummary(outline, []string{"建设目标"}, completedContent)

	assert.Contains(t, summary, "Current unfinished section snapshot")
	assert.Contains(t, summary, "## 第2章 平台架构")
	assert.Contains(t, summary, "2.1 全域数据湖建设")
	assert.Contains(t, summary, "当前已生成平台架构开篇，并明确接口依赖和模块边界")
}

func TestBuildFullDocumentObservabilitySummary_ExtractsBudgetRunAndQualityIssues(t *testing.T) {
	summary := buildFullDocumentObservabilitySummary(map[string]interface{}{
		"generation_run_id": "run-obs-1",
		"quality_issues":    []interface{}{types.ChatDocumentQualityIssueMarkdownStructureInvalid, types.ChatDocumentQualityIssueMarkdownTooShort},
		"budget": map[string]interface{}{
			"source":                             "runtime_adjusted",
			"outline_max_completion_tokens":      1536,
			"section_max_completion_tokens":      4608,
			"continuation_max_completion_tokens": 5120,
			"outline_evidence_top_k":             8,
			"section_evidence_top_k":             9,
			"continuation_evidence_top_k":        10,
		},
	})

	assert.Equal(t, "run-obs-1", summary.GenerationRunID)
	assert.Equal(t, "runtime_adjusted", summary.BudgetSource)
	assert.Equal(t, 1536, summary.OutlineMaxTokens)
	assert.Equal(t, 4608, summary.SectionMaxTokens)
	assert.Equal(t, 5120, summary.ContinuationMaxTokens)
	assert.Equal(t, 8, summary.OutlineEvidenceTopK)
	assert.Equal(t, 9, summary.SectionEvidenceTopK)
	assert.Equal(t, 10, summary.ContinuationEvidenceTopK)
	assert.Equal(t, []string{types.ChatDocumentQualityIssueMarkdownStructureInvalid, types.ChatDocumentQualityIssueMarkdownTooShort}, summary.QualityIssues)
}

func TestBuildKnowledgeGroundedLocalKnowledgeContext_EscapesXMLBreakingCharacters(t *testing.T) {
	contextText := buildKnowledgeGroundedLocalKnowledgeContext(knowledgeGroundedEvidencePack{
		Queries: []string{"<query> & test"},
		Items: []knowledgeGroundedEvidenceItem{{
			Query: "风险 <section>",
			Result: &types.SearchResult{
				ID:              "chunk-1",
				KnowledgeBaseID: "kb-1",
				KnowledgeID:     "doc-1",
				KnowledgeTitle:  "标题 <危险>",
				Content:         "正文包含 <tag> 与 & 字符",
			},
		}},
	})
	assert.Contains(t, contextText, "&lt;query&gt; &amp; test")
	assert.Contains(t, contextText, "风险 &lt;section&gt;")
	assert.Contains(t, contextText, "标题 &lt;危险&gt;")
	assert.Contains(t, contextText, "正文包含 &lt;tag&gt; 与 &amp; 字符")
}

func TestBuildKnowledgeGroundedDocumentContinuationMessages_UsesOriginalGoalAndSnapshot(t *testing.T) {
	req := &types.QARequest{
		Query:                     "以当前文档为基准，继续剩余内容输出",
		AutoContinuePrompt:        "以当前文档为基准，继续剩余内容输出",
		AutoContinueOriginalQuery: "请输出完整的智慧运行投标技术方案",
	}
	baseArtifact := &types.ChatDocumentArtifact{ContentSnapshot: "# 智慧运行方案\n\n## 第一章\n\n已有内容"}
	messages := buildKnowledgeGroundedDocumentContinuationMessages(req, "Chinese (Simplified)", baseArtifact, knowledgeGroundedEvidencePack{
		Items: []knowledgeGroundedEvidenceItem{{Result: &types.SearchResult{ID: "chunk-1", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "智慧运行总体方案", Content: "增量内容依据。"}}},
	})
	require.Len(t, messages, 2)
	assert.Contains(t, messages[1].Content, "请输出完整的智慧运行投标技术方案")
	assert.Contains(t, messages[1].Content, "# 智慧运行方案")
	assert.Contains(t, messages[1].Content, "<local_knowledge_context>")
}

func TestBuildDedicatedDocumentEditMessages(t *testing.T) {
	req := &types.QARequest{
		Query:              "把 2.5.5 后续内容合并到第二章",
		DocumentIntent:     types.ChatDocumentIntentRevise,
		DocumentOutputMode: types.ChatDocumentOutputModeDelta,
		BaseArtifactID:     "artifact-1",
		QuotedContext:      "<document_revision_context><source_anchor_heading>2.5.5</source_anchor_heading></document_revision_context>",
	}

	messages := buildDedicatedDocumentEditMessages(req, "Chinese (Simplified)")
	require.Len(t, messages, 2)
	assert.Contains(t, messages[0].Content, "dedicated document editor")
	assert.Contains(t, messages[0].Content, "<document_patch>")
	assert.Contains(t, messages[1].Content, req.Query)
	assert.Contains(t, messages[1].Content, "<document_revision_context>")
}

func TestBuildDeterministicDocumentEditPatch_MoveAfterHeadingToSection(t *testing.T) {
	req := &types.QARequest{
		Query:              "把 2.5.5 火电设备运维智能体——技术实现 后续的内容，合并到第二章。",
		DocumentIntent:     types.ChatDocumentIntentRevise,
		DocumentOutputMode: types.ChatDocumentOutputModeDelta,
		BaseArtifactID:     "artifact-1",
		QuotedContext: `<document_revision_context>
上一份文档元数据：
- document_edit_operation: move_after_heading_to_section
- document_merge_mode: append_to_section

<source_anchor_heading>
### 2.5.5 火电设备运维智能体——技术实现
</source_anchor_heading>
<source_section>
### 2.5.5 火电设备运维智能体——技术实现

待迁移内容 A

#### 2.5.5.1 技术细节

待迁移内容 B
</source_section>
<destination_section_heading>
## 第二章
</destination_section_heading>
</document_revision_context>`,
	}

	patch, ok := buildDeterministicDocumentEditPatch(req)
	require.True(t, ok)
	assert.Contains(t, patch, `<document_patch>`)
	assert.Contains(t, patch, `<append heading="## 第二章">`)
	assert.Contains(t, patch, "### 2.5.5 火电设备运维智能体——技术实现")
	assert.Contains(t, patch, "待迁移内容 B")
}

func TestBuildDeterministicDocumentEditPatch_SkipsContainedSourceSection(t *testing.T) {
	sourceSection := `### 2.5.5 火电设备运维智能体——技术实现

待迁移内容 A`
	req := &types.QARequest{
		Query:              "把 2.5.5 火电设备运维智能体——技术实现 后续的内容，合并到第二章。",
		DocumentIntent:     types.ChatDocumentIntentRevise,
		DocumentOutputMode: types.ChatDocumentOutputModeDelta,
		BaseArtifactID:     "artifact-1",
		QuotedContext: `<document_revision_context>
上一份文档元数据：
- document_edit_operation: move_after_heading_to_section
- document_merge_mode: append_to_section

<source_section>
` + sourceSection + `
</source_section>
<destination_section_heading>
## 第二章
</destination_section_heading>
<destination_section>
## 第二章

第二章已有内容。

` + sourceSection + `
</destination_section>
</document_revision_context>`,
	}

	patch, ok := buildDeterministicDocumentEditPatch(req)
	assert.False(t, ok)
	assert.Empty(t, patch)
}

func TestRunDedicatedDocumentEditPath_UsesDeterministicPatchForMoveAppend(t *testing.T) {
	svc := &sessionService{}
	chatModel := &failIfCalledDocumentEditChat{}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:              "把 2.5.5 火电设备运维智能体——技术实现 后续的内容，合并到第二章。",
		Session:            &types.Session{ID: "sess-deterministic"},
		AssistantMessageID: "msg-deterministic",
		DocumentIntent:     types.ChatDocumentIntentRevise,
		DocumentOutputMode: types.ChatDocumentOutputModeDelta,
		BaseArtifactID:     "artifact-1",
		QuotedContext: `<document_revision_context>
上一份文档元数据：
- document_edit_operation: move_after_heading_to_section
- document_merge_mode: append_to_section
<source_section>
### 2.5.5 火电设备运维智能体——技术实现

待迁移内容 A
</source_section>
<destination_section_heading>
## 第二章
</destination_section_heading>
</document_revision_context>`,
	}

	var answerChunks []event.AgentFinalAnswerData
	var completes []event.AgentCompleteData
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		require.True(t, ok)
		answerChunks = append(answerChunks, data)
		return nil
	})
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runDedicatedDocumentEditPath(context.Background(), req, bus, chatModel, &types.AgentConfig{})
	require.NoError(t, err)
	assert.False(t, chatModel.called)
	require.Len(t, answerChunks, 1)
	assert.True(t, answerChunks[0].Done)
	assert.Contains(t, answerChunks[0].Content, `<append heading="## 第二章">`)
	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusCompleted, completes[0].CompletionStatus)
	assert.Equal(t, "deterministic_patch", completes[0].FinishReason)
	assert.Equal(t, answerChunks[0].Content, completes[0].FinalAnswer)
}

func TestConsumeDedicatedDocumentEditStream_ClassifiesTimeoutFailure(t *testing.T) {
	svc := &sessionService{}
	bus := event.NewEventBus()
	responses := make(chan types.StreamResponse, 2)
	responses <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: "partial patch"}
	responses <- types.StreamResponse{ResponseType: types.ResponseTypeError, Content: "context deadline exceeded"}
	close(responses)
	req := &types.QARequest{
		Session:            &types.Session{ID: "sess-1"},
		AssistantMessageID: "msg-1",
	}

	var completes []event.AgentCompleteData
	var errs []event.ErrorData
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})
	bus.On(event.EventError, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.ErrorData)
		require.True(t, ok)
		errs = append(errs, data)
		return nil
	})

	svc.consumeDedicatedDocumentEditStream(context.Background(), req, bus, responses, time.Second)

	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusPartial, completes[0].CompletionStatus)
	assert.Equal(t, "llm_timeout", completes[0].FinishReason)
	assert.Equal(t, "llm_timeout", completes[0].FailureReason)
	assert.Equal(t, "partial patch", completes[0].FinalAnswer)
	assert.Len(t, errs, 0)
}

func TestConsumeDedicatedDocumentEditStream_ClassifiesCancelledAfterVisibleContent(t *testing.T) {
	svc := &sessionService{}
	bus := event.NewEventBus()
	responses := make(chan types.StreamResponse, 1)
	responses <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: "partial patch"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := &types.QARequest{
		Session:            &types.Session{ID: "sess-1"},
		AssistantMessageID: "msg-1",
	}

	var completes []event.AgentCompleteData
	var errs []event.ErrorData
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})
	bus.On(event.EventError, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.ErrorData)
		require.True(t, ok)
		errs = append(errs, data)
		return nil
	})

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	svc.consumeDedicatedDocumentEditStream(ctx, req, bus, responses, time.Second)

	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusCancelled, completes[0].CompletionStatus)
	assert.Equal(t, types.MessageCompletionStatusCancelled, completes[0].FinishReason)
	assert.Equal(t, types.MessageCompletionStatusCancelled, completes[0].FailureReason)
	assert.Equal(t, "partial patch", completes[0].FinalAnswer)
	assert.Len(t, errs, 0)
}

func TestEmitDedicatedDocumentEditFailure_ClassifiesCancelledWithoutError(t *testing.T) {
	svc := &sessionService{}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Session:            &types.Session{ID: "sess-cancelled"},
		AssistantMessageID: "msg-cancelled",
	}

	var completes []event.AgentCompleteData
	var errs []event.ErrorData
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})
	bus.On(event.EventError, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.ErrorData)
		require.True(t, ok)
		errs = append(errs, data)
		return nil
	})

	err := svc.emitDedicatedDocumentEditFailure(context.Background(), req, bus, types.MessageCompletionStatusCancelled, context.Canceled)

	assert.ErrorIs(t, err, context.Canceled)
	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusCancelled, completes[0].CompletionStatus)
	assert.Equal(t, types.MessageCompletionStatusCancelled, completes[0].FinishReason)
	assert.Equal(t, types.MessageCompletionStatusCancelled, completes[0].FailureReason)
	assert.Empty(t, completes[0].FinalAnswer)
	assert.Len(t, errs, 0)
}

func TestConsumeDedicatedDocumentEditStream_EmitsProgressBeforeFirstContentTimeout(t *testing.T) {
	oldHeartbeat := dedicatedDocumentEditProgressHeartbeatInterval
	oldTimeout := dedicatedDocumentEditFirstContentTimeout
	dedicatedDocumentEditProgressHeartbeatInterval = 5 * time.Millisecond
	dedicatedDocumentEditFirstContentTimeout = 40 * time.Millisecond
	defer func() {
		dedicatedDocumentEditProgressHeartbeatInterval = oldHeartbeat
		dedicatedDocumentEditFirstContentTimeout = oldTimeout
	}()

	svc := &sessionService{}
	bus := event.NewEventBus()
	responses := make(chan types.StreamResponse)
	req := &types.QARequest{
		Session:            &types.Session{ID: "sess-progress"},
		AssistantMessageID: "msg-progress",
	}

	var thoughts []event.AgentThoughtData
	var completes []event.AgentCompleteData
	bus.On(event.EventAgentThought, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentThoughtData)
		require.True(t, ok)
		thoughts = append(thoughts, data)
		return nil
	})
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	go func() {
		time.Sleep(15 * time.Millisecond)
		responses <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: "<document_patch>", Done: false}
		time.Sleep(2 * time.Millisecond)
		responses <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: "</document_patch>", Done: true, FinishReason: "stop"}
		close(responses)
	}()

	svc.consumeDedicatedDocumentEditStream(context.Background(), req, bus, responses, dedicatedDocumentEditFirstContentTimeout)

	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusCompleted, completes[0].CompletionStatus)
	assert.Equal(t, "<document_patch></document_patch>", completes[0].FinalAnswer)
	require.GreaterOrEqual(t, len(thoughts), 3)
	assert.Contains(t, thoughts[0].Content, "正在分析基线文档")
	assert.True(t, thoughts[0].Replace)
	assert.True(t, thoughts[0].Synthetic)
	assert.True(t, thoughts[len(thoughts)-1].Done)
	hasWaitingHeartbeat := false
	for _, thought := range thoughts {
		if strings.Contains(thought.Content, "仍在等待模型") {
			hasWaitingHeartbeat = true
			assert.True(t, thought.Replace)
			assert.True(t, thought.Synthetic)
			break
		}
	}
	assert.True(t, hasWaitingHeartbeat)
}

func TestConsumeDedicatedDocumentEditStream_ClassifiesFirstContentTimeout(t *testing.T) {
	oldHeartbeat := dedicatedDocumentEditProgressHeartbeatInterval
	oldTimeout := dedicatedDocumentEditFirstContentTimeout
	dedicatedDocumentEditProgressHeartbeatInterval = 5 * time.Millisecond
	dedicatedDocumentEditFirstContentTimeout = 20 * time.Millisecond
	defer func() {
		dedicatedDocumentEditProgressHeartbeatInterval = oldHeartbeat
		dedicatedDocumentEditFirstContentTimeout = oldTimeout
	}()

	svc := &sessionService{}
	bus := event.NewEventBus()
	responses := make(chan types.StreamResponse)
	req := &types.QARequest{
		Session:            &types.Session{ID: "sess-timeout"},
		AssistantMessageID: "msg-timeout",
	}

	var thoughts []event.AgentThoughtData
	var completes []event.AgentCompleteData
	var errs []event.ErrorData
	bus.On(event.EventAgentThought, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentThoughtData)
		require.True(t, ok)
		thoughts = append(thoughts, data)
		return nil
	})
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})
	bus.On(event.EventError, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.ErrorData)
		require.True(t, ok)
		errs = append(errs, data)
		return nil
	})

	svc.consumeDedicatedDocumentEditStream(context.Background(), req, bus, responses, dedicatedDocumentEditFirstContentTimeout)

	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusFailed, completes[0].CompletionStatus)
	assert.Equal(t, "first_visible_stream_timeout", completes[0].FinishReason)
	assert.Equal(t, "first_visible_stream_timeout", completes[0].FailureReason)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error, "timed out waiting for first visible document edit stream")
	require.NotEmpty(t, thoughts)
	assert.Contains(t, thoughts[0].Content, "正在分析基线文档")
	assert.True(t, thoughts[len(thoughts)-1].Done)
}

func TestConsumeDedicatedDocumentEditStream_StreamsModelThinkingBeforeAnswer(t *testing.T) {
	oldHeartbeat := dedicatedDocumentEditProgressHeartbeatInterval
	oldTimeout := dedicatedDocumentEditFirstContentTimeout
	dedicatedDocumentEditProgressHeartbeatInterval = 5 * time.Millisecond
	dedicatedDocumentEditFirstContentTimeout = 15 * time.Millisecond
	defer func() {
		dedicatedDocumentEditProgressHeartbeatInterval = oldHeartbeat
		dedicatedDocumentEditFirstContentTimeout = oldTimeout
	}()

	svc := &sessionService{}
	bus := event.NewEventBus()
	responses := make(chan types.StreamResponse)
	req := &types.QARequest{
		Session:            &types.Session{ID: "sess-thinking"},
		AssistantMessageID: "msg-thinking",
	}

	type thoughtEvent struct {
		id   string
		data event.AgentThoughtData
	}
	var thoughts []thoughtEvent
	var completes []event.AgentCompleteData
	bus.On(event.EventAgentThought, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentThoughtData)
		require.True(t, ok)
		thoughts = append(thoughts, thoughtEvent{id: evt.ID, data: data})
		return nil
	})
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	go func() {
		time.Sleep(5 * time.Millisecond)
		responses <- types.StreamResponse{ResponseType: types.ResponseTypeThinking, Content: "我们先定位源章节", Done: false}
		time.Sleep(25 * time.Millisecond)
		responses <- types.StreamResponse{ResponseType: types.ResponseTypeThinking, Content: "并组织修订补丁", Done: false}
		responses <- types.StreamResponse{ResponseType: types.ResponseTypeThinking, Done: true}
		responses <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: "<document_patch>patched</document_patch>", Done: true, FinishReason: "stop"}
		close(responses)
	}()

	svc.consumeDedicatedDocumentEditStream(context.Background(), req, bus, responses, dedicatedDocumentEditFirstContentTimeout)

	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusCompleted, completes[0].CompletionStatus)
	assert.Equal(t, "<document_patch>patched</document_patch>", completes[0].FinalAnswer)

	var modelThinkingID string
	var modelThinkingContent strings.Builder
	modelThinkingDone := false
	for _, thought := range thoughts {
		if strings.Contains(thought.data.Content, "定位源章节") || strings.Contains(thought.data.Content, "组织修订补丁") {
			if modelThinkingID == "" {
				modelThinkingID = thought.id
			}
			assert.Equal(t, modelThinkingID, thought.id)
			assert.False(t, thought.data.Replace)
			assert.False(t, thought.data.Synthetic)
			modelThinkingContent.WriteString(thought.data.Content)
		}
		if modelThinkingID != "" && thought.id == modelThinkingID && thought.data.Done {
			modelThinkingDone = true
		}
	}
	assert.Contains(t, modelThinkingContent.String(), "我们先定位源章节")
	assert.Contains(t, modelThinkingContent.String(), "并组织修订补丁")
	assert.True(t, modelThinkingDone)
}

func TestConsumeDedicatedDocumentEditStream_AllowsEmptyDoneAfterContent(t *testing.T) {
	svc := &sessionService{}
	bus := event.NewEventBus()
	responses := make(chan types.StreamResponse, 2)
	responses <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: "<document_patch>patched</document_patch>", Done: false}
	responses <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: "", Done: true, FinishReason: "stop"}
	close(responses)
	req := &types.QARequest{
		Session:            &types.Session{ID: "sess-empty-done"},
		AssistantMessageID: "msg-empty-done",
	}

	var completes []event.AgentCompleteData
	var errs []event.ErrorData
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})
	bus.On(event.EventError, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.ErrorData)
		require.True(t, ok)
		errs = append(errs, data)
		return nil
	})

	svc.consumeDedicatedDocumentEditStream(context.Background(), req, bus, responses, time.Second)

	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusCompleted, completes[0].CompletionStatus)
	assert.Equal(t, "stop", completes[0].FinishReason)
	assert.Equal(t, "<document_patch>patched</document_patch>", completes[0].FinalAnswer)
	assert.Empty(t, errs)
}

func TestRunDedicatedDocumentEditPath_WaitsForStreamCompletionBeforeReturn(t *testing.T) {
	svc := &sessionService{cfg: &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}}}
	responses := make(chan types.StreamResponse)
	chatModel := &blockingDocumentEditChat{
		responses: responses,
		started:   make(chan struct{}),
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:              "把 2.5.5 后续内容合并到第二章",
		Session:            &types.Session{ID: "sess-1"},
		AssistantMessageID: "msg-1",
		DocumentIntent:     types.ChatDocumentIntentRevise,
		DocumentOutputMode: types.ChatDocumentOutputModeDelta,
		BaseArtifactID:     "artifact-1",
		QuotedContext:      "<document_revision_context>...</document_revision_context>",
	}

	var completes []event.AgentCompleteData
	var thoughts []event.AgentThoughtData
	bus.On(event.EventAgentThought, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentThoughtData)
		require.True(t, ok)
		thoughts = append(thoughts, data)
		return nil
	})
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	done := make(chan error, 1)
	go func() {
		done <- svc.runDedicatedDocumentEditPath(context.Background(), req, bus, chatModel, &types.AgentConfig{})
	}()

	select {
	case <-chatModel.started:
	case <-time.After(time.Second):
		t.Fatal("expected document edit stream to start")
	}

	select {
	case err := <-done:
		t.Fatalf("runDedicatedDocumentEditPath returned before stream completion: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	responses <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: "<document_patch>", Done: false}
	select {
	case err := <-done:
		t.Fatalf("runDedicatedDocumentEditPath returned before done chunk: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	responses <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: "</document_patch>", Done: true, FinishReason: "stop"}
	close(responses)

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("expected document edit path to return after stream completion")
	}

	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusCompleted, completes[0].CompletionStatus)
	assert.Equal(t, "<document_patch></document_patch>", completes[0].FinalAnswer)
}

func TestRunDedicatedFullDocumentGenerationPath_CompletesAllPlannedSectionsInInitialRun(t *testing.T) {
	oldLimit := dedicatedFullDocumentSectionLimitPerRun
	dedicatedFullDocumentSectionLimitPerRun = 2
	defer func() {
		dedicatedFullDocumentSectionLimitPerRun = oldLimit
	}()

	svc := &sessionService{cfg: &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}}}
	chatModel := &stagedFullDocumentChat{
		outlineResponse: types.ChatResponse{Content: "# 北海电厂二期智慧电厂项目投标技术方案\n\n## 一、项目背景与总体思路\n## 二、总体技术架构\n## 三、核心价值\n"},
		outlineStream: []types.StreamResponse{
			{ResponseType: types.ResponseTypeAnswer, Content: "# 北海电厂二期智慧电厂项目投标技术方案\n\n## 一、项目背景与总体思路\n", Done: false},
			{ResponseType: types.ResponseTypeAnswer, Content: "## 二、总体技术架构\n", Done: false},
			{ResponseType: types.ResponseTypeAnswer, Content: "## 三、核心价值\n", Done: true, FinishReason: "stop"},
		},
		sectionStreams: [][]types.StreamResponse{
			{{ResponseType: types.ResponseTypeAnswer, Content: "本节说明项目背景与总体目标。", Done: true, FinishReason: "stop"}},
			{{ResponseType: types.ResponseTypeAnswer, Content: "本节说明总体技术架构与平台分层。", Done: true, FinishReason: "stop"}},
			{{ResponseType: types.ResponseTypeAnswer, Content: "本节说明核心价值与预期收益。", Done: true, FinishReason: "stop"}},
		},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:              "请输出完整的投标技术方案",
		Session:            &types.Session{ID: "sess-full-document"},
		AssistantMessageID: "msg-full-document",
		DocumentOutputMode: types.ChatDocumentOutputModeFull,
	}

	var answers []event.AgentFinalAnswerData
	var completes []event.AgentCompleteData
	var thoughts []event.AgentThoughtData
	bus.On(event.EventAgentThought, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentThoughtData)
		require.True(t, ok)
		thoughts = append(thoughts, data)
		return nil
	})
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		require.True(t, ok)
		answers = append(answers, data)
		return nil
	})
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runDedicatedFullDocumentGenerationPath(context.Background(), req, bus, chatModel, &types.AgentConfig{Temperature: 0.3})
	require.NoError(t, err)
	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusCompleted, completes[0].CompletionStatus)
	assert.Equal(t, "stop", completes[0].FinishReason)
	assert.Equal(t, types.ChatDocumentGenerationStatusCompleted, completes[0].DocumentGenerationStatus)
	require.NotNil(t, completes[0].AutoContinueNext)
	assert.False(t, *completes[0].AutoContinueNext)
	assert.True(t, completes[0].AllowComplete)
	assert.True(t, completes[0].AllowIndexing)
	require.NotNil(t, completes[0].Extra)
	outlinePayload, ok := completes[0].Extra["outline"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "北海电厂二期智慧电厂项目投标技术方案", outlinePayload["title"])
	assert.Contains(t, completes[0].FinalAnswer, "# 北海电厂二期智慧电厂项目投标技术方案")
	assert.Contains(t, completes[0].FinalAnswer, "## 第1章 一、项目背景与总体思路")
	assert.Contains(t, completes[0].FinalAnswer, "## 第2章 二、总体技术架构")
	assert.Contains(t, completes[0].FinalAnswer, "## 第3章 三、核心价值")
	assert.Equal(t, 0, chatModel.chatCalls)
	assert.Equal(t, 4, chatModel.streamCalls)
	require.NotEmpty(t, answers)
	assert.True(t, answers[len(answers)-1].Done)
	assert.Equal(t, types.MessageCompletionStatusCompleted, answers[len(answers)-1].CompletionStatus)
	assert.Equal(t, "stop", answers[len(answers)-1].FinishReason)
	require.NotNil(t, completes[0].Extra)
	budgetPayload, ok := completes[0].Extra["budget"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "fallback", budgetPayload["source"])
	assert.Equal(t, 1536, budgetPayload["outline_max_completion_tokens"])
	assert.Equal(t, 4096, budgetPayload["section_max_completion_tokens"])
	assert.Equal(t, 4096, budgetPayload["continuation_max_completion_tokens"])
	runtimeFeedbackPayload, ok := completes[0].Extra["runtime_feedback"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 0, runtimeFeedbackPayload["low_evidence_count"])
	sectionsPayload, ok := runtimeFeedbackPayload["sections"].([]map[string]interface{})
	if !ok {
		genericSections, genericOK := runtimeFeedbackPayload["sections"].([]interface{})
		require.True(t, genericOK)
		require.NotEmpty(t, genericSections)
		firstSection, mapOK := genericSections[0].(map[string]interface{})
		require.True(t, mapOK)
		_, hasEvidenceCount := firstSection["evidence_count"]
		assert.False(t, hasEvidenceCount)
	} else {
		require.NotEmpty(t, sectionsPayload)
		_, hasEvidenceCount := sectionsPayload[0]["evidence_count"]
		assert.False(t, hasEvidenceCount)
	}
	require.NotEmpty(t, thoughts)
	var sawOutlineProgress bool
	var sawOutlineStreamingProgress bool
	var sawSectionProgress bool
	var sawPlanningStage bool
	var sawGeneratingStage bool
	var sawPublishedOutline bool
	for _, thought := range thoughts {
		if strings.Contains(thought.Content, "已规划 3 个章节，将连续生成全部章节") {
			sawOutlineProgress = true
		}
		if strings.Contains(thought.Content, "正在生成大纲：已识别 3 个章节") {
			sawOutlineStreamingProgress = true
		}
		if strings.Contains(thought.Content, "正在生成第 1/3 章") {
			sawSectionProgress = true
		}
		if thought.Stage == "planning" {
			sawPlanningStage = true
		}
		if thought.Stage == "generating" {
			sawGeneratingStage = true
		}
		if thought.Stage == "planning" && thought.Done && !thought.Replace && thought.Synthetic && strings.Contains(thought.Content, "# 北海电厂二期智慧电厂项目投标技术方案") && strings.Contains(thought.Content, "## 第1章 一、项目背景与总体思路") && strings.Contains(thought.Content, "## 第2章 二、总体技术架构") && strings.Contains(thought.Content, "## 第3章 三、核心价值") {
			sawPublishedOutline = true
		}
	}
	assert.True(t, sawOutlineProgress)
	assert.True(t, sawOutlineStreamingProgress)
	assert.True(t, sawSectionProgress)
	assert.True(t, sawPlanningStage)
	assert.True(t, sawGeneratingStage)
	assert.True(t, sawPublishedOutline)
	assert.True(t, thoughts[len(thoughts)-1].Done)
	require.Len(t, chatModel.streamOptions, 4)
	for index, options := range chatModel.streamOptions {
		require.NotNil(t, options.Thinking)
		assert.False(t, *options.Thinking)
		if index == 0 {
			assert.Equal(t, 1536, options.MaxCompletionTokens)
			continue
		}
		assert.Zero(t, options.MaxCompletionTokens)
	}
}

func TestRunDedicatedFullDocumentGenerationPath_CompletesWhenAllSectionsFitInOneRun(t *testing.T) {
	oldLimit := dedicatedFullDocumentSectionLimitPerRun
	dedicatedFullDocumentSectionLimitPerRun = 4
	defer func() {
		dedicatedFullDocumentSectionLimitPerRun = oldLimit
	}()

	svc := &sessionService{cfg: &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}}}
	chatModel := &stagedFullDocumentChat{
		outlineResponse: types.ChatResponse{Content: "# 智慧运行建设方案\n\n## 一、建设目标\n## 二、实施路径\n"},
		sectionStreams: [][]types.StreamResponse{
			{{ResponseType: types.ResponseTypeAnswer, Content: "说明建设目标与预期收益。", Done: true, FinishReason: "stop"}},
			{{ResponseType: types.ResponseTypeAnswer, Content: "说明实施路径与保障措施。", Done: true, FinishReason: "stop"}},
		},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:              "请输出完整的智慧运行建设方案",
		Session:            &types.Session{ID: "sess-full-document-complete"},
		AssistantMessageID: "msg-full-document-complete",
		DocumentOutputMode: types.ChatDocumentOutputModeFull,
	}

	var completes []event.AgentCompleteData
	var thoughts []event.AgentThoughtData
	bus.On(event.EventAgentThought, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentThoughtData)
		require.True(t, ok)
		thoughts = append(thoughts, data)
		return nil
	})
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runDedicatedFullDocumentGenerationPath(context.Background(), req, bus, chatModel, &types.AgentConfig{})
	require.NoError(t, err)
	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusCompleted, completes[0].CompletionStatus)
	assert.Equal(t, "stop", completes[0].FinishReason)
	assert.Equal(t, types.ChatDocumentGenerationStatusCompleted, completes[0].DocumentGenerationStatus)
	require.NotNil(t, completes[0].AutoContinueNext)
	assert.False(t, *completes[0].AutoContinueNext)
	assert.True(t, completes[0].AllowComplete)
	assert.True(t, completes[0].AllowIndexing)
	assert.Contains(t, completes[0].FinalAnswer, "## 第1章 一、建设目标")
	assert.Contains(t, completes[0].FinalAnswer, "## 第2章 二、实施路径")
	assert.Equal(t, 3, chatModel.streamCalls)
	var sawOutlineStreamingProgress bool
	for _, thought := range thoughts {
		if strings.Contains(thought.Content, "正在生成大纲：已识别 2 个章节") {
			sawOutlineStreamingProgress = true
		}
	}
	assert.True(t, sawOutlineStreamingProgress)
}

func TestRunDedicatedFullDocumentGenerationPath_NormalizesFinalAnswerAndEmitsQualityIssues(t *testing.T) {
	oldLimit := dedicatedFullDocumentSectionLimitPerRun
	dedicatedFullDocumentSectionLimitPerRun = 4
	defer func() {
		dedicatedFullDocumentSectionLimitPerRun = oldLimit
	}()

	svc := &sessionService{cfg: &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}}}
	chatModel := &stagedFullDocumentChat{
		outlineResponse: types.ChatResponse{Content: `{"title":"智慧运行建设方案","sections":[{"number":1,"title":"建设目标","heading":"第1章 建设目标","subsections":[{"number":"1.1","title":"总体目标"}]}]}`},
		sectionStreams: [][]types.StreamResponse{{
			{ResponseType: types.ResponseTypeAnswer, Content: "###1.1总体目标\n -事项一\n**结论：**需要尽快推进", Done: true, FinishReason: "stop"},
		}},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:              "请输出完整的智慧运行建设方案",
		Session:            &types.Session{ID: "sess-full-document-normalized"},
		AssistantMessageID: "msg-full-document-normalized",
		DocumentOutputMode: types.ChatDocumentOutputModeFull,
	}

	var completes []event.AgentCompleteData
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runDedicatedFullDocumentGenerationPath(context.Background(), req, bus, chatModel, &types.AgentConfig{})
	require.NoError(t, err)
	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusCompleted, completes[0].CompletionStatus)
	assert.Contains(t, completes[0].FinalAnswer, "### 1.1 总体目标")
	assert.Contains(t, completes[0].FinalAnswer, "  - 事项一")
	assert.Contains(t, completes[0].FinalAnswer, "**结论：** 需要尽快推进")
	require.NotNil(t, completes[0].Extra)
	issues, ok := completes[0].Extra["quality_issues"].([]string)
	if !ok {
		rawIssues, rawOK := completes[0].Extra["quality_issues"].([]interface{})
		require.True(t, rawOK)
		issues = make([]string, 0, len(rawIssues))
		for _, item := range rawIssues {
			text, typeOK := item.(string)
			require.True(t, typeOK)
			issues = append(issues, text)
		}
	}
	assert.Contains(t, issues, types.ChatDocumentQualityIssueMarkdownHeadingNormalized)
}

func TestRunDedicatedFullDocumentGenerationPath_DeterministicallyRepairsGluedSubsectionHeading(t *testing.T) {
	svc := &sessionService{cfg: &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}}}
	chatModel := &stagedFullDocumentChat{
		outlineResponse: types.ChatResponse{Content: `{"title":"智慧运行建设方案","sections":[{"number":1,"title":"建设目标","heading":"第1章 建设目标","subsections":[{"number":"1.1","title":"总体目标"}]}]}`},
		sectionStreams: [][]types.StreamResponse{{
			{ResponseType: types.ResponseTypeAnswer, Content: "## 第1章 建设目标\n\n### 1 .1 总体目标**建设内容：**围绕智慧运行能力建设，形成目标、路径与保障闭环。", Done: true, FinishReason: "stop"},
		}},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:              "请输出完整的智慧运行建设方案",
		Session:            &types.Session{ID: "sess-full-document-glued-heading"},
		AssistantMessageID: "msg-full-document-glued-heading",
		DocumentOutputMode: types.ChatDocumentOutputModeFull,
	}

	var completes []event.AgentCompleteData
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runDedicatedFullDocumentGenerationPath(context.Background(), req, bus, chatModel, &types.AgentConfig{})
	require.NoError(t, err)
	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusCompleted, completes[0].CompletionStatus)
	assert.Equal(t, types.ChatDocumentGenerationStatusCompleted, completes[0].DocumentGenerationStatus)
	assert.Contains(t, completes[0].FinalAnswer, "### 1.1 总体目标\n\n**建设内容：** 围绕智慧运行能力建设")
	assert.NotContains(t, completes[0].FinalAnswer, "总体目标**建设内容")
	assert.NotContains(t, completes[0].FinalAnswer, "## 第1章 建设目标\n\n## 第1章 建设目标")
}

func TestRunDedicatedFullDocumentGenerationPath_ContinuesWhenMarkdownQualityGateWarns(t *testing.T) {
	oldLimit := dedicatedFullDocumentSectionLimitPerRun
	dedicatedFullDocumentSectionLimitPerRun = 4
	defer func() {
		dedicatedFullDocumentSectionLimitPerRun = oldLimit
	}()

	svc := &sessionService{cfg: &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}}}
	chatModel := &stagedFullDocumentChat{
		outlineResponse: types.ChatResponse{Content: `{"title":"智慧运行建设方案","sections":[{"number":1,"title":"建设目标","heading":"第1章 建设目标"}]}`},
		repairErr:       assert.AnError,
		sectionStreams: [][]types.StreamResponse{{
			{ResponseType: types.ResponseTypeAnswer, Content: "## 第1章 建设目标\n\n说明内容覆盖目标、路径与保障，足以形成可读章节。", Done: true, FinishReason: "stop"},
		}},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:              "请输出完整的智慧运行建设方案",
		Session:            &types.Session{ID: "sess-full-document-quality-failed"},
		AssistantMessageID: "msg-full-document-quality-failed",
		DocumentOutputMode: types.ChatDocumentOutputModeFull,
	}

	var completes []event.AgentCompleteData
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runDedicatedFullDocumentGenerationPath(context.Background(), req, bus, chatModel, &types.AgentConfig{})
	require.NoError(t, err)
	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusCompleted, completes[0].CompletionStatus)
	assert.Equal(t, "stop", completes[0].FinishReason)
	assert.Empty(t, completes[0].FailureReason)
	assert.Equal(t, types.ChatDocumentGenerationStatusCompleted, completes[0].DocumentGenerationStatus)
	require.NotNil(t, completes[0].Extra)
	issues, ok := completes[0].Extra["quality_issues"].([]string)
	if !ok {
		rawIssues, rawOK := completes[0].Extra["quality_issues"].([]interface{})
		require.True(t, rawOK)
		issues = make([]string, 0, len(rawIssues))
		for _, item := range rawIssues {
			text, typeOK := item.(string)
			require.True(t, typeOK)
			issues = append(issues, text)
		}
	}
	assert.Contains(t, issues, types.ChatDocumentQualityIssueMarkdownStructureInvalid)
}

func TestRunDedicatedFullDocumentGenerationPath_CompletesNeedsReviewWhenStructureWarningsRemain(t *testing.T) {
	svc := &sessionService{cfg: &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}}}
	chatModel := &stagedFullDocumentChat{
		outlineResponse: types.ChatResponse{Content: `{"title":"智慧运行建设方案","sections":[{"number":1,"title":"建设目标","heading":"第1章 建设目标","subsections":[{"number":"1.1","title":"总体目标"}]}]}`},
		repairErr:       assert.AnError,
		sectionStreams: [][]types.StreamResponse{{
			{ResponseType: types.ResponseTypeAnswer, Content: "### 1.1 错误标题\n\n正文内容覆盖目标、路径与保障，足以形成可用章节。", Done: true, FinishReason: "stop"},
		}},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:              "请输出完整的智慧运行建设方案",
		Session:            &types.Session{ID: "sess-full-document-needs-review"},
		AssistantMessageID: "msg-full-document-needs-review",
		DocumentOutputMode: types.ChatDocumentOutputModeFull,
	}

	var completes []event.AgentCompleteData
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runDedicatedFullDocumentGenerationPath(context.Background(), req, bus, chatModel, &types.AgentConfig{})
	require.NoError(t, err)
	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusCompleted, completes[0].CompletionStatus)
	assert.Equal(t, "stop", completes[0].FinishReason)
	assert.Empty(t, completes[0].FailureReason)
	assert.Equal(t, types.ChatDocumentGenerationStatusNeedsReview, completes[0].DocumentGenerationStatus)
	require.NotNil(t, completes[0].AutoContinueNext)
	assert.False(t, *completes[0].AutoContinueNext)
	require.NotNil(t, completes[0].Extra)
	issues, ok := completes[0].Extra["quality_issues"].([]string)
	if !ok {
		rawIssues, rawOK := completes[0].Extra["quality_issues"].([]interface{})
		require.True(t, rawOK)
		issues = make([]string, 0, len(rawIssues))
		for _, item := range rawIssues {
			text, typeOK := item.(string)
			require.True(t, typeOK)
			issues = append(issues, text)
		}
	}
	assert.Contains(t, issues, types.ChatDocumentQualityIssueMarkdownUnplannedSubsection)
}

func TestParseDedicatedFullDocumentOutline_NormalizesConcatenatedHeadings(t *testing.T) {
	outline := parseDedicatedFullDocumentOutline("# 北海电厂二期智慧电厂项目投标技术方案##项目背景与建设目标##总体技术架构与模块划分##智慧运行系统方案")

	assert.Equal(t, "北海电厂二期智慧电厂项目投标技术方案", outline.Title)
	assert.Equal(t, []string{"项目背景与建设目标", "总体技术架构与模块划分", "智慧运行系统方案"}, dedicatedFullDocumentSectionTitles(outline))
	assert.Equal(t, []string{"第1章 项目背景与建设目标", "第2章 总体技术架构与模块划分", "第3章 智慧运行系统方案"}, dedicatedFullDocumentSectionHeadings(outline))
}

func TestParseDedicatedFullDocumentOutline_ParsesStructuredJSONAndSubsections(t *testing.T) {
	outline := parseDedicatedFullDocumentOutline(`{"title":"北海电厂二期智慧电厂项目投标技术方案","sections":[{"number":1,"title":"项目背景与建设目标","heading":"第1章 项目背景与建设目标","subsections":[{"number":"1.1","title":"项目背景与行业机遇"},{"number":"1.2","title":"总体建设目标"}]},{"number":2,"title":"数据湖与基础算力平台技术方案","heading":"第2章 数据湖与基础算力平台技术方案","subsections":[{"number":"2.1","title":"全域数据湖建设"}]}]}`)

	assert.Equal(t, "北海电厂二期智慧电厂项目投标技术方案", outline.Title)
	require.Len(t, outline.Sections, 2)
	assert.Equal(t, 1, outline.Sections[0].Number)
	assert.Equal(t, "第1章 项目背景与建设目标", outline.Sections[0].Heading)
	assert.Equal(t, []dedicatedFullDocumentSubsection{{Number: "1.1", Title: "项目背景与行业机遇"}, {Number: "1.2", Title: "总体建设目标"}}, outline.Sections[0].Subsections)
}

func TestParseDedicatedFullDocumentOutline_ReindexesStructuredJSONNumbersAndSubsections(t *testing.T) {
	outline := parseDedicatedFullDocumentOutline(`{"title":"北海电厂二期智慧电厂项目投标技术方案","sections":[{"number":4,"title":"项目背景与建设目标","heading":"第4章 项目背景与建设目标","subsections":[{"number":"4.7","title":"项目背景与行业机遇"},{"number":"9.2","title":"总体建设目标"}]},{"number":9,"title":"数据湖与基础算力平台技术方案","heading":"第9章 数据湖与基础算力平台技术方案","subsections":[{"number":"3.9","title":"全域数据湖建设"}]}]}`)

	assert.Equal(t, "北海电厂二期智慧电厂项目投标技术方案", outline.Title)
	require.Len(t, outline.Sections, 2)
	assert.Equal(t, 1, outline.Sections[0].Number)
	assert.Equal(t, "第1章 项目背景与建设目标", outline.Sections[0].Heading)
	assert.Equal(t, []dedicatedFullDocumentSubsection{{Number: "1.1", Title: "项目背景与行业机遇"}, {Number: "1.2", Title: "总体建设目标"}}, outline.Sections[0].Subsections)
	assert.Equal(t, 2, outline.Sections[1].Number)
	assert.Equal(t, "第2章 数据湖与基础算力平台技术方案", outline.Sections[1].Heading)
	assert.Equal(t, []dedicatedFullDocumentSubsection{{Number: "2.1", Title: "全域数据湖建设"}}, outline.Sections[1].Subsections)
	require.NoError(t, validateDedicatedFullDocumentOutline(outline))
}

func TestCompletedFullDocumentIntegrityFailureReason_DetectsWrongSubsectionPlan(t *testing.T) {
	outline := dedicatedFullDocumentOutline{
		Title: "北海电厂二期智慧电厂项目投标技术方案",
		Sections: []dedicatedFullDocumentSection{{
			Number:  1,
			Title:   "项目背景与建设目标",
			Heading: "第1章 项目背景与建设目标",
			Subsections: []dedicatedFullDocumentSubsection{
				{Number: "1.1", Title: "项目背景与行业机遇"},
				{Number: "1.2", Title: "总体建设目标"},
			},
		}},
	}
	failure := completedFullDocumentIntegrityFailureReason(outline, []string{"项目背景与建设目标"}, "# 北海电厂二期智慧电厂项目投标技术方案\n\n## 第1章 项目背景与建设目标\n\n### 2.1 项目背景与行业机遇\n\n正文\n\n### 2.2 总体建设目标\n\n正文")
	assert.Equal(t, "outline_or_section_incomplete", failure)
}

func TestRunDedicatedFullDocumentGenerationPath_NormalizesConcatenatedOutlineInsteadOfFallingBackTo正文(t *testing.T) {
	svc := &sessionService{cfg: &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}}}
	chatModel := &stagedFullDocumentChat{
		outlineResponse: types.ChatResponse{Content: "# 北海电厂二期智慧电厂项目投标技术方案##项目背景与建设目标##总体技术架构与模块划分##智慧运行系统方案"},
		outlineStream: []types.StreamResponse{
			{ResponseType: types.ResponseTypeAnswer, Content: "# 北海电厂二期智慧电厂项目投标技术方案##项目背景与建设目标##总体技术架构与模块划分##智慧运行系统方案", Done: true, FinishReason: "stop"},
		},
		sectionStreams: [][]types.StreamResponse{
			{{ResponseType: types.ResponseTypeAnswer, Content: "本节说明项目背景与建设目标。", Done: true, FinishReason: "stop"}},
			{{ResponseType: types.ResponseTypeAnswer, Content: "本节说明总体技术架构与平台分层。", Done: true, FinishReason: "stop"}},
			{{ResponseType: types.ResponseTypeAnswer, Content: "本节说明智慧运行系统方案。", Done: true, FinishReason: "stop"}},
		},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:              "请输出完整的投标技术方案",
		Session:            &types.Session{ID: "sess-full-document-concatenated-outline"},
		AssistantMessageID: "msg-full-document-concatenated-outline",
		DocumentOutputMode: types.ChatDocumentOutputModeFull,
	}

	var completes []event.AgentCompleteData
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runDedicatedFullDocumentGenerationPath(context.Background(), req, bus, chatModel, &types.AgentConfig{})
	require.NoError(t, err)
	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusCompleted, completes[0].CompletionStatus)
	assert.Equal(t, types.ChatDocumentGenerationStatusCompleted, completes[0].DocumentGenerationStatus)
	assert.Contains(t, completes[0].FinalAnswer, "## 第1章 项目背景与建设目标")
	assert.Contains(t, completes[0].FinalAnswer, "## 第2章 总体技术架构与模块划分")
	assert.Contains(t, completes[0].FinalAnswer, "## 第3章 智慧运行系统方案")
	assert.NotContains(t, completes[0].FinalAnswer, "## 正文")
	assert.NotContains(t, completes[0].FinalAnswer, "项目投标技术方案##项目背景")
	assert.Equal(t, 4, chatModel.streamCalls)
}

func TestRunDedicatedFullDocumentGenerationPath_PartialWhenSectionStreamClosesWithoutDone(t *testing.T) {
	oldLimit := dedicatedFullDocumentSectionLimitPerRun
	dedicatedFullDocumentSectionLimitPerRun = 2
	defer func() {
		dedicatedFullDocumentSectionLimitPerRun = oldLimit
	}()

	svc := &sessionService{cfg: &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}}}
	chatModel := &stagedFullDocumentChat{
		outlineResponse: types.ChatResponse{Content: "# 智慧运行建设方案\n\n## 一、建设目标\n## 二、实施路径\n"},
		sectionStreams: [][]types.StreamResponse{
			{{ResponseType: types.ResponseTypeAnswer, Content: "说明建设目标与预期收益。", Done: true, FinishReason: "stop"}},
			{{ResponseType: types.ResponseTypeAnswer, Content: "说明实施路径与保障措施。", Done: false}},
		},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:              "请输出完整的智慧运行建设方案",
		Session:            &types.Session{ID: "sess-full-document-truncated"},
		AssistantMessageID: "msg-full-document-truncated",
		DocumentOutputMode: types.ChatDocumentOutputModeFull,
	}

	var completes []event.AgentCompleteData
	var thoughts []event.AgentThoughtData
	bus.On(event.EventAgentThought, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentThoughtData)
		require.True(t, ok)
		thoughts = append(thoughts, data)
		return nil
	})
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runDedicatedFullDocumentGenerationPath(context.Background(), req, bus, chatModel, &types.AgentConfig{})
	require.NoError(t, err)
	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusPartial, completes[0].CompletionStatus)
	assert.Equal(t, "section_generation_truncated", completes[0].FinishReason)
	assert.Equal(t, "section_generation_truncated", completes[0].FailureReason)
	assert.Equal(t, types.ChatDocumentGenerationStatusContinuing, completes[0].DocumentGenerationStatus)
	require.NotNil(t, completes[0].AutoContinueNext)
	assert.False(t, *completes[0].AutoContinueNext)
	assert.Equal(t, "section_generation_truncated", completes[0].AutoContinueReason)
	assert.Contains(t, completes[0].FinalAnswer, "## 第2章 二、实施路径")
	assert.Contains(t, completes[0].FinalAnswer, "说明实施路径与保障措施。")
}

func TestDocumentGenerationStatusForCompletedFullDocumentIntegrityFailure(t *testing.T) {
	assert.Equal(t, types.ChatDocumentGenerationStatusBlocked, documentGenerationStatusForCompletedFullDocumentIntegrityFailure("outline_parse_failed"))
	assert.Equal(t, types.ChatDocumentGenerationStatusContinuing, documentGenerationStatusForCompletedFullDocumentIntegrityFailure("outline_or_section_incomplete"))
}

func TestRunKnowledgeGroundedDocumentContinuationPath_DefaultsToPartialContinuingWithoutCompletionMarker(t *testing.T) {
	svc := &sessionService{
		cfg: &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}},
		knowledgeBaseService: &fullDocumentKnowledgeSearchStub{
			kbs: map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", Type: types.KnowledgeBaseTypeDocument}},
			results: map[string][]*types.SearchResult{
				"kb-1|请输出完整的智慧运行投标技术方案": {
					{ID: "chunk-1", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "智慧运行总体方案", Content: "支持继续输出实施与保障章节。", Score: 0.9},
				},
				"kb-1|请输出完整的智慧运行投标技术方案 继续剩余内容": {
					{ID: "chunk-2", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-2", KnowledgeTitle: "实施保障", Content: "支持继续输出实施保障内容。", Score: 0.88},
				},
				"kb-1|智慧运行方案 第一章": {
					{ID: "chunk-3", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-3", KnowledgeTitle: "第一章补充", Content: "支持第一章后续增量。", Score: 0.8},
				},
			},
		},
	}
	chatModel := &stagedFullDocumentChat{
		sectionStreams: [][]types.StreamResponse{{
			{ResponseType: types.ResponseTypeAnswer, Content: "继续补充第二章实施与保障内容。", Done: true, FinishReason: "stop"},
		}},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:                     "以当前文档为基准，继续剩余内容输出",
		Session:                   &types.Session{ID: "sess-grounded-continuation"},
		AssistantMessageID:        "msg-grounded-continuation",
		DocumentIntent:            types.ChatDocumentIntentContinue,
		DocumentOperation:         types.ChatDocumentOperationContinue,
		DocumentOutputMode:        types.ChatDocumentOutputModeDelta,
		BaseArtifactID:            "artifact-1",
		BaseArtifact:              &types.ChatDocumentArtifact{ID: "artifact-1", ContentSnapshot: "# 智慧运行方案\n\n## 第一章\n\n已有内容"},
		AutoContinue:              true,
		AutoContinuePrompt:        "以当前文档为基准，继续剩余内容输出",
		AutoContinueOriginalQuery: "请输出完整的智慧运行投标技术方案",
	}
	agentConfig := &types.AgentConfig{
		Temperature:    0.3,
		KnowledgeBases: []string{"kb-1"},
		SearchTargets:  types.SearchTargets{{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase}},
	}

	var completes []event.AgentCompleteData
	var thoughts []event.AgentThoughtData
	bus.On(event.EventAgentThought, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentThoughtData)
		require.True(t, ok)
		thoughts = append(thoughts, data)
		return nil
	})
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runKnowledgeGroundedDocumentContinuationPath(context.Background(), req, bus, chatModel, agentConfig)
	require.NoError(t, err)
	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusPartial, completes[0].CompletionStatus)
	assert.Equal(t, types.ChatDocumentGenerationStatusContinuing, completes[0].DocumentGenerationStatus)
	require.NotNil(t, completes[0].AutoContinueNext)
	assert.True(t, *completes[0].AutoContinueNext)
	require.NotNil(t, completes[0].Extra)
	outlinePayload, ok := completes[0].Extra["outline"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "智慧运行方案", outlinePayload["title"])
	assert.Equal(t, "base_document", completes[0].Extra["outline_role"])
	assert.False(t, completes[0].AllowComplete)
	assert.False(t, completes[0].AllowIndexing)
	assert.Contains(t, completes[0].FinalAnswer, "继续补充第二章实施与保障内容")
	assert.NotEmpty(t, completes[0].KnowledgeRefs)
	var sawRetrievalProgress bool
	var sawGenerationProgress bool
	var sawRetrievingStage bool
	var sawGeneratingStage bool
	for _, thought := range thoughts {
		if strings.Contains(thought.Content, "正在检索本地知识库（1/3）") {
			sawRetrievalProgress = true
		}
		if strings.Contains(thought.Content, "已命中 3 条本地知识证据，正在继续生成剩余文档内容") || strings.Contains(thought.Content, "当前轮剩余内容已生成，正在判断是否完成全文") {
			sawGenerationProgress = true
		}
		if thought.Stage == "retrieving" {
			sawRetrievingStage = true
		}
		if thought.Stage == "generating" || thought.Stage == "finalizing" {
			sawGeneratingStage = true
		}
	}
	assert.True(t, sawRetrievalProgress)
	assert.True(t, sawGenerationProgress)
	assert.True(t, sawRetrievingStage)
	assert.True(t, sawGeneratingStage)
}

func TestRunKnowledgeGroundedDocumentContinuationPath_PersistsOutlineWhenLocalKnowledgeNotFound(t *testing.T) {
	svc := &sessionService{
		cfg: &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}},
		knowledgeBaseService: &fullDocumentKnowledgeSearchStub{
			kbs:     map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", Type: types.KnowledgeBaseTypeDocument}},
			results: map[string][]*types.SearchResult{},
		},
	}
	chatModel := &stagedFullDocumentChat{}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:                     "以当前文档为基准，继续剩余内容输出",
		Session:                   &types.Session{ID: "sess-grounded-continuation-no-evidence"},
		AssistantMessageID:        "msg-grounded-continuation-no-evidence",
		DocumentIntent:            types.ChatDocumentIntentContinue,
		DocumentOperation:         types.ChatDocumentOperationContinue,
		DocumentOutputMode:        types.ChatDocumentOutputModeDelta,
		BaseArtifactID:            "artifact-no-evidence-1",
		BaseArtifact:              &types.ChatDocumentArtifact{ID: "artifact-no-evidence-1", ContentSnapshot: "# 智慧运行方案\n\n## 第一章\n\n已有内容"},
		AutoContinue:              true,
		AutoContinuePrompt:        "以当前文档为基准，继续剩余内容输出",
		AutoContinueOriginalQuery: "请输出完整的智慧运行投标技术方案",
	}
	agentConfig := &types.AgentConfig{
		Temperature:    0.3,
		KnowledgeBases: []string{"kb-1"},
		SearchTargets:  types.SearchTargets{{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase}},
	}

	var completes []event.AgentCompleteData
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runKnowledgeGroundedDocumentContinuationPath(context.Background(), req, bus, chatModel, agentConfig)
	require.NoError(t, err)
	require.Len(t, completes, 1)
	assert.Equal(t, types.MessageCompletionStatusPartial, completes[0].CompletionStatus)
	assert.Equal(t, types.ChatDocumentGenerationStatusBlocked, completes[0].DocumentGenerationStatus)
	require.NotNil(t, completes[0].Extra)
	outlinePayload, ok := completes[0].Extra["outline"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "智慧运行方案", outlinePayload["title"])
	assert.Equal(t, "base_document", completes[0].Extra["outline_role"])
	assert.Equal(t, "local_knowledge_not_found", completes[0].FinishReason)
	assert.Equal(t, 0, chatModel.streamCalls)
}

func TestRunKnowledgeGroundedDocumentContinuationPath_AdjustsBudgetWithRuntimeFeedback(t *testing.T) {
	oldLimit := dedicatedFullDocumentSectionLimitPerRun
	dedicatedFullDocumentSectionLimitPerRun = 2
	defer func() {
		dedicatedFullDocumentSectionLimitPerRun = oldLimit
	}()

	svc := &sessionService{
		cfg: &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}},
		knowledgeBaseService: &fullDocumentKnowledgeSearchStub{
			kbs: map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", Type: types.KnowledgeBaseTypeDocument}},
			results: map[string][]*types.SearchResult{
				"kb-1|请输出完整的智慧运行投标技术方案": {
					{ID: "chunk-1", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "智慧运行总体方案", Content: "支持继续输出实施与保障章节。", Score: 0.9},
				},
				"kb-1|请输出完整的智慧运行投标技术方案 继续剩余内容": {
					{ID: "chunk-2", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-2", KnowledgeTitle: "实施保障", Content: "支持继续输出实施保障内容。", Score: 0.88},
				},
				"kb-1|智慧运行方案 第一章": {
					{ID: "chunk-3", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-3", KnowledgeTitle: "第一章补充", Content: "支持第一章后续增量。", Score: 0.8},
				},
			},
		},
	}
	chatModel := &stagedFullDocumentChat{
		sectionStreams: [][]types.StreamResponse{{
			{ResponseType: types.ResponseTypeAnswer, Content: "继续补充第二章实施与保障内容，但当前输出被长度限制截断。", Done: true, FinishReason: "length"},
		}},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:                     "以当前文档为基准，继续剩余内容输出",
		Session:                   &types.Session{ID: "sess-grounded-continuation-runtime-feedback"},
		AssistantMessageID:        "msg-grounded-continuation-runtime-feedback",
		DocumentIntent:            types.ChatDocumentIntentContinue,
		DocumentOperation:         types.ChatDocumentOperationContinue,
		DocumentOutputMode:        types.ChatDocumentOutputModeDelta,
		BaseArtifactID:            "artifact-runtime-feedback-1",
		BaseArtifact:              &types.ChatDocumentArtifact{ID: "artifact-runtime-feedback-1", ContentSnapshot: "# 智慧运行方案\n\n## 第一章\n\n已有内容"},
		AutoContinue:              true,
		AutoContinuePrompt:        "以当前文档为基准，继续剩余内容输出",
		AutoContinueOriginalQuery: "请输出完整的智慧运行投标技术方案",
	}
	agentConfig := &types.AgentConfig{
		Temperature:    0.3,
		KnowledgeBases: []string{"kb-1"},
		SearchTargets:  types.SearchTargets{{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase}},
	}

	var completes []event.AgentCompleteData
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runKnowledgeGroundedDocumentContinuationPath(context.Background(), req, bus, chatModel, agentConfig)
	require.NoError(t, err)
	require.Len(t, completes, 1)
	require.NotNil(t, completes[0].Extra)
	budgetPayload, ok := completes[0].Extra["budget"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "runtime_feedback", budgetPayload["source"])
	assert.Equal(t, 4608, budgetPayload["section_max_completion_tokens"])
	assert.Equal(t, 4608, budgetPayload["continuation_max_completion_tokens"])
	feedbackPayload, ok := completes[0].Extra["runtime_feedback"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, feedbackPayload["budget_adjusted"])
	assert.Equal(t, 1, feedbackPayload["length_stop_count"])
	assert.Equal(t, 1, feedbackPayload["recommended_section_limit_per_run"])
	var firstSection map[string]interface{}
	if sectionsPayload, ok := feedbackPayload["sections"].([]interface{}); ok {
		require.Len(t, sectionsPayload, 1)
		firstSection, ok = sectionsPayload[0].(map[string]interface{})
		require.True(t, ok)
	} else {
		sectionsPayload, ok := feedbackPayload["sections"].([]map[string]interface{})
		require.True(t, ok)
		require.Len(t, sectionsPayload, 1)
		firstSection = sectionsPayload[0]
	}
	assert.Equal(t, 3, firstSection["evidence_count"])
	assert.Equal(t, true, firstSection["budget_adjusted"])
	assert.Equal(t, "length", firstSection["finish_reason"])
	if adjustReasons, ok := firstSection["budget_adjust_reasons"].([]interface{}); ok {
		assert.NotEmpty(t, adjustReasons)
	} else {
		adjustReasons, ok := firstSection["budget_adjust_reasons"].([]string)
		require.True(t, ok)
		assert.NotEmpty(t, adjustReasons)
	}
}

func TestEffectiveFullDocumentSectionMaxCompletionTokens_AppliesMinimumBudget(t *testing.T) {
	assert.Equal(t, 4096, effectiveFullDocumentSectionMaxCompletionTokens(&config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}}))
	assert.Equal(t, 4096, effectiveFullDocumentSectionMaxCompletionTokens(&config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 4096}}}))
	assert.Equal(t, 4096, effectiveFullDocumentSectionMaxCompletionTokens(nil))
}

func TestFallbackDocumentGenerationBudget_UsesUnifiedFallbacks(t *testing.T) {
	budget := fallbackDocumentGenerationBudget(&config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}})
	assert.Equal(t, "fallback", budget.Source)
	assert.Equal(t, 1536, budget.OutlineMaxCompletionTokens)
	assert.Equal(t, 4096, budget.SectionMaxCompletionTokens)
	assert.Equal(t, 4096, budget.ContinuationMaxCompletionTokens)
	assert.Equal(t, 8, budget.OutlineEvidenceTopK)
	assert.Equal(t, 8, budget.SectionEvidenceTopK)
	assert.Equal(t, 8, budget.ContinuationEvidenceTopK)
	assert.Equal(t, documentGenerationDefaultLLMTimeoutSeconds, budget.SectionCallTimeoutSeconds)

	defaultBudget := fallbackDocumentGenerationBudget(nil)
	assert.Equal(t, 1536, defaultBudget.OutlineMaxCompletionTokens)
	assert.Equal(t, 4096, defaultBudget.SectionMaxCompletionTokens)
	assert.Equal(t, 4096, defaultBudget.ContinuationMaxCompletionTokens)
	assert.Equal(t, documentGenerationDefaultLLMTimeoutSeconds, defaultBudget.SectionCallTimeoutSeconds)
}

func TestApplyStaticModelCapabilityToBudget_ClampsAndExpands(t *testing.T) {
	fallback := fallbackDocumentGenerationBudget(&config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}})
	streaming := true
	thinkingControl := true

	largeModel := applyStaticModelCapabilityToBudget(fallback, &ModelCapability{
		ModelID:                 "model-large",
		Provider:                "openai",
		ContextWindowTokens:     131072,
		MaxOutputTokens:         8192,
		SupportsStreaming:       &streaming,
		SupportsThinkingControl: &thinkingControl,
	})
	assert.Equal(t, "capability", largeModel.Source)
	assert.Equal(t, "model-large", largeModel.ModelID)
	assert.Equal(t, "openai", largeModel.Provider)
	assert.Equal(t, 1536, largeModel.OutlineMaxCompletionTokens)
	assert.Equal(t, 4096, largeModel.SectionMaxCompletionTokens)
	assert.Equal(t, 4096, largeModel.ContinuationMaxCompletionTokens)
	assert.Equal(t, 8, largeModel.OutlineEvidenceTopK)
	assert.Equal(t, 8, largeModel.SectionEvidenceTopK)
	assert.Equal(t, 8, largeModel.ContinuationEvidenceTopK)
	assert.Equal(t, 131072, largeModel.ContextWindowTokens)
	assert.Equal(t, 8192, largeModel.MaxOutputTokens)
	require.NotNil(t, largeModel.SupportsStreaming)
	assert.True(t, *largeModel.SupportsStreaming)
	require.NotNil(t, largeModel.SupportsThinkingControl)
	assert.True(t, *largeModel.SupportsThinkingControl)

	smallModel := applyStaticModelCapabilityToBudget(fallback, &ModelCapability{
		ModelID:             "model-small",
		Provider:            "openai",
		ContextWindowTokens: 8192,
		MaxOutputTokens:     1536,
	})
	assert.Equal(t, "capability", smallModel.Source)
	assert.Equal(t, 1536, smallModel.OutlineMaxCompletionTokens)
	assert.Equal(t, 1536, smallModel.SectionMaxCompletionTokens)
	assert.Equal(t, 1536, smallModel.ContinuationMaxCompletionTokens)
	assert.Equal(t, 4, smallModel.OutlineEvidenceTopK)
	assert.Equal(t, 4, smallModel.SectionEvidenceTopK)
	assert.Equal(t, 4, smallModel.ContinuationEvidenceTopK)
}

func TestBuildModelCapability_ReturnsNilWithoutUsableCapabilityMetadata(t *testing.T) {
	capability := buildModelCapability(&types.Model{
		ID:     "model-without-capability",
		Source: types.ModelSourceOpenAI,
		Parameters: types.ModelParameters{
			Provider: "openai",
		},
	})
	assert.Nil(t, capability)
}

func TestBuildModelCapability_InfersDeepSeekDefaultsWhenMetadataMissing(t *testing.T) {
	capability := buildModelCapability(&types.Model{
		ID:     "deepseek-v4-pro-id",
		Name:   "deepseek-v4-pro",
		Source: types.ModelSourceRemote,
		Parameters: types.ModelParameters{
			Provider: "deepseek",
		},
	})

	require.NotNil(t, capability)
	assert.Equal(t, "deepseek-v4-pro-id", capability.ModelID)
	assert.Equal(t, "deepseek", capability.Provider)
	assert.Equal(t, 64000, capability.ContextWindowTokens)
	assert.Equal(t, 8192, capability.MaxOutputTokens)
	require.NotNil(t, capability.SupportsStreaming)
	assert.True(t, *capability.SupportsStreaming)
	assert.Equal(t, 180, capability.RecommendedTimeoutSec)
}

func TestBuildModelCapability_ReadsCamelCaseExtraConfigKeys(t *testing.T) {
	capability := buildModelCapability(&types.Model{
		ID:     "model-camel-extra-config",
		Source: types.ModelSourceOpenAI,
		Parameters: types.ModelParameters{
			ExtraConfig: map[string]string{
				"contextWindowTokens":     "65536",
				"maxOutputTokens":         "6144",
				"supportsStreaming":       "true",
				"supportsThinkingControl": "true",
				"defaultThinkingEnabled":  "false",
				"recommendedTimeoutSec":   "180",
			},
		},
	})
	require.NotNil(t, capability)
	assert.Equal(t, 65536, capability.ContextWindowTokens)
	assert.Equal(t, 6144, capability.MaxOutputTokens)
	require.NotNil(t, capability.SupportsStreaming)
	assert.True(t, *capability.SupportsStreaming)
	require.NotNil(t, capability.SupportsThinkingControl)
	assert.True(t, *capability.SupportsThinkingControl)
	require.NotNil(t, capability.DefaultThinkingEnabled)
	assert.False(t, *capability.DefaultThinkingEnabled)
	assert.Equal(t, 180, capability.RecommendedTimeoutSec)
}

func TestValidateAndClampNegotiatedDocumentBudget_ClampsUnsafeSuggestions(t *testing.T) {
	baseBudget := applyStaticModelCapabilityToBudget(fallbackDocumentGenerationBudget(&config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}}), &ModelCapability{
		ModelID:             "model-negotiated",
		Provider:            "openai",
		ContextWindowTokens: 65536,
		MaxOutputTokens:     6144,
	})

	resolved := validateAndClampNegotiatedDocumentBudget(baseBudget, documentBudgetNegotiationResponse{
		OutlineMaxCompletionTokens:      4096,
		SectionMaxCompletionTokens:      12000,
		ContinuationMaxCompletionTokens: 256,
		OutlineEvidenceTopK:             1,
		SectionEvidenceTopK:             50,
		ContinuationEvidenceTopK:        2,
		SectionCallTimeoutSeconds:       999,
		Reason:                          "aggressive suggestion",
	})

	assert.Equal(t, "negotiated", resolved.Source)
	assert.Equal(t, 2048, resolved.OutlineMaxCompletionTokens)
	assert.Equal(t, 6144, resolved.SectionMaxCompletionTokens)
	assert.Equal(t, 4096, resolved.ContinuationMaxCompletionTokens)
	assert.Equal(t, 4, resolved.OutlineEvidenceTopK)
	assert.Equal(t, 12, resolved.SectionEvidenceTopK)
	assert.Equal(t, 4, resolved.ContinuationEvidenceTopK)
	assert.Equal(t, 300, resolved.SectionCallTimeoutSeconds)
	assert.Equal(t, "aggressive suggestion", resolved.NegotiationReason)
}

func TestAdjustDocumentGenerationBudgetWithRuntimeFeedback_DoesNotLowerEvidenceForSlowFirstTokenOnly(t *testing.T) {
	baseBudget := fallbackDocumentGenerationBudget(&config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}})

	adjusted, reasons, recommendedSectionLimit := adjustDocumentGenerationBudgetWithRuntimeFeedback(baseBudget, documentGenerationRuntimeSectionFeedback{
		Section:             "详细设计",
		EvidenceCount:       baseBudget.SectionEvidenceTopK,
		FirstTokenLatencyMs: documentRuntimeSlowFirstTokenThresholdMs + 1000,
		CompletionStatus:    types.MessageCompletionStatusCompleted,
		FinishReason:        "stop",
		OutputTokenEstimate: 1800,
	})

	assert.Equal(t, baseBudget.SectionEvidenceTopK, adjusted.SectionEvidenceTopK)
	assert.Equal(t, baseBudget.ContinuationEvidenceTopK, adjusted.ContinuationEvidenceTopK)
	assert.Empty(t, reasons)
	assert.Equal(t, 0, recommendedSectionLimit)
}

func TestConsumeFullDocumentSectionStream_TreatsDeadlineExceededAsTimeout(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now())
	defer cancel()

	result := consumeFullDocumentSectionStream(ctx, make(chan types.StreamResponse), nil, "测试章节", nil, func(string) {})

	assert.Equal(t, types.MessageCompletionStatusPartial, result.completionStatus)
	assert.Equal(t, "section_generation_timeout", result.finishReason)
	assert.Equal(t, "llm_timeout", result.failureReason)
	assert.Equal(t, types.ChatDocumentGenerationStatusContinuing, result.documentGenerationState)
}

func TestChatFullDocumentOutlineWithProgress_EmitsThinkingAndClosesOnAnswer(t *testing.T) {
	chatModel := &stagedFullDocumentChat{
		outlineStream: []types.StreamResponse{
			{ResponseType: types.ResponseTypeThinking, Content: "先分析用户需求并规划章节。"},
			{ResponseType: types.ResponseTypeAnswer, Content: "{\"title\":\"智慧运行建设方案\",\"sections\":[{\"number\":1,\"title\":\"建设目标\",\"heading\":\"第1章 建设目标\",\"subsections\":[{\"number\":\"1.1\",\"title\":\"总体目标\"}]}]}", Done: true, FinishReason: "stop"},
		},
	}
	var thinkingContents []string
	var thinkingDone []bool

	response, err := chatFullDocumentOutlineWithProgress(context.Background(), chatModel, nil, &chat.ChatOptions{}, nil, "正在规划完整文档大纲", func(content string, done bool) {
		thinkingContents = append(thinkingContents, content)
		thinkingDone = append(thinkingDone, done)
	})

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "{\"title\":\"智慧运行建设方案\",\"sections\":[{\"number\":1,\"title\":\"建设目标\",\"heading\":\"第1章 建设目标\",\"subsections\":[{\"number\":\"1.1\",\"title\":\"总体目标\"}]}]}", response.Content)
	assert.Equal(t, []string{"先分析用户需求并规划章节。", ""}, thinkingContents)
	assert.Equal(t, []bool{false, true}, thinkingDone)
}

func TestConsumeFullDocumentSectionStream_EmitsThinkingAndClosesOnAnswer(t *testing.T) {
	sectionStream := make(chan types.StreamResponse, 2)
	sectionStream <- types.StreamResponse{ResponseType: types.ResponseTypeThinking, Content: "先分析本章约束并规划输出。"}
	sectionStream <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: "### 1.1 总体目标\n正文内容", Done: true, FinishReason: "stop"}
	close(sectionStream)

	var thinkingContents []string
	var thinkingDone []bool
	var answerChunks []string

	result := consumeFullDocumentSectionStream(context.Background(), sectionStream, nil, "测试章节", func(content string, done bool) {
		thinkingContents = append(thinkingContents, content)
		thinkingDone = append(thinkingDone, done)
	}, func(content string) {
		answerChunks = append(answerChunks, content)
	})

	assert.Equal(t, types.MessageCompletionStatusCompleted, result.completionStatus)
	assert.Equal(t, "stop", result.finishReason)
	assert.True(t, result.sectionDone)
	assert.Equal(t, []string{"先分析本章约束并规划输出。", ""}, thinkingContents)
	assert.Equal(t, []bool{false, true}, thinkingDone)
	assert.Equal(t, []string{"### 1.1 总体目标\n正文内容"}, answerChunks)
}

func TestResolveDocumentGenerationCallTimeoutObservability_UsesGlobalAgentFallbackSource(t *testing.T) {
	resolution := resolveDocumentGenerationCallTimeoutObservability(
		&types.QARequest{CustomAgent: &types.CustomAgent{}},
		&types.AgentConfig{LLMCallTimeout: 120},
		120,
		DocumentGenerationBudget{SectionCallTimeoutSeconds: 180},
	)

	assert.Equal(t, 180, resolution.BudgetSectionTimeoutSeconds)
	assert.Equal(t, 0, resolution.AgentLLMCallTimeoutSeconds)
	assert.Equal(t, 120, resolution.GlobalAgentLLMCallTimeoutSeconds)
	assert.Equal(t, 180, resolution.EffectiveSectionTimeoutSeconds)
	assert.Equal(t, "budget", resolution.EffectiveTimeoutSource)
}

func TestResolveDocumentGenerationCallTimeoutObservability_UsesAgentSpecificSource(t *testing.T) {
	resolution := resolveDocumentGenerationCallTimeoutObservability(
		&types.QARequest{CustomAgent: &types.CustomAgent{Config: types.CustomAgentConfig{LLMCallTimeout: 240}}},
		&types.AgentConfig{LLMCallTimeout: 240},
		120,
		DocumentGenerationBudget{SectionCallTimeoutSeconds: 180},
	)

	assert.Equal(t, 180, resolution.BudgetSectionTimeoutSeconds)
	assert.Equal(t, 240, resolution.AgentLLMCallTimeoutSeconds)
	assert.Equal(t, 120, resolution.GlobalAgentLLMCallTimeoutSeconds)
	assert.Equal(t, 240, resolution.EffectiveSectionTimeoutSeconds)
	assert.Equal(t, "agent_config", resolution.EffectiveTimeoutSource)
}

func TestResolveDocumentGenerationCallTimeoutSeconds_PrefersBudgetOverLowerAgentTimeout(t *testing.T) {
	assert.Equal(t, 180, resolveDocumentGenerationCallTimeoutSeconds(DocumentGenerationBudget{SectionCallTimeoutSeconds: 180}, 120))
	assert.Equal(t, 240, resolveDocumentGenerationCallTimeoutSeconds(DocumentGenerationBudget{SectionCallTimeoutSeconds: 180}, 240))
	assert.Equal(t, documentGenerationDefaultLLMTimeoutSeconds, resolveDocumentGenerationCallTimeoutSeconds(DocumentGenerationBudget{}, 120))
}

func TestResolveDocumentGenerationBudget_FallsBackToCapabilityBudgetWhenNegotiationJSONInvalid(t *testing.T) {
	streaming := true
	svc := &sessionService{
		cfg: &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}},
		modelService: &fullDocumentModelServiceStub{model: &types.Model{
			ID:     "staged-full-document-chat",
			Source: types.ModelSourceOpenAI,
			Parameters: types.ModelParameters{
				ContextWindowTokens: 65536,
				MaxOutputTokens:     8192,
				SupportsStreaming:   &streaming,
			},
		}},
	}
	chatModel := &stagedFullDocumentChat{
		negotiationResponse: &types.ChatResponse{Content: "not-json"},
	}
	budget := svc.resolveDocumentGenerationBudget(context.Background(), &types.QARequest{DocumentOutputMode: types.ChatDocumentOutputModeFull, Query: "请输出完整的技术方案"}, chatModel, buildDocumentProfile(&types.QARequest{DocumentOutputMode: types.ChatDocumentOutputModeFull, Query: "请输出完整的技术方案"}, &types.AgentConfig{}, true, 6), nil)
	assert.Equal(t, "capability", budget.Source)
	assert.Equal(t, 4096, budget.SectionMaxCompletionTokens)
	assert.Equal(t, 8, budget.SectionEvidenceTopK)
	assert.Equal(t, 0, chatModel.streamCalls)
	assert.Equal(t, 1, chatModel.chatCalls)
}

func TestRunKnowledgeGroundedFullDocumentGenerationPath_UsesNegotiatedBudgetWhenModelReturnsValidJSON(t *testing.T) {
	streaming := true
	thinkingControl := true
	searchStub := &fullDocumentKnowledgeSearchStub{
		kbs: map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", Type: types.KnowledgeBaseTypeDocument}},
		results: map[string][]*types.SearchResult{
			"kb-1|请输出完整的智慧运行建设方案": {
				{ID: "outline-1", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-outline", KnowledgeTitle: "智慧运行总体方案", Content: "包含建设目标。", Score: 0.92},
			},
			"kb-1|请输出完整的智慧运行建设方案 建设目标": {
				{ID: "sec-1", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "建设目标", Content: "建设目标章节依据。", Score: 0.91},
			},
			"kb-1|智慧运行建设方案 建设目标": {
				{ID: "sec-1b", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "建设目标", Content: "建设目标补充依据。", Score: 0.88},
			},
			"kb-1|请检索与当前章节直接相关的本地事实和能力说明：建设目标": {
				{ID: "sec-1c", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "建设目标", Content: "建设目标事实。", Score: 0.87},
			},
		},
	}
	svc := &sessionService{
		cfg:                  &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}},
		knowledgeBaseService: searchStub,
		modelService: &fullDocumentModelServiceStub{model: &types.Model{
			ID:     "staged-full-document-chat",
			Source: types.ModelSourceOpenAI,
			Parameters: types.ModelParameters{
				ContextWindowTokens:     131072,
				MaxOutputTokens:         8192,
				SupportsStreaming:       &streaming,
				SupportsThinkingControl: &thinkingControl,
			},
		}},
	}
	chatModel := &stagedFullDocumentChat{
		negotiationResponse: &types.ChatResponse{Content: `{"outline_max_completion_tokens":1536,"section_max_completion_tokens":5120,"continuation_max_completion_tokens":5120,"outline_evidence_top_k":7,"section_evidence_top_k":9,"continuation_evidence_top_k":8,"section_call_timeout_seconds":180,"reason":"technical proposal with richer evidence"}`},
		outlineResponse:     types.ChatResponse{Content: "# 智慧运行建设方案\n\n## 建设目标\n"},
		sectionStreams: [][]types.StreamResponse{{
			{ResponseType: types.ResponseTypeAnswer, Content: "说明建设目标与业务价值。", Done: true, FinishReason: "stop"},
		}},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:              "请输出完整的智慧运行建设方案",
		Session:            &types.Session{ID: "sess-grounded-negotiated"},
		AssistantMessageID: "msg-grounded-negotiated",
		DocumentOutputMode: types.ChatDocumentOutputModeFull,
	}
	agentConfig := &types.AgentConfig{
		Temperature:    0.3,
		KnowledgeBases: []string{"kb-1"},
		SearchTargets:  types.SearchTargets{{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase}},
	}

	var completes []event.AgentCompleteData
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runKnowledgeGroundedFullDocumentGenerationPath(context.Background(), req, bus, chatModel, agentConfig)
	require.NoError(t, err)
	require.Len(t, completes, 1)
	require.Len(t, chatModel.streamOptions, 2)
	assert.Equal(t, 1536, chatModel.streamOptions[0].MaxCompletionTokens)
	assert.Zero(t, chatModel.streamOptions[1].MaxCompletionTokens)
	require.Len(t, chatModel.streamHasDeadline, 2)
	assert.False(t, chatModel.streamHasDeadline[0])
	assert.True(t, chatModel.streamHasDeadline[1])
	assert.Greater(t, chatModel.streamTimeouts[1], 170*time.Second)
	assert.LessOrEqual(t, chatModel.streamTimeouts[1], 180*time.Second)
	require.NotEmpty(t, searchStub.params)
	outlineQueryCount := len(buildKnowledgeGroundedOutlineQueries(req))
	require.Greater(t, outlineQueryCount, 0)
	for _, params := range searchStub.params[:outlineQueryCount] {
		assert.Equal(t, 7, params.MatchCount)
	}
	for _, params := range searchStub.params[outlineQueryCount:] {
		assert.Equal(t, 9, params.MatchCount)
	}
	require.NotNil(t, completes[0].Extra)
	budgetPayload, ok := completes[0].Extra["budget"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "negotiated", budgetPayload["source"])
	assert.Equal(t, 1536, budgetPayload["outline_max_completion_tokens"])
	assert.Equal(t, 5120, budgetPayload["section_max_completion_tokens"])
	assert.Equal(t, 5120, budgetPayload["continuation_max_completion_tokens"])
	assert.Equal(t, 7, budgetPayload["outline_evidence_top_k"])
	assert.Equal(t, 9, budgetPayload["section_evidence_top_k"])
	assert.Equal(t, 8, budgetPayload["continuation_evidence_top_k"])
	assert.Equal(t, 180, budgetPayload["section_call_timeout_seconds"])
	assert.Equal(t, "technical proposal with richer evidence", budgetPayload["negotiation_reason"])
	assert.Equal(t, 1, chatModel.chatCalls)
}

func TestRunKnowledgeGroundedFullDocumentGenerationPath_UsesStaticModelCapabilityBudget(t *testing.T) {
	streaming := true
	thinkingControl := true
	searchStub := &fullDocumentKnowledgeSearchStub{
		kbs: map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", Type: types.KnowledgeBaseTypeDocument}},
		results: map[string][]*types.SearchResult{
			"kb-1|请输出完整的智慧运行建设方案": {
				{ID: "outline-1", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-outline", KnowledgeTitle: "智慧运行总体方案", Content: "包含建设目标。", Score: 0.92},
			},
			"kb-1|请输出完整的智慧运行建设方案 建设目标": {
				{ID: "sec-1", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "建设目标", Content: "建设目标章节依据。", Score: 0.91},
			},
			"kb-1|智慧运行建设方案 建设目标": {
				{ID: "sec-1b", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "建设目标", Content: "建设目标补充依据。", Score: 0.88},
			},
			"kb-1|请检索与当前章节直接相关的本地事实和能力说明：建设目标": {
				{ID: "sec-1c", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "建设目标", Content: "建设目标事实。", Score: 0.87},
			},
		},
	}
	svc := &sessionService{
		cfg:                  &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}},
		knowledgeBaseService: searchStub,
		modelService: &fullDocumentModelServiceStub{model: &types.Model{
			ID:     "staged-full-document-chat",
			Source: types.ModelSourceOpenAI,
			Parameters: types.ModelParameters{
				ContextWindowTokens:     131072,
				MaxOutputTokens:         8192,
				SupportsStreaming:       &streaming,
				SupportsThinkingControl: &thinkingControl,
			},
		}},
	}
	chatModel := &stagedFullDocumentChat{
		outlineResponse: types.ChatResponse{Content: "# 智慧运行建设方案\n\n## 建设目标\n"},
		sectionStreams: [][]types.StreamResponse{{
			{ResponseType: types.ResponseTypeAnswer, Content: "说明建设目标与业务价值。", Done: true, FinishReason: "stop"},
		}},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:              "请输出完整的智慧运行建设方案",
		Session:            &types.Session{ID: "sess-grounded-capability"},
		AssistantMessageID: "msg-grounded-capability",
		DocumentOutputMode: types.ChatDocumentOutputModeFull,
	}
	agentConfig := &types.AgentConfig{
		Temperature:    0.3,
		KnowledgeBases: []string{"kb-1"},
		SearchTargets:  types.SearchTargets{{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase}},
	}

	var completes []event.AgentCompleteData
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runKnowledgeGroundedFullDocumentGenerationPath(context.Background(), req, bus, chatModel, agentConfig)
	require.NoError(t, err)
	require.Len(t, completes, 1)
	require.Len(t, chatModel.streamOptions, 2)
	assert.Equal(t, 1536, chatModel.streamOptions[0].MaxCompletionTokens)
	assert.Zero(t, chatModel.streamOptions[1].MaxCompletionTokens)
	require.NotEmpty(t, searchStub.params)
	for _, params := range searchStub.params {
		assert.Equal(t, 8, params.MatchCount)
	}
	require.NotNil(t, completes[0].Extra)
	budgetPayload, ok := completes[0].Extra["budget"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "capability", budgetPayload["source"])
	assert.Equal(t, "staged-full-document-chat", budgetPayload["model_id"])
	assert.Equal(t, "openai", budgetPayload["provider"])
	assert.Equal(t, 131072, budgetPayload["context_window_tokens"])
	assert.Equal(t, 8192, budgetPayload["max_output_tokens"])
	assert.Equal(t, 8, budgetPayload["section_evidence_top_k"])
	assert.Equal(t, 4096, budgetPayload["section_max_completion_tokens"])
	assert.Equal(t, true, budgetPayload["supports_streaming"])
	assert.Equal(t, true, budgetPayload["supports_thinking_control"])
}

func TestRunKnowledgeGroundedFullDocumentGenerationPath_DoesNotCapSectionTimeoutBelowBudgetWhenGlobalAgentTimeoutIsLower(t *testing.T) {
	streaming := true
	thinkingControl := true
	searchStub := &fullDocumentKnowledgeSearchStub{
		kbs: map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", Type: types.KnowledgeBaseTypeDocument}},
		results: map[string][]*types.SearchResult{
			"kb-1|请输出完整的智慧运行建设方案": {
				{ID: "outline-1", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-outline", KnowledgeTitle: "智慧运行总体方案", Content: "包含建设目标。", Score: 0.92},
			},
			"kb-1|请输出完整的智慧运行建设方案 建设目标": {
				{ID: "sec-1", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "建设目标", Content: "建设目标章节依据。", Score: 0.91},
			},
			"kb-1|智慧运行建设方案 建设目标": {
				{ID: "sec-1b", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "建设目标", Content: "建设目标补充依据。", Score: 0.88},
			},
			"kb-1|请检索与当前章节直接相关的本地事实和能力说明：建设目标": {
				{ID: "sec-1c", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "建设目标", Content: "建设目标事实。", Score: 0.87},
			},
		},
	}
	svc := &sessionService{
		cfg: &config.Config{
			Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}},
			Agent:        &config.AgentConfig{LLMCallTimeout: 120},
		},
		knowledgeBaseService: searchStub,
		modelService: &fullDocumentModelServiceStub{model: &types.Model{
			ID:     "staged-full-document-chat",
			Source: types.ModelSourceOpenAI,
			Parameters: types.ModelParameters{
				ContextWindowTokens:     131072,
				MaxOutputTokens:         8192,
				SupportsStreaming:       &streaming,
				SupportsThinkingControl: &thinkingControl,
			},
		}},
	}
	chatModel := &stagedFullDocumentChat{
		negotiationResponse: &types.ChatResponse{Content: `{"outline_max_completion_tokens":1536,"section_max_completion_tokens":5120,"continuation_max_completion_tokens":5120,"outline_evidence_top_k":7,"section_evidence_top_k":9,"continuation_evidence_top_k":8,"section_call_timeout_seconds":180,"reason":"technical proposal with richer evidence"}`},
		outlineResponse:     types.ChatResponse{Content: "# 智慧运行建设方案\n\n## 建设目标\n"},
		sectionStreams: [][]types.StreamResponse{{
			{ResponseType: types.ResponseTypeAnswer, Content: "说明建设目标与业务价值。", Done: true, FinishReason: "stop"},
		}},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:              "请输出完整的智慧运行建设方案",
		Session:            &types.Session{ID: "sess-grounded-global-timeout-floor"},
		AssistantMessageID: "msg-grounded-global-timeout-floor",
		DocumentOutputMode: types.ChatDocumentOutputModeFull,
	}
	agentConfig := &types.AgentConfig{
		Temperature:    0.3,
		KnowledgeBases: []string{"kb-1"},
		SearchTargets:  types.SearchTargets{{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase}},
		LLMCallTimeout: 120,
	}

	err := svc.runKnowledgeGroundedFullDocumentGenerationPath(context.Background(), req, bus, chatModel, agentConfig)
	require.NoError(t, err)
	require.Len(t, chatModel.streamHasDeadline, 2)
	assert.True(t, chatModel.streamHasDeadline[1])
	assert.Greater(t, chatModel.streamTimeouts[1], 170*time.Second)
	assert.LessOrEqual(t, chatModel.streamTimeouts[1], 180*time.Second)
}

func TestRunKnowledgeGroundedDocumentContinuationPath_UsesGenerationRunProgress(t *testing.T) {
	oldLimit := dedicatedFullDocumentSectionLimitPerRun
	dedicatedFullDocumentSectionLimitPerRun = 2
	defer func() {
		dedicatedFullDocumentSectionLimitPerRun = oldLimit
	}()

	run := &types.ChatDocumentGenerationRun{
		ID:                    "run-1",
		TenantID:              0,
		SessionID:             "sess-grounded-run-continuation",
		OriginalQuery:         "请输出完整的智慧运行建设方案",
		DocumentTitle:         "智慧运行建设方案",
		OutlineJSON:           marshalGenerationRunJSON(newDedicatedFullDocumentOutlineFromStrings("智慧运行建设方案", []string{"建设目标", "平台架构", "实施保障"})),
		EffectiveKBIDsJSON:    marshalGenerationRunJSON([]string{"kb-1"}),
		CompletedSectionsJSON: marshalGenerationRunJSON([]string{"建设目标"}),
		Status:                types.ChatDocumentGenerationRunStatusContinuing,
		AutoContinueRound:     1,
	}
	runRepo := &fullDocumentGenerationRunRepoStub{runs: map[string]*types.ChatDocumentGenerationRun{"run-1": cloneChatDocumentGenerationRun(run)}}
	svc := &sessionService{
		cfg: &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}},
		knowledgeBaseService: &fullDocumentKnowledgeSearchStub{
			kbs: map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", Type: types.KnowledgeBaseTypeDocument}},
			results: map[string][]*types.SearchResult{
				"kb-1|请输出完整的智慧运行建设方案 平台架构": {
					{ID: "sec-2", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-2", KnowledgeTitle: "平台架构", Content: "平台架构章节依据。", Score: 0.9},
				},
				"kb-1|智慧运行建设方案 平台架构": {
					{ID: "sec-2b", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-2", KnowledgeTitle: "平台架构", Content: "平台架构补充依据。", Score: 0.88},
				},
				"kb-1|请检索与当前章节直接相关的本地事实和能力说明：平台架构": {
					{ID: "sec-2c", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-2", KnowledgeTitle: "平台架构", Content: "平台架构事实。", Score: 0.86},
				},
				"kb-1|请输出完整的智慧运行建设方案 实施保障": {
					{ID: "sec-3", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-3", KnowledgeTitle: "实施保障", Content: "实施保障章节依据。", Score: 0.91},
				},
				"kb-1|智慧运行建设方案 实施保障": {
					{ID: "sec-3b", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-3", KnowledgeTitle: "实施保障", Content: "实施保障补充依据。", Score: 0.87},
				},
				"kb-1|请检索与当前章节直接相关的本地事实和能力说明：实施保障": {
					{ID: "sec-3c", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-3", KnowledgeTitle: "实施保障", Content: "实施保障事实。", Score: 0.85},
				},
			},
		},
		generationRunRepo: runRepo,
	}
	chatModel := &stagedFullDocumentChat{
		sectionStreams: [][]types.StreamResponse{
			{{ResponseType: types.ResponseTypeAnswer, Content: "说明平台架构与能力分层。", Done: true, FinishReason: "stop"}},
			{{ResponseType: types.ResponseTypeAnswer, Content: "说明实施保障与交付机制。", Done: true, FinishReason: "stop"}},
		},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:                     "以当前文档为基准，继续剩余内容输出",
		Session:                   &types.Session{ID: "sess-grounded-run-continuation"},
		AssistantMessageID:        "msg-grounded-run-continuation",
		DocumentIntent:            types.ChatDocumentIntentContinue,
		DocumentOperation:         types.ChatDocumentOperationContinue,
		DocumentOutputMode:        types.ChatDocumentOutputModeDelta,
		BaseArtifactID:            "artifact-1",
		BaseArtifact:              &types.ChatDocumentArtifact{ID: "artifact-1", ContentSnapshot: "# 智慧运行建设方案\n\n## 建设目标\n\n说明建设目标与业务价值。\n\n## 第2章 平台架构\n\n### 2.1 全域数据湖建设\n\n当前已生成的平台架构片段，需要延续接口边界与依赖说明。\n历史正文尾部标记-不应完整透传"},
		AutoContinue:              true,
		GenerationRunID:           "run-1",
		AutoContinueRound:         1,
		AutoContinuePrompt:        "以当前文档为基准，继续剩余内容输出",
		AutoContinueOriginalQuery: "请输出完整的智慧运行建设方案",
	}
	agentConfig := &types.AgentConfig{
		Temperature:    0.3,
		KnowledgeBases: []string{"kb-1"},
		SearchTargets:  types.SearchTargets{{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase}},
	}

	var completes []event.AgentCompleteData
	var thoughts []event.AgentThoughtData
	bus.On(event.EventAgentThought, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentThoughtData)
		require.True(t, ok)
		thoughts = append(thoughts, data)
		return nil
	})
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runKnowledgeGroundedDocumentContinuationPath(context.Background(), req, bus, chatModel, agentConfig)
	require.NoError(t, err)
	require.Len(t, completes, 1)
	require.NotNil(t, runRepo.updated)
	assert.Equal(t, types.ChatDocumentGenerationRunStatusCompleted, runRepo.updated.Status)
	assert.Equal(t, 2, runRepo.updated.AutoContinueRound)
	assert.Equal(t, []string{"建设目标", "平台架构", "实施保障"}, unmarshalGenerationRunStringSlice(runRepo.updated.CompletedSectionsJSON))
	assert.Equal(t, types.MessageCompletionStatusCompleted, completes[0].CompletionStatus)
	assert.Contains(t, completes[0].FinalAnswer, "## 第2章 平台架构")
	assert.Contains(t, completes[0].FinalAnswer, "## 第3章 实施保障")
	require.NotNil(t, completes[0].Extra)
	assert.Equal(t, "run-1", completes[0].Extra["generation_run_id"])
	budgetPayload, ok := completes[0].Extra["budget"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 4096, budgetPayload["continuation_max_completion_tokens"])
	assert.Equal(t, 8, budgetPayload["section_evidence_top_k"])
	var sawSectionRetrievalProgress bool
	var sawSectionGenerationProgress bool
	for _, thought := range thoughts {
		if strings.Contains(thought.Content, "正在检索第 2/3 章“平台架构”的本地证据") {
			sawSectionRetrievalProgress = true
		}
		if strings.Contains(thought.Content, "已检索到 3 条证据，正在生成第 3/3 章“实施保障”") {
			sawSectionGenerationProgress = true
		}
	}
	assert.True(t, sawSectionRetrievalProgress)
	assert.True(t, sawSectionGenerationProgress)
	require.Len(t, chatModel.streamOptions, 2)
	for _, options := range chatModel.streamOptions {
		require.NotNil(t, options.Thinking)
		assert.False(t, *options.Thinking)
		assert.Zero(t, options.MaxCompletionTokens)
	}
	require.Len(t, chatModel.streamMessages, 2)
	assert.Contains(t, chatModel.streamMessages[0][1].Content, "Current unfinished section snapshot")
	assert.Contains(t, chatModel.streamMessages[0][1].Content, "## 第2章 平台架构")
	assert.Contains(t, chatModel.streamMessages[0][1].Content, "当前已生成的平台架构片段，需要延续接口边界与依赖说明")
	assert.NotContains(t, chatModel.streamMessages[0][1].Content, "历史正文尾部标记-不应完整透传")
}

func TestRunKnowledgeGroundedDocumentContinuationPath_UsesPersistedRuntimeFeedbackBudget(t *testing.T) {
	oldLimit := dedicatedFullDocumentSectionLimitPerRun
	dedicatedFullDocumentSectionLimitPerRun = 2
	defer func() {
		dedicatedFullDocumentSectionLimitPerRun = oldLimit
	}()

	run := &types.ChatDocumentGenerationRun{
		ID:                    "run-feedback-1",
		TenantID:              0,
		SessionID:             "sess-grounded-feedback-continuation",
		OriginalQuery:         "请输出完整的智慧运行建设方案",
		DocumentTitle:         "智慧运行建设方案",
		OutlineJSON:           marshalGenerationRunJSON(newDedicatedFullDocumentOutlineFromStrings("智慧运行建设方案", []string{"建设目标", "平台架构", "实施保障"})),
		BudgetJSON:            marshalGenerationRunJSON(DocumentGenerationBudget{Source: "runtime_feedback", OutlineMaxCompletionTokens: 1536, SectionMaxCompletionTokens: 4608, ContinuationMaxCompletionTokens: 4608, OutlineEvidenceTopK: 8, SectionEvidenceTopK: 9, ContinuationEvidenceTopK: 9, SectionCallTimeoutSeconds: 150}),
		RuntimeFeedbackJSON:   marshalGenerationRunJSON(documentGenerationRuntimeFeedback{BudgetAdjusted: true, RecommendedSectionLimitPerRun: 1, AdjustmentReasons: []string{"section_tokens_up_length", "section_batch_limit_down_length"}}),
		EffectiveKBIDsJSON:    marshalGenerationRunJSON([]string{"kb-1"}),
		CompletedSectionsJSON: marshalGenerationRunJSON([]string{"建设目标"}),
		Status:                types.ChatDocumentGenerationRunStatusContinuing,
		AutoContinueRound:     1,
	}
	runRepo := &fullDocumentGenerationRunRepoStub{runs: map[string]*types.ChatDocumentGenerationRun{"run-feedback-1": cloneChatDocumentGenerationRun(run)}}
	svc := &sessionService{
		cfg: &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}},
		knowledgeBaseService: &fullDocumentKnowledgeSearchStub{
			kbs: map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", Type: types.KnowledgeBaseTypeDocument}},
			results: map[string][]*types.SearchResult{
				"kb-1|请输出完整的智慧运行建设方案 平台架构": {
					{ID: "sec-2", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-2", KnowledgeTitle: "平台架构", Content: "平台架构章节依据。", Score: 0.9},
				},
				"kb-1|智慧运行建设方案 平台架构": {
					{ID: "sec-2b", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-2", KnowledgeTitle: "平台架构", Content: "平台架构补充依据。", Score: 0.88},
				},
				"kb-1|请检索与当前章节直接相关的本地事实和能力说明：平台架构": {
					{ID: "sec-2c", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-2", KnowledgeTitle: "平台架构", Content: "平台架构事实。", Score: 0.86},
				},
				"kb-1|请输出完整的智慧运行建设方案 实施保障": {
					{ID: "sec-3", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-3", KnowledgeTitle: "实施保障", Content: "实施保障章节依据。", Score: 0.91},
				},
				"kb-1|智慧运行建设方案 实施保障": {
					{ID: "sec-3b", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-3", KnowledgeTitle: "实施保障", Content: "实施保障补充依据。", Score: 0.87},
				},
				"kb-1|请检索与当前章节直接相关的本地事实和能力说明：实施保障": {
					{ID: "sec-3c", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-3", KnowledgeTitle: "实施保障", Content: "实施保障事实。", Score: 0.85},
				},
			},
		},
		generationRunRepo: runRepo,
	}
	chatModel := &stagedFullDocumentChat{
		sectionStreams: [][]types.StreamResponse{
			{{ResponseType: types.ResponseTypeAnswer, Content: "说明平台架构与能力分层。", Done: true, FinishReason: "stop"}},
			{{ResponseType: types.ResponseTypeAnswer, Content: "说明实施保障与交付机制。", Done: true, FinishReason: "stop"}},
		},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:                     "以当前文档为基准，继续剩余内容输出",
		Session:                   &types.Session{ID: "sess-grounded-feedback-continuation"},
		AssistantMessageID:        "msg-grounded-feedback-continuation",
		DocumentIntent:            types.ChatDocumentIntentContinue,
		DocumentOperation:         types.ChatDocumentOperationContinue,
		DocumentOutputMode:        types.ChatDocumentOutputModeDelta,
		BaseArtifactID:            "artifact-feedback-1",
		BaseArtifact:              &types.ChatDocumentArtifact{ID: "artifact-feedback-1", ContentSnapshot: "# 智慧运行建设方案\n\n## 建设目标\n\n说明建设目标与业务价值。"},
		AutoContinue:              true,
		GenerationRunID:           "run-feedback-1",
		AutoContinueRound:         1,
		AutoContinuePrompt:        "以当前文档为基准，继续剩余内容输出",
		AutoContinueOriginalQuery: "请输出完整的智慧运行建设方案",
	}
	agentConfig := &types.AgentConfig{
		Temperature:    0.3,
		KnowledgeBases: []string{"kb-1"},
		SearchTargets:  types.SearchTargets{{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase}},
	}

	var completes []event.AgentCompleteData
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runKnowledgeGroundedDocumentContinuationPath(context.Background(), req, bus, chatModel, agentConfig)
	require.NoError(t, err)
	require.Len(t, completes, 1)
	require.NotNil(t, runRepo.updated)
	assert.Equal(t, types.ChatDocumentGenerationRunStatusContinuing, runRepo.updated.Status)
	assert.Equal(t, 2, runRepo.updated.AutoContinueRound)
	assert.Equal(t, []string{"建设目标", "平台架构"}, unmarshalGenerationRunStringSlice(runRepo.updated.CompletedSectionsJSON))
	require.Len(t, chatModel.streamOptions, 1)
	assert.Zero(t, chatModel.streamOptions[0].MaxCompletionTokens)
	require.NotEmpty(t, chatModel.streamHasDeadline)
	assert.True(t, chatModel.streamHasDeadline[0])
	for _, params := range svc.knowledgeBaseService.(*fullDocumentKnowledgeSearchStub).params {
		assert.Equal(t, 9, params.MatchCount)
	}
	assert.Equal(t, types.MessageCompletionStatusPartial, completes[0].CompletionStatus)
	assert.Equal(t, "section_batch_limit", completes[0].FinishReason)
	assert.Equal(t, types.ChatDocumentGenerationStatusContinuing, completes[0].DocumentGenerationStatus)
	require.NotNil(t, completes[0].AutoContinueNext)
	assert.True(t, *completes[0].AutoContinueNext)
	assert.Contains(t, completes[0].FinalAnswer, "## 第2章 平台架构")
	assert.NotContains(t, completes[0].FinalAnswer, "## 第3章 实施保障")
	budgetPayload, ok := completes[0].Extra["budget"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "runtime_feedback", budgetPayload["source"])
	assert.Equal(t, 4608, budgetPayload["continuation_max_completion_tokens"])
	assert.Equal(t, 9, budgetPayload["section_evidence_top_k"])
	feedbackPayload, ok := completes[0].Extra["runtime_feedback"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 1, feedbackPayload["recommended_section_limit_per_run"])
}

func TestRunKnowledgeGroundedFullDocumentGenerationPath_AutoContinuesAfterRecoverableLLMTimeout(t *testing.T) {
	streaming := true
	thinkingControl := true
	runRepo := &fullDocumentGenerationRunRepoStub{}
	searchStub := &fullDocumentKnowledgeSearchStub{
		kbs: map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", Type: types.KnowledgeBaseTypeDocument}},
		results: map[string][]*types.SearchResult{
			"kb-1|请输出完整的智慧运行建设方案": {
				{ID: "outline-1", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-outline", KnowledgeTitle: "智慧运行总体方案", Content: "包含建设目标。", Score: 0.92},
			},
			"kb-1|请输出完整的智慧运行建设方案 建设目标": {
				{ID: "sec-1", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "建设目标", Content: "建设目标章节依据。", Score: 0.91},
			},
			"kb-1|智慧运行建设方案 建设目标": {
				{ID: "sec-1b", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "建设目标", Content: "建设目标补充依据。", Score: 0.88},
			},
			"kb-1|请检索与当前章节直接相关的本地事实和能力说明：建设目标": {
				{ID: "sec-1c", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-1", KnowledgeTitle: "建设目标", Content: "建设目标事实。", Score: 0.87},
			},
		},
	}
	svc := &sessionService{
		cfg:                  &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}},
		knowledgeBaseService: searchStub,
		generationRunRepo:    runRepo,
		modelService: &fullDocumentModelServiceStub{model: &types.Model{
			ID:     "staged-full-document-chat",
			Source: types.ModelSourceOpenAI,
			Parameters: types.ModelParameters{
				ContextWindowTokens:     131072,
				MaxOutputTokens:         8192,
				SupportsStreaming:       &streaming,
				SupportsThinkingControl: &thinkingControl,
			},
		}},
	}
	chatModel := &stagedFullDocumentChat{
		outlineResponse: types.ChatResponse{Content: "# 智慧运行建设方案\n\n## 建设目标\n"},
		sectionStreams: [][]types.StreamResponse{{
			{ResponseType: types.ResponseTypeAnswer, Content: "说明建设目标与业务价值，当前章节仍需补充接口与实施边界。"},
			{ResponseType: types.ResponseTypeError, Content: "context deadline exceeded"},
		}},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:              "请输出完整的智慧运行建设方案",
		Session:            &types.Session{ID: "sess-grounded-timeout-autocontinue"},
		AssistantMessageID: "msg-grounded-timeout-autocontinue",
		DocumentOutputMode: types.ChatDocumentOutputModeFull,
	}
	agentConfig := &types.AgentConfig{
		Temperature:    0.3,
		KnowledgeBases: []string{"kb-1"},
		SearchTargets:  types.SearchTargets{{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase}},
	}

	var completes []event.AgentCompleteData
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runKnowledgeGroundedFullDocumentGenerationPath(context.Background(), req, bus, chatModel, agentConfig)
	require.NoError(t, err)
	require.Len(t, completes, 1)
	require.NotNil(t, runRepo.updated)
	assert.Equal(t, types.ChatDocumentGenerationRunStatusContinuing, runRepo.updated.Status)
	assert.Equal(t, types.MessageCompletionStatusPartial, completes[0].CompletionStatus)
	assert.Equal(t, "section_generation_error", completes[0].FinishReason)
	assert.Equal(t, "llm_timeout", completes[0].FailureReason)
	assert.Equal(t, types.ChatDocumentGenerationStatusContinuing, completes[0].DocumentGenerationStatus)
	require.NotNil(t, completes[0].AutoContinueNext)
	assert.True(t, *completes[0].AutoContinueNext)
	assert.Empty(t, completes[0].AutoContinueReason)
	assert.Contains(t, completes[0].FinalAnswer, "## 第1章 建设目标")
	assert.Contains(t, completes[0].FinalAnswer, "说明建设目标与业务价值")
	require.NotNil(t, completes[0].Extra)
	assert.Equal(t, runRepo.created.ID, completes[0].Extra["generation_run_id"])
	assert.Empty(t, unmarshalGenerationRunStringSlice(runRepo.updated.CompletedSectionsJSON))
}

func TestRunKnowledgeGroundedDocumentContinuationPath_StopsAfterSecondRecoverableLLMTimeout(t *testing.T) {
	streaming := true
	thinkingControl := true
	run := &types.ChatDocumentGenerationRun{
		ID:                    "run-timeout-stop-1",
		TenantID:              0,
		SessionID:             "sess-grounded-timeout-stop",
		OriginalQuery:         "请输出完整的智慧运行建设方案",
		DocumentTitle:         "智慧运行建设方案",
		OutlineJSON:           marshalGenerationRunJSON(newDedicatedFullDocumentOutlineFromStrings("智慧运行建设方案", []string{"建设目标", "平台架构"})),
		EffectiveKBIDsJSON:    marshalGenerationRunJSON([]string{"kb-1"}),
		CompletedSectionsJSON: marshalGenerationRunJSON([]string{"建设目标"}),
		Status:                types.ChatDocumentGenerationRunStatusContinuing,
		AutoContinueRound:     1,
		MaxRounds:             8,
	}
	runRepo := &fullDocumentGenerationRunRepoStub{runs: map[string]*types.ChatDocumentGenerationRun{"run-timeout-stop-1": cloneChatDocumentGenerationRun(run)}}
	searchStub := &fullDocumentKnowledgeSearchStub{
		kbs: map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", Type: types.KnowledgeBaseTypeDocument}},
		results: map[string][]*types.SearchResult{
			"kb-1|请检索与当前章节直接相关的本地事实和能力说明：平台架构": {
				{ID: "sec-2", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-2", KnowledgeTitle: "平台架构", Content: "平台架构依据。", Score: 0.9},
			},
			"kb-1|智慧运行建设方案 平台架构": {
				{ID: "sec-2b", KnowledgeBaseID: "kb-1", KnowledgeID: "doc-2", KnowledgeTitle: "平台架构", Content: "平台架构补充依据。", Score: 0.88},
			},
		},
	}
	svc := &sessionService{
		cfg:                  &config.Config{Conversation: &config.ConversationConfig{Summary: &config.SummaryConfig{MaxCompletionTokens: 256}}},
		knowledgeBaseService: searchStub,
		generationRunRepo:    runRepo,
		modelService: &fullDocumentModelServiceStub{model: &types.Model{
			ID:     "staged-full-document-chat",
			Source: types.ModelSourceOpenAI,
			Parameters: types.ModelParameters{
				ContextWindowTokens:     131072,
				MaxOutputTokens:         8192,
				SupportsStreaming:       &streaming,
				SupportsThinkingControl: &thinkingControl,
			},
		}},
	}
	chatModel := &stagedFullDocumentChat{
		sectionStreams: [][]types.StreamResponse{{
			{ResponseType: types.ResponseTypeAnswer, Content: "说明平台架构与分层能力。"},
			{ResponseType: types.ResponseTypeError, Content: "context deadline exceeded"},
		}},
	}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:                     "以当前文档为基准，继续剩余内容输出",
		Session:                   &types.Session{ID: "sess-grounded-timeout-stop"},
		AssistantMessageID:        "msg-grounded-timeout-stop",
		DocumentIntent:            types.ChatDocumentIntentContinue,
		DocumentOperation:         types.ChatDocumentOperationContinue,
		DocumentOutputMode:        types.ChatDocumentOutputModeDelta,
		BaseArtifactID:            "artifact-timeout-stop-1",
		BaseArtifact:              &types.ChatDocumentArtifact{ID: "artifact-timeout-stop-1", ContentSnapshot: "# 智慧运行建设方案\n\n## 建设目标\n\n说明建设目标与业务价值。"},
		AutoContinue:              true,
		GenerationRunID:           "run-timeout-stop-1",
		AutoContinueRound:         1,
		AutoContinuePrompt:        "以当前文档为基准，继续剩余内容输出",
		AutoContinueOriginalQuery: "请输出完整的智慧运行建设方案",
	}
	agentConfig := &types.AgentConfig{
		Temperature:    0.3,
		KnowledgeBases: []string{"kb-1"},
		SearchTargets:  types.SearchTargets{{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase}},
	}

	var completes []event.AgentCompleteData
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runKnowledgeGroundedDocumentContinuationPath(context.Background(), req, bus, chatModel, agentConfig)
	require.NoError(t, err)
	require.Len(t, completes, 1)
	require.NotNil(t, runRepo.updated)
	assert.Equal(t, types.ChatDocumentGenerationRunStatusContinuing, runRepo.updated.Status)
	assert.Equal(t, 2, runRepo.updated.AutoContinueRound)
	assert.Equal(t, types.MessageCompletionStatusPartial, completes[0].CompletionStatus)
	assert.Equal(t, "llm_timeout_retry_exhausted", completes[0].FinishReason)
	assert.Equal(t, "llm_timeout", completes[0].FailureReason)
	assert.Equal(t, types.ChatDocumentGenerationStatusContinuing, completes[0].DocumentGenerationStatus)
	require.NotNil(t, completes[0].AutoContinueNext)
	assert.False(t, *completes[0].AutoContinueNext)
	assert.Equal(t, "llm_timeout_retry_exhausted", completes[0].AutoContinueReason)
	assert.Contains(t, completes[0].FinalAnswer, "## 第2章 平台架构")
}

func TestBindKnowledgeGroundedGenerationRunArtifact_SetsRootArtifactOnce(t *testing.T) {
	runRepo := &fullDocumentGenerationRunRepoStub{runs: map[string]*types.ChatDocumentGenerationRun{
		"run-1": {
			ID:        "run-1",
			TenantID:  0,
			SessionID: "sess-1",
			Status:    types.ChatDocumentGenerationRunStatusContinuing,
		},
	}}
	svc := &sessionService{generationRunRepo: runRepo}

	err := svc.BindKnowledgeGroundedGenerationRunArtifact(context.Background(), "run-1", &types.ChatDocumentArtifact{ID: "artifact-1", SessionID: "sess-1"})
	require.NoError(t, err)
	require.NotNil(t, runRepo.updated)
	assert.Equal(t, "artifact-1", runRepo.updated.RootArtifactID)

	runRepo.updated = nil
	err = svc.BindKnowledgeGroundedGenerationRunArtifact(context.Background(), "run-1", &types.ChatDocumentArtifact{ID: "artifact-2", SessionID: "sess-1"})
	require.NoError(t, err)
	assert.Nil(t, runRepo.updated)
	stored := runRepo.runs["run-1"]
	require.NotNil(t, stored)
	assert.Equal(t, "artifact-1", stored.RootArtifactID)
}

func TestBuildChatDocumentTerminalReplayExtra_RestoresTranslationContinuationMetadata(t *testing.T) {
	runOutline := longDocumentTranslationRunOutline{
		TaskKind:           types.ChatDocumentTaskKindTranslation,
		KnowledgeID:        "knowledge-1",
		KnowledgeTitle:     "原始文档",
		SourceSnapshotHash: "snapshot-hash-1",
		SourceLanguage:     "auto",
		TargetLanguage:     "English",
		OutputFormat:       "markdown",
		PreserveStructure:  true,
		Segments: []longDocumentTranslationRunSegment{
			{ID: "seg-1", ChunkStartSeq: 0, ChunkEndSeq: 9},
			{ID: "seg-2", ChunkStartSeq: 10, ChunkEndSeq: 19},
			{ID: "seg-3", ChunkStartSeq: 20, ChunkEndSeq: 29},
		},
	}
	run := &types.ChatDocumentGenerationRun{
		ID:                    "run-translation-1",
		TenantID:              1,
		SessionID:             "sess-1",
		RootMessageID:         "msg-1",
		RootArtifactID:        "artifact-1",
		OutlineJSON:           marshalGenerationRunJSON(runOutline),
		CompletedSectionsJSON: marshalGenerationRunJSON([]string{"seg-1", "seg-2"}),
	}
	runRepo := &fullDocumentGenerationRunRepoStub{runs: map[string]*types.ChatDocumentGenerationRun{"run-translation-1": cloneChatDocumentGenerationRun(run)}}
	svc := &sessionService{generationRunRepo: runRepo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	extra, err := svc.BuildChatDocumentTerminalReplayExtra(ctx, &types.Message{ID: "msg-1", SessionID: "sess-1"}, &types.ChatDocumentArtifact{ID: "artifact-1"})
	require.NoError(t, err)
	require.NotNil(t, extra)
	assert.Equal(t, types.ChatDocumentTaskKindTranslation, extra["document_task_kind"])
	assert.Equal(t, "run-translation-1", extra["generation_run_id"])
	assert.Equal(t, "原始文档", extra["document_title"])
	translationOptions, ok := extra["translation_options"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "English", translationOptions["target_language"])
	translationProgress, ok := extra["translation_progress"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 3, translationProgress["total_segments"])
	assert.Equal(t, 2, translationProgress["completed_segments"])
	assert.Equal(t, 1, translationProgress["remaining_segments"])
}

func TestRunKnowledgeGroundedDocumentContinuationPath_StopsWhenRunMaxRoundsReached(t *testing.T) {
	run := &types.ChatDocumentGenerationRun{
		ID:                    "run-max-rounds",
		TenantID:              0,
		SessionID:             "sess-grounded-run-limit",
		OriginalQuery:         "请输出完整的智慧运行建设方案",
		DocumentTitle:         "智慧运行建设方案",
		OutlineJSON:           marshalGenerationRunJSON(newDedicatedFullDocumentOutlineFromStrings("智慧运行建设方案", []string{"建设目标", "平台架构", "实施保障"})),
		EffectiveKBIDsJSON:    marshalGenerationRunJSON([]string{"kb-1"}),
		CompletedSectionsJSON: marshalGenerationRunJSON([]string{"建设目标"}),
		Status:                types.ChatDocumentGenerationRunStatusContinuing,
		AutoContinueRound:     1,
		MaxRounds:             1,
	}
	runRepo := &fullDocumentGenerationRunRepoStub{runs: map[string]*types.ChatDocumentGenerationRun{"run-max-rounds": cloneChatDocumentGenerationRun(run)}}
	svc := &sessionService{generationRunRepo: runRepo}
	bus := event.NewEventBus()
	req := &types.QARequest{
		Query:                     "以当前文档为基准，继续剩余内容输出",
		Session:                   &types.Session{ID: "sess-grounded-run-limit"},
		AssistantMessageID:        "msg-grounded-run-limit",
		DocumentIntent:            types.ChatDocumentIntentContinue,
		DocumentOperation:         types.ChatDocumentOperationContinue,
		DocumentOutputMode:        types.ChatDocumentOutputModeDelta,
		BaseArtifactID:            "artifact-1",
		BaseArtifact:              &types.ChatDocumentArtifact{ID: "artifact-1", ContentSnapshot: "# 智慧运行建设方案\n\n## 建设目标\n\n说明建设目标与业务价值。"},
		AutoContinue:              true,
		GenerationRunID:           "run-max-rounds",
		AutoContinueRound:         1,
		AutoContinuePrompt:        "以当前文档为基准，继续剩余内容输出",
		AutoContinueOriginalQuery: "请输出完整的智慧运行建设方案",
	}
	agentConfig := &types.AgentConfig{
		Temperature:    0.3,
		KnowledgeBases: []string{"kb-1"},
		SearchTargets:  types.SearchTargets{{KnowledgeBaseID: "kb-1", Type: types.SearchTargetTypeKnowledgeBase}},
	}

	var completes []event.AgentCompleteData
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completes = append(completes, data)
		return nil
	})

	err := svc.runKnowledgeGroundedDocumentContinuationPath(context.Background(), req, bus, &stagedFullDocumentChat{}, agentConfig)
	require.NoError(t, err)
	require.Len(t, completes, 1)
	require.NotNil(t, runRepo.updated)
	assert.Equal(t, types.ChatDocumentGenerationRunStatusBlocked, runRepo.updated.Status)
	assert.Equal(t, types.MessageCompletionStatusPartial, completes[0].CompletionStatus)
	assert.Equal(t, types.ChatDocumentGenerationStatusBlocked, completes[0].DocumentGenerationStatus)
	assert.Contains(t, completes[0].FinalAnswer, "已达到自动续写轮次上限")
}
