package session

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type streamManagerStub struct {
	events []interfaces.StreamEvent
}

func (s *streamManagerStub) AppendEvent(ctx context.Context, sessionID, messageID string, evt interfaces.StreamEvent) error {
	s.events = append(s.events, evt)
	return nil
}

func (s *streamManagerStub) GetEvents(ctx context.Context, sessionID, messageID string, fromOffset int) ([]interfaces.StreamEvent, int, error) {
	if fromOffset >= len(s.events) {
		return nil, len(s.events), nil
	}
	return s.events[fromOffset:], len(s.events), nil
}

func TestHandleComplete_UsesStreamedAnswerWithoutAppendingCompletePayload(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", time.Time{}, assistant, streamStub, event.NewEventBus(), nil)

	require.NoError(t, handler.handleFinalAnswer(context.Background(), event.Event{
		ID:   "answer-1",
		Data: event.AgentFinalAnswerData{Content: "streamed answer", Done: false},
	}))
	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "should-not-be-appended",
			CompletionStatus: types.MessageCompletionStatusCompleted,
			FinishReason:     "stop",
			TotalDurationMs:  321,
		},
	}))

	assert.Equal(t, "streamed answer", assistant.Content)
	assert.Equal(t, types.MessageCompletionStatusCompleted, assistant.CompletionStatus)
	assert.Equal(t, "stop", assistant.FinishReason)
	assert.True(t, assistant.IsCompleted)

	answerEvents := 0
	completeEvents := 0
	for _, evt := range streamStub.events {
		if evt.Type == types.ResponseTypeAnswer {
			answerEvents++
			assert.NotEqual(t, "should-not-be-appended", evt.Content)
		}
		if evt.Type == types.ResponseTypeComplete {
			completeEvents++
			assert.Equal(t, "streamed answer", evt.Data["final_answer"])
		}
	}
	assert.Equal(t, 1, answerEvents)
	assert.Equal(t, 1, completeEvents)
}

func TestHandleComplete_DoesNotOverrideCancelledAssistantState(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{
		ID:               "msg-1",
		SessionID:        "sess-1",
		Role:             "assistant",
		Content:          "partial content",
		CompletionStatus: types.MessageCompletionStatusCancelled,
		FinishReason:     "cancelled",
		FailureReason:    "cancelled",
	}
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", time.Time{}, assistant, streamStub, event.NewEventBus(), nil)

	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "stale completed answer",
			CompletionStatus: types.MessageCompletionStatusCompleted,
			FinishReason:     "stop",
			AllowIndexing:    true,
			AllowComplete:    true,
		},
	}))

	assert.Equal(t, "partial content", assistant.Content)
	assert.Equal(t, types.MessageCompletionStatusCancelled, assistant.CompletionStatus)
	assert.Equal(t, "cancelled", assistant.FinishReason)
	assert.False(t, assistant.IsCompleted)
	require.Len(t, streamStub.events, 1)
	completeEvent := streamStub.events[0]
	assert.Equal(t, types.ResponseTypeComplete, completeEvent.Type)
	assert.Equal(t, "partial content", completeEvent.Data["final_answer"])
	assert.Equal(t, types.MessageCompletionStatusCancelled, completeEvent.Data["completion_status"])
	assert.Equal(t, "cancelled", completeEvent.Data["finish_reason"])
	assert.Equal(t, false, completeEvent.Data["allow_indexing"])
	assert.Equal(t, false, completeEvent.Data["allow_complete"])
}

func TestHandleError_IgnoresLateErrorAfterTerminalCompletion(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", time.Time{}, assistant, streamStub, event.NewEventBus(), nil)

	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "final answer",
			CompletionStatus: types.MessageCompletionStatusCompleted,
			FinishReason:     "stop",
		},
	}))
	eventCountAfterComplete := len(streamStub.events)
	require.NoError(t, handler.handleError(context.Background(), event.Event{
		ID: "error-1",
		Data: event.ErrorData{
			Stage:     "agent_execution",
			Error:     "late error",
			SessionID: "sess-1",
		},
	}))

	require.Len(t, streamStub.events, eventCountAfterComplete)
	for _, evt := range streamStub.events {
		assert.NotEqual(t, types.ResponseTypeError, evt.Type)
	}
	assert.Equal(t, types.MessageCompletionStatusCompleted, assistant.CompletionStatus)
	assert.Equal(t, "final answer", assistant.Content)
}

