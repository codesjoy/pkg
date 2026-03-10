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
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestNewRedlockDefaults(t *testing.T) {
	_, clients := newMiniRedisClients(t, 3)

	locker, err := New(clients[0], Config{
		Redlock: &RedlockConfig{
			Peers: clients[1:],
		},
	})
	require.NoError(t, err)

	require.NotNil(t, locker.cfg.Redlock)
	require.Equal(t, defaultRedlockPerNodeTimeout, locker.cfg.Redlock.PerNodeTimeout)
	require.Equal(t, defaultRedlockClockDrift, locker.cfg.Redlock.ClockDrift)
}

func TestNewRedlockRequiresAtLeastThreeClients(t *testing.T) {
	_, clients := newMiniRedisClients(t, 2)

	_, err := New(clients[0], Config{
		Redlock: &RedlockConfig{
			Peers: clients[1:],
		},
	})
	require.ErrorContains(t, err, "at least 3 independent clients")
}

func TestNewRedlockRejectsNilPeer(t *testing.T) {
	_, clients := newMiniRedisClients(t, 3)
	var nilPeer redis.UniversalClient

	_, err := New(clients[0], Config{
		Redlock: &RedlockConfig{
			Peers: []redis.UniversalClient{clients[1], nilPeer},
		},
	})
	require.ErrorIs(t, err, ErrNilClient)
}

func TestRedlockAcquireReleaseAndRefresh(t *testing.T) {
	ctx := context.Background()
	servers, clients := newMiniRedisClients(t, 3)
	locker := newRedlockLocker(t, clients, nil)

	lease, err := locker.TryAcquire(ctx, "job", 300*time.Millisecond)
	require.NoError(t, err)

	for _, server := range servers {
		server.FastForward(220 * time.Millisecond)
	}
	require.NoError(t, lease.Refresh(ctx))

	for _, client := range clients {
		ttl, ttlErr := client.PTTL(ctx, locker.prefixedKey("job")).Result()
		require.NoError(t, ttlErr)
		require.Greater(t, ttl, 200*time.Millisecond)
	}

	require.NoError(t, lease.Release(ctx))
	requireDoneClosed(t, lease.Done())
	require.NoError(t, lease.Err())

	reacquired, err := locker.TryAcquire(ctx, "job", 300*time.Millisecond)
	require.NoError(t, err)
	require.NoError(t, reacquired.Release(ctx))
}

func TestRedlockQuorumFailureCleansUp(t *testing.T) {
	ctx := context.Background()
	_, clients := newMiniRedisClients(t, 3)
	locker := newRedlockLocker(t, clients, nil)

	require.NoError(
		t,
		clients[0].Set(ctx, locker.prefixedKey("job"), "other-owner", time.Second).Err(),
	)
	require.NoError(
		t,
		clients[1].Set(ctx, locker.prefixedKey("job"), "other-owner", time.Second).Err(),
	)

	_, err := locker.TryAcquire(ctx, "job", 300*time.Millisecond)
	require.ErrorIs(t, err, ErrNotObtained)

	value0, err := clients[0].Get(ctx, locker.prefixedKey("job")).Result()
	require.NoError(t, err)
	require.Equal(t, "other-owner", value0)

	value1, err := clients[1].Get(ctx, locker.prefixedKey("job")).Result()
	require.NoError(t, err)
	require.Equal(t, "other-owner", value1)

	exists, err := clients[2].Exists(ctx, locker.prefixedKey("job")).Result()
	require.NoError(t, err)
	require.Zero(t, exists)
}

func TestRedlockAcquireElapsedTTLFailure(t *testing.T) {
	ctx := context.Background()
	_, clients := newMiniRedisClients(t, 3, func(client redis.UniversalClient) {
		client.AddHook(&sleepHook{delay: 40 * time.Millisecond})
	})
	locker := newRedlockLocker(t, clients, &RedlockConfig{
		Peers:          clients[1:],
		PerNodeTimeout: 80 * time.Millisecond,
	})

	_, err := locker.TryAcquire(ctx, "job", 10*time.Millisecond)
	require.ErrorIs(t, err, ErrNotObtained)

	for _, client := range clients {
		exists, existsErr := client.Exists(ctx, locker.prefixedKey("job")).Result()
		require.NoError(t, existsErr)
		require.Zero(t, exists)
	}
}

