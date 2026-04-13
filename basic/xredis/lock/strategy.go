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
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// lockStrategy abstracts lock operations for single-node and Redlock modes.
type lockStrategy interface {
	tryAcquire(context.Context, *Lease) error
	release(context.Context, *Lease) error
	refresh(context.Context, *Lease) error
}

// singleNodeStrategy delegates lock operations to a single Redis node.
type singleNodeStrategy struct {
	client redis.UniversalClient
}

// tryAcquire uses SET NX to atomically set the key only if it does not exist.
func (s *singleNodeStrategy) tryAcquire(ctx context.Context, lease *Lease) error {
	obtained, err := s.client.SetNX(ctx, lease.fullKey, lease.token, lease.ttl).Result()
	if err != nil {
		return err
	}
	if !obtained {
		return ErrNotObtained
	}
	return nil
}

// release runs a Lua script that deletes the key only if the token matches.
func (s *singleNodeStrategy) release(ctx context.Context, lease *Lease) error {
	deleted, err := releaseScript.Run(ctx, s.client, []string{lease.fullKey}, lease.token).Int64()
	if err != nil {
		return err
	}
	if deleted == 0 {
		return ErrLockNotHeld
	}
	return nil
}

// refresh runs a Lua script that extends the TTL only if the token matches.
func (s *singleNodeStrategy) refresh(ctx context.Context, lease *Lease) error {
	extended, err := refreshScript.Run(
		ctx,
		s.client,
		[]string{lease.fullKey},
		lease.token,
		lease.ttl.Milliseconds(),
	).Int64()
	if err != nil {
		return err
	}
	if extended == 0 {
		return ErrLockNotHeld
	}
	return nil
}

// redlockStrategy implements the Redlock algorithm across multiple Redis nodes.
type redlockStrategy struct {
	nodes          []redis.UniversalClient
	quorum         int
	perNodeTimeout time.Duration
	clockDrift     time.Duration
}

// newRedlockStrategy builds a Redlock strategy with the primary node plus peers.
func newRedlockStrategy(primary redis.UniversalClient, cfg RedlockConfig) *redlockStrategy {
	nodes := make([]redis.UniversalClient, 0, 1+len(cfg.Peers))
	nodes = append(nodes, primary)
	nodes = append(nodes, cfg.Peers...)

	return &redlockStrategy{
		nodes:          nodes,
		quorum:         len(nodes)/2 + 1,
		perNodeTimeout: cfg.PerNodeTimeout,
		clockDrift:     cfg.ClockDrift,
	}
}

// tryAcquire attempts to acquire the lock on a quorum of nodes and checks
// that the remaining validity is positive after accounting for elapsed time
// and clock drift.
func (s *redlockStrategy) tryAcquire(ctx context.Context, lease *Lease) error {
	startedAt := time.Now()
	successCount := s.countSuccesses(
		ctx,
		func(opCtx context.Context, client redis.UniversalClient) bool {
			ok, err := client.SetNX(opCtx, lease.fullKey, lease.token, lease.ttl).Result()
			return err == nil && ok
		},
	)
	elapsed := time.Since(startedAt)
	validity := lease.ttl - elapsed - s.clockDrift

	if successCount >= s.quorum && validity > 0 {
		return nil
	}

	s.cleanupAcquire(lease)
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrNotObtained
}

// release runs the release Lua script on all nodes and requires quorum success.
func (s *redlockStrategy) release(ctx context.Context, lease *Lease) error {
	successCount := s.countSuccesses(
		ctx,
		func(opCtx context.Context, client redis.UniversalClient) bool {
			deleted, err := releaseScript.Run(opCtx, client, []string{lease.fullKey}, lease.token).
				Int64()
			return err == nil && deleted == 1
		},
	)
	if successCount < s.quorum {
		return ErrLockNotHeld
	}
	return nil
}

// refresh runs the refresh Lua script on all nodes and requires quorum success.
func (s *redlockStrategy) refresh(ctx context.Context, lease *Lease) error {
	successCount := s.countSuccesses(
		ctx,
		func(opCtx context.Context, client redis.UniversalClient) bool {
			extended, err := refreshScript.Run(
				opCtx,
				client,
				[]string{lease.fullKey},
				lease.token,
				lease.ttl.Milliseconds(),
			).Int64()
			return err == nil && extended == 1
		},
	)
	if successCount < s.quorum {
		return ErrLockNotHeld
	}
	return nil
}

// cleanupAcquire rolls back a failed acquire by releasing the lock on all nodes.
func (s *redlockStrategy) cleanupAcquire(lease *Lease) {
	s.countSuccesses(
		context.Background(),
		func(opCtx context.Context, client redis.UniversalClient) bool {
			deleted, err := releaseScript.Run(opCtx, client, []string{lease.fullKey}, lease.token).
				Int64()
			return err == nil && deleted == 1
		},
	)
}

// countSuccesses runs fn concurrently on all nodes with per-node timeouts
// and returns the number of successful results.
func (s *redlockStrategy) countSuccesses(
	ctx context.Context,
	fn func(context.Context, redis.UniversalClient) bool,
) int {
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		success int
	)

	for _, node := range s.nodes {
		wg.Add(1)
		go func(client redis.UniversalClient) {
			defer wg.Done()

			opCtx, cancel := context.WithTimeout(ctx, s.perNodeTimeout)
			defer cancel()

			if !fn(opCtx, client) {
				return
			}

			mu.Lock()
			success++
			mu.Unlock()
		}(node)
	}

	wg.Wait()
	return success
}

// releaseScript atomically deletes the key only when the stored token matches,
// preventing another holder from releasing a lock it does not own.
var releaseScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
end
return 0
`)

// refreshScript atomically extends the TTL only when the stored token matches.
var refreshScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("pexpire", KEYS[1], ARGV[2])
end
return 0
`)
