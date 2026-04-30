package session

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestBuildLongDocumentTaskCreateRequest(t *testing.T) {
	request := buildLongDocumentTaskCreateRequest(
		"session-1",
		"请把这篇文档全文翻译成 Markdown",
		types.LongDocumentTaskKindTranslation,
		"api",
		[]string{"knowledge-1"},
		"deepseek-v4-pro",
		"Chinese (Simplified)",
	)
	if request == nil {
		t.Fatal("expected create request")
	}
	if request.SessionID != "session-1" || request.KnowledgeID != "knowledge-1" {
		t.Fatalf("unexpected request identity fields: %+v", request)
	}
	if request.OutputFormat != types.LongDocumentOutputFormatMarkdown {
		t.Fatalf("expected markdown output, got %q", request.OutputFormat)
	}
	if request.Channel != "api" {
		t.Fatalf("expected channel to be preserved, got %q", request.Channel)
	}
	if request.SummaryModelID != "deepseek-v4-pro" {
		t.Fatalf("expected summary model override to be preserved, got %q", request.SummaryModelID)
	}
	if request.Options.TargetLanguage != "Chinese (Simplified)" {
		t.Fatalf("expected target language to be set, got %+v", request.Options)
	}
}

func TestBuildLongDocumentTaskCreateRequestRejectsAmbiguousKnowledgeSelection(t *testing.T) {
	request := buildLongDocumentTaskCreateRequest(
		"session-1",
		"translate full document",
		types.LongDocumentTaskKindTranslation,
		"web",
		[]string{"knowledge-1", "knowledge-2"},
		"deepseek-v4-pro",
		"English",
	)
	if request != nil {
		t.Fatalf("expected nil request for ambiguous knowledge selection, got %+v", request)
	}
}
