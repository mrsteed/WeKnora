package tools

import (
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/models/chat"
)

// SanitizeMessages validates and fixes a message array for LLM compatibility.
// It handles common issues that cause provider API errors:
//   - Ensures no consecutive same-role messages (some providers reject these)
//   - Verifies tool result messages have matching tool_call in the preceding assistant message
//   - Removes empty content messages that can cause API errors
//
// Returns the sanitized message slice (may be shorter than input).
func SanitizeMessages(messages []chat.Message) []chat.Message {
	if len(messages) == 0 {
		return messages
	}

	result := make([]chat.Message, 0, len(messages))
	pendingToolCalls := make(map[string]bool)
	pendingToolMessages := make([]chat.Message, 0)
	pendingAssistantIndex := -1

	closeBrokenPending := func() {
		if pendingAssistantIndex < 0 || len(pendingToolCalls) == 0 {
			pendingToolCalls = make(map[string]bool)
			pendingToolMessages = pendingToolMessages[:0]
			pendingAssistantIndex = -1
			return
		}

		assistantMsg := result[pendingAssistantIndex]
		assistantMsg.ToolCalls = nil
		assistantMsg.Content = appendHistoricalToolOmittedNote(assistantMsg.Content)
		result[pendingAssistantIndex] = assistantMsg
		pendingToolCalls = make(map[string]bool)
		pendingToolMessages = pendingToolMessages[:0]
		pendingAssistantIndex = -1
	}

	appendNonTool := func(msg chat.Message) {
		if len(result) > 0 && msg.Role != "tool" {
			prev := result[len(result)-1]
			if prev.Role == msg.Role && prev.Role != "tool" && len(prev.ToolCalls) == 0 && len(msg.ToolCalls) == 0 {
				result[len(result)-1] = mergeMessages(prev, msg)
				return
			}
		}
		result = append(result, msg)
	}

	for _, msg := range messages {
		// Skip empty non-system messages (some providers reject these)
		if msg.Content == "" && msg.Role != "system" &&
			msg.Role != "tool" && len(msg.ToolCalls) == 0 {
			continue
		}

		switch msg.Role {
		case "tool":
			if pendingAssistantIndex >= 0 && pendingToolCalls[msg.ToolCallID] {
				pendingToolMessages = append(pendingToolMessages, msg)
				delete(pendingToolCalls, msg.ToolCallID)
				if len(pendingToolCalls) == 0 {
					result = append(result, pendingToolMessages...)
					pendingToolMessages = pendingToolMessages[:0]
					pendingAssistantIndex = -1
				}
				continue
			}

			closeBrokenPending()
			result = append(result, orphanedToolResultMessage(msg))

		case "assistant":
			closeBrokenPending()
			msg.ToolCalls = validToolCalls(msg.ToolCalls)
			if shouldSkipEmptyMessage(msg) {
				continue
			}
			appendNonTool(msg)
			if len(msg.ToolCalls) > 0 {
				pendingAssistantIndex = len(result) - 1
				pendingToolCalls = make(map[string]bool, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					pendingToolCalls[tc.ID] = true
				}
			}

		case "user", "system":
			closeBrokenPending()
			appendNonTool(msg)

		default:
			closeBrokenPending()
			appendNonTool(msg)
		}
	}
	closeBrokenPending()

	return result
}

func shouldSkipEmptyMessage(msg chat.Message) bool {
	return msg.Content == "" && msg.Role != "system" && msg.Role != "tool" && len(msg.ToolCalls) == 0
}

