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

// Package kafka provides Kafka adapters for xevent using the xkafka package.
package kafka

import (
	"errors"
	"strings"

	"github.com/IBM/sarama"
)

var (
	// ErrNilProducer indicates the Kafka producer dependency is nil.
	ErrNilProducer = errors.New("xevent kafka producer is nil")
	// ErrNilConsumer indicates the Kafka consumer dependency is nil.
	ErrNilConsumer = errors.New("xevent kafka consumer is nil")
	// ErrNilDispatcher indicates the Kafka dispatcher dependency is nil.
	ErrNilDispatcher = errors.New("xevent kafka dispatcher is nil")
	// ErrTopicRequired indicates the Kafka topic is empty.
	ErrTopicRequired = errors.New("xevent kafka topic is required")
	// ErrEventTypeHeaderRequired indicates the Kafka event type header is missing.
	ErrEventTypeHeaderRequired = errors.New("xevent kafka event type header is required")
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

func consumerHeaderValue(headers []*sarama.RecordHeader, key string) string {
	for _, header := range headers {
		if header == nil {
			continue
		}
		if string(header.Key) == key {
			return string(header.Value)
		}
	}
	return ""
}
