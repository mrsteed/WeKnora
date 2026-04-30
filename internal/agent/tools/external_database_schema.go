package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/Tencent/WeKnora/internal/utils"
)

var externalDatabaseSchemaTool = BaseTool{
	name: ToolExternalDatabaseSchema,
	description: `Inspect the schema of a database knowledge base before generating SQL.

Use this tool first when you need to understand which tables and columns are available in an external business database.

Rules:
- Only query database knowledge bases already in the current agent scope.
- Prefer calling this tool before external_database_query.
- Use only the tables and columns returned by this tool when writing SQL.
- Pay attention to [sensitive] field markers and allowed query scope.
`,
	schema: utils.GenerateSchema[ExternalDatabaseSchemaInput](),
}

type ExternalDatabaseSchemaInput struct {
	KnowledgeBaseID string   `json:"knowledge_base_id" jsonschema:"Database knowledge base ID to inspect."`
	Tables          []string `json:"tables,omitempty" jsonschema:"Optional table names to narrow the schema output."`
}

// ExternalDatabaseSchemaTool exposes prompt-friendly schema metadata for database KBs.
// The tool also enforces that the requested KB must already be in the current
// agent scope, so the LLM cannot probe arbitrary KB IDs outside the configured session.
type ExternalDatabaseSchemaTool struct {
	BaseTool
	schemaRegistry interfaces.SchemaRegistryService
	searchTargets  types.SearchTargets
}

func NewExternalDatabaseSchemaTool(
	schemaRegistry interfaces.SchemaRegistryService,
	searchTargets types.SearchTargets,
) *ExternalDatabaseSchemaTool {
	return &ExternalDatabaseSchemaTool{
		BaseTool:       externalDatabaseSchemaTool,
		schemaRegistry: schemaRegistry,
		searchTargets:  searchTargets,
	}
}

func (t *ExternalDatabaseSchemaTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	var input ExternalDatabaseSchemaInput
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

	selectedTables := normalizeSelectedTables(input.Tables)
	promptSchema, err := t.schemaRegistry.BuildPromptSchema(ctx, kbID, selectedTables)
	if err != nil {
		return &types.ToolResult{Success: false, Error: "Failed to build prompt schema: " + err.Error()}, nil
	}

	schema, err := t.schemaRegistry.GetDatabaseSchema(ctx, kbID)
	if err != nil {
		return &types.ToolResult{Success: false, Error: "Failed to load database schema: " + err.Error()}, nil
	}

	tables := filterSchemaTables(schema.Tables, selectedTables)
	if len(tables) == 0 {
		return &types.ToolResult{Success: false, Error: "No tables matched the requested scope"}, nil
	}

	allowedTables := make([]string, 0, len(tables))
	for _, table := range tables {
		allowedTables = append(allowedTables, table.Name)
	}
	joinHints := inferJoinHints(tables)
	sampleQueries := buildSampleQueries(tables)

	return &types.ToolResult{
		Success: true,
		Output:  formatExternalDatabaseSchemaOutput(promptSchema, allowedTables, joinHints, sampleQueries),
		Data: map[string]interface{}{
			"display_type":      "external_database_schema",
			"knowledge_base_id": kbID,
			"database_name":     schema.DatabaseName,
			"schema_name":       schema.SchemaName,
			"allowed_tables":    allowedTables,
			"join_hints":        joinHints,
			"sample_queries":    sampleQueries,
			"tables":            toExternalSchemaTableData(tables),
		},
	}, nil
}

const (
	schemaOutputTableLimit       = 8
	schemaOutputColumnLimit      = 8
	schemaOutputJoinHintLimit    = 8
	schemaOutputSampleQueryLimit = 4
	schemaOutputCommentLimit     = 120
	schemaDataIndexLimit         = 4
)

func normalizeSelectedTables(tables []string) []string {
	if len(tables) == 0 {
		return nil
	}
	items := make([]string, 0, len(tables))
	seen := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		normalized := strings.ToLower(strings.TrimSpace(table))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		items = append(items, normalized)
	}
	return items
}

func filterSchemaTables(tables []types.TableSchema, selected []string) []types.TableSchema {
	if len(selected) == 0 {
		return append([]types.TableSchema(nil), tables...)
	}
	selectedSet := make(map[string]struct{}, len(selected))
	for _, table := range selected {
		selectedSet[table] = struct{}{}
	}
	filtered := make([]types.TableSchema, 0, len(tables))
	for _, table := range tables {
		if _, ok := selectedSet[strings.ToLower(table.Name)]; ok {
			filtered = append(filtered, table)
		}
	}
	return filtered
}

func toExternalSchemaTableData(tables []types.TableSchema) []map[string]interface{} {
	tableLimit := minInt(len(tables), schemaOutputTableLimit)
	result := make([]map[string]interface{}, 0, tableLimit)
	for tableIndex, table := range tables {
		if tableIndex >= schemaOutputTableLimit {
			break
		}
		columns := make([]map[string]interface{}, 0, minInt(len(table.Columns), schemaOutputColumnLimit))
		sensitiveCount := 0
		for columnIndex, column := range table.Columns {
			if column.IsSensitive {
				sensitiveCount++
			}
			if columnIndex >= schemaOutputColumnLimit {
				continue
			}
			columns = append(columns, map[string]interface{}{
				"name":         column.Name,
				"data_type":    column.DataType,
				"nullable":     column.Nullable,
				"comment":      truncateSchemaText(column.Comment, schemaOutputCommentLimit),
				"is_sensitive": column.IsSensitive,
			})
		}
		indexes := make([]map[string]interface{}, 0, minInt(len(table.Indexes), schemaDataIndexLimit))
		for indexIndex, index := range table.Indexes {
			if indexIndex >= schemaDataIndexLimit {
				break
			}
			indexes = append(indexes, map[string]interface{}{
				"name":       index.Name,
				"unique":     index.Unique,
				"columns":    limitStringSlice(index.Columns, schemaOutputColumnLimit),
				"index_type": index.IndexType,
			})
		}
		result = append(result, map[string]interface{}{
			"name":                       table.Name,
			"type":                       table.Type,
			"comment":                    truncateSchemaText(table.Comment, schemaOutputCommentLimit),
			"primary_keys":               limitStringSlice(table.PrimaryKeys, schemaOutputColumnLimit),
			"index_count":                len(table.Indexes),
			"indexes":                    indexes,
			"column_count":               len(table.Columns),
			"columns":                    columns,
			"additional_columns_omitted": maxInt(len(table.Columns)-schemaOutputColumnLimit, 0),
			"sensitive_column_count":     sensitiveCount,
		})
	}
	return result
}

