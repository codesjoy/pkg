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
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
)

func TestCommandSlowAndFailureLogging(t *testing.T) {
	t.Parallel()

	handler := &memoryHandler{}
	monitors := New(Config{
		Logger:         slog.New(handler),
		SlowThreshold:  time.Nanosecond,
		LogCommandBody: true,
	})

	raw, err := bson.Marshal(bson.D{{Key: "ping", Value: 1}})
	require.NoError(t, err)

	monitors.Command.Started(context.Background(), &event.CommandStartedEvent{
		Command:      raw,
		DatabaseName: "app",
		CommandName:  "find",
		RequestID:    1,
		ConnectionID: "conn-1",
	})
	monitors.Command.Succeeded(context.Background(), &event.CommandSucceededEvent{
		CommandFinishedEvent: event.CommandFinishedEvent{
			CommandName:  "find",
			RequestID:    1,
			ConnectionID: "conn-1",
			Duration:     time.Millisecond,
		},
	})

	monitors.Command.Started(context.Background(), &event.CommandStartedEvent{
		CommandName:  "insert",
		RequestID:    2,
		ConnectionID: "conn-2",
	})
	monitors.Command.Failed(context.Background(), &event.CommandFailedEvent{
		CommandFinishedEvent: event.CommandFinishedEvent{
			CommandName:  "insert",
			RequestID:    2,
			ConnectionID: "conn-2",
			Duration:     time.Millisecond,
		},
		Failure: errors.New("boom"),
	})

	records := handler.Records()
	require.Len(t, records, 2)
	require.Equal(t, "xmongo command slow", records[0].message)
	require.Equal(t, "find", records[0].attrs["command"])
	require.Equal(t, "app", records[0].attrs["database"])
	require.NotEmpty(t, records[0].attrs["command_body"])
	require.Equal(t, "xmongo command failed", records[1].message)
	require.Equal(t, "boom", records[1].attrs["error"])
}

func TestPoolEventFilterAndHeartbeatLogging(t *testing.T) {
	t.Parallel()

	handler := &memoryHandler{}
	monitors := New(Config{
		Logger: slog.New(handler),
		PoolEventFilter: func(eventType string) bool {
			return eventType == event.ConnectionPoolCleared
		},
	})

	monitors.Pool.Event(&event.PoolEvent{Type: event.ConnectionPoolCleared})
	monitors.Pool.Event(&event.PoolEvent{
		Type:    event.ConnectionCheckOutFailed,
		Address: "127.0.0.1:27017",
	})
	monitors.Server.ServerHeartbeatFailed(&event.ServerHeartbeatFailedEvent{
		ConnectionID: "server-1",
		Duration:     time.Millisecond,
		Failure:      context.DeadlineExceeded,
	})

	records := handler.Records()
	require.Len(t, records, 2)
	require.Equal(t, "xmongo pool event", records[0].message)
	require.Equal(t, event.ConnectionCheckOutFailed, records[0].attrs["event_type"])
	require.Equal(t, "xmongo server heartbeat failed", records[1].message)
}

type memoryHandler struct {
	mu      sync.Mutex
	records []memoryRecord
}

type memoryRecord struct {
	message string
	attrs   map[string]any
}

func (h *memoryHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *memoryHandler) Handle(_ context.Context, r slog.Record) error {
	record := memoryRecord{
		message: r.Message,
		attrs:   make(map[string]any),
	}
	r.Attrs(func(attr slog.Attr) bool {
		record.attrs[attr.Key] = attr.Value.Any()
		return true
	})
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, record)
	return nil
}

func (h *memoryHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *memoryHandler) WithGroup(string) slog.Handler      { return h }

func (h *memoryHandler) Records() []memoryRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	cloned := make([]memoryRecord, len(h.records))
	copy(cloned, h.records)
	return cloned
}
