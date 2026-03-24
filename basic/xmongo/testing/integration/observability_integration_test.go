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
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/codesjoy/pkg/basic/xmongo"
	logmiddleware "github.com/codesjoy/pkg/basic/xmongo/middleware/logger"
	xotel "github.com/codesjoy/pkg/basic/xmongo/middleware/otel"
)

func TestOpenTelemetryMetricsAndTracingIntegration(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	spanRecorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	metricReader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))

	client, err := xmongo.New(
		xmongo.Config{URI: mustURI(t), DefaultDatabase: "xmongo_integration"},
		xmongo.WithOpenTelemetry(xotel.Config{
			EnableTracing:            true,
			EnableMetrics:            true,
			TracerProvider:           traceProvider,
			MeterProvider:            meterProvider,
			CommandAttributeDisabled: true,
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, client.Close(cleanupCtx))
		require.NoError(t, traceProvider.Shutdown(cleanupCtx))
	})

	collection, err := client.Collection("otel_metrics_widgets")
	require.NoError(t, err)

	_, err = collection.InsertOne(ctx, bson.D{{Key: "_id", Value: "otel-metrics-1"}})
	require.NoError(t, err)
	_, err = collection.InsertOne(ctx, bson.D{{Key: "_id", Value: "otel-metrics-1"}})
	require.Error(t, err)

	var doc bson.M
	err = collection.FindOne(ctx, bson.D{{Key: "_id", Value: "otel-metrics-1"}}).Decode(&doc)
	require.NoError(t, err)

	require.NotEmpty(t, spanRecorder.Ended())

	var rm metricdata.ResourceMetrics
	require.NoError(t, metricReader.Collect(ctx, &rm))
	require.True(t, hasMetricName(rm, "xmongo.command.count"))
	require.True(t, hasMetricName(rm, "xmongo.command.duration"))
	require.True(t, hasMetricName(rm, "xmongo.pool.event.count"))
}

func TestOpenTelemetryInstrumentation(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	spanRecorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))

	client, err := xmongo.New(
		xmongo.Config{URI: mustURI(t)},
		xmongo.WithOpenTelemetry(xotel.Config{
			TracerProvider:           traceProvider,
			CommandAttributeDisabled: true,
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, client.Disconnect(cleanupCtx))
		require.NoError(t, traceProvider.Shutdown(cleanupCtx))
	})

	collection := client.Database("xmongo_integration").Collection("otel_widgets")

	_, err = collection.InsertOne(ctx, bson.D{{Key: "_id", Value: "otel-1"}})
	require.NoError(t, err)

	var doc bson.M
	err = collection.FindOne(ctx, bson.D{{Key: "_id", Value: "otel-1"}}).Decode(&doc)
	require.NoError(t, err)

	ended := spanRecorder.Ended()
	require.NotEmpty(t, ended)
}

func TestLoggerIntegrationDoesNotAffectCRUD(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	handler := &integrationLogHandler{}
	client, err := xmongo.New(
		xmongo.Config{URI: mustURI(t), DefaultDatabase: "xmongo_integration"},
		xmongo.WithLogger(logmiddleware.Config{
			Logger:        slog.New(handler),
			SlowThreshold: time.Nanosecond,
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, client.Close(cleanupCtx))
	})

	collection, err := client.Collection("logger_widgets")
	require.NoError(t, err)

	_, err = collection.InsertOne(ctx, bson.D{{Key: "_id", Value: "logger-1"}})
	require.NoError(t, err)

	var doc bson.M
	err = collection.FindOne(ctx, bson.D{{Key: "_id", Value: "logger-1"}}).Decode(&doc)
	require.NoError(t, err)

	require.NotEmpty(t, handler.Messages())
}

func TestHealthTrackingSnapshot(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	client, err := xmongo.New(
		xmongo.Config{URI: mustURI(t), DefaultDatabase: "xmongo_integration"},
		xmongo.WithHealthTracking(),
		xmongo.WithClientOptions(options.Client().SetHeartbeatInterval(600*time.Millisecond)),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, client.Close(cleanupCtx))
	})

	require.NoError(t, client.PingPrimary(ctx))
	collection, err := client.Collection("health_widgets")
	require.NoError(t, err)
	_, err = collection.InsertOne(ctx, bson.D{{Key: "_id", Value: "health-1"}})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		snapshot := client.HealthSnapshot()
		return !snapshot.LastPingAt.IsZero() &&
			snapshot.LastPingErr == nil &&
			!snapshot.LastPoolEventAt.IsZero() &&
			snapshot.LastPoolEventType != "" &&
			!snapshot.LastHeartbeatAt.IsZero() &&
			!snapshot.UpdatedAt.IsZero()
	}, 15*time.Second, 100*time.Millisecond)
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

type integrationLogHandler struct {
	mu       sync.Mutex
	messages []string
}

func (h *integrationLogHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *integrationLogHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.messages = append(h.messages, record.Message)
	return nil
}

func (h *integrationLogHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *integrationLogHandler) WithGroup(string) slog.Handler      { return h }

func (h *integrationLogHandler) Messages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	cloned := make([]string, len(h.messages))
	copy(cloned, h.messages)
	return cloned
}
