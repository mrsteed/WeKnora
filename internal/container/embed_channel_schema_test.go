package container

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupEmbedSchemaTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	return db
}

func sqliteIndexExists(t *testing.T, db *gorm.DB, indexName string) bool {
	t.Helper()
	var count int64
	err := db.Raw(`SELECT COUNT(1) FROM sqlite_master WHERE type = 'index' AND name = ?`, indexName).Scan(&count).Error
	require.NoError(t, err)
	return count > 0
}

func TestEnsureEmbedChannelSchema_SelfHealsMissingTable(t *testing.T) {
	db := setupEmbedSchemaTestDB(t)

	ensureEmbedChannelSchema(db)

	assert.True(t, db.Migrator().HasTable(&types.EmbedChannel{}))
	assert.True(t, sqliteIndexExists(t, db, "idx_embed_channels_publish_token"))
}

func TestEnsureEmbedChannelSchema_AddsPublishTokenIndexToDriftedTable(t *testing.T) {
	db := setupEmbedSchemaTestDB(t)
	require.NoError(t, db.AutoMigrate(&types.EmbedChannel{}))
	assert.False(t, sqliteIndexExists(t, db, "idx_embed_channels_publish_token"))

	ensureEmbedChannelSchema(db)

	assert.True(t, sqliteIndexExists(t, db, "idx_embed_channels_publish_token"))
}