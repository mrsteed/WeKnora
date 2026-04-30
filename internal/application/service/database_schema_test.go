package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/databaseconnector"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubDatabaseConnector struct {
	schema *types.DatabaseSchema
	err    error
}

func (s *stubDatabaseConnector) Type() string { return types.DatabaseTypeMySQL }
func (s *stubDatabaseConnector) Validate(context.Context, *types.DatabaseConnectionConfig) error {
	return nil
}
func (s *stubDatabaseConnector) DiscoverSchema(context.Context, *types.DatabaseConnectionConfig) (*types.DatabaseSchema, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.schema, nil
}
func (s *stubDatabaseConnector) Query(context.Context, *types.DatabaseConnectionConfig, string, time.Duration) (*sql.Rows, error) {
	return nil, nil
}
func (s *stubDatabaseConnector) Dialect() types.SQLDialect { return types.SQLDialectMySQL }

type stubKnowledgeBaseRepo struct {
	byID map[string]*types.KnowledgeBase
}

func (s *stubKnowledgeBaseRepo) CreateKnowledgeBase(context.Context, *types.KnowledgeBase) error {
	return nil
}
func (s *stubKnowledgeBaseRepo) GetKnowledgeBaseByID(_ context.Context, id string) (*types.KnowledgeBase, error) {
	kb, ok := s.byID[id]
	if !ok {
		return nil, apprepo.ErrKnowledgeBaseNotFound
	}
	return kb, nil
}
func (s *stubKnowledgeBaseRepo) GetKnowledgeBaseByIDAndTenant(_ context.Context, id string, tenantID uint64) (*types.KnowledgeBase, error) {
	kb, ok := s.byID[id]
	if !ok || kb.TenantID != tenantID {
		return nil, apprepo.ErrKnowledgeBaseNotFound
	}
	return kb, nil
}
func (s *stubKnowledgeBaseRepo) GetKnowledgeBaseByIDs(context.Context, []string) ([]*types.KnowledgeBase, error) {
	return nil, nil
}
func (s *stubKnowledgeBaseRepo) ListKnowledgeBases(context.Context) ([]*types.KnowledgeBase, error) {
	return nil, nil
}
func (s *stubKnowledgeBaseRepo) ListKnowledgeBasesByTenantID(context.Context, uint64) ([]*types.KnowledgeBase, error) {
	return nil, nil
}
func (s *stubKnowledgeBaseRepo) UpdateKnowledgeBase(context.Context, *types.KnowledgeBase) error {
	return nil
}
func (s *stubKnowledgeBaseRepo) DeleteKnowledgeBase(context.Context, string) error { return nil }
func (s *stubKnowledgeBaseRepo) TogglePinKnowledgeBase(context.Context, string, uint64) (*types.KnowledgeBase, error) {
	return nil, nil
}
func (s *stubKnowledgeBaseRepo) ListAccessibleKBs(context.Context, string, uint64, []string) ([]*types.KnowledgeBase, error) {
	return nil, nil
}
func (s *stubKnowledgeBaseRepo) ListKBsByOrganization(context.Context, string) ([]*types.KnowledgeBase, error) {
	return nil, nil
}

type stubDataSourceRepo struct {
	byID map[string]*types.DataSource
	byKB map[string][]*types.DataSource
}

func (s *stubDataSourceRepo) Create(context.Context, *types.DataSource) error { return nil }
func (s *stubDataSourceRepo) FindByID(_ context.Context, id string) (*types.DataSource, error) {
	ds, ok := s.byID[id]
	if !ok {
		return nil, errors.New("data source not found")
	}
	return ds, nil
}
func (s *stubDataSourceRepo) FindByKnowledgeBase(_ context.Context, kbID string) ([]*types.DataSource, error) {
	return s.byKB[kbID], nil
}
func (s *stubDataSourceRepo) Update(context.Context, *types.DataSource) error { return nil }
func (s *stubDataSourceRepo) Delete(context.Context, string) error            { return nil }
func (s *stubDataSourceRepo) FindActive(context.Context) ([]*types.DataSource, error) {
	return nil, nil
}

