package tools

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/utils"
)

const (
	webFetchTimeout                 = 60 * time.Second // timeout for web fetch
	webFetchMaxChars                = 100000           // maximum number of characters to fetch
	webFetchChromedpBaseDirEnv      = "WEKNORA_WEB_FETCH_CHROMEDP_BASE_DIR"
	webFetchChromedpDefaultBaseDir  = "/tmp"
)

var webFetchTool = BaseTool{
	name: ToolWebFetch,
	description: `Fetch detailed web content from previously discovered URLs and analyze it with an LLM.

## Usage
- Receive one or more {url, prompt} combinations
- Fetch web page content and convert to Markdown text
- Use prompt to call small model for analysis and summary (if model is available)
- Return summary result and original content fragment

## When to Use
- **MANDATORY**: After web_search returns results, if content is truncated or incomplete, use web_fetch to get full page content
- When web_search snippet is insufficient for answering the question`,
	schema: utils.GenerateSchema[WebFetchInput](),
}

// WebFetchInput defines the input parameters for web fetch tool
type WebFetchInput struct {
	Items []WebFetchItem `json:"items" jsonschema:"Batch fetch tasks, each containing a url and prompt"`
}

// WebFetchItem represents a single web fetch task
type WebFetchItem struct {
	URL    string `json:"url" jsonschema:"URL of the web page to fetch, should come from web_search results"`
	Prompt string `json:"prompt" jsonschema:"Prompt for analyzing the fetched web page content"`
}

// webFetchParams is the parameters for the web fetch tool
type webFetchParams struct {
	URL    string
	Prompt string
}

// validatedParams holds validated input plus DNS-pinned host/IP for SSRF protection.
// PinnedIP is the single IP we resolved at validation time; chromedp and HTTP both use it.
type validatedParams struct {
	URL      string
	Prompt   string
	Host     string
	Port     string
	PinnedIP net.IP
}

// webFetchItemResult is the result for a web fetch item
type webFetchItemResult struct {
	output string
	data   map[string]interface{}
	err    error
}

type webFetchSummary struct {
	RequestedCount int
	FetchedCount   int
	FailedCount    int
	AllFetchFailed bool
	AnswerBasis    string
	StatusNotice   string
}

// WebFetchTool fetches web page content and summarizes it using an LLM
type WebFetchTool struct {
	BaseTool
	client          *http.Client
	chatModel       chat.Chat
	chromedpBaseDir string
}

// NewWebFetchTool creates a new web_fetch tool instance
func NewWebFetchTool(chatModel chat.Chat) *WebFetchTool {
	// Use SSRF-safe HTTP client to prevent redirect-based SSRF attacks
	ssrfConfig := utils.DefaultSSRFSafeHTTPClientConfig()
	ssrfConfig.Timeout = webFetchTimeout

	return &WebFetchTool{
		BaseTool:        webFetchTool,
		client:          utils.NewSSRFSafeHTTPClient(ssrfConfig),
		chatModel:       chatModel,
		chromedpBaseDir: resolveWebFetchChromedpBaseDir(),
	}
}

