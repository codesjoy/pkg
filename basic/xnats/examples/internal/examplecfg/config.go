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
	"strings"
	"time"
)

const (
	defaultURL      = "nats://127.0.0.1:4222"
	defaultSubject  = "xnats.example"
	defaultStream   = "XNATS_EXAMPLE"
	defaultConsumer = "xnats-example-consumer"
	defaultTimeout  = 30 * time.Second
)

// Config holds shared runtime values for all examples.
type Config struct {
	URL      string
	Subject  string
	Stream   string
	Consumer string
	Timeout  time.Duration
}

// Load reads example config from environment variables.
func Load() (Config, error) {
	cfg := Config{
		URL:      valueOrDefault("XNATS_URL", defaultURL),
		Subject:  valueOrDefault("XNATS_SUBJECT", defaultSubject),
		Stream:   valueOrDefault("XNATS_STREAM", defaultStream),
		Consumer: valueOrDefault("XNATS_CONSUMER", defaultConsumer),
		Timeout:  defaultTimeout,
	}

	timeoutRaw := strings.TrimSpace(os.Getenv("XNATS_TIMEOUT"))
	if timeoutRaw != "" {
		parsed, err := time.ParseDuration(timeoutRaw)
		if err != nil {
			return Config{}, fmt.Errorf("parse XNATS_TIMEOUT: %w", err)
		}
		if parsed <= 0 {
			return Config{}, fmt.Errorf("XNATS_TIMEOUT must be > 0, got %s", parsed)
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
