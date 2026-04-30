package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/datasource"
	"github.com/Tencent/WeKnora/internal/types"
	mysqlDriver "github.com/go-sql-driver/mysql"
)

const (
	defaultPort            = 3306
	defaultValidateTimeout = 5 * time.Second
	defaultQueryTimeout    = 30 * time.Second
)

// Connector implements realtime MySQL connectivity, schema discovery and query execution.
// It keeps a small in-memory DB handle cache keyed by DSN so repeated schema/query calls
// can reuse pooled connections instead of reopening database handles every time.
type Connector struct {
	mu      sync.RWMutex
	clients map[string]*sql.DB
}

// NewConnector creates a MySQL database connector instance.
func NewConnector() *Connector {
	return &Connector{clients: make(map[string]*sql.DB)}
}

func (c *Connector) Type() string { return types.DatabaseTypeMySQL }

func (c *Connector) Dialect() types.SQLDialect { return types.SQLDialectMySQL }

func (c *Connector) Validate(ctx context.Context, cfg *types.DatabaseConnectionConfig) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}

	db, err := c.openDB(cfg)
	if err != nil {
		return err
	}

	pingCtx, cancel := withDefaultTimeout(ctx, defaultValidateTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return fmt.Errorf("validate mysql connection: %w", err)
	}
	return nil
}

func (c *Connector) DiscoverSchema(ctx context.Context, cfg *types.DatabaseConnectionConfig) (*types.DatabaseSchema, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	db, err := c.openDB(cfg)
	if err != nil {
		return nil, err
	}

	meta, err := fetchTableMetadata(ctx, db, cfg.Settings.Database)
	if err != nil {
		return nil, err
	}
	if err := fetchColumnMetadata(ctx, db, cfg.Settings.Database, meta); err != nil {
		return nil, err
	}
	if err := fetchPrimaryKeys(ctx, db, cfg.Settings.Database, meta); err != nil {
		return nil, err
	}
	if err := fetchIndexes(ctx, db, cfg.Settings.Database, meta); err != nil {
		return nil, err
	}

	tables := finalizeTables(meta)
	return &types.DatabaseSchema{
		DatabaseType: types.DatabaseTypeMySQL,
		DatabaseName: cfg.Settings.Database,
		SchemaName:   cfg.Settings.Database,
		Tables:       tables,
		RefreshedAt:  time.Now().UTC(),
	}, nil
}

func (c *Connector) Query(ctx context.Context, cfg *types.DatabaseConnectionConfig, query string, timeout time.Duration) (*sql.Rows, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	db, err := c.openDB(cfg)
	if err != nil {
		return nil, err
	}

	queryCtx, cancel := withDefaultTimeout(ctx, normalizedTimeout(timeout, cfg.Settings.QueryTimeoutSec))
	rows, err := db.QueryContext(queryCtx, query)
	if err != nil {
		cancel()
		return nil, err
	}
	return rows, nil
}

func (c *Connector) openDB(cfg *types.DatabaseConnectionConfig) (*sql.DB, error) {
	dsnCfg, err := buildConfig(cfg)
	if err != nil {
		return nil, err
	}
	dsn := dsnCfg.FormatDSN()

	c.mu.RLock()
	if db, ok := c.clients[dsn]; ok {
		c.mu.RUnlock()
		return db, nil
	}
	c.mu.RUnlock()

	connector, err := mysqlDriver.NewConnector(dsnCfg)
	if err != nil {
		return nil, fmt.Errorf("build mysql connector: %w", err)
	}
	db := sql.OpenDB(connector)
	if db == nil {
		return nil, fmt.Errorf("open mysql connection: connector returned nil db")
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)

	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.clients[dsn]; ok {
		_ = db.Close()
		return existing, nil
	}
	c.clients[dsn] = db
	return db, nil
}

