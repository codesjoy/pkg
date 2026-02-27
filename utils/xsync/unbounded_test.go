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
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestUnboundedFIFO(t *testing.T) {
	t.Parallel()

	q := NewUnbounded[int]()
	for i := 0; i < 5; i++ {
		if err := q.Put(i); err != nil {
			t.Fatalf("Put(%d) error = %v; want nil", i, err)
		}
	}

	if got := q.Len(); got != 5 {
		t.Fatalf("Len() = %d; want 5", got)
	}

	for i := 0; i < 5; i++ {
		v, ok := q.Get()
		if !ok {
			t.Fatalf("Get() at index %d returned ok=false", i)
		}
		if v != i {
			t.Fatalf("Get() = %d; want %d", v, i)
		}
	}

	q.Close()
	if _, ok := q.Get(); ok {
		t.Fatal("Get() after close+drain returned ok=true; want false")
	}
}

func TestUnboundedTryGet(t *testing.T) {
	t.Parallel()

	q := NewUnbounded[string]()
	if _, ok := q.TryGet(); ok {
		t.Fatal("TryGet() on empty queue returned ok=true; want false")
	}

	if err := q.Put("a"); err != nil {
		t.Fatalf("Put() error = %v; want nil", err)
	}

	v, ok := q.TryGet()
	if !ok || v != "a" {
		t.Fatalf("TryGet() = (%q, %v); want (%q, true)", v, ok, "a")
	}
}

func TestUnboundedGetBlocksUntilPut(t *testing.T) {
	t.Parallel()

	q := NewUnbounded[int]()
	gotCh := make(chan int, 1)

	go func() {
		v, ok := q.Get()
		if ok {
			gotCh <- v
		}
	}()

	select {
	case <-gotCh:
		t.Fatal("Get() returned before Put()")
	case <-time.After(20 * time.Millisecond):
	}

	if err := q.Put(42); err != nil {
		t.Fatalf("Put() error = %v; want nil", err)
	}

	select {
	case got := <-gotCh:
		if got != 42 {
			t.Fatalf("Get() = %d; want 42", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for Get() to receive Put() value")
	}
}

func TestUnboundedCloseAndDrain(t *testing.T) {
	t.Parallel()

	q := NewUnbounded[int]()
	if err := q.Put(1); err != nil {
		t.Fatalf("Put(1) error = %v; want nil", err)
	}
	if err := q.Put(2); err != nil {
		t.Fatalf("Put(2) error = %v; want nil", err)
	}

	q.Close()
	q.Close() // idempotent

	if err := q.Put(3); !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("Put() error = %v; want %v", err, ErrQueueClosed)
	}

	v, ok := q.Get()
	if !ok || v != 1 {
		t.Fatalf("first Get() = (%d, %v); want (1, true)", v, ok)
	}
	v, ok = q.Get()
	if !ok || v != 2 {
		t.Fatalf("second Get() = (%d, %v); want (2, true)", v, ok)
	}
	if _, ok = q.Get(); ok {
		t.Fatal("Get() after drain returned ok=true; want false")
	}
}

func TestUnboundedConcurrentWriters(t *testing.T) {
	t.Parallel()

	q := NewUnbounded[int]()

	const writers = 8
	const writesPerWriter = 100
	const total = writers * writesPerWriter

	var readCount atomic.Int32
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			v, ok := q.Get()
			if !ok {
				return
			}
			if v < 0 || v >= writers {
				return
			}
			readCount.Add(1)
		}
	}()

	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		id := i
		go func() {
			defer wg.Done()
			for j := 0; j < writesPerWriter; j++ {
				if err := q.Put(id); err != nil {
					t.Errorf("Put() error = %v; want nil", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	q.Close()
	<-readerDone

	if got := int(readCount.Load()); got != total {
		t.Fatalf("read count = %d; want %d", got, total)
	}
	if got := q.Len(); got != 0 {
		t.Fatalf("Len() after drain = %d; want 0", got)
	}
}
