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

package trace

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
	cretry "github.com/codesjoy/pkg/basic/xkafka/middleware/consume/retry"
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
		Message:    &sarama.ConsumerMessage{Topic: "orders", Partition: 3, Offset: 99},
		LogicalKey: "order-1",
		Shard:      7,
		Attempt:    2,
	}
	err := middleware.Handle(
		context.Background(),
		msg,
		func(context.Context, *consume.MessageContext) error {
			return nil
		},
	)
	require.NoError(t, err)

	span := findSpanByName(recorder.Ended(), "xkafka.consume orders")
	require.NotNil(t, span)
	require.Equal(t, codes.Ok, span.Status().Code)
	require.Equal(t, "kafka", attrString(span, "messaging.system"))
	require.Equal(t, "orders", attrString(span, "messaging.destination.name"))
	require.Equal(t, int64(3), attrInt(span, "messaging.kafka.partition"))
	require.Equal(t, int64(99), attrInt(span, "messaging.kafka.message.offset"))
	require.Equal(t, "order-1", attrString(span, "xkafka.logical_key"))
	require.Equal(t, int64(7), attrInt(span, "xkafka.shard"))
	require.Equal(t, int64(2), attrInt(span, "xkafka.attempt"))
}

func TestMiddlewareExtractParentContext(t *testing.T) {
	t.Parallel()

	provider, recorder := newTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()

	tracer := provider.Tracer("test")
	propagator := propagation.TraceContext{}

	parentCtx, parent := tracer.Start(context.Background(), "parent")
	carrier := propagation.MapCarrier{}
	propagator.Inject(parentCtx, carrier)
	parentSpanContext := parent.SpanContext()
	parent.End()

	headers := make([]*sarama.RecordHeader, 0, len(carrier))
	for key, value := range carrier {
		headers = append(headers, &sarama.RecordHeader{Key: []byte(key), Value: []byte(value)})
	}

	middleware := New(Config{Tracer: tracer, Propagator: propagator})
	err := middleware.Handle(context.Background(), &consume.MessageContext{
		Message: &sarama.ConsumerMessage{Topic: "orders", Headers: headers},
	}, func(context.Context, *consume.MessageContext) error {
		return nil
	})
	require.NoError(t, err)

	child := findSpanByName(recorder.Ended(), "xkafka.consume orders")
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
		Message: &sarama.ConsumerMessage{Topic: "orders"},
	}, func(context.Context, *consume.MessageContext) error {
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)

	span := findSpanByName(recorder.Ended(), "xkafka.consume orders")
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

	span := findSpanByName(recorder.Ended(), "xkafka.consume _unknown")
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
		}, cretry.ExhaustedPolicyStop, nil, retryLogger, nil),
		New(Config{Tracer: provider.Tracer("test"), Propagator: propagation.TraceContext{}}),
	}, func(_ context.Context, msg *consume.MessageContext) error {
		if msg.Attempt == 1 {
			return errors.New("retry once")
		}
		return nil
	})

	err := chain(context.Background(), &consume.MessageContext{
		Message: &sarama.ConsumerMessage{Topic: "orders"},
	})
	require.NoError(t, err)
	require.Equal(t, 2, countSpanByName(recorder.Ended(), "xkafka.consume orders"))
}

func newTracerProvider() (*trace.TracerProvider, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	return provider, recorder
}

func findSpanByName(spans []trace.ReadOnlySpan, name string) trace.ReadOnlySpan {
	for _, span := range spans {
		if span.Name() == name {
			return span
		}
	}
	return nil
}

func countSpanByName(spans []trace.ReadOnlySpan, name string) int {
	count := 0
	for _, span := range spans {
		if span.Name() == name {
			count++
		}
	}
	return count
}

func attrString(span trace.ReadOnlySpan, key string) string {
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

func attrInt(span trace.ReadOnlySpan, key string) int64 {
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
