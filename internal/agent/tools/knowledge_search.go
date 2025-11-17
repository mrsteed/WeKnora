package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/models/rerank"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// searchResultWithMeta wraps search result with metadata about which query matched it
type searchResultWithMeta struct {
	*types.SearchResult
	SourceQuery       string
	QueryType         string // "vector" or "keyword"
	KnowledgeBaseID   string // ID of the knowledge base this result came from
	KnowledgeBaseType string // Type of the knowledge base (document, faq, etc.)
}

// KnowledgeSearchTool searches knowledge bases with flexible query modes
type KnowledgeSearchTool struct {
	BaseTool
	knowledgeService interfaces.KnowledgeBaseService
	chunkService     interfaces.ChunkService
	tenantID         uint
	allowedKBs       []string
	rerankModel      rerank.Reranker
	chatModel        chat.Chat // Optional chat model for LLM-based reranking
}

// NewKnowledgeSearchTool creates a new knowledge search tool
func NewKnowledgeSearchTool(
	knowledgeService interfaces.KnowledgeBaseService,
	chunkService interfaces.ChunkService,
	tenantID uint,
	allowedKBs []string,
	rerankModel rerank.Reranker,
	chatModel chat.Chat,
) *KnowledgeSearchTool {
	description := `Search within knowledge bases with flexible query modes. Unified tool that supports both targeted and broad searches.

## Features
- Multi-KB search: Search across multiple knowledge bases concurrently
- Flexible queries: Support vector, keyword, or hybrid search modes
- Quality filtering: Automatically filters low-quality chunks

## Usage

**Use when**:
- You know which knowledge bases to target (specify knowledge_base_ids)
- You're unsure which KB contains the info (omit knowledge_base_ids to search all allowed KBs)
- Want to search specific KBs with same query
- Need semantic (vector) or exact keyword searches
- Want to search only specific documents within KBs


**Search Modes**:
- Simple: Provide single query parameter (hybrid search)
- Vector only: Provide vector_queries only
- Keyword only: Provide keyword_queries only
- Hybrid: Provide both vector_queries and keyword_queries
- At least one query parameter must be provided

**Returns**: Merged and deduplicated search results from all KBs

## Examples

` + "`" + `
# Simple search in specific KBs
{
  "knowledge_base_ids": ["kb1", "kb2"],
  "query": "什么是向量数据库"
}

# Search all allowed KBs with vector queries
{
  "vector_queries": ["什么是向量数据库", "向量数据库的定义"]
}

# Multiple query types with thresholds
{
  "knowledge_base_ids": ["kb1"],
  "vector_queries": ["向量数据库应用"],
  "keyword_queries": ["Docker", "部署"],
  "vector_threshold": 0.7,
  "keyword_threshold": 0.6
}

# Search specific documents
{
  "knowledge_base_ids": ["kb1"],
  "query": "彗星的起源",
  "knowledge_ids": ["doc1", "doc2"]
}
` + "`" + `

## Tips

- Concurrent search across multiple KBs and queries
- Results are automatically reranked to unify scores from different sources
- Reranked scores are in 0-1 range and directly comparable
- Results are merged, deduplicated and sorted by relevance
- Use vector_queries for semantic/conceptual searches
- Use keyword_queries for exact term matching`

	return &KnowledgeSearchTool{
		BaseTool:         NewBaseTool("knowledge_search", description),
		knowledgeService: knowledgeService,
		chunkService:     chunkService,
		tenantID:         tenantID,
		allowedKBs:       allowedKBs,
		rerankModel:      rerankModel,
		chatModel:        chatModel,
	}
}

// Parameters returns the JSON schema for the tool's parameters
func (t *KnowledgeSearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"knowledge_base_ids": map[string]interface{}{
				"type":        "array",
				"description": "Array of knowledge base IDs to search in (optional, if omitted searches all allowed KBs)",
				"items": map[string]interface{}{
					"type": "string",
				},
				"minItems": 1,
				"maxItems": 10,
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Single search query for simple hybrid search",
			},
			"vector_queries": map[string]interface{}{
				"type":        "array",
				"description": "Array of semantic queries for vector search (1-5 queries)",
				"items": map[string]interface{}{
					"type": "string",
				},
				"minItems": 1,
				"maxItems": 5,
			},
			"keyword_queries": map[string]interface{}{
				"type":        "array",
				"description": "Array of keyword queries for keyword search (1-5 queries)",
				"items": map[string]interface{}{
					"type": "string",
				},
				"minItems": 1,
				"maxItems": 5,
			},
			"top_k": map[string]interface{}{
				"type":        "integer",
				"description": "Number of results per knowledge base per query (default: 5)",
				"default":     5,
				"minimum":     1,
				"maximum":     20,
			},
			"vector_threshold": map[string]interface{}{
				"type":        "number",
				"description": "Minimum score for vector results (default: 0.6)",
				"default":     0.6,
				"minimum":     0.0,
				"maximum":     1.0,
			},
			"keyword_threshold": map[string]interface{}{
				"type":        "number",
				"description": "Minimum score for keyword results (default: 0.5)",
				"default":     0.5,
				"minimum":     0.0,
				"maximum":     1.0,
			},
			"knowledge_ids": map[string]interface{}{
				"type":        "array",
				"description": "Optional array of document IDs to filter results (only return results from these specific documents)",
				"items": map[string]interface{}{
					"type": "string",
				},
				"minItems": 1,
				"maxItems": 50,
			},
			"min_score": map[string]interface{}{
				"type":        "number",
				"description": "Absolute minimum score threshold for filtering very low quality results (default: 0.3)",
				"default":     0.3,
				"minimum":     0.0,
				"maximum":     1.0,
			},
		},
		"required": []string{},
	}
}

