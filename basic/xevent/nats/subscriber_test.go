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

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	natsio "github.com/nats-io/nats.go"

	"github.com/codesjoy/pkg/basic/xevent"
	"github.com/codesjoy/pkg/basic/xnats"
	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
)

type fakeConsumer struct {
	msg      *consume.MessageContext
	err      error
	closeErr error
	closed   int
	calls    int
}

func (f *fakeConsumer) Consume(ctx context.Context, handler consume.HandlerFunc) error {
	f.calls++
	if f.err != nil {
		return f.err
	}
	return handler(ctx, f.msg)
}

func (f *fakeConsumer) Close() error {
	f.closed++
	return f.closeErr
}

type testOrderCreated struct {
	ID string `json:"id"`
}

func (*testOrderCreated) EventType() string    { return "order.created" }
func (e *testOrderCreated) EventID() string    { return e.ID }
func (*testOrderCreated) PartitionKey() string { return "" }
func (e *testOrderCreated) MarshalPayload() ([]byte, error) {
	return json.Marshal(e)
}
func (e *testOrderCreated) UnmarshalPayload(data []byte) error {
	if err := json.Unmarshal(data, e); err != nil {
		return err
	}
	if len(data) > 0 {
		data[0] = '['
	}
	return nil
}

func TestNewSubscriberValidate(t *testing.T) {
	_, err := NewSubscriber(SubscriberConfig{})
	if !errors.Is(err, ErrNilConsumer) {
		t.Fatalf("expected ErrNilConsumer, got %v", err)
	}

	_, err = NewSubscriber(SubscriberConfig{Consumer: &xnats.JetStreamConsumer{}})
	if !errors.Is(err, ErrNilDispatcher) {
		t.Fatalf("expected ErrNilDispatcher, got %v", err)
	}
}

func TestSubscriberSubscribeDispatchesJetStreamMessage(t *testing.T) {
	payload := []byte(`{"id":"evt_1"}`)
	header := natsio.Header{}
	header.Add(defaultEventTypeHeader, "order.created")
	consumer := &fakeConsumer{
		msg: &consume.MessageContext{
			Subject: "ignored.subject",
			Message: &natsio.Msg{
				Subject: "ignored.subject",
				Data:    payload,
				Header:  header,
			},
		},
	}

	dispatcher := xevent.NewDispatcher()
	var got *testOrderCreated
	if err := xevent.On[*testOrderCreated](dispatcher, func(_ context.Context, event *testOrderCreated) error {
		got = event
		return nil
	}); err != nil {
		t.Fatalf("On returned error: %v", err)
	}

	subscriber := &Subscriber{
		consumer:        consumer,
		dispatcher:      dispatcher,
		eventTypeHeader: defaultEventTypeHeader,
	}

	err := subscriber.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	if got == nil {
		t.Fatal("expected event")
	}
	if got.ID != "evt_1" {
		t.Fatalf("unexpected event id: %q", got.ID)
	}
	if string(payload) != `{"id":"evt_1"}` {
		t.Fatalf("payload should be copied before delivery, got %q", string(payload))
	}
}

func TestSubscriberSubscribeFallsBackToSubject(t *testing.T) {
	consumer := &fakeConsumer{
		msg: &consume.MessageContext{
			Subject: "order.created",
			Message: &natsio.Msg{
				Subject: "order.created",
				Data:    []byte(`{"id":"evt_2"}`),
			},
		},
	}

	dispatcher := xevent.NewDispatcher()
	var got *testOrderCreated
	if err := xevent.On[*testOrderCreated](dispatcher, func(_ context.Context, event *testOrderCreated) error {
		got = event
		return nil
	}); err != nil {
		t.Fatalf("On returned error: %v", err)
	}

	subscriber := &Subscriber{
		consumer:        consumer,
		dispatcher:      dispatcher,
		eventTypeHeader: defaultEventTypeHeader,
	}

	err := subscriber.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	if got == nil || got.ID != "evt_2" {
		t.Fatalf("unexpected event: %+v", got)
	}
}

