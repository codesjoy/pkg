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
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/codesjoy/pkg/basic/xevent/outbox/debezium"
	debeziumgorm "github.com/codesjoy/pkg/basic/xevent/outbox/debezium/gorm"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	postgresc "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	postgresgorm "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	postgresImage           = "postgres:15-alpine"
	integrationStartupLimit = 2 * time.Minute
	integrationShutdownWait = 30 * time.Second
	integrationTestTimeout  = 30 * time.Second
	testDBName              = "xevent_debezium_outbox_integration"
	testDBUser              = "xevent"
	testDBPassword          = "xevent"
)

var postgresHarness *dbHarness

type dbHarness struct {
	dsn       string
	terminate func(context.Context) error
}

type txContextKey struct{}

type testEvent struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (*testEvent) EventType() string { return "order.created" }
func (e *testEvent) EventID() string { return e.ID }
func (e *testEvent) PartitionKey() string {
	return e.Key
}
func (*testEvent) Topic() string { return "" }
func (e *testEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(e)
}

func (e *testEvent) UnmarshalPayload(data []byte) error {
	return json.Unmarshal(data, e)
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

	exitCode := m.Run()

	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		integrationShutdownWait,
	)
	defer shutdownCancel()
	if postgresHarness != nil {
		if err := postgresHarness.terminate(shutdownCtx); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to stop postgres harness: %v\n", err)
			if exitCode == 0 {
				exitCode = 1
			}
		}
	}

	os.Exit(exitCode)
}

func TestAppendRollbackAndDeleteBefore(t *testing.T) {
	db := mustPostgresDB(t)
	resetTable(t, db)

	store, err := debeziumgorm.NewGORMStore(debeziumgorm.GORMStoreConfig{
		DB:                 db,
		SessionFromContext: transactionFromContext,
	})
	require.NoError(t, err)

	var committed *debezium.Record
	err = db.Transaction(func(tx *gorm.DB) error {
		ctx := withTransaction(context.Background(), tx)

		var appendErr error
		committed, appendErr = debezium.AppendEvent(ctx, store, &testEvent{
			ID:    "evt_1",
			Key:   "order-1",
			Value: "committed",
		}, debezium.AppendOptions{Topic: "orders"})
		if appendErr != nil {
			return appendErr
		}

		var count int64
		if err := tx.Model(&debezium.Record{}).Where("id = ?", committed.ID).Count(&count).Error; err != nil {
			return err
		}
		require.EqualValues(t, 1, count)
		return nil
	})
	require.NoError(t, err)
	require.NotNil(t, committed)

	err = db.Transaction(func(tx *gorm.DB) error {
		ctx := withTransaction(context.Background(), tx)
		_, err := debezium.AppendEvent(ctx, store, &testEvent{
			ID:    "evt_rollback",
			Key:   "order-2",
			Value: "rolled-back",
		}, debezium.AppendOptions{Topic: "orders"})
		if err != nil {
			return err
		}
		return context.Canceled
	})
	require.ErrorIs(t, err, context.Canceled)

	var count int64
	require.NoError(
		t,
		db.Model(&debezium.Record{}).Where("event_id = ?", "evt_rollback").Count(&count).Error,
	)
	require.EqualValues(t, 0, count)

	oldRecord := debezium.Record{
		ID:           "old",
		Topic:        "orders",
		PartitionKey: "order-3",
		EventType:    "order.created",
		EventID:      "evt_old",
		Payload:      []byte("old"),
		CreatedAt:    time.Now().UTC().Add(-48 * time.Hour),
	}
	require.NoError(t, db.Create(&oldRecord).Error)

	deleted, err := store.DeleteBefore(
		context.Background(),
		time.Now().UTC().Add(-24*time.Hour),
		10,
	)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)

	require.NoError(
		t,
		db.Model(&debezium.Record{}).Where("id = ?", oldRecord.ID).Count(&count).Error,
	)
	require.EqualValues(t, 0, count)
}

func withTransaction(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txContextKey{}, tx)
}

func transactionFromContext(ctx context.Context) *gorm.DB {
	tx, _ := ctx.Value(txContextKey{}).(*gorm.DB)
	return tx
}

func mustPostgresDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgresgorm.Open(postgresHarness.dsn), &gorm.Config{})
	require.NoError(t, err)
	configureSQLDB(t, db)
	return db
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

func resetTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Migrator().DropTable(&debezium.Record{}))
	require.NoError(t, db.AutoMigrate(&debezium.Record{}))
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
		_ = container.Terminate(context.Background())
		return nil, err
	}
	if err := waitForDB(ctx, "pgx", dsn); err != nil {
		_ = container.Terminate(context.Background())
		return nil, err
	}

	return &dbHarness{
		dsn: dsn,
		terminate: func(ctx context.Context) error {
			return container.Terminate(ctx)
		},
	}, nil
}
