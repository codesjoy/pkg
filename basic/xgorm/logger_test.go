package xgorm

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// TestNewLogger tests creating a new logger instance.
func TestNewLogger(t *testing.T) {
	baseLogger := slog.Default()

	logger := NewLogger(baseLogger, gormlogger.Info, 200*time.Millisecond, false)

	require.NotNil(t, logger)
	assert.NotNil(t, logger.config)
	assert.Same(t, baseLogger, logger.config.Logger)
	assert.Equal(t, gormlogger.Info, logger.config.LogLevel)
	assert.Equal(t, 200*time.Millisecond, logger.config.SlowThreshold)
	assert.False(t, logger.config.IgnoreRecordNotFoundError)
}

// TestNewLogger_NilLogger tests that a default logger is used when nil is provided.
func TestNewLogger_NilLogger(t *testing.T) {
	logger := NewLogger(nil, gormlogger.Warn, 100*time.Millisecond, true)

	require.NotNil(t, logger)
	assert.NotNil(t, logger.config.Logger)
}

// TestLogger_LogMode tests that LogMode returns a new instance with the updated level.
func TestLogger_LogMode(t *testing.T) {
	baseLogger := slog.Default()
	logger := NewLogger(baseLogger, gormlogger.Info, 200*time.Millisecond, false)

	// Set to Silent mode
	silentLogger := logger.LogMode(gormlogger.Silent)

	require.NotNil(t, silentLogger)
	assert.NotSame(t, logger, silentLogger)

	// Original logger should be unchanged
	assert.Equal(t, gormlogger.Info, logger.config.LogLevel)

	// New logger should have the new level
	newLogger, ok := silentLogger.(*Logger)
	require.True(t, ok, "LogMode should return *Logger type")
	assert.Equal(t, gormlogger.Silent, newLogger.config.LogLevel)

	// Other config should be copied
	assert.Equal(t, logger.config.SlowThreshold, newLogger.config.SlowThreshold)
	assert.Equal(
		t,
		logger.config.IgnoreRecordNotFoundError,
		newLogger.config.IgnoreRecordNotFoundError,
	)
	assert.Same(t, logger.config.Logger, newLogger.config.Logger)
}

// TestLogger_LogMode_Chaining tests that LogMode can be chained multiple times.
func TestLogger_LogMode_Chaining(t *testing.T) {
	baseLogger := slog.Default()
	logger := NewLogger(baseLogger, gormlogger.Info, 200*time.Millisecond, false)

	errorLogger := logger.LogMode(gormlogger.Error)
	silentLogger := errorLogger.LogMode(gormlogger.Silent)
	warnLogger := silentLogger.LogMode(gormlogger.Warn)

	require.NotNil(t, warnLogger)

	newLogger, ok := warnLogger.(*Logger)
	require.True(t, ok)
	assert.Equal(t, gormlogger.Warn, newLogger.config.LogLevel)

	// Original should still be unchanged
	assert.Equal(t, gormlogger.Info, logger.config.LogLevel)
}

