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

// Package logger provides slog logging for decoded xevent events.
package logger

import (
	"context"
	"log/slog"
	"time"

	"github.com/codesjoy/pkg/basic/xevent"
)

// Middleware logs each typed event message invocation.
type Middleware struct {
	logger   *slog.Logger
	logEvent bool
}

// Config controls event logger middleware behavior.
type Config struct {
	// Logger is the slog logger instance. Nil uses slog.Default().
	Logger *slog.Logger
	// LogEvent controls whether the complete decoded event is included.
	LogEvent bool
}

// New creates a slog-based event middleware.
func New(cfg Config) *Middleware {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Middleware{logger: logger, logEvent: cfg.LogEvent}
}

// Handle invokes the next handler and logs its outcome.
func (m *Middleware) Handle(
	ctx context.Context,
	eventCtx *xevent.EventContext,
	next xevent.Next,
) error {
	started := time.Now()
	err := next(ctx, eventCtx)
	attrs := logAttrs(eventCtx, time.Since(started), m.logEvent)
	if err != nil {
		attrs = append(attrs, slog.Any("err", err))
		m.logger.ErrorContext(ctx, "event handle", attrs...)
		return err
	}
	m.logger.InfoContext(ctx, "event handle", attrs...)
	return nil
}

func logAttrs(eventCtx *xevent.EventContext, duration time.Duration, logEvent bool) []any {
	attrs := []any{slog.Duration("duration", duration)}
	if eventCtx == nil {
		return attrs
	}

	eventType := ""
	if eventCtx.Message != nil {
		eventType = eventCtx.Message.EventType
	}
	eventID := ""
	if eventCtx.Event != nil {
		if typedEventType := eventCtx.Event.EventType(); typedEventType != "" {
			eventType = typedEventType
		}
		eventID = eventCtx.Event.EventID()
	}
	attrs = append(attrs,
		slog.String("event_type", eventType),
		slog.String("event_id", eventID),
	)
	if logEvent {
		attrs = append(attrs, slog.Any("event", eventCtx.Event))
	}
	return attrs
}

var _ xevent.Middleware = (*Middleware)(nil)
