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

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// MetricsReporter handles the lifecycle of OpenTelemetry metrics registration.
// It provides a Stop() method to unregister callbacks when closing the database.
type MetricsReporter struct {
	// registered holds the registration for cleanup
	registered []metric.Registration
}

// Stop unregisters all metrics callbacks.
// Should be called when closing the database to clean up resources.
func (mr *MetricsReporter) Stop(_ context.Context) error {
	for _, reg := range mr.registered {
		if err := reg.Unregister(); err != nil {
			return err
		}
	}
	return nil
}

// RegisterConnectionPoolMetrics registers OpenTelemetry metrics for monitoring database connection pool.
//
// It tracks the following metrics from sql.DBStats:
//
//	Gauges (current values):
//	  - db.sql.pool.open_connections: Total number of open connections
//	  - db.sql.pool.in_use_connections: Number of connections currently in use
//	  - db.sql.pool.idle_connections: Number of idle connections
//
//	Counters (cumulative values):
//	  - db.sql.pool.wait_count: Total number of connection wait attempts
//	  - db.sql.pool.wait_duration_ms: Total time waited for connections (milliseconds)
//	  - db.sql.pool.max_idle_closed: Total connections closed due to max idle limit
//	  - db.sql.pool.max_lifetime_closed: Total connections closed due to max lifetime
//
// Parameters:
//   - meter: OpenTelemetry meter for creating instruments
//   - db: *gorm.DB instance to monitor
//   - attrs: Optional attributes to add to all metrics (e.g., database name, system type)
//
// Returns:
//   - *MetricsReporter: Call Stop() when closing the database
//   - error: Returns ErrNilMeter or ErrNilDatabase if parameters are nil
//
// Example:
//
//	meter := otel.Meter("github.com/codesjoy/pkg/basic/xgorm")
//	db, err := xgorm.New(postgres.Open(dsn), xgorm.WithMeter(meter))
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// When shutting down:
//	if err := xgorm.CloseMetrics(db); err != nil {
//	    log.Printf("Failed to close metrics: %v", err)
//	}
func RegisterConnectionPoolMetrics(
	meter metric.Meter,
	db gormDB,
	attrs []attribute.KeyValue,
) (*MetricsReporter, error) {
	if meter == nil {
		return nil, ErrNilMeter
	}
	if db == nil {
		return nil, ErrNilDatabase
	}

	reporter := &MetricsReporter{
		registered: make([]metric.Registration, 0),
	}

	// Get the underlying *sql.DB
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// Create gauges for current connection pool values
	openConnections, err := meter.Int64ObservableGauge(
		"db.sql.pool.open_connections",
		metric.WithDescription("Number of open connections to the database"),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, err
	}

	inUseConnections, err := meter.Int64ObservableGauge(
		"db.sql.pool.in_use_connections",
		metric.WithDescription("Number of connections currently in use"),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, err
	}

	idleConnections, err := meter.Int64ObservableGauge(
		"db.sql.pool.idle_connections",
		metric.WithDescription("Number of idle connections in the pool"),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, err
	}

	// Create counters for cumulative connection pool values
	waitCount, err := meter.Int64ObservableCounter(
		"db.sql.pool.wait_count",
		metric.WithDescription("Total number of times a connection wait was needed"),
		metric.WithUnit("{wait}"),
	)
	if err != nil {
		return nil, err
	}

	waitDuration, err := meter.Int64ObservableCounter(
		"db.sql.pool.wait_duration_ms",
		metric.WithDescription("Total time waited for new connections (milliseconds)"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	maxIdleClosed, err := meter.Int64ObservableCounter(
		"db.sql.pool.max_idle_closed",
		metric.WithDescription("Total number of connections closed due to max idle limit"),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, err
	}

	maxLifetimeClosed, err := meter.Int64ObservableCounter(
		"db.sql.pool.max_lifetime_closed",
		metric.WithDescription("Total number of connections closed due to max lifetime limit"),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, err
	}

	// Register callback function to observe metrics
	callback := func(_ context.Context, observer metric.Observer) error {
		stats := sqlDB.Stats()

		// Convert attributes to ObserveOption
		opts := []metric.ObserveOption{}
		if len(attrs) > 0 {
			opts = append(opts, metric.WithAttributes(attrs...))
		}

		// Observe gauge values (current state)
		observer.ObserveInt64(openConnections, int64(stats.OpenConnections), opts...)
		observer.ObserveInt64(inUseConnections, int64(stats.InUse), opts...)
		observer.ObserveInt64(idleConnections, int64(stats.Idle), opts...)

		// Observe counter values (cumulative)
		observer.ObserveInt64(waitCount, stats.WaitCount, opts...)
		observer.ObserveInt64(waitDuration, stats.WaitDuration.Milliseconds(), opts...)
		observer.ObserveInt64(maxIdleClosed, stats.MaxIdleClosed, opts...)
		observer.ObserveInt64(maxLifetimeClosed, stats.MaxLifetimeClosed, opts...)

		return nil
	}

	// Register the callback with all instruments
	reg, err := meter.RegisterCallback(
		callback,
		openConnections,
		inUseConnections,
		idleConnections,
		waitCount,
		waitDuration,
		maxIdleClosed,
		maxLifetimeClosed,
	)
	if err != nil {
		return nil, err
	}

	reporter.registered = append(reporter.registered, reg)

	return reporter, nil
}

// gormDB is a minimal interface to get *sql.DB from *gorm.DB.
// This avoids importing the full gorm package in tests.
type gormDB interface {
	DB() (*sql.DB, error)
}

// Ensure *gorm.DB implements gormDB at compile time.
// This will fail to compile if *gorm.DB doesn't have the DB() method.
