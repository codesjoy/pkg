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

package gormtx

import (
	"context"
	"errors"

	"github.com/codesjoy/pkg/basic/transaction"
	"github.com/codesjoy/pkg/basic/transaction/internal/scope"
	"gorm.io/gorm"
)

type slotKey struct {
	_ *gorm.DB
}

// ContextSlot stores and retrieves *gorm.DB values from context without collisions.
type ContextSlot struct {
	key *slotKey
}

var defaultSlot = NewContextSlot()

// Option customizes the GORM transaction runner.
type Option func(*Runner)

// NewContextSlot constructs an isolated context slot for *gorm.DB values.
func NewContextSlot() ContextSlot {
	return ContextSlot{key: &slotKey{}}
}

// WithContextSlot overrides the context slot used to propagate *gorm.DB.
func WithContextSlot(slot ContextSlot) Option {
	return func(r *Runner) {
		r.slot = slot
	}
}

// Runner executes REQUIRED transactions over GORM and injects the current DB into context.
type Runner struct {
	db   *gorm.DB
	slot ContextSlot
}

// New constructs a REQUIRED transaction runner over GORM.
func New(db *gorm.DB, opts ...Option) *Runner {
	runner := &Runner{
		db:   db,
		slot: defaultSlot,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(runner)
		}
	}
	return runner
}

// IntoContext stores value in ctx and returns the derived context.
func (s ContextSlot) IntoContext(ctx context.Context, db *gorm.DB) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, s.key, db)
}

// FromContext retrieves a *gorm.DB from ctx.
func (s ContextSlot) FromContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		return nil
	}
	db, _ := ctx.Value(s.key).(*gorm.DB)
	return db
}

// WithDB stores db in the default GORM context slot.
func WithDB(ctx context.Context, db *gorm.DB) context.Context {
	return defaultSlot.IntoContext(ctx, db)
}

// DB retrieves db from the default GORM context slot.
func DB(ctx context.Context) *gorm.DB {
	return defaultSlot.FromContext(ctx)
}

// DB retrieves the active transaction from ctx or falls back to the base DB.
func (r *Runner) DB(ctx context.Context) *gorm.DB {
	if tx := r.slot.FromContext(ctx); tx != nil {
		return tx
	}
	return r.db
}

// Within reuses an existing transaction in ctx or starts a new one.
func (r *Runner) Within(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if tx := r.slot.FromContext(ctx); tx != nil {
		return fn(ctx)
	}
	if r.db == nil {
		return gorm.ErrInvalidDB
	}

	scopeCtx := scope.IntoContext(ctx, scope.New())
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := r.slot.IntoContext(scopeCtx, tx)
		return fn(txCtx)
	}); err != nil {
		return err
	}

	hookCtx := r.slot.IntoContext(ctx, r.db.WithContext(ctx))
	if err := scope.Run(scopeCtx, hookCtx); err != nil {
		return errors.Join(transaction.ErrAfterCommitFailed, err)
	}

	return nil
}
