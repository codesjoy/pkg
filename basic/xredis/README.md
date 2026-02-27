# xredis

go-redis/v9 native-style client builder with middleware-based observability.

## Features

- Native command style through `*xredis.Client` embedding `redis.UniversalClient`
- `Config + Option` constructor model
- Middleware order is fully caller-controlled via option order
- Explicit opt-in observability (`WithLogger`, `WithOpenTelemetry`)
- `Config.Validate` trims addresses and drops blank entries

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
