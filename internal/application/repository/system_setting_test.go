package repository

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newSystemSettingRepositoryForTest(t *testing.T) (*systemSettingRepository, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.SystemSetting{}))

	repo, ok := NewSystemSettingRepository(db).(*systemSettingRepository)
	require.True(t, ok)
	return repo, db
}

func TestSystemSettingRepositoryGetReturnsNilForMissingKey(t *testing.T) {
	repo, _ := newSystemSettingRepositoryForTest(t)

	got, err := repo.Get(context.Background(), "asynq.concurrency")
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestSystemSettingRepositoryGetReturnsPersistedRow(t *testing.T) {
	repo, db := newSystemSettingRepositoryForTest(t)
	require.NoError(t, db.Create(&types.SystemSetting{
		Key:              "asynq.concurrency",
		Value:            types.JSON("5"),
		ValueType:        "int",
		Category:         "worker",
		Description:      "worker pool size",
		RequiresRestart:  true,
		LastModifiedBy:   "tester",
	}).Error)

	got, err := repo.Get(context.Background(), "asynq.concurrency")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "asynq.concurrency", got.Key)
	require.Equal(t, types.JSON("5"), got.Value)
}