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
	"sort"
	"sync"
	"time"
)

// MemoryStore is an in-memory Store implementation useful for tests.
type MemoryStore struct {
	mu      sync.Mutex
	nextID  uint64
	records map[uint64]Record
}

// NewMemoryStore creates a new in-memory outbox store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		records: make(map[uint64]Record),
	}
}

// Append stores one record in memory.
func (s *MemoryStore) Append(ctx context.Context, record *Record) error {
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if record == nil {
		return errors.New("xevent outbox record is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	stored := prepareStoredRecord(*record, time.Now().UTC())
	// Auto-increment: assign the next ID when the caller did not supply one,
	// or advance the counter past an explicitly-set ID.
	if stored.ID == 0 {
		s.nextID++
		stored.ID = s.nextID
	} else if stored.ID > s.nextID {
		s.nextID = stored.ID
	}

	s.records[stored.ID] = stored
	*record = cloneRecord(stored)
	return nil
}

// Claim reserves one batch of eligible records.
func (s *MemoryStore) Claim(ctx context.Context, req ClaimRequest) ([]Record, error) {
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	req, err := normalizeClaimRequest(req)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Step 1: find claimable candidates (at most one per partition key).
	candidates := s.claimCandidatesLocked(req)
	// Step 2: sort by available time then ID for deterministic ordering.
	sort.Slice(candidates, func(i, j int) bool {
		return recordOrderLess(candidates[i], candidates[j])
	})
	// Step 3: apply the batch limit.
	if len(candidates) > req.Limit {
		candidates = candidates[:req.Limit]
	}

	// Step 4: claim each candidate by mutating its status and owner.
	claimed := make([]Record, 0, len(candidates))
	claimUntil := req.Now.Add(req.ClaimTTL).UTC()
	for _, record := range candidates {
		record = claimRecord(record, req.Owner, claimUntil, req.Now)
		s.records[record.ID] = record
		claimed = append(claimed, cloneRecord(record))
	}

	return claimed, nil
}

// MarkSent marks one claimed record as sent.
func (s *MemoryStore) MarkSent(ctx context.Context, req MarkSentRequest) error {
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}

	req, err := normalizeMarkSentRequest(req)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.updateClaimedRecordLocked(req.ID, req.Owner, func(record *Record) {
		record.Status = StatusSent
		record.LastError = ""
		record.ClaimOwner = ""
		record.ClaimUntil = nil
		record.SentAt = &req.SentAt
		record.UpdatedAt = req.Now
	})
}

// Retry requeues one claimed record for a later attempt.
func (s *MemoryStore) Retry(ctx context.Context, req RetryRequest) error {
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}

	req, err := normalizeRetryRequest(req)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.updateClaimedRecordLocked(req.ID, req.Owner, func(record *Record) {
		record.Status = StatusPending
		record.LastError = req.LastError
		record.ClaimOwner = ""
		record.ClaimUntil = nil
		record.SentAt = nil
		record.AvailableAt = req.NextAvailableAt
		record.UpdatedAt = req.Now
	})
}

// MarkFailed marks one claimed record as failed.
func (s *MemoryStore) MarkFailed(ctx context.Context, req FailRequest) error {
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}

	req, err := normalizeFailRequest(req)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.updateClaimedRecordLocked(req.ID, req.Owner, func(record *Record) {
		record.Status = StatusFailed
		record.LastError = req.LastError
		record.ClaimOwner = ""
		record.ClaimUntil = nil
		record.UpdatedAt = req.Now
	})
}

// claimCandidatesLocked returns the earliest unfinished record per partition
// key that is eligible for claiming. This guarantees ordered delivery within a
// partition: only the earliest unfinished record per partition is claimable at
// any time.
func (s *MemoryStore) claimCandidatesLocked(req ClaimRequest) []Record {
	earliestByPartition := make(map[string]Record)
	for _, record := range s.records {
		if !isUnfinished(record) {
			continue
		}

		current, exists := earliestByPartition[record.PartitionKey]
		if !exists || recordOrderLess(record, current) {
			earliestByPartition[record.PartitionKey] = record
		}
	}

	// Filter down to those that pass the time-based eligibility check.
	candidates := make([]Record, 0, len(earliestByPartition))
	for _, record := range earliestByPartition {
		if isClaimEligible(record, req.Now) {
			candidates = append(candidates, record)
		}
	}
	return candidates
}

// claimRecord applies the claim mutation: transitions to Sending, sets the
// owner and deadline, and increments the attempt counter.
func claimRecord(record Record, owner string, claimUntil time.Time, now time.Time) Record {
	record.Status = StatusSending
	record.ClaimOwner = owner
	record.ClaimUntil = &claimUntil
	record.Attempts++
	record.UpdatedAt = now
	return record
}

// updateClaimedRecordLocked is a generic helper that validates ownership of a
// claimed record and applies an arbitrary update mutation.
func (s *MemoryStore) updateClaimedRecordLocked(
	id uint64,
	owner string,
	update func(*Record),
) error {
	record, err := s.claimedRecordLocked(id, owner)
	if err != nil {
		return err
	}
	update(&record)
	s.records[record.ID] = record
	return nil
}

// claimedRecordLocked loads a record by ID and validates that it is currently
// in the Sending state and owned by the given owner.
func (s *MemoryStore) claimedRecordLocked(id uint64, owner string) (Record, error) {
	record, ok := s.records[id]
	if !ok {
		return Record{}, ErrRecordNotFound
	}
	// Ownership validation: status must be Sending and owner must match.
	if record.Status != StatusSending || record.ClaimOwner != owner {
		return Record{}, ErrClaimNotOwned
	}
	return record, nil
}

func isUnfinished(record Record) bool {
	return record.Status == StatusPending || record.Status == StatusSending
}

// isClaimEligible returns true when a record can be claimed. There are two
// eligibility paths:
//   - Pending + available time has passed: fresh record ready for first send.
//   - Sending + available + claim has expired: orphaned record from a previous
//     claim that never completed, eligible for re-claim.
func isClaimEligible(record Record, now time.Time) bool {
	switch record.Status {
	case StatusPending:
		return !record.AvailableAt.After(now)
	case StatusSending:
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

func recordOrderLess(left Record, right Record) bool {
	if !left.AvailableAt.Equal(right.AvailableAt) {
		return left.AvailableAt.Before(right.AvailableAt)
	}
	return left.ID < right.ID
}
