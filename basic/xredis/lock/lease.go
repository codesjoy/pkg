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
	"sync"
	"time"
)

// closedLeaseDone is a pre-closed channel used as the Done return value
// for nil Lease pointers.
var closedLeaseDone = func() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}()

// Lease represents an acquired lock instance.
type Lease struct {
	// locker is the Locker that created this lease.
	locker *Locker
	// key is the logical lock key provided by the caller.
	key string
	// fullKey includes the prefix and is the actual Redis key.
	fullKey string
	// ttl is the lock time-to-live.
	ttl time.Duration
	// token uniquely identifies the lock holder.
	token string

	// done is closed when the lease finishes (released or lost).
	done chan struct{}

	// finishOnce ensures finish logic runs exactly once.
	finishOnce sync.Once

	mu              sync.Mutex
	released        bool
	terminalErr     error
	autoRenewCancel context.CancelFunc
	autoRenewDone   chan struct{}
}

// Key returns the logical lock key.
func (l *Lease) Key() string {
	if l == nil {
		return ""
	}
	return l.key
}

// TTL returns the original lock TTL.
func (l *Lease) TTL() time.Duration {
	if l == nil {
		return 0
	}
	return l.ttl
}

// Done returns a channel closed when the lease finishes locally.
func (l *Lease) Done() <-chan struct{} {
	if l == nil {
		return closedLeaseDone
	}
	return l.done
}

// Err returns the terminal lease error once Done is closed.
func (l *Lease) Err() error {
	if l == nil {
		return ErrLockNotHeld
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	return l.terminalErr
}

// Release deletes the lock if the caller still owns it.
func (l *Lease) Release(ctx context.Context) error {
	if l == nil {
		return ErrLockNotHeld
	}

	// Stop the background auto-renew goroutine before releasing.
	l.stopAutoRenew()

	if err := l.ensureUsable(); err != nil {
		return err
	}

	err := l.locker.strategy.release(ctx, l)
	if err != nil {
		// Mark the lease as finished if the lock is no longer held.
		if errors.Is(err, ErrLockNotHeld) {
			l.finish(ErrLockNotHeld)
		}
		return err
	}

	l.finish(nil)
	return nil
}

// Refresh extends the lock TTL if the caller still owns it.
func (l *Lease) Refresh(ctx context.Context) error {
	if err := l.ensureUsable(); err != nil {
		return err
	}

	err := l.locker.strategy.refresh(ctx, l)
	if err != nil {
		if errors.Is(err, ErrLockNotHeld) {
			l.finish(ErrLockNotHeld)
		}
		return err
	}

	return nil
}

// KeepAlive keeps refreshing the lock until ctx is done or the lock is lost.
func (l *Lease) KeepAlive(ctx context.Context, interval time.Duration) error {
	if l == nil {
		return ErrLockNotHeld
	}
	if interval <= 0 || interval >= l.ttl {
		return ErrInvalidKeepAliveInterval
	}
	if err := l.ensureUsable(); err != nil {
		return err
	}

	return l.keepAliveLoop(ctx, interval)
}

// keepAliveLoop periodically refreshes the lock TTL until ctx is done or
// a refresh fails.
func (l *Lease) keepAliveLoop(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := l.Refresh(ctx); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

// startAutoRenew launches a background goroutine that periodically refreshes
// the lock TTL.
func (l *Lease) startAutoRenew(interval time.Duration) error {
	if l == nil {
		return ErrLockNotHeld
	}
	if interval <= 0 || interval >= l.ttl {
		return ErrInvalidKeepAliveInterval
	}
	if err := l.ensureUsable(); err != nil {
		return err
	}

	// Create an independent context so auto-renew is not tied to the caller's ctx.
	renewCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	// Register the cancel and done channel under the lock, checking for a race
	// with Release.
	l.mu.Lock()
	if l.released {
		l.mu.Unlock()
		cancel()
		return ErrLockNotHeld
	}
	l.autoRenewCancel = cancel
	l.autoRenewDone = done
	l.mu.Unlock()

	// Start the background keep-alive loop.
	go func() {
		defer close(done)
		defer l.clearAutoRenew(done)

		if err := l.keepAliveLoop(renewCtx, interval); err != nil {
			l.finish(err)
		}
	}()

	return nil
}

// stopAutoRenew cancels the auto-renew goroutine and waits for it to finish.
func (l *Lease) stopAutoRenew() {
	if l == nil {
		return
	}

	l.mu.Lock()
	cancel := l.autoRenewCancel
	done := l.autoRenewDone
	l.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// ensureUsable checks that the lease is still held and the locker is valid.
func (l *Lease) ensureUsable() error {
	if l == nil {
		return ErrLockNotHeld
	}
	if l.locker == nil || isNilClient(l.locker.client) || l.locker.strategy == nil {
		return ErrNilClient
	}
	if l.isReleased() {
		return ErrLockNotHeld
	}
	return nil
}

// isReleased returns whether the lease has been released or lost.
func (l *Lease) isReleased() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.released
}

// clearAutoRenew clears the auto-renew state if the done channel matches,
// preventing a stale goroutine from overwriting a newer auto-renew cycle.
func (l *Lease) clearAutoRenew(done chan struct{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.autoRenewDone == done {
		l.autoRenewDone = nil
		l.autoRenewCancel = nil
	}
}

// finish marks the lease as released with the given terminal error and
// closes the done channel. Safe to call multiple times via sync.Once.
func (l *Lease) finish(err error) {
	l.finishOnce.Do(func() {
		l.mu.Lock()
		l.released = true
		l.terminalErr = err
		l.mu.Unlock()
		close(l.done)
	})
}
