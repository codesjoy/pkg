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

	natsio "github.com/nats-io/nats.go"

	"github.com/codesjoy/pkg/basic/xevent"
	"github.com/codesjoy/pkg/basic/xnats"
	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
)

type publisherAPI interface {
	Publish(context.Context, *publish.Message) (*publish.Result, error)
}

// PublisherConfig configures the JetStream-backed xevent publisher.
type PublisherConfig struct {
	Publisher       *xnats.JetStreamPublisher
	EventTypeHeader string
	EventIDHeader   string
}

// Publisher adapts xevent.Publisher onto xnats.JetStreamPublisher.
type Publisher struct {
	publisher       publisherAPI
	eventTypeHeader string
	eventIDHeader   string
}

// NewPublisher creates a JetStream-backed xevent publisher.
func NewPublisher(cfg PublisherConfig) (*Publisher, error) {
	if cfg.Publisher == nil {
		return nil, ErrNilPublisher
	}

	return &Publisher{
		publisher:       cfg.Publisher,
		eventTypeHeader: normalizeHeaderName(cfg.EventTypeHeader, defaultEventTypeHeader),
		eventIDHeader:   normalizeHeaderName(cfg.EventIDHeader, defaultEventIDHeader),
	}, nil
}

// Publish publishes one xevent.Event to JetStream.
func (p *Publisher) Publish(ctx context.Context, event xevent.Event) error {
	outbound, err := xevent.Encode(event)
	if err != nil {
		return err
	}
	return p.Send(ctx, outbound)
}

// Send publishes one xevent.Outbound to JetStream.
func (p *Publisher) Send(ctx context.Context, outbound *xevent.Outbound) error {
	if p == nil || p.publisher == nil {
		return ErrNilPublisher
	}
	if outbound == nil {
		return xevent.ErrNilOutbound
	}
	if outbound.EventType == "" {
		return xevent.ErrEventTypeRequired
	}

	header := make(natsio.Header, 2)
	header.Add(p.eventTypeHeader, outbound.EventType)
	if eventID := outbound.EventID; eventID != "" {
		header.Add(p.eventIDHeader, eventID)
	}

	_, err := p.publisher.Publish(ctx, &publish.Message{
		Subject: outbound.EventType,
		Data:    cloneBytes(outbound.Payload),
		Header:  header,
	})
	return err
}