// Execute 执行 web_fetch 工具
func (t *WebFetchTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	logger.Infof(ctx, "[Tool][WebFetch] Execute started")

	// Parse args from json.RawMessage
	var input WebFetchInput
	if err := json.Unmarshal(args, &input); err != nil {
		logger.Errorf(ctx, "[Tool][WebFetch] Failed to parse args: %v", err)
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to parse args: %v", err),
		}, err
	}

	if len(input.Items) == 0 {
		logger.Errorf(ctx, "[Tool][WebFetch] 参数缺失: items")
		return &types.ToolResult{
			Success: false,
			Error:   "missing required parameter: items",
		}, nil
	}

	results := make([]*webFetchItemResult, len(input.Items))

	var wg sync.WaitGroup
	wg.Add(len(input.Items))

	for idx := range input.Items {
		i := idx
		item := input.Items[i]

		params := webFetchParams{
			URL:    item.URL,
			Prompt: item.Prompt,
		}

		go func(index int, p webFetchParams) {
			defer wg.Done()

			// Normalize URL before validation so we pin the host we actually fetch (e.g. raw.githubusercontent.com)
			finalURL := t.normalizeGitHubURL(p.URL)
			vp, err := t.validateAndResolve(webFetchParams{URL: finalURL, Prompt: p.Prompt})
			if err != nil {
				results[index] = &webFetchItemResult{
					err: err,
					data: map[string]interface{}{
						"url":    p.URL,
						"prompt": p.Prompt,
						"error":  err.Error(),
					},
					output: fmt.Sprintf("URL: %s\nError: %v\n\n", p.URL, err),
				}
				return
			}

			output, data, err := t.executeFetch(ctx, vp, p.URL)
			results[index] = &webFetchItemResult{
				output: output,
				data:   data,
				err:    err,
			}
		}(i, params)
	}

	wg.Wait()
	summary := summarizeWebFetchResults(results)

	var builder strings.Builder
	builder.WriteString("=== Web Fetch Results ===\n\n")

	aggregated := make([]map[string]interface{}, 0, len(results))
	success := true
	var firstErr error

	for idx, res := range results {
		if res == nil {
			success = false
			if firstErr == nil {
				firstErr = fmt.Errorf("fetch item %d returned nil", idx)
			}
			builder.WriteString(fmt.Sprintf("#%d: No result (internal error)\n\n", idx+1))
			continue
		}

		builder.WriteString(fmt.Sprintf("#%d:\n%s", idx+1, res.output))
		if !strings.HasSuffix(res.output, "\n") {
			builder.WriteString("\n")
		}
		builder.WriteString("\n")

		if res.data != nil {
			aggregated = append(aggregated, res.data)
		}

		if res.err != nil {
			success = false
			if firstErr == nil {
				firstErr = res.err
			}
		}
	}

	builder.WriteString("=== Fetch Summary ===\n")
	builder.WriteString(fmt.Sprintf("- Requested URLs: %d\n", summary.RequestedCount))
	builder.WriteString(fmt.Sprintf("- Full pages fetched: %d\n", summary.FetchedCount))
	builder.WriteString(fmt.Sprintf("- Fetch failures: %d\n", summary.FailedCount))
	if summary.StatusNotice != "" {
		builder.WriteString(fmt.Sprintf("- Notice: %s\n", summary.StatusNotice))
	}
	builder.WriteString("\n")

	// Add guidance for next steps
	builder.WriteString("\n=== Next Steps ===\n")
	if summary.FetchedCount > 0 {
		builder.WriteString(fmt.Sprintf("- ✅ Full page content was fetched for %d of %d URL(s).\n", summary.FetchedCount, summary.RequestedCount))
		builder.WriteString("- Evaluate if the content is sufficient to answer the question completely.\n")
		builder.WriteString("- Synthesize information from all fetched pages for comprehensive answers.\n")
		if summary.FailedCount > 0 {
			builder.WriteString("- ⚠️ Some URLs could not be fetched. Treat those URLs as snippet-only evidence from prior web_search results.\n")
		}
	} else {
		builder.WriteString("- ❌ No full page content was successfully fetched.\n")
		builder.WriteString("- Any downstream answer should rely on prior web_search snippets rather than full page text.\n")
		builder.WriteString("- Consider:\n")
		builder.WriteString("  - Verify URLs are accessible\n")
		builder.WriteString("  - Try alternative sources from web_search results\n")
		builder.WriteString("  - Check if information can be found in knowledge base instead\n")
	}

	data := map[string]interface{}{
		"results":          aggregated,
		"count":            len(aggregated),
		"requested_count":  summary.RequestedCount,
		"fetched_count":    summary.FetchedCount,
		"failed_count":     summary.FailedCount,
		"all_fetch_failed": summary.AllFetchFailed,
		"answer_basis":     summary.AnswerBasis,
		"status_notice":    summary.StatusNotice,
		"display_type":     "web_fetch_results",
	}

	logger.Infof(
		ctx,
		"[Tool][WebFetch] Completed with success=%v, requested=%d, fetched=%d, failed=%d",
		success,
		summary.RequestedCount,
		summary.FetchedCount,
		summary.FailedCount,
	)

	return &types.ToolResult{
		Success: success,
		Output:  builder.String(),
		Data:    data,
		Error: func() string {
			if firstErr != nil {
				return firstErr.Error()
			}
			return ""
		}(),
	}, nil
}

func resolveWebFetchChromedpBaseDir() string {
	baseDir := strings.TrimSpace(os.Getenv(webFetchChromedpBaseDirEnv))
	if baseDir == "" {
		baseDir = webFetchChromedpDefaultBaseDir
	}
	return filepath.Clean(baseDir)
}

