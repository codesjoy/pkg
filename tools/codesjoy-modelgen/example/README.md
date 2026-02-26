# Example: codesjoy-modelgen (PostgreSQL)

This example is fully reproducible with local Docker:

- starts a PostgreSQL demo database
- runs `codesjoy-modelgen` against the demo table
- includes a committed sample output at `output/users_gen.go` for quick inspection

## Prerequisites

- Docker (Docker Desktop or compatible daemon)
- Go toolchain

## 1-Minute Start (Docker)

From repo root:

```bash
cd tools/codesjoy-modelgen/example
docker compose up -d
```

## Generate Files

From repo root:

```bash
go run ./tools/codesjoy-modelgen \
  --dialect postgres \
  --dsn "postgres://modelgen:modelgen@127.0.0.1:5432/modelgen_it?sslmode=disable" \
  --schema public \
  --tables users \
  --package demo \
  --out-dir ./tools/codesjoy-modelgen/example/output \
  --gen-aipsql=true \
  --timestamp-mode unix_nano \
  --override ./tools/codesjoy-modelgen/example/override.yaml
```

The generated file is:

- `tools/codesjoy-modelgen/example/output/users_gen.go`

## Quick Checks

From repo root:

```bash
grep -n 'WithDatabaseName("name")' tools/codesjoy-modelgen/example/output/users_gen.go
grep -n 'WithMatchModes(aipsql.MatchModeExact' tools/codesjoy-modelgen/example/output/users_gen.go
grep -n 'Name:    "idx_users_created_id"' tools/codesjoy-modelgen/example/output/users_gen.go
grep -n 'Columns: \\[\\]string{"created_at", "id"}' tools/codesjoy-modelgen/example/output/users_gen.go
grep -n 'WithDatabaseName("deleted_at")' tools/codesjoy-modelgen/example/output/users_gen.go
```

## Regenerate Sample Output

Re-run the same generate command above to refresh `output/users_gen.go`.

## Cleanup

From repo root:

```bash
cd tools/codesjoy-modelgen/example
docker compose down -v
```

## FAQ

- DSN auth failed:
  - Ensure container uses `modelgen/modelgen` and DB `modelgen_it` from `docker-compose.yaml`.
- Port `5432` already in use:
  - Stop local PostgreSQL or change `ports` mapping in `docker-compose.yaml`.
- Generated file not updated:
  - Verify `--out-dir` points to `./tools/codesjoy-modelgen/example/output`.
