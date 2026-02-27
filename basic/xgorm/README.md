# xgorm

Enhanced GORM utilities with pagination, transaction management, plugins, and more.

## Features

- **Pagination**: Built-in support for paginated queries with count tracking
- **Transaction Management**: Type-safe context-based transaction handling
- **Sharding**: Table sharding support powered by `gorm.io/sharding`
- **Database Routing**: Multi-database read/write routing powered by `gorm.io/plugin/dbresolver`
- **Plugins**:
  - Structured logging with `log/slog`
  - Metrics collection (query count, duration, errors)
  - OpenTelemetry distributed tracing
- **Error Handling**: Custom error types with proper wrapping
- **Thread-Safe**: All operations are safe for concurrent use

## Installation

```bash
go get github.com/codesjoy/pkg/basic/xgorm
```

## Quick Start

### Basic Setup

```go
package main

import (
    "log/slog"
    "time"
    "github.com/codesjoy/pkg/basic/xgorm"
    gormlogger "gorm.io/gorm/logger"
    "gorm.io/driver/postgres"
)

func main() {
    // Create GORM instance with plugins
    db, err := xgorm.New(
        postgres.Open("postgres://user:pass@localhost/db"),
        xgorm.WithSlogLogger(slog.Default()),
        xgorm.WithLoggerConfig(gormlogger.Info, 200*time.Millisecond, false),
        xgorm.WithMeter(meter), // meter from otel.Meter(...)
    )
    if err != nil {
        panic(err)
    }

    // Use db...
}
```

### Pagination

```go
type User struct {
    ID   uint
    Name string
}

// Paginated query
var users []User
result, err := xgorm.WrapPageQuery(db, xgorm.PaginationParam{
    Pagination: true,
    Current:    1,
    PageSize:   20,
}, &users)

if err != nil {
    panic(err)
}

fmt.Printf("Total: %d, Page: %d, Size: %d\n",
    result.Total, result.Current, result.PageSize)
```

### Count Only (No Data Fetch)

```go
result, err := xgorm.WrapPageQuery(db, xgorm.PaginationParam{
    OnlyCount: true,
}, &[]User{})

fmt.Printf("Total records: %d\n", result.Total)
```

### Transaction Management

```go
trans := xgorm.NewTransaction(db)
ctx := context.Background()

// Manual transaction management
ctx = trans.Begin(ctx)
tx := trans.GetTx(ctx)

// ... perform operations ...

if err != nil {
    trans.Rollback(ctx)
    return err
}

return trans.Commit(ctx)
```

#### Transaction Helper

```go
trans := xgorm.NewTransaction(db)

err := trans.Transaction(ctx, func(tx *gorm.DB) error {
    // All operations in this function use the transaction
    if err := tx.Create(&user).Error; err != nil {
        return err  // Automatically rolls back
    }
    return nil  // Commits on nil return
})
```

### Finding a Single Record

```go
var user User
found, err := xgorm.FindOne(db.Where("id = ?", 1), &user)
if err != nil {
    panic(err)
}

if !found {
    fmt.Println("User not found")
} else {
    fmt.Printf("Found user: %v\n", user)
}
```

## Plugins

### Logger Plugin

Structured logging with slow query detection:

```go
loggerPlugin := logger.New(
    slog.Default(),
    slog.LevelInfo,
    200,  // slow query threshold in milliseconds
)
db.Use(loggerPlugin)
```

Features:
- Logs all database operations (CREATE, QUERY, UPDATE, DELETE, RAW)
- Records SQL, table name, duration, rows affected
- Warns on slow queries exceeding threshold
- Error logging for failed operations

### Metrics Plugin

Track database operation metrics:

```go
metricPlugin := metric.New()
db.Use(metricPlugin)

// ... perform operations ...

// Get metrics
m := metricPlugin.Metrics()
fmt.Printf("Total queries: %d\n", m.TotalQueryCount)
fmt.Printf("Total errors: %d\n", m.TotalErrorCount)
fmt.Printf("Total duration: %v\n", m.TotalDuration)
fmt.Printf("CREATE count: %d\n", m.CreateQueryCount)
```

Metrics tracked:
- Query count (total and per operation)
- Error count (total and per operation)
- Query duration (total and per operation)
- Thread-safe with `Metrics()` returning a snapshot

### Tracer Plugin

Distributed tracing with OpenTelemetry:

```go
import (
    "go.opentelemetry.io/otel"
    tracerplugin "github.com/codesjoy/pkg/basic/xgorm/plugin/tracer"
)

tracer := otel.Tracer("gorm")
tracerPlugin := tracerplugin.New(tracer)
db.Use(tracerPlugin)
```

Spans created for:
- Each database operation
- Includes attributes: operation type, table name, SQL statement
- Records errors as span status

## Configuration Options

