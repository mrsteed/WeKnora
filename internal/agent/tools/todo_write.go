package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// TodoWriteTool implements a planning tool for complex tasks
// This is an optional tool that helps organize multi-step research
type TodoWriteTool struct {
	BaseTool
}

// PlanStep represents a single step in the research plan
type PlanStep struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	ToolsToUse  []string `json:"tools_to_use"`
	Status      string   `json:"status"` // pending, in_progress, completed, skipped
}

// NewTodoWriteTool creates a new todo_write tool instance
func NewTodoWriteTool() *TodoWriteTool {
	description := `Use this tool to create and manage a structured task list for your current coding session. This helps you track progress, organize complex tasks, and demonstrate thoroughness to the user.
It also helps the user understand the progress of the task and overall progress of their requests.

## When to Use This Tool
Use this tool proactively in these scenarios:

1. Complex multi-step tasks - When a task requires 3 or more distinct steps or actions
2. Non-trivial and complex tasks - Tasks that require careful planning or multiple operations
3. User explicitly requests todo list - When the user directly asks you to use the todo list
4. User provides multiple tasks - When users provide a list of things to be done (numbered or comma-separated)
5. After receiving new instructions - Immediately capture user requirements as todos
6. When you start working on a task - Mark it as in_progress BEFORE beginning work. Ideally you should only have one todo as in_progress at a time
7. After completing a task - Mark it as completed and add any new follow-up tasks discovered during implementation

## When NOT to Use This Tool

Skip using this tool when:
1. There is only a single, straightforward task
2. The task is trivial and tracking it provides no organizational benefit
3. The task is purely conversational or informational

NOTE that you should not use this tool if there is only one trivial task to do. In this case you are better off just doing the task directly.

## Examples of When to Use the Todo List

<example>
User: I need to implement these features for my e-commerce site: user registration, product catalog, shopping cart, and checkout flow.
Assistant: I'll help implement these features for your e-commerce site. Let me first think through the architecture and dependencies between these features to create an optimal implementation plan.
*Uses think tool to analyze the e-commerce architecture, identify dependencies between features (user auth needed for cart, catalog needed for checkout), consider database design, security requirements, and implementation order*
Assistant: After analyzing the system architecture and feature dependencies, let me create a strategic todo list that accounts for the interconnected nature of these features.
*Creates a todo list breaking down each feature into specific tasks based on the project architecture and dependencies*
Assistant: Let's start with user registration since it's foundational for the shopping cart and checkout features. This will involve creating database models, API endpoints, and frontend forms that integrate with your existing authentication system.

<reasoning>
The assistant used both think and todo_write tools because:
1. First used think tool to analyze the complex e-commerce architecture and feature dependencies
2. Considered the interconnected nature of features (user auth → cart → checkout)
3. Analyzed security requirements, database design, and optimal implementation order
4. Then created a strategic todo list that accounts for these dependencies
5. This approach ensures features are implemented in the correct order with proper foundations
</reasoning>
</example>

<example>
User: Can you help optimize my React application? It's rendering slowly and has performance issues.</user>
Assistant: I'll help optimize your React application. First, let me examine your codebase to identify potential performance bottlenecks.
*Reviews component structure, render patterns, state management, and data fetching*
Assistant: After analyzing your codebase, I've identified several performance issues. Let me create a todo list to track our optimization efforts.
*Creates todo list with items like: 1) Implement memoization for expensive calculations in ProductList, 2) Add virtualization for long lists in Dashboard, 3) Optimize image loading in Gallery component, 4) Fix state update loops in ShoppingCart, 5) Review bundle size and implement code splitting*
Let's start by implementing memoization for the expensive calculations in your ProductList component.</assistant>

<reasoning>
The assistant used the todo list because:
1. First, the assistant examined the codebase to identify specific performance issues
2. Based on this analysis, it identified multiple optimization opportunities
3. Performance optimization is a non-trivial task requiring multiple steps
4. The todo list helps methodically track improvements across different components
5. This systematic approach ensures all performance bottlenecks are addressed
</reasoning>
</example>

## Examples of When NOT to Use the Todo List

<example>
User: How do I print 'Hello World' in Python?
Assistant: In Python, you can print "Hello World" with this simple code:

print("Hello World")

This will output the text "Hello World" to the console when executed.</assistant>

<reasoning>
The assistant did not use the todo list because this is a single, trivial task that can be completed in one step. There's no need to track multiple tasks or steps for such a straightforward request.
</reasoning>
</example>

<example>
User: What does the git status command do?
Assistant: The git status command shows the current state of your working directory and staging area. It displays which changes have been staged, which haven't, and which files aren't being tracked by Git. This is a fundamental command for understanding the current state of your repository before making commits.

<reasoning>
The assistant did not use the todo list because this is an informational request with no actual coding task to complete. The user is simply asking for an explanation, not for the assistant to perform multiple steps or tasks.
</reasoning>
</example>

## Task States and Management

1. **Task States**: Use these states to track progress:
  - pending: Task not yet started
  - in_progress: Currently working on (limit to ONE task at a time)
  - completed: Task finished successfully

2. **Task Management**:
  - Update task status in real-time as you work
  - Mark tasks complete IMMEDIATELY after finishing (don't batch completions)
  - Only have ONE task in_progress at any time
  - Complete current tasks before starting new ones
  - Remove tasks that are no longer relevant from the list entirely

3. **Task Completion Requirements**:
  - ONLY mark a task as completed when you have FULLY accomplished it
  - If you encounter errors, blockers, or cannot finish, keep the task as in_progress
  - When blocked, create a new task describing what needs to be resolved
  - Never mark a task as completed if:
    - Tests are failing
    - Implementation is partial
    - You encountered unresolved errors
    - You couldn't find necessary files or dependencies

4. **Task Breakdown**:
  - Create specific, actionable items
  - Break complex tasks into smaller, manageable steps
  - Use clear, descriptive task names

When in doubt, use this tool. Being proactive with task management demonstrates attentiveness and ensures you complete all requirements successfully.`

	return &TodoWriteTool{
		BaseTool: NewBaseTool("todo_write", description),
	}
}

