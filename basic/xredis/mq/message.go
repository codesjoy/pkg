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

package mq

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	// ErrNilClient indicates the redis client is nil.
	ErrNilClient = errors.New("mq redis client is nil")
	// ErrNilPublisher indicates the publisher receiver is nil.
	ErrNilPublisher = errors.New("mq publisher is nil")
	// ErrNilConsumer indicates the consumer receiver is nil.
	ErrNilConsumer = errors.New("mq consumer is nil")
	// ErrNilMessage indicates the message is nil.
	ErrNilMessage = errors.New("mq message is nil")
	// ErrNilHandlerFunc indicates the final consumer handler is nil.
	ErrNilHandlerFunc = errors.New("mq handler is nil")
	// ErrMessageStreamRequired indicates the message stream is empty.
	ErrMessageStreamRequired = errors.New("mq message stream is required")
	// ErrConsumerStreamRequired indicates the consumer stream is empty.
	ErrConsumerStreamRequired = errors.New("mq consumer stream is required")
	// ErrConsumerGroupRequired indicates the consumer group is empty.
	ErrConsumerGroupRequired = errors.New("mq consumer group is required")
	// ErrConsumerNameRequired indicates the consumer name is empty.
	ErrConsumerNameRequired = errors.New("mq consumer name is required")
	// ErrConsumerClosed indicates the consumer has already been closed.
	ErrConsumerClosed = errors.New("mq consumer is closed")
	// ErrConsumerActive indicates the consumer is already running.
	ErrConsumerActive = errors.New("mq consumer is already running")
)

// Message is one logical Redis Streams message.
type Message struct {
	Stream  string
	Payload []byte
	Headers map[string]string
}

// PublishResult is one successful publish result.
type PublishResult struct {
	BaseStream string
	Stream     string
	Shard      int
	ID         string
	Published  time.Time
}

// MessageContext contains per-message metadata passed to the business handler.
type MessageContext struct {
	Message       *Message
	BaseStream    string
	ShardStream   string
	Stream        string
	ID            string
	Group         string
	Consumer      string
	LogicalKey    string
	Shard         int
	DeliveryCount int64
	Claimed       bool
	ReceivedAt    time.Time
}

// HandlerFunc handles one consumed message.
type HandlerFunc func(context.Context, *MessageContext) error

// encodeMessage converts a Message into redis.XAddArgs for XADD, placing the
// payload under cfg.PayloadField and headers under cfg.HeaderPrefix-prefixed keys.
func encodeMessage(stream string, cfg PublisherConfig, msg *Message) *redis.XAddArgs {
	values := make([]interface{}, 0, 2+len(msg.Headers)*2)
	values = append(values, cfg.PayloadField, string(msg.Payload))

	headerKeys := make([]string, 0, len(msg.Headers))
	for key := range msg.Headers {
		headerKeys = append(headerKeys, key)
	}
	sort.Strings(headerKeys)
	for _, key := range headerKeys {
		values = append(values, cfg.HeaderPrefix+key, msg.Headers[key])
	}

	return &redis.XAddArgs{
		Stream: stream,
		Values: values,
	}
}

// decodeMessage extracts a Message from a raw Redis XMessage, separating the
// payload field from header fields based on the configured prefix.
func decodeMessage(
	stream string,
	headerPrefix string,
	payloadField string,
	raw redis.XMessage,
) *Message {
	msg := &Message{
		Stream:  stream,
		Payload: nil,
	}

	for key, value := range raw.Values {
		switch {
		case key == payloadField:
			msg.Payload = asBytes(value)
		case strings.HasPrefix(key, headerPrefix):
			if msg.Headers == nil {
				msg.Headers = make(map[string]string)
			}
			msg.Headers[strings.TrimPrefix(key, headerPrefix)] = asString(value)
		}
	}

	return msg
}

// asBytes converts a Redis stream value to a byte slice.
func asBytes(value interface{}) []byte {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return []byte(typed)
	case []byte:
		return append([]byte(nil), typed...)
	default:
		return []byte(fmt.Sprint(typed))
	}
}

// asString converts a Redis stream value to a string.
func asString(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}
