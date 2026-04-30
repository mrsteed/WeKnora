package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAESKey = "12345678901234567890123456789012"

func TestPrepareDatabaseDataSourceForStorageEncryptsAndNormalizes(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", testAESKey)
	t.Setenv("APP_ENV", "development")

	ds := &types.DataSource{
		Type:         types.DatabaseTypeMySQL,
		SyncSchedule: "0 0 */6 * * *",
		SyncMode:     types.SyncModeFull,
		Config: types.JSON(`{
			"type":"mysql",
			"credentials":{"username":"readonly_user","password":"secret-pass"},
			"settings":{"host":"127.0.0.1","database":"crm","port":3306}
		}`),
	}

	require.NoError(t, prepareDatabaseDataSourceForStorage(ds, nil))
	assert.Empty(t, ds.SyncSchedule)
	assert.Equal(t, types.SyncModeIncremental, ds.SyncMode)
	assert.Equal(t, types.ConflictStrategyOverwrite, ds.ConflictStrategy)
	assert.False(t, ds.SyncDeletions)
	assert.NotContains(t, string(ds.Config), "secret-pass")

	parsed, err := ds.ParseDatabaseConnectionConfig()
	require.NoError(t, err)
	assert.Equal(t, "secret-pass", parsed.Credentials.Password)
	assert.Equal(t, types.DatabaseTypeMySQL, parsed.Type)
	assert.Equal(t, "crm", parsed.Settings.Database)
}

func TestPrepareDatabaseDataSourceForStoragePreservesExistingPassword(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", testAESKey)
	t.Setenv("APP_ENV", "development")

	existing := &types.DataSource{Type: types.DatabaseTypePostgreSQL}
	require.NoError(t, existing.SetDatabaseConnectionConfig(&types.DatabaseConnectionConfig{
		Type: types.DatabaseTypePostgreSQL,
		Credentials: types.DatabaseCredentials{
			Username: "readonly_user",
			Password: "secret-pass",
		},
		Settings: types.DatabaseSourceSettings{
			Host:     "127.0.0.1",
			Database: "crm",
		},
	}))

	updated := &types.DataSource{
		Type: types.DatabaseTypePostgreSQL,
		Config: types.JSON(`{
			"type":"postgresql",
			"credentials":{"username":"readonly_user","password":"***"},
			"settings":{"host":"127.0.0.1","database":"crm","schema":"analytics"}
		}`),
	}

	require.NoError(t, prepareDatabaseDataSourceForStorage(updated, existing))
	parsed, err := updated.ParseDatabaseConnectionConfig()
	require.NoError(t, err)
	assert.Equal(t, "secret-pass", parsed.Credentials.Password)
	assert.Equal(t, "analytics", parsed.Settings.Schema)
}

func TestPrepareDatabaseDataSourceForStoragePreservesExistingSettingsWhenOmitted(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", testAESKey)
	t.Setenv("APP_ENV", "development")

	existing := &types.DataSource{Type: types.DatabaseTypeMySQL}
	require.NoError(t, existing.SetDatabaseConnectionConfig(&types.DatabaseConnectionConfig{
		Type: types.DatabaseTypeMySQL,
		Credentials: types.DatabaseCredentials{
			Username: "readonly_user",
			Password: "secret-pass",
		},
		Settings: types.DatabaseSourceSettings{
			Host:            "127.0.0.1",
			Port:            3306,
			Database:        "crm",
			SSLMode:         "required",
			QueryTimeoutSec: 15,
		},
	}))

	updated := &types.DataSource{
		Type:   types.DatabaseTypeMySQL,
		Config: types.JSON(`{"type":"mysql","credentials":{"password":"***"},"settings":{}}`),
	}

	require.NoError(t, prepareDatabaseDataSourceForStorage(updated, existing))
	parsed, err := updated.ParseDatabaseConnectionConfig()
	require.NoError(t, err)
	assert.Equal(t, "readonly_user", parsed.Credentials.Username)
	assert.Equal(t, "secret-pass", parsed.Credentials.Password)
	assert.Equal(t, "127.0.0.1", parsed.Settings.Host)
	assert.Equal(t, 3306, parsed.Settings.Port)
	assert.Equal(t, "crm", parsed.Settings.Database)
	assert.Equal(t, "required", parsed.Settings.SSLMode)
	assert.Equal(t, 15, parsed.Settings.QueryTimeoutSec)
}

func TestIsDatabaseDataSourceType(t *testing.T) {
	assert.True(t, isDatabaseDataSourceType(types.DatabaseTypeMySQL))
	assert.True(t, isDatabaseDataSourceType(types.DatabaseTypePostgreSQL))
	assert.False(t, isDatabaseDataSourceType(types.ConnectorTypeFeishu))
}
