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
	"sync"
	"testing"
	"time"

	"github.com/codesjoy/pkg/basic/xevent"
	"github.com/google/uuid"
)

type fakeSender struct {
	mu      sync.Mutex
	errByID map[string]error
	calls   []string
	ch      chan string
}

func (s *fakeSender) Send(_ context.Context, outbound *xevent.Outbound) error {
	s.mu.Lock()
	s.calls = append(s.calls, outbound.EventID)
	err := s.errByID[outbound.EventID]
	ch := s.ch
	s.mu.Unlock()

	if ch != nil {
		ch <- outbound.EventID
	}
	return err
}

type fakeBatchSender struct {
	fakeSender
	batchCalls   [][]string
	batchErrs    []error
	useBatchErrs bool
}

func (s *fakeBatchSender) BatchSend(_ context.Context, outbounds []*xevent.Outbound) []error {
	s.mu.Lock()
	ids := make([]string, len(outbounds))
	for i, o := range outbounds {
		ids[i] = o.EventID
		s.calls = append(s.calls, o.EventID)
	}
	s.batchCalls = append(s.batchCalls, ids)
	errs := make([]error, len(outbounds))
	if s.useBatchErrs {
		errs = append([]error(nil), s.batchErrs...)
	} else {
		for i, o := range outbounds {
			errs[i] = s.errByID[o.EventID]
		}
	}
	ch := s.ch
	s.mu.Unlock()

	if ch != nil {
		for _, id := range ids {
			ch <- id
		}
	}
	return errs
}

func TestRecordOutboundCarriesTopic(t *testing.T) {
	record := Record{
		EventType:    "evt",
		EventID:      "evt-topic",
		PartitionKey: "p1",
		Payload:      []byte("payload"),
		Topic:        "custom-topic",
	}
	outbound := record.outbound()
	if outbound.Topic != "custom-topic" {
		t.Fatalf("expected topic %q, got %q", "custom-topic", outbound.Topic)
	}
	if outbound.EventType != "evt" {
		t.Fatalf("expected event type %q, got %q", "evt", outbound.EventType)
	}
}

func TestNewRelayRejectsNilDependencies(t *testing.T) {
	_, err := NewRelay(RelayConfig{Sender: &fakeSender{}})
	if err == nil {
		t.Fatal("expected nil store error")
	}
	if err.Error() != "xevent outbox store is nil" {
		t.Fatalf("unexpected nil store error: %v", err)
	}

	_, err = NewRelay(RelayConfig{Store: NewMemoryStore()})
	if err == nil {
		t.Fatal("expected nil sender error")
	}
	if err.Error() != "xevent outbox sender is nil" {
		t.Fatalf("unexpected nil sender error: %v", err)
	}
}

func TestNewRelayGeneratesUUIDOwnerByDefault(t *testing.T) {
	first, err := NewRelay(RelayConfig{
		Store:  NewMemoryStore(),
		Sender: &fakeSender{},
	})
	if err != nil {
		t.Fatalf("first NewRelay returned error: %v", err)
	}

	second, err := NewRelay(RelayConfig{
		Store:  NewMemoryStore(),
		Sender: &fakeSender{},
	})
	if err != nil {
		t.Fatalf("second NewRelay returned error: %v", err)
	}

	if first.owner == "" {
		t.Fatal("expected first relay owner to be set")
	}
	if _, err := uuid.Parse(first.owner); err != nil {
		t.Fatalf("expected first relay owner to be a UUID, got %q: %v", first.owner, err)
	}
	if _, err := uuid.Parse(second.owner); err != nil {
		t.Fatalf("expected second relay owner to be a UUID, got %q: %v", second.owner, err)
	}
	if first.owner == second.owner {
		t.Fatalf("expected unique relay owners, both were %q", first.owner)
	}
}

func TestNewRelayUsesConfiguredOwner(t *testing.T) {
	relay, err := NewRelay(RelayConfig{
		Store:  NewMemoryStore(),
		Sender: &fakeSender{},
		Owner:  "relay-owner-explicit",
	})
	if err != nil {
		t.Fatalf("NewRelay returned error: %v", err)
	}

	if relay.owner != "relay-owner-explicit" {
		t.Fatalf("expected configured relay owner, got %q", relay.owner)
	}
}

