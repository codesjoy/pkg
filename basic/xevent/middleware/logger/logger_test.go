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

	"github.com/codesjoy/pkg/basic/xevent"
)

type testEvent struct{}

func (*testEvent) EventType() string               { return "order.created" }
func (*testEvent) EventID() string                 { return "evt-1" }
func (*testEvent) PartitionKey() string            { return "" }
func (*testEvent) Topic() string                   { return "" }
func (*testEvent) MarshalPayload() ([]byte, error) { return nil, nil }
func (*testEvent) UnmarshalPayload([]byte) error   { return nil }

func TestMiddlewareLogsSuccess(t *testing.T) {
	handler := &captureHandler{}
	event := &testEvent{}
	err := New(Config{Logger: slog.New(handler), LogEvent: true}).Handle(
		context.Background(),
		&xevent.EventContext{
			Message: &xevent.Message{EventType: "message.type"},
			Event:   event,
		},
		func(context.Context, *xevent.EventContext) error { return nil },
	)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	record := handler.singleRecord(t)
	if record.level != slog.LevelInfo || record.message != "event handle" {
		t.Fatalf("unexpected log record: %#v", record)
	}
	if record.attrs["event_type"] != "order.created" || record.attrs["event_id"] != "evt-1" {
		t.Fatalf("unexpected event attributes: %v", record.attrs)
	}
	if record.attrs["event"] != event || record.attrs["duration"] == nil {
		t.Fatalf("unexpected event attributes: %v", record.attrs)
	}
	if _, ok := record.attrs["result"]; ok {
		t.Fatalf("unexpected result attribute: %v", record.attrs)
	}
	if _, ok := record.attrs["err"]; ok {
		t.Fatalf("unexpected err attribute: %v", record.attrs)
	}
}

func TestMiddlewareLogsAndReturnsError(t *testing.T) {
	handler := &captureHandler{}
	wantErr := errors.New("handler failed")
	err := New(Config{Logger: slog.New(handler)}).Handle(
		context.Background(),
		&xevent.EventContext{Event: &testEvent{}},
		func(context.Context, *xevent.EventContext) error { return wantErr },
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected handler error, got %v", err)
	}

	record := handler.singleRecord(t)
	if record.level != slog.LevelError || record.message != "event handle" {
		t.Fatalf("unexpected log record: %#v", record)
	}
	if record.attrs["err"] != wantErr {
		t.Fatalf("unexpected error attributes: %v", record.attrs)
	}
	if _, ok := record.attrs["event"]; ok {
		t.Fatalf("unexpected event attribute: %v", record.attrs)
	}
	if _, ok := record.attrs["result"]; ok {
		t.Fatalf("unexpected result attribute: %v", record.attrs)
	}
}

func TestMiddlewareDoesNotLogEventByDefault(t *testing.T) {
	handler := &captureHandler{}
	event := &testEvent{}
	err := New(Config{Logger: slog.New(handler)}).Handle(
		context.Background(),
		&xevent.EventContext{Event: event},
		func(context.Context, *xevent.EventContext) error { return nil },
	)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if _, ok := handler.singleRecord(t).attrs["event"]; ok {
		t.Fatal("expected event attribute to be omitted by default")
	}
}

func TestNewUsesDefaultLoggerWhenNil(t *testing.T) {
	if got := New(Config{}).logger; got != slog.Default() {
		t.Fatalf("expected slog.Default, got %p", got)
	}
}

type capturedRecord struct {
	level   slog.Level
	message string
	attrs   map[string]any
}

type captureHandler struct {
	mu      sync.Mutex
	records []capturedRecord
}

func (*captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := make(map[string]any)
	record.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})
	h.mu.Lock()
	h.records = append(h.records, capturedRecord{record.Level, record.Message, attrs})
	h.mu.Unlock()
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func (h *captureHandler) singleRecord(t *testing.T) capturedRecord {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.records) != 1 {
		t.Fatalf("expected one log record, got %d", len(h.records))
	}
	return h.records[0]
}
