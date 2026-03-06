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

package lock

import (
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultPrefix                = "lock:"
	defaultRetryInterval         = 100 * time.Millisecond
	defaultRetryJitter           = 25 * time.Millisecond
	defaultRedlockPerNodeTimeout = 50 * time.Millisecond
	defaultRedlockClockDrift     = 2 * time.Millisecond
)

// Config controls locker defaults.
type Config struct {
	Prefix        string
	RetryInterval time.Duration
	RetryJitter   time.Duration
	Redlock       *RedlockConfig
}

// Validate validates and normalizes lock config.
func (c *Config) Validate() error {
	if c == nil {
		return nil
	}

	if c.Prefix == "" {
		c.Prefix = defaultPrefix
	}
	if c.RetryInterval < 0 {
		return ErrInvalidRetryInterval
	}
	if c.RetryJitter < 0 {
		return fmt.Errorf("%w: retry jitter must be non-negative", ErrInvalidRetryInterval)
	}

	if c.RetryInterval == 0 {
		c.RetryInterval = defaultRetryInterval
		if c.RetryJitter == 0 {
			c.RetryJitter = defaultRetryJitter
		}
	}
	if c.Redlock != nil {
		normalized := *c.Redlock
		c.Redlock = &normalized
		if err := c.Redlock.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// RedlockConfig controls optional Redlock multi-master behavior.
type RedlockConfig struct {
	Peers          []redis.UniversalClient
	PerNodeTimeout time.Duration
	ClockDrift     time.Duration
}

// Validate validates and normalizes Redlock config.
func (c *RedlockConfig) Validate() error {
	if c == nil {
		return nil
	}
	if len(c.Peers) < 2 {
		return fmt.Errorf("redlock requires at least 3 independent clients")
	}
	for idx, peer := range c.Peers {
		if isNilClient(peer) {
			return fmt.Errorf("redlock peer at index %d: %w", idx, ErrNilClient)
		}
	}
	if c.PerNodeTimeout < 0 {
		return fmt.Errorf("redlock per-node timeout must be non-negative")
	}
	if c.ClockDrift < 0 {
		return fmt.Errorf("redlock clock drift must be non-negative")
	}
	if c.PerNodeTimeout == 0 {
		c.PerNodeTimeout = defaultRedlockPerNodeTimeout
	}
	if c.ClockDrift == 0 {
		c.ClockDrift = defaultRedlockClockDrift
	}
	return nil
}
