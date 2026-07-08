package types

import (
	"context"
	"testing"
)

func TestApplyBuiltinAgentLocalizedMetadata_PrefersUILocale(t *testing.T) {
	origEntries := builtinAgentEntries
	origRegistry := BuiltinAgentRegistry
	builtinAgentEntries = map[string]*BuiltinAgentEntry{
		BuiltinQuickAnswerID: {
			ID:        BuiltinQuickAnswerID,
			IsBuiltin: true,
			Avatar:    "📚",
			I18n: map[string]BuiltinAgentI18n{
				"default": {Name: "Quick Answer", Description: "English"},
				"zh-CN":   {Name: "快速问答", Description: "中文"},
			},
			Config: CustomAgentConfig{AgentMode: AgentModeQuickAnswer},
		},
	}
	BuiltinAgentRegistry = map[string]func(uint64) *CustomAgent{}
	rebuildRegistryFromConfig()
	t.Cleanup(func() {
		builtinAgentEntries = origEntries
		BuiltinAgentRegistry = origRegistry
	})

	ctx := context.WithValue(context.Background(), UILocaleContextKey, "zh-CN")
	ctx = context.WithValue(ctx, LanguageContextKey, "en-US")
	override := &CustomAgent{
		ID:          BuiltinQuickAnswerID,
		TenantID:    7,
		Name:        "Quick Answer",
		Description: "English",
		Config:      CustomAgentConfig{ModelID: "m1"},
	}

	got := ApplyBuiltinAgentLocalizedMetadata(ctx, override)
	if got.Name != "快速问答" {
		t.Fatalf("localized name = %q, want zh-CN", got.Name)
	}
	if got.Description != "中文" {
		t.Fatalf("localized description = %q, want zh-CN", got.Description)
	}
	if got.Config.ModelID != "m1" {
		t.Fatalf("config override lost, got model_id=%q", got.Config.ModelID)
	}
}