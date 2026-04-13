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

	phandler "github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
)

// Middleware logs producer handling outcomes with slog.
// 生产者日志中间件，使用 slog 记录消息发送结果。
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
	msg *phandler.MessageContext,
	next phandler.Next,
) (*phandler.Result, error) {
	if m == nil || m.logger == nil {
		return next(ctx, msg)
	}

	// 记录开始时间
	start := time.Now()
	// 调用下游处理器
	result, err := next(ctx, msg)
	// 构建日志属性
	attrs := logAttrs(msg, result, time.Since(start))
	if err != nil {
		// 记录失败日志
		attrs = append(attrs,
			slog.String("result", "error"),
			slog.String("error", err.Error()),
		)
		m.logger.ErrorContext(ctx, "xkafka produce failed", attrs...)
		return nil, err
	}

	// 记录成功日志
	attrs = append(attrs, slog.String("result", "success"))
	m.logger.InfoContext(ctx, "xkafka produce success", attrs...)
	return result, nil
}

// logAttrs 构建生产者日志的结构化属性列表。
func logAttrs(msg *phandler.MessageContext, result *phandler.Result, duration time.Duration) []any {
	attrs := make([]any, 0, 10)
	attrs = append(attrs, slog.Duration("duration", duration))

	if msg != nil {
		attrs = append(attrs,
			slog.Int("attempt", msg.Attempt),
			slog.String("dispatch_key", msg.DispatchKey),
			slog.Int("worker", msg.Worker),
		)
		if msg.Message != nil {
			attrs = append(attrs,
				slog.String("topic", msg.Message.Topic),
				slog.String("key", string(msg.Message.Key)),
			)
		}
	}

	if result != nil {
		attrs = append(attrs,
			slog.Int64("partition", int64(result.Partition)),
			slog.Int64("offset", result.Offset),
		)
	}

	return attrs
}
