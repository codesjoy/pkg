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

package transaction

import (
	"context"
	"errors"
	"testing"

	"github.com/codesjoy/pkg/basic/transaction/internal/scope"
)

func TestAfterCommitReturnsErrorWithoutScope(t *testing.T) {
	err := AfterCommit(context.Background(), func(context.Context) error { return nil })
	if !errors.Is(err, ErrAfterCommitOutsideTransaction) {
		t.Fatalf("AfterCommit() error = %v, want ErrAfterCommitOutsideTransaction", err)
	}
}

func TestAfterCommitReturnsErrorForNilHook(t *testing.T) {
	ctx := scope.IntoContext(context.Background(), scope.New())

	err := AfterCommit(ctx, nil)
	if !errors.Is(err, ErrNilHook) {
		t.Fatalf("AfterCommit() error = %v, want ErrNilHook", err)
	}
}

func TestAfterCommitRunsHooksInRegistrationOrder(t *testing.T) {
	txCtx := scope.IntoContext(context.Background(), scope.New())
	order := make([]string, 0, 2)

	if err := AfterCommit(txCtx, func(context.Context) error {
		order = append(order, "first")
		return nil
	}); err != nil {
		t.Fatalf("AfterCommit() error = %v", err)
	}
	if err := AfterCommit(txCtx, func(context.Context) error {
		order = append(order, "second")
		return nil
	}); err != nil {
		t.Fatalf("AfterCommit() error = %v", err)
	}

	if err := scope.Run(txCtx, context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("hook order = %v, want [first second]", order)
	}
}

func TestAfterCommitHooksRunOnlyOnce(t *testing.T) {
	txCtx := scope.IntoContext(context.Background(), scope.New())
	calls := 0

	if err := AfterCommit(txCtx, func(context.Context) error {
		calls++
		return nil
	}); err != nil {
		t.Fatalf("AfterCommit() error = %v", err)
	}

	if err := scope.Run(txCtx, context.Background()); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	if err := scope.Run(txCtx, context.Background()); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("hook calls = %d, want 1", calls)
	}
}

func TestAfterCommitReturnsErrorOnceScopeIsClosed(t *testing.T) {
	txCtx := scope.IntoContext(context.Background(), scope.New())

	if err := scope.Run(txCtx, context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	err := AfterCommit(txCtx, func(context.Context) error { return nil })
	if !errors.Is(err, ErrAfterCommitClosed) {
		t.Fatalf("AfterCommit() error = %v, want ErrAfterCommitClosed", err)
	}
}

func TestAfterCommitAggregatesHookErrors(t *testing.T) {
	txCtx := scope.IntoContext(context.Background(), scope.New())
	errFirst := errors.New("first hook failed")
	errSecond := errors.New("second hook failed")

	if err := AfterCommit(txCtx, func(context.Context) error { return errFirst }); err != nil {
		t.Fatalf("AfterCommit() error = %v", err)
	}
	if err := AfterCommit(txCtx, func(context.Context) error { return errSecond }); err != nil {
		t.Fatalf("AfterCommit() error = %v", err)
	}

	err := scope.Run(txCtx, context.Background())
	if !errors.Is(err, errFirst) {
		t.Fatalf("Run() error = %v, want joined errFirst", err)
	}
	if !errors.Is(err, errSecond) {
		t.Fatalf("Run() error = %v, want joined errSecond", err)
	}
}
