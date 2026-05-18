package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type chatDocumentArtifactRepoStub struct {
	artifactsByID        map[string]*types.ChatDocumentArtifact
	artifactsBySession   map[string][]*types.ChatDocumentArtifact
	artifactsByMessageID map[string]*types.ChatDocumentArtifact
	createdArtifacts     []*types.ChatDocumentArtifact
}

type chatDocumentEvidenceRefRepoStub struct {
	createdRefs      []*types.ChatDocumentEvidenceRef
	refsByArtifactID map[string][]*types.ChatDocumentEvidenceRef
}

func (s *chatDocumentArtifactRepoStub) CreateArtifact(ctx context.Context, artifact *types.ChatDocumentArtifact) error {
	_ = ctx
	copyArtifact := *artifact
	if copyArtifact.CreatedAt.IsZero() {
		copyArtifact.CreatedAt = time.Now()
	}
	copyArtifact.UpdatedAt = copyArtifact.CreatedAt
	s.createdArtifacts = append(s.createdArtifacts, &copyArtifact)
	if s.artifactsByID == nil {
		s.artifactsByID = map[string]*types.ChatDocumentArtifact{}
	}
	if s.artifactsBySession == nil {
		s.artifactsBySession = map[string][]*types.ChatDocumentArtifact{}
	}
	if s.artifactsByMessageID == nil {
		s.artifactsByMessageID = map[string]*types.ChatDocumentArtifact{}
	}
	s.artifactsByID[copyArtifact.ID] = &copyArtifact
	s.artifactsByMessageID[copyArtifact.SourceMessageID] = &copyArtifact
	s.artifactsBySession[copyArtifact.SessionID] = append([]*types.ChatDocumentArtifact{&copyArtifact}, s.artifactsBySession[copyArtifact.SessionID]...)
	return nil
}

func (s *chatDocumentArtifactRepoStub) GetArtifactByID(ctx context.Context, tenantID uint64, artifactID string) (*types.ChatDocumentArtifact, error) {
	_ = ctx
	_ = tenantID
	return s.artifactsByID[artifactID], nil
}

func (s *chatDocumentArtifactRepoStub) GetArtifactBySourceMessageID(ctx context.Context, tenantID uint64, sourceMessageID string) (*types.ChatDocumentArtifact, error) {
	_ = ctx
	_ = tenantID
	return s.artifactsByMessageID[sourceMessageID], nil
}

func (s *chatDocumentArtifactRepoStub) GetLatestArtifactBySession(ctx context.Context, tenantID uint64, sessionID string) (*types.ChatDocumentArtifact, error) {
	_ = ctx
	_ = tenantID
	items := s.artifactsBySession[sessionID]
	if len(items) == 0 {
		return nil, nil
	}
	return items[0], nil
}

func (s *chatDocumentArtifactRepoStub) ListArtifactsBySession(ctx context.Context, tenantID uint64, sessionID string, limit int) ([]*types.ChatDocumentArtifact, error) {
	_ = ctx
	_ = tenantID
	items := s.artifactsBySession[sessionID]
	if limit > 0 && len(items) > limit {
		return items[:limit], nil
	}
	return items, nil
}

var _ interfaces.ChatDocumentArtifactRepository = (*chatDocumentArtifactRepoStub)(nil)

func (s *chatDocumentEvidenceRefRepoStub) CreateEvidenceRefs(ctx context.Context, refs []*types.ChatDocumentEvidenceRef) error {
	_ = ctx
	if s.refsByArtifactID == nil {
		s.refsByArtifactID = map[string][]*types.ChatDocumentEvidenceRef{}
	}
	for _, ref := range refs {
		if ref == nil {
			continue
		}
		copied := *ref
		s.createdRefs = append(s.createdRefs, &copied)
		s.refsByArtifactID[copied.ArtifactID] = append(s.refsByArtifactID[copied.ArtifactID], &copied)
	}
	return nil
}

func (s *chatDocumentEvidenceRefRepoStub) ListEvidenceRefsByArtifactIDs(ctx context.Context, tenantID uint64, artifactIDs []string) ([]*types.ChatDocumentEvidenceRef, error) {
	_ = ctx
	_ = tenantID
	result := make([]*types.ChatDocumentEvidenceRef, 0)
	for _, artifactID := range artifactIDs {
		result = append(result, s.refsByArtifactID[artifactID]...)
	}
	return result, nil
}

var _ interfaces.ChatDocumentEvidenceRefRepository = (*chatDocumentEvidenceRefRepoStub)(nil)

func TestChatDocumentArtifactDetectIntent(t *testing.T) {
	svc := &chatDocumentArtifactService{}

	result, err := svc.DetectIntent(context.Background(), "session-1", "重新生成一版，不要基于上一版", "continue_document")
	require.NoError(t, err)
	assert.Equal(t, types.ChatDocumentIntentRegenerate, result.Intent)
	assert.Equal(t, types.ChatDocumentOperationRegenerate, result.Operation)

	result, err = svc.DetectIntent(context.Background(), "session-1", "继续生成剩余章节", "")
	require.NoError(t, err)
	assert.Equal(t, types.ChatDocumentIntentContinue, result.Intent)

	result, err = svc.DetectIntent(context.Background(), "session-1", "请继续补齐智慧运行章节", "")
	require.NoError(t, err)
	assert.Equal(t, types.ChatDocumentIntentRevise, result.Intent)
	assert.Equal(t, types.ChatDocumentOperationRevise, result.Operation)
	assert.Equal(t, "智慧运行", result.TargetHeading)
	assert.Equal(t, types.ChatDocumentMergeModeAppendToSection, result.MergeMode)

	result, err = svc.DetectIntent(context.Background(), "session-1", "继续扩写 2.5 火电设备运维智能体", "")
	require.NoError(t, err)
	assert.Equal(t, types.ChatDocumentIntentRevise, result.Intent)

	result, err = svc.DetectIntent(context.Background(), "session-1", "帮我再细化一下", types.ChatDocumentIntentRevise)
	require.NoError(t, err)
	assert.Equal(t, types.ChatDocumentIntentRevise, result.Intent)
}

func TestChatDocumentArtifactBuildQuotedContextForContinue(t *testing.T) {
	svc := &chatDocumentArtifactService{}
	artifact := &types.ChatDocumentArtifact{
		ID:               "artifact-1",
		RevisionNo:       2,
		CompletionStatus: types.MessageCompletionStatusPartial,
		Operation:        types.ChatDocumentOperationContinue,
		ArtifactKind:     types.ChatDocumentArtifactKindMarkdown,
		Status:           types.ChatDocumentArtifactStatusPartial,
		ContentSnapshot:  "# 标题\n\n## 背景\n\n内容一\n\n## 方案\n\n内容二",
	}

	quoted, err := svc.BuildQuotedContext(context.Background(), artifact, "继续生成", types.ChatDocumentIntentContinue, types.ChatDocumentOutputModeDelta, "", "")
	require.NoError(t, err)
	assert.Contains(t, quoted, "document_continuation_context")
	assert.Contains(t, quoted, "artifact-1")
	assert.Contains(t, quoted, "## 方案")
	assert.Contains(t, quoted, types.ChatDocumentCompletionMarker)
	assert.Contains(t, quoted, "<original_user_goal>")
	assert.Contains(t, quoted, "继续生成")
}

