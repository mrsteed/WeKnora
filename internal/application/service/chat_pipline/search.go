package chatpipline

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// PluginSearch implements search functionality for chat pipeline
type PluginSearch struct {
	knowledgeBaseService interfaces.KnowledgeBaseService
	knowledgeService     interfaces.KnowledgeService
	modelService         interfaces.ModelService
	config               *config.Config
	webSearchService     interfaces.WebSearchService
	tenantService        interfaces.TenantService
	sessionService       interfaces.SessionService
}

func NewPluginSearch(eventManager *EventManager,
	knowledgeBaseService interfaces.KnowledgeBaseService,
	knowledgeService interfaces.KnowledgeService,
	modelService interfaces.ModelService,
	config *config.Config,
	webSearchService interfaces.WebSearchService,
	tenantService interfaces.TenantService,
	sessionService interfaces.SessionService,
) *PluginSearch {
	res := &PluginSearch{
		knowledgeBaseService: knowledgeBaseService,
		knowledgeService:     knowledgeService,
		modelService:         modelService,
		config:               config,
		webSearchService:     webSearchService,
		tenantService:        tenantService,
		sessionService:       sessionService,
	}
	eventManager.Register(res)
	return res
}

// ActivationEvents returns the event types this plugin handles
func (p *PluginSearch) ActivationEvents() []types.EventType {
	return []types.EventType{types.CHUNK_SEARCH}
}

