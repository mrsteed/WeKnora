package chat

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitContentAndReasoning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		input         string
		wantContent   string
		wantReasoning string
	}{
		{
			name:          "plain content keeps visible text",
			input:         "final answer",
			wantContent:   "final answer",
			wantReasoning: "",
		},
		{
			name:          "leading think block is split",
			input:         "<think>step 1\nstep 2</think>final answer",
			wantContent:   "final answer",
			wantReasoning: "step 1\nstep 2",
		},
		{
			name:          "multiple think blocks are merged",
			input:         "<think>first</think>answer<think>second</think>",
			wantContent:   "answer",
			wantReasoning: "first\n\nsecond",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, reasoning := SplitContentAndReasoning(tt.input)
			assert.Equal(t, tt.wantContent, content)
			assert.Equal(t, tt.wantReasoning, reasoning)
		})
	}
}

func TestConvertMessages_PropagatesReasoningContent(t *testing.T) {
	t.Parallel()

	chatClient := newTestRemoteChat(t)
	converted := chatClient.ConvertMessages([]Message{{
		Role:             "assistant",
		Content:          "final answer",
		ReasoningContent: "hidden reasoning",
	}})

	assert.Len(t, converted, 1)
	assert.Equal(t, "final answer", converted[0].Content)
	assert.Equal(t, "hidden reasoning", converted[0].ReasoningContent)
}
