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

package xgorm

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/plugin/dbresolver"
	"gorm.io/sharding"
)

type ShardingOrder struct {
	ID      int64 `gorm:"primaryKey"`
	UserID  int64
	Product string
}

func (ShardingOrder) TableName() string {
	return "orders"
}

type ResolverUser struct {
	ID   int64 `gorm:"primaryKey"`
	Name string
}

func (ResolverUser) TableName() string {
	return "resolver_users"
}

type ResolverOrder struct {
	ID      int64 `gorm:"primaryKey"`
	Product string
}

func (ResolverOrder) TableName() string {
	return "resolver_orders"
}

func createShardingTables(t *testing.T, db *gorm.DB, table string, shardCount int) {
	t.Helper()
	for i := 0; i < shardCount; i++ {
		err := db.Exec(
			fmt.Sprintf(
				"CREATE TABLE IF NOT EXISTS %s_%d (id INTEGER PRIMARY KEY, user_id INTEGER, product TEXT)",
				table,
				i,
			),
		).Error
		require.NoError(t, err)
	}
}

func openSQLiteForTest(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	return db
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.NotNil(t, cfg)
	assert.NotNil(t, cfg.Logger)
	assert.False(t, cfg.EnableTracer)
	assert.False(t, cfg.EnableMetrics)
	assert.False(t, cfg.DryRun)
	assert.False(t, cfg.SkipDefaultTransaction)
	assert.Equal(t, 10, cfg.MaxIdleConns)
	assert.Equal(t, 100, cfg.MaxOpenConns)
	assert.Equal(t, 3600, cfg.ConnMaxLifetime)
	assert.Nil(t, cfg.ShardingConfig)
	assert.Empty(t, cfg.ShardingTables)
	assert.Empty(t, cfg.dbResolverRules)
	assert.Nil(t, cfg.dbResolverConnPool)
}

func TestNew_Basic(t *testing.T) {
	db, err := New(sqlite.Open(":memory:"))
	require.NoError(t, err)
	assert.NotNil(t, db)

	// Should be able to use the database
	err = db.Exec("SELECT 1").Error
	assert.NoError(t, err)
}

func TestNew_WithLoggerConfig(t *testing.T) {
	db, err := New(
		sqlite.Open(":memory:"),
		WithLoggerConfig(gormlogger.Info, 200*time.Millisecond, false),
	)
	require.NoError(t, err)
	assert.NotNil(t, db)
}

func TestNew_WithMeter(t *testing.T) {
	// WithMeter(nil) sets EnableMetrics to true but meter is nil
	// This should cause an error when trying to register metrics
	_, err := New(
		sqlite.Open(":memory:"),
		WithMeter(nil),
	)
	// Should error because meter is nil and EnableMetrics is true
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNilMeter)
}

func TestNew_WithRealMeter(t *testing.T) {
	meter := metricnoop.NewMeterProvider().Meter("test")

	db, err := New(
		sqlite.Open(":memory:"),
		WithMeter(meter),
	)
	require.NoError(t, err)
	assert.NotNil(t, db)
	assert.NoError(t, CloseMetrics(db))
}

func TestRegisterMetrics_WithRealMeter(t *testing.T) {
	db := openSQLiteForTest(t, ":memory:").Session(&gorm.Session{Initialized: true})
	cfg := DefaultConfig()
	cfg.EnableMetrics = true
	cfg.Meter = metricnoop.NewMeterProvider().Meter("test")

	require.NoError(t, registerMetrics(db, cfg))

	reporter, ok := db.InstanceGet("otel_metrics_reporter")
	require.True(t, ok)
	assert.IsType(t, &MetricsReporter{}, reporter)
	assert.NoError(t, CloseMetrics(db))
}

func TestNew_WithTracer(t *testing.T) {
	mockTracer := noop.NewTracerProvider().Tracer("test")
	db, err := New(
		sqlite.Open(":memory:"),
		WithTracer(mockTracer),
	)
	require.NoError(t, err)
	assert.NotNil(t, db)
}

