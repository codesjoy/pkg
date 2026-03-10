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
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		_, err := New(nil, Config{})
		require.ErrorIs(t, err, ErrNilClient)
	})

	t.Run("normalizes defaults", func(t *testing.T) {
		client := newMiniRedisClient(t)

		locker, err := New(client, Config{})
		require.NoError(t, err)
		require.NotNil(t, locker)
		require.Equal(t, defaultPrefix, locker.cfg.Prefix)
		require.Equal(t, defaultRetryInterval, locker.cfg.RetryInterval)
		require.Equal(t, defaultRetryJitter, locker.cfg.RetryJitter)
	})
}

func TestConfigValidate(t *testing.T) {
	t.Run("negative retry interval", func(t *testing.T) {
		cfg := Config{RetryInterval: -time.Second}
		require.ErrorIs(t, cfg.Validate(), ErrInvalidRetryInterval)
	})

	t.Run("negative retry jitter", func(t *testing.T) {
		cfg := Config{RetryJitter: -time.Second}
		require.ErrorIs(t, cfg.Validate(), ErrInvalidRetryInterval)
	})

	t.Run("explicit zero jitter is kept when interval is set", func(t *testing.T) {
		cfg := Config{RetryInterval: 20 * time.Millisecond}
		require.NoError(t, cfg.Validate())
		require.Zero(t, cfg.RetryJitter)
	})
}

func TestTryAcquireReleaseAndReacquire(t *testing.T) {
	ctx := context.Background()
	locker, client, _ := newMiniLocker(t, Config{RetryInterval: 5 * time.Millisecond})

	lease, err := locker.TryAcquire(ctx, "job", 200*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, "job", lease.Key())
	require.Equal(t, 200*time.Millisecond, lease.TTL())

	_, err = locker.TryAcquire(ctx, "job", 200*time.Millisecond)
	require.ErrorIs(t, err, ErrNotObtained)

	require.NoError(t, lease.Release(ctx))
	requireDoneClosed(t, lease.Done())
	require.NoError(t, lease.Err())
	require.ErrorIs(t, lease.Release(ctx), ErrLockNotHeld)

	reacquired, err := locker.TryAcquire(ctx, "job", 200*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, reacquired)
	require.NoError(t, reacquired.Release(ctx))

	exists, err := client.Exists(ctx, locker.prefixedKey("job")).Result()
	require.NoError(t, err)
	require.Zero(t, exists)
}

func TestAcquireWaitsUntilReleased(t *testing.T) {
	ctx := context.Background()
	locker1, locker2, _ := newMiniLockerPair(
		t,
		Config{RetryInterval: 5 * time.Millisecond, RetryJitter: 0},
		Config{RetryInterval: 5 * time.Millisecond, RetryJitter: 0},
	)

	first, err := locker1.TryAcquire(ctx, "job", 250*time.Millisecond)
	require.NoError(t, err)

	resultCh := make(chan *Lease, 1)
	errCh := make(chan error, 1)
	go func() {
		lease, acquireErr := locker2.Acquire(ctx, "job", 250*time.Millisecond)
		if acquireErr != nil {
			errCh <- acquireErr
			return
		}
		resultCh <- lease
	}()

	time.Sleep(30 * time.Millisecond)
	require.NoError(t, first.Release(ctx))

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case lease := <-resultCh:
		require.NotNil(t, lease)
		require.NoError(t, lease.Release(ctx))
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for acquire")
	}
}

func TestAcquireRespectsContextCancellation(t *testing.T) {
	ctx := context.Background()
	locker1, locker2, _ := newMiniLockerPair(
		t,
		Config{RetryInterval: 5 * time.Millisecond, RetryJitter: 0},
		Config{RetryInterval: 5 * time.Millisecond, RetryJitter: 0},
	)

	lease, err := locker1.TryAcquire(ctx, "job", 250*time.Millisecond)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = lease.Release(context.Background())
	})

	waitCtx, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer cancel()

	_, err = locker2.Acquire(waitCtx, "job", 250*time.Millisecond)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled))
}

