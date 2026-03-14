package jetstream

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
)

func TestShardForKeyAndErrorOr(t *testing.T) {
	t.Parallel()

	require.Equal(t, ShardForKey("order-1", 4), ShardForKey("order-1", 4))

	fallback := errors.New("fallback")
	require.ErrorIs(t, ErrorOr(nil, fallback), fallback)

	rt := New(context.Background(), 1, 1, func(context.Context, *consume.MessageContext) error {
		return errors.New("boom")
	})
	defer rt.Shutdown()
	require.NoError(t, rt.Enqueue(&consume.MessageContext{Shard: 0}))
	require.Eventually(t, func() bool {
		return rt.FatalErr() != nil
	}, time.Second, 10*time.Millisecond)
	require.EqualError(t, ErrorOr(rt, fallback), "boom")
}
