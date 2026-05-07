package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubToolSchemaRegistry struct {
	schema       *types.DatabaseSchema
	promptSchema string
	buildCalls   int
	getCalls     int
	buildUsesGet bool
	err          error
}

func (s *stubToolSchemaRegistry) RefreshSchema(context.Context, string) error { return nil }
func (s *stubToolSchemaRegistry) GetDatabaseSchema(context.Context, string) (*types.DatabaseSchema, error) {
	s.getCalls++
	return s.schema, s.err
}
func (s *stubToolSchemaRegistry) GetTableSchema(context.Context, string, string) (*types.TableSchema, error) {
	return nil, s.err
}
func (s *stubToolSchemaRegistry) BuildPromptSchema(context.Context, string, []string) (string, error) {
	s.buildCalls++
	if s.buildUsesGet {
		s.getCalls++
	}
	return s.promptSchema, s.err
}
func (s *stubToolSchemaRegistry) BuildPromptSchemaResult(_ context.Context, _ string, selectedTables []string, opts types.PromptSchemaOptions) (*types.PromptSchemaBuildResult, error) {
	s.buildCalls++
	s.getCalls++
	if s.err != nil {
		return nil, s.err
	}
	mode := opts.Mode
	if mode == "" {
		mode = types.PromptSchemaModeAuto
	}
	prompt := s.promptSchema
	tables := s.schema.Tables
	if len(selectedTables) > 0 {
		selectedSet := make(map[string]struct{}, len(selectedTables))
		for _, table := range selectedTables {
			selectedSet[table] = struct{}{}
		}
		filtered := make([]types.TableSchema, 0, len(tables))
		for _, table := range tables {
			if _, ok := selectedSet[strings.ToLower(table.Name)]; ok {
				filtered = append(filtered, table)
			}
		}
		tables = filtered
		if len(selectedTables) > 0 && len(tables) > 0 && mode == types.PromptSchemaModeAuto {
			mode = types.PromptSchemaModeDetail
		}
	}
	if mode == types.PromptSchemaModeAuto {
		mode = types.PromptSchemaModeDetail
		if len(tables) > 12 {
			mode = types.PromptSchemaModeCatalog
		}
	}
	if prompt == "" && s.schema != nil {
		prompt = fmt.Sprintf("Database: %s\nSchema output mode: %s", s.schema.DatabaseName, mode)
		if mode == types.PromptSchemaModeCatalog {
			prompt += "\nCatalog view: representative columns are shown first."
		}
		prompt += "\nQuery planning rules:\n"
		prompt += "- Add LIMIT to any query that can return multiple rows. This includes detail previews, JOIN inspections, DISTINCT value lists, GROUP BY/HAVING summaries, ORDER BY top-N checks, window-function queries, and multi-row CTE outputs.\n"
		prompt += "- Only pure global aggregates that return one row may omit LIMIT, such as COUNT(*), SUM(amount), AVG(score), MIN(created_at), MAX(created_at), or DISTINCT COUNT(*), with no GROUP BY and no window clause.\n"
		prompt += "- For exploratory inspection, start with LIMIT 10 or LIMIT 20 and tighten WHERE conditions before widening scope."
	}
	displayTables := append([]types.TableSchema(nil), tables...)
	if mode == types.PromptSchemaModeCatalog && len(displayTables) > schemaOutputTableLimit {
		displayTables = displayTables[:schemaOutputTableLimit]
	}
	columnCount := 0
	displayedColumns := 0
	for _, table := range tables {
		columnCount += len(table.Columns)
	}
	if mode == types.PromptSchemaModeCatalog {
		for _, table := range displayTables {
			displayedColumns += minInt(len(table.Columns), schemaCatalogColumnLimit)
		}
	}
	additionalColumns := 0
	if mode == types.PromptSchemaModeCatalog {
		additionalColumns = maxInt(columnCount-displayedColumns, 0)
	}
	return &types.PromptSchemaBuildResult{
		Prompt:                   prompt,
		Mode:                     mode,
		DatabaseName:             s.schema.DatabaseName,
		SchemaName:               s.schema.SchemaName,
		SchemaHash:               s.schema.SchemaHash,
		RefreshedAt:              s.schema.RefreshedAt,
		AllTables:                append([]types.TableSchema(nil), tables...),
		DisplayTables:            displayTables,
		PossibleJoinHints:        append([]string(nil), s.schema.BusinessJoinHints...),
		TableCount:               len(tables),
		ColumnCount:              columnCount,
		AdditionalTablesOmitted:  maxInt(len(tables)-len(displayTables), 0),
		AdditionalColumnsOmitted: additionalColumns,
	}, nil
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
	assert.True(t, seen[ToolExternalDatabaseSearchTables])
	assert.True(t, seen[ToolExternalDatabaseQuery])
	assert.Equal(t, ToolRequirement{AllOf: []KBCapability{CapDatabase}}, ToolCapabilityRequirements[ToolExternalDatabaseSchema])
	assert.Equal(t, ToolRequirement{AllOf: []KBCapability{CapDatabase}}, ToolCapabilityRequirements[ToolExternalDatabaseSearchTables])
	assert.Equal(t, ToolRequirement{AllOf: []KBCapability{CapDatabase}}, ToolCapabilityRequirements[ToolExternalDatabaseQuery])
	assert.LessOrEqual(t, len(ToolExternalDatabaseSchema), maxFunctionNameLength)
	assert.LessOrEqual(t, len(ToolExternalDatabaseSearchTables), maxFunctionNameLength)
	assert.LessOrEqual(t, len(ToolExternalDatabaseQuery), maxFunctionNameLength)
}

