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

//go:build integration

package integration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/codesjoy/pkg/basic/xevent/outbox/internal/shared"
	outbox "github.com/codesjoy/pkg/basic/xevent/outbox/relay"
	outboxgorm "github.com/codesjoy/pkg/basic/xevent/outbox/relay/gorm"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	mysqlc "github.com/testcontainers/testcontainers-go/modules/mysql"
	postgresc "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	mysqlgorm "gorm.io/driver/mysql"
	postgresgorm "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	postgresImage           = "postgres:15-alpine"
	mysqlImage              = "mysql:8.4"
	integrationStartupLimit = 2 * time.Minute
	integrationShutdownWait = 30 * time.Second
	integrationTestTimeout  = 30 * time.Second

	testDBName     = "xevent_outbox_integration"
	testDBUser     = "xevent"
	testDBPassword = "xevent"
)

var (
	postgresHarness *dbHarness
	mysqlHarness    *dbHarness
)

type dbHarness struct {
	dsn       string
	terminate func(context.Context) error
}

func TestMain(m *testing.M) {
	startupCtx, startupCancel := context.WithTimeout(context.Background(), integrationStartupLimit)
	defer startupCancel()

	var err error
	postgresHarness, err = startPostgresHarness(startupCtx)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to start postgres harness: %v\n", err)
		os.Exit(1)
	}

	mysqlHarness, err = startMySQLHarness(startupCtx)
	if err != nil {
		_ = postgresHarness.terminate(context.Background())
		_, _ = fmt.Fprintf(os.Stderr, "failed to start mysql harness: %v\n", err)
		os.Exit(1)
	}

	exitCode := m.Run()

	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		integrationShutdownWait,
	)
	defer shutdownCancel()
	var closeErr error
	if postgresHarness != nil {
		closeErr = errors.Join(closeErr, postgresHarness.terminate(shutdownCtx))
	}
	if mysqlHarness != nil {
		closeErr = errors.Join(closeErr, mysqlHarness.terminate(shutdownCtx))
	}
	if closeErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to stop integration harnesses: %v\n", closeErr)
		if exitCode == 0 {
			exitCode = 1
		}
	}

	os.Exit(exitCode)
}

func TestGORMStoreClaimAcrossDialects(t *testing.T) {
	tests := []struct {
		name    string
		openDB  func(*testing.T) *gorm.DB
		dialect outboxgorm.GORMStoreDialect
	}{
		{name: "postgres auto detect", openDB: mustPostgresDB},
		{
			name:    "postgres standard override",
			openDB:  mustPostgresDB,
			dialect: outboxgorm.GORMStoreDialectStandard,
		},
		{name: "mysql auto detect", openDB: mustMySQLDB},
		{
			name:    "mysql standard override",
			openDB:  mustMySQLDB,
			dialect: outboxgorm.GORMStoreDialectStandard,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := tt.openDB(t)
			resetOutboxTable(t, db)

			now := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)
			records := []outbox.Record{
				{
					EventType:    "evt",
					EventID:      "a1",
					PartitionKey: "a",
					Payload:      []byte("a1"),
					Status:       outbox.StatusPending,
					AvailableAt:  now.Add(-time.Minute),
				},
				{
					EventType:    "evt",
					EventID:      "a2",
					PartitionKey: "a",
					Payload:      []byte("a2"),
					Status:       outbox.StatusPending,
					AvailableAt:  now,
				},
				{
					EventType:    "evt",
					EventID:      "b1",
					PartitionKey: "b",
					Payload:      []byte("b1"),
					Status:       outbox.StatusPending,
					AvailableAt:  now.Add(-2 * time.Minute),
				},
				{
					EventType:    "evt",
					EventID:      "future",
					PartitionKey: "c",
					Payload:      []byte("c"),
					Status:       outbox.StatusPending,
					AvailableAt:  now.Add(time.Hour),
				},
			}
			createRecords(t, db, records)

			store, err := outboxgorm.NewGORMStore(outboxgorm.GORMStoreConfig{
				DB:      db,
				Dialect: tt.dialect,
			})
			require.NoError(t, err)

			claimed, err := store.Claim(context.Background(), outbox.ClaimRequest{
				Owner:    "relay-1",
				Now:      now,
				ClaimTTL: time.Minute,
				Limit:    10,
			})
			require.NoError(t, err)
			require.Len(t, claimed, 2)
			require.Equal(t, "b1", claimed[0].EventID)
			require.Equal(t, "a1", claimed[1].EventID)
			for _, record := range claimed {
				require.Equal(t, outbox.StatusSending, record.Status)
				require.Equal(t, 1, record.Attempts)
				require.Equal(t, "relay-1", record.ClaimOwner)
				require.NotNil(t, record.ClaimUntil)
				require.True(t, record.ClaimUntil.Equal(now.Add(time.Minute)))
			}

			require.NoError(t, store.MarkSent(context.Background(), outbox.MarkSentRequest{
				ID:     claimed[1].ID,
				Owner:  "relay-1",
				Now:    now,
				SentAt: now,
			}))

			nextClaimed, err := store.Claim(context.Background(), outbox.ClaimRequest{
				Owner:    "relay-2",
				Now:      now,
				ClaimTTL: time.Minute,
				Limit:    10,
			})
			require.NoError(t, err)
			require.Len(t, nextClaimed, 1)
			require.Equal(t, "a2", nextClaimed[0].EventID)
		})
	}
}

