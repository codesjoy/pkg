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

package shared

import (
	"context"
	"strings"
	"time"

	"github.com/codesjoy/pkg/basic/xevent"
	"github.com/codesjoy/pkg/basic/xevent/outbox/debezium"
	outbox "github.com/codesjoy/pkg/basic/xevent/outbox/relay"
	"github.com/google/uuid"
)

const (
	// DefaultTableName is the shared physical outbox table used by relay and cdc.
	DefaultTableName = "xevent_outbox_records"

	relayStatusHandedOff = "handed_off"
)

// Mode identifies which publish strategy owns one persisted row.
type Mode string

const (
	// ModeRelay indicates the row is managed by the relay publish path.
	ModeRelay Mode = "relay"
	// ModeCDC indicates the row is managed by the CDC (Debezium) publish path.
	ModeCDC Mode = "cdc"
)

// DBRecord is the shared physical outbox row.
type DBRecord struct {
	// Identity: auto-incremented primary key and logical identifiers.
	ID            uint64  `gorm:"primaryKey;autoIncrement;index:idx_xevent_outbox_mode_status_partition_available_id,priority:5"`
	MessageID     string  `gorm:"column:message_id;size:36;not null;default:''"`
	Mode          Mode    `gorm:"type:varchar(16);not null;index:idx_xevent_outbox_mode_status_partition_available_id,priority:1;index:idx_xevent_outbox_mode_created_at,priority:1"`
	HandoffFromID *uint64 `gorm:"uniqueIndex:idx_xevent_outbox_handoff_from_id"`

	// Event metadata: describes the domain event carried by this row.
	EventType    string `gorm:"size:255;not null"`
	EventID      string `gorm:"size:255;not null;default:''"`
	PartitionKey string `gorm:"size:255;not null;default:'';index:idx_xevent_outbox_mode_status_partition_available_id,priority:3"`
	Payload      []byte `gorm:"not null"`
	Topic        string `gorm:"size:255;not null;default:''"`

	// Timing: controls when the record becomes eligible for processing.
	AvailableAt time.Time `gorm:"not null;index:idx_xevent_outbox_mode_status_partition_available_id,priority:4"`

	// Status and retry tracking: tracks the lifecycle and failure state.
	Status    string `gorm:"type:varchar(16);not null;default:'';index:idx_xevent_outbox_mode_status_partition_available_id,priority:2"`
	Attempts  int    `gorm:"not null;default:0"`
	LastError string `gorm:"type:text"`

	// Claim tracking: which owner is currently processing this record and until when.
	ClaimOwner string `gorm:"size:255;not null;default:''"`
	ClaimUntil *time.Time

	// Timestamps: lifecycle milestones.
	SentAt    *time.Time
	CreatedAt time.Time `gorm:"not null;index:idx_xevent_outbox_mode_created_at,priority:2"`
	UpdatedAt time.Time `gorm:"not null"`
}

// TableName returns the default shared table name.
func (DBRecord) TableName() string {
	return DefaultTableName
}

// CloneDBRecord returns a deep copy of record with all slice and pointer
// fields duplicated so that mutations to the original do not affect the clone.
func CloneDBRecord(record DBRecord) DBRecord {
	cloned := record
	cloned.Payload = cloneBytes(record.Payload)
	if record.HandoffFromID != nil {
		value := *record.HandoffFromID
		cloned.HandoffFromID = &value
	}
	if record.ClaimUntil != nil {
		value := record.ClaimUntil.UTC()
		cloned.ClaimUntil = &value
	}
	if record.SentAt != nil {
		value := record.SentAt.UTC()
		cloned.SentAt = &value
	}
	if !record.AvailableAt.IsZero() {
		cloned.AvailableAt = record.AvailableAt.UTC()
	}
	if !record.CreatedAt.IsZero() {
		cloned.CreatedAt = record.CreatedAt.UTC()
	}
	if !record.UpdatedAt.IsZero() {
		cloned.UpdatedAt = record.UpdatedAt.UTC()
	}
	return cloned
}

// RelayRecordToDBRecord converts a relay Record into a DBRecord, applying
// defaults for MessageID, Mode, Status, and normalising all timestamps to UTC.
func RelayRecordToDBRecord(record outbox.Record, now time.Time) DBRecord {
	stored := CloneDBRecord(DBRecord{
		ID:           record.ID,
		Mode:         ModeRelay,
		EventType:    record.EventType,
		EventID:      record.EventID,
		PartitionKey: record.PartitionKey,
		Payload:      cloneBytes(record.Payload),
		Topic:        record.Topic,
		AvailableAt:  record.AvailableAt,
		Status:       string(record.Status),
		Attempts:     record.Attempts,
		LastError:    record.LastError,
		ClaimOwner:   record.ClaimOwner,
		ClaimUntil:   cloneTimePtr(record.ClaimUntil),
		SentAt:       cloneTimePtr(record.SentAt),
		CreatedAt:    record.CreatedAt,
		UpdatedAt:    record.UpdatedAt,
	})
	if strings.TrimSpace(stored.MessageID) == "" {
		stored.MessageID = uuid.NewString()
	}
	if stored.Mode == "" {
		stored.Mode = ModeRelay
	}
	if stored.Status == "" {
		stored.Status = string(outbox.StatusPending)
	}
	if stored.AvailableAt.IsZero() {
		stored.AvailableAt = now.UTC()
	} else {
		stored.AvailableAt = stored.AvailableAt.UTC()
	}
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = now.UTC()
	} else {
		stored.CreatedAt = stored.CreatedAt.UTC()
	}
	if stored.UpdatedAt.IsZero() {
		stored.UpdatedAt = now.UTC()
	} else {
		stored.UpdatedAt = stored.UpdatedAt.UTC()
	}
	if stored.ClaimOwner == "" {
		stored.ClaimOwner = ""
	}
	return stored
}

