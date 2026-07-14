package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTenantMemberRepositoryGet_MissingRowReturnsNil(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTenantMemberRepository(db)

	member, err := repo.Get(context.Background(), "missing-user", 99999)
	require.NoError(t, err)
	assert.Nil(t, member)
}