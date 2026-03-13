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

package tracecarrier

import (
	"strings"

	"github.com/nats-io/nats.go"
)

// HeaderCarrier adapts nats.Header to OpenTelemetry text-map propagation.
type HeaderCarrier struct {
	Header *nats.Header
}

// Get returns a header value by key.
func (c HeaderCarrier) Get(key string) string {
	if c.Header == nil {
		return ""
	}
	for headerKey, values := range *c.Header {
		if !strings.EqualFold(headerKey, key) || len(values) == 0 {
			continue
		}
		return values[0]
	}
	return ""
}

// Set sets a header value by key.
func (c HeaderCarrier) Set(key, value string) {
	if c.Header == nil {
		return
	}
	for headerKey := range *c.Header {
		if strings.EqualFold(headerKey, key) {
			(*c.Header)[headerKey] = []string{value}
			return
		}
	}
	(*c.Header)[key] = []string{value}
}

// Keys returns all header keys.
func (c HeaderCarrier) Keys() []string {
	if c.Header == nil {
		return nil
	}
	keys := make([]string, 0, len(*c.Header))
	for key := range *c.Header {
		keys = append(keys, key)
	}
	return keys
}
