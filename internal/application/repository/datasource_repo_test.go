package repository

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataSourceRepositoryFindActiveIncludesDatabaseSchemaRefreshCronSources(t *testing.T) {
	ctx := context.Background()
	db := setupDatabaseSchemaRepoDB(t)
	repo := NewDataSourceRepository(db)

	docSource := &types.DataSource{
		ID:           "doc-active",
		TenantID:     1,
		Status:       types.DataSourceStatusActive,
		Type:         types.ConnectorTypeFeishu,
		SyncSchedule: "0 */5 * * * *",
	}
	dbSource := &types.DataSource{
		ID:       "db-active",
		TenantID: 1,
		Status:   types.DataSourceStatusActive,
		Type:     types.DatabaseTypeMySQL,
		Config: types.JSON(`{
			"type":"mysql",
			"credentials":{"username":"readonly","password":"secret"},
			"settings":{"host":"127.0.0.1","database":"crm","port":3306,"schema_refresh_cron":"0 */30 * * * *"}
		}`),
	}
	pausedDBSource := &types.DataSource{
		ID:       "db-paused",
		TenantID: 1,
		Status:   types.DataSourceStatusPaused,
		Type:     types.DatabaseTypeMySQL,
	}

	for _, ds := range []*types.DataSource{docSource, dbSource, pausedDBSource} {
		require.NoError(t, repo.Create(ctx, ds))
	}

	activeSources, err := repo.FindActive(ctx)
	require.NoError(t, err)
	require.Len(t, activeSources, 2)
	assert.Equal(t, []string{"db-active", "doc-active"}, []string{activeSources[0].ID, activeSources[1].ID})
}
