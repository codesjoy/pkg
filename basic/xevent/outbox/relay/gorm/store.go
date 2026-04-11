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
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	outbox "github.com/codesjoy/pkg/basic/xevent/outbox/relay"
	"gorm.io/gorm"
)

// ErrUnsupportedGORMStoreDialect indicates the configured SQL dialect is not supported.
var ErrUnsupportedGORMStoreDialect = errors.New("xevent outbox gorm dialect is unsupported")

// GORMStoreDialect selects the SQL strategy used by GORMStore.
type GORMStoreDialect string

const (
	// GORMStoreDialectStandard uses portable SQL without dialect-specific locking or returning clauses.
	GORMStoreDialectStandard GORMStoreDialect = "standard"
	// GORMStoreDialectMySQL uses MySQL-specific claim SQL.
	GORMStoreDialectMySQL GORMStoreDialect = "mysql"
	// GORMStoreDialectPostgres uses PostgreSQL-specific claim SQL.
	GORMStoreDialectPostgres GORMStoreDialect = "postgres"
)

// GORMStoreConfig configures the default GORM-backed Store implementation.
type GORMStoreConfig struct {
	DB                 *gorm.DB
	TableName          string
	Dialect            GORMStoreDialect
	SessionFromContext func(context.Context) *gorm.DB
}

// GORMStore persists outbox records through GORM.
type GORMStore struct {
	db                 *gorm.DB
	tableName          string
	dialect            GORMStoreDialect
	sessionFromContext func(context.Context) *gorm.DB
}

var _ outbox.Store = (*GORMStore)(nil)

// NewGORMStore creates a default GORM-backed outbox store. If no dialect is
// explicitly configured, it is auto-detected from the GORM Dialector.
func NewGORMStore(cfg GORMStoreConfig) (*GORMStore, error) {
	if cfg.DB == nil {
		return nil, errors.New("xevent outbox gorm db is nil")
	}

	tableName := strings.TrimSpace(cfg.TableName)
	if tableName == "" {
		tableName = (outbox.Record{}).TableName()
	}

	// Auto-detect the SQL dialect when not explicitly set.
	dialect, err := resolveGORMStoreDialect(cfg.DB, cfg.Dialect)
	if err != nil {
		return nil, err
	}

	return &GORMStore{
		db:                 cfg.DB,
		tableName:          tableName,
		dialect:            dialect,
		sessionFromContext: cfg.SessionFromContext,
	}, nil
}

// Append inserts one outbox record using the current GORM handle.
// The session is resolved via session(), which supports transaction-bound
// contexts (e.g. a *gorm.DB obtained from SessionFromContext).
func (s *GORMStore) Append(ctx context.Context, record *outbox.Record) error {
	if record == nil {
		return errors.New("xevent outbox record is nil")
	}
	ctx = normalizeContext(ctx)

	stored := prepareStoredRecord(*record, time.Now().UTC())

	// Use the resolved session so that records are inserted within the
	// caller's transaction when one is active.
	if err := s.session(ctx).Table(s.tableName).Create(&stored).Error; err != nil {
		return err
	}
	*record = cloneRecord(stored)
	return nil
}

// Claim reserves one batch of eligible records ordered by available time then id.
// The entire claim runs inside a transaction so that candidate selection and
// the status update are atomic.
func (s *GORMStore) Claim(ctx context.Context, req outbox.ClaimRequest) ([]outbox.Record, error) {
	ctx = normalizeContext(ctx)

	req, err := normalizeClaimRequest(req)
	if err != nil {
		return nil, err
	}

	// Transactional claim: select candidates and update their status atomically.
	var claimed []outbox.Record
	claim := s.claimer()
	err = s.session(ctx).Transaction(func(tx *gorm.DB) error {
		claimUntil := req.Now.Add(req.ClaimTTL).UTC()
		var claimErr error
		claimed, claimErr = claim(tx, req, claimUntil)
		return claimErr
	})
	if err != nil {
		return nil, err
	}

	for i := range claimed {
		claimed[i] = cloneRecord(claimed[i])
	}
	return claimed, nil
}

