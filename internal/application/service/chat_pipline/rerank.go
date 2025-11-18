package chatpipline

import (
  	"context"
  	"encoding/json"
  	"fmt"
  	"math"
  	"strings"

  	"github.com/Tencent/WeKnora/internal/models/rerank"
  	"github.com/Tencent/WeKnora/internal/types"
  	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// PluginRerank implements reranking functionality for chat pipeline
type PluginRerank struct {
	modelService interfaces.ModelService // Service to access rerank models
}

// NewPluginRerank creates a new rerank plugin instance
func NewPluginRerank(eventManager *EventManager, modelService interfaces.ModelService) *PluginRerank {
	res := &PluginRerank{
		modelService: modelService,
	}
	eventManager.Register(res)
	return res
}

// ActivationEvents returns the event types this plugin handles
func (p *PluginRerank) ActivationEvents() []types.EventType {
	return []types.EventType{types.CHUNK_RERANK}
}

// OnEvent handles reranking events in the chat pipeline
func (p *PluginRerank) OnEvent(ctx context.Context,
    eventType types.EventType, chatManage *types.ChatManage, next func() *PluginError,
) *PluginError {
	pipelineInfo(ctx, "Rerank", "input", map[string]interface{}{
		"session_id":      chatManage.SessionID,
		"candidate_cnt":   len(chatManage.SearchResult),
		"rerank_model":    chatManage.RerankModelID,
		"rerank_thresh":   chatManage.RerankThreshold,
		"rewrite_query":   chatManage.RewriteQuery,
		"processed_query": chatManage.ProcessedQuery,
	})
	if len(chatManage.SearchResult) == 0 {
		pipelineInfo(ctx, "Rerank", "skip", map[string]interface{}{
			"reason": "empty_search_result",
		})
		return next()
	}
	if chatManage.RerankModelID == "" {
		pipelineWarn(ctx, "Rerank", "skip", map[string]interface{}{
			"reason": "empty_model_id",
		})
		return next()
	}

	// Get rerank model from service
	rerankModel, err := p.modelService.GetRerankModel(ctx, chatManage.RerankModelID)
	if err != nil {
		pipelineError(ctx, "Rerank", "get_model", map[string]interface{}{
			"model_id": chatManage.RerankModelID,
			"error":    err.Error(),
		})
		return ErrGetRerankModel.WithError(err)
	}

	// Prepare passages for reranking
	pipelineInfo(ctx, "Rerank", "build_passages", map[string]interface{}{
		"candidate_cnt": len(chatManage.SearchResult),
	})
	var passages []string
	for _, result := range chatManage.SearchResult {
		// 合并Content和ImageInfo的文本内容
		passage := getEnrichedPassage(ctx, result)
		passages = append(passages, passage)
	}

	// Try reranking with different query variants in priority order
	rerankResp := p.rerank(ctx, chatManage, rerankModel, chatManage.RewriteQuery, passages)
	if len(rerankResp) == 0 {
		rerankResp = p.rerank(ctx, chatManage, rerankModel, chatManage.ProcessedQuery, passages)
		if len(rerankResp) == 0 {
			rerankResp = p.rerank(ctx, chatManage, rerankModel, chatManage.Query, passages)
		}
	}

    pipelineInfo(ctx, "Rerank", "model_response", map[string]interface{}{
        "result_cnt": len(rerankResp),
    })
    for i := range chatManage.SearchResult {
        chatManage.SearchResult[i].Metadata = ensureMetadata(chatManage.SearchResult[i].Metadata)
    }
    reranked := make([]*types.SearchResult, 0, len(rerankResp))
    for _, rr := range rerankResp {
        sr := chatManage.SearchResult[rr.Index]
        base := sr.Score
        sr.Metadata["base_score"] = fmt.Sprintf("%.4f", base)
        sr.Score = rr.RelevanceScore
        sr.Score = compositeScore(sr, rr.RelevanceScore, base, chatManage)
        reranked = append(reranked, sr)
    }
    final := applyMMR(ctx, reranked, chatManage, min(len(reranked), max(1, chatManage.RerankTopK)), 0.7)
    chatManage.RerankResult = final

    // Log composite top scores and MMR selection summary
    topN := min(3, len(reranked))
    for i := 0; i < topN; i++ {
        pipelineInfo(ctx, "Rerank", "composite_top", map[string]interface{}{
            "rank":        i + 1,
            "chunk_id":    reranked[i].ID,
            "base_score":  reranked[i].Metadata["base_score"],
            "final_score": fmt.Sprintf("%.4f", reranked[i].Score),
            "intent":      chatManage.QueryIntent,
        })
    }

    if len(chatManage.RerankResult) == 0 {
        pipelineWarn(ctx, "Rerank", "output", map[string]interface{}{
            "filtered_cnt": 0,
        })
        return ErrSearchNothing
    }

	pipelineInfo(ctx, "Rerank", "output", map[string]interface{}{
		"filtered_cnt": len(chatManage.RerankResult),
	})
    return next()
}

// rerank performs the actual reranking operation with given query and passages
func (p *PluginRerank) rerank(ctx context.Context,
    chatManage *types.ChatManage, rerankModel rerank.Reranker, query string, passages []string,
) []rerank.RankResult {
	pipelineInfo(ctx, "Rerank", "model_call", map[string]interface{}{
		"query_variant": query,
		"passages":      len(passages),
	})
	rerankResp, err := rerankModel.Rerank(ctx, query, passages)
	if err != nil {
		pipelineError(ctx, "Rerank", "model_call", map[string]interface{}{
			"query_variant": query,
			"error":         err.Error(),
		})
		return nil
	}

	// Log top scores for debugging
	pipelineInfo(ctx, "Rerank", "threshold", map[string]interface{}{
		"threshold": chatManage.RerankThreshold,
	})
	for i := range min(5, len(rerankResp)) {
		pipelineInfo(ctx, "Rerank", "top_score", map[string]interface{}{
			"rank":       i + 1,
			"score":      rerankResp[i].RelevanceScore,
			"chunk_id":   chatManage.SearchResult[rerankResp[i].Index].ID,
			"match_type": chatManage.SearchResult[rerankResp[i].Index].MatchType,
			"chunk_type": chatManage.SearchResult[rerankResp[i].Index].ChunkType,
			"content":    chatManage.SearchResult[rerankResp[i].Index].Content,
		})
	}

	// Filter results based on threshold with special handling for history matches
	rankFilter := []rerank.RankResult{}
	for _, result := range rerankResp {
		th := chatManage.RerankThreshold
		matchType := chatManage.SearchResult[result.Index].MatchType
		if matchType == types.MatchTypeHistory {
			th = math.Max(th-0.1, 0.5) // Lower threshold for history matches
		}
		if result.RelevanceScore > th {
			rankFilter = append(rankFilter, result)
		}
	}
    return rankFilter
}

func ensureMetadata(m map[string]string) map[string]string {
    if m == nil {
        return make(map[string]string)
    }
    return m
}

func compositeScore(sr *types.SearchResult, modelScore, baseScore float64, chatManage *types.ChatManage) float64 {
    sourceWeight := 1.0
    switch strings.ToLower(sr.KnowledgeSource) {
    case "web_search":
        sourceWeight = 0.95
    default:
        sourceWeight = 1.0
    }
    intentBoost := 1.0
    if chatManage.QueryIntent != "" {
        switch chatManage.QueryIntent {
        case "definition":
            if sr.ChunkType == string(types.ChunkTypeSummary) {
                intentBoost = 1.05
            }
        case "howto":
            if sr.EndAt-sr.StartAt > 300 {
                intentBoost = 1.03
            }
        case "compare":
            intentBoost = 1.0
        }
    }
    positionPrior := 1.0
    if sr.StartAt >= 0 {
        positionPrior += clampFloat(1.0-float64(sr.StartAt)/float64(sr.EndAt+1), -0.05, 0.05)
    }
    composite := 0.6*modelScore + 0.3*baseScore + 0.1*sourceWeight
    composite *= intentBoost
    composite *= positionPrior
    if composite < 0 {
        composite = 0
    }
    if composite > 1 {
        composite = 1
    }
    return composite
}

func applyMMR(ctx context.Context, results []*types.SearchResult, chatManage *types.ChatManage, k int, lambda float64) []*types.SearchResult {
    if k <= 0 || len(results) == 0 {
        return nil
    }
    pipelineInfo(ctx, "Rerank", "mmr_start", map[string]interface{}{
        "lambda": lambda,
        "k":      k,
        "candidates": len(results),
    })
    selected := make([]*types.SearchResult, 0, k)
    candidates := make([]*types.SearchResult, len(results))
    copy(candidates, results)
    tokenSets := make([]map[string]struct{}, len(candidates))
    for i, r := range candidates {
        tokenSets[i] = tokenizeSimple(getEnrichedPassage(ctx, r))
    }
    for len(selected) < k && len(candidates) > 0 {
        bestIdx := 0
        bestScore := -1.0
        for i, r := range candidates {
            relevance := r.Score
            redundancy := 0.0
            for _, s := range selected {
                redundancy = math.Max(redundancy, jaccard(tokenSets[i], tokenizeSimple(getEnrichedPassage(ctx, s))))
            }
            mmr := lambda*relevance - (1.0-lambda)*redundancy
            if mmr > bestScore {
                bestScore = mmr
                bestIdx = i
            }
        }
        selected = append(selected, candidates[bestIdx])
        candidates = append(candidates[:bestIdx], candidates[bestIdx+1:]...)
    }
    // Compute average redundancy among selected
    avgRed := 0.0
    if len(selected) > 1 {
        pairs := 0
        for i := 0; i < len(selected); i++ {
            for j := i + 1; j < len(selected); j++ {
                si := tokenizeSimple(getEnrichedPassage(ctx, selected[i]))
                sj := tokenizeSimple(getEnrichedPassage(ctx, selected[j]))
                avgRed += jaccard(si, sj)
                pairs++
            }
        }
        if pairs > 0 {
            avgRed /= float64(pairs)
        }
    }
    pipelineInfo(ctx, "Rerank", "mmr_done", map[string]interface{}{
        "selected": len(selected),
        "avg_redundancy": fmt.Sprintf("%.4f", avgRed),
    })
    return selected
}

func tokenizeSimple(text string) map[string]struct{} {
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

func jaccard(a, b map[string]struct{}) float64 {
    if len(a) == 0 && len(b) == 0 {
        return 0
    }
    inter := 0
    for k := range a {
        if _, ok := b[k]; ok {
            inter++
        }
    }
    union := len(a) + len(b) - inter
    if union == 0 {
        return 0
    }
    return float64(inter) / float64(union)
}

func clampFloat(v, minV, maxV float64) float64 {
    if v < minV {
        return minV
    }
    if v > maxV {
        return maxV
    }
    return v
}

// getEnrichedPassage 合并Content和ImageInfo的文本内容
func getEnrichedPassage(ctx context.Context, result *types.SearchResult) string {
	if result.ImageInfo == "" {
		return result.Content
	}

	// 解析ImageInfo
	var imageInfos []types.ImageInfo
	err := json.Unmarshal([]byte(result.ImageInfo), &imageInfos)
	if err != nil {
		pipelineWarn(ctx, "Rerank", "image_info_parse", map[string]interface{}{
			"error": err.Error(),
		})
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

	pipelineInfo(ctx, "Rerank", "image_info_merge", map[string]interface{}{
		"content_len": len(result.Content),
		"image_len":   len(strings.Join(imageTexts, "\n")),
	})

	return combinedText
}
