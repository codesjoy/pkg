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
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// Locker coordinates lock acquisition against Redis.
type Locker struct {
	client   redis.UniversalClient
	cfg      Config
	strategy lockStrategy
}

// New creates a locker backed by redis.UniversalClient.
func New(client redis.UniversalClient, cfg Config) (*Locker, error) {
	if isNilClient(client) {
		return nil, ErrNilClient
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	strategy, err := newLockStrategy(client, cfg)
	if err != nil {
		return nil, err
	}

	return &Locker{client: client, cfg: cfg, strategy: strategy}, nil
}

// TryAcquire tries to obtain a lock once.
func (l *Locker) TryAcquire(
	ctx context.Context,
	key string,
	ttl time.Duration,
	opts ...AcquireOption,
) (*Lease, error) {
	if err := l.validateAcquireInput(key, ttl); err != nil {
		return nil, err
	}

	acquireCfg, err := buildAcquireConfig(ttl, opts)
	if err != nil {
		return nil, err
	}

	return l.tryAcquire(ctx, key, ttl, acquireCfg)
}

// Acquire keeps retrying until the lock is obtained or ctx is done.
func (l *Locker) Acquire(
	ctx context.Context,
	key string,
	ttl time.Duration,
	opts ...AcquireOption,
) (*Lease, error) {
	if err := l.validateAcquireInput(key, ttl); err != nil {
		return nil, err
	}

	acquireCfg, err := buildAcquireConfig(ttl, opts)
	if err != nil {
		return nil, err
	}

	for {
		lease, err := l.tryAcquire(ctx, key, ttl, acquireCfg)
		if !errors.Is(err, ErrNotObtained) {
			return lease, err
		}

		if err := sleepContext(ctx, l.retryDelay()); err != nil {
			return nil, err
		}
	}
}

func (l *Locker) tryAcquire(
	ctx context.Context,
	key string,
	ttl time.Duration,
	acquireCfg acquireConfig,
) (*Lease, error) {
	token, err := newToken()
	if err != nil {
		return nil, err
	}

	fullKey := l.prefixedKey(key)
	lease := newLease(l, key, fullKey, ttl, token)
	if err := l.strategy.tryAcquire(ctx, lease); err != nil {
		return nil, err
	}
	if !acquireCfg.autoRenew {
		return lease, nil
	}
	if err := lease.startAutoRenew(acquireCfg.autoRenewInterval); err != nil {
		_ = lease.Release(context.Background())
		return nil, err
	}

	return lease, nil
}

func newLease(
	locker *Locker,
	key string,
	fullKey string,
	ttl time.Duration,
	token string,
) *Lease {
	return &Lease{
		locker:  locker,
		key:     key,
		fullKey: fullKey,
		ttl:     ttl,
		token:   token,
		done:    make(chan struct{}),
	}
}

func newLockStrategy(client redis.UniversalClient, cfg Config) (lockStrategy, error) {
	if cfg.Redlock == nil {
		return &singleNodeStrategy{client: client}, nil
	}
	return newRedlockStrategy(client, *cfg.Redlock), nil
}

func (l *Locker) validateAcquireInput(key string, ttl time.Duration) error {
	if l == nil || isNilClient(l.client) || l.strategy == nil {
		return ErrNilClient
	}
	if key == "" {
		return ErrEmptyKey
	}
	if ttl <= 0 {
		return ErrInvalidTTL
	}
	return nil
}

func (l *Locker) prefixedKey(key string) string {
	return l.cfg.Prefix + key
}

func (l *Locker) retryDelay() time.Duration {
	return l.cfg.RetryInterval + randomDuration(l.cfg.RetryJitter)
}
