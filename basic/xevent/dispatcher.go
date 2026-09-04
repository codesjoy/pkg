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

package xevent

import (
	"context"
	"fmt"
	"reflect"
)

type dispatchFunc = Next

type eventBinding struct {
	prototypeType reflect.Type
	newEvent      func() Event
	handler       dispatchFunc
}

// Dispatcher routes subscribed event messages to typed handlers.
// Configure it with On, Use, and SetFallback before calling Handle; those
// configuration methods are not safe to call concurrently with Handle.
type Dispatcher struct {
	bindings    map[string]eventBinding
	fallback    FallbackHandler
	middlewares []Middleware
	dispatch    Next
}

// NewDispatcher creates a typed event dispatcher.
func NewDispatcher() *Dispatcher {
	d := &Dispatcher{
		bindings: make(map[string]eventBinding),
	}
	d.dispatch = d.dispatchBoundHandler
	return d
}

// Handle decodes one subscribed message and routes it through the dispatcher
// middleware chain to its bound typed handler.
func (d *Dispatcher) Handle(ctx context.Context, msg *Message) error {
	if d == nil {
		return ErrInvalidEventBinding
	}
	if msg == nil {
		return ErrNilMessage
	}
	if msg.EventType == "" {
		return ErrEventTypeRequired
	}

	binding := d.bindings[msg.EventType]
	if binding.handler == nil {
		// Step 2: no typed handler — try fallback.
		if d.fallback != nil {
			return d.fallback(ctx, msg)
		}
		return ErrNoHandlers
	}

	// Step 3: decode one event before running the dispatcher middleware chain.
	payload := cloneBytes(msg.Payload)
	event := binding.newEvent()
	if err := event.UnmarshalPayload(payload); err != nil {
		return err
	}

	eventCtx := &EventContext{Message: msg, Event: event}
	err := d.dispatch(ctx, eventCtx)
	return filterDiscardErrors(err)
}

// Use appends event middleware to the dispatcher chain.
// Middleware is executed in registration order once for each typed message.
// A nil middleware or nil dispatcher is ignored.
func (d *Dispatcher) Use(middlewares ...Middleware) {
	if d == nil || len(middlewares) == 0 {
		return
	}

	valid := make([]Middleware, 0, len(middlewares))
	for _, middleware := range middlewares {
		if isNilValue(middleware) {
			continue
		}
		valid = append(valid, middleware)
	}
	if len(valid) == 0 {
		return
	}
	next := make([]Middleware, len(d.middlewares)+len(valid))
	copy(next, d.middlewares)
	copy(next[len(d.middlewares):], valid)
	d.middlewares = next
	d.dispatch = composeEventMiddleware(d.middlewares, d.dispatchBoundHandler)
}

// On binds one typed handler to a dispatcher and automatically registers the
// underlying event type. An event type can only be bound once.
func On[T Event](d *Dispatcher, handler func(context.Context, T) error) error {
	eventType, prototypeType, newEvent, err := bindingSpec[T]()
	if err != nil {
		return err
	}
	if d == nil || isNilValue(handler) {
		return ErrInvalidEventBinding
	}

	binding, exists := d.bindings[eventType]
	// Conflict detection: reject if the same event type name was already bound
	// to a different concrete Go type.
	if exists {
		if binding.prototypeType != prototypeType {
			return fmt.Errorf(
				"%w: eventType=%s existing=%s new=%s",
				ErrEventTypeConflict,
				eventType,
				binding.prototypeType,
				prototypeType,
			)
		}
		if binding.handler != nil {
			return fmt.Errorf(
				"%w: eventType=%s",
				ErrEventHandlerConflict,
				eventType,
			)
		}
	} else {
		binding = eventBinding{
			prototypeType: prototypeType,
			newEvent:      newEvent,
		}
	}

	// Wrap the user's typed handler in a closure that performs the
	// Event → T type assertion before delegating.
	binding.handler = func(ctx context.Context, eventCtx *EventContext) error {
		if eventCtx == nil || isNilValue(eventCtx.Event) {
			return ErrNilEvent
		}
		typed, ok := eventCtx.Event.(T)
		if !ok {
			return fmt.Errorf(
				"%w: handler expects %s but got %T",
				ErrInvalidEventBinding,
				prototypeType,
				eventCtx.Event,
			)
		}
		return handler(ctx, typed)
	}
	d.bindings[eventType] = binding
	return nil
}

// SetFallback registers a fallback handler for events with no registered typed handler.
func (d *Dispatcher) SetFallback(fb FallbackHandler) {
	d.fallback = fb
}

func (d *Dispatcher) dispatchBoundHandler(ctx context.Context, eventCtx *EventContext) error {
	if eventCtx == nil || eventCtx.Message == nil {
		return ErrInvalidEventBinding
	}
	binding := d.bindings[eventCtx.Message.EventType]
	if binding.handler == nil {
		return ErrNoHandlers
	}
	return binding.handler(ctx, eventCtx)
}

func composeEventMiddleware(middlewares []Middleware, final Next) Next {
	chain := final
	for i := len(middlewares) - 1; i >= 0; i-- {
		middleware := middlewares[i]
		next := chain
		chain = func(ctx context.Context, eventCtx *EventContext) error {
			return middleware.Handle(ctx, eventCtx, next)
		}
	}
	return chain
}

// filterDiscardErrors treats any error containing an explicitly discarded
// branch as fully discardable, including wrapped and joined errors.
func filterDiscardErrors(err error) error {
	if IsDiscard(err) {
		return nil
	}
	return err
}

func bindingSpec[T Event]() (string, reflect.Type, func() Event, error) {
	eventType := reflect.TypeFor[T]()
	// Enforce the generic constraint: T must be a pointer-to-struct so that
	// reflect.New can instantiate a concrete prototype.
	if eventType.Kind() != reflect.Pointer ||
		eventType.Elem().Kind() != reflect.Struct {
		return "", nil, nil, ErrInvalidEventBinding
	}

	prototype := reflect.New(eventType.Elem()).Interface().(Event)
	derivedEventType := prototype.EventType()
	if derivedEventType == "" {
		return "", nil, nil, ErrEventTypeRequired
	}

	return derivedEventType, eventType, func() Event {
		return reflect.New(eventType.Elem()).Interface().(Event)
	}, nil
}