// DBRecordToRelayRecord converts a DBRecord back to a relay Record.
func DBRecordToRelayRecord(record DBRecord) outbox.Record {
	return outbox.Record{
		ID:           record.ID,
		EventType:    record.EventType,
		EventID:      record.EventID,
		PartitionKey: record.PartitionKey,
		Payload:      cloneBytes(record.Payload),
		Topic:        record.Topic,
		AvailableAt:  record.AvailableAt.UTC(),
		Status:       outbox.Status(record.Status),
		Attempts:     record.Attempts,
		LastError:    record.LastError,
		ClaimOwner:   record.ClaimOwner,
		ClaimUntil:   cloneTimePtr(record.ClaimUntil),
		SentAt:       cloneTimePtr(record.SentAt),
		CreatedAt:    record.CreatedAt.UTC(),
		UpdatedAt:    record.UpdatedAt.UTC(),
	}
}

// DebeziumRecordToDBRecord converts a debezium Record into a DBRecord with
// validation. It returns an error if EventType or Topic is empty.
func DebeziumRecordToDBRecord(record debezium.Record, now time.Time) (DBRecord, error) {
	if strings.TrimSpace(record.EventType) == "" {
		return DBRecord{}, xevent.ErrEventTypeRequired
	}
	if strings.TrimSpace(record.Topic) == "" {
		return DBRecord{}, debezium.ErrTopicRequired
	}

	stored := CloneDBRecord(DBRecord{
		MessageID:    strings.TrimSpace(record.ID),
		Mode:         ModeCDC,
		EventType:    record.EventType,
		EventID:      record.EventID,
		PartitionKey: record.PartitionKey,
		Payload:      cloneBytes(record.Payload),
		Topic:        record.Topic,
		CreatedAt:    record.CreatedAt,
	})
	if stored.MessageID == "" {
		stored.MessageID = uuid.NewString()
	}
	if stored.Mode == "" {
		stored.Mode = ModeCDC
	}
	if stored.AvailableAt.IsZero() {
		stored.AvailableAt = now.UTC()
	} else {
		stored.AvailableAt = stored.AvailableAt.UTC()
	}
	stored.Status = ""
	stored.ClaimOwner = ""
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = now.UTC()
	} else {
		stored.CreatedAt = stored.CreatedAt.UTC()
	}
	if stored.UpdatedAt.IsZero() {
		stored.UpdatedAt = stored.CreatedAt
	} else {
		stored.UpdatedAt = stored.UpdatedAt.UTC()
	}
	return stored, nil
}

// DBRecordToDebeziumRecord converts a DBRecord back to a debezium Record.
func DBRecordToDebeziumRecord(record DBRecord) debezium.Record {
	return debezium.Record{
		ID:           record.MessageID,
		Topic:        record.Topic,
		PartitionKey: record.PartitionKey,
		EventType:    record.EventType,
		EventID:      record.EventID,
		Payload:      cloneBytes(record.Payload),
		CreatedAt:    record.CreatedAt.UTC(),
	}
}

// PrepareCutoverDebeziumRecord builds a CDC DBRecord from a relay DBRecord for
// handoff during cutover. The resulting record references the source relay row
// via HandoffFromID.
func PrepareCutoverDebeziumRecord(record DBRecord, now time.Time) (DBRecord, error) {
	if strings.TrimSpace(record.Topic) == "" {
		return DBRecord{}, debezium.ErrTopicRequired
	}

	messageID := strings.TrimSpace(record.MessageID)
	if messageID == "" {
		messageID = uuid.NewString()
	}

	sourceID := record.ID
	return CloneDBRecord(DBRecord{
		MessageID:     messageID,
		Mode:          ModeCDC,
		HandoffFromID: &sourceID,
		EventType:     record.EventType,
		EventID:       record.EventID,
		PartitionKey:  record.PartitionKey,
		Payload:       cloneBytes(record.Payload),
		Topic:         record.Topic,
		AvailableAt:   now.UTC(),
		Status:        "",
		ClaimOwner:    "",
		CreatedAt:     now.UTC(),
		UpdatedAt:     now.UTC(),
	}), nil
}

// NormalizeContext returns context.Background when ctx is nil, otherwise
// returns ctx unchanged.
func NormalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// NormalizeTime returns fallback when value is zero, otherwise returns value
// normalised to UTC.
func NormalizeTime(value time.Time, fallback func() time.Time) time.Time {
	if value.IsZero() {
		return fallback().UTC()
	}
	return value.UTC()
}

// IsRelayCutoverEligible reports whether a relay record is eligible for
// cutover to the CDC path at the given time.
func IsRelayCutoverEligible(record DBRecord, now time.Time) bool {
	if record.Mode != ModeRelay {
		return false
	}
	switch record.Status {
	case string(outbox.StatusPending):
		return !record.AvailableAt.After(now)
	case string(outbox.StatusSending):
		if record.AvailableAt.After(now) {
			return false
		}
		if record.ClaimUntil == nil {
			return true
		}
		return !record.ClaimUntil.After(now)
	default:
		return false
	}
}

// cloneBytes returns a shallow-independent copy of src.
func cloneBytes(src []byte) []byte {
	if src == nil {
		return nil
	}
	return append([]byte(nil), src...)
}

// cloneTimePtr returns a UTC-normalised copy of the pointed-to time, or nil.
func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
