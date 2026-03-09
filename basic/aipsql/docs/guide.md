# aipsql User Guide

Complete guide for using the aipsql package to implement Google API Improvement Proposals (AIPs) in SQL-based applications.

## Table of Contents

1. [Quick Reference](#quick-reference)
2. [Core Concepts](#core-concepts)
3. [Usage Patterns](#usage-patterns)
4. [Advanced Features](#advanced-features)
5. [Best Practices](#best-practices)

## Quick Reference

### Common Patterns

**Basic filtering**:
```go
filter, _ := aipsql.ParseFilter("status=\"active\"")
sql, params, _ := table.WhereClause(filter, "p")
```

**Sorting**:
```go
orderBy, _ := aipsql.ParseOrderBy("created_at DESC")
sql, params, _ := table.OrderByClause(orderBy, "p")
```

**With optimization**:
```go
opts := aipsql.WhereClauseOptions{
    Dialect:                           aipsql.SQLDialectPostgres,
    EnableCompositeIndexOptimization: true,
}
sql, params, _ := table.WhereClauseWithOptions(filter, "p", opts)
```

### Filter Syntax Quick Reference

| Operator | Description | Example |
|----------|-------------|---------|
| `=` | Equal | `status="active"` |
| `!=` | Not equal | `status!="pending"` |
| `<` `<=` `>` `>=` | Comparison | `created_at>"2024-01-01"` |
| `:` | Has (text search) | `name:"John"` |
| `AND` / `OR` | Logical | `(status="active" OR role="admin")` |

### OrderBy Syntax Quick Reference

```go
// Single field
"created_at DESC"

// Multiple fields
"name ASC, created_at DESC"

// With default tie-breaker
"created_at DESC, id DESC"
```

## Core Concepts

### Table and Column Definition

Use the builder pattern to define table schema:

```go
table := aipsql.NewTable().WithColumns(
    aipsql.NewColumn().
        WithFieldPath("status").
        WithDatabaseName("status").
        WithMatchModes(aipsql.MatchModeExact).
        Filterable().
        Sortable().
        Build(),
    aipsql.NewColumn().
        WithFieldPath("displayName").
        WithDatabaseName("display_name").
        WithMatchModes(aipsql.MatchModePrefix, aipsql.MatchModeExact).
        Filterable().
        Sortable().
        Build(),
).Build()

// Add composite indexes
table.CompositeIndexes = []aipsql.CompositeIndex{
    {
        Name:    "idx_status_name",
        Columns: []string{"status", "display_name"},
    },
}
```

**Column Options**:
- `WithFieldPath()` - API field path (required)
- `WithDatabaseName()` - DB column name (required)
- `WithMatchModes()` - Text search modes (defaults to contains)
- `Filterable()` - Enable WHERE filtering
- `Sortable()` - Enable ORDER BY
- `FilterableImplicitly()` - Enable implicit search
- `KeyValue()` - Key-value column (labels/tags)

### Match Modes

Match modes control how the `:` (has) operator translates to SQL:

| Mode | SQL Generated | Performance | Use Case |
|------|---------------|-------------|----------|
| **Exact** | `column = @param` | ~1ms (fastest) | Status codes, IDs |
| **Prefix** | `column LIKE @param` | ~5ms | Autocomplete, names |
| **FullText** | `to_tsvector(...) @@ ...` | ~10ms | Document content |
| **Contains** | `column LIKE %@param%` | ~500ms | Small tables only |

**Configuration**:
```go
// Prefer index-friendly modes
column.WithMatchModes(
    aipsql.MatchModePrefix,   // Try first
    aipsql.MatchModeExact,    // Fallback
)

// Fulltext with fallback
column.WithMatchModes(
    aipsql.MatchModeFullText,  // Postgres/MySQL
    aipsql.MatchModeContains,  // Generic fallback
)
```

**When to use each**:
- **Exact**: Enums, status codes, exact lookups
- **Prefix**: Autocomplete, name search, search-as-you-type
- **FullText**: Long text, natural language queries (requires Postgres/MySQL)
- **Contains**: Small tables (<10K rows), fallback only

### Composite Index Optimization

Automatically reorders WHERE conditions to match database indexes for 10x-100x performance improvement.

**Setup**:
```go
table.CompositeIndexes = []aipsql.CompositeIndex{
    {Name: "idx_status_user_created", Columns: []string{"status", "user_id", "created_at"}},
}

opts := aipsql.WhereClauseOptions{
    EnableCompositeIndexOptimization: true,
}
```

**Example**:
```
Input:  user_id=123 AND created_at>"2024-01-01" AND status="active"
Output: status = @p2 AND user_id = @p0 AND created_at > @p1
        ^^^^^^ equality first (index order)
                ^^^^^^^ equality second
                                   ^^^^^^^^^^^^^^^ range last

Performance: 50x faster (150ms → 3ms)
```

**Key rules**:
- Use database column names (not field paths)
- Equality conditions before range conditions
- Match database index order

### Seek Pagination

Efficient cursor-based pagination for large datasets:

```go
// First page
request := aipsql.QueryRequest{
    Filter:   "status=\"active\"",
    PageSize: 20,
}
plan, _ := planner.PlanList(ctx, request)

// Execute the current page, then derive the next token from the actual rows.
rows := fetchRows(plan)
nextPageToken, _ := plan.NextPageToken(rows)

// Second page
request.PageToken = nextPageToken
plan, _ = planner.PlanList(ctx, request)
```

**Performance**: Consistent ~5ms vs OFFSET (degrades to 5000ms at page 10000)

**Best practices**:
- Always include unique tie-breaker (e.g., `id`)
- Create indexes on sort columns
- Limit to 2-3 sort fields

## Usage Patterns

### Basic Filtering

```go
// Single condition
filter, _ := aipsql.ParseFilter("status=\"active\"")
sql, params, _ := table.WhereClause(filter, "p")

// Multiple conditions (AND)
filter, _ = aipsql.ParseFilter("status=\"active\" AND name:\"John\"")

// Multiple conditions (OR)
filter, _ = aipsql.ParseFilter("(status=\"active\" OR status=\"pending\")")

// Complex
filter, _ = aipsql.ParseFilter("(status=\"active\" OR role=\"admin\") AND name:\"John\"")
```

### Sorting and Pagination

```go
// Parse order by
orderBy, _ := aipsql.ParseOrderBy("created_at DESC, id DESC")

// Generate clause
sql, params, _ := table.OrderByClause(orderBy, "p")

// Merge with default
userOrder, _ := aipsql.ParseOrderBy("name ASC")
defaultOrder, _ := aipsql.ParseOrderBy("id DESC")
mergedOrder := aipsql.MergeWithDefaultOrder(userOrder, defaultOrder)
```

### Full-Text Search

**PostgreSQL**:
```go
column.WithMatchModes(aipsql.MatchModeFullText)

opts := aipsql.WhereClauseOptions{
    Dialect: aipsql.SQLDialectPostgres,
}

// Generates: to_tsvector('simple', content) @@ websearch_to_tsquery('simple', @p0)
```

**MySQL**:
```go
opts.Dialect = aipsql.SQLDialectMySQL
// Generates: MATCH(content) AGAINST (@p0 IN BOOLEAN MODE)
```

**Database index**:
```sql
-- PostgreSQL
CREATE INDEX idx_content_fts ON documents USING GIN(to_tsvector('simple', content));

-- MySQL
ALTER TABLE documents ADD FULLTEXT INDEX idx_content_fts (content);
```

### Key-Value Labels

Flexible metadata filtering (Kubernetes-style labels, AWS tags):

```go
column.WithMatchModes(aipsql.MatchModeExact).
    KeyValue().
    Filterable()

// Filter: labels.environment="production"
// SQL: WHERE EXISTS (SELECT 1 FROM UNNEST(labels) WHERE key = @p0 AND value = @p1)

// Multiple labels
filter, _ := aipsql.ParseFilter("labels.env=\"prod\" AND labels.team=\"platform\"")
```

**Database schema** (PostgreSQL):
```sql
CREATE TABLE resources (
    labels JSONB
);

CREATE INDEX idx_labels ON resources USING GIN(labels);
```

### QueryPlanner Usage

High-level API for complete query generation:

```go
planner, _ := aipsql.NewQueryPlanner(aipsql.TableSpec{
    Table:               ordersTable,
    DefaultOrder:        []aipsql.OrderBy{{FieldPath: aipsql.NewFieldPath("created_at"), Descending: true}},
    TieBreakerFieldPath: aipsql.NewFieldPath("id"),
}, aipsql.QueryPlannerOptions{
    Dialect:                          aipsql.SQLDialectPostgres,
    EnableCompositeIndexOptimization: true,
    DefaultPageSize:                  20,
    MaxPageSize:                      100,
})

// Plan query
plan, _ := planner.PlanList(ctx, aipsql.QueryRequest{
    Filter:    "status=\"active\"",
    OrderBy:   "name ASC",
    PageSize:  20,
    PageToken: token,  // From previous page
})

// Execute
query := fmt.Sprintf(
    "SELECT id, name, created_at FROM orders WHERE %s ORDER BY %s LIMIT %d",
    plan.WhereClause,
    plan.OrderByClause,
    plan.Limit,
)
rows, _ := db.Query(query, aipsql.ParamValues(plan.Parameters)...)
```

### QueryPlanner Fragments + Offset

Use `PlanList` when you want final clauses and will stitch the base query yourself:

```go
plan, _ := planner.PlanList(ctx, aipsql.QueryRequest{
    Filter:         `status="active"`,
    PageSize:       20,
    PaginationMode: aipsql.PaginationModeOffset,
    PageToken:      aipsql.EncodeOffsetPageToken(40),
})

query := fmt.Sprintf(
    "SELECT id, created_at FROM orders WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
    plan.WhereClause,
    plan.OrderByClause,
    plan.Limit,
    plan.Offset,
)
_ = query
```

## Advanced Features

### Implicit Filtering

Search across multiple fields without specifying field names:

```go
column.WithMatchModes(aipsql.MatchModePrefix).
    FilterableImplicitly()

// Filter: "\"John\""  (no field specified)
// SQL: WHERE ((name LIKE @p0) OR (email LIKE @p1))
```

### Custom Dialects

Extend with custom SQL generation:

```go
// Add dialect-specific handling
opts := aipsql.WhereClauseOptions{
    Dialect: aipsql.SQLDialectPostgres,  // or MySQL, Generic
}
```

### Performance Tuning

**1. Use appropriate match modes**:
```go
// Good: Index-friendly
column.WithMatchModes(aipsql.MatchModePrefix)

// Bad: Full table scan
column.WithMatchModes(aipsql.MatchModeContains)  // 500x slower!
```

**2. Create indexes**:
```sql
-- For prefix/exact
CREATE INDEX idx_name ON users(name);

-- For fulltext (Postgres)
CREATE INDEX idx_content_fts ON documents USING GIN(to_tsvector('simple', content));

-- For composite
CREATE INDEX idx_status_user_created ON orders(status, user_id, created_at);
```

**3. Enable optimization selectively**:
```go
// Large tables: enable
if tableName == "orders" {
    opts.EnableCompositeIndexOptimization = true
}

// Small tables: disable (overhead not worth it)
```

## Best Practices

### 1. Index Strategy

**DO**:
```go
// Define indexes matching query patterns
table.CompositeIndexes = []aipsql.CompositeIndex{
    {Name: "idx_status_created", Columns: []string{"status", "created_at"}},
}

// Enable optimization
opts.EnableCompositeIndexOptimization = true
```

**DON'T**:
```go
// Don't use contains for large tables
column.WithMatchModes(aipsql.MatchModeContains)  // 500ms query time!
```

### 2. Match Mode Selection

**DO**:
```go
// Prefer index-friendly modes
column.WithMatchModes(
    aipsql.MatchModePrefix,  // Fast index scan
    aipsql.MatchModeExact,   // Direct lookup
)
```

**DON'T**:
```go
// Don't use fulltext for short text
column.WithMatchModes(aipsql.MatchModeFullText)  // Overkill for names
```

### 3. Security

**DO**:
```go
// Always parameterize queries
sql, params, _ := table.WhereClauseWithOptions(filter, "p", opts)
db.Query(sql, aipsql.ParamValues(params)...)
```

**DON'T**:
```go
// Never concatenate user input
query := "SELECT * WHERE name = '" + userName + "'"  // SQL injection!
```

### 4. Error Handling

**DO**:
```go
filter, err := aipsql.ParseFilter(userInput)
if err != nil {
    return fmt.Errorf("invalid filter: %w", err)
}

sql, params, err := table.WhereClauseWithOptions(filter, "p", opts)
if err != nil {
    return fmt.Errorf("query generation failed: %w", err)
}
```

**DON'T**:
```go
// Don't ignore errors
filter, _ := aipsql.ParseFilter(userInput)  // May panic!
```

### 5. Pagination

**DO**:
```go
// Use seek pagination for large datasets
token := aipsql.SeekPageToken{SortValues: values}
encoded := token.Encode()

nextToken, _ := aipsql.ParseSeekPageToken(encoded)
```

**DON'T**:
```go
// Don't use OFFSET for large page numbers
query := fmt.Sprintf("SELECT * OFFSET %d", page * size)  // Slow!
```

### 6. Configuration

**DO**:
```go
// Configure per-table defaults
planner.RegisterTable("orders", &aipsql.TableSpec{
    Table:               ordersTable,
    DefaultOrder:        "created_at DESC, id DESC",
    TieBreakerFieldPath: "id",
    MaxPageSize:         100,
})
```

**DON'T**:
```go
// Don't duplicate configuration across every request
```

## Summary

The aipsql package provides:

- **AIP-160 Filtering**: Standard Google API filtering
- **AIP-132 Sorting**: Standard sorting with tie-breakers
- **Query Optimization**: Composite index matching, seek pagination
- **SQL Dialect Support**: PostgreSQL, MySQL, Generic
- **Security**: Built-in SQL injection prevention

**Key takeaways**:
- Define tables with builder pattern
- Choose match modes based on use case
- Create database indexes for performance
- Enable optimization for large tables
- Use QueryPlanner for complete query generation
- Always handle errors and parameterize queries

For more details:
- [Examples](examples.md) - Real-world usage examples
- [API Reference](api.md) - Complete API documentation
- [Performance](performance.md) - Performance characteristics
- [Security](security.md) - Security best practices
