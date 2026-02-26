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
	"strings"

	"github.com/IBM/sarama"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
)

const (
	defaultTracerName = "github.com/codesjoy/pkg/basic/xkafka"
	unknownTopic      = "_unknown"
)

// Config controls consume trace middleware behavior.
type Config struct {
	Tracer     trace.Tracer
	Propagator propagation.TextMapPropagator
}

// Middleware traces consume handler execution with OpenTelemetry.
type Middleware struct {
	tracer     trace.Tracer
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
		carrier := newHeaderCarrier(&msg.Message.Headers)
		ctx = m.propagator.Extract(ctx, &carrier)
	}

	spanCtx, span := m.tracer.Start(ctx, spanName(msg), trace.WithAttributes(spanAttrs(msg)...))
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
	return "xkafka.consume " + topicName(msg)
}

func topicName(msg *consume.MessageContext) string {
	if msg == nil || msg.Message == nil || msg.Message.Topic == "" {
		return unknownTopic
	}
	return msg.Message.Topic
}

func spanAttrs(msg *consume.MessageContext) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("messaging.system", "kafka"),
		attribute.String("messaging.destination.name", topicName(msg)),
	}

	if msg == nil {
		return attrs
	}

	attrs = append(attrs,
		attribute.String("xkafka.logical_key", msg.LogicalKey),
		attribute.Int("xkafka.shard", msg.Shard),
		attribute.Int("xkafka.attempt", msg.Attempt),
	)

	if msg.Message != nil {
		attrs = append(attrs,
			attribute.Int("messaging.kafka.partition", int(msg.Message.Partition)),
			attribute.Int64("messaging.kafka.message.offset", msg.Message.Offset),
		)
	}

	return attrs
}

type headerCarrier struct {
	headers *[]*sarama.RecordHeader
}

func newHeaderCarrier(headers *[]*sarama.RecordHeader) headerCarrier {
	return headerCarrier{headers: headers}
}

func (c *headerCarrier) Get(key string) string {
	if c == nil || c.headers == nil {
		return ""
	}
	for _, header := range *c.headers {
		if header == nil {
			continue
		}
		if strings.EqualFold(string(header.Key), key) {
			return string(header.Value)
		}
	}
	return ""
}

func (c *headerCarrier) Set(key, value string) {
	if c == nil || c.headers == nil {
		return
	}

	for _, header := range *c.headers {
		if header == nil {
			continue
		}
		if strings.EqualFold(string(header.Key), key) {
			header.Key = []byte(key)
			header.Value = []byte(value)
			return
		}
	}

	*c.headers = append(*c.headers, &sarama.RecordHeader{Key: []byte(key), Value: []byte(value)})
}

func (c *headerCarrier) Keys() []string {
	if c == nil || c.headers == nil {
		return nil
	}

	keys := make([]string, 0, len(*c.headers))
	for _, header := range *c.headers {
		if header == nil {
			continue
		}
		keys = append(keys, string(header.Key))
	}
	return keys
}
