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

package xgorm

import (
	"context"

	"gorm.io/gorm"
)

// contextKeyType is a type-safe context key type.
// Using a private struct type prevents context key collisions.
type contextKeyType struct{}

// Transaction context key for storing GORM transaction in context.
var txKey contextKeyType

// Transaction defines transaction lifecycle helpers backed by GORM.
type Transaction interface {
	Begin(ctx context.Context) context.Context
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
	GetTx(ctx context.Context) *gorm.DB
	Transaction(ctx context.Context, fx func(tx *gorm.DB) error) error
}

type transaction struct {
	db *gorm.DB
}

// NewTransaction creates a transaction helper bound to a base GORM DB handle.
func NewTransaction(db *gorm.DB) Transaction {
	return &transaction{db: db}
}

// WithTransaction adds a transaction to the context.
// This is used internally by Transaction.Begin() and external helpers.
func WithTransaction(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txKey, tx)
}

// TransactionFromContext retrieves the transaction from the context.
// Returns nil if no transaction is found in the context.
func TransactionFromContext(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(txKey).(*gorm.DB); ok {
		return tx
	}
	return nil
}

// HasTransaction checks if a transaction exists in the context.
func HasTransaction(ctx context.Context) bool {
	value := ctx.Value(txKey)
	return value != nil
}

func (t *transaction) Begin(ctx context.Context) context.Context {
	if TransactionFromContext(ctx) != nil {
		return ctx
	}

	return WithTransaction(ctx, t.db.WithContext(ctx).Begin())
}

func (t *transaction) Commit(ctx context.Context) error {
	return t.runWithActiveTransaction(ctx, "commit", func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Commit().Error
	})
}

func (t *transaction) Rollback(ctx context.Context) error {
	return t.runWithActiveTransaction(ctx, "rollback", func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Rollback().Error
	})
}

func (t *transaction) GetTx(ctx context.Context) *gorm.DB {
	return t.txFromContextOrBase(ctx)
}

func (t *transaction) Transaction(ctx context.Context, fx func(tx *gorm.DB) error) error {
	if err := t.txFromContextOrBase(ctx).Transaction(fx); err != nil {
		return NewTransactionError("transaction", err)
	}
	return nil
}

func (t *transaction) txFromContextOrBase(ctx context.Context) *gorm.DB {
	if tx := TransactionFromContext(ctx); tx != nil {
		return tx
	}
	return t.db
}

func (t *transaction) activeTransaction(ctx context.Context) (*gorm.DB, error) {
	tx := TransactionFromContext(ctx)
	if tx == nil {
		return nil, ErrTransactionNotActive
	}
	return tx, nil
}

func (t *transaction) runWithActiveTransaction(
	ctx context.Context,
	action string,
	fn func(*gorm.DB) error,
) error {
	tx, err := t.activeTransaction(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return NewTransactionError(action, err)
	}
	return nil
}
