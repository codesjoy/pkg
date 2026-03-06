# xredis

go-redis/v9 native-style client builder with middleware-based observability.

## Features

- Native command style through `*xredis.Client` embedding `redis.UniversalClient`
- `Config + Option` constructor model
- Middleware order is fully caller-controlled via option order
- Explicit opt-in observability (`WithLogger`, `WithOpenTelemetry`)
- `Config.Validate` trims addresses and drops blank entries
- Distributed lock subpackage via `github.com/codesjoy/pkg/basic/xredis/lock`

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
