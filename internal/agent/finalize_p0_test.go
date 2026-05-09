package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleMaxIterations_MarksStateAsPartial(t *testing.T) {
	mock := &mockChat{
		responses: []mockResponse{
			{chunks: []types.StreamResponse{{Content: "assembled answer", Done: true}}},
		},
	}
	engine := newTestEngine(t, mock)
	state := &types.AgentState{
		RoundSteps: []types.AgentStep{{Iteration: 0}},
	}

	engine.handleMaxIterations(context.Background(), "test query", state, "sess-1")

	assert.True(t, state.IsComplete)
	assert.Equal(t, "partial", state.CompletionStatus)
	assert.Equal(t, "max_iterations", state.FinishReason)
	assert.Equal(t, "max_iterations", state.FailureReason)
	assert.False(t, state.AllowIndexing)
	assert.False(t, state.AllowComplete)
	assert.Equal(t, "assembled answer", state.FinalAnswer)
}

func TestStreamFinalAnswerToEventBus_UsesStateCompletionMetadata(t *testing.T) {
	mock := &mockChat{
		responses: []mockResponse{{chunks: []types.StreamResponse{{Content: "partial answer", Done: true}}}},
	}
	engine := newTestEngine(t, mock)
	state := &types.AgentState{
		CompletionStatus: "partial",
		FinishReason:     "max_iterations",
		FailureReason:    "max_iterations",
		AllowIndexing:    false,
		AllowComplete:    false,
		RoundSteps:       []types.AgentStep{{Iteration: 0}},
	}

	var emitted []event.AgentFinalAnswerData
	engine.eventBus.On(event.EventAgentFinalAnswer, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		require.True(t, ok)
		emitted = append(emitted, data)
		return nil
	})

	err := engine.streamFinalAnswerToEventBus(context.Background(), "test query", state, "sess-1")
	require.NoError(t, err)
	require.NotEmpty(t, emitted)
	assert.Equal(t, "partial", emitted[0].CompletionStatus)
	assert.Equal(t, "max_iterations", emitted[0].FinishReason)
	assert.False(t, emitted[0].AllowIndexing)
	assert.False(t, emitted[0].AllowComplete)
	assert.Equal(t, "max_iterations", emitted[0].FailureReason)
	assert.True(t, emitted[0].Done)
}

func TestEmitCompletionEvent_NormalizesRecoveredToolErrorState(t *testing.T) {
	engine := newTestEngine(t, &mockChat{})
	state := &types.AgentState{
		FinalAnswer:            "recovered answer",
		FinalAnswerSynthesized: true,
		CompletionStatus:       types.MessageCompletionStatusFailed,
		FinishReason:           "tool_error",
		FailureReason:          "tool_error",
		AllowIndexing:          false,
		AllowComplete:          false,
		RoundSteps:             []types.AgentStep{{Iteration: 0}},
	}

	var emitted []event.AgentCompleteData
	engine.eventBus.On(event.EventAgentComplete, func(ctx context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		emitted = append(emitted, data)
		return nil
	})

	engine.emitCompletionEvent(context.Background(), state, "sess-1", "msg-1", time.Now().Add(-time.Second))

	require.Len(t, emitted, 1)
	assert.Equal(t, types.MessageCompletionStatusPartial, emitted[0].CompletionStatus)
	assert.Equal(t, "fallback_stop", emitted[0].FinishReason)
	assert.Empty(t, emitted[0].FailureReason)
	assert.True(t, emitted[0].IsPartial)
	assert.False(t, emitted[0].AllowIndexing)
	assert.False(t, emitted[0].AllowComplete)
	assert.Equal(t, "recovered answer", emitted[0].FinalAnswer)
}

