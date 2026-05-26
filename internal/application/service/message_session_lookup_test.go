package service

import (
	"context"
	"errors"
	"testing"

	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type messageLookupSessionRepoStub struct {
	getCalled bool
	getErr    error
	getResult *types.Session
}

func (s *messageLookupSessionRepoStub) Create(context.Context, *types.Session) (*types.Session, error) {
	return nil, nil
}

func (s *messageLookupSessionRepoStub) Get(context.Context, uint64, string, string) (*types.Session, error) {
	s.getCalled = true
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.getResult, nil
}

func (s *messageLookupSessionRepoStub) GetByTenantAndUser(context.Context, uint64, string) ([]*types.Session, error) {
	return nil, nil
}

func (s *messageLookupSessionRepoStub) GetPagedByTenantAndUser(context.Context, uint64, string, *types.Pagination) ([]*types.Session, int64, error) {
	return nil, 0, nil
}

func (s *messageLookupSessionRepoStub) QueryPaged(context.Context, *types.SessionListQuery) ([]*types.SessionListItem, int64, error) {
	return nil, 0, nil
}

func (s *messageLookupSessionRepoStub) Update(context.Context, *types.Session) error { return nil }

func (s *messageLookupSessionRepoStub) SetPinned(context.Context, uint64, string, string, bool) (int64, error) {
	return 0, nil
}

func (s *messageLookupSessionRepoStub) Delete(context.Context, uint64, string, string) error {
	return nil
}

func (s *messageLookupSessionRepoStub) BatchDelete(context.Context, uint64, []string) error {
	return nil
}

func (s *messageLookupSessionRepoStub) DeleteAllByTenantID(context.Context, uint64) error { return nil }

type messageLookupShareSessionRepoStub struct {
	getCalled bool
	getErr    error
	getResult *types.Session
}

func (s *messageLookupShareSessionRepoStub) GetByID(context.Context, string) (*types.Session, error) {
	s.getCalled = true
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.getResult, nil
}

func (s *messageLookupShareSessionRepoStub) CountByShareID(context.Context, string) (int64, error) {
	return 0, nil
}

func TestEnsureSessionAccessible_AllowsAnonymousSharePageSession(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.SessionTenantIDContextKey, uint64(42))
	shareRepo := &messageLookupShareSessionRepoStub{getResult: &types.Session{ID: "share-session", TenantID: 42, AccessMode: types.SessionAccessModeAgentSharePage}}
	platformRepo := &messageLookupSessionRepoStub{getErr: errors.New("platform repo should not be used")}
	svc := &messageService{sessionRepo: platformRepo, shareSessionRepo: shareRepo}

	err := svc.ensureSessionAccessible(ctx, 42, "", "share-session")
	require.NoError(t, err)
	require.True(t, shareRepo.getCalled)
	require.False(t, platformRepo.getCalled)
}

func TestEnsureSessionAccessible_FallsBackToPlatformSessionLookup(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.SessionTenantIDContextKey, uint64(42))
	shareRepo := &messageLookupShareSessionRepoStub{getErr: apprepo.ErrAgentPageShareSessionNotFound}
	platformRepo := &messageLookupSessionRepoStub{getResult: &types.Session{ID: "platform-session", TenantID: 42}}
	svc := &messageService{sessionRepo: platformRepo, shareSessionRepo: shareRepo}

	err := svc.ensureSessionAccessible(ctx, 42, "user-1", "platform-session")
	require.NoError(t, err)
	require.False(t, shareRepo.getCalled)
	require.True(t, platformRepo.getCalled)
}

var _ interfaces.SessionRepository = (*messageLookupSessionRepoStub)(nil)
var _ interfaces.AgentPageShareSessionRepository = (*messageLookupShareSessionRepoStub)(nil)
var _ = event.EventBus{}
