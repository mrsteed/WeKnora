package types

import "testing"

func TestMessageAfterFind_InfersLongDocumentEnabledFromAgentSteps(t *testing.T) {
	message := &Message{
		Role: "assistant",
		AgentSteps: AgentSteps{
			{Iteration: 0, Thought: "正在规划完整文档大纲。", Stage: "planning"},
		},
	}

	if err := message.AfterFind(nil); err != nil {
		t.Fatalf("AfterFind returned error: %v", err)
	}

	if !message.LongDocumentEnabled {
		t.Fatalf("expected LongDocumentEnabled to be true")
	}
}
