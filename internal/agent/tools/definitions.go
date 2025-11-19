package tools

// AvailableTool defines a simple tool metadata used by settings APIs.
type AvailableTool struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// AvailableToolDefinitions returns the list of tools exposed to the UI.
// Keep this in sync with registered tools in this package.
func AvailableToolDefinitions() []AvailableTool {
	return []AvailableTool{
		{Name: "thinking", Label: "思考", Description: "动态和反思性的问题解决思考工具"},
		{Name: "todo_write", Label: "制定计划", Description: "创建结构化的研究计划"},
		{Name: "knowledge_search", Label: "知识搜索", Description: "在知识库中搜索相关信息"},
		{Name: "list_knowledge_chunks", Label: "查看知识分块", Description: "按 knowledge_id 拉取文档分块列表"},
		{Name: "query_knowledge_graph", Label: "查询知识图谱", Description: "从知识图谱中查询关系"},
		{Name: "get_document_info", Label: "获取文档信息", Description: "查看文档元数据"},
		{Name: "database_query", Label: "查询数据库", Description: "查询数据库中的信息"},
	}
}

// DefaultAllowedTools returns the default allowed tools list.
func DefaultAllowedTools() []string {
	return []string{
		"thinking",
		"todo_write",
		"knowledge_search",
		"list_knowledge_chunks",
		"query_knowledge_graph",
		"get_document_info",
		"database_query",
	}
}
