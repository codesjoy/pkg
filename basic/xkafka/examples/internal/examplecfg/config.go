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

package examplecfg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBrokers   = "127.0.0.1:9092"
	defaultTopic     = "xkafka-example"
	defaultGroupID   = "xkafka-example-group"
	defaultPartition = 0
	defaultTimeout   = 30 * time.Second
)

// Config holds shared runtime values for all examples.
type Config struct {
	Brokers   []string
	Topic     string
	GroupID   string
	Partition int32
	Timeout   time.Duration
}

// Load reads example config from environment variables.
func Load() (Config, error) {
	cfg := Config{
		Topic:     valueOrDefault("XKAFKA_TOPIC", defaultTopic),
		GroupID:   valueOrDefault("XKAFKA_GROUP_ID", defaultGroupID),
		Partition: defaultPartition,
		Timeout:   defaultTimeout,
	}

	brokers := valueOrDefault("XKAFKA_BROKERS", defaultBrokers)
	cfg.Brokers = splitAndTrim(brokers)
	if len(cfg.Brokers) == 0 {
		return Config{}, fmt.Errorf("XKAFKA_BROKERS is empty")
	}

	partitionRaw := strings.TrimSpace(os.Getenv("XKAFKA_PARTITION"))
	if partitionRaw != "" {
		parsed, err := strconv.ParseInt(partitionRaw, 10, 32)
		if err != nil {
			return Config{}, fmt.Errorf("parse XKAFKA_PARTITION: %w", err)
		}
		if parsed < 0 {
			return Config{}, fmt.Errorf("XKAFKA_PARTITION must be >= 0, got %d", parsed)
		}
		cfg.Partition = int32(parsed)
	}

	timeoutRaw := strings.TrimSpace(os.Getenv("XKAFKA_TIMEOUT"))
	if timeoutRaw != "" {
		parsed, err := time.ParseDuration(timeoutRaw)
		if err != nil {
			return Config{}, fmt.Errorf("parse XKAFKA_TIMEOUT: %w", err)
		}
		if parsed <= 0 {
			return Config{}, fmt.Errorf("XKAFKA_TIMEOUT must be > 0, got %s", parsed)
		}
		cfg.Timeout = parsed
	}

	return cfg, nil
}

// NewLogger creates a text logger for local example runs.
func NewLogger() *slog.Logger {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(handler)
}

// WithTimeout applies timeout to parent context.
func WithTimeout(
	parent context.Context,
	timeout time.Duration,
) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return context.WithTimeout(parent, timeout)
}

func valueOrDefault(key, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	return value
}

func splitAndTrim(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}