func TestHandleThought_PropagatesReplaceSyntheticAndOutlineMetadata(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", time.Time{}, assistant, streamStub, event.NewEventBus(), nil)

	require.NoError(t, handler.handleThought(context.Background(), event.Event{
		ID: "document-edit-progress",
		Data: event.AgentThoughtData{
			Content:        "仍在等待模型输出首个修订片段",
			Iteration:      0,
			Replace:        true,
			Synthetic:      true,
			Stage:          "generating",
			SectionCurrent: 2,
			SectionTotal:   8,
			SectionTitle:   "AR眼镜智能作业系统",
			QueryCurrent:   1,
			QueryTotal:     3,
			ProgressLabel:  "第 2/8 章：AR眼镜智能作业系统 · 检索 1/3",
			Outline: map[string]interface{}{
				"title":    "智慧运行建设方案",
				"sections": []string{"建设目标", "平台架构", "实施保障"},
			},
		},
	}))

	require.Len(t, streamStub.events, 1)
	thoughtEvent := streamStub.events[0]
	assert.Equal(t, types.ResponseTypeThinking, thoughtEvent.Type)
	assert.Equal(t, "仍在等待模型输出首个修订片段", thoughtEvent.Content)
	assert.Equal(t, "document-edit-progress", thoughtEvent.Data["event_id"])
	assert.Equal(t, true, thoughtEvent.Data["replace"])
	assert.Equal(t, true, thoughtEvent.Data["synthetic"])
	assert.Equal(t, "generating", thoughtEvent.Data["stage"])
	assert.Equal(t, 2, thoughtEvent.Data["section_current"])
	assert.Equal(t, 8, thoughtEvent.Data["section_total"])
	assert.Equal(t, "AR眼镜智能作业系统", thoughtEvent.Data["section_title"])
	assert.Equal(t, 1, thoughtEvent.Data["query_current"])
	assert.Equal(t, 3, thoughtEvent.Data["query_total"])
	assert.Equal(t, "第 2/8 章：AR眼镜智能作业系统 · 检索 1/3", thoughtEvent.Data["progress_label"])
	assert.Equal(t, map[string]interface{}{
		"title":    "智慧运行建设方案",
		"sections": []string{"建设目标", "平台架构", "实施保障"},
	}, thoughtEvent.Data["outline"])
}

func TestHandleComplete_FallbackAnswerOnlyForCompletedWithoutStreamedAnswer(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", time.Time{}, assistant, streamStub, event.NewEventBus(), nil)

	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "fallback answer",
			CompletionStatus: types.MessageCompletionStatusCompleted,
			FinishReason:     "stop",
			TotalDurationMs:  time.Second.Milliseconds(),
		},
	}))

	assert.Equal(t, "fallback answer", assistant.Content)

	answerEvents := 0
	completeEvents := 0
	for _, evt := range streamStub.events {
		switch evt.Type {
		case types.ResponseTypeAnswer:
			answerEvents++
		case types.ResponseTypeComplete:
			completeEvents++
			assert.Equal(t, "fallback answer", evt.Data["final_answer"])
		}
	}
	assert.Equal(t, 2, answerEvents)
	assert.Equal(t, 1, completeEvents)
}

func TestHandleComplete_EmitsFallbackAfterPersistenceMarkedCompleted(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{
		ID:               "msg-1",
		SessionID:        "sess-1",
		Role:             "assistant",
		Content:          "fallback answer",
		CompletionStatus: types.MessageCompletionStatusCompleted,
		FinishReason:     "stop",
		IsCompleted:      true,
	}
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", time.Time{}, assistant, streamStub, event.NewEventBus(), nil)

	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "fallback answer",
			CompletionStatus: types.MessageCompletionStatusCompleted,
			FinishReason:     "stop",
			TotalDurationMs:  time.Second.Milliseconds(),
		},
	}))

	answerEvents := 0
	completeEvents := 0
	for _, evt := range streamStub.events {
		switch evt.Type {
		case types.ResponseTypeAnswer:
			answerEvents++
		case types.ResponseTypeComplete:
			completeEvents++
			assert.Equal(t, "fallback answer", evt.Data["final_answer"])
			assert.Equal(t, types.MessageCompletionStatusCompleted, evt.Data["completion_status"])
		}
	}
	assert.Equal(t, 2, answerEvents)
	assert.Equal(t, 1, completeEvents)
}

