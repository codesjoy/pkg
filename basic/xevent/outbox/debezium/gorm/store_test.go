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

package debeziumgorm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/codesjoy/pkg/basic/xevent/outbox/debezium"
	"github.com/codesjoy/pkg/basic/xevent/outbox/internal/shared"
	outbox "github.com/codesjoy/pkg/basic/xevent/outbox/relay"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type testTxContextKey struct{}

type testEvent struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	Value     string `json:"value"`
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

func TestNewGORMStoreRejectsNilDB(t *testing.T) {
	store, err := NewGORMStore(GORMStoreConfig{})
	if err == nil {
		t.Fatal("expected error")
	}
	if store != nil {
		t.Fatalf("expected nil store, got %#v", store)
	}
	if err.Error() != "xevent outbox debezium gorm db is nil" {
		t.Fatalf("unexpected error: %v", err)
	}
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

	var appended *debezium.Record
	err = db.Transaction(func(tx *gorm.DB) error {
		ctx := testWithTransaction(context.Background(), tx)
		var appendErr error
		appended, appendErr = debezium.AppendEvent(ctx, store, &testEvent{
			ID:    "evt_1",
			Key:   "order-1",
			Value: "alice",
		}, debezium.AppendOptions{Topic: "orders"})
		if appendErr != nil {
			return appendErr
		}

		var count int64
		if err := tx.Model(&debezium.Record{}).Where("message_id = ?", appended.ID).Count(&count).Error; err != nil {
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
	if appended == nil || appended.ID == "" {
		t.Fatalf("expected appended record id, got %#v", appended)
	}

	var stored debezium.Record
	if err := db.First(&stored, "message_id = ?", appended.ID).Error; err != nil {
		t.Fatalf("First returned error: %v", err)
	}
	if stored.Topic != "orders" {
		t.Fatalf("unexpected topic: %q", stored.Topic)
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
	if stored.CreatedAt.IsZero() {
		t.Fatal("expected created_at")
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

	record, err := debezium.AppendEvent(context.Background(), store, &testEvent{
		ID:        "evt_base",
		Key:       "base",
		Value:     "fallback",
		TopicName: "orders.direct",
	}, debezium.AppendOptions{Topic: "orders.ignored"})
	if err != nil {
		t.Fatalf("AppendEvent returned error: %v", err)
	}
	if record == nil || record.ID == "" {
		t.Fatalf("expected record id, got %#v", record)
	}

	var stored debezium.Record
	if err := db.First(&stored, "message_id = ?", record.ID).Error; err != nil {
		t.Fatalf("First returned error: %v", err)
	}
	if stored.Topic != "orders.direct" {
		t.Fatalf("expected event topic to win, got %q", stored.Topic)
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
		_, err := debezium.AppendEvent(ctx, store, &testEvent{
			ID:    "evt_rollback",
			Key:   "rollback",
			Value: "rollback",
		}, debezium.AppendOptions{Topic: "orders"})
		if err != nil {
			return err
		}
		return context.Canceled
	})
	if rollbackErr == nil {
		t.Fatal("expected rollback error")
	}

	var count int64
	if err := db.Model(&debezium.Record{}).Where("event_id = ?", "evt_rollback").Count(&count).Error; err != nil {
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

	record, err := debezium.AppendEvent(context.Background(), store, &testEvent{
		ID:    "evt_nil_session",
		Key:   "nil",
		Value: "resolver",
	}, debezium.AppendOptions{Topic: "orders"})
	if err != nil {
		t.Fatalf("AppendEvent returned error: %v", err)
	}

	var count int64
	if err := db.Model(&debezium.Record{}).Where("message_id = ?", record.ID).Count(&count).Error; err != nil {
		t.Fatalf("Count returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected fallback write, got count=%d", count)
	}
}

func TestGORMStoreAppendLazilyInitializesSharedStore(t *testing.T) {
	db := openTestDB(t)
	autoMigrateTestSchema(t, db)
	store := &GORMStore{db: db}

	record := debezium.Record{
		Topic:        "orders",
		PartitionKey: "order-1",
		EventType:    "order.created",
		EventID:      "evt_lazy",
		Payload:      []byte("payload"),
	}
	if err := store.Append(context.Background(), &record); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}
	if store.store == nil {
		t.Fatal("expected shared store to be initialized")
	}
	if record.ID == "" {
		t.Fatal("expected generated record id")
	}

	var count int64
	if err := db.Model(&debezium.Record{}).Where("message_id = ?", record.ID).Count(&count).Error; err != nil {
		t.Fatalf("Count returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected inserted row, got count=%d", count)
	}
}

func TestGORMStoreAppendRejectsNilDBDuringLazyInit(t *testing.T) {
	store := &GORMStore{}
	err := store.Append(context.Background(), &debezium.Record{
		Topic:     "orders",
		EventType: "order.created",
		Payload:   []byte("payload"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "xevent outbox debezium gorm db is nil" {
		t.Fatalf("unexpected error: %v", err)
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
	if err.Error() != "xevent outbox debezium record is nil" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGORMStoreAppendValidatesRequiredFields(t *testing.T) {
	db := openTestDB(t)
	store, err := NewGORMStore(GORMStoreConfig{DB: db})
	if err != nil {
		t.Fatalf("NewGORMStore returned error: %v", err)
	}

	if err := store.Append(context.Background(), &debezium.Record{
		Topic:   "orders",
		Payload: []byte("payload"),
	}); err == nil {
		t.Fatal("expected missing event type validation error")
	}

	if err := store.Append(context.Background(), &debezium.Record{
		EventType: "order.created",
		Payload:   []byte("payload"),
	}); err == nil {
		t.Fatal("expected missing topic validation error")
	}
}

func TestGORMStoreDeleteBefore(t *testing.T) {
	db := openTestDB(t)
	autoMigrateTestSchema(t, db)
	store, err := NewGORMStore(GORMStoreConfig{DB: db})
	if err != nil {
		t.Fatalf("NewGORMStore returned error: %v", err)
	}

	now := time.Date(2026, 4, 11, 16, 0, 0, 0, time.UTC)
	records := []debezium.Record{
		{
			ID:           "a",
			Topic:        "orders",
			PartitionKey: "1",
			EventType:    "order.created",
			EventID:      "evt_a",
			Payload:      []byte("a"),
			CreatedAt:    now.Add(-2 * time.Hour),
		},
		{
			ID:           "b",
			Topic:        "orders",
			PartitionKey: "2",
			EventType:    "order.created",
			EventID:      "evt_b",
			Payload:      []byte("b"),
			CreatedAt:    now.Add(-time.Hour),
		},
		{
			ID:           "c",
			Topic:        "orders",
			PartitionKey: "3",
			EventType:    "order.created",
			EventID:      "evt_c",
			Payload:      []byte("c"),
			CreatedAt:    now.Add(time.Hour),
		},
	}
	for i := range records {
		insertDebeziumRecord(t, db, &records[i])
	}

	deleted, err := store.DeleteBefore(context.Background(), now, 1)
	if err != nil {
		t.Fatalf("DeleteBefore returned error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted row, got %d", deleted)
	}

	var count int64
	if err := db.Model(&debezium.Record{}).Where("message_id = ?", "a").Count(&count).Error; err != nil {
		t.Fatalf("Count(a) returned error: %v", err)
	}
	if count != 0 {
		t.Fatal("expected oldest row to be deleted first")
	}

	deleted, err = store.DeleteBefore(context.Background(), now, 10)
	if err != nil {
		t.Fatalf("DeleteBefore(second) returned error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected second delete to remove 1 row, got %d", deleted)
	}

	var remaining []string
	if err := db.Model(&debezium.Record{}).Order("message_id ASC").Pluck("message_id", &remaining).Error; err != nil {
		t.Fatalf("Pluck returned error: %v", err)
	}
	if len(remaining) != 1 || remaining[0] != "c" {
		t.Fatalf("unexpected remaining rows: %#v", remaining)
	}
}

func TestGORMStoreDeleteBeforeRejectsInvalidLimit(t *testing.T) {
	db := openTestDB(t)
	store, err := NewGORMStore(GORMStoreConfig{DB: db})
	if err != nil {
		t.Fatalf("NewGORMStore returned error: %v", err)
	}

	if _, err := store.DeleteBefore(context.Background(), time.Now(), 0); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestCutoverRelayBacklog(t *testing.T) {
	db := openTestDB(t)
	autoMigrateTestSchema(t, db)

	now := time.Date(2026, 4, 11, 20, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	active := now.Add(time.Minute)

	pending := outbox.Record{
		EventType:    "order.created",
		EventID:      "evt_pending",
		PartitionKey: "order-1",
		Payload:      []byte("pending"),
		Topic:        "orders",
		Status:       outbox.StatusPending,
		AvailableAt:  now.Add(-time.Minute),
	}
	expiredSending := outbox.Record{
		EventType:    "order.created",
		EventID:      "evt_expired",
		PartitionKey: "order-2",
		Payload:      []byte("expired"),
		Topic:        "orders",
		Status:       outbox.StatusSending,
		ClaimOwner:   "relay-old",
		ClaimUntil:   &expired,
		AvailableAt:  now.Add(-2 * time.Minute),
	}
	activeSending := outbox.Record{
		EventType:    "order.created",
		EventID:      "evt_active",
		PartitionKey: "order-3",
		Payload:      []byte("active"),
		Topic:        "orders",
		Status:       outbox.StatusSending,
		ClaimOwner:   "relay-live",
		ClaimUntil:   &active,
		AvailableAt:  now.Add(-2 * time.Minute),
	}
	insertRelayRecord(t, db, &pending)
	insertRelayRecord(t, db, &expiredSending)
	insertRelayRecord(t, db, &activeSending)

	moved, err := CutoverRelayBacklog(context.Background(), CutoverConfig{
		DB:        db,
		BatchSize: 10,
		Now:       now,
	})
	if err != nil {
		t.Fatalf("CutoverRelayBacklog returned error: %v", err)
	}
	if moved != 2 {
		t.Fatalf("expected 2 moved rows, got %d", moved)
	}

	var relayRows []shared.DBRecord
	if err := db.Table((shared.DBRecord{}).TableName()).
		Where("mode = ?", shared.ModeRelay).
		Order("id ASC").
		Find(&relayRows).Error; err != nil {
		t.Fatalf("Find relay rows returned error: %v", err)
	}
	if len(relayRows) != 3 {
		t.Fatalf("expected 3 relay rows, got %d", len(relayRows))
	}
	for _, row := range relayRows {
		switch row.EventID {
		case "evt_pending", "evt_expired":
			if row.Status != string(outbox.StatusHandedOff) {
				t.Fatalf("expected handed_off status for %s, got %q", row.EventID, row.Status)
			}
		case "evt_active":
			if row.Status != string(outbox.StatusSending) {
				t.Fatalf("expected active relay row to stay sending, got %q", row.Status)
			}
		}
	}

	var cdcRows []shared.DBRecord
	if err := db.Table((shared.DBRecord{}).TableName()).
		Where("mode = ?", shared.ModeCDC).
		Order("handoff_from_id ASC").
		Find(&cdcRows).Error; err != nil {
		t.Fatalf("Find cdc rows returned error: %v", err)
	}
	if len(cdcRows) != 2 {
		t.Fatalf("expected 2 cdc rows, got %d", len(cdcRows))
	}
	if cdcRows[0].HandoffFromID == nil || *cdcRows[0].HandoffFromID != pending.ID {
		t.Fatalf("unexpected first handoff source: %#v", cdcRows[0].HandoffFromID)
	}
	if cdcRows[1].HandoffFromID == nil || *cdcRows[1].HandoffFromID != expiredSending.ID {
		t.Fatalf("unexpected second handoff source: %#v", cdcRows[1].HandoffFromID)
	}

	moved, err = CutoverRelayBacklog(context.Background(), CutoverConfig{
		DB:        db,
		BatchSize: 10,
		Now:       now,
	})
	if err != nil {
		t.Fatalf("CutoverRelayBacklog(second) returned error: %v", err)
	}
	if moved != 0 {
		t.Fatalf("expected idempotent second cutover, got moved=%d", moved)
	}
}

func TestCutoverRelayBacklogRejectsNilDB(t *testing.T) {
	moved, err := CutoverRelayBacklog(context.Background(), CutoverConfig{})
	if err == nil {
		t.Fatal("expected error")
	}
	if moved != 0 {
		t.Fatalf("expected 0 moved rows, got %d", moved)
	}
	if err.Error() != "xevent outbox debezium gorm db is nil" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCutoverRelayBacklogUsesDefaultBatchSize(t *testing.T) {
	db := openTestDB(t)
	autoMigrateTestSchema(t, db)

	now := time.Date(2026, 4, 11, 22, 0, 0, 0, time.UTC)
	record := outbox.Record{
		EventType:    "order.created",
		EventID:      "evt_default_batch",
		PartitionKey: "order-default",
		Payload:      []byte("payload"),
		Topic:        "orders",
		Status:       outbox.StatusPending,
		AvailableAt:  now.Add(-time.Minute),
	}
	insertRelayRecord(t, db, &record)

	moved, err := CutoverRelayBacklog(context.Background(), CutoverConfig{
		DB:  db,
		Now: now,
	})
	if err != nil {
		t.Fatalf("CutoverRelayBacklog returned error: %v", err)
	}
	if moved != 1 {
		t.Fatalf("expected 1 moved row, got %d", moved)
	}

	var cdcRows []shared.DBRecord
	if err := db.Table((shared.DBRecord{}).TableName()).
		Where("mode = ?", shared.ModeCDC).
		Find(&cdcRows).Error; err != nil {
		t.Fatalf("Find cdc rows returned error: %v", err)
	}
	if len(cdcRows) != 1 {
		t.Fatalf("expected 1 cdc row, got %d", len(cdcRows))
	}
	if cdcRows[0].HandoffFromID == nil || *cdcRows[0].HandoffFromID != record.ID {
		t.Fatalf("unexpected handoff source: %#v", cdcRows[0].HandoffFromID)
	}
}

func TestCutoverRelayBacklogRejectsMissingTopic(t *testing.T) {
	db := openTestDB(t)
	autoMigrateTestSchema(t, db)

	now := time.Date(2026, 4, 11, 21, 0, 0, 0, time.UTC)
	record := outbox.Record{
		EventType:    "order.created",
		EventID:      "evt_missing_topic",
		PartitionKey: "order-9",
		Payload:      []byte("payload"),
		Status:       outbox.StatusPending,
		AvailableAt:  now.Add(-time.Minute),
	}
	insertRelayRecord(t, db, &record)

	if _, err := CutoverRelayBacklog(context.Background(), CutoverConfig{
		DB:        db,
		BatchSize: 10,
		Now:       now,
	}); err == nil {
		t.Fatal("expected missing topic error")
	}
}

func TestPrepareStoredRecordRejectsMissingTopic(t *testing.T) {
	_, err := prepareStoredRecord(debezium.Record{
		EventType: "order.created",
		Payload:   []byte("payload"),
	}, time.Now())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGORMStoreHelpers(t *testing.T) {
	nowLocal := time.Date(2026, 4, 11, 18, 0, 0, 0, time.FixedZone("CST", 8*3600))
	record := debezium.Record{
		Topic:        "orders",
		PartitionKey: "order-1",
		EventType:    "order.created",
		EventID:      "evt_1",
		Payload:      []byte("payload"),
	}

	stored, err := prepareStoredRecord(record, nowLocal)
	if err != nil {
		t.Fatalf("prepareStoredRecord returned error: %v", err)
	}
	if stored.ID == "" {
		t.Fatal("expected generated id")
	}
	if !stored.CreatedAt.Equal(nowLocal.UTC()) {
		t.Fatalf("unexpected created_at: %v", stored.CreatedAt)
	}

	record.Payload[0] = 'P'
	if string(stored.Payload) != "payload" {
		t.Fatalf("expected payload clone, got %q", stored.Payload)
	}

	//nolint:staticcheck // Intentionally pass nil to verify fallback to context.Background().
	if normalizeContext(nil) == nil {
		t.Fatal("expected normalized context")
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

func insertDebeziumRecord(t *testing.T, db *gorm.DB, record *debezium.Record) {
	t.Helper()
	stored, err := shared.DebeziumRecordToDBRecord(*record, time.Now().UTC())
	if err != nil {
		t.Fatalf("DebeziumRecordToDBRecord returned error: %v", err)
	}
	if err := db.Table(record.TableName()).Create(&stored).Error; err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	*record = shared.DBRecordToDebeziumRecord(stored)
}

func insertRelayRecord(t *testing.T, db *gorm.DB, record *outbox.Record) {
	t.Helper()
	stored := shared.RelayRecordToDBRecord(*record, time.Now().UTC())
	if err := db.Table(record.TableName()).Create(&stored).Error; err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	*record = shared.DBRecordToRelayRecord(stored)
}
