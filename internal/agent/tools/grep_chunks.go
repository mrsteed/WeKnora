package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// GrepChunksTool performs text pattern matching in knowledge base chunks
// Similar to grep command in Unix-like systems, but operates on knowledge base content
type GrepChunksTool struct {
	BaseTool
	db               *gorm.DB
	tenantID         uint
	knowledgeBaseIDs []string
}

// NewGrepChunksTool creates a new grep chunks tool
func NewGrepChunksTool(db *gorm.DB, tenantID uint, knowledgeBaseIDs []string) *GrepChunksTool {
	description := `Unix-style text pattern matching tool for knowledge base chunks.

Searches for text patterns in chunk content, similar to the Unix grep command.

## Core Function
Performs **exact text pattern matching** (NOT semantic search). Finds chunks containing any of the specified patterns using literal text matching (fixed string). Supports multiple patterns with OR logic.

## CRITICAL - Keyword Granularity
**MUST use SHORT keywords (1-3 words), NOT long phrases.** Break down long phrases into smaller keywords for better match rate.

**Why**: Long phrases like "中国饮食文化" may not match if the document contains "中国" and "饮食" and "文化" separately but not the exact phrase.

**Guidelines**:
- ❌ **Bad**: ["中国饮食文化", "日本饮食文化"] - too long, may not match
- ✅ **Good**: ["中国", "饮食", "文化", "日本", "料理", "和食"] - short keywords, higher match rate
- For comparisons: Extract keywords for each entity separately
- Use single words or 2-word phrases, avoid 3+ word phrases

**Examples**:
- "中国饮食文化" → ["中国", "饮食", "文化", "中餐", "中华"]
- "日本饮食文化" → ["日本", "饮食", "文化", "料理", "和食", "日式"]
- "对比中国和日本" → Search separately: ["中国", "中华"] then ["日本", "日式"]

## Usage
grep_chunks searches through all enabled chunks in the knowledge base(s) and displays matching chunks with context. When multiple patterns are provided, results match any pattern (OR logic).

Default behavior: case-insensitive matching, shows chunk indices, displays context around matches.

## When to Use
- Finding specific entities: "FAISS", "Redis", "404", "RAG"
- Exact keyword lookup
- Quick text search before semantic search

## Examples
- Single pattern: pattern=["FAISS"]
- Multiple patterns: pattern=["向量", "vector", "embedding"]
- Search synonyms: pattern=["RAG", "检索增强生成"]
- Short keywords: pattern=["中国", "饮食", "文化"] (NOT ["中国饮食文化"])
`

	return &GrepChunksTool{
		BaseTool:         NewBaseTool("grep_chunks", description),
		db:               db,
		tenantID:         tenantID,
		knowledgeBaseIDs: knowledgeBaseIDs,
	}
}

// Parameters returns the JSON schema for the tool's parameters
func (t *GrepChunksTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "array",
				"description": "REQUIRED: Text patterns to search for. Can be a single pattern or multiple patterns. Treated as literal text (fixed string matching). Results match any of the patterns (OR logic).",
				"items": map[string]interface{}{
					"type": "string",
				},
				"minItems": 1,
			},
			"knowledge_base_ids": map[string]interface{}{
				"type":        "array",
				"description": "Filter by knowledge base IDs. If empty, searches all allowed KBs.",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
			"knowledge_ids": map[string]interface{}{
				"type":        "array",
				"description": "Filter by document/knowledge IDs. If empty, searches all documents.",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
			"max_results": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of matching chunks to return (default: 50, max: 200)",
				"default":     50,
				"minimum":     1,
				"maximum":     200,
			},
		},
		"required": []string{"pattern"},
	}
}

