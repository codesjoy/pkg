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
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Logger implements gorm/logger.Interface using slog for structured logging.
//
// It provides GORM-compatible logging with support for:
// - Configurable log levels (Silent, Error, Warn, Info)
// - Slow query detection with configurable threshold
// - Optional filtering of gorm.ErrRecordNotFound errors
// - Thread-safe LogMode() that returns new instances
//
// Example:
//
//	logger := NewLogger(slog.Default(), gormlogger.Info, 200*time.Millisecond, false)
//	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger})
type Logger struct {
	config *LoggerConfig
}

// LoggerConfig holds configuration for the GORM logger.
type LoggerConfig struct {
	// Logger is the slog logger to use for structured logging.
	Logger *slog.Logger

	// LogLevel is the minimum GORM log level to record.
	LogLevel gormlogger.LogLevel

	// SlowThreshold is the duration threshold for slow query logging.
	// Queries taking longer than this are logged at Warn level.
	// Set to 0 to disable slow query logging.
	SlowThreshold time.Duration

	// IgnoreRecordNotFoundError ignores gorm.ErrRecordNotFound errors.
	// When enabled, "record not found" errors are not logged.
	IgnoreRecordNotFoundError bool
}

// NewLogger creates a new GORM logger with slog support.
//
// Parameters:
//   - logger: The slog logger to use (required)
//   - logLevel: The GORM log level (Silent, Error, Warn, Info)
//   - slowThreshold: Duration threshold for slow query logging
//   - ignoreNotFound: Whether to ignore gorm.ErrRecordNotFound errors
//
// Example:
//
//	logger := NewLogger(slog.Default(), gormlogger.Info, 200*time.Millisecond, false)
func NewLogger(
	logger *slog.Logger,
	logLevel gormlogger.LogLevel,
	slowThreshold time.Duration,
	ignoreNotFound bool,
) *Logger {
	if logger == nil {
		logger = slog.Default()
	}

	return &Logger{
		config: &LoggerConfig{
			Logger:                    logger,
			LogLevel:                  logLevel,
			SlowThreshold:             slowThreshold,
			IgnoreRecordNotFoundError: ignoreNotFound,
		},
	}
}

// LogMode sets the log level and returns a new logger instance.
// This implements gorm/logger.Interface and ensures thread safety
// by returning a new instance instead of modifying the existing one.
func (l *Logger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	newLogger := *l
	newLogger.config = &LoggerConfig{
		Logger:                    l.config.Logger,
		LogLevel:                  level,
		SlowThreshold:             l.config.SlowThreshold,
		IgnoreRecordNotFoundError: l.config.IgnoreRecordNotFoundError,
	}
	return &newLogger
}

// Info logs a message at Info level.
func (l *Logger) Info(ctx context.Context, msg string, data ...any) {
	if l.config.LogLevel >= gormlogger.Info {
		if ctx == nil {
			ctx = context.Background()
		}
		l.config.Logger.Log(ctx, slog.LevelInfo, msg, data...)
	}
}

// Warn logs a message at Warn level.
func (l *Logger) Warn(ctx context.Context, msg string, data ...any) {
	if l.config.LogLevel >= gormlogger.Warn {
		if ctx == nil {
			ctx = context.Background()
		}
		l.config.Logger.Log(ctx, slog.LevelWarn, msg, data...)
	}
}

// Error logs a message at Error level.
func (l *Logger) Error(ctx context.Context, msg string, data ...any) {
	if l.config.LogLevel >= gormlogger.Error {
		if ctx == nil {
			ctx = context.Background()
		}
		l.config.Logger.Log(ctx, slog.LevelError, msg, data...)
	}
}

// Trace logs SQL query execution with timing and error information.
// This implements the core logging logic for GORM operations.
func (l *Logger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	// Check if we should log at all based on log level
	if l.config.LogLevel <= gormlogger.Silent {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Execute the callback to get SQL and rows affected
	sql, rows := fc()

	// Calculate duration
	elapsed := time.Since(begin)

	// Determine log level and message
	var level slog.Level
	var msg string

	switch {
	case err != nil && l.config.LogLevel >= gormlogger.Error:
		// Check if we should ignore this error
		if l.config.IgnoreRecordNotFoundError && errors.Is(err, gorm.ErrRecordNotFound) {
			return
		}
		level = slog.LevelError
		msg = "sql query failed"

	case elapsed > l.config.SlowThreshold && l.config.SlowThreshold > 0 && l.config.LogLevel >= gormlogger.Warn:
		level = slog.LevelWarn
		msg = "slow sql query"

	case l.config.LogLevel >= gormlogger.Info:
		level = slog.LevelInfo
		msg = "sql query"

	default:
		return
	}

	// Build attributes using a fixed array to minimize allocations in hot paths.
	var attrs [6]slog.Attr
	n := 0
	attrs[n] = slog.Duration("duration", elapsed)
	n++
	attrs[n] = slog.String("sql", sql)
	n++
	attrs[n] = slog.Int64("rows", rows)
	n++

	// Add file:line if available (from GORM's context)
	if v, ok := ctx.Value("file").(string); ok {
		attrs[n] = slog.String("file", v)
		n++
	}
	if v, ok := ctx.Value("line").(int); ok {
		attrs[n] = slog.Int("line", v)
		n++
	}

	// Add error if present
	if err != nil {
		attrs[n] = slog.String("error", err.Error())
		n++
	}

	l.config.Logger.LogAttrs(ctx, level, msg, attrs[:n]...)
}

// GetConfig returns the current logger configuration.
// This is useful for inspection and testing.
func (l *Logger) GetConfig() *LoggerConfig {
	return l.config
}
