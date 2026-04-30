package postgres

import (
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/datasource"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDSN(t *testing.T) {
	cfg := &types.DatabaseConnectionConfig{
		Type: types.DatabaseTypePostgreSQL,
		Credentials: types.DatabaseCredentials{
			Username: "readonly_user",
			Password: "secret-pass",
		},
		Settings: types.DatabaseSourceSettings{
			Host:     "127.0.0.1",
			Port:     5433,
			Database: "crm",
			Schema:   "analytics",
			SSLMode:  "require",
		},
	}

	dsn, err := buildDSN(cfg)
	require.NoError(t, err)
	assert.Contains(t, dsn, "postgresql://readonly_user:secret-pass@127.0.0.1:5433/crm")
	assert.Contains(t, dsn, "search_path=analytics")
	assert.Contains(t, dsn, "sslmode=require")
}

func TestBuildDSNEscapesSpecialCharacters(t *testing.T) {
	cfg := &types.DatabaseConnectionConfig{
		Type: types.DatabaseTypePostgreSQL,
		Credentials: types.DatabaseCredentials{
			Username: "readonly_user",
			Password: "p@ss word/with:special?chars",
		},
		Settings: types.DatabaseSourceSettings{
			Host:     "127.0.0.1",
			Database: "crm",
		},
	}

	dsn, err := buildDSN(cfg)
	require.NoError(t, err)
	assert.NotContains(t, dsn, "p@ss word/with:special?chars@127.0.0.1")
	assert.True(t, strings.Contains(dsn, "p%40ss%20word%2Fwith%3Aspecial%3Fchars") || strings.Contains(dsn, "p%40ss+word%2Fwith%3Aspecial%3Fchars"))
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
