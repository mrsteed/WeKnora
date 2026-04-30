package database

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/databaseconnector"
	"github.com/Tencent/WeKnora/internal/datasource"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAESKey = "12345678901234567890123456789012"

type stubConnector struct {
	typeName string
	validate func(ctx context.Context, cfg *types.DatabaseConnectionConfig) error
	schema   *types.DatabaseSchema
	err      error
}

func (s *stubConnector) Type() string { return s.typeName }

func (s *stubConnector) Dialect() types.SQLDialect { return types.SQLDialectMySQL }

func (s *stubConnector) Validate(ctx context.Context, cfg *types.DatabaseConnectionConfig) error {
	if s.validate != nil {
		return s.validate(ctx, cfg)
	}
	return s.err
}

func (s *stubConnector) DiscoverSchema(context.Context, *types.DatabaseConnectionConfig) (*types.DatabaseSchema, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.schema, nil
}

func (s *stubConnector) Query(context.Context, *types.DatabaseConnectionConfig, string, time.Duration) (*sql.Rows, error) {
	return nil, s.err
}

func TestAdapterValidateDecryptsPassword(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", testAESKey)

	encryptedPassword, err := utils.EncryptAESGCM("secret-pass", []byte(testAESKey))
	require.NoError(t, err)

	registry := databaseconnector.NewRegistry()
	var seenPassword string
	require.NoError(t, registry.Register(&stubConnector{
		typeName: types.DatabaseTypeMySQL,
		validate: func(_ context.Context, cfg *types.DatabaseConnectionConfig) error {
			seenPassword = cfg.Credentials.Password
			assert.Equal(t, "127.0.0.1", cfg.Settings.Host)
			assert.Equal(t, "crm", cfg.Settings.Database)
			return nil
		},
	}))

	adapter := NewAdapter(types.DatabaseTypeMySQL, registry)
	err = adapter.Validate(context.Background(), &types.DataSourceConfig{
		Type: types.DatabaseTypeMySQL,
		Credentials: map[string]interface{}{
			"username": "readonly_user",
			"password": encryptedPassword,
		},
		Settings: map[string]interface{}{
			"host":     "127.0.0.1",
			"database": "crm",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "secret-pass", seenPassword)
}

func TestAdapterListResourcesBuildsDatabaseTree(t *testing.T) {
	registry := databaseconnector.NewRegistry()
	require.NoError(t, registry.Register(&stubConnector{
		typeName: types.DatabaseTypePostgreSQL,
		schema: &types.DatabaseSchema{
			DatabaseType: types.DatabaseTypePostgreSQL,
			DatabaseName: "crm",
			SchemaName:   "analytics",
			Tables: []types.TableSchema{
				{Name: "orders", Type: "table", Comment: "sales orders", Columns: []types.ColumnSchema{{Name: "id"}}},
				{Name: "daily_sales", Type: "view"},
			},
		},
	}))

	adapter := NewAdapter(types.DatabaseTypePostgreSQL, registry)
	resources, err := adapter.ListResources(context.Background(), &types.DataSourceConfig{
		Type:        types.DatabaseTypePostgreSQL,
		Credentials: map[string]interface{}{"username": "readonly_user"},
		Settings: map[string]interface{}{
			"host":     "127.0.0.1",
			"database": "crm",
			"schema":   "analytics",
		},
	})
	require.NoError(t, err)
	require.Len(t, resources, 4)
	assert.Equal(t, "database", resources[0].Type)
	assert.Equal(t, "schema", resources[1].Type)
	assert.Equal(t, "table", resources[2].Type)
	assert.Equal(t, "view", resources[3].Type)
	assert.Equal(t, resources[1].ExternalID, resources[2].ParentID)
	assert.Equal(t, 1, resources[2].Metadata["column_count"])
}

func TestAdapterRejectsSyncOperations(t *testing.T) {
	adapter := NewAdapter(types.DatabaseTypeMySQL, databaseconnector.NewRegistry())

	_, err := adapter.FetchAll(context.Background(), nil, nil)
	assert.ErrorIs(t, err, datasource.ErrSyncNotSupported)

	_, _, err = adapter.FetchIncremental(context.Background(), nil, nil)
	assert.ErrorIs(t, err, datasource.ErrSyncNotSupported)
}

func TestAdapterRejectsEncryptedPasswordWithoutAESKey(t *testing.T) {
	registry := databaseconnector.NewRegistry()
	require.NoError(t, registry.Register(&stubConnector{typeName: types.DatabaseTypeMySQL}))

	adapter := NewAdapter(types.DatabaseTypeMySQL, registry)
	err := adapter.Validate(context.Background(), &types.DataSourceConfig{
		Type: types.DatabaseTypeMySQL,
		Credentials: map[string]interface{}{
			"username": "readonly_user",
			"password": utils.EncPrefix + "ciphertext",
		},
		Settings: map[string]interface{}{
			"host":     "127.0.0.1",
			"database": "crm",
		},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, datasource.ErrInvalidConfig)
	assert.True(t, errors.Is(err, datasource.ErrInvalidConfig))
}
