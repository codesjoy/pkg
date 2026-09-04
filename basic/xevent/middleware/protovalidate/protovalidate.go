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

// Package protovalidate provides Buf Protovalidate middleware for decoded
// protobuf xevent events.
package protovalidate

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	bufprotovalidate "buf.build/go/protovalidate"
	"github.com/codesjoy/pkg/basic/xevent"
	"google.golang.org/protobuf/proto"
)

// ErrValidation identifies an event validation failure. Validation failures
// are also marked with xevent.ErrDiscard by Middleware.
var ErrValidation = errors.New("event validation failed")

// Config controls Protovalidate middleware initialization.
type Config struct {
	// ValidatorOptions are applied once when New constructs the Validator.
	ValidatorOptions []bufprotovalidate.ValidatorOption
}

// Middleware validates protobuf events before invoking the next event
// middleware or the bound typed handler. Non-protobuf events pass through
// unchanged.
type Middleware struct {
	validator bufprotovalidate.Validator
}

// New creates a protobuf event validation middleware and initializes its
// validator once from cfg.ValidatorOptions.
func New(cfg Config) (*Middleware, error) {
	options := append([]bufprotovalidate.ValidatorOption(nil), cfg.ValidatorOptions...)
	validator, err := bufprotovalidate.New(options...)
	if err != nil {
		return nil, fmt.Errorf("initialize protovalidate validator: %w", err)
	}
	return &Middleware{validator: validator}, nil
}

// Handle validates a protobuf event and forwards valid events.
func (m *Middleware) Handle(
	ctx context.Context,
	eventCtx *xevent.EventContext,
	next xevent.Next,
) error {
	if eventCtx == nil || isNilMessageValue(eventCtx.Event) {
		return xevent.ErrNilEvent
	}

	message, ok := eventCtx.Event.(proto.Message)
	if !ok {
		return next(ctx, eventCtx)
	}
	if message == nil || isNilMessage(message) {
		return xevent.ErrNilEvent
	}
	if err := m.validator.Validate(message); err != nil {
		return xevent.Discard(fmt.Errorf("%w: %w", ErrValidation, err))
	}
	return next(ctx, eventCtx)
}

func isNilMessage(message proto.Message) bool {
	return isNilMessageValue(message)
}

func isNilMessageValue(message any) bool {
	if message == nil {
		return true
	}
	value := reflect.ValueOf(message)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

var _ xevent.Middleware = (*Middleware)(nil)
