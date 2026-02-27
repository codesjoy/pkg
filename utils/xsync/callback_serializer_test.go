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
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	defaultTestTimeout      = 5 * time.Second
	defaultTestShortTimeout = 10 * time.Millisecond
)

func TestSerializerSubmitFIFO(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer cancel()

	s := NewSerializer(ctx)
	defer func() {
		s.Close()
		<-s.Done()
	}()

	const callbacks = 100
	scheduleOrderCh := make(chan int, callbacks)
	executionOrderCh := make(chan int, callbacks)

	var mu sync.Mutex
	for i := 0; i < callbacks; i++ {
		id := i
		go func() {
			mu.Lock()
			defer mu.Unlock()
			scheduleOrderCh <- id
			if err := s.Submit(func(context.Context) {
				executionOrderCh <- id
			}); err != nil {
				t.Errorf("Submit() error = %v; want nil", err)
			}
		}()
	}

	scheduleOrder := make([]int, callbacks)
	executionOrder := make([]int, callbacks)
	for i := 0; i < callbacks; i++ {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting schedule order: %v", ctx.Err())
		case id := <-scheduleOrderCh:
			scheduleOrder[i] = id
		}
	}
	for i := 0; i < callbacks; i++ {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting execution order: %v", ctx.Err())
		case id := <-executionOrderCh:
			executionOrder[i] = id
		}
	}

	if !slices.Equal(executionOrder, scheduleOrder) {
		t.Fatalf("execution order = %v; want %v", executionOrder, scheduleOrder)
	}
}

func TestSerializerSubmitConcurrentCompleteness(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer cancel()

	s := NewSerializer(ctx)
	defer func() {
		s.Close()
		<-s.Done()
	}()

	const callbacks = 200
	var executed atomic.Int32
	var wg sync.WaitGroup
	wg.Add(callbacks)

	for i := 0; i < callbacks; i++ {
		go func() {
			err := s.Submit(func(context.Context) {
				executed.Add(1)
				wg.Done()
			})
			if err != nil {
				t.Errorf("Submit() error = %v; want nil", err)
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("timeout waiting callbacks complete: %v", ctx.Err())
	}

	if got := int(executed.Load()); got != callbacks {
		t.Fatalf("executed callbacks = %d; want %d", got, callbacks)
	}
}

func TestSerializerCloseRejectsSubmitAndDrains(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer cancel()

	s := NewSerializer(ctx)

	executed := make(chan int, 2)
	if err := s.Submit(func(context.Context) { executed <- 1 }); err != nil {
		t.Fatalf("Submit(first) error = %v; want nil", err)
	}
	if err := s.Submit(func(context.Context) { executed <- 2 }); err != nil {
		t.Fatalf("Submit(second) error = %v; want nil", err)
	}

	s.Close()
	if err := s.Submit(func(context.Context) {}); !errors.Is(err, ErrSerializerClosed) {
		t.Fatalf("Submit(after close) error = %v; want %v", err, ErrSerializerClosed)
	}

	for i := 1; i <= 2; i++ {
		select {
		case got := <-executed:
			if got != i {
				t.Fatalf("executed callback = %d; want %d", got, i)
			}
		case <-ctx.Done():
			t.Fatalf("timeout waiting drained callback %d: %v", i, ctx.Err())
		}
	}

	select {
	case <-s.Done():
	case <-ctx.Done():
		t.Fatalf("timeout waiting serializer done: %v", ctx.Err())
	}
}

func TestSerializerContextCanceledRejectsSubmit(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	s := NewSerializer(ctx)
	cancel()

	deadlineCtx, stop := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer stop()

	if err := s.Submit(func(context.Context) {}); !errors.Is(err, ErrSerializerClosed) {
		t.Fatalf("Submit(canceled) error = %v; want %v", err, ErrSerializerClosed)
	}

	select {
	case <-s.Done():
	case <-deadlineCtx.Done():
		t.Fatalf("timeout waiting serializer done: %v", deadlineCtx.Err())
	}
}

func TestSerializerRecoversFromPanic(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer cancel()

	s := NewSerializer(ctx)

	executed := make(chan struct{}, 1)
	if err := s.Submit(func(context.Context) {
		panic("boom")
	}); err != nil {
		t.Fatalf("Submit(panic callback) error = %v; want nil", err)
	}
	if err := s.Submit(func(context.Context) {
		executed <- struct{}{}
	}); err != nil {
		t.Fatalf("Submit(follow-up callback) error = %v; want nil", err)
	}

	select {
	case <-executed:
	case <-ctx.Done():
		t.Fatalf("timeout waiting follow-up callback: %v", ctx.Err())
	}

	s.Close()
	select {
	case <-s.Done():
	case <-ctx.Done():
		t.Fatalf("timeout waiting serializer done: %v", ctx.Err())
	}
}

func TestSerializerCloseIdempotent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer cancel()

	s := NewSerializer(ctx)
	time.Sleep(defaultTestShortTimeout)
	s.Close()
	s.Close()

	select {
	case <-s.Done():
	case <-ctx.Done():
		t.Fatalf("timeout waiting serializer done: %v", ctx.Err())
	}
}
