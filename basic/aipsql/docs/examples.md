# Usage Examples

Real-world examples demonstrating common aipsql patterns and use cases.

## Table of Contents

1. [Example 1: Basic Filtering](#example-1-basic-filtering)
2. [Example 2: Autocomplete Search](#example-2-autocomplete-search)
3. [Example 3: Full-Text Search](#example-3-full-text-search)
4. [Example 4: Key-Value Labels](#example-4-key-value-labels)
5. [Example 5: Performance Optimization](#example-5-performance-optimization)
6. [Example 6: GORM Integration](#example-6-gorm-integration)

## Example 1: Basic Filtering

Demonstrates AIP-160 filtering for a user listing API.

### Table Definition

```go
var usersTable = aipsql.NewTable().WithColumns(
    aipsql.NewColumn().
        WithFieldPath("id").
        WithDatabaseName("id").
        Filterable().
        Sortable().
        Build(),
    aipsql.NewColumn().
        WithFieldPath("name").
        WithDatabaseName("name").
        WithMatchModes(aipsql.MatchModePrefix, aipsql.MatchModeExact).
        Filterable().
        Sortable().
        Build(),
    aipsql.NewColumn().
        WithFieldPath("status").
        WithDatabaseName("status").
        WithMatchModes(aipsql.MatchModeExact).
        Filterable().
        Sortable().
        Build(),
    aipsql.NewColumn().
        WithFieldPath("role").
        WithDatabaseName("role").
        WithMatchModes(aipsql.MatchModeExact).
        Filterable().
        Build(),
    aipsql.NewColumn().
        WithFieldPath("createdAt").
        WithDatabaseName("created_at").
        Filterable().
        Sortable().
        Build(),
).Build()
```

### Common Filter Patterns

**Active users**:
```go
filter, _ := aipsql.ParseFilter("status=\"active\"")
sql, params, _ := usersTable.WhereClause(filter, "p")
// WHERE (status = @p0)
```

**Users by name prefix**:
```go
filter, _ := aipsql.ParseFilter("name:\"John\"")
sql, params, _ := usersTable.WhereClause(filter, "p")
// WHERE (name LIKE @p0)  -- @p0 = 'John%'
```

**Admin users**:
```go
filter, _ := aipsql.ParseFilter("role=\"admin\"")
// WHERE (role = @p0)
```

**Active admins**:
```go
filter, _ := aipsql.ParseFilter("status=\"active\" AND role=\"admin\"")
// WHERE ((status = @p0) AND (role = @p1))
```

**Active or pending users**:
```go
filter, _ := aipsql.ParseFilter("(status=\"active\" OR status=\"pending\")")
// WHERE ((status = @p0) OR (status = @p1))
```

### HTTP Handler Example

```go
func listUsersHandler(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Get filter from query params
        filterStr := r.URL.Query().Get("filter")
        if filterStr == "" {
            filterStr = "status=\"active\""
        }

        // Parse and generate SQL
        filter, err := aipsql.ParseFilter(filterStr)
        if err != nil {
            http.Error(w, "invalid filter", http.StatusBadRequest)
            return
        }

        sql, params, err := usersTable.WhereClause(filter, "p")
        if err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
            return
        }

        // Execute
        query := fmt.Sprintf("SELECT * FROM users WHERE %s", sql)
        rows, err := db.Query(query, aipsql.ParamValues(params)...)
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        defer rows.Close()

        // Scan and return
        users := scanUsers(rows)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "users": users,
            "count": len(users),
        })
    }
}
```

## Example 2: Autocomplete Search

Search-as-you-type for products using prefix matching.

### Table Setup

```go
var productsTable = aipsql.NewTable().WithColumns(
    aipsql.NewColumn().
        WithFieldPath("name").
        WithDatabaseName("name").
        WithMatchModes(aipsql.MatchModePrefix, aipsql.MatchModeExact).
        Filterable().
        Sortable().
        Build(),
    aipsql.NewColumn().
        WithFieldPath("category").
        WithDatabaseName("category").
        WithMatchModes(aipsql.MatchModeExact).
        Filterable().
        Build(),
).Build()

// Database index
// CREATE INDEX idx_products_name ON products(name);
```

### Autocomplete Function

```go
func autocompleteProducts(db *sql.DB, prefix string, limit int) ([]Product, error) {
    // Parse filter with prefix match
    filter, _ := aipsql.ParseFilter(fmt.Sprintf("name:\"%s\"", prefix))

    // Generate SQL
    sql, params, _ := productsTable.WhereClause(filter, "p")

    // Build query
    query := fmt.Sprintf(
        "SELECT * FROM products WHERE %s ORDER BY name ASC LIMIT %d",
        sql, limit,
    )

    // Execute
    rows, _ := db.Query(query, aipsql.ParamValues(params)...)
    defer rows.Close()

    // Scan results
    var products []Product
    for rows.Next() {
        var p Product
        rows.Scan(&p.ID, &p.Name, &p.Category)
        products = append(products, p)
    }

    return products, nil
}
```

**Usage**:
```go
// User types: "iPh"
products, _ := autocompleteProducts(db, "iPh", 10)

// Generated SQL:
// WHERE (name LIKE @p0) ORDER BY name ASC LIMIT 10
// @p0 = 'iPh%'
// Results: iPhone, iPhone Case, iPhone Charger, ...
```

### HTTP Endpoint

```go
func autocompleteHandler(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        searchTerm := r.URL.Query().Get("q")
        if searchTerm == "" {
            http.Error(w, "Missing search term", http.StatusBadRequest)
            return
        }

        limit := parseInt(r.URL.Query().Get("limit"), 10)
        limit = min(limit, 50) // Max 50

        products, err := autocompleteProducts(db, searchTerm, limit)
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }

        // Return suggestions
        suggestions := make([]string, len(products))
        for i, p := range products {
            suggestions[i] = p.Name
        }

        json.NewEncoder(w).Encode(map[string]interface{}{
            "suggestions": suggestions,
        })
    }
}
```

**Request/Response**:
```
GET /autocomplete?q=iPh&limit=10

{
  "suggestions": [
    "iPhone",
    "iPhone Case",
    "iPhone Charger",
    "iPhone 13",
    "iPhone 14"
  ]
}
```

### Client-Side Debouncing

```javascript
let debounceTimer;

function onSearchInput(event) {
    clearTimeout(debounceTimer);

    debounceTimer = setTimeout(() => {
        const searchTerm = event.target.value;

        if (searchTerm.length >= 2) {
            fetch(`/autocomplete?q=${encodeURIComponent(searchTerm)}`)
                .then(response => response.json())
                .then(data => showSuggestions(data.suggestions));
        }
    }, 300);  // Wait 300ms after user stops typing
}
```

## Example 3: Full-Text Search

Natural language document search with PostgreSQL/MySQL.

### Table Setup

```go
var documentsTable = aipsql.NewTable().WithColumns(
    aipsql.NewColumn().
        WithFieldPath("title").
        WithDatabaseName("title").
        WithMatchModes(aipsql.MatchModePrefix).
        Filterable().
        Build(),
    aipsql.NewColumn().
        WithFieldPath("content").
        WithDatabaseName("content").
        WithMatchModes(aipsql.MatchModeFullText, aipsql.MatchModeContains).
        Filterable().
        Build(),
).Build()
```

### Database Schema

**PostgreSQL**:
```sql
CREATE TABLE documents (
    id BIGSERIAL PRIMARY KEY,
    title VARCHAR(500),
    content TEXT
);

-- Full-text index
CREATE INDEX idx_content_fts ON documents
USING GIN(to_tsvector('simple', content));
```

**MySQL**:
```sql
CREATE TABLE documents (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    title VARCHAR(500),
    content TEXT,
    FULLTEXT INDEX idx_content_fts (content)
) ENGINE=InnoDB;
```

### Search Function

**PostgreSQL**:
```go
func searchDocuments(db *sql.DB, query string, limit int) ([]Document, error) {
    filter, _ := aipsql.ParseFilter(fmt.Sprintf("content:\"%s\"", query))

    opts := aipsql.WhereClauseOptions{
        Dialect: aipsql.SQLDialectPostgres,
    }

    sql, params, _ := documentsTable.WhereClauseWithOptions(filter, "p", opts)

    queryStr := fmt.Sprintf(
        "SELECT * FROM documents WHERE %s ORDER BY ts_rank_cd(textsearch, query) DESC LIMIT %d",
        sql, limit,
    )

    rows, _ := db.Query(queryStr, aipsql.ParamValues(params)...)
    defer rows.Close()

    var docs []Document
    for rows.Next() {
        var d Document
        rows.Scan(&d.ID, &d.Title, &d.Content)
        docs = append(docs, d)
    }

    return docs, nil
}
```

**Generated SQL**:
```sql
SELECT * FROM documents
WHERE to_tsvector('simple', content) @@ websearch_to_tsquery('simple', @p0)
ORDER BY ts_rank_cd(textsearch, query) DESC
LIMIT 10

-- @p0 = 'machine learning'
```

**MySQL**:
```go
opts.Dialect = aipsql.SQLDialectMySQL
// Generates: WHERE MATCH(content) AGAINST (@p0 IN BOOLEAN MODE)
```

### HTTP Endpoint

```go
func searchHandler(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        query := r.URL.Query().Get("q")
        if query == "" {
            http.Error(w, "Missing query", http.StatusBadRequest)
            return
        }

        limit := parseInt(r.URL.Query().Get("limit"), 10)
        limit = min(limit, 100)

        docs, err := searchDocuments(db, query, limit)
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }

        json.NewEncoder(w).Encode(map[string]interface{}{
            "results": docs,
            "count":   len(docs),
        })
    }
}
```

### Performance Comparison

| Method | Query Time (1M rows) | Index Used |
|--------|---------------------|------------|
| FullText | ~10ms | GIN/FULLTEXT |
| Contains | ~500ms | None (full scan) |

**Improvement**: 50x faster with full-text search.

## Example 4: Key-Value Labels

Flexible metadata filtering (Kubernetes labels, AWS tags).

### Table Setup

```go
var resourcesTable = aipsql.NewTable().WithColumns(
    aipsql.NewColumn().
        WithFieldPath("name").
        WithDatabaseName("name").
        WithMatchModes(aipsql.MatchModePrefix).
        Filterable().
        Build(),
    aipsql.NewColumn().
        WithFieldPath("labels").
        WithDatabaseName("labels").
        KeyValue().
        WithMatchModes(aipsql.MatchModeExact).
        Filterable().
        Build(),
).Build()
```

### Database Schema

```sql
CREATE TABLE resources (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(500),
    labels JSONB
);

-- GIN index for efficient queries
CREATE INDEX idx_labels ON resources USING GIN(labels);

-- Example data
INSERT INTO resources (name, labels) VALUES
('web-server-1', '{"environment": "production", "tier": "frontend"}'),
('db-server-1', '{"environment": "production", "tier": "backend"}');
```

### Filtering Examples

**Exact match**:
```go
filter, _ := aipsql.ParseFilter("labels.environment=\"production\"")
sql, params, _ := resourcesTable.WhereClause(filter, "p")

// WHERE EXISTS (SELECT 1 FROM UNNEST(labels) WHERE key = @p0 AND value = @p1)
```

**Multiple labels (AND)**:
```go
filter, _ := aipsql.ParseFilter(
    "labels.environment=\"production\" AND labels.tier=\"frontend\"",
)

// WHERE (
//   EXISTS (SELECT 1 FROM UNNEST(labels) WHERE key = @p0 AND value = @p1) AND
//   EXISTS (SELECT 1 FROM UNNEST(labels) WHERE key = @p2 AND value = @p3)
// )
```

**Real-world patterns**:

**Kubernetes-style**:
```go
filter, _ := aipsql.ParseFilter(
    "labels.app=\"my-app\" AND labels.env=\"production\" AND labels.tier=\"frontend\"",
)
```

**AWS-style tags**:
```go
filter, _ := aipsql.ParseFilter(
    "tags.Environment=\"Production\" AND tags.Team=\"Platform\"",
)
```

**GCP labels**:
```go
filter, _ := aipsql.ParseFilter(
    "labels.env=\"prod\" AND labels.service=\"api-gateway\"",
)
```

## Example 5: Performance Optimization

Real-world composite index optimization for e-commerce orders.

### Problem

Order listing query is slow with multiple filters.

**Before optimization**:
```go
filter := "user_id=123 AND status=\"active\" AND created_at>\"2024-01-01\""

// Generated SQL (original order):
WHERE user_id = @p0 AND status = @p1 AND created_at > @p2

// Performance:
// - Time: ~150ms
// - Rows scanned: ~50,000
// - Index used: Partial (only user_id)
```

### Solution

**Enable composite index optimization**:
```go
ordersTable.CompositeIndexes = []aipsql.CompositeIndex{
    {Name: "idx_status_user_created", Columns: []string{"status", "user_id", "created_at"}},
}

opts := aipsql.WhereClauseOptions{
    EnableCompositeIndexOptimization: true,
}

sql, params, _ := ordersTable.WhereClauseWithOptions(filter, "p", opts)

// Generated SQL (optimized order):
WHERE status = @p1 AND user_id = @p0 AND created_at > @p2
//        ^^^^^^ equality first (index order)
//                    ^^^^^^^ equality second
//                                        ^^^^^^^^^^^^^^^ range last

// Performance:
// - Time: ~3ms (50x faster!)
// - Rows scanned: ~50
// - Index used: Full (all three columns)
```

### Database Indexes

```sql
-- Must match aipsql configuration
CREATE INDEX idx_status_user_created ON orders(status, user_id, created_at DESC);
CREATE INDEX idx_user_created ON orders(user_id, created_at DESC);
CREATE INDEX idx_created_updated ON orders(created_at DESC, updated_at DESC);
```

### Real-World Scenarios

**Scenario 1: User Dashboard**
```go
filter := "user_id=123 AND status=\"active\""

// Before: WHERE user_id = @p0 AND status = @p1
//   Time: ~50ms, Rows: ~5,000

// After: WHERE status = @p1 AND user_id = @p0
//   Time: ~2ms, Rows: ~50
```

**Scenario 2: Order Queue**
```go
filter := "status=\"pending\" AND priority>5 AND created_at>\"2024-01-01\""

// Optimized: WHERE status = @p0 AND created_at > @p1 AND priority > @p2
//   Equality first, then range conditions
```

### HTTP Handler with Optimization

```go
func listOrdersHandler(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Build filter from query params
        filterParts := []string{}
        if v := r.URL.Query().Get("user_id"); v != "" {
            filterParts = append(filterParts, fmt.Sprintf("user_id=%s", v))
        }
        if v := r.URL.Query().Get("status"); v != "" {
            filterParts = append(filterParts, fmt.Sprintf("status=\"%s\"", v))
        }
        if v := r.URL.Query().Get("min_date"); v != "" {
            filterParts = append(filterParts, fmt.Sprintf("created_at>\"%s\"", v))
        }
        filterStr := strings.Join(filterParts, " AND ")

        // Enable optimization
        filter, _ := aipsql.ParseFilter(filterStr)
        opts := aipsql.WhereClauseOptions{
            EnableCompositeIndexOptimization: true,
        }

        whereSQL, whereParams, _ := ordersTable.WhereClauseWithOptions(filter, "p", opts)

        orderBy, _ := aipsql.ParseOrderBy("created_at DESC, id DESC")
        orderBySQL, orderParams, _ := ordersTable.OrderByClause(orderBy, "p")

        params := append(whereParams, orderParams...)

        query := fmt.Sprintf(
            "SELECT * FROM orders WHERE %s %s LIMIT 100",
            whereSQL, orderBySQL,
        )

        // Execute with timing
        start := time.Now()
        rows, err := db.Query(query, aipsql.ParamValues(params)...)
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        defer rows.Close()

        orders := scanOrders(rows)
        duration := time.Since(start)

        // Return with timing
        json.NewEncoder(w).Encode(map[string]interface{}{
            "orders":      orders,
            "count":       len(orders),
            "duration_ms": duration.Milliseconds(),
        })
    }
}
```

### Benchmark Results

**Table size**: 1,000,000 orders

| Query Pattern | Before | After | Improvement |
|--------------|--------|-------|-------------|
| `user_id=123` | 20ms | 20ms | 1x |
| `user_id=123 AND status="active"` | 150ms | 3ms | 50x |
| `status="active" AND created_at>..."` | 200ms | 2ms | 100x |
| All three filters | 250ms | 3ms | 83x |

## Example 6: GORM Integration

Use `aipsql` to build SQL fragments, then apply them to `*gorm.DB` with the adapter package:
[`../adapter/gorm/README.md`](../adapter/gorm/README.md)

```go
import (
    aipsql "github.com/codesjoy/pkg/basic/aipsql"
    aipsqlgorm "github.com/codesjoy/pkg/basic/aipsql/adapter/gorm"
)

filter, _ := aipsql.ParseFilter(`status="active" AND name:"Al"`)
whereSQL, params, _ := usersTable.WhereClause(filter, "p_")

var users []User
err := aipsqlgorm.ApplyWhere(
    db.Model(&User{}),
    whereSQL,
    params,
).Order("created_at DESC, id DESC").Find(&users).Error
```

For QueryPlanner-driven queries, use:

```go
err := aipsqlgorm.ApplyPlan(db.Model(&Order{}), plan).Find(&orders).Error
```

## Summary

These examples demonstrate:

1. **Basic Filtering**: Simple and complex filter patterns
2. **Autocomplete**: Prefix matching with debouncing
3. **Full-Text Search**: Natural language with dialect-specific features
4. **Key-Value Labels**: Flexible metadata for cloud resources
5. **Performance Optimization**: 10x-100x improvements with composite indexes
6. **GORM Integration**: Execute generated clauses with named arguments

**Key takeaways**:
- Use prefix matching for autocomplete
- Use full-text search for long content
- Create indexes matching query patterns
- Enable optimization for large tables
- Always parameterize queries

For more details:
- [User Guide](guide.md) - Complete usage guide
- [API Reference](api.md) - Complete API documentation
- [Performance](performance.md) - Performance analysis
