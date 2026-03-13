//go:build integration

package integration

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/codesjoy/pkg/basic/xnats"
	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
	ctrace "github.com/codesjoy/pkg/basic/xnats/middleware/consume/trace"
	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
	ptrace "github.com/codesjoy/pkg/basic/xnats/middleware/publish/trace"
)

func TestCoreTracePropagationEndToEnd(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	provider := sdktrace.NewTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()
	tracer := provider.Tracer("integration")
	parentCtx, parent := tracer.Start(context.Background(), "parent")
	parentTraceID := parent.SpanContext().TraceID()

	subject := uniqueName("core_trace")
	subscriber, err := xnats.NewSubscriber(xnats.SubscriberConfig{
		URLs:     []string{mustURL(t)},
		Subjects: []string{subject},
		GlobalHandlers: []consume.Handler{
			ctrace.New(ctrace.Config{Tracer: tracer, Propagator: propagation.TraceContext{}}),
		},
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, subscriber.Close())
	}()

	seenCh := make(chan struct{}, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- subscriber.Consume(ctx, func(handlerCtx context.Context, msg *consume.MessageContext) error {
			require.Equal(t, parentTraceID, oteltrace.SpanContextFromContext(handlerCtx).TraceID())
			require.NotEmpty(t, msg.Message.Header.Get("traceparent"))
			select {
			case seenCh <- struct{}{}:
			default:
			}
			cancel()
			return nil
		})
	}()

	publisher, err := xnats.NewPublisher(xnats.PublisherConfig{
		URLs:           []string{mustURL(t)},
		DefaultSubject: subject,
		GlobalHandlers: []publish.Handler{
			ptrace.New(ptrace.Config{Tracer: tracer, Propagator: propagation.TraceContext{}}),
		},
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, publisher.Close())
	}()

	_, err = publisher.Publish(parentCtx, &publish.Message{Data: []byte("trace")})
	require.NoError(t, err)
	parent.End()

	select {
	case <-seenCh:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for trace-aware core consume")
	}
	require.ErrorIs(t, <-errCh, context.Canceled)
}

func TestJetStreamPullTracePropagationEndToEnd(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	nc := newConn(t)
	defer nc.Close()
	js := newJetStream(t, nc)

	provider := sdktrace.NewTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()
	tracer := provider.Tracer("integration")
	parentCtx, parent := tracer.Start(context.Background(), "parent")
	parentTraceID := parent.SpanContext().TraceID()

	stream := uniqueName("stream_pull_trace")
	subject := uniqueName("subject.pull.trace")
	consumerName := uniqueName("consumer_pull_trace")
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
		GlobalHandlers: []publish.Handler{
			ptrace.New(ptrace.Config{Tracer: tracer, Propagator: propagation.TraceContext{}}),
		},
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
		GlobalHandlers: []consume.Handler{
			ctrace.New(ctrace.Config{Tracer: tracer, Propagator: propagation.TraceContext{}}),
		},
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, consumer.Close())
	}()

	var seen atomic.Bool
	errCh := make(chan error, 1)
	go func() {
		errCh <- consumer.Consume(ctx, func(handlerCtx context.Context, msg *consume.MessageContext) error {
			require.Equal(t, parentTraceID, oteltrace.SpanContextFromContext(handlerCtx).TraceID())
			require.NotEmpty(t, msg.Message.Header.Get("traceparent"))
			if seen.CompareAndSwap(false, true) {
				cancel()
			}
			return nil
		})
	}()

	_, err = publisher.Publish(parentCtx, &publish.Message{Data: []byte("trace")})
	require.NoError(t, err)
	parent.End()

	require.Eventually(t, seen.Load, 10*time.Second, 50*time.Millisecond)
	require.ErrorIs(t, <-errCh, context.Canceled)
}
