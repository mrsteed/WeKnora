package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveKnowledgeBases_AppendsAttachmentTempKB(t *testing.T) {
	service := &sessionService{}
	req := &types.QARequest{
		Session:            &types.Session{TenantID: 10000},
		AttachmentTempKBID: "temp-kb-1",
		CustomAgent: &types.CustomAgent{
			Config: types.CustomAgentConfig{
				RetrieveKBOnlyWhenMentioned: true,
			},
		},
	}

	kbIDs, knowledgeIDs := service.resolveKnowledgeBases(context.Background(), req)
	require.Empty(t, knowledgeIDs)
	assert.Equal(t, []string{"temp-kb-1"}, kbIDs)
}

func TestResolveKnowledgeBases_DoesNotDuplicateAttachmentTempKB(t *testing.T) {
	service := &sessionService{}
	req := &types.QARequest{
		Session:            &types.Session{TenantID: 10000},
		KnowledgeBaseIDs:   []string{"temp-kb-1"},
		AttachmentTempKBID: "temp-kb-1",
	}

	kbIDs, _ := service.resolveKnowledgeBases(context.Background(), req)
	require.Len(t, kbIDs, 1)
	assert.Equal(t, "temp-kb-1", kbIDs[0])
}
