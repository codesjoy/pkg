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
func NewQueryPlanner() *QueryPlanner
```

Create a new QueryPlanner instance.

**Example**:
```go
planner := aipsql.NewQueryPlanner()
```

### QueryPlanner.RegisterTable

```go
func (p *QueryPlanner) RegisterTable(name string, spec *TableSpec) error
```

Register a table with the planner.

**Parameters**:
- `name`: Unique table identifier
- `spec`: Table specification

**Example**:
```go
planner.RegisterTable("users", &aipsql.TableSpec{
    Name:                "users",
    Table:               usersTable,
    DefaultOrder:        "created_at DESC, id DESC",
    TieBreakerFieldPath: "id",
    FromClause:          "users",
})
```

### QueryPlanner.SetDefaultOptions

```go
func (p *QueryPlanner) SetDefaultOptions(opts DefaultOptions)
```

Set global default options for all queries.

**Example**:
```go
planner.SetDefaultOptions(aipsql.DefaultOptions{
    Dialect:                           aipsql.SQLDialectPostgres,
    EnableCompositeIndexOptimization: true,
    DefaultPageSize:                   20,
    MaxPageSize:                       100,
})
```

### QueryPlanner.PlanList

```go
func (p *QueryPlanner) PlanList(req *QueryRequest) (*QueryPlan, error)
```

Generate a complete query plan.

**Parameters**:
- `req`: Query request

**Returns**:
- `*QueryPlan`: Generated query plan
- `error`: Error if planning fails

**Example**:
```go
plan, err := planner.PlanList(&aipsql.QueryRequest{
    Table:     "users",
    Filter:    "status=\"active\"",
    PageSize:  20,
    PageToken: "ey...",
})
```

### TableSpec

Table specification for QueryPlanner.

```go
type TableSpec struct {
    Name                string
    Table               *Table
    DefaultOrder        string
    TieBreakerFieldPath string
    FromClause          string
    SelectClause        string
}
```

**Fields**:
- `Name` - Table identifier
- `Table` - Table schema
- `DefaultOrder` - Default ORDER BY (AIP-132 syntax)
- `TieBreakerFieldPath` - Field for unique ordering
- `FromClause` - Custom FROM (for JOINs)
- `SelectClause` - Custom SELECT (optional)

### QueryRequest

Query request structure.

```go
type QueryRequest struct {
    Table       string
    Filter      string
    OrderBy     string
    PageSize    int
    PageToken   string
    Options     *WhereClauseOptions
    EnableDebug bool
}
```

**Fields**:
- `Table` (required) - Table name
- `Filter` (required) - AIP-160 filter
- `PageSize` (required) - Results per page
- `OrderBy` (optional) - AIP-132 order by
- `PageToken` (optional) - Pagination token
- `Options` (optional) - Override options
- `EnableDebug` (optional) - Debug info

### QueryPlan

Generated query plan.

```go
type QueryPlan struct {
    FromClause      string
    WhereClause     string
    OrderByClause   string
    Limit           int
    Params          []Param
    TokenDescriptor *TokenDescriptor
    Debug           *DebugInfo
}
```

**Fields**:
- `FromClause` - FROM clause
- `WhereClause` - WHERE clause
- `OrderByClause` - ORDER BY clause
- `Limit` - LIMIT value
- `Params` - All parameters
- `TokenDescriptor` - Pagination metadata
- `Debug` - Debug info (if enabled)

**Example**:
```go
plan, _ := planner.PlanList(request)

query := fmt.Sprintf(
    "SELECT * FROM %s WHERE %s %s LIMIT %d",
    plan.FromClause,
    plan.WhereClause,
    plan.OrderByClause,
    plan.Limit,
)

rows, _ := db.Query(query, aipsql.ParamValues(plan.Params)...)
```

### DebugInfo

Debug information for troubleshooting.

```go
type DebugInfo struct {
    SelectedIndex      string
    ConditionsReordered bool
    OriginalOrder      []string
    OptimizedOrder     []string
    MatchModes         map[string]string
}
```

**Example**:
```go
plan, _ := planner.PlanList(&aipsql.QueryRequest{
    EnableDebug: true,
})

if plan.Debug != nil {
    log.Printf("Index: %s", plan.Debug.SelectedIndex)
    log.Printf("Reordered: %v", plan.Debug.ConditionsReordered)
}
```

### TokenDescriptor

Pagination token metadata.

```go
type TokenDescriptor struct {
    SortFields       []SortField
    TieBreakerColumn *Column
}

type SortField struct {
    FieldPath string
    Direction string
}
```

**Example**:
```go
// Encode next page token
if len(results) == plan.Limit {
    lastRow := results[len(results)-1]
    values := extractSortValues(lastRow, plan.TokenDescriptor.SortFields)

    token := aipsql.SeekPageToken{
        SortValues: values,
    }

    nextPageToken := token.Encode()
}
```

## Helper Functions

### SeekPageToken

Pagination token for seek pagination.

```go
type SeekPageToken struct {
    SortValues      []interface{}
    TieBreakerValue interface{}
}

func (t *SeekPageToken) Encode() string
func ParseSeekPageToken(token string) (*SeekPageToken, error)
```

**Example**:
```go
// Encode
token := aipsql.SeekPageToken{
    SortValues: []interface{}{lastRow.CreatedAt, lastRow.ID},
}
encoded := token.Encode()

// Decode
decoded, err := aipsql.ParseSeekPageToken(encoded)
```

### BuildSeekPaginationClause

```go
func BuildSeekPaginationClause(
    orderByFields []OrderByField,
    sortValues []interface{},
    paramPrefix string,
) (string, []Param, error)
```

Generate seek pagination WHERE clause.

**Example**:
```go
orderByFields := []aipsql.OrderByField{
    {Column: createdAtColumn, Direction: "DESC"},
    {Column: idColumn, Direction: "DESC"},
}

seekClause, params, _ := aipsql.BuildSeekPaginationClause(
    orderByFields,
    []interface{}{time.Now(), 123},
    "p",
)

// Result:
// (created_at < @p0 OR (created_at = @p1 AND id < @p2))
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

Applies the plan's `WhereClause`, `OrderByClause`, and `Limit` to a GORM query chain:

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
- **QueryPlanner**: `NewQueryPlanner`, `RegisterTable`, `PlanList`
- **GORM Adapter**: `NamedArgs`, `ApplyWhere`, `ApplyPlan`

**Helpers**:
- `ParamValues` - Convert params for database driver
- `SeekPageToken` - Pagination token encoding
- `BuildSeekPaginationClause` - Seek pagination generation

For more details:
- [User Guide](guide.md) - Complete usage guide
- [Examples](examples.md) - Real-world examples
- [Performance](performance.md) - Performance characteristics
