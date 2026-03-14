//go:build integration

package integration

import (
	"context"
	"errors"
	"hash/fnv"
	"strconv"
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

const orderedKeyHeader = "x-order-key"

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

func TestJetStreamPullConsumeOrderedByKey(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	nc := newConn(t)
	defer nc.Close()
	js := newJetStream(t, nc)

	stream := uniqueName("stream_pull_ordered")
	subject := uniqueName("subject.pull.ordered")
	consumerName := uniqueName("consumer_pull_ordered")
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
		Conn:           nc,
		JetStream:      js,
		Stream:         stream,
		Consumer:       consumerName,
		Mode:           xnats.JetStreamConsumerModePull,
		PullBatchSize:  8,
		PullMaxWait:    500 * time.Millisecond,
		IdleBackoff:    50 * time.Millisecond,
		ShardCount:     2,
		ShardQueueSize: 8,
		KeyExtractor: func(msg *consume.MessageContext) (string, error) {
			return msg.Message.Header.Get(orderedKeyHeader), nil
		},
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, consumer.Close())
	}()

	keyOne, keyTwo := distinctOrderedKeys(2)
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	otherStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var firstDone atomic.Bool
	var parallelObserved atomic.Bool
	var secondStartedEarly atomic.Bool
	var handled atomic.Int32

	errCh := make(chan error, 1)
	go func() {
		errCh <- consumer.Consume(ctx, func(_ context.Context, msg *consume.MessageContext) error {
			switch string(msg.Message.Data) {
			case "a1":
				close(firstStarted)
				<-releaseFirst
				firstDone.Store(true)
			case "a2":
				if !firstDone.Load() {
					secondStartedEarly.Store(true)
				}
				close(secondStarted)
			case "b1":
				if !firstDone.Load() {
					parallelObserved.Store(true)
				}
				close(otherStarted)
			}

			if handled.Add(1) == 3 {
				cancel()
			}
			return nil
		})
	}()

	_, err = publisher.Publish(ctx, &publish.Message{
		Data:   []byte("a1"),
		Header: nats.Header{orderedKeyHeader: []string{keyOne}},
	})
	require.NoError(t, err)
	_, err = publisher.Publish(ctx, &publish.Message{
		Data:   []byte("b1"),
		Header: nats.Header{orderedKeyHeader: []string{keyTwo}},
	})
	require.NoError(t, err)
	_, err = publisher.Publish(ctx, &publish.Message{
		Data:   []byte("a2"),
		Header: nats.Header{orderedKeyHeader: []string{keyOne}},
	})
	require.NoError(t, err)

	select {
	case <-firstStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for first ordered message")
	}

	select {
	case <-otherStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for cross-key parallel message")
	}

	close(releaseFirst)

	select {
	case <-secondStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for second same-key message")
	}

	require.ErrorIs(t, <-errCh, context.Canceled)
	require.True(t, parallelObserved.Load())
	require.False(t, secondStartedEarly.Load())
}