type stubDatabaseSchemaRepo struct {
	snapshot *types.DatabaseSchemaSnapshot
	columns  []*types.DatabaseTableColumn
}

func (s *stubDatabaseSchemaRepo) ReplaceSnapshot(_ context.Context, snapshot *types.DatabaseSchemaSnapshot, columns []*types.DatabaseTableColumn) error {
	s.snapshot = snapshot
	s.columns = columns
	return nil
}
func (s *stubDatabaseSchemaRepo) GetLatestSnapshotByKnowledgeBase(context.Context, uint64, string) (*types.DatabaseSchemaSnapshot, error) {
	return s.snapshot, nil
}
func (s *stubDatabaseSchemaRepo) GetLatestSnapshotByDataSource(context.Context, uint64, string) (*types.DatabaseSchemaSnapshot, error) {
	return s.snapshot, nil
}
func (s *stubDatabaseSchemaRepo) ListColumnsByKnowledgeBase(context.Context, uint64, string) ([]*types.DatabaseTableColumn, error) {
	return s.columns, nil
}
func (s *stubDatabaseSchemaRepo) ListColumnsByTable(_ context.Context, _ uint64, _ string, tableName string) ([]*types.DatabaseTableColumn, error) {
	var filtered []*types.DatabaseTableColumn
	for _, column := range s.columns {
		if column.Table == tableName {
			filtered = append(filtered, column)
		}
	}
	return filtered, nil
}

func TestSchemaRegistryServiceRefreshSchemaFiltersAndPersists(t *testing.T) {
	registry := databaseconnector.NewRegistry()
	require.NoError(t, registry.Register(&stubDatabaseConnector{schema: &types.DatabaseSchema{
		DatabaseType: types.DatabaseTypeMySQL,
		DatabaseName: "crm",
		SchemaName:   "crm",
		RefreshedAt:  time.Now().UTC(),
		Tables: []types.TableSchema{
			{
				Name:        "customers",
				Type:        "table",
				PrimaryKeys: []string{"id"},
				Columns:     []types.ColumnSchema{{Name: "id", DataType: "bigint"}},
			},
			{
				Name:        "orders",
				Type:        "table",
				Comment:     "订单事实表",
				PrimaryKeys: []string{"id"},
				Columns: []types.ColumnSchema{
					{Name: "id", DataType: "bigint", Nullable: false},
					{Name: "phone", DataType: "varchar", Nullable: true},
					{Name: "amount", DataType: "decimal", Nullable: false, Comment: "订单金额", IsSensitive: true},
				},
			},
		},
	}}))

	ds := &types.DataSource{ID: "ds-1", TenantID: 1, KnowledgeBaseID: "kb-1", Type: types.DatabaseTypeMySQL}
	require.NoError(t, ds.SetDatabaseConnectionConfig(&types.DatabaseConnectionConfig{
		Type:        types.DatabaseTypeMySQL,
		Credentials: types.DatabaseCredentials{Username: "reader", Password: "secret"},
		Settings: types.DatabaseSourceSettings{
			Host:           "127.0.0.1",
			Port:           3306,
			Database:       "crm",
			TableAllowlist: []string{"orders"},
			ColumnDenylist: []string{"orders.phone"},
		},
	}))

	schemaRepo := &stubDatabaseSchemaRepo{}
	svc := NewSchemaRegistryService(
		&stubKnowledgeBaseRepo{byID: map[string]*types.KnowledgeBase{
			"kb-1": {ID: "kb-1", TenantID: 1, Type: types.KnowledgeBaseTypeDatabase, Name: "DB KB"},
		}},
		&stubDataSourceRepo{byID: map[string]*types.DataSource{"ds-1": ds}, byKB: map[string][]*types.DataSource{"kb-1": {ds}}},
		schemaRepo,
		registry,
	)

	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	require.NoError(t, svc.RefreshSchema(ctx, "ds-1"))
	require.NotNil(t, schemaRepo.snapshot)
	require.Len(t, schemaRepo.columns, 2)
	assert.Equal(t, "orders", schemaRepo.columns[0].Table)
	assert.Equal(t, "id", schemaRepo.columns[0].ColumnName)
	assert.Equal(t, "amount", schemaRepo.columns[1].ColumnName)

	schema, err := svc.GetDatabaseSchema(ctx, "kb-1")
	require.NoError(t, err)
	require.Len(t, schema.Tables, 1)
	assert.Equal(t, "orders", schema.Tables[0].Name)
	require.Len(t, schema.Tables[0].Columns, 2)
	assert.Equal(t, "id", schema.Tables[0].Columns[0].Name)
	assert.Equal(t, "amount", schema.Tables[0].Columns[1].Name)

	table, err := svc.GetTableSchema(ctx, "kb-1", "orders")
	require.NoError(t, err)
	assert.Equal(t, "orders", table.Name)

	prompt, err := svc.BuildPromptSchema(ctx, "kb-1", []string{"orders"})
	require.NoError(t, err)
	assert.Contains(t, prompt, "Database: crm")
	assert.Contains(t, prompt, "- orders (table)")
	assert.Contains(t, prompt, "amount decimal NOT NULL [sensitive]")
	assert.NotContains(t, prompt, "phone")
	assert.NotContains(t, prompt, "customers")
}