```go
db, err := xgorm.New(
    dialector,
    xgorm.WithSlogLogger(logger),
    xgorm.WithLoggerConfig(gormlogger.Info, 100*time.Millisecond, false),

    xgorm.WithTracer(tracer),
    xgorm.WithMeter(meter),

    xgorm.WithDryRun(false),
    xgorm.WithSkipDefaultTransaction(false),

    xgorm.WithMaxIdleConns(10),
    xgorm.WithMaxOpenConns(100),
    xgorm.WithConnMaxLifetime(3600),  // seconds

    // Table sharding
    xgorm.WithSharding(
        sharding.Config{
            ShardingKey:         "user_id",
            NumberOfShards:      64,
            PrimaryKeyGenerator: sharding.PKSnowflake,
        },
        &Order{},
    ),

    // Multi-database routing (global rule + table-specific rule)
    xgorm.WithDBResolver(dbresolver.Config{
        Sources:  []gorm.Dialector{postgres.Open(primaryDSN)},
        Replicas: []gorm.Dialector{postgres.Open(replicaDSN)},
    }),
    xgorm.WithDBResolver(dbresolver.Config{
        Sources:  []gorm.Dialector{postgres.Open(orderPrimaryDSN)},
        Replicas: []gorm.Dialector{postgres.Open(orderReplicaDSN)},
    }, &Order{}),
    xgorm.WithDBResolverConnPool(
        20,                 // max idle
        200,                // max open
        time.Hour,          // conn max lifetime
        30*time.Minute,     // conn max idle time
    ),
)
```

## Sharding & Multi-Database

### Table Sharding

```go
db, err := xgorm.New(
    postgres.Open(dsn),
    xgorm.WithSharding(
        sharding.Config{
            ShardingKey:         "user_id",
            NumberOfShards:      16,
            PrimaryKeyGenerator: sharding.PKSnowflake,
        },
        &Order{},
    ),
)
```

### Multi-Database Read/Write Routing

```go
db, err := xgorm.New(
    postgres.Open(primaryDSN),
    xgorm.WithDBResolver(dbresolver.Config{
        Sources:  []gorm.Dialector{postgres.Open(primaryDSN)},
        Replicas: []gorm.Dialector{postgres.Open(replicaDSN)},
    }),
)
```

### Combine Sharding + DBResolver

```go
db, err := xgorm.New(
    postgres.Open(primaryDSN),
    xgorm.WithSharding(sharding.Config{
        ShardingKey:         "user_id",
        NumberOfShards:      16,
        PrimaryKeyGenerator: sharding.PKSnowflake,
    }, &Order{}),
    xgorm.WithDBResolver(dbresolver.Config{
        Sources:  []gorm.Dialector{postgres.Open(primaryDSN)},
        Replicas: []gorm.Dialector{postgres.Open(replicaDSN)},
    }),
)
```

Notes:
- Missing sharding key on sharded tables returns `sharding.ErrMissingShardingKey`.
- `PrepareStmt=true` is not supported together with sharding.

### Breaking Changes

- Removed deprecated options: `WithLogger`, `WithLogLevel`, `WithSlowThreshold`, `WithMetrics`
- Use `WithLoggerConfig` + `WithSlogLogger` for logging
- Use `WithMeter` for OpenTelemetry metrics registration

## Error Handling

The package provides custom error types:

```go
import (
    "github.com/codesjoy/pkg/basic/xgorm"
    "errors"
)

// Check for specific errors
result, err := xgorm.WrapPageQuery(db, pp, &users)
if err != nil {
    if xgorm.IsPaginationError(err) {
        var pgErr *xgorm.PaginationError
        errors.As(err, &pgErr)
        fmt.Printf("Pagination failed in %s: %v\n",
            pgErr.Operation, pgErr.Err)
    }
    if xgorm.IsInvalidSliceType(err) {
        fmt.Println("Invalid slice type provided")
    }
}
```

Available error types:
- `ErrInvalidSliceType` - Invalid input type
- `ErrInvalidModel` - Invalid model
- `ErrTransactionNotActive` - No active transaction
- `PaginationError` - Pagination operation failed
- `TransactionError` - Transaction operation failed

## Context Utilities

Type-safe context keys for transaction management:

```go
// Add transaction to context
ctx := xgorm.WithTransaction(ctx, tx)

// Get transaction from context
tx := xgorm.TransactionFromContext(ctx)

// Check if transaction exists
if xgorm.HasTransaction(ctx) {
    // Use transaction
}
```

## Testing

Run unit tests:

```bash
# From repository root
make MODULES="basic/xgorm" test

make MODULES="basic/xgorm" test.race

make MODULES="basic/xgorm" coverage

# Or run directly inside module
cd basic/xgorm && GOWORK=off go test ./...
```

Run integration tests (Docker-backed PostgreSQL):

```bash
cd basic/xgorm
GOWORK=off go test -tags=integration ./testing/integration -v
```

Notes:
- `make MODULES="basic/xgorm" test` remains unit-test focused.
- Integration tests require Docker Desktop (or another compatible Docker daemon).
- Integration suite covers pagination/transaction and PostgreSQL dbresolver + sharding routing scenarios.

## Performance

The package is optimized for performance:

- Minimal allocations in hot paths
- Thread-safe metrics with efficient RWMutex
- Zero-cost abstraction for type-safe context
- Efficient pagination with count optimization

## Best Practices

1. **Always handle errors**: All errors from `rowSliceElement` are now properly propagated
2. **Use transactions**: For multi-step operations requiring atomicity
3. **Set appropriate page sizes**: Default is 100, adjust based on your needs
4. **Enable slow query logging**: Helps identify performance issues
5. **Use `NoCount: true`** for large datasets when count is not needed
6. **Reuse transaction contexts**: Pass through call stacks for nested operations

## Examples

Runnable examples live under `examples/`:

- `examples/postgres`: PostgreSQL example with `WrapPageQuery`, `Transaction`, and `FindOne`

Run from `basic/xgorm/examples`:

```bash
GOWORK=off go run ./postgres
```

See `basic/xgorm/examples/README.md` for Docker setup and environment variables.

## License

See package root for license information.
