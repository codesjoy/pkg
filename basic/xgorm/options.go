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
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/plugin/dbresolver"
	"gorm.io/sharding"
)

// Option is a function that configures a GORM instance.
// It follows the functional options pattern for flexible configuration.
type Option func(*Config)

// ensureLoggerForOption lazily initializes cfg.Logger with sensible defaults
// and returns the logger. Used by options that need to modify logger settings.
func ensureLoggerForOption(cfg *Config) *Logger {
	if cfg.Logger == nil {
		cfg.Logger = NewLogger(nil, gormlogger.Info, 200*time.Millisecond, false)
	}
	return cfg.Logger
}

// newDBResolverRule creates a dbResolverRule with a defensive copy of the datas slice
// to prevent accidental mutation of the caller's slice.
func newDBResolverRule(config dbresolver.Config, datas []any) dbResolverRule {
	return dbResolverRule{
		Config: config,
		Datas:  append([]any(nil), datas...),
	}
}

// WithLoggerConfig creates a GORM logger with the specified configuration.
// This replaces the old WithLogger, WithLogLevel, and WithSlowThreshold options.
//
// Parameters:
//   - level: The GORM log level (Silent, Error, Warn, Info)
//   - slowThreshold: Duration threshold for slow query logging
//   - ignoreNotFound: Whether to ignore gorm.ErrRecordNotFound errors
//
// Example:
//
//	db, err := xgorm.New(
//	    sqlite.Open("test.db"),
//	    xgorm.WithLoggerConfig(gormlogger.Info, 200*time.Millisecond, false),
//	)
func WithLoggerConfig(
	level gormlogger.LogLevel,
	slowThreshold time.Duration,
	ignoreNotFound bool,
) Option {
	return func(cfg *Config) {
		cfg.Logger = NewLogger(slog.Default(), level, slowThreshold, ignoreNotFound)
	}
}

// WithSlogLogger sets a custom slog.Logger for the GORM logger.
// This is a convenience option for using a custom slog logger.
//
// Example:
//
//	customLogger := slog.New(myHandler)
//	db, err := xgorm.New(
//	    sqlite.Open("test.db"),
//	    xgorm.WithSlogLogger(customLogger),
//	    xgorm.WithLoggerConfig(gormlogger.Info, 200*time.Millisecond, false),
//	)
func WithSlogLogger(logger *slog.Logger) Option {
	return func(cfg *Config) {
		if cfg.Logger == nil {
			cfg.Logger = NewLogger(logger, gormlogger.Info, 200*time.Millisecond, false)
			return
		}
		ensureLoggerForOption(cfg).config.Logger = logger
	}
}

// WithMeter sets the OpenTelemetry meter for connection pool metrics.
// This replaces the old WithMetrics option.
//
// Example:
//
//	meter := otel.Meter("github.com/codesjoy/pkg/basic/xgorm")
//	db, err := xgorm.New(
//	    sqlite.Open("test.db"),
//	    xgorm.WithMeter(meter),
//	)
//	defer xgorm.CloseMetrics(db)
func WithMeter(meter metric.Meter) Option {
	return func(cfg *Config) {
		cfg.Meter = meter
		cfg.EnableMetrics = true
	}
}

// WithTracer enables distributed tracing using OpenTelemetry.
// The tracer will create spans for each database operation.
//
// Example:
//
//	tracer := otel.Tracer("github.com/codesjoy/pkg/basic/xgorm")
//	db, err := xgorm.New(
//	    sqlite.Open("test.db"),
//	    xgorm.WithTracer(tracer),
//	)
func WithTracer(tracer trace.Tracer) Option {
	return func(cfg *Config) {
		cfg.Tracer = tracer
		cfg.EnableTracer = true
	}
}

// WithDryRun enables GORM's dry run mode.
// In dry run mode, SQL statements are not executed.
// Useful for testing and debugging.
func WithDryRun(enable bool) Option {
	return func(cfg *Config) {
		cfg.DryRun = enable
	}
}

// WithSkipDefaultTransaction disables GORM's default transaction mode.
// When enabled, each write operation will run in its own transaction.
func WithSkipDefaultTransaction(enable bool) Option {
	return func(cfg *Config) {
		cfg.SkipDefaultTransaction = enable
	}
}

// WithMaxIdleConns sets the maximum number of idle connections in the pool.
func WithMaxIdleConns(n int) Option {
	return func(cfg *Config) {
		cfg.MaxIdleConns = n
	}
}

// WithMaxOpenConns sets the maximum number of open connections to the database.
func WithMaxOpenConns(n int) Option {
	return func(cfg *Config) {
		cfg.MaxOpenConns = n
	}
}

// WithConnMaxLifetime sets the maximum amount of time a connection may be reused.
// The value is in seconds. 0 means connections are reused forever.
func WithConnMaxLifetime(seconds int) Option {
	return func(cfg *Config) {
		cfg.ConnMaxLifetime = seconds
	}
}

// WithGormLogger allows passing a pre-configured Logger instance directly.
// This provides maximum flexibility for custom logger configurations.
//
// Example:
//
//	logger := xgorm.NewLogger(mySlogLogger, gormlogger.Warn, 100*time.Millisecond, true)
//	db, err := xgorm.New(
//	    sqlite.Open("test.db"),
//	    xgorm.WithGormLogger(logger),
//	)
func WithGormLogger(logger *Logger) Option {
	return func(cfg *Config) {
		cfg.Logger = logger
	}
}

// WithGormConfig allows passing a custom GORM config function.
// This is useful for advanced GORM configurations not covered by other options.
//
// Example:
//
//	db, err := xgorm.New(
//	    sqlite.Open("test.db"),
//	    xgorm.WithGormConfig(func(cfg *gorm.Config) {
//	        cfg.DisableForeignKeyConstraintWhenMigrating = true
//	    }),
//	)
func WithGormConfig(fn func(*gorm.Config)) Option {
	return func(cfg *Config) {
		if fn == nil {
			return
		}
		cfg.gormConfigMutators = append(cfg.gormConfigMutators, fn)
	}
}

// WithSharding enables table sharding for the specified tables.
//
// It uses gorm.io/sharding as the underlying plugin.
//
// Note: When sharding is enabled, PrepareStmt mode is not supported.
func WithSharding(config sharding.Config, tables ...any) Option {
	return func(cfg *Config) {
		c := config
		cfg.ShardingConfig = &c
		cfg.ShardingTables = append([]any(nil), tables...)
	}
}

// WithDBResolver registers a dbresolver routing rule.
//
// Multiple calls are supported and will be registered in order.
// It uses gorm.io/plugin/dbresolver as the underlying plugin.
func WithDBResolver(config dbresolver.Config, datas ...any) Option {
	return func(cfg *Config) {
		cfg.dbResolverRules = append(cfg.dbResolverRules, newDBResolverRule(config, datas))
	}
}

// WithDBResolverConnPool sets connection pool settings for all dbresolver pools.
//
// This option requires at least one WithDBResolver(...) rule.
func WithDBResolverConnPool(
	maxIdleConns int,
	maxOpenConns int,
	connMaxLifetime time.Duration,
	connMaxIdleTime time.Duration,
) Option {
	return func(cfg *Config) {
		cfg.dbResolverConnPool = &dbResolverConnPoolConfig{
			MaxIdleConns:    maxIdleConns,
			MaxOpenConns:    maxOpenConns,
			ConnMaxLifetime: connMaxLifetime,
			ConnMaxIdleTime: connMaxIdleTime,
		}
	}
}
