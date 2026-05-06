package mysql

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
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

func TestFetchForeignKeys(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	meta := map[string]*tableAccumulator{
		"orders": {Table: types.TableSchema{Name: "orders"}},
	}
	mock.ExpectQuery("FROM information_schema.key_column_usage").
		WithArgs("crm").
		WillReturnRows(sqlmock.NewRows([]string{"table_name", "constraint_name", "column_name", "referenced_table_name", "referenced_column_name", "ordinal_position"}).
			AddRow("orders", "fk_orders_customer", "customer_id", "customers", "id", 1))

	require.NoError(t, fetchForeignKeys(context.Background(), db, "crm", meta))
	require.Len(t, meta["orders"].Table.ForeignKeys, 1)
	assert.Equal(t, "fk_orders_customer", meta["orders"].Table.ForeignKeys[0].Name)
	assert.Equal(t, []string{"customer_id"}, meta["orders"].Table.ForeignKeys[0].Columns)
	assert.Equal(t, "customers", meta["orders"].Table.ForeignKeys[0].ReferencedTable)
	assert.Equal(t, []string{"id"}, meta["orders"].Table.ForeignKeys[0].ReferencedColumns)
	require.NoError(t, mock.ExpectationsWereMet())
}
