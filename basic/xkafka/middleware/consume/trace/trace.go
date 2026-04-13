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

// Package trace provides consume-side OpenTelemetry tracing middleware.
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
// 消费者追踪中间件的配置。
type Config struct {
	// Tracer 是 OpenTelemetry Tracer 实例，nil 时使用全局默认。
	Tracer trace.Tracer
	// Propagator 是 trace context 传播器，nil 时使用全局默认。
	Propagator propagation.TextMapPropagator
}

// Middleware traces consume handler execution with OpenTelemetry.
// 消费者追踪中间件，从消息头提取 trace context 并创建 span。
type Middleware struct {
	// tracer 是 OpenTelemetry Tracer。
	tracer trace.Tracer
	// propagator 是 trace context 传播器。
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
// 从消息头提取 trace context，创建 span，记录错误状态。
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

	// 从消息头提取 trace context
	if msg != nil && msg.Message != nil {
		carrier := newHeaderCarrier(&msg.Message.Headers)
		ctx = m.propagator.Extract(ctx, &carrier)
	}

	// 创建 consumer span
	spanCtx, span := m.tracer.Start(ctx, spanName(msg), trace.WithAttributes(spanAttrs(msg)...))
	defer span.End()

	// 调用下游处理器
	err := next(spanCtx, msg)
	if err != nil {
		// 记录错误到 span
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetStatus(codes.Ok, "")
	return nil
}

// spanName 生成 span 名称，格式为 "xkafka.consume {topic}"。
func spanName(msg *consume.MessageContext) string {
	return "xkafka.consume " + topicName(msg)
}

// topicName 安全提取消息的 topic 名称。
func topicName(msg *consume.MessageContext) string {
	if msg == nil || msg.Message == nil || msg.Message.Topic == "" {
		return unknownTopic
	}
	return msg.Message.Topic
}

// spanAttrs 构建 span 的属性列表。
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

// headerCarrier 适配 Kafka 消息头为 OpenTelemetry TextMapCarrier 接口。
type headerCarrier struct {
	headers *[]*sarama.RecordHeader
}

// newHeaderCarrier 创建 headerCarrier 实例。
func newHeaderCarrier(headers *[]*sarama.RecordHeader) headerCarrier {
	return headerCarrier{headers: headers}
}

// Get 从消息头中获取指定 key 的值（大小写不敏感）。
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

// Set 设置消息头中指定 key 的值（大小写不敏感），不存在则追加。
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

// Keys 返回所有消息头的 key 列表。
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
