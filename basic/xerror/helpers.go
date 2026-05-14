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

package xerror

import "google.golang.org/genproto/googleapis/rpc/code"

// Cancelled creates an Error with CANCELLED code.
func Cancelled(message string) *Error {
	return New(code.Code_CANCELLED, message)
}

// Unknown creates an Error with UNKNOWN code.
func Unknown(message string) *Error {
	return New(code.Code_UNKNOWN, message)
}

// InvalidArgument creates an Error with INVALID_ARGUMENT code.
func InvalidArgument(message string) *Error {
	return New(code.Code_INVALID_ARGUMENT, message)
}

// DeadlineExceeded creates an Error with DEADLINE_EXCEEDED code.
func DeadlineExceeded(message string) *Error {
	return New(code.Code_DEADLINE_EXCEEDED, message)
}

// NotFound creates an Error with NOT_FOUND code.
func NotFound(message string) *Error {
	return New(code.Code_NOT_FOUND, message)
}

// AlreadyExists creates an Error with ALREADY_EXISTS code.
func AlreadyExists(message string) *Error {
	return New(code.Code_ALREADY_EXISTS, message)
}

// PermissionDenied creates an Error with PERMISSION_DENIED code.
func PermissionDenied(message string) *Error {
	return New(code.Code_PERMISSION_DENIED, message)
}

// ResourceExhausted creates an Error with RESOURCE_EXHAUSTED code.
func ResourceExhausted(message string) *Error {
	return New(code.Code_RESOURCE_EXHAUSTED, message)
}

// FailedPrecondition creates an Error with FAILED_PRECONDITION code.
func FailedPrecondition(message string) *Error {
	return New(code.Code_FAILED_PRECONDITION, message)
}

// Aborted creates an Error with ABORTED code.
func Aborted(message string) *Error {
	return New(code.Code_ABORTED, message)
}

// OutOfRange creates an Error with OUT_OF_RANGE code.
func OutOfRange(message string) *Error {
	return New(code.Code_OUT_OF_RANGE, message)
}

// Unimplemented creates an Error with UNIMPLEMENTED code.
func Unimplemented(message string) *Error {
	return New(code.Code_UNIMPLEMENTED, message)
}

// Internal creates an Error with INTERNAL code.
func Internal(message string) *Error {
	return New(code.Code_INTERNAL, message)
}

// Unavailable creates an Error with UNAVAILABLE code.
func Unavailable(message string) *Error {
	return New(code.Code_UNAVAILABLE, message)
}

// DataLoss creates an Error with DATA_LOSS code.
func DataLoss(message string) *Error {
	return New(code.Code_DATA_LOSS, message)
}

// Unauthenticated creates an Error with UNAUTHENTICATED code.
func Unauthenticated(message string) *Error {
	return New(code.Code_UNAUTHENTICATED, message)
}

// WrapCancelled wraps err with CANCELLED code and message.
func WrapCancelled(err error, message string) *Error {
	return Wrap(err, code.Code_CANCELLED, message)
}

// WrapUnknown wraps err with UNKNOWN code and message.
func WrapUnknown(err error, message string) *Error {
	return Wrap(err, code.Code_UNKNOWN, message)
}

// WrapInvalidArgument wraps err with INVALID_ARGUMENT code and message.
func WrapInvalidArgument(err error, message string) *Error {
	return Wrap(err, code.Code_INVALID_ARGUMENT, message)
}

// WrapDeadlineExceeded wraps err with DEADLINE_EXCEEDED code and message.
func WrapDeadlineExceeded(err error, message string) *Error {
	return Wrap(err, code.Code_DEADLINE_EXCEEDED, message)
}

// WrapNotFound wraps err with NOT_FOUND code and message.
func WrapNotFound(err error, message string) *Error {
	return Wrap(err, code.Code_NOT_FOUND, message)
}

// WrapAlreadyExists wraps err with ALREADY_EXISTS code and message.
func WrapAlreadyExists(err error, message string) *Error {
	return Wrap(err, code.Code_ALREADY_EXISTS, message)
}

// WrapPermissionDenied wraps err with PERMISSION_DENIED code and message.
func WrapPermissionDenied(err error, message string) *Error {
	return Wrap(err, code.Code_PERMISSION_DENIED, message)
}

// WrapResourceExhausted wraps err with RESOURCE_EXHAUSTED code and message.
func WrapResourceExhausted(err error, message string) *Error {
	return Wrap(err, code.Code_RESOURCE_EXHAUSTED, message)
}

// WrapFailedPrecondition wraps err with FAILED_PRECONDITION code and message.
func WrapFailedPrecondition(err error, message string) *Error {
	return Wrap(err, code.Code_FAILED_PRECONDITION, message)
}

// WrapAborted wraps err with ABORTED code and message.
func WrapAborted(err error, message string) *Error {
	return Wrap(err, code.Code_ABORTED, message)
}

// WrapOutOfRange wraps err with OUT_OF_RANGE code and message.
func WrapOutOfRange(err error, message string) *Error {
	return Wrap(err, code.Code_OUT_OF_RANGE, message)
}

// WrapUnimplemented wraps err with UNIMPLEMENTED code and message.
func WrapUnimplemented(err error, message string) *Error {
	return Wrap(err, code.Code_UNIMPLEMENTED, message)
}

// WrapInternal wraps err with INTERNAL code and message.
func WrapInternal(err error, message string) *Error {
	return Wrap(err, code.Code_INTERNAL, message)
}

// WrapUnavailable wraps err with UNAVAILABLE code and message.
func WrapUnavailable(err error, message string) *Error {
	return Wrap(err, code.Code_UNAVAILABLE, message)
}

// WrapDataLoss wraps err with DATA_LOSS code and message.
func WrapDataLoss(err error, message string) *Error {
	return Wrap(err, code.Code_DATA_LOSS, message)
}

// WrapUnauthenticated wraps err with UNAUTHENTICATED code and message.
func WrapUnauthenticated(err error, message string) *Error {
	return Wrap(err, code.Code_UNAUTHENTICATED, message)
}
