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
	last *produce.Message
	err  error
}

func (f *fakeProducer) Produce(_ context.Context, msg *produce.Message) (*produce.Result, error) {
	f.last = msg
	if f.err != nil {
		return nil, f.err
	}
	return &produce.Result{Topic: msg.Topic}, nil
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
