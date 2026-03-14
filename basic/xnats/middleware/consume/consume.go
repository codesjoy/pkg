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

// Package consume defines middleware contracts for xnats consume paths.
package consume

import (
	"context"
	"errors"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/codesjoy/pkg/basic/xnats/internal/primitives/pipeline"
)

// ErrNilHandlerFunc indicates no final consumer message handler is provided.
var ErrNilHandlerFunc = errors.New("consume handler is nil")

// Transport identifies which transport delivered the message.
type Transport string

const (
	// TransportCore indicates core NATS pub/sub delivery.
	TransportCore Transport = "core"
	// TransportJetStream indicates JetStream delivery.
	TransportJetStream Transport = "jetstream"
)

// Acknowledger abstracts transport-specific ack/nak behavior.
type Acknowledger interface {
	Ack() error
	Nak() error
	Handled() bool
}

// JetStreamMetadata contains optional JetStream delivery metadata.
type JetStreamMetadata struct {
	Stream           string
	Consumer         string
	Domain           string
	NumDelivered     uint64
	NumPending       uint64
	StreamSequence   uint64
	ConsumerSequence uint64
	Timestamp        time.Time
}

// HandlerFunc handles one NATS message.
type HandlerFunc func(context.Context, *MessageContext) error

// Next is the next handler in a chain.
type Next func(context.Context, *MessageContext) error

// Handler is a chain middleware for message handling.
type Handler interface {
	Handle(context.Context, *MessageContext, Next) error
}

// Func adapts a function into a Handler.
type Func func(context.Context, *MessageContext, Next) error

// Handle executes the adapted handler function.
func (f Func) Handle(ctx context.Context, msg *MessageContext, next Next) error {
	if f == nil {
		return next(ctx, msg)
	}
	return f(ctx, msg, next)
}

// MessageContext contains per-message metadata passed through handlers.
type MessageContext struct {
	Message    *nats.Msg
	Transport  Transport
	Subject    string
	Reply      string
	Attempt    int
	LogicalKey string
	Shard      int
	ReceivedAt time.Time
	JetStream  *JetStreamMetadata
	Acker      Acknowledger
}

// Compose builds a middleware chain around a final business handler.
func Compose(handlers []Handler, final HandlerFunc) HandlerFunc {
	if final == nil {
		return func(context.Context, *MessageContext) error {
			return ErrNilHandlerFunc
		}
	}

	adapted := make(
		[]func(context.Context, *MessageContext, func(context.Context, *MessageContext) error) error,
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
			next func(context.Context, *MessageContext) error,
		) error {
			return current.Handle(ctx, msg, Next(next))
		})
	}

	return pipeline.ComposeError(adapted, final)
}
