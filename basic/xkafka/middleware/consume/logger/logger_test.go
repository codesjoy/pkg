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

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
)

func TestMiddlewareSuccess(t *testing.T) {
	t.Parallel()

	h := &captureSlogHandler{}
	middleware := New(slog.New(h))

	err := middleware.Handle(context.Background(), &consume.MessageContext{
		Message:    &sarama.ConsumerMessage{Topic: "orders", Partition: 1, Offset: 8},
		LogicalKey: "order-1",
		Shard:      2,
		Attempt:    1,
	}, func(context.Context, *consume.MessageContext) error {
		return nil
	})
	require.NoError(t, err)

	records := h.recordsSnapshot()
	require.Len(t, records, 1)
	require.Equal(t, slog.LevelInfo, records[0].level)
	require.Equal(t, "success", records[0].attrs["result"])
}

func TestMiddlewareFailure(t *testing.T) {
	t.Parallel()

	h := &captureSlogHandler{}
	middleware := New(slog.New(h))

	wantErr := errors.New("business failed")
	err := middleware.Handle(context.Background(), &consume.MessageContext{
		Message:    &sarama.ConsumerMessage{Topic: "orders", Partition: 1, Offset: 8},
		LogicalKey: "order-1",
		Shard:      2,
		Attempt:    3,
	}, func(context.Context, *consume.MessageContext) error {
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)

	records := h.recordsSnapshot()
	require.Len(t, records, 1)
	require.Equal(t, slog.LevelError, records[0].level)
	require.Equal(t, "error", records[0].attrs["result"])
	require.Equal(t, "business failed", records[0].attrs["error"])
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
