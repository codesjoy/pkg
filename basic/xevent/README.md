# xevent

`xevent` is a transport-agnostic domain event contract package. It defines a
minimal `Event` interface, transport-level message abstractions, and a typed
dispatcher that decodes `eventType + []byte` into a concrete event handler.

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
- `Outbound`: transport-neutral outbound event payload
- `Message`: transport-level input shape (`eventType + payload`)
- `Handler`: transport-level message handler
- `Middleware`: event-level chain around the decoded typed handler
- `Discard`: marks an explicit non-retryable handler error
- `Dispatcher`: routes one `Message` into its bound typed handler
- `Publisher`: transport-facing publish abstraction over an `Event`
- `Sender`: transport-facing send abstraction over an `Outbound`
- `Subscriber`: transport-facing lifecycle abstraction for consumption

## Outbound Encoding and Sending

If you need to persist an event before publishing it, convert it into an
`Outbound` first:

```go
outbound, err := xevent.Encode(&OrderCreated{
	ID:      "evt_1",
	OrderID: "o_123",
	UserID:  "u_1",
})
if err != nil {
	panic(err)
}
```

Existing publishers can be adapted into `Sender` values:

```go
sender := xevent.SenderFromPublisher(publisher)
if err := sender.Send(context.Background(), outbound); err != nil {
	panic(err)
}
```

This is the transport-neutral path used by the `xevent/outbox/relay` and
`xevent/outbox/debezium` packages and their optional storage adapters.

## Typed Dispatch

Register a typed handler with `xevent.On[T]`, then hand transport messages to
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

## Event Middleware

Register middleware on a dispatcher to run logic once after a payload has been
decoded into a concrete event and around the routed typed handler. Middleware is
constructed once for the dispatcher during configuration. Standard-library
logging middleware is available:

```go
import (
	"log/slog"

	"github.com/codesjoy/pkg/basic/xevent"
	eventlogger "github.com/codesjoy/pkg/basic/xevent/middleware/logger"
)

dispatcher := xevent.NewDispatcher()
dispatcher.Use(
	eventlogger.New(eventlogger.Config{
		Logger:   slog.Default(),
		LogEvent: false,
	}),
)
```

`EventContext` contains both the original `Message` and the concrete `Event`,
so middleware can perform domain validation, structured logging, metrics, or
auditing without coupling the core package to a particular framework. The
dispatcher-level chain runs once per typed message and routes to the single
bound business handler for that event type.
Each event type can have only one typed handler; compose multiple business
actions inside that handler when they share the same retry and acknowledgement
boundary. Middleware is not invoked for unknown event types handled by a
fallback handler. Transport concerns such as topic metadata,
retries, ack/nak, and transport-level tracing remain in the Kafka/NATS
middleware layers.
Configure all handlers, middleware, and fallback behavior before starting
consumption or calling `Handle`; configuration changes during dispatch are not
supported.
The logger omits the complete decoded event by default; set `LogEvent: true` to
include it. Use `xevent.Discard(err)` for explicitly non-retryable handler
errors; unmarked errors continue through the configured Kafka/NATS retry policy.

## Optional Adapters

`xevent` stays transport-neutral. Adapter modules are documented separately.

- Outbox relay core: `github.com/codesjoy/pkg/basic/xevent/outbox/relay`
- Outbox relay GORM adapter: `github.com/codesjoy/pkg/basic/xevent/outbox/relay/gorm`
- Debezium outbox core: `github.com/codesjoy/pkg/basic/xevent/outbox/debezium`
- Debezium outbox GORM adapter: `github.com/codesjoy/pkg/basic/xevent/outbox/debezium/gorm`
- Kafka adapter: `github.com/codesjoy/pkg/basic/xevent/kafka`
- NATS/JetStream adapter: `github.com/codesjoy/pkg/basic/xevent/nats`

Use transport adapters to translate transport-native messages into
`xevent.Message` values and route them through a bound dispatcher.

## Generate Event Methods From Protobuf

If your events are defined in protobuf, `tools/protoc-gen-codesjoy-event` can
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

  - local: protoc-gen-codesjoy-event
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

- `OnYourProtoEvent(dispatcher, handler)`
- `xevent.On[*YourProtoEvent](dispatcher, handler)`
- `publisher.Publish(ctx, protoEvent)`
- `dispatcher.Handle(ctx, message)`
