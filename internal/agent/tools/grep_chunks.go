package tools

import (
	"context"
	"fmt"
	"math"
	"sort"
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

	// Apply deduplication to remove duplicate or near-duplicate chunks
	deduplicatedResults := t.deduplicateChunks(ctx, results)
	logger.Infof(ctx, "[Tool][GrepChunks] After deduplication: %d chunks (from %d)",
		len(deduplicatedResults), len(results))

	// Calculate match scores for sorting (based on match count and position)
	scoredResults := t.scoreChunks(ctx, deduplicatedResults, patterns)

	// Apply MMR to reduce redundancy if we have many results
	finalResults := scoredResults
	if len(scoredResults) > 10 {
		// Use MMR when we have more than 10 results
		mmrK := len(scoredResults)
		if maxResults > 0 && mmrK > maxResults {
			mmrK = maxResults
		}
		logger.Debugf(ctx, "[Tool][GrepChunks] Applying MMR: k=%d, lambda=0.7, input=%d results", mmrK, len(scoredResults))
		mmrResults := t.applyMMR(ctx, scoredResults, patterns, mmrK, 0.7)
		if len(mmrResults) > 0 {
			finalResults = mmrResults
			logger.Infof(ctx, "[Tool][GrepChunks] MMR completed: %d results selected", len(finalResults))
		}
	}

	// Sort by match score (descending), then by chunk index
	sort.Slice(finalResults, func(i, j int) bool {
		if finalResults[i].MatchScore != finalResults[j].MatchScore {
			return finalResults[i].MatchScore > finalResults[j].MatchScore
		}
		return finalResults[i].ChunkIndex < finalResults[j].ChunkIndex
	})

	if len(finalResults) > 20 {
		finalResults = finalResults[:20]
	}

	// Format output
	output := t.formatOutput(ctx, finalResults, totalCount, patterns, contextLines, countOnly, showLineNumbers, kbIDs, knowledgeIDs)

	return &types.ToolResult{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"patterns":           patterns,
			"results":            finalResults,
			"result_count":       len(finalResults),
			"total_matches":      totalCount,
			"knowledge_base_ids": kbIDs,
			"knowledge_ids":      knowledgeIDs,
			"max_results":        maxResults,
			"display_type":       "grep_results",
		},
	}, nil
}

type chunkWithTitle struct {
	types.Chunk
	Title      string
	MatchScore float64 // Score based on match count and position
}

// searchChunks performs the database search with pattern matching
func (t *GrepChunksTool) searchChunks(
	ctx context.Context,
	patterns []string,
	kbIDs []string,
	knowledgeIDs []string,
	maxResults int,
) ([]chunkWithTitle, int64, error) {
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
	var results []chunkWithTitle
	if err := query.Limit(maxResults).Order("chunks.created_at DESC").Find(&results).Error; err != nil {
		logger.Errorf(ctx, "[Tool][GrepChunks] Failed to fetch results: %v", err)
		return nil, 0, err
	}

	return results, totalCount, nil
}