func TestReleaseOnlyDeletesOwnedLock(t *testing.T) {
	ctx := context.Background()
	locker, client, _ := newMiniLocker(t, Config{})

	lease, err := locker.TryAcquire(ctx, "job", 200*time.Millisecond)
	require.NoError(t, err)

	require.NoError(t, client.Set(ctx, locker.prefixedKey("job"), "other-owner", time.Second).Err())

	err = lease.Release(ctx)
	require.ErrorIs(t, err, ErrLockNotHeld)

	value, getErr := client.Get(ctx, locker.prefixedKey("job")).Result()
	require.NoError(t, getErr)
	require.Equal(t, "other-owner", value)
	require.ErrorIs(t, lease.Release(ctx), ErrLockNotHeld)
}

func TestRefreshExtendsTTLAndDetectsLostOwnership(t *testing.T) {
	ctx := context.Background()
	locker, client, mr := newMiniLocker(t, Config{})

	lease, err := locker.TryAcquire(ctx, "job", 200*time.Millisecond)
	require.NoError(t, err)

	mr.FastForward(150 * time.Millisecond)
	require.NoError(t, lease.Refresh(ctx))

	ttl, err := client.PTTL(ctx, locker.prefixedKey("job")).Result()
	require.NoError(t, err)
	require.Greater(t, ttl, 100*time.Millisecond)

	require.NoError(t, client.Set(ctx, locker.prefixedKey("job"), "other-owner", time.Second).Err())

	err = lease.Refresh(ctx)
	require.ErrorIs(t, err, ErrLockNotHeld)
	require.ErrorIs(t, lease.Refresh(ctx), ErrLockNotHeld)
}

func TestKeepAliveKeepsLockAliveAndStopsOnCancel(t *testing.T) {
	ctx := context.Background()
	locker, client, mr := newMiniLocker(t, Config{})

	lease, err := locker.TryAcquire(ctx, "job", 120*time.Millisecond)
	require.NoError(t, err)

	keepAliveCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- lease.KeepAlive(keepAliveCtx, 30*time.Millisecond)
	}()

	mr.FastForward(100 * time.Millisecond)
	require.Eventually(t, func() bool {
		ttl, err := client.PTTL(ctx, locker.prefixedKey("job")).Result()
		require.NoError(t, err)
		return ttl > 80*time.Millisecond
	}, time.Second, 10*time.Millisecond)

	exists, err := client.Exists(ctx, locker.prefixedKey("job")).Result()
	require.NoError(t, err)
	require.EqualValues(t, 1, exists)

	cancel()
	require.NoError(t, <-done)
	requireDoneOpen(t, lease.Done())
	require.NoError(t, lease.Err())

	mr.FastForward(150 * time.Millisecond)

	exists, err = client.Exists(ctx, locker.prefixedKey("job")).Result()
	require.NoError(t, err)
	require.Zero(t, exists)
}

func TestExpiredLeaseReturnsErrLockNotHeld(t *testing.T) {
	ctx := context.Background()
	locker, _, mr := newMiniLocker(t, Config{})

	lease, err := locker.TryAcquire(ctx, "job", 50*time.Millisecond)
	require.NoError(t, err)

	mr.FastForward(60 * time.Millisecond)
	require.ErrorIs(t, lease.Release(ctx), ErrLockNotHeld)
	require.ErrorIs(t, lease.Refresh(ctx), ErrLockNotHeld)
	requireDoneClosed(t, lease.Done())
	require.ErrorIs(t, lease.Err(), ErrLockNotHeld)
}

func TestValidateAcquireInputs(t *testing.T) {
	ctx := context.Background()
	locker, _, _ := newMiniLocker(t, Config{})

	_, err := locker.TryAcquire(ctx, "", time.Second)
	require.ErrorIs(t, err, ErrEmptyKey)

	_, err = locker.TryAcquire(ctx, "job", 0)
	require.ErrorIs(t, err, ErrInvalidTTL)

	var nilLocker *Locker
	_, err = nilLocker.TryAcquire(ctx, "job", time.Second)
	require.ErrorIs(t, err, ErrNilClient)
}

