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
	"time"

	"github.com/codesjoy/pkg/basic/xevent"
)

const defaultTableName = "xevent_outbox_records"

var (
	// ErrRecordNotFound indicates the target record does not exist.
	ErrRecordNotFound = errors.New("xevent outbox record not found")
	// ErrClaimNotOwned indicates the target record is not owned by the caller.
	ErrClaimNotOwned = errors.New("xevent outbox claim is not owned")
)

// Status represents one outbox record lifecycle state.
type Status string

const (
	// StatusPending indicates the record is ready to be claimed.
	StatusPending Status = "pending"
	// StatusSending indicates the record is currently claimed for sending.
	StatusSending Status = "sending"
	// StatusSent indicates the record has been published successfully.
	StatusSent Status = "sent"
	// StatusFailed indicates the record exhausted retries.
	StatusFailed Status = "failed"
)

// Record is the persisted outbox payload.
type Record struct {
	// Event metadata: identifies and classifies the event.
	ID           uint64 `gorm:"primaryKey;autoIncrement;index:idx_xevent_outbox_status_partition_available_id,priority:4"`
	EventType    string `gorm:"size:255;not null"`
	EventID      string `gorm:"size:255"`
	PartitionKey string `gorm:"size:255;not null;default:'';index:idx_xevent_outbox_status_partition_available_id,priority:2"`

	// Payload: the serialised event body and target topic.
	Payload []byte `gorm:"not null"`
	Topic   string `gorm:"size:255;not null;default:''"`

	// Timing: controls when the record becomes eligible for processing.
	AvailableAt time.Time `gorm:"not null;index:idx_xevent_outbox_status_partition_available_id,priority:3"`

	// Status and retry tracking.
	Status    Status `gorm:"type:varchar(16);not null;index:idx_xevent_outbox_status_partition_available_id,priority:1"`
	Attempts  int    `gorm:"not null;default:0"`
	LastError string `gorm:"type:text"`

	// Claim tracking: which owner is currently processing this record and until when.
	ClaimOwner string `gorm:"size:255"`
	ClaimUntil *time.Time

	// Timestamps: lifecycle milestones.
	SentAt    *time.Time
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

// TableName returns the default outbox table name.
func (Record) TableName() string {
	return defaultTableName
}

// Store persists and advances outbox records.
type Store interface {
	Append(context.Context, *Record) error
	Claim(context.Context, ClaimRequest) ([]Record, error)
	MarkSent(context.Context, MarkSentRequest) error
	Retry(context.Context, RetryRequest) error
	MarkFailed(context.Context, FailRequest) error
}

// ClaimRequest controls one claim batch.
type ClaimRequest struct {
	Owner    string
	Now      time.Time
	ClaimTTL time.Duration
	Limit    int
}

// MarkSentRequest finalizes one successful send.
type MarkSentRequest struct {
	ID     uint64
	Owner  string
	Now    time.Time
	SentAt time.Time
}

// RetryRequest reschedules one failed send.
type RetryRequest struct {
	ID              uint64
	Owner           string
	Now             time.Time
	NextAvailableAt time.Time
	LastError       string
}

// FailRequest marks one exhausted record as failed.
type FailRequest struct {
	ID        uint64
	Owner     string
	Now       time.Time
	LastError string
}

// AppendOptions configures appended outbox records.
type AppendOptions struct {
	AvailableAt time.Time
}

// AppendEvent encodes one xevent.Event and appends it into the store.
//
// Steps: (1) encode the event to an Outbound, (2) wrap it in a pending
// Record, (3) persist via the Store.
func AppendEvent(
	ctx context.Context,
	store Store,
	event xevent.Event,
	opts AppendOptions,
) (*Record, error) {
	if store == nil {
		return nil, errors.New("xevent outbox store is nil")
	}

	// Step 1: encode event to outbound.
	outbound, err := xevent.Encode(event)
	if err != nil {
		return nil, err
	}

	// Step 2: build a pending record from the outbound.
	record, err := NewRecord(outbound, opts)
	if err != nil {
		return nil, err
	}

	// Step 3: persist the record.
	if err := store.Append(ctx, record); err != nil {
		return nil, err
	}
	return record, nil
}

// NewRecord converts one outbound payload into a pending outbox record.
func NewRecord(outbound *xevent.Outbound, opts AppendOptions) (*Record, error) {
	if outbound == nil {
		return nil, xevent.ErrNilOutbound
	}
	if outbound.EventType == "" {
		return nil, xevent.ErrEventTypeRequired
	}

	record := &Record{
		EventType:    outbound.EventType,
		EventID:      outbound.EventID,
		PartitionKey: outbound.PartitionKey,
		Payload:      cloneBytes(outbound.Payload),
		Topic:        outbound.Topic,
		Status:       StatusPending,
	}
	// Default AvailableAt to the zero time (i.e. immediately eligible);
	// store implementations will fill in time.Now() if still zero at persist.
	if !opts.AvailableAt.IsZero() {
		record.AvailableAt = opts.AvailableAt.UTC()
	}

	return record, nil
}

// prepareStoredRecord deep-copies the record and fills in zero-valued fields
// (status, available_at, created_at, updated_at) with sensible defaults.
func prepareStoredRecord(record Record, now time.Time) Record {
	stored := cloneRecord(record)
	// Default any unset fields so the persisted record is always complete.
	if stored.Status == "" {
		stored.Status = StatusPending
	}
	if stored.AvailableAt.IsZero() {
		stored.AvailableAt = now
	}
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = now
	}
	if stored.UpdatedAt.IsZero() {
		stored.UpdatedAt = now
	}
	return stored
}

