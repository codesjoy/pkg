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
)

func TestMemoryStoreAppendAndClaimRespectCanceledContext(t *testing.T) {
	store := NewMemoryStore()

	appendCtx, appendCancel := context.WithCancel(context.Background())
	appendCancel()
	err := store.Append(
		appendCtx,
		&Record{EventType: "evt", EventID: "append", PartitionKey: "p", Payload: []byte("payload")},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled from Append, got %v", err)
	}

	claimCtx, claimCancel := context.WithCancel(context.Background())
	claimCancel()
	_, err = store.Claim(claimCtx, ClaimRequest{
		Owner:    "relay-1",
		Now:      time.Now(),
		ClaimTTL: time.Minute,
		Limit:    1,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled from Claim, got %v", err)
	}
}

func TestMemoryStoreRetryAndMarkFailedErrors(t *testing.T) {
	now := time.Date(2026, 3, 26, 16, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		run  func(*MemoryStore, uint64) error
	}{
		{
			name: "retry not found",
			run: func(store *MemoryStore, _ uint64) error {
				return store.Retry(context.Background(), RetryRequest{
					ID:              999,
					Owner:           "relay-1",
					Now:             now,
					NextAvailableAt: now,
					LastError:       "temporary",
				})
			},
		},
		{
			name: "mark failed not found",
			run: func(store *MemoryStore, _ uint64) error {
				return store.MarkFailed(context.Background(), FailRequest{
					ID:        999,
					Owner:     "relay-1",
					Now:       now,
					LastError: "permanent",
				})
			},
		},
		{
			name: "retry wrong owner",
			run: func(store *MemoryStore, id uint64) error {
				return store.Retry(context.Background(), RetryRequest{
					ID:              id,
					Owner:           "relay-2",
					Now:             now,
					NextAvailableAt: now,
					LastError:       "temporary",
				})
			},
		},
		{
			name: "mark failed wrong owner",
			run: func(store *MemoryStore, id uint64) error {
				return store.MarkFailed(context.Background(), FailRequest{
					ID:        id,
					Owner:     "relay-2",
					Now:       now,
					LastError: "permanent",
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMemoryStore()
			record := &Record{
				EventType:    "evt",
				EventID:      "evt-1",
				PartitionKey: "p1",
				Payload:      []byte("payload"),
				AvailableAt:  now.Add(-time.Minute),
			}
			if err := store.Append(context.Background(), record); err != nil {
				t.Fatalf("Append returned error: %v", err)
			}
			if _, err := store.Claim(context.Background(), ClaimRequest{
				Owner:    "relay-1",
				Now:      now,
				ClaimTTL: time.Minute,
				Limit:    1,
			}); err != nil {
				t.Fatalf("Claim returned error: %v", err)
			}

			err := tt.run(store, record.ID)
			if stringsContain(tt.name, "not found") {
				if !errors.Is(err, ErrRecordNotFound) {
					t.Fatalf("expected ErrRecordNotFound, got %v", err)
				}
				return
			}
			if !errors.Is(err, ErrClaimNotOwned) {
				t.Fatalf("expected ErrClaimNotOwned, got %v", err)
			}
		})
	}
}

func TestIsClaimEligibleBranches(t *testing.T) {
	now := time.Date(2026, 3, 26, 17, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Second)
	future := now.Add(time.Second)

	tests := []struct {
		name   string
		record Record
		want   bool
	}{
		{
			name: "pending available",
			record: Record{
				Status:      StatusPending,
				AvailableAt: now,
			},
			want: true,
		},
		{
			name: "pending future",
			record: Record{
				Status:      StatusPending,
				AvailableAt: future,
			},
			want: false,
		},
		{
			name: "sending future available",
			record: Record{
				Status:      StatusSending,
				AvailableAt: future,
			},
			want: false,
		},
		{
			name: "sending nil claim until",
			record: Record{
				Status:      StatusSending,
				AvailableAt: now,
			},
			want: true,
		},
		{
			name: "sending expired claim",
			record: Record{
				Status:      StatusSending,
				AvailableAt: now,
				ClaimUntil:  &expired,
			},
			want: true,
		},
		{
			name: "finished sent",
			record: Record{
				Status:      StatusSent,
				AvailableAt: now,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isClaimEligible(tt.record, now); got != tt.want {
				t.Fatalf("unexpected eligibility: got %v want %v", got, tt.want)
			}
		})
	}
}

func TestRecordOrderLessUsesIDTiebreaker(t *testing.T) {
	now := time.Date(2026, 3, 26, 18, 0, 0, 0, time.UTC)
	left := Record{ID: 1, AvailableAt: now}
	right := Record{ID: 2, AvailableAt: now}

	if !recordOrderLess(left, right) {
		t.Fatal("expected lower id to win when available_at ties")
	}
	if recordOrderLess(right, left) {
		t.Fatal("expected higher id to lose when available_at ties")
	}
}

func stringsContain(value string, needle string) bool {
	return len(value) >= len(needle) && (value == needle || containsSubstring(value, needle))
}

func containsSubstring(value string, needle string) bool {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