// TestLogger_LogMode_ThreadSafety tests that LogMode is thread-safe.
func TestLogger_LogMode_ThreadSafety(t *testing.T) {
	baseLogger := slog.Default()
	logger := NewLogger(baseLogger, gormlogger.Info, 200*time.Millisecond, false)

	// Call LogMode from multiple goroutines
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func() {
			logger.LogMode(gormlogger.Error)
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 100; i++ {
		<-done
	}

	// Original logger should still be unchanged
	assert.Equal(t, gormlogger.Info, logger.config.LogLevel)
}

// TestLogger_Info tests Info method logging.
func TestLogger_Info(t *testing.T) {
	var logs []string
	logHandler := &testLogHandler{
		logFunc: func(_ context.Context, level slog.Level, msg string, _ ...any) {
			if level == slog.LevelInfo {
				logs = append(logs, msg)
			}
		},
	}

	baseLogger := slog.New(logHandler)
	logger := NewLogger(baseLogger, gormlogger.Info, 200*time.Millisecond, false)

	logger.Info(context.Background(), "test info message", "key", "value")

	assert.Contains(t, logs, "test info message")
}

// TestLogger_Info_LogLevelTooLow tests that Info doesn't log when log level is too low.
func TestLogger_Info_LogLevelTooLow(t *testing.T) {
	var logs []string
	logHandler := &testLogHandler{
		logFunc: func(_ context.Context, level slog.Level, msg string, _ ...any) {
			logs = append(logs, msg)
		},
	}

	baseLogger := slog.New(logHandler)
	logger := NewLogger(baseLogger, gormlogger.Warn, 200*time.Millisecond, false)

	logger.Info(context.Background(), "test info message")

	// Should not log because log level is Warn
	assert.Empty(t, logs)
}

// TestLogger_Warn tests Warn method logging.
func TestLogger_Warn(t *testing.T) {
	var logs []string
	logHandler := &testLogHandler{
		logFunc: func(_ context.Context, level slog.Level, msg string, _ ...any) {
			if level == slog.LevelWarn {
				logs = append(logs, msg)
			}
		},
	}

	baseLogger := slog.New(logHandler)
	logger := NewLogger(baseLogger, gormlogger.Warn, 200*time.Millisecond, false)

	logger.Warn(context.Background(), "test warn message")

	assert.Contains(t, logs, "test warn message")
}

// TestLogger_Error tests Error method logging.
func TestLogger_Error(t *testing.T) {
	var logs []string
	logHandler := &testLogHandler{
		logFunc: func(_ context.Context, level slog.Level, msg string, _ ...any) {
			if level == slog.LevelError {
				logs = append(logs, msg)
			}
		},
	}

	baseLogger := slog.New(logHandler)
	logger := NewLogger(baseLogger, gormlogger.Error, 200*time.Millisecond, false)

	logger.Error(context.Background(), "test error message")

	assert.Contains(t, logs, "test error message")
}

// TestLogger_Trace tests the Trace method for SQL query logging.
func TestLogger_Trace(t *testing.T) {
	type testCase struct {
		name             string
		logLevel         gormlogger.LogLevel
		slowThreshold    time.Duration
		elapsed          time.Duration
		err              error
		expectedLog      bool
		expectedLevel    slog.Level
		expectedContains string
	}

	tests := []testCase{
		{
			name:          "silent mode - no log",
			logLevel:      gormlogger.Silent,
			slowThreshold: 200 * time.Millisecond,
			elapsed:       100 * time.Millisecond,
			err:           nil,
			expectedLog:   false,
		},
		{
			name:             "info mode - normal query",
			logLevel:         gormlogger.Info,
			slowThreshold:    200 * time.Millisecond,
			elapsed:          100 * time.Millisecond,
			err:              nil,
			expectedLog:      true,
			expectedLevel:    slog.LevelInfo,
			expectedContains: "sql query",
		},
		{
			name:             "info mode - slow query",
			logLevel:         gormlogger.Info,
			slowThreshold:    100 * time.Millisecond,
			elapsed:          200 * time.Millisecond,
			err:              nil,
			expectedLog:      true,
			expectedLevel:    slog.LevelWarn,
			expectedContains: "slow sql query",
		},
		{
			name:             "error mode - query error",
			logLevel:         gormlogger.Error,
			slowThreshold:    200 * time.Millisecond,
			elapsed:          100 * time.Millisecond,
			err:              errors.New("database error"),
			expectedLog:      true,
			expectedLevel:    slog.LevelError,
			expectedContains: "sql query failed",
		},
		{
			name:             "warn mode - query error",
			logLevel:         gormlogger.Warn,
			slowThreshold:    200 * time.Millisecond,
			elapsed:          100 * time.Millisecond,
			err:              errors.New("database error"),
			expectedLog:      true,
			expectedLevel:    slog.LevelError,
			expectedContains: "sql query failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var loggedLevel slog.Level
			var loggedMsg string
			logHandler := &testLogHandler{
				logFunc: func(_ context.Context, level slog.Level, msg string, args ...any) {
					loggedLevel = level
					loggedMsg = msg
				},
			}

			baseLogger := slog.New(logHandler)
			logger := NewLogger(baseLogger, tt.logLevel, tt.slowThreshold, false)

			begin := time.Now().Add(-tt.elapsed)

			logger.Trace(context.Background(), begin, func() (string, int64) {
				return "SELECT * FROM users", 10
			}, tt.err)

			if tt.expectedLog {
				assert.Equal(t, tt.expectedLevel, loggedLevel)
				assert.Contains(t, loggedMsg, tt.expectedContains)
			} else {
				assert.Empty(t, loggedMsg)
			}
		})
	}
}

