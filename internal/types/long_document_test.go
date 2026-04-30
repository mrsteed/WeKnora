package types

import (
	"strings"
	"testing"
)

func TestBuildLongDocumentTaskIdempotencyKey_IsBounded(t *testing.T) {
	key := BuildLongDocumentTaskIdempotencyKey(
		10000,
		"b81c60d8-d261-4778-9df3-f42d62016bf9",
		"8db582da-d19f-4e3c-bae0-70e418eb0746",
		LongDocumentTaskKindTranslation,
		"请对文件进行全文翻译，每个字都需要进行翻译为中文，图和表也需要进行翻译，并尽量按照原文的格式输出为markdown格式的文件。",
		"deepseek-v4-pro",
	)
	if got := len(key); got > LongDocumentTaskIdempotencyKeyMaxLength {
		t.Fatalf("expected idempotency key length <= %d, got %d (%q)", LongDocumentTaskIdempotencyKeyMaxLength, got, key)
	}
	if !strings.HasPrefix(key, "ldt:") {
		t.Fatalf("expected hashed idempotency key prefix, got %q", key)
	}
}

func TestBuildLongDocumentTaskIdempotencyKey_ChangesWithSummaryModel(t *testing.T) {
	flashKey := BuildLongDocumentTaskIdempotencyKey(
		10000,
		"session-1",
		"knowledge-1",
		LongDocumentTaskKindTranslation,
		"translate full document",
		"deepseek-v4-flash",
	)
	proKey := BuildLongDocumentTaskIdempotencyKey(
		10000,
		"session-1",
		"knowledge-1",
		LongDocumentTaskKindTranslation,
		"translate full document",
		"deepseek-v4-pro",
	)
	if flashKey == proKey {
		t.Fatalf("expected idempotency key to change with summary model, got %q", flashKey)
	}
}

func TestNormalizeLongDocumentTaskIdempotencyKey_PreservesShortKey(t *testing.T) {
	key := NormalizeLongDocumentTaskIdempotencyKey("custom-short-key")
	if key != "custom-short-key" {
		t.Fatalf("expected short key to be preserved, got %q", key)
	}
}

func TestNormalizeLongDocumentTaskIdempotencyKey_HashesLongKey(t *testing.T) {
	longKey := strings.Repeat("x", LongDocumentTaskIdempotencyKeyMaxLength+32)
	normalized := NormalizeLongDocumentTaskIdempotencyKey(longKey)
	if normalized == longKey {
		t.Fatal("expected long key to be normalized")
	}
	if got := len(normalized); got > LongDocumentTaskIdempotencyKeyMaxLength {
		t.Fatalf("expected normalized key length <= %d, got %d", LongDocumentTaskIdempotencyKeyMaxLength, got)
	}
	if !strings.HasPrefix(normalized, "ldt:") {
		t.Fatalf("expected normalized key prefix ldt:, got %q", normalized)
	}
}
