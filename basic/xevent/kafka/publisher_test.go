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

package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/IBM/sarama"

	"github.com/codesjoy/pkg/basic/xevent"
	"github.com/codesjoy/pkg/basic/xkafka"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
)

type testEvent struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

func (*testEvent) EventType() string { return "order.created" }
func (e *testEvent) EventID() string { return e.ID }
func (e *testEvent) PartitionKey() string {
	return e.Key
}
func (*testEvent) Topic() string { return "" }

func (e *testEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(e)
}

func (e *testEvent) UnmarshalPayload(data []byte) error {
	return json.Unmarshal(data, e)
}

type fakeProducer struct {
	mu    sync.Mutex
	last  *produce.Message
	msgs  []*produce.Message
	err   error
	errFn func(*produce.Message) error
}

func (f *fakeProducer) Produce(_ context.Context, msg *produce.Message) (*produce.Result, error) {
	f.mu.Lock()
	f.last = msg
	f.msgs = append(f.msgs, msg)
	errFn := f.errFn
	err := f.err
	f.mu.Unlock()

	if errFn != nil {
		if produceErr := errFn(msg); produceErr != nil {
			return nil, produceErr
		}
	}
	if err != nil {
		return nil, err
	}
	return &produce.Result{Topic: msg.Topic}, nil
}

type fakeBatchProducer struct {
	fakeProducer
	batchMsgs  []*produce.Message
	batchCalls int
}

func (f *fakeBatchProducer) ProduceBatch(
	_ context.Context,
	msgs ...*produce.Message,
) ([]*produce.Result, error) {
	f.mu.Lock()
	f.batchMsgs = append([]*produce.Message(nil), msgs...)
	f.batchCalls++
	err := f.err
	f.mu.Unlock()

	if err != nil {
		return nil, err
	}
	results := make([]*produce.Result, len(msgs))
	for i, msg := range msgs {
		results[i] = &produce.Result{Topic: msg.Topic}
	}
	return results, nil
}

func TestNewPublisherValidate(t *testing.T) {
	_, err := NewPublisher(PublisherConfig{})
	if !errors.Is(err, ErrNilProducer) {
		t.Fatalf("expected ErrNilProducer, got %v", err)
	}

	_, err = NewPublisher(PublisherConfig{Producer: &xkafka.Producer{}})
	if !errors.Is(err, ErrTopicRequired) {
		t.Fatalf("expected ErrTopicRequired, got %v", err)
	}
}

func TestPublisherPublishMapsEventToKafkaMessage(t *testing.T) {
	producer := &fakeProducer{}
	publisher := &Publisher{
		producer:        producer,
		topic:           "orders",
		eventTypeHeader: defaultEventTypeHeader,
		eventIDHeader:   defaultEventIDHeader,
	}

	err := publisher.Publish(context.Background(), &testEvent{
		ID:   "evt_1",
		Key:  "order-1",
		Name: "alice",
	})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if producer.last == nil {
		t.Fatal("expected produce message")
	}
	if producer.last.Topic != "orders" {
		t.Fatalf("unexpected topic: %q", producer.last.Topic)
	}
	if string(producer.last.Key) != "order-1" {
		t.Fatalf("unexpected key: %q", string(producer.last.Key))
	}
	if string(producer.last.Value) == "" {
		t.Fatal("expected payload")
	}
	if got := recordHeaderValue(producer.last.Headers, defaultEventTypeHeader); got != "order.created" {
		t.Fatalf("unexpected event type header: %q", got)
	}
	if got := recordHeaderValue(producer.last.Headers, defaultEventIDHeader); got != "evt_1" {
		t.Fatalf("unexpected event id header: %q", got)
	}
}

