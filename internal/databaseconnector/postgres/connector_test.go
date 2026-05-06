package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
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

func TestFetchForeignKeys(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	meta := map[string]*tableAccumulator{
		"orders": {Table: types.TableSchema{Name: "orders"}},
	}
	mock.ExpectQuery("FROM pg_constraint con").
		WithArgs("public").
		WillReturnRows(sqlmock.NewRows([]string{"table_name", "constraint_name", "column_name", "referenced_table_name", "referenced_column_name", "ordinality"}).
			AddRow("orders", "fk_orders_customer", "customer_id", "customers", "id", 1))

	require.NoError(t, fetchForeignKeys(context.Background(), db, "public", meta))
	require.Len(t, meta["orders"].Table.ForeignKeys, 1)
	assert.Equal(t, "fk_orders_customer", meta["orders"].Table.ForeignKeys[0].Name)
	assert.Equal(t, []string{"customer_id"}, meta["orders"].Table.ForeignKeys[0].Columns)
	assert.Equal(t, "customers", meta["orders"].Table.ForeignKeys[0].ReferencedTable)
	assert.Equal(t, []string{"id"}, meta["orders"].Table.ForeignKeys[0].ReferencedColumns)
	require.NoError(t, mock.ExpectationsWereMet())
}
