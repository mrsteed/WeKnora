package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

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
- Add LIMIT to any query that can return multiple rows. This includes detail previews, JOIN inspections, DISTINCT value lists, GROUP BY/HAVING summaries, ORDER BY top-N checks, window-function queries, and multi-row CTE outputs.
- Only pure global aggregates that return a single row may omit LIMIT, such as COUNT(*), SUM(amount), AVG(score), MIN(created_at), MAX(created_at), or DISTINCT COUNT(*), with no GROUP BY and no window clause.
- Sensitive fields and disallowed columns are rejected.
- DDL and DML statements are forbidden.
`,
	schema: utils.GenerateSchema[ExternalDatabaseQueryInput](),
}

type ExternalDatabaseQueryInput struct {
	KnowledgeBaseID string `json:"knowledge_base_id" jsonschema:"Database knowledge base ID to query."`
	SQL             string `json:"sql" jsonschema:"Read-only SQL to execute against the external database. Add LIMIT to any multi-row query, including detail previews, joins, DISTINCT lists, GROUP BY/HAVING summaries, ORDER BY top-N checks, window functions, and multi-row CTE outputs. Only pure single-row global aggregates may omit LIMIT."`
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

type budgetedQueryRows struct {
	rows                   []map[string]any
	rowCellTruncatedCounts []int
	cellTruncatedCount     int
}

type budgetedQueryOutput struct {
	output             string
	outputTruncated    bool
	outputRowCount     int
	cellTruncatedCount int
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

	frontendRows := buildBudgetedQueryRows(columnNames, result.Rows, types.QueryFrontendRowLimit, types.QueryFrontendCellCharLimit)
	outputBudget := formatExternalDatabaseQueryOutput(columnNames, result.Rows, result.RowCount, result.Truncated, result.DurationMS, result.ExecutedSQL)
	outputTruncated := outputBudget.outputTruncated || len(result.Rows) > len(frontendRows.rows) || frontendRows.cellTruncatedCount > 0

	return &types.ToolResult{
		Success: true,
		Output:  outputBudget.output,
		Data: map[string]interface{}{
			"display_type":         "external_database_query",
			"knowledge_base_id":    kbID,
			"columns":              columnNames,
			"column_definitions":   result.Columns,
			"rows":                 frontendRows.rows,
			"row_count":            result.RowCount,
			"display_row_count":    len(frontendRows.rows),
			"truncated":            result.Truncated,
			"output_truncated":     outputTruncated,
			"cell_truncated_count": frontendRows.cellTruncatedCount,
			"duration_ms":          result.DurationMS,
			"executed_sql":         result.ExecutedSQL,
		},
	}, nil
}

func formatExternalDatabaseQueryOutput(columns []string, rows []map[string]any, rowCount int, truncated bool, durationMS int64, executedSQL string) budgetedQueryOutput {
	llmColumns := limitStringSlice(columns, types.QueryLLMColumnDetailLimit)
	llmRows := buildBudgetedQueryRows(llmColumns, rows, types.QueryLLMRowDetailLimit, types.QueryLLMCellCharLimit)
	displayedRows := len(llmRows.rows)
	displaySQL, sqlTruncated := truncateQueryCellTextWithFlag(executedSQL, 2000)
	outputTruncated := len(columns) > len(llmColumns) || len(rows) > displayedRows || llmRows.cellTruncatedCount > 0 || sqlTruncated

	for {
		cellTruncatedCount := sumIntSlice(limitIntSlice(llmRows.rowCellTruncatedCounts, displayedRows))
		message := renderBudgetedQueryOutput(columns, llmColumns, llmRows.rows[:displayedRows], rowCount, truncated, durationMS, displaySQL, outputTruncated, cellTruncatedCount)
		if utf8.RuneCountInString(message) <= types.QueryLLMOutputCharBudget || displayedRows == 0 {
			return budgetedQueryOutput{
				output:             message,
				outputTruncated:    outputTruncated,
				outputRowCount:     displayedRows,
				cellTruncatedCount: cellTruncatedCount,
			}
		}
		displayedRows--
		outputTruncated = true
	}
}

func renderBudgetedQueryOutput(columns []string, displayedColumns []string, rows []map[string]any, rowCount int, truncated bool, durationMS int64, executedSQL string, outputTruncated bool, cellTruncatedCount int) string {
	var builder strings.Builder
	builder.WriteString("=== External Database Query Results ===\n\n")
	builder.WriteString(fmt.Sprintf("Returned %d rows in %d ms", rowCount, durationMS))
	if truncated {
		builder.WriteString(" (database result truncated)")
	}
	builder.WriteString("\n\n")
	if len(columns) > 0 {
		builder.WriteString(fmt.Sprintf("Columns shown to model: %d / %d\n", len(displayedColumns), len(columns)))
	}
	builder.WriteString(fmt.Sprintf("Rows available to model: %d / %d\n", len(rows), rowCount))
	if outputTruncated {
		builder.WriteString("Output budget truncated: true\n")
	}
	if cellTruncatedCount > 0 {
		builder.WriteString(fmt.Sprintf("Long cells truncated for model: %d\n", cellTruncatedCount))
	}
	if executedSQL != "" {
		builder.WriteString("\n=== Executed SQL ===\n")
		builder.WriteString(executedSQL)
		builder.WriteString("\n")
	}
	if len(rows) == 0 {
		builder.WriteString("\n")
		builder.WriteString("No matching records found.")
		return strings.TrimSpace(builder.String())
	}
	builder.WriteString("\n=== Data Details ===\n\n")
	for index, row := range rows {
		builder.WriteString(fmt.Sprintf("--- Record #%d ---\n", index+1))
		for _, column := range displayedColumns {
			builder.WriteString(fmt.Sprintf("  %s: %s\n", column, formatQueryCellValue(row[column])))
		}
		builder.WriteString("\n")
	}
	if len(columns) > len(displayedColumns) {
		builder.WriteString(fmt.Sprintf("Additional columns omitted from model view: %d\n", len(columns)-len(displayedColumns)))
	}
	if rowCount > len(rows) {
		builder.WriteString(fmt.Sprintf("Additional rows omitted from model view: %d\n", rowCount-len(rows)))
	}
	return strings.TrimSpace(builder.String())
}

func buildBudgetedQueryRows(columns []string, rows []map[string]any, rowLimit int, cellCharLimit int) budgetedQueryRows {
	displayLimit := minInt(len(rows), rowLimit)
	result := budgetedQueryRows{
		rows:                   make([]map[string]any, 0, displayLimit),
		rowCellTruncatedCounts: make([]int, 0, displayLimit),
	}
	for rowIndex, row := range rows {
		if rowIndex >= rowLimit {
			break
		}
		budgetedRow := make(map[string]any, len(columns))
		rowTruncatedCount := 0
		for _, column := range columns {
			value, truncated := budgetQueryCellValue(row[column], cellCharLimit)
			budgetedRow[column] = value
			if truncated {
				rowTruncatedCount++
			}
		}
		result.rows = append(result.rows, budgetedRow)
		result.rowCellTruncatedCounts = append(result.rowCellTruncatedCounts, rowTruncatedCount)
		result.cellTruncatedCount += rowTruncatedCount
	}
	return result
}

func formatQueryCellValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return "<NULL>"
	case string:
		return typed
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprintf("%v", typed)
		}
		return string(raw)
	}
}

func budgetQueryCellValue(value any, maxChars int) (any, bool) {
	if value == nil {
		return nil, false
	}
	switch typed := value.(type) {
	case string:
		truncated := truncateQueryCellText(typed, maxChars)
		return truncated, truncated != typed
	case []byte:
		text := string(typed)
		truncated := truncateQueryCellText(text, maxChars)
		return truncated, truncated != text
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return typed, false
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			text := fmt.Sprintf("%v", typed)
			truncated := truncateQueryCellText(text, maxChars)
			return truncated, truncated != text
		}
		text := string(raw)
		truncated := truncateQueryCellText(text, maxChars)
		return truncated, truncated != text
	}
}

func truncateQueryCellText(text string, maxChars int) string {
	truncated, _ := truncateQueryCellTextWithFlag(text, maxChars)
	return truncated
}

func truncateQueryCellTextWithFlag(text string, maxChars int) (string, bool) {
	text = strings.TrimSpace(text)
	if maxChars <= 0 || utf8.RuneCountInString(text) <= maxChars {
		return text, false
	}
	marker := "... [truncated]"
	markerLen := utf8.RuneCountInString(marker)
	if maxChars <= markerLen {
		return string([]rune(text)[:maxChars]), true
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:maxChars-markerLen])) + marker, true
}

func limitIntSlice(items []int, limit int) []int {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func sumIntSlice(items []int) int {
	total := 0
	for _, item := range items {
		total += item
	}
	return total
}

func limitStringSlice(items []string, limit int) []string {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}
