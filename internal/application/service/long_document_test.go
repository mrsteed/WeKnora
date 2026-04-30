package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestLongDocumentInferTaskKind(t *testing.T) {
	svc := &longDocumentTaskService{}
	if got := svc.InferTaskKind(context.Background(), "请把这篇文档全文翻译成 markdown 文件", []string{"knowledge-1"}); got != types.LongDocumentTaskKindTranslation {
		t.Fatalf("expected translation task kind, got %q", got)
	}
	if got := svc.InferTaskKind(context.Background(), "帮我总结一下这个文档", []string{"knowledge-1"}); got != "" {
		t.Fatalf("expected empty task kind for non translation query, got %q", got)
	}
	if got := svc.InferTaskKind(context.Background(), "translate the full document to markdown", []string{"knowledge-1", "knowledge-2"}); got != "" {
		t.Fatalf("expected empty task kind for multiple knowledge ids, got %q", got)
	}
}

func TestLongDocumentPlanBatches(t *testing.T) {
	svc := &longDocumentTaskService{
		cfg: &config.Config{LongDocument: &config.LongDocumentConfig{BatchChunkSize: 2, BatchMaxChars: 60}},
	}
	chunks := []*types.Chunk{
		{ChunkIndex: 1, Content: "第一段内容"},
		{ChunkIndex: 2, Content: "第二段内容"},
		{ChunkIndex: 3, Content: "第三段内容，长度稍微长一点，确保可以触发下一批"},
	}
	plans := svc.planBatches(chunks)
	if len(plans) != 2 {
		t.Fatalf("expected 2 plans, got %d", len(plans))
	}
	if plans[0].start != 1 || plans[0].end != 2 {
		t.Fatalf("unexpected first plan range: %+v", plans[0])
	}
	if plans[1].start != 3 || plans[1].end != 3 {
		t.Fatalf("unexpected second plan range: %+v", plans[1])
	}
}

func TestLongDocumentPlanBatches_RemovesChunkOverlapWithoutInjectingBlankLines(t *testing.T) {
	svc := &longDocumentTaskService{
		cfg: &config.Config{LongDocument: &config.LongDocumentConfig{BatchChunkSize: 4, BatchMaxChars: 200}},
	}
	original := "# Title\n\nFirst paragraph continues here.\n\nSecond paragraph."
	firstChunk := "# Title\n\nFirst paragraph continues"
	secondChunk := "paragraph continues here.\n\nSecond paragraph."
	chunks := []*types.Chunk{
		{
			ChunkIndex: 1,
			Content:    firstChunk,
			StartAt:    0,
			EndAt:      len([]rune(firstChunk)),
		},
		{
			ChunkIndex: 2,
			Content:    secondChunk,
			StartAt:    len([]rune("# Title\n\nFirst ")),
			EndAt:      len([]rune(original)),
		},
	}

	plans := svc.planBatches(chunks)
	if len(plans) != 1 {
		t.Fatalf("expected 1 plan, got %d", len(plans))
	}
	if plans[0].input != original {
		t.Fatalf("expected restored original markdown, got %q", plans[0].input)
	}
	if strings.Contains(plans[0].input, "continues\n\nparagraph") {
		t.Fatalf("expected no injected blank line inside paragraph, got %q", plans[0].input)
	}
}

func TestSanitizeGeneratedMarkdown(t *testing.T) {
	input := "```markdown\n# 标题\n\n正文\n```"
	if got := sanitizeGeneratedMarkdown(input); got != "# 标题\n\n正文" {
		t.Fatalf("unexpected sanitized markdown: %q", got)
	}
}

func TestSanitizeGeneratedMarkdown_ReplacesFormFeed(t *testing.T) {
	input := "# 标题\f\n正文"
	if got := sanitizeGeneratedMarkdown(input); got != "# 标题\n\n\n正文" {
		t.Fatalf("unexpected sanitized markdown with form feed: %q", got)
	}
}

func TestStorageBackendFromPath(t *testing.T) {
	testCases := []struct {
		name     string
		filePath string
		want     string
	}{
		{name: "local path", filePath: "local://1/exports/demo.md", want: "local"},
		{name: "minio path", filePath: "minio://bucket/object.md", want: "minio"},
		{name: "empty path", filePath: "", want: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := storageBackendFromPath(testCase.filePath); got != testCase.want {
				t.Fatalf("expected backend %q, got %q", testCase.want, got)
			}
		})
	}
}