func TestStreamFinalAnswerToEventBus_CompressesLargeToolOutputsInSynthesisContext(t *testing.T) {
	mock := &mockChat{
		responses: []mockResponse{{chunks: []types.StreamResponse{{Content: "assembled answer", Done: true}}}},
	}
	engine := newTestEngine(t, mock)
	state := &types.AgentState{
		RoundSteps: []types.AgentStep{{
			Iteration: 0,
			ToolCalls: []types.ToolCall{{
				Name:   "external_database_query",
				Result: &types.ToolResult{Output: strings.Repeat("very long tool output ", 300)},
			}},
		}},
	}

	err := engine.streamFinalAnswerToEventBus(context.Background(), "test query", state, "sess-1")
	require.NoError(t, err)
	require.NotEmpty(t, mock.lastMessages)

	var toolSummary string
	for _, msg := range mock.lastMessages {
		if msg.Role == "user" && strings.Contains(msg.Content, "Tool external_database_query summary:") {
			toolSummary = msg.Content
			break
		}
	}
	require.NotEmpty(t, toolSummary)
	assert.Contains(t, toolSummary, "truncated for synthesis")
	assert.Less(t, len(toolSummary), len(strings.Repeat("very long tool output ", 300)))
}

func TestStreamFinalAnswerToEventBus_DisablesThinkingForFinalSynthesis(t *testing.T) {
	mock := &mockChat{
		responses: []mockResponse{{chunks: []types.StreamResponse{
			{Content: "part-1 ", Done: false},
			{Content: "part-2", Done: true},
		}}},
	}
	engine := newTestEngine(t, mock, withThinking(true))
	state := &types.AgentState{
		RoundSteps: []types.AgentStep{{Iteration: 0}},
	}

	err := engine.streamFinalAnswerToEventBus(context.Background(), "test query", state, "sess-1")
	require.NoError(t, err)
	require.NotNil(t, mock.lastOptions)
	require.NotNil(t, mock.lastOptions.Thinking)
	assert.False(t, *mock.lastOptions.Thinking)
	assert.Equal(t, "part-1 part-2", state.FinalAnswer)
}

func TestFinalizeSummarizesBudgetedDatabaseQueryData(t *testing.T) {
	longCell := strings.Repeat("detail-", 40)
	summary := summarizeStructuredToolResult(agenttools.ToolExternalDatabaseQuery, &types.ToolResult{
		Data: map[string]interface{}{
			"columns":              []string{"id", "note"},
			"rows":                 []map[string]interface{}{{"id": 1, "note": longCell}},
			"row_count":            5,
			"truncated":            true,
			"output_truncated":     true,
			"cell_truncated_count": 1,
			"duration_ms":          int64(48),
			"executed_sql":         "SELECT id, note FROM orders LIMIT 30",
		},
	})

	assert.Contains(t, summary, "Database query summary")
	assert.Contains(t, summary, "Columns: id, note")
	assert.Contains(t, summary, "Row count: 5")
	assert.Contains(t, summary, "Result truncated: true")
	assert.Contains(t, summary, "Output truncated: true")
	assert.Contains(t, summary, "Cells truncated: 1")
	assert.Contains(t, summary, "Duration: 48 ms")
	assert.Contains(t, summary, "Executed SQL: SELECT id, note FROM orders LIMIT 30")
	assert.Contains(t, summary, "Sample rows:")
	assert.Contains(t, summary, "truncated for synthesis")
	assert.NotContains(t, summary, longCell)
}

