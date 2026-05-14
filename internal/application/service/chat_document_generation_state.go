package service

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
)

func resolveLongDocumentAutoContinueMaxRounds(cfg *config.Config) int {
	if cfg != nil && cfg.LongDocument != nil && cfg.LongDocument.AutoContinueMaxRounds > 0 {
		return cfg.LongDocument.AutoContinueMaxRounds
	}
	return types.ChatDocumentGenerationRunDefaultMaxRounds
}

func resolveLongDocumentAutoContinueMinGrowthChars(cfg *config.Config) int {
	if cfg != nil && cfg.LongDocument != nil && cfg.LongDocument.AutoContinueMinGrowthChars > 0 {
		return cfg.LongDocument.AutoContinueMinGrowthChars
	}
	return 200
}

func resolveLongDocumentAutoContinueMaxLowGrowthRounds(cfg *config.Config) int {
	if cfg != nil && cfg.LongDocument != nil && cfg.LongDocument.AutoContinueMaxLowGrowthRounds > 0 {
		return cfg.LongDocument.AutoContinueMaxLowGrowthRounds
	}
	return 2
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func documentGenerationRunStateFromFeedback(feedback documentGenerationRuntimeFeedback) types.ChatDocumentGenerationRunState {
	return types.NormalizeChatDocumentGenerationRunState(types.ChatDocumentGenerationRunState{
		TaskKind:                feedback.TaskKind,
		ActiveArtifactID:        feedback.ActiveArtifactID,
		LastCompletionStatus:    feedback.LastCompletionStatus,
		LastFinishReason:        feedback.LastFinishReason,
		LastFailureReason:       feedback.LastFailureReason,
		LastDocumentStatus:      feedback.LastDocumentStatus,
		LastAutoContinueReason:  feedback.LastAutoContinueReason,
		AutoContinueRound:       feedback.AutoContinueRound,
		MaxAutoContinueRounds:   feedback.MaxAutoContinueRounds,
		MinGrowthChars:          feedback.MinGrowthChars,
		MaxLowGrowthRounds:      feedback.MaxLowGrowthRounds,
		LastSnapshotCharCount:   feedback.LastSnapshotCharCount,
		LowGrowthRounds:         feedback.LowGrowthRounds,
		CompletedCount:          feedback.CompletedCount,
		RemainingCount:          feedback.RemainingCount,
		NextSourceChunkStartSeq: feedback.NextSourceChunkStartSeq,
		NextSourceChunkEndSeq:   feedback.NextSourceChunkEndSeq,
		NextSection:             feedback.NextSection,
	})
}

func applyDocumentGenerationRunState(feedback documentGenerationRuntimeFeedback, state types.ChatDocumentGenerationRunState) documentGenerationRuntimeFeedback {
	normalized := types.NormalizeChatDocumentGenerationRunState(state)
	feedback.TaskKind = normalized.TaskKind
	feedback.ActiveArtifactID = normalized.ActiveArtifactID
	feedback.LastCompletionStatus = normalized.LastCompletionStatus
	feedback.LastFinishReason = normalized.LastFinishReason
	feedback.LastFailureReason = normalized.LastFailureReason
	feedback.LastDocumentStatus = normalized.LastDocumentStatus
	feedback.LastAutoContinueReason = normalized.LastAutoContinueReason
	feedback.AutoContinueRound = normalized.AutoContinueRound
	feedback.MaxAutoContinueRounds = normalized.MaxAutoContinueRounds
	feedback.MinGrowthChars = normalized.MinGrowthChars
	feedback.MaxLowGrowthRounds = normalized.MaxLowGrowthRounds
	feedback.LastSnapshotCharCount = normalized.LastSnapshotCharCount
	feedback.LowGrowthRounds = normalized.LowGrowthRounds
	feedback.CompletedCount = normalized.CompletedCount
	feedback.RemainingCount = normalized.RemainingCount
	feedback.NextSourceChunkStartSeq = normalized.NextSourceChunkStartSeq
	feedback.NextSourceChunkEndSeq = normalized.NextSourceChunkEndSeq
	feedback.NextSection = normalized.NextSection
	return feedback
}

func newDocumentGenerationRuntimeFeedback(taskKind string, maxRounds int, minGrowthChars int, maxLowGrowthRounds int) documentGenerationRuntimeFeedback {
	return applyDocumentGenerationRunState(documentGenerationRuntimeFeedback{}, types.ChatDocumentGenerationRunState{
		TaskKind:              strings.TrimSpace(taskKind),
		MaxAutoContinueRounds: maxRounds,
		MinGrowthChars:        minGrowthChars,
		MaxLowGrowthRounds:    maxLowGrowthRounds,
	})
}

func buildDocumentGenerationRunState(run *types.ChatDocumentGenerationRun, feedback documentGenerationRuntimeFeedback) types.ChatDocumentGenerationRunState {
	state := documentGenerationRunStateFromFeedback(feedback)
	if run != nil {
		if run.AutoContinueRound > state.AutoContinueRound {
			state.AutoContinueRound = run.AutoContinueRound
		}
		if run.MaxRounds > 0 && state.MaxAutoContinueRounds <= 0 {
			state.MaxAutoContinueRounds = run.MaxRounds
		}
	}
	return types.NormalizeChatDocumentGenerationRunState(state)
}

func withDocumentGenerationRunStateExtra(extra map[string]interface{}, run *types.ChatDocumentGenerationRun, feedback documentGenerationRuntimeFeedback) map[string]interface{} {
	state := buildDocumentGenerationRunState(run, feedback)
	data := state.Data()
	if len(data) == 0 {
		return extra
	}
	if extra == nil {
		extra = map[string]interface{}{}
	}
	extra["generation_run_state"] = data
	return extra
}

func (s *sessionService) RecordChatDocumentGenerationRunState(
	ctx context.Context,
	runID string,
	update types.ChatDocumentGenerationRunState,
) (*types.ChatDocumentGenerationRunState, error) {
	if s == nil || s.generationRunRepo == nil || strings.TrimSpace(runID) == "" {
		return nil, nil
	}
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return nil, nil
	}
	run, err := s.generationRunRepo.GetRunByID(ctx, tenantID, strings.TrimSpace(runID))
	if err != nil || run == nil {
		return nil, err
	}

	feedback := unmarshalGenerationRunRuntimeFeedback(run.RuntimeFeedbackJSON)
	state := buildDocumentGenerationRunState(run, feedback)
	patch := types.NormalizeChatDocumentGenerationRunState(update)

	state.MaxAutoContinueRounds = firstPositiveInt(patch.MaxAutoContinueRounds, state.MaxAutoContinueRounds, run.MaxRounds, resolveLongDocumentAutoContinueMaxRounds(s.cfg))
	state.MinGrowthChars = firstPositiveInt(patch.MinGrowthChars, state.MinGrowthChars, resolveLongDocumentAutoContinueMinGrowthChars(s.cfg))
	state.MaxLowGrowthRounds = firstPositiveInt(patch.MaxLowGrowthRounds, state.MaxLowGrowthRounds, resolveLongDocumentAutoContinueMaxLowGrowthRounds(s.cfg))

	previousArtifactID := state.ActiveArtifactID
	previousSnapshotCharCount := state.LastSnapshotCharCount

	if patch.TaskKind != "" {
		state.TaskKind = patch.TaskKind
	}
	if patch.ActiveArtifactID != "" {
		state.ActiveArtifactID = patch.ActiveArtifactID
	}
	if patch.LastCompletionStatus != "" {
		state.LastCompletionStatus = patch.LastCompletionStatus
	}
	if patch.LastFinishReason != "" {
		state.LastFinishReason = patch.LastFinishReason
	}
	if patch.LastFailureReason != "" {
		state.LastFailureReason = patch.LastFailureReason
	}
	if patch.LastDocumentStatus != "" {
		state.LastDocumentStatus = patch.LastDocumentStatus
	}
	if patch.LastAutoContinueReason != "" {
		state.LastAutoContinueReason = patch.LastAutoContinueReason
	}
	if patch.AutoContinueRound > state.AutoContinueRound {
		state.AutoContinueRound = patch.AutoContinueRound
	}
	if patch.CompletedCount > 0 {
		state.CompletedCount = patch.CompletedCount
	}
	if patch.RemainingCount > 0 || (patch.CompletedCount > 0 && patch.RemainingCount == 0) {
		state.RemainingCount = patch.RemainingCount
	}
	if patch.NextSourceChunkStartSeq > 0 || patch.NextSourceChunkEndSeq > 0 {
		state.NextSourceChunkStartSeq = patch.NextSourceChunkStartSeq
		state.NextSourceChunkEndSeq = patch.NextSourceChunkEndSeq
	}
	if patch.NextSection != "" || (patch.CompletedCount > 0 && patch.RemainingCount == 0) {
		state.NextSection = patch.NextSection
	}
	if patch.LastSnapshotCharCount > 0 {
		if previousSnapshotCharCount > 0 && patch.ActiveArtifactID != "" && patch.ActiveArtifactID != previousArtifactID {
			if patch.LastSnapshotCharCount-previousSnapshotCharCount < state.MinGrowthChars {
				state.LowGrowthRounds++
			} else {
				state.LowGrowthRounds = 0
			}
		}
		state.LastSnapshotCharCount = patch.LastSnapshotCharCount
	}

	state = types.NormalizeChatDocumentGenerationRunState(state)
	feedback = applyDocumentGenerationRunState(feedback, state)
	run.RuntimeFeedbackJSON = marshalGenerationRunJSON(normalizeDocumentGenerationRuntimeFeedback(feedback))
	if state.AutoContinueRound > run.AutoContinueRound {
		run.AutoContinueRound = state.AutoContinueRound
	}
	if state.MaxAutoContinueRounds > 0 {
		run.MaxRounds = state.MaxAutoContinueRounds
	}
	if err := s.generationRunRepo.UpdateRun(ctx, run); err != nil {
		return nil, err
	}
	return &state, nil
}