// Execute executes the knowledge search tool with flexible query modes
func (t *KnowledgeSearchTool) Execute(ctx context.Context, args map[string]interface{}) (*types.ToolResult, error) {
	logger.Infof(ctx, "[Tool][KnowledgeSearch] Execute started")

	// Log input arguments
	argsJSON, _ := json.MarshalIndent(args, "", "  ")
	logger.Debugf(ctx, "[Tool][KnowledgeSearch] Input args:\n%s", string(argsJSON))

	// Determine which KBs to search
	var kbIDs []string
	if kbIDsRaw, ok := args["knowledge_base_ids"].([]interface{}); ok && len(kbIDsRaw) > 0 {
		for _, id := range kbIDsRaw {
			if idStr, ok := id.(string); ok && idStr != "" {
				kbIDs = append(kbIDs, idStr)
			}
		}
		logger.Infof(ctx, "[Tool][KnowledgeSearch] User specified %d knowledge bases: %v", len(kbIDs), kbIDs)
	}

	// If no KBs specified, use allowed KBs
	if len(kbIDs) == 0 {
		kbIDs = t.allowedKBs
		if len(kbIDs) == 0 {
			logger.Errorf(ctx, "[Tool][KnowledgeSearch] No knowledge bases available")
			return &types.ToolResult{
				Success: false,
				Error:   "no knowledge bases specified and no allowed KBs configured",
			}, fmt.Errorf("no knowledge bases available")
		}
		logger.Infof(ctx, "[Tool][KnowledgeSearch] Using all allowed KBs (%d): %v", len(kbIDs), kbIDs)
	}

	// Parse query parameters
	var singleQuery string
	var vectorQueries, keywordQueries []string

	// Parse single query
	if q, ok := args["query"].(string); ok && q != "" {
		singleQuery = q
	}

	// Parse vector_queries
	if vq, ok := args["vector_queries"].([]interface{}); ok {
		for _, q := range vq {
			if queryStr, ok := q.(string); ok && queryStr != "" {
				vectorQueries = append(vectorQueries, queryStr)
			}
		}
	}

	// Parse keyword_queries
	if kq, ok := args["keyword_queries"].([]interface{}); ok {
		for _, q := range kq {
			if queryStr, ok := q.(string); ok && queryStr != "" {
				keywordQueries = append(keywordQueries, queryStr)
			}
		}
	}

	// If single query provided, treat it as both vector and keyword query
	if singleQuery != "" {
		if len(vectorQueries) == 0 && len(keywordQueries) == 0 {
			vectorQueries = []string{singleQuery}
			keywordQueries = []string{singleQuery}
		}
	}

	// Validate: at least one query must be provided
	if len(vectorQueries) == 0 && len(keywordQueries) == 0 {
		logger.Errorf(ctx, "[Tool][KnowledgeSearch] No query provided")
		return &types.ToolResult{
			Success: false,
			Error:   "at least one of query, vector_queries, or keyword_queries must be provided",
		}, fmt.Errorf("no query provided")
	}

	logger.Infof(ctx, "[Tool][KnowledgeSearch] Query mode: single=%v, vector_queries=%d, keyword_queries=%d",
		singleQuery != "", len(vectorQueries), len(keywordQueries))
	if singleQuery != "" {
		logger.Debugf(ctx, "[Tool][KnowledgeSearch] Single query: %s", singleQuery)
	}
	if len(vectorQueries) > 0 {
		logger.Debugf(ctx, "[Tool][KnowledgeSearch] Vector queries: %v", vectorQueries)
	}
	if len(keywordQueries) > 0 {
		logger.Debugf(ctx, "[Tool][KnowledgeSearch] Keyword queries: %v", keywordQueries)
	}

	// Parse thresholds
	vectorThreshold := 0.6
	if vt, ok := args["vector_threshold"].(float64); ok {
		vectorThreshold = vt
	}

	keywordThreshold := 0.5
	if kt, ok := args["keyword_threshold"].(float64); ok {
		keywordThreshold = kt
	}

	// Parse min_score for absolute filtering
	minScore := 0.3
	if ms, ok := args["min_score"].(float64); ok {
		minScore = ms
	}

	// Parse top_k
	topK := 5
	if topKVal, ok := args["top_k"]; ok {
		switch v := topKVal.(type) {
		case float64:
			topK = int(v)
		case int:
			topK = v
		}
	}

	logger.Infof(ctx, "[Tool][KnowledgeSearch] Search params: top_k=%d, vector_threshold=%.2f, keyword_threshold=%.2f, min_score=%.2f",
		topK, vectorThreshold, keywordThreshold, minScore)

	// Extract knowledge_ids filter if provided
	var knowledgeIDsFilter map[string]bool
	if knowledgeIDsRaw, ok := args["knowledge_ids"].([]interface{}); ok && len(knowledgeIDsRaw) > 0 {
		knowledgeIDsFilter = make(map[string]bool)
		for _, id := range knowledgeIDsRaw {
			if idStr, ok := id.(string); ok && idStr != "" {
				knowledgeIDsFilter[idStr] = true
			}
		}
	}

	// Execute concurrent search
	logger.Infof(ctx, "[Tool][KnowledgeSearch] Starting concurrent search across %d KBs", len(kbIDs))
	kbTypeMap := t.getKnowledgeBaseTypes(ctx, kbIDs)

	allResults := t.concurrentSearch(ctx, vectorQueries, keywordQueries, kbIDs,
		topK, vectorThreshold, keywordThreshold, kbTypeMap)
	logger.Infof(ctx, "[Tool][KnowledgeSearch] Concurrent search completed: %d raw results", len(allResults))

	// Filter by knowledge_ids if provided
	if len(knowledgeIDsFilter) > 0 {
		logger.Infof(ctx, "[Tool][KnowledgeSearch] Filtering by %d knowledge IDs", len(knowledgeIDsFilter))
		filtered := make([]*searchResultWithMeta, 0)
		for _, r := range allResults {
			if knowledgeIDsFilter[r.KnowledgeID] {
				filtered = append(filtered, r)
			}
		}
		logger.Infof(ctx, "[Tool][KnowledgeSearch] After knowledge_id filter: %d results (from %d)",
			len(filtered), len(allResults))
		allResults = filtered
	}

	// Filter by threshold first
	logger.Infof(ctx, "[Tool][KnowledgeSearch] Applying threshold filter...")
	filteredResults := t.filterByThreshold(allResults, vectorThreshold, keywordThreshold)
	logger.Infof(ctx, "[Tool][KnowledgeSearch] After threshold filter: %d results (from %d)",
		len(filteredResults), len(allResults))

	// Apply ReRank if model is configured
	// Prefer chatModel (LLM-based reranking) over rerankModel if both are available
	if t.chatModel != nil && len(filteredResults) > 0 {
		logger.Infof(ctx, "[Tool][KnowledgeSearch] Applying LLM-based rerank with model: %s, input: %d results",
			t.chatModel.GetModelName(), len(filteredResults))
		rerankQuery := singleQuery
		if rerankQuery == "" && len(vectorQueries) > 0 {
			rerankQuery = vectorQueries[0] // Use first vector query as rerank query
		} else if rerankQuery == "" && len(keywordQueries) > 0 {
			rerankQuery = keywordQueries[0] // Use first keyword query as fallback
		}

		if rerankQuery != "" {
			logger.Debugf(ctx, "[Tool][KnowledgeSearch] Rerank query: %s", rerankQuery)
			rerankedResults, err := t.rerankResults(ctx, rerankQuery, filteredResults)
			if err != nil {
				logger.Warnf(ctx, "[Tool][KnowledgeSearch] LLM rerank failed, using original results: %v", err)
			} else {
				filteredResults = rerankedResults
				logger.Infof(ctx, "[Tool][KnowledgeSearch] LLM rerank completed successfully: %d results",
					len(filteredResults))
			}
		}
	} else if t.rerankModel != nil && len(filteredResults) > 0 {
		logger.Infof(ctx, "[Tool][KnowledgeSearch] Applying rerank with model: %s, input: %d results",
			t.rerankModel.GetModelName(), len(filteredResults))
		rerankQuery := singleQuery
		if rerankQuery == "" && len(vectorQueries) > 0 {
			rerankQuery = vectorQueries[0] // Use first vector query as rerank query
		} else if rerankQuery == "" && len(keywordQueries) > 0 {
			rerankQuery = keywordQueries[0] // Use first keyword query as fallback
		}

		if rerankQuery != "" {
			logger.Debugf(ctx, "[Tool][KnowledgeSearch] Rerank query: %s", rerankQuery)
			rerankedResults, err := t.rerankResults(ctx, rerankQuery, filteredResults)
			if err != nil {
				logger.Warnf(ctx, "[Tool][KnowledgeSearch] Rerank failed, using original results: %v", err)
			} else {
				filteredResults = rerankedResults
				logger.Infof(ctx, "[Tool][KnowledgeSearch] Rerank completed successfully: %d results",
					len(filteredResults))
			}
		}
	}

	// Apply absolute minimum score filter to remove very low quality chunks
	logger.Debugf(ctx, "[Tool][KnowledgeSearch] Applying min_score filter (%.2f)...", minScore)
	filteredResults = t.filterByMinScore(filteredResults, minScore)
	logger.Infof(ctx, "[Tool][KnowledgeSearch] After min_score filter: %d results", len(filteredResults))

	logger.Debugf(ctx, "[Tool][KnowledgeSearch] Deduplicating results...")
	deduplicatedResults := t.deduplicateResults(filteredResults)
	logger.Infof(ctx, "[Tool][KnowledgeSearch] After deduplication: %d results (from %d)",
		len(deduplicatedResults), len(filteredResults))

	// Sort results by score (descending)
	logger.Debugf(ctx, "[Tool][KnowledgeSearch] Sorting results by score...")
	sort.Slice(deduplicatedResults, func(i, j int) bool {
		if deduplicatedResults[i].Score != deduplicatedResults[j].Score {
			return deduplicatedResults[i].Score > deduplicatedResults[j].Score
		}
		// If scores are equal, prefer vector matches
		if deduplicatedResults[i].QueryType != deduplicatedResults[j].QueryType {
			return deduplicatedResults[i].QueryType == "vector"
		}
		return deduplicatedResults[i].KnowledgeID < deduplicatedResults[j].KnowledgeID
	})

	// Log top results
	if len(deduplicatedResults) > 0 {
		logger.Infof(ctx, "[Tool][KnowledgeSearch] Top 5 results by score:")
		for i := 0; i < len(deduplicatedResults) && i < 5; i++ {
			r := deduplicatedResults[i]
			logger.Infof(ctx, "[Tool][KnowledgeSearch]   #%d: score=%.3f, type=%s, kb=%s, chunk_id=%s",
				i+1, r.Score, r.QueryType, r.KnowledgeID, r.ID)
		}
	}

	// Build output
	logger.Infof(ctx, "[Tool][KnowledgeSearch] Formatting output with %d final results", len(deduplicatedResults))
	result, err := t.formatOutput(ctx, deduplicatedResults, vectorQueries, keywordQueries,
		kbIDs, len(allResults), vectorThreshold, keywordThreshold, knowledgeIDsFilter, singleQuery)
	if err != nil {
		logger.Errorf(ctx, "[Tool][KnowledgeSearch] Failed to format output: %v", err)
		return result, err
	}

	logger.Infof(ctx, "[Tool][KnowledgeSearch] Execute completed successfully")
	return result, nil
}

