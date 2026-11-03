package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

func tenantPolicyErrorCapture() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		if appErr, ok := c.Errors.Last().Err.(*apperrors.AppError); ok {
			c.JSON(appErr.HTTPCode, gin.H{"error": appErr})
		}
	}
}

type tenantPolicySettingService struct {
	interfaces.SystemSettingService
	enabled bool
}

func (s *tenantPolicySettingService) GetBool(context.Context, string, string, bool) bool {
	return s.enabled
}

func (s *tenantPolicySettingService) GetString(_ context.Context, _ string, _ string, def string) string {
	return def
}

func (s *tenantPolicySettingService) GetInt(_ context.Context, _ string, _ string, def int64) int64 {
	return def
}

type tenantPolicyUserService struct {
	interfaces.UserService
	user *types.User
}

func (s *tenantPolicyUserService) GetCurrentUser(context.Context) (*types.User, error) {
	return s.user, nil
}

func (s *tenantPolicyUserService) BuildLoginMemberships(context.Context, *types.User, *types.Tenant) []types.Membership {
	return []types.Membership{}
}

type tenantPolicyTenantService struct {
	interfaces.TenantService
	createCalls int
}

func (s *tenantPolicyTenantService) CreateTenant(_ context.Context, tenant *types.Tenant) (*types.Tenant, error) {
	s.createCalls++
	tenant.ID = 99
	return tenant, nil
}

// 仅供 /auth/me 的空间管理员用例：让带租户的用户能解析出租户信息而不 panic。
type tenantPolicyAuthTenantService struct {
	interfaces.TenantService
}

func (s *tenantPolicyAuthTenantService) GetTenantByID(_ context.Context, id uint64) (*types.Tenant, error) {
	return &types.Tenant{ID: id, Name: "Space One"}, nil
}

func TestCreateTenantRejectsRegularUserWhenSelfServiceDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenants := &tenantPolicyTenantService{}
	h := &TenantHandler{
		service:          tenants,
		userService:      &tenantPolicyUserService{user: &types.User{ID: "regular-user"}},
		config:           &config.Config{Tenant: &config.TenantConfig{}},
		systemSettingSvc: &tenantPolicySettingService{enabled: false},
	}
	r := gin.New()
	r.Use(tenantPolicyErrorCapture())
	r.POST("/tenants", h.CreateTenant)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tenants", bytes.NewBufferString(`{"name":"blocked"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if tenants.createCalls != 0 {
		t.Fatalf("CreateTenant called %d times, want 0", tenants.createCalls)
	}
	if !strings.Contains(w.Body.String(), `"code":2005`) {
		t.Fatalf("response missing typed disabled code: %s", w.Body.String())
	}
}

func TestCreateTenantAllowsCrossTenantSuperuserWhenSelfServiceDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenants := &tenantPolicyTenantService{}
	h := &TenantHandler{
		service: tenants,
		userService: &tenantPolicyUserService{user: &types.User{
			ID:                  "super-user",
			TenantID:            1,
			CanAccessAllTenants: true,
		}},
		config:           &config.Config{Tenant: &config.TenantConfig{}},
		systemSettingSvc: &tenantPolicySettingService{enabled: false},
	}
	r := gin.New()
	r.Use(errorCapture())
	r.POST("/tenants", h.CreateTenant)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tenants", bytes.NewBufferString(`{"name":"admin-created"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if tenants.createCalls != 1 {
		t.Fatalf("CreateTenant called %d times, want 1", tenants.createCalls)
	}
}

func TestAuthMeProjectsTenantCreationCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AuthHandler{
		userService: &tenantPolicyUserService{user: &types.User{
			ID:       "tenantless-user",
			Username: "tenantless",
			Email:    "tenantless@example.com",
		}},
		configInfo:       &config.Config{Tenant: &config.TenantConfig{}},
		systemSettingSvc: &tenantPolicySettingService{enabled: false},
	}
	r := gin.New()
	r.GET("/auth/me", h.GetCurrentUser)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/me", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"can_create_tenant":false`) {
		t.Fatalf("response missing capability: %s", w.Body.String())
	}
}