func TestExternalDatabaseSearchTablesToolExecuteSuccess(t *testing.T) {
	tool := NewExternalDatabaseSearchTablesTool(&stubToolSchemaRegistry{
		schema: &types.DatabaseSchema{
			DatabaseName: "crm",
			SchemaName:   "public",
			SchemaHash:   "hash-1",
			Tables: []types.TableSchema{
				{Name: "emergency_plans", Type: "table", Comment: "预案主数据", Columns: []types.ColumnSchema{{Name: "id", DataType: "bigint"}, {Name: "plan_name", DataType: "varchar", Comment: "预案名称"}}},
				{Name: "emergency_plan_calls", Type: "table", Comment: "预案调用记录", Columns: []types.ColumnSchema{{Name: "plan_id", DataType: "bigint", Comment: "预案ID"}, {Name: "call_count", DataType: "int"}}},
				{Name: "orders", Type: "table", Comment: "订单表", Columns: []types.ColumnSchema{{Name: "id", DataType: "bigint"}}},
			},
		},
	}, types.SearchTargets{{KnowledgeBaseID: "kb-db"}})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_base_id":"kb-db","keyword":"预案","limit":5}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "External Database Table Search")
	assert.Contains(t, result.Output, "Matched table count: 2")
	assert.Contains(t, result.Output, "emergency_plan_calls")
	assert.Contains(t, result.Output, "Match reasons")
	assert.Equal(t, "external_database_search_tables", result.Data["display_type"])
	assert.Equal(t, 3, result.Data["scope_table_count"])
	assert.Equal(t, 2, result.Data["matched_table_count"])
	assert.Equal(t, 2, result.Data["returned_hit_count"])
	assert.Equal(t, "预案", result.Data["keyword"])

	matchedTables, ok := result.Data["matched_tables"].([]string)
	require.True(t, ok)
	require.Len(t, matchedTables, 2)
	assert.Contains(t, matchedTables, "emergency_plans")
	assert.Contains(t, matchedTables, "emergency_plan_calls")

	results, ok := result.Data["results"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, results, 2)
	assert.Equal(t, "emergency_plan_calls", results[0]["table_name"])
	assert.Equal(t, "fact_log", results[0]["likely_role"])
	assert.NotEmpty(t, results[0]["matched_columns"])
	assert.NotEmpty(t, results[0]["match_reasons"])
}

