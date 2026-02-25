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
	"reflect"
	"testing"

	"google.golang.org/genproto/googleapis/rpc/code"
)

type testReason struct {
	reason string
	domain string
	code   code.Code
}

func (r *testReason) Reason() string {
	return r.reason
}

func (r *testReason) Domain() string {
	return r.domain
}

func (r *testReason) Code() code.Code {
	return r.code
}

func TestNew(t *testing.T) {
	err := New(code.Code_PERMISSION_DENIED, "permission denied")
	if err == nil {
		t.Fatal("New returned nil")
	}

	if err.Code() != code.Code_PERMISSION_DENIED {
		t.Fatalf("code mismatch: got %v", err.Code())
	}
	if err.Message() != "permission denied" {
		t.Fatalf("message mismatch: got %q", err.Message())
	}
	if err.Error() != "permission denied" {
		t.Fatalf("error string mismatch: got %q", err.Error())
	}
}

func TestNew_EmptyMessageFallsBackToCode(t *testing.T) {
	err := New(code.Code_NOT_FOUND, "")
	if err.Error() != code.Code_NOT_FOUND.String() {
		t.Fatalf("unexpected fallback error string: got %q", err.Error())
	}
}

func TestNewWithReason(t *testing.T) {
	r := &testReason{
		reason: "USER_NOT_FOUND",
		domain: "user.v1",
		code:   code.Code_NOT_FOUND,
	}
	meta := map[string]string{
		"key": "value",
	}

	err := NewWithReason(r, "not found", meta)
	if err == nil {
		t.Fatal("NewWithReason returned nil")
	}
	if err.Code() != code.Code_NOT_FOUND {
		t.Fatalf("code mismatch: got %v", err.Code())
	}
	if err.Reason() != "USER_NOT_FOUND" {
		t.Fatalf("reason mismatch: got %q", err.Reason())
	}
	if err.Domain() != "user.v1" {
		t.Fatalf("domain mismatch: got %q", err.Domain())
	}
	if !reflect.DeepEqual(err.Metadata(), map[string]string{"key": "value"}) {
		t.Fatalf("metadata mismatch: got %#v", err.Metadata())
	}

	meta["key"] = "changed"
	if got := err.Metadata()["key"]; got != "value" {
		t.Fatalf("metadata should be copied from input: got %q", got)
	}

	metadataView := err.Metadata()
	metadataView["key"] = "mutated"
	if got := err.Metadata()["key"]; got != "value" {
		t.Fatalf("metadata should be immutable from caller view: got %q", got)
	}
}

func TestNewWithReason_NilReason(t *testing.T) {
	err := NewWithReason(nil, "fallback", map[string]string{"k": "v"})
	if err.Code() != code.Code_UNKNOWN {
		t.Fatalf("expected unknown code, got %v", err.Code())
	}
	if err.Reason() != "" || err.Domain() != "" {
		t.Fatalf(
			"expected empty reason/domain, got reason=%q domain=%q",
			err.Reason(),
			err.Domain(),
		)
	}
	if got := err.Metadata()["k"]; got != "v" {
		t.Fatalf("metadata mismatch: got %q", got)
	}
}

func TestNewWithReason_TypedNilReason(t *testing.T) {
	var reason *testReason
	var typedNil Reason = reason

	err := NewWithReason(typedNil, "fallback", map[string]string{"k": "v"})
	if err.Code() != code.Code_UNKNOWN {
		t.Fatalf("expected unknown code, got %v", err.Code())
	}
	if err.Reason() != "" || err.Domain() != "" {
		t.Fatalf(
			"expected empty reason/domain, got reason=%q domain=%q",
			err.Reason(),
			err.Domain(),
		)
	}
	if got := err.Metadata()["k"]; got != "v" {
		t.Fatalf("metadata mismatch: got %q", got)
	}
}

