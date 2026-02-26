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

package retry

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
)

func TestBackoff(t *testing.T) {
	t.Parallel()

	cfg := NormalizeConfig(Config{
		InitialBackoff: time.Millisecond,
		MaxBackoff:     4 * time.Millisecond,
		Multiplier:     2,
	})

	require.Equal(t, time.Millisecond, Backoff(cfg, 1))
	require.Equal(t, 2*time.Millisecond, Backoff(cfg, 2))
	require.Equal(t, 4*time.Millisecond, Backoff(cfg, 3))
	require.Equal(t, 4*time.Millisecond, Backoff(cfg, 4))
}

func TestMiddlewareRetriesDownstream(t *testing.T) {
	t.Parallel()

	middleware := New(Config{
		MaxRetries:     3,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1,
	}, ExhaustedPolicyBlock, nil, slog.Default())

	attempts := 0
	result, err := middleware.Handle(context.Background(), &produce.MessageContext{},
		func(context.Context, *produce.MessageContext) (*produce.Result, error) {
			attempts++
			if attempts < 3 {
				return nil, errors.New("temporary")
			}
			return &produce.Result{Topic: "orders", Partition: 1, Offset: 9}, nil
		},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 3, attempts)
	require.Equal(t, 3, result.Attempt)
}

func TestMiddlewareCancelInfiniteRetry(t *testing.T) {
	t.Parallel()

	middleware := New(Config{
		MaxRetries:     InfiniteRetries,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1,
	}, ExhaustedPolicyBlock, nil, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	result, err := middleware.Handle(ctx, &produce.MessageContext{},
		func(context.Context, *produce.MessageContext) (*produce.Result, error) {
			return nil, errors.New("always fail")
		},
	)
	require.Nil(t, result)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestMiddlewareStop(t *testing.T) {
	t.Parallel()

	middleware := New(Config{
		MaxRetries:     0,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1,
	}, ExhaustedPolicyStop, nil, slog.Default())

	result, err := middleware.Handle(context.Background(), &produce.MessageContext{},
		func(context.Context, *produce.MessageContext) (*produce.Result, error) {
			return nil, errors.New("failed")
		},
	)
	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exhausted retries")
}

func TestMiddlewareDrop(t *testing.T) {
	t.Parallel()

	middleware := New(Config{
		MaxRetries:     0,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1,
	}, ExhaustedPolicyDrop, nil, slog.Default())

	result, err := middleware.Handle(context.Background(), &produce.MessageContext{},
		func(context.Context, *produce.MessageContext) (*produce.Result, error) {
			return nil, errors.New("failed")
		},
	)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrMessageDropped)
}

func TestMiddlewareFailureHookStages(t *testing.T) {
	t.Parallel()

	stages := make([]FailureStage, 0, 3)
	middleware := New(Config{
		MaxRetries:     0,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1,
	}, ExhaustedPolicyDrop,
		func(_ context.Context, event Event) {
			stages = append(stages, event.Stage)
		},
		slog.Default(),
	)

	_, err := middleware.Handle(context.Background(), &produce.MessageContext{},
		func(context.Context, *produce.MessageContext) (*produce.Result, error) {
			return nil, errors.New("failed")
		},
	)
	require.Error(t, err)
	require.Equal(
		t,
		[]FailureStage{FailureStageRetry, FailureStageExhausted, FailureStageDrop},
		stages,
	)
}

func TestMiddlewareLogsWarnOnRetry(t *testing.T) {
	t.Parallel()

	h := &captureSlogHandler{}
	middleware := New(Config{
		MaxRetries:     1,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1,
	}, ExhaustedPolicyStop, nil, slog.New(h))

	calls := 0
	_, err := middleware.Handle(context.Background(), &produce.MessageContext{},
		func(context.Context, *produce.MessageContext) (*produce.Result, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("temporary")
			}
			return nil, errors.New("failed")
		},
	)
	require.Error(t, err)

	records := h.recordsSnapshot()
	warnCount := 0
	for _, rec := range records {
		if rec.level == slog.LevelWarn {
			warnCount++
		}
	}
	require.GreaterOrEqual(t, warnCount, 1)
}

type captureRecord struct {
	level slog.Level
	msg   string
	attrs map[string]any
}

type captureSlogHandler struct {
	mu      sync.Mutex
	records []captureRecord
}

func (h *captureSlogHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *captureSlogHandler) Handle(_ context.Context, rec slog.Record) error {
	attrs := make(map[string]any)
	rec.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})

	h.mu.Lock()
	h.records = append(h.records, captureRecord{level: rec.Level, msg: rec.Message, attrs: attrs})
	h.mu.Unlock()
	return nil
}

func (h *captureSlogHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h *captureSlogHandler) WithGroup(string) slog.Handler {
	return h
}

func (h *captureSlogHandler) recordsSnapshot() []captureRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]captureRecord, len(h.records))
	copy(out, h.records)
	return out
}
