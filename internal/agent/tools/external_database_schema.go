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

When the database has many tables or the target business entity is still unclear, prefer calling external_database_search_tables first, then rerun external_database_schema with tables=[...] and mode=detail.

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
	Keyword         string   `json:"keyword,omitempty" jsonschema:"Optional keyword used to search table names, table comments, column names, and column comments."`
	TableNameLike   string   `json:"table_name_like,omitempty" jsonschema:"Optional case-insensitive substring filter applied to table names."`
	CommentLike     string   `json:"comment_like,omitempty" jsonschema:"Optional case-insensitive substring filter applied to table comments."`
	ListOnly        bool     `json:"list_only,omitempty" jsonschema:"When true, return the full matched table name list without expanding table structure details."`
}

type externalSchemaSearchFilter struct {
	keyword       string
	tableNameLike string
	commentLike   string
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

	searchFilter := normalizeExternalSchemaSearchFilter(input)
	viewResult := buildExternalSchemaViewResult(buildResult, searchFilter, input.ListOnly, types.PromptSchemaMode(input.Mode))

	allowedTables := make([]string, 0, len(buildResult.AllTables))
	for _, table := range buildResult.AllTables {
		allowedTables = append(allowedTables, table.Name)
	}
	matchedTables := make([]string, 0, len(viewResult.AllTables))
	for _, table := range viewResult.AllTables {
		matchedTables = append(matchedTables, table.Name)
	}
	foreignKeys := flattenForeignKeys(viewResult.DisplayTables)
	possibleJoinHints := buildPossibleJoinHints(viewResult.DisplayTables, buildResult.PossibleJoinHints, foreignKeys)
	sampleQueries := buildSampleQueries(viewResult.DisplayTables)
	tableData := make([]map[string]interface{}, 0)
	if !input.ListOnly {
		tableData, _ = toExternalSchemaTableData(viewResult.DisplayTables, viewResult.Mode)
	}

	return &types.ToolResult{
		Success: true,
		Output:  formatExternalDatabaseSchemaOutput(viewResult, allowedTables, matchedTables, foreignKeys, possibleJoinHints, sampleQueries, input.ListOnly, searchFilter),
		Data: map[string]interface{}{
			"display_type":               "external_database_schema",
			"knowledge_base_id":          kbID,
			"database_name":              viewResult.DatabaseName,
			"schema_name":                viewResult.SchemaName,
			"schema_hash":                viewResult.SchemaHash,
			"refreshed_at":               formatSchemaTimestamp(viewResult.RefreshedAt),
			"mode":                       string(viewResult.Mode),
			"table_count":                viewResult.TableCount,
			"column_count":               viewResult.ColumnCount,
			"display_table_count":        len(viewResult.DisplayTables),
			"additional_tables_omitted":  viewResult.AdditionalTablesOmitted,
			"additional_columns_omitted": viewResult.AdditionalColumnsOmitted,
			"scope_table_count":          len(buildResult.AllTables),
			"allowed_tables":             allowedTables,
			"matched_tables":             matchedTables,
			"matched_table_count":        len(matchedTables),
			"foreign_keys":               foreignKeys,
			"possible_join_hints":        possibleJoinHints,
			"join_hints":                 possibleJoinHints,
			"sample_queries":             sampleQueries,
			"keyword":                    searchFilter.keyword,
			"table_name_like":            searchFilter.tableNameLike,
			"comment_like":               searchFilter.commentLike,
			"list_only":                  input.ListOnly,
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

func normalizeExternalSchemaSearchFilter(input ExternalDatabaseSchemaInput) externalSchemaSearchFilter {
	return externalSchemaSearchFilter{
		keyword:       strings.ToLower(strings.TrimSpace(input.Keyword)),
		tableNameLike: strings.ToLower(strings.TrimSpace(input.TableNameLike)),
		commentLike:   strings.ToLower(strings.TrimSpace(input.CommentLike)),
	}
}

func (f externalSchemaSearchFilter) active() bool {
	return f.keyword != "" || f.tableNameLike != "" || f.commentLike != ""
}

func buildExternalSchemaViewResult(
	base *types.PromptSchemaBuildResult,
	filter externalSchemaSearchFilter,
	listOnly bool,
	requestedMode types.PromptSchemaMode,
) *types.PromptSchemaBuildResult {
	if base == nil {
		return nil
	}
	matchedTables := append([]types.TableSchema(nil), base.AllTables...)
	if filter.active() {
		matchedTables = filterExternalSchemaTables(matchedTables, filter)
	}
	mode := resolveExternalSchemaViewMode(requestedMode, matchedTables, listOnly)
	displayTables := append([]types.TableSchema(nil), matchedTables...)
	if listOnly {
		displayTables = nil
	} else if mode == types.PromptSchemaModeCatalog && len(displayTables) > schemaOutputTableLimit {
		displayTables = displayTables[:schemaOutputTableLimit]
	}
	columnCount := countExternalSchemaColumns(matchedTables)
	additionalColumns := 0
	if mode == types.PromptSchemaModeCatalog {
		displayedColumns := 0
		for _, table := range displayTables {
			displayedColumns += minInt(len(table.Columns), schemaCatalogColumnLimit)
		}
		additionalColumns = maxInt(columnCount-displayedColumns, 0)
	}
	result := &types.PromptSchemaBuildResult{
		Prompt:                   renderExternalSchemaViewPrompt(base, matchedTables, displayTables, mode, filter, listOnly),
		Mode:                     mode,
		DatabaseName:             base.DatabaseName,
		SchemaName:               base.SchemaName,
		SchemaHash:               base.SchemaHash,
		RefreshedAt:              base.RefreshedAt,
		AllTables:                matchedTables,
		DisplayTables:            displayTables,
		PossibleJoinHints:        append([]string(nil), base.PossibleJoinHints...),
		TableCount:               len(matchedTables),
		ColumnCount:              columnCount,
		AdditionalTablesOmitted:  maxInt(len(matchedTables)-len(displayTables), 0),
		AdditionalColumnsOmitted: additionalColumns,
	}
	return result
}

func resolveExternalSchemaViewMode(mode types.PromptSchemaMode, tables []types.TableSchema, listOnly bool) types.PromptSchemaMode {
	if listOnly {
		return types.PromptSchemaModeDetail
	}
	switch types.PromptSchemaMode(strings.ToLower(strings.TrimSpace(string(mode)))) {
	case types.PromptSchemaModeCatalog:
		return types.PromptSchemaModeCatalog
	case types.PromptSchemaModeDetail:
		return types.PromptSchemaModeDetail
	}
	if len(tables) <= 12 && countExternalSchemaColumns(tables) <= 160 {
		return types.PromptSchemaModeDetail
	}
	return types.PromptSchemaModeCatalog
}

func filterExternalSchemaTables(tables []types.TableSchema, filter externalSchemaSearchFilter) []types.TableSchema {
	if !filter.active() {
		return tables
	}
	matched := make([]types.TableSchema, 0, len(tables))
	for _, table := range tables {
		if externalSchemaTableMatches(table, filter) {
			matched = append(matched, table)
		}
	}
	return matched
}

func externalSchemaTableMatches(table types.TableSchema, filter externalSchemaSearchFilter) bool {
	name := strings.ToLower(strings.TrimSpace(table.Name))
	comment := strings.ToLower(strings.TrimSpace(table.Comment))
	if filter.tableNameLike != "" && !strings.Contains(name, filter.tableNameLike) {
		return false
	}
	if filter.commentLike != "" && !strings.Contains(comment, filter.commentLike) {
		return false
	}
	if filter.keyword == "" {
		return true
	}
	if strings.Contains(name, filter.keyword) || strings.Contains(comment, filter.keyword) {
		return true
	}
	for _, column := range table.Columns {
		columnName := strings.ToLower(strings.TrimSpace(column.Name))
		columnComment := strings.ToLower(strings.TrimSpace(column.Comment))
		if strings.Contains(columnName, filter.keyword) || strings.Contains(columnComment, filter.keyword) {
			return true
		}
	}
	return false
}

func countExternalSchemaColumns(tables []types.TableSchema) int {
	total := 0
	for _, table := range tables {
		total += len(table.Columns)
	}
	return total
}

func renderExternalSchemaViewPrompt(
	base *types.PromptSchemaBuildResult,
	matchedTables []types.TableSchema,
	displayTables []types.TableSchema,
	mode types.PromptSchemaMode,
	filter externalSchemaSearchFilter,
	listOnly bool,
) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Database: %s\n", base.DatabaseName))
	if strings.TrimSpace(base.SchemaName) != "" {
		builder.WriteString(fmt.Sprintf("Schema: %s\n", base.SchemaName))
	}
	if !base.RefreshedAt.IsZero() || strings.TrimSpace(base.SchemaHash) != "" {
		parts := make([]string, 0, 2)
		if !base.RefreshedAt.IsZero() {
			parts = append(parts, fmt.Sprintf("refreshed_at=%s", base.RefreshedAt.UTC().Format(time.RFC3339)))
		}
		if strings.TrimSpace(base.SchemaHash) != "" {
			parts = append(parts, fmt.Sprintf("schema_hash=%s", base.SchemaHash))
		}
		builder.WriteString("Schema snapshot: ")
		builder.WriteString(strings.Join(parts, ", "))
		builder.WriteString("\n")
	}
	if listOnly {
		builder.WriteString("Schema output mode: list_only\n")
	} else {
		builder.WriteString(fmt.Sprintf("Schema output mode: %s\n", mode))
	}
	builder.WriteString(fmt.Sprintf("Scope summary: %d matched tables, %d columns\n", len(matchedTables), countExternalSchemaColumns(matchedTables)))
	if listOnly {
		builder.WriteString(fmt.Sprintf("Current view table count: %d / %d (names only)\n", len(matchedTables), len(matchedTables)))
	} else {
		builder.WriteString(fmt.Sprintf("Current view table count: %d / %d\n", len(displayTables), len(matchedTables)))
	}
	if filter.active() {
		builder.WriteString("Applied filters:\n")
		if filter.keyword != "" {
			builder.WriteString(fmt.Sprintf("- keyword=%s\n", filter.keyword))
		}
		if filter.tableNameLike != "" {
			builder.WriteString(fmt.Sprintf("- table_name_like=%s\n", filter.tableNameLike))
		}
		if filter.commentLike != "" {
			builder.WriteString(fmt.Sprintf("- comment_like=%s\n", filter.commentLike))
		}
	}
	if listOnly {
		builder.WriteString("List-only view: returning the full matched table name list without expanding columns.\n")
		return strings.TrimSpace(builder.String())
	}
	if mode == types.PromptSchemaModeCatalog {
		builder.WriteString("Catalog view: representative columns are shown first. If the target table is unclear, call external_database_search_tables first; once candidate tables are known, rerun external_database_schema with tables=[...] and mode=detail.\n")
	}
	builder.WriteString(renderExternalSchemaQueryGuidance())
	if len(displayTables) == 0 {
		builder.WriteString("No tables matched the current filters.\n")
		return strings.TrimSpace(builder.String())
	}
	builder.WriteString("Tables:\n")
	for _, table := range displayTables {
		builder.WriteString(renderExternalSchemaTable(table, mode))
	}
	if len(matchedTables) > len(displayTables) {
		builder.WriteString(fmt.Sprintf("Additional matched tables omitted from this view: %d\n", len(matchedTables)-len(displayTables)))
		builder.WriteString("Next step hint: use external_database_search_tables to narrow candidate tables, then rerun external_database_schema with tables=[...] and mode=detail for full columns.\n")
	}
	return strings.TrimSpace(builder.String())
}

func renderExternalSchemaQueryGuidance() string {
	var builder strings.Builder
	builder.WriteString("Query planning rules:\n")
	builder.WriteString("- Add LIMIT to any query that can return multiple rows. This includes detail previews, JOIN inspections, DISTINCT value lists, GROUP BY/HAVING summaries, ORDER BY top-N checks, window-function queries, and multi-row CTE outputs.\n")
	builder.WriteString("- Only pure global aggregates that return one row may omit LIMIT, such as COUNT(*), SUM(amount), AVG(score), MIN(created_at), MAX(created_at), or DISTINCT COUNT(*), with no GROUP BY and no window clause.\n")
	builder.WriteString("- For exploratory inspection, start with LIMIT 10 or LIMIT 20 and tighten WHERE conditions before widening scope.\n")
	return builder.String()
}

func renderExternalSchemaTable(table types.TableSchema, mode types.PromptSchemaMode) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("- %s (%s)", table.Name, table.Type))
	if comment := truncateSchemaText(table.Comment, schemaOutputCommentLimit); comment != "" {
		builder.WriteString(": ")
		builder.WriteString(comment)
	}
	builder.WriteString("\n")
	if len(table.PrimaryKeys) > 0 {
		builder.WriteString(fmt.Sprintf("  Primary keys: %s\n", strings.Join(table.PrimaryKeys, ", ")))
	}
	if table.RowEstimate > 0 {
		builder.WriteString(fmt.Sprintf("  Row estimate: %d\n", table.RowEstimate))
	}
	if mode == types.PromptSchemaModeCatalog {
		builder.WriteString(fmt.Sprintf("  Representative columns: %s\n", summarizeSchemaColumns(table.Columns, schemaCatalogColumnLimit)))
		if len(table.ForeignKeys) > 0 {
			builder.WriteString(fmt.Sprintf("  Foreign keys: %s\n", summarizePromptSchemaTableForeignKeys(table.ForeignKeys, schemaCatalogIndexLimit)))
		}
		if omitted := maxInt(len(table.Columns)-schemaCatalogColumnLimit, 0); omitted > 0 {
			builder.WriteString(fmt.Sprintf("  Additional columns omitted: %d\n", omitted))
		}
		return builder.String()
	}
	builder.WriteString("  Columns:\n")
	for _, column := range table.Columns {
		nullability := "NOT NULL"
		if column.Nullable {
			nullability = "NULL"
		}
		builder.WriteString(fmt.Sprintf("  - %s %s %s", column.Name, column.DataType, nullability))
		if column.IsSensitive {
			builder.WriteString(" [sensitive]")
		}
		if comment := truncateSchemaText(column.Comment, schemaOutputCommentLimit); comment != "" {
			builder.WriteString(fmt.Sprintf(" -- %s", comment))
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func summarizePromptSchemaTableForeignKeys(foreignKeys []types.ForeignKeySchema, limit int) string {
	items := make([]string, 0, minInt(limit, len(foreignKeys)))
	for index, fk := range foreignKeys {
		if index >= limit {
			break
		}
		items = append(items, formatExternalForeignKeyTarget(fk))
	}
	return strings.Join(items, "; ")
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
	matchedTables []string,
	foreignKeys []string,
	possibleJoinHints []string,
	sampleQueries []string,
	listOnly bool,
	filter externalSchemaSearchFilter,
) string {
	var builder strings.Builder
	builder.WriteString("=== External Database Schema ===\n\n")
	if buildResult != nil {
		builder.WriteString(buildResult.Prompt)
	}
	sectionTitle := "\n\n=== Allowed Query Scope ===\n"
	sectionTables := limitStringSlice(allowedTables, schemaOutputTableLimit)
	showAllTables := listOnly
	if listOnly {
		sectionTables = allowedTables
	}
	if filter.active() {
		sectionTitle = "\n\n=== Matched Tables ===\n"
		sectionTables = matchedTables
		showAllTables = true
	}
	builder.WriteString(sectionTitle)
	for _, table := range sectionTables {
		builder.WriteString("- ")
		builder.WriteString(table)
		builder.WriteString("\n")
	}
	if !showAllTables && len(allowedTables) > len(sectionTables) {
		builder.WriteString(fmt.Sprintf("Additional tables omitted from scope list: %d\n", len(allowedTables)-len(sectionTables)))
	}
	if filter.active() {
		builder.WriteString(fmt.Sprintf("Matched tables: %d / scope tables: %d\n", len(matchedTables), len(allowedTables)))
	} else if listOnly {
		builder.WriteString(fmt.Sprintf("Full table list returned: %d\n", len(sectionTables)))
	}
	if listOnly {
		return strings.TrimSpace(builder.String())
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
