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

import "errors"

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
