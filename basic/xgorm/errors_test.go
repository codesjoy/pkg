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
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrInvalidSliceType(t *testing.T) {
	err := ErrInvalidSliceType
	assert.Error(t, err)
	assert.Equal(t, "invalid slice type: must be a pointer to slice or map", err.Error())
}

func TestErrInvalidModel(t *testing.T) {
	err := ErrInvalidModel
	assert.Error(t, err)
	assert.Equal(t, "invalid model", err.Error())
}

func TestErrInvalidQuery(t *testing.T) {
	err := ErrInvalidQuery
	assert.Error(t, err)
	assert.Equal(t, "invalid query parameters", err.Error())
}

func TestErrTransactionFailed(t *testing.T) {
	err := ErrTransactionFailed
	assert.Error(t, err)
	assert.Equal(t, "failed to begin transaction", err.Error())
}

func TestErrTransactionNotActive(t *testing.T) {
	err := ErrTransactionNotActive
	assert.Error(t, err)
	assert.Equal(t, "no active transaction in context", err.Error())
}

func TestPaginationError(t *testing.T) {
	t.Run("with underlying error", func(t *testing.T) {
		underlying := errors.New("database connection failed")
		err := NewPaginationError("count", underlying)

		assert.Error(t, err)
		assert.Equal(
			t,
			"pagination error in count operation: database connection failed",
			err.Error(),
		)
		assert.Same(t, underlying, err.Unwrap())
		assert.True(t, IsPaginationError(err))
	})

	t.Run("without underlying error", func(t *testing.T) {
		err := &PaginationError{Operation: "find"}

		assert.Error(t, err)
		assert.Equal(t, "pagination error in find operation", err.Error())
		assert.Nil(t, err.Unwrap())
	})

	t.Run("errors.Is", func(t *testing.T) {
		underlying := errors.New("some error")
		err := NewPaginationError("find", underlying)

		assert.True(t, errors.Is(err, underlying))
		assert.True(t, IsPaginationError(err))
	})

	t.Run("errors.As", func(t *testing.T) {
		underlying := errors.New("test error")
		err := NewPaginationError("offset", underlying)

		var pgErr *PaginationError
		require.True(t, errors.As(err, &pgErr))
		assert.Equal(t, "offset", pgErr.Operation)
		assert.Same(t, underlying, pgErr.Err)
	})
}

func TestTransactionError(t *testing.T) {
	t.Run("with underlying error", func(t *testing.T) {
		underlying := errors.New("connection lost")
		err := NewTransactionError("commit", underlying)

		assert.Error(t, err)
		assert.Equal(t, "transaction error in commit phase: connection lost", err.Error())
		assert.Same(t, underlying, err.Unwrap())
		assert.True(t, IsTransactionError(err))
	})

	t.Run("without underlying error", func(t *testing.T) {
		err := &TransactionError{Phase: "begin"}

		assert.Error(t, err)
		assert.Equal(t, "transaction error in begin phase", err.Error())
		assert.Nil(t, err.Unwrap())
	})

	t.Run("errors.Is", func(t *testing.T) {
		underlying := errors.New("transaction failed")
		err := NewTransactionError("rollback", underlying)

		assert.True(t, errors.Is(err, underlying))
		assert.True(t, IsTransactionError(err))
	})

	t.Run("errors.As", func(t *testing.T) {
		underlying := errors.New("test error")
		err := NewTransactionError("commit", underlying)

		var txErr *TransactionError
		require.True(t, errors.As(err, &txErr))
		assert.Equal(t, "commit", txErr.Phase)
		assert.Same(t, underlying, txErr.Err)
	})
}

