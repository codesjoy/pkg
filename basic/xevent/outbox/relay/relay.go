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
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/codesjoy/pkg/basic/xevent"
	"github.com/google/uuid"
)

const (
	defaultPollInterval = time.Second
	defaultBatchSize    = 128
	defaultClaimTTL     = 30 * time.Second
	defaultRetryDelay   = time.Second
	defaultMaxAttempts  = 3
)

// ErrBatchSendResultCountMismatch indicates a BatchSender returned a result
// vector whose length does not match the requested batch size.
var ErrBatchSendResultCountMismatch = errors.New(
	"xevent outbox batch sender returned mismatched result count",
)

// RelayConfig configures one local outbox relay loop.
type RelayConfig struct {
	Store        Store
	Sender       xevent.Sender
	Owner        string
	PollInterval time.Duration
	BatchSize    int
	ClaimTTL     time.Duration
	RetryDelay   time.Duration
	MaxAttempts  int
	Now          func() time.Time
	Logger       *slog.Logger
}

// Relay polls, claims, and sends outbox records.
type Relay struct {
	store        Store
	sender       xevent.Sender
	pollInterval time.Duration
	batchSize    int
	claimTTL     time.Duration
	retryDelay   time.Duration
	maxAttempts  int
	now          func() time.Time
	logger       *slog.Logger
	owner        string
	wakeCh       chan struct{}

	processMu sync.Mutex
}

// NewRelay creates a configured outbox relay.
// Zero-valued or nil optional fields are filled with sensible defaults.
func NewRelay(cfg RelayConfig) (*Relay, error) {
	if cfg.Store == nil {
		return nil, errors.New("xevent outbox store is nil")
	}
	if cfg.Sender == nil {
		return nil, errors.New("xevent outbox sender is nil")
	}
	// Default optional numeric / duration fields.
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultBatchSize
	}
	if cfg.ClaimTTL <= 0 {
		cfg.ClaimTTL = defaultClaimTTL
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = defaultRetryDelay
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = defaultMaxAttempts
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	// Generate a unique owner ID so multiple relay instances do not clash.
	if cfg.Owner == "" {
		cfg.Owner = uuid.NewString()
	}

	return &Relay{
		store:        cfg.Store,
		sender:       cfg.Sender,
		pollInterval: cfg.PollInterval,
		batchSize:    cfg.BatchSize,
		claimTTL:     cfg.ClaimTTL,
		retryDelay:   cfg.RetryDelay,
		maxAttempts:  cfg.MaxAttempts,
		now:          cfg.Now,
		logger:       cfg.Logger,
		owner:        cfg.Owner,
		wakeCh:       make(chan struct{}, 1),
	}, nil
}

// Run starts the polling relay loop until ctx is canceled or a store error occurs.
//
// The loop structure is: process immediately once, then wait on a ticker or
// an explicit wake signal before processing again.
func (r *Relay) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	// Process immediately on start rather than waiting for the first tick.
	if err := r.ProcessOnce(ctx); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		case <-r.wakeCh:
		}

		if err := r.ProcessOnce(ctx); err != nil {
			return err
		}
	}
}

// Wake requests one extra relay scan without blocking. If a wake signal is
// already pending the new one is discarded — the relay will process on the
// next cycle regardless.
func (r *Relay) Wake() {
	select {
	case r.wakeCh <- struct{}{}:
	default:
	}
}

// ProcessOnce drains all currently claimable outbox records. It repeatedly
// claims batches until no more eligible records remain. The mutex prevents
// concurrent ProcessOnce calls (e.g. from Run + manual invocation).
func (r *Relay) ProcessOnce(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	r.processMu.Lock()
	defer r.processMu.Unlock()

	// Drain loop: keep claiming batches until the store returns nothing.
	for {
		now := r.now().UTC()
		claimed, err := r.store.Claim(ctx, ClaimRequest{
			Owner:    r.owner,
			Now:      now,
			ClaimTTL: r.claimTTL,
			Limit:    r.batchSize,
		})
		if err != nil {
			return err
		}
		if len(claimed) == 0 {
			return nil
		}

		if err := r.processBatch(ctx, claimed); err != nil {
			return err
		}
	}
}

// processBatch sends records using BatchSender when available, falling back
// to the goroutine-per-record approach otherwise.
func (r *Relay) processBatch(ctx context.Context, records []Record) error {
	if bs, ok := r.sender.(xevent.BatchSender); ok {
		return r.processBatchBulk(ctx, records, bs)
	}
	return r.processBatchIndividual(ctx, records)
}

