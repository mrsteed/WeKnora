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

var externalDatabaseSearchTablesTool = BaseTool{
	name: ToolExternalDatabaseSearchTables,
	description: `Search external database tables before inspecting full schema details.

Use this tool when you need to find which tables may contain a business concept such as plans, orders, incidents, workflows, or logs.

Use this tool before external_database_schema detail mode when the user asks which table stores a business object, when many tables are omitted from catalog view, or when you need to narrow candidate tables before writing SQL.

Rules:
- Only query database knowledge bases already in the current agent scope.
- Provide at least one search condition.
- Use the returned candidate tables to narrow the next external_database_schema call with tables=[...] and mode=detail.
`,
	schema: utils.GenerateSchema[ExternalDatabaseSearchTablesInput](),
}

type ExternalDatabaseSearchTablesInput struct {
	KnowledgeBaseID   string `json:"knowledge_base_id" jsonschema:"Database knowledge base ID to inspect."`
	Keyword           string `json:"keyword,omitempty" jsonschema:"Optional keyword matched against table names, table comments, column names, and column comments."`
	TableNameLike     string `json:"table_name_like,omitempty" jsonschema:"Optional case-insensitive substring filter applied to table names."`
	CommentLike       string `json:"comment_like,omitempty" jsonschema:"Optional case-insensitive substring filter applied to table comments."`
	ColumnNameLike    string `json:"column_name_like,omitempty" jsonschema:"Optional case-insensitive substring filter applied to column names."`
	ColumnCommentLike string `json:"column_comment_like,omitempty" jsonschema:"Optional case-insensitive substring filter applied to column comments."`
	Limit             int    `json:"limit,omitempty" jsonschema:"Optional max number of matched tables to return. Defaults to 20, max 50."`
}

type ExternalDatabaseSearchTablesTool struct {
	BaseTool
	schemaRegistry interfaces.SchemaRegistryService
	searchTargets  types.SearchTargets
}

type externalDatabaseSearchTablesFilter struct {
	keyword           string
	tableNameLike     string
	commentLike       string
	columnNameLike    string
	columnCommentLike string
}

type externalDatabaseTableSearchHit struct {
	TableName          string
	TableComment       string
	MatchedColumns     []string
	MatchReasons       []string
	LikelyRole         string
	Score              int
	ColumnCount        int
	MatchedColumnCount int
}

type externalDatabaseSearchTablesResult struct {
	ReturnedHits      []externalDatabaseTableSearchHit
	AllMatchedTables  []string
	TotalMatchedCount int
}

func NewExternalDatabaseSearchTablesTool(
	schemaRegistry interfaces.SchemaRegistryService,
	searchTargets types.SearchTargets,
) *ExternalDatabaseSearchTablesTool {
	return &ExternalDatabaseSearchTablesTool{
		BaseTool:       externalDatabaseSearchTablesTool,
		schemaRegistry: schemaRegistry,
		searchTargets:  searchTargets,
	}
}

func (t *ExternalDatabaseSearchTablesTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	var input ExternalDatabaseSearchTablesInput
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
	filter := normalizeExternalDatabaseSearchTablesFilter(input)
	if !filter.active() {
		return &types.ToolResult{Success: false, Error: "at least one search filter is required"}, nil
	}
	limit := normalizeExternalDatabaseSearchTablesLimit(input.Limit)
	schema, err := t.schemaRegistry.GetDatabaseSchema(ctx, kbID)
	if err != nil {
		return &types.ToolResult{Success: false, Error: "Failed to load database schema: " + err.Error()}, nil
	}
	searchResult := searchExternalDatabaseTables(schema.Tables, filter, limit)
	resultItems := make([]map[string]interface{}, 0, len(searchResult.ReturnedHits))
	for _, hit := range searchResult.ReturnedHits {
		resultItems = append(resultItems, map[string]interface{}{
			"table_name":           hit.TableName,
			"table_comment":        hit.TableComment,
			"matched_columns":      hit.MatchedColumns,
			"match_reasons":        hit.MatchReasons,
			"likely_role":          hit.LikelyRole,
			"score":                hit.Score,
			"column_count":         hit.ColumnCount,
			"matched_column_count": hit.MatchedColumnCount,
		})
	}
	return &types.ToolResult{
		Success: true,
		Output:  formatExternalDatabaseSearchTablesOutput(schema, filter, searchResult.ReturnedHits, searchResult.TotalMatchedCount, limit),
		Data: map[string]interface{}{
			"display_type":               "external_database_search_tables",
			"knowledge_base_id":          kbID,
			"database_name":              schema.DatabaseName,
			"schema_name":                schema.SchemaName,
			"schema_hash":                schema.SchemaHash,
			"refreshed_at":               formatSchemaTimestamp(schema.RefreshedAt),
			"scope_table_count":          len(schema.Tables),
			"matched_table_count":        searchResult.TotalMatchedCount,
			"returned_hit_count":         len(searchResult.ReturnedHits),
			"additional_matches_omitted": maxInt(searchResult.TotalMatchedCount-len(searchResult.ReturnedHits), 0),
			"keyword":                    filter.keyword,
			"table_name_like":            filter.tableNameLike,
			"comment_like":               filter.commentLike,
			"column_name_like":           filter.columnNameLike,
			"column_comment_like":        filter.columnCommentLike,
			"limit":                      limit,
			"matched_tables":             searchResult.AllMatchedTables,
			"results":                    resultItems,
		},
	}, nil
}

func normalizeExternalDatabaseSearchTablesFilter(input ExternalDatabaseSearchTablesInput) externalDatabaseSearchTablesFilter {
	return externalDatabaseSearchTablesFilter{
		keyword:           strings.ToLower(strings.TrimSpace(input.Keyword)),
		tableNameLike:     strings.ToLower(strings.TrimSpace(input.TableNameLike)),
		commentLike:       strings.ToLower(strings.TrimSpace(input.CommentLike)),
		columnNameLike:    strings.ToLower(strings.TrimSpace(input.ColumnNameLike)),
		columnCommentLike: strings.ToLower(strings.TrimSpace(input.ColumnCommentLike)),
	}
}

func (f externalDatabaseSearchTablesFilter) active() bool {
	return f.keyword != "" || f.tableNameLike != "" || f.commentLike != "" || f.columnNameLike != "" || f.columnCommentLike != ""
}

func normalizeExternalDatabaseSearchTablesLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func searchExternalDatabaseTables(
	tables []types.TableSchema,
	filter externalDatabaseSearchTablesFilter,
	limit int,
) externalDatabaseSearchTablesResult {
	hits := make([]externalDatabaseTableSearchHit, 0)
	for _, table := range tables {
		hit, ok := matchExternalDatabaseTable(table, filter)
		if ok {
			hits = append(hits, hit)
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].TableName < hits[j].TableName
		}
		return hits[i].Score > hits[j].Score
	})
	allMatchedTables := make([]string, 0, len(hits))
	for _, hit := range hits {
		allMatchedTables = append(allMatchedTables, hit.TableName)
	}
	returnedHits := append([]externalDatabaseTableSearchHit(nil), hits...)
	if len(returnedHits) > limit {
		returnedHits = returnedHits[:limit]
	}
	return externalDatabaseSearchTablesResult{
		ReturnedHits:      returnedHits,
		AllMatchedTables:  allMatchedTables,
		TotalMatchedCount: len(hits),
	}
}

