package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/redis/go-redis/v9"
)

type attachmentTempKBStateService struct {
	redisClient          *redis.Client
	knowledgeService     interfaces.KnowledgeService
	knowledgeBaseService interfaces.KnowledgeBaseService
}

// NewAttachmentTempKBStateService creates a session-scoped attachment temp KB state service.
func NewAttachmentTempKBStateService(
	redisClient *redis.Client,
	knowledgeService interfaces.KnowledgeService,
	knowledgeBaseService interfaces.KnowledgeBaseService,
) interfaces.AttachmentTempKBStateService {
	return &attachmentTempKBStateService{
		redisClient:          redisClient,
		knowledgeService:     knowledgeService,
		knowledgeBaseService: knowledgeBaseService,
	}
}

func attachmentTempKBStateKey(sessionID string) string {
	return fmt.Sprintf("attachment-tempkb:%s", sessionID)
}

func (s *attachmentTempKBStateService) GetAttachmentTempKBState(
	ctx context.Context,
	sessionID string,
) *types.AttachmentTempKBState {
	state := &types.AttachmentTempKBState{}
	state.EnsureDefaults()
	if s == nil || s.redisClient == nil || strings.TrimSpace(sessionID) == "" {
		return state
	}
	raw, err := s.redisClient.Get(ctx, attachmentTempKBStateKey(sessionID)).Bytes()
	if err != nil || len(raw) == 0 {
		return state
	}
	if err := json.Unmarshal(raw, state); err != nil {
		logger.Warnf(ctx, "Failed to unmarshal attachment temp KB state for session %s: %v", sessionID, err)
		return &types.AttachmentTempKBState{KnowledgeIDs: []string{}, AttachmentKnowledge: map[string]string{}}
	}
	state.EnsureDefaults()
	return state
}

func (s *attachmentTempKBStateService) SaveAttachmentTempKBState(
	ctx context.Context,
	sessionID string,
	state *types.AttachmentTempKBState,
) {
	if s == nil || s.redisClient == nil || strings.TrimSpace(sessionID) == "" || state == nil {
		return
	}
	state.EnsureDefaults()
	state.UpdatedAt = time.Now()
	b, err := json.Marshal(state)
	if err != nil {
		logger.Warnf(ctx, "Failed to marshal attachment temp KB state for session %s: %v", sessionID, err)
		return
	}
	if err := s.redisClient.Set(ctx, attachmentTempKBStateKey(sessionID), b, 0).Err(); err != nil {
		logger.Warnf(ctx, "Failed to save attachment temp KB state for session %s: %v", sessionID, err)
	}
}

func (s *attachmentTempKBStateService) DeleteAttachmentTempKBState(ctx context.Context, sessionID string) error {
	if s == nil || s.redisClient == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	raw, getErr := s.redisClient.Get(ctx, attachmentTempKBStateKey(sessionID)).Bytes()
	if getErr != nil || len(raw) == 0 {
		return nil
	}
	state := &types.AttachmentTempKBState{}
	if err := json.Unmarshal(raw, state); err != nil {
		_ = s.redisClient.Del(ctx, attachmentTempKBStateKey(sessionID)).Err()
		return nil
	}
	state.EnsureDefaults()

	if strings.TrimSpace(state.KBID) != "" {
		logger.Infof(ctx, "Cleaning attachment temp KB for session %s: %s", sessionID, state.KBID)
		seenKnowledge := make(map[string]struct{})
		for _, knowledgeID := range state.KnowledgeIDs {
			if strings.TrimSpace(knowledgeID) == "" {
				continue
			}
			seenKnowledge[knowledgeID] = struct{}{}
		}
		for _, knowledgeID := range state.AttachmentKnowledge {
			if strings.TrimSpace(knowledgeID) == "" {
				continue
			}
			seenKnowledge[knowledgeID] = struct{}{}
		}
		for knowledgeID := range seenKnowledge {
			if delErr := s.knowledgeService.DeleteKnowledge(ctx, knowledgeID); delErr != nil {
				logger.Warnf(ctx, "Failed to delete attachment temp knowledge %s: %v", knowledgeID, delErr)
			}
		}
		if delErr := s.knowledgeBaseService.DeleteKnowledgeBase(ctx, state.KBID); delErr != nil {
			logger.Warnf(ctx, "Failed to delete attachment temp KB %s: %v", state.KBID, delErr)
		}
	}

	if err := s.redisClient.Del(ctx, attachmentTempKBStateKey(sessionID)).Err(); err != nil {
		logger.Warnf(ctx, "Failed to delete attachment temp KB state key for session %s: %v", sessionID, err)
		return fmt.Errorf("failed to delete Redis key: %w", err)
	}
	return nil
}

func (s *attachmentTempKBStateService) CleanupExpiredAttachmentTempKBStates(ctx context.Context, maxIdle time.Duration) (int, error) {
	if s == nil || s.redisClient == nil || maxIdle <= 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-maxIdle)
	var (
		cursor  uint64
		cleaned int
	)
	for {
		keys, nextCursor, err := s.redisClient.Scan(ctx, cursor, attachmentTempKBStateKey("*"), 100).Result()
		if err != nil {
			return cleaned, err
		}
		for _, key := range keys {
			raw, getErr := s.redisClient.Get(ctx, key).Bytes()
			if getErr != nil || len(raw) == 0 {
				continue
			}
			state := &types.AttachmentTempKBState{}
			if err := json.Unmarshal(raw, state); err != nil {
				continue
			}
			state.EnsureDefaults()
			if !state.UpdatedAt.IsZero() && state.UpdatedAt.After(cutoff) {
				continue
			}
			sessionID := strings.TrimPrefix(key, attachmentTempKBStateKey(""))
			if err := s.DeleteAttachmentTempKBState(ctx, sessionID); err != nil {
				logger.Warnf(ctx, "Failed to cleanup expired attachment temp KB state for session %s: %v", sessionID, err)
				continue
			}
			cleaned++
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return cleaned, nil
}
