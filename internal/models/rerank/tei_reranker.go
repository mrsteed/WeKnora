package rerank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
)

// TEIReranker implements reranking using HuggingFace Text Embeddings Inference API
type TEIReranker struct {
	modelName string       // Name of the model used for reranking
	modelID   string       // Unique identifier of the model
	apiKey    string       // API key for authentication
	baseURL   string       // Base URL for API requests
	client    *http.Client // HTTP client for making API requests
}

// TEIRerankRequest represents a request to TEI rerank API
// Note: TEI uses "texts" instead of "documents"
type TEIRerankRequest struct {
	Query      string   `json:"query"`
	Texts      []string `json:"texts"`
	ReturnText bool     `json:"return_text,omitempty"`
	Truncate   bool     `json:"truncate,omitempty"`
}

// TEIRankResult represents a single result item from TEI API
// Note: TEI returns "score" directly, not "relevance_score"
type TEIRankResult struct {
	Index int     `json:"index"`
	Score float64 `json:"score"`
	Text  string  `json:"text,omitempty"`
}

// NewTEIReranker creates a new instance of TEI reranker
func NewTEIReranker(config *RerankerConfig) (*TEIReranker, error) {
	baseURL := strings.TrimSuffix(config.BaseURL, "/")
	// Strip trailing /rerank or /rerank/ to avoid double path when we append /rerank later
	baseURL = strings.TrimSuffix(baseURL, "/rerank")
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &TEIReranker{
		modelName: config.ModelName,
		modelID:   config.ModelID,
		apiKey:    config.APIKey,
		baseURL:   baseURL,
		client:    &http.Client{},
	}, nil
}

// Rerank performs document reranking using TEI API
func (r *TEIReranker) Rerank(ctx context.Context, query string, documents []string) ([]RankResult, error) {
	reqBody := TEIRerankRequest{
		Query:      query,
		Texts:      documents,
		ReturnText: false, // We don't need text returned, we just want scores and indices
		Truncate:   true,  // Auto truncate to max context length
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal TEI request: %w", err)
	}

	// Send the request
	// Support both /rerank and /v1/rerank paths, but usually TEI is /rerank at root
	url := fmt.Sprintf("%s/rerank", r.baseURL)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create TEI request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if r.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", r.apiKey))
	}

	// Log for debugging
	logger.GetLogger(ctx).Debugf(
		"TEI Rerank request: curl -X POST %s -d '%s'",
		url, string(jsonData),
	)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do TEI request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read TEI response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TEI Rerank API error: Status: %s, Body: %s", resp.Status, string(bodyBytes))
	}

	// TEI returns a JSON Array of results, not wrapped in an object
	var teiResults []TEIRankResult
	if err := json.Unmarshal(bodyBytes, &teiResults); err != nil {
		return nil, fmt.Errorf("unmarshal TEI response: %w, body: %s", err, string(bodyBytes))
	}

	// Convert to standard RankResult
	results := make([]RankResult, len(teiResults))
	for i, tr := range teiResults {
		results[i] = RankResult{
			Index:          tr.Index,
			RelevanceScore: tr.Score,
			Document:       DocumentInfo{Text: ""}, // Text not returned to save bandwidth
		}
	}

	return results, nil
}

// GetModelName returns the name of the reranking model
func (r *TEIReranker) GetModelName() string {
	return r.modelName
}

// GetModelID returns the unique identifier of the reranking model
func (r *TEIReranker) GetModelID() string {
	return r.modelID
}