func TestSchemaRegistryServiceRejectsNonDatabaseKB(t *testing.T) {
	registry := databaseconnector.NewRegistry()
	require.NoError(t, registry.Register(&stubDatabaseConnector{schema: &types.DatabaseSchema{}}))
	ds := &types.DataSource{ID: "ds-1", TenantID: 1, KnowledgeBaseID: "kb-1", Type: types.DatabaseTypeMySQL}
	require.NoError(t, ds.SetDatabaseConnectionConfig(&types.DatabaseConnectionConfig{
		Type:        types.DatabaseTypeMySQL,
		Credentials: types.DatabaseCredentials{Username: "reader"},
		Settings:    types.DatabaseSourceSettings{Host: "127.0.0.1", Database: "crm"},
	}))

	svc := NewSchemaRegistryService(
		&stubKnowledgeBaseRepo{byID: map[string]*types.KnowledgeBase{
			"kb-1": {ID: "kb-1", TenantID: 1, Type: types.KnowledgeBaseTypeDocument},
		}},
		&stubDataSourceRepo{byID: map[string]*types.DataSource{"ds-1": ds}, byKB: map[string][]*types.DataSource{"kb-1": {ds}}},
		&stubDatabaseSchemaRepo{},
		registry,
	)

	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	err := svc.RefreshSchema(ctx, "ds-1")
	require.ErrorIs(t, err, ErrDatabaseKnowledgeBaseRequired)

	_, err = svc.GetDatabaseSchema(ctx, "kb-1")
	require.ErrorIs(t, err, ErrDatabaseKnowledgeBaseRequired)

	_, err = svc.BuildPromptSchema(ctx, "kb-1", nil)
	require.ErrorIs(t, err, ErrDatabaseKnowledgeBaseRequired)
}

func TestSchemaRegistryServiceGetDatabaseSchemaRejectsMissingDatabaseDataSource(t *testing.T) {
	svc := NewSchemaRegistryService(
		&stubKnowledgeBaseRepo{byID: map[string]*types.KnowledgeBase{
			"kb-1": {ID: "kb-1", TenantID: 1, Type: types.KnowledgeBaseTypeDatabase, Name: "DB KB"},
		}},
		&stubDataSourceRepo{byKB: map[string][]*types.DataSource{"kb-1": nil}},
		&stubDatabaseSchemaRepo{},
		databaseconnector.NewRegistry(),
	)

	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	_, err := svc.GetDatabaseSchema(ctx, "kb-1")
	require.ErrorIs(t, err, ErrDatabaseDataSourceNotFound)

	_, err = svc.BuildPromptSchema(ctx, "kb-1", nil)
	require.ErrorIs(t, err, ErrDatabaseDataSourceNotFound)
}