// 需求：只有超级管理员 (平台超管) 才能创建空间。
// 部署里的超管由 bootstrap 标记为 IsSystemAdmin，self_service 关闭时也应保留入口。
func TestAuthMeGivesSystemAdminCreateCapabilityWhenSelfServiceDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AuthHandler{
		userService: &tenantPolicyUserService{user: &types.User{
			ID:           "bootstrap-superadmin",
			Username:     "superadmin",
			Email:        "superadmin@example.com",
			IsSystemAdmin: true,
		}},
		configInfo:       &config.Config{Tenant: &config.TenantConfig{}},
		systemSettingSvc: &tenantPolicySettingService{enabled: false},
	}
	r := gin.New()
	r.GET("/auth/me", h.GetCurrentUser)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/me", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"can_create_tenant":true`) {
		t.Fatalf("system admin should get create entry: %s", w.Body.String())
	}
}

// 需求：空间(租户)管理员不能创建新空间——即便 self_service 打开也不得暴露入口。
func TestAuthMeDeniesSpaceAdminCreateCapabilityWhenSelfServiceEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AuthHandler{
		userService: &tenantPolicyUserService{user: &types.User{
			ID:       "space-admin",
			Username: "spaceadmin",
			Email:    "spaceadmin@example.com",
			TenantID: 1,
		}},
		tenantService:    &tenantPolicyAuthTenantService{},
		configInfo:       &config.Config{Tenant: &config.TenantConfig{}},
		systemSettingSvc: &tenantPolicySettingService{enabled: true},
	}
	r := gin.New()
	r.GET("/auth/me", h.GetCurrentUser)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/me", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"can_create_tenant":false`) {
		t.Fatalf("space admin must not get create entry: %s", w.Body.String())
	}
}

// 需求：bootstrap 超管 (IsSystemAdmin) 在 self_service 关闭时仍可调用创建接口。
func TestCreateTenantAllowsSystemAdminWhenSelfServiceDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenants := &tenantPolicyTenantService{}
	h := &TenantHandler{
		service: tenants,
		userService: &tenantPolicyUserService{user: &types.User{
			ID:            "bootstrap-superadmin",
			TenantID:      1,
			IsSystemAdmin: true,
		}},
		config:           &config.Config{Tenant: &config.TenantConfig{}},
		systemSettingSvc: &tenantPolicySettingService{enabled: false},
	}
	r := gin.New()
	r.Use(errorCapture())
	r.POST("/tenants", h.CreateTenant)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tenants", bytes.NewBufferString(`{"name":"admin-created"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if tenants.createCalls != 1 {
		t.Fatalf("CreateTenant called %d times, want 1", tenants.createCalls)
	}
}

// 需求：空间(租户)管理员在 self_service 关闭时被后端拒绝（服务端策略，前端入口仅为体验）。
func TestCreateTenantRejectsSpaceAdminWhenSelfServiceDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenants := &tenantPolicyTenantService{}
	h := &TenantHandler{
		service: tenants,
		userService: &tenantPolicyUserService{user: &types.User{
			ID:       "space-admin",
			TenantID: 1,
		}},
		config:           &config.Config{Tenant: &config.TenantConfig{}},
		systemSettingSvc: &tenantPolicySettingService{enabled: false},
	}
	r := gin.New()
	r.Use(tenantPolicyErrorCapture())
	r.POST("/tenants", h.CreateTenant)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tenants", bytes.NewBufferString(`{"name":"blocked"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if tenants.createCalls != 0 {
		t.Fatalf("CreateTenant called %d times, want 0", tenants.createCalls)
	}
	if !strings.Contains(w.Body.String(), `"code":2005`) {
		t.Fatalf("response missing typed disabled code: %s", w.Body.String())
	}
}
