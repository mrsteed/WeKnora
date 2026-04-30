package databaseconnector

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubConnector struct{ connectorType string }

func (s *stubConnector) Type() string { return s.connectorType }

func (s *stubConnector) Validate(context.Context, *types.DatabaseConnectionConfig) error { return nil }

func (s *stubConnector) DiscoverSchema(context.Context, *types.DatabaseConnectionConfig) (*types.DatabaseSchema, error) {
	return nil, nil
}

func (s *stubConnector) Query(context.Context, *types.DatabaseConnectionConfig, string, time.Duration) (*sql.Rows, error) {
	return nil, nil
}

func (s *stubConnector) Dialect() types.SQLDialect { return types.SQLDialectMySQL }

func TestRegistry(t *testing.T) {
	r := NewRegistry()

	require.ErrorIs(t, r.Register(nil), ErrConnectorNil)
	require.ErrorIs(t, r.Register(&stubConnector{}), ErrConnectorTypeEmpty)

	conn := &stubConnector{connectorType: types.DatabaseTypeMySQL}
	require.NoError(t, r.Register(conn))

	got, err := r.Get(types.DatabaseTypeMySQL)
	require.NoError(t, err)
	assert.Same(t, conn, got)

	_, err = r.Get(types.DatabaseTypePostgreSQL)
	require.True(t, errors.Is(err, ErrConnectorNotFound))

	assert.Equal(t, []string{types.DatabaseTypeMySQL}, r.List())
}
