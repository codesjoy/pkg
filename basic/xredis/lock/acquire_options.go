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
)

// AcquireOption customizes one acquire attempt.
type AcquireOption func(*acquireConfig) error

type acquireConfig struct {
	autoRenew         bool
	autoRenewInterval time.Duration
	intervalSet       bool
}

// WithAutoRenew enables background auto-renew with the default interval `ttl / 3`.
func WithAutoRenew() AcquireOption {
	return func(cfg *acquireConfig) error {
		if cfg == nil {
			return nil
		}
		cfg.autoRenew = true
		return nil
	}
}

// WithAutoRenewInterval enables background auto-renew with an explicit interval.
func WithAutoRenewInterval(interval time.Duration) AcquireOption {
	return func(cfg *acquireConfig) error {
		if cfg == nil {
			return nil
		}
		cfg.autoRenew = true
		cfg.autoRenewInterval = interval
		cfg.intervalSet = true
		return nil
	}
}

func buildAcquireConfig(ttl time.Duration, opts []AcquireOption) (acquireConfig, error) {
	cfg := acquireConfig{}

	for idx, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&cfg); err != nil {
			return acquireConfig{}, fmt.Errorf("apply acquire option #%d: %w", idx, err)
		}
	}

	if !cfg.autoRenew {
		return cfg, nil
	}

	if !cfg.intervalSet {
		cfg.autoRenewInterval = ttl / 3
	}
	if cfg.autoRenewInterval <= 0 || cfg.autoRenewInterval >= ttl {
		return acquireConfig{}, ErrInvalidKeepAliveInterval
	}

	return cfg, nil
}
