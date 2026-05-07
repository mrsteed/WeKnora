package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/databaseconnector"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

var (
	ErrDatabaseDataSourceNotFound     = errors.New("database datasource not found")
	ErrDatabaseSchemaSnapshotNotFound = errors.New("database schema snapshot not found")
	ErrDatabaseKnowledgeBaseRequired  = errors.New("knowledge base is not a database knowledge base")
	ErrMultipleDatabaseDataSources    = errors.New("multiple database datasources found for knowledge base")
	ErrRequestedTableNotFoundInSchema = errors.New("requested table not found in database schema")
	joinHintTablePattern              = regexp.MustCompile(`(?i)\b([a-z_][a-z0-9_]*)\.`)
)

type schemaRegistryService struct {
	kbRepo     interfaces.KnowledgeBaseRepository
	dsRepo     interfaces.DataSourceRepository
	schemaRepo interfaces.DatabaseSchemaRepository
	dbRegistry *databaseconnector.Registry
}

const (
	schemaAutoDetailTableLimit  = 12
	schemaAutoDetailColumnLimit = 160
	schemaCatalogTableLimit     = 20
	schemaCatalogColumnLimit    = 6
	schemaCatalogIndexLimit     = 3
	schemaPromptCommentLimit    = 160
)

func NewSchemaRegistryService(
	kbRepo interfaces.KnowledgeBaseRepository,
	dsRepo interfaces.DataSourceRepository,
	schemaRepo interfaces.DatabaseSchemaRepository,
	dbRegistry *databaseconnector.Registry,
) interfaces.SchemaRegistryService {
	return &schemaRegistryService{
		kbRepo:     kbRepo,
		dsRepo:     dsRepo,
		schemaRepo: schemaRepo,
		dbRegistry: dbRegistry,
	}
}

func (s *schemaRegistryService) RefreshSchema(ctx context.Context, dataSourceID string) error {
	ds, kb, cfg, err := s.resolveDataSourceAndKnowledgeBase(ctx, dataSourceID)
	if err != nil {
		return err
	}

	connector, err := s.dbRegistry.Get(ds.Type)
	if err != nil {
		return err
	}

	schema, err := connector.DiscoverSchema(ctx, cfg)
	if err != nil {
		return err
	}

	filteredSchema := applySchemaPolicies(schema, cfg)
	filteredSchema.TenantID = ds.TenantID
	filteredSchema.KnowledgeBaseID = ds.KnowledgeBaseID
	filteredSchema.DataSourceID = ds.ID
	filteredSchema.DatabaseType = ds.Type
	filteredSchema.DatabaseName = cfg.Settings.Database
	if filteredSchema.SchemaName == "" {
		filteredSchema.SchemaName = cfg.Settings.Schema
	}
	filteredSchema.SchemaHash = hashDatabaseSchema(filteredSchema)

	snapshot := &types.DatabaseSchemaSnapshot{
		TenantID:        ds.TenantID,
		KnowledgeBaseID: ds.KnowledgeBaseID,
		DataSourceID:    ds.ID,
		DatabaseType:    ds.Type,
		DatabaseName:    filteredSchema.DatabaseName,
		SchemaName:      filteredSchema.SchemaName,
		SchemaHash:      filteredSchema.SchemaHash,
		RefreshedAt:     filteredSchema.RefreshedAt,
	}
	if err := snapshot.SetSchema(filteredSchema); err != nil {
		return err
	}

	columns := flattenDatabaseColumns(filteredSchema)
	if err := s.schemaRepo.ReplaceSnapshot(ctx, snapshot, columns); err != nil {
		return err
	}

	logger.Infof(ctx, "database schema refreshed: datasource=%s kb=%s tables=%d", ds.ID, kb.ID, len(filteredSchema.Tables))
	return nil
}