// MarkSent marks one claimed record as sent.
func (s *GORMStore) MarkSent(ctx context.Context, req outbox.MarkSentRequest) error {
	ctx = normalizeContext(ctx)

	req, err := normalizeMarkSentRequest(req)
	if err != nil {
		return err
	}

	return s.updateClaimed(ctx, req.ID, req.Owner, map[string]any{
		"status":      outbox.StatusSent,
		"last_error":  "",
		"claim_owner": "",
		"claim_until": nil,
		"sent_at":     req.SentAt,
		"updated_at":  req.Now,
	})
}

// Retry requeues one claimed record for a later attempt.
func (s *GORMStore) Retry(ctx context.Context, req outbox.RetryRequest) error {
	ctx = normalizeContext(ctx)

	req, err := normalizeRetryRequest(req)
	if err != nil {
		return err
	}

	return s.updateClaimed(ctx, req.ID, req.Owner, map[string]any{
		"status":       outbox.StatusPending,
		"available_at": req.NextAvailableAt,
		"last_error":   req.LastError,
		"claim_owner":  "",
		"claim_until":  nil,
		"sent_at":      nil,
		"updated_at":   req.Now,
	})
}

// MarkFailed marks one claimed record as permanently failed.
func (s *GORMStore) MarkFailed(ctx context.Context, req outbox.FailRequest) error {
	ctx = normalizeContext(ctx)

	req, err := normalizeFailRequest(req)
	if err != nil {
		return err
	}

	return s.updateClaimed(ctx, req.ID, req.Owner, map[string]any{
		"status":      outbox.StatusFailed,
		"last_error":  req.LastError,
		"claim_owner": "",
		"claim_until": nil,
		"updated_at":  req.Now,
	})
}

func (s *GORMStore) claimStandard(
	tx *gorm.DB,
	req outbox.ClaimRequest,
	claimUntil time.Time,
) ([]outbox.Record, error) {
	tableName := s.quotedTable(tx)
	// Standard SQL claim: the NOT EXISTS subquery enforces per-partition
	// ordering by ensuring only records with no earlier unfinished sibling
	// (same partition_key, earlier available_at or id) are selected.
	sqlText := fmt.Sprintf(
		`SELECT records.*
FROM %s AS records
WHERE %s
  AND NOT EXISTS (
    SELECT 1
    FROM %s AS earlier
    WHERE earlier.partition_key = records.partition_key
      AND earlier.status IN (?, ?)
      AND (
        earlier.available_at < records.available_at OR
        (earlier.available_at = records.available_at AND earlier.id < records.id)
      )
  )
ORDER BY records.available_at ASC, records.id ASC
LIMIT ?`,
		tableName,
		claimEligibilitySQLForAlias("records"),
		tableName,
	)

	candidates, err := s.selectClaimCandidates(
		tx,
		sqlText,
		outbox.StatusPending,
		req.Now,
		outbox.StatusSending,
		req.Now,
		req.Now,
		outbox.StatusPending,
		outbox.StatusSending,
		req.Limit,
	)
	if err != nil || len(candidates) == 0 {
		return candidates, err
	}

	return s.claimSelectedRecords(tx, recordIDs(candidates), req, claimUntil)
}

type claimerFunc func(*gorm.DB, outbox.ClaimRequest, time.Time) ([]outbox.Record, error)

// claimer returns the dialect-appropriate claim function.
func (s *GORMStore) claimer() claimerFunc {
	switch s.dialect {
	case GORMStoreDialectPostgres:
		return s.claimPostgres
	case GORMStoreDialectMySQL:
		return s.claimMySQL
	default:
		return s.claimStandard
	}
}