func TestGORMStoreAutoMigrateCreatesMinimalIndexesAcrossDialects(t *testing.T) {
	tests := []struct {
		name   string
		openDB func(*testing.T) *gorm.DB
	}{
		{name: "postgres", openDB: mustPostgresDB},
		{name: "mysql", openDB: mustMySQLDB},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := tt.openDB(t)
			resetOutboxTable(t, db)

			indexes := outboxIndexes(t, db)
			require.Equal(
				t,
				[]string{"mode", "status", "partition_key", "available_at", "id"},
				indexes["idx_xevent_outbox_mode_status_partition_available_id"],
			)
			require.Equal(t, []string{"mode", "created_at"}, indexes["idx_xevent_outbox_mode_created_at"])
			require.Equal(t, []string{"handoff_from_id"}, indexes["idx_xevent_outbox_handoff_from_id"])

			delete(indexes, "idx_xevent_outbox_mode_status_partition_available_id")
			delete(indexes, "idx_xevent_outbox_mode_created_at")
			delete(indexes, "idx_xevent_outbox_handoff_from_id")
			require.Empty(t, indexes)
		})
	}
}

func TestGORMStoreClaimReclaimsExpiredSendingAcrossDialects(t *testing.T) {
	tests := []struct {
		name    string
		openDB  func(*testing.T) *gorm.DB
		dialect outboxgorm.GORMStoreDialect
	}{
		{name: "postgres auto detect", openDB: mustPostgresDB},
		{
			name:    "postgres standard override",
			openDB:  mustPostgresDB,
			dialect: outboxgorm.GORMStoreDialectStandard,
		},
		{name: "mysql auto detect", openDB: mustMySQLDB},
		{
			name:    "mysql standard override",
			openDB:  mustMySQLDB,
			dialect: outboxgorm.GORMStoreDialectStandard,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := tt.openDB(t)
			resetOutboxTable(t, db)

			now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
			expired := now.Add(-time.Minute)
			record := outbox.Record{
				EventType:    "evt",
				EventID:      "stale",
				PartitionKey: "same",
				Payload:      []byte("payload"),
				Status:       outbox.StatusSending,
				Attempts:     1,
				ClaimOwner:   "relay-old",
				ClaimUntil:   &expired,
				AvailableAt:  now.Add(-2 * time.Minute),
			}
			insertRelayRecord(t, db, &record)

			store, err := outboxgorm.NewGORMStore(outboxgorm.GORMStoreConfig{
				DB:      db,
				Dialect: tt.dialect,
			})
			require.NoError(t, err)

			claimed, err := store.Claim(context.Background(), outbox.ClaimRequest{
				Owner:    "relay-new",
				Now:      now,
				ClaimTTL: 2 * time.Minute,
				Limit:    1,
			})
			require.NoError(t, err)
			require.Len(t, claimed, 1)
			require.Equal(t, "stale", claimed[0].EventID)
			require.Equal(t, 2, claimed[0].Attempts)
			require.Equal(t, "relay-new", claimed[0].ClaimOwner)
		})
	}
}