func (s *schemaRegistryService) GetDatabaseSchema(ctx context.Context, kbID string) (*types.DatabaseSchema, error) {
	kb, err := s.getKnowledgeBase(ctx, kbID)
	if err != nil {
		return nil, err
	}
	if !kb.IsDatabaseEnabled() {
		return nil, ErrDatabaseKnowledgeBaseRequired
	}
	if _, err := s.resolveSingleDatabaseDataSource(ctx, kb); err != nil {
		return nil, err
	}

	snapshot, err := s.schemaRepo.GetLatestSnapshotByKnowledgeBase(ctx, kb.TenantID, kb.ID)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, ErrDatabaseSchemaSnapshotNotFound
	}
	return hydrateSchemaFromSnapshot(snapshot)
}

func (s *schemaRegistryService) GetTableSchema(ctx context.Context, kbID string, tableName string) (*types.TableSchema, error) {
	trimmedTableName := strings.TrimSpace(tableName)
	if trimmedTableName == "" {
		return nil, ErrRequestedTableNotFoundInSchema
	}
	schema, err := s.GetDatabaseSchema(ctx, kbID)
	if err != nil {
		return nil, err
	}
	for i := range schema.Tables {
		if strings.EqualFold(schema.Tables[i].Name, trimmedTableName) {
			table := schema.Tables[i]
			return &table, nil
		}
	}
	return nil, ErrRequestedTableNotFoundInSchema
}

func (s *schemaRegistryService) BuildPromptSchema(ctx context.Context, kbID string, selectedTables []string) (string, error) {
	schema, err := s.GetDatabaseSchema(ctx, kbID)
	if err != nil {
		return "", err
	}
	result, err := BuildPromptSchemaFromSchema(schema, selectedTables, types.PromptSchemaOptions{Mode: types.PromptSchemaModeAuto})
	if err != nil {
		return "", err
	}
	return result.Prompt, nil
}

