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

// Package publish defines middleware contracts for xnats publish paths.
package publish

import (
	"context"
	"errors"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/codesjoy/pkg/basic/xnats/internal/primitives/pipeline"
)

// ErrNilHandlerFunc indicates no final publish handler is provided.
var ErrNilHandlerFunc = errors.New("publish handler is nil")

// Message is one logical message to publish.
type Message struct {
	Subject string
	Reply   string
	Data    []byte
	Header  nats.Header
}

// Result is one successful publish result.
type Result struct {
	Subject   string
	Published time.Time
	Stream    string
	Sequence  uint64
	Duplicate bool
}

// MessageContext contains per-message metadata passed through handlers.
type MessageContext struct {
	Message    *Message
	Attempt    int
	ReceivedAt time.Time
}

// HandlerFunc handles one publish message and returns a result.
type HandlerFunc func(context.Context, *MessageContext) (*Result, error)

// Next is the next handler in a chain.
type Next func(context.Context, *MessageContext) (*Result, error)

// Handler is a chain middleware for publish handling.
type Handler interface {
	Handle(context.Context, *MessageContext, Next) (*Result, error)
}

// Func adapts a function into a Handler.
type Func func(context.Context, *MessageContext, Next) (*Result, error)

// Handle executes the adapted handler function.
func (f Func) Handle(ctx context.Context, msg *MessageContext, next Next) (*Result, error) {
	if f == nil {
		return next(ctx, msg)
	}
	return f(ctx, msg, next)
}

// Compose builds a middleware chain around a final publish handler.
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
