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

//go:build integration

package integration

import (
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/codesjoy/pkg/basic/xredis"
	xotel "github.com/codesjoy/pkg/basic/xredis/middleware/otel"
)

func TestOpenTelemetryInstrumentation(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	spanRecorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	metricReader := metric.NewManualReader()
	metricProvider := metric.NewMeterProvider(metric.WithReader(metricReader))

	client, err := xredis.New(
		xredis.Config{UniversalOptions: redis.UniversalOptions{Addrs: []string{mustAddr(t)}}},
		xredis.WithOpenTelemetry(xotel.Config{
			EnableTracing:  true,
			EnableMetrics:  true,
			TracerProvider: traceProvider,
			MeterProvider:  metricProvider,
			DBStatement:    false,
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	require.NoError(t, client.Set(ctx, "it:otel:key", "value", 0).Err())
	_, err = client.Get(ctx, "it:otel:key").Result()
	require.NoError(t, err)

	ended := spanRecorder.Ended()
	require.NotEmpty(t, ended)
	for _, span := range ended {
		for _, attr := range span.Attributes() {
			require.NotEqual(t, "db.statement", string(attr.Key))
		}
	}

	var rm metricdata.ResourceMetrics
	require.NoError(t, metricReader.Collect(ctx, &rm))
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
