# Snowflake

[![GoDoc](https://pkg.go.dev/badge/github.com/codesjoy/pkg/basic/snowflake)](https://pkg.go.dev/github.com/codesjoy/pkg/basic/snowflake)

A high-performance, distributed ID generator based on the Snowflake algorithm, with support for multiple worker allocation strategies and time drift handling.

---

## Overview

This package provides a robust implementation of the Snowflake ID generation algorithm with the following features:

- **Distributed ID Generation**: Generate unique, roughly-sorted IDs across multiple nodes
- **Time Drift Handling**: Automatically handles system clock backward adjustments
- **Pluggable Worker Allocation**: Support for static and database-backed worker allocation
- **High Performance**: Optimized for high-throughput scenarios with minimal lock contention
- **Flexible Configuration**: Customize bit allocation, base time, and sequence parameters

---

## Installation

```bash
go get github.com/codesjoy/pkg/basic/snowflake
```

---

## Quick Start

### Basic Usage with Static Worker

```go
package main

import (
    "fmt"
    "github.com/codesjoy/pkg/basic/snowflake"
    "github.com/codesjoy/pkg/basic/snowflake/worker/static"
)

func main() {
    // Create a static worker
    staticWorker, err := static.NewWorker(&static.Config{
        WorkerID:          1,
        WorkerIDBitLength: 6,
    })
    if err != nil {
        panic(err)
    }

    // Create Snowflake generator
    cfg := snowflake.NewConfig().WithWorker(staticWorker)

    sf, err := snowflake.NewSnowflake(cfg)
    if err != nil {
        panic(err)
    }

    // Generate IDs
    for i := 0; i < 10; i++ {
        id := sf.FetchID()
        fmt.Println(id)
    }
}
```

### Usage with GORM Worker (Database-Backed)

```go
package main

import (
    "fmt"
    "github.com/codesjoy/pkg/basic/snowflake"
    "github.com/codesjoy/pkg/basic/snowflake/worker/gorm"
    "gorm.io/driver/mysql"
    "gorm.io/gorm"
)

func main() {
    // Setup database connection
    db, err := gorm.Open(mysql.Open("dsn"), &gorm.Config{})
    if err != nil {
        panic(err)
    }

    // Create GORM worker allocator
    cfg := &gorm.Config{
        WorkerIDBitLength: 6,
        Business:          "my-app",
        DBName:            "production",
    }
    cfg.WithDB(db)

    worker, err := gorm.NewWorker(cfg)
    if err != nil {
        panic(err)
    }

    // Create Snowflake generator
    sfCfg := snowflake.NewConfig().WithWorker(worker)

    sf, err := snowflake.NewSnowflake(sfCfg)
    if err != nil {
        panic(err)
    }

    // Generate IDs
    id := sf.FetchID()
    fmt.Println(id)

    // Release worker when done
    sf.ReleaseWorkerID()
}
```

---

## Configuration

### Config Structure

```go
type Config struct {
    BaseTime         int64 // Epoch timestamp for ID generation (default: 2020-02-19 18:20:02 UTC)
    SeqBitLength     byte  // Number of bits for sequence (1-12, default: 12)
    MaxSeqNumber     int64 // Maximum sequence number (auto-calculated if 0)
    MinSeqNumber     int64 // Minimum sequence number (default: 5, min: 5)
    TopOverCostCount int   // Maximum over-cost count in one term (default: 2000)
    WorkerName       string // Worker name (default: "static")
}
```

`NewSnowflake` applies defaults for zero-value fields (`BaseTime`, `SeqBitLength`, `MinSeqNumber`, `TopOverCostCount`), so `&snowflake.Config{}` is valid when a worker is set.

### Default Config Helper

```go
cfg := snowflake.NewConfig().WithWorker(worker)
```

### Builder Pattern

The package supports a fluent builder pattern for configuration:

```go
cfg := snowflake.NewConfig().
    WithWorker(worker).
    WithBaseTime(snowflake.BaseTime2024()).
    WithSeqBitLength(12).
    WithMinSeqNumber(10).
    WithTopOverCostCount(5000)
```

### Base Time Helpers

Convenience functions for setting the base time:

```go
// Use 2020-02-19 18:20:02 UTC as base time
cfg.WithBaseTime(snowflake.BaseTime2020())

// Use 2024-01-01 00:00:00 UTC as base time
cfg.WithBaseTime(snowflake.BaseTime2024())

// Use custom base time
cfg.WithBaseTime(snowflake.BaseTimeCustom(time.Now()))
```

---

## Architecture

### ID Structure

A Snowflake ID is composed of three parts:

```
| Timestamp | Worker ID | Sequence |
|-----------|-----------|----------|
| Variable  | Configurable | Configurable |
```

- **Timestamp**: Milliseconds since base time
- **Worker ID**: Unique identifier for the generating instance
- **Sequence**: Counter for IDs generated within the same millisecond

### Bit Allocation

The total bit length for Worker ID and Sequence must not exceed 22 bits:

- **Minimum sequence bits**: 1
- **Maximum sequence bits**: 12
- **Minimum worker ID bits**: 1
- **Maximum worker ID bits**: 10

Example configurations:

| Worker ID Bits | Sequence Bits | Max Workers | Max IDs/ms | Total IDs/second |
|----------------|---------------|-------------|------------|------------------|
| 6              | 12            | 64          | 4095       | ~4 million       |
| 10             | 8             | 1024        | 255        | ~250 thousand    |
| 8              | 10            | 256         | 1023       | ~1 million       |

---

## Worker Allocation Strategies

### Static Worker

Best for single-process applications or when you can manually assign unique worker IDs.

**Pros**:
- No database dependency
- Fastest ID generation
- Simple setup

**Cons**:
- Manual worker ID management
- No coordination between processes

**Example**:
```go
worker, _ := static.NewWorker(&static.Config{
    WorkerID:          1,
    WorkerIDBitLength: 6,
})
```

### GORM Worker

Best for distributed systems requiring automatic worker allocation.

**Pros**:
- Automatic worker allocation
- Worker sharing across processes
- Time drift persistence

**Cons**:
- Database dependency
- Slightly slower due to database round-trips

**Example**:
```go
cfg := &gorm.Config{
    WorkerIDBitLength: 6,
    Business:          "my-app",
}
cfg.WithDB(db)

worker, _ := gorm.NewWorker(cfg)
```

---

## Performance Characteristics

### Throughput

Benchmark results on typical hardware:

| Scenario                  | Operations/Second |
|---------------------------|-------------------|
| Single goroutine          | ~2-3 million      |
| 4 concurrent goroutines   | ~5-7 million      |
| 16 concurrent goroutines  | ~10-15 million    |

### Optimization Tips

1. **Use appropriate bit allocation**: Balance between max workers and max sequence
2. **Minimize lock contention**: Use separate workers per goroutine if possible
3. **Database connection pooling**: Essential for GORM worker performance
4. **Batch operations**: Generate IDs in batches when possible

---

## Error Handling

### Common Errors

**"min seq number must be greater than or equal to 5"**
- The minimum sequence number must be at least 5
- Solution: Increase `MinSeqNumber` to 5 or higher

**"worker id bit length + seq bit length must be less than or equal to 22"**
- Total bit allocation exceeds maximum
- Solution: Reduce either `WorkerIDBitLength` or `SeqBitLength`

**"static worker does not support updating over last time"**
- Static worker cannot persist time drift data
- Solution: Use GORM worker for time drift handling

---

## Advanced Features

### Time Drift Handling

The algorithm automatically handles system clock backward adjustments:

- **Over-cost mode**: When sequence overflows, advance time by 1ms
- **Turn-back mode**: When clock goes backward, use reserved sequence numbers

### Worker Release

Always release workers when shutting down:

```go
defer sf.ReleaseWorkerID()
```

For GORM workers, this marks the worker ID as available for reuse.

---

## Testing

Run the test suite:

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run benchmarks
go test -bench=. -benchmem

# Race detection
go test -race ./...
```

---

## Examples

See the [examples](./examples/) directory for complete working examples:

- `examples/basic/`: Basic usage patterns
- `examples/gorm/`: GORM worker integration
- `examples/custom/`: Custom configuration

---

## License

Copyright 2022 The codesjoy Authors.

Licensed under the Apache License, Version 2.0.
