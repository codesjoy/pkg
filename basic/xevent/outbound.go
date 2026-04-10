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

import "context"

// Outbound is the transport-neutral outbound event representation used by
// stores and sender adapters.
type Outbound struct {
	EventType    string
	EventID      string
	PartitionKey string
	Payload      []byte
	Topic        string
}

// Sender sends one outbound event payload.
type Sender interface {
	Send(context.Context, *Outbound) error
}

// BatchSender sends multiple outbound events in one call.
// Implementations that embed Sender can optionally implement BatchSend
// for more efficient batch delivery to the broker.
//
// Contract:
//   - for non-empty input, the returned error slice must have exactly the same
//     length as outbounds
//   - each returned item maps 1:1 to the input at the same index
//   - nil means that item was sent successfully; non-nil means that item failed
//   - sending is not atomic: partial success and partial failure are allowed
//   - for empty input, implementations may return nil
type BatchSender interface {
	Sender
	BatchSend(ctx context.Context, outbounds []*Outbound) []error
}

// Encode converts one Event into a reusable outbound payload.
//
// Steps: (1) validate the event is non-nil, (2) marshal its payload to bytes,
// (3) build an Outbound with a defensive copy of those bytes.
func Encode(event Event) (*Outbound, error) {
	// Step 1: validate.
	if isNilValue(event) {
		return nil, ErrNilEvent
	}

	eventType := event.EventType()
	if eventType == "" {
		return nil, ErrEventTypeRequired
	}

	// Step 2: marshal payload.
	payload, err := event.MarshalPayload()
	if err != nil {
		return nil, err
	}

	// Step 3: build outbound with a deep-copied payload to prevent aliasing.
	return &Outbound{
		EventType:    eventType,
		EventID:      event.EventID(),
		PartitionKey: event.PartitionKey(),
		Payload:      cloneBytes(payload),
		Topic:        event.Topic(),
	}, nil
}

// SenderFromPublisher adapts an Event publisher into an outbound sender.
func SenderFromPublisher(p Publisher) Sender {
	return publisherSender{publisher: p}
}

type publisherSender struct {
	publisher Publisher
}

// Send bridges an Outbound back to the Publisher interface by wrapping the
// outbound fields in an outboundEvent adapter.
func (s publisherSender) Send(ctx context.Context, outbound *Outbound) error {
	if outbound == nil {
		return ErrNilOutbound
	}
	if outbound.EventType == "" {
		return ErrEventTypeRequired
	}
	if s.publisher == nil {
		return ErrInvalidEventBinding
	}

	return s.publisher.Publish(ctx, &outboundEvent{
		eventType:    outbound.EventType,
		eventID:      outbound.EventID,
		partitionKey: outbound.PartitionKey,
		payload:      cloneBytes(outbound.Payload),
		topic:        outbound.Topic,
	})
}

// outboundEvent is the internal Event adapter that carries fields from an
// Outbound so they can be passed to a Publisher.
type outboundEvent struct {
	eventType    string
	eventID      string
	partitionKey string
	payload      []byte
	topic        string
}

func (e outboundEvent) EventType() string {
	return e.eventType
}

func (e outboundEvent) EventID() string {
	return e.eventID
}

func (e outboundEvent) PartitionKey() string {
	return e.partitionKey
}

func (e outboundEvent) MarshalPayload() ([]byte, error) {
	return cloneBytes(e.payload), nil
}

func (e outboundEvent) Topic() string {
	return e.topic
}

func (e *outboundEvent) UnmarshalPayload(data []byte) error {
	e.payload = cloneBytes(data)
	return nil
}