func (s *schemaRegistryService) BuildPromptSchemaResult(
	ctx context.Context,
	kbID string,
	selectedTables []string,
	opts types.PromptSchemaOptions,
) (*types.PromptSchemaBuildResult, error) {
	schema, err := s.GetDatabaseSchema(ctx, kbID)
	if err != nil {
		return nil, err
	}
	result, err := BuildPromptSchemaFromSchema(schema, selectedTables, opts)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func BuildPromptSchemaFromSchema(
	schema *types.DatabaseSchema,
	selectedTables []string,
	opts types.PromptSchemaOptions,
) (*types.PromptSchemaBuildResult, error) {
	filteredTables, err := filterPromptSchemaTables(schema, selectedTables)
	if err != nil {
		return nil, err
	}
	tableCount := len(filteredTables)
	columnCount := countSchemaColumns(filteredTables)
	mode := resolvePromptSchemaMode(opts.Mode, filteredTables, selectedTables)
	displayTables := buildPromptDisplayTables(filteredTables, mode)
	result := &types.PromptSchemaBuildResult{
		Mode:                    mode,
		DatabaseName:            schema.DatabaseName,
		SchemaName:              schema.SchemaName,
		SchemaHash:              schema.SchemaHash,
		RefreshedAt:             schema.RefreshedAt,
		AllTables:               append([]types.TableSchema(nil), filteredTables...),
		DisplayTables:           displayTables,
		PossibleJoinHints:       filterPossibleJoinHintsForTables(schema.BusinessJoinHints, filteredTables),
		TableCount:              tableCount,
		ColumnCount:             columnCount,
		AdditionalTablesOmitted: maxInt(tableCount-len(displayTables), 0),
	}
	if mode == types.PromptSchemaModeCatalog {
		displayedColumns := 0
		for _, table := range displayTables {
			displayedColumns += minInt(len(table.Columns), schemaCatalogColumnLimit)
		}
		result.AdditionalColumnsOmitted = maxInt(columnCount-displayedColumns, 0)
	}
	result.Prompt = renderPromptSchema(schema, result)
	return result, nil
}

func filterPromptSchemaTables(schema *types.DatabaseSchema, selectedTables []string) ([]types.TableSchema, error) {
	if schema == nil || len(schema.Tables) == 0 {
		return nil, ErrDatabaseSchemaSnapshotNotFound
	}
	filterSet := make(map[string]struct{})
	for _, table := range selectedTables {
		trimmed := strings.ToLower(strings.TrimSpace(table))
		if trimmed != "" {
			filterSet[trimmed] = struct{}{}
		}
	}
	filtered := make([]types.TableSchema, 0, len(schema.Tables))
	for _, table := range schema.Tables {
		if len(filterSet) > 0 {
			if _, ok := filterSet[strings.ToLower(table.Name)]; !ok {
				continue
			}
		}
		filtered = append(filtered, table)
	}
	if len(filterSet) > 0 && len(filtered) == 0 {
		return nil, ErrRequestedTableNotFoundInSchema
	}
	if len(filtered) == 0 {
		return nil, ErrDatabaseSchemaSnapshotNotFound
	}
	return filtered, nil
}

func resolvePromptSchemaMode(mode types.PromptSchemaMode, tables []types.TableSchema, selectedTables []string) types.PromptSchemaMode {
	switch normalizePromptSchemaMode(mode) {
	case types.PromptSchemaModeCatalog:
		return types.PromptSchemaModeCatalog
	case types.PromptSchemaModeDetail:
		return types.PromptSchemaModeDetail
	}
	if len(selectedTables) > 0 {
		return types.PromptSchemaModeDetail
	}
	if len(tables) <= schemaAutoDetailTableLimit && countSchemaColumns(tables) <= schemaAutoDetailColumnLimit {
		return types.PromptSchemaModeDetail
	}
	return types.PromptSchemaModeCatalog
}

func normalizePromptSchemaMode(mode types.PromptSchemaMode) types.PromptSchemaMode {
	switch types.PromptSchemaMode(strings.ToLower(strings.TrimSpace(string(mode)))) {
	case types.PromptSchemaModeCatalog:
		return types.PromptSchemaModeCatalog
	case types.PromptSchemaModeDetail:
		return types.PromptSchemaModeDetail
	default:
		return types.PromptSchemaModeAuto
	}
}

func buildPromptDisplayTables(tables []types.TableSchema, mode types.PromptSchemaMode) []types.TableSchema {
	if mode == types.PromptSchemaModeDetail {
		return append([]types.TableSchema(nil), tables...)
	}
	limit := minInt(len(tables), schemaCatalogTableLimit)
	return append([]types.TableSchema(nil), tables[:limit]...)
}

func renderPromptSchema(schema *types.DatabaseSchema, result *types.PromptSchemaBuildResult) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Database: %s\n", schema.DatabaseName))
	if strings.TrimSpace(schema.SchemaName) != "" {
		builder.WriteString(fmt.Sprintf("Schema: %s\n", schema.SchemaName))
	}
	if !schema.RefreshedAt.IsZero() || strings.TrimSpace(schema.SchemaHash) != "" {
		parts := make([]string, 0, 2)
		if !schema.RefreshedAt.IsZero() {
			parts = append(parts, fmt.Sprintf("refreshed_at=%s", schema.RefreshedAt.UTC().Format(time.RFC3339)))
		}
		if strings.TrimSpace(schema.SchemaHash) != "" {
			parts = append(parts, fmt.Sprintf("schema_hash=%s", schema.SchemaHash))
		}
		builder.WriteString("Schema snapshot: ")
		builder.WriteString(strings.Join(parts, ", "))
		builder.WriteString("\n")
	}
	builder.WriteString(fmt.Sprintf("Schema output mode: %s\n", result.Mode))
	builder.WriteString(fmt.Sprintf("Scope summary: %d tables, %d columns\n", result.TableCount, result.ColumnCount))
	if dialect := strings.TrimSpace(schema.DatabaseType); dialect != "" {
		builder.WriteString(fmt.Sprintf("SQL dialect: %s\n", dialect))
		if strings.EqualFold(dialect, types.DatabaseTypeMySQL) {
			builder.WriteString("Dialect note: validation is dialect-aware for a safe SELECT-oriented subset. Prefer common analytic SQL patterns and keep MySQL-specific date expressions simple when possible.\n")
		}
	}
	if result.Mode == types.PromptSchemaModeCatalog {
		builder.WriteString("Catalog view: representative columns are shown first. Request detail mode with specific tables when you need full column lists.\n")
	}
	builder.WriteString(renderPromptQueryGuidance())
	builder.WriteString("Tables:\n")
	for _, table := range result.DisplayTables {
		builder.WriteString(renderPromptSchemaTable(table, result.Mode))
	}
	foreignKeys := summarizePromptSchemaForeignKeys(result.DisplayTables)
	if len(foreignKeys) > 0 {
		builder.WriteString("Foreign keys:\n")
		for _, item := range foreignKeys {
			builder.WriteString("- ")
			builder.WriteString(item)
			builder.WriteString("\n")
		}
	}
	if len(result.PossibleJoinHints) > 0 {
		builder.WriteString("Possible join hints:\n")
		for _, item := range result.PossibleJoinHints {
			builder.WriteString("- ")
			builder.WriteString(item)
			builder.WriteString("\n")
		}
	}
	if result.AdditionalTablesOmitted > 0 {
		builder.WriteString(fmt.Sprintf("Additional tables omitted from this view: %d\n", result.AdditionalTablesOmitted))
	}
	if result.Mode == types.PromptSchemaModeCatalog && result.AdditionalColumnsOmitted > 0 {
		builder.WriteString(fmt.Sprintf("Additional columns omitted from this view: %d\n", result.AdditionalColumnsOmitted))
		builder.WriteString("If required tables or columns are missing, call external_database_schema again with tables=[...] and mode=detail, or refresh schema when the snapshot looks stale.\n")
	}
	return strings.TrimSpace(builder.String())
}

