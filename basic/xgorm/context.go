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
	val := ctx.Value(txKey)
	// Check if the key exists in the context, even if the value is nil
	return val != nil
}
