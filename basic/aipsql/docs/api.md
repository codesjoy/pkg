# API Reference

Complete API reference for the aipsql package.

## Table of Contents

1. [Types](#types)
2. [Filter API](#filter-api)
3. [OrderBy API](#orderby-api)
4. [QueryPlanner API](#queryplanner-api)
5. [GORM Adapter API](#gorm-adapter-api)

## Types

### Table

**Type**: `struct`

**Description**: Represents a database table schema with columns and optimization hints.

```go
type Table struct {
    columns               []*Column
    implicitFilterColumns []*Column
    columnByFieldPath     map[string]*Column
    CompositeIndexes      []CompositeIndex
}
```

**Methods**:
- `WhereClause(filter *Filter, paramPrefix string) (string, []Param, error)` - Basic WHERE clause
- `WhereClauseWithOptions(filter *Filter, paramPrefix string, opts WhereClauseOptions) (string, []Param, error)` - WHERE clause with options
- `OrderByClause(orderBy *OrderBy, paramPrefix string) (string, []Param, error)` - ORDER BY clause

**Example**:
```go
table := aipsql.NewTable().WithColumns(
    aipsql.NewColumn().
        WithFieldPath("name").
        WithDatabaseName("name").
        Filterable().
        Build(),
).Build()
```

### Column

**Type**: `struct`

**Description**: Defines a column's mapping from API field to database column.

**Builder methods**:
- `WithFieldPath(...string)` - API field path
- `WithDatabaseName(string)` - DB column name
- `WithMatchModes(...MatchMode)` - Text search modes
- `Filterable()` - Enable WHERE
- `Sortable()` - Enable ORDER BY
- `FilterableImplicitly()` - Enable implicit search
- `KeyValue()` - Key-value column
- `Bool()` - Boolean type
- `WithIndexHint(string)` - Index documentation
- `Build()` - Create column

**Example**:
```go
column := aipsql.NewColumn().
    WithFieldPath("displayName").
    WithDatabaseName("display_name").
    WithMatchModes(aipsql.MatchModePrefix, aipsql.MatchModeExact).
    Filterable().
    Sortable().
    Build()
```

### MatchMode

**Type**: `string` (enum)

**Values**:
- `MatchModeExact` - Exact equality: `column = @param`
- `MatchModePrefix` - Prefix match: `column LIKE @param` (where `@param = 'value%'`)
- `MatchModeFullText` - Full-text search (Postgres/MySQL only)
- `MatchModeContains` - Substring: `column LIKE %@param%`

**Example**:
```go
column.WithMatchModes(aipsql.MatchModePrefix, aipsql.MatchModeExact)
```

### SQLDialect

**Type**: `string` (enum)

**Values**:
- `SQLDialectGeneric` - Standard SQL (default)
- `SQLDialectPostgres` - PostgreSQL features
- `SQLDialectMySQL` - MySQL features

**Example**:
```go
opts := aipsql.WhereClauseOptions{
    Dialect: aipsql.SQLDialectPostgres,
}
```

### Filter

**Type**: `struct`

**Description**: Parsed AIP-160 filter expression AST.

**Parsing**:
```go
func ParseFilter(expr string) (*Filter, error)
```

**Example**:
```go
filter, err := aipsql.ParseFilter("status=\"active\" AND name:\"John\"")
```

### OrderBy

**Type**: `struct`

**Description**: Parsed AIP-132 order by expression.

**Parsing**:
```go
func ParseOrderBy(expr string) (*OrderBy, error)
```

**Example**:
```go
orderBy, err := aipsql.ParseOrderBy("name ASC, created_at DESC")
```

### Param

**Type**: `struct`

**Description**: SQL query parameter.

```go
type Param struct {
    Name  string
    Value interface{}
}
```

**Helper**:
```go
func ParamValues(params []Param) []interface{}
```

**Example**:
```go
params := []Param{{Name: "p0", Value: "active"}}
db.Query("WHERE status = @p0", ParamValues(params)...)
```

### CompositeIndex

**Type**: `struct`

**Description**: Multi-column database index.

```go
type CompositeIndex struct {
    Name    string
    Columns []string  // Database column names
}
```

**Example**:
```go
table.CompositeIndexes = []aipsql.CompositeIndex{
    {Name: "idx_status_user", Columns: []string{"status", "user_id"}},
}
```

### WhereClauseOptions

**Type**: `struct`

**Description**: Options for WHERE clause generation.

```go
type WhereClauseOptions struct {
    Dialect                          SQLDialect
    StrictMode                       bool
    EnableCompositeIndexOptimization bool
}
```

**Fields**:
- `Dialect` - SQL dialect (default: Generic)
- `StrictMode` - Fail on unsupported match mode (default: false)
- `EnableCompositeIndexOptimization` - Reorder conditions for indexes (default: false)

**Example**:
```go
opts := aipsql.WhereClauseOptions{
    Dialect:                           aipsql.SQLDialectPostgres,
    EnableCompositeIndexOptimization: true,
    StrictMode:                        true,
}
```

## Filter API

### ParseFilter

```go
func ParseFilter(expr string) (*Filter, error)
```

Parse an AIP-160 filter expression.

**Parameters**:
- `expr`: Filter expression string

**Returns**:
- `*Filter`: Parsed AST
- `error`: Parse error

**Example**:
```go
filter, err := aipsql.ParseFilter("status=\"active\" AND name:\"John\"")
if err != nil {
    return fmt.Errorf("invalid filter: %w", err)
}
```

### Table.WhereClause

```go
func (t *Table) WhereClause(filter *Filter, paramPrefix string) (string, []Param, error)
```

Generate basic WHERE clause.

**Parameters**:
- `filter`: Parsed filter AST
- `paramPrefix`: Parameter name prefix (e.g., "p")

**Returns**:
- `string`: SQL WHERE clause
- `[]Param`: Query parameters
- `error`: Error if generation fails

**Example**:
```go
sql, params, _ := table.WhereClause(filter, "p")
// Result: "(status = @p0)", [{Name: "p0", Value: "active"}]
```

### Table.WhereClauseWithOptions

```go
func (t *Table) WhereClauseWithOptions(
    filter *Filter,
    paramPrefix string,
    opts WhereClauseOptions,
) (string, []Param, error)
```

Generate WHERE clause with optimization options.

**Example**:
```go
opts := aipsql.WhereClauseOptions{
    Dialect:                           aipsql.SQLDialectPostgres,
    EnableCompositeIndexOptimization: true,
}

sql, params, _ := table.WhereClauseWithOptions(filter, "p", opts)
```

### Filter Syntax

**Operators**:
- `=` - Equal: `status="active"`
- `!=` - Not equal: `status!="pending"`
- `<` `<=` `>` `>=` - Comparison: `created_at>"2024-01-01"`
- `:` - Has (text search): `name:"John"`

**Logical**:
- `AND` - Logical AND
- `OR` - Logical OR
- `()` - Grouping

**Examples**:
```go
// Simple equality
ParseFilter("status=\"active\"")
// WHERE (status = @p0)

// Multiple conditions
ParseFilter("status=\"active\" AND name:\"John\"")
// WHERE ((status = @p0) AND (name LIKE @p1))

// Complex
ParseFilter("(status=\"active\" OR status=\"pending\") AND name:\"John\"")
// WHERE (((status = @p0) OR (status = @p1)) AND (name LIKE @p2))
```

## OrderBy API

### ParseOrderBy

```go
func ParseOrderBy(expr string) (*OrderBy, error)
```

Parse an AIP-132 order by expression.

**Parameters**:
- `expr`: Order by expression string

**Returns**:
- `*OrderBy`: Parsed AST
- `error`: Parse error

**Example**:
```go
orderBy, err := aipsql.ParseOrderBy("name ASC, created_at DESC")
```

### Table.OrderByClause

```go
func (t *Table) OrderByClause(orderBy *OrderBy, paramPrefix string) (string, []Param, error)
```

Generate ORDER BY clause.

**Parameters**:
- `orderBy`: Parsed order by AST
- `paramPrefix`: Parameter name prefix

**Returns**:
- `string`: SQL ORDER BY clause (includes "ORDER BY")
- `[]Param`: Query parameters
- `error`: Error if generation fails

**Example**:
```go
orderBy, _ := aipsql.ParseOrderBy("name ASC, created_at DESC")
sql, params, _ := table.OrderByClause(orderBy, "p")
// Result: "ORDER BY name ASC, created_at DESC"
```

### OrderBy Syntax

**Directions**:
- `ASC` - Ascending
- `DESC` - Descending (default: ASC)

**Examples**:
```go
// Single field
ParseOrderBy("created_at DESC")
// ORDER BY created_at DESC

// Multiple fields
ParseOrderBy("name ASC, created_at DESC, id DESC")
// ORDER BY name ASC, created_at DESC, id DESC

// Nested field
ParseOrderBy("user.profile.displayName ASC")
// ORDER BY display_name ASC
```

### MergeWithDefaultOrder

```go
func MergeWithDefaultOrder(userOrder *OrderBy, defaultOrder *OrderBy) *OrderBy
```

Merge user-specified order with default order (user takes precedence).

**Example**:
```go
userOrder, _ := aipsql.ParseOrderBy("created_at DESC")
defaultOrder, _ := aipsql.ParseOrderBy("id DESC")
merged := aipsql.MergeWithDefaultOrder(userOrder, defaultOrder)
// If user orders by created_at, adds id as tie-breaker
```

## QueryPlanner API

### NewQueryPlanner

```go
func NewQueryPlanner(spec TableSpec, options QueryPlannerOptions) (*QueryPlanner, error)
```

Create a QueryPlanner bound to one table specification.

**Example**:
```go
planner, err := aipsql.NewQueryPlanner(aipsql.TableSpec{
    Table:               usersTable,
    DefaultOrder:        []aipsql.OrderBy{{FieldPath: aipsql.NewFieldPath("created_at"), Descending: true}},
    TieBreakerFieldPath: aipsql.NewFieldPath("id"),
}, aipsql.QueryPlannerOptions{
    Dialect:                           aipsql.SQLDialectPostgres,
    EnableCompositeIndexOptimization: true,
    DefaultPageSize:                   20,
    MaxPageSize:                       100,
})
```

### QueryPlanner.PlanList

```go
func (p *QueryPlanner) PlanList(ctx context.Context, req QueryRequest) (*QueryPlan, error)
```

Generate final executable clauses for a list query.

**Parameters**:
- `req`: Query request

**Returns**:
- `*QueryPlan`: Generated query plan
- `error`: Error if planning fails

**Example**:
```go
plan, err := planner.PlanList(ctx, aipsql.QueryRequest{
    Filter:    "status=\"active\"",
    PageSize:  20,
    PageToken: "ey...",
})
```

### TableSpec

Table specification for QueryPlanner.

```go
type TableSpec struct {
    Table               *Table
    DefaultOrder        []OrderBy
    TieBreakerFieldPath FieldPath
    PaginationMode      PaginationMode
    DefaultPageSize     int
    MaxPageSize         int
}
```

**Fields**:
- `Table` - Table schema
- `DefaultOrder` - Default ORDER BY entries
- `TieBreakerFieldPath` - Field for unique ordering
- `PaginationMode` - Default pagination mode (`seek` or `offset`)
- `DefaultPageSize` - Default LIMIT value
- `MaxPageSize` - Maximum LIMIT value

### QueryRequest

Query request structure.

```go
type QueryRequest struct {
    Filter                        string
    OrderBy                       string
    PageSize                      int
    PageToken                     string
    PaginationMode                PaginationMode
    ParameterPrefix               string
    Dialect                       SQLDialect
    StrictMode                    *bool
    EnableCompositeIndexOptimization *bool
}
```

**Fields**:
- `Filter` (required) - AIP-160 filter
- `PageSize` (required) - Results per page
- `OrderBy` (optional) - AIP-132 order by
- `PageToken` (optional) - Pagination token
- `PaginationMode` (optional) - Override default pagination mode

### QueryPlan

Generated query plan.

```go
type QueryPlan struct {
    WhereClause     string
    OrderByClause   string
    Parameters      []QueryParameter
    Limit           int
    Offset          int
}
```

**Fields**:
- `WhereClause` - WHERE clause
- `OrderByClause` - ORDER BY clause
- `Limit` - LIMIT value
- `Offset` - OFFSET value for offset pagination
- `Parameters` - All parameters

**Example**:
```go
plan, _ := planner.PlanList(ctx, request)

query := fmt.Sprintf(
    "SELECT id, created_at FROM users WHERE %s ORDER BY %s LIMIT %d",
    plan.WhereClause,
    plan.OrderByClause,
    plan.Limit,
)
results, _ := loadUsers(query, aipsql.ParamValues(plan.Parameters)...)

// next token is derived after executing the current page because it depends
// on the actual last row returned by the database.
nextPageToken, _ := plan.NextPageToken(results)
_ = nextPageToken
```

### QueryPlan.NextPageToken

Computes the next page token from the current page rows.

```go
func (p *QueryPlan) NextPageToken(rows any) (string, error)
```

**Example**:
```go
token, _ := plan.NextPageToken(results)
_ = token
```

`PlanList` does not directly return `nextToken` because the token depends on the
actual last row in the current page, which is only known after query execution.

## Helper Functions

### SeekPageToken

Seek pagination token payload.

```go
type SeekPageToken struct {
    SortValues      []string
    TieBreakerValue string
}

func EncodeSeekPageToken(token SeekPageToken) (string, error)
func DecodeSeekPageToken(token string) (SeekPageToken, error)
```

**Example**:
```go
token := aipsql.SeekPageToken{
    SortValues:      []string{lastRow.CreatedAt.Format(time.RFC3339Nano)},
    TieBreakerValue: strconv.FormatInt(lastRow.ID, 10),
}
encoded, _ := aipsql.EncodeSeekPageToken(token)
decoded, _ := aipsql.DecodeSeekPageToken(encoded)
_ = decoded
```

### OffsetPageToken

Offset pagination helper functions.

```go
func EncodeOffsetPageToken(offset int) string
func DecodeOffsetPageToken(token string) (int, error)
```

**Example**:
```go
nextPageToken := aipsql.EncodeOffsetPageToken(40)
offset, _ := aipsql.DecodeOffsetPageToken(nextPageToken)
_ = offset
```

## GORM Adapter API

Package: `github.com/codesjoy/pkg/basic/aipsql/adapter/gorm`

### NamedArgs

```go
func NamedArgs(params []aipsql.QueryParameter) []any
```

Converts `aipsql` named parameters into `sql.NamedArg` values that can be passed to GORM.

### ApplyWhere

```go
func ApplyWhere(
    db *gorm.DB,
    whereSQL string,
    params []aipsql.QueryParameter,
) *gorm.DB
```

Applies one generated WHERE fragment with named parameters:

```go
db = aipsqlgorm.ApplyWhere(db, whereSQL, params)
```

Returns `db` unchanged when `db == nil` or `whereSQL` is empty.

### ApplyPlan

```go
func ApplyPlan(db *gorm.DB, plan *aipsql.QueryPlan) *gorm.DB
```

Applies the plan's `WhereClause`, `OrderByClause`, `Limit`, and `Offset` to a GORM query chain:

```go
db = aipsqlgorm.ApplyPlan(db, plan)
```

Returns `db` unchanged when `db == nil` or `plan == nil`.

### Adapter Guide

For full examples and caveats, see:
- [GORM Adapter README](../adapter/gorm/README.md)

## Summary

**Core Types**:
- `Table` - Database table schema
- `Column` - Column definition with builder
- `MatchMode` - Text search mode
- `SQLDialect` - SQL dialect
- `Filter`, `OrderBy` - Parsed expressions
- `Param` - Query parameter
- `CompositeIndex` - Multi-column index
- `WhereClauseOptions` - Generation options

**APIs**:
- **Filter**: `ParseFilter`, `WhereClause`, `WhereClauseWithOptions`
- **OrderBy**: `ParseOrderBy`, `OrderByClause`, `MergeWithDefaultOrder`
- **QueryPlanner**: `NewQueryPlanner`, `PlanList`
- **GORM Adapter**: `NamedArgs`, `ApplyWhere`, `ApplyPlan`

**Helpers**:
- `ParamValues` - Convert params for database driver
- `SeekPageToken` - Seek pagination token encoding
- `OffsetPageToken` - Offset pagination helpers

For more details:
- [User Guide](guide.md) - Complete usage guide
- [Examples](examples.md) - Real-world examples
- [Performance](performance.md) - Performance characteristics
