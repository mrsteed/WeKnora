package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/databaseconnector"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type structuredQueryService struct {
	schemaRegistry interfaces.SchemaRegistryService
	dsRepo         interfaces.DataSourceRepository
	auditRepo      interfaces.DatabaseQueryAuditRepository
	dbRegistry     *databaseconnector.Registry
	guard          *sqlGuard
}

// NewStructuredQueryService creates the realtime query service used by later
// external database tools. It depends on schema snapshots for permission scope,
// datasource configs for connection policy and audit storage for traceability.
func NewStructuredQueryService(
	schemaRegistry interfaces.SchemaRegistryService,
	dsRepo interfaces.DataSourceRepository,
	auditRepo interfaces.DatabaseQueryAuditRepository,
	dbRegistry *databaseconnector.Registry,
) interfaces.StructuredQueryService {
	return &structuredQueryService{
		schemaRegistry: schemaRegistry,
		dsRepo:         dsRepo,
		auditRepo:      auditRepo,
		dbRegistry:     dbRegistry,
		guard:          newSQLGuard(),
	}
}

func (s *structuredQueryService) ValidateSQL(ctx context.Context, req types.ValidateSQLRequest) (*types.ValidatedSQL, error) {
	target, err := s.resolveQueryTarget(ctx, req.KnowledgeBaseID)
	if err != nil {
		return nil, err
	}
	return s.guard.Validate(req.SQL, target.schema, target.config, target.connector.Dialect(), req.MaxRows, req.TimeoutSeconds)
}

func (s *structuredQueryService) ExecuteQuery(ctx context.Context, req types.ExecuteQueryRequest) (*types.QueryResult, error) {
	startTime := time.Now()
	target, err := s.resolveQueryTarget(ctx, req.KnowledgeBaseID)
	if err != nil {
		return nil, err
	}

	validated, err := s.guard.Validate(req.SQL, target.schema, target.config, target.connector.Dialect(), req.MaxRows, req.TimeoutSeconds)
	if err != nil {
		return nil, s.finishAudit(ctx, target.dataSource.ID, req, validated, startTime, types.DatabaseQueryAuditStatusRejected, 0, err)
	}

	userID := req.UserID
	if userID == "" {
		if contextUserID, ok := types.UserIDFromContext(ctx); ok {
			userID = contextUserID
		}
	}
	if userID == "" {
		return nil, s.finishAudit(ctx, target.dataSource.ID, req, validated, startTime, types.DatabaseQueryAuditStatusRejected, 0, ErrStructuredQueryUserRequired)
	}
	if req.UserID == "" {
		req.UserID = userID
	}

	queryCtx, cancel := context.WithTimeout(ctx, validated.Timeout)
	defer cancel()

	rows, err := target.connector.Query(queryCtx, target.config, validated.ExecutedSQL, validated.Timeout)
	if err != nil {
		return nil, s.finishAudit(ctx, target.dataSource.ID, req, validated, startTime, types.DatabaseQueryAuditStatusFailed, 0, err)
	}
	defer rows.Close()

	result, err := scanExternalQueryRows(rows, validated.MaxRows)
	if err != nil {
		return nil, s.finishAudit(ctx, target.dataSource.ID, req, validated, startTime, types.DatabaseQueryAuditStatusFailed, 0, err)
	}
	result.ExecutedSQL = validated.ExecutedSQL
	result.DurationMS = time.Since(startTime).Milliseconds()
	result.DisplayType = "external_database_query"

	if err := s.writeAuditLog(ctx, &types.DatabaseQueryAuditLog{
		TenantID:        target.schema.TenantID,
		UserID:          req.UserID,
		SessionID:       req.SessionID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		DataSourceID:    target.dataSource.ID,
		OriginalSQL:     req.SQL,
		ExecutedSQL:     validated.ExecutedSQL,
		Purpose:         req.Purpose,
		Status:          types.DatabaseQueryAuditStatusSuccess,
		RowCount:        result.RowCount,
		DurationMS:      result.DurationMS,
	}); err != nil {
		return nil, err
	}

	logger.Infof(ctx, "external database query executed: kb=%s datasource=%s rows=%d truncated=%t", req.KnowledgeBaseID, target.dataSource.ID, result.RowCount, result.Truncated)
	return result, nil
}

func (s *structuredQueryService) ExplainQuery(ctx context.Context, req types.ExplainQueryRequest) (*types.QueryPlan, error) {
	validated, err := s.ValidateSQL(ctx, types.ValidateSQLRequest{
		TenantID:        req.TenantID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		SQL:             req.SQL,
		MaxRows:         req.MaxRows,
		TimeoutSeconds:  req.TimeoutSeconds,
	})
	if err != nil {
		return nil, err
	}
	return &types.QueryPlan{
		SQL:            validated.ExecutedSQL,
		Dialect:        validated.Dialect,
		Tables:         validated.Tables,
		MaxRows:        validated.MaxRows,
		TimeoutSeconds: int(validated.Timeout / time.Second),
		Notes: []string{
			"MVP explain only validates the query envelope and does not run EXPLAIN remotely.",
		},
	}, nil
}

