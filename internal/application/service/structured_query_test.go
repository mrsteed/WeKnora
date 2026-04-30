package service

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Tencent/WeKnora/internal/databaseconnector"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubSchemaRegistryService struct {
	schema *types.DatabaseSchema
	err    error
}

func (s *stubSchemaRegistryService) RefreshSchema(context.Context, string) error { return nil }
func (s *stubSchemaRegistryService) GetDatabaseSchema(context.Context, string) (*types.DatabaseSchema, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.schema, nil
}
func (s *stubSchemaRegistryService) GetTableSchema(context.Context, string, string) (*types.TableSchema, error) {
	return nil, nil
}
func (s *stubSchemaRegistryService) BuildPromptSchema(context.Context, string, []string) (string, error) {
	return "", nil
}

type stubDatabaseQueryAuditRepo struct {
	logs []*types.DatabaseQueryAuditLog
	err  error
}

func (s *stubDatabaseQueryAuditRepo) Create(_ context.Context, log *types.DatabaseQueryAuditLog) error {
	if s.err != nil {
		return s.err
	}
	clone := *log
	s.logs = append(s.logs, &clone)
	return nil
}
func (s *stubDatabaseQueryAuditRepo) ListByTenant(context.Context, uint64, string, int, int) ([]*types.DatabaseQueryAuditLog, error) {
	return s.logs, nil
}
func (s *stubDatabaseQueryAuditRepo) CountByTenant(context.Context, uint64, string) (int64, error) {
	return int64(len(s.logs)), nil
}

type stubQueryConnector struct {
	typeName string
	schema   *types.DatabaseSchema
	queryErr error
	queryDB  *sql.DB
	lastSQL  string
	lastTO   time.Duration
}

func (s *stubQueryConnector) Type() string { return s.typeName }
func (s *stubQueryConnector) Validate(context.Context, *types.DatabaseConnectionConfig) error {
	return nil
}
func (s *stubQueryConnector) DiscoverSchema(context.Context, *types.DatabaseConnectionConfig) (*types.DatabaseSchema, error) {
	return s.schema, nil
}
func (s *stubQueryConnector) Query(ctx context.Context, _ *types.DatabaseConnectionConfig, query string, timeout time.Duration) (*sql.Rows, error) {
	s.lastSQL = query
	s.lastTO = timeout
	if s.queryErr != nil {
		return nil, s.queryErr
	}
	return s.queryDB.QueryContext(ctx, query)
}
func (s *stubQueryConnector) Dialect() types.SQLDialect {
	if s.typeName == types.DatabaseTypeMySQL {
		return types.SQLDialectMySQL
	}
	return types.SQLDialectPostgreSQL
}

func TestStructuredQueryServiceExecuteQuerySuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, amount FROM orders LIMIT 10")).WillReturnRows(
		sqlmock.NewRows([]string{"id", "amount"}).AddRow(1, 99.5),
	)

	connector := &stubQueryConnector{typeName: types.DatabaseTypePostgreSQL, queryDB: db}
	svc, auditRepo := newStructuredQueryServiceForTest(t, connector, &types.DatabaseSchema{
		TenantID:     1,
		DataSourceID: "ds-1",
		Tables: []types.TableSchema{{
			Name:    "orders",
			Columns: []types.ColumnSchema{{Name: "id", DataType: "bigint"}, {Name: "amount", DataType: "decimal"}},
		}},
	})

	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-1")
	result, err := svc.ExecuteQuery(ctx, types.ExecuteQueryRequest{
		KnowledgeBaseID: "kb-1",
		SQL:             "SELECT id, amount FROM orders LIMIT 10",
		Purpose:         "order summary",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "external_database_query", result.DisplayType)
	assert.Equal(t, "SELECT id, amount FROM orders LIMIT 10", result.ExecutedSQL)
	assert.Equal(t, 1, result.RowCount)
	assert.Len(t, result.Rows, 1)
	assert.Equal(t, 99.5, result.Rows[0]["amount"])
	require.Len(t, auditRepo.logs, 1)
	assert.Equal(t, types.DatabaseQueryAuditStatusSuccess, auditRepo.logs[0].Status)
	assert.Equal(t, 1, auditRepo.logs[0].RowCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStructuredQueryServiceRejectsDangerousStatement(t *testing.T) {
	connector := &stubQueryConnector{typeName: types.DatabaseTypePostgreSQL}
	svc, auditRepo := newStructuredQueryServiceForTest(t, connector, &types.DatabaseSchema{
		TenantID:     1,
		DataSourceID: "ds-1",
		Tables:       []types.TableSchema{{Name: "orders", Columns: []types.ColumnSchema{{Name: "id"}}}},
	})

	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-1")
	_, err := svc.ExecuteQuery(ctx, types.ExecuteQueryRequest{
		KnowledgeBaseID: "kb-1",
		SQL:             "DELETE FROM orders WHERE id = 1",
	})
	require.Error(t, err)
	require.Len(t, auditRepo.logs, 1)
	assert.Equal(t, types.DatabaseQueryAuditStatusRejected, auditRepo.logs[0].Status)
	assert.Contains(t, auditRepo.logs[0].ErrorMessage, "Only SELECT queries are allowed")
}

func TestStructuredQueryServiceRejectsSensitiveColumnSelection(t *testing.T) {
	connector := &stubQueryConnector{typeName: types.DatabaseTypePostgreSQL}
	svc, auditRepo := newStructuredQueryServiceForTest(t, connector, &types.DatabaseSchema{
		TenantID:     1,
		DataSourceID: "ds-1",
		Tables: []types.TableSchema{{
			Name:    "customers",
			Columns: []types.ColumnSchema{{Name: "id"}, {Name: "phone", IsSensitive: true}},
		}},
	})

	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-1")
	_, err := svc.ExecuteQuery(ctx, types.ExecuteQueryRequest{
		KnowledgeBaseID: "kb-1",
		SQL:             "SELECT phone FROM customers LIMIT 10",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrStructuredQuerySensitiveColumn)
	require.Len(t, auditRepo.logs, 1)
	assert.Equal(t, types.DatabaseQueryAuditStatusRejected, auditRepo.logs[0].Status)
}

func TestStructuredQueryServiceRejectsTableOutsideSchema(t *testing.T) {
	connector := &stubQueryConnector{typeName: types.DatabaseTypePostgreSQL}
	svc, auditRepo := newStructuredQueryServiceForTest(t, connector, &types.DatabaseSchema{
		TenantID:     1,
		DataSourceID: "ds-1",
		Tables:       []types.TableSchema{{Name: "orders", Columns: []types.ColumnSchema{{Name: "id"}}}},
	})

	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-1")
	_, err := svc.ExecuteQuery(ctx, types.ExecuteQueryRequest{
		KnowledgeBaseID: "kb-1",
		SQL:             "SELECT * FROM invoices LIMIT 10",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrStructuredQueryTableNotAllowed)
	require.Len(t, auditRepo.logs, 1)
	assert.Equal(t, types.DatabaseQueryAuditStatusRejected, auditRepo.logs[0].Status)
}

func TestStructuredQueryServiceRecordsFailedTimeoutQueries(t *testing.T) {
	connector := &stubQueryConnector{typeName: types.DatabaseTypePostgreSQL, queryErr: context.DeadlineExceeded}
	svc, auditRepo := newStructuredQueryServiceForTest(t, connector, &types.DatabaseSchema{
		TenantID:     1,
		DataSourceID: "ds-1",
		Tables:       []types.TableSchema{{Name: "orders", Columns: []types.ColumnSchema{{Name: "id"}}}},
	})

	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-1")
	_, err := svc.ExecuteQuery(ctx, types.ExecuteQueryRequest{
		KnowledgeBaseID: "kb-1",
		SQL:             "SELECT id FROM orders LIMIT 10",
		TimeoutSeconds:  1,
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Len(t, auditRepo.logs, 1)
	assert.Equal(t, types.DatabaseQueryAuditStatusFailed, auditRepo.logs[0].Status)
	assert.Equal(t, time.Second, connector.lastTO)
}

func TestStructuredQueryServiceValidateSQLSupportsMySQLQuotedIdentifiers(t *testing.T) {
	connector := &stubQueryConnector{typeName: types.DatabaseTypeMySQL}
	svc, _ := newStructuredQueryServiceForTest(t, connector, &types.DatabaseSchema{
		TenantID:     1,
		DataSourceID: "ds-1",
		Tables:       []types.TableSchema{{Name: "orders", Columns: []types.ColumnSchema{{Name: "id"}}}},
	})

	validated, err := svc.ValidateSQL(context.Background(), types.ValidateSQLRequest{
		KnowledgeBaseID: "kb-1",
		SQL:             "SELECT `id` FROM `orders` LIMIT 10",
	})
	require.NoError(t, err)
	assert.Equal(t, types.SQLDialectMySQL, validated.Dialect)
	assert.Equal(t, []string{"orders"}, validated.Tables)
	assert.Equal(t, []string{"id"}, validated.SelectFields)
}

func TestStructuredQueryServiceValidateSQLClampsRequestToDataSourcePolicy(t *testing.T) {
	connector := &stubQueryConnector{typeName: types.DatabaseTypePostgreSQL}
	svc, _ := newStructuredQueryServiceForTestWithSettings(t, connector, &types.DatabaseSchema{
		TenantID:     1,
		DataSourceID: "ds-1",
		Tables:       []types.TableSchema{{Name: "orders", Columns: []types.ColumnSchema{{Name: "id"}}}},
	}, types.DatabaseSourceSettings{Host: "127.0.0.1", Database: "crm", MaxRows: 100, QueryTimeoutSec: 3})

	validated, err := svc.ValidateSQL(context.Background(), types.ValidateSQLRequest{
		KnowledgeBaseID: "kb-1",
		SQL:             "SELECT id FROM orders LIMIT 10",
		MaxRows:         500,
		TimeoutSeconds:  15,
	})
	require.NoError(t, err)
	assert.Equal(t, 100, validated.MaxRows)
	assert.Equal(t, 3*time.Second, validated.Timeout)
}

func TestStructuredQueryServiceValidateSQLRejectsUnknownColumn(t *testing.T) {
	connector := &stubQueryConnector{typeName: types.DatabaseTypePostgreSQL}
	svc, _ := newStructuredQueryServiceForTest(t, connector, &types.DatabaseSchema{
		TenantID:     1,
		DataSourceID: "ds-1",
		Tables:       []types.TableSchema{{Name: "orders", Columns: []types.ColumnSchema{{Name: "id"}}}},
	})

	_, err := svc.ValidateSQL(context.Background(), types.ValidateSQLRequest{
		KnowledgeBaseID: "kb-1",
		SQL:             "SELECT unknown_field FROM orders LIMIT 10",
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "column")
}

func TestStructuredQueryServiceValidateSQLRejectsLimitAbovePolicy(t *testing.T) {
	connector := &stubQueryConnector{typeName: types.DatabaseTypePostgreSQL}
	svc, _ := newStructuredQueryServiceForTestWithSettings(t, connector, &types.DatabaseSchema{
		TenantID:     1,
		DataSourceID: "ds-1",
		Tables:       []types.TableSchema{{Name: "orders", Columns: []types.ColumnSchema{{Name: "id"}}}},
	}, types.DatabaseSourceSettings{Host: "127.0.0.1", Database: "crm", MaxRows: 50, QueryTimeoutSec: 10})

	_, err := svc.ValidateSQL(context.Background(), types.ValidateSQLRequest{
		KnowledgeBaseID: "kb-1",
		SQL:             "SELECT id FROM orders LIMIT 200",
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "max rows")
}

func TestStructuredQueryServiceValidateSQLSupportsWithSelect(t *testing.T) {
	connector := &stubQueryConnector{typeName: types.DatabaseTypePostgreSQL}
	svc, _ := newStructuredQueryServiceForTest(t, connector, &types.DatabaseSchema{
		TenantID:     1,
		DataSourceID: "ds-1",
		Tables:       []types.TableSchema{{Name: "orders", Columns: []types.ColumnSchema{{Name: "id"}}}},
	})

	validated, err := svc.ValidateSQL(context.Background(), types.ValidateSQLRequest{
		KnowledgeBaseID: "kb-1",
		SQL:             "WITH recent_orders AS (SELECT id FROM orders LIMIT 5) SELECT id FROM recent_orders LIMIT 5",
	})
	require.NoError(t, err)
	assert.NotNil(t, validated)
}

func TestStructuredQueryServiceValidateSQLSupportsSchemaQualifiedAndPostgresQuotedIdentifiers(t *testing.T) {
	connector := &stubQueryConnector{typeName: types.DatabaseTypePostgreSQL}
	svc, _ := newStructuredQueryServiceForTest(t, connector, &types.DatabaseSchema{
		TenantID:     1,
		DataSourceID: "ds-1",
		Tables:       []types.TableSchema{{Name: "orders", Columns: []types.ColumnSchema{{Name: "id"}}}},
	})

	validated, err := svc.ValidateSQL(context.Background(), types.ValidateSQLRequest{
		KnowledgeBaseID: "kb-1",
		SQL:             "SELECT \"id\" FROM public.\"orders\" LIMIT 10",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"orders"}, validated.Tables)
	assert.Equal(t, []string{"id"}, validated.SelectFields)
}

func newStructuredQueryServiceForTest(
	t *testing.T,
	connector databaseconnector.DatabaseConnector,
	schema *types.DatabaseSchema,
) (interfaces.StructuredQueryService, *stubDatabaseQueryAuditRepo) {
	t.Helper()
	return newStructuredQueryServiceForTestWithSettings(
		t,
		connector,
		schema,
		types.DatabaseSourceSettings{Host: "127.0.0.1", Database: "crm", MaxRows: 500, QueryTimeoutSec: 10},
	)
}

func newStructuredQueryServiceForTestWithSettings(
	t *testing.T,
	connector databaseconnector.DatabaseConnector,
	schema *types.DatabaseSchema,
	settings types.DatabaseSourceSettings,
) (interfaces.StructuredQueryService, *stubDatabaseQueryAuditRepo) {
	t.Helper()
	registry := databaseconnector.NewRegistry()
	require.NoError(t, registry.Register(connector))

	ds := &types.DataSource{ID: "ds-1", TenantID: 1, KnowledgeBaseID: "kb-1", Type: connector.Type()}
	require.NoError(t, ds.SetDatabaseConnectionConfig(&types.DatabaseConnectionConfig{
		Type:        connector.Type(),
		Credentials: types.DatabaseCredentials{Username: "reader", Password: "secret"},
		Settings:    settings,
	}))

	auditRepo := &stubDatabaseQueryAuditRepo{}
	svc := NewStructuredQueryService(
		&stubSchemaRegistryService{schema: schema},
		&stubDataSourceRepo{byID: map[string]*types.DataSource{"ds-1": ds}, byKB: map[string][]*types.DataSource{"kb-1": {ds}}},
		auditRepo,
		registry,
	)
	return svc, auditRepo
}

func TestStructuredQueryServiceReturnsAuditErrorWhenPersistenceFails(t *testing.T) {
	connector := &stubQueryConnector{typeName: types.DatabaseTypePostgreSQL, queryErr: errors.New("boom")}
	registry := databaseconnector.NewRegistry()
	require.NoError(t, registry.Register(connector))
	ds := &types.DataSource{ID: "ds-1", TenantID: 1, KnowledgeBaseID: "kb-1", Type: connector.Type()}
	require.NoError(t, ds.SetDatabaseConnectionConfig(&types.DatabaseConnectionConfig{
		Type:        connector.Type(),
		Credentials: types.DatabaseCredentials{Username: "reader", Password: "secret"},
		Settings:    types.DatabaseSourceSettings{Host: "127.0.0.1", Database: "crm"},
	}))

	svc := NewStructuredQueryService(
		&stubSchemaRegistryService{schema: &types.DatabaseSchema{TenantID: 1, DataSourceID: "ds-1", Tables: []types.TableSchema{{Name: "orders", Columns: []types.ColumnSchema{{Name: "id"}}}}}},
		&stubDataSourceRepo{byID: map[string]*types.DataSource{"ds-1": ds}, byKB: map[string][]*types.DataSource{"kb-1": {ds}}},
		&stubDatabaseQueryAuditRepo{err: errors.New("audit failed")},
		registry,
	)

	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-1")
	_, err := svc.ExecuteQuery(ctx, types.ExecuteQueryRequest{KnowledgeBaseID: "kb-1", SQL: "SELECT id FROM orders LIMIT 10"})
	require.EqualError(t, err, "audit failed")
}
