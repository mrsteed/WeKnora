package middleware

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// RequireSuperAdmin returns a middleware that checks if the current user is a super admin.
// It must be placed after the Auth middleware so that the user is already set in context.
// Returns 401 if no user in context, 403 if user is not a super admin.
func RequireSuperAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user from context (set by Auth middleware)
		userVal, exists := c.Get(types.UserContextKey.String())
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized: authentication required",
			})
			c.Abort()
			return
		}

		user, ok := userVal.(*types.User)
		if !ok || user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized: invalid user context",
			})
			c.Abort()
			return
		}

		if !user.IsSuperAdmin {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Forbidden: super admin privileges required",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
