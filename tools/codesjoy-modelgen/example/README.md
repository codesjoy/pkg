# Example: codesjoy-modelgen (PostgreSQL)

This example is fully reproducible with local Docker:

- starts a PostgreSQL demo database
- runs `codesjoy-modelgen` against the demo table
- includes a committed sample output at `output/user_model_gen.go` for quick inspection

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
  --dsn "postgres://modelgen:modelgen@127.0.0.1:5432/modelgen_it?sslmode=disable" \
  --schema public \
  --tables users \
  --out-dir ./tools/codesjoy-modelgen/example/output \
  --gen-aipsql=true \
  --timestamp-mode unix_nano \
  --override ./tools/codesjoy-modelgen/example/override.yaml
```

`--out-dir` defaults to `./`, and `--package` defaults to the output directory name (or the current working directory name when `--out-dir=./`).

The generated file is:

- `tools/codesjoy-modelgen/example/output/user_model_gen.go`

## Quick Checks

From repo root:

```bash
grep -n 'WithDatabaseName("name")' tools/codesjoy-modelgen/example/output/user_model_gen.go
grep -n 'WithMatchModes(aipsql.MatchModeExact' tools/codesjoy-modelgen/example/output/user_model_gen.go
grep -n 'Name:    "idx_users_created_id"' tools/codesjoy-modelgen/example/output/user_model_gen.go
grep -n 'Columns: \\[\\]string{"created_at", "id"}' tools/codesjoy-modelgen/example/output/user_model_gen.go
grep -n 'WithDatabaseName("deleted_at")' tools/codesjoy-modelgen/example/output/user_model_gen.go
```

## Regenerate Sample Output

Re-run the same generate command above to refresh `output/user_model_gen.go`.

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
