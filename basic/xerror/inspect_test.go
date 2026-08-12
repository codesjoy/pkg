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
	"fmt"
	"testing"

	"google.golang.org/genproto/googleapis/rpc/code"
)

type testCodeCarrier struct {
	code code.Code
}

func (e *testCodeCarrier) Error() string   { return "code carrier" }
func (e *testCodeCarrier) Code() code.Code { return e.code }

type testReasonCarrier struct {
	reason   string
	domain   string
	metadata map[string]string
}

func (e *testReasonCarrier) Error() string               { return "reason carrier" }
func (e *testReasonCarrier) Reason() string              { return e.reason }
func (e *testReasonCarrier) Domain() string              { return e.domain }
func (e *testReasonCarrier) Metadata() map[string]string { return e.metadata }

func TestIsCode(t *testing.T) {
	base := Wrap(errors.New("boom"), code.Code_INTERNAL, "")
	wrapped := fmt.Errorf("wrapped: %w", base)

	if !IsCode(wrapped, code.Code_INTERNAL) {
		t.Fatal("expected IsCode to match internal code")
	}
	if IsCode(wrapped, code.Code_NOT_FOUND) {
		t.Fatal("expected IsCode to reject non-matching code")
	}
	if IsCode(errors.New("standard error"), code.Code_INTERNAL) {
		t.Fatal("expected IsCode to reject non-xerror")
	}
}

func TestIsReason(t *testing.T) {
	target := &testReason{
		reason: "USER_EXISTS",
		domain: "user.v1",
		code:   code.Code_ALREADY_EXISTS,
	}
	nonMatch := &testReason{
		reason: "USER_NOT_FOUND",
		domain: "user.v1",
		code:   code.Code_NOT_FOUND,
	}

	base := WrapWithReason(errors.New("boom"), target, "", nil)
	wrapped := fmt.Errorf("wrapped: %w", base)

	if !IsReason(wrapped, target) {
		t.Fatal("expected IsReason to match target reason")
	}
	if IsReason(wrapped, nonMatch) {
		t.Fatal("expected IsReason to reject non-matching reason")
	}
	if IsReason(wrapped, nil) {
		t.Fatal("expected IsReason to reject nil target")
	}
	if IsReason(errors.New("standard error"), target) {
		t.Fatal("expected IsReason to reject non-xerror")
	}

	var typedNilTarget *testReason
	var typedNilReason Reason = typedNilTarget
	if IsReason(wrapped, typedNilReason) {
		t.Fatal("expected IsReason to reject typed-nil target")
	}
}

func TestCodeOf(t *testing.T) {
	base := Wrap(errors.New("boom"), code.Code_UNAVAILABLE, "")
	wrapped := fmt.Errorf("wrapped: %w", base)

	gotCode, ok := CodeOf(wrapped)
	if !ok {
		t.Fatal("expected CodeOf to extract code")
	}
	if gotCode != code.Code_UNAVAILABLE {
		t.Fatalf("unexpected code: got %v", gotCode)
	}

	gotCode, ok = CodeOf(errors.New("standard error"))
	if ok {
		t.Fatal("expected CodeOf to fail for non-xerror")
	}
	if gotCode != code.Code_UNKNOWN {
		t.Fatalf("expected unknown for non-xerror, got %v", gotCode)
	}

	var typedNilErr *Error
	var err error = typedNilErr
	gotCode, ok = CodeOf(err)
	if ok {
		t.Fatal("expected CodeOf to fail for typed-nil *Error")
	}
	if gotCode != code.Code_UNKNOWN {
		t.Fatalf("expected unknown for typed-nil *Error, got %v", gotCode)
	}
}

func TestCodeOfCarrier(t *testing.T) {
	carrier := &testCodeCarrier{code: code.Code_NOT_FOUND}
	wrapped := fmt.Errorf("wrapped: %w", carrier)

	gotCode, ok := CodeOf(wrapped)
	if !ok || gotCode != code.Code_NOT_FOUND {
		t.Fatalf("unexpected carrier code: code=%v ok=%v", gotCode, ok)
	}
	if !IsCode(wrapped, code.Code_NOT_FOUND) {
		t.Fatal("expected IsCode to match CodeCarrier")
	}

	var typedNil *testCodeCarrier
	var err error = typedNil
	if _, ok := CodeOf(err); ok {
		t.Fatal("expected CodeOf to reject typed-nil CodeCarrier")
	}
}

func TestReasonOf(t *testing.T) {
	base := WrapWithReason(
		errors.New("boom"),
		&testReason{
			reason: "PAYLOAD_INVALID",
			domain: "api.v1",
			code:   code.Code_INVALID_ARGUMENT,
		},
		"",
		map[string]string{"field": "name"},
	)
	wrapped := fmt.Errorf("wrapped: %w", base)

	reason, domain, metadata, ok := ReasonOf(wrapped)
	if !ok {
		t.Fatal("expected ReasonOf to extract reason")
	}
	if reason != "PAYLOAD_INVALID" || domain != "api.v1" {
		t.Fatalf("unexpected reason payload: reason=%q domain=%q", reason, domain)
	}
	if metadata["field"] != "name" {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}

	metadata["field"] = "mutated"
	_, _, metadata2, ok := ReasonOf(wrapped)
	if !ok {
		t.Fatal("expected ReasonOf to still extract reason after metadata mutation")
	}
	if metadata2["field"] != "name" {
		t.Fatalf("metadata should be returned as defensive copy: %#v", metadata2)
	}

	_, _, _, ok = ReasonOf(New(code.Code_INTERNAL, "internal"))
	if ok {
		t.Fatal("expected ReasonOf to fail for xerror without reason")
	}

	_, _, _, ok = ReasonOf(errors.New("standard error"))
	if ok {
		t.Fatal("expected ReasonOf to fail for non-xerror")
	}
}

func TestReasonOfCarrier(t *testing.T) {
	carrier := &testReasonCarrier{
		reason:   "USER_NOT_FOUND",
		domain:   "user.v1",
		metadata: map[string]string{"user_id": "42"},
	}
	wrapped := fmt.Errorf("wrapped: %w", carrier)
	target := &testReason{
		reason: "USER_NOT_FOUND",
		domain: "user.v1",
		code:   code.Code_NOT_FOUND,
	}

	reason, domain, metadata, ok := ReasonOf(wrapped)
	if !ok || reason != target.Reason() || domain != target.Domain() {
		t.Fatalf(
			"unexpected carrier reason: reason=%q domain=%q ok=%v",
			reason,
			domain,
			ok,
		)
	}
	if !IsReason(wrapped, target) {
		t.Fatal("expected IsReason to match ReasonCarrier")
	}

	metadata["user_id"] = "changed"
	if carrier.metadata["user_id"] != "42" {
		t.Fatalf("carrier metadata was mutated: %#v", carrier.metadata)
	}

	if _, ok := CodeOf(wrapped); ok {
		t.Fatal("reason-only carrier must not satisfy CodeCarrier")
	}
	if _, _, _, ok := ReasonOf(&testCodeCarrier{}); ok {
		t.Fatal("code-only carrier must not satisfy ReasonCarrier")
	}

	var typedNil *testReasonCarrier
	var err error = typedNil
	if _, _, _, ok := ReasonOf(err); ok {
		t.Fatal("expected ReasonOf to reject typed-nil ReasonCarrier")
	}
}

func TestIsCode_TypedNilError(t *testing.T) {
	var typedNilErr *Error
	var err error = typedNilErr

	if IsCode(err, code.Code_UNKNOWN) {
		t.Fatal("expected IsCode to reject typed-nil *Error")
	}
}
