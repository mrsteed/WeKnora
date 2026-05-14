package tools

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assertValidToolProtocol(t *testing.T, messages []chat.Message) {
	t.Helper()
	assert.Empty(t, ValidateToolMessageProtocol(messages))
}

func TestSanitizeMessages(t *testing.T) {
	t.Run("normal messages unchanged", func(t *testing.T) {
		messages := []chat.Message{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there"},
		}
		result := SanitizeMessages(messages)
		assert.Len(t, result, 3)
		assertValidToolProtocol(t, result)
	})

	t.Run("consecutive user messages merged", func(t *testing.T) {
		messages := []chat.Message{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "Hello"},
			{Role: "user", Content: "How are you?"},
		}
		result := SanitizeMessages(messages)
		require.Len(t, result, 2) // system + merged user
		assert.Contains(t, result[1].Content, "Hello")
		assert.Contains(t, result[1].Content, "How are you?")
		assertValidToolProtocol(t, result)
	})

	t.Run("consecutive tool messages not merged", func(t *testing.T) {
		messages := []chat.Message{
			{Role: "system", Content: "system"},
			{Role: "assistant", Content: "thinking", ToolCalls: []chat.ToolCall{
				{ID: "call_1"}, {ID: "call_2"},
			}},
			{Role: "tool", Content: "result1", ToolCallID: "call_1"},
			{Role: "tool", Content: "result2", ToolCallID: "call_2"},
		}
		result := SanitizeMessages(messages)
		assert.Len(t, result, 4) // all preserved
		assertValidToolProtocol(t, result)
	})

	t.Run("empty content messages removed and consecutive merged", func(t *testing.T) {
		messages := []chat.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: ""},
			{Role: "user", Content: "bye"},
		}
		result := SanitizeMessages(messages)
		// empty assistant removed → two user messages merge
		assert.Len(t, result, 2)
		assert.Contains(t, result[1].Content, "hello")
		assert.Contains(t, result[1].Content, "bye")
		assertValidToolProtocol(t, result)
	})

	t.Run("empty system message preserved", func(t *testing.T) {
		messages := []chat.Message{
			{Role: "system", Content: ""},
			{Role: "user", Content: "hello"},
		}
		result := SanitizeMessages(messages)
		assert.Len(t, result, 2) // system preserved even if empty
		assertValidToolProtocol(t, result)
	})

	t.Run("orphaned tool result converted", func(t *testing.T) {
		messages := []chat.Message{
			{Role: "system", Content: "system"},
			{Role: "tool", Content: "some result",
				ToolCallID: "nonexistent_id", Name: "search"},
		}
		result := SanitizeMessages(messages)
		require.Len(t, result, 2)
		assert.Equal(t, "system", result[1].Role) // converted
		assert.Contains(t, result[1].Content, "search")
		assertValidToolProtocol(t, result)
	})

	t.Run("tool result without tool call id converted", func(t *testing.T) {
		messages := []chat.Message{
			{Role: "system", Content: "system"},
			{Role: "tool", Content: "some result", Name: "wiki_read_page"},
		}
		result := SanitizeMessages(messages)
		require.Len(t, result, 2)
		assert.Equal(t, "system", result[1].Role)
		assert.Contains(t, result[1].Content, "wiki_read_page")
		assertValidToolProtocol(t, result)
	})

	t.Run("tool result cannot match older completed assistant", func(t *testing.T) {
		messages := []chat.Message{
			{Role: "system", Content: "system"},
			{Role: "assistant", Content: "searching", ToolCalls: []chat.ToolCall{{ID: "call_1"}}},
			{Role: "tool", Content: "result1", ToolCallID: "call_1", Name: "search"},
			{Role: "user", Content: "next"},
			{Role: "tool", Content: "stale result", ToolCallID: "call_1", Name: "search"},
		}
		result := SanitizeMessages(messages)
		require.Len(t, result, 5)
		assert.Equal(t, "system", result[4].Role)
		assert.Contains(t, result[4].Content, "stale result")
		assertValidToolProtocol(t, result)
	})

	t.Run("consecutive assistant merge does not drop tool calls", func(t *testing.T) {
		messages := []chat.Message{
			{Role: "system", Content: "system"},
			{Role: "assistant", Content: "first"},
			{Role: "assistant", Content: "second", ToolCalls: []chat.ToolCall{{ID: "call_1"}}},
			{Role: "tool", Content: "result", ToolCallID: "call_1", Name: "search"},
		}
		result := SanitizeMessages(messages)
		require.Len(t, result, 4)
		assert.Empty(t, result[1].ToolCalls)
		require.Len(t, result[2].ToolCalls, 1)
		assert.Equal(t, "tool", result[3].Role)
		assertValidToolProtocol(t, result)
	})

	t.Run("incomplete multi tool group stripped and partial tool discarded", func(t *testing.T) {
		messages := []chat.Message{
			{Role: "system", Content: "system"},
			{Role: "assistant", Content: "searching", ToolCalls: []chat.ToolCall{{ID: "call_1"}, {ID: "call_2"}}},
			{Role: "tool", Content: "result1", ToolCallID: "call_1", Name: "search"},
			{Role: "user", Content: "next"},
		}
		result := SanitizeMessages(messages)
		require.Len(t, result, 3)
		assert.Equal(t, "assistant", result[1].Role)
		assert.Empty(t, result[1].ToolCalls)
		assert.Contains(t, result[1].Content, "Historical tool calls omitted")
		assert.Equal(t, "user", result[2].Role)
		assertValidToolProtocol(t, result)
	})

	t.Run("unexpected tool id breaks pending group", func(t *testing.T) {
		messages := []chat.Message{
			{Role: "system", Content: "system"},
			{Role: "assistant", Content: "searching", ToolCalls: []chat.ToolCall{{ID: "call_1"}, {ID: "call_2"}}},
			{Role: "tool", Content: "wrong", ToolCallID: "call_3", Name: "search"},
		}
		result := SanitizeMessages(messages)
		require.Len(t, result, 3)
		assert.Empty(t, result[1].ToolCalls)
		assert.Equal(t, "system", result[2].Role)
		assert.Contains(t, result[2].Content, "wrong")
		assertValidToolProtocol(t, result)
	})

	t.Run("assistant with only invalid tool calls removed", func(t *testing.T) {
		messages := []chat.Message{
			{Role: "system", Content: "system"},
			{Role: "assistant", ToolCalls: []chat.ToolCall{{ID: ""}}},
			{Role: "user", Content: "next"},
		}
		result := SanitizeMessages(messages)
		require.Len(t, result, 2)
		assert.Equal(t, "system", result[0].Role)
		assert.Equal(t, "user", result[1].Role)
		assertValidToolProtocol(t, result)
	})

	t.Run("empty slice", func(t *testing.T) {
		result := SanitizeMessages(nil)
		assert.Empty(t, result)
	})
}

