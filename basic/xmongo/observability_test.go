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

package xmongo

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"

	logmiddleware "github.com/codesjoy/pkg/basic/xmongo/middleware/logger"
)

func TestWithLoggerRespectsCommandMonitorOptionOrder(t *testing.T) {
	t.Parallel()

	recorder := &eventRecorder{}
	handler := newEventLogHandler(func(msg string, _ map[string]any) {
		if msg == "xmongo command slow" {
			recorder.Append("logger")
		}
	})

	before := &event.CommandMonitor{
		Succeeded: func(context.Context, *event.CommandSucceededEvent) {
			recorder.Append("before")
		},
	}
	after := &event.CommandMonitor{
		Succeeded: func(context.Context, *event.CommandSucceededEvent) {
			recorder.Append("after")
		},
	}

	merged, err := buildClientOptions(
		Config{URI: "mongodb://127.0.0.1:27017"},
		WithCommandMonitor(before),
		WithLogger(logmiddleware.Config{
			Logger:        slog.New(handler),
			SlowThreshold: time.Nanosecond,
		}),
		WithCommandMonitor(after),
	)
	require.NoError(t, err)
	require.NotNil(t, merged.Monitor)

	raw, err := bson.Marshal(bson.D{{Key: "find", Value: "widgets"}})
	require.NoError(t, err)
	merged.Monitor.Started(context.Background(), &event.CommandStartedEvent{
		Command:      raw,
		DatabaseName: "app",
		CommandName:  "find",
		RequestID:    1,
		ConnectionID: "conn-1",
	})
	merged.Monitor.Succeeded(context.Background(), &event.CommandSucceededEvent{
		CommandFinishedEvent: event.CommandFinishedEvent{
			CommandName:  "find",
			RequestID:    1,
			ConnectionID: "conn-1",
			Duration:     time.Millisecond,
		},
	})

	require.Equal(t, []string{"before", "logger", "after"}, recorder.Values())
}

func TestWithLoggerRespectsPoolMonitorOptionOrder(t *testing.T) {
	t.Parallel()

	recorder := &eventRecorder{}
	handler := newEventLogHandler(func(msg string, _ map[string]any) {
		if msg == "xmongo pool event" {
			recorder.Append("logger")
		}
	})

	before := &event.PoolMonitor{
		Event: func(*event.PoolEvent) {
			recorder.Append("before")
		},
	}
	after := &event.PoolMonitor{
		Event: func(*event.PoolEvent) {
			recorder.Append("after")
		},
	}

	merged, err := buildClientOptions(
		Config{URI: "mongodb://127.0.0.1:27017"},
		WithPoolMonitor(before),
		WithLogger(logmiddleware.Config{Logger: slog.New(handler)}),
		WithPoolMonitor(after),
	)
	require.NoError(t, err)
	require.NotNil(t, merged.PoolMonitor)

	merged.PoolMonitor.Event(&event.PoolEvent{
		Type:    event.ConnectionPoolCleared,
		Address: "127.0.0.1:27017",
	})

	require.Equal(t, []string{"before", "logger", "after"}, recorder.Values())
}

type eventRecorder struct {
	mu     sync.Mutex
	events []string
}

func (r *eventRecorder) Append(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *eventRecorder) Values() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	cloned := make([]string, len(r.events))
	copy(cloned, r.events)
	return cloned
}

type eventLogHandler struct {
	record func(msg string, attrs map[string]any)
}

func newEventLogHandler(fn func(msg string, attrs map[string]any)) *eventLogHandler {
	return &eventLogHandler{record: fn}
}

func (h *eventLogHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *eventLogHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]any)
	r.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})
	h.record(r.Message, attrs)
	return nil
}

func (h *eventLogHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *eventLogHandler) WithGroup(string) slog.Handler      { return h }