func TestHandleComplete_PrefersAuthoritativeCompleteAnswerOverPartialStreamedContent(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", time.Time{}, assistant, streamStub, event.NewEventBus(), nil)

	require.NoError(t, handler.handleFinalAnswer(context.Background(), event.Event{
		ID:   "answer-1",
		Data: event.AgentFinalAnswerData{Content: "partial preface", Done: false},
	}))
	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "authoritative full answer",
			CompletionStatus: types.MessageCompletionStatusCompleted,
			FinishReason:     "tool_calls",
			TotalDurationMs:  321,
		},
	}))

	assert.Equal(t, "authoritative full answer", assistant.Content)
	assert.Equal(t, types.MessageCompletionStatusCompleted, assistant.CompletionStatus)
	assert.Equal(t, "tool_calls", assistant.FinishReason)
	assert.True(t, assistant.IsCompleted)

	answerEvents := 0
	completeEvents := 0
	for _, evt := range streamStub.events {
		if evt.Type == types.ResponseTypeAnswer {
			answerEvents++
		}
		if evt.Type == types.ResponseTypeComplete {
			completeEvents++
			assert.Equal(t, "authoritative full answer", evt.Data["final_answer"])
		}
	}
	assert.Equal(t, 1, answerEvents)
	assert.Equal(t, 1, completeEvents)
}

func TestHandleComplete_PrefersAuthoritativeLongDocumentAnswer(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", time.Time{}, assistant, streamStub, event.NewEventBus(), nil)

	require.NoError(t, handler.handleFinalAnswer(context.Background(), event.Event{
		ID:   "answer-1",
		Data: event.AgentFinalAnswerData{Content: "###1.1总体目标\n -事项一", Done: false},
	}))
	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:                "msg-1",
			FinalAnswer:              "### 1.1 总体目标\n\n  - 事项一",
			CompletionStatus:         types.MessageCompletionStatusCompleted,
			FinishReason:             "stop",
			DocumentGenerationStatus: types.ChatDocumentGenerationStatusCompleted,
		},
	}))

	assert.Equal(t, "### 1.1 总体目标\n\n  - 事项一", assistant.Content)
	completeEvent := streamStub.events[len(streamStub.events)-1]
	require.Equal(t, types.ResponseTypeComplete, completeEvent.Type)
	assert.Equal(t, "### 1.1 总体目标\n\n  - 事项一", completeEvent.Data["final_answer"])
}

func TestHandleComplete_PartialDoesNotFallbackFromCompletePayload(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", time.Time{}, assistant, streamStub, event.NewEventBus(), nil)

	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "partial final answer",
			CompletionStatus: types.MessageCompletionStatusPartial,
			FinishReason:     "length",
			FailureReason:    "length",
		},
	}))

	assert.Empty(t, assistant.Content)
	assert.Equal(t, types.MessageCompletionStatusPartial, assistant.CompletionStatus)
	assert.False(t, assistant.IsCompleted)

	answerEvents := 0
	for _, evt := range streamStub.events {
		if evt.Type == types.ResponseTypeAnswer {
			answerEvents++
		}
	}
	assert.Equal(t, 0, answerEvents)
}

func TestHandleComplete_RecoveredPartialFallsBackFromCompletePayload(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", time.Time{}, assistant, streamStub, event.NewEventBus(), nil)

	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "recovered partial final answer",
			CompletionStatus: types.MessageCompletionStatusPartial,
			FinishReason:     "fallback_stop",
			FailureReason:    "",
			TotalDurationMs:  time.Second.Milliseconds(),
		},
	}))

	assert.Equal(t, "recovered partial final answer", assistant.Content)
	assert.Equal(t, types.MessageCompletionStatusPartial, assistant.CompletionStatus)
	assert.Equal(t, "fallback_stop", assistant.FinishReason)
	assert.False(t, assistant.IsCompleted)

	answerEvents := 0
	completeEvents := 0
	for _, evt := range streamStub.events {
		switch evt.Type {
		case types.ResponseTypeAnswer:
			answerEvents++
		case types.ResponseTypeComplete:
			completeEvents++
			assert.Equal(t, "recovered partial final answer", evt.Data["final_answer"])
			assert.Equal(t, types.MessageCompletionStatusPartial, evt.Data["completion_status"])
			assert.Equal(t, "fallback_stop", evt.Data["finish_reason"])
		}
	}
	assert.Equal(t, 2, answerEvents)
	assert.Equal(t, 1, completeEvents)
}

