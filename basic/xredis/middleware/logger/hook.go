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

package logger

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultSlowThreshold = 200 * time.Millisecond

// Config controls logger middleware behavior.
type Config struct {
	// Logger is the slog logger instance.
	Logger *slog.Logger
	// SlowThreshold defines the duration above which commands are logged as slow.
	SlowThreshold time.Duration
	// LogArgs controls whether command args are included in logs.
	LogArgs bool
	// CommandFilter returns true to skip logging a command.
	CommandFilter func(redis.Cmder) bool
}

// DefaultConfig returns the default logger middleware config.
func DefaultConfig() Config {
	return Config{
		Logger:        slog.Default(),
		SlowThreshold: defaultSlowThreshold,
		LogArgs:       false,
	}
}

// hook implements redis.Hook for structured logging of Redis commands.
type hook struct {
	logger        *slog.Logger
	slowThreshold time.Duration
	logArgs       bool
	commandFilter func(redis.Cmder) bool
}

var _ redis.Hook = (*hook)(nil)

// New creates a slog-based redis.Hook logger middleware.
func New(cfg Config) redis.Hook {
	normalized := normalizeConfig(cfg)
	return &hook{
		logger:        normalized.Logger,
		slowThreshold: normalized.SlowThreshold,
		logArgs:       normalized.LogArgs,
		commandFilter: normalized.CommandFilter,
	}
}

// DialHook passes through without logging; dial events are typically noisy.
func (h *hook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

// ProcessHook logs errors and slow commands for individual Redis commands.
func (h *hook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		start := time.Now()
		err := next(ctx, cmd)

		if h.shouldSkip(cmd) {
			return err
		}

		duration := time.Since(start)
		attrs := h.commandAttrs(cmd, duration)

		if err != nil {
			attrs = append(attrs, slog.String("error", err.Error()))
			h.logger.ErrorContext(ctx, "xredis command failed", attrs...)
			return err
		}

		if duration >= h.slowThreshold {
			h.logger.WarnContext(ctx, "xredis command slow", attrs...)
		}

		return nil
	}
}

// ProcessPipelineHook logs errors and slow commands for Redis pipeline batches.
func (h *hook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		start := time.Now()
		err := next(ctx, cmds)

		if h.shouldSkipPipeline(cmds) {
			return err
		}

		duration := time.Since(start)
		attrs := h.pipelineAttrs(cmds, duration)

		if err != nil {
			attrs = append(attrs, slog.String("error", err.Error()))
			h.logger.ErrorContext(ctx, "xredis pipeline failed", attrs...)
			return err
		}

		if duration >= h.slowThreshold {
			h.logger.WarnContext(ctx, "xredis pipeline slow", attrs...)
		}

		return nil
	}
}

// shouldSkip returns true when the command should be excluded from logging.
func (h *hook) shouldSkip(cmd redis.Cmder) bool {
	if h.commandFilter == nil || cmd == nil {
		return false
	}
	return h.commandFilter(cmd)
}

// shouldSkipPipeline returns true only when every command in the pipeline
// is filtered out by the command filter.
func (h *hook) shouldSkipPipeline(cmds []redis.Cmder) bool {
	if h.commandFilter == nil || len(cmds) == 0 {
		return false
	}
	for _, cmd := range cmds {
		if cmd == nil {
			continue
		}
		if !h.commandFilter(cmd) {
			return false
		}
	}
	return true
}

// commandAttrs builds slog attributes for a single command log entry.
func (h *hook) commandAttrs(cmd redis.Cmder, duration time.Duration) []any {
	attrs := make([]any, 0, 4)
	attrs = append(attrs,
		slog.String("command", commandName(cmd)),
		slog.Duration("duration", duration),
	)
	if h.logArgs && cmd != nil {
		attrs = append(attrs, slog.Any("args", cmd.Args()))
	}
	return attrs
}

// pipelineAttrs builds slog attributes for a pipeline log entry.
func (h *hook) pipelineAttrs(cmds []redis.Cmder, duration time.Duration) []any {
	names := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		names = append(names, commandName(cmd))
	}

	attrs := make([]any, 0, 5)
	attrs = append(attrs,
		slog.Int("command_count", len(cmds)),
		slog.Any("commands", names),
		slog.Duration("duration", duration),
	)

	if h.logArgs {
		args := make([][]any, 0, len(cmds))
		for _, cmd := range cmds {
			if cmd == nil {
				args = append(args, nil)
				continue
			}
			args = append(args, cmd.Args())
		}
		attrs = append(attrs, slog.Any("args", args))
	}

	return attrs
}

// commandName returns the Redis command name, falling back to the type name
// when neither FullName nor Name is available.
func commandName(cmd redis.Cmder) string {
	if cmd == nil {
		return "unknown"
	}
	if cmd.FullName() != "" {
		return cmd.FullName()
	}
	if cmd.Name() != "" {
		return cmd.Name()
	}
	return fmt.Sprintf("%T", cmd)
}

// normalizeConfig fills zero-valued fields with defaults.
func normalizeConfig(cfg Config) Config {
	normalized := cfg
	if normalized.Logger == nil {
		normalized.Logger = slog.Default()
	}
	if normalized.SlowThreshold <= 0 {
		normalized.SlowThreshold = defaultSlowThreshold
	}
	return normalized
}
