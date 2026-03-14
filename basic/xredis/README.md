# xredis

go-redis/v9 native-style client builder with middleware-based observability.

## Features

- Native command style through `*xredis.Client` embedding `redis.UniversalClient`
- `Config + Option` constructor model
- Middleware order is fully caller-controlled via option order
- Explicit opt-in observability (`WithLogger`, `WithOpenTelemetry`)
- `Config.Validate` trims addresses and drops blank entries
- Distributed lock subpackage via `github.com/codesjoy/pkg/basic/xredis/lock`
- Redis Streams MQ subpackage via `github.com/codesjoy/pkg/basic/xredis/mq`

## Installation

```bash
go get github.com/codesjoy/pkg/basic/xredis
```

## Quick Start

```go
package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/codesjoy/pkg/basic/xredis"
	"github.com/codesjoy/pkg/basic/xredis/middleware/logger"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := xredis.Config{
		UniversalOptions: redis.UniversalOptions{
			Addrs: []string{"127.0.0.1:6379"},
		},
	}

	client, err := xredis.New(
		cfg,
		xredis.WithLogger(logger.Config{
			Logger:        slog.Default(),
			SlowThreshold: 200 * time.Millisecond,
		}),
	)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	ctx := context.Background()
	if err := client.Set(ctx, "hello", "world", 0).Err(); err != nil {
		panic(err)
	}
}
```

## OpenTelemetry

```go
import (
	"github.com/codesjoy/pkg/basic/xredis"
	xotel "github.com/codesjoy/pkg/basic/xredis/middleware/otel"
	"github.com/redis/go-redis/v9"
)

client, err := xredis.New(
	xredis.Config{
		UniversalOptions: redis.UniversalOptions{
			Addrs: []string{"127.0.0.1:6379"},
		},
	},
	xredis.WithOpenTelemetry(xotel.Config{
		EnableTracing: true,
		EnableMetrics: true,
		DBStatement:   false,
	}),
)
```

## Distributed Lock

```go
import (
	"context"
	"time"

	"github.com/codesjoy/pkg/basic/xredis"
	"github.com/codesjoy/pkg/basic/xredis/lock"
	"github.com/redis/go-redis/v9"
)

func main() {
	client, err := xredis.New(
		xredis.Config{
			UniversalOptions: redis.UniversalOptions{
				Addrs: []string{"127.0.0.1:6379"},
			},
		},
	)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	locker, err := lock.New(client, lock.Config{})
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	lease, err := locker.Acquire(
		ctx,
		"jobs:daily-report",
		5*time.Second,
		lock.WithAutoRenew(),
	)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = lease.Release(ctx)
	}()

	// Observe the local lease lifecycle when background renew stops unexpectedly.
	go func() {
		<-lease.Done()
		if err := lease.Err(); err != nil {
			panic(err)
		}
	}()

	// If you prefer explicit control instead of background renew, call lease.Refresh(ctx).
	if err := client.Set(ctx, "jobs:daily-report:last-run", time.Now().UTC().String(), 0).Err(); err != nil {
		panic(err)
	}
}
```

`WithAutoRenew()` and `WithAutoRenewInterval(...)` work for the default single-client lock path.
Sentinel and Cluster clients are also supported in this single-client mode.

## Redis MQ

```go
import (
	"context"
	"time"

	"github.com/codesjoy/pkg/basic/xredis"
	"github.com/codesjoy/pkg/basic/xredis/mq"
	"github.com/redis/go-redis/v9"
)

func main() {
	client, err := xredis.New(
		xredis.Config{
			UniversalOptions: redis.UniversalOptions{
				Addrs: []string{"127.0.0.1:6379"},
			},
		},
	)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	publisher, err := mq.NewPublisher(client, mq.PublisherConfig{
		DefaultStream:     "jobs",
		OrderedShardCount: 8,
		OrderKeyHeader:    "x-order-key",
	})
	if err != nil {
		panic(err)
	}

	consumer, err := mq.NewConsumer(client, mq.ConsumerConfig{
		Stream:            "jobs",
		Group:             "workers",
		Consumer:          "worker-1",
		AutoCreateGroup:   true,
		OrderedShardCount: 8,
		OwnedShards:       []int{0, 1, 2, 3},
		OrderKeyHeader:    "x-order-key",
		Block:             time.Second,
		AutoClaimMinIdle:  30 * time.Second,
	})
	if err != nil {
		panic(err)
	}
	defer consumer.Close()

	if _, err := publisher.Publish(context.Background(), &mq.Message{
		Payload: []byte("send-email"),
		Headers: map[string]string{
			"kind":        "welcome",
			"x-order-key": "user-42",
		},
	}); err != nil {
		panic(err)
	}

	go func() {
		err := consumer.Consume(context.Background(), func(ctx context.Context, msg *mq.MessageContext) error {
			if string(msg.Message.Payload) == "send-email" {
				return consumer.Close()
			}
			return nil
		})
		if err != nil && err != mq.ErrConsumerClosed {
			panic(err)
		}
	}()
}
```

`mq.Consumer` uses Redis Streams consumer groups with auto-ack on successful handler return.
Set `PublisherConfig.OrderedShardCount` to route same-key messages into stable shard streams, then let each
consumer instance claim a disjoint `OwnedShards` set. That combination gives group-wide same-key ordering.
When a handler returns an error, the message stays in pending and can be recovered by a later `XAUTOCLAIM`
on the same shard stream.

## Redlock

Use Redlock only with multiple independent Redis masters.
Do not treat Sentinel failover or Redis Cluster nodes as Redlock peers.

```go
import (
	"context"
	"time"

	"github.com/codesjoy/pkg/basic/xredis/lock"
	"github.com/redis/go-redis/v9"
)

func main() {
	primary := redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs: []string{"127.0.0.1:6379"},
	})
	peer1 := redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs: []string{"127.0.0.1:6380"},
	})
	peer2 := redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs: []string{"127.0.0.1:6381"},
	})
	defer primary.Close()
	defer peer1.Close()
	defer peer2.Close()

	locker, err := lock.New(primary, lock.Config{
		Redlock: &lock.RedlockConfig{
			Peers: []redis.UniversalClient{peer1, peer2},
		},
	})
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	lease, err := locker.Acquire(
		ctx,
		"jobs:global-report",
		5*time.Second,
		lock.WithAutoRenew(),
	)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = lease.Release(ctx)
	}()
}
```
