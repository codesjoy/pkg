# Snowflake Examples

This directory contains usage examples for the Snowflake distributed ID generator.

## Overview

Each example is a standalone Go module with its own `go.mod` file, demonstrating different use cases and configurations:

- **[basic](./basic/)** - Basic usage with static worker allocator
- **[custom](./custom/)** - Advanced configuration and performance tuning
- **[gorm](./gorm/)** - Database-backed worker allocation using GORM

## Prerequisites

Go 1.25.7 or later is required.

## Running Examples

### Basic Example

Demonstrates basic Snowflake ID generation with static worker configuration:

```bash
cd examples/basic
go run main.go
```

**Features shown:**
- Default configuration usage
- Custom configuration with different bit lengths
- Custom base time epochs (2020, 2024)
- Builder pattern for fluent configuration

### Custom Example

Demonstrates advanced configuration for different deployment scenarios:

```bash
cd examples/custom
go run main.go
```

**Features shown:**
- High-scale deployment (up to 1024 workers)
- Performance tuning options
- Different bit allocation strategies
- Custom epoch time creation
- Throughput capacity planning

### GORM Example

Demonstrates database-backed worker allocation for distributed systems:

**Note:** Requires MySQL database connection.
The default DSN in `gorm/main.go` is `root:password@tcp(127.0.0.1:3306)/snowflake`; replace it before running.

```bash
cd examples/gorm

# Update database connection string in main.go:
# mysql.Open("user:password@tcp(127.0.0.1:3306)/dbname")

go run main.go
```

**Features shown:**
- Database connection setup
- Worker allocation from database pool
- Concurrent ID generation across multiple workers
- Worker allocation and release lifecycle
- Automatic worker ID reuse

## Database Setup (GORM Example)

For the GORM example, you need to create a MySQL database:

```sql
CREATE DATABASE snowflake;
```

The example will automatically create the required table (`snowflake_workers`) using GORM's AutoMigrate.

## Module Structure

Each example is an independent Go module:

```
examples/
├── basic/
│   ├── go.mod           # Module definition
│   ├── go.sum           # Dependencies
│   └── main.go          # Example code
├── custom/
│   ├── go.mod
│   ├── go.sum
│   └── main.go
└── gorm/
    ├── go.mod
    ├── go.sum
    └── main.go
```

All examples use a local replace directive to reference the parent snowflake package:

```go
replace github.com/codesjoy/pkg/basic/snowflake => ../../
```

## Configuration Quick Reference

### Bit Allocation

| Scenario            | Worker ID Bits | Sequence Bits | Max Workers | IDs/ms |
|---------------------|----------------|---------------|-------------|--------|
| High-scale          | 10             | 12            | 1024        | 4096   |
| Balanced (default)  | 6              | 12            | 64          | 4096   |
| Small cluster       | 4              | 12            | 16          | 4096   |
| Minimal             | 2              | 12            | 4           | 4096   |

### Base Time Options

- `BaseTime2020()` - Epoch: 2020-02-19 18:20:02 UTC
- `BaseTime2024()` - Epoch: 2024-01-01 00:00:00 UTC
- `BaseTimeCustom(time.Time)` - Custom epoch

### Builder Pattern

```go
cfg := snowflake.NewConfig().
    WithWorker(worker).
    WithBaseTime(snowflake.BaseTime2024()).
    WithSeqBitLength(12).
    WithMinSeqNumber(20).
    WithTopOverCostCount(3000)
```

## Performance Considerations

- **Static workers** are faster but require manual worker ID coordination
- **GORM workers** provide automatic allocation but add database latency
- **Higher TopOverCostCount** allows better burst handling
- **Lower MinSeqNumber** reduces initial padding overhead

## Troubleshooting

### Import Errors

If you see import errors, ensure you're running from the example directory (not the parent):

```bash
cd examples/basic   # Correct
go run main.go

# Not from parent directory:
cd examples
go run basic/main.go  # Wrong - will fail to find local modules
```

### Database Connection (GORM)

Make sure MySQL is running and the connection string in `gorm/main.go` is correct:

```go
mysql.Open("username:password@tcp(host:port)/database")
```

## Further Reading

- [Main README](../README.md) - Package documentation
- [Configuration Guide](../README.md#configuration) - Detailed configuration options
- [Worker Allocators](../worker/) - Worker allocator implementations
