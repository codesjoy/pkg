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

	"google.golang.org/genproto/googleapis/rpc/code"
)

// IsCode reports whether err (or wrapped err) is Error with target code.
func IsCode(err error, target code.Code) bool {
	e, ok := asError(err)
	if !ok {
		return false
	}
	return e.Code() == target
}

// IsReason reports whether err (or wrapped err) is Error with target reason.
func IsReason(err error, target Reason) bool {
	if isNilReason(target) {
		return false
	}

	e, ok := asError(err)
	if !ok {
		return false
	}

	return e.Reason() == target.Reason() && e.Domain() == target.Domain()
}

// CodeOf returns code from err when it carries Error.
func CodeOf(err error) (code.Code, bool) {
	e, ok := asError(err)
	if !ok {
		return code.Code_UNKNOWN, false
	}
	return e.Code(), true
}

// ReasonOf returns reason payload from err when it carries Error and reason.
func ReasonOf(err error) (reason string, domain string, metadata map[string]string, ok bool) {
	e, ok := asError(err)
	if !ok {
		return "", "", nil, false
	}
	if e.Reason() == "" {
		return "", "", nil, false
	}
	return e.Reason(), e.Domain(), e.Metadata(), true
}

func asError(err error) (*Error, bool) {
	if err == nil {
		return nil, false
	}
	var e *Error
	if !errors.As(err, &e) {
		return nil, false
	}
	if e == nil {
		return nil, false
	}
	return e, true
}
