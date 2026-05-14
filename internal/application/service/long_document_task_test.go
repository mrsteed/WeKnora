package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldDispatchLongDocumentTask(t *testing.T) {
	t.Run("translation full document", func(t *testing.T) {
		req := &types.QARequest{
			Session:            &types.Session{ID: "session-1", TenantID: 1},
			AssistantMessageID: "assistant-1",
			DocumentOutputMode: types.ChatDocumentOutputModeFull,
			DocumentTaskKind:   types.ChatDocumentTaskKindTranslation,
			KnowledgeIDs:       []string{"knowledge-1"},
		}
		assert.True(t, shouldDispatchLongDocumentTask(types.LongDocumentExecutionModeKnowledgeQA, req))
	})

	t.Run("agent full document", func(t *testing.T) {
		req := &types.QARequest{
			Session:            &types.Session{ID: "session-2", TenantID: 1},
			AssistantMessageID: "assistant-2",
			DocumentOutputMode: types.ChatDocumentOutputModeFull,
		}
		assert.True(t, shouldDispatchLongDocumentTask(types.LongDocumentExecutionModeAgentQA, req))
	})

	t.Run("attachments keep inline path", func(t *testing.T) {
		req := &types.QARequest{
			Session:            &types.Session{ID: "session-3", TenantID: 1},
			AssistantMessageID: "assistant-3",
			DocumentOutputMode: types.ChatDocumentOutputModeFull,
			Attachments:        types.MessageAttachments{{FileName: "demo.md"}},
		}
		assert.False(t, shouldDispatchLongDocumentTask(types.LongDocumentExecutionModeAgentQA, req))
	})
}

func TestPopulateLongDocumentContinuationPayload(t *testing.T) {
	payload := map[string]interface{}{
		"document_generation_status": types.ChatDocumentGenerationStatusContinuing,
		"finish_reason":              "section_batch_limit",
		"failure_reason":             "",
		"generation_run_id":          "run-1",
		"translation_progress":       map[string]interface{}{"completed": 1},
	}
	artifact := &types.ChatDocumentArtifact{
		ID:                       "artifact-1",
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusContinuing,
	}

	populateLongDocumentContinuationPayload(payload, artifact, 2)

	assert.Equal(t, longDocumentNextActionContinueAuto, payload["next_action"])
	assert.Equal(t, true, payload["auto_continue_next"])
	assert.Equal(t, true, payload["can_auto_continue"])

	recommended, ok := payload["recommended_request"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "run-1", recommended["generation_run_id"])
	assert.Equal(t, "artifact-1", recommended["base_artifact_id"])
	assert.Equal(t, types.ChatDocumentTaskKindTranslation, recommended["document_task_kind"])
	assert.Equal(t, true, recommended["auto_continue"])
	assert.Equal(t, 3, recommended["auto_continue_round"])
	assert.Equal(t, longDocumentAutoContinuePrompt, recommended["query"])
}

func TestBuildLongDocumentGenerationRunStateUpdate_PreservesProgressCheckpoint(t *testing.T) {
	message := &types.Message{
		CompletionStatus: types.MessageCompletionStatusPartial,
		FinishReason:     "section_batch_limit",
		FailureReason:    "",
	}
	completion := &event.AgentCompleteData{
		CompletionStatus:         types.MessageCompletionStatusPartial,
		FinishReason:             "section_batch_limit",
		FailureReason:            "",
		DocumentGenerationStatus: types.ChatDocumentGenerationStatusContinuing,
	}
	req := &types.QARequest{
		DocumentTaskKind:  types.ChatDocumentTaskKindTranslation,
		AutoContinueRound: 2,
	}
	extra := map[string]interface{}{
		"translation_progress": map[string]interface{}{
			"completed_segments": 2,
			"remaining_segments": 1,
			"next_source_chunk_range": map[string]interface{}{
				"chunk_start_seq": 5,
				"chunk_end_seq":   8,
			},
		},
	}

	state := buildLongDocumentGenerationRunStateUpdate(message, nil, completion, req, extra)

	assert.Equal(t, types.ChatDocumentTaskKindTranslation, state.TaskKind)
	assert.Equal(t, 2, state.AutoContinueRound)
	assert.Equal(t, 2, state.CompletedCount)
	assert.Equal(t, 1, state.RemainingCount)
	assert.Equal(t, 5, state.NextSourceChunkStartSeq)
	assert.Equal(t, 8, state.NextSourceChunkEndSeq)
	assert.Equal(t, types.ChatDocumentGenerationStatusContinuing, state.LastDocumentStatus)
}
