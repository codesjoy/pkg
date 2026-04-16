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

	"github.com/codesjoy/pkg/basic/xevent/outbox/internal/shared"
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
	// DB is the GORM database handle used for all operations.
	DB *gorm.DB
	// TableName overrides the default outbox table name when non-empty.
	TableName string
	// Dialect selects the SQL strategy; auto-detected when empty.
	Dialect GORMStoreDialect
	// SessionFromContext optionally extracts a per-request GORM session from ctx.
	SessionFromContext func(context.Context) *gorm.DB
}

// GORMStore persists relay outbox records through the shared store.
type GORMStore struct {
	db                 *gorm.DB
	store              *shared.GORMStore
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

	dialect, err := resolveGORMStoreDialect(cfg.DB, cfg.Dialect)
	if err != nil {
		return nil, err
	}

	store, err := shared.NewGORMStore(shared.GORMStoreConfig{
		DB:                 cfg.DB,
		TableName:          tableName,
		Dialect:            shared.Dialect(dialect),
		SessionFromContext: cfg.SessionFromContext,
	})
	if err != nil {
		return nil, err
	}

	return &GORMStore{
		db:                 cfg.DB,
		store:              store,
		tableName:          tableName,
		dialect:            dialect,
		sessionFromContext: cfg.SessionFromContext,
	}, nil
}

// Append inserts one outbox record using the current GORM handle.
func (s *GORMStore) Append(ctx context.Context, record *outbox.Record) error {
	store, err := s.sharedStore()
	if err != nil {
		return err
	}
	return store.AppendRelay(ctx, record)
}

// Claim reserves one batch of eligible records ordered by available time then id.
func (s *GORMStore) Claim(ctx context.Context, req outbox.ClaimRequest) ([]outbox.Record, error) {
	store, err := s.sharedStore()
	if err != nil {
		return nil, err
	}
	return store.ClaimRelay(ctx, req)
}

// MarkSent marks one claimed record as sent.
func (s *GORMStore) MarkSent(ctx context.Context, req outbox.MarkSentRequest) error {
	store, err := s.sharedStore()
	if err != nil {
		return err
	}
	return store.MarkRelaySent(ctx, req)
}

// Retry requeues one claimed record for a later attempt.
func (s *GORMStore) Retry(ctx context.Context, req outbox.RetryRequest) error {
	store, err := s.sharedStore()
	if err != nil {
		return err
	}
	return store.RetryRelay(ctx, req)
}

// MarkFailed marks one claimed record as permanently failed.
func (s *GORMStore) MarkFailed(ctx context.Context, req outbox.FailRequest) error {
	store, err := s.sharedStore()
	if err != nil {
		return err
	}
	return store.MarkRelayFailed(ctx, req)
}

// claimMySQL delegates to the shared store's MySQL-specific relay claim path.
func (s *GORMStore) claimMySQL(
	tx *gorm.DB,
	req outbox.ClaimRequest,
	claimUntil time.Time,
) ([]outbox.Record, error) {
	store, err := s.sharedStore()
	if err != nil {
		return nil, err
	}
	claimed, err := store.ClaimRelayMySQLForTest(tx, req, claimUntil)
	if err != nil {
		return nil, err
	}
	return relayRecordsFromDBRecords(claimed), nil
}

// claimPostgres delegates to the shared store's PostgreSQL-specific relay claim path.
func (s *GORMStore) claimPostgres(
	tx *gorm.DB,
	req outbox.ClaimRequest,
	claimUntil time.Time,
) ([]outbox.Record, error) {
	store, err := s.sharedStore()
	if err != nil {
		return nil, err
	}
	claimed, err := store.ClaimRelayPostgresForTest(tx, req, claimUntil)
	if err != nil {
		return nil, err
	}
	return relayRecordsFromDBRecords(claimed), nil
}

