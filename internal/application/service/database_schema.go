package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

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
)

type schemaRegistryService struct {
	kbRepo     interfaces.KnowledgeBaseRepository
	dsRepo     interfaces.DataSourceRepository
	schemaRepo interfaces.DatabaseSchemaRepository
	dbRegistry *databaseconnector.Registry
}

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

	filterSet := make(map[string]struct{})
	for _, table := range selectedTables {
		trimmed := strings.ToLower(strings.TrimSpace(table))
		if trimmed != "" {
			filterSet[trimmed] = struct{}{}
		}
	}

	var tables []types.TableSchema
	for _, table := range schema.Tables {
		if len(filterSet) > 0 {
			if _, ok := filterSet[strings.ToLower(table.Name)]; !ok {
				continue
			}
		}
		tables = append(tables, table)
	}
	if len(filterSet) > 0 && len(tables) == 0 {
		return "", ErrRequestedTableNotFoundInSchema
	}
	if len(tables) == 0 {
		return "", ErrDatabaseSchemaSnapshotNotFound
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Database: %s\n", schema.DatabaseName))
	if strings.TrimSpace(schema.SchemaName) != "" {
		builder.WriteString(fmt.Sprintf("Schema: %s\n", schema.SchemaName))
	}
	builder.WriteString("Tables:\n")
	for _, table := range tables {
		builder.WriteString(fmt.Sprintf("- %s (%s)", table.Name, table.Type))
		if table.Comment != "" {
			builder.WriteString(": ")
			builder.WriteString(table.Comment)
		}
		builder.WriteString("\n")
		if len(table.PrimaryKeys) > 0 {
			builder.WriteString(fmt.Sprintf("  Primary keys: %s\n", strings.Join(table.PrimaryKeys, ", ")))
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
			if column.Comment != "" {
				builder.WriteString(fmt.Sprintf(" -- %s", column.Comment))
			}
			builder.WriteString("\n")
		}
	}
	return strings.TrimSpace(builder.String()), nil
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
