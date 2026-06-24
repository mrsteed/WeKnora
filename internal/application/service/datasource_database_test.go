package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/databaseconnector"
	"github.com/Tencent/WeKnora/internal/datasource"
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

type stubLifecycleDataSourceRepo struct {
	byID      map[string]*types.DataSource
	updated   *types.DataSource
	deletedID string
}

func (s *stubLifecycleDataSourceRepo) Create(context.Context, *types.DataSource) error { return nil }
func (s *stubLifecycleDataSourceRepo) FindByID(_ context.Context, id string) (*types.DataSource, error) {
	ds, ok := s.byID[id]
	if !ok {
		return nil, errors.New("not found")
	}
	clone := *ds
	return &clone, nil
}
func (s *stubLifecycleDataSourceRepo) FindByKnowledgeBase(context.Context, string) ([]*types.DataSource, error) {
	return nil, nil
}
func (s *stubLifecycleDataSourceRepo) Update(_ context.Context, ds *types.DataSource) error {
	clone := *ds
	s.updated = &clone
	s.byID[ds.ID] = &clone
	return nil
}
func (s *stubLifecycleDataSourceRepo) UpdateSyncState(ctx context.Context, ds *types.DataSource) error {
	return s.Update(ctx, ds)
}
func (s *stubLifecycleDataSourceRepo) Delete(_ context.Context, id string) error {
	s.deletedID = id
	delete(s.byID, id)
	return nil
}
func (s *stubLifecycleDataSourceRepo) FindActive(context.Context) ([]*types.DataSource, error) {
	return nil, nil
}

type stubLifecycleSyncLogRepo struct{ canceledID string }

func (s *stubLifecycleSyncLogRepo) Create(context.Context, *types.SyncLog) error { return nil }
func (s *stubLifecycleSyncLogRepo) FindByID(context.Context, string) (*types.SyncLog, error) {
	return nil, nil
}
func (s *stubLifecycleSyncLogRepo) FindByDataSource(context.Context, string, int, int) ([]*types.SyncLog, error) {
	return nil, nil
}
func (s *stubLifecycleSyncLogRepo) FindLatest(context.Context, string) (*types.SyncLog, error) {
	return nil, nil
}
func (s *stubLifecycleSyncLogRepo) HasRunningSync(context.Context, string) (bool, error) {
	return false, nil
}
func (s *stubLifecycleSyncLogRepo) Update(context.Context, *types.SyncLog) error { return nil }
func (s *stubLifecycleSyncLogRepo) UpdateResult(ctx context.Context, log *types.SyncLog) error {
	return s.Update(ctx, log)
}
func (s *stubLifecycleSyncLogRepo) CancelPendingByDataSource(_ context.Context, id string) error {
	s.canceledID = id
	return nil
}
func (s *stubLifecycleSyncLogRepo) CleanupOldLogs(context.Context, int) error { return nil }

func newLifecycleScheduler() *datasource.Scheduler {
	return datasource.NewScheduler(nil, nil, nil)
}

type stubLifecycleSchemaRegistry struct {
	refreshID  string
	refreshErr error
}

func (s *stubLifecycleSchemaRegistry) RefreshSchema(_ context.Context, dataSourceID string) error {
	s.refreshID = dataSourceID
	return s.refreshErr
}
func (s *stubLifecycleSchemaRegistry) GetDatabaseSchema(context.Context, string) (*types.DatabaseSchema, error) {
	return nil, nil
}
func (s *stubLifecycleSchemaRegistry) GetTableSchema(context.Context, string, string) (*types.TableSchema, error) {
	return nil, nil
}
func (s *stubLifecycleSchemaRegistry) BuildPromptSchema(context.Context, string, []string) (string, error) {
	return "", nil
}
func (s *stubLifecycleSchemaRegistry) BuildPromptSchemaResult(context.Context, string, []string, types.PromptSchemaOptions) (*types.PromptSchemaBuildResult, error) {
	return nil, nil
}

type stubLifecycleSchemaRepo struct {
	deletedTenantID     uint64
	deletedDataSourceID string
}

func (s *stubLifecycleSchemaRepo) ReplaceSnapshot(context.Context, *types.DatabaseSchemaSnapshot, []*types.DatabaseTableColumn) error {
	return nil
}
func (s *stubLifecycleSchemaRepo) GetLatestSnapshotByKnowledgeBase(context.Context, uint64, string) (*types.DatabaseSchemaSnapshot, error) {
	return nil, nil
}
func (s *stubLifecycleSchemaRepo) GetLatestSnapshotByDataSource(context.Context, uint64, string) (*types.DatabaseSchemaSnapshot, error) {
	return nil, nil
}
func (s *stubLifecycleSchemaRepo) ListColumnsByKnowledgeBase(context.Context, uint64, string) ([]*types.DatabaseTableColumn, error) {
	return nil, nil
}
func (s *stubLifecycleSchemaRepo) ListColumnsByTable(context.Context, uint64, string, string) ([]*types.DatabaseTableColumn, error) {
	return nil, nil
}
func (s *stubLifecycleSchemaRepo) DeleteSnapshotsByDataSource(_ context.Context, tenantID uint64, dataSourceID string) error {
	s.deletedTenantID = tenantID
	s.deletedDataSourceID = dataSourceID
	return nil
}

type stubLifecycleDBConnector struct{ invalidated int }