// getKnowledgeBaseTypes fetches knowledge base types for the given IDs
func (t *KnowledgeSearchTool) getKnowledgeBaseTypes(ctx context.Context, kbIDs []string) map[string]string {
	kbTypeMap := make(map[string]string, len(kbIDs))

	for _, kbID := range kbIDs {
		if kbID == "" {
			continue
		}
		if _, exists := kbTypeMap[kbID]; exists {
			continue
		}

		kb, err := t.knowledgeService.GetKnowledgeBaseByID(ctx, kbID)
		if err != nil {
			logger.Warnf(ctx, "[Tool][KnowledgeSearch] Failed to fetch knowledge base %s info: %v", kbID, err)
			continue
		}

		kbTypeMap[kbID] = kb.Type
	}

	return kbTypeMap
}

// concurrentSearch executes vector and keyword searches concurrently
func (t *KnowledgeSearchTool) concurrentSearch(
	ctx context.Context,
	vectorQueries, keywordQueries []string,
	kbsToSearch []string,
	topK int,
	vectorThreshold, keywordThreshold float64,
	kbTypeMap map[string]string,
) []*searchResultWithMeta {
	var wg sync.WaitGroup
	var mu sync.Mutex
	allResults := make([]*searchResultWithMeta, 0)

	// Launch vector searches
	if len(vectorQueries) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results := t.searchWithQueries(ctx, vectorQueries, kbsToSearch, topK,
				vectorThreshold, 1.0, "vector", kbTypeMap)
			mu.Lock()
			allResults = append(allResults, results...)
			mu.Unlock()
		}()
	}

	// Launch keyword searches
	if len(keywordQueries) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results := t.searchWithQueries(ctx, keywordQueries, kbsToSearch, topK,
				1.0, keywordThreshold, "keyword", kbTypeMap)
			mu.Lock()
			allResults = append(allResults, results...)
			mu.Unlock()
		}()
	}

	wg.Wait()
	return allResults
}

