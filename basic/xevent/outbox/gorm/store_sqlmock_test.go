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
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/codesjoy/pkg/basic/xevent/outbox"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestGORMStoreClaimMySQLUsesDialectSpecificSQL(t *testing.T) {
	db, mock, closeFn := openMySQLMockDB(t)
	defer closeFn()

	store := &GORMStore{
		db:        db,
		tableName: (outbox.Record{}).TableName(),
		dialect:   GORMStoreDialectMySQL,
	}

	now := time.Date(2026, 3, 26, 15, 0, 0, 0, time.UTC)
	claimUntil := now.Add(time.Minute)
	req := outbox.ClaimRequest{
		Owner:    "relay-1",
		Now:      now,
		ClaimTTL: time.Minute,
		Limit:    1,
	}

	mock.ExpectQuery(regexp.QuoteMeta("SELECT records.*") + `[\s\S]*` + regexp.QuoteMeta("FOR UPDATE SKIP LOCKED")).
		WillReturnRows(outboxRows(outbox.Record{
			ID:           7,
			EventType:    "evt",
			EventID:      "evt-7",
			PartitionKey: "p1",
			Payload:      []byte("payload"),
			Status:       outbox.StatusPending,
			AvailableAt:  now,
			CreatedAt:    now,
			UpdatedAt:    now,
		}))
	mock.ExpectExec(`UPDATE .*xevent_outbox_records.*claim_owner.*`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM .*xevent_outbox_records.*claim_owner.*`).
		WillReturnRows(outboxRows(outbox.Record{
			ID:           7,
			EventType:    "evt",
			EventID:      "evt-7",
			PartitionKey: "p1",
			Payload:      []byte("payload"),
			Status:       outbox.StatusSending,
			Attempts:     1,
			ClaimOwner:   "relay-1",
			ClaimUntil:   &claimUntil,
			AvailableAt:  now,
			CreatedAt:    now,
			UpdatedAt:    now,
		}))

	claimed, err := store.claimMySQL(db, req, claimUntil)
	if err != nil {
		t.Fatalf("claimMySQL returned error: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed record, got %d", len(claimed))
	}
	if claimed[0].Status != outbox.StatusSending || claimed[0].ClaimOwner != "relay-1" {
		t.Fatalf("unexpected claimed record: %#v", claimed[0])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestGORMStoreClaimPostgresUsesReturningSQL(t *testing.T) {
	db, mock, closeFn := openPostgresMockDB(t)
	defer closeFn()

	store := &GORMStore{
		db:        db,
		tableName: (outbox.Record{}).TableName(),
		dialect:   GORMStoreDialectPostgres,
	}

	now := time.Date(2026, 3, 26, 15, 30, 0, 0, time.UTC)
	claimUntil := now.Add(2 * time.Minute)
	req := outbox.ClaimRequest{
		Owner:    "relay-2",
		Now:      now,
		ClaimTTL: 2 * time.Minute,
		Limit:    1,
	}

	mock.ExpectQuery(regexp.QuoteMeta("WITH distinct_partitions AS (") + `[\s\S]*` + regexp.QuoteMeta("RETURNING records.*")).
		WillReturnRows(outboxRows(outbox.Record{
			ID:           8,
			EventType:    "evt",
			EventID:      "evt-8",
			PartitionKey: "p2",
			Payload:      []byte("payload"),
			Status:       outbox.StatusSending,
			Attempts:     2,
			ClaimOwner:   "relay-2",
			ClaimUntil:   &claimUntil,
			AvailableAt:  now,
			CreatedAt:    now,
			UpdatedAt:    now,
		}))

	claimed, err := store.claimPostgres(db, req, claimUntil)
	if err != nil {
		t.Fatalf("claimPostgres returned error: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ClaimOwner != "relay-2" {
		t.Fatalf("unexpected claimed records: %#v", claimed)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestGORMStoreSelectClaimCandidatesPropagatesRawError(t *testing.T) {
	db, mock, closeFn := openMySQLMockDB(t)
	defer closeFn()

	store := &GORMStore{db: db, tableName: (outbox.Record{}).TableName()}
	want := errors.New("raw query failed")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT broken")).
		WillReturnError(want)

	_, err := store.selectClaimCandidates(db, "SELECT broken")
	if !errors.Is(err, want) {
		t.Fatalf("expected raw query error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestGORMStoreClaimSelectedRecordsReturnsNilWhenUpdateMisses(t *testing.T) {
	db, mock, closeFn := openMySQLMockDB(t)
	defer closeFn()

	store := &GORMStore{db: db, tableName: (outbox.Record{}).TableName()}
	now := time.Date(2026, 3, 26, 16, 0, 0, 0, time.UTC)
	mock.ExpectExec(`UPDATE .*xevent_outbox_records.*`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	claimed, err := store.claimSelectedRecords(db, []uint64{1}, outbox.ClaimRequest{
		Owner:    "relay-1",
		Now:      now,
		ClaimTTL: time.Minute,
		Limit:    1,
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("claimSelectedRecords returned error: %v", err)
	}
	if claimed != nil {
		t.Fatalf("expected nil claimed records, got %#v", claimed)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func openMySQLMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New returned error: %v", err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("gorm.Open(mysql) returned error: %v", err)
	}
	return db, mock, func() {
		_ = sqlDB.Close()
	}
}

func openPostgresMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New returned error: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("gorm.Open(postgres) returned error: %v", err)
	}
	return db, mock, func() {
		_ = sqlDB.Close()
	}
}

func outboxRows(record outbox.Record) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{
		"id",
		"event_type",
		"event_id",
		"partition_key",
		"payload",
		"available_at",
		"status",
		"attempts",
		"last_error",
		"claim_owner",
		"claim_until",
		"sent_at",
		"created_at",
		"updated_at",
	})

	return rows.AddRow(
		record.ID,
		record.EventType,
		record.EventID,
		record.PartitionKey,
		record.Payload,
		record.AvailableAt,
		record.Status,
		record.Attempts,
		record.LastError,
		record.ClaimOwner,
		record.ClaimUntil,
		record.SentAt,
		record.CreatedAt,
		record.UpdatedAt,
	)
}