func TestSliceElementError(t *testing.T) {
	t.Run("with underlying error", func(t *testing.T) {
		underlying := errors.New("reflection failed")
		err := NewSliceElementError("*[]User", underlying)

		assert.Error(t, err)
		assert.Equal(t, "slice element error for type *[]User: reflection failed", err.Error())
		assert.Same(t, underlying, err.Unwrap())
	})

	t.Run("without underlying error", func(t *testing.T) {
		err := &SliceElementError{Type: "int"}

		assert.Error(t, err)
		assert.Equal(t, "slice element error: invalid type int", err.Error())
		assert.Nil(t, err.Unwrap())
	})

	t.Run("errors.Is with underlying error", func(t *testing.T) {
		underlying := ErrInvalidSliceType
		err := NewSliceElementError("[]string", underlying)

		assert.True(t, errors.Is(err, ErrInvalidSliceType))
		assert.True(t, IsInvalidSliceType(err))
	})
}

func TestErrorHelperFunctions(t *testing.T) {
	t.Run("IsInvalidSliceType", func(t *testing.T) {
		t.Run("direct error", func(t *testing.T) {
			assert.True(t, IsInvalidSliceType(ErrInvalidSliceType))
		})

		t.Run("wrapped error", func(t *testing.T) {
			err := fmt.Errorf("wrapped: %w", ErrInvalidSliceType)
			assert.True(t, IsInvalidSliceType(err))
		})

		t.Run("different error", func(t *testing.T) {
			assert.False(t, IsInvalidSliceType(ErrInvalidModel))
		})
	})

	t.Run("IsInvalidModel", func(t *testing.T) {
		t.Run("direct error", func(t *testing.T) {
			assert.True(t, IsInvalidModel(ErrInvalidModel))
		})

		t.Run("wrapped error", func(t *testing.T) {
			err := fmt.Errorf("wrapped: %w", ErrInvalidModel)
			assert.True(t, IsInvalidModel(err))
		})

		t.Run("different error", func(t *testing.T) {
			assert.False(t, IsInvalidModel(ErrInvalidSliceType))
		})
	})

	t.Run("IsTransactionError", func(t *testing.T) {
		t.Run("transaction error", func(t *testing.T) {
			err := NewTransactionError("begin", errors.New("failed"))
			assert.True(t, IsTransactionError(err))
		})

		t.Run("wrapped transaction error", func(t *testing.T) {
			err := NewTransactionError("commit", errors.New("failed"))
			wrapped := fmt.Errorf("wrapped: %w", err)
			assert.True(t, IsTransactionError(wrapped))
		})

		t.Run("different error type", func(t *testing.T) {
			err := NewPaginationError("find", errors.New("failed"))
			assert.False(t, IsTransactionError(err))
		})
	})

	t.Run("IsPaginationError", func(t *testing.T) {
		t.Run("pagination error", func(t *testing.T) {
			err := NewPaginationError("count", errors.New("failed"))
			assert.True(t, IsPaginationError(err))
		})

		t.Run("wrapped pagination error", func(t *testing.T) {
			err := NewPaginationError("find", errors.New("failed"))
			wrapped := fmt.Errorf("wrapped: %w", err)
			assert.True(t, IsPaginationError(wrapped))
		})

		t.Run("different error type", func(t *testing.T) {
			err := NewTransactionError("begin", errors.New("failed"))
			assert.False(t, IsPaginationError(err))
		})
	})
}

func TestErrorWrappingChain(t *testing.T) {
	// Test that we can wrap multiple levels and still check errors
	baseErr := errors.New("base error")
	pgErr := NewPaginationError("find", baseErr)
	wrapped1 := fmt.Errorf("level 1: %w", pgErr)
	wrapped2 := fmt.Errorf("level 2: %w", wrapped1)

	// Should still be able to unwrap and check
	assert.True(t, errors.Is(wrapped2, baseErr))
	assert.True(t, IsPaginationError(wrapped2))

	var target *PaginationError
	require.True(t, errors.As(wrapped2, &target))
	assert.Equal(t, "find", target.Operation)
}

// Benchmark error checking
func BenchmarkIsInvalidSliceType(b *testing.B) {
	err := ErrInvalidSliceType
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsInvalidSliceType(err)
	}
}

func BenchmarkIsPaginationError(b *testing.B) {
	err := NewPaginationError("count", errors.New("test"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsPaginationError(err)
	}
}
