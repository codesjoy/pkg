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
	trans := &transaction{
		db: db,
	}
	return trans
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
	tx := TransactionFromContext(ctx)
	if tx == nil {
		return ErrTransactionNotActive
	}

	err := tx.WithContext(ctx).Commit().Error
	if err != nil {
		return NewTransactionError("commit", err)
	}

	// Clear transaction from context
	return nil
}

func (t *transaction) Rollback(ctx context.Context) error {
	tx := TransactionFromContext(ctx)
	if tx == nil {
		return ErrTransactionNotActive
	}

	err := tx.WithContext(ctx).Rollback().Error
	if err != nil {
		return NewTransactionError("rollback", err)
	}

	// Clear transaction from context
	return nil
}

func (t *transaction) GetTx(ctx context.Context) *gorm.DB {
	tx := TransactionFromContext(ctx)
	if tx != nil {
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