// formatOutput formats the search results for display (grep-style output)
func (t *GrepChunksTool) formatOutput(
	ctx context.Context,
	results []chunkWithTitle,
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
		title           string
		knowledgeBaseID string
		chunks          []chunkWithTitle
	})

	for _, result := range results {
		knowledgeID := fmt.Sprintf("%v", result.KnowledgeID)
		title := "Untitled"
		if t := result.Title; t != "" {
			title = fmt.Sprintf("%v", t)
		}

		group := docGroups[knowledgeID]
		group.title = title
		group.knowledgeBaseID = result.KnowledgeBaseID
		group.chunks = append(group.chunks, result)
		docGroups[knowledgeID] = group
	}

	// Display results in grep-style format
	for docID, group := range docGroups {
		// Show document header (like grep showing filename)
		output.WriteString(fmt.Sprintf("\n--- knowledge_base_id: %s, knowledge_id: %s, title: %s ---\n",
			group.knowledgeBaseID, docID, group.title))

		// Show each matching chunk
		for _, chunk := range group.chunks {
			chunkIndex := chunk.ChunkIndex
			content := chunk.Content

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

// deduplicateChunks removes duplicate or near-duplicate chunks using content signature
func (t *GrepChunksTool) deduplicateChunks(ctx context.Context, results []chunkWithTitle) []chunkWithTitle {
	seen := make(map[string]bool)
	contentSig := make(map[string]bool)
	uniqueResults := make([]chunkWithTitle, 0)

	for _, r := range results {
		// Build multiple keys for deduplication
		keys := []string{r.ID}
		if r.ParentChunkID != "" {
			keys = append(keys, "parent:"+r.ParentChunkID)
		}
		if r.KnowledgeID != "" {
			keys = append(keys, fmt.Sprintf("kb:%s#%d", r.KnowledgeID, r.ChunkIndex))
		}

		// Check if any key is already seen
		dup := false
		for _, k := range keys {
			if seen[k] {
				dup = true
				break
			}
		}
		if dup {
			continue
		}

		// Check content signature for near-duplicate content
		sig := t.buildContentSignature(r.Content)
		if sig != "" {
			if contentSig[sig] {
				continue
			}
			contentSig[sig] = true
		}

		// Mark all keys as seen
		for _, k := range keys {
			seen[k] = true
		}

		uniqueResults = append(uniqueResults, r)
	}

	// If we have duplicates by ID, keep the first one
	seenByID := make(map[string]bool)
	deduplicated := make([]chunkWithTitle, 0)
	for _, r := range uniqueResults {
		if !seenByID[r.ID] {
			seenByID[r.ID] = true
			deduplicated = append(deduplicated, r)
		}
	}

	return deduplicated
}

// buildContentSignature creates a normalized signature for content to detect near-duplicates
func (t *GrepChunksTool) buildContentSignature(content string) string {
	c := strings.ToLower(strings.TrimSpace(content))
	if c == "" {
		return ""
	}
	// Normalize whitespace
	c = strings.Join(strings.Fields(c), " ")
	// Use first 128 characters as signature
	if len(c) > 128 {
		c = c[:128]
	}
	return c
}

// scoreChunks calculates match scores for chunks based on pattern matches
func (t *GrepChunksTool) scoreChunks(ctx context.Context, results []chunkWithTitle, patterns []string) []chunkWithTitle {
	scored := make([]chunkWithTitle, len(results))
	for i := range results {
		scored[i] = results[i]
		scored[i].MatchScore = t.calculateMatchScore(results[i].Content, patterns)
	}
	return scored
}

// calculateMatchScore calculates a score based on how many patterns match and their positions
func (t *GrepChunksTool) calculateMatchScore(content string, patterns []string) float64 {
	if content == "" || len(patterns) == 0 {
		return 0.0
	}

	contentLower := strings.ToLower(content)
	matchCount := 0
	earliestPos := len(content)

	// Count how many patterns match and find earliest position
	for _, pattern := range patterns {
		patternLower := strings.ToLower(pattern)
		if strings.Contains(contentLower, patternLower) {
			matchCount++
			// Find position of first match
			pos := strings.Index(contentLower, patternLower)
			if pos >= 0 && pos < earliestPos {
				earliestPos = pos
			}
		}
	}

	// Score: higher for more matches, slightly higher for earlier positions
	// Base score: match ratio (0.0 to 1.0)
	baseScore := float64(matchCount) / float64(len(patterns))

	// Position bonus: earlier matches get slight boost (max 0.1)
	positionBonus := 0.0
	if earliestPos < len(content) {
		// Normalize position to [0, 1] and apply small bonus
		positionRatio := 1.0 - float64(earliestPos)/float64(len(content))
		positionBonus = positionRatio * 0.1
	}

	return math.Min(baseScore+positionBonus, 1.0)
}

// applyMMR applies Maximal Marginal Relevance algorithm to reduce redundancy
func (t *GrepChunksTool) applyMMR(
	ctx context.Context,
	results []chunkWithTitle,
	patterns []string,
	k int,
	lambda float64,
) []chunkWithTitle {
	if k <= 0 || len(results) == 0 {
		return nil
	}

	logger.Debugf(ctx, "[Tool][GrepChunks] Applying MMR: lambda=%.2f, k=%d, candidates=%d",
		lambda, k, len(results))

	selected := make([]chunkWithTitle, 0, k)
	candidates := make([]chunkWithTitle, len(results))
	copy(candidates, results)

	// Pre-compute token sets for all candidates
	tokenSets := make([]map[string]struct{}, len(candidates))
	for i, r := range candidates {
		tokenSets[i] = t.tokenizeSimple(r.Content)
	}

	// MMR selection loop
	for len(selected) < k && len(candidates) > 0 {
		bestIdx := 0
		bestScore := -1.0

		for i, r := range candidates {
			relevance := r.MatchScore
			redundancy := 0.0

			// Calculate maximum redundancy with already selected results
			for _, s := range selected {
				selectedTokens := t.tokenizeSimple(s.Content)
				redundancy = math.Max(redundancy, t.jaccard(tokenSets[i], selectedTokens))
			}

			// MMR score: balance relevance and diversity
			mmr := lambda*relevance - (1.0-lambda)*redundancy
			if mmr > bestScore {
				bestScore = mmr
				bestIdx = i
			}
		}

		// Add best candidate to selected and remove from candidates
		selected = append(selected, candidates[bestIdx])
		candidates = append(candidates[:bestIdx], candidates[bestIdx+1:]...)
		// Remove corresponding token set
		tokenSets = append(tokenSets[:bestIdx], tokenSets[bestIdx+1:]...)
	}

	// Compute average redundancy among selected results
	avgRed := 0.0
	if len(selected) > 1 {
		pairs := 0
		for i := 0; i < len(selected); i++ {
			for j := i + 1; j < len(selected); j++ {
				si := t.tokenizeSimple(selected[i].Content)
				sj := t.tokenizeSimple(selected[j].Content)
				avgRed += t.jaccard(si, sj)
				pairs++
			}
		}
		if pairs > 0 {
			avgRed /= float64(pairs)
		}
	}

	logger.Debugf(ctx, "[Tool][GrepChunks] MMR completed: selected=%d, avg_redundancy=%.4f",
		len(selected), avgRed)

	return selected
}

// tokenizeSimple tokenizes text into a set of words (simple whitespace-based)
func (t *GrepChunksTool) tokenizeSimple(text string) map[string]struct{} {
	text = strings.ToLower(text)
	fields := strings.Fields(text)
	set := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		if len(f) > 1 {
			set[f] = struct{}{}
		}
	}
	return set
}

// jaccard calculates Jaccard similarity between two token sets
func (t *GrepChunksTool) jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}

	// Calculate intersection
	inter := 0
	for k := range a {
		if _, ok := b[k]; ok {
			inter++
		}
	}

	// Calculate union
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}

	return float64(inter) / float64(union)
}
