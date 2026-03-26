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
	ID           uint64     `gorm:"primaryKey;autoIncrement"`
	EventType    string     `gorm:"size:255;not null;index"`
	EventID      string     `gorm:"size:255;index"`
	PartitionKey string     `gorm:"size:255;not null;default:'';index;index:idx_xevent_outbox_status_available_partition_id,priority:3;index:idx_xevent_outbox_status_claim_until_available_partition_id,priority:4"`
	Payload      []byte     `gorm:"not null"`
	AvailableAt  time.Time  `gorm:"not null;index;index:idx_xevent_outbox_status_available_partition_id,priority:2;index:idx_xevent_outbox_status_claim_until_available_partition_id,priority:3"`
	Status       Status     `gorm:"type:varchar(16);not null;index;index:idx_xevent_outbox_status_available_partition_id,priority:1;index:idx_xevent_outbox_status_claim_until_available_partition_id,priority:1"`
	Attempts     int        `gorm:"not null;default:0"`
	LastError    string     `gorm:"type:text"`
	ClaimOwner   string     `gorm:"size:255;index"`
	ClaimUntil   *time.Time `gorm:"index;index:idx_xevent_outbox_status_claim_until_available_partition_id,priority:2"`
	SentAt       *time.Time `gorm:"index"`
	CreatedAt    time.Time  `gorm:"not null"`
	UpdatedAt    time.Time  `gorm:"not null"`
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
func AppendEvent(
	ctx context.Context,
	store Store,
	event xevent.Event,
	opts AppendOptions,
) (*Record, error) {
	if store == nil {
		return nil, errors.New("xevent outbox store is nil")
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
		Status:       StatusPending,
	}
	if !opts.AvailableAt.IsZero() {
		record.AvailableAt = opts.AvailableAt.UTC()
	}

	return record, nil
}

func prepareStoredRecord(record Record, now time.Time) Record {
	stored := cloneRecord(record)
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
