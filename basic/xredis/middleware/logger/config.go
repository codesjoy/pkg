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

package logger

import (
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultSlowThreshold = 200 * time.Millisecond

// Config controls logger middleware behavior.
type Config struct {
	// Logger is the slog logger instance.
	Logger *slog.Logger
	// SlowThreshold defines the duration above which commands are logged as slow.
	SlowThreshold time.Duration
	// LogArgs controls whether command args are included in logs.
	LogArgs bool
	// CommandFilter returns true to skip logging a command.
	CommandFilter func(redis.Cmder) bool
}

// DefaultConfig returns the default logger middleware config.
func DefaultConfig() Config {
	return Config{
		Logger:        slog.Default(),
		SlowThreshold: defaultSlowThreshold,
		LogArgs:       false,
	}
}

func normalizeConfig(cfg Config) Config {
	normalized := cfg
	if normalized.Logger == nil {
		normalized.Logger = slog.Default()
	}
	if normalized.SlowThreshold <= 0 {
		normalized.SlowThreshold = defaultSlowThreshold
	}
	return normalized
}