func renderPromptQueryGuidance() string {
	var builder strings.Builder
	builder.WriteString("Query planning rules:\n")
	builder.WriteString("- Add LIMIT to any query that can return multiple rows. This includes detail previews, JOIN inspections, DISTINCT value lists, GROUP BY/HAVING summaries, ORDER BY top-N checks, window-function queries, and multi-row CTE outputs.\n")
	builder.WriteString("- Only pure global aggregates that return one row may omit LIMIT, such as COUNT(*), SUM(amount), AVG(score), MIN(created_at), MAX(created_at), or DISTINCT COUNT(*), with no GROUP BY and no window clause.\n")
	builder.WriteString("- For exploratory inspection, start with LIMIT 10 or LIMIT 20 and tighten WHERE conditions before widening scope.\n")
	return builder.String()
}

func renderPromptSchemaTable(table types.TableSchema, mode types.PromptSchemaMode) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("- %s (%s)", table.Name, table.Type))
	if comment := truncatePromptSchemaText(table.Comment, schemaPromptCommentLimit); comment != "" {
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
		if len(table.Indexes) > 0 {
			builder.WriteString(fmt.Sprintf("  Index summary: %s\n", summarizePromptSchemaIndexes(table.Indexes, schemaCatalogIndexLimit)))
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
		if comment := truncatePromptSchemaText(column.Comment, schemaPromptCommentLimit); comment != "" {
			builder.WriteString(fmt.Sprintf(" -- %s", comment))
		}
		builder.WriteString("\n")
	}
	if len(table.ForeignKeys) > 0 {
		builder.WriteString(fmt.Sprintf("  Foreign keys: %s\n", summarizePromptSchemaTableForeignKeys(table.ForeignKeys, len(table.ForeignKeys))))
	}
	if len(table.Indexes) > 0 {
		builder.WriteString(fmt.Sprintf("  Indexes: %s\n", summarizePromptSchemaIndexes(table.Indexes, len(table.Indexes))))
	}
	return builder.String()
}