func TestExternalDatabaseSearchTablesToolRequiresFilter(t *testing.T) {
	tool := NewExternalDatabaseSearchTablesTool(&stubToolSchemaRegistry{
		schema: &types.DatabaseSchema{DatabaseName: "crm", Tables: []types.TableSchema{{Name: "orders", Type: "table"}}},
	}, types.SearchTargets{{KnowledgeBaseID: "kb-db"}})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_base_id":"kb-db"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "at least one search filter is required")
}

func TestExternalDatabaseSearchTablesToolMatchedTablesKeepFullSetWhenResultsAreLimited(t *testing.T) {
	tables := make([]types.TableSchema, 0, 5)
	for index := 0; index < 5; index++ {
		tables = append(tables, types.TableSchema{
			Name:    fmt.Sprintf("plan_table_%d", index),
			Type:    "table",
			Comment: "预案相关表",
			Columns: []types.ColumnSchema{{Name: "plan_id", DataType: "bigint", Comment: "预案ID"}},
		})
	}
	tool := NewExternalDatabaseSearchTablesTool(&stubToolSchemaRegistry{
		schema: &types.DatabaseSchema{DatabaseName: "crm", SchemaName: "public", Tables: tables},
	}, types.SearchTargets{{KnowledgeBaseID: "kb-db"}})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_base_id":"kb-db","keyword":"预案","limit":2}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, 5, result.Data["matched_table_count"])
	assert.Equal(t, 2, result.Data["returned_hit_count"])
	assert.Equal(t, 3, result.Data["additional_matches_omitted"])

	matchedTables, ok := result.Data["matched_tables"].([]string)
	require.True(t, ok)
	require.Len(t, matchedTables, 5)
	assert.Contains(t, matchedTables, "plan_table_4")

	results, ok := result.Data["results"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, results, 2)
	assert.Contains(t, result.Output, "Additional matches omitted: 3")
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
			DatabaseName:      "crm",
			SchemaName:        "public",
			SchemaHash:        "hash-1",
			BusinessJoinHints: []string{"orders.customer_id = customers.id"},
			Tables: []types.TableSchema{{
				Name:        "orders",
				Type:        "table",
				PrimaryKeys: []string{"id"},
				ForeignKeys: []types.ForeignKeySchema{{Name: "fk_orders_customer", Columns: []string{"customer_id"}, ReferencedTable: "customers", ReferencedColumns: []string{"id"}}},
				Columns:     columns,
			}},
		},
	}, types.SearchTargets{{KnowledgeBaseID: "kb-db"}})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_base_id":"kb-db"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "External Database Schema")
	assert.Contains(t, result.Output, "Database: crm")
	assert.Contains(t, result.Output, "Schema output mode: detail")
	assert.Contains(t, result.Output, "Query planning rules:")
	assert.Contains(t, result.Output, "LIMIT 10 or LIMIT 20")
	assert.Contains(t, result.Output, "Allowed Query Scope")
	assert.Contains(t, result.Output, "Foreign Keys")
	assert.Contains(t, result.Output, "Possible Join Hints")
	assert.Equal(t, "external_database_schema", result.Data["display_type"])
	assert.Equal(t, []string{"orders"}, result.Data["allowed_tables"])
	assert.Equal(t, []string{"orders.customer_id -> customers.id"}, result.Data["foreign_keys"])
	assert.Equal(t, []string{"orders.customer_id = customers.id"}, result.Data["possible_join_hints"])
	assert.NotEmpty(t, result.Data["sample_queries"])
	assert.NotContains(t, result.Data, "prompt_schema")
	assert.Equal(t, 1, tool.schemaRegistry.(*stubToolSchemaRegistry).buildCalls)
	assert.Equal(t, 1, tool.schemaRegistry.(*stubToolSchemaRegistry).getCalls)
	assert.Equal(t, "detail", result.Data["mode"])
	assert.Equal(t, "hash-1", result.Data["schema_hash"])

	tables, ok := result.Data["tables"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, tables, 1)
	assert.Equal(t, 10, tables[0]["column_count"])
	assert.Equal(t, 0, tables[0]["additional_columns_omitted"])
	assert.Equal(t, 1, tables[0]["sensitive_column_count"])
	dataColumns, ok := tables[0]["columns"].([]map[string]interface{})
	require.True(t, ok)
	assert.Len(t, dataColumns, 10)
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
	assert.Equal(t, false, result.Data["output_truncated"])
	assert.Equal(t, 0, result.Data["cell_truncated_count"])
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
	output := formatExternalDatabaseQueryOutput([]string{"id"}, []map[string]any{{"id": 1}}, 1, true, int64((2 * time.Second).Milliseconds()), "SELECT id FROM orders LIMIT 1")
	assert.Contains(t, output.output, "truncated")
	assert.Contains(t, output.output, "Data Details")
	assert.Contains(t, output.output, "--- Record #1 ---")
}

