# aipsql

A Go library for implementing Google API Improvement Proposals (AIPs) in SQL-based applications. Provides utilities for AIP-160 (filtering) and AIP-132 (sorting) with advanced query optimization features.

## Features

- **AIP-160 Filtering**: Parse and generate SQL WHERE clauses from Google-style filter expressions
- **AIP-132 Sorting**: Parse and generate SQL ORDER BY clauses
- **Match Modes**: Control how text searches translate to SQL (exact, prefix, fulltext, contains)
- **Composite Index Optimization**: Automatic reordering of WHERE conditions to match database indexes
- **SQL Dialect Support**: Optimized SQL generation for PostgreSQL, MySQL, and generic SQL
- **Seek Pagination**: Efficient pagination using indexed columns instead of OFFSET
- **Type Safety**: Builder pattern for compile-time safety

## Installation

```bash
go get github.com/codesjoy/pkg/basic/aipsql
```

## Quick Start

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

    // Parse filter expression
    filter, err := aipsql.ParseFilter("status=\"active\" AND name:\"John\"")
    if err != nil {
        panic(err)
    }

    // Generate SQL WHERE clause
    sql, params, err := table.WhereClauseWithOptions(filter, "p", aipsql.WhereClauseOptions{
        Dialect:                           aipsql.SQLDialectGeneric,
        EnableCompositeIndexOptimization: true,
    })
    if err != nil {
        panic(err)
    }

    fmt.Println("WHERE", sql)
    fmt.Println("Params:", params)
    // Output: WHERE ((status = @p0) AND (name LIKE @p1))
    // Params: [{p0 active} {p1 John%}]
}
```

## Documentation

For comprehensive documentation, see:

- **[User Guide](docs/guide.md)** - Complete usage guide with core concepts
- **[Examples](docs/examples.md)** - Real-world usage examples
- **[API Reference](docs/api.md)** - Full API documentation
- **[GORM Adapter](adapter/gorm/README.md)** - Execute generated clauses with `gorm`
- **[Performance](docs/performance.md)** - Performance optimization strategies
- **[Security](docs/security.md)** - Security best practices
- **[Documentation Hub](docs/README.md)** - Overview of all documentation

## Usage Examples

### Basic Filtering

```go
table := aipsql.NewTable().WithColumns(
    aipsql.NewColumn().
        WithFieldPath("status").
        WithDatabaseName("status").
        Filterable().
        Build(),
).Build()

filter, _ := aipsql.ParseFilter("status=\"active\"")
sql, params, _ := table.WhereClause(filter, "p")
// WHERE (status = @p0), [{p0 active}]
```

### Autocomplete with Prefix Matching

```go
nameColumn := aipsql.NewColumn().
    WithFieldPath("name").
    WithDatabaseName("name").
    WithMatchModes(aipsql.MatchModePrefix).
    Filterable().
    Build()

table := aipsql.NewTable().WithColumns(nameColumn).Build()
filter, _ := aipsql.ParseFilter("name:\"John\"")
sql, params, _ := table.WhereClauseWithOptions(filter, "p", aipsql.WhereClauseOptions{
    Dialect: aipsql.SQLDialectGeneric,
})
// WHERE (name LIKE @p0), [{p0 John%}]
```

### Full-Text Search with PostgreSQL

```go
contentColumn := aipsql.NewColumn().
    WithFieldPath("content").
    WithDatabaseName("content").
    WithMatchModes(aipsql.MatchModeFullText, aipsql.MatchModeContains).
    Filterable().
    Build()

table := aipsql.NewTable().WithColumns(contentColumn).Build()
filter, _ := aipsql.ParseFilter("content:\"machine learning\"")
opts := aipsql.WhereClauseOptions{
    Dialect: aipsql.SQLDialectPostgres,
}
sql, params, _ := table.WhereClauseWithOptions(filter, "p", opts)
// WHERE (to_tsvector('simple', content) @@ websearch_to_tsquery('simple', @p0))
```

### Key-Value Labels

```go
labelsColumn := aipsql.NewColumn().
    WithFieldPath("labels").
    WithDatabaseName("labels").
    KeyValue().
    WithMatchModes(aipsql.MatchModeExact).
    Filterable().
    Build()

table := aipsql.NewTable().WithColumns(labelsColumn).Build()
filter, _ := aipsql.ParseFilter("labels.environment=\"production\"")
sql, params, _ := table.WhereClause(filter, "p")
// WHERE (EXISTS (SELECT key, value FROM UNNEST(labels) WHERE key = @p0 AND value = @p1))
```

### Execute with GORM

```go
import (
    aipsql "github.com/codesjoy/pkg/basic/aipsql"
    aipsqlgorm "github.com/codesjoy/pkg/basic/aipsql/adapter/gorm"
)

filter, _ := aipsql.ParseFilter(`status="active" AND name:"Al"`)
whereSQL, params, _ := table.WhereClause(filter, "p_")

var users []User
err := aipsqlgorm.ApplyWhere(
    db.Model(&User{}),
    whereSQL,
    params,
).Order("created_at DESC, id DESC").Find(&users).Error

// See adapter/gorm/README.md for complete integration patterns.
```

### Execute QueryPlan with GORM

```go
planner, _ := aipsql.NewQueryPlanner(aipsql.QueryPlannerOptions{
    Dialect: aipsql.SQLDialectPostgres,
})

plan, _ := planner.PlanList(ctx, "orders", aipsql.QueryRequest{
    Filter:   `status="active"`,
    OrderBy:  "created_at DESC",
    PageSize: 20,
})

var orders []Order
err := aipsqlgorm.ApplyPlan(db.Model(&Order{}), plan).Find(&orders).Error

// See adapter/gorm/README.md for complete integration patterns.
```

### Seek Pagination

```go
// First page
// SELECT * FROM orders ORDER BY created_at DESC, id DESC LIMIT 10

// Get last row values: created_at='2024-01-15T10:30:00Z', id=12345
token := aipsql.PaginationToken{
    Values: []interface{}{"2024-01-15T10:30:00Z", 12345},
}

// Next page uses seek predicate
// WHERE (created_at < @seek_cmp_0 OR (created_at = @seek_eq_0 AND id < @seek_cmp_1))
// ORDER BY created_at DESC, id DESC LIMIT 10
```

## Testing

Run tests:
```bash
cd basic/aipsql
go test ./...
```

Run benchmarks:
```bash
go test -bench=. -benchmem
```

Run with coverage:
```bash
go test -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Related Documentation

- **[AIP-160](https://google.aip.dev/160)** - Filtering specification
- **[AIP-132](https://google.aip.dev/132)** - Sort order specification

## License

Copyright 2024 The codesjoy Authors. Licensed under the Apache License, Version 2.0.
