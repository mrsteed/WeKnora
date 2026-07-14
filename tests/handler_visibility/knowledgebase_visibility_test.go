package handler_visibility_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type stubKnowledgeBaseService struct {
	interfaces.KnowledgeBaseService
	created  *types.KnowledgeBase
	updated  *types.KnowledgeBase
	existing *types.KnowledgeBase
}

func (s *stubKnowledgeBaseService) CreateKnowledgeBase(_ context.Context, kb *types.KnowledgeBase) (*types.KnowledgeBase, error) {
	s.created = kb
	kb.ID = "kb-new"
	return kb, nil
}

func (s *stubKnowledgeBaseService) GetKnowledgeBaseByID(_ context.Context, _ string) (*types.KnowledgeBase, error) {
	if s.existing != nil {
		return s.existing, nil
	}
	return nil, nil
}

func (s *stubKnowledgeBaseService) UpdateKnowledgeBase(
	_ context.Context,
	_ string,
	name string,
	description string,
	_ *types.KnowledgeBaseConfig,
	visibility string,
	organizationID string,
) (*types.KnowledgeBase, error) {
	s.updated = &types.KnowledgeBase{
		Name:           name,
		Description:    description,
		Visibility:     visibility,
		OrganizationID: organizationID,
	}
	return s.updated, nil
}

func newKBRouter(svc interfaces.KnowledgeBaseService, user *types.User) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(1))
		c.Set(types.UserIDContextKey.String(), user.ID)
		c.Set(types.UserContextKey.String(), user)
		c.Next()
	})
	h := handler.NewKnowledgeBaseHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil)
	r.POST("/knowledge-bases", h.CreateKnowledgeBase)
	r.PUT("/knowledge-bases/:id", h.UpdateKnowledgeBase)
	return r
}

func TestCreateKnowledgeBase_GlobalVisibilityAllowedForContributor(t *testing.T) {
	svc := &stubKnowledgeBaseService{}
	user := &types.User{ID: "user-1", IsSuperAdmin: false}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/knowledge-bases", strings.NewReader(`{"name":"kb","visibility":"global"}`))
	req.Header.Set("Content-Type", "application/json")

	newKBRouter(svc, user).ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
	if svc.created == nil || svc.created.Visibility != types.KBVisibilityGlobal {
		t.Fatalf("expected created KB to use global visibility, got %#v", svc.created)
	}
}

func TestUpdateKnowledgeBase_GlobalPromotionAllowedForCreator(t *testing.T) {
	svc := &stubKnowledgeBaseService{existing: &types.KnowledgeBase{
		ID:         "kb-1",
		Name:       "before",
		TenantID:   1,
		CreatedBy:  "user-1",
		Visibility: types.KBVisibilityPrivate,
	}}
	user := &types.User{ID: "user-1", IsSuperAdmin: false}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/knowledge-bases/kb-1", strings.NewReader(`{"name":"after","visibility":"global"}`))
	req.Header.Set("Content-Type", "application/json")

	newKBRouter(svc, user).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if svc.updated == nil || svc.updated.Visibility != types.KBVisibilityGlobal {
		t.Fatalf("expected updated KB to switch to global visibility, got %#v", svc.updated)
	}
}

func TestUpdateKnowledgeBase_ExistingGlobalVisibilityCanStillBeEdited(t *testing.T) {
	svc := &stubKnowledgeBaseService{existing: &types.KnowledgeBase{
		ID:         "kb-1",
		Name:       "before",
		TenantID:   1,
		CreatedBy:  "user-1",
		Visibility: types.KBVisibilityGlobal,
	}}
	user := &types.User{ID: "user-1", IsSuperAdmin: false}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/knowledge-bases/kb-1", strings.NewReader(`{"name":"after","visibility":"global"}`))
	req.Header.Set("Content-Type", "application/json")

	newKBRouter(svc, user).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if svc.updated == nil || svc.updated.Visibility != types.KBVisibilityGlobal {
		t.Fatalf("expected updated KB to retain global visibility, got %#v", svc.updated)
	}
}