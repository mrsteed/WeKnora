package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Tencent/WeKnora/internal/config"
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
	knowledgeBaseService interfaces.KnowledgeBaseService
	chunkService         interfaces.ChunkService
	tenantID             uint
	allowedKBs           []string
	rerankModel          rerank.Reranker
	chatModel            chat.Chat      // Optional chat model for LLM-based reranking
	config               *config.Config // Global config for fallback values
}

// NewKnowledgeSearchTool creates a new knowledge search tool
func NewKnowledgeSearchTool(
	knowledgeBaseService interfaces.KnowledgeBaseService,
	chunkService interfaces.ChunkService,
	tenantID uint,
	allowedKBs []string,
	rerankModel rerank.Reranker,
	chatModel chat.Chat,
	cfg *config.Config,
) *KnowledgeSearchTool {
	description := `Search within knowledge bases. Unified tool that supports both targeted and broad searches.

## Features
- Multi-KB search: Search across multiple knowledge bases concurrently

## Usage

**Use when**:
- You know which knowledge bases to target (specify knowledge_base_ids)
- You're unsure which KB contains the info (omit knowledge_base_ids to search all allowed KBs)
- Want to search with multiple queries to get comprehensive results
- Want to filter results from specific documents (use knowledge_ids)

**Returns**: Merged and deduplicated search results from KBs 

## Examples

` + "`" + `
# Simple search in specific KBs
{
  "knowledge_base_ids": ["kb1", "kb2"],
  "queries": ["什么是向量数据库"]
}

# Search all allowed KBs with multiple queries
{
  "queries": ["什么是向量数据库", "向量数据库的应用场景"]
}

# Search specific documents
{
  "knowledge_base_ids": ["kb1"],
  "queries": ["彗星的起源"],
  "knowledge_ids": ["doc1", "doc2"]
}
` + "`" + `

## Tips

- Concurrent search across multiple KBs
- Results are automatically reranked to unify scores from different sources
- Results are merged, deduplicated and sorted by relevance`

	return &KnowledgeSearchTool{
		BaseTool:             NewBaseTool("knowledge_search", description),
		knowledgeBaseService: knowledgeBaseService,
		chunkService:         chunkService,
		tenantID:             tenantID,
		allowedKBs:           allowedKBs,
		rerankModel:          rerankModel,
		chatModel:            chatModel,
		config:               cfg,
	}
}

// Parameters returns the JSON schema for the tool's parameters
func (t *KnowledgeSearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"queries": map[string]interface{}{
				"type":        "array",
				"description": "Array of search queries",
				"items": map[string]interface{}{
					"type": "string",
				},
				"minItems": 1,
				"maxItems": 5,
			},
			"knowledge_base_ids": map[string]interface{}{
				"type":        "array",
				"description": "Array of knowledge base IDs to search in (optional, if omitted searches all allowed KBs)",
				"items": map[string]interface{}{
					"type": "string",
				},
				"minItems": 0,
				"maxItems": 10,
			},
			"knowledge_ids": map[string]interface{}{
				"type":        "array",
				"description": "Optional array of document IDs to filter results (only return results from these specific documents)",
				"items": map[string]interface{}{
					"type": "string",
				},
				"minItems": 0,
				"maxItems": 50,
			},
		},
		"required": []string{"queries"},
	}
}

