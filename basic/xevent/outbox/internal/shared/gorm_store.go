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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/codesjoy/pkg/basic/xevent/outbox/debezium"
	outbox "github.com/codesjoy/pkg/basic/xevent/outbox/relay"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrUnsupportedDialect indicates the configured SQL dialect is not supported.
var ErrUnsupportedDialect = errors.New("xevent outbox shared gorm dialect is unsupported")

// Dialect selects the SQL strategy used by GORMStore.
type Dialect string

const (
	// DialectStandard uses portable SQL without dialect-specific locking.
	DialectStandard Dialect = "standard"
	// DialectMySQL uses MySQL-specific claim SQL with FOR UPDATE SKIP LOCKED.
	DialectMySQL Dialect = "mysql"
	// DialectPostgres uses PostgreSQL-specific claim SQL with RETURNING.
	DialectPostgres Dialect = "postgres"
)

// GORMStoreConfig configures the shared GORM-backed store implementation.
type GORMStoreConfig struct {
	// DB is the GORM database handle used for all operations.
	DB *gorm.DB
	// TableName overrides the default outbox table name when non-empty.
	TableName string
	// Dialect selects the SQL strategy; auto-detected when empty.
	Dialect Dialect
	// SessionFromContext optionally extracts a per-request GORM session from ctx.
	SessionFromContext func(context.Context) *gorm.DB
}

// GORMStore persists shared outbox rows through GORM.
type GORMStore struct {
	db                 *gorm.DB
	tableName          string
	dialect            Dialect
	sessionFromContext func(context.Context) *gorm.DB
}

// CutoverRequest controls one relay backlog cutover run.
type CutoverRequest struct {
	// Now is the reference time for eligibility checks.
	Now time.Time
	// BatchSize limits how many records are migrated in one transaction.
	BatchSize int
}