func TestGORMStoreClaimSkipsLockedRowsForDialectSpecificStrategies(t *testing.T) {
	tests := []struct {
		name   string
		openDB func(*testing.T) *gorm.DB
	}{
		{name: "postgres auto detect", openDB: mustPostgresDB},
		{name: "mysql auto detect", openDB: mustMySQLDB},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := tt.openDB(t)
			resetOutboxTable(t, db)

			now := time.Date(2026, 3, 26, 14, 0, 0, 0, time.UTC)
			records := []outbox.Record{
				{
					EventType:    "evt",
					EventID:      "a1",
					PartitionKey: "a",
					Payload:      []byte("a1"),
					Status:       outbox.StatusPending,
					AvailableAt:  now.Add(-2 * time.Minute),
				},
				{
					EventType:    "evt",
					EventID:      "a2",
					PartitionKey: "a",
					Payload:      []byte("a2"),
					Status:       outbox.StatusPending,
					AvailableAt:  now.Add(-time.Minute),
				},
				{
					EventType:    "evt",
					EventID:      "b1",
					PartitionKey: "b",
					Payload:      []byte("b1"),
					Status:       outbox.StatusPending,
					AvailableAt:  now.Add(-time.Minute),
				},
			}
			createRecords(t, db, records)

			var earliest outbox.Record
			require.NoError(t, db.Where("event_id = ?", "a1").First(&earliest).Error)

			locker := db.Begin()
			require.NoError(t, locker.Error)
			t.Cleanup(func() {
				if locker.Error == nil {
					_ = locker.Rollback().Error
				}
			})

			var lockedID uint64
			require.NoError(
				t,
				locker.Raw(
					fmt.Sprintf(
						"SELECT id FROM %s WHERE id = ? FOR UPDATE",
						quotedTable(locker, recordTableName()),
					),
					earliest.ID,
				).Scan(&lockedID).Error,
			)
			require.Equal(t, earliest.ID, lockedID)

			store, err := outboxgorm.NewGORMStore(outboxgorm.GORMStoreConfig{DB: db})
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			claimed, err := store.Claim(ctx, outbox.ClaimRequest{
				Owner:    "relay-skip",
				Now:      now,
				ClaimTTL: time.Minute,
				Limit:    10,
			})
			require.NoError(t, err)
			require.Len(t, claimed, 1)
			require.Equal(t, "b1", claimed[0].EventID)

			require.NoError(t, locker.Rollback().Error)
		})
	}
}

func startPostgresHarness(ctx context.Context) (*dbHarness, error) {
	container, err := postgresc.Run(
		ctx,
		postgresImage,
		postgresc.WithDatabase(testDBName),
		postgresc.WithUsername(testDBUser),
		postgresc.WithPassword(testDBPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(integrationStartupLimit),
		),
	)
	if err != nil {
		return nil, err
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, err
	}

	if err := waitForDB(ctx, "pgx", dsn); err != nil {
		_ = container.Terminate(ctx)
		return nil, err
	}

	return &dbHarness{
		dsn: dsn,
		terminate: func(stopCtx context.Context) error {
			return container.Terminate(stopCtx)
		},
	}, nil
}

func startMySQLHarness(ctx context.Context) (*dbHarness, error) {
	container, err := mysqlc.Run(
		ctx,
		mysqlImage,
		mysqlc.WithDatabase(testDBName),
		mysqlc.WithUsername(testDBUser),
		mysqlc.WithPassword(testDBPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("ready for connections").
				WithOccurrence(1).
				WithStartupTimeout(integrationStartupLimit),
		),
	)
	if err != nil {
		return nil, err
	}

	dsn, err := container.ConnectionString(ctx, "parseTime=true", "multiStatements=true")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, err
	}

	if err := waitForDB(ctx, "mysql", dsn); err != nil {
		_ = container.Terminate(ctx)
		return nil, err
	}

	return &dbHarness{
		dsn: dsn,
		terminate: func(stopCtx context.Context) error {
			return container.Terminate(stopCtx)
		},
	}, nil
}

