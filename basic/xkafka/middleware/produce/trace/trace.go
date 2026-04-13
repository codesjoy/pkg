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

// Package trace provides produce-side OpenTelemetry tracing middleware.
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

	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
)

const (
	defaultTracerName = "github.com/codesjoy/pkg/basic/xkafka"
	unknownTopic      = "_unknown"
)

// Config controls produce trace middleware behavior.
// 生产者追踪中间件的配置。
type Config struct {
	// Tracer 是 OpenTelemetry Tracer 实例，nil 时使用全局默认。
	Tracer trace.Tracer
	// Propagator 是 trace context 传播器，nil 时使用全局默认。
	Propagator propagation.TextMapPropagator
}

// Middleware traces produce handler execution with OpenTelemetry.
// 生产者追踪中间件，创建 span 并注入 trace context 到消息头。
type Middleware struct {
	// tracer 是 OpenTelemetry Tracer。
	tracer trace.Tracer
	// propagator 是 trace context 传播器。
	propagator propagation.TextMapPropagator
}

// New creates produce tracing middleware.
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

// Handle injects trace context into message headers and traces one produce attempt.
// 创建 producer span，注入 trace context 到消息头，记录结果和错误状态。
func (m *Middleware) Handle(
	ctx context.Context,
	msg *produce.MessageContext,
	next produce.Next,
) (*produce.Result, error) {
	if m == nil {
		return next(ctx, msg)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// 创建 producer span
	spanCtx, span := m.tracer.Start(ctx, spanName(msg), trace.WithAttributes(spanAttrs(msg)...))
	defer span.End()

	// 将 trace context 注入到消息头
	if msg != nil && msg.Message != nil {
		carrier := newHeaderCarrier(&msg.Message.Headers)
		m.propagator.Inject(spanCtx, &carrier)
	}

	// 调用下游处理器
	result, err := next(spanCtx, msg)
	if err != nil {
		// 记录错误到 span
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	// 将发送结果写入 span 属性
	if result != nil {
		span.SetAttributes(
			attribute.Int("messaging.kafka.partition", int(result.Partition)),
			attribute.Int64("messaging.kafka.message.offset", result.Offset),
		)
	}

	span.SetStatus(codes.Ok, "")
	return result, nil
}

// spanName 生成 span 名称，格式为 "xkafka.produce {topic}"。
func spanName(msg *produce.MessageContext) string {
	return "xkafka.produce " + topicName(msg)
}

// topicName 安全提取消息的 topic 名称。
func topicName(msg *produce.MessageContext) string {
	if msg == nil || msg.Message == nil || msg.Message.Topic == "" {
		return unknownTopic
	}
	return msg.Message.Topic
}

// spanAttrs 构建 span 的属性列表。
func spanAttrs(msg *produce.MessageContext) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("messaging.system", "kafka"),
		attribute.String("messaging.destination.name", topicName(msg)),
	}

	if msg == nil {
		return attrs
	}

	attrs = append(attrs,
		attribute.String("xkafka.dispatch_key", msg.DispatchKey),
		attribute.Int("xkafka.worker", msg.Worker),
		attribute.Int("xkafka.attempt", msg.Attempt),
	)

	return attrs
}

// headerCarrier 适配 Kafka 消息头为 OpenTelemetry TextMapCarrier 接口。
type headerCarrier struct {
	headers *[]sarama.RecordHeader
}

// newHeaderCarrier 创建 headerCarrier 实例。
func newHeaderCarrier(headers *[]sarama.RecordHeader) headerCarrier {
	return headerCarrier{headers: headers}
}

// Get 从消息头中获取指定 key 的值（大小写不敏感）。
func (c *headerCarrier) Get(key string) string {
	if c == nil || c.headers == nil {
		return ""
	}

	for idx := range *c.headers {
		header := (*c.headers)[idx]
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

	for idx := range *c.headers {
		header := &(*c.headers)[idx]
		if strings.EqualFold(string(header.Key), key) {
			header.Key = []byte(key)
			header.Value = []byte(value)
			return
		}
	}

	*c.headers = append(*c.headers, sarama.RecordHeader{Key: []byte(key), Value: []byte(value)})
}

// Keys 返回所有消息头的 key 列表。
func (c *headerCarrier) Keys() []string {
	if c == nil || c.headers == nil {
		return nil
	}

	keys := make([]string, 0, len(*c.headers))
	for idx := range *c.headers {
		keys = append(keys, string((*c.headers)[idx].Key))
	}
	return keys
}
