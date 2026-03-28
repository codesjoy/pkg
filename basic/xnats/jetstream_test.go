// Copyright 2022 The codesjoy Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package xnats

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	jetstreamrt "github.com/codesjoy/pkg/basic/xnats/internal/runtime/jetstream"
	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
)

func TestNewJetStreamPublisherAndPublish(t *testing.T) {
	srv := newTestServerWithArgs(t, "-js")
	nc := newTestConn(t, srv)
	js, err := jetstream.New(nc)
	require.NoError(t, err)

	_, err = js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
		Name:     "ORDERS",
		Subjects: []string{"orders.created"},
	})
	require.NoError(t, err)

	publisher, err := NewJetStreamPublisher(JetStreamPublisherConfig{
		URLs:           []string{srv.ClientURL()},
		DefaultSubject: "orders.created",
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, publisher.Close())
	}()

	result, err := publisher.Publish(context.Background(), &publish.Message{Data: []byte("one")})
	require.NoError(t, err)
	require.Equal(t, "ORDERS", result.Stream)
	require.Greater(t, result.Sequence, uint64(0))

	results, err := publisher.PublishBatch(
		context.Background(),
		&publish.Message{Data: []byte("two")},
		&publish.Message{Data: []byte("three")},
	)
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, result.Sequence+1, results[0].Sequence)
	require.Equal(t, result.Sequence+2, results[1].Sequence)
}

func TestJetStreamPublisherErrorPaths(t *testing.T) {
	var nilPublisher *JetStreamPublisher
	_, err := nilPublisher.Publish(context.Background(), &publish.Message{})
	require.EqualError(t, err, "jetstream publisher is nil")
	require.NoError(t, nilPublisher.Close())

	publisher := &JetStreamPublisher{}
	_, err = publisher.Publish(context.Background(), &publish.Message{Subject: "orders"})
	require.ErrorIs(t, err, ErrJetStreamPublisherClosed)

	_, err = publisher.send(context.Background(), nil)
	require.ErrorIs(t, err, ErrNilPublishMessage)

	_, err = publisher.send(context.Background(), &publish.MessageContext{})
	require.ErrorIs(t, err, ErrNilPublishMessage)
}

func TestJetStreamPublisherPrepareMessageAndClose(t *testing.T) {
	srv := newTestServerWithArgs(t, "-js")
	nc := newTestConn(t, srv)
	js, err := jetstream.New(nc)
	require.NoError(t, err)

	publisher := &JetStreamPublisher{
		cfg: JetStreamPublisherConfig{DefaultSubject: "orders.created"},
		js:  js,
	}

	prepared, err := publisher.prepareMessage(&publish.Message{Data: []byte("payload")})
	require.NoError(t, err)
	require.Equal(t, "orders.created", prepared.Subject)

	_, err = publisher.prepareMessage(nil)
	require.ErrorIs(t, err, ErrNilPublishMessage)

	require.NoError(t, publisher.Close())
	require.NoError(t, publisher.Close())
}

func TestJetStreamPublisherBuildPublishChainModes(t *testing.T) {
	t.Parallel()

	cfg := JetStreamPublisherConfig{
		SubjectHandlers: make(map[string]PublishSubjectHandlers),
	}
	publisherInstance := &JetStreamPublisher{cfg: cfg}

	var appendOrder []string
	publisherInstance.cfg.GlobalHandlers = []publish.Handler{
		newPublishRecorder("global", &appendOrder),
	}
	publisherInstance.cfg.SubjectHandlers["append"] = PublishSubjectHandlers{
		Mode:     ChainModeAppend,
		Handlers: []publish.Handler{newPublishRecorder("subject", &appendOrder)},
	}

	_, err := publisherInstance.buildPublishChain("append", func(
		_ context.Context,
		_ *publish.MessageContext,
	) (*publish.Result, error) {
		appendOrder = append(appendOrder, "business")
		return &publish.Result{}, nil
	})(context.Background(), &publish.MessageContext{})
	require.NoError(t, err)
	require.Equal(t, []string{"global", "subject", "business"}, appendOrder)
}

func TestJetStreamConsumerPrepareOrderedMessage(t *testing.T) {
	t.Parallel()

	consumer := &JetStreamConsumer{
		cfg: JetStreamConsumerConfig{
			ShardCount: 4,
			KeyExtractor: func(*consume.MessageContext) (string, error) {
				return "order-1", nil
			},
		},
	}

	msgCtx := &consume.MessageContext{}
	require.NoError(t, consumer.prepareOrderedMessage(msgCtx))
	require.Equal(t, "order-1", msgCtx.LogicalKey)
	require.Equal(t, jetstreamrt.ShardForKey("order-1", 4), msgCtx.Shard)
}