// searchWithQueries executes multiple queries concurrently
func (t *KnowledgeSearchTool) searchWithQueries(
	ctx context.Context,
	queries []string,
	kbsToSearch []string,
	topK int,
	vectorThreshold, keywordThreshold float64,
	queryType string,
	kbTypeMap map[string]string,
) []*searchResultWithMeta {
	var wg sync.WaitGroup
	var mu sync.Mutex
	allResults := make([]*searchResultWithMeta, 0)

	for _, query := range queries {
		wg.Add(1)
		go func(q string) {
			defer wg.Done()
			results := t.searchSingleQuery(ctx, q, kbsToSearch, topK,
				vectorThreshold, keywordThreshold, queryType, kbTypeMap)
			mu.Lock()
			allResults = append(allResults, results...)
			mu.Unlock()
		}(query)
	}

	wg.Wait()
	return allResults
}

// searchSingleQuery searches a single query across multiple KBs concurrently
func (t *KnowledgeSearchTool) searchSingleQuery(
	ctx context.Context,
	query string,
	kbsToSearch []string,
	topK int,
	vectorThreshold, keywordThreshold float64,
	queryType string,
	kbTypeMap map[string]string,
) []*searchResultWithMeta {
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make([]*searchResultWithMeta, 0)

	searchParams := types.SearchParams{
		QueryText:        query,
		MatchCount:       topK,
		VectorThreshold:  vectorThreshold,
		KeywordThreshold: keywordThreshold,
	}

	for _, kbID := range kbsToSearch {
		wg.Add(1)
		go func(kb string) {
			defer wg.Done()

			kbResults, err := t.knowledgeService.HybridSearch(ctx, kb, searchParams)
			if err != nil {
				// Log error but continue with other KBs
				return
			}

			// Wrap results with metadata
			mu.Lock()
			for _, r := range kbResults {
				results = append(results, &searchResultWithMeta{
					SearchResult:      r,
					SourceQuery:       query,
					QueryType:         queryType,
					KnowledgeBaseID:   kb,
					KnowledgeBaseType: kbTypeMap[kb],
				})
			}
			mu.Unlock()
		}(kbID)
	}

	wg.Wait()
	return results
}

