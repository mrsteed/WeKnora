package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type stubPreferencesUserService struct {
	interfaces.UserService
	getCurrentUser func(ctx context.Context) (*types.User, error)
	updateUser     func(ctx context.Context, user *types.User) error
}

func (s *stubPreferencesUserService) GetCurrentUser(ctx context.Context) (*types.User, error) {
	return s.getCurrentUser(ctx)
}

func (s *stubPreferencesUserService) UpdateUser(ctx context.Context, user *types.User) error {
	return s.updateUser(ctx, user)
}

func newPreferencesTestRouter(h *AuthHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(errorCapture())
	r.PUT("/auth/me/preferences", h.UpdateCurrentUserPreferences)
	return r
}

func TestUpdateCurrentUserPreferences_MergesPatch(t *testing.T) {
	oldMemory := false
	newMemory := true
	oldTenantID := uint64(42)

	currentUser := &types.User{
		ID: "u-1",
		Preferences: types.UserPreferences{
			EnableMemory:       &oldMemory,
			LastActiveTenantID: &oldTenantID,
		},
	}

	var savedUser *types.User
	h := NewAuthHandler(&config.Config{}, &stubPreferencesUserService{
		getCurrentUser: func(context.Context) (*types.User, error) {
			return currentUser, nil
		},
		updateUser: func(_ context.Context, user *types.User) error {
			copied := *user
			savedUser = &copied
			return nil
		},
	}, nil, nil, nil, nil)

	body, err := json.Marshal(types.UserPreferences{EnableMemory: &newMemory})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/auth/me/preferences", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	newPreferencesTestRouter(h).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if savedUser == nil {
		t.Fatalf("UpdateUser should have been called")
	}
	if savedUser.Preferences.EnableMemory == nil || *savedUser.Preferences.EnableMemory != newMemory {
		t.Fatalf("EnableMemory = %v, want %v", savedUser.Preferences.EnableMemory, newMemory)
	}
	if savedUser.Preferences.LastActiveTenantID == nil || *savedUser.Preferences.LastActiveTenantID != oldTenantID {
		t.Fatalf("LastActiveTenantID = %v, want %d", savedUser.Preferences.LastActiveTenantID, oldTenantID)
	}

	var resp struct {
		Success bool                  `json:"success"`
		Data    types.UserPreferences `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("success = false, body=%s", w.Body.String())
	}
	if resp.Data.EnableMemory == nil || *resp.Data.EnableMemory != newMemory {
		t.Fatalf("response enable_memory = %v, want %v", resp.Data.EnableMemory, newMemory)
	}
	if resp.Data.LastActiveTenantID == nil || *resp.Data.LastActiveTenantID != oldTenantID {
		t.Fatalf("response last_active_tenant_id = %v, want %d", resp.Data.LastActiveTenantID, oldTenantID)
	}
}