func TestHandleComplete_AppendsAgentStepsToCompletePayload(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", time.Time{}, assistant, streamStub, event.NewEventBus(), nil)

	steps := types.AgentSteps{{
		Iteration: 0,
		Thought:   "first thought",
	}}

	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "final answer",
			CompletionStatus: types.MessageCompletionStatusCompleted,
			FinishReason:     "stop",
			TotalDurationMs:  456,
			AgentSteps:       steps,
			TotalSteps:       len(steps),
		},
	}))

	require.Len(t, streamStub.events, 3)
	completeEvent := streamStub.events[len(streamStub.events)-1]
	require.Equal(t, types.ResponseTypeComplete, completeEvent.Type)
	require.NotNil(t, completeEvent.Data)

	assert.Equal(t, "final answer", completeEvent.Data["final_answer"])
	assert.Equal(t, int64(456), completeEvent.Data["agent_duration_ms"])
	assert.Equal(t, int64(456), completeEvent.Data["total_duration_ms"])
	assert.Equal(t, steps, assistant.AgentSteps)

	streamedSteps, ok := completeEvent.Data["agent_steps"].(types.AgentSteps)
	require.True(t, ok)
	assert.Equal(t, steps, streamedSteps)
}

func TestHandleComplete_IncludesChatDocumentArtifactMetadata(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(
		context.Background(),
		"sess-1",
		"msg-1",
		"req-1",
		time.Time{},
		assistant,
		streamStub,
		event.NewEventBus(),
		func() *types.ChatDocumentArtifact {
			return &types.ChatDocumentArtifact{ID: "artifact-1", SessionID: "sess-1", SourceMessageID: "msg-1", RevisionNo: 2, Title: "技术方案", ArtifactKind: types.ChatDocumentArtifactKindMarkdown, Status: types.ChatDocumentArtifactStatusAvailable, Operation: types.ChatDocumentOperationContinue, DocumentTaskKind: types.ChatDocumentTaskKindTranslation, SourceTitle: "原始文档", TargetLanguage: "English", OutputFormat: "markdown", CanContinueDocument: true, CanInlineContinue: true, QualityIssues: []string{"unclosed_code_fence"}, UserHint: "检测到末尾代码块未闭合，系统已自动补全代码围栏。", ContentSnapshot: "# 完整文档\n\n## 第一章"}
		},
	)

	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "final answer",
			CompletionStatus: types.MessageCompletionStatusCompleted,
			FinishReason:     "stop",
		},
	}))

	completeEvent := streamStub.events[len(streamStub.events)-1]
	require.Equal(t, types.ResponseTypeComplete, completeEvent.Type)
	artifact, ok := completeEvent.Data["chat_document_artifact"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "artifact-1", artifact["id"])
	assert.Equal(t, "msg-1", artifact["source_message_id"])
	assert.Equal(t, types.ChatDocumentOperationContinue, artifact["operation"])
	assert.Equal(t, types.ChatDocumentTaskKindTranslation, artifact["document_task_kind"])
	assert.Equal(t, "原始文档", artifact["source_title"])
	assert.Equal(t, "English", artifact["target_language"])
	assert.Equal(t, "markdown", artifact["output_format"])
	assert.Equal(t, true, artifact["can_continue"])
	assert.Equal(t, true, artifact["can_inline_continue"])
	assert.Equal(t, "检测到末尾代码块未闭合，系统已自动补全代码围栏。", artifact["user_hint"])
	assert.Equal(t, types.ChatDocumentFinalDocumentModeInlineSnapshot, completeEvent.Data["final_document_mode"])
	assert.Equal(t, "artifact-1", completeEvent.Data["final_document_artifact_id"])
	assert.Equal(t, "# 完整文档\n\n## 第一章", completeEvent.Data["final_document"])
}

