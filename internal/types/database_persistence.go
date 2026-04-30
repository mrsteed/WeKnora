package types

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	DatabaseQueryAuditStatusSuccess  = "success"
	DatabaseQueryAuditStatusFailed   = "failed"
	DatabaseQueryAuditStatusRejected = "rejected"
)

// DatabaseSchemaSnapshot persists one effective schema refresh result for a
// database-backed knowledge base. The full logical schema is stored in
// SchemaJSON so later services can reconstruct prompt-friendly metadata without
// hitting the external database again.
type DatabaseSchemaSnapshot struct {
	ID              string         `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID        uint64         `json:"tenant_id" gorm:"index"`
	KnowledgeBaseID string         `json:"knowledge_base_id" gorm:"index"`
	DataSourceID    string         `json:"data_source_id" gorm:"index"`
	DatabaseType    string         `json:"database_type" gorm:"type:varchar(32);index"`
	DatabaseName    string         `json:"database_name" gorm:"type:varchar(255)"`
	SchemaName      string         `json:"schema_name,omitempty" gorm:"type:varchar(255)"`
	SchemaHash      string         `json:"schema_hash" gorm:"type:varchar(128);index"`
	SchemaJSON      JSON           `json:"schema_json" gorm:"column:schema_json;type:jsonb"`
	RefreshedAt     time.Time      `json:"refreshed_at" gorm:"index"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

func (DatabaseSchemaSnapshot) TableName() string {
	return "database_schema_snapshots"
}

func (s *DatabaseSchemaSnapshot) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	if s.RefreshedAt.IsZero() {
		s.RefreshedAt = time.Now().UTC()
	}
	return nil
}

// SetSchema serializes the in-memory schema DTO into the persisted snapshot
// payload. The helper keeps later services from duplicating JSON marshaling.
func (s *DatabaseSchemaSnapshot) SetSchema(schema *DatabaseSchema) error {
	if schema == nil {
		s.SchemaJSON = nil
		return nil
	}
	b, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	s.SchemaJSON = JSON(b)
	return nil
}

// ParseSchema reconstructs the full schema DTO from the persisted JSON payload.
func (s *DatabaseSchemaSnapshot) ParseSchema() (*DatabaseSchema, error) {
	if s == nil || len(s.SchemaJSON) == 0 {
		return nil, nil
	}
	var schema DatabaseSchema
	if err := json.Unmarshal(s.SchemaJSON, &schema); err != nil {
		return nil, err
	}
	return &schema, nil
}

// DatabaseTableColumn persists flattened per-column metadata so later services
// can efficiently query visible columns without reparsing the whole snapshot.
type DatabaseTableColumn struct {
	ID              string         `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID        uint64         `json:"tenant_id" gorm:"index"`
	KnowledgeBaseID string         `json:"knowledge_base_id" gorm:"index"`
	DataSourceID    string         `json:"data_source_id" gorm:"index"`
	Table           string         `json:"table_name" gorm:"column:table_name;type:varchar(255);index"`
	ColumnName      string         `json:"column_name" gorm:"type:varchar(255);index"`
	DataType        string         `json:"data_type" gorm:"type:varchar(128)"`
	Nullable        bool           `json:"nullable" gorm:"default:true"`
	Comment         string         `json:"comment,omitempty"`
	IsSensitive     bool           `json:"is_sensitive" gorm:"default:false"`
	OrdinalPosition int            `json:"ordinal_position,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

func (DatabaseTableColumn) TableName() string {
	return "database_table_columns"
}

func (c *DatabaseTableColumn) BeforeCreate(tx *gorm.DB) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	return nil
}

// DatabaseQueryAuditLog records every external database query execution or
// rejection for tenant-scoped auditability.
type DatabaseQueryAuditLog struct {
	ID              string    `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID        uint64    `json:"tenant_id" gorm:"index"`
	UserID          string    `json:"user_id" gorm:"type:varchar(36);index"`
	SessionID       string    `json:"session_id,omitempty" gorm:"type:varchar(36);index"`
	KnowledgeBaseID string    `json:"knowledge_base_id" gorm:"type:varchar(36);index"`
	DataSourceID    string    `json:"data_source_id" gorm:"type:varchar(36);index"`
	OriginalSQL     string    `json:"original_sql"`
	ExecutedSQL     string    `json:"executed_sql,omitempty"`
	Purpose         string    `json:"purpose,omitempty"`
	Status          string    `json:"status" gorm:"type:varchar(32);index"`
	RowCount        int       `json:"row_count"`
	DurationMS      int64     `json:"duration_ms"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	CreatedAt       time.Time `json:"created_at" gorm:"index"`
}

func (DatabaseQueryAuditLog) TableName() string {
	return "database_query_audit_logs"
}

func (l *DatabaseQueryAuditLog) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	if l.CreatedAt.IsZero() {
		l.CreatedAt = time.Now().UTC()
	}
	return nil
}
