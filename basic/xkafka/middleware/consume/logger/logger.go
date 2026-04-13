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

package logger

import (
	"context"
	"log/slog"
	"time"

	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
)

// Middleware logs message handling outcomes with slog.
// 消费者日志中间件，使用 slog 记录消息处理结果。
type Middleware struct {
	// logger 是结构化日志记录器。
	logger *slog.Logger
}

// New creates a slog-based logging middleware.
func New(logger *slog.Logger) *Middleware {
	if logger == nil {
		logger = slog.Default()
	}
	return &Middleware{logger: logger}
}

// Handle logs result and forwards execution to next.
// 记录处理开始时间，调用下游处理器，然后根据结果记录日志。
func (m *Middleware) Handle(
	ctx context.Context,
	msg *consume.MessageContext,
	next consume.Next,
) error {
	if m == nil || m.logger == nil {
		return next(ctx, msg)
	}

	// 记录开始时间
	start := time.Now()
	// 调用下游处理器
	err := next(ctx, msg)
	// 构建日志属性
	attrs := logAttrs(msg, time.Since(start))

	if err != nil {
		// 记录失败日志
		attrs = append(attrs,
			slog.String("result", "error"),
			slog.String("error", err.Error()),
		)
		m.logger.ErrorContext(ctx, "xkafka handle failed", attrs...)
		return err
	}

	// 记录成功日志
	attrs = append(attrs, slog.String("result", "success"))
	m.logger.InfoContext(ctx, "xkafka handle success", attrs...)
	return nil
}

// logAttrs 构建消费者日志的结构化属性列表。
func logAttrs(msg *consume.MessageContext, duration time.Duration) []any {
	attrs := make([]any, 0, 9)
	attrs = append(attrs, slog.Duration("duration", duration))

	if msg == nil || msg.Message == nil {
		return attrs
	}

	attrs = append(attrs,
		slog.String("topic", msg.Message.Topic),
		slog.Int64("partition", int64(msg.Message.Partition)),
		slog.Int64("offset", msg.Message.Offset),
		slog.String("logical_key", msg.LogicalKey),
		slog.Int("shard", msg.Shard),
		slog.Int("attempt", msg.Attempt),
	)
	return attrs
}
