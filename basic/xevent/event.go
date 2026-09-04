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
	"errors"
	"fmt"
	"reflect"
)

// Event describes one domain event.
type Event interface {
	EventType() string
	EventID() string
	PartitionKey() string
	Topic() string
	MarshalPayload() ([]byte, error)
	UnmarshalPayload([]byte) error
}

// FallbackHandler handles subscribed events that have no registered typed handler.
type FallbackHandler func(context.Context, *Message) error

// Message is the minimal subscribed event input for typed dispatch.
type Message struct {
	EventType string
	Payload   []byte
}

// Handler handles one subscribed event message.
type Handler func(context.Context, *Message) error

// EventContext contains the decoded event and its original subscribed message.
// It is passed through the dispatcher middleware chain before the typed handler runs.
type EventContext struct {
	Message *Message
	Event   Event
}

// Next invokes the next event middleware or the routed typed handler.
type Next func(context.Context, *EventContext) error

// Middleware runs around the routed typed handler after the event payload has
// been decoded into a concrete Event.
type Middleware interface {
	Handle(context.Context, *EventContext, Next) error
}

// MiddlewareFunc adapts a function into a Middleware.
type MiddlewareFunc func(context.Context, *EventContext, Next) error

// Handle executes the adapted middleware function.
func (f MiddlewareFunc) Handle(
	ctx context.Context,
	eventCtx *EventContext,
	next Next,
) error {
	return f(ctx, eventCtx, next)
}

// Publisher publishes domain events.
type Publisher interface {
	Publish(context.Context, Event) error
}

// Subscriber starts and stops event consumption.
type Subscriber interface {
	Subscribe(context.Context) error
	Close() error
}

var (
	// ErrNilEvent indicates the event value is nil.
	ErrNilEvent = errors.New("event is nil")
	// ErrDiscard identifies an error for which the input message should be
	// discarded instead of retried.
	ErrDiscard = errors.New("discard event message")
	// ErrNilOutbound indicates the outbound value is nil.
	ErrNilOutbound = errors.New("event outbound is nil")
	// ErrNilMessage indicates the message is nil.
	ErrNilMessage = errors.New("event message is nil")
	// ErrEventTypeRequired indicates the event type is empty.
	ErrEventTypeRequired = errors.New("event type is required")
	// ErrNoHandlers indicates no typed handler is bound for the event type.
	ErrNoHandlers = errors.New("event handlers not found")
	// ErrInvalidEventBinding indicates the dispatcher binding is invalid.
	ErrInvalidEventBinding = errors.New("event binding is invalid")
	// ErrEventTypeConflict indicates different event types attempted to bind the same event name.
	ErrEventTypeConflict = errors.New("event type binding conflict")
	// ErrEventHandlerConflict indicates an event type already has a typed handler.
	ErrEventHandlerConflict = errors.New("event handler binding conflict")
	// ErrSubscriberStarted indicates the subscriber has already been started once.
	ErrSubscriberStarted = errors.New("subscriber has already been started")
	// ErrSubscriberClosed indicates the subscriber was closed before it started.
	ErrSubscriberClosed = errors.New("subscriber is closed")
)

// Discard marks err as a non-retryable error. Dispatchers filter discard
// errors so the transport can commit the message.
func Discard(err error) error {
	if err == nil {
		return nil
	}
	return discardError{err: err}
}

type discardError struct {
	err error
}

func (e discardError) Error() string {
	return fmt.Sprintf("%s: %v", ErrDiscard, e.err)
}

func (e discardError) Is(target error) bool {
	return target == ErrDiscard
}

func (e discardError) Unwrap() error {
	return e.err
}

// IsDiscard reports whether err, including any wrapped error, is marked for
// disposal without retry.
func IsDiscard(err error) bool {
	return errors.Is(err, ErrDiscard)
}

func isNilValue(v any) bool {
	if v == nil {
		return true
	}

	rv := reflect.ValueOf(v)
	// Reference-like kinds can be non-nil interface values wrapping nil underlying
	// values (e.g. (*MyStruct)(nil)). Check IsNil for these types specifically.
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

// cloneBytes returns a deep copy of src so the caller cannot accidentally
// alias and mutate the original slice.
func cloneBytes(src []byte) []byte {
	if src == nil {
		return nil
	}
	return append([]byte(nil), src...)
}
