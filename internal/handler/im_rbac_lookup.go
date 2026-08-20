package handler

import (
	stderrors "errors"
	"strings"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func currentIMChannelOwnerID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	ownerID, _ := types.UserIDFromContext(c.Request.Context())
	return strings.TrimSpace(ownerID)
}

// IMChannelCreatorLookup resolves :id (an IM channel ID) to the channel's
// recorded creator within the caller's tenant. Used by OwnedIMChannelOnly so
// only the channel owner may mutate or toggle it.
func (h *IMHandler) IMChannelCreatorLookup(c *gin.Context) (string, error) {
	channelID := strings.TrimSpace(c.Param("id"))
	if channelID == "" {
		return "", stderrors.New("missing :id param for IM channel owner lookup")
	}
	ctx := c.Request.Context()
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return "", stderrors.New("workspace context missing")
	}
	if h == nil || h.imService == nil {
		return "", stderrors.New("im service missing")
	}
	ch, err := h.imService.GetChannelByIDAndTenant(channelID, tenantID, "")
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return "", middleware.ErrResourceNotFound
		}
		return "", err
	}
	if ch == nil {
		return "", middleware.ErrResourceNotFound
	}
	return strings.TrimSpace(ch.CreatedBy), nil
}