func inferJoinHints(tables []types.TableSchema) []string {
	hints := make([]string, 0)
	seen := make(map[string]struct{})
	pkByTable := make(map[string][]string, len(tables))
	for _, table := range tables {
		pkByTable[strings.ToLower(table.Name)] = normalizeSelectedTables(table.PrimaryKeys)
	}
	for _, source := range tables {
		for _, column := range source.Columns {
			columnName := strings.ToLower(strings.TrimSpace(column.Name))
			if columnName == "" || column.IsSensitive {
				continue
			}
			for _, target := range tables {
				if strings.EqualFold(source.Name, target.Name) {
					continue
				}
				targetPKs := pkByTable[strings.ToLower(target.Name)]
				if len(targetPKs) == 0 && hasColumn(target.Columns, "id") {
					targetPKs = []string{"id"}
				}
				if len(targetPKs) == 0 {
					continue
				}
				singularTarget := singularTableName(target.Name)
				candidate := singularTarget + "_id"
				for _, targetPK := range targetPKs {
					if columnName == targetPK && columnName != "id" {
						hint := fmt.Sprintf("%s.%s = %s.%s", source.Name, column.Name, target.Name, targetPK)
						if _, ok := seen[hint]; !ok {
							seen[hint] = struct{}{}
							hints = append(hints, hint)
						}
					}
					if targetPK == "id" && (columnName == candidate || columnName == strings.ToLower(target.Name)+"_id") {
						hint := fmt.Sprintf("%s.%s = %s.id", source.Name, column.Name, target.Name)
						if _, ok := seen[hint]; !ok {
							seen[hint] = struct{}{}
							hints = append(hints, hint)
						}
					}
				}
			}
		}
	}
	sort.Strings(hints)
	return hints
}

func hasColumn(columns []types.ColumnSchema, wanted string) bool {
	for _, column := range columns {
		if strings.EqualFold(column.Name, wanted) {
			return true
		}
	}
	return false
}

func singularTableName(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasSuffix(lower, "ies") && len(lower) > 3:
		return lower[:len(lower)-3] + "y"
	case strings.HasSuffix(lower, "ses") && len(lower) > 3:
		return lower[:len(lower)-2]
	case strings.HasSuffix(lower, "s") && len(lower) > 1:
		return lower[:len(lower)-1]
	default:
		return lower
	}
}

func buildSampleQueries(tables []types.TableSchema) []string {
	queries := make([]string, 0, len(tables))
	for _, table := range tables {
		columns := make([]string, 0, 4)
		for _, column := range table.Columns {
			if column.IsSensitive {
				continue
			}
			columns = append(columns, column.Name)
			if len(columns) == 4 {
				break
			}
		}
		if len(columns) == 0 {
			for _, column := range table.Columns {
				columns = append(columns, column.Name)
				if len(columns) == 4 {
					break
				}
			}
		}
		if len(columns) == 0 {
			continue
		}
		queries = append(queries, fmt.Sprintf("SELECT %s FROM %s LIMIT 10", strings.Join(columns, ", "), table.Name))
	}
	return queries
}

func formatExternalDatabaseSchemaOutput(promptSchema string, allowedTables []string, joinHints []string, sampleQueries []string) string {
	var builder strings.Builder
	builder.WriteString("=== External Database Schema ===\n\n")
	builder.WriteString(promptSchema)
	builder.WriteString("\n\n=== Allowed Query Scope ===\n")
	for _, table := range allowedTables {
		builder.WriteString("- ")
		builder.WriteString(table)
		builder.WriteString("\n")
	}
	if len(joinHints) > 0 {
		builder.WriteString("\n=== Join Hints ===\n")
		for _, hint := range joinHints {
			builder.WriteString("- ")
			builder.WriteString(hint)
			builder.WriteString("\n")
		}
	}
	if len(sampleQueries) > 0 {
		builder.WriteString("\n=== Sample Queries ===\n")
		for _, query := range sampleQueries {
			builder.WriteString("- ")
			builder.WriteString(query)
			builder.WriteString("\n")
		}
	}
	return strings.TrimSpace(builder.String())
}

func summarizeSchemaColumns(columns []types.ColumnSchema, limit int) string {
	if len(columns) == 0 {
		return "<no columns>"
	}
	items := make([]string, 0, minInt(limit, len(columns)))
	for index, column := range columns {
		if index >= limit {
			break
		}
		nullability := "NOT NULL"
		if column.Nullable {
			nullability = "NULL"
		}
		segment := fmt.Sprintf("%s %s %s", column.Name, column.DataType, nullability)
		if column.IsSensitive {
			segment += " [sensitive]"
		}
		items = append(items, segment)
	}
	return strings.Join(items, "; ")
}

func truncateSchemaText(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || len([]rune(text)) <= limit {
		return text
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
