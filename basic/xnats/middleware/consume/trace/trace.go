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

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/codesjoy/pkg/basic/xnats/internal/tracecarrier"
	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
)

const (
	defaultTracerName = "github.com/codesjoy/pkg/basic/xnats"
	unknownSubject    = "_unknown"
)

// Config controls consume trace middleware behavior.
type Config struct {
	Tracer     oteltrace.Tracer
	Propagator propagation.TextMapPropagator
}

// Middleware traces consume handler execution with OpenTelemetry.
type Middleware struct {
	tracer     oteltrace.Tracer
	propagator propagation.TextMapPropagator
}

// New creates consume tracing middleware.
func New(cfg Config) *Middleware {
	tracerProvider := cfg.Tracer
	if tracerProvider == nil {
		tracerProvider = otel.Tracer(defaultTracerName)
	}
	propagator := cfg.Propagator
	if propagator == nil {
		propagator = otel.GetTextMapPropagator()
	}

	return &Middleware{tracer: tracerProvider, propagator: propagator}
}

// Handle extracts trace context from message headers and traces one consume attempt.
func (m *Middleware) Handle(
	ctx context.Context,
	msg *consume.MessageContext,
	next consume.Next,
) error {
	if m == nil {
		return next(ctx, msg)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if msg != nil && msg.Message != nil {
		carrier := tracecarrier.HeaderCarrier{Header: &msg.Message.Header}
		ctx = m.propagator.Extract(ctx, carrier)
	}

	spanCtx, span := m.tracer.Start(ctx, spanName(msg), oteltrace.WithAttributes(spanAttrs(msg)...))
	defer span.End()

	err := next(spanCtx, msg)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetStatus(codes.Ok, "")
	return nil
}

func spanName(msg *consume.MessageContext) string {
	return "xnats.consume " + subjectName(msg)
}

func subjectName(msg *consume.MessageContext) string {
	if msg == nil || msg.Subject == "" {
		return unknownSubject
	}
	return msg.Subject
}

func spanAttrs(msg *consume.MessageContext) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("messaging.system", "nats"),
		attribute.String("messaging.destination.name", subjectName(msg)),
		attribute.String("messaging.operation", "process"),
	}
	if msg == nil {
		return attrs
	}

	attrs = append(attrs,
		attribute.String("xnats.transport", string(msg.Transport)),
		attribute.Int("xnats.attempt", msg.Attempt),
		attribute.String("xnats.logical_key", msg.LogicalKey),
		attribute.Int("xnats.shard", msg.Shard),
	)
	if msg.JetStream != nil {
		attrs = append(attrs,
			attribute.String("xnats.stream", msg.JetStream.Stream),
			attribute.String("xnats.consumer", msg.JetStream.Consumer),
			attribute.Int64("xnats.stream_sequence", int64(msg.JetStream.StreamSequence)),
			attribute.Int64("xnats.consumer_sequence", int64(msg.JetStream.ConsumerSequence)),
			attribute.Int64("xnats.num_delivered", int64(msg.JetStream.NumDelivered)),
		)
	}

	return attrs
}