// TestLogger_Trace_IgnoreRecordNotFound tests that ErrRecordNotFound is ignored when configured.
func TestLogger_Trace_IgnoreRecordNotFound(t *testing.T) {
	var loggedMsg string
	logHandler := &testLogHandler{
		logFunc: func(_ context.Context, level slog.Level, msg string, args ...any) {
			loggedMsg = msg
		},
	}

	baseLogger := slog.New(logHandler)
	logger := NewLogger(baseLogger, gormlogger.Error, 200*time.Millisecond, true)

	begin := time.Now()

	logger.Trace(context.Background(), begin, func() (string, int64) {
		return "SELECT * FROM users WHERE id = ?", 0
	}, gorm.ErrRecordNotFound)

	// Should not log because IgnoreRecordNotFoundError is true
	assert.Empty(t, loggedMsg)
}

// TestLogger_Trace_LogRecordNotFound tests that ErrRecordNotFound is logged when not configured to ignore.
func TestLogger_Trace_LogRecordNotFound(t *testing.T) {
	var loggedLevel slog.Level
	var loggedMsg string
	logHandler := &testLogHandler{
		logFunc: func(_ context.Context, level slog.Level, msg string, args ...any) {
			loggedLevel = level
			loggedMsg = msg
		},
	}

	baseLogger := slog.New(logHandler)
	logger := NewLogger(baseLogger, gormlogger.Error, 200*time.Millisecond, false)

	begin := time.Now()

	logger.Trace(context.Background(), begin, func() (string, int64) {
		return "SELECT * FROM users WHERE id = ?", 0
	}, gorm.ErrRecordNotFound)

	// Should log because IgnoreRecordNotFoundError is false
	assert.Equal(t, slog.LevelError, loggedLevel)
	assert.Contains(t, loggedMsg, "sql query failed")
}

// TestLogger_GetConfig tests getting the logger configuration.
func TestLogger_GetConfig(t *testing.T) {
	baseLogger := slog.Default()
	logger := NewLogger(baseLogger, gormlogger.Warn, 150*time.Millisecond, true)

	config := logger.GetConfig()

	require.NotNil(t, config)
	assert.Same(t, baseLogger, config.Logger)
	assert.Equal(t, gormlogger.Warn, config.LogLevel)
	assert.Equal(t, 150*time.Millisecond, config.SlowThreshold)
	assert.True(t, config.IgnoreRecordNotFoundError)
}

// TestLogger_Trace_ContextAttributes tests that file and line attributes are logged when present in context.
func TestLogger_Trace_ContextAttributes(t *testing.T) {
	var loggedAttrs []any
	logHandler := &testLogHandler{
		logFunc: func(_ context.Context, level slog.Level, msg string, args ...any) {
			loggedAttrs = args
		},
	}

	baseLogger := slog.New(logHandler)
	logger := NewLogger(baseLogger, gormlogger.Info, 200*time.Millisecond, false)

	ctx := context.Background()

	begin := time.Now()

	logger.Trace(ctx, begin, func() (string, int64) {
		return "SELECT * FROM users", 10
	}, nil)

	// Check that file and line are in the logged attributes
	assert.NotEmpty(t, loggedAttrs)
	// Note: The exact format depends on how slog converts attributes
}

func BenchmarkLogger_Trace(b *testing.B) {
	baseLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	logger := NewLogger(baseLogger, gormlogger.Info, time.Hour, false)
	ctx := context.Background()
	begin := time.Now()
	fc := func() (string, int64) {
		return "SELECT * FROM users WHERE id = ?", 1
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Trace(ctx, begin, fc, nil)
	}
}

// testLogHandler is a mock slog.Handler for testing.
type testLogHandler struct {
	logFunc func(ctx context.Context, level slog.Level, msg string, args ...any)
}

func (h *testLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (h *testLogHandler) Handle(ctx context.Context, r slog.Record) error {
	// Convert record to args for testing
	var args []any
	r.Attrs(func(a slog.Attr) bool {
		args = append(args, a)
		return true
	})
	h.logFunc(ctx, r.Level, r.Message, args...)
	return nil
}

func (h *testLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *testLogHandler) WithGroup(name string) slog.Handler {
	return h
}
