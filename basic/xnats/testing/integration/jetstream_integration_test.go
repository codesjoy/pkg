//go:build integration

package integration

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xnats"
	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
)

func TestJetStreamPublishAndPullConsumeEndToEnd(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	nc := newConn(t)
	defer nc.Close()
	js := newJetStream(t, nc)

	stream := uniqueName("stream_pull")
	subject := uniqueName("subject.pull")
	consumerName := uniqueName("consumer_pull")
	createStream(t, js, stream, subject)
	_, err := js.CreateOrUpdateConsumer(ctx, stream, jetstream.ConsumerConfig{
		Name:          consumerName,
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	require.NoError(t, err)

	publisher, err := xnats.NewJetStreamPublisher(xnats.JetStreamPublisherConfig{
		Conn:           nc,
		JetStream:      js,
		DefaultSubject: subject,
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, publisher.Close())
	}()

	consumer, err := xnats.NewJetStreamConsumer(xnats.JetStreamConsumerConfig{
		Conn:          nc,
		JetStream:     js,
		Stream:        stream,
		Consumer:      consumerName,
		Mode:          xnats.JetStreamConsumerModePull,
		PullBatchSize: 1,
		PullMaxWait:   500 * time.Millisecond,
		IdleBackoff:   50 * time.Millisecond,
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, consumer.Close())
	}()

	receivedCh := make(chan string, 2)
	errCh := make(chan error, 1)
	var receivedCount atomic.Int32
	go func() {
		errCh <- consumer.Consume(ctx, func(_ context.Context, msg *consume.MessageContext) error {
			receivedCh <- string(msg.Message.Data)
			if receivedCount.Add(1) == 2 {
				cancel()
			}
			return nil
		})
	}()

	_, err = publisher.Publish(ctx, &publish.Message{Data: []byte("a")})
	require.NoError(t, err)
	_, err = publisher.Publish(ctx, &publish.Message{Data: []byte("b")})
	require.NoError(t, err)

	var got []string
	for len(got) < 2 {
		select {
		case value := <-receivedCh:
			got = append(got, value)
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for JetStream pull messages")
		}
	}

	require.ElementsMatch(t, []string{"a", "b"}, got)
	require.ErrorIs(t, <-errCh, context.Canceled)
}

func TestJetStreamPublishAndPushConsumeEndToEnd(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	nc := newConn(t)
	defer nc.Close()
	js := newJetStream(t, nc)
	legacyJS, err := nc.JetStream()
	require.NoError(t, err)

	stream := uniqueName("stream_push")
	subject := uniqueName("subject.push")
	consumerName := uniqueName("consumer_push")
	createStream(t, js, stream, subject)
	_, err = legacyJS.AddConsumer(stream, &nats.ConsumerConfig{
		Durable:        consumerName,
		DeliverSubject: nats.NewInbox(),
		AckPolicy:      nats.AckExplicitPolicy,
	})
	require.NoError(t, err)

	publisher, err := xnats.NewJetStreamPublisher(xnats.JetStreamPublisherConfig{
		Conn:           nc,
		JetStream:      js,
		DefaultSubject: subject,
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, publisher.Close())
	}()

	consumer, err := xnats.NewJetStreamConsumer(xnats.JetStreamConsumerConfig{
		Conn:        nc,
		JetStream:   js,
		Stream:      stream,
		Consumer:    consumerName,
		Mode:        xnats.JetStreamConsumerModePush,
		PullMaxWait: 500 * time.Millisecond,
		IdleBackoff: 50 * time.Millisecond,
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, consumer.Close())
	}()

	receivedCh := make(chan string, 2)
	errCh := make(chan error, 1)
	var receivedCount atomic.Int32
	go func() {
		errCh <- consumer.Consume(ctx, func(_ context.Context, msg *consume.MessageContext) error {
			receivedCh <- string(msg.Message.Data)
			if receivedCount.Add(1) == 2 {
				cancel()
			}
			return nil
		})
	}()
	info := waitForPushBoundOrConsumeError(t, ctx, legacyJS, stream, consumerName, errCh)
	require.True(t, info.PushBound)

	publishCtx, publishCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer publishCancel()

	_, err = publisher.Publish(publishCtx, &publish.Message{Data: []byte("x")})
	require.NoError(t, err)
	_, err = publisher.Publish(publishCtx, &publish.Message{Data: []byte("y")})
	require.NoError(t, err)

	var got []string
	for len(got) < 2 {
		select {
		case value := <-receivedCh:
			got = append(got, value)
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for JetStream push messages")
		}
	}

	require.ElementsMatch(t, []string{"x", "y"}, got)
	require.ErrorIs(t, <-errCh, context.Canceled)
}

func TestJetStreamDropPolicyAcksMessage(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	nc := newConn(t)
	defer nc.Close()
	js := newJetStream(t, nc)

	stream := uniqueName("stream_drop")
	subject := uniqueName("subject.drop")
	consumerName := uniqueName("consumer_drop")
	createStream(t, js, stream, subject)
	consumerRef, err := js.CreateOrUpdateConsumer(ctx, stream, jetstream.ConsumerConfig{
		Name:          consumerName,
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	require.NoError(t, err)

	publisher, err := xnats.NewJetStreamPublisher(xnats.JetStreamPublisherConfig{
		Conn:           nc,
		JetStream:      js,
		DefaultSubject: subject,
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, publisher.Close())
	}()

	consumer, err := xnats.NewJetStreamConsumer(xnats.JetStreamConsumerConfig{
		Conn:          nc,
		JetStream:     js,
		Stream:        stream,
		Consumer:      consumerName,
		Mode:          xnats.JetStreamConsumerModePull,
		PullBatchSize: 1,
		PullMaxWait:   500 * time.Millisecond,
		IdleBackoff:   50 * time.Millisecond,
		RetryConfig: xnats.RetryConfig{
			MaxRetries:     0,
			InitialBackoff: 10 * time.Millisecond,
			MaxBackoff:     10 * time.Millisecond,
			Multiplier:     1,
		},
		ExhaustedPolicy: xnats.ConsumeExhaustedPolicyDrop,
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, consumer.Close())
	}()

	_, err = publisher.Publish(ctx, &publish.Message{Data: []byte("drop")})
	require.NoError(t, err)

	invoked := make(chan struct{}, 1)
	errCh := make(chan error, 1)
	consumeCtx, consumeCancel := context.WithCancel(ctx)
	go func() {
		errCh <- consumer.Consume(consumeCtx, func(_ context.Context, _ *consume.MessageContext) error {
			select {
			case invoked <- struct{}{}:
			default:
			}
			return errors.New("boom")
		})
	}()

	select {
	case <-invoked:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for drop handler invocation")
	}
	consumeCancel()
	require.ErrorIs(t, <-errCh, context.Canceled)

	batch, err := consumerRef.Fetch(1, jetstream.FetchMaxWait(250*time.Millisecond))
	require.NoError(t, err)
	found := false
	for range batch.Messages() {
		found = true
	}
	require.NoError(t, batch.Error())
	require.False(t, found)
}

func TestJetStreamStopPolicyNaksMessage(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	nc := newConn(t)
	defer nc.Close()
	js := newJetStream(t, nc)

	stream := uniqueName("stream_stop")
	subject := uniqueName("subject.stop")
	consumerName := uniqueName("consumer_stop")
	createStream(t, js, stream, subject)
	consumerRef, err := js.CreateOrUpdateConsumer(ctx, stream, jetstream.ConsumerConfig{
		Name:          consumerName,
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	require.NoError(t, err)

	publisher, err := xnats.NewJetStreamPublisher(xnats.JetStreamPublisherConfig{
		Conn:           nc,
		JetStream:      js,
		DefaultSubject: subject,
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, publisher.Close())
	}()

	consumer, err := xnats.NewJetStreamConsumer(xnats.JetStreamConsumerConfig{
		Conn:          nc,
		JetStream:     js,
		Stream:        stream,
		Consumer:      consumerName,
		Mode:          xnats.JetStreamConsumerModePull,
		PullBatchSize: 1,
		PullMaxWait:   500 * time.Millisecond,
		IdleBackoff:   50 * time.Millisecond,
		RetryConfig: xnats.RetryConfig{
			MaxRetries:     0,
			InitialBackoff: 10 * time.Millisecond,
			MaxBackoff:     10 * time.Millisecond,
			Multiplier:     1,
		},
		ExhaustedPolicy: xnats.ConsumeExhaustedPolicyStop,
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, consumer.Close())
	}()

	_, err = publisher.Publish(ctx, &publish.Message{Data: []byte("stop")})
	require.NoError(t, err)

	err = consumer.Consume(ctx, func(_ context.Context, _ *consume.MessageContext) error {
		return errors.New("boom")
	})
	require.Error(t, err)

	batch, err := consumerRef.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
	require.NoError(t, err)
	found := false
	for msg := range batch.Messages() {
		found = true
		require.Equal(t, "stop", string(msg.Data()))
		require.NoError(t, msg.Ack())
	}
	require.NoError(t, batch.Error())
	require.True(t, found)
}
