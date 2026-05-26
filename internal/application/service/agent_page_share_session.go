package service

import (
	"context"
	crand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

const defaultAgentPageShareSessionTTL = 24 * time.Hour

var (
	ErrAgentPageShareSessionNotFound     = errors.New("agent page share session not found")
	ErrAgentPageShareSessionExpired      = errors.New("agent page share session expired")
	ErrAgentPageShareSessionForbidden    = errors.New("agent page share session forbidden")
	ErrAgentPageShareSessionLimitReached = errors.New("agent page share session limit reached")
)

type agentPageShareSessionService struct {
	shareRepo        interfaces.AgentPageShareRepository
	shareSessionRepo interfaces.AgentPageShareSessionRepository
	sessionRepo      interfaces.SessionRepository
	customAgent      interfaces.CustomAgentService
}

// NewAgentPageShareSessionService creates a service for anonymous agent share sessions.
func NewAgentPageShareSessionService(
	shareRepo interfaces.AgentPageShareRepository,
	shareSessionRepo interfaces.AgentPageShareSessionRepository,
	sessionRepo interfaces.SessionRepository,
	customAgent interfaces.CustomAgentService,
) interfaces.AgentPageShareSessionService {
	return &agentPageShareSessionService{
		shareRepo:        shareRepo,
		shareSessionRepo: shareSessionRepo,
		sessionRepo:      sessionRepo,
		customAgent:      customAgent,
	}
}

// CreateAnonymousSession creates a new anonymous share-page session and returns the one-time visitor token.
func (s *agentPageShareSessionService) CreateAnonymousSession(ctx context.Context, shareCode string, clientIP string, userAgent string) (*types.AgentPageShareSessionCreateResult, error) {
	share, agent, err := s.resolveActiveShare(ctx, shareCode)
	if err != nil {
		return nil, err
	}
	if share.AnonymousSessionLimit > 0 {
		count, err := s.shareSessionRepo.CountByShareID(ctx, share.ID)
		if err != nil {
			return nil, err
		}
		if count >= int64(share.AnonymousSessionLimit) {
			return nil, ErrAgentPageShareSessionLimitReached
		}
	}

	now := time.Now()
	expiresAt := now.Add(defaultAgentPageShareSessionTTL)
	visitorToken := generateAgentPageShareVisitorToken()
	session := &types.Session{
		Title:              strings.TrimSpace(agent.Name),
		Description:        strings.TrimSpace(agent.Description),
		TenantID:           share.SourceTenantID,
		AccessMode:         types.SessionAccessModeAgentSharePage,
		AgentPageShareID:   share.ID,
		AnonymousVisitorID: uuid.New().String(),
		VisitorTokenHash:   sha256Hex(visitorToken),
		VisitorIPHash:      sha256Hex(strings.TrimSpace(clientIP)),
		UserAgentHash:      sha256Hex(strings.TrimSpace(userAgent)),
		ExpiresAt:          &expiresAt,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	created, err := s.sessionRepo.Create(ctx, session)
	if err != nil {
		return nil, err
	}
	return &types.AgentPageShareSessionCreateResult{
		SessionID:          created.ID,
		AnonymousVisitorID: created.AnonymousVisitorID,
		VisitorToken:       visitorToken,
		ExpiresAt:          expiresAt,
	}, nil
}

// ValidateAnonymousSession validates share availability, session ownership, and visitor token.
func (s *agentPageShareSessionService) ValidateAnonymousSession(ctx context.Context, shareCode string, sessionID string, visitorToken string) (*types.AgentPageShareSessionContext, error) {
	share, agent, err := s.resolveActiveShare(ctx, shareCode)
	if err != nil {
		return nil, err
	}
	session, err := s.shareSessionRepo.GetByID(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		if errors.Is(err, repository.ErrAgentPageShareSessionNotFound) {
			return nil, ErrAgentPageShareSessionNotFound
		}
		return nil, err
	}
	if session.AgentPageShareID != share.ID {
		return nil, ErrAgentPageShareSessionNotFound
	}
	if session.ExpiresAt != nil && time.Now().After(*session.ExpiresAt) {
		return nil, ErrAgentPageShareSessionExpired
	}
	if !constantTimeHashEquals(session.VisitorTokenHash, sha256Hex(strings.TrimSpace(visitorToken))) {
		return nil, ErrAgentPageShareSessionForbidden
	}
	return &types.AgentPageShareSessionContext{
		Share:   share,
		Agent:   agent,
		Session: session,
	}, nil
}

func (s *agentPageShareSessionService) resolveActiveShare(ctx context.Context, shareCode string) (*types.AgentPageShare, *types.CustomAgent, error) {
	share, err := s.shareRepo.GetByShareCode(ctx, strings.TrimSpace(shareCode))
	if err != nil {
		if errors.Is(err, repository.ErrAgentPageShareNotFound) {
			return nil, nil, ErrAgentPageShareNotFound
		}
		return nil, nil, err
	}
	if !isAgentPageShareAvailable(share) {
		return nil, nil, ErrAgentPageShareUnavailable
	}
	agent, err := s.customAgent.GetAgentByIDAndTenant(ctx, share.AgentID, share.SourceTenantID)
	if err != nil {
		if errors.Is(err, ErrAgentNotFound) {
			return nil, nil, ErrSharedAgentNotFound
		}
		return nil, nil, err
	}
	agent.EnsureDefaults()
	return share, agent, nil
}

func generateAgentPageShareVisitorToken() string {
	b := make([]byte, 32)
	if _, err := crand.Read(b); err != nil {
		return uuid.New().String()
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func constantTimeHashEquals(left string, right string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
