package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type stubAgentVisibilityHandlerService struct {
	interfaces.CustomAgentService
	created *types.CustomAgent
	updated *types.CustomAgent
	existing *types.CustomAgent
}

func (s *stubAgentVisibilityHandlerService) CreateAgent(_ context.Context, agent *types.CustomAgent) (*types.CustomAgent, error) {
	s.created = agent
	agent.ID = "agent-new"
	return agent, nil
}

func (s *stubAgentVisibilityHandlerService) GetAgentByID(_ context.Context, _ string) (*types.CustomAgent, error) {
	if s.existing != nil {
		return s.existing, nil
	}
	return nil, nil
}

func (s *stubAgentVisibilityHandlerService) UpdateAgent(_ context.Context, agent *types.CustomAgent) (*types.CustomAgent, error) {
	s.updated = agent
	return agent, nil
}

func newAgentVisibilityRouter(svc interfaces.CustomAgentService, user *types.User) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(1))
		c.Set(types.UserIDContextKey.String(), user.ID)
		c.Set(types.UserContextKey.String(), user)
		c.Next()
	})
	h := &CustomAgentHandler{service: svc}
	r.POST("/agents", h.CreateAgent)
	r.PUT("/agents/:id", h.UpdateAgent)
	return r
}

func TestCreateAgent_GlobalVisibilityAllowedForContributor(t *testing.T) {
	svc := &stubAgentVisibilityHandlerService{}
	user := &types.User{ID: "user-1", IsSuperAdmin: false}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/agents", strings.NewReader(`{"name":"a","visibility":"global"}`))
	req.Header.Set("Content-Type", "application/json")

	newAgentVisibilityRouter(svc, user).ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
	if svc.created == nil || svc.created.Visibility != types.AgentVisibilityGlobal {
		t.Fatalf("expected created agent to use global visibility, got %#v", svc.created)
	}
}

func TestUpdateAgent_GlobalVisibilityAllowedForCreator(t *testing.T) {
	svc := &stubAgentVisibilityHandlerService{existing: &types.CustomAgent{
		ID:        "agent-1",
		Name:      "before",
		CreatedBy: "user-1",
		Visibility: types.AgentVisibilityPrivate,
	}}
	user := &types.User{ID: "user-1", IsSuperAdmin: false}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/agents/agent-1", strings.NewReader(`{"name":"after","visibility":"global"}`))
	req.Header.Set("Content-Type", "application/json")

	newAgentVisibilityRouter(svc, user).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if svc.updated == nil || svc.updated.Visibility != types.AgentVisibilityGlobal {
		t.Fatalf("expected updated agent to switch to global visibility, got %#v", svc.updated)
	}
}