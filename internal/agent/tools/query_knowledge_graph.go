package tools

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// QueryKnowledgeGraphTool queries the knowledge graph for entities and relationships
type QueryKnowledgeGraphTool struct {
	BaseTool
	knowledgeService interfaces.KnowledgeBaseService
}

// NewQueryKnowledgeGraphTool creates a new query knowledge graph tool
func NewQueryKnowledgeGraphTool(knowledgeService interfaces.KnowledgeBaseService) *QueryKnowledgeGraphTool {
	description := `查询知识图谱，探索实体关系和知识网络。

## 何时使用

**适用场景**:
- ✅ 需要了解实体之间的关系（如"Docker和Kubernetes的关系"）
- ✅ 探索知识网络和概念关联
- ✅ 查找特定实体的相关信息
- ✅ 理解技术架构和系统关系

**不适用**:
- ❌ 普通文本搜索（用 knowledge_search 更合适）
- ❌ 知识库未配置图谱抽取
- ❌ 需要精确的文档内容（用 knowledge_search）

## 参数说明

**knowledge_base_ids** (required): 要查询的知识库ID数组（1-10个）
- 只有配置了图谱抽取的知识库才会有效
- 支持批量并发查询多个知识库
- 示例: ["kb_tech", "kb_arch"]

**query** (required): 查询内容
- 可以是实体名称（如"Docker"）
- 可以是关系查询（如"容器编排"）
- 可以是概念搜索（如"微服务架构"）

## 图谱配置

知识图谱需要在知识库中预先配置：
- **实体类型**（Nodes）：如"技术"、"工具"、"概念"
- **关系类型**（Relations）：如"依赖"、"使用"、"包含"

如果知识库未配置图谱，工具会提示并返回普通搜索结果。

## 配合使用

1. **关系探索**: query_knowledge_graph → get_chunk_detail（查看详细内容）
2. **网络分析**: query_knowledge_graph → list_knowledge_chunks（扩展上下文）
3. **主题研究**: knowledge_search → query_knowledge_graph（深入实体关系）

## 当前状态

⚠️ **注意**: 完整的图数据库集成正在开发中。当前版本：
- ✓ 支持图谱配置查询
- ✓ 返回图谱相关的文档片段
- ✓ 显示实体和关系配置信息
- ⏳ 完整的图查询语言（Cypher）支持开发中
- ⏳ 可视化图数据结构开发中

## Tips

- 结果会标注图谱配置状态
- 返回的Data字段包含结构化图信息供前端展示
- 跨知识库结果自动去重
- 按相关度排序`

	return &QueryKnowledgeGraphTool{
		BaseTool:         NewBaseTool("query_knowledge_graph", description),
		knowledgeService: knowledgeService,
	}
}

// Parameters returns the JSON schema for the tool's parameters
func (t *QueryKnowledgeGraphTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"knowledge_base_ids": map[string]interface{}{
				"type":        "array",
				"description": "Array of knowledge base IDs to query",
				"items": map[string]interface{}{
					"type": "string",
				},
				"minItems": 1,
				"maxItems": 10,
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "查询内容（实体名称或查询文本）",
			},
		},
		"required": []string{"knowledge_base_ids", "query"},
	}
}

