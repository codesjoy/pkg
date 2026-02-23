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

package xgo

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGo(t *testing.T) {
	t.Parallel()

	done := make(chan struct{})
	Go(func() {
		close(done)
	})

	waitDone(t, done)
}

func TestGoWithCtx(t *testing.T) {
	t.Parallel()

	type ctxKey string
	const key ctxKey = "request_id"

	ctx := context.WithValue(context.Background(), key, "req-1")
	done := make(chan struct{})

	GoWithCtx(ctx, func(got context.Context) {
		value := got.Value(key)
		if value != "req-1" {
			t.Errorf("context value = %v, want req-1", value)
		}
		close(done)
	})

	waitDone(t, done)
}

func TestGoWithCtx_NilContext(t *testing.T) {
	t.Parallel()

	done := make(chan struct{})
	//nolint:staticcheck // Intentionally pass nil to verify fallback to context.Background().
	Default().GoWithCtx(nil, func(ctx context.Context) {
		if ctx == nil {
			t.Error("context should default to context.Background()")
		}
		close(done)
	})

	waitDone(t, done)
}

func TestRunnerPanicHandler(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	panicCh := make(chan PanicInfo, 1)
	runner := New(
		WithLogger(nil),
		WithPanicHandler(func(info PanicInfo) {
			panicCh <- info
		}),
	)

	runner.GoWithCtx(ctx, func(context.Context) {
		panic("boom")
	})

	select {
	case info := <-panicCh:
		if info.Recovered != "boom" {
			t.Fatalf("panic value = %v, want boom", info.Recovered)
		}
		if len(info.Stack) == 0 {
			t.Fatal("stack should not be empty")
		}
		if info.Ctx != ctx {
			t.Fatal("panic context should match original context")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for panic handler")
	}
}

func TestRunnerLogger(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&out, nil))
	done := make(chan struct{})

	runner := New(
		WithLogger(logger),
		WithPanicHandler(func(PanicInfo) {
			close(done)
		}),
	)

	runner.Go(func() {
		panic("boom")
	})

	waitDone(t, done)

	logOutput := out.String()
	if !strings.Contains(logOutput, "\"msg\":\"goroutine panic\"") {
		t.Fatalf("logger output missing panic message: %s", logOutput)
	}
	if !strings.Contains(logOutput, "\"panic\":\"boom\"") {
		t.Fatalf("logger output missing panic value: %s", logOutput)
	}
}

func TestRunnerGo_Concurrent(t *testing.T) {
	t.Parallel()

	const workers = 50
	var wg sync.WaitGroup
	wg.Add(workers)

	runner := New(WithLogger(nil))
	for i := 0; i < workers; i++ {
		runner.Go(func() {
			wg.Done()
		})
	}

	waitWG(t, &wg)
}

func waitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for goroutine completion")
	}
}

func waitWG(t *testing.T, wg *sync.WaitGroup) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	waitDone(t, done)
}
