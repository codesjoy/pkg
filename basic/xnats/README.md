# xnats

NATS and JetStream extension library with five facades:

- `Publisher`
- `Subscriber`
- `Requester`
- `JetStreamPublisher`
- `JetStreamConsumer`

It provides explicit configs, middleware pipelines (`logger -> retry -> user -> transport`),
and unified retry/failure semantics for core NATS and JetStream.

## Installation

```bash
go get github.com/codesjoy/pkg/basic/xnats
```

## Examples

Runnable examples live in [`examples/`](./examples/).

From that directory:

```bash
go run ./publisher
go run ./requester
go run ./jetstream-publisher
go run ./jetstream-consumer-pull
go run ./jetstream-consumer-push
```

Supported environment variables:

- `XNATS_URL` (default `nats://127.0.0.1:4222`)
- `XNATS_SUBJECT` (default `xnats.example`)
- `XNATS_STREAM` (default `XNATS_EXAMPLE`)
- `XNATS_CONSUMER` (default `xnats-example-consumer`)
- `XNATS_TIMEOUT` (default `30s`)

## Quick Start

### Core Publish / Subscribe

```go
package main

import (
	"context"
	"log/slog"

	"github.com/codesjoy/pkg/basic/xnats"
	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
)

func main() {
	subscriber, err := xnats.NewSubscriber(xnats.SubscriberConfig{
		URLs:     []string{"nats://127.0.0.1:4222"},
		Subjects: []string{"orders.created"},
		Logger:   slog.Default(),
	})
	if err != nil {
		panic(err)
	}
	defer subscriber.Close()

	go func() {
		_ = subscriber.Consume(context.Background(), func(_ context.Context, msg *consume.MessageContext) error {
			return nil
		})
	}()

	publisher, err := xnats.NewPublisher(xnats.PublisherConfig{
		URLs:           []string{"nats://127.0.0.1:4222"},
		DefaultSubject: "orders.created",
		Logger:         slog.Default(),
	})
	if err != nil {
		panic(err)
	}
	defer publisher.Close()

	_, _ = publisher.Publish(context.Background(), &publish.Message{
		Data: []byte("hello"),
	})
}
```

### Request / Reply

```go
requester, err := xnats.NewRequester(xnats.RequesterConfig{
	URLs: []string{"nats://127.0.0.1:4222"},
})
if err != nil {
	panic(err)
}
defer requester.Close()

resp, err := requester.Request(context.Background(), &publish.Message{
	Subject: "svc.echo",
	Data:    []byte("ping"),
})
```

### JetStream Publish / Consume

```go
publisher, err := xnats.NewJetStreamPublisher(xnats.JetStreamPublisherConfig{
	URLs:           []string{"nats://127.0.0.1:4222"},
	DefaultSubject: "orders.created",
})
if err != nil {
	panic(err)
}
defer publisher.Close()

consumer, err := xnats.NewJetStreamConsumer(xnats.JetStreamConsumerConfig{
	URLs:       []string{"nats://127.0.0.1:4222"},
	Stream:     "ORDERS",
	Consumer:   "worker",
	Mode:       xnats.JetStreamConsumerModePull,
	PullBatchSize: 1,
})
if err != nil {
	panic(err)
}
defer consumer.Close()
```

## OpenTelemetry Trace Middleware

Trace middleware is opt-in plugin middleware and is not injected by default.

- consume middleware: `github.com/codesjoy/pkg/basic/xnats/middleware/consume/trace`
- publish middleware: `github.com/codesjoy/pkg/basic/xnats/middleware/publish/trace`

Example (consume):

```go
import (
	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
	ctrace "github.com/codesjoy/pkg/basic/xnats/middleware/consume/trace"
)

subscriber, err := xnats.NewSubscriber(xnats.SubscriberConfig{
	URLs:     []string{"nats://127.0.0.1:4222"},
	Subjects: []string{"orders.created"},
	GlobalHandlers: []consume.Handler{
		ctrace.New(ctrace.Config{}),
	},
})
```

Example (publish):

```go
import (
	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
	ptrace "github.com/codesjoy/pkg/basic/xnats/middleware/publish/trace"
)

publisher, err := xnats.NewPublisher(xnats.PublisherConfig{
	URLs:           []string{"nats://127.0.0.1:4222"},
	DefaultSubject: "orders.created",
	GlobalHandlers: []publish.Handler{
		ptrace.New(ptrace.Config{}),
	},
})
```

Publish injects trace context into `nats.Header`; consume extracts parent context from message headers.

## Failure Policies

Publish retry middleware:

- `block`: keep retrying forever
- `stop`: return error after retries exhaust
- `drop`: drop the message and return `ErrMessageDropped`

Consume retry middleware:

- `block`: keep retrying forever
- `stop`: stop consume loop; JetStream messages are `Nak`ed
- `drop`: swallow the business error; JetStream messages are `Ack`ed

## Integration Tests

The integration suite is Docker-backed and starts a NATS server with JetStream enabled.

Run from the module root:

```bash
go test -tags=integration ./testing/integration -v
```
