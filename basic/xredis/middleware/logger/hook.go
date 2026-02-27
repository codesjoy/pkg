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

func (h *hook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

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

func (h *hook) shouldSkip(cmd redis.Cmder) bool {
	if h.commandFilter == nil || cmd == nil {
		return false
	}
	return h.commandFilter(cmd)
}

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
