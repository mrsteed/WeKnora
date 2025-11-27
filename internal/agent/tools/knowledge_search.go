package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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
	tenantID             uint64
	allowedKBs           []string
	rerankModel          rerank.Reranker
	chatModel            chat.Chat      // Optional chat model for LLM-based reranking
	config               *config.Config // Global config for fallback values
}

// NewKnowledgeSearchTool creates a new knowledge search tool
func NewKnowledgeSearchTool(
	knowledgeBaseService interfaces.KnowledgeBaseService,
	chunkService interfaces.ChunkService,
	tenantID uint64,
	allowedKBs []string,
	rerankModel rerank.Reranker,
	chatModel chat.Chat,
	cfg *config.Config,
) *KnowledgeSearchTool {
	description := `Semantic/vector search tool for retrieving knowledge by meaning, intent, and conceptual relevance.

This tool uses embeddings to understand the user's query and find semantically similar content across knowledge base chunks.

## Purpose
Designed for high-level understanding tasks, such as:
- conceptual explanations
- topic overviews
- reasoning-based information needs
- contextual or intent-driven retrieval
- queries that cannot be answered with literal keyword matching

The tool searches by MEANING rather than exact text. It identifies chunks that are conceptually relevant even when the wording differs.

## What the Tool Does NOT Do
- Does NOT perform exact keyword matching
- Does NOT search for specific named entities
- Should NOT be used for literal lookup tasks
- Should NOT receive long raw text or user messages as queries
- Should NOT be used to locate specific strings or error codes

For literal/keyword/entity search, another tool should be used.

## Required Input Behavior
"queries" must contain **1–5 short, well-formed semantic questions or conceptual statements** that clearly express the meaning the model is trying to retrieve.

Each query should represent a **concept, idea, topic, explanation, or intent**, such as:
- abstract topics
- definitions
- mechanisms
- best practices
- comparisons
- how/why questions

Avoid:
- keyword lists
- raw text from user messages
- full paragraphs
- unprocessed input

## Examples of valid query shapes (not content):
- "What is the main idea of..."
- "How does X work in general?"
- "Explain the purpose of..."
- "What are the key principles behind..."
- "Overview of ..."

## Parameters
- queries (required): 1–5 semantic questions or conceptual statements.
  These should reflect the meaning or topic you want embeddings to capture.
- knowledge_base_ids (optional): limit the search scope.

## Output
Returns chunks ranked by semantic similarity, reranked when applicable.  
Results represent conceptual relevance, not literal keyword overlap.
`

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
				"description": "REQUIRED: 1-5 semantic questions/topics (e.g., ['What is RAG?', 'RAG benefits'])",
				"items": map[string]interface{}{
					"type": "string",
				},
				"minItems": 1,
				"maxItems": 5,
			},
			"knowledge_base_ids": map[string]interface{}{
				"type":        "array",
				"description": "Optional: KB IDs to search",
				"items": map[string]interface{}{
					"type": "string",
				},
				"minItems": 0,
				"maxItems": 10,
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

	// Execute concurrent search (hybrid search handles both vector and keyword)
	logger.Infof(ctx, "[Tool][KnowledgeSearch] Starting concurrent search across %d KBs", len(kbIDs))
	kbTypeMap := t.getKnowledgeBaseTypes(ctx, kbIDs)

	allResults := t.concurrentSearch(ctx, queries, kbIDs,
		topK, vectorThreshold, keywordThreshold, kbTypeMap)
	logger.Infof(ctx, "[Tool][KnowledgeSearch] Concurrent search completed: %d raw results", len(allResults))

	// Normalize keyword search results to ensure fair comparison across knowledge bases
	logger.Debugf(ctx, "[Tool][KnowledgeSearch] Normalizing keyword search results...")
	t.normalizeKeywordSearchResults(ctx, allResults)
	logger.Infof(ctx, "[Tool][KnowledgeSearch] After keyword normalization: %d results", len(allResults))

	// Filter by threshold first
	filteredResults := t.filterByThreshold(allResults, vectorThreshold, keywordThreshold)
	// Deduplicate before reranking to reduce processing overhead
	deduplicatedBeforeRerank := t.deduplicateResults(filteredResults)

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

	// Apply MMR (Maximal Marginal Relevance) to reduce redundancy and improve diversity
	// Note: composite scoring is already applied inside rerankResults
	if len(filteredResults) > 0 {
		// Calculate k for MMR: use min(len(results), max(1, topK))
		mmrK := len(filteredResults)
		if topK > 0 && mmrK > topK {
			mmrK = topK
		}
		if mmrK < 1 {
			mmrK = 1
		}
		// Apply MMR with lambda=0.7 (balance between relevance and diversity)
		logger.Debugf(ctx, "[Tool][KnowledgeSearch] Applying MMR: k=%d, lambda=0.7, input=%d results", mmrK, len(filteredResults))
		mmrResults := t.applyMMR(ctx, filteredResults, mmrK, 0.7)
		if len(mmrResults) > 0 {
			filteredResults = mmrResults
			logger.Infof(ctx, "[Tool][KnowledgeSearch] MMR completed: %d results selected", len(filteredResults))
		} else {
			logger.Warnf(ctx, "[Tool][KnowledgeSearch] MMR returned no results, using original results")
		}
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
	result, err := t.formatOutput(ctx, deduplicatedResults, kbIDs, queries)
	if err != nil {
		logger.Errorf(ctx, "[Tool][KnowledgeSearch] Failed to format output: %v", err)
		return result, err
	}
	logger.Infof(ctx, "[Tool][KnowledgeSearch] Output: %s", result.Output)
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
// Special handling for history matches: uses lower threshold (reduced by 0.1, minimum 0.5)
func (t *KnowledgeSearchTool) filterByThreshold(
	results []*searchResultWithMeta,
	vectorThreshold, keywordThreshold float64,
) []*searchResultWithMeta {
	filtered := make([]*searchResultWithMeta, 0)
	for _, r := range results {
		var threshold float64

		// Special handling for history matches: use lower threshold
		if r.MatchType == types.MatchTypeHistory {
			// Use the lower of the two thresholds, then reduce by 0.1 (minimum 0.5)
			th := vectorThreshold
			if keywordThreshold < th {
				th = keywordThreshold
			}
			threshold = math.Max(th-0.1, 0.5)
		} else if r.MatchType == types.MatchTypeEmbedding {
			threshold = vectorThreshold
		} else if r.MatchType == types.MatchTypeKeywords {
			threshold = keywordThreshold
		} else {
			// For other match types (graph, nearby chunk, etc.), use the lower threshold
			threshold = vectorThreshold
			if keywordThreshold < threshold {
				threshold = keywordThreshold
			}
		}

		// Check if result meets threshold
		if r.Score >= threshold {
			filtered = append(filtered, r)
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
	// Try rerankModel first, fallback to chatModel if rerankModel fails or returns no results
	if t.rerankModel != nil {
		rerankedNonFAQ, err = t.rerankWithModel(ctx, query, nonFAQResults)
		// If rerankModel fails or returns no results, fallback to chatModel
		if err != nil || len(rerankedNonFAQ) == 0 {
			if err != nil {
				logger.Warnf(ctx, "[Tool][KnowledgeSearch] Rerank model failed, falling back to chat model: %v", err)
			} else {
				logger.Warnf(ctx, "[Tool][KnowledgeSearch] Rerank model returned no results, falling back to chat model")
			}
			// Reset error to allow fallback
			err = nil
			// Try chatModel if available
			if t.chatModel != nil {
				rerankedNonFAQ, err = t.rerankWithLLM(ctx, query, nonFAQResults)
			} else {
				// No fallback available, use original results
				rerankedNonFAQ = nonFAQResults
			}
		}
	} else if t.chatModel != nil {
		// No rerankModel, use chatModel directly
		rerankedNonFAQ, err = t.rerankWithLLM(ctx, query, nonFAQResults)
	} else {
		// No reranking available, use original results
		rerankedNonFAQ = nonFAQResults
	}

	if err != nil {
		return nil, err
	}

	// Apply composite scoring to reranked results
	// Get query intent from context if available (optional)
	queryIntent := t.getQueryIntentFromContext(ctx)
	logger.Debugf(ctx, "[Tool][KnowledgeSearch] Applying composite scoring with query_intent=%s", queryIntent)

	// Store base scores before composite scoring
	for _, result := range rerankedNonFAQ {
		baseScore := result.Score
		// Apply composite score
		result.Score = t.compositeScore(result, result.Score, baseScore, queryIntent)
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
			// Get enriched passage (content + image info)
			enrichedContent := t.getEnrichedPassage(ctx, result.SearchResult)
			// Truncate content if too long to save tokens
			content := enrichedContent
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
	// Prepare passages for reranking (with enriched content including image info)
	passages := make([]string, len(results))
	for i, result := range results {
		passages[i] = t.getEnrichedPassage(ctx, result.SearchResult)
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
// Uses multiple keys (ID, parent chunk ID, knowledge+index) and content signature for deduplication
func (t *KnowledgeSearchTool) deduplicateResults(results []*searchResultWithMeta) []*searchResultWithMeta {
	seen := make(map[string]bool)
	contentSig := make(map[string]bool)
	uniqueResults := make([]*searchResultWithMeta, 0)

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

	// If we have duplicates by ID but different scores, keep the highest score
	// This handles cases where the same chunk appears multiple times with different scores
	seenByID := make(map[string]*searchResultWithMeta)
	for _, r := range uniqueResults {
		if existing, ok := seenByID[r.ID]; ok {
			// Keep the result with higher score
			if r.Score > existing.Score {
				seenByID[r.ID] = r
			}
		} else {
			seenByID[r.ID] = r
		}
	}

	// Convert back to slice
	deduplicated := make([]*searchResultWithMeta, 0, len(seenByID))
	for _, r := range seenByID {
		deduplicated = append(deduplicated, r)
	}

	return deduplicated
}

// buildContentSignature creates a normalized signature for content to detect near-duplicates
func (t *KnowledgeSearchTool) buildContentSignature(content string) string {
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

// formatOutput formats the search results for display
func (t *KnowledgeSearchTool) formatOutput(
	ctx context.Context,
	results []*searchResultWithMeta,
	kbsToSearch []string,
	queries []string,
) (*types.ToolResult, error) {
	if len(results) == 0 {
		data := map[string]interface{}{
			"knowledge_base_ids": kbsToSearch,
			"results":            []interface{}{},
			"count":              0,
		}
		if len(queries) > 0 {
			data["queries"] = queries
		}
		output := fmt.Sprintf("No relevant content found in %d knowledge base(s).\n\n", len(kbsToSearch))
		output += "=== ⚠️ CRITICAL - Next Steps ===\n"
		output += "- ❌ DO NOT use training data or general knowledge to answer\n"
		output += "- ✅ If web_search is enabled: You MUST use web_search to find information\n"
		output += "- ✅ If web_search is disabled: State 'I couldn't find relevant information in the knowledge base'\n"
		output += "- NEVER fabricate or infer answers - ONLY use retrieved content\n"

		return &types.ToolResult{
			Success: true,
			Output:  output,
			Data:    data,
		}, nil
	}

	// Build output header
	output := "=== Search Results ===\n"
	output += fmt.Sprintf("Found %d relevant results", len(results))
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

	// Track chunks per knowledge for statistics
	knowledgeChunkMap := make(map[string]map[int]bool) // knowledge_id -> set of chunk_index
	knowledgeTotalMap := make(map[string]int64)        // knowledge_id -> total chunks
	knowledgeTitleMap := make(map[string]string)       // knowledge_id -> title

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

		// Track chunk indices per knowledge
		if knowledgeChunkMap[result.KnowledgeID] == nil {
			knowledgeChunkMap[result.KnowledgeID] = make(map[int]bool)
		}
		knowledgeChunkMap[result.KnowledgeID][result.ChunkIndex] = true
		knowledgeTitleMap[result.KnowledgeID] = result.KnowledgeTitle

		// Group by knowledge base
		if result.KnowledgeID != currentKB {
			currentKB = result.KnowledgeID
			if i > 0 {
				output += "\n"
			}
			output += fmt.Sprintf("[Source Document: %s]\n", result.KnowledgeTitle)

			// Get total chunk count for this knowledge (cache it)
			if _, exists := knowledgeTotalMap[result.KnowledgeID]; !exists {
				_, total, err := t.chunkService.GetRepository().ListPagedChunksByKnowledgeID(ctx,
					t.tenantID, result.KnowledgeID,
					&types.Pagination{Page: 1, PageSize: 1},
					[]types.ChunkType{types.ChunkTypeText}, "")
				if err != nil {
					logger.Warnf(ctx, "[Tool][KnowledgeSearch] Failed to get total chunks for knowledge %s: %v", result.KnowledgeID, err)
					knowledgeTotalMap[result.KnowledgeID] = 0
				} else {
					knowledgeTotalMap[result.KnowledgeID] = total
				}
			}
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
				for ansIdx, ans := range faqMeta.Answers {
					output += fmt.Sprintf("    Answer Choice %d: %s\n", ansIdx+1, ans)
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

	// Add statistics and recommendations for each knowledge
	output += "\n=== 检索统计与建议 ===\n\n"
	for knowledgeID, retrievedChunks := range knowledgeChunkMap {
		totalChunks := knowledgeTotalMap[knowledgeID]
		retrievedCount := len(retrievedChunks)
		title := knowledgeTitleMap[knowledgeID]

		if totalChunks > 0 {
			percentage := float64(retrievedCount) / float64(totalChunks) * 100
			remaining := totalChunks - int64(retrievedCount)

			output += fmt.Sprintf("文档: %s (%s)\n", title, knowledgeID)
			output += fmt.Sprintf("  总 Chunk 数: %d\n", totalChunks)
			output += fmt.Sprintf("  已召回: %d 个 (%.1f%%)\n", retrievedCount, percentage)
			output += fmt.Sprintf("  未召回: %d 个\n", remaining)

			if remaining > 0 {
				output += "  建议: 使用 list_knowledge_chunks 工具获取完整内容\n"

				// Find missing chunk ranges (gaps in retrieved chunks)
				missingRanges := t.findMissingChunkRanges(retrievedChunks, int(totalChunks))

				if len(missingRanges) == 0 {
					// No gaps found (shouldn't happen if remaining > 0, but handle it)
					output += fmt.Sprintf("    - 获取全部内容: list_knowledge_chunks(knowledge_id=\"%s\", offset=0, limit=%d)\n", knowledgeID, totalChunks)
				} else if len(missingRanges) == 1 && missingRanges[0].start == 0 && missingRanges[0].end == int(totalChunks)-1 {
					// All chunks are missing (shouldn't happen, but handle it)
					output += fmt.Sprintf("    - 获取全部内容: list_knowledge_chunks(knowledge_id=\"%s\", offset=0, limit=%d)\n", knowledgeID, totalChunks)
				} else {
					// Suggest getting each missing range
					for idx, r := range missingRanges {
						rangeSize := r.end - r.start + 1
						if rangeSize <= 100 {
							// Small range, get all at once
							output += fmt.Sprintf("    - 区间 %d: chunk_index %d-%d (%d 个) → list_knowledge_chunks(knowledge_id=\"%s\", offset=%d, limit=%d)\n",
								idx+1, r.start, r.end, rangeSize, knowledgeID, r.start, rangeSize)
						} else {
							// Large range, suggest getting in batches
							output += fmt.Sprintf("    - 区间 %d: chunk_index %d-%d (%d 个，建议分批获取):\n",
								idx+1, r.start, r.end, rangeSize)
							output += fmt.Sprintf("      首次: list_knowledge_chunks(knowledge_id=\"%s\", offset=%d, limit=100)\n",
								knowledgeID, r.start)
							if rangeSize > 100 {
								output += "      继续: 根据返回结果调整 offset 继续获取剩余内容\n"
							}
						}
					}
				}
			}
			output += "\n"
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
		"count":              len(formattedResults),
		"kb_counts":          kbCounts,
		"display_type":       "search_results",
	}

	if len(queries) > 0 {
		data["queries"] = queries
	}

	return &types.ToolResult{
		Success: true,
		Output:  output,
		Data:    data,
	}, nil
}

// chunkRange represents a continuous range of chunk indices
type chunkRange struct {
	start int
	end   int
}

// findMissingChunkRanges finds all continuous ranges of missing chunks
// retrievedChunks is a set of retrieved chunk indices
// totalChunks is the total number of chunks
func (t *KnowledgeSearchTool) findMissingChunkRanges(retrievedChunks map[int]bool, totalChunks int) []chunkRange {
	if totalChunks <= 0 {
		return nil
	}

	var ranges []chunkRange
	var currentStart int = -1

	// Iterate through all possible chunk indices (0 to totalChunks-1)
	for i := 0; i < totalChunks; i++ {
		if !retrievedChunks[i] {
			// This chunk is missing
			if currentStart == -1 {
				// Start of a new missing range
				currentStart = i
			}
		} else {
			// This chunk is retrieved
			if currentStart != -1 {
				// End of a missing range
				ranges = append(ranges, chunkRange{
					start: currentStart,
					end:   i - 1,
				})
				currentStart = -1
			}
		}
	}

	// Handle case where missing chunks extend to the end
	if currentStart != -1 {
		ranges = append(ranges, chunkRange{
			start: currentStart,
			end:   totalChunks - 1,
		})
	}

	return ranges
}

// normalizeKeywordSearchResults normalizes keyword search result scores into [0,1] globally across all knowledge bases
// Improvements:
// 1. Uses robust normalization with percentile-based bounds to handle outliers
// 2. Handles edge cases: single result, no variance, negative scores
// 3. Global normalization ensures fair comparison across different knowledge bases
func (t *KnowledgeSearchTool) normalizeKeywordSearchResults(ctx context.Context, results []*searchResultWithMeta) {
	// Filter keyword match results
	keywordResults := make([]*searchResultWithMeta, 0)
	for _, result := range results {
		if result.MatchType == types.MatchTypeKeywords {
			keywordResults = append(keywordResults, result)
		}
	}

	if len(keywordResults) == 0 {
		return
	}

	// Single result: set to 1.0
	if len(keywordResults) == 1 {
		keywordResults[0].Score = 1.0
		return
	}

	// Find min and max scores globally
	minS := keywordResults[0].Score
	maxS := keywordResults[0].Score
	for _, r := range keywordResults {
		if r.Score < minS {
			minS = r.Score
		}
		if r.Score > maxS {
			maxS = r.Score
		}
	}

	// No variance: all scores are the same
	if maxS <= minS {
		for _, r := range keywordResults {
			r.Score = 1.0
		}
		logger.Infof(ctx, "[Tool][KnowledgeSearch] Keyword scores have no variance, all set to 1.0: count=%d, score=%.3f",
			len(keywordResults), minS)
		return
	}

	// Robust normalization: use percentile-based bounds to reduce outlier impact
	// For small groups, use min/max; for larger groups, use 5th and 95th percentiles
	normalizeMin := minS
	normalizeMax := maxS

	if len(keywordResults) >= 10 {
		// For larger groups, use percentile-based bounds to handle outliers
		// Sort scores to find percentiles
		scores := make([]float64, len(keywordResults))
		for i, r := range keywordResults {
			scores[i] = r.Score
		}
		sort.Float64s(scores)

		// Use 5th and 95th percentiles to reduce outlier impact
		p5Idx := len(scores) * 5 / 100
		p95Idx := len(scores) * 95 / 100
		if p5Idx < len(scores) {
			normalizeMin = scores[p5Idx]
		}
		if p95Idx < len(scores) {
			normalizeMax = scores[p95Idx]
		}
	}

	// Normalize scores with bounds checking
	rangeSize := normalizeMax - normalizeMin
	if rangeSize > 0 {
		for _, r := range keywordResults {
			// Clamp to [normalizeMin, normalizeMax] before normalization
			clampedScore := r.Score
			if clampedScore < normalizeMin {
				clampedScore = normalizeMin
			} else if clampedScore > normalizeMax {
				clampedScore = normalizeMax
			}

			// Normalize to [0, 1]
			ns := (clampedScore - normalizeMin) / rangeSize
			if ns < 0 {
				ns = 0
			} else if ns > 1 {
				ns = 1
			}
			r.Score = ns
		}

		logger.Infof(ctx, "[Tool][KnowledgeSearch] Normalized keyword scores: count=%d, raw_min=%.3f, raw_max=%.3f, normalize_min=%.3f, normalize_max=%.3f",
			len(keywordResults), minS, maxS, normalizeMin, normalizeMax)
	} else {
		// Fallback: all scores are the same after percentile filtering
		for _, r := range keywordResults {
			r.Score = 1.0
		}
	}
}

// getEnrichedPassage 合并Content和ImageInfo的文本内容
func (t *KnowledgeSearchTool) getEnrichedPassage(ctx context.Context, result *types.SearchResult) string {
	if result.ImageInfo == "" {
		return result.Content
	}

	// 解析ImageInfo
	var imageInfos []types.ImageInfo
	err := json.Unmarshal([]byte(result.ImageInfo), &imageInfos)
	if err != nil {
		logger.Warnf(ctx, "[Tool][KnowledgeSearch] Failed to parse image info: %v", err)
		return result.Content
	}

	if len(imageInfos) == 0 {
		return result.Content
	}

	// 提取所有图片的描述和OCR文本
	var imageTexts []string
	for _, img := range imageInfos {
		if img.Caption != "" {
			imageTexts = append(imageTexts, fmt.Sprintf("图片描述: %s", img.Caption))
		}
		if img.OCRText != "" {
			imageTexts = append(imageTexts, fmt.Sprintf("图片文本: %s", img.OCRText))
		}
	}

	if len(imageTexts) == 0 {
		return result.Content
	}

	// 组合内容和图片信息
	combinedText := result.Content
	if combinedText != "" {
		combinedText += "\n\n"
	}
	combinedText += strings.Join(imageTexts, "\n")

	logger.Debugf(ctx, "[Tool][KnowledgeSearch] Enriched passage: content_len=%d, image_texts=%d",
		len(result.Content), len(imageTexts))

	return combinedText
}

// getQueryIntentFromContext attempts to extract query intent from context (optional)
func (t *KnowledgeSearchTool) getQueryIntentFromContext(ctx context.Context) string {
	// Try to get query intent from context if available
	// This is optional and may not always be present in agent tool context
	// Return empty string if not available
	return ""
}

// compositeScore calculates a composite score considering multiple factors
func (t *KnowledgeSearchTool) compositeScore(
	result *searchResultWithMeta,
	modelScore, baseScore float64,
	queryIntent string,
) float64 {
	// Source weight: web_search results get slightly lower weight
	sourceWeight := 1.0
	if strings.ToLower(result.KnowledgeSource) == "web_search" {
		sourceWeight = 0.95
	}

	// Intent boost: adjust score based on query intent and chunk characteristics
	intentBoost := 1.0
	if queryIntent != "" {
		switch queryIntent {
		case "definition":
			// Boost summary chunks for definition queries
			if result.ChunkType == string(types.ChunkTypeSummary) {
				intentBoost = 1.05
			}
		case "howto":
			// Boost longer chunks for howto queries
			if result.EndAt-result.StartAt > 300 {
				intentBoost = 1.03
			}
		case "compare":
			// No boost for compare queries
			intentBoost = 1.0
		}
	}

	// Position prior: slightly favor chunks earlier in the document
	positionPrior := 1.0
	if result.StartAt >= 0 && result.EndAt > result.StartAt {
		// Calculate position ratio and apply small boost for earlier positions
		positionRatio := 1.0 - float64(result.StartAt)/float64(result.EndAt+1)
		positionPrior += t.clampFloat(positionRatio, -0.05, 0.05)
	}

	// Composite formula: weighted combination of model score, base score, and source weight
	composite := 0.6*modelScore + 0.3*baseScore + 0.1*sourceWeight
	composite *= intentBoost
	composite *= positionPrior

	// Clamp to [0, 1]
	if composite < 0 {
		composite = 0
	}
	if composite > 1 {
		composite = 1
	}

	return composite
}

// clampFloat clamps a float value to the specified range
func (t *KnowledgeSearchTool) clampFloat(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

// applyMMR applies Maximal Marginal Relevance algorithm to reduce redundancy
func (t *KnowledgeSearchTool) applyMMR(
	ctx context.Context,
	results []*searchResultWithMeta,
	k int,
	lambda float64,
) []*searchResultWithMeta {
	if k <= 0 || len(results) == 0 {
		return nil
	}

	logger.Infof(ctx, "[Tool][KnowledgeSearch] Applying MMR: lambda=%.2f, k=%d, candidates=%d",
		lambda, k, len(results))

	selected := make([]*searchResultWithMeta, 0, k)
	candidates := make([]*searchResultWithMeta, len(results))
	copy(candidates, results)

	// Pre-compute token sets for all candidates
	tokenSets := make([]map[string]struct{}, len(candidates))
	for i, r := range candidates {
		tokenSets[i] = t.tokenizeSimple(t.getEnrichedPassage(ctx, r.SearchResult))
	}

	// MMR selection loop
	for len(selected) < k && len(candidates) > 0 {
		bestIdx := 0
		bestScore := -1.0

		for i, r := range candidates {
			relevance := r.Score
			redundancy := 0.0

			// Calculate maximum redundancy with already selected results
			for _, s := range selected {
				selectedTokens := t.tokenizeSimple(t.getEnrichedPassage(ctx, s.SearchResult))
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
				si := t.tokenizeSimple(t.getEnrichedPassage(ctx, selected[i].SearchResult))
				sj := t.tokenizeSimple(t.getEnrichedPassage(ctx, selected[j].SearchResult))
				avgRed += t.jaccard(si, sj)
				pairs++
			}
		}
		if pairs > 0 {
			avgRed /= float64(pairs)
		}
	}

	logger.Infof(ctx, "[Tool][KnowledgeSearch] MMR completed: selected=%d, avg_redundancy=%.4f",
		len(selected), avgRed)

	return selected
}

// tokenizeSimple tokenizes text into a set of words (simple whitespace-based)
func (t *KnowledgeSearchTool) tokenizeSimple(text string) map[string]struct{} {
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
func (t *KnowledgeSearchTool) jaccard(a, b map[string]struct{}) float64 {
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