func TestToolRegistry_FallbackTruncatesExternalDatabaseQueryOutput(t *testing.T) {
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
	assert.LessOrEqual(t, utf8.RuneCountInString(result.Output), 40)
	assert.NotContains(t, result.Output, strings.Repeat("x", 120))
}

func TestExternalDatabaseSchemaToolUsesSingleSchemaLoad(t *testing.T) {
	registry := &stubToolSchemaRegistry{
		schema: &types.DatabaseSchema{
			DatabaseName: "crm",
			SchemaName:   "public",
			Tables: []types.TableSchema{{
				Name:    "orders",
				Type:    "table",
				Columns: []types.ColumnSchema{{Name: "id", DataType: "bigint"}},
			}},
		},
	}
	tool := NewExternalDatabaseSchemaTool(registry, types.SearchTargets{{KnowledgeBaseID: "kb-db"}})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_base_id":"kb-db"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, registry.getCalls)
	assert.Equal(t, 1, registry.buildCalls)
}

func TestExternalDatabaseSchemaToolAutoModeUsesCatalogForLargeSchema(t *testing.T) {
	tables := make([]types.TableSchema, 0, 21)
	for tableIndex := 0; tableIndex < 21; tableIndex++ {
		columns := make([]types.ColumnSchema, 0, 8)
		for columnIndex := 0; columnIndex < 8; columnIndex++ {
			columns = append(columns, types.ColumnSchema{Name: fmt.Sprintf("col_%d", columnIndex), DataType: "varchar"})
		}
		tables = append(tables, types.TableSchema{Name: fmt.Sprintf("table_%02d", tableIndex), Type: "table", Columns: columns})
	}
	tool := NewExternalDatabaseSchemaTool(&stubToolSchemaRegistry{
		schema: &types.DatabaseSchema{DatabaseName: "crm", SchemaName: "public", Tables: tables},
	}, types.SearchTargets{{KnowledgeBaseID: "kb-db"}})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_base_id":"kb-db","mode":"auto"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "catalog", result.Data["mode"])
	assert.Equal(t, 21, result.Data["table_count"])
	assert.Equal(t, 20, result.Data["display_table_count"])
	assert.Equal(t, 1, result.Data["additional_tables_omitted"])
	assert.Equal(t, 48, result.Data["additional_columns_omitted"])
	assert.Contains(t, result.Output, "Catalog view")
	assert.Contains(t, result.Output, "Current view table count: 20 / 21")
	assert.Contains(t, result.Output, "external_database_search_tables")

	allowedTables, ok := result.Data["allowed_tables"].([]string)
	require.True(t, ok)
	assert.Len(t, allowedTables, 21)
	assert.Contains(t, allowedTables, "table_20")

	dataTables, ok := result.Data["tables"].([]map[string]interface{})
	require.True(t, ok)
	require.NotEmpty(t, dataTables)
	assert.Len(t, dataTables, schemaOutputTableLimit)
	dataColumns, ok := dataTables[0]["columns"].([]map[string]interface{})
	require.True(t, ok)
	assert.Len(t, dataColumns, schemaCatalogColumnLimit)
}

