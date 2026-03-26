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
}

// Sender sends one outbound event payload.
type Sender interface {
	Send(context.Context, *Outbound) error
}

// Encode converts one Event into a reusable outbound payload.
func Encode(event Event) (*Outbound, error) {
	if isNilValue(event) {
		return nil, ErrNilEvent
	}

	eventType := event.EventType()
	if eventType == "" {
		return nil, ErrEventTypeRequired
	}

	payload, err := event.MarshalPayload()
	if err != nil {
		return nil, err
	}

	return &Outbound{
		EventType:    eventType,
		EventID:      event.EventID(),
		PartitionKey: event.PartitionKey(),
		Payload:      cloneBytes(payload),
	}, nil
}

// SenderFromPublisher adapts an Event publisher into an outbound sender.
func SenderFromPublisher(p Publisher) Sender {
	return publisherSender{publisher: p}
}

type publisherSender struct {
	publisher Publisher
}

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
	})
}

type outboundEvent struct {
	eventType    string
	eventID      string
	partitionKey string
	payload      []byte
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

func (e *outboundEvent) UnmarshalPayload(data []byte) error {
	e.payload = cloneBytes(data)
	return nil
}
