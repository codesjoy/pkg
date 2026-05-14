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

import (
	"errors"
	"testing"

	"google.golang.org/genproto/googleapis/rpc/code"
)

func TestCanonicalHelpers(t *testing.T) {
	tests := []struct {
		name string
		fn   func(string) *Error
		code code.Code
	}{
		{name: "Cancelled", fn: Cancelled, code: code.Code_CANCELLED},
		{name: "Unknown", fn: Unknown, code: code.Code_UNKNOWN},
		{name: "InvalidArgument", fn: InvalidArgument, code: code.Code_INVALID_ARGUMENT},
		{name: "DeadlineExceeded", fn: DeadlineExceeded, code: code.Code_DEADLINE_EXCEEDED},
		{name: "NotFound", fn: NotFound, code: code.Code_NOT_FOUND},
		{name: "AlreadyExists", fn: AlreadyExists, code: code.Code_ALREADY_EXISTS},
		{name: "PermissionDenied", fn: PermissionDenied, code: code.Code_PERMISSION_DENIED},
		{name: "ResourceExhausted", fn: ResourceExhausted, code: code.Code_RESOURCE_EXHAUSTED},
		{name: "FailedPrecondition", fn: FailedPrecondition, code: code.Code_FAILED_PRECONDITION},
		{name: "Aborted", fn: Aborted, code: code.Code_ABORTED},
		{name: "OutOfRange", fn: OutOfRange, code: code.Code_OUT_OF_RANGE},
		{name: "Unimplemented", fn: Unimplemented, code: code.Code_UNIMPLEMENTED},
		{name: "Internal", fn: Internal, code: code.Code_INTERNAL},
		{name: "Unavailable", fn: Unavailable, code: code.Code_UNAVAILABLE},
		{name: "DataLoss", fn: DataLoss, code: code.Code_DATA_LOSS},
		{name: "Unauthenticated", fn: Unauthenticated, code: code.Code_UNAUTHENTICATED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn("request failed")
			if err == nil {
				t.Fatal("helper returned nil")
			}
			if err.Code() != tt.code {
				t.Fatalf("code mismatch: got %v", err.Code())
			}
			if err.Message() != "request failed" {
				t.Fatalf("message mismatch: got %q", err.Message())
			}
			if err.Error() != "request failed" {
				t.Fatalf("error string mismatch: got %q", err.Error())
			}
		})
	}
}

func TestCanonicalHelpers_EmptyMessageFallsBackToCode(t *testing.T) {
	err := NotFound("")
	if err.Error() != code.Code_NOT_FOUND.String() {
		t.Fatalf("unexpected fallback error string: got %q", err.Error())
	}
}

func TestCanonicalWrapHelpers(t *testing.T) {
	tests := []struct {
		name string
		fn   func(error, string) *Error
		code code.Code
	}{
		{name: "WrapCancelled", fn: WrapCancelled, code: code.Code_CANCELLED},
		{name: "WrapUnknown", fn: WrapUnknown, code: code.Code_UNKNOWN},
		{name: "WrapInvalidArgument", fn: WrapInvalidArgument, code: code.Code_INVALID_ARGUMENT},
		{name: "WrapDeadlineExceeded", fn: WrapDeadlineExceeded, code: code.Code_DEADLINE_EXCEEDED},
		{name: "WrapNotFound", fn: WrapNotFound, code: code.Code_NOT_FOUND},
		{name: "WrapAlreadyExists", fn: WrapAlreadyExists, code: code.Code_ALREADY_EXISTS},
		{name: "WrapPermissionDenied", fn: WrapPermissionDenied, code: code.Code_PERMISSION_DENIED},
		{name: "WrapResourceExhausted", fn: WrapResourceExhausted, code: code.Code_RESOURCE_EXHAUSTED},
		{name: "WrapFailedPrecondition", fn: WrapFailedPrecondition, code: code.Code_FAILED_PRECONDITION},
		{name: "WrapAborted", fn: WrapAborted, code: code.Code_ABORTED},
		{name: "WrapOutOfRange", fn: WrapOutOfRange, code: code.Code_OUT_OF_RANGE},
		{name: "WrapUnimplemented", fn: WrapUnimplemented, code: code.Code_UNIMPLEMENTED},
		{name: "WrapInternal", fn: WrapInternal, code: code.Code_INTERNAL},
		{name: "WrapUnavailable", fn: WrapUnavailable, code: code.Code_UNAVAILABLE},
		{name: "WrapDataLoss", fn: WrapDataLoss, code: code.Code_DATA_LOSS},
		{name: "WrapUnauthenticated", fn: WrapUnauthenticated, code: code.Code_UNAUTHENTICATED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cause := errors.New("upstream failed")
			err := tt.fn(cause, "request failed")
			if err == nil {
				t.Fatal("helper returned nil")
			}
			if err.Code() != tt.code {
				t.Fatalf("code mismatch: got %v", err.Code())
			}
			if err.Message() != "request failed" {
				t.Fatalf("message mismatch: got %q", err.Message())
			}
			if err.Error() != "request failed: upstream failed" {
				t.Fatalf("error string mismatch: got %q", err.Error())
			}
			if !errors.Is(err, cause) {
				t.Fatal("errors.Is should match wrapped cause")
			}
		})
	}
}

func TestCanonicalWrapHelpers_EmptyMessageFallsBackToCause(t *testing.T) {
	cause := errors.New("upstream failed")
	err := WrapUnavailable(cause, "")
	if err.Error() != cause.Error() {
		t.Fatalf("unexpected fallback error string: got %q", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is should match wrapped cause")
	}
}
