package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldIndexMessageToKB_RejectsNonCompletedOrPlanningContent(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		answer  string
		options interfaces.MessageIndexOptions
		want    bool
	}{
		{
			name:   "completed answer allowed",
			query:  "what happened",
			answer: "final answer",
			options: interfaces.MessageIndexOptions{
				CompletionStatus: "completed",
				FinishReason:     "stop",
				AllowIndexing:    true,
			},
			want: true,
		},
		{
			name:   "partial length blocked",
			query:  "translate",
			answer: "partial answer",
			options: interfaces.MessageIndexOptions{
				CompletionStatus: "partial",
				FinishReason:     "length",
				AllowIndexing:    false,
			},
			want: false,
		},
		{
			name:   "planning text blocked",
			query:  "translate",
			answer: "让我整理后继续输出完整译文。",
			options: interfaces.MessageIndexOptions{
				CompletionStatus: "completed",
				FinishReason:     "stop",
				AllowIndexing:    true,
			},
			want: false,
		},
		{
			name:   "long document completed allowed",
			query:  "请输出完整技术方案",
			answer: "# 技术方案\n\n## 第一章\n\n正文",
			options: interfaces.MessageIndexOptions{
				CompletionStatus:         "completed",
				FinishReason:             "stop",
				AllowIndexing:            true,
				TaskKind:                 messageIndexTaskKindLongDocument,
				DocumentGenerationStatus: types.ChatDocumentGenerationStatusCompleted,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shouldIndexMessageToKB(tt.query, tt.answer, tt.options))
		})
	}
}

func TestBuildMessageIndexPassages_UsesStructuredSummaryForLongDocument(t *testing.T) {
	passages := buildMessageIndexPassages(
		"这是一份提交给甲方的投标技术方案，请输出完整技术方案。",
		"# 北海电厂二期智慧电厂项目技术方案\n\n## 项目背景与建设目标\n\n### 3.1 全域数据湖建设\n\n建设内容覆盖统一汇聚、治理、算力调度与应用支撑能力。\n\n## 实施与运维保障\n\n通过分阶段上线和运维体系保障项目落地。",
		"msg-1",
		"sess-1",
		interfaces.MessageIndexOptions{
			CompletionStatus:         "completed",
			FinishReason:             "stop",
			AllowIndexing:            true,
			TaskKind:                 messageIndexTaskKindLongDocument,
			DocumentGenerationStatus: types.ChatDocumentGenerationStatusCompleted,
			ArtifactID:               "artifact-1",
			DocumentTitle:            "北海电厂二期智慧电厂项目技术方案",
			DocumentSections:         []string{"项目背景与建设目标", "实施与运维保障"},
		},
	)
	require.Len(t, passages, 1)
	assert.Contains(t, passages[0], "Type: long_document")
	assert.Contains(t, passages[0], "Title: 北海电厂二期智慧电厂项目技术方案")
	assert.Contains(t, passages[0], "Outline: 项目背景与建设目标；实施与运维保障")
	assert.Contains(t, passages[0], "Artifact ID: artifact-1")
	assert.Contains(t, passages[0], "Summary: 建设内容覆盖统一汇聚、治理、算力调度与应用支撑能力。 通过分阶段上线和运维体系保障项目落地。")
	assert.NotContains(t, passages[0], "\nA: # 北海电厂二期智慧电厂项目技术方案")
	assert.LessOrEqual(t, len([]rune(passages[0])), messageIndexLongDocumentPassageMaxRunes)
}

func TestMessageIndexSkipReason_ExplainsWhyIndexingWasSkipped(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		answer  string
		options interfaces.MessageIndexOptions
		want    string
	}{
		{
			name:   "allow indexing disabled",
			query:  "query",
			answer: "answer",
			options: interfaces.MessageIndexOptions{
				AllowIndexing: false,
			},
			want: "allow_indexing_disabled",
		},
		{
			name:   "partial completion",
			query:  "query",
			answer: "answer",
			options: interfaces.MessageIndexOptions{
				AllowIndexing:    true,
				CompletionStatus: "partial",
			},
			want: "completion_status_partial",
		},
		{
			name:   "planning content",
			query:  "query",
			answer: "让我整理后继续输出完整内容。",
			options: interfaces.MessageIndexOptions{
				AllowIndexing:    true,
				CompletionStatus: "completed",
			},
			want: "planning_content",
		},
		{
			name:   "completed answer allowed",
			query:  "query",
			answer: "final answer",
			options: interfaces.MessageIndexOptions{
				AllowIndexing:    true,
				CompletionStatus: "completed",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, messageIndexSkipReason(tt.query, tt.answer, tt.options))
		})
	}
}
