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

// Package xerror provides framework-agnostic domain error modeling with code
// and reason metadata.
package xerror

import (
	"reflect"

	"google.golang.org/genproto/googleapis/rpc/code"
)

// Reason describes a domain-level reason value that maps to a canonical code.
type Reason interface {
	Reason() string
	Domain() string
	Code() code.Code
}

// CodeCarrier is implemented by errors that expose a canonical code.
type CodeCarrier interface {
	Code() code.Code
}

// ReasonCarrier is implemented by errors that expose domain reason metadata.
type ReasonCarrier interface {
	Reason() string
	Domain() string
	Metadata() map[string]string
}

// Error is a domain error carrying canonical code and optional reason metadata.
type Error struct {
	code     code.Code
	message  string
	reason   string
	domain   string
	metadata map[string]string
	cause    error
}

var (
	_ CodeCarrier   = (*Error)(nil)
	_ ReasonCarrier = (*Error)(nil)
)

// New creates a new Error with canonical code and message.
func New(c code.Code, message string) *Error {
	return &Error{
		code:    c,
		message: message,
	}
}

// NewWithReason creates a new Error from Reason and message.
func NewWithReason(r Reason, message string, metadata map[string]string) *Error {
	if isNilReason(r) {
		return &Error{
			code:     code.Code_UNKNOWN,
			message:  message,
			metadata: cloneMetadata(metadata),
		}
	}

	return &Error{
		code:     r.Code(),
		reason:   r.Reason(),
		domain:   r.Domain(),
		message:  message,
		metadata: cloneMetadata(metadata),
	}
}

// Wrap wraps an existing error with canonical code and optional message.
func Wrap(err error, c code.Code, message string) *Error {
	return &Error{
		code:    c,
		message: message,
		cause:   err,
	}
}

// WrapWithReason wraps an existing error with Reason and optional message.
func WrapWithReason(err error, r Reason, message string, metadata map[string]string) *Error {
	e := NewWithReason(r, message, metadata)
	e.cause = err
	return e
}

// Error returns the display message for the error.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.message == "" {
		return e.fallbackMessage()
	}
	return e.messageWithCause(e.message)
}

// Unwrap returns the wrapped error.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Code returns canonical code.
func (e *Error) Code() code.Code {
	if e == nil {
		return code.Code_UNKNOWN
	}
	return e.code
}

// Reason returns domain reason text.
func (e *Error) Reason() string {
	if e == nil {
		return ""
	}
	return e.reason
}

// Domain returns reason domain.
func (e *Error) Domain() string {
	if e == nil {
		return ""
	}
	return e.domain
}

// Message returns the explicit message without fallback.
func (e *Error) Message() string {
	if e == nil {
		return ""
	}
	return e.message
}

// Metadata returns a defensive copy of reason metadata.
func (e *Error) Metadata() map[string]string {
	if e == nil {
		return nil
	}
	return cloneMetadata(e.metadata)
}

func cloneMetadata(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func isNilReason(r Reason) bool {
	return isNilCarrier(r)
}

func isNilCarrier(carrier any) bool {
	if carrier == nil {
		return true
	}

	v := reflect.ValueOf(carrier)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func (e *Error) fallbackMessage() string {
	if e.cause != nil {
		return e.cause.Error()
	}
	return e.code.String()
}

func (e *Error) messageWithCause(message string) string {
	if e.cause == nil {
		return message
	}
	return message + ": " + e.cause.Error()
}
