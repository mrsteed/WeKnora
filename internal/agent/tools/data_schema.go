package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/Tencent/WeKnora/internal/utils"
)

var dataSchemaTool = BaseTool{
	name:        ToolDataSchema,
	description: "Use this tool to get the schema information of a CSV or Excel file loaded into DuckDB. It returns the table name, columns, and row count.",
	schema:      utils.GenerateSchema[DataSchemaInput](),
}

type DataSchemaInput struct {
	KnowledgeID string `json:"knowledge_id" jsonschema:"id of the knowledge to query"`
}

type DataSchemaTool struct {
	BaseTool
	knowledgeService interfaces.KnowledgeService
	chunkRepo        interfaces.ChunkRepository
	targetChunkTypes []types.ChunkType
	// Optional: for fallback dynamic schema loading via DuckDB
	fileService interfaces.FileService
	db          *sql.DB
	sessionID   string
}

func NewDataSchemaTool(
	knowledgeService interfaces.KnowledgeService,
	chunkRepo interfaces.ChunkRepository,
	fileService interfaces.FileService,
	db *sql.DB,
	sessionID string,
	targetChunkTypes ...types.ChunkType,
) *DataSchemaTool {
	if len(targetChunkTypes) == 0 {
		targetChunkTypes = []types.ChunkType{types.ChunkTypeTableSummary, types.ChunkTypeTableColumn}
	}
	return &DataSchemaTool{
		BaseTool:         dataSchemaTool,
		knowledgeService: knowledgeService,
		chunkRepo:        chunkRepo,
		targetChunkTypes: targetChunkTypes,
		fileService:      fileService,
		db:               db,
		sessionID:        sessionID,
	}
}

// Execute executes the tool logic
func (t *DataSchemaTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	var input DataSchemaInput
	if err := json.Unmarshal(args, &input); err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to parse input args: %v", err),
		}, err
	}

	// Get knowledge to get TenantID (use IDOnly to support cross-tenant shared KB)
	knowledge, err := t.knowledgeService.GetKnowledgeByIDOnly(ctx, input.KnowledgeID)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to get knowledge '%s': %v", input.KnowledgeID, err),
		}, err
	}

	// Try to get schema from pre-existing chunks first
	output, err := t.getSchemaFromChunks(ctx, knowledge)
	if err == nil && output != "" {
		return &types.ToolResult{
			Success: true,
			Output:  output,
		}, nil
	}

	// Fallback: dynamically load data into DuckDB and get schema
	if t.db != nil && t.fileService != nil {
		logger.Infof(ctx, "[Tool][DataSchema] No pre-existing schema chunks for knowledge '%s', falling back to DuckDB dynamic loading", input.KnowledgeID)
		return t.getSchemaFromDuckDB(ctx, knowledge)
	}

	return &types.ToolResult{
		Success: false,
		Error:   fmt.Sprintf("No table schema information found for knowledge ID '%s'", input.KnowledgeID),
	}, fmt.Errorf("no schema info found")
}

// getSchemaFromChunks tries to get schema from pre-existing table_summary/table_column chunks
func (t *DataSchemaTool) getSchemaFromChunks(ctx context.Context, knowledge *types.Knowledge) (string, error) {
	chunkTypes := t.targetChunkTypes
	page := &types.Pagination{
		Page:     1,
		PageSize: 100,
	}

	chunks, _, err := t.chunkRepo.ListPagedChunksByKnowledgeID(
		ctx,
		knowledge.TenantID,
		knowledge.ID,
		page,
		chunkTypes,
		"", "", "", "", "",
	)
	if err != nil {
		return "", err
	}

	var summaryContent, columnContent string
	for _, chunk := range chunks {
		if chunk.ChunkType == types.ChunkTypeTableSummary {
			summaryContent = chunk.Content
		} else if chunk.ChunkType == types.ChunkTypeTableColumn {
			columnContent = chunk.Content
		}
	}

	if summaryContent == "" || columnContent == "" {
		return "", fmt.Errorf("no schema chunks found")
	}

	return fmt.Sprintf("%s\n\n%s", summaryContent, columnContent), nil
}

// getSchemaFromDuckDB dynamically loads the knowledge data into DuckDB and returns schema
func (t *DataSchemaTool) getSchemaFromDuckDB(ctx context.Context, knowledge *types.Knowledge) (*types.ToolResult, error) {
	daTool := NewDataAnalysisTool(t.knowledgeService, t.fileService, t.db, t.sessionID)
	// Note: we do NOT defer Cleanup here because the data_analysis tool in the same
	// agent session will reuse the loaded table and clean up at session end.

	tableSchema, err := daTool.LoadFromKnowledge(ctx, knowledge)
	if err != nil {
		logger.Errorf(ctx, "[Tool][DataSchema] DuckDB fallback failed for knowledge '%s': %v", knowledge.ID, err)
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to load and analyze knowledge '%s': %v", knowledge.ID, err),
		}, err
	}

	output := tableSchema.Description()
	logger.Infof(ctx, "[Tool][DataSchema] DuckDB fallback succeeded for knowledge '%s': %d columns, %d rows",
		knowledge.ID, len(tableSchema.Columns), tableSchema.RowCount)

	return &types.ToolResult{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"table_name": tableSchema.TableName,
			"columns":    tableSchema.Columns,
			"row_count":  tableSchema.RowCount,
			"source":     "duckdb_dynamic",
		},
	}, nil
}
