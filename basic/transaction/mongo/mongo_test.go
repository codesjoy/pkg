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

package mongotx

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/codesjoy/pkg/basic/transaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
)

type testContextKey struct{}

func testNilContext() context.Context {
	return nil
}

func TestWithinRequiresClient(t *testing.T) {
	var runner *Runner

	err := runner.Within(context.Background(), func(context.Context) error { return nil })
	if !errors.Is(err, mongo.ErrClientDisconnected) {
		t.Fatalf("Within() error = %v, want mongo.ErrClientDisconnected", err)
	}
}

func TestWithinAllowsNilContextAndNilFunc(t *testing.T) {
	runner := New(nil)

	if err := runner.Within(testNilContext(), nil); err != nil {
		t.Fatalf("Within(nil, nil) error = %v", err)
	}
}

func TestWithinPassesDefaultConfigAndCallbackContext(t *testing.T) {
	original := startSessionRunner
	defer func() {
		startSessionRunner = original
	}()

	sessionOpt := options.Session().SetDefaultTransactionOptions(options.Transaction())
	txOpt := options.Transaction().SetReadConcern(readconcern.Majority())

	fake := &fakeTransactionRunner{}
	var gotSessionOptions []options.Lister[options.SessionOptions]
	startSessionRunner = func(
		_ *mongo.Client,
		opts ...options.Lister[options.SessionOptions],
	) (transactionSessionRunner, error) {
		gotSessionOptions = append([]options.Lister[options.SessionOptions](nil), opts...)
		return fake, nil
	}

	runner := New(
		&mongo.Client{},
		WithDefaultConfig(Config{
			SessionOptions:     []options.Lister[options.SessionOptions]{sessionOpt},
			TransactionOptions: []options.Lister[options.TransactionOptions]{txOpt},
		}),
	)

	ctx := context.WithValue(context.Background(), testContextKey{}, "outer")
	err := runner.Within(ctx, func(txCtx context.Context) error {
		if mongo.SessionFromContext(txCtx) == nil {
			t.Fatal("SessionFromContext(txCtx) = nil, want active session")
		}
		if got := txCtx.Value(testContextKey{}); got != "outer" {
			t.Fatalf("txCtx.Value() = %v, want outer", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Within() error = %v", err)
	}
	if len(gotSessionOptions) != 1 {
		t.Fatalf("session options len = %d, want 1", len(gotSessionOptions))
	}
	if reflect.ValueOf(gotSessionOptions[0]).Pointer() != reflect.ValueOf(sessionOpt).Pointer() {
		t.Fatal("session option pointer mismatch")
	}
	if len(fake.txOptions) != 1 {
		t.Fatalf("transaction options len = %d, want 1", len(fake.txOptions))
	}
	if reflect.ValueOf(fake.txOptions[0]).Pointer() != reflect.ValueOf(txOpt).Pointer() {
		t.Fatal("transaction option pointer mismatch")
	}
	if fake.endCtx != ctx {
		t.Fatal("EndSession() received unexpected context")
	}
}

func TestWithinReusesExistingTransaction(t *testing.T) {
	original := startSessionRunner
	defer func() {
		startSessionRunner = original
	}()

	startCalls := 0
	fake := &fakeTransactionRunner{}
	startSessionRunner = func(
		_ *mongo.Client,
		_ ...options.Lister[options.SessionOptions],
	) (transactionSessionRunner, error) {
		startCalls++
		return fake, nil
	}

	runner := New(&mongo.Client{})
	err := runner.Within(context.Background(), func(ctx context.Context) error {
		return runner.Within(ctx, func(inner context.Context) error {
			if mongo.SessionFromContext(inner) == nil {
				t.Fatal("SessionFromContext(inner) = nil, want active session")
			}
			return nil
		})
	})
	if err != nil {
		t.Fatalf("Within() error = %v", err)
	}
	if startCalls != 1 {
		t.Fatalf("startSessionRunner() calls = %d, want 1", startCalls)
	}
}

func TestWithinSkipsCommitHooksOnRollback(t *testing.T) {
	original := startSessionRunner
	defer func() {
		startSessionRunner = original
	}()

	fake := &fakeTransactionRunner{}
	startSessionRunner = func(
		_ *mongo.Client,
		_ ...options.Lister[options.SessionOptions],
	) (transactionSessionRunner, error) {
		return fake, nil
	}

	called := false
	sentinel := errors.New("rollback")
	runner := New(&mongo.Client{})

	err := runner.Within(context.Background(), func(ctx context.Context) error {
		if err := transaction.AfterCommit(ctx, func(context.Context) error {
			called = true
			return nil
		}); err != nil {
			t.Fatalf("AfterCommit() error = %v", err)
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Within() error = %v, want sentinel", err)
	}
	if called {
		t.Fatal("after-commit hook ran on rollback")
	}
}

func TestWithinRunsCommitHooksInOrderAfterCommit(t *testing.T) {
	original := startSessionRunner
	defer func() {
		startSessionRunner = original
	}()

	fake := &fakeTransactionRunner{}
	startSessionRunner = func(
		_ *mongo.Client,
		_ ...options.Lister[options.SessionOptions],
	) (transactionSessionRunner, error) {
		return fake, nil
	}

	order := make([]string, 0, 2)
	ctx := context.WithValue(context.Background(), testContextKey{}, "outer")
	runner := New(&mongo.Client{})

	err := runner.Within(ctx, func(txCtx context.Context) error {
		if err := transaction.AfterCommit(txCtx, func(hookCtx context.Context) error {
			if mongo.SessionFromContext(hookCtx) != nil {
				t.Fatal("SessionFromContext(hookCtx) != nil, want no transaction session")
			}
			if got := hookCtx.Value(testContextKey{}); got != "outer" {
				t.Fatalf("hookCtx.Value() = %v, want outer", got)
			}
			order = append(order, "first")
			return nil
		}); err != nil {
			t.Fatalf("AfterCommit() error = %v", err)
		}
		if err := transaction.AfterCommit(txCtx, func(context.Context) error {
			order = append(order, "second")
			return nil
		}); err != nil {
			t.Fatalf("AfterCommit() error = %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Within() error = %v", err)
	}
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("hook order = %v, want [first second]", order)
	}
}

func TestWithinReturnsJoinedAfterCommitErrors(t *testing.T) {
	original := startSessionRunner
	defer func() {
		startSessionRunner = original
	}()

	fake := &fakeTransactionRunner{}
	startSessionRunner = func(
		_ *mongo.Client,
		_ ...options.Lister[options.SessionOptions],
	) (transactionSessionRunner, error) {
		return fake, nil
	}

	hookErr := errors.New("hook failed")
	runner := New(&mongo.Client{})

	err := runner.Within(context.Background(), func(ctx context.Context) error {
		return transaction.AfterCommit(ctx, func(context.Context) error {
			return hookErr
		})
	})
	if !errors.Is(err, transaction.ErrAfterCommitFailed) {
		t.Fatalf("Within() error = %v, want ErrAfterCommitFailed", err)
	}
	if !errors.Is(err, hookErr) {
		t.Fatalf("Within() error = %v, want joined hookErr", err)
	}
}

func TestWithinReturnsStartSessionError(t *testing.T) {
	original := startSessionRunner
	defer func() {
		startSessionRunner = original
	}()

	sentinel := errors.New("start session failed")
	startSessionRunner = func(
		_ *mongo.Client,
		_ ...options.Lister[options.SessionOptions],
	) (transactionSessionRunner, error) {
		return nil, sentinel
	}

	runner := New(&mongo.Client{})
	err := runner.Within(context.Background(), func(context.Context) error { return nil })
	if !errors.Is(err, sentinel) {
		t.Fatalf("Within() error = %v, want sentinel", err)
	}
}

func TestWithinDoesNotReuseMarkerWithoutSession(t *testing.T) {
	original := startSessionRunner
	defer func() {
		startSessionRunner = original
	}()

	startCalls := 0
	fake := &fakeTransactionRunner{}
	client := &mongo.Client{}
	startSessionRunner = func(
		_ *mongo.Client,
		_ ...options.Lister[options.SessionOptions],
	) (transactionSessionRunner, error) {
		startCalls++
		return fake, nil
	}

	runner := New(client)
	ctx := withMarker(context.Background(), client)
	err := runner.Within(ctx, func(txCtx context.Context) error {
		if mongo.SessionFromContext(txCtx) == nil {
			t.Fatal("SessionFromContext(txCtx) = nil, want active session")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Within() error = %v", err)
	}
	if startCalls != 1 {
		t.Fatalf("startSessionRunner() calls = %d, want 1", startCalls)
	}
}

func TestWithinDoesNotReuseMarkerForDifferentClient(t *testing.T) {
	original := startSessionRunner
	defer func() {
		startSessionRunner = original
	}()

	startCalls := 0
	fake := &fakeTransactionRunner{}
	startSessionRunner = func(
		_ *mongo.Client,
		_ ...options.Lister[options.SessionOptions],
	) (transactionSessionRunner, error) {
		startCalls++
		return fake, nil
	}

	runner := New(&mongo.Client{})
	ctx := withMarker(context.Background(), &mongo.Client{})
	err := mongo.WithSession(ctx, new(mongo.Session), func(sessionCtx context.Context) error {
		return runner.Within(sessionCtx, func(txCtx context.Context) error {
			if mongo.SessionFromContext(txCtx) == nil {
				t.Fatal("SessionFromContext(txCtx) = nil, want active session")
			}
			return nil
		})
	})
	if err != nil {
		t.Fatalf("WithSession() error = %v", err)
	}
	if startCalls != 1 {
		t.Fatalf("startSessionRunner() calls = %d, want 1", startCalls)
	}
}

func TestWithDefaultConfigClonesInput(t *testing.T) {
	sessionOpt := options.Session().SetSnapshot(true)
	txOpt := options.Transaction().SetReadConcern(readconcern.Majority())
	cfg := Config{
		SessionOptions:     []options.Lister[options.SessionOptions]{sessionOpt},
		TransactionOptions: []options.Lister[options.TransactionOptions]{txOpt},
	}

	runner := New(&mongo.Client{}, WithDefaultConfig(cfg))
	cfg.SessionOptions[0] = options.Session()
	cfg.TransactionOptions[0] = options.Transaction()

	if got := reflect.ValueOf(
		runner.cfg.SessionOptions[0],
	).Pointer(); got != reflect.ValueOf(sessionOpt).Pointer() {
		t.Fatal("runner session options changed after input mutation")
	}
	if got := reflect.ValueOf(
		runner.cfg.TransactionOptions[0],
	).Pointer(); got != reflect.ValueOf(txOpt).Pointer() {
		t.Fatal("runner transaction options changed after input mutation")
	}

	cloned := cloneConfig(cfg)
	cfg.SessionOptions = append(cfg.SessionOptions, options.Session())
	cfg.TransactionOptions = append(cfg.TransactionOptions, options.Transaction())
	if len(cloned.SessionOptions) != 1 {
		t.Fatalf("cloned session options len = %d, want 1", len(cloned.SessionOptions))
	}
	if len(cloned.TransactionOptions) != 1 {
		t.Fatalf("cloned transaction options len = %d, want 1", len(cloned.TransactionOptions))
	}
}

func TestMarkerHelpers(t *testing.T) {
	if marker, ok := markerFromContext(testNilContext()); ok || marker.client != nil {
		t.Fatalf("markerFromContext(nil) = (%v, %v), want (zero, false)", marker, ok)
	}

	client := &mongo.Client{}
	ctx := withMarker(context.Background(), client)
	marker, ok := markerFromContext(ctx)
	if !ok {
		t.Fatal("markerFromContext() ok = false, want true")
	}
	if marker.client != client {
		t.Fatal("marker client mismatch")
	}
}

func TestMongoTransactionSessionRunnerEndSessionAllowsNilSession(t *testing.T) {
	var runner mongoTransactionSessionRunner
	runner.EndSession(context.Background())
}

func TestStartSessionRunnerRequiresClient(t *testing.T) {
	runner, err := startSessionRunner(nil)
	if !errors.Is(err, mongo.ErrClientDisconnected) {
		t.Fatalf("startSessionRunner(nil) error = %v, want ErrClientDisconnected", err)
	}
	if runner != nil {
		t.Fatalf("startSessionRunner(nil) runner = %v, want nil", runner)
	}
}

type fakeTransactionRunner struct {
	endCtx    context.Context
	txOptions []options.Lister[options.TransactionOptions]
}

func (r *fakeTransactionRunner) EndSession(ctx context.Context) {
	r.endCtx = ctx
}

func (r *fakeTransactionRunner) WithTransaction(
	ctx context.Context,
	fn func(context.Context) (any, error),
	opts ...options.Lister[options.TransactionOptions],
) (any, error) {
	r.txOptions = append([]options.Lister[options.TransactionOptions](nil), opts...)

	var result any
	err := mongo.WithSession(ctx, new(mongo.Session), func(sessionCtx context.Context) error {
		var callErr error
		result, callErr = fn(
			context.WithValue(sessionCtx, testContextKey{}, ctx.Value(testContextKey{})),
		)
		return callErr
	})
	return result, err
}
