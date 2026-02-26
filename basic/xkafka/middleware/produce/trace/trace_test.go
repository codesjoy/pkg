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
	"strings"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
	pretry "github.com/codesjoy/pkg/basic/xkafka/middleware/produce/retry"
)

func TestMiddlewareSuccessInjectAndAttrs(t *testing.T) {
	t.Parallel()

	provider, recorder := newTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()

	middleware := New(Config{
		Tracer:     provider.Tracer("test"),
		Propagator: propagation.TraceContext{},
	})

	msg := &produce.MessageContext{
		Message: &produce.Message{
			Topic:   "orders",
			Headers: []sarama.RecordHeader{{Key: []byte("x-business"), Value: []byte("v")}},
		},
		DispatchKey: "order-1",
		Worker:      3,
		Attempt:     2,
	}

	result, err := middleware.Handle(
		context.Background(),
		msg,
		func(context.Context, *produce.MessageContext) (*produce.Result, error) {
			return &produce.Result{Topic: "orders", Partition: 9, Offset: 101}, nil
		},
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, "v", headerValue(msg.Message.Headers, "x-business"))
	require.NotEmpty(t, headerValue(msg.Message.Headers, "traceparent"))

	span := findSpanByName(recorder.Ended(), "xkafka.produce orders")
	require.NotNil(t, span)
	require.Equal(t, codes.Ok, span.Status().Code)
	require.Equal(t, "kafka", attrString(span, "messaging.system"))
	require.Equal(t, "orders", attrString(span, "messaging.destination.name"))
	require.Equal(t, "order-1", attrString(span, "xkafka.dispatch_key"))
	require.Equal(t, int64(3), attrInt(span, "xkafka.worker"))
	require.Equal(t, int64(2), attrInt(span, "xkafka.attempt"))
	require.Equal(t, int64(9), attrInt(span, "messaging.kafka.partition"))
	require.Equal(t, int64(101), attrInt(span, "messaging.kafka.message.offset"))
}

func TestMiddlewareErrorStatus(t *testing.T) {
	t.Parallel()

	provider, recorder := newTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()

	middleware := New(Config{Tracer: provider.Tracer("test")})
	wantErr := errors.New("send failed")

	result, err := middleware.Handle(context.Background(), &produce.MessageContext{
		Message: &produce.Message{Topic: "orders"},
	}, func(context.Context, *produce.MessageContext) (*produce.Result, error) {
		return nil, wantErr
	})
	require.Nil(t, result)
	require.ErrorIs(t, err, wantErr)

	span := findSpanByName(recorder.Ended(), "xkafka.produce orders")
	require.NotNil(t, span)
	require.Equal(t, codes.Error, span.Status().Code)
	require.Equal(t, "send failed", span.Status().Description)
}

func TestMiddlewareRetryCreatesAttemptSpans(t *testing.T) {
	t.Parallel()

	provider, recorder := newTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()

	retryLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	chain := produce.Compose([]produce.Handler{
		pretry.New(pretry.Config{
			MaxRetries:     1,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     time.Millisecond,
			Multiplier:     1,
		}, pretry.ExhaustedPolicyStop, nil, retryLogger),
		New(Config{Tracer: provider.Tracer("test"), Propagator: propagation.TraceContext{}}),
	}, func(_ context.Context, msg *produce.MessageContext) (*produce.Result, error) {
		if msg.Attempt == 1 {
			return nil, errors.New("retry once")
		}
		return &produce.Result{Topic: "orders", Partition: 0, Offset: 1}, nil
	})

	_, err := chain(context.Background(), &produce.MessageContext{
		Message: &produce.Message{Topic: "orders"},
	})
	require.NoError(t, err)
	require.Equal(t, 2, countSpanByName(recorder.Ended(), "xkafka.produce orders"))
}

func TestMiddlewareInjectReplacesExistingTraceHeader(t *testing.T) {
	t.Parallel()

	provider, _ := newTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()

	middleware := New(
		Config{Tracer: provider.Tracer("test"), Propagator: propagation.TraceContext{}},
	)

	msg := &produce.MessageContext{
		Message: &produce.Message{Topic: "orders", Headers: []sarama.RecordHeader{
			{
				Key:   []byte("traceparent"),
				Value: []byte("00-00000000000000000000000000000000-0000000000000000-01"),
			},
		}},
	}

	_, err := middleware.Handle(
		context.Background(),
		msg,
		func(context.Context, *produce.MessageContext) (*produce.Result, error) {
			return &produce.Result{Topic: "orders"}, nil
		},
	)
	require.NoError(t, err)

	traceparent := headerValue(msg.Message.Headers, "traceparent")
	require.NotEmpty(t, traceparent)
	require.False(t, strings.Contains(traceparent, "00000000000000000000000000000000"))
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

func headerValue(headers []sarama.RecordHeader, key string) string {
	for idx := range headers {
		header := headers[idx]
		if strings.EqualFold(string(header.Key), key) {
			return string(header.Value)
		}
	}
	return ""
}