// Execute executes the grep chunks tool
func (t *GrepChunksTool) Execute(ctx context.Context, args map[string]interface{}) (*types.ToolResult, error) {
	logger.Infof(ctx, "[Tool][GrepChunks] Execute started")

	// Parse pattern parameter (required) - support multiple patterns
	var patterns []string
	if patternsRaw, ok := args["pattern"].([]interface{}); ok && len(patternsRaw) > 0 {
		for _, p := range patternsRaw {
			if pStr, ok := p.(string); ok && strings.TrimSpace(pStr) != "" {
				patterns = append(patterns, strings.TrimSpace(pStr))
			}
		}
	}
	// Also support single string for backward compatibility
	if len(patterns) == 0 {
		if patternStr, ok := args["pattern"].(string); ok && strings.TrimSpace(patternStr) != "" {
			patterns = append(patterns, strings.TrimSpace(patternStr))
		}
	}
	if len(patterns) == 0 {
		logger.Errorf(ctx, "[Tool][GrepChunks] Missing or invalid pattern parameter")
		return &types.ToolResult{
			Success: false,
			Error:   "pattern parameter is required and must contain at least one non-empty pattern",
		}, fmt.Errorf("missing pattern parameter")
	}

	// Use default values for all options
	contextLines := 50      // default: 50 context characters
	countOnly := false      // default: show results
	showLineNumbers := true // default: show chunk indices

	maxResults := 50
	if mr, ok := args["max_results"].(float64); ok {
		maxResults = int(mr)
		if maxResults < 1 {
			maxResults = 1
		} else if maxResults > 200 {
			maxResults = 200
		}
	}

	// Parse knowledge_base_ids filter
	var kbIDs []string
	if kbIDsRaw, ok := args["knowledge_base_ids"].([]interface{}); ok {
		for _, id := range kbIDsRaw {
			if idStr, ok := id.(string); ok && idStr != "" {
				kbIDs = append(kbIDs, idStr)
			}
		}
	}
	if len(kbIDs) == 0 {
		kbIDs = t.knowledgeBaseIDs
	}

	// Parse knowledge_ids filter
	var knowledgeIDs []string
	if knowledgeIDsRaw, ok := args["knowledge_ids"].([]interface{}); ok {
		for _, id := range knowledgeIDsRaw {
			if idStr, ok := id.(string); ok && idStr != "" {
				knowledgeIDs = append(knowledgeIDs, idStr)
			}
		}
	}

	logger.Infof(ctx, "[Tool][GrepChunks] Patterns: %v, MaxResults: %d",
		patterns, maxResults)

	// Build and execute query
	results, totalCount, err := t.searchChunks(ctx, patterns, kbIDs, knowledgeIDs, maxResults)
	if err != nil {
		logger.Errorf(ctx, "[Tool][GrepChunks] Search failed: %v", err)
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Search failed: %v", err),
		}, err
	}

	logger.Infof(ctx, "[Tool][GrepChunks] Found %d matching chunks", len(results))

	// Format output
	output := t.formatOutput(ctx, results, totalCount, patterns, contextLines, countOnly, showLineNumbers, kbIDs, knowledgeIDs)

	return &types.ToolResult{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"patterns":           patterns,
			"results":            results,
			"result_count":       len(results),
			"total_matches":      totalCount,
			"knowledge_base_ids": kbIDs,
			"knowledge_ids":      knowledgeIDs,
			"max_results":        maxResults,
			"display_type":       "grep_results",
		},
	}, nil
}

