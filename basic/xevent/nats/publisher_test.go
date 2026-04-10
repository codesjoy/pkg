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
	"sync"
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
func (*testEvent) Topic() string                     { return "" }
func (e *testEvent) MarshalPayload() ([]byte, error) { return json.Marshal(e) }
func (e *testEvent) UnmarshalPayload(data []byte) error {
	return json.Unmarshal(data, e)
}

type testMarshalErrorEvent struct{}

func (*testMarshalErrorEvent) EventType() string    { return "order.created" }
func (*testMarshalErrorEvent) EventID() string      { return "" }
func (*testMarshalErrorEvent) PartitionKey() string { return "" }
func (*testMarshalErrorEvent) Topic() string        { return "" }
func (*testMarshalErrorEvent) MarshalPayload() ([]byte, error) {
	return nil, errors.New("marshal failed")
}
func (*testMarshalErrorEvent) UnmarshalPayload([]byte) error { return nil }

type fakePublisher struct {
	mu    sync.Mutex
	last  *publish.Message
	msgs  []*publish.Message
	err   error
	errFn func(*publish.Message) error
}

func (f *fakePublisher) Publish(_ context.Context, msg *publish.Message) (*publish.Result, error) {
	f.mu.Lock()
	f.last = msg
	f.msgs = append(f.msgs, msg)
	errFn := f.errFn
	err := f.err
	f.mu.Unlock()

	if errFn != nil {
		if publishErr := errFn(msg); publishErr != nil {
			return nil, publishErr
		}
	}
	if err != nil {
		return nil, err
	}
	return &publish.Result{Subject: msg.Subject}, nil
}

type fakeBatchPublisher struct {
	fakePublisher
	batchMsgs  []*publish.Message
	batchCalls int
}

