package retry

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
)

func TestDropPolicyAcksJetStreamMessage(t *testing.T) {
	acker := &fakeAcker{}
	middleware := New(Config{MaxRetries: 0}, ExhaustedPolicyDrop, nil, nil)

	err := middleware.Handle(context.Background(), &consume.MessageContext{
		Transport: consume.TransportJetStream,
		Acker:     acker,
	}, func(context.Context, *consume.MessageContext) error {
		return errors.New("boom")
	})

	require.NoError(t, err)
	require.True(t, acker.acked)
	require.False(t, acker.nacked)
}

func TestStopPolicyNaksJetStreamMessage(t *testing.T) {
	acker := &fakeAcker{}
	middleware := New(Config{MaxRetries: 0}, ExhaustedPolicyStop, nil, nil)

	err := middleware.Handle(context.Background(), &consume.MessageContext{
		Transport: consume.TransportJetStream,
		Acker:     acker,
	}, func(context.Context, *consume.MessageContext) error {
		return errors.New("boom")
	})

	require.Error(t, err)
	require.True(t, acker.nacked)
	require.False(t, acker.acked)
}

type fakeAcker struct {
	acked  bool
	nacked bool
}

func (a *fakeAcker) Ack() error {
	a.acked = true
	return nil
}

func (a *fakeAcker) Nak() error {
	a.nacked = true
	return nil
}

func (a *fakeAcker) Handled() bool {
	return a.acked || a.nacked
}
