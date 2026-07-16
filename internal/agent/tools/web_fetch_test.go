package tools

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestResolveWebFetchChromedpBaseDirDefault(t *testing.T) {
	t.Setenv(webFetchChromedpBaseDirEnv, "")
	if got := resolveWebFetchChromedpBaseDir(); got != webFetchChromedpDefaultBaseDir {
		t.Fatalf("resolveWebFetchChromedpBaseDir() = %q, want %q", got, webFetchChromedpDefaultBaseDir)
	}
}

func TestPrepareChromedpProfileDirUsesConfiguredBaseDir(t *testing.T) {
	baseDir := t.TempDir()
	tool := &WebFetchTool{chromedpBaseDir: baseDir}

	profileDir, err := tool.prepareChromedpProfileDir()
	if err != nil {
		t.Fatalf("prepareChromedpProfileDir() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(profileDir) })

	if !strings.HasPrefix(profileDir, baseDir) {
		t.Fatalf("profileDir %q does not use configured baseDir %q", profileDir, baseDir)
	}

	if err := os.WriteFile(profileDir+"/probe", []byte("ok"), 0o600); err != nil {
		t.Fatalf("profileDir %q is not writable: %v", profileDir, err)
	}
}

func TestHTTPClientForFetchSetsServerNameForHTTPS(t *testing.T) {
	tool := NewWebFetchTool(nil)
	vp := &validatedParams{URL: "https://example.com/path", Host: "example.com", Port: "443"}

	client := tool.httpClientForFetch(vp)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", client.Transport)
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	if got := transport.TLSClientConfig.ServerName; got != vp.Host {
		t.Fatalf("ServerName = %q, want %q", got, vp.Host)
	}
}

func TestSummarizeWebFetchResultsMarksAllFetchFailed(t *testing.T) {
	results := []*webFetchItemResult{
		{data: map[string]interface{}{"fetch_status": "failed"}},
		{data: map[string]interface{}{"fetch_status": "failed"}},
	}

	summary := summarizeWebFetchResults(results)
	if !summary.AllFetchFailed {
		t.Fatal("expected AllFetchFailed to be true")
	}
	if summary.AnswerBasis != "web_search_snippets_only" {
		t.Fatalf("AnswerBasis = %q", summary.AnswerBasis)
	}
	if summary.FetchedCount != 0 || summary.FailedCount != 2 {
		t.Fatalf("summary counts = %+v", summary)
	}
	if !strings.Contains(summary.StatusNotice, "web_search snippets") {
		t.Fatalf("unexpected status notice: %q", summary.StatusNotice)
	}
}

func TestSummarizeWebFetchResultsMarksPartialFetch(t *testing.T) {
	results := []*webFetchItemResult{
		{data: map[string]interface{}{"fetch_status": "fetched", "raw_content": "ok"}},
		{data: map[string]interface{}{"fetch_status": "failed"}},
	}

	summary := summarizeWebFetchResults(results)
	if summary.AllFetchFailed {
		t.Fatal("expected AllFetchFailed to be false")
	}
	if summary.AnswerBasis != "mixed_fulltext_and_snippets" {
		t.Fatalf("AnswerBasis = %q", summary.AnswerBasis)
	}
	if summary.FetchedCount != 1 || summary.FailedCount != 1 {
		t.Fatalf("summary counts = %+v", summary)
	}
	if !strings.Contains(summary.StatusNotice, "1 of 2") {
		t.Fatalf("unexpected status notice: %q", summary.StatusNotice)
	}
}