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
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestHookLogsSlowAndErrorOnly(t *testing.T) {
	t.Parallel()

	handler := &memoryHandler{}
	logger := slog.New(handler)
	hook := New(Config{Logger: logger, SlowThreshold: 5 * time.Millisecond})
	process := hook.ProcessHook(func(_ context.Context, _ redis.Cmder) error {
		return nil
	})

	fastCmd := redis.NewCmd(context.Background(), "get", "k")
	require.NoError(t, process(context.Background(), fastCmd))
	require.Len(t, handler.Entries(), 0)

	processSlow := hook.ProcessHook(func(_ context.Context, _ redis.Cmder) error {
		time.Sleep(8 * time.Millisecond)
		return nil
	})
	require.NoError(
		t,
		processSlow(context.Background(), redis.NewCmd(context.Background(), "set", "k", "v")),
	)

	processErr := hook.ProcessHook(func(_ context.Context, _ redis.Cmder) error {
		return errors.New("boom")
	})
	require.Error(
		t,
		processErr(context.Background(), redis.NewCmd(context.Background(), "del", "k")),
	)

	entries := handler.Entries()
	require.Len(t, entries, 2)
	require.Equal(t, "xredis command slow", entries[0].Message)
	require.Equal(t, slog.LevelWarn, entries[0].Level)
	require.NotContains(t, entries[0].Attrs, "args")
	require.Equal(t, "xredis command failed", entries[1].Message)
	require.Equal(t, slog.LevelError, entries[1].Level)
}

func TestHookLogArgsEnabled(t *testing.T) {
	t.Parallel()

	handler := &memoryHandler{}
	logger := slog.New(handler)
	hook := New(Config{
		Logger:        logger,
		SlowThreshold: time.Nanosecond,
		LogArgs:       true,
	})

	process := hook.ProcessHook(func(_ context.Context, _ redis.Cmder) error {
		time.Sleep(2 * time.Millisecond)
		return nil
	})
	require.NoError(
		t,
		process(context.Background(), redis.NewCmd(context.Background(), "set", "k", "v")),
	)

	entries := handler.Entries()
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Attrs, "args")
}

func TestHookCommandFilter(t *testing.T) {
	t.Parallel()

	handler := &memoryHandler{}
	logger := slog.New(handler)
	hook := New(Config{
		Logger:        logger,
		SlowThreshold: time.Nanosecond,
		CommandFilter: func(cmd redis.Cmder) bool {
			return cmd != nil && cmd.Name() == "get"
		},
	})

	process := hook.ProcessHook(func(_ context.Context, _ redis.Cmder) error {
		time.Sleep(2 * time.Millisecond)
		return nil
	})
	require.NoError(
		t,
		process(context.Background(), redis.NewCmd(context.Background(), "get", "k")),
	)
	require.Len(t, handler.Entries(), 0)
}

func TestHookPipelineSlow(t *testing.T) {
	t.Parallel()

	handler := &memoryHandler{}
	logger := slog.New(handler)
	hook := New(Config{Logger: logger, SlowThreshold: 5 * time.Millisecond})
	pipeline := hook.ProcessPipelineHook(func(_ context.Context, _ []redis.Cmder) error {
		time.Sleep(8 * time.Millisecond)
		return nil
	})

	require.NoError(t, pipeline(context.Background(), []redis.Cmder{
		redis.NewCmd(context.Background(), "set", "k", "v"),
		redis.NewCmd(context.Background(), "expire", "k", 10),
	}))

	entries := handler.Entries()
	require.Len(t, entries, 1)
	require.Equal(t, "xredis pipeline slow", entries[0].Message)
	require.NotContains(t, entries[0].Attrs, "args")
}

type memoryEntry struct {
	Message string
	Level   slog.Level
	Attrs   map[string]any
}

type memoryHandler struct {
	state *memoryState
	attrs []slog.Attr
}

type memoryState struct {
	mu      sync.Mutex
	entries []memoryEntry
}

func (h *memoryHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *memoryHandler) Handle(_ context.Context, r slog.Record) error {
	if h.state == nil {
		h.state = &memoryState{}
	}

	attrs := make(map[string]any)
	for _, attr := range h.attrs {
		attrs[attr.Key] = attr.Value.Any()
	}
	r.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})

	h.state.mu.Lock()
	defer h.state.mu.Unlock()
	h.state.entries = append(
		h.state.entries,
		memoryEntry{Message: r.Message, Level: r.Level, Attrs: attrs},
	)
	return nil
}

func (h *memoryHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if h.state == nil {
		h.state = &memoryState{}
	}

	copied := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	copied = append(copied, h.attrs...)
	copied = append(copied, attrs...)
	return &memoryHandler{state: h.state, attrs: copied}
}

func (h *memoryHandler) WithGroup(string) slog.Handler {
	return h
}

func (h *memoryHandler) Entries() []memoryEntry {
	if h.state == nil {
		return nil
	}

	h.state.mu.Lock()
	defer h.state.mu.Unlock()
	cloned := make([]memoryEntry, len(h.state.entries))
	for idx := range h.state.entries {
		entry := h.state.entries[idx]
		attrs := make(map[string]any, len(entry.Attrs))
		for key, value := range entry.Attrs {
			attrs[key] = value
		}
		cloned[idx] = memoryEntry{Message: entry.Message, Level: entry.Level, Attrs: attrs}
	}
	return cloned
}