// NewGORMStore creates a configured shared GORM-backed outbox store.
func NewGORMStore(cfg GORMStoreConfig) (*GORMStore, error) {
	if cfg.DB == nil {
		return nil, errors.New("xevent outbox shared gorm db is nil")
	}

	tableName := strings.TrimSpace(cfg.TableName)
	if tableName == "" {
		tableName = DefaultTableName
	}

	dialect, err := ResolveDialect(cfg.DB, cfg.Dialect)
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

// AppendRelay inserts one relay outbox row and updates record with the
// database-assigned values.
func (s *GORMStore) AppendRelay(ctx context.Context, record *outbox.Record) error {
	if record == nil {
		return errors.New("xevent outbox record is nil")
	}
	ctx = NormalizeContext(ctx)

	stored := RelayRecordToDBRecord(*record, time.Now().UTC())
	if err := s.session(ctx).Table(s.tableName).Create(&stored).Error; err != nil {
		return err
	}
	*record = DBRecordToRelayRecord(stored)
	return nil
}

// ClaimRelay reserves one batch of eligible relay records ordered by
// available_at then id, using the dialect-appropriate locking strategy.
func (s *GORMStore) ClaimRelay(
	ctx context.Context,
	req outbox.ClaimRequest,
) ([]outbox.Record, error) {
	ctx = NormalizeContext(ctx)

	req, err := normalizeRelayClaimRequest(req)
	if err != nil {
		return nil, err
	}

	var claimed []DBRecord
	claim := s.relayClaimer()
	err = s.session(ctx).Transaction(func(tx *gorm.DB) error {
		claimUntil := req.Now.Add(req.ClaimTTL).UTC()
		var claimErr error
		claimed, claimErr = claim(tx, req, claimUntil)
		return claimErr
	})
	if err != nil {
		return nil, err
	}

	records := make([]outbox.Record, 0, len(claimed))
	for _, record := range claimed {
		records = append(records, DBRecordToRelayRecord(record))
	}
	return records, nil
}

// ClaimRelayMySQLForTest exposes the MySQL-specific relay claim path to package-local tests.
func (s *GORMStore) ClaimRelayMySQLForTest(
	tx *gorm.DB,
	req outbox.ClaimRequest,
	claimUntil time.Time,
) ([]DBRecord, error) {
	return s.claimRelayMySQL(tx, req, claimUntil)
}

// ClaimRelayPostgresForTest exposes the PostgreSQL-specific relay claim path to package-local tests.
func (s *GORMStore) ClaimRelayPostgresForTest(
	tx *gorm.DB,
	req outbox.ClaimRequest,
	claimUntil time.Time,
) ([]DBRecord, error) {
	return s.claimRelayPostgres(tx, req, claimUntil)
}

// SelectClaimCandidatesForTest exposes the raw candidate query helper to package-local tests.
func (s *GORMStore) SelectClaimCandidatesForTest(
	tx *gorm.DB,
	sqlText string,
	args ...any,
) ([]DBRecord, error) {
	return s.selectClaimCandidates(tx, sqlText, args...)
}

// ClaimSelectedRelayRecordsForTest exposes the optimistic claim update helper to package-local tests.
func (s *GORMStore) ClaimSelectedRelayRecordsForTest(
	tx *gorm.DB,
	ids []uint64,
	req outbox.ClaimRequest,
	claimUntil time.Time,
) ([]DBRecord, error) {
	return s.claimSelectedRelayRecords(tx, ids, req, claimUntil)
}

// MarkRelaySent marks one claimed relay record as sent.
func (s *GORMStore) MarkRelaySent(ctx context.Context, req outbox.MarkSentRequest) error {
	ctx = NormalizeContext(ctx)

	req, err := normalizeRelayMarkSentRequest(req)
	if err != nil {
		return err
	}

	return s.updateClaimedRelay(ctx, req.ID, req.Owner, map[string]any{
		"status":      outbox.StatusSent,
		"last_error":  "",
		"claim_owner": "",
		"claim_until": nil,
		"sent_at":     req.SentAt,
		"updated_at":  req.Now,
	})
}

// RetryRelay requeues one claimed relay record for a later attempt.
func (s *GORMStore) RetryRelay(ctx context.Context, req outbox.RetryRequest) error {
	ctx = NormalizeContext(ctx)

	req, err := normalizeRelayRetryRequest(req)
	if err != nil {
		return err
	}

	return s.updateClaimedRelay(ctx, req.ID, req.Owner, map[string]any{
		"status":       outbox.StatusPending,
		"available_at": req.NextAvailableAt,
		"last_error":   req.LastError,
		"claim_owner":  "",
		"claim_until":  nil,
		"sent_at":      nil,
		"updated_at":   req.Now,
	})
}

// MarkRelayFailed marks one claimed relay record as permanently failed.
func (s *GORMStore) MarkRelayFailed(ctx context.Context, req outbox.FailRequest) error {
	ctx = NormalizeContext(ctx)

	req, err := normalizeRelayFailRequest(req)
	if err != nil {
		return err
	}

	return s.updateClaimedRelay(ctx, req.ID, req.Owner, map[string]any{
		"status":      outbox.StatusFailed,
		"last_error":  req.LastError,
		"claim_owner": "",
		"claim_until": nil,
		"updated_at":  req.Now,
	})
}

// AppendDebezium inserts one CDC outbox row and updates record with the
// database-assigned values.
func (s *GORMStore) AppendDebezium(ctx context.Context, record *debezium.Record) error {
	if record == nil {
		return errors.New("xevent outbox debezium record is nil")
	}
	ctx = NormalizeContext(ctx)

	stored, err := DebeziumRecordToDBRecord(*record, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := s.session(ctx).Table(s.tableName).Create(&stored).Error; err != nil {
		return err
	}
	*record = DBRecordToDebeziumRecord(stored)
	return nil
}

// DeleteDebeziumBefore deletes up to limit CDC rows whose created_at is older
// than cutoff, returning the number of rows deleted.
func (s *GORMStore) DeleteDebeziumBefore(
	ctx context.Context,
	cutoff time.Time,
	limit int,
) (int64, error) {
	ctx = NormalizeContext(ctx)
	if limit <= 0 {
		return 0, errors.New("xevent outbox debezium delete limit must be > 0")
	}

	cutoff = cutoff.UTC()
	var deleted int64
	err := s.session(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []uint64
		if err := tx.Table(s.tableName).
			Model(&DBRecord{}).
			Where("mode = ? AND created_at < ?", ModeCDC, cutoff).
			Order("created_at ASC").
			Order("id ASC").
			Limit(limit).
			Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}

		result := tx.Table(s.tableName).
			Where("mode = ? AND id IN ?", ModeCDC, ids).
			Delete(&DBRecord{})
		if result.Error != nil {
			return result.Error
		}
		deleted = result.RowsAffected
		return nil
	})
	return deleted, err
}

