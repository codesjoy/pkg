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

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/IBM/sarama"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/codesjoy/pkg/basic/xkafka"
	"github.com/codesjoy/pkg/basic/xkafka/examples/internal/examplecfg"
	cmw "github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
	ctrace "github.com/codesjoy/pkg/basic/xkafka/middleware/consume/trace"
	pmw "github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
	ptrace "github.com/codesjoy/pkg/basic/xkafka/middleware/produce/trace"
)

func main() {
	cfg, err := examplecfg.Load()
	if err != nil {
		fail(fmt.Errorf("load config: %w", err))
	}

	logger := examplecfg.NewLogger()
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx, cancel := examplecfg.WithTimeout(rootCtx, cfg.Timeout)
	defer cancel()

	tracerProvider := trace.NewTracerProvider(trace.WithSampler(trace.AlwaysSample()))
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutdownCancel()
		if shutdownErr := tracerProvider.Shutdown(shutdownCtx); shutdownErr != nil {
			logger.Error("shutdown tracer provider failed", "error", shutdownErr)
		}
	}()

	propagator := propagation.TraceContext{}
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagator)

	tracer := tracerProvider.Tracer("github.com/codesjoy/pkg/basic/xkafka/examples/trace")
	groupID := fmt.Sprintf("%s-trace-%d", cfg.GroupID, time.Now().UnixNano())

	producer, err := xkafka.NewProducer(xkafka.ProducerConfig{
		Brokers:      cfg.Brokers,
		DefaultTopic: cfg.Topic,
		Logger:       logger,
		GlobalHandlers: []pmw.Handler{
			ptrace.New(ptrace.Config{Tracer: tracer, Propagator: propagator}),
		},
	})
	if err != nil {
		fail(fmt.Errorf("create producer: %w", err))
	}
	defer closeOrLog(logger, "producer", producer.Close)

	consumeSaramaCfg := sarama.NewConfig()
	consumeSaramaCfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	consumer, err := xkafka.NewGroupConsumer(xkafka.GroupConsumerConfig{
		Brokers:      cfg.Brokers,
		GroupID:      groupID,
		Topics:       []string{cfg.Topic},
		Logger:       logger,
		SaramaConfig: consumeSaramaCfg,
		GlobalHandlers: []cmw.Handler{
			ctrace.New(ctrace.Config{Tracer: tracer, Propagator: propagator}),
		},
	})
	if err != nil {
		fail(fmt.Errorf("create group consumer: %w", err))
	}
	defer closeOrLog(logger, "group consumer", consumer.Close)

	parentCtx, parentSpan := tracer.Start(ctx, "xkafka.example.parent")
	parentTraceID := parentSpan.SpanContext().TraceID()
	_, err = producer.Produce(parentCtx, &pmw.Message{
		Key:   []byte("trace-demo-key"),
		Value: []byte("trace-demo-value"),
	})
	parentSpan.End()
	if err != nil {
		fail(fmt.Errorf("produce trace message: %w", err))
	}

	logger.Info("produced message with trace context", "trace_id", parentTraceID.String())

	consumeCtx, stopConsume := context.WithCancel(ctx)
	defer stopConsume()

	err = consumer.Consume(
		consumeCtx,
		func(handlerCtx context.Context, msg *cmw.MessageContext) error {
			if msg == nil || msg.Message == nil {
				return nil
			}

			traceHeader := findHeader(msg.Message.Headers, "traceparent")
			consumedTraceID := oteltrace.SpanFromContext(handlerCtx).SpanContext().TraceID()
			logger.Info(
				"consumed traced message",
				"topic", msg.Message.Topic,
				"partition", msg.Message.Partition,
				"offset", msg.Message.Offset,
				"traceparent", traceHeader,
				"consumed_trace_id", consumedTraceID.String(),
				"same_trace", consumedTraceID == parentTraceID,
			)

			stopConsume()
			return nil
		},
	)
	if err != nil && !errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded) {
		fail(fmt.Errorf("consume traced message: %w", err))
	}

	logger.Info("trace example completed")
}

func findHeader(headers []*sarama.RecordHeader, key string) string {
	for idx := range headers {
		header := headers[idx]
		if header == nil {
			continue
		}
		if strings.EqualFold(string(header.Key), key) {
			return string(header.Value)
		}
	}
	return ""
}

func closeOrLog(logger *slog.Logger, name string, closeFn func() error) {
	if closeFn == nil {
		return
	}
	if err := closeFn(); err != nil {
		logger.Error("close failed", "resource", name, "error", err)
	}
}

func fail(err error) {
	if err == nil {
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
