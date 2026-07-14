package handler

import (
	stderrors "errors"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// IMChannelCreatorLookup resolves :id -> IMChannel.CreatedBy within the current
// tenant. Unlike KB/Agent ownership lookups, there is intentionally no admin
// override here — IM channels are treated as creator-private resources.
func (h *IMHandler) IMChannelCreatorLookup(c *gin.Context) (string, error) {
	channelID := c.Param("id")
	if channelID == "" {
		return "", stderrors.New("missing :id param for im channel creator lookup")
	}
	tenantID, ok := types.TenantIDFromContext(c.Request.Context())
	if !ok {
		return "", stderrors.New("tenant context missing")
	}
	channel, err := h.imService.GetChannelByIDAndTenant(channelID, tenantID)
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return "", middleware.ErrResourceNotFound
		}
		return "", err
	}
	if channel == nil {
		return "", middleware.ErrResourceNotFound
	}
	if channel.TenantID != tenantID {
		return "", middleware.ErrResourceNotFound
	}
	return channel.CreatedBy, nil
}