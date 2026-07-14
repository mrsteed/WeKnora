package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequireOwnershipOnly_AllowsCreator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enabled := true
	cfg := &config.Config{}
	cfg.Tenant = &config.TenantConfig{EnableRBAC: &enabled}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.UserIDContextKey, "u-1")
		ctx = context.WithValue(ctx, types.TenantRoleContextKey, types.TenantRoleOwner)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.PUT("/res/:id", RequireOwnershipOnly(func(c *gin.Context) (string, error) {
		return "u-1", nil
	}, cfg), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/res/1", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestRequireOwnershipOnly_RejectsNonCreatorEvenIfOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enabled := true
	cfg := &config.Config{}
	cfg.Tenant = &config.TenantConfig{EnableRBAC: &enabled}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.UserIDContextKey, "u-owner")
		ctx = context.WithValue(ctx, types.TenantRoleContextKey, types.TenantRoleOwner)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.PUT("/res/:id", RequireOwnershipOnly(func(c *gin.Context) (string, error) {
		return "u-other", nil
	}, cfg), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/res/1", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)
}