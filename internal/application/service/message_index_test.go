package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shouldIndexMessageToKB(tt.query, tt.answer, tt.options))
		})
	}
}
