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
	"strings"

	"github.com/IBM/sarama"

	"github.com/codesjoy/pkg/basic/xevent"
	"github.com/codesjoy/pkg/basic/xkafka"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
)

type producerAPI interface {
	Produce(context.Context, *produce.Message) (*produce.Result, error)
}

// PublisherConfig configures the Kafka-backed xevent publisher.
type PublisherConfig struct {
	Producer        *xkafka.Producer
	Topic           string
	EventTypeHeader string
	EventIDHeader   string
}

// Publisher adapts xevent.Publisher onto xkafka.Producer.
type Publisher struct {
	producer        producerAPI
	topic           string
	eventTypeHeader string
	eventIDHeader   string
}

// NewPublisher creates a Kafka-backed xevent publisher.
func NewPublisher(cfg PublisherConfig) (*Publisher, error) {
	if cfg.Producer == nil {
		return nil, ErrNilProducer
	}

	topic := strings.TrimSpace(cfg.Topic)
	if topic == "" {
		return nil, ErrTopicRequired
	}

	return &Publisher{
		producer:        cfg.Producer,
		topic:           topic,
		eventTypeHeader: normalizeHeaderName(cfg.EventTypeHeader, defaultEventTypeHeader),
		eventIDHeader:   normalizeHeaderName(cfg.EventIDHeader, defaultEventIDHeader),
	}, nil
}

// Publish publishes one xevent.Event to Kafka.
func (p *Publisher) Publish(ctx context.Context, event xevent.Event) error {
	if p == nil || p.producer == nil {
		return ErrNilProducer
	}
	if event == nil {
		return xevent.ErrNilEvent
	}

	payload, err := event.MarshalPayload()
	if err != nil {
		return err
	}

	msg := &produce.Message{
		Topic: p.topic,
		Value: cloneBytes(payload),
		Headers: []sarama.RecordHeader{
			{Key: []byte(p.eventTypeHeader), Value: []byte(event.EventType())},
		},
	}
	if partitionKey := event.PartitionKey(); partitionKey != "" {
		msg.Key = []byte(partitionKey)
	}
	if eventID := event.EventID(); eventID != "" {
		msg.Headers = append(msg.Headers, sarama.RecordHeader{
			Key:   []byte(p.eventIDHeader),
			Value: []byte(eventID),
		})
	}

	_, err = p.producer.Produce(ctx, msg)
	return err
}