func TestJetStreamPushConsumeOrderedByKey(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	nc := newConn(t)
	defer nc.Close()
	js := newJetStream(t, nc)
	legacyJS, err := nc.JetStream()
	require.NoError(t, err)

	stream := uniqueName("stream_push_ordered")
	subject := uniqueName("subject.push.ordered")
	consumerName := uniqueName("consumer_push_ordered")
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
		Conn:           nc,
		JetStream:      js,
		Stream:         stream,
		Consumer:       consumerName,
		Mode:           xnats.JetStreamConsumerModePush,
		PullMaxWait:    500 * time.Millisecond,
		IdleBackoff:    50 * time.Millisecond,
		ShardCount:     2,
		ShardQueueSize: 8,
		KeyExtractor: func(msg *consume.MessageContext) (string, error) {
			return msg.Message.Header.Get(orderedKeyHeader), nil
		},
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, consumer.Close())
	}()

	keyOne, keyTwo := distinctOrderedKeys(2)
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	otherStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var firstDone atomic.Bool
	var parallelObserved atomic.Bool
	var secondStartedEarly atomic.Bool
	var handled atomic.Int32

	errCh := make(chan error, 1)
	go func() {
		errCh <- consumer.Consume(ctx, func(_ context.Context, msg *consume.MessageContext) error {
			switch string(msg.Message.Data) {
			case "a1":
				close(firstStarted)
				<-releaseFirst
				firstDone.Store(true)
			case "a2":
				if !firstDone.Load() {
					secondStartedEarly.Store(true)
				}
				close(secondStarted)
			case "b1":
				if !firstDone.Load() {
					parallelObserved.Store(true)
				}
				close(otherStarted)
			}

			if handled.Add(1) == 3 {
				cancel()
			}
			return nil
		})
	}()

	info := waitForPushBoundOrConsumeError(t, ctx, legacyJS, stream, consumerName, errCh)
	require.True(t, info.PushBound)

	_, err = publisher.Publish(ctx, &publish.Message{
		Data:   []byte("a1"),
		Header: nats.Header{orderedKeyHeader: []string{keyOne}},
	})
	require.NoError(t, err)
	_, err = publisher.Publish(ctx, &publish.Message{
		Data:   []byte("b1"),
		Header: nats.Header{orderedKeyHeader: []string{keyTwo}},
	})
	require.NoError(t, err)
	_, err = publisher.Publish(ctx, &publish.Message{
		Data:   []byte("a2"),
		Header: nats.Header{orderedKeyHeader: []string{keyOne}},
	})
	require.NoError(t, err)

	select {
	case <-firstStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for first ordered push message")
	}

	select {
	case <-otherStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for cross-key parallel push message")
	}

	close(releaseFirst)

	select {
	case <-secondStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for second same-key push message")
	}

	require.ErrorIs(t, <-errCh, context.Canceled)
	require.True(t, parallelObserved.Load())
	require.False(t, secondStartedEarly.Load())
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
		AckWait:       500 * time.Millisecond,
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

func TestJetStreamDropPolicyAcksMessageOrdered(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	nc := newConn(t)
	defer nc.Close()
	js := newJetStream(t, nc)

	stream := uniqueName("stream_drop_ordered")
	subject := uniqueName("subject.drop.ordered")
	consumerName := uniqueName("consumer_drop_ordered")
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
		Conn:           nc,
		JetStream:      js,
		Stream:         stream,
		Consumer:       consumerName,
		Mode:           xnats.JetStreamConsumerModePull,
		PullBatchSize:  4,
		PullMaxWait:    500 * time.Millisecond,
		IdleBackoff:    50 * time.Millisecond,
		ShardCount:     2,
		ShardQueueSize: 8,
		KeyExtractor: func(msg *consume.MessageContext) (string, error) {
			return msg.Message.Header.Get(orderedKeyHeader), nil
		},
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

	_, err = publisher.Publish(ctx, &publish.Message{
		Data:   []byte("drop"),
		Header: nats.Header{orderedKeyHeader: []string{"order-drop"}},
	})
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
		t.Fatalf("timed out waiting for ordered drop handler invocation")
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

	require.Eventually(t, func() bool {
		batch, fetchErr := consumerRef.Fetch(1, jetstream.FetchMaxWait(500*time.Millisecond))
		require.NoError(t, fetchErr)

		found := false
		for msg := range batch.Messages() {
			found = true
			require.Equal(t, "stop", string(msg.Data()))
			require.NoError(t, msg.Ack())
		}
		require.NoError(t, batch.Error())
		return found
	}, 10*time.Second, 250*time.Millisecond)
}

func distinctOrderedKeys(shardCount int) (string, string) {
	first := "order-1"
	firstShard := orderedShardForKey(first, shardCount)
	for i := 2; ; i++ {
		candidate := "order-" + strconv.Itoa(i)
		if orderedShardForKey(candidate, shardCount) != firstShard {
			return first, candidate
		}
	}
}

func orderedShardForKey(key string, shardCount int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32() % uint32(shardCount))
}