func TestChatDocumentArtifactBuildQuotedContextForReviseDelta(t *testing.T) {
	svc := &chatDocumentArtifactService{}
	artifact := &types.ChatDocumentArtifact{
		ID:               "artifact-2",
		RevisionNo:       3,
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Operation:        types.ChatDocumentOperationRevise,
		ArtifactKind:     types.ChatDocumentArtifactKindMarkdown,
		Status:           types.ChatDocumentArtifactStatusAvailable,
		ContentSnapshot:  "# 标题\n\n## 智慧运行\n\n原始内容",
	}

	quoted, err := svc.BuildQuotedContext(context.Background(), artifact, "请继续补齐智慧运行章节", types.ChatDocumentIntentRevise, types.ChatDocumentOutputModeDelta, "智慧运行", types.ChatDocumentMergeModeAppendToSection)
	require.NoError(t, err)
	assert.Contains(t, quoted, "<document_patch>")
	assert.Contains(t, quoted, "<replace heading=\"## 标题\">")
	assert.Contains(t, quoted, "把新增内容追加到目标章节内")
	assert.Contains(t, quoted, "<document_edit_target>")
	assert.Contains(t, quoted, "target_heading: 智慧运行")
	assert.NotContains(t, quoted, "输出修改后的完整 Markdown 文档")
}

func TestChatDocumentArtifactBuildQuotedContextUsesOutlineTailForOversizedArtifact(t *testing.T) {
	svc := &chatDocumentArtifactService{}
	longContent := "# 超长方案\n\n## 第一章\n\n" + strings.Repeat("章节内容", types.ChatDocumentArtifactInlineContinuationMaxChars/4+10) + "\n\n## 当前结尾\n\n继续从这里展开"
	artifact := &types.ChatDocumentArtifact{
		ID:               "artifact-oversized",
		RevisionNo:       6,
		CompletionStatus: types.MessageCompletionStatusCompleted,
		ArtifactKind:     types.ChatDocumentArtifactKindMarkdown,
		Status:           types.ChatDocumentArtifactStatusAvailable,
		ContentSnapshot:  longContent,
	}

	quoted, err := svc.BuildQuotedContext(context.Background(), artifact, "生成一篇完整的超长技术方案", types.ChatDocumentIntentContinue, types.ChatDocumentOutputModeDelta, "", "")
	require.NoError(t, err)
	assert.NotEmpty(t, quoted)
	assert.True(t, artifact.CanContinue())
	assert.Contains(t, quoted, "continuation_context_mode: outline_tail")
	assert.Contains(t, quoted, "<document_outline>")
	assert.Contains(t, quoted, "<document_tail>")
	assert.Contains(t, quoted, "<original_user_goal>")
	assert.Contains(t, quoted, "生成一篇完整的超长技术方案")
	assert.Contains(t, quoted, "## 当前结尾")
	assert.NotContains(t, quoted, longContent)
	hydrateChatDocumentArtifactDerivedFields(artifact)
	assert.Equal(t, types.ChatDocumentContinuationContextModeOutlineTail, artifact.ContinuationContextMode)
	assert.NotEmpty(t, artifact.UserHint)
}

func TestChatDocumentArtifactBuildQuotedContextSkipsBlockedArtifact(t *testing.T) {
	svc := &chatDocumentArtifactService{}
	artifact := &types.ChatDocumentArtifact{
		ID:                       "artifact-blocked",
		RevisionNo:               2,
		CompletionStatus:         types.MessageCompletionStatusPartial,
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusBlocked,
		Operation:                types.ChatDocumentOperationContinue,
		ArtifactKind:             types.ChatDocumentArtifactKindMarkdown,
		Status:                   types.ChatDocumentArtifactStatusPartial,
		ContentSnapshot:          "# 技术方案\n\n> 本地知识库未检索到足够内容，无法继续生成。",
	}

	hydrateChatDocumentArtifactDerivedFields(artifact)
	assert.False(t, artifact.CanContinue())
	assert.False(t, artifact.CanContinueDocument)

	quoted, err := svc.BuildQuotedContext(context.Background(), artifact, "继续生成", types.ChatDocumentIntentContinue, types.ChatDocumentOutputModeDelta, "", "")
	require.NoError(t, err)
	assert.Empty(t, quoted)
}

func TestChatDocumentArtifactBuildQuotedContextSkipsNeedsReviewArtifact(t *testing.T) {
	svc := &chatDocumentArtifactService{}
	artifact := &types.ChatDocumentArtifact{
		ID:                       "artifact-needs-review",
		RevisionNo:               2,
		CompletionStatus:         types.MessageCompletionStatusCompleted,
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusNeedsReview,
		Operation:                types.ChatDocumentOperationContinue,
		ArtifactKind:             types.ChatDocumentArtifactKindMarkdown,
		Status:                   types.ChatDocumentArtifactStatusPartial,
		ContentSnapshot:          "# 技术方案\n\n## 第一章\n\n存在结构告警，待人工复核。",
	}

	hydrateChatDocumentArtifactDerivedFields(artifact)
	assert.False(t, artifact.CanContinue())
	assert.False(t, artifact.CanContinueDocument)

	quoted, err := svc.BuildQuotedContext(context.Background(), artifact, "继续生成", types.ChatDocumentIntentContinue, types.ChatDocumentOutputModeDelta, "", "")
	require.NoError(t, err)
	assert.Empty(t, quoted)
}

func TestChatDocumentArtifactBuildQuotedContextAllowsNeedsReviewRevision(t *testing.T) {
	svc := &chatDocumentArtifactService{}
	artifact := &types.ChatDocumentArtifact{
		ID:                       "artifact-needs-review-revise",
		RevisionNo:               3,
		CompletionStatus:         types.MessageCompletionStatusCompleted,
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusNeedsReview,
		Operation:                types.ChatDocumentOperationRevise,
		ArtifactKind:             types.ChatDocumentArtifactKindMarkdown,
		Status:                   types.ChatDocumentArtifactStatusPartial,
		ContentSnapshot:          "# 技术方案\n\n## 第一章\n\n存在结构告警，待人工复核。",
		QualityIssues:            []string{types.ChatDocumentQualityIssueMarkdownTooShort},
	}

	hydrateChatDocumentArtifactDerivedFields(artifact)
	assert.False(t, artifact.CanContinue())
	assert.True(t, artifact.CanManualRevise())
	assert.True(t, artifact.CanUseAsBase())
	assert.False(t, artifact.CanIndex())
	require.NotEmpty(t, artifact.QualityIssueDetails)

	quoted, err := svc.BuildQuotedContext(context.Background(), artifact, "请补充第一章细节", types.ChatDocumentIntentRevise, types.ChatDocumentOutputModeDelta, "第一章", types.ChatDocumentMergeModeAppendToSection)
	require.NoError(t, err)
	assert.NotEmpty(t, quoted)
	assert.Contains(t, quoted, "target_heading: 第一章")
}

func TestChatDocumentArtifactBuildQuotedContextUsesTargetSectionWindowForOversizedRevision(t *testing.T) {
	svc := &chatDocumentArtifactService{}
	longTarget := strings.Repeat("智慧运行章节内容", 2400)
	longNeighbor := strings.Repeat("智慧安防章节内容", 1800)
	artifact := &types.ChatDocumentArtifact{
		ID:               "artifact-targeted",
		RevisionNo:       4,
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Operation:        types.ChatDocumentOperationRevise,
		ArtifactKind:     types.ChatDocumentArtifactKindMarkdown,
		Status:           types.ChatDocumentArtifactStatusAvailable,
		ContentSnapshot:  "# 超长方案\n\n## 二、五大核心功能模块\n\n### 2.2 智慧运行\n\n" + longTarget + "\n\n### 2.3 智慧安防系统\n\n" + longNeighbor + "\n\n## 三、项目总结\n\n总结内容",
	}

	quoted, err := svc.BuildQuotedContext(context.Background(), artifact, "继续补充智慧运行章节", types.ChatDocumentIntentRevise, types.ChatDocumentOutputModeDelta, "智慧运行", types.ChatDocumentMergeModeAppendToSection)
	require.NoError(t, err)
	assert.Contains(t, quoted, "snapshot_mode: targeted_section_context")
	assert.Contains(t, quoted, "<target_section_heading>")
	assert.Contains(t, quoted, "### 2.2 智慧运行")
	assert.Contains(t, quoted, "<target_section>")
	assert.Contains(t, quoted, "<target_parent>")
	assert.Contains(t, quoted, "## 二、五大核心功能模块")
	assert.Contains(t, quoted, "<nearby_siblings>")
	assert.Contains(t, quoted, "### 2.3 智慧安防系统")
	assert.NotContains(t, quoted, "<document_tail>")
}

