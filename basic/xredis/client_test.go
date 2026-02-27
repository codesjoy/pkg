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

package xredis

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	logmiddleware "github.com/codesjoy/pkg/basic/xredis/middleware/logger"
	otelmiddleware "github.com/codesjoy/pkg/basic/xredis/middleware/otel"
)

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	var nilCfg *Config
	require.ErrorIs(t, nilCfg.Validate(), ErrEmptyAddrs)

	cfg := Config{}
	require.ErrorIs(t, cfg.Validate(), ErrEmptyAddrs)

	cfg = Config{UniversalOptions: redis.UniversalOptions{Addrs: []string{"", "  "}}}
	require.ErrorIs(t, cfg.Validate(), ErrEmptyAddrs)

	cfg = Config{UniversalOptions: redis.UniversalOptions{Addrs: []string{"127.0.0.1:6379"}}}
	require.NoError(t, cfg.Validate())

	cfg = Config{
		UniversalOptions: redis.UniversalOptions{
			Addrs: []string{" 127.0.0.1:6379 ", "", "   ", "\t127.0.0.1:6380\t"},
		},
	}
	require.NoError(t, cfg.Validate())
	require.Equal(t, []string{"127.0.0.1:6379", "127.0.0.1:6380"}, cfg.Addrs)
}

func TestNewAndMustNew(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	cfg := Config{UniversalOptions: redis.UniversalOptions{Addrs: []string{mr.Addr()}}}

	client, err := New(cfg)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NotNil(t, client.Raw())
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	ctx := context.Background()
	require.NoError(t, client.Set(ctx, "k", "v", 0).Err())
	value, err := client.Get(ctx, "k").Result()
	require.NoError(t, err)
	require.Equal(t, "v", value)

	require.Panics(t, func() {
		_ = MustNew(Config{})
	})
}

func TestOptionOrderStrict(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	cfg := Config{UniversalOptions: redis.UniversalOptions{Addrs: []string{mr.Addr()}}}

	recorder := &eventRecorder{}
	opt := func(name string) Option {
		return func(_ *Client) error {
			recorder.Append(name)
			return nil
		}
	}

	client, err := New(cfg, opt("a"), opt("b"), opt("c"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	require.Equal(t, []string{"a", "b", "c"}, recorder.Values())
}

func TestWithHookKeepsHookOrder(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	cfg := Config{UniversalOptions: redis.UniversalOptions{Addrs: []string{mr.Addr()}}}

	recorder := &eventRecorder{}
	h1 := &recordHook{name: "h1", recorder: recorder}
	h2 := &recordHook{name: "h2", recorder: recorder}
	h3 := &recordHook{name: "h3", recorder: recorder}

	client, err := New(cfg, WithHook(h1, h2), WithHook(h3))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	require.NoError(t, client.Set(context.Background(), "k", "v", 0).Err())
	require.Equal(
		t,
		[]string{"h1:before", "h2:before", "h3:before", "h3:after", "h2:after", "h1:after"},
		recorder.Values(),
	)
}

func TestWithLoggerRespectsOptionOrder(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	cfg := Config{UniversalOptions: redis.UniversalOptions{Addrs: []string{mr.Addr()}}}

	recorder := &eventRecorder{}
	logHandler := newEventLogHandler(func(msg string, attrs map[string]any) {
		if msg != "xredis command slow" {
			return
		}
		if attrs["command"] != "set" {
			return
		}
		recorder.Append("logger")
	})

	before := &recordHook{name: "before", recorder: recorder}
	after := &recordHook{name: "after", recorder: recorder}

	client, err := New(
		cfg,
		WithHook(before),
		WithLogger(logmiddleware.Config{
			Logger:        slog.New(logHandler),
			SlowThreshold: time.Nanosecond,
		}),
		WithHook(after),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	require.NoError(t, client.Set(context.Background(), "k", "v", 0).Err())
	require.Equal(
		t,
		[]string{"before:before", "after:before", "after:after", "logger", "before:after"},
		recorder.Values(),
	)
}

func TestWithOpenTelemetryRespectsOptionOrder(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	cfg := Config{UniversalOptions: redis.UniversalOptions{Addrs: []string{mr.Addr()}}}

	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	hookAfterOtel := &spanAwareHook{}
	clientAfterOtel, err := New(
		cfg,
		WithOpenTelemetry(otelmiddleware.Config{EnableTracing: true, TracerProvider: provider}),
		WithHook(hookAfterOtel),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, clientAfterOtel.Close())
	})
	require.NoError(t, clientAfterOtel.Set(context.Background(), "k1", "v", 0).Err())
	require.True(t, hookAfterOtel.SeenSpan())

	hookBeforeOtel := &spanAwareHook{}
	clientBeforeOtel, err := New(
		cfg,
		WithHook(hookBeforeOtel),
		WithOpenTelemetry(otelmiddleware.Config{EnableTracing: true, TracerProvider: provider}),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, clientBeforeOtel.Close())
	})
	require.NoError(t, clientBeforeOtel.Set(context.Background(), "k2", "v", 0).Err())
	require.False(t, hookBeforeOtel.SeenSpan())
}

