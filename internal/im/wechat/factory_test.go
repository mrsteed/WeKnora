package wechat

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrTokenExpiredRemainsClassifiableThroughWrapping(t *testing.T) {
	require.ErrorIs(t, ErrTokenExpired, ErrTokenExpired)
	require.ErrorIs(t, fmt.Errorf("poll stopped: %w", ErrTokenExpired), ErrTokenExpired)
	require.NotErrorIs(t, fmt.Errorf("poll stopped: %w", fmt.Errorf("network timeout")), ErrTokenExpired)
}