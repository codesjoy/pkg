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
	"sync"
)

type dispatchFunc func(context.Context, Event) error

type eventBinding struct {
	prototypeType reflect.Type
	newEvent      func() Event
	handlers      []dispatchFunc
}

// Dispatcher routes subscribed event messages to typed handlers.
type Dispatcher struct {
	mu       sync.RWMutex
	bindings map[string]eventBinding
}

// NewDispatcher creates a typed event dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		bindings: make(map[string]eventBinding),
	}
}

// Handle decodes one subscribed message and routes it to all bound typed handlers.
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

	binding := d.bindingFor(msg.EventType)
	if binding.newEvent == nil || len(binding.handlers) == 0 {
		return ErrNoHandlers
	}

	var errs []error
	for _, handler := range binding.handlers {
		event := binding.newEvent()
		if isNilValue(event) {
			return ErrNilEvent
		}
		if err := event.UnmarshalPayload(cloneBytes(msg.Payload)); err != nil {
			return err
		}
		if err := handler(ctx, event); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// On binds a typed handler to a dispatcher and automatically registers the
// underlying event type.
func On[T Event](d *Dispatcher, handler func(context.Context, T) error) error {
	eventType, prototypeType, newEvent, err := bindingSpec[T]()
	if err != nil {
		return err
	}
	if d == nil || isNilValue(handler) {
		return ErrInvalidEventBinding
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	binding, exists := d.bindings[eventType]
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
	} else {
		binding = eventBinding{
			prototypeType: prototypeType,
			newEvent:      newEvent,
		}
	}

	binding.handlers = append(binding.handlers, func(ctx context.Context, event Event) error {
		typed, ok := event.(T)
		if !ok {
			return fmt.Errorf(
				"%w: handler expects %s but got %T",
				ErrInvalidEventBinding,
				prototypeType,
				event,
			)
		}
		return handler(ctx, typed)
	})
	d.bindings[eventType] = binding
	return nil
}

func (d *Dispatcher) bindingFor(eventType string) eventBinding {
	d.mu.RLock()
	defer d.mu.RUnlock()

	binding, ok := d.bindings[eventType]
	if !ok {
		return eventBinding{}
	}

	cloned := make([]dispatchFunc, len(binding.handlers))
	copy(cloned, binding.handlers)
	binding.handlers = cloned
	return binding
}

func bindingSpec[T Event]() (string, reflect.Type, func() Event, error) {
	eventType := reflect.TypeFor[T]()
	if eventType == nil || eventType.Kind() != reflect.Pointer ||
		eventType.Elem().Kind() != reflect.Struct {
		return "", nil, nil, ErrInvalidEventBinding
	}

	instance, ok := reflect.New(eventType.Elem()).Interface().(T)
	if !ok || isNilValue(instance) {
		return "", nil, nil, ErrInvalidEventBinding
	}

	derivedEventType := instance.EventType()
	if derivedEventType == "" {
		return "", nil, nil, ErrEventTypeRequired
	}

	return derivedEventType, eventType, func() Event {
		created, ok := reflect.New(eventType.Elem()).Interface().(Event)
		if !ok {
			return nil
		}
		return created
	}, nil
}