func (f *fakeBatchPublisher) PublishBatch(_ context.Context, msgs ...*publish.Message) ([]*publish.Result, error) {
	f.mu.Lock()
	f.batchMsgs = append([]*publish.Message(nil), msgs...)
	f.batchCalls++
	err := f.err
	f.mu.Unlock()

	if err != nil {
		return nil, err
	}
	results := make([]*publish.Result, len(msgs))
	for i, msg := range msgs {
		results[i] = &publish.Result{Subject: msg.Subject}
	}
	return results, nil
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

func TestPublisherSendUsesOutboundTopicAsSubject(t *testing.T) {
	publisherImpl := &fakePublisher{}
	publisher := &Publisher{
		publisher:       publisherImpl,
		eventTypeHeader: defaultEventTypeHeader,
		eventIDHeader:   defaultEventIDHeader,
	}

	err := publisher.Send(context.Background(), &xevent.Outbound{
		EventType: "order.created",
		EventID:   "evt_3",
		Payload:   []byte(`{}`),
		Topic:     "custom-orders",
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if publisherImpl.last.Subject != "custom-orders" {
		t.Fatalf("expected subject %q, got %q", "custom-orders", publisherImpl.last.Subject)
	}
}

func TestPublisherSendFallsBackToEventTypeAsSubject(t *testing.T) {
	publisherImpl := &fakePublisher{}
	publisher := &Publisher{
		publisher:       publisherImpl,
		eventTypeHeader: defaultEventTypeHeader,
		eventIDHeader:   defaultEventIDHeader,
	}

	err := publisher.Send(context.Background(), &xevent.Outbound{
		EventType: "order.created",
		EventID:   "evt_4",
		Payload:   []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if publisherImpl.last.Subject != "order.created" {
		t.Fatalf("expected subject %q, got %q", "order.created", publisherImpl.last.Subject)
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

func TestPublisherBatchSendUsesPerItemPublishWhenBatchAPIExists(t *testing.T) {
	publisherImpl := &fakeBatchPublisher{}
	publisher := &Publisher{
		publisher:       publisherImpl,
		eventTypeHeader: defaultEventTypeHeader,
		eventIDHeader:   defaultEventIDHeader,
	}

	outbounds := []*xevent.Outbound{
		{EventType: "order.created", EventID: "evt_1", Payload: []byte(`{"a":1}`)},
		{EventType: "order.updated", EventID: "evt_2", Payload: []byte(`{"a":2}`)},
	}

	errs := publisher.BatchSend(context.Background(), outbounds)
	for i, err := range errs {
		if err != nil {
			t.Fatalf("errs[%d] unexpected error: %v", i, err)
		}
	}

	publisherImpl.mu.Lock()
	defer publisherImpl.mu.Unlock()

	if publisherImpl.batchCalls != 0 {
		t.Fatalf("expected PublishBatch to be unused, got %d calls", publisherImpl.batchCalls)
	}
	if len(publisherImpl.msgs) != 2 {
		t.Fatalf("expected 2 publish messages, got %d", len(publisherImpl.msgs))
	}
	first := findPublishedMessageByEventID(publisherImpl.msgs, defaultEventIDHeader, "evt_1")
	second := findPublishedMessageByEventID(publisherImpl.msgs, defaultEventIDHeader, "evt_2")
	if first == nil || second == nil {
		t.Fatalf("expected both messages to be published, got %#v", publisherImpl.msgs)
	}
	if first.Subject != "order.created" {
		t.Fatalf("expected subject %q, got %q", "order.created", first.Subject)
	}
	if got := second.Header.Get(defaultEventTypeHeader); got != "order.updated" {
		t.Fatalf("unexpected event type header: %q", got)
	}
}

func TestPublisherBatchSendEmptyInput(t *testing.T) {
	publisher := &Publisher{
		publisher:       &fakeBatchPublisher{},
		eventTypeHeader: defaultEventTypeHeader,
		eventIDHeader:   defaultEventIDHeader,
	}

	errs := publisher.BatchSend(context.Background(), nil)
	if errs != nil {
		t.Fatalf("expected nil, got %v", errs)
	}

	errs = publisher.BatchSend(context.Background(), []*xevent.Outbound{})
	if errs != nil {
		t.Fatalf("expected nil, got %v", errs)
	}
}

func TestPublisherBatchSendUsesPerItemPublish(t *testing.T) {
	publisherImpl := &fakePublisher{}
	publisher := &Publisher{
		publisher:       publisherImpl,
		eventTypeHeader: defaultEventTypeHeader,
		eventIDHeader:   defaultEventIDHeader,
	}

	outbounds := []*xevent.Outbound{
		{EventType: "order.created", EventID: "evt_1", Payload: []byte(`1`)},
		{EventType: "order.updated", EventID: "evt_2", Payload: []byte(`2`)},
	}

	errs := publisher.BatchSend(context.Background(), outbounds)
	for i, err := range errs {
		if err != nil {
			t.Fatalf("errs[%d] unexpected error: %v", i, err)
		}
	}

	publisherImpl.mu.Lock()
	defer publisherImpl.mu.Unlock()

	if len(publisherImpl.msgs) != 2 {
		t.Fatalf("expected 2 publish calls, got %d", len(publisherImpl.msgs))
	}
}

func TestPublisherBatchSendReturnsErrorsPerOutbound(t *testing.T) {
	publisherImpl := &fakeBatchPublisher{
		fakePublisher: fakePublisher{
			errFn: func(msg *publish.Message) error {
				if msg.Header.Get(defaultEventIDHeader) == "evt_2" {
					return errors.New("publish failed")
				}
				return nil
			},
		},
	}
	publisher := &Publisher{
		publisher:       publisherImpl,
		eventTypeHeader: defaultEventTypeHeader,
		eventIDHeader:   defaultEventIDHeader,
	}

	outbounds := []*xevent.Outbound{
		{EventType: "order.created", EventID: "evt_1", Payload: []byte(`1`)},
		{EventType: "order.updated", EventID: "evt_2", Payload: []byte(`2`)},
	}

	errs := publisher.BatchSend(context.Background(), outbounds)
	if errs[0] != nil {
		t.Fatalf("errs[0] expected nil, got %v", errs[0])
	}
	if errs[1] == nil || errs[1].Error() != "publish failed" {
		t.Fatalf("errs[1] expected publish failed, got %v", errs[1])
	}
}

func TestPublisherBatchSendValidatesOutbounds(t *testing.T) {
	publisherImpl := &fakeBatchPublisher{}
	publisher := &Publisher{
		publisher:       publisherImpl,
		eventTypeHeader: defaultEventTypeHeader,
		eventIDHeader:   defaultEventIDHeader,
	}

	outbounds := []*xevent.Outbound{
		{EventType: "order.created", EventID: "evt_1", Payload: []byte(`1`)},
		nil,
		{EventType: "", EventID: "evt_3", Payload: []byte(`3`)},
	}

	errs := publisher.BatchSend(context.Background(), outbounds)
	if errs[0] != nil {
		t.Fatalf("errs[0] expected nil, got %v", errs[0])
	}
	if !errors.Is(errs[1], xevent.ErrNilOutbound) {
		t.Fatalf("errs[1] expected ErrNilOutbound, got %v", errs[1])
	}
	if !errors.Is(errs[2], xevent.ErrEventTypeRequired) {
		t.Fatalf("errs[2] expected ErrEventTypeRequired, got %v", errs[2])
	}
}

func findPublishedMessageByEventID(
	msgs []*publish.Message,
	header string,
	eventID string,
) *publish.Message {
	for _, msg := range msgs {
		if msg.Header.Get(header) == eventID {
			return msg
		}
	}
	return nil
}
