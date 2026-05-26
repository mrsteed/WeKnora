package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAttachPublicShareSessionContextSetsLookupKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	requestCtx := context.WithValue(context.Background(), types.RequestIDContextKey, "req-public-share")
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/public/agent-page-shares/demo/sessions/demo/messages", nil).WithContext(requestCtx)

	handler := &Handler{}
	handler.attachPublicShareSessionContext(ctx, &types.Session{TenantID: 42})

	sessionTenantID, ok := types.SessionTenantIDFromContext(ctx.Request.Context())
	require.True(t, ok)
	require.Equal(t, uint64(42), sessionTenantID)

	tenantID, ok := types.TenantIDFromContext(ctx.Request.Context())
	require.True(t, ok)
	require.Equal(t, uint64(42), tenantID)

	require.Equal(t, uint64(42), ctx.MustGet(types.SessionTenantIDContextKey.String()))
	require.Equal(t, uint64(42), ctx.MustGet(types.TenantIDContextKey.String()))
	require.Equal(t, "req-public-share", ctx.Request.Context().Value(types.RequestIDContextKey))
}

func TestAttachPublicShareSessionContextPreservesExistingTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	requestCtx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(requestCtx)
	ctx.Set(types.TenantIDContextKey.String(), uint64(7))

	handler := &Handler{}
	handler.attachPublicShareSessionContext(ctx, &types.Session{TenantID: 42})

	sessionTenantID, ok := types.SessionTenantIDFromContext(ctx.Request.Context())
	require.True(t, ok)
	require.Equal(t, uint64(42), sessionTenantID)

	tenantID, ok := types.TenantIDFromContext(ctx.Request.Context())
	require.True(t, ok)
	require.Equal(t, uint64(7), tenantID)

	require.Equal(t, uint64(42), ctx.MustGet(types.SessionTenantIDContextKey.String()))
	require.Equal(t, uint64(7), ctx.MustGet(types.TenantIDContextKey.String()))
}