func TestPrepareTaskForRetry(t *testing.T) {
	completedAt := time.Now().Add(-2 * time.Minute)
	task := &types.LongDocumentTask{
		ID:               "task-1",
		Status:           types.LongDocumentTaskStatusPartial,
		ArtifactID:       "artifact-1",
		ArtifactPath:     "local://artifact.md",
		CompletedBatches: 1,
		FailedBatches:    1,
		CompletedAt:      &completedAt,
		QualityStatus:    "partial",
	}
	batches := []*types.LongDocumentTaskBatch{
		{ID: "batch-1", Status: types.LongDocumentBatchStatusCompleted},
		{ID: "batch-2", Status: types.LongDocumentBatchStatusFailed, ErrorMessage: "boom", OutputPayload: "bad", RetryCount: 2, QualityStatus: "failed"},
	}

	if !prepareTaskForRetry(task, batches) {
		t.Fatal("expected task to be retryable")
	}
	if task.Status != types.LongDocumentTaskStatusPending || task.ArtifactID != "" || task.ArtifactPath != "" {
		t.Fatalf("expected task to be reset for retry, got %+v", task)
	}
	if task.CompletedBatches != 1 || task.FailedBatches != 0 {
		t.Fatalf("unexpected task counters after retry reset: %+v", task)
	}
	if batches[1].Status != types.LongDocumentBatchStatusPending || batches[1].ErrorMessage != "" || batches[1].OutputPayload != "" {
		t.Fatalf("expected failed batch to be reset, got %+v", batches[1])
	}
}

func TestBuildLongDocumentTaskEvents(t *testing.T) {
	startedAt := time.Now().Add(-3 * time.Minute)
	completedAt := startedAt.Add(30 * time.Second)
	assembledAt := completedAt.Add(15 * time.Second)
	taskCompletedAt := assembledAt.Add(10 * time.Second)
	task := &types.LongDocumentTask{
		ID:               "task-1",
		Status:           types.LongDocumentTaskStatusCompleted,
		TotalBatches:     1,
		CompletedBatches: 1,
		ArtifactPath:     "local://artifact.md",
		QualityStatus:    "passed",
		UpdatedAt:        taskCompletedAt,
		CompletedAt:      &taskCompletedAt,
	}
	batches := []*types.LongDocumentTaskBatch{{
		ID:                  "batch-1",
		BatchNo:             1,
		Status:              types.LongDocumentBatchStatusCompleted,
		RetryCount:          0,
		QualityStatus:       "passed",
		InputTokenEstimate:  120,
		OutputTokenEstimate: 100,
		ModelName:           "demo-model",
		StartedAt:           &startedAt,
		CompletedAt:         &completedAt,
		UpdatedAt:           completedAt,
	}}
	artifact := &types.LongDocumentArtifact{
		ID:             "artifact-1",
		Status:         types.LongDocumentArtifactStatusAvailable,
		FileName:       "demo.md",
		FileType:       "text/markdown",
		FileSize:       1024,
		Checksum:       "checksum",
		StorageBackend: "local",
		CreatedAt:      assembledAt,
	}

	events := buildLongDocumentTaskEvents(task, batches, artifact)
	if len(events) == 0 {
		t.Fatal("expected structured task events")
	}
	want := map[string]bool{
		"task_started":       false,
		"batch_started":      false,
		"batch_completed":    false,
		"task_assembling":    false,
		"artifact_available": false,
		"task_completed":     false,
		"task.snapshot":      false,
	}
	for _, event := range events {
		if _, ok := want[event.Type]; ok {
			want[event.Type] = true
		}
	}
	for eventType, seen := range want {
		if !seen {
			t.Fatalf("expected event %q to be present, got %+v", eventType, events)
		}
	}
}

func TestResolveLongDocumentArtifactProvider(t *testing.T) {
	testCases := []struct {
		name      string
		knowledge *types.Knowledge
		kb        *types.KnowledgeBase
		artifact  string
		want      string
	}{
		{
			name:      "artifact path wins",
			knowledge: &types.Knowledge{FilePath: "local://10000/demo.pdf"},
			kb:        &types.KnowledgeBase{},
			artifact:  "minio://bucket/export.md",
			want:      "minio",
		},
		{
			name:      "kb provider before knowledge path",
			knowledge: &types.Knowledge{FilePath: "local://10000/demo.pdf"},
			kb:        &types.KnowledgeBase{StorageProviderConfig: &types.StorageProviderConfig{Provider: "minio"}},
			want:      "minio",
		},
		{
			name:      "fallback to knowledge file path",
			knowledge: &types.Knowledge{FilePath: "minio://bucket/source.pdf"},
			kb:        &types.KnowledgeBase{},
			want:      "minio",
		},
		{
			name:      "empty when unresolved",
			knowledge: &types.Knowledge{FilePath: "/tmp/source.pdf"},
			kb:        &types.KnowledgeBase{},
			want:      "",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := resolveLongDocumentArtifactProvider(testCase.knowledge, testCase.kb, testCase.artifact); got != testCase.want {
				t.Fatalf("expected provider %q, got %q", testCase.want, got)
			}
		})
	}
}
