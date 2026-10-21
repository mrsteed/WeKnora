package middleware

import (
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/gin-gonic/gin"
)

func abortWithReason(c *gin.Context, appErr *apperrors.AppError, reason string) {
	if appErr == nil {
		c.Abort()
		return
	}
	if reason != "" {
		appErr = appErr.WithDetails(map[string]string{"reason": reason})
	}
	_ = c.Error(appErr)
	c.Abort()
}