// filterByThreshold filters results based on match type and threshold
func (t *KnowledgeSearchTool) filterByThreshold(
	results []*searchResultWithMeta,
	vectorThreshold, keywordThreshold float64,
) []*searchResultWithMeta {
	filtered := make([]*searchResultWithMeta, 0)
	for _, r := range results {
		// Check if result meets threshold based on match type
		if r.MatchType == types.MatchTypeEmbedding && r.Score >= vectorThreshold {
			filtered = append(filtered, r)
		} else if r.MatchType == types.MatchTypeKeywords && r.Score >= keywordThreshold {
			filtered = append(filtered, r)
		} else {
			// For other match types (graph, nearby chunk, etc.), use the lower threshold
			minThreshold := vectorThreshold
			if keywordThreshold < minThreshold {
				minThreshold = keywordThreshold
			}
			if r.Score >= minThreshold {
				filtered = append(filtered, r)
			}
		}
	}
	return filtered
}

// rerankResults applies reranking to search results using LLM prompt scoring or rerank model
func (t *KnowledgeSearchTool) rerankResults(
	ctx context.Context,
	query string,
	results []*searchResultWithMeta,
) ([]*searchResultWithMeta, error) {
	// Separate FAQ and non-FAQ results. FAQ results keep original scores.
	faqResults := make([]*searchResultWithMeta, 0)
	nonFAQResults := make([]*searchResultWithMeta, 0, len(results))

	for _, result := range results {
		if result.KnowledgeBaseType == types.KnowledgeBaseTypeFAQ {
			faqResults = append(faqResults, result)
		} else {
			nonFAQResults = append(nonFAQResults, result)
		}
	}

	// If there are no non-FAQ results, return original list (already all FAQ)
	if len(nonFAQResults) == 0 {
		return results, nil
	}

	var (
		rerankedNonFAQ []*searchResultWithMeta
		err            error
	)

	// Apply reranking only to non-FAQ results
	switch {
	case t.chatModel != nil:
		rerankedNonFAQ, err = t.rerankWithLLM(ctx, query, nonFAQResults)
	case t.rerankModel != nil:
		rerankedNonFAQ, err = t.rerankWithModel(ctx, query, nonFAQResults)
	default:
		rerankedNonFAQ = nonFAQResults
	}

	if err != nil {
		return nil, err
	}

	// Combine FAQ results (with original order) and reranked non-FAQ results
	combined := make([]*searchResultWithMeta, 0, len(results))
	combined = append(combined, faqResults...)
	combined = append(combined, rerankedNonFAQ...)

	// Sort by score (descending) to keep consistent output order
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].Score > combined[j].Score
	})

	return combined, nil
}

func (t *KnowledgeSearchTool) getFAQMetadata(
	ctx context.Context,
	chunkID string,
	cache map[string]*types.FAQChunkMetadata,
) (*types.FAQChunkMetadata, error) {
	if chunkID == "" || t.chunkService == nil {
		return nil, nil
	}

	if meta, ok := cache[chunkID]; ok {
		return meta, nil
	}

	chunk, err := t.chunkService.GetChunkByID(ctx, chunkID)
	if err != nil {
		cache[chunkID] = nil
		return nil, err
	}
	if chunk == nil {
		cache[chunkID] = nil
		return nil, nil
	}

	meta, err := chunk.FAQMetadata()
	if err != nil {
		cache[chunkID] = nil
		return nil, err
	}
	cache[chunkID] = meta
	return meta, nil
}

