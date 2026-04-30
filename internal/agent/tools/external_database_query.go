package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/Tencent/WeKnora/internal/utils"
)

var externalDatabaseQueryTool = BaseTool{
	name: ToolExternalDatabaseQuery,
	description: `Execute a read-only SQL query against a database knowledge base.

Use this tool only after checking the schema with external_database_schema.

Rules:
- Only SELECT or WITH ... SELECT queries are allowed.
- Use only tables and columns already returned by external_database_schema.
- Always include aggregation or an explicit LIMIT clause.
- Sensitive fields and disallowed columns are rejected.
- DDL and DML statements are forbidden.
`,
	schema: utils.GenerateSchema[ExternalDatabaseQueryInput](),
}

type ExternalDatabaseQueryInput struct {
	KnowledgeBaseID string `json:"knowledge_base_id" jsonschema:"Database knowledge base ID to query."`
	SQL             string `json:"sql" jsonschema:"Read-only SQL to execute against the external database. Must be SELECT-only and include LIMIT or aggregation."`
	Purpose         string `json:"purpose" jsonschema:"Short explanation of why this query is being executed."`
	MaxRows         int    `json:"max_rows,omitempty" jsonschema:"Optional tighter max rows for this query."`
	TimeoutSeconds  int    `json:"timeout_seconds,omitempty" jsonschema:"Optional tighter timeout in seconds for this query."`
}

// ExternalDatabaseQueryTool executes realtime structured queries against a database KB.
// Like the schema tool, it stays bound to the current agent scope so the LLM can
// only query KBs that were explicitly selected for the running agent session.
type ExternalDatabaseQueryTool struct {
	BaseTool
	structuredQueryService interfaces.StructuredQueryService
	searchTargets          types.SearchTargets
}

func NewExternalDatabaseQueryTool(
	structuredQueryService interfaces.StructuredQueryService,
	searchTargets types.SearchTargets,
) *ExternalDatabaseQueryTool {
	return &ExternalDatabaseQueryTool{
		BaseTool:               externalDatabaseQueryTool,
		structuredQueryService: structuredQueryService,
		searchTargets:          searchTargets,
	}
}

func (t *ExternalDatabaseQueryTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	var input ExternalDatabaseQueryInput
	if err := json.Unmarshal(args, &input); err != nil {
		return &types.ToolResult{Success: false, Error: "Failed to parse arguments: " + err.Error()}, nil
	}

	kbID := strings.TrimSpace(input.KnowledgeBaseID)
	if kbID == "" {
		return &types.ToolResult{Success: false, Error: "knowledge_base_id is required"}, nil
	}
	if !t.searchTargets.ContainsKB(kbID) {
		return &types.ToolResult{Success: false, Error: "knowledge_base_id is outside the current agent scope"}, nil
	}
	if strings.TrimSpace(input.SQL) == "" {
		return &types.ToolResult{Success: false, Error: "sql is required"}, nil
	}
	purpose := strings.TrimSpace(input.Purpose)
	if purpose == "" {
		return &types.ToolResult{Success: false, Error: "purpose is required"}, nil
	}

	tenantID, _ := types.TenantIDFromContext(ctx)
	userID, _ := types.UserIDFromContext(ctx)

	result, err := t.structuredQueryService.ExecuteQuery(ctx, types.ExecuteQueryRequest{
		TenantID:        tenantID,
		UserID:          userID,
		KnowledgeBaseID: kbID,
		SQL:             input.SQL,
		Purpose:         purpose,
		MaxRows:         input.MaxRows,
		TimeoutSeconds:  input.TimeoutSeconds,
	})
	if err != nil {
		return &types.ToolResult{Success: false, Error: err.Error()}, nil
	}

	columnNames := make([]string, 0, len(result.Columns))
	for _, column := range result.Columns {
		columnNames = append(columnNames, column.Name)
	}

	return &types.ToolResult{
		Success: true,
		Output:  formatExternalDatabaseQueryOutput(columnNames, result.Rows, result.RowCount, result.Truncated, result.DurationMS),
		Data: map[string]interface{}{
			"display_type":       "external_database_query",
			"knowledge_base_id":  kbID,
			"columns":            columnNames,
			"column_definitions": result.Columns,
			"rows":               result.Rows,
			"row_count":          result.RowCount,
			"truncated":          result.Truncated,
			"duration_ms":        result.DurationMS,
			"executed_sql":       result.ExecutedSQL,
		},
	}, nil
}

func formatExternalDatabaseQueryOutput(columns []string, rows []map[string]any, rowCount int, truncated bool, durationMS int64) string {
	var builder strings.Builder
	builder.WriteString("=== External Database Query Results ===\n\n")
	builder.WriteString(fmt.Sprintf("Returned %d rows in %d ms", rowCount, durationMS))
	if truncated {
		builder.WriteString(" (truncated)")
	}
	builder.WriteString("\n\n")
	if len(rows) == 0 {
		builder.WriteString("No matching records found.")
		return builder.String()
	}
	builder.WriteString("=== Data Details ===\n\n")
	for index, row := range rows {
		builder.WriteString(fmt.Sprintf("--- Record #%d ---\n", index+1))
		for _, column := range columns {
			value := row[column]
			switch typed := value.(type) {
			case nil:
				builder.WriteString(fmt.Sprintf("  %s: <NULL>\n", column))
			case string:
				builder.WriteString(fmt.Sprintf("  %s: %s\n", column, typed))
			default:
				raw, err := json.Marshal(typed)
				if err != nil {
					builder.WriteString(fmt.Sprintf("  %s: %v\n", column, typed))
				} else {
					builder.WriteString(fmt.Sprintf("  %s: %s\n", column, string(raw)))
				}
			}
		}
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}