func waitForDB(ctx context.Context, driverName, dsn string) error {
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	deadline := time.Now().Add(45 * time.Second)
	consecutiveSuccess := 0
	for {
		if err := db.PingContext(ctx); err == nil {
			consecutiveSuccess++
			if consecutiveSuccess >= 3 {
				return nil
			}
		} else {
			consecutiveSuccess = 0
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("database %s did not become ready in time", driverName)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func mustPostgresDB(t *testing.T) *gorm.DB {
	t.Helper()
	require.NotNil(t, postgresHarness)
	db, err := gorm.Open(postgresgorm.Open(postgresHarness.dsn), &gorm.Config{})
	require.NoError(t, err)
	configureSQLDB(t, db)
	return db
}

func mustMySQLDB(t *testing.T) *gorm.DB {
	t.Helper()
	require.NotNil(t, mysqlHarness)
	db, err := gorm.Open(mysqlgorm.Open(mysqlHarness.dsn), &gorm.Config{})
	require.NoError(t, err)
	configureSQLDB(t, db)
	return db
}

func configureSQLDB(t *testing.T, db *gorm.DB) {
	t.Helper()

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Minute)

	pingCtx, cancel := context.WithTimeout(context.Background(), integrationTestTimeout)
	defer cancel()
	require.NoError(t, sqlDB.PingContext(pingCtx))

	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})
}

func resetOutboxTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Migrator().DropTable(&shared.DBRecord{}))
	require.NoError(t, db.AutoMigrate(&shared.DBRecord{}))
}

func createRecords(t *testing.T, db *gorm.DB, records []outbox.Record) {
	t.Helper()
	for i := range records {
		insertRelayRecord(t, db, &records[i])
	}
}

func insertRelayRecord(t *testing.T, db *gorm.DB, record *outbox.Record) {
	t.Helper()
	stored := shared.RelayRecordToDBRecord(*record, time.Now().UTC())
	require.NoError(t, db.Table(record.TableName()).Create(&stored).Error)
	*record = shared.DBRecordToRelayRecord(stored)
}

func recordTableName() string {
	return (outbox.Record{}).TableName()
}

func quotedTable(db *gorm.DB, tableName string) string {
	stmt := &gorm.Statement{DB: db}
	return stmt.Quote(tableName)
}

func outboxIndexes(t *testing.T, db *gorm.DB) map[string][]string {
	t.Helper()

	switch db.Name() {
	case "postgres", "postgresql", "pgx":
		return postgresOutboxIndexes(t, db)
	case "mysql", "mariadb":
		return mySQLOutboxIndexes(t, db)
	default:
		t.Fatalf("unsupported integration dialect for index introspection: %s", db.Name())
		return nil
	}
}

func postgresOutboxIndexes(t *testing.T, db *gorm.DB) map[string][]string {
	t.Helper()

	type indexRow struct {
		Name string `gorm:"column:indexname"`
		Def  string `gorm:"column:indexdef"`
	}

	var rows []indexRow
	require.NoError(
		t,
		db.Raw(
			`SELECT indexname, indexdef
FROM pg_indexes
WHERE schemaname = CURRENT_SCHEMA()
  AND tablename = ?`,
			recordTableName(),
		).Scan(&rows).Error,
	)

	got := make(map[string][]string)
	for _, row := range rows {
		if row.Name == recordTableName()+"_pkey" {
			continue
		}
		got[row.Name] = parseIndexColumns(t, row.Def)
	}

	return got
}

func mySQLOutboxIndexes(t *testing.T, db *gorm.DB) map[string][]string {
	t.Helper()

	type indexRow struct {
		KeyName    string `gorm:"column:Key_name"`
		ColumnName string `gorm:"column:Column_name"`
	}

	var rows []indexRow
	require.NoError(
		t,
		db.Raw(fmt.Sprintf("SHOW INDEX FROM %s", quotedTable(db, recordTableName()))).Scan(&rows).Error,
	)

	got := make(map[string][]string)
	for _, row := range rows {
		if row.KeyName == "PRIMARY" {
			continue
		}
		got[row.KeyName] = append(got[row.KeyName], row.ColumnName)
	}

	return got
}

func parseIndexColumns(t *testing.T, indexDef string) []string {
	t.Helper()

	start := strings.Index(indexDef, "(")
	if start < 0 {
		t.Fatalf("failed to parse index definition: %s", indexDef)
	}
	end := strings.Index(indexDef[start+1:], ")")
	if end < 0 {
		t.Fatalf("failed to parse index definition: %s", indexDef)
	}

	columns := strings.Split(indexDef[start+1:start+1+end], ",")
	for i := range columns {
		columns[i] = strings.Trim(columns[i], " \"")
	}
	return columns
}
