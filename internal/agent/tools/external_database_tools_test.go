package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubToolSchemaRegistry struct {
	schema       *types.DatabaseSchema
	promptSchema string
	err          error
}

func (s *stubToolSchemaRegistry) RefreshSchema(context.Context, string) error { return nil }
func (s *stubToolSchemaRegistry) GetDatabaseSchema(context.Context, string) (*types.DatabaseSchema, error) {
	return s.schema, s.err
}
func (s *stubToolSchemaRegistry) GetTableSchema(context.Context, string, string) (*types.TableSchema, error) {
	return nil, s.err
}
func (s *stubToolSchemaRegistry) BuildPromptSchema(context.Context, string, []string) (string, error) {
	return s.promptSchema, s.err
}

type stubToolStructuredQueryService struct {
	result *types.QueryResult
	err    error
	req    types.ExecuteQueryRequest
}

func (s *stubToolStructuredQueryService) ValidateSQL(context.Context, types.ValidateSQLRequest) (*types.ValidatedSQL, error) {
	return nil, s.err
}
func (s *stubToolStructuredQueryService) ExecuteQuery(_ context.Context, req types.ExecuteQueryRequest) (*types.QueryResult, error) {
	s.req = req
	return s.result, s.err
}
func (s *stubToolStructuredQueryService) ExplainQuery(context.Context, types.ExplainQueryRequest) (*types.QueryPlan, error) {
	return nil, s.err
}

func TestExternalDatabaseToolDefinitions(t *testing.T) {
	defs := AvailableToolDefinitions()
	seen := map[string]bool{}
	for _, def := range defs {
		seen[def.Name] = true
	}
	assert.True(t, seen[ToolExternalDatabaseSchema])
	assert.True(t, seen[ToolExternalDatabaseQuery])
	assert.Equal(t, ToolRequirement{AllOf: []KBCapability{CapDatabase}}, ToolCapabilityRequirements[ToolExternalDatabaseSchema])
	assert.Equal(t, ToolRequirement{AllOf: []KBCapability{CapDatabase}}, ToolCapabilityRequirements[ToolExternalDatabaseQuery])
	assert.LessOrEqual(t, len(ToolExternalDatabaseSchema), maxFunctionNameLength)
	assert.LessOrEqual(t, len(ToolExternalDatabaseQuery), maxFunctionNameLength)
}

func TestExternalDatabaseSchemaToolExecuteSuccess(t *testing.T) {
	tool := NewExternalDatabaseSchemaTool(&stubToolSchemaRegistry{
		schema: &types.DatabaseSchema{
			DatabaseName: "crm",
			SchemaName:   "public",
			Tables: []types.TableSchema{{
				Name:        "orders",
				Type:        "table",
				PrimaryKeys: []string{"id"},
				Columns:     []types.ColumnSchema{{Name: "id", DataType: "bigint"}, {Name: "customer_id", DataType: "bigint"}},
			}},
		},
		promptSchema: "Database: crm\nTables:\n- orders (table)",
	}, types.SearchTargets{{KnowledgeBaseID: "kb-db"}})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_base_id":"kb-db"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "External Database Schema")
	assert.Equal(t, "external_database_schema", result.Data["display_type"])
	assert.Equal(t, []string{"orders"}, result.Data["allowed_tables"])
	assert.NotEmpty(t, result.Data["sample_queries"])
}

func TestExternalDatabaseSchemaToolRejectsKBOutsideScope(t *testing.T) {
	tool := NewExternalDatabaseSchemaTool(&stubToolSchemaRegistry{}, types.SearchTargets{{KnowledgeBaseID: "kb-db"}})
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_base_id":"other-kb"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "outside the current agent scope")
}

func TestExternalDatabaseQueryToolExecuteSuccess(t *testing.T) {
	service := &stubToolStructuredQueryService{result: &types.QueryResult{
		Columns:     []types.QueryColumn{{Name: "id", DataType: "bigint"}},
		Rows:        []map[string]any{{"id": 1}},
		RowCount:    1,
		Truncated:   false,
		DurationMS:  12,
		ExecutedSQL: "SELECT id FROM orders LIMIT 10",
	}}
	tool := NewExternalDatabaseQueryTool(service, types.SearchTargets{{KnowledgeBaseID: "kb-db"}})
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(23))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-7")

	result, err := tool.Execute(ctx, json.RawMessage(`{"knowledge_base_id":"kb-db","sql":"SELECT id FROM orders LIMIT 10","purpose":"test"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, "external_database_query", result.Data["display_type"])
	assert.Equal(t, []string{"id"}, result.Data["columns"])
	assert.Equal(t, uint64(23), service.req.TenantID)
	assert.Equal(t, "user-7", service.req.UserID)
	assert.Equal(t, "kb-db", service.req.KnowledgeBaseID)
	assert.Equal(t, "test", service.req.Purpose)
	assert.Contains(t, result.Output, "Returned 1 rows in 12 ms")
}

func TestExternalDatabaseQueryToolRequiresPurpose(t *testing.T) {
	tool := NewExternalDatabaseQueryTool(&stubToolStructuredQueryService{}, types.SearchTargets{{KnowledgeBaseID: "kb-db"}})
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_base_id":"kb-db","sql":"SELECT 1 LIMIT 1"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "purpose is required")
}

func TestExternalDatabaseQueryToolRejectsKBOutsideScope(t *testing.T) {
	tool := NewExternalDatabaseQueryTool(&stubToolStructuredQueryService{}, types.SearchTargets{{KnowledgeBaseID: "kb-db"}})
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_base_id":"other-kb","sql":"SELECT 1 LIMIT 1"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "outside the current agent scope")
}

func TestFormatExternalDatabaseQueryOutputIncludesTruncation(t *testing.T) {
	output := formatExternalDatabaseQueryOutput([]string{"id"}, []map[string]any{{"id": 1}}, 1, true, int64((2 * time.Second).Milliseconds()))
	assert.Contains(t, output, "truncated")
	assert.Contains(t, output, "Record #1")
}
