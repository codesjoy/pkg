# xevent

`xevent` is a transport-agnostic domain event contract package. It defines a
minimal `Event` interface, transport-level message abstractions, and a typed
dispatcher that decodes `eventType + []byte` into concrete event handlers.

## Installation

```bash
go get github.com/codesjoy/pkg/basic/xevent
```

## Quick Start

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/codesjoy/pkg/basic/xevent"
)

type OrderCreated struct {
	ID      string `json:"id"`
	OrderID string `json:"order_id"`
	UserID  string `json:"user_id"`
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
			fmt.Println(event.EventType(), event.EventID(), event.PartitionKey())
			return nil
		},
	); err != nil {
		panic(err)
	}

	payload, err := (&OrderCreated{
		ID:      "evt_1",
		OrderID: "o_123",
		UserID:  "u_1",
	}).MarshalPayload()
	if err != nil {
		panic(err)
	}

	if err := dispatcher.Handle(context.Background(), &xevent.Message{
		EventType: "order.created",
		Payload:   payload,
	}); err != nil {
		panic(err)
	}
}
```

## API Overview

- `Event`: event identity, partition key, and payload encode/decode contract
- `Message`: transport-level input shape (`eventType + payload`)
- `Handler`: transport-level message handler
- `Dispatcher`: routes one `Message` into bound typed handlers
- `Publisher`: transport-facing publish abstraction over an `Event`
- `Subscriber`: transport-facing lifecycle abstraction for consumption

## Typed Dispatch

Register typed handlers with `xevent.On[T]`, then hand transport messages to
the dispatcher:

```go
dispatcher := xevent.NewDispatcher()

_ = xevent.On[*OrderCreated](dispatcher, func(ctx context.Context, event *OrderCreated) error {
	return nil
})

payload, _ := (&OrderCreated{
	ID:      "evt_1",
	OrderID: "o_123",
	UserID:  "u_1",
}).MarshalPayload()

_ = dispatcher.Handle(context.Background(), &xevent.Message{
	EventType: "order.created",
	Payload:   payload,
})
```

`xevent.On[T](dispatcher, handler)` is a package-level function because Go does
not support generic methods.

## Transport Adapters

`xevent` stays transport-neutral. Adapter modules are documented separately.

- Kafka adapter: `github.com/codesjoy/pkg/basic/xevent/kafka`
- NATS/JetStream adapter: `github.com/codesjoy/pkg/basic/xevent/nats`

Use transport adapters to translate transport-native messages into
`xevent.Message` values and route them through a bound dispatcher.

## Generate Event Methods From Protobuf

If your events are defined in protobuf, `tools/protoc-gen-codesjoy-ddd` can
generate the `xevent.Event` methods for you:

```proto
import "codesjoy/ddd/event/v1/event.proto";

message OrderCreated {
  option (codesjoy.ddd.event.v1.event) = {};

  string id = 1 [(codesjoy.ddd.event.v1.event_id) = true];
  string order_id = 2 [(codesjoy.ddd.event.v1.partition_key) = true];
  string user_id = 3;
}
```

Example Buf configuration:

```yaml
version: v2

plugins:
  - local: protoc-gen-go
    out: ./gen
    opt: paths=source_relative

  - local: protoc-gen-codesjoy-ddd
    out: ./gen
    opt: paths=source_relative
```

If `event_type` is not set explicitly, the generated `EventType()` defaults to
the protobuf message full name. If you need a stable business event name, set
it explicitly:

```proto
option (codesjoy.ddd.event.v1.event) = { event_type: "order.created" };
```

Generated messages can be used directly with:

- `OnYourProtoEvent(dispatcher, handler1, handler2)`
- `xevent.On[*YourProtoEvent](dispatcher, handler)`
- `publisher.Publish(ctx, protoEvent)`
- `dispatcher.Handle(ctx, message)`
