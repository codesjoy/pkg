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

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/codesjoy/pkg/basic/xnats/internal/tracecarrier"
	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
)

const (
	defaultTracerName = "github.com/codesjoy/pkg/basic/xnats"
	unknownSubject    = "_unknown"
)

// Config controls publish trace middleware behavior.
type Config struct {
	Tracer     oteltrace.Tracer
	Propagator propagation.TextMapPropagator
}

// Middleware traces publish handler execution with OpenTelemetry.
type Middleware struct {
	tracer     oteltrace.Tracer
	propagator propagation.TextMapPropagator
}

// New creates publish tracing middleware.
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

// Handle injects trace context into message headers and traces one publish attempt.
func (m *Middleware) Handle(
	ctx context.Context,
	msg *publish.MessageContext,
	next publish.Next,
) (*publish.Result, error) {
	if m == nil {
		return next(ctx, msg)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	spanCtx, span := m.tracer.Start(ctx, spanName(msg), oteltrace.WithAttributes(spanAttrs(msg)...))
	defer span.End()

	if msg != nil && msg.Message != nil {
		if msg.Message.Header == nil {
			msg.Message.Header = nats.Header{}
		}
		carrier := tracecarrier.HeaderCarrier{Header: &msg.Message.Header}
		m.propagator.Inject(spanCtx, carrier)
	}

	result, err := next(spanCtx, msg)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	if result != nil {
		if result.Stream != "" {
			span.SetAttributes(attribute.String("xnats.stream", result.Stream))
		}
		if result.Sequence > 0 {
			span.SetAttributes(attribute.Int64("xnats.sequence", int64(result.Sequence)))
		}
	}

	span.SetStatus(codes.Ok, "")
	return result, nil
}

func spanName(msg *publish.MessageContext) string {
	return "xnats.publish " + subjectName(msg)
}

func subjectName(msg *publish.MessageContext) string {
	if msg == nil || msg.Message == nil || msg.Message.Subject == "" {
		return unknownSubject
	}
	return msg.Message.Subject
}

func spanAttrs(msg *publish.MessageContext) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("messaging.system", "nats"),
		attribute.String("messaging.destination.name", subjectName(msg)),
		attribute.String("messaging.operation", "publish"),
		attribute.String("messaging.destination.kind", "topic"),
	}
	if msg == nil {
		return attrs
	}
	attrs = append(attrs, attribute.Int("xnats.attempt", msg.Attempt))
	return attrs
}