func TestJetStreamConsumerPrepareOrderedMessageKeyExtractorError(t *testing.T) {
	t.Parallel()

	consumer := &JetStreamConsumer{
		cfg: JetStreamConsumerConfig{
			ShardCount: 2,
			KeyExtractor: func(*consume.MessageContext) (string, error) {
				return "", errors.New("boom")
			},
		},
	}

	err := consumer.prepareOrderedMessage(&consume.MessageContext{})
	require.EqualError(t, err, "extract logical key: boom")
}

func TestJetStreamConsumerValidateOrderedConsumeAckPolicy(t *testing.T) {
	t.Parallel()

	consumer := &JetStreamConsumer{cfg: JetStreamConsumerConfig{ShardCount: 2}}
	err := consumer.validateOrderedConsumeAckPolicy(consumeAckPolicyAll)
	require.EqualError(t, err, `ordered consume does not support jetstream ack policy "AckAll"`)
}

func TestAutoAckMessage(t *testing.T) {
	t.Parallel()

	t.Run("explicit acks unhandled messages", func(t *testing.T) {
		t.Parallel()

		acker := &stubAcknowledger{}
		msgCtx := &consume.MessageContext{Acker: acker}

		require.NoError(t, autoAckMessage(msgCtx, consumeAckPolicyExplicit))
		require.Equal(t, 1, acker.acks)
	})

	t.Run("none skips automatic ack", func(t *testing.T) {
		t.Parallel()

		acker := &stubAcknowledger{}
		msgCtx := &consume.MessageContext{Acker: acker}

		require.NoError(t, autoAckMessage(msgCtx, consumeAckPolicyNone))
		require.Zero(t, acker.acks)
	})

	t.Run("handled message is not acked twice", func(t *testing.T) {
		t.Parallel()

		acker := &stubAcknowledger{handled: true}
		msgCtx := &consume.MessageContext{Acker: acker}

		require.NoError(t, autoAckMessage(msgCtx, consumeAckPolicyExplicit))
		require.Zero(t, acker.acks)
	})
}

func TestWrapAcknowledgerAckNoneNoops(t *testing.T) {
	t.Parallel()

	inner := &stubAcknowledger{}
	wrapped := wrapAcknowledger(inner, consumeAckPolicyNone)
	require.NoError(t, wrapped.Ack())
	require.NoError(t, wrapped.Nak())
	require.True(t, wrapped.Handled())
	require.Zero(t, inner.acks)
	require.Zero(t, inner.naks)
}

func TestJetStreamConsumerHandleConsumedMessageStopPolicyNaks(t *testing.T) {
	t.Parallel()

	enabled := false
	consumer := &JetStreamConsumer{
		cfg: JetStreamConsumerConfig{
			LoggerHandlerEnabled: &enabled,
			RetryConfig: RetryConfig{
				MaxRetries:     0,
				InitialBackoff: time.Millisecond,
				MaxBackoff:     time.Millisecond,
				Multiplier:     1,
			},
			ExhaustedPolicy: ConsumeExhaustedPolicyStop,
		},
	}

	inner := &stubAcknowledger{}
	msgCtx := &consume.MessageContext{
		Subject:   "orders.created",
		Transport: consume.TransportJetStream,
		Acker:     wrapAcknowledger(inner, consumeAckPolicyExplicit),
	}

	err := consumer.handleConsumedMessage(
		context.Background(),
		func(context.Context, *consume.MessageContext) error { return errors.New("boom") },
		msgCtx,
		consumeAckPolicyExplicit,
	)
	require.Error(t, err)
	require.Zero(t, inner.acks)
	require.Equal(t, 1, inner.naks)
}

