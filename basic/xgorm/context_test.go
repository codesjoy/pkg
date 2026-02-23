package xgorm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestWithTransaction(t *testing.T) {
	ctx := context.Background()

	// Create a mock DB (we don't need a real connection for context operations)
	var db *gorm.DB

	// Add transaction to context
	ctx = WithTransaction(ctx, db)

	// Verify transaction is in context
	retrieved := TransactionFromContext(ctx)
	assert.Same(t, db, retrieved, "Should return the same transaction instance")
}

func TestTransactionFromContext_NotFound(t *testing.T) {
	ctx := context.Background()

	// Context without transaction should return nil
	tx := TransactionFromContext(ctx)
	assert.Nil(t, tx, "Should return nil when no transaction in context")
}

func TestHasTransaction(t *testing.T) {
	tests := []struct {
		name     string
		setupCtx func() context.Context
		want     bool
	}{
		{
			name:     "no transaction",
			setupCtx: context.Background,
			want:     false,
		},
		{
			name: "with transaction",
			setupCtx: func() context.Context {
				var db *gorm.DB
				return WithTransaction(context.Background(), db)
			},
			want: true,
		},
		{
			name: "with nil transaction",
			setupCtx: func() context.Context {
				return WithTransaction(context.Background(), nil)
			},
			want: true, // Even nil transactions count as "having a transaction"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx()
			got := HasTransaction(ctx)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTransactionContextKeyIsolation(t *testing.T) {
	// Verify that our context key doesn't collide with string keys
	type anotherTxKey struct{}
	ctx1 := context.WithValue(context.Background(), anotherTxKey{}, &gorm.DB{})
	ctx2 := WithTransaction(context.Background(), &gorm.DB{})

	// String key should not interfere with type-safe key
	tx1 := TransactionFromContext(ctx1)
	assert.Nil(t, tx1, "Should not find transaction with string key")

	tx2 := TransactionFromContext(ctx2)
	assert.NotNil(t, tx2, "Should find transaction with type-safe key")
}

func TestWithTransaction_ChainContexts(t *testing.T) {
	// Test that transactions can be chained through multiple context levels
	var db1, db2 *gorm.DB

	ctx1 := WithTransaction(context.Background(), db1)
	ctx2 := WithTransaction(ctx1, db2)

	// The most recent transaction should be returned
	tx := TransactionFromContext(ctx2)
	assert.Same(t, db2, tx, "Should return the most recent transaction")
}

func TestTransactionFromContext_WithValue(t *testing.T) {
	// Test with nested context values
	type otherKey struct{}
	ctx := context.WithValue(context.Background(), otherKey{}, "other_value")

	var db *gorm.DB
	ctx = WithTransaction(ctx, db)

	// Should still be able to retrieve transaction
	tx := TransactionFromContext(ctx)
	assert.Same(t, db, tx, "Should retrieve transaction from context with other values")

	// Other values should still be accessible
	val := ctx.Value(otherKey{})
	assert.Equal(t, "other_value", val, "Other context values should be preserved")
}

func TestWithTransaction_Parallel(t *testing.T) {
	// Test that context operations are safe for concurrent use
	var db *gorm.DB

	ctx1 := WithTransaction(context.Background(), db)
	ctx2 := WithTransaction(context.Background(), db)

	// Different contexts should not interfere
	tx1 := TransactionFromContext(ctx1)
	tx2 := TransactionFromContext(ctx2)

	assert.Same(t, db, tx1)
	assert.Same(t, db, tx2)
}

// Benchmark context operations
func BenchmarkWithTransaction(b *testing.B) {
	var db *gorm.DB
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = WithTransaction(ctx, db)
	}
}

func BenchmarkTransactionFromContext(b *testing.B) {
	var db *gorm.DB
	ctx := WithTransaction(context.Background(), db)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TransactionFromContext(ctx)
	}
}