func matchExternalDatabaseTable(table types.TableSchema, filter externalDatabaseSearchTablesFilter) (externalDatabaseTableSearchHit, bool) {
	tableName := strings.ToLower(strings.TrimSpace(table.Name))
	tableComment := strings.ToLower(strings.TrimSpace(table.Comment))
	reasons := make([]string, 0, 6)
	matchedColumns := make([]string, 0, 6)
	score := 0
	keywordMatched := filter.keyword == ""
	columnNameMatched := filter.columnNameLike == ""
	columnCommentMatched := filter.columnCommentLike == ""

	if filter.tableNameLike != "" && !strings.Contains(tableName, filter.tableNameLike) {
		return externalDatabaseTableSearchHit{}, false
	}
	if filter.tableNameLike != "" {
		reasons = append(reasons, fmt.Sprintf("table_name contains %q", filter.tableNameLike))
		score += 90
	}
	if filter.commentLike != "" && !strings.Contains(tableComment, filter.commentLike) {
		return externalDatabaseTableSearchHit{}, false
	}
	if filter.commentLike != "" {
		reasons = append(reasons, fmt.Sprintf("table_comment contains %q", filter.commentLike))
		score += 70
	}
	if filter.keyword != "" {
		if strings.Contains(tableName, filter.keyword) {
			keywordMatched = true
			reasons = append(reasons, fmt.Sprintf("table_name contains %q", filter.keyword))
			score += 100
		}
		if strings.Contains(tableComment, filter.keyword) {
			keywordMatched = true
			reasons = append(reasons, fmt.Sprintf("table_comment contains %q", filter.keyword))
			score += 80
		}
	}

	seenColumns := make(map[string]struct{})
	for _, column := range table.Columns {
		columnName := strings.ToLower(strings.TrimSpace(column.Name))
		columnComment := strings.ToLower(strings.TrimSpace(column.Comment))
		if filter.columnNameLike != "" && strings.Contains(columnName, filter.columnNameLike) {
			columnNameMatched = true
			reasons = append(reasons, fmt.Sprintf("column_name contains %q (%s)", filter.columnNameLike, column.Name))
			score += 60
			if _, ok := seenColumns[column.Name]; !ok {
				seenColumns[column.Name] = struct{}{}
				matchedColumns = append(matchedColumns, column.Name)
			}
		}
		if filter.columnCommentLike != "" && strings.Contains(columnComment, filter.columnCommentLike) {
			columnCommentMatched = true
			reasons = append(reasons, fmt.Sprintf("column_comment contains %q (%s)", filter.columnCommentLike, column.Name))
			score += 50
			if _, ok := seenColumns[column.Name]; !ok {
				seenColumns[column.Name] = struct{}{}
				matchedColumns = append(matchedColumns, column.Name)
			}
		}
		if filter.keyword != "" && strings.Contains(columnName, filter.keyword) {
			keywordMatched = true
			reasons = append(reasons, fmt.Sprintf("column_name contains %q (%s)", filter.keyword, column.Name))
			score += 45
			if _, ok := seenColumns[column.Name]; !ok {
				seenColumns[column.Name] = struct{}{}
				matchedColumns = append(matchedColumns, column.Name)
			}
		}
		if filter.keyword != "" && strings.Contains(columnComment, filter.keyword) {
			keywordMatched = true
			reasons = append(reasons, fmt.Sprintf("column_comment contains %q (%s)", filter.keyword, column.Name))
			score += 35
			if _, ok := seenColumns[column.Name]; !ok {
				seenColumns[column.Name] = struct{}{}
				matchedColumns = append(matchedColumns, column.Name)
			}
		}
	}

	if !keywordMatched || !columnNameMatched || !columnCommentMatched {
		return externalDatabaseTableSearchHit{}, false
	}
	if len(reasons) == 0 {
		return externalDatabaseTableSearchHit{}, false
	}
	return externalDatabaseTableSearchHit{
		TableName:          table.Name,
		TableComment:       table.Comment,
		MatchedColumns:     matchedColumns,
		MatchReasons:       dedupOrderedStrings(reasons),
		LikelyRole:         inferExternalDatabaseTableRole(table),
		Score:              score,
		ColumnCount:        len(table.Columns),
		MatchedColumnCount: len(matchedColumns),
	}, true
}

