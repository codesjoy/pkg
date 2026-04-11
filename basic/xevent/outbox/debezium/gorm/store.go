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

	"github.com/codesjoy/pkg/basic/xevent"
	"github.com/codesjoy/pkg/basic/xevent/outbox/debezium"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GORMStoreConfig configures the GORM-backed Debezium append-only store.
type GORMStoreConfig struct {
	DB                 *gorm.DB
	TableName          string
	SessionFromContext func(context.Context) *gorm.DB
}

// GORMStore persists Debezium outbox records through GORM.
type GORMStore struct {
	db                 *gorm.DB
	tableName          string
	sessionFromContext func(context.Context) *gorm.DB
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

	return &GORMStore{
		db:                 cfg.DB,
		tableName:          tableName,
		sessionFromContext: cfg.SessionFromContext,
	}, nil
}

// Append inserts one Debezium outbox row using the current GORM handle.
func (s *GORMStore) Append(ctx context.Context, record *debezium.Record) error {
	if record == nil {
		return errors.New("xevent outbox debezium record is nil")
	}
	ctx = normalizeContext(ctx)

	stored, err := prepareStoredRecord(*record, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := s.session(ctx).Table(s.tableName).Create(&stored).Error; err != nil {
		return err
	}
	*record = cloneRecord(stored)
	return nil
}

// DeleteBefore deletes up to limit rows whose created_at is older than cutoff.
// It is intended for retention tasks only and must not be used as part of the
// publish path.
func (s *GORMStore) DeleteBefore(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	ctx = normalizeContext(ctx)
	if limit <= 0 {
		return 0, errors.New("xevent outbox debezium delete limit must be > 0")
	}

	cutoff = cutoff.UTC()
	var deleted int64
	err := s.session(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []string
		if err := tx.Table(s.tableName).
			Model(&debezium.Record{}).
			Where("created_at < ?", cutoff).
			Order("created_at ASC").
			Order("id ASC").
			Limit(limit).
			Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}

		result := tx.Table(s.tableName).Where("id IN ?", ids).Delete(&debezium.Record{})
		if result.Error != nil {
			return result.Error
		}
		deleted = result.RowsAffected
		return nil
	})
	return deleted, err
}

func prepareStoredRecord(record debezium.Record, now time.Time) (debezium.Record, error) {
	stored := cloneRecord(record)
	if err := validateRecord(stored); err != nil {
		return debezium.Record{}, err
	}
	if strings.TrimSpace(stored.ID) == "" {
		stored.ID = uuid.NewString()
	}
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = now.UTC()
	} else {
		stored.CreatedAt = stored.CreatedAt.UTC()
	}
	return stored, nil
}

func cloneRecord(record debezium.Record) debezium.Record {
	cloned := record
	cloned.Payload = cloneBytes(record.Payload)
	if !record.CreatedAt.IsZero() {
		cloned.CreatedAt = record.CreatedAt.UTC()
	}
	return cloned
}

func cloneBytes(src []byte) []byte {
	if src == nil {
		return nil
	}
	return append([]byte(nil), src...)
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (s *GORMStore) session(ctx context.Context) *gorm.DB {
	ctx = normalizeContext(ctx)
	if s.sessionFromContext != nil {
		if session := s.sessionFromContext(ctx); session != nil {
			return session.WithContext(ctx)
		}
	}
	return s.db.WithContext(ctx)
}

func validateRecord(record debezium.Record) error {
	if strings.TrimSpace(record.EventType) == "" {
		return xevent.ErrEventTypeRequired
	}
	if strings.TrimSpace(record.Topic) == "" {
		return debezium.ErrTopicRequired
	}
	return nil
}
