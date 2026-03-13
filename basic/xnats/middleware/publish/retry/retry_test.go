package retry

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
)

func TestDropPolicyReturnsDroppedError(t *testing.T) {
	middleware := New(Config{MaxRetries: 0}, ExhaustedPolicyDrop, nil, nil)

	_, err := middleware.Handle(context.Background(), &publish.MessageContext{
		Message: &publish.Message{Subject: "orders"},
	}, func(context.Context, *publish.MessageContext) (*publish.Result, error) {
		return nil, errors.New("boom")
	})

	require.Error(t, err)
	require.ErrorIs(t, err, ErrMessageDropped)
}