type queryExecutionTarget struct {
	schema     *types.DatabaseSchema
	dataSource *types.DataSource
	config     *types.DatabaseConnectionConfig
	connector  databaseconnector.DatabaseConnector
}

func (s *structuredQueryService) resolveQueryTarget(ctx context.Context, kbID string) (*queryExecutionTarget, error) {
	if kbID == "" {
		return nil, ErrStructuredQueryKnowledgeBaseMissing
	}
	schema, err := s.schemaRegistry.GetDatabaseSchema(ctx, kbID)
	if err != nil {
		return nil, err
	}
	dataSources, err := s.dsRepo.FindByKnowledgeBase(ctx, kbID)
	if err != nil {
		return nil, err
	}
	var matched []*types.DataSource
	for _, ds := range dataSources {
		if ds == nil || !isDatabaseDataSourceType(ds.Type) {
			continue
		}
		if schema.TenantID != 0 && ds.TenantID != schema.TenantID {
			continue
		}
		if schema.DataSourceID != "" && ds.ID != schema.DataSourceID {
			continue
		}
		matched = append(matched, ds)
	}
	switch len(matched) {
	case 0:
		return nil, ErrDatabaseDataSourceNotFound
	case 1:
	default:
		return nil, ErrMultipleDatabaseDataSources
	}
	config, err := matched[0].ParseDatabaseConnectionConfig()
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, fmt.Errorf("database datasource config is empty")
	}
	connector, err := s.dbRegistry.Get(matched[0].Type)
	if err != nil {
		return nil, err
	}
	return &queryExecutionTarget{
		schema:     schema,
		dataSource: matched[0],
		config:     config,
		connector:  connector,
	}, nil
}

func scanExternalQueryRows(rows *sql.Rows, maxRows int) (*types.QueryResult, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}

	result := &types.QueryResult{
		Columns: make([]types.QueryColumn, 0, len(columns)),
		Rows:    make([]map[string]any, 0),
	}
	for index, column := range columns {
		dataType := ""
		if index < len(columnTypes) {
			dataType = columnTypes[index].DatabaseTypeName()
		}
		result.Columns = append(result.Columns, types.QueryColumn{Name: column, DataType: dataType})
	}

	for rows.Next() {
		if maxRows > 0 && len(result.Rows) >= maxRows {
			result.Truncated = true
			break
		}
		rowValues := make([]any, len(columns))
		rowPointers := make([]any, len(columns))
		for index := range rowValues {
			rowPointers[index] = &rowValues[index]
		}
		if err := rows.Scan(rowPointers...); err != nil {
			return nil, err
		}
		rowMap := make(map[string]any, len(columns))
		for index, column := range columns {
			value := rowValues[index]
			if bytes, ok := value.([]byte); ok {
				rowMap[column] = string(bytes)
			} else {
				rowMap[column] = value
			}
		}
		result.Rows = append(result.Rows, rowMap)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result.RowCount = len(result.Rows)
	return result, nil
}

func (s *structuredQueryService) finishAudit(
	ctx context.Context,
	dataSourceID string,
	req types.ExecuteQueryRequest,
	validated *types.ValidatedSQL,
	startTime time.Time,
	status string,
	rowCount int,
	originalErr error,
) error {
	logEntry := &types.DatabaseQueryAuditLog{
		TenantID:        coalesceTenantID(req.TenantID, ctx),
		UserID:          req.UserID,
		SessionID:       req.SessionID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		DataSourceID:    dataSourceID,
		OriginalSQL:     req.SQL,
		Purpose:         req.Purpose,
		Status:          status,
		RowCount:        rowCount,
		DurationMS:      time.Since(startTime).Milliseconds(),
	}
	if validated != nil {
		logEntry.ExecutedSQL = validated.ExecutedSQL
	}
	if originalErr != nil {
		logEntry.ErrorMessage = originalErr.Error()
	}
	if logEntry.UserID == "" {
		if userID, ok := types.UserIDFromContext(ctx); ok {
			logEntry.UserID = userID
		}
	}
	if logEntry.TenantID == 0 && validated != nil {
		logEntry.TenantID = coalesceTenantID(req.TenantID, ctx)
	}
	if err := s.writeAuditLog(ctx, logEntry); err != nil {
		return err
	}
	return originalErr
}

func (s *structuredQueryService) writeAuditLog(ctx context.Context, entry *types.DatabaseQueryAuditLog) error {
	if entry == nil || entry.DataSourceID == "" {
		return nil
	}
	if entry.TenantID == 0 {
		if tenantID, ok := types.TenantIDFromContext(ctx); ok {
			entry.TenantID = tenantID
		}
	}
	if entry.UserID == "" {
		if userID, ok := types.UserIDFromContext(ctx); ok {
			entry.UserID = userID
		}
	}
	return s.auditRepo.Create(ctx, entry)
}

func coalesceTenantID(requestTenantID uint64, ctx context.Context) uint64 {
	if requestTenantID != 0 {
		return requestTenantID
	}
	if tenantID, ok := types.TenantIDFromContext(ctx); ok {
		return tenantID
	}
	return 0
}
