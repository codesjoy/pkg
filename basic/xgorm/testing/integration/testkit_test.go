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
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/codesjoy/pkg/basic/xgorm"
	postgresdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	postgresc "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	postgresImage           = "postgres:15-alpine"
	postgresStartupTimeout  = 2 * time.Minute
	postgresShutdownTimeout = 30 * time.Second
	integrationTimeout      = 90 * time.Second

	testDBUser      = "xgorm"
	testDBPassword  = "xgorm"
	sourceDBName    = "xgorm_integration_source"
	replicaDBName   = "xgorm_integration_replica"
	defaultShardNum = 4
)

var (
	sourceHarness  *postgresHarness
	replicaHarness *postgresHarness
)

type postgresHarness struct {
	container *postgresc.PostgresContainer
	dsn       string
}

func TestMain(m *testing.M) {
	startupCtx, startupCancel := context.WithTimeout(context.Background(), postgresStartupTimeout)
	defer startupCancel()

	source, err := startPostgresHarness(startupCtx, sourceDBName)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to start source postgres harness: %v\n", err)
		os.Exit(1)
	}
	sourceHarness = source

	replica, err := startPostgresHarness(startupCtx, replicaDBName)
	if err != nil {
		_ = sourceHarness.Close(context.Background())
		_, _ = fmt.Fprintf(os.Stderr, "failed to start replica postgres harness: %v\n", err)
		os.Exit(1)
	}
	replicaHarness = replica

	exitCode := m.Run()

	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		postgresShutdownTimeout,
	)
	defer shutdownCancel()
	var closeErr error
	if sourceHarness != nil {
		closeErr = errors.Join(closeErr, sourceHarness.Close(shutdownCtx))
	}
	if replicaHarness != nil {
		closeErr = errors.Join(closeErr, replicaHarness.Close(shutdownCtx))
	}
	if closeErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to stop postgres harnesses: %v\n", closeErr)
		if exitCode == 0 {
			exitCode = 1
		}
	}

	os.Exit(exitCode)
}

func startPostgresHarness(ctx context.Context, dbName string) (*postgresHarness, error) {
	container, err := postgresc.Run(
		ctx,
		postgresImage,
		postgresc.WithDatabase(dbName),
		postgresc.WithUsername(testDBUser),
		postgresc.WithPassword(testDBPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(postgresStartupTimeout),
		),
	)
	if err != nil {
		return nil, err
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, err
	}

	return &postgresHarness{container: container, dsn: dsn}, nil
}

func (h *postgresHarness) Close(_ context.Context) error {
	if h == nil || h.container == nil {
		return nil
	}
	return testcontainers.TerminateContainer(h.container)
}

func integrationContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), integrationTimeout)
}

func mustSourceDB(t *testing.T) *gorm.DB {
	t.Helper()
	require.NotNil(t, sourceHarness)
	require.NotEmpty(t, sourceHarness.dsn)
	return mustXgormDBByDSN(t, sourceHarness.dsn)
}

func mustReplicaDB(t *testing.T) *gorm.DB {
	t.Helper()
	require.NotNil(t, replicaHarness)
	require.NotEmpty(t, replicaHarness.dsn)
	return mustXgormDBByDSN(t, replicaHarness.dsn)
}

func mustRoutedDB(t *testing.T, opts ...xgorm.Option) *gorm.DB {
	t.Helper()
	require.NotNil(t, sourceHarness)
	require.NotEmpty(t, sourceHarness.dsn)

	db, err := xgorm.New(postgresdriver.Open(sourceHarness.dsn), opts...)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, xgorm.CloseMetrics(db))
		sqlDB, dbErr := db.DB()
		require.NoError(t, dbErr)
		require.NoError(t, sqlDB.Close())
	})

	return db
}

func mustDB(t *testing.T) *gorm.DB {
	t.Helper()
	return mustSourceDB(t)
}

func mustXgormDBByDSN(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := xgorm.New(postgresdriver.Open(dsn))
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, xgorm.CloseMetrics(db))
		sqlDB, dbErr := db.DB()
		require.NoError(t, dbErr)
		require.NoError(t, sqlDB.Close())
	})

	return db
}

func resetTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS xgorm_integration_users").Error)
	require.NoError(t, db.AutoMigrate(&integrationUser{}))
}

func resetResolverTables(t *testing.T, sourceDB, replicaDB *gorm.DB) {
	t.Helper()
	for _, db := range []*gorm.DB{sourceDB, replicaDB} {
		if db == nil {
			continue
		}
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS xgorm_integration_resolver_users").Error)
		require.NoError(t, db.AutoMigrate(&resolverUser{}))
	}
}

func resetShardingTables(t *testing.T, sourceDB, replicaDB *gorm.DB, shardCount int) {
	t.Helper()
	for _, db := range []*gorm.DB{sourceDB, replicaDB} {
		if db == nil {
			continue
		}
		for i := 0; i < shardCount; i++ {
			tableName := fmt.Sprintf("orders_%d", i)
			require.NoError(t, db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)).Error)
			require.NoError(
				t,
				db.Exec(
					fmt.Sprintf(
						"CREATE TABLE %s (id BIGINT PRIMARY KEY, user_id BIGINT, product TEXT)",
						tableName,
					),
				).Error,
			)
		}
	}
}
