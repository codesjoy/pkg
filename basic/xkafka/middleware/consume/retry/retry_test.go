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

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
)

func TestBackoff(t *testing.T) {
	t.Parallel()

	cfg := NormalizeConfig(Config{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
		Multiplier:     2,
	})

	require.Equal(t, 10*time.Millisecond, Backoff(cfg, 1))
	require.Equal(t, 20*time.Millisecond, Backoff(cfg, 2))
	require.Equal(t, 40*time.Millisecond, Backoff(cfg, 3))
	require.Equal(t, 50*time.Millisecond, Backoff(cfg, 4))
}

func TestMiddlewareRetriesDownstream(t *testing.T) {
	t.Parallel()

	middleware := New(Config{
		MaxRetries:     InfiniteRetries,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1,
	}, ExhaustedPolicyBlock, nil, slog.Default(), nil)

	attempts := 0
	msgCtx := &consume.MessageContext{
		Message: &sarama.ConsumerMessage{Topic: "orders", Partition: 0, Offset: 1},
	}
	err := middleware.Handle(
		context.Background(),
		msgCtx,
		func(context.Context, *consume.MessageContext) error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary")
			}
			return nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, 3, attempts)
	require.Equal(t, 3, msgCtx.Attempt)
}

func TestMiddlewareCancelInfiniteRetry(t *testing.T) {
	t.Parallel()

	middleware := New(Config{
		MaxRetries:     InfiniteRetries,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1,
	}, ExhaustedPolicyBlock, nil, slog.Default(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := middleware.Handle(
		ctx,
		&consume.MessageContext{Message: &sarama.ConsumerMessage{}},
		func(context.Context, *consume.MessageContext) error {
			return errors.New("always fail")
		},
	)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestMiddlewareDLQCommit(t *testing.T) {
	t.Parallel()

	dlq := &mockDLQPublisher{}
	middleware := New(Config{
		MaxRetries:     0,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1,
	}, ExhaustedPolicyDLQCommit, nil, slog.Default(), dlq)

	err := middleware.Handle(context.Background(), &consume.MessageContext{
		Message: &sarama.ConsumerMessage{
			Topic:     "orders",
			Partition: 0,
			Offset:    10,
			Value:     []byte("hello"),
		},
		LogicalKey: "order-1",
		Shard:      2,
	}, func(context.Context, *consume.MessageContext) error {
		return errors.New("business failed")
	})
	require.NoError(t, err)
	require.Len(t, dlq.events, 1)
	require.Equal(t, FailureStageDLQ, dlq.events[0].Stage)
}

func TestMiddlewareStop(t *testing.T) {
	t.Parallel()

	middleware := New(Config{
		MaxRetries:     0,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1,
	}, ExhaustedPolicyStop, nil, slog.Default(), nil)

	err := middleware.Handle(
		context.Background(),
		&consume.MessageContext{Message: &sarama.ConsumerMessage{}},
		func(context.Context, *consume.MessageContext) error {
			return errors.New("failed")
		},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exhausted")
}

func TestMiddlewareFailureHookStages(t *testing.T) {
	t.Parallel()

	var stages []FailureStage
	middleware := New(Config{
		MaxRetries:     0,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1,
	}, ExhaustedPolicyStop, func(_ context.Context, event Event) {
		stages = append(stages, event.Stage)
	}, slog.Default(), nil)

	err := middleware.Handle(
		context.Background(),
		&consume.MessageContext{Message: &sarama.ConsumerMessage{}},
		func(context.Context, *consume.MessageContext) error {
			return errors.New("failed")
		},
	)
	require.Error(t, err)
	require.Equal(
		t,
		[]FailureStage{FailureStageRetry, FailureStageExhausted, FailureStageStop},
		stages,
	)
}

func TestMiddlewareLogsWarnOnRetry(t *testing.T) {
	t.Parallel()

	capture := &captureSlogHandler{}
	middleware := New(Config{
		MaxRetries:     InfiniteRetries,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1,
	}, ExhaustedPolicyBlock, nil, slog.New(capture), nil)

	attempts := 0
	err := middleware.Handle(
		context.Background(),
		&consume.MessageContext{Message: &sarama.ConsumerMessage{}},
		func(context.Context, *consume.MessageContext) error {
			attempts++
			if attempts == 1 {
				return errors.New("retry me")
			}
			return nil
		},
	)
	require.NoError(t, err)

	records := capture.recordsSnapshot()
	require.NotEmpty(t, records)
	require.Equal(t, slog.LevelWarn, records[0].level)
	require.Equal(t, "xkafka retrying message", records[0].msg)
}

type mockDLQPublisher struct {
	events []Event
	err    error
}

func (m *mockDLQPublisher) Publish(_ context.Context, event Event) error {
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, event)
	return nil
}

type captureRecord struct {
	level slog.Level
	msg   string
}

type captureSlogHandler struct {
	mu      sync.Mutex
	records []captureRecord
}

func (h *captureSlogHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *captureSlogHandler) Handle(_ context.Context, rec slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, captureRecord{level: rec.Level, msg: rec.Message})
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
