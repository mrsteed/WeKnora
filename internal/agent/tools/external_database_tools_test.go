package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubToolSchemaRegistry struct {
	schema       *types.DatabaseSchema
	promptSchema string
	buildCalls   int
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
	s.buildCalls++
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
	columns := make([]types.ColumnSchema, 0, 10)
	for index := 0; index < 10; index++ {
		columns = append(columns, types.ColumnSchema{
			Name:        "col_" + string(rune('a'+index)),
			DataType:    "varchar",
			Comment:     "comment " + string(rune('a'+index)),
			IsSensitive: index == 9,
		})
	}
	tool := NewExternalDatabaseSchemaTool(&stubToolSchemaRegistry{
		schema: &types.DatabaseSchema{
			DatabaseName: "crm",
			SchemaName:   "public",
			Tables: []types.TableSchema{{
				Name:        "orders",
				Type:        "table",
				PrimaryKeys: []string{"id"},
				Columns:     columns,
			}},
		},
		promptSchema: "Database: crm\nTables:\n- orders (table)",
	}, types.SearchTargets{{KnowledgeBaseID: "kb-db"}})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_base_id":"kb-db"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "External Database Schema")
	assert.Contains(t, result.Output, "Database: crm")
	assert.Contains(t, result.Output, "Allowed Query Scope")
	assert.Equal(t, "external_database_schema", result.Data["display_type"])
	assert.Equal(t, []string{"orders"}, result.Data["allowed_tables"])
	assert.NotEmpty(t, result.Data["sample_queries"])
	assert.NotContains(t, result.Data, "prompt_schema")
	assert.Equal(t, 1, tool.schemaRegistry.(*stubToolSchemaRegistry).buildCalls)

	tables, ok := result.Data["tables"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, tables, 1)
	assert.Equal(t, 10, tables[0]["column_count"])
	assert.Equal(t, 2, tables[0]["additional_columns_omitted"])
	assert.Equal(t, 1, tables[0]["sensitive_column_count"])
	dataColumns, ok := tables[0]["columns"].([]map[string]interface{})
	require.True(t, ok)
	assert.Len(t, dataColumns, 8)
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
	assert.Contains(t, result.Output, "Data Details")
	assert.Contains(t, result.Output, "--- Record #1 ---")
	assert.Contains(t, result.Output, "id: 1")
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
	assert.Contains(t, output, "Data Details")
	assert.Contains(t, output, "--- Record #1 ---")
}

func TestToolRegistry_DoesNotTruncateExternalDatabaseQueryOutput(t *testing.T) {
	service := &stubToolStructuredQueryService{result: &types.QueryResult{
		Columns:     []types.QueryColumn{{Name: "payload", DataType: "text"}},
		Rows:        []map[string]any{{"payload": strings.Repeat("x", 120)}},
		RowCount:    1,
		Truncated:   false,
		DurationMS:  9,
		ExecutedSQL: "SELECT payload FROM orders LIMIT 1",
	}}
	registry := NewToolRegistry()
	registry.SetMaxToolOutputSize(40)
	registry.RegisterTool(NewExternalDatabaseQueryTool(service, types.SearchTargets{{KnowledgeBaseID: "kb-db"}}))

	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-1")

	result, err := registry.ExecuteTool(ctx, ToolExternalDatabaseQuery, json.RawMessage(`{"knowledge_base_id":"kb-db","sql":"SELECT payload FROM orders LIMIT 1","purpose":"test"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.NotContains(t, result.Output, "output truncated")
	assert.Contains(t, result.Output, strings.Repeat("x", 120))
}
