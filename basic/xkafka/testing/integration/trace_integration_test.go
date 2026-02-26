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

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/codesjoy/pkg/basic/xkafka"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
	ctrace "github.com/codesjoy/pkg/basic/xkafka/middleware/consume/trace"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
	ptrace "github.com/codesjoy/pkg/basic/xkafka/middleware/produce/trace"
)

type traceObservation struct {
	traceID     string
	traceParent string
}

func TestTracePropagationProducerToGroupConsumer(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	topic := uniqueKafkaName("trace")
	createTopic(t, topic, 1)

	provider, recorder := newTracerProvider()
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	tracer := provider.Tracer("xkafka-integration")
	propagator := propagation.TraceContext{}

	consumer, err := xkafka.NewGroupConsumer(xkafka.GroupConsumerConfig{
		Brokers:      mustBrokers(t),
		GroupID:      uniqueKafkaName("trace_group"),
		Topics:       []string{topic},
		SaramaConfig: newConsumerSaramaConfig(),
		GlobalHandlers: []consume.Handler{
			ctrace.New(ctrace.Config{Tracer: tracer, Propagator: propagator}),
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, consumer.Close())
	})

	obsCh := make(chan traceObservation, 1)
	consumeErrCh := make(chan error, 1)
	go func() {
		consumeErrCh <- consumer.Consume(ctx, func(handlerCtx context.Context, msg *consume.MessageContext) error {
			spanContext := oteltrace.SpanContextFromContext(handlerCtx)
			obs := traceObservation{
				traceID:     spanContext.TraceID().String(),
				traceParent: consumeHeaderValue(msg.Message.Headers, "traceparent"),
			}
			select {
			case obsCh <- obs:
			default:
			}
			cancel()
			return nil
		})
	}()

	producer, err := xkafka.NewProducer(xkafka.ProducerConfig{
		Brokers:      mustBrokers(t),
		DefaultTopic: topic,
		SaramaConfig: newProducerSaramaConfig(),
		GlobalHandlers: []produce.Handler{
			ptrace.New(ptrace.Config{Tracer: tracer, Propagator: propagator}),
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, producer.Close())
	})

	parentCtx, parentSpan := tracer.Start(ctx, "integration-parent")
	parentTraceID := parentSpan.SpanContext().TraceID().String()

	_, err = producer.Produce(parentCtx, &produce.Message{
		Key:   []byte("trace-key"),
		Value: []byte("trace-value"),
	})
	parentSpan.End()
	require.NoError(t, err)

	var observation traceObservation
	select {
	case observation = <-obsCh:
	case <-ctx.Done():
		t.Fatal("timed out waiting for consume observation")
	}

	consumeErr := waitForError(t, consumeErrCh, 30*time.Second)
	require.ErrorIs(t, consumeErr, context.Canceled)

	require.NotEmpty(t, observation.traceParent)
	require.NotEmpty(t, observation.traceID)
	require.Equal(t, parentTraceID, observation.traceID)

	require.GreaterOrEqual(t, countSpanByName(recorder.Ended(), "xkafka.produce "+topic), 1)
	require.GreaterOrEqual(t, countSpanByName(recorder.Ended(), "xkafka.consume "+topic), 1)
}

func newTracerProvider() (*sdktrace.TracerProvider, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	return provider, recorder
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

func consumeHeaderValue(headers []*sarama.RecordHeader, key string) string {
	for _, header := range headers {
		if header == nil {
			continue
		}
		if string(header.Key) == key {
			return string(header.Value)
		}
	}
	return ""
}
