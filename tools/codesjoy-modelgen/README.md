# codesjoy-modelgen

`codesjoy-modelgen` introspects MySQL/PostgreSQL metadata and generates:

- Single combined file per table: `<singular_table>_model_gen.go`
- GORM model + optional `AIPTable()` method + optional compatibility wrapper

## Install

```bash
go install github.com/codesjoy/pkg/tools/codesjoy-modelgen@latest
```

## Usage

```bash
codesjoy-modelgen \
  --dsn "user:pass@tcp(127.0.0.1:3306)/demo?parseTime=true" \
  --schema demo \
  --tables users,orders \
  --out-dir ./internal/model \
  --gen-aipsql=true \
  --timestamp-mode unix_sec \
  --override ./override.yaml
```

Required flags:

- `--dsn`
  - MySQL example: `user:pass@tcp(127.0.0.1:3306)/demo?parseTime=true`
  - PostgreSQL example: `postgres://user:pass@127.0.0.1:5432/demo?sslmode=disable`

Optional flags:

- `--out-dir` (default `./`)
- `--schema`
- `--tables`
- `--override`
- `--package` (defaults to the cleaned `--out-dir` directory name; when `--out-dir` is `./`, uses the current working directory name)
- `--gen-aipsql` (default `true`)
- `--timestamp-mode` (`unix_sec|unix_milli|unix_nano`, default `unix_sec`; only affects integer-like timestamp columns)
- `--dry-run`
- `--force`

## Override YAML

```yaml
include_tables: []
exclude_tables: []
gen_aipsql: true
timestamp_mode: unix_sec
tables:
  users:
    skip: false
    model_name: User
    aipsql_builder: NewUserAIPTable
    gen_aipsql: true
    timestamp_mode: unix_sec
    columns:
      password_hash:
        skip: true
      is_active:
        go_field: IsActive
        go_type: bool
        json_name: is_active
        field_path: is_active
        filterable: true
        sortable: true
        implicit_filter: false
        bool_type: true
        key_value: false
        match_modes: [exact]
        gorm_tag_append: "default:true"
        timestamp_mode: unix_milli
```

## Behavior notes

- By default, scalar columns are `Filterable`.
- Primary-key or indexed columns are `Sortable` by default.
- Text columns default to `WithMatchModes(aipsql.MatchModeExact)`.
  - PostgreSQL `character varying` / `character` columns are treated as text by default.
- Composite indexes are generated from multi-column physical indexes.
- Timestamp role fields (`created*/updated*/deleted*`) choose Go types from the physical database type first.
- `--timestamp-mode` only controls integer-like timestamp precision: `unix_sec`, `unix_milli`, or `unix_nano`.
- Timestamp role fields are normalized to `CreatedAt`, `UpdatedAt`, `DeletedAt` by default (override-able with `go_field`).
- datetime-like `created*` / `updated*` columns -> `time.Time`
- `deleted*` columns are soft-delete aware:
  - datetime-like physical columns -> `gorm.DeletedAt` + `index`
  - integer-like physical columns -> `soft_delete.DeletedAt` + `softDelete[:milli|:nano]`
- `deleted*` columns are hidden from AIPTable by default; set `filterable=true` or `sortable=true` or `implicit_filter=true` in override to expose them.
- With `gen_aipsql=true`, generated file includes:
  - `func (Model) AIPTable() *aipsql.Table`
  - `func New<Model>AIPTable() *aipsql.Table` compatibility wrapper.
- Existing files without the generated header are protected unless `--force` is set.
- Legacy `*_gen.go`, `<table>_model_gen.go`, and `<table>_aipsql_gen.go` files are not deleted automatically; the generator prints migrate hints when they are detected.

## Test

```bash
go test ./...
```

## Integration Tests (Docker)

The integration suite is Docker-backed and uses Testcontainers to start MySQL and PostgreSQL.

Run from the module root:

```bash
go test -tags=integration -v ./testing/integration/...
```

Requirements:

- Docker Desktop (or another compatible Docker daemon) must be available.
- Integration tests are opt-in; default `go test ./...` and
  `make MODULES="tools/codesjoy-modelgen" test` remain unit-test focused.
- Directory conventions:
  - unit/golden fixtures: `testdata/golden/`
  - Docker integration tests: `testing/integration/`
- AIP index coverage in integration tests includes:
  - composite index generation with stable column ordering
  - hidden-column-driven composite index pruning
  - override-based deleted column exposure restoring composite indexes
  - single-column/unique/expression index filtering from `CompositeIndexes`
  - column-level `index_hint` passthrough (`WithIndexHint(...)`)

## Example

See [`example/`](./example/README.md) for a Docker-runnable PostgreSQL walkthrough and a committed sample generated output.
