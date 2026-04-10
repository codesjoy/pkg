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
	"sync"

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
	outbound, err := xevent.Encode(event)
	if err != nil {
		return err
	}
	return p.Send(ctx, outbound)
}

// Send publishes one xevent.Outbound to Kafka.
func (p *Publisher) Send(ctx context.Context, outbound *xevent.Outbound) error {
	msg, err := p.buildMessage(outbound)
	if err != nil {
		return err
	}
	_, err = p.producer.Produce(ctx, msg)
	return err
}

// BatchSend sends multiple xevent.Outbound payloads to Kafka in one batch.
// It returns a slice of errors, one per outbound; nil means success.
// An empty input returns nil.
func (p *Publisher) BatchSend(ctx context.Context, outbounds []*xevent.Outbound) []error {
	if len(outbounds) == 0 {
		return nil
	}

	errs := make([]error, len(outbounds))

	var wg sync.WaitGroup
	for i, outbound := range outbounds {
		msg, err := p.buildMessage(outbound)
		if err != nil {
			errs[i] = err
			continue
		}

		wg.Add(1)
		go func(index int, prepared *produce.Message) {
			defer wg.Done()
			_, errs[index] = p.producer.Produce(ctx, prepared)
		}(i, msg)
	}

	wg.Wait()
	return errs
}

func (p *Publisher) buildMessage(outbound *xevent.Outbound) (*produce.Message, error) {
	if p == nil || p.producer == nil {
		return nil, ErrNilProducer
	}
	if outbound == nil {
		return nil, xevent.ErrNilOutbound
	}
	if outbound.EventType == "" {
		return nil, xevent.ErrEventTypeRequired
	}

	topic := p.topic
	if outbound.Topic != "" {
		topic = outbound.Topic
	}

	msg := &produce.Message{
		Topic: topic,
		Value: cloneBytes(outbound.Payload),
		Headers: []sarama.RecordHeader{
			{Key: []byte(p.eventTypeHeader), Value: []byte(outbound.EventType)},
		},
	}
	if partitionKey := outbound.PartitionKey; partitionKey != "" {
		msg.Key = []byte(partitionKey)
	}
	if eventID := outbound.EventID; eventID != "" {
		msg.Headers = append(msg.Headers, sarama.RecordHeader{
			Key:   []byte(p.eventIDHeader),
			Value: []byte(eventID),
		})
	}
	return msg, nil
}
