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

package otel

import (
	"context"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	require.Equal(t, "redis", cfg.DBSystem)
	require.False(t, cfg.EnableTracing)
	require.False(t, cfg.EnableMetrics)
	require.False(t, cfg.DBStatement)
}

func TestApplyNilClient(t *testing.T) {
	t.Parallel()

	err := Apply(nil, Config{EnableTracing: true})
	require.Error(t, err)
}

func TestApplyTracingOnly(t *testing.T) {
	t.Parallel()

	client := newMiniRedisClient(t)
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	err := Apply(client, Config{EnableTracing: true, TracerProvider: tp})
	require.NoError(t, err)

	require.NoError(t, client.Set(context.Background(), "k", "v", 0).Err())
	_, err = client.Get(context.Background(), "k").Result()
	require.NoError(t, err)

	ended := recorder.Ended()
	require.NotEmpty(t, ended)
	for _, span := range ended {
		for _, attr := range span.Attributes() {
			require.NotEqual(t, "db.statement", string(attr.Key))
		}
	}
}

func TestApplyMetricsOnly(t *testing.T) {
	t.Parallel()

	client := newMiniRedisClient(t)
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))

	err := Apply(client, Config{EnableMetrics: true, MeterProvider: provider})
	require.NoError(t, err)

	require.NoError(t, client.Set(context.Background(), "k", "v", 0).Err())
	_, err = client.Get(context.Background(), "k").Result()
	require.NoError(t, err)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	found := false
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if strings.HasPrefix(metric.Name, "db.client.") {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	require.True(t, found)
}

func TestApplyTraceAndMetrics(t *testing.T) {
	t.Parallel()

	client := newMiniRedisClient(t)
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	reader := metric.NewManualReader()
	mp := metric.NewMeterProvider(metric.WithReader(reader))

	err := Apply(client, Config{
		EnableTracing:  true,
		EnableMetrics:  true,
		TracerProvider: tp,
		MeterProvider:  mp,
	})
	require.NoError(t, err)

	require.NoError(t, client.Set(context.Background(), "k", "v", 0).Err())

	require.NotEmpty(t, recorder.Ended())
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	require.NotEmpty(t, rm.ScopeMetrics)
}

func newMiniRedisClient(t *testing.T) redis.UniversalClient {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewUniversalClient(&redis.UniversalOptions{Addrs: []string{mr.Addr()}})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	return client
}
