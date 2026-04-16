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
	"errors"
	"strings"
	"time"

	"github.com/codesjoy/pkg/basic/xevent/outbox/debezium"
	"github.com/codesjoy/pkg/basic/xevent/outbox/internal/shared"
	"gorm.io/gorm"
)

// defaultCutoverBatchSize is the number of records migrated per cutover
// transaction when BatchSize is not set.
const defaultCutoverBatchSize = 128

// GORMStoreConfig configures the GORM-backed Debezium append-only store.
type GORMStoreConfig struct {
	// DB is the GORM database handle used for all operations.
	DB *gorm.DB
	// TableName overrides the default outbox table name when non-empty.
	TableName string
	// SessionFromContext optionally extracts a per-request GORM session from ctx.
	SessionFromContext func(context.Context) *gorm.DB
}

// CutoverConfig configures one relay backlog handoff into cdc rows.
type CutoverConfig struct {
	// DB is the GORM database handle used for the cutover operation.
	DB *gorm.DB
	// TableName overrides the default outbox table name when non-empty.
	TableName string
	// SessionFromContext optionally extracts a per-request GORM session from ctx.
	SessionFromContext func(context.Context) *gorm.DB
	// BatchSize limits how many records are migrated per transaction.
	BatchSize int
	// Now is the reference time for eligibility checks.
	Now time.Time
}

// GORMStore persists Debezium outbox records through the shared store.
type GORMStore struct {
	db        *gorm.DB
	store     *shared.GORMStore
	tableName string
}

var _ debezium.Store = (*GORMStore)(nil)

// NewGORMStore creates a configured GORM-backed Debezium outbox store.
func NewGORMStore(cfg GORMStoreConfig) (*GORMStore, error) {
	if cfg.DB == nil {
		return nil, errors.New("xevent outbox debezium gorm db is nil")
	}

	tableName := strings.TrimSpace(cfg.TableName)
	if tableName == "" {
		tableName = (debezium.Record{}).TableName()
	}

	store, err := shared.NewGORMStore(shared.GORMStoreConfig{
		DB:                 cfg.DB,
		TableName:          tableName,
		SessionFromContext: cfg.SessionFromContext,
	})
	if err != nil {
		return nil, err
	}

	return &GORMStore{
		db:        cfg.DB,
		store:     store,
		tableName: tableName,
	}, nil
}

// Append inserts one Debezium outbox row using the current GORM handle.
func (s *GORMStore) Append(ctx context.Context, record *debezium.Record) error {
	store, err := s.sharedStore()
	if err != nil {
		return err
	}
	return store.AppendDebezium(ctx, record)
}

// DeleteBefore deletes up to limit rows whose created_at is older than cutoff.
// It is intended for retention tasks only and must not be used as part of the
// publish path.
func (s *GORMStore) DeleteBefore(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	store, err := s.sharedStore()
	if err != nil {
		return 0, err
	}
	return store.DeleteDebeziumBefore(ctx, cutoff, limit)
}

// CutoverRelayBacklog inserts cdc rows for eligible relay backlog entries and
// marks the source relay rows as handed off.
func CutoverRelayBacklog(ctx context.Context, cfg CutoverConfig) (int64, error) {
	if cfg.DB == nil {
		return 0, errors.New("xevent outbox debezium gorm db is nil")
	}

	tableName := strings.TrimSpace(cfg.TableName)
	if tableName == "" {
		tableName = (debezium.Record{}).TableName()
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultCutoverBatchSize
	}

	store, err := shared.NewGORMStore(shared.GORMStoreConfig{
		DB:                 cfg.DB,
		TableName:          tableName,
		SessionFromContext: cfg.SessionFromContext,
	})
	if err != nil {
		return 0, err
	}

	return store.CutoverRelayBacklog(ctx, shared.CutoverRequest{
		Now:       cfg.Now,
		BatchSize: cfg.BatchSize,
	})
}

// prepareStoredRecord converts a debezium Record through the shared DBRecord
// layer and back, filling in defaults and normalising timestamps.
func prepareStoredRecord(record debezium.Record, now time.Time) (debezium.Record, error) {
	stored, err := shared.DebeziumRecordToDBRecord(record, now)
	if err != nil {
		return debezium.Record{}, err
	}
	return shared.DBRecordToDebeziumRecord(stored), nil
}

// normalizeContext delegates to shared.NormalizeContext.
func normalizeContext(ctx context.Context) context.Context {
	return shared.NormalizeContext(ctx)
}

// sharedStore lazily initialises and returns the underlying shared.GORMStore.
func (s *GORMStore) sharedStore() (*shared.GORMStore, error) {
	if s.store != nil {
		return s.store, nil
	}
	if s.db == nil {
		return nil, errors.New("xevent outbox debezium gorm db is nil")
	}
	store, err := shared.NewGORMStore(shared.GORMStoreConfig{
		DB:        s.db,
		TableName: s.tableName,
	})
	if err != nil {
		return nil, err
	}
	s.store = store
	return s.store, nil
}
