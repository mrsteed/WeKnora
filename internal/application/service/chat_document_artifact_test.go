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
	assert.Equal(t, types.ChatDocumentIntentContinue, result.Intent)

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

	quoted, err := svc.BuildQuotedContext(context.Background(), artifact, "继续生成", types.ChatDocumentIntentContinue, types.ChatDocumentOutputModeDelta)
	require.NoError(t, err)
	assert.Contains(t, quoted, "document_continuation_context")
	assert.Contains(t, quoted, "artifact-1")
	assert.Contains(t, quoted, "## 方案")
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

	quoted, err := svc.BuildQuotedContext(context.Background(), artifact, "请继续补齐", types.ChatDocumentIntentRevise, types.ChatDocumentOutputModeDelta)
	require.NoError(t, err)
	assert.Contains(t, quoted, "<document_patch>")
	assert.Contains(t, quoted, "<replace heading=\"## 标题\">")
	assert.NotContains(t, quoted, "输出修改后的完整 Markdown 文档")
}

func TestChatDocumentArtifactBuildQuotedContextSkipsOversizedArtifact(t *testing.T) {
	svc := &chatDocumentArtifactService{}
	artifact := &types.ChatDocumentArtifact{
		ID:              "artifact-oversized",
		ArtifactKind:    types.ChatDocumentArtifactKindMarkdown,
		Status:          types.ChatDocumentArtifactStatusAvailable,
		ContentSnapshot: strings.Repeat("章节内容", types.ChatDocumentArtifactInlineContinuationMaxChars/4+10),
	}

	quoted, err := svc.BuildQuotedContext(context.Background(), artifact, "继续生成", types.ChatDocumentIntentContinue, types.ChatDocumentOutputModeDelta)
	require.NoError(t, err)
	assert.Empty(t, quoted)
	assert.False(t, artifact.CanContinue())
	assert.NotEmpty(t, hydrateChatDocumentArtifactDerivedFields(artifact).UserHint)
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
		Content:          "# 超长方案\n\n## 内容\n\n" + strings.Repeat("超长内容段落。", 20000),
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
