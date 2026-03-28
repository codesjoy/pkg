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

package scope

import (
	"context"
	"errors"
	"testing"
)

type wrongStateKey struct{}

func testNilContext() context.Context {
	return nil
}

func TestIntoContextUsesBackgroundWhenContextIsNil(t *testing.T) {
	state := New()

	ctx := IntoContext(testNilContext(), state)
	got, ok := FromContext(ctx)
	if !ok {
		t.Fatal("FromContext() ok = false, want true")
	}
	if got != state {
		t.Fatal("FromContext() state mismatch")
	}
}

func TestFromContextHandlesMissingAndWrongValues(t *testing.T) {
	if got, ok := FromContext(testNilContext()); ok || got != nil {
		t.Fatalf("FromContext(nil) = (%v, %v), want (nil, false)", got, ok)
	}

	ctx := context.Background()
	if got, ok := FromContext(ctx); ok || got != nil {
		t.Fatalf("FromContext(background) = (%v, %v), want (nil, false)", got, ok)
	}

	ctx = context.WithValue(context.Background(), wrongStateKey{}, New())
	if got, ok := FromContext(ctx); ok || got != nil {
		t.Fatalf("FromContext(wrong key) = (%v, %v), want (nil, false)", got, ok)
	}

	ctx = context.WithValue(context.Background(), stateKey{}, "wrong-type")
	if got, ok := FromContext(ctx); ok || got != nil {
		t.Fatalf("FromContext(wrong type) = (%v, %v), want (nil, false)", got, ok)
	}
}

func TestStateAddHandlesNilHooksAndClosedStates(t *testing.T) {
	state := New()

	if err := state.Add(nil); err != nil {
		t.Fatalf("Add(nil) error = %v", err)
	}
	if len(state.hooks) != 0 {
		t.Fatalf("hooks len = %d, want 0", len(state.hooks))
	}

	var nilState *State
	if err := nilState.Add(func(context.Context) error { return nil }); !errors.Is(err, ErrClosed) {
		t.Fatalf("nilState.Add() error = %v, want ErrClosed", err)
	}

	if err := state.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if err := state.Add(func(context.Context) error { return nil }); !errors.Is(err, ErrClosed) {
		t.Fatalf("Add() after Run error = %v, want ErrClosed", err)
	}
}

func TestStateRunIsIdempotentAndAggregatesErrors(t *testing.T) {
	state := New()
	order := make([]string, 0, 2)
	firstErr := errors.New("first")
	secondErr := errors.New("second")

	if err := state.Add(func(context.Context) error {
		order = append(order, "first")
		return firstErr
	}); err != nil {
		t.Fatalf("first Add() error = %v", err)
	}
	if err := state.Add(func(context.Context) error {
		order = append(order, "second")
		return secondErr
	}); err != nil {
		t.Fatalf("second Add() error = %v", err)
	}

	err := state.Run(context.Background())
	if !errors.Is(err, firstErr) {
		t.Fatalf("Run() error = %v, want joined firstErr", err)
	}
	if !errors.Is(err, secondErr) {
		t.Fatalf("Run() error = %v, want joined secondErr", err)
	}
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("hook order = %v, want [first second]", order)
	}

	if err := state.Run(context.Background()); err != nil {
		t.Fatalf("second Run() error = %v, want nil", err)
	}
	if len(order) != 2 {
		t.Fatalf("hook runs = %d, want 2 total", len(order))
	}
}

func TestStateRunAllowsNilAndClosedState(t *testing.T) {
	var nilState *State
	if err := nilState.Run(context.Background()); err != nil {
		t.Fatalf("nilState.Run() error = %v", err)
	}

	state := New()
	state.closed = true
	if err := state.Run(context.Background()); err != nil {
		t.Fatalf("closed Run() error = %v", err)
	}
}

func TestRunHandlesMissingScopeAndExecutesAttachedScope(t *testing.T) {
	if err := Run(context.Background(), context.Background()); err != nil {
		t.Fatalf("Run() without scope error = %v", err)
	}

	state := New()
	txCtx := IntoContext(context.Background(), state)
	hookCtx := context.WithValue(context.Background(), wrongStateKey{}, "hook")

	if err := state.Add(func(ctx context.Context) error {
		if got := ctx.Value(wrongStateKey{}); got != "hook" {
			t.Fatalf("hook ctx value = %v, want hook", got)
		}
		return nil
	}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if err := Run(txCtx, hookCtx); err != nil {
		t.Fatalf("Run() with scope error = %v", err)
	}
}
