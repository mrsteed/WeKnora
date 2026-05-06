package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectSQLReferencesCollectsClauseSpecificColumns(t *testing.T) {
	refs, err := collectSQLReferences(
		"SELECT orders.id FROM orders JOIN customers ON orders.customer_id = customers.id WHERE orders.id > 10 GROUP BY orders.id HAVING MAX(customers.tier) IS NOT NULL ORDER BY customers.email LIMIT 10",
		buildSQLGuardPolicy(&types.DatabaseSchema{Tables: []types.TableSchema{{
			Name:    "orders",
			Columns: []types.ColumnSchema{{Name: "id"}, {Name: "customer_id"}},
		}, {
			Name:    "customers",
			Columns: []types.ColumnSchema{{Name: "id"}, {Name: "tier"}, {Name: "email"}},
		}}}, nil, types.SQLDialectPostgreSQL, 0, 0),
	)
	require.NoError(t, err)
	assert.True(t, hasCollectedColumn(refs.Columns, "orders", "customer_id", "join_on"))
	assert.True(t, hasCollectedColumn(refs.Columns, "customers", "id", "join_on"))
	assert.True(t, hasCollectedColumn(refs.Columns, "customers", "tier", "having"))
	assert.True(t, hasCollectedColumn(refs.Columns, "customers", "email", "order_by"))
	assert.True(t, refs.HasGroupBy)
	assert.True(t, refs.HasExplicitLimit)
}

func TestCollectSQLReferencesRejectsAmbiguousUnqualifiedColumn(t *testing.T) {
	_, err := collectSQLReferences(
		"SELECT orders.id FROM orders JOIN shipments ON orders.id = shipments.id ORDER BY status LIMIT 10",
		buildSQLGuardPolicy(&types.DatabaseSchema{Tables: []types.TableSchema{{
			Name:    "orders",
			Columns: []types.ColumnSchema{{Name: "id"}, {Name: "status"}},
		}, {
			Name:    "shipments",
			Columns: []types.ColumnSchema{{Name: "id"}, {Name: "status"}},
		}}}, nil, types.SQLDialectPostgreSQL, 0, 0),
	)
	require.Error(t, err)
	assert.ErrorContains(t, err, "ambiguous")
}

func TestCollectSQLReferencesDetectsPureGlobalAggregate(t *testing.T) {
	refs, err := collectSQLReferences(
		"SELECT COALESCE(SUM(amount), 0) FROM orders",
		buildSQLGuardPolicy(&types.DatabaseSchema{Tables: []types.TableSchema{{
			Name:    "orders",
			Columns: []types.ColumnSchema{{Name: "amount"}},
		}}}, nil, types.SQLDialectPostgreSQL, 0, 0),
	)
	require.NoError(t, err)
	assert.True(t, refs.HasAggregate)
	assert.True(t, refs.PureGlobalAggregate)
	assert.False(t, refs.HasGroupBy)
	assert.False(t, refs.HasExplicitLimit)
}

func TestCollectSQLReferencesDetectsDistinctGlobalAggregateAsPureAggregate(t *testing.T) {
	refs, err := collectSQLReferences(
		"SELECT DISTINCT COUNT(*) FROM orders",
		buildSQLGuardPolicy(&types.DatabaseSchema{Tables: []types.TableSchema{{
			Name:    "orders",
			Columns: []types.ColumnSchema{{Name: "id"}},
		}}}, nil, types.SQLDialectPostgreSQL, 0, 0),
	)
	require.NoError(t, err)
	assert.True(t, refs.HasAggregate)
	assert.True(t, refs.HasDistinct)
	assert.True(t, refs.PureGlobalAggregate)
}

func TestCollectSQLReferencesCollectsBaseTableReferencesInsideCTE(t *testing.T) {
	refs, err := collectSQLReferences(
		"WITH recent_customers AS (SELECT phone FROM customers) SELECT phone FROM recent_customers LIMIT 10",
		buildSQLGuardPolicy(&types.DatabaseSchema{Tables: []types.TableSchema{{
			Name:    "customers",
			Columns: []types.ColumnSchema{{Name: "id"}, {Name: "phone"}},
		}}}, nil, types.SQLDialectPostgreSQL, 0, 0),
	)
	require.NoError(t, err)
	assert.True(t, hasCollectedColumn(refs.Columns, "customers", "phone", "select"))
	_, hasCTE := refs.CTEs["recent_customers"]
	assert.True(t, hasCTE)
}

func hasCollectedColumn(columns []sqlColumnReference, table string, column string, clause string) bool {
	for _, item := range columns {
		if item.Table == table && item.Column == column && item.Clause == clause {
			return true
		}
	}
	return false
}