func TestChatDocumentArtifactBuildQuotedContextUsesDualAnchorContextForMoveRequest(t *testing.T) {
	svc := &chatDocumentArtifactService{}
	artifact := &types.ChatDocumentArtifact{
		ID:               "artifact-dual-anchor",
		RevisionNo:       5,
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Operation:        types.ChatDocumentOperationRevise,
		ArtifactKind:     types.ChatDocumentArtifactKindMarkdown,
		Status:           types.ChatDocumentArtifactStatusAvailable,
		ContentSnapshot:  "# 总体方案\n\n## 第二章\n\n目标章节内容\n\n### 2.1 项目背景\n\n背景内容\n\n## 第三章\n\n### 2.5.5 火电设备运维智能体——技术实现\n\n待迁移内容 A\n\n#### 2.5.5.1 技术细节\n\n待迁移内容 B\n\n### 2.5.6 相邻章节\n\n相邻内容",
	}

	quoted, err := svc.BuildQuotedContext(context.Background(), artifact, "把 2.5.5 火电设备运维智能体——技术实现 后续的内容，合并到第二章。", types.ChatDocumentIntentRevise, types.ChatDocumentOutputModeDelta, "", "")
	require.NoError(t, err)
	assert.Contains(t, quoted, "snapshot_mode: dual_anchor_context")
	assert.Contains(t, quoted, "document_edit_operation: move_after_heading_to_section")
	assert.Contains(t, quoted, "source_heading: 2.5.5 火电设备运维智能体——技术实现")
	assert.Contains(t, quoted, "target_heading: 第二章")
	assert.Contains(t, quoted, "<source_anchor_heading>")
	assert.Contains(t, quoted, "<destination_section_heading>")
	assert.Contains(t, quoted, "<destination_section>")
	assert.Contains(t, quoted, "<source_section>")
	assert.Contains(t, quoted, "<nearby_siblings>")
	assert.NotContains(t, quoted, "<document_tail>")
}

func TestBuildChatDocumentSnapshot_MergesReviseDeltaByHeading(t *testing.T) {
	base := &types.ChatDocumentArtifact{
		ContentSnapshot: "# 总体方案\n\n## 背景\n\n旧背景\n\n## 智慧运行\n\n旧章节\n\n## 实施计划\n\n旧计划",
	}

	snapshot := buildChatDocumentSnapshot("## 智慧运行\n\n新章节\n\n### 模块 A\n\n说明", types.RegisterChatDocumentArtifactOptions{
		Intent:       types.ChatDocumentIntentRevise,
		Operation:    types.ChatDocumentOperationRevise,
		OutputMode:   types.ChatDocumentOutputModeDelta,
		BaseArtifact: base,
	})

	assert.Contains(t, snapshot, "## 背景\n\n旧背景")
	assert.Contains(t, snapshot, "## 智慧运行\n\n新章节")
	assert.Contains(t, snapshot, "### 模块 A")
	assert.NotContains(t, snapshot, "## 智慧运行\n\n旧章节")
	assert.Contains(t, snapshot, "## 实施计划\n\n旧计划")
}

func TestBuildChatDocumentSnapshot_AppliesStructuredPatchReplace(t *testing.T) {
	base := &types.ChatDocumentArtifact{
		ContentSnapshot: "# 总体方案\n\n## 背景\n\n旧背景\n\n## 智慧运行\n\n旧章节\n\n## 实施计划\n\n旧计划",
	}

	snapshot := buildChatDocumentSnapshot(`<document_patch>
<replace heading="## 智慧运行">
## 智慧运行

新章节

### 模块 A

说明
</replace>
</document_patch>`, types.RegisterChatDocumentArtifactOptions{
		Intent:       types.ChatDocumentIntentRevise,
		Operation:    types.ChatDocumentOperationRevise,
		OutputMode:   types.ChatDocumentOutputModeDelta,
		BaseArtifact: base,
	})

	assert.Contains(t, snapshot, "## 背景\n\n旧背景")
	assert.Contains(t, snapshot, "## 智慧运行\n\n新章节")
	assert.Contains(t, snapshot, "### 模块 A")
	assert.NotContains(t, snapshot, "## 智慧运行\n\n旧章节")
	assert.Contains(t, snapshot, "## 实施计划\n\n旧计划")
}

func TestBuildChatDocumentSnapshot_AppliesStructuredPatchAppendAndInsertAfter(t *testing.T) {
	base := &types.ChatDocumentArtifact{
		ContentSnapshot: "# 总体方案\n\n## 背景\n\n旧背景\n\n## 智慧运行\n\n旧章节\n\n## 实施计划\n\n旧计划",
	}

	snapshot := buildChatDocumentSnapshot(`<document_patch>
<append heading="## 智慧运行">
### 模块 B

新增说明
</append>
<insert_after heading="## 智慧运行">
## 保障体系

新增保障
</insert_after>
</document_patch>`, types.RegisterChatDocumentArtifactOptions{
		Intent:       types.ChatDocumentIntentRevise,
		Operation:    types.ChatDocumentOperationRevise,
		OutputMode:   types.ChatDocumentOutputModeDelta,
		BaseArtifact: base,
	})

	assert.Contains(t, snapshot, "## 智慧运行\n\n旧章节\n\n### 模块 B\n\n新增说明")
	assert.Contains(t, snapshot, "## 保障体系\n\n新增保障\n\n## 实施计划")
}

func TestBuildChatDocumentSnapshot_AppliesStructuredPatchAppendByNormalizedHeading(t *testing.T) {
	base := &types.ChatDocumentArtifact{
		ContentSnapshot: "# 总体方案\n\n## 二、五大核心功能模块\n\n### 2.2 智慧运行\n\n旧章节\n\n### 2.3 智慧安防系统\n\n旧安防\n\n## 三、项目总结\n\n总结内容",
	}

	snapshot := buildChatDocumentSnapshot(`<document_patch>
<append heading="智慧运行">
### 模块 B

新增说明
</append>
</document_patch>`, types.RegisterChatDocumentArtifactOptions{
		Intent:       types.ChatDocumentIntentRevise,
		Operation:    types.ChatDocumentOperationRevise,
		OutputMode:   types.ChatDocumentOutputModeDelta,
		BaseArtifact: base,
	})

	assert.Contains(t, snapshot, "### 2.2 智慧运行\n\n旧章节\n\n### 模块 B\n\n新增说明")
	assert.Contains(t, snapshot, "### 2.3 智慧安防系统\n\n旧安防")
	assert.Contains(t, snapshot, "## 三、项目总结\n\n总结内容")
}