func summarizeWebFetchResults(results []*webFetchItemResult) webFetchSummary {
	summary := webFetchSummary{
		RequestedCount: len(results),
		AnswerBasis:    "full_page_content",
	}

	for _, res := range results {
		if res != nil && isFetchedWebFetchResult(res.data) {
			summary.FetchedCount++
		}
	}

	summary.FailedCount = summary.RequestedCount - summary.FetchedCount
	summary.AllFetchFailed = summary.RequestedCount > 0 && summary.FetchedCount == 0

	switch {
	case summary.AllFetchFailed:
		summary.AnswerBasis = "web_search_snippets_only"
		summary.StatusNotice = "No full-page content was fetched. Any downstream answer should rely on prior web_search snippets rather than full-page text."
	case summary.FailedCount > 0:
		summary.AnswerBasis = "mixed_fulltext_and_snippets"
		summary.StatusNotice = fmt.Sprintf(
			"Fetched full-page content for %d of %d URL(s). Downstream answers should prioritize fetched page text and treat the remaining URLs as snippet-only evidence.",
			summary.FetchedCount,
			summary.RequestedCount,
		)
	}

	return summary
}

func isFetchedWebFetchResult(data map[string]interface{}) bool {
	if data == nil {
		return false
	}
	if status, ok := data["fetch_status"].(string); ok && status != "" {
		return status == "fetched"
	}
	rawContent, _ := data["raw_content"].(string)
	return strings.TrimSpace(rawContent) != ""
}

// parseParams parses the parameters for a web fetch item
func (t *WebFetchTool) parseParams(item interface{}) webFetchParams {
	params := webFetchParams{}
	if m, ok := item.(map[string]interface{}); ok {
		if v, ok := m["url"].(string); ok {
			params.URL = strings.TrimSpace(v)
		}
		if v, ok := m["prompt"].(string); ok {
			params.Prompt = strings.TrimSpace(v)
		}
	}
	return params
}

// validateAndResolve validates parameters and resolves the host to a single public IP (DNS pinning).
// The returned PinnedIP is used for both chromedp (host-resolver-rules) and HTTP to prevent DNS rebinding.
func (t *WebFetchTool) validateAndResolve(p webFetchParams) (*validatedParams, error) {
	if p.URL == "" {
		return nil, fmt.Errorf("url is required")
	}
	if p.Prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	if !strings.HasPrefix(p.URL, "http://") && !strings.HasPrefix(p.URL, "https://") {
		return nil, fmt.Errorf("invalid URL format")
	}

	// SSRF protection: validate URL is safe (uses centralised entry-point with whitelist support)
	if err := utils.ValidateURLForSSRF(p.URL); err != nil {
		return nil, fmt.Errorf("URL rejected for security reasons: %v", err)
	}

	u, err := url.Parse(p.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	hostname := u.Hostname()
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	// Resolve and pin to the first safe IP (same resolver as isSSRFSafeURL; we pin so chromedp cannot re-resolve).
	// Whitelisted hosts may resolve to private/restricted IPs, so we allow any IP for them.
	ips, err := net.DefaultResolver.LookupIP(context.Background(), "ip", hostname)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("DNS lookup failed for %s: %w", hostname, err)
	}
	isWhitelisted := utils.IsSSRFWhitelisted(hostname)
	var pinnedIP net.IP
	for _, ip := range ips {
		if isWhitelisted || utils.IsPublicIP(ip) {
			pinnedIP = ip
			break
		}
	}
	if pinnedIP == nil {
		return nil, fmt.Errorf("no public IP available for host %s", hostname)
	}

	return &validatedParams{
		URL:      p.URL,
		Prompt:   p.Prompt,
		Host:     hostname,
		Port:     port,
		PinnedIP: pinnedIP,
	}, nil
}