func TestPublisherSendMapsOutboundToKafkaMessage(t *testing.T) {
	producer := &fakeProducer{}
	publisher := &Publisher{
		producer:        producer,
		topic:           "orders",
		eventTypeHeader: defaultEventTypeHeader,
		eventIDHeader:   defaultEventIDHeader,
	}

	err := publisher.Send(context.Background(), &xevent.Outbound{
		EventType:    "order.created",
		EventID:      "evt_2",
		PartitionKey: "order-2",
		Payload:      []byte(`{"id":"evt_2"}`),
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if producer.last == nil {
		t.Fatal("expected produce message")
	}
	if producer.last.Topic != "orders" {
		t.Fatalf("unexpected topic: %q", producer.last.Topic)
	}
	if string(producer.last.Key) != "order-2" {
		t.Fatalf("unexpected key: %q", string(producer.last.Key))
	}
	if got := recordHeaderValue(producer.last.Headers, defaultEventTypeHeader); got != "order.created" {
		t.Fatalf("unexpected event type header: %q", got)
	}
	if got := recordHeaderValue(producer.last.Headers, defaultEventIDHeader); got != "evt_2" {
		t.Fatalf("unexpected event id header: %q", got)
	}
}

func TestPublisherPublishWithoutPartitionKeyOrEventID(t *testing.T) {
	producer := &fakeProducer{}
	publisher := &Publisher{
		producer:        producer,
		topic:           "orders",
		eventTypeHeader: defaultEventTypeHeader,
		eventIDHeader:   defaultEventIDHeader,
	}

	err := publisher.Publish(context.Background(), &testEvent{Name: "alice"})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if producer.last == nil {
		t.Fatal("expected produce message")
	}
	if len(producer.last.Key) != 0 {
		t.Fatalf("expected empty key, got %q", string(producer.last.Key))
	}
	if got := recordHeaderValue(producer.last.Headers, defaultEventIDHeader); got != "" {
		t.Fatalf("expected empty event id header, got %q", got)
	}
}

func TestPublisherPublishNilEventAndErrors(t *testing.T) {
	producer := &fakeProducer{}
	publisher := &Publisher{
		producer:        producer,
		topic:           "orders",
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

	producer.err = errors.New("produce failed")
	err = publisher.Publish(context.Background(), &testEvent{Name: "alice"})
	if err == nil || err.Error() != "produce failed" {
		t.Fatalf("expected produce error, got %v", err)
	}
}

func TestPublisherSendUsesOutboundTopicOverride(t *testing.T) {
	producer := &fakeProducer{}
	publisher := &Publisher{
		producer:        producer,
		topic:           "default-topic",
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
	if producer.last.Topic != "custom-orders" {
		t.Fatalf("expected topic %q, got %q", "custom-orders", producer.last.Topic)
	}
}

func TestPublisherSendFallsBackToDefaultTopic(t *testing.T) {
	producer := &fakeProducer{}
	publisher := &Publisher{
		producer:        producer,
		topic:           "default-topic",
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
	if producer.last.Topic != "default-topic" {
		t.Fatalf("expected default topic %q, got %q", "default-topic", producer.last.Topic)
	}
}

func TestRecordHeaderValueFindsKafkaHeader(t *testing.T) {
	headers := []sarama.RecordHeader{
		{Key: []byte("a"), Value: []byte("1")},
		{Key: []byte("b"), Value: []byte("2")},
	}
	if got := recordHeaderValue(headers, "b"); got != "2" {
		t.Fatalf("unexpected header value: %q", got)
	}
}

func recordHeaderValue(headers []sarama.RecordHeader, key string) string {
	for _, header := range headers {
		if string(header.Key) == key {
			return string(header.Value)
		}
	}
	return ""
}

func TestPublisherBatchSendUsesPerItemProduceWhenBatchAPIExists(t *testing.T) {
	producer := &fakeBatchProducer{}
	publisher := &Publisher{
		producer:        producer,
		topic:           "orders",
		eventTypeHeader: defaultEventTypeHeader,
		eventIDHeader:   defaultEventIDHeader,
	}

	outbounds := []*xevent.Outbound{
		{
			EventType:    "order.created",
			EventID:      "evt_1",
			PartitionKey: "k1",
			Payload:      []byte(`{"a":1}`),
		},
		{
			EventType:    "order.updated",
			EventID:      "evt_2",
			PartitionKey: "k2",
			Payload:      []byte(`{"a":2}`),
		},
	}

	errs := publisher.BatchSend(context.Background(), outbounds)
	for i, err := range errs {
		if err != nil {
			t.Fatalf("errs[%d] unexpected error: %v", i, err)
		}
	}

	producer.mu.Lock()
	defer producer.mu.Unlock()

	if producer.batchCalls != 0 {
		t.Fatalf("expected ProduceBatch to be unused, got %d calls", producer.batchCalls)
	}
	if len(producer.msgs) != 2 {
		t.Fatalf("expected 2 produce messages, got %d", len(producer.msgs))
	}
	first := findProducedMessageByEventID(producer.msgs, defaultEventIDHeader, "evt_1")
	second := findProducedMessageByEventID(producer.msgs, defaultEventIDHeader, "evt_2")
	if first == nil || second == nil {
		t.Fatalf("expected both messages to be produced, got %#v", producer.msgs)
	}
	if first.Topic != "orders" {
		t.Fatalf("expected topic %q, got %q", "orders", first.Topic)
	}
	if string(first.Key) != "k1" {
		t.Fatalf("expected key %q, got %q", "k1", string(first.Key))
	}
	if got := recordHeaderValue(second.Headers, defaultEventTypeHeader); got != "order.updated" {
		t.Fatalf("unexpected event type header: %q", got)
	}
}

func TestPublisherBatchSendEmptyInput(t *testing.T) {
	publisher := &Publisher{
		producer:        &fakeBatchProducer{},
		topic:           "orders",
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

func TestPublisherBatchSendUsesPerItemProduce(t *testing.T) {
	producer := &fakeProducer{}
	publisher := &Publisher{
		producer:        producer,
		topic:           "orders",
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

	producer.mu.Lock()
	defer producer.mu.Unlock()

	if producer.last == nil {
		t.Fatal("expected at least one produce message")
	}
	if len(producer.msgs) != 2 {
		t.Fatalf("expected 2 produce calls, got %d", len(producer.msgs))
	}
}

func TestPublisherBatchSendReturnsErrorsPerOutbound(t *testing.T) {
	producer := &fakeBatchProducer{
		fakeProducer: fakeProducer{
			errFn: func(msg *produce.Message) error {
				if got := recordHeaderValue(msg.Headers, defaultEventIDHeader); got == "evt_2" {
					return errors.New("produce failed")
				}
				return nil
			},
		},
	}
	publisher := &Publisher{
		producer:        producer,
		topic:           "orders",
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
	if errs[1] == nil || errs[1].Error() != "produce failed" {
		t.Fatalf("errs[1] expected produce failed, got %v", errs[1])
	}
}

func TestPublisherBatchSendValidatesOutbounds(t *testing.T) {
	producer := &fakeBatchProducer{}
	publisher := &Publisher{
		producer:        producer,
		topic:           "orders",
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

func findProducedMessageByEventID(
	msgs []*produce.Message,
	header string,
	eventID string,
) *produce.Message {
	for _, msg := range msgs {
		if recordHeaderValue(msg.Headers, header) == eventID {
			return msg
		}
	}
	return nil
}