// Parameters returns the JSON schema for the tool's parameters
func (t *TodoWriteTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task": map[string]interface{}{
				"type":        "string",
				"description": "The complex task or question you need to create a plan for",
			},
			"steps": map[string]interface{}{
				"type":        "array",
				"description": "Array of research plan steps with status tracking",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "Unique identifier for this step (e.g., 'step1', 'step2')",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "Clear description of what to investigate or accomplish in this step",
						},
						"tools_to_use": map[string]interface{}{
							"type":        "array",
							"description": "Suggested tools for this step (e.g., ['knowledge_search', 'list_knowledge_chunks'])",
							"items": map[string]interface{}{
								"type": "string",
							},
						},
						"status": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"pending", "in_progress", "completed"},
							"description": "Current status: pending (not started), in_progress (executing), completed (finished)",
						},
					},
					"required": []string{"id", "description", "status"},
				},
			},
		},
		"required": []string{"task", "steps"},
	}
}

// Execute executes the todo_write tool
func (t *TodoWriteTool) Execute(ctx context.Context, args map[string]interface{}) (*types.ToolResult, error) {
	task, ok := args["task"].(string)
	if !ok {
		task = "未提供任务描述"
	}

	// Parse plan steps
	var planSteps []PlanStep
	if stepsData, ok := args["steps"].([]interface{}); ok {
		for _, stepData := range stepsData {
			if stepMap, ok := stepData.(map[string]interface{}); ok {
				step := PlanStep{
					ID:          getStringField(stepMap, "id"),
					Description: getStringField(stepMap, "description"),
					ToolsToUse:  getStringArrayField(stepMap, "tools_to_use"),
					Status:      getStringField(stepMap, "status"),
				}
				planSteps = append(planSteps, step)
			}
		}
	}

	// Generate formatted output
	output := generatePlanOutput(task, planSteps)

	// Prepare structured data for response
	stepsJSON, _ := json.Marshal(planSteps)

	return &types.ToolResult{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"task":         task,
			"steps":        planSteps,
			"steps_json":   string(stepsJSON),
			"total_steps":  len(planSteps),
			"plan_created": true,
			"display_type": "plan",
		},
	}, nil
}

// Helper function to safely get string field from map
func getStringField(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// Helper function to safely get string array field from map
func getStringArrayField(m map[string]interface{}, key string) []string {
	if val, ok := m[key].([]interface{}); ok {
		result := make([]string, 0, len(val))
		for _, item := range val {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}
	// Handle legacy string format for backward compatibility
	if val, ok := m[key].(string); ok && val != "" {
		return []string{val}
	}
	return []string{}
}

// generatePlanOutput generates a formatted plan output
func generatePlanOutput(task string, steps []PlanStep) string {
	output := "计划已创建\n\n"
	output += fmt.Sprintf("**任务**: %s\n\n", task)

	if len(steps) == 0 {
		output += "注意：未提供具体步骤。建议创建3-7个结构化步骤以系统化研究。\n\n"
		output += "建议的通用流程：\n"
		output += "1. 使用 knowledge_search 进行初步信息收集\n"
		output += "2. 使用 list_knowledge_chunks 获取关键信息详情\n"
		output += "3. 使用 list_knowledge_chunks 扩展上下文理解\n"
		output += "4. 使用 think 工具评估结果并综合答案\n"
		return output
	}

	output += "**计划步骤**:\n\n"

	// Display all steps in order
	for i, step := range steps {
		output += formatPlanStep(i+1, step)
	}

	output += "\n**执行指南**:\n"
	output += "- 每步执行前标记为 in_progress，完成后标记为 completed\n"
	output += "- 根据搜索结果灵活调整计划，可跳过不必要的步骤\n"
	output += "- 在关键决策点使用 think 工具深入分析\n"
	output += "- 如果某一步骤已获得足够信息，可跳过后续步骤\n\n"
	output += "注意：计划是指导而非硬性要求，保持灵活应对。"

	return output
}

// formatPlanStep formats a single plan step for output
func formatPlanStep(index int, step PlanStep) string {
	statusEmoji := map[string]string{
		"pending":     "⏳",
		"in_progress": "🔄",
		"completed":   "✅",
		"skipped":     "⏭️",
	}

	emoji, ok := statusEmoji[step.Status]
	if !ok {
		emoji = "⏳"
	}

	output := fmt.Sprintf("  %d. %s [%s] %s\n", index, emoji, step.Status, step.Description)

	if len(step.ToolsToUse) > 0 {
		output += fmt.Sprintf("     工具: %s\n", strings.Join(step.ToolsToUse, ", "))
	}

	return output
}
