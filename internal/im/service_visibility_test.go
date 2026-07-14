package im

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const imVisibilityTestDDL = `
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
);
CREATE TABLE custom_agents (
	id TEXT NOT NULL,
	tenant_id INTEGER NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	deleted_at DATETIME,
	PRIMARY KEY (id, tenant_id)
);
`

func setupIMVisibilityTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(imVisibilityTestDDL).Error)
	return db
}

func insertIMChannelRow(t *testing.T, db *gorm.DB, id, creator, name string, enabled bool) {
	t.Helper()
	enabledVal := 0
	if enabled {
		enabledVal = 1
	}
	require.NoError(t, db.Exec(
		`INSERT INTO im_channels (id, tenant_id, agent_id, created_by, platform, name, enabled, mode, output_mode, knowledge_base_id, bot_identity, session_mode, credentials) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, 10008, types.BuiltinSmartReasoningID, creator, "wechat", name, enabledVal, "longpoll", "full", "", "wechat:"+id, "user", `{}`,
	).Error)
}

func TestListChannelsByAgentFiltersByCreator(t *testing.T) {
	db := setupIMVisibilityTestDB(t)
	svc := &Service{db: db}

	insertIMChannelRow(t, db, "ch-1", "user-a", "A", true)
	insertIMChannelRow(t, db, "ch-2", "user-b", "B", true)

	rows, err := svc.ListChannelsByAgent(types.BuiltinSmartReasoningID, 10008, "user-a")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "ch-1", rows[0].ID)
	assert.Equal(t, "user-a", rows[0].CreatedBy)
}

func TestListChannelsByTenantFiltersByCreator(t *testing.T) {
	db := setupIMVisibilityTestDB(t)
	svc := &Service{db: db}

	insertIMChannelRow(t, db, "ch-1", "user-a", "A", true)
	insertIMChannelRow(t, db, "ch-2", "user-b", "B", true)

	rows, err := svc.ListChannelsByTenant(10008, "user-b")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "ch-2", rows[0].ID)
	assert.Equal(t, "B", rows[0].Name)
}

func TestGetChannelByIDAndTenantAndCreator_RejectsOtherCreator(t *testing.T) {
	db := setupIMVisibilityTestDB(t)
	svc := &Service{db: db}

	insertIMChannelRow(t, db, "ch-1", "user-a", "A", true)

	_, err := svc.GetChannelByIDAndTenantAndCreator("ch-1", 10008, "user-b")
	require.Error(t, err)
}

func TestDeleteChannelFiltersByCreator(t *testing.T) {
	db := setupIMVisibilityTestDB(t)
	svc := &Service{db: db, channels: map[string]*channelState{}}

	insertIMChannelRow(t, db, "ch-1", "user-a", "A", false)

	err := svc.DeleteChannel("ch-1", 10008, "user-b")
	require.Error(t, err)

	var count int64
	require.NoError(t, db.Model(&IMChannel{}).Where("id = ?", "ch-1").Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestToggleChannelFiltersByCreator(t *testing.T) {
	db := setupIMVisibilityTestDB(t)
	svc := &Service{db: db, channels: map[string]*channelState{}}

	insertIMChannelRow(t, db, "ch-1", "user-a", "A", false)

	_, err := svc.ToggleChannel("ch-1", 10008, "user-b")
	require.Error(t, err)

	var ch IMChannel
	require.NoError(t, db.Where("id = ?", "ch-1").First(&ch).Error)
	assert.False(t, ch.Enabled)
}