func TestValidateToolMessageProtocol(t *testing.T) {
	t.Run("detects empty tool call id", func(t *testing.T) {
		messages := []chat.Message{{Role: "tool", Content: "result", Name: "search"}}
		problems := ValidateToolMessageProtocol(messages)
		require.NotEmpty(t, problems)
		assert.Contains(t, problems[0], "empty tool_call_id")
	})

	t.Run("detects interrupted pending tool calls", func(t *testing.T) {
		messages := []chat.Message{
			{Role: "assistant", Content: "searching", ToolCalls: []chat.ToolCall{{ID: "call_1"}}},
			{Role: "user", Content: "new question"},
		}
		problems := ValidateToolMessageProtocol(messages)
		require.NotEmpty(t, problems)
		assert.Contains(t, problems[0], "interrupted pending tool results")
	})

	t.Run("last resort drop removes all tool protocol fields", func(t *testing.T) {
		messages := []chat.Message{
			{Role: "assistant", Content: "searching", ToolCalls: []chat.ToolCall{{ID: "call_1"}}},
			{Role: "tool", Content: "result", ToolCallID: "wrong", Name: "search"},
		}
		result := DropInvalidToolProtocolMessages(messages)
		assertValidToolProtocol(t, result)
		for _, msg := range result {
			assert.NotEqual(t, "tool", msg.Role)
			assert.Empty(t, msg.ToolCalls)
		}
	})
}