func TestNewJetStreamConsumerAndConsumePull(t *testing.T) {
	srv := newTestServerWithArgs(t, "-js")
	nc := newTestConn(t, srv)
	js, err := jetstream.New(nc)
	require.NoError(t, err)

	_, err = js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
		Name:     "ORDERS",
		Subjects: []string{"orders.pull"},
	})
	require.NoError(t, err)
	_, err = js.CreateOrUpdateConsumer(context.Background(), "ORDERS", jetstream.ConsumerConfig{
		Name:          "puller",
		FilterSubject: "orders.pull",
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	require.NoError(t, err)

	_, err = js.Publish(context.Background(), "orders.pull", []byte("payload"))
	require.NoError(t, err)

	consumer, err := NewJetStreamConsumer(JetStreamConsumerConfig{
		URLs:          []string{srv.ClientURL()},
		Stream:        "ORDERS",
		Consumer:      "puller",
		PullMaxWait:   time.Second,
		IdleBackoff:   10 * time.Millisecond,
		PullBatchSize: 1,
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, consumer.Close())
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	received := make(chan *consume.MessageContext, 1)
	go func() {
		errCh <- consumer.Consume(ctx, func(_ context.Context, msg *consume.MessageContext) error {
			received <- msg
			cancel()
			return nil
		})
	}()

	msg := <-received
	require.Equal(t, "orders.pull", msg.Subject)
	require.Equal(t, "payload", string(msg.Message.Data))
	require.NotNil(t, msg.JetStream)
	require.ErrorIs(t, <-errCh, context.Canceled)
}

func TestNewJetStreamConsumerAndConsumePush(t *testing.T) {
	srv := newTestServerWithArgs(t, "-js")
	nc := newTestConn(t, srv)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	legacyJS, err := nc.JetStream()
	require.NoError(t, err)

	_, err = js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
		Name:     "ORDERS",
		Subjects: []string{"orders.push"},
	})
	require.NoError(t, err)
	_, err = legacyJS.AddConsumer("ORDERS", &nats.ConsumerConfig{
		Durable:        "pusher",
		DeliverSubject: nats.NewInbox(),
		FilterSubject:  "orders.push",
		AckPolicy:      nats.AckExplicitPolicy,
	})
	require.NoError(t, err)

	consumer, err := NewJetStreamConsumer(JetStreamConsumerConfig{
		Conn:        nc,
		JetStream:   js,
		Stream:      "ORDERS",
		Consumer:    "pusher",
		Mode:        JetStreamConsumerModePush,
		PullMaxWait: 200 * time.Millisecond,
		IdleBackoff: 10 * time.Millisecond,
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, consumer.Close())
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	received := make(chan *consume.MessageContext, 1)
	go func() {
		errCh <- consumer.Consume(ctx, func(_ context.Context, msg *consume.MessageContext) error {
			received <- msg
			cancel()
			return nil
		})
	}()

	require.Eventually(t, func() bool {
		info, infoErr := legacyJS.ConsumerInfo("ORDERS", "pusher")
		return infoErr == nil && info != nil && info.PushBound
	}, 5*time.Second, 50*time.Millisecond)

	_, err = js.Publish(context.Background(), "orders.push", []byte("payload"))
	require.NoError(t, err)

	msg := <-received
	require.Equal(t, "orders.push", msg.Subject)
	require.Equal(t, "payload", string(msg.Message.Data))
	require.ErrorIs(t, <-errCh, context.Canceled)
}

func TestJetStreamConsumerHelpers(t *testing.T) {
	t.Parallel()

	var nilConsumer *JetStreamConsumer
	err := nilConsumer.Consume(
		context.Background(),
		func(context.Context, *consume.MessageContext) error { return nil },
	)
	require.EqualError(t, err, "jetstream consumer is nil")
	require.NoError(t, nilConsumer.Close())

	consumer := &JetStreamConsumer{
		cfg:      JetStreamConsumerConfig{ShardCount: 2},
		closedCh: make(chan struct{}),
	}
	err = consumer.validateOrderedConsumeAckPolicy(consumeAckPolicyAll)
	require.EqualError(t, err, `ordered consume does not support jetstream ack policy "AckAll"`)

	ctx := consumeContextFromRawMessage(
		nil,
		consume.TransportJetStream,
		nil,
		consumeAckPolicyExplicit,
	)
	require.Equal(t, consume.TransportJetStream, ctx.Transport)
	require.NotNil(t, consumeContextFromJetStreamMessage(nil, consumeAckPolicyExplicit))

	require.Equal(t, consumeAckPolicyExplicit, ackPolicyFromJetStreamInfo(nil))
	require.Equal(t, consumeAckPolicyExplicit, ackPolicyFromLegacyInfo(nil))

	raw := &rawAcker{}
	require.NoError(t, raw.Ack())
	require.NoError(t, raw.Nak())
	require.False(t, raw.Handled())

	jsAcker := &jetStreamAcker{}
	require.NoError(t, jsAcker.Ack())
	require.NoError(t, jsAcker.Nak())
	require.False(t, jsAcker.Handled())

	require.Equal(t, "AckExplicit", consumeAckPolicyExplicit.String())
	require.Equal(t, "AckAll", consumeAckPolicyAll.String())
	require.Equal(t, "AckNone", consumeAckPolicyNone.String())
}

func TestNewJetStreamConsumerAndConsumePullOrdered(t *testing.T) {
	srv := newTestServerWithArgs(t, "-js")
	nc := newTestConn(t, srv)
	js, err := jetstream.New(nc)
	require.NoError(t, err)

	_, err = js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
		Name:     "ORDERS_ORDERED_PULL",
		Subjects: []string{"orders.ordered.pull"},
	})
	require.NoError(t, err)
	_, err = js.CreateOrUpdateConsumer(
		context.Background(),
		"ORDERS_ORDERED_PULL",
		jetstream.ConsumerConfig{
			Name:          "puller-ordered",
			FilterSubject: "orders.ordered.pull",
			AckPolicy:     jetstream.AckExplicitPolicy,
		},
	)
	require.NoError(t, err)

	raw := nats.NewMsg("orders.ordered.pull")
	raw.Header.Set("X-Order-Key", "order-1")
	raw.Data = []byte("payload")
	_, err = js.PublishMsg(context.Background(), raw)
	require.NoError(t, err)

	consumer, err := NewJetStreamConsumer(JetStreamConsumerConfig{
		URLs:           []string{srv.ClientURL()},
		Stream:         "ORDERS_ORDERED_PULL",
		Consumer:       "puller-ordered",
		ShardCount:     2,
		ShardQueueSize: 8,
		KeyExtractor: func(msg *consume.MessageContext) (string, error) {
			return msg.Message.Header.Get("X-Order-Key"), nil
		},
		PullMaxWait:   time.Second,
		IdleBackoff:   10 * time.Millisecond,
		PullBatchSize: 1,
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, consumer.Close())
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	received := make(chan *consume.MessageContext, 1)
	go func() {
		errCh <- consumer.Consume(ctx, func(_ context.Context, msg *consume.MessageContext) error {
			received <- msg
			cancel()
			return nil
		})
	}()

	msg := <-received
	require.Equal(t, "order-1", msg.LogicalKey)
	require.GreaterOrEqual(t, msg.Shard, 0)
	require.ErrorIs(t, <-errCh, context.Canceled)
}

