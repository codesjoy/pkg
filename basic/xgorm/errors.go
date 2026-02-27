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
)

// Base error types for the xgorm package.
// These errors can be checked using errors.Is() and errors.As().

var (
	// ErrInvalidSliceType is returned when the provided slice has an invalid type.
	// This typically occurs when a non-slice value is passed to functions expecting a slice.
	ErrInvalidSliceType = errors.New("invalid slice type: must be a pointer to slice or map")

	// ErrInvalidModel is returned when the model provided to GORM operations is invalid.
	// This includes nil models, non-pointer models, or models with unsupported types.
	ErrInvalidModel = errors.New("invalid model")

	// ErrInvalidQuery is returned when the query parameters are invalid.
	ErrInvalidQuery = errors.New("invalid query parameters")

	// ErrTransactionFailed is returned when a database transaction cannot be started.
	ErrTransactionFailed = errors.New("failed to begin transaction")

	// ErrTransactionNotActive is returned when trying to commit/rollback a non-existent transaction.
	ErrTransactionNotActive = errors.New("no active transaction in context")

	// ErrTransactionAlreadyCommitted is returned when trying to commit an already committed transaction.
	ErrTransactionAlreadyCommitted = errors.New("transaction already committed")

	// ErrTransactionAlreadyRolledBack is returned when trying to rollback an already rolled back transaction.
	ErrTransactionAlreadyRolledBack = errors.New("transaction already rolled back")

	// ErrNilMeter is returned when attempting to register metrics with a nil meter.
	ErrNilMeter = errors.New("meter cannot be nil")

	// ErrNilDatabase is returned when attempting to register metrics with a nil database.
	ErrNilDatabase = errors.New("database cannot be nil")

	// ErrShardingTablesRequired is returned when sharding is enabled without any target tables.
	ErrShardingTablesRequired = errors.New("sharding tables are required")

	// ErrShardingPrepareStmtUnsupported is returned when PrepareStmt is enabled with sharding.
	ErrShardingPrepareStmtUnsupported = errors.New(
		"prepare statement mode is not supported with sharding",
	)

	// ErrDBResolverNotConfigured is returned when dbresolver pool options are set without resolver rules.
	ErrDBResolverNotConfigured = errors.New(
		"dbresolver connection pool options require at least one dbresolver rule",
	)
)

// PaginationError represents errors related to pagination operations.
// It wraps the underlying error and provides context about the pagination operation.
type PaginationError struct {
	// Operation is the pagination operation that failed (e.g., "count", "find", "offset")
	Operation string
	// Err is the underlying error from the database or other source
	Err error
}

// Error implements the error interface.
func (e *PaginationError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("pagination error in %s operation", e.Operation)
	}
	return fmt.Sprintf("pagination error in %s operation: %s", e.Operation, e.Err.Error())
}

// Unwrap returns the underlying error for use with errors.Is() and errors.As().
func (e *PaginationError) Unwrap() error {
	return e.Err
}

// NewPaginationError creates a new PaginationError with the given operation and underlying error.
func NewPaginationError(operation string, err error) *PaginationError {
	return &PaginationError{
		Operation: operation,
		Err:       err,
	}
}

// TransactionError represents errors related to transaction operations.
// It provides context about which transaction phase failed (begin, commit, rollback).
type TransactionError struct {
	// Phase is the transaction phase that failed (e.g., "begin", "commit", "rollback")
	Phase string
	// Err is the underlying error
	Err error
}

// Error implements the error interface.
func (e *TransactionError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("transaction error in %s phase", e.Phase)
	}
	return fmt.Sprintf("transaction error in %s phase: %s", e.Phase, e.Err.Error())
}

// Unwrap returns the underlying error for use with errors.Is() and errors.As().
func (e *TransactionError) Unwrap() error {
	return e.Err
}

// NewTransactionError creates a new TransactionError with the given phase and underlying error.
func NewTransactionError(phase string, err error) *TransactionError {
	return &TransactionError{
		Phase: phase,
		Err:   err,
	}
}

// SliceElementError represents errors related to extracting elements from slices.
// This occurs when converting slice pointers to model instances.
type SliceElementError struct {
	// Type is the actual type received
	Type string
	// Err is the underlying error
	Err error
}

// Error implements the error interface.
func (e *SliceElementError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("slice element error: invalid type %s", e.Type)
	}
	return fmt.Sprintf("slice element error for type %s: %s", e.Type, e.Err.Error())
}

// Unwrap returns the underlying error for use with errors.Is() and errors.As().
func (e *SliceElementError) Unwrap() error {
	return e.Err
}

// NewSliceElementError creates a new SliceElementError with the given type and underlying error.
func NewSliceElementError(typ string, err error) *SliceElementError {
	return &SliceElementError{
		Type: typ,
		Err:  err,
	}
}

// IsInvalidSliceType checks if an error is or wraps ErrInvalidSliceType.
func IsInvalidSliceType(err error) bool {
	return errors.Is(err, ErrInvalidSliceType)
}

// IsInvalidModel checks if an error is or wraps ErrInvalidModel.
func IsInvalidModel(err error) bool {
	return errors.Is(err, ErrInvalidModel)
}

// IsTransactionError checks if an error is a TransactionError.
func IsTransactionError(err error) bool {
	var txErr *TransactionError
	return errors.As(err, &txErr)
}

// IsPaginationError checks if an error is a PaginationError.
func IsPaginationError(err error) bool {
	var pgErr *PaginationError
	return errors.As(err, &pgErr)
}