func TestExternalDatabaseSchemaToolAutoModeUsesDetailForSelectedTables(t *testing.T) {
	tool := NewExternalDatabaseSchemaTool(&stubToolSchemaRegistry{
		schema: &types.DatabaseSchema{DatabaseName: "crm", SchemaName: "public", Tables: []types.TableSchema{
			{Name: "orders", Type: "table", Columns: []types.ColumnSchema{{Name: "id", DataType: "bigint"}, {Name: "status", DataType: "varchar"}, {Name: "created_at", DataType: "timestamp"}, {Name: "amount", DataType: "decimal"}, {Name: "note", DataType: "text"}, {Name: "region", DataType: "varchar"}, {Name: "channel", DataType: "varchar"}}},
			{Name: "customers", Type: "table", Columns: []types.ColumnSchema{{Name: "id", DataType: "bigint"}}},
		}},
	}, types.SearchTargets{{KnowledgeBaseID: "kb-db"}})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_base_id":"kb-db","tables":["orders"],"mode":"auto"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "detail", result.Data["mode"])
	assert.Equal(t, 1, result.Data["table_count"])

	dataTables, ok := result.Data["tables"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, dataTables, 1)
	dataColumns, ok := dataTables[0]["columns"].([]map[string]interface{})
	require.True(t, ok)
	assert.Len(t, dataColumns, 7)
	assert.Equal(t, 0, dataTables[0]["additional_columns_omitted"])
}

func TestExternalDatabaseSchemaToolTableNameLikeFindsTablesOutsideCatalogWindow(t *testing.T) {
	tables := make([]types.TableSchema, 0, 30)
	for tableIndex := 0; tableIndex < 30; tableIndex++ {
		table := types.TableSchema{
			Name:    fmt.Sprintf("table_%02d", tableIndex),
			Type:    "table",
			Columns: []types.ColumnSchema{{Name: "id", DataType: "bigint"}},
		}
		if tableIndex == 27 {
			table.Name = "emergency_plan_calls"
			table.Comment = "预案调用记录"
			table.Columns = []types.ColumnSchema{{Name: "plan_id", DataType: "bigint", Comment: "预案ID"}, {Name: "call_count", DataType: "int"}}
		}
		tables = append(tables, table)
	}
	tool := NewExternalDatabaseSchemaTool(&stubToolSchemaRegistry{
		schema: &types.DatabaseSchema{DatabaseName: "crm", SchemaName: "public", Tables: tables},
	}, types.SearchTargets{{KnowledgeBaseID: "kb-db"}})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_base_id":"kb-db","mode":"auto","table_name_like":"plan"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "Matched tables")
	assert.Contains(t, result.Output, "emergency_plan_calls")
	assert.Equal(t, 1, result.Data["matched_table_count"])
	assert.Equal(t, 30, result.Data["scope_table_count"])
	assert.Equal(t, "plan", result.Data["table_name_like"])

	matchedTables, ok := result.Data["matched_tables"].([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"emergency_plan_calls"}, matchedTables)

	dataTables, ok := result.Data["tables"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, dataTables, 1)
	assert.Equal(t, "emergency_plan_calls", dataTables[0]["name"])
	assert.Equal(t, 2, dataTables[0]["column_count"])
}