func validateConfig(cfg *types.DatabaseConnectionConfig) error {
	if cfg == nil {
		return fmt.Errorf("%w: config is nil", datasource.ErrInvalidConfig)
	}
	if strings.TrimSpace(cfg.Settings.Host) == "" {
		return fmt.Errorf("%w: host is required", datasource.ErrInvalidConfig)
	}
	if strings.TrimSpace(cfg.Settings.Database) == "" {
		return fmt.Errorf("%w: database is required", datasource.ErrInvalidConfig)
	}
	if strings.TrimSpace(cfg.Credentials.Username) == "" {
		return fmt.Errorf("%w: username is required", datasource.ErrInvalidCredentials)
	}
	return nil
}

func buildDSN(cfg *types.DatabaseConnectionConfig) (string, error) {
	dsnCfg, err := buildConfig(cfg)
	if err != nil {
		return "", err
	}
	return dsnCfg.FormatDSN(), nil
}

func buildConfig(cfg *types.DatabaseConnectionConfig) (*mysqlDriver.Config, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	port := cfg.Settings.Port
	if port == 0 {
		port = defaultPort
	}
	dsnCfg := mysqlDriver.NewConfig()
	dsnCfg.User = cfg.Credentials.Username
	dsnCfg.Passwd = cfg.Credentials.Password
	dsnCfg.Net = "tcp"
	dsnCfg.Addr = fmt.Sprintf("%s:%d", cfg.Settings.Host, port)
	dsnCfg.DBName = cfg.Settings.Database
	dsnCfg.ParseTime = true
	dsnCfg.MultiStatements = false
	dsnCfg.Params = map[string]string{
		"charset": "utf8mb4",
	}
	dsnCfg.TLSConfig = normalizeTLSMode(cfg.Settings.SSLMode)
	return dsnCfg, nil
}

func normalizeTLSMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "required", "require", "true":
		return "true"
	case "preferred":
		return "preferred"
	case "skip-verify", "insecure":
		return "skip-verify"
	default:
		return "false"
	}
}

type tableAccumulator struct {
	Table      types.TableSchema
	indexOrder map[string][]indexedColumn
}

type indexedColumn struct {
	seq    int
	column string
}

