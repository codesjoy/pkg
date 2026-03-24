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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/event"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestNewMonitorsTracingOnlyProducesSpans(t *testing.T) {
	t.Parallel()

	spanRecorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	t.Cleanup(func() {
		require.NoError(t, traceProvider.Shutdown(context.Background()))
	})

	monitors, err := NewMonitors(Config{
		EnableTracing:            true,
		TracerProvider:           traceProvider,
		CommandAttributeDisabled: true,
	})
	require.NoError(t, err)
	require.NotNil(t, monitors.Command)
	require.Nil(t, monitors.Pool)

	ctx := context.Background()
	monitors.Command.Started(ctx, &event.CommandStartedEvent{
		CommandName:  "find",
		RequestID:    1,
		ConnectionID: "conn-1",
	})
	monitors.Command.Succeeded(ctx, &event.CommandSucceededEvent{
		CommandFinishedEvent: event.CommandFinishedEvent{
			CommandName:  "find",
			RequestID:    1,
			ConnectionID: "conn-1",
			Duration:     time.Millisecond,
		},
	})

	require.NotEmpty(t, spanRecorder.Ended())
}

func TestNewMonitorsMetricsOnlyProducesMetrics(t *testing.T) {
	t.Parallel()

	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	monitors, err := NewMonitors(Config{
		EnableMetrics: true,
		MeterProvider: meterProvider,
	})
	require.NoError(t, err)
	require.NotNil(t, monitors.Command)
	require.NotNil(t, monitors.Pool)

	ctx := context.Background()
	monitors.Command.Succeeded(ctx, &event.CommandSucceededEvent{
		CommandFinishedEvent: event.CommandFinishedEvent{
			CommandName: "find",
			Duration:    time.Millisecond,
		},
	})
	monitors.Command.Failed(ctx, &event.CommandFailedEvent{
		CommandFinishedEvent: event.CommandFinishedEvent{
			CommandName: "insert",
			Duration:    time.Millisecond,
		},
		Failure: context.DeadlineExceeded,
	})
	monitors.Pool.Event(&event.PoolEvent{Type: event.ConnectionPoolCleared})

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &rm))
	require.True(t, hasMetricName(rm, "xmongo.command.count"))
	require.True(t, hasMetricName(rm, "xmongo.command.duration"))
	require.True(t, hasMetricName(rm, "xmongo.command.error.count"))
	require.True(t, hasMetricName(rm, "xmongo.pool.event.count"))
}

func hasMetricName(rm metricdata.ResourceMetrics, name string) bool {
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				return true
			}
		}
	}
	return false
}