func (s *stubLifecycleDBConnector) Type() string { return types.DatabaseTypeMySQL }
func (s *stubLifecycleDBConnector) Validate(context.Context, *types.DatabaseConnectionConfig) error {
	return nil
}
func (s *stubLifecycleDBConnector) DiscoverSchema(context.Context, *types.DatabaseConnectionConfig) (*types.DatabaseSchema, error) {
	return nil, nil
}
func (s *stubLifecycleDBConnector) Query(context.Context, *types.DatabaseConnectionConfig, string, time.Duration) (*sql.Rows, error) {
	return nil, nil
}
func (s *stubLifecycleDBConnector) Dialect() types.SQLDialect { return types.SQLDialectMySQL }
func (s *stubLifecycleDBConnector) Invalidate(context.Context, *types.DatabaseConnectionConfig) error {
	s.invalidated++
	return nil
}

type stubLifecycleConnector struct{}

func (s *stubLifecycleConnector) Type() string                                            { return types.DatabaseTypeMySQL }
func (s *stubLifecycleConnector) Validate(context.Context, *types.DataSourceConfig) error { return nil }
func (s *stubLifecycleConnector) ListResources(context.Context, *types.DataSourceConfig) ([]types.Resource, error) {
	return nil, nil
}
func (s *stubLifecycleConnector) FetchAll(context.Context, *types.DataSourceConfig, []string) ([]types.FetchedItem, error) {
	return nil, nil
}
func (s *stubLifecycleConnector) FetchIncremental(context.Context, *types.DataSourceConfig, *types.SyncCursor) ([]types.FetchedItem, *types.SyncCursor, error) {
	return nil, nil, nil
}

func buildLifecycleDatabaseDataSource(t *testing.T, id string, host string, allowlist []string) *types.DataSource {
	t.Helper()
	ds := &types.DataSource{
		ID:              id,
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		Type:            types.DatabaseTypeMySQL,
		Status:          types.DataSourceStatusActive,
	}
	require.NoError(t, ds.SetDatabaseConnectionConfig(&types.DatabaseConnectionConfig{
		Type:        types.DatabaseTypeMySQL,
		Credentials: types.DatabaseCredentials{Username: "readonly", Password: "secret"},
		Settings: types.DatabaseSourceSettings{
			Host:           host,
			Port:           3306,
			Database:       "crm",
			TableAllowlist: allowlist,
		},
	}))
	return ds
}

func TestDataSourceServiceUpdateDataSourceRefreshesSchemaForDatabaseChanges(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", testAESKey)
	t.Setenv("APP_ENV", "development")

	existing := buildLifecycleDatabaseDataSource(t, "ds-1", "127.0.0.1", []string{"orders"})
	updated := &types.DataSource{
		ID:              "ds-1",
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		Type:            types.DatabaseTypeMySQL,
		Config: types.JSON(`{
			"type":"mysql",
			"credentials":{"username":"readonly","password":"***"},
			"settings":{"host":"127.0.0.2","database":"crm","port":3306,"table_allowlist":["orders","customers"]}
		}`),
	}

	dsRepo := &stubLifecycleDataSourceRepo{byID: map[string]*types.DataSource{"ds-1": existing}}
	schemaRegistry := &stubLifecycleSchemaRegistry{}
	schemaRepo := &stubLifecycleSchemaRepo{}
	dbRegistry := databaseconnector.NewRegistry()
	stubDBConnector := &stubLifecycleDBConnector{}
	require.NoError(t, dbRegistry.Register(stubDBConnector))
	connectorRegistry := datasource.NewConnectorRegistry()
	require.NoError(t, connectorRegistry.Register(&stubLifecycleConnector{}))

	svc := &DataSourceService{
		dsRepo:            dsRepo,
		syncLogRepo:       &stubLifecycleSyncLogRepo{},
		connectorRegistry: connectorRegistry,
		databaseRegistry:  dbRegistry,
		schemaRegistry:    schemaRegistry,
		schemaRepo:        schemaRepo,
		scheduler:         newLifecycleScheduler(),
	}

	result, err := svc.UpdateDataSource(context.Background(), updated)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "ds-1", schemaRegistry.refreshID)
	assert.Equal(t, 1, stubDBConnector.invalidated)
	require.NotNil(t, dsRepo.updated)
	parsed, parseErr := dsRepo.updated.ParseDatabaseConnectionConfig()
	require.NoError(t, parseErr)
	assert.Equal(t, "127.0.0.2", parsed.Settings.Host)
	assert.Equal(t, []string{"orders", "customers"}, parsed.Settings.TableAllowlist)
}

func TestDataSourceServiceDeleteDataSourceCleansSchemaAndInvalidatesConnector(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", testAESKey)
	t.Setenv("APP_ENV", "development")

	existing := buildLifecycleDatabaseDataSource(t, "ds-1", "127.0.0.1", []string{"orders"})
	dsRepo := &stubLifecycleDataSourceRepo{byID: map[string]*types.DataSource{"ds-1": existing}}
	syncRepo := &stubLifecycleSyncLogRepo{}
	schemaRepo := &stubLifecycleSchemaRepo{}
	dbRegistry := databaseconnector.NewRegistry()
	stubDBConnector := &stubLifecycleDBConnector{}
	require.NoError(t, dbRegistry.Register(stubDBConnector))

	svc := &DataSourceService{
		dsRepo:           dsRepo,
		syncLogRepo:      syncRepo,
		databaseRegistry: dbRegistry,
		schemaRepo:       schemaRepo,
		scheduler:        newLifecycleScheduler(),
	}

	err := svc.DeleteDataSource(context.Background(), "ds-1")
	require.NoError(t, err)
	assert.Equal(t, "ds-1", dsRepo.deletedID)
	assert.Equal(t, "ds-1", syncRepo.canceledID)
	assert.Equal(t, uint64(1), schemaRepo.deletedTenantID)
	assert.Equal(t, "ds-1", schemaRepo.deletedDataSourceID)
	assert.Equal(t, 1, stubDBConnector.invalidated)
}
