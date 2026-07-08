package handler

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestCanMutateAgent(t *testing.T) {
	ctxAdmin := context.WithValue(context.Background(), types.TenantRoleContextKey, types.TenantRoleAdmin)
	ctxContributor := context.WithValue(context.Background(), types.TenantRoleContextKey, types.TenantRoleContributor)

	t.Run("builtin agent allows tenant admin", func(t *testing.T) {
		user := &types.User{ID: "u-admin"}
		agent := &types.CustomAgent{ID: types.BuiltinQuickAnswerID}
		if !canMutateAgent(ctxAdmin, user, agent) {
			t.Fatal("tenant admin should be allowed to mutate built-in agent config")
		}
	})

	t.Run("builtin agent rejects contributor", func(t *testing.T) {
		user := &types.User{ID: "u-contrib"}
		agent := &types.CustomAgent{ID: types.BuiltinQuickAnswerID}
		if canMutateAgent(ctxContributor, user, agent) {
			t.Fatal("contributor must not mutate built-in agent config")
		}
	})

	t.Run("custom agent allows creator", func(t *testing.T) {
		user := &types.User{ID: "u-creator"}
		agent := &types.CustomAgent{ID: "agent-1", CreatedBy: "u-creator"}
		if !canMutateAgent(ctxContributor, user, agent) {
			t.Fatal("creator should be allowed to mutate own custom agent")
		}
	})

	t.Run("custom agent allows tenant admin", func(t *testing.T) {
		user := &types.User{ID: "u-admin"}
		agent := &types.CustomAgent{ID: "agent-1", CreatedBy: "u-other"}
		if !canMutateAgent(ctxAdmin, user, agent) {
			t.Fatal("tenant admin should be allowed to mutate another user's custom agent")
		}
	})
}