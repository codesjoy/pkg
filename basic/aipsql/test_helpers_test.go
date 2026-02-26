// Copyright 2026 The codesjoy Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package aipsql

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/stretchr/testify/assert"
)

// ErrLike asserts that an error contains all specified substrings,
// or that the error is nil when expected is nil.
//
// If multiple errors/strings are provided in expected, they must all
// be contained in the stringified error.
//
// If the expected is the singular nil, this expects the error to be nil.
func ErrLike(t assert.TestingT, actual error, expected []any, msgAndArgs ...any) bool {
	if len(expected) == 0 {
		assert.Fail(t, "ErrLike requires 1 or more expected values, got 0", msgAndArgs...)
		return false
	}

	// If we have multiple expected arguments, they must all be non-nil.
	if len(expected) > 1 {
		for _, e := range expected {
			if e == nil {
				assert.Fail(
					t,
					"ErrLike only accepts `nil` on the right hand side as the sole argument",
					msgAndArgs...,
				)
				return false
			}
		}
	}

	if expected[0] == nil { // this can only happen if len(expected) == 1
		return assert.Nil(t, actual, msgAndArgs...)
	}

	if assert.NotNil(t, actual, msgAndArgs...) {
		errStr := actual.Error()
		for _, exp := range expected {
			switch v := exp.(type) {
			case string:
				if !assert.Contains(t, errStr, v, msgAndArgs...) {
					return false
				}
			case error:
				if !assert.Contains(t, errStr, v.Error(), msgAndArgs...) {
					return false
				}
			default:
				assert.Fail(
					t,
					fmt.Sprintf("unexpected argument type %T, expected string or error", exp),
					msgAndArgs...,
				)
				return false
			}
		}
		return true
	}
	return false
}

// UnwrapTo asserts that an error, when unwrapped, equals another error.
func UnwrapTo(t assert.TestingT, actual, expected error, msgAndArgs ...any) bool {
	if assert.NotNil(t, actual, msgAndArgs...) {
		return assert.Equal(t, errors.Unwrap(actual), expected, msgAndArgs...)
	}
	return false
}

// quoteFilterLiteral safely quotes arbitrary input as an AIP filter string literal.
func quoteFilterLiteral(value string) string {
	return strconv.Quote(value)
}