func summarizePromptSchemaForeignKeys(tables []types.TableSchema) []string {
	items := make([]string, 0)
	for _, table := range tables {
		for _, fk := range table.ForeignKeys {
			items = append(items, formatForeignKeyHint(table.Name, fk))
		}
	}
	return items
}

func summarizePromptSchemaTableForeignKeys(foreignKeys []types.ForeignKeySchema, limit int) string {
	if len(foreignKeys) == 0 || limit <= 0 {
		return "<no foreign keys>"
	}
	items := make([]string, 0, minInt(len(foreignKeys), limit))
	for index, fk := range foreignKeys {
		if index >= limit {
			break
		}
		items = append(items, formatForeignKeyTarget(fk))
	}
	if len(foreignKeys) > limit {
		items = append(items, fmt.Sprintf("... +%d more", len(foreignKeys)-limit))
	}
	return strings.Join(items, "; ")
}

func formatForeignKeyHint(tableName string, fk types.ForeignKeySchema) string {
	return tableName + "." + formatForeignKeyTarget(fk)
}

func formatForeignKeyTarget(fk types.ForeignKeySchema) string {
	sourceColumns := strings.Join(fk.Columns, ", ")
	targetColumns := strings.Join(fk.ReferencedColumns, ", ")
	if len(fk.Columns) == 1 && len(fk.ReferencedColumns) == 1 {
		return fmt.Sprintf("%s -> %s.%s", sourceColumns, fk.ReferencedTable, targetColumns)
	}
	return fmt.Sprintf("(%s) -> %s(%s)", sourceColumns, fk.ReferencedTable, targetColumns)
}

func sanitizePossibleJoinHints(hints []string) []string {
	items := make([]string, 0, len(hints))
	seen := make(map[string]struct{}, len(hints))
	for _, hint := range hints {
		trimmed := strings.TrimSpace(hint)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		items = append(items, trimmed)
	}
	sort.Strings(items)
	return items
}

func filterPossibleJoinHintsForTables(hints []string, tables []types.TableSchema) []string {
	if len(hints) == 0 || len(tables) == 0 {
		return nil
	}
	allowedTables := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		name := strings.ToLower(strings.TrimSpace(table.Name))
		if name != "" {
			allowedTables[name] = struct{}{}
		}
	}
	filtered := make([]string, 0, len(hints))
	for _, hint := range hints {
		trimmed := strings.TrimSpace(hint)
		if trimmed == "" {
			continue
		}
		matches := joinHintTablePattern.FindAllStringSubmatch(trimmed, -1)
		if len(matches) == 0 {
			filtered = append(filtered, trimmed)
			continue
		}
		withinScope := true
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			if _, ok := allowedTables[strings.ToLower(match[1])]; !ok {
				withinScope = false
				break
			}
		}
		if withinScope {
			filtered = append(filtered, trimmed)
		}
	}
	return sanitizePossibleJoinHints(filtered)
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

func summarizePromptSchemaIndexes(indexes []types.IndexSchema, limit int) string {
	if len(indexes) == 0 || limit <= 0 {
		return "<no indexes>"
	}
	items := make([]string, 0, minInt(len(indexes), limit))
	for index, item := range indexes {
		if index >= limit {
			break
		}
		segment := item.Name
		if len(item.Columns) > 0 {
			segment += "(" + strings.Join(item.Columns, ", ") + ")"
		}
		if item.Unique {
			segment += " [unique]"
		}
		items = append(items, segment)
	}
	if len(indexes) > limit {
		items = append(items, fmt.Sprintf("... +%d more", len(indexes)-limit))
	}
	return strings.Join(items, "; ")
}

func countSchemaColumns(tables []types.TableSchema) int {
	total := 0
	for _, table := range tables {
		total += len(table.Columns)
	}
	return total
}

