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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type testOrderCreated struct {
	ID      string `json:"id"`
	OrderID string `json:"order_id"`
	UserID  string `json:"user_id"`
}

func (*testOrderCreated) EventType() string { return "order.created" }
func (e *testOrderCreated) EventID() string { return e.ID }
func (e *testOrderCreated) PartitionKey() string {
	return e.OrderID
}

func (e *testOrderCreated) MarshalPayload() ([]byte, error) {
	return json.Marshal(e)
}

func (e *testOrderCreated) UnmarshalPayload(data []byte) error {
	return json.Unmarshal(data, e)
}

type testNoPartitionEvent struct {
	ID string `json:"id"`
}

func (*testNoPartitionEvent) EventType() string { return "order.no_partition" }
func (e *testNoPartitionEvent) EventID() string { return e.ID }
func (*testNoPartitionEvent) PartitionKey() string {
	return ""
}

func (e *testNoPartitionEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(e)
}

func (e *testNoPartitionEvent) UnmarshalPayload(data []byte) error {
	return json.Unmarshal(data, e)
}

type testOrderCreatedAlias struct {
	ID string `json:"id"`
}

func (*testOrderCreatedAlias) EventType() string { return "order.created" }
func (e *testOrderCreatedAlias) EventID() string { return e.ID }
func (*testOrderCreatedAlias) PartitionKey() string {
	return ""
}

func (e *testOrderCreatedAlias) MarshalPayload() ([]byte, error) {
	return json.Marshal(e)
}

func (e *testOrderCreatedAlias) UnmarshalPayload(data []byte) error {
	return json.Unmarshal(data, e)
}

type testValueEvent struct{}

func (testValueEvent) EventType() string               { return "value.event" }
func (testValueEvent) EventID() string                 { return "evt" }
func (testValueEvent) PartitionKey() string            { return "" }
func (testValueEvent) MarshalPayload() ([]byte, error) { return nil, nil }
func (testValueEvent) UnmarshalPayload([]byte) error   { return nil }

type testSubscriber struct{}

func (testSubscriber) Subscribe(context.Context) error { return nil }
func (testSubscriber) Close() error                    { return nil }

var _ Subscriber = testSubscriber{}

func TestEventPayloadRoundTrip(t *testing.T) {
	input := &testOrderCreated{
		ID:      "evt_1",
		OrderID: "o_123",
		UserID:  "u_1",
	}

	payload, err := input.MarshalPayload()
	if err != nil {
		t.Fatalf("MarshalPayload returned error: %v", err)
	}

	var output testOrderCreated
	if err := output.UnmarshalPayload(payload); err != nil {
		t.Fatalf("UnmarshalPayload returned error: %v", err)
	}

	if output.EventType() != "order.created" {
		t.Fatalf("unexpected event type: %q", output.EventType())
	}
	if output.EventID() != "evt_1" {
		t.Fatalf("unexpected event id: %q", output.EventID())
	}
	if output.PartitionKey() != "o_123" {
		t.Fatalf("unexpected partition key: %q", output.PartitionKey())
	}
}

func TestEventPartitionKeyMayBeEmpty(t *testing.T) {
	event := &testNoPartitionEvent{ID: "evt_2"}
	if event.PartitionKey() != "" {
		t.Fatalf("expected empty partition key, got %q", event.PartitionKey())
	}
	if event.EventID() != "evt_2" {
		t.Fatalf("unexpected event id: %q", event.EventID())
	}
}

func TestSubscriberLifecycleErrors(t *testing.T) {
	if ErrSubscriberStarted == nil {
		t.Fatal("expected ErrSubscriberStarted")
	}
	if ErrSubscriberClosed == nil {
		t.Fatal("expected ErrSubscriberClosed")
	}
	if errors.Is(ErrSubscriberStarted, ErrSubscriberClosed) {
		t.Fatal("subscriber lifecycle errors should be distinct")
	}
}

