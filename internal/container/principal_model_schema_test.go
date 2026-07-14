package container

import (
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func expectColumnExists(mock sqlmock.Sqlmock, tableName, columnName string, exists bool) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = $1
			  AND column_name = $2
		)`)).
		WithArgs(tableName, columnName).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(exists))
}

func expectTableExists(mock sqlmock.Sqlmock, tableName string, exists bool) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = current_schema()
			  AND table_name = $1
		)`)).
		WithArgs(tableName).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(exists))
}

func expectVarcharLength(mock sqlmock.Sqlmock, tableName, columnName string, length interface{}) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT character_maximum_length
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = $1
		  AND column_name = $2
		LIMIT 1`)).
		WithArgs(tableName, columnName).
		WillReturnRows(sqlmock.NewRows([]string{"character_maximum_length"}).AddRow(length))
}

func TestValidatePrincipalModelSchemaCompatibility_RejectsDriftedPostgresSchema(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer sqlDB.Close()

	expectTableExists(mock, "mcp_oauth_clients", true)
	expectTableExists(mock, "mcp_oauth_tokens", true)
	expectColumnExists(mock, "tenants", "api_principal_config", false)
	expectColumnExists(mock, "mcp_oauth_tokens", "principal_type", true)
	expectColumnExists(mock, "mcp_oauth_tokens", "principal_id", true)
	expectVarcharLength(mock, "sessions", "user_id", 512)
	expectVarcharLength(mock, "mcp_oauth_tokens", "user_id", 512)
	expectVarcharLength(mock, "mcp_oauth_tokens", "principal_id", 512)
	expectVarcharLength(mock, "mcp_oauth_tokens", "principal_type", 32)

	err = validatePrincipalModelSchemaCompatibility(sqlDB, "postgres")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing tenants.api_principal_config")
	assert.Contains(t, err.Error(), "000083_principal_model_schema_repair")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestValidatePrincipalModelSchemaCompatibility_AcceptsRepairedPostgresSchema(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer sqlDB.Close()

	expectTableExists(mock, "mcp_oauth_clients", true)
	expectTableExists(mock, "mcp_oauth_tokens", true)
	expectColumnExists(mock, "tenants", "api_principal_config", true)
	expectColumnExists(mock, "mcp_oauth_tokens", "principal_type", true)
	expectColumnExists(mock, "mcp_oauth_tokens", "principal_id", true)
	expectVarcharLength(mock, "sessions", "user_id", 512)
	expectVarcharLength(mock, "mcp_oauth_tokens", "user_id", 512)
	expectVarcharLength(mock, "mcp_oauth_tokens", "principal_id", 512)
	expectVarcharLength(mock, "mcp_oauth_tokens", "principal_type", 32)

	err = validatePrincipalModelSchemaCompatibility(sqlDB, "postgres")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestValidatePrincipalModelSchemaCompatibility_SkipsNonPostgres(t *testing.T) {
	err := validatePrincipalModelSchemaCompatibility(&sql.DB{}, "sqlite")
	assert.NoError(t, err)
	err = validatePrincipalModelSchemaCompatibility(nil, "postgres")
	assert.NoError(t, err)
}
