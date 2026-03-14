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
	"errors"
	"fmt"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Config controls OpenTelemetry instrumentation for redis clients.
type Config struct {
	EnableTracing bool
	EnableMetrics bool

	DBSystem   string
	Attributes []attribute.KeyValue

	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider

	DBStatement   bool
	CallerEnabled bool
	DialTracing   bool

	CommandFilter  func(redis.Cmder) bool
	CommandsFilter func([]redis.Cmder) bool
}

// DefaultConfig returns default OpenTelemetry middleware config.
func DefaultConfig() Config {
	return Config{
		EnableTracing: false,
		EnableMetrics: false,
		DBSystem:      "redis",
		DBStatement:   false,
		CallerEnabled: false,
		DialTracing:   false,
	}
}

// Apply configures trace/metrics instrumentation to a redis.UniversalClient.
func Apply(client redis.UniversalClient, cfg Config) error {
	if client == nil {
		return errors.New("redis client is nil")
	}

	normalized := normalizeConfig(cfg)
	if !normalized.EnableTracing && !normalized.EnableMetrics {
		return nil
	}

	if normalized.EnableTracing {
		if err := redisotel.InstrumentTracing(client, buildTracingOptions(normalized)...); err != nil {
			return fmt.Errorf("instrument tracing: %w", err)
		}
	}

	if normalized.EnableMetrics {
		if err := redisotel.InstrumentMetrics(client, buildMetricsOptions(normalized)...); err != nil {
			return fmt.Errorf("instrument metrics: %w", err)
		}
	}

	return nil
}

func buildTracingOptions(cfg Config) []redisotel.TracingOption {
	opts := make([]redisotel.TracingOption, 0, 8)
	opts = append(opts,
		redisotel.WithDBSystem(cfg.DBSystem),
		redisotel.WithDBStatement(cfg.DBStatement),
		redisotel.WithCallerEnabled(cfg.CallerEnabled),
		redisotel.WithDialFilter(!cfg.DialTracing),
	)

	if len(cfg.Attributes) > 0 {
		opts = append(opts, redisotel.WithAttributes(cfg.Attributes...))
	}
	if cfg.TracerProvider != nil {
		opts = append(opts, redisotel.WithTracerProvider(cfg.TracerProvider))
	}
	if cfg.CommandFilter != nil {
		opts = append(opts, redisotel.WithCommandFilter(cfg.CommandFilter))
	}
	if cfg.CommandsFilter != nil {
		opts = append(opts, redisotel.WithCommandsFilter(cfg.CommandsFilter))
	}
	return opts
}

func buildMetricsOptions(cfg Config) []redisotel.MetricsOption {
	opts := make([]redisotel.MetricsOption, 0, 4)
	opts = append(opts, redisotel.WithDBSystem(cfg.DBSystem))

	if len(cfg.Attributes) > 0 {
		opts = append(opts, redisotel.WithAttributes(cfg.Attributes...))
	}
	if cfg.MeterProvider != nil {
		opts = append(opts, redisotel.WithMeterProvider(cfg.MeterProvider))
	}

	return opts
}

func normalizeConfig(cfg Config) Config {
	normalized := cfg
	if normalized.DBSystem == "" {
		normalized.DBSystem = "redis"
	}
	return normalized
}
