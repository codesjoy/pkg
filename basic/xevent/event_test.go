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
	"sync"
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
func (*testOrderCreated) Topic() string { return "" }

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
func (*testNoPartitionEvent) Topic() string { return "" }

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
func (*testOrderCreatedAlias) Topic() string { return "" }

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
func (testValueEvent) Topic() string                   { return "" }
func (testValueEvent) MarshalPayload() ([]byte, error) { return nil, nil }
func (testValueEvent) UnmarshalPayload([]byte) error   { return nil }

type testEmptyTypeEvent struct{}

func (*testEmptyTypeEvent) EventType() string               { return "" }
func (*testEmptyTypeEvent) EventID() string                 { return "evt-empty" }
func (*testEmptyTypeEvent) PartitionKey() string            { return "" }
func (*testEmptyTypeEvent) Topic() string                   { return "" }
func (*testEmptyTypeEvent) MarshalPayload() ([]byte, error) { return []byte("payload"), nil }
func (*testEmptyTypeEvent) UnmarshalPayload([]byte) error   { return nil }

type testFailMarshalEvent struct {
	err error
}

func (*testFailMarshalEvent) EventType() string { return "marshal.failed" }
func (*testFailMarshalEvent) EventID() string   { return "evt-fail" }
func (*testFailMarshalEvent) PartitionKey() string {
	return ""
}
func (*testFailMarshalEvent) Topic() string { return "" }

func (e *testFailMarshalEvent) MarshalPayload() ([]byte, error) {
	return nil, e.err
}

func (*testFailMarshalEvent) UnmarshalPayload([]byte) error {
	return nil
}

type testSubscriber struct{}

func (testSubscriber) Subscribe(context.Context) error { return nil }
func (testSubscriber) Close() error                    { return nil }

type testPublisher struct {
	last Event
	err  error
}

func (p *testPublisher) Publish(_ context.Context, event Event) error {
	p.last = event
	return p.err
}

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

func TestEncodeCopiesOutboundFields(t *testing.T) {
	event := &testOrderCreated{
		ID:      "evt_4",
		OrderID: "o_999",
		UserID:  "u_9",
	}

	outbound, err := Encode(event)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	if outbound.EventType != "order.created" {
		t.Fatalf("unexpected event type: %q", outbound.EventType)
	}
	if outbound.EventID != "evt_4" {
		t.Fatalf("unexpected event id: %q", outbound.EventID)
	}
	if outbound.PartitionKey != "o_999" {
		t.Fatalf("unexpected partition key: %q", outbound.PartitionKey)
	}
	if len(outbound.Payload) == 0 {
		t.Fatal("expected outbound payload")
	}

	payload, err := event.MarshalPayload()
	if err != nil {
		t.Fatalf("MarshalPayload returned error: %v", err)
	}
	payload[0] = '['
	if string(outbound.Payload) == string(payload) {
		t.Fatal("expected Encode to copy payload")
	}
}

func TestEncodeValidationAndMarshalErrors(t *testing.T) {
	t.Run("nil event", func(t *testing.T) {
		var event *testOrderCreated
		_, err := Encode(event)
		if !errors.Is(err, ErrNilEvent) {
			t.Fatalf("expected ErrNilEvent, got %v", err)
		}
	})

	t.Run("empty event type", func(t *testing.T) {
		_, err := Encode(&testEmptyTypeEvent{})
		if !errors.Is(err, ErrEventTypeRequired) {
			t.Fatalf("expected ErrEventTypeRequired, got %v", err)
		}
	})

	t.Run("marshal error", func(t *testing.T) {
		want := errors.New("marshal failed")
		_, err := Encode(&testFailMarshalEvent{err: want})
		if !errors.Is(err, want) {
			t.Fatalf("expected marshal error, got %v", err)
		}
	})
}