func TestRedlockWithAutoRenewKeepsLockAlive(t *testing.T) {
	ctx := context.Background()
	_, clients := newMiniRedisClients(t, 3)
	locker := newRedlockLocker(t, clients, nil)

	lease, err := locker.TryAcquire(
		ctx,
		"job",
		250*time.Millisecond,
		WithAutoRenewInterval(50*time.Millisecond),
	)
	require.NoError(t, err)

	time.Sleep(420 * time.Millisecond)

	for _, client := range clients {
		exists, existsErr := client.Exists(ctx, locker.prefixedKey("job")).Result()
		require.NoError(t, existsErr)
		require.EqualValues(t, 1, exists)
	}

	require.NoError(t, lease.Release(ctx))
	requireDoneClosed(t, lease.Done())
	require.NoError(t, lease.Err())
}

func TestRedlockAutoRenewLosesQuorum(t *testing.T) {
	ctx := context.Background()
	_, clients := newMiniRedisClients(t, 3)
	locker := newRedlockLocker(t, clients, nil)

	lease, err := locker.TryAcquire(
		ctx,
		"job",
		250*time.Millisecond,
		WithAutoRenewInterval(30*time.Millisecond),
	)
	require.NoError(t, err)

	require.NoError(
		t,
		clients[0].Set(ctx, locker.prefixedKey("job"), "other-owner", time.Second).Err(),
	)
	require.NoError(
		t,
		clients[1].Set(ctx, locker.prefixedKey("job"), "other-owner", time.Second).Err(),
	)

	require.Eventually(t, func() bool {
		select {
		case <-lease.Done():
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	require.ErrorIs(t, lease.Err(), ErrLockNotHeld)
	require.ErrorIs(t, lease.Release(ctx), ErrLockNotHeld)
}

func newRedlockLocker(
	t *testing.T,
	clients []redis.UniversalClient,
	redlockCfg *RedlockConfig,
) *Locker {
	t.Helper()

	require.GreaterOrEqual(t, len(clients), 3)

	cfg := Config{}
	if redlockCfg == nil {
		cfg.Redlock = &RedlockConfig{Peers: clients[1:]}
	} else {
		copied := *redlockCfg
		if len(copied.Peers) == 0 {
			copied.Peers = clients[1:]
		}
		cfg.Redlock = &copied
	}

	locker, err := New(clients[0], cfg)
	require.NoError(t, err)
	return locker
}

func newMiniRedisClients(
	t *testing.T,
	count int,
	clientMutators ...func(redis.UniversalClient),
) ([]*miniredis.Miniredis, []redis.UniversalClient) {
	t.Helper()

	servers := make([]*miniredis.Miniredis, 0, count)
	clients := make([]redis.UniversalClient, 0, count)
	for i := 0; i < count; i++ {
		server := miniredis.RunT(t)
		client := newMiniRedisClientForAddr(t, server.Addr())
		for _, mutate := range clientMutators {
			if mutate != nil {
				mutate(client)
			}
		}
		servers = append(servers, server)
		clients = append(clients, client)
	}
	return servers, clients
}

type sleepHook struct {
	delay time.Duration
}

func (h *sleepHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h *sleepHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if h.delay > 0 && cmd != nil && cmd.Name() == "set" {
			time.Sleep(h.delay)
		}
		return next(ctx, cmd)
	}
}

func (h *sleepHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			if cmd != nil && cmd.Name() == "set" && h.delay > 0 {
				time.Sleep(h.delay)
				break
			}
		}
		return next(ctx, cmds)
	}
}

var _ redis.Hook = (*sleepHook)(nil)
