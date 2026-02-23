# aipsql Documentation

Welcome to the comprehensive documentation for the `aipsql` package - a Go library for implementing Google API Improvement Proposals (AIPs) in SQL-based applications.

## Overview

The `aipsql` package provides production-ready utilities for:

- **[AIP-160](https://google.aip.dev/160)** - Standard filtering for list APIs
- **[AIP-132](https://google.aip.dev/132)** - Standard sorting for list APIs
- **Query Optimization** - Composite index matching and Seek pagination
- **SQL Dialect Support** - PostgreSQL, MySQL, and Generic SQL
- **Security** - Built-in SQL injection prevention

## Key Features

### 1. Index-Friendly Query Generation
Four match modes that leverage different index types:
- **Exact Match**: B-tree index, ~1ms query time
- **Prefix Match**: B-tree index, ~5ms query time
- **Full-Text Search**: Full-text index, ~10ms query time
- **Contains Match**: Full scan, ~500ms query time (fallback)

### 2. Composite Index Optimization
Automatically reorders WHERE conditions to match database indexes:
- 10x-100x performance improvement on large tables
- Intelligent index selection based on query patterns
- Equality before range condition ordering

### 3. Seek Pagination
Efficient cursor-based pagination:
- O(log n) vs O(n) for OFFSET pagination
- Uses indexed columns for direct positioning
- Supports multi-field sorting

### 4. SQL Injection Prevention
All user input is parameterized:
- Column names from table metadata only
- Values bound as parameters
- LIKE special character escaping

### 5. Multi-Dialect Support
Optimized SQL for different databases:
- **PostgreSQL**: `to_tsvector`, `websearch_to_tsquery`, GIN indexes
- **MySQL**: `MATCH...AGAINST`, full-text indexes
- **Generic**: Standard SQL with maximum compatibility

## Quick Start

### Installation

```bash
go get github.com/codesjoy/pkg/basic/aipsql
```

### Basic Usage

```go
package main

import (
    "fmt"
    "github.com/codesjoy/pkg/basic/aipsql"
)

func main() {
    // Define table schema
    table := aipsql.NewTable().WithColumns(
        aipsql.NewColumn().
            WithFieldPath("status").
            WithDatabaseName("status").
            WithMatchModes(aipsql.MatchModeExact).
            Filterable().
            Build(),
        aipsql.NewColumn().
            WithFieldPath("name").
            WithDatabaseName("name").
            WithMatchModes(aipsql.MatchModePrefix, aipsql.MatchModeExact).
            Filterable().
            Sortable().
            Build(),
    ).Build()

    // Add composite index
    table.CompositeIndexes = []aipsql.CompositeIndex{
        {
            Name:    "idx_status_name",
            Columns: []string{"status", "name"},
        },
    }

    // Parse filter expression
    filter, err := aipsql.ParseFilter("status=\"active\" AND name:\"John\"")
    if err != nil {
        panic(err)
    }

    // Generate SQL WHERE clause with optimization
    sql, params, err := table.WhereClauseWithOptions(filter, "p", aipsql.WhereClauseOptions{
        Dialect:                           aipsql.SQLDialectPostgres,
        EnableCompositeIndexOptimization: true,
    })
    if err != nil {
        panic(err)
    }

    fmt.Println("WHERE", sql)
    fmt.Println("Params:", params)
    // Output:
    // WHERE ((status = @p0) AND (name LIKE @p1))
    // Params: [{Name: p0 Value: active} {Name: p1 Value: John%}]
}
```

## Documentation

### Core Documentation

- **[User Guide](guide.md)** - Complete guide for using the aipsql package
- **[Examples](examples.md)** - Real-world usage examples
- **[API Reference](api.md)** - Complete API documentation

### Advanced Topics

- **[Performance](performance.md)** - Performance characteristics and optimization strategies
- **[Security](security.md)** - SQL injection prevention and input validation

### Integrations

- **[GORM Adapter](../adapter/gorm/README.md)** - Apply generated clauses to `*gorm.DB`

## Performance Characteristics

| Operation | Time Complexity | Space Complexity |
|-----------|----------------|------------------|
| Filter Generation | O(n) | O(n) |
| Composite Index Optimization | O(m × k × log(k)) | O(k) |
| Seek Pagination | O(n²) | O(n²) |

Where:
- n = number of conditions/fields
- m = number of indexes
- k = number of index columns

## SQL Dialect Support

| Dialect | Full-Text Search | Key-Value | Seek Pagination |
|---------|-----------------|-----------|-----------------|
| PostgreSQL | ✅ `to_tsvector` | ✅ `UNNEST` | ✅ |
| MySQL | ✅ `MATCH...AGAINST` | ✅ `JSON_TABLE` | ✅ |
| Generic | ❌ (falls back to contains) | ⚠️ (limited) | ✅ |

## Testing

The package includes comprehensive test coverage:

```bash
# Run all tests
cd basic/aipsql
go test ./...

# Run with coverage
go test -coverprofile=coverage.out
go tool cover -html=coverage.out

# Run benchmarks
go test -bench=. -benchmem
```

## License

Copyright 2024 The codesjoy Authors. Licensed under the Apache License, Version 2.0.

## Related Resources

- **[AIP-160: Filtering](https://google.aip.dev/160)** - Standard filtering specification
- **[AIP-132: List Order By](https://google.aip.dev/132)** - Standard sorting specification
- **[AIP-158: Pagination](https://google.aip.dev/158)** - Pagination guidelines
- **[PostgreSQL Full-Text Search](https://www.postgresql.org/docs/current/textsearch.html)**
- **[MySQL Full-Text Search](https://dev.mysql.com/doc/refman/8.0/en/fulltext-search.html)**

## Support

For issues, questions, or contributions, please visit the [GitHub repository](https://github.com/codesjoy/pkg).
