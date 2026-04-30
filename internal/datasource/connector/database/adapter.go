package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/databaseconnector"
	"github.com/Tencent/WeKnora/internal/datasource"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/utils"
)

// Adapter bridges the datasource management APIs onto realtime database connectors.
// It reuses the existing datasource.Connector contract for create/validate/resource-discovery
// while explicitly rejecting sync-oriented fetch operations.
type Adapter struct {
	connectorType string
	registry      *databaseconnector.Registry
}

// NewAdapter creates a datasource adapter for one realtime database connector type.
func NewAdapter(connectorType string, registry *databaseconnector.Registry) *Adapter {
	return &Adapter{connectorType: connectorType, registry: registry}
}

// Type returns the datasource type identifier exposed through datasource APIs.
func (a *Adapter) Type() string {
	return a.connectorType
}

// Validate checks that the typed database configuration can connect successfully.
func (a *Adapter) Validate(ctx context.Context, config *types.DataSourceConfig) error {
	connector, cfg, err := a.resolve(config)
	if err != nil {
		return err
	}
	return connector.Validate(ctx, cfg)
}

// ListResources discovers database/schema/table/view resources for datasource management UI.
func (a *Adapter) ListResources(ctx context.Context, config *types.DataSourceConfig) ([]types.Resource, error) {
	connector, cfg, err := a.resolve(config)
	if err != nil {
		return nil, err
	}

	schema, err := connector.DiscoverSchema(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return schemaToResources(schema), nil
}

// FetchAll is intentionally unsupported because realtime databases do not use sync ingestion.
func (a *Adapter) FetchAll(context.Context, *types.DataSourceConfig, []string) ([]types.FetchedItem, error) {
	return nil, datasource.ErrSyncNotSupported
}

// FetchIncremental is intentionally unsupported because realtime databases do not use sync ingestion.
func (a *Adapter) FetchIncremental(context.Context, *types.DataSourceConfig, *types.SyncCursor) ([]types.FetchedItem, *types.SyncCursor, error) {
	return nil, nil, datasource.ErrSyncNotSupported
}

func (a *Adapter) resolve(config *types.DataSourceConfig) (databaseconnector.DatabaseConnector, *types.DatabaseConnectionConfig, error) {
	if a.registry == nil {
		return nil, nil, databaseconnector.ErrConnectorNotFound
	}
	connector, err := a.registry.Get(a.connectorType)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := toDatabaseConnectionConfig(a.connectorType, config)
	if err != nil {
		return nil, nil, err
	}
	return connector, cfg, nil
}

func toDatabaseConnectionConfig(connectorType string, config *types.DataSourceConfig) (*types.DatabaseConnectionConfig, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: config is nil", datasource.ErrInvalidConfig)
	}

	raw, err := json.Marshal(struct {
		Type        string                 `json:"type"`
		Credentials map[string]interface{} `json:"credentials"`
		Settings    map[string]interface{} `json:"settings"`
	}{
		Type:        firstNonEmpty(strings.TrimSpace(config.Type), connectorType),
		Credentials: config.Credentials,
		Settings:    config.Settings,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: marshal database config: %v", datasource.ErrInvalidConfig, err)
	}

	var cfg types.DatabaseConnectionConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("%w: decode database config: %v", datasource.ErrInvalidConfig, err)
	}
	if cfg.Type == "" {
		cfg.Type = connectorType
	}

	password, err := decryptPasswordIfNeeded(cfg.Credentials.Password)
	if err != nil {
		return nil, err
	}
	cfg.Credentials.Password = password
	return &cfg, nil
}

func decryptPasswordIfNeeded(password string) (string, error) {
	if !strings.HasPrefix(password, utils.EncPrefix) {
		return password, nil
	}
	key := utils.GetAESKey()
	if key == nil {
		return "", fmt.Errorf("%w: encrypted database password requires SYSTEM_AES_KEY", datasource.ErrInvalidConfig)
	}
	plaintext, err := utils.DecryptAESGCM(password, key)
	if err != nil {
		return "", fmt.Errorf("%w: decrypt database password: %v", datasource.ErrInvalidConfig, err)
	}
	return plaintext, nil
}

func schemaToResources(schema *types.DatabaseSchema) []types.Resource {
	if schema == nil {
		return nil
	}

	databaseID := fmt.Sprintf("database:%s", schema.DatabaseName)
	schemaName := firstNonEmpty(schema.SchemaName, schema.DatabaseName)
	schemaID := fmt.Sprintf("schema:%s:%s", schema.DatabaseName, schemaName)

	resources := []types.Resource{
		{
			ExternalID:  databaseID,
			Name:        schema.DatabaseName,
			Type:        "database",
			Description: schema.DatabaseType,
			HasChildren: true,
		},
		{
			ExternalID:  schemaID,
			Name:        schemaName,
			Type:        "schema",
			Description: schema.DatabaseType,
			ParentID:    databaseID,
			HasChildren: len(schema.Tables) > 0,
		},
	}

	for _, table := range schema.Tables {
		resourceType := normalizedResourceType(table.Type)
		resources = append(resources, types.Resource{
			ExternalID:  fmt.Sprintf("%s:%s.%s", resourceType, schemaName, table.Name),
			Name:        table.Name,
			Type:        resourceType,
			Description: strings.TrimSpace(table.Comment),
			ParentID:    schemaID,
			Metadata: map[string]interface{}{
				"row_estimate": table.RowEstimate,
				"column_count": len(table.Columns),
			},
		})
	}

	return resources
}

func normalizedResourceType(tableType string) string {
	switch strings.ToLower(strings.TrimSpace(tableType)) {
	case "view", "materialized_view":
		return "view"
	default:
		return "table"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

var _ datasource.Connector = (*Adapter)(nil)
