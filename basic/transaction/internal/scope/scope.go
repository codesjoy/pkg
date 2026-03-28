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
)

// ErrClosed indicates the transaction scope has already completed.
var ErrClosed = errors.New("transaction scope has already completed")

type stateKey struct{}

// State stores after-commit hooks for one transaction scope.
type State struct {
	closed bool
	hooks  []func(context.Context) error
}

// New allocates a fresh transaction scope state.
func New() *State {
	return &State{}
}

// IntoContext stores state in ctx and returns the derived context.
func IntoContext(ctx context.Context, state *State) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, stateKey{}, state)
}

// FromContext retrieves transaction scope state from ctx.
func FromContext(ctx context.Context) (*State, bool) {
	if ctx == nil {
		return nil, false
	}
	state, ok := ctx.Value(stateKey{}).(*State)
	if !ok || state == nil {
		return nil, false
	}
	return state, true
}

// Add appends hook to the transaction scope.
func (s *State) Add(hook func(context.Context) error) error {
	if hook == nil {
		return nil
	}
	if s == nil {
		return ErrClosed
	}
	if s.closed {
		return ErrClosed
	}
	s.hooks = append(s.hooks, hook)
	return nil
}

// Run executes hooks once in registration order and joins returned errors.
func (s *State) Run(ctx context.Context) error {
	if s == nil || s.closed {
		return nil
	}

	s.closed = true

	var errs []error
	for _, hook := range s.hooks {
		if err := hook(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
