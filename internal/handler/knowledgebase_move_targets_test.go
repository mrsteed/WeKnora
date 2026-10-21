package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type stubMoveTargetsKBService struct {
	interfaces.KnowledgeBaseService
	byID map[string]*types.KnowledgeBase
	list []*types.KnowledgeBase
}

func (s *stubMoveTargetsKBService) GetKnowledgeBaseByID(_ context.Context, id string) (*types.KnowledgeBase, error) {
	if kb, ok := s.byID[id]; ok {
		return kb, nil
	}
	return nil, repository.ErrKnowledgeBaseNotFound
}

func (s *stubMoveTargetsKBService) ListKnowledgeBases(context.Context) ([]*types.KnowledgeBase, error) {
	return s.list, nil
}

type moveTargetsKBVisibilityStub struct {
	accessible []*types.KnowledgeBase
	manageByKB map[string]bool
}

func (s moveTargetsKBVisibilityStub) ListAccessibleKBs(context.Context, string, uint64, bool) ([]*types.KnowledgeBase, error) {
	return s.accessible, nil
}

func (s moveTargetsKBVisibilityStub) CanManageKB(_ context.Context, _ string, _ uint64, kbID string, _ bool) (bool, error) {
	return s.manageByKB[kbID], nil
}

func (s moveTargetsKBVisibilityStub) CanAccessKB(context.Context, string, uint64, string, bool) (bool, error) {
	return false, nil
}

func newMoveTargetsRouter(
	t *testing.T,
	svc interfaces.KnowledgeBaseService,
	vis interfaces.KBVisibilityService,
	withAPIKeyScope *types.TenantAPIKeyScope,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(1))
		c.Set(types.UserIDContextKey.String(), "u-test")
		if withAPIKeyScope != nil {
			c.Request = c.Request.WithContext(contextWithTenantAPIKeyScope(c.Request.Context(), *withAPIKeyScope))
		}
		c.Next()
	})
	h := &KnowledgeBaseHandler{service: svc, kbVisibility: vis}
	r.GET("/knowledge-bases/:id/move-targets", h.ListMoveTargets)
	return r
}

func decodeMoveTargetsBody(t *testing.T, w *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	var envelope struct {
		Success bool                   `json:"success"`
		Data    []map[string]any       `json:"data"`
		Error   map[string]interface{} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	return envelope.Data
}

func TestListMoveTargets_FiltersToManageableVisibleKBs(t *testing.T) {
	source := &types.KnowledgeBase{ID: "kb-src", TenantID: 1, Type: "document", EmbeddingModelID: "m1"}
	kbOK := &types.KnowledgeBase{ID: "kb-ok", TenantID: 1, Type: "document", EmbeddingModelID: "m1", KnowledgeCount: 3}
	kbNoManage := &types.KnowledgeBase{ID: "kb-no-manage", TenantID: 1, Type: "document", EmbeddingModelID: "m1"}
	kbWrongType := &types.KnowledgeBase{ID: "kb-faq", TenantID: 1, Type: "faq", EmbeddingModelID: "m1"}
	kbWrongModel := &types.KnowledgeBase{ID: "kb-m2", TenantID: 1, Type: "document", EmbeddingModelID: "m2"}
	kbTemp := &types.KnowledgeBase{ID: "kb-temp", TenantID: 1, Type: "document", EmbeddingModelID: "m1", IsTemporary: true}

	router := newMoveTargetsRouter(t,
		&stubMoveTargetsKBService{byID: map[string]*types.KnowledgeBase{"kb-src": source}},
		moveTargetsKBVisibilityStub{
			accessible: []*types.KnowledgeBase{source, kbOK, kbNoManage, kbWrongType, kbWrongModel, kbTemp},
			manageByKB: map[string]bool{
				"kb-src":       true,
				"kb-ok":        true,
				"kb-no-manage": false,
				"kb-faq":       true,
				"kb-m2":        true,
				"kb-temp":      true,
			},
		},
		nil,
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge-bases/kb-src/move-targets", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	rows := decodeMoveTargetsBody(t, w)
	if len(rows) != 1 {
		t.Fatalf("expected 1 target, got %d body=%s", len(rows), w.Body.String())
	}
	if got := rows[0]["id"]; got != "kb-ok" {
		t.Fatalf("target id = %v, want kb-ok", got)
	}
}

func TestListMoveTargets_RejectsSourceWithoutManagePermission(t *testing.T) {
	source := &types.KnowledgeBase{ID: "kb-src", TenantID: 1, Type: "document", EmbeddingModelID: "m1"}
	router := newMoveTargetsRouter(t,
		&stubMoveTargetsKBService{byID: map[string]*types.KnowledgeBase{"kb-src": source}},
		moveTargetsKBVisibilityStub{
			accessible: []*types.KnowledgeBase{source},
			manageByKB: map[string]bool{"kb-src": false},
		},
		nil,
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge-bases/kb-src/move-targets", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestListMoveTargets_APIKeyScopedToAllowList(t *testing.T) {
	source := &types.KnowledgeBase{ID: "kb-src", TenantID: 1, Type: "document", EmbeddingModelID: "m1"}
	kbAllowed := &types.KnowledgeBase{ID: "kb-allowed", TenantID: 1, Type: "document", EmbeddingModelID: "m1"}
	kbBlocked := &types.KnowledgeBase{ID: "kb-blocked", TenantID: 1, Type: "document", EmbeddingModelID: "m1"}
	kbWrongType := &types.KnowledgeBase{ID: "kb-faq", TenantID: 1, Type: "faq", EmbeddingModelID: "m1"}
	scope := &types.TenantAPIKeyScope{KeyID: 7, KnowledgeBaseIDs: types.StringArray{"kb-src", "kb-allowed"}}

	router := newMoveTargetsRouter(t,
		&stubMoveTargetsKBService{
			byID: map[string]*types.KnowledgeBase{"kb-src": source},
			list: []*types.KnowledgeBase{source, kbAllowed, kbBlocked, kbWrongType},
		},
		nil,
		scope,
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge-bases/kb-src/move-targets", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	rows := decodeMoveTargetsBody(t, w)
	if len(rows) != 1 {
		t.Fatalf("expected 1 target, got %d body=%s", len(rows), w.Body.String())
	}
	if got := rows[0]["id"]; got != "kb-allowed" {
		t.Fatalf("target id = %v, want kb-allowed", got)
	}
}