// CutoverRelayBacklog migrates eligible relay rows to CDC rows in batches,
// marking each source relay row as handed off. It returns the total number of
// rows migrated.
func (s *GORMStore) CutoverRelayBacklog(ctx context.Context, req CutoverRequest) (int64, error) {
	ctx = NormalizeContext(ctx)
	req, err := normalizeCutoverRequest(req)
	if err != nil {
		return 0, err
	}

	var total int64
	for {
		var movedThisBatch int64
		var candidatesFound bool
		err := s.session(ctx).Transaction(func(tx *gorm.DB) error {
			candidates, err := s.selectCutoverCandidates(tx, req)
			if err != nil {
				return err
			}
			if len(candidates) == 0 {
				return nil
			}
			candidatesFound = true

			for _, candidate := range candidates {
				cdcRecord, err := PrepareCutoverDebeziumRecord(candidate, req.Now)
				if err != nil {
					return err
				}

				result := tx.Table(s.tableName).
					Clauses(clause.OnConflict{
						Columns:   []clause.Column{{Name: "handoff_from_id"}},
						DoNothing: true,
					}).
					Create(&cdcRecord)
				if result.Error != nil {
					return result.Error
				}
				movedThisBatch += result.RowsAffected

				if err := tx.Table(s.tableName).
					Where("id = ? AND mode = ? AND status IN ?", candidate.ID, ModeRelay, []string{
						string(outbox.StatusPending),
						string(outbox.StatusSending),
						relayStatusHandedOff,
					}).
					Updates(map[string]any{
						"status":      relayStatusHandedOff,
						"claim_owner": "",
						"claim_until": nil,
						"updated_at":  req.Now,
					}).Error; err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return total, err
		}
		if !candidatesFound {
			return total, nil
		}
		total += movedThisBatch
	}
}

// ResolveDialect returns the normalised dialect when configured is non-empty,
// otherwise auto-detects from db.
func ResolveDialect(db *gorm.DB, configured Dialect) (Dialect, error) {
	if strings.TrimSpace(string(configured)) == "" {
		return DetectDialect(db), nil
	}
	return NormalizeDialect(configured)
}

// NormalizeDialect lowercases and trims the dialect value, returning an error
// for unsupported values.
func NormalizeDialect(dialect Dialect) (Dialect, error) {
	normalized := Dialect(strings.ToLower(strings.TrimSpace(string(dialect))))
	switch normalized {
	case DialectStandard, DialectMySQL, DialectPostgres:
		return normalized, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedDialect, dialect)
	}
}

// DetectDialect inspects the GORM Dialector name and returns the matching
// Dialect constant, defaulting to DialectStandard.
func DetectDialect(db *gorm.DB) Dialect {
	if db == nil || db.Dialector == nil {
		return DialectStandard
	}

	switch strings.ToLower(strings.TrimSpace(db.Name())) {
	case "postgres", "postgresql", "pgx":
		return DialectPostgres
	case "mysql", "mariadb":
		return DialectMySQL
	default:
		return DialectStandard
	}
}

// relayClaimerFunc is the signature for dialect-specific relay claim implementations.
type relayClaimerFunc func(*gorm.DB, outbox.ClaimRequest, time.Time) ([]DBRecord, error)

// relayClaimer returns the dialect-appropriate claim function.
func (s *GORMStore) relayClaimer() relayClaimerFunc {
	switch s.dialect {
	case DialectPostgres:
		return s.claimRelayPostgres
	case DialectMySQL:
		return s.claimRelayMySQL
	default:
		return s.claimRelayStandard
	}
}

// claimRelayStandard claims relay records using portable SQL with a two-step
// select-then-update strategy.
func (s *GORMStore) claimRelayStandard(
	tx *gorm.DB,
	req outbox.ClaimRequest,
	claimUntil time.Time,
) ([]DBRecord, error) {
	tableName := s.quotedTable(tx)
	sqlText := fmt.Sprintf(
		`SELECT records.*
FROM %s AS records
WHERE %s
  AND NOT EXISTS (
    SELECT 1
    FROM %s AS earlier
    WHERE earlier.mode = ?
      AND earlier.partition_key = records.partition_key
      AND earlier.status IN (?, ?)
      AND (
        earlier.available_at < records.available_at OR
        (earlier.available_at = records.available_at AND earlier.id < records.id)
      )
  )
ORDER BY records.available_at ASC, records.id ASC
LIMIT ?`,
		tableName,
		relayClaimEligibilitySQLForAlias("records"),
		tableName,
	)

	candidates, err := s.selectClaimCandidates(
		tx,
		sqlText,
		ModeRelay,
		outbox.StatusPending,
		req.Now,
		outbox.StatusSending,
		req.Now,
		req.Now,
		ModeRelay,
		outbox.StatusPending,
		outbox.StatusSending,
		req.Limit,
	)
	if err != nil || len(candidates) == 0 {
		return candidates, err
	}

	return s.claimSelectedRelayRecords(tx, recordIDs(candidates), req, claimUntil)
}

// claimRelayMySQL claims relay records using MySQL-specific FOR UPDATE SKIP
// LOCKED with window-function-based candidate selection.
func (s *GORMStore) claimRelayMySQL(
	tx *gorm.DB,
	req outbox.ClaimRequest,
	claimUntil time.Time,
) ([]DBRecord, error) {
	tableName := s.quotedTable(tx)
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
        WHERE records.mode = ? AND records.status = ?
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
        WHERE records.mode = ? AND records.status = ?
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
		relayClaimEligibilitySQLForAlias("records"),
	)

	candidates, err := s.selectClaimCandidates(
		tx,
		sqlText,
		ModeRelay,
		outbox.StatusPending,
		ModeRelay,
		outbox.StatusSending,
		ModeRelay,
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

	return s.claimSelectedRelayRecords(tx, recordIDs(candidates), req, claimUntil)
}

// claimRelayPostgres claims relay records using a single PostgreSQL UPDATE ...
// FROM CTE with FOR UPDATE SKIP LOCKED and RETURNING.
func (s *GORMStore) claimRelayPostgres(
	tx *gorm.DB,
	req outbox.ClaimRequest,
	claimUntil time.Time,
) ([]DBRecord, error) {
	tableName := s.quotedTable(tx)
	pendingStatus := string(outbox.StatusPending)
	sendingStatus := string(outbox.StatusSending)
	sqlText := fmt.Sprintf(
		`WITH distinct_partitions AS (
  SELECT partition_key
  FROM %s
  WHERE mode = '%s' AND status = '%s'
  UNION
  SELECT partition_key
  FROM %s
  WHERE mode = '%s' AND status = '%s'
),
earliest_pending AS (
  SELECT dp.partition_key, pending.id, pending.available_at
  FROM distinct_partitions AS dp
  LEFT JOIN LATERAL (
    SELECT records.id, records.available_at
    FROM %s AS records
    WHERE records.mode = '%s'
      AND records.partition_key = dp.partition_key
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
    WHERE records.mode = '%s'
      AND records.partition_key = dp.partition_key
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
		ModeRelay,
		pendingStatus,
		tableName,
		ModeRelay,
		sendingStatus,
		tableName,
		ModeRelay,
		pendingStatus,
		tableName,
		ModeRelay,
		sendingStatus,
		tableName,
		relayClaimEligibilityLiteralSQL("records"),
		tableName,
	)

	claimed := make([]DBRecord, 0, req.Limit)
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

// selectClaimCandidates executes raw SQL and scans the result into DBRecords.
func (s *GORMStore) selectClaimCandidates(
	tx *gorm.DB,
	sqlText string,
	args ...any,
) ([]DBRecord, error) {
	candidates := make([]DBRecord, 0)
	if err := tx.Raw(sqlText, args...).Scan(&candidates).Error; err != nil {
		return nil, err
	}
	return candidates, nil
}

// claimSelectedRelayRecords performs an optimistic UPDATE on the given IDs,
// returning only those rows whose status and owner match after the update.
func (s *GORMStore) claimSelectedRelayRecords(
	tx *gorm.DB,
	ids []uint64,
	req outbox.ClaimRequest,
	claimUntil time.Time,
) ([]DBRecord, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	result := tx.Table(s.tableName).
		Where("mode = ? AND id IN ?", ModeRelay, ids).
		Where(relayClaimEligibilitySQL(), ModeRelay, outbox.StatusPending, req.Now, outbox.StatusSending, req.Now, req.Now).
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

	claimed := make([]DBRecord, 0, len(ids))
	if err := tx.Table(s.tableName).
		Where("mode = ? AND id IN ?", ModeRelay, ids).
		Where("status = ? AND claim_owner = ?", outbox.StatusSending, req.Owner).
		Order("available_at ASC").
		Order("id ASC").
		Find(&claimed).Error; err != nil {
		return nil, err
	}
	return claimed, nil
}

// updateClaimedRelay applies column updates to one claimed relay record,
// distinguishing between not-found and not-owned errors.
func (s *GORMStore) updateClaimedRelay(
	ctx context.Context,
	id uint64,
	owner string,
	updates map[string]any,
) error {
	session := s.session(ctx)
	result := session.
		Table(s.tableName).
		Where("mode = ? AND id = ? AND status = ? AND claim_owner = ?", ModeRelay, id, outbox.StatusSending, owner).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var count int64
		if err := session.Table(s.tableName).Where("mode = ? AND id = ?", ModeRelay, id).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return outbox.ErrRecordNotFound
		}
		return outbox.ErrClaimNotOwned
	}
	return nil
}

// selectCutoverCandidates fetches up to BatchSize relay records that are
// eligible for cutover at the given time.
func (s *GORMStore) selectCutoverCandidates(tx *gorm.DB, req CutoverRequest) ([]DBRecord, error) {
	candidates := make([]DBRecord, 0, req.BatchSize)
	err := tx.Table(s.tableName).
		Where("mode = ?", ModeRelay).
		Where("available_at <= ?", req.Now).
		Where(`(
status = ? OR (
status = ? AND (claim_until IS NULL OR claim_until <= ?)
))`, outbox.StatusPending, outbox.StatusSending, req.Now).
		Order("available_at ASC").
		Order("id ASC").
		Limit(req.BatchSize).
		Find(&candidates).Error
	return candidates, err
}

// relayClaimEligibilitySQL returns the parameterised eligibility WHERE clause
// without a table alias.
func relayClaimEligibilitySQL() string {
	return relayClaimEligibilitySQLForAlias("")
}

// relayClaimEligibilityLiteralSQL returns the eligibility WHERE clause with
// literal status values instead of placeholders.
func relayClaimEligibilityLiteralSQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}

	return prefix + `mode = '` + string(ModeRelay) + `' AND ((
` + prefix + `status = '` + string(outbox.StatusPending) + `' AND ` + prefix + `available_at <= ?
) OR (
` + prefix + `status = '` + string(outbox.StatusSending) + `' AND ` + prefix + `available_at <= ? AND (` + prefix + `claim_until IS NULL OR ` + prefix + `claim_until <= ?)
))`
}

// relayClaimEligibilitySQLForAlias returns the parameterised eligibility WHERE
// clause qualified by the given alias.
func relayClaimEligibilitySQLForAlias(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}

	return prefix + `mode = ? AND ((
` + prefix + `status = ? AND ` + prefix + `available_at <= ?
) OR (
` + prefix + `status = ? AND ` + prefix + `available_at <= ? AND (` + prefix + `claim_until IS NULL OR ` + prefix + `claim_until <= ?)
))`
}

// normalizeRelayClaimRequest validates required fields and defaults Now to the
// current time when zero.
func normalizeRelayClaimRequest(req outbox.ClaimRequest) (outbox.ClaimRequest, error) {
	if req.Owner == "" {
		return outbox.ClaimRequest{}, errors.New("xevent outbox claim owner is required")
	}
	if req.Limit <= 0 {
		return outbox.ClaimRequest{}, errors.New("xevent outbox claim limit must be > 0")
	}
	if req.ClaimTTL <= 0 {
		return outbox.ClaimRequest{}, errors.New("xevent outbox claim ttl must be > 0")
	}
	req.Now = NormalizeTime(req.Now, time.Now)
	return req, nil
}

// NormalizeRelayClaimRequest exposes relay request normalization to package-local tests.
func NormalizeRelayClaimRequest(req outbox.ClaimRequest) (outbox.ClaimRequest, error) {
	return normalizeRelayClaimRequest(req)
}

// normalizeRelayMarkSentRequest validates owner, defaults Now and SentAt when
// zero, and normalises times to UTC.
func normalizeRelayMarkSentRequest(req outbox.MarkSentRequest) (outbox.MarkSentRequest, error) {
	if req.Owner == "" {
		return outbox.MarkSentRequest{}, errors.New("xevent outbox claim owner is required")
	}
	req.Now = NormalizeTime(req.Now, time.Now)
	if req.SentAt.IsZero() {
		req.SentAt = req.Now
	} else {
		req.SentAt = req.SentAt.UTC()
	}
	return req, nil
}

// NormalizeRelayMarkSentRequest exposes relay request normalization to package-local tests.
func NormalizeRelayMarkSentRequest(req outbox.MarkSentRequest) (outbox.MarkSentRequest, error) {
	return normalizeRelayMarkSentRequest(req)
}

// normalizeRelayRetryRequest validates owner, defaults Now and NextAvailableAt
// when zero, and normalises times to UTC.
func normalizeRelayRetryRequest(req outbox.RetryRequest) (outbox.RetryRequest, error) {
	if req.Owner == "" {
		return outbox.RetryRequest{}, errors.New("xevent outbox claim owner is required")
	}
	req.Now = NormalizeTime(req.Now, time.Now)
	if req.NextAvailableAt.IsZero() {
		req.NextAvailableAt = req.Now
	} else {
		req.NextAvailableAt = req.NextAvailableAt.UTC()
	}
	return req, nil
}

// NormalizeRelayRetryRequest exposes relay request normalization to package-local tests.
func NormalizeRelayRetryRequest(req outbox.RetryRequest) (outbox.RetryRequest, error) {
	return normalizeRelayRetryRequest(req)
}

// normalizeRelayFailRequest validates owner and defaults Now to the current time.
func normalizeRelayFailRequest(req outbox.FailRequest) (outbox.FailRequest, error) {
	if req.Owner == "" {
		return outbox.FailRequest{}, errors.New("xevent outbox claim owner is required")
	}
	req.Now = NormalizeTime(req.Now, time.Now)
	return req, nil
}

// NormalizeRelayFailRequest exposes relay request normalization to package-local tests.
func NormalizeRelayFailRequest(req outbox.FailRequest) (outbox.FailRequest, error) {
	return normalizeRelayFailRequest(req)
}

// normalizeCutoverRequest validates BatchSize and defaults Now to the current time.
func normalizeCutoverRequest(req CutoverRequest) (CutoverRequest, error) {
	if req.BatchSize <= 0 {
		return CutoverRequest{}, errors.New("xevent outbox cutover batch size must be > 0")
	}
	req.Now = NormalizeTime(req.Now, time.Now)
	return req, nil
}

// recordIDs extracts the ID from each record into a flat slice.
func recordIDs(records []DBRecord) []uint64 {
	ids := make([]uint64, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	return ids
}

// quotedTable returns the dialect-quoted table name suitable for raw SQL.
func (s *GORMStore) quotedTable(tx *gorm.DB) string {
	stmt := &gorm.Statement{DB: tx}
	return stmt.Quote(s.tableName)
}

// session returns a GORM session for the given context, preferring
// SessionFromContext when configured.
func (s *GORMStore) session(ctx context.Context) *gorm.DB {
	ctx = NormalizeContext(ctx)
	if s.sessionFromContext != nil {
		if session := s.sessionFromContext(ctx); session != nil {
			return session.WithContext(ctx)
		}
	}
	return s.db.WithContext(ctx)
}