// rerankWithLLM uses LLM prompt to score and rerank search results
// Uses batch processing to handle large result sets efficiently
func (t *KnowledgeSearchTool) rerankWithLLM(
	ctx context.Context,
	query string,
	results []*searchResultWithMeta,
) ([]*searchResultWithMeta, error) {
	logger.Infof(ctx, "[Tool][KnowledgeSearch] Using LLM for reranking %d results", len(results))

	if len(results) == 0 {
		return results, nil
	}

	// Batch size: process 15 results at a time to balance quality and token usage
	// This prevents token overflow and improves processing efficiency
	const batchSize = 15
	const maxContentLength = 800 // Maximum characters per passage to avoid excessive tokens

	// Process in batches
	allScores := make([]float64, len(results))
	allReranked := make([]*searchResultWithMeta, 0, len(results))

	for batchStart := 0; batchStart < len(results); batchStart += batchSize {
		batchEnd := batchStart + batchSize
		if batchEnd > len(results) {
			batchEnd = len(results)
		}

		batch := results[batchStart:batchEnd]
		logger.Debugf(ctx, "[Tool][KnowledgeSearch] Processing rerank batch %d-%d of %d results",
			batchStart+1, batchEnd, len(results))

		// Build prompt with query and batch passages
		var passagesBuilder strings.Builder
		for i, result := range batch {
			// Truncate content if too long to save tokens
			content := result.Content
			if len([]rune(content)) > maxContentLength {
				runes := []rune(content)
				content = string(runes[:maxContentLength]) + "..."
			}
			// Use clear separators to distinguish each passage
			if i > 0 {
				passagesBuilder.WriteString("\n")
			}
			passagesBuilder.WriteString("─────────────────────────────────────────────────────────────\n")
			passagesBuilder.WriteString(fmt.Sprintf("Passage %d:\n", i+1))
			passagesBuilder.WriteString("─────────────────────────────────────────────────────────────\n")
			passagesBuilder.WriteString(content + "\n")
		}

		// Optimized prompt focused on retrieval matching and reranking
		prompt := fmt.Sprintf(`You are a search result reranking expert. Your task is to evaluate how well each retrieved passage matches the user's search query and information need.

User Query: %s

Your task: Rerank these search results by evaluating their retrieval relevance - how well each passage answers or relates to the query.

Scoring Criteria (0.0 to 1.0):
- 1.0 (0.9-1.0): Directly answers the query, contains key information needed, highly relevant
- 0.8 (0.7-0.8): Strongly related, provides substantial relevant information
- 0.6 (0.5-0.6): Moderately related, contains some relevant information but may be incomplete
- 0.4 (0.3-0.4): Weakly related, minimal relevance to the query
- 0.2 (0.1-0.2): Barely related, mostly irrelevant
- 0.0 (0.0): Completely irrelevant, no relation to the query

Evaluation Factors:
1. Query-Answer Match: Does the passage directly address what the user is asking?
2. Information Completeness: Does it provide sufficient information to answer the query?
3. Semantic Relevance: Does the content semantically relate to the query intent?
4. Key Term Coverage: Does it cover important terms/concepts from the query?
5. Information Accuracy: Is the information accurate and trustworthy?

Retrieved Passages:
%s

IMPORTANT: Return exactly %d scores, one per line, in this exact format:
Passage 1: X.XX
Passage 2: X.XX
Passage 3: X.XX
...
Passage %d: X.XX

Output only the scores, no explanations or additional text.`, query, passagesBuilder.String(), len(batch), len(batch))

		messages := []chat.Message{
			{
				Role:    "system",
				Content: "You are a professional search result reranking expert specializing in information retrieval. You evaluate how well retrieved passages match user queries in search scenarios. Focus on retrieval relevance: whether the passage answers the query, provides needed information, and matches the user's information need. Always respond with scores only, no explanations.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		}

		// Calculate appropriate max tokens based on batch size
		// Each score line is ~15 tokens, add buffer for safety
		maxTokens := len(batch)*20 + 100

		response, err := t.chatModel.Chat(ctx, messages, &chat.ChatOptions{
			Temperature: 0.1, // Low temperature for consistent scoring
			MaxTokens:   maxTokens,
		})
		if err != nil {
			logger.Warnf(ctx, "[Tool][KnowledgeSearch] LLM rerank batch %d-%d failed: %v, using original scores",
				batchStart+1, batchEnd, err)
			// Use original scores for this batch on error
			for i := batchStart; i < batchEnd; i++ {
				allScores[i] = results[i].Score
			}
			continue
		}

		logger.Infof(ctx, "[Tool][KnowledgeSearch] LLM rerank batch %d-%d response: %s",
			batchStart+1, batchEnd, response.Content)

		// Parse scores from response
		batchScores, err := t.parseScoresFromResponse(response.Content, len(batch))
		if err != nil {
			logger.Warnf(ctx, "[Tool][KnowledgeSearch] Failed to parse LLM scores for batch %d-%d: %v, using original scores",
				batchStart+1, batchEnd, err)
			// Use original scores for this batch on parsing error
			for i := batchStart; i < batchEnd; i++ {
				allScores[i] = results[i].Score
			}
			continue
		}

		// Store scores for this batch
		for i, score := range batchScores {
			if batchStart+i < len(allScores) {
				allScores[batchStart+i] = score
			}
		}
	}

	// Create reranked results with new scores
	for i, result := range results {
		newResult := *result
		if i < len(allScores) {
			newResult.Score = allScores[i]
		}
		allReranked = append(allReranked, &newResult)
	}

	// Sort by new scores (descending)
	sort.Slice(allReranked, func(i, j int) bool {
		return allReranked[i].Score > allReranked[j].Score
	})

	logger.Infof(ctx, "[Tool][KnowledgeSearch] LLM reranked %d results from %d original results (processed in batches)",
		len(allReranked), len(results))
	return allReranked, nil
}

// parseScoresFromResponse parses scores from LLM response text
func (t *KnowledgeSearchTool) parseScoresFromResponse(responseText string, expectedCount int) ([]float64, error) {
	lines := strings.Split(strings.TrimSpace(responseText), "\n")
	scores := make([]float64, 0, expectedCount)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try to extract score from various formats:
		// "Passage 1: 0.85"
		// "1: 0.85"
		// "0.85"
		// etc.
		parts := strings.Split(line, ":")
		var scoreStr string
		if len(parts) >= 2 {
			scoreStr = strings.TrimSpace(parts[len(parts)-1])
		} else {
			scoreStr = strings.TrimSpace(line)
		}

		// Remove any non-numeric characters except decimal point
		scoreStr = strings.TrimFunc(scoreStr, func(r rune) bool {
			return (r < '0' || r > '9') && r != '.'
		})

		if scoreStr == "" {
			continue
		}

		score, err := strconv.ParseFloat(scoreStr, 64)
		if err != nil {
			continue // Skip invalid scores
		}

		// Clamp score to [0.0, 1.0]
		if score < 0.0 {
			score = 0.0
		}
		if score > 1.0 {
			score = 1.0
		}

		scores = append(scores, score)
	}

	if len(scores) == 0 {
		return nil, fmt.Errorf("no valid scores found in response")
	}

	// If we got fewer scores than expected, pad with last score or 0.5
	for len(scores) < expectedCount {
		if len(scores) > 0 {
			scores = append(scores, scores[len(scores)-1])
		} else {
			scores = append(scores, 0.5)
		}
	}

	// Truncate if we got more scores than expected
	if len(scores) > expectedCount {
		scores = scores[:expectedCount]
	}

	return scores, nil
}

// rerankWithModel uses the rerank model for reranking (fallback)
func (t *KnowledgeSearchTool) rerankWithModel(
	ctx context.Context,
	query string,
	results []*searchResultWithMeta,
) ([]*searchResultWithMeta, error) {
	// Prepare passages for reranking
	passages := make([]string, len(results))
	for i, result := range results {
		passages[i] = result.Content
	}

	// Call rerank model
	rerankResp, err := t.rerankModel.Rerank(ctx, query, passages)
	if err != nil {
		return nil, fmt.Errorf("rerank call failed: %w", err)
	}

	// Map reranked results back with new scores
	reranked := make([]*searchResultWithMeta, 0, len(rerankResp))
	for _, rr := range rerankResp {
		if rr.Index >= 0 && rr.Index < len(results) {
			// Create new result with reranked score
			newResult := *results[rr.Index]
			newResult.Score = rr.RelevanceScore
			reranked = append(reranked, &newResult)
		}
	}

	logger.Infof(ctx, "[Tool][KnowledgeSearch] Reranked %d results from %d original results", len(reranked), len(results))
	return reranked, nil
}

// filterByMinScore filters results by absolute minimum score
func (t *KnowledgeSearchTool) filterByMinScore(
	results []*searchResultWithMeta,
	minScore float64,
) []*searchResultWithMeta {
	filtered := make([]*searchResultWithMeta, 0)
	for _, r := range results {
		if r.Score >= minScore {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// deduplicateResults removes duplicate chunks, keeping the highest score
func (t *KnowledgeSearchTool) deduplicateResults(results []*searchResultWithMeta) []*searchResultWithMeta {
	seen := make(map[string]*searchResultWithMeta)

	for _, r := range results {
		if existing, ok := seen[r.ID]; ok {
			// Keep the result with higher score
			if r.Score > existing.Score {
				seen[r.ID] = r
			}
		} else {
			seen[r.ID] = r
		}
	}

	deduplicated := make([]*searchResultWithMeta, 0, len(seen))
	for _, r := range seen {
		deduplicated = append(deduplicated, r)
	}

	return deduplicated
}

// formatOutput formats the search results for display
func (t *KnowledgeSearchTool) formatOutput(
	ctx context.Context,
	results []*searchResultWithMeta,
	vectorQueries, keywordQueries []string,
	kbsToSearch []string,
	totalBeforeFilter int,
	vectorThreshold, keywordThreshold float64,
	knowledgeIDsFilter map[string]bool,
	singleQuery string,
) (*types.ToolResult, error) {
	if len(results) == 0 {
		data := map[string]interface{}{
			"knowledge_base_ids": kbsToSearch,
			"results":            []interface{}{},
			"count":              0,
		}
		if len(knowledgeIDsFilter) > 0 {
			filterList := make([]string, 0, len(knowledgeIDsFilter))
			for id := range knowledgeIDsFilter {
				filterList = append(filterList, id)
			}
			data["knowledge_ids"] = filterList
		}
		if singleQuery != "" {
			data["query"] = singleQuery
		}
		return &types.ToolResult{
			Success: true,
			Output:  fmt.Sprintf("No relevant content found in %d knowledge base(s).", len(kbsToSearch)),
			Data:    data,
		}, nil
	}

	// Determine search mode
	searchMode := "Hybrid (Vector + Keyword)"
	if len(vectorQueries) > 0 && len(keywordQueries) == 0 {
		searchMode = "Vector"
	} else if len(vectorQueries) == 0 && len(keywordQueries) > 0 {
		searchMode = "Keyword"
	}

	// Build output header
	output := "=== Search Results ===\n"
	output += fmt.Sprintf("Knowledge Bases: %v\n", kbsToSearch)
	if len(knowledgeIDsFilter) > 0 {
		filterList := make([]string, 0, len(knowledgeIDsFilter))
		for id := range knowledgeIDsFilter {
			filterList = append(filterList, id)
		}
		output += fmt.Sprintf("Document Filter: %v\n", filterList)
	}
	output += fmt.Sprintf("Search Mode: %s\n", searchMode)

	if singleQuery != "" {
		output += fmt.Sprintf("Query: %s\n", singleQuery)
	} else {
		if len(vectorQueries) > 0 {
			output += fmt.Sprintf("Vector Queries: %v\n", vectorQueries)
			output += fmt.Sprintf("Vector Threshold: %.2f\n", vectorThreshold)
		}
		if len(keywordQueries) > 0 {
			output += fmt.Sprintf("Keyword Queries: %v\n", keywordQueries)
			output += fmt.Sprintf("Keyword Threshold: %.2f\n", keywordThreshold)
		}
	}

	output += fmt.Sprintf("Found %d relevant results (deduplicated)", len(results))
	if totalBeforeFilter > len(results) {
		output += fmt.Sprintf(" (filtered from %d)", totalBeforeFilter)
	}
	output += "\n\n"

	// Count results by KB
	kbCounts := make(map[string]int)
	for _, r := range results {
		kbCounts[r.KnowledgeID]++
	}

	output += "Knowledge Base Coverage:\n"
	for kbID, count := range kbCounts {
		output += fmt.Sprintf("  - %s: %d results\n", kbID, count)
	}
	output += "\n=== Detailed Results ===\n\n"

	// Format individual results
	formattedResults := make([]map[string]interface{}, 0, len(results))
	currentKB := ""

	faqMetadataCache := make(map[string]*types.FAQChunkMetadata)

	for i, result := range results {
		var faqMeta *types.FAQChunkMetadata
		if result.KnowledgeBaseType == types.KnowledgeBaseTypeFAQ {
			meta, err := t.getFAQMetadata(ctx, result.ID, faqMetadataCache)
			if err != nil {
				logger.Warnf(ctx, "[Tool][KnowledgeSearch] Failed to load FAQ metadata for chunk %s: %v", result.ID, err)
			} else {
				faqMeta = meta
			}
		}

		// Group by knowledge base
		if result.KnowledgeID != currentKB {
			currentKB = result.KnowledgeID
			if i > 0 {
				output += "\n"
			}
			output += fmt.Sprintf("[Source Document: %s]\n", result.KnowledgeTitle)
		}

		relevanceLevel := GetRelevanceLevel(result.Score)
		output += fmt.Sprintf("\nResult #%d:\n", i+1)
		output += fmt.Sprintf("  Relevance: %.2f (%s)\n", result.Score, relevanceLevel)
		output += fmt.Sprintf("  Match Type: %s", FormatMatchType(result.MatchType))
		if result.SourceQuery != "" && result.SourceQuery != singleQuery {
			output += fmt.Sprintf(" (Query: \"%s\")", result.SourceQuery)
		}
		output += "\n"
		output += fmt.Sprintf("  Content: %s\n", result.Content)
		output += fmt.Sprintf("  [chunk_id: %s - full content included above]\n", result.ID)

		if faqMeta != nil {
			if faqMeta.StandardQuestion != "" {
				output += fmt.Sprintf("  FAQ Standard Question: %s\n", faqMeta.StandardQuestion)
			}
			if len(faqMeta.SimilarQuestions) > 0 {
				output += fmt.Sprintf("  FAQ Similar Questions: %s\n", strings.Join(faqMeta.SimilarQuestions, "; "))
			}
			if len(faqMeta.Answers) > 0 {
				output += "  FAQ Answers:\n"
				for _, ans := range faqMeta.Answers {
					output += fmt.Sprintf("    - %s\n", ans)
				}
			}
		}

		formattedResults = append(formattedResults, map[string]interface{}{
			"result_index":        i + 1,
			"chunk_id":            result.ID,
			"content":             result.Content,
			"score":               result.Score,
			"relevance_level":     relevanceLevel,
			"knowledge_id":        result.KnowledgeID,
			"knowledge_title":     result.KnowledgeTitle,
			"match_type":          result.MatchType,
			"source_query":        result.SourceQuery,
			"query_type":          result.QueryType,
			"knowledge_base_type": result.KnowledgeBaseType,
		})

		last := formattedResults[len(formattedResults)-1]
		if faqMeta != nil {
			if faqMeta.StandardQuestion != "" {
				last["faq_standard_question"] = faqMeta.StandardQuestion
			}
			if len(faqMeta.SimilarQuestions) > 0 {
				last["faq_similar_questions"] = faqMeta.SimilarQuestions
			}
			if len(faqMeta.Answers) > 0 {
				last["faq_answers"] = faqMeta.Answers
			}
		}
	}

	// // Add usage guidance
	// output += "\n\n=== Usage Guidelines ===\n"
	// output += "- High relevance (>=0.8): directly usable for answering\n"
	// output += "- Medium relevance (0.6-0.8): use as supplementary reference\n"
	// output += "- Low relevance (<0.6): use with caution, may not be accurate\n"
	// if totalBeforeFilter > len(results) {
	// 	output += "- Results below threshold have been automatically filtered\n"
	// }
	// output += "- Full content is already included in search results above\n"
	// output += "- Results are deduplicated across knowledge bases and sorted by relevance\n"
	// output += "- Use get_related_chunks to expand context if needed\n"

	data := map[string]interface{}{
		"knowledge_base_ids": kbsToSearch,
		"results":            formattedResults,
		"count":              len(results),
		"kb_counts":          kbCounts,
		"search_mode":        searchMode,
		"display_type":       "search_results",
	}
	if len(knowledgeIDsFilter) > 0 {
		filterList := make([]string, 0, len(knowledgeIDsFilter))
		for id := range knowledgeIDsFilter {
			filterList = append(filterList, id)
		}
		data["knowledge_ids"] = filterList
	}
	if singleQuery != "" {
		data["query"] = singleQuery
	}
	if len(vectorQueries) > 0 {
		data["vector_queries"] = vectorQueries
	}
	if len(keywordQueries) > 0 {
		data["keyword_queries"] = keywordQueries
	}
	if totalBeforeFilter > len(results) {
		data["total_before_filter"] = totalBeforeFilter
		data["total_after_filter"] = len(results)
	}

	return &types.ToolResult{
		Success: true,
		Output:  output,
		Data:    data,
	}, nil
}