func TestExternalDatabaseSchemaToolListOnlyReturnsFullTableList(t *testing.T) {
	tables := make([]types.TableSchema, 0, 25)
	for tableIndex := 0; tableIndex < 25; tableIndex++ {
		tables = append(tables, types.TableSchema{
			Name:    fmt.Sprintf("table_%02d", tableIndex),
			Type:    "table",
			Columns: []types.ColumnSchema{{Name: "id", DataType: "bigint"}},
		})
	}
	tool := NewExternalDatabaseSchemaTool(&stubToolSchemaRegistry{
		schema: &types.DatabaseSchema{DatabaseName: "crm", SchemaName: "public", Tables: tables},
	}, types.SearchTargets{{KnowledgeBaseID: "kb-db"}})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_base_id":"kb-db","mode":"auto","list_only":true}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "Full table list returned: 25")
	assert.Contains(t, result.Output, "table_24")
	assert.NotContains(t, result.Output, "Additional tables omitted from scope list")
	assert.Equal(t, true, result.Data["list_only"])
	assert.Equal(t, 25, result.Data["matched_table_count"])

	matchedTables, ok := result.Data["matched_tables"].([]string)
	require.True(t, ok)
	require.Len(t, matchedTables, 25)
	assert.Equal(t, "table_24", matchedTables[24])

	dataTables, ok := result.Data["tables"].([]map[string]interface{})
	require.True(t, ok)
	assert.Len(t, dataTables, 0)
}

func TestExternalDatabaseQueryOutputCellBudget(t *testing.T) {
	longCell := strings.Repeat("payload-", 200)
	service := &stubToolStructuredQueryService{result: &types.QueryResult{
		Columns:     []types.QueryColumn{{Name: "payload", DataType: "text"}},
		Rows:        []map[string]any{{"payload": longCell}},
		RowCount:    1,
		Truncated:   false,
		DurationMS:  9,
		ExecutedSQL: "SELECT payload FROM orders LIMIT 1",
	}}
	tool := NewExternalDatabaseQueryTool(service, types.SearchTargets{{KnowledgeBaseID: "kb-db"}})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_base_id":"kb-db","sql":"SELECT payload FROM orders LIMIT 1","purpose":"test"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotContains(t, result.Output, longCell)
	assert.Equal(t, 1, result.Data["cell_truncated_count"])
	assert.Equal(t, true, result.Data["output_truncated"])
}

func TestExternalDatabaseQueryOutputBudgetLimitsRowsForModel(t *testing.T) {
	rows := make([]map[string]any, 0, 35)
	for index := 0; index < 35; index++ {
		rows = append(rows, map[string]any{"id": index + 1, "payload": fmt.Sprintf("row-%02d", index+1)})
	}
	service := &stubToolStructuredQueryService{result: &types.QueryResult{
		Columns:     []types.QueryColumn{{Name: "id", DataType: "bigint"}, {Name: "payload", DataType: "text"}},
		Rows:        rows,
		RowCount:    35,
		Truncated:   false,
		DurationMS:  11,
		ExecutedSQL: "SELECT id, payload FROM orders LIMIT 35",
	}}
	tool := NewExternalDatabaseQueryTool(service, types.SearchTargets{{KnowledgeBaseID: "kb-db"}})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_base_id":"kb-db","sql":"SELECT id, payload FROM orders LIMIT 35","purpose":"test"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Contains(t, result.Output, "Rows available to model: 30 / 35")
	assert.NotContains(t, result.Output, "--- Record #31 ---")
	assert.Equal(t, true, result.Data["output_truncated"])
	assert.Equal(t, 35, result.Data["display_row_count"])
}

func TestExternalDatabaseQueryDoesNotMarkOutputBudgetTruncatedForDatabaseOnlyTruncation(t *testing.T) {
	service := &stubToolStructuredQueryService{result: &types.QueryResult{
		Columns:     []types.QueryColumn{{Name: "id", DataType: "bigint"}},
		Rows:        []map[string]any{{"id": 1}},
		RowCount:    1,
		Truncated:   true,
		DurationMS:  15,
		ExecutedSQL: "SELECT id FROM orders LIMIT 1",
	}}
	tool := NewExternalDatabaseQueryTool(service, types.SearchTargets{{KnowledgeBaseID: "kb-db"}})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_base_id":"kb-db","sql":"SELECT id FROM orders LIMIT 1","purpose":"test"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Contains(t, result.Output, "database result truncated")
	assert.NotContains(t, result.Output, "Output budget truncated: true")
	assert.Equal(t, true, result.Data["truncated"])
	assert.Equal(t, false, result.Data["output_truncated"])
}