func TestWrap(t *testing.T) {
	cause := errors.New("database unavailable")
	err := Wrap(cause, code.Code_UNAVAILABLE, "")

	if err.Error() != "database unavailable" {
		t.Fatalf("expected fallback to cause message, got %q", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is should match wrapped cause")
	}
}

func TestWrapWithReason(t *testing.T) {
	cause := errors.New("write failed")
	r := &testReason{
		reason: "WRITE_FAILED",
		domain: "storage.v1",
		code:   code.Code_INTERNAL,
	}
	err := WrapWithReason(cause, r, "save failed", map[string]string{"op": "insert"})

	if err.Error() != "save failed: write failed" {
		t.Fatalf("message mismatch: got %q", err.Error())
	}
	if err.Code() != code.Code_INTERNAL {
		t.Fatalf("code mismatch: got %v", err.Code())
	}
	if err.Reason() != "WRITE_FAILED" || err.Domain() != "storage.v1" {
		t.Fatalf("reason/domain mismatch: reason=%q domain=%q", err.Reason(), err.Domain())
	}
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is should match wrapped cause")
	}
}

func TestWrapWithReason_TypedNilReason(t *testing.T) {
	cause := errors.New("write failed")
	var reason *testReason
	var typedNil Reason = reason

	err := WrapWithReason(cause, typedNil, "save failed", nil)
	if err.Code() != code.Code_UNKNOWN {
		t.Fatalf("expected unknown code, got %v", err.Code())
	}
	if err.Reason() != "" || err.Domain() != "" {
		t.Fatalf(
			"expected empty reason/domain, got reason=%q domain=%q",
			err.Reason(),
			err.Domain(),
		)
	}
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is should match wrapped cause")
	}
}

func TestErrorsIs_DefaultSemantics(t *testing.T) {
	t.Run("same instance matches", func(t *testing.T) {
		err := WrapWithReason(
			errors.New("cause"),
			&testReason{
				reason: "WRITE_FAILED",
				domain: "storage.v1",
				code:   code.Code_INTERNAL,
			},
			"save failed",
			nil,
		)
		if !errors.Is(err, err) {
			t.Fatal("expected errors.Is to match same instance")
		}
	})

	t.Run("different instance does not match by classification fields", func(t *testing.T) {
		left := NewWithReason(
			&testReason{reason: "NOT_FOUND", domain: "user.v1", code: code.Code_NOT_FOUND},
			"a",
			nil,
		)
		right := NewWithReason(
			&testReason{reason: "NOT_FOUND", domain: "user.v1", code: code.Code_NOT_FOUND},
			"b",
			nil,
		)
		if errors.Is(left, right) {
			t.Fatal("expected errors.Is to reject different instances")
		}
	})

	t.Run("wrapped cause still matches", func(t *testing.T) {
		cause := errors.New("db failed")
		err := Wrap(cause, code.Code_INTERNAL, "write failed")
		if !errors.Is(err, cause) {
			t.Fatal("expected errors.Is to match wrapped cause")
		}
	})
}

func TestNilReceiver(t *testing.T) {
	var err *Error

	if err.Error() != "" {
		t.Fatalf("nil receiver Error() should return empty string, got %q", err.Error())
	}
	if err.Unwrap() != nil {
		t.Fatal("nil receiver Unwrap() should return nil")
	}
	if err.Code() != code.Code_UNKNOWN {
		t.Fatalf("nil receiver Code() should return unknown, got %v", err.Code())
	}
	if err.Reason() != "" {
		t.Fatalf("nil receiver Reason() should return empty, got %q", err.Reason())
	}
	if err.Domain() != "" {
		t.Fatalf("nil receiver Domain() should return empty, got %q", err.Domain())
	}
	if err.Message() != "" {
		t.Fatalf("nil receiver Message() should return empty, got %q", err.Message())
	}
	if err.Metadata() != nil {
		t.Fatalf("nil receiver Metadata() should return nil, got %#v", err.Metadata())
	}
}