// processBatchBulk sends all records through BatchSender in one call.
func (r *Relay) processBatchBulk(
	ctx context.Context,
	records []Record,
	bs xevent.BatchSender,
) error {
	outbounds := make([]*xevent.Outbound, len(records))
	for i, record := range records {
		outbounds[i] = record.outbound()
	}

	errs := bs.BatchSend(ctx, outbounds)
	if len(errs) != len(records) {
		return fmt.Errorf(
			"%w: expected %d, got %d",
			ErrBatchSendResultCountMismatch,
			len(records),
			len(errs),
		)
	}

	transitionCtx := relayTransitionContext(ctx)
	now := r.now().UTC()

	var joinErrs []error
	for i, err := range errs {
		if err == nil {
			if markErr := r.store.MarkSent(transitionCtx, MarkSentRequest{
				ID:     records[i].ID,
				Owner:  r.owner,
				Now:    now,
				SentAt: now,
			}); markErr != nil {
				joinErrs = append(joinErrs, markErr)
			}
			continue
		}
		if handleErr := r.handleSendError(transitionCtx, records[i], now, err); handleErr != nil {
			joinErrs = append(joinErrs, handleErr)
		}
	}
	return errors.Join(joinErrs...)
}

// processBatchIndividual sends each record concurrently using a goroutine-per-record
// pattern. A WaitGroup coordinates completion; a mutex guards the shared
// error slice.
func (r *Relay) processBatchIndividual(ctx context.Context, records []Record) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	errs := make([]error, 0, len(records))

	for _, record := range records {
		record := record
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.processRecord(ctx, record); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	return errors.Join(errs...)
}

// processRecord sends one record and transitions its store state. Two
// contexts are used: ctx governs the send (and may be canceled by the caller),
// while transitionCtx is used for store mutations and is never canceled
// mid-flight.
func (r *Relay) processRecord(ctx context.Context, record Record) error {
	now := r.now().UTC()
	transitionCtx := relayTransitionContext(ctx)

	// Send using the original (possibly cancelable) context.
	sendErr := r.sender.Send(ctx, record.outbound())
	if sendErr == nil {
		return r.store.MarkSent(transitionCtx, MarkSentRequest{
			ID:     record.ID,
			Owner:  r.owner,
			Now:    now,
			SentAt: now,
		})
	}

	return r.handleSendError(transitionCtx, record, now, sendErr)
}

// handleSendError decides between retry and permanent failure based on the
// attempt count. When the record has not exhausted retries it is rescheduled
// with a delay; otherwise it is marked as permanently failed.
func (r *Relay) handleSendError(
	ctx context.Context,
	record Record,
	now time.Time,
	sendErr error,
) error {
	// Still within the retry budget — schedule another attempt.
	if record.Attempts < r.maxAttempts {
		retryErr := r.store.Retry(ctx, RetryRequest{
			ID:              record.ID,
			Owner:           r.owner,
			Now:             now,
			NextAvailableAt: now.Add(r.retryDelay),
			LastError:       sendErr.Error(),
		})
		if retryErr == nil {
			r.logger.Warn(
				"xevent outbox send failed; scheduled retry",
				"record_id", record.ID,
				"event_type", record.EventType,
				"attempts", record.Attempts,
				"error", sendErr.Error(),
			)
			return nil
		}
		return errors.Join(sendErr, retryErr)
	}

	// Retries exhausted — mark permanently failed.
	failErr := r.store.MarkFailed(ctx, FailRequest{
		ID:        record.ID,
		Owner:     r.owner,
		Now:       now,
		LastError: sendErr.Error(),
	})
	if failErr == nil {
		r.logger.Error(
			"xevent outbox send failed permanently",
			"record_id", record.ID,
			"event_type", record.EventType,
			"attempts", record.Attempts,
			"error", sendErr.Error(),
		)
		return nil
	}
	return errors.Join(sendErr, failErr)
}

func (r Record) outbound() *xevent.Outbound {
	return &xevent.Outbound{
		EventType:    r.EventType,
		EventID:      r.EventID,
		PartitionKey: r.PartitionKey,
		Payload:      cloneBytes(r.Payload),
		Topic:        r.Topic,
	}
}

// relayTransitionContext returns a fresh context when ctx is nil or already
// canceled. Store state transitions (MarkSent, Retry, MarkFailed) must succeed
// even when the original caller context is done, to avoid leaving records in
// an inconsistent state (claimed but never finalized).
func relayTransitionContext(ctx context.Context) context.Context {
	if ctx == nil || ctx.Err() != nil {
		return context.Background()
	}
	return ctx
}
