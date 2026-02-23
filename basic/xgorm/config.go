package xgorm

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	tracerplugin "github.com/codesjoy/pkg/basic/xgorm/plugin/tracer"
)

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
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxLifetime int // in seconds

	// gormConfigMutators are applied after building the default gorm.Config.
	gormConfigMutators []func(*gorm.Config)
}

// DefaultConfig returns a configuration with sensible defaults.
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
		gormConfigMutators:     make([]func(*gorm.Config), 0),
	}
}

// New creates a new GORM instance with the given dialector and options.
// It applies configuration options and registers the tracer plugin.
//
// The logger is set directly in gorm.Config (not as a plugin) for better integration.
// Metrics are registered as OpenTelemetry callbacks (not as a plugin).
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
	cfg := DefaultConfig()

	// Apply functional options
	for _, opt := range opts {
		opt(cfg)
	}

	// Create GORM config with the logger
	gormConfig := &gorm.Config{
		DryRun:                 cfg.DryRun,
		SkipDefaultTransaction: cfg.SkipDefaultTransaction,
		Logger:                 cfg.Logger,
	}
	for _, mutate := range cfg.gormConfigMutators {
		mutate(gormConfig)
	}

	// Open database connection
	db, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		return nil, err
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	if cfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)
	}

	cleanup := func() {
		_ = CloseMetrics(db)
		_ = sqlDB.Close()
	}

	// Register OpenTelemetry metrics
	if cfg.EnableMetrics {
		if cfg.Meter == nil {
			cleanup()
			return nil, ErrNilMeter
		}
		reporter, err := RegisterConnectionPoolMetrics(cfg.Meter, db, nil)
		if err != nil {
			cleanup()
			return nil, err
		}
		// Store the reporter in the DB instance for later cleanup
		db.InstanceSet("otel_metrics_reporter", reporter)
	}

	// Register tracer plugin (only remaining plugin)
	if cfg.EnableTracer && cfg.Tracer != nil {
		tracerPlugin := tracerplugin.New(cfg.Tracer)
		if err := db.Use(tracerPlugin); err != nil {
			cleanup()
			return nil, err
		}
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
