package mysql

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/datasource"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDSN(t *testing.T) {
	cfg := &types.DatabaseConnectionConfig{
		Type: types.DatabaseTypeMySQL,
		Credentials: types.DatabaseCredentials{
			Username: "readonly_user",
			Password: "secret-pass",
		},
		Settings: types.DatabaseSourceSettings{
			Host:     "127.0.0.1",
			Port:     3307,
			Database: "crm",
			SSLMode:  "required",
		},
	}

	dsn, err := buildDSN(cfg)
	require.NoError(t, err)
	assert.Contains(t, dsn, "readonly_user:secret-pass@tcp(127.0.0.1:3307)/crm")
	assert.Contains(t, dsn, "tls=true")
	assert.Contains(t, dsn, "parseTime=true")
	assert.Contains(t, dsn, "charset=utf8mb4")
}

func TestBuildConfigPreservesSpecialCharacters(t *testing.T) {
	cfg := &types.DatabaseConnectionConfig{
		Type: types.DatabaseTypeMySQL,
		Credentials: types.DatabaseCredentials{
			Username: "readonly_user",
			Password: "p@ss word/with:special?chars",
		},
		Settings: types.DatabaseSourceSettings{
			Host:     "127.0.0.1",
			Database: "crm",
		},
	}

	dsnCfg, err := buildConfig(cfg)
	require.NoError(t, err)
	assert.Equal(t, "readonly_user", dsnCfg.User)
	assert.Equal(t, "p@ss word/with:special?chars", dsnCfg.Passwd)
	assert.Equal(t, "127.0.0.1:3306", dsnCfg.Addr)
	assert.Equal(t, "crm", dsnCfg.DBName)
	assert.False(t, dsnCfg.MultiStatements)
	assert.Equal(t, "utf8mb4", dsnCfg.Params["charset"])
	assert.NotContains(t, dsnCfg.Params, "multiStatements")
}

func TestNormalizeTableType(t *testing.T) {
	assert.Equal(t, "table", normalizeTableType("BASE TABLE"))
	assert.Equal(t, "view", normalizeTableType("VIEW"))
	assert.Equal(t, "view", normalizeTableType("SYSTEM VIEW"))
}

func TestValidateConfig(t *testing.T) {
	err := validateConfig(&types.DatabaseConnectionConfig{})
	require.Error(t, err)
	assert.ErrorIs(t, err, datasource.ErrInvalidConfig)

	err = validateConfig(&types.DatabaseConnectionConfig{Settings: types.DatabaseSourceSettings{Host: "127.0.0.1", Database: "crm"}})
	require.Error(t, err)
	assert.ErrorIs(t, err, datasource.ErrInvalidCredentials)

	err = validateConfig(&types.DatabaseConnectionConfig{
		Credentials: types.DatabaseCredentials{Username: "readonly_user"},
		Settings:    types.DatabaseSourceSettings{Host: "127.0.0.1", Database: "crm"},
	})
	require.NoError(t, err)
}
