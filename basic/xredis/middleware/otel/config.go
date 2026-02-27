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

func normalizeConfig(cfg Config) Config {
	normalized := cfg
	if normalized.DBSystem == "" {
		normalized.DBSystem = "redis"
	}
	return normalized
}