func TestSenderFromPublisherPublishesOutbound(t *testing.T) {
	publisher := &testPublisher{}
	sender := SenderFromPublisher(publisher)

	outbound := &Outbound{
		EventType:    "order.created",
		EventID:      "evt_5",
		PartitionKey: "o_555",
		Payload:      []byte(`{"id":"evt_5","order_id":"o_555"}`),
	}
	if err := sender.Send(context.Background(), outbound); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if publisher.last == nil {
		t.Fatal("expected publisher event")
	}
	if publisher.last.EventType() != outbound.EventType {
		t.Fatalf("unexpected event type: %q", publisher.last.EventType())
	}
	if publisher.last.EventID() != outbound.EventID {
		t.Fatalf("unexpected event id: %q", publisher.last.EventID())
	}
	if publisher.last.PartitionKey() != outbound.PartitionKey {
		t.Fatalf("unexpected partition key: %q", publisher.last.PartitionKey())
	}
	payload, err := publisher.last.MarshalPayload()
	if err != nil {
		t.Fatalf("MarshalPayload returned error: %v", err)
	}
	if string(payload) != string(outbound.Payload) {
		t.Fatalf("unexpected payload: %s", payload)
	}

	outbound.Payload[0] = '['
	if string(payload) == string(outbound.Payload) {
		t.Fatal("expected payload copy inside sender adapter")
	}
}

