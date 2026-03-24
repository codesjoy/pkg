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

	"go.mongodb.org/mongo-driver/v2/event"
	"go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/v2/mongo/otelmongo"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationScope = "github.com/codesjoy/pkg/basic/xmongo"

// Config controls OpenTelemetry tracing and metrics for MongoDB commands.
type Config struct {
	EnableTracing            bool
	EnableMetrics            bool
	TracerProvider           trace.TracerProvider
	MeterProvider            metric.MeterProvider
	CommandAttributeDisabled bool
}

// Monitors groups OpenTelemetry monitors for xmongo.
type Monitors struct {
	Command *event.CommandMonitor
	Pool    *event.PoolMonitor
}

// DefaultConfig returns the default OpenTelemetry monitor config.
func DefaultConfig() Config {
	return Config{EnableTracing: true}
}

// NewMonitor builds an otelmongo-backed command monitor.
func NewMonitor(cfg Config) *event.CommandMonitor {
	normalized := normalizeConfig(cfg)
	if !normalized.EnableTracing {
		return nil
	}

	opts := make([]otelmongo.Option, 0, 2)
	if cfg.TracerProvider != nil {
		opts = append(opts, otelmongo.WithTracerProvider(cfg.TracerProvider))
	}
	if cfg.CommandAttributeDisabled {
		opts = append(opts, otelmongo.WithCommandAttributeDisabled(true))
	}
	return otelmongo.NewMonitor(opts...)
}

// NewMonitors builds OpenTelemetry tracing and metrics monitors.
func NewMonitors(cfg Config) (Monitors, error) {
	normalized := normalizeConfig(cfg)

	monitors := Monitors{}
	commandMonitors := make([]*event.CommandMonitor, 0, 2)
	if normalized.EnableTracing {
		commandMonitors = append(commandMonitors, NewMonitor(normalized))
	}
	if normalized.EnableMetrics {
		metricsMonitors, err := newMetricsMonitors(normalized)
		if err != nil {
			return Monitors{}, err
		}
		commandMonitors = append(commandMonitors, metricsMonitors.Command)
		monitors.Pool = metricsMonitors.Pool
	}

	monitors.Command = composeCommandMonitors(commandMonitors...)
	return monitors, nil
}

func normalizeConfig(cfg Config) Config {
	normalized := cfg
	if !normalized.EnableTracing && !normalized.EnableMetrics {
		if normalized.MeterProvider != nil {
			normalized.EnableMetrics = true
		}
		if normalized.TracerProvider != nil || !normalized.EnableMetrics {
			normalized.EnableTracing = true
		}
	}
	return normalized
}

func composeCommandMonitors(monitors ...*event.CommandMonitor) *event.CommandMonitor {
	nonNil := compactNonNil(monitors)
	switch len(nonNil) {
	case 0:
		return nil
	case 1:
		return nonNil[0]
	}
	return &event.CommandMonitor{
		Started: collectCommandHandlers(
			nonNil,
			func(monitor *event.CommandMonitor) func(context.Context, *event.CommandStartedEvent) {
				return monitor.Started
			},
		),
		Succeeded: collectCommandHandlers(
			nonNil,
			func(monitor *event.CommandMonitor) func(context.Context, *event.CommandSucceededEvent) {
				return monitor.Succeeded
			},
		),
		Failed: collectCommandHandlers(
			nonNil,
			func(monitor *event.CommandMonitor) func(context.Context, *event.CommandFailedEvent) {
				return monitor.Failed
			},
		),
	}
}

func compactNonNil[T any](values []*T) []*T {
	compacted := make([]*T, 0, len(values))
	for _, value := range values {
		if value != nil {
			compacted = append(compacted, value)
		}
	}
	return compacted
}

func collectCommandHandlers[T any](
	monitors []*event.CommandMonitor,
	pick func(*event.CommandMonitor) func(context.Context, *T),
) func(context.Context, *T) {
	handlers := make([]func(context.Context, *T), 0, len(monitors))
	for _, monitor := range monitors {
		if handler := pick(monitor); handler != nil {
			handlers = append(handlers, handler)
		}
	}
	return func(ctx context.Context, evt *T) {
		for _, handler := range handlers {
			handler(ctx, evt)
		}
	}
}