// OnEvent handles search events in the chat pipeline
func (p *PluginSearch) OnEvent(ctx context.Context,
	eventType types.EventType, chatManage *types.ChatManage, next func() *PluginError,
) *PluginError {
	// Get knowledge base IDs list
	knowledgeBaseIDs := chatManage.KnowledgeBaseIDs
	if len(knowledgeBaseIDs) == 0 && chatManage.KnowledgeBaseID != "" {
		// Fall back to single knowledge base
		knowledgeBaseIDs = []string{chatManage.KnowledgeBaseID}
		pipelineInfo(ctx, "Search", "fallback_kb", map[string]interface{}{
			"session_id": chatManage.SessionID,
			"kb_id":      chatManage.KnowledgeBaseID,
		})
	}

	if len(knowledgeBaseIDs) == 0 {
		pipelineError(ctx, "Search", "kb_not_found", map[string]interface{}{
			"session_id": chatManage.SessionID,
		})
		return ErrSearch.WithError(nil)
	}

	pipelineInfo(ctx, "Search", "input", map[string]interface{}{
		"session_id":      chatManage.SessionID,
		"rewrite_query":   chatManage.RewriteQuery,
		"processed_query": chatManage.ProcessedQuery,
		"kb_ids":          strings.Join(knowledgeBaseIDs, ","),
		"tenant_id":       chatManage.TenantID,
		"web_enabled":     chatManage.WebSearchEnabled,
	})

	// Run KB search and web search concurrently
	pipelineInfo(ctx, "Search", "plan", map[string]interface{}{
		"kb_count":          len(knowledgeBaseIDs),
		"embedding_top_k":   chatManage.EmbeddingTopK,
		"vector_threshold":  chatManage.VectorThreshold,
		"keyword_threshold": chatManage.KeywordThreshold,
	})
	var wg sync.WaitGroup
	var mu sync.Mutex
	allResults := make([]*types.SearchResult, 0)

	wg.Add(2)
	// Goroutine 1: Knowledge base search (rewrite + processed)
	go func() {
		defer wg.Done()
		kbResults := p.searchKnowledgeBases(ctx, knowledgeBaseIDs, chatManage)
		if len(kbResults) > 0 {
			mu.Lock()
			allResults = append(allResults, kbResults...)
			mu.Unlock()
		}
	}()

	// Goroutine 2: Web search (if enabled)
	go func() {
		defer wg.Done()
		webResults := p.searchWebIfEnabled(ctx, chatManage)
		if len(webResults) > 0 {
			mu.Lock()
			allResults = append(allResults, webResults...)
			mu.Unlock()
		}
	}()

	wg.Wait()

	chatManage.SearchResult = allResults

	// If recall is low, attempt query expansion with keyword-focused search
	if chatManage.EnableQueryExpansion && len(chatManage.SearchResult) < max(1, chatManage.EmbeddingTopK/2) {
		pipelineInfo(ctx, "Search", "recall_low", map[string]interface{}{
			"current":   len(chatManage.SearchResult),
			"threshold": chatManage.EmbeddingTopK / 2,
		})
		expansions := p.expandQueries(ctx, chatManage)
		if len(expansions) > 0 {
			pipelineInfo(ctx, "Search", "expansion_start", map[string]interface{}{
				"variants": len(expansions),
			})
			expTopK := max(chatManage.EmbeddingTopK*2, chatManage.RerankTopK*2)
			expKwTh := chatManage.KeywordThreshold * 0.8
			// Concurrent expansion retrieval across queries and KBs
			expResults := make([]*types.SearchResult, 0, expTopK*len(expansions))
			var muExp sync.Mutex
			var wgExp sync.WaitGroup
			jobs := len(expansions) * len(knowledgeBaseIDs)
			capSem := 16
			if jobs < capSem {
				capSem = jobs
			}
			if capSem <= 0 {
				capSem = 1
			}
			sem := make(chan struct{}, capSem)
			pipelineInfo(ctx, "Search", "expansion_concurrency", map[string]interface{}{
				"jobs": jobs,
				"cap":  capSem,
			})
			for _, q := range expansions {
				for _, kbID := range knowledgeBaseIDs {
					wgExp.Add(1)
					go func(q string, kbID string) {
						defer wgExp.Done()
						sem <- struct{}{}
						defer func() { <-sem }()
						paramsExp := types.SearchParams{
							QueryText:            q,
							VectorThreshold:      chatManage.VectorThreshold,
							KeywordThreshold:     expKwTh,
							MatchCount:           expTopK,
							DisableVectorMatch:   true,
							DisableKeywordsMatch: false,
						}
						res, err := p.knowledgeBaseService.HybridSearch(ctx, kbID, paramsExp)
						if err != nil {
							pipelineWarn(ctx, "Search", "expansion_error", map[string]interface{}{
								"kb_id": kbID,
								"error": err.Error(),
							})
							return
						}
						if len(res) > 0 {
							pipelineInfo(ctx, "Search", "expansion_hits", map[string]interface{}{
								"kb_id": kbID,
								"query": q,
								"hits":  len(res),
							})
							muExp.Lock()
							expResults = append(expResults, res...)
							muExp.Unlock()
						}
					}(q, kbID)
				}
			}
			wgExp.Wait()
			if len(expResults) > 0 {
				pipelineInfo(ctx, "Search", "expansion_done", map[string]interface{}{
					"added": len(expResults),
				})
				chatManage.SearchResult = append(chatManage.SearchResult, expResults...)
			}
		}
	}

	// Add relevant results from chat history
	historyResult := p.getSearchResultFromHistory(chatManage)
	if historyResult != nil {
		pipelineInfo(ctx, "Search", "history_hits", map[string]interface{}{
			"session_id":   chatManage.SessionID,
			"history_hits": len(historyResult),
		})
		chatManage.SearchResult = append(chatManage.SearchResult, historyResult...)
	}

	// Remove duplicate results
	before := len(chatManage.SearchResult)
	chatManage.SearchResult = removeDuplicateResults(chatManage.SearchResult)
	pipelineInfo(ctx, "Search", "dedup_summary", map[string]interface{}{
		"before": before,
		"after":  len(chatManage.SearchResult),
	})

	// Return if we have results
	if len(chatManage.SearchResult) != 0 {
		pipelineInfo(ctx, "Search", "output", map[string]interface{}{
			"session_id":   chatManage.SessionID,
			"result_count": len(chatManage.SearchResult),
		})
		return next()
	}
	pipelineWarn(ctx, "Search", "output", map[string]interface{}{
		"session_id":   chatManage.SessionID,
		"result_count": 0,
	})
	return ErrSearchNothing
}

// getSearchResultFromHistory retrieves relevant knowledge references from chat history
func (p *PluginSearch) getSearchResultFromHistory(chatManage *types.ChatManage) []*types.SearchResult {
	if len(chatManage.History) == 0 {
		return nil
	}
	// Search history in reverse chronological order
	for i := len(chatManage.History) - 1; i >= 0; i-- {
		if len(chatManage.History[i].KnowledgeReferences) > 0 {
			// Mark all references as history matches
			for _, reference := range chatManage.History[i].KnowledgeReferences {
				reference.MatchType = types.MatchTypeHistory
			}
			return chatManage.History[i].KnowledgeReferences
		}
	}
	return nil
}

