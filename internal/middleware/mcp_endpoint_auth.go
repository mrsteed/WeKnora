package middleware

import (
	"context"
	"errors"
	"fmt"
	"strings"

	apperrors "github.com/Tencent/WeKnora/internal/errors"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// MCPEndpointAuth authenticates calls on the workspace MCP server surface
// (/mcp/:endpoint_id). The client presents the endpoint's bearer token; on
// success the request carries the endpoint's workspace, a synthetic machine
// user, an MCP-endpoint principal and an API-key style scope derived from the
// endpoint's tool allowlist and knowledge-base scope, so every downstream
// service applies the same checks it applies to a scoped API key.
func MCPEndpointAuth(svc interfaces.MCPEndpointService, tenantSvc interfaces.TenantService) gin.HandlerFunc {
	return func(c *gin.Context) {
		endpointID := strings.TrimSpace(c.Param("endpoint_id"))
		token := extractMCPEndpointToken(c)
		if endpointID == "" || token == "" {
			c.Header("WWW-Authenticate", `Bearer realm="weknora-mcp"`)
			c.Error(apperrors.NewUnauthorizedError("unauthorized: missing bearer token"))
			c.Abort()
			return
		}
		ep, err := svc.Authenticate(c.Request.Context(), endpointID, token)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrMCPEndpointDisabled):
				c.Error(apperrors.NewForbiddenError("mcp endpoint disabled"))
			case errors.Is(err, service.ErrMCPEndpointTokenInvalid), errors.Is(err, service.ErrMCPEndpointNotFound):
				c.Header("WWW-Authenticate", `Bearer realm="weknora-mcp"`)
				c.Error(apperrors.NewUnauthorizedError("unauthorized: invalid token"))
			default:
				logger.Warnf(c.Request.Context(), "[mcp-endpoint] auth lookup failed: %v", err)
				c.Error(apperrors.NewInternalServerError("authentication unavailable"))
			}
			c.Abort()
			return
		}
		tenant, err := tenantSvc.GetTenantByID(c.Request.Context(), ep.TenantID)
		if err != nil || tenant == nil {
			c.Error(apperrors.NewInternalServerError("workspace unavailable"))
			c.Abort()
			return
		}
		if strings.EqualFold(tenant.Status, "inactive") || strings.EqualFold(tenant.Status, "disabled") {
			c.Error(apperrors.NewTenantInactiveError())
			c.Abort()
			return
		}

		scope := types.MCPEndpointScope(ep)
		user := &types.User{
			ID:       fmt.Sprintf("mcp-%s", ep.ID),
			Username: fmt.Sprintf("mcp-%s", ep.ID),
			Email:    fmt.Sprintf("mcp-%s@mcp.local", ep.ID),
			TenantID: ep.TenantID,
			IsActive: true,
		}
		applyAuthSession(c, authSession{
			User:        user,
			Principal:   types.MCPEndpointPrincipal(ep.TenantID, ep.ID),
			TenantID:    ep.TenantID,
			Tenant:      tenant,
			Role:        types.TenantRoleViewer,
			APIKeyScope: &scope,
			Extra:       map[types.ContextKey]any{types.MCPEndpointContextKey: ep},
		})
		c.Next()
	}
}

// extractMCPEndpointToken reads "Authorization: Bearer mcp_..." only. Query
// string tokens are rejected so they never land in access logs.
func extractMCPEndpointToken(c *gin.Context) string {
	auth := strings.TrimSpace(c.GetHeader("Authorization"))
	if len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}

// MCPEndpointFromContext returns the endpoint authenticated for this request.
func MCPEndpointFromContext(ctx context.Context) (*types.MCPEndpoint, bool) {
	ep, ok := ctx.Value(types.MCPEndpointContextKey).(*types.MCPEndpoint)
	return ep, ok && ep != nil
}
