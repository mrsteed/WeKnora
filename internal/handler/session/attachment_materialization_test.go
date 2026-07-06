package session

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type attachmentTempKBStateRepoStub struct {
	state *types.AttachmentTempKBState
}

func (s *attachmentTempKBStateRepoStub) GetAttachmentTempKBState(context.Context, string) *types.AttachmentTempKBState {
	if s.state == nil {
		return &types.AttachmentTempKBState{KnowledgeIDs: []string{}, AttachmentKnowledge: map[string]string{}}
	}
	return s.state
}

func (s *attachmentTempKBStateRepoStub) SaveAttachmentTempKBState(_ context.Context, _ string, state *types.AttachmentTempKBState) {
	s.state = state
}

func (s *attachmentTempKBStateRepoStub) DeleteAttachmentTempKBState(context.Context, string) error {
	return nil
}

func (s *attachmentTempKBStateRepoStub) CleanupExpiredAttachmentTempKBStates(context.Context, time.Duration) (int, error) {
	return 0, nil
}

type attachmentKnowledgeServiceStub struct {
	interfaces.AttachmentKnowledgeService
	readResultCalls int
	storedFileCalls int
}

type attachmentMaterializationModelServiceStub struct {
	interfaces.ModelService
}

func (s *attachmentKnowledgeServiceStub) CreateKnowledgeFromReadResult(
	context.Context,
	string,
	string,
	string,
	string,
	int64,
	string,
	*types.ReadResult,
	[]types.StoredImageReference,
	*types.KnowledgeProcessOverrides,
) (*types.Knowledge, error) {
	s.readResultCalls++
	return &types.Knowledge{ID: "knowledge-read-result", EnableStatus: "enabled"}, nil
}

func (s *attachmentKnowledgeServiceStub) CreateKnowledgeFromStoredFile(
	context.Context,
	string,
	string,
	string,
	string,
	int64,
	string,
	*types.KnowledgeProcessOverrides,
) (*types.Knowledge, error) {
	s.storedFileCalls++
	return &types.Knowledge{ID: "knowledge-stored-file", EnableStatus: "enabled"}, nil
}

type attachmentMaterializationKBServiceStub struct {
	interfaces.KnowledgeBaseService
	templateKB *types.KnowledgeBase
	createdKB  *types.KnowledgeBase
}

func (s *attachmentMaterializationKBServiceStub) GetKnowledgeBaseByID(context.Context, string) (*types.KnowledgeBase, error) {
	if s.createdKB != nil {
		return s.createdKB, nil
	}
	return nil, assert.AnError
}

func (s *attachmentMaterializationKBServiceStub) GetKnowledgeBaseByIDOnly(_ context.Context, id string) (*types.KnowledgeBase, error) {
	if s.templateKB != nil && s.templateKB.ID == id {
		return s.templateKB, nil
	}
	if s.createdKB != nil && s.createdKB.ID == id {
		return s.createdKB, nil
	}
	return nil, assert.AnError
}

func (s *attachmentMaterializationKBServiceStub) CreateKnowledgeBase(_ context.Context, kb *types.KnowledgeBase) (*types.KnowledgeBase, error) {
	cloned := *kb
	cloned.ID = "temp-kb-1"
	s.createdKB = &cloned
	return s.createdKB, nil
}

func TestMaterializeAttachmentsForSession_PrefersReadResultPath(t *testing.T) {
	attachmentSvc := &attachmentKnowledgeServiceStub{}
	handler := &Handler{
		knowledgebaseService:       &attachmentMaterializationKBServiceStub{templateKB: &types.KnowledgeBase{ID: "template-kb", EmbeddingModelID: "embed-1", SummaryModelID: "chat-1", IndexingStrategy: types.DefaultIndexingStrategy()}},
		attachmentKnowledgeService: attachmentSvc,
		attachmentTempKBStateRepo:  &attachmentTempKBStateRepoStub{},
		modelService:               &attachmentMaterializationModelServiceStub{},
	}

	state := handler.materializeAttachmentsForSession(
		context.Background(),
		&types.Session{ID: "session-1"},
		&CreateKnowledgeQARequest{Channel: "web"},
		nil,
		[]string{"template-kb"},
		nil,
		[]*AttachmentProcessResult{{
			Attachment:      &types.MessageAttachment{URL: "local://10000/exports/1.txt", FileName: "1.txt", FileType: ".txt", FileSize: 10},
			MarkdownContent: "全文内容",
		}},
	)

	require.NotNil(t, state)
	assert.Equal(t, 1, attachmentSvc.readResultCalls)
	assert.Equal(t, 0, attachmentSvc.storedFileCalls)
	assert.Equal(t, "temp-kb-1", state.KBID)
	assert.Contains(t, state.KnowledgeIDs, "knowledge-read-result")
}
