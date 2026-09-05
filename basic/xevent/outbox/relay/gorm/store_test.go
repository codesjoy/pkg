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

package outboxgorm

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/codesjoy/pkg/basic/xevent/outbox/internal/shared"
	outbox "github.com/codesjoy/pkg/basic/xevent/outbox/relay"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type testTxContextKey struct{}

type testEvent struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (*testEvent) EventType() string {
	return "order.created"
}
func (e *testEvent) EventID() string { return e.ID }
func (e *testEvent) PartitionKey() string {
	return e.Key
}
func (*testEvent) Topic() string { return "" }

func (e *testEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(e)
}

func (e *testEvent) UnmarshalPayload(data []byte) error {
	return json.Unmarshal(data, e)
}

func TestAppendEventWithGORMStoreUsesCurrentTransaction(t *testing.T) {
	db := openTestDB(t)
	autoMigrateTestSchema(t, db)
	store, err := NewGORMStore(GORMStoreConfig{
		DB:                 db,
		SessionFromContext: testTransactionFromContext,
	})
	if err != nil {
		t.Fatalf("NewGORMStore returned error: %v", err)
	}

	availableAt := time.Date(2026, 3, 26, 8, 30, 0, 0, time.FixedZone("CST", 8*3600))
	var appended *outbox.Record
	err = db.Transaction(func(tx *gorm.DB) error {
		ctx := testWithTransaction(context.Background(), tx)
		var appendErr error
		appended, appendErr = outbox.AppendEvent(ctx, store, &testEvent{
			ID:    "evt_1",
			Key:   "order-1",
			Value: "alice",
		}, outbox.AppendOptions{AvailableAt: availableAt})
		if appendErr != nil {
			return appendErr
		}

		var count int64
		if err := tx.Model(&outbox.Record{}).
			Where("id = ?", appended.ID).
			Count(&count).
			Error; err != nil {
			return err
		}
		if count != 1 {
			t.Fatalf("expected record visible inside tx, got count=%d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Transaction returned error: %v", err)
	}
	if appended == nil || appended.ID == 0 {
		t.Fatalf("expected appended record id, got %#v", appended)
	}

	var stored outbox.Record
	if err := db.First(&stored, "id = ?", appended.ID).Error; err != nil {
		t.Fatalf("First returned error: %v", err)
	}
	if stored.EventType != "order.created" {
		t.Fatalf("unexpected event type: %q", stored.EventType)
	}
	if stored.EventID != "evt_1" {
		t.Fatalf("unexpected event id: %q", stored.EventID)
	}
	if stored.PartitionKey != "order-1" {
		t.Fatalf("unexpected partition key: %q", stored.PartitionKey)
	}
	if stored.Status != outbox.StatusPending {
		t.Fatalf("unexpected status: %q", stored.Status)
	}
	if !stored.AvailableAt.Equal(availableAt.UTC()) {
		t.Fatalf("unexpected available_at: %v", stored.AvailableAt)
	}
	if len(stored.Payload) == 0 {
		t.Fatal("expected payload")
	}
}

func TestAppendEventWithGORMStoreFallsBackToBaseDB(t *testing.T) {
	db := openTestDB(t)
	autoMigrateTestSchema(t, db)
	store, err := NewGORMStore(GORMStoreConfig{
		DB:                 db,
		SessionFromContext: testTransactionFromContext,
	})
	if err != nil {
		t.Fatalf("NewGORMStore returned error: %v", err)
	}

	record, err := outbox.AppendEvent(context.Background(), store, &testEvent{
		ID:    "evt_base",
		Key:   "base",
		Value: "fallback",
	}, outbox.AppendOptions{})
	if err != nil {
		t.Fatalf("AppendEvent returned error: %v", err)
	}
	if record == nil || record.ID == 0 {
		t.Fatalf("expected record id, got %#v", record)
	}

	var count int64
	if err := db.Model(&outbox.Record{}).
		Where("id = ?", record.ID).
		Count(&count).
		Error; err != nil {
		t.Fatalf("Count returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected fallback write in base db, got count=%d", count)
	}
}

func TestAppendEventWithGORMStoreRollbackUsesContextSession(t *testing.T) {
	db := openTestDB(t)
	autoMigrateTestSchema(t, db)
	store, err := NewGORMStore(GORMStoreConfig{
		DB:                 db,
		SessionFromContext: testTransactionFromContext,
	})
	if err != nil {
		t.Fatalf("NewGORMStore returned error: %v", err)
	}

	rollbackErr := db.Transaction(func(tx *gorm.DB) error {
		ctx := testWithTransaction(context.Background(), tx)
		_, err := outbox.AppendEvent(ctx, store, &testEvent{
			ID:    "evt_rollback",
			Key:   "rollback",
			Value: "rollback",
		}, outbox.AppendOptions{})
		if err != nil {
			return err
		}
		return context.Canceled
	})
	if rollbackErr == nil {
		t.Fatal("expected rollback error")
	}

	var count int64
	if err := db.Model(&outbox.Record{}).
		Where("event_id = ?", "evt_rollback").
		Count(&count).
		Error; err != nil {
		t.Fatalf("Count returned error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected rollback to discard record, got count=%d", count)
	}
}

func TestGORMStoreSessionResolverNilFallsBackToBaseDB(t *testing.T) {
	db := openTestDB(t)
	autoMigrateTestSchema(t, db)
	store, err := NewGORMStore(GORMStoreConfig{
		DB: db,
		SessionFromContext: func(context.Context) *gorm.DB {
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewGORMStore returned error: %v", err)
	}

	record, err := outbox.AppendEvent(context.Background(), store, &testEvent{
		ID:    "evt_nil_session",
		Key:   "nil",
		Value: "resolver",
	}, outbox.AppendOptions{})
	if err != nil {
		t.Fatalf("AppendEvent returned error: %v", err)
	}
	if record == nil || record.ID == 0 {
		t.Fatalf("expected record id, got %#v", record)
	}

	var count int64
	if err := db.Model(&outbox.Record{}).
		Where("id = ?", record.ID).
		Count(&count).
		Error; err != nil {
		t.Fatalf("Count returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected fallback write, got count=%d", count)
	}
}

func TestGORMStoreAppendRejectsNilRecord(t *testing.T) {
	db := openTestDB(t)
	store, err := NewGORMStore(GORMStoreConfig{DB: db})
	if err != nil {
		t.Fatalf("NewGORMStore returned error: %v", err)
	}

	err = store.Append(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "xevent outbox record is nil" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGORMStoreClaimRespectsAvailabilityAndPartitionOrdering(t *testing.T) {
	db := openTestDB(t)
	autoMigrateTestSchema(t, db)

	now := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)
	records := []outbox.Record{
		{
			EventType:    "evt",
			EventID:      "a1",
			PartitionKey: "a",
			Payload:      []byte("a1"),
			Status:       outbox.StatusPending,
			AvailableAt:  now.Add(-time.Minute),
		},
		{
			EventType:    "evt",
			EventID:      "a2",
			PartitionKey: "a",
			Payload:      []byte("a2"),
			Status:       outbox.StatusPending,
			AvailableAt:  now,
		},
		{
			EventType:    "evt",
			EventID:      "b1",
			PartitionKey: "b",
			Payload:      []byte("b1"),
			Status:       outbox.StatusPending,
			AvailableAt:  now.Add(-2 * time.Minute),
		},
		{
			EventType:    "evt",
			EventID:      "future",
			PartitionKey: "c",
			Payload:      []byte("c"),
			Status:       outbox.StatusPending,
			AvailableAt:  now.Add(time.Hour),
		},
	}
	for i := range records {
		insertRelayRecord(t, db, &records[i])
	}

	store, err := NewGORMStore(GORMStoreConfig{DB: db})
	if err != nil {
		t.Fatalf("NewGORMStore returned error: %v", err)
	}

	claimed, err := store.Claim(context.Background(), outbox.ClaimRequest{
		Owner:    "relay-1",
		Now:      now,
		ClaimTTL: time.Minute,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("Claim returned error: %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("expected 2 claimed records, got %d", len(claimed))
	}
	if claimed[0].EventID != "b1" || claimed[1].EventID != "a1" {
		t.Fatalf("unexpected claim order: %#v", claimed)
	}
	for _, record := range claimed {
		if record.Status != outbox.StatusSending {
			t.Fatalf("expected sending status, got %q", record.Status)
		}
		if record.Attempts != 1 {
			t.Fatalf("expected attempts=1, got %d", record.Attempts)
		}
		if record.ClaimOwner != "relay-1" {
			t.Fatalf("unexpected claim owner: %q", record.ClaimOwner)
		}
		if record.ClaimUntil == nil || !record.ClaimUntil.Equal(now.Add(time.Minute)) {
			t.Fatalf("unexpected claim until: %v", record.ClaimUntil)
		}
	}

	if err := store.MarkSent(context.Background(), outbox.MarkSentRequest{
		ID:     claimed[1].ID,
		Owner:  "relay-1",
		Now:    now,
		SentAt: now,
	}); err != nil {
		t.Fatalf("MarkSent returned error: %v", err)
	}

	nextClaimed, err := store.Claim(context.Background(), outbox.ClaimRequest{
		Owner:    "relay-2",
		Now:      now,
		ClaimTTL: time.Minute,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("Claim(second) returned error: %v", err)
	}
	if len(nextClaimed) != 1 || nextClaimed[0].EventID != "a2" {
		t.Fatalf("unexpected second claim: %#v", nextClaimed)
	}
}

func TestGORMStoreClaimReclaimsExpiredSending(t *testing.T) {
	db := openTestDB(t)
	autoMigrateTestSchema(t, db)

	now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	record := outbox.Record{
		EventType:    "evt",
		EventID:      "stale",
		PartitionKey: "same",
		Payload:      []byte("payload"),
		Status:       outbox.StatusSending,
		Attempts:     1,
		ClaimOwner:   "relay-old",
		ClaimUntil:   &expired,
		AvailableAt:  now.Add(-2 * time.Minute),
	}
	insertRelayRecord(t, db, &record)

	store, err := NewGORMStore(GORMStoreConfig{DB: db})
	if err != nil {
		t.Fatalf("NewGORMStore returned error: %v", err)
	}

	claimed, err := store.Claim(context.Background(), outbox.ClaimRequest{
		Owner:    "relay-new",
		Now:      now,
		ClaimTTL: 2 * time.Minute,
		Limit:    1,
	})
	if err != nil {
		t.Fatalf("Claim returned error: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed record, got %d", len(claimed))
	}
	if claimed[0].Attempts != 2 {
		t.Fatalf("expected attempts=2, got %d", claimed[0].Attempts)
	}
	if claimed[0].ClaimOwner != "relay-new" {
		t.Fatalf("unexpected claim owner: %q", claimed[0].ClaimOwner)
	}
}

func TestGORMStoreRetryAndMarkFailed(t *testing.T) {
	db := openTestDB(t)
	autoMigrateTestSchema(t, db)
	store, err := NewGORMStore(GORMStoreConfig{DB: db})
	if err != nil {
		t.Fatalf("NewGORMStore returned error: %v", err)
	}

	now := time.Date(2026, 3, 26, 13, 0, 0, 0, time.UTC)
	claimUntil := now.Add(time.Minute)
	retryRecord := outbox.Record{
		EventType:    "evt",
		EventID:      "retry",
		PartitionKey: "retry",
		Payload:      []byte("retry"),
		Status:       outbox.StatusSending,
		Attempts:     1,
		ClaimOwner:   "relay-1",
		ClaimUntil:   &claimUntil,
		AvailableAt:  now.Add(-time.Minute),
	}
	failedRecord := outbox.Record{
		EventType:    "evt",
		EventID:      "failed",
		PartitionKey: "failed",
		Payload:      []byte("failed"),
		Status:       outbox.StatusSending,
		Attempts:     2,
		ClaimOwner:   "relay-1",
		ClaimUntil:   &claimUntil,
		AvailableAt:  now.Add(-time.Minute),
	}
	insertRelayRecord(t, db, &retryRecord)
	insertRelayRecord(t, db, &failedRecord)

	nextAvailableAt := now.Add(5 * time.Minute)
	if err := store.Retry(context.Background(), outbox.RetryRequest{
		ID:              retryRecord.ID,
		Owner:           "relay-1",
		Now:             now,
		NextAvailableAt: nextAvailableAt,
		LastError:       "temporary",
	}); err != nil {
		t.Fatalf("Retry returned error: %v", err)
	}

	if err := store.MarkFailed(context.Background(), outbox.FailRequest{
		ID:        failedRecord.ID,
		Owner:     "relay-1",
		Now:       now,
		LastError: "permanent",
	}); err != nil {
		t.Fatalf("MarkFailed returned error: %v", err)
	}

	var storedRetry outbox.Record
	if err := db.First(&storedRetry, "id = ?", retryRecord.ID).Error; err != nil {
		t.Fatalf("First retry returned error: %v", err)
	}
	if storedRetry.Status != outbox.StatusPending {
		t.Fatalf("unexpected retry status: %q", storedRetry.Status)
	}
	if storedRetry.LastError != "temporary" {
		t.Fatalf("unexpected retry last_error: %q", storedRetry.LastError)
	}
	if storedRetry.ClaimOwner != "" || storedRetry.ClaimUntil != nil || storedRetry.SentAt != nil {
		t.Fatalf("expected retry claim fields cleared, got %#v", storedRetry)
	}
	if !storedRetry.AvailableAt.Equal(nextAvailableAt) {
		t.Fatalf("unexpected retry available_at: %v", storedRetry.AvailableAt)
	}

	var storedFailed outbox.Record
	if err := db.First(&storedFailed, "id = ?", failedRecord.ID).Error; err != nil {
		t.Fatalf("First failed returned error: %v", err)
	}
	if storedFailed.Status != outbox.StatusFailed {
		t.Fatalf("unexpected failed status: %q", storedFailed.Status)
	}
	if storedFailed.LastError != "permanent" {
		t.Fatalf("unexpected failed last_error: %q", storedFailed.LastError)
	}
	if storedFailed.ClaimOwner != "" || storedFailed.ClaimUntil != nil {
		t.Fatalf("expected failed claim fields cleared, got %#v", storedFailed)
	}
}

func TestGORMStoreTransitionOwnershipErrors(t *testing.T) {
	db := openTestDB(t)
	autoMigrateTestSchema(t, db)
	store, err := NewGORMStore(GORMStoreConfig{DB: db})
	if err != nil {
		t.Fatalf("NewGORMStore returned error: %v", err)
	}

	now := time.Date(2026, 3, 26, 13, 30, 0, 0, time.UTC)
	claimUntil := now.Add(time.Minute)
	record := outbox.Record{
		EventType:    "evt",
		EventID:      "claimed",
		PartitionKey: "p1",
		Payload:      []byte("payload"),
		Status:       outbox.StatusSending,
		ClaimOwner:   "relay-1",
		ClaimUntil:   &claimUntil,
		AvailableAt:  now.Add(-time.Minute),
	}
	insertRelayRecord(t, db, &record)

	tests := []struct {
		name    string
		run     func(uint64) error
		wantErr error
	}{
		{
			name: "mark sent wrong owner",
			run: func(id uint64) error {
				return store.MarkSent(
					context.Background(),
					outbox.MarkSentRequest{ID: id, Owner: "relay-2", Now: now, SentAt: now},
				)
			},
			wantErr: outbox.ErrClaimNotOwned,
		},
		{
			name: "retry wrong owner",
			run: func(id uint64) error {
				return store.Retry(
					context.Background(),
					outbox.RetryRequest{ID: id, Owner: "relay-2", Now: now, NextAvailableAt: now},
				)
			},
			wantErr: outbox.ErrClaimNotOwned,
		},
		{
			name: "mark failed wrong owner",
			run: func(id uint64) error {
				return store.MarkFailed(
					context.Background(),
					outbox.FailRequest{ID: id, Owner: "relay-2", Now: now},
				)
			},
			wantErr: outbox.ErrClaimNotOwned,
		},
		{
			name: "mark sent not found",
			run: func(uint64) error {
				return store.MarkSent(
					context.Background(),
					outbox.MarkSentRequest{ID: 999, Owner: "relay-1", Now: now, SentAt: now},
				)
			},
			wantErr: outbox.ErrRecordNotFound,
		},
		{
			name: "retry not found",
			run: func(uint64) error {
				return store.Retry(
					context.Background(),
					outbox.RetryRequest{ID: 999, Owner: "relay-1", Now: now, NextAvailableAt: now},
				)
			},
			wantErr: outbox.ErrRecordNotFound,
		},
		{
			name: "mark failed not found",
			run: func(uint64) error {
				return store.MarkFailed(
					context.Background(),
					outbox.FailRequest{ID: 999, Owner: "relay-1", Now: now},
				)
			},
			wantErr: outbox.ErrRecordNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run(record.ID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestGORMStoreNormalizeRequestsAndHelpers(t *testing.T) {
	nowLocal := time.Date(2026, 3, 26, 14, 0, 0, 0, time.FixedZone("CST", 8*3600))

	claimReq, err := normalizeClaimRequest(outbox.ClaimRequest{
		Owner:    "relay-1",
		Now:      nowLocal,
		ClaimTTL: time.Minute,
		Limit:    1,
	})
	if err != nil {
		t.Fatalf("normalizeClaimRequest returned error: %v", err)
	}
	if !claimReq.Now.Equal(nowLocal.UTC()) {
		t.Fatalf("unexpected claim now: %v", claimReq.Now)
	}
	if _, err := normalizeClaimRequest(
		outbox.ClaimRequest{Owner: "", Limit: 1, ClaimTTL: time.Minute},
	); err == nil {
		t.Fatal("expected owner validation error")
	}
	if _, err := normalizeClaimRequest(
		outbox.ClaimRequest{Owner: "relay-1", Limit: 0, ClaimTTL: time.Minute},
	); err == nil {
		t.Fatal("expected limit validation error")
	}
	if _, err := normalizeClaimRequest(
		outbox.ClaimRequest{Owner: "relay-1", Limit: 1},
	); err == nil {
		t.Fatal("expected ttl validation error")
	}

	markReq, err := normalizeMarkSentRequest(
		outbox.MarkSentRequest{Owner: "relay-1", Now: nowLocal},
	)
	if err != nil {
		t.Fatalf("normalizeMarkSentRequest returned error: %v", err)
	}
	if !markReq.SentAt.Equal(markReq.Now) {
		t.Fatalf(
			"expected sent_at default to now, got sent_at=%v now=%v",
			markReq.SentAt,
			markReq.Now,
		)
	}
	if _, err := normalizeMarkSentRequest(outbox.MarkSentRequest{}); err == nil {
		t.Fatal("expected mark-sent owner validation error")
	}

	retryReq, err := normalizeRetryRequest(outbox.RetryRequest{Owner: "relay-1", Now: nowLocal})
	if err != nil {
		t.Fatalf("normalizeRetryRequest returned error: %v", err)
	}
	if !retryReq.NextAvailableAt.Equal(retryReq.Now) {
		t.Fatalf(
			"expected next_available_at default to now, got next=%v now=%v",
			retryReq.NextAvailableAt,
			retryReq.Now,
		)
	}
	if _, err := normalizeRetryRequest(outbox.RetryRequest{}); err == nil {
		t.Fatal("expected retry owner validation error")
	}

	failReq, err := normalizeFailRequest(outbox.FailRequest{Owner: "relay-1"})
	if err != nil {
		t.Fatalf("normalizeFailRequest returned error: %v", err)
	}
	if failReq.Now.IsZero() {
		t.Fatal("expected normalized fail request time")
	}
	if _, err := normalizeFailRequest(outbox.FailRequest{}); err == nil {
		t.Fatal("expected fail owner validation error")
	}

	ctx := context.TODO()
	if normalizeContext(ctx) != ctx {
		t.Fatal("expected non-nil context normalization to pass through unchanged")
	}
	if cloneBytes(nil) != nil {
		t.Fatal("expected nil clone bytes")
	}

	payload := []byte("payload")
	clonedBytes := cloneBytes(payload)
	payload[0] = 'P'
	if string(clonedBytes) != "payload" {
		t.Fatalf("expected byte clone copy, got %q", clonedBytes)
	}

	claimUntil := nowLocal.Add(time.Minute)
	sentAt := nowLocal.Add(2 * time.Minute)
	record := outbox.Record{
		ID:           7,
		EventType:    "evt",
		EventID:      "evt-7",
		PartitionKey: "p7",
		Payload:      []byte("payload"),
		Status:       outbox.StatusSending,
		ClaimUntil:   &claimUntil,
		SentAt:       &sentAt,
		AvailableAt:  nowLocal,
		CreatedAt:    nowLocal,
		UpdatedAt:    nowLocal,
	}
	clonedRecord := cloneRecord(record)
	record.Payload[0] = 'P'
	if string(clonedRecord.Payload) != "payload" {
		t.Fatalf("expected record payload clone, got %q", clonedRecord.Payload)
	}
	if clonedRecord.ClaimUntil == nil || !clonedRecord.ClaimUntil.Equal(claimUntil.UTC()) {
		t.Fatalf("unexpected cloned claim_until: %v", clonedRecord.ClaimUntil)
	}
	if clonedRecord.SentAt == nil || !clonedRecord.SentAt.Equal(sentAt.UTC()) {
		t.Fatalf("unexpected cloned sent_at: %v", clonedRecord.SentAt)
	}

	stored := prepareStoredRecord(
		outbox.Record{EventType: "evt", Payload: []byte("payload")},
		nowLocal.UTC(),
	)
	if stored.Status != outbox.StatusPending {
		t.Fatalf("unexpected prepared status: %q", stored.Status)
	}
	if stored.AvailableAt.IsZero() || stored.CreatedAt.IsZero() || stored.UpdatedAt.IsZero() {
		t.Fatalf("expected prepared timestamps, got %#v", stored)
	}

	if got := detectGORMStoreDialect(nil); got != GORMStoreDialectStandard {
		t.Fatalf("unexpected nil-db dialect: %q", got)
	}
	if got := detectGORMStoreDialect(
		&gorm.DB{Config: &gorm.Config{Dialector: stubDialector{name: "mariadb"}}},
	); got != GORMStoreDialectMySQL {
		t.Fatalf("unexpected mariadb dialect: %q", got)
	}
	if got := detectGORMStoreDialect(
		&gorm.DB{Config: &gorm.Config{Dialector: stubDialector{name: "sqlite"}}},
	); got != GORMStoreDialectStandard {
		t.Fatalf("unexpected sqlite dialect: %q", got)
	}
}

func TestGORMStoreClaimSelectedRecordsReturnsNilWhenUpdateMatchesNothing(t *testing.T) {
	db := openTestDB(t)
	autoMigrateTestSchema(t, db)
	store, err := NewGORMStore(GORMStoreConfig{DB: db})
	if err != nil {
		t.Fatalf("NewGORMStore returned error: %v", err)
	}

	now := time.Date(2026, 3, 26, 14, 30, 0, 0, time.UTC)
	record := outbox.Record{
		EventType:    "evt",
		EventID:      "future",
		PartitionKey: "p1",
		Payload:      []byte("payload"),
		Status:       outbox.StatusPending,
		AvailableAt:  now.Add(time.Hour),
	}
	insertRelayRecord(t, db, &record)

	claimed, err := store.claimSelectedRecords(db, []uint64{record.ID}, outbox.ClaimRequest{
		Owner:    "relay-1",
		Now:      now,
		ClaimTTL: time.Minute,
		Limit:    1,
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("claimSelectedRecords returned error: %v", err)
	}
	if claimed != nil {
		t.Fatalf("expected nil claimed records when no rows updated, got %#v", claimed)
	}
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	replacer := strings.NewReplacer("/", "_", " ", "_")
	dsn := "file:" + replacer.Replace(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open returned error: %v", err)
	}
	return db
}

func testWithTransaction(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, testTxContextKey{}, tx)
}

func testTransactionFromContext(ctx context.Context) *gorm.DB {
	tx, _ := ctx.Value(testTxContextKey{}).(*gorm.DB)
	return tx
}

func autoMigrateTestSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.AutoMigrate(&shared.DBRecord{}); err != nil {
		t.Fatalf("AutoMigrate returned error: %v", err)
	}
}

func insertRelayRecord(t *testing.T, db *gorm.DB, record *outbox.Record) {
	t.Helper()
	stored := shared.RelayRecordToDBRecord(*record, time.Now().UTC())
	if err := db.Table(record.TableName()).Create(&stored).Error; err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	*record = shared.DBRecordToRelayRecord(stored)
}
