//go:build integration

package integration

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xnats"
	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
)

func TestPublisherAndSubscriberEndToEnd(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	subject := uniqueName("core")

	subscriber, err := xnats.NewSubscriber(xnats.SubscriberConfig{
		URLs:     []string{mustURL(t)},
		Subjects: []string{subject},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, subscriber.Close())
	})

	receivedCh := make(chan string, 2)
	errCh := make(chan error, 1)
	var receivedCount atomic.Int32
	go func() {
		errCh <- subscriber.Consume(ctx, func(_ context.Context, msg *consume.MessageContext) error {
			receivedCh <- string(msg.Message.Data)
			if receivedCount.Add(1) == 2 {
				cancel()
			}
			return nil
		})
	}()

	publisher, err := xnats.NewPublisher(xnats.PublisherConfig{
		URLs:           []string{mustURL(t)},
		DefaultSubject: subject,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, publisher.Close())
	})

	publishCtx, publishCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer publishCancel()

	_, err = publisher.Publish(publishCtx, &publish.Message{Data: []byte("one")})
	require.NoError(t, err)
	_, err = publisher.Publish(publishCtx, &publish.Message{Data: []byte("two")})
	require.NoError(t, err)

	var got []string
	for len(got) < 2 {
		select {
		case value := <-receivedCh:
			got = append(got, value)
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for subscriber messages")
		}
	}

	require.ElementsMatch(t, []string{"one", "two"}, got)
	require.ErrorIs(t, <-errCh, context.Canceled)
}

func TestRequesterEndToEnd(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	nc := newConn(t)
	defer nc.Close()

	subject := uniqueName("request")
	sub, err := nc.Subscribe(subject, func(msg *nats.Msg) {
		_ = msg.Respond([]byte("pong"))
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, sub.Drain())
	}()

	requester, err := xnats.NewRequester(xnats.RequesterConfig{
		URLs: []string{mustURL(t)},
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, requester.Close())
	}()

	resp, err := requester.Request(ctx, &publish.Message{
		Subject: subject,
		Data:    []byte("ping"),
	})
	require.NoError(t, err)
	require.Equal(t, "pong", string(resp.Data))
}
