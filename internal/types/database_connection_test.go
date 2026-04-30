package types

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDatabaseConnectionConfig_ValueScan(t *testing.T) {
	t.Run("encrypts password on Value and decrypts on Scan", func(t *testing.T) {
		t.Setenv("SYSTEM_AES_KEY", testAESKey)
		t.Setenv("APP_ENV", "development")

		original := DatabaseConnectionConfig{
			Type: DatabaseTypeMySQL,
			Credentials: DatabaseCredentials{
				Username: "readonly_user",
				Password: "secret-pass",
			},
			Settings: DatabaseSourceSettings{
				Host:     "10.0.0.10",
				Port:     3306,
				Database: "crm",
			},
		}

		raw, err := original.Value()
		require.NoError(t, err)

		var intermediate map[string]any
		require.NoError(t, json.Unmarshal(raw.([]byte), &intermediate))
		credentials := intermediate["credentials"].(map[string]any)
		settings := intermediate["settings"].(map[string]any)
		assert.Equal(t, "readonly_user", credentials["username"])
		assert.True(t, strings.HasPrefix(credentials["password"].(string), "enc:v1:"))
		assert.Equal(t, "10.0.0.10", settings["host"])
		assert.EqualValues(t, 3306, settings["port"])

		var scanned DatabaseConnectionConfig
		require.NoError(t, scanned.Scan(raw.([]byte)))
		assert.Equal(t, "secret-pass", scanned.Credentials.Password)
		assert.Equal(t, "readonly_user", scanned.Credentials.Username)
		assert.Equal(t, "crm", scanned.Settings.Database)
	})

	t.Run("allows plaintext password in non-production when AES key is missing", func(t *testing.T) {
		t.Setenv("SYSTEM_AES_KEY", "")
		t.Setenv("APP_ENV", "development")

		original := DatabaseConnectionConfig{
			Type:        DatabaseTypePostgreSQL,
			Credentials: DatabaseCredentials{Password: "secret-pass"},
		}

		raw, err := original.Value()
		require.NoError(t, err)

		var intermediate map[string]any
		require.NoError(t, json.Unmarshal(raw.([]byte), &intermediate))
		credentials := intermediate["credentials"].(map[string]any)
		assert.Equal(t, "secret-pass", credentials["password"])
	})

	t.Run("rejects plaintext password in production when AES key is missing", func(t *testing.T) {
		t.Setenv("SYSTEM_AES_KEY", "")
		t.Setenv("APP_ENV", "production")

		original := DatabaseConnectionConfig{
			Type:        DatabaseTypeMySQL,
			Credentials: DatabaseCredentials{Password: "secret-pass"},
		}

		_, err := original.Value()
		require.ErrorIs(t, err, ErrDatabaseCredentialsRequireAESKey)
	})

	t.Run("does not double-encrypt an already encrypted password", func(t *testing.T) {
		t.Setenv("SYSTEM_AES_KEY", testAESKey)
		t.Setenv("APP_ENV", "development")

		original := DatabaseConnectionConfig{
			Type:        DatabaseTypeMySQL,
			Credentials: DatabaseCredentials{Password: "secret-pass"},
		}

		raw1, err := original.Value()
		require.NoError(t, err)

		var stored DatabaseConnectionConfig
		require.NoError(t, json.Unmarshal(raw1.([]byte), &stored))
		raw2, err := stored.Value()
		require.NoError(t, err)

		var scanned DatabaseConnectionConfig
		require.NoError(t, scanned.Scan(raw2.([]byte)))
		assert.Equal(t, "secret-pass", scanned.Credentials.Password)
	})

	t.Run("original struct is not mutated by Value", func(t *testing.T) {
		t.Setenv("SYSTEM_AES_KEY", testAESKey)
		t.Setenv("APP_ENV", "development")

		original := DatabaseConnectionConfig{
			Credentials: DatabaseCredentials{Password: "secret-pass"},
		}

		_, err := original.Value()
		require.NoError(t, err)
		assert.Equal(t, "secret-pass", original.Credentials.Password)
	})

	t.Run("scan nil value returns no error", func(t *testing.T) {
		var cfg DatabaseConnectionConfig
		assert.NoError(t, cfg.Scan(nil))
	})
}

func TestDatabaseConnectionConfig_DataSourceRoundTrip(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", testAESKey)
	t.Setenv("APP_ENV", "development")

	cfg := &DatabaseConnectionConfig{
		Type: DatabaseTypeMySQL,
		Credentials: DatabaseCredentials{
			Username: "readonly_user",
			Password: "secret-pass",
		},
		Settings: DatabaseSourceSettings{
			Host:     "127.0.0.1",
			Port:     3306,
			Database: "orders",
		},
	}

	ds := &DataSource{}
	require.NoError(t, ds.SetDatabaseConnectionConfig(cfg))
	require.NotEmpty(t, ds.Config)
	assert.NotContains(t, string(ds.Config), "secret-pass")

	parsed, err := ds.ParseDatabaseConnectionConfig()
	require.NoError(t, err)
	require.NotNil(t, parsed)
	assert.Equal(t, "secret-pass", parsed.Credentials.Password)
	assert.Equal(t, "readonly_user", parsed.Credentials.Username)
	assert.Equal(t, "orders", parsed.Settings.Database)
}

func TestDatabaseConnectionConfig_MaskSensitiveFields(t *testing.T) {
	cfg := DatabaseConnectionConfig{
		Credentials: DatabaseCredentials{
			Username: "readonly_user",
			Password: "secret-pass",
		},
	}

	masked := cfg.MaskSensitiveFields()
	assert.Equal(t, "readonly_user", masked.Credentials.Username)
	assert.Equal(t, "***", masked.Credentials.Password)
	assert.Equal(t, "secret-pass", cfg.Credentials.Password)
}