// Execute executes the knowledge search tool
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

	// Parse query parameter
	var queries []string
	if queriesRaw, ok := args["queries"].([]interface{}); ok && len(queriesRaw) > 0 {
		for _, q := range queriesRaw {
			if qStr, ok := q.(string); ok && qStr != "" {
				queries = append(queries, qStr)
			}
		}
	}

	// Validate: query must be provided
	if len(queries) == 0 {
		logger.Errorf(ctx, "[Tool][KnowledgeSearch] No queries provided")
		return &types.ToolResult{
			Success: false,
			Error:   "queries parameter is required",
		}, fmt.Errorf("no queries provided")
	}

	logger.Infof(ctx, "[Tool][KnowledgeSearch] Queries: %v", queries)

	// Get search parameters from tenant conversation config, fallback to global config
	var topK int
	var vectorThreshold, keywordThreshold, minScore float64

	// Try to get from tenant conversation config
	if tenantVal := ctx.Value(types.TenantInfoContextKey); tenantVal != nil {
		if tenant, ok := tenantVal.(*types.Tenant); ok && tenant != nil && tenant.ConversationConfig != nil {
			cc := tenant.ConversationConfig
			if cc.EmbeddingTopK > 0 {
				topK = cc.EmbeddingTopK
			}
			if cc.VectorThreshold > 0 {
				vectorThreshold = cc.VectorThreshold
			}
			if cc.KeywordThreshold > 0 {
				keywordThreshold = cc.KeywordThreshold
			}
			// minScore is not in ConversationConfig, use default or config
			minScore = 0.3
		}
	}

	// Fallback to global config if not set
	if topK == 0 && t.config != nil {
		topK = t.config.Conversation.EmbeddingTopK
	}
	if vectorThreshold == 0 && t.config != nil {
		vectorThreshold = t.config.Conversation.VectorThreshold
	}
	if keywordThreshold == 0 && t.config != nil {
		keywordThreshold = t.config.Conversation.KeywordThreshold
	}

	// Final fallback to hardcoded defaults if config is not available
	if topK == 0 {
		topK = 5
	}
	if vectorThreshold == 0 {
		vectorThreshold = 0.6
	}
	if keywordThreshold == 0 {
		keywordThreshold = 0.5
	}
	if minScore == 0 {
		minScore = 0.3
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

	// Execute concurrent search (hybrid search handles both vector and keyword)
	logger.Infof(ctx, "[Tool][KnowledgeSearch] Starting concurrent search across %d KBs", len(kbIDs))
	kbTypeMap := t.getKnowledgeBaseTypes(ctx, kbIDs)

	allResults := t.concurrentSearch(ctx, queries, kbIDs,
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
	filteredResults := t.filterByThreshold(allResults, vectorThreshold, keywordThreshold)
	logger.Infof(ctx, "[Tool][KnowledgeSearch] After threshold filter: %d results (from %d)",
		len(filteredResults), len(allResults))

	// Deduplicate before reranking to reduce processing overhead
	deduplicatedBeforeRerank := t.deduplicateResults(filteredResults)
	logger.Infof(ctx, "[Tool][KnowledgeSearch] After deduplication before rerank: %d results (from %d)",
		len(deduplicatedBeforeRerank), len(filteredResults))

	// Apply ReRank if model is configured
	// Prefer chatModel (LLM-based reranking) over rerankModel if both are available
	// Use first query for reranking (or combine all queries if needed)
	rerankQuery := ""
	if len(queries) > 0 {
		rerankQuery = queries[0]
		if len(queries) > 1 {
			// Combine multiple queries for reranking
			rerankQuery = strings.Join(queries, " ")
		}
	}

	if t.chatModel != nil && len(deduplicatedBeforeRerank) > 0 && rerankQuery != "" {
		logger.Infof(ctx, "[Tool][KnowledgeSearch] Applying LLM-based rerank with model: %s, input: %d results, queries: %v",
			t.chatModel.GetModelName(), len(deduplicatedBeforeRerank), queries)
		rerankedResults, err := t.rerankResults(ctx, rerankQuery, deduplicatedBeforeRerank)
		if err != nil {
			logger.Warnf(ctx, "[Tool][KnowledgeSearch] LLM rerank failed, using original results: %v", err)
			filteredResults = deduplicatedBeforeRerank
		} else {
			filteredResults = rerankedResults
			logger.Infof(ctx, "[Tool][KnowledgeSearch] LLM rerank completed successfully: %d results",
				len(filteredResults))
		}
	} else if t.rerankModel != nil && len(deduplicatedBeforeRerank) > 0 && rerankQuery != "" {
		logger.Infof(ctx, "[Tool][KnowledgeSearch] Applying rerank with model: %s, input: %d results, queries: %v",
			t.rerankModel.GetModelName(), len(deduplicatedBeforeRerank), queries)
		rerankedResults, err := t.rerankResults(ctx, rerankQuery, deduplicatedBeforeRerank)
		if err != nil {
			logger.Warnf(ctx, "[Tool][KnowledgeSearch] Rerank failed, using original results: %v", err)
			filteredResults = deduplicatedBeforeRerank
		} else {
			filteredResults = rerankedResults
			logger.Infof(ctx, "[Tool][KnowledgeSearch] Rerank completed successfully: %d results",
				len(filteredResults))
		}
	} else {
		// No reranking, use deduplicated results
		filteredResults = deduplicatedBeforeRerank
	}

	// Apply absolute minimum score filter to remove very low quality chunks
	logger.Debugf(ctx, "[Tool][KnowledgeSearch] Applying min_score filter (%.2f)...", minScore)
	filteredResults = t.filterByMinScore(filteredResults, minScore)
	logger.Infof(ctx, "[Tool][KnowledgeSearch] After min_score filter: %d results", len(filteredResults))

	// Final deduplication after rerank (in case rerank changed scores/order but duplicates remain)
	logger.Debugf(ctx, "[Tool][KnowledgeSearch] Final deduplication after rerank...")
	deduplicatedResults := t.deduplicateResults(filteredResults)
	logger.Infof(ctx, "[Tool][KnowledgeSearch] After final deduplication: %d results (from %d)",
		len(deduplicatedResults), len(filteredResults))

	// Sort results by score (descending)
	sort.Slice(deduplicatedResults, func(i, j int) bool {
		if deduplicatedResults[i].Score != deduplicatedResults[j].Score {
			return deduplicatedResults[i].Score > deduplicatedResults[j].Score
		}
		// If scores are equal, sort by knowledge ID for consistency
		return deduplicatedResults[i].KnowledgeID < deduplicatedResults[j].KnowledgeID
	})

	// Log top results
	if len(deduplicatedResults) > 0 {
		for i := 0; i < len(deduplicatedResults) && i < 5; i++ {
			r := deduplicatedResults[i]
			logger.Infof(ctx, "[Tool][KnowledgeSearch][Top %d] score=%.3f, type=%s, kb=%s, chunk_id=%s",
				i+1, r.Score, r.QueryType, r.KnowledgeID, r.ID)
		}
	}

	// Build output
	logger.Infof(ctx, "[Tool][KnowledgeSearch] Formatting output with %d final results", len(deduplicatedResults))
	result, err := t.formatOutput(ctx, deduplicatedResults, kbIDs, len(allResults), knowledgeIDsFilter, queries)
	if err != nil {
		logger.Errorf(ctx, "[Tool][KnowledgeSearch] Failed to format output: %v", err)
		return result, err
	}
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

		kb, err := t.knowledgeBaseService.GetKnowledgeBaseByID(ctx, kbID)
		if err != nil {
			logger.Warnf(ctx, "[Tool][KnowledgeSearch] Failed to fetch knowledge base %s info: %v", kbID, err)
			continue
		}

		kbTypeMap[kbID] = kb.Type
	}

	return kbTypeMap
}

