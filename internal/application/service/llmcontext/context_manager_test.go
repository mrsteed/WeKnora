package llmcontext

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type messageRepoStub struct {
	recent []*types.Message
}

func (s *messageRepoStub) CreateMessage(context.Context, *types.Message) (*types.Message, error) {
	return nil, nil
}

func (s *messageRepoStub) GetMessage(context.Context, string, string) (*types.Message, error) {
	return nil, nil
}

func (s *messageRepoStub) GetMessagesBySession(context.Context, string, int, int) ([]*types.Message, error) {
	return nil, nil
}

func (s *messageRepoStub) GetRecentMessagesBySession(context.Context, string, int) ([]*types.Message, error) {
	return s.recent, nil
}

func (s *messageRepoStub) GetMessagesBySessionBeforeTime(context.Context, string, time.Time, int) ([]*types.Message, error) {
	return nil, nil
}

func (s *messageRepoStub) UpdateMessage(context.Context, *types.Message) error {
	return nil
}

func (s *messageRepoStub) UpdateMessageImages(context.Context, string, string, types.MessageImages) error {
	return nil
}

func (s *messageRepoStub) UpdateMessageRenderedContent(context.Context, string, string, string) error {
	return nil
}

func (s *messageRepoStub) DeleteMessage(context.Context, string, string) error {
	return nil
}

func (s *messageRepoStub) DeleteMessagesBySessionID(context.Context, string) error {
	return nil
}

func (s *messageRepoStub) GetFirstMessageOfUser(context.Context, string) (*types.Message, error) {
	return nil, nil
}

func (s *messageRepoStub) SearchMessagesByKeyword(context.Context, uint64, string, []string, int) ([]*types.MessageWithSession, error) {
	return nil, nil
}

func (s *messageRepoStub) GetMessagesByKnowledgeIDs(context.Context, []string) ([]*types.MessageWithSession, error) {
	return nil, nil
}

func (s *messageRepoStub) GetMessagesByRequestIDs(context.Context, []string) ([]*types.MessageWithSession, error) {
	return nil, nil
}

func (s *messageRepoStub) GetKnowledgeIDsBySessionID(context.Context, string) ([]string, error) {
	return nil, nil
}

func (s *messageRepoStub) UpdateMessageKnowledgeID(context.Context, string, string) error {
	return nil
}

var _ interfaces.MessageRepository = (*messageRepoStub)(nil)

func TestContextManagerRebuildFromDB_SkipsFailedAssistantAgentSteps(t *testing.T) {
	createdAt := time.Now().Add(-time.Minute)
	repo := &messageRepoStub{recent: []*types.Message{
		{SessionID: "sess-1", RequestID: "req-1", Role: "user", Content: "first question", CreatedAt: createdAt},
		{SessionID: "sess-1", RequestID: "req-1", Role: "assistant", Content: "failed answer", CompletionStatus: types.MessageCompletionStatusFailed, AgentSteps: types.AgentSteps{{Iteration: 0, Thought: "need tool", ToolCalls: []types.ToolCall{{ID: "tool-1", Name: "wiki_read_page", Result: &types.ToolResult{Success: true, Output: "stale tool result"}}}}}, CreatedAt: createdAt.Add(time.Second)},
		{SessionID: "sess-1", RequestID: "req-2", Role: "user", Content: "second question", CreatedAt: createdAt.Add(2 * time.Second)},
		{SessionID: "sess-1", RequestID: "req-2", Role: "assistant", Content: "good answer", CompletionStatus: types.MessageCompletionStatusCompleted, CreatedAt: createdAt.Add(3 * time.Second)},
	}}
	cm := NewContextManager(NewMemoryStorage(), repo)

	messages, err := cm.GetContext(context.Background(), "sess-1")
	require.NoError(t, err)
	require.Len(t, messages, 2)
	assert.Equal(t, "user", messages[0].Role)
	assert.Equal(t, "second question", messages[0].Content)
	assert.Equal(t, "assistant", messages[1].Role)
	assert.Equal(t, "good answer", messages[1].Content)
	for _, msg := range messages {
		assert.NotEqual(t, "tool", msg.Role)
		assert.NotContains(t, msg.Content, "stale tool result")
	}
}