func TestBuildChatDocumentSnapshot_ContinueWithScopedTargetFallsBackToSectionAppend(t *testing.T) {
	base := &types.ChatDocumentArtifact{
		ContentSnapshot: "# 北海电厂二期智慧电厂项目技术方案\n\n## 二、五大核心功能模块\n\n### 2.2 智慧运行\n\n原有智慧运行内容。\n\n### 2.3 智慧安防系统\n\n原有智慧安防内容。\n\n## 三、项目总结\n\n总结内容。",
	}

	result := buildChatDocumentSnapshotResult("### 模块能力\n\n新增的监盘、预测、调度说明。", types.RegisterChatDocumentArtifactOptions{
		UserQuery:    "继续补充智慧运行章节，详细阐述每个模块的功能。",
		Intent:       types.ChatDocumentIntentContinue,
		Operation:    types.ChatDocumentOperationContinue,
		OutputMode:   types.ChatDocumentOutputModeDelta,
		BaseArtifact: base,
	})

	assert.Contains(t, result.Snapshot, "### 2.2 智慧运行\n\n原有智慧运行内容。\n\n### 模块能力\n\n新增的监盘、预测、调度说明。")
	assert.Contains(t, result.Snapshot, "### 2.3 智慧安防系统\n\n原有智慧安防内容。")
	assert.Contains(t, result.Snapshot, "## 三、项目总结\n\n总结内容。")
	assert.NotContains(t, result.QualityIssues, types.ChatDocumentQualityIssueDeltaMergeUncertain)
}

func TestInferSectionTargetFromQuery_StripsContinuationLeadPhrase(t *testing.T) {
	assert.Equal(t, "智慧运行", inferSectionTargetFromQuery("请继续补齐智慧运行章节"))
	assert.Equal(t, "智慧运行", inferSectionTargetFromQuery("继续补充智慧运行章节，详细阐述每个模块功能"))
}

func TestChatDocumentArtifactRegisterFromAssistantMessageAcceptsPatchWithRevisionPreamble(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	base := &types.ChatDocumentArtifact{
		ID:              "artifact-base",
		SessionID:       "session-1",
		RevisionNo:      2,
		ArtifactKind:    types.ChatDocumentArtifactKindMarkdown,
		Status:          types.ChatDocumentArtifactStatusAvailable,
		ContentSnapshot: "# 技术方案\n\n## 智慧运行\n\n旧内容\n\n## 实施计划\n\n原有计划",
	}
	message := &types.Message{
		ID:               "message-patch-preamble",
		SessionID:        "session-1",
		RequestID:        "request-patch-preamble",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Content: `已根据你的要求修改，以下是 patch：

<document_patch>
<replace heading="## 智慧运行">
## 智慧运行

新内容
</replace>
</document_patch>`,
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery:    "修改上一版，完善智慧运行章节",
		Intent:       types.ChatDocumentIntentRevise,
		Operation:    types.ChatDocumentOperationRevise,
		OutputMode:   types.ChatDocumentOutputModeDelta,
		BaseArtifact: base,
	})
	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Equal(t, types.ChatDocumentArtifactStatusAvailable, artifact.Status)
	assert.Contains(t, artifact.ContentSnapshot, "## 智慧运行\n\n新内容")
	assert.NotContains(t, artifact.ContentSnapshot, "<document_patch>")
	assert.NotContains(t, artifact.ContentSnapshot, "以下是 patch")
	assert.Contains(t, artifact.QualityIssues, types.ChatDocumentQualityIssueRevisionPreambleTrimmed)
	assert.NotContains(t, artifact.QualityIssues, types.ChatDocumentQualityIssueDeltaMergeUncertain)
	assert.Contains(t, artifact.ContentSnapshot, "## 实施计划\n\n原有计划")
}

func TestChatDocumentArtifactRegisterFromAssistantMessageMergesContinuation(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	base := &types.ChatDocumentArtifact{
		ID:              "artifact-base",
		SessionID:       "session-1",
		RevisionNo:      1,
		ContentSnapshot: "# 标题\n\n## 第一部分\n\n这里是上一版内容。",
	}
	largeParagraph := strings.Repeat("补充说明", 800)
	message := &types.Message{
		ID:               "message-1",
		SessionID:        "session-1",
		RequestID:        "request-1",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Content:          "\n\n## 第二部分\n\n这里是新增内容。\n\n### 小节\n\n1. 条目一\n2. 条目二\n\n|列1|列2|\n|---|---|\n|a|b|\n\n" + largeParagraph,
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery:    "继续生成技术方案",
		Intent:       types.ChatDocumentIntentContinue,
		Operation:    types.ChatDocumentOperationContinue,
		BaseArtifact: base,
	})
	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Equal(t, "artifact-base", artifact.ParentArtifactID)
	assert.Equal(t, 2, artifact.RevisionNo)
	assert.Contains(t, artifact.ContentSnapshot, "## 第一部分")
	assert.Contains(t, artifact.ContentSnapshot, "## 第二部分")
	assert.Equal(t, types.ChatDocumentArtifactStatusAvailable, artifact.Status)
}

func TestChatDocumentArtifactRegisterFromAssistantMessageKeepsSnapshotAboveLegacyLimit(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	base := &types.ChatDocumentArtifact{
		ID:              "artifact-large-base",
		SessionID:       "session-1",
		RevisionNo:      7,
		ArtifactKind:    types.ChatDocumentArtifactKindMarkdown,
		Status:          types.ChatDocumentArtifactStatusAvailable,
		ContentSnapshot: "# 超长方案\n\n## 已完成章节\n\n" + strings.Repeat("既有内容", 25000),
	}
	message := &types.Message{
		ID:               "message-large-continue",
		SessionID:        "session-1",
		RequestID:        "request-large-continue",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Content:          "## 新增章节\n\n" + strings.Repeat("新增内容", 2000),
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery:    "以当前文档为基准，继续剩余内容输出",
		Intent:       types.ChatDocumentIntentContinue,
		Operation:    types.ChatDocumentOperationContinue,
		OutputMode:   types.ChatDocumentOutputModeDelta,
		BaseArtifact: base,
	})

	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Greater(t, len([]rune(artifact.ContentSnapshot)), 100000)
	assert.Contains(t, artifact.ContentSnapshot, "## 新增章节")
	assert.NotContains(t, artifact.QualityIssues, types.ChatDocumentQualityIssueSnapshotTruncated)
	assert.Equal(t, types.ChatDocumentContinuationContextModeOutlineTail, artifact.ContinuationContextMode)
	assert.True(t, artifact.CanContinueDocument)
	assert.False(t, artifact.CanInlineContinue)
}

func TestChatDocumentArtifactRegisterFromAssistantMessageStripsCompletionMarker(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	base := &types.ChatDocumentArtifact{
		ID:              "artifact-base",
		SessionID:       "session-1",
		RevisionNo:      2,
		ArtifactKind:    types.ChatDocumentArtifactKindMarkdown,
		Status:          types.ChatDocumentArtifactStatusAvailable,
		ContentSnapshot: "# 技术方案\n\n## 第一章\n\n已有内容",
	}
	message := &types.Message{
		ID:               "message-complete-marker",
		SessionID:        "session-1",
		RequestID:        "request-complete-marker",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Content:          "## 总结\n\n最终内容\n\n" + types.ChatDocumentCompletionMarker,
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery:    "以当前文档为基准，继续剩余内容输出",
		Intent:       types.ChatDocumentIntentContinue,
		Operation:    types.ChatDocumentOperationContinue,
		OutputMode:   types.ChatDocumentOutputModeDelta,
		BaseArtifact: base,
	})

	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Equal(t, types.ChatDocumentGenerationStatusCompleted, artifact.DocumentGenerationStatus)
	assert.NotContains(t, artifact.ContentSnapshot, types.ChatDocumentCompletionMarker)
	assert.NotContains(t, message.Content, types.ChatDocumentCompletionMarker)
	assert.Contains(t, artifact.ContentSnapshot, "## 总结\n\n最终内容")
}

