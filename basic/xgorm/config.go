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
	"context"
	"database/sql"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/plugin/dbresolver"
	"gorm.io/sharding"

	tracerplugin "github.com/codesjoy/pkg/basic/xgorm/plugin/tracer"
)

// dbResolverRule pairs a dbresolver.Config with the datasources it applies to.
type dbResolverRule struct {
	Config dbresolver.Config
	Datas  []any
}

// dbResolverConnPoolConfig holds connection pool settings for dbresolver pools.
type dbResolverConnPoolConfig struct {
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// Config holds configuration for creating a new GORM instance.
type Config struct {
	// Logger is the GORM logger implementing gorm/logger.Interface.
	// When set, it will be used directly in gorm.Config (not as a plugin).
	Logger *Logger

	// Tracer is the OpenTelemetry tracer for distributed tracing.
	Tracer trace.Tracer
	// EnableTracer enables the tracer plugin.
	EnableTracer bool

	// Meter is the OpenTelemetry meter for metrics collection.
	// When set, connection pool metrics will be registered.
	Meter metric.Meter
	// EnableMetrics enables connection pool metrics.
	EnableMetrics bool

	// DryRun enables SQL dry run mode (no execution).
	DryRun bool
	// SkipDefaultTransaction skips GORM's default transaction mode.
	SkipDefaultTransaction bool

	// Database connection pool settings.
	MaxIdleConns    int // Maximum number of idle connections.
	MaxOpenConns    int // Maximum number of open connections.
	ConnMaxLifetime int // Maximum connection lifetime in seconds.

	// ShardingConfig is the table sharding rule.
	// When set, sharding plugin will be registered for ShardingTables.
	ShardingConfig *sharding.Config
	// ShardingTables are tables/models that use ShardingConfig.
	ShardingTables []any

	// dbResolverRules stores dbresolver register rules in order.
	dbResolverRules []dbResolverRule
	// dbResolverConnPool applies connection pool settings to resolver pools.
	dbResolverConnPool *dbResolverConnPoolConfig

	// gormConfigMutators are applied after building the default gorm.Config.
	gormConfigMutators []func(*gorm.Config)
}

// DefaultConfig returns a configuration with sensible defaults:
// silent logging, connection pool of [10 idle, 100 open, 1h lifetime], and all extras disabled.
func DefaultConfig() *Config {
	return &Config{
		Logger:                 NewLogger(nil, gormlogger.Silent, 200*time.Millisecond, false),
		EnableTracer:           false,
		EnableMetrics:          false,
		DryRun:                 false,
		SkipDefaultTransaction: false,
		MaxIdleConns:           10,
		MaxOpenConns:           100,
		ConnMaxLifetime:        3600, // 1 hour
		dbResolverRules:        make([]dbResolverRule, 0),
		gormConfigMutators:     make([]func(*gorm.Config), 0),
	}
}

// New creates a new GORM instance with the given dialector and options.
// It applies configuration options and registers optional plugins.
//
// The logger is set directly in gorm.Config (not as a plugin) for better integration.
// Metrics are registered as OpenTelemetry callbacks (not as a plugin).
// Sharding/dbresolver are registered as GORM plugins when configured.
//
// Example usage:
//
//	db, err := xgorm.New(
//	    sqlite.Open("test.db"),
//	    xgorm.WithLoggerConfig(gormlogger.Info, 200*time.Millisecond, false),
//	    xgorm.WithMeter(meter),
//	    xgorm.WithTracer(tracer),
//	)
//	defer xgorm.CloseMetrics(db)
func New(dialector gorm.Dialector, opts ...Option) (*gorm.DB, error) {
	// Build configuration from defaults and functional options.
	cfg := DefaultConfig()
	applyOptions(cfg, opts)

	// Construct the GORM config and validate incompatible feature combinations.
	gormConfig := buildGORMConfig(cfg)
	if err := validateConfigCompatibility(cfg, gormConfig); err != nil {
		return nil, err
	}

	// Open the database and configure the connection pool.
	db, cleanup, err := openConfiguredDB(dialector, gormConfig, cfg)
	if err != nil {
		return nil, err
	}
	// Register plugins (sharding, dbresolver) before instrumentation.
	if err := registerConfiguredPlugins(db, cfg); err != nil {
		cleanup()
		return nil, err
	}
	// Register instrumentation (metrics, tracing) after plugins.
	if err := registerConfiguredInstrumentation(db, cfg); err != nil {
		cleanup()
		return nil, err
	}

	return db, nil
}

// MustNew is like New but panics on error.
// Useful for initialization in main/func main.
func MustNew(dialector gorm.Dialector, opts ...Option) *gorm.DB {
	db, err := New(dialector, opts...)
	if err != nil {
		panic(err)
	}
	return db
}

// CloseMetrics unregisters the OpenTelemetry metrics callback.
// Should be called when closing the database to clean up resources.
//
// Example:
//
//	db, err := xgorm.New(...)
//	defer xgorm.CloseMetrics(db)
func CloseMetrics(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if reporter, ok := db.InstanceGet("otel_metrics_reporter"); ok {
		if mr, ok := reporter.(*MetricsReporter); ok {
			return mr.Stop(context.Background())
		}
	}
	return nil
}

// applyOptions applies each functional Option to the Config.
func applyOptions(cfg *Config, opts []Option) {
	for _, opt := range opts {
		opt(cfg)
	}
}

// openConfiguredDB opens a GORM database with the given dialector and config,
// then configures the underlying connection pool. It returns a cleanup function
// that closes metrics and the database.
func openConfiguredDB(
	dialector gorm.Dialector,
	gormConfig *gorm.Config,
	cfg *Config,
) (*gorm.DB, func(), error) {
	// Open the database connection via GORM.
	db, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		return nil, nil, err
	}

	// Access the underlying *sql.DB for pool configuration.
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	configureConnectionPool(sqlDB, cfg)

	// Return a cleanup function for resource teardown on error paths.
	cleanup := func() {
		_ = CloseMetrics(db)
		_ = sqlDB.Close()
	}
	return db, cleanup, nil
}