func (s *GORMStore) claimMySQL(
	tx *gorm.DB,
	req outbox.ClaimRequest,
	claimUntil time.Time,
) ([]outbox.Record, error) {
	tableName := s.quotedTable(tx)
	// MySQL claim: find the earliest pending and earliest sending record per
	// partition separately, UNION ALL them, then pick the earliest unfinished
	// row per partition before applying SKIP LOCKED.
	sqlText := fmt.Sprintf(
		`SELECT records.*
FROM %s AS records
JOIN (
  SELECT ranked.id
  FROM (
    SELECT unfinished.id,
      ROW_NUMBER() OVER (
        PARTITION BY unfinished.partition_key
        ORDER BY unfinished.available_at ASC, unfinished.id ASC
      ) AS partition_rank
    FROM (
      SELECT pending.partition_key, pending.id, pending.available_at
      FROM (
        SELECT records.partition_key, records.id, records.available_at,
          ROW_NUMBER() OVER (
            PARTITION BY records.partition_key
            ORDER BY records.available_at ASC, records.id ASC
          ) AS status_rank
        FROM %s AS records
        WHERE records.status = ?
      ) AS pending
      WHERE pending.status_rank = 1
      UNION ALL
      SELECT sending.partition_key, sending.id, sending.available_at
      FROM (
        SELECT records.partition_key, records.id, records.available_at,
          ROW_NUMBER() OVER (
            PARTITION BY records.partition_key
            ORDER BY records.available_at ASC, records.id ASC
          ) AS status_rank
        FROM %s AS records
        WHERE records.status = ?
      ) AS sending
      WHERE sending.status_rank = 1
    ) AS unfinished
  ) AS ranked
  WHERE ranked.partition_rank = 1
) AS candidates ON candidates.id = records.id
WHERE %s
ORDER BY records.available_at ASC, records.id ASC
LIMIT ?
FOR UPDATE SKIP LOCKED`,
		tableName,
		tableName,
		tableName,
		claimEligibilitySQLForAlias("records"),
	)

	candidates, err := s.selectClaimCandidates(
		tx,
		sqlText,
		outbox.StatusPending,
		outbox.StatusSending,
		outbox.StatusPending,
		req.Now,
		outbox.StatusSending,
		req.Now,
		req.Now,
		req.Limit,
	)
	if err != nil || len(candidates) == 0 {
		return candidates, err
	}

	return s.claimSelectedRecords(tx, recordIDs(candidates), req, claimUntil)
}

func (s *GORMStore) claimPostgres(
	tx *gorm.DB,
	req outbox.ClaimRequest,
	claimUntil time.Time,
) ([]outbox.Record, error) {
	tableName := s.quotedTable(tx)
	pendingStatus := string(outbox.StatusPending)
	sendingStatus := string(outbox.StatusSending)
	// PostgreSQL claim: use literal status predicates so reviewed migrations can
	// rely on partial indexes, then do one pending seek and one sending seek per
	// partition before selecting the earliest unfinished row.
	sqlText := fmt.Sprintf(
		`WITH distinct_partitions AS (
  SELECT partition_key
  FROM %s
  WHERE status = '%s'
  UNION
  SELECT partition_key
  FROM %s
  WHERE status = '%s'
),
earliest_pending AS (
  SELECT dp.partition_key, pending.id, pending.available_at
  FROM distinct_partitions AS dp
  LEFT JOIN LATERAL (
    SELECT records.id, records.available_at
    FROM %s AS records
    WHERE records.partition_key = dp.partition_key
      AND records.status = '%s'
    ORDER BY records.available_at ASC, records.id ASC
    LIMIT 1
  ) AS pending ON TRUE
),
earliest_sending AS (
  SELECT dp.partition_key, sending.id, sending.available_at
  FROM distinct_partitions AS dp
  LEFT JOIN LATERAL (
    SELECT records.id, records.available_at
    FROM %s AS records
    WHERE records.partition_key = dp.partition_key
      AND records.status = '%s'
    ORDER BY records.available_at ASC, records.id ASC
    LIMIT 1
  ) AS sending ON TRUE
),
earliest_unfinished AS (
  SELECT pending.partition_key,
    CASE
      WHEN pending.id IS NULL THEN sending.id
      WHEN sending.id IS NULL THEN pending.id
      WHEN pending.available_at < sending.available_at THEN pending.id
      WHEN pending.available_at > sending.available_at THEN sending.id
      WHEN pending.id < sending.id THEN pending.id
      ELSE sending.id
    END AS id
  FROM earliest_pending AS pending
  JOIN earliest_sending AS sending
    ON sending.partition_key = pending.partition_key
  WHERE pending.id IS NOT NULL OR sending.id IS NOT NULL
),
candidates AS (
  SELECT records.id
  FROM earliest_unfinished AS eu
  JOIN %s AS records ON records.id = eu.id
  WHERE %s
  ORDER BY records.available_at ASC, records.id ASC
  LIMIT ?
  FOR UPDATE OF records SKIP LOCKED
)
UPDATE %s AS records
SET status = ?,
    claim_owner = ?,
    claim_until = ?,
    attempts = records.attempts + 1,
    updated_at = ?
FROM candidates
WHERE records.id = candidates.id
RETURNING records.*`,
		tableName,
		pendingStatus,
		tableName,
		sendingStatus,
		tableName,
		pendingStatus,
		tableName,
		sendingStatus,
		tableName,
		claimEligibilitySQLWithLiteralStatuses("records"),
		tableName,
	)

	claimed := make([]outbox.Record, 0, req.Limit)
	if err := tx.Raw(
		sqlText,
		req.Now,
		req.Now,
		req.Now,
		req.Limit,
		outbox.StatusSending,
		req.Owner,
		claimUntil,
		req.Now,
	).Scan(&claimed).Error; err != nil {
		return nil, err
	}
	return claimed, nil
}

