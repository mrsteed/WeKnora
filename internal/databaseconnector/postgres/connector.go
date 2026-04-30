package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/datasource"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

const (
	defaultPort            = 5432
	defaultSchema          = "public"
	defaultValidateTimeout = 5 * time.Second
	defaultQueryTimeout    = 30 * time.Second
)

// Connector implements realtime PostgreSQL connectivity, schema discovery and query execution.
// A small cache of *sql.DB handles is kept per DSN so later services can reuse pooled
// connections for schema refresh and query execution within the singleton connector instance.
type Connector struct {
	mu      sync.RWMutex
	clients map[string]*sql.DB
}

// NewConnector creates a PostgreSQL database connector instance.
func NewConnector() *Connector {
	return &Connector{clients: make(map[string]*sql.DB)}
}

func (c *Connector) Type() string { return types.DatabaseTypePostgreSQL }

func (c *Connector) Dialect() types.SQLDialect { return types.SQLDialectPostgreSQL }

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
		return fmt.Errorf("validate postgres connection: %w", err)
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

	schemaName := normalizedSchema(cfg)
	meta, err := fetchTableMetadata(ctx, db, schemaName)
	if err != nil {
		return nil, err
	}
	if err := fetchColumnMetadata(ctx, db, schemaName, meta); err != nil {
		return nil, err
	}
	if err := fetchPrimaryKeys(ctx, db, schemaName, meta); err != nil {
		return nil, err
	}
	if err := fetchIndexes(ctx, db, schemaName, meta); err != nil {
		return nil, err
	}

	return &types.DatabaseSchema{
		DatabaseType: types.DatabaseTypePostgreSQL,
		DatabaseName: cfg.Settings.Database,
		SchemaName:   schemaName,
		Tables:       finalizeTables(meta),
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
	dsn, err := buildDSN(cfg)
	if err != nil {
		return nil, err
	}

	c.mu.RLock()
	if db, ok := c.clients[dsn]; ok {
		c.mu.RUnlock()
		return db, nil
	}
	c.mu.RUnlock()

	pgCfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres connection string: %w", err)
	}
	db := stdlib.OpenDB(*pgCfg)
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
	if err := validateConfig(cfg); err != nil {
		return "", err
	}
	port := cfg.Settings.Port
	if port == 0 {
		port = defaultPort
	}
	sslMode := strings.TrimSpace(cfg.Settings.SSLMode)
	if sslMode == "" {
		sslMode = "disable"
	}
	query := url.Values{}
	query.Set("sslmode", sslMode)
	query.Set("search_path", normalizedSchema(cfg))
	return (&url.URL{
		Scheme:   "postgresql",
		User:     url.UserPassword(cfg.Credentials.Username, cfg.Credentials.Password),
		Host:     fmt.Sprintf("%s:%d", cfg.Settings.Host, port),
		Path:     cfg.Settings.Database,
		RawQuery: query.Encode(),
	}).String(), nil
}

func normalizedSchema(cfg *types.DatabaseConnectionConfig) string {
	if cfg == nil || strings.TrimSpace(cfg.Settings.Schema) == "" {
		return defaultSchema
	}
	return strings.TrimSpace(cfg.Settings.Schema)
}

type tableAccumulator struct {
	Table      types.TableSchema
	indexOrder map[string][]indexedColumn
}

type indexedColumn struct {
	seq    int
	column string
}

