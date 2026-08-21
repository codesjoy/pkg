# xevent/kafka

`xevent/kafka` provides Kafka adapters for the transport-agnostic
[`xevent`](https://pkg.go.dev/github.com/codesjoy/pkg/basic/xevent) core. It
bridges `xevent.Event` and `xevent.Dispatcher` onto the `xkafka` producer and
consumer primitives.

## Installation

```bash
go get github.com/codesjoy/pkg/basic/xevent/kafka
```

This module depends on:

- `github.com/codesjoy/pkg/basic/xevent`
- `github.com/codesjoy/pkg/basic/xkafka`

## What This Module Provides

- `Publisher`: maps an `xevent.Event` onto a Kafka message
- `Publisher.Send`: maps an `xevent.Outbound` onto a Kafka message
- `Subscriber`: consumes Kafka messages and dispatches them through a bound
  `xevent.Dispatcher`

## Publish Example

```go
package main

import (
	"context"
	"encoding/json"

	xeventkafka "github.com/codesjoy/pkg/basic/xevent/kafka"
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
	producer := mustNewProducer() // returns *xkafka.Producer

	publisher, err := xeventkafka.NewPublisher(xeventkafka.PublisherConfig{
		Producer: producer,
		Topic:    "orders",
	})
	if err != nil {
		panic(err)
	}

	if err := publisher.Publish(context.Background(), &OrderCreated{
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
	xeventkafka "github.com/codesjoy/pkg/basic/xevent/kafka"
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

	consumer := mustNewGroupConsumer() // returns *xkafka.GroupConsumer

	subscriber, err := xeventkafka.NewSubscriber(xeventkafka.SubscriberConfig{
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

- `Producer`: required `*xkafka.Producer`
- `Topic`: optional default Kafka topic name, used when the outbound event does
  not carry a topic
- `EventTypeHeader`: optional override for the event type header name
- `EventIDHeader`: optional override for the event ID header name

### `SubscriberConfig`

- `Consumer`: required `*xkafka.GroupConsumer`
- `Dispatcher`: required `*xevent.Dispatcher`
- `EventTypeHeader`: optional override for the event type header name

## Behavior Notes

- Default Kafka headers:
  - event type: `x-event-type`
  - event ID: `x-event-id`
- Publisher topic resolution uses `Outbound.Topic` first and falls back to
  `PublisherConfig.Topic`; publishing fails with `ErrTopicRequired` when both
  are empty.
- `Subscriber.Subscribe(ctx)` is one-shot for a subscriber instance.
- `Subscriber.Close()` is idempotent.
- `NewSubscriber` requires a dispatcher up front; typed handlers are registered
  on that dispatcher before consumption starts.

## Use As Outbox Sender

`Publisher` also implements `xevent.Sender`, so it can be used directly by
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

Use `xevent` to define events, bind typed handlers, and work with
transport-neutral dispatch. Use `xevent/kafka` when you need to publish those
events to Kafka or consume Kafka messages into an `xevent.Dispatcher`.