func removeDuplicateResults(results []*types.SearchResult) []*types.SearchResult {
	seen := make(map[string]bool)
	contentSig := make(map[string]bool)
	var uniqueResults []*types.SearchResult
	for _, r := range results {
		keys := []string{r.ID}
		if r.ParentChunkID != "" {
			keys = append(keys, "parent:"+r.ParentChunkID)
		}
		if r.KnowledgeID != "" {
			keys = append(keys, fmt.Sprintf("kb:%s#%d", r.KnowledgeID, r.ChunkIndex))
		}
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
		sig := buildContentSignature(r.Content)
		if sig != "" {
			if contentSig[sig] {
				continue
			}
			contentSig[sig] = true
		}
		for _, k := range keys {
			seen[k] = true
		}
		uniqueResults = append(uniqueResults, r)
	}
	return uniqueResults
}

func buildContentSignature(content string) string {
	c := strings.ToLower(strings.TrimSpace(content))
	if c == "" {
		return ""
	}
	c = strings.Join(strings.Fields(c), " ")
	if len(c) > 128 {
		c = c[:128]
	}
	return c
}

// searchKnowledgeBases performs KB searches for rewrite and processed queries across KB IDs
func (p *PluginSearch) searchKnowledgeBases(
	ctx context.Context,
	knowledgeBaseIDs []string,
	chatManage *types.ChatManage,
) []*types.SearchResult {
	// Build base params for rewrite query
	baseParams := types.SearchParams{
		QueryText:        strings.TrimSpace(chatManage.RewriteQuery),
		VectorThreshold:  chatManage.VectorThreshold,
		KeywordThreshold: chatManage.KeywordThreshold,
		MatchCount:       chatManage.EmbeddingTopK,
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []*types.SearchResult

	// Search with rewrite query
	for _, kbID := range knowledgeBaseIDs {
		wg.Add(1)
		go func(knowledgeBaseID string) {
			defer wg.Done()
			res, err := p.knowledgeBaseService.HybridSearch(ctx, knowledgeBaseID, baseParams)
			if err != nil {
				pipelineWarn(ctx, "Search", "kb_search_error", map[string]interface{}{
					"kb_id":    knowledgeBaseID,
					"query":    baseParams.QueryText,
					"error":    err.Error(),
					"query_ty": "rewrite",
				})
				return
			}
			pipelineInfo(ctx, "Search", "kb_result", map[string]interface{}{
				"kb_id":     knowledgeBaseID,
				"query_ty":  "rewrite",
				"hit_count": len(res),
			})
			mu.Lock()
			results = append(results, res...)
			mu.Unlock()
		}(kbID)
	}

	wg.Wait()

	// If processed query differs, search again
	if chatManage.RewriteQuery != chatManage.ProcessedQuery {
		paramsProcessed := baseParams
		paramsProcessed.QueryText = strings.TrimSpace(chatManage.ProcessedQuery)
		pipelineInfo(ctx, "Search", "processed_query_search", map[string]interface{}{
			"query": paramsProcessed.QueryText,
		})

		wg = sync.WaitGroup{}
		for _, kbID := range knowledgeBaseIDs {
			wg.Add(1)
			go func(knowledgeBaseID string) {
				defer wg.Done()
				res, err := p.knowledgeBaseService.HybridSearch(ctx, knowledgeBaseID, paramsProcessed)
				if err != nil {
					pipelineWarn(ctx, "Search", "kb_search_error", map[string]interface{}{
						"kb_id":    knowledgeBaseID,
						"query":    paramsProcessed.QueryText,
						"error":    err.Error(),
						"query_ty": "processed",
					})
					return
				}
				pipelineInfo(ctx, "Search", "kb_result", map[string]interface{}{
					"kb_id":     knowledgeBaseID,
					"query_ty":  "processed",
					"hit_count": len(res),
				})
				mu.Lock()
				results = append(results, res...)
				mu.Unlock()
			}(kbID)
		}
		wg.Wait()
	}

	// Normalize keyword retriever scores after collecting all results from multiple knowledge bases
	normalizeKeywordSearchResults(ctx, results)

	pipelineInfo(ctx, "Search", "kb_result_summary", map[string]interface{}{
		"total_hits": len(results),
	})
	return results
}

// searchWebIfEnabled executes web search when enabled and returns converted results
func (p *PluginSearch) searchWebIfEnabled(ctx context.Context, chatManage *types.ChatManage) []*types.SearchResult {
	if !chatManage.WebSearchEnabled || p.webSearchService == nil || p.tenantService == nil || chatManage.TenantID <= 0 {
		return nil
	}
	tenant := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
	if tenant == nil || tenant.WebSearchConfig == nil || tenant.WebSearchConfig.Provider == "" {
		pipelineWarn(ctx, "Search", "web_config_missing", map[string]interface{}{
			"tenant_id": chatManage.TenantID,
		})
		return nil
	}

	pipelineInfo(ctx, "Search", "web_request", map[string]interface{}{
		"tenant_id": chatManage.TenantID,
		"provider":  tenant.WebSearchConfig.Provider,
	})
	webResults, err := p.webSearchService.Search(ctx, tenant.WebSearchConfig, chatManage.RewriteQuery)
	if err != nil {
		pipelineWarn(ctx, "Search", "web_search_error", map[string]interface{}{
			"tenant_id": chatManage.TenantID,
			"error":     err.Error(),
		})
		return nil
	}
	// Build questions (rewrite + processed if different)
	questions := []string{strings.TrimSpace(chatManage.RewriteQuery)}
	if chatManage.ProcessedQuery != "" && chatManage.ProcessedQuery != chatManage.RewriteQuery {
		questions = append(questions, strings.TrimSpace(chatManage.ProcessedQuery))
	}
	// Load session-scoped temp KB state from Redis using SessionService
	tempKBID, seen, ids := p.sessionService.GetWebSearchTempKBState(ctx, chatManage.SessionID)
	compressed, kbID, newSeen, newIDs, err := p.webSearchService.CompressWithRAG(
		ctx, chatManage.SessionID, tempKBID, questions, webResults, tenant.WebSearchConfig,
		p.knowledgeBaseService, p.knowledgeService, seen, ids,
	)
	if err != nil {
		pipelineWarn(ctx, "Search", "web_compress_error", map[string]interface{}{
			"error": err.Error(),
		})
	} else {
		webResults = compressed
		// Persist temp KB state back into Redis using SessionService
		p.sessionService.SaveWebSearchTempKBState(ctx, chatManage.SessionID, kbID, newSeen, newIDs)
	}
	res := convertWebSearchResults(webResults)
	pipelineInfo(ctx, "Search", "web_hits", map[string]interface{}{
		"hit_count": len(res),
	})
	return res
}

// convertWebSearchResults converts WebSearchResult to SearchResult
// This is a duplicate of the function in service/web_search.go to avoid circular imports
func convertWebSearchResults(webResults []*types.WebSearchResult) []*types.SearchResult {
	results := make([]*types.SearchResult, 0, len(webResults))

	for i, webResult := range webResults {
		// Use URL as ChunkID for web search results
		chunkID := webResult.URL
		if chunkID == "" {
			chunkID = fmt.Sprintf("web_search_%d", i)
		}

		// Combine title and snippet as content
		content := webResult.Title
		if webResult.Snippet != "" {
			if content != "" {
				content += "\n\n" + webResult.Snippet
			} else {
				content = webResult.Snippet
			}
		}
		if webResult.Content != "" {
			if content != "" {
				content += "\n\n" + webResult.Content
			} else {
				content = webResult.Content
			}
		}

		// Set a default score for web search results (0.6, indicating medium relevance)
		score := 0.6

		result := &types.SearchResult{
			ID:             chunkID,
			Content:        content,
			KnowledgeID:    "", // Web search results don't have knowledge ID
			ChunkIndex:     0,
			KnowledgeTitle: webResult.Title,
			StartAt:        0,
			EndAt:          runeLen(content),
			Seq:            1,
			Score:          score,
			MatchType:      types.MatchTypeWebSearch,
			SubChunkID:     []string{},
			Metadata: map[string]string{
				"url":     webResult.URL,
				"source":  webResult.Source,
				"title":   webResult.Title,
				"snippet": webResult.Snippet,
			},
			ChunkType:         string(types.ChunkTypeWebSearch),
			ParentChunkID:     "",
			ImageInfo:         "",
			KnowledgeFilename: "",
			KnowledgeSource:   "web_search",
		}

		// Add published date to metadata if available
		if webResult.PublishedAt != nil {
			result.Metadata["published_at"] = webResult.PublishedAt.Format(time.RFC3339)
		}

		results = append(results, result)
	}

	return results
}

// expandQueries generates paraphrases and synonyms using chat model to improve keyword recall
func (p *PluginSearch) expandQueries(ctx context.Context, chatManage *types.ChatManage) []string {
	if p.modelService == nil || chatManage.ChatModelID == "" {
		pipelineWarn(ctx, "Search", "expansion_skip", map[string]interface{}{
			"reason": "no_model",
		})
		return nil
	}
	model, err := p.modelService.GetChatModel(ctx, chatManage.ChatModelID)
	if err != nil {
		pipelineWarn(ctx, "Search", "expansion_get_model_failed", map[string]interface{}{
			"error": err.Error(),
		})
		return nil
	}
	sys := "Generate up to 5 diverse paraphrases or keyword variants for the user query to improve keyword-based search recall. Respond ONLY with a JSON array of strings inside a fenced code block."
	usr := chatManage.RewriteQuery
	think := false
	resp, err := model.Chat(ctx, []chat.Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: usr},
	}, &chat.ChatOptions{Temperature: 0.2, MaxCompletionTokens: 200, Thinking: &think})
	if err != nil || resp.Content == "" {
		pipelineWarn(ctx, "Search", "expansion_model_call_failed", map[string]interface{}{
			"error": err,
		})
		return nil
	}
	body := extractJSONBlock(resp.Content)
	var arr []string
	if err := json.Unmarshal([]byte(body), &arr); err != nil || len(arr) == 0 {
		// Fallback: split lines
		lines := strings.Split(resp.Content, "\n")
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l != "" {
				arr = append(arr, l)
			}
		}
	}
	uniq := make(map[string]struct{})
	base := []string{chatManage.Query, chatManage.RewriteQuery, chatManage.ProcessedQuery}
	for _, b := range base {
		if s := strings.TrimSpace(b); s != "" {
			uniq[strings.ToLower(s)] = struct{}{}
		}
	}
	expansions := make([]string, 0, len(arr))
	for _, a := range arr {
		s := strings.TrimSpace(a)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if _, ok := uniq[key]; ok {
			continue
		}
		uniq[key] = struct{}{}
		expansions = append(expansions, s)
		if len(expansions) >= 5 {
			break
		}
	}
	pipelineInfo(ctx, "Search", "expansion_result", map[string]interface{}{
		"variants": len(expansions),
	})
	return expansions
}