func inferExternalDatabaseTableRole(table types.TableSchema) string {
	text := strings.ToLower(strings.TrimSpace(table.Name + " " + table.Comment))
	if containsAny(text, []string{"log", "logs", "record", "records", "history", "event", "call", "calls", "execute", "execution", "dispatch", "流水", "日志", "记录", "事件", "调用", "执行", "处置"}) {
		return "fact_log"
	}
	if containsAny(text, []string{"rel", "relation", "map", "mapping", "link", "assoc", "bridge", "junction", "关联", "关系", "映射"}) {
		return "relation"
	}
	if containsAny(text, []string{"dict", "dictionary", "type", "types", "category", "enum", "code", "字典", "类型", "分类", "编码"}) {
		return "reference"
	}
	return "master"
}

func containsAny(text string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.Contains(text, candidate) {
			return true
		}
	}
	return false
}

func dedupOrderedStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func formatExternalDatabaseSearchTablesOutput(
	schema *types.DatabaseSchema,
	filter externalDatabaseSearchTablesFilter,
	hits []externalDatabaseTableSearchHit,
	totalMatched int,
	limit int,
) string {
	var builder strings.Builder
	builder.WriteString("=== External Database Table Search ===\n\n")
	builder.WriteString(fmt.Sprintf("Database: %s\n", schema.DatabaseName))
	if strings.TrimSpace(schema.SchemaName) != "" {
		builder.WriteString(fmt.Sprintf("Schema: %s\n", schema.SchemaName))
	}
	builder.WriteString(fmt.Sprintf("Scope table count: %d\n", len(schema.Tables)))
	builder.WriteString(fmt.Sprintf("Matched table count: %d\n", totalMatched))
	builder.WriteString(fmt.Sprintf("Returned hits: %d (limit=%d)\n", len(hits), limit))
	builder.WriteString("Filters:\n")
	if filter.keyword != "" {
		builder.WriteString(fmt.Sprintf("- keyword=%s\n", filter.keyword))
	}
	if filter.tableNameLike != "" {
		builder.WriteString(fmt.Sprintf("- table_name_like=%s\n", filter.tableNameLike))
	}
	if filter.commentLike != "" {
		builder.WriteString(fmt.Sprintf("- comment_like=%s\n", filter.commentLike))
	}
	if filter.columnNameLike != "" {
		builder.WriteString(fmt.Sprintf("- column_name_like=%s\n", filter.columnNameLike))
	}
	if filter.columnCommentLike != "" {
		builder.WriteString(fmt.Sprintf("- column_comment_like=%s\n", filter.columnCommentLike))
	}
	if len(hits) == 0 {
		builder.WriteString("\nNo tables matched the current filters.\n")
		builder.WriteString("Try broader keywords, or search by table_name_like / comment_like / column_name_like / column_comment_like.\n")
		return strings.TrimSpace(builder.String())
	}
	builder.WriteString("\nCandidate tables:\n")
	for _, hit := range hits {
		builder.WriteString(fmt.Sprintf("- %s [%s]\n", hit.TableName, hit.LikelyRole))
		if strings.TrimSpace(hit.TableComment) != "" {
			builder.WriteString(fmt.Sprintf("  Comment: %s\n", truncateSchemaText(hit.TableComment, schemaOutputCommentLimit)))
		}
		if len(hit.MatchedColumns) > 0 {
			builder.WriteString(fmt.Sprintf("  Matched columns: %s\n", strings.Join(hit.MatchedColumns, ", ")))
		}
		if len(hit.MatchReasons) > 0 {
			builder.WriteString(fmt.Sprintf("  Match reasons: %s\n", strings.Join(hit.MatchReasons, "; ")))
		}
	}
	if totalMatched > len(hits) {
		builder.WriteString(fmt.Sprintf("Additional matches omitted: %d\n", totalMatched-len(hits)))
	}
	builder.WriteString("Next step: choose the best candidate tables, then call external_database_schema with tables=[...] and mode=detail for full columns before generating SQL.\n")
	return strings.TrimSpace(builder.String())
}