// selectClaimCandidates executes a raw SQL query and returns the matching records.
func (s *GORMStore) selectClaimCandidates(
	tx *gorm.DB,
	sqlText string,
	args ...any,
) ([]outbox.Record, error) {
	candidates := make([]outbox.Record, 0)
	if err := tx.Raw(sqlText, args...).Scan(&candidates).Error; err != nil {
		return nil, err
	}
	return candidates, nil
}

// claimSelectedRecords performs an optimistic update: it updates only those
// records whose IDs match AND are still eligible (re-checking the eligibility
// SQL to guard against concurrent claims). It then re-fetches the successfully
// claimed records.
func (s *GORMStore) claimSelectedRecords(
	tx *gorm.DB,
	ids []uint64,
	req outbox.ClaimRequest,
	claimUntil time.Time,
) ([]outbox.Record, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// Optimistic update: only claim records that are still eligible at the
	// moment of the UPDATE. This prevents races where another transaction
	// claimed the same record between SELECT and UPDATE.
	result := tx.Table(s.tableName).
		Where("id IN ?", ids).
		Where(claimEligibilitySQL(), outbox.StatusPending, req.Now, outbox.StatusSending, req.Now, req.Now).
		Updates(map[string]any{
			"status":      outbox.StatusSending,
			"claim_owner": req.Owner,
			"claim_until": claimUntil,
			"attempts":    gorm.Expr("attempts + ?", 1),
			"updated_at":  req.Now,
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}

	// Re-fetch the records that were successfully claimed to return to the caller.
	claimed := make([]outbox.Record, 0, len(ids))
	if err := tx.Table(s.tableName).
		Where("id IN ?", ids).
		Where("status = ? AND claim_owner = ?", outbox.StatusSending, req.Owner).
		Order("available_at ASC").
		Order("id ASC").
		Find(&claimed).Error; err != nil {
		return nil, err
	}
	return claimed, nil
}

// updateClaimed updates a claimed record after validating ownership. When no
// rows are affected it performs a secondary query to discriminate between
// "record not found" and "record exists but not owned by caller".
func (s *GORMStore) updateClaimed(
	ctx context.Context,
	id uint64,
	owner string,
	updates map[string]any,
) error {
	session := s.session(ctx)
	result := session.
		Table(s.tableName).
		// Ownership validation: only update records in Sending state owned by caller.
		Where("id = ? AND status = ? AND claim_owner = ?", id, outbox.StatusSending, owner).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// Discriminate: does the record not exist, or is it owned by someone else?
		var count int64
		if err := session.Table(s.tableName).Where("id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return outbox.ErrRecordNotFound
		}
		return outbox.ErrClaimNotOwned
	}
	return nil
}

func claimEligibilitySQL() string {
	return claimEligibilitySQLForAlias("")
}

func claimEligibilitySQLWithLiteralStatuses(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}

	return `((
` + prefix + `status = '` + string(outbox.StatusPending) + `' AND ` + prefix + `available_at <= ?
) OR (
` + prefix + `status = '` + string(outbox.StatusSending) + `' AND ` + prefix + `available_at <= ? AND (` + prefix + `claim_until IS NULL OR ` + prefix + `claim_until <= ?)
))`
}

// claimEligibilitySQLForAlias generates a SQL fragment with two conditions
// OR-ed together:
//   - Condition 1: status=pending AND available_at <= now (fresh record ready to send).
//   - Condition 2: status=sending AND available_at <= now AND claim expired
//     (orphaned claim eligible for re-claim).
func claimEligibilitySQLForAlias(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}

	return `((
` + prefix + `status = ? AND ` + prefix + `available_at <= ?
) OR (
` + prefix + `status = ? AND ` + prefix + `available_at <= ? AND (` + prefix + `claim_until IS NULL OR ` + prefix + `claim_until <= ?)
))`
}