// executeFetch executes a web fetch item. displayURL is the URL shown to the user (e.g. original); vp.URL is the normalized URL we fetch.
func (t *WebFetchTool) executeFetch(
	ctx context.Context,
	vp *validatedParams,
	displayURL string,
) (string, map[string]interface{}, error) {
	logger.Infof(ctx, "[Tool][WebFetch] Fetching URL: %s", displayURL)

	htmlContent, method, err := t.fetchHTMLContent(ctx, vp)
	if err != nil {
		logger.Errorf(ctx, "[Tool][WebFetch] 获取页面失败 url=%s err=%v", vp.URL, err)
		return fmt.Sprintf("URL: %s\nError: %v\n", displayURL, err),
			map[string]interface{}{
				"url":         displayURL,
				"prompt":      vp.Prompt,
				"error":       err.Error(),
				"fetch_status": "failed",
			}, err
	}

	textContent := t.convertHTMLToText(htmlContent)

	resultData := map[string]interface{}{
		"url":            displayURL,
		"prompt":         vp.Prompt,
		"raw_content":    textContent,
		"content_length": len(textContent),
		"method":         method,
		"fetch_status":   "fetched",
	}
	params := webFetchParams{URL: displayURL, Prompt: vp.Prompt}
	var summary string
	var summaryErr error
	summary, summaryErr = t.processWithLLM(ctx, params, textContent)
	if summaryErr != nil {
		logger.Warnf(ctx, "[Tool][WebFetch] LLM 处理失败 url=%s err=%v", displayURL, summaryErr)
	} else if summary != "" {
		resultData["summary"] = summary
	}

	output := t.buildOutputText(params, textContent, summary, summaryErr)

	return output, resultData, summaryErr
}

// normalizeGitHubURL normalizes a GitHub URL
func (t *WebFetchTool) normalizeGitHubURL(source string) string {
	if strings.Contains(source, "github.com") && strings.Contains(source, "/blob/") {
		source = strings.Replace(source, "github.com", "raw.githubusercontent.com", 1)
		source = strings.Replace(source, "/blob/", "/", 1)
	}
	return source
}

// processWithLLM processes the content with an LLM
func (t *WebFetchTool) processWithLLM(ctx context.Context, params webFetchParams, content string) (string, error) {
	if t.chatModel == nil {
		return "", fmt.Errorf("chat model not available for web_fetch")
	}

	systemMessage := "You are an intelligent assistant skilled at reading web page content. Answer the user's request based on the provided web page text. Never fabricate information that does not appear in the text."
	userTemplate := `User request:
%s

Web page content:
%s`

	messages := []chat.Message{
		{
			Role:    "system",
			Content: systemMessage,
		},
		{
			Role:    "user",
			Content: fmt.Sprintf(userTemplate, params.Prompt, content),
		},
	}

	response, err := t.chatModel.Chat(ctx, messages, &chat.ChatOptions{
		Temperature: 0.3,
		MaxTokens:   1024,
	})
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(response.Content), nil
}

// buildOutputText builds the output text for a web fetch item
func (t *WebFetchTool) buildOutputText(params webFetchParams, content string, summary string, summaryErr error) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("URL: %s\n", params.URL))
	builder.WriteString(fmt.Sprintf("Prompt: %s\n", params.Prompt))

	if summaryErr == nil && summary != "" {
		builder.WriteString("Summary:\n")
		builder.WriteString(summary)
		builder.WriteString("\n")
	} else {
		builder.WriteString("Content Preview:\n")
		builder.WriteString(content)
		builder.WriteString("\n")
	}

	return builder.String()
}

func (t *WebFetchTool) prepareChromedpProfileDir() (string, error) {
	baseDir := resolveWebFetchChromedpBaseDir()
	if t.chromedpBaseDir != "" {
		baseDir = filepath.Clean(strings.TrimSpace(t.chromedpBaseDir))
	}
	if baseDir == "" {
		baseDir = webFetchChromedpDefaultBaseDir
	}
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return "", fmt.Errorf("failed to prepare chromedp base directory %s: %w", baseDir, err)
	}
	profileDir, err := os.MkdirTemp(baseDir, "chromedp-runner-*")
	if err != nil {
		return "", fmt.Errorf("failed to create chromedp profile directory under %s: %w", baseDir, err)
	}
	return profileDir, nil
}

// fetchHTMLContent fetches the HTML content for a web fetch item using pinned IP (DNS pinning).
func (t *WebFetchTool) fetchHTMLContent(ctx context.Context, vp *validatedParams) (string, string, error) {
	html, err := t.fetchWithChromedp(ctx, vp)
	if err == nil && strings.TrimSpace(html) != "" {
		return html, "chromedp", nil
	}

	if err != nil {
		logger.Debugf(ctx, "[Tool][WebFetch] Chromedp 抓取失败 url=%s err=%v，尝试直接请求", vp.URL, err)
	}

	html, httpErr := t.fetchWithHTTP(ctx, vp)
	if httpErr != nil {
		if err != nil {
			return "", "", fmt.Errorf("chromedp error: %v; http error: %w", err, httpErr)
		}
		return "", "", httpErr
	}

	return html, "http", nil
}