// cloneRecord returns a deep copy of the record. Time fields are normalized
// to UTC so that comparisons and storage are timezone-consistent.
func cloneRecord(record Record) Record {
	cloned := record
	cloned.Payload = cloneBytes(record.Payload)
	if record.ClaimUntil != nil {
		value := record.ClaimUntil.UTC()
		cloned.ClaimUntil = &value
	}
	if record.SentAt != nil {
		value := record.SentAt.UTC()
		cloned.SentAt = &value
	}
	cloned.AvailableAt = record.AvailableAt.UTC()
	cloned.CreatedAt = record.CreatedAt.UTC()
	cloned.UpdatedAt = record.UpdatedAt.UTC()
	return cloned
}

func cloneBytes(src []byte) []byte {
	if src == nil {
		return nil
	}
	return append([]byte(nil), src...)
}

// normalizeClaimRequest validates required fields and defaults Now to the
// current time when zero.
func normalizeClaimRequest(req ClaimRequest) (ClaimRequest, error) {
	if req.Owner == "" {
		return ClaimRequest{}, errors.New("xevent outbox claim owner is required")
	}
	if req.Limit <= 0 {
		return ClaimRequest{}, errors.New("xevent outbox claim limit must be > 0")
	}
	if req.ClaimTTL <= 0 {
		return ClaimRequest{}, errors.New("xevent outbox claim ttl must be > 0")
	}
	req.Now = normalizeTime(req.Now, time.Now)
	return req, nil
}

// normalizeMarkSentRequest validates owner, defaults Now and SentAt when zero,
// and normalises SentAt to UTC.
func normalizeMarkSentRequest(req MarkSentRequest) (MarkSentRequest, error) {
	if req.Owner == "" {
		return MarkSentRequest{}, errors.New("xevent outbox claim owner is required")
	}
	req.Now = normalizeTime(req.Now, time.Now)
	if req.SentAt.IsZero() {
		req.SentAt = req.Now
	} else {
		req.SentAt = req.SentAt.UTC()
	}
	return req, nil
}

// normalizeRetryRequest validates owner, defaults Now and NextAvailableAt when
// zero, and normalises times to UTC.
func normalizeRetryRequest(req RetryRequest) (RetryRequest, error) {
	if req.Owner == "" {
		return RetryRequest{}, errors.New("xevent outbox claim owner is required")
	}
	req.Now = normalizeTime(req.Now, time.Now)
	if req.NextAvailableAt.IsZero() {
		req.NextAvailableAt = req.Now
	} else {
		req.NextAvailableAt = req.NextAvailableAt.UTC()
	}
	return req, nil
}

// normalizeFailRequest validates owner and defaults Now to the current time.
func normalizeFailRequest(req FailRequest) (FailRequest, error) {
	if req.Owner == "" {
		return FailRequest{}, errors.New("xevent outbox claim owner is required")
	}
	req.Now = normalizeTime(req.Now, time.Now)
	return req, nil
}

func normalizeTime(value time.Time, fallback func() time.Time) time.Time {
	if value.IsZero() {
		return fallback().UTC()
	}
	return value.UTC()
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
