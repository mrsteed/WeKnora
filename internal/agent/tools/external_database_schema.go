package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

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
	Mode            string   `json:"mode,omitempty" jsonschema:"Optional output mode: auto, catalog, or detail."`
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
	buildResult, err := t.schemaRegistry.BuildPromptSchemaResult(
		ctx,
		kbID,
		selectedTables,
		types.PromptSchemaOptions{Mode: types.PromptSchemaMode(input.Mode)},
	)
	if err != nil {
		return &types.ToolResult{Success: false, Error: "Failed to build prompt schema: " + err.Error()}, nil
	}

	allowedTables := make([]string, 0, len(buildResult.AllTables))
	for _, table := range buildResult.AllTables {
		allowedTables = append(allowedTables, table.Name)
	}
	visibleAllowedTables := make([]string, 0, len(buildResult.DisplayTables))
	for _, table := range buildResult.DisplayTables {
		visibleAllowedTables = append(visibleAllowedTables, table.Name)
	}
	foreignKeys := flattenForeignKeys(buildResult.DisplayTables)
	possibleJoinHints := buildPossibleJoinHints(buildResult.DisplayTables, buildResult.PossibleJoinHints, foreignKeys)
	sampleQueries := buildSampleQueries(buildResult.DisplayTables)
	tableData, _ := toExternalSchemaTableData(buildResult.DisplayTables, buildResult.Mode)

	return &types.ToolResult{
		Success: true,
		Output:  formatExternalDatabaseSchemaOutput(buildResult, allowedTables, foreignKeys, possibleJoinHints, sampleQueries),
		Data: map[string]interface{}{
			"display_type":               "external_database_schema",
			"knowledge_base_id":          kbID,
			"database_name":              buildResult.DatabaseName,
			"schema_name":                buildResult.SchemaName,
			"schema_hash":                buildResult.SchemaHash,
			"refreshed_at":               formatSchemaTimestamp(buildResult.RefreshedAt),
			"mode":                       string(buildResult.Mode),
			"table_count":                buildResult.TableCount,
			"column_count":               buildResult.ColumnCount,
			"additional_tables_omitted":  buildResult.AdditionalTablesOmitted,
			"additional_columns_omitted": buildResult.AdditionalColumnsOmitted,
			"allowed_tables":             allowedTables,
			"foreign_keys":               foreignKeys,
			"possible_join_hints":        possibleJoinHints,
			"join_hints":                 possibleJoinHints,
			"sample_queries":             sampleQueries,
			"tables":                     tableData,
		},
	}, nil
}

