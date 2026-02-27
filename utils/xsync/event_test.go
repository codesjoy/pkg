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

package xsync

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEventStateAndFire(t *testing.T) {
	t.Parallel()

	e := NewEvent()
	if e.HasFired() {
		t.Fatal("HasFired() = true; want false")
	}

	select {
	case <-e.Done():
		t.Fatal("Done() closed before fire")
	default:
	}

	if !e.Fire() {
		t.Fatal("first Fire() = false; want true")
	}
	if !e.HasFired() {
		t.Fatal("HasFired() = false; want true")
	}

	if e.Fire() {
		t.Fatal("second Fire() = true; want false")
	}

	select {
	case <-e.Done():
	default:
		t.Fatal("Done() still open after fire")
	}
}

func TestEventConcurrentFire(t *testing.T) {
	t.Parallel()

	e := NewEvent()
	const workers = 64

	var firedCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			if e.Fire() {
				firedCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := firedCount.Load(); got != 1 {
		t.Fatalf("Fire() true count = %d; want 1", got)
	}
}

func TestEventWait(t *testing.T) {
	t.Parallel()

	t.Run("wait success", func(t *testing.T) {
		t.Parallel()

		e := NewEvent()
		go func() {
			time.Sleep(10 * time.Millisecond)
			e.Fire()
		}()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		if err := e.Wait(ctx); err != nil {
			t.Fatalf("Wait() error = %v; want nil", err)
		}
	})

	t.Run("wait canceled", func(t *testing.T) {
		t.Parallel()

		e := NewEvent()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		err := e.Wait(ctx)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Wait() error = %v; want %v", err, context.DeadlineExceeded)
		}
	})
}
