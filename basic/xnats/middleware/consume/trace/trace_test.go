package trace

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
	cretry "github.com/codesjoy/pkg/basic/xnats/middleware/consume/retry"
)

func TestMiddlewareSuccess(t *testing.T) {
	t.Parallel()

	provider, recorder := newTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()

	middleware := New(Config{
		Tracer:     provider.Tracer("test"),
		Propagator: propagation.TraceContext{},
	})

	msg := &consume.MessageContext{
		Transport: consume.TransportJetStream,
		Subject:   "orders.created",
		Attempt:   2,
		JetStream: &consume.JetStreamMetadata{
			Stream:           "ORDERS",
			Consumer:         "worker",
			StreamSequence:   12,
			ConsumerSequence: 9,
			NumDelivered:     1,
		},
		Message: &nats.Msg{Subject: "orders.created"},
	}

	err := middleware.Handle(
		context.Background(),
		msg,
		func(context.Context, *consume.MessageContext) error {
			return nil
		},
	)
	require.NoError(t, err)

	span := findSpanByName(recorder.Ended(), "xnats.consume orders.created")
	require.NotNil(t, span)
	require.Equal(t, codes.Ok, span.Status().Code)
	require.Equal(t, "nats", attrString(span, "messaging.system"))
	require.Equal(t, "orders.created", attrString(span, "messaging.destination.name"))
	require.Equal(t, "jetstream", attrString(span, "xnats.transport"))
	require.Equal(t, int64(2), attrInt(span, "xnats.attempt"))
	require.Equal(t, "ORDERS", attrString(span, "xnats.stream"))
	require.Equal(t, "worker", attrString(span, "xnats.consumer"))
}

func TestMiddlewareExtractParentContext(t *testing.T) {
	t.Parallel()

	provider, recorder := newTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()

	tracer := provider.Tracer("test")
	propagator := propagation.TraceContext{}

	parentCtx, parent := tracer.Start(context.Background(), "parent")
	header := nats.Header{}
	propagator.Inject(parentCtx, propagation.HeaderCarrier(header))
	parentSpanContext := parent.SpanContext()
	parent.End()

	middleware := New(Config{Tracer: tracer, Propagator: propagator})
	err := middleware.Handle(context.Background(), &consume.MessageContext{
		Subject: "orders.created",
		Message: &nats.Msg{Subject: "orders.created", Header: header},
	}, func(context.Context, *consume.MessageContext) error {
		return nil
	})
	require.NoError(t, err)

	child := findSpanByName(recorder.Ended(), "xnats.consume orders.created")
	require.NotNil(t, child)
	require.Equal(t, parentSpanContext.TraceID(), child.SpanContext().TraceID())
	require.Equal(t, parentSpanContext.SpanID(), child.Parent().SpanID())
}

func TestMiddlewareErrorStatus(t *testing.T) {
	t.Parallel()

	provider, recorder := newTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()

	middleware := New(Config{Tracer: provider.Tracer("test")})
	wantErr := errors.New("business failed")

	err := middleware.Handle(context.Background(), &consume.MessageContext{
		Subject: "orders.created",
		Message: &nats.Msg{Subject: "orders.created"},
	}, func(context.Context, *consume.MessageContext) error {
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)

	span := findSpanByName(recorder.Ended(), "xnats.consume orders.created")
	require.NotNil(t, span)
	require.Equal(t, codes.Error, span.Status().Code)
	require.Equal(t, "business failed", span.Status().Description)
}

func TestMiddlewareNilMessageSafe(t *testing.T) {
	t.Parallel()

	provider, recorder := newTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()

	middleware := New(Config{Tracer: provider.Tracer("test")})
	err := middleware.Handle(
		context.Background(),
		nil,
		func(_ context.Context, msg *consume.MessageContext) error {
			require.Nil(t, msg)
			return nil
		},
	)
	require.NoError(t, err)

	span := findSpanByName(recorder.Ended(), "xnats.consume _unknown")
	require.NotNil(t, span)
	require.Equal(t, codes.Ok, span.Status().Code)
}

func TestMiddlewareRetryCreatesAttemptSpans(t *testing.T) {
	t.Parallel()

	provider, recorder := newTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()

	retryLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	chain := consume.Compose([]consume.Handler{
		cretry.New(cretry.Config{
			MaxRetries:     1,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     time.Millisecond,
			Multiplier:     1,
		}, cretry.ExhaustedPolicyStop, nil, retryLogger),
		New(Config{Tracer: provider.Tracer("test"), Propagator: propagation.TraceContext{}}),
	}, func(_ context.Context, msg *consume.MessageContext) error {
		if msg.Attempt == 1 {
			return errors.New("retry once")
		}
		return nil
	})

	err := chain(context.Background(), &consume.MessageContext{
		Subject: "orders.created",
		Message: &nats.Msg{Subject: "orders.created"},
	})
	require.NoError(t, err)
	require.Equal(t, 2, countSpanByName(recorder.Ended(), "xnats.consume orders.created"))
}

func newTracerProvider() (*sdktrace.TracerProvider, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	return provider, recorder
}

func findSpanByName(spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	for _, span := range spans {
		if span.Name() == name {
			return span
		}
	}
	return nil
}

func countSpanByName(spans []sdktrace.ReadOnlySpan, name string) int {
	count := 0
	for _, span := range spans {
		if span.Name() == name {
			count++
		}
	}
	return count
}

func attrString(span sdktrace.ReadOnlySpan, key string) string {
	if span == nil {
		return ""
	}
	for _, item := range span.Attributes() {
		if string(item.Key) == key {
			return item.Value.AsString()
		}
	}
	return ""
}

func attrInt(span sdktrace.ReadOnlySpan, key string) int64 {
	if span == nil {
		return 0
	}
	for _, item := range span.Attributes() {
		if string(item.Key) == key {
			return item.Value.AsInt64()
		}
	}
	return 0
}
