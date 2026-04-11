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

package debezium

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/codesjoy/pkg/basic/xevent"
	"github.com/google/uuid"
)

type testEvent struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	TopicName string `json:"-"`
}

func (*testEvent) EventType() string {
	return "order.created"
}

func (e *testEvent) EventID() string {
	return e.ID
}

func (e *testEvent) PartitionKey() string {
	return e.Key
}

func (e *testEvent) Topic() string {
	return e.TopicName
}

func (e *testEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(e)
}

func (e *testEvent) UnmarshalPayload(data []byte) error {
	return json.Unmarshal(data, e)
}

type badTestEvent struct{}

func (*badTestEvent) EventType() string               { return "" }
func (*badTestEvent) EventID() string                 { return "" }
func (*badTestEvent) PartitionKey() string            { return "" }
func (*badTestEvent) Topic() string                   { return "" }
func (*badTestEvent) MarshalPayload() ([]byte, error) { return nil, nil }
func (*badTestEvent) UnmarshalPayload([]byte) error   { return nil }

type stubStore struct {
	appendErr error
	appended  *Record
}

func (s *stubStore) Append(_ context.Context, record *Record) error {
	if s.appendErr != nil {
		return s.appendErr
	}
	cloned := cloneRecord(*record)
	s.appended = &cloned
	return nil
}

func TestAppendEventRejectsNilStore(t *testing.T) {
	_, err := AppendEvent(
		context.Background(),
		nil,
		&testEvent{ID: "evt_1"},
		AppendOptions{Topic: "orders"},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "xevent outbox debezium store is nil" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecordTableName(t *testing.T) {
	if got := (Record{}).TableName(); got != defaultTableName {
		t.Fatalf("unexpected table name: %q", got)
	}
}

func TestNewRecordValidationAndTopicResolution(t *testing.T) {
	t.Run("nil outbound", func(t *testing.T) {
		_, err := NewRecord(nil, AppendOptions{})
		if !errors.Is(err, xevent.ErrNilOutbound) {
			t.Fatalf("expected ErrNilOutbound, got %v", err)
		}
	})

	t.Run("empty event type", func(t *testing.T) {
		_, err := NewRecord(&xevent.Outbound{}, AppendOptions{Topic: "orders"})
		if !errors.Is(err, xevent.ErrEventTypeRequired) {
			t.Fatalf("expected ErrEventTypeRequired, got %v", err)
		}
	})

	t.Run("missing topic", func(t *testing.T) {
		_, err := NewRecord(&xevent.Outbound{EventType: "evt"}, AppendOptions{})
		if !errors.Is(err, ErrTopicRequired) {
			t.Fatalf("expected ErrTopicRequired, got %v", err)
		}
	})

	t.Run("outbound topic wins", func(t *testing.T) {
		outbound := &xevent.Outbound{
			EventType:    "evt",
			EventID:      "evt-1",
			PartitionKey: "p1",
			Topic:        "orders.v2",
			Payload:      []byte("payload"),
		}
		record, err := NewRecord(outbound, AppendOptions{Topic: "orders.v1"})
		if err != nil {
			t.Fatalf("NewRecord returned error: %v", err)
		}
		if record.Topic != "orders.v2" {
			t.Fatalf("expected outbound topic to win, got %q", record.Topic)
		}
		if _, err := uuid.Parse(record.ID); err != nil {
			t.Fatalf("expected generated UUID id, got %q: %v", record.ID, err)
		}
		outbound.Payload[0] = 'P'
		if string(record.Payload) != "payload" {
			t.Fatalf("expected payload clone, got %q", record.Payload)
		}
	})

	t.Run("append option topic fallback", func(t *testing.T) {
		record, err := NewRecord(&xevent.Outbound{
			EventType:    "evt",
			EventID:      "evt-2",
			PartitionKey: "p2",
			Payload:      []byte("payload"),
		}, AppendOptions{Topic: "orders"})
		if err != nil {
			t.Fatalf("NewRecord returned error: %v", err)
		}
		if record.Topic != "orders" {
			t.Fatalf("unexpected topic: %q", record.Topic)
		}
	})
}

func TestAppendEventPropagatesErrors(t *testing.T) {
	t.Run("encode error", func(t *testing.T) {
		_, err := AppendEvent(
			context.Background(),
			&stubStore{},
			&badTestEvent{},
			AppendOptions{Topic: "orders"},
		)
		if !errors.Is(err, xevent.ErrEventTypeRequired) {
			t.Fatalf("expected ErrEventTypeRequired, got %v", err)
		}
	})

	t.Run("store append error", func(t *testing.T) {
		want := errors.New("append failed")
		_, err := AppendEvent(
			context.Background(),
			&stubStore{appendErr: want},
			&testEvent{ID: "evt_1", Key: "order-1"},
			AppendOptions{Topic: "orders"},
		)
		if !errors.Is(err, want) {
			t.Fatalf("expected append error, got %v", err)
		}
	})
}

func TestPrepareStoredRecordDefaultsAndClone(t *testing.T) {
	now := time.Date(2026, 4, 11, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	record := Record{
		Topic:        "orders",
		PartitionKey: "order-1",
		EventType:    "order.created",
		EventID:      "evt_1",
		Payload:      []byte("payload"),
	}

	stored := prepareStoredRecord(record, now)
	if _, err := uuid.Parse(stored.ID); err != nil {
		t.Fatalf("expected generated UUID id, got %q: %v", stored.ID, err)
	}
	if !stored.CreatedAt.Equal(now.UTC()) {
		t.Fatalf("unexpected created_at: %v", stored.CreatedAt)
	}

	record.Payload[0] = 'P'
	if string(stored.Payload) != "payload" {
		t.Fatalf("expected payload clone, got %q", stored.Payload)
	}

	cloned := cloneRecord(stored)
	stored.Payload[0] = 'X'
	if string(cloned.Payload) != "payload" {
		t.Fatalf("expected clone copy, got %q", cloned.Payload)
	}
}
