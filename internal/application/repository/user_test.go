package repository

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestUserRepository_AllowsRecreateAfterSoftDelete(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.User{}))

	repo := NewUserRepository(db)
	ctx := context.Background()
	first := &types.User{
		ID:           "u-1",
		Username:     "hlsa",
		Email:        "hlsa@hlsa.com",
		Phone:        "13663861188",
		PasswordHash: "hash-1",
	}
	require.NoError(t, repo.CreateUser(ctx, first))
	require.NoError(t, repo.DeleteUser(ctx, first.ID))

	second := &types.User{
		ID:           "u-2",
		Username:     "hlsa",
		Email:        "hlsa@hlsa.com",
		Phone:        "13663861188",
		PasswordHash: "hash-2",
	}
	require.NoError(t, repo.CreateUser(ctx, second))
	stored, err := repo.GetUserByID(ctx, second.ID)
	require.NoError(t, err)
	require.Equal(t, second.Username, stored.Username)
	require.Equal(t, second.Email, stored.Email)
	require.Equal(t, second.Phone, stored.Phone)
}