func fetchTableMetadata(ctx context.Context, db *sql.DB, databaseName string) (map[string]*tableAccumulator, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT table_name, table_type, COALESCE(table_comment, ''), COALESCE(table_rows, 0)
		FROM information_schema.tables
		WHERE table_schema = ?
		ORDER BY table_name
	`, databaseName)
	if err != nil {
		return nil, fmt.Errorf("discover mysql tables: %w", err)
	}
	defer rows.Close()

	meta := make(map[string]*tableAccumulator)
	for rows.Next() {
		var name, tableType, comment string
		var rowEstimate sql.NullInt64
		if err := rows.Scan(&name, &tableType, &comment, &rowEstimate); err != nil {
			return nil, fmt.Errorf("scan mysql table metadata: %w", err)
		}
		meta[name] = &tableAccumulator{
			Table: types.TableSchema{
				Name:        name,
				Type:        normalizeTableType(tableType),
				Comment:     comment,
				RowEstimate: rowEstimate.Int64,
			},
			indexOrder: make(map[string][]indexedColumn),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mysql table metadata: %w", err)
	}
	return meta, nil
}

func fetchColumnMetadata(ctx context.Context, db *sql.DB, databaseName string, meta map[string]*tableAccumulator) error {
	rows, err := db.QueryContext(ctx, `
		SELECT table_name, column_name, data_type, is_nullable, COALESCE(column_comment, ''), ordinal_position
		FROM information_schema.columns
		WHERE table_schema = ?
		ORDER BY table_name, ordinal_position
	`, databaseName)
	if err != nil {
		return fmt.Errorf("discover mysql columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tableName, columnName, dataType, isNullable, comment string
		var ordinal int
		if err := rows.Scan(&tableName, &columnName, &dataType, &isNullable, &comment, &ordinal); err != nil {
			return fmt.Errorf("scan mysql column metadata: %w", err)
		}
		entry := meta[tableName]
		if entry == nil {
			continue
		}
		entry.Table.Columns = append(entry.Table.Columns, types.ColumnSchema{
			Name:     columnName,
			DataType: dataType,
			Nullable: strings.EqualFold(isNullable, "YES"),
			Comment:  comment,
		})
	}
	return rows.Err()
}

func fetchPrimaryKeys(ctx context.Context, db *sql.DB, databaseName string, meta map[string]*tableAccumulator) error {
	rows, err := db.QueryContext(ctx, `
		SELECT table_name, column_name
		FROM information_schema.key_column_usage
		WHERE table_schema = ? AND constraint_name = 'PRIMARY'
		ORDER BY table_name, ordinal_position
	`, databaseName)
	if err != nil {
		return fmt.Errorf("discover mysql primary keys: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tableName, columnName string
		if err := rows.Scan(&tableName, &columnName); err != nil {
			return fmt.Errorf("scan mysql primary keys: %w", err)
		}
		entry := meta[tableName]
		if entry == nil {
			continue
		}
		entry.Table.PrimaryKeys = append(entry.Table.PrimaryKeys, columnName)
	}
	return rows.Err()
}

func fetchIndexes(ctx context.Context, db *sql.DB, databaseName string, meta map[string]*tableAccumulator) error {
	rows, err := db.QueryContext(ctx, `
		SELECT table_name, index_name, column_name, non_unique, seq_in_index, index_type
		FROM information_schema.statistics
		WHERE table_schema = ?
		ORDER BY table_name, index_name, seq_in_index
	`, databaseName)
	if err != nil {
		return fmt.Errorf("discover mysql indexes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tableName, indexName, columnName, indexType string
		var nonUnique, seq int
		if err := rows.Scan(&tableName, &indexName, &columnName, &nonUnique, &seq, &indexType); err != nil {
			return fmt.Errorf("scan mysql indexes: %w", err)
		}
		entry := meta[tableName]
		if entry == nil {
			continue
		}

		found := false
		for idx := range entry.Table.Indexes {
			if entry.Table.Indexes[idx].Name == indexName {
				entry.Table.Indexes[idx].Unique = nonUnique == 0
				entry.Table.Indexes[idx].IndexType = indexType
				entry.indexOrder[indexName] = append(entry.indexOrder[indexName], indexedColumn{seq: seq, column: columnName})
				found = true
				break
			}
		}
		if !found {
			entry.Table.Indexes = append(entry.Table.Indexes, types.IndexSchema{
				Name:      indexName,
				Unique:    nonUnique == 0,
				IndexType: indexType,
			})
			entry.indexOrder[indexName] = append(entry.indexOrder[indexName], indexedColumn{seq: seq, column: columnName})
		}
	}
	return rows.Err()
}

func finalizeTables(meta map[string]*tableAccumulator) []types.TableSchema {
	tables := make([]types.TableSchema, 0, len(meta))
	names := make([]string, 0, len(meta))
	for name := range meta {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		entry := meta[name]
		for idx := range entry.Table.Indexes {
			items := entry.indexOrder[entry.Table.Indexes[idx].Name]
			sort.Slice(items, func(i, j int) bool { return items[i].seq < items[j].seq })
			entry.Table.Indexes[idx].Columns = make([]string, 0, len(items))
			for _, item := range items {
				entry.Table.Indexes[idx].Columns = append(entry.Table.Indexes[idx].Columns, item.column)
			}
		}
		tables = append(tables, entry.Table)
	}
	return tables
}

func normalizeTableType(tableType string) string {
	switch strings.ToUpper(strings.TrimSpace(tableType)) {
	case "BASE TABLE":
		return "table"
	case "VIEW", "SYSTEM VIEW":
		return "view"
	default:
		return strings.ToLower(strings.TrimSpace(tableType))
	}
}

func withDefaultTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func normalizedTimeout(timeout time.Duration, configuredSeconds int) time.Duration {
	if timeout > 0 {
		return timeout
	}
	if configuredSeconds > 0 {
		return time.Duration(configuredSeconds) * time.Second
	}
	return defaultQueryTimeout
}
