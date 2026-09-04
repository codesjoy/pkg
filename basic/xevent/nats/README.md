# xevent/nats

`xevent/nats` provides JetStream adapters for the transport-agnostic
[`xevent`](https://pkg.go.dev/github.com/codesjoy/pkg/basic/xevent) core. It
bridges `xevent.Event` and `xevent.Dispatcher` onto the `xnats` JetStream
publisher and consumer primitives.

## Installation

```bash
go get github.com/codesjoy/pkg/basic/xevent/nats
```

This module depends on:

- `github.com/codesjoy/pkg/basic/xevent`
- `github.com/codesjoy/pkg/basic/xnats`

## What This Module Provides

- `Publisher`: maps an `xevent.Event` onto a JetStream message
- `Publisher.Send`: maps an `xevent.Outbound` onto a JetStream message
- `Subscriber`: consumes JetStream messages and dispatches them through a bound
  `xevent.Dispatcher`

## Publish Example

```go
package main

import (
	"context"
	"encoding/json"

	xeventnats "github.com/codesjoy/pkg/basic/xevent/nats"
)

type OrderCreated struct {
	ID      string `json:"id"`
	OrderID string `json:"order_id"`
}

func (*OrderCreated) EventType() string {
	return "order.created"
}

func (e *OrderCreated) EventID() string {
	return e.ID
}

func (e *OrderCreated) PartitionKey() string {
	return e.OrderID
}

func (e *OrderCreated) MarshalPayload() ([]byte, error) {
	return json.Marshal(e)
}

func (e *OrderCreated) UnmarshalPayload(data []byte) error {
	return json.Unmarshal(data, e)
}

func main() {
	publisher := mustNewJetStreamPublisher() // returns *xnats.JetStreamPublisher

	adapter, err := xeventnats.NewPublisher(xeventnats.PublisherConfig{
		Publisher: publisher,
	})
	if err != nil {
		panic(err)
	}

	if err := adapter.Publish(context.Background(), &OrderCreated{
		ID:      "evt_1",
		OrderID: "order-1",
	}); err != nil {
		panic(err)
	}
}
```

## Subscribe Example

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"

	xevent "github.com/codesjoy/pkg/basic/xevent"
	xeventnats "github.com/codesjoy/pkg/basic/xevent/nats"
)

type OrderCreated struct {
	ID      string `json:"id"`
	OrderID string `json:"order_id"`
}

func (*OrderCreated) EventType() string {
	return "order.created"
}

func (e *OrderCreated) EventID() string {
	return e.ID
}

func (e *OrderCreated) PartitionKey() string {
	return e.OrderID
}

func (e *OrderCreated) MarshalPayload() ([]byte, error) {
	return json.Marshal(e)
}

func (e *OrderCreated) UnmarshalPayload(data []byte) error {
	return json.Unmarshal(data, e)
}

func main() {
	dispatcher := xevent.NewDispatcher()

	if err := xevent.On[*OrderCreated](
		dispatcher,
		func(_ context.Context, event *OrderCreated) error {
			fmt.Println(event.ID, event.OrderID)
			return nil
		},
	); err != nil {
		panic(err)
	}

	consumer := mustNewJetStreamConsumer() // returns *xnats.JetStreamConsumer

	subscriber, err := xeventnats.NewSubscriber(xeventnats.SubscriberConfig{
		Consumer:   consumer,
		Dispatcher: dispatcher,
	})
	if err != nil {
		panic(err)
	}
	defer subscriber.Close()

	if err := subscriber.Subscribe(context.Background()); err != nil {
		panic(err)
	}
}
```

## API Overview

### `PublisherConfig`

- `Publisher`: required `*xnats.JetStreamPublisher`
- `EventTypeHeader`: optional override for the event type header name
- `EventIDHeader`: optional override for the event ID header name

### `SubscriberConfig`

- `Consumer`: required `*xnats.JetStreamConsumer`
- `Dispatcher`: required `*xevent.Dispatcher`
- `EventTypeHeader`: optional override for the event type header name

## Behavior Notes

- Default headers:
  - event type: `x-event-type`
  - event ID: `x-event-id`
- Publish uses `event.EventType()` as the JetStream subject
- Subscribe resolves the event type from header first, then falls back to the
  message subject
- `Subscriber.Subscribe(ctx)` is one-shot for a subscriber instance
- `Subscriber.Close()` is idempotent
- Consumer mode selection stays in `xnats.JetStreamConsumerConfig`; `Pull` is
  the default operational recommendation, and `Push` is still available through
  the wrapped `xnats` consumer

## Use As Outbox Sender

`Publisher` also implements `xevent.Sender`, so it can be passed directly to
`xevent/outbox`:

```go
relay, err := outbox.NewRelay(outbox.RelayConfig{
	Store:  store,
	Sender: publisher,
})
if err != nil {
	panic(err)
}
_ = relay
```

## Relationship To `xevent`

Use `xevent` to define events, bind typed handlers before consumption starts, and work with
transport-neutral dispatch. Use `xevent/nats` when you need to publish those
events to JetStream or consume JetStream messages into an `xevent.Dispatcher`.