func TestNew_WithConnectionPool(t *testing.T) {
	db, err := New(
		sqlite.Open(":memory:"),
		WithMaxIdleConns(5),
		WithMaxOpenConns(20),
		WithConnMaxLifetime(1800), // 30 minutes
	)
	require.NoError(t, err)
	assert.NotNil(t, db)

	// Can't directly verify MaxIdleConns/MaxOpenConns via Stats()
	// but we can verify the database is functional
	err = db.Exec("SELECT 1").Error
	assert.NoError(t, err)
}

func TestNew_WithDryRun(t *testing.T) {
	db, err := New(
		sqlite.Open(":memory:"),
		WithDryRun(true),
	)
	require.NoError(t, err)
	assert.NotNil(t, db)

	// In dry run mode, queries should not execute
	type TestModel struct {
		ID   uint
		Name string
	}
	err = db.Table("test_table").Create(&TestModel{Name: "test"}).Error
	assert.NoError(t, err, "Dry run should not error")
}

func TestNew_WithSkipDefaultTransaction(t *testing.T) {
	db, err := New(
		sqlite.Open(":memory:"),
		WithSkipDefaultTransaction(true),
	)
	require.NoError(t, err)
	assert.NotNil(t, db)
}

func TestNew_AllOptions(t *testing.T) {
	mockTracer := noop.NewTracerProvider().Tracer("test")

	db, err := New(
		sqlite.Open(":memory:"),
		WithLoggerConfig(gormlogger.Warn, 150*time.Millisecond, false),
		WithTracer(mockTracer),
		WithDryRun(false),
		WithSkipDefaultTransaction(false),
		WithMaxIdleConns(15),
		WithMaxOpenConns(50),
		WithConnMaxLifetime(1200),
	)
	require.NoError(t, err)
	assert.NotNil(t, db)

	// Test that database is functional
	err = db.Exec("SELECT 1").Error
	assert.NoError(t, err)
}

func TestMustNew_Success(t *testing.T) {
	db := MustNew(sqlite.Open(":memory:"))
	assert.NotNil(t, db)
}

func TestMustNew_Panic(t *testing.T) {
	assert.Panics(t, func() {
		// Invalid dialector should panic
		MustNew(nil)
	})
}

func TestWithLoggerConfig(t *testing.T) {
	cfg := DefaultConfig()

	opt := WithLoggerConfig(gormlogger.Info, 200*time.Millisecond, true)
	opt(cfg)

	assert.NotNil(t, cfg.Logger)
	assert.Equal(t, gormlogger.Info, cfg.Logger.config.LogLevel)
	assert.Equal(t, 200*time.Millisecond, cfg.Logger.config.SlowThreshold)
	assert.True(t, cfg.Logger.config.IgnoreRecordNotFoundError)
}

func TestWithSlogLogger(t *testing.T) {
	cfg := DefaultConfig()
	customLogger := slog.Default()

	// First set logger config
	WithLoggerConfig(gormlogger.Warn, 100*time.Millisecond, false)(cfg)

	// Then set custom slog logger
	opt := WithSlogLogger(customLogger)
	opt(cfg)

	assert.Same(t, customLogger, cfg.Logger.config.Logger)
}

func TestWithMeter(t *testing.T) {
	cfg := DefaultConfig()

	// nil meter for testing - will error when used in New()
	opt := WithMeter(nil)
	opt(cfg)

	assert.True(t, cfg.EnableMetrics)
	assert.Nil(t, cfg.Meter)
}

func TestWithTracer(t *testing.T) {
	cfg := DefaultConfig()
	mockTracer := noop.NewTracerProvider().Tracer("test")

	opt := WithTracer(mockTracer)
	opt(cfg)

	assert.True(t, cfg.EnableTracer)
	assert.NotNil(t, cfg.Tracer)
}

func TestWithDryRun(t *testing.T) {
	cfg := DefaultConfig()

	opt := WithDryRun(true)
	opt(cfg)

	assert.True(t, cfg.DryRun)
}