// concurrentSearch executes hybrid search across multiple KBs concurrently
func (t *KnowledgeSearchTool) concurrentSearch(
	ctx context.Context,
	queries []string,
	kbsToSearch []string,
	topK int,
	vectorThreshold, keywordThreshold float64,
	kbTypeMap map[string]string,
) []*searchResultWithMeta {
	var wg sync.WaitGroup
	var mu sync.Mutex
	allResults := make([]*searchResultWithMeta, 0)

	for _, query := range queries {
		// Capture query in local variable to avoid closure issues
		q := query
		for _, kbID := range kbsToSearch {
			// Capture kbID in local variable to avoid closure issues
			kb := kbID
			wg.Add(1)
			go func() {
				defer wg.Done()
				searchParams := types.SearchParams{
					QueryText:        q,
					MatchCount:       topK,
					VectorThreshold:  vectorThreshold,
					KeywordThreshold: keywordThreshold,
				}
				kbResults, err := t.knowledgeBaseService.HybridSearch(ctx, kb, searchParams)
				if err != nil {
					// Log error but continue with other KBs
					logger.Warnf(ctx, "[Tool][KnowledgeSearch] Failed to search knowledge base %s: %v", kb, err)
					return
				}

				// Wrap results with metadata
				mu.Lock()
				for _, r := range kbResults {
					allResults = append(allResults, &searchResultWithMeta{
						SearchResult:      r,
						SourceQuery:       q,
						QueryType:         "hybrid", // Hybrid search combines both vector and keyword
						KnowledgeBaseID:   kb,
						KnowledgeBaseType: kbTypeMap[kb],
					})
				}
				mu.Unlock()
			}()
		}
	}
	wg.Wait()
	return allResults
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
	kbsToSearch []string,
	totalBeforeFilter int,
	knowledgeIDsFilter map[string]bool,
	queries []string,
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
		if len(queries) > 0 {
			data["queries"] = queries
		}
		return &types.ToolResult{
			Success: true,
			Output:  fmt.Sprintf("No relevant content found in %d knowledge base(s).", len(kbsToSearch)),
			Data:    data,
		}, nil
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
	if len(queries) == 1 {
		output += fmt.Sprintf("Query: %s\n", queries[0])
	} else if len(queries) > 1 {
		output += fmt.Sprintf("Queries (%d): %v\n", len(queries), queries)
	}

	output += fmt.Sprintf("Found %d relevant results", len(results))
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

		// relevanceLevel := GetRelevanceLevel(result.Score)
		output += fmt.Sprintf("\nResult #%d:\n", i+1)
		output += fmt.Sprintf("  [chunk_id: %s][chunk_index: %d]\nContent: %s\n", result.ID, result.ChunkIndex, result.Content)

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
					output += fmt.Sprintf("    Answer Choice %d: %s\n", i+1, ans)
				}
			}
		}

		formattedResults = append(formattedResults, map[string]interface{}{
			"result_index": i + 1,
			"chunk_id":     result.ID,
			"content":      result.Content,
			// "score":        result.Score,
			// "relevance_level":     relevanceLevel,
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
	// output += "- Use list_knowledge_chunks to expand context if needed\n"

	data := map[string]interface{}{
		"knowledge_base_ids": kbsToSearch,
		"results":            formattedResults,
		"count":              len(results),
		"kb_counts":          kbCounts,
		"display_type":       "search_results",
	}
	if len(knowledgeIDsFilter) > 0 {
		filterList := make([]string, 0, len(knowledgeIDsFilter))
		for id := range knowledgeIDsFilter {
			filterList = append(filterList, id)
		}
		data["knowledge_ids"] = filterList
	}
	if len(queries) > 0 {
		data["queries"] = queries
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