func TestFinalizeSummarizesBudgetedDatabaseSchemaData(t *testing.T) {
	summary := summarizeStructuredToolResult(agenttools.ToolExternalDatabaseSchema, &types.ToolResult{
		Data: map[string]interface{}{
			"database_name":              "crm",
			"schema_name":                "public",
			"schema_hash":                "hash-1",
			"refreshed_at":               "2026-05-06T12:00:00Z",
			"mode":                       "catalog",
			"table_count":                21,
			"display_table_count":        20,
			"scope_table_count":          21,
			"matched_table_count":        21,
			"additional_tables_omitted":  5,
			"allowed_tables":             []string{"orders", "customers", "shipments"},
			"foreign_keys":               []string{"orders.customer_id -> customers.id"},
			"possible_join_hints":        []string{"shipments.order_id = orders.id"},
			"additional_columns_omitted": 48,
		},
	})

	assert.Contains(t, summary, "Database schema summary")
	assert.Contains(t, summary, "Database: crm")
	assert.Contains(t, summary, "Schema: public")
	assert.Contains(t, summary, "Schema hash: hash-1")
	assert.Contains(t, summary, "Refreshed at: 2026-05-06T12:00:00Z")
	assert.Contains(t, summary, "Mode: catalog")
	assert.Contains(t, summary, "Table count: 21")
	assert.Contains(t, summary, "Current view table count: 20")
	assert.Contains(t, summary, "Scope table count: 21")
	assert.Contains(t, summary, "Matched table count: 21")
	assert.Contains(t, summary, "Additional tables omitted from current view: 5")
	assert.Contains(t, summary, "Table preview: customers, orders, shipments")
	assert.Contains(t, summary, "Foreign keys:")
	assert.Contains(t, summary, "orders.customer_id -> customers.id")
	assert.Contains(t, summary, "Possible join hints:")
	assert.Contains(t, summary, "shipments.order_id = orders.id")
	assert.Contains(t, summary, "Retrieval hint:")
}

func TestSummarizeStructuredToolResultForExternalDatabaseSchemaListOnlyAndFilters(t *testing.T) {
	summary := summarizeStructuredToolResult(agenttools.ToolExternalDatabaseSchema, &types.ToolResult{
		Data: map[string]interface{}{
			"database_name":       "crm",
			"schema_name":         "public",
			"mode":                "detail",
			"table_count":         1,
			"scope_table_count":   82,
			"matched_table_count": 1,
			"allowed_tables":      []string{"table_01", "table_02", "emergency_plan_calls"},
			"matched_tables":      []string{"emergency_plan_calls"},
			"table_name_like":     "plan",
			"list_only":           true,
		},
	})

	assert.Contains(t, summary, "Database schema summary")
	assert.Contains(t, summary, "Scope table count: 82")
	assert.Contains(t, summary, "Matched table count: 1")
	assert.Contains(t, summary, "Table name filter: plan")
	assert.Contains(t, summary, "Table preview: emergency_plan_calls")
	assert.Contains(t, summary, "list_only mode")
	assert.NotContains(t, summary, "table_01, table_02")
}

func TestSummarizeStructuredToolResultForExternalDatabaseSearchTables(t *testing.T) {
	summary := summarizeStructuredToolResult(agenttools.ToolExternalDatabaseSearchTables, &types.ToolResult{
		Data: map[string]interface{}{
			"database_name":              "crm",
			"schema_name":                "public",
			"scope_table_count":          82,
			"matched_table_count":        4,
			"returned_hit_count":         2,
			"additional_matches_omitted": 2,
			"keyword":                    "预案",
			"matched_tables":             []string{"emergency_plan_calls", "emergency_plan_links", "emergency_plan_logs", "emergency_plans"},
			"results": []map[string]interface{}{
				{"table_name": "emergency_plan_calls", "likely_role": "fact_log", "matched_columns": []string{"plan_id", "call_count"}},
			},
		},
	})

	assert.Contains(t, summary, "Database table search summary")
	assert.Contains(t, summary, "Scope table count: 82")
	assert.Contains(t, summary, "Matched table count: 4")
	assert.Contains(t, summary, "Returned hit count: 2")
	assert.Contains(t, summary, "Keyword filter: 预案")
	assert.Contains(t, summary, "Candidate tables: emergency_plan_calls, emergency_plan_links, emergency_plan_logs, emergency_plans")
	assert.Contains(t, summary, "Additional candidate tables omitted: 2")
	assert.Contains(t, summary, "Top matches:")
	assert.Contains(t, summary, "emergency_plan_calls [fact_log]")
	assert.Contains(t, summary, "Retrieval hint:")
}