func TestWithSkipDefaultTransaction(t *testing.T) {
	cfg := DefaultConfig()

	opt := WithSkipDefaultTransaction(true)
	opt(cfg)

	assert.True(t, cfg.SkipDefaultTransaction)
}

func TestWithMaxIdleConns(t *testing.T) {
	cfg := DefaultConfig()

	opt := WithMaxIdleConns(25)
	opt(cfg)

	assert.Equal(t, 25, cfg.MaxIdleConns)
}

func TestWithMaxOpenConns(t *testing.T) {
	cfg := DefaultConfig()

	opt := WithMaxOpenConns(75)
	opt(cfg)

	assert.Equal(t, 75, cfg.MaxOpenConns)
}

func TestWithConnMaxLifetime(t *testing.T) {
	cfg := DefaultConfig()

	opt := WithConnMaxLifetime(7200)
	opt(cfg)

	assert.Equal(t, 7200, cfg.ConnMaxLifetime)
}

func TestWithGormLogger(t *testing.T) {
	cfg := DefaultConfig()
	customLogger := NewLogger(slog.Default(), gormlogger.Error, 500*time.Millisecond, true)

	opt := WithGormLogger(customLogger)
	opt(cfg)

	assert.Same(t, customLogger, cfg.Logger)
}

func TestNew_MultipleOptions(t *testing.T) {
	cfg := DefaultConfig()

	// Apply multiple options
	opts := []Option{
		WithLoggerConfig(gormlogger.Error, 300*time.Millisecond, false),
		WithMaxIdleConns(5),
		WithMaxOpenConns(10),
	}

	for _, opt := range opts {
		opt(cfg)
	}

	assert.Equal(t, gormlogger.Error, cfg.Logger.config.LogLevel)
	assert.Equal(t, 300*time.Millisecond, cfg.Logger.config.SlowThreshold)
	assert.Equal(t, 5, cfg.MaxIdleConns)
	assert.Equal(t, 10, cfg.MaxOpenConns)
}

func TestNew_Integration_WithOptions(t *testing.T) {
	// Test that options are applied correctly
	mockTracer := noop.NewTracerProvider().Tracer("test")

	db, err := New(
		sqlite.Open(":memory:"),
		WithLoggerConfig(gormlogger.Info, 200*time.Millisecond, false),
		WithTracer(mockTracer),
	)
	require.NoError(t, err)
	assert.NotNil(t, db)

	// Create a test table
	type TestTable struct {
		ID   uint
		Name string
	}
	err = db.AutoMigrate(&TestTable{})
	require.NoError(t, err)

	// Perform operations
	item := TestTable{Name: "test"}
	err = db.Create(&item).Error
	require.NoError(t, err)

	var found TestTable
	err = db.First(&found, item.ID).Error
	require.NoError(t, err)
	assert.Equal(t, "test", found.Name)
}

func TestNew_ConnectionPoolDefaults(t *testing.T) {
	db, err := New(sqlite.Open(":memory:"))
	require.NoError(t, err)

	// Verify DB is functional with default settings
	err = db.Exec("SELECT 1").Error
	assert.NoError(t, err)
}

func TestConfig_ConnMaxLifetime(t *testing.T) {
	db, err := New(
		sqlite.Open(":memory:"),
		WithConnMaxLifetime(600), // 10 minutes
	)
	require.NoError(t, err)

	// Verify DB is functional
	err = db.Exec("SELECT 1").Error
	assert.NoError(t, err)
}