func resolveGORMStoreDialect(
	db *gorm.DB,
	configured GORMStoreDialect,
) (GORMStoreDialect, error) {
	if strings.TrimSpace(string(configured)) == "" {
		return detectGORMStoreDialect(db), nil
	}
	return normalizeGORMStoreDialect(configured)
}

func normalizeGORMStoreDialect(dialect GORMStoreDialect) (GORMStoreDialect, error) {
	normalized := GORMStoreDialect(strings.ToLower(strings.TrimSpace(string(dialect))))
	switch normalized {
	case GORMStoreDialectStandard, GORMStoreDialectMySQL, GORMStoreDialectPostgres:
		return normalized, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedGORMStoreDialect, dialect)
	}
}

func detectGORMStoreDialect(db *gorm.DB) GORMStoreDialect {
	if db == nil || db.Dialector == nil {
		return GORMStoreDialectStandard
	}

	switch strings.ToLower(strings.TrimSpace(db.Name())) {
	case "postgres", "postgresql", "pgx":
		return GORMStoreDialectPostgres
	case "mysql", "mariadb":
		return GORMStoreDialectMySQL
	default:
		return GORMStoreDialectStandard
	}
}

func recordIDs(records []outbox.Record) []uint64 {
	ids := make([]uint64, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	return ids
}

func prepareStoredRecord(record outbox.Record, now time.Time) outbox.Record {
	stored := cloneRecord(record)
	if stored.Status == "" {
		stored.Status = outbox.StatusPending
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

func cloneRecord(record outbox.Record) outbox.Record {
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

func normalizeClaimRequest(req outbox.ClaimRequest) (outbox.ClaimRequest, error) {
	if req.Owner == "" {
		return outbox.ClaimRequest{}, errors.New("xevent outbox claim owner is required")
	}
	if req.Limit <= 0 {
		return outbox.ClaimRequest{}, errors.New("xevent outbox claim limit must be > 0")
	}
	if req.ClaimTTL <= 0 {
		return outbox.ClaimRequest{}, errors.New("xevent outbox claim ttl must be > 0")
	}
	req.Now = normalizeTime(req.Now, time.Now)
	return req, nil
}

func normalizeMarkSentRequest(req outbox.MarkSentRequest) (outbox.MarkSentRequest, error) {
	if req.Owner == "" {
		return outbox.MarkSentRequest{}, errors.New("xevent outbox claim owner is required")
	}
	req.Now = normalizeTime(req.Now, time.Now)
	if req.SentAt.IsZero() {
		req.SentAt = req.Now
	} else {
		req.SentAt = req.SentAt.UTC()
	}
	return req, nil
}

func normalizeRetryRequest(req outbox.RetryRequest) (outbox.RetryRequest, error) {
	if req.Owner == "" {
		return outbox.RetryRequest{}, errors.New("xevent outbox claim owner is required")
	}
	req.Now = normalizeTime(req.Now, time.Now)
	if req.NextAvailableAt.IsZero() {
		req.NextAvailableAt = req.Now
	} else {
		req.NextAvailableAt = req.NextAvailableAt.UTC()
	}
	return req, nil
}

func normalizeFailRequest(req outbox.FailRequest) (outbox.FailRequest, error) {
	if req.Owner == "" {
		return outbox.FailRequest{}, errors.New("xevent outbox claim owner is required")
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

func (s *GORMStore) quotedTable(tx *gorm.DB) string {
	stmt := &gorm.Statement{DB: tx}
	return stmt.Quote(s.tableName)
}

// session resolves the appropriate *gorm.DB for the given context. When
// SessionFromContext is configured (e.g. to inject a transaction-bound
// session), it takes priority; otherwise the default db handle is used.
func (s *GORMStore) session(ctx context.Context) *gorm.DB {
	ctx = normalizeContext(ctx)
	if s.sessionFromContext != nil {
		if session := s.sessionFromContext(ctx); session != nil {
			return session.WithContext(ctx)
		}
	}
	return s.db.WithContext(ctx)
}
