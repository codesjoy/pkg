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

// Package nats provides JetStream adapters for xevent using the xnats package.
package nats

import (
	"errors"
	"strings"

	natsio "github.com/nats-io/nats.go"
)

var (
	// ErrNilPublisher indicates the JetStream publisher dependency is nil.
	ErrNilPublisher = errors.New("xevent nats publisher is nil")
	// ErrNilConsumer indicates the JetStream consumer dependency is nil.
	ErrNilConsumer = errors.New("xevent nats consumer is nil")
	// ErrNilDispatcher indicates the dispatcher dependency is nil.
	ErrNilDispatcher = errors.New("xevent nats dispatcher is nil")
)

const (
	defaultEventTypeHeader = "x-event-type"
	defaultEventIDHeader   = "x-event-id"
)

func normalizeHeaderName(name string, defaultValue string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return defaultValue
	}
	return trimmed
}

func cloneBytes(src []byte) []byte {
	if src == nil {
		return nil
	}
	return append([]byte(nil), src...)
}

func cloneHeader(header natsio.Header) natsio.Header {
	if header == nil {
		return nil
	}

	cloned := make(natsio.Header, len(header))
	for key, values := range header {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}