func fetchTableMetadata(ctx context.Context, db *sql.DB, schemaName string) (map[string]*tableAccumulator, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT c.relname,
		       CASE c.relkind WHEN 'v' THEN 'view' WHEN 'm' THEN 'view' ELSE 'table' END,
		       COALESCE(obj_description(c.oid, 'pg_class'), ''),
		       COALESCE(s.n_live_tup::bigint, 0)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN pg_stat_user_tables s ON s.relid = c.oid
		WHERE n.nspname = $1 AND c.relkind IN ('r', 'p', 'v', 'm')
		ORDER BY c.relname
	`, schemaName)
	if err != nil {
		return nil, fmt.Errorf("discover postgres tables: %w", err)
	}
	defer rows.Close()

	meta := make(map[string]*tableAccumulator)
	for rows.Next() {
		var name, tableType, comment string
		var rowEstimate sql.NullInt64
		if err := rows.Scan(&name, &tableType, &comment, &rowEstimate); err != nil {
			return nil, fmt.Errorf("scan postgres table metadata: %w", err)
		}
		meta[name] = &tableAccumulator{
			Table: types.TableSchema{
				Name:        name,
				Type:        tableType,
				Comment:     comment,
				RowEstimate: rowEstimate.Int64,
			},
			indexOrder: make(map[string][]indexedColumn),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres table metadata: %w", err)
	}
	return meta, nil
}

func fetchColumnMetadata(ctx context.Context, db *sql.DB, schemaName string, meta map[string]*tableAccumulator) error {
	rows, err := db.QueryContext(ctx, `
		SELECT c.table_name,
		       c.column_name,
		       c.data_type,
		       c.is_nullable,
		       COALESCE(pg_catalog.col_description(cls.oid, a.attnum), ''),
		       c.ordinal_position
		FROM information_schema.columns c
		JOIN pg_namespace n ON n.nspname = c.table_schema
		JOIN pg_class cls ON cls.relname = c.table_name AND cls.relnamespace = n.oid
		JOIN pg_attribute a ON a.attrelid = cls.oid AND a.attname = c.column_name
		WHERE c.table_schema = $1
		ORDER BY c.table_name, c.ordinal_position
	`, schemaName)
	if err != nil {
		return fmt.Errorf("discover postgres columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tableName, columnName, dataType, isNullable, comment string
		var ordinal int
		if err := rows.Scan(&tableName, &columnName, &dataType, &isNullable, &comment, &ordinal); err != nil {
			return fmt.Errorf("scan postgres column metadata: %w", err)
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

func fetchPrimaryKeys(ctx context.Context, db *sql.DB, schemaName string, meta map[string]*tableAccumulator) error {
	rows, err := db.QueryContext(ctx, `
		SELECT kc.table_name, kc.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kc
		  ON tc.constraint_name = kc.constraint_name
		 AND tc.table_schema = kc.table_schema
		WHERE tc.constraint_type = 'PRIMARY KEY' AND tc.table_schema = $1
		ORDER BY kc.table_name, kc.ordinal_position
	`, schemaName)
	if err != nil {
		return fmt.Errorf("discover postgres primary keys: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tableName, columnName string
		if err := rows.Scan(&tableName, &columnName); err != nil {
			return fmt.Errorf("scan postgres primary keys: %w", err)
		}
		entry := meta[tableName]
		if entry == nil {
			continue
		}
		entry.Table.PrimaryKeys = append(entry.Table.PrimaryKeys, columnName)
	}
	return rows.Err()
}

func fetchIndexes(ctx context.Context, db *sql.DB, schemaName string, meta map[string]*tableAccumulator) error {
	rows, err := db.QueryContext(ctx, `
		SELECT t.relname,
		       i.relname,
		       a.attname,
		       NOT ix.indisunique,
		       array_position(ix.indkey, a.attnum),
		       am.amname
		FROM pg_class t
		JOIN pg_namespace n ON n.oid = t.relnamespace
		JOIN pg_index ix ON t.oid = ix.indrelid
		JOIN pg_class i ON i.oid = ix.indexrelid
		JOIN pg_am am ON am.oid = i.relam
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
		WHERE n.nspname = $1 AND t.relkind IN ('r', 'p', 'v', 'm')
		ORDER BY t.relname, i.relname, array_position(ix.indkey, a.attnum)
	`, schemaName)
	if err != nil {
		return fmt.Errorf("discover postgres indexes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tableName, indexName, columnName, indexType string
		var nonUnique bool
		var seq sql.NullInt64
		if err := rows.Scan(&tableName, &indexName, &columnName, &nonUnique, &seq, &indexType); err != nil {
			return fmt.Errorf("scan postgres indexes: %w", err)
		}
		entry := meta[tableName]
		if entry == nil {
			continue
		}

		found := false
		for idx := range entry.Table.Indexes {
			if entry.Table.Indexes[idx].Name == indexName {
				entry.Table.Indexes[idx].Unique = !nonUnique
				entry.Table.Indexes[idx].IndexType = indexType
				entry.indexOrder[indexName] = append(entry.indexOrder[indexName], indexedColumn{seq: int(seq.Int64), column: columnName})
				found = true
				break
			}
		}
		if !found {
			entry.Table.Indexes = append(entry.Table.Indexes, types.IndexSchema{
				Name:      indexName,
				Unique:    !nonUnique,
				IndexType: indexType,
			})
			entry.indexOrder[indexName] = append(entry.indexOrder[indexName], indexedColumn{seq: int(seq.Int64), column: columnName})
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
