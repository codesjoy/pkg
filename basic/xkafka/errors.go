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

package xkafka

import (
	"errors"

	pretry "github.com/codesjoy/pkg/basic/xkafka/middleware/produce/retry"
)

var (
	// ErrProducerClosed indicates producer is already closed.
	ErrProducerClosed = errors.New("producer is closed")
	// ErrNilProducerMessage indicates produce message is nil.
	ErrNilProducerMessage = errors.New("producer message is nil")
	// ErrProducerTopicRequired indicates topic cannot be resolved.
	ErrProducerTopicRequired = errors.New("producer topic is required")
	// ErrProducerDropped indicates retry policy dropped one message.
	ErrProducerDropped = pretry.ErrMessageDropped
)
