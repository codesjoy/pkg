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

	"github.com/codesjoy/pkg/basic/transaction/internal/scope"
)

// Runner executes callbacks with REQUIRED transaction propagation semantics.
type Runner interface {
	Within(context.Context, func(context.Context) error) error
}

// Hook is executed after the outermost transaction has committed successfully.
type Hook func(context.Context) error

var (
	// ErrAfterCommitOutsideTransaction indicates AfterCommit was called without an active transaction scope.
	ErrAfterCommitOutsideTransaction = errors.New(
		"transaction after-commit hook requires an active transaction",
	)
	// ErrAfterCommitClosed indicates hooks can no longer be registered for the current scope.
	ErrAfterCommitClosed = errors.New(
		"transaction after-commit hooks can no longer be registered",
	)
	// ErrNilHook indicates the provided after-commit hook was nil.
	ErrNilHook = errors.New("transaction after-commit hook is nil")
	// ErrAfterCommitFailed indicates the transaction committed but one or more after-commit hooks failed.
	ErrAfterCommitFailed = errors.New(
		"transaction committed but after-commit hooks failed",
	)
)

// AfterCommit registers hook on the current transaction scope.
func AfterCommit(ctx context.Context, hook Hook) error {
	if hook == nil {
		return ErrNilHook
	}

	state, ok := scope.FromContext(ctx)
	if !ok || state == nil {
		return ErrAfterCommitOutsideTransaction
	}

	if err := state.Add(func(runCtx context.Context) error {
		return hook(runCtx)
	}); err != nil {
		if errors.Is(err, scope.ErrClosed) {
			return ErrAfterCommitClosed
		}
		return err
	}

	return nil
}