func TestHandleComplete_PreservesExplicitAutoContinueDecisionWithArtifact(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(
		context.Background(),
		"sess-1",
		"msg-1",
		"req-1",
		time.Time{},
		assistant,
		streamStub,
		event.NewEventBus(),
		func() *types.ChatDocumentArtifact {
			return &types.ChatDocumentArtifact{
				ID:                       "artifact-1",
				SessionID:                "sess-1",
				SourceMessageID:          "msg-1",
				DocumentGenerationStatus: types.ChatDocumentGenerationStatusContinuing,
				Status:                   types.ChatDocumentArtifactStatusPartial,
				ContentSnapshot:          "# 完整文档\n\n## 第一章",
			}
		},
	)

	autoContinueNext := false
	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:                "msg-1",
			FinalAnswer:              "final answer",
			CompletionStatus:         types.MessageCompletionStatusPartial,
			FinishReason:             "llm_timeout_retry_exhausted",
			FailureReason:            "llm_timeout",
			DocumentGenerationStatus: types.ChatDocumentGenerationStatusContinuing,
			AutoContinueNext:         &autoContinueNext,
			AutoContinueReason:       "llm_timeout_retry_exhausted",
		},
	}))

	completeEvent := streamStub.events[len(streamStub.events)-1]
	require.Equal(t, types.ResponseTypeComplete, completeEvent.Type)
	assert.Equal(t, false, completeEvent.Data["auto_continue_next"])
	assert.Equal(t, "llm_timeout_retry_exhausted", completeEvent.Data["auto_continue_reason"])
	assert.Equal(t, "模型响应连续两轮超时，自动续写已停止", completeEvent.Data["auto_continue_reason_message"])
}

func TestHandleComplete_FlattensExtraPayloadFields(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(context.Background(), "sess-1", "msg-1", "req-1", time.Time{}, assistant, streamStub, event.NewEventBus(), nil)

	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "final answer",
			CompletionStatus: types.MessageCompletionStatusCompleted,
			FinishReason:     "stop",
			Extra: map[string]interface{}{
				"generation_run_id": "run-1",
				"effective_kb_ids":  []string{"kb-1"},
			},
		},
	}))

	completeEvent := streamStub.events[len(streamStub.events)-1]
	require.Equal(t, types.ResponseTypeComplete, completeEvent.Type)
	assert.Equal(t, "run-1", completeEvent.Data["generation_run_id"])
	assert.Equal(t, []string{"kb-1"}, completeEvent.Data["effective_kb_ids"])
}

func TestHandleComplete_UsesFetchModeForOversizedArtifactSnapshot(t *testing.T) {
	streamStub := &streamManagerStub{}
	assistant := &types.Message{ID: "msg-1", SessionID: "sess-1", Role: "assistant"}
	handler := NewAgentStreamHandler(
		context.Background(),
		"sess-1",
		"msg-1",
		"req-1",
		time.Time{},
		assistant,
		streamStub,
		event.NewEventBus(),
		func() *types.ChatDocumentArtifact {
			return &types.ChatDocumentArtifact{
				ID:              "artifact-oversized",
				SessionID:       "sess-1",
				SourceMessageID: "msg-1",
				RevisionNo:      4,
				ContentSnapshot: strings.Repeat("超长正文", types.ChatDocumentArtifactInlineContinuationMaxChars/4+10),
			}
		},
	)

	require.NoError(t, handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:        "msg-1",
			FinalAnswer:      "final answer",
			CompletionStatus: types.MessageCompletionStatusCompleted,
			FinishReason:     "stop",
		},
	}))

	completeEvent := streamStub.events[len(streamStub.events)-1]
	require.Equal(t, types.ResponseTypeComplete, completeEvent.Type)
	assert.Equal(t, types.ChatDocumentFinalDocumentModeFetchArtifactSnapshot, completeEvent.Data["final_document_mode"])
	assert.Equal(t, "artifact-oversized", completeEvent.Data["final_document_artifact_id"])
	_, hasInlineDocument := completeEvent.Data["final_document"]
	assert.False(t, hasInlineDocument)
}