func TestChatDocumentArtifactRegisterFromAssistantMessageCreatesCompletedArtifactForMarkerOnly(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	base := &types.ChatDocumentArtifact{
		ID:              "artifact-base",
		SessionID:       "session-1",
		RevisionNo:      7,
		ArtifactKind:    types.ChatDocumentArtifactKindMarkdown,
		Status:          types.ChatDocumentArtifactStatusAvailable,
		ContentSnapshot: "# 技术方案\n\n## 7.2 实施方能力保障\n\n最终内容。",
	}
	message := &types.Message{
		ID:               "message-marker-only",
		SessionID:        "session-1",
		RequestID:        "request-marker-only",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Content:          types.ChatDocumentCompletionMarker,
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery:    "以当前文档为基准，继续剩余内容输出",
		Intent:       types.ChatDocumentIntentContinue,
		Operation:    types.ChatDocumentOperationContinue,
		OutputMode:   types.ChatDocumentOutputModeDelta,
		BaseArtifact: base,
	})

	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Equal(t, types.ChatDocumentGenerationStatusCompleted, artifact.DocumentGenerationStatus)
	assert.Equal(t, "artifact-base", artifact.ParentArtifactID)
	assert.Equal(t, 8, artifact.RevisionNo)
	assert.Equal(t, strings.TrimSpace(base.ContentSnapshot), artifact.ContentSnapshot)
	assert.Empty(t, strings.TrimSpace(message.Content))
}

func TestChatDocumentArtifactRegisterFromAssistantMessageTreatsCompletionNoticeAsCompleted(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	base := &types.ChatDocumentArtifact{
		ID:              "artifact-base",
		SessionID:       "session-1",
		RevisionNo:      8,
		ArtifactKind:    types.ChatDocumentArtifactKindMarkdown,
		Status:          types.ChatDocumentArtifactStatusAvailable,
		ContentSnapshot: "# 技术方案\n\n## 7.2 实施方能力保障\n\n最终内容。",
	}
	message := &types.Message{
		ID:               "message-completion-notice",
		SessionID:        "session-1",
		RequestID:        "request-completion-notice",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Content:          "文档已完成。",
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery:    "以当前文档为基准，继续剩余内容输出",
		Intent:       types.ChatDocumentIntentContinue,
		Operation:    types.ChatDocumentOperationContinue,
		OutputMode:   types.ChatDocumentOutputModeDelta,
		BaseArtifact: base,
	})

	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Equal(t, types.ChatDocumentGenerationStatusCompleted, artifact.DocumentGenerationStatus)
	assert.Contains(t, artifact.QualityIssues, types.ChatDocumentQualityIssueTerminalSectionTail)
	assert.Equal(t, strings.TrimSpace(base.ContentSnapshot), artifact.ContentSnapshot)
	assert.NotContains(t, artifact.ContentSnapshot, "文档已完成")
}

func TestChatDocumentArtifactRegisterFromAssistantMessageStopsAutoContinueOnDuplicateDocumentHead(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	base := &types.ChatDocumentArtifact{
		ID:              "artifact-base",
		SessionID:       "session-1",
		RevisionNo:      4,
		ArtifactKind:    types.ChatDocumentArtifactKindMarkdown,
		Status:          types.ChatDocumentArtifactStatusAvailable,
		ContentSnapshot: "# 北海电厂二期智慧电厂项目 — 智慧运行技术方案\n\n## 7.2 实施方能力保障\n\n保障内容。",
	}
	message := &types.Message{
		ID:               "message-duplicate-head",
		SessionID:        "session-1",
		RequestID:        "request-duplicate-head",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Content:          "## 北海电厂二期智慧电厂项目 — 智慧运行技术方案\n\n一、智慧运行总体概述\n\n重复回到开头。",
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery:    "以当前文档为基准，继续剩余内容输出",
		Intent:       types.ChatDocumentIntentContinue,
		Operation:    types.ChatDocumentOperationContinue,
		OutputMode:   types.ChatDocumentOutputModeDelta,
		BaseArtifact: base,
	})

	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Equal(t, types.ChatDocumentGenerationStatusNeedsReview, artifact.DocumentGenerationStatus)
	assert.Contains(t, artifact.QualityIssues, types.ChatDocumentQualityIssueDuplicateDocumentHead)
	assert.Equal(t, "检测到本轮续写重新输出了文档开头，系统已暂停自动续写。请检查完整文档后再继续。", artifact.UserHint)
}

func TestChatDocumentArtifactRegisterFromAssistantMessageStopsAutoContinueOnSectionReset(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	base := &types.ChatDocumentArtifact{
		ID:              "artifact-base",
		SessionID:       "session-1",
		RevisionNo:      5,
		ArtifactKind:    types.ChatDocumentArtifactKindMarkdown,
		Status:          types.ChatDocumentArtifactStatusAvailable,
		ContentSnapshot: "# 技术方案\n\n## 6. 智慧安防\n\n内容。\n\n## 7.2 实施方能力保障\n\n保障内容。",
	}
	message := &types.Message{
		ID:               "message-section-reset",
		SessionID:        "session-1",
		RequestID:        "request-section-reset",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Content:          "一、智慧运行总体概述\n\n章节编号回退。",
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery:    "以当前文档为基准，继续剩余内容输出",
		Intent:       types.ChatDocumentIntentContinue,
		Operation:    types.ChatDocumentOperationContinue,
		OutputMode:   types.ChatDocumentOutputModeDelta,
		BaseArtifact: base,
	})

	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Equal(t, types.ChatDocumentGenerationStatusNeedsReview, artifact.DocumentGenerationStatus)
	assert.Contains(t, artifact.QualityIssues, types.ChatDocumentQualityIssueSectionNumberReset)
	assert.Equal(t, "检测到本轮续写出现章节编号回退，系统已暂停自动续写。请检查完整文档后再继续。", artifact.UserHint)
}

func TestChatDocumentArtifactRegisterFromAssistantMessageStopsAutoContinueOnLowNoveltyDelta(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	repeated := strings.Repeat("智慧运行系统依托全域数据底座实现智能监盘、故障预测和调度辅助。", 20)
	base := &types.ChatDocumentArtifact{
		ID:              "artifact-base",
		SessionID:       "session-1",
		RevisionNo:      6,
		ArtifactKind:    types.ChatDocumentArtifactKindMarkdown,
		Status:          types.ChatDocumentArtifactStatusAvailable,
		ContentSnapshot: "# 技术方案\n\n## 7.2 实施方能力保障\n\n" + repeated,
	}
	message := &types.Message{
		ID:               "message-low-novelty",
		SessionID:        "session-1",
		RequestID:        "request-low-novelty",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Content:          repeated,
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery:    "以当前文档为基准，继续剩余内容输出",
		Intent:       types.ChatDocumentIntentContinue,
		Operation:    types.ChatDocumentOperationContinue,
		OutputMode:   types.ChatDocumentOutputModeDelta,
		BaseArtifact: base,
	})

	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Equal(t, types.ChatDocumentGenerationStatusNeedsReview, artifact.DocumentGenerationStatus)
	assert.Contains(t, artifact.QualityIssues, types.ChatDocumentQualityIssueLowNoveltyDelta)
	assert.Equal(t, "检测到本轮续写与已有内容高度重复，系统已暂停自动续写。请检查完整文档后再继续。", artifact.UserHint)
}