func TestDispatcherOnAndHandle(t *testing.T) {
	dispatcher := newTestDispatcher()

	var got *testOrderCreated
	err := On[*testOrderCreated](
		dispatcher,
		func(_ context.Context, event *testOrderCreated) error {
			got = event
			return nil
		},
	)
	if err != nil {
		t.Fatalf("On returned error: %v", err)
	}

	payload, err := (&testOrderCreated{
		ID:      "evt_1",
		OrderID: "o_123",
		UserID:  "u_1",
	}).MarshalPayload()
	if err != nil {
		t.Fatalf("MarshalPayload returned error: %v", err)
	}

	err = dispatcher.Handle(context.Background(), &Message{
		EventType: "order.created",
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if got == nil {
		t.Fatal("expected typed handler to receive event")
	}
	if got.EventID() != "evt_1" || got.PartitionKey() != "o_123" || got.UserID != "u_1" {
		t.Fatalf("unexpected typed event: %#v", got)
	}
}

func TestDispatcherHandleMultipleHandlersOrderAndErrors(t *testing.T) {
	dispatcher := newTestDispatcher()

	var order []string
	firstErr := errors.New("first")
	secondErr := errors.New("second")

	err := On[*testOrderCreated](
		dispatcher,
		func(_ context.Context, event *testOrderCreated) error {
			order = append(order, "first:"+event.UserID)
			return firstErr
		},
	)
	if err != nil {
		t.Fatalf("On returned error: %v", err)
	}
	err = On[*testOrderCreated](dispatcher, func(_ context.Context, event *testOrderCreated) error {
		order = append(order, "second:"+event.UserID)
		return secondErr
	})
	if err != nil {
		t.Fatalf("On returned error: %v", err)
	}

	payload, err := (&testOrderCreated{
		ID:      "evt_2",
		OrderID: "o_456",
		UserID:  "u_2",
	}).MarshalPayload()
	if err != nil {
		t.Fatalf("MarshalPayload returned error: %v", err)
	}

	err = dispatcher.Handle(context.Background(), &Message{
		EventType: "order.created",
		Payload:   payload,
	})
	if !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("expected joined handler errors, got %v", err)
	}
	if strings.Join(order, ",") != "first:u_2,second:u_2" {
		t.Fatalf("unexpected handler order: %v", order)
	}
}

func TestDispatcherHandleNoHandlers(t *testing.T) {
	dispatcher := newTestDispatcher()

	payload, err := (&testOrderCreated{
		ID:      "evt_3",
		OrderID: "o_789",
		UserID:  "u_3",
	}).MarshalPayload()
	if err != nil {
		t.Fatalf("MarshalPayload returned error: %v", err)
	}

	err = dispatcher.Handle(context.Background(), &Message{
		EventType: "order.created",
		Payload:   payload,
	})
	if !errors.Is(err, ErrNoHandlers) {
		t.Fatalf("expected ErrNoHandlers, got %v", err)
	}
}

func TestDispatcherHandleInvalidPayload(t *testing.T) {
	dispatcher := newTestDispatcher()

	err := On[*testOrderCreated](dispatcher, func(context.Context, *testOrderCreated) error {
		return nil
	})
	if err != nil {
		t.Fatalf("On returned error: %v", err)
	}

	err = dispatcher.Handle(context.Background(), &Message{
		EventType: "order.created",
		Payload:   []byte(`{`),
	})
	if err == nil {
		t.Fatal("expected payload decode error")
	}
}

func TestDispatcherHandleValidation(t *testing.T) {
	dispatcher := newTestDispatcher()

	err := dispatcher.Handle(context.Background(), nil)
	if !errors.Is(err, ErrNilMessage) {
		t.Fatalf("expected ErrNilMessage, got %v", err)
	}

	err = dispatcher.Handle(context.Background(), &Message{})
	if !errors.Is(err, ErrEventTypeRequired) {
		t.Fatalf("expected ErrEventTypeRequired, got %v", err)
	}
}

func TestDispatcherOnInvalidGenericBinding(t *testing.T) {
	dispatcher := newTestDispatcher()

	err := On[testValueEvent](dispatcher, func(context.Context, testValueEvent) error {
		return nil
	})
	if !errors.Is(err, ErrInvalidEventBinding) {
		t.Fatalf("expected ErrInvalidEventBinding, got %v", err)
	}
}

func TestDispatcherOnSameTypeMultipleHandlers(t *testing.T) {
	dispatcher := newTestDispatcher()

	if err := On[*testOrderCreated](dispatcher, func(context.Context, *testOrderCreated) error { return nil }); err != nil {
		t.Fatalf("first On returned error: %v", err)
	}
	if err := On[*testOrderCreated](dispatcher, func(context.Context, *testOrderCreated) error { return nil }); err != nil {
		t.Fatalf("second On returned error: %v", err)
	}

	if got := len(dispatcher.bindings["order.created"].handlers); got != 2 {
		t.Fatalf("expected 2 handlers, got %d", got)
	}
	if dispatcher.bindings["order.created"].newEvent == nil {
		t.Fatal("expected constructor to be registered")
	}
}

func TestDispatcherOnEventTypeConflict(t *testing.T) {
	dispatcher := newTestDispatcher()

	if err := On[*testOrderCreated](dispatcher, func(context.Context, *testOrderCreated) error { return nil }); err != nil {
		t.Fatalf("first On returned error: %v", err)
	}

	err := On[*testOrderCreatedAlias](
		dispatcher,
		func(context.Context, *testOrderCreatedAlias) error { return nil },
	)
	if !errors.Is(err, ErrEventTypeConflict) {
		t.Fatalf("expected ErrEventTypeConflict, got %v", err)
	}
}

func newTestDispatcher() *Dispatcher {
	return NewDispatcher()
}

func ExampleOn() {
	dispatcher := NewDispatcher()
	_ = On[*testOrderCreated](dispatcher, func(_ context.Context, event *testOrderCreated) error {
		fmt.Println(event.OrderID)
		return nil
	})

	payload, _ := (&testOrderCreated{OrderID: "o_123"}).MarshalPayload()
	_ = dispatcher.Handle(context.Background(), &Message{
		EventType: "order.created",
		Payload:   payload,
	})

	// Output: o_123
}
