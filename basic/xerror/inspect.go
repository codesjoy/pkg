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

// IsCode reports whether err or a wrapped error carries target code.
func IsCode(err error, target code.Code) bool {
	carrier, ok := asCodeCarrier(err)
	if !ok {
		return false
	}
	return carrier.Code() == target
}

// IsReason reports whether err or a wrapped error carries target reason.
func IsReason(err error, target Reason) bool {
	if isNilReason(target) {
		return false
	}

	carrier, ok := asReasonCarrier(err)
	if !ok {
		return false
	}

	return carrier.Reason() == target.Reason() && carrier.Domain() == target.Domain()
}

// CodeOf returns the canonical code carried by err or a wrapped error.
func CodeOf(err error) (code.Code, bool) {
	carrier, ok := asCodeCarrier(err)
	if !ok {
		return code.Code_UNKNOWN, false
	}
	return carrier.Code(), true
}

// ReasonOf returns reason metadata carried by err or a wrapped error.
func ReasonOf(err error) (reason string, domain string, metadata map[string]string, ok bool) {
	carrier, ok := asReasonCarrier(err)
	if !ok {
		return "", "", nil, false
	}
	if carrier.Reason() == "" {
		return "", "", nil, false
	}
	return carrier.Reason(), carrier.Domain(), cloneMetadata(carrier.Metadata()), true
}

func asCodeCarrier(err error) (CodeCarrier, bool) {
	if err == nil {
		return nil, false
	}
	var carrier CodeCarrier
	if !errors.As(err, &carrier) || isNilCarrier(carrier) {
		return nil, false
	}
	return carrier, true
}

func asReasonCarrier(err error) (ReasonCarrier, bool) {
	if err == nil {
		return nil, false
	}
	var carrier ReasonCarrier
	if !errors.As(err, &carrier) || isNilCarrier(carrier) {
		return nil, false
	}
	return carrier, true
}