func TestSenderFromPublisherValidationAndErrorPropagation(t *testing.T) {
	t.Run("nil outbound", func(t *testing.T) {
		err := SenderFromPublisher(&testPublisher{}).Send(context.Background(), nil)
		if !errors.Is(err, ErrNilOutbound) {
			t.Fatalf("expected ErrNilOutbound, got %v", err)
		}
	})

	t.Run("empty event type", func(t *testing.T) {
		err := SenderFromPublisher(&testPublisher{}).Send(context.Background(), &Outbound{})
		if !errors.Is(err, ErrEventTypeRequired) {
			t.Fatalf("expected ErrEventTypeRequired, got %v", err)
		}
	})

	t.Run("nil publisher", func(t *testing.T) {
		err := SenderFromPublisher(nil).Send(context.Background(), &Outbound{EventType: "evt"})
		if !errors.Is(err, ErrInvalidEventBinding) {
			t.Fatalf("expected ErrInvalidEventBinding, got %v", err)
		}
	})

	t.Run("publisher error", func(t *testing.T) {
		want := errors.New("publish failed")
		err := SenderFromPublisher(&testPublisher{err: want}).Send(context.Background(), &Outbound{
			EventType: "evt",
			Payload:   []byte("payload"),
		})
		if !errors.Is(err, want) {
			t.Fatalf("expected publisher error, got %v", err)
		}
	})
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

func TestDispatcherMiddlewareOrderAndContext(t *testing.T) {
	dispatcher := newTestDispatcher()
	var order []string

	dispatcher.Use(
		MiddlewareFunc(func(ctx context.Context, eventCtx *EventContext, next Next) error {
			if eventCtx.Message == nil || eventCtx.Event == nil {
				t.Fatal("expected middleware to receive message and decoded event")
			}
			if eventCtx.Event.EventID() != "evt-middleware" {
				t.Fatalf("unexpected event ID: %q", eventCtx.Event.EventID())
			}
			order = append(order, "first-before")
			err := next(context.WithValue(ctx, middlewareContextKey{}, "set"), eventCtx)
			order = append(order, "first-after")
			return err
		}),
		MiddlewareFunc(func(ctx context.Context, eventCtx *EventContext, next Next) error {
			order = append(order, "second-before")
			if ctx.Value(middlewareContextKey{}) != "set" {
				t.Fatal("expected middleware context value")
			}
			err := next(ctx, eventCtx)
			order = append(order, "second-after")
			return err
		}),
	)

	if err := On[*testOrderCreated](
		dispatcher,
		func(_ context.Context, event *testOrderCreated) error {
			order = append(order, "handler:"+event.EventID())
			return nil
		},
	); err != nil {
		t.Fatalf("On returned error: %v", err)
	}

	err := dispatcher.Handle(context.Background(), &Message{
		EventType: "order.created",
		Payload:   []byte(`{"id":"evt-middleware"}`),
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	want := []string{
		"first-before",
		"second-before",
		"handler:evt-middleware",
		"second-after",
		"first-after",
	}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected middleware order: %v", order)
	}
}

func TestDispatcherMiddlewareRunsOnceAroundHandler(t *testing.T) {
	dispatcher := newTestDispatcher()
	var order []string
	dispatcher.Use(
		MiddlewareFunc(func(ctx context.Context, eventCtx *EventContext, next Next) error {
			order = append(order, "before")
			err := next(ctx, eventCtx)
			order = append(order, "after")
			return err
		}),
	)
	if err := On[*testOrderCreated](dispatcher, func(context.Context, *testOrderCreated) error {
		order = append(order, "handler")
		return nil
	}); err != nil {
		t.Fatalf("On returned error: %v", err)
	}

	if err := dispatcher.Handle(context.Background(), &Message{
		EventType: "order.created",
		Payload:   []byte(`{"id":"evt-handler"}`),
	}); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if got, want := strings.Join(order, ","), "before,handler,after"; got != want {
		t.Fatalf("unexpected middleware handler order: %s", got)
	}
}

func TestDispatcherGlobalMiddlewareRoutesMultipleEvents(t *testing.T) {
	dispatcher := newTestDispatcher()
	var middlewareEvents []string
	dispatcher.Use(
		MiddlewareFunc(func(ctx context.Context, eventCtx *EventContext, next Next) error {
			middlewareEvents = append(middlewareEvents, eventCtx.Event.EventType())
			return next(ctx, eventCtx)
		}),
	)

	var orderCreated, noPartition bool
	if err := On[*testOrderCreated](dispatcher, func(context.Context, *testOrderCreated) error {
		orderCreated = true
		return nil
	}); err != nil {
		t.Fatalf("On order.created returned error: %v", err)
	}
	if err := On[*testNoPartitionEvent](
		dispatcher,
		func(context.Context, *testNoPartitionEvent) error {
			noPartition = true
			return nil
		},
	); err != nil {
		t.Fatalf("On order.no_partition returned error: %v", err)
	}

	for _, message := range []*Message{
		{EventType: "order.created", Payload: []byte(`{"id":"created"}`)},
		{EventType: "order.no_partition", Payload: []byte(`{"id":"no-partition"}`)},
	} {
		if err := dispatcher.Handle(context.Background(), message); err != nil {
			t.Fatalf("Handle(%s) returned error: %v", message.EventType, err)
		}
	}

	if !orderCreated || !noPartition {
		t.Fatalf(
			"expected both handlers to run: created=%v no_partition=%v",
			orderCreated,
			noPartition,
		)
	}
	if got := strings.Join(middlewareEvents, ","); got != "order.created,order.no_partition" {
		t.Fatalf("unexpected middleware events: %s", got)
	}
}

func TestDispatcherMiddlewareShortCircuit(t *testing.T) {
	dispatcher := newTestDispatcher()
	wantErr := errors.New("middleware stopped dispatch")
	dispatcher.Use(MiddlewareFunc(func(context.Context, *EventContext, Next) error {
		return wantErr
	}))

	called := false
	if err := On[*testOrderCreated](dispatcher, func(context.Context, *testOrderCreated) error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("On returned error: %v", err)
	}

	err := dispatcher.Handle(context.Background(), &Message{
		EventType: "order.created",
		Payload:   []byte(`{"id":"evt-short-circuit"}`),
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected middleware error, got %v", err)
	}
	if called {
		t.Fatal("expected middleware to prevent handler execution")
	}
}

func TestDispatcherUseIgnoresNilMiddlewareFunc(t *testing.T) {
	dispatcher := newTestDispatcher()
	var middleware MiddlewareFunc
	dispatcher.Use(middleware)

	called := false
	if err := On[*testOrderCreated](dispatcher, func(context.Context, *testOrderCreated) error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("On returned error: %v", err)
	}

	err := dispatcher.Handle(context.Background(), &Message{
		EventType: "order.created",
		Payload:   []byte(`{"id":"evt-nil-middleware"}`),
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if !called {
		t.Fatal("expected handler to be called")
	}
}

func TestDispatcherDiscardErrorsAreFiltered(t *testing.T) {
	dispatcher := newTestDispatcher()
	wantErr := errors.New("invalid event payload")
	dispatcher.Use(MiddlewareFunc(func(context.Context, *EventContext, Next) error {
		return Discard(wantErr)
	}))

	called := false
	if err := On[*testOrderCreated](dispatcher, func(context.Context, *testOrderCreated) error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("On returned error: %v", err)
	}

	err := dispatcher.Handle(context.Background(), &Message{
		EventType: "order.created",
		Payload:   []byte(`{"id":"evt-discard"}`),
	})
	if err != nil {
		t.Fatalf("expected discard error to be filtered, got %v", err)
	}
	if called {
		t.Fatal("expected discard middleware to prevent handler execution")
	}
}

func TestDispatcherHandlerBusinessError(t *testing.T) {
	dispatcher := newTestDispatcher()
	businessErr := errors.New("database unavailable")

	if err := On[*testOrderCreated](dispatcher, func(context.Context, *testOrderCreated) error {
		return businessErr
	}); err != nil {
		t.Fatalf("On returned error: %v", err)
	}

	err := dispatcher.Handle(context.Background(), &Message{
		EventType: "order.created",
		Payload:   []byte(`{"id":"evt-mixed-errors"}`),
	})
	if !errors.Is(err, businessErr) {
		t.Fatalf("expected business error, got %v", err)
	}
}

func TestDiscardHelpers(t *testing.T) {
	if Discard(nil) != nil {
		t.Fatal("expected Discard(nil) to return nil")
	}
	baseErr := errors.New("permanent failure")
	err := Discard(baseErr)
	if !IsDiscard(err) || !errors.Is(err, baseErr) {
		t.Fatalf("expected discard and cause matches, got %v", err)
	}
}

func TestDispatcherMiddlewarePreservesJoinedErrors(t *testing.T) {
	dispatcher := newTestDispatcher()
	firstErr := errors.New("first middleware error")
	secondErr := errors.New("second middleware error")
	dispatcher.Use(MiddlewareFunc(func(context.Context, *EventContext, Next) error {
		return errors.Join(firstErr, secondErr)
	}))

	handler := func(context.Context, *testOrderCreated) error {
		t.Fatal("expected middleware to short-circuit the typed handler")
		return nil
	}
	if err := On[*testOrderCreated](dispatcher, handler); err != nil {
		t.Fatalf("On returned error: %v", err)
	}

	err := dispatcher.Handle(context.Background(), &Message{
		EventType: "order.created",
		Payload:   []byte(`{"id":"evt-errors"}`),
	})
	if !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("expected joined middleware errors, got %v", err)
	}
}

func TestDispatcherFiltersDiscardBranchFromMiddlewareError(t *testing.T) {
	dispatcher := newTestDispatcher()
	discardErr := errors.New("invalid event")
	businessErr := errors.New("backend unavailable")
	dispatcher.Use(MiddlewareFunc(func(context.Context, *EventContext, Next) error {
		return errors.Join(Discard(discardErr), businessErr)
	}))
	if err := On[*testOrderCreated](dispatcher, func(context.Context, *testOrderCreated) error {
		t.Fatal("middleware should short-circuit the handler")
		return nil
	}); err != nil {
		t.Fatalf("On returned error: %v", err)
	}

	err := dispatcher.Handle(context.Background(), &Message{
		EventType: "order.created",
		Payload:   []byte(`{"id":"evt-filter"}`),
	})
	if !errors.Is(err, businessErr) {
		t.Fatalf("expected business error to survive filtering, got %v", err)
	}
	if errors.Is(err, discardErr) {
		t.Fatalf("expected discard branch to be removed, got %v", err)
	}
}

func TestDispatcherPreservesOrdinaryErrorWrapper(t *testing.T) {
	dispatcher := newTestDispatcher()
	businessErr := errors.New("backend unavailable")
	wrappedErr := fmt.Errorf("handle order: %w", businessErr)
	dispatcher.Use(MiddlewareFunc(func(context.Context, *EventContext, Next) error {
		return wrappedErr
	}))
	if err := On[*testOrderCreated](dispatcher, func(context.Context, *testOrderCreated) error {
		return nil
	}); err != nil {
		t.Fatalf("On returned error: %v", err)
	}

	err := dispatcher.Handle(context.Background(), &Message{
		EventType: "order.created",
		Payload:   []byte(`{"id":"evt-wrapped"}`),
	})
	if err != wrappedErr || !errors.Is(err, businessErr) {
		t.Fatalf("expected ordinary wrapper to be preserved, got %v", err)
	}
}

func TestFilterDiscardErrorsPreservesUnmarkedError(t *testing.T) {
	baseErr := errors.New("backend unavailable")
	wrappedErr := fmt.Errorf("handle order: %w", baseErr)

	if got := filterDiscardErrors(wrappedErr); got != wrappedErr {
		t.Fatalf("expected unmarked error wrapper to be preserved, got %v", got)
	}
}

func TestFilterDiscardErrorsRemovesNestedDiscardBranches(t *testing.T) {
	discardErr := errors.New("invalid payload")
	businessErr := errors.New("backend unavailable")
	joined := errors.Join(
		fmt.Errorf("validation: %w", Discard(discardErr)),
		businessErr,
	)

	filtered := filterDiscardErrors(joined)
	if filtered == nil || !errors.Is(filtered, businessErr) {
		t.Fatalf("expected business error to survive filtering, got %v", filtered)
	}
	if errors.Is(filtered, discardErr) || errors.Is(filtered, ErrDiscard) {
		t.Fatalf("expected nested discard branch to be removed, got %v", filtered)
	}
}

func TestFilterDiscardErrorsReturnsNilForAllDiscardBranches(t *testing.T) {
	filtered := filterDiscardErrors(errors.Join(
		Discard(errors.New("first")),
		fmt.Errorf("wrapped: %w", Discard(errors.New("second"))),
	))
	if filtered != nil {
		t.Fatalf("expected all discard branches to be removed, got %v", filtered)
	}
}

func BenchmarkDispatcherSingleHandler(b *testing.B) {
	dispatcher := NewDispatcher()
	if err := On[*testOrderCreated](dispatcher, func(context.Context, *testOrderCreated) error {
		return nil
	}); err != nil {
		b.Fatal(err)
	}
	msg := &Message{EventType: "order.created", Payload: []byte(`{"id":"benchmark"}`)}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := dispatcher.Handle(context.Background(), msg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDispatcherHandlerErrors(b *testing.B) {
	dispatcher := NewDispatcher()
	firstErr := errors.New("first")
	secondErr := errors.New("second")
	if err := On[*testOrderCreated](dispatcher, func(context.Context, *testOrderCreated) error {
		return errors.Join(firstErr, secondErr)
	}); err != nil {
		b.Fatal(err)
	}
	msg := &Message{EventType: "order.created", Payload: []byte(`{"id":"benchmark"}`)}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := dispatcher.Handle(context.Background(), msg); !errors.Is(err, firstErr) ||
			!errors.Is(err, secondErr) {
			b.Fatal(err)
		}
	}
}

func BenchmarkDispatcherMiddleware(b *testing.B) {
	dispatcher := NewDispatcher()
	dispatcher.Use(
		MiddlewareFunc(func(ctx context.Context, eventCtx *EventContext, next Next) error {
			return next(ctx, eventCtx)
		}),
	)
	if err := On[*testOrderCreated](dispatcher, func(context.Context, *testOrderCreated) error {
		return nil
	}); err != nil {
		b.Fatal(err)
	}
	msg := &Message{EventType: "order.created", Payload: []byte(`{"id":"benchmark"}`)}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := dispatcher.Handle(context.Background(), msg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDispatcherDiscardAndBusinessErrors(b *testing.B) {
	dispatcher := NewDispatcher()
	discardErr := errors.New("invalid payload")
	businessErr := errors.New("backend unavailable")
	if err := On[*testOrderCreated](dispatcher, func(context.Context, *testOrderCreated) error {
		return errors.Join(Discard(discardErr), businessErr)
	}); err != nil {
		b.Fatal(err)
	}
	msg := &Message{EventType: "order.created", Payload: []byte(`{"id":"benchmark"}`)}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := dispatcher.Handle(context.Background(), msg); !errors.Is(err, businessErr) ||
			errors.Is(err, discardErr) {
			b.Fatal(err)
		}
	}
}

func TestDispatcherMiddlewareDoesNotWrapFallback(t *testing.T) {
	dispatcher := newTestDispatcher()
	middlewareCalled := false
	dispatcher.Use(
		MiddlewareFunc(func(ctx context.Context, eventCtx *EventContext, next Next) error {
			middlewareCalled = true
			return next(ctx, eventCtx)
		}),
	)
	fallbackCalled := false
	dispatcher.SetFallback(func(context.Context, *Message) error {
		fallbackCalled = true
		return nil
	})

	err := dispatcher.Handle(context.Background(), &Message{
		EventType: "unknown.event",
		Payload:   []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if middlewareCalled {
		t.Fatal("expected event middleware to bypass fallback")
	}
	if !fallbackCalled {
		t.Fatal("expected fallback handler to be called")
	}
}

func TestDispatcherMiddlewarePassesDecodedEventToHandler(t *testing.T) {
	dispatcher := newTestDispatcher()
	middlewareCalls := 0
	dispatcher.Use(
		MiddlewareFunc(func(ctx context.Context, eventCtx *EventContext, next Next) error {
			middlewareCalls++
			if middlewareCalls == 1 {
				eventCtx.Event.(*testOrderCreated).ID = "mutated-by-middleware"
			}
			return next(ctx, eventCtx)
		}),
	)

	var received *testOrderCreated
	if err := On[*testOrderCreated](
		dispatcher,
		func(_ context.Context, event *testOrderCreated) error {
			received = event
			return nil
		},
	); err != nil {
		t.Fatalf("On returned error: %v", err)
	}

	err := dispatcher.Handle(context.Background(), &Message{
		EventType: "order.created",
		Payload:   []byte(`{"id":"evt-independent"}`),
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if middlewareCalls != 1 {
		t.Fatalf("expected one middleware invocation per message, got %d", middlewareCalls)
	}
	if received == nil || received.EventID() != "mutated-by-middleware" {
		t.Fatalf("expected middleware mutation to reach handler, got %#v", received)
	}
}

func TestDispatcherConcurrentHandle(t *testing.T) {
	dispatcher := newTestDispatcher()
	dispatcher.Use(
		MiddlewareFunc(func(ctx context.Context, eventCtx *EventContext, next Next) error {
			return next(ctx, eventCtx)
		}),
	)
	if err := On[*testOrderCreated](dispatcher, func(context.Context, *testOrderCreated) error {
		return nil
	}); err != nil {
		t.Fatalf("On returned error: %v", err)
	}

	payload := []byte(`{"id":"evt-concurrent"}`)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				if err := dispatcher.Handle(context.Background(), &Message{
					EventType: "order.created",
					Payload:   payload,
				}); err != nil {
					t.Errorf("Handle returned error: %v", err)
				}
			}
		}()
	}
	wg.Wait()
}

type middlewareContextKey struct{}

func TestDispatcherHandleSingleHandlerError(t *testing.T) {
	dispatcher := newTestDispatcher()

	wantErr := errors.New("handler failed")

	err := On[*testOrderCreated](
		dispatcher,
		func(_ context.Context, event *testOrderCreated) error {
			if event.UserID != "u_2" {
				t.Fatalf("unexpected user ID: %q", event.UserID)
			}
			return wantErr
		},
	)
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
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected handler error, got %v", err)
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

	err = (*Dispatcher)(nil).Handle(context.Background(), &Message{EventType: "evt"})
	if !errors.Is(err, ErrInvalidEventBinding) {
		t.Fatalf("expected ErrInvalidEventBinding, got %v", err)
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

func TestDispatcherOnValidation(t *testing.T) {
	err := On[*testOrderCreated](nil, func(context.Context, *testOrderCreated) error { return nil })
	if !errors.Is(err, ErrInvalidEventBinding) {
		t.Fatalf("expected ErrInvalidEventBinding for nil dispatcher, got %v", err)
	}

	var handler func(context.Context, *testOrderCreated) error
	err = On[*testOrderCreated](newTestDispatcher(), handler)
	if !errors.Is(err, ErrInvalidEventBinding) {
		t.Fatalf("expected ErrInvalidEventBinding for nil handler, got %v", err)
	}

	err = On[*testEmptyTypeEvent](
		newTestDispatcher(),
		func(context.Context, *testEmptyTypeEvent) error {
			return nil
		},
	)
	if !errors.Is(err, ErrEventTypeRequired) {
		t.Fatalf("expected ErrEventTypeRequired, got %v", err)
	}
}

func TestInternalHelpers(t *testing.T) {
	if !isNilValue((*testOrderCreated)(nil)) {
		t.Fatal("expected typed nil pointer to be nil")
	}
	if !isNilValue([]byte(nil)) {
		t.Fatal("expected nil slice to be nil")
	}
	if isNilValue(testValueEvent{}) {
		t.Fatal("expected value event to be non-nil")
	}
	if cloneBytes(nil) != nil {
		t.Fatal("expected nil clone to stay nil")
	}

	src := []byte("payload")
	cloned := cloneBytes(src)
	src[0] = 'P'
	if string(cloned) != "payload" {
		t.Fatalf("expected cloned payload copy, got %q", cloned)
	}

	event := outboundEvent{payload: []byte("payload")}
	if err := event.UnmarshalPayload([]byte("updated")); err != nil {
		t.Fatalf("UnmarshalPayload returned error: %v", err)
	}

	payload, err := event.MarshalPayload()
	if err != nil {
		t.Fatalf("MarshalPayload returned error: %v", err)
	}
	payload[0] = 'P'

	again, err := event.MarshalPayload()
	if err != nil {
		t.Fatalf("MarshalPayload returned error: %v", err)
	}
	if string(again) != "updated" {
		t.Fatalf("expected MarshalPayload to clone bytes, got %q", again)
	}
}

func TestDispatcherOnSameTypeRejectsDuplicateHandler(t *testing.T) {
	dispatcher := newTestDispatcher()

	if err := On[*testOrderCreated](
		dispatcher,
		func(context.Context, *testOrderCreated) error { return nil },
	); err != nil {
		t.Fatalf("first On returned error: %v", err)
	}
	if err := On[*testOrderCreated](
		dispatcher,
		func(context.Context, *testOrderCreated) error { return nil },
	); !errors.Is(
		err,
		ErrEventHandlerConflict,
	) {
		t.Fatalf("expected ErrEventHandlerConflict, got %v", err)
	}

	if dispatcher.bindings["order.created"].handler == nil {
		t.Fatal("expected handler to remain registered")
	}
	if dispatcher.bindings["order.created"].newEvent == nil {
		t.Fatal("expected constructor to be registered")
	}
}

func TestDispatcherOnEventTypeConflict(t *testing.T) {
	dispatcher := newTestDispatcher()

	if err := On[*testOrderCreated](
		dispatcher,
		func(context.Context, *testOrderCreated) error { return nil },
	); err != nil {
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

type testTopicEvent struct {
	ID     string `json:"id"`
	Topic_ string `json:"-"`
}

func (*testTopicEvent) EventType() string      { return "topic.event" }
func (e *testTopicEvent) EventID() string      { return e.ID }
func (e *testTopicEvent) PartitionKey() string { return "" }
func (e *testTopicEvent) Topic() string        { return e.Topic_ }

func (e *testTopicEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(e)
}

func (e *testTopicEvent) UnmarshalPayload(data []byte) error {
	return json.Unmarshal(data, e)
}

func TestEncodeExtractsTopic(t *testing.T) {
	event := &testTopicEvent{ID: "evt_topic", Topic_: "custom-topic"}
	outbound, err := Encode(event)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	if outbound.Topic != "custom-topic" {
		t.Fatalf("expected topic %q, got %q", "custom-topic", outbound.Topic)
	}
}

func TestEncodeTopicEmptyByDefault(t *testing.T) {
	event := &testOrderCreated{ID: "evt_no_topic"}
	outbound, err := Encode(event)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	if outbound.Topic != "" {
		t.Fatalf("expected empty topic, got %q", outbound.Topic)
	}
}

func TestSenderFromPublisherPreservesTopic(t *testing.T) {
	publisher := &testPublisher{}
	sender := SenderFromPublisher(publisher)

	outbound := &Outbound{
		EventType: "topic.event",
		EventID:   "evt_topic",
		Payload:   []byte(`{}`),
		Topic:     "custom-topic",
	}
	if err := sender.Send(context.Background(), outbound); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	if publisher.last == nil {
		t.Fatal("expected publisher event")
	}

	reEncoded, err := Encode(publisher.last)
	if err != nil {
		t.Fatalf("Encode of published event returned error: %v", err)
	}
	if reEncoded.Topic != "custom-topic" {
		t.Fatalf("expected topic %q after round-trip, got %q", "custom-topic", reEncoded.Topic)
	}
}

func TestDispatcherFallbackHandler(t *testing.T) {
	dispatcher := newTestDispatcher()

	var gotMsg *Message
	dispatcher.SetFallback(func(_ context.Context, msg *Message) error {
		gotMsg = msg
		return nil
	})

	payload := []byte(`{"id":"fb_1"}`)
	err := dispatcher.Handle(context.Background(), &Message{
		EventType: "unknown.event",
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if gotMsg == nil {
		t.Fatal("expected fallback to receive message")
	}
	if gotMsg.EventType != "unknown.event" {
		t.Fatalf("expected event type %q, got %q", "unknown.event", gotMsg.EventType)
	}
}

func TestDispatcherFallbackHandlerError(t *testing.T) {
	dispatcher := newTestDispatcher()
	wantErr := errors.New("fallback failed")
	dispatcher.SetFallback(func(_ context.Context, msg *Message) error {
		return wantErr
	})

	err := dispatcher.Handle(context.Background(), &Message{
		EventType: "unknown.event",
		Payload:   []byte(`{}`),
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected fallback error, got %v", err)
	}
}

func TestDispatcherNoFallbackReturnsErrNoHandlers(t *testing.T) {
	dispatcher := newTestDispatcher()

	err := dispatcher.Handle(context.Background(), &Message{
		EventType: "unknown.event",
		Payload:   []byte(`{}`),
	})
	if !errors.Is(err, ErrNoHandlers) {
		t.Fatalf("expected ErrNoHandlers, got %v", err)
	}
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
