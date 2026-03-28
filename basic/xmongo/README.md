# xmongo

Native-style MongoDB Go driver v2 client builder with optional monitor-based observability.

## Features

- Native driver API through `*xmongo.Client` embedding `*mongo.Client`
- `Config + Option` constructor model
- Deterministic native option merge order
- Deterministic monitor fan-out order for command, pool, and server monitors
- Explicit opt-in OpenTelemetry tracing and metrics via `WithOpenTelemetry`
- Structured slog logging via `WithLogger`
- Optional default database helpers via `DB()` and `Collection(...)`
- Explicit readiness check helper via `PingPrimary(ctx)`
- Lightweight health snapshots via `WithHealthTracking`
- Transaction integration via `github.com/codesjoy/pkg/basic/transaction/mongo`
- `Config.Validate` trims the URI, rejects empty URI, rejects nil native options, and validates merged driver options

## Installation

```bash
go get github.com/codesjoy/pkg/basic/xmongo
```

## Quick Start

```go
package main

import (
	"context"
	"time"

	"github.com/codesjoy/pkg/basic/xmongo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := xmongo.New(xmongo.Config{
		URI:             "mongodb://127.0.0.1:27017",
		DefaultDatabase: "app",
	})
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := client.Disconnect(ctx); err != nil {
			panic(err)
		}
	}()

	// Connect does not Ping. Perform explicit readiness checks when needed.
	if err := client.PingPrimary(ctx); err != nil {
		panic(err)
	}

	collection, err := client.Collection("widgets")
	if err != nil {
		panic(err)
	}
	_, err = collection.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "widget-1"},
		{Key: "name", Value: "gear"},
	})
	if err != nil {
		panic(err)
	}
}
```

## OpenTelemetry

```go
import (
	"github.com/codesjoy/pkg/basic/xmongo"
	xotel "github.com/codesjoy/pkg/basic/xmongo/middleware/otel"
)

client, err := xmongo.New(
	xmongo.Config{URI: "mongodb://127.0.0.1:27017"},
	xmongo.WithOpenTelemetry(xotel.Config{
		EnableTracing:  true,
		EnableMetrics:  true,
		TracerProvider: tracerProvider,
		MeterProvider:  meterProvider,
	}),
)
```

## Logger

```go
import (
	"log/slog"

	"github.com/codesjoy/pkg/basic/xmongo"
	xlog "github.com/codesjoy/pkg/basic/xmongo/middleware/logger"
)

client, err := xmongo.New(
	xmongo.Config{URI: "mongodb://127.0.0.1:27017"},
	xmongo.WithLogger(xlog.Config{
		Logger:        slog.Default(),
		SlowThreshold: 200 * time.Millisecond,
	}),
)
```

## Optional Default Database

If `DefaultDatabase` is configured, `DB()` and `Collection(name)` return handles from that database.

```go
client, err := xmongo.New(xmongo.Config{
	URI:             "mongodb://127.0.0.1:27017",
	DefaultDatabase: "app",
})
if err != nil {
	panic(err)
}

db, err := client.DB()
if err != nil {
	panic(err)
}

widgets, err := client.Collection("widgets")
if err != nil {
	panic(err)
}

_ = db
_ = widgets
```

If you need another database name or advanced options, continue to use the native driver entry points directly:

```go
analytics := client.Database("analytics")
events := analytics.Collection("events")

_ = events
```

## Health Snapshot

```go
client, err := xmongo.New(
	xmongo.Config{URI: "mongodb://127.0.0.1:27017"},
	xmongo.WithHealthTracking(),
)
if err != nil {
	panic(err)
}

if err := client.PingPrimary(context.Background()); err != nil {
	panic(err)
}

snapshot := client.HealthSnapshot()
_ = snapshot
```

## Transaction Integration

```go
import (
	"github.com/codesjoy/pkg/basic/transaction"
	mongotx "github.com/codesjoy/pkg/basic/transaction/mongo"
)

client, err := xmongo.New(xmongo.Config{
	URI:             "mongodb://127.0.0.1:27017",
	DefaultDatabase: "app",
})
if err != nil {
	panic(err)
}

widgets, err := client.Collection("widgets")
if err != nil {
	panic(err)
}

runner := mongotx.New(client.Raw())

err = runner.Within(context.Background(), func(txCtx context.Context) error {
	_, err := widgets.InsertOne(txCtx, bson.D{{Key: "_id", Value: "widget-1"}})
	if err != nil {
		return err
	}

	return transaction.AfterCommit(txCtx, func(context.Context) error {
		// Trigger follow-up work after commit.
		return nil
	})
})
if err != nil {
	panic(err)
}
```

## Retry Behavior

`xmongo` does not wrap MongoDB operations in an extra retry layer. Continue to use the native driver retry settings through `Config.ClientOptions` or `WithClientOptions(...)`, for example:

```go
client, err := xmongo.New(
	xmongo.Config{
		URI: "mongodb://127.0.0.1:27017/?retryReads=true&retryWrites=true",
	},
	xmongo.WithClientOptions(
		options.Client().SetRetryReads(true).SetRetryWrites(true),
	),
)
```

## Native Driver Options

`Config.ClientOptions` are merged after `ApplyURI`, then `WithClientOptions(...)` values are merged in call order after that. Later native options override earlier ones following the MongoDB driver's native merge semantics.

If a native option already includes a command, pool, or server monitor, `xmongo` keeps that monitor as the baseline and appends monitors from `WithCommandMonitor(...)`, `WithPoolMonitor(...)`, `WithServerMonitor(...)`, and `WithOpenTelemetry(...)` after it.