func TestChatDocumentArtifactRegisterFromAssistantMessageMarksReviseAsPartial(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	base := &types.ChatDocumentArtifact{
		ID:              "artifact-base",
		SessionID:       "session-1",
		RevisionNo:      3,
		ContentSnapshot: "# 标题\n\n## 第一部分\n\n" + strings.Repeat("原始内容", 600),
	}
	message := &types.Message{
		ID:               "message-2",
		SessionID:        "session-1",
		RequestID:        "request-2",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Content:          strings.Repeat("已调整为更精简的版本。", 260),
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery:    "修改上一版，但不要精简",
		Intent:       types.ChatDocumentIntentRevise,
		Operation:    types.ChatDocumentOperationRevise,
		BaseArtifact: base,
	})
	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Equal(t, types.ChatDocumentArtifactStatusPartial, artifact.Status)
	assert.Equal(t, 4, artifact.RevisionNo)
	assert.Equal(t, "artifact-base", artifact.ParentArtifactID)
}

func TestChatDocumentArtifactRegisterFromAssistantMessageCreatesRecoveredPartialRevisionFromShortDelta(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	base := &types.ChatDocumentArtifact{
		ID:              "artifact-base",
		SessionID:       "session-1",
		RevisionNo:      1,
		ArtifactKind:    types.ChatDocumentArtifactKindMarkdown,
		Status:          types.ChatDocumentArtifactStatusAvailable,
		ContentSnapshot: "# 技术方案\n\n## 智慧运行\n\n旧内容\n\n## 保障体系\n\n原有保障",
	}
	message := &types.Message{
		ID:               "message-short-delta",
		SessionID:        "session-1",
		RequestID:        "request-short-delta",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusPartial,
		Content:          "## 智慧运行\n\n补齐后的新内容",
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery:    "请继续补齐智慧运行章节",
		Intent:       types.ChatDocumentIntentContinue,
		Operation:    types.ChatDocumentOperationRevise,
		OutputMode:   types.ChatDocumentOutputModeDelta,
		BaseArtifact: base,
	})
	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Equal(t, "artifact-base", artifact.ParentArtifactID)
	assert.Equal(t, 2, artifact.RevisionNo)
	assert.Equal(t, types.ChatDocumentArtifactStatusPartial, artifact.Status)
	assert.Contains(t, artifact.ContentSnapshot, "补齐后的新内容")
	assert.Contains(t, artifact.ContentSnapshot, "## 保障体系\n\n原有保障")
}

func TestChatDocumentArtifactRegisterFromAssistantMessageMarksUncertainPatchMergeAsPartial(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	base := &types.ChatDocumentArtifact{
		ID:              "artifact-base",
		SessionID:       "session-1",
		RevisionNo:      2,
		ArtifactKind:    types.ChatDocumentArtifactKindMarkdown,
		Status:          types.ChatDocumentArtifactStatusAvailable,
		ContentSnapshot: "# 技术方案\n\n## 智慧运行\n\n旧内容\n\n## 实施计划\n\n原有计划",
	}
	message := &types.Message{
		ID:               "message-patch-uncertain",
		SessionID:        "session-1",
		RequestID:        "request-patch-uncertain",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Content: `<document_patch>
<replace heading="## 不存在章节">
## 新增章节

补充内容
</replace>
</document_patch>`,
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery:    "修改上一版，补充新章节",
		Intent:       types.ChatDocumentIntentRevise,
		Operation:    types.ChatDocumentOperationRevise,
		OutputMode:   types.ChatDocumentOutputModeDelta,
		BaseArtifact: base,
	})
	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Equal(t, types.ChatDocumentArtifactStatusPartial, artifact.Status)
	assert.Contains(t, artifact.QualityIssues, types.ChatDocumentQualityIssueDeltaMergeUncertain)
	assert.Contains(t, artifact.ContentSnapshot, "## 智慧运行\n\n旧内容")
	assert.Contains(t, artifact.ContentSnapshot, "## 新增章节\n\n补充内容")
	assert.Equal(t, "本次修改有部分片段无法精确定位，系统已按保守策略合并到文档末尾。建议检查完整文档后继续微调。", artifact.UserHint)
}

func TestChatDocumentArtifactRegisterFromAssistantMessageMarksAmbiguousTargetSectionWithSpecificHint(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	base := &types.ChatDocumentArtifact{
		ID:              "artifact-base",
		SessionID:       "session-1",
		RevisionNo:      2,
		ArtifactKind:    types.ChatDocumentArtifactKindMarkdown,
		Status:          types.ChatDocumentArtifactStatusAvailable,
		ContentSnapshot: "# 技术方案\n\n## 一、总览\n\n### 2.2 智慧运行\n\n旧内容 A\n\n## 二、专题\n\n### 智慧运行\n\n旧内容 B\n\n## 三、项目总结\n\n总结",
	}
	message := &types.Message{
		ID:               "message-patch-ambiguous",
		SessionID:        "session-1",
		RequestID:        "request-patch-ambiguous",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Content: `<document_patch>
<append heading="智慧运行">
新增内容
</append>
</document_patch>`,
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery:    "修改上一版，补充智慧运行章节",
		Intent:       types.ChatDocumentIntentRevise,
		Operation:    types.ChatDocumentOperationRevise,
		OutputMode:   types.ChatDocumentOutputModeDelta,
		BaseArtifact: base,
	})
	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Equal(t, types.ChatDocumentArtifactStatusPartial, artifact.Status)
	assert.Contains(t, artifact.QualityIssues, types.ChatDocumentQualityIssueDeltaMergeUncertain)
	assert.Contains(t, artifact.QualityIssues, types.ChatDocumentQualityIssueTargetSectionUncertain)
	assert.Equal(t, "目标章节未能唯一定位，系统已按保守策略合并到文档末尾。建议明确章节标题或编号后重试。", artifact.UserHint)
	assert.True(t, strings.HasSuffix(strings.TrimSpace(artifact.ContentSnapshot), "新增内容"))
}

func TestChatDocumentArtifactRegisterFromAssistantMessageRepairsRevisionPreambleAndCodeFence(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	base := &types.ChatDocumentArtifact{
		ID:              "artifact-base",
		SessionID:       "session-1",
		RevisionNo:      2,
		ContentSnapshot: "# 原始标题\n\n## 背景\n\n" + strings.Repeat("原始内容", 500),
	}
	message := &types.Message{
		ID:               "message-3",
		SessionID:        "session-1",
		RequestID:        "request-3",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Content:          "已根据你的要求修改，以下是完整版本：\n\n# 更新后的方案\n\n## 背景\n\n" + strings.Repeat("更新内容", 520) + "\n\n```go\nfmt.Println(\"hello\")\n",
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery:    "修改上一版，补齐细节",
		Intent:       types.ChatDocumentIntentRevise,
		Operation:    types.ChatDocumentOperationRevise,
		BaseArtifact: base,
	})
	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.NotContains(t, artifact.ContentSnapshot, "已根据你的要求修改")
	assert.True(t, strings.HasSuffix(strings.TrimSpace(artifact.ContentSnapshot), "```"))
	assert.Contains(t, artifact.QualityIssues, types.ChatDocumentQualityIssueRevisionPreambleTrimmed)
	assert.Contains(t, artifact.QualityIssues, types.ChatDocumentQualityIssueUnclosedCodeFence)
	assert.Equal(t, "检测到末尾代码块未闭合，系统已自动补全代码围栏。", artifact.UserHint)
}