// buildGORMConfig constructs a gorm.Config from the xgorm Config,
// then applies any user-supplied mutators.
func buildGORMConfig(cfg *Config) *gorm.Config {
	gormConfig := &gorm.Config{
		DryRun:                 cfg.DryRun,
		SkipDefaultTransaction: cfg.SkipDefaultTransaction,
		Logger:                 cfg.Logger,
	}
	applyGORMConfigMutators(gormConfig, cfg.gormConfigMutators)
	return gormConfig
}

// applyGORMConfigMutators applies user-supplied mutator functions to the gorm.Config.
func applyGORMConfigMutators(
	gormConfig *gorm.Config,
	mutators []func(*gorm.Config),
) {
	for _, mutate := range mutators {
		mutate(gormConfig)
	}
}

// validateConfigCompatibility checks for invalid feature combinations,
// such as enabling sharding without tables or with prepared statements.
func validateConfigCompatibility(cfg *Config, gormConfig *gorm.Config) error {
	if cfg.ShardingConfig != nil {
		if len(cfg.ShardingTables) == 0 {
			return ErrShardingTablesRequired
		}
		if gormConfig.PrepareStmt {
			return ErrShardingPrepareStmtUnsupported
		}
	}
	if cfg.dbResolverConnPool != nil && len(cfg.dbResolverRules) == 0 {
		return ErrDBResolverNotConfigured
	}
	return nil
}

// configureConnectionPool applies connection pool settings to the underlying *sql.DB.
func configureConnectionPool(sqlDB *sql.DB, cfg *Config) {
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	if cfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)
	}
}

// registerConfiguredPlugins registers optional GORM plugins (sharding, dbresolver).
func registerConfiguredPlugins(db *gorm.DB, cfg *Config) error {
	if err := registerSharding(db, cfg); err != nil {
		return err
	}
	return registerDBResolver(db, cfg)
}

// registerConfiguredInstrumentation registers metrics and tracing plugins.
func registerConfiguredInstrumentation(db *gorm.DB, cfg *Config) error {
	if err := registerMetrics(db, cfg); err != nil {
		return err
	}
	return registerTracer(db, cfg)
}

// registerSharding registers the sharding plugin when ShardingConfig is set.
// It is registered before dbresolver so read/write routing is applied after table routing.
func registerSharding(db *gorm.DB, cfg *Config) error {
	if cfg.ShardingConfig == nil {
		return nil
	}
	// Register sharding before dbresolver so read/write routing can be applied
	// after table routing in current callback order.
	plugin := sharding.Register(*cfg.ShardingConfig, cfg.ShardingTables...)
	return db.Use(plugin)
}

// registerDBResolver registers the dbresolver plugin when resolver rules are configured.
// It also applies connection pool settings to the resolver if provided.
func registerDBResolver(db *gorm.DB, cfg *Config) error {
	if len(cfg.dbResolverRules) == 0 {
		return nil
	}

	// Build the resolver from configured rules.
	resolver := buildDBResolver(cfg.dbResolverRules)
	// Apply optional connection pool settings for resolver-managed connections.
	if cfg.dbResolverConnPool != nil {
		resolver = resolver.
			SetMaxIdleConns(cfg.dbResolverConnPool.MaxIdleConns).
			SetMaxOpenConns(cfg.dbResolverConnPool.MaxOpenConns).
			SetConnMaxLifetime(cfg.dbResolverConnPool.ConnMaxLifetime).
			SetConnMaxIdleTime(cfg.dbResolverConnPool.ConnMaxIdleTime)
	}
	return db.Use(resolver)
}

// buildDBResolver constructs a dbresolver.DBResolver from the given rules.
// The first rule initializes the resolver; subsequent rules are chained via Register.
func buildDBResolver(rules []dbResolverRule) *dbresolver.DBResolver {
	first := rules[0]
	resolver := dbresolver.Register(first.Config, first.Datas...)
	for _, rule := range rules[1:] {
		resolver = resolver.Register(rule.Config, rule.Datas...)
	}
	return resolver
}

// registerMetrics registers OpenTelemetry connection pool metrics when enabled.
// The reporter is stored on the DB instance for later cleanup via CloseMetrics.
func registerMetrics(db *gorm.DB, cfg *Config) error {
	if !cfg.EnableMetrics {
		return nil
	}
	if cfg.Meter == nil {
		return ErrNilMeter
	}

	reporter, err := RegisterConnectionPoolMetrics(cfg.Meter, db, nil)
	if err != nil {
		return err
	}
	// Store the reporter in the DB instance for later cleanup.
	db.InstanceSet("otel_metrics_reporter", reporter)
	return nil
}

// registerTracer registers the OpenTelemetry tracer plugin when enabled.
func registerTracer(db *gorm.DB, cfg *Config) error {
	if !cfg.EnableTracer || cfg.Tracer == nil {
		return nil
	}
	return db.Use(tracerplugin.New(cfg.Tracer))
}