func extractJSONBlock(text string) string {
	t := strings.TrimSpace(text)
	if i := strings.Index(t, "["); i >= 0 {
		j := strings.LastIndex(t, "]")
		if j > i {
			return t[i : j+1]
		}
	}
	return "[]"
}

// normalizeKeywordSearchResults normalizes keyword search result scores into [0,1] globally across all knowledge bases
// Improvements:
// 1. Uses robust normalization with percentile-based bounds to handle outliers
// 2. Handles edge cases: single result, no variance, negative scores
// 3. Global normalization ensures fair comparison across different knowledge bases
func normalizeKeywordSearchResults(ctx context.Context, results []*types.SearchResult) {
	// Filter keyword match results
	keywordResults := make([]*types.SearchResult, 0)
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
		pipelineInfo(ctx, "Search", "keyword_scores_no_variance", map[string]interface{}{
			"count": len(keywordResults),
			"score": minS,
		})
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

		pipelineInfo(ctx, "Search", "normalize_keyword_scores", map[string]interface{}{
			"count":         len(keywordResults),
			"raw_min":       minS,
			"raw_max":       maxS,
			"normalize_min": normalizeMin,
			"normalize_max": normalizeMax,
		})
	} else {
		// Fallback: all scores are the same after percentile filtering
		for _, r := range keywordResults {
			r.Score = 1.0
		}
	}
}
