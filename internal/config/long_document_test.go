package config

import (
	"testing"
)

func TestApplyLongDocumentDefaults_InitializesMissingSection(t *testing.T) {
	cfg := &Config{}

	applyLongDocumentDefaults(cfg)

	if cfg.LongDocument == nil {
		t.Fatal("expected long document config to be initialized")
	}
	if !cfg.LongDocument.EnableTaskRouter || !cfg.LongDocument.EnableTaskWorker || !cfg.LongDocument.EnableArtifactDownload {
		t.Fatal("expected long document feature flags to default to enabled when section is absent")
	}
	if cfg.LongDocument.BatchChunkSize != 8 || cfg.LongDocument.BatchMaxChars != 24000 || cfg.LongDocument.BatchRetryLimit != 3 || cfg.LongDocument.TaskPollIntervalSec != 3 {
		t.Fatalf("unexpected defaults: %+v", cfg.LongDocument)
	}
}

func TestApplyLongDocumentDefaults_PreservesExplicitFalse(t *testing.T) {
	cfg := &Config{LongDocument: &LongDocumentConfig{
		EnableTaskRouter:       false,
		EnableTaskWorker:       false,
		EnableArtifactDownload: false,
	}}

	applyLongDocumentDefaults(cfg)

	if cfg.LongDocument.EnableTaskRouter || cfg.LongDocument.EnableTaskWorker || cfg.LongDocument.EnableArtifactDownload {
		t.Fatalf("expected explicit false flags to be preserved, got %+v", cfg.LongDocument)
	}
}