func TestPrepareChatDocumentArtifactSnapshot_NormalizesMalformedMarkdownHeadings(t *testing.T) {
	result := prepareChatDocumentArtifactSnapshot("# 北海电厂二期智慧电厂项目技术方案\n\n## 数据湖与基础算力平台\n###3.1全域数据湖建设\n建设内容覆盖统一汇聚、治理与算力调度。", types.RegisterChatDocumentArtifactOptions{
		Operation: types.ChatDocumentOperationCreate,
	})
	assert.True(t, result.ShouldCreate)
	assert.Contains(t, result.Snapshot, "### 3.1 全域数据湖建设\n\n建设内容覆盖统一汇聚、治理与算力调度。")
	assert.Contains(t, result.QualityIssues, types.ChatDocumentQualityIssueMarkdownHeadingNormalized)
}

func TestPrepareChatDocumentArtifactSnapshot_DoesNotRewriteCodeFenceBodyAsHeading(t *testing.T) {
	result := prepareChatDocumentArtifactSnapshot("# 技术方案\n\n```markdown\n###3.1全域数据湖建设\n```", types.RegisterChatDocumentArtifactOptions{
		Operation: types.ChatDocumentOperationCreate,
	})
	assert.True(t, result.ShouldCreate)
	assert.Contains(t, result.Snapshot, "```markdown\n###3.1全域数据湖建设\n```")
	assert.NotContains(t, result.QualityIssues, types.ChatDocumentQualityIssueMarkdownHeadingNormalized)
}

func TestChatDocumentArtifactRegisterFromAssistantMessage_MergesExplicitQualityIssues(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	message := &types.Message{
		ID:               "message-quality-1",
		SessionID:        "session-1",
		RequestID:        "request-quality-1",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Content:          "# 技术方案\n\n## 第一章\n\n" + strings.Repeat("内容说明。", 500),
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery:     "生成完整技术方案文档",
		Operation:     types.ChatDocumentOperationCreate,
		OutputMode:    types.ChatDocumentOutputModeFull,
		QualityIssues: []string{types.ChatDocumentQualityIssueMarkdownHeadingNormalized, types.ChatDocumentQualityIssueMarkdownStructureInvalid},
	})
	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Contains(t, artifact.QualityIssues, types.ChatDocumentQualityIssueMarkdownHeadingNormalized)
	assert.Contains(t, artifact.QualityIssues, types.ChatDocumentQualityIssueMarkdownStructureInvalid)
}

func TestChatDocumentArtifactRegisterFromAssistantMessageMarksCompletedLongDocumentNeedsReviewOnPromptLeakage(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	message := &types.Message{
		ID:               "message-quality-leak-1",
		SessionID:        "session-1",
		RequestID:        "request-quality-leak-1",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Content:          "# 技术方案\n\nCurrent section heading: ## 第一章 建设目标\n\n<local_knowledge_context>\n- knowledge_id: doc-1\n</local_knowledge_context>\n\n## 第一章 建设目标\n\n" + strings.Repeat("建设目标说明。", 120),
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery:                "生成完整技术方案文档",
		NeedArtifact:             true,
		Operation:                types.ChatDocumentOperationCreate,
		OutputMode:               types.ChatDocumentOutputModeFull,
		UseLongDocument:          true,
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusCompleted,
	})
	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Equal(t, types.ChatDocumentGenerationStatusNeedsReview, artifact.DocumentGenerationStatus)
	assert.Equal(t, types.ChatDocumentArtifactStatusPartial, artifact.Status)
	assert.Contains(t, artifact.QualityIssues, types.ChatDocumentQualityIssueInternalPromptLeakage)
	assert.Equal(t, "检测到内部提示词或上下文标记混入文档，系统已将该版本标记为待复核。请检查正文后再继续。", artifact.UserHint)
}

func TestChatDocumentArtifactRegisterFromAssistantMessageKeepsOversizedArtifactWithHint(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	message := &types.Message{
		ID:               "message-4",
		SessionID:        "session-1",
		RequestID:        "request-4",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Content:          "# 超长方案\n\n## 内容\n\n" + strings.Repeat("超长内容段落。", types.ChatDocumentArtifactSnapshotMaxChars/7+100),
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery: "生成完整技术方案文档",
		Operation: types.ChatDocumentOperationCreate,
	})
	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Equal(t, types.ChatDocumentArtifactStatusPartial, artifact.Status)
	assert.Len(t, []rune(artifact.ContentSnapshot), types.ChatDocumentArtifactSnapshotMaxChars)
	assert.Contains(t, artifact.QualityIssues, types.ChatDocumentQualityIssueSnapshotTruncated)
	assert.Contains(t, artifact.QualityIssues, types.ChatDocumentQualityIssueInlineContextTooLarge)
	assert.False(t, artifact.CanInlineContinue)
	assert.NotEmpty(t, artifact.UserHint)
	assert.NotNil(t, artifact.StructureInfo)
	assert.GreaterOrEqual(t, artifact.StructureInfo.HeadingCount, 2)
}

func TestChatDocumentArtifactRegisterFromAssistantMessageSkipsStructuredQAWithoutArtifactSignal(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	message := &types.Message{
		ID:               "message-qa-1",
		SessionID:        "session-1",
		RequestID:        "request-qa-1",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Content:          "# 默认调派规则\n\n## 总流程\n\n" + strings.Repeat("这是普通问答的结构化说明。", 160) + "\n\n## 规则细节\n\n" + strings.Repeat("进一步解释。", 160),
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery: "默认调派规则是什么",
		Operation: types.ChatDocumentOperationCreate,
	})
	require.NoError(t, err)
	assert.Nil(t, artifact)
	assert.Empty(t, repo.createdArtifacts)
}

func TestChatDocumentArtifactRegisterFromAssistantMessageCreatesShortDocumentArtifactWithoutLongDocumentStatus(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	message := &types.Message{
		ID:               "message-short-doc-1",
		SessionID:        "session-1",
		RequestID:        "request-short-doc-1",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Content:          "会议纪要\n\n参会人员：A、B、C。\n\n会议结论：" + strings.Repeat("本次会议确认推进实施计划。", 12),
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery:       "请写一份会议纪要",
		Operation:       types.ChatDocumentOperationCreate,
		NeedArtifact:    true,
		UseLongDocument: false,
	})
	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Empty(t, artifact.DocumentGenerationStatus)
	assert.Equal(t, types.ChatDocumentArtifactStatusAvailable, artifact.Status)
}