// searchChunks performs the database search with pattern matching
func (t *GrepChunksTool) searchChunks(
	ctx context.Context,
	patterns []string,
	kbIDs []string,
	knowledgeIDs []string,
	maxResults int,
) ([]map[string]interface{}, int64, error) {
	// Build base query
	query := t.db.WithContext(ctx).Table("chunks").
		Select("chunks.id, chunks.content, chunks.chunk_index, chunks.knowledge_id, chunks.knowledge_base_id, chunks.chunk_type, chunks.created_at, knowledges.title as knowledge_title").
		Joins("LEFT JOIN knowledges ON chunks.knowledge_id = knowledges.id").
		Where("chunks.tenant_id = ?", t.tenantID).
		Where("chunks.is_enabled = ?", true).
		Where("chunks.deleted_at IS NULL").
		Where("knowledges.deleted_at IS NULL")

	// Apply knowledge base filter
	if len(kbIDs) > 0 {
		query = query.Where("chunks.knowledge_base_id IN ?", kbIDs)
	}

	// Apply knowledge filter
	if len(knowledgeIDs) > 0 {
		query = query.Where("chunks.knowledge_id IN ?", knowledgeIDs)
	}

	// Apply pattern matching (case-insensitive fixed string matching, OR logic for multiple patterns)
	if len(patterns) == 1 {
		query = query.Where("chunks.content ILIKE ?", "%"+patterns[0]+"%")
	} else {
		// Multiple patterns: use OR logic
		var conditions []string
		var args []interface{}
		for _, pattern := range patterns {
			conditions = append(conditions, "chunks.content ILIKE ?")
			args = append(args, "%"+pattern+"%")
		}
		query = query.Where("("+strings.Join(conditions, " OR ")+")", args...)
	}

	// Count total matches first (for count_only mode)
	var totalCount int64
	if err := query.Count(&totalCount).Error; err != nil {
		logger.Warnf(ctx, "[Tool][GrepChunks] Failed to count matches: %v", err)
	}

	// Fetch results
	var results []map[string]interface{}
	if err := query.Limit(maxResults).Order("chunks.created_at DESC").Find(&results).Error; err != nil {
		logger.Errorf(ctx, "[Tool][GrepChunks] Failed to fetch results: %v", err)
		return nil, 0, err
	}

	return results, totalCount, nil
}

// formatOutput formats the search results for display (grep-style output)
func (t *GrepChunksTool) formatOutput(
	ctx context.Context,
	results []map[string]interface{},
	totalCount int64,
	patterns []string,
	contextLines int,
	countOnly bool,
	showLineNumbers bool,
	kbIDs []string,
	knowledgeIDs []string,
) string {
	var output strings.Builder

	// If count_only mode, just return the count
	if countOnly {
		output.WriteString(fmt.Sprintf("%d\n", totalCount))
		return output.String()
	}

	// Show search info
	if len(patterns) == 1 {
		output.WriteString(fmt.Sprintf("Pattern: '%s' (case-insensitive)\n", patterns[0]))
	} else {
		output.WriteString(fmt.Sprintf("Patterns (%d): %v (case-insensitive, OR logic)\n", len(patterns), patterns))
	}
	output.WriteString(fmt.Sprintf("Matches: %d chunk(s)\n\n", len(results)))

	if len(results) == 0 {
		output.WriteString("No matches found.\n")
		output.WriteString("\n=== ⚠️ CRITICAL - Next Steps ===\n")
		output.WriteString("- ❌ DO NOT use training data or general knowledge to answer\n")
		output.WriteString("- ✅ Try knowledge_search for semantic search\n")
		output.WriteString("- ✅ If KB search fails and web_search is enabled: You MUST use web_search\n")
		output.WriteString("- NEVER fabricate or infer answers - ONLY use retrieved content\n")
		return output.String()
	}

	// Group by document (like grep showing filename)
	docGroups := make(map[string]struct {
		title  string
		chunks []map[string]interface{}
	})

	for _, result := range results {
		knowledgeID := fmt.Sprintf("%v", result["knowledge_id"])
		title := "Untitled"
		if t := result["knowledge_title"]; t != nil {
			title = fmt.Sprintf("%v", t)
		}

		group := docGroups[knowledgeID]
		group.title = title
		group.chunks = append(group.chunks, result)
		docGroups[knowledgeID] = group
	}

	// Display results in grep-style format
	for docID, group := range docGroups {
		// Show document header (like grep showing filename)
		if len(docGroups) > 1 {
			output.WriteString(fmt.Sprintf("\n--- %s (knowledge_id: %s) ---\n", group.title, docID))
		}

		// Show each matching chunk
		for _, chunk := range group.chunks {
			chunkIndex := 0
			if idx, ok := chunk["chunk_index"]; ok {
				switch v := idx.(type) {
				case int:
					chunkIndex = v
				case int64:
					chunkIndex = int(v)
				case float64:
					chunkIndex = int(v)
				}
			}

			content := ""
			if c, ok := chunk["content"]; ok && c != nil {
				content = fmt.Sprintf("%v", c)
			}

			// Extract preview with context around match (case-insensitive)
			// Try each pattern and use the first match
			preview := extractPreviewMultiPattern(content, patterns, false, contextLines)

			// Format like grep: [filename:]line:content
			if showLineNumbers {
				if len(docGroups) > 1 {
					output.WriteString(fmt.Sprintf("%s:chunk[%d]:%s\n", group.title, chunkIndex, preview))
				} else {
					output.WriteString(fmt.Sprintf("chunk[%d]:%s\n", chunkIndex, preview))
				}
			} else {
				if len(docGroups) > 1 {
					output.WriteString(fmt.Sprintf("%s:%s\n", group.title, preview))
				} else {
					output.WriteString(fmt.Sprintf("%s\n", preview))
				}
			}
		}
	}

	// Add guidance for next steps
	output.WriteString("\n=== Next Steps ===\n")
	if len(results) > 0 {
		output.WriteString("- Found matching documents. Use knowledge_search with semantic queries to understand the context.\n")
		output.WriteString("- Filter knowledge_search by knowledge_ids from above results for better relevance.\n")
	} else {
		output.WriteString("- No matches found. Try different keywords or use knowledge_search for semantic search.\n")
		output.WriteString("- Consider using synonyms or related terms in your search patterns.\n")
	}

	return output.String()
}

