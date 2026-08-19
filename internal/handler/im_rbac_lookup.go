package handler

import (
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

func currentIMChannelOwnerID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	ownerID, _ := types.UserIDFromContext(c.Request.Context())
	return strings.TrimSpace(ownerID)
}
