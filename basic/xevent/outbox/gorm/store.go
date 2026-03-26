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

	"github.com/codesjoy/pkg/basic/xevent/outbox"
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

// NewGORMStore creates a default GORM-backed outbox store.
func NewGORMStore(cfg GORMStoreConfig) (*GORMStore, error) {
	if cfg.DB == nil {
		return nil, errors.New("xevent outbox gorm db is nil")
	}

	tableName := strings.TrimSpace(cfg.TableName)
	if tableName == "" {
		tableName = (outbox.Record{}).TableName()
	}

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
func (s *GORMStore) Append(ctx context.Context, record *outbox.Record) error {
	if record == nil {
		return errors.New("xevent outbox record is nil")
	}
	ctx = normalizeContext(ctx)

	stored := prepareStoredRecord(*record, time.Now().UTC())

	if err := s.session(ctx).Table(s.tableName).Create(&stored).Error; err != nil {
		return err
	}
	*record = cloneRecord(stored)
	return nil
}

// Claim reserves one batch of eligible records ordered by available time then id.
func (s *GORMStore) Claim(ctx context.Context, req outbox.ClaimRequest) ([]outbox.Record, error) {
	ctx = normalizeContext(ctx)

	req, err := normalizeClaimRequest(req)
	if err != nil {
		return nil, err
	}

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
	sqlText := fmt.Sprintf(
		`SELECT records.*
FROM %s AS records
JOIN (
  SELECT ranked.id
  FROM (
    SELECT records.id, records.available_at,
      ROW_NUMBER() OVER (
        PARTITION BY records.partition_key
        ORDER BY records.available_at ASC, records.id ASC
      ) AS partition_rank
    FROM %s AS records
    WHERE records.status IN (?, ?)
  ) AS ranked
  JOIN %s AS records ON records.id = ranked.id
  WHERE ranked.partition_rank = 1
    AND %s
  ORDER BY records.available_at ASC, records.id ASC
  LIMIT ?
) AS candidates ON candidates.id = records.id
ORDER BY records.available_at ASC, records.id ASC
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
	sqlText := fmt.Sprintf(
		`WITH ranked AS (
  SELECT records.id,
    ROW_NUMBER() OVER (
      PARTITION BY records.partition_key
      ORDER BY records.available_at ASC, records.id ASC
    ) AS partition_rank
  FROM %s AS records
  WHERE records.status IN (?, ?)
),
candidates AS (
  SELECT records.id
  FROM %s AS records
  JOIN ranked ON ranked.id = records.id
  WHERE ranked.partition_rank = 1
    AND %s
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
		tableName,
		claimEligibilitySQLForAlias("records"),
		tableName,
	)

	claimed := make([]outbox.Record, 0, req.Limit)
	if err := tx.Raw(
		sqlText,
		outbox.StatusPending,
		outbox.StatusSending,
		outbox.StatusPending,
		req.Now,
		outbox.StatusSending,
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

func (s *GORMStore) claimSelectedRecords(
	tx *gorm.DB,
	ids []uint64,
	req outbox.ClaimRequest,
	claimUntil time.Time,
) ([]outbox.Record, error) {
	if len(ids) == 0 {
		return nil, nil
	}

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

func (s *GORMStore) updateClaimed(
	ctx context.Context,
	id uint64,
	owner string,
	updates map[string]any,
) error {
	session := s.session(ctx)
	result := session.
		Table(s.tableName).
		Where("id = ? AND status = ? AND claim_owner = ?", id, outbox.StatusSending, owner).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
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

func (s *GORMStore) session(ctx context.Context) *gorm.DB {
	ctx = normalizeContext(ctx)
	if s.sessionFromContext != nil {
		if session := s.sessionFromContext(ctx); session != nil {
			return session.WithContext(ctx)
		}
	}
	return s.db.WithContext(ctx)
}
