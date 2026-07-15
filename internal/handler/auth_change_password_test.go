package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// stubChangePasswordUserService 镜像 stubPreferencesUserService
// 的最小实现，只覆盖 ChangePassword 链路需要的 GetCurrentUser / ChangePassword
// 两个方法；其它方法会自动从 UserService 嵌入接口拿到 nil，触发 panic 即失败。
type stubChangePasswordUserService struct {
	interfaces.UserService
	getCurrentUser func(ctx context.Context) (*types.User, error)
	changePassword func(ctx context.Context, userID string, oldPassword, newPassword string) error
}

func (s *stubChangePasswordUserService) GetCurrentUser(ctx context.Context) (*types.User, error) {
	return s.getCurrentUser(ctx)
}

func (s *stubChangePasswordUserService) ChangePassword(
	ctx context.Context,
	userID string,
	oldPassword string,
	newPassword string,
) error {
	return s.changePassword(ctx, userID, oldPassword, newPassword)
}

func newChangePasswordTestRouter(h *AuthHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(errorCapture())
	r.POST("/auth/change-password", h.ChangePassword)
	return r
}

// TestChangePassword_RejectsPasswordsShorterThanEight 在 gin binding 层
// 锁住 ≥8 位硬规约；任何把 handler 改回 min=6 的 PR 都会被这个测试挡住。
func TestChangePassword_RejectsPasswordsShorterThanEight(t *testing.T) {
	for _, pwd := range []string{"short1!", "1234567", "abc1", ""} {
		pwd := pwd
		t.Run("len="+pwd, func(t *testing.T) {
			var changeCalled bool
			h := NewAuthHandler(&config.Config{}, &stubChangePasswordUserService{
				getCurrentUser: func(context.Context) (*types.User, error) {
					return &types.User{ID: "u-1", Username: "u-1"}, nil
				},
				changePassword: func(_ context.Context, _, oldPwd, _ string) error {
					changeCalled = true
					if oldPwd == "" {
						return errors.New("old password missing")
					}
					return nil
				},
			}, nil, nil, nil, nil, nil)

			body, err := json.Marshal(map[string]string{
				"old_password": "old-secret-12",
				"new_password": pwd,
			})
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}
			req := httptest.NewRequest(http.MethodPost, "/auth/change-password", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			newChangePasswordTestRouter(h).ServeHTTP(w, req)

			if w.Code == http.StatusOK {
				t.Fatalf("len=%d should be rejected; body=%s", len(pwd), w.Body.String())
			}
			if !strings.Contains(strings.ToLower(w.Body.String()), "password") {
				t.Fatalf("expected rejection body to mention password; got %s", w.Body.String())
			}
			if changeCalled {
				t.Fatalf("service ChangePassword should not have run for len=%d", len(pwd))
			}
		})
	}
}

// TestChangePassword_RejectsPasswordsLongerThanThirtyTwo 同样在 binding 层
// 锁住 ≤32 位 + ≥33 位拒掉；handler 现在是 min=8,max=32。
func TestChangePassword_RejectsPasswordsLongerThanThirtyTwo(t *testing.T) {
	longPwd := strings.Repeat("a", 33) + "1"
	body, err := json.Marshal(map[string]string{
		"old_password": "old-secret-12",
		"new_password": longPwd,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/change-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	var changeCalled bool
	h := NewAuthHandler(&config.Config{}, &stubChangePasswordUserService{
		getCurrentUser: func(context.Context) (*types.User, error) {
			return &types.User{ID: "u-1", Username: "u-1"}, nil
		},
		changePassword: func(context.Context, string, string, string) error {
			changeCalled = true
			return nil
		},
	}, nil, nil, nil, nil, nil)

	newChangePasswordTestRouter(h).ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Fatalf("33+1 char password should be rejected; body=%s", w.Body.String())
	}
	if changeCalled {
		t.Fatal("service ChangePassword should not have run for oversized password")
	}
}

// TestChangePassword_AcceptsEightCharPassword 保证“刚好 8 位”命中分支
// 仍然落到 service.ChangePassword，对前端弹框正则 / handler binding 起到
// 双向回归保护。
func TestChangePassword_AcceptsEightCharPassword(t *testing.T) {
	body, err := json.Marshal(map[string]string{
		"old_password": "old-secret-12",
		"new_password": "abcd1234", // 8 位，含字母与数字
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/change-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	var gotNewPassword string
	h := NewAuthHandler(&config.Config{}, &stubChangePasswordUserService{
		getCurrentUser: func(context.Context) (*types.User, error) {
			return &types.User{ID: "u-1", Username: "u-1"}, nil
		},
		changePassword: func(_ context.Context, _, _, newPwd string) error {
			gotNewPassword = newPwd
			return nil
		},
	}, nil, nil, nil, nil, nil)

	newChangePasswordTestRouter(h).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("8-char password should be accepted; status=%d body=%s", w.Code, w.Body.String())
	}
	if gotNewPassword != "abcd1234" {
		t.Fatalf("service received %q, want abcd1234", gotNewPassword)
	}
}
