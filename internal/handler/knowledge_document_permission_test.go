package handler

import (
	"context"
	stdErrors "errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type stubDocumentPermissionKnowledgeService struct {
	interfaces.KnowledgeService
	byID map[string]*types.Knowledge
}

func (s *stubDocumentPermissionKnowledgeService) GetKnowledgeByIDOnly(_ context.Context, id string) (*types.Knowledge, error) {
	knowledge, ok := s.byID[id]
	if !ok {
		return nil, stdErrors.New("knowledge not found")
	}
	copy := *knowledge
	return &copy, nil
}

func (s *stubDocumentPermissionKnowledgeService) GetKnowledgeBatch(_ context.Context, _ uint64, ids []string) ([]*types.Knowledge, error) {
	result := make([]*types.Knowledge, 0, len(ids))
	for _, id := range ids {
		knowledge, ok := s.byID[id]
		if !ok {
			continue
		}
		copy := *knowledge
		result = append(result, &copy)
	}
	return result, nil
}

type stubDocumentPermissionChunkService struct {
	interfaces.ChunkService
	byID map[string]*types.Chunk
}

func (s *stubDocumentPermissionChunkService) GetChunkByIDOnly(_ context.Context, id string) (*types.Chunk, error) {
	chunk, ok := s.byID[id]
	if !ok {
		return nil, stdErrors.New("chunk not found")
	}
	copy := *chunk
	return &copy, nil
}

func newKnowledgePermissionContext(userID string, permission types.OrgMemberRole) *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), types.TenantIDContextKey, uint64(1))
	ctx = context.WithValue(ctx, types.UserIDContextKey, userID)
	c.Request = req.WithContext(ctx)
	c.Set(types.TenantIDContextKey.String(), uint64(1))
	c.Set(types.UserIDContextKey.String(), userID)
	c.Set(middleware.KBAccessContextKey, &middleware.KBAccess{
		KnowledgeBase:     &types.KnowledgeBase{ID: "kb-1", TenantID: 1},
		EffectiveTenantID: 1,
		Permission:        permission,
	})
	return c
}

func TestResolveKnowledgeAndValidateDocumentMutation_AllowsUploaderWithViewerPermission(t *testing.T) {
	h := &KnowledgeHandler{kgService: &stubDocumentPermissionKnowledgeService{byID: map[string]*types.Knowledge{
		"k-1": {ID: "k-1", KnowledgeBaseID: "kb-1", TenantID: 1, CreatedBy: "u-owner"},
	}}}

	knowledge, _, err := h.resolveKnowledgeAndValidateDocumentMutation(newKnowledgePermissionContext("u-owner", types.OrgRoleViewer), "k-1")
	if err != nil {
		t.Fatalf("resolveKnowledgeAndValidateDocumentMutation() error = %v", err)
	}
	if knowledge == nil || knowledge.ID != "k-1" {
		t.Fatalf("knowledge = %#v, want ID k-1", knowledge)
	}
}

func TestResolveKnowledgeAndValidateDocumentMutation_DeniesViewerOnOthersDocument(t *testing.T) {
	h := &KnowledgeHandler{kgService: &stubDocumentPermissionKnowledgeService{byID: map[string]*types.Knowledge{
		"k-1": {ID: "k-1", KnowledgeBaseID: "kb-1", TenantID: 1, CreatedBy: "u-other"},
	}}}

	_, _, err := h.resolveKnowledgeAndValidateDocumentMutation(newKnowledgePermissionContext("u-viewer", types.OrgRoleViewer), "k-1")
	if err == nil {
		t.Fatal("expected forbidden error for viewer mutating someone else's document")
	}
}

func TestResolveKnowledgeAndValidateDocumentMutation_AllowsKBAdminOnOthersDocument(t *testing.T) {
	h := &KnowledgeHandler{kgService: &stubDocumentPermissionKnowledgeService{byID: map[string]*types.Knowledge{
		"k-1": {ID: "k-1", KnowledgeBaseID: "kb-1", TenantID: 1, CreatedBy: "u-other"},
	}}}

	knowledge, _, err := h.resolveKnowledgeAndValidateDocumentMutation(newKnowledgePermissionContext("u-admin", types.OrgRoleAdmin), "k-1")
	if err != nil {
		t.Fatalf("resolveKnowledgeAndValidateDocumentMutation() admin error = %v", err)
	}
	if knowledge == nil || knowledge.ID != "k-1" {
		t.Fatalf("knowledge = %#v, want ID k-1", knowledge)
	}
}

func TestResolveKnowledgeBatchAndValidateDocumentMutation_DeniesForeignKnowledgeForViewer(t *testing.T) {
	h := &KnowledgeHandler{kgService: &stubDocumentPermissionKnowledgeService{byID: map[string]*types.Knowledge{
		"k-own":   {ID: "k-own", KnowledgeBaseID: "kb-1", TenantID: 1, CreatedBy: "u-owner"},
		"k-other": {ID: "k-other", KnowledgeBaseID: "kb-1", TenantID: 1, CreatedBy: "u-other"},
	}}}

	_, _, err := h.resolveKnowledgeBatchAndValidateDocumentMutation(
		newKnowledgePermissionContext("u-owner", types.OrgRoleViewer),
		"kb-1",
		[]string{"k-own", "k-other"},
	)
	if err == nil {
		t.Fatal("expected forbidden error for batch mutation containing someone else's document")
	}
}

func TestResolveKnowledgeAndValidateGeneratedQuestionMutation_AllowsUploaderWithViewerPermission(t *testing.T) {
	chunkHandler := &ChunkHandler{
		service: &stubDocumentPermissionChunkService{byID: map[string]*types.Chunk{
			"chunk-1": {ID: "chunk-1", KnowledgeID: "k-1", KnowledgeBaseID: "kb-1"},
		}},
		kgService: &stubDocumentPermissionKnowledgeService{byID: map[string]*types.Knowledge{
			"k-1": {ID: "k-1", KnowledgeBaseID: "kb-1", TenantID: 1, CreatedBy: "u-owner"},
		}},
	}

	knowledge, err := chunkHandler.resolveKnowledgeAndValidateGeneratedQuestionMutation(
		newKnowledgePermissionContext("u-owner", types.OrgRoleViewer),
		"chunk-1",
	)
	if err != nil {
		t.Fatalf("resolveKnowledgeAndValidateGeneratedQuestionMutation() error = %v", err)
	}
	if knowledge == nil || knowledge.ID != "k-1" {
		t.Fatalf("knowledge = %#v, want ID k-1", knowledge)
	}
}

func TestResolveKnowledgeAndValidateGeneratedQuestionMutation_DeniesViewerOnOthersDocument(t *testing.T) {
	chunkHandler := &ChunkHandler{
		service: &stubDocumentPermissionChunkService{byID: map[string]*types.Chunk{
			"chunk-1": {ID: "chunk-1", KnowledgeID: "k-1", KnowledgeBaseID: "kb-1"},
		}},
		kgService: &stubDocumentPermissionKnowledgeService{byID: map[string]*types.Knowledge{
			"k-1": {ID: "k-1", KnowledgeBaseID: "kb-1", TenantID: 1, CreatedBy: "u-other"},
		}},
	}

	_, err := chunkHandler.resolveKnowledgeAndValidateGeneratedQuestionMutation(
		newKnowledgePermissionContext("u-viewer", types.OrgRoleViewer),
		"chunk-1",
	)
	if err == nil {
		t.Fatal("expected forbidden error for deleting generated questions from someone else's document")
	}
}