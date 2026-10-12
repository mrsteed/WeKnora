package handler

import (
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/gin-gonic/gin"
)

func respondHTTPError(c *gin.Context, status int, message string) {
	appErr := apperrors.NewHTTPStatusError(status, message)
	c.AbortWithStatusJSON(appErr.HTTPCode, gin.H{
		"success": false,
		"error": gin.H{
			"code":    appErr.Code,
			"message": appErr.Message,
			"details": appErr.Details,
		},
	})
}
