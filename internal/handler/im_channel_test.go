package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	imsvc "github.com/Tencent/WeKnora/internal/im"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const imHandlerTestDDL = `
CREATE TABLE im_channels (
	id TEXT PRIMARY KEY,
	tenant_id INTEGER NOT NULL,
	agent_id TEXT NOT NULL,
	created_by TEXT NOT NULL DEFAULT '',
	platform TEXT NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	enabled INTEGER NOT NULL DEFAULT 1,
	mode TEXT NOT NULL DEFAULT 'websocket',
	output_mode TEXT NOT NULL DEFAULT 'stream',
	knowledge_base_id TEXT DEFAULT '',
	bot_identity TEXT NOT NULL DEFAULT '',
	session_mode TEXT NOT NULL DEFAULT 'user',
	credentials TEXT NOT NULL DEFAULT '{}',
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME
);`

func setupIMHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(imHandlerTestDDL).Error)
	return db
}

func TestCreateIMChannelWritesCreatedBy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupIMHandlerTestDB(t)
	svc := imsvc.NewService(db, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	defer svc.Stop()
	h := NewIMHandler(svc)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "user-created-channel")
		ctx := context.WithValue(c.Request.Context(), types.TenantIDContextKey, uint64(10008))
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.POST("/agents/:id/im-channels", h.CreateIMChannel)

	body := `{"platform":"wechat","name":"微信","enabled":false,"credentials":{}}`
	req := httptest.NewRequest(http.MethodPost, "/agents/builtin-smart-reasoning/im-channels", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var channels []imsvc.IMChannel
	require.NoError(t, db.Find(&channels).Error)
	require.Len(t, channels, 1)
	assert.Equal(t, "user-created-channel", channels[0].CreatedBy)

	var resp struct {
		Data imsvc.IMChannel `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "user-created-channel", resp.Data.CreatedBy)
}

func TestUpdateIMChannelRejectsOtherCreator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupIMHandlerTestDB(t)
	require.NoError(t, db.Exec(`INSERT INTO im_channels (id, tenant_id, agent_id, created_by, platform, name, enabled, mode, output_mode, knowledge_base_id, bot_identity, session_mode, credentials) VALUES ('ch-1', 10008, 'builtin-smart-reasoning', 'creator-a', 'wechat', 'A', 0, 'longpoll', 'full', '', 'wechat:bot-a', 'user', '{}')`).Error)
	svc := imsvc.NewService(db, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	defer svc.Stop()
	h := NewIMHandler(svc)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "creator-b")
		ctx := context.WithValue(c.Request.Context(), types.TenantIDContextKey, uint64(10008))
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.PUT("/im-channels/:id", h.UpdateIMChannel)

	req := httptest.NewRequest(http.MethodPut, "/im-channels/ch-1", strings.NewReader(`{"name":"B"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}