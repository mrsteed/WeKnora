package databaseconnector

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// DatabaseConnector is the runtime contract for realtime external database
// access. Unlike sync-oriented datasource connectors, it exposes schema
// discovery and SQL execution primitives needed by later schema/query services.
type DatabaseConnector interface {
	// Type returns the datasource type identifier such as mysql or postgresql.
	Type() string
	// Validate verifies that configuration is well-formed and that the remote
	// database is reachable with the supplied credentials.
	Validate(ctx context.Context, cfg *types.DatabaseConnectionConfig) error
	// DiscoverSchema returns a structured description of tables, views, columns,
	// primary keys and indexes visible to the configured database user.
	DiscoverSchema(ctx context.Context, cfg *types.DatabaseConnectionConfig) (*types.DatabaseSchema, error)
	// Query executes a read-only statement against the configured database.
	Query(ctx context.Context, cfg *types.DatabaseConnectionConfig, query string, timeout time.Duration) (*sql.Rows, error)
	// Dialect returns the SQL dialect implemented by the connector.
	Dialect() types.SQLDialect
}

// Registry stores the available realtime database connectors.
// It mirrors the existing datasource connector registry so container wiring and
// later adapters can follow the same lookup pattern.
type Registry struct {
	connectors map[string]DatabaseConnector
}

// NewRegistry creates an empty database connector registry.
func NewRegistry() *Registry {
	return &Registry{connectors: make(map[string]DatabaseConnector)}
}

// Register adds one connector implementation to the registry.
func (r *Registry) Register(connector DatabaseConnector) error {
	if connector == nil {
		return ErrConnectorNil
	}
	if connector.Type() == "" {
		return ErrConnectorTypeEmpty
	}
	r.connectors[connector.Type()] = connector
	return nil
}

// Get returns the connector bound to the provided datasource type.
func (r *Registry) Get(connectorType string) (DatabaseConnector, error) {
	connector, ok := r.connectors[connectorType]
	if !ok {
		return nil, ErrConnectorNotFound
	}
	return connector, nil
}

// List returns a stable, sorted list of registered connector type names.
func (r *Registry) List() []string {
	items := make([]string, 0, len(r.connectors))
	for key := range r.connectors {
		items = append(items, key)
	}
	sort.Strings(items)
	return items
}