// selectClaimCandidates delegates to the shared store's raw candidate query helper.
func (s *GORMStore) selectClaimCandidates(
	tx *gorm.DB,
	sqlText string,
	args ...any,
) ([]outbox.Record, error) {
	store, err := s.sharedStore()
	if err != nil {
		return nil, err
	}
	candidates, err := store.SelectClaimCandidatesForTest(tx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	return relayRecordsFromDBRecords(candidates), nil
}

// claimSelectedRecords delegates to the shared store's optimistic claim update helper.
func (s *GORMStore) claimSelectedRecords(
	tx *gorm.DB,
	ids []uint64,
	req outbox.ClaimRequest,
	claimUntil time.Time,
) ([]outbox.Record, error) {
	store, err := s.sharedStore()
	if err != nil {
		return nil, err
	}
	claimed, err := store.ClaimSelectedRelayRecordsForTest(tx, ids, req, claimUntil)
	if err != nil {
		return nil, err
	}
	return relayRecordsFromDBRecords(claimed), nil
}

// normalizeClaimRequest delegates to shared.NormalizeRelayClaimRequest.
func normalizeClaimRequest(req outbox.ClaimRequest) (outbox.ClaimRequest, error) {
	return shared.NormalizeRelayClaimRequest(req)
}

// normalizeMarkSentRequest delegates to shared.NormalizeRelayMarkSentRequest.
func normalizeMarkSentRequest(req outbox.MarkSentRequest) (outbox.MarkSentRequest, error) {
	return shared.NormalizeRelayMarkSentRequest(req)
}

// normalizeRetryRequest delegates to shared.NormalizeRelayRetryRequest.
func normalizeRetryRequest(req outbox.RetryRequest) (outbox.RetryRequest, error) {
	return shared.NormalizeRelayRetryRequest(req)
}

// normalizeFailRequest delegates to shared.NormalizeRelayFailRequest.
func normalizeFailRequest(req outbox.FailRequest) (outbox.FailRequest, error) {
	return shared.NormalizeRelayFailRequest(req)
}

// normalizeContext delegates to shared.NormalizeContext.
func normalizeContext(ctx context.Context) context.Context {
	return shared.NormalizeContext(ctx)
}

// cloneBytes returns a shallow-independent copy of src.
func cloneBytes(src []byte) []byte {
	if src == nil {
		return nil
	}
	return append([]byte(nil), src...)
}

// cloneRecord returns a deep copy of the record via a round-trip through the
// shared DBRecord layer.
func cloneRecord(record outbox.Record) outbox.Record {
	return shared.DBRecordToRelayRecord(shared.RelayRecordToDBRecord(record, time.Now().UTC()))
}

// prepareStoredRecord converts a relay Record through the shared DBRecord
// layer and back, applying defaults and normalising timestamps.
func prepareStoredRecord(record outbox.Record, now time.Time) outbox.Record {
	return shared.DBRecordToRelayRecord(shared.RelayRecordToDBRecord(record, now))
}

// relayRecordsFromDBRecords converts a slice of shared DBRecords to relay Records.
func relayRecordsFromDBRecords(records []shared.DBRecord) []outbox.Record {
	if len(records) == 0 {
		return nil
	}
	converted := make([]outbox.Record, 0, len(records))
	for _, record := range records {
		converted = append(converted, shared.DBRecordToRelayRecord(record))
	}
	return converted
}

// sharedStore lazily initialises and returns the underlying shared.GORMStore.
func (s *GORMStore) sharedStore() (*shared.GORMStore, error) {
	if s.store != nil {
		return s.store, nil
	}
	if s.db == nil {
		return nil, errors.New("xevent outbox gorm db is nil")
	}
	store, err := shared.NewGORMStore(shared.GORMStoreConfig{
		DB:                 s.db,
		TableName:          s.tableName,
		Dialect:            shared.Dialect(s.dialect),
		SessionFromContext: s.sessionFromContext,
	})
	if err != nil {
		return nil, err
	}
	s.store = store
	return s.store, nil
}

// resolveGORMStoreDialect returns the normalised dialect when configured is
// non-empty, otherwise auto-detects from db.
func resolveGORMStoreDialect(
	db *gorm.DB,
	configured GORMStoreDialect,
) (GORMStoreDialect, error) {
	if strings.TrimSpace(string(configured)) == "" {
		return detectGORMStoreDialect(db), nil
	}
	return normalizeGORMStoreDialect(configured)
}

// normalizeGORMStoreDialect lowercases and trims the dialect, returning an error
// for unsupported values.
func normalizeGORMStoreDialect(dialect GORMStoreDialect) (GORMStoreDialect, error) {
	normalized := GORMStoreDialect(strings.ToLower(strings.TrimSpace(string(dialect))))
	switch normalized {
	case GORMStoreDialectStandard, GORMStoreDialectMySQL, GORMStoreDialectPostgres:
		return normalized, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedGORMStoreDialect, dialect)
	}
}

// detectGORMStoreDialect inspects the GORM Dialector name and returns the
// matching GORMStoreDialect, defaulting to GORMStoreDialectStandard.
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
