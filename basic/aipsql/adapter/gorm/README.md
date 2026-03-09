# aipsql GORM Adapter

Lightweight integration helpers for running `aipsql`-generated SQL fragments with `gorm`.

## Package

```go
import aipsqlgorm "github.com/codesjoy/pkg/basic/aipsql/adapter/gorm"
```

## Quick Start

### Apply filter SQL to `*gorm.DB`

```go
filter, err := aipsql.ParseFilter(`status="active" AND name:"Al"`)
if err != nil {
    return err
}

whereSQL, params, err := table.WhereClause(filter, "p_")
if err != nil {
    return err
}

var users []User
err = aipsqlgorm.ApplyWhere(
    db.Model(&User{}),
    whereSQL,
    params,
).Order("created_at DESC, id DESC").Find(&users).Error
```

### Apply `QueryPlan` output to `*gorm.DB`

```go
plan, err := planner.PlanList(ctx, aipsql.QueryRequest{
    Filter:   `status="active"`,
    OrderBy:  "created_at DESC",
    PageSize: 20,
})
if err != nil {
    return err
}

var orders []Order
err = aipsqlgorm.ApplyPlan(db.Model(&Order{}), plan).Find(&orders).Error
```

## API

- `NamedArgs(params []aipsql.QueryParameter) []any`
  - Converts `aipsql` named parameters into `sql.NamedArg` values for GORM.
- `ApplyWhere(db *gorm.DB, whereSQL string, params []aipsql.QueryParameter) *gorm.DB`
  - Applies one `WHERE` fragment plus named args to a query chain.
- `ApplyPlan(db *gorm.DB, plan *aipsql.QueryPlan) *gorm.DB`
  - Applies `WhereClause`, `OrderByClause`, `Limit`, and `Offset` from a plan.

## Behavior Notes

- Uses named parameter binding (`@p_0`, `@p_1`, ...).
- `ApplyWhere` returns `db` unchanged when `db == nil` or `whereSQL` is empty.
- `ApplyPlan` returns `db` unchanged when `db == nil` or `plan == nil`.
- `ApplyPlan` does not stitch a full SQL statement; it only applies individual clauses.

## Verify

```bash
cd basic/aipsql
go test ./adapter/gorm -v
```