// fetchWithChromedp fetches the HTML content with Chromedp. Uses host-resolver-rules to pin host to vp.PinnedIP (DNS rebinding protection).
func (t *WebFetchTool) fetchWithChromedp(ctx context.Context, vp *validatedParams) (string, error) {
	logger.Debugf(ctx, "[Tool][WebFetch] Chromedp 抓取开始 url=%s", vp.URL)
	profileDir, err := t.prepareChromedpProfileDir()
	if err != nil {
		return "", fmt.Errorf("chromedp profile setup failed: %w", err)
	}
	defer os.RemoveAll(profileDir)
	logger.Debugf(ctx, "[Tool][WebFetch] Chromedp profile_dir=%s base_dir=%s", profileDir, filepath.Dir(profileDir))

	// DNS pinning: force Chrome to use the IP we resolved at validation time, not a second resolution.
	hostRule := fmt.Sprintf("MAP %s %s", vp.Host, vp.PinnedIP.String())
	opts := append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserDataDir(profileDir),
		chromedp.Flag("host-resolver-rules", hostRule),
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-setuid-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("disable-features", "VizDisplayCompositor"),
		chromedp.UserAgent(
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	defer cancel()

	ctx, cancel = chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, webFetchTimeout)
	defer cancel()

	var html string
	err = chromedp.Run(ctx,
		chromedp.Navigate(vp.URL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.OuterHTML("html", &html),
	)
	if err != nil {
		logger.Warnf(ctx, "[Tool][WebFetch] Chromedp 运行失败 profile_dir=%s err=%v", profileDir, err)
		return "", fmt.Errorf("chromedp run failed: %w", err)
	}

	logger.Debugf(ctx, "[Tool][WebFetch] Chromedp 抓取成功 url=%s profile_dir=%s", vp.URL, profileDir)
	return html, nil
}

// fetchWithHTTP fetches the HTML content with HTTP using pinned IP (same as chromedp path).
func (t *WebFetchTool) fetchWithHTTP(ctx context.Context, vp *validatedParams) (string, error) {
	resp, err := t.fetchWithTimeout(ctx, vp)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request failed with status %d %s", resp.StatusCode, resp.Status)
	}

	limitedReader := io.LimitReader(resp.Body, webFetchMaxChars*2)
	htmlBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return string(htmlBytes), nil
}

// fetchWithTimeout fetches the HTML content with a timeout. Uses pinned IP and original Host header (DNS pinning).
func (t *WebFetchTool) fetchWithTimeout(ctx context.Context, vp *validatedParams) (*http.Response, error) {
	// Connect to pinned IP so we do not re-resolve; set Host so the server gets the right virtual host.
	hostPort := net.JoinHostPort(vp.PinnedIP.String(), vp.Port)
	rawURL := vp.URL
	u, _ := url.Parse(rawURL)
	originalHost := u.Host
	u.Host = hostPort
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	// Preserve original host for TLS SNI and Host header (required for virtual hosting).
	req.Host = originalHost

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; WebFetchTool/1.0)")
	req.Header.Set(
		"Accept",
		"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8",
	)
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")

	return t.httpClientForFetch(vp).Do(req)
}

func (t *WebFetchTool) httpClientForFetch(vp *validatedParams) *http.Client {
	if t.client == nil || !strings.HasPrefix(strings.ToLower(vp.URL), "https://") {
		return t.client
	}

	baseTransport, ok := t.client.Transport.(*http.Transport)
	if !ok {
		return t.client
	}

	clientClone := *t.client
	transportClone := baseTransport.Clone()
	if transportClone.TLSClientConfig == nil {
		transportClone.TLSClientConfig = &tls.Config{}
	} else {
		transportClone.TLSClientConfig = transportClone.TLSClientConfig.Clone()
	}
	transportClone.TLSClientConfig.ServerName = vp.Host
	clientClone.Transport = transportClone
	return &clientClone
}

