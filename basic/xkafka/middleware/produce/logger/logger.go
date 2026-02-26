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
type Middleware struct {
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
func (m *Middleware) Handle(
	ctx context.Context,
	msg *phandler.MessageContext,
	next phandler.Next,
) (*phandler.Result, error) {
	if m == nil || m.logger == nil {
		return next(ctx, msg)
	}

	start := time.Now()
	result, err := next(ctx, msg)
	attrs := logAttrs(msg, result, time.Since(start))
	if err != nil {
		attrs = append(attrs,
			slog.String("result", "error"),
			slog.String("error", err.Error()),
		)
		m.logger.ErrorContext(ctx, "xkafka produce failed", attrs...)
		return nil, err
	}

	attrs = append(attrs, slog.String("result", "success"))
	m.logger.InfoContext(ctx, "xkafka produce success", attrs...)
	return result, nil
}

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
