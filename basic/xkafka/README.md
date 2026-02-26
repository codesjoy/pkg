# xkafka

Sarama-based Kafka extension library with three facades:

- `GroupConsumer` (ConsumerGroup mode)
- `PartitionConsumer` (single topic+partition mode with checkpoint store)
- `Producer` (sync + batch + async)

It provides middleware pipelines (`logger -> retry -> user -> transport`),
sharded same-key ordering, and at-least-once semantics.

## Installation

```bash
go get github.com/codesjoy/pkg/basic/xkafka
```

## Examples

Runnable examples live in [`examples/`](./examples/).

From that directory:

```bash
go run ./group
go run ./partition
go run ./producer
go run ./trace
```

Supported environment variables:

- `XKAFKA_BROKERS` (default `127.0.0.1:9092`)
- `XKAFKA_TOPIC` (default `xkafka-example`)
- `XKAFKA_GROUP_ID` (default `xkafka-example-group`)
- `XKAFKA_PARTITION` (default `0`)
- `XKAFKA_TIMEOUT` (default `30s`)

## Layered Architecture

```text
xkafka/
├── group_consumer.go
├── partition_consumer.go
├── producer.go
├── group_consumer_config.go
├── partition_consumer_config.go
├── producer_config.go
├── types.go
├── errors.go
├── middleware/
│   ├── consume/
│   │   ├── chain.go
│   │   ├── logger/
│   │   ├── retry/
│   │   └── trace/
│   └── produce/
│       ├── chain.go
│       ├── logger/
│       ├── retry/
│       └── trace/
└── internal/
    ├── primitives/
    ├── runtime/
    │   ├── group/
    │   ├── partition/
    │   └── producer/
    ├── transport/sarama/
    └── store/
```

## Breaking Changes

- Removed all Option APIs (`WithXxx`, `Option`, `PartitionOption`, `ProducerOption`).
- New explicit constructors:
  - `NewGroupConsumer(GroupConsumerConfig)`
  - `NewPartitionConsumer(PartitionConsumerConfig)`
  - `NewProducer(ProducerConfig)`
- Middleware imports moved to:
  - consume: `github.com/codesjoy/pkg/basic/xkafka/middleware/consume`
  - produce: `github.com/codesjoy/pkg/basic/xkafka/middleware/produce`

## Group Consumer Example

```go
package main

import (
	"context"
	"log/slog"

	"github.com/codesjoy/pkg/basic/xkafka"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
)

func main() {
	consumer, err := xkafka.NewGroupConsumer(xkafka.GroupConsumerConfig{
		Brokers: []string{"127.0.0.1:9092"},
		GroupID: "demo-group",
		Topics:  []string{"orders"},
		Logger:  slog.Default(),
	})
	if err != nil {
		panic(err)
	}
	defer consumer.Close()

	err = consumer.Consume(context.Background(), func(ctx context.Context, msg *consume.MessageContext) error {
		return nil
	})
	if err != nil {
		panic(err)
	}
}
```

## Partition Consumer Example

```go
package main

import (
	"context"
	"log/slog"

	"github.com/IBM/sarama"
	"github.com/codesjoy/pkg/basic/xkafka"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
)

func main() {
	store := xkafka.NewMemoryOffsetStore()
	consumer, err := xkafka.NewPartitionConsumer(xkafka.PartitionConsumerConfig{
		Brokers:       []string{"127.0.0.1:9092"},
		Topic:         "orders",
		Partition:     0,
		OffsetStore:   store,
		InitialOffset: sarama.OffsetOldest,
		Logger:        slog.Default(),
	})
	if err != nil {
		panic(err)
	}
	defer consumer.Close()

	err = consumer.Consume(context.Background(), func(ctx context.Context, msg *consume.MessageContext) error {
		return nil
	})
	if err != nil {
		panic(err)
	}
}
```

## Producer Example

```go
package main

import (
	"context"
	"time"

	"github.com/codesjoy/pkg/basic/xkafka"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
)

func main() {
	producer, err := xkafka.NewProducer(xkafka.ProducerConfig{
		Brokers:      []string{"127.0.0.1:9092"},
		DefaultTopic: "orders",
		Dispatch: xkafka.ProducerDispatchConfig{
			Mode:       xkafka.ProducerDispatchModeKeySharded,
			ShardCount: 32,
			QueueSize:  1024,
		},
	})
	if err != nil {
		panic(err)
	}
	defer producer.Close()

	_, _ = producer.Produce(context.Background(), &produce.Message{Value: []byte("one")})
	_, _ = producer.ProduceBatch(context.Background(),
		&produce.Message{Value: []byte("a")},
		&produce.Message{Value: []byte("b")},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	future, _ := producer.ProduceAsync(ctx, &produce.Message{Value: []byte("async")})
	_, _ = future.Await(context.Background())
}
```

## Default Pipelines

- Consume: `consume/logger -> consume/retry -> user handlers -> business`
- Produce: `produce/logger -> produce/retry -> user handlers -> transport send`

Retry middleware wraps downstream handlers, so user handlers and business logic may
run multiple times. Keep handlers idempotent.

## OpenTelemetry Trace Middleware

Trace middleware is opt-in plugin middleware and is not injected by default.

- Consume middleware: `github.com/codesjoy/pkg/basic/xkafka/middleware/consume/trace`
- Produce middleware: `github.com/codesjoy/pkg/basic/xkafka/middleware/produce/trace`

Consume extracts parent context from Kafka headers (`traceparent/tracestate/...`),
produce injects current context into Kafka headers.

Example (consume):

```go
import (
	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
	ctrace "github.com/codesjoy/pkg/basic/xkafka/middleware/consume/trace"
)

consumer, err := xkafka.NewGroupConsumer(xkafka.GroupConsumerConfig{
	Brokers: []string{"127.0.0.1:9092"},
	GroupID: "demo-group",
	Topics:  []string{"orders"},
	GlobalHandlers: []consume.Handler{
		ctrace.New(ctrace.Config{}),
	},
})
```

Example (produce):

```go
import (
	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
	ptrace "github.com/codesjoy/pkg/basic/xkafka/middleware/produce/trace"
)

producer, err := xkafka.NewProducer(xkafka.ProducerConfig{
	Brokers: []string{"127.0.0.1:9092"},
	DefaultTopic: "orders",
	GlobalHandlers: []produce.Handler{
		ptrace.New(ptrace.Config{}),
	},
})
```

Trace span granularity is per-attempt. Since trace middleware is user middleware
inside retry middleware, each retry attempt creates a new span.

## Integration Tests (Docker)

The integration suite is Docker-backed and uses Testcontainers to start Kafka.

Run from the module root:

```bash
go test -tags=integration ./testing/integration -v
```

The default `go test ./...` and `make MODULES="basic/xkafka" test` remain unit-test focused.

## Semantics

- At-least-once delivery (duplicates are possible).
- Group mode commits only contiguous completed offsets.
- Partition mode persists contiguous `nextOffset` checkpoints.
- Async producer modes:
  - `serial`: global order
  - `key_sharded`: same-key order
  - `parallel`: no ordering guarantee