// convertHTMLToText converts the HTML content to text
func (t *WebFetchTool) convertHTMLToText(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return t.basicTextExtraction(html)
	}

	doc.Find("script, style, nav, footer, header").Remove()

	var markdown strings.Builder
	doc.Find("body").Each(func(i int, body *goquery.Selection) {
		t.processNode(body, &markdown)
	})

	result := markdown.String()
	result = regexp.MustCompile(`\n{3,}`).ReplaceAllString(result, "\n\n")
	return strings.TrimSpace(result)
}

// processNode processes a node in the HTML content
func (t *WebFetchTool) processNode(s *goquery.Selection, markdown *strings.Builder) {
	s.Contents().Each(func(i int, node *goquery.Selection) {
		nodeName := goquery.NodeName(node)

		switch nodeName {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			headerLevel := int(nodeName[1] - '0')
			markdown.WriteString("\n")
			markdown.WriteString(strings.Repeat("#", headerLevel))
			markdown.WriteString(" ")
			markdown.WriteString(strings.TrimSpace(node.Text()))
			markdown.WriteString("\n\n")
		case "p":
			t.processNode(node, markdown)
			markdown.WriteString("\n\n")
		case "a":
			href, exists := node.Attr("href")
			text := strings.TrimSpace(node.Text())
			if exists && text != "" {
				markdown.WriteString("[")
				markdown.WriteString(text)
				markdown.WriteString("](")
				markdown.WriteString(href)
				markdown.WriteString(")")
			} else if text != "" {
				markdown.WriteString(text)
			}
		case "img":
			src, _ := node.Attr("src")
			alt, _ := node.Attr("alt")
			if src != "" {
				markdown.WriteString("![")
				markdown.WriteString(alt)
				markdown.WriteString("](")
				markdown.WriteString(src)
				markdown.WriteString(")\n\n")
			}
		case "ul", "ol":
			markdown.WriteString("\n")
			isOrdered := nodeName == "ol"
			node.Find("li").Each(func(idx int, li *goquery.Selection) {
				if isOrdered {
					fmt.Fprintf(markdown, "%d. ", idx+1)
				} else {
					markdown.WriteString("- ")
				}
				markdown.WriteString(strings.TrimSpace(li.Text()))
				markdown.WriteString("\n")
			})
			markdown.WriteString("\n")
		case "br":
			markdown.WriteString("\n")
		case "code":
			parent := node.Parent()
			if goquery.NodeName(parent) == "pre" {
				markdown.WriteString("\n```\n")
				markdown.WriteString(node.Text())
				markdown.WriteString("\n```\n\n")
			} else {
				markdown.WriteString("`")
				markdown.WriteString(node.Text())
				markdown.WriteString("`")
			}
		case "blockquote":
			lines := strings.Split(strings.TrimSpace(node.Text()), "\n")
			for _, line := range lines {
				markdown.WriteString("> ")
				markdown.WriteString(strings.TrimSpace(line))
				markdown.WriteString("\n")
			}
			markdown.WriteString("\n")
		case "strong", "b":
			markdown.WriteString("**")
			markdown.WriteString(strings.TrimSpace(node.Text()))
			markdown.WriteString("**")
		case "em", "i":
			markdown.WriteString("*")
			markdown.WriteString(strings.TrimSpace(node.Text()))
			markdown.WriteString("*")
		case "hr":
			markdown.WriteString("\n---\n\n")
		case "table":
			markdown.WriteString("\n")
			node.Find("tr").Each(func(idx int, tr *goquery.Selection) {
				tr.Find("th, td").Each(func(i int, cell *goquery.Selection) {
					markdown.WriteString("| ")
					markdown.WriteString(strings.TrimSpace(cell.Text()))
					markdown.WriteString(" ")
				})
				markdown.WriteString("|\n")
				if idx == 0 {
					tr.Find("th").Each(func(i int, _ *goquery.Selection) {
						markdown.WriteString("|---")
					})
					markdown.WriteString("|\n")
				}
			})
			markdown.WriteString("\n")
		case "#text":
			text := node.Text()
			if strings.TrimSpace(text) != "" {
				markdown.WriteString(text)
			}
		default:
			t.processNode(node, markdown)
		}
	})
}

// basicTextExtraction extracts the text from the HTML content
func (t *WebFetchTool) basicTextExtraction(html string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	text := re.ReplaceAllString(html, " ")
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}