const (
	schemaOutputTableLimit       = 20
	schemaOutputColumnLimit      = 8
	schemaOutputJoinHintLimit    = 8
	schemaOutputSampleQueryLimit = 4
	schemaOutputCommentLimit     = 120
	schemaDataIndexLimit         = 4
	schemaCatalogColumnLimit     = 6
	schemaCatalogIndexLimit      = 3
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

func toExternalSchemaTableData(tables []types.TableSchema, mode types.PromptSchemaMode) ([]map[string]interface{}, int) {
	tableLimit := minInt(len(tables), schemaOutputTableLimit)
	result := make([]map[string]interface{}, 0, tableLimit)
	totalColumnOmitted := 0
	for tableIndex, table := range tables {
		if tableIndex >= schemaOutputTableLimit {
			break
		}
		columnLimit := len(table.Columns)
		indexLimit := len(table.Indexes)
		if mode == types.PromptSchemaModeCatalog {
			columnLimit = minInt(len(table.Columns), schemaCatalogColumnLimit)
			indexLimit = minInt(len(table.Indexes), schemaCatalogIndexLimit)
		}
		columns := make([]map[string]interface{}, 0, columnLimit)
		sensitiveCount := 0
		for columnIndex, column := range table.Columns {
			if column.IsSensitive {
				sensitiveCount++
			}
			if columnIndex >= columnLimit {
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
		indexes := make([]map[string]interface{}, 0, indexLimit)
		for indexIndex, index := range table.Indexes {
			if indexIndex >= indexLimit {
				break
			}
			indexes = append(indexes, map[string]interface{}{
				"name":       index.Name,
				"unique":     index.Unique,
				"columns":    limitStringSlice(index.Columns, schemaCatalogColumnLimit),
				"index_type": index.IndexType,
			})
		}
		additionalColumnsOmitted := maxInt(len(table.Columns)-columnLimit, 0)
		totalColumnOmitted += additionalColumnsOmitted
		result = append(result, map[string]interface{}{
			"name":                       table.Name,
			"type":                       table.Type,
			"comment":                    truncateSchemaText(table.Comment, schemaOutputCommentLimit),
			"primary_keys":               limitStringSlice(table.PrimaryKeys, schemaOutputColumnLimit),
			"foreign_keys":               toExternalForeignKeyData(table.ForeignKeys),
			"row_estimate":               table.RowEstimate,
			"index_count":                len(table.Indexes),
			"indexes":                    indexes,
			"column_count":               len(table.Columns),
			"columns":                    columns,
			"additional_columns_omitted": additionalColumnsOmitted,
			"sensitive_column_count":     sensitiveCount,
		})
	}
	return result, totalColumnOmitted
}

func toExternalForeignKeyData(foreignKeys []types.ForeignKeySchema) []map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(foreignKeys))
	for _, fk := range foreignKeys {
		items = append(items, map[string]interface{}{
			"name":               fk.Name,
			"columns":            append([]string(nil), fk.Columns...),
			"referenced_table":   fk.ReferencedTable,
			"referenced_columns": append([]string(nil), fk.ReferencedColumns...),
		})
	}
	return items
}

func flattenForeignKeys(tables []types.TableSchema) []string {
	items := make([]string, 0)
	for _, table := range tables {
		for _, fk := range table.ForeignKeys {
			items = append(items, formatExternalForeignKeyHint(table.Name, fk))
		}
	}
	sort.Strings(items)
	return items
}

func formatExternalForeignKeyHint(tableName string, fk types.ForeignKeySchema) string {
	return tableName + "." + formatExternalForeignKeyTarget(fk)
}

func formatExternalForeignKeyTarget(fk types.ForeignKeySchema) string {
	sourceColumns := strings.Join(fk.Columns, ", ")
	targetColumns := strings.Join(fk.ReferencedColumns, ", ")
	if len(fk.Columns) == 1 && len(fk.ReferencedColumns) == 1 {
		return fmt.Sprintf("%s -> %s.%s", sourceColumns, fk.ReferencedTable, targetColumns)
	}
	return fmt.Sprintf("(%s) -> %s(%s)", sourceColumns, fk.ReferencedTable, targetColumns)
}

func buildPossibleJoinHints(tables []types.TableSchema, configuredHints []string, foreignKeys []string) []string {
	seenForeignKeys := make(map[string]struct{}, len(foreignKeys))
	for _, item := range foreignKeys {
		seenForeignKeys[item] = struct{}{}
	}
	seen := make(map[string]struct{})
	items := make([]string, 0, len(configuredHints))
	for _, hint := range configuredHints {
		trimmed := strings.TrimSpace(hint)
		if trimmed == "" {
			continue
		}
		if _, ok := seenForeignKeys[trimmed]; ok {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		items = append(items, trimmed)
	}
	for _, hint := range inferJoinHints(tables) {
		if _, ok := seenForeignKeys[hint]; ok {
			continue
		}
		if _, ok := seen[hint]; ok {
			continue
		}
		seen[hint] = struct{}{}
		items = append(items, hint)
	}
	sort.Strings(items)
	return items
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

func formatExternalDatabaseSchemaOutput(
	buildResult *types.PromptSchemaBuildResult,
	allowedTables []string,
	foreignKeys []string,
	possibleJoinHints []string,
	sampleQueries []string,
) string {
	var builder strings.Builder
	builder.WriteString("=== External Database Schema ===\n\n")
	if buildResult != nil {
		builder.WriteString(buildResult.Prompt)
	}
	builder.WriteString("\n\n=== Allowed Query Scope ===\n")
	visibleTables := limitStringSlice(allowedTables, schemaOutputTableLimit)
	for _, table := range visibleTables {
		builder.WriteString("- ")
		builder.WriteString(table)
		builder.WriteString("\n")
	}
	if len(allowedTables) > len(visibleTables) {
		builder.WriteString(fmt.Sprintf("Additional tables omitted from scope list: %d\n", len(allowedTables)-len(visibleTables)))
	}
	if len(foreignKeys) > 0 {
		builder.WriteString("\n=== Foreign Keys ===\n")
		for _, hint := range limitStringSlice(foreignKeys, schemaOutputJoinHintLimit) {
			builder.WriteString("- ")
			builder.WriteString(hint)
			builder.WriteString("\n")
		}
	}
	if len(possibleJoinHints) > 0 {
		builder.WriteString("\n=== Possible Join Hints ===\n")
		builder.WriteString("Treat these as candidate relationships that still need confirmation from business semantics.\n")
		for _, hint := range limitStringSlice(possibleJoinHints, schemaOutputJoinHintLimit) {
			builder.WriteString("- ")
			builder.WriteString(hint)
			builder.WriteString("\n")
		}
	}
	if len(sampleQueries) > 0 {
		builder.WriteString("\n=== Sample Queries ===\n")
		for _, query := range limitStringSlice(sampleQueries, schemaOutputSampleQueryLimit) {
			builder.WriteString("- ")
			builder.WriteString(query)
			builder.WriteString("\n")
		}
	}
	return strings.TrimSpace(builder.String())
}

func formatSchemaTimestamp(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
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