// ValidateToolMessageProtocol returns human-readable protocol violations that
// would make an OpenAI-compatible provider reject the message array.
func ValidateToolMessageProtocol(messages []chat.Message) []string {
	var problems []string
	pendingToolCalls := map[string]bool{}
	pendingAssistantIndex := -1

	for i, msg := range messages {
		switch msg.Role {
		case "assistant":
			if len(pendingToolCalls) > 0 {
				problems = append(problems, fmt.Sprintf("assistant message at index %d interrupted pending tool results from assistant index %d", i, pendingAssistantIndex))
			}
			pendingToolCalls = map[string]bool{}
			pendingAssistantIndex = -1
			for _, tc := range msg.ToolCalls {
				if strings.TrimSpace(tc.ID) == "" {
					problems = append(problems, fmt.Sprintf("assistant message at index %d has tool_call with empty id", i))
					continue
				}
				pendingToolCalls[tc.ID] = true
			}
			if len(pendingToolCalls) > 0 {
				pendingAssistantIndex = i
			}

		case "tool":
			if strings.TrimSpace(msg.ToolCallID) == "" {
				problems = append(problems, fmt.Sprintf("tool message at index %d has empty tool_call_id", i))
				continue
			}
			if pendingAssistantIndex < 0 || !pendingToolCalls[msg.ToolCallID] {
				problems = append(problems, fmt.Sprintf("tool message at index %d has no matching pending tool_call_id %q", i, msg.ToolCallID))
				continue
			}
			delete(pendingToolCalls, msg.ToolCallID)
			if len(pendingToolCalls) == 0 {
				pendingAssistantIndex = -1
			}

		case "user", "system":
			if len(pendingToolCalls) > 0 {
				problems = append(problems, fmt.Sprintf("%s message at index %d interrupted pending tool results from assistant index %d", msg.Role, i, pendingAssistantIndex))
			}
			pendingToolCalls = map[string]bool{}
			pendingAssistantIndex = -1
		}
	}

	if len(pendingToolCalls) > 0 {
		problems = append(problems, fmt.Sprintf("assistant message at index %d has incomplete tool results", pendingAssistantIndex))
	}

	return problems
}

// DropInvalidToolProtocolMessages is a last-resort guard for request building.
// It removes all tool-role messages and strips assistant tool_calls so the
// provider never receives an invalid tool-calling protocol sequence.
func DropInvalidToolProtocolMessages(messages []chat.Message) []chat.Message {
	result := make([]chat.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "tool" {
			continue
		}
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			msg.ToolCalls = nil
			msg.Content = appendHistoricalToolOmittedNote(msg.Content)
		}
		result = append(result, msg)
	}
	return result
}

func validToolCalls(toolCalls []chat.ToolCall) []chat.ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	result := make([]chat.ToolCall, 0, len(toolCalls))
	seen := make(map[string]bool, len(toolCalls))
	for _, tc := range toolCalls {
		if strings.TrimSpace(tc.ID) == "" || seen[tc.ID] {
			continue
		}
		result = append(result, tc)
		seen[tc.ID] = true
	}
	return result
}

func orphanedToolResultMessage(msg chat.Message) chat.Message {
	name := strings.TrimSpace(msg.Name)
	if name == "" {
		name = "unknown"
	}
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		content = "(empty tool result)"
	}
	return chat.Message{
		Role:    "system",
		Content: "[Tool result for " + name + " omitted from tool protocol]: " + content,
	}
}

func appendHistoricalToolOmittedNote(content string) string {
	content = strings.TrimSpace(content)
	note := "[Historical tool calls omitted because their tool results were incomplete or invalid.]"
	if content == "" {
		return note
	}
	if strings.Contains(content, note) {
		return content
	}
	return content + "\n" + note
}

func mergeMessages(prev, next chat.Message) chat.Message {
	merged := prev
	merged.Content = mergeText(prev.Content, next.Content)
	merged.ReasoningContent = mergeText(prev.ReasoningContent, next.ReasoningContent)
	if len(merged.MultiContent) == 0 && len(next.MultiContent) > 0 {
		merged.MultiContent = next.MultiContent
	}
	if len(merged.Images) == 0 && len(next.Images) > 0 {
		merged.Images = next.Images
	}
	return merged
}

func mergeText(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	return left + "\n\n" + right
}
