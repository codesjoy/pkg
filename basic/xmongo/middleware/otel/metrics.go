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
	"time"

	"go.mongodb.org/mongo-driver/v2/event"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type metricsMonitors struct {
	Command *event.CommandMonitor
	Pool    *event.PoolMonitor
}

type metricsRecorder struct {
	commandCount      metric.Int64Counter
	commandDuration   metric.Float64Histogram
	commandErrorCount metric.Int64Counter
	poolEventCount    metric.Int64Counter
}

func newMetricsMonitors(cfg Config) (metricsMonitors, error) {
	meterProvider := cfg.MeterProvider
	if meterProvider == nil {
		meterProvider = otel.GetMeterProvider()
	}

	meter := meterProvider.Meter(instrumentationScope)
	commandCount, err := meter.Int64Counter("xmongo.command.count")
	if err != nil {
		return metricsMonitors{}, err
	}
	commandDuration, err := meter.Float64Histogram(
		"xmongo.command.duration",
		metric.WithUnit("ms"),
	)
	if err != nil {
		return metricsMonitors{}, err
	}
	commandErrorCount, err := meter.Int64Counter("xmongo.command.error.count")
	if err != nil {
		return metricsMonitors{}, err
	}
	poolEventCount, err := meter.Int64Counter("xmongo.pool.event.count")
	if err != nil {
		return metricsMonitors{}, err
	}

	recorder := &metricsRecorder{
		commandCount:      commandCount,
		commandDuration:   commandDuration,
		commandErrorCount: commandErrorCount,
		poolEventCount:    poolEventCount,
	}
	return metricsMonitors{
		Command: recorder.commandMonitor(),
		Pool:    recorder.poolMonitor(),
	}, nil
}

func (r *metricsRecorder) commandMonitor() *event.CommandMonitor {
	return &event.CommandMonitor{
		Succeeded: func(ctx context.Context, evt *event.CommandSucceededEvent) {
			if evt == nil {
				return
			}
			attrs := commandMetricAttrs(evt.CommandName, "success")
			r.commandCount.Add(ctx, 1, metric.WithAttributes(attrs...))
			r.commandDuration.Record(
				ctx,
				float64(evt.Duration)/float64(time.Millisecond),
				metric.WithAttributes(attrs...),
			)
		},
		Failed: func(ctx context.Context, evt *event.CommandFailedEvent) {
			if evt == nil {
				return
			}
			attrs := commandMetricAttrs(evt.CommandName, "error")
			r.commandCount.Add(ctx, 1, metric.WithAttributes(attrs...))
			r.commandErrorCount.Add(ctx, 1, metric.WithAttributes(attrs...))
			r.commandDuration.Record(
				ctx,
				float64(evt.Duration)/float64(time.Millisecond),
				metric.WithAttributes(attrs...),
			)
		},
	}
}

func (r *metricsRecorder) poolMonitor() *event.PoolMonitor {
	return &event.PoolMonitor{
		Event: func(evt *event.PoolEvent) {
			if evt == nil {
				return
			}
			r.poolEventCount.Add(
				context.Background(),
				1,
				metric.WithAttributes(
					attribute.String("db.system", "mongodb"),
					attribute.String("event_type", evt.Type),
				),
			)
		},
	}
}

func commandMetricAttrs(commandName string, status string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("db.system", "mongodb"),
		attribute.String("command", commandName),
		attribute.String("status", status),
	}
}
