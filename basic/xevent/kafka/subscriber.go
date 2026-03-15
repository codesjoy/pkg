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
	"sync"

	"github.com/codesjoy/pkg/basic/xevent"
	"github.com/codesjoy/pkg/basic/xkafka"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
)

type consumerAPI interface {
	Consume(context.Context, consume.HandlerFunc) error
	Close() error
}

// SubscriberConfig configures the Kafka-backed xevent subscriber.
type SubscriberConfig struct {
	Consumer        *xkafka.GroupConsumer
	Dispatcher      *xevent.Dispatcher
	EventTypeHeader string
}

// Subscriber adapts xkafka.GroupConsumer onto xevent.Subscriber.
type Subscriber struct {
	consumer        consumerAPI
	dispatcher      *xevent.Dispatcher
	eventTypeHeader string

	mu        sync.Mutex
	started   bool
	closed    bool
	closeOnce sync.Once
	closeErr  error
}

// NewSubscriber creates a Kafka-backed xevent subscriber.
func NewSubscriber(cfg SubscriberConfig) (*Subscriber, error) {
	if cfg.Consumer == nil {
		return nil, ErrNilConsumer
	}
	if cfg.Dispatcher == nil {
		return nil, ErrNilDispatcher
	}

	return &Subscriber{
		consumer:        cfg.Consumer,
		dispatcher:      cfg.Dispatcher,
		eventTypeHeader: normalizeHeaderName(cfg.EventTypeHeader, defaultEventTypeHeader),
	}, nil
}

// Subscribe starts consuming Kafka messages and dispatching them to typed handlers.
func (s *Subscriber) Subscribe(ctx context.Context) error {
	if s == nil || s.consumer == nil {
		return ErrNilConsumer
	}
	if s.dispatcher == nil {
		return ErrNilDispatcher
	}
	if err := s.markStarted(); err != nil {
		return err
	}

	return s.consumer.Consume(
		ctx,
		func(handlerCtx context.Context, msg *consume.MessageContext) error {
			if msg == nil || msg.Message == nil {
				return ErrEventTypeHeaderRequired
			}

			eventType := consumerHeaderValue(msg.Message.Headers, s.eventTypeHeader)
			if eventType == "" {
				return ErrEventTypeHeaderRequired
			}

			return s.dispatcher.Handle(handlerCtx, &xevent.Message{
				EventType: eventType,
				Payload:   cloneBytes(msg.Message.Value),
			})
		},
	)
}

// Close releases the wrapped Kafka consumer.
func (s *Subscriber) Close() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()

	s.closeOnce.Do(func() {
		if s.consumer != nil {
			s.closeErr = s.consumer.Close()
		}
	})

	return s.closeErr
}

func (s *Subscriber) markStarted() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return xevent.ErrSubscriberStarted
	}
	if s.closed {
		return xevent.ErrSubscriberClosed
	}

	s.started = true
	return nil
}
