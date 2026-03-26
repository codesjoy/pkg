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

package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/codesjoy/pkg/basic/xevent"
)

func TestAppendEventRejectsNilStore(t *testing.T) {
	_, err := AppendEvent(context.Background(), nil, &stubEvent{eventID: "evt"}, AppendOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "xevent outbox store is nil" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecordTableName(t *testing.T) {
	if got := (Record{}).TableName(); got != defaultTableName {
		t.Fatalf("unexpected table name: %q", got)
	}
}

func TestNewRecordValidationAndCopy(t *testing.T) {
	t.Run("nil outbound", func(t *testing.T) {
		_, err := NewRecord(nil, AppendOptions{})
		if !errors.Is(err, xevent.ErrNilOutbound) {
			t.Fatalf("expected ErrNilOutbound, got %v", err)
		}
	})

	t.Run("empty event type", func(t *testing.T) {
		_, err := NewRecord(&xevent.Outbound{}, AppendOptions{})
		if !errors.Is(err, xevent.ErrEventTypeRequired) {
			t.Fatalf("expected ErrEventTypeRequired, got %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		availableAt := time.Date(2026, 3, 26, 10, 30, 0, 0, time.FixedZone("CST", 8*3600))
		outbound := &xevent.Outbound{
			EventType:    "evt",
			EventID:      "evt-1",
			PartitionKey: "p1",
			Payload:      []byte("payload"),
		}

		record, err := NewRecord(outbound, AppendOptions{AvailableAt: availableAt})
		if err != nil {
			t.Fatalf("NewRecord returned error: %v", err)
		}
		if record.Status != StatusPending {
			t.Fatalf("unexpected status: %q", record.Status)
		}
		if !record.AvailableAt.Equal(availableAt.UTC()) {
			t.Fatalf("unexpected available_at: %v", record.AvailableAt)
		}

		outbound.Payload[0] = 'P'
		if string(record.Payload) != "payload" {
			t.Fatalf("expected payload clone, got %q", record.Payload)
		}
	})
}

func TestAppendEventPropagatesErrors(t *testing.T) {
	t.Run("encode error", func(t *testing.T) {
		_, err := AppendEvent(context.Background(), &stubStore{}, &badStubEvent{}, AppendOptions{})
		if !errors.Is(err, xevent.ErrEventTypeRequired) {
			t.Fatalf("expected ErrEventTypeRequired, got %v", err)
		}
	})

	t.Run("store append error", func(t *testing.T) {
		want := errors.New("append failed")
		_, err := AppendEvent(
			context.Background(),
			&stubStore{appendErr: want},
			&stubEvent{eventID: "evt"},
			AppendOptions{},
		)
		if !errors.Is(err, want) {
			t.Fatalf("expected append error, got %v", err)
		}
	})
}

func TestNormalizeClaimRequestRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		req     ClaimRequest
		wantErr string
	}{
		{
			name:    "missing owner",
			req:     ClaimRequest{Limit: 1, ClaimTTL: time.Second},
			wantErr: "xevent outbox claim owner is required",
		},
		{
			name:    "missing limit",
			req:     ClaimRequest{Owner: "relay", ClaimTTL: time.Second},
			wantErr: "xevent outbox claim limit must be > 0",
		},
		{
			name:    "missing ttl",
			req:     ClaimRequest{Owner: "relay", Limit: 1},
			wantErr: "xevent outbox claim ttl must be > 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeClaimRequest(tt.req)
			if err == nil {
				t.Fatal("expected error")
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestMemoryStoreErrors(t *testing.T) {
	store := NewMemoryStore()

	if err := store.Append(context.Background(), nil); err == nil {
		t.Fatal("expected nil record error")
	} else if err.Error() != "xevent outbox record is nil" {
		t.Fatalf("unexpected nil record error: %v", err)
	}

	if err := store.MarkSent(context.Background(), MarkSentRequest{
		ID:     1,
		Owner:  "relay-1",
		Now:    time.Now(),
		SentAt: time.Now(),
	}); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got %v", err)
	}

	record := &Record{
		EventType:    "evt",
		EventID:      "owned-by-other",
		PartitionKey: "p1",
		Payload:      []byte("payload"),
	}
	if err := store.Append(context.Background(), record); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}
	if _, err := store.Claim(context.Background(), ClaimRequest{
		Owner:    "relay-1",
		Now:      time.Now(),
		ClaimTTL: time.Minute,
		Limit:    1,
	}); err != nil {
		t.Fatalf("Claim returned error: %v", err)
	}

	err := store.MarkSent(context.Background(), MarkSentRequest{
		ID:     record.ID,
		Owner:  "relay-2",
		Now:    time.Now(),
		SentAt: time.Now(),
	})
	if !errors.Is(err, ErrClaimNotOwned) {
		t.Fatalf("expected ErrClaimNotOwned, got %v", err)
	}
}

func TestNormalizeMutationRequests(t *testing.T) {
	t.Run("mark sent", func(t *testing.T) {
		normalized, err := normalizeMarkSentRequest(MarkSentRequest{
			Owner: "relay-1",
			Now:   time.Date(2026, 3, 26, 19, 0, 0, 0, time.FixedZone("CST", 8*3600)),
		})
		if err != nil {
			t.Fatalf("normalizeMarkSentRequest returned error: %v", err)
		}
		if normalized.SentAt.IsZero() || !normalized.SentAt.Equal(normalized.Now) {
			t.Fatalf(
				"expected SentAt to default to Now, got sent_at=%v now=%v",
				normalized.SentAt,
				normalized.Now,
			)
		}
		if normalized.Now.Location() != time.UTC {
			t.Fatalf("expected UTC now, got %v", normalized.Now.Location())
		}
	})

	t.Run("retry", func(t *testing.T) {
		normalized, err := normalizeRetryRequest(RetryRequest{
			Owner: "relay-1",
			Now:   time.Date(2026, 3, 26, 19, 30, 0, 0, time.FixedZone("CST", 8*3600)),
		})
		if err != nil {
			t.Fatalf("normalizeRetryRequest returned error: %v", err)
		}
		if normalized.NextAvailableAt.IsZero() ||
			!normalized.NextAvailableAt.Equal(normalized.Now) {
			t.Fatalf(
				"expected NextAvailableAt to default to Now, got next_available_at=%v now=%v",
				normalized.NextAvailableAt,
				normalized.Now,
			)
		}
	})

	t.Run("fail", func(t *testing.T) {
		normalized, err := normalizeFailRequest(FailRequest{Owner: "relay-1"})
		if err != nil {
			t.Fatalf("normalizeFailRequest returned error: %v", err)
		}
		if normalized.Now.IsZero() {
			t.Fatal("expected normalized now")
		}
	})

	t.Run("owner required", func(t *testing.T) {
		if _, err := normalizeMarkSentRequest(MarkSentRequest{}); err == nil {
			t.Fatal("expected mark-sent owner error")
		}
		if _, err := normalizeRetryRequest(RetryRequest{}); err == nil {
			t.Fatal("expected retry owner error")
		}
		if _, err := normalizeFailRequest(FailRequest{}); err == nil {
			t.Fatal("expected fail owner error")
		}
	})
}

func TestNormalizeHelpers(t *testing.T) {
	fallback := time.Date(2026, 3, 26, 20, 0, 0, 0, time.FixedZone("CST", 8*3600))
	if got := normalizeTime(time.Time{}, func() time.Time { return fallback }); !got.Equal(
		fallback.UTC(),
	) {
		t.Fatalf("unexpected fallback time: %v", got)
	}

	local := time.Date(2026, 3, 26, 20, 30, 0, 0, time.FixedZone("CST", 8*3600))
	if got := normalizeTime(local, time.Now); !got.Equal(local.UTC()) {
		t.Fatalf("unexpected normalized time: %v", got)
	}

	ctx := context.TODO()
	if normalizeContext(ctx) != ctx {
		t.Fatal("expected non-nil context to pass through unchanged")
	}
}

type stubEvent struct {
	eventID string
}

func (*stubEvent) EventType() string {
	return "evt"
}

func (e *stubEvent) EventID() string {
	return e.eventID
}

func (*stubEvent) PartitionKey() string {
	return "partition"
}

func (*stubEvent) MarshalPayload() ([]byte, error) {
	return []byte("payload"), nil
}

func (*stubEvent) UnmarshalPayload([]byte) error {
	return nil
}

type badStubEvent struct{}

func (*badStubEvent) EventType() string {
	return ""
}

func (*badStubEvent) EventID() string {
	return "bad"
}

func (*badStubEvent) PartitionKey() string {
	return ""
}

func (*badStubEvent) MarshalPayload() ([]byte, error) {
	return []byte("bad"), nil
}

func (*badStubEvent) UnmarshalPayload([]byte) error {
	return nil
}

type stubStore struct {
	appendErr error
}

func (s *stubStore) Append(context.Context, *Record) error {
	return s.appendErr
}

func (*stubStore) Claim(context.Context, ClaimRequest) ([]Record, error) {
	return nil, nil
}

func (*stubStore) MarkSent(context.Context, MarkSentRequest) error {
	return nil
}

func (*stubStore) Retry(context.Context, RetryRequest) error {
	return nil
}

func (*stubStore) MarkFailed(context.Context, FailRequest) error {
	return nil
}
