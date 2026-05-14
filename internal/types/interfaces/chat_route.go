package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

type ChatRouteService interface {
	Decide(ctx context.Context, input types.ChatRouteInput) (*types.ChatRouteDecision, error)
}
