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

// Package produce provides producer-side middleware types and composition helpers.
package produce

import (
	"context"
	"errors"
	"time"

	"github.com/IBM/sarama"

	"github.com/codesjoy/pkg/basic/xkafka/internal/primitives/pipeline"
)

// ErrNilHandlerFunc indicates no final message producer handler is provided.
var ErrNilHandlerFunc = errors.New("produce handler is nil")

// Message is one logical message to produce.
type Message struct {
	Topic     string
	Key       []byte
	Value     []byte
	Headers   []sarama.RecordHeader
	Timestamp time.Time
}

// Result is one successful produce result.
type Result struct {
	Topic     string
	Partition int32
	Offset    int64
	Timestamp time.Time
	Attempt   int
}

// Future is an async result handle for one produce call.
type Future interface {
	Await(context.Context) (*Result, error)
	Done() <-chan struct{}
}

// MessageContext contains per-message metadata passed through handlers.
type MessageContext struct {
	Message     *Message
	Attempt     int
	ReceivedAt  time.Time
	DispatchKey string
	Worker      int
}

// HandlerFunc handles one producer message and returns a broker result.
type HandlerFunc func(context.Context, *MessageContext) (*Result, error)

// Next is the next handler in a chain.
type Next func(context.Context, *MessageContext) (*Result, error)

// Handler is a chain middleware for produce handling.
type Handler interface {
	Handle(context.Context, *MessageContext, Next) (*Result, error)
}

// Func adapts a function into a Handler.
type Func func(context.Context, *MessageContext, Next) (*Result, error)

// Handle executes the adapted handler function.
func (f Func) Handle(
	ctx context.Context,
	msg *MessageContext,
	next Next,
) (*Result, error) {
	if f == nil {
		return next(ctx, msg)
	}
	return f(ctx, msg, next)
}

// Compose builds a middleware chain around a final business handler.
func Compose(handlers []Handler, final HandlerFunc) HandlerFunc {
	if final == nil {
		return func(context.Context, *MessageContext) (*Result, error) {
			return nil, ErrNilHandlerFunc
		}
	}

	adapted := make(
		[]func(context.Context, *MessageContext, func(context.Context, *MessageContext) (*Result, error)) (*Result, error),
		0,
		len(handlers),
	)
	for _, handler := range handlers {
		if handler == nil {
			continue
		}
		current := handler
		adapted = append(adapted, func(
			ctx context.Context,
			msg *MessageContext,
			next func(context.Context, *MessageContext) (*Result, error),
		) (*Result, error) {
			return current.Handle(ctx, msg, Next(next))
		})
	}

	return pipeline.ComposeResult(adapted, final)
}