func TestOptionFailureClosesClient(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	cfg := Config{UniversalOptions: redis.UniversalOptions{Addrs: []string{mr.Addr()}}}

	boom := errors.New("boom")
	var captured *Client

	_, err := New(cfg, func(client *Client) error {
		captured = client
		return boom
	})
	require.Error(t, err)
	require.ErrorIs(t, err, boom)
	require.NotNil(t, captured)
	require.ErrorContains(t, captured.Set(context.Background(), "k", "v", 0).Err(), "closed")
}

type eventRecorder struct {
	mu     sync.Mutex
	events []string
}

func (r *eventRecorder) Append(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *eventRecorder) Values() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	cloned := make([]string, len(r.events))
	copy(cloned, r.events)
	return cloned
}

type recordHook struct {
	name     string
	recorder *eventRecorder
}

var _ redis.Hook = (*recordHook)(nil)

func (h *recordHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h *recordHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd != nil && cmd.Name() == "set" {
			h.recorder.Append(h.name + ":before")
		}
		err := next(ctx, cmd)
		if cmd != nil && cmd.Name() == "set" {
			h.recorder.Append(h.name + ":after")
		}
		return err
	}
}

func (h *recordHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

type spanAwareHook struct {
	mu       sync.Mutex
	seenSpan bool
}

var _ redis.Hook = (*spanAwareHook)(nil)

func (h *spanAwareHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h *spanAwareHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd != nil && cmd.Name() == "set" {
			span := trace.SpanFromContext(ctx)
			h.mu.Lock()
			h.seenSpan = span != nil && span.SpanContext().IsValid()
			h.mu.Unlock()
		}
		return next(ctx, cmd)
	}
}

func (h *spanAwareHook) ProcessPipelineHook(
	next redis.ProcessPipelineHook,
) redis.ProcessPipelineHook {
	return next
}

func (h *spanAwareHook) SeenSpan() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.seenSpan
}

type eventLogHandler struct {
	cb    func(string, map[string]any)
	attrs []slog.Attr
}

func newEventLogHandler(cb func(string, map[string]any)) *eventLogHandler {
	return &eventLogHandler{cb: cb, attrs: make([]slog.Attr, 0)}
}

func (h *eventLogHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *eventLogHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := make(map[string]any)
	for _, attr := range h.attrs {
		attrs[attr.Key] = attr.Value.Any()
	}
	record.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})
	if h.cb != nil {
		h.cb(record.Message, attrs)
	}
	return nil
}

func (h *eventLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return &eventLogHandler{cb: h.cb, attrs: merged}
}

func (h *eventLogHandler) WithGroup(string) slog.Handler {
	return h
}