func truncatePromptSchemaText(text string, limit int) string {
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

func (s *schemaRegistryService) resolveDataSourceAndKnowledgeBase(
	ctx context.Context,
	dataSourceID string,
) (*types.DataSource, *types.KnowledgeBase, *types.DatabaseConnectionConfig, error) {
	ds, err := s.dsRepo.FindByID(ctx, dataSourceID)
	if err != nil {
		return nil, nil, nil, err
	}
	if ds == nil || !isDatabaseDataSourceType(ds.Type) {
		return nil, nil, nil, ErrDatabaseDataSourceNotFound
	}
	if tenantID, ok := types.TenantIDFromContext(ctx); ok && tenantID != ds.TenantID {
		return nil, nil, nil, ErrDatabaseDataSourceNotFound
	}

	kb, err := s.getKnowledgeBase(ctx, ds.KnowledgeBaseID)
	if err != nil {
		return nil, nil, nil, err
	}
	if kb.TenantID != ds.TenantID {
		return nil, nil, nil, ErrDatabaseDataSourceNotFound
	}
	if !kb.IsDatabaseEnabled() {
		return nil, nil, nil, ErrDatabaseKnowledgeBaseRequired
	}

	cfg, err := ds.ParseDatabaseConnectionConfig()
	if err != nil {
		return nil, nil, nil, err
	}
	if cfg == nil {
		return nil, nil, nil, errors.New("database datasource config is empty")
	}
	return ds, kb, cfg, nil
}

func (s *schemaRegistryService) getKnowledgeBase(ctx context.Context, kbID string) (*types.KnowledgeBase, error) {
	if tenantID, ok := types.TenantIDFromContext(ctx); ok {
		return s.kbRepo.GetKnowledgeBaseByIDAndTenant(ctx, kbID, tenantID)
	}
	return s.kbRepo.GetKnowledgeBaseByID(ctx, kbID)
}

func (s *schemaRegistryService) resolveSingleDatabaseDataSource(
	ctx context.Context,
	kb *types.KnowledgeBase,
) (*types.DataSource, error) {
	if kb == nil {
		return nil, ErrDatabaseDataSourceNotFound
	}
	dataSources, err := s.dsRepo.FindByKnowledgeBase(ctx, kb.ID)
	if err != nil {
		return nil, err
	}
	var databaseSources []*types.DataSource
	for _, ds := range dataSources {
		if ds != nil && isDatabaseDataSourceType(ds.Type) && ds.TenantID == kb.TenantID {
			databaseSources = append(databaseSources, ds)
		}
	}
	switch len(databaseSources) {
	case 0:
		return nil, ErrDatabaseDataSourceNotFound
	case 1:
		return databaseSources[0], nil
	default:
		return nil, ErrMultipleDatabaseDataSources
	}
}

func hydrateSchemaFromSnapshot(snapshot *types.DatabaseSchemaSnapshot) (*types.DatabaseSchema, error) {
	schema, err := snapshot.ParseSchema()
	if err != nil {
		return nil, err
	}
	if schema == nil {
		return nil, ErrDatabaseSchemaSnapshotNotFound
	}
	schema.ID = snapshot.ID
	schema.TenantID = snapshot.TenantID
	schema.KnowledgeBaseID = snapshot.KnowledgeBaseID
	schema.DataSourceID = snapshot.DataSourceID
	if schema.DatabaseType == "" {
		schema.DatabaseType = snapshot.DatabaseType
	}
	if schema.DatabaseName == "" {
		schema.DatabaseName = snapshot.DatabaseName
	}
	if schema.SchemaName == "" {
		schema.SchemaName = snapshot.SchemaName
	}
	if schema.SchemaHash == "" {
		schema.SchemaHash = snapshot.SchemaHash
	}
	if schema.RefreshedAt.IsZero() {
		schema.RefreshedAt = snapshot.RefreshedAt
	}
	return schema, nil
}

func applySchemaPolicies(schema *types.DatabaseSchema, cfg *types.DatabaseConnectionConfig) *types.DatabaseSchema {
	if schema == nil {
		return &types.DatabaseSchema{}
	}
	filtered := *schema
	filtered.Tables = make([]types.TableSchema, 0, len(schema.Tables))
	filtered.BusinessJoinHints = sanitizePossibleJoinHints(cfg.Settings.BusinessJoinHints)

	allowlist := make(map[string]struct{})
	for _, table := range cfg.Settings.TableAllowlist {
		trimmed := strings.ToLower(strings.TrimSpace(table))
		if trimmed != "" {
			allowlist[trimmed] = struct{}{}
		}
	}

	denylist := make(map[string]struct{})
	for _, column := range cfg.Settings.ColumnDenylist {
		trimmed := strings.ToLower(strings.TrimSpace(column))
		if trimmed != "" {
			denylist[trimmed] = struct{}{}
		}
	}

	for _, table := range schema.Tables {
		if len(allowlist) > 0 {
			if _, ok := allowlist[strings.ToLower(table.Name)]; !ok {
				continue
			}
		}
		nextTable := table
		nextTable.Columns = nextTable.Columns[:0]
		for _, column := range table.Columns {
			key := strings.ToLower(strings.TrimSpace(table.Name + "." + column.Name))
			if _, denied := denylist[key]; denied {
				continue
			}
			nextTable.Columns = append(nextTable.Columns, column)
		}
		if len(nextTable.Columns) == 0 && len(table.Columns) > 0 {
			continue
		}
		filtered.Tables = append(filtered.Tables, nextTable)
	}

	visibleColumns := make(map[string]map[string]struct{}, len(filtered.Tables))
	for _, table := range filtered.Tables {
		columns := make(map[string]struct{}, len(table.Columns))
		for _, column := range table.Columns {
			columns[strings.ToLower(column.Name)] = struct{}{}
		}
		visibleColumns[strings.ToLower(table.Name)] = columns
	}
	for index := range filtered.Tables {
		tableName := strings.ToLower(filtered.Tables[index].Name)
		nextForeignKeys := make([]types.ForeignKeySchema, 0, len(filtered.Tables[index].ForeignKeys))
		for _, fk := range filtered.Tables[index].ForeignKeys {
			targetColumns, ok := visibleColumns[strings.ToLower(fk.ReferencedTable)]
			if !ok {
				continue
			}
			sourceColumns := visibleColumns[tableName]
			valid := true
			for _, column := range fk.Columns {
				if _, ok := sourceColumns[strings.ToLower(column)]; !ok {
					valid = false
					break
				}
			}
			if !valid {
				continue
			}
			for _, column := range fk.ReferencedColumns {
				if _, ok := targetColumns[strings.ToLower(column)]; !ok {
					valid = false
					break
				}
			}
			if !valid {
				continue
			}
			nextForeignKeys = append(nextForeignKeys, fk)
		}
		filtered.Tables[index].ForeignKeys = nextForeignKeys
	}

	sort.Slice(filtered.Tables, func(i, j int) bool {
		return strings.ToLower(filtered.Tables[i].Name) < strings.ToLower(filtered.Tables[j].Name)
	})
	return &filtered
}

func flattenDatabaseColumns(schema *types.DatabaseSchema) []*types.DatabaseTableColumn {
	if schema == nil {
		return nil
	}
	columns := make([]*types.DatabaseTableColumn, 0)
	for _, table := range schema.Tables {
		for index, column := range table.Columns {
			columns = append(columns, &types.DatabaseTableColumn{
				Table:           table.Name,
				ColumnName:      column.Name,
				DataType:        column.DataType,
				Nullable:        column.Nullable,
				Comment:         column.Comment,
				IsSensitive:     column.IsSensitive,
				OrdinalPosition: index + 1,
			})
		}
	}
	return columns
}

func hashDatabaseSchema(schema *types.DatabaseSchema) string {
	if schema == nil {
		return ""
	}
	raw, _ := json.Marshal(schema)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
