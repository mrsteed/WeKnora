package im

import (
	"fmt"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newOwnerScopeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:im-owner-scope-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE im_channels (
		id TEXT PRIMARY KEY,
		tenant_id INTEGER NOT NULL,
		agent_id TEXT NOT NULL,
		platform TEXT NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		enabled NUMERIC NOT NULL DEFAULT 1,
		mode TEXT NOT NULL DEFAULT 'websocket',
		output_mode TEXT NOT NULL DEFAULT 'stream',
		knowledge_base_id TEXT DEFAULT '',
		bot_identity TEXT NOT NULL DEFAULT '',
		session_mode TEXT NOT NULL DEFAULT 'user',
		credentials TEXT NOT NULL DEFAULT '{}',
		created_by TEXT NOT NULL DEFAULT '',
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create im_channels: %v", err)
	}
	if err := db.Exec(`CREATE TABLE custom_agents (
		id TEXT NOT NULL,
		tenant_id INTEGER NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		deleted_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create custom_agents: %v", err)
	}
	return db
}

func insertOwnerScopeAgent(t *testing.T, db *gorm.DB, id string, tenantID uint64, name string) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO custom_agents(id, tenant_id, name, deleted_at) VALUES (?, ?, ?, NULL)`,
		id, tenantID, name,
	).Error; err != nil {
		t.Fatalf("insert custom agent: %v", err)
	}
}

func insertOwnerScopeChannel(t *testing.T, db *gorm.DB, ch IMChannel) {
	t.Helper()
	if ch.Mode == "" {
		ch.Mode = "webhook"
	}
	if ch.OutputMode == "" {
		ch.OutputMode = "full"
	}
	if ch.SessionMode == "" {
		ch.SessionMode = string(SessionModeUser)
	}
	if ch.Credentials == nil {
		ch.Credentials = types.JSON(`{}`)
	}
	if err := db.Create(&ch).Error; err != nil {
		t.Fatalf("insert im channel: %v", err)
	}
}

func TestListChannelsByAgentFiltersByCreator(t *testing.T) {
	db := newOwnerScopeTestDB(t)
	svc := &Service{db: db}
	insertOwnerScopeChannel(t, db, IMChannel{ID: "ch-a", TenantID: 1, AgentID: "agent-1", Platform: "slack", Name: "mine", CreatedBy: "owner-a", Enabled: true})
	insertOwnerScopeChannel(t, db, IMChannel{ID: "ch-b", TenantID: 1, AgentID: "agent-1", Platform: "slack", Name: "other", CreatedBy: "owner-b", Enabled: true})

	channels, err := svc.ListChannelsByAgent("agent-1", 1, "owner-a")
	if err != nil {
		t.Fatalf("ListChannelsByAgent() error = %v", err)
	}
	if len(channels) != 1 || channels[0].ID != "ch-a" {
		t.Fatalf("ListChannelsByAgent() = %#v, want only owner-a channel", channels)
	}
}

func TestListChannelsByTenantFiltersByCreator(t *testing.T) {
	db := newOwnerScopeTestDB(t)
	svc := &Service{db: db}
	insertOwnerScopeAgent(t, db, "agent-1", 1, "Custom Agent")
	insertOwnerScopeChannel(t, db, IMChannel{ID: "ch-custom", TenantID: 1, AgentID: "agent-1", Platform: "slack", Name: "custom", CreatedBy: "owner-a", Enabled: true})
	insertOwnerScopeChannel(t, db, IMChannel{ID: "ch-builtin", TenantID: 1, AgentID: types.BuiltinSmartReasoningID, Platform: "feishu", Name: "builtin", CreatedBy: "owner-a", Enabled: true})
	insertOwnerScopeChannel(t, db, IMChannel{ID: "ch-other", TenantID: 1, AgentID: "agent-1", Platform: "slack", Name: "other", CreatedBy: "owner-b", Enabled: true})

	channels, err := svc.ListChannelsByTenant(1, "owner-a")
	if err != nil {
		t.Fatalf("ListChannelsByTenant() error = %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("ListChannelsByTenant() len = %d, want 2", len(channels))
	}
	if channels[0].ID != "ch-builtin" || channels[1].ID != "ch-custom" {
		t.Fatalf("ListChannelsByTenant() ids = %#v, want [ch-builtin ch-custom]", channels)
	}
	if channels[1].AgentName != "Custom Agent" {
		t.Fatalf("custom agent name = %q, want Custom Agent", channels[1].AgentName)
	}
	if channels[0].AgentName != "" {
		t.Fatalf("builtin agent name = %q, want empty fallback", channels[0].AgentName)
	}
}

func TestChannelCRUDRespectsCreatorScope(t *testing.T) {
	db := newOwnerScopeTestDB(t)
	svc := &Service{db: db}
	insertOwnerScopeChannel(t, db, IMChannel{ID: "ch-own", TenantID: 1, AgentID: "agent-1", Platform: "slack", Name: "mine", CreatedBy: "owner-a", Enabled: true})

	if _, err := svc.GetChannelByIDAndTenant("ch-own", 1, "owner-b"); err == nil {
		t.Fatal("GetChannelByIDAndTenant() expected not found for other owner")
	}
	if _, err := svc.ToggleChannel("ch-own", 1, "owner-b"); err == nil {
		t.Fatal("ToggleChannel() expected not found for other owner")
	}
	if err := svc.DeleteChannel("ch-own", 1, "owner-b"); err == nil {
		t.Fatal("DeleteChannel() expected not found for other owner")
	}

	channel, err := svc.GetChannelByIDAndTenant("ch-own", 1, "owner-a")
	if err != nil {
		t.Fatalf("GetChannelByIDAndTenant() error = %v", err)
	}
	if channel.ID != "ch-own" {
		t.Fatalf("GetChannelByIDAndTenant() id = %q, want ch-own", channel.ID)
	}
	channel, err = svc.ToggleChannel("ch-own", 1, "owner-a")
	if err != nil {
		t.Fatalf("ToggleChannel() error = %v", err)
	}
	if channel.Enabled {
		t.Fatal("ToggleChannel() should disable owner channel")
	}
	if err := svc.DeleteChannel("ch-own", 1, "owner-a"); err != nil {
		t.Fatalf("DeleteChannel() error = %v", err)
	}
	if _, err := svc.GetChannelByIDAndTenant("ch-own", 1, "owner-a"); err == nil {
		t.Fatal("GetChannelByIDAndTenant() expected not found after delete")
	}
}
