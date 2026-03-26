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
	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
)

type testEvent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (*testEvent) EventType() string                 { return "order.created" }
func (e *testEvent) EventID() string                 { return e.ID }
func (*testEvent) PartitionKey() string              { return "" }
func (e *testEvent) MarshalPayload() ([]byte, error) { return json.Marshal(e) }
func (e *testEvent) UnmarshalPayload(data []byte) error {
	return json.Unmarshal(data, e)
}

type testMarshalErrorEvent struct{}

func (*testMarshalErrorEvent) EventType() string    { return "order.created" }
func (*testMarshalErrorEvent) EventID() string      { return "" }
func (*testMarshalErrorEvent) PartitionKey() string { return "" }
func (*testMarshalErrorEvent) MarshalPayload() ([]byte, error) {
	return nil, errors.New("marshal failed")
}
func (*testMarshalErrorEvent) UnmarshalPayload([]byte) error { return nil }

type fakePublisher struct {
	last *publish.Message
	err  error
}

func (f *fakePublisher) Publish(_ context.Context, msg *publish.Message) (*publish.Result, error) {
	f.last = msg
	if f.err != nil {
		return nil, f.err
	}
	return &publish.Result{Subject: msg.Subject}, nil
}

func TestNewPublisherValidate(t *testing.T) {
	_, err := NewPublisher(PublisherConfig{})
	if !errors.Is(err, ErrNilPublisher) {
		t.Fatalf("expected ErrNilPublisher, got %v", err)
	}

	_, err = NewPublisher(PublisherConfig{Publisher: &xnats.JetStreamPublisher{}})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestPublisherPublishMapsEventToJetStreamMessage(t *testing.T) {
	publisherImpl := &fakePublisher{}
	publisher := &Publisher{
		publisher:       publisherImpl,
		eventTypeHeader: defaultEventTypeHeader,
		eventIDHeader:   defaultEventIDHeader,
	}

	err := publisher.Publish(context.Background(), &testEvent{
		ID:   "evt_1",
		Name: "alice",
	})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if publisherImpl.last == nil {
		t.Fatal("expected publish message")
	}
	if publisherImpl.last.Subject != "order.created" {
		t.Fatalf("unexpected subject: %q", publisherImpl.last.Subject)
	}
	if string(publisherImpl.last.Data) == "" {
		t.Fatal("expected payload")
	}
	if got := publisherImpl.last.Header.Get(defaultEventTypeHeader); got != "order.created" {
		t.Fatalf("unexpected event type header: %q", got)
	}
	if got := publisherImpl.last.Header.Get(defaultEventIDHeader); got != "evt_1" {
		t.Fatalf("unexpected event id header: %q", got)
	}
}

func TestPublisherSendMapsOutboundToJetStreamMessage(t *testing.T) {
	publisherImpl := &fakePublisher{}
	publisher := &Publisher{
		publisher:       publisherImpl,
		eventTypeHeader: defaultEventTypeHeader,
		eventIDHeader:   defaultEventIDHeader,
	}

	err := publisher.Send(context.Background(), &xevent.Outbound{
		EventType: "order.created",
		EventID:   "evt_2",
		Payload:   []byte(`{"id":"evt_2"}`),
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if publisherImpl.last == nil {
		t.Fatal("expected publish message")
	}
	if publisherImpl.last.Subject != "order.created" {
		t.Fatalf("unexpected subject: %q", publisherImpl.last.Subject)
	}
	if got := publisherImpl.last.Header.Get(defaultEventTypeHeader); got != "order.created" {
		t.Fatalf("unexpected event type header: %q", got)
	}
	if got := publisherImpl.last.Header.Get(defaultEventIDHeader); got != "evt_2" {
		t.Fatalf("unexpected event id header: %q", got)
	}
}

func TestPublisherPublishWithoutEventIDAndWithCopiedPayload(t *testing.T) {
	publisherImpl := &fakePublisher{}
	publisher := &Publisher{
		publisher:       publisherImpl,
		eventTypeHeader: defaultEventTypeHeader,
		eventIDHeader:   defaultEventIDHeader,
	}

	event := &testEvent{Name: "alice"}
	err := publisher.Publish(context.Background(), event)
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	payload, err := event.MarshalPayload()
	if err != nil {
		t.Fatalf("MarshalPayload returned error: %v", err)
	}
	payload[0] = '['
	if string(publisherImpl.last.Data) == string(payload) {
		t.Fatal("expected publish payload to be copied")
	}
	if got := publisherImpl.last.Header.Get(defaultEventIDHeader); got != "" {
		t.Fatalf("expected empty event id header, got %q", got)
	}
}

func TestPublisherPublishNilEventAndErrors(t *testing.T) {
	publisherImpl := &fakePublisher{}
	publisher := &Publisher{
		publisher:       publisherImpl,
		eventTypeHeader: defaultEventTypeHeader,
		eventIDHeader:   defaultEventIDHeader,
	}

	err := publisher.Publish(context.Background(), nil)
	if !errors.Is(err, xevent.ErrNilEvent) {
		t.Fatalf("expected xevent.ErrNilEvent, got %v", err)
	}

	err = publisher.Send(context.Background(), nil)
	if !errors.Is(err, xevent.ErrNilOutbound) {
		t.Fatalf("expected xevent.ErrNilOutbound, got %v", err)
	}

	err = publisher.Publish(context.Background(), &testMarshalErrorEvent{})
	if err == nil || err.Error() != "marshal failed" {
		t.Fatalf("expected marshal error, got %v", err)
	}

	publisherImpl.err = errors.New("publish failed")
	err = publisher.Publish(context.Background(), &testEvent{Name: "alice"})
	if err == nil || err.Error() != "publish failed" {
		t.Fatalf("expected publish error, got %v", err)
	}
}

func TestCloneHeaderCopiesValues(t *testing.T) {
	header := natsio.Header{}
	header.Add("x-test", "1")

	cloned := cloneHeader(header)
	cloned.Add("x-test", "2")

	if got := header.Values("x-test"); len(got) != 1 || got[0] != "1" {
		t.Fatalf("unexpected original header values: %v", got)
	}
}