func TestWithGormConfig(t *testing.T) {
	db, err := New(
		sqlite.Open(":memory:"),
		WithGormConfig(func(cfg *gorm.Config) {
			cfg.DisableNestedTransaction = true
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, db)
	assert.True(t, db.DisableNestedTransaction)
}

func TestWithGormConfig_NilFunc(t *testing.T) {
	cfg := DefaultConfig()
	WithGormConfig(nil)(cfg)
	assert.Empty(t, cfg.gormConfigMutators)
}

func TestWithGormConfig_MultipleMutators(t *testing.T) {
	db, err := New(
		sqlite.Open(":memory:"),
		WithGormConfig(func(cfg *gorm.Config) {
			cfg.DisableNestedTransaction = true
		}),
		WithGormConfig(func(cfg *gorm.Config) {
			cfg.AllowGlobalUpdate = true
		}),
	)
	require.NoError(t, err)
	assert.NotNil(t, db)
	assert.True(t, db.DisableNestedTransaction)
	assert.True(t, db.AllowGlobalUpdate)
}

func TestWithSharding(t *testing.T) {
	cfg := DefaultConfig()
	opt := WithSharding(sharding.Config{
		ShardingKey:         "user_id",
		NumberOfShards:      4,
		PrimaryKeyGenerator: sharding.PKSnowflake,
	}, &ShardingOrder{})
	opt(cfg)

	require.NotNil(t, cfg.ShardingConfig)
	assert.Equal(t, "user_id", cfg.ShardingConfig.ShardingKey)
	assert.Len(t, cfg.ShardingTables, 1)
}

func TestWithDBResolver(t *testing.T) {
	cfg := DefaultConfig()
	opt := WithDBResolver(dbresolver.Config{
		Sources:  []gorm.Dialector{sqlite.Open(":memory:")},
		Replicas: []gorm.Dialector{sqlite.Open(":memory:")},
	}, &ResolverOrder{})
	opt(cfg)

	assert.Len(t, cfg.dbResolverRules, 1)
	assert.Len(t, cfg.dbResolverRules[0].Datas, 1)
}

func TestWithDBResolverConnPool(t *testing.T) {
	cfg := DefaultConfig()
	opt := WithDBResolverConnPool(11, 22, time.Minute, 2*time.Minute)
	opt(cfg)

	require.NotNil(t, cfg.dbResolverConnPool)
	assert.Equal(t, 11, cfg.dbResolverConnPool.MaxIdleConns)
	assert.Equal(t, 22, cfg.dbResolverConnPool.MaxOpenConns)
	assert.Equal(t, time.Minute, cfg.dbResolverConnPool.ConnMaxLifetime)
	assert.Equal(t, 2*time.Minute, cfg.dbResolverConnPool.ConnMaxIdleTime)
}

func TestNew_WithSharding_Basic(t *testing.T) {
	db, err := New(
		sqlite.Open(":memory:"),
		WithSharding(sharding.Config{
			ShardingKey:         "user_id",
			NumberOfShards:      4,
			PrimaryKeyGenerator: sharding.PKSnowflake,
		}, &ShardingOrder{}),
	)
	require.NoError(t, err)
	require.NotNil(t, db)

	createShardingTables(t, db, "orders", 4)

	err = db.Create(&ShardingOrder{UserID: 5, Product: "phone"}).Error
	require.NoError(t, err)

	var got ShardingOrder
	err = db.Model(&ShardingOrder{}).Where("user_id = ?", 5).First(&got).Error
	require.NoError(t, err)
	assert.Equal(t, "phone", got.Product)
}

func TestNew_WithSharding_MissingShardingKey(t *testing.T) {
	db, err := New(
		sqlite.Open(":memory:"),
		WithSharding(sharding.Config{
			ShardingKey:         "user_id",
			NumberOfShards:      4,
			PrimaryKeyGenerator: sharding.PKSnowflake,
		}, &ShardingOrder{}),
	)
	require.NoError(t, err)

	err = db.Model(&ShardingOrder{}).Where("product = ?", "phone").Find(&[]ShardingOrder{}).Error
	assert.ErrorIs(t, err, sharding.ErrMissingShardingKey)
}

func TestNew_WithSharding_NoTables(t *testing.T) {
	_, err := New(
		sqlite.Open(":memory:"),
		WithSharding(sharding.Config{
			ShardingKey:         "user_id",
			NumberOfShards:      4,
			PrimaryKeyGenerator: sharding.PKSnowflake,
		}),
	)
	assert.ErrorIs(t, err, ErrShardingTablesRequired)
}

func TestNew_WithSharding_PrepareStmtUnsupported(t *testing.T) {
	_, err := New(
		sqlite.Open(":memory:"),
		WithSharding(sharding.Config{
			ShardingKey:         "user_id",
			NumberOfShards:      4,
			PrimaryKeyGenerator: sharding.PKSnowflake,
		}, &ShardingOrder{}),
		WithGormConfig(func(cfg *gorm.Config) {
			cfg.PrepareStmt = true
		}),
	)
	assert.ErrorIs(t, err, ErrShardingPrepareStmtUnsupported)
}

func TestNew_DBResolverConnPoolWithoutRules(t *testing.T) {
	_, err := New(
		sqlite.Open(":memory:"),
		WithDBResolverConnPool(10, 20, time.Minute, time.Minute),
	)
	assert.ErrorIs(t, err, ErrDBResolverNotConfigured)
}

func TestNew_WithDBResolver_GlobalAndPerTable(t *testing.T) {
	tempDir := t.TempDir()
	globalSourcePath := filepath.Join(tempDir, "global_source.db")
	globalReplicaPath := filepath.Join(tempDir, "global_replica.db")
	orderSourcePath := filepath.Join(tempDir, "order_source.db")
	orderReplicaPath := filepath.Join(tempDir, "order_replica.db")

	db, err := New(
		sqlite.Open(globalSourcePath),
		WithDBResolver(dbresolver.Config{
			Sources:  []gorm.Dialector{sqlite.Open(globalSourcePath)},
			Replicas: []gorm.Dialector{sqlite.Open(globalReplicaPath)},
		}),
		WithDBResolver(dbresolver.Config{
			Sources:  []gorm.Dialector{sqlite.Open(orderSourcePath)},
			Replicas: []gorm.Dialector{sqlite.Open(orderReplicaPath)},
		}, &ResolverOrder{}),
		WithDBResolverConnPool(5, 50, time.Minute, time.Minute),
	)
	require.NoError(t, err)

	globalSource := openSQLiteForTest(t, globalSourcePath)
	globalReplica := openSQLiteForTest(t, globalReplicaPath)
	orderSource := openSQLiteForTest(t, orderSourcePath)
	orderReplica := openSQLiteForTest(t, orderReplicaPath)

	for _, handle := range []*gorm.DB{globalSource, globalReplica, orderSource, orderReplica} {
		require.NoError(t, handle.AutoMigrate(&ResolverUser{}, &ResolverOrder{}))
	}

	require.NoError(t, globalSource.Create(&ResolverUser{ID: 1, Name: "global-source"}).Error)
	require.NoError(t, globalReplica.Create(&ResolverUser{ID: 1, Name: "global-replica"}).Error)
	require.NoError(t, orderSource.Create(&ResolverOrder{ID: 1, Product: "order-source"}).Error)
	require.NoError(t, orderReplica.Create(&ResolverOrder{ID: 1, Product: "order-replica"}).Error)

	var user ResolverUser
	err = db.Model(&ResolverUser{}).First(&user, 1).Error
	require.NoError(t, err)
	assert.Equal(t, "global-replica", user.Name)

	var order ResolverOrder
	err = db.Model(&ResolverOrder{}).First(&order, 1).Error
	require.NoError(t, err)
	assert.Equal(t, "order-replica", order.Product)
}

func TestNew_WithShardingAndDBResolver_Combined(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "source.db")
	replicaPath := filepath.Join(tempDir, "replica.db")

	db, err := New(
		sqlite.Open(sourcePath),
		WithSharding(sharding.Config{
			ShardingKey:         "user_id",
			NumberOfShards:      4,
			PrimaryKeyGenerator: sharding.PKSnowflake,
		}, &ShardingOrder{}),
		WithDBResolver(dbresolver.Config{
			Sources:  []gorm.Dialector{sqlite.Open(sourcePath)},
			Replicas: []gorm.Dialector{sqlite.Open(replicaPath)},
		}),
	)
	require.NoError(t, err)

	sourceDB := openSQLiteForTest(t, sourcePath)
	replicaDB := openSQLiteForTest(t, replicaPath)
	createShardingTables(t, sourceDB, "orders", 4)
	createShardingTables(t, replicaDB, "orders", 4)

	require.NoError(
		t,
		sourceDB.Exec("INSERT INTO orders_1(id, user_id, product) VALUES(1, 1, 'source')").Error,
	)
	require.NoError(
		t,
		replicaDB.Exec("INSERT INTO orders_1(id, user_id, product) VALUES(1, 1, 'replica')").Error,
	)

	var got ShardingOrder
	err = db.Model(&ShardingOrder{}).Where("user_id = ?", 1).First(&got).Error
	require.NoError(t, err)
	assert.Equal(t, "replica", got.Product)

	err = db.Model(&ShardingOrder{}).
		Where("user_id = ?", 1).
		Update("product", "updated-source").
		Error
	require.NoError(t, err)

	var sourceRow ShardingOrder
	var replicaRow ShardingOrder
	require.NoError(t, sourceDB.Table("orders_1").Where("id = ?", 1).First(&sourceRow).Error)
	require.NoError(t, replicaDB.Table("orders_1").Where("id = ?", 1).First(&replicaRow).Error)
	assert.Equal(t, "updated-source", sourceRow.Product)
	assert.Equal(t, "replica", replicaRow.Product)
}

func TestCloseMetrics(t *testing.T) {
	t.Run("nil database", func(t *testing.T) {
		require.NoError(t, CloseMetrics(nil))
	})

	t.Run("missing reporter", func(t *testing.T) {
		db := openSQLiteForTest(t, ":memory:").Session(&gorm.Session{Initialized: true})
		require.NoError(t, CloseMetrics(db))
	})

	t.Run("ignores unexpected reporter type", func(t *testing.T) {
		db := openSQLiteForTest(t, ":memory:").Session(&gorm.Session{Initialized: true})
		db.InstanceSet("otel_metrics_reporter", "not-a-reporter")

		require.NoError(t, CloseMetrics(db))
	})

	t.Run("stops reporter successfully", func(t *testing.T) {
		db := openSQLiteForTest(t, ":memory:").Session(&gorm.Session{Initialized: true})
		registration := &stubRegistration{}
		db.InstanceSet("otel_metrics_reporter", &MetricsReporter{
			registered: []metric.Registration{registration},
		})

		require.NoError(t, CloseMetrics(db))
		assert.Equal(t, 1, registration.calls)
	})

	t.Run("propagates reporter stop error", func(t *testing.T) {
		db := openSQLiteForTest(t, ":memory:").Session(&gorm.Session{Initialized: true})
		registration := &stubRegistration{err: assert.AnError}
		db.InstanceSet("otel_metrics_reporter", &MetricsReporter{
			registered: []metric.Registration{registration},
		})

		err := CloseMetrics(db)
		require.ErrorIs(t, err, assert.AnError)
		assert.Equal(t, 1, registration.calls)
	})
}

// Benchmark tests
func BenchmarkNew_Basic(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db, err := New(sqlite.Open(":memory:"))
		if err != nil {
			b.Fatal(err)
		}
		_ = db
	}
}

func BenchmarkNew_WithOptions(b *testing.B) {
	opts := []Option{
		WithLoggerConfig(gormlogger.Info, 200*time.Millisecond, false),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db, err := New(sqlite.Open(":memory:"), opts...)
		if err != nil {
			b.Fatal(err)
		}
		_ = db
	}
}

func BenchmarkNew_WithAllOptions(b *testing.B) {
	mockTracer := noop.NewTracerProvider().Tracer("test")
	opts := []Option{
		WithLoggerConfig(gormlogger.Info, 200*time.Millisecond, false),
		WithTracer(mockTracer),
		WithDryRun(false),
		WithSkipDefaultTransaction(false),
		WithMaxIdleConns(10),
		WithMaxOpenConns(50),
		WithConnMaxLifetime(1800),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db, err := New(sqlite.Open(":memory:"), opts...)
		if err != nil {
			b.Fatal(err)
		}
		_ = db
	}
}
