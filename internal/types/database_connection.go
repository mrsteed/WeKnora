package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strings"

	"github.com/Tencent/WeKnora/internal/utils"
)

const (
	DatabaseTypeMySQL      = "mysql"
	DatabaseTypePostgreSQL = "postgresql"
	DatabaseTypeOracle     = "oracle"
	DatabaseTypeSQLServer  = "sqlserver"
	DatabaseTypeClickHouse = "clickhouse"
)

var ErrDatabaseCredentialsRequireAESKey = errors.New("SYSTEM_AES_KEY is required to store database credentials in production")

// DatabaseConnectionConfig represents the structured configuration for an external database datasource.
// Sensitive fields are kept in the Credentials section so Value/Scan can transparently encrypt/decrypt them.
type DatabaseConnectionConfig struct {
	Type        string                 `json:"type"`
	Credentials DatabaseCredentials    `json:"credentials"`
	Settings    DatabaseSourceSettings `json:"settings"`
}

// DatabaseCredentials holds the authentication data used to connect to an external database.
// Password is encrypted at rest when SYSTEM_AES_KEY is configured.
type DatabaseCredentials struct {
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
}

// DatabaseSourceSettings holds non-secret connection settings and query guard defaults.
type DatabaseSourceSettings struct {
	Host              string   `json:"host"`
	Port              int      `json:"port"`
	Database          string   `json:"database"`
	Schema            string   `json:"schema,omitempty"`
	SSLMode           string   `json:"ssl_mode,omitempty"`
	TableAllowlist    []string `json:"table_allowlist,omitempty"`
	ColumnDenylist    []string `json:"column_denylist,omitempty"`
	MaxRows           int      `json:"max_rows,omitempty"`
	QueryTimeoutSec   int      `json:"query_timeout_sec,omitempty"`
	SampleRows        int      `json:"sample_rows,omitempty"`
	SchemaRefreshCron string   `json:"schema_refresh_cron,omitempty"`
}

// DatabaseKnowledgeBaseConfig captures the database connector payload required
// to bootstrap a database knowledge base during the initial create request.
// The KnowledgeBase record only persists the KB metadata itself; this struct is
// used by the handler layer to create the linked datasource and initialize the
// first schema snapshot in the same request flow.
type DatabaseKnowledgeBaseConfig struct {
	DataSourceName string                   `json:"data_source_name,omitempty"`
	Connection     DatabaseConnectionConfig `json:"connection"`
}

// Value implements the driver.Valuer interface.
// It encrypts the database password before the configuration is persisted.
// In production-like environments, a plaintext password is rejected when SYSTEM_AES_KEY is missing.
func (c DatabaseConnectionConfig) Value() (driver.Value, error) {
	if err := c.ValidateForStorage(); err != nil {
		return nil, err
	}

	if key := utils.GetAESKey(); key != nil {
		if c.Credentials.Password != "" {
			if encrypted, err := utils.EncryptAESGCM(c.Credentials.Password, key); err == nil {
				c.Credentials.Password = encrypted
			} else {
				return nil, err
			}
		}
	} else if c.Credentials.Password != "" {
		log.Printf("warning: SYSTEM_AES_KEY is not set; database password for datasource type=%s will be stored in plaintext", c.Type)
	}

	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface.
// It decrypts the stored database password after loading the configuration from the database.
func (c *DatabaseConnectionConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return nil
	}

	if len(bytes) == 0 {
		return nil
	}
	if err := json.Unmarshal(bytes, c); err != nil {
		return err
	}

	if key := utils.GetAESKey(); key != nil && c.Credentials.Password != "" {
		decrypted, err := utils.DecryptAESGCM(c.Credentials.Password, key)
		if err != nil {
			return err
		}
		c.Credentials.Password = decrypted
	}
	return nil
}

// ValidateForStorage checks whether the configuration can be safely persisted.
// A configured password requires SYSTEM_AES_KEY in production-like environments.
func (c DatabaseConnectionConfig) ValidateForStorage() error {
	if c.Credentials.Password == "" {
		return nil
	}
	if utils.GetAESKey() != nil {
		return nil
	}
	if isStrictDatabaseCredentialEnv() {
		return ErrDatabaseCredentialsRequireAESKey
	}
	return nil
}

// MaskSensitiveFields returns a copy with the password hidden for API responses or logs.
func (c DatabaseConnectionConfig) MaskSensitiveFields() DatabaseConnectionConfig {
	masked := c
	if masked.Credentials.Password != "" {
		masked.Credentials.Password = "***"
	}
	return masked
}

// ToJSON converts the structured config into the JSON payload stored by data_sources.config.
func (c *DatabaseConnectionConfig) ToJSON() (JSON, error) {
	if c == nil {
		return nil, nil
	}
	raw, err := c.Value()
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	bytes, ok := raw.([]byte)
	if !ok {
		return nil, errors.New("database connection config value is not []byte")
	}
	return JSON(bytes), nil
}

// ParseDatabaseConnectionConfig decodes a database datasource configuration from the raw JSON payload.
func ParseDatabaseConnectionConfig(raw JSON) (*DatabaseConnectionConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var cfg DatabaseConnectionConfig
	if err := cfg.Scan([]byte(raw)); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ParseDatabaseConnectionConfig decodes the DataSource.Config field into a typed database config.
func (d *DataSource) ParseDatabaseConnectionConfig() (*DatabaseConnectionConfig, error) {
	if d == nil {
		return nil, nil
	}
	return ParseDatabaseConnectionConfig(d.Config)
}

// SetDatabaseConnectionConfig serializes a typed database config into DataSource.Config.
func (d *DataSource) SetDatabaseConnectionConfig(cfg *DatabaseConnectionConfig) error {
	if d == nil {
		return nil
	}
	raw, err := cfg.ToJSON()
	if err != nil {
		return err
	}
	d.Config = raw
	return nil
}

func isStrictDatabaseCredentialEnv() bool {
	for _, value := range []string{os.Getenv("APP_ENV"), os.Getenv("ENV"), os.Getenv("GIN_MODE")} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "prod", "production", "release":
			return true
		}
	}
	return false
}