func TestSubscriberSubscribeValidationAndErrors(t *testing.T) {
	subscriber := &Subscriber{}

	err := subscriber.Subscribe(context.Background())
	if !errors.Is(err, ErrNilConsumer) {
		t.Fatalf("expected ErrNilConsumer, got %v", err)
	}

	subscriber = &Subscriber{consumer: &fakeConsumer{}, eventTypeHeader: defaultEventTypeHeader}
	err = subscriber.Subscribe(context.Background())
	if !errors.Is(err, ErrNilDispatcher) {
		t.Fatalf("expected ErrNilDispatcher, got %v", err)
	}

	subscriber = &Subscriber{
		consumer: &fakeConsumer{
			msg: &consume.MessageContext{
				Message: &natsio.Msg{},
			},
		},
		dispatcher:      xevent.NewDispatcher(),
		eventTypeHeader: defaultEventTypeHeader,
	}
	err = subscriber.Subscribe(context.Background())
	if !errors.Is(err, xevent.ErrEventTypeRequired) {
		t.Fatalf("expected xevent.ErrEventTypeRequired, got %v", err)
	}

	consumeErr := errors.New("consume failed")
	subscriber = &Subscriber{
		consumer:        &fakeConsumer{err: consumeErr},
		dispatcher:      xevent.NewDispatcher(),
		eventTypeHeader: defaultEventTypeHeader,
	}
	err = subscriber.Subscribe(context.Background())
	if !errors.Is(err, consumeErr) {
		t.Fatalf("expected consume error, got %v", err)
	}
}

func TestSubscriberSubscribeIsOneShot(t *testing.T) {
	dispatcher := xevent.NewDispatcher()
	if err := xevent.On[*testOrderCreated](dispatcher, func(context.Context, *testOrderCreated) error { return nil }); err != nil {
		t.Fatalf("On returned error: %v", err)
	}

	header := natsio.Header{}
	header.Add(defaultEventTypeHeader, "order.created")
	subscriber := &Subscriber{
		consumer: &fakeConsumer{
			msg: &consume.MessageContext{
				Message: &natsio.Msg{
					Subject: "order.created",
					Data:    []byte(`{"id":"evt_3"}`),
					Header:  header,
				},
			},
		},
		dispatcher:      dispatcher,
		eventTypeHeader: defaultEventTypeHeader,
	}

	if err := subscriber.Subscribe(context.Background()); err != nil {
		t.Fatalf("first Subscribe returned error: %v", err)
	}

	err := subscriber.Subscribe(context.Background())
	if !errors.Is(err, xevent.ErrSubscriberStarted) {
		t.Fatalf("expected ErrSubscriberStarted, got %v", err)
	}
}

func TestSubscriberClose(t *testing.T) {
	closeErr := errors.New("close failed")
	consumer := &fakeConsumer{closeErr: closeErr}
	subscriber := &Subscriber{
		consumer:   consumer,
		dispatcher: xevent.NewDispatcher(),
	}

	err := subscriber.Close()
	if !errors.Is(err, closeErr) {
		t.Fatalf("expected close error, got %v", err)
	}
	if consumer.closed != 1 {
		t.Fatalf("close count = %d, want 1", consumer.closed)
	}
	if err := subscriber.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("second close error = %v, want %v", err, closeErr)
	}
	if consumer.closed != 1 {
		t.Fatalf("close count after second close = %d, want 1", consumer.closed)
	}

	var nilSubscriber *Subscriber
	if err := nilSubscriber.Close(); err != nil {
		t.Fatalf("nil subscriber close returned error: %v", err)
	}
}

func TestSubscriberCloseBeforeSubscribePreventsStart(t *testing.T) {
	consumer := &fakeConsumer{}
	subscriber := &Subscriber{
		consumer:   consumer,
		dispatcher: xevent.NewDispatcher(),
	}

	if err := subscriber.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if consumer.closed != 1 {
		t.Fatalf("close count = %d, want 1", consumer.closed)
	}

	err := subscriber.Subscribe(context.Background())
	if !errors.Is(err, xevent.ErrSubscriberClosed) {
		t.Fatalf("expected ErrSubscriberClosed, got %v", err)
	}
	if consumer.calls != 0 {
		t.Fatalf("consume call count = %d, want 0", consumer.calls)
	}
}