func TestNewJetStreamConsumerAndConsumePushOrdered(t *testing.T) {
	srv := newTestServerWithArgs(t, "-js")
	nc := newTestConn(t, srv)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	legacyJS, err := nc.JetStream()
	require.NoError(t, err)

	_, err = js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
		Name:     "ORDERS_ORDERED_PUSH",
		Subjects: []string{"orders.ordered.push"},
	})
	require.NoError(t, err)
	_, err = legacyJS.AddConsumer("ORDERS_ORDERED_PUSH", &nats.ConsumerConfig{
		Durable:        "pusher-ordered",
		DeliverSubject: nats.NewInbox(),
		FilterSubject:  "orders.ordered.push",
		AckPolicy:      nats.AckExplicitPolicy,
	})
	require.NoError(t, err)

	consumer, err := NewJetStreamConsumer(JetStreamConsumerConfig{
		Conn:           nc,
		JetStream:      js,
		Stream:         "ORDERS_ORDERED_PUSH",
		Consumer:       "pusher-ordered",
		Mode:           JetStreamConsumerModePush,
		ShardCount:     2,
		ShardQueueSize: 8,
		KeyExtractor: func(msg *consume.MessageContext) (string, error) {
			return msg.Message.Header.Get("X-Order-Key"), nil
		},
		PullMaxWait: 200 * time.Millisecond,
		IdleBackoff: 10 * time.Millisecond,
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, consumer.Close())
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	received := make(chan *consume.MessageContext, 1)
	go func() {
		errCh <- consumer.Consume(ctx, func(_ context.Context, msg *consume.MessageContext) error {
			received <- msg
			cancel()
			return nil
		})
	}()

	require.Eventually(t, func() bool {
		info, infoErr := legacyJS.ConsumerInfo("ORDERS_ORDERED_PUSH", "pusher-ordered")
		return infoErr == nil && info != nil && info.PushBound
	}, 5*time.Second, 50*time.Millisecond)

	raw := nats.NewMsg("orders.ordered.push")
	raw.Header.Set("X-Order-Key", "order-1")
	raw.Data = []byte("payload")
	_, err = js.PublishMsg(context.Background(), raw)
	require.NoError(t, err)

	msg := <-received
	require.Equal(t, "order-1", msg.LogicalKey)
	require.GreaterOrEqual(t, msg.Shard, 0)
	require.ErrorIs(t, <-errCh, context.Canceled)
}

type stubAcknowledger struct {
	acks    int
	naks    int
	handled bool
}

func (a *stubAcknowledger) Ack() error {
	a.acks++
	a.handled = true
	return nil
}

func (a *stubAcknowledger) Nak() error {
	a.naks++
	a.handled = true
	return nil
}

func (a *stubAcknowledger) Handled() bool {
	return a.handled
}
