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
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

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