func TestRelayProcessOnceTransitionsRecords(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, 3, 26, 14, 0, 0, 0, time.UTC)

	success := &Record{
		EventType:    "evt",
		EventID:      "success",
		PartitionKey: "a",
		Payload:      []byte("success"),
		Status:       StatusPending,
		AvailableAt:  now.Add(-time.Minute),
	}
	retry := &Record{
		EventType:    "evt",
		EventID:      "retry",
		PartitionKey: "b",
		Payload:      []byte("retry"),
		Status:       StatusPending,
		AvailableAt:  now.Add(-time.Minute),
	}
	failed := &Record{
		EventType:    "evt",
		EventID:      "failed",
		PartitionKey: "c",
		Payload:      []byte("failed"),
		Status:       StatusPending,
		Attempts:     1,
		AvailableAt:  now.Add(-time.Minute),
	}
	for _, record := range []*Record{success, retry, failed} {
		if err := store.Append(context.Background(), record); err != nil {
			t.Fatalf("Append returned error: %v", err)
		}
	}

	sender := &fakeSender{
		errByID: map[string]error{
			"retry":  errors.New("temporary"),
			"failed": errors.New("permanent"),
		},
	}
	relay, err := NewRelay(RelayConfig{
		Store:       store,
		Sender:      sender,
		Owner:       "relay-owner",
		RetryDelay:  5 * time.Minute,
		MaxAttempts: 2,
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewRelay returned error: %v", err)
	}

	if err := relay.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce returned error: %v", err)
	}

	store.mu.Lock()
	successRecord := store.records[success.ID]
	retryRecord := store.records[retry.ID]
	failedRecord := store.records[failed.ID]
	store.mu.Unlock()

	if successRecord.Status != StatusSent {
		t.Fatalf("expected success status sent, got %q", successRecord.Status)
	}
	if successRecord.SentAt == nil || !successRecord.SentAt.Equal(now) {
		t.Fatalf("unexpected success sent_at: %v", successRecord.SentAt)
	}

	if retryRecord.Status != StatusPending {
		t.Fatalf("expected retry status pending, got %q", retryRecord.Status)
	}
	if retryRecord.LastError != "temporary" {
		t.Fatalf("unexpected retry last_error: %q", retryRecord.LastError)
	}
	if !retryRecord.AvailableAt.Equal(now.Add(5 * time.Minute)) {
		t.Fatalf("unexpected retry available_at: %v", retryRecord.AvailableAt)
	}
	if retryRecord.Attempts != 1 {
		t.Fatalf("expected retry attempts=1, got %d", retryRecord.Attempts)
	}

	if failedRecord.Status != StatusFailed {
		t.Fatalf("expected failed status failed, got %q", failedRecord.Status)
	}
	if failedRecord.LastError != "permanent" {
		t.Fatalf("unexpected failed last_error: %q", failedRecord.LastError)
	}
	if failedRecord.Attempts != 2 {
		t.Fatalf("expected failed attempts=2, got %d", failedRecord.Attempts)
	}
}