func TestKeepAliveValidatesInterval(t *testing.T) {
	ctx := context.Background()
	locker, _, _ := newMiniLocker(t, Config{})

	lease, err := locker.TryAcquire(ctx, "job", 100*time.Millisecond)
	require.NoError(t, err)

	require.ErrorIs(t, lease.KeepAlive(ctx, 0), ErrInvalidKeepAliveInterval)
	require.ErrorIs(t, lease.KeepAlive(ctx, 100*time.Millisecond), ErrInvalidKeepAliveInterval)
	require.NoError(t, lease.Release(ctx))
}

func TestAcquireWithAutoRenewKeepsLockAlive(t *testing.T) {
	ctx := context.Background()
	locker, client, _ := newMiniLocker(t, Config{})

	lease, err := locker.Acquire(ctx, "job", 250*time.Millisecond, WithAutoRenew())
	require.NoError(t, err)

	time.Sleep(420 * time.Millisecond)

	exists, err := client.Exists(ctx, locker.prefixedKey("job")).Result()
	require.NoError(t, err)
	require.EqualValues(t, 1, exists)

	require.NoError(t, lease.Release(ctx))
	requireDoneClosed(t, lease.Done())
	require.NoError(t, lease.Err())
}

func TestTryAcquireWithAutoRenewIntervalImplicitlyEnablesAutoRenew(t *testing.T) {
	ctx := context.Background()
	locker, client, _ := newMiniLocker(t, Config{})

	lease, err := locker.TryAcquire(
		ctx,
		"job",
		250*time.Millisecond,
		WithAutoRenewInterval(50*time.Millisecond),
	)
	require.NoError(t, err)

	time.Sleep(420 * time.Millisecond)

	exists, err := client.Exists(ctx, locker.prefixedKey("job")).Result()
	require.NoError(t, err)
	require.EqualValues(t, 1, exists)

	require.NoError(t, lease.Release(ctx))
	requireDoneClosed(t, lease.Done())
	require.NoError(t, lease.Err())
}

func TestAcquireWithAutoRenewIntervalValidatesInput(t *testing.T) {
	ctx := context.Background()
	locker, _, _ := newMiniLocker(t, Config{})

	_, err := locker.TryAcquire(
		ctx,
		"job",
		100*time.Millisecond,
		WithAutoRenewInterval(100*time.Millisecond),
	)
	require.ErrorIs(t, err, ErrInvalidKeepAliveInterval)
}

func TestAutoRenewFailureClosesDoneAndReportsErr(t *testing.T) {
	ctx := context.Background()
	locker, client, _ := newMiniLocker(t, Config{})

	lease, err := locker.TryAcquire(
		ctx,
		"job",
		150*time.Millisecond,
		WithAutoRenewInterval(20*time.Millisecond),
	)
	require.NoError(t, err)

	require.NoError(t, client.Set(ctx, locker.prefixedKey("job"), "other-owner", time.Second).Err())

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

func requireDoneClosed(t *testing.T, done <-chan struct{}) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("expected done channel to be closed")
	}
}

func requireDoneOpen(t *testing.T, done <-chan struct{}) {
	t.Helper()

	select {
	case <-done:
		t.Fatal("expected done channel to remain open")
	default:
	}
}

func newMiniLocker(
	t *testing.T,
	cfg Config,
) (*Locker, redis.UniversalClient, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := newMiniRedisClientForAddr(t, mr.Addr())

	locker, err := New(client, cfg)
	require.NoError(t, err)
	return locker, client, mr
}

func newMiniRedisClient(t *testing.T) redis.UniversalClient {
	t.Helper()

	mr := miniredis.RunT(t)
	return newMiniRedisClientForAddr(t, mr.Addr())
}

func newMiniLockerPair(
	t *testing.T,
	cfg1 Config,
	cfg2 Config,
) (*Locker, *Locker, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client1 := newMiniRedisClientForAddr(t, mr.Addr())
	client2 := newMiniRedisClientForAddr(t, mr.Addr())

	locker1, err := New(client1, cfg1)
	require.NoError(t, err)

	locker2, err := New(client2, cfg2)
	require.NoError(t, err)

	return locker1, locker2, mr
}

func newMiniRedisClientForAddr(t *testing.T, addr string) redis.UniversalClient {
	t.Helper()

	client := redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs: []string{addr},
	})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	return client
}
