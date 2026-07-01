package tools

import (
	"context"
	"encoding/json"

	"github.com/Tencent/WeKnora/internal/types"
)

var finalAnswerTool = BaseTool{
	name:        ToolFinalAnswer,
	description: "Use this tool to submit the final answer to the user. Provide the complete answer text in the answer field.",
	schema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"answer": {
				"type": "string",
				"description": "The complete final answer to send to the user."
			}
		},
		"required": ["answer"],
		"additionalProperties": false
	}`),
}

type FinalAnswerTool struct {
	BaseTool
}

func NewFinalAnswerTool() *FinalAnswerTool {
	return &FinalAnswerTool{BaseTool: finalAnswerTool}
}

func (t *FinalAnswerTool) Execute(_ context.Context, args json.RawMessage) (*types.ToolResult, error) {
	var payload struct {
		Answer string `json:"answer"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return &types.ToolResult{Success: false, Error: err.Error()}, nil
	}
	return &types.ToolResult{Success: true, Output: payload.Answer}, nil
}