// extractPreview extracts a preview of content around the matched pattern
func extractPreview(content, pattern string, caseSensitive bool, contextLen int) string {
	if content == "" {
		return ""
	}

	// Find the pattern in content
	searchContent := content
	searchPattern := pattern
	if !caseSensitive {
		searchContent = strings.ToLower(content)
		searchPattern = strings.ToLower(pattern)
	}

	pos := strings.Index(searchContent, searchPattern)
	if pos == -1 {
		// If pattern not found (might be regex), just return first 60 chars
		runes := []rune(content)
		if len(runes) <= contextLen*2 {
			return string(runes)
		}
		return string(runes[:contextLen*2])
	}

	// Extract context around the match
	runes := []rune(content)
	start := pos

	// Convert to rune positions for proper unicode handling
	runePos := 0
	for i, r := range runes {
		if runePos >= start {
			start = i
			break
		}
		runePos += len(string(r))
	}

	// Calculate preview bounds
	previewStart := start - contextLen
	if previewStart < 0 {
		previewStart = 0
	}

	previewEnd := start + len([]rune(pattern)) + contextLen
	if previewEnd > len(runes) {
		previewEnd = len(runes)
	}

	preview := string(runes[previewStart:previewEnd])

	// Clean up whitespace
	preview = strings.ReplaceAll(preview, "\n", " ")
	preview = strings.ReplaceAll(preview, "\t", " ")

	// Collapse multiple spaces
	for strings.Contains(preview, "  ") {
		preview = strings.ReplaceAll(preview, "  ", " ")
	}

	return strings.TrimSpace(preview)
}

// extractPreviewMultiPattern extracts a preview around the first matched pattern
func extractPreviewMultiPattern(content string, patterns []string, caseSensitive bool, contextLen int) string {
	if content == "" || len(patterns) == 0 {
		return ""
	}

	// Try each pattern and find the first match
	for _, pattern := range patterns {
		preview := extractPreview(content, pattern, caseSensitive, contextLen)
		// Check if this pattern found a match (not just truncated content)
		searchContent := content
		searchPattern := pattern
		if !caseSensitive {
			searchContent = strings.ToLower(content)
			searchPattern = strings.ToLower(pattern)
		}
		if strings.Contains(searchContent, searchPattern) {
			return preview
		}
	}

	// If no pattern matched, return preview from first pattern
	if len(patterns) > 0 {
		return extractPreview(content, patterns[0], caseSensitive, contextLen)
	}

	// Fallback: return first N chars
	runes := []rune(content)
	if len(runes) <= contextLen*2 {
		return string(runes)
	}
	return string(runes[:contextLen*2])
}