func TestRegisterChatDocumentArtifact_PersistsEvidenceRefs(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	evidenceRepo := &chatDocumentEvidenceRefRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo, evidenceRefRepo: evidenceRepo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-1")
	message := &types.Message{
		ID:               "msg-1",
		SessionID:        "sess-1",
		RequestID:        "req-1",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusPartial,
		Content:          "# 方案标题\n\n## 第一章\n\n" + strings.Repeat("章节内容", 600) + "\n\n## 第二章\n\n" + strings.Repeat("更多内容", 600),
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery:                "生成完整技术方案",
		Intent:                   types.ChatDocumentIntentNormal,
		Operation:                types.ChatDocumentOperationCreate,
		OutputMode:               types.ChatDocumentOutputModeFull,
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusContinuing,
		GenerationRunID:          "run-1",
		LocalKnowledgeUsed:       true,
		EvidenceRefs: []types.ChatDocumentEvidenceRef{{
			Query:           "智慧运行 平台架构",
			KnowledgeBaseID: "kb-1",
			KnowledgeID:     "doc-1",
			ChunkID:         "chunk-1",
			SourceTitle:     "智慧运行总体方案",
			Excerpt:         "这是当次生成时绑定的不可变证据摘录。",
			SourceStartAt:   128,
			SourceEndAt:     512,
			Score:           0.91,
			EvidenceType:    types.ChatDocumentEvidenceTypeChunk,
			ContentChecksum: "checksum-1",
		}},
	})

	require.NoError(t, err)
	require.NotNil(t, artifact)
	require.Len(t, evidenceRepo.createdRefs, 1)
	assert.Equal(t, artifact.ID, evidenceRepo.createdRefs[0].ArtifactID)
	assert.Equal(t, "msg-1", evidenceRepo.createdRefs[0].MessageID)
	assert.Equal(t, "run-1", evidenceRepo.createdRefs[0].RunID)
	assert.Equal(t, "这是当次生成时绑定的不可变证据摘录。", evidenceRepo.createdRefs[0].Excerpt)
	assert.Equal(t, 128, evidenceRepo.createdRefs[0].SourceStartAt)
	assert.Equal(t, 512, evidenceRepo.createdRefs[0].SourceEndAt)
	require.Len(t, artifact.EvidenceRefs, 1)
	assert.Equal(t, "chunk-1", artifact.EvidenceRefs[0].ChunkID)
	assert.Equal(t, "智慧运行总体方案", artifact.EvidenceRefs[0].SourceTitle)
	assert.Equal(t, "这是当次生成时绑定的不可变证据摘录。", artifact.EvidenceRefs[0].Excerpt)
	assert.Equal(t, 128, artifact.EvidenceRefs[0].SourceStartAt)
	assert.Equal(t, 512, artifact.EvidenceRefs[0].SourceEndAt)
	require.NotNil(t, artifact.EvidenceSummary)
	assert.Equal(t, 1, artifact.EvidenceSummary.RefCount)
	assert.Equal(t, 1, artifact.EvidenceSummary.KnowledgeBaseCount)
	assert.Equal(t, 1, artifact.EvidenceSummary.KnowledgeCount)
	assert.Equal(t, 1, artifact.EvidenceSummary.ChunkCount)
	require.Len(t, artifact.EvidenceSummary.Sources, 1)
	assert.Equal(t, "智慧运行总体方案", artifact.EvidenceSummary.Sources[0].SourceTitle)
}

func TestNormalizeChatDocumentEvidenceRefs_PreservesExcerptAndSourceSpan(t *testing.T) {
	refs := types.NormalizeChatDocumentEvidenceRefs([]interface{}{
		map[string]interface{}{
			"query":             "智慧运行 平台架构",
			"knowledge_base_id": "kb-1",
			"knowledge_id":      "doc-1",
			"chunk_id":          "chunk-1",
			"source_title":      "智慧运行总体方案",
			"excerpt":           "证据摘录正文",
			"source_start_at":   42,
			"source_end_at":     96,
			"content_checksum":  "checksum-1",
		},
	})

	require.Len(t, refs, 1)
	assert.Equal(t, "证据摘录正文", refs[0].Excerpt)
	assert.Equal(t, 42, refs[0].SourceStartAt)
	assert.Equal(t, 96, refs[0].SourceEndAt)
}

func TestRegisterChatDocumentArtifact_PersistsTranslationMetadata(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-1")
	message := &types.Message{
		ID:               "msg-translation-1",
		SessionID:        "sess-1",
		RequestID:        "req-1",
		Role:             "assistant",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		Content:          strings.Repeat("First paragraph translated from the source document. ", 60) + "\n\n" + strings.Repeat("Second paragraph translated from the source document. ", 60),
	}

	artifact, err := svc.RegisterFromAssistantMessage(ctx, message, types.RegisterChatDocumentArtifactOptions{
		UserQuery:                "请把这篇文档完整翻译成英文",
		Operation:                types.ChatDocumentOperationCreate,
		OutputMode:               types.ChatDocumentOutputModeFull,
		DocumentTaskKind:         types.ChatDocumentTaskKindTranslation,
		SourceTitle:              "原始文档",
		TargetLanguage:           "English",
		TranslationOutputFormat:  "markdown",
		NeedArtifact:             true,
		UseLongDocument:          true,
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusCompleted,
	})

	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Equal(t, types.ChatDocumentTaskKindTranslation, artifact.DocumentTaskKind)
	assert.Equal(t, "原始文档", artifact.SourceTitle)
	assert.Equal(t, "English", artifact.TargetLanguage)
	assert.Equal(t, "markdown", artifact.OutputFormat)
	assert.Equal(t, "原始文档（English译文）", artifact.Title)
	assert.Equal(t, artifact.Title, repo.artifactsByID[artifact.ID].Title)
	assert.Equal(t, artifact.SourceTitle, repo.artifactsByID[artifact.ID].SourceTitle)
	assert.Equal(t, artifact.TargetLanguage, repo.artifactsByID[artifact.ID].TargetLanguage)
}

func TestGetChatDocumentArtifact_HydratesEvidenceRefs(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{
		artifactsByID: map[string]*types.ChatDocumentArtifact{
			"artifact-1": {ID: "artifact-1", TenantID: 1, SessionID: "sess-1", SourceMessageID: "msg-1", ArtifactKind: types.ChatDocumentArtifactKindMarkdown, Status: types.ChatDocumentArtifactStatusAvailable, ContentSnapshot: "# 标题\n\n## 第一章\n\n内容"},
		},
	}
	evidenceRepo := &chatDocumentEvidenceRefRepoStub{
		refsByArtifactID: map[string][]*types.ChatDocumentEvidenceRef{
			"artifact-1": {{ArtifactID: "artifact-1", ChunkID: "chunk-1", Query: "智慧运行", SourceTitle: "智慧运行总体方案", EvidenceType: types.ChatDocumentEvidenceTypeChunk}},
		},
	}
	svc := &chatDocumentArtifactService{repo: repo, evidenceRefRepo: evidenceRepo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	artifact, err := svc.GetArtifact(ctx, "artifact-1")

	require.NoError(t, err)
	require.NotNil(t, artifact)
	require.Len(t, artifact.EvidenceRefs, 1)
	assert.Equal(t, "chunk-1", artifact.EvidenceRefs[0].ChunkID)
	assert.Equal(t, "智慧运行总体方案", artifact.EvidenceRefs[0].SourceTitle)
	require.NotNil(t, artifact.EvidenceSummary)
	assert.Equal(t, 1, artifact.EvidenceSummary.RefCount)
	require.Len(t, artifact.EvidenceSummary.Sources, 1)
	assert.Equal(t, "智慧运行总体方案", artifact.EvidenceSummary.Sources[0].SourceTitle)
}

func TestListChatDocumentArtifacts_StripsContentSnapshotForMetadataView(t *testing.T) {
	repo := &chatDocumentArtifactRepoStub{
		artifactsBySession: map[string][]*types.ChatDocumentArtifact{
			"sess-1": {{
				ID:              "artifact-1",
				TenantID:        1,
				SessionID:       "sess-1",
				SourceMessageID: "msg-1",
				ArtifactKind:    types.ChatDocumentArtifactKindMarkdown,
				Status:          types.ChatDocumentArtifactStatusAvailable,
				ContentSnapshot: "# 标题\n\n## 第一章\n\n正文内容",
			}},
		},
	}
	svc := &chatDocumentArtifactService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	artifacts, err := svc.ListBySession(ctx, "sess-1", 10)

	require.NoError(t, err)
	require.Len(t, artifacts, 1)
	assert.Empty(t, artifacts[0].ContentSnapshot)
	assert.Equal(t, len([]rune("# 标题\n\n## 第一章\n\n正文内容")), artifacts[0].SnapshotCharCount)
}