func TestRelayRunWakeProcessesRecordImmediately(t *testing.T) {
	store := NewMemoryStore()

	sender := &fakeSender{
		errByID: map[string]error{},
		ch:      make(chan string, 1),
	}
	relay, err := NewRelay(RelayConfig{
		Store:        store,
		Sender:       sender,
		Owner:        "relay-owner",
		PollInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewRelay returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- relay.Run(ctx)
	}()

	record := &Record{
		EventType:    "evt",
		EventID:      "wake",
		PartitionKey: "p1",
		Payload:      []byte("wake"),
		Status:       StatusPending,
		AvailableAt:  time.Now().Add(-time.Minute).UTC(),
	}
	if err := store.Append(context.Background(), record); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	relay.Wake()

	select {
	case got := <-sender.ch:
		if got != "wake" {
			t.Fatalf("unexpected sender call: %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for wake-driven send")
	}

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for relay shutdown")
	}
}

func TestRelayProcessOnceUsesBatchSender(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, 3, 26, 14, 0, 0, 0, time.UTC)

	success := &Record{
		EventType:    "evt",
		EventID:      "bs-success",
		PartitionKey: "a",
		Payload:      []byte("success"),
		Status:       StatusPending,
		AvailableAt:  now.Add(-time.Minute),
	}
	retry := &Record{
		EventType:    "evt",
		EventID:      "bs-retry",
		PartitionKey: "b",
		Payload:      []byte("retry"),
		Status:       StatusPending,
		AvailableAt:  now.Add(-time.Minute),
	}
	for _, record := range []*Record{success, retry} {
		if err := store.Append(context.Background(), record); err != nil {
			t.Fatalf("Append returned error: %v", err)
		}
	}

	sender := &fakeBatchSender{
		fakeSender: fakeSender{
			errByID: map[string]error{
				"bs-retry": errors.New("temporary"),
			},
		},
	}
	relay, err := NewRelay(RelayConfig{
		Store:       store,
		Sender:      sender,
		Owner:       "batch-relay",
		RetryDelay:  5 * time.Minute,
		MaxAttempts: 2,
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewRelay returned error: %v", err)
	}

	if err := relay.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce returned error: %v", err)
	}

	// Verify BatchSend was called instead of individual Send.
	sender.mu.Lock()
	batchCalls := sender.batchCalls
	sender.mu.Unlock()
	if len(batchCalls) == 0 {
		t.Fatal("expected BatchSend to be called")
	}

	// Verify records transitioned correctly.
	store.mu.Lock()
	successRecord := store.records[success.ID]
	retryRecord := store.records[retry.ID]
	store.mu.Unlock()

	if successRecord.Status != StatusSent {
		t.Fatalf("expected success status sent, got %q", successRecord.Status)
	}
	if retryRecord.Status != StatusPending {
		t.Fatalf("expected retry status pending, got %q", retryRecord.Status)
	}
	if retryRecord.LastError != "temporary" {
		t.Fatalf("unexpected retry last_error: %q", retryRecord.LastError)
	}
}

func TestRelayProcessOnceRejectsBatchResultCountMismatch(t *testing.T) {
	tests := []struct {
		name      string
		batchErrs []error
	}{
		{name: "nil result slice", batchErrs: nil},
		{name: "short result slice", batchErrs: []error{nil}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMemoryStore()
			now := time.Date(2026, 3, 26, 14, 30, 0, 0, time.UTC)

			first := &Record{
				EventType:    "evt",
				EventID:      "bs-first",
				PartitionKey: "a",
				Payload:      []byte("first"),
				Status:       StatusPending,
				AvailableAt:  now.Add(-time.Minute),
			}
			second := &Record{
				EventType:    "evt",
				EventID:      "bs-second",
				PartitionKey: "b",
				Payload:      []byte("second"),
				Status:       StatusPending,
				AvailableAt:  now.Add(-time.Minute),
			}
			for _, record := range []*Record{first, second} {
				if err := store.Append(context.Background(), record); err != nil {
					t.Fatalf("Append returned error: %v", err)
				}
			}

			sender := &fakeBatchSender{
				useBatchErrs: true,
				batchErrs:    tt.batchErrs,
			}
			relay, err := NewRelay(RelayConfig{
				Store:       store,
				Sender:      sender,
				Owner:       "batch-relay",
				RetryDelay:  5 * time.Minute,
				MaxAttempts: 2,
				ClaimTTL:    time.Minute,
				Now:         func() time.Time { return now },
			})
			if err != nil {
				t.Fatalf("NewRelay returned error: %v", err)
			}

			err = relay.ProcessOnce(context.Background())
			if !errors.Is(err, ErrBatchSendResultCountMismatch) {
				t.Fatalf("expected ErrBatchSendResultCountMismatch, got %v", err)
			}

			store.mu.Lock()
			firstRecord := store.records[first.ID]
			secondRecord := store.records[second.ID]
			store.mu.Unlock()

			for _, record := range []Record{firstRecord, secondRecord} {
				if record.Status != StatusSending {
					t.Fatalf("expected sending status, got %q", record.Status)
				}
				if record.ClaimOwner != "batch-relay" {
					t.Fatalf("unexpected claim owner: %q", record.ClaimOwner)
				}
				if record.ClaimUntil == nil || !record.ClaimUntil.Equal(now.Add(time.Minute)) {
					t.Fatalf("unexpected claim until: %v", record.ClaimUntil)
				}
				if record.LastError != "" {
					t.Fatalf("expected empty last_error, got %q", record.LastError)
				}
			}
		})
	}
}