// Execute queries the knowledge graph with concurrent KB processing
func (t *QueryKnowledgeGraphTool) Execute(ctx context.Context, args map[string]interface{}) (*types.ToolResult, error) {
	// Extract knowledge_base_ids array
	kbIDsRaw, ok := args["knowledge_base_ids"].([]interface{})
	if !ok || len(kbIDsRaw) == 0 {
		return &types.ToolResult{
			Success: false,
			Error:   "knowledge_base_ids is required and must be a non-empty array",
		}, fmt.Errorf("knowledge_base_ids is required")
	}

	// Convert to string slice
	var kbIDs []string
	for _, id := range kbIDsRaw {
		if idStr, ok := id.(string); ok && idStr != "" {
			kbIDs = append(kbIDs, idStr)
		}
	}

	if len(kbIDs) == 0 {
		return &types.ToolResult{
			Success: false,
			Error:   "knowledge_base_ids must contain at least one valid KB ID",
		}, fmt.Errorf("no valid KB IDs provided")
	}

	query, ok := args["query"].(string)
	if !ok || query == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "query is required",
		}, fmt.Errorf("invalid query")
	}

	// Concurrently query all knowledge bases
	type graphQueryResult struct {
		kbID    string
		kb      *types.KnowledgeBase
		results []*types.SearchResult
		err     error
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	kbResults := make(map[string]*graphQueryResult)

	searchParams := types.SearchParams{
		QueryText:  query,
		MatchCount: 10,
	}

	for _, kbID := range kbIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()

			// Get knowledge base to check graph configuration
			kb, err := t.knowledgeService.GetKnowledgeBaseByID(ctx, id)
			if err != nil {
				mu.Lock()
				kbResults[id] = &graphQueryResult{kbID: id, err: fmt.Errorf("获取知识库失败: %v", err)}
				mu.Unlock()
				return
			}

			// Check if graph extraction is enabled
			if kb.ExtractConfig == nil || (len(kb.ExtractConfig.Nodes) == 0 && len(kb.ExtractConfig.Relations) == 0) {
				mu.Lock()
				kbResults[id] = &graphQueryResult{kbID: id, err: fmt.Errorf("未配置知识图谱抽取")}
				mu.Unlock()
				return
			}

			// Query graph
			results, err := t.knowledgeService.HybridSearch(ctx, id, searchParams)
			if err != nil {
				mu.Lock()
				kbResults[id] = &graphQueryResult{kbID: id, kb: kb, err: fmt.Errorf("查询失败: %v", err)}
				mu.Unlock()
				return
			}

			mu.Lock()
			kbResults[id] = &graphQueryResult{kbID: id, kb: kb, results: results}
			mu.Unlock()
		}(kbID)
	}

	wg.Wait()

	// Collect and deduplicate results
	seenChunks := make(map[string]*types.SearchResult)
	var errors []string
	graphConfigs := make(map[string]map[string]interface{})
	kbCounts := make(map[string]int)

	for _, kbID := range kbIDs {
		result := kbResults[kbID]
		if result.err != nil {
			errors = append(errors, fmt.Sprintf("KB %s: %v", kbID, result.err))
			continue
		}

		if result.kb != nil && result.kb.ExtractConfig != nil {
			graphConfigs[kbID] = map[string]interface{}{
				"nodes":     result.kb.ExtractConfig.Nodes,
				"relations": result.kb.ExtractConfig.Relations,
			}
		}

		kbCounts[kbID] = len(result.results)
		for _, r := range result.results {
			if _, seen := seenChunks[r.ID]; !seen {
				seenChunks[r.ID] = r
			}
		}
	}

	// Convert map to slice and sort by score
	allResults := make([]*types.SearchResult, 0, len(seenChunks))
	for _, result := range seenChunks {
		allResults = append(allResults, result)
	}

	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Score > allResults[j].Score
	})

	if len(allResults) == 0 {
		return &types.ToolResult{
			Success: true,
			Output:  "未找到相关的图谱信息。",
			Data: map[string]interface{}{
				"knowledge_base_ids": kbIDs,
				"query":              query,
				"results":            []interface{}{},
				"graph_configs":      graphConfigs,
				"errors":             errors,
			},
		}, nil
	}

	// Format output with enhanced graph information
	output := "=== 知识图谱查询 ===\n\n"
	output += fmt.Sprintf("📊 查询: %s\n", query)
	output += fmt.Sprintf("🎯 目标知识库: %v\n", kbIDs)
	output += fmt.Sprintf("✓ 找到 %d 条相关结果（已去重）\n\n", len(allResults))

	if len(errors) > 0 {
		output += "=== ⚠️ 部分失败 ===\n"
		for _, errMsg := range errors {
			output += fmt.Sprintf("  - %s\n", errMsg)
		}
		output += "\n"
	}

	// Display graph configuration status
	hasGraphConfig := false
	output += "=== 📈 图谱配置状态 ===\n\n"
	for kbID, config := range graphConfigs {
		hasGraphConfig = true
		output += fmt.Sprintf("知识库【%s】:\n", kbID)

		nodes, _ := config["nodes"].([]interface{})
		relations, _ := config["relations"].([]interface{})

		if len(nodes) > 0 {
			output += fmt.Sprintf("  ✓ 实体类型 (%d): ", len(nodes))
			nodeNames := make([]string, 0, len(nodes))
			for _, n := range nodes {
				if nodeMap, ok := n.(map[string]interface{}); ok {
					if name, ok := nodeMap["name"].(string); ok {
						nodeNames = append(nodeNames, name)
					}
				}
			}
			output += fmt.Sprintf("%v\n", nodeNames)
		} else {
			output += "  ⚠️ 未配置实体类型\n"
		}

		if len(relations) > 0 {
			output += fmt.Sprintf("  ✓ 关系类型 (%d): ", len(relations))
			relNames := make([]string, 0, len(relations))
			for _, r := range relations {
				if relMap, ok := r.(map[string]interface{}); ok {
					if name, ok := relMap["name"].(string); ok {
						relNames = append(relNames, name)
					}
				}
			}
			output += fmt.Sprintf("%v\n", relNames)
		} else {
			output += "  ⚠️ 未配置关系类型\n"
		}
		output += "\n"
	}

	if !hasGraphConfig {
		output += "⚠️ 所查询的知识库均未配置图谱抽取\n"
		output += "💡 提示: 需要在知识库设置中配置实体和关系类型\n\n"
	}

	// Display result counts by KB
	if len(kbCounts) > 0 {
		output += "=== 📚 知识库覆盖 ===\n"
		for kbID, count := range kbCounts {
			output += fmt.Sprintf("  - %s: %d 条结果\n", kbID, count)
		}
		output += "\n"
	}

	// Display search results
	output += "=== 🔍 查询结果 ===\n\n"
	if !hasGraphConfig {
		output += "💡 当前返回相关文档片段（知识库未配置图谱）\n\n"
	} else {
		output += "💡 基于图谱配置的相关内容检索\n\n"
	}

	formattedResults := make([]map[string]interface{}, 0, len(allResults))
	currentKB := ""

	for i, result := range allResults {
		// Group by knowledge base
		if result.KnowledgeID != currentKB {
			currentKB = result.KnowledgeID
			if i > 0 {
				output += "\n"
			}
			output += fmt.Sprintf("【来源文档: %s】\n\n", result.KnowledgeTitle)
		}

		relevanceLevel := GetRelevanceLevel(result.Score)

		output += fmt.Sprintf("结果 #%d:\n", i+1)
		output += fmt.Sprintf("  📍 相关度: %.2f (%s)\n", result.Score, relevanceLevel)
		output += fmt.Sprintf("  🔗 匹配方式: %s\n", FormatMatchType(result.MatchType))
		output += fmt.Sprintf("  📄 内容: %s\n", result.Content)
		output += fmt.Sprintf("  🆔 chunk_id: %s\n\n", result.ID)

		formattedResults = append(formattedResults, map[string]interface{}{
			"result_index":    i + 1,
			"chunk_id":        result.ID,
			"content":         result.Content,
			"score":           result.Score,
			"relevance_level": relevanceLevel,
			"knowledge_id":    result.KnowledgeID,
			"knowledge_title": result.KnowledgeTitle,
			"match_type":      FormatMatchType(result.MatchType),
		})
	}

	output += "=== 💡 使用提示 ===\n"
	output += "- ✓ 结果已跨知识库去重并按相关度排序\n"
	output += "- ✓ 使用 get_chunk_detail 获取完整内容\n"
	output += "- ✓ 使用 list_knowledge_chunks 探索上下文\n"
	if !hasGraphConfig {
		output += "- ⚠️ 配置图谱抽取以获得更精准的实体关系结果\n"
	}
	output += "- ⏳ 完整的图查询语言（Cypher）支持开发中\n"

	// Build structured graph data for frontend visualization
	graphData := buildGraphVisualizationData(allResults, graphConfigs)

	return &types.ToolResult{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"knowledge_base_ids": kbIDs,
			"query":              query,
			"results":            formattedResults,
			"count":              len(allResults),
			"kb_counts":          kbCounts,
			"graph_configs":      graphConfigs,
			"graph_data":         graphData,
			"has_graph_config":   hasGraphConfig,
			"errors":             errors,
			"display_type":       "graph_query_results",
		},
	}, nil
}

// buildGraphVisualizationData builds structured data for graph visualization
func buildGraphVisualizationData(results []*types.SearchResult, graphConfigs map[string]map[string]interface{}) map[string]interface{} {
	// Build a simple graph structure for frontend visualization
	nodes := make([]map[string]interface{}, 0)
	edges := make([]map[string]interface{}, 0)

	// Create nodes from results
	seenEntities := make(map[string]bool)
	for i, result := range results {
		if !seenEntities[result.ID] {
			nodes = append(nodes, map[string]interface{}{
				"id":       result.ID,
				"label":    fmt.Sprintf("Chunk %d", i+1),
				"content":  result.Content,
				"kb_id":    result.KnowledgeID,
				"kb_title": result.KnowledgeTitle,
				"score":    result.Score,
				"type":     "chunk",
			})
			seenEntities[result.ID] = true
		}
	}

	// TODO: Extract actual entities and relations when graph extraction is fully implemented
	// For now, create placeholder structure

	return map[string]interface{}{
		"nodes":              nodes,
		"edges":              edges,
		"total_nodes":        len(nodes),
		"total_edges":        len(edges),
		"visualization_note": "完整的图可视化功能开发中，当前显示文档节点",
	}
}
