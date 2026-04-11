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
	"errors"
	"strings"
	"time"

	"github.com/codesjoy/pkg/basic/xevent"
	"github.com/google/uuid"
)

const defaultTableName = "xevent_debezium_outbox_records"

// ErrTopicRequired indicates the final Kafka topic is missing.
var ErrTopicRequired = errors.New("xevent outbox debezium topic is required")

// Record is the append-only outbox row consumed by Debezium.
type Record struct {
	ID string `gorm:"primaryKey;size:36"`

	Topic        string `gorm:"size:255;not null;index"`
	PartitionKey string `gorm:"size:255;not null;default:''"`
	EventType    string `gorm:"size:255;not null"`
	EventID      string `gorm:"size:255;not null;default:''"`
	Payload      []byte `gorm:"not null"`

	CreatedAt time.Time `gorm:"not null;index"`
}

// TableName returns the default Debezium outbox table name.
func (Record) TableName() string {
	return defaultTableName
}

// Store persists insert-only Debezium outbox rows.
type Store interface {
	Append(context.Context, *Record) error
}

// AppendOptions configures Debezium outbox appends.
type AppendOptions struct {
	Topic string
}

// AppendEvent encodes one xevent.Event and appends it into a Debezium outbox
// table. The final topic must be known before the row is inserted.
func AppendEvent(
	ctx context.Context,
	store Store,
	event xevent.Event,
	opts AppendOptions,
) (*Record, error) {
	if store == nil {
		return nil, errors.New("xevent outbox debezium store is nil")
	}

	outbound, err := xevent.Encode(event)
	if err != nil {
		return nil, err
	}

	record, err := NewRecord(outbound, opts)
	if err != nil {
		return nil, err
	}

	if err := store.Append(ctx, record); err != nil {
		return nil, err
	}
	return record, nil
}

// NewRecord converts one outbound payload into an append-only Debezium row.
func NewRecord(outbound *xevent.Outbound, opts AppendOptions) (*Record, error) {
	if outbound == nil {
		return nil, xevent.ErrNilOutbound
	}
	if outbound.EventType == "" {
		return nil, xevent.ErrEventTypeRequired
	}

	topic := strings.TrimSpace(outbound.Topic)
	if topic == "" {
		topic = strings.TrimSpace(opts.Topic)
	}
	if topic == "" {
		return nil, ErrTopicRequired
	}

	return &Record{
		ID:           uuid.NewString(),
		Topic:        topic,
		PartitionKey: outbound.PartitionKey,
		EventType:    outbound.EventType,
		EventID:      outbound.EventID,
		Payload:      cloneBytes(outbound.Payload),
	}, nil
}

func prepareStoredRecord(record Record, now time.Time) Record {
	stored := cloneRecord(record)
	if strings.TrimSpace(stored.ID) == "" {
		stored.ID = uuid.NewString()
	}
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = now.UTC()
	} else {
		stored.CreatedAt = stored.CreatedAt.UTC()
	}
	return stored
}

func cloneRecord(record Record) Record {
	cloned := record
	cloned.Payload = cloneBytes(record.Payload)
	if !record.CreatedAt.IsZero() {
		cloned.CreatedAt = record.CreatedAt.UTC()
	}
	return cloned
}

func validateRecord(record Record) error {
	if strings.TrimSpace(record.EventType) == "" {
		return xevent.ErrEventTypeRequired
	}
	if strings.TrimSpace(record.Topic) == "" {
		return ErrTopicRequired
	}
	return nil
}

func cloneBytes(src []byte) []byte {
	if src == nil {
		return nil
	}
	return append([]byte(nil), src...)
}
