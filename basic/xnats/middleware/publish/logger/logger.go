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

	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
)

// Middleware logs publish handling outcomes with slog.
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
	msg *publish.MessageContext,
	next publish.Next,
) (*publish.Result, error) {
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
		m.logger.ErrorContext(ctx, "xnats publish failed", attrs...)
		return nil, err
	}

	attrs = append(attrs, slog.String("result", "success"))
	m.logger.InfoContext(ctx, "xnats publish success", attrs...)
	return result, nil
}

func logAttrs(msg *publish.MessageContext, result *publish.Result, duration time.Duration) []any {
	attrs := []any{slog.Duration("duration", duration)}
	if msg != nil && msg.Message != nil {
		attrs = append(attrs,
			slog.String("subject", msg.Message.Subject),
			slog.String("reply", msg.Message.Reply),
			slog.Int("attempt", msg.Attempt),
		)
	}
	if result != nil {
		attrs = append(attrs,
			slog.String("stream", result.Stream),
			slog.Uint64("sequence", result.Sequence),
			slog.Bool("duplicate", result.Duplicate),
		)
	}
	return attrs
}
