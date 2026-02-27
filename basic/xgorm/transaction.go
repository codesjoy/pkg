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

func (t *transaction) Begin(ctx context.Context) context.Context {
	// Check if transaction already exists in context
	if tx := TransactionFromContext(ctx); tx != nil {
		return ctx
	}

	// Begin new transaction
	tx := t.db.WithContext(ctx).Begin()
	return WithTransaction(ctx, tx)
}

func (t *transaction) Commit(ctx context.Context) error {
	return t.runWithTransaction(ctx, "commit", func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Commit().Error
	})
}

func (t *transaction) Rollback(ctx context.Context) error {
	return t.runWithTransaction(ctx, "rollback", func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Rollback().Error
	})
}

func (t *transaction) GetTx(ctx context.Context) *gorm.DB {
	if tx := TransactionFromContext(ctx); tx != nil {
		return tx
	}
	return t.db
}

func (t *transaction) Transaction(ctx context.Context, fx func(tx *gorm.DB) error) error {
	tx := t.GetTx(ctx)
	if err := tx.Transaction(fx); err != nil {
		return NewTransactionError("transaction", err)
	}
	return nil
}

func (t *transaction) runWithTransaction(
	ctx context.Context,
	action string,
	fn func(*gorm.DB) error,
) error {
	tx := TransactionFromContext(ctx)
	if tx == nil {
		return ErrTransactionNotActive
	}

	if err := fn(tx); err != nil {
		return NewTransactionError(action, err)
	}
	return nil
}
