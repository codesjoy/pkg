package trace

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
	pretry "github.com/codesjoy/pkg/basic/xnats/middleware/publish/retry"
)

func TestMiddlewareSuccessInjectAndAttrs(t *testing.T) {
	t.Parallel()

	provider, recorder := newTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()

	middleware := New(Config{
		Tracer:     provider.Tracer("test"),
		Propagator: propagation.TraceContext{},
	})

	msg := &publish.MessageContext{
		Message: &publish.Message{
			Subject: "orders.created",
			Header:  nats.Header{"x-business": []string{"v"}},
		},
		Attempt: 2,
	}

	result, err := middleware.Handle(
		context.Background(),
		msg,
		func(context.Context, *publish.MessageContext) (*publish.Result, error) {
			return &publish.Result{
				Subject:  "orders.created",
				Stream:   "ORDERS",
				Sequence: 101,
			}, nil
		},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "v", msg.Message.Header.Get("x-business"))
	require.NotEmpty(t, msg.Message.Header.Get("traceparent"))

	span := findSpanByName(recorder.Ended(), "xnats.publish orders.created")
	require.NotNil(t, span)
	require.Equal(t, codes.Ok, span.Status().Code)
	require.Equal(t, "nats", attrString(span, "messaging.system"))
	require.Equal(t, "orders.created", attrString(span, "messaging.destination.name"))
	require.Equal(t, int64(2), attrInt(span, "xnats.attempt"))
	require.Equal(t, "ORDERS", attrString(span, "xnats.stream"))
	require.Equal(t, int64(101), attrInt(span, "xnats.sequence"))
}

func TestMiddlewareErrorStatus(t *testing.T) {
	t.Parallel()

	provider, recorder := newTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()

	middleware := New(Config{Tracer: provider.Tracer("test")})
	wantErr := errors.New("send failed")

	result, err := middleware.Handle(context.Background(), &publish.MessageContext{
		Message: &publish.Message{Subject: "orders.created"},
	}, func(context.Context, *publish.MessageContext) (*publish.Result, error) {
		return nil, wantErr
	})
	require.Nil(t, result)
	require.ErrorIs(t, err, wantErr)

	span := findSpanByName(recorder.Ended(), "xnats.publish orders.created")
	require.NotNil(t, span)
	require.Equal(t, codes.Error, span.Status().Code)
	require.Equal(t, "send failed", span.Status().Description)
}

func TestMiddlewareRetryCreatesAttemptSpans(t *testing.T) {
	t.Parallel()

	provider, recorder := newTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()

	retryLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	chain := publish.Compose([]publish.Handler{
		pretry.New(pretry.Config{
			MaxRetries:     1,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     time.Millisecond,
			Multiplier:     1,
		}, pretry.ExhaustedPolicyStop, nil, retryLogger),
		New(Config{Tracer: provider.Tracer("test"), Propagator: propagation.TraceContext{}}),
	}, func(_ context.Context, msg *publish.MessageContext) (*publish.Result, error) {
		if msg.Attempt == 1 {
			return nil, errors.New("retry once")
		}
		return &publish.Result{Subject: "orders.created"}, nil
	})

	_, err := chain(context.Background(), &publish.MessageContext{
		Message: &publish.Message{Subject: "orders.created"},
	})
	require.NoError(t, err)
	require.Equal(t, 2, countSpanByName(recorder.Ended(), "xnats.publish orders.created"))
}

func TestMiddlewareInjectReplacesExistingTraceHeader(t *testing.T) {
	t.Parallel()

	provider, _ := newTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()

	middleware := New(
		Config{Tracer: provider.Tracer("test"), Propagator: propagation.TraceContext{}},
	)
	msg := &publish.MessageContext{
		Message: &publish.Message{
			Subject: "orders.created",
			Header: nats.Header{
				"traceparent": []string{"00-00000000000000000000000000000000-0000000000000000-01"},
			},
		},
	}

	_, err := middleware.Handle(
		context.Background(),
		msg,
		func(context.Context, *publish.MessageContext) (*publish.Result, error) {
			return &publish.Result{Subject: "orders.created"}, nil
		},
	)
	require.NoError(t, err)

	traceparent := msg.Message.Header.Get("traceparent")
	require.NotEmpty(t, traceparent)
	require.False(t, strings.Contains(traceparent, "00000000000000000000000000000000"))
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
